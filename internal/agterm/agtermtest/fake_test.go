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
	seen := ""
	f := &Fake{CompanionID: "C", Outcome: agterm.Outcome{Value: "v"}, InfoErr: errors.New("x"), OnOverlay: func(p string) { seen = p }}
	id, _ := f.NewCompanion(ctx, "P", "n", "cmd", "/d")
	_ = f.Status(ctx, "C", "active")
	infoErr := f.Info(ctx, "C", "md")
	out, _ := f.OverlayHTML(ctx, "S", "/p.html")
	if id != "C" || out.Value != "v" || infoErr == nil || seen != "/p.html" {
		t.Fatalf("scripted results not returned: id=%q out=%q infoErr=%v seen=%q", id, out.Value, infoErr, seen)
	}
	want := []Call{
		{"NewCompanion", []string{"P", "n", "cmd", "/d"}},
		{"Status", []string{"C", "active"}},
		{"Info", []string{"C", "md"}},
		{"OverlayHTML", []string{"S", "/p.html"}},
	}
	if !reflect.DeepEqual(f.Calls, want) {
		t.Fatalf("calls = %v", f.Calls)
	}
	if !reflect.DeepEqual(f.Methods(), []string{"NewCompanion", "Status", "Info", "OverlayHTML"}) {
		t.Fatalf("methods = %v", f.Methods())
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
