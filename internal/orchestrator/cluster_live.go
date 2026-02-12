//go:build live

package orchestrator

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/example/thule/internal/render"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	api "google.golang.org/api/container/v1"
	"google.golang.org/api/option"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	cached "k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
)

type LiveClusterReader struct {
	mu      sync.Mutex
	service *api.Service
	creds   *google.Credentials
	clients map[string]*liveClient

	localClusterRefs map[string]struct{}
	kubeconfigDir    string
}

const liveRequestTimeout = 15 * time.Second

type liveClient struct {
	dynamic dynamic.Interface
	mapper  *restmapper.DeferredDiscoveryRESTMapper
}

func NewLiveClusterReader() (*LiveClusterReader, error) {
	ctx := context.Background()
	creds, err := google.FindDefaultCredentials(ctx, api.CloudPlatformScope)
	if err != nil {
		return nil, fmt.Errorf("gcp credentials: %w", err)
	}
	service, err := api.NewService(ctx, option.WithCredentials(creds))
	if err != nil {
		return nil, fmt.Errorf("container api client: %w", err)
	}
	return &LiveClusterReader{
		service:          service,
		creds:            creds,
		clients:          map[string]*liveClient{},
		localClusterRefs: parseCSVSet(getEnv("THULE_LOCAL_CLUSTER_REFS", "b14,cadmus")),
		kubeconfigDir:    strings.TrimSpace(os.Getenv("THULE_KUBECONFIG_DIR")),
	}, nil
}

func (l *LiveClusterReader) ListResources(_ context.Context, _ string, _ string) ([]render.Resource, error) {
	// Planner calls ListResourcesWithProject when available.
	return nil, nil
}

func (l *LiveClusterReader) ListResourcesWithProject(ctx context.Context, projectID, clusterRef, namespace string, desired []render.Resource) ([]render.Resource, error) {
	client, err := l.getClient(ctx, projectID, clusterRef)
	if err != nil {
		return nil, err
	}
	out := make([]render.Resource, 0, len(desired))
	for _, d := range desired {
		gv, err := schema.ParseGroupVersion(d.APIVersion)
		if err != nil {
			continue
		}
		mapping, err := client.mapper.RESTMapping(schema.GroupKind{Group: gv.Group, Kind: d.Kind}, gv.Version)
		if err != nil {
			continue
		}
		target := client.dynamic.Resource(mapping.Resource)
		ns := d.Namespace
		if ns == "" && namespace != "" && namespace != "all" {
			ns = namespace
		}
		var getter dynamic.ResourceInterface
		if mapping.Scope.Name() != "root" {
			if ns == "" || ns == "all" {
				continue
			}
			getter = target.Namespace(ns)
		} else {
			getter = target
		}
		reqCtx, cancel := context.WithTimeout(ctx, liveRequestTimeout)
		obj, err := getter.Get(reqCtx, d.Name, metav1.GetOptions{})
		cancel()
		if errors.IsNotFound(err) {
			continue
		}
		if errors.IsForbidden(err) {
			// Some IAM profiles intentionally cannot read sensitive objects (e.g. Secrets).
			// Keep these resources neutral in the diff rather than failing the whole plan.
			out = append(out, render.Resource{
				APIVersion: d.APIVersion,
				Kind:       d.Kind,
				Namespace:  d.Namespace,
				Name:       d.Name,
				Body:       d.Body,
			})
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("get live resource %s: %w", d.ID(), err)
		}
		out = append(out, render.Resource{
			APIVersion: obj.GetAPIVersion(),
			Kind:       obj.GetKind(),
			Namespace:  obj.GetNamespace(),
			Name:       obj.GetName(),
			Body:       obj.Object,
		})
	}
	return out, nil
}

func (l *LiveClusterReader) getClient(ctx context.Context, projectID, clusterRef string) (*liveClient, error) {
	key := projectID + "/" + clusterRef
	l.mu.Lock()
	if c, ok := l.clients[key]; ok {
		l.mu.Unlock()
		return c, nil
	}
	l.mu.Unlock()

	if c, ok, err := l.clientFromKubeconfig(clusterRef); err != nil {
		return nil, err
	} else if ok {
		l.mu.Lock()
		l.clients[key] = c
		l.mu.Unlock()
		return c, nil
	}

	if _, ok := l.localClusterRefs[clusterRef]; ok {
		c, err := l.clientFromInCluster()
		if err != nil {
			return nil, err
		}
		l.mu.Lock()
		l.clients[key] = c
		l.mu.Unlock()
		return c, nil
	}

	cluster, err := l.resolveCluster(ctx, projectID, clusterRef)
	if err != nil {
		return nil, err
	}
	caData, err := base64.StdEncoding.DecodeString(cluster.MasterAuth.ClusterCaCertificate)
	if err != nil {
		return nil, fmt.Errorf("decode cluster CA for %s/%s: %w", projectID, clusterRef, err)
	}
	cfg := &rest.Config{
		Host: "https://" + cluster.Endpoint,
		TLSClientConfig: rest.TLSClientConfig{
			CAData: caData,
		},
	}
	cfg.WrapTransport = func(rt http.RoundTripper) http.RoundTripper {
		return &oauth2.Transport{Base: rt, Source: l.creds.TokenSource}
	}
	client, err := newLiveClient(cfg, fmt.Sprintf("%s/%s", projectID, clusterRef))
	if err != nil {
		return nil, err
	}

	l.mu.Lock()
	l.clients[key] = client
	l.mu.Unlock()
	return client, nil
}

func (l *LiveClusterReader) clientFromInCluster() (*liveClient, error) {
	cfg, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("in-cluster config: %w", err)
	}
	return newLiveClient(cfg, "in-cluster")
}

func (l *LiveClusterReader) clientFromKubeconfig(clusterRef string) (*liveClient, bool, error) {
	if l.kubeconfigDir == "" {
		return nil, false, nil
	}
	path := filepath.Join(l.kubeconfigDir, clusterRef+".conf")
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("stat kubeconfig %s: %w", path, err)
	}
	cfg, err := clientcmd.BuildConfigFromFlags("", path)
	if err != nil {
		return nil, false, fmt.Errorf("load kubeconfig %s: %w", path, err)
	}
	client, err := newLiveClient(cfg, path)
	if err != nil {
		return nil, false, err
	}
	return client, true, nil
}

func newLiveClient(cfg *rest.Config, ref string) (*liveClient, error) {
	cfgCopy := rest.CopyConfig(cfg)
	if cfgCopy.Timeout == 0 {
		cfgCopy.Timeout = liveRequestTimeout
	}
	dc, err := dynamic.NewForConfig(cfgCopy)
	if err != nil {
		return nil, fmt.Errorf("dynamic client for %s: %w", ref, err)
	}
	disc, err := discovery.NewDiscoveryClientForConfig(cfgCopy)
	if err != nil {
		return nil, fmt.Errorf("discovery client for %s: %w", ref, err)
	}
	return &liveClient{
		dynamic: dc,
		mapper:  restmapper.NewDeferredDiscoveryRESTMapper(cached.NewMemCacheClient(disc)),
	}, nil
}

func parseCSVSet(raw string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, part := range strings.Split(raw, ",") {
		val := strings.TrimSpace(part)
		if val == "" {
			continue
		}
		out[val] = struct{}{}
	}
	return out
}

func getEnv(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func (l *LiveClusterReader) resolveCluster(ctx context.Context, projectID, clusterRef string) (*api.Cluster, error) {
	parent := fmt.Sprintf("projects/%s/locations/-", projectID)
	reqCtx, cancel := context.WithTimeout(ctx, liveRequestTimeout)
	defer cancel()
	resp, err := l.service.Projects.Locations.Clusters.List(parent).Context(reqCtx).Do()
	if err != nil {
		return nil, fmt.Errorf("list clusters in project %s: %w", projectID, err)
	}
	for _, c := range resp.Clusters {
		if c.Name == clusterRef {
			return c, nil
		}
	}
	return nil, fmt.Errorf("cluster %q not found in project %q", clusterRef, projectID)
}
