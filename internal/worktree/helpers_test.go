package worktree

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// repo is a temporary main checkout on main, with a bare origin as its
// upstream and a test-controlled hooks directory.
type repo struct {
	Main   string // main checkout
	Origin string // bare remote
	Hooks  string // core.hooksPath: empty until a test installs a hook
}

// newRepo builds a repository with one commit pushed to a bare origin.
// Git's global and system config are ignored, so the machine's identity,
// signing and hook settings cannot leak into the tests.
func newRepo(t *testing.T) *repo {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_TERMINAL_PROMPT", "0")
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	r := &repo{
		Main:   filepath.Join(base, "main"),
		Origin: filepath.Join(base, "origin.git"),
		Hooks:  filepath.Join(base, "hooks"),
	}
	for _, d := range []string{r.Main, r.Origin, r.Hooks} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	git(t, r.Origin, "init", "-q", "--bare", "-b", "main")
	git(t, r.Main, "init", "-q", "-b", "main")
	git(t, r.Main, "config", "user.name", "t")
	git(t, r.Main, "config", "user.email", "t@t")
	git(t, r.Main, "config", "commit.gpgsign", "false")
	git(t, r.Main, "config", "core.hooksPath", r.Hooks)
	write(t, r.Main, "README.md", "readme\n")
	write(t, r.Main, "main.go", "package main\n")
	git(t, r.Main, "add", "-A")
	git(t, r.Main, "commit", "-qm", "init")
	git(t, r.Main, "remote", "add", "origin", r.Origin)
	git(t, r.Main, "push", "-q", "-u", "origin", "main")
	return r
}

// clone makes a second checkout of origin, to push commits upstream from.
func (r *repo) clone(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(filepath.Dir(r.Main), "other")
	git(t, filepath.Dir(r.Main), "clone", "-q", r.Origin, dir)
	git(t, dir, "config", "user.name", "o")
	git(t, dir, "config", "user.email", "o@o")
	git(t, dir, "config", "commit.gpgsign", "false")
	git(t, dir, "config", "core.hooksPath", r.Hooks)
	return dir
}

// pushUpstream commits file=content on origin's main through the second checkout.
func (r *repo) pushUpstream(t *testing.T, file, content string) {
	t.Helper()
	dir := r.clone(t)
	write(t, dir, file, content)
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-qm", "upstream "+file)
	git(t, dir, "push", "-q", "origin", "main")
}

// hook installs an executable hook script into the test hooks directory.
func (r *repo) hook(t *testing.T, name, script string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(r.Hooks, name), []byte("#!/bin/sh\n"+script+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

func write(t *testing.T, dir, name, content string) {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// commit writes a file and commits it in dir.
func commit(t *testing.T, dir, file, content, msg string) {
	t.Helper()
	write(t, dir, file, content)
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-qm", msg)
}
