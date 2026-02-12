package render

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/example/thule/pkg/thuleconfig"
	"gopkg.in/yaml.v3"
)

var (
	apiVersionPattern = regexp.MustCompile(`(?m)^apiVersion:\s*\S+`)
	kindPattern       = regexp.MustCompile(`(?m)^kind:\s*\S+`)
)

type Resource struct {
	APIVersion string
	Kind       string
	Namespace  string
	Name       string
	Body       map[string]any
	SourcePath string
}

func (r Resource) ID() string {
	ns := r.Namespace
	if ns == "" {
		ns = "_cluster"
	}
	return fmt.Sprintf("%s|%s|%s|%s", r.APIVersion, r.Kind, ns, r.Name)
}

func RenderProject(projectRoot string, cfg thuleconfig.Config) ([]Resource, error) {
	target := filepath.Join(projectRoot, cfg.Render.Path)
	switch cfg.Render.Mode {
	case "yaml", "kustomize", "helm":
		return renderYAMLPath(target)
	case "flux":
		resources, err := renderYAMLPath(target)
		if err != nil {
			return nil, err
		}
		return filterFluxResources(resources, cfg), nil
	default:
		return nil, fmt.Errorf("render mode %q not implemented", cfg.Render.Mode)
	}
}

func filterFluxResources(resources []Resource, cfg thuleconfig.Config) []Resource {
	if len(cfg.Render.Flux.IncludeKinds) == 0 {
		return resources
	}

	allowed := map[string]struct{}{
		"HelmRelease":   {},
		"Kustomization": {},
		"GitRepository": {},
		"OCIRepository": {},
	}
	for _, k := range cfg.Render.Flux.IncludeKinds {
		allowed[k] = struct{}{}
	}

	out := make([]Resource, 0, len(resources))
	for _, r := range resources {
		if _, ok := allowed[r.Kind]; ok {
			out = append(out, r)
		}
	}
	return out
}

func renderYAMLPath(path string) ([]Resource, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	files := []string{}
	if fi.IsDir() {
		err := filepath.WalkDir(path, func(p string, d os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if d.IsDir() {
				return nil
			}
			if strings.HasSuffix(d.Name(), ".yaml") || strings.HasSuffix(d.Name(), ".yml") {
				files = append(files, p)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	} else {
		files = append(files, path)
	}
	sort.Strings(files)

	var out []Resource
	for _, f := range files {
		content, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		res, err := parseYAMLDocumentsWithSource(string(content), f)
		if err != nil {
			if looksLikeKubernetesManifest(string(content)) {
				return nil, fmt.Errorf("parse %s: %w", f, err)
			}
			continue
		}
		out = append(out, res...)
	}
	return out, nil
}

func parseYAMLDocuments(content string) ([]Resource, error) {
	return parseYAMLDocumentsWithSource(content, "")
}

func parseYAMLDocumentsWithSource(content, sourcePath string) ([]Resource, error) {
	dec := yaml.NewDecoder(strings.NewReader(content))
	out := []Resource{}
	for {
		var doc map[string]any
		if err := dec.Decode(&doc); err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		if len(doc) == 0 {
			continue
		}
		apiVersion, _ := doc["apiVersion"].(string)
		kind, _ := doc["kind"].(string)
		meta, _ := doc["metadata"].(map[string]any)
		name, _ := meta["name"].(string)
		namespace, _ := meta["namespace"].(string)
		if apiVersion == "" || kind == "" || name == "" {
			// Skip non-resource YAML (values files, kustomize configs, etc.).
			continue
		}
		out = append(out, Resource{
			APIVersion: apiVersion,
			Kind:       kind,
			Namespace:  namespace,
			Name:       name,
			Body:       doc,
			SourcePath: sourcePath,
		})
	}
	return out, nil
}

func looksLikeKubernetesManifest(content string) bool {
	return apiVersionPattern.MatchString(content) && kindPattern.MatchString(content)
}
