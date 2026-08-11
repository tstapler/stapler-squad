# Architecture Review: ci-status-diff-viewer
**Date**: 2026-08-02
**Verdict**: CONCERNS

## Constitution Violations

`docs/adr/ADR-000-architecture-constitution.md` does not exist in this repository (checked
via direct file test). No constitution constraints apply — this section is N/A.

## Blockers

None. The plan's core design decisions are sound: ADR-001's rejection of a shared
`ApprovalGate`/`CIGate` interface between AC5 (`ApprovalService.ResolveApproval`) and AC6
(`matchesRule`) is a correct, well-evidenced application of
`.claude/rules/interface-pollution-checklist.md` ("speculative interface" smell — one
implementation, incompatible call shapes) rather than over-engineering. `Rule.RequireCIPassing`
and `RuleSpec.RequireCIPassing` are added as flat ANDed bool fields, matching the existing
`FilePattern`/`CommandPattern` convention in `pkg/classifier/classifier.go:343-369` exactly —
no new abstraction layer where none is needed. Proto field number (`require_ci_passing = 29`)
was verified against the live file (`proto/session/v1/types.proto:1076-1105`, last used field
is `tool_category = 28`) and is correct. `matchesRule`/`classifySingle` call-site line citations
(`pkg/classifier/classifier.go:502,506,511,550,679`) were verified against the live file and
are accurate — no missed call sites (including test files) that would break the signature
change in Task 1.1.1c.

## Concerns

- [ ] **Domain Glossary citation error — `HasAssociatedPR()` does not exist** (affects
  `requirements.md:54`, `research/architecture.md:106,196`, `research/pitfalls.md:77`, and
  `plan.md`'s Domain Glossary row for `HasAssociatedPR()`) — all four docs cite an existing
  `Instance` method `HasAssociatedPR()` at `session/instance.go:740` as the mechanism for
  gating AC7 ("no PR → no badge / unaffected by blocking rule"). The method actually at that
  location is named `HasGitHubPR()` (`session/instance.go:739-741`:
  `func (i *Instance) HasGitHubPR() bool { return i.Snapshot().GitHub.GitHubPRNumber > 0 }`),
  and it operates on `*Instance` via `Snapshot().GitHub`, not on `*InstanceData` — the type
  `session.Storage.FindInstanceDataByID` actually returns and the type both new gate call
  sites (Task 1.1.2a, Task 2.2.2a) use. The concrete task instructions don't literally invoke
  the wrong name (they inline `data.GitHubPRNumber > 0` on `*InstanceData` instead), so this
  won't break compilation directly, but it's a research citation propagated unverified across
  4 documents, and risks a subagent implementer trying to call a method that doesn't exist by
  that name. **Remediation**: correct the citation to `HasGitHubPR()` in all 4 docs, and note
  explicitly that the new `InstanceData`-based checks are intentionally inline rather than
  reusing `HasGitHubPR()`, because that method requires a live `*Instance`, which neither new
  call site holds (both work from `*InstanceData` via storage lookup).

- [ ] **DRY: "has a PR" check duplicated inline across two new call sites** (Task 1.1.2a in
  `server/services/approval_handler.go`, Task 2.2.2a in `server/services/approval_service.go`)
  — both independently write `data.GitHubPRNumber > 0` as a literal comparison. ADR-001
  correctly rejects sharing a full gate *interface* between these two call sites (different
  call shapes: boolean predicate under a read lock vs. RPC returning `connect.Error`), but that
  reasoning doesn't extend to the trivial "has an associated PR" boolean check itself.
  **Remediation**: add `func (d *InstanceData) HasGitHubPR() bool { return d.GitHubPRNumber > 0 }`
  to `session/storage.go` (mirroring the naming already established on `Instance`) and call it
  from both Task 1.1.2a and Task 2.2.2a instead of repeating the literal comparison.

- [ ] **Primitive obsession: no canonical constants for the CI-conclusion vocabulary** (Task
  1.1.1d's `ctx.CIStatus != "success"` in `pkg/classifier/classifier.go`, Task 2.2.2a's
  `data.GitHubCheckConclusion == "failure"` in `server/services/approval_service.go`) — the
  plan correctly avoids introducing a new `type CheckConclusion string` newtype/sum type for
  `GitHubCheckConclusion`/`ClassificationContext.CIStatus`, since that field's raw-string shape
  is already an established, wire-compatible convention shared by 3 existing fetchers
  (`github/client.go`, `github/user_pr_cache.go`, `session/backlog_plugin_github_prs.go`) and
  the frontend's `CheckConclusion` type — a full type-driven refactor here is out of this
  feature's scope. However, the plan's two new comparison sites add more literal-string
  matching on top of that existing pile with no single canonical source for the values, raising
  the surface area for a silent typo (e.g. `"Success"` vs `"success"`) to 5+ call sites.
  **Remediation** (low-cost, no wire-format change): define unexported string constants (e.g.
  `const ciConclusionSuccess = "success"`, `ciConclusionFailure = "failure"`) colocated with
  `Rule` in `pkg/classifier` and with the guard in `approval_service.go`, and reference them in
  both new comparisons instead of literal strings.

- [ ] **Pattern-selection inconsistency: new `CIStatusBadge.tsx` component vs. researched
  `VcsWidgetGithubRow.tsx` reuse** (Story 3.1.2 / Task 3.1.2a, `web-app/src/components/
  sessions/CIStatusBadge.tsx`) — `research/build-vs-buy.md` §4 recommends "extend
  `GitHubBadge.tsx`... rather than building a new standalone status-badge component."
  Separately, `research/architecture.md`'s Gap Analysis independently found
  `web-app/src/components/shared/vcs-widget/VcsWidgetGithubRow.tsx` already renders a
  color-coded CI-conclusion badge (`CI: {checkConclusion}` via `ciClassName`) and — verified
  directly in the file — already has a purpose-built `showPrLink?: boolean` prop
  (`VcsWidgetGithubRow.tsx:8-18`) specifically for suppressing the duplicate PR-number chip
  while still rendering the CI-conclusion span, which is exactly the scenario plan.md's own
  Pattern Decisions table cites as its reason for rejecting `GitHubBadge.tsx` reuse ("PR number
  chip... already shown elsewhere in the same `SessionDetailView`"). `research/architecture.md`'s
  explicit recommendation is: "rendering `VcsWidgetGithubRow` (or a slimmer CI-only variant) in
  the diff viewer's header — not building new fetch/cache/display logic from scratch." The
  plan's Pattern Decisions table discusses and rejects two alternatives (extend `GitHubBadge.tsx`
  in place; invent a new CSS idiom) but never discusses or rejects `VcsWidgetGithubRow.tsx` — the
  specific component its own research flagged by name and prop-for-prop matches the stated need
  — before committing to a brand-new sibling component (new file, new CSS-class mapping, new
  ARIA labels, new unit tests, new e2e wiring). This risks a third parallel "CI badge" rendering
  implementation (`GitHubBadge.tsx`'s tooltip-only surfacing, `VcsWidgetGithubRow.tsx`'s already-
  shipped badge, and now `CIStatusBadge.tsx`) for one domain concept, going forward as separate
  drift-prone implementations. **Remediation**: either (a) render
  `<VcsWidgetGithubRow data={fromSessionVcs(...)} showPrLink={false} />` in `DiffViewer` and
  extend it with a small optional `ciHref` prop to satisfy AC2's checks-page link requirement, or
  (b) if AC1's specific label/icon requirements ("Failing"/"Passing" text + `GitHubBadge.css.ts`
  variant classes vs. `VcsWidgetGithubRow.css`'s plainer `CI: {conclusion}` text) are judged
  different enough to warrant a new component, add that explicit comparison and rejection to the
  Pattern Decisions table so the choice is traceable rather than silently dropping the researched
  alternative.

## Nitpicks

- The plan cites exact source line numbers extensively (dozens of `file.go:NNN` references)
  across a 4-phase, ~20-task sequence where earlier phases modify some of the same files later
  phases also touch (e.g. `server/services/rules_service.go`, `approval_service.go`). Treat cited
  line numbers as "true as of research date," not literal, once any prior task in the same file
  has landed — a couple of task descriptions (e.g. Task 1.1.3c's "3 existing
  `SafePythonImportsOnly`-adjacent mapping sites") would benefit from a one-line reminder of this
  for whichever subagent executes out of the cited order.
