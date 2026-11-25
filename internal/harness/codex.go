package harness

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/shurastik/agterm-kit/assets"
	"github.com/shurastik/agterm-kit/internal/config"
)

// Codex drives codex-cli.
type Codex struct {
	Bin string
	rt  Runtime
}

// NewCodex returns a codex driver using codex from PATH.
func NewCodex(rt Runtime) *Codex { return &Codex{Bin: "codex", rt: rt} }

func (c *Codex) Name() string { return config.Codex }

// tuning is the model, effort and extra arguments of w.
func codexTuning(w config.Worker) []string {
	var args []string
	if w.Model != "" {
		args = append(args, "-m", w.Model)
	}
	if w.Effort != "" {
		args = append(args, "-c", fmt.Sprintf("model_reasoning_effort=%q", w.Effort))
	}
	return append(args, w.ExtraArgs...)
}

// codexNoPasteBurst turns off codex's paste-burst detection. agtermctl types a
// line and its Enter in one burst; without bracketed paste codex takes that
// burst for a paste and turns the Enter into a newline, so the line is never
// submitted. It comes after the user's tuning, so a later -c of theirs cannot
// turn the detection back on.
var codexNoPasteBurst = []string{"-c", "tui.disable_paste_burst=true"}

// codexNoDaemon is the flag that keeps an interactive codex out of the shared
// app-server daemon. Without it a codex with no -c overrides attaches to a
// running daemon, and its hooks then run there with the daemon's environment,
// so they report to the AGTERM_SESSION_ID the daemon was started from, often a
// session that no longer exists, instead of the worker's own. Needs codex
// 0.156.0 or newer.
const codexNoDaemon = "--no-daemon"

// withNoDaemon appends codexNoDaemon to argv unless the user's extra_args
// already carry that exact token: codex exits on a repeated flag.
func withNoDaemon(argv []string, w config.Worker) []string {
	for _, a := range w.ExtraArgs {
		if a == codexNoDaemon {
			return argv
		}
	}
	return append(argv, codexNoDaemon)
}

// Interactive runs codex with --no-daemon (see codexNoDaemon) after the
// tuning, next to the paste-burst flag.
func (c *Codex) Interactive(w config.Worker, root string, _ ApplierFiles, _, kickoff string) []string {
	argv := []string{c.Bin, "--sandbox", "workspace-write", "--ask-for-approval", "never", "-C", root}
	argv = append(argv, codexTuning(w)...)
	argv = append(argv, codexNoPasteBurst...)
	argv = withNoDaemon(argv, w)
	return append(argv, kickoff)
}

// InteractiveReviewer runs codex in a workspace-write sandbox whose only
// writable root is out (the working directory): configured extra roots are
// reset and the tmp roots excluded. Approvals are off, so a command that needs
// to leave the sandbox fails. The mandatory flags come after the tuning, so a
// later user -c cannot widen them. It also runs with --no-daemon (see
// codexNoDaemon), so its hooks run in its own process.
func (c *Codex) InteractiveReviewer(w config.Worker, _, out string, _ ReviewerFiles, _, kickoff string) []string {
	argv := append([]string{c.Bin}, codexTuning(w)...)
	argv = append(argv,
		"--sandbox", "workspace-write", "--ask-for-approval", "never", "-C", out,
		"-c", "sandbox_workspace_write.writable_roots=[]",
		"-c", "sandbox_workspace_write.exclude_tmpdir_env_var=true",
		"-c", "sandbox_workspace_write.exclude_slash_tmp=true",
		"-c", "tui.disable_paste_burst=true") // see codexNoPasteBurst
	argv = withNoDaemon(argv, w)
	return append(argv, kickoff)
}

// codexArgs is `exec` with a sandbox, or `exec <tuning> resume <thread>`
// keeping it; exec-level options must precede the resume subcommand, which
// rejects some of them (--profile). out are the output flags.
func codexArgs(t Turn, sandbox string, out ...string) []string {
	tuning := codexTuning(t.Worker)
	if t.Resume && t.Session != "" {
		args := append(append([]string{"exec"}, tuning...), "resume", t.Session, "-c", fmt.Sprintf("sandbox_mode=%q", sandbox), "--json")
		return append(append(args, out...), "-")
	}
	args := append([]string{"exec", "-s", sandbox, "-C", t.Root, "--json"}, out...)
	return append(append(args, tuning...), "-")
}

func (c *Codex) run(ctx context.Context, t Turn, args []string, term io.Writer) (string, error) {
	session := t.Session
	observe := func(ev event) {
		if ev["type"] == "thread.started" {
			if id := str(ev, "thread_id"); id != "" {
				session = id
			}
		}
	}
	if err := c.rt.stream(ctx, c.Bin, args, t, term, renderCodex, observe); err != nil {
		return session, fmt.Errorf("codex failed: %w", err)
	}
	return session, nil
}

func (c *Codex) Headless(ctx context.Context, t Turn, _ ApplierFiles, term io.Writer) (string, error) {
	return c.run(ctx, t, codexArgs(t, "workspace-write"), term)
}

func (c *Codex) Review(ctx context.Context, t Turn, term io.Writer) (string, error) {
	schema := filepath.Join(t.Dir, SchemaFile)
	if err := os.WriteFile(schema, assets.ReviewSchema, 0o644); err != nil {
		return t.Session, err
	}
	return c.run(ctx, t, codexArgs(t, "read-only", "--output-schema", schema, "-o", filepath.Join(t.Dir, OutputFile)), term)
}

// renderCodex turns a codex --json event into display lines.
func renderCodex(ev event) []string {
	switch ev["type"] {
	case "item.started":
		if str(ev, "item", "type") == "command_execution" {
			return []string{"$ " + str(ev, "item", "command")}
		}
	case "item.completed":
		switch str(ev, "item", "type") {
		case "agent_message":
			return []string{str(ev, "item", "text")}
		case "reasoning":
			if l := firstLine(str(ev, "item", "text")); l != "" {
				return []string{"· " + l}
			}
		}
	case "turn.failed":
		return []string{"error: " + str(ev, "error", "message")}
	case "error":
		return []string{"error: " + str(ev, "message")}
	}
	return nil
}
