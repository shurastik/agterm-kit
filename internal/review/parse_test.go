package review

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/shurastik/agterm-kit/assets"
)

const validReview = `{
  "verdict": "needs-attention",
  "summary": "One race.",
  "findings": [{
    "severity": "high", "title": "Race on map", "body": "Concurrent writes.",
    "file": "internal/x.go", "line_start": 10, "line_end": 12,
    "confidence": 0.8, "recommendation": "Use a mutex."
  }],
  "next_steps": ["Add a lock"]
}`

func TestParseValid(t *testing.T) {
	r, err := Parse([]byte(validReview))
	if err != nil {
		t.Fatal(err)
	}
	want := Review{
		Verdict: "needs-attention", Summary: "One race.",
		Findings: []Finding{{
			Severity: "high", Title: "Race on map", Body: "Concurrent writes.",
			File: "internal/x.go", LineStart: 10, LineEnd: 12, Confidence: 0.8, Recommendation: "Use a mutex.",
		}},
		NextSteps: []string{"Add a lock"},
	}
	if !reflect.DeepEqual(r, want) {
		t.Fatalf("got %+v", r)
	}
}

func TestParseApproveEmpty(t *testing.T) {
	r, err := Parse([]byte(`{"verdict":"approve","summary":"ok","findings":[],"next_steps":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Findings) != 0 || r.Verdict != "approve" {
		t.Fatalf("got %+v", r)
	}
}

func TestParseInvalid(t *testing.T) {
	mutate := func(f func(m map[string]any)) string {
		var m map[string]any
		_ = json.Unmarshal([]byte(validReview), &m)
		f(m)
		b, _ := json.Marshal(m)
		return string(b)
	}
	finding := func(m map[string]any) map[string]any { return m["findings"].([]any)[0].(map[string]any) }
	cases := map[string]string{
		"not json":               `{"verdict":`,
		"trailing data":          validReview + `{}`,
		"array":                  `[]`,
		"bad verdict":            mutate(func(m map[string]any) { m["verdict"] = "lgtm" }),
		"missing summary":        mutate(func(m map[string]any) { delete(m, "summary") }),
		"empty summary":          mutate(func(m map[string]any) { m["summary"] = "" }),
		"null findings":          mutate(func(m map[string]any) { m["findings"] = nil }),
		"missing next_steps":     mutate(func(m map[string]any) { delete(m, "next_steps") }),
		"empty next step":        mutate(func(m map[string]any) { m["next_steps"] = []any{""} }),
		"unknown top field":      mutate(func(m map[string]any) { m["extra"] = 1 }),
		"bad severity":           mutate(func(m map[string]any) { finding(m)["severity"] = "blocker" }),
		"missing title":          mutate(func(m map[string]any) { delete(finding(m), "title") }),
		"empty body":             mutate(func(m map[string]any) { finding(m)["body"] = "" }),
		"missing recommendation": mutate(func(m map[string]any) { delete(finding(m), "recommendation") }),
		"missing confidence":     mutate(func(m map[string]any) { delete(finding(m), "confidence") }),
		"line_start zero":        mutate(func(m map[string]any) { finding(m)["line_start"] = 0 }),
		"line_end zero":          mutate(func(m map[string]any) { finding(m)["line_end"] = 0 }),
		"line fractional":        mutate(func(m map[string]any) { finding(m)["line_start"] = 1.5 }),
		"confidence > 1":         mutate(func(m map[string]any) { finding(m)["confidence"] = 1.2 }),
		"confidence < 0":         mutate(func(m map[string]any) { finding(m)["confidence"] = -0.1 }),
		"unknown finding field":  mutate(func(m map[string]any) { finding(m)["cwe"] = "x" }),
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse([]byte(in)); err == nil {
				t.Fatalf("want error for %s", in)
			}
		})
	}
}

func TestParseErrorNamesFinding(t *testing.T) {
	in := strings.Replace(validReview, `"high"`, `"blocker"`, 1)
	_, err := Parse([]byte(in))
	if err == nil || !strings.Contains(err.Error(), "findings[0].severity") {
		t.Fatalf("err = %v", err)
	}
}

// The Go-side enums must match the embedded schema so they cannot drift.
func TestEnumsMatchSchema(t *testing.T) {
	var s struct {
		Properties struct {
			Verdict  struct{ Enum []string } `json:"verdict"`
			Findings struct {
				Items struct {
					Properties struct {
						Severity struct{ Enum []string } `json:"severity"`
					} `json:"properties"`
					Required []string `json:"required"`
				} `json:"items"`
			} `json:"findings"`
		} `json:"properties"`
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(assets.ReviewSchema, &s); err != nil {
		t.Fatal(err)
	}
	same := func(name string, a, b []string) {
		a, b = append([]string(nil), a...), append([]string(nil), b...)
		sort.Strings(a)
		sort.Strings(b)
		if !reflect.DeepEqual(a, b) {
			t.Errorf("%s: schema %v, Go %v", name, a, b)
		}
	}
	same("verdict", s.Properties.Verdict.Enum, Verdicts)
	same("severity", s.Properties.Findings.Items.Properties.Severity.Enum, Severities)
	same("required", s.Required, []string{"verdict", "summary", "findings", "next_steps"})
	same("finding required", s.Properties.Findings.Items.Required,
		[]string{"severity", "title", "body", "file", "line_start", "line_end", "confidence", "recommendation"})
}
