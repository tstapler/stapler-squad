//go:build gitoracle

package gitconfig

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// cleanEnv is a hermetic environment for the real git: no user, system or XDG config, fixed locale.
func cleanEnv(home string, extra ...string) []string {
	env := []string{
		"PATH=" + os.Getenv("PATH"), "HOME=" + home, "LC_ALL=C", "GIT_CONFIG_NOSYSTEM=1",
		"GIT_TERMINAL_PROMPT=0", "XDG_CONFIG_HOME=" + filepath.Join(home, ".config"),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com",
	}
	return append(env, extra...)
}

// gitRun runs real git in dir with env and returns stdout, stderr and the exit error.
func gitRun(t testing.TB, dir string, env []string, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir, cmd.Env = dir, env
	var so, se bytes.Buffer
	cmd.Stdout, cmd.Stderr = &so, &se
	err = cmd.Run()
	return so.String(), se.String(), err
}

// parseZList decodes `git config --list -z` output: "key\nvalue" or bare "key", NUL-terminated.
func parseZList(out string) []string {
	var res []string
	for _, rec := range strings.Split(strings.TrimSuffix(out, "\x00"), "\x00") {
		if rec == "" {
			continue
		}
		key, val, hasVal := strings.Cut(rec, "\n")
		if hasVal {
			res = append(res, key+"="+val)
		} else {
			res = append(res, key)
		}
	}
	return res
}

func writeTestFile(t testing.TB, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
}
