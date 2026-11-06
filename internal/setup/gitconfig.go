package setup

import (
	"errors"
	"os/exec"
	"strings"
)

// GlobalGitConfig reads a global git config value, following [include] files
// (off by default with --global) and expanding paths. An unset
// key, or no git at all, is "" (the dependency report flags a missing git).
func GlobalGitConfig(key string) (string, error) {
	out, err := exec.Command("git", "config", "--global", "--includes", "--path", "--get", key).Output()
	var exit *exec.ExitError
	switch {
	case errors.Is(err, exec.ErrNotFound):
		return "", nil
	case errors.As(err, &exit) && exit.ExitCode() == 1:
		return "", nil
	case err != nil:
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
