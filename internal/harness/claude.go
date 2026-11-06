package harness

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"

	"github.com/shurastik/agterm-kit/assets"
	"github.com/shurastik/agterm-kit/internal/config"
	"github.com/shurastik/agterm-kit/internal/fsx"
)

// Claude drives Claude Code.
type Claude struct {
	Bin string
	rt  Runtime
}

// NewClaude returns a claude driver using claude from PATH.
func NewClaude(rt Runtime) *Claude { return &Claude{Bin: "claude", rt: rt} }

func (c *Claude) Name() string { return config.Claude }

func claudeTuning(w config.Worker) []string {
	var args []string
	if w.Model != "" {
		args = append(args, "--model", w.Model)
	}
	if w.Effort != "" {
		args = append(args, "--effort", w.Effort)
	}
	return append(args, w.ExtraArgs...)
}

func applierFlags(w config.Worker, f ApplierFiles) []string {
	var args []string
	if w.PermissionMode != "" {
		args = append(args, "--permission-mode", w.PermissionMode)
	}
	if f.Settings != "" {
		args = append(args, "--settings", f.Settings)
	}
	if f.Rules != "" {
		args = append(args, "--append-system-prompt-file", f.Rules)
	}
	return args
}

func (c *Claude) Interactive(w config.Worker, _ string, f ApplierFiles, session, kickoff string) []string {
	argv := append([]string{c.Bin}, applierFlags(w, f)...)
	if session != "" {
		argv = append(argv, "--session-id", session)
	}
	argv = append(argv, claudeTuning(w)...)
	return append(argv, kickoff)
}

// sessionArgs creates t.Session (a fresh one when empty) or resumes it.
func sessionArgs(t *Turn) []string {
	if t.Resume && t.Session != "" {
		return []string{"--resume", t.Session}
	}
	if t.Session == "" {
		t.Session = NewSessionID()
	}
	return []string{"--session-id", t.Session}
}

var printFlags = []string{"-p", "--output-format", "stream-json", "--verbose"}

// run streams one print-mode turn and returns its result event, plus the
// session: t.Session once claude confirmed it (an init or result event, or
// a resume), "" when claude never got that far.
func (c *Claude) run(ctx context.Context, t Turn, args []string, term io.Writer) (event, string, error) {
	var result event
	confirmed := t.Resume
	observe := func(ev event) {
		switch {
		case ev["type"] == "result":
			result, confirmed = ev, true
		case ev["type"] == "system" && ev["subtype"] == "init":
			confirmed = true
		}
	}
	err := c.rt.stream(ctx, c.Bin, args, t, term, renderClaude, observe)
	session := ""
	if confirmed {
		session = t.Session
	}
	if err != nil {
		return nil, session, fmt.Errorf("claude failed: %w", err)
	}
	if result == nil {
		return nil, session, errors.New("claude failed: no result event")
	}
	if isErr, _ := result["is_error"].(bool); isErr {
		return nil, session, fmt.Errorf("claude failed: %s", str(result, "result"))
	}
	return result, session, nil
}

func (c *Claude) Headless(ctx context.Context, t Turn, f ApplierFiles, term io.Writer) (string, error) {
	args := append(append([]string{}, printFlags...), applierFlags(t.Worker, f)...)
	args = append(args, claudeTuning(t.Worker)...)
	args = append(args, sessionArgs(&t)...)
	_, session, err := c.run(ctx, t, args, term)
	return session, err
}

// ReviewerTools is the claude reviewer's tool allowlist from the embedded asset.
func ReviewerTools() []string {
	var out []string
	for _, l := range strings.Split(assets.ReviewerTools, "\n") {
		if l = strings.TrimSpace(l); l != "" && !strings.HasPrefix(l, "#") {
			out = append(out, l)
		}
	}
	return out
}

// reviewerToolNames are the distinct tool names of ReviewerTools ("Bash" for
// every Bash(...) rule), the only tools a claude reviewer has at all.
func reviewerToolNames() []string {
	var out []string
	for _, t := range ReviewerTools() {
		name, _, _ := strings.Cut(t, "(")
		if !slices.Contains(out, name) {
			out = append(out, name)
		}
	}
	return out
}

func (c *Claude) Review(ctx context.Context, t Turn, term io.Writer) (string, error) {
	// --restricted ignores the user's and project's settings, so no inherited
	// allow rule widens the reviewer; --tools leaves only the reading tools and
	// Bash, which dontAsk limits to the allowlisted read-only git commands.
	args := append(append([]string{}, printFlags...),
		"--json-schema", claudeReviewSchema(),
		"--restricted", "--strict-mcp-config", "--tools", strings.Join(reviewerToolNames(), ","),
		"--permission-mode", "dontAsk", "--allowedTools", strings.Join(ReviewerTools(), ","))
	args = append(args, claudeTuning(t.Worker)...)
	args = append(args, sessionArgs(&t)...)
	result, session, err := c.run(ctx, t, args, term)
	if err != nil {
		return session, err
	}
	so, ok := result["structured_output"]
	if !ok || so == nil {
		return session, errors.New("claude failed: no structured output in the result")
	}
	b, err := json.MarshalIndent(so, "", "  ")
	if err != nil {
		return session, err
	}
	return session, fsx.ReplaceFile(filepath.Join(t.Dir, OutputFile), append(b, '\n'), 0o644)
}

// claudeReviewSchema is the review schema without "$schema": claude's
// validator does not know the 2020-12 meta-schema and rejects the flag.
func claudeReviewSchema() string {
	var m map[string]any
	if err := json.Unmarshal(assets.ReviewSchema, &m); err != nil {
		panic("embedded review schema: " + err.Error())
	}
	delete(m, "$schema")
	b, err := json.Marshal(m)
	if err != nil {
		panic(err)
	}
	return string(b)
}

// renderClaude turns a claude stream-json event into display lines.
func renderClaude(ev event) []string {
	switch ev["type"] {
	case "assistant":
		msg, _ := ev["message"].(map[string]any)
		parts, _ := msg["content"].([]any)
		var out []string
		for _, p := range parts {
			part, _ := p.(map[string]any)
			switch part["type"] {
			case "text":
				if s := str(part, "text"); s != "" {
					out = append(out, s)
				}
			case "tool_use":
				name := str(part, "name")
				switch name {
				case "StructuredOutput":
				case "Bash":
					out = append(out, "$ "+str(part, "input", "command"))
				default:
					line := name
					for _, k := range []string{"file_path", "pattern", "path", "url"} {
						if v := str(part, "input", k); v != "" {
							line += " " + v
							break
						}
					}
					out = append(out, line)
				}
			}
		}
		return out
	case "result":
		if isErr, _ := ev["is_error"].(bool); isErr {
			return []string{"error: " + str(ev, "result")}
		}
	}
	return nil
}
