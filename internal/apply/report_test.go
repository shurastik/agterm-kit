package apply

import (
	"encoding/json"
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
		got := Decide(2, c.report, c.status, c.err, nil)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: %+v, want %+v", c.name, got, c.want)
		}
	}
}

func TestDecideWithTurnState(t *testing.T) {
	done := []byte(`{"status":"done","summary":"all green"}`)
	running := &Turn{Round: 2, State: TurnRunning}
	stopped0 := &Turn{Round: 2, State: TurnStopped}
	paused := &Turn{Round: 2, State: TurnStopped, Background: 2, Wakeups: 1}
	wakeups := &Turn{Round: 2, State: TurnStopped, Wakeups: 1}
	earlier := &Turn{Round: 1, State: TurnStopped}
	earlierPaused := &Turn{Round: 1, State: TurnStopped, Background: 1}
	later := &Turn{Round: 3, State: TurnStopped, Background: 1}
	noReport := Outcome{Status: "needs-human", Round: 2, Reason: "no-report"}
	pause := Outcome{Status: "running", Round: 2, Reason: "background", Background: 3}
	plain := Outcome{Status: "running", Round: 2}
	cases := []struct {
		name   string
		report []byte
		status string
		err    error
		turn   *Turn
		want   Outcome
	}{
		{"report beats a pause", done, "completed", nil, paused, Outcome{Status: "done", Round: 2, Summary: "all green"}},
		{"report beats a running turn", done, "completed", nil, running, Outcome{Status: "done", Round: 2, Summary: "all green"}},
		{"paused, status completed: keep waiting", nil, "completed", nil, paused, pause},
		{"paused, status idle: keep waiting", nil, "idle", nil, paused, pause},
		{"paused on wakeups only", nil, "completed", nil, wakeups, Outcome{Status: "running", Round: 2, Reason: "background", Background: 1}},
		{"paused, timeout", nil, "", agterm.ErrTimeout, paused, pause},
		{"paused, no status yet", nil, "", nil, paused, pause},
		{"stopped with nothing in flight, completed", nil, "completed", nil, stopped0, noReport},
		{"stopped with nothing in flight, idle", nil, "idle", nil, stopped0, noReport},
		{"stopped with nothing in flight, timeout", nil, "", agterm.ErrTimeout, stopped0, plain},
		{"running turn, status completed: the hook lags", nil, "completed", nil, running, plain},
		{"running turn, status idle", nil, "idle", nil, running, plain},
		{"running turn, timeout", nil, "", agterm.ErrTimeout, running, plain},
		{"permission while paused", nil, "blocked", nil, paused, Outcome{Status: "needs-human", Round: 2, Reason: "permission"}},
		{"permission while running", nil, "blocked", nil, running, Outcome{Status: "needs-human", Round: 2, Reason: "permission"}},
		{"gone while paused", nil, "", agterm.ErrGone, paused, Outcome{Status: "cancelled", Round: 2}},
		{"gone after stopped with nothing", nil, "", agterm.ErrGone, stopped0, Outcome{Status: "cancelled", Round: 2}},
		{"earlier round's stopped state counts as missing", nil, "completed", nil, earlier, noReport},
		{"earlier round's pause counts as missing", nil, "completed", nil, earlierPaused, noReport},
		{"earlier round's pause, timeout", nil, "", agterm.ErrTimeout, earlierPaused, plain},
		{"later round's state counts as missing", nil, "completed", nil, later, noReport},
		{"no turn state: status only", nil, "completed", nil, nil, noReport},
	}
	for _, c := range cases {
		got := Decide(2, c.report, c.status, c.err, c.turn)
		if got != c.want {
			t.Errorf("%s: %+v, want %+v", c.name, got, c.want)
		}
	}
}

func TestOutcomeJSONOfAPause(t *testing.T) {
	b, err := json.Marshal(Decide(1, nil, "", agterm.ErrTimeout, &Turn{Round: 1, State: TurnStopped, Background: 2}))
	if err != nil || string(b) != `{"status":"running","round":1,"reason":"background","background":2}` {
		t.Fatalf("%s, %v", b, err)
	}
}
