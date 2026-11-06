package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/shurastik/agterm-kit/internal/agterm/agtermtest"
	"github.com/shurastik/agterm-kit/internal/review"
)

// fakeRunner writes a scripted codex output file.
type fakeRunner struct {
	output string // written to OutputPath when non-empty
	err    error
	calls  []string // root, dir, prompt
}

func (f *fakeRunner) OutputPath(dir string) string { return filepath.Join(dir, "codex-output.json") }

func (f *fakeRunner) Run(_ context.Context, root, dir, prompt string, term io.Writer) error {
	f.calls = append(f.calls, root, dir, prompt)
	io.WriteString(term, "codex streaming\n")
	if f.output != "" {
		if err := os.WriteFile(f.OutputPath(dir), []byte(f.output), 0o644); err != nil {
			return err
		}
	}
	return f.err
}

type harness struct {
	env      *Env
	ctl      *agtermtest.Fake
	runner   *fakeRunner
	out, err *bytes.Buffer
	vars     map[string]string
}

func newHarness(t *testing.T, inAgterm bool) *harness {
	t.Helper()
	old := statusRetryDelay
	statusRetryDelay = 0
	t.Cleanup(func() { statusRetryDelay = old })
	h := &harness{
		ctl:    &agtermtest.Fake{CompanionID: "COMP-1"},
		runner: &fakeRunner{},
		out:    &bytes.Buffer{},
		err:    &bytes.Buffer{},
		vars:   map[string]string{},
	}
	if inAgterm {
		h.vars["AGTERM_ENABLED"] = "1"
		h.vars["AGTERM_SESSION_ID"] = "SELF"
	}
	h.env = &Env{
		Stdout:     h.out,
		Stderr:     h.err,
		Getenv:     func(k string) string { return h.vars[k] },
		Ctl:        h.ctl,
		Runner:     h.runner,
		Now:        func() time.Time { return time.Date(2026, 11, 5, 10, 0, 0, 0, time.Local) },
		Executable: func() (string, error) { return "/usr/local/bin/agk", nil },
		Cwd:        t.TempDir(),
		Collect: func(context.Context, string, string) (review.Scope, error) {
			return review.Scope{}, errors.New("collect not scripted")
		},
	}
	return h
}

// runDir makes <root>/.agk/reviews/<ts>/prompt.md and returns root and dir.
func runDir(t *testing.T) (string, string) {
	t.Helper()
	root, _ := filepath.EvalSymlinks(t.TempDir())
	dir := filepath.Join(root, ".agk", "reviews", "20261105-100000")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "prompt.md"), []byte("THE PROMPT"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root, dir
}

func exists(p string) bool { _, err := os.Stat(p); return err == nil }
