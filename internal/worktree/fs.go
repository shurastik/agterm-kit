package worktree

import "os"

// exists reports whether something is at path (without following a final symlink).
func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}
