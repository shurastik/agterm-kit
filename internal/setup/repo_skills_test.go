package setup

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The shipped skills/ dir: every agk-* skill has a SKILL.md whose front
// matter name matches its directory, and the workflow skills are all there.
func TestRepoSkillsHaveMatchingNames(t *testing.T) {
	dir := filepath.Join("..", "..", "skills")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	nameLine := regexp.MustCompile(`(?m)^name:\s*(\S+)\s*$`)
	found := map[string]bool{}
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), "agk-") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name(), "SKILL.md"))
		if err != nil {
			t.Errorf("%s: %v", e.Name(), err)
			continue
		}
		front, _, ok := strings.Cut(strings.TrimPrefix(string(b), "---\n"), "\n---\n")
		if !strings.HasPrefix(string(b), "---\n") || !ok {
			t.Errorf("%s: no front matter", e.Name())
			continue
		}
		m := nameLine.FindStringSubmatch(front)
		if m == nil || m[1] != e.Name() {
			t.Errorf("%s: front matter name %v does not match the directory", e.Name(), m)
		}
		found[e.Name()] = true
	}
	for _, want := range []string{"agk-apply", "agk-review", "agk-propose"} {
		if !found[want] {
			t.Errorf("skill %s missing from %s", want, dir)
		}
	}
}
