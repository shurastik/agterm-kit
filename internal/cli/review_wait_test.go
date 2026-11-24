package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/shurastik/agterm-kit/internal/agterm"
	"github.com/shurastik/agterm-kit/internal/agterm/agtermtest"
	"github.com/shurastik/agterm-kit/internal/config"
	"github.com/shurastik/agterm-kit/internal/rundir"
)

const (
	completedResult = `{"status":"completed","round":1,"findings":3,"verdict":"needs-attention"}`
	blockedResult   = `{"status":"blocked","round":1,"reason":"codex failed","findings":0}`
)

// waitDir makes a (non-interactive) run dir with an optional companion file
// and result.json.
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

// idir makes an interactive review at round 1 with companion C-1, out/
// and schema.json, as `review start` leaves it. The round counts as started.
func idir(t *testing.T) string {
	t.Helper()
	_, dir := runDir(t)
	if err := rundir.WriteJSON(dir, workerFile, reviewState{Worker: config.Worker{Harness: config.Codex}, Base: "main", Interactive: true}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "companion"), []byte("C-1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := prepareOut(dir); err != nil {
		t.Fatal(err)
	}
	markStarted(dir, 1)
	return dir
}

func writeOut(t *testing.T, dir string, k int, content string) {
	t.Helper()
	if err := os.WriteFile(outputPath(dir, k), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// noStatus fails when agk set the reviewer companion's agent status: it
// belongs to the harness.
func noStatus(t *testing.T, h *tenv) {
	t.Helper()
	for _, c := range h.ctl.Calls {
		if c.Method == "Status" {
			t.Fatalf("agk set an agent status: %v", h.ctl.Calls)
		}
	}
}

func infos(h *tenv) []string {
	var out []string
	for _, c := range h.ctl.Calls {
		if c.Method == "Info" {
			out = append(out, c.Args[1])
		}
	}
	return out
}

func typed(h *tenv) []string {
	var out []string
	for _, c := range h.ctl.Calls {
		if c.Method == "Type" {
			out = append(out, c.Args[1])
		}
	}
	return out
}

func mainWait(timeout string) agtermtest.Call {
	return agtermtest.Call{Method: "Wait", Args: []string{"C-1", "completed,blocked,idle", timeout}}
}

func TestReviewWaitValidOutputMidTurnCompletes(t *testing.T) {
	h := newHarness(t, true)
	dir := idir(t)
	h.ctl.WaitStatus, h.ctl.WaitErr = "", agterm.ErrTimeout // the turn keeps running
	h.ctl.OnWait = func() { writeOut(t, dir, 1, threeFindings) }
	if code := runWait(h, dir); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	if h.out.String() != completedResult+"\n" {
		t.Fatalf("stdout = %q", h.out)
	}
	rd := rundir.RoundDir(dir, 1)
	assertResult(t, rd, "completed", 3)
	if !exists(filepath.Join(rd, "review.json")) {
		t.Fatal("review.json missing")
	}
	assertCalls(t, h.ctl,
		mainWait("30m0s"),
		agtermtest.Call{Method: "Info", Args: []string{"C-1", "codex r1: **3** findings (^[1 high](color: \"orange\"))"}},
	)
	noStatus(t, h)
}

func TestReviewWaitValidOutputAlreadyThereNeedsNoWait(t *testing.T) {
	h := newHarness(t, true)
	dir := idir(t)
	writeOut(t, dir, 1, approve)
	if code := runWait(h, dir); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	if h.out.String() != `{"status":"completed","round":1,"findings":0,"verdict":"approve"}`+"\n" {
		t.Fatalf("stdout = %q", h.out)
	}
	// agterm may have reset a completed status: waiting would hang.
	if got := h.ctl.Methods(); !reflect.DeepEqual(got, []string{"Info"}) {
		t.Fatalf("calls = %v", h.ctl.Calls)
	}
	if got := infos(h); len(got) != 1 || got[0] != `codex r1: ^[approve](color: "green"), no findings` {
		t.Fatalf("infos = %q", got)
	}
}

func TestReviewWaitCompletedTurnWithValidOutput(t *testing.T) {
	h := newHarness(t, true)
	dir := idir(t)
	h.ctl.WaitStatus = "completed"
	h.ctl.OnWait = func() { writeOut(t, dir, 1, threeFindings) }
	if code := runWait(h, dir); code != 0 || h.out.String() != completedResult+"\n" {
		t.Fatalf("exit %d stdout %q stderr %q", code, h.out, h.err)
	}
	noStatus(t, h)
}

func TestReviewWaitInvalidOutputGetsOneFixupThenCompletes(t *testing.T) {
	h := newHarness(t, true)
	dir := idir(t)
	writeOut(t, dir, 1, `{"verdict":"maybe"}`)
	rd := rundir.RoundDir(dir, 1)
	h.ctl.WaitSeq = []agtermtest.WaitResult{
		{Status: "completed"}, // the turn ends with an invalid file
		{Status: "idle"},      // typeWhenIdle: the turn is over
		{Status: "active"},    // the fix-up turn starts
	}
	h.ctl.OnType = func(_, _ string) { writeOut(t, dir, 1, threeFindings) }
	if code := runWait(h, dir); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	if h.out.String() != completedResult+"\n" {
		t.Fatalf("stdout = %q", h.out)
	}
	fix := filepath.Join(rd, "fixup-1.md")
	if got := typed(h); !reflect.DeepEqual(got, []string{"Read " + fix + " and follow it.\r"}) {
		t.Fatalf("typed = %q", got)
	}
	p := readFile(t, fix)
	if !strings.Contains(p, "verdict") || !strings.Contains(p, outputPath(dir, 1)) || !strings.Contains(p, filepath.Join(dir, "schema.json")) {
		t.Fatalf("fix-up prompt = %q", p)
	}
	if readFile(t, filepath.Join(rd, "fixups")) != "1\n" || !exists(fix+".started") {
		t.Fatal("the attempt and its start must be recorded")
	}
	noStatus(t, h)
}

// A fix-up line that codex keeps unsent in its input box gets one Enter of its
// own, and the attempt is recorded as started once active arrives.
func TestReviewWaitFixupLineNotPickedUpGetsOneEnter(t *testing.T) {
	h := newHarness(t, true)
	dir := idir(t)
	writeOut(t, dir, 1, `{"verdict":"maybe"}`)
	rd := rundir.RoundDir(dir, 1)
	fix := filepath.Join(rd, "fixup-1.md")
	h.ctl.TextOut = "› Read " + fix + " and\n  follow it.\n" // the composer still holds the line
	h.ctl.WaitSeq = []agtermtest.WaitResult{
		{Status: "completed"},    // the turn ends with an invalid file
		{Status: "idle"},         // typeWhenIdle: the turn is over
		{Err: agterm.ErrTimeout}, // the first wait: the line is not picked up
		{Status: "idle"},         // current status
		{Status: "active"},       // the Enter submits it
	}
	h.ctl.OnType = func(_, text string) {
		if text == "\r" {
			writeOut(t, dir, 1, threeFindings)
		}
	}
	if code := runWait(h, dir); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	if h.out.String() != completedResult+"\n" {
		t.Fatalf("stdout = %q", h.out)
	}
	if got := typed(h); !reflect.DeepEqual(got, []string{"Read " + fix + " and follow it.\r", "\r"}) {
		t.Fatalf("typed = %q: the line, then one Enter alone", got)
	}
	if !exists(fixupStartedFile(rd, 1)) || !roundStarted(dir, 1) {
		t.Fatal("the fix-up must be recorded as started once active arrives")
	}
	if h.err.Len() != 0 {
		t.Fatalf("stderr = %q: a recovered fix-up warns about nothing", h.err)
	}
	noStatus(t, h)
}

func TestReviewWaitThreeInvalidOutputsBlockTheRound(t *testing.T) {
	h := newHarness(t, true)
	dir := idir(t)
	const bad = `{"verdict":"maybe"}`
	writeOut(t, dir, 1, bad)
	rd := rundir.RoundDir(dir, 1)
	h.ctl.WaitStatus = "completed" // every turn ends at once; every fix-up turn starts
	h.ctl.WaitSeq = []agtermtest.WaitResult{
		{Status: "completed"},                                       // turn 1 over
		{Status: "idle"}, {Status: "active"}, {Status: "completed"}, // fix-up 1 typed, started, over
		{Status: "idle"}, {Status: "active"}, // fix-up 2 typed, started
	}
	if code := runWait(h, dir); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	want := `{"status":"blocked","round":1,"reason":"invalid review output","findings":0}` + "\n"
	if h.out.String() != want {
		t.Fatalf("stdout = %q", h.out)
	}
	if got := len(typed(h)); got != 2 {
		t.Fatalf("fix-ups typed = %d, want 2", got)
	}
	if readFile(t, filepath.Join(rd, "output.json")) != bad {
		t.Fatal("the raw output must be kept")
	}
	if exists(filepath.Join(rd, "review.json")) {
		t.Fatal("review.json must not be written")
	}
	assertResult(t, rd, "blocked", 0)
	if got := infos(h); !reflect.DeepEqual(got, []string{"codex r1: invalid review output"}) {
		t.Fatalf("infos = %q", got)
	}
	noStatus(t, h)
}

func TestReviewWaitInvalidFileOutlastingTimeoutGivesRunning(t *testing.T) {
	h := newHarness(t, true)
	dir := idir(t)
	writeOut(t, dir, 1, `{"verdict": "needs-att`) // half written, the turn keeps running
	pollSlice = 5 * 1e9
	h.ctl.WaitErr = agterm.ErrTimeout
	if code := runWait(h, dir, "--timeout", "12"); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	if h.out.String() != `{"status":"running","round":1}`+"\n" {
		t.Fatalf("stdout = %q", h.out)
	}
	assertCalls(t, h.ctl, mainWait("5s"), mainWait("5s"), mainWait("2s"))
	if exists(filepath.Join(rundir.RoundDir(dir, 1), "result.json")) || len(typed(h)) != 0 {
		t.Fatal("nothing may be recorded or typed")
	}
}

func TestReviewWaitNoOutputNeedsHuman(t *testing.T) {
	h := newHarness(t, true)
	dir := idir(t)
	h.ctl.WaitStatus = "completed"
	if code := runWait(h, dir); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	if h.out.String() != `{"status":"needs-human","round":1,"reason":"no-output"}`+"\n" {
		t.Fatalf("stdout = %q", h.out)
	}
	if exists(filepath.Join(rundir.RoundDir(dir, 1), "result.json")) {
		t.Fatal("no result.json for needs-human")
	}
	noStatus(t, h)
}

func TestReviewWaitPermissionNeedsHuman(t *testing.T) {
	h := newHarness(t, true)
	dir := idir(t)
	h.ctl.WaitStatus = "blocked"
	if code := runWait(h, dir); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	if h.out.String() != `{"status":"needs-human","round":1,"reason":"permission"}`+"\n" {
		t.Fatalf("stdout = %q", h.out)
	}
	if exists(filepath.Join(rundir.RoundDir(dir, 1), "result.json")) {
		t.Fatal("no result.json for needs-human")
	}
	assertCalls(t, h.ctl, mainWait("30m0s"))
}

func TestReviewWaitSecondWaitAfterNeedsHumanCompletes(t *testing.T) {
	h := newHarness(t, true)
	dir := idir(t)
	h.ctl.WaitStatus = "completed"
	runWait(h, dir)
	if !strings.Contains(h.out.String(), "needs-human") {
		t.Fatalf("stdout = %q", h.out)
	}
	// The user talks the reviewer into writing its review.
	h.out.Reset()
	h.ctl.Calls = nil
	writeOut(t, dir, 1, threeFindings)
	if code := runWait(h, dir); code != 0 || h.out.String() != completedResult+"\n" {
		t.Fatalf("exit %d stdout %q", code, h.out)
	}
	assertResult(t, rundir.RoundDir(dir, 1), "completed", 3)
}

// fixup1Unconfirmed makes a run whose fix-up 1 was typed without `active`
// being seen afterwards, over an invalid file.
func fixup1Unconfirmed(t *testing.T, content string) (dir string) {
	t.Helper()
	dir = idir(t)
	writeOut(t, dir, 1, content)
	rd := rundir.RoundDir(dir, 1)
	if err := os.WriteFile(filepath.Join(rd, "fixups"), []byte("1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rd, "fixup-1.md"), []byte("fix"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

var activeBlocked = func(timeout string) agtermtest.Call {
	return agtermtest.Call{Method: "Wait", Args: []string{"C-1", "active,blocked", timeout}}
}

func TestReviewWaitUnconfirmedFixupTypesNoSecondOne(t *testing.T) {
	h := newHarness(t, true)
	dir := idir(t)
	writeOut(t, dir, 1, `{"verdict":"maybe"}`)
	// The first wait types fix-up 1; its handshake times out.
	h.ctl.WaitSeq = []agtermtest.WaitResult{{Status: "completed"}, {Status: "idle"}, {Err: agterm.ErrTimeout}}
	h.ctl.WaitErr = agterm.ErrTimeout
	if code := runWait(h, dir, "--timeout", "60"); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	if h.out.String() != `{"status":"running","round":1}`+"\n" || len(typed(h)) != 1 {
		t.Fatalf("stdout %q typed %q", h.out, typed(h))
	}
	rd := rundir.RoundDir(dir, 1)
	if exists(filepath.Join(rd, "fixup-1.md.started")) {
		t.Fatal("an unconfirmed fix-up must not be marked started")
	}
	// A new wait sees the old `completed` and the unchanged file: no second
	// fix-up, it waits for `active` and gives running at the timeout.
	h.out.Reset()
	h.ctl.Calls = nil
	h.ctl.WaitErr = agterm.ErrTimeout
	if code := runWait(h, dir, "--timeout", "60"); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	if h.out.String() != `{"status":"running","round":1}`+"\n" {
		t.Fatalf("stdout = %q", h.out)
	}
	assertCalls(t, h.ctl, activeBlocked("1m0s"))
	if exists(filepath.Join(rd, "result.json")) {
		t.Fatal("no outcome may be recorded")
	}
	noStatus(t, h)
}

func TestReviewWaitUnconfirmedFixupThenActiveOpensTheGate(t *testing.T) {
	h := newHarness(t, true)
	dir := fixup1Unconfirmed(t, `{"verdict":"maybe"}`)
	h.ctl.WaitSeq = []agtermtest.WaitResult{
		{Status: "active"},                   // the fix-up turn starts (gate opens)
		{Status: "completed"},                // and ends with the file still invalid
		{Status: "idle"}, {Status: "active"}, // fix-up 2 typed and started
		{Status: "completed"}, // fix-up 2 turn over, still invalid
	}
	if code := runWait(h, dir); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	rd := rundir.RoundDir(dir, 1)
	if !exists(filepath.Join(rd, "fixup-1.md.started")) {
		t.Fatal("fix-up 1 must be marked started once active is seen")
	}
	if got := len(typed(h)); got != 1 || !exists(filepath.Join(rd, "fixup-2.md")) {
		t.Fatalf("typed %d, want exactly fix-up 2", got)
	}
	if !strings.Contains(h.out.String(), `"status":"blocked"`) {
		t.Fatalf("stdout = %q", h.out)
	}
}

func TestReviewWaitHalfRewrittenFileAfterUnconfirmedFixup(t *testing.T) {
	h := newHarness(t, true)
	dir := fixup1Unconfirmed(t, `{"verdict":"maybe"}`)
	pollSlice = 5 * 1e9
	h.ctl.WaitErr = agterm.ErrTimeout
	n := 0
	h.ctl.OnWait = func() {
		switch n++; n {
		case 1:
			writeOut(t, dir, 1, `{"verdict":"needs-attention","summ`) // half-rewritten
		case 2:
			writeOut(t, dir, 1, threeFindings)
		}
	}
	if code := runWait(h, dir, "--timeout", "60"); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	if h.out.String() != completedResult+"\n" || len(typed(h)) != 0 {
		t.Fatalf("stdout %q typed %q", h.out, typed(h))
	}
	assertCalls(t, h.ctl, activeBlocked("5s"), activeBlocked("5s"),
		agtermtest.Call{Method: "Info", Args: []string{"C-1", "codex r1: **3** findings (^[1 high](color: \"orange\"))"}})
}

func TestReviewWaitPermissionDuringUnconfirmedFixup(t *testing.T) {
	h := newHarness(t, true)
	dir := fixup1Unconfirmed(t, `{"verdict":"maybe"}`)
	h.ctl.WaitStatus = "blocked"
	if code := runWait(h, dir); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	if h.out.String() != `{"status":"needs-human","round":1,"reason":"permission"}`+"\n" {
		t.Fatalf("stdout = %q", h.out)
	}
	if len(typed(h)) != 0 || exists(filepath.Join(rundir.RoundDir(dir, 1), "result.json")) {
		t.Fatal("nothing may be typed or recorded")
	}
}

func TestReviewWaitFixupRefusedIsTakenBack(t *testing.T) {
	h := newHarness(t, true)
	dir := idir(t)
	writeOut(t, dir, 1, `{"verdict":"maybe"}`)
	h.ctl.WaitSeq = []agtermtest.WaitResult{{Status: "completed"}, {Status: "blocked"}} // a prompt opens before typing
	if code := runWait(h, dir); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	if h.out.String() != `{"status":"needs-human","round":1,"reason":"permission"}`+"\n" {
		t.Fatalf("stdout = %q", h.out)
	}
	rd := rundir.RoundDir(dir, 1)
	if n, _ := readFixups(rd); n != 0 || exists(filepath.Join(rd, "fixup-1.md")) || len(typed(h)) != 0 {
		t.Fatalf("a refused fix-up must leave no trace (count %d)", n)
	}
}

func TestReviewWaitCancelledAndRunning(t *testing.T) {
	h := newHarness(t, true)
	dir := idir(t)
	h.ctl.WaitErr = agterm.ErrGone
	if code := runWait(h, dir); code != 0 || h.out.String() != `{"status":"cancelled","round":1}`+"\n" {
		t.Fatalf("exit %d stdout %q", code, h.out)
	}
	h = newHarness(t, true)
	h.ctl.WaitErr = agterm.ErrTimeout
	if code := runWait(h, dir, "--timeout", "60"); code != 0 || h.out.String() != `{"status":"running","round":1}`+"\n" {
		t.Fatalf("exit %d stdout %q", code, h.out)
	}
	assertCalls(t, h.ctl, mainWait("1m0s"))
}

func TestReviewWaitInfoLineHoldsNoReviewerText(t *testing.T) {
	h := newHarness(t, true)
	dir := idir(t)
	evil := strings.Replace(threeFindings, `"title":"a"`, `"title":"\u001b]52;c;ZXZpbA==\u0007 clipboard"`, 1)
	writeOut(t, dir, 1, evil)
	if code := runWait(h, dir); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	for _, s := range append(infos(h), h.out.String(), h.err.String()) {
		if strings.ContainsAny(s, "\x1b\x07") || strings.Contains(s, "clipboard") {
			t.Fatalf("reviewer text leaked: %q", s)
		}
	}
}

func TestReviewWaitRoundStartGate(t *testing.T) {
	// Round 1 whose start was never confirmed: a fresh session's idle must not
	// read as "turn over"; the wait begins at active, then waits for the end.
	h := newHarness(t, true)
	dir := idir(t)
	os.Remove(filepath.Join(rundir.RoundDir(dir, 1), startedFile))
	h.ctl.WaitSeq = []agtermtest.WaitResult{{Status: "active"}, {Status: "completed"}}
	h.ctl.OnWait = func() {
		if len(h.ctl.Calls) == 2 {
			writeOut(t, dir, 1, threeFindings)
		}
	}
	if code := runWait(h, dir); code != 0 || h.out.String() != completedResult+"\n" {
		t.Fatalf("exit %d stdout %q stderr %q", code, h.out, h.err)
	}
	if !roundStarted(dir, 1) {
		t.Fatal("seeing active confirms the round")
	}
	assertCalls(t, h.ctl,
		agtermtest.Call{Method: "Wait", Args: []string{"C-1", "active,blocked,completed", "30m0s"}},
		mainWait("30m0s"),
		agtermtest.Call{Method: "Info", Args: []string{"C-1", "codex r1: **3** findings (^[1 high](color: \"orange\"))"}},
	)
}

// After `review send`, the companion may still show round 1's completed: the
// wait must not take it for round 2's outcome.
func TestReviewWaitSecondRoundIgnoresTheLastRoundsStatus(t *testing.T) {
	h := newHarness(t, true)
	dir := idir(t)
	writeOut(t, dir, 1, threeFindings)
	writeResultFile(t, dir, completedResult)
	rd, _ := rundir.MakeRound(dir, 2)
	os.WriteFile(filepath.Join(rd, "prompt.md"), []byte("R2"), 0o644)
	rundir.SetRound(dir, 2)
	h.ctl.WaitErr = agterm.ErrTimeout
	if code := runWait(h, dir, "--timeout", "60"); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	if h.out.String() != `{"status":"running","round":2}`+"\n" {
		t.Fatalf("stdout = %q", h.out)
	}
	assertCalls(t, h.ctl, activeBlocked("1m0s"))
}

func TestReviewWaitOtherWaitErrorExits1(t *testing.T) {
	h := newHarness(t, true)
	dir := idir(t)
	h.ctl.WaitErr = errors.New("socket refused")
	if code := runWait(h, dir); code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
	if h.out.Len() != 0 || !strings.Contains(h.err.String(), "socket refused") {
		t.Fatalf("stdout %q stderr %q", h.out, h.err)
	}
}

func TestReviewWaitResultAlreadyThere(t *testing.T) {
	for name, res := range map[string]string{"completed": completedResult, "blocked": blockedResult} {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t, true)
			dir := idir(t)
			writeResultFile(t, dir, res)
			if code := runWait(h, dir); code != 0 || h.out.String() != res+"\n" {
				t.Fatalf("exit %d stdout %q", code, h.out)
			}
			assertCalls(t, h.ctl) // result.json decides: no wait
		})
	}
}

// 5.3: a run started by an older agk (companion, no interactive flag).
func TestReviewWaitOlderRunWithResult(t *testing.T) {
	h := newHarness(t, true)
	dir := waitDir(t, "C-1\n", completedResult)
	if code := runWait(h, dir); code != 0 || h.out.String() != completedResult+"\n" {
		t.Fatalf("exit %d stdout %q", code, h.out)
	}
	assertCalls(t, h.ctl)
}

func TestReviewWaitOlderRunWithoutResult(t *testing.T) {
	h := newHarness(t, true)
	dir := waitDir(t, "C-1\n", "")
	if code := runWait(h, dir); code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
	if h.out.Len() != 0 || !strings.Contains(h.err.String(), "older agk") || !strings.Contains(h.err.String(), "new review") {
		t.Fatalf("stdout %q stderr %q", h.out, h.err)
	}
	assertCalls(t, h.ctl) // no Ctl wait call
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

// A lost info-line update is retried, and the outcome is on disk regardless.
func TestReviewWaitRetriesInfoAndRecordsResult(t *testing.T) {
	h := newHarness(t, true)
	dir := idir(t)
	writeOut(t, dir, 1, threeFindings)
	h.ctl.InfoErr = errors.New("socket hiccup")
	if code := runWait(h, dir); code != 0 || h.out.String() != completedResult+"\n" {
		t.Fatalf("exit %d stdout %q", code, h.out)
	}
	if got := len(infos(h)); got != 3 {
		t.Fatalf("info attempts = %d, want 3", got)
	}
	assertResult(t, rundir.RoundDir(dir, 1), "completed", 3)
	noStatus(t, h)
}

// ticking makes h.env.Now a controllable clock and every scripted Wait cost
// the time it was asked to wait when it times out, or cost(call) otherwise.
func ticking(t *testing.T, h *tenv, cost func(timeout time.Duration, err error) time.Duration) *time.Time {
	t.Helper()
	now := time.Date(2026, 11, 5, 10, 0, 0, 0, time.Local)
	h.env.Now = func() time.Time { return now }
	waits := 0
	h.ctl.OnWait = func() {
		c := h.ctl.Calls[len(h.ctl.Calls)-1]
		d, err := time.ParseDuration(c.Args[2])
		if err != nil {
			t.Fatal(err)
		}
		var werr error
		if waits < len(h.ctl.WaitSeq) {
			werr = h.ctl.WaitSeq[0].Err // OnWait runs before the pop
		} else {
			werr = h.ctl.WaitErr
		}
		waits++
		now = now.Add(cost(d, werr))
	}
	return &now
}

func waitCost(d time.Duration, err error) time.Duration {
	if errors.Is(err, agterm.ErrTimeout) {
		return d
	}
	return 0
}

// One deadline for the whole wait: a finished turn with invalid output and a
// fix-up that never shows `active` must not run past --timeout.
func TestReviewWaitFixupHandshakeIsCappedByTheDeadline(t *testing.T) {
	h := newHarness(t, true)
	dir := idir(t)
	writeOut(t, dir, 1, `{"verdict":"maybe"}`)
	h.ctl.WaitSeq = []agtermtest.WaitResult{{Status: "completed"}, {Status: "idle"}, {Err: agterm.ErrTimeout}}
	h.ctl.WaitErr = agterm.ErrTimeout
	now := ticking(t, h, waitCost)
	start := *now
	if code := runWait(h, dir, "--timeout", "1"); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	if h.out.String() != `{"status":"running","round":1}`+"\n" {
		t.Fatalf("stdout = %q", h.out)
	}
	if spent := now.Sub(start); spent > time.Second {
		t.Fatalf("the wait spent %v of a 1 s budget", spent)
	}
	for _, c := range h.ctl.Calls {
		if c.Method == "Wait" {
			if d, _ := time.ParseDuration(c.Args[2]); d > time.Second {
				t.Fatalf("a wait asked for %v: %v", d, c)
			}
		}
	}
	if len(typed(h)) != 1 {
		t.Fatalf("typed = %q", typed(h))
	}
}

// A status wait that took time leaves less for the next one.
func TestReviewWaitSuccessfulWaitsSpendTheBudget(t *testing.T) {
	h := newHarness(t, true)
	dir := idir(t)
	os.Remove(filepath.Join(rundir.RoundDir(dir, 1), startedFile))
	// The first wait returns `active` after 40 s.
	h.ctl.WaitSeq = []agtermtest.WaitResult{{Status: "active"}}
	h.ctl.WaitErr = agterm.ErrTimeout
	n := 0
	ticking(t, h, func(d time.Duration, err error) time.Duration {
		if n++; n == 1 {
			return 40 * time.Second
		}
		return waitCost(d, err)
	})
	if code := runWait(h, dir, "--timeout", "100"); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	assertCalls(t, h.ctl,
		agtermtest.Call{Method: "Wait", Args: []string{"C-1", "active,blocked,completed", "1m40s"}},
		mainWait("1m0s"),
	)
	if h.out.String() != `{"status":"running","round":1}`+"\n" {
		t.Fatalf("stdout = %q", h.out)
	}
}
