package apply

import (
	"encoding/json"
	"errors"
	"fmt"
)

// TurnFile is the run-level turn state of a claude applier: the hooks write
// it, `agk apply wait` reads it.
const TurnFile = "turn.json"

// Turn states.
const (
	TurnRunning = "running" // a turn is on
	TurnStopped = "stopped" // the turn ended; Background and Wakeups say what is still in flight
)

// Turn is the applier's turn state with the round the turn belongs to. It
// holds counts only, never model text.
type Turn struct {
	Round      int    `json:"round"`
	State      string `json:"state"`
	Background int    `json:"background,omitempty"`
	Wakeups    int    `json:"wakeups,omitempty"`
}

// InFlight is the number of background tasks and scheduled wakeups still
// pending when the turn ended.
func (t Turn) InFlight() int { return t.Background + t.Wakeups }

// Paused reports a turn that ended with work in flight: the applier is
// waiting for something to wake it up, not done.
func (t Turn) Paused() bool { return t.State == TurnStopped && t.InFlight() > 0 }

// Marshal is the file content of t.
func (t Turn) Marshal() []byte {
	b, _ := json.Marshal(t) // plain struct of ints and strings
	return append(b, '\n')
}

// ParseTurn validates the content of turn.json.
func ParseTurn(b []byte) (Turn, error) {
	var t Turn
	if err := json.Unmarshal(b, &t); err != nil {
		return Turn{}, fmt.Errorf("turn state is not a JSON object: %v", err)
	}
	switch {
	case t.Round < 1:
		return Turn{}, fmt.Errorf("turn state round %d is not a round number", t.Round)
	case t.State != TurnRunning && t.State != TurnStopped:
		return Turn{}, fmt.Errorf("turn state %q is not %s or %s", t.State, TurnRunning, TurnStopped)
	case t.Background < 0 || t.Wakeups < 0:
		return Turn{}, errors.New("turn state has a negative count")
	}
	return t, nil
}

// stopInput is the part of a claude `Stop` hook input that matters here: what
// is still in flight when the turn ends. Entries are counted, never read.
type stopInput struct {
	BackgroundTasks []json.RawMessage `json:"background_tasks"`
	SessionCrons    []json.RawMessage `json:"session_crons"`
}

// TurnFromStop builds round k's stopped turn state from a `Stop` hook input.
// A missing field counts as empty, so an older Claude Code reads as nothing
// in flight.
func TurnFromStop(k int, input []byte) (Turn, error) {
	var in stopInput
	if err := json.Unmarshal(input, &in); err != nil {
		return Turn{}, fmt.Errorf("stop hook input: %v", err)
	}
	return Turn{Round: k, State: TurnStopped, Background: len(in.BackgroundTasks), Wakeups: len(in.SessionCrons)}, nil
}
