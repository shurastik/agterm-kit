package apply

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/shurastik/agterm-kit/internal/agterm"
)

// ReportFile is the per-round report the applier writes.
const ReportFile = "report.json"

// Artifacts a blocked report may point at.
var Artifacts = []string{"proposal", "specs", "design", "tasks", "finding"}

// Report is a validated round report.
type Report struct {
	Status   string `json:"status"`
	Progress string `json:"progress,omitempty"`
	Summary  string `json:"summary"`
	Problem  string `json:"problem,omitempty"`
	Artifact string `json:"artifact,omitempty"`
}

// ParseReport validates the applier's report.json.
func ParseReport(b []byte) (Report, error) {
	var r Report
	if err := json.Unmarshal(b, &r); err != nil {
		return Report{}, fmt.Errorf("report is not a JSON object with string fields: %v", err)
	}
	switch {
	case r.Status != "done" && r.Status != "blocked":
		return Report{}, fmt.Errorf("report status %q is not done or blocked", r.Status)
	case r.Summary == "":
		return Report{}, errors.New("report has no summary")
	case r.Status == "blocked" && r.Problem == "":
		return Report{}, errors.New("blocked report has no problem")
	case r.Status == "blocked" && !slices.Contains(Artifacts, r.Artifact):
		return Report{}, fmt.Errorf("blocked report artifact %q is not one of %v", r.Artifact, Artifacts)
	}
	return r, nil
}

// Outcome is what `agk apply wait` prints.
type Outcome struct {
	Status   string `json:"status"`
	Round    int    `json:"round"`
	Reason   string `json:"reason,omitempty"`
	Progress string `json:"progress,omitempty"`
	Summary  string `json:"summary,omitempty"`
	Problem  string `json:"problem,omitempty"`
	Artifact string `json:"artifact,omitempty"`
}

// Decide maps round k's report (nil when absent), the companion's matched
// status and the wait error (nil, agterm.ErrGone or agterm.ErrTimeout) to an
// outcome. A report, when present, always decides.
func Decide(k int, report []byte, status string, waitErr error) Outcome {
	if report != nil {
		r, err := ParseReport(report)
		if err != nil {
			return Outcome{Status: "needs-human", Round: k, Reason: "invalid-report"}
		}
		return Outcome{Status: r.Status, Round: k, Progress: r.Progress, Summary: r.Summary, Problem: r.Problem, Artifact: r.Artifact}
	}
	switch {
	case errors.Is(waitErr, agterm.ErrGone):
		return Outcome{Status: "cancelled", Round: k}
	case errors.Is(waitErr, agterm.ErrTimeout):
		return Outcome{Status: "running", Round: k}
	case status == "blocked":
		return Outcome{Status: "needs-human", Round: k, Reason: "permission"}
	case status == "completed" || status == "idle":
		return Outcome{Status: "needs-human", Round: k, Reason: "no-report"}
	}
	return Outcome{Status: "running", Round: k}
}
