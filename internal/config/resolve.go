package config

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
)

// Worker is the resolved configuration of one role.
type Worker struct {
	Harness        string   `json:"harness"`
	Model          string   `json:"model,omitempty"`
	Effort         string   `json:"effort,omitempty"`
	ExtraArgs      []string `json:"extra_args,omitempty"`
	PermissionMode string   `json:"permission_mode,omitempty"`
}

// Flags are the raw role flags, HARNESS[:MODEL]; empty when not given.
type Flags struct {
	Applier, Reviewer string
}

// Resolved is the configuration of both roles and where each key came from:
// "flag", a file path or "default".
type Resolved struct {
	Applier, Reviewer Worker
	Sources           map[string]map[string]string
}

// Source names.
const (
	SourceFlag    = "flag"
	SourceDefault = "default"
)

// DefaultPermissionMode is a claude applier's permission mode unless configured.
const DefaultPermissionMode = "auto"

func defaults(role string) Worker {
	if role == Applier {
		return Worker{Harness: Claude, PermissionMode: DefaultPermissionMode}
	}
	return Worker{Harness: Codex}
}

// state tracks one role while layers are applied.
type state struct {
	role string
	w    Worker
	src  map[string]string
}

func newState(role string) *state {
	s := &state{role: role, w: defaults(role), src: map[string]string{}}
	for _, k := range []string{"harness", "model", "effort", "extra_args"} {
		s.src[k] = SourceDefault
	}
	s.fixPermission()
	return s
}

// fixPermission keeps permission_mode only for a claude applier.
func (s *state) fixPermission() {
	if s.role == Applier && s.w.Harness == Claude {
		if _, ok := s.src["permission_mode"]; !ok {
			s.w.PermissionMode = DefaultPermissionMode
			s.src["permission_mode"] = SourceDefault
		}
		return
	}
	s.w.PermissionMode = ""
	delete(s.src, "permission_mode")
}

// setHarness switches harness; options set for another harness are dropped.
func (s *state) setHarness(h, source string) {
	if h != s.w.Harness {
		s.w = Worker{Harness: h}
		for _, k := range []string{"model", "effort", "extra_args"} {
			s.src[k] = SourceDefault
		}
		delete(s.src, "permission_mode")
	}
	s.src["harness"] = source
	s.fixPermission()
}

func (s *state) apply(l *Layer, source string) error {
	if l.Harness != nil {
		s.setHarness(*l.Harness, source)
	}
	if l.Model != nil {
		s.w.Model, s.src["model"] = *l.Model, source
	}
	if l.Effort != nil {
		s.w.Effort, s.src["effort"] = *l.Effort, source
	}
	if l.ExtraArgs != nil {
		s.w.ExtraArgs, s.src["extra_args"] = slices.Clone(*l.ExtraArgs), source
	}
	if l.PermissionMode != nil {
		if s.role != Applier || s.w.Harness != Claude {
			return fmt.Errorf("%s: %s.permission_mode: only a claude applier has a permission mode", source, s.role)
		}
		s.w.PermissionMode, s.src["permission_mode"] = *l.PermissionMode, source
	}
	return nil
}

// Resolve applies files (weakest first; nil entries are skipped) and then
// the flags over the built-in defaults.
func Resolve(files []*File, flags Flags) (Resolved, error) {
	out := Resolved{Sources: map[string]map[string]string{}}
	for _, role := range []string{Applier, Reviewer} {
		s := newState(role)
		for _, f := range files {
			if f == nil || f.Roles[role] == nil {
				continue
			}
			if err := s.apply(f.Roles[role], f.Path); err != nil {
				return Resolved{}, err
			}
		}
		flag := flags.Applier
		if role == Reviewer {
			flag = flags.Reviewer
		}
		if flag != "" {
			h, m, err := ParseFlag("--"+role, flag)
			if err != nil {
				return Resolved{}, err
			}
			s.setHarness(h, SourceFlag)
			if m != "" {
				s.w.Model, s.src["model"] = m, SourceFlag
			}
		}
		out.Sources[role] = s.src
		if role == Applier {
			out.Applier = s.w
		} else {
			out.Reviewer = s.w
		}
	}
	return out, nil
}

// ParseFlag splits a role flag value HARNESS[:MODEL].
func ParseFlag(name, v string) (harness, model string, err error) {
	h, m, hasModel := strings.Cut(v, ":")
	bad := func(why string) (string, string, error) {
		return "", "", fmt.Errorf("%s %q: %s; want HARNESS[:MODEL] with HARNESS %s", name, v, why, strings.Join(Harnesses, " or "))
	}
	if !slices.Contains(Harnesses, h) {
		return bad("unknown harness")
	}
	if hasModel && m == "" {
		return bad("empty model")
	}
	if err := plain(name, m); err != nil {
		return bad("model contains a backslash or line break")
	}
	return h, m, nil
}

// Same reports whether two workers are the same harness and model (an unset
// model counts as the same).
func Same(a, b Worker) bool {
	return a.Harness == b.Harness && a.Model == b.Model
}

// UserFile is $XDG_CONFIG_HOME/agk/config.toml (absolute XDG only), else
// ~/.config/agk/config.toml.
func UserFile(getenv func(string) string, home string) string {
	if x := getenv("XDG_CONFIG_HOME"); filepath.IsAbs(x) {
		return filepath.Join(x, "agk", "config.toml")
	}
	return filepath.Join(home, ".config", "agk", "config.toml")
}

// RepoFile is the optional per-repository config file.
func RepoFile(root string) string { return filepath.Join(root, ".agk.toml") }
