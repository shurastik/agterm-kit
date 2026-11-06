package setup

import (
	"bufio"
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// IgnoreLine is the line setup adds to the global git excludes file.
const IgnoreLine = ".agk/"

// excludesPath is git's global excludes file: core.excludesFile, else
// $XDG_CONFIG_HOME/git/ignore, else ~/.config/git/ignore.
func excludesPath(c Config) (string, error) {
	p, err := c.GitConfig("core.excludesFile")
	if err != nil {
		return "", err
	}
	if p != "" {
		return p, nil
	}
	if x := c.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "git", "ignore"), nil
	}
	return filepath.Join(c.Home, ".config", "git", "ignore"), nil
}

func planIgnore(c Config) ([]Action, error) {
	p, err := excludesPath(c)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	if ignoresAgk(data) {
		return []Action{{Kind: KindOK, Path: p, Detail: IgnoreLine}}, nil
	}
	return []Action{{Kind: KindAppend, Path: p, Detail: IgnoreLine}}, nil
}

func ignoresAgk(data []byte) bool {
	s := bufio.NewScanner(bytes.NewReader(data))
	for s.Scan() {
		switch strings.TrimSpace(s.Text()) {
		case ".agk", ".agk/", "/.agk", "/.agk/":
			return true
		}
	}
	return false
}
