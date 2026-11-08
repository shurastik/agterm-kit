package agterm

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// failure turns a non-zero agtermctl exit into an error, wrapping ErrGone when
// the target session does not exist.
func failure(cmd string, res Result) error {
	msg := strings.TrimSpace(string(res.Stderr))
	var resp struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(res.Stdout, &resp) == nil && resp.Error != "" {
		msg = resp.Error
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
