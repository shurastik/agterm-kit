package cli

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/shurastik/agterm-kit/internal/apply"
	"github.com/shurastik/agterm-kit/internal/fsx"
	"github.com/shurastik/agterm-kit/internal/rundir"
)

// hookInputLimit caps what a turn-state hook reads from stdin.
const hookInputLimit = 1 << 20

// Events of the hidden `agk apply hook`.
const (
	hookTurn = "turn" // UserPromptSubmit: a turn starts
	hookStop = "stop" // Stop: a turn ends
)

// applierSettings is the settings layer of a claude applier: the deny rules of
// the asset plus the hooks that record the turn state. Claude merges hook
// arrays across settings sources, so the user's agterm status hooks keep
// running. exe and dir are quoted for the shell; a path with a backslash or a
// line break is refused.
func applierSettings(exe, dir string) ([]byte, error) {
	qe, err := shellQuote(exe)
	if err != nil {
		return nil, err
	}
	qd, err := shellQuote(dir)
	if err != nil {
		return nil, err
	}
	var s map[string]any
	if err := json.Unmarshal(apply.ClaudeSettings(), &s); err != nil {
		return nil, err
	}
	hook := func(event string) []hookEntry {
		return []hookEntry{{Hooks: []hookCommand{{Type: "command", Command: qe + " apply hook " + event + " " + qd}}}}
	}
	s["hooks"] = map[string][]hookEntry{
		"UserPromptSubmit": hook(hookTurn),
		"Stop":             hook(hookStop),
		// A final API error (rate limit, authentication) ends the turn with
		// StopFailure instead of Stop: without it the state would stay running.
		"StopFailure": hook(hookStop),
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

// ApplyHook implements the hidden `agk apply hook <turn|stop> <run-dir>`: it
// records the claude applier's turn state in <run-dir>/turn.json from the hook
// input on stdin. It never sets the agent status. Whatever goes wrong it
// writes nothing, prints nothing and exits 0: UserPromptSubmit stdout would
// land in the applier's context, Stop stdout is parsed as a decision, and a
// failing hook must not stop the applier.
func (e *Env) ApplyHook(_ context.Context, args []string) int {
	if len(args) != 2 || (args[0] != hookTurn && args[0] != hookStop) {
		return ExitOK
	}
	dir := args[1]
	if _, err := rundir.Root(dir, rundir.Applies); err != nil {
		return ExitOK
	}
	input, err := io.ReadAll(io.LimitReader(e.Stdin, hookInputLimit))
	if err != nil {
		return ExitOK
	}
	prev, havePrev := readTurn(dir)
	var next apply.Turn
	switch args[0] {
	case hookTurn:
		k, ok := kickoffRound(dir, input)
		switch {
		case ok:
		case havePrev:
			k = prev.Round
		default:
			return ExitOK
		}
		next = apply.Turn{Round: k, State: apply.TurnRunning}
	case hookStop:
		if !havePrev {
			return ExitOK
		}
		if next, err = apply.TurnFromStop(prev.Round, input); err != nil {
			return ExitOK
		}
	}
	_ = fsx.ReplaceFile(filepath.Join(dir, apply.TurnFile), next.Marshal(), 0o644)
	return ExitOK
}

// readTurn returns the run's recorded turn state; ok is false when there is
// none or it is not valid.
func readTurn(dir string) (apply.Turn, bool) {
	b, err := os.ReadFile(filepath.Join(dir, apply.TurnFile))
	if err != nil {
		return apply.Turn{}, false
	}
	t, err := apply.ParseTurn(b)
	return t, err == nil
}

// kickoffRound reads the prompt of a UserPromptSubmit input and returns k when
// it is, trimmed, exactly the kickoff line of <dir>/rounds/<k>/prompt.md. The
// round is bound to what the applier actually received, not to the run's
// current round, which `agk apply send` changes from outside.
func kickoffRound(dir string, input []byte) (int, bool) {
	var in struct {
		Prompt string `json:"prompt"`
	}
	if json.Unmarshal(input, &in) != nil {
		return 0, false
	}
	line := strings.TrimSpace(in.Prompt)
	sep := string(filepath.Separator)
	rest, ok := strings.CutPrefix(line, "Read "+filepath.Join(dir, "rounds")+sep)
	if !ok {
		return 0, false
	}
	num, _, found := strings.Cut(rest, sep)
	if !found {
		return 0, false
	}
	k, err := strconv.Atoi(num)
	if err != nil || k < 1 || strconv.Itoa(k) != num {
		return 0, false
	}
	if line != kickoff(filepath.Join(rundir.RoundDir(dir, k), "prompt.md")) {
		return 0, false
	}
	return k, true
}
