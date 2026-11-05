package triage

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/shurastik/agterm-kit/internal/review"
)

// MaxNoteRunes caps a decision note.
const MaxNoteRunes = 2000

// Decision is the user's call on one finding. Title and File are copied from
// the findings file, never from the page.
type Decision struct {
	ID       int    `json:"id"`
	Title    string `json:"title"`
	File     string `json:"file"`
	Decision string `json:"decision"`
	Note     string `json:"note"`
}

type submitted struct {
	ID       *int    `json:"id"`
	Decision *string `json:"decision"`
	Note     *string `json:"note"`
}

// Decide validates the value a triage page submitted against r: exactly one
// entry per finding id 1..N with a known decision. Results are in id order.
func Decide(r review.Review, value string) ([]Decision, error) {
	dec := json.NewDecoder(strings.NewReader(value))
	dec.DisallowUnknownFields()
	var in []submitted
	if err := dec.Decode(&in); err != nil {
		return nil, fmt.Errorf("invalid triage result: %w", err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("invalid triage result: trailing data")
	}
	n := len(r.Findings)
	out := make([]Decision, n)
	seen := make([]bool, n)
	for _, s := range in {
		if s.ID == nil || s.Decision == nil || s.Note == nil {
			return nil, errors.New("invalid triage result: entry needs id, decision and note")
		}
		id := *s.ID
		if id < 1 || id > n {
			return nil, fmt.Errorf("invalid triage result: no finding #%d", id)
		}
		if seen[id-1] {
			return nil, fmt.Errorf("invalid triage result: finding #%d decided twice", id)
		}
		if !slices.Contains(Decisions, *s.Decision) {
			return nil, fmt.Errorf("invalid triage result: finding #%d: decision %q is not one of %v", id, *s.Decision, Decisions)
		}
		seen[id-1] = true
		f := r.Findings[id-1]
		out[id-1] = Decision{ID: id, Title: f.Title, File: f.File, Decision: *s.Decision, Note: capNote(*s.Note)}
	}
	for i, ok := range seen {
		if !ok {
			return nil, fmt.Errorf("invalid triage result: finding #%d has no decision", i+1)
		}
	}
	return out, nil
}

func capNote(s string) string {
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > MaxNoteRunes {
		s = string(r[:MaxNoteRunes])
	}
	return s
}
