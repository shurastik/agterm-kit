package review

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"
)

// ErrNothingToReview means the review scope is empty.
var ErrNothingToReview = errors.New("nothing to review")

// MaxUntrackedBytes is the largest untracked file whose content goes into the prompt.
const MaxUntrackedBytes = 64 << 10

// DiffFlags make git show the raw change: no external diff tools, no textconv
// filters and no submodule hiding from config, any of which could drop changes
// from the review.
var DiffFlags = []string{"--no-color", "--no-ext-diff", "--no-textconv", "--ignore-submodules=none"}

// diffArgs is the git diff argv for the review scope; the prompt tells codex
// to run the same command to read past the size cap.
func diffArgs(mergeBase string, extra ...string) []string {
	args := append([]string{"diff"}, DiffFlags...)
	args = append(args, extra...)
	return append(args, mergeBase, "--", ".", ":(exclude)"+agkDir)
}

// ReadRestCommand is the shell command that reproduces the review diff. A
// non-empty root renders `git -C <root> diff …`, for a reviewer whose working
// directory is not the repository.
func ReadRestCommand(mergeBase, root string) string {
	parts := []string{"git"}
	if root != "" {
		parts = append(parts, "-C", shellWord(root))
	}
	for _, a := range diffArgs(mergeBase) {
		parts = append(parts, shellWord(a))
	}
	return strings.Join(parts, " ")
}

// shellWord quotes a command line word unless it is plain.
func shellWord(a string) string {
	if plainArg.MatchString(a) {
		return a
	}
	return "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
}

var plainArg = regexp.MustCompile(`^[A-Za-z0-9._/=-]+$`)

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
	// Fingerprint identifies the working-tree state this scope was read from;
	// see fingerprintOf.
	Fingerprint string
}

// Collect gathers the review scope of the repo containing dir against base.
// An empty scope is ErrNothingToReview, returned with the scope's Root, Base
// and MergeBase set and nothing else.
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
	collect := func() error {
		var err error
		if s.Diff, err = gitOut(ctx, s.Root, diffArgs(s.MergeBase)...); err != nil {
			return err
		}
		if s.Stat, err = gitOut(ctx, s.Root, diffArgs(s.MergeBase, "--stat")...); err != nil {
			return err
		}
		list, err := gitOut(ctx, s.Root, "ls-files", "--others", "--exclude-standard", "-z", "--", ".", exclude)
		if err != nil {
			return err
		}
		s.Untracked = nil
		for _, p := range strings.Split(list, "\x00") {
			if p != "" {
				s.Untracked = append(s.Untracked, readUntracked(s.Root, p))
			}
		}
		return nil
	}
	if s.Fingerprint, err = collectConsistent(ctx, s.Root, s.MergeBase, collect); err != nil {
		return Scope{}, err
	}
	if s.Diff == "" && len(s.Untracked) == 0 {
		// The merge base is still worth reporting: `agk review fresh` prints it.
		return Scope{Root: s.Root, Base: s.Base, MergeBase: s.MergeBase}, ErrNothingToReview
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
	if !utf8.Valid(b) {
		u.Omitted = "not valid UTF-8 (legacy encoding?), read it directly"
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

// IsAncestor reports whether commit is an ancestor of HEAD (HEAD itself
// included) in the repository holding dir. A commit the repository does not
// have is an error naming it; history that merely does not contain it, an
// unrelated one included, is false with no error.
func IsAncestor(ctx context.Context, dir, commit string) (bool, error) {
	if _, err := gitOut(ctx, dir, "cat-file", "-e", commit+"^{commit}"); err != nil {
		return false, fmt.Errorf("commit %s is not in the repository: %w", commit, err)
	}
	cmd := exec.CommandContext(ctx, "git", "merge-base", "--is-ancestor", commit, "HEAD")
	cmd.Dir = dir
	var errb bytes.Buffer
	cmd.Stderr = &errb
	err := cmd.Run()
	if err == nil {
		return true, nil
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) && ee.ExitCode() == 1 {
		return false, nil
	}
	return false, fmt.Errorf("git merge-base --is-ancestor %s HEAD: %v: %s", commit, err, strings.TrimSpace(errb.String()))
}

// RepoRoot returns the top-level directory of the git repository holding dir.
func RepoRoot(ctx context.Context, dir string) (string, error) {
	out, err := gitOut(ctx, dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}
