package review

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/shurastik/agterm-kit/assets"
)

// Runner runs the reviewer for a prepared run directory.
type Runner interface {
	// Run reviews the repo at root with prompt, streaming the reviewer's
	// output to term and dir/codex.log. The raw result lands at OutputPath(dir).
	Run(ctx context.Context, root, dir, prompt string, term io.Writer) error
	OutputPath(dir string) string
}

// StreamExec runs name with args in dir, feeding stdin and sending combined
// stdout and stderr to out. A non-zero exit is an error.
type StreamExec func(ctx context.Context, name string, args []string, dir string, stdin io.Reader, out io.Writer) error

// RunStreaming is the real StreamExec.
func RunStreaming(ctx context.Context, name string, args []string, dir string, stdin io.Reader, out io.Writer) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir, cmd.Stdin, cmd.Stdout, cmd.Stderr = dir, stdin, out, out
	return cmd.Run()
}

// Codex runs `codex exec` read-only with the bundled output schema.
type Codex struct {
	Bin  string
	Exec StreamExec
}

// NewCodex returns a Codex runner using codex from PATH.
func NewCodex() *Codex { return &Codex{Bin: "codex", Exec: RunStreaming} }

var _ Runner = (*Codex)(nil)

// OutputPath is where codex writes its final message.
func (c *Codex) OutputPath(dir string) string { return filepath.Join(dir, "codex-output.json") }

func (c *Codex) Run(ctx context.Context, root, dir, prompt string, term io.Writer) error {
	schema := filepath.Join(dir, "review-output.schema.json")
	if err := os.WriteFile(schema, assets.ReviewSchema, 0o644); err != nil {
		return err
	}
	logf, err := os.Create(filepath.Join(dir, "codex.log"))
	if err != nil {
		return err
	}
	defer logf.Close()
	args := []string{"exec", "-s", "read-only", "-C", root, "--ephemeral",
		"--output-schema", schema, "-o", c.OutputPath(dir), "-"}
	if err := c.Exec(ctx, c.Bin, args, root, strings.NewReader(prompt), io.MultiWriter(term, logf)); err != nil {
		return fmt.Errorf("codex failed: %w", err)
	}
	return nil
}
