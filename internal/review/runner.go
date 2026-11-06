package review

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/shurastik/agterm-kit/assets"
	"github.com/shurastik/agterm-kit/internal/termsafe"
)

// Runner runs the reviewer for a prepared run directory.
type Runner interface {
	// Run reviews the repo at root with prompt, streaming the reviewer's
	// output to term and dir/codex.log. The raw result lands at OutputPath(dir).
	Run(ctx context.Context, root, dir, prompt string, term io.Writer) error
	OutputPath(dir string) string
}

// StreamExec runs name with args in dir and environment env, feeding stdin and
// sending combined stdout and stderr to out. A non-zero exit is an error.
type StreamExec func(ctx context.Context, name string, args []string, dir string, env []string, stdin io.Reader, out io.Writer) error

// waitDelay bounds how long RunStreaming waits for output pipes after the
// process exits or is cancelled.
const waitDelay = 5 * time.Second

// interruptGrace is how long codex gets to handle SIGINT (interrupt the turn
// and clean up its tool processes, which run in their own sessions) before
// SIGTERM. killGrace is how long the group then gets before SIGKILL.
var (
	interruptGrace = 5 * time.Second
	killGrace      = 3 * time.Second
)

// RunStreaming is the real StreamExec. The process runs in its own process
// group. Cancellation first sends SIGINT, which codex handles by interrupting
// the turn and cleaning up its tool processes (they live in separate sessions
// that only codex can reach); then SIGTERM after interruptGrace, and SIGKILL
// after killGrace. The npm codex wrapper spawns the native binary, so the
// whole group is signalled, not just the direct child.
func RunStreaming(ctx context.Context, name string, args []string, dir string, env []string, stdin io.Reader, out io.Writer) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir, cmd.Env, cmd.Stdin, cmd.Stdout, cmd.Stderr = dir, env, stdin, out, out
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	var (
		mu       sync.Mutex
		escalate *time.Timer
	)
	cmd.Cancel = func() error {
		pgid := cmd.Process.Pid
		mu.Lock()
		// Safe as a timer: it only matters while Run is still blocked.
		escalate = time.AfterFunc(interruptGrace, func() { _ = syscall.Kill(-pgid, syscall.SIGTERM) })
		mu.Unlock()
		return syscall.Kill(-pgid, syscall.SIGINT)
	}
	cmd.WaitDelay = waitDelay
	err := cmd.Run()
	mu.Lock()
	if escalate != nil {
		escalate.Stop()
	}
	mu.Unlock()
	if ctx.Err() != nil && cmd.Process != nil {
		// Synchronous on purpose: agk exits right after, so a timer would die with it.
		stopGroup(cmd.Process.Pid, killGrace)
	}
	return err
}

// stopGroup sends SIGTERM to the process group, waits up to grace for it to
// disappear, then sends SIGKILL.
func stopGroup(pgid int, grace time.Duration) {
	_ = syscall.Kill(-pgid, syscall.SIGTERM)
	deadline := time.Now().Add(grace)
	for time.Now().Before(deadline) {
		if syscall.Kill(-pgid, 0) != nil {
			return // no process left in the group
		}
		time.Sleep(50 * time.Millisecond)
	}
	_ = syscall.Kill(-pgid, syscall.SIGKILL)
}

// Codex runs `codex exec` read-only with the bundled output schema.
type Codex struct {
	Bin     string
	Exec    StreamExec
	Environ func() []string
	Now     func() time.Time // clock for log line stamps
}

// NewCodex returns a Codex runner using codex from PATH.
func NewCodex() *Codex {
	return &Codex{Bin: "codex", Exec: RunStreaming, Environ: os.Environ, Now: time.Now}
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
	if err := c.Exec(ctx, c.Bin, args, root, childEnv(c.Environ()), strings.NewReader(prompt), NewStampWriter(termsafe.NewWriter(io.MultiWriter(term, logf)), c.clock())); err != nil {
		return fmt.Errorf("codex failed: %w", err)
	}
	return nil
}
