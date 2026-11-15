package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"

	"github.com/shurastik/agterm-kit/internal/agterm"
	"github.com/shurastik/agterm-kit/internal/rundir"
)

// finishResult is the stdout of `agk finish`.
type finishResult struct {
	Change string   `json:"change"`
	Base   string   `json:"base"`
	Head   string   `json:"head"`
	Closed []string `json:"closed"`
}

// Finish implements `agk finish <change>`: fast-forward the base branch to
// the finished change branch, close the companions of the worktree's runs,
// remove the worktree and delete the branch. It checks everything before it
// changes anything, never pulls or rebases, and a rerun after any failed step
// completes what is left.
func (e *Env) Finish(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("agk finish", flag.ContinueOnError)
	fs.SetOutput(e.Stderr)
	pos, code, ok := parseInterleaved(fs, args)
	if !ok {
		return code
	}
	if len(pos) != 1 {
		return e.errorf("usage: agk finish <change>")
	}
	change := pos[0]
	where, err := e.locateChange(ctx, change, true)
	if err != nil {
		return e.errorf("%v", err)
	}
	if where.Present {
		if err := e.requireClean(ctx, where.Path); err != nil {
			return e.errorf("%v", err)
		}
	}
	archived, err := e.Worktree.ArchivedOnBranch(ctx, where.Main, change)
	if err != nil {
		return e.errorf("%v", err)
	}
	if !archived {
		return e.errorf("change %s is not archived on branch %s: archive it and commit the archive first", change, change)
	}
	if !where.Merged {
		synced, err := e.Worktree.IsAncestor(ctx, where.Main, where.Base, change)
		if err != nil {
			return e.errorf("%v", err)
		}
		if !synced {
			return e.errorf("%s has commits that branch %s lacks: run `agk sync %s`, check the result again, then finish", where.Base, change, change)
		}
		if err := e.Worktree.MergeFFOnly(ctx, where.Main, change); err != nil {
			return e.errorf("fast-forward %s to %s: %v", where.Base, change, err)
		}
	}
	head, err := e.Worktree.Head(ctx, where.Main, where.Base)
	if err != nil {
		return e.errorf("%s is merged but its tip is unreadable: %v", where.Base, err)
	}
	closed := []string{}
	if where.Present {
		if closed, err = e.closeWorktreeRuns(ctx, where.Path); err != nil {
			return e.errorf("%s is merged into %s, but %v; finish it again after fixing that (the worktree %s and branch %s are left)",
				change, where.Base, err, where.Path, change)
		}
		if err := e.Worktree.Remove(ctx, where.Main, where.Path); err != nil {
			return e.errorf("%s is merged into %s, but removing the worktree %s failed: %v; branch %s is left, finish again after fixing that",
				change, where.Base, where.Path, err, change)
		}
	}
	if err := e.Worktree.DeleteBranch(ctx, where.Main, change); err != nil {
		return e.errorf("%s is merged into %s and the worktree is gone, but deleting branch %s failed: %v; finish again after fixing that",
			change, where.Base, change, err)
	}
	return e.printJSON(finishResult{Change: change, Base: where.Base, Head: head, Closed: closed})
}

var runDirName = regexp.MustCompile(`^\d{8}-\d{6}(-\d+)?$`)

// closeWorktreeRuns closes the companion of every apply and review run under
// the worktree's .agk/ (a companion already gone counts as closed) and returns
// the run directories it closed. Runs that have no companion (inline runs)
// are skipped.
func (e *Env) closeWorktreeRuns(ctx context.Context, worktreePath string) ([]string, error) {
	closed := []string{}
	for _, kind := range []string{rundir.Applies, rundir.Reviews} {
		parent := filepath.Join(worktreePath, rundir.Dir, kind)
		entries, err := os.ReadDir(parent)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return closed, fmt.Errorf("read %s: %v", parent, err)
		}
		var names []string
		for _, ent := range entries {
			if ent.IsDir() && runDirName.MatchString(ent.Name()) {
				names = append(names, ent.Name())
			}
		}
		sort.Strings(names)
		for _, name := range names {
			dir := filepath.Join(parent, name)
			id, err := readCompanion(dir)
			if err != nil {
				return closed, fmt.Errorf("run %s: %v", dir, err)
			}
			if id == "" {
				continue
			}
			if err := e.Ctl.Close(ctx, id); err != nil && !errors.Is(err, agterm.ErrGone) {
				return closed, fmt.Errorf("closing the companion %s of run %s failed: %v", id, dir, err)
			}
			closed = append(closed, dir)
		}
	}
	return closed, nil
}
