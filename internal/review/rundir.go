package review

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
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
