package cli

import (
	"context"
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
	if want := `{"fresh":true,"round":1}` + "\n"; h.out.String() != want {
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
	if want := `{"fresh":true,"round":2}` + "\n"; h.out.String() != want {
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
	if want := `{"fresh":false,"round":1}` + "\n"; h.out.String() != want {
		t.Fatalf("stdout = %q, want %q", h.out, want)
	}
}

func TestReviewFreshFalseWhenNothingToReview(t *testing.T) {
	h := newHarness(t, true)
	_, dir := freshDir(t, h, scopeFingerprint)
	h.env.Collect = func(context.Context, string, string) (review.Scope, error) {
		return review.Scope{}, review.ErrNothingToReview
	}
	if code := runFresh(h, dir); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	if want := `{"fresh":false,"round":1}` + "\n"; h.out.String() != want {
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
