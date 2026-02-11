package diff

import (
	"encoding/json"
	"sort"

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
	ID     string
	Action Action
}

type Summary struct {
	Creates int
	Patches int
	Deletes int
	NoOps   int
}

func Compute(desired, actual []render.Resource) ([]Change, Summary) {
	dm := map[string]render.Resource{}
	am := map[string]render.Resource{}
	for _, r := range desired {
		dm[r.ID()] = normalize(r)
	}
	for _, r := range actual {
		am[r.ID()] = normalize(r)
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
			change.Action = Delete
			summary.Deletes++
		case equal(d.Body, a.Body):
			change.Action = NoOp
			summary.NoOps++
		default:
			change.Action = Patch
			summary.Patches++
		}
		changes = append(changes, change)
	}

	return changes, summary
}

func normalize(r render.Resource) render.Resource {
	cp := deepCopyMap(r.Body)
	delete(cp, "status")
	if m, ok := cp["metadata"].(map[string]any); ok {
		delete(m, "managedFields")
		delete(m, "resourceVersion")
		delete(m, "uid")
		delete(m, "creationTimestamp")
		cp["metadata"] = m
	}
	r.Body = cp
	return r
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
