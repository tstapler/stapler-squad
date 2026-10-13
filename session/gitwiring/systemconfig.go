package gitwiring

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/tstapler/stapler-squad/executor/safeexec"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/git/backend"
	"github.com/tstapler/stapler-squad/session/git/backend/gogit"
)

// SystemConfigFiles are the system-scope config files git reads.
type SystemConfigFiles struct {
	// Default is the file(s) GIT_CONFIG_SYSTEM replaces: git's compiled-in system config.
	Default []string
	// Vendor are files a build adds that GIT_CONFIG_SYSTEM does not replace (Apple git's
	// share/git-core/gitconfig); only GIT_CONFIG_NOSYSTEM drops them.
	Vendor []string
}

// DiscoverSystemConfig asks git, once, which system-wide config files it reads. The path is
// compiled into the git binary (and differs between Apple git, Homebrew, distro packages), so
// the in-process backend, which never starts a process, cannot learn it itself.
//
// git runs with HOME, XDG and every GIT_CONFIG_* / GIT_DIR variable scrubbed so only system
// config can show up, and three facts are combined:
//   - the origin of every entry of `git config --list --no-includes`: the files git really reads
//     (this is how Apple's share/git-core/gitconfig is found);
//   - the same listing with GIT_CONFIG_SYSTEM=/dev/null: what survives is vendor-added, and the
//     rest is the replaceable default;
//   - the compiled-in default path named by git's "unable to read config file" error for a
//     missing system file, so a file created after startup is still honoured.
//
// GIT_CONFIG_SYSTEM and GIT_CONFIG_NOSYSTEM themselves are not applied here: the backend applies
// them at read time.
//
// Known gap: git names a file only through its entries (or its "unable to read" error when it is
// missing), so a system or vendor file that is empty at startup, or a vendor file that does not
// exist yet, is not discovered; a restart picks it up. The result is also fixed at startup: a
// different git binary on PATH later is not noticed.
func DiscoverSystemConfig(ctx context.Context) (SystemConfigFiles, error) {
	dir, err := os.MkdirTemp("", "ssq-gitcfg-")
	if err != nil {
		return SystemConfigFiles{}, fmt.Errorf("system git config discovery: %w", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()
	counter := backend.NewSpawnCounter(soakMeter())
	run := func(extraEnv []string, args ...string) ([]byte, []byte, error) {
		return runScrubbedGit(ctx, counter, dir, extraEnv, args...)
	}
	return discoverSystemConfig(run)
}

// gitRun runs git with args in the scrubbed sandbox and returns stdout and stderr separately.
type gitRun func(extraEnv []string, args ...string) (stdout, stderr []byte, err error)

func discoverSystemConfig(run gitRun) (SystemConfigFiles, error) {
	listing := []string{"config", "--show-origin", "--list", "--no-includes", "-z"}
	out, _, err := run(nil, listing...)
	if err != nil {
		return SystemConfigFiles{}, fmt.Errorf("system git config discovery: %w", err)
	}
	all, err := parseOriginList(out)
	if err != nil {
		return SystemConfigFiles{}, err
	}
	out, _, err = run([]string{"GIT_CONFIG_SYSTEM=" + os.DevNull}, listing...)
	if err != nil {
		return SystemConfigFiles{}, fmt.Errorf("system git config discovery: %w", err)
	}
	vendor, err := parseOriginList(out)
	if err != nil {
		return SystemConfigFiles{}, err
	}
	var files SystemConfigFiles
	// The compiled-in default is named by the error git gives when that file is missing.
	if _, stderr, sysErr := run(nil, "config", "--system", "--list"); sysErr != nil {
		if m := missingSystemConfig.FindSubmatch(stderr); m != nil {
			files.Default = append(files.Default, string(m[1]))
		}
	}
	for _, p := range all {
		if containsString(vendor, p) {
			continue
		}
		if !containsString(files.Default, p) {
			files.Default = append(files.Default, p)
		}
	}
	files.Vendor = vendor
	return files, nil
}

var missingSystemConfig = regexp.MustCompile(`unable to read config file '(.+)': `)

// parseOriginList reads `git config --show-origin --list -z` output: for each entry the origin
// ("file:<path>", "command line:", ...) then "key\nvalue" (or just "key"), each NUL-terminated.
// It returns the distinct file origins in first-seen order.
func parseOriginList(out []byte) ([]string, error) {
	if len(out) == 0 {
		return nil, nil
	}
	fields := bytes.Split(bytes.TrimSuffix(out, []byte{0}), []byte{0})
	if len(fields)%2 != 0 {
		return nil, errors.New("system git config discovery: malformed `git config --list -z` output")
	}
	var paths []string
	for i := 0; i < len(fields); i += 2 {
		origin := string(fields[i])
		path, ok := strings.CutPrefix(origin, "file:")
		if !ok {
			continue
		}
		if !containsString(paths, path) {
			paths = append(paths, path)
		}
	}
	return paths, nil
}

func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// scrubbedGitEnv is base minus everything that selects config sources, with HOME and XDG
// pointing at an empty directory, so git reports system scope only.
func scrubbedGitEnv(base []string, sandbox string) []string {
	var env []string
	for _, kv := range base {
		name, _, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(name, "GIT_CONFIG") || strings.HasPrefix(name, "GIT_DIR") ||
			name == "HOME" || name == "XDG_CONFIG_HOME" || name == "GIT_WORK_TREE" ||
			name == "GIT_COMMON_DIR" || name == "GIT_INDEX_FILE" || name == "GIT_CEILING_DIRECTORIES" {
			continue
		}
		env = append(env, kv)
	}
	return append(env,
		"HOME="+sandbox,
		"XDG_CONFIG_HOME="+filepath.Join(sandbox, "xdg"),
		"GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_CEILING_DIRECTORIES="+filepath.Dir(sandbox),
	)
}

// discoveryEnv is the full environment of a discovery git process: scrubbed, plus the override
// under test, plus the locale the "unable to read" error parsing depends on.
func discoveryEnv(base []string, sandbox string, extraEnv []string) []string {
	return append(append(scrubbedGitEnv(base, sandbox), extraEnv...), backend.RequiredGitEnv()...)
}

// runScrubbedGit is the one place discovery starts git. The spawn is counted under
// (GetConfig, config) so the zero-spawn gate (sum(backstop) <= sum(backend)) stays balanced.
func runScrubbedGit(ctx context.Context, counter *backend.SpawnCounter, sandbox string, extraEnv []string, args ...string) (stdout, stderr []byte, err error) {
	ctx = backend.WithCallInfo(ctx, backend.CallInfo{Op: backend.OpGetConfig, Reason: backend.ReasonConfig})
	counter.Count(ctx, backend.OpGetConfig, false)
	cmd := safeexec.CommandContext(ctx, "git", args...)
	cmd.Dir = sandbox
	cmd.Env = discoveryEnv(os.Environ(), sandbox, extraEnv)
	var so, se bytes.Buffer
	cmd.Stdout, cmd.Stderr = &so, &se
	err = cmd.Run()
	return so.Bytes(), se.Bytes(), err
}

// NewGoGit builds the in-process backend with the system config learned from git. When
// discovery fails (no git, an old git without --show-origin) it logs and keeps the backend's
// conservative default, which sends any read that a candidate system file could answer to the CLI.
func NewGoGit(ctx context.Context) *gogit.Backend {
	files, err := DiscoverSystemConfig(ctx)
	return newGoGit(files, err)
}

func newGoGit(files SystemConfigFiles, discoveryErr error) *gogit.Backend {
	if discoveryErr != nil {
		log.Warn("git system config discovery failed; in-process config reads stay conservative", "err", discoveryErr)
		return gogit.New()
	}
	return gogit.New(gogit.WithSystemConfig(files.Default...), gogit.WithVendorSystemConfig(files.Vendor...))
}
