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
	calls  []call
	stdout string
	stderr string
	code   int
	err    error
}

func (s *stubExec) run(_ context.Context, name string, args []string) (Result, error) {
	s.calls = append(s.calls, call{name, append([]string(nil), args...)})
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

func TestOverlayHTMLSubmitted(t *testing.T) {
	s := &stubExec{stdout: `{"pageID":"X","outcome":"submitted","value":"[{\"id\":1}]"}` + "\n"}
	out, err := newCLI(s).OverlayHTML(context.Background(), "S-1", "/r/triage.html")
	if err != nil {
		t.Fatal(err)
	}
	if out.Value != `[{"id":1}]` {
		t.Fatalf("value = %q", out.Value)
	}
	assertArgs(t, s, []string{"session", "overlay", "open", "--html", "/r/triage.html", "--js", "--block", "--follow", "--target", "S-1"})
	for _, a := range s.calls[0].args {
		if a == "--cwd" {
			t.Fatal("page must not get file access")
		}
	}
}

func TestOverlayHTMLDismissed(t *testing.T) {
	s := &stubExec{code: 2, stdout: `{"outcome":"dismissed","pageID":"X"}`}
	_, err := newCLI(s).OverlayHTML(context.Background(), "S-1", "/f.html")
	if !errors.Is(err, ErrDismissed) {
		t.Fatalf("err = %v, want ErrDismissed", err)
	}
}

func TestOverlayHTMLErrors(t *testing.T) {
	cases := map[string]*stubExec{
		"exit 1":                     {code: 1, stderr: "no socket"},
		"garbage stdout":             {stdout: "garbage"},
		"exit 0 but dismissed":       {stdout: `{"outcome":"dismissed"}`},
		"exit 2 but submitted":       {code: 2, stdout: `{"outcome":"submitted","value":"x"}`},
		"submitted no value":         {stdout: `{"outcome":"submitted"}`},
		"submitted value not string": {stdout: `{"outcome":"submitted","value":[1]}`},
		"exec failed":                {err: errors.New("enoent")},
	}
	for name, s := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := newCLI(s).OverlayHTML(context.Background(), "S", "/f.html")
			if err == nil || errors.Is(err, ErrDismissed) {
				t.Fatalf("err = %v, want non-dismiss error", err)
			}
		})
	}
	s := &stubExec{code: 1, stderr: "no socket"}
	if _, err := newCLI(s).OverlayHTML(context.Background(), "S", "/f.html"); !strings.Contains(err.Error(), "no socket") {
		t.Fatalf("err = %v, want stderr", err)
	}
}
