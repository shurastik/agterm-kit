package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/shurastik/agterm-kit/internal/agterm"
	"github.com/shurastik/agterm-kit/internal/fsx"
	"github.com/shurastik/agterm-kit/internal/review"
	"github.com/shurastik/agterm-kit/internal/triage"
)

// Triage implements `agk triage <review.json>`.
func (e *Env) Triage(ctx context.Context, args []string) int {
	if len(args) != 1 {
		return e.errorf("usage: agk triage <review.json>")
	}
	path, err := filepath.Abs(args[0])
	if err != nil {
		return e.errorf("%v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return e.errorf("%v", err)
	}
	r, err := review.Parse(raw)
	if err != nil {
		return e.errorf("%s: %v", path, err)
	}
	if len(r.Findings) == 0 {
		fmt.Fprintln(e.Stdout, "[]")
		return ExitOK
	}
	session := e.session()
	if session == "" {
		fmt.Fprint(e.Stdout, triage.Table(r))
		return ExitNoAgterm
	}
	page, err := triage.Render(r, triage.NewNonce())
	if err != nil {
		return e.errorf("render triage page: %v", err)
	}
	dir := filepath.Dir(path)
	// A fresh file per call (O_EXCL, never through a symlink) so overlay
	// cleanup in one triage call cannot match another call's open page.
	f, err := os.CreateTemp(dir, "triage-*.html")
	if err != nil {
		return e.errorf("%v", err)
	}
	htmlPath := f.Name()
	defer os.Remove(htmlPath)
	_, werr := f.Write(page)
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	if werr != nil {
		return e.errorf("%v", werr)
	}
	out, err := e.Ctl.OverlayHTML(ctx, session, htmlPath)
	if errors.Is(err, agterm.ErrDismissed) {
		fmt.Fprintln(e.Stderr, "agk: triage dismissed, no decisions")
		return ExitDismissed
	}
	if err != nil {
		return e.errorf("%v", err)
	}
	decisions, err := triage.Decide(r, out.Value)
	if err != nil {
		return e.errorf("%v", err)
	}
	b, err := json.Marshal(decisions)
	if err != nil {
		return e.errorf("%v", err)
	}
	b = append(b, '\n')
	if err := fsx.ReplaceFile(filepath.Join(dir, "decisions.json"), b, 0o644); err != nil {
		return e.errorf("%v", err)
	}
	e.Stdout.Write(b)
	return ExitOK
}
