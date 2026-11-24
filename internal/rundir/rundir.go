// Package rundir manages agk run directories: .agk/<kind>/<timestamp>/ under a
// repository root, their numbering, rounds and small state files.
package rundir

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/shurastik/agterm-kit/internal/fsx"
)

// Dir is where run artifacts live inside a repository.
const Dir = ".agk"

// Run kinds.
const (
	Reviews = "reviews"
	Applies = "applies"
)

// RoundFile holds the current round number of a run.
const RoundFile = "round"

func checkKind(kind string) error {
	if kind != Reviews && kind != Applies {
		return fmt.Errorf("unknown run kind %q", kind)
	}
	return nil
}

// New creates a fresh .agk/<kind>/<YYYYMMDD-HHMMSS>[-N] directory under root.
func New(root, kind string, now time.Time) (string, error) {
	if err := checkKind(kind); err != nil {
		return "", err
	}
	parent := filepath.Join(root, Dir, kind)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return "", err
	}
	base := filepath.Join(parent, now.Format("20060102-150405"))
	for n := 1; n < 1000; n++ {
		dir := base
		if n > 1 {
			dir = fmt.Sprintf("%s-%d", base, n)
		}
		err := os.Mkdir(dir, 0o755)
		if err == nil {
			return dir, nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return "", err
		}
	}
	return "", fmt.Errorf("no free run directory under %s", parent)
}

// Root maps <root>/.agk/<kind>/<run> to <root>.
func Root(dir, kind string) (string, error) {
	parent := filepath.Dir(dir)
	agk := filepath.Dir(parent)
	if filepath.Base(parent) != kind || filepath.Base(agk) != Dir || !runName.MatchString(filepath.Base(dir)) {
		return "", fmt.Errorf("%s is not a %s/%s/<run> directory", dir, Dir, kind)
	}
	return filepath.Dir(agk), nil
}

var runName = regexp.MustCompile(`^\d{8}-\d{6}(-\d+)?$`)

// RoundDir is dir/rounds/<k>.
func RoundDir(dir string, k int) string {
	return filepath.Join(dir, "rounds", strconv.Itoa(k))
}

// MakeRound creates dir/rounds/<k> and returns it.
func MakeRound(dir string, k int) (string, error) {
	rd := RoundDir(dir, k)
	if err := os.MkdirAll(rd, 0o755); err != nil {
		return "", err
	}
	return rd, nil
}

// Round reads the run's current round number.
func Round(dir string) (int, error) {
	p := filepath.Join(dir, RoundFile)
	b, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return 0, fmt.Errorf("%s has no round file: not a round-based run directory", dir)
		}
		return 0, err
	}
	k, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil || k < 1 {
		return 0, fmt.Errorf("%s: not a round number: %q", p, strings.TrimSpace(string(b)))
	}
	return k, nil
}

// SetRound records k as the run's current round.
func SetRound(dir string, k int) error {
	return fsx.ReplaceFile(filepath.Join(dir, RoundFile), []byte(strconv.Itoa(k)+"\n"), 0o644)
}

// WriteJSON writes v as indented JSON to dir/name, replacing it atomically.
func WriteJSON(dir, name string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return fsx.ReplaceFile(filepath.Join(dir, name), append(b, '\n'), 0o644)
}

// ReadJSON decodes dir/name into v; have is false when the file does not exist.
func ReadJSON(dir, name string, v any) (have bool, err error) {
	p := filepath.Join(dir, name)
	b, err := os.ReadFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err := json.Unmarshal(b, v); err != nil {
		return false, fmt.Errorf("%s: %w", p, err)
	}
	return true, nil
}
