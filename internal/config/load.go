// Package config resolves which harness, model and options each agk worker
// role runs with, from flags, .agk.toml, the user config file and defaults.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"
)

// Roles.
const (
	Applier  = "applier"
	Reviewer = "reviewer"
)

// Harnesses.
const (
	Claude = "claude"
	Codex  = "codex"
)

// Harnesses lists the supported harnesses.
var Harnesses = []string{Claude, Codex}

// Layer is one file's settings for a role; nil fields are unset.
type Layer struct {
	Harness        *string   `toml:"harness"`
	Model          *string   `toml:"model"`
	Effort         *string   `toml:"effort"`
	ExtraArgs      *[]string `toml:"extra_args"`
	PermissionMode *string   `toml:"permission_mode"`
}

// File is a parsed config file.
type File struct {
	Path  string
	Roles map[string]*Layer
}

// LoadFile parses and validates path. A missing file yields nil, nil.
func LoadFile(path string) (*File, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	fail := func(format string, args ...any) (*File, error) {
		return nil, fmt.Errorf("%s: "+format, append([]any{path}, args...)...)
	}
	var tables map[string]any
	if _, err := toml.Decode(string(raw), &tables); err != nil {
		return fail("%v", err)
	}
	keys := make([]string, 0, len(tables))
	for k := range tables {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		if k != Applier && k != Reviewer {
			return fail("unknown table or key %q (only [applier] and [reviewer])", k)
		}
		if _, ok := tables[k].(map[string]any); !ok {
			return fail("%s must be a table", k)
		}
	}
	var doc struct {
		Applier  *Layer `toml:"applier"`
		Reviewer *Layer `toml:"reviewer"`
	}
	md, err := toml.Decode(string(raw), &doc)
	if err != nil {
		return fail("%v", err)
	}
	if und := md.Undecoded(); len(und) > 0 {
		return fail("unknown key %q", und[0].String())
	}
	f := &File{Path: path, Roles: map[string]*Layer{}}
	for role, l := range map[string]*Layer{Applier: doc.Applier, Reviewer: doc.Reviewer} {
		if l == nil {
			continue
		}
		if err := l.validate(role); err != nil {
			return fail("%v", err)
		}
		f.Roles[role] = l
	}
	return f, nil
}

func (l *Layer) validate(role string) error {
	if l.Harness != nil && !slices.Contains(Harnesses, *l.Harness) {
		return fmt.Errorf("%s.harness: unknown harness %q (want %s)", role, *l.Harness, strings.Join(Harnesses, " or "))
	}
	if l.PermissionMode != nil {
		if role != Applier {
			return fmt.Errorf("%s.permission_mode: only the applier has a permission mode", role)
		}
		if l.Harness != nil && *l.Harness == Codex {
			return fmt.Errorf("%s.permission_mode: a codex applier has no permission mode", role)
		}
	}
	for key, v := range map[string]*string{"harness": l.Harness, "model": l.Model, "effort": l.Effort, "permission_mode": l.PermissionMode} {
		if v != nil {
			if err := plain(role+"."+key, *v); err != nil {
				return err
			}
		}
	}
	if l.ExtraArgs != nil {
		for _, a := range *l.ExtraArgs {
			if err := plain(role+".extra_args", a); err != nil {
				return err
			}
		}
	}
	return nil
}

// plain refuses values the companion command line cannot carry: agterm runs
// it through the user's login shell (fish), where a backslash or a line break
// cannot be quoted safely.
func plain(key, v string) error {
	if strings.ContainsAny(v, "\\\n\r") {
		return fmt.Errorf("%s: %q contains a backslash or line break", key, v)
	}
	return nil
}
