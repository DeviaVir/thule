package diff

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/example/thule/internal/render"
)

type Action string

const (
	Create Action = "CREATE"
	Patch  Action = "PATCH"
	Delete Action = "DELETE"
	NoOp   Action = "NO-OP"
)

type Change struct {
	ID          string
	Action      Action
	ChangedKeys []string
	Risks       []string
}

type Summary struct {
	Creates int
	Patches int
	Deletes int
	NoOps   int
}

type Options struct {
	PruneDeletes bool
	IgnoreFields []string
}

func Compute(desired, actual []render.Resource, opts Options) ([]Change, Summary) {
	dm := map[string]render.Resource{}
	am := map[string]render.Resource{}
	for _, r := range desired {
		dm[r.ID()] = normalize(r, opts.IgnoreFields)
	}
	for _, r := range actual {
		am[r.ID()] = normalize(r, opts.IgnoreFields)
	}

	keys := map[string]struct{}{}
	for k := range dm {
		keys[k] = struct{}{}
	}
	for k := range am {
		keys[k] = struct{}{}
	}
	sorted := make([]string, 0, len(keys))
	for k := range keys {
		sorted = append(sorted, k)
	}
	sort.Strings(sorted)

	changes := make([]Change, 0, len(sorted))
	summary := Summary{}
	for _, k := range sorted {
		d, dok := dm[k]
		a, aok := am[k]
		change := Change{ID: k}
		switch {
		case dok && !aok:
			change.Action = Create
			summary.Creates++
		case !dok && aok:
			if opts.PruneDeletes {
				change.Action = Delete
				summary.Deletes++
			} else {
				continue
			}
		case equal(d.Body, a.Body):
			change.Action = NoOp
			summary.NoOps++
		default:
			change.Action = Patch
			change.ChangedKeys = changedTopLevelKeys(d.Body, a.Body)
			change.Risks = detectRisks(d, a, change.ChangedKeys)
			summary.Patches++
		}
		changes = append(changes, change)
	}

	return changes, summary
}

func normalize(r render.Resource, ignore []string) render.Resource {
	cp := deepCopyMap(r.Body)
	delete(cp, "status")
	if m, ok := cp["metadata"].(map[string]any); ok {
		delete(m, "managedFields")
		delete(m, "resourceVersion")
		delete(m, "uid")
		delete(m, "creationTimestamp")
		cp["metadata"] = m
	}
	for _, p := range ignore {
		deletePath(cp, p)
	}
	r.Body = cp
	return r
}

func deletePath(obj map[string]any, path string) {
	parts := strings.Split(path, ".")
	if len(parts) == 0 {
		return
	}
	cur := obj
	for i := 0; i < len(parts)-1; i++ {
		next, ok := cur[parts[i]].(map[string]any)
		if !ok {
			return
		}
		cur = next
	}
	delete(cur, parts[len(parts)-1])
}

func changedTopLevelKeys(a, b map[string]any) []string {
	keys := map[string]struct{}{}
	for k := range a {
		keys[k] = struct{}{}
	}
	for k := range b {
		keys[k] = struct{}{}
	}
	out := []string{}
	for k := range keys {
		if !equalAny(a[k], b[k]) {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

func detectRisks(desired, actual render.Resource, changed []string) []string {
	r := []string{}
	if contains(changed, "spec") && (desired.Kind == "Deployment" || desired.Kind == "StatefulSet" || desired.Kind == "DaemonSet") {
		r = append(r, "workload-spec-change")
	}
	if contains(changed, "metadata") {
		r = append(r, "metadata-change")
	}
	if desired.Kind == "CustomResourceDefinition" {
		r = append(r, "crd-change")
	}
	_ = actual
	return r
}

func contains(items []string, needle string) bool {
	for _, i := range items {
		if i == needle {
			return true
		}
	}
	return false
}

func equalAny(a, b any) bool {
	ab, _ := json.Marshal(a)
	bb, _ := json.Marshal(b)
	return string(ab) == string(bb)
}

func equal(a, b map[string]any) bool {
	ab, _ := json.Marshal(a)
	bb, _ := json.Marshal(b)
	return string(ab) == string(bb)
}

func deepCopyMap(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	b, _ := json.Marshal(in)
	out := map[string]any{}
	_ = json.Unmarshal(b, &out)
	return out
}
