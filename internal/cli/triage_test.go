package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/shurastik/agterm-kit/internal/agterm"
	"github.com/shurastik/agterm-kit/internal/triage"
)

func reviewFile(t *testing.T, content string) string {
	t.Helper()
	_, dir := runDir(t)
	p := filepath.Join(dir, "review.json")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

const allDecided = `[{"id":1,"decision":"fix","note":""},{"id":2,"decision":"skip","note":""},{"id":3,"decision":"discuss","note":"why not a mutex?"}]`

func TestTriageSubmitted(t *testing.T) {
	h := newHarness(t, true)
	p := reviewFile(t, threeFindings)
	var page string
	h.ctl.Outcome = agterm.Outcome{Value: allDecided}
	h.ctl.OnOverlay = func(f string) { b, _ := os.ReadFile(f); page = string(b) }

	if code := h.env.Triage(context.Background(), []string{p}); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	htmlPath := filepath.Join(filepath.Dir(p), "triage.html")
	want := []string{"SELF", htmlPath}
	if len(h.ctl.Calls) != 1 || h.ctl.Calls[0].Method != "OverlayHTML" || !reflect.DeepEqual(h.ctl.Calls[0].Args, want) {
		t.Fatalf("calls = %v", h.ctl.Calls)
	}
	if !strings.Contains(page, `name="d-3"`) {
		t.Fatal("overlay page was not the rendered triage page")
	}
	var got []triage.Decision
	if err := json.Unmarshal(h.out.Bytes(), &got); err != nil {
		t.Fatalf("stdout %q: %v", h.out, err)
	}
	if len(got) != 3 || got[2].Decision != "discuss" || got[2].Note != "why not a mutex?" {
		t.Fatalf("decisions = %+v", got)
	}
	saved, err := os.ReadFile(filepath.Join(filepath.Dir(p), "decisions.json"))
	if err != nil || strings.TrimSpace(string(saved)) != strings.TrimSpace(h.out.String()) {
		t.Fatalf("decisions.json = %q (%v), stdout %q", saved, err, h.out)
	}
}

func TestTriageDismissed(t *testing.T) {
	h := newHarness(t, true)
	p := reviewFile(t, threeFindings)
	h.ctl.OverlayErr = agterm.ErrDismissed
	if code := h.env.Triage(context.Background(), []string{p}); code != ExitDismissed {
		t.Fatalf("exit %d", code)
	}
	if h.out.Len() != 0 || exists(filepath.Join(filepath.Dir(p), "decisions.json")) {
		t.Fatal("dismissed triage must emit nothing")
	}
}

func TestTriageGarbageResult(t *testing.T) {
	h := newHarness(t, true)
	p := reviewFile(t, threeFindings)
	h.ctl.Outcome = agterm.Outcome{Value: `[{"id":1,"decision":"fix","note":""}]`}
	if code := h.env.Triage(context.Background(), []string{p}); code != ExitError {
		t.Fatalf("exit %d", code)
	}
	if h.out.Len() != 0 || exists(filepath.Join(filepath.Dir(p), "decisions.json")) {
		t.Fatal("invalid result must emit nothing")
	}
	if h.err.Len() == 0 {
		t.Fatal("error must be explained on stderr")
	}
}

func TestTriageOverlayError(t *testing.T) {
	h := newHarness(t, true)
	p := reviewFile(t, threeFindings)
	h.ctl.OverlayErr = os.ErrClosed
	if code := h.env.Triage(context.Background(), []string{p}); code != ExitError {
		t.Fatalf("exit %d", code)
	}
}

func TestTriageInvalidInput(t *testing.T) {
	for name, content := range map[string]string{"garbage": "nope", "bad severity": strings.Replace(threeFindings, `"high"`, `"blocker"`, 1)} {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t, true)
			p := reviewFile(t, content)
			if code := h.env.Triage(context.Background(), []string{p}); code != ExitError {
				t.Fatalf("exit %d", code)
			}
			if len(h.ctl.Calls) != 0 || h.err.Len() == 0 {
				t.Fatalf("calls %v stderr %q", h.ctl.Calls, h.err)
			}
		})
	}
	h := newHarness(t, true)
	if code := h.env.Triage(context.Background(), []string{filepath.Join(t.TempDir(), "missing.json")}); code != ExitError {
		t.Fatalf("missing file: exit %d", code)
	}
	if code := h.env.Triage(context.Background(), nil); code != ExitError {
		t.Fatalf("no args: exit %d", code)
	}
}

func TestTriageNoFindings(t *testing.T) {
	for _, inAgterm := range []bool{true, false} {
		h := newHarness(t, inAgterm)
		p := reviewFile(t, `{"verdict":"approve","summary":"ok","findings":[],"next_steps":[]}`)
		if code := h.env.Triage(context.Background(), []string{p}); code != 0 {
			t.Fatalf("exit %d", code)
		}
		if h.out.String() != "[]\n" || len(h.ctl.Calls) != 0 {
			t.Fatalf("stdout %q calls %v", h.out, h.ctl.Calls)
		}
	}
}

func TestMainRoutesTriage(t *testing.T) {
	h := newHarness(t, true)
	p := reviewFile(t, threeFindings)
	h.ctl.Outcome = agterm.Outcome{Value: allDecided}
	if code := h.env.Main(context.Background(), []string{"triage", p}); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
}

func TestTriageOutsideAgterm(t *testing.T) {
	h := newHarness(t, false)
	p := reviewFile(t, threeFindings)
	if code := h.env.Triage(context.Background(), []string{p}); code != ExitNoAgterm {
		t.Fatalf("exit %d", code)
	}
	if len(h.ctl.Calls) != 0 {
		t.Fatalf("no agterm calls expected: %v", h.ctl.Calls)
	}
	lines := strings.Split(strings.TrimRight(h.out.String(), "\n"), "\n")
	if len(lines) != 5 || !strings.HasPrefix(lines[0], "| # |") { // header, separator, 3 rows
		t.Fatalf("table:\n%s", h.out)
	}
}
