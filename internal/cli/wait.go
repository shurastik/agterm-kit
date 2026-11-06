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
)

// defaultWaitTimeout bounds `agk review wait` unless --timeout says otherwise.
const defaultWaitTimeout = 1800

// companionID is the shape of an agterm session id (a UUID, or a prefix of one).
var companionID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]*$`)

// waitOutcome is what `agk review wait` prints when result.json has no say.
type waitOutcome struct {
	Status string `json:"status"`
}

// ReviewWait implements `agk review wait <run-dir> [--timeout SECONDS]`.
func (e *Env) ReviewWait(ctx context.Context, args []string) int {
	dir, timeout, code, ok := e.parseWaitArgs(args)
	if !ok {
		return code
	}
	companion, err := readCompanion(dir)
	if err != nil {
		return e.errorf("%v", err)
	}
	res, have, err := readResult(dir)
	if err != nil {
		return e.errorf("%v", err)
	}
	if companion == "" {
		// Inline run: it finished before `review start` returned.
		if !have {
			return e.errorf("%s has no result.json and no companion: the review did not record an outcome", dir)
		}
		return e.printJSON(res)
	}
	// open: true while the companion is known to be in agterm, false once it
	// is known to be gone; unknown when result.json was there before waiting.
	open := true
	var status string
	if !have {
		// result.json first: agterm may already have auto-reset a completed
		// status, and the wait would then block until its timeout.
		status, err = e.Ctl.Wait(ctx, companion, []string{"completed", "blocked"}, time.Duration(timeout)*time.Second)
		switch {
		case errors.Is(err, agterm.ErrGone):
			open = false
		case errors.Is(err, agterm.ErrTimeout):
		case err != nil:
			return e.errorf("wait for companion %s: %v", companion, err)
		}
		if res, have, err = readResult(dir); err != nil {
			return e.errorf("%v", err)
		}
	}
	if !have {
		switch {
		case !open:
			return e.printJSON(waitOutcome{Status: "cancelled"})
		case status == "":
			return e.printJSON(waitOutcome{Status: "running"})
		case status == "blocked":
			// Failed before the run dir was verified; no result.json then.
			return e.printJSON(waitOutcome{Status: "blocked"})
		}
		return e.errorf("companion %s reported %s but %s has no result.json", companion, status, dir)
	}
	if res.Status == "completed" && open {
		cctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), statusTimeout)
		defer cancel()
		if err := e.Ctl.Close(cctx, companion); err != nil && !errors.Is(err, agterm.ErrGone) {
			fmt.Fprintf(e.Stderr, "agk: warning: close companion %s: %v\n", companion, err)
		}
	}
	return e.printJSON(res)
}

// parseWaitArgs accepts the run dir before or after --timeout.
func (e *Env) parseWaitArgs(args []string) (dir string, timeout int, code int, ok bool) {
	fs := flag.NewFlagSet("agk review wait", flag.ContinueOnError)
	fs.SetOutput(e.Stderr)
	fs.IntVar(&timeout, "timeout", defaultWaitTimeout, "seconds to wait for the outcome")
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return "", 0, ExitOK, false
			}
			return "", 0, ExitError, false
		}
		if fs.NArg() == 0 {
			break
		}
		pos = append(pos, fs.Arg(0))
		args = fs.Args()[1:]
	}
	if len(pos) != 1 {
		return "", 0, e.errorf("usage: agk review wait <run-dir> [--timeout SECONDS]"), false
	}
	if timeout <= 0 {
		return "", 0, e.errorf("--timeout must be a positive number of seconds, got %d", timeout), false
	}
	dir, err := filepath.Abs(pos[0])
	if err != nil {
		return "", 0, e.errorf("%v", err), false
	}
	if _, err := runRoot(dir); err != nil {
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

// readResult reads result.json; have is false when it does not exist yet.
func readResult(dir string) (res runResult, have bool, err error) {
	p := filepath.Join(dir, "result.json")
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
