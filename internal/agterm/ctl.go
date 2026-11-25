// Package agterm drives agterm through the agtermctl CLI.
package agterm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
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
	Bin string
	// Socket, when set, is passed as --socket; agtermctl ignores AGTERM_SOCKET.
	Socket string
	// Window is the caller's window ($AGTERM_WINDOW_ID); `tree` only shows one
	// window, the frontmost unless told otherwise.
	Window string
	Exec   ExecFunc
}

// BundledBin is where the agterm app ships agtermctl.
const BundledBin = "/Applications/agterm.app/Contents/MacOS/agtermctl"

// ResolveBin picks agtermctl the way agterm's own hooks do: $AGTERMCTL, then
// the app bundle, then PATH.
func ResolveBin(getenv func(string) string, isExec func(string) bool) string {
	if b := getenv("AGTERMCTL"); b != "" {
		return b
	}
	if isExec(BundledBin) {
		return BundledBin
	}
	return "agtermctl"
}

func isExecutable(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.Mode().IsRegular() && fi.Mode()&0o111 != 0
}

// NewCLI returns a CLI for the agterm instance this process runs in.
func NewCLI() *CLI {
	return &CLI{
		Bin:    ResolveBin(os.Getenv, isExecutable),
		Socket: os.Getenv("AGTERM_SOCKET"),
		Window: os.Getenv("AGTERM_WINDOW_ID"),
		Exec:   RunProcess,
	}
}

func (c *CLI) run(ctx context.Context, args ...string) (Result, error) {
	if c.Socket != "" {
		args = withSocket(args, c.Socket)
	}
	res, err := c.Exec(ctx, c.Bin, args)
	if err != nil {
		return res, fmt.Errorf("agtermctl %s: %w", strings.Join(args[:2], " "), err)
	}
	return res, nil
}

// withSocket adds --socket before a trailing "--" terminator, if any.
func withSocket(args []string, socket string) []string {
	out := make([]string, 0, len(args)+2)
	for i, a := range args {
		if a == "--" {
			out = append(out, "--socket", socket)
			return append(out, args[i:]...)
		}
		out = append(out, a)
	}
	return append(out, "--socket", socket)
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

// Info replaces the sidebar info line of target with inline markdown. The line
// hides itself after hideAfter (whole seconds, restarted by each set); 0 keeps
// it until it is replaced or cleared.
func (c *CLI) Info(ctx context.Context, target, markdown string, hideAfter time.Duration) error {
	_, err := c.runOK(ctx, "session", "info", "--markdown", "--hide-after", strconv.Itoa(int(hideAfter/time.Second)), "--target", target, "--", markdown)
	return err
}

// ClearInfo removes the sidebar info line of target.
func (c *CLI) ClearInfo(ctx context.Context, target string) error {
	_, err := c.runOK(ctx, "session", "info", "--clear", "--target", target)
	return err
}

// Tint colors the sidebar icon of target.
func (c *CLI) Tint(ctx context.Context, target, color string) error {
	_, err := c.runOK(ctx, "session", "tint", color, "--on", "icon", "--target", target)
	return err
}

// Rename sets the sidebar name of target.
func (c *CLI) Rename(ctx context.Context, target, name string) error {
	_, err := c.runOK(ctx, "session", "rename", name, "--target", target)
	return err
}

// Ctl is the slice of agterm that agk uses.
type Ctl interface {
	NewCompanion(ctx context.Context, parent, name, command, cwd string) (string, error)
	Status(ctx context.Context, target, state string) error
	Info(ctx context.Context, target, markdown string, hideAfter time.Duration) error
	ClearInfo(ctx context.Context, target string) error
	Tint(ctx context.Context, target, color string) error
	Rename(ctx context.Context, target, name string) error
	Wait(ctx context.Context, target string, until []string, timeout time.Duration) (string, error)
	Close(ctx context.Context, target string) error
	Type(ctx context.Context, target, text string) error
	Text(ctx context.Context, target string, lines int) (string, error)
}

var _ Ctl = (*CLI)(nil)
