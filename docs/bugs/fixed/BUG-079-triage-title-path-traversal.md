# BUG-079: Path traversal via unsanitized LLM-controlled triage result Title [SEVERITY: High]

**Status**: ✅ Fixed
**Discovered**: 2026-08-16
**Fixed**: 2026-08-16
**Impact**: Security — arbitrary-file-read primitive if a backlog item's content can steer the triage LLM's JSON `title` output (this repo already treats triage-time prompt injection as a realistic threat, see the open backlog item on that risk). Also affected a git commit message and (defense-in-depth, not independently exploitable — see below) a branch name.

## Actual Root Cause

`session.ParseHeadlessTriageResult` (`session/backlog_triage.go:229-257`) unmarshals the triage LLM's own JSON output directly into `HeadlessTriageResult`. It caps `Tasks` length but never validates or sanitizes `Title` — that field is LLM-controlled end to end.

`BacklogService.TriggerTriage`'s goroutine (`server/services/backlog_service_triage.go`) used `result.Title` raw in three places:

1. `pap = filepath.Join(triageWorkDir, "project_plans", result.Title, "implementation")` (was line 2815) — persisted as `PlanArtifactsPath` on the backlog item and later opened by `readPlanFile` (`session/backlog_review.go`) to render plan content in the review UI. A title containing `../` sequences resolved `pap` outside `triageWorkDir`, giving an arbitrary-file-read primitive.
2. `triageWorktree.CommitChanges(fmt.Sprintf("chore(sdd): planning artifacts for %s", result.Title))` (was line 2766) — an unvalidated string persisted into git history via go-git (no shell involved, so no injection, but still an unbounded/uncontrolled commit message).
3. `retitleTriageWorktreeToFinalBranch(itemID, itemRepoPath, result.Title, triageWorktree)` (was line 2769) — on inspection this call site was **not** independently vulnerable: it passes `title` straight into `backlogWorkBranchSlug(repoPath, title)`, which already wraps it in `slugify(...)` before using it as a branch name. The raw title never reaches `wt.RenameBranch` directly.

The SDD triage prompt (`session/pipeline_mode_seed.go` Step 3, `sddInitialPromptTemplate`) instructs the LLM to use "the same short kebab-case name you used for `project_plans/<name>/` above" as the `title` field, and a `slugify()` helper already existed in the same file (used by `triageShortTitle`/`backlogWorkBranchSlug` for branch-name construction) — so under compliant LLM behavior, `slugify(result.Title) == result.Title` (idempotent on an already-kebab-case string), meaning the fix is a no-op for well-behaved output and only changes behavior for adversarial or malformed titles.

## Fix

Added `sanitizeTriageTitle(title, itemID string) string` next to the existing `slugify` helper (`server/services/backlog_service_triage.go`) — reuses `slugify` (already proven safe for file paths: strips everything outside `[a-z0-9-]`, which rules out `..` and any path separator) and falls back to `"item-" + itemID[:8]` when sanitization collapses the title to empty (all-punctuation or blank titles).

Computed once, immediately after `result.Feedback = feedback` and before any of the three use sites, and reused (`sanitizedTitle`) at all three: the `pap` `filepath.Join`, the `CommitChanges` message, and the `retitleTriageWorktreeToFinalBranch` call — rather than three separate ad hoc fixes. This also means site 3 (already indirectly safe via `backlogWorkBranchSlug`'s own `slugify` call) now receives an already-sanitized value, which is harmless (slugify is idempotent) and keeps all three call sites consistent.

Regression tests added in `server/services/backlog_service_triage_test.go`:
- `TestSanitizeTriageTitle_should_NeverProducePathEscapingValue_When_TitleContainsTraversalSequences` — table of `../`, backslash, and separator-only payloads; asserts structurally (reproduces the actual `filepath.Join` the production code performs and checks the result stays under `triageWorkDir` via `strings.HasPrefix`), not just that a specific crafted string is absent.
- `TestSanitizeTriageTitle_should_FallBackToItemIDSlug_When_TitleSanitizesToEmpty` — empty/whitespace/punctuation-only titles.
- `TestSanitizeTriageTitle_should_PreserveReadableSlug_When_TitleIsBenign` — happy-path slug output is unchanged for normal titles.

Verified via `go test ./server/services/... -run TestSanitizeTriageTitle -v` (all pass) and the full `make build && make test` (all packages pass, including `server/services` and `session`) and `make lint` (0 issues).

## Phase D — Reflect

**Classification**: API Contract Gap — `ParseHeadlessTriageResult`'s contract implicitly assumed the model would only ever emit compliant kebab-case output (the prompt says so), but the function sits at a trust boundary (parsing arbitrary LLM JSON output, which this repo already treats as attacker-influenceable via prompt injection) and enforced no invariant on `Title` before it flowed into a path-construction and commit-message context three call sites away.

**Earliest enforcement point**: A unit test (the regression tests added here) is the earliest practical level for this specific case — the unsafe value only becomes dangerous at the point of use (`filepath.Join`), and Go's type system has no way to express "sanitized string" vs. "arbitrary string" without a wrapper type, which would be a heavier change than this bug's blast radius justifies for a single hot call site. No compile-time or lint-level catch was available here.

**Recurring-shape check**: This is the first bug filed against `ParseHeadlessTriageResult`'s output specifically, but it is an instance of a broader recurring shape in this codebase: **an LLM-controlled string field, parsed from JSON, flows into a filesystem or git operation without validation at the trust boundary.** `applyTriageResultToUpdate` shows the correct pattern already in use for two other fields on the same struct (`Priority` range-checked, `ItemCategory` checked against `IsValidBacklogCategory` before being applied) — `Title` was the one field on `HeadlessTriageResult` that reached a filesystem/git sink without an equivalent check. No other current field on `HeadlessTriageResult` reaches a path or shell context unsanitized (confirmed by grep for `result\.` across `backlog_service_triage.go`), so no broader lint rule was added — if a future field is added to `HeadlessTriageResult` that flows into a path, git operation, or shell command, apply the same sanitize-once-at-the-boundary pattern (`sanitizeTriageTitle` is now the concrete precedent to point to).

## Related Tasks

None — self-contained fix, scoped to `server/services/backlog_service_triage.go` and its test file. A separate, parallel fix-bug run addressed an unrelated `classifyHeadlessCallError` issue in the same file in an isolated worktree; this diff does not touch remediation-gating logic, the reconciliation loop's control flow, or that function.
