//go:build !live

package main

import (
	"github.com/DeviaVir/thule/internal/orchestrator"
	"github.com/DeviaVir/thule/internal/render"
)

func newClusterReader() (orchestrator.ClusterReader, error) {
	return &orchestrator.MemoryClusterReader{ByClusterNS: map[string][]render.Resource{}}, nil
}
