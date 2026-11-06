package setup

import (
	"os"
	"path/filepath"
	"testing"
)

func fullCfg(e *linkEnv) Config {
	c := e.cfg()
	c.GitConfig = func(string) (string, error) { return "", nil }
	c.LookPath = func(n string) (string, error) { return "/bin/" + n, nil }
	c.IsExec = func(string) bool { return false }
	return c
}

func applyPlan(t *testing.T, c Config) []Action {
	t.Helper()
	acts, err := Plan(c)
	if err != nil {
		t.Fatal(err)
	}
	if err := Apply(acts); err != nil {
		t.Fatal(err)
	}
	return acts
}

func TestApplyCreatesAbsoluteLinksAndIgnore(t *testing.T) {
	e := newLinkEnv(t)
	applyPlan(t, fullCfg(e))
	want := filepath.Join(e.skills, "agk-review")
	for _, agent := range []string{".claude", ".codex"} {
		got, err := os.Readlink(filepath.Join(e.home, agent, "skills", "agk-review"))
		if err != nil || got != want {
			t.Errorf("%s: %q, %v", agent, got, err)
		}
	}
	data, err := os.ReadFile(filepath.Join(e.home, ".config", "git", "ignore"))
	if err != nil || string(data) != ".agk/\n" {
		t.Fatalf("ignore = %q, %v", data, err)
	}
}

func TestApplySecondPlanAllOK(t *testing.T) {
	e := newLinkEnv(t)
	symlink(t, "/old/agk-review", filepath.Join(e.home, ".claude", "skills", "agk-review"))
	symlink(t, "/x/agterm-codex-review", filepath.Join(e.home, ".claude", "skills", LegacyLink))
	applyPlan(t, fullCfg(e))
	acts, err := Plan(fullCfg(e))
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range acts {
		if a.Mutates() || a.Failed() {
			t.Errorf("second plan not clean: %+v", a)
		}
	}
	if _, err := os.Lstat(filepath.Join(e.home, ".claude", "skills", LegacyLink)); !os.IsNotExist(err) {
		t.Errorf("legacy link still there: %v", err)
	}
}

func TestApplyKeepsSymlinkedExcludesFile(t *testing.T) {
	e := newLinkEnv(t)
	dot := filepath.Join(e.home, "dotfiles", "gitignore")
	touch(t, dot)
	os.WriteFile(dot, []byte("**/.claude/settings.local.json"), 0o600) // no trailing newline
	os.Chmod(dot, 0o600)
	link := filepath.Join(e.home, ".config", "git", "ignore")
	symlink(t, dot, link)
	applyPlan(t, fullCfg(e))
	applyPlan(t, fullCfg(e))
	fi, err := os.Lstat(link)
	if err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("excludes symlink replaced: %v %v", fi, err)
	}
	data, _ := os.ReadFile(dot)
	if string(data) != "**/.claude/settings.local.json\n.agk/\n" {
		t.Fatalf("dotfile = %q", data)
	}
	if st, _ := os.Stat(dot); st.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v", st.Mode())
	}
}

func TestApplyRemoveOnlySymlinks(t *testing.T) {
	dir := realTemp(t)
	real := mkdir(t, dir, "agk-real")
	if err := Apply([]Action{{Kind: KindRemove, Path: real}}); err == nil {
		t.Fatal("removing a real dir must fail")
	}
	if !isDir(real) {
		t.Fatal("real dir removed")
	}
}

func TestApplyLeavesConflictsAndContinues(t *testing.T) {
	e := newLinkEnv(t)
	conflict := mkdir(t, e.home, ".claude", "skills", "agk-review")
	acts := applyPlan(t, fullCfg(e))
	if find(t, acts, conflict).Kind != KindConflict || !isDir(conflict) {
		t.Fatal("conflict not left alone")
	}
	if _, err := os.Readlink(filepath.Join(e.home, ".codex", "skills", "agk-review")); err != nil {
		t.Fatalf("other link not created: %v", err)
	}
}
