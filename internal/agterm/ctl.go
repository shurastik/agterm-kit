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
	"path/filepath"
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
	return &CLI{Bin: ResolveBin(os.Getenv, isExecutable), Socket: os.Getenv("AGTERM_SOCKET"), Exec: RunProcess}
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
	if err := ctx.Err(); err != nil {
		return Outcome{}, err // never opened: leave whatever overlay is there alone
	}
	res, err := c.run(ctx, "session", "overlay", "open", "--html", file, "--js", "--block", "--follow", "--target", target)
	if ctx.Err() != nil {
		// The page belongs to the agterm app and outlives the killed agtermctl;
		// close it, but only if the overlay shown is still our page.
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if c.showsFile(cleanup, target, file) {
			_, _ = c.run(cleanup, "session", "overlay", "close", "--target", target)
		}
		return Outcome{}, ctx.Err()
	}
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

// showsFile reports whether target currently shows an HTML overlay of file.
func (c *CLI) showsFile(ctx context.Context, target, file string) bool {
	res, err := c.runOK(ctx, "tree", "--json")
	if err != nil {
		return false
	}
	var tree any
	if json.Unmarshal(res.Stdout, &tree) != nil {
		return false
	}
	want := canonical(file)
	for _, f := range overlayFiles(tree, target) {
		if canonical(f) == want {
			return true
		}
	}
	return false
}

// overlayFiles finds the session node with id target anywhere in the tree and
// returns its htmlOverlays[].file values.
func overlayFiles(node any, target string) []string {
	switch n := node.(type) {
	case map[string]any:
		if n["id"] == target {
			var out []string
			list, _ := n["htmlOverlays"].([]any)
			for _, o := range list {
				if m, ok := o.(map[string]any); ok {
					if f, ok := m["file"].(string); ok {
						out = append(out, f)
					}
				}
			}
			return out
		}
		for _, v := range n {
			if out := overlayFiles(v, target); out != nil {
				return out
			}
		}
	case []any:
		for _, v := range n {
			if out := overlayFiles(v, target); out != nil {
				return out
			}
		}
	}
	return nil
}

// canonical resolves symlinks (agterm reports /tmp for /private/tmp).
func canonical(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return filepath.Clean(p)
}

// Ctl is the slice of agterm that agk uses.
type Ctl interface {
	NewCompanion(ctx context.Context, parent, name, command, cwd string) (string, error)
	Status(ctx context.Context, target, state string) error
	Info(ctx context.Context, target, markdown string) error
	OverlayHTML(ctx context.Context, target, file string) (Outcome, error)
}

var _ Ctl = (*CLI)(nil)
