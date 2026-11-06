package termsafe

import (
	"bytes"
	"strings"
	"testing"
)

func TestWriterNeutralizesAcrossWrites(t *testing.T) {
	var b bytes.Buffer
	w := NewWriter(&b)
	chunks := []string{
		"ok\tline\n",
		"\x1b]52;c;dGVz", "dA==\x07\n", // OSC 52 split across writes
		"\x1b[2J",
		"c1:\xc2", "\x9b31m\n", // U+009B (C1 CSI) split inside its UTF-8 encoding
		"bidi:\u202e\n",
		"bad:\x9b\n", // lone C1 byte, invalid UTF-8
		"héllo\n",
	}
	for _, c := range chunks {
		if n, err := w.Write([]byte(c)); err != nil || n != len(c) {
			t.Fatalf("Write(%q) = %d, %v", c, n, err)
		}
	}
	out := b.String()
	for _, raw := range []byte{0x1b, 0x07, 0x9b} {
		if bytes.IndexByte(b.Bytes(), raw) >= 0 {
			t.Errorf("raw byte %#x reached the terminal: %q", raw, out)
		}
	}
	if strings.ContainsRune(out, '\u202e') || strings.ContainsRune(out, '\u009b') {
		t.Errorf("unsafe rune reached the terminal: %q", out)
	}
	want := "ok\tline\n" +
		`\u001b]52;c;dGVzdA==\u0007` + "\n" +
		`\u001b[2J` + `c1:\u009b31m` + "\n" +
		`bidi:\u202e` + "\n" +
		`bad:\x9b` + "\n" +
		"héllo\n"
	if out != want {
		t.Fatalf("got\n%q\nwant\n%q", out, want)
	}
}

func TestUnsafe(t *testing.T) {
	for _, r := range []rune{0x00, 0x07, 0x1b, 0x7f, 0x85, 0x9b, 0x202a, 0x202e, 0x2066, 0x2069, 0x2028, 0x2029} {
		if !Unsafe(r) {
			t.Errorf("%U should be unsafe", r)
		}
	}
	for _, r := range []rune{'a', ' ', 'é', 'я', 0x2027, 0x202f} {
		if Unsafe(r) {
			t.Errorf("%U should be safe", r)
		}
	}
}
