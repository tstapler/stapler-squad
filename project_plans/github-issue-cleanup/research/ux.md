# UX Research: GitHub Issue Cleanup

**Feature:** github-issue-cleanup (backlog item 404529d0)
**Date:** 2026-11-03

---

## 1. GitHub Issue Watcher Experience

### Existing comment tone (baseline)

All three existing comments follow the same pattern: plain prose, no emoji,
no markdown except backticks around the host ID, declarative/passive voice,
brief. No "🤖 Bot" attribution marker. Source:

- `progressStartedComment`: `"Work has started on this issue in stapler-squad."` (+ `" on host \`{hostID}\`."` when known)
- `progressReviewComment`: `"A pull request has been opened for this issue: {url}"`
- `forwardSyncCloseComment`: `"Closed automatically — the linked backlog item was marked done in stapler-squad."` (+ `" on host \`{hostID}\`."` via `forwardSyncCloseCommentFor`)

### AC-2: Close comment including the PR URL

The PR URL is the single most useful piece of information for a watcher: they
want to find the code that resolved their issue. The existing progressReviewComment
already links the PR, but that comment is gated by the `github_issue_progress_comments`
flag (default off). The close comment is always posted, so it is the reliable
fallback channel for watchers who may not have seen the review comment.

**Recommended wording** (matches the existing AC-2 example in requirements.md):

> Closed automatically — the linked backlog item was marked done in stapler-squad. Implemented in: https://github.com/owner/repo/pull/123.

Rationale:
- "Implemented in:" mirrors the register of "A pull request has been opened for this issue:" — both use "in:" as a preposition-link.
- Sentence stays declarative, no exclamation mark, consistent with baseline.
- When no PR URL is recorded, the comment is unchanged from today (requirements AC-2 is explicit: "if no PR URL is recorded, the comment is unchanged"). Do not write "No PR URL recorded" — omit the clause entirely.

**Host ID**: Keep the existing `forwardSyncCloseCommentFor` behavior (append `" on host \`{hostID}\`."` when non-empty). The host tag helps when multiple stapler-squad instances manage issues in the same GitHub org.

**stapler-squad instance URL**: The existing comments do not expose the instance URL, and this feature should not start doing so. The host ID already disambiguates multi-instance setups, and the public URL is not reliably known at comment time.

**"What changed" summary**: Do not include. The implementing PR is the place for a change summary. Embedding a summary in the automated comment creates stale-data risk (the comment is written at close time, before code review may still revise things) and adds noise to the issue thread.

### Overall watcher sequence

A watched GitHub issue now produces at most four automated comments in a full run:
1. "Work has started on this issue in stapler-squad." (in_progress, flag-gated)
2. "A pull request has been opened for this issue: {url}" (review, flag-gated)
3. "Closed automatically — the linked backlog item was marked done in stapler-squad. Implemented in: {prURL}." (done, always)
4. The GitHub issue is closed by the API call; GitHub itself appends "Closed by {actor}" in the event stream.

This is a reasonable sequence. Items 1 and 2 are already shipped (flag off by
default). Item 3 is the only one this feature introduces for MCP-imported items.

---

## 2. Agent (Claude) Reading the Prompt

### AC-4 instruction line

The instruction must be:
- **Unambiguous**: the agent must not need to infer whether "Fixes" is the right keyword.
- **Deterministic**: not conditional on agent judgment ("if relevant", "try to").
- **Non-intrusive**: fits in a single two-line block, not a paragraph.

**Recommended prompt rendering** (issue URL case):

```
Linked GitHub Issue: https://github.com/owner/repo/issues/42

When opening a pull request, include this exact line in the PR body:
Fixes https://github.com/owner/repo/issues/42
```

The "exact line" framing removes ambiguity about keyword choice. The agent
does not need to decide between "Closes", "Fixes", "Resolves" — it has the
literal string to paste. This is safer than "use a GitHub closing keyword"
which the agent could interpret as a suggestion.

**PR URL case** (requirements AC-4: `/pull/` path → "Related: <url>"):

```
Linked GitHub PR: https://github.com/owner/repo/pull/55

When opening a pull request, include this exact line in the PR body:
Related: https://github.com/owner/repo/pull/55
```

**Empty ExternalURL**: Render nothing. No new section appears in the prompt.
This is a strict no-regression requirement — the existing prompt for items
without an ExternalURL must be byte-for-byte identical to today.

### Truncation survival (BuildTokenBudgetedPrompt)

The two-line block should be placed in the item context section that survives
the first truncation pass. If the prompt has a "hard facts" section (project
path, branch, item title/ID), place the linked-issue block there — not in an
"additional context" section that the budgeted builder drops first. The
instruction line is the higher-value of the two; if only one survives
truncation, it should be the instruction, not the label line.

---

## 3. Comment Deduplication for the Done-Close Comment

### Current state

Progress comments deduplicate via `GitHubLastProgressCommentStatus` stored on
the item. The done-close comment in `handleForwardSyncClose` has no equivalent
deduplication guard: if an item is marked done, reopened by an operator, and
marked done again, the close comment is posted twice and the GitHub API
close is called twice (which is idempotent — a closed issue ignores a second
close call, so no error).

### Recommendation: do not add deduplication

The done-close path already has loop-prevention via the `GitHubSyncedIssueUpdatedAt`
watermark: the backward-sync ingestor skips events whose `updated_at` is at or
before the stored watermark, so the second close+comment does not cause a
re-ingest loop.

The duplicate comment is the only visible side effect. Its UX cost is low:
- Done-then-reopened-then-done-again is rare in practice.
- When it does happen, a second "Closed automatically" comment is accurate and
  informative — it tells the watcher that the item was reworked and completed
  a second time. This is better than silence.
- Deduplication would require either a new DB field (schema change, out of
  scope per requirements) or a live API read to check the issue's current
  close state (extra GitHub API quota, latency on the subscriber goroutine).

The progress-comment deduplication exists because "Work has started" twice on
a rework cycle (review → in_progress) would be misleading — the second
in_progress is a rework, not a fresh start. The done-close comment does not
have the same misleading quality: done twice means done twice.

**Conclusion**: leave the done-close comment undeduplicated. The watermark
already prevents the only harmful consequence (re-ingest loop). The duplicate
comment is rare and contextually correct.

---

## 4. Comment Tone and Format

### Ruling

Follow the existing baseline exactly:
- **No emoji.** No GitHub-flavored markdown beyond backticks for the host ID.
- **Plain declarative prose.** Passive or nominal constructions ("Closed
  automatically", "Implemented in") not imperative ("Check the PR at…").
- **No bot attribution marker.** The existing comments do not say "🤖 This is
  an automated comment". This is intentional — the phrasing ("Closed
  automatically") already signals automation without a noisy header.
- **Sentence case**, period-terminated, no exclamation mark.
- **Short.** One to two sentences. The close comment with PR URL is currently
  the longest: two short sentences. That is the ceiling.

### Applying the standard to new comments

The AC-2 close comment with PR URL:
> Closed automatically — the linked backlog item was marked done in stapler-squad. Implemented in: {prURL}.

The em-dash (`—`) is already used in `forwardSyncCloseComment`. Keeping it
maintains visual consistency across the three comment types a watcher may see
in the same thread.

No new comment types are introduced by this feature beyond the PR-URL extension
of the existing close comment.

---

## 5. Silent Failure UX

### Current behavior in `handleForwardSyncClose`

Three distinct failure paths:

| Failure | Log level | Stored | UI |
|---|---|---|---|
| Close (CloseIssue) fails | WARN | `RecordSourceSyncFailure` on `source.ID` | None on item |
| Post comment fails (after successful close) | WARN | Not stored | None |
| Persist watermark fails | WARN | Not stored | None |
| Decrypt token fails | WARN | Not stored | None |

### Where `RecordSourceSyncFailure` data surfaces

`GetSyncHistory` RPC (`backlog_service_query.go:263`) exposes sync history records to the UI.
They appear in the **BacklogSources settings panel** (`BacklogSourcesSettings.tsx`), not on the
backlog item detail page. A user who marks an item `done` and whose linked issue fails to close
gets no feedback on the item itself — the item shows `done`, the GitHub issue remains open, and
the error is only discoverable by visiting Settings → Backlog Sources.

### Design intent vs. implementation

`design/ux.md` UAC-1.5 says "no error is surfaced in the stapler-squad UI or logs at a level
above DEBUG" — but that criterion is scoped to the no-token case specifically. The actual code
uses WARN (not DEBUG) for genuine close/comment/watermark failures, which is consistent with the
existing convention (unexpected failures are WARN; expected no-ops are DEBUG). The mismatch is in
the UAC wording, not the implementation.

### Recommendation (follow-up, out of scope for this change)

Add a per-item "GitHub sync failed" indicator to the backlog item detail page. Source: query the
sync history for the item's `SourceID` and surface the most recent failure message inline. This is
a one-directional read from existing data (`GetSyncHistory`); it requires no schema change. Scope
it as a follow-up backlog item once the close path is live and generating real failure records.

---

## 6. Summary of Decisions

| Question | Decision |
|---|---|
| Close comment format (with PR URL) | "Closed automatically — the linked backlog item was marked done in stapler-squad. Implemented in: {prURL}." |
| Close comment format (no PR URL) | Unchanged from today |
| Include host ID in close comment | Yes, via existing `forwardSyncCloseCommentFor` |
| Include stapler-squad URL | No |
| Include change summary | No |
| Agent instruction keyword | Render literal "Fixes {url}" or "Related: {url}" — no keyword inference |
| Agent instruction placement | Hard-facts section; instruction line survives truncation before label line |
| Deduplication for done-close | None — watermark prevents re-ingest, duplicate comment is rare and informative |
| Markdown / emoji in comments | None, matching existing baseline |
| Silent failure visibility | No item-level indicator (follow-up); close failure stored in source sync history, visible in settings panel |
