package setup

import (
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// GlobalGitConfig reads a machine-wide git config value the way git resolves it
// outside a repository: the global (user) config wins over the system config.
// [include] files are followed (off by default with --global/--system) and
// paths are expanded. set tells an explicitly empty value from an unset key;
// no git at all counts as unset (the dependency report flags a missing git).
func GlobalGitConfig(key string) (value string, set bool, err error) {
	scopes := []string{"--global", "--system"}
	if noSystem(os.Getenv("GIT_CONFIG_NOSYSTEM")) {
		scopes = scopes[:1] // git skips the system config, but an explicit --system would still read it
	}
	for _, scope := range scopes {
		v, ok, err := gitConfigGet(scope, key)
		if err != nil || ok {
			return v, ok, err
		}
	}
	return "", false, nil
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

// noSystem parses GIT_CONFIG_NOSYSTEM like git's boolean env vars: true/yes/on
// or any non-zero integer. Unknown values count as false (git rejects them, so
// the system config stays on).
func noSystem(v string) bool {
	v = strings.ToLower(strings.TrimSpace(v))
	switch v {
	case "true", "yes", "on":
		return true
	}
	n, err := strconv.Atoi(v)
	return err == nil && n != 0
}
