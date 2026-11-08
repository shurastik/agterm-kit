package review

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// maxCollectAttempts bounds how often Collect re-reads a scope whose files
// changed under it.
const maxCollectAttempts = 3

// afterFirstFingerprint is a test hook: it runs between the first fingerprint
// of an attempt and the content reads, the window in which a file can change.
var afterFirstFingerprint func()

// Fingerprint kinds of an in-scope path.
const (
	kindFile      = "file"
	kindSymlink   = "symlink"
	kindSubmodule = "submodule"
	kindDeleted   = "deleted"
	kindIndex     = "index" // skip-worktree path absent on disk: the index entry stands in
)

// fingerprintOf hashes the working-tree state of the review scope: the merge
// base, then one sorted line per in-scope path (the union of the diff against
// the merge base and the untracked files) with its kind, executable bit and
// content. Whether a change is committed, staged or untracked does not enter.
func fingerprintOf(ctx context.Context, root, mergeBase string) (string, error) {
	lines, err := fingerprintLines(ctx, root, mergeBase)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	fmt.Fprintf(h, "%s\n", mergeBase)
	for _, l := range lines {
		io.WriteString(h, l)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// fingerprintLines returns the sorted "path NUL kind NUL mode NUL value\n"
// records of the scope of the repository at root against base.
func fingerprintLines(ctx context.Context, root, base string) ([]string, error) {
	diffNames, err := gitOut(ctx, root, diffArgs(base, "--name-only", "--no-renames", "-z")...)
	if err != nil {
		return nil, err
	}
	others, err := gitOut(ctx, root, "ls-files", "--others", "--exclude-standard", "-z", "--", ".", ":(exclude)"+agkDir)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var paths []string
	for _, list := range []string{diffNames, others} {
		for _, p := range strings.Split(list, "\x00") {
			p = strings.TrimSuffix(p, "/")
			if p != "" && !seen[p] {
				seen[p] = true
				paths = append(paths, p)
			}
		}
	}
	sort.Strings(paths)
	filemode := filemodeTracked(ctx, root)
	lines := make([]string, 0, len(paths))
	for _, p := range paths {
		kind, mode, value, err := pathState(ctx, root, base, p, filemode)
		if err != nil {
			return nil, err
		}
		lines = append(lines, p+"\x00"+kind+"\x00"+mode+"\x00"+value+"\n")
	}
	return lines, nil
}

// pathState describes rel (relative to root) as the working tree has it now.
func pathState(ctx context.Context, root, base, rel string, filemode bool) (kind, mode, value string, err error) {
	p := filepath.Join(root, rel)
	fi, err := os.Lstat(p)
	switch {
	case err != nil:
		// A skip-worktree file (sparse checkout) is absent on disk but git
		// diffs its index content: record that, not a deletion.
		if f, ok := indexEntry(ctx, root, rel); ok && f[0] == "S" {
			return kindIndex, f[1], f[2], nil
		}
		return kindDeleted, "", "", nil
	case fi.Mode()&os.ModeSymlink != 0:
		target, err := os.Readlink(p)
		if err != nil {
			return kindDeleted, "", "", nil
		}
		return kindSymlink, "", target, nil
	case fi.Mode().IsRegular():
		mode = "644"
		if fi.Mode()&0o100 != 0 { // git looks at the owner execute bit only
			mode = "755"
		}
		if !filemode {
			// core.filemode=false: git takes a tracked file's mode from the index
			// and stores a new file as 644 whatever its permissions.
			mode = "644"
			if m, ok := indexMode(ctx, root, rel); ok {
				mode = m
			}
		}
		f, err := os.Open(p)
		if err != nil {
			return "", "", "", fmt.Errorf("fingerprint %s: %w", rel, err)
		}
		defer f.Close()
		h := sha256.New()
		if _, err := io.Copy(h, f); err != nil {
			return "", "", "", fmt.Errorf("fingerprint %s: %w", rel, err)
		}
		return kindFile, mode, hex.EncodeToString(h.Sum(nil)), nil
	case fi.IsDir():
		if _, err := os.Lstat(filepath.Join(p, ".git")); err == nil {
			v, err := submoduleState(ctx, p)
			if err != nil {
				return "", "", "", err
			}
			return kindSubmodule, "", v, nil
		}
		if sha, ok := gitlinkSHA(ctx, root, base, rel); ok {
			return kindSubmodule, "", "uninitialized:" + sha, nil
		}
	}
	return kindDeleted, "", "", nil
}

// filemodeTracked reports whether git trusts the working tree's executable
// bit in the repository at root (core.filemode, true unless set to false).
func filemodeTracked(ctx context.Context, root string) bool {
	out, err := gitOut(ctx, root, "config", "--type=bool", "core.filemode")
	return err != nil || strings.TrimSpace(out) != "false"
}

// entryFor returns the whitespace-separated metadata fields of the entry for
// exactly rel in the -z output of ls-files --stage or ls-tree ("meta TAB path"
// records), so a neighbouring path that a pathspec also matched is not taken.
func entryFor(out, rel string) ([]string, bool) {
	for _, rec := range strings.Split(out, "\x00") {
		meta, path, ok := strings.Cut(rec, "\t")
		if ok && path == rel {
			return strings.Fields(meta), true
		}
	}
	return nil, false
}

// indexEntry returns the index entry of exactly rel as the fields tag (S for
// skip-worktree, H for a normal entry, ...), mode, sha and stage. The path goes
// in as a literal pathspec: "run[1].sh" is not a glob here.
func indexEntry(ctx context.Context, root, rel string) ([]string, bool) {
	out, err := gitOut(ctx, root, "ls-files", "-t", "--stage", "-z", "--", ":(literal)"+rel)
	if err != nil {
		return nil, false
	}
	f, ok := entryFor(out, rel)
	return f, ok && len(f) >= 4
}

// indexMode returns "755" or "644" for a tracked regular file from its index
// entry, and false for an untracked path or another kind of entry.
func indexMode(ctx context.Context, root, rel string) (string, bool) {
	f, ok := indexEntry(ctx, root, rel)
	if !ok {
		return "", false
	}
	switch f[1] {
	case "100755":
		return "755", true
	case "100644":
		return "644", true
	}
	return "", false
}

// gitlinkSHA returns the commit the superproject records for the submodule at
// rel: the index entry, else the one in the merge base tree when the path was
// dropped from the index.
func gitlinkSHA(ctx context.Context, root, base, rel string) (string, bool) {
	if f, ok := indexEntry(ctx, root, rel); ok && f[1] == "160000" {
		return f[2], true
	}
	if out, err := gitOut(ctx, root, "ls-tree", "-z", base, "--", rel); err == nil {
		if f, ok := entryFor(out, rel); ok && len(f) >= 3 && f[0] == "160000" {
			return f[2], true
		}
	}
	return "", false
}

// submoduleState is the submodule's HEAD plus the fingerprint of its own scope
// against that HEAD: every edit in it counts, not just new commits.
func submoduleState(ctx context.Context, dir string) (string, error) {
	head, err := gitOut(ctx, dir, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	head = strings.TrimSpace(head)
	inner, err := fingerprintOf(ctx, dir, head)
	if err != nil {
		return "", err
	}
	return head + ":" + inner, nil
}

// collectConsistent runs collect until the scope's fingerprint is the same
// before and after it, so the content and the fingerprint describe one state.
func collectConsistent(ctx context.Context, root, mergeBase string, collect func() error) (string, error) {
	for attempt := 0; attempt < maxCollectAttempts; attempt++ {
		before, err := fingerprintOf(ctx, root, mergeBase)
		if err != nil {
			return "", err
		}
		if afterFirstFingerprint != nil {
			afterFirstFingerprint()
		}
		if err := collect(); err != nil {
			return "", err
		}
		after, err := fingerprintOf(ctx, root, mergeBase)
		if err != nil {
			return "", err
		}
		if before == after {
			return before, nil
		}
	}
	return "", fmt.Errorf("files changed during collection (%d attempts); finish editing and retry", maxCollectAttempts)
}
