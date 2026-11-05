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
	c := &Codex{Bin: "codex", Exec: func(_ context.Context, name string, args []string, wd string, stdin io.Reader, out io.Writer) error {
		gotName, gotArgs, gotDir = name, args, wd
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
	if gotStdin != "PROMPT" {
		t.Fatalf("stdin = %q", gotStdin)
	}
	if term.String() != "thinking...\n" {
		t.Fatalf("terminal = %q", term.String())
	}
	logb, _ := os.ReadFile(filepath.Join(dir, "codex.log"))
	if string(logb) != "thinking...\n" {
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
	c := &Codex{Bin: "codex", Exec: func(_ context.Context, _ string, _ []string, _ string, _ io.Reader, out io.Writer) error {
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
