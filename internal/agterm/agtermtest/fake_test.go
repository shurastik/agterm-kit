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
	infoErr := f.Info(ctx, "C", "md")
	_ = f.Type(ctx, "C", "go\r")
	renameErr := f.Rename(ctx, "C", "new name")
	screen, _ := f.Text(ctx, "C", 20)
	if renameErr != nil {
		t.Fatalf("renameErr = %v", renameErr)
	}
	if id != "C" || infoErr == nil || typed != "go\r" || screen != "screen" {
		t.Fatalf("scripted results not returned: id=%q infoErr=%v typed=%q screen=%q", id, infoErr, typed, screen)
	}
	want := []Call{
		{"NewCompanion", []string{"P", "n", "cmd", "/d"}},
		{"Status", []string{"C", "active"}},
		{"Info", []string{"C", "md"}},
		{"Type", []string{"C", "go\r"}},
		{"Rename", []string{"C", "new name"}},
		{"Text", []string{"C", "20"}},
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
