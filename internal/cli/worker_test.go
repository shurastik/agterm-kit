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
	started, err := h.env.typeWhenIdle(context.Background(), reviewerWorker, "C-1", "Read x and follow it.", "round 2", 10*time.Minute, time.Time{})
	if err != nil || !started {
		t.Fatalf("started %v err %v", started, err)
	}
	assertCalls(t, h.ctl,
		agtermtest.Call{Method: "Wait", Args: []string{"C-1", "completed,blocked,idle", "10m0s"}},
		agtermtest.Call{Method: "Type", Args: []string{"C-1", "Read x and follow it.\r"}},
		agtermtest.Call{Method: "Wait", Args: []string{"C-1", "active", "30s"}},
	)
}

func TestTypeWhenIdleUnconfirmedStartWarns(t *testing.T) {
	h := newHarness(t, true)
	h.ctl.WaitSeq = []agtermtest.WaitResult{{Status: "idle"}, {Err: agterm.ErrTimeout}}
	started, err := h.env.typeWhenIdle(context.Background(), reviewerWorker, "C-1", "line", "round 2", time.Minute, time.Time{})
	if err != nil || started {
		t.Fatalf("started %v err %v", started, err)
	}
	if !strings.Contains(h.err.String(), "reviewer C-1 did not report round 2 as started") {
		t.Fatalf("stderr = %q", h.err)
	}
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
			started, err := h.env.typeWhenIdle(context.Background(), reviewerWorker, "C-1", "line", "round 2", 30*time.Second, time.Time{})
			if err == nil || started || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("started %v err %v, want %q", started, err, c.want)
			}
			if name != "type gone" && name != "type fails" && !reflect.DeepEqual(h.ctl.Methods(), []string{"Wait"}) {
				t.Fatalf("calls = %v: nothing may be typed", h.ctl.Calls)
			}
		})
	}
}
