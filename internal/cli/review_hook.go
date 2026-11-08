package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"

	"github.com/shurastik/agterm-kit/internal/fsx"
)

// reviewerSettingsFile is the claude reviewer's settings in the run directory.
const reviewerSettingsFile = "reviewer-settings.json"

// hookStates are the agent states `agk review hook` sets.
var hookStates = []string{"active", "completed", "blocked"}

type hookCommand struct {
	Type    string `json:"type"`
	Command string `json:"command"`
}

type hookEntry struct {
	Matcher string        `json:"matcher,omitempty"`
	Hooks   []hookCommand `json:"hooks"`
}

// reviewerSettings is the settings layer of a claude reviewer. `--restricted`
// drops the user's settings and with them agterm's agent-status hooks, so
// agk's own hooks set the companion's status: active when a turn starts,
// completed when it ends and blocked on a permission request.
func reviewerSettings(exe string) ([]byte, error) {
	q, err := shellQuote(exe)
	if err != nil {
		return nil, err
	}
	hook := func(state string) []hookCommand {
		return []hookCommand{{Type: "command", Command: q + " review hook " + state}}
	}
	s := map[string]any{"hooks": map[string][]hookEntry{
		"UserPromptSubmit": {{Hooks: hook("active")}},
		"Stop":             {{Hooks: hook("completed")}},
		"Notification":     {{Matcher: "permission_prompt", Hooks: hook("blocked")}},
	}}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

// writeReviewerSettings writes <run>/reviewer-settings.json for the resolved
// agk executable and returns its path.
func (e *Env) writeReviewerSettings(dir string) (string, error) {
	exe, err := e.Executable()
	if err != nil {
		return "", fmt.Errorf("locate agk binary: %v", err)
	}
	b, err := reviewerSettings(exe)
	if err != nil {
		return "", err
	}
	p := filepath.Join(dir, reviewerSettingsFile)
	return p, fsx.ReplaceFile(p, b, 0o644)
}

// ReviewHook implements the hidden `agk review hook <active|completed|blocked>`:
// it sets the agent status of the caller's own agterm session. Claude runs it
// from the reviewer's hooks. Outside agterm it does nothing.
func (e *Env) ReviewHook(ctx context.Context, args []string) int {
	if len(args) != 1 || !slices.Contains(hookStates, args[0]) {
		return e.errorf("usage: agk review hook <active|completed|blocked>")
	}
	target := e.Getenv("AGTERM_SESSION_ID")
	if target == "" {
		return ExitOK
	}
	if err := e.Ctl.Status(ctx, target, args[0]); err != nil {
		return e.errorf("set status %s: %v", args[0], err)
	}
	return ExitOK
}

// checkReviewerSettings checks that the hook command can be built for the
// resolved agk binary, without writing anything.
func (e *Env) checkReviewerSettings() error {
	exe, err := e.Executable()
	if err != nil {
		return fmt.Errorf("locate agk binary: %v", err)
	}
	_, err = reviewerSettings(exe)
	return err
}
