package harness

import (
	"bytes"
	"testing"
	"time"
)

func fixedNow() time.Time { return time.Date(2026, 11, 5, 17, 53, 55, 0, time.Local) }

func TestStampWriter(t *testing.T) {
	var b bytes.Buffer
	w := NewStampWriter(&b, fixedNow)
	for _, chunk := range []string{"thin", "king\nand ", "more\n\nlast", " part\n"} {
		n, err := w.Write([]byte(chunk))
		if err != nil || n != len(chunk) {
			t.Fatalf("Write(%q) = %d, %v", chunk, n, err)
		}
	}
	want := "[2026-11-05 17:53:55] thinking\n" +
		"[2026-11-05 17:53:55] and more\n" +
		"[2026-11-05 17:53:55] \n" +
		"[2026-11-05 17:53:55] last part\n"
	if b.String() != want {
		t.Fatalf("got\n%q\nwant\n%q", b.String(), want)
	}
}

func TestStampWriterNoPrefixWithoutOutput(t *testing.T) {
	var b bytes.Buffer
	w := NewStampWriter(&b, fixedNow)
	w.Write(nil)
	if b.Len() != 0 {
		t.Fatalf("empty write produced %q", b.String())
	}
}
