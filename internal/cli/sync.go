package cli

import (
	"context"
	"errors"
	"flag"
	"strings"

	"github.com/shurastik/agterm-kit/internal/worktree"
)

// syncResult is the stdout of `agk sync`.
type syncResult struct {
	Change  string `json:"change"`
	Base    string `json:"base"`
	Pulled  bool   `json:"pulled"`
	Rebased bool   `json:"rebased"`
	Head    string `json:"head"`
}

// Sync implements `agk sync <change> [--no-pull]`: pull the base branch and
// rebase the change's branch onto it, in the change's worktree.
func (e *Env) Sync(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("agk sync", flag.ContinueOnError)
	fs.SetOutput(e.Stderr)
	noPull := fs.Bool("no-pull", false, "do not pull the base branch first")
	pos, code, ok := parseInterleaved(fs, args)
	if !ok {
		return code
	}
	if len(pos) != 1 {
		return e.errorf("usage: agk sync <change> [--no-pull]")
	}
	change := pos[0]
	where, err := e.locateChange(ctx, change, false)
	if err != nil {
		return e.errorf("%v", err)
	}
	if err := e.requireClean(ctx, where.Path); err != nil {
		return e.errorf("%v", err)
	}
	pulled, err := e.pullBase(ctx, where.Main, where.Base, *noPull)
	if err != nil {
		return e.errorf("%v", err)
	}
	ref := "refs/heads/" + change
	before, err := e.Worktree.Head(ctx, where.Path, ref)
	if err != nil {
		return e.errorf("%v", err)
	}
	if err := e.Worktree.Rebase(ctx, where.Path, where.Base); err != nil {
		var ce *worktree.ConflictError
		if errors.As(err, &ce) {
			return e.errorf("rebase of %s onto %s conflicts in %s; the rebase was aborted and the branch is as it was: resolve it by hand",
				change, where.Base, strings.Join(ce.Files, ", "))
		}
		return e.errorf("rebase %s onto %s: %v", change, where.Base, err)
	}
	after, err := e.Worktree.Head(ctx, where.Path, ref)
	if err != nil {
		return e.errorf("%v", err)
	}
	return e.printJSON(syncResult{Change: change, Base: where.Base, Pulled: pulled, Rebased: before != after, Head: after})
}
