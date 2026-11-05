# Architecture Review: github-issue-cleanup
**Date**: 2026-11-03
**Reviewer**: Architecture review pass
**Verdict**: BLOCKED

> This review supersedes the earlier draft that predated Task 1.1.2a-timeout and
> Task 1.1.2c's explicit "replace early-return block" language in the plan.

## Constitution Violations
_(No ADR-000 found in `docs/adr/`; no hard-constraint violations identified.)_

---

## Blockers

- [ ] **Task 1.1.2b vs 1.1.2c — `resolveCloseTarget` return-signature contradiction**
  `resolveCloseTarget` is defined in Task 1.1.2b with **4** return values:
  `(plugin externalIssueCloser, cfg session.PluginConfig, externalID string, ok bool)`.
  Task 1.1.2c's Notes section then says to widen it to **5** return values:
  `(plugin, cfg, externalID, closeLabel string, ok bool)`.
  The two tasks are sequential in the dependency graph (1.1.2c depends on 1.1.2b),
  so this won't cause a parallel-build conflict — but the plan contains two
  contradictory signatures for the same function. An implementer who follows
  Task 1.1.2b literally ships a 4-value function; Task 1.1.2c's pseudocode then
  fails to compile (the call site at step 4 destructures into 5 variables).
  **Remediation:** Specify the final 5-value signature directly in Task 1.1.2b's
  code example and remove the "update `resolveCloseTarget`'s signature" note from
  1.1.2c (turn it into a reminder that the fifth value is already there).

---

## Concerns

- [ ] **Task 1.1.2c — Linear/JIRA fallback doubles `GetItemSourceByID`**
  `resolveCloseTarget` Path A calls `storage.GetItemSourceByID` internally and
  returns `!ok` when the plugin type-asserts to `externalIssueStateUpdater` instead
  of `externalIssueCloser` (and in several other failure modes). `handleForwardSyncClose`
  then immediately calls `storage.GetItemSourceByID` a second time for the Linear/JIRA
  fallback. Because `resolveCloseTarget` doesn't return the fetched source, the
  re-fetch is structurally forced for every Linear/JIRA item's done-transition.
  The existing code is a single lookup; the refactored version is two for the same
  source row. The concern is not O(N) — it's O(1) per event — but it also masks
  the distinction between "Path A type-assert fail" (need fallback) and "Path A
  ForwardSyncEnabled false" (no fallback needed), causing an avoidable re-fetch
  in the disabled case too.
  **Recommendation:** Add a typed sentinel error (e.g. `errNotACloser`) or return
  the fetched `*ItemSource` as an extra out-param from `resolveCloseTarget`
  specifically for the type-assert-fail case, so `handleForwardSyncClose` can
  reuse it without a second DB round-trip. Alternatively, absorb the
  `externalIssueStateUpdater` path into `resolveCloseTarget` as Path C (a close
  analogue of `resolveProgressTarget`'s clean separation), removing the fallback
  block entirely.

- [ ] **Task 1.1.1a + Task 1.1.2b — `closingKeywordFor` and `resolveCloseTarget` Path B
  classify `ExternalURL` with divergent methods**
  `closingKeywordFor` uses `strings.Contains(url, "/issues/")` to decide whether
  to emit `"Closes"`. `resolveCloseTarget` Path B uses
  `githubpkg.ParseGitHubRefWithHosts(item.ExternalURL, nil)` and checks
  `ref.Type != githubpkg.RefTypeIssue`. Both read the same `item.ExternalURL`
  field. For well-formed GitHub URLs the results are identical. For a malformed
  or non-canonical URL that one accepts and the other rejects, the PR body will
  carry a `Closes <url>` keyword but `resolveCloseTarget` will return `!ok` —
  the issue won't be closed but GitHub will still try to auto-close on merge.
  The inverse (resolved close, no keyword) is less likely but equally silent.
  **Recommendation:** Have `closingKeywordFor` call `ParseGitHubRefWithHosts`
  (passing `nil`) and check `ref.Type`, matching the resolution logic exactly.
  Since `closingKeywordFor` is a pure function in `session/backlog_context.go`,
  it can import the `github` package (no circular dependency — `server/services`
  already does this, and `session/backlog_plugin_github.go` already imports it).
  If the parse overhead is a concern, inline the call in `buildFallbackPRBody`
  and `buildSessionInitialPrompt` rather than adding a helper with different
  semantics from the resolver.

---

## Nitpicks

- **Task 1.1.3a — inline `strings.Contains` re-duplicates `closingKeywordFor`'s logic.**
  The block uses `closingKeywordFor(item.ExternalURL)` to get `kw`, then immediately
  re-checks `strings.Contains(item.ExternalURL, "/issues/")` and
  `strings.Contains(item.ExternalURL, "/pull/")` to pick the fact-line label
  ("Linked GitHub Issue:" vs "Linked GitHub PR:"). Use `kw == "Closes"` and
  `kw == "Related:"` to branch on the already-computed value, keeping the
  classification in one place.

- **`&session.GitHubIssuesPlugin{}` zero-value allocation in Path B** (both
  `resolveProgressTarget` line 198 and the new `resolveCloseTarget`) has no structural
  guard. If `GitHubIssuesPlugin` gains instance-level state (e.g. a cached HTTP
  client), Path B will silently use a nil/zero instance. The pattern is established
  in `resolveProgressTarget` already — just add a one-line comment at the
  `resolveCloseTarget` call site: `// zero-value is valid: all state comes from PluginConfig`.

- **`forwardSyncCloseCommentWithPR(current.ClaimingHostID, ...)` on Path B.**
  For MCP-imported items `ClaimingHostID` is likely `""`, so the close comment
  won't include a host suffix even though the host is unambiguously known from
  `ExternalURL`. Cosmetically inconsistent with Path A. Consider deriving the
  host from the parsed `ref.Host` and passing it to `forwardSyncCloseCommentFor`
  when `ClaimingHostID` is empty on Path B.
