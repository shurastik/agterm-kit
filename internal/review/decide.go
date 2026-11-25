package review

import (
	"errors"

	"github.com/shurastik/agterm-kit/internal/agterm"
)

// Kind is what `agk review wait` does next.
type Kind string

const (
	Completed     Kind = "record-completed" // write review.json and result.json
	RecordBlocked Kind = "record-blocked"   // write result.json blocked with Reason
	Fixup         Kind = "type-fixup"       // ask the reviewer to rewrite its output
	NeedsHuman    Kind = "needs-human"      // report without recording an outcome
	Cancelled     Kind = "cancelled"        // the reviewer's companion is gone
	Running       Kind = "running"          // the timeout passed first
	KeepWaiting   Kind = "keep-waiting"     // nothing decided yet
)

// Reasons of a needs-human outcome.
const (
	ReasonNoOutput   = "no-output"
	ReasonPermission = "permission"
)

// MaxFixups is how many fix-up instructions a round gets before it fails.
const MaxFixups = 2

// Action is a decision of Decide.
type Action struct {
	Kind   Kind
	Reason string // NeedsHuman: ReasonNoOutput | ReasonPermission; RecordBlocked: ReasonInvalidOutput
	Review Review // Completed: the parsed output
	Err    error  // Fixup, RecordBlocked: why the output is invalid
}

// Decide maps round k's output file (output, exists), the reviewer
// companion's agent status, the wait error (nil, agterm.ErrGone or
// agterm.ErrTimeout), the fix-ups typed so far and whether the latest of them
// has started to one Action. settled says that a codex reviewer's screen has
// settled with its output on disk (see the cli settler): it counts as a turn
// end next to a completed or idle status, for a reviewer whose status never
// leaves active, and overrides none of the other rules.
//
// A valid output always completes the round, whatever the status or the
// fix-up gate. While a typed fix-up has not started, the status and the file
// may still belong to the turn before it, so only a permission prompt, a gone
// companion or the timeout end the wait and everything else keeps waiting;
// that keeps a second fix-up from being typed into a running turn.
func Decide(k int, output []byte, exists bool, status string, waitErr error, fixups int, fixupStarted bool, settled bool) Action {
	var perr error
	if exists {
		r, err := Parse(output)
		if err == nil {
			return Action{Kind: Completed, Review: r}
		}
		perr = err
	}
	gone, timeout := errors.Is(waitErr, agterm.ErrGone), errors.Is(waitErr, agterm.ErrTimeout)
	switch {
	case gone:
		return Action{Kind: Cancelled}
	case timeout:
		return Action{Kind: Running}
	case status == "blocked":
		return Action{Kind: NeedsHuman, Reason: ReasonPermission}
	case fixups > 0 && !fixupStarted:
		return Action{Kind: KeepWaiting}
	}
	turnOver := status == "completed" || status == "idle" || settled
	switch {
	case !turnOver:
		return Action{Kind: KeepWaiting}
	case !exists:
		return Action{Kind: NeedsHuman, Reason: ReasonNoOutput}
	case fixups < MaxFixups:
		return Action{Kind: Fixup, Err: perr}
	}
	return Action{Kind: RecordBlocked, Reason: ReasonInvalidOutput, Err: perr}
}
