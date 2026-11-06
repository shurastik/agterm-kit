package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/shurastik/agterm-kit/internal/agterm/agtermtest"
	"github.com/shurastik/agterm-kit/internal/config"
	"github.com/shurastik/agterm-kit/internal/harness"
	"github.com/shurastik/agterm-kit/internal/review"
	"github.com/shurastik/agterm-kit/internal/rundir"
)

// fakeDriver scripts harness turns.
type fakeDriver struct {
	name     string
	outputs  []string // Review: written to OutputFile per call; "" writes nothing
	errs     []error  // per call
	sessions []string // session returned per call; default: the turn's, else "SESS-<name>"
	turns    []harness.Turn
	files    []harness.ApplierFiles
	headless []harness.Turn
	// onHeadless runs inside Headless (e.g. to write the round report).
	onHeadless func(t harness.Turn)
}

func (f *fakeDriver) Name() string { return f.name }

func (f *fakeDriver) Interactive(w config.Worker, root string, files harness.ApplierFiles, session, kickoff string) []string {
	return []string{f.name, "--model=" + w.Model, "--session=" + session, kickoff}
}

func (f *fakeDriver) pick(i int, t harness.Turn) (string, error) {
	var err error
	if i < len(f.errs) {
		err = f.errs[i]
	}
	sess := t.Session
	if i < len(f.sessions) {
		sess = f.sessions[i]
	} else if sess == "" {
		sess = "SESS-" + f.name
	}
	return sess, err
}

func (f *fakeDriver) Headless(_ context.Context, t harness.Turn, files harness.ApplierFiles, term io.Writer) (string, error) {
	f.headless = append(f.headless, t)
	f.files = append(f.files, files)
	io.WriteString(term, f.name+" applying\n")
	if f.onHeadless != nil {
		f.onHeadless(t)
	}
	return f.pick(len(f.headless)-1, t)
}

func (f *fakeDriver) Review(_ context.Context, t harness.Turn, term io.Writer) (string, error) {
	f.turns = append(f.turns, t)
	i := len(f.turns) - 1
	io.WriteString(term, f.name+" streaming\n")
	if i < len(f.outputs) && f.outputs[i] != "" {
		if err := os.WriteFile(filepath.Join(t.Dir, harness.OutputFile), []byte(f.outputs[i]), 0o644); err != nil {
			return "", err
		}
	}
	return f.pick(i, t)
}

type tenv struct {
	env      *Env
	ctl      *agtermtest.Fake
	drivers  map[string]*fakeDriver
	out, err *bytes.Buffer
	vars     map[string]string
}

func newHarness(t *testing.T, inAgterm bool) *tenv {
	t.Helper()
	old, oldSlice := statusRetryDelay, applyPollSlice
	statusRetryDelay = 0
	applyPollSlice = 24 * time.Hour // one Wait call per wait unless a test says otherwise
	t.Cleanup(func() { statusRetryDelay, applyPollSlice = old, oldSlice })
	h := &tenv{
		ctl:     &agtermtest.Fake{CompanionID: "COMP-1"},
		drivers: map[string]*fakeDriver{"codex": {name: "codex"}, "claude": {name: "claude"}},
		out:     &bytes.Buffer{},
		err:     &bytes.Buffer{},
		vars:    map[string]string{},
	}
	if inAgterm {
		h.vars["AGTERM_ENABLED"] = "1"
		h.vars["AGTERM_SESSION_ID"] = "SELF"
	}
	h.env = &Env{
		Stdout: h.out,
		Stderr: h.err,
		Getenv: func(k string) string { return h.vars[k] },
		Ctl:    h.ctl,
		Driver: func(name string) (harness.Driver, error) {
			if d, ok := h.drivers[name]; ok {
				return d, nil
			}
			return nil, errors.New("no driver " + name)
		},
		Now:          func() time.Time { return time.Date(2026, 11, 5, 10, 0, 0, 0, time.Local) },
		Executable:   func() (string, error) { return "/usr/local/bin/agk", nil },
		Cwd:          t.TempDir(),
		Home:         t.TempDir(),
		Stdin:        strings.NewReader(""),
		NewSessionID: func() string { return "SESS-NEW" },
		ApplyState: func(context.Context, string, string) (string, string, error) {
			return "ready", "", nil
		},
		RepoRoot: func(context.Context, string) (string, error) {
			return "", errors.New("not a git repository")
		},
		Collect: func(context.Context, string, string) (review.Scope, error) {
			return review.Scope{}, errors.New("collect not scripted")
		},
	}
	return h
}

// runDir makes a codex review run <root>/.agk/reviews/<ts>/ at round 1 with
// rounds/1/prompt.md and returns root and dir.
func runDir(t *testing.T) (string, string) {
	t.Helper()
	return runDirWith(t, config.Worker{Harness: config.Codex}, "")
}

func runDirWith(t *testing.T, w config.Worker, session string) (string, string) {
	t.Helper()
	root, _ := filepath.EvalSymlinks(t.TempDir())
	dir := filepath.Join(root, ".agk", "reviews", "20261105-100000")
	rd := filepath.Join(dir, "rounds", "1")
	if err := os.MkdirAll(rd, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := rundir.WriteJSON(dir, workerFile, reviewState{Worker: w, Base: "main", Session: session}); err != nil {
		t.Fatal(err)
	}
	if err := rundir.SetRound(dir, 1); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rd, "prompt.md"), []byte("THE PROMPT"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root, dir
}

func exists(p string) bool { _, err := os.Stat(p); return err == nil }
