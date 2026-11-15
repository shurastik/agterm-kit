package worktree

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPullFFOnlyFastForwards(t *testing.T) {
	r := newRepo(t)
	r.pushUpstream(t, "new.txt", "n\n")
	before := git(t, r.Main, "rev-parse", "HEAD")
	if err := (Git{}).PullFFOnly(ctx, r.Main); err != nil {
		t.Fatal(err)
	}
	after := git(t, r.Main, "rev-parse", "HEAD")
	if before == after || after != git(t, r.Main, "rev-parse", "origin/main") {
		t.Fatalf("HEAD %s -> %s, origin/main %s", before, after, git(t, r.Main, "rev-parse", "origin/main"))
	}
}

func TestPullFFOnlyFailsOnDivergedBase(t *testing.T) {
	r := newRepo(t)
	r.pushUpstream(t, "up.txt", "u\n")
	commit(t, r.Main, "local.txt", "l\n", "local")
	before := git(t, r.Main, "rev-parse", "HEAD")
	err := (Git{}).PullFFOnly(ctx, r.Main)
	if err == nil {
		t.Fatal("diverged base must fail")
	}
	if !strings.Contains(err.Error(), "fast-forward") {
		t.Fatalf("error %q does not carry git's message", err)
	}
	if git(t, r.Main, "rev-parse", "HEAD") != before {
		t.Fatal("HEAD moved on a failed pull")
	}
}

func TestPullFFOnlyFailsOnBlockingLocalEdit(t *testing.T) {
	r := newRepo(t)
	r.pushUpstream(t, "README.md", "upstream readme\n")
	write(t, r.Main, "README.md", "local edit\n")
	before := git(t, r.Main, "rev-parse", "HEAD")
	if err := (Git{}).PullFFOnly(ctx, r.Main); err == nil {
		t.Fatal("a local edit to a file the pull changes must fail")
	}
	if git(t, r.Main, "rev-parse", "HEAD") != before {
		t.Fatal("HEAD moved on a failed pull")
	}
}

const planDir = "openspec/changes/c"

func planFiles(t *testing.T, r *repo) {
	t.Helper()
	write(t, r.Main, planDir+"/proposal.md", "p\n")
	write(t, r.Main, planDir+"/specs/x/spec.md", "s\n")
}

func TestCommitPathsOnlyTheChange(t *testing.T) {
	r := newRepo(t)
	planFiles(t, r)
	write(t, r.Main, "README.md", "modified\n")
	write(t, r.Main, "main.go", "package main\n// staged\n")
	git(t, r.Main, "add", "main.go")
	commitsBefore := git(t, r.Main, "rev-list", "--count", "HEAD")

	done, err := (Git{}).CommitPaths(ctx, r.Main, "docs(openspec): plan c", planDir)
	if err != nil || !done {
		t.Fatalf("CommitPaths = %v, %v", done, err)
	}
	if git(t, r.Main, "log", "-1", "--format=%s") != "docs(openspec): plan c" {
		t.Fatalf("message = %q", git(t, r.Main, "log", "-1", "--format=%s"))
	}
	if n := git(t, r.Main, "rev-list", "--count", "HEAD"); n == commitsBefore {
		t.Fatal("no commit made")
	}
	files := strings.Fields(git(t, r.Main, "show", "--name-only", "--format=", "HEAD"))
	if strings.Join(files, ",") != planDir+"/proposal.md,"+planDir+"/specs/x/spec.md" {
		t.Fatalf("commit holds %v", files)
	}
	status := git(t, r.Main, "status", "--porcelain")
	if !strings.Contains(status, "M README.md") || !strings.Contains(status, "M  main.go") {
		t.Fatalf("other changes were not left alone:\n%s", status)
	}
}

func TestCommitPathsNothingToCommit(t *testing.T) {
	r := newRepo(t)
	planFiles(t, r)
	git(t, r.Main, "add", "-A")
	git(t, r.Main, "commit", "-qm", "plan")
	head := git(t, r.Main, "rev-parse", "HEAD")
	done, err := (Git{}).CommitPaths(ctx, r.Main, "docs(openspec): plan c", planDir)
	if err != nil || done {
		t.Fatalf("CommitPaths = %v, %v; want false, nil", done, err)
	}
	if git(t, r.Main, "rev-parse", "HEAD") != head {
		t.Fatal("a commit was made")
	}
}

func TestCommitPathsReturnsHookOutput(t *testing.T) {
	r := newRepo(t)
	planFiles(t, r)
	r.hook(t, "pre-commit", "echo 'lint failed: nope' >&2\nexit 1")
	head := git(t, r.Main, "rev-parse", "HEAD")
	done, err := (Git{}).CommitPaths(ctx, r.Main, "docs(openspec): plan c", planDir)
	if err == nil || done {
		t.Fatalf("CommitPaths = %v, %v; want a hook failure", done, err)
	}
	if !strings.Contains(err.Error(), "lint failed: nope") {
		t.Fatalf("error %q lacks the hook's output", err)
	}
	if git(t, r.Main, "rev-parse", "HEAD") != head {
		t.Fatal("a commit was made despite the failing hook")
	}
}

func TestAddCreatesWorktree(t *testing.T) {
	r := newRepo(t)
	path, err := (Git{}).Add(ctx, r.Main, "c", "main")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(r.Main, ".worktrees", "c")
	if path != want {
		t.Fatalf("path = %q, want %q", path, want)
	}
	if got := git(t, path, "rev-parse", "--abbrev-ref", "HEAD"); got != "c" {
		t.Fatalf("worktree branch = %q", got)
	}
	if git(t, path, "rev-parse", "HEAD") != git(t, r.Main, "rev-parse", "main") {
		t.Fatal("branch c does not start at the base tip")
	}
	if got := git(t, r.Main, "config", "--get", "branch.c.agkBase"); got != "main" {
		t.Fatalf("agkBase = %q", got)
	}
	if st := git(t, r.Main, "status", "--porcelain"); strings.Contains(st, ".worktrees") {
		t.Fatalf("main checkout lists .worktrees:\n%s", st)
	}
	exclude, err := os.ReadFile(filepath.Join(r.Main, ".git", "info", "exclude"))
	if err != nil || !strings.Contains(string(exclude), "/.worktrees/") {
		t.Fatalf("exclude = %q, %v", exclude, err)
	}
}

func TestAddLeavesExcludeAloneWhenIgnored(t *testing.T) {
	r := newRepo(t)
	write(t, r.Main, ".gitignore", ".worktrees/\n")
	git(t, r.Main, "add", ".gitignore")
	git(t, r.Main, "commit", "-qm", "ignore worktrees")
	excludeFile := filepath.Join(r.Main, ".git", "info", "exclude")
	before, _ := os.ReadFile(excludeFile)
	if _, err := (Git{}).Add(ctx, r.Main, "c", "main"); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(excludeFile)
	if string(before) != string(after) {
		t.Fatalf("exclude changed although .worktrees/ was already ignored:\n%s", after)
	}
}

func TestAddKeepsExcludeEntryOnce(t *testing.T) {
	r := newRepo(t)
	g := Git{}
	for _, c := range []string{"c", "d"} {
		if _, err := g.Add(ctx, r.Main, c, "main"); err != nil {
			t.Fatal(err)
		}
	}
	exclude, _ := os.ReadFile(filepath.Join(r.Main, ".git", "info", "exclude"))
	if n := strings.Count(string(exclude), "/.worktrees/"); n != 1 {
		t.Fatalf("exclude holds the entry %d times:\n%s", n, exclude)
	}
}

func TestAgkBase(t *testing.T) {
	r := newRepo(t)
	g := Git{}
	if b, ok, err := g.AgkBase(ctx, r.Main, "c"); err != nil || ok || b != "" {
		t.Fatalf("AgkBase unset = %q, %v, %v", b, ok, err)
	}
	if _, err := g.Add(ctx, r.Main, "c", "main"); err != nil {
		t.Fatal(err)
	}
	if b, ok, err := g.AgkBase(ctx, r.Main, "c"); err != nil || !ok || b != "main" {
		t.Fatalf("AgkBase = %q, %v, %v", b, ok, err)
	}
}

// A post-checkout hook that fails after `git worktree add` created the branch
// and the worktree: the base recorded before the add stays.
func TestAddKeepsTheBaseWhenAHookFailsAfterTheBranchExists(t *testing.T) {
	r := newRepo(t)
	r.hook(t, "post-checkout", "echo hook says no >&2\nexit 1")
	if _, err := (Git{}).Add(ctx, r.Main, "c", "main"); err == nil {
		t.Fatal("the failing hook must fail the add")
	}
	if ok, _ := (Git{}).BranchExists(ctx, r.Main, "c"); !ok {
		t.Fatal("the scenario does not hold: the failed add left no branch")
	}
	if got := git(t, r.Main, "config", "--get", "branch.c.agkBase"); got != "main" {
		t.Fatalf("agkBase = %q, want main", got)
	}
}

// An add that fails before it creates the branch leaves no agkBase behind.
func TestAddRemovesTheBaseWhenNoBranchWasCreated(t *testing.T) {
	r := newRepo(t)
	if _, err := (Git{}).Add(ctx, r.Main, "c", "no-such-base"); err == nil {
		t.Fatal("an unknown base must fail the add")
	}
	if ok, _ := (Git{}).BranchExists(ctx, r.Main, "c"); ok {
		t.Fatal("the scenario does not hold: the failed add created the branch")
	}
	if b, ok, err := (Git{}).AgkBase(ctx, r.Main, "c"); err != nil || ok {
		t.Fatalf("agkBase = %q, %v, %v; want it removed", b, ok, err)
	}
}
