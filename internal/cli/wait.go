package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/shurastik/agterm-kit/internal/agterm"
	"github.com/shurastik/agterm-kit/internal/rundir"
)

// defaultWaitTimeout bounds `agk review wait` unless --timeout says otherwise.
const defaultWaitTimeout = 1800

// companionID is the shape of an agterm session or pane id (a UUID, or a prefix of one).
var companionID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]*$`)

// waitOutcome is what `agk review wait` prints when result.json has no say.
type waitOutcome struct {
	Status string `json:"status"`
	Round  int    `json:"round"`
}

// ReviewWait implements `agk review wait <run-dir> [--timeout SECONDS]`.
func (e *Env) ReviewWait(ctx context.Context, args []string) int {
	dir, timeout, code, ok := e.parseWaitArgs("agk review wait", rundir.Reviews, defaultWaitTimeout, args)
	if !ok {
		return code
	}
	k, err := rundir.Round(dir)
	if err != nil {
		return e.errorf("%v", err)
	}
	rd := rundir.RoundDir(dir, k)
	companion, err := readCompanion(dir)
	if err != nil {
		return e.errorf("%v", err)
	}
	res, have, err := readResult(rd)
	if err != nil {
		return e.errorf("%v", err)
	}
	if companion == "" {
		// Inline run: it finished before `review start`/`send` returned.
		if !have {
			return e.errorf("%s has no result.json and no companion: the round did not record an outcome", rd)
		}
		return e.printJSON(res)
	}
	open := true
	var status string
	left := time.Duration(timeout) * time.Second
	if !have && k > 1 && !roundStarted(dir, k) {
		// The companion may still show the last round's completed or blocked
		// until the loop picks this round up: wait for that first.
		status, err, _, left = e.waitPolling(ctx, companion, []string{"active"}, left, func() bool {
			_, done, _ := readResult(rd)
			return done || roundStarted(dir, k)
		})
		switch {
		case errors.Is(err, agterm.ErrGone):
			open = false
		case errors.Is(err, agterm.ErrTimeout):
			if res, have, err = readResult(rd); err != nil {
				return e.errorf("%v", err)
			}
			if have {
				return e.printJSON(res)
			}
			return e.printJSON(waitOutcome{Status: "running", Round: k})
		case err != nil:
			return e.errorf("wait for companion %s: %v", companion, err)
		}
		status = ""
		if res, have, err = readResult(rd); err != nil {
			return e.errorf("%v", err)
		}
	}
	if !have && open {
		// result.json first: agterm may already have auto-reset a completed
		// status, and the wait would then block until its timeout.
		status, err = e.Ctl.Wait(ctx, companion, []string{"completed", "blocked"}, left)
		switch {
		case errors.Is(err, agterm.ErrGone):
			open = false
		case errors.Is(err, agterm.ErrTimeout):
		case err != nil:
			return e.errorf("wait for companion %s: %v", companion, err)
		}
		if res, have, err = readResult(rd); err != nil {
			return e.errorf("%v", err)
		}
	}
	if have {
		return e.printJSON(res)
	}
	switch {
	case !open:
		return e.printJSON(waitOutcome{Status: "cancelled", Round: k})
	case status == "":
		return e.printJSON(waitOutcome{Status: "running", Round: k})
	case status == "blocked":
		// Failed before the round could record an outcome.
		return e.printJSON(waitOutcome{Status: "blocked", Round: k})
	}
	return e.errorf("companion %s reported %s but %s has no result.json", companion, status, rd)
}

// parseInterleaved parses fs over args with positional arguments allowed
// before, between and after flags.
func parseInterleaved(fs *flag.FlagSet, args []string) (pos []string, code int, ok bool) {
	for {
		if err := fs.Parse(args); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return nil, ExitOK, false
			}
			return nil, ExitError, false
		}
		if fs.NArg() == 0 {
			return pos, ExitOK, true
		}
		pos = append(pos, fs.Arg(0))
		args = fs.Args()[1:]
	}
}

// parseWaitArgs accepts the run dir before or after --timeout.
func (e *Env) parseWaitArgs(name, kind string, def int, args []string) (dir string, timeout int, code int, ok bool) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(e.Stderr)
	fs.IntVar(&timeout, "timeout", def, "seconds to wait for the outcome")
	pos, code, ok := parseInterleaved(fs, args)
	if !ok {
		return "", 0, code, false
	}
	if len(pos) != 1 {
		return "", 0, e.errorf("usage: %s <run-dir> [--timeout SECONDS]", name), false
	}
	if timeout <= 0 {
		return "", 0, e.errorf("--timeout must be a positive number of seconds, got %d", timeout), false
	}
	dir, err := filepath.Abs(pos[0])
	if err != nil {
		return "", 0, e.errorf("%v", err), false
	}
	if _, err := rundir.Root(dir, kind); err != nil {
		return "", 0, e.errorf("%v", err), false
	}
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		return "", 0, e.errorf("%s is not a run directory", dir), false
	}
	return dir, timeout, ExitOK, true
}

// readCompanion returns the recorded companion id, or "" for an inline run.
func readCompanion(dir string) (string, error) {
	b, err := os.ReadFile(filepath.Join(dir, CompanionFile))
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	id := strings.TrimSpace(string(b))
	if !companionID.MatchString(id) {
		return "", fmt.Errorf("%s: not a session id: %q", filepath.Join(dir, CompanionFile), id)
	}
	return id, nil
}

// readResult reads rd/result.json; have is false when it does not exist yet.
func readResult(rd string) (res runResult, have bool, err error) {
	p := filepath.Join(rd, "result.json")
	b, err := os.ReadFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		return runResult{}, false, nil
	}
	if err != nil {
		return runResult{}, false, err
	}
	if err := json.Unmarshal(b, &res); err != nil {
		return runResult{}, false, fmt.Errorf("%s: %v", p, err)
	}
	if res.Status != "completed" && res.Status != "blocked" {
		return runResult{}, false, fmt.Errorf("%s: unknown status %q", p, res.Status)
	}
	return res, true, nil
}
