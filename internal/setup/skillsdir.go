// Package setup wires an installed agk into the machine: agent skill links,
// the global git ignore entry, and a dependency report.
package setup

import (
	"fmt"
	"os"
	"path/filepath"
)

// ResolveSkillsDir picks the directory holding the agk-* skills: the flag,
// then $AGK_SKILLS_DIR, then Homebrew's stable opt path when exe lives in a
// Cellar, then share/agk/skills beside the binary's parent directory.
// An explicit flag or env value must exist; it never falls through.
func ResolveSkillsDir(flag, env, exe string) (string, error) {
	if flag != "" {
		return explicitDir(flag, "--skills-dir")
	}
	if env != "" {
		return explicitDir(env, "AGK_SKILLS_DIR")
	}
	real, err := filepath.EvalSymlinks(exe)
	if err != nil {
		real = exe
	}
	var candidates []string
	if prefix, ok := cellarPrefix(real); ok {
		candidates = append(candidates, filepath.Join(prefix, "opt", "agk", "share", "agk", "skills"))
	}
	candidates = append(candidates, filepath.Join(filepath.Dir(filepath.Dir(real)), "share", "agk", "skills"))
	for _, c := range candidates {
		if isDir(c) {
			return c, nil
		}
	}
	return "", fmt.Errorf("no skills directory found next to %s; pass --skills-dir DIR", real)
}

func explicitDir(p, source string) (string, error) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	if !isDir(abs) {
		return "", fmt.Errorf("%s %s: not a directory", source, p)
	}
	return abs, nil
}

// cellarPrefix reports the Homebrew prefix for <prefix>/Cellar/agk/<version>/bin/agk.
func cellarPrefix(exe string) (string, bool) {
	bin := filepath.Dir(exe)
	version := filepath.Dir(bin)
	formula := filepath.Dir(version)
	cellar := filepath.Dir(formula)
	if filepath.Base(bin) != "bin" || filepath.Base(formula) != "agk" || filepath.Base(cellar) != "Cellar" {
		return "", false
	}
	return filepath.Dir(cellar), true
}

func isDir(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}
