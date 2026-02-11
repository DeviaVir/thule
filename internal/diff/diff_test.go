package diff

import (
	"testing"

	"github.com/example/thule/internal/render"
)

func TestCompute(t *testing.T) {
	desired := []render.Resource{{APIVersion: "v1", Kind: "ConfigMap", Namespace: "n", Name: "a", Body: map[string]any{"apiVersion": "v1", "kind": "ConfigMap", "metadata": map[string]any{"namespace": "n", "name": "a"}, "data": map[string]any{"k": "v2"}}}, {APIVersion: "v1", Kind: "ConfigMap", Namespace: "n", Name: "c", Body: map[string]any{"apiVersion": "v1", "kind": "ConfigMap", "metadata": map[string]any{"namespace": "n", "name": "c"}}}}
	actual := []render.Resource{{APIVersion: "v1", Kind: "ConfigMap", Namespace: "n", Name: "a", Body: map[string]any{"apiVersion": "v1", "kind": "ConfigMap", "metadata": map[string]any{"namespace": "n", "name": "a", "uid": "x"}, "data": map[string]any{"k": "v1"}, "status": map[string]any{"x": "y"}}}, {APIVersion: "v1", Kind: "ConfigMap", Namespace: "n", Name: "b", Body: map[string]any{"apiVersion": "v1", "kind": "ConfigMap", "metadata": map[string]any{"namespace": "n", "name": "b"}}}}

	changes, summary := Compute(desired, actual)
	if len(changes) != 3 || summary.Creates != 1 || summary.Patches != 1 || summary.Deletes != 1 {
		t.Fatalf("unexpected result: %+v %+v", changes, summary)
	}
}
