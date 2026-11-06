package harness

import (
	"context"
	"io"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

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
	if cmd.Process != nil {
		// Whatever ended the run (cancel, crash, normal exit), nothing from
		// codex's group may outlive it. Synchronous on purpose: agk exits right
		// after, so a timer would die with it. Usually the group is already
		// empty and this returns at once.
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
