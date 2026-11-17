package cli

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/shurastik/agterm-kit/internal/review"
	"github.com/shurastik/agterm-kit/internal/rundir"
)

func runFresh(h *tenv, args ...string) int {
	return h.env.Main(context.Background(), append([]string{"review", "fresh"}, args...))
}

// freshDir is a diff run at round 1 whose scope.sha256 holds fp.
func freshDir(t *testing.T, h *tenv, fp string) (root, dir string) {
	t.Helper()
	root, dir = sendDir(t, h, "", true)
	if err := os.WriteFile(scopeFilePath(dir, 1), []byte(fp+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root, dir
}

// treeState lists every file under dir with its content, to prove a command
// wrote nothing.
func treeState(t *testing.T, dir string) map[string]string {
	t.Helper()
	m := map[string]string{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			m[p+"/"] = ""
			return nil
		}
		b, err := os.ReadFile(p)
		m[p] = string(b)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestReviewFreshTrueWhenScopeMatches(t *testing.T) {
	h := newHarness(t, true)
	root, dir := freshDir(t, h, scopeFingerprint)
	seen := scriptScope(h, root)
	before := treeState(t, dir)
	if code := runFresh(h, dir); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	if want := `{"fresh":true,"round":1,"merge_base":"mb"}` + "\n"; h.out.String() != want {
		t.Fatalf("stdout = %q, want %q", h.out, want)
	}
	if !reflect.DeepEqual(*seen, []string{root, "main"}) {
		t.Fatalf("collect = %q, want the run's root and base", *seen)
	}
	if !reflect.DeepEqual(treeState(t, dir), before) {
		t.Fatal("fresh changed the run directory")
	}
	if len(h.ctl.Calls) != 0 {
		t.Fatalf("fresh must not call agterm: %v", h.ctl.Calls)
	}
}

func TestReviewFreshUsesCurrentRound(t *testing.T) {
	h := newHarness(t, true)
	root, dir := freshDir(t, h, "fp-old")
	scriptScope(h, root)
	if _, err := rundir.MakeRound(dir, 2); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(scopeFilePath(dir, 2), []byte(scopeFingerprint+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := rundir.SetRound(dir, 2); err != nil {
		t.Fatal(err)
	}
	if code := runFresh(h, dir); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	if want := `{"fresh":true,"round":2,"merge_base":"mb"}` + "\n"; h.out.String() != want {
		t.Fatalf("stdout = %q, want %q", h.out, want)
	}
}

func TestReviewFreshFalseWhenScopeDiffers(t *testing.T) {
	h := newHarness(t, true)
	root, dir := freshDir(t, h, "fp-of-an-older-state")
	scriptScope(h, root)
	if code := runFresh(h, dir); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	if want := `{"fresh":false,"round":1,"merge_base":"mb"}` + "\n"; h.out.String() != want {
		t.Fatalf("stdout = %q, want %q", h.out, want)
	}
}

func TestReviewFreshFalseWhenNothingToReview(t *testing.T) {
	h := newHarness(t, true)
	_, dir := freshDir(t, h, scopeFingerprint)
	h.env.Collect = func(context.Context, string, string) (review.Scope, error) {
		return review.Scope{MergeBase: "mb-old"}, review.ErrNothingToReview
	}
	if code := runFresh(h, dir); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	if want := `{"fresh":false,"round":1,"merge_base":"mb-old"}` + "\n"; h.out.String() != want {
		t.Fatalf("stdout = %q, want %q", h.out, want)
	}
}

func TestReviewFreshRefusals(t *testing.T) {
	cases := map[string]struct {
		prep func(t *testing.T, h *tenv) string // returns the run dir argument
		want string
	}{
		"plan run": {func(t *testing.T, h *tenv) string {
			_, dir, _ := planSendDir(t, h, "", true, "TASKS\n")
			return dir
		}, "plan review"},
		"round without fingerprint": {func(t *testing.T, h *tenv) string {
			_, dir := sendDir(t, h, "", true)
			return dir
		}, "older agk"},
		"bad run dir": {func(t *testing.T, h *tenv) string {
			return t.TempDir()
		}, "agk"},
		"collect error": {func(t *testing.T, h *tenv) string {
			_, dir := freshDir(t, h, scopeFingerprint)
			h.env.Collect = func(context.Context, string, string) (review.Scope, error) {
				return review.Scope{}, os.ErrPermission
			}
			return dir
		}, "permission"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t, true)
			dir := c.prep(t, h)
			if code := runFresh(h, dir); code != 1 {
				t.Fatalf("exit %d, want 1", code)
			}
			if h.out.Len() != 0 {
				t.Fatalf("stdout = %q, want no JSON", h.out)
			}
			if !strings.Contains(h.err.String(), c.want) {
				t.Fatalf("stderr = %q, want %q", h.err, c.want)
			}
			if len(h.ctl.Calls) != 0 {
				t.Fatalf("agterm called: %v", h.ctl.Calls)
			}
		})
	}
}

func TestReviewFreshUsage(t *testing.T) {
	for _, args := range [][]string{nil, {"a", "b"}} {
		h := newHarness(t, true)
		if code := runFresh(h, args...); code != 1 || h.out.Len() != 0 || !strings.Contains(h.err.String(), "usage: agk review fresh <run-dir>") {
			t.Errorf("args %q: exit %d stdout %q stderr %q", args, code, h.out, h.err)
		}
	}
}

// recordMergeBase writes round k's merge-base file, as a diff round of this
// agk does.
func recordMergeBase(t *testing.T, dir string, k int, mb string) {
	t.Helper()
	if err := os.WriteFile(mergeBaseFilePath(dir, k), []byte(mb+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// scriptAncestry makes IsAncestor answer ok/err and records the (dir, commit)
// pairs it was asked.
func scriptAncestry(h *tenv, ok bool, err error) *[]string {
	var seen []string
	h.env.IsAncestor = func(_ context.Context, dir, commit string) (bool, error) {
		seen = append(seen, dir, commit)
		return ok, err
	}
	return &seen
}

func TestReviewFreshCollectsAgainstRecordedMergeBase(t *testing.T) {
	h := newHarness(t, true)
	root, dir := freshDir(t, h, scopeFingerprint)
	recordMergeBase(t, dir, 1, "M-recorded")
	asked := scriptAncestry(h, true, nil)
	var collected []string
	h.env.Collect = func(_ context.Context, d, base string) (review.Scope, error) {
		collected = append(collected, d, base)
		return review.Scope{Root: root, Base: base, MergeBase: base, Diff: "+x\n", Fingerprint: scopeFingerprint}, nil
	}
	before := treeState(t, dir)
	if code := runFresh(h, dir); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	if want := `{"fresh":true,"round":1,"merge_base":"M-recorded"}` + "\n"; h.out.String() != want {
		t.Fatalf("stdout = %q, want %q", h.out, want)
	}
	if !reflect.DeepEqual(*asked, []string{root, "M-recorded"}) {
		t.Fatalf("ancestry asked %q, want the root and the recorded merge base", *asked)
	}
	if !reflect.DeepEqual(collected, []string{root, "M-recorded"}) {
		t.Fatalf("collect = %q, want the recorded merge base, not the run's base", collected)
	}
	if !reflect.DeepEqual(treeState(t, dir), before) {
		t.Fatal("fresh changed the run directory")
	}
}

func TestReviewFreshFalseWhenScopeDiffersAtRecordedMergeBase(t *testing.T) {
	h := newHarness(t, true)
	root, dir := freshDir(t, h, "fp-of-an-older-state")
	recordMergeBase(t, dir, 1, "M-recorded")
	scriptAncestry(h, true, nil)
	scriptScope(h, root)
	if code := runFresh(h, dir); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	if want := `{"fresh":false,"round":1,"merge_base":"M-recorded"}` + "\n"; h.out.String() != want {
		t.Fatalf("stdout = %q, want %q", h.out, want)
	}
}

func TestReviewFreshNotAncestorIsStaleWithoutCollect(t *testing.T) {
	h := newHarness(t, true)
	_, dir := freshDir(t, h, scopeFingerprint)
	recordMergeBase(t, dir, 1, "M-recorded")
	scriptAncestry(h, false, nil)
	h.env.Collect = func(context.Context, string, string) (review.Scope, error) {
		t.Fatal("Collect must not run when the recorded merge base is not an ancestor of HEAD")
		return review.Scope{}, nil
	}
	if code := runFresh(h, dir); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	if want := `{"fresh":false,"round":1,"merge_base":"M-recorded"}` + "\n"; h.out.String() != want {
		t.Fatalf("stdout = %q, want %q", h.out, want)
	}
}

func TestReviewFreshAncestryErrorExitsOne(t *testing.T) {
	h := newHarness(t, true)
	_, dir := freshDir(t, h, scopeFingerprint)
	recordMergeBase(t, dir, 1, "M-recorded")
	scriptAncestry(h, false, errors.New("commit M-recorded is not in the repository"))
	h.env.Collect = func(context.Context, string, string) (review.Scope, error) {
		t.Fatal("Collect must not run after an ancestry error")
		return review.Scope{}, nil
	}
	if code := runFresh(h, dir); code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
	if h.out.Len() != 0 || !strings.Contains(h.err.String(), "M-recorded") {
		t.Fatalf("stdout = %q stderr = %q, want no JSON and an error naming the merge base", h.out, h.err)
	}
}

func TestReviewFreshNothingToReviewAtRecordedMergeBase(t *testing.T) {
	h := newHarness(t, true)
	_, dir := freshDir(t, h, scopeFingerprint)
	recordMergeBase(t, dir, 1, "M-recorded")
	scriptAncestry(h, true, nil)
	h.env.Collect = func(context.Context, string, string) (review.Scope, error) {
		return review.Scope{MergeBase: "M-recorded"}, review.ErrNothingToReview
	}
	if code := runFresh(h, dir); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	if want := `{"fresh":false,"round":1,"merge_base":"M-recorded"}` + "\n"; h.out.String() != want {
		t.Fatalf("stdout = %q, want %q", h.out, want)
	}
}

func TestReviewFreshOldRoundSkipsAncestry(t *testing.T) {
	h := newHarness(t, true)
	root, dir := freshDir(t, h, scopeFingerprint)
	h.env.IsAncestor = func(context.Context, string, string) (bool, error) {
		t.Fatal("a round without a merge-base file takes no ancestry check")
		return false, nil
	}
	seen := scriptScope(h, root)
	if code := runFresh(h, dir); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	if !reflect.DeepEqual(*seen, []string{root, "main"}) {
		t.Fatalf("collect = %q, want the run's base", *seen)
	}
}

func TestReviewFreshEmptyMergeBaseFileExitsOne(t *testing.T) {
	h := newHarness(t, true)
	_, dir := freshDir(t, h, scopeFingerprint)
	recordMergeBase(t, dir, 1, "")
	scriptAncestry(h, true, nil)
	if code := runFresh(h, dir); code != 1 || h.out.Len() != 0 || !strings.Contains(h.err.String(), "merge-base") {
		t.Fatalf("exit %d stdout %q stderr %q, want exit 1, no JSON and an error naming the file", code, h.out, h.err)
	}
}

// The whole freshness story on real git: a diff review of uncommitted work on
// main, a commit of that work on main, an edit, and a switch to unrelated history.
func TestReviewFreshEndToEndOnBaseBranch(t *testing.T) {
	h, main, _ := realRepo(t)
	h.env.Collect = review.Collect
	h.env.IsAncestor = review.IsAncestor
	h.drivers["codex"].outputs = []string{approve}
	writeT(t, main, "widget.go", "package w\n")

	if code := h.env.ReviewStart(context.Background(), []string{"--base", "main"}); code != 0 {
		t.Fatalf("review start: exit %d: %s", code, h.err)
	}
	var started startResult
	decodeOut(t, h, &started)
	dir := started.Dir
	wantMB := gitT(t, main, "rev-parse", "HEAD")
	if got, err := os.ReadFile(mergeBaseFilePath(dir, 1)); err != nil || strings.TrimSpace(string(got)) != wantMB {
		t.Fatalf("recorded merge base = %q, %v; want %s", got, err, wantMB)
	}

	fresh := func(t *testing.T) (freshResult, int) {
		t.Helper()
		h.out.Reset()
		h.err.Reset()
		code := runFresh(h, dir)
		var r freshResult
		if code == 0 {
			decodeOut(t, h, &r)
		}
		return r, code
	}

	// Committing the reviewed file on main moves merge-base(main, HEAD), not the
	// recorded merge base.
	gitT(t, main, "add", "widget.go")
	gitT(t, main, "commit", "-qm", "widget")
	r, code := fresh(t)
	if code != 0 || !r.Fresh || r.Round != 1 || r.MergeBase != wantMB {
		t.Fatalf("after the commit: exit %d, %+v, stderr %s; want fresh at %s", code, r, h.err, wantMB)
	}

	// An edit makes the round stale.
	writeT(t, main, "widget.go", "package w\n\nfunc W() {}\n")
	r, code = fresh(t)
	if code != 0 || r.Fresh || r.MergeBase != wantMB {
		t.Fatalf("after the edit: exit %d, %+v, stderr %s; want stale at %s", code, r, h.err, wantMB)
	}

	// Unrelated history: the recorded merge base is no longer an ancestor of HEAD.
	gitT(t, main, "checkout", "-q", "--orphan", "other")
	commitT(t, main, "orphan")
	r, code = fresh(t)
	if code != 0 || r.Fresh || r.MergeBase != wantMB {
		t.Fatalf("on an orphan branch: exit %d, %+v, stderr %s; want stale at %s, exit 0", code, r, h.err, wantMB)
	}
}
