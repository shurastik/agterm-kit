package cli

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shurastik/agterm-kit/internal/review"
	"github.com/shurastik/agterm-kit/internal/worktree"
)

func gitT(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

func writeT(t *testing.T, dir, name, content string) {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func commitT(t *testing.T, dir, msg string) {
	t.Helper()
	gitT(t, dir, "add", "-A")
	gitT(t, dir, "commit", "-qm", msg)
}

// decodeOut decodes what the command printed into v and clears stdout.
func decodeOut(t *testing.T, h *tenv, v any) {
	t.Helper()
	if err := json.Unmarshal(h.out.Bytes(), v); err != nil {
		t.Fatalf("stdout %q: %v", h.out, err)
	}
	h.out.Reset()
}

// realRepo builds a repository on main with a bare origin and an untracked plan
// of change c, and returns a harness wired to real git inside it.
func realRepo(t *testing.T) (h *tenv, main, base string) {
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_TERMINAL_PROMPT", "0")
	base, _ = filepath.EvalSymlinks(t.TempDir())
	origin, main := filepath.Join(base, "origin.git"), filepath.Join(base, "repo")
	for _, d := range []string{origin, main} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	gitT(t, origin, "init", "-q", "--bare", "-b", "main")
	gitT(t, main, "init", "-q", "-b", "main")
	for k, v := range map[string]string{"user.name": "t", "user.email": "t@t", "commit.gpgsign": "false", "core.hooksPath": filepath.Join(base, "no-hooks")} {
		gitT(t, main, "config", k, v)
	}
	// run directories are ignored, as `agk setup` arranges globally
	writeT(t, main, ".git/info/exclude", "/.agk/\n")
	writeT(t, main, "README.md", "readme\n")
	commitT(t, main, "init")
	gitT(t, main, "remote", "add", "origin", origin)
	gitT(t, main, "push", "-q", "-u", "origin", "main")
	writeT(t, main, "openspec/changes/c/proposal.md", "plan\n")
	writeT(t, main, "openspec/changes/c/tasks.md", "- [ ] 1.1 do it\n")

	h = newHarness(t, false)
	h.env.Worktree = worktree.Git{}
	h.env.RepoRoot = review.RepoRoot
	h.env.Cwd = main
	return h, main, base
}

// The whole worktree flow on real git: start, work, a plan of another change
// lands on main, sync, a refused finish, archive, sync again, finish.
func TestWorktreeFlowEndToEnd(t *testing.T) {
	h, main, _ := realRepo(t)
	wtPath := filepath.Join(main, ".worktrees", "c")

	// 1. start: plan committed on main, worktree created, applier run inside
	if code := runApply(h, "start", "c", "--worktree"); code != 0 {
		t.Fatalf("apply start: exit %d: %s", code, h.err)
	}
	var started struct {
		Companion *string `json:"companion"`
		Dir       string  `json:"dir"`
		Worktree  string  `json:"worktree"`
		Pulled    bool    `json:"pulled"`
	}
	decodeOut(t, h, &started)
	if started.Companion != nil || started.Worktree != wtPath || !started.Pulled || !strings.HasPrefix(started.Dir, filepath.Join(wtPath, ".agk", "applies")) {
		t.Fatalf("apply start printed %+v", started)
	}
	if got := gitT(t, main, "log", "-1", "--format=%s"); got != "docs(openspec): plan c" {
		t.Fatalf("main's last commit = %q", got)
	}
	if got := gitT(t, wtPath, "rev-parse", "--abbrev-ref", "HEAD"); got != "c" {
		t.Fatalf("worktree branch = %q", got)
	}
	if got := gitT(t, main, "config", "--get", "branch.c.agkBase"); got != "main" {
		t.Fatalf("agkBase = %q", got)
	}

	// 2. the applier's work, committed on the branch
	writeT(t, wtPath, "feature.go", "package main\n")
	commitT(t, wtPath, "feat: the feature")

	// 3. another change's plan lands on main; sync replays the branch
	writeT(t, main, "openspec/changes/other/proposal.md", "other\n")
	commitT(t, main, "docs(openspec): plan other")
	if code := h.env.Main(context.Background(), []string{"sync", "c"}); code != 0 {
		t.Fatalf("sync: exit %d: %s", code, h.err)
	}
	var synced syncResult
	decodeOut(t, h, &synced)
	if !synced.Rebased || synced.Base != "main" || synced.Head != gitT(t, wtPath, "rev-parse", "HEAD") {
		t.Fatalf("sync printed %+v", synced)
	}

	// 4. finish before the archive refuses and changes nothing
	mainTip := gitT(t, main, "rev-parse", "main")
	if code := runFinish(h, "c"); code != 1 || !strings.Contains(h.err.String(), "not archived") {
		t.Fatalf("finish before archive: exit %d, %q", code, h.err)
	}
	if gitT(t, main, "rev-parse", "main") != mainTip || !exists(wtPath) {
		t.Fatal("a refused finish changed something")
	}
	h.err.Reset()

	// 5. the archive, committed; a second sync has nothing to do
	if err := os.MkdirAll(filepath.Join(wtPath, "openspec", "changes", "archive"), 0o755); err != nil {
		t.Fatal(err)
	}
	gitT(t, wtPath, "mv", "openspec/changes/c", "openspec/changes/archive/2026-01-01-c")
	commitT(t, wtPath, "chore(openspec): archive c")
	if code := h.env.Main(context.Background(), []string{"sync", "c"}); code != 0 {
		t.Fatalf("second sync: exit %d: %s", code, h.err)
	}
	synced = syncResult{}
	decodeOut(t, h, &synced)
	if synced.Rebased {
		t.Fatalf("second sync rebased again: %+v", synced)
	}
	branchTip := gitT(t, wtPath, "rev-parse", "HEAD")

	// 6. finish: fast-forward, cleanup
	if code := runFinish(h, "c"); code != 0 {
		t.Fatalf("finish: exit %d: %s", code, h.err)
	}
	var finished finishResult
	decodeOut(t, h, &finished)
	if finished.Base != "main" || finished.Head != branchTip || len(finished.Closed) != 0 {
		t.Fatalf("finish printed %+v", finished)
	}
	if got := gitT(t, main, "rev-parse", "main"); got != branchTip {
		t.Fatalf("main = %s, want the branch tip %s", got, branchTip)
	}
	if merges := gitT(t, main, "rev-list", "--merges", "--count", "main"); merges != "0" {
		t.Fatalf("%s merge commits", merges)
	}
	subjects := gitT(t, main, "log", "--format=%s", "-4", "main")
	if want := "chore(openspec): archive c\nfeat: the feature\ndocs(openspec): plan other\ndocs(openspec): plan c"; subjects != want {
		t.Fatalf("history on main:\n%s\nwant\n%s", subjects, want)
	}
	if exists(wtPath) {
		t.Fatal("the worktree is still there")
	}
	if gitT(t, main, "branch", "--list", "c") != "" {
		t.Fatal("branch c is still there")
	}
	if out, err := exec.Command("git", "-C", main, "config", "--get", "branch.c.agkBase").Output(); err == nil {
		t.Fatalf("agkBase survived: %s", out)
	}

	// 7. a rerun finds nothing left to finish
	if code := runFinish(h, "c"); code != 1 {
		t.Fatalf("finish after finish: exit %d", code)
	}
}

// `git worktree add` can create the branch and the worktree and still fail (a
// failing post-checkout hook). The base is recorded before the add, so a start
// that reuses the worktree finds the original base even after the main checkout
// switched branches, and also when it runs inside the worktree.
func TestApplyStartAfterAFailedAddKeepsTheOriginalBase(t *testing.T) {
	for name, fromWorktree := range map[string]bool{"restart from the main checkout": false, "restart inside the worktree": true} {
		t.Run(name, func(t *testing.T) {
			h, main, base := realRepo(t)
			hooks := filepath.Join(base, "no-hooks")
			writeT(t, hooks, "post-checkout", "#!/bin/sh\necho hook says no >&2\nexit 1\n")
			if err := os.Chmod(filepath.Join(hooks, "post-checkout"), 0o755); err != nil {
				t.Fatal(err)
			}
			if code := runApply(h, "start", "c", "--worktree"); code != 1 {
				t.Fatalf("first start: exit %d, want 1 (the hook fails)", code)
			}
			wtPath := filepath.Join(main, ".worktrees", "c")
			if !exists(wtPath) {
				t.Fatal("the failed `worktree add` left no worktree: the scenario does not hold")
			}
			gitT(t, main, "branch", "release")
			os.Remove(filepath.Join(hooks, "post-checkout")) // the switch below must not trip the hook
			gitT(t, main, "checkout", "-q", "release")
			if fromWorktree {
				h.env.Cwd = wtPath
			}
			h.out.Reset()
			h.err.Reset()
			if code := runApply(h, "start", "c", "--worktree"); code != 0 {
				t.Fatalf("second start: exit %d: %s", code, h.err)
			}
			if got := gitT(t, main, "config", "--get", "branch.c.agkBase"); got != "main" {
				t.Fatalf("agkBase = %q, want main (the main checkout is on release now)", got)
			}
			if !strings.Contains(h.out.String(), `"worktree":"`+wtPath+`"`) {
				t.Fatalf("stdout = %q", h.out)
			}
		})
	}
}
