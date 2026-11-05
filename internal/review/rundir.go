package review

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// NewRunDir creates a fresh .agk/reviews/<YYYYMMDD-HHMMSS>[-N] directory under root.
func NewRunDir(root string, now time.Time) (string, error) {
	parent := filepath.Join(root, agkDir, "reviews")
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

// RunNumber is the 1-based position of run dir among its sibling run dirs,
// ordered by name (timestamp, then collision suffix). Newer runs sort after,
// so an existing run keeps its number.
func RunNumber(dir string) (int, error) {
	base := filepath.Base(dir)
	entries, err := os.ReadDir(filepath.Dir(dir))
	if err != nil {
		return 0, err
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() && runName.MatchString(e.Name()) {
			names = append(names, e.Name())
		}
	}
	slices.SortFunc(names, compareRunNames)
	if i := slices.Index(names, base); i >= 0 {
		return i + 1, nil
	}
	return 0, fmt.Errorf("%s is not a run directory", dir)
}

var runName = regexp.MustCompile(`^\d{8}-\d{6}(-\d+)?$`)

// compareRunNames orders "20261105-100000" < "20261105-100000-2" < "-10".
func compareRunNames(a, b string) int {
	ta, na := splitRunName(a)
	tb, nb := splitRunName(b)
	if c := strings.Compare(ta, tb); c != 0 {
		return c
	}
	return na - nb
}

func splitRunName(s string) (string, int) {
	if len(s) > 15 {
		n, _ := strconv.Atoi(s[16:])
		return s[:15], n
	}
	return s, 1
}
