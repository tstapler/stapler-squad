//go:build gitoracle

package gitconfig

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/testutil/gitoracle"
)

// launchModes are the ways a CLI can be started for the same repository; git's answer to a
// gitdir: pattern written through a symlink depends on which one is used (plan c1).
func launchModes(root, linkRoot string, env []string) map[string]func() *exec.Cmd {
	sh := func(script string) func() *exec.Cmd {
		return func() *exec.Cmd {
			cmd := exec.Command("sh", "-c", script, "sh")
			cmd.Env = env
			return cmd
		}
	}
	return map[string]func() *exec.Cmd{
		"logical-PWD": func() *exec.Cmd {
			cmd := exec.Command("git", "config", "--get", "zz.hit")
			cmd.Dir, cmd.Env = linkRoot, append(append([]string(nil), env...), "PWD="+linkRoot)
			return cmd
		},
		"cd-P":         sh("cd -P '" + linkRoot + "' && git config --get zz.hit"),
		"env-u-PWD":    sh("env -u PWD git -C '" + linkRoot + "' config --get zz.hit"),
		"cd-root--git": sh("cd / && git -C '" + linkRoot + "' config --get zz.hit"),
	}
}

func TestResolverGitdirThroughSymlinkedParent(t *testing.T) {
	gitoracle.RequireGit(t)
	base, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	home := filepath.Join(base, "home")
	realRoot := filepath.Join(base, "real", "repo")
	link := filepath.Join(base, "lnk")
	require.NoError(t, os.MkdirAll(home, 0o700))
	require.NoError(t, os.MkdirAll(realRoot, 0o700))
	require.NoError(t, os.Symlink(filepath.Join(base, "real"), link))
	g := gitoracle.Git{T: t, Home: home}
	g.Run(realRoot, "init", "-q", "-b", "main", "--template=")
	linkRoot := filepath.Join(link, "repo")
	writeTestFile(t, filepath.Join(home, "cond.cfg"), "[zz]\n\thit = yes\n")

	for name, pattern := range map[string]string{
		"pattern-through-symlink": link + "/",
		"pattern-real-path":       filepath.Join(base, "real") + "/",
		"pattern-no-match":        filepath.Join(base, "other") + "/",
	} {
		t.Run(name, func(t *testing.T) {
			writeTestFile(t, filepath.Join(home, ".gitconfig"), "[includeIf \"gitdir:"+pattern+"\"]\n\tpath = "+filepath.Join(home, "cond.cfg")+"\n")
			env := cleanEnv(home)
			answers := map[string]bool{}
			for mode, mk := range launchModes(realRoot, linkRoot, env) {
				out, err := mk().Output()
				if err != nil {
					var ee *exec.ExitError
					require.ErrorAs(t, err, &ee)
					require.Equal(t, 1, ee.ExitCode(), "git config --get exits 1 for an unset key")
				}
				answers[mode] = strings.TrimSpace(string(out)) == "yes"
			}
			agree := true
			for _, v := range answers {
				agree = agree && v == answers["logical-PWD"]
			}

			common := filepath.Join(linkRoot, ".git")
			res, rerr := Resolve(context.Background(), Env{Environ: env}, Repo{GitDir: common, CommonDir: common})
			if !agree {
				var ce *Error
				require.ErrorAs(t, rerr, &ce, "git answers depend on the launch mode %v, the resolver must fail closed", answers)
				assert.Equal(t, SubGitdirSymlink, ce.Sub)
				return
			}
			require.NoError(t, rerr, "every launch mode agrees (%v)", answers)
			_, hit := res.Lookup("zz.hit")
			assert.Equal(t, answers["logical-PWD"], hit, "modes: %v", answers)
		})
	}
}
