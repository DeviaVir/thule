package orchestrator

import (
	"context"
	"os"
	"path/filepath"

	"github.com/example/thule/internal/config"
	"github.com/example/thule/internal/diff"
	"github.com/example/thule/internal/project"
	"github.com/example/thule/internal/render"
	"github.com/example/thule/internal/report"
	"github.com/example/thule/internal/vcs"
)

type ClusterReader interface {
	ListResources(ctx context.Context, clusterRef string, namespace string) ([]render.Resource, error)
}

type Planner struct {
	repoRoot string
	cluster  ClusterReader
	comments vcs.CommentStore
}

func NewPlanner(repoRoot string, cluster ClusterReader, comments vcs.CommentStore) *Planner {
	return &Planner{repoRoot: repoRoot, cluster: cluster, comments: comments}
}

func (p *Planner) PlanForEvent(ctx context.Context, evt MergeRequestEvent) error {
	projects := project.DiscoverFromChangedFiles(evt.ChangedFiles)
	for _, prj := range projects {
		configPath := filepath.Join(p.repoRoot, prj.ConfigPath)
		if _, err := os.Stat(configPath); err != nil {
			continue
		}
		cfg, err := config.Load(configPath)
		if err != nil {
			return err
		}
		desired, err := render.RenderProject(filepath.Join(p.repoRoot, prj.Root), cfg)
		if err != nil {
			return err
		}
		actual, err := p.cluster.ListResources(ctx, cfg.ClusterRef, cfg.Namespace)
		if err != nil {
			return err
		}
		changes, summary := diff.Compute(desired, actual)
		body := report.BuildPlanComment(cfg.Project, evt.HeadSHA, changes, summary)
		p.comments.PostOrSupersede(evt.MergeReqID, body)
	}
	return nil
}
