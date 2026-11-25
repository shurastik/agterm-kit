package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"syscall"
	"time"

	"github.com/shurastik/agterm-kit/internal/fsx"
)

// The autopilot line of a session and how long it lives. Every set restarts
// the expiry in agterm and the live period on disk, so a crashed autopilot
// goes dark by itself.
const (
	autopilotLine   = `🤖 ^[autopilot](color: "purple")`
	autopilotExpiry = 1200 * time.Second
	// autopilotBeatEvery is the least time between two refreshes of the line
	// by one command.
	autopilotBeatEvery = 60 * time.Second
)

// safeSessionID is the shape of a session id that may become a file name.
var safeSessionID = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// autopilotDir is where the live state of every session lives: outside any
// repository, because the main checkout and a change worktree both read it.
// <state> is $XDG_STATE_HOME when absolute, else ~/.local/state.
func (e *Env) autopilotDir() string {
	state := e.Getenv("XDG_STATE_HOME")
	if !filepath.IsAbs(state) {
		state = filepath.Join(e.Home, ".local", "state")
	}
	return filepath.Join(state, "agk", "autopilot")
}

func (e *Env) autopilotPath(session, ext string) (string, error) {
	if !safeSessionID.MatchString(session) {
		return "", fmt.Errorf("session id %q cannot name a state file", session)
	}
	return filepath.Join(e.autopilotDir(), session+ext), nil
}

type autopilotUntil struct {
	Until time.Time `json:"until"`
}

// autopilotMark records the session's autopilot as live for autopilotExpiry
// from now.
func (e *Env) autopilotMark(session string) error {
	p, err := e.autopilotPath(session, ".json")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	b, err := json.Marshal(autopilotUntil{Until: e.Now().Add(autopilotExpiry)})
	if err != nil {
		return err
	}
	return fsx.ReplaceFile(p, append(b, '\n'), 0o644)
}

// autopilotLive reports whether the session's autopilot is on and its live
// period has not run out. An unreadable or malformed file is not live.
func (e *Env) autopilotLive(session string) bool {
	p, err := e.autopilotPath(session, ".json")
	if err != nil {
		return false
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return false
	}
	var u autopilotUntil
	if json.Unmarshal(b, &u) != nil {
		return false
	}
	return u.Until.After(e.Now())
}

// autopilotDrop ends the session's live autopilot; a missing file is fine.
func (e *Env) autopilotDrop(session string) error {
	p, err := e.autopilotPath(session, ".json")
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// lockAutopilot takes the exclusive lock that serializes `on`, `off` and the
// refreshes of one session: a refresh that found the autopilot live must not
// set the line after an `off` returned. The lock file is never removed, so it
// cannot race its own deletion.
func (e *Env) lockAutopilot(session string) (unlock func(), err error) {
	p, err := e.autopilotPath(session, ".lock")
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return nil, err
	}
	return func() { f.Close() }, nil // closing the descriptor releases the lock
}

func (e *Env) autopilotWarn(what, session string, err error) {
	fmt.Fprintf(e.Stderr, "agk: warning: autopilot %s %s: %v\n", what, session, err)
}

// setAutopilotLine sets the session's autopilot line; the call runs detached
// from the caller's context and under the usual status timeout.
func (e *Env) setAutopilotLine(ctx context.Context, session string) error {
	cctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), statusTimeout)
	defer cancel()
	return e.Ctl.Info(cctx, session, autopilotLine, autopilotExpiry)
}

// autopilotOn sets the session's autopilot line and marks it live, under the
// session's lock. It reports whether both happened; a failure is a warning.
func (e *Env) autopilotOn(ctx context.Context, session string) bool {
	unlock, err := e.lockAutopilot(session)
	if err != nil {
		e.autopilotWarn("on", session, err)
		return false
	}
	defer unlock()
	if err := e.setAutopilotLine(ctx, session); err != nil {
		e.autopilotWarn("line", session, err)
		return false
	}
	if err := e.autopilotMark(session); err != nil {
		e.autopilotWarn("state", session, err)
		return false
	}
	return true
}

// autopilotOff ends the session's live autopilot and clears its line, under
// the session's lock. Both are attempted whatever the other did.
func (e *Env) autopilotOff(ctx context.Context, session string) {
	unlock, err := e.lockAutopilot(session)
	if err != nil {
		// Still try: leaving the line up is worse than an unserialized off.
		e.autopilotWarn("off", session, err)
	} else {
		defer unlock()
	}
	if err := e.autopilotDrop(session); err != nil {
		e.autopilotWarn("state", session, err)
	}
	cctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), statusTimeout)
	defer cancel()
	if err := e.Ctl.ClearInfo(cctx, session); err != nil {
		e.autopilotWarn("clear", session, err)
	}
}

// autopilotRefresh sets the line again and extends the live period, when the
// session's autopilot is live; under the session's lock.
func (e *Env) autopilotRefresh(ctx context.Context, session string) {
	unlock, err := e.lockAutopilot(session)
	if err != nil {
		e.autopilotWarn("refresh", session, err)
		return
	}
	defer unlock()
	if !e.autopilotLive(session) {
		return
	}
	if err := e.setAutopilotLine(ctx, session); err != nil {
		e.autopilotWarn("line", session, err)
		return
	}
	if err := e.autopilotMark(session); err != nil {
		e.autopilotWarn("state", session, err)
	}
}

// heartbeat keeps the autopilot line of one session alive for the length of a
// command: an armed command refreshes it on entry and then at most once per
// autopilotBeatEvery, wherever it waits.
type heartbeat struct {
	session string
	last    time.Time
}

// armBeat arms the heartbeat of this command for an autopilot run that
// recorded the session whose line it keeps. A run without the flag, an inline
// run (no session) and an id that cannot name a state file arm nothing.
func (e *Env) armBeat(autopilot bool, session string) {
	e.beat = nil
	if autopilot && safeSessionID.MatchString(session) {
		e.beat = &heartbeat{session: session}
	}
}

// beatNow refreshes the armed session's line unless it did so less than
// autopilotBeatEvery ago; the refresh itself does nothing unless the
// session's autopilot is live.
func (e *Env) beatNow(ctx context.Context) {
	b := e.beat
	if b == nil {
		return
	}
	now := e.Now()
	if !b.last.IsZero() && now.Sub(b.last) < autopilotBeatEvery {
		return
	}
	b.last = now
	e.autopilotRefresh(ctx, b.session)
}
