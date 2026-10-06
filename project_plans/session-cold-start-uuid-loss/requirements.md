# Requirements: Session Cold-Start UUID Loss

Complexity: 3 (system design — touches session lifecycle, persistence, and needs a UX signal)

## Problem

A session whose tmux pane has died can be revived as a **brand-new Claude conversation**
instead of resuming the prior one, silently discarding in-progress context. This happens
in `session/instance.go` (two call sites: `startLocked` around line 878-885, and `start`
around line 1068-1080), which branch on `i.HasClaudeSession()`:

```go
if !firstTimeSetup {
    if !i.pm().IsAlive() {
        startPath := i.resolveStartPath(i.GetEffectiveRootDir())
        if i.HasClaudeSession() {
            log.Info("cold restoring with --resume", "session", i.Title, "uuid", i.claudeSession.ConversationUUID, "path", startPath)
        } else {
            log.Warn("cold start: tmux dead, no conversation UUID, starting fresh", "session", i.Title, "path", startPath)
        }
```

If the in-memory `ConversationUUID` is empty when a revive happens — which the reporter
observed occurs after rapid restart churn (inactivity-timeout restarts, a service
restart, then hibernation, all within roughly 2 hours) — the code starts `claude` fresh
with no `--resume` flag, and the previous conversation's context is gone. The only trace
is a single `WARN` log line; nothing in the UI signals that context was lost.

Observed on v1.40.0, macOS, one-off session type (`~/oneoff/...`). The same session
cold-started fresh at one restart and then correctly resumed via `--resume` at a later
restart — so UUID capture is racy/inconsistent across back-to-back restarts, not
permanently missing.

## Impact

High when triggered: the active conversation is silently discarded and replaced with a
fresh agent with no memory of prior work. Users may not notice until the agent
demonstrably "forgot" everything, at which point recovery requires manually
reconstructing state from git diffs/files.

## Root-cause hypothesis (to confirm in research)

The `ConversationUUID` is populated by scraping the live tmux pane/JSONL after the
session starts (not persisted durably before that capture completes). Restarting the
session repeatedly in a short window can race the revive path ahead of a UUID
(re)capture, so `HasClaudeSession()` returns false even though a real, resumable
conversation exists on disk under `~/.claude/projects/<encoded-path>/`.

## Suggested directions from the reporter

1. **Don't silently start fresh.** If a JSONL transcript already exists for this
   session's working directory (or a UUID was ever persisted for it) but the in-memory
   UUID is missing at revive time, don't spawn a fresh Claude un-announced — either
   attempt UUID recovery from the newest JSONL under
   `~/.claude/projects/<encoded-path>/` first, or treat it as a state needing attention.
2. **Persist the UUID earlier / more durably** so it survives back-to-back restarts,
   and reload it from persistent storage on revive *before* the `HasClaudeSession()`
   check runs.
3. **Make the cold-start-fresh path louder** — surface it in the UI ("started a new
   session; previous conversation could not be resumed") instead of only a log `WARN`.

## Related (separate follow-up, not in scope for this item's fix)

The restart churn that creates the racy window is itself worth investigating:
`driverInactivityTimeout` is a hardcoded `10 * time.Minute`
(`session/session_driver.go:46`) with no config override, and fires on the `Ready`
state keyed off terminal output — a session that looks quiescent to the driver but is
mid-work can be restarted, feeding the UUID-loss window above. Noting this for
traceability; the plan below should not expand scope to fix the watchdog itself unless
research shows it's cheap to include.

## Acceptance Criteria

1. When a session revives (tmux dead, not first-time setup) and a conversation
   transcript exists on disk for that session (JSONL under
   `~/.claude/projects/<encoded-path>/`, or a UUID was previously persisted), the revive
   path recovers/reuses that UUID and resumes with `--resume` instead of silently
   starting fresh, even if the in-memory `ConversationUUID` was empty going into the
   revive.
2. The `ConversationUUID` is persisted to durable session storage as soon as it is
   captured (not only held in memory), so it survives back-to-back
   restart/hibernate/revive cycles within the same process lifetime and across process
   restarts.
3. If, after recovery is attempted, no resumable conversation can be found at all (a
   genuinely first-ever start, or the transcript is missing/corrupt), the existing
   fresh-start behavior is preserved — this is not a regression target, only the false
   negative case.
4. When a cold start proceeds without a resume (the true fresh-start case above), that
   fact is surfaced beyond the log `WARN` — visible in the session's UI state/history
   (e.g. a system message or status flag) so a user does not have to read server logs to
   discover their context was not resumed.
5. Both call sites in `session/instance.go` (`startLocked` and `start`) apply the same
   recovery logic — no divergence between the two paths.
6. Existing passing tests for normal cold-restore (`HasClaudeSession()` true) and
   first-time-setup continue to pass unmodified in behavior.
7. New test coverage exists for: (a) revive with empty in-memory UUID but a discoverable
   on-disk transcript recovers and resumes, (b) revive with empty UUID and no on-disk
   transcript still starts fresh, and (c) the persisted-UUID-survives-restart path.

## Out of Scope

- Fixing/making-configurable `driverInactivityTimeout` (tracked as a related but
  separate concern above).
- Redesigning the broader session hibernation/reconciliation architecture.
