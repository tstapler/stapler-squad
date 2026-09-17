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
// `--version` output against verifiedAgainstVersion
// (ScrollForwardCapability.VerifiedAgainstVersion -- the version Story
// 1.2.1's live spike confirmed the PageUp scroll-forward keybinding
// against), logging log.Error the FIRST time a mismatch is found for
// binaryPath. Skips entirely (via LoadOrStore) on every later call for the
// same path -- at most once per distinct binary path, never once per
// session or per scroll attempt.
//
// Deliberately NOT volume-gated like scrollForwardKeybindingCanary: a
// version mismatch is a strong enough signal (a confirmed input to the
// scroll-forwarding contract has changed) to warrant immediate visibility
// regardless of how many sessions/scroll attempts have hit it yet --
// closes pre-mortem P1 #1, that the canary's volume gate could otherwise
// let a low-traffic session's regression go undetected indefinitely.
//
// A run failure (binary not found, non-zero exit) logs log.Warn instead of
// log.Error and does not claim a mismatch -- mirrors
// checkControlModeVersionMatchOnce's treatment of its own client-version
// lookup failing (session/tmux/version_check.go): a lookup failure is a
// different, lower-severity condition than a confirmed mismatch, not the
// same thing reported the same way.
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
// --version`'s output, discarding everything after it. VERIFIED live in this
// environment (research/stack.md line 150): the real output is
// "2.1.270 (Claude Code)\n", not a bare version string -- an earlier
// TrimSpace-only implementation compared that whole string against
// verifiedAgainstVersion ("2.1.270") and reported a false mismatch on every
// real Claude Code install, caught by this package's own ForwardScroll tests
// actually shelling out to the real binary. Falls back to TrimSpace's result
// if no leading semver-shaped token is found, so an unexpected future output
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
// for i's resolved claude binary, if any (Story 1.5.3, Task 1.5.3b) -- called
// from finishInstanceConstruction (session/instance.go), the one choke-point
// every Instance construction site funnels through, so the check runs the
// moment a claude session is built, independent of whether its user ever
// scrolls (pre-mortem P1 #1). Also called from ForwardScroll as a fallback
// for instances built via bare struct literals in tests, which bypass
// finishInstanceConstruction -- harmless either way, since the check is
// memoized per binary path. Fires in a goroutine with a detached context (not
// ctx, which is scoped to a single caller's request and may already be
// canceled by the time the one-time `claude --version` shell-out completes)
// so it never adds latency to whichever call site triggered it.
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
