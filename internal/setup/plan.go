package setup

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Kind is what an action does, or what a check found.
type Kind string

const (
	KindOK       Kind = "ok"       // already in place
	KindCreate   Kind = "create"   // new skill link
	KindRepoint  Kind = "repoint"  // agk-named symlink pointing elsewhere
	KindRemove   Kind = "remove"   // stale or legacy symlink
	KindConflict Kind = "conflict" // regular file or dir where a link belongs
	KindSkip     Kind = "skip"     // agent not set up on this machine
	KindAppend   Kind = "append"   // add the .agk/ line to the excludes file
	KindFound    Kind = "found"    // dependency present
	KindMissing  Kind = "missing"  // dependency absent
)

// LegacyLink is the pre-agk name of the review skill.
const LegacyLink = "agterm-codex-review"

// Action is one planned step or check result.
type Action struct {
	Kind     Kind
	Path     string // link, excludes file, agent dir, or tool name
	Target   string // link target or resolved tool path
	Detail   string
	Optional bool // missing tool that does not fail setup
}

// Mutates reports whether applying the action changes the filesystem.
func (a Action) Mutates() bool {
	switch a.Kind {
	case KindCreate, KindRepoint, KindRemove, KindAppend:
		return true
	}
	return false
}

// Failed reports whether the action makes setup exit non-zero.
func (a Action) Failed() bool {
	return a.Kind == KindConflict || a.Kind == KindMissing && !a.Optional
}

// Config is everything Plan reads from the machine.
type Config struct {
	Home      string
	SkillsDir string // resolved, absolute
	Getenv    func(string) string
	// GitConfig returns a machine-wide git config value with paths expanded and
	// whether the key is set at all (an empty value is not the same as unset).
	GitConfig func(key string) (value string, set bool, err error)
	LookPath  func(string) (string, error)
	IsExec    func(string) bool
}

type agent struct{ name, dir string }

func (c Config) agents() []agent {
	dir := func(env, def string) string {
		if v := c.Getenv(env); v != "" {
			return v
		}
		return filepath.Join(c.Home, def)
	}
	return []agent{
		{"Claude Code", dir("CLAUDE_CONFIG_DIR", ".claude")},
		{"Codex", dir("CODEX_HOME", ".codex")},
	}
}

// skills lists agk-* subdirectories of the skills dir that hold a SKILL.md.
func (c Config) skills() ([]string, error) {
	entries, err := os.ReadDir(c.SkillsDir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		n := e.Name()
		if !strings.HasPrefix(n, "agk-") || !isDir(filepath.Join(c.SkillsDir, n)) {
			continue
		}
		if _, err := os.Stat(filepath.Join(c.SkillsDir, n, "SKILL.md")); err == nil {
			names = append(names, n)
		}
	}
	return names, nil
}

func planLinks(c Config) ([]Action, error) {
	if err := c.checkSource(); err != nil {
		return nil, err
	}
	names, err := c.skills()
	if err != nil {
		return nil, err
	}
	if len(names) == 0 {
		// Planning against it would remove every installed agk-* link as stale.
		return nil, fmt.Errorf("no agk-* skills in %s", c.SkillsDir)
	}
	shipped := map[string]bool{}
	for _, n := range names {
		shipped[n] = true
	}
	var acts []Action
	planned := map[string]string{} // physical skills dir -> agent that covers it
	for _, ag := range c.agents() {
		if !isDir(ag.dir) {
			acts = append(acts, Action{Kind: KindSkip, Path: ag.dir, Detail: ag.name + " is not set up"})
			continue
		}
		skillsDir := filepath.Join(ag.dir, "skills")
		if real, err := filepath.EvalSymlinks(skillsDir); err == nil {
			if other, ok := planned[real]; ok {
				acts = append(acts, Action{Kind: KindSkip, Path: skillsDir, Detail: "same skills directory as " + other})
				continue
			}
			planned[real] = ag.name
		}
		for _, n := range names {
			acts = append(acts, planLink(filepath.Join(skillsDir, n), filepath.Join(c.SkillsDir, n)))
		}
		stale, err := staleLinks(skillsDir, shipped)
		if err != nil {
			return nil, err
		}
		acts = append(acts, stale...)
	}
	return acts, nil
}

// checkSource refuses a skills dir that is an agent's own skills directory
// (directly or through symlinks): linking it into itself makes self-loops.
func (c Config) checkSource() error {
	src, err := filepath.EvalSymlinks(c.SkillsDir)
	if err != nil {
		return err
	}
	agentDirs := map[string]string{} // physical skills dir -> agent name
	for _, ag := range c.agents() {
		if dst, err := filepath.EvalSymlinks(filepath.Join(ag.dir, "skills")); err == nil {
			agentDirs[dst] = ag.name
		}
	}
	if name, ok := agentDirs[src]; ok {
		return fmt.Errorf("%s is the %s agent skills directory, not an agk skills source", c.SkillsDir, name)
	}
	// A skill reached through a link inside an agent skills dir would be linked to itself.
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), "agk-") {
			continue
		}
		hops, err := linkChain(filepath.Join(src, e.Name()))
		if err != nil {
			continue // a broken skill is skipped by skills() anyway
		}
		for _, hop := range hops {
			if name, ok := agentDirs[filepath.Dir(hop)]; ok {
				return fmt.Errorf("skill %s resolves through the %s agent skills directory (%s)", e.Name(), name, hop)
			}
		}
	}
	return nil
}

func planLink(link, target string) Action {
	fi, err := os.Lstat(link)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return Action{Kind: KindCreate, Path: link, Target: target}
	case err != nil:
		return Action{Kind: KindConflict, Path: link, Target: target, Detail: err.Error()}
	case fi.Mode()&fs.ModeSymlink == 0:
		return Action{Kind: KindConflict, Path: link, Target: target, Detail: "not a symlink; left unchanged"}
	}
	if cur, err := os.Readlink(link); err == nil && cur == target {
		return Action{Kind: KindOK, Path: link, Target: target}
	}
	return Action{Kind: KindRepoint, Path: link, Target: target}
}

// staleLinks plans removal of agk-* symlinks no longer shipped and of the legacy link.
func staleLinks(dir string, shipped map[string]bool) ([]Action, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var acts []Action
	for _, e := range entries {
		n := e.Name()
		ours := strings.HasPrefix(n, "agk-") && !shipped[n] || n == LegacyLink
		if !ours || e.Type()&fs.ModeSymlink == 0 {
			continue
		}
		acts = append(acts, Action{Kind: KindRemove, Path: filepath.Join(dir, n)})
	}
	sort.Slice(acts, func(i, j int) bool { return acts[i].Path < acts[j].Path })
	return acts, nil
}
