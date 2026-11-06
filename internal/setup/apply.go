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
	target, err := followLinks(path)
	if err != nil {
		return err
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

// followLinks resolves a chain of symlinks at path, allowing the final target
// (and its directories) not to exist yet, unlike filepath.EvalSymlinks.
func followLinks(path string) (string, error) {
	hops, err := linkChain(path)
	if err != nil {
		return "", err
	}
	return hops[len(hops)-1], nil
}

// linkChain returns every path a symlink chain visits, starting with path; each
// hop after the first has a physical (symlink-free) directory. The last element
// is not a symlink and may not exist.
func linkChain(path string) ([]string, error) {
	hops := []string{path}
	for range 40 {
		fi, err := os.Lstat(path)
		if errors.Is(err, fs.ErrNotExist) {
			return hops, nil
		}
		if err != nil {
			return nil, err
		}
		if fi.Mode()&fs.ModeSymlink == 0 {
			return hops, nil
		}
		next, err := os.Readlink(path)
		if err != nil {
			return nil, err
		}
		if !filepath.IsAbs(next) {
			// Relative to the link's physical directory, which exists since the link does.
			dir, err := filepath.EvalSymlinks(filepath.Dir(path))
			if err != nil {
				return nil, err
			}
			next = filepath.Join(dir, next)
		}
		// Keep every hop's directory physical, so callers can compare it with real paths.
		if dir, err := filepath.EvalSymlinks(filepath.Dir(next)); err == nil {
			next = filepath.Join(dir, filepath.Base(next))
		}
		path = next
		hops = append(hops, path)
	}
	return nil, fmt.Errorf("%s: too many levels of symbolic links", path)
}
