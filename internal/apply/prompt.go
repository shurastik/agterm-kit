// Package apply holds the applier's prompts, its round report and the
// decision of what an applier wait reports.
package apply

import (
	"strings"
	"text/template"

	"github.com/shurastik/agterm-kit/assets"
)

var (
	firstTmpl = template.Must(template.New("apply").Parse(assets.ApplyPrompt))
	fixTmpl   = template.Must(template.New("fix").Parse(assets.FixPrompt))
)

// Rules are the applier's standing rules.
func Rules() string { return assets.ApplierRules }

// ClaudeSettings is the claude applier's settings layer (deny rules).
func ClaudeSettings() []byte { return assets.ClaudeApplierSettings }

func render(t *template.Template, data any, inlineRules bool) (string, error) {
	var b strings.Builder
	if inlineRules {
		b.WriteString(Rules())
		b.WriteString("\n---\n\n")
	}
	if err := t.Execute(&b, data); err != nil {
		return "", err
	}
	return b.String(), nil
}

// FirstPrompt is round 1: implement change, report to report. inlineRules
// prepends the rules, for a harness without a system-prompt flag (codex).
func FirstPrompt(change, report string, inlineRules bool) (string, error) {
	return render(firstTmpl, struct{ Change, Report string }{change, report}, inlineRules)
}

// FixPrompt is round k: address message, report to report.
func FixPrompt(k int, message, report string, inlineRules bool) (string, error) {
	return render(fixTmpl, struct {
		Round           int
		Message, Report string
	}{k, strings.TrimSpace(message), report}, inlineRules)
}
