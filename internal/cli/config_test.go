package cli

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// repoWith makes a repo root, points RepoRoot at it and writes .agk.toml.
func repoWith(t *testing.T, h *harness, agkToml string) string {
	t.Helper()
	root, _ := filepath.EvalSymlinks(t.TempDir())
	h.env.RepoRoot = func(context.Context, string) (string, error) { return root, nil }
	if agkToml != "" {
		if err := os.WriteFile(filepath.Join(root, ".agk.toml"), []byte(agkToml), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

type shownRole struct {
	Harness        string            `json:"harness"`
	Model          string            `json:"model"`
	Effort         string            `json:"effort"`
	ExtraArgs      []string          `json:"extra_args"`
	PermissionMode *string           `json:"permission_mode"`
	Sources        map[string]string `json:"sources"`
}

func showConfig(t *testing.T, h *harness, args ...string) map[string]shownRole {
	t.Helper()
	if code := h.env.Config(context.Background(), args); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	var got map[string]shownRole
	if err := json.Unmarshal(h.out.Bytes(), &got); err != nil {
		t.Fatalf("stdout %q: %v", h.out, err)
	}
	return got
}

func TestConfigDefaults(t *testing.T) {
	h := newHarness(t, false)
	got := showConfig(t, h)
	a, r := got["applier"], got["reviewer"]
	if a.Harness != "claude" || a.PermissionMode == nil || *a.PermissionMode != "auto" || a.Sources["harness"] != "default" {
		t.Fatalf("applier = %+v", a)
	}
	if r.Harness != "codex" || r.PermissionMode != nil || r.ExtraArgs == nil || len(r.ExtraArgs) != 0 {
		t.Fatalf("reviewer = %+v (extra_args must be [])", r)
	}
	if h.err.Len() != 0 {
		t.Fatalf("stderr = %q", h.err)
	}
}

func TestConfigSourcesPerKey(t *testing.T) {
	h := newHarness(t, false)
	root := repoWith(t, h, "[applier]\neffort = \"high\"\n")
	got := showConfig(t, h)
	a := got["applier"]
	if a.Effort != "high" || a.Sources["effort"] != filepath.Join(root, ".agk.toml") || a.Sources["harness"] != "default" {
		t.Fatalf("applier = %+v", a)
	}
}

func TestConfigUserFileAndFlags(t *testing.T) {
	h := newHarness(t, false)
	xdg := t.TempDir()
	h.vars["XDG_CONFIG_HOME"] = xdg
	os.MkdirAll(filepath.Join(xdg, "agk"), 0o755)
	os.WriteFile(filepath.Join(xdg, "agk", "config.toml"), []byte("[applier]\nmodel = \"claude-sonnet-5-5\"\n"), 0o644)
	got := showConfig(t, h, "--reviewer", "claude:claude-opus-5-5")
	if a := got["applier"]; a.Model != "claude-sonnet-5-5" || a.Sources["model"] != filepath.Join(xdg, "agk", "config.toml") {
		t.Fatalf("applier = %+v", a)
	}
	if r := got["reviewer"]; r.Harness != "claude" || r.Model != "claude-opus-5-5" || r.Sources["model"] != "flag" {
		t.Fatalf("reviewer = %+v", r)
	}
}

func TestConfigSameWorkerWarns(t *testing.T) {
	h := newHarness(t, false)
	showConfig(t, h, "--reviewer", "claude")
	if !strings.Contains(h.err.String(), "not independent") {
		t.Fatalf("stderr = %q", h.err)
	}
}

func TestConfigInvalid(t *testing.T) {
	h := newHarness(t, false)
	root := repoWith(t, h, "[reviewer]\nmodle = \"x\"\n")
	if code := h.env.Config(context.Background(), nil); code != 1 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(h.err.String(), filepath.Join(root, ".agk.toml")) || !strings.Contains(h.err.String(), "reviewer.modle") || h.out.Len() != 0 {
		t.Fatalf("stderr %q stdout %q", h.err, h.out)
	}
	h = newHarness(t, false)
	if code := h.env.Config(context.Background(), []string{"--applier", "gemini"}); code != 1 || !strings.Contains(h.err.String(), "--applier") {
		t.Fatalf("bad flag: exit %d, %q", code, h.err)
	}
	h = newHarness(t, false)
	if code := h.env.Config(context.Background(), []string{"extra"}); code != 1 {
		t.Fatalf("positional arg: exit %d", code)
	}
}

func TestConfigOutsideRepo(t *testing.T) {
	h := newHarness(t, false)
	h.env.RepoRoot = func(context.Context, string) (string, error) { return "", errors.New("not a git repository") }
	showConfig(t, h)
}

func TestConfigViaMain(t *testing.T) {
	h := newHarness(t, false)
	if code := h.env.Main(context.Background(), []string{"config"}); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	if !strings.Contains(usage, "agk config") {
		t.Fatal("usage lacks agk config")
	}
}
