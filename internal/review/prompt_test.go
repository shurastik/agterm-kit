package review

import (
	"os"
	"strings"
	"testing"
	"unicode/utf8"
)

func sampleScope() Scope {
	return Scope{
		Root: "/repo", Base: "main", MergeBase: "abc123",
		Diff: "diff --git a/x.go b/x.go\n+func Changed() {}\n",
		Stat: " x.go | 1 +\n",
		Untracked: []UntrackedFile{
			{Path: "new.go", Content: "package x\nfunc New() {}\n"},
			{Path: "blob.bin", Omitted: "binary"},
		},
	}
}

func TestPromptContainsScope(t *testing.T) {
	p, err := Prompt(sampleScope())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"main", "abc123", "x.go | 1 +",
		"+func Changed() {}",
		"new.go", "func New() {}",
		"blob.bin", "binary",
		"JSON", // output contract
	} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt lacks %q", want)
		}
	}
	if strings.Contains(p, "truncated") {
		t.Error("small diff must not be truncated")
	}
}

func TestPromptTruncatesLargeDiff(t *testing.T) {
	s := sampleScope()
	line := "+" + strings.Repeat("y", 99) + "\n"
	s.Diff = strings.Repeat(line, MaxPromptDiffBytes/len(line)+50) + "+TAIL_MARKER\n"
	s.Untracked = []UntrackedFile{{Path: "late.go", Content: "LATE_CONTENT"}}
	p, err := Prompt(s)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(p, "TAIL_MARKER") || strings.Contains(p, "LATE_CONTENT") {
		t.Fatal("content past the cap must be cut")
	}
	// The read-the-rest command must disable the same git drivers as Collect,
	// or textconv/ext-diff/submodule config could hide the part past the cap.
	if !strings.Contains(p, "truncated") || !strings.Contains(p, "git diff "+strings.Join(DiffFlags, " ")+" abc123 -- . ':(exclude).agk'") {
		t.Fatal("truncation note with the exact command to read the rest is missing")
	}
	if !strings.Contains(p, "x.go | 1 +") || !strings.Contains(p, "late.go") {
		t.Fatal("stat and untracked file names must survive truncation")
	}
	if len(p) > MaxPromptDiffBytes+16<<10 {
		t.Fatalf("prompt too large: %d", len(p))
	}
	for _, l := range strings.Split(p, "\n") {
		if strings.HasPrefix(l, "+y") && len(l) != len(line)-1 {
			t.Fatal("diff must be cut on a line boundary")
		}
	}
}

// codex rejects a prompt that is not valid UTF-8, so one legacy-encoded file
// must not sink the whole review.
func TestPromptIsValidUTF8(t *testing.T) {
	s := sampleScope()
	s.Diff = "diff --git a/w.txt b/w.txt\n+\xcf\xf0\xe8\xe2\xe5\xf2\n" // "Привет" in cp1251
	s.Untracked = []UntrackedFile{{Path: "ok.go", Content: "package x\n"}}
	p, err := Prompt(s)
	if err != nil {
		t.Fatal(err)
	}
	if !utf8.ValidString(p) {
		t.Fatal("prompt is not valid UTF-8")
	}
	if !strings.Contains(p, `+\xcf\xf0\xe8\xe2\xe5\xf2`) {
		t.Fatal("invalid bytes should stay visible as \\xNN escapes")
	}
}

// Escaping invalid bytes grows them 4x; the size cap must apply afterwards.
func TestPromptCapAppliesAfterEscaping(t *testing.T) {
	s := sampleScope()
	line := "+" + strings.Repeat("\xcf", 99) + "\n"
	s.Diff = strings.Repeat(line, MaxPromptDiffBytes/len(line)-10) // just under the cap raw
	p, err := Prompt(s)
	if err != nil {
		t.Fatal(err)
	}
	if len(p) > MaxPromptDiffBytes+16<<10 {
		t.Fatalf("prompt is %d bytes, cap %d", len(p), MaxPromptDiffBytes)
	}
	if !strings.Contains(p, "truncated") {
		t.Fatal("escaped diff over the cap must be truncated with the read-the-rest note")
	}
}

func TestPromptIsHarnessNeutral(t *testing.T) {
	p, _ := Prompt(sampleScope())
	if strings.Contains(p, "You are Codex") {
		t.Fatal("prompt must not assume codex")
	}
}

func TestRereviewPrompt(t *testing.T) {
	p, err := RereviewPrompt(sampleScope(), 3, "Fixed the nil check in x.go.")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"round 3", "earlier findings", "regression", "Fixed the nil check in x.go.", "+func Changed() {}", "JSON"} {
		if !strings.Contains(p, want) {
			t.Errorf("re-review prompt lacks %q", want)
		}
	}
	p, _ = RereviewPrompt(sampleScope(), 2, "")
	if strings.Contains(p, "<note_from_author>") {
		t.Error("no note block without a note")
	}
}

// The reviewer may have the agk skills installed; it must review, not start
// another review or touch workers.
func TestPromptForbidsDrivingAgk(t *testing.T) {
	p, _ := Prompt(sampleScope())
	if !strings.Contains(p, "Do not run `agk`") {
		t.Fatal("prompt must forbid running agk")
	}
}

func samplePlan() PlanScope {
	return PlanScope{
		Root: "/repo", Change: "add-x",
		Files: []UntrackedFile{
			{Path: "openspec/changes/add-x/proposal.md", Content: "PROPOSAL_BODY\n"},
			{Path: "openspec/changes/add-x/specs/rc/spec.md", Content: "DELTA_BODY\n"},
			{Path: "openspec/changes/add-x/design.md", Omitted: "symlink, not followed"},
		},
		MainSpecs: []UntrackedFile{{Path: "openspec/specs/rc/spec.md", Content: "MAIN_BODY\n"}},
	}
}

// between returns the text between open and the next close after it.
func between(t *testing.T, p, open, close string) string {
	t.Helper()
	i := strings.Index(p, open)
	if i < 0 {
		t.Fatalf("prompt lacks %q", open)
	}
	rest := p[i+len(open):]
	j := strings.Index(rest, close)
	if j < 0 {
		t.Fatalf("no %q after %q", close, open)
	}
	return rest[:j]
}

func TestPlanPrompt(t *testing.T) {
	p, err := PlanPrompt(samplePlan())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"`add-x`", "openspec/changes/add-x/", "not of a code diff", "JSON", "Do not run `agk`"} {
		if !strings.Contains(p, want) {
			t.Errorf("plan prompt lacks %q", want)
		}
	}
	if strings.Contains(p, "<diff>") || strings.Contains(p, "merge base") {
		t.Error("plan prompt must not be a diff review")
	}
	if !strings.Contains(between(t, p, `<change_file path="openspec/changes/add-x/proposal.md">`, "</change_file>"), "PROPOSAL_BODY") {
		t.Error("proposal not inside its change_file block")
	}
	if !strings.Contains(between(t, p, `<change_file path="openspec/changes/add-x/specs/rc/spec.md">`, "</change_file>"), "DELTA_BODY") {
		t.Error("delta not inside its change_file block")
	}
	if !strings.Contains(between(t, p, `<main_spec path="openspec/specs/rc/spec.md">`, "</main_spec>"), "MAIN_BODY") {
		t.Error("main spec not inside its main_spec block")
	}
	if !strings.Contains(p, `<change_file path="openspec/changes/add-x/design.md" omitted="symlink, not followed">`) {
		t.Error("omitted file must be listed with its reason")
	}
	// Every inlined body sits in a block the prompt declares as data.
	data := between(t, p, "<untrusted_input>", "</untrusted_input>")
	for _, tag := range []string{"<change_file>", "<main_spec>"} {
		if !strings.Contains(data, tag) {
			t.Errorf("untrusted_input does not declare %s as data", tag)
		}
	}
	if strings.Contains(p, "past the prompt size cap") {
		t.Error("small plan must not be truncated")
	}
}

func TestPlanPromptBudget(t *testing.T) {
	s := samplePlan()
	big := strings.Repeat("y", MaxPromptDiffBytes-1000)
	s.Files = []UntrackedFile{
		{Path: "openspec/changes/add-x/a.md", Content: big},
		{Path: "openspec/changes/add-x/b.md", Content: "LATE_FILE" + strings.Repeat("z", 2000)},
	}
	s.MainSpecs = []UntrackedFile{{Path: "openspec/specs/rc/spec.md", Content: "LATE_MAIN"}}
	p, err := PlanPrompt(s)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(p, "LATE_FILE") || strings.Contains(p, "LATE_MAIN") {
		t.Fatal("content past the budget must be left out")
	}
	for _, want := range []string{
		`<change_file path="openspec/changes/add-x/b.md" omitted="past the prompt size cap, read it directly">`,
		`<main_spec path="openspec/specs/rc/spec.md" omitted="past the prompt size cap, read it directly">`,
		"Read them yourself",
	} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt lacks %q", want)
		}
	}
	if len(p) > MaxPromptDiffBytes+16<<10 {
		t.Fatalf("prompt too large: %d", len(p))
	}
}

func TestPlanPromptEscapesInvalidUTF8(t *testing.T) {
	s := samplePlan()
	s.Files[0].Content = "caf\xe9\n"
	p, err := PlanPrompt(s)
	if err != nil {
		t.Fatal(err)
	}
	if !utf8.ValidString(p) || !strings.Contains(p, `caf\xe9`) {
		t.Fatal("invalid bytes must be escaped as \\xNN")
	}
}

// Escaping grows invalid bytes 4x; the budget applies afterwards.
func TestPlanPromptBudgetAfterEscaping(t *testing.T) {
	s := samplePlan()
	s.Files = []UntrackedFile{{Path: "openspec/changes/add-x/a.md", Content: strings.Repeat("\xcf", MaxPromptDiffBytes/2)}}
	p, err := PlanPrompt(s)
	if err != nil {
		t.Fatal(err)
	}
	if len(p) > MaxPromptDiffBytes+16<<10 || !strings.Contains(p, "past the prompt size cap") {
		t.Fatalf("escaped file over the budget must be left out (prompt %d bytes)", len(p))
	}
}

// Splitting the re-review wrapper out must not change the diff re-review.
func TestRereviewPromptUnchanged(t *testing.T) {
	want, err := os.ReadFile("testdata/rereview-diff.golden")
	if err != nil {
		t.Fatal(err)
	}
	p, err := RereviewPrompt(sampleScope(), 3, "Fixed the nil check in x.go.\n")
	if err != nil {
		t.Fatal(err)
	}
	if p != string(want) {
		t.Fatalf("diff re-review prompt changed:\n%s", p)
	}
}

func TestRereviewWrapsPlanPrompt(t *testing.T) {
	body, _ := PlanPrompt(samplePlan())
	p, err := Rereview(body, 2, "Edited tasks.md.")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"round 2", "earlier findings", "Edited tasks.md.", "PROPOSAL_BODY"} {
		if !strings.Contains(p, want) {
			t.Errorf("plan re-review prompt lacks %q", want)
		}
	}
	if !strings.HasSuffix(p, body) {
		t.Error("the wrapper must end with the rendered body")
	}
}
