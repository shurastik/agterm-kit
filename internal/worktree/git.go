// Package worktree runs the git operations behind agk's worktree mode: the
// base-branch preparation of `agk apply start`, `agk sync` and `agk finish`.
// It calls the real git binary; its tests run on temporary repositories.
package worktree

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// Git is the real implementation of every operation. The zero value works.
type Git struct{}

// ErrDetached is returned when HEAD is not on a branch.
var ErrDetached = errors.New("HEAD is detached")

// exitError is a failed git command; Code is its exit status (-1 when git did
// not run) and Message the trimmed stderr, or stdout when stderr is empty.
type exitError struct {
	Args    []string
	Code    int
	Message string
}

func (e *exitError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("git %s: exit status %d", strings.Join(e.Args, " "), e.Code)
	}
	return fmt.Sprintf("git %s: %s", strings.Join(e.Args, " "), e.Message)
}

// run runs git in dir and returns its stdout.
func run(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	// Never wait for a password or an editor: agk runs unattended.
	cmd.Env = append(cmd.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_EDITOR=true")
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	if err == nil {
		return out.String(), nil
	}
	msg := strings.TrimSpace(errb.String())
	if msg == "" {
		msg = strings.TrimSpace(out.String())
	}
	code := -1
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		code = ee.ExitCode()
	} else {
		msg = err.Error()
	}
	return out.String(), &exitError{Args: args, Code: code, Message: msg}
}

// exitCode returns the git exit status behind err, or -1.
func exitCode(err error) int {
	var ee *exitError
	if errors.As(err, &ee) {
		return ee.Code
	}
	return -1
}

func trim(s string, err error) (string, error) {
	return strings.TrimSpace(s), err
}

// Location says where a directory sits in a repository.
type Location struct {
	Main string // the main checkout (the first entry of `git worktree list`)
	Root string // the top of the worktree holding the directory
}

// Linked reports whether the directory is in a linked worktree, not the main
// checkout.
func (l Location) Linked() bool { return l.Main != l.Root }

// listEntry is one `git worktree list --porcelain` record.
type listEntry struct {
	Path   string
	Branch string // short name; "" when detached or bare
}

func list(ctx context.Context, dir string) ([]listEntry, error) {
	out, err := run(ctx, dir, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	var entries []listEntry
	for _, block := range strings.Split(strings.TrimSpace(out), "\n\n") {
		var e listEntry
		for _, line := range strings.Split(block, "\n") {
			switch {
			case strings.HasPrefix(line, "worktree "):
				e.Path = real(strings.TrimPrefix(line, "worktree "))
			case strings.HasPrefix(line, "branch "):
				e.Branch = strings.TrimPrefix(strings.TrimPrefix(line, "branch "), "refs/heads/")
			}
		}
		if e.Path != "" {
			entries = append(entries, e)
		}
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("git worktree list in %s: no worktrees", dir)
	}
	return entries, nil
}

// real resolves symlinks so paths from git and from the caller compare equal.
func real(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return filepath.Clean(p)
}

// Locate finds the main checkout and the worktree root of dir.
func (Git) Locate(ctx context.Context, dir string) (Location, error) {
	top, err := trim(run(ctx, dir, "rev-parse", "--show-toplevel"))
	if err != nil {
		return Location{}, err
	}
	entries, err := list(ctx, dir)
	if err != nil {
		return Location{}, err
	}
	return Location{Main: entries[0].Path, Root: real(top)}, nil
}

// CurrentBranch is the branch checked out in dir; ErrDetached on a detached HEAD.
func (Git) CurrentBranch(ctx context.Context, dir string) (string, error) {
	b, err := trim(run(ctx, dir, "symbolic-ref", "--short", "-q", "HEAD"))
	if err != nil {
		if exitCode(err) == 1 {
			return "", ErrDetached
		}
		return "", err
	}
	return b, nil
}

// Upstream is the upstream of branch ("origin/main"), and false when it has none.
func (Git) Upstream(ctx context.Context, dir, branch string) (string, bool, error) {
	up, err := trim(run(ctx, dir, "rev-parse", "--abbrev-ref", "--symbolic-full-name", branch+"@{upstream}"))
	if err != nil {
		// 128: no upstream configured (or no such branch, which the caller
		// has already established).
		if exitCode(err) == 128 {
			return "", false, nil
		}
		return "", false, err
	}
	return up, true, nil
}

// Kind classifies what `.worktrees/<change>` is.
type Kind int

const (
	// Absent: nothing is at the path.
	Absent Kind = iota
	// IsWorktree: a worktree of branch <change>.
	IsWorktree
	// OtherBranch: a worktree of another branch, or of a detached HEAD.
	OtherBranch
	// PlainDir: something at the path that is not a worktree.
	PlainDir
)

// Entry is the lookup result for `.worktrees/<change>`.
type Entry struct {
	Path   string // <main>/.worktrees/<change>
	Kind   Kind
	Branch string // the branch checked out there, for IsWorktree and OtherBranch
}

// WorktreePath is where the worktree of change lives under the main checkout.
func WorktreePath(main, change string) string {
	return filepath.Join(main, ".worktrees", change)
}

// Lookup inspects `.worktrees/<change>` under the main checkout.
func (Git) Lookup(ctx context.Context, main, change string) (Entry, error) {
	path := WorktreePath(main, change)
	e := Entry{Path: path}
	entries, err := list(ctx, main)
	if err != nil {
		return e, err
	}
	want := real(path)
	for _, w := range entries {
		if w.Path != want {
			continue
		}
		e.Branch = w.Branch
		if w.Branch == change {
			e.Kind = IsWorktree
		} else {
			e.Kind = OtherBranch
		}
		return e, nil
	}
	if exists(path) {
		e.Kind = PlainDir
	}
	return e, nil
}

// BranchExists reports whether the local branch exists.
func (Git) BranchExists(ctx context.Context, dir, branch string) (bool, error) {
	_, err := run(ctx, dir, "show-ref", "--verify", "--quiet", "refs/heads/"+branch)
	if err == nil {
		return true, nil
	}
	if exitCode(err) == 1 {
		return false, nil
	}
	return false, err
}
