package policy

import (
	"fmt"

	"github.com/example/thule/internal/render"
)

type Severity string

const (
	SeverityWarn  Severity = "WARN"
	SeverityError Severity = "ERROR"
)

type Finding struct {
	ResourceID string
	RuleID     string
	Severity   Severity
	Message    string
}

type Evaluator interface {
	Evaluate(resources []render.Resource, profile string) []Finding
}

type BuiltinEvaluator struct{}

func NewBuiltinEvaluator() *BuiltinEvaluator {
	return &BuiltinEvaluator{}
}

func (e *BuiltinEvaluator) Evaluate(resources []render.Resource, profile string) []Finding {
	if profile == "" {
		profile = "baseline"
	}
	findings := []Finding{}
	for _, r := range resources {
		if r.Kind == "Secret" {
			findings = append(findings, Finding{ResourceID: r.ID(), RuleID: "review-secret-change", Severity: SeverityWarn, Message: "Secret change detected; validate secret rotation and source of truth"})
		}
		if profile == "strict" && r.Kind == "ClusterRoleBinding" {
			findings = append(findings, Finding{ResourceID: r.ID(), RuleID: "restrict-cluster-admin-bindings", Severity: SeverityError, Message: fmt.Sprintf("cluster-wide RBAC binding change requires security review: %s", r.ID())})
		}
	}
	return findings
}
