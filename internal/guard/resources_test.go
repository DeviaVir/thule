package guard

import (
	"encoding/json"
	"math"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DeviaVir/thule/internal/render"
	"gopkg.in/yaml.v3"
)

const resourceConfig = `guards:
  - name: routes-need-auth
    description: Rate-limited rules need bearer authentication
    type: list-item-requires
    kind: ApisixRoute
    apiGroup: apisix.apache.org
    list: spec.http[].plugins
    when:
      name: limit-count
      enable: true
    require:
      name: openid-connect
      enable: true
      config.bearer_only: true
`

func TestLoadResourceConfig(t *testing.T) {
	cfg, err := LoadConfig(writeConfig(t, resourceConfig))
	if err != nil || len(cfg.Guards) != 1 {
		t.Fatalf("config: %+v, %v", cfg, err)
	}
	for _, field := range []string{"name", "kind", "list", "when", "require"} {
		t.Run("missing "+field, func(t *testing.T) {
			var doc map[string]any
			if err := yaml.Unmarshal([]byte(resourceConfig), &doc); err != nil {
				t.Fatal(err)
			}
			delete(doc["guards"].([]any)[0].(map[string]any), field)
			data, err := yaml.Marshal(doc)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := LoadConfig(writeConfig(t, string(data))); err == nil || !strings.Contains(err.Error(), field) {
				t.Fatalf("expected %s error, got %v", field, err)
			}
		})
	}
	for _, tc := range []struct{ old, replacement string }{
		{"kind: ApisixRoute", "kind: ' '"},
		{"list: spec.http[].plugins", "list: spec..plugins"},
		{"list: spec.http[].plugins", "list: spec.http[0].plugins"},
		{"list: spec.http[].plugins", "list: spec.http[].plugins[]"},
		{"name: limit-count", "'': limit-count"},
		{"name: limit-count", "'name[]': limit-count"},
		{"name: limit-count", "name: [limit-count]"},
		{"name: limit-count", "name: {nested: value}"},
		{"enable: true", "enable: .inf"},
	} {
		if _, err := LoadConfig(writeConfig(t, strings.Replace(resourceConfig, tc.old, tc.replacement, 1))); err == nil {
			t.Errorf("accepted %s", tc.replacement)
		}
	}
	for _, field := range []string{"when", "require"} {
		g := cfg.Guards[0]
		if field == "when" {
			g.When = map[string]any{}
		} else {
			g.Require = map[string]any{}
		}
		if err := validateListItemRequires(g); err == nil {
			t.Errorf("accepted empty %s", field)
		}
	}
	if vs, touched := Evaluate(cfg, []string{"anything.yaml"}); len(vs) != 0 || touched {
		t.Fatal("path evaluator must skip resource guards")
	}
}

func TestTypedFieldMatching(t *testing.T) {
	for _, tc := range []struct {
		name         string
		actual, want any
		match        bool
	}{
		{"bool", true, true, true}, {"string true", "true", true, false}, {"integer one", 1, true, false},
		{"exact string", "auth", "auth", true}, {"case sensitive", "Auth", "auth", false},
		{"number", 1, float64(1), true}, {"decimal", float32(1.5), float64(1.5), true},
		{"different number", 2, 1, false}, {"numeric string", "1", 1, false},
		{"large integer", uint64(9007199254740993), int64(9007199254740993), true},
		{"large integer precision", uint64(9007199254740993), float64(9007199254740992), false},
		{"json number", json.Number("1e2"), 100, true}, {"null", nil, nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := matchesFields(map[string]any{"config": map[string]any{"value": tc.actual}}, map[string]any{"config.value": tc.want}); got != tc.match {
				t.Fatalf("got %v", got)
			}
		})
	}
	for _, item := range []any{nil, "text", map[string]any{}, map[string]any{"config": true}, map[string]any{"config": map[string]any{}}} {
		if matchesFields(item, map[string]any{"config.value": nil}) {
			t.Fatalf("missing field matched: %#v", item)
		}
	}
	for _, v := range []any{nil, true, "1", math.Inf(1), json.Number("invalid")} {
		if _, ok := numberValue(v); ok {
			t.Errorf("accepted number %#v", v)
		}
	}
}

func routeResource(t *testing.T, rules string) render.Resource {
	t.Helper()
	var body map[string]any
	if err := yaml.Unmarshal([]byte("spec:\n  http:\n"+rules), &body); err != nil {
		t.Fatal(err)
	}
	return render.Resource{APIVersion: "apisix.apache.org/v2", Kind: "ApisixRoute", Namespace: "web", Name: "routes", SourcePath: "clusters/prod/routes.yaml", Body: body}
}

const triggerRule = `    - name: public
      plugins:
        - name: limit-count
          enable: true
`
const authItem = `        - name: openid-connect
          enable: true
          config:
            bearer_only: true
`

func TestEvaluateResourceLists(t *testing.T) {
	cfg, err := LoadConfig(writeConfig(t, resourceConfig))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, rules string
		count       int
	}{
		{"same rule", triggerRule + authItem, 0},
		{"missing auth", triggerRule, 1},
		{"different rule", triggerRule + "    - name: private\n      plugins:\n" + authItem, 1},
		{"two violations", triggerRule + strings.Replace(triggerRule, "public", "other", 1), 2},
		{"no trigger", "    - plugins:\n" + authItem, 0},
		{"disabled trigger", strings.Replace(triggerRule, "true", "false", 1), 0},
		{"string enabled", strings.Replace(triggerRule, "true", `"true"`, 1), 0},
		{"disabled auth", triggerRule + strings.Replace(authItem, "enable: true", "enable: false", 1), 1},
		{"non bearer", triggerRule + strings.Replace(authItem, "bearer_only: true", "bearer_only: false", 1), 1},
		{"unnamed parent", strings.Replace(triggerRule, "name: public\n      ", "", 1), 1},
		{"missing list", "    - name: empty\n", 0},
		{"wrong list type", "    - plugins: {}\n", 0},
		{"non object parent", "    - text\n", 0},
		{"empty parents", "    []\n", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			vs, touched := EvaluateResources(cfg, []render.Resource{routeResource(t, tc.rules)}, "/repo")
			if !touched || len(vs) != tc.count {
				t.Fatalf("got %v, %v", vs, touched)
			}
			if tc.name == "missing auth" {
				for _, want := range []string{"routes-need-auth", "ApisixRoute web/routes", "clusters/prod/routes.yaml", "spec.http[0] (name: public)", "openid-connect", "config.bearer_only", "same list"} {
					if !strings.Contains(vs[0].Message(), want) {
						t.Errorf("message missing %q: %s", want, vs[0].Message())
					}
				}
			}
			if tc.name == "unnamed parent" && !strings.Contains(vs[0].Message(), "spec.http[0]") {
				t.Fatal(vs[0].Message())
			}
		})
	}
	a := routeResource(t, triggerRule)
	b := routeResource(t, "    - plugins:\n"+authItem)
	b.Name = "other"
	if vs, _ := EvaluateResources(cfg, []render.Resource{a, b}, "/repo"); len(vs) != 1 {
		t.Fatalf("cross-resource match: %+v", vs)
	}
}

func TestResourceSelectorsAndPaths(t *testing.T) {
	cfg, err := LoadConfig(writeConfig(t, resourceConfig))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, kind, version, source, prefix string
		touched                             bool
	}{
		{"absolute source", "ApisixRoute", "apisix.apache.org/v2", "/repo/clusters/prod/routes.yaml", "clusters/prod/", true},
		{"relative source", "ApisixRoute", "apisix.apache.org/v2", "clusters/prod/routes.yaml", "clusters/prod", true},
		{"different prefix", "ApisixRoute", "apisix.apache.org/v2", "clusters/dev/routes.yaml", "clusters/prod", false},
		{"prefix boundary", "ApisixRoute", "apisix.apache.org/v2", "clusters/production/routes.yaml", "clusters/prod", false},
		{"wrong kind", "ConfigMap", "apisix.apache.org/v2", "clusters/prod/routes.yaml", "", false},
		{"wrong group", "ApisixRoute", "other.example/v2", "clusters/prod/routes.yaml", "", false},
		{"core group", "ApisixRoute", "v1", "clusters/prod/routes.yaml", "", false},
		{"missing source", "ApisixRoute", "apisix.apache.org/v2", "", "clusters/prod", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := routeResource(t, triggerRule)
			r.Kind = tc.kind
			r.APIVersion = tc.version
			r.SourcePath = tc.source
			cfg.Guards[0].Prefix = tc.prefix
			vs, touched := EvaluateResources(cfg, []render.Resource{r}, "/repo")
			if touched != tc.touched || (len(vs) > 0) != tc.touched {
				t.Fatalf("got %+v %v", vs, touched)
			}
			if touched && strings.Contains(vs[0].Message(), filepath.Join("/repo", "clusters")) {
				t.Fatal("source must be relative")
			}
		})
	}
	cfg.Guards[0].Prefix = ""
	cfg.Guards[0].APIGroup = ""
	cfg.Guards = append(cfg.Guards, validSpec())
	r := routeResource(t, triggerRule)
	r.APIVersion = "v1"
	if vs, touched := EvaluateResources(cfg, []render.Resource{r}, "/repo"); !touched || len(vs) != 1 {
		t.Fatal("optional group or path guard dispatch")
	}
	cfg.Guards[0].List = "plugins"
	r.Body = map[string]any{"plugins": []any{map[string]any{"name": "limit-count", "enable": true}}}
	if vs, _ := EvaluateResources(cfg, []render.Resource{r}, "/repo"); len(vs) != 1 || !strings.Contains(vs[0].Message(), "at plugins:") {
		t.Fatalf("root list: %+v", vs)
	}
	cfg.Guards[0].List = "spec.groups[].rules[].plugins"
	var body map[string]any
	if err := yaml.Unmarshal([]byte("spec:\n  groups:\n    - rules:\n        - name: nested\n          plugins:\n            - name: limit-count\n              enable: true\n"), &body); err != nil {
		t.Fatal(err)
	}
	r.Body = body
	if vs, _ := EvaluateResources(cfg, []render.Resource{r}, "/repo"); len(vs) != 1 || !strings.Contains(vs[0].Message(), "spec.groups[0].rules[0] (name: nested)") {
		t.Fatalf("nested lists: %+v", vs)
	}
}
