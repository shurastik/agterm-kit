package harness

import (
	"bytes"
	"context"
	"encoding/json"
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

const claudeEvents = `{"type":"system","subtype":"init","session_id":"S-1"}
{"type":"assistant","message":{"content":[{"type":"thinking","thinking":"hmm"},{"type":"text","text":"Checking the diff."}]}}
{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Bash","input":{"command":"git diff main"}}]}}
{"type":"user","message":{"content":[{"type":"tool_result","content":"diff..."}]}}
{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Read","input":{"file_path":"/repo/a.go"}}]}}
{"type":"assistant","message":{"content":[{"type":"tool_use","name":"StructuredOutput","input":{"verdict":"approve"}}]}}
{"type":"result","subtype":"success","is_error":false,"session_id":"S-1","structured_output":{"verdict":"approve","summary":"ok","findings":[],"next_steps":[]}}
`

func reviewerTools() string {
	var out []string
	for _, l := range strings.Split(assets.ReviewerTools, "\n") {
		if l = strings.TrimSpace(l); l != "" && !strings.HasPrefix(l, "#") {
			out = append(out, l)
		}
	}
	return strings.Join(out, ",")
}

func TestClaudeReviewFirstRound(t *testing.T) {
	dir := t.TempDir()
	f := &fakeExec{stdout: claudeEvents}
	var term bytes.Buffer
	w := config.Worker{Harness: config.Claude, Model: "claude-opus-5-5", Effort: "high", ExtraArgs: []string{"--add-dir", "/x"}}
	sess, err := NewClaude(testRuntime(f)).Review(context.Background(), Turn{Worker: w, Root: "/repo", Dir: dir, Log: "reviewer.log", Prompt: "PROMPT", Session: "S-1"}, &term)
	if err != nil || sess != "S-1" {
		t.Fatalf("session %q, %v", sess, err)
	}
	want := []string{"-p", "--output-format", "stream-json", "--verbose",
		"--json-schema", string(assets.ReviewSchema),
		"--permission-mode", "dontAsk", "--allowedTools", reviewerTools(),
		"--model", "claude-opus-5-5", "--effort", "high", "--add-dir", "/x",
		"--session-id", "S-1"}
	if f.name != "claude" || f.dir != "/repo" || !reflect.DeepEqual(f.args, want) {
		t.Fatalf("exec %q %q in %q", f.name, f.args, f.dir)
	}
	for _, tool := range strings.Split(reviewerTools(), ",") {
		if strings.Contains(tool, "Edit") || strings.Contains(tool, "Write") || tool == "Bash" {
			t.Fatalf("reviewer may %s", tool)
		}
	}
	if !reflect.DeepEqual(f.env, []string{"PATH=/bin", "HOME=/h"}) || f.stdin != "PROMPT" {
		t.Fatalf("env %q stdin %q", f.env, f.stdin)
	}
	out, err := os.ReadFile(filepath.Join(dir, OutputFile))
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if json.Unmarshal(out, &got) != nil || got["verdict"] != "approve" {
		t.Fatalf("output.json = %s", out)
	}
	wantTerm := "[2026-11-05 17:53:55] Checking the diff.\n" +
		"[2026-11-05 17:53:55] $ git diff main\n" +
		"[2026-11-05 17:53:55] Read /repo/a.go\n"
	if term.String() != wantTerm {
		t.Fatalf("terminal =\n%s", term.String())
	}
	if b, _ := os.ReadFile(filepath.Join(dir, EventsFile)); string(b) != claudeEvents {
		t.Fatal("events.jsonl must keep the raw stream")
	}
}

func TestClaudeReviewResumeAndNewSession(t *testing.T) {
	f := &fakeExec{stdout: claudeEvents}
	d := NewClaude(testRuntime(f))
	turn := Turn{Worker: config.Worker{Harness: config.Claude}, Root: "/repo", Dir: t.TempDir(), Log: "reviewer.log", Session: "S-1", Resume: true}
	if _, err := d.Review(context.Background(), turn, io.Discard); err != nil {
		t.Fatal(err)
	}
	if got := f.args[len(f.args)-2:]; !reflect.DeepEqual(got, []string{"--resume", "S-1"}) {
		t.Fatalf("tail = %q", got)
	}
	turn.Session, turn.Resume = "", false
	sess, err := d.Review(context.Background(), turn, io.Discard)
	if err != nil || len(sess) != 36 || f.args[len(f.args)-2] != "--session-id" || f.args[len(f.args)-1] != sess {
		t.Fatalf("fresh session %q, %v, args %q", sess, err, f.args)
	}
}

func TestClaudeReviewFailures(t *testing.T) {
	cases := map[string]struct {
		stdout string
		err    error
		want   string
	}{
		"exec":          {stdout: "boom\n", err: errors.New("exit status 1"), want: "claude failed"},
		"no result":     {stdout: `{"type":"system","subtype":"init"}` + "\n", want: "no result"},
		"error result":  {stdout: `{"type":"result","is_error":true,"result":"rate limited"}` + "\n", want: "rate limited"},
		"no structured": {stdout: `{"type":"result","is_error":false,"result":"text"}` + "\n", want: "structured output"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			f := &fakeExec{stdout: tc.stdout, err: tc.err}
			var term bytes.Buffer
			_, err := NewClaude(testRuntime(f)).Review(context.Background(), Turn{Worker: config.Worker{Harness: config.Claude}, Root: "/r", Dir: dir, Log: "reviewer.log", Session: "S"}, &term)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v", err)
			}
			if _, statErr := os.Stat(filepath.Join(dir, OutputFile)); statErr == nil {
				t.Fatal("no output.json on failure")
			}
		})
	}
}

func TestClaudeErrorResultShown(t *testing.T) {
	f := &fakeExec{stdout: `{"type":"result","is_error":true,"result":"rate limited"}` + "\n"}
	var term bytes.Buffer
	NewClaude(testRuntime(f)).Review(context.Background(), Turn{Worker: config.Worker{Harness: config.Claude}, Root: "/r", Dir: t.TempDir(), Log: "reviewer.log", Session: "S"}, &term)
	if !strings.Contains(term.String(), "error: rate limited") {
		t.Fatalf("terminal = %q", term.String())
	}
}

func TestClaudeHeadless(t *testing.T) {
	f := &fakeExec{stdout: `{"type":"result","is_error":false,"result":"done"}` + "\n"}
	d := NewClaude(testRuntime(f))
	files := ApplierFiles{Settings: "/run/claude-settings.json", Rules: "/run/applier-rules.md"}
	turn := Turn{Worker: config.Worker{Harness: config.Claude, PermissionMode: "auto", Model: "claude-sonnet-5-5"}, Root: "/repo", Dir: t.TempDir(), Log: "applier.log", Prompt: "Read /p and follow it.", Session: "S-7"}
	sess, err := d.Headless(context.Background(), turn, files, io.Discard)
	if err != nil || sess != "S-7" {
		t.Fatalf("session %q, %v", sess, err)
	}
	want := []string{"-p", "--output-format", "stream-json", "--verbose",
		"--permission-mode", "auto", "--settings", files.Settings, "--append-system-prompt-file", files.Rules,
		"--model", "claude-sonnet-5-5", "--session-id", "S-7"}
	if !reflect.DeepEqual(f.args, want) || f.stdin != "Read /p and follow it." {
		t.Fatalf("args %q stdin %q", f.args, f.stdin)
	}
	turn.Resume = true
	d.Headless(context.Background(), turn, files, io.Discard)
	if got := f.args[len(f.args)-2:]; !reflect.DeepEqual(got, []string{"--resume", "S-7"}) {
		t.Fatalf("resume tail = %q", got)
	}
	f.stdout = `{"type":"result","is_error":true,"result":"denied"}` + "\n"
	if _, err := d.Headless(context.Background(), turn, files, io.Discard); err == nil || !strings.Contains(err.Error(), "denied") {
		t.Fatalf("error result: %v", err)
	}
}

func TestClaudeInteractive(t *testing.T) {
	d := NewClaude(Runtime{})
	files := ApplierFiles{Settings: "/run/s.json", Rules: "/run/r.md"}
	got := d.Interactive(config.Worker{Harness: config.Claude, PermissionMode: "acceptEdits", Effort: "high", ExtraArgs: []string{"--add-dir", "/x"}}, "/repo", files, "S-1", "Read /p and follow it.")
	want := []string{"claude", "--permission-mode", "acceptEdits", "--settings", "/run/s.json",
		"--append-system-prompt-file", "/run/r.md", "--session-id", "S-1",
		"--effort", "high", "--add-dir", "/x", "Read /p and follow it."}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("argv = %q", got)
	}
	if d.Name() != config.Claude {
		t.Fatal("name")
	}
}

func TestNewDriver(t *testing.T) {
	for _, n := range []string{config.Claude, config.Codex} {
		d, err := New(n, Runtime{})
		if err != nil || d.Name() != n {
			t.Errorf("New(%s) = %v, %v", n, d, err)
		}
	}
	if _, err := New("gemini", Runtime{}); err == nil {
		t.Error("unknown harness")
	}
	if id := NewSessionID(); len(id) != 36 || id[14] != '4' || id == NewSessionID() {
		t.Errorf("session id %q", id)
	}
}
