package agtermtest

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/shurastik/agterm-kit/internal/agterm"
)

func TestFakeRecordsAndScripts(t *testing.T) {
	ctx := context.Background()
	typed := ""
	f := &Fake{CompanionID: "C", InfoErr: errors.New("x"), TextOut: "screen", OnType: func(_, text string) { typed = text }}
	id, _ := f.NewCompanion(ctx, "P", "n", "cmd", "/d")
	_ = f.Status(ctx, "C", "active")
	infoErr := f.Info(ctx, "C", "md", 0)
	_ = f.Type(ctx, "C", "go\r")
	renameErr := f.Rename(ctx, "C", "new name")
	screen, _ := f.Text(ctx, "C", 20)
	_ = f.Info(ctx, "S", "line", 1200*time.Second)
	_ = f.ClearInfo(ctx, "S")
	_ = f.Tint(ctx, "C", "purple")
	if renameErr != nil {
		t.Fatalf("renameErr = %v", renameErr)
	}
	if id != "C" || infoErr == nil || typed != "go\r" || screen != "screen" {
		t.Fatalf("scripted results not returned: id=%q infoErr=%v typed=%q screen=%q", id, infoErr, typed, screen)
	}
	want := []Call{
		{"NewCompanion", []string{"P", "n", "cmd", "/d"}},
		{"Status", []string{"C", "active"}},
		{"Info", []string{"C", "md", "0"}},
		{"Type", []string{"C", "go\r"}},
		{"Rename", []string{"C", "new name"}},
		{"Text", []string{"C", "20"}},
		{"Info", []string{"S", "line", "1200"}},
		{"ClearInfo", []string{"S"}},
		{"Tint", []string{"C", "purple"}},
	}
	if !reflect.DeepEqual(f.Calls, want) {
		t.Fatalf("calls = %v", f.Calls)
	}
}

func TestFakeWaitSeq(t *testing.T) {
	f := &Fake{WaitStatus: "idle", WaitSeq: []WaitResult{{Status: "completed"}, {Err: agterm.ErrTimeout}}}
	ctx := context.Background()
	s1, _ := f.Wait(ctx, "C", nil, time.Second)
	_, e2 := f.Wait(ctx, "C", nil, time.Second)
	s3, _ := f.Wait(ctx, "C", nil, time.Second)
	if s1 != "completed" || !errors.Is(e2, agterm.ErrTimeout) || s3 != "idle" {
		t.Fatalf("%q %v %q", s1, e2, s3)
	}
}

func TestFakeTextSeq(t *testing.T) {
	boom := errors.New("boom")
	f := &Fake{TextOut: "last", TextSeq: []TextResult{{Out: "one"}, {Err: boom}}}
	ctx := context.Background()
	s1, _ := f.Text(ctx, "C", 12)
	_, e2 := f.Text(ctx, "C", 12)
	s3, e3 := f.Text(ctx, "C", 12)
	if s1 != "one" || !errors.Is(e2, boom) || s3 != "last" || e3 != nil {
		t.Fatalf("%q %v %q %v", s1, e2, s3, e3)
	}
	if got := f.Methods(); !reflect.DeepEqual(got, []string{"Text", "Text", "Text"}) {
		t.Fatalf("methods = %v", got)
	}
}

func TestFakeTextBlocksUntilContextDone(t *testing.T) {
	f := &Fake{TextBlocks: true, TextOut: "never"}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	out, err := f.Text(ctx, "C", 12)
	if out != "" || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("%q %v", out, err)
	}
}

func TestFakeWaitAndClose(t *testing.T) {
	ctx := context.Background()
	f := &Fake{WaitStatus: "completed", CloseErr: agterm.ErrGone}
	st, werr := f.Wait(ctx, "C", []string{"completed", "blocked"}, 90*time.Second)
	cerr := f.Close(ctx, "C")
	if st != "completed" || werr != nil || !errors.Is(cerr, agterm.ErrGone) {
		t.Fatalf("scripted results not returned: st=%q werr=%v cerr=%v", st, werr, cerr)
	}
	want := []Call{
		{"Wait", []string{"C", "completed,blocked", "1m30s"}},
		{"Close", []string{"C"}},
	}
	if !reflect.DeepEqual(f.Calls, want) {
		t.Fatalf("calls = %v", f.Calls)
	}
	f = &Fake{WaitErr: agterm.ErrTimeout}
	if _, err := f.Wait(ctx, "C", nil, time.Second); !errors.Is(err, agterm.ErrTimeout) {
		t.Fatalf("err = %v", err)
	}
}

func TestFakeRecordsDeadlines(t *testing.T) {
	f := &Fake{}
	_ = f.Type(context.Background(), "C", "a")
	ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
	defer cancel()
	_ = f.Type(ctx, "C", "b")
	want, _ := ctx.Deadline()
	if !f.Deadlines[0].IsZero() || !f.Deadlines[1].Equal(want) {
		t.Fatalf("deadlines = %v, want zero then %v", f.Deadlines, want)
	}
}

func TestFakeClearInfoAndTintErrors(t *testing.T) {
	ctx := context.Background()
	f := &Fake{ClearInfoErr: errors.New("a"), TintErr: errors.New("b")}
	if err := f.ClearInfo(ctx, "S"); err == nil {
		t.Error("ClearInfoErr not returned")
	}
	if err := f.Tint(ctx, "C", "purple"); err == nil {
		t.Error("TintErr not returned")
	}
}
