package harness

import (
	"bytes"
	"testing"
)

func TestEventWriterSplitsAcrossWritesAndPassesNonJSON(t *testing.T) {
	var raw, out bytes.Buffer
	var seen []string
	w := &eventWriter{raw: &raw, out: &out, render: renderCodex, observe: func(ev event) { seen = append(seen, str(ev, "type")) }}
	stream := `{"type":"item.completed","item":{"type":"agent_message","text":"hi"}}` + "\n" +
		"{not json\n" +
		`{"type":"mystery"}` + "\n" +
		"tail without newline"
	for i := 0; i < len(stream); i += 7 {
		end := min(i+7, len(stream))
		w.Write([]byte(stream[i:end]))
	}
	w.flush()
	if got, want := out.String(), "hi\n{not json\ntail without newline\n"; got != want {
		t.Fatalf("out = %q, want %q", got, want)
	}
	if raw.String() != stream+"\n" {
		t.Fatalf("raw = %q", raw.String())
	}
	if len(seen) != 2 || seen[0] != "item.completed" || seen[1] != "mystery" {
		t.Fatalf("observed %q", seen)
	}
}
