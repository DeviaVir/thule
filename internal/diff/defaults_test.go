package diff

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/DeviaVir/thule/internal/render"
)

func TestComputeEmptyOwnershipDefaults(t *testing.T) {
	const deploymentDefault = `{"type":"RollingUpdate","rollingUpdate":{"maxUnavailable":"25%","maxSurge":"25%"}}`
	const statefulSetDefault = `{"type":"RollingUpdate","rollingUpdate":{"partition":0}}`
	const daemonSetDefault = `{"type":"RollingUpdate","rollingUpdate":{"maxUnavailable":1,"maxSurge":0}}`
	for _, tc := range []struct {
		name, kind, version, path, live, owned, desired string
		patch, realChange, noProjection, mergedOwner    bool
	}{
		{name: "deployment default", live: deploymentDefault},
		{name: "deployment recreate", live: `{"type":"Recreate"}`, patch: true},
		{name: "deployment numeric surge", live: `{"type":"RollingUpdate","rollingUpdate":{"maxUnavailable":"25%","maxSurge":1}}`, patch: true},
		{name: "deployment extra field", live: `{"type":"RollingUpdate","rollingUpdate":{"maxUnavailable":"25%","maxSurge":"25%"},"extra":true}`, patch: true},
		{name: "deployment incomplete default", live: `{"type":"RollingUpdate"}`, patch: true},
		{name: "explicit child ownership", live: deploymentDefault, owned: `{"f:type":{}}`, patch: true},
		{name: "dot ownership is not empty", live: deploymentDefault, owned: `{".":{}}`, patch: true},
		{name: "merged child ownership", live: deploymentDefault, mergedOwner: true, patch: true},
		{name: "statefulset default", kind: "StatefulSet", path: "spec.updateStrategy", live: statefulSetDefault},
		{name: "statefulset default without rolling update", kind: "StatefulSet", path: "spec.updateStrategy", live: `{"type":"RollingUpdate"}`},
		{name: "statefulset partition", kind: "StatefulSet", path: "spec.updateStrategy", live: `{"type":"RollingUpdate","rollingUpdate":{"partition":1}}`, patch: true},
		{name: "statefulset partition two", kind: "StatefulSet", path: "spec.updateStrategy", live: `{"type":"RollingUpdate","rollingUpdate":{"partition":2}}`, patch: true},
		{name: "statefulset on delete", kind: "StatefulSet", path: "spec.updateStrategy", live: `{"type":"OnDelete"}`, patch: true},
		{name: "daemonset default", kind: "DaemonSet", path: "spec.updateStrategy", live: daemonSetDefault},
		{name: "daemonset surge", kind: "DaemonSet", path: "spec.updateStrategy", live: `{"type":"RollingUpdate","rollingUpdate":{"maxUnavailable":1,"maxSurge":1}}`, patch: true},
		{name: "daemonset numeric string", kind: "DaemonSet", path: "spec.updateStrategy", live: `{"type":"RollingUpdate","rollingUpdate":{"maxUnavailable":"1","maxSurge":0}}`, patch: true},
		{name: "atomic node affinity", path: "spec.template.spec.affinity.nodeAffinity.requiredDuringSchedulingIgnoredDuringExecution", live: `{"nodeSelectorTerms":[{"matchExpressions":[{"key":"disk","operator":"In","values":["ssd"]}]}]}`, patch: true},
		{name: "atomic secret ref", path: "spec.template.spec.containers[].env[].valueFrom.secretKeyRef", live: `{"name":"example","key":"value"}`, patch: true},
		{name: "empty container resources", path: "spec.template.spec.containers[].resources", live: `{}`},
		{name: "empty list", path: "spec.template.spec.containers[].args", live: `[]`},
		{name: "owned empty map with children", path: "spec.template.spec.containers[].resources", live: `{}`, owned: `{"f:limits":{}}`, patch: true},
		{name: "nonempty list", path: "spec.template.spec.containers[].args", live: `["serve"]`, patch: true},
		{name: "scalar", path: "spec.revisionHistoryLimit", live: `0`, patch: true},
		{name: "default and real change", live: deploymentDefault, realChange: true, patch: true},
		{name: "different kind", kind: "Example", version: "example.com/v1", live: deploymentDefault, patch: true},
		{name: "different kind same apiVersion", kind: "StatefulSet", live: deploymentDefault, patch: true},
		{name: "different apiVersion", version: "example.com/v1", live: deploymentDefault, patch: true},
		{name: "different path", path: "spec.template.spec.strategy", live: deploymentDefault, patch: true},
		{name: "desired key present", live: deploymentDefault, desired: `{"type":"Recreate"}`, patch: true},
		{name: "projection disabled", live: deploymentDefault, noProjection: true, patch: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.kind == "" {
				tc.kind = "Deployment"
			}
			if tc.version == "" {
				tc.version = "apps/v1"
			}
			if tc.path == "" {
				tc.path = "spec.strategy"
			}
			if tc.owned == "" {
				tc.owned = `{}`
			}
			decode := func(s string) any {
				t.Helper()
				var value any
				if err := json.Unmarshal([]byte(s), &value); err != nil {
					t.Fatal(err)
				}
				return value
			}
			desired := render.Resource{APIVersion: tc.version, Kind: tc.kind, Namespace: "synthetics", Name: "apigw-heartbeat-staging", Body: map[string]any{
				"apiVersion": tc.version, "kind": tc.kind,
				"metadata": map[string]any{"name": "apigw-heartbeat-staging", "namespace": "synthetics"},
			}}
			actual := desired
			actual.Body = deepCopyMap(desired.Body)
			fields := map[string]any{}
			d, a, o := desired.Body, actual.Body, fields
			parts := strings.Split(tc.path, ".")
			for _, part := range parts[:len(parts)-1] {
				key := strings.TrimSuffix(part, "[]")
				dChild, aChild, oChild := map[string]any{}, map[string]any{}, map[string]any{}
				if strings.HasSuffix(part, "[]") {
					dChild["name"], aChild["name"] = "app", "app"
					d[key], a[key] = []any{dChild}, []any{aChild}
					o["f:"+key] = map[string]any{`k:{"name":"app"}`: oChild}
				} else {
					d[key], a[key], o["f:"+key] = dChild, aChild, oChild
				}
				d, a, o = dChild, aChild, oChild
			}
			leaf := parts[len(parts)-1]
			a[leaf], o["f:"+leaf] = decode(tc.live), decode(tc.owned)
			if tc.desired != "" {
				d[leaf] = decode(tc.desired)
			}
			entries := []any{map[string]any{"manager": "kustomize-controller", "operation": "Apply", "fieldsType": "FieldsV1", "fieldsV1": fields}}
			if tc.mergedOwner {
				entries = append(entries, map[string]any{"manager": "second-apply-manager", "operation": "Apply", "fieldsType": "FieldsV1", "fieldsV1": decode(`{"f:spec":{"f:strategy":{"f:type":{}}}}`)})
			}
			actual.Body["metadata"].(map[string]any)["managedFields"] = entries
			if tc.realChange {
				desired.Body["spec"].(map[string]any)["replicas"] = 2
				actual.Body["spec"].(map[string]any)["replicas"] = 1
			}
			beforeDesired, beforeActual := mustYAML(desired.Body), mustYAML(actual.Body)
			changes, summary := Compute([]render.Resource{desired}, []render.Resource{actual}, Options{
				IgnoreActualExtraFields: !tc.noProjection,
				ApplyManagers:           []string{"kustomize-controller", "second-apply-manager"},
			})
			if !tc.patch {
				assertQuantityNoOp(t, changes, summary)
			} else {
				if summary != (Summary{Patches: 1}) || len(changes) != 1 || changes[0].Action != Patch {
					t.Fatalf("expected PATCH, got changes=%+v summary=%+v", changes, summary)
				}
				path := strings.Split(tc.path, "[]")[0]
				value, _ := json.Marshal(a[leaf])
				lines := []string{"- " + strings.ReplaceAll(tc.path, "[]", "[0]") + ": " + string(value)}
				if tc.realChange {
					path = "spec.replicas"
					lines = []string{"- spec.replicas: 1", "+ spec.replicas: 2"}
				} else if tc.desired != "" {
					// Existing keys are compared recursively; the default must not
					// be omitted when desired explicitly changes it.
					path = "spec.strategy.type"
					lines = []string{`- spec.strategy.type: "RollingUpdate"`, `+ spec.strategy.type: "Recreate"`}
				}
				ch := changes[0]
				if !reflect.DeepEqual(ch.ChangedKeys, []string{"spec"}) || !reflect.DeepEqual(ch.ChangedPaths, []string{path}) || !reflect.DeepEqual(ch.AttributeDiff, lines) {
					t.Fatalf("unexpected change details: %+v; want path=%s lines=%v", ch, path, lines)
				}
			}
			if mustYAML(desired.Body) != beforeDesired || mustYAML(actual.Body) != beforeActual {
				t.Fatal("Compute mutated input resources")
			}
		})
	}
}
