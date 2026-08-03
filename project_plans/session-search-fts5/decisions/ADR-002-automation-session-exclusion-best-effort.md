# ADR-002: Automation-Session Exclusion Is Best-Effort, Via Live `Instance.Hidden`, Not a Persisted Field

**Date**: 2026-08-02
**Status**: Accepted
**Context**: session-search-fts5 feature — `exclude_automation_sessions` request flag on `SearchClaudeHistory`, resolving requirements.md Open Question 1 ("source=tool" exclusion).

**Correction (2026-08-02, pre-implementation validation pass)**: this ADR originally named `Instance.AutonomousMode` as the filter signal. `AutonomousMode` is the user-facing "Fix Autonomously (Beta)" opt-in (`.claude/rules/session-creation-registry.md`) — a *human* deliberately choosing autonomous execution, not a background/system session marker. Background/triage automation sessions are created via `CreateDirectorySession(..., hidden=true)` and do not set `AutonomousMode`; filtering on it would both fail to exclude the real background sessions this feature targets and incorrectly exclude legitimate human "Fix Autonomously" sessions. `Instance.Hidden` — the same field `ListSessions`' `include_hidden` flag already uses to hide background/triage/review sessions from the default list — is the correct signal, and is what plan.md's Epic 1.5 (`isAutomationSession`) actually implements. This file is updated below to match; do not revert to `AutonomousMode`.

---

## Context

The originating backlog item assumed a `source=tool` tag exists on history entries (referencing Hermes Agent's convention) that could be filtered on to hide background/automation sessions from human-facing search results. requirements.md flagged this as an open question because stapler-squad's schema was not confirmed to have an equivalent field.

Three full-lineage reads confirm no such field exists anywhere upstream:

1. `~/.claude/history.jsonl` (`historyJSONLEntry`, `session/history.go:95-100`) — owned and written by the Claude Code CLI itself, not stapler-squad. Fields: `display`, `timestamp`, `project`, `sessionId`. No source/origin field.
2. Per-session conversation JSONL (`conversationMessage`, `session/history.go:80-91`) — also Claude-CLI-owned, same absence.
3. `session.search.Document` (`document_store.go:9`) is synthesized entirely from (1)+(2) at index time — there is no origin signal available *to add* into `Document`, because nothing upstream carries one.

The only origin signal that exists anywhere in the repo is `session.Instance.Hidden` (`session/instance.go:220-223`), persisted alongside `ConversationUUID` (`session/storage.go:163`) — but only in the **live, in-memory session store**. `Storage.DeleteInstance` (`storage.go:458`) removes a session's persisted record once it's deleted/archived; there is no closed-session archive that retains `Hidden` indefinitely.

## Decision

`exclude_automation_sessions=true` filters a search result out **only** when its `SessionId` matches a currently-tracked `Instance` (via `ConversationUUID`) whose `Hidden` is `true`. A session with no matching live `Instance` (the common case for anything more than a few days/restarts old) is **kept** — treated as "signal unavailable," never as "confirmed human." This is implemented as `isAutomationSession`/`filterAutomationSessions` in `server/services/search_related_work.go`, called from the `SearchService.SearchClaudeHistory` handler (which already has `ss.getInstances()` wired for the analogous `liveSessionStatus` cross-reference at `search_service.go:157-167`) — not inside `SearchEngine`, which has no dependency on `Storage`/`Instance` state and shouldn't gain one.

The filter is explicitly documented (proto comment on `exclude_automation_sessions`, and in code) as best-effort, not exhaustive.

## Consequences

- Zero schema/data-migration cost: no new field on `Document`, `ClaudeHistoryEntry`, or any persisted JSONL structure, so `IndexStore.CurrentIndexVersion` does not need to bump.
- The filter's coverage degrades over time: a background/automation session is reliably excluded while its `Instance` is live or recently active, and silently stops being excludable once that `Instance` record is deleted (e.g., after the session is archived/reconciled away). Users should not expect the filter to hide old automation sessions from search history.
- If exhaustive coverage (including long-deleted `Instance` records) becomes a real requirement later, the two concrete escalation paths are: (a) persist `conversation_uuid` on the `Session` ent schema so historical sessions retain the join key past `DeleteInstance`, or (b) add a durable field written into `history.jsonl`/session metadata at session-creation time. Both are deliberately out of scope here — they touch JSONL-writing and/or ent schema surfaces that requirements.md scoped as risky/unnecessary unless a concrete need emerges (see requirements.md's Rabbit Holes section).

## Alternatives Considered

- **Add a new `Document.Source` field, populated somehow at index time** — rejected. There is nothing upstream (Claude-CLI-owned JSONL) to populate it with in the general case; adding the field would just create a place to store a signal that still doesn't exist. Would also force an `IndexStore` gob-schema version bump for no coverage gain over the live-`Instance` approach.
- **Filter on `Instance.AutonomousMode`** — rejected (see Correction above): it identifies a human's opt-in to autonomous execution, not a background/system session, and using it would both miss real automation sessions and wrongly hide legitimate human ones.
- **Path-based heuristic** (match `entry.Project` against a known backlog-automation worktree path pattern) — considered as a documented fallback in `research/architecture.md` §6 if exhaustive coverage is later required, but not adopted for v1: it's a heuristic with false-positive/negative risk (a legitimate human session in a path that happens to match the pattern), whereas the live-`Instance.Hidden` cross-reference is a precise signal for the cases it *can* see. Layering an imprecise fallback on top of a precise-but-partial signal was judged to add complexity without a clearly better trade-off, and is deferred rather than rejected outright.
- **Block on this question and add a persisted field before shipping the other four gaps** — rejected. requirements.md's own "Risk Control: not needed — low risk" framing and Appetite ("medium, 1-2 weeks, additive") don't support gating dedup/context-window/scroll-mode work on solving a fundamentally separate, harder data-lineage problem. Shipping the best-effort filter now with the limitation explicitly documented (this ADR, plus the proto comment on `exclude_automation_sessions`) is preferred over either blocking or silently shipping a filter that looks exhaustive but isn't.

Full evidence trail: `project_plans/session-search-fts5/research/architecture.md` §6, `project_plans/session-search-fts5/research/features.md` §2, `project_plans/session-search-fts5/research/pitfalls.md` §6.
