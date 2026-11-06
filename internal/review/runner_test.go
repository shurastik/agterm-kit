package review

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/shurastik/agterm-kit/assets"
)

func TestCodexRun(t *testing.T) {
	dir := t.TempDir()
	var gotName, gotDir, gotStdin string
	var gotArgs []string
	var gotEnv []string
	c := &Codex{Bin: "codex", Now: fixedNow, Environ: func() []string {
		return []string{"PATH=/bin", "AGTERM_SESSION_ID=C", "AGTERM_ENABLED=1", "AGTERM_SOCKET=/s", "HOME=/h"}
	}, Exec: func(_ context.Context, name string, args []string, wd string, env []string, stdin io.Reader, out io.Writer) error {
		gotName, gotArgs, gotDir, gotEnv = name, args, wd, env
		b, _ := io.ReadAll(stdin)
		gotStdin = string(b)
		io.WriteString(out, "thinking...\n")
		return nil
	}}
	var term bytes.Buffer
	if err := c.Run(context.Background(), "/repo", dir, "PROMPT", &term); err != nil {
		t.Fatal(err)
	}
	wantArgs := []string{"exec", "-s", "read-only", "-C", "/repo", "--ephemeral",
		"--output-schema", filepath.Join(dir, "review-output.schema.json"),
		"-o", filepath.Join(dir, "codex-output.json"), "-"}
	if gotName != "codex" || gotDir != "/repo" || !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Fatalf("exec %q %q in %q", gotName, gotArgs, gotDir)
	}
	// The child must not see agterm identity, or its own status hooks would
	// mark the companion completed before agk validates the result.
	if !reflect.DeepEqual(gotEnv, []string{"PATH=/bin", "HOME=/h"}) {
		t.Fatalf("env = %q", gotEnv)
	}
	if gotStdin != "PROMPT" {
		t.Fatalf("stdin = %q", gotStdin)
	}
	if term.String() != "[2026-11-05 17:53:55] thinking...\n" {
		t.Fatalf("terminal = %q", term.String())
	}
	logb, _ := os.ReadFile(filepath.Join(dir, "codex.log"))
	if string(logb) != "[2026-11-05 17:53:55] thinking...\n" {
		t.Fatalf("codex.log = %q", logb)
	}
	schema, _ := os.ReadFile(filepath.Join(dir, "review-output.schema.json"))
	if !bytes.Equal(schema, assets.ReviewSchema) {
		t.Fatal("schema file differs from embedded schema")
	}
	if c.OutputPath(dir) != filepath.Join(dir, "codex-output.json") {
		t.Fatal("OutputPath mismatch")
	}
}

func TestCodexRunFailure(t *testing.T) {
	dir := t.TempDir()
	c := &Codex{Bin: "codex", Environ: os.Environ, Exec: func(_ context.Context, _ string, _ []string, _ string, _ []string, _ io.Reader, out io.Writer) error {
		io.WriteString(out, "auth error\n")
		return errors.New("exit status 1")
	}}
	err := c.Run(context.Background(), "/repo", dir, "P", io.Discard)
	if err == nil || !strings.Contains(err.Error(), "codex") {
		t.Fatalf("err = %v", err)
	}
	logb, _ := os.ReadFile(filepath.Join(dir, "codex.log"))
	if !strings.Contains(string(logb), "auth error") {
		t.Fatal("failure output must still be logged")
	}
}

// A grandchild that inherits stdout (the npm codex wrapper spawning the native
// binary) must not keep RunStreaming alive after cancellation.
// A descendant that ignores SIGTERM must still be killed after the grace period.
// When the descendant has detached its output, Run returns as soon as the
// shell dies; the group must still be dead by the time RunStreaming returns,
// because agk exits right after and an async timer would die with it.
// Codex runs tool commands in their own Unix sessions, outside our process
// group; only codex itself can clean them up, and it does so on SIGINT
// (turn interrupt). Cancellation must therefore interrupt before it terminates.
// shortGraces shrinks the cancellation grace periods for a test.
// Codex echoes the prompt, which carries untracked repository files; their
// escape sequences must not drive the companion terminal or a later cat of the log.
func TestCodexRunNeutralizesTerminalControls(t *testing.T) {
	dir := t.TempDir()
	c := &Codex{Bin: "codex", Now: fixedNow, Environ: os.Environ, Exec: func(_ context.Context, _ string, _ []string, _ string, _ []string, _ io.Reader, out io.Writer) error {
		io.WriteString(out, "file: \x1b]52;c;ZXZp")
		io.WriteString(out, "bA==\x07\x1b[2J done\n")
		return nil
	}}
	var term bytes.Buffer
	if err := c.Run(context.Background(), "/repo", dir, "P", &term); err != nil {
		t.Fatal(err)
	}
	logb, _ := os.ReadFile(filepath.Join(dir, "codex.log"))
	for name, out := range map[string][]byte{"terminal": term.Bytes(), "codex.log": logb} {
		if bytes.IndexByte(out, 0x1b) >= 0 || bytes.IndexByte(out, 0x07) >= 0 {
			t.Errorf("%s got raw control bytes: %q", name, out)
		}
		if !bytes.Contains(out, []byte(`\u001b]52;c;ZXZpbA==\u0007\u001b[2J done`)) {
			t.Errorf("%s lacks the visible escapes: %q", name, out)
		}
	}
}

// A wrapper that crashes and leaves a child behind must not leak that child,
// even without cancellation.
