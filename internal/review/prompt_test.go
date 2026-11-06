package review

import (
	"strings"
	"testing"
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
