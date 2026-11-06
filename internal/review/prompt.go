package review

import (
	"strings"
	"text/template"

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
	d := promptData{Base: s.Base, MergeBase: s.MergeBase, Stat: s.Stat, ReadRest: ReadRestCommand(s.MergeBase)}
	budget := MaxPromptDiffBytes
	d.Diff, d.Truncated = cutLines(s.Diff, budget)
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
	return b.String(), nil
}

// cutLines returns the longest prefix of s within max bytes ending on a line break.
func cutLines(s string, max int) (string, bool) {
	if len(s) <= max {
		return s, false
	}
	cut := strings.LastIndexByte(s[:max], '\n')
	return s[:cut+1], true
}
