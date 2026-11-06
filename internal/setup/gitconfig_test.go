package setup

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGlobalGitConfigFollowsIncludes(t *testing.T) {
	dir := realTemp(t)
	inc := filepath.Join(dir, "included")
	os.WriteFile(inc, []byte("[core]\n\texcludesFile = /dot/gitignore\n"), 0o644)
	global := filepath.Join(dir, "gitconfig")
	os.WriteFile(global, []byte("[include]\n\tpath = "+inc+"\n"), 0o644)
	t.Setenv("GIT_CONFIG_GLOBAL", global)
	t.Setenv("GIT_CONFIG_SYSTEM", filepath.Join(dir, "nosystem"))
	got, _, err := GlobalGitConfig("core.excludesFile")
	if err != nil || got != "/dot/gitignore" {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestGlobalGitConfigUnset(t *testing.T) {
	global := filepath.Join(realTemp(t), "gitconfig")
	os.WriteFile(global, nil, 0o644)
	t.Setenv("GIT_CONFIG_GLOBAL", global)
	t.Setenv("GIT_CONFIG_SYSTEM", global)
	got, _, err := GlobalGitConfig("core.excludesFile")
	if err != nil || got != "" {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestGlobalGitConfigFallsBackToSystem(t *testing.T) {
	dir := realTemp(t)
	global := filepath.Join(dir, "global")
	system := filepath.Join(dir, "system")
	os.WriteFile(global, nil, 0o644)
	os.WriteFile(system, []byte("[core]\n\texcludesFile = /etc/gitignore\n"), 0o644)
	t.Setenv("GIT_CONFIG_GLOBAL", global)
	t.Setenv("GIT_CONFIG_SYSTEM", system)
	if got, _, err := GlobalGitConfig("core.excludesFile"); err != nil || got != "/etc/gitignore" {
		t.Fatalf("system only: got %q, %v", got, err)
	}
	os.WriteFile(global, []byte("[core]\n\texcludesFile = /home/gitignore\n"), 0o644)
	if got, _, err := GlobalGitConfig("core.excludesFile"); err != nil || got != "/home/gitignore" {
		t.Fatalf("global wins: got %q, %v", got, err)
	}
}

func TestGlobalGitConfigExplicitlyEmpty(t *testing.T) {
	dir := realTemp(t)
	global := filepath.Join(dir, "global")
	os.WriteFile(global, []byte("[core]\n\texcludesFile =\n"), 0o644)
	t.Setenv("GIT_CONFIG_GLOBAL", global)
	t.Setenv("GIT_CONFIG_SYSTEM", filepath.Join(dir, "nosystem"))
	got, set, err := GlobalGitConfig("core.excludesFile")
	if err != nil || got != "" || !set {
		t.Fatalf("got %q, set %v, %v; want empty and set", got, set, err)
	}
	os.WriteFile(global, nil, 0o644)
	if _, set, _ := GlobalGitConfig("core.excludesFile"); set {
		t.Fatal("unset key reported as set")
	}
}

func TestGlobalGitConfigHonoursNoSystem(t *testing.T) {
	dir := realTemp(t)
	global := filepath.Join(dir, "global")
	system := filepath.Join(dir, "system")
	os.WriteFile(global, nil, 0o644)
	os.WriteFile(system, []byte("[core]\n\texcludesFile = /etc/gitignore\n"), 0o644)
	t.Setenv("GIT_CONFIG_GLOBAL", global)
	t.Setenv("GIT_CONFIG_SYSTEM", system)
	for _, v := range []string{"1", "true", "YES", "on"} {
		t.Setenv("GIT_CONFIG_NOSYSTEM", v)
		if got, set, err := GlobalGitConfig("core.excludesFile"); err != nil || set {
			t.Errorf("NOSYSTEM=%s: got %q set %v, %v; want unset", v, got, set, err)
		}
	}
	for _, v := range []string{"0", "false", "no", "off", ""} {
		t.Setenv("GIT_CONFIG_NOSYSTEM", v)
		if got, _, err := GlobalGitConfig("core.excludesFile"); err != nil || got != "/etc/gitignore" {
			t.Errorf("NOSYSTEM=%q: got %q, %v; want system value", v, got, err)
		}
	}
}
