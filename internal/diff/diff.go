package diff

import (
	"encoding/json"
	"fmt"
	"math/big"
	"sort"
	"strconv"
	"strings"

	"github.com/DeviaVir/thule/internal/render"
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
	// ApplyManagers lists field managers (metadata.managedFields) whose
	// live-only fields are kept in the comparison even when
	// IgnoreActualExtraFields is set: a field owned by the GitOps apply
	// manager that is absent from the desired manifest is a pending REMOVAL
	// the next apply performs, not server-side noise. Without this, deleting
	// e.g. an annotation from a manifest renders as a no-op.
	// Empty means the default ["kustomize-controller"].
	ApplyManagers []string
}

var defaultApplyManagers = []string{"kustomize-controller"}

func Compute(desired, actual []render.Resource, opts Options) ([]Change, Summary) {
	applyManagers := opts.ApplyManagers
	if len(applyManagers) == 0 {
		applyManagers = defaultApplyManagers
	}
	dm := map[string]render.Resource{}
	am := map[string]render.Resource{}
	ownedByID := map[string]map[string]any{}
	for _, r := range desired {
		dm[r.ID()] = normalize(r, opts.IgnoreFields)
	}
	for _, r := range actual {
		// Extract apply-manager field ownership before normalize strips
		// managedFields from the compared body.
		ownedByID[r.ID()] = applyManagerFields(r.Body, applyManagers)
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
				if projected, ok := projectActualToDesired(desiredBody, actualBody, ownedByID[k]).(map[string]any); ok {
					actualBody = projected
				}
			}
			normalizeQuantityPairs(desiredBody, actualBody, "")
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

// normalizeQuantityPairs reuses the desired representation for equal quantities
// in the copied live body, so every downstream comparison sees the same values.
// Run after projection to avoid pairing desired items with injected live items.
func normalizeQuantityPairs(desired, actual any, parent string) {
	switch d := desired.(type) {
	case map[string]any:
		a, ok := actual.(map[string]any)
		if !ok {
			return
		}
		for key, dv := range d {
			av, exists := a[key]
			if !exists {
				continue
			}
			if isQuantityField(parent, key) {
				dq, dok := parseQuantity(fmt.Sprint(dv))
				aq, aok := parseQuantity(fmt.Sprint(av))
				if dok && aok && dq.Cmp(aq) == 0 {
					a[key] = dv
					continue
				}
			}
			normalizeQuantityPairs(dv, av, key)
		}
	case []any:
		a, ok := actual.([]any)
		if !ok {
			return
		}
		for i := 0; i < len(d) && i < len(a); i++ {
			normalizeQuantityPairs(d[i], a[i], "")
		}
	}
}

// Match quantity map leaves at any nesting depth, including pod templates and
// LimitRange entries. Ordinary strings (e.g. env[].value and resource claims)
// retain their existing comparison semantics.
func isQuantityField(parent, key string) bool {
	if key == "sizeLimit" {
		return true
	}
	switch parent {
	case "requests", "limits", "hard", "overhead", "max", "min", "default", "defaultRequest", "maxLimitRequestRatio":
		return true
	default:
		return false
	}
}

// quantityScales maps Kubernetes quantity suffixes to their multipliers.
var quantityScales = map[string]*big.Rat{
	"n": big.NewRat(1, 1_000_000_000), "u": big.NewRat(1, 1_000_000), "m": big.NewRat(1, 1000),
	"": big.NewRat(1, 1), "k": big.NewRat(1_000, 1), "M": big.NewRat(1_000_000, 1),
	"G": big.NewRat(1_000_000_000, 1), "T": big.NewRat(1_000_000_000_000, 1),
	"P": big.NewRat(1_000_000_000_000_000, 1), "E": big.NewRat(1_000_000_000_000_000_000, 1),
	"Ki": big.NewRat(1<<10, 1), "Mi": big.NewRat(1<<20, 1), "Gi": big.NewRat(1<<30, 1),
	"Ti": big.NewRat(1<<40, 1), "Pi": big.NewRat(1<<50, 1), "Ei": big.NewRat(1<<60, 1),
}

// parseQuantity returns the exact value of a Kubernetes quantity
// (<signedNumber><suffix>, where suffix is binary SI, decimal SI, or e/E<int>).
// It does not import k8s.io/apimachinery/pkg/api/resource: that import makes the
// go command rewrite go.mod to `go 1.25.0`, which lsif-go cannot parse.
func parseQuantity(s string) (*big.Rat, bool) {
	i := 0
	if i < len(s) && (s[i] == '+' || s[i] == '-') {
		i++
	}
	digits, dots := 0, 0
	for ; i < len(s) && (s[i] == '.' || (s[i] >= '0' && s[i] <= '9')); i++ {
		if s[i] == '.' {
			dots++
		} else {
			digits++
		}
	}
	if digits == 0 || dots > 1 {
		return nil, false
	}
	number, suffix := strings.TrimSuffix(s[:i], "."), s[i:]
	value, ok := new(big.Rat).SetString(number)
	if !ok {
		return nil, false
	}
	if scale, known := quantityScales[suffix]; known {
		return value.Mul(value, scale), true
	}
	if len(suffix) < 2 || (suffix[0] != 'e' && suffix[0] != 'E') {
		return nil, false
	}
	exp, err := strconv.Atoi(suffix[1:])
	if err != nil || exp < -1000 || exp > 1000 {
		return nil, false
	}
	scale := new(big.Rat).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(abs(exp))), nil))
	if exp < 0 {
		scale.Inv(scale)
	}
	return value.Mul(value, scale), true
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
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

// projectActualToDesired drops live-only fields from the comparison, EXCEPT
// fields the apply manager owns: those are pending removals the next apply
// performs, so hiding them would hide a real change. `owned` is the merged
// FieldsV1 trie of the apply managers at the current path (nil disables the
// ownership exception and restores pure projection).
func projectActualToDesired(desired, actual, owned any) any {
	switch dv := desired.(type) {
	case map[string]any:
		av, ok := actual.(map[string]any)
		if !ok {
			return actual
		}
		om, _ := owned.(map[string]any)
		out := make(map[string]any, len(dv))
		for k, avv := range av {
			var childOwned any
			if om != nil {
				childOwned = om["f:"+k]
			}
			dvv, exists := dv[k]
			if !exists {
				if childOwned != nil {
					// Owned by the apply manager but gone from the desired
					// manifest: surface the pending removal.
					out[k] = avv
				}
				continue
			}
			out[k] = projectActualToDesired(dvv, avv, childOwned)
		}
		return out
	case []any:
		av, ok := actual.([]any)
		if !ok {
			return actual
		}
		for _, key := range listKeyCandidates(owned) {
			desiredByKey, dok := indexListItems(dv, key)
			_, aok := indexListItems(av, key)
			if !dok || !aok {
				continue
			}
			ownedByKey := listOwnershipByKey(owned, key)
			out := make([]any, 0, len(av))
			for _, item := range av {
				value, _ := listKeyValue(item.(map[string]any)[key])
				childOwned := ownedByKey[value]
				if desiredItem, exists := desiredByKey[value]; exists {
					out = append(out, projectActualToDesired(desiredItem, item, childOwned))
				} else if childOwned != nil {
					// Keep owned live-only items verbatim to surface removals.
					out = append(out, item)
				}
			}
			return out
		}
		out := make([]any, 0, len(dv))
		for i := range dv {
			if i >= len(av) {
				out = append(out, nil)
				continue
			}
			// Without a unique associative key, keep positional projection;
			// FieldsV1 item ownership cannot reliably match these positions.
			out = append(out, projectActualToDesired(dv[i], av[i], nil))
		}
		return out
	default:
		return actual
	}
}

var defaultListKeys = []string{"name", "mountPath", "containerPort", "port", "devicePath"}

// listKeyCandidates orders the associative keys to try, preferring fields the
// apply manager's FieldsV1 k: entries use (e.g. volumeMounts are owned by
// mountPath even when every mount also has a unique name), so ownership
// lookups match the key the items are paired by.
func listKeyCandidates(owned any) []string {
	om, _ := owned.(map[string]any)
	used := map[string]bool{}
	for field := range om {
		if !strings.HasPrefix(field, "k:") {
			continue
		}
		var fields map[string]any
		if err := json.Unmarshal([]byte(strings.TrimPrefix(field, "k:")), &fields); err != nil {
			continue
		}
		for k := range fields {
			used[k] = true
		}
	}
	out := make([]string, 0, len(defaultListKeys))
	for _, k := range defaultListKeys {
		if used[k] {
			out = append(out, k)
		}
	}
	for _, k := range defaultListKeys {
		if !used[k] {
			out = append(out, k)
		}
	}
	return out
}

// indexListItems accepts a key only when every item has a unique scalar value.
func indexListItems(items []any, key string) (map[string]any, bool) {
	indexed := make(map[string]any, len(items))
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, false
		}
		value, ok := listKeyValue(m[key])
		if !ok {
			return nil, false
		}
		if _, exists := indexed[value]; exists {
			return nil, false
		}
		indexed[value] = item
	}
	return indexed, true
}

// listKeyValue normalizes string and numeric keys for bodies and FieldsV1 JSON.
func listKeyValue(value any) (string, bool) {
	switch value.(type) {
	case string, float64, float32, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, json.Number:
		return fmt.Sprint(value), true
	default:
		return "", false
	}
}

// listOwnershipByKey indexes k: subtrees by the selected field, including
// multi-field keys. Merge subtrees that identify the same selected value.
func listOwnershipByKey(owned any, key string) map[string]any {
	om, _ := owned.(map[string]any)
	indexed := map[string]any{}
	for field, subtree := range om {
		if !strings.HasPrefix(field, "k:") {
			continue
		}
		var fields map[string]any
		if err := json.Unmarshal([]byte(strings.TrimPrefix(field, "k:")), &fields); err != nil {
			continue
		}
		value, ok := listKeyValue(fields[key])
		if !ok {
			continue
		}
		trie, ok := subtree.(map[string]any)
		if !ok {
			continue
		}
		cp, ok := indexed[value].(map[string]any)
		if !ok {
			cp = map[string]any{}
			indexed[value] = cp
		}
		mergeFieldsTries(cp, trie)
		// Key fields identify the item; SSA records them as owned even when
		// the API server defaulted them (ports[].protocol), so they are not
		// pending removals.
		for f := range fields {
			delete(cp, "f:"+f)
		}
	}
	return indexed
}

// applyManagerFields merges the FieldsV1 tries of every managedFields entry
// whose manager is in `managers`. Returns nil when nothing matches (e.g. the
// object was never server-side applied by the GitOps manager).
func applyManagerFields(body map[string]any, managers []string) map[string]any {
	meta, _ := body["metadata"].(map[string]any)
	if meta == nil {
		return nil
	}
	entries, _ := meta["managedFields"].([]any)
	var merged map[string]any
	for _, e := range entries {
		em, _ := e.(map[string]any)
		if em == nil {
			continue
		}
		manager, _ := em["manager"].(string)
		if !stringInSlice(manager, managers) {
			continue
		}
		if ft, _ := em["fieldsType"].(string); ft != "" && ft != "FieldsV1" {
			continue
		}
		fields, _ := em["fieldsV1"].(map[string]any)
		if fields == nil {
			continue
		}
		if merged == nil {
			merged = map[string]any{}
		}
		mergeFieldsTries(merged, fields)
	}
	return merged
}

func mergeFieldsTries(dst, src map[string]any) {
	for k, v := range src {
		if sv, ok := v.(map[string]any); ok {
			if dv, ok := dst[k].(map[string]any); ok {
				mergeFieldsTries(dv, sv)
				continue
			}
			cp := map[string]any{}
			mergeFieldsTries(cp, sv)
			dst[k] = cp
			continue
		}
		if _, exists := dst[k]; !exists {
			dst[k] = v
		}
	}
}

func stringInSlice(needle string, items []string) bool {
	for _, i := range items {
		if i == needle {
			return true
		}
	}
	return false
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
