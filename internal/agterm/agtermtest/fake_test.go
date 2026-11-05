package agtermtest

import (
	"context"
	"errors"
	"reflect"
	"testing"

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
