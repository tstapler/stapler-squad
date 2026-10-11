package safeexec

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/tstapler/stapler-squad/telemetry"
)

// MetricGitSpawnBackstop counts every local `git` process built through this package, by
// subcommand only. It carries no call-site label: it is the backstop behind
// git_backend_cli_spawn_total, so a spawn that bypasses the Backend seam raises this counter
// without raising the backend one. Gate invariant: sum(backstop) <= sum(backend counter,
// reason != remote_host).
const MetricGitSpawnBackstop = "git_cli_spawn_backstop_total"

// SpawnDumpEnv names the test-only file that receives one line per git spawn with the
// caller's file:line. The same variable makes the Backend write its own attributed lines
// (session/git/backend.SpawnDumpEnv).
const SpawnDumpEnv = "SSQ_GIT_SPAWN_DUMP"

// gitSpawnBackstop is built against the global meter so it is safe before telemetry.Initialize.
var gitSpawnBackstop = newBackstopCounter(telemetry.GetMeter())

func newBackstopCounter(meter metric.Meter) metric.Int64Counter {
	counter, err := meter.Int64Counter(MetricGitSpawnBackstop,
		metric.WithDescription("Local git processes started through safeexec, by subcommand; a bypass detector"))
	if err != nil {
		// Only a malformed name or config, a build-time constant here (see mustInt64Counter).
		panic(err)
	}
	return counter
}

func isGit(name string) bool { return name == "git" || filepath.Base(name) == "git" }

// recordGitSpawn counts one git process and, when SpawnDumpEnv is set, logs its caller.
func recordGitSpawn(ctx context.Context, name string, args []string) {
	if !isGit(name) {
		return
	}
	sub := GitSubcommand(args)
	gitSpawnBackstop.Add(ctx, 1, metric.WithAttributes(attribute.String("subcommand", sub)))
	if path := os.Getenv(SpawnDumpEnv); path != "" {
		dumpExec(path, sub)
	}
}

func dumpExec(path, sub string) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600) // #nosec G304 -- test-only diagnostic path chosen by the developer's environment
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = fmt.Fprintf(f, "exec %s subcommand=%s\n", callerOutsideSpawnPath(), sub)
}

// callerOutsideSpawnPath is the first frame outside safeexec, the tmux runner and the git
// backend: whoever started the chain, not the sanctioned helper every spawn funnels through.
func callerOutsideSpawnPath() string {
	pcs := make([]uintptr, 32)
	n := runtime.Callers(3, pcs)
	frames := runtime.CallersFrames(pcs[:n])
	for {
		fr, more := frames.Next()
		if !isSpawnPlumbing(fr.Function) {
			return fmt.Sprintf("%s:%d", fr.File, fr.Line)
		}
		if !more {
			return "unknown:0"
		}
	}
}

func isSpawnPlumbing(fn string) bool {
	if strings.Contains(fn, ".Test") {
		return false
	}
	for _, p := range []string{
		"github.com/tstapler/stapler-squad/executor/safeexec.",
		"github.com/tstapler/stapler-squad/session/tmux.LocalRunner",
		"github.com/tstapler/stapler-squad/session/git/backend",
	} {
		if strings.HasPrefix(fn, p) {
			return true
		}
	}
	return false
}

// gitOptionsWithArg are git's global options that consume the next argument when not written
// as --opt=value.
var gitOptionsWithArg = map[string]bool{
	"-C": true, "-c": true, "--git-dir": true, "--work-tree": true, "--namespace": true,
	"--exec-path": true, "--super-prefix": true, "--config-env": true, "--attr-source": true,
}

// gitSubcommands is the closed label set for the backstop counter. Anything else is "other",
// so an arbitrary argument can never become a label value.
var gitSubcommands = map[string]bool{
	"add": true, "apply": true, "archive": true, "bisect": true, "blame": true, "branch": true,
	"cat-file": true, "check-ignore": true, "checkout": true, "cherry-pick": true, "clean": true,
	"clone": true, "commit": true, "config": true, "count-objects": true, "describe": true,
	"diff": true, "diff-files": true, "diff-index": true, "diff-tree": true, "fetch": true,
	"for-each-ref": true, "format-patch": true, "fsck": true, "gc": true, "grep": true,
	"hash-object": true, "init": true, "log": true, "ls-files": true, "ls-remote": true,
	"ls-tree": true, "merge": true, "merge-base": true, "merge-file": true, "merge-tree": true,
	"mv": true, "pull": true, "push": true, "read-tree": true, "rebase": true, "reflog": true,
	"remote": true, "repack": true, "reset": true, "restore": true, "rev-list": true,
	"rev-parse": true, "revert": true, "rm": true, "show": true, "show-ref": true,
	"sparse-checkout": true, "stash": true, "status": true, "submodule": true, "switch": true,
	"symbolic-ref": true, "tag": true, "unpack-file": true, "update-index": true, "update-ref": true,
	"version": true, "worktree": true, "write-tree": true,
}

// GitSubcommand returns the git subcommand named by args, skipping global options (-C <path>,
// -c k=v, --git-dir[=]<p>, flags such as --no-pager). It returns "none" when there is no
// subcommand and "other" for one outside the closed list.
func GitSubcommand(args []string) string {
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case gitOptionsWithArg[a]:
			i++ // the option's value
		case strings.HasPrefix(a, "-"):
			// --opt=value or a bare flag: nothing further to skip
		case gitSubcommands[a]:
			return a
		default:
			return "other"
		}
	}
	return "none"
}
