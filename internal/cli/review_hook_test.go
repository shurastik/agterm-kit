package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/shurastik/agterm-kit/internal/agterm/agtermtest"
)

type settingsHooks struct {
	Hooks map[string][]struct {
		Matcher string `json:"matcher"`
		Hooks   []struct {
			Type    string `json:"type"`
			Command string `json:"command"`
		} `json:"hooks"`
	} `json:"hooks"`
}

func TestReviewerSettingsHooks(t *testing.T) {
	b, err := reviewerSettings("/usr/local/bin/agk")
	if err != nil {
		t.Fatal(err)
	}
	var s settingsHooks
	if err := json.Unmarshal(b, &s); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, b)
	}
	want := map[string]struct{ matcher, cmd string }{
		"UserPromptSubmit": {"", "'/usr/local/bin/agk' review hook active"},
		"Stop":             {"", "'/usr/local/bin/agk' review hook completed"},
		"Notification":     {"permission_prompt", "'/usr/local/bin/agk' review hook blocked"},
	}
	if len(s.Hooks) != len(want) {
		t.Fatalf("hook events = %v", s.Hooks)
	}
	for event, w := range want {
		entries := s.Hooks[event]
		if len(entries) != 1 || entries[0].Matcher != w.matcher || len(entries[0].Hooks) != 1 {
			t.Fatalf("%s = %+v", event, entries)
		}
		if h := entries[0].Hooks[0]; h.Type != "command" || h.Command != w.cmd {
			t.Fatalf("%s hook = %+v, want %q", event, h, w.cmd)
		}
	}
}

func TestReviewerSettingsQuotesAndRefuses(t *testing.T) {
	b, err := reviewerSettings("/opt/it's/agk")
	if err != nil || !strings.Contains(string(b), `'/opt/it'\\''s/agk' review hook active`) {
		t.Fatalf("quoting: %v\n%s", err, b)
	}
	if _, err := reviewerSettings(`/opt/a\b/agk`); err == nil {
		t.Fatal("a path with a backslash must be refused")
	}
}

func TestWriteReviewerSettings(t *testing.T) {
	h := newHarness(t, true)
	_, dir := runDir(t)
	p, err := h.env.writeReviewerSettings(dir)
	if err != nil || p != filepath.Join(dir, "reviewer-settings.json") {
		t.Fatalf("path %q err %v", p, err)
	}
	b, _ := os.ReadFile(p)
	if !strings.Contains(string(b), "/usr/local/bin/agk' review hook completed") {
		t.Fatalf("settings = %s", b)
	}
}

func TestReviewHook(t *testing.T) {
	for _, state := range []string{"active", "completed", "blocked"} {
		h := newHarness(t, true)
		h.vars["AGTERM_SESSION_ID"] = "COMP-9"
		if code := h.env.Main(context.Background(), []string{"review", "hook", state}); code != 0 {
			t.Fatalf("%s: exit %d: %s", state, code, h.err)
		}
		assertCalls(t, h.ctl, agtermtest.Call{Method: "Status", Args: []string{"COMP-9", state}})
		if h.out.Len() != 0 {
			t.Fatalf("a hook prints nothing: %q", h.out)
		}
	}
}

func TestReviewHookWithoutSessionDoesNothing(t *testing.T) {
	h := newHarness(t, false)
	if code := h.env.Main(context.Background(), []string{"review", "hook", "completed"}); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	if len(h.ctl.Calls) != 0 {
		t.Fatalf("agterm must not be called: %v", h.ctl.Calls)
	}
}

func TestReviewHookRefusals(t *testing.T) {
	for name, args := range map[string][]string{
		"unknown state": {"review", "hook", "dancing"},
		"no state":      {"review", "hook"},
		"two states":    {"review", "hook", "active", "blocked"},
	} {
		h := newHarness(t, true)
		if code := h.env.Main(context.Background(), args); code != ExitError {
			t.Errorf("%s: exit %d", name, code)
		}
		if len(h.ctl.Calls) != 0 {
			t.Errorf("%s: agterm called: %v", name, h.ctl.Calls)
		}
	}
	h := newHarness(t, true)
	h.ctl.StatusErr = os.ErrPermission
	if code := h.env.Main(context.Background(), []string{"review", "hook", "active"}); code != ExitError || !reflect.DeepEqual(h.ctl.Methods(), []string{"Status"}) {
		t.Fatalf("a failing status exits 1: %d %v", code, h.ctl.Methods())
	}
}
