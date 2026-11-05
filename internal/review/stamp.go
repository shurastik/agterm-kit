package review

import (
	"bytes"
	"io"
	"time"
)

// StampFormat is the local date and time put in front of every log line.
const StampFormat = "2006-01-02 15:04:05"

// StampWriter prefixes every line written through it with "[date time] ".
// Lines may arrive split across writes; the stamp is taken when a line starts.
type StampWriter struct {
	w       io.Writer
	now     func() time.Time
	midLine bool
}

// NewStampWriter wraps w; now supplies the clock.
func NewStampWriter(w io.Writer, now func() time.Time) *StampWriter {
	return &StampWriter{w: w, now: now}
}

func (s *StampWriter) Write(p []byte) (int, error) {
	var out bytes.Buffer
	for rest := p; len(rest) > 0; {
		if !s.midLine {
			out.WriteString("[" + s.now().Format(StampFormat) + "] ")
			s.midLine = true
		}
		i := bytes.IndexByte(rest, '\n')
		if i < 0 {
			out.Write(rest)
			break
		}
		out.Write(rest[:i+1])
		rest = rest[i+1:]
		s.midLine = false
	}
	if _, err := s.w.Write(out.Bytes()); err != nil {
		return 0, err
	}
	return len(p), nil
}
