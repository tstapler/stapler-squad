package testenv

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestIsolatedHome_should_GiveGitAnIdentity(t *testing.T) {
	home := IsolatedHome(t)
	if os.Getenv("HOME") != home {
		t.Fatal("HOME not set")
	}
	out, err := exec.Command("git", "config", "--global", "user.email").Output() //nolint:norawexec test probe, one short-lived git read
	if err != nil || !strings.Contains(string(out), "@") {
		t.Fatalf("git identity missing: %q %v", out, err)
	}
}

func TestShortTempDir_should_StayUnderTmp(t *testing.T) {
	dir := ShortTempDir(t)
	if !strings.HasPrefix(dir, "/tmp/") || len(dir) > 40 {
		t.Fatalf("dir %q not short", dir)
	}
}

func TestUniqueTmuxSocket_should_SetEnv(t *testing.T) {
	name := UniqueTmuxSocket(t)
	if os.Getenv("STAPLER_SQUAD_TMUX_SOCKET") != name || !strings.HasPrefix(name, "ssq-test-") {
		t.Fatalf("socket env = %q", os.Getenv("STAPLER_SQUAD_TMUX_SOCKET"))
	}
}

func TestLiveStateError(t *testing.T) {
	tmp := t.TempDir()
	cases := []struct {
		name    string
		env     map[string]string
		wantErr bool
	}{
		{"not under bazel", map[string]string{"TEST_TMPDIR": "", "HOME": "/home/someone"}, false},
		{"home outside", map[string]string{"TEST_TMPDIR": tmp, "HOME": "/home/someone"}, true},
		{"home inside", map[string]string{"TEST_TMPDIR": tmp, "HOME": tmp + "/home"}, false},
		{"shared instance", map[string]string{"TEST_TMPDIR": tmp, "HOME": tmp + "/home", "STAPLER_SQUAD_INSTANCE": "shared"}, true},
		{"test dir inside", map[string]string{"TEST_TMPDIR": tmp, "HOME": "/home/someone", "STAPLER_SQUAD_TEST_DIR": tmp + "/s"}, false},
		{"test dir outside", map[string]string{"TEST_TMPDIR": tmp, "HOME": tmp, "STAPLER_SQUAD_TEST_DIR": "/home/x/.stapler-squad"}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("STAPLER_SQUAD_INSTANCE", "")
			t.Setenv("STAPLER_SQUAD_TEST_DIR", "")
			for k, v := range c.env {
				t.Setenv(k, v)
			}
			if err := LiveStateError(); (err != nil) != c.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, c.wantErr)
			}
		})
	}
}
