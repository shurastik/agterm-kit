package review

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ErrNothingToReview means the review scope is empty.
var ErrNothingToReview = errors.New("nothing to review")

// MaxUntrackedBytes is the largest untracked file whose content goes into the prompt.
const MaxUntrackedBytes = 64 << 10

// agkDir is where run artifacts live inside a reviewed repo; never reviewed.
const agkDir = ".agk"

// UntrackedFile is a new file not yet known to git. Content is empty and
// Omitted holds the reason when the content is not included.
type UntrackedFile struct {
	Path    string
	Content string
	Omitted string
}

// Scope is what a review covers: everything between the merge base of the
// base ref and the working tree.
type Scope struct {
	Root      string
	Base      string
	MergeBase string
	Diff      string
	Stat      string
	Untracked []UntrackedFile
}

// Collect gathers the review scope of the repo containing dir against base.
func Collect(ctx context.Context, dir, base string) (Scope, error) {
	root, err := gitOut(ctx, dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return Scope{}, err
	}
	s := Scope{Root: strings.TrimSpace(root), Base: base}
	mb, err := gitOut(ctx, s.Root, "merge-base", base, "HEAD")
	if err != nil {
		return Scope{}, fmt.Errorf("merge base of %q and HEAD: %w", base, err)
	}
	s.MergeBase = strings.TrimSpace(mb)
	exclude := ":(exclude)" + agkDir
	if s.Diff, err = gitOut(ctx, s.Root, "diff", "--no-color", "--no-ext-diff", s.MergeBase, "--", ".", exclude); err != nil {
		return Scope{}, err
	}
	if s.Stat, err = gitOut(ctx, s.Root, "diff", "--no-color", "--stat", s.MergeBase, "--", ".", exclude); err != nil {
		return Scope{}, err
	}
	list, err := gitOut(ctx, s.Root, "ls-files", "--others", "--exclude-standard", "-z", "--", ".", exclude)
	if err != nil {
		return Scope{}, err
	}
	for _, p := range strings.Split(list, "\x00") {
		if p != "" {
			s.Untracked = append(s.Untracked, readUntracked(s.Root, p))
		}
	}
	if s.Diff == "" && len(s.Untracked) == 0 {
		return Scope{}, ErrNothingToReview
	}
	return s, nil
}

func readUntracked(root, rel string) UntrackedFile {
	u := UntrackedFile{Path: rel}
	p := filepath.Join(root, rel)
	fi, err := os.Lstat(p)
	switch {
	case err != nil:
		u.Omitted = "unreadable"
		return u
	case fi.Mode()&os.ModeSymlink != 0:
		u.Omitted = "symlink, not followed"
		return u
	case !fi.Mode().IsRegular():
		u.Omitted = "not a regular file"
		return u
	case fi.Size() > MaxUntrackedBytes:
		u.Omitted = fmt.Sprintf("too large (%d bytes)", fi.Size())
		return u
	}
	b, err := os.ReadFile(p)
	if err != nil {
		u.Omitted = "unreadable"
		return u
	}
	head := b
	if len(head) > 8000 {
		head = head[:8000]
	}
	if bytes.IndexByte(head, 0) >= 0 {
		u.Omitted = "binary"
		return u
	}
	u.Content = string(b)
	return u
}

func gitOut(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(errb.String()))
	}
	return out.String(), nil
}
