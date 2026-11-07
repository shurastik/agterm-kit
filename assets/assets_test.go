package assets

import (
	"strings"
	"testing"
	"text/template"
)

func TestPlanReviewPromptParses(t *testing.T) {
	if _, err := template.New("plan").Parse(PlanReviewPrompt); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"<role>", "<finding_bar>", "<untrusted_input>", "<output_contract>", "<change_file", "<main_spec", "Do not run `agk`"} {
		if !strings.Contains(PlanReviewPrompt, want) {
			t.Errorf("plan review prompt lacks %q", want)
		}
	}
}
