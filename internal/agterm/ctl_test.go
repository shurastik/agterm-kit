package agterm

import (
	"context"
	"errors"
	"os"
	"path/filepath"
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

func TestSocketPassedToEveryCall(t *testing.T) {
	ctx := context.Background()
	s := &stubExec{stdout: `{"ok":true,"result":{"id":"C"}}`}
	c := &CLI{Bin: "agtermctl", Socket: "/alt.sock", Exec: s.run}
	c.NewCompanion(ctx, "P", "n", "cmd", "/d")
	c.Status(ctx, "C", "active")
	c.Info(ctx, "C", "-x")
	s.stdout = `{"outcome":"submitted","value":"v"}`
	c.OverlayHTML(ctx, "S", "/f.html")
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

func TestOverlayHTMLCancelledBeforeOpenTouchesNothing(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s := &scripted{answer: func(context.Context, []string) Result { return Result{} }}
	c := &CLI{Bin: "agtermctl", Exec: s.run}
	if _, err := c.OverlayHTML(ctx, "S-1", "/f.html"); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
	if len(s.calls) != 0 {
		t.Fatalf("an existing overlay must be left alone, calls = %q", s.calls)
	}
}

// Cancelling while our page is open closes it: the page belongs to the agterm
// app and would otherwise outlive agk and block the next triage.
func TestOverlayHTMLCancelClosesOwnPage(t *testing.T) {
	dir, _ := filepath.EvalSymlinks(t.TempDir())
	file := filepath.Join(dir, "triage.html")
	os.WriteFile(file, nil, 0o644)
	for name, tc := range map[string]struct {
		shown     string
		wantClose bool
	}{
		"own page":     {file, true},
		"foreign page": {filepath.Join(dir, "other.html"), false},
	} {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			s := &scripted{answer: func(_ context.Context, args []string) Result {
				switch args[1] {
				case "overlay":
					if args[2] == "open" {
						cancel() // user hits Ctrl-C while the page is up
						return Result{ExitCode: -1}
					}
					return Result{Stdout: []byte("ok")}
				default: // tree
					return Result{Stdout: []byte(treeWith("S-1", tc.shown))}
				}
			}}
			c := &CLI{Bin: "agtermctl", Socket: "/s", Window: "W-1", Exec: s.run}
			if _, err := c.OverlayHTML(ctx, "S-1", file); !errors.Is(err, context.Canceled) {
				t.Fatalf("err = %v", err)
			}
			closed := false
			for i, a := range s.calls {
				// tree shows only one window: ask for the caller's, not the frontmost.
				if a[0] == "tree" && !reflect.DeepEqual(a, []string{"tree", "--json", "--window", "W-1", "--socket", "/s"}) {
					t.Errorf("tree args = %q", a)
				}
				if strings.Join(a[:3], " ") == "session overlay close" {
					closed = true
					if !reflect.DeepEqual(a, []string{"session", "overlay", "close", "--target", "S-1", "--socket", "/s"}) {
						t.Errorf("close args = %q", a)
					}
					if s.ctxErrs[i] != nil {
						t.Error("close must run on a detached context")
					}
				}
			}
			if closed != tc.wantClose {
				t.Fatalf("closed = %v, want %v; calls %q", closed, tc.wantClose, s.calls)
			}
		})
	}
}
