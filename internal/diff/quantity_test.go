package diff

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/DeviaVir/thule/internal/render"
	"gopkg.in/yaml.v3"
)

func quantityDeployment(t *testing.T, cpuYAML string) render.Resource {
	t.Helper()
	var body map[string]any
	err := yaml.Unmarshal([]byte(fmt.Sprintf(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: example
  namespace: default
spec:
  replicas: 1
  selector:
    matchLabels:
      app: example
  template:
    metadata:
      labels:
        app: example
    spec:
      containers:
        - name: app
          image: example:v1
          resources:
            limits:
              cpu: %s
`, cpuYAML)), &body)
	if err != nil {
		t.Fatal(err)
	}
	return render.Resource{APIVersion: "apps/v1", Kind: "Deployment", Namespace: "default", Name: "example", Body: body}
}

func assertQuantityNoOp(t *testing.T, changes []Change, summary Summary) {
	t.Helper()
	if summary != (Summary{NoOps: 1}) || len(changes) != 1 || changes[0].Action != NoOp {
		t.Fatalf("expected NO-OP, got changes=%+v summary=%+v", changes, summary)
	}
	ch := changes[0]
	if len(ch.ChangedKeys) != 0 || len(ch.ChangedPaths) != 0 || len(ch.AttributeDiff) != 0 || len(ch.Risks) != 0 || ch.CurrentYAML != "" || ch.DesiredYAML != "" {
		t.Fatalf("NO-OP must have no change details or risks: %+v", ch)
	}
}

func TestComputeQuantityPairs(t *testing.T) {
	for _, tc := range []struct {
		name, desired, actual string
		patch                 bool
	}{
		{name: "cpu millicores", desired: `"1000m"`, actual: `"1"`},
		{name: "binary memory", desired: `"1024Mi"`, actual: `"1Gi"`},
		{name: "fractional cpu", desired: `"500m"`, actual: `"0.5"`},
		{name: "numeric YAML integer", desired: `1`, actual: `"1"`},
		{name: "numeric YAML fraction", desired: `0.5`, actual: `"500m"`},
		{name: "decimal and binary formats", desired: `"1024"`, actual: `"1Ki"`},
		{name: "unequal cpu", desired: `"1"`, actual: `"2"`, patch: true},
		{name: "unequal memory", desired: `"52Mi"`, actual: `"30Mi"`, patch: true},
		{name: "unequal decimal quantities", desired: `"128974848"`, actual: `"129e6"`, patch: true},
		{name: "invalid desired", desired: `"invalid"`, actual: `"1"`, patch: true},
		{name: "invalid actual", desired: `"1"`, actual: `"invalid"`, patch: true},
		{name: "identical invalid values", desired: `"invalid"`, actual: `"invalid"`},
		{name: "invalid boolean", desired: `true`, actual: `"true"`, patch: true},
	} {
		for _, project := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/project=%t", tc.name, project), func(t *testing.T) {
				desired, actual := quantityDeployment(t, tc.desired), quantityDeployment(t, tc.actual)
				beforeDesired, beforeActual := mustYAML(desired.Body), mustYAML(actual.Body)
				changes, summary := Compute([]render.Resource{desired}, []render.Resource{actual}, Options{IgnoreActualExtraFields: project})
				if !tc.patch {
					assertQuantityNoOp(t, changes, summary)
				} else {
					if summary != (Summary{Patches: 1}) || len(changes) != 1 || changes[0].Action != Patch {
						t.Fatalf("expected PATCH, got changes=%+v summary=%+v", changes, summary)
					}
					if !reflect.DeepEqual(changes[0].ChangedPaths, []string{"spec.template.spec.containers"}) || len(changes[0].AttributeDiff) != 2 || !strings.Contains(changes[0].AttributeDiff[0], ".resources.limits.cpu:") {
						t.Fatalf("expected quantity change details: %+v", changes[0])
					}
				}
				if mustYAML(desired.Body) != beforeDesired || mustYAML(actual.Body) != beforeActual {
					t.Fatal("Compute mutated input resources")
				}
			})
		}
	}
}

func TestComputeQuantityWithRealChange(t *testing.T) {
	for _, tc := range []struct {
		name, path, key, risk string
		mutate                func(map[string]any)
		lines                 []string
	}{
		{
			name: "replicas", path: "spec.replicas", key: "spec", risk: "workload-spec-change",
			mutate: func(body map[string]any) { body["spec"].(map[string]any)["replicas"] = 2 },
			lines:  []string{`- spec.replicas: 1`, `+ spec.replicas: 2`},
		},
		{
			name: "metadata only risk", path: "metadata.labels", key: "metadata", risk: "metadata-change",
			mutate: func(body map[string]any) { body["metadata"].(map[string]any)["labels"] = map[string]any{"team": "new"} },
			lines:  []string{`+ metadata.labels: {"team":"new"}`},
		},
		{
			name: "container image", path: "spec.template.spec.containers", key: "spec", risk: "workload-spec-change",
			mutate: func(body map[string]any) { quantityContainer(body)["image"] = "example:v2" },
			lines: []string{
				`- spec.template.spec.containers[0].image: "example:v1"`,
				`+ spec.template.spec.containers[0].image: "example:v2"`,
			},
		},
		{
			name: "env remains a string", path: "spec.template.spec.containers", key: "spec", risk: "workload-spec-change",
			mutate: func(body map[string]any) {
				quantityContainer(body)["env"] = []any{map[string]any{"name": "VALUE", "value": "1000m"}}
			},
			lines: []string{
				`- spec.template.spec.containers[0].env[0].value: "1"`,
				`+ spec.template.spec.containers[0].env[0].value: "1000m"`,
			},
		},
		{
			name: "resource claims remain strings", path: "spec.template.spec.containers", key: "spec", risk: "workload-spec-change",
			mutate: func(body map[string]any) {
				quantityContainer(body)["resources"].(map[string]any)["claims"] = []any{map[string]any{"name": "claim", "request": "1000m"}}
			},
			lines: []string{
				`- spec.template.spec.containers[0].resources.claims[0].request: "1"`,
				`+ spec.template.spec.containers[0].resources.claims[0].request: "1000m"`,
			},
		},
	} {
		for _, project := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/project=%t", tc.name, project), func(t *testing.T) {
				desired, actual := quantityDeployment(t, `"1000m"`), quantityDeployment(t, `"1"`)
				for _, body := range []map[string]any{desired.Body, actual.Body} {
					quantityContainer(body)["env"] = []any{map[string]any{"name": "VALUE", "value": "1"}}
					quantityContainer(body)["resources"].(map[string]any)["claims"] = []any{map[string]any{"name": "claim", "request": "1"}}
				}
				tc.mutate(desired.Body)
				if project {
					// Quantity pairing must happen after injected containers are removed.
					pod := actual.Body["spec"].(map[string]any)["template"].(map[string]any)["spec"].(map[string]any)
					pod["containers"] = append([]any{map[string]any{
						"name": "injected", "resources": map[string]any{"limits": map[string]any{"cpu": "2"}},
					}}, pod["containers"].([]any)...)
				}
				changes, summary := Compute([]render.Resource{desired}, []render.Resource{actual}, Options{IgnoreActualExtraFields: project})
				if summary != (Summary{Patches: 1}) || len(changes) != 1 || changes[0].Action != Patch {
					t.Fatalf("expected PATCH, got changes=%+v summary=%+v", changes, summary)
				}
				ch := changes[0]
				if !reflect.DeepEqual(ch.ChangedKeys, []string{tc.key}) || !reflect.DeepEqual(ch.ChangedPaths, []string{tc.path}) || !reflect.DeepEqual(ch.AttributeDiff, tc.lines) || !reflect.DeepEqual(ch.Risks, []string{tc.risk}) {
					t.Fatalf("expected only real change and risk, got %+v", ch)
				}
			})
		}
	}
}

func quantityContainer(body map[string]any) map[string]any {
	return body["spec"].(map[string]any)["template"].(map[string]any)["spec"].(map[string]any)["containers"].([]any)[0].(map[string]any)
}

func TestComputeQuantityLocations(t *testing.T) {
	type location struct{ kind, path string }
	locations := []location{
		{"PersistentVolumeClaim", "spec.resources.requests.storage"},
		{"PersistentVolumeClaim", "spec.resources.limits.storage"},
		{"ResourceQuota", "spec.hard.memory"},
	}
	for _, field := range []string{"max", "min", "default", "defaultRequest", "maxLimitRequestRatio"} {
		locations = append(locations, location{"LimitRange", "spec.limits[]." + field + ".memory"})
	}
	for _, workload := range []location{
		{"Pod", "spec"},
		{"Deployment", "spec.template.spec"},
		{"StatefulSet", "spec.template.spec"},
		{"DaemonSet", "spec.template.spec"},
		{"Job", "spec.template.spec"},
		{"CronJob", "spec.jobTemplate.spec.template.spec"},
		{"ReplicaSet", "spec.template.spec"},
	} {
		for _, suffix := range []string{"overhead.memory", "resources.requests.memory", "resources.limits.memory", "volumes[].emptyDir.sizeLimit"} {
			locations = append(locations, location{workload.kind, workload.path + "." + suffix})
		}
		for _, container := range []string{"containers", "initContainers", "ephemeralContainers"} {
			for _, field := range []string{"requests", "limits"} {
				locations = append(locations, location{workload.kind, workload.path + "." + container + "[].resources." + field + ".memory"})
			}
		}
	}
	for _, loc := range locations {
		t.Run(loc.kind+"/"+loc.path, func(t *testing.T) {
			makeResource := func(value string) render.Resource {
				var node any = value
				parts := strings.Split(loc.path, ".")
				for i := len(parts) - 1; i >= 0; i-- {
					key := parts[i]
					if strings.HasSuffix(key, "[]") {
						key = strings.TrimSuffix(key, "[]")
						node = []any{node}
					}
					node = map[string]any{key: node}
				}
				return render.Resource{Kind: loc.kind, Name: "example", Body: node.(map[string]any)}
			}
			for _, project := range []bool{false, true} {
				for _, pair := range []struct{ desired, actual string }{{"1024Mi", "1Gi"}, {"52Mi", "30Mi"}} {
					changes, summary := Compute([]render.Resource{makeResource(pair.desired)}, []render.Resource{makeResource(pair.actual)}, Options{IgnoreActualExtraFields: project})
					if pair.desired == "1024Mi" {
						assertQuantityNoOp(t, changes, summary)
					} else if summary != (Summary{Patches: 1}) || len(changes) != 1 || len(changes[0].ChangedPaths) != 1 || len(changes[0].AttributeDiff) != 2 {
						t.Fatalf("expected unequal quantity to change, got changes=%+v summary=%+v", changes, summary)
					}
				}
			}
		})
	}
}

func TestNormalizeQuantityPairsPreservesStructuralDifferences(t *testing.T) {
	for _, tc := range []struct {
		name            string
		desired, actual any
	}{
		{"missing quantity is not zero", map[string]any{"limits": map[string]any{"cpu": "0"}}, map[string]any{"limits": map[string]any{}}},
		{"removed quantity is not zero", map[string]any{"limits": map[string]any{}}, map[string]any{"limits": map[string]any{"cpu": "0"}}},
		{"map becomes scalar", map[string]any{"limits": map[string]any{"cpu": "1"}}, map[string]any{"limits": "1"}},
		{"scalar becomes map", map[string]any{"limits": "1"}, map[string]any{"limits": map[string]any{"cpu": "1"}}},
		{"list becomes scalar", []any{map[string]any{"limits": map[string]any{"cpu": "1"}}}, "1"},
		{"list grows", []any{"1", "2"}, []any{"1"}},
		{"list shrinks", []any{"1"}, []any{"1", "2"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := mustYAML(tc.actual)
			normalizeQuantityPairs(tc.desired, tc.actual, "")
			if got := mustYAML(tc.actual); got != before {
				t.Fatalf("structural change was modified: got %s, want %s", got, before)
			}
			if equalAny(tc.desired, tc.actual) {
				t.Fatal("structural difference must remain visible")
			}
		})
	}
}

func TestParseQuantity(t *testing.T) {
	equal := [][2]string{
		{"1", "1000m"}, {"-1", "-1000m"}, {"1.", "1"}, {".5", "500m"}, {"+1", "1"},
		{"1e-3", "1m"}, {"1E2", "100"}, {"1e+09", "1G"}, {"10e-1", "1"}, {"1Ei", "1152921504606846976"},
		{"1n", "0.000000001"}, {"1u", "1000n"}, {"1Gi", "1024Mi"},
	}
	for _, pair := range equal {
		a, aok := parseQuantity(pair[0])
		b, bok := parseQuantity(pair[1])
		if !aok || !bok || a.Cmp(b) != 0 {
			t.Errorf("parseQuantity(%q) and parseQuantity(%q) should be equal valid quantities", pair[0], pair[1])
		}
	}
	for _, s := range []string{"", ".", "+", "1..2", "1.2.3", "abc", "1 Gi", " 1", "1e", "1e1.5", "1ee1", "0x10", "1_000", "1/2", "1Gib", "1mi", "1e1001", "1e-1001"} {
		if _, ok := parseQuantity(s); ok {
			t.Errorf("parseQuantity(%q) should be invalid", s)
		}
	}
}
