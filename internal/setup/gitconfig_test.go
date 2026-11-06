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
	got, err := GlobalGitConfig("core.excludesFile")
	if err != nil || got != "/dot/gitignore" {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestGlobalGitConfigUnset(t *testing.T) {
	global := filepath.Join(realTemp(t), "gitconfig")
	os.WriteFile(global, nil, 0o644)
	t.Setenv("GIT_CONFIG_GLOBAL", global)
	got, err := GlobalGitConfig("core.excludesFile")
	if err != nil || got != "" {
		t.Fatalf("got %q, %v", got, err)
	}
}
