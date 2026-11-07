package cli

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/shurastik/agterm-kit/internal/agterm"
	"github.com/shurastik/agterm-kit/internal/agterm/agtermtest"
	"github.com/shurastik/agterm-kit/internal/config"
	"github.com/shurastik/agterm-kit/internal/review"
	"github.com/shurastik/agterm-kit/internal/rundir"
)

// sendDir is a codex review run whose round 1 finished, with companion C-1 in pane P-1.
func sendDir(t *testing.T, h *tenv, session string, companion bool) (string, string) {
	t.Helper()
	root, dir := runDirWith(t, config.Worker{Harness: "codex"}, session)
	writeResultFile(t, dir, completedResult)
	if companion {
		os.WriteFile(filepath.Join(dir, CompanionFile), []byte("C-1\n"), 0o644)
		os.WriteFile(filepath.Join(dir, paneFile), []byte("P-1\n"), 0o644)
	}
	scriptScope(h, root)
	return root, dir
}

func runSend(h *tenv, args ...string) int {
	return h.env.Main(context.Background(), append([]string{"review", "send"}, args...))
}

func TestReviewSendSecondRound(t *testing.T) {
	h := newHarness(t, true)
	root, dir := sendDir(t, h, "T-1", true)
	seen := scriptScope(h, root)
	note := filepath.Join(t.TempDir(), "fixed.md")
	os.WriteFile(note, []byte("Fixed the nil check."), 0o644)
	h.ctl.WaitStatus = "active"

	if code := runSend(h, dir, "--note", note); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	if !reflect.DeepEqual(*seen, []string{root, "main"}) {
		t.Fatalf("collect = %q", *seen)
	}
	p, err := os.ReadFile(filepath.Join(dir, "rounds", "2", "prompt.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"round 2", "earlier findings", "Fixed the nil check.", "+x"} {
		if !strings.Contains(string(p), want) {
			t.Errorf("round 2 prompt lacks %q", want)
		}
	}
	if k, _ := rundir.Round(dir); k != 2 {
		t.Fatalf("round = %d", k)
	}
	assertCalls(t, h.ctl,
		agtermtest.Call{Method: "MsgSend", Args: []string{"C-1:P-1", "round 2"}},
		agtermtest.Call{Method: "Wait", Args: []string{"C-1", "active", "30s"}},
	)
	if want := `{"companion":"C-1","dir":"` + dir + `","round":2}` + "\n"; h.out.String() != want {
		t.Fatalf("stdout = %q", h.out)
	}
}

func TestReviewSendWithoutSessionUsesFirstRoundPrompt(t *testing.T) {
	h := newHarness(t, true)
	_, dir := sendDir(t, h, "", true)
	h.ctl.WaitStatus = "active"
	if code := runSend(h, dir); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	p, _ := os.ReadFile(filepath.Join(dir, "rounds", "2", "prompt.md"))
	first, _ := review.Prompt(review.Scope{Base: "main", MergeBase: "mb", Diff: "+x\n", Stat: " f | 1 +\n"})
	if string(p) != first {
		t.Fatalf("prompt = %q", p)
	}
}

func TestReviewSendRefusals(t *testing.T) {
	cases := map[string]struct {
		prep func(t *testing.T, h *tenv, dir string)
		want string
	}{
		"still running": {func(t *testing.T, h *tenv, dir string) { os.Remove(filepath.Join(dir, "rounds", "1", "result.json")) }, "still running"},
		"nothing to review": {func(t *testing.T, h *tenv, dir string) {
			h.env.Collect = func(context.Context, string, string) (review.Scope, error) {
				return review.Scope{}, review.ErrNothingToReview
			}
		}, "nothing to review"},
		"pane unknown":  {func(t *testing.T, h *tenv, dir string) { os.Remove(filepath.Join(dir, paneFile)) }, "pane"},
		"reviewer gone": {func(t *testing.T, h *tenv, dir string) { h.ctl.MsgSendErr = agterm.ErrGone }, "gone"},
		"send error":    {func(t *testing.T, h *tenv, dir string) { h.ctl.MsgSendErr = errors.New("socket refused") }, "socket refused"},
		"missing note":  {func(t *testing.T, h *tenv, dir string) {}, "no-such-note"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t, true)
			_, dir := sendDir(t, h, "T-1", true)
			c.prep(t, h, dir)
			args := []string{dir}
			if name == "missing note" {
				args = append(args, "--note", filepath.Join(t.TempDir(), "no-such-note"))
			}
			if code := runSend(h, args...); code != 1 {
				t.Fatalf("exit %d, want 1 (stderr %q)", code, h.err)
			}
			if !strings.Contains(h.err.String(), c.want) || h.out.Len() != 0 {
				t.Fatalf("stderr %q stdout %q", h.err, h.out)
			}
			if k, _ := rundir.Round(dir); k != 1 || exists(filepath.Join(dir, "rounds", "2")) {
				t.Fatalf("no round may be recorded: round %d", k)
			}
		})
	}
}

func TestReviewSendHandshakeTimeoutWarns(t *testing.T) {
	h := newHarness(t, true)
	_, dir := sendDir(t, h, "T-1", true)
	h.ctl.WaitErr = agterm.ErrTimeout
	if code := runSend(h, dir); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	if !strings.Contains(h.err.String(), "warning") || !strings.Contains(h.out.String(), `"round":2`) {
		t.Fatalf("stderr %q stdout %q", h.err, h.out)
	}
}

func TestReviewSendInline(t *testing.T) {
	h := newHarness(t, false)
	root, dir := sendDir(t, h, "T-1", false)
	h.drivers["codex"].outputs = []string{approve}
	if code := runSend(h, dir); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	tr := h.drivers["codex"].turns
	if len(tr) != 1 || !tr[0].Resume || tr[0].Session != "T-1" || tr[0].Root != root || !strings.Contains(tr[0].Prompt, "round 2") {
		t.Fatalf("turns = %+v", tr)
	}
	if want := `{"companion":null,"dir":"` + dir + `","round":2}` + "\n"; h.out.String() != want {
		t.Fatalf("stdout = %q", h.out)
	}
	if len(h.ctl.Calls) != 0 {
		t.Fatalf("no agterm calls: %v", h.ctl.Calls)
	}
	assertResult(t, filepath.Join(dir, "rounds", "2"), "completed", 0)
}

func runClose(h *tenv, args ...string) int {
	return h.env.Main(context.Background(), append([]string{"review", "close"}, args...))
}

func TestReviewClose(t *testing.T) {
	h := newHarness(t, true)
	_, dir := sendDir(t, h, "T-1", true)
	if code := runClose(h, dir); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	assertCalls(t, h.ctl, agtermtest.Call{Method: "Close", Args: []string{"C-1"}})

	h = newHarness(t, true)
	h.ctl.CloseErr = errors.Join(agterm.ErrGone, errors.New("no such session"))
	if code := runClose(h, dir); code != 0 || h.err.Len() != 0 {
		t.Fatalf("gone: exit %d, %q", code, h.err)
	}

	h = newHarness(t, true)
	_, inline := sendDir(t, h, "", false)
	if code := runClose(h, inline); code != 0 || len(h.ctl.Calls) != 0 {
		t.Fatalf("no companion: exit %d, calls %v", code, h.ctl.Calls)
	}

	h = newHarness(t, true)
	h.ctl.CloseErr = errors.New("socket refused")
	if code := runClose(h, dir); code != 1 || !strings.Contains(h.err.String(), "socket refused") {
		t.Fatalf("error: exit %d, %q", code, h.err)
	}
	if code := runClose(h, filepath.Dir(dir)); code != 1 {
		t.Fatalf("bad dir: exit %d", code)
	}
}

// Run dirs written before plan reviews have no change field: they load as
// diff reviews.
func TestLoadReviewRunWithoutChange(t *testing.T) {
	_, dir := runDir(t)
	if err := os.WriteFile(filepath.Join(dir, workerFile), []byte(`{"worker":{"harness":"codex"},"base":"main","session":"T-1"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run, err := loadReviewRun(dir)
	if err != nil {
		t.Fatal(err)
	}
	if run.state.Change != "" || run.state.Base != "main" || run.state.Session != "T-1" {
		t.Fatalf("state = %+v", run.state)
	}
}

func TestReviewStateRecordsChangeWithoutBase(t *testing.T) {
	_, dir := runDir(t)
	if err := rundir.WriteJSON(dir, workerFile, reviewState{Worker: config.Worker{Harness: "codex"}, Change: "add-x"}); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, workerFile))
	var raw map[string]any
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatal(err)
	}
	if _, hasBase := raw["base"]; raw["change"] != "add-x" || hasBase {
		t.Fatalf("worker.json = %s", b)
	}
	b2, _ := json.Marshal(reviewState{Worker: config.Worker{Harness: "codex"}, Base: "main"})
	if strings.Contains(string(b2), `"change"`) {
		t.Fatalf("diff run must not record a change: %s", b2)
	}
}

// planSendDir is sendDir for a plan review of add-x whose CollectPlan returns
// tasks.md with body; it records CollectPlan's arguments.
func planSendDir(t *testing.T, h *tenv, session string, companion bool, body string) (string, string, *[]string) {
	t.Helper()
	root, dir := sendDir(t, h, session, companion)
	if err := rundir.WriteJSON(dir, workerFile, reviewState{Worker: config.Worker{Harness: "codex"}, Change: "add-x", Session: session}); err != nil {
		t.Fatal(err)
	}
	var seen []string
	h.env.CollectPlan = func(_ context.Context, d, change string) (review.PlanScope, error) {
		seen = append(seen, d, change)
		return review.PlanScope{Root: root, Change: change, Files: []review.UntrackedFile{
			{Path: "openspec/changes/add-x/tasks.md", Content: body},
		}}, nil
	}
	h.env.Collect = func(context.Context, string, string) (review.Scope, error) {
		return review.Scope{}, errors.New("diff scope must not be collected for a plan review")
	}
	return root, dir, &seen
}

func TestReviewSendPlanRereview(t *testing.T) {
	h := newHarness(t, true)
	root, dir, seen := planSendDir(t, h, "T-1", true, "EDITED_TASKS\n")
	note := filepath.Join(t.TempDir(), "fixed.md")
	os.WriteFile(note, []byte("Added the missing task."), 0o644)
	h.ctl.WaitStatus = "active"
	if code := runSend(h, dir, "--note", note); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	if !reflect.DeepEqual(*seen, []string{root, "add-x"}) {
		t.Fatalf("collect plan = %q", *seen)
	}
	p, _ := os.ReadFile(filepath.Join(dir, "rounds", "2", "prompt.md"))
	for _, want := range []string{"round 2", "earlier findings", "Added the missing task.", "EDITED_TASKS", "`add-x`"} {
		if !strings.Contains(string(p), want) {
			t.Errorf("round 2 prompt lacks %q", want)
		}
	}
	if s := readState(t, dir); s.Change != "add-x" || s.Base != "" {
		t.Fatalf("worker.json = %+v", s)
	}
}

func TestReviewSendPlanInlineResumesSession(t *testing.T) {
	h := newHarness(t, false)
	_, dir, _ := planSendDir(t, h, "T-1", false, "EDITED_TASKS\n")
	h.drivers["codex"].outputs = []string{approve}
	if code := runSend(h, dir); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	tr := h.drivers["codex"].turns
	if len(tr) != 1 || !tr[0].Resume || tr[0].Session != "T-1" || !strings.Contains(tr[0].Prompt, "EDITED_TASKS") {
		t.Fatalf("turns = %+v", tr)
	}
}

func TestReviewSendPlanWithoutSessionUsesPlanPrompt(t *testing.T) {
	h := newHarness(t, true)
	root, dir, _ := planSendDir(t, h, "", true, "TASKS\n")
	h.ctl.WaitStatus = "active"
	if code := runSend(h, dir); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	p, _ := os.ReadFile(filepath.Join(dir, "rounds", "2", "prompt.md"))
	first, _ := review.PlanPrompt(review.PlanScope{Root: root, Change: "add-x", Files: []review.UntrackedFile{
		{Path: "openspec/changes/add-x/tasks.md", Content: "TASKS\n"},
	}})
	if string(p) != first {
		t.Fatalf("prompt = %q", p)
	}
}

func TestReviewSendPlanChangeGone(t *testing.T) {
	h := newHarness(t, true)
	_, dir, _ := planSendDir(t, h, "T-1", true, "")
	h.env.CollectPlan = func(context.Context, string, string) (review.PlanScope, error) {
		return review.PlanScope{}, errors.New(`change "add-x" not found: no openspec/changes/add-x`)
	}
	if code := runSend(h, dir); code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
	if !strings.Contains(h.err.String(), `"add-x" not found`) || h.out.Len() != 0 {
		t.Fatalf("stderr %q stdout %q", h.err, h.out)
	}
	if k, _ := rundir.Round(dir); k != 1 || exists(filepath.Join(dir, "rounds", "2")) {
		t.Fatalf("no round may be recorded: round %d", k)
	}
	if len(h.ctl.Calls) != 0 {
		t.Fatalf("no agterm calls: %v", h.ctl.Calls)
	}
}

func TestReviewSendDiffRunDoesNotCollectPlan(t *testing.T) {
	h := newHarness(t, true)
	_, dir := sendDir(t, h, "T-1", true)
	h.env.CollectPlan = func(context.Context, string, string) (review.PlanScope, error) {
		t.Fatal("a diff run must not collect a plan scope")
		return review.PlanScope{}, nil
	}
	h.ctl.WaitStatus = "active"
	if code := runSend(h, dir); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
}

// legacyRun rewrites dir's worker.json as written before plan reviews: a
// base, no change field.
func legacyRun(t *testing.T, dir string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, workerFile), []byte(`{"worker":{"harness":"codex"},"base":"develop","session":"T-1"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLegacyRunIsSentAsDiffReview(t *testing.T) {
	h := newHarness(t, true)
	root, dir := sendDir(t, h, "T-1", true)
	legacyRun(t, dir)
	seen := scriptScope(h, root)
	h.env.CollectPlan = func(context.Context, string, string) (review.PlanScope, error) {
		t.Fatal("a legacy run must not collect a plan scope")
		return review.PlanScope{}, nil
	}
	h.ctl.WaitStatus = "active"
	if code := runSend(h, dir); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	if !reflect.DeepEqual(*seen, []string{root, "develop"}) {
		t.Fatalf("collect = %q, want the recorded base", *seen)
	}
	p, _ := os.ReadFile(filepath.Join(dir, "rounds", "2", "prompt.md"))
	if !strings.Contains(string(p), "+x") || !strings.Contains(string(p), "round 2") {
		t.Fatalf("round 2 prompt = %q", p)
	}
}

func TestLegacyRunIsWaitedOn(t *testing.T) {
	h := newHarness(t, true)
	_, dir := sendDir(t, h, "T-1", false)
	legacyRun(t, dir)
	if code := runWait(h, dir); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	if !strings.Contains(h.out.String(), `"status":"completed"`) {
		t.Fatalf("stdout = %q", h.out)
	}
}

// A plan re-review reads the change from disk: committed files on a clean
// tree (an empty git diff) still make round 2.
func TestReviewSendPlanCommittedCleanTree(t *testing.T) {
	root, _ := filepath.EvalSymlinks(t.TempDir())
	gitCmd := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@t", "-c", "commit.gpgsign=false", "-c", "core.hooksPath=/dev/null"}, args...)...)
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	gitCmd("init", "-q", "-b", "main")
	os.MkdirAll(filepath.Join(root, "openspec/changes/add-x"), 0o755)
	os.WriteFile(filepath.Join(root, "openspec/changes/add-x/tasks.md"), []byte("- [ ] COMMITTED_TASK\n"), 0o644)
	os.WriteFile(filepath.Join(root, ".gitignore"), []byte(".agk/\n"), 0o644)
	gitCmd("add", "-A")
	gitCmd("commit", "-qm", "change")

	h := newHarness(t, false)
	h.env.CollectPlan = review.CollectPlan
	h.env.Collect = review.Collect
	dir := filepath.Join(root, ".agk", "reviews", "20261105-100000")
	os.MkdirAll(rundir.RoundDir(dir, 1), 0o755)
	rundir.WriteJSON(dir, workerFile, reviewState{Worker: config.Worker{Harness: "codex"}, Change: "add-x", Session: "T-1"})
	rundir.SetRound(dir, 1)
	writeResultFile(t, dir, completedResult)
	h.drivers["codex"].outputs = []string{approve}

	if code := runSend(h, dir); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	p, _ := os.ReadFile(filepath.Join(dir, "rounds", "2", "prompt.md"))
	if !strings.Contains(string(p), "COMMITTED_TASK") || !strings.Contains(string(p), "round 2") {
		t.Fatalf("round 2 prompt lacks the committed change:\n%s", p)
	}
}
