package render

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/example/thule/pkg/thuleconfig"
)

type Resource struct {
	APIVersion string
	Kind       string
	Namespace  string
	Name       string
	Body       map[string]any
}

func (r Resource) ID() string {
	ns := r.Namespace
	if ns == "" {
		ns = "_cluster"
	}
	return fmt.Sprintf("%s|%s|%s|%s", r.APIVersion, r.Kind, ns, r.Name)
}

func RenderProject(projectRoot string, cfg thuleconfig.Config) ([]Resource, error) {
	target := filepath.Join(projectRoot, cfg.Render.Path)
	switch cfg.Render.Mode {
	case "yaml", "kustomize", "helm":
		return renderYAMLPath(target)
	case "flux":
		resources, err := renderYAMLPath(target)
		if err != nil {
			return nil, err
		}
		return filterFluxResources(resources, cfg), nil
	default:
		return nil, fmt.Errorf("render mode %q not implemented", cfg.Render.Mode)
	}
}

func filterFluxResources(resources []Resource, cfg thuleconfig.Config) []Resource {
	if len(cfg.Render.Flux.IncludeKinds) == 0 {
		return resources
	}

	allowed := map[string]struct{}{
		"HelmRelease":   {},
		"Kustomization": {},
		"GitRepository": {},
		"OCIRepository": {},
	}
	for _, k := range cfg.Render.Flux.IncludeKinds {
		allowed[k] = struct{}{}
	}

	out := make([]Resource, 0, len(resources))
	for _, r := range resources {
		if _, ok := allowed[r.Kind]; ok {
			out = append(out, r)
		}
	}
	return out
}

func renderYAMLPath(path string) ([]Resource, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	files := []string{}
	if fi.IsDir() {
		err := filepath.WalkDir(path, func(p string, d os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if d.IsDir() {
				return nil
			}
			if strings.HasSuffix(d.Name(), ".yaml") || strings.HasSuffix(d.Name(), ".yml") {
				files = append(files, p)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	} else {
		files = append(files, path)
	}
	sort.Strings(files)

	var out []Resource
	for _, f := range files {
		content, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		res, err := parseYAMLDocuments(string(content))
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", f, err)
		}
		out = append(out, res...)
	}
	return out, nil
}

func parseYAMLDocuments(content string) ([]Resource, error) {
	docs := strings.Split(content, "\n---")
	out := make([]Resource, 0, len(docs))
	for _, d := range docs {
		if strings.TrimSpace(d) == "" {
			continue
		}
		r := Resource{Body: map[string]any{}}
		section := ""
		for _, raw := range strings.Split(d, "\n") {
			if strings.TrimSpace(raw) == "" || strings.HasPrefix(strings.TrimSpace(raw), "#") {
				continue
			}
			trim := strings.TrimSpace(raw)
			if strings.HasSuffix(trim, ":") {
				section = strings.TrimSuffix(trim, ":")
				continue
			}
			parts := strings.SplitN(trim, ":", 2)
			if len(parts) != 2 {
				continue
			}
			k := strings.TrimSpace(parts[0])
			v := strings.Trim(strings.TrimSpace(parts[1]), `"'`)
			switch section {
			case "metadata":
				switch k {
				case "name":
					r.Name = v
				case "namespace":
					r.Namespace = v
				}
			default:
				switch k {
				case "apiVersion":
					r.APIVersion = v
				case "kind":
					r.Kind = v
				}
			}
		}
		if r.APIVersion == "" || r.Kind == "" || r.Name == "" {
			// Skip non-resource YAML (values files, kustomize configs, etc.).
			continue
		}
		r.Body["apiVersion"] = r.APIVersion
		r.Body["kind"] = r.Kind
		r.Body["metadata"] = map[string]any{"name": r.Name, "namespace": r.Namespace}
		out = append(out, r)
	}
	return out, nil
}
