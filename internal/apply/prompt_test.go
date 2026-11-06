package apply

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestFirstPrompt(t *testing.T) {
	p, err := FirstPrompt("add-auth", "/repo/.agk/applies/x/rounds/1/report.json", false)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"`add-auth`",
		"openspec instructions apply --change add-auth --json",
		"/repo/.agk/applies/x/rounds/1/report.json",
		"AGENTS.md", "test", "- [x]",
	} {
		if !strings.Contains(p, want) {
			t.Errorf("first prompt lacks %q", want)
		}
	}
	if strings.Contains(p, "never commit") {
		t.Error("rules belong to the system prompt for claude")
	}
	inline, _ := FirstPrompt("add-auth", "/r.json", true)
	if !strings.HasPrefix(inline, Rules()) {
		t.Error("codex prompt must start with the rules")
	}
}

func TestFixPrompt(t *testing.T) {
	p, err := FixPrompt(2, "Finding 1: nil deref in x.go:10", "/run/rounds/2/report.json", false)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"round 2", "Finding 1: nil deref in x.go:10", "/run/rounds/2/report.json"} {
		if !strings.Contains(p, want) {
			t.Errorf("fix prompt lacks %q", want)
		}
	}
}

func TestRules(t *testing.T) {
	r := Rules()
	for _, want := range []string{"never ask", "blocked", "git commit", "git push", "deploy", "archive", "report", `"status"`, `"artifact"`} {
		if !strings.Contains(strings.ToLower(r), strings.ToLower(want)) {
			t.Errorf("rules lack %q", want)
		}
	}
}

func TestClaudeSettingsDenyHistoryRewrites(t *testing.T) {
	var s struct {
		Permissions struct {
			Deny []string `json:"deny"`
		} `json:"permissions"`
	}
	if err := json.Unmarshal(ClaudeSettings(), &s); err != nil {
		t.Fatal(err)
	}
	deny := strings.Join(s.Permissions.Deny, "\n")
	for _, want := range []string{"Bash(git commit:*)", "Bash(git push:*)", "Bash(git tag:*)", "Bash(git reset --hard:*)", "Bash(git rebase:*)", "Bash(git checkout --:*)", "Bash(gh pr:*)"} {
		if !strings.Contains(deny, want) {
			t.Errorf("deny lacks %s", want)
		}
	}
}
