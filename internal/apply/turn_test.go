package apply

import (
	"reflect"
	"testing"
)

func TestTurnFromStop(t *testing.T) {
	// The spike's inputs (claude 2.1.325): paused on a shell, then the final Stop.
	paused := `{"session_id":"s","hook_event_name":"Stop","stop_hook_active":false,
	"background_tasks":[{"id":"b1","type":"shell","status":"running","description":"d","command":"sleep 60"}],
	"session_crons":[]}`
	final := `{"hook_event_name":"Stop","background_tasks":[],"session_crons":[]}`
	cases := map[string]struct {
		in   string
		want Turn
	}{
		"paused on a shell":      {paused, Turn{Round: 3, State: TurnStopped, Background: 1}},
		"final stop":             {final, Turn{Round: 3, State: TurnStopped}},
		"tasks and wakeups":      {`{"background_tasks":[{},{}],"session_crons":[{"id":"c"}]}`, Turn{Round: 3, State: TurnStopped, Background: 2, Wakeups: 1}},
		"missing fields":         {`{"hook_event_name":"Stop"}`, Turn{Round: 3, State: TurnStopped}},
		"null fields":            {`{"background_tasks":null,"session_crons":null}`, Turn{Round: 3, State: TurnStopped}},
		"unknown fields ignored": {`{"x":1,"background_tasks":[{"a":[1,2]}]}`, Turn{Round: 3, State: TurnStopped, Background: 1}},
	}
	for name, c := range cases {
		got, err := TurnFromStop(3, []byte(c.in))
		if err != nil || got != c.want {
			t.Errorf("%s: %+v, %v, want %+v", name, got, err, c.want)
		}
	}
	for _, in := range []string{``, `nope`, `[]`, `{"background_tasks":"x"}`, `{"session_crons":{}}`, `{"background_tasks":[1`} {
		if _, err := TurnFromStop(3, []byte(in)); err == nil {
			t.Errorf("%q: want error", in)
		}
	}
}

func TestTurnInFlight(t *testing.T) {
	cases := []struct {
		t        Turn
		inFlight int
		paused   bool
	}{
		{Turn{Round: 1, State: TurnRunning}, 0, false},
		{Turn{Round: 1, State: TurnStopped}, 0, false},
		{Turn{Round: 1, State: TurnStopped, Background: 2}, 2, true},
		{Turn{Round: 1, State: TurnStopped, Wakeups: 1}, 1, true},
		{Turn{Round: 1, State: TurnStopped, Background: 1, Wakeups: 2}, 3, true},
	}
	for _, c := range cases {
		if c.t.InFlight() != c.inFlight || c.t.Paused() != c.paused {
			t.Errorf("%+v: inflight %d paused %v", c.t, c.t.InFlight(), c.t.Paused())
		}
	}
}

func TestParseTurn(t *testing.T) {
	ok := map[string]Turn{
		`{"round":1,"state":"running"}`:                                  {Round: 1, State: TurnRunning},
		`{"round":2,"state":"stopped","background":1,"wakeups":2}`:       {Round: 2, State: TurnStopped, Background: 1, Wakeups: 2},
		`{"round":2,"state":"stopped","background":0,"extra":"ignored"}`: {Round: 2, State: TurnStopped},
	}
	for in, want := range ok {
		got, err := ParseTurn([]byte(in))
		if err != nil || got != want {
			t.Errorf("%s: %+v, %v", in, got, err)
		}
	}
	for _, in := range []string{
		``, `nope`, `[]`, `{}`, `{"round":0,"state":"running"}`, `{"round":-1,"state":"running"}`,
		`{"round":1}`, `{"round":1,"state":"paused"}`, `{"round":"1","state":"running"}`,
		`{"round":1,"state":"stopped","background":-1}`, `{"round":1,"state":"stopped","wakeups":-2}`,
	} {
		if _, err := ParseTurn([]byte(in)); err == nil {
			t.Errorf("%q: want error", in)
		}
	}
}

func TestTurnMarshalRoundTrip(t *testing.T) {
	for _, tn := range []Turn{
		{Round: 1, State: TurnRunning},
		{Round: 2, State: TurnStopped},
		{Round: 4, State: TurnStopped, Background: 2, Wakeups: 1},
	} {
		got, err := ParseTurn(tn.Marshal())
		if err != nil || !reflect.DeepEqual(got, tn) {
			t.Errorf("%+v: round trip %+v, %v", tn, got, err)
		}
	}
	if got := string((Turn{Round: 1, State: TurnRunning}).Marshal()); got != `{"round":1,"state":"running"}`+"\n" {
		t.Errorf("running = %q", got)
	}
}
