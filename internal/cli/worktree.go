package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/shurastik/agterm-kit/internal/worktree"
)

// Worktrees is the git side of worktree mode, as `agk apply start`, `agk sync`
// and `agk finish` use it. The production implementation is worktree.Git;
// tests assert the call sequence against a fake.
type Worktrees interface {
	// Locate finds the main checkout and the worktree root holding dir.
	Locate(ctx context.Context, dir string) (worktree.Location, error)
	// CurrentBranch is the branch checked out in dir (worktree.ErrDetached when none).
	CurrentBranch(ctx context.Context, dir string) (string, error)
	// Upstream is the upstream of branch, and false when it has none.
	Upstream(ctx context.Context, dir, branch string) (string, bool, error)
	// Lookup inspects .worktrees/<change> under the main checkout.
	Lookup(ctx context.Context, main, change string) (worktree.Entry, error)
	BranchExists(ctx context.Context, dir, branch string) (bool, error)

	PullFFOnly(ctx context.Context, dir string) error
	// CommitPaths commits the uncommitted content of paths only; false when there is none.
	CommitPaths(ctx context.Context, root, message string, paths ...string) (bool, error)
	// Add creates .worktrees/<change> on a new branch <change> from base and records agkBase.
	Add(ctx context.Context, main, change, base string) (string, error)
	AgkBase(ctx context.Context, dir, branch string) (string, bool, error)

	// Status lists uncommitted and untracked (not ignored) files.
	Status(ctx context.Context, dir string) ([]string, error)
	ArchivedOnBranch(ctx context.Context, dir, change string) (bool, error)
	IsAncestor(ctx context.Context, dir, ancestor, descendant string) (bool, error)
	Head(ctx context.Context, dir, ref string) (string, error)
	// Rebase rebases the branch checked out in dir onto onto; *worktree.ConflictError on a conflict.
	Rebase(ctx context.Context, dir, onto string) error
	MergeFFOnly(ctx context.Context, dir, branch string) error
	Remove(ctx context.Context, dir, path string) error
	DeleteBranch(ctx context.Context, dir, branch string) error
}

var _ Worktrees = worktree.Git{}

// planCommitMessage is the message of the commit that puts a change's
// artifacts on the base branch before its worktree is created.
func planCommitMessage(change string) string { return "docs(openspec): plan " + change }

// checkReady refuses, with exit 1, a change that is missing, blocked or done.
func (e *Env) checkReady(ctx context.Context, root, change string) (code int, ok bool) {
	state, msg, err := e.ApplyState(ctx, root, change)
	switch {
	case err != nil:
		return e.errorf("%v", err), false
	case state == "all_done":
		return e.errorf("nothing to apply: every task of %s is done", change), false
	case state != "ready":
		return e.errorf("change %s is not ready to apply (%s): %s", change, state, msg), false
	}
	return ExitOK, true
}

// worktreeRoot prepares worktree mode for `agk apply start` and returns the
// root the applier runs in: the change's worktree, the linked worktree the
// command runs in, or a new worktree. Every refusal comes before the first
// change; see design decision 3. wt is the worktree to report in stdout, and
// pulled says whether the base branch was pulled.
func (e *Env) worktreeRoot(ctx context.Context, change string, noPull bool) (root string, wt *string, pulled bool, code int, ok bool) {
	fail := func(format string, args ...any) (string, *string, bool, int, bool) {
		return "", nil, false, e.errorf(format, args...), false
	}
	loc, err := e.Worktree.Locate(ctx, e.Cwd)
	if err != nil {
		return fail("not in a git repository: %v", err)
	}
	reuse := func(dir string) (string, *string, bool, int, bool) {
		if code, ok := e.checkReady(ctx, dir, change); !ok {
			return "", nil, false, code, false
		}
		return dir, &dir, false, ExitOK, true
	}
	if loc.Linked() {
		// Already in a linked worktree: no git steps, the applier runs here.
		if loc.Root == worktree.WorktreePath(loc.Main, change) {
			return reuse(loc.Root)
		}
		if code, ok := e.checkReady(ctx, loc.Root, change); !ok {
			return "", nil, false, code, false
		}
		return loc.Root, nil, false, ExitOK, true
	}
	main := loc.Main
	entry, err := e.Worktree.Lookup(ctx, main, change)
	if err != nil {
		return fail("%v", err)
	}
	switch entry.Kind {
	case worktree.IsWorktree:
		return reuse(entry.Path)
	case worktree.OtherBranch:
		return fail("%s is a worktree of branch %q, not %q: remove or rename it first", entry.Path, entry.Branch, change)
	case worktree.PlainDir:
		return fail("%s exists and is not a worktree of branch %q: move it away first", entry.Path, change)
	}
	if has, err := e.Worktree.BranchExists(ctx, main, change); err != nil {
		return fail("%v", err)
	} else if has {
		return fail("branch %s already exists without the worktree %s: delete or rename the branch first", change, entry.Path)
	}
	base, err := e.Worktree.CurrentBranch(ctx, main)
	if errors.Is(err, worktree.ErrDetached) {
		return fail("the main checkout %s is on a detached HEAD: worktree mode needs a base branch, check one out first", main)
	} else if err != nil {
		return fail("%v", err)
	}
	if code, ok := e.checkReady(ctx, main, change); !ok {
		return "", nil, false, code, false
	}
	pulled, err = e.pullBase(ctx, main, base, noPull)
	if err != nil {
		return fail("%v", err)
	}
	if pulled {
		// The pull may have finished or removed the change.
		if code, ok := e.checkReady(ctx, main, change); !ok {
			return "", nil, true, code, false
		}
	}
	if _, err := e.Worktree.CommitPaths(ctx, main, planCommitMessage(change), "openspec/changes/"+change); err != nil {
		return fail("commit the plan of %s on %s: %v", change, base, err)
	}
	path, err := e.Worktree.Add(ctx, main, change, base)
	if err != nil {
		return fail("create the worktree of %s (the plan commit on %s stays): %v", change, base, err)
	}
	return path, &path, pulled, ExitOK, true
}

// changeLoc is where the worktree of a change lives, as `agk sync` and
// `agk finish` find it.
type changeLoc struct {
	Main string // the main checkout
	Path string // .worktrees/<change> under it
	Base string // branch.<change>.agkBase
	// Present: the worktree exists. False only for a finish rerun after the
	// worktree was removed.
	Present bool
	// Merged: branch <change> is already contained in the base (finish only).
	Merged bool
}

// locateChange applies the location checks shared by sync and finish: the
// change's worktree .worktrees/<change> is a worktree of branch <change>,
// branch.<change>.agkBase is set, and the main checkout has that base checked
// out. With finish set, a branch already contained in the base is a rerun
// after an earlier merge: the base need not be checked out any more, and a
// missing worktree counts as already removed.
func (e *Env) locateChange(ctx context.Context, change string, finish bool) (changeLoc, error) {
	if !changeName.MatchString(change) {
		return changeLoc{}, fmt.Errorf("change name %q is not kebab-case", change)
	}
	loc, err := e.Worktree.Locate(ctx, e.Cwd)
	if err != nil {
		return changeLoc{}, fmt.Errorf("not in a git repository: %v", err)
	}
	main := loc.Main
	entry, err := e.Worktree.Lookup(ctx, main, change)
	if err != nil {
		return changeLoc{}, err
	}
	out := changeLoc{Main: main, Path: entry.Path, Present: entry.Kind == worktree.IsWorktree}
	branch, err := e.Worktree.BranchExists(ctx, main, change)
	if err != nil {
		return changeLoc{}, err
	}
	switch entry.Kind {
	case worktree.OtherBranch:
		return changeLoc{}, fmt.Errorf("%s is a worktree of branch %q, not %q", entry.Path, entry.Branch, change)
	case worktree.PlainDir:
		return changeLoc{}, fmt.Errorf("%s is not a worktree of branch %q", entry.Path, change)
	}
	if !branch {
		return changeLoc{}, fmt.Errorf("there is no worktree %s and no branch %s: nothing to %s", entry.Path, change, verb(finish))
	}
	base, set, err := e.Worktree.AgkBase(ctx, main, change)
	if err != nil {
		return changeLoc{}, err
	}
	if !set {
		return changeLoc{}, fmt.Errorf("branch.%s.agkBase is not set: %s is not a worktree that `agk apply start` made", change, change)
	}
	out.Base = base
	if finish {
		merged, err := e.Worktree.IsAncestor(ctx, main, change, base)
		if err != nil {
			return changeLoc{}, err
		}
		out.Merged = merged
	}
	if !out.Present && !out.Merged {
		return changeLoc{}, fmt.Errorf("the worktree %s is missing but branch %s is not merged into %s: restore the worktree, or merge or delete the branch yourself", entry.Path, change, base)
	}
	if !out.Merged {
		current, err := e.Worktree.CurrentBranch(ctx, main)
		if err != nil {
			return changeLoc{}, fmt.Errorf("the main checkout %s has no branch checked out; check out %s: %v", main, base, err)
		}
		if current != base {
			return changeLoc{}, fmt.Errorf("%s records base branch %s but the main checkout %s has %s checked out", change, base, main, current)
		}
	}
	return out, nil
}

func verb(finish bool) string {
	if finish {
		return "finish"
	}
	return "sync"
}

// requireClean refuses a worktree with uncommitted or untracked files.
func (e *Env) requireClean(ctx context.Context, path string) error {
	files, err := e.Worktree.Status(ctx, path)
	if err != nil {
		return err
	}
	if len(files) > 0 {
		return fmt.Errorf("%s has uncommitted files, commit them first: %s", path, strings.Join(files, ", "))
	}
	return nil
}

// pullBase runs `git pull --ff-only` of base in the main checkout under the
// rules shared by start and sync: skipped with noPull or without an upstream.
func (e *Env) pullBase(ctx context.Context, main, base string, noPull bool) (pulled bool, err error) {
	if noPull {
		return false, nil
	}
	_, has, err := e.Worktree.Upstream(ctx, main, base)
	if err != nil || !has {
		return false, err
	}
	if err := e.Worktree.PullFFOnly(ctx, main); err != nil {
		return false, fmt.Errorf("pull %s: %v", base, err)
	}
	return true, nil
}
