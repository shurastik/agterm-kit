package apply

import "testing"

func TestParseApplyState(t *testing.T) {
	st, msg, err := parseApplyState([]byte(`{"changeName":"x","state":"blocked","instruction":"Missing: specs"}`))
	if err != nil || st != "blocked" || msg != "Missing: specs" {
		t.Fatalf("%q %q %v", st, msg, err)
	}
	if _, _, err := parseApplyState([]byte(`nope`)); err == nil {
		t.Fatal("bad json")
	}
	if _, _, err := parseApplyState([]byte(`{}`)); err == nil {
		t.Fatal("no state")
	}
}
