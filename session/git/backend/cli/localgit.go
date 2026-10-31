package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"sync"
)

// The developer-machine ~/.local/bin/git is a ~450-line POSIX-sh wrapper
// (git-ssh-fallback) that shadows /usr/bin/git via PATH. It only changes behavior
// for network subcommands (HTTPS->SSH fallback) but runs first for every spawn:
// `git rev-parse HEAD` measured 426 ms median / 1.8 s p95 through it versus 38 ms
// for /usr/bin/git on the same host under load. Local subcommands therefore run the
// real binary directly; network ones keep going through PATH so the fallback applies.

// wrapperNetworkOps mirrors the subcommands the wrapper intercepts.
var wrapperNetworkOps = map[string]bool{
	"clone": true, "fetch": true, "pull": true, "push": true, "ls-remote": true, "submodule": true, "remote": true,
}

// gitSubcommand returns the first non-flag token, skipping flags that take a value.
func gitSubcommand(args []string) string {
	skip := false
	for _, a := range args {
		if skip {
			skip = false
			continue
		}
		switch {
		case a == "-C" || a == "-c" || a == "--git-dir" || a == "--work-tree" || a == "--namespace" || a == "--super-prefix" || a == "--config-env":
			skip = true
		case len(a) > 0 && a[0] == '-':
		default:
			return a
		}
	}
	return ""
}

// localGitName is the binary for a local (non-remote) run of git args.
func localGitName(args []string) string {
	if wrapperNetworkOps[gitSubcommand(args)] {
		return "git"
	}
	return directGit()
}

// directGit is the real git binary when PATH's git is the ssh-fallback wrapper,
// else "git" (resolved by the runner as before). Memoized per (PATH, REAL_GIT) so a
// process that changes PATH (tests installing a fake git) is not served a stale binary.
var directGit = func() string { // var so tests with fake runners can pin it
	key := [2]string{os.Getenv("PATH"), os.Getenv("REAL_GIT")}
	directGitMu.Lock()
	defer directGitMu.Unlock()
	if v, ok := directGitCache[key]; ok {
		return v
	}
	v := resolveDirectGit(key[0], key[1])
	directGitCache[key] = v
	return v
}

var (
	directGitMu    sync.Mutex
	directGitCache = map[[2]string]string{}
)

// resolveDirectGit mirrors the wrapper's own _find_real_git: the first executable
// `git` on PATH that is not the wrapper (REAL_GIT wins when set). Only binaries named
// "git" qualify, because the runner injects RequiredGitEnv and counts spawns by
// basename. Returns "git" when PATH's git is not the wrapper or nothing else is found.
func resolveDirectGit(pathEnv, realGit string) string {
	first := ""
	for _, dir := range filepath.SplitList(pathEnv) {
		if dir == "" {
			continue
		}
		if c := filepath.Join(dir, "git"); isExecutableFile(c) {
			first = c
			break
		}
	}
	if first == "" || !isSSHFallbackWrapper(first) {
		return "git"
	}
	if realGit != "" && filepath.Base(realGit) == "git" && isExecutableFile(realGit) && !isSSHFallbackWrapper(realGit) {
		return realGit
	}
	for _, dir := range filepath.SplitList(pathEnv) {
		if dir == "" {
			continue
		}
		if c := filepath.Join(dir, "git"); isExecutableFile(c) && !isSSHFallbackWrapper(c) {
			return c
		}
	}
	return "git"
}

func isExecutableFile(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.Mode().IsRegular() && st.Mode()&0o111 != 0
}

func isSSHFallbackWrapper(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	head := make([]byte, 512)
	n, _ := f.Read(head)
	head = head[:n]
	return bytes.HasPrefix(head, []byte("#!")) && bytes.Contains(head, []byte("git-ssh-fallback"))
}
