package cli

import (
	"context"
	"strings"
	"testing"
)

func TestMainUsage(t *testing.T) {
	for _, args := range [][]string{nil, {"--help"}, {"-h"}, {"help"}} {
		h := newHarness(t, true)
		code := h.env.Main(context.Background(), args)
		help := len(args) > 0
		if help && code != 0 || !help && code != ExitError {
			t.Errorf("%q: exit %d", args, code)
		}
		if !strings.Contains(h.out.String()+h.err.String(), "agk review start") {
			t.Errorf("%q: usage missing", args)
		}
	}
}

func TestMainUnknown(t *testing.T) {
	for _, args := range [][]string{{"bogus"}, {"review"}, {"review", "bogus"}} {
		h := newHarness(t, true)
		if code := h.env.Main(context.Background(), args); code != ExitError {
			t.Errorf("%q: exit %d", args, code)
		}
	}
}

// The reviewer-loop subcommand of the old companion is gone.
func TestMainReviewRunIsGone(t *testing.T) {
	h := newHarness(t, false)
	_, dir := runDir(t)
	if code := h.env.Main(context.Background(), []string{"review", "run", dir}); code != ExitError {
		t.Fatalf("exit %d, want 1", code)
	}
	if !strings.Contains(h.err.String(), `unknown review command "run"`) || len(h.drivers["codex"].turns) != 0 {
		t.Fatalf("stderr = %q", h.err)
	}
}

func TestMainReviewHelp(t *testing.T) {
	h := newHarness(t, true)
	if code := h.env.Main(context.Background(), []string{"review", "--help"}); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(h.out.String(), "--base") {
		t.Fatalf("review help = %q", h.out)
	}
}

func TestReviewStartHelpExitsZero(t *testing.T) {
	h := newHarness(t, true)
	if code := h.env.Main(context.Background(), []string{"review", "start", "--help"}); code != 0 {
		t.Fatalf("exit %d", code)
	}
}

// Every usage text documents the plan review mode, and `review start -h`
// lists the flag.
func TestUsageDocumentsChange(t *testing.T) {
	const line = "agk review start [--base REF [--for NAME] | --change NAME] [--reviewer H[:M]]"
	for name, text := range map[string]string{"usage": usage, "review usage": reviewUsage} {
		if !strings.Contains(text, line) {
			t.Errorf("%s lacks %q", name, line)
		}
	}
	h := newHarness(t, true)
	h.env.Main(context.Background(), []string{"review", "start", "-h"})
	if !strings.Contains(h.err.String(), "-change") || !strings.Contains(h.err.String(), "-for") {
		t.Fatalf("review start -h = %q", h.err)
	}
}

// Every usage text documents the worktree flags and the new stdout keys of
// `apply start`.
func TestUsageDocumentsWorktreeMode(t *testing.T) {
	const line = "agk apply start <change> [--applier H[:M]] [--worktree | --no-worktree] [--no-pull]"
	for name, text := range map[string]string{"usage": usage, "apply usage": applyUsage} {
		if !strings.Contains(text, line) {
			t.Errorf("%s lacks %q", name, line)
		}
	}
	for _, key := range []string{`"worktree"`, `"pulled"`} {
		if !strings.Contains(usage, key) {
			t.Errorf("usage lacks the %s stdout key", key)
		}
	}
	h := newHarness(t, true)
	h.env.Main(context.Background(), []string{"apply", "start", "-h"})
	for _, f := range []string{"-worktree", "-no-worktree", "-no-pull"} {
		if !strings.Contains(h.err.String(), f) {
			t.Errorf("apply start -h lacks %s: %q", f, h.err)
		}
	}
}
