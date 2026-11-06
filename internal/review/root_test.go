package review

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestRepoRoot(t *testing.T) {
	root, _ := filepath.EvalSymlinks(t.TempDir())
	if out, err := exec.Command("git", "init", "-q", root).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, out)
	}
	sub := filepath.Join(root, "a")
	if err := exec.Command("mkdir", sub).Run(); err != nil {
		t.Fatal(err)
	}
	got, err := RepoRoot(context.Background(), sub)
	if err != nil || got != root {
		t.Fatalf("RepoRoot = %q, %v", got, err)
	}
	if _, err := RepoRoot(context.Background(), t.TempDir()); err == nil {
		t.Fatal("outside a repo must error")
	}
}
