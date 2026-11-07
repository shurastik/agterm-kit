package review

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// Plan-review layout under the repository root.
const (
	changesDir = "openspec/changes"
	specsDir   = "openspec/specs"
)

// PlanScope is what a plan review covers: the files of one OpenSpec change
// and the current main spec of each capability the change has a delta for.
// Paths are relative to Root, with forward slashes.
type PlanScope struct {
	Root      string
	Change    string
	Files     []UntrackedFile
	MainSpecs []UntrackedFile
}

// CollectPlan gathers the plan review scope of change in the repo containing
// dir. Files are read from disk whatever their git state.
func CollectPlan(ctx context.Context, dir, change string) (PlanScope, error) {
	if change == "" || change == "." || change == ".." || strings.ContainsAny(change, `/\`) {
		return PlanScope{}, fmt.Errorf("invalid change name %q", change)
	}
	root, err := RepoRoot(ctx, dir)
	if err != nil {
		return PlanScope{}, err
	}
	s := PlanScope{Root: root, Change: change}
	rel := path.Join(changesDir, change)
	link, err := firstSymlink(root, rel)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return PlanScope{}, fmt.Errorf("change %q not found: no %s", change, rel)
	case err != nil:
		return PlanScope{}, fmt.Errorf("change %q: %v", change, err)
	case link != "":
		return PlanScope{}, fmt.Errorf("change %q: %s is a symlink, not followed", change, link)
	}
	if fi, err := os.Lstat(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
		return PlanScope{}, fmt.Errorf("change %q: %v", change, err)
	} else if !fi.IsDir() {
		return PlanScope{}, fmt.Errorf("change %q: %s is not a directory", change, rel)
	}
	var paths []string
	// WalkDir does not follow symlinks: a linked file or dir is an entry of
	// its own, which readUntracked reports as not followed.
	err = filepath.WalkDir(filepath.Join(root, filepath.FromSlash(rel)), func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		r, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		paths = append(paths, filepath.ToSlash(r))
		return nil
	})
	if err != nil {
		return PlanScope{}, fmt.Errorf("read change %q: %v", change, err)
	}
	if len(paths) == 0 {
		return PlanScope{}, ErrNothingToReview
	}
	sort.Strings(paths)
	deltaPrefix := rel + "/specs/"
	for _, p := range paths {
		s.Files = append(s.Files, readUntracked(root, p))
		capName, ok := strings.CutPrefix(p, deltaPrefix)
		if !ok {
			continue
		}
		capName, ok = strings.CutSuffix(capName, "/spec.md")
		if !ok || strings.Contains(capName, "/") {
			continue
		}
		main := path.Join(specsDir, capName, "spec.md")
		link, err := firstSymlink(root, main)
		switch {
		case errors.Is(err, fs.ErrNotExist):
		case link != "" && link != main:
			// A linked parent dir is not followed, like a linked file.
			s.MainSpecs = append(s.MainSpecs, UntrackedFile{Path: main, Omitted: link + " is a symlink, not followed"})
		default:
			s.MainSpecs = append(s.MainSpecs, readUntracked(root, main))
		}
	}
	return s, nil
}

// firstSymlink returns the first component path of rel (relative to root,
// slash-separated) that is a symlink, or "" when none is. Checking every
// component keeps a linked parent dir from pulling outside files into the
// scope. A missing component yields an fs.ErrNotExist error.
func firstSymlink(root, rel string) (string, error) {
	cur := ""
	for _, part := range strings.Split(rel, "/") {
		cur = path.Join(cur, part)
		fi, err := os.Lstat(filepath.Join(root, filepath.FromSlash(cur)))
		if err != nil {
			return "", err
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			return cur, nil
		}
	}
	return "", nil
}
