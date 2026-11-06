package termsafe

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Bidi control characters in source can hide code from reviewers (Trojan
// Source); tests must spell them as escapes.
func TestNoBidiControlsInSource(t *testing.T) {
	root := filepath.Join("..", "..")
	filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (d.Name() == ".git" || d.Name() == ".agk" || d.Name() == ".worktrees") {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(p, ".go") {
			return nil
		}
		b, _ := os.ReadFile(p)
		for _, r := range string(b) {
			if (r >= 0x202a && r <= 0x202e) || (r >= 0x2066 && r <= 0x2069) {
				t.Errorf("%s contains bidi control %U", p, r)
				break
			}
		}
		return nil
	})
}
