# Requirements: session-revive-uuid-loss

## Source

Backlog item `ab4293a9-4a84-4207-886f-3a4ef48bbd20`: "Session silently restarts as a
fresh Claude (context lost) when revived without a captured conversation UUID."

## Problem

`startLocked` and its restart-path mirror in `session/instance.go` (~L867-921 and
~L1067-1127) decide whether to cold-restore with `--resume <uuid>` or start a brand-new
`claude` process based solely on `i.HasClaudeSession()`
(`session/instance_claude.go:269` — `claudeSession != nil && ConversationUUID != ""`).

Confirmed in code: `tryExtractConversationUUID()` (`session/instance_claude.go:308`),
which has a path-based fallback (`detector.DetectByPath`) that can recover the UUID by
scanning `~/.claude/projects/<encoded-path>/` for JSONL files even when the tmux pane is
dead, is only ever called **after** the fresh-vs-resume branch has already been chosen
(`instance.go:921`, `instance.go:1127`) — never before it. So if the in-memory UUID was
never captured or was cleared (e.g. repeated restarts by the inactivity watchdog
happening faster than UUID capture, or `ClearConversationState()` at
`instance_claude.go:278`), the branch takes the "start fresh" path even when a resumable
JSONL transcript exists on disk right now. The user gets a brand-new Claude with no
memory of the prior conversation, signaled only by a single `log.Warn` line.

## Goals

1. Before deciding to start a session fresh (no `--resume`), attempt UUID recovery via
   the existing path-based detector (`DetectByPath`) using the session's effective root
   dir, so a UUID that exists on disk but was never captured in memory is not discarded.
2. If recovery still finds nothing but the session previously had *some* recorded
   conversation state (a non-empty `HistoryFilePath` was ever stored, or a JSONL project
   dir exists for this path with no matching UUID), do not silently proceed as a normal
   fresh start — surface this distinctly (see Goal 3) rather than treating it identically
   to a legitimate brand-new session.
3. Make the cold-start-fresh outcome visible to the user, not just a backend log line —
   at minimum a session event/notification the UI can render (e.g. "started a new
   session; the previous conversation could not be resumed"), consistent with this
   repo's existing `feedback_document_ai_decisions_in_edge_cases` convention that
   self-heal/fallback actions must be visible, not silent.
4. Do not regress legitimate fresh-start cases (first-time setup, one-off sessions with
   no prior conversation, user-initiated "start over") — the louder signal and recovery
   attempt must only fire for the *unexpected* fresh-start case (session had history,
   revive lost track of it).

## Non-goals

- Fixing the upstream restart churn itself (inactivity watchdog cadence, hardcoded
  `driverInactivityTimeout` in `session/session_driver.go:46`). Noted in the source item
  as "related" and worth a separate look, but out of scope here — this item is about not
  losing context on revive, not about reducing how often revive happens.
- Persisting the UUID to a new durable store/schema. `SetHistoryInfo`
  (`instance_claude.go:464`) already fires a save callback on UUID change; if research
  finds the existing persistence path has its own bug (save not reaching disk before a
  crash), that's a candidate finding but the requirement here is decision-time recovery
  and visibility, not a persistence-layer rewrite.
- Changing `RunWithResume`'s one-off `-p --resume` subprocess flow — unaffected by this
  bug (it already errors cleanly if no UUID is set).

## Status update (2026-08-21 re-triage)

This exact backlog item (`ab4293a9-4a84-4207-886f-3a4ef48bbd20`) already has a full
requirements/research/plan artifact set in this directory from an earlier planning pass.
Since then, a **separate** backlog item (`f5c5a35e-b5f1-491d-81de-66e94028a085`, same
underlying bug) shipped `fix(session): recover conversation UUID from disk before
cold-restore launch decision` (#439, commit `e156a3f9d`, 2026-08-11) — **after** this
directory's `plan.md` (dated 2026-08-06) was written. Re-reading the current code
confirms the core problem this requirements doc describes is already fixed:

- `recoverConversationBeforeLaunch()` (`session/instance.go:939-944`) now runs
  `tryExtractConversationUUID()`'s `DetectByPath` fallback **before** the
  resume-vs-fresh decision, at both cold-restore call sites (`startLocked` line 970,
  `start` line 1166) — this satisfies **Goal 1 / AC1**, **AC4** (single shared call,
  no divergent duplication), and **AC2** (no-op when nothing to recover).
- `conversationClearedAt` (`session/instance_claude.go:293,354-359`) guards recovery
  against resurrecting a conversation the user explicitly cleared — this is the
  `RecoverySuppressed` mechanism `plan.md`'s glossary proposed as new work; it already
  exists under a different name. **Goal 4 / AC6** satisfied.
- Regression tests already exist for this path (`TestColdRestore_WithoutUUID_
  RecoversFromJSONL`, `TestKillSessionThenStart_DoesNotRebuildLaunchCommand`,
  `TestTryExtractConversationUUID_ClearedAtGuard`) — **AC5** satisfied for the
  recovery-ordering behavior.

**Not yet implemented** — confirmed absent via `grep -rn "ConversationLost\|ReviveOutcome\|
EverHadConversationHistory\|ResumeFailed" session/ server/ web-app/src`:

- **Goal 3 / AC3** — a durable, user-visible signal distinguishing "resumed" from "lost
  history, started fresh anyway." Today, when recovery still finds nothing, the session
  just starts fresh with only a `log.Warn` line (`session/instance.go:1003`) — exactly
  the silent-data-loss complaint the original bug report was about, just now triggered
  only in the genuinely-unrecoverable case (no JSONL on disk at all) rather than on
  every UUID-capture race.

**Effective remaining scope of this item is AC3 only.** `plan.md`'s proposed
`prepareColdRestore`/`RecoverySuppressed`/shared-helper refactor (Pattern Decisions rows
1-3) is superseded by the already-shipped `recoverConversationBeforeLaunch`/
`conversationClearedAt` — do not re-implement it. The `ReviveOutcome` enum,
`EverHadConversationHistory` bool, `onColdRestoreLostHistory` notification, and
`RevivedContextBadge` frontend component (Pattern Decisions rows 4-7) are still valid
and are the actual remaining work.

## Acceptance Criteria

Status markers added 2026-08-21 — see "Status update" above for evidence.

1. **[SHIPPED — #439]** Given a session whose tmux pane is dead and whose in-memory
   `ConversationUUID` is empty, but a JSONL transcript exists under
   `~/.claude/projects/<encoded-effective-root>/` for that session's path, revive resumes
   using the UUID recovered from that JSONL (via the existing `DetectByPath` fallback,
   invoked before the resume/fresh decision) instead of starting a fresh Claude process.
2. **[SHIPPED — #439]** Given the same dead-tmux/no-UUID case where recovery genuinely
   finds nothing (no JSONL for this path — a real brand-new session, or first-time
   setup), the session starts fresh exactly as it does today, with no change in behavior
   or added latency that matters.
3. **[REMAINING — this item's actual scope]** When a session is forced to start fresh in a case where it previously had a captured
   UUID or `HistoryFilePath` (recovery attempted and still failed), a durable,
   user-visible signal is recorded (not only a log line) — e.g. a session event/status
   field the frontend can surface — distinguishing "resumed" from "lost & restarted
   fresh."
4. **[SHIPPED — #439]** The fix applies symmetrically to both cold-restore call sites
   (`session/instance.go` `startLocked` line 970, `start` line 1166, via the shared
   `recoverConversationBeforeLaunch` helper) — no duplicated divergent logic between
   the two.
5. **[PARTIAL]** Existing tests for `HasClaudeSession`, `tryExtractConversationUUID`, and
   the cold-restore paths continue to pass, and recovery-ordering regression tests
   already exist (shipped in #439). **Remaining**: a new test covering the
   fresh-with-prior-history visibility signal (AC3).
6. **[SHIPPED — #439]** No change to legitimate first-time-setup or explicit "start
   fresh" flows (`conversationClearedAt` guard).
