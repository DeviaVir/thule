//go:build live

package main

import "github.com/example/thule/internal/orchestrator"

func newClusterReader() (orchestrator.ClusterReader, error) {
	return orchestrator.NewLiveClusterReader()
}
