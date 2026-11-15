package worktree

import (
	"context"
	"os"
	"path/filepath"
	"strings"
)

// PullFFOnly updates the branch checked out in dir with `git pull --ff-only`.
// Git's message is in the error when it fails; nothing changes then.
func (Git) PullFFOnly(ctx context.Context, dir string) error {
	_, err := run(ctx, dir, "pull", "--ff-only")
	return err
}

// CommitPaths commits the uncommitted content of paths in root with message,
// and nothing else: other staged entries stay staged and are not committed
// (a pathspec commit implies --only). It reports false, with no commit, when
// the paths have nothing to commit. Hooks run; a failing hook's output is in
// the error.
func (Git) CommitPaths(ctx context.Context, root, message string, paths ...string) (bool, error) {
	status, err := run(ctx, root, append([]string{"status", "--porcelain", "--"}, paths...)...)
	if err != nil {
		return false, err
	}
	if strings.TrimSpace(status) == "" {
		return false, nil
	}
	if _, err := run(ctx, root, append([]string{"add", "--"}, paths...)...); err != nil {
		return false, err
	}
	if _, err := run(ctx, root, append([]string{"commit", "-m", message, "--"}, paths...)...); err != nil {
		return false, err
	}
	return true, nil
}

// excludeEntry hides the worktrees directory in the main checkout.
const excludeEntry = "/.worktrees/"

// Add creates the worktree <main>/.worktrees/<change> on a new branch <change>
// starting at base, records base as branch.<change>.agkBase, and hides
// .worktrees/ through the repository's info/exclude unless it is ignored
// already. It returns the worktree path.
func (g Git) Add(ctx context.Context, main, change, base string) (string, error) {
	if err := ensureExcluded(ctx, main); err != nil {
		return "", err
	}
	path := WorktreePath(main, change)
	// The base is recorded first: `git worktree add` can fail after it created
	// the branch and the worktree (a failing post-checkout hook), and a later
	// start that reuses them must find the original base.
	if err := g.SetAgkBase(ctx, main, change, base); err != nil {
		return "", err
	}
	if _, err := run(ctx, main, "worktree", "add", "-b", change, path, base); err != nil {
		if made, berr := g.BranchExists(ctx, main, change); berr == nil && !made {
			g.unsetAgkBase(ctx, main, change)
		}
		return "", err
	}
	return path, nil
}

func ensureExcluded(ctx context.Context, main string) error {
	if _, err := run(ctx, main, "check-ignore", "-q", ".worktrees/"); err == nil {
		return nil
	} else if exitCode(err) != 1 {
		return err
	}
	common, err := trim(run(ctx, main, "rev-parse", "--git-common-dir"))
	if err != nil {
		return err
	}
	if !filepath.IsAbs(common) {
		common = filepath.Join(main, common)
	}
	file := filepath.Join(common, "info", "exclude")
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return err
	}
	existing, err := os.ReadFile(file)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	text := string(existing)
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	return os.WriteFile(file, []byte(text+excludeEntry+"\n"), 0o644)
}

// agkBaseKey is the git config key recording the base branch of a change branch.
func agkBaseKey(branch string) string { return "branch." + branch + ".agkBase" }

// AgkBase reads branch.<branch>.agkBase; false when it is not set.
func (Git) AgkBase(ctx context.Context, dir, branch string) (string, bool, error) {
	v, err := trim(run(ctx, dir, "config", "--get", agkBaseKey(branch)))
	if err != nil {
		if exitCode(err) == 1 {
			return "", false, nil
		}
		return "", false, err
	}
	return v, v != "", nil
}

// unsetAgkBase removes branch.<branch>.agkBase; a key that is not there is fine.
func (Git) unsetAgkBase(ctx context.Context, dir, branch string) {
	run(ctx, dir, "config", "--unset", agkBaseKey(branch))
}

// SetAgkBase records base as branch.<branch>.agkBase.
func (Git) SetAgkBase(ctx context.Context, dir, branch, base string) error {
	_, err := run(ctx, dir, "config", agkBaseKey(branch), base)
	return err
}
