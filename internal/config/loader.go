package config

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/example/thule/pkg/thuleconfig"
)

//go:embed thule.schema.json
var thuleSchema []byte

func Load(path string) (thuleconfig.Config, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return thuleconfig.Config{}, fmt.Errorf("read config: %w", err)
	}
	cfg, err := Decode(content)
	if err != nil {
		return thuleconfig.Config{}, err
	}
	if err := Validate(cfg); err != nil {
		return thuleconfig.Config{}, err
	}
	return cfg, nil
}

func Decode(content []byte) (thuleconfig.Config, error) {
	var cfg thuleconfig.Config
	if err := json.Unmarshal(content, &cfg); err == nil {
		return cfg, nil
	}
	return decodeSimpleYAML(string(content))
}

func ValidateBytes(content []byte) error {
	cfg, err := Decode(content)
	if err != nil {
		return err
	}
	return Validate(cfg)
}

func Validate(cfg thuleconfig.Config) error {
	if cfg.Version == "" || cfg.Project == "" || cfg.ClusterRef == "" || cfg.Namespace == "" {
		return fmt.Errorf("missing required top-level fields: version, project, clusterRef, namespace")
	}

	switch cfg.Render.Mode {
	case "yaml", "kustomize", "helm", "flux":
	default:
		return fmt.Errorf("unsupported render.mode %q", cfg.Render.Mode)
	}

	if cfg.Render.Path == "" {
		return fmt.Errorf("render.path is required")
	}

	_ = thuleSchema
	return nil
}

func decodeSimpleYAML(in string) (thuleconfig.Config, error) {
	cfg := thuleconfig.Config{}
	lines := strings.Split(in, "\n")
	section := ""
	subsection := ""

	for _, raw := range lines {
		if strings.TrimSpace(raw) == "" || strings.HasPrefix(strings.TrimSpace(raw), "#") {
			continue
		}
		indent := len(raw) - len(strings.TrimLeft(raw, " "))
		line := strings.TrimSpace(raw)

		if strings.HasPrefix(line, "- ") {
			item := strings.Trim(strings.TrimSpace(strings.TrimPrefix(line, "- ")), `"'`)
			switch section + "." + subsection {
			case "diff.ignoreFields":
				cfg.Diff.IgnoreFields = append(cfg.Diff.IgnoreFields, item)
			case "render.helm.valuesFiles":
				cfg.Render.Helm.ValuesFiles = append(cfg.Render.Helm.ValuesFiles, item)
			case "render.flux.includeKinds":
				cfg.Render.Flux.IncludeKinds = append(cfg.Render.Flux.IncludeKinds, item)
			}
			continue
		}

		if strings.HasSuffix(line, ":") {
			key := strings.TrimSuffix(line, ":")
			switch indent {
			case 0:
				section = key
				subsection = ""
			case 2:
				subsection = key
			}
			continue
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		k := strings.TrimSpace(parts[0])
		v := strings.Trim(strings.TrimSpace(parts[1]), `"'`)

		switch section {
		case "":
			switch k {
			case "version":
				cfg.Version = v
			case "project":
				cfg.Project = v
			case "clusterRef":
				cfg.ClusterRef = v
			case "namespace":
				cfg.Namespace = v
			}
		case "render":
			switch subsection {
			case "":
				switch k {
				case "mode":
					cfg.Render.Mode = v
				case "path":
					cfg.Render.Path = v
				}
			case "helm":
				if k == "releaseName" {
					cfg.Render.Helm.ReleaseName = v
				}
			}
		case "diff":
			if k == "prune" {
				cfg.Diff.Prune = (v == "true")
			}
		case "comment":
			if k == "maxResourceDetails" {
				if iv, err := strconv.Atoi(v); err == nil {
					cfg.Comment.MaxResourceDetails = iv
				}
			}
		}
	}

	if cfg.Version == "" && cfg.Project == "" && cfg.ClusterRef == "" && cfg.Namespace == "" && cfg.Render.Mode == "" && cfg.Render.Path == "" {
		return thuleconfig.Config{}, fmt.Errorf("invalid YAML/JSON config payload")
	}
	return cfg, nil
}
