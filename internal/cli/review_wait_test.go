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
	"github.com/shurastik/agterm-kit/internal/rundir"
)

const (
	completedResult = `{"status":"completed","round":1,"findings":3,"verdict":"needs-attention"}`
	blockedResult   = `{"status":"blocked","round":1,"reason":"codex failed","findings":0}`
)

// waitDir makes a run dir with an optional companion file and result.json.
func waitDir(t *testing.T, companion, result string) string {
	t.Helper()
	_, dir := runDir(t)
	if companion != "" {
		if err := os.WriteFile(filepath.Join(dir, "companion"), []byte(companion), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if result != "" {
		writeResultFile(t, dir, result)
	}
	return dir
}

func writeResultFile(t *testing.T, dir, result string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(rundir.RoundDir(dir, 1), "result.json"), []byte(result+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func runWait(h *tenv, args ...string) int {
	return h.env.Main(context.Background(), append([]string{"review", "wait"}, args...))
}

func assertCalls(t *testing.T, f *agtermtest.Fake, want ...agtermtest.Call) {
	t.Helper()
	if len(want) == 0 && len(f.Calls) == 0 {
		return
	}
	if !reflect.DeepEqual(f.Calls, want) {
		t.Fatalf("calls = %v, want %v", f.Calls, want)
	}
}

var (
	waitCall = agtermtest.Call{Method: "Wait", Args: []string{"C-1", "completed,blocked", "30m0s"}}
)

func TestReviewWaitCompleted(t *testing.T) {
	h := newHarness(t, true)
	dir := waitDir(t, "C-1\n", "")
	h.ctl.WaitStatus = "completed"
	// The review finishes while agk waits.
	h.ctl.OnWait = func() { writeResultFile(t, dir, completedResult) }
	if code := runWait(h, dir); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	if h.out.String() != completedResult+"\n" {
		t.Fatalf("stdout = %q", h.out)
	}
	assertCalls(t, h.ctl, waitCall) // the reviewer stays for the next round
	if h.err.Len() != 0 {
		t.Fatalf("stderr = %q", h.err)
	}
}

func TestReviewWaitBlockedKeepsCompanion(t *testing.T) {
	h := newHarness(t, true)
	dir := waitDir(t, "C-1\n", "")
	h.ctl.WaitStatus = "blocked"
	h.ctl.OnWait = func() { writeResultFile(t, dir, blockedResult) }
	if code := runWait(h, dir); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	if h.out.String() != blockedResult+"\n" {
		t.Fatalf("stdout = %q", h.out)
	}
	assertCalls(t, h.ctl, waitCall)
}

func TestReviewWaitBlockedWithoutResult(t *testing.T) {
	h := newHarness(t, true)
	dir := waitDir(t, "C-1\n", "")
	h.ctl.WaitStatus = "blocked"
	if code := runWait(h, dir); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	if h.out.String() != `{"status":"blocked","round":1}`+"\n" {
		t.Fatalf("stdout = %q", h.out)
	}
	assertCalls(t, h.ctl, waitCall)
}

func TestReviewWaitCompletedWithoutResultIsAnError(t *testing.T) {
	h := newHarness(t, true)
	dir := waitDir(t, "C-1\n", "")
	h.ctl.WaitStatus = "completed"
	if code := runWait(h, dir); code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
	if h.out.Len() != 0 || !strings.Contains(h.err.String(), "result.json") {
		t.Fatalf("stdout %q stderr %q", h.out, h.err)
	}
	assertCalls(t, h.ctl, waitCall)
}

func TestReviewWaitCancelled(t *testing.T) {
	h := newHarness(t, true)
	dir := waitDir(t, "C-1\n", "")
	h.ctl.WaitErr = agterm.ErrGone
	if code := runWait(h, dir); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	if h.out.String() != `{"status":"cancelled","round":1}`+"\n" {
		t.Fatalf("stdout = %q", h.out)
	}
	assertCalls(t, h.ctl, waitCall)
}

func TestReviewWaitRunning(t *testing.T) {
	h := newHarness(t, true)
	dir := waitDir(t, "C-1\n", "")
	h.ctl.WaitErr = agterm.ErrTimeout
	if code := runWait(h, dir, "--timeout", "60"); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	if h.out.String() != `{"status":"running","round":1}`+"\n" {
		t.Fatalf("stdout = %q", h.out)
	}
	assertCalls(t, h.ctl, agtermtest.Call{Method: "Wait", Args: []string{"C-1", "completed,blocked", "1m0s"}})
}

func TestReviewWaitTimeoutButCompleted(t *testing.T) {
	h := newHarness(t, true)
	dir := waitDir(t, "C-1\n", "")
	h.ctl.WaitErr = agterm.ErrTimeout
	h.ctl.OnWait = func() { writeResultFile(t, dir, completedResult) }
	if code := runWait(h, "--timeout", "60", dir); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	if h.out.String() != completedResult+"\n" {
		t.Fatalf("stdout = %q", h.out)
	}
	if got := h.ctl.Methods(); !reflect.DeepEqual(got, []string{"Wait"}) {
		t.Fatalf("calls = %v", h.ctl.Calls)
	}
}

func TestReviewWaitGoneButCompletedDoesNotClose(t *testing.T) {
	h := newHarness(t, true)
	dir := waitDir(t, "C-1\n", "")
	h.ctl.WaitErr = agterm.ErrGone
	h.ctl.OnWait = func() { writeResultFile(t, dir, completedResult) }
	if code := runWait(h, dir); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	if h.out.String() != completedResult+"\n" {
		t.Fatalf("stdout = %q", h.out)
	}
	assertCalls(t, h.ctl, waitCall)
}

func TestReviewWaitResultAlreadyThere(t *testing.T) {
	h := newHarness(t, true)
	dir := waitDir(t, "C-1\n", completedResult)
	if code := runWait(h, dir); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	if h.out.String() != completedResult+"\n" {
		t.Fatalf("stdout = %q", h.out)
	}
	// No wait: a completed status agterm already reset would block it.
	assertCalls(t, h.ctl)
}

func TestReviewWaitBlockedResultAlreadyThere(t *testing.T) {
	h := newHarness(t, true)
	dir := waitDir(t, "C-1\n", blockedResult)
	if code := runWait(h, dir); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	assertCalls(t, h.ctl)
}

func TestReviewWaitUserClosedFinishedCompanion(t *testing.T) {
	h := newHarness(t, true)
	dir := waitDir(t, "C-1\n", completedResult)
	h.ctl.CloseErr = errors.Join(agterm.ErrGone, errors.New("no such session: C-1"))
	if code := runWait(h, dir); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	if h.out.String() != completedResult+"\n" || h.err.Len() != 0 {
		t.Fatalf("stdout %q stderr %q", h.out, h.err)
	}
}

func TestReviewWaitWaitError(t *testing.T) {
	h := newHarness(t, true)
	dir := waitDir(t, "C-1\n", "")
	h.ctl.WaitErr = errors.New("socket refused")
	if code := runWait(h, dir); code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
	if h.out.Len() != 0 || !strings.Contains(h.err.String(), "socket refused") {
		t.Fatalf("stdout %q stderr %q", h.out, h.err)
	}
}

func TestReviewWaitInline(t *testing.T) {
	// Outside agterm too: the inline path never touches agterm.
	h := newHarness(t, false)
	dir := waitDir(t, "", completedResult)
	if code := runWait(h, dir); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	if h.out.String() != completedResult+"\n" {
		t.Fatalf("stdout = %q", h.out)
	}
	assertCalls(t, h.ctl)
}

func TestReviewWaitInlineWithoutResult(t *testing.T) {
	h := newHarness(t, true)
	dir := waitDir(t, "", "")
	if code := runWait(h, dir); code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
	if h.out.Len() != 0 || h.err.Len() == 0 {
		t.Fatalf("stdout %q stderr %q", h.out, h.err)
	}
	assertCalls(t, h.ctl)
}

func TestReviewWaitRejectsBadInput(t *testing.T) {
	cases := map[string]struct {
		companion, result string
		args              func(dir string) []string
	}{
		"no args":            {args: func(string) []string { return nil }},
		"two dirs":           {args: func(d string) []string { return []string{d, d} }},
		"not a run dir":      {args: func(d string) []string { return []string{filepath.Dir(d)} }},
		"missing dir":        {args: func(d string) []string { return []string{d + "-gone"} }},
		"zero timeout":       {args: func(d string) []string { return []string{d, "--timeout", "0"} }},
		"negative timeout":   {args: func(d string) []string { return []string{d, "--timeout", "-5"} }},
		"word timeout":       {args: func(d string) []string { return []string{d, "--timeout", "soon"} }},
		"empty companion":    {companion: "\n", args: func(d string) []string { return []string{d} }},
		"two-word companion": {companion: "C-1 C-2\n", args: func(d string) []string { return []string{d} }},
		"flag companion":     {companion: "--undoable\n", args: func(d string) []string { return []string{d} }},
		"bad result json":    {companion: "C-1\n", result: "nope", args: func(d string) []string { return []string{d} }},
		"unknown status":     {companion: "C-1\n", result: `{"status":"done"}`, args: func(d string) []string { return []string{d} }},
		"flat run dir":       {companion: "C-1\n", args: func(d string) []string { os.Remove(filepath.Join(d, rundir.RoundFile)); return []string{d} }},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t, true)
			dir := waitDir(t, c.companion, c.result)
			if code := runWait(h, c.args(dir)...); code != 1 {
				t.Fatalf("exit %d, want 1 (stderr %q)", code, h.err)
			}
			if h.out.Len() != 0 || h.err.Len() == 0 {
				t.Fatalf("stdout %q stderr %q", h.out, h.err)
			}
			assertCalls(t, h.ctl)
		})
	}
}

func TestReviewHelpListsWait(t *testing.T) {
	h := newHarness(t, true)
	h.env.Main(context.Background(), []string{"review", "--help"})
	h.env.Main(context.Background(), []string{"--help"})
	if strings.Count(h.out.String(), "agk review wait <run-dir>") != 2 {
		t.Fatalf("help = %q", h.out)
	}
}

// round2Dir is a run at round 2 whose round 1 finished, with companion C-1.
func round2Dir(t *testing.T) string {
	t.Helper()
	dir := waitDir(t, "C-1\n", completedResult)
	rd, _ := rundir.MakeRound(dir, 2)
	os.WriteFile(filepath.Join(rd, "prompt.md"), []byte("R2"), 0o644)
	rundir.SetRound(dir, 2)
	return dir
}

// After `review send`, the companion may still show round 1's completed: the
// wait must not take it for round 2's outcome.
func TestReviewWaitIgnoresTheLastRoundsStatus(t *testing.T) {
	h := newHarness(t, true)
	dir := round2Dir(t)
	h.ctl.WaitSeq = []agtermtest.WaitResult{{Status: "active"}, {Status: "completed"}}
	n := 0
	h.ctl.OnWait = func() {
		if n++; n == 2 {
			os.WriteFile(filepath.Join(rundir.RoundDir(dir, 2), "result.json"), []byte(`{"status":"completed","round":2,"findings":0,"verdict":"approve"}`), 0o644)
		}
	}
	if code := runWait(h, dir); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	if !strings.Contains(h.out.String(), `"round":2`) || !strings.Contains(h.out.String(), "approve") {
		t.Fatalf("stdout = %q", h.out)
	}
	assertCalls(t, h.ctl,
		agtermtest.Call{Method: "Wait", Args: []string{"C-1", "active", "30m0s"}},
		agtermtest.Call{Method: "Wait", Args: []string{"C-1", "completed,blocked", "30m0s"}},
	)
}

func TestReviewWaitRoundNeverStarts(t *testing.T) {
	h := newHarness(t, true)
	dir := round2Dir(t)
	h.ctl.WaitErr = agterm.ErrTimeout
	if code := runWait(h, dir, "--timeout", "60"); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	if h.out.String() != `{"status":"running","round":2}`+"\n" {
		t.Fatalf("stdout = %q", h.out)
	}
	assertCalls(t, h.ctl, agtermtest.Call{Method: "Wait", Args: []string{"C-1", "active", "1m0s"}})
}

func TestReviewWaitStartedRoundTrustsStatus(t *testing.T) {
	h := newHarness(t, true)
	dir := round2Dir(t)
	os.WriteFile(filepath.Join(rundir.RoundDir(dir, 2), startedFile), nil, 0o644)
	h.ctl.WaitStatus = "blocked"
	if code := runWait(h, dir); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	if h.out.String() != `{"status":"blocked","round":2}`+"\n" {
		t.Fatalf("stdout = %q", h.out)
	}
	assertCalls(t, h.ctl, waitCall)
}
