//go:build live

package main

import "github.com/DeviaVir/thule/internal/orchestrator"

func newClusterReader() (orchestrator.ClusterReader, error) {
	return orchestrator.NewLiveClusterReader()
}
