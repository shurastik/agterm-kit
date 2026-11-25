package review

import (
	"errors"
	"testing"

	"github.com/shurastik/agterm-kit/internal/agterm"
)

const invalidReview = `{"verdict":"maybe"}`

func TestDecide(t *testing.T) {
	valid, invalid, half := []byte(validReview), []byte(invalidReview), []byte(`{"verdict": "needs-att`)
	cases := []struct {
		name         string
		output       []byte
		exists       bool
		status       string
		waitErr      error
		fixups       int
		fixupStarted bool
		settled      bool
		want         Kind
		reason       string
	}{
		// A valid output completes the round at any status and any gate.
		{name: "valid, turn over", output: valid, exists: true, status: "completed", want: Completed},
		{name: "valid, idle", output: valid, exists: true, status: "idle", want: Completed},
		{name: "valid mid-turn", output: valid, exists: true, status: "active", want: Completed},
		{name: "valid, status unknown", output: valid, exists: true, want: Completed},
		{name: "valid, permission prompt open", output: valid, exists: true, status: "blocked", want: Completed},
		{name: "valid after timeout", output: valid, exists: true, waitErr: agterm.ErrTimeout, want: Completed},
		{name: "valid after gone", output: valid, exists: true, waitErr: agterm.ErrGone, want: Completed},
		{name: "valid, fix-up unconfirmed", output: valid, exists: true, status: "completed", fixups: 1, want: Completed},
		{name: "valid, fix-up unconfirmed, prompt", output: valid, exists: true, status: "blocked", fixups: 2, want: Completed},

		// A typed fix-up that has not started closes the gate.
		{name: "gate: blocked", output: invalid, exists: true, status: "blocked", fixups: 1, want: NeedsHuman, reason: "permission"},
		{name: "gate: blocked, no file", status: "blocked", fixups: 1, want: NeedsHuman, reason: "permission"},
		{name: "gate: gone", output: invalid, exists: true, waitErr: agterm.ErrGone, fixups: 1, want: Cancelled},
		{name: "gate: timeout", output: invalid, exists: true, waitErr: agterm.ErrTimeout, fixups: 1, want: Running},
		{name: "gate: old completed, invalid file", output: invalid, exists: true, status: "completed", fixups: 1, want: KeepWaiting},
		{name: "gate: old idle, invalid file", output: invalid, exists: true, status: "idle", fixups: 1, want: KeepWaiting},
		{name: "gate: old completed, half-rewritten file", output: half, exists: true, status: "completed", fixups: 1, want: KeepWaiting},
		{name: "gate: active seen but not recorded", output: invalid, exists: true, status: "active", fixups: 1, want: KeepWaiting},
		{name: "gate: file removed", status: "completed", fixups: 2, want: KeepWaiting},
		{name: "gate after the second fix-up", output: invalid, exists: true, status: "completed", fixups: 2, want: KeepWaiting},

		// An open gate (no fix-up, or the latest one started) decides on the turn.
		{name: "invalid mid-turn", output: invalid, exists: true, status: "active", want: KeepWaiting},
		{name: "half-written mid-turn", output: half, exists: true, status: "active", want: KeepWaiting},
		{name: "half-written, status unknown", output: half, exists: true, want: KeepWaiting},
		{name: "invalid after the turn: first fix-up", output: invalid, exists: true, status: "completed", want: Fixup},
		{name: "invalid after the turn (idle)", output: invalid, exists: true, status: "idle", want: Fixup},
		{name: "half-written after the turn", output: half, exists: true, status: "completed", want: Fixup},
		{name: "second fix-up", output: invalid, exists: true, status: "completed", fixups: 1, fixupStarted: true, want: Fixup},
		{name: "third invalid output fails the round", output: invalid, exists: true, status: "completed", fixups: 2, fixupStarted: true, want: RecordBlocked, reason: ReasonInvalidOutput},
		{name: "empty file after the turn", output: []byte{}, exists: true, status: "completed", want: Fixup},
		{name: "invalid mid-turn of a started fix-up", output: invalid, exists: true, status: "active", fixups: 1, fixupStarted: true, want: KeepWaiting},
		{name: "invalid, permission prompt open", output: invalid, exists: true, status: "blocked", want: NeedsHuman, reason: "permission"},

		// No output file.
		{name: "no output after the turn", status: "completed", want: NeedsHuman, reason: "no-output"},
		{name: "no output after the turn (idle)", status: "idle", want: NeedsHuman, reason: "no-output"},
		{name: "no output after a started fix-up", status: "completed", fixups: 1, fixupStarted: true, want: NeedsHuman, reason: "no-output"},
		{name: "no output mid-turn", status: "active", want: KeepWaiting},
		{name: "no output, status unknown", want: KeepWaiting},
		{name: "permission prompt, no output", status: "blocked", want: NeedsHuman, reason: "permission"},
		{name: "gone, no output", waitErr: agterm.ErrGone, want: Cancelled},
		{name: "gone, invalid output", output: invalid, exists: true, waitErr: agterm.ErrGone, want: Cancelled},
		{name: "timeout, no output", waitErr: agterm.ErrTimeout, want: Running},
		{name: "timeout with an invalid file and a running turn", output: invalid, exists: true, waitErr: agterm.ErrTimeout, want: Running},

		// A settled screen counts as a turn end next to completed and idle, but
		// overrides nothing else.
		{name: "settled, invalid output, status stuck active", output: invalid, exists: true, status: "active", settled: true, want: Fixup},
		{name: "settled, half-written output, status unknown", output: half, exists: true, settled: true, want: Fixup},
		{name: "settled, no output", status: "active", settled: true, want: NeedsHuman, reason: "no-output"},
		{name: "settled, no output, status unknown", settled: true, want: NeedsHuman, reason: "no-output"},
		{name: "settled, second fix-up after a started first", output: invalid, exists: true, status: "active", fixups: 1, fixupStarted: true, settled: true, want: Fixup},
		{name: "settled, third invalid output fails the round", output: invalid, exists: true, status: "active", fixups: 2, fixupStarted: true, settled: true, want: RecordBlocked, reason: ReasonInvalidOutput},
		{name: "settled, valid output completes", output: valid, exists: true, status: "active", settled: true, want: Completed},
		{name: "settled, fix-up not started", output: invalid, exists: true, status: "active", fixups: 1, settled: true, want: KeepWaiting},
		{name: "settled, fix-up not started, no file", status: "active", fixups: 1, settled: true, want: KeepWaiting},
		{name: "settled, gone", output: invalid, exists: true, status: "active", waitErr: agterm.ErrGone, settled: true, want: Cancelled},
		{name: "settled, timeout", output: invalid, exists: true, status: "active", waitErr: agterm.ErrTimeout, settled: true, want: Running},
		{name: "settled, blocked", output: invalid, exists: true, status: "blocked", settled: true, want: NeedsHuman, reason: "permission"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := Decide(2, c.output, c.exists, c.status, c.waitErr, c.fixups, c.fixupStarted, c.settled)
			if a.Kind != c.want || a.Reason != c.reason {
				t.Fatalf("got %s/%q, want %s/%q", a.Kind, a.Reason, c.want, c.reason)
			}
			switch a.Kind {
			case Completed:
				if a.Review.Verdict != "needs-attention" || len(a.Review.Findings) != 1 {
					t.Fatalf("completed without the parsed review: %+v", a.Review)
				}
			case Fixup, RecordBlocked:
				if a.Err == nil {
					t.Fatal("the validator error must travel with the action")
				}
			}
		})
	}
}

func TestDecideOtherWaitErrorIsNotSwallowed(t *testing.T) {
	// Any other wait error is the caller's to report; Decide treats it as no news.
	a := Decide(1, nil, false, "", errors.New("boom"), 0, false, false)
	if a.Kind != KeepWaiting {
		t.Fatalf("kind = %s", a.Kind)
	}
}
