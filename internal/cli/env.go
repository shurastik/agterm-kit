// Package cli implements the agk subcommands.
package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"

	"github.com/shurastik/agterm-kit/internal/agterm"
	"github.com/shurastik/agterm-kit/internal/review"
	"github.com/shurastik/agterm-kit/internal/setup"
)

// Exit codes shared by all commands.
const (
	ExitOK        = 0
	ExitError     = 1
	ExitDismissed = 2
	ExitNoAgterm  = 3
)

// Env carries every side effect a command may have.
type Env struct {
	Stdout, Stderr io.Writer
	Getenv         func(string) string
	Ctl            agterm.Ctl
	Runner         review.Runner
	Collect        func(ctx context.Context, dir, base string) (review.Scope, error)
	Now            func() time.Time
	Executable     func() (string, error)
	Cwd            string

	// Machine wiring for agk setup.
	Home      string
	LookPath  func(string) (string, error)
	GitConfig func(key string) (string, error)
	IsExec    func(string) bool
}

// NewEnv returns the production environment.
func NewEnv() (*Env, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	return &Env{
		Stdout:     os.Stdout,
		Stderr:     os.Stderr,
		Getenv:     os.Getenv,
		Ctl:        agterm.NewCLI(),
		Runner:     review.NewCodex(),
		Collect:    review.Collect,
		Now:        time.Now,
		Executable: os.Executable,
		Cwd:        cwd,
		Home:       home,
		LookPath:   exec.LookPath,
		GitConfig:  setup.GlobalGitConfig,
		IsExec:     isExecutable,
	}, nil
}

// session returns the caller's agterm session id, or "" outside agterm.
func (e *Env) session() string {
	if e.Getenv("AGTERM_ENABLED") == "" {
		return ""
	}
	return e.Getenv("AGTERM_SESSION_ID")
}

func (e *Env) errorf(format string, args ...any) int {
	fmt.Fprintf(e.Stderr, "agk: "+format+"\n", args...)
	return ExitError
}

func isExecutable(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.Mode().IsRegular() && fi.Mode()&0o111 != 0
}
