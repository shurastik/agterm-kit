package setup

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/shurastik/agterm-kit/internal/fsx"
)

// Plan inspects the machine and returns every action and check, links first,
// then the excludes file, then dependencies. It changes nothing.
func Plan(c Config) ([]Action, error) {
	acts, err := planLinks(c)
	if err != nil {
		return nil, err
	}
	ign, err := planIgnore(c)
	if err != nil {
		return nil, err
	}
	acts = append(acts, ign...)
	return append(acts, planDeps(c)...), nil
}

// Apply performs the mutating actions. It keeps going after a failure and
// returns all errors joined.
func Apply(acts []Action) error {
	var errs []error
	for _, a := range acts {
		var err error
		switch a.Kind {
		case KindCreate, KindRepoint:
			err = putLink(a.Path, a.Target)
		case KindRemove:
			err = removeLink(a.Path)
		case KindAppend:
			err = appendIgnore(a.Path)
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("%s %s: %w", a.Kind, a.Path, err))
		}
	}
	return errors.Join(errs...)
}

// putLink points link at target by renaming a fresh symlink over it, so the
// link is never missing while it is replaced.
func putLink(link, target string) error {
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		return err
	}
	if fi, err := os.Lstat(link); err == nil && fi.Mode()&fs.ModeSymlink == 0 {
		return errors.New("not a symlink")
	}
	var b [6]byte
	if _, err := rand.Read(b[:]); err != nil {
		return err
	}
	tmp := filepath.Join(filepath.Dir(link), "."+filepath.Base(link)+".tmp-"+hex.EncodeToString(b[:]))
	if err := os.Symlink(target, tmp); err != nil {
		return err
	}
	if err := os.Rename(tmp, link); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

func removeLink(link string) error {
	fi, err := os.Lstat(link)
	if err != nil {
		return err
	}
	if fi.Mode()&fs.ModeSymlink == 0 {
		return errors.New("not a symlink")
	}
	return os.Remove(link)
}

// appendIgnore adds IgnoreLine to the excludes file, editing a symlinked
// file at its target so a dotfiles link survives.
func appendIgnore(path string) error {
	target := path
	if fi, err := os.Lstat(path); err == nil && fi.Mode()&fs.ModeSymlink != 0 {
		if target, err = filepath.EvalSymlinks(path); err != nil {
			return err
		}
	}
	data, err := os.ReadFile(target)
	mode := fs.FileMode(0o644)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
	case err != nil:
		return err
	default:
		if fi, err := os.Stat(target); err == nil {
			mode = fi.Mode().Perm()
		}
	}
	if len(data) > 0 && data[len(data)-1] != '\n' {
		data = append(data, '\n')
	}
	data = append(data, IgnoreLine+"\n"...)
	return fsx.ReplaceFile(target, data, mode)
}
