package agterm

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestWait(t *testing.T) {
	s := &stubExec{stdout: `{"id":"C-1","status":"completed"}`}
	got, err := newCLI(s).Wait(context.Background(), "C-1", []string{"completed", "blocked"}, 1800*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if got != "completed" {
		t.Fatalf("status = %q", got)
	}
	assertArgs(t, s, []string{"session", "wait",
		"--target", "C-1",
		"--until", "completed", "--until", "blocked",
		"--timeout", "1800",
		"--json"})
}

func TestWaitOutcomes(t *testing.T) {
	cases := map[string]struct {
		s    *stubExec
		want error
	}{
		"left during wait": {&stubExec{code: 2}, ErrGone},
		"already gone":     {&stubExec{code: 1, stderr: "Error: no such session: C-1\n"}, ErrGone},
		"timeout":          {&stubExec{code: 124}, ErrTimeout},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := newCLI(c.s).Wait(context.Background(), "C-1", []string{"completed"}, time.Minute)
			if !errors.Is(err, c.want) {
				t.Fatalf("err = %v, want %v", err, c.want)
			}
		})
	}
}

func TestWaitErrors(t *testing.T) {
	cases := map[string]*stubExec{
		"other exit":     {code: 1, stderr: "socket refused"},
		"bad json":       {stdout: `nope`},
		"missing status": {stdout: `{"id":"C-1"}`},
		"exec failed":    {err: errors.New("not found")},
	}
	for name, s := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := newCLI(s).Wait(context.Background(), "C-1", []string{"completed"}, time.Minute)
			if err == nil || errors.Is(err, ErrGone) || errors.Is(err, ErrTimeout) {
				t.Fatalf("err = %v, want a plain error", err)
			}
		})
	}
	s := &stubExec{code: 1, stderr: "socket refused"}
	_, err := newCLI(s).Wait(context.Background(), "C-1", []string{"completed"}, time.Minute)
	if !strings.Contains(err.Error(), "socket refused") {
		t.Fatalf("err = %v, want stderr", err)
	}
}

func TestWaitRoundsTimeoutUp(t *testing.T) {
	s := &stubExec{stdout: `{"id":"C","status":"blocked"}`}
	newCLI(s).Wait(context.Background(), "C", []string{"blocked"}, 1500*time.Millisecond)
	args := s.calls[0].args
	for i, a := range args {
		if a == "--timeout" && args[i+1] == "2" {
			return
		}
	}
	t.Fatalf("args = %q, want --timeout 2", args)
}

func TestClose(t *testing.T) {
	s := &stubExec{stdout: `{"ok":true}`}
	if err := newCLI(s).Close(context.Background(), "C-1"); err != nil {
		t.Fatal(err)
	}
	assertArgs(t, s, []string{"session", "close", "--target", "C-1", "--undoable", "--json"})
}

func TestCloseGone(t *testing.T) {
	s := &stubExec{code: 1, stdout: `{"ok":false,"error":"no such session: C-1"}`}
	if err := newCLI(s).Close(context.Background(), "C-1"); !errors.Is(err, ErrGone) {
		t.Fatalf("err = %v, want ErrGone", err)
	}
}

func TestCloseErrors(t *testing.T) {
	cases := map[string]*stubExec{
		"other error": {code: 1, stdout: `{"ok":false,"error":"boom"}`},
		"bare exit":   {code: 1, stderr: "socket refused"},
		"exec failed": {err: errors.New("not found")},
	}
	for name, s := range cases {
		t.Run(name, func(t *testing.T) {
			err := newCLI(s).Close(context.Background(), "C-1")
			if err == nil || errors.Is(err, ErrGone) {
				t.Fatalf("err = %v, want a plain error", err)
			}
		})
	}
}

func TestWaitAndCloseUseSocket(t *testing.T) {
	s := &stubExec{stdout: `{"id":"C","status":"completed"}`}
	c := &CLI{Bin: "agtermctl", Socket: "/alt.sock", Exec: s.run}
	c.Wait(context.Background(), "C", []string{"completed"}, time.Minute)
	c.Close(context.Background(), "C")
	for _, cl := range s.calls {
		if !strings.Contains(strings.Join(cl.args, " "), "--socket /alt.sock") {
			t.Errorf("no --socket in %q", cl.args)
		}
	}
}
