package diff

import (
	"testing"

	"github.com/example/thule/internal/render"
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
