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
	"github.com/shurastik/agterm-kit/internal/apply"
	"github.com/shurastik/agterm-kit/internal/config"
	"github.com/shurastik/agterm-kit/internal/harness"
	"github.com/shurastik/agterm-kit/internal/rundir"
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
	if b, _ := os.ReadFile(filepath.Join(dir, settingsFile)); string(b) != string(apply.ClaudeSettings()) {
		t.Fatal("settings file missing")
	}
	kickoff := "Read " + prompt + " and follow it."
	wantCmd := "'claude' '--model=' '--session=SESS-NEW' '" + kickoff + "'"
	assertCalls(t, h.ctl,
		agtermtest.Call{Method: "NewCompanion", Args: []string{"SELF", "claude-apply #1", wantCmd, root}},
		agtermtest.Call{Method: "Wait", Args: []string{"COMP-1", "active,completed,blocked", "1m0s"}},
	)
	if want := `{"companion":"COMP-1","dir":"` + dir + `","started":true}` + "\n"; h.out.String() != want {
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
	if name := h.ctl.Calls[0].Args[1]; name != "codex-apply #1" {
		t.Fatalf("name = %q", name)
	}
	if s := readApplyState(t, dir); s.Session != "" || s.Worker.Model != "gpt-5.5" || s.Worker.PermissionMode != "" {
		t.Fatalf("worker.json = %+v", s)
	}
	p, _ := os.ReadFile(filepath.Join(dir, "rounds", "1", "prompt.md"))
	if !strings.HasPrefix(string(p), apply.Rules()) {
		t.Fatal("codex gets the rules at the top of the prompt")
	}
	if cmd := h.ctl.Calls[0].Args[2]; !strings.HasPrefix(cmd, "'codex' '--model=gpt-5.5' '--session=' ") {
		t.Fatalf("cmd = %q", cmd)
	}
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
		"blocked":        {"add-auth", func(context.Context, string, string) (string, string, error) { return "blocked", "missing artifacts: specs", nil }, "missing artifacts: specs"},
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
	if w := `{"companion":null,"dir":"` + dir + `","started":true}` + "\n"; h.out.String() != w {
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
		agtermtest.Call{Method: "Wait", Args: []string{"C-1", "active", "30s"}},
	)
	if h.out.String() != `{"round":2}`+"\n" {
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
	if !strings.Contains(h.err.String(), "warning") {
		t.Fatalf("stderr = %q", h.err)
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
