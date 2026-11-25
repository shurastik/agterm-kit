package cli

import (
	"context"
	"fmt"
)

const autopilotUsage = `Usage:
  agk autopilot on|off   Mark (on) or unmark (off) the calling agterm session as run by the autopilot
`

// autopilotResult is the stdout of `agk autopilot on|off`: the session the
// command acted on (null outside agterm) and whether its autopilot line is now
// set.
type autopilotResult struct {
	Session *string `json:"session"`
	Marked  bool    `json:"marked"`
}

// Autopilot implements `agk autopilot on|off`. Every agterm failure is a
// warning: the mark is cosmetic and must not stop an unattended run.
func (e *Env) Autopilot(ctx context.Context, args []string) int {
	if len(args) == 1 && (args[0] == "-h" || args[0] == "--help") {
		fmt.Fprint(e.Stdout, autopilotUsage)
		return ExitOK
	}
	if len(args) != 1 || (args[0] != "on" && args[0] != "off") {
		fmt.Fprint(e.Stderr, autopilotUsage)
		return ExitError
	}
	session := e.session()
	if session == "" {
		return e.printJSON(autopilotResult{})
	}
	marked := false
	switch {
	case !safeSessionID.MatchString(session):
		e.autopilotWarn(args[0], session, fmt.Errorf("session id %q cannot name a state file", session))
	case args[0] == "on":
		marked = e.autopilotOn(ctx, session)
	default:
		e.autopilotOff(ctx, session)
	}
	return e.printJSON(autopilotResult{Session: &session, Marked: marked})
}
