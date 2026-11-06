package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setupHarness is a harness whose home has ~/.claude and ~/.codex and whose
// skills dir holds agk-review; every required tool is on the fake PATH.
type setupHarness struct {
	*tenv
	home, skills string
	path         map[string]bool
}

func newSetupHarness(t *testing.T) *setupHarness {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s := &setupHarness{
		tenv:   newHarness(t, false),
		home:   filepath.Join(root, "home"),
		skills: filepath.Join(root, "skills"),
		path:   map[string]bool{"git": true, "codex": true, "claude": true, "openspec": true, "agtermctl": true},
	}
	for _, d := range []string{
		filepath.Join(s.home, ".claude"), filepath.Join(s.home, ".codex"), filepath.Join(s.skills, "agk-review"),
	} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	os.WriteFile(filepath.Join(s.skills, "agk-review", "SKILL.md"), []byte("---\nname: agk-review\n---\n"), 0o644)
	s.env.Home = s.home
	s.env.LookPath = func(n string) (string, error) {
		if s.path[n] {
			return "/bin/" + n, nil
		}
		return "", errors.New("not found")
	}
	s.env.GitConfig = func(string) (string, bool, error) { return "", false, nil }
	s.env.IsExec = func(string) bool { return false }
	return s
}

func (s *setupHarness) run(args ...string) int {
	return s.env.Main(context.Background(), append([]string{"setup", "--skills-dir", s.skills}, args...))
}

func (s *setupHarness) link(agent string) string {
	return filepath.Join(s.home, agent, "skills", "agk-review")
}

func TestSetupLinksAndIgnore(t *testing.T) {
	s := newSetupHarness(t)
	if code := s.run(); code != ExitOK {
		t.Fatalf("exit %d: %s%s", code, s.out, s.err)
	}
	for _, agent := range []string{".claude", ".codex"} {
		if got, _ := os.Readlink(s.link(agent)); got != filepath.Join(s.skills, "agk-review") {
			t.Errorf("%s link = %q", agent, got)
		}
	}
	if data, _ := os.ReadFile(filepath.Join(s.home, ".config", "git", "ignore")); string(data) != ".agk/\n" {
		t.Errorf("ignore = %q", data)
	}
	out := s.out.String()
	for _, want := range []string{"create", "~/.claude/skills/agk-review", "append", "found"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestSetupSecondRunAllOK(t *testing.T) {
	s := newSetupHarness(t)
	s.run()
	s.out.Reset()
	if code := s.run(); code != ExitOK {
		t.Fatalf("exit %d: %s", code, s.err)
	}
	for _, kind := range []string{"create", "repoint", "remove", "append"} {
		if strings.Contains(s.out.String(), kind) {
			t.Errorf("second run reports %q:\n%s", kind, s.out)
		}
	}
}

func TestSetupDryRunChangesNothing(t *testing.T) {
	s := newSetupHarness(t)
	if code := s.run("--dry-run"); code != ExitOK {
		t.Fatalf("exit %d: %s", code, s.err)
	}
	if _, err := os.Lstat(s.link(".claude")); !os.IsNotExist(err) {
		t.Errorf("dry run created a link: %v", err)
	}
	if _, err := os.Stat(filepath.Join(s.home, ".config")); !os.IsNotExist(err) {
		t.Errorf("dry run touched the excludes file: %v", err)
	}
	out := s.out.String()
	if !strings.Contains(out, "create") || !strings.Contains(out, "append") || !strings.Contains(out, "dry run") {
		t.Errorf("dry run output:\n%s", out)
	}
}

func TestSetupConflictExitsOneAfterRest(t *testing.T) {
	s := newSetupHarness(t)
	os.MkdirAll(s.link(".claude"), 0o755)
	if code := s.run(); code != ExitError {
		t.Fatalf("exit %d", code)
	}
	if fi, err := os.Lstat(s.link(".claude")); err != nil || !fi.IsDir() {
		t.Errorf("conflicting dir changed: %v %v", fi, err)
	}
	if _, err := os.Readlink(s.link(".codex")); err != nil {
		t.Errorf("codex link not created: %v", err)
	}
	if !strings.Contains(s.out.String(), "conflict") {
		t.Errorf("no conflict line:\n%s", s.out)
	}
}

func TestSetupMissingRequiredTool(t *testing.T) {
	s := newSetupHarness(t)
	delete(s.path, "openspec")
	if code := s.run(); code != ExitError {
		t.Fatalf("exit %d", code)
	}
	if _, err := os.Readlink(s.link(".claude")); err != nil {
		t.Errorf("wiring skipped: %v", err)
	}
	if !strings.Contains(s.out.String(), "missing  openspec (required)") {
		t.Errorf("output:\n%s", s.out)
	}
}

func TestSetupMissingAgtermIsOptional(t *testing.T) {
	s := newSetupHarness(t)
	delete(s.path, "agtermctl")
	if code := s.run(); code != ExitOK {
		t.Fatalf("exit %d: %s", code, s.out)
	}
	if !strings.Contains(s.out.String(), "missing  agtermctl (optional)") {
		t.Errorf("output:\n%s", s.out)
	}
}

func TestSetupWritesNothingInWorkingDir(t *testing.T) {
	s := newSetupHarness(t)
	s.run()
	entries, err := os.ReadDir(s.env.Cwd)
	if err != nil || len(entries) != 0 {
		t.Fatalf("cwd has %v, %v", entries, err)
	}
}

func TestSetupRelativeSkillsDirFromCwd(t *testing.T) {
	s := newSetupHarness(t)
	s.env.Cwd = filepath.Dir(s.skills)
	code := s.env.Main(context.Background(), []string{"setup", "--skills-dir", "skills"})
	if code != ExitOK {
		t.Fatalf("exit %d: %s", code, s.err)
	}
	if got, _ := os.Readlink(s.link(".claude")); got != filepath.Join(s.skills, "agk-review") {
		t.Errorf("link = %q", got)
	}
}

func TestSetupNoSkillsDir(t *testing.T) {
	s := newSetupHarness(t)
	code := s.env.Main(context.Background(), []string{"setup"}) // Executable is /usr/local/bin/agk
	if code != ExitError || !strings.Contains(s.err.String(), "--skills-dir") {
		t.Fatalf("exit %d, stderr %q", code, s.err)
	}
}

func TestSetupArgs(t *testing.T) {
	s := newSetupHarness(t)
	if code := s.env.Main(context.Background(), []string{"setup", "--help"}); code != ExitOK {
		t.Errorf("--help exit %d", code)
	}
	if code := s.env.Main(context.Background(), []string{"setup", "extra"}); code != ExitError {
		t.Errorf("extra arg exit %d", code)
	}
}

func TestSetupRelativeExcludesFileLeavesProjectAlone(t *testing.T) {
	s := newSetupHarness(t)
	s.env.GitConfig = func(string) (string, bool, error) { return ".gitignore", true, nil }
	if code := s.run(); code != ExitError {
		t.Fatalf("exit %d", code)
	}
	entries, _ := os.ReadDir(s.env.Cwd)
	if len(entries) != 0 {
		t.Fatalf("cwd has %v", entries)
	}
	if _, err := os.Readlink(s.link(".claude")); err != nil {
		t.Errorf("links not created: %v", err)
	}
	if !strings.Contains(s.out.String(), "conflict") {
		t.Errorf("output:\n%s", s.out)
	}
}
