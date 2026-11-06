package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/shurastik/agterm-kit/internal/agterm"
	"github.com/shurastik/agterm-kit/internal/agterm/agtermtest"
	"github.com/shurastik/agterm-kit/internal/config"
	"github.com/shurastik/agterm-kit/internal/review"
	"github.com/shurastik/agterm-kit/internal/rundir"
)

// sendDir is a codex review run whose round 1 finished, with companion C-1 in pane P-1.
func sendDir(t *testing.T, h *tenv, session string, companion bool) (string, string) {
	t.Helper()
	root, dir := runDirWith(t, config.Worker{Harness: "codex"}, session)
	writeResultFile(t, dir, completedResult)
	if companion {
		os.WriteFile(filepath.Join(dir, CompanionFile), []byte("C-1\n"), 0o644)
		os.WriteFile(filepath.Join(dir, paneFile), []byte("P-1\n"), 0o644)
	}
	scriptScope(h, root)
	return root, dir
}

func runSend(h *tenv, args ...string) int {
	return h.env.Main(context.Background(), append([]string{"review", "send"}, args...))
}

func TestReviewSendSecondRound(t *testing.T) {
	h := newHarness(t, true)
	root, dir := sendDir(t, h, "T-1", true)
	seen := scriptScope(h, root)
	note := filepath.Join(t.TempDir(), "fixed.md")
	os.WriteFile(note, []byte("Fixed the nil check."), 0o644)
	h.ctl.WaitStatus = "active"

	if code := runSend(h, dir, "--note", note); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	if !reflect.DeepEqual(*seen, []string{root, "main"}) {
		t.Fatalf("collect = %q", *seen)
	}
	p, err := os.ReadFile(filepath.Join(dir, "rounds", "2", "prompt.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"round 2", "earlier findings", "Fixed the nil check.", "+x"} {
		if !strings.Contains(string(p), want) {
			t.Errorf("round 2 prompt lacks %q", want)
		}
	}
	if k, _ := rundir.Round(dir); k != 2 {
		t.Fatalf("round = %d", k)
	}
	assertCalls(t, h.ctl,
		agtermtest.Call{Method: "MsgSend", Args: []string{"C-1:P-1", "round 2"}},
		agtermtest.Call{Method: "Wait", Args: []string{"C-1", "active", "30s"}},
	)
	if want := `{"companion":"C-1","dir":"` + dir + `","round":2}` + "\n"; h.out.String() != want {
		t.Fatalf("stdout = %q", h.out)
	}
}

func TestReviewSendWithoutSessionUsesFirstRoundPrompt(t *testing.T) {
	h := newHarness(t, true)
	_, dir := sendDir(t, h, "", true)
	h.ctl.WaitStatus = "active"
	if code := runSend(h, dir); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	p, _ := os.ReadFile(filepath.Join(dir, "rounds", "2", "prompt.md"))
	first, _ := review.Prompt(review.Scope{Base: "main", MergeBase: "mb", Diff: "+x\n", Stat: " f | 1 +\n"})
	if string(p) != first {
		t.Fatalf("prompt = %q", p)
	}
}

func TestReviewSendRefusals(t *testing.T) {
	cases := map[string]struct {
		prep func(t *testing.T, h *tenv, dir string)
		want string
	}{
		"still running": {func(t *testing.T, h *tenv, dir string) { os.Remove(filepath.Join(dir, "rounds", "1", "result.json")) }, "still running"},
		"nothing to review": {func(t *testing.T, h *tenv, dir string) {
			h.env.Collect = func(context.Context, string, string) (review.Scope, error) {
				return review.Scope{}, review.ErrNothingToReview
			}
		}, "nothing to review"},
		"pane unknown":  {func(t *testing.T, h *tenv, dir string) { os.Remove(filepath.Join(dir, paneFile)) }, "pane"},
		"reviewer gone": {func(t *testing.T, h *tenv, dir string) { h.ctl.MsgSendErr = agterm.ErrGone }, "gone"},
		"send error":    {func(t *testing.T, h *tenv, dir string) { h.ctl.MsgSendErr = errors.New("socket refused") }, "socket refused"},
		"missing note":  {func(t *testing.T, h *tenv, dir string) {}, "no-such-note"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t, true)
			_, dir := sendDir(t, h, "T-1", true)
			c.prep(t, h, dir)
			args := []string{dir}
			if name == "missing note" {
				args = append(args, "--note", filepath.Join(t.TempDir(), "no-such-note"))
			}
			if code := runSend(h, args...); code != 1 {
				t.Fatalf("exit %d, want 1 (stderr %q)", code, h.err)
			}
			if !strings.Contains(h.err.String(), c.want) || h.out.Len() != 0 {
				t.Fatalf("stderr %q stdout %q", h.err, h.out)
			}
			if k, _ := rundir.Round(dir); k != 1 || exists(filepath.Join(dir, "rounds", "2")) {
				t.Fatalf("no round may be recorded: round %d", k)
			}
		})
	}
}

func TestReviewSendHandshakeTimeoutWarns(t *testing.T) {
	h := newHarness(t, true)
	_, dir := sendDir(t, h, "T-1", true)
	h.ctl.WaitErr = agterm.ErrTimeout
	if code := runSend(h, dir); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	if !strings.Contains(h.err.String(), "warning") || !strings.Contains(h.out.String(), `"round":2`) {
		t.Fatalf("stderr %q stdout %q", h.err, h.out)
	}
}

func TestReviewSendInline(t *testing.T) {
	h := newHarness(t, false)
	root, dir := sendDir(t, h, "T-1", false)
	h.drivers["codex"].outputs = []string{approve}
	if code := runSend(h, dir); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	tr := h.drivers["codex"].turns
	if len(tr) != 1 || !tr[0].Resume || tr[0].Session != "T-1" || tr[0].Root != root || !strings.Contains(tr[0].Prompt, "round 2") {
		t.Fatalf("turns = %+v", tr)
	}
	if want := `{"companion":null,"dir":"` + dir + `","round":2}` + "\n"; h.out.String() != want {
		t.Fatalf("stdout = %q", h.out)
	}
	if len(h.ctl.Calls) != 0 {
		t.Fatalf("no agterm calls: %v", h.ctl.Calls)
	}
	assertResult(t, filepath.Join(dir, "rounds", "2"), "completed", 0)
}

func runClose(h *tenv, args ...string) int {
	return h.env.Main(context.Background(), append([]string{"review", "close"}, args...))
}

func TestReviewClose(t *testing.T) {
	h := newHarness(t, true)
	_, dir := sendDir(t, h, "T-1", true)
	if code := runClose(h, dir); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	assertCalls(t, h.ctl, agtermtest.Call{Method: "Close", Args: []string{"C-1"}})

	h = newHarness(t, true)
	h.ctl.CloseErr = errors.Join(agterm.ErrGone, errors.New("no such session"))
	if code := runClose(h, dir); code != 0 || h.err.Len() != 0 {
		t.Fatalf("gone: exit %d, %q", code, h.err)
	}

	h = newHarness(t, true)
	_, inline := sendDir(t, h, "", false)
	if code := runClose(h, inline); code != 0 || len(h.ctl.Calls) != 0 {
		t.Fatalf("no companion: exit %d, calls %v", code, h.ctl.Calls)
	}

	h = newHarness(t, true)
	h.ctl.CloseErr = errors.New("socket refused")
	if code := runClose(h, dir); code != 1 || !strings.Contains(h.err.String(), "socket refused") {
		t.Fatalf("error: exit %d, %q", code, h.err)
	}
	if code := runClose(h, filepath.Dir(dir)); code != 1 {
		t.Fatalf("bad dir: exit %d", code)
	}
}
