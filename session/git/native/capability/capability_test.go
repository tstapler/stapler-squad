package capability

import (
	"context"
	"crypto/sha1" //nolint:gosec // the index trailer is SHA-1 by format
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/format/index"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/git/backend"
)

// fx is a hand-built repository: .git directory, a fixture HOME, and a Detector reading both.
type fx struct {
	t        *testing.T
	home     string
	root     string
	gitDir   string
	extraEnv []string
	detCfg   Config
	detOnce  *Detector
}

func newFx(t *testing.T) *fx {
	t.Helper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	f := &fx{t: t, home: filepath.Join(base, "home"), root: filepath.Join(base, "repo")}
	f.gitDir = filepath.Join(f.root, ".git")
	require.NoError(t, os.MkdirAll(filepath.Join(f.gitDir, "hooks"), 0o700))
	require.NoError(t, os.MkdirAll(f.home, 0o700))
	f.write(filepath.Join(f.gitDir, "config"), "[core]\n\trepositoryformatversion = 0\n")
	f.write(filepath.Join(f.gitDir, "HEAD"), "ref: refs/heads/main\n")
	return f
}

func (f *fx) write(path, content string) {
	f.t.Helper()
	require.NoError(f.t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(f.t, os.WriteFile(path, []byte(content), 0o600))
}

func (f *fx) appendConfig(text string) {
	f.t.Helper()
	p := filepath.Join(f.gitDir, "config")
	data, err := os.ReadFile(p)
	require.NoError(f.t, err)
	f.write(p, string(data)+text)
}

func (f *fx) hook(name string, mode os.FileMode) string {
	f.t.Helper()
	p := filepath.Join(f.gitDir, "hooks", name)
	f.write(p, "#!/bin/sh\nexit 0\n")
	require.NoError(f.t, os.Chmod(p, mode))
	return p
}

func (f *fx) detector() *Detector {
	if f.detOnce == nil {
		cfg := f.detCfg
		cfg.Environ = func() []string {
			return append([]string{"HOME=" + f.home, "GIT_CONFIG_NOSYSTEM=1", "PATH=" + os.Getenv("PATH")}, f.extraEnv...)
		}
		if cfg.SSHConfigs == nil {
			cfg.SSHConfigs = []string{filepath.Join(f.home, ".ssh", "config")}
		}
		if cfg.SystemAttributes == nil {
			cfg.SystemAttributes = []string{}
		}
		f.detOnce = New(cfg)
	}
	return f.detOnce
}

func (f *fx) caps(op backend.OperationName) ([]backend.Capability, error) {
	return f.detector().Capabilities(context.Background(), backend.Local{Root: backend.RepoRoot(f.root)}, op)
}

func kinds(caps []backend.Capability) []backend.CapabilityKind {
	out := make([]backend.CapabilityKind, 0, len(caps))
	for _, c := range caps {
		out = append(out, c.Kind)
	}
	return out
}

func (f *fx) wantKinds(op backend.OperationName, want ...backend.CapabilityKind) []backend.Capability {
	f.t.Helper()
	caps, err := f.caps(op)
	require.NoError(f.t, err)
	assert.ElementsMatch(f.t, want, kinds(caps), "%s: %+v", op, caps)
	return caps
}

// writeIndex writes a real index file with the given entries.
func (f *fx) writeIndex(entries ...*index.Entry) {
	f.t.Helper()
	file, err := os.Create(filepath.Join(f.gitDir, "index"))
	require.NoError(f.t, err)
	idx := &index.Index{Version: 2, Entries: entries}
	require.NoError(f.t, index.NewEncoder(file).Encode(idx))
	require.NoError(f.t, file.Close())
}

func entry(name string, mode filemode.FileMode) *index.Entry {
	return &index.Entry{Name: name, Mode: mode, Hash: plumbing.NewHash("0123456789012345678901234567890123456789")}
}

func TestRefsReadsNeverConsultTheDetector(t *testing.T) {
	f := newFx(t)
	f.write(filepath.Join(f.gitDir, "config"), "[broken\n") // would be a parse error
	for _, op := range []backend.OperationName{backend.OpResolveRef, backend.OpCurrentBranch, backend.OpGetConfig, backend.OpListRefs, backend.OpListWorktrees} {
		caps, err := f.caps(op)
		require.NoError(t, err, op)
		assert.Empty(t, caps, op)
	}
}

func TestHooksAnyExecutableNonSampleFileForEveryMutatingOperation(t *testing.T) {
	for _, name := range []string{"pre-commit", "post-index-change", "post-rewrite", "pre-merge-commit", "pre-rebase", "post-update",
		"push-to-checkout", "reference-transaction", "zz-future-hook"} {
		t.Run(name, func(t *testing.T) {
			f := newFx(t)
			f.hook(name, 0o755)
			for _, op := range []backend.OperationName{backend.OpAdd, backend.OpRemoveFiles, backend.OpRestore, backend.OpRenameCurrentBranch,
				backend.OpDeleteBranch, backend.OpSetUpstream, backend.OpCommit, backend.OpPush} {
				caps := f.wantKinds(op, append(otherKinds(op), backend.CapHooks)...)
				var found bool
				for _, c := range caps {
					if c.Kind == backend.CapHooks {
						found = true
						assert.Equal(t, name, c.Detail)
					}
				}
				assert.True(t, found, op)
			}
		})
	}
}

// otherKinds are the non-hook capabilities an empty fixture reports for op: none.
func otherKinds(backend.OperationName) []backend.CapabilityKind { return nil }

func TestHooksIgnoreSamplesAndNonExecutableFiles(t *testing.T) {
	f := newFx(t)
	f.hook("pre-commit.sample", 0o755)
	f.hook("pre-commit", 0o644)
	f.wantKinds(backend.OpCommit)
	assert.NoError(t, os.Mkdir(filepath.Join(f.gitDir, "hooks", "a-directory"), 0o755))
	f.wantKinds(backend.OpCommit)
}

func TestHookChmodAndEditAreSeenOnTheNextCall(t *testing.T) {
	f := newFx(t)
	p := f.hook("pre-commit", 0o644)
	f.wantKinds(backend.OpCommit)
	require.NoError(t, os.Chmod(p, 0o755))
	f.wantKinds(backend.OpCommit, backend.CapHooks)
	require.NoError(t, os.Chmod(p, 0o644))
	f.wantKinds(backend.OpCommit)
	f.hook("commit-msg", 0o755)
	f.wantKinds(backend.OpCommit, backend.CapHooks)
}

func TestHookSymlinkToExecutableCounts(t *testing.T) {
	f := newFx(t)
	target := filepath.Join(f.home, "real-hook")
	f.write(target, "#!/bin/sh\n")
	require.NoError(t, os.Chmod(target, 0o755))
	require.NoError(t, os.Symlink(target, filepath.Join(f.gitDir, "hooks", "pre-push")))
	f.wantKinds(backend.OpPush, backend.CapHooks)
}

func TestHooksPathFromLocalGlobalAndWorktreeConfig(t *testing.T) {
	setups := map[string]func(f *fx, dir string){
		"local": func(f *fx, dir string) { f.appendConfig("[core]\n\thooksPath = " + dir + "\n") },
		"global": func(f *fx, dir string) {
			f.write(filepath.Join(f.home, ".gitconfig"), "[core]\n\thooksPath = "+dir+"\n")
		},
		"tilde": func(f *fx, dir string) { f.write(filepath.Join(f.home, ".gitconfig"), "[core]\n\thooksPath = ~/hk\n") },
		"relative-to-worktree": func(f *fx, dir string) {
			f.appendConfig("[core]\n\thooksPath = ../hk\n")
			require.NoError(f.t, os.Rename(filepath.Join(f.home, "hk"), filepath.Join(filepath.Dir(f.root), "hk")))
		},
		"config.worktree": func(f *fx, dir string) {
			f.appendConfig("[extensions]\n\tworktreeConfig = true\n")
			f.write(filepath.Join(f.gitDir, "config.worktree"), "[core]\n\thooksPath = "+dir+"\n")
		},
	}
	for name, setup := range setups {
		t.Run(name, func(t *testing.T) {
			f := newFx(t)
			dir := filepath.Join(f.home, "hk")
			require.NoError(t, os.MkdirAll(dir, 0o700))
			p := filepath.Join(dir, "pre-commit")
			f.write(p, "#!/bin/sh\n")
			require.NoError(t, os.Chmod(p, 0o755))
			setup(f, dir)
			f.wantKinds(backend.OpCommit, backend.CapHooks)
		})
	}
}

func TestHooksPathHidesTheDefaultHooksDirectory(t *testing.T) {
	f := newFx(t)
	f.hook("pre-commit", 0o755)
	empty := filepath.Join(f.home, "empty-hooks")
	require.NoError(t, os.MkdirAll(empty, 0o700))
	f.appendConfig("[core]\n\thooksPath = " + empty + "\n")
	f.wantKinds(backend.OpCommit)
}

func TestHookConfigKeysCount(t *testing.T) {
	f := newFx(t)
	f.appendConfig("[hook \"lint\"]\n\tcommand = ./lint\n\tevent = pre-commit\n")
	caps := f.wantKinds(backend.OpAdd, backend.CapHooks)
	assert.Equal(t, "config:hook.lint.command", caps[0].Detail)
}

func TestLinkedWorktreeUsesTheMainRepositoryHooksAndConfig(t *testing.T) {
	f := newFx(t)
	f.hook("pre-commit", 0o755)
	linked := filepath.Join(filepath.Dir(f.root), "linked")
	admin := filepath.Join(f.gitDir, "worktrees", "linked")
	f.write(filepath.Join(admin, "commondir"), "../..\n")
	f.write(filepath.Join(admin, "HEAD"), "ref: refs/heads/wt\n")
	f.write(filepath.Join(linked, ".git"), "gitdir: "+admin+"\n")
	f.root = linked
	f.wantKinds(backend.OpCommit, backend.CapHooks)

	// a relative gitdir line resolves against the worktree
	f.write(filepath.Join(linked, ".git"), "gitdir: "+mustRel(t, linked, admin)+"\n")
	f.wantKinds(backend.OpCommit, backend.CapHooks)
}

func mustRel(t *testing.T, base, target string) string {
	t.Helper()
	r, err := filepath.Rel(base, target)
	require.NoError(t, err)
	return r
}

func TestSigningShapes(t *testing.T) {
	cases := map[string]struct {
		config string
		want   bool
	}{
		"commit.gpgsign true":      {"[commit]\n\tgpgsign = true\n", true},
		"commit.gpgsign yes":       {"[commit]\n\tgpgsign = yes\n", true},
		"commit.gpgsign bare":      {"[commit]\n\tgpgsign\n", true},
		"commit.gpgsign 1":         {"[commit]\n\tgpgsign = 1\n", true},
		"commit.gpgsign false":     {"[commit]\n\tgpgsign = false\n", false},
		"commit.gpgsign empty":     {"[commit]\n\tgpgsign =\n", false},
		"commit.gpgsign quoted":    {"[commit]\n\tgpgsign = \"\"\n", false},
		"commit.gpgsign 0":         {"[commit]\n\tgpgsign = 0\n", false},
		"commit.gpgsign garbage":   {"[commit]\n\tgpgsign = banana\n", true},
		"tag.gpgsign":              {"[tag]\n\tgpgSign = true\n", true},
		"push.gpgsign if-asked":    {"[push]\n\tgpgSign = if-asked\n", true},
		"push.gpgsign true":        {"[push]\n\tgpgSign = true\n", true},
		"push.gpgsign false":       {"[push]\n\tgpgSign = false\n", false},
		"gpg.format presence":      {"[gpg]\n\tformat = ssh\n", true},
		"gpg.program presence":     {"[gpg]\n\tprogram = gpg2\n", true},
		"user.signingkey presence": {"[user]\n\tsigningkey = ABC\n", true},
		"unrelated user.name":      {"[user]\n\tname = x\n", false},
		"last value wins":          {"[commit]\n\tgpgsign = true\n\tgpgsign = false\n", false},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			f := newFx(t)
			f.appendConfig(c.config)
			if c.want {
				f.wantKinds(backend.OpCommit, backend.CapGPGSign)
			} else {
				f.wantKinds(backend.OpCommit)
			}
		})
	}
}

func TestSigningFromGlobalConfigAndOnlyForCommitCreatingOperations(t *testing.T) {
	f := newFx(t)
	f.write(filepath.Join(f.home, ".gitconfig"), "[commit]\n\tgpgsign = true\n")
	f.wantKinds(backend.OpCommit, backend.CapGPGSign)
	f.wantKinds(backend.OpPush, backend.CapGPGSign)
	f.wantKinds(backend.OpAdd) // Add does not sign
}

func TestSigningThroughConditionalIncludeFollowsGitdirMatch(t *testing.T) {
	f := newFx(t)
	f.write(filepath.Join(f.home, "extra.cfg"), "[commit]\n\tgpgsign = true\n")
	f.write(filepath.Join(f.home, ".gitconfig"), "[includeIf \"gitdir:"+filepath.Dir(f.root)+"/\"]\n\tpath = "+filepath.Join(f.home, "extra.cfg")+"\n")
	f.wantKinds(backend.OpCommit, backend.CapGPGSign)
	f.write(filepath.Join(f.home, ".gitconfig"), "[includeIf \"gitdir:/elsewhere/\"]\n\tpath = "+filepath.Join(f.home, "extra.cfg")+"\n")
	f.wantKinds(backend.OpCommit)
}

func TestFilterNeedsAnAttributeThatReferencesAConfiguredDriver(t *testing.T) {
	f := newFx(t)
	f.write(filepath.Join(f.home, ".gitconfig"), "[filter \"lfs\"]\n\tclean = git-lfs clean -- %f\n\tsmudge = git-lfs smudge -- %f\n")
	f.wantKinds(backend.OpStatus) // configured everywhere, referenced nowhere

	sources := map[string]func(){
		"root .gitattributes": func() { f.write(filepath.Join(f.root, ".gitattributes"), "*.bin filter=lfs diff=lfs\n") },
		"nested tracked": func() {
			f.write(filepath.Join(f.root, "sub", ".gitattributes"), "*.bin filter=lfs\n")
			f.writeIndex(entry("sub/.gitattributes", filemode.Regular))
		},
		"info/attributes": func() { f.write(filepath.Join(f.gitDir, "info", "attributes"), "*.bin filter=lfs\n") },
		"core.attributesFile": func() {
			f.write(filepath.Join(f.home, "attrs"), "*.bin filter=lfs\n")
			f.write(filepath.Join(f.home, ".gitconfig"), "[core]\n\tattributesFile = ~/attrs\n[filter \"lfs\"]\n\tclean = x\n")
		},
		"default XDG attributes": func() { f.write(filepath.Join(f.home, ".config", "git", "attributes"), "*.bin filter=lfs\n") },
		"macro": func() {
			f.write(filepath.Join(f.root, ".gitattributes"), "[attr]lfs filter=lfs diff=lfs\n*.bin lfs\n")
		},
	}
	for name, add := range sources {
		t.Run(name, func(t *testing.T) {
			g := newFx(t)
			g.write(filepath.Join(g.home, ".gitconfig"), "[filter \"lfs\"]\n\tclean = git-lfs clean -- %f\n")
			f = g
			add()
			caps := f.wantKinds(backend.OpStatus, backend.CapLFS)
			assert.Equal(t, "filter=lfs", caps[0].Detail)
		})
	}
}

func TestFilterWithoutConfiguredDriverDoesNotRoute(t *testing.T) {
	f := newFx(t)
	f.write(filepath.Join(f.root, ".gitattributes"), "*.bin filter=lfs\n-text\n# filter=lfs\n*.x -filter\n")
	f.wantKinds(backend.OpStatus)
}

func TestFilterCacheInvalidatesOnContentChangeOfSameSize(t *testing.T) {
	f := newFx(t)
	f.write(filepath.Join(f.home, ".gitconfig"), "[filter \"lfs\"]\n\tclean = x\n")
	attrs := filepath.Join(f.root, ".gitattributes")
	f.write(attrs, "*.bin filter=zzz\n")
	f.wantKinds(backend.OpStatus)
	f.write(attrs, "*.bin filter=lfs\n") // same size
	f.wantKinds(backend.OpStatus, backend.CapLFS)
}

func TestIndexFactsAndCacheInvalidation(t *testing.T) {
	f := newFx(t)
	f.writeIndex(entry("a.txt", filemode.Regular))
	f.wantKinds(backend.OpStatus)

	f.writeIndex(entry("a.txt", filemode.Regular), entry("sub", filemode.Submodule))
	f.wantKinds(backend.OpStatus, backend.CapSubmodule)

	skipped := entry("b.txt", filemode.Regular)
	skipped.SkipWorktree = true
	file, err := os.Create(filepath.Join(f.gitDir, "index"))
	require.NoError(t, err)
	require.NoError(t, index.NewEncoder(file).Encode(&index.Index{Version: 3, Entries: []*index.Entry{skipped}}))
	require.NoError(t, file.Close())
	f.wantKinds(backend.OpStatus, backend.CapSparse)
}

func TestSplitIndexIsUnsupportedButV3AndV4AreNot(t *testing.T) {
	f := newFx(t)
	f.writeSplitIndex()
	f.wantKinds(backend.OpStatus, backend.CapUnsupportedIndex)

	for _, version := range []uint32{3, 4} {
		file, err := os.Create(filepath.Join(f.gitDir, "index"))
		require.NoError(t, err)
		e := entry("a.txt", filemode.Regular)
		if version == 3 {
			e.IntentToAdd = true // forces the extended-flags (v3) layout
		}
		require.NoError(t, index.NewEncoder(file).Encode(&index.Index{Version: version, Entries: []*index.Entry{e}}))
		require.NoError(t, file.Close())
		f.wantKinds(backend.OpStatus)
	}
}

// writeSplitIndex writes an index with a mandatory "link" extension, which is what
// `git update-index --split-index` produces.
func (f *fx) writeSplitIndex() {
	f.t.Helper()
	body := []byte("DIRC")
	body = binary.BigEndian.AppendUint32(body, 2)
	body = binary.BigEndian.AppendUint32(body, 0)
	body = append(body, "link"...)
	body = binary.BigEndian.AppendUint32(body, 20)
	body = append(body, make([]byte, 20)...)
	sum := sha1.Sum(body) //nolint:gosec // format-mandated
	require.NoError(f.t, os.WriteFile(filepath.Join(f.gitDir, "index"), append(body, sum[:]...), 0o600))
}

func TestCorruptIndexFailsClosed(t *testing.T) {
	f := newFx(t)
	f.write(filepath.Join(f.gitDir, "index"), "this is not an index, but it is long enough to have a trailer")
	_, err := f.caps(backend.OpStatus)
	var de *backend.DetectError
	require.ErrorAs(t, err, &de)
	assert.Equal(t, backend.DetectUnsupportedIndex, de.Sub)
}

func TestShallowRepository(t *testing.T) {
	f := newFx(t)
	f.wantKinds(backend.OpMergeBase)
	f.write(filepath.Join(f.gitDir, "shallow"), "0123456789012345678901234567890123456789\n")
	f.wantKinds(backend.OpMergeBase, backend.CapShallow)
	f.wantKinds(backend.OpResolveRef)
}

func TestLocalTransportIncludingRewrites(t *testing.T) {
	cases := map[string]struct {
		config string
		want   bool
	}{
		"https":              {"[remote \"o\"]\n\turl = https://h/r.git\n", false},
		"scp-style":          {"[remote \"o\"]\n\turl = git@h:r.git\n", false},
		"ssh url":            {"[remote \"o\"]\n\turl = ssh://h/r.git\n", false},
		"absolute path":      {"[remote \"o\"]\n\turl = /srv/r.git\n", true},
		"relative path":      {"[remote \"o\"]\n\turl = ../r.git\n", true},
		"file url":           {"[remote \"o\"]\n\turl = file:///srv/r.git\n", true},
		"pushurl is a path":  {"[remote \"o\"]\n\turl = https://h/r.git\n\tpushurl = /srv/r.git\n", true},
		"insteadOf to path":  {"[url \"/mirror/\"]\n\tinsteadOf = https://h/\n[remote \"o\"]\n\turl = https://h/r.git\n", true},
		"insteadOf to https": {"[url \"https://m/\"]\n\tinsteadOf = gh:\n[remote \"o\"]\n\turl = gh:r.git\n", false},
		"pushInsteadOf":      {"[url \"/mirror/\"]\n\tpushInsteadOf = https://h/\n[remote \"o\"]\n\turl = https://h/r.git\n", true},
		"longest prefix":     {"[url \"/short/\"]\n\tinsteadOf = https://\n[url \"https://m/\"]\n\tinsteadOf = https://h/\n[remote \"o\"]\n\turl = https://h/r.git\n", false},
		"no remotes":         {"", false},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			f := newFx(t)
			f.appendConfig(c.config)
			if c.want {
				f.wantKinds(backend.OpFetch, backend.CapLocalTransport)
			} else {
				f.wantKinds(backend.OpFetch)
			}
		})
	}
}

func TestLocalTransportFromGlobalRewriteAndNotForCommit(t *testing.T) {
	f := newFx(t)
	f.write(filepath.Join(f.home, ".gitconfig"), "[url \"/mirror/\"]\n\tinsteadOf = https://h/\n")
	f.appendConfig("[remote \"o\"]\n\turl = https://h/r.git\n")
	f.wantKinds(backend.OpPush, backend.CapLocalTransport)
	f.wantKinds(backend.OpCommit)
}

func TestSSHProxyDirectives(t *testing.T) {
	sshRemote := "[remote \"o\"]\n\turl = git@h:r.git\n"
	t.Run("ProxyJump in ssh config", func(t *testing.T) {
		f := newFx(t)
		f.appendConfig(sshRemote)
		f.write(filepath.Join(f.home, ".ssh", "config"), "Host h\n  ProxyJump bastion\n")
		f.wantKinds(backend.OpFetch, backend.CapSSHProxy)
	})
	t.Run("ProxyCommand via Include", func(t *testing.T) {
		f := newFx(t)
		f.appendConfig(sshRemote)
		f.write(filepath.Join(f.home, ".ssh", "config"), "Include conf.d/*\n")
		f.write(filepath.Join(f.home, ".ssh", "conf.d", "corp"), "Host *.corp\n\tProxyCommand nc %h %p\n")
		f.wantKinds(backend.OpFetch, backend.CapSSHProxy)
	})
	t.Run("Match", func(t *testing.T) {
		f := newFx(t)
		f.appendConfig(sshRemote)
		f.write(filepath.Join(f.home, ".ssh", "config"), "match host h\n  User x\n")
		f.wantKinds(backend.OpFetch, backend.CapSSHProxy)
	})
	t.Run("plain ssh config", func(t *testing.T) {
		f := newFx(t)
		f.appendConfig(sshRemote)
		f.write(filepath.Join(f.home, ".ssh", "config"), "Host h\n  User git\n  IdentityFile ~/.ssh/id\n")
		f.wantKinds(backend.OpFetch)
	})
	t.Run("core.sshCommand", func(t *testing.T) {
		f := newFx(t)
		f.appendConfig(sshRemote + "[core]\n\tsshCommand = ssh -F /x\n")
		f.wantKinds(backend.OpFetch, backend.CapSSHProxy)
	})
	t.Run("GIT_SSH_COMMAND", func(t *testing.T) {
		f := newFx(t)
		f.appendConfig(sshRemote)
		f.extraEnv = []string{"GIT_SSH_COMMAND=ssh -v"}
		f.wantKinds(backend.OpFetch, backend.CapSSHProxy)
	})
	t.Run("https remote ignores ssh config", func(t *testing.T) {
		f := newFx(t)
		f.appendConfig("[remote \"o\"]\n\turl = https://h/r.git\n")
		f.write(filepath.Join(f.home, ".ssh", "config"), "Host *\n  ProxyJump x\n")
		f.wantKinds(backend.OpFetch)
	})
}

func TestFailClosedCases(t *testing.T) {
	cases := map[string]struct {
		setup func(f *fx)
		op    backend.OperationName
		sub   backend.DetectSub
	}{
		"parse error in repo config":    {func(f *fx) { f.appendConfig("[a\nk=1\n") }, backend.OpCommit, backend.DetectParse},
		"unknown includeIf condition":   {func(f *fx) { f.write(filepath.Join(f.home, ".gitconfig"), "[includeIf \"weird:x\"]\n\tpath = x\n") }, backend.OpCommit, backend.DetectUnknownCondition},
		"GIT_DIR in the environment":    {func(f *fx) { f.extraEnv = []string{"GIT_DIR=/x"} }, backend.OpCommit, backend.DetectEnvOverride},
		"GIT_INDEX_FILE":                {func(f *fx) { f.extraEnv = []string{"GIT_INDEX_FILE=/x"} }, backend.OpCommit, backend.DetectEnvOverride},
		"GIT_CONFIG_PARAMETERS":         {func(f *fx) { f.extraEnv = []string{"GIT_CONFIG_PARAMETERS='a.b=c'"} }, backend.OpCommit, backend.DetectConfigParameters},
		"missing .git":                  {func(f *fx) { require.NoError(f.t, os.RemoveAll(f.gitDir)) }, backend.OpCommit, backend.DetectCommonDir},
		"unresolvable commondir":        {func(f *fx) { f.linkedWithBadCommondir() }, backend.OpCommit, backend.DetectCommonDir},
		".git file without gitdir line": {func(f *fx) { f.replaceGitWithFile("nonsense\n") }, backend.OpCommit, backend.DetectCommonDir},
		"include cycle":                 {func(f *fx) { f.cycle() }, backend.OpCommit, backend.DetectIncludeDepth},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			f := newFx(t)
			c.setup(f)
			caps, err := f.caps(c.op)
			require.Error(t, err)
			assert.Nil(t, caps)
			var de *backend.DetectError
			require.ErrorAs(t, err, &de)
			assert.Equal(t, c.sub, de.Sub)
		})
	}
}

func (f *fx) linkedWithBadCommondir() {
	admin := filepath.Join(f.gitDir, "worktrees", "w")
	f.write(filepath.Join(admin, "commondir"), "../../nowhere\n")
	f.replaceGitWithFile("gitdir: " + admin + "\n")
}

func (f *fx) replaceGitWithFile(content string) {
	f.t.Helper()
	moved := filepath.Join(filepath.Dir(f.root), "moved-git")
	require.NoError(f.t, os.Rename(f.gitDir, moved))
	f.write(f.gitDir, content)
	f.gitDir = moved
}

func (f *fx) cycle() {
	f.write(filepath.Join(f.home, "loop.cfg"), "[include]\n\tpath = loop.cfg\n")
	f.write(filepath.Join(f.home, ".gitconfig"), "[include]\n\tpath = loop.cfg\n")
}

func TestContextCancellationIsACanceledDetectError(t *testing.T) {
	f := newFx(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := f.detector().Capabilities(ctx, backend.Local{Root: backend.RepoRoot(f.root)}, backend.OpCommit)
	require.Error(t, err)
}

func TestEveryGitconfigSubMapsToAKnownDetectSub(t *testing.T) {
	known := map[string]bool{}
	for _, s := range backend.AllDetectSubs() {
		known[string(s)] = true
	}
	for _, s := range gitconfigSubs() {
		assert.True(t, known[string(s)], "gitconfig sub %q has no backend.DetectSub", s)
	}
}

func lfsHome(f *fx) {
	f.write(filepath.Join(f.home, ".gitconfig"), "[filter \"foo\"]\n\tclean = cat\n")
}

func TestFilterInUntrackedNestedGitattributes(t *testing.T) {
	f := newFx(t)
	lfsHome(f)
	f.write(filepath.Join(f.root, "newdir", ".gitattributes"), "* filter=foo\n") // `git lfs track` in a new directory; nothing is in the index
	f.wantKinds(backend.OpAdd, backend.CapLFS)
	f.wantKinds(backend.OpStatus, backend.CapLFS)
}

func TestFilterSkipsNestedRepositoriesAndDotGit(t *testing.T) {
	f := newFx(t)
	lfsHome(f)
	f.write(filepath.Join(f.root, "vendor", "lib", ".git"), "gitdir: elsewhere\n")
	f.write(filepath.Join(f.root, "vendor", "lib", ".gitattributes"), "* filter=foo\n")
	f.wantKinds(backend.OpStatus)
}

func TestFilterNestedMacrosAndMacrosFromOtherSources(t *testing.T) {
	f := newFx(t)
	lfsHome(f)
	f.write(filepath.Join(f.root, ".gitattributes"), "[attr]inner filter=foo\n[attr]outer inner\n*.dat outer\n")
	f.wantKinds(backend.OpStatus, backend.CapLFS)

	g := newFx(t)
	lfsHome(g)
	g.write(filepath.Join(g.home, ".config", "git", "attributes"), "[attr]viafoo filter=foo\n")
	g.write(filepath.Join(g.root, "sub", ".gitattributes"), "*.dat viafoo\n")
	g.wantKinds(backend.OpStatus, backend.CapLFS)
}

func TestFilterMacroCyclesTerminate(t *testing.T) {
	f := newFx(t)
	lfsHome(f)
	f.write(filepath.Join(f.root, ".gitattributes"), "[attr]a b\n[attr]b a\n*.dat a\n")
	f.wantKinds(backend.OpStatus)
}

func TestRequiredFilterWithoutDriverStillCounts(t *testing.T) {
	f := newFx(t)
	f.write(filepath.Join(f.home, ".gitconfig"), "[filter \"rq\"]\n\trequired = true\n")
	f.write(filepath.Join(f.root, ".gitattributes"), "*.dat filter=rq\n")
	f.wantKinds(backend.OpStatus, backend.CapLFS) // git dies on a required filter with no driver; let it say so
}

func TestNoConfiguredFilterMeansNoWorktreeScan(t *testing.T) {
	f := newFx(t)
	// an unreadable attributes file would fail closed if it were opened
	p := filepath.Join(f.root, ".gitattributes")
	f.write(p, "*.dat filter=x\n")
	require.NoError(t, os.Chmod(p, 0))
	t.Cleanup(func() { _ = os.Chmod(p, 0o600) })
	f.wantKinds(backend.OpStatus)
}

func TestHooksPathInsideTheWorktreeAlwaysRoutes(t *testing.T) {
	for name, value := range map[string]string{"relative": ".githooks", "relative dot": "./hooks/ci", "absolute inside": "ROOT/.githooks"} {
		t.Run(name, func(t *testing.T) {
			f := newFx(t)
			v := value
			if v == "ROOT/.githooks" {
				v = filepath.Join(f.root, ".githooks")
			}
			f.appendConfig("[core]\n\thooksPath = " + v + "\n") // the directory does not exist yet: a checkout may create it
			caps := f.wantKinds(backend.OpSwitchBranch, backend.CapHooks)
			assert.Equal(t, "core.hooksPath:inside-worktree", caps[0].Detail)
		})
	}
	g := newFx(t)
	g.appendConfig("[core]\n\thooksPath = ~someone/hooks\n")
	g.wantKinds(backend.OpCommit, backend.CapHooks)
	h := newFx(t)
	h.appendConfig("[core]\n\thooksPath = %(prefix)/hooks\n")
	h.wantKinds(backend.OpCommit, backend.CapHooks)
}

func TestSSHConfigIncludeTildeAndQuotedKeyword(t *testing.T) {
	sshRemote := "[remote \"o\"]\n\turl = git@h:r.git\n"
	f := newFx(t)
	f.appendConfig(sshRemote)
	f.write(filepath.Join(f.home, ".ssh", "config"), "Include ~/.ssh/conf.d/*\n")
	f.write(filepath.Join(f.home, ".ssh", "conf.d", "a"), "Host h\n  ProxyCommand nc %h %p\n")
	f.wantKinds(backend.OpFetch, backend.CapSSHProxy)

	g := newFx(t)
	g.appendConfig(sshRemote)
	g.write(filepath.Join(g.home, ".ssh", "config"), "Host h\n  \"ProxyCommand\" nc %h %p\n")
	g.wantKinds(backend.OpFetch, backend.CapSSHProxy)

	h := newFx(t)
	h.appendConfig(sshRemote)
	h.write(filepath.Join(h.home, ".ssh", "config"), "Host h\n  ProxyJump=bastion\n")
	h.wantKinds(backend.OpFetch, backend.CapSSHProxy)
}

func TestRemoteHelperAndDriveLetterURLsRouteAsLocal(t *testing.T) {
	for _, u := range []string{"ext::sh -c 'cat'", "codecommit::us-east-1://repo", "hg::https://x/y", "persistent-https::https://x", `C:/repo`, `C:\\repo`} {
		f := newFx(t)
		f.appendConfig("[remote \"o\"]\n\turl = " + u + "\n")
		f.wantKinds(backend.OpFetch, backend.CapLocalTransport)
	}
}

func TestGitProtocolProxy(t *testing.T) {
	remote := "[remote \"o\"]\n\turl = git://h/r.git\n"
	f := newFx(t)
	f.appendConfig(remote)
	f.wantKinds(backend.OpFetch)
	f.appendConfig("[core]\n\tgitProxy = nc %h\n")
	f.wantKinds(backend.OpFetch, backend.CapSSHProxy)
	g := newFx(t)
	g.appendConfig(remote)
	g.extraEnv = []string{"GIT_PROXY_COMMAND=/bin/proxy"}
	g.wantKinds(backend.OpFetch, backend.CapSSHProxy)
}

func TestFetchSeesSubmodules(t *testing.T) {
	f := newFx(t)
	f.writeIndex(entry("sub", filemode.Submodule))
	f.wantKinds(backend.OpFetch, backend.CapSubmodule)
}

func TestSSHConfigLongLineDoesNotEndTheScan(t *testing.T) {
	f := newFx(t)
	f.appendConfig("[remote \"o\"]\n\turl = git@h:r.git\n")
	f.write(filepath.Join(f.home, ".ssh", "config"), "# "+strings.Repeat("x", 70000)+"\nHost h\n  ProxyJump bastion\n")
	f.wantKinds(backend.OpFetch, backend.CapSSHProxy)
}

func TestUnreadableHooksDirectoryFailsClosed(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission rows need a non-root user")
	}
	f := newFx(t)
	dir := filepath.Join(f.gitDir, "hooks")
	require.NoError(t, os.Chmod(dir, 0))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	_, err := f.caps(backend.OpCommit)
	var de *backend.DetectError
	require.ErrorAs(t, err, &de)
	assert.Equal(t, backend.DetectHooksUnreadable, de.Sub)
}

func TestIndexCacheIsKeyedByContentNotSize(t *testing.T) {
	f := newFx(t)
	f.writeIndex(entry("sub", filemode.Regular))
	f.wantKinds(backend.OpStatus)
	before, err := os.Stat(filepath.Join(f.gitDir, "index"))
	require.NoError(t, err)
	f.writeIndex(entry("sub", filemode.Submodule)) // same name, same encoded size, different content
	after, err := os.Stat(filepath.Join(f.gitDir, "index"))
	require.NoError(t, err)
	require.Equal(t, before.Size(), after.Size(), "premise: same size")
	f.wantKinds(backend.OpStatus, backend.CapSubmodule)
}

func TestDetectorIsSafeForConcurrentUse(t *testing.T) {
	f := newFx(t)
	f.writeIndex(entry("a.txt", filemode.Regular))
	d := f.detector()
	var wg sync.WaitGroup
	for i := range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if i%4 == 0 {
				f.writeIndexRacy(entry("a.txt", filemode.Regular), entry(fmt.Sprintf("n%d", i), filemode.Regular))
			}
			_, _ = d.Capabilities(context.Background(), backend.Local{Root: backend.RepoRoot(f.root)}, backend.OpStatus)
		}()
	}
	wg.Wait()
}

// writeIndexRacy is writeIndex without test assertions, for use off the test goroutine.
func (f *fx) writeIndexRacy(entries ...*index.Entry) {
	file, err := os.Create(filepath.Join(f.gitDir, "index"))
	if err != nil {
		return
	}
	_ = index.NewEncoder(file).Encode(&index.Index{Version: 2, Entries: entries})
	_ = file.Close()
}

func TestUnparseableRemoteURLRoutesToTheCLI(t *testing.T) {
	f := newFx(t)
	f.appendConfig("[remote \"o\"]\n\turl = ssh://[bad\n")
	f.wantKinds(backend.OpFetch, backend.CapLocalTransport)
}

func TestFilterDriverKindsAndOtherAttributeSources(t *testing.T) {
	for _, key := range []string{"clean", "smudge", "process", "required"} {
		f := newFx(t)
		f.write(filepath.Join(f.home, ".gitconfig"), "[filter \"foo\"]\n\t"+key+" = x\n")
		f.write(filepath.Join(f.root, ".gitattributes"), "*.dat filter=foo\n")
		f.wantKinds(backend.OpStatus, backend.CapLFS)
	}
	// a driver key that is not one of the four configures nothing
	g := newFx(t)
	g.write(filepath.Join(g.home, ".gitconfig"), "[filter \"foo\"]\n\tsomethingElse = x\n")
	g.write(filepath.Join(g.root, ".gitattributes"), "*.dat filter=foo\n")
	g.wantKinds(backend.OpStatus)

	// system attributes and an explicit XDG_CONFIG_HOME
	h := newFx(t)
	lfsHome(h)
	sys := filepath.Join(h.home, "sysattrs")
	h.write(sys, "*.dat filter=foo\n")
	h.detCfg.SystemAttributes = []string{sys}
	h.wantKinds(backend.OpStatus, backend.CapLFS)

	x := newFx(t)
	lfsHome(x)
	xdg := filepath.Join(x.home, "xdgdir")
	x.write(filepath.Join(xdg, "git", "attributes"), "*.dat filter=foo\n")
	x.extraEnv = []string{"XDG_CONFIG_HOME=" + xdg}
	x.wantKinds(backend.OpStatus, backend.CapLFS)
}

func TestUnreadableAttributesFileFailsClosedWhenAFilterIsConfigured(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission rows need a non-root user")
	}
	f := newFx(t)
	lfsHome(f)
	p := filepath.Join(f.root, ".gitattributes")
	f.write(p, "*.dat filter=foo\n")
	require.NoError(t, os.Chmod(p, 0))
	t.Cleanup(func() { _ = os.Chmod(p, 0o600) })
	_, err := f.caps(backend.OpStatus)
	var de *backend.DetectError
	require.ErrorAs(t, err, &de)
	assert.Equal(t, backend.DetectAttributesUnread, de.Sub)
}

func TestEmptyShallowFileIsNotShallow(t *testing.T) {
	f := newFx(t)
	f.write(filepath.Join(f.gitDir, "shallow"), "")
	f.wantKinds(backend.OpMergeBase)
}
