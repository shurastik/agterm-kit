package agterm

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

type call struct {
	name string
	args []string
}

type stubExec struct {
	calls   []call
	ctxErrs []error
	stdout  string
	stderr  string
	code    int
	err     error
}

func (s *stubExec) run(ctx context.Context, name string, args []string) (Result, error) {
	s.calls = append(s.calls, call{name, append([]string(nil), args...)})
	s.ctxErrs = append(s.ctxErrs, ctx.Err())
	if s.err != nil {
		return Result{}, s.err
	}
	return Result{Stdout: []byte(s.stdout), Stderr: []byte(s.stderr), ExitCode: s.code}, nil
}

func newCLI(s *stubExec) *CLI { return &CLI{Bin: "agtermctl", Exec: s.run} }

func assertArgs(t *testing.T, s *stubExec, want []string) {
	t.Helper()
	if len(s.calls) != 1 {
		t.Fatalf("calls = %d, want 1", len(s.calls))
	}
	if s.calls[0].name != "agtermctl" {
		t.Fatalf("bin = %q", s.calls[0].name)
	}
	if !reflect.DeepEqual(s.calls[0].args, want) {
		t.Fatalf("args =\n%q\nwant\n%q", s.calls[0].args, want)
	}
}

func TestNewCompanion(t *testing.T) {
	s := &stubExec{stdout: `{"ok":true,"result":{"id":"C-1","parent":"P-1"}}`}
	id, err := newCLI(s).NewCompanion(context.Background(), "P-1", "codex-review", "'/bin/agk' review run '/r'", "/repo")
	if err != nil {
		t.Fatal(err)
	}
	if id != "C-1" {
		t.Fatalf("id = %q", id)
	}
	assertArgs(t, s, []string{"session", "new",
		"--parent", "P-1",
		"--command", "'/bin/agk' review run '/r'",
		"--wait", "--no-select",
		"--name", "codex-review",
		"--cwd", "/repo",
		"--json"})
}

func TestNewCompanionErrors(t *testing.T) {
	cases := map[string]*stubExec{
		"exit code":   {code: 1, stderr: "no such session"},
		"not ok":      {stdout: `{"ok":false,"error":"boom"}`},
		"missing id":  {stdout: `{"ok":true,"result":{}}`},
		"bad json":    {stdout: `nope`},
		"exec failed": {err: errors.New("not found")},
	}
	for name, s := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := newCLI(s).NewCompanion(context.Background(), "P", "n", "c", "/d"); err == nil {
				t.Fatal("want error")
			}
		})
	}
}

func TestStatus(t *testing.T) {
	s := &stubExec{stdout: "ok"}
	if err := newCLI(s).Status(context.Background(), "C-1", "completed"); err != nil {
		t.Fatal(err)
	}
	assertArgs(t, s, []string{"session", "status", "completed", "--target", "C-1"})
}

func TestStatusError(t *testing.T) {
	s := &stubExec{code: 1, stderr: "bad target"}
	err := newCLI(s).Status(context.Background(), "C-1", "active")
	if err == nil || !strings.Contains(err.Error(), "bad target") {
		t.Fatalf("err = %v, want stderr in message", err)
	}
}

func TestInfo(t *testing.T) {
	s := &stubExec{stdout: "ok"}
	if err := newCLI(s).Info(context.Background(), "C-1", "-codex: reviewing"); err != nil {
		t.Fatal(err)
	}
	assertArgs(t, s, []string{"session", "info", "--markdown", "--hide-after", "0", "--target", "C-1", "--", "-codex: reviewing"})
}

func TestSocketPassedToEveryCall(t *testing.T) {
	ctx := context.Background()
	s := &stubExec{stdout: `{"ok":true,"result":{"id":"C"}}`}
	c := &CLI{Bin: "agtermctl", Socket: "/alt.sock", Exec: s.run}
	c.NewCompanion(ctx, "P", "n", "cmd", "/d")
	c.Status(ctx, "C", "active")
	c.Info(ctx, "C", "-x")
	for _, cl := range s.calls {
		args := strings.Join(cl.args, " ")
		if !strings.Contains(args, "--socket /alt.sock") {
			t.Errorf("no --socket in %q", args)
		}
	}
	info := s.calls[2].args
	if info[len(info)-2] != "--" || info[len(info)-1] != "-x" {
		t.Errorf("--socket must precede the -- terminator: %q", info)
	}
}

func TestResolveBin(t *testing.T) {
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	execOK := func(paths ...string) func(string) bool {
		return func(p string) bool {
			for _, q := range paths {
				if p == q {
					return true
				}
			}
			return false
		}
	}
	if got := ResolveBin(env(map[string]string{"AGTERMCTL": "/custom/agtermctl"}), execOK(BundledBin)); got != "/custom/agtermctl" {
		t.Errorf("override: %q", got)
	}
	if got := ResolveBin(env(nil), execOK(BundledBin)); got != BundledBin {
		t.Errorf("bundled: %q", got)
	}
	if got := ResolveBin(env(nil), execOK()); got != "agtermctl" {
		t.Errorf("path fallback: %q", got)
	}
}

// scripted answers per call and can cancel the context mid-call.
type scripted struct {
	calls   [][]string
	ctxErrs []error
	answer  func(ctx context.Context, args []string) Result
}

func (s *scripted) run(ctx context.Context, _ string, args []string) (Result, error) {
	s.calls = append(s.calls, append([]string(nil), args...))
	s.ctxErrs = append(s.ctxErrs, ctx.Err())
	return s.answer(ctx, args), nil
}

func treeWith(session, file string) string {
	return `{"windows":[{"workspaces":[{"sessions":[{"id":"` + session + `","htmlOverlays":[{"file":"` + file + `"}]}]}]}]}`
}
