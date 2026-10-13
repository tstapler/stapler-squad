//go:build gitoracle

package gitconfig

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func none(*testing.T, fixture) []string { return nil }

// hasconfigInclude writes ~/.gitconfig with one hasconfig include of a target that sets zz.hit,
// then adds the given remotes to the repository config.
func hasconfigInclude(glob string, remotes ...string) func(*testing.T, fixture) []string {
	return func(t *testing.T, f fixture) []string {
		f.write("target.cfg", "[zz]\n\thit = yes\n")
		f.write(".gitconfig", "[user]\n\tname = A\n[includeIf \"hasconfig:remote.*.url:"+glob+"\"]\n\tpath = "+filepath.Join(f.home, "target.cfg")+"\n[user]\n\temail = after\n")
		for i, r := range remotes {
			f.appendLocal(fmt.Sprintf("[remote \"r%d\"]\n\turl = %s\n", i, r))
		}
		return nil
	}
}

func scenarios() []scenario {
	list := []scenario{
		{name: "global-gitconfig-only", setup: func(t *testing.T, f fixture) []string {
			f.write(".gitconfig", "[user]\n\tname = G\n[core]\n\teditor = vim\n")
			return nil
		}},
		{name: "xdg-only", setup: func(t *testing.T, f fixture) []string {
			f.write(".config/git/config", "[user]\n\tname = X\n")
			return nil
		}},
		{name: "xdg-and-gitconfig-order", setup: func(t *testing.T, f fixture) []string {
			f.write(".config/git/config", "[user]\n\tname = X\n[x]\n\ty = xdg\n")
			f.write(".gitconfig", "[user]\n\tname = G\n[x]\n\ty = home\n")
			f.appendLocal("[user]\n\tname = L\n")
			return nil
		}},
		{name: "GIT_CONFIG_GLOBAL-replaces-both-files", setup: func(t *testing.T, f fixture) []string {
			f.write(".gitconfig", "[user]\n\tname = ignored\n")
			f.write(".config/git/config", "[user]\n\tname = ignored\n")
			p := f.write("elsewhere.cfg", "[user]\n\tname = env\n")
			return []string{"GIT_CONFIG_GLOBAL=" + p}
		}},
		{name: "GIT_CONFIG_SYSTEM-file", drop: []string{"GIT_CONFIG_NOSYSTEM"}, setup: func(t *testing.T, f fixture) []string {
			p := f.write("sys.cfg", "[sys]\n\tk = system\n[user]\n\tname = S\n")
			f.write(".gitconfig", "[user]\n\tname = G\n")
			return []string{"GIT_CONFIG_SYSTEM=" + p}
		}},
		{name: "GIT_CONFIG_SYSTEM-empty-value", drop: []string{"GIT_CONFIG_NOSYSTEM"}, setup: func(t *testing.T, f fixture) []string {
			f.write(".gitconfig", "[user]\n\tname = G\n")
			return []string{"GIT_CONFIG_SYSTEM="}
		}},
		{name: "NOSYSTEM-drops-system", setup: func(t *testing.T, f fixture) []string {
			p := f.write("sys.cfg", "[sys]\n\tk = system\n")
			f.write(".gitconfig", "[user]\n\tname = G\n")
			return []string{"GIT_CONFIG_SYSTEM=" + p}
		}},
		{name: "GIT_CONFIG_COUNT", setup: func(t *testing.T, f fixture) []string {
			f.write(".gitconfig", "[user]\n\tname = G\n")
			return []string{"GIT_CONFIG_COUNT=3", "GIT_CONFIG_KEY_0=user.name", "GIT_CONFIG_VALUE_0=cmd",
				"GIT_CONFIG_KEY_1=remote.o.b.url", "GIT_CONFIG_VALUE_1=https://x/y", "GIT_CONFIG_KEY_2=Core.Mixed", "GIT_CONFIG_VALUE_2="}
		}},
		{name: "GIT_CONFIG_COUNT-zero", setup: func(t *testing.T, f fixture) []string {
			return []string{"GIT_CONFIG_COUNT=0"}
		}},
		{name: "nested-includes-3-deep", setup: func(t *testing.T, f fixture) []string {
			f.write(".gitconfig", "[a]\n\tk = 0\n[include]\n\tpath = ~/i1.cfg\n[a]\n\tk = 4\n")
			f.write("i1.cfg", "[a]\n\tk = 1\n[include]\n\tpath = i2.cfg\n")
			f.write("i2.cfg", "[a]\n\tk = 2\n[include]\n\tpath = sub/i3.cfg\n")
			f.write("sub/i3.cfg", "[a]\n\tk = 3\n")
			return nil
		}},
		{name: "relative-and-dotdot-include", setup: func(t *testing.T, f fixture) []string {
			f.write("cfg/main.cfg", "[include]\n\tpath = ../up.cfg\n[include]\n\tpath = ./same.cfg\n")
			f.write("up.cfg", "[up]\n\tk = 1\n")
			f.write("cfg/same.cfg", "[same]\n\tk = 1\n")
			f.write(".gitconfig", "[include]\n\tpath = cfg/main.cfg\n")
			return nil
		}},
		{name: "missing-include-target", setup: func(t *testing.T, f fixture) []string {
			f.write(".gitconfig", "[a]\n\tk = 1\n[include]\n\tpath = nope.cfg\n[includeIf \"gitdir:/nowhere/\"]\n\tpath = nope2.cfg\n[b]\n\tk = 2\n")
			return nil
		}},
		{name: "include-in-repo-config", setup: func(t *testing.T, f fixture) []string {
			p := f.write("repoinc.cfg", "[ri]\n\tk = included\n")
			f.appendLocal("[include]\n\tpath = " + p + "\n[ri]\n\tk = after\n")
			return nil
		}},
		{name: "include-tilde", setup: func(t *testing.T, f fixture) []string {
			f.write("t.cfg", "[t]\n\tk = tilde\n")
			f.write(".gitconfig", "[include]\n\tpath = ~/t.cfg\n")
			return nil
		}},
		{name: "include-without-subsection-only", setup: func(t *testing.T, f fixture) []string {
			f.write("n.cfg", "[n]\n\tk = no\n")
			f.write(".gitconfig", "[include \"x\"]\n\tpath = "+filepath.Join(f.home, "n.cfg")+"\n[INCLUDE]\n\tPath = "+filepath.Join(f.home, "m.cfg")+"\n")
			f.write("m.cfg", "[m]\n\tk = upper\n")
			return nil
		}},
		{name: "worktreeConfig-enabled", setup: func(t *testing.T, f fixture) []string {
			f.appendLocal("[extensions]\n\tworktreeConfig = true\n")
			f.write("work/repo/.git/config.worktree", "[core]\n\thooksPath = /main-hooks\n")
			f.write("work/repo/.git/worktrees/linked/config.worktree", "[core]\n\thooksPath = /linked-hooks\n")
			return nil
		}},
		{name: "worktreeConfig-disabled-ignores-file", setup: func(t *testing.T, f fixture) []string {
			f.write("work/repo/.git/config.worktree", "[core]\n\thooksPath = /main-hooks\n")
			f.write("work/repo/.git/worktrees/linked/config.worktree", "[core]\n\thooksPath = /linked-hooks\n")
			return nil
		}},
		{name: "worktreeConfig-from-global-is-ignored", setup: func(t *testing.T, f fixture) []string {
			f.write(".gitconfig", "[extensions]\n\tworktreeConfig = true\n")
			f.write("work/repo/.git/config.worktree", "[core]\n\thooksPath = /main-hooks\n")
			return nil
		}},
		{name: "tokenizer-include-position", setup: func(t *testing.T, f fixture) []string {
			f.write("p.cfg", "[core]\n\thooksPath = /included\n")
			f.write(".gitconfig", "[core]\n\thooksPath = /a\n[include]\n\tpath = "+filepath.Join(f.home, "p.cfg")+"\n[core]\n\thooksPath = /b\n")
			return nil
		}},
		{name: "tokenizer-gpgsign-shapes", setup: func(t *testing.T, f fixture) []string {
			f.write(".gitconfig", "[commit]\n\tgpgsign\n[tag]\n\tgpgsign =\n[push]\n\tgpgSign = \"\"\n[gpg]\n\tformat = ssh\n")
			return nil
		}},
		{name: "tokenizer-syntax-grab-bag", setup: func(t *testing.T, f fixture) []string {
			f.write(".gitconfig", "\xef\xbb\xbf[Core]\r\n\tHooksPath = /a\r\n[a \"x\\\"y\\\\z\\q\"]\n\tk = v\n[a.Foo]\n\tk = v\n[a \"\"]\n\tk = e\n[a] k = 1 ; c\n[b]\n\tq = \"x ; y\" # c\n\tc = x\\\n  y\n")
			return nil
		}},
	}
	list = append(list, gitdirScenarios()...)
	list = append(list, onbranchScenarios()...)
	list = append(list, hasconfigScenarios()...)
	list = append(list, failClosedScenarios()...)
	return list
}

func gitdirScenarios() []scenario {
	incl := func(cond string) func(*testing.T, fixture) []string {
		return func(t *testing.T, f fixture) []string {
			f.write("cond.cfg", "[zz]\n\thit = yes\n")
			f.write(".gitconfig", "[user]\n\tname = A\n[includeIf \""+cond+"\"]\n\tpath = "+filepath.Join(f.home, "cond.cfg")+"\n[user]\n\temail = after\n")
			return nil
		}
	}
	inclF := func(condFor func(f fixture) string) func(*testing.T, fixture) []string {
		return func(t *testing.T, f fixture) []string {
			return incl(condFor(f))(t, f)
		}
	}
	return []scenario{
		{name: "gitdir-repo-root-slash", setup: inclF(func(f fixture) string { return "gitdir:" + f.root + "/" })},
		{name: "gitdir-dotgit-worktrees-glob", setup: inclF(func(f fixture) string { return "gitdir:" + f.root + "/.git/worktrees/**" })},
		{name: "gitdir-dotgit-exact", setup: inclF(func(f fixture) string { return "gitdir:" + f.root + "/.git" })},
		{name: "gitdir-linked-worktree-path", setup: inclF(func(f fixture) string { return "gitdir:" + f.linked + "/" })},
		{name: "gitdir-parent-dir", setup: inclF(func(f fixture) string { return "gitdir:" + filepath.Dir(f.root) + "/" })},
		{name: "gitdir-nonmatching", setup: inclF(func(f fixture) string { return "gitdir:/nowhere/" })},
		{name: "gitdir-tilde", setup: inclF(func(f fixture) string { return "gitdir:~/work/" })},
		{name: "gitdir-relative-without-prefix", setup: inclF(func(f fixture) string { return "gitdir:work/repo/.git" })},
		{name: "gitdir-star-component", setup: inclF(func(f fixture) string { return "gitdir:" + f.home + "/*/repo/.git" })},
		{name: "gitdir-case-sensitive", setup: inclF(func(f fixture) string { return "gitdir:" + strings.ToUpper(f.root) + "/" })},
		{name: "gitdir-i-case-insensitive", setup: inclF(func(f fixture) string { return "gitdir/i:" + strings.ToUpper(f.root) + "/" })},
		{name: "gitdir-dot-slash-relative-to-file", setup: func(t *testing.T, f fixture) []string {
			f.write("cond.cfg", "[zz]\n\thit = yes\n")
			f.write("work/inc.cfg", "[includeIf \"gitdir:./repo/\"]\n\tpath = "+filepath.Join(f.home, "cond.cfg")+"\n")
			f.write(".gitconfig", "[include]\n\tpath = ~/work/inc.cfg\n")
			return nil
		}},
		{name: "gitdir-in-repo-config", setup: func(t *testing.T, f fixture) []string {
			f.write("cond.cfg", "[zz]\n\thit = yes\n")
			f.appendLocal("[includeIf \"gitdir:" + f.root + "/\"]\n\tpath = " + filepath.Join(f.home, "cond.cfg") + "\n")
			return nil
		}},
		{name: "maintainer-shape-gitdir-and-overlay", setup: func(t *testing.T, f fixture) []string {
			f.write(".gitconfig-proxy", "[http]\n\tproxy = http://proxy.example:8080\n")
			f.write("work.cfg", "[user]\n\temail = work@example.com\n")
			f.write(".config/git/config", "[user]\n\tname = M\n[includeIf \"gitdir:"+filepath.Dir(f.root)+"/\"]\n\tpath = "+filepath.Join(f.home, "work.cfg")+"\n[include]\n\tpath = ~/.gitconfig.overlay\n")
			f.write(".gitconfig", "[include]\n\tpath = ~/.gitconfig-proxy\n")
			var sb strings.Builder
			for _, pat := range []string{"https://**.corp.example/**", "https://**.corp.example:*/**", "ssh://**.corp.example/**", "git@host.example:Org/**"} {
				for range 3 {
					sb.WriteString("[includeIf \"hasconfig:remote.*.url:" + pat + "\"]\n\tpath = " + filepath.Join(f.home, "corp.cfg") + "\n")
				}
			}
			f.write(".gitconfig.overlay", sb.String())
			f.write("corp.cfg", "[corp]\n\tproxy = yes\n")
			f.appendLocal("[remote \"origin\"]\n\turl = https://git.corp.example/org/repo\n")
			return nil
		}},
		{name: "maintainer-shape-no-matching-remote", setup: func(t *testing.T, f fixture) []string {
			f.write(".gitconfig", "[include]\n\tpath = ~/.gitconfig.overlay\n")
			f.write(".gitconfig.overlay", "[includeIf \"hasconfig:remote.*.url:https://**.corp.example/**\"]\n\tpath = "+filepath.Join(f.home, "corp.cfg")+"\n")
			f.write("corp.cfg", "[corp]\n\tproxy = yes\n")
			f.appendLocal("[remote \"origin\"]\n\turl = https://github.com/o/r\n")
			return nil
		}},
	}
}

func onbranchScenarios() []scenario {
	mk := func(cond string, pre func(t *testing.T, f fixture)) func(*testing.T, fixture) []string {
		return func(t *testing.T, f fixture) []string {
			if pre != nil {
				pre(t, f)
			}
			f.write("cond.cfg", "[zz]\n\thit = yes\n")
			f.write(".gitconfig", "[includeIf \""+cond+"\"]\n\tpath = "+filepath.Join(f.home, "cond.cfg")+"\n")
			return nil
		}
	}
	feat := func(t *testing.T, f fixture) { f.g.Run(f.root, "branch", "feat/x/y") }
	return []scenario{
		{name: "onbranch-main", setup: mk("onbranch:main", nil)},
		{name: "onbranch-linked-branch", setup: mk("onbranch:wt", nil)},
		{name: "onbranch-glob", setup: mk("onbranch:m*", nil)},
		{name: "onbranch-nonmatching", setup: mk("onbranch:other", nil)},
		{name: "onbranch-dir-prefix", setup: mk("onbranch:feat/", func(t *testing.T, f fixture) {
			feat(t, f)
			f.g.Run(f.root, "checkout", "-q", "feat/x/y")
		}), places: []string{"main"}},
		{name: "onbranch-star-stops-at-slash", setup: mk("onbranch:feat/*", func(t *testing.T, f fixture) {
			feat(t, f)
			f.g.Run(f.root, "checkout", "-q", "feat/x/y")
		}), places: []string{"main"}},
		{name: "onbranch-detached-head", setup: mk("onbranch:main", func(t *testing.T, f fixture) {
			f.g.Run(f.root, "checkout", "-q", "--detach")
		}), places: []string{"main"}},
	}
}

func hasconfigScenarios() []scenario {
	return []scenario{
		{name: "hasconfig-no-remote", setup: hasconfigInclude("https://**.corp.example/**")},
		{name: "hasconfig-match-https", setup: hasconfigInclude("https://**.corp.example/**", "https://git.corp.example/org/repo")},
		{name: "hasconfig-scp-style", setup: hasconfigInclude("git@host.example:team/**", "git@host.example:team/x.git")},
		{name: "hasconfig-star-stops-at-slash", setup: hasconfigInclude("https://*.example/x", "https://a/b.example/x")},
		{name: "hasconfig-pushurl-only", setup: func(t *testing.T, f fixture) []string {
			hasconfigInclude("https://p.example/**")(t, f)
			f.appendLocal("[remote \"o\"]\n\tpushurl = https://p.example/x\n")
			return nil
		}},
		{name: "hasconfig-insteadof-only", setup: func(t *testing.T, f fixture) []string {
			hasconfigInclude("https://p.example/**")(t, f)
			f.appendLocal("[url \"https://p.example/\"]\n\tinsteadOf = x:\n")
			return nil
		}},
		{name: "hasconfig-url-from-command-scope", setup: func(t *testing.T, f fixture) []string {
			hasconfigInclude("https://c.example/**")(t, f)
			return []string{"GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=remote.cmd.url", "GIT_CONFIG_VALUE_0=https://c.example/r"}
		}},
		{name: "hasconfig-dotted-remote-name", setup: func(t *testing.T, f fixture) []string {
			hasconfigInclude("https://d.example/**")(t, f)
			f.appendLocal("[remote \"a.b\"]\n\turl = https://d.example/r\n")
			return nil
		}},
		{name: "hasconfig-url-after-directive-same-file", setup: func(t *testing.T, f fixture) []string {
			f.write("target.cfg", "[zz]\n\thit = yes\n")
			f.write(".gitconfig", "[includeIf \"hasconfig:remote.*.url:https://late.example/**\"]\n\tpath = "+filepath.Join(f.home, "target.cfg")+"\n[remote \"late\"]\n\turl = https://late.example/r\n")
			return nil
		}},
		{name: "hasconfig-url-in-plain-include", setup: func(t *testing.T, f fixture) []string {
			f.write("urls.cfg", "[remote \"inc\"]\n\turl = https://inc.example/r\n")
			f.write("target.cfg", "[zz]\n\thit = yes\n")
			f.write(".gitconfig", "[include]\n\tpath = "+filepath.Join(f.home, "urls.cfg")+"\n[includeIf \"hasconfig:remote.*.url:https://inc.example/**\"]\n\tpath = "+filepath.Join(f.home, "target.cfg")+"\n")
			return nil
		}},
		{name: "hasconfig-nested-from-plain-include", setup: func(t *testing.T, f fixture) []string {
			f.write("target.cfg", "[zz]\n\thit = yes\n")
			f.write("outer.cfg", "[includeIf \"hasconfig:remote.*.url:https://n.example/**\"]\n\tpath = "+filepath.Join(f.home, "target.cfg")+"\n")
			f.write(".gitconfig", "[include]\n\tpath = "+filepath.Join(f.home, "outer.cfg")+"\n")
			f.appendLocal("[remote \"n\"]\n\turl = https://n.example/r\n")
			return nil
		}},
		{name: "hasconfig-other-key-is-false", setup: func(t *testing.T, f fixture) []string {
			f.write("target.cfg", "[zz]\n\thit = yes\n")
			f.write(".gitconfig", "[includeIf \"hasconfig:foo.*.bar:x\"]\n\tpath = "+filepath.Join(f.home, "target.cfg")+"\n")
			return nil
		}},
		{name: "hasconfig-keyword-is-case-sensitive-fails-closed", wantSub: SubUnknownCondition, gitOK: true, setup: func(t *testing.T, f fixture) []string {
			f.write("target.cfg", "[zz]\n\thit = yes\n")
			f.write(".gitconfig", "[includeIf \"HASCONFIG:remote.*.url:x\"]\n\tpath = "+filepath.Join(f.home, "target.cfg")+"\n")
			return nil
		}},
		{name: "hasconfig-glob-rows-double-star-slash", setup: hasconfigInclude("https://h/**/repo", "https://h/repo")},
		{name: "hasconfig-glob-rows-trailing-slash", setup: hasconfigInclude("https://h/Foo/**", "https://h/Foo/")},
		{name: "hasconfig-glob-rows-trailing-no-slash", setup: hasconfigInclude("https://h/Foo/**", "https://h/Foo")},
		{name: "hasconfig-glob-rows-case", setup: hasconfigInclude("https://H/**", "https://h/x")},
		{name: "hasconfig-glob-rows-question", setup: hasconfigInclude("https://h/?/repo", "https://h///repo")},
		{name: "hasconfig-glob-rows-embedded-double-star", setup: hasconfigInclude("https://h/a**b/repo", "https://h/aX/Yb/repo")},
		{name: "hasconfig-glob-rows-bare-star", setup: hasconfigInclude("*", "https://h/x")},
		{name: "hasconfig-glob-rows-bare-double-star", setup: hasconfigInclude("**", "https://h/x")},
		{name: "hasconfig-glob-rows-leading-double-star", setup: hasconfigInclude("**/repo", "repo")},
	}
}

func failClosedScenarios() []scenario {
	chain := func(n int) func(*testing.T, fixture) []string {
		return func(t *testing.T, f fixture) []string {
			for i := 1; i <= n; i++ {
				body := "[c]\n\tk" + fmt.Sprint(i) + " = 1\n"
				if i < n {
					body += "[include]\n\tpath = c" + fmt.Sprint(i+1) + ".cfg\n"
				}
				f.write(fmt.Sprintf("c%d.cfg", i), body)
			}
			f.write(".gitconfig", "[include]\n\tpath = c1.cfg\n")
			return nil
		}
	}
	return []scenario{
		{name: "include-depth-10-is-fine", setup: chain(10)},
		{name: "include-depth-12-fails", wantSub: SubIncludeDepth, setup: chain(12)},
		{name: "include-cycle-fails", wantSub: SubIncludeDepth, setup: func(t *testing.T, f fixture) []string {
			f.write("loop.cfg", "[include]\n\tpath = loop.cfg\n")
			f.write(".gitconfig", "[include]\n\tpath = loop.cfg\n")
			return nil
		}},
		{name: "include-target-unreadable-fails", wantSub: SubUnreadable, setup: func(t *testing.T, f fixture) []string {
			requireNotRoot(t)
			p := f.write("secret.cfg", "[s]\n\tk = 1\n")
			if err := os.Chmod(p, 0); err != nil {
				t.Fatal(err)
			}
			f.write(".gitconfig", "[include]\n\tpath = "+p+"\n")
			return nil
		}},
		{name: "unreadable-global-file-is-skipped", setup: func(t *testing.T, f fixture) []string {
			requireNotRoot(t)
			p := f.write(".gitconfig", "[s]\n\tk = 1\n")
			if err := os.Chmod(p, 0); err != nil {
				t.Fatal(err)
			}
			return nil
		}},
		{name: "parse-error-bad-escape", wantSub: SubParse, setup: func(t *testing.T, f fixture) []string {
			f.write(".gitconfig", "[a]\n\tk = bad\\qescape\n")
			return nil
		}},
		{name: "parse-error-in-included-file", wantSub: SubParse, setup: func(t *testing.T, f fixture) []string {
			f.write("bad.cfg", "[a\nk = 1\n")
			f.write(".gitconfig", "[include]\n\tpath = "+filepath.Join(f.home, "bad.cfg")+"\n")
			return nil
		}},
		{name: "parse-error-padded-header", wantSub: SubParse, setup: func(t *testing.T, f fixture) []string {
			f.write(".gitconfig", "[ a ]\n\tk = 1\n")
			return nil
		}},
		{name: "unknown-condition-fails-closed", wantSub: SubUnknownCondition, gitOK: true, setup: func(t *testing.T, f fixture) []string {
			f.write("t.cfg", "[t]\n\tk = 1\n")
			f.write(".gitconfig", "[includeIf \"weird:x\"]\n\tpath = "+filepath.Join(f.home, "t.cfg")+"\n")
			return nil
		}},
		{name: "unsupported-glob-bracket", wantSub: SubUnsupportedGlob, gitOK: true, setup: hasconfigInclude("https://h/[ab]/x", "https://h/a/x")},
		{name: "unsupported-glob-backslash", wantSub: SubUnsupportedGlob, gitOK: true, setup: hasconfigInclude("https://h/\\\\*", "https://h/a")},
		{name: "remote-url-in-hasconfig-included-file-matching", wantSub: SubRemoteURLInInclude, setup: func(t *testing.T, f fixture) []string {
			f.write("bad.cfg", "[remote \"x\"]\n\turl = https://m.example/r\n")
			f.write(".gitconfig", "[includeIf \"hasconfig:remote.*.url:https://m.example/**\"]\n\tpath = "+filepath.Join(f.home, "bad.cfg")+"\n")
			return nil
		}},
		{name: "remote-url-in-hasconfig-included-file-non-matching", wantSub: SubRemoteURLInInclude, setup: func(t *testing.T, f fixture) []string {
			f.write("bad.cfg", "[remote \"x\"]\n\turl = https://m.example/r\n")
			f.write(".gitconfig", "[includeIf \"hasconfig:remote.*.url:https://other.example/**\"]\n\tpath = "+filepath.Join(f.home, "bad.cfg")+"\n")
			return nil
		}},
		{name: "remote-url-in-gitdir-included-file-while-hasconfig-exists", wantSub: SubRemoteURLInInclude, setup: func(t *testing.T, f fixture) []string {
			f.write("bad.cfg", "[remote \"x\"]\n\turl = https://m.example/r\n")
			f.write("t.cfg", "[t]\n\tk = 1\n")
			f.write(".gitconfig", "[includeIf \"gitdir:"+f.root+"/\"]\n\tpath = "+filepath.Join(f.home, "bad.cfg")+"\n[includeIf \"hasconfig:remote.*.url:x\"]\n\tpath = "+filepath.Join(f.home, "t.cfg")+"\n")
			return nil
		}},
		{name: "bare-remote-url-while-hasconfig-runs", wantSub: SubBareRemoteURL, gitAny: true, setup: func(t *testing.T, f fixture) []string {
			hasconfigInclude("https://x.example/**")(t, f)
			f.appendLocal("[remote \"bk\"]\n\turl\n")
			return nil
		}},
		{name: "tilde-user-include-fails-closed", wantSub: SubUnsupportedInclude, gitAny: true, setup: func(t *testing.T, f fixture) []string {
			f.write(".gitconfig", "[include]\n\tpath = ~nobody/x.cfg\n")
			return nil
		}},
		{name: "relative-include-from-command-scope-fails-closed", wantSub: SubUnsupportedInclude, gitAny: true, setup: func(t *testing.T, f fixture) []string {
			return []string{"GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=include.path", "GIT_CONFIG_VALUE_0=rel.cfg"}
		}},
		{name: "GIT_DIR-set-fails-closed", wantSub: SubEnvOverride, gitAny: true, setup: func(t *testing.T, f fixture) []string {
			return []string{"GIT_DIR=" + filepath.Join(f.root, ".git")}
		}},
		{name: "GIT_CONFIG_PARAMETERS-set-fails-closed", wantSub: SubConfigParametersUsed, gitAny: true, setup: func(t *testing.T, f fixture) []string {
			return []string{"GIT_CONFIG_PARAMETERS='a.b=c'"}
		}},
		{name: "unresolvable-config-count", wantSub: SubEnvOverride, gitAny: true, setup: func(t *testing.T, f fixture) []string {
			return []string{"GIT_CONFIG_COUNT=2", "GIT_CONFIG_KEY_0=a.b", "GIT_CONFIG_VALUE_0=c"}
		}},
		{name: "include-path-without-value", wantSub: SubBadInclude, gitAny: true, setup: func(t *testing.T, f fixture) []string {
			f.write(".gitconfig", "[include]\n\tpath\n")
			return nil
		}},
	}
}
