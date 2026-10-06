# Requirements: cold-start-uuid-loss

**Date**: 2026-08-06
**Type**: bug fix
**Complexity**: 2 — focused feature (touches actor-serialized session lifecycle code + needs a UI-visible signal, not a one-line fix)

## Problem Statement
When a session's tmux pane is dead and gets revived (`startLocked`/`start` in `session/instance.go:845-935` and `:1023-1150`), the revive path decides whether to `--resume` the existing Claude conversation or start a brand-new one purely from `i.HasClaudeSession()` — i.e., whether `i.claudeSession.ConversationUUID` is non-empty **in memory at that instant** (`session/instance_claude.go:269-273`). If the UUID is empty — because it was cleared (`ClearConversationState`, `session/instance_claude.go:278`) or was never (re)captured after a prior restart — the code silently starts a fresh `claude` process with no `--resume`, discarding the working conversation. The only signal is a single `log.Warn("cold start: tmux dead, no conversation UUID, starting fresh", ...)` line; nothing surfaces to the user or blocks the revive.

Critically, the code already has the fix mechanism in hand and doesn't use it in time: `tryExtractConversationUUID()` (`session/instance_claude.go:308-363`) has a **path-based fallback** (`detector.DetectByPath(effectivePath)`) that scans `~/.claude/projects/<encoded-path>/` for the newest JSONL and recovers the UUID without needing a live process — but the current code only calls `tryExtractConversationUUID()` *after* it has already committed to the fresh-start branch (`session/instance.go:921`, called after `i.pm().Start(startPath)` with no `--resume`). The `HasClaudeSession()` gate at line 881 (and its mirror at line 1072) runs before any attempt at recovery.

## Baseline
Today: a session that gets restarted several times in a short window (inactivity-timeout restarts, a service restart, hibernation) can lose its in-memory UUID before it's recaptured, then revive as a brand-new Claude with no memory of the prior conversation — observed on v1.40.0, macOS, one-off session type. The user only discovers this by noticing the agent "forgot everything" and has to reconstruct context from git/files. The same session type resumed correctly on a later revive in the reported log trace, confirming the failure is racy/inconsistent, not a permanent inability to resume.

## Users / Consumers
- Any Claude Code session managed by stapler-squad that goes through a cold restart (tmux dead: reboot, `tmux kill-server`, service restart, hibernation-then-revive) — all session types (`SessionTypeDirectory`, worktree-backed, one-off) share this code path.
- The user (Tyler / solo operator) — silently loses conversation continuity and has to notice + reconstruct state by hand.
- `session/session_driver.go`'s restart machinery (`driverInactivityTimeout = 10 * time.Minute`, `session_driver.go:46,519`) — the reported trigger sequence for the racy window, called out as a related but separately-scoped issue.

## Success Metrics
- **No silent data loss when recoverable**: if a JSONL conversation history exists on disk for a session's path at revive time but the in-memory UUID is empty, the revive path attempts `DetectByPath`-style recovery **before** deciding to start fresh — not after. Target: 0 fresh-starts in cases where a resumable JSONL was present, verified by unit test around `startLocked`'s branch logic.
- **No silent fresh-start when unrecoverable**: if recovery genuinely fails (no JSONL, or a JSONL exists but can't be attributed with confidence), the fresh-start still happens (never block revive indefinitely) but the resulting session state/event stream carries an explicit, durable "started fresh — could not resume prior conversation" marker, not just a log line. Target: the marker is inspectable via the existing session events/status API, not only `journalctl`/log grep.
- **Recapture reliability**: the racy window where a live UUID capture hasn't happened yet before the next restart is narrowed — either by persisting/reloading the UUID earlier in the restart lifecycle, or by making the path-based fallback the first check rather than a last resort.

## Appetite
Small (1–2 days) — this is a bug fix confined to the revive branch in `session/instance.go` (two near-identical blocks) plus `session/instance_claude.go`'s UUID helpers; it does not require new proto fields, new UI surfaces beyond an existing notification/event mechanism, or schema migrations.

## Constraints
None hard — no deadline, no compliance surface, solo-maintainer project. Must not regress the existing `--resume` happy path (the majority case where a UUID is already known).

## Non-functional Requirements
- **Performance SLO**: `DetectByPath` scans a directory for JSONL files — moving it earlier (before every cold-start decision, not just after committing to fresh) must not meaningfully slow down revives; it's already used unconditionally today after fresh-start, so cost is not new, only its position in the flow changes.
- **Scalability**: not applicable (single-operator, one revive at a time per session).
- **Security classification**: internal — reads local `~/.claude/projects/` JSONL files already trusted by the existing detector.
- **Data residency**: no special requirements.

## Scope
### In Scope
- Reordering/extending the cold-start decision in `startLocked` (`session/instance.go:878-921`) and its duplicate in `start` (`session/instance.go:1068-1127`, if not already unified — see Rabbit Holes) so a path-based UUID recovery attempt happens **before** the fresh-vs-resume branch decision, not after.
- Deduplicating the two near-identical cold-restore code blocks if that can be done within the Small appetite without expanding blast radius (see Rabbit Holes — may be deferred).
- A durable, user-visible signal (not just a log line) when a session actually did start fresh despite having (or possibly having had) a prior conversation — e.g. a session event/status field the existing UI can render, per the item's suggested direction #3.
- Unit test coverage for: (a) UUID present → resume; (b) UUID empty, JSONL recoverable via path fallback → resume; (c) UUID empty, no JSONL → fresh start with visible marker.

### Out of Scope
- Changing `driverInactivityTimeout` or the restart-churn behavior in `session/session_driver.go` that precedes this bug — flagged by the item as "worth a look too" but explicitly a separate concern with its own hardcoded-config question.
- Persisting the UUID to durable storage (DB/ent) earlier in the lifecycle if the path-based recovery alone closes the observed gap — only pursue if research/planning shows the path fallback can't reliably cover the racy window (e.g., multiple concurrent JSONL files, ambiguous newest-file selection).
- Building a general "session history" UI feature — only the minimal marker/notification needed to make a fresh-start visible.

## Rabbit Holes
- **Duplicate cold-restore logic**: `session/instance.go` has two nearly-identical blocks implementing the same "tmux dead → resume or fresh" decision (`startLocked` at ~845-935, `start` at ~1023-1150+). Unclear yet whether both are live call paths or one is legacy/being migrated to the actor model (`startLocked` looks like the newer actor-safe version per its doc comment). Phase 3 planning must confirm which is authoritative before touching both, to avoid fixing one and leaving the other's copy of the same bug.
- **Ambiguous JSONL selection**: `DetectByPath` picks "the newest JSONL" for a project path — if a path has been reused across genuinely different conversations (e.g., a worktree recycled for a new session), blindly resuming the newest file could resume the *wrong* conversation instead of losing one. Needs explicit handling, not just "found a file, use it."
- **When recovery is attempted but wrong**: moving `DetectByPath` earlier changes it from "best-effort enrichment after the fact" to "load-bearing for the resume-vs-fresh decision" — its false-positive/false-negative behavior needs more scrutiny than it needed before.

## Alternatives Considered
- **Persist the UUID to the DB/ent store on every capture, reload it first on revive** (item's suggested direction #2) — more durable than relying on filesystem re-detection every time, but larger surface (schema/storage plumbing) for a Small-appetite fix; keep as a fallback design if Phase 3 finds the path-detector approach insufficient.
- **Just make the log line louder (toast/notification) without attempting recovery** (item's suggested direction #3 alone) — insufficient on its own; it tells the user data was lost after the fact instead of preventing the avoidable case where recovery was possible.

## Feasibility Risks
- The two duplicate code blocks in `session/instance.go` increase the risk of an incomplete fix (patching one path, missing the other).
- `DetectByPath`'s reliability under concurrent/ambiguous JSONL files is unverified — needs research into `HistoryFileDetector`'s matching logic before trusting it as a pre-decision gate rather than a post-hoc enrichment.

## Observability Requirements
Not required at complexity 2, but low-cost and directly requested by the item: log a distinguishable structured event for each of the three outcomes (resumed via known UUID / resumed via recovered UUID / started fresh) so future tuning of the racy window is possible without re-instrumenting.

## Risk Control
Not needed — this is a bug fix to existing, tested lifecycle code; standard revert via PR close/revert commit is sufficient. No feature flag needed given Small appetite and no schema changes.

## Open Questions
- Which of the two cold-restore blocks (`startLocked` vs `start`) is the live/authoritative path today, and is the other dead code or still reachable? Needs codebase research, not user input.
- Is there an existing session-event/notification mechanism the "started fresh" marker should hook into (e.g., the pattern used by `feedback_document_ai_decisions_in_edge_cases` — self-heal/auto-close actions posting a visible comment + notify()), or does one need to be added?
