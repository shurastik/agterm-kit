package config

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func load(t *testing.T, body string) *File {
	t.Helper()
	f, err := LoadFile(writeFile(t, body))
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func TestResolveDefaults(t *testing.T) {
	r, err := Resolve(nil, Flags{})
	if err != nil {
		t.Fatal(err)
	}
	want := Worker{Harness: Claude, PermissionMode: "auto"}
	if !reflect.DeepEqual(r.Applier, want) {
		t.Fatalf("applier = %+v", r.Applier)
	}
	if !reflect.DeepEqual(r.Reviewer, Worker{Harness: Codex}) {
		t.Fatalf("reviewer = %+v", r.Reviewer)
	}
	for _, role := range []string{Applier, Reviewer} {
		for _, k := range []string{"harness", "model", "effort", "extra_args"} {
			if s := r.Sources[role][k]; s != "default" {
				t.Errorf("%s.%s source = %q", role, k, s)
			}
		}
	}
	if r.Sources[Applier]["permission_mode"] != "default" {
		t.Error("applier permission_mode source")
	}
	if _, ok := r.Sources[Reviewer]["permission_mode"]; ok {
		t.Error("reviewer has no permission_mode")
	}
}

func TestResolveRepoOverridesUser(t *testing.T) {
	user := load(t, "[reviewer]\nmodel = \"gpt-5.5\"\neffort = \"high\"\n")
	repo := load(t, "[reviewer]\neffort = \"medium\"\n")
	r, err := Resolve([]*File{user, repo}, Flags{})
	if err != nil {
		t.Fatal(err)
	}
	if r.Reviewer.Model != "gpt-5.5" || r.Reviewer.Effort != "medium" || r.Reviewer.Harness != Codex {
		t.Fatalf("reviewer = %+v", r.Reviewer)
	}
	if r.Sources[Reviewer]["model"] != user.Path || r.Sources[Reviewer]["effort"] != repo.Path {
		t.Fatalf("sources = %v", r.Sources[Reviewer])
	}
}

func TestResolveNilFilesSkipped(t *testing.T) {
	if _, err := Resolve([]*File{nil, nil}, Flags{}); err != nil {
		t.Fatal(err)
	}
}

func TestResolveFlagModel(t *testing.T) {
	user := load(t, "[reviewer]\nharness = \"codex\"\neffort = \"high\"\n")
	r, err := Resolve([]*File{user}, Flags{Reviewer: "codex:gpt-5.5-mini"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(r.Reviewer, Worker{Harness: Codex, Model: "gpt-5.5-mini", Effort: "high"}) {
		t.Fatalf("reviewer = %+v", r.Reviewer)
	}
	if r.Sources[Reviewer]["model"] != "flag" || r.Sources[Reviewer]["harness"] != "flag" || r.Sources[Reviewer]["effort"] != user.Path {
		t.Fatalf("sources = %v", r.Sources[Reviewer])
	}
}

func TestResolveFlagHarnessSwitchDropsForeignOptions(t *testing.T) {
	user := load(t, "[reviewer]\nharness = \"codex\"\nmodel = \"gpt-5.5\"\nextra_args = [\"--profile\", \"review\"]\n")
	r, err := Resolve([]*File{user}, Flags{Reviewer: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(r.Reviewer, Worker{Harness: Claude}) {
		t.Fatalf("reviewer = %+v", r.Reviewer)
	}
	if r.Sources[Reviewer]["model"] != "default" || r.Sources[Reviewer]["extra_args"] != "default" {
		t.Fatalf("sources = %v", r.Sources[Reviewer])
	}
}

func TestResolveFileHarnessSwitchDropsWeakerOptions(t *testing.T) {
	user := load(t, "[reviewer]\nmodel = \"gpt-5.5\"\neffort = \"high\"\n")
	repo := load(t, "[reviewer]\nharness = \"claude\"\neffort = \"low\"\n")
	r, err := Resolve([]*File{user, repo}, Flags{})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(r.Reviewer, Worker{Harness: Claude, Effort: "low"}) {
		t.Fatalf("reviewer = %+v", r.Reviewer)
	}
}

func TestResolveApplierFlagToCodexDropsPermissionMode(t *testing.T) {
	user := load(t, "[applier]\npermission_mode = \"acceptEdits\"\n")
	r, err := Resolve([]*File{user}, Flags{Applier: "codex:gpt-5.5"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(r.Applier, Worker{Harness: Codex, Model: "gpt-5.5"}) {
		t.Fatalf("applier = %+v", r.Applier)
	}
	if _, ok := r.Sources[Applier]["permission_mode"]; ok {
		t.Fatal("codex applier must not list permission_mode")
	}
}

func TestResolvePermissionModeOnCodexFromAnotherLayer(t *testing.T) {
	user := load(t, "[applier]\nharness = \"codex\"\n")
	repo := load(t, "[applier]\npermission_mode = \"auto\"\n")
	_, err := Resolve([]*File{user, repo}, Flags{})
	if err == nil || !strings.Contains(err.Error(), repo.Path) || !strings.Contains(err.Error(), "applier.permission_mode") {
		t.Fatalf("err = %v", err)
	}
}

func TestParseFlag(t *testing.T) {
	ok := map[string][2]string{"claude": {Claude, ""}, "codex:gpt-5.5": {Codex, "gpt-5.5"}, "claude:claude-opus-5-5": {Claude, "claude-opus-5-5"}}
	for in, want := range ok {
		h, m, err := ParseFlag("--applier", in)
		if err != nil || h != want[0] || m != want[1] {
			t.Errorf("%q: %q %q %v", in, h, m, err)
		}
	}
	for _, in := range []string{"gemini", "claude:", "", ":x", "codex:a\nb"} {
		_, _, err := ParseFlag("--applier", in)
		if err == nil || !strings.Contains(err.Error(), "--applier") || !strings.Contains(err.Error(), "claude") {
			t.Errorf("%q: err = %v", in, err)
		}
	}
	if _, err := Resolve(nil, Flags{Reviewer: "gemini"}); err == nil || !strings.Contains(err.Error(), "--reviewer") {
		t.Fatalf("Resolve bad flag: %v", err)
	}
}

func TestSame(t *testing.T) {
	if !Same(Worker{Harness: Claude}, Worker{Harness: Claude}) {
		t.Error("unset models count as same")
	}
	if Same(Worker{Harness: Claude, Model: "a"}, Worker{Harness: Claude, Model: "b"}) {
		t.Error("different models")
	}
	if Same(Worker{Harness: Claude}, Worker{Harness: Codex}) {
		t.Error("different harnesses")
	}
}

func TestPaths(t *testing.T) {
	env := map[string]string{}
	get := func(k string) string { return env[k] }
	if got := UserFile(get, "/home/u"); got != "/home/u/.config/agk/config.toml" {
		t.Errorf("default user file = %q", got)
	}
	env["XDG_CONFIG_HOME"] = "/x"
	if got := UserFile(get, "/home/u"); got != "/x/agk/config.toml" {
		t.Errorf("XDG user file = %q", got)
	}
	env["XDG_CONFIG_HOME"] = "rel" // relative XDG is ignored per the XDG spec
	if got := UserFile(get, "/home/u"); got != "/home/u/.config/agk/config.toml" {
		t.Errorf("relative XDG user file = %q", got)
	}
	if got := RepoFile("/repo"); got != filepath.Join("/repo", ".agk.toml") {
		t.Errorf("repo file = %q", got)
	}
}
