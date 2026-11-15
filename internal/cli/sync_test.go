package cli

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/shurastik/agterm-kit/internal/worktree"
)

func runSync(h *tenv, args ...string) int {
	return h.env.Main(context.Background(), append([]string{"sync"}, args...))
}

// syncHarness is a harness whose fake describes a healthy change worktree
// .worktrees/c under a main checkout on `main`.
func syncHarness(t *testing.T) (*tenv, string) {
	t.Helper()
	h := newHarness(t, false)
	main, _ := filepath.EvalSymlinks(t.TempDir())
	h.wt = newFakeWorktree(main)
	h.env.Worktree = h.wt
	h.wt.entry = worktree.Entry{Path: worktree.WorktreePath(main, "c"), Kind: worktree.IsWorktree, Branch: "c"}
	h.wt.branchOK = true
	h.wt.agkBase, h.wt.hasAgkBase = "main", true
	h.wt.heads["refs/heads/c"] = []string{"OLD", "NEW"}
	return h, worktree.WorktreePath(main, "c")
}

func TestSyncLocationRefusals(t *testing.T) {
	cases := map[string]struct {
		change string
		prep   func(h *tenv)
		want   []string
	}{
		"bad change name": {"Bad_Name", nil, []string{"kebab-case"}},
		"no worktree": {"c", func(h *tenv) {
			h.wt.entry = worktree.Entry{Path: worktree.WorktreePath(h.wt.loc.Main, "c"), Kind: worktree.Absent}
		}, []string{filepath.Join(".worktrees", "c")}},
		"plain directory":        {"c", func(h *tenv) { h.wt.entry.Kind = worktree.PlainDir }, []string{filepath.Join(".worktrees", "c")}},
		"another branch":         {"c", func(h *tenv) { h.wt.entry.Kind = worktree.OtherBranch; h.wt.entry.Branch = "x" }, []string{"x"}},
		"no agkBase":             {"c", func(h *tenv) { h.wt.hasAgkBase = false }, []string{"agkBase"}},
		"main on another branch": {"c", func(h *tenv) { h.wt.branch = "release" }, []string{"release", "main"}},
		"locate fails":           {"c", func(h *tenv) { h.wt.errs["Locate"] = errors.New("git exploded") }, []string{"git exploded"}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			h, _ := syncHarness(t)
			if c.prep != nil {
				c.prep(h)
			}
			if code := runSync(h, c.change); code != 1 {
				t.Fatalf("exit %d", code)
			}
			for _, w := range c.want {
				if !strings.Contains(h.err.String(), w) {
					t.Errorf("stderr %q lacks %q", h.err, w)
				}
			}
			if got := h.wt.only("PullFFOnly", "Rebase", "Status"); len(got) != 0 || h.out.Len() != 0 {
				t.Fatalf("calls %v, stdout %q", h.wt.calls, h.out)
			}
		})
	}
}

func TestSyncUsage(t *testing.T) {
	for _, args := range [][]string{nil, {"a", "b"}} {
		h, _ := syncHarness(t)
		if code := runSync(h, args...); code != 1 || len(h.wt.calls) != 0 {
			t.Fatalf("%q: exit %d, calls %v", args, code, h.wt.calls)
		}
	}
}

func TestSyncUncommittedFiles(t *testing.T) {
	h, _ := syncHarness(t)
	h.wt.status = []string{"internal/a.go", "new.txt"}
	if code := runSync(h, "c"); code != 1 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(h.err.String(), "internal/a.go") || !strings.Contains(h.err.String(), "new.txt") {
		t.Fatalf("stderr = %q", h.err)
	}
	if got := h.wt.only("PullFFOnly", "Rebase"); len(got) != 0 {
		t.Fatalf("calls = %v", h.wt.calls)
	}
}

func TestSyncPullsThenRebases(t *testing.T) {
	h, wt := syncHarness(t)
	if code := runSync(h, "c"); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	if got, want := h.wt.only("Status", "PullFFOnly", "Rebase"), []string{"Status", "PullFFOnly", "Rebase"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("calls = %v", h.wt.calls)
	}
	for _, c := range h.wt.calls {
		switch c.Op {
		case "Status":
			if c.Args[0] != wt {
				t.Errorf("status of %q, want the worktree %q", c.Args[0], wt)
			}
		case "PullFFOnly":
			if c.Args[0] != h.wt.loc.Main {
				t.Errorf("pull in %q, want the main checkout", c.Args[0])
			}
		case "Rebase":
			if want := []string{wt, "main"}; !reflect.DeepEqual(c.Args, want) {
				t.Errorf("Rebase(%v), want %v", c.Args, want)
			}
		}
	}
	if want := `{"change":"c","base":"main","pulled":true,"rebased":true,"head":"NEW"}` + "\n"; h.out.String() != want {
		t.Fatalf("stdout = %q, want %q", h.out, want)
	}
}

func TestSyncUnchangedBranch(t *testing.T) {
	h, _ := syncHarness(t)
	h.wt.heads["refs/heads/c"] = []string{"SAME"}
	if code := runSync(h, "c"); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	if want := `{"change":"c","base":"main","pulled":true,"rebased":false,"head":"SAME"}` + "\n"; h.out.String() != want {
		t.Fatalf("stdout = %q", h.out)
	}
}

func TestSyncSkipsThePull(t *testing.T) {
	for name, tc := range map[string]struct {
		args []string
		prep func(h *tenv)
	}{
		"--no-pull":   {[]string{"--no-pull"}, nil},
		"no upstream": {nil, func(h *tenv) { h.wt.hasUpstream = false }},
	} {
		t.Run(name, func(t *testing.T) {
			h, _ := syncHarness(t)
			if tc.prep != nil {
				tc.prep(h)
			}
			if code := runSync(h, append([]string{"c"}, tc.args...)...); code != 0 {
				t.Fatalf("exit %d: %s", code, h.err)
			}
			if got := h.wt.only("PullFFOnly", "Rebase"); !reflect.DeepEqual(got, []string{"Rebase"}) {
				t.Fatalf("calls = %v", h.wt.calls)
			}
			if !strings.Contains(h.out.String(), `"pulled":false`) {
				t.Fatalf("stdout = %q", h.out)
			}
		})
	}
}

func TestSyncFailedPull(t *testing.T) {
	h, _ := syncHarness(t)
	h.wt.errs["PullFFOnly"] = errors.New("fatal: Not possible to fast-forward, aborting.")
	if code := runSync(h, "c"); code != 1 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(h.err.String(), "Not possible to fast-forward") || h.wt.called("Rebase") || h.out.Len() != 0 {
		t.Fatalf("stderr %q, calls %v, stdout %q", h.err, h.wt.calls, h.out)
	}
}

func TestSyncRebaseConflict(t *testing.T) {
	h, _ := syncHarness(t)
	h.wt.errs["Rebase"] = &worktree.ConflictError{Files: []string{"a.go", "dir/b.go"}}
	if code := runSync(h, "c"); code != 1 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(h.err.String(), "a.go") || !strings.Contains(h.err.String(), "dir/b.go") || h.out.Len() != 0 {
		t.Fatalf("stderr %q stdout %q", h.err, h.out)
	}
	if h.wt.called("Remove") || h.wt.called("DeleteBranch") {
		t.Fatalf("sync must not touch the worktree: %v", h.wt.calls)
	}
}
