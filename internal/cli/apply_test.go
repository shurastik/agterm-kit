package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/shurastik/agterm-kit/internal/agterm"
	"github.com/shurastik/agterm-kit/internal/agterm/agtermtest"
	"github.com/shurastik/agterm-kit/internal/apply"
	"github.com/shurastik/agterm-kit/internal/config"
	"github.com/shurastik/agterm-kit/internal/harness"
	"github.com/shurastik/agterm-kit/internal/rundir"
	"github.com/shurastik/agterm-kit/internal/worktree"
)

func runApply(h *tenv, args ...string) int {
	return h.env.Main(context.Background(), append([]string{"apply"}, args...))
}

func applyDir(root string) string { return filepath.Join(root, ".agk", "applies", "20261105-100000") }

func readApplyState(t *testing.T, dir string) applyState {
	t.Helper()
	var s applyState
	if have, err := rundir.ReadJSON(dir, workerFile, &s); !have || err != nil {
		t.Fatalf("worker.json: %v %v", have, err)
	}
	return s
}

func writeReport(t *testing.T, dir string, k int, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(rundir.RoundDir(dir, k), apply.ReportFile), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

const doneReport = `{"status":"done","progress":"7/7","summary":"all tasks, go test pass"}`

func TestApplyStartClaude(t *testing.T) {
	h := newHarness(t, true)
	root := repoWith(t, h, "")
	h.ctl.WaitStatus = "active"
	if code := runApply(h, "start", "add-auth"); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	dir := applyDir(root)
	s := readApplyState(t, dir)
	if s.Change != "add-auth" || s.Session != "SESS-NEW" || !reflect.DeepEqual(s.Worker, config.Worker{Harness: "claude", PermissionMode: "auto"}) {
		t.Fatalf("worker.json = %+v", s)
	}
	if k, _ := rundir.Round(dir); k != 1 {
		t.Fatalf("round = %d", k)
	}
	prompt := filepath.Join(dir, "rounds", "1", "prompt.md")
	p, _ := os.ReadFile(prompt)
	if !strings.Contains(string(p), "openspec instructions apply --change add-auth --json") ||
		!strings.Contains(string(p), filepath.Join(dir, "rounds", "1", apply.ReportFile)) {
		t.Fatalf("prompt = %q", p)
	}
	if strings.HasPrefix(string(p), apply.Rules()) {
		t.Fatal("claude gets the rules as a system prompt, not in the prompt")
	}
	if b, _ := os.ReadFile(filepath.Join(dir, rulesFile)); string(b) != apply.Rules() {
		t.Fatal("rules file missing")
	}
	wantSettings, err := applierSettings("/usr/local/bin/agk", dir)
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, settingsFile)); string(b) != string(wantSettings) {
		t.Fatalf("settings file = %q, want %q", b, wantSettings)
	}
	if !s.TurnHooks {
		t.Fatal("a claude run records its turn hooks in worker.json")
	}
	kickoff := "Read " + prompt + " and follow it."
	wantCmd := "'claude' '--model=' '--session=SESS-NEW' '" + kickoff + "'"
	assertCalls(t, h.ctl,
		agtermtest.Call{Method: "NewCompanion", Args: []string{"SELF", "add-auth · claude-apply r1", wantCmd, root}},
		agtermtest.Call{Method: "Wait", Args: []string{"COMP-1", "active,completed,blocked", "1m0s"}},
	)
	for _, m := range h.ctl.Methods() {
		if strings.Contains(strings.ToLower(m), "rename") {
			t.Fatalf("agk must not rename a session, got %s", m)
		}
	}
	if want := `{"companion":"COMP-1","dir":"` + dir + `","started":true,"worktree":null,"pulled":false}` + "\n"; h.out.String() != want {
		t.Fatalf("stdout = %q", h.out)
	}
	if id, _ := os.ReadFile(filepath.Join(dir, CompanionFile)); string(id) != "COMP-1\n" {
		t.Fatalf("companion file = %q", id)
	}
}

func TestApplyStartCodex(t *testing.T) {
	h := newHarness(t, true)
	root := repoWith(t, h, "")
	h.ctl.WaitStatus = "active"
	if code := runApply(h, "start", "--applier", "codex:gpt-5.5", "add-auth"); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	dir := applyDir(root)
	if name := h.ctl.Calls[0].Args[1]; name != "add-auth · codex-apply r1" {
		t.Fatalf("name = %q", name)
	}
	if s := readApplyState(t, dir); s.Session != "" || s.Worker.Model != "gpt-5.5" || s.Worker.PermissionMode != "" || s.TurnHooks {
		t.Fatalf("worker.json = %+v", s)
	}
	for _, f := range []string{settingsFile, rulesFile} {
		if exists(filepath.Join(dir, f)) {
			t.Fatalf("codex run wrote %s", f)
		}
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "worker.json")); strings.Contains(string(b), "turn_hooks") {
		t.Fatalf("worker.json = %s", b)
	}
	p, _ := os.ReadFile(filepath.Join(dir, "rounds", "1", "prompt.md"))
	if !strings.HasPrefix(string(p), apply.Rules()) {
		t.Fatal("codex gets the rules at the top of the prompt")
	}
	if cmd := h.ctl.Calls[0].Args[2]; !strings.HasPrefix(cmd, "'codex' '--model=gpt-5.5' '--session=' ") {
		t.Fatalf("cmd = %q", cmd)
	}
}

// A claude run whose hook command cannot be quoted is refused before any
// companion exists.
func TestApplyStartRefusesPathsThatCannotGoIntoAHookCommand(t *testing.T) {
	t.Run("agk binary", func(t *testing.T) {
		h := newHarness(t, true)
		root := repoWith(t, h, "")
		h.env.Executable = func() (string, error) { return `/opt/a\b/agk`, nil }
		h.ctl.WaitStatus = "active"
		if code := runApply(h, "start", "add-auth"); code != 1 {
			t.Fatalf("exit %d, stdout %q", code, h.out)
		}
		if !strings.Contains(h.err.String(), strconv.Quote(`/opt/a\b/agk`)) {
			t.Fatalf("stderr = %q, want the path named", h.err)
		}
		if h.out.Len() != 0 || len(h.ctl.Calls) != 0 {
			t.Fatalf("stdout %q, calls %v: no companion may be created", h.out, h.ctl.Calls)
		}
		if exists(filepath.Join(applyDir(root), CompanionFile)) {
			t.Fatal("companion file written")
		}
	})
	t.Run("run directory, inline", func(t *testing.T) {
		h := newHarness(t, false)
		bad := filepath.Join(t.TempDir(), `a\b`)
		if err := os.MkdirAll(bad, 0o755); err != nil {
			t.Fatal(err)
		}
		h.env.RepoRoot = func(context.Context, string) (string, error) { return bad, nil }
		if code := runApply(h, "start", "add-auth"); code != 1 {
			t.Fatalf("exit %d, stdout %q", code, h.out)
		}
		if !strings.Contains(h.err.String(), strconv.Quote(applyDir(bad))) {
			t.Fatalf("stderr = %q, want the path named", h.err)
		}
		if len(h.drivers["claude"].headless) != 0 {
			t.Fatal("the applier must not run")
		}
	})
	t.Run("codex needs no hook command", func(t *testing.T) {
		h := newHarness(t, true)
		repoWith(t, h, "")
		h.env.Executable = func() (string, error) { return `/opt/a\b/agk`, nil }
		h.ctl.WaitStatus = "active"
		if code := runApply(h, "start", "--applier", "codex", "add-auth"); code != 0 {
			t.Fatalf("exit %d: %s", code, h.err)
		}
	})
}

func TestApplyStartRefusals(t *testing.T) {
	cases := map[string]struct {
		change string
		state  func(context.Context, string, string) (string, string, error)
		want   string
	}{
		"not kebab-case": {"Add_Auth", nil, "kebab-case"},
		"flag-like":      {"-x", nil, ""},
		"all done":       {"add-auth", func(context.Context, string, string) (string, string, error) { return "all_done", "", nil }, "nothing to apply"},
		"blocked": {"add-auth", func(context.Context, string, string) (string, string, error) {
			return "blocked", "missing artifacts: specs", nil
		}, "missing artifacts: specs"},
		"unknown change": {"no-such", func(context.Context, string, string) (string, string, error) {
			return "", "", errors.New(`Change "no-such" not found`)
		}, "not found"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t, true)
			root := repoWith(t, h, "")
			if c.state != nil {
				h.env.ApplyState = c.state
			}
			if code := runApply(h, "start", c.change); code != 1 {
				t.Fatalf("exit %d", code)
			}
			if !strings.Contains(h.err.String(), c.want) || h.out.Len() != 0 {
				t.Fatalf("stderr %q stdout %q", h.err, h.out)
			}
			if exists(filepath.Join(root, ".agk")) || len(h.ctl.Calls) != 0 {
				t.Fatalf("no run dir and no companion expected: %v", h.ctl.Calls)
			}
		})
	}
}

func TestApplyStartOutsideRepo(t *testing.T) {
	h := newHarness(t, true)
	if code := runApply(h, "start", "add-auth"); code != 1 || !strings.Contains(h.err.String(), "git repository") {
		t.Fatalf("exit %d, %q", code, h.err)
	}
}

func TestApplyStartHandshakeTimeout(t *testing.T) {
	h := newHarness(t, true)
	root := repoWith(t, h, "")
	h.ctl.WaitErr = agterm.ErrTimeout
	h.ctl.TextOut = "Accessing workspace:\n\x1b[1mQuick safety check: Is this a project you trust?\n❯ No, exit\n"
	if code := runApply(h, "start", "add-auth"); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	if !strings.Contains(h.out.String(), `"started":false`) {
		t.Fatalf("stdout = %q", h.out)
	}
	if !strings.Contains(h.err.String(), "trust") || strings.ContainsRune(h.err.String(), 0x1b) {
		t.Fatalf("stderr = %q", h.err)
	}
	if got := h.ctl.Methods(); !reflect.DeepEqual(got, []string{"NewCompanion", "Wait", "Text"}) {
		t.Fatalf("calls = %v (the companion stays open)", h.ctl.Calls)
	}
	if h.ctl.Calls[2].Args[0] != "COMP-1" {
		t.Fatalf("text target = %v", h.ctl.Calls[2])
	}
	_ = root
}

func TestApplyStartSameWorkerWarns(t *testing.T) {
	h := newHarness(t, true)
	repoWith(t, h, "[reviewer]\nharness = \"claude\"\n")
	h.ctl.WaitStatus = "active"
	if code := runApply(h, "start", "add-auth"); code != 0 || !strings.Contains(h.err.String(), "not independent") {
		t.Fatalf("exit %d, %q", code, h.err)
	}
}

func TestApplyInline(t *testing.T) {
	h := newHarness(t, false)
	root := repoWith(t, h, "")
	claude := h.drivers["claude"]
	claude.onHeadless = func(tr harness.Turn) {
		os.WriteFile(filepath.Join(tr.Dir, apply.ReportFile), []byte(doneReport), 0o644)
	}
	if code := runApply(h, "start", "add-auth"); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	dir := applyDir(root)
	prompt := filepath.Join(dir, "rounds", "1", "prompt.md")
	want := harness.Turn{Worker: config.Worker{Harness: "claude", PermissionMode: "auto"}, Root: root,
		Dir: filepath.Join(dir, "rounds", "1"), Log: applierLog, Prompt: "Read " + prompt + " and follow it.", Session: "SESS-NEW"}
	if len(claude.headless) != 1 || !reflect.DeepEqual(claude.headless[0], want) {
		t.Fatalf("headless = %+v", claude.headless)
	}
	if f := claude.files[0]; f.Settings != filepath.Join(dir, settingsFile) || f.Rules != filepath.Join(dir, rulesFile) {
		t.Fatalf("files = %+v", f)
	}
	if w := `{"companion":null,"dir":"` + dir + `","started":true,"worktree":null,"pulled":false}` + "\n"; h.out.String() != w {
		t.Fatalf("stdout = %q", h.out)
	}
	if !strings.Contains(h.err.String(), "claude applying") {
		t.Fatal("applier output goes to stderr")
	}
	h.out.Reset()
	if code := runApply(h, "wait", dir); code != 0 {
		t.Fatalf("wait exit %d: %s", code, h.err)
	}
	if !strings.Contains(h.out.String(), `"status":"done","round":1`) || len(h.ctl.Calls) != 0 {
		t.Fatalf("wait stdout %q calls %v", h.out, h.ctl.Calls)
	}

	// Follow-up resumes the recorded session.
	h.out.Reset()
	note := filepath.Join(t.TempDir(), "notes.md")
	os.WriteFile(note, []byte("fix the nil check"), 0o644)
	if code := runApply(h, "send", dir, note); code != 0 {
		t.Fatalf("send exit %d: %s", code, h.err)
	}
	if tr := claude.headless[1]; !tr.Resume || tr.Session != "SESS-NEW" || tr.Dir != filepath.Join(dir, "rounds", "2") {
		t.Fatalf("round 2 turn = %+v", tr)
	}
	if h.out.String() != `{"round":2}`+"\n" || len(h.ctl.Calls) != 0 {
		t.Fatalf("send stdout %q calls %v", h.out, h.ctl.Calls)
	}
}

func TestApplyInlineCodexRecordsThread(t *testing.T) {
	h := newHarness(t, false)
	root := repoWith(t, h, "[applier]\nharness = \"codex\"\n")
	h.drivers["codex"].sessions = []string{"T-9"}
	if code := runApply(h, "start", "add-auth"); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	if s := readApplyState(t, applyDir(root)); s.Session != "T-9" {
		t.Fatalf("session = %q", s.Session)
	}
	if f := h.drivers["codex"].files[0]; f != (harness.ApplierFiles{}) {
		t.Fatalf("codex takes no files: %+v", f)
	}
}

// startedApply makes an agterm apply run with companion C-1 at round k.
func startedApply(t *testing.T, h *tenv, k int) string {
	t.Helper()
	root := repoWith(t, h, "")
	h.ctl.WaitStatus = "active"
	if code := runApply(h, "start", "add-auth"); code != 0 {
		t.Fatalf("start: exit %d: %s", code, h.err)
	}
	dir := applyDir(root)
	os.WriteFile(filepath.Join(dir, CompanionFile), []byte("C-1\n"), 0o644)
	for i := 2; i <= k; i++ {
		rundir.MakeRound(dir, i)
		rundir.SetRound(dir, i)
	}
	*h = *newHarnessLike(t, h)
	return dir
}

// newHarnessLike is a fresh harness (clean calls and output) sharing h's repo.
func newHarnessLike(t *testing.T, h *tenv) *tenv {
	n := newHarness(t, h.env.Getenv("AGTERM_ENABLED") != "")
	n.env.RepoRoot = h.env.RepoRoot
	return n
}

func TestApplyWait(t *testing.T) {
	cases := map[string]struct {
		prep func(h *tenv, dir string)
		args []string
		want string
		wait bool
	}{
		"done while waiting": {func(h *tenv, dir string) {
			h.ctl.WaitStatus = "completed"
			h.ctl.OnWait = func() { writeReport(t, dir, 1, doneReport) }
		}, nil, `{"status":"done","round":1,"progress":"7/7","summary":"all tasks, go test pass"}`, true},
		"report already there": {func(h *tenv, dir string) { writeReport(t, dir, 1, doneReport) }, nil, `"status":"done"`, false},
		"permission":           {func(h *tenv, dir string) { h.ctl.WaitStatus = "blocked" }, nil, `{"status":"needs-human","round":1,"reason":"permission"}`, true},
		"no report":            {func(h *tenv, dir string) { h.ctl.WaitStatus = "idle" }, nil, `{"status":"needs-human","round":1,"reason":"no-report"}`, true},
		"garbage report":       {func(h *tenv, dir string) { writeReport(t, dir, 1, "nope") }, nil, `{"status":"needs-human","round":1,"reason":"invalid-report"}`, false},
		"cancelled":            {func(h *tenv, dir string) { h.ctl.WaitErr = agterm.ErrGone }, nil, `{"status":"cancelled","round":1}`, true},
		"running":              {func(h *tenv, dir string) { h.ctl.WaitErr = agterm.ErrTimeout }, []string{"--timeout", "60"}, `{"status":"running","round":1}`, true},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t, true)
			dir := startedApply(t, h, 1)
			setTurnHooks(t, dir, false) // the status alone decides: see apply_wait_hooked_test.go for hooked runs
			c.prep(h, dir)
			if code := runApply(h, append([]string{"wait", dir}, c.args...)...); code != 0 {
				t.Fatalf("exit %d: %s", code, h.err)
			}
			if !strings.Contains(h.out.String(), c.want) {
				t.Fatalf("stdout = %q, want %q", h.out, c.want)
			}
			for _, m := range h.ctl.Methods() {
				if m == "Close" {
					t.Fatal("wait must not close the applier")
				}
			}
			if c.wait {
				timeout := "1h0m0s"
				if c.args != nil {
					timeout = "1m0s"
				}
				assertCalls(t, h.ctl, agtermtest.Call{Method: "Wait", Args: []string{"C-1", "completed,blocked,idle", timeout}})
			} else {
				assertCalls(t, h.ctl)
			}
		})
	}
}

func TestApplyWaitErrors(t *testing.T) {
	h := newHarness(t, true)
	dir := startedApply(t, h, 1)
	h.ctl.WaitErr = errors.New("socket refused")
	if code := runApply(h, "wait", dir); code != 1 || h.out.Len() != 0 {
		t.Fatalf("exit %d stdout %q", code, h.out)
	}
	if code := runApply(h, "wait", filepath.Dir(dir)); code != 1 {
		t.Fatal("bad dir accepted")
	}
}

func TestApplySend(t *testing.T) {
	h := newHarness(t, true)
	dir := startedApply(t, h, 1)
	writeReport(t, dir, 1, doneReport)
	h.ctl.WaitSeq = []agtermtest.WaitResult{{Status: "completed"}, {Status: "active"}}
	msg := filepath.Join(t.TempDir(), "findings.md")
	os.WriteFile(msg, []byte("1. nil deref in x.go:10\n2. missing test"), 0o644)
	if code := runApply(h, "send", dir, msg); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	prompt := filepath.Join(dir, "rounds", "2", "prompt.md")
	p, _ := os.ReadFile(prompt)
	if !strings.Contains(string(p), "nil deref in x.go:10") || !strings.Contains(string(p), filepath.Join(dir, "rounds", "2", apply.ReportFile)) {
		t.Fatalf("prompt = %q", p)
	}
	if k, _ := rundir.Round(dir); k != 2 {
		t.Fatalf("round = %d", k)
	}
	assertCalls(t, h.ctl,
		agtermtest.Call{Method: "Wait", Args: []string{"C-1", "completed,blocked,idle", "10m0s"}},
		agtermtest.Call{Method: "Type", Args: []string{"C-1", "Read " + prompt + " and follow it.\r"}},
		agtermtest.Call{Method: "Wait", Args: []string{"C-1", "active", "10s"}},
		agtermtest.Call{Method: "Rename", Args: []string{"C-1", "add-auth · claude-apply r2"}},
	)
	if h.out.String() != `{"round":2,"started":true}`+"\n" {
		t.Fatalf("stdout = %q", h.out)
	}
}

func TestApplySendStdin(t *testing.T) {
	h := newHarness(t, true)
	dir := startedApply(t, h, 1)
	h.ctl.WaitSeq = []agtermtest.WaitResult{{Status: "idle"}, {Status: "active"}}
	h.env.Stdin = strings.NewReader("from stdin")
	if code := runApply(h, "send", "--stdin", dir); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	if p, _ := os.ReadFile(filepath.Join(dir, "rounds", "2", "prompt.md")); !strings.Contains(string(p), "from stdin") {
		t.Fatalf("prompt = %q", p)
	}
}

func TestApplySendRefusals(t *testing.T) {
	cases := map[string]struct {
		prep func(h *tenv)
		args []string
		want string
	}{
		"busy":         {func(h *tenv) { h.ctl.WaitSeq = []agtermtest.WaitResult{{Err: agterm.ErrTimeout}} }, []string{"--timeout", "30"}, "busy"},
		"permission":   {func(h *tenv) { h.ctl.WaitSeq = []agtermtest.WaitResult{{Status: "blocked"}} }, nil, "waits for the user"},
		"gone":         {func(h *tenv) { h.ctl.WaitSeq = []agtermtest.WaitResult{{Err: agterm.ErrGone}} }, nil, "gone"},
		"type gone":    {func(h *tenv) { h.ctl.WaitStatus = "completed"; h.ctl.TypeErr = agterm.ErrGone }, nil, "gone"},
		"empty":        {func(h *tenv) { h.env.Stdin = strings.NewReader("  \n") }, []string{"--stdin"}, "empty"},
		"two messages": {func(h *tenv) {}, []string{"extra.md"}, "usage"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t, true)
			dir := startedApply(t, h, 1)
			c.prep(h)
			args := []string{"send", dir}
			if !reflect.DeepEqual(c.args, []string{"--stdin"}) {
				msg := filepath.Join(t.TempDir(), "m.md")
				os.WriteFile(msg, []byte("m"), 0o644)
				args = append(args, msg)
			}
			args = append(args, c.args...)
			if code := runApply(h, args...); code != 1 {
				t.Fatalf("exit %d (stderr %q)", code, h.err)
			}
			if !strings.Contains(h.err.String(), c.want) || h.out.Len() != 0 {
				t.Fatalf("stderr %q stdout %q", h.err, h.out)
			}
			if k, _ := rundir.Round(dir); k != 1 || exists(filepath.Join(dir, "rounds", "2")) {
				t.Fatalf("no round may be recorded: %d", k)
			}
			for _, call := range h.ctl.Calls {
				if call.Method == "Type" && name != "type gone" {
					t.Fatal("nothing may be typed")
				}
				if call.Method == "Rename" {
					t.Fatalf("a refused round must not rename the companion: %v", h.ctl.Calls)
				}
			}
		})
	}
}

func TestApplySendHandshakeTimeoutWarns(t *testing.T) {
	h := newHarness(t, true)
	dir := startedApply(t, h, 1)
	h.ctl.WaitSeq = []agtermtest.WaitResult{{Status: "completed"}, {Err: agterm.ErrTimeout}}
	msg := filepath.Join(t.TempDir(), "m.md")
	os.WriteFile(msg, []byte("m"), 0o644)
	if code := runApply(h, "send", dir, msg); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	if !strings.Contains(h.err.String(), "did not report round 2 as started") || h.out.String() != "{\"round\":2,\"started\":false}\n" {
		t.Fatalf("stderr %q stdout %q", h.err, h.out)
	}
	if k, _ := rundir.Round(dir); k != 2 {
		t.Fatalf("round = %d, want 2 kept", k)
	}
	if exists(filepath.Join(rundir.RoundDir(dir, 2), startedFile)) {
		t.Fatal("round 2 must not be marked started without active")
	}
	if c := h.ctl.Calls[len(h.ctl.Calls)-1]; c.Method != "Rename" || c.Args[1] != "add-auth · claude-apply r2" {
		t.Fatalf("an unconfirmed round stays recorded, so the companion is renamed: %v", h.ctl.Calls)
	}
}

func TestApplySendRenameFailureIsAWarning(t *testing.T) {
	h := newHarness(t, true)
	dir := startedApply(t, h, 1)
	h.ctl.WaitSeq = []agtermtest.WaitResult{{Status: "completed"}, {Status: "active"}}
	h.ctl.RenameErr = errors.New("agterm said no")
	msg := filepath.Join(t.TempDir(), "m.md")
	os.WriteFile(msg, []byte("m"), 0o644)
	if code := runApply(h, "send", dir, msg); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	if !strings.Contains(h.err.String(), "agk: warning: rename applier C-1: agterm said no") {
		t.Fatalf("stderr %q", h.err)
	}
	if h.out.String() != `{"round":2,"started":true}`+"\n" {
		t.Fatalf("stdout = %q", h.out)
	}
}

func TestApplyClose(t *testing.T) {
	h := newHarness(t, true)
	dir := startedApply(t, h, 1)
	if code := runApply(h, "close", dir); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	assertCalls(t, h.ctl, agtermtest.Call{Method: "Close", Args: []string{"C-1"}})
	h2 := newHarness(t, true)
	h2.ctl.CloseErr = agterm.ErrGone
	if code := runApply(h2, "close", dir); code != 0 {
		t.Fatalf("gone: exit %d", code)
	}
}

func TestApplyHelp(t *testing.T) {
	h := newHarness(t, true)
	if code := runApply(h, "--help"); code != 0 {
		t.Fatalf("exit %d", code)
	}
	for _, sub := range []string{"agk apply start", "agk apply wait", "agk apply send", "agk apply close"} {
		if !strings.Contains(h.out.String(), sub) || !strings.Contains(usage, sub) {
			t.Errorf("help lacks %q", sub)
		}
	}
}

// A round whose start was never confirmed (the handshake timed out, e.g. on a
// trust prompt) must not read a fresh session's idle as "turn over".
func TestApplyWaitBeforeTheRoundStarted(t *testing.T) {
	h := newHarness(t, true)
	root := repoWith(t, h, "")
	h.ctl.WaitErr = agterm.ErrTimeout
	if code := runApply(h, "start", "add-auth"); code != 0 {
		t.Fatalf("start: exit %d: %s", code, h.err)
	}
	dir := applyDir(root)
	if exists(filepath.Join(rundir.RoundDir(dir, 1), startedFile)) {
		t.Fatal("an unconfirmed round must not be marked started")
	}
	setTurnHooks(t, dir, false) // the old path: hooked runs are in apply_wait_hooked_test.go
	h = newHarnessLike(t, h)
	h.ctl.WaitSeq = []agtermtest.WaitResult{{Status: "active"}, {Status: "completed"}}
	n := 0
	h.ctl.OnWait = func() {
		if n++; n == 2 {
			writeReport(t, dir, 1, doneReport)
		}
	}
	if code := runApply(h, "wait", dir); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	if !strings.Contains(h.out.String(), `"status":"done"`) {
		t.Fatalf("stdout = %q", h.out)
	}
	assertCalls(t, h.ctl,
		agtermtest.Call{Method: "Wait", Args: []string{"COMP-1", "active,blocked,completed", "1h0m0s"}},
		agtermtest.Call{Method: "Wait", Args: []string{"COMP-1", "completed,blocked,idle", "1h0m0s"}},
	)
	if !exists(filepath.Join(rundir.RoundDir(dir, 1), startedFile)) {
		t.Fatal("seeing active marks the round started")
	}

	// Still not started when the timeout passes: running, not no-report.
	h = newHarnessLike(t, h)
	os.Remove(filepath.Join(rundir.RoundDir(dir, 1), startedFile))
	os.Remove(filepath.Join(rundir.RoundDir(dir, 1), apply.ReportFile))
	h.ctl.WaitErr = agterm.ErrTimeout
	if code := runApply(h, "wait", dir, "--timeout", "5"); code != 0 || !strings.Contains(h.out.String(), `"status":"running"`) {
		t.Fatalf("exit %d stdout %q", code, h.out)
	}
}

func TestApplyStartAndSendMarkRoundsStarted(t *testing.T) {
	h := newHarness(t, true)
	dir := startedApply(t, h, 1)
	if !exists(filepath.Join(rundir.RoundDir(dir, 1), startedFile)) {
		t.Fatal("round 1 confirmed by the start handshake")
	}
	h.ctl.WaitSeq = []agtermtest.WaitResult{{Status: "completed"}, {Status: "active"}}
	msg := filepath.Join(t.TempDir(), "m.md")
	os.WriteFile(msg, []byte("m"), 0o644)
	if code := runApply(h, "send", dir, msg); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	if !exists(filepath.Join(rundir.RoundDir(dir, 2), startedFile)) {
		t.Fatal("round 2 confirmed by the send handshake")
	}
}

// Without agent-status hooks (seen with codex) the status never changes; the
// report on disk must still end the wait.
func TestApplyWaitPollsTheReport(t *testing.T) {
	h := newHarness(t, true)
	dir := startedApply(t, h, 1)
	pollSlice = 5 * time.Second // newHarness restores it after the test
	h.ctl.WaitErr = agterm.ErrTimeout
	n := 0
	h.ctl.OnWait = func() {
		if n++; n == 3 {
			writeReport(t, dir, 1, doneReport)
		}
	}
	if code := runApply(h, "wait", dir, "--timeout", "60"); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	if !strings.Contains(h.out.String(), `"status":"done"`) || n != 3 {
		t.Fatalf("stdout %q after %d waits", h.out, n)
	}
	for _, c := range h.ctl.Calls {
		if c.Args[2] != "5s" {
			t.Fatalf("slice = %v", c)
		}
	}

	// No report ever: the slices add up to the timeout, then running.
	h = newHarnessLike(t, h)
	pollSlice = 5 * time.Second
	os.Remove(filepath.Join(rundir.RoundDir(dir, 1), apply.ReportFile))
	h.ctl.WaitErr = agterm.ErrTimeout
	if code := runApply(h, "wait", dir, "--timeout", "12"); code != 0 || !strings.Contains(h.out.String(), `"status":"running"`) {
		t.Fatalf("exit %d stdout %q", code, h.out)
	}
	if got := len(h.ctl.Calls); got != 3 {
		t.Fatalf("waits = %d, want 3 (5s+5s+2s)", got)
	}
	if last := h.ctl.Calls[2].Args[2]; last != "2s" {
		t.Fatalf("last slice = %s", last)
	}
}

func TestApplyInlineFailedStartKeepsNoSession(t *testing.T) {
	h := newHarness(t, false)
	root := repoWith(t, h, "")
	claude := h.drivers["claude"]
	claude.sessions = []string{""}
	claude.errs = []error{errors.New("claude failed: exec: claude: not found")}
	if code := runApply(h, "start", "add-auth"); code != 1 {
		t.Fatalf("exit %d", code)
	}
	if s := readApplyState(t, applyDir(root)); s.Session != "" {
		t.Fatalf("an unconfirmed session must not be kept: %q", s.Session)
	}
}

func TestApplyStartMalformedApplierFlag(t *testing.T) {
	h := newHarness(t, true)
	root := repoWith(t, h, "")
	if code := runApply(h, "start", "--applier", "gemini", "add-auth"); code != 1 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(h.err.String(), "--applier") || h.out.Len() != 0 {
		t.Fatalf("stderr %q stdout %q", h.err, h.out)
	}
	if exists(filepath.Join(root, ".agk")) || len(h.ctl.Calls) != 0 {
		t.Fatalf("no run dir and no companion expected: %v", h.ctl.Calls)
	}
}

func TestApplyStartNamesRoundOneAfterEarlierRuns(t *testing.T) {
	h := newHarness(t, true)
	root := repoWith(t, h, "")
	h.ctl.WaitStatus = "active"
	for _, d := range []string{"applies/20261101-090000", "applies/20261102-090000", "reviews/20261103-090000"} {
		if err := os.MkdirAll(filepath.Join(root, ".agk", d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if code := runApply(h, "start", "add-auth"); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	if name := h.ctl.Calls[0].Args[1]; name != "add-auth · claude-apply r1" {
		t.Fatalf("name = %q", name)
	}
}

// --- worktree mode ---

// wtHarness is a harness whose repo turns worktree mode on, with the fake
// worktree sitting in the main checkout and every apply-state read recorded in
// the fake's call list.
func wtHarness(t *testing.T, inAgterm bool) (*tenv, string) {
	t.Helper()
	h := newHarness(t, inAgterm)
	root := repoWith(t, h, "[applier]\nworktree = true\n")
	h.wt.loc = worktree.Location{Main: root, Root: root}
	h.env.ApplyState = func(_ context.Context, r, _ string) (string, string, error) {
		h.wt.calls = append(h.wt.calls, wtCall{"ApplyState", []string{r}})
		return "ready", "", nil
	}
	if inAgterm {
		h.ctl.WaitStatus = "active"
	}
	return h, root
}

// only keeps the named operations of the fake's call list, in order.
func (f *fakeWorktree) only(ops ...string) []string {
	var out []string
	for _, c := range f.calls {
		for _, o := range ops {
			if c.Op == o {
				out = append(out, c.Op)
			}
		}
	}
	return out
}

func noRunDir(t *testing.T, root string) {
	t.Helper()
	for _, d := range []string{root, filepath.Join(root, ".worktrees", "add-auth")} {
		if exists(filepath.Join(d, ".agk")) {
			t.Fatalf("a run directory was created under %s", d)
		}
	}
}

func TestApplyStartBothWorktreeFlags(t *testing.T) {
	h, root := wtHarness(t, true)
	if code := runApply(h, "start", "add-auth", "--worktree", "--no-worktree"); code != 1 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(h.err.String(), "--worktree") || !strings.Contains(h.err.String(), "--no-worktree") {
		t.Fatalf("stderr = %q", h.err)
	}
	if len(h.wt.calls) != 0 || len(h.ctl.Calls) != 0 || h.out.Len() != 0 {
		t.Fatalf("worktree calls %v, ctl calls %v, stdout %q", h.wt.calls, h.ctl.Calls, h.out)
	}
	noRunDir(t, root)
}

func TestApplyStartModeOffTouchesNoWorktree(t *testing.T) {
	h := newHarness(t, true)
	root := repoWith(t, h, "")
	h.ctl.WaitStatus = "active"
	if code := runApply(h, "start", "add-auth"); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	if len(h.wt.calls) != 0 {
		t.Fatalf("worktree calls with the mode off: %v", h.wt.calls)
	}
	if !strings.Contains(h.out.String(), `"worktree":null,"pulled":false}`) {
		t.Fatalf("stdout = %q", h.out)
	}
	if !exists(applyDir(root)) {
		t.Fatal("run dir not under the repo root")
	}
}

func TestApplyStartNoWorktreeFlagTurnsTheModeOff(t *testing.T) {
	h, root := wtHarness(t, true)
	if code := runApply(h, "start", "add-auth", "--no-worktree"); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	if got := h.wt.only("Locate", "Lookup", "PullFFOnly", "CommitPaths", "Add"); len(got) != 0 {
		t.Fatalf("worktree calls with --no-worktree: %v", h.wt.calls)
	}
	if !exists(applyDir(root)) || !strings.Contains(h.out.String(), `"worktree":null`) {
		t.Fatalf("stdout = %q", h.out)
	}
}

func TestApplyStartWorktreeFlagTurnsTheModeOn(t *testing.T) {
	h := newHarness(t, true)
	root := repoWith(t, h, "")
	h.wt.loc = worktree.Location{Main: root, Root: root}
	h.ctl.WaitStatus = "active"
	if code := runApply(h, "start", "--worktree", "add-auth"); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	if !h.wt.called("Add") {
		t.Fatalf("--worktree created no worktree: %v", h.wt.calls)
	}
}

func TestApplyStartInsideLinkedWorktree(t *testing.T) {
	for name, tc := range map[string]struct {
		dir      string // the linked worktree, relative to the main checkout
		worktree bool   // stdout carries it as "worktree"
	}{
		"the change's worktree": {".worktrees/add-auth", true},
		"another worktree":      {"elsewhere", false},
	} {
		t.Run(name, func(t *testing.T) {
			h, root := wtHarness(t, true)
			linked := filepath.Join(root, tc.dir)
			h.wt.loc = worktree.Location{Main: root, Root: linked}
			if code := runApply(h, "start", "add-auth"); code != 0 {
				t.Fatalf("exit %d: %s", code, h.err)
			}
			if got := h.wt.only("PullFFOnly", "CommitPaths", "Add", "Upstream", "BranchExists"); len(got) != 0 {
				t.Fatalf("git steps inside a linked worktree: %v", h.wt.calls)
			}
			if !exists(filepath.Join(linked, ".agk", "applies", "20261105-100000")) || exists(filepath.Join(root, ".agk")) {
				t.Fatal("run dir not under the linked worktree")
			}
			if h.ctl.Calls[0].Args[3] != linked {
				t.Fatalf("companion cwd = %q, want %q", h.ctl.Calls[0].Args[3], linked)
			}
			if got := h.wt.only("ApplyState"); len(got) != 1 || h.wt.calls[len(h.wt.calls)-1].Args[0] != linked {
				t.Fatalf("apply state read: %v", h.wt.calls)
			}
			want := `"worktree":null,`
			if tc.worktree {
				want = `"worktree":"` + linked + `",`
			}
			if !strings.Contains(h.out.String(), want) || !strings.Contains(h.out.String(), `"pulled":false`) {
				t.Fatalf("stdout = %q, want %q", h.out, want)
			}
		})
	}
}

func TestApplyStartNewWorktree(t *testing.T) {
	h, root := wtHarness(t, true)
	if code := runApply(h, "start", "add-auth"); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	wt := filepath.Join(root, ".worktrees", "add-auth")
	if got, want := h.wt.only("Lookup", "ApplyState", "PullFFOnly", "CommitPaths", "Add"),
		[]string{"Lookup", "ApplyState", "PullFFOnly", "ApplyState", "CommitPaths", "Add"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("call order = %v, want %v", got, want)
	}
	for _, c := range h.wt.calls {
		switch c.Op {
		case "ApplyState":
			if c.Args[0] != root {
				t.Fatalf("apply state read in %q, want the main checkout", c.Args[0])
			}
		case "PullFFOnly":
			if c.Args[0] != root {
				t.Fatalf("pull in %q", c.Args[0])
			}
		case "CommitPaths":
			if want := []string{root, "docs(openspec): plan add-auth", "openspec/changes/add-auth"}; !reflect.DeepEqual(c.Args, want) {
				t.Fatalf("CommitPaths(%v), want %v", c.Args, want)
			}
		case "Add":
			if want := []string{root, "add-auth", "main"}; !reflect.DeepEqual(c.Args, want) {
				t.Fatalf("Add(%v), want %v", c.Args, want)
			}
		}
	}
	dir := filepath.Join(wt, ".agk", "applies", "20261105-100000")
	if !exists(dir) || exists(filepath.Join(root, ".agk")) {
		t.Fatalf("run dir not under the worktree %s", wt)
	}
	if cwd := h.ctl.Calls[0].Args[3]; cwd != wt {
		t.Fatalf("companion cwd = %q, want %q", cwd, wt)
	}
	if want := `{"companion":"COMP-1","dir":"` + dir + `","started":true,"worktree":"` + wt + `","pulled":true}` + "\n"; h.out.String() != want {
		t.Fatalf("stdout = %q, want %q", h.out, want)
	}
}

func TestApplyStartNewWorktreeInline(t *testing.T) {
	h, root := wtHarness(t, false)
	claude := h.drivers["claude"]
	claude.onHeadless = func(tr harness.Turn) {
		os.WriteFile(filepath.Join(tr.Dir, apply.ReportFile), []byte(doneReport), 0o644)
	}
	if code := runApply(h, "start", "add-auth"); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	wt := filepath.Join(root, ".worktrees", "add-auth")
	if len(claude.headless) != 1 || claude.headless[0].Root != wt {
		t.Fatalf("headless = %+v, want root %s", claude.headless, wt)
	}
	if !strings.Contains(h.out.String(), `"companion":null,`) || !strings.Contains(h.out.String(), `"worktree":"`+wt+`","pulled":true`) {
		t.Fatalf("stdout = %q", h.out)
	}
	if len(h.ctl.Calls) != 0 {
		t.Fatalf("agterm calls outside agterm: %v", h.ctl.Calls)
	}
}

func TestApplyStartSecondStateCheckRefuses(t *testing.T) {
	h, root := wtHarness(t, true)
	n := 0
	h.env.ApplyState = func(_ context.Context, r, _ string) (string, string, error) {
		h.wt.calls = append(h.wt.calls, wtCall{"ApplyState", []string{r}})
		if n++; n == 2 {
			return "all_done", "", nil
		}
		return "ready", "", nil
	}
	if code := runApply(h, "start", "add-auth"); code != 1 || !strings.Contains(h.err.String(), "nothing to apply") {
		t.Fatalf("exit %d, %q", code, h.err)
	}
	if got := h.wt.only("PullFFOnly", "ApplyState", "CommitPaths", "Add"); !reflect.DeepEqual(got, []string{"ApplyState", "PullFFOnly", "ApplyState"}) {
		t.Fatalf("calls = %v", h.wt.calls)
	}
	noRunDir(t, root)
	if len(h.ctl.Calls) != 0 {
		t.Fatalf("companion started: %v", h.ctl.Calls)
	}
}

func TestApplyStartSkipsThePull(t *testing.T) {
	for name, tc := range map[string]struct {
		args []string
		prep func(h *tenv)
	}{
		"--no-pull":   {[]string{"--no-pull"}, nil},
		"no upstream": {nil, func(h *tenv) { h.wt.hasUpstream = false }},
	} {
		t.Run(name, func(t *testing.T) {
			h, _ := wtHarness(t, true)
			if tc.prep != nil {
				tc.prep(h)
			}
			if code := runApply(h, append([]string{"start", "add-auth"}, tc.args...)...); code != 0 {
				t.Fatalf("exit %d: %s", code, h.err)
			}
			if got := h.wt.only("PullFFOnly", "ApplyState", "CommitPaths", "Add"); !reflect.DeepEqual(got, []string{"ApplyState", "CommitPaths", "Add"}) {
				t.Fatalf("calls = %v", h.wt.calls)
			}
			if !strings.Contains(h.out.String(), `"pulled":false`) {
				t.Fatalf("stdout = %q", h.out)
			}
		})
	}
}

func TestApplyStartWorktreeRefusals(t *testing.T) {
	cases := map[string]struct {
		prep func(h *tenv, root string)
		want string
	}{
		"detached HEAD":           {func(h *tenv, root string) { h.wt.errs["CurrentBranch"] = worktree.ErrDetached }, "base branch"},
		"branch without worktree": {func(h *tenv, root string) { h.wt.branchOK = true }, "branch add-auth"},
		"plain directory": {func(h *tenv, root string) {
			h.wt.entry = worktree.Entry{Kind: worktree.PlainDir}
		}, filepath.Join(".worktrees", "add-auth")},
		"worktree of another branch": {func(h *tenv, root string) {
			h.wt.entry = worktree.Entry{Kind: worktree.OtherBranch, Branch: "other"}
		}, "other"},
		"change not ready": {func(h *tenv, root string) {
			h.env.ApplyState = func(context.Context, string, string) (string, string, error) {
				return "blocked", "missing artifacts: specs", nil
			}
		}, "missing artifacts: specs"},
		"pull fails": {func(h *tenv, root string) {
			h.wt.errs["PullFFOnly"] = errors.New("fatal: Not possible to fast-forward, aborting.")
		}, "Not possible to fast-forward"},
		"commit fails": {func(h *tenv, root string) {
			h.wt.errs["CommitPaths"] = errors.New("pre-commit hook: lint failed")
		}, "lint failed"},
		"worktree add fails": {func(h *tenv, root string) {
			h.wt.errs["Add"] = errors.New("fatal: invalid reference: main")
		}, "invalid reference"},
		"locate fails": {func(h *tenv, root string) { h.wt.errs["Locate"] = errors.New("git exploded") }, "git exploded"},
	}
	// what must not have happened by the time each case stops
	forbidden := map[string][]string{
		"detached HEAD":              {"PullFFOnly", "CommitPaths", "Add"},
		"branch without worktree":    {"PullFFOnly", "CommitPaths", "Add"},
		"plain directory":            {"PullFFOnly", "CommitPaths", "Add"},
		"worktree of another branch": {"PullFFOnly", "CommitPaths", "Add"},
		"change not ready":           {"PullFFOnly", "CommitPaths", "Add"},
		"pull fails":                 {"CommitPaths", "Add"},
		"commit fails":               {"Add"},
		"worktree add fails":         {},
		"locate fails":               {"PullFFOnly", "CommitPaths", "Add"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			h, root := wtHarness(t, true)
			c.prep(h, root)
			if code := runApply(h, "start", "add-auth"); code != 1 {
				t.Fatalf("exit %d", code)
			}
			if !strings.Contains(h.err.String(), c.want) || h.out.Len() != 0 {
				t.Fatalf("stderr %q stdout %q, want it to mention %q", h.err, h.out, c.want)
			}
			if got := h.wt.only(forbidden[name]...); len(got) != 0 {
				t.Fatalf("%v ran before the refusal: %v", got, h.wt.calls)
			}
			noRunDir(t, root)
			if len(h.ctl.Calls) != 0 {
				t.Fatalf("companion started: %v", h.ctl.Calls)
			}
		})
	}
}

func TestApplyStartReusesTheChangeWorktree(t *testing.T) {
	h, root := wtHarness(t, true)
	wt := filepath.Join(root, ".worktrees", "add-auth")
	h.wt.entry = worktree.Entry{Path: wt, Kind: worktree.IsWorktree, Branch: "add-auth"}
	if code := runApply(h, "start", "add-auth"); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	if got := h.wt.only("PullFFOnly", "CommitPaths", "Add", "Upstream"); len(got) != 0 {
		t.Fatalf("git steps on reuse: %v", h.wt.calls)
	}
	var states []string
	for _, c := range h.wt.calls {
		if c.Op == "ApplyState" {
			states = append(states, c.Args[0])
		}
	}
	if !reflect.DeepEqual(states, []string{wt}) {
		t.Fatalf("apply state read in %v, want the worktree", states)
	}
	dir := filepath.Join(wt, ".agk", "applies", "20261105-100000")
	if !exists(dir) || exists(filepath.Join(root, ".agk")) {
		t.Fatal("new run dir not under the existing worktree")
	}
	if want := `{"companion":"COMP-1","dir":"` + dir + `","started":true,"worktree":"` + wt + `","pulled":false}` + "\n"; h.out.String() != want {
		t.Fatalf("stdout = %q", h.out)
	}
}

// withHarness switches the worker.json of apply run dir to harnessName, the
// way a run started with that applier looks.
func withHarness(t *testing.T, dir, harnessName string) {
	t.Helper()
	var st applyState
	if have, err := rundir.ReadJSON(dir, workerFile, &st); err != nil || !have {
		t.Fatalf("read worker.json: %v %v", have, err)
	}
	st.Worker.Harness = harnessName
	if err := rundir.WriteJSON(dir, workerFile, st); err != nil {
		t.Fatal(err)
	}
}

func applyMsg(t *testing.T) string {
	t.Helper()
	msg := filepath.Join(t.TempDir(), "m.md")
	if err := os.WriteFile(msg, []byte("m"), 0o644); err != nil {
		t.Fatal(err)
	}
	return msg
}

func TestApplySendCodexStatusStuckActiveSettledScreen(t *testing.T) {
	h := newHarness(t, true)
	dir := startedApply(t, h, 1)
	withHarness(t, dir, config.Codex)
	writeReport(t, dir, 1, doneReport)
	stuckSlices(h)
	h.ctl.TextOut = "idle composer\n"
	h.ctl.WaitSeq = []agtermtest.WaitResult{
		{Err: agterm.ErrTimeout}, {Err: agterm.ErrTimeout}, // two slices
		{Status: "active"}, // status check
		{Status: "active"}, // the line was picked up
	}
	if code := runApply(h, "send", dir, applyMsg(t), "--timeout", "30"); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	prompt := filepath.Join(dir, "rounds", "2", "prompt.md")
	if got := types(h); !reflect.DeepEqual(got, []string{"Read " + prompt + " and follow it.\r"}) {
		t.Fatalf("typed %q", got)
	}
	for _, want := range []string{"the applier C-1 still reports active", "round 1 is on disk", "treating its turn as over"} {
		if !strings.Contains(h.err.String(), want) {
			t.Errorf("stderr lacks %q: %q", want, h.err)
		}
	}
	if h.out.String() != `{"round":2,"started":true}`+"\n" {
		t.Fatalf("stdout = %q", h.out)
	}
}

func TestApplySendCodexKeepsBusy(t *testing.T) {
	cases := map[string]struct {
		report bool
		screen func(h *tenv)
	}{
		"no report yet":         {false, func(h *tenv) { h.ctl.TextOut = "idle composer\n" }},
		"screen still changing": {true, runningScreen},
		"codex dialog":          {true, func(h *tenv) { h.ctl.TextOut = "Allow command? (y/n)\n" }},
		"screen cannot be read": {true, func(h *tenv) { h.ctl.TextErr = errors.New("boom") }},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t, true)
			dir := startedApply(t, h, 1)
			withHarness(t, dir, config.Codex)
			if c.report {
				writeReport(t, dir, 1, doneReport)
			}
			stuckSlices(h)
			c.screen(h)
			code := runApply(h, "send", dir, applyMsg(t), "--timeout", "30")
			if code != 1 || !strings.Contains(h.err.String(), "applier C-1 is busy") {
				t.Fatalf("exit %d: %s", code, h.err)
			}
			if len(types(h)) != 0 {
				t.Fatalf("typed %q", types(h))
			}
			if !c.report && textReads(h) != 0 {
				t.Fatalf("%d screen reads without a report", textReads(h))
			}
			if k, _ := rundir.Round(dir); k != 1 || exists(filepath.Join(dir, "rounds", "2")) {
				t.Fatalf("no round may be recorded: %d", k)
			}
		})
	}
}

func TestApplySendClaudeKeepsStatusRule(t *testing.T) {
	h := newHarness(t, true)
	dir := startedApply(t, h, 1)
	writeReport(t, dir, 1, doneReport)
	stuckSlices(h)
	h.ctl.TextOut = "idle composer\n"
	code := runApply(h, "send", dir, applyMsg(t), "--timeout", "30")
	if code != 1 || !strings.Contains(h.err.String(), "applier C-1 is busy") {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	if len(types(h)) != 0 || textReads(h) != 0 {
		t.Fatalf("typed %q, %d screen reads: a claude applier keeps the status rule", types(h), textReads(h))
	}
}
