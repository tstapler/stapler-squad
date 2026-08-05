# Research: Feature Landscape — Session Completion Summary

Agent 2 (Features). Grounding reads only — no code written.

## 1. Closest existing analog: the review-verdict pipeline

The `ReviewVerdict` flow (spawned by `ReviewGateRunner.Run`, `session/review_gate.go`) is
the single closest precedent in the codebase for almost every mechanic this feature needs:
an async, LLM-backed document generated from a session's diff at session-end time, persisted
independently, with graceful degradation on failure.

- **Schema precedent** — `ReviewVerdict`
  (`session/ent/schema/review_verdict.go`) has exactly the shape a completion-summary schema
  should copy: `diff_hash`, `prompt_hash`, `diff_token_count`, `diff_truncated` (bool,
  default false), `summary`, `per_criterion` (JSON blob), plus `override_by`/`override_reason`/
  `override_at` for the human-correction path. Unlike `AnalyticsEvent` and the notification
  store, `ReviewVerdict` *is* ent-modeled with a edge to `ItemSession` (`Required().Unique()`)
  — but requirements.md explicitly wants independence from the `Session` row, so the better
  structural precedent for *persistence style* is `AnalyticsEvent`
  (`session/ent/schema/analytics_event.go`), which keys off a plain `session_id string`
  field with no required edge, or the notification store (`server/notifications/store.go`,
  see §3) which is a flat file store keyed by `SessionID string`, not an ent edge at all.
- **Diff truncation precedent** — `GetGitDiff`/`GetGitDiffRef`
  (`session/backlog_review.go:646-680`) truncate diff content at
  `headless.MaxDiffSizeReview = 40_000` bytes (`session/headless/features.go:56`) before
  feeding it to an LLM prompt, and set a `truncated` bool that gets threaded all the way into
  the persisted `ReviewVerdict.DiffTruncated` field. The narrative-generation step of this
  feature should reuse this exact function/constant rather than inventing new truncation
  logic.
- **Prompt-injection sanitization precedent** — `SanitizeDiff`
  (`session/backlog_review.go:611-615`) neutralizes ` ``` ` sequences in diff content before
  interpolating it into an LLM prompt, to prevent a diff from breaking out of its markdown
  code fence and injecting instructions. The narrative step must apply the same sanitization.
- **Graceful-degradation precedent** — `RecordDegradedReviewVerdict`
  (`session/backlog_review.go:595-603`) persists a synthetic `UNVERIFIABLE` verdict when the
  LLM step can't produce a real one. This is the direct precedent for FR-5's "narrative
  failure still produces a READY document" requirement: don't leave the row absent or in a
  perpetual pending state, write a synthetic/deterministic result and mark it accordingly.
- **Async fire-and-forget trigger precedent** — `instanceBacklogListener.OnLifecycleEvent`
  (`session/backlog_lifecycle.go:797-811`) is the literal listener requirements.md points at
  (BUG-027 comment): it treats `EventExited` and `EventStopped` identically and dispatches
  via `go il.parent.onSessionExited(...)`, i.e. lifecycle events fire generation
  asynchronously off the event-handling goroutine so teardown itself is never blocked (FR-5).
  A new listener (or a case added to this one) is the natural trigger point for FR-1.
- **Worktree-gone / diff-unavailable handling precedent** — `ReviewGateRunner.Run`
  (`session/review_gate.go:96-177`) already handles: worktree missing (falls back to
  `item.RepoPath` + explicit branch ref via `GetGitDiffRef`, since a fallback dir's `HEAD`
  isn't the work branch's tip), uncommitted changes left in the worktree (warns but doesn't
  block), and a stale/corrupted `base_commit_sha` (attempts auto-repair, see lines ~180+,
  not fully read but flagged as worth reading in planning). This is the pattern to copy for
  "git worktree deleted before generation runs" and "diff computation fails" edge cases
  rather than reinventing it.

## 2. GetSessionDiff / DiffStats do NOT truncate — real gap for huge diffs

`SessionService.GetSessionDiff` (`server/services/session_service.go:2586`, referenced in
requirements.md) and the underlying `GitWorktree.Diff()` (`session/git/diff.go:43-112`) that
it calls for completed sessions have **no size cap or pagination** — `DiffStats.Content` is
the full, unbounded `git diff` output as a single string. Contrast with `GetGitDiff` (§1),
which is capped at 40,000 bytes specifically because it feeds an LLM prompt.

Implication for this feature: a session with a multi-thousand-file diff would (a) blow up
the narrative LLM prompt if fed `GetSessionDiff`'s content directly — use the already-capped
`GetGitDiff`/`GetGitDiffRef` for the narrative step, not `GetSessionDiff` — and (b) risk an
oversized markdown document if the "changes" section of FR-2 tries to embed the diff itself
rather than a diff *stat* (files/lines changed) plus a link back to the session's existing
Diff tab. Recommend the summary embed only aggregate stats (added/removed line counts, file
count) and link out to the live diff view, never embed full diff content — sidesteps both the
truncation gap and the markdown-export bloat problem (FR-4 wants this pasted into a PR body).

## 3. Independent persistence — two divergent precedents, pick one deliberately

- **Ent-modeled, `session_id` string field, no required edge**: `AnalyticsEvent`
  (`session/ent/schema/analytics_event.go`) — durable via the normal ent/DB path, survives
  Session-row deletion by construction since there's no FK/edge to cascade.
- **Flat JSON file store, keyed by `SessionID string`**: `NotificationHistoryStore`
  (`server/notifications/store.go`) — notably has `PruneOrphaned` /
  `pruneOrphanedRecords` (`store.go:368-410`), which **deletes** notification records once
  their `SessionID` is absent from `existingSessionIDs()`. This is the *opposite* of what
  this feature needs (FR-3: survive Session-row deletion) — flag explicitly in planning that
  if a file-store pattern is chosen, it must NOT wire up an equivalent orphan-pruning pass,
  or the summary will be deleted exactly when it's most needed (after the session/worktree is
  gone and scrollback is the only other record).

  Recommend the `AnalyticsEvent`-style ent schema (string `session_id` field, indexed, no
  required edge) over the file-store pattern — it gets transactional writes, existing
  migration tooling, and matches `ReviewVerdict`'s field vocabulary (§1) more closely than a
  bespoke JSON file would.

## 4. Idempotency (FR-7) — `singleflight` is an established pattern here

`golang.org/x/sync/singleflight` is already a repo dependency and used for exactly this
class of problem — coalescing concurrent duplicate work — in `github/client.go`,
`github/user_pr_cache.go`, `session/tmux/tmux.go`, and
`session/unfinished/gogit_vcs_reader.go` (`sfDo[T any]` helper at line 619 is a generic
wrapper worth copying). Recommend a `singleflight.Group` keyed by session ID (or summary ID)
so: (a) duplicate `EventExited` fires for the same session coalesce into one generation run,
and (b) a `Regenerate` click while a generation is already in flight joins that same call
instead of spawning a second overlapping pipeline — directly satisfies FR-7 without inventing
new locking/dedup machinery. Combine with a stored `status` field (`PENDING`/`READY`/`ERROR`)
so the UI can show "generation in progress" immediately rather than needing to poll
singleflight state directly.

## 5. Edge cases beyond requirements.md, found in code

- **No linked backlog item** — approval-decision data (FR-2's decisions breakdown) comes
  from `ApprovalHandler` (`server/services/approval_handler.go`, auto-allow/auto-deny/manual
  classification around lines 279-299) and `NotificationHistoryStore.AppendAutoApproved`
  (`store.go:181`), both of which key on plain `sessionID`, not backlog-item ID — so approval
  data is available even for backlog-less/ad-hoc sessions. The parts of FR-2 that are
  backlog-shaped (review-queue-resolved, still-open decisions) come from `ItemSession`/
  `ReviewVerdict`, which genuinely has no data for a non-backlog session — the deterministic
  fallback (FR-6) must handle "no backlog item" as a first-class empty state, not just "no
  activity."
- **Session never reached Ready (crashed during startup)** — not explicitly examined in
  code during this pass, but `EventExited` fires on any process exit including a startup
  crash (`session/instance.go:788,811`), so the trigger will fire regardless of whether the
  session ever did meaningful work. FR-6's "minimal-activity" empty-state document is the
  correct backstop for this case too — no special-case needed as long as diff/approval/token
  lookups all tolerate "session had almost no lifecycle" gracefully (empty diff, zero
  approvals, near-zero token usage all already produce zero-valued structs per §2 above, not
  errors).
- **Multiple sessions sharing a worktree** — not found as an explicit supported scenario in
  the code searched (worktree lookups are consistently by session UUID via
  `GetWorktreeDataBySessionUUID`); flag as an open question for the architecture-research
  agent rather than resolved here — if worktree sharing is possible, diff attribution between
  the two sessions' summaries needs a defined boundary (likely: diff since *that session's*
  own base_commit_sha, which is already how `GetGitDiff` scopes it, so this may be a
  non-issue by construction rather than something new to build).
- **Reconciler-fired `EventExited`** — `session/review_queue_poller.go:451` fires
  `EventExited` with reason `"reconcile-session-missing"`; requirements.md already excludes
  this from triggering generation (FR-1) — confirmed this is a distinct, filterable reason
  string on the same event type, so the listener needs a reason check, not a new event type.

## 6. Unstated needs

- **History/search surfacing** — the web UI already has a full-text session search and
  browsing feature under `web-app/src/components/history/` (`HistorySearchInput.tsx`,
  `HistorySearchResults.tsx`, `HistoryEntryCard.tsx`, `HistoryGroupView.tsx`) backed by
  `useHistoryFullTextSearch.ts`, which calls a `SessionService` `SearchSessions`-style RPC
  (`SearchResult`/`SearchSnippet`/`HighlightRange` types from `@/gen/session/v1/session_pb`)
  and a card grid (`HistoryEntryCard.tsx`) showing session name, status, model, message
  count, project, time-ago. This is exactly the "history search" / "session card history
  view" the backlog item description alludes to. The completion summary is a natural
  candidate to (a) be indexed into this same search so its narrative text is discoverable,
  and (b) show a summary excerpt/badge on the `HistoryEntryCard` for terminal sessions. This
  is a real gap between requirements.md (which only requires a per-session Summary *tab*) and
  the item's own stated context — worth flagging to the product/UX research agent as a
  candidate follow-on scope item, not silently added to FR-3's scope.
- **Completion notification** — the notifier (`Notifier.Notify`, used e.g. in
  `session/review_gate.go:127-132` for the branch-drift-blocked case) is the established
  mechanism for "something finished, tell the user" across this codebase (toast +
  notification-history entry). Requirements.md's FR-5 doesn't mention a toast on generation
  completion; given every other async background-completion event in this codebase (auto-
  approvals, review verdicts, branch-drift blocks) fires through `Notifier`, silently *not*
  notifying on summary-ready would be inconsistent with the rest of the app's UX vocabulary.
  Flag as a likely FR gap for the plan phase, not decided here.
- **Staleness/regenerate hint** — confirmed by grep (`regenerate` in Go/TS sources) there is
  no existing "stale, click to regenerate" UI pattern anywhere in the codebase to copy from;
  this concept doesn't need to exist per requirements.md (session is terminal so the diff
  can't change after generation) — no action needed, just confirming there's no existing
  precedent being missed.

## 7. Feature registry — process requirement, not product

Per `.claude/rules/feature-registry.md`, this feature will need:
- A new backend entry under `docs/registry/features/backend/<feature>.json` for the new RPC
  (likely `GetSessionSummary` / `RegenerateSessionSummary` or similar) — shape confirmed via
  existing examples (`docs/registry/features/backend/GetInsightsSummary.json`,
  `ImportGitHubIssue.json`): fields `id`, `type: "backend"`, `service`, `method`,
  `protoFile`, `markerFound` (true only if a `// +api:` marker is added in the handler),
  `handlerFile`, `tested`, `testIds`, `lastModified`.
- A new frontend entry under `docs/registry/features/frontend/<feature>.json` for the new
  Summary tab component, with `filePath` pointing at the new tab component.
- `make registry-generate` run after adding both, and `make registry-diff` checked for
  unexpected coverage-gap growth before the PR is considered complete. This is process, not
  product scope, but should be listed as an explicit task in the implementation plan since
  it's easy to forget when the PR's main focus is a new ent schema + RPC + React tab.
