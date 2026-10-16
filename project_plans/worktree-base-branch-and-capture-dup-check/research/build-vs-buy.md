# Build vs Buy: duplicate-backlog-item handling

Research question (adapted from Agent 6's "build vs buy" lens): does this
codebase already have tooling/conventions for detecting or handling duplicate
backlog items, and is the detection gap itself worth a follow-up backlog item?

## 1. Existing duplicate-handling tooling

The codebase already has a purpose-built mechanism for exactly this
situation: the `report_duplicate` MCP tool
(`server/mcp/tools_backlog.go:2158` `reportDuplicate`, plus
`reportDuplicateUnclaimed` at line 2391 and the shared verifier
`resolveDuplicateRef` at line 2341). It was built for
`project_plans/backlog-self-resolve/` (source backlog item
`da58b867-bf4e-4720-8fe4-9cfcfa5b6eed`), whose own problem statement is
almost a mirror of this triage: *"a work session that discovers, after
opening its own PR, that a parallel session already shipped the same fix...
has no way to self-resolve."* Five ADRs back it
(`project_plans/backlog-self-resolve/decisions/ADR-001..005`).

Mechanics:
- **Two modes**, split on whether the calling session is linked to the item:
  1. *Linked, work-role session* (`item_id` + `duplicate_ref` + `reason`) →
     transitions the item to `review` with a note
     `"duplicate of <ref>: <reason>"`, appended to `VerificationNotes` so the
     reviewer prompt sees it. Never closes the item directly — ADR-001
     ("no new backlog status for duplicates") keeps this on the existing
     `review` rail rather than adding a `duplicate` status, so a human/the
     review gate still confirms before it's archived.
  2. *Unclaimed item, passerby session* (`reportDuplicateUnclaimed`, only for
     `idea/refining/ready/queued` status) → archives the item directly,
     since there's no diff for a reviewer to look at anyway.
- **Evidence bar**: `duplicate_ref` must parse as a real GitHub PR, issue, or
  commit URL (`resolveDuplicateRef` calls `githubpkg.ParseGitHubRefWithHosts`
  then verifies it exists via `GetPR`/`GetIssue`/`GetCommit`) — it does
  **not** accept a reference to another backlog item ID directly. To point
  at `c7466f05-...`'s fix, the caller supplies the *PR* that closed it
  (`https://github.com/tstapler/stapler-squad/pull/837` or `/833`), not the
  other item's UUID.
- **Idempotency and races**: ADR-004 (exact-retry no-op) and ADR-005 (no
  active-reviewer refusal, only wording changes) are handled; `report_duplicate`
  is rejected outright for `SkipReviewGate` items (must always reach a human).

This is the "buy" answer to the adapted question: don't build new
duplicate-detection tooling — the mechanism for *acting on* a known
duplicate already exists, is well-tested (ADRs + adversarial review already
ran), and is the correct next tool call for backlog item `4d856751` itself
(see §4).

## 2. What's still missing: nothing *detects* duplicates proactively

`report_duplicate` only handles the case where a human or agent has *already
noticed* the duplication — it's a resolution tool, not a detection tool.
Confirmed by search, nothing in the codebase proactively flags "this new
item looks like an existing one":

- No full-text/fuzzy search over backlog items themselves. `search_service.go`
  and `search_related_work.go` (`server/services/`) implement `SearchClaudeHistory`
  — full-text search over Claude conversation transcripts — not backlog item
  titles/descriptions. There is no `SearchBacklogItems`-equivalent RPC.
- No similarity/embedding check at item-creation time (ideation, GitHub-issue
  import via `github-issue-to-backlog` skill, or `ParseBacklogItemIntent` in
  `server/services/backlog_service_intent.go`) that would compare a new
  item's description against open/recently-closed items.
- The "Backlog item: `<url>`" convention in a PR body
  (`backlogItemLink`, `session/backlog_lifecycle_pr.go:380`, used at
  `:395` and `:617`) is **one-directional and unstructured**: Go code
  writes a clickable deep link (`<dashboardBaseURL>/backlog?item=<id>`)
  into the PR body so a *human reviewer* can jump to the item. Confirmed by
  repo-wide grep: nothing parses `"Backlog item:"` back out of a PR body,
  no webhook handler cross-references it against other open items, and there
  is no queryable field anywhere (schema, ent, proto) storing "this PR
  closed backlog item X" for later lookup. It is convenience text for a
  human, not a structured backlink — so PR #837's body linking to
  `c7466f05-...` was itself undiscoverable by any automation; it only
  surfaced because this triage's `requirements.md` phase happened to read
  the PR bodies by hand.

`project_plans/backlog-already-implemented/` (source item, 2026-07-14) is
adjacent but answers a different question: it's about an implementation
agent finding its *own* item's acceptance criteria already satisfied
mid-work (empty-diff review path, reviewer given codebase-read access to
verify the claim) — not about a *second, separately filed* item duplicating
a *first* item's already-shipped fix. It's fully planned (requirements →
research → implementation/plan.md → adversarial-review.md → validation.md
all present) — status of actual implementation wasn't re-verified here since
it's out of scope for this question, but the point stands regardless: even
fully implemented, it would not have caught the `4d856751`/`c7466f05`
situation, because there was no "own work already done" moment — two
separate items were filed for the same user report, and one's fix (#833/#837)
shipped without ever touching the other's status.

## 3. Is the detection gap itself worth a follow-up backlog item?

Yes, narrowly. The concrete, falsifiable gap: **when a PR closes one backlog
item, nothing checks whether other open items describe the same
issue/repro.** A cheap version doesn't need embeddings or an LLM
similarity pass — even a title/description substring or trigram match
against open items at PR-merge time (or at new-item creation time via the
existing `ParseBacklogItemIntent` LLM call, which already reads the raw
text) would have caught `4d856751` filed against the same repro as
`c7466f05` before both spent separate triage cycles. This is scoped as its
own small item, not a blocker for closing `4d856751` — see §4.

## 4. Recommendation for backlog item `4d856751` (brief — verdict belongs in plan.md)

Close it as a duplicate of `c7466f05-3d19-4d25-a822-9ea1ac7a6faa`, using the
existing `report_duplicate` MCP tool rather than a manual status edit: call
it with `item_id=4d856751-...`, `duplicate_ref=` the PR that actually shipped
the fix (`https://github.com/tstapler/stapler-squad/pull/837`, since #837 is
the one explicitly linked to `c7466f05` in its body; #833 landed the
underlying mechanism a day earlier), and `reason` citing both PRs and this
triage's verification (passing `TestAmbientHEAD|TestResolveRemoteWorktreeBaseCommit|TestResolveWorktreeBaseCommit`
tests, `CreationWarning`/`SESSION_NOT_READY` plumbing confirmed in-tree).
This is (a) from the task's own framing, and it's the right call — it routes
through the review gate (or archives directly if unclaimed) rather than a
silent edit, matches the tool's intended use case almost exactly, and
produces the same audit trail (`VerificationNotes`, status-history note)
every other duplicate resolution on this repo already gets.
