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
)

func scriptScope(h *harness, root string) *[]string {
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
	prompt, err := os.ReadFile(filepath.Join(dir, "prompt.md"))
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
	if len(h.runner.calls) != 0 {
		t.Fatal("start must not run codex itself inside agterm")
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
	h.runner.output = threeFindings
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
		t.Fatal("codex output goes to stderr inline so stdout stays JSON")
	}
	if !exists(filepath.Join(dir, "review.json")) {
		t.Fatal("review.json missing")
	}
}

func TestReviewStartOutsideAgtermFailure(t *testing.T) {
	h := newHarness(t, false)
	scriptScope(h, t.TempDir())
	h.runner.err = errors.New("codex failed: boom")
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
	h.vars["AGTERMCTL"] = "/opt/agterm/agtermctl"
	h.vars["OPENAI_API_KEY"] = "sk-secret"
	scriptScope(h, t.TempDir())
	if code := h.env.ReviewStart(context.Background(), nil); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	cmd := h.ctl.Calls[0].Args[2]
	if !strings.HasPrefix(cmd, "env 'PATH=/usr/bin' 'CODEX_HOME=/custom/codex' 'AGTERMCTL=/opt/agterm/agtermctl' '/usr/local/bin/agk' review run ") {
		t.Fatalf("command = %q", cmd)
	}
	if strings.Contains(cmd, "sk-secret") {
		t.Fatal("secrets must never be put on the companion command line")
	}
}
