package worktree

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

var ctx = context.Background()

func TestLocate(t *testing.T) {
	r := newRepo(t)
	g := Git{}
	git(t, r.Main, "worktree", "add", "-q", "-b", "c", filepath.Join(r.Main, ".worktrees", "c"))
	linked := filepath.Join(r.Main, ".worktrees", "c")
	write(t, r.Main, "sub/dir/x", "x")
	write(t, linked, "deep/er/y", "y")

	for name, tc := range map[string]struct {
		dir, root string
		linked    bool
	}{
		"main checkout":     {r.Main, r.Main, false},
		"main subdirectory": {filepath.Join(r.Main, "sub", "dir"), r.Main, false},
		"linked worktree":   {linked, linked, true},
		"linked subdir":     {filepath.Join(linked, "deep", "er"), linked, true},
	} {
		t.Run(name, func(t *testing.T) {
			loc, err := g.Locate(ctx, tc.dir)
			if err != nil {
				t.Fatal(err)
			}
			if loc.Main != r.Main || loc.Root != tc.root || loc.Linked() != tc.linked {
				t.Fatalf("Locate = %+v, want main %s root %s linked %v", loc, r.Main, tc.root, tc.linked)
			}
		})
	}
	if _, err := g.Locate(ctx, t.TempDir()); err == nil {
		t.Fatal("Locate outside a repository must fail")
	}
}

func TestCurrentBranch(t *testing.T) {
	r := newRepo(t)
	g := Git{}
	b, err := g.CurrentBranch(ctx, r.Main)
	if err != nil || b != "main" {
		t.Fatalf("CurrentBranch = %q, %v", b, err)
	}
	git(t, r.Main, "checkout", "-q", "--detach")
	if _, err := g.CurrentBranch(ctx, r.Main); !errors.Is(err, ErrDetached) {
		t.Fatalf("detached HEAD: err = %v, want ErrDetached", err)
	}
}

func TestUpstream(t *testing.T) {
	r := newRepo(t)
	g := Git{}
	up, ok, err := g.Upstream(ctx, r.Main, "main")
	if err != nil || !ok || up != "origin/main" {
		t.Fatalf("Upstream(main) = %q, %v, %v", up, ok, err)
	}
	git(t, r.Main, "branch", "local")
	up, ok, err = g.Upstream(ctx, r.Main, "local")
	if err != nil || ok || up != "" {
		t.Fatalf("Upstream(local) = %q, %v, %v; want none", up, ok, err)
	}
}

func TestLookup(t *testing.T) {
	r := newRepo(t)
	g := Git{}
	wt := filepath.Join(r.Main, ".worktrees")

	e, err := g.Lookup(ctx, r.Main, "c")
	if err != nil || e.Kind != Absent || e.Path != filepath.Join(wt, "c") {
		t.Fatalf("absent: %+v, %v", e, err)
	}

	git(t, r.Main, "worktree", "add", "-q", "-b", "c", filepath.Join(wt, "c"))
	e, err = g.Lookup(ctx, r.Main, "c")
	if err != nil || e.Kind != IsWorktree || e.Branch != "c" {
		t.Fatalf("worktree of c: %+v, %v", e, err)
	}

	if err := os.MkdirAll(filepath.Join(wt, "plain"), 0o755); err != nil {
		t.Fatal(err)
	}
	e, err = g.Lookup(ctx, r.Main, "plain")
	if err != nil || e.Kind != PlainDir {
		t.Fatalf("plain dir: %+v, %v", e, err)
	}

	git(t, r.Main, "worktree", "add", "-q", "-b", "other", filepath.Join(wt, "wrong"))
	e, err = g.Lookup(ctx, r.Main, "wrong")
	if err != nil || e.Kind != OtherBranch || e.Branch != "other" {
		t.Fatalf("worktree of another branch: %+v, %v", e, err)
	}
}

func TestBranchExists(t *testing.T) {
	r := newRepo(t)
	g := Git{}
	if ok, err := g.BranchExists(ctx, r.Main, "c"); err != nil || ok {
		t.Fatalf("BranchExists(c) = %v, %v", ok, err)
	}
	git(t, r.Main, "branch", "c")
	if ok, err := g.BranchExists(ctx, r.Main, "c"); err != nil || !ok {
		t.Fatalf("BranchExists(c) after create = %v, %v", ok, err)
	}
}
