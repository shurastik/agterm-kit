package review

import (
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
