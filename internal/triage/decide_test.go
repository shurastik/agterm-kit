package triage

import (
	"reflect"
	"strings"
	"testing"
)

func TestDecideValid(t *testing.T) {
	r := sample()
	got, err := Decide(r, `[{"id":2,"decision":"discuss","note":"  why not a mutex?  "},{"id":1,"decision":"fix","note":""}]`)
	if err != nil {
		t.Fatal(err)
	}
	want := []Decision{
		{ID: 1, Title: "Race on cache map", File: "internal/cache.go", Decision: "fix", Note: ""},
		{ID: 2, Title: "Typo in log", File: "main.go", Decision: "discuss", Note: "why not a mutex?"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v", got)
	}
}

func TestDecideTitleAndFileComeFromFindings(t *testing.T) {
	got, err := Decide(sample(), `[{"id":1,"decision":"fix","note":"","title":"forged"},{"id":2,"decision":"skip","note":""}]`)
	if err == nil {
		t.Fatalf("unknown fields must be rejected, got %+v", got)
	}
	got, err = Decide(sample(), `[{"id":1,"decision":"fix","note":""},{"id":2,"decision":"skip","note":""}]`)
	if err != nil || got[0].Title != "Race on cache map" || got[1].File != "main.go" {
		t.Fatalf("got %+v err %v", got, err)
	}
}

func TestDecideNoteCapped(t *testing.T) {
	long := strings.Repeat("é", MaxNoteRunes+10)
	got, err := Decide(sample(), `[{"id":1,"decision":"fix","note":"`+long+`"},{"id":2,"decision":"skip","note":""}]`)
	if err != nil {
		t.Fatal(err)
	}
	if n := len([]rune(got[0].Note)); n != MaxNoteRunes {
		t.Fatalf("note runes = %d", n)
	}
}

func TestDecideInvalid(t *testing.T) {
	cases := map[string]string{
		"not json":        `nope`,
		"object":          `{"id":1}`,
		"missing":         `[{"id":1,"decision":"fix","note":""}]`,
		"unknown id":      `[{"id":1,"decision":"fix","note":""},{"id":3,"decision":"fix","note":""}]`,
		"zero id":         `[{"id":0,"decision":"fix","note":""},{"id":1,"decision":"fix","note":""},{"id":2,"decision":"fix","note":""}]`,
		"duplicate":       `[{"id":1,"decision":"fix","note":""},{"id":1,"decision":"skip","note":""}]`,
		"bad decision":    `[{"id":1,"decision":"ignore","note":""},{"id":2,"decision":"fix","note":""}]`,
		"empty decision":  `[{"id":1,"decision":"","note":""},{"id":2,"decision":"fix","note":""}]`,
		"note not string": `[{"id":1,"decision":"fix","note":5},{"id":2,"decision":"fix","note":""}]`,
		"trailing":        `[{"id":1,"decision":"fix","note":""},{"id":2,"decision":"fix","note":""}] x`,
	}
	for name, v := range cases {
		t.Run(name, func(t *testing.T) {
			if got, err := Decide(sample(), v); err == nil {
				t.Fatalf("want error, got %+v", got)
			}
		})
	}
}
