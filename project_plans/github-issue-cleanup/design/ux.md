# UX Design: GitHub Issue Cleanup

**Feature:** github-issue-cleanup (backlog item 404529d0)
**Date:** 2026-11-03
**ACs covered:** AC-1, AC-2, AC-3, AC-4

---

## Surfaces

This feature touches three non-interactive text surfaces. There are no new UI controls; all outputs are automated text produced by the backend at specific lifecycle transitions.

| # | Surface | Trigger | Audience |
|---|---|---|---|
| 1 | GitHub issue close comment | Backlog item → `done` | Issue watchers, assignees |
| 2 | Fallback PR body | Agent opens a PR via `buildFallbackPRBody` | PR reviewers, GitHub automation |
| 3 | Agent session prompt | Session start, `BuildSessionInitialPrompt` / `BuildTokenBudgetedPrompt` | Claude agent |

---

## Surface 1: GitHub Issue Close Comment

### Context

Posted to the GitHub issue thread when a backlog item transitions to `done`. Extends the existing `forwardSyncCloseComment` / `forwardSyncCloseCommentFor` behavior to cover MCP-imported items (those with `ExternalURL` but no `SourceID`) and to include the PR URL when known.

Issue watchers subscribe for notification of this comment. It is the last automated message they receive in the thread and their primary way to navigate to the implementing code.

### Representative outputs

**Case A — PR URL known, no host ID (most common):**
```
Closed automatically — the linked backlog item was marked done in stapler-squad. Implemented in: https://github.com/owner/repo/pull/123.
```

**Case B — PR URL known, host ID set:**
```
Closed automatically — the linked backlog item was marked done in stapler-squad. Implemented in: https://github.com/owner/repo/pull/123. on host `my-host`.
```

**Case C — no PR URL recorded (unchanged from today):**
```
Closed automatically — the linked backlog item was marked done in stapler-squad.
```

**Case D — no PR URL, host ID set (unchanged from today):**
```
Closed automatically — the linked backlog item was marked done in stapler-squad. on host `my-host`.
```

**Case E — no GitHub token available:**
No comment is posted. The issue is not closed. No user-visible output of any kind.

### Design rationale

- "Implemented in:" mirrors the register of the existing `progressReviewComment` ("A pull request has been opened for this issue: {url}") — both use ": {url}" as a preposition-link, maintaining visual consistency for watchers who may see all three automated comments in the same thread.
- The em dash (`—`) is carried over from the existing `forwardSyncCloseComment`. Changing it would create inconsistency between the no-PR-URL form (old) and the with-PR-URL form (new).
- No change summary, no stapler-squad URL, no bot attribution marker. The existing baseline sets these standards and they must be preserved.
- When no PR URL is recorded, the clause is omitted entirely — not replaced by a placeholder.

### UX acceptance criteria

**UAC-1.1** A human watching a GitHub issue sees exactly the Case A string when the item is marked done and a PR URL is recorded. No trailing whitespace, no double period.

**UAC-1.2** When no PR URL is recorded on the item at close time, the comment is exactly the Case C string — the "Implemented in:" clause is absent, with no placeholder such as "(no PR URL)" or "N/A".

**UAC-1.3** When a non-empty host ID is configured, the ` on host \`{id}\`.` suffix appears after the last sentence in the same position as today (Cases B, D).

**UAC-1.4** The comment contains no emoji, no Markdown headers, no bold/italic formatting, and no bot-attribution marker ("🤖", "automated comment", etc.). Plain prose only.

**UAC-1.5** When no GitHub token is available for the issue's host, the GitHub issue remains open, no comment appears in the thread, and no error is surfaced in the stapler-squad UI or logs at a level above DEBUG.

**UAC-1.6** A watched GitHub issue shows all three automated comments in the same thread (started, review, closed) with consistent tone: sentence case, declarative voice, period-terminated, no exclamation marks.

---

## Surface 2: Fallback PR Body

### Context

`buildFallbackPRBody` constructs a PR body when the agent has not drafted one. It already includes a backlog-item link. AC-3 adds a GitHub closing keyword so GitHub auto-links the issue to the PR and closes it on merge.

The line appears in the PR body as seen by reviewers on GitHub.

### Representative outputs

**GitHub issue URL (`/issues/` path):**
```
Closes https://github.com/owner/repo/issues/42
```

**GitHub PR reference URL (`/pull/` path):**
```
Related: https://github.com/owner/repo/pull/55
```

**No ExternalURL (plain backlog item):**
No keyword line. The PR body is identical to today.

### Design rationale

- "Closes" is GitHub's canonical closing keyword. It works on both github.com and GHE. Using the full URL (not `#N`) handles cross-repository references and avoids ambiguity when the PR and issue are in different repos.
- "Related:" is used for `/pull/` URLs because a PR cannot auto-close another PR via GitHub automation. "Fixes" or "Closes" on a PR URL would create a confusing expectation.
- The keyword line lives in the Summary section of the PR body, before prose content, so it is visible at a glance and not buried in a footer.

### UX acceptance criteria

**UAC-2.1** A human reviewing the fallback PR body for a GitHub-issue-backed item sees `Closes https://github.com/...` on its own line. The line is not embedded in a sentence and not duplicated.

**UAC-2.2** For a `/pull/` ExternalURL, the line reads `Related: https://github.com/...` — not "Closes" or "Fixes".

**UAC-2.3** For a plain backlog item (empty ExternalURL), no "Closes" or "Related:" line appears anywhere in the PR body.

**UAC-2.4** The keyword line uses the full issue URL, not a short `#N` reference, so it is unambiguous when the PR and issue are in different repositories.

---

## Surface 3: Agent Session Prompt

### Context

`BuildSessionInitialPrompt` and `BuildTokenBudgetedPrompt` inject context into the Claude agent's starting prompt. AC-4 adds two lines: a fact line naming the linked GitHub issue, and a deterministic instruction line telling the agent the exact text to include in its PR body.

The agent reads this prompt; it is not shown to the user. Its primary goal is to eliminate the keyword-inference step — the agent must not need to decide between "Closes", "Fixes", or "Resolves".

### Representative outputs

**GitHub issue URL (`/issues/` path):**
```
Linked GitHub Issue: https://github.com/owner/repo/issues/42

When opening a pull request, include this exact line in the PR body:
Fixes https://github.com/owner/repo/issues/42
```

**GitHub PR reference URL (`/pull/` path):**
```
Linked GitHub PR: https://github.com/owner/repo/pull/55

When opening a pull request, include this exact line in the PR body:
Related: https://github.com/owner/repo/pull/55
```

**No ExternalURL:**
No new content. The prompt is byte-for-byte identical to today.

### Design rationale

- "This exact line" is unambiguous direction. It removes the agent's latitude to rephrase, choose a different keyword, or omit the line "if it doesn't seem relevant." The literal string to paste is on the next line.
- The blank line between the fact line and the instruction block makes the two concepts visually distinct when the agent scans the prompt.
- Placement in the hard-facts section (project path, branch, item title) ensures the block survives the first truncation pass in `BuildTokenBudgetedPrompt`. The instruction line is higher-value than the label line; if only one line survives a second pass, the instruction ("Fixes <url>") must survive before the label ("Linked GitHub Issue: <url>").
- No new content when ExternalURL is empty. The requirement is a strict no-regression: items without an ExternalURL must have an identical prompt to today.

### UX acceptance criteria

**UAC-3.1** An agent session started for a GitHub-issue-backed item contains the literal string `Linked GitHub Issue: https://github.com/...` in its prompt, using the item's exact `ExternalURL` value.

**UAC-3.2** The same prompt contains the literal string `Fixes https://github.com/...` as a standalone line in the instruction block (not embedded in a sentence).

**UAC-3.3** For a `/pull/` ExternalURL, the fact line reads `Linked GitHub PR: https://github.com/...` and the instruction line reads `Related: https://github.com/...`.

**UAC-3.4** When ExternalURL is empty, the agent prompt contains no "Linked GitHub" section and no "Fixes" or "Related:" instruction line. A diff of the prompt against a baseline (item without ExternalURL) shows zero additions.

**UAC-3.5** The two-line block appears in the section of the prompt that survives truncation (hard-facts section), not in an optional-context section that the budgeted builder drops on the first pass.

**UAC-3.6** The instruction line survives when the prompt is truncated to its minimum viable size — confirm by forcing a low token budget and verifying `Fixes https://...` is present even if `Linked GitHub Issue: https://...` is dropped.

---

## Summary of UX decisions

| Decision | Ruling |
|---|---|
| Close comment format (with PR URL) | "Closed automatically — the linked backlog item was marked done in stapler-squad. Implemented in: {prURL}." |
| Close comment format (no PR URL) | Unchanged from today — omit the clause entirely |
| Closing keyword in PR body | `Closes <full-url>` for issues, `Related: <full-url>` for PR references |
| Agent instruction style | Literal "Fixes <url>" or "Related: <url>" — no keyword inference |
| Emoji, Markdown, bot attribution | None — matches existing comment baseline |
| Deduplication for done-close comment | None — watermark prevents re-ingest; duplicate is rare and contextually accurate |
| No-token failure mode | Silent skip — DEBUG log only, no user-visible error |
