// Package review runs a codex review of a git diff and validates its output.
package review

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
)

// Verdicts and Severities mirror the enums of assets/review-output.schema.json.
var (
	Verdicts   = []string{"approve", "needs-attention"}
	Severities = []string{"critical", "high", "medium", "low"}
)

// Finding is one review finding.
type Finding struct {
	Severity       string  `json:"severity"`
	Title          string  `json:"title"`
	Body           string  `json:"body"`
	File           string  `json:"file"`
	LineStart      int     `json:"line_start"`
	LineEnd        int     `json:"line_end"`
	Confidence     float64 `json:"confidence"`
	Recommendation string  `json:"recommendation"`
}

// Review is the structured codex review output.
type Review struct {
	Verdict   string    `json:"verdict"`
	Summary   string    `json:"summary"`
	Findings  []Finding `json:"findings"`
	NextSteps []string  `json:"next_steps"`
}

// Pointer fields let Parse tell a missing field from a zero value.
type rawFinding struct {
	Severity       *string  `json:"severity"`
	Title          *string  `json:"title"`
	Body           *string  `json:"body"`
	File           *string  `json:"file"`
	LineStart      *int     `json:"line_start"`
	LineEnd        *int     `json:"line_end"`
	Confidence     *float64 `json:"confidence"`
	Recommendation *string  `json:"recommendation"`
}

type rawReview struct {
	Verdict   *string       `json:"verdict"`
	Summary   *string       `json:"summary"`
	Findings  *[]rawFinding `json:"findings"`
	NextSteps *[]string     `json:"next_steps"`
}

// Parse decodes and validates codex output against the review schema.
func Parse(data []byte) (Review, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var raw rawReview
	if err := dec.Decode(&raw); err != nil {
		return Review{}, fmt.Errorf("invalid review output: %w", err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return Review{}, errors.New("invalid review output: trailing data after JSON object")
	}
	v := validator{}
	r := Review{
		Verdict: v.enum("verdict", raw.Verdict, Verdicts),
		Summary: v.text("summary", raw.Summary),
	}
	if raw.Findings == nil {
		v.fail("findings is required")
	} else {
		r.Findings = make([]Finding, 0, len(*raw.Findings))
		for i, f := range *raw.Findings {
			p := fmt.Sprintf("findings[%d].", i)
			r.Findings = append(r.Findings, Finding{
				Severity:       v.enum(p+"severity", f.Severity, Severities),
				Title:          v.text(p+"title", f.Title),
				Body:           v.text(p+"body", f.Body),
				File:           v.text(p+"file", f.File),
				LineStart:      v.line(p+"line_start", f.LineStart),
				LineEnd:        v.line(p+"line_end", f.LineEnd),
				Confidence:     v.confidence(p+"confidence", f.Confidence),
				Recommendation: v.present(p+"recommendation", f.Recommendation),
			})
		}
	}
	if raw.NextSteps == nil {
		v.fail("next_steps is required")
	} else {
		r.NextSteps = make([]string, 0, len(*raw.NextSteps))
		for i, s := range *raw.NextSteps {
			r.NextSteps = append(r.NextSteps, v.text(fmt.Sprintf("next_steps[%d]", i), &s))
		}
	}
	if v.err != nil {
		return Review{}, fmt.Errorf("invalid review output: %w", v.err)
	}
	return r, nil
}

// validator keeps the first failure.
type validator struct{ err error }

func (v *validator) fail(format string, args ...any) {
	if v.err == nil {
		v.err = fmt.Errorf(format, args...)
	}
}

func (v *validator) present(name string, s *string) string {
	if s == nil {
		v.fail("%s is required", name)
		return ""
	}
	return *s
}

func (v *validator) text(name string, s *string) string {
	out := v.present(name, s)
	if s != nil && out == "" {
		v.fail("%s must not be empty", name)
	}
	return out
}

func (v *validator) enum(name string, s *string, allowed []string) string {
	out := v.present(name, s)
	if s != nil && !slices.Contains(allowed, out) {
		v.fail("%s %q is not one of %v", name, out, allowed)
	}
	return out
}

func (v *validator) line(name string, n *int) int {
	if n == nil {
		v.fail("%s is required", name)
		return 0
	}
	if *n < 1 {
		v.fail("%s must be >= 1, got %d", name, *n)
	}
	return *n
}

func (v *validator) confidence(name string, f *float64) float64 {
	if f == nil {
		v.fail("%s is required", name)
		return 0
	}
	if *f < 0 || *f > 1 {
		v.fail("%s must be within [0,1], got %v", name, *f)
	}
	return *f
}
