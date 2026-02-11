package config

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"

	"github.com/example/thule/pkg/thuleconfig"
)

//go:embed thule.schema.json
var thuleSchema []byte

func Load(path string) (thuleconfig.Config, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return thuleconfig.Config{}, fmt.Errorf("read config: %w", err)
	}
	if err := Validate(content); err != nil {
		return thuleconfig.Config{}, err
	}

	var cfg thuleconfig.Config
	if err := json.Unmarshal(content, &cfg); err != nil {
		return thuleconfig.Config{}, fmt.Errorf("decode config: %w", err)
	}

	return cfg, nil
}

func Validate(content []byte) error {
	var cfg thuleconfig.Config
	if err := json.Unmarshal(content, &cfg); err != nil {
		return fmt.Errorf("invalid JSON/YAML-converted config payload: %w", err)
	}

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

	_ = thuleSchema // embedded schema is shipped for tooling/documentation parity.
	return nil
}
