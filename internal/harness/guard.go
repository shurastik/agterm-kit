package harness

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/shurastik/agterm-kit/internal/config"
)

// Reviewer arguments that would widen its permissions. long flags match by
// exact name or `name=value`; short flags also match with the value attached
// (`-sread-only`).
var (
	codexForbiddenLong = []string{"--sandbox", "--ask-for-approval", "--full-auto",
		"--dangerously-bypass-approvals-and-sandbox", "--yolo", "--add-dir", "--cd"}
	codexForbiddenShort = []string{"-s", "-a", "-C"}
	claudeForbidden     = []string{"--dangerously-skip-permissions", "--permission-mode", "--allowedTools",
		"--allowed-tools", "--tools", "--add-dir", "--settings", "--setting-sources", "--mcp-config"}
)

// CheckReviewerArgs refuses extra_args of a reviewer that change its sandbox,
// approvals, permission mode, allowed tools, extra directories or settings.
// agk puts its own flags after the user's tuning, which is the second line of
// defence; this is the first.
func CheckReviewerArgs(name string, args []string) error {
	switch name {
	case config.Codex:
		return checkCodexArgs(args)
	case config.Claude:
		for _, a := range args {
			if matchLong(a, claudeForbidden) {
				return refused(a)
			}
		}
	}
	return nil
}

func refused(a string) error {
	return fmt.Errorf("reviewer extra_args contain %q, which would change the reviewer's permissions; remove it", a)
}

func matchLong(a string, flags []string) bool {
	for _, f := range flags {
		if a == f || strings.HasPrefix(a, f+"=") {
			return true
		}
	}
	return false
}

func checkCodexArgs(args []string) error {
	for i, a := range args {
		if matchLong(a, codexForbiddenLong) {
			return refused(a)
		}
		for _, s := range codexForbiddenShort {
			if strings.HasPrefix(a, s) && !strings.HasPrefix(a, "--") {
				return refused(a)
			}
		}
		// -c key=value / --config key=value, also attached or with "=".
		var kv string
		switch {
		case a == "-c" || a == "--config":
			if i+1 < len(args) {
				kv = args[i+1]
			}
		case strings.HasPrefix(a, "--config="):
			kv = strings.TrimPrefix(a, "--config=")
		case strings.HasPrefix(a, "-c") && !strings.HasPrefix(a, "--"):
			kv = strings.TrimPrefix(strings.TrimPrefix(a, "-c"), "=")
		}
		if kv != "" {
			key := strings.TrimSpace(strings.SplitN(kv, "=", 2)[0])
			if strings.HasPrefix(key, "sandbox") || strings.HasPrefix(key, "approval") {
				return fmt.Errorf("reviewer extra_args set %q, which would change the reviewer's permissions; remove it", key)
			}
		}
	}
	return nil
}

// CheckRulePath refuses a path that cannot be written literally inside a
// claude permission rule such as Edit(//<path>/**): parentheses and commas
// would end or split the rule (injecting more rules into --allowedTools), and
// whitespace or glob metacharacters would be parsed, not matched literally.
func CheckRulePath(p string) error {
	if i := strings.IndexFunc(p, func(r rune) bool {
		return strings.ContainsRune("(),*?[]{}", r) || unicode.IsSpace(r) || unicode.IsControl(r)
	}); i >= 0 {
		return fmt.Errorf("path %q has %q, which cannot be written in a claude permission rule; "+
			"move the repository to a path without parentheses, commas, whitespace or glob characters", p, p[i:i+1])
	}
	return nil
}
