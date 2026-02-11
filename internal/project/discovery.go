package project

import (
	"path/filepath"
	"strings"
)

type DiscoveredProject struct {
	Root       string
	ConfigPath string
}

const configFilename = "thule.conf"

func DiscoverFromChangedFiles(changedFiles []string) []DiscoveredProject {
	uniq := map[string]struct{}{}
	for _, p := range changedFiles {
		if strings.TrimSpace(p) == "" {
			continue
		}
		dir := filepath.Dir(filepath.Clean(p))
		for {
			candidate := filepath.Join(dir, configFilename)
			uniq[candidate] = struct{}{}
			if dir == "." || dir == "/" {
				break
			}
			next := filepath.Dir(dir)
			if next == dir {
				break
			}
			dir = next
		}
	}

	out := make([]DiscoveredProject, 0, len(uniq))
	for configPath := range uniq {
		out = append(out, DiscoveredProject{Root: filepath.Dir(configPath), ConfigPath: configPath})
	}
	return out
}
