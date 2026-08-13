package services

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/git"
)

// driftCheckMainBranch is the branch this hook checks drift against. Matches the
// hardcoded "main" used elsewhere for the same purpose (session/stuck_decisions.go's
// bounceMainBranch, backlog_service_triage.go's prFixMainBranch) — this codebase does
// not yet support a configurable default branch.
const driftCheckMainBranch = "main"

// defaultDriftCheckMinInterval rate-limits how often HandlePostToolUseDriftCheck will
// actually run git.BehindOriginMain (a real `git fetch`, ~subprocess + network cost)
// for a given worktree. A session that commits in a tight burst (e.g. several small
// commits a few seconds apart while iterating) would otherwise re-fetch origin on
// every single commit; skipping re-checks inside this window trades a few minutes of
// staleness for avoiding fetch spam. Deliberately short relative to
// git.SteeringBranchDriftThreshold's timescale (drift accumulates over many commits
// across potentially hours) — this only smooths out same-minute commit bursts, not
// meaningful detection latency.
const defaultDriftCheckMinInterval = 3 * time.Minute

// postToolUseHookPayload is the subset of Claude Code's PostToolUse hook JSON input
// (https://code.claude.com/docs/en/hooks) this receiver cares about: session_id, cwd,
// tool_name, and tool_input. cwd doubles as the worktree path for a backlog work
// session (an autonomous session's cwd IS its dedicated git worktree — see
// spawnSessionAfterGates), so no separate storage lookup is needed to locate it.
type postToolUseHookPayload struct {
	SessionID string          `json:"session_id"`
	Cwd       string          `json:"cwd"`
	ToolName  string          `json:"tool_name"`
	ToolInput json.RawMessage `json:"tool_input"`
}

// bashToolInput is tool_input's shape when tool_name == "Bash" — the only tool this
// handler inspects further (git commit/push only ever happen via Bash in this
// environment; other tools' tool_input shapes are irrelevant here).
type bashToolInput struct {
	Command string `json:"command"`
}

// postToolUseHookResponse is the JSON response format Claude Code's PostToolUse hook
// protocol reads for context injection: hookSpecificOutput.additionalContext is
// wrapped in a system reminder and inserted into the agent's own context at the point
// the tool call completed — read on the model's next turn. Confirmed against Claude
// Code's official hooks documentation (code.claude.com/docs/en/hooks) and
// cross-checked against this repo's own scripts/ssq-hook-handler (which already
// parses `.tool_name` / `.tool_output.error` from a live PostToolUse payload).
// PostToolUse cannot block (the tool already ran) — additionalContext is the
// documented mechanism for feeding information back into the agent's context instead.
type postToolUseHookResponse struct {
	HookSpecificOutput hookSpecificOutputContext `json:"hookSpecificOutput"`
}

type hookSpecificOutputContext struct {
	HookEventName     string `json:"hookEventName"`
	AdditionalContext string `json:"additionalContext"`
}

// SetDriftCheckMinInterval overrides defaultDriftCheckMinInterval. Zero restores the
// default. Test-only; not called during normal server wiring.
func (h *HookReceiver) SetDriftCheckMinInterval(d time.Duration) {
	h.driftCheckMinInterval = d
}

// SetDriftThreshold overrides git.SteeringBranchDriftThreshold. Zero restores the
// default. Test-only; not called during normal server wiring.
func (h *HookReceiver) SetDriftThreshold(n int) {
	h.driftThreshold = n
}

// SetDriftCheckFn overrides the function used to compute how many commits a worktree
// is behind origin/mainBranch. Test-only — lets tests exercise
// HandlePostToolUseDriftCheck without a real git repo or network fetch.
func (h *HookReceiver) SetDriftCheckFn(fn func(worktreePath, mainBranch string) (int, error)) {
	h.driftCheckFn = fn
}

// HandlePostToolUseDriftCheck receives the Claude Code PostToolUse hook, wired only
// into autonomous backlog work sessions (see HookGitDriftCheck's doc comment and
// spawnSessionAfterGates — the sole call site that injects this hook). It is a
// steering hook, not a gate: it never blocks, never merges, never modifies the
// worktree. On every git commit/push it re-runs the same branch-drift detection
// BUG-044's review-gate precondition uses (git.BehindOriginMain) and, only once past
// git.SteeringBranchDriftThreshold commits behind main, feeds an explanatory nudge
// back into the calling agent's own context via additionalContext — so the agent
// notices and can proactively sync while it's still working, instead of only finding
// out from a review verdict hours or days later.
//
// Deliberately silent (no additionalContext) on every non-actionable path: not a
// Bash tool call, not a git commit/push command, under threshold, rate-limited, or a
// detection error (fails open, matching every other best-effort git check in this
// codebase — see git.EnsureBranchSyncedWithMain's identical fail-open rationale).
// Firing "you're fine" context on every ordinary commit would just be noise the agent
// has to read past; this only ever speaks up when there's something to act on.
func (h *HookReceiver) HandlePostToolUseDriftCheck(w http.ResponseWriter, r *http.Request) {
	sessionID := r.Header.Get(sessionIDHeader)

	body, err := io.ReadAll(io.LimitReader(r.Body, 64*1024))
	if err != nil {
		log.Warn("[hook/post-tool-use-drift-check] read body failed", "session", sessionID, "err", err)
		w.WriteHeader(http.StatusOK)
		return
	}

	var payload postToolUseHookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		// Malformed payload: fail open/silent. Never error out a hook call — Claude
		// Code treats a non-2xx or malformed response as a (non-blocking) hook error.
		w.WriteHeader(http.StatusOK)
		return
	}

	if payload.ToolName != "Bash" || payload.Cwd == "" {
		w.WriteHeader(http.StatusOK)
		return
	}

	var bashInput bashToolInput
	if err := json.Unmarshal(payload.ToolInput, &bashInput); err != nil || bashInput.Command == "" {
		w.WriteHeader(http.StatusOK)
		return
	}

	if !isGitCommitOrPushCommand(bashInput.Command) {
		w.WriteHeader(http.StatusOK)
		return
	}

	if !h.shouldRunDriftCheck(payload.Cwd) {
		w.WriteHeader(http.StatusOK)
		return
	}

	checkFn := h.driftCheckFn
	if checkFn == nil {
		checkFn = git.BehindOriginMain
	}
	behind, err := checkFn(payload.Cwd, driftCheckMainBranch)
	if err != nil {
		log.Warn("[hook/post-tool-use-drift-check] drift check failed — failing open",
			"session", sessionID, "cwd", payload.Cwd, "err", err)
		w.WriteHeader(http.StatusOK)
		return
	}

	threshold := h.driftThreshold
	if threshold <= 0 {
		threshold = git.SteeringBranchDriftThreshold
	}
	if behind < threshold {
		w.WriteHeader(http.StatusOK)
		return
	}

	log.Info("[hook/post-tool-use-drift-check] branch drift detected — injecting steering context",
		"session", sessionID, "cwd", payload.Cwd, "behind", behind, "threshold", threshold)
	writeDriftAdditionalContext(w, payload.Cwd, behind, threshold)
}

// shouldRunDriftCheck rate-limits BehindOriginMain fetches per worktree path. Returns
// true (and marks worktreePath as checked now) if enough time has passed since the
// last check; false if a check for this worktree ran within the interval.
func (h *HookReceiver) shouldRunDriftCheck(worktreePath string) bool {
	interval := h.driftCheckMinInterval
	if interval <= 0 {
		interval = defaultDriftCheckMinInterval
	}

	h.driftMu.Lock()
	defer h.driftMu.Unlock()
	if h.driftLastChecked == nil {
		h.driftLastChecked = make(map[string]time.Time)
	}
	now := time.Now()
	if last, ok := h.driftLastChecked[worktreePath]; ok && now.Sub(last) < interval {
		return false
	}
	h.driftLastChecked[worktreePath] = now
	return true
}

// writeDriftAdditionalContext writes the PostToolUse hookSpecificOutput.additionalContext
// response naming the drift, the threshold, and a concrete next step.
func writeDriftAdditionalContext(w http.ResponseWriter, worktreePath string, behind, threshold int) {
	branchDesc := "your current branch"
	if branch, err := git.GetCurrentBranchName(worktreePath); err == nil && branch != "" {
		branchDesc = fmt.Sprintf("your current branch (%s)", branch)
	}

	resp := postToolUseHookResponse{}
	resp.HookSpecificOutput.HookEventName = "PostToolUse"
	resp.HookSpecificOutput.AdditionalContext = fmt.Sprintf(
		"Branch drift check: %s is %d commits behind origin/%s (steering threshold: %d commits). "+
			"Unchecked drift is what causes review to block later (see BUG-044) — main sync becomes a hard "+
			"precondition of review once a branch falls %d+ commits behind, at which point your diff can end up "+
			"dominated by unrelated upstream changes instead of your own work. Catching it now, while you're "+
			"still working, is far cheaper: consider running `git fetch origin && git merge origin/%s` "+
			"(resolving any conflicts before continuing) so your diff stays focused on this item's actual "+
			"changes.",
		branchDesc, behind, driftCheckMainBranch, threshold, git.DefaultBranchDriftThreshold, driftCheckMainBranch,
	)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

// gitGlobalFlagsWithArg lists git global options that consume the following token as
// a separate argument (as opposed to "--flag=value" forms, which don't need special
// handling since they never split the subcommand token from a preceding flag).
var gitGlobalFlagsWithArg = map[string]bool{
	"-C":          true,
	"-c":          true,
	"--git-dir":   true,
	"--work-tree": true,
	"--namespace": true,
}

// isGitCommitOrPushCommand reports whether command (a Bash tool_input.command string)
// invokes `git commit` or `git push` as one of its shell segments — including chained
// invocations like `git add -A && git commit -m "x" && git push`.
//
// This is a heuristic, not a full shell parser: it tokenizes on whitespace after
// splitting on &&/||/;/| and a leading-newline (multi-line Bash tool calls), which
// correctly handles every git-commit/push invocation this codebase's own agents
// actually produce, but can under-match commands that set env vars inline before
// `git` (e.g. `GIT_AUTHOR_NAME=x git commit ...`) or invoke git through a wrapper
// script. Under-matching only means a missed steering nudge (fails toward silence,
// never toward a false "you're behind" alarm on an unrelated command), which is the
// safe direction for a steering hook that must never be disruptive noise.
func isGitCommitOrPushCommand(command string) bool {
	for _, segment := range splitShellSegments(command) {
		if segmentIsGitCommitOrPush(segment) {
			return true
		}
	}
	return false
}

// splitShellSegments splits a compound shell command into its individual pipeline/
// chain segments on &&, ||, ;, |, and newlines.
func splitShellSegments(command string) []string {
	replacer := strings.NewReplacer("&&", "\n", "||", "\n", ";", "\n", "|", "\n")
	return strings.Split(replacer.Replace(command), "\n")
}

// segmentIsGitCommitOrPush reports whether a single shell segment is a `git commit`
// or `git push` invocation: the first token must be "git", and the first subsequent
// non-flag token must be "commit" or "push" (any other subcommand — status, add,
// diff, log, etc. — returns false, since git only recognizes one subcommand per
// invocation).
func segmentIsGitCommitOrPush(segment string) bool {
	tokens := strings.Fields(segment)
	if len(tokens) == 0 || tokens[0] != "git" {
		return false
	}
	for i := 1; i < len(tokens); i++ {
		tok := tokens[i]
		if strings.HasPrefix(tok, "-") {
			if gitGlobalFlagsWithArg[tok] {
				i++ // skip the flag's argument token too
			}
			continue
		}
		return tok == "commit" || tok == "push"
	}
	return false
}
