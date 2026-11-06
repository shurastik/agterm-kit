package harness

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
	"github.com/shurastik/agterm-kit/internal/config"
)

// fakeExec records one call and plays a scripted stdout.
type fakeExec struct {
	name, dir, stdin string
	args, env        []string
	stdout           string
	err              error
}

func (f *fakeExec) run(_ context.Context, name string, args []string, dir string, env []string, stdin io.Reader, out io.Writer) error {
	f.name, f.args, f.dir, f.env = name, args, dir, env
	b, _ := io.ReadAll(stdin)
	f.stdin = string(b)
	io.WriteString(out, f.stdout)
	return f.err
}

func testRuntime(f *fakeExec) Runtime {
	return Runtime{Exec: f.run, Now: fixedNow, Environ: func() []string {
		return []string{"PATH=/bin", "AGTERM_SESSION_ID=C", "AGTERM_ENABLED=1", "HOME=/h"}
	}}
}

const codexEvents = `{"type":"thread.started","thread_id":"T-1"}
{"type":"turn.started"}
{"type":"item.completed","item":{"id":"i0","type":"reasoning","text":"Looking at the diff\nmore"}}
{"type":"item.started","item":{"id":"i1","type":"command_execution","command":"git diff main","status":"in_progress"}}
{"type":"item.completed","item":{"id":"i1","type":"command_execution","command":"git diff main","exit_code":0}}
{"type":"item.completed","item":{"id":"i2","type":"agent_message","text":"{\"verdict\":\"approve\"}"}}
{"type":"turn.completed","usage":{}}
`

func TestCodexReviewFirstRound(t *testing.T) {
	dir := t.TempDir()
	f := &fakeExec{stdout: codexEvents}
	d := NewCodex(testRuntime(f))
	var term bytes.Buffer
	w := config.Worker{Harness: config.Codex, Model: "gpt-5.5", Effort: "high", ExtraArgs: []string{"--profile", "review"}}
	sess, err := d.Review(context.Background(), Turn{Worker: w, Root: "/repo", Dir: dir, Log: "reviewer.log", Prompt: "PROMPT"}, &term)
	if err != nil {
		t.Fatal(err)
	}
	if sess != "T-1" {
		t.Fatalf("session = %q", sess)
	}
	schema := filepath.Join(dir, SchemaFile)
	want := []string{"exec", "-s", "read-only", "-C", "/repo", "--json",
		"--output-schema", schema, "-o", filepath.Join(dir, OutputFile),
		"-m", "gpt-5.5", "-c", `model_reasoning_effort="high"`, "--profile", "review", "-"}
	if f.name != "codex" || f.dir != "/repo" || !reflect.DeepEqual(f.args, want) {
		t.Fatalf("exec %q %q in %q", f.name, f.args, f.dir)
	}
	if !reflect.DeepEqual(f.env, []string{"PATH=/bin", "HOME=/h"}) {
		t.Fatalf("env = %q", f.env)
	}
	if f.stdin != "PROMPT" {
		t.Fatalf("stdin = %q", f.stdin)
	}
	if b, _ := os.ReadFile(schema); !bytes.Equal(b, assets.ReviewSchema) {
		t.Fatal("schema file differs from the embedded schema")
	}
	wantTerm := "[2026-11-05 17:53:55] · Looking at the diff\n" +
		"[2026-11-05 17:53:55] $ git diff main\n" +
		"[2026-11-05 17:53:55] {\"verdict\":\"approve\"}\n"
	if term.String() != wantTerm {
		t.Fatalf("terminal =\n%s", term.String())
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "reviewer.log")); string(b) != wantTerm {
		t.Fatalf("log = %q", b)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, EventsFile)); string(b) != codexEvents {
		t.Fatalf("events.jsonl = %q", b)
	}
}

func TestCodexReviewResume(t *testing.T) {
	dir := t.TempDir()
	f := &fakeExec{stdout: `{"type":"thread.started","thread_id":"T-1"}` + "\n"}
	d := NewCodex(testRuntime(f))
	sess, err := d.Review(context.Background(), Turn{Worker: config.Worker{Harness: config.Codex}, Root: "/repo", Dir: dir, Log: "reviewer.log", Prompt: "P", Session: "T-1", Resume: true}, io.Discard)
	if err != nil || sess != "T-1" {
		t.Fatalf("session %q, %v", sess, err)
	}
	want := []string{"exec", "resume", "T-1", "-c", `sandbox_mode="read-only"`, "--json",
		"--output-schema", filepath.Join(dir, SchemaFile), "-o", filepath.Join(dir, OutputFile), "-"}
	if !reflect.DeepEqual(f.args, want) || f.dir != "/repo" {
		t.Fatalf("args = %q in %q", f.args, f.dir)
	}
}

func TestCodexReviewFailure(t *testing.T) {
	dir := t.TempDir()
	f := &fakeExec{stdout: "auth error\n" + `{"type":"turn.failed","error":{"message":"quota"}}` + "\n", err: errors.New("exit status 1")}
	var term bytes.Buffer
	_, err := NewCodex(testRuntime(f)).Review(context.Background(), Turn{Worker: config.Worker{Harness: config.Codex}, Root: "/r", Dir: dir, Log: "reviewer.log"}, &term)
	if err == nil || !strings.Contains(err.Error(), "codex") {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(term.String(), "auth error") || !strings.Contains(term.String(), "error: quota") {
		t.Fatalf("terminal = %q", term.String())
	}
}

func TestCodexHeadless(t *testing.T) {
	dir := t.TempDir()
	f := &fakeExec{stdout: `{"type":"thread.started","thread_id":"T-9"}` + "\n"}
	d := NewCodex(testRuntime(f))
	turn := Turn{Worker: config.Worker{Harness: config.Codex, Model: "gpt-5.5"}, Root: "/repo", Dir: dir, Log: "applier.log", Prompt: "Read /p and follow it."}
	sess, err := d.Headless(context.Background(), turn, ApplierFiles{}, io.Discard)
	if err != nil || sess != "T-9" {
		t.Fatalf("session %q, %v", sess, err)
	}
	want := []string{"exec", "-s", "workspace-write", "-C", "/repo", "--json", "-m", "gpt-5.5", "-"}
	if !reflect.DeepEqual(f.args, want) || f.stdin != "Read /p and follow it." {
		t.Fatalf("args = %q stdin %q", f.args, f.stdin)
	}
	turn.Session, turn.Resume = "T-9", true
	if _, err := d.Headless(context.Background(), turn, ApplierFiles{}, io.Discard); err != nil {
		t.Fatal(err)
	}
	want = []string{"exec", "resume", "T-9", "-c", `sandbox_mode="workspace-write"`, "--json", "-m", "gpt-5.5", "-"}
	if !reflect.DeepEqual(f.args, want) {
		t.Fatalf("resume args = %q", f.args)
	}
}

func TestCodexInteractive(t *testing.T) {
	d := NewCodex(Runtime{})
	got := d.Interactive(config.Worker{Harness: config.Codex, Model: "gpt-5.5", Effort: "medium", ExtraArgs: []string{"--search"}}, "/repo", ApplierFiles{}, "", "Read /p and follow it.")
	want := []string{"codex", "--sandbox", "workspace-write", "--ask-for-approval", "never", "-C", "/repo",
		"-m", "gpt-5.5", "-c", `model_reasoning_effort="medium"`, "--search", "Read /p and follow it."}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("argv = %q", got)
	}
	if d.Name() != config.Codex {
		t.Fatal("name")
	}
}

func TestCodexNeutralizesTerminalControls(t *testing.T) {
	dir := t.TempDir()
	f := &fakeExec{stdout: "file: \x1b]52;c;ZXZpbA==\x07\x1b[2J done\n" +
		`{"type":"item.completed","item":{"type":"agent_message","text":"x\u001b]52;c;QQ==\u0007"}}` + "\n"}
	var term bytes.Buffer
	if _, err := NewCodex(testRuntime(f)).Review(context.Background(), Turn{Worker: config.Worker{Harness: config.Codex}, Root: "/r", Dir: dir, Log: "reviewer.log"}, &term); err != nil {
		t.Fatal(err)
	}
	logb, _ := os.ReadFile(filepath.Join(dir, "reviewer.log"))
	for name, out := range map[string][]byte{"terminal": term.Bytes(), "log": logb} {
		if bytes.IndexByte(out, 0x1b) >= 0 || bytes.IndexByte(out, 0x07) >= 0 {
			t.Errorf("%s got raw control bytes: %q", name, out)
		}
		if !bytes.Contains(out, []byte(`\u001b]52;c;ZXZpbA==\u0007\u001b[2J done`)) || !bytes.Contains(out, []byte(`x\u001b]52;c;QQ==\u0007`)) {
			t.Errorf("%s lacks the visible escapes: %q", name, out)
		}
	}
}
