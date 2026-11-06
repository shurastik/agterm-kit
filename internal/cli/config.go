package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"strings"

	"github.com/shurastik/agterm-kit/internal/config"
)

// repoRoot is the repository root of the working directory, or "" outside one.
func (e *Env) repoRoot(ctx context.Context) string {
	root, err := e.RepoRoot(ctx, e.Cwd)
	if err != nil {
		return ""
	}
	return root
}

// resolveConfig loads the user file and, when root is set, root/.agk.toml,
// then applies flags.
func (e *Env) resolveConfig(root string, flags config.Flags) (config.Resolved, error) {
	user, err := config.LoadFile(config.UserFile(e.Getenv, e.Home))
	if err != nil {
		return config.Resolved{}, err
	}
	var repo *config.File
	if root != "" {
		if repo, err = config.LoadFile(config.RepoFile(root)); err != nil {
			return config.Resolved{}, err
		}
	}
	return config.Resolve([]*config.File{user, repo}, flags)
}

// warnSame warns when the reviewer would review its own kind of work.
func (e *Env) warnSame(r config.Resolved) {
	if !config.Same(r.Applier, r.Reviewer) {
		return
	}
	who := r.Applier.Harness
	if r.Applier.Model != "" {
		who += " " + r.Applier.Model
	}
	fmt.Fprintf(e.Stderr, "agk: warning: applier and reviewer are both %s: the review is not independent\n", who)
}

type shownWorker struct {
	Harness        string            `json:"harness"`
	Model          string            `json:"model"`
	Effort         string            `json:"effort"`
	ExtraArgs      []string          `json:"extra_args"`
	PermissionMode *string           `json:"permission_mode,omitempty"`
	Sources        map[string]string `json:"sources"`
}

func show(w config.Worker, src map[string]string) shownWorker {
	s := shownWorker{Harness: w.Harness, Model: w.Model, Effort: w.Effort, ExtraArgs: w.ExtraArgs, Sources: src}
	if s.ExtraArgs == nil {
		s.ExtraArgs = []string{}
	}
	if _, ok := src["permission_mode"]; ok {
		pm := w.PermissionMode
		s.PermissionMode = &pm
	}
	return s
}

// Config implements `agk config [--applier H[:M]] [--reviewer H[:M]]`.
func (e *Env) Config(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("agk config", flag.ContinueOnError)
	fs.SetOutput(e.Stderr)
	var flags config.Flags
	fs.StringVar(&flags.Applier, "applier", "", "applier HARNESS[:MODEL] for this run")
	fs.StringVar(&flags.Reviewer, "reviewer", "", "reviewer HARNESS[:MODEL] for this run")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return ExitOK
		}
		return ExitError
	}
	if fs.NArg() != 0 {
		return e.errorf("unexpected arguments %q", fs.Args())
	}
	r, err := e.resolveConfig(e.repoRoot(ctx), flags)
	if err != nil {
		return e.errorf("%v", err)
	}
	e.warnSame(r)
	b, err := json.MarshalIndent(map[string]shownWorker{
		config.Applier:  show(r.Applier, r.Sources[config.Applier]),
		config.Reviewer: show(r.Reviewer, r.Sources[config.Reviewer]),
	}, "", "  ")
	if err != nil {
		return e.errorf("%v", err)
	}
	fmt.Fprintln(e.Stdout, strings.TrimSpace(string(b)))
	return ExitOK
}
