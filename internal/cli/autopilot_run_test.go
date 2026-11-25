package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shurastik/agterm-kit/internal/agterm/agtermtest"
	"github.com/shurastik/agterm-kit/internal/config"
	"github.com/shurastik/agterm-kit/internal/rundir"
)

func rawWorker(t *testing.T, dir string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, workerFile))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestCompanionNameAutopilotPrefix(t *testing.T) {
	if got := companionName("add-auth", "claude", "apply", 1, true); got != "🤖 add-auth · claude-apply r1" {
		t.Errorf("got %q", got)
	}
	if got := companionName("", "codex", "review", 3, true); got != "🤖 codex-review r3" {
		t.Errorf("got %q", got)
	}
	if got := companionName("add-auth", "claude", "apply", 1, false); got != "add-auth · claude-apply r1" {
		t.Errorf("got %q", got)
	}
}

func TestApplyStartAutopilotRecordsNamesAndTints(t *testing.T) {
	h := newHarness(t, true)
	root := repoWith(t, h, "")
	h.ctl.WaitStatus = "active"
	if code := runApply(h, "start", "add-auth", "--autopilot"); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	dir := applyDir(root)
	if s := readApplyState(t, dir); !s.Autopilot || s.AutopilotSession != "SELF" {
		t.Fatalf("worker.json = %+v", s)
	}
	if got := h.ctl.Calls[0]; got.Method != "NewCompanion" || got.Args[1] != "🤖 add-auth · claude-apply r1" {
		t.Fatalf("calls = %v", h.ctl.Calls)
	}
	if got := h.ctl.Calls[1]; got.Method != "Tint" || strings.Join(got.Args, ",") != "COMP-1,purple" {
		t.Fatalf("second call = %v, want the tint right after the companion", got)
	}
	if want := `{"companion":"COMP-1","dir":"` + dir + `","started":true,"worktree":null,"pulled":false}` + "\n"; h.out.String() != want {
		t.Fatalf("stdout = %q", h.out)
	}
}

func TestApplyStartAutopilotInline(t *testing.T) {
	h := newHarness(t, false)
	root := repoWith(t, h, "")
	if code := runApply(h, "start", "add-auth", "--autopilot"); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	s := readApplyState(t, applyDir(root))
	if !s.Autopilot || s.AutopilotSession != "" {
		t.Fatalf("worker.json = %+v", s)
	}
	if len(h.ctl.Calls) != 0 {
		t.Fatalf("agterm calls: %v", h.ctl.Calls)
	}
}

func TestApplyStartWithoutAutopilotRecordsNothing(t *testing.T) {
	h := newHarness(t, true)
	root := repoWith(t, h, "")
	h.ctl.WaitStatus = "active"
	if code := runApply(h, "start", "add-auth"); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	if raw := rawWorker(t, applyDir(root)); strings.Contains(raw, "autopilot") {
		t.Fatalf("worker.json = %s", raw)
	}
	for _, c := range h.ctl.Calls {
		if c.Method == "Tint" {
			t.Fatal("tinted a companion without the flag")
		}
	}
	if h.ctl.Calls[0].Args[1] != "add-auth · claude-apply r1" {
		t.Fatalf("name = %q", h.ctl.Calls[0].Args[1])
	}
}

func TestApplyStartAutopilotTintRefusedIsWarning(t *testing.T) {
	h := newHarness(t, true)
	repoWith(t, h, "")
	h.ctl.WaitStatus = "active"
	h.ctl.TintErr = errors.New("no such color")
	if code := runApply(h, "start", "add-auth", "--autopilot"); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	if !strings.Contains(h.err.String(), "agk: warning: tint applier COMP-1: no such color") {
		t.Fatalf("stderr = %q", h.err)
	}
	if !strings.Contains(h.out.String(), `"companion":"COMP-1"`) || !strings.Contains(h.out.String(), `"started":true`) {
		t.Fatalf("stdout = %q", h.out)
	}
}

func TestApplySendAutopilotRenameKeepsPrefix(t *testing.T) {
	h := newHarness(t, true)
	root := repoWith(t, h, "")
	h.ctl.WaitStatus = "active"
	if code := runApply(h, "start", "add-auth", "--autopilot"); code != 0 {
		t.Fatalf("start: %d %s", code, h.err)
	}
	dir := applyDir(root)
	writeReport(t, dir, 1, doneReport)
	*h = *newHarnessLike(t, h)
	h.ctl.WaitSeq = []agtermtest.WaitResult{{Status: "completed"}, {Status: "active"}}
	msg := filepath.Join(t.TempDir(), "m.md")
	os.WriteFile(msg, []byte("m"), 0o644)
	if code := runApply(h, "send", dir, msg); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	found := false
	for _, c := range h.ctl.Calls {
		if c.Method == "Rename" {
			found = true
			if c.Args[1] != "🤖 add-auth · claude-apply r2" {
				t.Fatalf("rename = %v", c)
			}
		}
		if c.Method == "Tint" {
			t.Fatal("send must not tint again")
		}
	}
	if !found {
		t.Fatal("no rename")
	}
}

func TestReviewStartAutopilotNamesTintsAndRecords(t *testing.T) {
	cases := map[string]struct {
		args []string
		name string
		plan bool
	}{
		"plan":        {[]string{"--change", "add-auth", "--autopilot"}, "🤖 add-auth · codex-plan r1", true},
		"diff for":    {[]string{"--base", "main", "--for", "add-auth", "--autopilot"}, "🤖 add-auth · codex-review r1", false},
		"diff no for": {[]string{"--autopilot"}, "🤖 codex-review r1", false},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t, true)
			root, _ := filepath.EvalSymlinks(t.TempDir())
			if c.plan {
				scriptPlan(h, root)
			} else {
				scriptScope(h, root)
			}
			if code := h.env.ReviewStart(context.Background(), c.args); code != 0 {
				t.Fatalf("exit %d: %s", code, h.err)
			}
			dir := filepath.Join(root, ".agk", "reviews", "20261105-100000")
			if s := readState(t, dir); !s.Autopilot || s.AutopilotSession != "SELF" {
				t.Fatalf("worker.json = %+v", s)
			}
			if h.ctl.Calls[0].Method != "NewCompanion" || h.ctl.Calls[0].Args[1] != c.name {
				t.Fatalf("calls = %v", h.ctl.Calls)
			}
			if got := h.ctl.Calls[1]; got.Method != "Tint" || strings.Join(got.Args, ",") != "COMP-1,purple" {
				t.Fatalf("second call = %v", got)
			}
		})
	}
}

func TestReviewStartAutopilotInline(t *testing.T) {
	h := newHarness(t, false)
	root, _ := filepath.EvalSymlinks(t.TempDir())
	scriptScope(h, root)
	h.drivers["codex"].outputs = []string{threeFindings}
	if code := h.env.ReviewStart(context.Background(), []string{"--autopilot"}); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	s := readState(t, filepath.Join(root, ".agk", "reviews", "20261105-100000"))
	if !s.Autopilot || s.AutopilotSession != "" || len(h.ctl.Calls) != 0 {
		t.Fatalf("state %+v calls %v", s, h.ctl.Calls)
	}
}

func TestReviewStartWithoutAutopilotRecordsNothing(t *testing.T) {
	h := newHarness(t, true)
	root, _ := filepath.EvalSymlinks(t.TempDir())
	scriptScope(h, root)
	if code := h.env.ReviewStart(context.Background(), nil); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	if raw := rawWorker(t, filepath.Join(root, ".agk", "reviews", "20261105-100000")); strings.Contains(raw, "autopilot") {
		t.Fatalf("worker.json = %s", raw)
	}
	for _, c := range h.ctl.Calls {
		if c.Method == "Tint" {
			t.Fatal("tinted a companion without the flag")
		}
	}
}

func TestReviewStartAutopilotTintRefusedIsWarning(t *testing.T) {
	h := newHarness(t, true)
	root, _ := filepath.EvalSymlinks(t.TempDir())
	scriptScope(h, root)
	h.ctl.TintErr = errors.New("nope")
	if code := h.env.ReviewStart(context.Background(), []string{"--autopilot"}); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	if !strings.Contains(h.err.String(), "agk: warning: tint reviewer COMP-1: nope") {
		t.Fatalf("stderr = %q", h.err)
	}
	if !strings.Contains(h.out.String(), `"started":true`) {
		t.Fatalf("stdout = %q", h.out)
	}
}

func autopilotReviewRun(t *testing.T, h *tenv, plan bool, round int) string {
	t.Helper()
	var dir string
	if plan {
		_, dir, _ = planSendDir(t, h, "", true, "T\n")
	} else {
		_, dir = sendDir(t, h, "", true)
	}
	st := reviewState{Worker: config.Worker{Harness: "codex"}, Base: "main", For: "add-auth", Interactive: true,
		Autopilot: true, AutopilotSession: "SELF"}
	if plan {
		st = reviewState{Worker: config.Worker{Harness: "claude"}, Change: "add-x", Interactive: true, Autopilot: true, AutopilotSession: "SELF"}
	}
	if err := rundir.WriteJSON(dir, workerFile, st); err != nil {
		t.Fatal(err)
	}
	for k := 2; k <= round; k++ {
		rundir.MakeRound(dir, k)
		rundir.SetRound(dir, k)
		writeResultFileRound(t, dir, k)
	}
	return dir
}

func writeResultFileRound(t *testing.T, dir string, k int) {
	t.Helper()
	if err := rundir.WriteJSON(rundir.RoundDir(dir, k), "result.json", runResult{Status: "completed", Round: k}); err != nil {
		t.Fatal(err)
	}
}

func TestReviewSendAutopilotRenameKeepsPrefix(t *testing.T) {
	for name, c := range map[string]struct {
		plan  bool
		round int
		want  string
	}{
		"diff": {false, 1, "🤖 add-auth · codex-review r2"},
		"plan": {true, 2, "🤖 add-x · claude-plan r3"},
	} {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t, true)
			dir := autopilotReviewRun(t, h, c.plan, c.round)
			h.ctl.WaitSeq = []agtermtest.WaitResult{{Status: "completed"}, {Status: "active"}}
			if code := runSend(h, dir); code != 0 {
				t.Fatalf("exit %d: %s", code, h.err)
			}
			for _, call := range h.ctl.Calls {
				if call.Method == "Rename" {
					if call.Args[1] != c.want {
						t.Fatalf("rename = %v, want %q", call, c.want)
					}
					return
				}
			}
			t.Fatalf("no rename: %v", h.ctl.Calls)
		})
	}
}
