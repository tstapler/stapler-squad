# ADR-001: Scan for threats at the orchestrator-wrapper level, not inside the `Build*` prompt builders

**Status**: Accepted
**Date**: 2026-08-06

## Context

`requirements.md` names four functions as the strict-scope wiring targets: `BuildSessionInitialPrompt`, `BuildHeadlessReviewPrompt`, `BuildHeadlessTriagePrompt`, `BuildHeadlessRetriagePrompt`. Read literally, that suggests scanning *inside* those functions (or immediately before each call to them).

Research surfaced two problems with that literal reading:

1. **research/pitfalls.md §3**: these four builders are called from up to 4 production + several test call sites each (grep-verified), all currently returning a plain `string` with no `error`. Adding scanning at each individual call site is exactly the kind of change a partial edit can silently miss — nothing catches a forgotten site at compile time, and the build still succeeds.
2. **research/features.md §4**: `session/pipeline_engine.go`'s `CachingPipelineEngine` only calls these `Build*` functions on `PipelineModeDefault`. On any custom `PipelineMode`, it instead calls `renderTemplate(rm.XxxTemplate, itemPlaceholders(item))` — a code path that never touches the named builders at all. Scanning inside/around the `Build*` functions would leave every custom-pipeline-mode item completely unscanned, defeating Requirements AC4 ("single source of truth").

Investigating the actual (non-test) call graph via grep found the fan-out is much smaller than "10-15 call sites" once test files are excluded: every real caller of the four `Build*` functions funnels through exactly one of six orchestrator/wrapper functions:

- `BacklogService.initialPromptFor` / `triagePromptFor` / `reviewPromptFor` (`server/services/backlog_service_triage.go`) — each has exactly one production call site of its own (`SpawnSessionFromItem`, `TriggerTriage`, `TriggerReReview` respectively), and each already either calls the `Build*` fallback directly (when `pipelineEngine` is nil) or delegates to `CachingPipelineEngine`'s matching method (which itself branches default-vs-custom internally).
- `TriggerTriage`'s retriage branch (bypasses `PipelineEngine` entirely by design — "refine the existing plan" is mode-independent, per an existing code comment).
- `WriteBacklogContextFile` (`session/backlog_commands.go`) — a separate direct caller of `BuildSessionInitialPrompt`, already returns `error`.
- `ReviewGateRunner.reviewPromptFor`'s caller, `Run` (`session/review_gate.go`) — the interactive-review-path equivalent of `BacklogService.reviewPromptFor`.

## Decision

Scan at these six orchestrator/wrapper call sites, using a single shared helper (`session.ScanItemForThreats`), **before** any of them dispatches to either the `PipelineModeDefault` `Build*` branch or the custom-mode `renderTemplate` branch.

Concretely:
- `BacklogService.initialPromptFor` / `triagePromptFor` / `reviewPromptFor` change from returning `string` to `(string, error)` — each has exactly one call site, so the ripple is contained and compiler-enforced.
- `ReviewGateRunner.Run` gets a new block mirroring `RunPreGateSecurityCheck`'s existing block-and-record-FAIL-verdict shape, rather than changing `reviewPromptFor`'s signature.
- `WriteBacklogContextFile` and the `TriggerTriage` retriage branch already return `error` (or are already inside a function that does) — the scan is added inline with no signature change.

## Consequences

- **Custom-pipeline-mode coverage is free.** Because the scan runs on the same raw `item.Title`/`item.Description` fields *before* `CachingPipelineEngine` branches on `PipelineMode`, and `itemPlaceholders` (the custom-mode template's only data source, `session/pipeline_engine.go:280-287`) never substitutes anything beyond `item.Title`/`item.Description`/`item.ID`/`item.RepoPath`, the custom-template path is covered without touching `PipelineEngine`'s interface or `CachingPipelineEngine`'s five methods at all. This resolves research/features.md §4's flagged gap as a side effect of the call-site choice, not as separate new code.
- **No re-scanning on retry passes.** `BuildTokenBudgetedPrompt` calls `BuildSessionInitialPrompt` up to 3 times internally (full → drop prior sessions → truncate description) to fit a token budget. Because the scan happens once, upstream of that retry loop, it never runs redundantly.
- **The four `Build*` functions keep their existing `string`-only signatures.** No change ripples into `pipeline_engine.go`'s five `PipelineEngine` methods, and none of the ~15 test call sites for the four builders need updating.
- **Smaller blast radius than it first appears.** Despite requirements.md naming four functions, this decision touches exactly 6 production call sites across 3 files (`server/services/backlog_service_triage.go`, `session/backlog_commands.go`, `session/review_gate.go`), not the four builders themselves.
- **Trade-off accepted**: `BacklogService`'s three wrapper methods gain an `error` return that a reader might expect to originate from I/O (as most Go `error` returns in this codebase do) rather than from a synchronous content-validation check. This is judged an acceptable, idiomatic cost — it is the same shape `RunPreGateSecurityCheck` already uses for the sibling secret-scanner, and Go's compiler-enforced error propagation is exactly the safety property research/pitfalls.md §3 asked for.
