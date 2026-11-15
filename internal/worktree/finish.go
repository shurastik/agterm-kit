package worktree

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Status lists the uncommitted and untracked files of the worktree at dir,
// as `git status --porcelain` sees them: ignored files (such as .agk/) are
// left out.
func (Git) Status(ctx context.Context, dir string) ([]string, error) {
	out, err := run(ctx, dir, "status", "--porcelain")
	if err != nil {
		return nil, err
	}
	var files []string
	for _, line := range strings.Split(out, "\n") {
		if len(line) > 3 {
			files = append(files, line[3:])
		}
	}
	return files, nil
}

// archiveDir matches the directory name `openspec archive` gives a change.
func archiveDir(change string) *regexp.Regexp {
	return regexp.MustCompile(`^\d{4}-\d{2}-\d{2}-` + regexp.QuoteMeta(change) + `$`)
}

// ArchivedOnBranch reports whether the committed tree of branch <change> has
// archived the change: openspec/changes/<change>/ is gone and
// openspec/changes/archive/<date>-<change>/ exists. It reads the branch's
// tree, so it needs no worktree, and sees only what is committed.
func (Git) ArchivedOnBranch(ctx context.Context, dir, change string) (bool, error) {
	ref := "refs/heads/" + change
	live, err := lsTree(ctx, dir, ref, "openspec/changes/")
	if err != nil {
		return false, err
	}
	for _, name := range live {
		if name == change {
			return false, nil
		}
	}
	archived, err := lsTree(ctx, dir, ref, "openspec/changes/archive/")
	if err != nil {
		return false, err
	}
	re := archiveDir(change)
	for _, name := range archived {
		if re.MatchString(name) {
			return true, nil
		}
	}
	return false, nil
}

// lsTree lists the entry names directly under the directory path (with a
// trailing slash) in ref's tree; a path the tree lacks yields nothing.
func lsTree(ctx context.Context, dir, ref, path string) ([]string, error) {
	out, err := run(ctx, dir, "ls-tree", "--name-only", ref, path)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line != "" {
			names = append(names, strings.TrimPrefix(line, path))
		}
	}
	return names, nil
}

// IsAncestor reports whether ancestor is contained in descendant's history.
func (Git) IsAncestor(ctx context.Context, dir, ancestor, descendant string) (bool, error) {
	_, err := run(ctx, dir, "merge-base", "--is-ancestor", ancestor, descendant)
	if err == nil {
		return true, nil
	}
	if exitCode(err) == 1 {
		return false, nil
	}
	return false, err
}

// Head resolves ref to a commit id.
func (Git) Head(ctx context.Context, dir, ref string) (string, error) {
	return trim(run(ctx, dir, "rev-parse", "--verify", ref+"^{commit}"))
}

// ConflictError is a rebase that hit conflicts. The rebase was aborted and
// the branch is as it was.
type ConflictError struct {
	Files []string
}

func (e *ConflictError) Error() string {
	return "rebase conflict in " + strings.Join(e.Files, ", ")
}

// cleanupTimeout bounds the work that restores a branch after an interrupted rebase.
const cleanupTimeout = 30 * time.Second

// Rebase rebases the branch checked out in the worktree at dir onto onto. On
// a conflict it aborts the rebase, leaves the branch where it was and returns
// a *ConflictError naming the conflicting files. It aborts a rebase that the
// cancelled ctx interrupted as well.
func (Git) Rebase(ctx context.Context, dir, onto string) error {
	_, err := run(ctx, dir, "rebase", onto)
	if err == nil {
		return nil
	}
	// The command's context may be the cancelled one (an interrupted sync): the
	// listing and the abort must still run, or the worktree stays mid-rebase.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cleanupTimeout)
	defer cancel()
	out, _ := run(ctx, dir, "diff", "--name-only", "--diff-filter=U")
	var files []string
	for _, f := range strings.Split(strings.TrimSpace(out), "\n") {
		if f != "" {
			files = append(files, f)
		}
	}
	// Restore the branch whatever went wrong; there is nothing to abort when
	// git refused to start.
	_, abortErr := run(ctx, dir, "rebase", "--abort")
	if len(files) > 0 {
		if abortErr != nil {
			return fmt.Errorf("rebase conflict in %s, and aborting it failed: %v", strings.Join(files, ", "), abortErr)
		}
		return &ConflictError{Files: files}
	}
	return err
}

// MergeFFOnly fast-forwards the branch checked out in dir to branch. A
// failure leaves everything as it was.
func (Git) MergeFFOnly(ctx context.Context, dir, branch string) error {
	_, err := run(ctx, dir, "merge", "--ff-only", "refs/heads/"+branch)
	return err
}

// Remove removes the linked worktree at path without forcing: untracked
// files block it, ignored ones do not.
func (Git) Remove(ctx context.Context, dir, path string) error {
	_, err := run(ctx, dir, "worktree", "remove", path)
	return err
}

// DeleteBranch deletes the branch with `git branch -d`, which refuses a
// branch the checked-out one does not contain. Git drops the branch's config
// section (agkBase) with it.
func (Git) DeleteBranch(ctx context.Context, dir, branch string) error {
	_, err := run(ctx, dir, "branch", "-d", branch)
	return err
}
