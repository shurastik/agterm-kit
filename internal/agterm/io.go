package agterm

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Message is one mailbox message.
type Message struct {
	ID   string
	From string
	Body string
}

// failure turns a non-zero agtermctl exit into an error, wrapping ErrGone when
// the target session or pane does not exist.
func failure(cmd string, res Result) error {
	msg := strings.TrimSpace(string(res.Stderr))
	var resp struct {
		Error  string `json:"error"`
		Result struct {
			Rejected []struct {
				Address string `json:"address"`
				Reason  string `json:"reason"`
			} `json:"rejected"`
		} `json:"result"`
	}
	if json.Unmarshal(res.Stdout, &resp) == nil {
		if resp.Error != "" {
			msg = resp.Error
		}
		for _, r := range resp.Result.Rejected {
			msg = r.Address + ": " + r.Reason
			if r.Reason == "unknown pane" || strings.Contains(r.Reason, noSuchSession) {
				return fmt.Errorf("%w: agtermctl %s: %s", ErrGone, cmd, msg)
			}
		}
	}
	if strings.Contains(msg, noSuchSession) {
		return fmt.Errorf("%w: agtermctl %s: %s", ErrGone, cmd, msg)
	}
	return fmt.Errorf("agtermctl %s: exit %d: %s", cmd, res.ExitCode, msg)
}

// Type injects text into target's primary pane, as if typed. Include "\r" to
// press Enter.
func (c *CLI) Type(ctx context.Context, target, text string) error {
	res, err := c.run(ctx, "session", "type", "--target", target, "--json", "--", text)
	if err != nil {
		return err
	}
	if res.ExitCode != 0 {
		return failure("session type", res)
	}
	return nil
}

// Text returns the last lines of target's terminal buffer as plain text.
func (c *CLI) Text(ctx context.Context, target string, lines int) (string, error) {
	res, err := c.run(ctx, "session", "text", "--target", target, "--lines", strconv.Itoa(lines), "--json")
	if err != nil {
		return "", err
	}
	if res.ExitCode != 0 {
		return "", failure("session text", res)
	}
	var resp struct {
		OK     bool `json:"ok"`
		Result struct {
			Text *string `json:"text"`
		} `json:"result"`
	}
	if err := json.Unmarshal(res.Stdout, &resp); err != nil || !resp.OK || resp.Result.Text == nil {
		return "", fmt.Errorf("agtermctl session text: unexpected response %q", strings.TrimSpace(string(res.Stdout)))
	}
	return *resp.Result.Text, nil
}

// MsgSend delivers body to the mailbox of pane to (SESSION:PANE).
func (c *CLI) MsgSend(ctx context.Context, to, body string) error {
	res, err := c.run(ctx, "msg", "send", "--to", to, "--json", "--", body)
	if err != nil {
		return err
	}
	if res.ExitCode != 0 {
		return failure("msg send", res)
	}
	return nil
}

// MsgRead waits for messages in the caller's own pane mailbox
// ($AGTERM_SESSION_ID:$AGTERM_PANE_ID) and removes the ones it returns. A
// zero timeout waits indefinitely; otherwise ErrTimeout ends the wait.
func (c *CLI) MsgRead(ctx context.Context, timeout time.Duration) ([]Message, error) {
	args := []string{"msg", "read", "--wait", "--ack", "--json"}
	if timeout > 0 {
		secs := int64((timeout + time.Second - 1) / time.Second)
		args = append(args, "--timeout", strconv.FormatInt(secs, 10))
	}
	res, err := c.run(ctx, args...)
	if err != nil {
		return nil, err
	}
	if res.ExitCode == 124 {
		return nil, ErrTimeout
	}
	if res.ExitCode != 0 {
		return nil, failure("msg read", res)
	}
	var resp struct {
		OK     bool `json:"ok"`
		Result struct {
			Messages []struct {
				ID   string `json:"id"`
				From string `json:"from"`
				Body string `json:"body"`
			} `json:"messages"`
		} `json:"result"`
	}
	if err := json.Unmarshal(res.Stdout, &resp); err != nil || !resp.OK {
		return nil, fmt.Errorf("agtermctl msg read: unexpected response %q", strings.TrimSpace(string(res.Stdout)))
	}
	out := make([]Message, 0, len(resp.Result.Messages))
	for _, m := range resp.Result.Messages {
		out = append(out, Message{ID: m.ID, From: m.From, Body: m.Body})
	}
	return out, nil
}
