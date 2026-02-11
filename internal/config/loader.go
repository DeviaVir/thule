package config

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
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
	lines := strings.Split(in, "\n")
	cfg := thuleconfig.Config{}
	section := ""
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasSuffix(line, ":") {
			section = strings.TrimSuffix(line, ":")
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		k := strings.TrimSpace(parts[0])
		v := strings.Trim(strings.TrimSpace(parts[1]), `"'`)
		switch section {
		case "render":
			switch k {
			case "mode":
				cfg.Render.Mode = v
			case "path":
				cfg.Render.Path = v
			}
		default:
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
		}
	}
	if cfg.Version == "" && cfg.Project == "" && cfg.ClusterRef == "" && cfg.Namespace == "" && cfg.Render.Mode == "" && cfg.Render.Path == "" {
		return thuleconfig.Config{}, fmt.Errorf("invalid YAML/JSON config payload")
	}
	return cfg, nil
}
