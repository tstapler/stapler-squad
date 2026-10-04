# Requirements: session-classifier-pipeline

**Date**: 2026-09-11
**Type**: feature addition
**Complexity**: 3 — system design

## Problem Statement

Sessions in stapler-squad are tagged manually today (blue pills, added/removed via the Tag Editor
Modal per `docs/reference/tag-organization.md`). There is no automatic classification: a user
starting a session named `fix-flaky-tests` on branch `bugfix/pr-poller`, running `claude` in a
worktree, gets zero tags unless they type them in by hand. As the number of concurrent sessions
grows, manual tagging does not scale — grouping/filtering by Tag, Program, or Branch is only as
useful as the tags a user remembers to add, and nothing captures cases that need actual judgment
(e.g. "is this session doing test work vs. a refactor vs. a bug fix") which a human would have to
read the session to answer.

stapler-squad already has a mature, priority-ordered rule-evaluation engine
(`pkg/classifier.RuleBasedClassifier` / `Rule` / seed-vs-user `Source` provenance) built for
approval-rule classification of tool-use requests. This project generalizes that same engine to
also classify *sessions* into tags — reusing its rule/priority/source model rather than building a
second, parallel rule system — and adds an LLM fallback step for the classification judgment calls
regex cannot make.

## Baseline

Today: tags are 100% manual (Tag Editor Modal). No regex- or LLM-derived tags exist. A session's
`Category` field auto-migrates to tags on first load (existing backward-compat path), but nothing
else populates `Tags []string` automatically. Users wanting tag-based grouping/filtering must
remember to tag every session themselves, and re-tag on rename/re-branch.

## Users / Consumers

- End users of the stapler-squad web UI who group/filter sessions by Tag (see
  `docs/reference/tag-organization.md`'s grouping modes).
- The existing rule-editing surface (whatever supports `ApprovalRule` seed/user/generated rules
  today, e.g. the `rules-page-robustness` / `rules-ux-redesign` project lineage) — this project's
  user-editable tagging rules should fit into that same surface/pattern rather than a new one.
- Internal callers: `session/instance_actor_setters.go` (session create/mutation path) and a new
  background poller (mirroring `session/pr_status_poller.go`, `session/worktree_pr_poller.go`).

## Success Metrics

- A newly created or mutated session (name/branch/path/program change) has all applicable sync
  (regex/program/name/tag-dependency) rule tags applied automatically, with zero manual tagging,
  measured by: creating a session matching a seeded rule and observing the tag on the session
  immediately after creation without any user action.
- Sessions whose classification needs LLM judgment (i.e., no sync rule matched, or a rule
  explicitly defers to the LLM step) get a tag (or the `Unclassified` fallback tag) within one
  debounce cycle of the poller, without re-invoking the LLM on every render/poll when the
  session's classification-relevant content hash hasn't changed.
- Zero regressions to the existing `pkg/classifier` approval-rule (tool-use permission)
  classification path — this is a generalization, not a rewrite; existing `ApprovalRule` /
  `RuleBasedClassifier` behavior for tool-use permission classification must remain unchanged and
  fully covered by existing tests.

## Appetite

Large (3–6 weeks)

*(Scope must fit the appetite. If it doesn't fit, cut scope — do not move the deadline.)*

## Constraints

- Must generalize `pkg/classifier/classifier.go`'s existing `RuleBasedClassifier`, `Rule`, and
  `Source` (seed/user/generated) types/provenance model — not build parallel infrastructure for
  session tagging.
- Sync rules (regex/program/name matching, and rules chained on already-applied tags) must run
  eagerly on session create/mutation, to a fixpoint (i.e., re-run until no new sync rule fires, so
  tag-dependency chains resolve fully before returning).
- The LLM fallback step must run as a separate, decoupled, debounced background poller — mirroring
  `session/pr_status_poller.go` / `session/worktree_pr_poller.go`'s structure — not inline on the
  session-mutation hot path.
- LLM results must be cached by content hash so an unchanged session is never re-classified by the
  LLM on every render/poll cycle.
- Tagging rules must be user-editable from day one (persistence + CRUD, following the
  `ApprovalRule` seed/user/generated pattern) — not seed-only/hardcoded.
- Must follow the existing tag/rule conventions documented in `docs/reference/tag-organization.md`
  and the `stapler-squad-rules` skill.

## Non-functional Requirements

- **Performance SLO**: Sync rule evaluation to fixpoint on session create/mutation must not
  perceptibly slow down session creation (no specific p99 number given — "not specified" beyond
  "must not become the visible bottleneck in session creation").
- **Scalability**: Must handle the current multi-session-per-workspace scale used interactively
  (tens of concurrent sessions); no specific volume target given beyond that.
- **Security classification**: Internal — session names/branches/paths are not secret, but LLM
  fallback calls should not leak full session content (diffs, transcripts) beyond what's needed to
  classify; scope the LLM prompt to session metadata unless research determines richer content is
  needed and safe.
- **Data residency**: Not applicable — local self-hosted service.

## Scope

### In Scope

- Generalizing `pkg/classifier.Rule` / `RuleBasedClassifier` (or introducing a parallel evaluation
  path within the same package/types) to support session-tagging rules: regex match against
  session name, branch, path, program; rules keyed on already-applied tags (chained/dependent
  rules); ordered/priority evaluation to a fixpoint.
- Wiring eager sync-rule evaluation into session create/mutation (`session/instance_actor_setters.go`
  or equivalent), running to a fixpoint.
- A new debounced background poller for the LLM fallback step, modeled on
  `pr_status_poller.go`/`worktree_pr_poller.go`, using the existing headless-pool LLM invocation
  pattern (`headless.GenerateSessionCompletionNarrative`-style) with a cheap model (e.g. Haiku).
- Content-hash caching so the LLM step does not re-run per render/poll for unchanged sessions.
- Applying an `Unclassified` fallback tag when the LLM call fails or times out.
- User-editable persistence (ent schema, CRUD, provenance) for tagging rules, following the
  `ApprovalRule` seed/user/generated pattern.
- Seed tagging rules covering common cases (by program, common branch-naming conventions, etc.)
  as a starting set, consistent with `SeedRules()`'s existing style.
- Test coverage for: fixpoint convergence (including cyclic/non-terminating rule chains), existing
  approval-rule classification regression coverage, LLM-poller debounce/caching behavior, and
  LLM-failure fallback tagging.

### Out of Scope

- Any change to the approval-rule (tool-use permission) *decision semantics* — this project reuses
  the engine's types/evaluation machinery, not its risk/approval domain logic.
- A rules-management UI beyond what's needed to exercise CRUD (a full redesigned rules page is
  tracked separately per the `rules-ux-redesign` project lineage — this project should integrate
  with, not duplicate, that surface).
- Retroactive bulk re-classification of all historical sessions (only sessions created/mutated
  after this ships, plus whatever the LLM poller picks up going forward, are in scope) — a
  backfill job is a candidate for a fast-follow, not this pass.
- Any non-Anthropic/non-Haiku-class LLM backend selection UI — a single configurable cheap-model
  fallback is sufficient; multi-provider selection is not required.

## Rabbit Holes

- **Fixpoint termination**: tag-dependency-chained rules can form cycles (rule A adds tag X when Y
  present, rule B adds tag Y when X present) or rules whose conditions never stabilize. Needs an
  explicit iteration cap and/or cycle detection, not just "loop until no change."
- **Generalizing `Rule`/`RuleBasedClassifier` without breaking `ApprovalRule` semantics**: the
  existing struct is deeply tied to `PermissionRequestPayload`/`ClassificationContext`/
  `ClassificationResult` (tool-use domain). Making it generic enough for session-tagging inputs
  (name/branch/path/program/tags) without a leaky abstraction is real design work — likely needs a
  shared "rule matching" core factored out from the tool-use-specific `Classify` orchestration,
  not a single unified `Rule` struct trying to serve both domains' fields.
- **Content-hash definition for LLM caching**: what counts as "content" (name+branch+path+program
  only? diff summary? transcript excerpt?) directly determines both cache-hit rate and
  classification quality — needs to be pinned down in planning, not left implicit.
- **Debounce/poller lifecycle interaction with existing pollers**: `pr_status_poller.go` and
  `worktree_pr_poller.go` patterns need to be checked for shared infrastructure (a common poller
  base/registration point) vs. copy-paste — reuse vs. duplicate is a real decision.
- **User-editable rule persistence schema**: mirroring `ApprovalRule`'s ent schema for a
  differently-shaped rule (tag-output instead of decision-output) may not be a drop-in reuse of
  the existing ent types.

## Alternatives Considered

- **Keep tagging fully manual**: rejected — doesn't scale, is the explicit baseline problem.
- **Build a standalone tagging-rule engine independent of `pkg/classifier`**: rejected per explicit
  user direction — would duplicate priority/source/provenance machinery already proven out for
  approval rules.
- **Always classify via LLM (skip regex sync rules)**: rejected — slower, costs money per session
  mutation, and most classification (by program, branch-naming convention, tag dependency) doesn't
  need a model call at all.
- **Run the LLM step inline on session mutation instead of a separate poller**: rejected per
  explicit user direction — would put LLM latency on the session-mutation hot path.

## Feasibility Risks

- Whether `pkg/classifier.Rule`'s tool-use-specific fields (`ToolName`, `ToolPattern`,
  `CommandPattern`, `Criteria`, `RequireCIPassing`, etc.) can cleanly coexist with new
  session-tagging fields in one struct, or whether the right design factors out a shared base —
  this determines a large fraction of the implementation shape and must be resolved in Phase 3
  planning before coding starts.
- Availability and interface of a "cheap model" headless invocation path — `headless.
  GenerateSessionCompletionNarrative` is the closest existing precedent but generates prose, not a
  tag decision; may need a new headless helper for structured tag-classification output.
- Ent schema/migration risk in adding a new user-editable rule type — must confirm this repo's
  `make ent-gen` workflow (per repo CLAUDE.md) is followed correctly and that generated code is
  not committed.

## Observability Requirements

- Log (structured, `slog`, per repo convention) when: a sync rule fires and its resulting tag, a
  fixpoint iteration cap is hit (this indicates a rule-authoring bug and should be visible), the
  LLM poller runs and its outcome (tag applied / cache hit / failure→Unclassified), and LLM call
  cost/latency (mirroring `session_summary_service.go`'s cost-tracking pattern for narrative
  generation).
- No new oncall alert required — this is a non-critical background enhancement; standard log-based
  debugging (per `docs/how-to/debug-with-logs.md`) is sufficient.

## Risk Control

- No feature flag required for the sync rule-engine changes — regression risk is covered by
  requiring the existing `pkg/classifier` test suite to remain green (per the Success Metrics'
  zero-regression requirement).
- The LLM poller should be structured so it can be disabled independently (e.g. a config toggle or
  simply not registering the poller) without affecting sync-rule tagging, in case the LLM step
  proves noisy or costly in practice — rollback is "stop registering the poller," not a full
  revert.

## Open Questions

- Exact shape of the shared "rule core" vs. tool-use-specific vs. session-tagging-specific fields
  on `Rule` — resolve in Phase 3 (Plan/architecture review).
- Which headless/LLM invocation path to reuse or extend for structured tag output — resolve in
  Phase 2 research.
- Where user-editable tagging rules should be surfaced in the existing rules UI/API surface —
  resolve in Phase 2 research (survey `rules-page-robustness`/`rules-ux-redesign` project lineage
  and current API).
- Precise content-hash input definition for LLM-result caching — resolve in Phase 3 planning.
