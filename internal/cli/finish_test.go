package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/shurastik/agterm-kit/internal/agterm/agtermtest"
	"github.com/shurastik/agterm-kit/internal/worktree"
)

func runFinish(h *tenv, args ...string) int {
	return h.env.Main(context.Background(), append([]string{"finish"}, args...))
}

// finishHarness is a synced, archived, clean change `c` in .worktrees/c under
// a temp main checkout on `main`, with an applier run (companion C-1) and a
// reviewer run (companion C-2) under the worktree's .agk/.
func finishHarness(t *testing.T) (*tenv, string) {
	t.Helper()
	h, wt := syncHarness(t)
	h.wt.archived = true
	h.wt.ancestors["main>c"] = true // synced: the base is contained in the branch
	h.wt.heads["main"] = []string{"TIP"}
	for dir, id := range map[string]string{
		filepath.Join(wt, ".agk", "applies", "20261105-100000"): "C-1",
		filepath.Join(wt, ".agk", "reviews", "20261105-100001"): "C-2",
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, CompanionFile), []byte(id+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return h, wt
}

// forbidCleanup fails when a merge or any cleanup ran.
func (h *tenv) noMergeOrCleanup(t *testing.T) {
	t.Helper()
	if got := h.wt.only("MergeFFOnly", "Remove", "DeleteBranch"); len(got) != 0 {
		t.Fatalf("merge or cleanup ran: %v", h.wt.calls)
	}
	if got := h.ctl.Methods(); len(got) != 0 {
		t.Fatalf("companions touched: %v", got)
	}
}

func TestFinishLocationRefusals(t *testing.T) {
	cases := map[string]struct {
		change string
		prep   func(h *tenv)
		want   []string
	}{
		"bad change name":        {"Bad_Name", nil, []string{"kebab-case"}},
		"plain directory":        {"c", func(h *tenv) { h.wt.entry.Kind = worktree.PlainDir }, []string{filepath.Join(".worktrees", "c")}},
		"no agkBase":             {"c", func(h *tenv) { h.wt.hasAgkBase = false }, []string{"agkBase"}},
		"main on another branch": {"c", func(h *tenv) { h.wt.branch = "release" }, []string{"release", "main"}},
		"no worktree no branch": {"c", func(h *tenv) {
			h.wt.entry.Kind, h.wt.branchOK = worktree.Absent, false
		}, []string{filepath.Join(".worktrees", "c")}},
		"worktree gone, branch unmerged": {"c", func(h *tenv) {
			h.wt.entry.Kind = worktree.Absent
		}, []string{filepath.Join(".worktrees", "c"), "not merged"}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			h, _ := finishHarness(t)
			if c.prep != nil {
				c.prep(h)
			}
			if code := runFinish(h, c.change); code != 1 {
				t.Fatalf("exit %d", code)
			}
			for _, w := range c.want {
				if !strings.Contains(h.err.String(), w) {
					t.Errorf("stderr %q lacks %q", h.err, w)
				}
			}
			h.noMergeOrCleanup(t)
			if h.out.Len() != 0 {
				t.Fatalf("stdout = %q", h.out)
			}
		})
	}
}

func TestFinishChecks(t *testing.T) {
	cases := map[string]struct {
		prep func(h *tenv)
		want []string
	}{
		"uncommitted files": {func(h *tenv) { h.wt.status = []string{"a.go", "b.txt"} }, []string{"a.go", "b.txt"}},
		"not archived":      {func(h *tenv) { h.wt.archived = false }, []string{"not archived"}},
		"base moved":        {func(h *tenv) { h.wt.ancestors["main>c"] = false }, []string{"agk sync", "main"}},
		"git fails":         {func(h *tenv) { h.wt.errs["ArchivedOnBranch"] = errors.New("fatal: bad ref") }, []string{"bad ref"}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			h, _ := finishHarness(t)
			c.prep(h)
			if code := runFinish(h, "c"); code != 1 {
				t.Fatalf("exit %d", code)
			}
			for _, w := range c.want {
				if !strings.Contains(h.err.String(), w) {
					t.Errorf("stderr %q lacks %q", h.err, w)
				}
			}
			h.noMergeOrCleanup(t)
		})
	}
}

func TestFinishNeverPullsOrRebases(t *testing.T) {
	h, _ := finishHarness(t)
	if code := runFinish(h, "c"); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	if got := h.wt.only("PullFFOnly", "Rebase", "Upstream"); len(got) != 0 {
		t.Fatalf("finish pulled or rebased: %v", h.wt.calls)
	}
}

func TestFinishFailedMergeChangesNothing(t *testing.T) {
	h, _ := finishHarness(t)
	h.wt.errs["MergeFFOnly"] = errors.New("error: Your local changes to the following files would be overwritten by merge: README.md")
	if code := runFinish(h, "c"); code != 1 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(h.err.String(), "would be overwritten") || h.out.Len() != 0 {
		t.Fatalf("stderr %q stdout %q", h.err, h.out)
	}
	if got := h.wt.only("Remove", "DeleteBranch"); len(got) != 0 || len(h.ctl.Calls) != 0 {
		t.Fatalf("cleanup after a failed merge: %v %v", h.wt.calls, h.ctl.Calls)
	}
}

func TestFinishMergesThenCleansUp(t *testing.T) {
	h, wt := finishHarness(t)
	closedBeforeRemove := -1
	h.wt.onRemove = func() { closedBeforeRemove = len(h.ctl.Calls) }
	if code := runFinish(h, "c"); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	if got, want := h.wt.only("MergeFFOnly", "Remove", "DeleteBranch"), []string{"MergeFFOnly", "Remove", "DeleteBranch"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("order = %v", h.wt.calls)
	}
	for _, c := range h.wt.calls {
		switch c.Op {
		case "MergeFFOnly":
			if want := []string{h.wt.loc.Main, "c"}; !reflect.DeepEqual(c.Args, want) {
				t.Errorf("MergeFFOnly(%v), want %v", c.Args, want)
			}
		case "Remove":
			if want := []string{h.wt.loc.Main, wt}; !reflect.DeepEqual(c.Args, want) {
				t.Errorf("Remove(%v), want %v", c.Args, want)
			}
		case "DeleteBranch":
			if want := []string{h.wt.loc.Main, "c"}; !reflect.DeepEqual(c.Args, want) {
				t.Errorf("DeleteBranch(%v), want %v", c.Args, want)
			}
		}
	}
	// both runs' companions were closed, applies first, before the removal
	assertCalls(t, h.ctl,
		agtermtest.Call{Method: "Close", Args: []string{"C-1"}},
		agtermtest.Call{Method: "Close", Args: []string{"C-2"}},
	)
	if closedBeforeRemove != 2 {
		t.Fatalf("%d companions closed before the removal, want 2", closedBeforeRemove)
	}
	want := `{"change":"c","base":"main","head":"TIP","closed":["` +
		filepath.Join(wt, ".agk", "applies", "20261105-100000") + `","` +
		filepath.Join(wt, ".agk", "reviews", "20261105-100001") + `"]}` + "\n"
	if h.out.String() != want {
		t.Fatalf("stdout = %q\nwant     %q", h.out, want)
	}
}

func TestFinishGoneCompanionCountsAsClosed(t *testing.T) {
	h, _ := finishHarness(t)
	h.ctl.CloseErr = agtermGone()
	if code := runFinish(h, "c"); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	if !strings.Contains(h.out.String(), "20261105-100000") || !h.wt.called("Remove") || !h.wt.called("DeleteBranch") {
		t.Fatalf("stdout %q, calls %v", h.out, h.wt.calls)
	}
}

func TestFinishRunWithoutCompanionIsLeftAlone(t *testing.T) {
	h, wt := finishHarness(t)
	inline := filepath.Join(wt, ".agk", "applies", "20261105-100005")
	if err := os.MkdirAll(inline, 0o755); err != nil {
		t.Fatal(err)
	}
	if code := runFinish(h, "c"); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	if strings.Contains(h.out.String(), "100005") || len(h.ctl.Calls) != 2 {
		t.Fatalf("stdout %q, ctl calls %v", h.out, h.ctl.Calls)
	}
}

func TestFinishFailedCloseKeepsTheWorktree(t *testing.T) {
	h, wt := finishHarness(t)
	h.ctl.CloseErr = errors.New("socket refused")
	if code := runFinish(h, "c"); code != 1 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(h.err.String(), filepath.Join(wt, ".agk", "applies", "20261105-100000")) || !strings.Contains(h.err.String(), "socket refused") {
		t.Fatalf("stderr = %q", h.err)
	}
	if got := h.wt.only("Remove", "DeleteBranch"); len(got) != 0 || h.out.Len() != 0 {
		t.Fatalf("calls %v, stdout %q", h.wt.calls, h.out)
	}
	if !h.wt.called("MergeFFOnly") {
		t.Fatal("the merge stays")
	}
}

func TestFinishFailedRemoveKeepsTheBranch(t *testing.T) {
	h, wt := finishHarness(t)
	h.wt.errs["Remove"] = errors.New("fatal: contains modified or untracked files")
	if code := runFinish(h, "c"); code != 1 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(h.err.String(), wt) || !strings.Contains(h.err.String(), "untracked files") || !strings.Contains(h.err.String(), "merged") {
		t.Fatalf("stderr = %q", h.err)
	}
	if h.wt.called("DeleteBranch") || h.out.Len() != 0 {
		t.Fatalf("calls %v stdout %q", h.wt.calls, h.out)
	}
}

func TestFinishFailedBranchDelete(t *testing.T) {
	h, _ := finishHarness(t)
	h.wt.errs["DeleteBranch"] = errors.New("error: the branch 'c' is not fully merged")
	if code := runFinish(h, "c"); code != 1 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(h.err.String(), "branch c") || !strings.Contains(h.err.String(), "not fully merged") || h.out.Len() != 0 {
		t.Fatalf("stderr %q stdout %q", h.err, h.out)
	}
}

func TestFinishRerunAfterTheMerge(t *testing.T) {
	h, _ := finishHarness(t)
	h.wt.ancestors["c>main"] = true // branch contained in the base: merged earlier
	if code := runFinish(h, "c"); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	if got := h.wt.only("MergeFFOnly", "Remove", "DeleteBranch"); !reflect.DeepEqual(got, []string{"Remove", "DeleteBranch"}) {
		t.Fatalf("calls = %v", h.wt.calls)
	}
	if len(h.ctl.Calls) != 2 {
		t.Fatalf("companions still due: %v", h.ctl.Calls)
	}
}

func TestFinishRerunWithTheWorktreeGone(t *testing.T) {
	h, wt := finishHarness(t)
	h.wt.ancestors["c>main"] = true
	h.wt.entry = worktree.Entry{Path: wt, Kind: worktree.Absent}
	if code := runFinish(h, "c"); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	if got := h.wt.only("MergeFFOnly", "Remove", "DeleteBranch", "Status"); !reflect.DeepEqual(got, []string{"DeleteBranch"}) {
		t.Fatalf("calls = %v", h.wt.calls)
	}
	if len(h.ctl.Calls) != 0 {
		t.Fatalf("closes with no worktree: %v", h.ctl.Calls)
	}
	if want := `{"change":"c","base":"main","head":"TIP","closed":[]}` + "\n"; h.out.String() != want {
		t.Fatalf("stdout = %q", h.out)
	}
}

func TestFinishRerunAfterTheBaseAdvanced(t *testing.T) {
	h, _ := finishHarness(t)
	h.wt.ancestors["c>main"] = true  // merged earlier
	h.wt.ancestors["main>c"] = false // main moved on since: not a "sync" case
	h.wt.branch = "release"          // the main checkout may even be elsewhere by now
	if code := runFinish(h, "c"); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	if strings.Contains(h.err.String(), "agk sync") || h.wt.called("MergeFFOnly") || !h.wt.called("DeleteBranch") {
		t.Fatalf("stderr %q calls %v", h.err, h.wt.calls)
	}
}

// A fresh branch with no commits is contained in its base, yet nothing was
// merged: the archive check keeps finish from cleaning it up.
func TestFinishRefusesAnUnarchivedBranchEvenWhenContained(t *testing.T) {
	h, _ := finishHarness(t)
	h.wt.ancestors["c>main"] = true
	h.wt.archived = false
	if code := runFinish(h, "c"); code != 1 || !strings.Contains(h.err.String(), "not archived") {
		t.Fatalf("exit %d, %q", code, h.err)
	}
	h.noMergeOrCleanup(t)
}

func TestFinishUsage(t *testing.T) {
	for _, args := range [][]string{nil, {"a", "b"}, {"c", "--no-pull"}} {
		h, _ := finishHarness(t)
		if code := runFinish(h, args...); code != 1 || len(h.wt.calls) != 0 {
			t.Fatalf("%q: exit %d, calls %v", args, code, h.wt.calls)
		}
	}
}
