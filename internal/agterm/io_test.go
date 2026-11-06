package agterm

import (
	"context"
	"errors"
	"reflect"
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

func TestMsgSend(t *testing.T) {
	s := &stubExec{stdout: `{"ok":true,"result":{"delivered":["C:P"],"rejected":[],"messageID":"M"}}`}
	if err := newCLI(s).MsgSend(context.Background(), "C:P", "round 2"); err != nil {
		t.Fatal(err)
	}
	assertArgs(t, s, []string{"msg", "send", "--to", "C:P", "--json", "--", "round 2"})
}

func TestMsgSendRejected(t *testing.T) {
	s := &stubExec{code: 1, stdout: `{"ok":true,"result":{"delivered":[],"rejected":[{"address":"C:P","reason":"unknown pane"}]}}`}
	err := newCLI(s).MsgSend(context.Background(), "C:P", "x")
	if !errors.Is(err, ErrGone) {
		t.Fatalf("err = %v, want ErrGone", err)
	}
	s = &stubExec{code: 1, stderr: "socket refused"}
	if err := newCLI(s).MsgSend(context.Background(), "C:P", "x"); err == nil || errors.Is(err, ErrGone) {
		t.Fatalf("err = %v, want plain error", err)
	}
}

func TestMsgRead(t *testing.T) {
	s := &stubExec{stdout: `{"result":{"remaining":0,"messages":[{"body":"round 2","from":"A:B","id":"M1"},{"body":"round 3","id":"M2"}],"acked":["M1","M2"]},"ok":true}`}
	msgs, err := newCLI(s).MsgRead(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	want := []Message{{ID: "M1", From: "A:B", Body: "round 2"}, {ID: "M2", Body: "round 3"}}
	if !reflect.DeepEqual(msgs, want) {
		t.Fatalf("msgs = %+v", msgs)
	}
	assertArgs(t, s, []string{"msg", "read", "--wait", "--ack", "--json"})

	s = &stubExec{code: 124, stdout: "timed out"}
	if _, err := newCLI(s).MsgRead(context.Background(), 1500*time.Millisecond); !errors.Is(err, ErrTimeout) {
		t.Fatalf("err = %v, want ErrTimeout", err)
	}
	assertArgs(t, s, []string{"msg", "read", "--wait", "--ack", "--json", "--timeout", "2"})

	if _, err := newCLI(&stubExec{stdout: "x"}).MsgRead(context.Background(), 0); err == nil {
		t.Fatal("bad json must error")
	}
	if _, err := newCLI(&stubExec{code: 1, stderr: "boom"}).MsgRead(context.Background(), 0); err == nil {
		t.Fatal("exit 1 must error")
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
	c.MsgSend(ctx, "C:P", "-y")
	c.MsgRead(ctx, 0)
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
	if len(s.calls) != 4 {
		t.Fatalf("calls = %d", len(s.calls))
	}
}
