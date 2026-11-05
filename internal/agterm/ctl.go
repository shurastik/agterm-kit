// Package agterm drives agterm through the agtermctl CLI.
package agterm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// Result is the captured outcome of one process run.
type Result struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
}

// ExecFunc runs a process to completion. A non-zero exit is reported through
// Result.ExitCode; the error is only for failing to run the process at all.
type ExecFunc func(ctx context.Context, name string, args []string) (Result, error)

// RunProcess is the real ExecFunc.
func RunProcess(ctx context.Context, name string, args []string) (Result, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	err := cmd.Run()
	res := Result{Stdout: out.Bytes(), Stderr: errb.Bytes()}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		res.ExitCode = exitErr.ExitCode()
		return res, nil
	}
	return res, err
}

// CLI implements Ctl by running agtermctl.
type CLI struct {
	Bin  string
	Exec ExecFunc
}

// NewCLI returns a CLI using agtermctl from PATH.
func NewCLI() *CLI { return &CLI{Bin: "agtermctl", Exec: RunProcess} }

func (c *CLI) run(ctx context.Context, args ...string) (Result, error) {
	res, err := c.Exec(ctx, c.Bin, args)
	if err != nil {
		return res, fmt.Errorf("agtermctl %s: %w", strings.Join(args[:2], " "), err)
	}
	return res, nil
}

func (c *CLI) runOK(ctx context.Context, args ...string) (Result, error) {
	res, err := c.run(ctx, args...)
	if err != nil {
		return res, err
	}
	if res.ExitCode != 0 {
		return res, fmt.Errorf("agtermctl %s: exit %d: %s", strings.Join(args[:2], " "), res.ExitCode, strings.TrimSpace(string(res.Stderr)))
	}
	return res, nil
}

// NewCompanion creates a background terminal companion of parent running
// command (through the user's login shell) and returns its session id.
func (c *CLI) NewCompanion(ctx context.Context, parent, name, command, cwd string) (string, error) {
	res, err := c.runOK(ctx, "session", "new",
		"--parent", parent,
		"--command", command,
		"--wait", "--no-select",
		"--name", name,
		"--cwd", cwd,
		"--json")
	if err != nil {
		return "", err
	}
	var resp struct {
		OK     bool `json:"ok"`
		Result struct {
			ID string `json:"id"`
		} `json:"result"`
	}
	if err := json.Unmarshal(res.Stdout, &resp); err != nil {
		return "", fmt.Errorf("agtermctl session new: bad response %q: %w", res.Stdout, err)
	}
	if !resp.OK || resp.Result.ID == "" {
		return "", fmt.Errorf("agtermctl session new: unexpected response %s", strings.TrimSpace(string(res.Stdout)))
	}
	return resp.Result.ID, nil
}

// Status sets the agent status glyph of target.
func (c *CLI) Status(ctx context.Context, target, state string) error {
	_, err := c.runOK(ctx, "session", "status", state, "--target", target)
	return err
}

// Info replaces the sidebar info line of target with inline markdown.
func (c *CLI) Info(ctx context.Context, target, markdown string) error {
	_, err := c.runOK(ctx, "session", "info", "--markdown", "--hide-after", "0", "--target", target, "--", markdown)
	return err
}

// ErrDismissed means the user closed an overlay page without submitting it.
var ErrDismissed = errors.New("overlay dismissed")

// Outcome is what a submitted overlay page returned.
type Outcome struct {
	Value string
}

// OverlayHTML shows file as a JavaScript-enabled page over target, switches to
// target, and blocks until the page submits (Outcome) or is closed
// (ErrDismissed). The page gets no file access.
func (c *CLI) OverlayHTML(ctx context.Context, target, file string) (Outcome, error) {
	res, err := c.run(ctx, "session", "overlay", "open", "--html", file, "--js", "--block", "--follow", "--target", target)
	if err != nil {
		return Outcome{}, err
	}
	if res.ExitCode != 0 && res.ExitCode != 2 {
		return Outcome{}, fmt.Errorf("agtermctl session overlay: exit %d: %s", res.ExitCode, strings.TrimSpace(string(res.Stderr)))
	}
	var resp struct {
		Outcome string           `json:"outcome"`
		Value   *json.RawMessage `json:"value"`
	}
	if err := json.Unmarshal(res.Stdout, &resp); err != nil {
		return Outcome{}, fmt.Errorf("agtermctl session overlay: bad response %q: %w", res.Stdout, err)
	}
	switch {
	case res.ExitCode == 2 && resp.Outcome == "dismissed":
		return Outcome{}, ErrDismissed
	case res.ExitCode == 0 && resp.Outcome == "submitted" && resp.Value != nil:
		var v string
		if err := json.Unmarshal(*resp.Value, &v); err != nil {
			return Outcome{}, fmt.Errorf("agtermctl session overlay: submitted value is not a string: %s", *resp.Value)
		}
		return Outcome{Value: v}, nil
	}
	return Outcome{}, fmt.Errorf("agtermctl session overlay: exit %d with unexpected response %s", res.ExitCode, strings.TrimSpace(string(res.Stdout)))
}

// Ctl is the slice of agterm that agk uses.
type Ctl interface {
	NewCompanion(ctx context.Context, parent, name, command, cwd string) (string, error)
	Status(ctx context.Context, target, state string) error
	Info(ctx context.Context, target, markdown string) error
	OverlayHTML(ctx context.Context, target, file string) (Outcome, error)
}

var _ Ctl = (*CLI)(nil)
