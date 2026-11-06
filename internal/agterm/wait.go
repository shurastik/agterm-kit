package agterm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ErrGone means the session is not (or no longer) in agterm's open tree.
var ErrGone = errors.New("session gone")

// ErrTimeout means a wait ended on its timeout before a matching status.
var ErrTimeout = errors.New("wait timed out")

// noSuchSession is how agtermctl reports an unknown or closed session.
const noSuchSession = "no such session"

// Wait blocks until target's agent status is one of until, and returns it.
// It returns ErrGone when target is closed (before or during the wait) and
// ErrTimeout when timeout passes first. agtermctl takes whole seconds, so
// timeout is rounded up.
func (c *CLI) Wait(ctx context.Context, target string, until []string, timeout time.Duration) (string, error) {
	args := []string{"session", "wait", "--target", target}
	for _, u := range until {
		args = append(args, "--until", u)
	}
	secs := int64((timeout + time.Second - 1) / time.Second)
	args = append(args, "--timeout", strconv.FormatInt(secs, 10), "--json")
	res, err := c.run(ctx, args...)
	if err != nil {
		return "", err
	}
	switch {
	case res.ExitCode == 2:
		return "", ErrGone
	case res.ExitCode == 124:
		return "", ErrTimeout
	case res.ExitCode != 0:
		msg := strings.TrimSpace(string(res.Stderr))
		if strings.Contains(msg, noSuchSession) {
			return "", fmt.Errorf("%w: %s", ErrGone, msg)
		}
		return "", fmt.Errorf("agtermctl session wait: exit %d: %s", res.ExitCode, msg)
	}
	var resp struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(res.Stdout, &resp); err != nil || resp.Status == "" {
		return "", fmt.Errorf("agtermctl session wait: unexpected response %q", strings.TrimSpace(string(res.Stdout)))
	}
	return resp.Status, nil
}

// Close closes target with ⌘W's undo grace. It returns ErrGone when target is
// not open.
func (c *CLI) Close(ctx context.Context, target string) error {
	res, err := c.run(ctx, "session", "close", "--target", target, "--undoable", "--json")
	if err != nil {
		return err
	}
	if res.ExitCode == 0 {
		return nil
	}
	var resp struct {
		Error string `json:"error"`
	}
	msg := strings.TrimSpace(string(res.Stderr))
	if json.Unmarshal(res.Stdout, &resp) == nil && resp.Error != "" {
		msg = resp.Error
	}
	if strings.Contains(msg, noSuchSession) {
		return fmt.Errorf("%w: %s", ErrGone, msg)
	}
	return fmt.Errorf("agtermctl session close: exit %d: %s", res.ExitCode, msg)
}
