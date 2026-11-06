package setup

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func ignoreCfg(home string, vars map[string]string, excludes string) Config {
	return Config{
		Home:      home,
		Getenv:    func(k string) string { return vars[k] },
		GitConfig: func(string) (string, error) { return excludes, nil },
	}
}

func TestIgnorePathSources(t *testing.T) {
	home := realTemp(t)
	for _, tc := range []struct {
		name     string
		vars     map[string]string
		excludes string
		want     string
	}{
		{"git config", map[string]string{"XDG_CONFIG_HOME": "/xdg"}, "/dot/gitignore", "/dot/gitignore"},
		{"xdg", map[string]string{"XDG_CONFIG_HOME": "/xdg"}, "", "/xdg/git/ignore"},
		{"default", map[string]string{}, "", filepath.Join(home, ".config", "git", "ignore")},
	} {
		got, err := excludesPath(ignoreCfg(home, tc.vars, tc.excludes))
		if err != nil || got != tc.want {
			t.Errorf("%s: got %q, %v; want %q", tc.name, got, err, tc.want)
		}
	}
}

func TestIgnoreGitConfigError(t *testing.T) {
	c := ignoreCfg(realTemp(t), map[string]string{}, "")
	c.GitConfig = func(string) (string, error) { return "", errors.New("boom") }
	if _, err := planIgnore(c); err == nil {
		t.Fatal("want error")
	}
}

func TestIgnoreMissingFileAppends(t *testing.T) {
	home := realTemp(t)
	acts, err := planIgnore(ignoreCfg(home, map[string]string{}, ""))
	if err != nil {
		t.Fatal(err)
	}
	if len(acts) != 1 || acts[0].Kind != KindAppend || acts[0].Path != filepath.Join(home, ".config", "git", "ignore") {
		t.Fatalf("%+v", acts)
	}
}

func TestIgnorePresenceSpellings(t *testing.T) {
	home := realTemp(t)
	p := filepath.Join(home, "ignore")
	for _, content := range []string{
		".agk\n", ".agk/\n", "/.agk\n", "/.agk/\n", "foo\n  .agk/  \nbar", "**/.claude/x\n.agk/",
	} {
		os.WriteFile(p, []byte(content), 0o644)
		acts, err := planIgnore(ignoreCfg(home, map[string]string{}, p))
		if err != nil || len(acts) != 1 || acts[0].Kind != KindOK {
			t.Errorf("%q: %+v, %v; want ok", content, acts, err)
		}
	}
	for _, content := range []string{"", "**/.claude/settings.local.json\n", ".agkx/\n", "# .agk/\n", "sub/.agk/\n"} {
		os.WriteFile(p, []byte(content), 0o644)
		acts, _ := planIgnore(ignoreCfg(home, map[string]string{}, p))
		if len(acts) != 1 || acts[0].Kind != KindAppend {
			t.Errorf("%q: %+v; want append", content, acts)
		}
	}
}
