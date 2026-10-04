# ADR-001: LLM fallback invocation uses `session/headless`, not `anthropic_client.go`

**Status**: Accepted
**Date**: 2026-09-11
**Context**: session-classifier-pipeline, Phase 3 planning

## Context

Two research agents (research/stack.md, research/build-vs-buy.md /
research/architecture.md) independently investigated how the new session-tag
classification poller should call an LLM, and reached opposite recommendations:

- **stack.md** initially recommended `server/services/anthropic_client.go`
  (`AnthropicAIClient`, direct call to `api.anthropic.com/v1/messages`,
  `anthropicModel = "claude-haiku-4-5-20251001"` already the default model),
  on the grounds that its sibling `RulesService.GenerateSuggestedRule` already
  has a working JSON-prompt → `parseSuggestions` hand-parse precedent
  (`server/services/rules_service.go:909-951`, `:1027-1054`).
- **build-vs-buy.md** / **architecture.md** recommended extending
  `session/headless/features.go` with a new `GenerateSessionTags`-style
  function, on the grounds that this package already has a *closer*
  JSON-returning precedent (`SummarizeBacklogItem` at
  `session/headless/features.go:234-250`, `GenerateAcceptanceCriteria` at
  `:254-269` — both call `pool.CallBlocking(...)` then `json.Unmarshal`), and
  that stack.md's initial claim "no structured/JSON output mode exists in
  this package" was simply wrong (corrected in build-vs-buy.md itself).

This ADR makes the final call, since the two research passes conflict.

## Decision

Use **`session/headless`** (`*headless.Pool` / `headless.PoolClient`), not
`server/services/anthropic_client.go`.

Concretely: add a new `FeatureKeySessionTagging` constant and a
`GenerateSessionTags(ctx context.Context, pool headless.PoolClient, meta SessionTaggingContext, vocabulary []string) (tags []string, cost float64, err error)`
function to `session/headless/features.go`, following the exact
`pool.CallBlocking(...) → json.Unmarshal` shape of `SummarizeBacklogItem` /
`GenerateAcceptanceCriteria`, with `CallOptions{Model: "haiku"}` (already a
plumbed-through per-call override per `session/headless/caller.go:19-30`).

## Rationale

1. **Already wired, zero new credential surface.** `server/dependencies.go`
   constructs a `*headless.Pool` unconditionally at startup (nil only if the
   `claude` binary isn't found, `server/dependencies.go:710-725`) and already
   exposes it through `ServerDependencies.HeadlessPool` via the existing
   `warren` DI pattern used for `PRStatusPoller` and friends. The new
   `SessionTagClassificationPoller` can take the exact same
   `*headless.Pool` dependency with no new wiring category.
   `AnthropicAIClient`, by contrast, requires a resolved `Credential` (API
   key or OAuth token) that isn't provisioned anywhere in the
   session/background-poller path today — it's only wired into
   `RulesService` for an explicit, interactive, user-invoked "suggest a
   rule" action.
2. **Local-first framing fits better.** stapler-squad's whole premise
   (README: "Run it with `ssq`, then open http://localhost:8543") already
   depends on a local `claude` CLI install for every session it manages —
   requiring the same binary for tag classification adds no new external
   dependency. Requiring `ANTHROPIC_API_KEY` specifically for this one
   background feature would be a new, narrower requirement that a
   self-hosted user might not have configured (they may be on a Claude
   subscription with no raw API key, the OAuth-credential path `AIClient`
   supports for *interactive* use but not obviously reachable from a
   background poller with no request context).
3. **Session-reuse fits the workload.** `headless.Pool` keys sessions by
   `FeatureKey` and reuses them for prompt-cache locality
   (`session/headless/pool.go`). A poller issuing many structurally-identical
   "classify this session's metadata into tags" calls over time is exactly
   the repeated-similar-prompt workload prompt-cache reuse is for. A raw
   per-call HTTP POST via `AnthropicAIClient` has no equivalent reuse
   mechanism.
4. **JSON-output precedent already exists in-package**, contra stack.md's
   initial (self-corrected) finding — `SummarizeBacklogItem` and
   `GenerateAcceptanceCriteria` are two existing, tested examples of
   `pool.CallBlocking` + `json.Unmarshal`, so the new
   `GenerateSessionTags` is additive, not a new pattern.
5. **Cost/latency tracking precedent matches the observability requirement.**
   `GenerateSessionCompletionNarrative` (`session/headless/features.go:345-364`)
   already returns `(string, float64, error)` with cost threaded through a
   `func(usd float64)` sink, matching this project's Observability
   Requirements ("LLM call cost/latency, mirroring
   `session_summary_service.go`'s cost-tracking pattern").

## Consequences

- The poller depends on `*headless.Pool`/`headless.PoolClient` being
  non-nil; when the `claude` binary isn't installed, `HeadlessPool` is `nil`
  and the poller must not be started (same "don't register it" disable path
  already required by requirements.md's Risk Control section) — every
  session falls back to `Unclassified` tags never being resolved by LLM,
  but sync rules still work unaffected.
- Testing uses the existing `headless.PoolClient` interface to inject a fake
  (per `session/headless/client.go:7-9`), consistent with
  `deterministic-fast-tests`/`fix-flaky-tests-dont-defer` — no real subprocess
  or network call in unit tests.
- If a future need arises for classification without a local `claude`
  install (e.g. a headless server deployment with no CLI), revisit this ADR;
  out of scope for this project per requirements.md's explicit non-goal
  ("Any non-Anthropic/non-Haiku-class LLM backend selection UI").
