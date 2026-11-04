package spike

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnv(t *testing.T) {
	for _, k := range []string{"HOME", "TMPDIR", "PATH", "TEST_TMPDIR", "TMUX_TMPDIR", "STAPLER_SQUAD_INSTANCE", "STAPLER_SQUAD_TMUX_SOCKET"} {
		t.Logf("%s=%s", k, os.Getenv(k))
	}
	if tt := os.Getenv("TEST_TMPDIR"); tt == "" {
		t.Skip("not under bazel test")
	}
	if rel, err := filepath.Rel(os.Getenv("TEST_TMPDIR"), os.Getenv("HOME")); err != nil || strings.HasPrefix(rel, "..") {
		t.Errorf("HOME %q not under TEST_TMPDIR", os.Getenv("HOME"))
	}
	if _, err := os.Stat(filepath.Join(os.Getenv("HOME"), ".gitconfig")); err != nil {
		t.Errorf(".gitconfig: %v", err)
	}
	if !strings.HasPrefix(os.Getenv("TMPDIR"), "/tmp") || len(os.TempDir()) > 30 {
		t.Errorf("TMPDIR %q not short", os.Getenv("TMPDIR"))
	}
	if os.Getenv("STAPLER_SQUAD_INSTANCE") != "bazel-test" {
		t.Errorf("STAPLER_SQUAD_INSTANCE = %q", os.Getenv("STAPLER_SQUAD_INSTANCE"))
	}
	if v := os.Getenv("STAPLER_SQUAD_TMUX_SOCKET"); v != "" {
		t.Errorf("STAPLER_SQUAD_TMUX_SOCKET must not be set globally, got %q", v)
	}
	if td := os.Getenv("TMUX_TMPDIR"); !strings.HasPrefix(td, "/tmp/ssq-tmux.") {
		t.Errorf("TMUX_TMPDIR = %q, want private /tmp/ssq-tmux.*", td)
	}
	for _, tool := range []string{"tmux", "gh"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Errorf("%s not on test PATH: %v", tool, err)
		}
	}
	out, err := exec.Command("git", "config", "--global", "user.name").Output() //nolint:norawexec test probe, one short-lived git read
	if err != nil || strings.TrimSpace(string(out)) == "" {
		t.Errorf("git identity: %q %v", out, err)
	}
}
