package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/shurastik/agterm-kit/internal/apply"
	"github.com/shurastik/agterm-kit/internal/rundir"
)

// hookRunDir makes an apply run directory at round `round` with rounds 1..round.
func hookRunDir(t *testing.T, round int) string {
	t.Helper()
	root, _ := filepath.EvalSymlinks(t.TempDir())
	dir := filepath.Join(root, ".agk", "applies", "20261105-100000")
	for k := 1; k <= round; k++ {
		rd, err := rundir.MakeRound(dir, k)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(rd, "prompt.md"), []byte("P"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := rundir.SetRound(dir, round); err != nil {
		t.Fatal(err)
	}
	return dir
}

// runHook runs `agk apply hook <event> <dir>` with stdin and checks the hook
// contract: exit 0, nothing on stdout or stderr.
func runHook(t *testing.T, event, dir, stdin string) {
	t.Helper()
	h := newHarness(t, true)
	h.env.Stdin = strings.NewReader(stdin)
	if code := h.env.Main(context.Background(), []string{"apply", "hook", event, dir}); code != 0 {
		t.Fatalf("hook %s: exit %d", event, code)
	}
	if h.out.Len() != 0 || h.err.Len() != 0 {
		t.Fatalf("hook %s printed: stdout %q stderr %q", event, h.out, h.err)
	}
}

func promptInput(prompt string) string {
	b, _ := json.Marshal(map[string]any{"hook_event_name": "UserPromptSubmit", "prompt": prompt})
	return string(b)
}

func kickoffOf(dir string, k int) string {
	return kickoff(filepath.Join(rundir.RoundDir(dir, k), "prompt.md"))
}

const stopWithShell = `{"hook_event_name":"Stop","background_tasks":[{"id":"b","type":"shell","status":"running"}],"session_crons":[]}`
const stopFinal = `{"hook_event_name":"Stop","background_tasks":[],"session_crons":[]}`

// turnState reads turn.json; ok is false when there is none.
func turnState(t *testing.T, dir string) (apply.Turn, bool) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, apply.TurnFile))
	if os.IsNotExist(err) {
		return apply.Turn{}, false
	}
	if err != nil {
		t.Fatal(err)
	}
	tn, err := apply.ParseTurn(b)
	if err != nil {
		t.Fatalf("turn.json: %v: %s", err, b)
	}
	return tn, true
}

func wantTurn(t *testing.T, dir string, want apply.Turn) {
	t.Helper()
	got, ok := turnState(t, dir)
	if !ok || got != want {
		t.Fatalf("turn.json = %+v (exists %v), want %+v", got, ok, want)
	}
}

func wantNoTurn(t *testing.T, dir string) {
	t.Helper()
	if tn, ok := turnState(t, dir); ok {
		t.Fatalf("turn.json = %+v, want none", tn)
	}
}

func TestApplyHookKickoffStartsRound(t *testing.T) {
	dir := hookRunDir(t, 2)
	runHook(t, "turn", dir, promptInput(kickoffOf(dir, 1)))
	wantTurn(t, dir, apply.Turn{Round: 1, State: apply.TurnRunning})

	runHook(t, "stop", dir, stopFinal)
	wantTurn(t, dir, apply.Turn{Round: 1, State: apply.TurnStopped})

	// Round 2's kickoff moves the round, whatever <run>/round says.
	runHook(t, "turn", dir, promptInput(kickoffOf(dir, 2)))
	wantTurn(t, dir, apply.Turn{Round: 2, State: apply.TurnRunning})
}

func TestApplyHookKickoffIsTrimmed(t *testing.T) {
	dir := hookRunDir(t, 1)
	runHook(t, "turn", dir, promptInput("  "+kickoffOf(dir, 1)+"\n"))
	wantTurn(t, dir, apply.Turn{Round: 1, State: apply.TurnRunning})
}

func TestApplyHookStopRecordsInFlight(t *testing.T) {
	dir := hookRunDir(t, 1)
	runHook(t, "turn", dir, promptInput(kickoffOf(dir, 1)))
	runHook(t, "stop", dir, stopWithShell)
	wantTurn(t, dir, apply.Turn{Round: 1, State: apply.TurnStopped, Background: 1})

	// The applier resumes by itself: the task notification keeps round 1.
	runHook(t, "turn", dir, promptInput("<task-notification>\n<task-id>b</task-id>\n</task-notification>"))
	wantTurn(t, dir, apply.Turn{Round: 1, State: apply.TurnRunning})

	runHook(t, "stop", dir, `{"background_tasks":[],"session_crons":[{"id":"c"},{"id":"d"}]}`)
	wantTurn(t, dir, apply.Turn{Round: 1, State: apply.TurnStopped, Wakeups: 2})

	runHook(t, "turn", dir, promptInput("<task-notification>x</task-notification>"))
	runHook(t, "stop", dir, stopFinal)
	wantTurn(t, dir, apply.Turn{Round: 1, State: apply.TurnStopped})
}

// agk apply send publishes round k+1 before round k's turn is over: the hooks
// must not read the run's current round.
func TestApplyHookIgnoresTheRunsCurrentRound(t *testing.T) {
	dir := hookRunDir(t, 1)
	runHook(t, "turn", dir, promptInput(kickoffOf(dir, 1)))
	// `agk apply send` made round 2 current while round 1's turn is running.
	os.MkdirAll(rundir.RoundDir(dir, 2), 0o755)
	if err := rundir.SetRound(dir, 2); err != nil {
		t.Fatal(err)
	}
	runHook(t, "stop", dir, stopWithShell)
	wantTurn(t, dir, apply.Turn{Round: 1, State: apply.TurnStopped, Background: 1})

	// A resumed turn (task notification) whose hook runs after the publish.
	runHook(t, "turn", dir, promptInput("<task-notification>done</task-notification>"))
	wantTurn(t, dir, apply.Turn{Round: 1, State: apply.TurnRunning})
	runHook(t, "stop", dir, stopWithShell)
	wantTurn(t, dir, apply.Turn{Round: 1, State: apply.TurnStopped, Background: 1})
}

func TestApplyHookUserTypedPromptKeepsTheRound(t *testing.T) {
	dir := hookRunDir(t, 2)
	runHook(t, "turn", dir, promptInput(kickoffOf(dir, 2)))
	runHook(t, "stop", dir, stopFinal)
	runHook(t, "turn", dir, promptInput("why did you pick that file?"))
	wantTurn(t, dir, apply.Turn{Round: 2, State: apply.TurnRunning})
}

func TestApplyHookNotAKickoff(t *testing.T) {
	dir := hookRunDir(t, 3)
	other := filepath.Join(filepath.Dir(dir), "20261105-110000")
	for name, prompt := range map[string]string{
		"user text":            "continue",
		"task notification":    "<task-notification>x</task-notification>",
		"another run's line":   kickoff(filepath.Join(rundir.RoundDir(other, 1), "prompt.md")),
		"round 0":              kickoff(filepath.Join(rundir.RoundDir(dir, 0), "prompt.md")),
		"leading zero round":   kickoff(filepath.Join(dir, "rounds", "02", "prompt.md")),
		"signed round":         kickoff(filepath.Join(dir, "rounds", "+2", "prompt.md")),
		"extra text":           kickoffOf(dir, 2) + " Then stop.",
		"quoted line":          "> " + kickoffOf(dir, 2),
		"other file":           "Read " + filepath.Join(rundir.RoundDir(dir, 2), "report.json") + " and follow it.",
		"nested round":         kickoff(filepath.Join(dir, "rounds", "2", "x", "prompt.md")),
		"empty prompt":         "",
		"round number too big": kickoff(filepath.Join(dir, "rounds", "99999999999999999999", "prompt.md")),
	} {
		t.Run(name, func(t *testing.T) {
			runHook(t, "turn", dir, promptInput(prompt))
			wantNoTurn(t, dir) // no file, no kickoff: nothing
		})
	}
}

func TestApplyHookWritesNothingWithoutAStart(t *testing.T) {
	dir := hookRunDir(t, 1)
	runHook(t, "stop", dir, stopWithShell)
	wantNoTurn(t, dir)
}

func TestApplyHookNeverWritesOnBadInput(t *testing.T) {
	dir := hookRunDir(t, 1)
	runHook(t, "turn", dir, promptInput(kickoffOf(dir, 1)))
	want := apply.Turn{Round: 1, State: apply.TurnRunning}
	for _, in := range []string{"", "garbage", "[]", `{"background_tasks":"x"}`, `{"background_tasks":[`} {
		runHook(t, "stop", dir, in)
		wantTurn(t, dir, want)
	}
	for _, in := range []string{"", "garbage", `{"prompt":5}`, `[]`} {
		runHook(t, "turn", dir, in)
		wantTurn(t, dir, want)
	}
}

func TestApplyHookGarbageTurnFileCountsAsMissing(t *testing.T) {
	dir := hookRunDir(t, 1)
	for _, bad := range []string{"", "nope", `{"round":0,"state":"running"}`, `{"round":1,"state":"gone"}`} {
		if err := os.WriteFile(filepath.Join(dir, apply.TurnFile), []byte(bad), 0o644); err != nil {
			t.Fatal(err)
		}
		runHook(t, "stop", dir, stopFinal)
		runHook(t, "turn", dir, promptInput("typed"))
		b, _ := os.ReadFile(filepath.Join(dir, apply.TurnFile))
		if string(b) != bad {
			t.Fatalf("%q: file rewritten to %q", bad, b)
		}
	}
	// A kickoff still starts a round over a garbage file.
	runHook(t, "turn", dir, promptInput(kickoffOf(dir, 1)))
	wantTurn(t, dir, apply.Turn{Round: 1, State: apply.TurnRunning})
}

func TestApplyHookRefusedDirsWriteNothing(t *testing.T) {
	good := hookRunDir(t, 1)
	missing := filepath.Join(filepath.Dir(good), "20261105-120000") // run-shaped, gone
	plain := t.TempDir()
	reviews := filepath.Join(filepath.Dir(filepath.Dir(good)), "reviews", "20261105-100000")
	os.MkdirAll(reviews, 0o755)
	for name, dir := range map[string]string{"gone": missing, "not a run dir": plain, "review run": reviews, "empty": ""} {
		t.Run(name, func(t *testing.T) {
			runHook(t, "turn", dir, promptInput(kickoff(filepath.Join(rundir.RoundDir(dir, 1), "prompt.md"))))
			runHook(t, "stop", dir, stopFinal)
			for _, d := range []string{dir, plain, reviews, missing} {
				if d == "" {
					continue
				}
				if _, err := os.Stat(filepath.Join(d, apply.TurnFile)); err == nil {
					t.Fatalf("%s: turn.json written", d)
				}
			}
		})
	}
}

func TestApplyHookBadUsageStaysSilent(t *testing.T) {
	dir := hookRunDir(t, 1)
	for _, args := range [][]string{{"apply", "hook"}, {"apply", "hook", "turn"}, {"apply", "hook", "pause", dir}, {"apply", "hook", "turn", dir, "extra"}} {
		h := newHarness(t, true)
		h.env.Stdin = strings.NewReader(promptInput(kickoffOf(dir, 1)))
		if code := h.env.Main(context.Background(), args); code != 0 || h.out.Len() != 0 || h.err.Len() != 0 {
			t.Fatalf("%v: exit %d stdout %q stderr %q", args, code, h.out, h.err)
		}
	}
	wantNoTurn(t, dir)
}

func TestApplyHookReadsAtMostOneMiB(t *testing.T) {
	dir := hookRunDir(t, 1)
	runHook(t, "turn", dir, promptInput(kickoffOf(dir, 1)))
	// A valid Stop input longer than the cap is cut mid-JSON: nothing recorded.
	pad := strings.Repeat("x", 1<<20)
	runHook(t, "stop", dir, `{"background_tasks":[],"pad":"`+pad+`"}`)
	wantTurn(t, dir, apply.Turn{Round: 1, State: apply.TurnRunning})
}

func TestApplierSettings(t *testing.T) {
	b, err := applierSettings("/usr/local/bin/agk", "/r/.agk/applies/20261105-100000")
	if err != nil {
		t.Fatal(err)
	}
	var got, base map[string]json.RawMessage
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, b)
	}
	if err := json.Unmarshal(apply.ClaudeSettings(), &base); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("keys = %v, want permissions and hooks", got)
	}
	var gotPerm, basePerm any
	json.Unmarshal(got["permissions"], &gotPerm)
	json.Unmarshal(base["permissions"], &basePerm)
	if !reflect.DeepEqual(gotPerm, basePerm) {
		t.Fatalf("deny rules changed: %s", got["permissions"])
	}
	want := `{"UserPromptSubmit":[{"hooks":[{"type":"command","command":"'/usr/local/bin/agk' apply hook turn '/r/.agk/applies/20261105-100000'"}]}],` +
		`"Stop":[{"hooks":[{"type":"command","command":"'/usr/local/bin/agk' apply hook stop '/r/.agk/applies/20261105-100000'"}]}],` +
		`"StopFailure":[{"hooks":[{"type":"command","command":"'/usr/local/bin/agk' apply hook stop '/r/.agk/applies/20261105-100000'"}]}]}`
	var wantV, gotV any
	json.Unmarshal([]byte(want), &wantV)
	json.Unmarshal(got["hooks"], &gotV)
	if !reflect.DeepEqual(gotV, wantV) {
		t.Fatalf("hooks = %s\nwant %s", got["hooks"], want)
	}
}

func TestApplierSettingsQuotesAndRefuses(t *testing.T) {
	b, err := applierSettings("/opt/it's/agk", "/r/it's/.agk/applies/20261105-100000")
	if err != nil || !strings.Contains(string(b), `'/opt/it'\\''s/agk' apply hook turn '/r/it'\\''s/.agk/applies/20261105-100000'`) {
		t.Fatalf("quoting: %v\n%s", err, b)
	}
	for _, c := range [][2]string{{`/opt/a\b/agk`, "/r/run"}, {"/opt/agk", `/r/a\b/run`}, {"/opt/agk", "/r/a\nb/run"}, {"/opt/a\rb/agk", "/r/run"}} {
		if _, err := applierSettings(c[0], c[1]); err == nil {
			t.Fatalf("%q %q must be refused", c[0], c[1])
		}
	}
}
