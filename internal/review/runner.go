package review

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/shurastik/agterm-kit/assets"
	"github.com/shurastik/agterm-kit/internal/harness"
	"github.com/shurastik/agterm-kit/internal/termsafe"
)

// Runner runs the reviewer for a prepared run directory.
type Runner interface {
	// Run reviews the repo at root with prompt, streaming the reviewer's
	// output to term and dir/codex.log. The raw result lands at OutputPath(dir).
	Run(ctx context.Context, root, dir, prompt string, term io.Writer) error
	OutputPath(dir string) string
}

// Codex runs `codex exec` read-only with the bundled output schema.
type Codex struct {
	Bin     string
	Exec    harness.StreamExec
	Environ func() []string
	Now     func() time.Time // clock for log line stamps
}

// NewCodex returns a Codex runner using codex from PATH.
func NewCodex() *Codex {
	return &Codex{Bin: "codex", Exec: harness.RunStreaming, Environ: os.Environ, Now: time.Now}
}

// childEnv drops agterm identity so codex's own agterm status hooks stay
// silent: only agk may set the companion's status, after validating output.
func childEnv(env []string) []string {
	out := make([]string, 0, len(env))
	for _, kv := range env {
		if !strings.HasPrefix(kv, "AGTERM_") {
			out = append(out, kv)
		}
	}
	return out
}

var _ Runner = (*Codex)(nil)

func (c *Codex) clock() func() time.Time {
	if c.Now == nil {
		return time.Now
	}
	return c.Now
}

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
	if err := c.Exec(ctx, c.Bin, args, root, childEnv(c.Environ()), strings.NewReader(prompt), harness.NewStampWriter(termsafe.NewWriter(io.MultiWriter(term, logf)), c.clock())); err != nil {
		return fmt.Errorf("codex failed: %w", err)
	}
	return nil
}
