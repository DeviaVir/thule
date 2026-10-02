package diff

import (
	"reflect"
	"strings"
	"testing"

	"github.com/DeviaVir/thule/internal/render"
)

func keyedListDeployment(env []any, image string, managedFields ...any) render.Resource {
	return render.Resource{APIVersion: "apps/v1", Kind: "Deployment", Namespace: "n", Name: "litellm", Body: map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata":   map[string]any{"name": "litellm", "namespace": "n", "managedFields": managedFields},
		"spec": map[string]any{"template": map[string]any{"spec": map[string]any{
			"containers": []any{
				map[string]any{"name": "litellm", "image": "litellm:v1", "env": env},
				map[string]any{"name": "sidecar", "image": image},
			},
		}}},
	}}
}

func keyedEnvOwnership(manager, operation string, envFields map[string]any) map[string]any {
	return map[string]any{
		"manager": manager, "operation": operation, "fieldsType": "FieldsV1",
		"fieldsV1": map[string]any{"f:spec": map[string]any{"f:template": map[string]any{"f:spec": map[string]any{
			"f:containers": map[string]any{`k:{"name":"litellm"}`: map[string]any{"f:env": envFields}},
		}}}},
	}
}

func TestComputeIgnoresReloaderEnvEntries(t *testing.T) {
	env := []any{
		map[string]any{"name": "DATABASE_URL", "value": "database-placeholder"},
		map[string]any{"name": "DIRECT_URL", "value": "direct-placeholder"},
	}
	liveEnv := append([]any{
		map[string]any{"name": "STAKATER_A", "value": "a"},
		map[string]any{"name": "STAKATER_B", "value": "b"},
		map[string]any{"name": "STAKATER_C", "value": "c"},
	}, env...)
	actual := keyedListDeployment(liveEnv, "sidecar:v1",
		keyedEnvOwnership("kustomize-controller", "Apply", map[string]any{
			".":                         map[string]any{},
			`k:{"name":"DATABASE_URL"}`: map[string]any{"f:value": map[string]any{}},
			`k:{"name":"DIRECT_URL"}`:   map[string]any{"f:value": map[string]any{}},
		}),
		keyedEnvOwnership("Reloader", "Update", map[string]any{
			`k:{"name":"STAKATER_A"}`: map[string]any{},
			`k:{"name":"STAKATER_B"}`: map[string]any{},
			`k:{"name":"STAKATER_C"}`: map[string]any{},
		}),
	)
	for _, tc := range []struct {
		name   string
		image  string
		action Action
		lines  []string
	}{
		{name: "identical desired entries", image: "sidecar:v1", action: NoOp},
		{name: "only sidecar image changes", image: "sidecar:v2", action: Patch, lines: []string{
			`- spec.template.spec.containers[1].image: "sidecar:v1"`,
			`+ spec.template.spec.containers[1].image: "sidecar:v2"`,
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			desired := keyedListDeployment(env, tc.image)
			changes, summary := Compute([]render.Resource{desired}, []render.Resource{actual}, Options{IgnoreActualExtraFields: true})
			if len(changes) != 1 || changes[0].Action != tc.action {
				t.Fatalf("unexpected changes=%+v summary=%+v", changes, summary)
			}
			if !reflect.DeepEqual(changes[0].AttributeDiff, tc.lines) {
				t.Fatalf("expected only %q, got %q", tc.lines, changes[0].AttributeDiff)
			}
		})
	}
}

func TestComputeKeyedEnvChanges(t *testing.T) {
	keep := map[string]any{"name": "KEEP", "value": "keep"}
	x := map[string]any{"name": "X", "value": "old", "extra": "preserved"}
	for _, tc := range []struct {
		name    string
		desired []any
		actual  []any
		owned   map[string]any
		action  Action
		lines   []string
	}{
		{
			name: "owned item removal", desired: []any{keep}, actual: []any{keep, x},
			owned: map[string]any{`k:{"name":"X"}`: map[string]any{}}, action: Patch,
			lines: []string{
				`- spec.template.spec.containers[0].env: [{"name":"KEEP","value":"keep"},{"extra":"preserved","name":"X","value":"old"}]`,
				`+ spec.template.spec.containers[0].env: [{"name":"KEEP","value":"keep"}]`,
			},
		},
		{
			name:    "owned field removal inside matched item",
			desired: []any{map[string]any{"name": "X"}}, actual: []any{keep, x},
			owned: map[string]any{`k:{"name":"X"}`: map[string]any{"f:value": map[string]any{}}}, action: Patch,
			lines: []string{`- spec.template.spec.containers[0].env[0].value: "old"`},
		},
		{
			name: "unowned live-only item", desired: []any{keep}, actual: []any{x, keep}, action: NoOp,
		},
		{
			name: "desired item missing from live", desired: []any{keep, x}, actual: []any{keep}, action: Patch,
			lines: []string{
				`- spec.template.spec.containers[0].env: [{"name":"KEEP","value":"keep"}]`,
				`+ spec.template.spec.containers[0].env: [{"name":"KEEP","value":"keep"},{"extra":"preserved","name":"X","value":"old"}]`,
			},
		},
		{
			name: "matched items reordered", desired: []any{keep, x}, actual: []any{x, keep}, action: Patch,
			lines: []string{
				`- spec.template.spec.containers[0].env[0].name: "X"`,
				`+ spec.template.spec.containers[0].env[0].name: "KEEP"`,
				`- spec.template.spec.containers[0].env[0].value: "old"`,
				`+ spec.template.spec.containers[0].env[0].value: "keep"`,
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			desired := keyedListDeployment(tc.desired, "sidecar:v1")
			actual := keyedListDeployment(tc.actual, "sidecar:v1", keyedEnvOwnership("kustomize-controller", "Apply", tc.owned))
			changes, summary := Compute([]render.Resource{desired}, []render.Resource{actual}, Options{IgnoreActualExtraFields: true})
			if len(changes) != 1 || changes[0].Action != tc.action {
				t.Fatalf("unexpected changes=%+v summary=%+v", changes, summary)
			}
			got := strings.Join(changes[0].AttributeDiff, "\n")
			for _, line := range tc.lines {
				if !strings.Contains(got, line) {
					t.Fatalf("expected %q in %q", line, got)
				}
			}
		})
	}
}

func TestProjectKeyedVolumeMountsAndPorts(t *testing.T) {
	for _, tc := range []struct {
		name    string
		desired []any
		actual  []any
		owned   map[string]any
		want    []any
	}{
		{
			name: "duplicate volume names use mountPath",
			desired: []any{
				map[string]any{"name": "data", "mountPath": "/a"},
				map[string]any{"name": "data", "mountPath": "/b"},
			},
			actual: []any{
				map[string]any{"name": "injected", "mountPath": "/injected"},
				map[string]any{"name": "data", "mountPath": "/a", "readOnly": false},
				map[string]any{"name": "data", "mountPath": "/b", "subPath": "removed"},
			},
			owned: map[string]any{`k:{"mountPath":"/b"}`: map[string]any{"f:subPath": map[string]any{}}},
			want: []any{
				map[string]any{"name": "data", "mountPath": "/a"},
				map[string]any{"name": "data", "mountPath": "/b", "subPath": "removed"},
			},
		},
		{
			name:    "containerPort with multi-field ownership keys",
			desired: []any{map[string]any{"containerPort": float64(80)}},
			actual: []any{
				map[string]any{"containerPort": float64(90)},
				map[string]any{"containerPort": float64(80), "protocol": "TCP", "hostPort": float64(8080)},
				map[string]any{"containerPort": float64(81), "protocol": "TCP"},
			},
			owned: map[string]any{
				".": map[string]any{},
				`k:{"containerPort":80,"protocol":"TCP"}`:   map[string]any{"f:hostPort": map[string]any{}},
				`k:{"containerPort":"81","protocol":"TCP"}`: map[string]any{},
			},
			want: []any{
				map[string]any{"containerPort": float64(80), "hostPort": float64(8080)},
				map[string]any{"containerPort": float64(81), "protocol": "TCP"},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := projectActualToDesired(projectionContext{}, tc.desired, tc.actual, tc.owned)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("expected %#v, got %#v", tc.want, got)
			}
		})
	}
}

func TestProjectListKeyCandidates(t *testing.T) {
	keys := []string{"name", "mountPath", "containerPort", "port", "devicePath"}
	for i, key := range keys {
		t.Run(key, func(t *testing.T) {
			desiredItem := map[string]any{key: "match"}
			actualItem := map[string]any{key: "match"}
			injectedItem := map[string]any{key: "injected"}
			// Later candidates would not match: the first qualifying key wins.
			for _, later := range keys[i+1:] {
				desiredItem[later] = "desired"
				actualItem[later] = "live"
				injectedItem[later] = "injected"
			}
			got := projectActualToDesired(projectionContext{}, []any{desiredItem}, []any{injectedItem, actualItem}, nil)
			if !reflect.DeepEqual(got, []any{actualItem}) {
				t.Fatalf("expected item matched by %s, got %#v", key, got)
			}
		})
	}
}

func TestProjectListKeyFieldsAreNotPendingRemovals(t *testing.T) {
	// SSA owns defaulted key fields (protocol) of a multi-field list key; the
	// manifest omits them, which must not render as a removal.
	desired := []any{map[string]any{"name": "http", "containerPort": float64(4000)}}
	actual := []any{map[string]any{"name": "http", "containerPort": float64(4000), "protocol": "TCP"}}
	owned := map[string]any{
		`k:{"containerPort":4000,"protocol":"TCP"}`: map[string]any{
			".": map[string]any{}, "f:containerPort": map[string]any{}, "f:name": map[string]any{}, "f:protocol": map[string]any{},
		},
	}
	got := projectActualToDesired(projectionContext{}, desired, actual, owned)
	if !reflect.DeepEqual(got, desired) {
		t.Fatalf("expected defaulted key field hidden, got %#v", got)
	}
}

func TestProjectListKeyPrefersOwnershipKey(t *testing.T) {
	// Every mount has a unique name, but FieldsV1 owns volumeMounts by
	// mountPath: pairing must use mountPath so the owned removal surfaces.
	desired := []any{map[string]any{"name": "data", "mountPath": "/data"}}
	actual := []any{
		map[string]any{"name": "data", "mountPath": "/data"},
		map[string]any{"name": "cache", "mountPath": "/cache"},
		map[string]any{"name": "injected", "mountPath": "/injected"},
	}
	owned := map[string]any{
		`k:{"mountPath":"/data"}`:  map[string]any{".": map[string]any{}},
		`k:{"mountPath":"/cache"}`: map[string]any{".": map[string]any{}},
	}
	got := projectActualToDesired(projectionContext{}, desired, actual, owned)
	want := []any{actual[0], actual[1]}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected owned /cache removal kept and /injected dropped, got %#v", got)
	}
	if keys := listKeyCandidates(owned); keys[0] != "mountPath" {
		t.Fatalf("expected mountPath first, got %v", keys)
	}
	if keys := listKeyCandidates(map[string]any{"k:{bad": map[string]any{}, "f:x": map[string]any{}}); !reflect.DeepEqual(keys, defaultListKeys) {
		t.Fatalf("expected default order, got %v", keys)
	}
}

func TestProjectListsWithoutQualifyingKeyStayPositional(t *testing.T) {
	for _, tc := range []struct {
		name    string
		desired []any
		actual  []any
		want    []any
	}{
		{
			name:    "duplicate desired keys",
			desired: []any{map[string]any{"name": "a"}, map[string]any{"name": "a"}},
			actual:  []any{map[string]any{"name": "b", "extra": "drop"}, map[string]any{"name": "a"}},
			want:    []any{map[string]any{"name": "b"}, map[string]any{"name": "a"}},
		},
		{
			name: "duplicate live keys", desired: []any{map[string]any{"name": "a"}},
			actual: []any{map[string]any{"name": "b", "extra": "drop"}, map[string]any{"name": "b"}},
			want:   []any{map[string]any{"name": "b"}},
		},
		{
			name: "non-map live item", desired: []any{map[string]any{"name": "a"}},
			actual: []any{"raw", map[string]any{"name": "a"}}, want: []any{"raw"},
		},
		{
			name: "non-map desired item", desired: []any{"desired", "missing"},
			actual: []any{"actual"}, want: []any{"actual", nil},
		},
		{
			name: "missing live key", desired: []any{map[string]any{"name": "a"}},
			actual: []any{map[string]any{"extra": "drop"}, map[string]any{"name": "a"}},
			want:   []any{map[string]any{}},
		},
		{
			name: "missing desired key", desired: []any{map[string]any{"value": "desired"}},
			actual: []any{map[string]any{"name": "a", "value": "actual"}},
			want:   []any{map[string]any{"value": "actual"}},
		},
		{
			name: "boolean key", desired: []any{map[string]any{"name": true}},
			actual: []any{map[string]any{"name": false}, map[string]any{"name": true}},
			want:   []any{map[string]any{"name": false}},
		},
		{
			name: "non-scalar key", desired: []any{map[string]any{"name": []any{"a"}}},
			actual: []any{map[string]any{"name": []any{"b"}}}, want: []any{map[string]any{"name": []any{"b"}}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Even an owned extra field stays projected away in positional fallback.
			owned := map[string]any{`k:{"name":"b"}`: map[string]any{"f:extra": map[string]any{}}}
			got := projectActualToDesired(projectionContext{}, tc.desired, tc.actual, owned)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("expected positional projection %#v, got %#v", tc.want, got)
			}
		})
	}
}

func TestProjectKeyedListEmptySidesAndOwnership(t *testing.T) {
	item := map[string]any{"name": "X", "value": "old"}
	owned := map[string]any{`k:{"name":"X"}`: map[string]any{}}
	for _, tc := range []struct {
		name    string
		desired []any
		actual  []any
		owned   any
		want    []any
	}{
		{name: "empty live", desired: []any{item}, actual: []any{}, want: []any{}},
		{name: "empty desired unowned", desired: []any{}, actual: []any{item}, want: []any{}},
		{name: "empty desired owned", desired: []any{}, actual: []any{item}, owned: owned, want: []any{item}},
		{name: "both empty", desired: []any{}, actual: []any{}, want: []any{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := projectActualToDesired(projectionContext{}, tc.desired, tc.actual, tc.owned)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("expected %#v, got %#v", tc.want, got)
			}
		})
	}
}

func TestListOwnershipByKey(t *testing.T) {
	owned := map[string]any{
		".":                        map[string]any{},
		"k:invalid":                map[string]any{},
		`k:["not an object"]`:      map[string]any{},
		`k:{"protocol":"TCP"}`:     map[string]any{},
		`k:{"containerPort":true}`: map[string]any{},
		`k:{"containerPort":90}`:   "invalid subtree",
		`k:{"containerPort":80,"protocol":"TCP"}`: map[string]any{"f:hostPort": map[string]any{}},
		`k:{"containerPort":80,"protocol":"UDP"}`: map[string]any{"f:name": map[string]any{}, "f:protocol": map[string]any{}},
	}
	// Subtrees merge per selected value; key fields (protocol) are dropped.
	want := map[string]any{"80": map[string]any{"f:hostPort": map[string]any{}, "f:name": map[string]any{}}}
	before := deepCopyMap(owned)
	if got := listOwnershipByKey(owned, "containerPort"); !reflect.DeepEqual(got, want) {
		t.Fatalf("expected merged ownership %#v, got %#v", want, got)
	}
	if !reflect.DeepEqual(owned, before) {
		t.Fatal("ownership indexing modified the original trie")
	}
}

func TestComputeWithPruneAndRisk(t *testing.T) {
	desired := []render.Resource{{APIVersion: "apps/v1", Kind: "Deployment", Namespace: "n", Name: "a", Body: map[string]any{"apiVersion": "apps/v1", "kind": "Deployment", "metadata": map[string]any{"namespace": "n", "name": "a"}, "spec": map[string]any{"replicas": 2}}}, {APIVersion: "v1", Kind: "ConfigMap", Namespace: "n", Name: "c", Body: map[string]any{"apiVersion": "v1", "kind": "ConfigMap", "metadata": map[string]any{"namespace": "n", "name": "c"}}}}
	actual := []render.Resource{{APIVersion: "apps/v1", Kind: "Deployment", Namespace: "n", Name: "a", Body: map[string]any{"apiVersion": "apps/v1", "kind": "Deployment", "metadata": map[string]any{"namespace": "n", "name": "a", "uid": "x"}, "spec": map[string]any{"replicas": 1}, "status": map[string]any{"x": "y"}}}, {APIVersion: "v1", Kind: "ConfigMap", Namespace: "n", Name: "b", Body: map[string]any{"apiVersion": "v1", "kind": "ConfigMap", "metadata": map[string]any{"namespace": "n", "name": "b"}}}}

	changes, summary := Compute(desired, actual, Options{PruneDeletes: true})
	if len(changes) != 3 || summary.Creates != 1 || summary.Patches != 1 || summary.Deletes != 1 {
		t.Fatalf("unexpected result: %+v %+v", changes, summary)
	}
	found := false
	for _, ch := range changes {
		if ch.Action == Patch {
			found = len(ch.Risks) > 0
			break
		}
	}
	if !found {
		t.Fatalf("expected risk flags on patch change: %+v", changes)
	}
}

func TestComputeWithoutPruneSkipsDelete(t *testing.T) {
	desired := []render.Resource{}
	actual := []render.Resource{{APIVersion: "v1", Kind: "ConfigMap", Namespace: "n", Name: "b", Body: map[string]any{"apiVersion": "v1", "kind": "ConfigMap", "metadata": map[string]any{"namespace": "n", "name": "b"}}}}
	changes, summary := Compute(desired, actual, Options{PruneDeletes: false})
	if len(changes) != 0 || summary.Deletes != 0 {
		t.Fatalf("expected no delete without prune: %+v %+v", changes, summary)
	}
}

func TestComputeIgnoreFields(t *testing.T) {
	desired := []render.Resource{{APIVersion: "v1", Kind: "ConfigMap", Namespace: "n", Name: "a", Body: map[string]any{"apiVersion": "v1", "kind": "ConfigMap", "metadata": map[string]any{"namespace": "n", "name": "a", "annotations": map[string]any{"x": "1"}}}}}
	actual := []render.Resource{{APIVersion: "v1", Kind: "ConfigMap", Namespace: "n", Name: "a", Body: map[string]any{"apiVersion": "v1", "kind": "ConfigMap", "metadata": map[string]any{"namespace": "n", "name": "a", "annotations": map[string]any{"x": "2"}}}}}
	changes, summary := Compute(desired, actual, Options{PruneDeletes: true, IgnoreFields: []string{"metadata.annotations"}})
	if len(changes) != 1 || changes[0].Action != NoOp || summary.NoOps != 1 {
		t.Fatalf("expected noop with ignore fields: %+v %+v", changes, summary)
	}
}

func TestComputeIncludesChangedPathsAndYAML(t *testing.T) {
	desired := []render.Resource{{APIVersion: "apps/v1", Kind: "Deployment", Namespace: "n", Name: "a", Body: map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata":   map[string]any{"namespace": "n", "name": "a"},
		"spec":       map[string]any{"replicas": 2, "template": map[string]any{"metadata": map[string]any{"labels": map[string]any{"app": "a"}}}},
	}}}
	actual := []render.Resource{{APIVersion: "apps/v1", Kind: "Deployment", Namespace: "n", Name: "a", Body: map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata":   map[string]any{"namespace": "n", "name": "a"},
		"spec":       map[string]any{"replicas": 1, "template": map[string]any{"metadata": map[string]any{"labels": map[string]any{"app": "a"}}}},
	}}}
	changes, _ := Compute(desired, actual, Options{PruneDeletes: true})
	if len(changes) != 1 || changes[0].Action != Patch {
		t.Fatalf("expected patch change: %+v", changes)
	}
	if len(changes[0].ChangedPaths) == 0 {
		t.Fatalf("expected changed paths: %+v", changes[0])
	}
	if changes[0].CurrentYAML == "" || changes[0].DesiredYAML == "" {
		t.Fatalf("expected current/desired yaml populated: %+v", changes[0])
	}
}

func TestComputeIgnoresServerManagedGenerationAndFluxLabels(t *testing.T) {
	desired := []render.Resource{{APIVersion: "v1", Kind: "ConfigMap", Namespace: "n", Name: "a", Body: map[string]any{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata": map[string]any{
			"name":      "a",
			"namespace": "n",
			"labels": map[string]any{
				"app": "demo",
			},
		},
	}}}
	actual := []render.Resource{{APIVersion: "v1", Kind: "ConfigMap", Namespace: "n", Name: "a", Body: map[string]any{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata": map[string]any{
			"name":       "a",
			"namespace":  "n",
			"generation": 7,
			"labels": map[string]any{
				"app":                                   "demo",
				"kustomize.toolkit.fluxcd.io/name":      "flux-system",
				"kustomize.toolkit.fluxcd.io/namespace": "flux-system",
			},
		},
	}}}
	changes, summary := Compute(desired, actual, Options{PruneDeletes: true})
	if len(changes) != 1 || changes[0].Action != NoOp || summary.NoOps != 1 {
		t.Fatalf("expected noop after normalization, got changes=%+v summary=%+v", changes, summary)
	}
}

func TestComputeIgnoresCRDDefaultConversionNone(t *testing.T) {
	desired := []render.Resource{{APIVersion: "apiextensions.k8s.io/v1", Kind: "CustomResourceDefinition", Name: "foos.example.com", Body: map[string]any{
		"apiVersion": "apiextensions.k8s.io/v1",
		"kind":       "CustomResourceDefinition",
		"metadata":   map[string]any{"name": "foos.example.com"},
		"spec":       map[string]any{"group": "example.com"},
	}}}
	actual := []render.Resource{{APIVersion: "apiextensions.k8s.io/v1", Kind: "CustomResourceDefinition", Name: "foos.example.com", Body: map[string]any{
		"apiVersion": "apiextensions.k8s.io/v1",
		"kind":       "CustomResourceDefinition",
		"metadata":   map[string]any{"name": "foos.example.com"},
		"spec": map[string]any{
			"group": "example.com",
			"conversion": map[string]any{
				"strategy": "None",
			},
		},
	}}}
	changes, summary := Compute(desired, actual, Options{PruneDeletes: true})
	if len(changes) != 1 || changes[0].Action != NoOp || summary.NoOps != 1 {
		t.Fatalf("expected noop for default CRD conversion, got changes=%+v summary=%+v", changes, summary)
	}
}

func TestComputeIgnoreActualExtraFields(t *testing.T) {
	desired := []render.Resource{{APIVersion: "v1", Kind: "Service", Namespace: "n", Name: "svc", Body: map[string]any{
		"apiVersion": "v1",
		"kind":       "Service",
		"metadata":   map[string]any{"name": "svc", "namespace": "n"},
		"spec": map[string]any{
			"ports": []any{
				map[string]any{"port": 80, "protocol": "TCP"},
			},
			"selector": map[string]any{"app": "demo"},
		},
	}}}
	actual := []render.Resource{{APIVersion: "v1", Kind: "Service", Namespace: "n", Name: "svc", Body: map[string]any{
		"apiVersion": "v1",
		"kind":       "Service",
		"metadata":   map[string]any{"name": "svc", "namespace": "n"},
		"spec": map[string]any{
			"clusterIP":  "10.0.0.1",
			"clusterIPs": []any{"10.0.0.1"},
			"ports": []any{
				map[string]any{"port": 80, "protocol": "TCP", "targetPort": 80},
			},
			"selector": map[string]any{"app": "demo"},
		},
	}}}

	changes, summary := Compute(desired, actual, Options{PruneDeletes: true, IgnoreActualExtraFields: true})
	if len(changes) != 1 || changes[0].Action != NoOp || summary.NoOps != 1 {
		t.Fatalf("expected noop when ignoring live-only computed fields, got changes=%+v summary=%+v", changes, summary)
	}
}

func TestComputeKeepsExtraFieldsWhenOptionDisabled(t *testing.T) {
	desired := []render.Resource{{APIVersion: "v1", Kind: "ConfigMap", Namespace: "n", Name: "cm", Body: map[string]any{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata":   map[string]any{"name": "cm", "namespace": "n"},
		"data":       map[string]any{"k": "v"},
	}}}
	actual := []render.Resource{{APIVersion: "v1", Kind: "ConfigMap", Namespace: "n", Name: "cm", Body: map[string]any{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata":   map[string]any{"name": "cm", "namespace": "n"},
		"data":       map[string]any{"k": "v"},
		"binaryData": map[string]any{"x": "eA=="},
	}}}
	changes, summary := Compute(desired, actual, Options{PruneDeletes: true, IgnoreActualExtraFields: false})
	if len(changes) != 1 || changes[0].Action != Patch || summary.Patches != 1 {
		t.Fatalf("expected patch when extra fields are not ignored, got changes=%+v summary=%+v", changes, summary)
	}
}

func TestComputeBuildsAttributeDiffLinesForPatch(t *testing.T) {
	desired := []render.Resource{{APIVersion: "v1", Kind: "ConfigMap", Namespace: "n", Name: "cm", Body: map[string]any{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata":   map[string]any{"name": "cm", "namespace": "n"},
		"data":       map[string]any{"key": "new"},
	}}}
	actual := []render.Resource{{APIVersion: "v1", Kind: "ConfigMap", Namespace: "n", Name: "cm", Body: map[string]any{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata":   map[string]any{"name": "cm", "namespace": "n"},
		"data":       map[string]any{"key": "old"},
	}}}

	changes, summary := Compute(desired, actual, Options{PruneDeletes: true, IgnoreActualExtraFields: true})
	if len(changes) != 1 || changes[0].Action != Patch || summary.Patches != 1 {
		t.Fatalf("expected one patch, got changes=%+v summary=%+v", changes, summary)
	}
	if len(changes[0].AttributeDiff) < 2 {
		t.Fatalf("expected +/- attribute diff lines, got %+v", changes[0].AttributeDiff)
	}
	got := strings.Join(changes[0].AttributeDiff, "\n")
	for _, want := range []string{
		"- data.key: \"old\"",
		"+ data.key: \"new\"",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected attribute diff %q in %q", want, got)
		}
	}
}

func TestHelpersAndRiskDetection(t *testing.T) {
	if got := displayPath(""); got != "<root>" {
		t.Fatalf("unexpected root path: %s", got)
	}
	if got := displayPath("spec.replicas"); got != "spec.replicas" {
		t.Fatalf("unexpected path passthrough: %s", got)
	}
	if got := formatValue(func() {}); !strings.Contains(got, "0x") {
		t.Fatalf("expected fmt fallback for unsupported json value, got %q", got)
	}
	if got := mustYAML(func() {}); !strings.Contains(got, "failed to marshal yaml") {
		t.Fatalf("expected marshal failure marker, got %q", got)
	}
	if got := mustYAML(nil); got != "" {
		t.Fatalf("expected empty yaml for nil object, got %q", got)
	}

	desired := render.Resource{Kind: "Deployment"}
	actual := render.Resource{Kind: "Deployment"}
	risks := detectRisks(desired, actual, []string{"spec", "metadata"})
	for _, want := range []string{"workload-spec-change", "metadata-change"} {
		if !contains(risks, want) {
			t.Fatalf("expected risk %q in %+v", want, risks)
		}
	}
	crdRisks := detectRisks(render.Resource{Kind: "CustomResourceDefinition"}, render.Resource{}, nil)
	if !contains(crdRisks, "crd-change") {
		t.Fatalf("expected crd risk, got %+v", crdRisks)
	}
}

func TestProjectActualToDesiredAndPruneNilValues(t *testing.T) {
	projected := projectActualToDesired(projectionContext{},
		map[string]any{
			"spec": map[string]any{
				"ports": []any{
					map[string]any{"port": 80},
				},
			},
		},
		map[string]any{
			"spec": map[string]any{
				"ports": []any{
					map[string]any{"port": 80, "targetPort": 8080},
				},
				"clusterIP": "10.0.0.1",
			},
		},
		nil,
	)
	gotMap, ok := projected.(map[string]any)
	if !ok {
		t.Fatalf("expected projected map, got %T", projected)
	}
	spec := gotMap["spec"].(map[string]any)
	if _, ok := spec["clusterIP"]; ok {
		t.Fatalf("expected live-only field to be removed, got %+v", spec)
	}

	// When desired/actual types differ, actual value is preserved.
	if got := projectActualToDesired(projectionContext{}, []any{1}, "raw-string", nil); got != "raw-string" {
		t.Fatalf("expected mismatched type passthrough, got %#v", got)
	}
	// Missing actual array entries produce nil placeholders.
	arr := projectActualToDesired(projectionContext{}, []any{"a", "b"}, []any{"a"}, nil).([]any)
	if len(arr) != 2 || arr[1] != nil {
		t.Fatalf("expected nil placeholder for missing entry, got %#v", arr)
	}

	pruned := pruneNilValues(map[string]any{
		"a": nil,
		"b": map[string]any{"c": nil, "d": "ok"},
		"e": []any{nil, "x"},
	}).(map[string]any)
	if _, ok := pruned["a"]; ok {
		t.Fatalf("expected nil map key removed, got %+v", pruned)
	}
	if _, ok := pruned["b"].(map[string]any)["c"]; ok {
		t.Fatalf("expected nested nil map key removed, got %+v", pruned["b"])
	}
	if len(pruned["e"].([]any)) != 2 || pruned["e"].([]any)[0] != nil {
		t.Fatalf("expected slice positions retained, got %+v", pruned["e"])
	}
}

// Regression: with IgnoreActualExtraFields enabled, a field REMOVED from the
// desired manifest but still present live used to project away entirely and
// render as a no-op. When the GitOps apply manager owns the field, the next
// apply removes it, so the plan must show a patch.
func TestComputeSurfacesOwnedFieldRemovalAsPatch(t *testing.T) {
	desired := []render.Resource{{APIVersion: "networking.k8s.io/v1", Kind: "Ingress", Namespace: "n", Name: "app", Body: map[string]any{
		"apiVersion": "networking.k8s.io/v1",
		"kind":       "Ingress",
		"metadata": map[string]any{
			"name": "app", "namespace": "n",
			"annotations": map[string]any{
				"nginx.ingress.kubernetes.io/auth-url": "http://sso.example/auth",
			},
		},
		"spec": map[string]any{"ingressClassName": "nginx"},
	}}}
	actual := []render.Resource{{APIVersion: "networking.k8s.io/v1", Kind: "Ingress", Namespace: "n", Name: "app", Body: map[string]any{
		"apiVersion": "networking.k8s.io/v1",
		"kind":       "Ingress",
		"metadata": map[string]any{
			"name": "app", "namespace": "n",
			"annotations": map[string]any{
				"nginx.ingress.kubernetes.io/auth-url":              "http://sso.example/auth",
				"nginx.ingress.kubernetes.io/auth-response-headers": "X-Auth-Request-User,X-Auth-Request-Email",
			},
			"managedFields": []any{
				map[string]any{
					"manager":    "kustomize-controller",
					"operation":  "Apply",
					"fieldsType": "FieldsV1",
					"fieldsV1": map[string]any{
						"f:metadata": map[string]any{
							"f:annotations": map[string]any{
								"f:nginx.ingress.kubernetes.io/auth-url":              map[string]any{},
								"f:nginx.ingress.kubernetes.io/auth-response-headers": map[string]any{},
							},
						},
						"f:spec": map[string]any{"f:ingressClassName": map[string]any{}},
					},
				},
			},
		},
		"spec": map[string]any{"ingressClassName": "nginx"},
	}}}

	changes, summary := Compute(desired, actual, Options{PruneDeletes: true, IgnoreActualExtraFields: true})
	if len(changes) != 1 || changes[0].Action != Patch || summary.Patches != 1 {
		t.Fatalf("expected patch for owned-field removal, got changes=%+v summary=%+v", changes, summary)
	}
	foundPath := false
	for _, path := range changes[0].ChangedPaths {
		if strings.Contains(path, "auth-response-headers") {
			foundPath = true
		}
	}
	if !foundPath {
		t.Fatalf("expected removed annotation in changed paths, got %+v", changes[0].ChangedPaths)
	}
}

// Live-only fields owned by OTHER managers (server defaults, controllers)
// stay hidden: they are not removed by the GitOps apply.
func TestComputeStillHidesUnownedExtraFields(t *testing.T) {
	desired := []render.Resource{{APIVersion: "apps/v1", Kind: "Deployment", Namespace: "n", Name: "app", Body: map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata": map[string]any{
			"name": "app", "namespace": "n",
			"annotations": map[string]any{"team": "x"},
		},
		"spec": map[string]any{"replicas": 1},
	}}}
	actual := []render.Resource{{APIVersion: "apps/v1", Kind: "Deployment", Namespace: "n", Name: "app", Body: map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata": map[string]any{
			"name": "app", "namespace": "n",
			"annotations": map[string]any{
				"team":                              "x",
				"deployment.kubernetes.io/revision": "42",
			},
			"managedFields": []any{
				map[string]any{
					"manager":    "kustomize-controller",
					"operation":  "Apply",
					"fieldsType": "FieldsV1",
					"fieldsV1": map[string]any{
						"f:metadata": map[string]any{
							"f:annotations": map[string]any{"f:team": map[string]any{}},
						},
						"f:spec": map[string]any{"f:replicas": map[string]any{}},
					},
				},
				map[string]any{
					"manager":    "kube-controller-manager",
					"operation":  "Update",
					"fieldsType": "FieldsV1",
					"fieldsV1": map[string]any{
						"f:metadata": map[string]any{
							"f:annotations": map[string]any{"f:deployment.kubernetes.io/revision": map[string]any{}},
						},
					},
				},
			},
		},
		"spec": map[string]any{"replicas": 1},
	}}}

	changes, summary := Compute(desired, actual, Options{PruneDeletes: true, IgnoreActualExtraFields: true})
	if len(changes) != 1 || changes[0].Action != NoOp || summary.NoOps != 1 {
		t.Fatalf("expected noop when extras are owned by other managers, got changes=%+v summary=%+v", changes, summary)
	}
}

// A ConfigMap key removal is a pending change the plan must show.
func TestComputeSurfacesOwnedDataKeyRemoval(t *testing.T) {
	desired := []render.Resource{{APIVersion: "v1", Kind: "ConfigMap", Namespace: "n", Name: "cm", Body: map[string]any{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata":   map[string]any{"name": "cm", "namespace": "n"},
		"data":       map[string]any{"KEEP": "1"},
	}}}
	actual := []render.Resource{{APIVersion: "v1", Kind: "ConfigMap", Namespace: "n", Name: "cm", Body: map[string]any{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata": map[string]any{
			"name": "cm", "namespace": "n",
			"managedFields": []any{
				map[string]any{
					"manager":    "kustomize-controller",
					"operation":  "Apply",
					"fieldsType": "FieldsV1",
					"fieldsV1": map[string]any{
						"f:data": map[string]any{
							"f:KEEP":   map[string]any{},
							"f:REMOVE": map[string]any{},
						},
					},
				},
			},
		},
		"data": map[string]any{"KEEP": "1", "REMOVE": "old"},
	}}}

	changes, summary := Compute(desired, actual, Options{PruneDeletes: true, IgnoreActualExtraFields: true})
	if len(changes) != 1 || changes[0].Action != Patch || summary.Patches != 1 {
		t.Fatalf("expected patch for removed data key, got changes=%+v summary=%+v", changes, summary)
	}
}

// Custom applyManagers replace the default list.
func TestComputeApplyManagersOption(t *testing.T) {
	body := func(withExtra bool) map[string]any {
		data := map[string]any{"KEEP": "1"}
		if withExtra {
			data["REMOVE"] = "old"
		}
		return map[string]any{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]any{
				"name": "cm", "namespace": "n",
				"managedFields": []any{
					map[string]any{
						"manager":    "helm-controller",
						"operation":  "Apply",
						"fieldsType": "FieldsV1",
						"fieldsV1": map[string]any{
							"f:data": map[string]any{
								"f:KEEP":   map[string]any{},
								"f:REMOVE": map[string]any{},
							},
						},
					},
				},
			},
			"data": data,
		}
	}
	desired := []render.Resource{{APIVersion: "v1", Kind: "ConfigMap", Namespace: "n", Name: "cm", Body: body(false)}}
	actual := []render.Resource{{APIVersion: "v1", Kind: "ConfigMap", Namespace: "n", Name: "cm", Body: body(true)}}

	// Default managers: helm-controller ownership is ignored -> no-op.
	changes, _ := Compute(desired, actual, Options{IgnoreActualExtraFields: true})
	if len(changes) != 1 || changes[0].Action != NoOp {
		t.Fatalf("expected noop under default managers, got %+v", changes)
	}
	// Explicit manager: removal surfaces.
	changes, _ = Compute(desired, actual, Options{IgnoreActualExtraFields: true, ApplyManagers: []string{"helm-controller"}})
	if len(changes) != 1 || changes[0].Action != Patch {
		t.Fatalf("expected patch with explicit manager, got %+v", changes)
	}
}
