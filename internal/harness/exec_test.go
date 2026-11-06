package harness

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestRunStreamingCancelKillsProcessGroup(t *testing.T) {
	shortGraces(t)
	ctx, cancel := context.WithCancel(context.Background())
	var out bytes.Buffer
	done := make(chan error, 1)
	go func() {
		done <- RunStreaming(ctx, "sh", []string{"-c", "(sleep 30; echo late) & echo started; wait"}, t.TempDir(), os.Environ(), strings.NewReader(""), &out)
	}()
	time.Sleep(300 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("cancelled run must return an error")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("RunStreaming still blocked 5s after cancel")
	}
	if strings.Contains(out.String(), "late") {
		t.Fatal("grandchild survived")
	}
}

func TestRunStreamingCancelEscalatesToSIGKILL(t *testing.T) {
	shortGraces(t)
	pidFile := filepath.Join(t.TempDir(), "pid")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		script := `(trap '' TERM; exec sleep 30) & echo $! > '` + pidFile + `'; wait`
		done <- RunStreaming(ctx, "sh", []string{"-c", script}, t.TempDir(), os.Environ(), strings.NewReader(""), io.Discard)
	}()
	var pid int
	for i := 0; i < 50 && pid == 0; i++ {
		time.Sleep(50 * time.Millisecond)
		b, _ := os.ReadFile(pidFile)
		pid, _ = strconv.Atoi(strings.TrimSpace(string(b)))
	}
	if pid == 0 {
		t.Fatal("grandchild pid not written")
	}
	cancel()
	<-done
	deadline := time.Now().Add(5 * time.Second)
	for syscall.Kill(pid, 0) == nil {
		if time.Now().After(deadline) {
			syscall.Kill(pid, syscall.SIGKILL)
			t.Fatal("SIGTERM-ignoring descendant still alive")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestRunStreamingStopsGroupBeforeReturning(t *testing.T) {
	shortGraces(t)
	killGrace = time.Second
	pidFile := filepath.Join(t.TempDir(), "pid")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		script := `(trap '' TERM; exec sleep 30 >/dev/null 2>&1) & echo $! > '` + pidFile + `'; wait`
		done <- RunStreaming(ctx, "sh", []string{"-c", script}, t.TempDir(), os.Environ(), strings.NewReader(""), io.Discard)
	}()
	var pid int
	for i := 0; i < 50 && pid == 0; i++ {
		time.Sleep(50 * time.Millisecond)
		b, _ := os.ReadFile(pidFile)
		pid, _ = strconv.Atoi(strings.TrimSpace(string(b)))
	}
	if pid == 0 {
		t.Fatal("grandchild pid not written")
	}
	defer syscall.Kill(pid, syscall.SIGKILL)
	cancel()
	<-done
	// Allow only for launchd reaping the killed orphan, far below killGrace.
	deadline := time.Now().Add(300 * time.Millisecond)
	for syscall.Kill(pid, 0) == nil {
		if time.Now().After(deadline) {
			t.Fatal("descendant still alive after RunStreaming returned")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestRunStreamingInterruptsBeforeTerminating(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "pid")
	script := `perl -MPOSIX -e 'POSIX::setsid(); exec "sleep", "30"' >/dev/null 2>&1 &
child=$!
echo $child > '` + pidFile + `'
trap 'kill $child; exit 130' INT
trap 'exit 143' TERM
wait`
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- RunStreaming(ctx, "sh", []string{"-c", script}, t.TempDir(), os.Environ(), strings.NewReader(""), io.Discard)
	}()
	var pid int
	for i := 0; i < 50 && pid == 0; i++ {
		time.Sleep(50 * time.Millisecond)
		b, _ := os.ReadFile(pidFile)
		pid, _ = strconv.Atoi(strings.TrimSpace(string(b)))
	}
	if pid == 0 {
		t.Fatal("tool pid not written")
	}
	defer syscall.Kill(pid, syscall.SIGKILL)
	time.Sleep(100 * time.Millisecond) // let the traps install
	cancel()
	<-done
	deadline := time.Now().Add(time.Second)
	for syscall.Kill(pid, 0) == nil {
		if time.Now().After(deadline) {
			t.Fatal("tool process in its own session survived cancellation")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func shortGraces(t *testing.T) {
	t.Helper()
	oi, ok := interruptGrace, killGrace
	interruptGrace, killGrace = 200*time.Millisecond, 200*time.Millisecond
	t.Cleanup(func() { interruptGrace, killGrace = oi, ok })
}

func TestRunStreamingReapsGroupAfterCrash(t *testing.T) {
	shortGraces(t)
	pidFile := filepath.Join(t.TempDir(), "pid")
	script := `sleep 30 >/dev/null 2>&1 & echo $! > '` + pidFile + `'; exit 1`
	err := RunStreaming(context.Background(), "sh", []string{"-c", script}, t.TempDir(), os.Environ(), strings.NewReader(""), io.Discard)
	if err == nil {
		t.Fatal("want the wrapper's failure")
	}
	b, _ := os.ReadFile(pidFile)
	pid, _ := strconv.Atoi(strings.TrimSpace(string(b)))
	if pid == 0 {
		t.Fatal("child pid not written")
	}
	defer syscall.Kill(pid, syscall.SIGKILL)
	deadline := time.Now().Add(time.Second)
	for syscall.Kill(pid, 0) == nil {
		if time.Now().After(deadline) {
			t.Fatal("orphaned child still running after RunStreaming returned")
		}
		time.Sleep(20 * time.Millisecond)
	}
}
