// Package triage renders review findings for per-finding decisions and turns
// the submitted result into decisions for the agent.
package triage

import (
	"bytes"
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"fmt"
	"html/template"
	"math"
	"regexp"

	"github.com/shurastik/agterm-kit/internal/review"
)

// Decisions a user can give a finding.
var Decisions = []string{"fix", "skip", "discuss"}

//go:embed page.html
var pageSrc string

var pageTmpl = template.Must(template.New("page").Funcs(template.FuncMap{
	"inc":       func(i int) int { return i + 1 },
	"pct":       func(f float64) string { return fmt.Sprintf("%d%%", int(math.Round(f*100))) },
	"loc":       location,
	"decisions": func() []string { return Decisions },
	"default":   DefaultDecision,
}).Parse(pageSrc))

var nonceRe = regexp.MustCompile(`^[A-Za-z0-9]+$`)

// NewNonce returns a random CSP nonce.
func NewNonce() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err) // crypto/rand never fails on supported platforms
	}
	return hex.EncodeToString(b)
}

// DefaultDecision is the preselected decision for a finding.
func DefaultDecision(f review.Finding) string {
	if f.Severity == "low" {
		return "skip"
	}
	return "fix"
}

func location(f review.Finding) string {
	if f.LineEnd <= f.LineStart {
		return fmt.Sprintf("%s:%d", f.File, f.LineStart)
	}
	return fmt.Sprintf("%s:%d-%d", f.File, f.LineStart, f.LineEnd)
}

// Render returns the self-contained triage page for r. All finding text is
// contextually escaped; the only script is static and carries nonce.
func Render(r review.Review, nonce string) ([]byte, error) {
	if !nonceRe.MatchString(nonce) {
		return nil, fmt.Errorf("invalid nonce %q", nonce)
	}
	// html/template refuses dynamic values in <meta content>, and the nonce is
	// alphanumeric, so the CSP tag is assembled here.
	csp := template.HTML(`<meta http-equiv="Content-Security-Policy" content="default-src 'none'; script-src 'nonce-` +
		nonce + `'; style-src 'unsafe-inline'; img-src 'none'; form-action 'none'">`)
	var b bytes.Buffer
	err := pageTmpl.Execute(&b, struct {
		review.Review
		CSP   template.HTML
		Nonce string
	}{r, csp, nonce})
	if err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}
