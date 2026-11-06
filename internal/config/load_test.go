package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadMissingFileIsSkipped(t *testing.T) {
	f, err := LoadFile(filepath.Join(t.TempDir(), "none.toml"))
	if err != nil || f != nil {
		t.Fatalf("LoadFile = %v, %v; want nil, nil", f, err)
	}
}

func TestLoadValid(t *testing.T) {
	p := writeFile(t, `
[applier]
harness = "claude"
model = "claude-sonnet-5-5"
effort = "high"
permission_mode = "acceptEdits"
extra_args = ["--add-dir", "/tmp/x"]

[reviewer]
harness = "codex"
`)
	f, err := LoadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	a := f.Roles[Applier]
	if a == nil || *a.Harness != "claude" || *a.Model != "claude-sonnet-5-5" || *a.Effort != "high" ||
		*a.PermissionMode != "acceptEdits" || strings.Join(*a.ExtraArgs, " ") != "--add-dir /tmp/x" {
		t.Fatalf("applier = %+v", a)
	}
	if r := f.Roles[Reviewer]; r == nil || *r.Harness != "codex" || r.Model != nil {
		t.Fatalf("reviewer = %+v", r)
	}
}

func TestLoadInvalid(t *testing.T) {
	cases := map[string]struct{ body, want string }{
		"not toml":             {`[applier`, ""},
		"unknown table":        {"[flow]\nx = 1\n", "flow"},
		"empty unknown table":  {"[flow]\n", "flow"},
		"top-level key":        {"harness = \"claude\"\n", "harness"},
		"unknown key":          {"[reviewer]\nmodle = \"x\"\n", "reviewer.modle"},
		"unknown harness":      {"[applier]\nharness = \"gemini\"\n", "applier.harness"},
		"wrong type":           {"[reviewer]\nmodel = 5\n", "reviewer.model"},
		"role not a table":     {"applier = \"claude\"\n", "applier"},
		"permission on review": {"[reviewer]\npermission_mode = \"auto\"\n", "reviewer.permission_mode"},
		"codex permission":     {"[applier]\nharness = \"codex\"\npermission_mode = \"auto\"\n", "applier.permission_mode"},
		"extra arg newline":    {"[applier]\nextra_args = [\"--x\\nrm\"]\n", "applier.extra_args"},
		"extra arg backslash":  {"[applier]\nextra_args = [\"a\\\\b\"]\n", "applier.extra_args"},
		"model line break":     {"[reviewer]\nmodel = \"a\\rb\"\n", "reviewer.model"},
		"empty harness":        {"[reviewer]\nharness = \"\"\n", "reviewer.harness"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			p := writeFile(t, tc.body)
			_, err := LoadFile(p)
			if err == nil {
				t.Fatal("want error")
			}
			if !strings.Contains(err.Error(), p) {
				t.Errorf("error %q does not name the file", err)
			}
			if tc.want != "" && !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not name %q", err, tc.want)
			}
		})
	}
}
