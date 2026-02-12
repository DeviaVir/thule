//go:build !live

package main

import (
	"github.com/example/thule/internal/orchestrator"
	"github.com/example/thule/internal/render"
)

func newClusterReader() (orchestrator.ClusterReader, error) {
	return &orchestrator.MemoryClusterReader{ByClusterNS: map[string][]render.Resource{}}, nil
}
