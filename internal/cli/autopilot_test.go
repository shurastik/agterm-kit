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

	"github.com/shurastik/agterm-kit/internal/agterm/agtermtest"
)

const wantAutopilotLine = `🤖 ^[autopilot](color: "purple")`

func runAutopilot(h *tenv, args ...string) int {
	return h.env.Main(context.Background(), append([]string{"autopilot"}, args...))
}

func TestAutopilotOnSetsLineAndMarks(t *testing.T) {
	h := newHarness(t, true)
	if code := runAutopilot(h, "on"); code != ExitOK {
		t.Fatalf("exit %d, stderr %q", code, h.err)
	}
	if got := strings.TrimSpace(h.out.String()); got != `{"session":"SELF","marked":true}` {
		t.Fatalf("stdout = %q", got)
	}
	assertCalls(t, h.ctl, agtermtest.Call{Method: "Info", Args: []string{"SELF", wantAutopilotLine, "1200"}})
	if !h.env.autopilotLive("SELF") {
		t.Fatal("not live after on")
	}
}

func TestAutopilotOnAgainRestartsExpiry(t *testing.T) {
	h := newHarness(t, true)
	now := h.env.Now()
	clock := now
	h.env.Now = func() time.Time { return clock }
	runAutopilot(h, "on")
	clock = now.Add(10 * time.Minute)
	h.out.Reset()
	if code := runAutopilot(h, "on"); code != ExitOK {
		t.Fatalf("exit %d", code)
	}
	assertCalls(t, h.ctl,
		agtermtest.Call{Method: "Info", Args: []string{"SELF", wantAutopilotLine, "1200"}},
		agtermtest.Call{Method: "Info", Args: []string{"SELF", wantAutopilotLine, "1200"}})
	clock = now.Add(10*time.Minute + 1199*time.Second)
	if !h.env.autopilotLive("SELF") {
		t.Fatal("the second on did not restart the live period")
	}
}

func TestAutopilotOnRenamesAndTintsNothing(t *testing.T) {
	h := newHarness(t, true)
	runAutopilot(h, "on")
	for _, m := range h.ctl.Methods() {
		if m == "Rename" || m == "Tint" || m == "Close" {
			t.Fatalf("on called %s", m)
		}
	}
}

func TestAutopilotOffClearsLineAndDrops(t *testing.T) {
	h := newHarness(t, true)
	runAutopilot(h, "on")
	h.out.Reset()
	if code := runAutopilot(h, "off"); code != ExitOK {
		t.Fatalf("exit %d, stderr %q", code, h.err)
	}
	if got := strings.TrimSpace(h.out.String()); got != `{"session":"SELF","marked":false}` {
		t.Fatalf("stdout = %q", got)
	}
	last := h.ctl.Calls[len(h.ctl.Calls)-1]
	if !reflect.DeepEqual(last, agtermtest.Call{Method: "ClearInfo", Args: []string{"SELF"}}) {
		t.Fatalf("last call = %v", last)
	}
	if h.env.autopilotLive("SELF") {
		t.Fatal("live after off")
	}
	// A later refresh sets nothing.
	n := len(h.ctl.Calls)
	h.env.autopilotRefresh(context.Background(), "SELF")
	if len(h.ctl.Calls) != n {
		t.Fatalf("refresh after off: %v", h.ctl.Calls[n:])
	}
}

func TestAutopilotOffWithoutOn(t *testing.T) {
	h := newHarness(t, true)
	if code := runAutopilot(h, "off"); code != ExitOK {
		t.Fatalf("exit %d, stderr %q", code, h.err)
	}
	assertCalls(t, h.ctl, agtermtest.Call{Method: "ClearInfo", Args: []string{"SELF"}})
	if got := strings.TrimSpace(h.out.String()); got != `{"session":"SELF","marked":false}` {
		t.Fatalf("stdout = %q", got)
	}
}

func TestAutopilotOffTouchesNoCompanion(t *testing.T) {
	h := newHarness(t, true)
	runAutopilot(h, "off")
	for _, m := range h.ctl.Methods() {
		if m != "ClearInfo" {
			t.Fatalf("off called %s", m)
		}
	}
}

func TestAutopilotOutsideAgterm(t *testing.T) {
	for _, arg := range []string{"on", "off"} {
		h := newHarness(t, false)
		h.vars["AGTERM_SESSION_ID"] = "SELF" // enabled is what counts
		if code := runAutopilot(h, arg); code != ExitOK {
			t.Fatalf("%s: exit %d", arg, code)
		}
		if got := strings.TrimSpace(h.out.String()); got != `{"session":null,"marked":false}` {
			t.Fatalf("%s: stdout = %q", arg, got)
		}
		if len(h.ctl.Calls) != 0 {
			t.Fatalf("%s: agterm calls %v", arg, h.ctl.Calls)
		}
		if _, err := os.Stat(h.env.autopilotDir()); err == nil {
			t.Fatalf("%s: wrote state", arg)
		}
	}
}

func TestAutopilotSessionUnsetOutsideAgterm(t *testing.T) {
	h := newHarness(t, true)
	delete(h.vars, "AGTERM_SESSION_ID")
	if code := runAutopilot(h, "on"); code != ExitOK {
		t.Fatalf("exit %d", code)
	}
	if got := strings.TrimSpace(h.out.String()); got != `{"session":null,"marked":false}` || len(h.ctl.Calls) != 0 {
		t.Fatalf("stdout %q calls %v", got, h.ctl.Calls)
	}
}

func TestAutopilotUsage(t *testing.T) {
	for _, args := range [][]string{nil, {"start"}, {"on", "off"}, {"on", "x"}, {""}} {
		h := newHarness(t, true)
		if code := runAutopilot(h, args...); code != ExitError {
			t.Errorf("%q: exit %d, want 1", args, code)
		}
		if !strings.Contains(h.err.String(), "agk autopilot on|off") {
			t.Errorf("%q: stderr = %q", args, h.err)
		}
		if h.out.Len() != 0 || len(h.ctl.Calls) != 0 {
			t.Errorf("%q: stdout %q calls %v", args, h.out, h.ctl.Calls)
		}
		if _, err := os.Stat(h.env.autopilotDir()); err == nil {
			t.Errorf("%q: wrote state", args)
		}
	}
}

func TestAutopilotHelp(t *testing.T) {
	for _, flag := range []string{"--help", "-h"} {
		h := newHarness(t, true)
		if code := runAutopilot(h, flag); code != ExitOK {
			t.Fatalf("%s: exit %d", flag, code)
		}
		if !strings.Contains(h.out.String(), "agk autopilot on|off") || h.err.Len() != 0 {
			t.Fatalf("%s: stdout %q stderr %q", flag, h.out, h.err)
		}
	}
}

func TestAutopilotLineRefused(t *testing.T) {
	h := newHarness(t, true)
	h.ctl.InfoErr = errors.New("agterm said no")
	if code := runAutopilot(h, "on"); code != ExitOK {
		t.Fatalf("exit %d", code)
	}
	if got := strings.TrimSpace(h.out.String()); got != `{"session":"SELF","marked":false}` {
		t.Fatalf("stdout = %q", got)
	}
	if !strings.Contains(h.err.String(), "warning") || !strings.Contains(h.err.String(), "agterm said no") {
		t.Fatalf("stderr = %q", h.err)
	}
	if h.env.autopilotLive("SELF") {
		t.Fatal("a refused line left the autopilot live")
	}
}

func TestAutopilotClearRefusedIsWarning(t *testing.T) {
	h := newHarness(t, true)
	h.ctl.ClearInfoErr = errors.New("no such session")
	if code := runAutopilot(h, "off"); code != ExitOK {
		t.Fatalf("exit %d", code)
	}
	if got := strings.TrimSpace(h.out.String()); got != `{"session":"SELF","marked":false}` {
		t.Fatalf("stdout = %q", got)
	}
	if !strings.Contains(h.err.String(), "no such session") {
		t.Fatalf("stderr = %q", h.err)
	}
}

func TestAutopilotUnsafeSession(t *testing.T) {
	for _, arg := range []string{"on", "off"} {
		h := newHarness(t, true)
		h.vars["AGTERM_SESSION_ID"] = "../evil"
		if code := runAutopilot(h, arg); code != ExitOK {
			t.Fatalf("%s: exit %d", arg, code)
		}
		if got := strings.TrimSpace(h.out.String()); got != `{"session":"../evil","marked":false}` {
			t.Fatalf("%s: stdout = %q", arg, got)
		}
		if !strings.Contains(h.err.String(), "warning") {
			t.Fatalf("%s: stderr = %q", arg, h.err)
		}
		if len(h.ctl.Calls) != 0 {
			t.Fatalf("%s: agterm calls %v", arg, h.ctl.Calls)
		}
		if _, err := os.Stat(filepath.Join(h.env.autopilotDir(), "..", "evil.json")); err == nil {
			t.Fatalf("%s: escaped the state dir", arg)
		}
	}
}

func TestMainUsageListsAutopilot(t *testing.T) {
	if !strings.Contains(usage, "agk autopilot on|off") {
		t.Fatal("agk --help does not list agk autopilot on|off")
	}
}
