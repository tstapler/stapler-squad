package session

import (
	"context"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/tstapler/stapler-squad/executor/safeexec"
	"github.com/tstapler/stapler-squad/log"
)

// CommandRunner abstracts *exec.Cmd.Output for
// scrollForwardVersionMismatchCheck's `<binaryPath> --version` shell-out,
// mirroring session/tmux.TmuxSession.cmdExec's existing seam
// (session/tmux/version_check.go) so tests can fake the shell-out instead of
// requiring a real claude binary on PATH.
type CommandRunner interface {
	Output(cmd *exec.Cmd) ([]byte, error)
}

// execCommandRunner is the production CommandRunner: a thin passthrough to
// (*exec.Cmd).Output.
type execCommandRunner struct{}

func (execCommandRunner) Output(cmd *exec.Cmd) ([]byte, error) {
	return cmd.Output()
}

// claudeVersionCheckedPaths memoizes which claude binary paths have already
// been checked (LoadOrStore key: binaryPath), mirroring
// session/tmux/version_check.go's versionCheckedSockets: the answer cannot
// change without a new binary being installed at that same path, so
// re-checking on every session launched against it would be pure waste
// (Story 1.5.3, pre-mortem P1 #1).
var claudeVersionCheckedPaths sync.Map

// scrollForwardVersionMismatchCheck compares binaryPath's actual
// `--version` against verifiedAgainstVersion (the version scroll-forwarding's
// PageUp keybinding was verified against), logging log.Error the first time a
// mismatch is found for binaryPath and never again (LoadOrStore memoizes per
// path). Deliberately not volume-gated like scrollForwardKeybindingCanary: a
// version mismatch means a confirmed input to the scroll-forwarding contract
// changed, which warrants immediate visibility even from a low-traffic
// session. A run failure (binary not found, non-zero exit) logs log.Warn
// instead and claims no mismatch -- a lookup failure is lower-severity than a
// confirmed one, not the same thing.
func scrollForwardVersionMismatchCheck(ctx context.Context, binaryPath, verifiedAgainstVersion string, runner CommandRunner) {
	if binaryPath == "" {
		return
	}
	if _, already := claudeVersionCheckedPaths.LoadOrStore(binaryPath, struct{}{}); already {
		return
	}
	if runner == nil {
		runner = execCommandRunner{}
	}

	cmd := safeexec.CommandContext(ctx, binaryPath, "--version")
	out, err := runner.Output(cmd)
	if err != nil {
		log.Warn("scroll_forward: failed to get claude --version, skipping version-mismatch check", "binary_path", binaryPath, "err", err)
		return
	}

	actualVersion := normalizeClaudeVersion(string(out))
	if actualVersion == "" || actualVersion == verifiedAgainstVersion {
		return
	}

	log.Error("scroll_forward: claude binary version no longer matches the version scroll-forwarding's keybinding was verified against -- forwarding may silently regress to a no-op",
		"binary_path", binaryPath,
		"expected_version", verifiedAgainstVersion,
		"actual_version", actualVersion)
}

// claudeVersionNumberRegex extracts the leading semver-shaped token from
// `claude --version`'s output.
var claudeVersionNumberRegex = regexp.MustCompile(`^\d+\.\d+\.\d+`)

// normalizeClaudeVersion extracts the leading version number from `claude
// --version`'s output ("2.1.270 (Claude Code)\n", not a bare version string,
// per research/stack.md), discarding the rest. Falls back to TrimSpace's
// result if no leading semver-shaped token is found, so an unexpected future
// format degrades to "probably still a mismatch" rather than silently always
// matching.
func normalizeClaudeVersion(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if m := claudeVersionNumberRegex.FindString(trimmed); m != "" {
		return m
	}
	return trimmed
}

// kickOffClaudeVersionMismatchCheck triggers scrollForwardVersionMismatchCheck
// for i's resolved claude binary, if any. Called from
// finishInstanceConstruction so the check runs independent of whether the
// user ever scrolls, and again from ForwardScroll as a fallback for
// test-only bare struct literals that bypass that constructor -- harmless
// either way since the check is memoized per binary path. Runs in a
// goroutine with a detached context (not ctx, which may already be canceled
// by the time the one-time shell-out completes) so it never adds latency to
// its caller.
func (i *Instance) kickOffClaudeVersionMismatchCheck() {
	binaryPath := claudeBinaryPathFromProgram(i.GetProgram())
	if binaryPath == "" {
		return
	}
	verifiedVersion := NewClaudeScrollAdapter().Capability().VerifiedAgainstVersion
	go func() {
		// Panic safety: this always runs as a detached goroutine, so an
		// unrecovered panic here would crash the entire server process --
		// mirrors this package's other fire-and-forget goroutines (e.g.
		// AutonomousDriver.run in session/autonomous_driver.go).
		defer func() {
			if r := recover(); r != nil {
				log.Error("scroll_forward: claude version-mismatch check panicked (recovered)", "binary_path", binaryPath, "panic", r)
			}
		}()
		scrollForwardVersionMismatchCheck(context.Background(), binaryPath, verifiedVersion, nil)
	}()
}

// claudeBinaryPathFromProgram extracts the whitespace-delimited token that
// invokes the claude binary from program, mirroring isClaude's own
// token-basename matching (session/instance_tmux.go) so
// scrollForwardVersionMismatchCheck checks the exact token this session
// launches -- a bare "claude" (resolved via PATH when actually run) or a
// path-qualified invocation. Returns "" if no token's basename is "claude".
func claudeBinaryPathFromProgram(program string) string {
	for _, token := range strings.Fields(program) {
		if filepath.Base(token) == "claude" {
			return token
		}
	}
	return ""
}
