package review

import (
	"fmt"
	"strings"
	"text/template"
	"unicode/utf8"

	"github.com/shurastik/agterm-kit/assets"
)

// MaxPromptDiffBytes caps the diff and untracked content inlined in the prompt.
// Past it, codex is told to read the rest itself.
const MaxPromptDiffBytes = 400 << 10

var (
	promptTmpl     = template.Must(template.New("prompt").Parse(assets.ReviewPrompt))
	planPromptTmpl = template.Must(template.New("plan").Parse(assets.PlanReviewPrompt))
	rereviewTmpl   = template.Must(template.New("rereview").Parse(assets.RereviewPrompt))
	fixupTmpl      = template.Must(template.New("fixup").Parse(assets.ReviewFixupPrompt))
)

// FixupPrompt renders the instruction typed to an interactive reviewer whose
// round k output failed validation with verr: rewrite out against schema.
func FixupPrompt(k int, out, schema string, verr error) (string, error) {
	msg := strings.ReplaceAll(escapeInvalidUTF8(verr.Error()), "</validator_error>", `<\/validator_error>`)
	var b strings.Builder
	if err := fixupTmpl.Execute(&b, struct {
		Round              int
		Out, Schema, Error string
	}{k, out, schema, msg}); err != nil {
		return "", err
	}
	return escapeInvalidUTF8(b.String()), nil
}

// omittedPastCap marks a file left out of the prompt to fit the size cap.
const omittedPastCap = "past the prompt size cap, read it directly"

// Output says how the reviewer delivers its result. The zero value is the
// inline review: the reply itself is the JSON object, constrained by the
// harness. An interactive reviewer cannot enforce a schema, so it writes the
// object to File instead.
type Output struct {
	Schema string // absolute path of the review schema file
	File   string // absolute path of this round's output file; "" means inline
	Root   string // repository root
	GitC   bool   // the reviewer's working directory is not the repository: git -C Root
}

// gitRoot is the root git commands must name with -C, or "".
func (o Output) gitRoot() string {
	if o.File != "" && o.GitC {
		return o.Root
	}
	return ""
}

// RereviewPrompt renders the prompt for re-review round k of s, with the
// author's optional note.
func RereviewPrompt(s Scope, k int, note string, o Output) (string, error) {
	body, err := Prompt(s, o)
	if err != nil {
		return "", err
	}
	return Rereview(body, k, note, o)
}

// Rereview wraps body, a rendered first-round prompt of either mode, for
// re-review round k with the author's optional note.
func Rereview(body string, k int, note string, o Output) (string, error) {
	var b strings.Builder
	if err := rereviewTmpl.Execute(&b, struct {
		Round        int
		Note, Review string
		Output       Output
	}{k, strings.TrimSpace(escapeInvalidUTF8(note)), body, o}); err != nil {
		return "", err
	}
	return b.String(), nil
}

type promptData struct {
	Base, MergeBase, Stat, Diff string
	ReadRest                    string
	Truncated                   bool
	Untracked                   []UntrackedFile
	Output                      Output
}

// Prompt renders the review prompt for s.
func Prompt(s Scope, o Output) (string, error) {
	// Escape before budgeting: each invalid byte grows to four characters.
	d := promptData{Base: s.Base, MergeBase: s.MergeBase, Stat: escapeInvalidUTF8(s.Stat),
		ReadRest: ReadRestCommand(s.MergeBase, o.gitRoot()), Output: o}
	budget := MaxPromptDiffBytes
	d.Diff, d.Truncated = cutLines(escapeInvalidUTF8(s.Diff), budget)
	budget -= len(d.Diff)
	for _, u := range s.Untracked {
		if u.Content != "" {
			if d.Truncated || len(u.Content) > budget {
				d.Truncated = true
				u.Content, u.Omitted = "", omittedPastCap
			} else {
				budget -= len(u.Content)
			}
		}
		d.Untracked = append(d.Untracked, u)
	}
	var b strings.Builder
	if err := promptTmpl.Execute(&b, d); err != nil {
		return "", err
	}
	return escapeInvalidUTF8(b.String()), nil // safety net for any other field
}

type planPromptData struct {
	Change           string
	Truncated        bool
	Files, MainSpecs []UntrackedFile
	Output           Output
}

// PlanPrompt renders the plan review prompt for s. File contents share the
// MaxPromptDiffBytes budget in order, change files first; past it a file is
// listed for the reviewer to read directly.
func PlanPrompt(s PlanScope, o Output) (string, error) {
	d := planPromptData{Change: s.Change, Output: o}
	budget := MaxPromptDiffBytes
	fit := func(fs []UntrackedFile) []UntrackedFile {
		var out []UntrackedFile
		for _, u := range fs {
			// Escape before budgeting: each invalid byte grows to four characters.
			u.Content = escapeInvalidUTF8(u.Content)
			if u.Content != "" {
				if d.Truncated || len(u.Content) > budget {
					d.Truncated = true
					u.Content, u.Omitted = "", omittedPastCap
				} else {
					budget -= len(u.Content)
				}
			}
			out = append(out, u)
		}
		return out
	}
	d.Files = fit(s.Files)
	d.MainSpecs = fit(s.MainSpecs)
	var b strings.Builder
	if err := planPromptTmpl.Execute(&b, d); err != nil {
		return "", err
	}
	return escapeInvalidUTF8(b.String()), nil
}

// escapeInvalidUTF8 shows bytes that are not valid UTF-8 as \xNN: codex
// refuses a prompt that is not valid UTF-8, and a diff of a legacy-encoded
// file would otherwise sink the whole review.
func escapeInvalidUTF8(s string) string {
	if utf8.ValidString(s) {
		return s
	}
	var b strings.Builder
	for len(s) > 0 {
		r, size := utf8.DecodeRuneInString(s)
		if r == utf8.RuneError && size == 1 {
			fmt.Fprintf(&b, `\x%02x`, s[0])
		} else {
			b.WriteString(s[:size])
		}
		s = s[size:]
	}
	return b.String()
}

// cutLines returns the longest prefix of s within max bytes ending on a line break.
func cutLines(s string, max int) (string, bool) {
	if len(s) <= max {
		return s, false
	}
	cut := strings.LastIndexByte(s[:max], '\n')
	return s[:cut+1], true
}
