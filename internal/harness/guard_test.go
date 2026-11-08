package harness

import (
	"strings"
	"testing"

	"github.com/shurastik/agterm-kit/internal/config"
)

func TestCheckReviewerArgsRefusesWidening(t *testing.T) {
	cases := []struct {
		harness string
		args    []string
		named   string // what the error must name
	}{
		{config.Codex, []string{"--sandbox", "danger-full-access"}, "--sandbox"},
		{config.Codex, []string{"--sandbox=danger-full-access"}, "--sandbox=danger-full-access"},
		{config.Codex, []string{"-s", "danger-full-access"}, "-s"},
		{config.Codex, []string{"-sdanger-full-access"}, "-sdanger-full-access"},
		{config.Codex, []string{"--ask-for-approval", "on-request"}, "--ask-for-approval"},
		{config.Codex, []string{"--ask-for-approval=never"}, "--ask-for-approval=never"},
		{config.Codex, []string{"-a", "never"}, "-a"},
		{config.Codex, []string{"--full-auto"}, "--full-auto"},
		{config.Codex, []string{"--dangerously-bypass-approvals-and-sandbox"}, "--dangerously-bypass-approvals-and-sandbox"},
		{config.Codex, []string{"--yolo"}, "--yolo"},
		{config.Codex, []string{"--add-dir", "/x"}, "--add-dir"},
		{config.Codex, []string{"--add-dir=/x"}, "--add-dir=/x"},
		{config.Codex, []string{"-C", "/x"}, "-C"},
		{config.Codex, []string{"--cd", "/x"}, "--cd"},
		{config.Codex, []string{"--cd=/x"}, "--cd=/x"},
		{config.Codex, []string{"-c", "sandbox_mode=\"danger-full-access\""}, "sandbox_mode"},
		{config.Codex, []string{"--config", "approval_policy=\"never\""}, "approval_policy"},
		{config.Codex, []string{"--config=sandbox_workspace_write.writable_roots=[\"/\"]"}, "sandbox_workspace_write.writable_roots"},
		{config.Codex, []string{"-c=approval_policy=never"}, "approval_policy"},
		{config.Codex, []string{"-capproval_policy=never"}, "approval_policy"},
		{config.Codex, []string{"-c", " sandbox_mode = \"x\""}, "sandbox_mode"},
		{config.Claude, []string{"--dangerously-skip-permissions"}, "--dangerously-skip-permissions"},
		{config.Claude, []string{"--permission-mode", "bypassPermissions"}, "--permission-mode"},
		{config.Claude, []string{"--permission-mode=acceptEdits"}, "--permission-mode=acceptEdits"},
		{config.Claude, []string{"--allowedTools", "Bash"}, "--allowedTools"},
		{config.Claude, []string{"--allowed-tools=Bash"}, "--allowed-tools=Bash"},
		{config.Claude, []string{"--tools", "Bash,Edit"}, "--tools"},
		{config.Claude, []string{"--add-dir", "/x"}, "--add-dir"},
		{config.Claude, []string{"--settings", "/x.json"}, "--settings"},
		{config.Claude, []string{"--setting-sources", "user"}, "--setting-sources"},
		{config.Claude, []string{"--mcp-config", "/x.json"}, "--mcp-config"},
	}
	for _, c := range cases {
		t.Run(c.harness+" "+strings.Join(c.args, " "), func(t *testing.T) {
			err := CheckReviewerArgs(c.harness, c.args)
			if err == nil || !strings.Contains(err.Error(), c.named) {
				t.Fatalf("err = %v, want it to name %q", err, c.named)
			}
		})
	}
}

func TestCheckReviewerArgsAllowsTuning(t *testing.T) {
	cases := []struct {
		harness string
		args    []string
	}{
		{config.Codex, nil},
		{config.Codex, []string{"-c", `model_verbosity="low"`}},
		{config.Codex, []string{"--config", `model_verbosity="low"`}},
		{config.Codex, []string{"--search", "--profile", "review", "-c", "tui.notifications=false"}},
		{config.Claude, []string{"--max-turns", "9", "--model", "x"}},
		// The sandbox is codex's; claude has no such flag, and the other way round.
		{config.Claude, []string{"--sandbox"}},
		{config.Codex, []string{"--permission-mode", "plan"}},
	}
	for _, c := range cases {
		if err := CheckReviewerArgs(c.harness, c.args); err != nil {
			t.Errorf("%s %q: %v", c.harness, c.args, err)
		}
	}
}

func TestCheckRulePath(t *testing.T) {
	for _, p := range []string{"/Users/a/proj", "/tmp/it's/proj-1_x.y", "/tmp/a=b+c@d:e"} {
		if err := CheckRulePath(p); err != nil {
			t.Errorf("%q: %v", p, err)
		}
	}
	for _, p := range []string{
		"/tmp/review),Bash,Read(x", "/tmp/a(b", "/tmp/a)b", "/tmp/a,b", "/tmp/a b", "/tmp/a\tb",
		"/tmp/a*b", "/tmp/a?b", "/tmp/a[b", "/tmp/a]b", "/tmp/a{b", "/tmp/a}b", "/tmp/a\nb",
	} {
		if err := CheckRulePath(p); err == nil {
			t.Errorf("%q: want an error", p)
		}
	}
}
