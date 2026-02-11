package policy

import (
	"testing"

	"github.com/example/thule/internal/render"
)

func TestBuiltinEvaluator(t *testing.T) {
	e := NewBuiltinEvaluator()
	resources := []render.Resource{
		{APIVersion: "v1", Kind: "Secret", Namespace: "n", Name: "s", Body: map[string]any{}},
		{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "ClusterRoleBinding", Name: "crb", Body: map[string]any{}},
	}
	findings := e.Evaluate(resources, "strict")
	if len(findings) != 2 {
		t.Fatalf("expected two findings, got %+v", findings)
	}
}
