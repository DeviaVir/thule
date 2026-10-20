package orchestrator

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DeviaVir/thule/internal/render"
	"github.com/DeviaVir/thule/internal/vcs"
)

const listGuardConfig = `guards:
  - name: routes-need-auth
    description: Rate-limited rules need bearer authentication
    type: list-item-requires
    kind: ApisixRoute
    list: spec.http[].plugins
    when:
      name: limit-count
      enable: true
    require:
      name: openid-connect
      enable: true
      config.bearer_only: true
  - name: independent-groups
    type: same-app-across-groups
    prefix: clusters/prod
`
const routeManifest = `apiVersion: apisix.apache.org/v2
kind: ApisixRoute
metadata:
  name: routes
  namespace: web
spec:
  http:
    - name: public
      plugins:
        - name: limit-count
          enable: true
`
const bearerPlugin = `        - name: openid-connect
          enable: true
          config:
            bearer_only: true
`

func writeGuardPlannerFile(t *testing.T, repo, path, content string) {
	t.Helper()
	target := filepath.Join(repo, path)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
func guardPlannerRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	writeGuardPlannerFile(t, repo, ".thule.yaml", listGuardConfig)
	writeGuardPlannerFile(t, repo, "apps/routes/thule.conf", "version: v1\nproject: routes\nclusterRef: test\nnamespace: web\nrender:\n  mode: yaml\n  path: manifests\n")
	writeGuardPlannerFile(t, repo, "apps/routes/manifests/route.yaml", routeManifest)
	return repo
}
func assertGuardStatus(t *testing.T, statuses *vcs.MemoryStatusPublisher, want vcs.CheckState) {
	t.Helper()
	count := 0
	for _, s := range statuses.ListStatuses(1, "local") {
		if s.Context != "thule/guards" {
			continue
		}
		count++
		if s.State != want {
			t.Errorf("guard status: %+v, want %s", s, want)
		}
	}
	expected := 1
	if want == "" {
		expected = 0
	}
	if count != expected {
		t.Fatalf("got %d guard statuses, want %d", count, expected)
	}
}

func TestPlannerResourceGuards(t *testing.T) {
	for _, tc := range []struct {
		name, manifest string
		changed        []string
		status         vcs.CheckState
		banner         bool
	}{
		{"violation", routeManifest, []string{"apps/routes/manifests/route.yaml"}, vcs.CheckFailed, true},
		{"compliant", routeManifest + bearerPlugin, []string{"apps/routes/manifests/route.yaml"}, vcs.CheckSuccess, false},
		{"no trigger", strings.Replace(routeManifest, "enable: true", "enable: false", 1), []string{"apps/routes/manifests/route.yaml"}, vcs.CheckSuccess, false},
		{"unrelated kind", strings.Replace(routeManifest, "kind: ApisixRoute", "kind: ConfigMap", 1), []string{"apps/routes/manifests/route.yaml"}, "", false},
		{"unchanged route", routeManifest, []string{"apps/routes/manifests/map.yaml"}, "", false},
		{"no project", routeManifest, []string{"README.md"}, "", false},
		{"combined", routeManifest, []string{"apps/routes/manifests/route.yaml", "clusters/prod/eu/db/a.yaml", "clusters/prod/us/db/a.yaml"}, vcs.CheckFailed, true},
		{"path violation with compliant resource", routeManifest + bearerPlugin, []string{"apps/routes/manifests/route.yaml", "clusters/prod/eu/db/a.yaml", "clusters/prod/us/db/a.yaml"}, vcs.CheckFailed, true},
		{"path banner without plan", routeManifest, []string{"clusters/prod/eu/db/a.yaml", "clusters/prod/us/db/a.yaml"}, vcs.CheckFailed, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := guardPlannerRepo(t)
			writeGuardPlannerFile(t, repo, "apps/routes/manifests/route.yaml", tc.manifest)
			writeGuardPlannerFile(t, repo, "apps/routes/manifests/map.yaml", "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: unrelated\n")
			comments := vcs.NewMemoryCommentStore()
			statuses := vcs.NewMemoryStatusPublisher()
			planner := NewPlanner(repo, &MemoryClusterReader{}, comments, statuses, nil, nil)
			if err := planner.PlanForEvent(context.Background(), MergeRequestEvent{MergeReqID: 1, HeadSHA: "local", ChangedFiles: tc.changed}); err != nil {
				t.Fatal(err)
			}
			assertGuardStatus(t, statuses, tc.status)
			items := comments.List(1)
			if len(items) != 1 {
				t.Fatalf("comments: %+v", items)
			}
			body := items[0].Body
			if got := strings.HasPrefix(body, "## :rotating_light: Guard violations"); got != tc.banner {
				t.Fatalf("banner: %s", body)
			}
			if tc.name == "violation" || tc.name == "combined" {
				for _, want := range []string{"routes-need-auth", "Rate-limited rules need bearer authentication", "ApisixRoute web/routes", "apps/routes/manifests/route.yaml", "spec.http[0] (name: public)", "openid-connect"} {
					if !strings.Contains(body, want) {
						t.Errorf("missing %q: %s", want, body)
					}
				}
			}
			if strings.Contains(tc.name, "path") || tc.name == "combined" {
				if !strings.Contains(body, "independent-groups") {
					t.Fatal("missing path guard")
				}
			}
		})
	}
}

func TestPlannerGuardStatusOnEarlyError(t *testing.T) {
	for _, failure := range []string{"render", "cluster", "config"} {
		for _, violate := range []bool{false, true} {
			t.Run(failure+map[bool]string{false: " clean", true: " violation"}[violate], func(t *testing.T) {
				repo := guardPlannerRepo(t)
				var cluster ClusterReader = &MemoryClusterReader{}
				switch failure {
				case "render":
					writeGuardPlannerFile(t, repo, "apps/routes/manifests/route.yaml", "apiVersion: v1\nkind: ApisixRoute\nspec: [")
				case "cluster":
					cluster = &errCluster{}
				case "config":
					writeGuardPlannerFile(t, repo, "apps/routes/thule.conf", "version: [")
				}
				changed := []string{"apps/routes/manifests/route.yaml", "clusters/prod/eu/db/a.yaml"}
				want := vcs.CheckSuccess
				if violate {
					changed = append(changed, "clusters/prod/us/db/a.yaml")
					want = vcs.CheckFailed
				}
				statuses := vcs.NewMemoryStatusPublisher()
				planner := NewPlanner(repo, cluster, vcs.NewMemoryCommentStore(), statuses, nil, nil)
				if err := planner.PlanForEvent(context.Background(), MergeRequestEvent{MergeReqID: 1, HeadSHA: "local", ChangedFiles: changed}); err == nil {
					t.Fatal("expected error")
				}
				assertGuardStatus(t, statuses, want)
			})
		}
	}
}

type guardCheckingCluster struct {
	t        *testing.T
	statuses *vcs.MemoryStatusPublisher
}

func (c *guardCheckingCluster) ListResources(context.Context, string, string) ([]render.Resource, error) {
	c.t.Helper()
	assertGuardStatus(c.t, c.statuses, "")
	return nil, nil
}
func TestPlannerPublishesGuardsAfterAllProjects(t *testing.T) {
	repo := guardPlannerRepo(t)
	writeGuardPlannerFile(t, repo, "apps/second/thule.conf", "version: v1\nproject: second\nclusterRef: test\nnamespace: web\nrender:\n  mode: yaml\n  path: manifests\n")
	writeGuardPlannerFile(t, repo, "apps/second/manifests/route.yaml", routeManifest+bearerPlugin)
	statuses := vcs.NewMemoryStatusPublisher()
	comments := vcs.NewMemoryCommentStore()
	planner := NewPlanner(repo, &guardCheckingCluster{t, statuses}, comments, statuses, nil, nil)
	if err := planner.PlanForEvent(context.Background(), MergeRequestEvent{MergeReqID: 1, HeadSHA: "local", ChangedFiles: []string{"apps/routes/manifests/route.yaml", "apps/second/manifests/route.yaml"}}); err != nil {
		t.Fatal(err)
	}
	assertGuardStatus(t, statuses, vcs.CheckFailed)
	if !strings.Contains(comments.List(1)[0].Body, "routes-need-auth") {
		t.Fatal("lost first project violation")
	}
}
