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

var promptTmpl = template.Must(template.New("prompt").Parse(assets.ReviewPrompt))

type promptData struct {
	Base, MergeBase, Stat, Diff string
	ReadRest                    string
	Truncated                   bool
	Untracked                   []UntrackedFile
}

// Prompt renders the codex review prompt for s.
func Prompt(s Scope) (string, error) {
	// Escape before budgeting: each invalid byte grows to four characters.
	d := promptData{Base: s.Base, MergeBase: s.MergeBase, Stat: escapeInvalidUTF8(s.Stat), ReadRest: ReadRestCommand(s.MergeBase)}
	budget := MaxPromptDiffBytes
	d.Diff, d.Truncated = cutLines(escapeInvalidUTF8(s.Diff), budget)
	budget -= len(d.Diff)
	for _, u := range s.Untracked {
		if u.Content != "" {
			if d.Truncated || len(u.Content) > budget {
				d.Truncated = true
				u.Content, u.Omitted = "", "past the prompt size cap, read it directly"
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
