package cli

import (
	"bytes"
	"os"
	"os/exec"
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
// else "git" (resolved by the runner as before).
var directGit = sync.OnceValue(func() string { // var so tests with fake runners can pin it
	return resolveDirectGit(exec.LookPath, os.Getenv("REAL_GIT"), []string{"/usr/bin/git", "/opt/homebrew/bin/git", "/usr/local/bin/git"})
})

func resolveDirectGit(lookPath func(string) (string, error), realGit string, candidates []string) string {
	p, err := lookPath("git")
	if err != nil || !isSSHFallbackWrapper(p) {
		return "git"
	}
	if realGit != "" {
		candidates = append([]string{realGit}, candidates...)
	}
	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && st.Mode().IsRegular() && st.Mode()&0o111 != 0 && !isSSHFallbackWrapper(c) {
			return c
		}
	}
	return "git"
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
