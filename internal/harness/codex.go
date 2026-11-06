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

func (c *Codex) Interactive(w config.Worker, root string, _ ApplierFiles, _, kickoff string) []string {
	argv := []string{c.Bin, "--sandbox", "workspace-write", "--ask-for-approval", "never", "-C", root}
	argv = append(argv, codexTuning(w)...)
	return append(argv, kickoff)
}

// start is `exec` with a sandbox, or `exec resume <thread>` keeping it.
func codexStart(t Turn, sandbox string) []string {
	if t.Resume && t.Session != "" {
		return []string{"exec", "resume", t.Session, "-c", fmt.Sprintf("sandbox_mode=%q", sandbox), "--json"}
	}
	return []string{"exec", "-s", sandbox, "-C", t.Root, "--json"}
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
	args := append(codexStart(t, "workspace-write"), codexTuning(t.Worker)...)
	return c.run(ctx, t, append(args, "-"), term)
}

func (c *Codex) Review(ctx context.Context, t Turn, term io.Writer) (string, error) {
	schema := filepath.Join(t.Dir, SchemaFile)
	if err := os.WriteFile(schema, assets.ReviewSchema, 0o644); err != nil {
		return t.Session, err
	}
	args := append(codexStart(t, "read-only"), "--output-schema", schema, "-o", filepath.Join(t.Dir, OutputFile))
	args = append(args, codexTuning(t.Worker)...)
	return c.run(ctx, t, append(args, "-"), term)
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
