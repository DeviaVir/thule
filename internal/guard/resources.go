package guard

import (
	"encoding/json"
	"fmt"
	"math/big"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/DeviaVir/thule/internal/render"
)

func validateListItemRequires(g Spec) error {
	if strings.TrimSpace(g.Kind) == "" {
		return fmt.Errorf("kind is required")
	}
	if !validFieldPath(g.List, true) {
		return fmt.Errorf("list is required and must be a dotted path with optional [] on intermediate segments")
	}
	for _, predicate := range []struct {
		name   string
		fields map[string]any
	}{{"when", g.When}, {"require", g.Require}} {
		if len(predicate.fields) == 0 {
			return fmt.Errorf("%s is required and must be non-empty", predicate.name)
		}
		for key, value := range predicate.fields {
			if !validFieldPath(key, false) {
				return fmt.Errorf("%s: invalid dotted field %q", predicate.name, key)
			}
			switch value.(type) {
			case nil, bool, string:
			default:
				if _, ok := numberValue(value); !ok {
					return fmt.Errorf("%s: field %q must be a scalar value", predicate.name, key)
				}
			}
		}
	}
	return nil
}

func validFieldPath(path string, lists bool) bool {
	parts := strings.Split(path, ".")
	for i, part := range parts {
		if lists && i < len(parts)-1 {
			part = strings.TrimSuffix(part, "[]")
		}
		if strings.TrimSpace(part) == "" || strings.ContainsAny(part, "[]") {
			return false
		}
	}
	return true
}

// EvaluateResources checks each selected resource's list instances independently.
// The second return reports a resource selected by kind, API group and prefix,
// even if its list is missing or no item matches when.
func EvaluateResources(cfg Config, resources []render.Resource, repoRoot string) ([]Violation, bool) {
	var violations []Violation
	touched := false
	for _, g := range cfg.Guards {
		if g.Type != TypeListItemRequires {
			continue
		}
		for _, r := range resources {
			group, _, grouped := strings.Cut(r.APIVersion, "/")
			if !grouped {
				group = ""
			}
			if r.Kind != g.Kind || (g.APIGroup != "" && g.APIGroup != group) {
				continue
			}
			source := r.SourcePath
			if filepath.IsAbs(source) {
				var err error
				source, err = filepath.Rel(repoRoot, source)
				if err != nil {
					continue
				}
			}
			source = filepath.ToSlash(filepath.Clean(source))
			if g.Prefix != "" {
				prefix := strings.TrimSuffix(filepath.ToSlash(filepath.Clean(g.Prefix)), "/") + "/"
				if !strings.HasPrefix(source, prefix) {
					continue
				}
			}
			touched = true
			walkLists(r.Body, strings.Split(g.List, "."), "", "", func(items []any, location string) {
				triggered, satisfied := false, false
				for _, item := range items {
					triggered = triggered || matchesFields(item, g.When)
					satisfied = satisfied || matchesFields(item, g.Require)
				}
				if triggered && !satisfied {
					name := r.Name
					if r.Namespace != "" {
						name = r.Namespace + "/" + name
					}
					violations = append(violations, Violation{Guard: g, Detail: fmt.Sprintf(
						"%s %s in %s at %s: an item matches when %s, but no item in the same list matches require %s",
						r.Kind, name, source, location, predicateText(g.When), predicateText(g.Require))})
				}
			})
		}
	}
	return violations, touched
}

// walkLists retains concrete indices through every [] traversal. The nearest
// iterated parent supplies the optional name; lists are never flattened.
func walkLists(value any, parts []string, path, parent string, visit func([]any, string)) {
	object, ok := value.(map[string]any)
	if !ok {
		return
	}
	part := parts[0]
	key := strings.TrimSuffix(part, "[]")
	value, exists := object[key]
	if !exists {
		return
	}
	if path != "" {
		path += "."
	}
	path += key
	if len(parts) == 1 {
		if items, ok := value.([]any); ok {
			location := path
			if parent != "" {
				location += " (parent: " + parent + ")"
			}
			visit(items, location)
		}
		return
	}
	if strings.HasSuffix(part, "[]") {
		items, _ := value.([]any)
		for i, item := range items {
			itemPath := fmt.Sprintf("%s[%d]", path, i)
			itemParent := itemPath
			if obj, ok := item.(map[string]any); ok {
				if name, exists := obj["name"]; exists {
					itemParent += fmt.Sprintf(" (name: %v)", name)
				}
			}
			walkLists(item, parts[1:], itemPath, itemParent, visit)
		}
		return
	}
	walkLists(value, parts[1:], path, parent, visit)
}

func matchesFields(item any, fields map[string]any) bool {
	for path, want := range fields {
		value := item
		for _, key := range strings.Split(path, ".") {
			object, ok := value.(map[string]any)
			if !ok {
				return false
			}
			var exists bool
			value, exists = object[key]
			if !exists {
				return false
			}
		}
		if n, ok := numberValue(want); ok {
			actual, ok := numberValue(value)
			if !ok || n.Cmp(actual) != 0 {
				return false
			}
		} else if !reflect.DeepEqual(value, want) {
			return false
		}
	}
	return true
}

// Compare decimal representations exactly, without converting integers to
// float64 (which would lose precision for large YAML integers).
func numberValue(value any) (*big.Rat, bool) {
	if value == nil {
		return nil, false
	}
	switch reflect.TypeOf(value).Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
	case reflect.String:
		if _, ok := value.(json.Number); !ok {
			return nil, false
		}
	default:
		return nil, false
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil, false
	}
	return new(big.Rat).SetString(string(data))
}

func predicateText(fields map[string]any) string {
	// JSON sorts map keys, keeping the banner deterministic and types visible.
	data, _ := json.Marshal(fields)
	return string(data)
}
