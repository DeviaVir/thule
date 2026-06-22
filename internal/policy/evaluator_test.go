package policy

import (
	"testing"

	"github.com/DeviaVir/thule/internal/render"
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

func hasRule(findings []Finding, ruleID string) bool {
	for _, f := range findings {
		if f.RuleID == ruleID {
			return true
		}
	}
	return false
}

func TestSecretWithInlineDataIsFlagged(t *testing.T) {
	e := NewBuiltinEvaluator()
	resources := []render.Resource{
		{APIVersion: "v1", Kind: "Secret", Namespace: "n", Name: "inline", Body: map[string]any{
			"data": map[string]any{"token": "aGVsbG8="},
		}},
	}
	findings := e.Evaluate(resources, "baseline")
	if !hasRule(findings, "secret-committed-to-gitops") {
		t.Fatalf("expected secret-committed-to-gitops finding, got %+v", findings)
	}
}

func TestSecretWithoutInlineDataIsNotFlagged(t *testing.T) {
	e := NewBuiltinEvaluator()
	resources := []render.Resource{
		{APIVersion: "v1", Kind: "Secret", Namespace: "n", Name: "shell", Body: map[string]any{}},
	}
	findings := e.Evaluate(resources, "baseline")
	if hasRule(findings, "secret-committed-to-gitops") {
		t.Fatalf("did not expect secret-committed-to-gitops finding, got %+v", findings)
	}
}
