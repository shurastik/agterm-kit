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
	got, err := GlobalGitConfig("core.excludesFile")
	if err != nil || got != "/dot/gitignore" {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestGlobalGitConfigUnset(t *testing.T) {
	global := filepath.Join(realTemp(t), "gitconfig")
	os.WriteFile(global, nil, 0o644)
	t.Setenv("GIT_CONFIG_GLOBAL", global)
	t.Setenv("GIT_CONFIG_SYSTEM", global)
	got, err := GlobalGitConfig("core.excludesFile")
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
	if got, err := GlobalGitConfig("core.excludesFile"); err != nil || got != "/etc/gitignore" {
		t.Fatalf("system only: got %q, %v", got, err)
	}
	os.WriteFile(global, []byte("[core]\n\texcludesFile = /home/gitignore\n"), 0o644)
	if got, err := GlobalGitConfig("core.excludesFile"); err != nil || got != "/home/gitignore" {
		t.Fatalf("global wins: got %q, %v", got, err)
	}
}
