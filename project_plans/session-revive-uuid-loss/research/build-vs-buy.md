# Build vs. Buy: session-revive-uuid-loss

## Summary

**Build.** All four options were assessed; none of the buy/fork/library alternatives apply.
The change is a reordering of an existing, already-correct recovery call
(`tryExtractConversationUUID` → `DetectByPath`, `session/history_detector.go:137-199`) plus a
new status field/event — no new algorithm, no parsing risk, no external dependency
justified.

## 1. Existing OSS library (crash-recovery / resumable state-machine patterns)

Checked `go.mod` (`/home/tstapler/Programming/stapler-squad/go.mod`) for anything in this
space:

- No state-machine library (`fsm`, `statemachine`, etc.) — grep for `statemachine|fsm` over
  `go.mod`/`go.sum` returned nothing.
- Filesystem watching is already covered by `github.com/fsnotify/fsnotify v1.9.0`, used in
  `session/history_watcher.go` (`HistoryFileWatcher`) for *live* JSONL tailing — but this bug
  is not about watching, it's about a one-shot directory scan at decision time, which
  `DetectByPath` already does with `os.ReadDir` (`session/history_detector.go:146`). No
  streaming/watching primitive is missing.
- The actual "recovery" logic — decide resume-vs-fresh based on in-memory + on-disk state —
  is a two-branch conditional over already-parsed data (`HasClaudeSession`,
  `instance_claude.go:269`; `tryExtractConversationUUID`, `instance_claude.go:308`). This is
  not the shape of problem "crash-recovery/resumable session state machine" libraries solve
  (those target multi-step distributed workflows with checkpointing, e.g. Temporal-style
  durable execution) — it's a single boolean decision gated on a filesystem read that already
  exists in this codebase. Pulling in a workflow-engine dependency for a two-branch `if` would
  be a severe case of the "unjustified generic" smell this repo's own
  `.claude/rules/interface-pollution-checklist.md` flags.

**Verdict: not applicable.** Nothing in `go.mod` fills this niche, and nothing should — the
problem is too small and too specific to this repo's on-disk layout to be served by a general
library.

## 2. SaaS / managed API

Not applicable. stapler-squad is a local, self-hosted Go binary (`localhost:8543`) managing
tmux sessions and git worktrees on the user's own machine (see repo
`/home/tstapler/Programming/stapler-squad/CLAUDE.md`, "Architecture Overview"). The data being
recovered — `~/.claude/projects/<encoded-path>/*.jsonl` — is Claude Code's own local
conversation transcript format, written and read entirely on the local filesystem. There is no
network boundary, no multi-tenant concern, and no managed service that has visibility into a
user's local `~/.claude/projects/` directory. A SaaS dependency would also contradict the
project's default assumption of NAS/self-hosted deployment (Ansible-bootstrapped machines,
tmux+worktree isolation model).

**Verdict: not applicable**, confirmed by architecture.

## 3. LLM-generated bespoke code vs. battle-tested library for the actual algorithm

Broken into the two sub-problems the fix touches:

### 3a. Reordering the recovery call

This is pure control flow — move an existing call (`tryExtractConversationUUID`) earlier in
`startLocked` and its mirror (`session/instance.go` ~L867-921, ~L1067-1127), so it runs before
the `HasClaudeSession()` check instead of after. There is no algorithm here to "buy" — it's
sequencing two calls that already exist and are already tested
(`session/history_detector_test.go`, `session/instance_claude.go` callers).

### 3b. The recovery algorithm itself (already implemented, not part of this change)

`DetectByPath` (`session/history_detector.go:137-199`) is already in-house and does:
1. `os.ReadDir` on `~/.claude/projects/<encoded-path>/` (filename-only, no file content read)
2. Regex/suffix filtering (`.jsonl`, not `agent-*`, valid UUID basename via `isValidUUID`)
3. Pick the most-recently-modified candidate by `entry.Info().ModTime()`

No JSON parsing occurs in this path at all — it's a directory listing plus filename pattern
matching, not a JSONL content decode. Confirmed via `Read` of the full function body: the only
file I/O is `os.ReadDir` and `entry.Info()` (stat), never `os.Open`/`json.Decode` on the
`.jsonl` files themselves. Grep for `encoding/json` in `session/history_detector.go` returns
zero hits.

This means there is no "streaming JSON parsing" risk to weigh a library against for the
recovery step. (Elsewhere in the package — `session/history_watcher.go`,
`session/history_adapter.go` — JSONL *content* is parsed for live transcript tailing/linking,
but that is pre-existing, out of scope per the requirements' Non-goals section, and unaffected
by this fix.)

### 3c. The new piece: user-visible "lost & restarted fresh" signal

Per Acceptance Criteria 3, this needs a durable, user-visible signal distinguishing "resumed"
vs. "lost & restarted fresh." The codebase already has an established pattern for this class of
signal — session event/notification plumbing exists in `session/instance_terminal.go`,
`session/autonomous_driver.go`, `session/backlog_lifecycle.go`, `session/backlog_remediation.go`,
and `session/review_queue_poller.go` (grep for `SessionEvent|NotifyUser|notification` across
`session/*.go`). This is a status field/event addition following an existing in-repo
convention (also mirrored by the repo-wide instinct captured in user memory:
"self-heal/auto-close actions should post a visible comment + notify(), not act silently") —
not new algorithmic surface, so no build-vs-buy question applies here either.

**Verdict: build**, using the existing in-house detector and existing event/notification
conventions. Nothing here rises to the complexity threshold (e.g., untrusted-input parsing,
cryptography, distributed consensus) where a battle-tested library would measurably reduce
risk over straightforward Go.

## 4. Fork/adapt from Claude Code's own SDK/CLI

Checked whether Claude Code's own SDK/CLI is vendored anywhere in this repo and exposes a
"list resumable conversations" primitive that could replace the hand-rolled JSONL scan:

- `find . -iname "*claude-code*"` (excluding `node_modules`) returned only a planning doc
  (`project_plans/context-health-monitoring/decisions/ADR-001-claude-code-only-health-substrate.md`),
  no vendored source.
- No `@anthropic-ai/*` or Claude Code SDK dependency in `go.mod`/`go.sum`
  (grep for `claude.?code.?sdk|anthropic.*sdk|@anthropic-ai` returned nothing) or in
  `web-app/package.json`'s dependency tree (not separately re-checked here, but the backend fix
  is Go-only and the detection logic in question already lives entirely in `session/`).
- `stapler-squad` treats `~/.claude/projects/*.jsonl` purely as a well-known on-disk file
  layout it reverse-engineered (see `ClaudeProjectDirName`,
  `session/history_detector.go:118-129`, whose doc comment describes Claude's own path-encoding
  scheme) — there is no public Claude Code CLI subcommand or SDK call that enumerates
  resumable conversations by project path; `claude --resume <uuid>` requires the caller to
  already know the UUID, which is exactly the gap `DetectByPath` fills.

**Verdict: not applicable.** No SDK primitive exists to fork/adapt; the existing hand-rolled
scan is the only implementation of this capability, in or out of this codebase.

## Conclusion

Build, using code that (for the recovery/detection algorithm) already exists in
`session/history_detector.go`. The requirements-scoped change is: (1) call-ordering in
`session/instance.go`'s two cold-restore branches, (2) a new status/event field surfaced
through the existing session-event/notification convention. Neither warrants a new dependency.
