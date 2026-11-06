// Package harness drives agent CLIs (claude, codex) as agk workers: the
// interactive applier, a headless applier turn and a structured review turn.
package harness

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/shurastik/agterm-kit/internal/config"
	"github.com/shurastik/agterm-kit/internal/termsafe"
)

// Files a turn leaves in its directory.
const (
	OutputFile = "output.json"               // structured review output
	EventsFile = "events.jsonl"              // raw event stream
	SchemaFile = "review-output.schema.json" // review output schema
)

// Turn is one non-interactive run of a harness.
type Turn struct {
	Worker  config.Worker
	Root    string // repository root, the working directory
	Dir     string // round directory: log, events and output files
	Log     string // log file name inside Dir
	Prompt  string // fed on stdin
	Session string // claude: session id to create or resume; codex: thread to resume
	Resume  bool
}

// ApplierFiles are the claude applier's settings (deny rules) and system
// rules; codex takes its rules inside the prompt.
type ApplierFiles struct {
	Settings, Rules string
}

// Driver turns a worker configuration into harness invocations.
type Driver interface {
	Name() string
	// Interactive is the argv of an interactive applier session started with
	// kickoff as its first prompt.
	Interactive(w config.Worker, root string, f ApplierFiles, session, kickoff string) []string
	// Headless runs one applier turn and returns the harness session id.
	Headless(ctx context.Context, t Turn, f ApplierFiles, term io.Writer) (string, error)
	// Review runs one read-only review turn constrained to the review schema,
	// leaving the structured result in t.Dir/OutputFile; returns the session id.
	Review(ctx context.Context, t Turn, term io.Writer) (string, error)
}

// Runtime is how drivers run processes.
type Runtime struct {
	Exec    StreamExec
	Environ func() []string
	Now     func() time.Time // clock for log line stamps
}

// DefaultRuntime runs real processes.
func DefaultRuntime() Runtime {
	return Runtime{Exec: RunStreaming, Environ: os.Environ, Now: time.Now}
}

// New returns the driver for harness name.
func New(name string, rt Runtime) (Driver, error) {
	switch name {
	case config.Claude:
		return NewClaude(rt), nil
	case config.Codex:
		return NewCodex(rt), nil
	}
	return nil, fmt.Errorf("unknown harness %q", name)
}

// NewSessionID returns a random version-4 UUID.
func NewSessionID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// ChildEnv drops agterm identity so the harness's own agterm status hooks
// stay silent: only agk sets a non-interactive worker's status.
func ChildEnv(env []string) []string {
	out := make([]string, 0, len(env))
	for _, kv := range env {
		if !strings.HasPrefix(kv, "AGTERM_") {
			out = append(out, kv)
		}
	}
	return out
}

// event is one decoded line of a harness's JSON event stream.
type event = map[string]any

// stream runs bin in t.Root with t.Prompt on stdin. Raw output goes to
// t.Dir/EventsFile; each line is rendered, sanitised, stamped and written to
// term and t.Dir/t.Log. observe sees every decoded event.
func (rt Runtime) stream(ctx context.Context, bin string, args []string, t Turn, term io.Writer, render func(event) []string, observe func(event)) error {
	logf, err := os.Create(filepath.Join(t.Dir, t.Log))
	if err != nil {
		return err
	}
	defer logf.Close()
	evf, err := os.Create(filepath.Join(t.Dir, EventsFile))
	if err != nil {
		return err
	}
	defer evf.Close()
	now := rt.Now
	if now == nil {
		now = time.Now
	}
	out := NewStampWriter(termsafe.NewWriter(io.MultiWriter(term, logf)), now)
	ew := &eventWriter{raw: evf, out: out, render: render, observe: observe}
	environ := rt.Environ
	if environ == nil {
		environ = os.Environ
	}
	err = rt.Exec(ctx, bin, args, t.Root, ChildEnv(environ()), strings.NewReader(t.Prompt), ew)
	ew.flush()
	return err
}

// eventWriter splits output into lines, keeps them raw, and renders JSON
// events into short human lines; other lines pass through.
type eventWriter struct {
	raw     io.Writer
	out     io.Writer
	render  func(event) []string
	observe func(event)
	buf     bytes.Buffer
}

func (w *eventWriter) Write(p []byte) (int, error) {
	w.buf.Write(p)
	for {
		i := bytes.IndexByte(w.buf.Bytes(), '\n')
		if i < 0 {
			break
		}
		line := string(w.buf.Next(i + 1))
		w.line(strings.TrimSuffix(line, "\n"))
	}
	return len(p), nil
}

func (w *eventWriter) flush() {
	if w.buf.Len() > 0 {
		w.line(w.buf.String())
		w.buf.Reset()
	}
}

func (w *eventWriter) line(l string) {
	io.WriteString(w.raw, l+"\n")
	var ev event
	if strings.HasPrefix(strings.TrimSpace(l), "{") && json.Unmarshal([]byte(l), &ev) == nil {
		if w.observe != nil {
			w.observe(ev)
		}
		for _, r := range w.render(ev) {
			io.WriteString(w.out, strings.TrimRight(r, "\n")+"\n")
		}
		return
	}
	io.WriteString(w.out, l+"\n")
}

// str digs a string out of nested maps: str(ev, "item", "text").
func str(m map[string]any, path ...string) string {
	var cur any = m
	for _, k := range path {
		mm, ok := cur.(map[string]any)
		if !ok {
			return ""
		}
		cur = mm[k]
	}
	s, _ := cur.(string)
	return s
}

// firstLine is s up to its first line break.
func firstLine(s string) string {
	sc := bufio.NewScanner(strings.NewReader(s))
	if sc.Scan() {
		return sc.Text()
	}
	return ""
}
