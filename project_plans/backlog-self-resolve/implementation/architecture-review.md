# Architecture Review: backlog-self-resolve
**Date**: 2026-08-02
**Verdict**: CONCERNS

## Constitution Violations
N/A — `docs/adr/ADR-000-architecture-constitution.md` does not exist anywhere in this
repository (confirmed via `find . -iname "*constitution*"`, no hits). No constitution
constraints apply to this review.

## Blockers

- [ ] **Story 3.1.2 / Task 3.1.2b — idempotency check uses substring `strings.Contains`
  instead of an exact/delimited match, which is a real false-positive hazard, not a style
  nit.** The plan builds `notesMarker := fmt.Sprintf("duplicate_ref=%s", duplicateRef)` and
  tests membership via `strings.Contains(itemSession.VerificationNotes, notesMarker)`. Because
  GitHub PR/issue/commit URLs share a common path prefix that differs only in a numeric
  suffix, a shorter ref is a literal substring of a longer one with the same prefix: the
  string `"duplicate_ref=https://github.com/tstapler/stapler-squad/pull/27"` is a substring
  of stored notes containing `"duplicate_ref=https://github.com/tstapler/stapler-squad/pull/272"`.
  A second, *different* `report_duplicate("…/pull/27", …)` call after the first one recorded
  `…/pull/272` would be misclassified as the exact-retry no-op path (Story 3.1.2's third GWT)
  instead of the reject-a-differing-ref path ADR-004 explicitly mandates — silently discarding
  a legitimately different duplicate report with a "no changes made" message that is factually
  wrong. This is exactly the "Parse, Don't Validate" failure mode the `type-driven-design` lens
  flags: re-deriving a yes/no domain fact via raw substring containment on a free-text blob
  instead of an exact/structured match.
  **Remediation**: match on an exact, delimited line rather than raw `Contains`. Simplest fix
  within the plan's existing "one string field" design (no schema change, ADR-001-compatible):
  build the marker as its own newline-bounded token and compare with delimiters included, e.g.
  `"\nduplicate_ref=" + duplicateRef + "\n"` against `"\n" + itemSession.VerificationNotes + "\n"`,
  or — cleaner — put `duplicate_ref=<ref>` on its own line and split `VerificationNotes` on
  `"\n"` before comparing each line with `==` to the exact marker. Either closes the prefix-
  collision hole; Task 3.1.2b and its test (Story 4.2.4, `TestReportDuplicate_NoOpOnExactRetry`
  / `_RejectsDifferentRefAfterAlreadyResolved`) should add a `pull/27` vs `pull/272`-style case
  to lock in the fix.

## Concerns

- [ ] **Epic 2.2 / Task 2.2.1a — `hasActiveReviewSession`'s stated justification for adding a
  4th copy is factually wrong; a cleaner fix is available with zero cycle risk.** The task
  claims "`server/mcp` cannot import `server/services`, and there is no existing precedent for
  it importing that package" — but `server/mcp` already imports `server/services` today in
  three files (`server/mcp/server.go:14`, `server/mcp/tools_github.go:16`,
  `server/mcp/tools_lifecycle.go:13`), and `server/services` does not import `server/mcp`
  anywhere (the one text hit is a comment, not an import) — so no cycle exists in either
  direction. The real, unstated blocker is that `server/services/backlog_service_triage.go:1106`'s
  `hasActiveReviewSession` is unexported. Separately, the "4th copy" count is itself inflated:
  only that one function is a literal duplicate of the `Role == SessionRoleReview && EndedAt == nil`
  predicate; `backlog_service_triage.go:928`'s `hasActiveWorkSession` checks the *Work* role
  (a sibling-shaped predicate, not a duplicate), and `session/backlog_lifecycle.go:3351`'s
  `hasActiveSession` checks *either* role — a third, distinct predicate. Adding this in
  `server/mcp` would create a 2nd true duplicate of the exact predicate, not a 4th.
  **Remediation**: export `hasActiveReviewSession` → `HasActiveReviewSession` in
  `server/services/backlog_service_triage.go` and call `services.HasActiveReviewSession(...)`
  from `server/mcp` (Task 2.2.1a, Task 3.3.3a) instead of adding a local copy — `server/mcp`
  already pays the import cost for that package. If a future reviewer prefers not to make an
  internal helper part of `server/services`'s exported surface, a `session`-package home would
  also work (both `server/services` and `server/mcp` already import `session`), but *not*
  re-deriving the same one-liner a third time locally.

- [ ] **Epic 3.2 (`verifyGitHubRefExists`) — inconsistent testability seam versus the sibling
  `report_pr_created` tool in the same file.** `reportPRCreated`'s GitHub cross-check goes
  through an injectable func-value field on `backlogHandlers`
  (`verifyPRMatchesBranch func(ctx, owner, repo string, prNumber int, expectedBranch string) (bool, error)`,
  overridden directly in tests, `tools_backlog.go:99-102`). `report_duplicate`'s dispatcher
  (ADR-002 Option A) instead calls package-level `github.GetPR`/`GetIssue`/`GetCommit`
  directly with no injectable field, which forces its tests onto the global mutable
  `github.GhBaseURL` + `httptest.Server` swap (Task 4.2.1a) — a second, structurally different
  mocking mechanism coexisting with the first inside the same handler struct and test file.
  This is not the rejected `GitHubRefVerifier` interface (ADR-002 correctly rejects per-ref-kind
  polymorphism for a closed 3-case switch) — it's a narrower ask: give the *dispatcher itself* a
  swappable seam, the same shape `verifyPRMatchesBranch` already uses, so both GitHub-verification
  code paths in this file are tested the same way.
  **Remediation**: add `verifyGitHubRef func(ctx context.Context, ref *githubpkg.ParsedGitHubRef) error`
  as a field on `backlogHandlers`, defaulting to the real dispatcher in the constructor (mirroring
  `verifyPRMatchesBranch`'s wiring), and have `reportDuplicate` call `h.verifyGitHubRef(...)`.
  `verifyGitHubRefExists`'s internal switch body is unchanged — this only adds one level of
  indirection at the call site, consistent with the file's existing Strategy-as-function-type
  idiom (`design-patterns` skill) rather than introducing a new interface.

- [ ] **Task 1.2.3b/1.2.3c — the plan directs implementers to a test-injection pattern that does
  not exist in the referenced location, risking wasted implementation time or a third divergent
  mocking approach.** Both tasks say to "check `github/repos_test.go` or `client_test.go` for
  the exact test-server-injection pattern... this repo already has one, don't invent a second."
  Neither file exists (confirmed: `ls github/*_test.go` → `keychain_test.go`, `url_parser_test.go`,
  `user_pr_cache_test.go` only), and the `github` package has zero `httptest.Server`-based tests
  today. The real pattern the plan is presumably thinking of — swap `github.GhBaseURL` to an
  `httptest.Server` URL, restore via a deferred closure — lives in a *different* package,
  `server/services/backlog_github_rpc_test.go`'s `resetGhBaseURL` helper (confirmed at
  `server/services/backlog_github_rpc_test.go:19-22`), which is a legitimate, reusable
  precedent, just not where the plan says to look, and not yet used *inside* `github/*_test.go`
  itself.
  **Remediation**: correct Tasks 1.2.3b/1.2.3c to point at
  `server/services/backlog_github_rpc_test.go`'s `resetGhBaseURL(ts *httptest.Server) func()`
  as the pattern to replicate, and note explicitly that `github/commits_test.go`/
  `github/repos_pr_test.go` will be the *first* httptest-based tests inside the `github`
  package itself (not a "don't invent a second" situation — there's no first one there yet).

## Nitpicks

- `allowedSelfResolveSourceStatuses` (Task 2.1.1a) as a package-level `map[session.BacklogStatus]bool`
  is fine as designed — it's a read-only lookup table, never mutated after init, the same idiom
  as any Go dispatch/allow-list map; it is not the "package-level mutable-looking state" the
  plan's own doc-comment worries about. A small predicate function
  (`isAllowedSelfResolveSource(status session.BacklogStatus) bool`) would encapsulate it slightly
  better and centralize the two call sites' error-message string, but this is optional polish,
  not a defect.
- `BacklogItemPrecondition.Note` being populated by `request_review`/`report_duplicate` for the
  first time is **not** a new layering violation — the field's existing doc comment
  (`session/repository.go:556-558`) already documents exactly this use ("record why the
  transition happened, e.g. 'auto-reopened after FAIL verdict'"). The plan is the first caller
  to exercise a pre-existing, intentionally-general field as designed, not introducing an
  MCP-formatted string into a layer that wasn't expecting one.
- `github.GetPR` (new, existence-only) and `github.GetPRInfoCtx` (existing, rich `gh`-CLI-shaped
  detail) will coexist with similar names but different shapes/purposes. ADR-002 already flags
  and accepts this as a negative consequence; agreed it's not worth blocking on, but a one-line
  doc-comment cross-reference on each function pointing at the other would save a future reader
  a grep.
- `session.ItemSessionSummary.Role` is a plain `string` (not a typed `SessionRole` sum type),
  so `SessionRoleWork`/`SessionRoleReview` are untyped string constants rather than a Go
  sum-type-style enum (unlike `BacklogStatus`, which *is* a proper `type BacklogStatus string`
  newtype). This is pre-existing primitive obsession, unrelated to this plan's diff, and out of
  scope to fix here — noted for a future cleanup pass, not this feature.
- ADR-003's own stated risk-mitigation step ("grep check before merging:
  `grep -rn "TriggeredBySystem" web-app/src session server`") was run as part of this review:
  every hit is a *write* site (a call passing `TriggeredBySystem` as an argument) or a doc
  comment; no hit is a *read-side filter* comparing a stored `TriggeredBy` value against
  `"system"` to select behavior. The ADR's risk assessment is confirmed low as written — just
  flagging that Phase 5's implementer should still be the one to actually run this check (not
  rely on this review substituting for it), since new code between now and implementation could
  change the picture.
