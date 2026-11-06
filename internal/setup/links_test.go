package setup

import (
	"os"
	"path/filepath"
	"testing"
)

// linkEnv is a temp HOME with ~/.claude and ~/.codex and a skills dir with agk-review.
type linkEnv struct {
	home, skills string
	vars         map[string]string
}

func newLinkEnv(t *testing.T) *linkEnv {
	t.Helper()
	root := realTemp(t)
	e := &linkEnv{home: mkdir(t, root, "home"), skills: mkdir(t, root, "skills"), vars: map[string]string{}}
	touch(t, filepath.Join(e.skills, "agk-review", "SKILL.md"))
	mkdir(t, e.home, ".claude")
	mkdir(t, e.home, ".codex")
	return e
}

func (e *linkEnv) cfg() Config {
	return Config{Home: e.home, SkillsDir: e.skills, Getenv: func(k string) string { return e.vars[k] }}
}

func symlink(t *testing.T, target, link string) {
	t.Helper()
	mkdir(t, filepath.Dir(link))
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
}

// find returns the action for path, failing when there is none.
func find(t *testing.T, acts []Action, path string) Action {
	t.Helper()
	for _, a := range acts {
		if a.Path == path {
			return a
		}
	}
	t.Fatalf("no action for %s in %+v", path, acts)
	return Action{}
}

func hasPath(acts []Action, path string) bool {
	for _, a := range acts {
		if a.Path == path {
			return true
		}
	}
	return false
}

func TestLinksFirstSetup(t *testing.T) {
	e := newLinkEnv(t)
	acts, err := planLinks(e.cfg())
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(e.skills, "agk-review")
	for _, agent := range []string{".claude", ".codex"} {
		a := find(t, acts, filepath.Join(e.home, agent, "skills", "agk-review"))
		if a.Kind != KindCreate || a.Target != want {
			t.Errorf("%s: %+v, want create -> %s", agent, a, want)
		}
	}
}

func TestLinksOnlyAgkSkillDirs(t *testing.T) {
	e := newLinkEnv(t)
	touch(t, filepath.Join(e.skills, "other", "SKILL.md"))
	mkdir(t, e.skills, "agk-empty") // no SKILL.md
	touch(t, filepath.Join(e.skills, "agk-file"))
	acts, err := planLinks(e.cfg())
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"other", "agk-empty", "agk-file"} {
		if hasPath(acts, filepath.Join(e.home, ".claude", "skills", name)) {
			t.Errorf("%s planned: %+v", name, acts)
		}
	}
}

func TestLinksAgentConfigDirVariables(t *testing.T) {
	e := newLinkEnv(t)
	claude := mkdir(t, e.home, "cc")
	codex := mkdir(t, e.home, "cx")
	e.vars["CLAUDE_CONFIG_DIR"] = claude
	e.vars["CODEX_HOME"] = codex
	acts, err := planLinks(e.cfg())
	if err != nil {
		t.Fatal(err)
	}
	find(t, acts, filepath.Join(claude, "skills", "agk-review"))
	find(t, acts, filepath.Join(codex, "skills", "agk-review"))
}

func TestLinksMissingAgentSkipped(t *testing.T) {
	e := newLinkEnv(t)
	os.Remove(filepath.Join(e.home, ".codex"))
	acts, err := planLinks(e.cfg())
	if err != nil {
		t.Fatal(err)
	}
	a := find(t, acts, filepath.Join(e.home, ".codex"))
	if a.Kind != KindSkip {
		t.Fatalf("codex: %+v, want skip", a)
	}
	if hasPath(acts, filepath.Join(e.home, ".codex", "skills", "agk-review")) {
		t.Fatal("link planned for a missing agent")
	}
	find(t, acts, filepath.Join(e.home, ".claude", "skills", "agk-review"))
}

func TestLinksAlreadyCorrect(t *testing.T) {
	e := newLinkEnv(t)
	link := filepath.Join(e.home, ".claude", "skills", "agk-review")
	symlink(t, filepath.Join(e.skills, "agk-review"), link)
	acts, _ := planLinks(e.cfg())
	if a := find(t, acts, link); a.Kind != KindOK {
		t.Fatalf("%+v, want ok", a)
	}
}

func TestLinksRepointElsewhere(t *testing.T) {
	e := newLinkEnv(t)
	link := filepath.Join(e.home, ".claude", "skills", "agk-review")
	symlink(t, "/old/place/agk-review", link)
	acts, _ := planLinks(e.cfg())
	if a := find(t, acts, link); a.Kind != KindRepoint || a.Target != filepath.Join(e.skills, "agk-review") {
		t.Fatalf("%+v, want repoint", a)
	}
}

func TestLinksConflictOnRealDirAndFile(t *testing.T) {
	e := newLinkEnv(t)
	touch(t, filepath.Join(e.skills, "agk-flow", "SKILL.md"))
	dir := mkdir(t, e.home, ".claude", "skills", "agk-review")
	file := filepath.Join(e.home, ".claude", "skills", "agk-flow")
	touch(t, file)
	acts, _ := planLinks(e.cfg())
	for _, p := range []string{dir, file} {
		if a := find(t, acts, p); a.Kind != KindConflict {
			t.Errorf("%s: %+v, want conflict", p, a)
		}
	}
	if a := find(t, acts, filepath.Join(e.home, ".codex", "skills", "agk-review")); a.Kind != KindCreate {
		t.Errorf("other links not planned: %+v", a)
	}
}

func TestLinksStaleAgkLinkRemoved(t *testing.T) {
	e := newLinkEnv(t)
	stale := filepath.Join(e.home, ".claude", "skills", "agk-old")
	symlink(t, filepath.Join(e.skills, "agk-old"), stale)
	acts, _ := planLinks(e.cfg())
	if a := find(t, acts, stale); a.Kind != KindRemove {
		t.Fatalf("%+v, want remove", a)
	}
}

func TestLinksLegacyLinkRemoved(t *testing.T) {
	e := newLinkEnv(t)
	legacy := filepath.Join(e.home, ".claude", "skills", "agterm-codex-review")
	symlink(t, "/Users/x/Projects/agterm-kit/skill/agterm-codex-review", legacy)
	acts, _ := planLinks(e.cfg())
	if a := find(t, acts, legacy); a.Kind != KindRemove {
		t.Fatalf("%+v, want remove", a)
	}
}

func TestLinksUnrelatedUntouched(t *testing.T) {
	e := newLinkEnv(t)
	agterm := mkdir(t, e.home, ".claude", "skills", "agterm")
	pohuy := filepath.Join(e.home, ".claude", "skills", "pohuy")
	symlink(t, "/somewhere/pohuy", pohuy)
	realAgk := mkdir(t, e.home, ".claude", "skills", "agk-mine") // real dir, not ours to remove
	legacyDir := mkdir(t, e.home, ".codex", "skills", "agterm-codex-review")
	acts, _ := planLinks(e.cfg())
	for _, p := range []string{agterm, pohuy, realAgk, legacyDir} {
		if hasPath(acts, p) {
			t.Errorf("%s has an action: %+v", p, acts)
		}
	}
}
