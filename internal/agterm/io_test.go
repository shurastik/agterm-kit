package agterm

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestType(t *testing.T) {
	s := &stubExec{stdout: `{"ok":true}`}
	if err := newCLI(s).Type(context.Background(), "C-1", "-read it.\r"); err != nil {
		t.Fatal(err)
	}
	assertArgs(t, s, []string{"session", "type", "--target", "C-1", "--json", "--", "-read it.\r"})
}

func TestTypeErrors(t *testing.T) {
	s := &stubExec{code: 1, stdout: `{"error":"no such session: C-1","ok":false}`}
	if err := newCLI(s).Type(context.Background(), "C-1", "x"); !errors.Is(err, ErrGone) {
		t.Fatalf("err = %v, want ErrGone", err)
	}
	s = &stubExec{code: 1, stderr: "boom"}
	if err := newCLI(s).Type(context.Background(), "C-1", "x"); err == nil || errors.Is(err, ErrGone) {
		t.Fatalf("err = %v, want plain error", err)
	}
}

func TestText(t *testing.T) {
	s := &stubExec{stdout: `{"ok":true,"result":{"pane":"left","text":"a\nb"}}`}
	got, err := newCLI(s).Text(context.Background(), "C-1", 20)
	if err != nil || got != "a\nb" {
		t.Fatalf("Text = %q, %v", got, err)
	}
	assertArgs(t, s, []string{"session", "text", "--target", "C-1", "--lines", "20", "--json"})
	for name, s := range map[string]*stubExec{
		"exit":     {code: 1, stdout: `{"error":"no such session: C","ok":false}`},
		"bad json": {stdout: `nope`},
		"not ok":   {stdout: `{"ok":false}`},
	} {
		if _, err := newCLI(s).Text(context.Background(), "C", 5); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
}

func TestWaitAcceptsIdle(t *testing.T) {
	s := &stubExec{stdout: `{"id":"C","status":"idle"}`}
	got, err := newCLI(s).Wait(context.Background(), "C", []string{"completed", "blocked", "idle"}, time.Minute)
	if err != nil || got != "idle" {
		t.Fatalf("Wait = %q, %v", got, err)
	}
}

func TestIOCallsUseSocketBeforeTerminator(t *testing.T) {
	ctx := context.Background()
	s := &stubExec{stdout: `{"ok":true,"result":{"text":"","messages":[]}}`}
	c := &CLI{Bin: "agtermctl", Socket: "/alt.sock", Exec: s.run}
	c.Type(ctx, "C", "-x")
	c.Text(ctx, "C", 3)
	for _, cl := range s.calls {
		args := cl.args
		i := len(args) // socket pair ends right before i
		if args[len(args)-2] == "--" {
			i = len(args) - 2
		}
		if args[i-2] != "--socket" || args[i-1] != "/alt.sock" {
			t.Errorf("--socket not right before the terminator/end: %q", args)
		}
	}
	if len(s.calls) != 2 {
		t.Fatalf("calls = %d", len(s.calls))
	}
}
