package orchestrator

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

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

type ProjectAwareClusterReader interface {
	ListResourcesWithProject(ctx context.Context, projectID, clusterRef, namespace string, desired []render.Resource) ([]render.Resource, error)
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
	sort.SliceStable(projects, func(i, j int) bool {
		return projects[i].ConfigPath < projects[j].ConfigPath
	})
	planned := false
	projectPlans := make([]report.ProjectPlan, 0, len(projects))
	runIDs := make([]int64, 0, len(projects))
	maxResourceDetails := 0
	failRuns := func(currentRunID int64, err error) {
		if p.runs != nil {
			for _, runID := range runIDs {
				if runID <= 0 || runID == currentRunID {
					continue
				}
				p.runs.Complete(runID, run.StateFailed, err.Error())
			}
		}
	}
	for _, prj := range projects {
		if p.runs != nil && p.runs.IsStale(evt.MergeReqID, evt.HeadSHA) {
			return nil
		}
		configPath := filepath.Join(p.repoRoot, prj.ConfigPath)
		if _, err := os.Stat(configPath); err != nil {
			continue
		}
		planned = true
		cfg, err := config.Load(configPath)
		if err != nil {
			failRuns(0, err)
			p.finishWithError(evt, 0, err)
			return err
		}
		if cfg.Comment.MaxResourceDetails > maxResourceDetails {
			maxResourceDetails = cfg.Comment.MaxResourceDetails
		}

		desired, err := render.RenderProject(filepath.Join(p.repoRoot, prj.Root), cfg)
		if err != nil {
			failRuns(0, err)
			p.finishWithError(evt, 0, err)
			return err
		}
		desired = filterDesiredByChangedFiles(desired, evt.ChangedFiles, p.repoRoot)
		if len(desired) == 0 {
			continue
		}

		var rr run.Record
		if p.runs != nil {
			rr = p.runs.Start(evt.MergeReqID, evt.HeadSHA, cfg.Project)
			runIDs = append(runIDs, rr.ID)
		}
		var actual []render.Resource
		if projectAware, ok := p.cluster.(ProjectAwareClusterReader); ok {
			actual, err = projectAware.ListResourcesWithProject(ctx, cfg.Project, cfg.ClusterRef, cfg.Namespace, desired)
		} else {
			actual, err = p.cluster.ListResources(ctx, cfg.ClusterRef, cfg.Namespace)
		}
		if err != nil {
			failRuns(rr.ID, err)
			p.finishWithError(evt, rr.ID, err)
			return err
		}
		changes, summary := diff.Compute(desired, actual, diff.Options{
			PruneDeletes:            cfg.Diff.Prune,
			IgnoreFields:            cfg.Diff.IgnoreFields,
			IgnoreActualExtraFields: true,
		})

		findings := []policy.Finding{}
		if p.policyEval != nil {
			findings = p.policyEval.Evaluate(desired, cfg.Policy.Profile)
		}
		projectPlans = append(projectPlans, report.ProjectPlan{
			Project:  cfg.Project,
			Changes:  changes,
			Summary:  summary,
			Findings: findings,
		})
	}

	if !planned && p.comments != nil {
		body := report.BuildNoChangesComment(evt.HeadSHA, evt.ChangedFiles, 50)
		p.comments.PostOrSupersede(evt.MergeReqID, body)
	}

	if planned {
		body := ""
		if len(projectPlans) == 0 {
			body = report.BuildNoChangesComment(evt.HeadSHA, evt.ChangedFiles, 50)
		} else {
			body = report.BuildAggregatedPlanComment(evt.HeadSHA, projectPlans, maxResourceDetails)
		}
		var commentID int64
		if p.comments != nil {
			c := p.comments.PostOrSupersede(evt.MergeReqID, body)
			commentID = c.ID
		}
		if p.runs != nil {
			for _, runID := range runIDs {
				if runID <= 0 {
					continue
				}
				p.runs.AddArtifact(runID, "plan-comment", body)
				if commentID > 0 {
					p.runs.AddArtifact(runID, "comment-id", fmt.Sprintf("%d", commentID))
				}
				p.runs.Complete(runID, run.StateSuccess, "")
			}
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

func filterDesiredByChangedFiles(desired []render.Resource, changedFiles []string, repoRoot string) []render.Resource {
	if len(desired) == 0 || len(changedFiles) == 0 {
		return desired
	}
	changedManifestFiles := map[string]struct{}{}
	for _, f := range changedFiles {
		cleaned := filepath.ToSlash(filepath.Clean(strings.TrimSpace(f)))
		switch strings.ToLower(filepath.Ext(cleaned)) {
		case ".yaml", ".yml":
			changedManifestFiles[cleaned] = struct{}{}
		}
	}
	if len(changedManifestFiles) == 0 {
		return nil
	}

	filtered := make([]render.Resource, 0, len(desired))
	for _, r := range desired {
		if r.SourcePath == "" {
			continue
		}
		rel, err := filepath.Rel(repoRoot, r.SourcePath)
		if err != nil {
			continue
		}
		rel = filepath.ToSlash(filepath.Clean(rel))
		if _, ok := changedManifestFiles[rel]; ok {
			filtered = append(filtered, r)
		}
	}
	return filtered
}
