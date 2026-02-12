package diff

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/example/thule/internal/render"
	"gopkg.in/yaml.v3"
)

type Action string

const (
	Create Action = "CREATE"
	Patch  Action = "PATCH"
	Delete Action = "DELETE"
	NoOp   Action = "NO-OP"
)

type Change struct {
	ID            string
	Action        Action
	ChangedKeys   []string
	ChangedPaths  []string
	AttributeDiff []string
	Risks         []string
	CurrentYAML   string
	DesiredYAML   string
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
	// IgnoreActualExtraFields drops fields that only exist in live resources
	// (e.g. API-server defaulted/computed attributes) before comparison.
	IgnoreActualExtraFields bool
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
			change.DesiredYAML = mustYAML(d.Body)
			summary.Creates++
		case !dok && aok:
			if opts.PruneDeletes {
				change.Action = Delete
				change.CurrentYAML = mustYAML(a.Body)
				summary.Deletes++
			} else {
				continue
			}
		case dok && aok:
			desiredBody := d.Body
			actualBody := a.Body
			if opts.IgnoreActualExtraFields {
				if projected, ok := projectActualToDesired(desiredBody, actualBody).(map[string]any); ok {
					actualBody = projected
				}
			}
			if equal(desiredBody, actualBody) {
				change.Action = NoOp
				summary.NoOps++
				changes = append(changes, change)
				continue
			}
			change.Action = Patch
			change.ChangedKeys = changedTopLevelKeys(desiredBody, actualBody)
			change.ChangedPaths = changedFieldPaths(desiredBody, actualBody)
			change.AttributeDiff = buildAttributeDiffLines(desiredBody, actualBody)
			change.Risks = detectRisks(d, a, change.ChangedKeys)
			change.CurrentYAML = mustYAML(actualBody)
			change.DesiredYAML = mustYAML(desiredBody)
			summary.Patches++
		}
		changes = append(changes, change)
	}

	return changes, summary
}

func normalize(r render.Resource, ignore []string) render.Resource {
	cp := deepCopyMap(r.Body)
	if cleaned, ok := pruneNilValues(cp).(map[string]any); ok {
		cp = cleaned
	}
	delete(cp, "status")
	if m, ok := cp["metadata"].(map[string]any); ok {
		delete(m, "managedFields")
		delete(m, "resourceVersion")
		delete(m, "uid")
		delete(m, "creationTimestamp")
		delete(m, "generation")
		if anns, ok := m["annotations"].(map[string]any); ok {
			delete(anns, "kubectl.kubernetes.io/last-applied-configuration")
			if len(anns) == 0 {
				delete(m, "annotations")
			} else {
				m["annotations"] = anns
			}
		}
		if labels, ok := m["labels"].(map[string]any); ok {
			delete(labels, "kustomize.toolkit.fluxcd.io/name")
			delete(labels, "kustomize.toolkit.fluxcd.io/namespace")
			m["labels"] = labels
		}
		cp["metadata"] = m
	}
	// API servers often default CRD conversion strategy to None; ignore this noise.
	if r.Kind == "CustomResourceDefinition" {
		if spec, ok := cp["spec"].(map[string]any); ok {
			if conv, ok := spec["conversion"].(map[string]any); ok {
				if strings.EqualFold(fmt.Sprint(conv["strategy"]), "none") {
					delete(spec, "conversion")
				}
			}
			cp["spec"] = spec
		}
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

func changedFieldPaths(desired, actual any) []string {
	seen := map[string]struct{}{}
	var walk func(path string, d, a any)
	walk = func(path string, d, a any) {
		if equalAny(d, a) {
			return
		}
		dm, dok := d.(map[string]any)
		am, aok := a.(map[string]any)
		if dok && aok {
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
			for _, k := range sorted {
				next := k
				if path != "" {
					next = path + "." + k
				}
				walk(next, dm[k], am[k])
			}
			return
		}
		if path == "" {
			path = "<root>"
		}
		seen[path] = struct{}{}
	}
	walk("", desired, actual)
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func mustYAML(obj any) (out string) {
	if obj == nil {
		return ""
	}
	defer func() {
		if rec := recover(); rec != nil {
			out = fmt.Sprintf("# failed to marshal yaml: %v\n", rec)
		}
	}()
	b, err := yaml.Marshal(obj)
	if err != nil {
		return fmt.Sprintf("# failed to marshal yaml: %v\n", err)
	}
	return string(b)
}

func projectActualToDesired(desired, actual any) any {
	switch dv := desired.(type) {
	case map[string]any:
		av, ok := actual.(map[string]any)
		if !ok {
			return actual
		}
		out := make(map[string]any, len(dv))
		for k, dvv := range dv {
			avv, exists := av[k]
			if !exists {
				continue
			}
			out[k] = projectActualToDesired(dvv, avv)
		}
		return out
	case []any:
		av, ok := actual.([]any)
		if !ok {
			return actual
		}
		out := make([]any, 0, len(dv))
		for i := range dv {
			if i >= len(av) {
				out = append(out, nil)
				continue
			}
			out = append(out, projectActualToDesired(dv[i], av[i]))
		}
		return out
	default:
		return actual
	}
}

func pruneNilValues(in any) any {
	switch v := in.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, vv := range v {
			clean := pruneNilValues(vv)
			if clean == nil {
				continue
			}
			out[k] = clean
		}
		return out
	case []any:
		out := make([]any, 0, len(v))
		for _, vv := range v {
			out = append(out, pruneNilValues(vv))
		}
		return out
	default:
		return in
	}
}

func buildAttributeDiffLines(desired, actual any) []string {
	lines := []string{}
	var walk func(path string, d any, dOK bool, a any, aOK bool)
	walk = func(path string, d any, dOK bool, a any, aOK bool) {
		switch {
		case dOK && aOK:
			dm, dok := d.(map[string]any)
			am, aok := a.(map[string]any)
			if dok && aok {
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
				for _, k := range sorted {
					child := k
					if path != "" {
						child = path + "." + k
					}
					dv, dok := dm[k]
					av, aok := am[k]
					walk(child, dv, dok, av, aok)
				}
				return
			}

			ds, dok := d.([]any)
			as, aok := a.([]any)
			if dok && aok {
				if len(ds) != len(as) {
					lines = append(lines, fmt.Sprintf("- %s: %s", displayPath(path), formatValue(a)))
					lines = append(lines, fmt.Sprintf("+ %s: %s", displayPath(path), formatValue(d)))
					return
				}
				for i := range ds {
					child := fmt.Sprintf("%s[%d]", displayPath(path), i)
					walk(child, ds[i], true, as[i], true)
				}
				return
			}

			if equalAny(d, a) {
				return
			}
			lines = append(lines, fmt.Sprintf("- %s: %s", displayPath(path), formatValue(a)))
			lines = append(lines, fmt.Sprintf("+ %s: %s", displayPath(path), formatValue(d)))
		case dOK && !aOK:
			lines = append(lines, fmt.Sprintf("+ %s: %s", displayPath(path), formatValue(d)))
		case !dOK && aOK:
			lines = append(lines, fmt.Sprintf("- %s: %s", displayPath(path), formatValue(a)))
		}
	}
	walk("", desired, true, actual, true)
	return lines
}

func displayPath(path string) string {
	if path == "" {
		return "<root>"
	}
	return path
}

func formatValue(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(b)
}
