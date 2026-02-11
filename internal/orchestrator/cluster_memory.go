package orchestrator

import (
	"context"

	"github.com/example/thule/internal/render"
)

type MemoryClusterReader struct {
	ByClusterNS map[string][]render.Resource
}

func (m *MemoryClusterReader) ListResources(_ context.Context, clusterRef string, namespace string) ([]render.Resource, error) {
	if m == nil {
		return nil, nil
	}
	key := clusterRef + "/" + namespace
	items := m.ByClusterNS[key]
	out := make([]render.Resource, len(items))
	copy(out, items)
	return out, nil
}
