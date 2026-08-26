package diff

import (
	"strings"
	"testing"

	"github.com/DeviaVir/thule/internal/render"
)

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
	projected := projectActualToDesired(
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
	if got := projectActualToDesired([]any{1}, "raw-string", nil); got != "raw-string" {
		t.Fatalf("expected mismatched type passthrough, got %#v", got)
	}
	// Missing actual array entries produce nil placeholders.
	arr := projectActualToDesired([]any{"a", "b"}, []any{"a"}, nil).([]any)
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
