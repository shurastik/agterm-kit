package apply

import (
	"errors"
	"reflect"
	"testing"

	"github.com/shurastik/agterm-kit/internal/agterm"
)

func TestParseReport(t *testing.T) {
	ok := map[string]Report{
		`{"status":"done","summary":"s","progress":"5/7"}`:                              {Status: "done", Summary: "s", Progress: "5/7"},
		`{"status":"blocked","summary":"s","problem":"p","artifact":"tasks","extra":1}`: {Status: "blocked", Summary: "s", Problem: "p", Artifact: "tasks"},
	}
	for in, want := range ok {
		got, err := ParseReport([]byte(in))
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("%s: %+v, %v", in, got, err)
		}
	}
	for _, in := range []string{
		``, `nope`, `[]`, `{"status":"ok","summary":"s"}`, `{"status":"done"}`, `{"status":"done","summary":""}`,
		`{"status":"done","summary":5}`,
		`{"status":"blocked","summary":"s","artifact":"tasks"}`,
		`{"status":"blocked","summary":"s","problem":"p","artifact":"code"}`,
		`{"status":"blocked","summary":"s","problem":"p"}`,
	} {
		if _, err := ParseReport([]byte(in)); err == nil {
			t.Errorf("%q: want error", in)
		}
	}
}

func TestDecide(t *testing.T) {
	done := []byte(`{"status":"done","summary":"all green"}`)
	blocked := []byte(`{"status":"blocked","summary":"s","problem":"p","artifact":"specs"}`)
	cases := []struct {
		name   string
		report []byte
		status string
		err    error
		want   Outcome
	}{
		{"done", done, "", nil, Outcome{Status: "done", Round: 2, Summary: "all green"}},
		{"done beats a gone companion", done, "", agterm.ErrGone, Outcome{Status: "done", Round: 2, Summary: "all green"}},
		{"blocked", blocked, "completed", nil, Outcome{Status: "blocked", Round: 2, Summary: "s", Problem: "p", Artifact: "specs"}},
		{"invalid report", []byte(`{}`), "completed", nil, Outcome{Status: "needs-human", Round: 2, Reason: "invalid-report"}},
		{"permission", nil, "blocked", nil, Outcome{Status: "needs-human", Round: 2, Reason: "permission"}},
		{"turn over, no report", nil, "completed", nil, Outcome{Status: "needs-human", Round: 2, Reason: "no-report"}},
		{"idle, no report", nil, "idle", nil, Outcome{Status: "needs-human", Round: 2, Reason: "no-report"}},
		{"gone", nil, "", agterm.ErrGone, Outcome{Status: "cancelled", Round: 2}},
		{"timeout", nil, "", agterm.ErrTimeout, Outcome{Status: "running", Round: 2}},
		{"wrapped gone", nil, "", errors.Join(agterm.ErrGone, errors.New("x")), Outcome{Status: "cancelled", Round: 2}},
	}
	for _, c := range cases {
		got := Decide(2, c.report, c.status, c.err)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: %+v, want %+v", c.name, got, c.want)
		}
	}
}
