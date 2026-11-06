package setup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// realTemp returns a temp dir with symlinks resolved (macOS /var -> /private/var).
func realTemp(t *testing.T) string {
	t.Helper()
	d, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func mkdir(t *testing.T, parts ...string) string {
	t.Helper()
	p := filepath.Join(parts...)
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func touch(t *testing.T, p string) {
	t.Helper()
	mkdir(t, filepath.Dir(p))
	if err := os.WriteFile(p, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestSkillsDirFlagWins(t *testing.T) {
	root := realTemp(t)
	flag := mkdir(t, root, "flag")
	env := mkdir(t, root, "env")
	got, err := ResolveSkillsDir(flag, env, filepath.Join(root, "bin", "agk"))
	if err != nil || got != flag {
		t.Fatalf("got %q, %v; want %q", got, err, flag)
	}
}

func TestSkillsDirFlagIsMadeAbsolute(t *testing.T) {
	root := realTemp(t)
	mkdir(t, root, "skills")
	t.Chdir(root)
	got, err := ResolveSkillsDir("./skills", "", "/nowhere/agk")
	if err != nil || got != filepath.Join(root, "skills") {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestSkillsDirEnvBeforeBinary(t *testing.T) {
	root := realTemp(t)
	env := mkdir(t, root, "env")
	exe := filepath.Join(root, "bin", "agk")
	touch(t, exe)
	mkdir(t, root, "share", "agk", "skills")
	got, err := ResolveSkillsDir("", env, exe)
	if err != nil || got != env {
		t.Fatalf("got %q, %v; want %q", got, err, env)
	}
}

func TestSkillsDirExplicitMissingIsError(t *testing.T) {
	root := realTemp(t)
	for _, tc := range []struct{ flag, env, name string }{
		{filepath.Join(root, "nope"), "", "--skills-dir"},
		{"", filepath.Join(root, "nope"), "AGK_SKILLS_DIR"},
	} {
		_, err := ResolveSkillsDir(tc.flag, tc.env, "/nowhere/agk")
		if err == nil || !strings.Contains(err.Error(), tc.name) {
			t.Errorf("flag=%q env=%q: err = %v, want mention of %s", tc.flag, tc.env, err, tc.name)
		}
	}
}

func TestSkillsDirHomebrewOptPath(t *testing.T) {
	prefix := realTemp(t)
	exe := filepath.Join(prefix, "Cellar", "agk", "0.3.0", "bin", "agk")
	touch(t, exe)
	mkdir(t, prefix, "Cellar", "agk", "0.3.0", "share", "agk", "skills")
	mkdir(t, prefix, "opt")
	if err := os.Symlink("../Cellar/agk/0.3.0", filepath.Join(prefix, "opt", "agk")); err != nil {
		t.Fatal(err)
	}
	// bin/agk -> ../Cellar/agk/0.3.0/bin/agk, as Homebrew links it
	mkdir(t, prefix, "bin")
	link := filepath.Join(prefix, "bin", "agk")
	if err := os.Symlink("../Cellar/agk/0.3.0/bin/agk", link); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(prefix, "opt", "agk", "share", "agk", "skills")
	for _, e := range []string{exe, link} {
		got, err := ResolveSkillsDir("", "", e)
		if err != nil || got != want {
			t.Errorf("exe %s: got %q, %v; want %q", e, got, err, want)
		}
	}
}

func TestSkillsDirBesideBinary(t *testing.T) {
	root := realTemp(t)
	exe := filepath.Join(root, "bin", "agk")
	touch(t, exe)
	want := mkdir(t, root, "share", "agk", "skills")
	got, err := ResolveSkillsDir("", "", exe)
	if err != nil || got != want {
		t.Fatalf("got %q, %v; want %q", got, err, want)
	}
}

func TestSkillsDirNoneFound(t *testing.T) {
	root := realTemp(t)
	exe := filepath.Join(root, "go", "bin", "agk")
	touch(t, exe)
	_, err := ResolveSkillsDir("", "", exe)
	if err == nil || !strings.Contains(err.Error(), "--skills-dir") {
		t.Fatalf("err = %v, want mention of --skills-dir", err)
	}
}
