package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAutopilotStateDirXDG(t *testing.T) {
	h := newHarness(t, true)
	xdg := t.TempDir()
	h.vars["XDG_STATE_HOME"] = xdg
	if got, want := h.env.autopilotDir(), filepath.Join(xdg, "agk", "autopilot"); got != want {
		t.Fatalf("dir = %q, want %q", got, want)
	}
}

func TestAutopilotStateDirFallback(t *testing.T) {
	h := newHarness(t, true)
	for _, xdg := range []string{"", "relative/state"} {
		h.vars["XDG_STATE_HOME"] = xdg
		want := filepath.Join(h.env.Home, ".local", "state", "agk", "autopilot")
		if got := h.env.autopilotDir(); got != want {
			t.Errorf("XDG_STATE_HOME=%q: dir = %q, want %q", xdg, got, want)
		}
	}
}

func TestAutopilotMarkWritesUntil(t *testing.T) {
	h := newHarness(t, true)
	if err := h.env.autopilotMark("SESS-1"); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(h.env.autopilotDir(), "SESS-1.json"))
	if err != nil {
		t.Fatal(err)
	}
	want := `{"until":"` + h.env.Now().Add(1200*time.Second).Format(time.RFC3339) + `"}`
	if got := string(b); got != want+"\n" && got != want {
		t.Fatalf("file = %q, want %q", got, want)
	}
}

func TestAutopilotLiveUntilThenNot(t *testing.T) {
	h := newHarness(t, true)
	now := h.env.Now()
	clock := now
	h.env.Now = func() time.Time { return clock }
	if h.env.autopilotLive("SESS-1") {
		t.Fatal("live before mark")
	}
	if err := h.env.autopilotMark("SESS-1"); err != nil {
		t.Fatal(err)
	}
	clock = now.Add(1199 * time.Second)
	if !h.env.autopilotLive("SESS-1") {
		t.Fatal("not live before until")
	}
	clock = now.Add(1200 * time.Second)
	if h.env.autopilotLive("SESS-1") {
		t.Fatal("live at until")
	}
	clock = now.Add(1300 * time.Second)
	if h.env.autopilotLive("SESS-1") {
		t.Fatal("live after until")
	}
}

func TestAutopilotLiveBadFile(t *testing.T) {
	h := newHarness(t, true)
	dir := h.env.autopilotDir()
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "SESS-1.json"), []byte("{nope"), 0o644)
	if h.env.autopilotLive("SESS-1") {
		t.Fatal("a broken file counts as live")
	}
}

func TestAutopilotDrop(t *testing.T) {
	h := newHarness(t, true)
	if err := h.env.autopilotDrop("SESS-1"); err != nil {
		t.Fatalf("drop of a missing file: %v", err)
	}
	if err := h.env.autopilotMark("SESS-1"); err != nil {
		t.Fatal(err)
	}
	if err := h.env.autopilotDrop("SESS-1"); err != nil {
		t.Fatal(err)
	}
	if h.env.autopilotLive("SESS-1") {
		t.Fatal("live after drop")
	}
}

func TestAutopilotUnsafeIDNeverLive(t *testing.T) {
	h := newHarness(t, true)
	for _, id := range []string{"", "../x", "a/b", "a.b", "a b", "a\x00b", ".."} {
		if err := h.env.autopilotMark(id); err == nil {
			t.Errorf("mark(%q) succeeded", id)
		}
		if h.env.autopilotLive(id) {
			t.Errorf("live(%q)", id)
		}
	}
	if _, err := os.Stat(h.env.autopilotDir()); err == nil {
		entries, _ := os.ReadDir(h.env.autopilotDir())
		if len(entries) != 0 {
			t.Fatalf("unsafe ids wrote %v", entries)
		}
	}
	if _, err := os.Stat(filepath.Join(h.env.autopilotDir(), "..", "x.json")); err == nil {
		t.Fatal("escaped the state dir")
	}
}

// An off that starts while a refresh is setting the line waits for it: after
// both returned the line is cleared, the autopilot is not live and the next
// refresh sets nothing.
func TestAutopilotOffDuringRefreshIsLast(t *testing.T) {
	h := newHarness(t, true)
	if !h.env.autopilotOn(context.Background(), "SELF") {
		t.Fatal("on failed")
	}
	offDone := make(chan struct{})
	started := false
	h.ctl.OnInfo = func() error {
		if started {
			return nil
		}
		started = true
		go func() {
			h.env.autopilotOff(context.Background(), "SELF")
			close(offDone)
		}()
		// Give the off every chance to run inside the refresh.
		time.Sleep(100 * time.Millisecond)
		return nil
	}
	h.env.autopilotRefresh(context.Background(), "SELF")
	<-offDone
	if h.env.autopilotLive("SELF") {
		t.Fatal("live after off")
	}
	calls := h.ctl.Calls
	if last := calls[len(calls)-1]; last.Method != "ClearInfo" {
		t.Fatalf("last agterm call = %v, want ClearInfo", last)
	}
	n := len(calls)
	h.env.autopilotRefresh(context.Background(), "SELF")
	if len(h.ctl.Calls) != n {
		t.Fatalf("a refresh after off called agterm: %v", h.ctl.Calls[n:])
	}
}

func TestAutopilotRefreshExtendsLive(t *testing.T) {
	h := newHarness(t, true)
	now := h.env.Now()
	clock := now
	h.env.Now = func() time.Time { return clock }
	h.env.autopilotOn(context.Background(), "SELF")
	clock = now.Add(1000 * time.Second)
	h.env.autopilotRefresh(context.Background(), "SELF")
	clock = now.Add(2000 * time.Second) // past the first until, inside the second
	if !h.env.autopilotLive("SELF") {
		t.Fatal("refresh did not extend the live period")
	}
}
