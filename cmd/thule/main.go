package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/example/thule/internal/config"
	"github.com/example/thule/internal/diff"
	"github.com/example/thule/internal/policy"
	"github.com/example/thule/internal/render"
	"github.com/example/thule/internal/report"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "plan":
		runPlan(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
}

func runPlan(args []string) {
	fs := flag.NewFlagSet("plan", flag.ExitOnError)
	project := fs.String("project", ".", "project directory containing thule.yaml")
	sha := fs.String("sha", "local", "commit sha label for report output")
	fs.Parse(args)

	cfgPath := filepath.Join(*project, "thule.yaml")
	cfg, err := config.Load(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(1)
	}
	desired, err := render.RenderProject(*project, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "render project: %v\n", err)
		os.Exit(1)
	}
	changes, summary := diff.Compute(desired, nil, diff.Options{PruneDeletes: cfg.Diff.Prune, IgnoreFields: cfg.Diff.IgnoreFields})
	findings := policy.NewBuiltinEvaluator().Evaluate(desired, cfg.Policy.Profile)
	body := report.BuildPlanComment(cfg.Project, *sha, changes, summary, findings, cfg.Comment.MaxResourceDetails)
	fmt.Println(strings.TrimSpace(body))
}

func usage() {
	fmt.Println("thule <command>\n\nCommands:\n  plan --project <path> [--sha <sha>]  Run local plan preview")
}
