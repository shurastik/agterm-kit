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
)

// An existing file must not turn a spent budget into an endless loop: the
// budget is checked before the file.
func TestWaitSlicedFileExistsAndBudgetSpent(t *testing.T) {
	h := newHarness(t, true)
	file := filepath.Join(t.TempDir(), "out.json")
	if err := os.WriteFile(file, []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	h.ctl.WaitErr = agterm.ErrTimeout
	status, err, found, rest := h.env.waitSliced(context.Background(), "C-1", []string{"completed"}, 3*time.Second, file)
	if status != "" || !errors.Is(err, agterm.ErrTimeout) || found || rest != 0 {
		t.Fatalf("status %q err %v found %v rest %v: want the budget returned as spent", status, err, found, rest)
	}
}

func TestWaitSlicedFileAppearsWithBudgetLeft(t *testing.T) {
	h := newHarness(t, true)
	pollSlice = 5 * time.Second
	file := filepath.Join(t.TempDir(), "out.json")
	h.ctl.WaitErr = agterm.ErrTimeout
	n := 0
	h.ctl.OnWait = func() {
		if n++; n == 2 {
			os.WriteFile(file, []byte("{}"), 0o644)
		}
	}
	_, err, found, rest := h.env.waitSliced(context.Background(), "C-1", []string{"completed"}, 60*time.Second, file)
	if err != nil || !found || rest != 50*time.Second {
		t.Fatalf("err %v found %v rest %v", err, found, rest)
	}
}

func TestTypeWhenIdleTypesAndConfirms(t *testing.T) {
	h := newHarness(t, true)
	h.ctl.WaitSeq = []agtermtest.WaitResult{{Status: "completed"}, {Status: "active"}}
	started, err := h.env.typeWhenIdle(context.Background(), reviewerWorker, "C-1", "Read x and follow it.", "round 2", 10*time.Minute, time.Time{}, nil)
	if err != nil || !started {
		t.Fatalf("started %v err %v", started, err)
	}
	assertCalls(t, h.ctl,
		agtermtest.Call{Method: "Wait", Args: []string{"C-1", "completed,blocked,idle", "10m0s"}},
		agtermtest.Call{Method: "Type", Args: []string{"C-1", "Read x and follow it.\r"}},
		agtermtest.Call{Method: "Wait", Args: []string{"C-1", "active", "10s"}},
	)
}

func TestTypeWhenIdleUnconfirmedStartWarns(t *testing.T) {
	h := newHarness(t, true)
	h.ctl.WaitSeq = []agtermtest.WaitResult{{Status: "idle"}, {Err: agterm.ErrTimeout}}
	started, err := h.env.typeWhenIdle(context.Background(), reviewerWorker, "C-1", "line", "round 2", time.Minute, time.Time{}, nil)
	if err != nil || started {
		t.Fatalf("started %v err %v", started, err)
	}
	if !strings.Contains(h.err.String(), "reviewer C-1 did not report round 2 as started") {
		t.Fatalf("stderr = %q", h.err)
	}
}

// stepClock is an injected clock that the scripted Ctl calls advance: each
// Wait, Text and Type pops the next duration of its list and moves the clock
// by it, the way a real agtermctl process takes real time.
type stepClock struct {
	now    time.Time
	waits  []time.Duration
	texts  []time.Duration
	types_ []time.Duration
}

func pop(l *[]time.Duration) time.Duration {
	if len(*l) == 0 {
		return 0
	}
	d := (*l)[0]
	*l = (*l)[1:]
	return d
}

// withClock installs a stepClock on h. The initial wait for the turn end and
// the line's own Type take no time unless the lists say so.
func withClock(h *tenv) *stepClock {
	c := &stepClock{now: time.Date(2026, 11, 5, 10, 0, 0, 0, time.Local)}
	h.env.Now = func() time.Time { return c.now }
	h.ctl.OnWait = func() { c.now = c.now.Add(pop(&c.waits)) }
	h.ctl.OnType = func(string, string) { c.now = c.now.Add(pop(&c.types_)) }
	h.ctl.OnText = func() { c.now = c.now.Add(pop(&c.texts)) }
	return c
}

const handshakeLine = "Read /repo/.agk/reviews/20261105-100000/rounds/2/prompt.md and follow it."

// wrappedScreen is the line as codex shows it in its composer: wrapped across
// two rows with indentation.
const wrappedScreen = "› Read /repo/.agk/reviews/20261105-100000/rounds/2/\n" +
	"  prompt.md and follow it.\n"

func typeLine(h *tenv, deadline time.Time) (bool, error) {
	return h.env.typeWhenIdle(context.Background(), reviewerWorker, "C-1", handshakeLine, "round 2", time.Minute, deadline, nil)
}

// types returns the texts of the recorded Type calls.
func types(h *tenv) []string {
	var out []string
	for _, c := range h.ctl.Calls {
		if c.Method == "Type" {
			out = append(out, c.Args[1])
		}
	}
	return out
}

func TestTypeWhenIdleConfirmedFirstWaitReadsNoScreen(t *testing.T) {
	h := newHarness(t, true)
	h.ctl.WaitSeq = []agtermtest.WaitResult{{Status: "completed"}, {Status: "active"}}
	if started, err := typeLine(h, time.Time{}); err != nil || !started {
		t.Fatalf("started %v err %v", started, err)
	}
	for _, m := range h.ctl.Methods() {
		if m == "Text" {
			t.Fatal("an active status within the first wait needs no screen read")
		}
	}
	if got := types(h); !reflect.DeepEqual(got, []string{handshakeLine + "\r"}) {
		t.Fatalf("typed %q", got)
	}
}

func TestTypeWhenIdleEnterRetrySubmitsWrappedLine(t *testing.T) {
	h := newHarness(t, true)
	h.ctl.TextOut = wrappedScreen
	h.ctl.WaitSeq = []agtermtest.WaitResult{
		{Status: "completed"},    // the turn is over
		{Err: agterm.ErrTimeout}, // first 10 s: not picked up
		{Status: "completed"},    // current status
		{Status: "active"},       // after the Enter
	}
	if started, err := typeLine(h, time.Time{}); err != nil || !started {
		t.Fatalf("started %v err %v", started, err)
	}
	assertCalls(t, h.ctl,
		agtermtest.Call{Method: "Wait", Args: []string{"C-1", "completed,blocked,idle", "1m0s"}},
		agtermtest.Call{Method: "Type", Args: []string{"C-1", handshakeLine + "\r"}},
		agtermtest.Call{Method: "Wait", Args: []string{"C-1", "active", "10s"}},
		agtermtest.Call{Method: "Wait", Args: []string{"C-1", "active,completed,idle,blocked", "1s"}},
		agtermtest.Call{Method: "Text", Args: []string{"C-1", "20"}},
		agtermtest.Call{Method: "Type", Args: []string{"C-1", "\r"}},
		agtermtest.Call{Method: "Wait", Args: []string{"C-1", "active,blocked", "30s"}},
	)
}

func TestTypeWhenIdleNoRetryCases(t *testing.T) {
	cases := map[string]struct {
		status []agtermtest.WaitResult // after the first wait times out
		screen string
		last   bool // a last Wait(active) follows the screen read
	}{
		"blocked":       {[]agtermtest.WaitResult{{Status: "blocked"}}, wrappedScreen, false},
		"status times":  {[]agtermtest.WaitResult{{Err: agterm.ErrTimeout}}, wrappedScreen, false},
		"status fails":  {[]agtermtest.WaitResult{{Err: errors.New("boom")}}, wrappedScreen, false},
		"unknown":       {[]agtermtest.WaitResult{{Status: ""}}, wrappedScreen, false},
		"idle, no line": {[]agtermtest.WaitResult{{Status: "idle"}, {Err: agterm.ErrTimeout}}, "› \n  gpt-5.5 high\n", true},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t, true)
			h.ctl.TextOut = c.screen
			h.ctl.WaitSeq = append([]agtermtest.WaitResult{{Status: "completed"}, {Err: agterm.ErrTimeout}}, c.status...)
			started, err := typeLine(h, time.Time{})
			if err != nil || started {
				t.Fatalf("started %v err %v", started, err)
			}
			if got := types(h); len(got) != 1 {
				t.Fatalf("typed %q: only the line itself may be typed", got)
			}
			warn := h.err.String()
			if strings.Contains(warn, "press Enter in the companion") || !strings.Contains(warn, "look at the companion: if the line is still in its input box, press Enter there; if it asks for permission, answer it deliberately") && name != "blocked" {
				t.Fatalf("stderr = %q: no bare Enter advice, but the conditional pointer to the companion", warn)
			}
			if name == "blocked" && (!strings.Contains(warn, "permission prompt") || strings.Contains(warn, "press Enter")) {
				t.Fatalf("stderr = %q: a blocked worker shows a permission prompt, no Enter advice at all", warn)
			}
			last := h.ctl.Calls[len(h.ctl.Calls)-1]
			if c.last != (last.Method == "Wait" && last.Args[1] == "active,blocked" && len(h.ctl.Calls) == 6) {
				t.Fatalf("calls = %v, last wait expected: %v", h.ctl.Calls, c.last)
			}
			if c.status[0].Status == "blocked" || c.status[0].Err != nil {
				for _, m := range h.ctl.Methods() {
					if m == "Text" {
						t.Fatal("no screen read when the status is not completed/idle")
					}
				}
			}
		})
	}
}

func TestTypeWhenIdleRetryTimesOutWarnsAboutInputBox(t *testing.T) {
	h := newHarness(t, true)
	h.ctl.TextOut = wrappedScreen
	h.ctl.WaitSeq = []agtermtest.WaitResult{
		{Status: "completed"}, {Err: agterm.ErrTimeout}, {Status: "completed"}, {Err: agterm.ErrTimeout},
	}
	started, err := typeLine(h, time.Time{})
	if err != nil || started {
		t.Fatalf("started %v err %v", started, err)
	}
	if got := types(h); !reflect.DeepEqual(got, []string{handshakeLine + "\r", "\r"}) {
		t.Fatalf("typed %q: exactly one Enter retry", got)
	}
	// The Enter was typed after the screen snapshot, so the snapshot no longer
	// proves an unsent draft: the hint is the conditional one.
	for _, want := range []string{"reviewer C-1 did not report round 2 as started", "look at the companion: if the line is still in its input box, press Enter there; if it asks for permission, answer it deliberately"} {
		if !strings.Contains(h.err.String(), want) {
			t.Errorf("stderr lacks %q: %q", want, h.err)
		}
	}
	if strings.Contains(h.err.String(), "press Enter in the companion") {
		t.Errorf("stderr = %q: no bare Enter advice after the retry Enter", h.err)
	}
}

// The retry Enter submitted the line and the worker then asked for permission:
// the last wait sees blocked, and the warning must not tell anyone to press Enter.
func TestTypeWhenIdleBlockedAfterRetryEnter(t *testing.T) {
	h := newHarness(t, true)
	h.ctl.TextOut = wrappedScreen
	h.ctl.WaitSeq = []agtermtest.WaitResult{
		{Status: "completed"}, {Err: agterm.ErrTimeout}, {Status: "completed"}, {Status: "blocked"},
	}
	started, err := typeLine(h, time.Time{})
	if err != nil || started {
		t.Fatalf("started %v err %v", started, err)
	}
	if got := types(h); !reflect.DeepEqual(got, []string{handshakeLine + "\r", "\r"}) {
		t.Fatalf("typed %q", got)
	}
	last := h.ctl.Calls[len(h.ctl.Calls)-1]
	if last.Method != "Wait" || last.Args[1] != "active,blocked" {
		t.Fatalf("last call = %v: the last wait is for active or blocked", last)
	}
	warn := h.err.String()
	if !strings.Contains(warn, "permission prompt") || strings.Contains(warn, "press Enter") {
		t.Fatalf("stderr = %q", warn)
	}
}

// The line was seen but the budget ended before any Enter was typed: the line
// is still known to be unsent, so the hint says so.
func TestTypeWhenIdleLineSeenNoEnterTyped(t *testing.T) {
	h := newHarness(t, true)
	c := withClock(h)
	c.waits = []time.Duration{0, 10 * time.Second, 0}
	c.texts = []time.Duration{2 * time.Second}
	h.ctl.TextOut = wrappedScreen
	h.ctl.WaitSeq = []agtermtest.WaitResult{{Status: "completed"}, {Err: agterm.ErrTimeout}, {Status: "completed"}}
	started, err := typeLine(h, c.now.Add(11*time.Second))
	if err != nil || started {
		t.Fatalf("started %v err %v", started, err)
	}
	if got := types(h); len(got) != 1 {
		t.Fatalf("typed %q: the budget is spent, no Enter", got)
	}
	if !strings.Contains(h.err.String(), "the line may still be in its input box: press Enter in the companion") {
		t.Fatalf("stderr = %q", h.err)
	}
}

func TestTypeWhenIdleDeadlineBoundsTheSteps(t *testing.T) {
	t.Run("5s left: first wait 5s, nothing more", func(t *testing.T) {
		h := newHarness(t, true)
		c := withClock(h)
		c.waits = []time.Duration{0, 5 * time.Second}
		h.ctl.TextOut = wrappedScreen
		h.ctl.WaitSeq = []agtermtest.WaitResult{{Status: "completed"}, {Err: agterm.ErrTimeout}}
		started, err := typeLine(h, c.now.Add(5*time.Second))
		if err != nil || started {
			t.Fatalf("started %v err %v", started, err)
		}
		last := h.ctl.Calls[len(h.ctl.Calls)-1]
		if last.Method != "Wait" || last.Args[1] != "active" || last.Args[2] != "5s" || len(h.ctl.Calls) != 3 {
			t.Fatalf("calls = %v", h.ctl.Calls)
		}
	})
	t.Run("15s left: first wait 10s, retry wait 5s", func(t *testing.T) {
		h := newHarness(t, true)
		c := withClock(h)
		c.waits = []time.Duration{0, 10 * time.Second, 0, 0}
		h.ctl.TextOut = wrappedScreen
		h.ctl.WaitSeq = []agtermtest.WaitResult{
			{Status: "completed"}, {Err: agterm.ErrTimeout}, {Status: "completed"}, {Status: "active"},
		}
		started, err := typeLine(h, c.now.Add(15*time.Second))
		if err != nil || !started {
			t.Fatalf("started %v err %v", started, err)
		}
		calls := h.ctl.Calls
		if calls[2].Args[2] != "10s" || calls[len(calls)-1].Args[2] != "5s" {
			t.Fatalf("calls = %v", calls)
		}
	})
	t.Run("clock advances: 11s left, status check takes 2s", func(t *testing.T) {
		h := newHarness(t, true)
		c := withClock(h)
		c.waits = []time.Duration{0, 10 * time.Second, 2 * time.Second}
		h.ctl.TextOut = wrappedScreen
		h.ctl.WaitSeq = []agtermtest.WaitResult{
			{Status: "completed"}, {Err: agterm.ErrTimeout}, {Status: "completed"},
		}
		started, err := typeLine(h, c.now.Add(11*time.Second))
		if err != nil || started {
			t.Fatalf("started %v err %v", started, err)
		}
		if got := types(h); len(got) != 1 {
			t.Fatalf("typed %q: the budget is spent, no Enter", got)
		}
		if last := h.ctl.Calls[len(h.ctl.Calls)-1]; last.Method == "Wait" && last.Args[1] == "active,blocked" && len(h.ctl.Calls) > 4 {
			t.Fatalf("calls = %v: no last wait on a spent budget", h.ctl.Calls)
		}
	})
	t.Run("clock advances: 15s left, status check takes 2s", func(t *testing.T) {
		h := newHarness(t, true)
		c := withClock(h)
		c.waits = []time.Duration{0, 10 * time.Second, 2 * time.Second, 0}
		h.ctl.TextOut = wrappedScreen
		h.ctl.WaitSeq = []agtermtest.WaitResult{
			{Status: "completed"}, {Err: agterm.ErrTimeout}, {Status: "completed"}, {Status: "active"},
		}
		started, err := typeLine(h, c.now.Add(15*time.Second))
		if err != nil || !started {
			t.Fatalf("started %v err %v", started, err)
		}
		if last := h.ctl.Calls[len(h.ctl.Calls)-1]; last.Args[2] != "3s" {
			t.Fatalf("last wait = %v, want 3s", last)
		}
	})
}

// Every call after the line is typed runs under a context that ends with the
// handshake, so a stuck agtermctl cannot outlive the budget. The wait for the
// turn end and the line's own Type keep the caller's context.
func TestTypeWhenIdleHandshakeCallsCarryTheDeadline(t *testing.T) {
	h := newHarness(t, true)
	h.ctl.TextOut = wrappedScreen
	h.ctl.WaitSeq = []agtermtest.WaitResult{
		{Status: "completed"}, {Err: agterm.ErrTimeout}, {Status: "completed"}, {Status: "active"},
	}
	if started, err := typeLine(h, time.Time{}); err != nil || !started {
		t.Fatalf("started %v err %v", started, err)
	}
	// Calls: Wait(turn end), Type(line), Wait, Wait, Text, Type(Enter), Wait.
	for i, c := range h.ctl.Calls {
		has := !h.ctl.Deadlines[i].IsZero()
		if want := i >= 2; has != want {
			t.Errorf("call %d %v: deadline %v, want %v", i, c, has, want)
		}
	}
}

func TestTypeWhenIdleHandshakeErrorsMeanNotStarted(t *testing.T) {
	t.Run("first wait cut off", func(t *testing.T) {
		h := newHarness(t, true)
		h.ctl.WaitSeq = []agtermtest.WaitResult{{Status: "completed"}, {Err: context.DeadlineExceeded}}
		started, err := typeLine(h, time.Time{})
		if err != nil || started {
			t.Fatalf("started %v err %v: the round stays recorded, so no error", started, err)
		}
		if got := types(h); len(got) != 1 {
			t.Fatalf("typed %q", got)
		}
	})
	t.Run("screen read cut off", func(t *testing.T) {
		h := newHarness(t, true)
		h.ctl.TextErr = context.DeadlineExceeded
		h.ctl.WaitSeq = []agtermtest.WaitResult{{Status: "completed"}, {Err: agterm.ErrTimeout}, {Status: "idle"}}
		started, err := typeLine(h, time.Time{})
		if err != nil || started {
			t.Fatalf("started %v err %v", started, err)
		}
		if got := types(h); len(got) != 1 {
			t.Fatalf("typed %q", got)
		}
	})
	t.Run("retry Enter cut off", func(t *testing.T) {
		h := newHarness(t, true)
		h.ctl.TextOut = wrappedScreen
		h.ctl.WaitSeq = []agtermtest.WaitResult{{Status: "completed"}, {Err: agterm.ErrTimeout}, {Status: "idle"}}
		h.ctl.OnType = func(_, text string) {
			if text == "\r" {
				h.ctl.TypeErr = context.DeadlineExceeded
			}
		}
		started, err := typeLine(h, time.Time{})
		if err != nil || started {
			t.Fatalf("started %v err %v", started, err)
		}
		// An Enter was attempted after the snapshot: the neutral hint, no bare advice.
		if strings.Contains(h.err.String(), "press Enter in the companion") || !strings.Contains(h.err.String(), "look at the companion") {
			t.Fatalf("stderr = %q", h.err)
		}
	})
}

func TestTypeWhenIdleRefusals(t *testing.T) {
	cases := map[string]struct {
		prep func(h *tenv)
		want string
	}{
		"busy":       {func(h *tenv) { h.ctl.WaitSeq = []agtermtest.WaitResult{{Err: agterm.ErrTimeout}} }, "reviewer C-1 is busy: its turn did not end within 30 s"},
		"permission": {func(h *tenv) { h.ctl.WaitSeq = []agtermtest.WaitResult{{Status: "blocked"}} }, "reviewer C-1 waits for the user (permission prompt)"},
		"gone":       {func(h *tenv) { h.ctl.WaitSeq = []agtermtest.WaitResult{{Err: agterm.ErrGone}} }, "reviewer C-1 is gone; start a new review"},
		"type gone":  {func(h *tenv) { h.ctl.WaitStatus = "idle"; h.ctl.TypeErr = agterm.ErrGone }, "reviewer C-1 is gone; start a new review"},
		"wait fails": {func(h *tenv) { h.ctl.WaitSeq = []agtermtest.WaitResult{{Err: errors.New("boom")}} }, "wait for reviewer C-1: boom"},
		"type fails": {func(h *tenv) { h.ctl.WaitStatus = "idle"; h.ctl.TypeErr = errors.New("boom") }, "type into reviewer C-1: boom"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t, true)
			c.prep(h)
			started, err := h.env.typeWhenIdle(context.Background(), reviewerWorker, "C-1", "line", "round 2", 30*time.Second, time.Time{}, nil)
			if err == nil || started || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("started %v err %v, want %q", started, err, c.want)
			}
			if name != "type gone" && name != "type fails" && !reflect.DeepEqual(h.ctl.Methods(), []string{"Wait"}) {
				t.Fatalf("calls = %v: nothing may be typed", h.ctl.Calls)
			}
		})
	}
}

func TestCompanionName(t *testing.T) {
	tests := []struct {
		change, harness, kind string
		round                 int
		want                  string
	}{
		{"", "codex", "review", 3, "codex-review r3"},
		{"add-auth", "claude", "apply", 1, "add-auth · claude-apply r1"},
		{"add-auth", "codex", "plan", 8, "add-auth · codex-plan r8"},
	}
	for _, tt := range tests {
		if got := companionName(tt.change, tt.harness, tt.kind, tt.round); got != tt.want {
			t.Errorf("companionName(%q, %q, %q, %d) = %q, want %q", tt.change, tt.harness, tt.kind, tt.round, got, tt.want)
		}
	}
}

// settleHarness is a harness for the screen fallback of typeWhenIdle: a short
// poll slice, a status that never reaches a turn end (every Wait times out
// unless WaitSeq says otherwise) and a settler whose evidence is evidence.
func settleHarness(t *testing.T, evidence bool) (*tenv, *settler) {
	t.Helper()
	h := newHarness(t, true)
	pollSlice = time.Second
	h.ctl.WaitErr = agterm.ErrTimeout
	return h, newSettler("round 1", func() bool { return evidence })
}

func typeSettled(h *tenv, s *settler) (bool, error) {
	return h.env.typeWhenIdle(context.Background(), reviewerWorker, "C-1", "line", "round 2", 3*time.Second, time.Time{}, s)
}

func textReads(h *tenv) int {
	n := 0
	for _, m := range h.ctl.Methods() {
		if m == "Text" {
			n++
		}
	}
	return n
}

func TestTypeWhenIdleSettledScreenTypesWithWarning(t *testing.T) {
	h, s := settleHarness(t, true)
	h.ctl.TextOut = "idle composer\n› \n"
	h.ctl.WaitSeq = []agtermtest.WaitResult{
		{Err: agterm.ErrTimeout}, {Err: agterm.ErrTimeout}, // two slices
		{Status: "active"}, // the status check before the warning
		{Status: "active"}, // the line was picked up
	}
	started, err := typeSettled(h, s)
	if err != nil || !started {
		t.Fatalf("started %v err %v", started, err)
	}
	assertCalls(t, h.ctl,
		agtermtest.Call{Method: "Wait", Args: []string{"C-1", "completed,blocked,idle", "1s"}},
		agtermtest.Call{Method: "Text", Args: []string{"C-1", "40"}},
		agtermtest.Call{Method: "Wait", Args: []string{"C-1", "completed,blocked,idle", "1s"}},
		agtermtest.Call{Method: "Text", Args: []string{"C-1", "40"}},
		agtermtest.Call{Method: "Wait", Args: []string{"C-1", "active,completed,idle,blocked", "1s"}},
		agtermtest.Call{Method: "Type", Args: []string{"C-1", "line\r"}},
		agtermtest.Call{Method: "Wait", Args: []string{"C-1", "active", "10s"}},
	)
	want := "agk: warning: the reviewer C-1 still reports active, but round 1 is on disk and its screen has not changed for 1 s; treating its turn as over\n"
	if h.err.String() != want {
		t.Fatalf("stderr = %q, want %q", h.err, want)
	}
}

func TestTypeWhenIdleSettledUnknownStatusWarns(t *testing.T) {
	h, s := settleHarness(t, true)
	h.ctl.TextOut = "same"
	h.ctl.WaitSeq = []agtermtest.WaitResult{{Err: agterm.ErrTimeout}, {Err: agterm.ErrTimeout}, {Err: agterm.ErrTimeout}, {Status: "active"}}
	if _, err := typeSettled(h, s); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(h.err.String(), "still reports no finished turn, but round 1 is on disk") {
		t.Fatalf("stderr = %q", h.err)
	}
	if got := types(h); !reflect.DeepEqual(got, []string{"line\r"}) {
		t.Fatalf("typed %q", got)
	}
}

// The status check before the warning is a last chance for the status: a turn
// end it reports is the ordinary path, a prompt is a refusal.
func TestTypeWhenIdleSettledStatusCheckDecides(t *testing.T) {
	t.Run("completed", func(t *testing.T) {
		h, s := settleHarness(t, true)
		h.ctl.TextOut = "same"
		h.ctl.WaitSeq = []agtermtest.WaitResult{{Err: agterm.ErrTimeout}, {Err: agterm.ErrTimeout}, {Status: "completed"}, {Status: "active"}}
		if started, err := typeSettled(h, s); err != nil || !started {
			t.Fatalf("started %v err %v", started, err)
		}
		if h.err.Len() != 0 {
			t.Fatalf("stderr = %q: a reported turn end needs no warning", h.err)
		}
	})
	t.Run("blocked", func(t *testing.T) {
		h, s := settleHarness(t, true)
		h.ctl.TextOut = "same"
		h.ctl.WaitSeq = []agtermtest.WaitResult{{Err: agterm.ErrTimeout}, {Err: agterm.ErrTimeout}, {Status: "blocked"}}
		_, err := typeSettled(h, s)
		if !errors.Is(err, errPrompt) || len(types(h)) != 0 {
			t.Fatalf("err %v typed %q", err, types(h))
		}
	})
	t.Run("gone", func(t *testing.T) {
		h, s := settleHarness(t, true)
		h.ctl.TextOut = "same"
		h.ctl.WaitSeq = []agtermtest.WaitResult{{Err: agterm.ErrTimeout}, {Err: agterm.ErrTimeout}, {Err: agterm.ErrGone}}
		_, err := typeSettled(h, s)
		if !errors.Is(err, agterm.ErrGone) || len(types(h)) != 0 {
			t.Fatalf("err %v typed %q", err, types(h))
		}
	})
}

func TestTypeWhenIdleChangingScreenIsBusy(t *testing.T) {
	h, s := settleHarness(t, true)
	h.ctl.TextSeq = []agtermtest.TextResult{{Out: "Working (5s)"}, {Out: "Working (10s)"}, {Out: "Working (15s)"}}
	started, err := typeSettled(h, s)
	var ie *idleError
	if started || !errors.As(err, &ie) || !errors.Is(err, agterm.ErrTimeout) || !strings.Contains(err.Error(), "is busy") {
		t.Fatalf("started %v err %v", started, err)
	}
	if got := types(h); len(got) != 0 {
		t.Fatalf("typed %q", got)
	}
}

func TestTypeWhenIdleDialogMarkerIsBusy(t *testing.T) {
	for _, marker := range []string{"Press Enter to confirm", "enter to submit answer", "ENTER TO SUBMIT ALL", "Allow command?"} {
		t.Run(marker, func(t *testing.T) {
			h, s := settleHarness(t, true)
			h.ctl.TextOut = "Do you trust this folder?\n  1. Yes\n  " + marker + " or esc to cancel\n"
			_, err := typeSettled(h, s)
			if !errors.Is(err, agterm.ErrTimeout) || len(types(h)) != 0 {
				t.Fatalf("err %v typed %q", err, types(h))
			}
			if textReads(h) < 2 {
				t.Fatalf("calls = %v: the screen was read twice and still refused", h.ctl.Calls)
			}
		})
	}
}

// Only the last 12 lines count: a marker that scrolled off the end is old
// output, and trailing blank rows do not push a live dialog out of the window.
func TestTypeWhenIdleDialogWindow(t *testing.T) {
	old := "press enter to confirm\n" + strings.Repeat("output\n", 12)
	h, s := settleHarness(t, true)
	h.ctl.TextOut = old
	h.ctl.WaitSeq = []agtermtest.WaitResult{{Err: agterm.ErrTimeout}, {Err: agterm.ErrTimeout}, {Status: "active"}, {Status: "active"}}
	if _, err := typeSettled(h, s); err != nil || len(types(h)) != 1 {
		t.Fatalf("err %v typed %q: a marker above the last 12 lines does not block", err, types(h))
	}
	live := "press enter to confirm\n" + strings.Repeat("output\n", 11) + strings.Repeat("\n", 20)
	h, s = settleHarness(t, true)
	h.ctl.TextOut = live
	if _, err := typeSettled(h, s); !errors.Is(err, agterm.ErrTimeout) || len(types(h)) != 0 {
		t.Fatalf("err %v typed %q: blank rows at the end must not hide a dialog", err, types(h))
	}
}

func TestTypeWhenIdleNoEvidenceReadsNoScreen(t *testing.T) {
	h, s := settleHarness(t, false)
	h.ctl.TextOut = "same"
	_, err := typeSettled(h, s)
	if !errors.Is(err, agterm.ErrTimeout) || len(types(h)) != 0 || textReads(h) != 0 {
		t.Fatalf("err %v typed %q, %d screen reads", err, types(h), textReads(h))
	}
}

func TestTypeWhenIdleUnreadableScreenIsBusy(t *testing.T) {
	h, s := settleHarness(t, true)
	h.ctl.TextErr = errors.New("boom")
	_, err := typeSettled(h, s)
	if !errors.Is(err, agterm.ErrTimeout) || len(types(h)) != 0 {
		t.Fatalf("err %v typed %q", err, types(h))
	}
}

// A failed read forgets the previous screen: two equal reads must be
// consecutive.
func TestTypeWhenIdleReadErrorResetsPreviousScreen(t *testing.T) {
	h, s := settleHarness(t, true)
	h.ctl.TextSeq = []agtermtest.TextResult{{Out: "same"}, {Err: errors.New("boom")}}
	h.ctl.TextOut = "same"
	pollSlice = time.Second
	_, err := h.env.typeWhenIdle(context.Background(), reviewerWorker, "C-1", "line", "round 2", 3*time.Second, time.Time{}, s)
	if !errors.Is(err, agterm.ErrTimeout) || len(types(h)) != 0 {
		t.Fatalf("err %v typed %q: same, error, (third read never taken) must not settle", err, types(h))
	}
}

func TestTypeWhenIdleStatusEndsBeforeAnyScreenRead(t *testing.T) {
	h, s := settleHarness(t, true)
	h.ctl.TextOut = "same"
	h.ctl.WaitSeq = []agtermtest.WaitResult{{Status: "completed"}, {Status: "active"}}
	if started, err := typeSettled(h, s); err != nil || !started {
		t.Fatalf("started %v err %v", started, err)
	}
	if textReads(h) != 0 || h.err.Len() != 0 {
		t.Fatalf("%d screen reads, stderr %q: a reported turn end wins at once", textReads(h), h.err)
	}
}

func TestTypeWhenIdleBlockedStillRefusesWithSettler(t *testing.T) {
	h, s := settleHarness(t, true)
	h.ctl.WaitSeq = []agtermtest.WaitResult{{Status: "blocked"}}
	_, err := typeSettled(h, s)
	if !errors.Is(err, errPrompt) || len(types(h)) != 0 || textReads(h) != 0 {
		t.Fatalf("err %v typed %q", err, types(h))
	}
}

func TestTypeWhenIdleWithoutSettlerWaitsOnce(t *testing.T) {
	h := newHarness(t, true)
	pollSlice = time.Second
	h.ctl.WaitErr = agterm.ErrTimeout
	_, err := h.env.typeWhenIdle(context.Background(), reviewerWorker, "C-1", "line", "round 2", 3*time.Second, time.Time{}, nil)
	if !errors.Is(err, agterm.ErrTimeout) {
		t.Fatalf("err = %v", err)
	}
	assertCalls(t, h.ctl, agtermtest.Call{Method: "Wait", Args: []string{"C-1", "completed,blocked,idle", "3s"}})
}

// The time the screen reads take comes out of the budget: two 5 s slices and
// two 1 s reads must not type after 12 s on an 11 s budget.
func TestTypeWhenIdleScreenReadsSpendTheBudget(t *testing.T) {
	h := newHarness(t, true)
	pollSlice = 5 * time.Second
	h.ctl.WaitErr = agterm.ErrTimeout
	h.ctl.TextOut = "same"
	c := withClock(h)
	c.waits = []time.Duration{5 * time.Second, 5 * time.Second, 5 * time.Second}
	c.texts = []time.Duration{time.Second, time.Second, time.Second, time.Second}
	s := newSettler("round 1", func() bool { return true })
	started, err := h.env.typeWhenIdle(context.Background(), reviewerWorker, "C-1", "line", "round 2", 11*time.Second, time.Time{}, s)
	if started || !errors.Is(err, agterm.ErrTimeout) || !strings.Contains(err.Error(), "is busy") {
		t.Fatalf("started %v err %v", started, err)
	}
	if got := types(h); len(got) != 0 {
		t.Fatalf("typed %q after the budget was spent", got)
	}
}

// The caller's deadline caps the fallback too, when it is earlier than the
// timeout (the fix-up's `review wait` deadline).
func TestTypeWhenIdleDeadlineCapsTheFallback(t *testing.T) {
	h := newHarness(t, true)
	pollSlice = 5 * time.Second
	h.ctl.WaitErr = agterm.ErrTimeout
	h.ctl.TextOut = "same"
	c := withClock(h)
	c.waits = []time.Duration{5 * time.Second, 5 * time.Second, 5 * time.Second}
	c.texts = []time.Duration{time.Second, time.Second, time.Second, time.Second}
	s := newSettler("round 1", func() bool { return true })
	deadline := h.env.Now().Add(11 * time.Second)
	_, err := h.env.typeWhenIdle(context.Background(), reviewerWorker, "C-1", "line", "round 2", time.Hour, deadline, s)
	if !errors.Is(err, agterm.ErrTimeout) || len(types(h)) != 0 {
		t.Fatalf("err %v typed %q", err, types(h))
	}
}

// A screen read that hangs ends with the budget, not never.
func TestTypeWhenIdleHungScreenReadEndsWithTheBudget(t *testing.T) {
	h := newHarness(t, true)
	pollSlice = 100 * time.Millisecond
	h.ctl.WaitErr = agterm.ErrTimeout
	h.ctl.TextBlocks = true
	s := newSettler("round 1", func() bool { return true })
	type res struct {
		started bool
		err     error
	}
	done := make(chan res, 1)
	go func() {
		started, err := h.env.typeWhenIdle(context.Background(), reviewerWorker, "C-1", "line", "round 2", 300*time.Millisecond, time.Time{}, s)
		done <- res{started, err}
	}()
	select {
	case r := <-done:
		if r.started || !errors.Is(r.err, agterm.ErrTimeout) || !strings.Contains(r.err.Error(), "is busy") {
			t.Fatalf("started %v err %v", r.started, r.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("typeWhenIdle hangs on a screen read that never returns")
	}
	if got := types(h); len(got) != 0 {
		t.Fatalf("typed %q", got)
	}
}
