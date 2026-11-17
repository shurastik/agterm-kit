package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/shurastik/agterm-kit/assets"
	"github.com/shurastik/agterm-kit/internal/agterm"
	"github.com/shurastik/agterm-kit/internal/agterm/agtermtest"
	"github.com/shurastik/agterm-kit/internal/config"
	"github.com/shurastik/agterm-kit/internal/review"
	"github.com/shurastik/agterm-kit/internal/rundir"
)

func scriptScope(h *tenv, root string) *[]string {
	var seen []string
	h.env.Collect = func(_ context.Context, dir, base string) (review.Scope, error) {
		seen = append(seen, dir, base)
		return review.Scope{Root: root, Base: base, MergeBase: "mb", Diff: "+x\n", Stat: " f | 1 +\n", Fingerprint: scopeFingerprint}, nil
	}
	return &seen
}

// scopeFingerprint is the fingerprint scriptScope's faked scope carries.
const scopeFingerprint = "fp-0123456789abcdef"

// scopeFile is the run-dir file holding a diff round's scope fingerprint.
func scopeFilePath(dir string, k int) string {
	return filepath.Join(dir, "rounds", fmt.Sprint(k), "scope.sha256")
}

// mergeBaseFilePath is the run-dir file holding a diff round's merge base.
func mergeBaseFilePath(dir string, k int) string {
	return filepath.Join(dir, "rounds", fmt.Sprint(k), "merge-base")
}

func TestReviewStartNothingToReview(t *testing.T) {
	h := newHarness(t, true)
	root, _ := filepath.EvalSymlinks(t.TempDir())
	h.env.Collect = func(context.Context, string, string) (review.Scope, error) {
		return review.Scope{}, review.ErrNothingToReview
	}
	if code := h.env.ReviewStart(context.Background(), nil); code == 0 {
		t.Fatal("want non-zero exit")
	}
	if !strings.Contains(h.err.String(), "nothing to review") {
		t.Fatalf("stderr = %q", h.err)
	}
	if len(h.ctl.Calls) != 0 || h.out.Len() != 0 {
		t.Fatalf("no companion and no stdout expected: calls %v out %q", h.ctl.Calls, h.out)
	}
	if exists(filepath.Join(root, ".agk")) {
		t.Fatal("no run dir expected")
	}
}

func TestReviewStartSpawnsCompanion(t *testing.T) {
	h := newHarness(t, true)
	h.vars["PATH"] = "/opt/homebrew/bin:/usr/bin"
	root, _ := filepath.EvalSymlinks(t.TempDir())
	seen := scriptScope(h, root)

	if code := h.env.ReviewStart(context.Background(), []string{"--base", "develop"}); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	if !reflect.DeepEqual(*seen, []string{h.env.Cwd, "develop"}) {
		t.Fatalf("collect args = %q", *seen)
	}
	dir := filepath.Join(root, ".agk", "reviews", "20261105-100000")
	prompt, err := os.ReadFile(filepath.Join(dir, "rounds", "1", "prompt.md"))
	if err != nil || !strings.Contains(string(prompt), "+x") {
		t.Fatalf("prompt.md: %v %q", err, prompt)
	}
	// The interactive output contract: schema, this round's file and the root.
	for _, want := range []string{filepath.Join(dir, "schema.json"), filepath.Join(dir, "out", "1.json"), "git -C " + root} {
		if !strings.Contains(string(prompt), want) {
			t.Errorf("prompt lacks %q", want)
		}
	}
	// The caller's PATH is pinned so codex and agtermctl resolve inside the
	// companion even when agterm starts it without a login shell.
	first := "Read " + filepath.Join(dir, "rounds", "1", "prompt.md") + " and follow it."
	wantCmd := fmt.Sprintf("env 'PATH=/opt/homebrew/bin:/usr/bin' 'codex' 'review' '--model=' '--root=%s' '--out=%s' '--settings=' '--session=' '%s'",
		root, filepath.Join(dir, "out"), first)
	want := []agtermtest.Call{
		{Method: "NewCompanion", Args: []string{"SELF", "codex-review #1", wantCmd, root}},
		{Method: "Wait", Args: []string{"COMP-1", "active,completed,blocked", "1m0s"}},
		{Method: "Info", Args: []string{"COMP-1", "codex #1 r1: reviewing"}},
	}
	if !reflect.DeepEqual(h.ctl.Calls, want) {
		t.Fatalf("calls = %v\nwant %v", h.ctl.Calls, want)
	}
	if h.out.String() != `{"companion":"COMP-1","dir":"`+dir+`","started":true}`+"\n" {
		t.Fatalf("stdout = %q", h.out)
	}
	if len(h.drivers["codex"].turns) != 0 {
		t.Fatal("start must not run the reviewer itself inside agterm")
	}
	if s := readState(t, dir); s.Worker.Harness != "codex" || s.Base != "develop" || s.Session != "" || !s.Interactive {
		t.Fatalf("worker.json = %+v", s)
	}
	if k, err := rundir.Round(dir); err != nil || k != 1 {
		t.Fatalf("round = %d, %v", k, err)
	}
	if id, err := os.ReadFile(filepath.Join(dir, "companion")); err != nil || string(id) != "COMP-1\n" {
		t.Fatalf("companion file = %q, %v", id, err)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "schema.json")); !bytes.Equal(got, assets.ReviewSchema) {
		t.Fatal("schema.json missing or different")
	}
	if fi, err := os.Stat(filepath.Join(dir, "out")); err != nil || !fi.IsDir() {
		t.Fatalf("out/: %v", err)
	}
	if !roundStarted(dir, 1) {
		t.Fatal("a confirmed handshake marks round 1 started")
	}
	noStatus(t, h)
}

func TestReviewStartClaudeInteractive(t *testing.T) {
	h := newHarness(t, true)
	root, _ := filepath.EvalSymlinks(t.TempDir())
	scriptScope(h, root)
	if code := h.env.ReviewStart(context.Background(), []string{"--reviewer", "claude:claude-opus-5-5"}); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	dir := filepath.Join(root, ".agk", "reviews", "20261105-100000")
	first := "Read " + filepath.Join(dir, "rounds", "1", "prompt.md") + " and follow it."
	wantCmd := fmt.Sprintf("'claude' 'review' '--model=claude-opus-5-5' '--root=%s' '--out=%s' '--settings=%s' '--session=SESS-NEW' '%s'",
		root, filepath.Join(dir, "out"), filepath.Join(dir, "reviewer-settings.json"), first)
	if c := h.ctl.Calls[0]; c.Method != "NewCompanion" || !reflect.DeepEqual(c.Args, []string{"SELF", "claude-review #1", wantCmd, root}) {
		t.Fatalf("companion call = %v", c)
	}
	if s := readState(t, dir); s.Worker.Harness != "claude" || s.Worker.Model != "claude-opus-5-5" || s.Session != "SESS-NEW" || !s.Interactive {
		t.Fatalf("worker.json = %+v", s)
	}
	settings, err := os.ReadFile(filepath.Join(dir, "reviewer-settings.json"))
	if err != nil || !strings.Contains(string(settings), "'/usr/local/bin/agk' review hook completed") {
		t.Fatalf("reviewer-settings.json: %v %s", err, settings)
	}
	prompt, _ := os.ReadFile(filepath.Join(dir, "rounds", "1", "prompt.md"))
	if strings.Contains(string(prompt), "git -C") || !strings.Contains(string(prompt), filepath.Join(dir, "out", "1.json")) {
		t.Fatalf("claude prompt:\n%s", prompt)
	}
	noStatus(t, h)
}

// An unconfirmed start leaves the companion open for the user (a trust
// prompt, a login screen) and says so.
func TestReviewStartHandshakeTimeout(t *testing.T) {
	h := newHarness(t, true)
	root, _ := filepath.EvalSymlinks(t.TempDir())
	scriptScope(h, root)
	h.ctl.WaitErr = agterm.ErrTimeout
	h.ctl.TextOut = "Do you trust the files in this folder?\n"
	if code := h.env.ReviewStart(context.Background(), nil); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	dir := filepath.Join(root, ".agk", "reviews", "20261105-100000")
	if h.out.String() != `{"companion":"COMP-1","dir":"`+dir+`","started":false}`+"\n" {
		t.Fatalf("stdout = %q", h.out)
	}
	if !strings.Contains(h.err.String(), "reviewer COMP-1 did not start") || !strings.Contains(h.err.String(), "Do you trust the files") {
		t.Fatalf("stderr = %q", h.err)
	}
	if roundStarted(dir, 1) {
		t.Fatal("an unconfirmed start must not mark round 1 started")
	}
	for _, m := range h.ctl.Methods() {
		if m == "Close" {
			t.Fatal("the companion stays open")
		}
	}
}

func TestReviewStartCompanionFileFailureClosesCompanion(t *testing.T) {
	h := newHarness(t, true)
	root, _ := filepath.EvalSymlinks(t.TempDir())
	scriptScope(h, root)
	dir := filepath.Join(root, ".agk", "reviews", "20261105-100000")
	h.ctl.OnNewCompanion = func() { os.Chmod(dir, 0o500) }
	t.Cleanup(func() { os.Chmod(dir, 0o755) })
	if code := h.env.ReviewStart(context.Background(), nil); code == 0 {
		t.Fatal("want non-zero exit")
	}
	if h.out.Len() != 0 || !strings.Contains(h.err.String(), "companion") {
		t.Fatalf("stdout %q stderr %q", h.out, h.err)
	}
	if got := h.ctl.Methods(); !reflect.DeepEqual(got, []string{"NewCompanion", "Close"}) || h.ctl.Calls[1].Args[0] != "COMP-1" {
		t.Fatalf("calls = %v", h.ctl.Calls)
	}
}

func TestReviewStartDefaultBase(t *testing.T) {
	h := newHarness(t, true)
	seen := scriptScope(h, t.TempDir())
	h.env.ReviewStart(context.Background(), nil)
	if (*seen)[1] != "main" {
		t.Fatalf("base = %q", (*seen)[1])
	}
}

func TestReviewStartQuotesSingleQuote(t *testing.T) {
	h := newHarness(t, true)
	root := filepath.Join(t.TempDir(), "it's")
	os.MkdirAll(root, 0o755)
	scriptScope(h, root)
	if code := h.env.ReviewStart(context.Background(), nil); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	cmd := h.ctl.Calls[0].Args[2]
	if !strings.Contains(cmd, `it'\''s`) {
		t.Fatalf("command = %q", cmd)
	}
}

func TestReviewStartRejectsUnquotablePaths(t *testing.T) {
	for name, exe := range map[string]string{
		"backslash": `/opt/a\b/agk`,
		"newline":   "/opt/a\nb/agk",
	} {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t, true)
			scriptScope(h, t.TempDir())
			h.env.Executable = func() (string, error) { return exe, nil }
			// The claude reviewer's hooks run agk through the shell.
			if code := h.env.ReviewStart(context.Background(), []string{"--reviewer", "claude"}); code == 0 {
				t.Fatal("want non-zero exit")
			}
			if len(h.ctl.Calls) != 0 {
				t.Fatalf("no companion expected: %v", h.ctl.Calls)
			}
		})
	}
}

func TestReviewStartCompanionError(t *testing.T) {
	h := newHarness(t, true)
	scriptScope(h, t.TempDir())
	h.ctl.CompanionErr = errors.New("socket gone")
	if code := h.env.ReviewStart(context.Background(), nil); code == 0 {
		t.Fatal("want non-zero exit")
	}
	if !strings.Contains(h.err.String(), "socket gone") || h.out.Len() != 0 {
		t.Fatalf("stderr %q stdout %q", h.err, h.out)
	}
}

func TestReviewStartBadFlag(t *testing.T) {
	h := newHarness(t, true)
	if code := h.env.ReviewStart(context.Background(), []string{"--nope"}); code == 0 {
		t.Fatal("want non-zero exit")
	}
}

func TestReviewStartOutsideAgtermRunsInline(t *testing.T) {
	h := newHarness(t, false)
	root, _ := filepath.EvalSymlinks(t.TempDir())
	scriptScope(h, root)
	h.drivers["codex"].outputs = []string{threeFindings}
	if code := h.env.ReviewStart(context.Background(), nil); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	if len(h.ctl.Calls) != 0 {
		t.Fatalf("no agterm calls expected: %v", h.ctl.Calls)
	}
	dir := filepath.Join(root, ".agk", "reviews", "20261105-100000")
	if h.out.String() != `{"companion":null,"dir":"`+dir+`"}`+"\n" {
		t.Fatalf("stdout = %q", h.out)
	}
	if !strings.Contains(h.err.String(), "codex streaming") {
		t.Fatal("reviewer output goes to stderr inline so stdout stays JSON")
	}
	if !exists(filepath.Join(dir, "rounds", "1", "review.json")) {
		t.Fatal("review.json missing")
	}
	if exists(filepath.Join(dir, "companion")) {
		t.Fatal("inline run must not record a companion")
	}
	if exists(filepath.Join(dir, "out")) || exists(filepath.Join(dir, "schema.json")) {
		t.Fatal("an inline run has no output directory")
	}
	if p, _ := os.ReadFile(filepath.Join(dir, "rounds", "1", "prompt.md")); strings.Contains(string(p), "out/1.json") || !strings.Contains(string(p), "Respond with a single JSON object") {
		t.Fatalf("inline prompt keeps the schema contract and names no file:\n%s", p)
	}
	if s := readState(t, dir); s.Interactive {
		t.Fatalf("inline state = %+v", s)
	}
}

func TestReviewStartOutsideAgtermFailure(t *testing.T) {
	h := newHarness(t, false)
	scriptScope(h, t.TempDir())
	h.drivers["codex"].errs = []error{errors.New("codex failed: boom")}
	if code := h.env.ReviewStart(context.Background(), nil); code == 0 {
		t.Fatal("want non-zero exit")
	}
	if !strings.Contains(h.out.String(), `"companion":null`) {
		t.Fatalf("stdout = %q", h.out)
	}
}

func TestReviewStartRejectsBeforeCreatingRunDir(t *testing.T) {
	h := newHarness(t, true)
	root := t.TempDir()
	scriptScope(h, root)
	h.env.Executable = func() (string, error) { return `/opt/a\b/agk`, nil }
	h.env.ReviewStart(context.Background(), []string{"--reviewer", "claude"})
	if exists(filepath.Join(root, ".agk")) {
		t.Fatal("run dir must not be created when the command cannot be built")
	}
}

func TestReviewStartWithoutPathSkipsEnvPrefix(t *testing.T) {
	h := newHarness(t, true)
	scriptScope(h, t.TempDir())
	if code := h.env.ReviewStart(context.Background(), nil); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	if cmd := h.ctl.Calls[0].Args[2]; !strings.HasPrefix(cmd, "'codex' 'review' ") {
		t.Fatalf("command = %q", cmd)
	}
}

func TestReviewStartForwardsCodexHome(t *testing.T) {
	h := newHarness(t, true)
	h.vars["PATH"] = "/usr/bin"
	h.vars["CODEX_HOME"] = "/custom/codex"
	h.vars["CLAUDE_CONFIG_DIR"] = "/custom/claude"
	h.vars["AGTERMCTL"] = "/opt/agterm/agtermctl"
	h.vars["OPENAI_API_KEY"] = "sk-secret"
	scriptScope(h, t.TempDir())
	if code := h.env.ReviewStart(context.Background(), nil); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	cmd := h.ctl.Calls[0].Args[2]
	if !strings.HasPrefix(cmd, "env 'PATH=/usr/bin' 'CODEX_HOME=/custom/codex' 'CLAUDE_CONFIG_DIR=/custom/claude' 'AGTERMCTL=/opt/agterm/agtermctl' 'codex' 'review' ") {
		t.Fatalf("command = %q", cmd)
	}
	if strings.Contains(cmd, "sk-secret") {
		t.Fatal("secrets must never be put on the companion command line")
	}
}

func TestReviewStartSameWorkerWarns(t *testing.T) {
	h := newHarness(t, true)
	scriptScope(h, t.TempDir())
	if code := h.env.ReviewStart(context.Background(), []string{"--reviewer", "claude"}); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	if !strings.Contains(h.err.String(), "not independent") {
		t.Fatalf("stderr = %q", h.err)
	}
}

func TestReviewStartInvalidConfig(t *testing.T) {
	h := newHarness(t, true)
	root, _ := filepath.EvalSymlinks(t.TempDir())
	scriptScope(h, root)
	os.WriteFile(filepath.Join(root, ".agk.toml"), []byte("[reviewer]\nmodle = \"x\"\n"), 0o644)
	if code := h.env.ReviewStart(context.Background(), nil); code != 1 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(h.err.String(), "reviewer.modle") || exists(filepath.Join(root, ".agk")) || len(h.ctl.Calls) != 0 {
		t.Fatalf("stderr %q, calls %v", h.err, h.ctl.Calls)
	}
}

func TestReviewStartMalformedReviewerFlag(t *testing.T) {
	h := newHarness(t, true)
	root, _ := filepath.EvalSymlinks(t.TempDir())
	scriptScope(h, root)
	if code := h.env.ReviewStart(context.Background(), []string{"--reviewer", "gemini"}); code != 1 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(h.err.String(), "--reviewer") || h.out.Len() != 0 {
		t.Fatalf("stderr %q stdout %q", h.err, h.out)
	}
	if exists(filepath.Join(root, ".agk")) || len(h.ctl.Calls) != 0 {
		t.Fatalf("no run dir and no companion expected: %v", h.ctl.Calls)
	}
}

func samplePlanScope(root string) review.PlanScope {
	return review.PlanScope{Root: root, Change: "add-x", Files: []review.UntrackedFile{
		{Path: "openspec/changes/add-x/proposal.md", Content: "PROPOSAL_BODY\n"},
	}}
}

// scriptPlan makes CollectPlan return the plan scope of add-x under root and
// records its arguments; Collect must not be called.
func scriptPlan(h *tenv, root string) *[]string {
	var seen []string
	h.env.CollectPlan = func(_ context.Context, dir, change string) (review.PlanScope, error) {
		seen = append(seen, dir, change)
		s := samplePlanScope(root)
		s.Change = change
		return s, nil
	}
	h.env.Collect = func(context.Context, string, string) (review.Scope, error) {
		return review.Scope{}, errors.New("diff scope must not be collected for a plan review")
	}
	return &seen
}

func TestReviewStartChangeSpawnsCompanion(t *testing.T) {
	h := newHarness(t, true)
	root, _ := filepath.EvalSymlinks(t.TempDir())
	seen := scriptPlan(h, root)
	if code := h.env.ReviewStart(context.Background(), []string{"--change", "add-x"}); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	if !reflect.DeepEqual(*seen, []string{h.env.Cwd, "add-x"}) {
		t.Fatalf("collect plan args = %q", *seen)
	}
	dir := filepath.Join(root, ".agk", "reviews", "20261105-100000")
	prompt, err := os.ReadFile(filepath.Join(dir, "rounds", "1", "prompt.md"))
	want, _ := review.PlanPrompt(samplePlanScope(root), review.Output{Schema: filepath.Join(dir, "schema.json"),
		File: filepath.Join(dir, "out", "1.json"), Root: root, GitC: true})
	if err != nil || string(prompt) != want {
		t.Fatalf("prompt.md is not the interactive plan prompt: %v\n%s", err, prompt)
	}
	if s := readState(t, dir); s.Change != "add-x" || s.Base != "" || s.Worker.Harness != "codex" {
		t.Fatalf("worker.json = %+v", s)
	}
	if h.ctl.Calls[0].Method != "NewCompanion" || h.ctl.Calls[0].Args[1] != "add-x · codex-plan #1" {
		t.Fatalf("calls = %v", h.ctl.Calls)
	}
	if !strings.Contains(h.out.String(), `"companion":"COMP-1"`) || !strings.Contains(h.out.String(), `"started":true`) {
		t.Fatalf("stdout = %q", h.out)
	}
}

func TestReviewStartChangeInline(t *testing.T) {
	h := newHarness(t, false)
	root, _ := filepath.EvalSymlinks(t.TempDir())
	scriptPlan(h, root)
	h.drivers["codex"].outputs = []string{threeFindings}
	if code := h.env.ReviewStart(context.Background(), []string{"--change", "add-x"}); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	dir := filepath.Join(root, ".agk", "reviews", "20261105-100000")
	if h.out.String() != `{"companion":null,"dir":"`+dir+`"}`+"\n" {
		t.Fatalf("stdout = %q", h.out)
	}
	if tr := h.drivers["codex"].turns; len(tr) != 1 || !strings.Contains(tr[0].Prompt, "PROPOSAL_BODY") {
		t.Fatalf("turns = %+v", tr)
	}
	if len(h.ctl.Calls) != 0 {
		t.Fatalf("no agterm calls expected: %v", h.ctl.Calls)
	}
}

func TestReviewStartChangeRefusals(t *testing.T) {
	cases := map[string]struct {
		args    []string
		collect func(context.Context, string, string) (review.PlanScope, error)
		want    string
		read    bool // whether CollectPlan may be called
	}{
		"path-like name":          {args: []string{"--change", "../secrets"}, want: "invalid change name"},
		"upper case name":         {args: []string{"--change", "Add_X"}, want: "invalid change name"},
		"with base":               {args: []string{"--change", "add-x", "--base", "develop"}, want: "exclude each other"},
		"with default base given": {args: []string{"--base", "main", "--change", "add-x"}, want: "exclude each other"},
		"missing change": {args: []string{"--change", "add-y"}, read: true, want: `change "add-y" not found`,
			collect: func(context.Context, string, string) (review.PlanScope, error) {
				return review.PlanScope{}, errors.New(`change "add-y" not found: no openspec/changes/add-y`)
			}},
		"empty change": {args: []string{"--change", "add-x"}, read: true, want: "nothing to review",
			collect: func(context.Context, string, string) (review.PlanScope, error) {
				return review.PlanScope{}, review.ErrNothingToReview
			}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t, true)
			root, _ := filepath.EvalSymlinks(t.TempDir())
			h.env.Cwd = root
			called := false
			h.env.CollectPlan = func(ctx context.Context, dir, change string) (review.PlanScope, error) {
				called = true
				if c.collect == nil {
					return samplePlanScope(root), nil
				}
				return c.collect(ctx, dir, change)
			}
			if code := h.env.ReviewStart(context.Background(), c.args); code != 1 {
				t.Fatalf("exit %d, want 1 (stderr %q)", code, h.err)
			}
			if !strings.Contains(h.err.String(), c.want) || h.out.Len() != 0 {
				t.Fatalf("stderr %q stdout %q", h.err, h.out)
			}
			if called != c.read {
				t.Fatalf("CollectPlan called = %v, want %v", called, c.read)
			}
			if exists(filepath.Join(root, ".agk")) || len(h.ctl.Calls) != 0 {
				t.Fatalf("no run dir and no companion expected: %v", h.ctl.Calls)
			}
		})
	}
}

// An empty --change (e.g. from an empty shell variable) must not fall back
// to a diff review.
func TestReviewStartEmptyChangeRefused(t *testing.T) {
	for name, args := range map[string][]string{
		"alone":     {"--change="},
		"separate":  {"--change", ""},
		"with base": {"--change=", "--base", "main"},
	} {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t, true)
			root, _ := filepath.EvalSymlinks(t.TempDir())
			h.env.Cwd = root
			collected := false
			h.env.Collect = func(context.Context, string, string) (review.Scope, error) {
				collected = true
				return review.Scope{Root: root, Base: "main", MergeBase: "mb", Diff: "+x\n"}, nil
			}
			h.env.CollectPlan = func(context.Context, string, string) (review.PlanScope, error) {
				collected = true
				return samplePlanScope(root), nil
			}
			if code := h.env.ReviewStart(context.Background(), args); code != 1 {
				t.Fatalf("exit %d, want 1 (stderr %q)", code, h.err)
			}
			if collected || h.out.Len() != 0 || exists(filepath.Join(root, ".agk")) || len(h.ctl.Calls) != 0 {
				t.Fatalf("nothing may be collected or started: stderr %q calls %v", h.err, h.ctl.Calls)
			}
		})
	}
}

// Each run records its target: a diff review its base and no change, a plan
// review its change and no base.
func TestReviewStartRecordsTargetOfEachKind(t *testing.T) {
	h := newHarness(t, true)
	root, _ := filepath.EvalSymlinks(t.TempDir())
	scriptScope(h, root)
	if code := h.env.ReviewStart(context.Background(), []string{"--base", "main"}); code != 0 {
		t.Fatalf("diff: exit %d: %s", code, h.err)
	}
	if s := readState(t, filepath.Join(root, ".agk", "reviews", "20261105-100000")); s.Base != "main" || s.Change != "" {
		t.Fatalf("diff run state = %+v", s)
	}

	h = newHarness(t, true)
	root2, _ := filepath.EvalSymlinks(t.TempDir())
	scriptPlan(h, root2)
	if code := h.env.ReviewStart(context.Background(), []string{"--change", "add-x"}); code != 0 {
		t.Fatalf("plan: exit %d: %s", code, h.err)
	}
	if s := readState(t, filepath.Join(root2, ".agk", "reviews", "20261105-100000")); s.Change != "add-x" || s.Base != "" {
		t.Fatalf("plan run state = %+v", s)
	}
}

// A worker.json written before the reviewer became interactive loads as
// non-interactive.
func TestReviewStateInteractiveDefaultsFalse(t *testing.T) {
	_, dir := runDir(t)
	if s := readState(t, dir); s.Interactive {
		t.Fatalf("state = %+v", s)
	}
	if err := rundir.WriteJSON(dir, workerFile, reviewState{Worker: config.Worker{Harness: config.Codex}, Interactive: true}); err != nil {
		t.Fatal(err)
	}
	if s := readState(t, dir); !s.Interactive {
		t.Fatalf("state = %+v", s)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, workerFile))
	if !strings.Contains(string(raw), `"interactive": true`) {
		t.Fatalf("worker.json = %s", raw)
	}
	if err := rundir.WriteJSON(dir, workerFile, reviewState{Worker: config.Worker{Harness: config.Codex}}); err != nil {
		t.Fatal(err)
	}
	raw, _ = os.ReadFile(filepath.Join(dir, workerFile))
	if strings.Contains(string(raw), "interactive") {
		t.Fatalf("interactive must be omitted when false: %s", raw)
	}
}

func TestPrepareOutWritesSchema(t *testing.T) {
	_, dir := runDir(t)
	out, schema, err := prepareOut(dir)
	if err != nil {
		t.Fatal(err)
	}
	if out != filepath.Join(dir, "out") || schema != filepath.Join(dir, "schema.json") {
		t.Fatalf("out %q schema %q", out, schema)
	}
	if fi, err := os.Stat(out); err != nil || !fi.IsDir() {
		t.Fatalf("out dir: %v", err)
	}
	got, err := os.ReadFile(schema)
	if err != nil || !bytes.Equal(got, assets.ReviewSchema) {
		t.Fatalf("schema.json differs from assets.ReviewSchema: %v", err)
	}
}

func writeReviewerConfig(t *testing.T, root, harnessName string, extra ...string) {
	t.Helper()
	q := make([]string, len(extra))
	for i, a := range extra {
		q[i] = fmt.Sprintf("%q", a)
	}
	toml := fmt.Sprintf("[reviewer]\nharness = %q\nextra_args = [%s]\n", harnessName, strings.Join(q, ", "))
	if err := os.WriteFile(filepath.Join(root, ".agk.toml"), []byte(toml), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Reviewer extra_args that would widen its permissions are refused before a
// run dir exists, inline and in agterm alike.
func TestReviewStartRefusesWideningExtraArgs(t *testing.T) {
	cases := []struct {
		harness string
		args    []string
		named   string
	}{
		{"codex", []string{"--sandbox", "danger-full-access"}, "--sandbox"},
		{"codex", []string{"--sandbox=danger-full-access"}, "--sandbox=danger-full-access"},
		{"codex", []string{"-a", "never"}, "-a"},
		{"codex", []string{"--yolo"}, "--yolo"},
		{"codex", []string{"--add-dir", "/"}, "--add-dir"},
		{"codex", []string{"-c", `sandbox_mode="danger-full-access"`}, "sandbox_mode"},
		{"codex", []string{"--config", `approval_policy="never"`}, "approval_policy"},
		{"claude", []string{"--dangerously-skip-permissions"}, "--dangerously-skip-permissions"},
		{"claude", []string{"--permission-mode=bypassPermissions"}, "--permission-mode=bypassPermissions"},
		{"claude", []string{"--allowedTools", "Bash"}, "--allowedTools"},
		{"claude", []string{"--settings", "/x.json"}, "--settings"},
	}
	for _, inAgterm := range []bool{true, false} {
		for _, c := range cases {
			name := fmt.Sprintf("agterm=%v %s %s", inAgterm, c.harness, strings.Join(c.args, " "))
			t.Run(name, func(t *testing.T) {
				h := newHarness(t, inAgterm)
				root, _ := filepath.EvalSymlinks(t.TempDir())
				scriptScope(h, root)
				writeReviewerConfig(t, root, c.harness, c.args...)
				if code := h.env.ReviewStart(context.Background(), nil); code != 1 {
					t.Fatalf("exit %d, want 1 (stderr %q)", code, h.err)
				}
				if !strings.Contains(h.err.String(), c.named) || h.out.Len() != 0 {
					t.Fatalf("stderr %q stdout %q", h.err, h.out)
				}
				if exists(filepath.Join(root, ".agk")) || len(h.ctl.Calls) != 0 {
					t.Fatalf("no run dir and no companion expected: %v", h.ctl.Calls)
				}
				if len(h.drivers[c.harness].turns) != 0 {
					t.Fatal("the reviewer must not run")
				}
			})
		}
	}
}

func TestReviewStartHarmlessExtraArgsPass(t *testing.T) {
	h := newHarness(t, true)
	root, _ := filepath.EvalSymlinks(t.TempDir())
	scriptScope(h, root)
	writeReviewerConfig(t, root, "codex", "-c", `model_verbosity="low"`)
	if code := h.env.ReviewStart(context.Background(), nil); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	if s := readState(t, filepath.Join(root, ".agk", "reviews", "20261105-100000")); !reflect.DeepEqual(s.Worker.ExtraArgs, []string{"-c", `model_verbosity="low"`}) {
		t.Fatalf("state = %+v", s)
	}
}

// The applier's extra_args are the applier's business.
func TestReviewStartIgnoresApplierExtraArgs(t *testing.T) {
	h := newHarness(t, true)
	root, _ := filepath.EvalSymlinks(t.TempDir())
	scriptScope(h, root)
	toml := "[applier]\nharness = \"codex\"\nextra_args = [\"--sandbox\", \"danger-full-access\"]\n"
	os.WriteFile(filepath.Join(root, ".agk.toml"), []byte(toml), 0o644)
	if code := h.env.ReviewStart(context.Background(), nil); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
}

// The out path goes into a claude permission rule: a repo path that could
// inject or widen rules is refused before anything exists.
func TestReviewStartRefusesRuleUnsafePathForClaude(t *testing.T) {
	for name, dirName := range map[string]string{
		"injection": "review),Bash,Read(x",
		"glob":      "star*",
		"space":     "my repo",
		"brace":     "a{b,c}",
	} {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t, true)
			base, _ := filepath.EvalSymlinks(t.TempDir())
			root := filepath.Join(base, dirName)
			os.MkdirAll(root, 0o755)
			scriptScope(h, root)
			if code := h.env.ReviewStart(context.Background(), []string{"--reviewer", "claude"}); code != 1 {
				t.Fatalf("exit %d, want 1 (stderr %q)", code, h.err)
			}
			if !strings.Contains(h.err.String(), "permission rule") || h.out.Len() != 0 {
				t.Fatalf("stderr %q stdout %q", h.err, h.out)
			}
			if exists(filepath.Join(root, ".agk")) || len(h.ctl.Calls) != 0 {
				t.Fatalf("no run dir and no companion expected: %v", h.ctl.Calls)
			}
		})
	}
}

// Only the interactive claude reviewer takes the rule: codex and inline runs
// do not care about such a path.
func TestReviewStartRuleUnsafePathOtherModesPass(t *testing.T) {
	for name, c := range map[string]struct {
		inAgterm bool
		args     []string
	}{
		"codex in agterm": {true, nil},
		"claude inline":   {false, []string{"--reviewer", "claude"}},
	} {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t, c.inAgterm)
			base, _ := filepath.EvalSymlinks(t.TempDir())
			root := filepath.Join(base, "review),Bash,Read(x")
			os.MkdirAll(root, 0o755)
			scriptScope(h, root)
			h.drivers["claude"].outputs = []string{approve}
			if code := h.env.ReviewStart(context.Background(), c.args); code != 0 {
				t.Fatalf("exit %d: %s", code, h.err)
			}
		})
	}
}

func TestReviewStartWritesScopeFingerprint(t *testing.T) {
	for _, inAgterm := range []bool{true, false} {
		h := newHarness(t, inAgterm)
		root, _ := filepath.EvalSymlinks(t.TempDir())
		scriptScope(h, root)
		h.drivers["codex"].outputs = []string{approve}
		if code := h.env.ReviewStart(context.Background(), nil); code != 0 {
			t.Fatalf("agterm=%v: exit %d: %s", inAgterm, code, h.err)
		}
		dir := filepath.Join(root, ".agk", "reviews", "20261105-100000")
		got, err := os.ReadFile(scopeFilePath(dir, 1))
		if err != nil || strings.TrimSpace(string(got)) != scopeFingerprint {
			t.Errorf("agterm=%v: scope.sha256 = %q, %v", inAgterm, got, err)
		}
	}
}

func TestReviewStartWritesMergeBase(t *testing.T) {
	for _, inAgterm := range []bool{true, false} {
		h := newHarness(t, inAgterm)
		root, _ := filepath.EvalSymlinks(t.TempDir())
		scriptScope(h, root)
		h.drivers["codex"].outputs = []string{approve}
		if code := h.env.ReviewStart(context.Background(), []string{"--base", "main"}); code != 0 {
			t.Fatalf("agterm=%v: exit %d: %s", inAgterm, code, h.err)
		}
		dir := filepath.Join(root, ".agk", "reviews", "20261105-100000")
		got, err := os.ReadFile(mergeBaseFilePath(dir, 1))
		if err != nil || string(got) != "mb\n" {
			t.Errorf("agterm=%v: merge-base = %q, %v; want the scope's merge base on one line", inAgterm, got, err)
		}
	}
}

func TestReviewStartPlanRoundHasNoMergeBase(t *testing.T) {
	h := newHarness(t, true)
	root, _ := filepath.EvalSymlinks(t.TempDir())
	scriptPlan(h, root)
	if code := h.env.ReviewStart(context.Background(), []string{"--change", "add-x"}); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	dir := filepath.Join(root, ".agk", "reviews", "20261105-100000")
	if exists(mergeBaseFilePath(dir, 1)) {
		t.Fatal("a plan round must not record a merge base")
	}
}

func TestReviewStartPlanRoundHasNoScopeFingerprint(t *testing.T) {
	h := newHarness(t, true)
	root, _ := filepath.EvalSymlinks(t.TempDir())
	scriptPlan(h, root)
	if code := h.env.ReviewStart(context.Background(), []string{"--change", "add-x"}); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	dir := filepath.Join(root, ".agk", "reviews", "20261105-100000")
	if exists(scopeFilePath(dir, 1)) {
		t.Fatal("a plan round must not record a scope fingerprint")
	}
}

func TestReviewStartPlanCompanionNameClaude(t *testing.T) {
	h := newHarness(t, true)
	root, _ := filepath.EvalSymlinks(t.TempDir())
	scriptPlan(h, root)
	if code := h.env.ReviewStart(context.Background(), []string{"--change", "add-x", "--reviewer", "claude"}); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	if c := h.ctl.Calls[0]; c.Method != "NewCompanion" || c.Args[1] != "add-x · claude-plan #1" {
		t.Fatalf("calls = %v", h.ctl.Calls)
	}
	for _, m := range h.ctl.Methods() {
		if strings.Contains(strings.ToLower(m), "rename") {
			t.Fatalf("agk must not rename a session, got %s", m)
		}
	}
}

func TestReviewStartForNamesDiffCompanion(t *testing.T) {
	for name, args := range map[string][]string{
		"with base": {"--base", "main", "--for", "add-auth"},
		"alone":     {"--for", "add-auth"},
		"equals":    {"--for=add-auth"},
	} {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t, true)
			root, _ := filepath.EvalSymlinks(t.TempDir())
			seen := scriptScope(h, root)
			if code := h.env.ReviewStart(context.Background(), args); code != 0 {
				t.Fatalf("exit %d: %s", code, h.err)
			}
			if !reflect.DeepEqual(*seen, []string{h.env.Cwd, "main"}) {
				t.Fatalf("collect args = %q", *seen)
			}
			if c := h.ctl.Calls[0]; c.Method != "NewCompanion" || c.Args[1] != "add-auth · codex-review #1" {
				t.Fatalf("calls = %v", h.ctl.Calls)
			}
			dir := filepath.Join(root, ".agk", "reviews", "20261105-100000")
			if s := readState(t, dir); s.For != "add-auth" || s.Base != "main" || s.Change != "" {
				t.Fatalf("worker.json = %+v", s)
			}
			var raw map[string]any
			b, _ := os.ReadFile(filepath.Join(dir, "worker.json"))
			if err := json.Unmarshal(b, &raw); err != nil {
				t.Fatal(err)
			}
			if _, has := raw["change"]; has || raw["for"] != "add-auth" {
				t.Fatalf("worker.json = %s", b)
			}
		})
	}
}

func TestReviewStartForRefusals(t *testing.T) {
	cases := map[string]struct {
		args []string
		want string
	}{
		"path-like": {[]string{"--for", "../secrets"}, `invalid change name "../secrets"`},
		"empty":     {[]string{"--for", ""}, `invalid change name ""`},
		"empty eq":  {[]string{"--for="}, `invalid change name ""`},
		"with plan": {[]string{"--change", "add-auth", "--for", "add-auth"}, "exclude each other"},
		"plan last": {[]string{"--for", "add-auth", "--change", "add-auth"}, "exclude each other"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t, true)
			root, _ := filepath.EvalSymlinks(t.TempDir())
			h.env.Cwd = root
			collected := false
			h.env.Collect = func(context.Context, string, string) (review.Scope, error) {
				collected = true
				return review.Scope{Root: root, Base: "main", MergeBase: "mb", Diff: "+x\n"}, nil
			}
			h.env.CollectPlan = func(context.Context, string, string) (review.PlanScope, error) {
				collected = true
				return samplePlanScope(root), nil
			}
			if code := h.env.ReviewStart(context.Background(), c.args); code != 1 {
				t.Fatalf("exit %d, want 1 (stderr %q)", code, h.err)
			}
			if !strings.Contains(h.err.String(), c.want) || h.out.Len() != 0 {
				t.Fatalf("stderr %q stdout %q", h.err, h.out)
			}
			if collected || exists(filepath.Join(root, ".agk")) || len(h.ctl.Calls) != 0 {
				t.Fatalf("nothing may be collected or started: calls %v", h.ctl.Calls)
			}
		})
	}
}

// --for only labels a run: the scope fingerprint is the same and the prompt
// differs only in the paths of the run's own schema and output file.
func TestReviewStartForDoesNotChangeScopeOrPrompt(t *testing.T) {
	h := newHarness(t, true)
	root, _ := filepath.EvalSymlinks(t.TempDir())
	scriptScope(h, root)
	if code := h.env.ReviewStart(context.Background(), []string{"--base", "main"}); code != 0 {
		t.Fatalf("plain: exit %d: %s", code, h.err)
	}
	if code := h.env.ReviewStart(context.Background(), []string{"--base", "main", "--for", "add-auth"}); code != 0 {
		t.Fatalf("for: exit %d: %s", code, h.err)
	}
	plain := filepath.Join(root, ".agk", "reviews", "20261105-100000")
	forDir := filepath.Join(root, ".agk", "reviews", "20261105-100000-2")
	if s := readState(t, forDir); s.For != "add-auth" {
		t.Fatalf("second run is not the --for run: %+v", s)
	}
	a, _ := os.ReadFile(scopeFilePath(plain, 1))
	b, _ := os.ReadFile(scopeFilePath(forDir, 1))
	if len(a) == 0 || !bytes.Equal(a, b) {
		t.Fatalf("scope.sha256 differs: %q vs %q", a, b)
	}
	norm := func(dir string) string {
		p, err := os.ReadFile(filepath.Join(dir, "rounds", "1", "prompt.md"))
		if err != nil {
			t.Fatal(err)
		}
		own := []string{filepath.Join(dir, "schema.json"), filepath.Join(dir, "out", "1.json")}
		for _, o := range own {
			if !strings.Contains(string(p), o) {
				t.Fatalf("prompt of %s does not name its own %s", dir, o)
			}
		}
		out := strings.ReplaceAll(string(p), own[0], "<schema>")
		return strings.ReplaceAll(out, own[1], "<out>")
	}
	if norm(plain) != norm(forDir) {
		t.Fatalf("prompts differ beyond run paths:\n%s\n---\n%s", norm(plain), norm(forDir))
	}
}

func TestReviewStartForInline(t *testing.T) {
	h := newHarness(t, false)
	root, _ := filepath.EvalSymlinks(t.TempDir())
	scriptScope(h, root)
	h.drivers["codex"].outputs = []string{threeFindings}
	if code := h.env.ReviewStart(context.Background(), []string{"--for", "add-auth"}); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	dir := filepath.Join(root, ".agk", "reviews", "20261105-100000")
	if s := readState(t, dir); s.For != "add-auth" || s.Base != "main" || s.Change != "" {
		t.Fatalf("worker.json = %+v", s)
	}
	if len(h.ctl.Calls) != 0 {
		t.Fatalf("no agterm calls expected: %v", h.ctl.Calls)
	}
}

// Two inline runs carry no output paths: with and without --for the prompts
// are byte for byte the same.
func TestReviewStartForInlinePromptsIdentical(t *testing.T) {
	h := newHarness(t, false)
	root, _ := filepath.EvalSymlinks(t.TempDir())
	scriptScope(h, root)
	h.drivers["codex"].outputs = []string{threeFindings, threeFindings}
	for _, args := range [][]string{nil, {"--for", "add-auth"}} {
		if code := h.env.ReviewStart(context.Background(), args); code != 0 {
			t.Fatalf("%v: exit %d: %s", args, code, h.err)
		}
	}
	a, _ := os.ReadFile(filepath.Join(root, ".agk", "reviews", "20261105-100000", "rounds", "1", "prompt.md"))
	b, _ := os.ReadFile(filepath.Join(root, ".agk", "reviews", "20261105-100000-2", "rounds", "1", "prompt.md"))
	if len(a) == 0 || !bytes.Equal(a, b) {
		t.Fatalf("inline prompts differ:\n%s\n---\n%s", a, b)
	}
}
