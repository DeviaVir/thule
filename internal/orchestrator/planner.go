package orchestrator

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/example/thule/internal/config"
	"github.com/example/thule/internal/diff"
	"github.com/example/thule/internal/policy"
	"github.com/example/thule/internal/project"
	"github.com/example/thule/internal/render"
	"github.com/example/thule/internal/report"
	"github.com/example/thule/internal/run"
	"github.com/example/thule/internal/vcs"
)

type ClusterReader interface {
	ListResources(ctx context.Context, clusterRef string, namespace string) ([]render.Resource, error)
}

type Planner struct {
	repoRoot   string
	cluster    ClusterReader
	comments   vcs.CommentStore
	status     vcs.StatusPublisher
	runs       run.Store
	policyEval policy.Evaluator
}

func NewPlanner(repoRoot string, cluster ClusterReader, comments vcs.CommentStore, status vcs.StatusPublisher, runs run.Store, policyEval policy.Evaluator) *Planner {
	return &Planner{repoRoot: repoRoot, cluster: cluster, comments: comments, status: status, runs: runs, policyEval: policyEval}
}

func (p *Planner) PlanForEvent(ctx context.Context, evt MergeRequestEvent) error {
	if p.runs != nil {
		p.runs.SetLatestSHA(evt.MergeReqID, evt.HeadSHA)
	}
	if p.status != nil {
		p.status.SetStatus(vcs.StatusCheck{MergeReqID: evt.MergeReqID, SHA: evt.HeadSHA, Context: "thule/plan", State: vcs.CheckPending, Description: "Thule plan running"})
	}

	projects := project.DiscoverFromChangedFiles(evt.ChangedFiles)
	for _, prj := range projects {
		if p.runs != nil && p.runs.IsStale(evt.MergeReqID, evt.HeadSHA) {
			return nil
		}
		configPath := filepath.Join(p.repoRoot, prj.ConfigPath)
		if _, err := os.Stat(configPath); err != nil {
			continue
		}
		cfg, err := config.Load(configPath)
		if err != nil {
			p.finishWithError(evt, 0, err)
			return err
		}

		var rr run.Record
		if p.runs != nil {
			rr = p.runs.Start(evt.MergeReqID, evt.HeadSHA, cfg.Project)
		}

		desired, err := render.RenderProject(filepath.Join(p.repoRoot, prj.Root), cfg)
		if err != nil {
			p.finishWithError(evt, rr.ID, err)
			return err
		}
		actual, err := p.cluster.ListResources(ctx, cfg.ClusterRef, cfg.Namespace)
		if err != nil {
			p.finishWithError(evt, rr.ID, err)
			return err
		}
		changes, summary := diff.Compute(desired, actual, diff.Options{PruneDeletes: cfg.Diff.Prune, IgnoreFields: cfg.Diff.IgnoreFields})

		findings := []policy.Finding{}
		if p.policyEval != nil {
			findings = p.policyEval.Evaluate(desired, cfg.Policy.Profile)
		}

		body := report.BuildPlanComment(cfg.Project, evt.HeadSHA, changes, summary, findings, cfg.Comment.MaxResourceDetails)
		c := p.comments.PostOrSupersede(evt.MergeReqID, body)
		if p.runs != nil {
			p.runs.AddArtifact(rr.ID, "plan-comment", body)
			p.runs.AddArtifact(rr.ID, "comment-id", fmt.Sprintf("%d", c.ID))
			p.runs.Complete(rr.ID, run.StateSuccess, "")
		}
	}

	if p.status != nil {
		p.status.SetStatus(vcs.StatusCheck{MergeReqID: evt.MergeReqID, SHA: evt.HeadSHA, Context: "thule/plan", State: vcs.CheckSuccess, Description: "Thule plan completed"})
	}
	return nil
}

func (p *Planner) finishWithError(evt MergeRequestEvent, runID int64, err error) {
	if p.runs != nil && runID > 0 {
		p.runs.Complete(runID, run.StateFailed, err.Error())
	}
	if p.status != nil {
		p.status.SetStatus(vcs.StatusCheck{MergeReqID: evt.MergeReqID, SHA: evt.HeadSHA, Context: "thule/plan", State: vcs.CheckFailed, Description: err.Error()})
	}
}
