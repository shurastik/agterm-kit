package worktree

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// changeWorktree creates worktree .worktrees/c on branch c through Add.
func changeWorktree(t *testing.T, r *repo) string {
	t.Helper()
	path, err := (Git{}).Add(ctx, r.Main, "c", "main")
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func TestStatus(t *testing.T) {
	r := newRepo(t)
	wt := changeWorktree(t, r)
	g := Git{}
	if st, err := g.Status(ctx, wt); err != nil || len(st) != 0 {
		t.Fatalf("clean Status = %v, %v", st, err)
	}
	write(t, wt, "README.md", "changed\n")
	write(t, wt, "new.go", "package main\n")
	write(t, wt, ".agk/applies/1/worker.json", "{}") // ignored through the repo's exclude
	write(t, r.Main, ".git/info/exclude", "/.agk/\n")
	st, err := g.Status(ctx, wt)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(st, ",") != "README.md,new.go" {
		t.Fatalf("Status = %v; want modified and untracked files, not .agk/", st)
	}
}

func TestArchivedOnBranch(t *testing.T) {
	r := newRepo(t)
	wt := changeWorktree(t, r)
	g := Git{}
	commit(t, wt, "openspec/changes/c/proposal.md", "p\n", "plan")
	if ok, err := g.ArchivedOnBranch(ctx, r.Main, "c"); err != nil || ok {
		t.Fatalf("plan still in place: ArchivedOnBranch = %v, %v", ok, err)
	}

	// moved in the working tree but not committed yet
	git(t, wt, "mv", "openspec/changes/c", "openspec/changes/archive-tmp")
	if err := os.MkdirAll(filepath.Join(wt, "openspec", "changes", "archive"), 0o755); err != nil {
		t.Fatal(err)
	}
	git(t, wt, "mv", "openspec/changes/archive-tmp", "openspec/changes/archive/2026-01-01-c")
	if ok, err := g.ArchivedOnBranch(ctx, r.Main, "c"); err != nil || ok {
		t.Fatalf("archive not committed: ArchivedOnBranch = %v, %v", ok, err)
	}
	git(t, wt, "commit", "-qm", "archive")
	if ok, err := g.ArchivedOnBranch(ctx, r.Main, "c"); err != nil || !ok {
		t.Fatalf("archive committed: ArchivedOnBranch = %v, %v", ok, err)
	}
}

func TestArchivedOnBranchWithoutAnyChange(t *testing.T) {
	r := newRepo(t)
	changeWorktree(t, r)
	// no openspec/ at all on the branch: not archived
	if ok, err := (Git{}).ArchivedOnBranch(ctx, r.Main, "c"); err != nil || ok {
		t.Fatalf("ArchivedOnBranch = %v, %v", ok, err)
	}
}

func TestArchivedOnBranchMatchesTheExactName(t *testing.T) {
	r := newRepo(t)
	wt := changeWorktree(t, r)
	commit(t, wt, "openspec/changes/archive/2026-01-01-other-c/proposal.md", "p\n", "archive of another change")
	if ok, err := (Git{}).ArchivedOnBranch(ctx, r.Main, "c"); err != nil || ok {
		t.Fatalf("archive of change other-c counted for c: %v, %v", ok, err)
	}
}

func TestIsAncestor(t *testing.T) {
	r := newRepo(t)
	wt := changeWorktree(t, r)
	g := Git{}
	commit(t, wt, "a.txt", "a\n", "a")
	if ok, err := g.IsAncestor(ctx, r.Main, "main", "c"); err != nil || !ok {
		t.Fatalf("main is an ancestor of c: %v, %v", ok, err)
	}
	if ok, err := g.IsAncestor(ctx, r.Main, "c", "main"); err != nil || ok {
		t.Fatalf("c is not an ancestor of main: %v, %v", ok, err)
	}
	if _, err := g.IsAncestor(ctx, r.Main, "nope", "main"); err == nil {
		t.Fatal("an unknown ref must be an error, not false")
	}
}

func TestHead(t *testing.T) {
	r := newRepo(t)
	h, err := (Git{}).Head(ctx, r.Main, "main")
	if err != nil || h != git(t, r.Main, "rev-parse", "main") {
		t.Fatalf("Head = %q, %v", h, err)
	}
}

func TestRebaseReplaysOntoMovedBase(t *testing.T) {
	r := newRepo(t)
	wt := changeWorktree(t, r)
	g := Git{}
	commit(t, wt, "feature.txt", "f\n", "feature")
	commit(t, r.Main, "other.txt", "o\n", "other plan")
	before := git(t, wt, "rev-parse", "HEAD")

	if err := g.Rebase(ctx, wt, "main"); err != nil {
		t.Fatal(err)
	}
	if git(t, wt, "rev-parse", "HEAD") == before {
		t.Fatal("tip unchanged after a rebase onto a moved base")
	}
	if ok, _ := g.IsAncestor(ctx, wt, "main", "c"); !ok {
		t.Fatal("c does not contain main after the rebase")
	}
	if git(t, wt, "log", "-1", "--format=%s") != "feature" {
		t.Fatal("c's commit was not replayed on top")
	}
}

func TestRebaseUpToDateKeepsTip(t *testing.T) {
	r := newRepo(t)
	wt := changeWorktree(t, r)
	commit(t, wt, "feature.txt", "f\n", "feature")
	before := git(t, wt, "rev-parse", "HEAD")
	if err := (Git{}).Rebase(ctx, wt, "main"); err != nil {
		t.Fatal(err)
	}
	if git(t, wt, "rev-parse", "HEAD") != before {
		t.Fatal("tip moved although the branch already contains main")
	}
}

func TestRebaseConflictAborts(t *testing.T) {
	r := newRepo(t)
	wt := changeWorktree(t, r)
	commit(t, wt, "README.md", "from the change\n", "change edits readme")
	commit(t, r.Main, "README.md", "from the base\n", "base edits readme")
	before := git(t, wt, "rev-parse", "HEAD")

	err := (Git{}).Rebase(ctx, wt, "main")
	var ce *ConflictError
	if !errors.As(err, &ce) {
		t.Fatalf("err = %v, want a ConflictError", err)
	}
	if strings.Join(ce.Files, ",") != "README.md" {
		t.Fatalf("conflicting files = %v", ce.Files)
	}
	if !strings.Contains(err.Error(), "README.md") {
		t.Fatalf("error %q does not name the file", err)
	}
	if git(t, wt, "rev-parse", "HEAD") != before {
		t.Fatal("branch tip moved")
	}
	if st := git(t, wt, "status", "--porcelain"); st != "" {
		t.Fatalf("worktree not restored:\n%s", st)
	}
	if _, err := os.Stat(filepath.Join(r.Main, ".git", "worktrees", "c", "rebase-merge")); err == nil {
		t.Fatal("rebase still in progress")
	}
}

func TestMergeFFOnly(t *testing.T) {
	r := newRepo(t)
	wt := changeWorktree(t, r)
	commit(t, wt, "feature.txt", "f\n", "feature")
	tip := git(t, wt, "rev-parse", "HEAD")
	if err := (Git{}).MergeFFOnly(ctx, r.Main, "c"); err != nil {
		t.Fatal(err)
	}
	if git(t, r.Main, "rev-parse", "main") != tip {
		t.Fatal("main is not at the tip of c")
	}
	if n := git(t, r.Main, "rev-list", "--merges", "--count", "main"); n != "0" {
		t.Fatalf("%s merge commits", n)
	}
}

func TestMergeFFOnlyBlockedByLocalEdit(t *testing.T) {
	r := newRepo(t)
	wt := changeWorktree(t, r)
	commit(t, wt, "README.md", "changed by the branch\n", "branch edits readme")
	write(t, r.Main, "README.md", "uncommitted edit\n")
	before := git(t, r.Main, "rev-parse", "main")
	if err := (Git{}).MergeFFOnly(ctx, r.Main, "c"); err == nil {
		t.Fatal("must fail when the main checkout edits a file the branch changes")
	}
	if git(t, r.Main, "rev-parse", "main") != before {
		t.Fatal("main moved on a failed merge")
	}
	if b, _ := os.ReadFile(filepath.Join(r.Main, "README.md")); string(b) != "uncommitted edit\n" {
		t.Fatal("the local edit was lost")
	}
}

func TestMergeFFOnlyRefusesDivergedBranch(t *testing.T) {
	r := newRepo(t)
	wt := changeWorktree(t, r)
	commit(t, wt, "feature.txt", "f\n", "feature")
	commit(t, r.Main, "other.txt", "o\n", "other")
	before := git(t, r.Main, "rev-parse", "main")
	if err := (Git{}).MergeFFOnly(ctx, r.Main, "c"); err == nil {
		t.Fatal("a branch that lacks the base must not merge")
	}
	if git(t, r.Main, "rev-parse", "main") != before {
		t.Fatal("main moved")
	}
}

func TestRemove(t *testing.T) {
	r := newRepo(t)
	wt := changeWorktree(t, r)
	write(t, r.Main, ".git/info/exclude", "/.agk/\n/.worktrees/\n")
	write(t, wt, ".agk/applies/1/worker.json", "{}")
	if err := (Git{}).Remove(ctx, r.Main, wt); err != nil {
		t.Fatalf("a worktree with only ignored files must be removable: %v", err)
	}
	if _, err := os.Stat(wt); !os.IsNotExist(err) {
		t.Fatalf("worktree still there: %v", err)
	}
}

func TestRemoveRefusesUntrackedFile(t *testing.T) {
	r := newRepo(t)
	wt := changeWorktree(t, r)
	write(t, wt, "untracked.txt", "u\n")
	if err := (Git{}).Remove(ctx, r.Main, wt); err == nil {
		t.Fatal("untracked files must block the removal")
	}
	if _, err := os.Stat(filepath.Join(wt, "untracked.txt")); err != nil {
		t.Fatal("the untracked file was destroyed")
	}
}

func TestDeleteBranch(t *testing.T) {
	r := newRepo(t)
	wt := changeWorktree(t, r)
	g := Git{}
	commit(t, wt, "feature.txt", "f\n", "feature")
	if err := g.MergeFFOnly(ctx, r.Main, "c"); err != nil {
		t.Fatal(err)
	}
	if err := g.Remove(ctx, r.Main, wt); err != nil {
		t.Fatal(err)
	}
	if err := g.DeleteBranch(ctx, r.Main, "c"); err != nil {
		t.Fatal(err)
	}
	if ok, _ := g.BranchExists(ctx, r.Main, "c"); ok {
		t.Fatal("branch c still exists")
	}
	if _, ok, _ := g.AgkBase(ctx, r.Main, "c"); ok {
		t.Fatal("agkBase survived the branch")
	}
}

func TestDeleteBranchRefusesUnmerged(t *testing.T) {
	r := newRepo(t)
	wt := changeWorktree(t, r)
	commit(t, wt, "feature.txt", "f\n", "feature")
	if err := (Git{}).Remove(ctx, r.Main, wt); err != nil {
		t.Fatal(err)
	}
	if err := (Git{}).DeleteBranch(ctx, r.Main, "c"); err == nil {
		t.Fatal("an unmerged branch must not be deleted by -d")
	}
	if ok, _ := (Git{}).BranchExists(ctx, r.Main, "c"); !ok {
		t.Fatal("branch c was deleted")
	}
}

// A sync interrupted during the rebase (the command's context cancelled) must
// still abort it: the branch goes back to its old tip, no rebase is left.
func TestRebaseCancelledStillAborts(t *testing.T) {
	r := newRepo(t)
	wt := changeWorktree(t, r)
	commit(t, wt, "feature.txt", "f\n", "feature")
	commit(t, r.Main, "other.txt", "o\n", "other plan")
	before := git(t, wt, "rev-parse", "HEAD")

	// post-checkout runs while the rebase is under way: the first call tells the
	// test and lingers, so the context is cancelled in the middle of it.
	mark := filepath.Join(filepath.Dir(r.Main), "in-rebase")
	r.hook(t, "post-checkout", "if [ ! -e '"+mark+"' ]; then touch '"+mark+"'; sleep 2; fi")

	cctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		for i := 0; i < 400; i++ {
			if _, err := os.Stat(mark); err == nil {
				cancel()
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		cancel()
	}()
	err := (Git{}).Rebase(cctx, wt, "main")
	if err == nil {
		t.Fatal("a cancelled rebase must fail")
	}
	if _, serr := os.Stat(mark); serr != nil {
		t.Fatal("the rebase never reached the hook: the scenario does not hold")
	}
	if git(t, wt, "rev-parse", "HEAD") != before {
		t.Fatalf("branch tip moved: %s", git(t, wt, "log", "--oneline", "-3"))
	}
	if b := git(t, wt, "rev-parse", "--abbrev-ref", "HEAD"); b != "c" {
		t.Fatalf("worktree HEAD = %q, want branch c (a rebase is still in progress)", b)
	}
	gitDir := git(t, wt, "rev-parse", "--git-dir")
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(wt, gitDir)
	}
	for _, d := range []string{"rebase-merge", "rebase-apply"} {
		if _, serr := os.Stat(filepath.Join(gitDir, d)); serr == nil {
			t.Fatalf("%s left behind", d)
		}
	}
}
