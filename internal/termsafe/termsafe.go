// Package termsafe keeps untrusted text from driving a terminal: control
// characters and bidi overrides are shown as visible escapes instead.
package termsafe

import (
	"bytes"
	"fmt"
	"io"
	"unicode/utf8"
)

// Unsafe reports whether r must not reach a terminal raw: C0 controls (the
// caller decides about '\n' and '\t'), DEL, C1 controls, bidi embeddings,
// overrides and isolates, and the Unicode line/paragraph separators.
func Unsafe(r rune) bool {
	switch {
	case r < 0x20, r == 0x7f, r >= 0x80 && r <= 0x9f:
		return true
	case r >= 0x202a && r <= 0x202e, r >= 0x2066 && r <= 0x2069, r == 0x2028, r == 0x2029:
		return true
	}
	return false
}

// Writer escapes unsafe characters (keeping '\n' and '\t') on the way to w.
// An incomplete UTF-8 sequence at the end of a write is held for the next one,
// so a sequence split across writes is judged whole.
type Writer struct {
	w       io.Writer
	pending []byte
}

// NewWriter wraps w.
func NewWriter(w io.Writer) *Writer { return &Writer{w: w} }

func (s *Writer) Write(p []byte) (int, error) {
	buf := append(s.pending, p...)
	s.pending = nil
	var out bytes.Buffer
	for len(buf) > 0 {
		r, size := utf8.DecodeRune(buf)
		if r == utf8.RuneError && size <= 1 {
			if !utf8.FullRune(buf) {
				s.pending = append([]byte(nil), buf...) // wait for the rest
				break
			}
			fmt.Fprintf(&out, `\x%02x`, buf[0])
			buf = buf[1:]
			continue
		}
		if r != '\n' && r != '\t' && Unsafe(r) {
			fmt.Fprintf(&out, `\u%04x`, r)
		} else {
			out.Write(buf[:size])
		}
		buf = buf[size:]
	}
	if _, err := s.w.Write(out.Bytes()); err != nil {
		return 0, err
	}
	return len(p), nil
}
