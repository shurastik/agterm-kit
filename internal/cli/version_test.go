package cli

import (
	"context"
	"testing"

	"github.com/shurastik/agterm-kit/internal/buildinfo"
)

func TestVersionDefaultsToDev(t *testing.T) {
	h := newHarness(t, false)
	if code := h.env.Main(context.Background(), []string{"version"}); code != ExitOK {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	if got := h.out.String(); got != "dev\n" {
		t.Fatalf("stdout = %q, want %q", got, "dev\n")
	}
}

func TestVersionPrintsStamp(t *testing.T) {
	old := buildinfo.Version
	buildinfo.Version = "0.3.0"
	t.Cleanup(func() { buildinfo.Version = old })
	h := newHarness(t, false)
	if code := h.env.Main(context.Background(), []string{"version"}); code != ExitOK {
		t.Fatalf("exit %d", code)
	}
	if got := h.out.String(); got != "0.3.0\n" {
		t.Fatalf("stdout = %q", got)
	}
}

func TestVersionRejectsArgs(t *testing.T) {
	h := newHarness(t, false)
	if code := h.env.Main(context.Background(), []string{"version", "x"}); code != ExitError {
		t.Fatalf("exit %d", code)
	}
}
