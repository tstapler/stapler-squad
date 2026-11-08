package gitwiring

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	"github.com/tstapler/stapler-squad/executor/safeexec"
)

// wrapperMarker identifies the dotfiles git-ssh-fallback shim, a shell script that runs several
// readlink/subshell spawns before exec'ing the real git. It only matters for network operations.
var wrapperMarker = []byte("git-ssh-fallback")

// wrapperNeededSubcommands are the ops the shim adds value for (HTTPS->SSH fallback, credentials).
var wrapperNeededSubcommands = map[string]bool{
	"clone": true, "fetch": true, "pull": true, "push": true, "ls-remote": true,
	"submodule": true, "remote": true, "credential": true, "lfs": true,
}

type resolvedGit struct{ path string }

var localGitCache sync.Map // PATH string -> resolvedGit; keyed by PATH so a changed PATH re-resolves

// localGitPath returns the real git to exec directly for a local-only git invocation when the first
// `git` on pathEnv is the git-ssh-fallback shim, or "" to keep normal PATH resolution (network or
// unknown subcommand, no shim, or no real git behind it). Each shim run costs ~8 process creations
// that an EDR inspects individually; a local read does not need it.
func localGitPath(pathEnv string, args []string) string {
	sub := safeexec.GitSubcommand(args)
	if sub == "none" || sub == "other" || wrapperNeededSubcommands[sub] {
		return ""
	}
	if v, ok := localGitCache.Load(pathEnv); ok {
		return v.(resolvedGit).path
	}
	p := findRealGitBehindShim(pathEnv)
	localGitCache.Store(pathEnv, resolvedGit{path: p})
	return p
}

func findRealGitBehindShim(pathEnv string) string {
	shimSeen := false
	for _, dir := range filepath.SplitList(pathEnv) {
		if dir == "" {
			continue
		}
		candidate := filepath.Join(dir, "git")
		info, err := os.Stat(candidate)
		if err != nil || info.IsDir() || info.Mode()&0o111 == 0 {
			continue
		}
		if !shimSeen {
			if !isSSHFallbackShim(candidate) {
				return "" // the first git is not the shim: PATH resolution already picks it
			}
			shimSeen = true
			continue
		}
		if !isSSHFallbackShim(candidate) {
			return candidate
		}
	}
	return ""
}

func isSSHFallbackShim(path string) bool {
	f, err := os.Open(path) // #nosec G304 -- path is a `git` entry on the process PATH
	if err != nil {
		return false
	}
	defer f.Close()
	head := make([]byte, 256)
	n, _ := f.Read(head)
	head = head[:n]
	return bytes.HasPrefix(head, []byte("#!")) && bytes.Contains(head, wrapperMarker)
}

// useLocalGit points cmd at the real git for local-only ops when `git` resolves to the shim.
// STAPLER_SQUAD_GIT_WRAPPER_BYPASS=0 disables it.
func useLocalGit(cmd *exec.Cmd, name string, args []string) {
	if name != "git" || os.Getenv("STAPLER_SQUAD_GIT_WRAPPER_BYPASS") == "0" {
		return
	}
	if p := localGitPath(os.Getenv("PATH"), args); p != "" {
		cmd.Path, cmd.Err = p, nil
	}
}
