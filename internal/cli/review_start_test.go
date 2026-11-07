package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/shurastik/agterm-kit/internal/review"
	"github.com/shurastik/agterm-kit/internal/rundir"
)

func scriptScope(h *tenv, root string) *[]string {
	var seen []string
	h.env.Collect = func(_ context.Context, dir, base string) (review.Scope, error) {
		seen = append(seen, dir, base)
		return review.Scope{Root: root, Base: base, MergeBase: "mb", Diff: "+x\n", Stat: " f | 1 +\n"}, nil
	}
	return &seen
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
	if len(h.ctl.Calls) != 1 || h.ctl.Calls[0].Method != "NewCompanion" {
		t.Fatalf("calls = %v", h.ctl.Calls)
	}
	// The caller's PATH is pinned so codex and agtermctl resolve inside the
	// companion even when agterm starts it without a login shell.
	wantCmd := fmt.Sprintf("env 'PATH=/opt/homebrew/bin:/usr/bin' '/usr/local/bin/agk' review run '%s'", dir)
	if got := h.ctl.Calls[0].Args; !reflect.DeepEqual(got, []string{"SELF", "codex-review #1", wantCmd, root}) {
		t.Fatalf("companion args = %q", got)
	}
	var out struct {
		Companion *string `json:"companion"`
		Dir       string  `json:"dir"`
	}
	if err := json.Unmarshal(h.out.Bytes(), &out); err != nil {
		t.Fatalf("stdout %q: %v", h.out, err)
	}
	if out.Companion == nil || *out.Companion != "COMP-1" || out.Dir != dir {
		t.Fatalf("stdout = %s", h.out)
	}
	if len(h.drivers["codex"].turns) != 0 {
		t.Fatal("start must not run the reviewer itself inside agterm")
	}
	if s := readState(t, dir); s.Worker.Harness != "codex" || s.Base != "develop" || s.Session != "" {
		t.Fatalf("worker.json = %+v", s)
	}
	if k, err := rundir.Round(dir); err != nil || k != 1 {
		t.Fatalf("round = %d, %v", k, err)
	}
	if id, err := os.ReadFile(filepath.Join(dir, "companion")); err != nil || string(id) != "COMP-1\n" {
		t.Fatalf("companion file = %q, %v", id, err)
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
			if code := h.env.ReviewStart(context.Background(), nil); code == 0 {
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
	h.env.ReviewStart(context.Background(), nil)
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
	if cmd := h.ctl.Calls[0].Args[2]; !strings.HasPrefix(cmd, "'/usr/local/bin/agk' review run ") {
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
	if !strings.HasPrefix(cmd, "env 'PATH=/usr/bin' 'CODEX_HOME=/custom/codex' 'CLAUDE_CONFIG_DIR=/custom/claude' 'AGTERMCTL=/opt/agterm/agtermctl' '/usr/local/bin/agk' review run ") {
		t.Fatalf("command = %q", cmd)
	}
	if strings.Contains(cmd, "sk-secret") {
		t.Fatal("secrets must never be put on the companion command line")
	}
}

func TestReviewStartClaudeReviewer(t *testing.T) {
	h := newHarness(t, true)
	root, _ := filepath.EvalSymlinks(t.TempDir())
	scriptScope(h, root)
	if code := h.env.ReviewStart(context.Background(), []string{"--reviewer", "claude:claude-opus-5-5"}); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	if name := h.ctl.Calls[0].Args[1]; name != "claude-review #1" {
		t.Fatalf("companion name = %q", name)
	}
	dir := filepath.Join(root, ".agk", "reviews", "20261105-100000")
	if s := readState(t, dir); s.Worker.Harness != "claude" || s.Worker.Model != "claude-opus-5-5" {
		t.Fatalf("worker.json = %+v", s)
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
	want, _ := review.PlanPrompt(samplePlanScope(root))
	if err != nil || string(prompt) != want {
		t.Fatalf("prompt.md is not the plan prompt: %v\n%s", err, prompt)
	}
	if s := readState(t, dir); s.Change != "add-x" || s.Base != "" || s.Worker.Harness != "codex" {
		t.Fatalf("worker.json = %+v", s)
	}
	if len(h.ctl.Calls) != 1 || h.ctl.Calls[0].Method != "NewCompanion" || h.ctl.Calls[0].Args[1] != "codex-review #1" {
		t.Fatalf("calls = %v", h.ctl.Calls)
	}
	if !strings.Contains(h.out.String(), `"companion":"COMP-1"`) {
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
