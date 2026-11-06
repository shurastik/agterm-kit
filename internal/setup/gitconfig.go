package setup

import (
	"errors"
	"os/exec"
	"strings"
)

// GlobalGitConfig reads a machine-wide git config value the way git resolves it
// outside a repository: the global (user) config wins over the system config.
// [include] files are followed (off by default with --global/--system) and
// paths are expanded. An unset key, or no git at all, is "" (the dependency
// report flags a missing git).
func GlobalGitConfig(key string) (string, error) {
	for _, scope := range []string{"--global", "--system"} {
		v, ok, err := gitConfigGet(scope, key)
		if err != nil || ok {
			return v, err
		}
	}
	return "", nil
}

func gitConfigGet(scope, key string) (string, bool, error) {
	out, err := exec.Command("git", "config", scope, "--includes", "--path", "--get", key).Output()
	var exit *exec.ExitError
	switch {
	case errors.Is(err, exec.ErrNotFound):
		return "", false, nil
	case errors.As(err, &exit) && exit.ExitCode() == 1:
		return "", false, nil // unset, or no such config file
	case err != nil:
		return "", false, err
	}
	return strings.TrimSpace(string(out)), true, nil
}
