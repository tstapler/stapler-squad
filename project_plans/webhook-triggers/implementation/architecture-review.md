# Architecture Review: webhook-triggers
**Date**: 2026-08-06
**Verdict**: CONCERNS

`docs/adr/ADR-000-architecture-constitution.md` does not exist in this repo, so no constitution
check was applicable — skipped per instructions.

## Specific verification asks (from review brief)

- **WIP-limit admission gate closes for both paths?** Confirmed YES. Story 1.3.1 (Tasks
  1.3.1a-d) adds a consumer-defined `AdmissionGate` interface on `Scheduler`, wired to
  `BacklogService.Admit` at construction (`server/dependencies.go`), and Task 3.2.1a explicitly
  refactors `FireNow` to become a thin wrapper around the new `FireTrigger`, so both the
  legacy manual/`run_workflow` path and every new webhook/cron path share one admission check.
  Well-designed use of a narrow, consumer-owned interface (matches
  `.claude/rules/interface-pollution-checklist.md`).
- **Trigger-created sessions get the same/stricter approval gate, not elevated auto-approve?**
  Partially addressed, not verified. Requirements Goal 4 / FR states triggers "create
  sessions/backlog items through the existing approval/review path," and `RenderTriggerPrompt`
  wraps webhook payload data in an inert-data-block (prompt-injection defense at the *prompt
  content* level). But no story/task in the plan adds an explicit AC or test asserting that a
  `Workflow`-sourced (`WorkflowId` set) session cannot match a looser `ApprovalRule` than a
  manually created one, nor any regression test pinning "trigger-origin sessions get the
  default/strictest approval posture." See Concern C6 below.

---

## Blockers

- [ ] **Epic 5.2 (Task 5.2.1c) and Epic 6.2 (Tasks 6.2.1a/6.2.1b) — persistence-layer methods
  gain infrastructure/orchestration dependencies that create a probable Go import cycle, and
  violate Repository/Unit-of-Work boundaries.**
  Task 5.2.1c wires `CallbackDispatcher` (to live in `server/services/callback_dispatcher.go`)
  directly into `session/ent_repository_backlog.go`'s `TransitionBacklogItemStatus`. Task
  6.2.1a wires a synchronous call to `Scheduler.FireTrigger` (to live in
  `server/workflows/scheduler.go`) into the same method. Verified via `grep` that
  `server/services/*.go` and `server/workflows/scheduler.go` already import
  `github.com/tstapler/stapler-squad/session`, while `session/ent_repository_backlog.go`
  imports neither `server/services` nor `server/workflows` today. If either task is
  implemented as literally described — a direct call from the `session` package into
  `server/services`/`server/workflows` — it creates a compile-breaking import cycle
  (`server/services` → `session` → `server/services`). Beyond the cycle risk, this also
  violates the Repository pattern (PoEAA: a Repository is a collection abstraction, not an
  orchestrator of outbound HTTP calls or cross-aggregate session creation) and Clean
  Architecture's inward-only dependency rule — the persistence adapter should not depend on
  application-layer collaborators (`Scheduler`, `CallbackDispatcher`).
  **Remediation**: The plan's own Epic 1.3 already solved this exact class of problem for
  `AdmissionGate` — a narrow interface defined in the *consumer* package (`session`, since
  that's where `TransitionBacklogItemStatus` lives) satisfied structurally by the concrete
  `server/services`/`server/workflows` types, injected via a setter at `server/dependencies.go`
  wiring time. Apply the same pattern here (e.g. `session.EventDispatcher` /
  `session.ChainFirer` interfaces), or — the architecturally cleaner option — move the actual
  dispatch/chain-fire *call* out of `TransitionBacklogItemStatus` entirely and into whatever
  service/lifecycle layer already invokes it (`BacklogService` / `session/backlog_lifecycle.go`
  per the plan's own file list), having the repository method return only the transitioned
  item and a `previousStatus`/`didTransition` signal for the caller to act on. This also fixes
  the secondary Unit-of-Work concern: a CAS-transition method making outbound network calls
  (callback POST) or cross-aggregate session creation (chain-fire) inside/immediately-adjacent
  to its own persistence transaction mixes two different consistency boundaries.

## Concerns

- [ ] **Task 1.1.1a / 1.2.1a — `trigger_type` and `outcome` are `field.String(...)`, not
  `field.Enum(...)` or a Go sum type; illegal cross-type field combinations are enforced only
  by RPC-layer validation, not the type system.** ADR-001 explicitly accepts a wide `Workflow`
  table with several nullable, trigger-type-conditional columns as a known, reasoned tradeoff —
  that decision itself is not being second-guessed here. But its stated mitigation ("RPC-layer
  conventions... only trigger-type-relevant fields are surfaced/validated per trigger_type,"
  Task 3.1.1b) is a *validate* strategy, not a *parse* one (per `type-driven-design`'s
  Parse-Don't-Validate principle) — nothing prevents a `Workflow` row from being persisted with
  `trigger_type="webhook"` and `cron_expression` simultaneously set, and every consumer
  (`Scheduler`, both webhook handlers) has to re-derive "which fields actually matter here" by
  re-checking the string discriminator itself. **Remediation**: use ent's `field.Enum(...)` for
  both `trigger_type` and `outcome` (compile-time-safe against typos at minimum), and add a
  `ParseTriggerConfig(wf *ent.Workflow) (TriggerConfig, error)` boundary function returning a
  small sum type (`CronTrigger`/`GitHubPushTrigger`/`WebhookTrigger`/`ManualTrigger`) at the one
  point each `Workflow` row is read for firing/matching — so `Scheduler.FireTrigger` and both
  webhook handlers consume the proven variant instead of ad hoc string checks.

- [ ] **Task 2.1.1a/2.2.1b — `VerifyGitHubSignature(secret string, body []byte, sigHeader
  string) bool` takes a bare `string` for the secret, with no type-level distinction between
  the still-encrypted `WebhookSecretEncrypted` ciphertext and the decrypted plaintext secret
  actually needed for HMAC comparison.** This is exactly the "two primitives of the same type
  swapped" smell `type-driven-design` calls out — nothing stops a future call site from passing
  `wf.WebhookSecretEncrypted` (ciphertext) directly instead of the decrypted value. Given this
  is the plan's own-flagged "one piece of this feature with zero in-repo precedent" (security-
  critical HMAC verification), it deserves the extra type-level guard.
  **Remediation**: introduce a small `type plaintextSecret string` (or reuse whatever
  `session.EncryptToken`'s decrypt counterpart already returns) so `VerifyGitHubSignature`'s
  signature can't compile against an encrypted value.

- [ ] **Task 2.2.1d — `github_push` payload is unmarshaled into `map[string]interface{}` and
  field-extracted by string key path (`repository.full_name`, `ref`), despite GitHub's push
  event having a fixed, well-documented JSON schema.** The Pattern Decisions table's
  justification for `map[string]interface{}` templating ("payload key sets are open/arbitrary")
  is correct and well-reasoned for the **generic `webhook`** trigger type, but does not extend
  to `github_push`, whose schema is closed and known in advance — this is precisely the case
  Parse-Don't-Validate is for. A typo'd key path here degrades silently to a zero-value string
  (no compile error, no runtime error — just a wrong/empty match), which is worse than a parse
  failure. **Remediation**: for Task 2.2.1d specifically, `json.Unmarshal` into a small typed
  `githubPushPayload struct { Repository struct{ FullName string \`json:"full_name"\` }
  \`json:"repository"\`; Ref string \`json:"ref"\` }` before matching repo/branch; keep
  `map[string]interface{}` only for the template-rendering step and the generic webhook path,
  where the open-schema justification genuinely applies.

- [ ] **Story 1.1.1 / proto layer (not shown in detail in plan.md, but implied by "New
  RPCs/config touch the feature registry") — `TriggerType` is likely surfaced on
  `CreateWorkflow`/`UpdateWorkflow` proto messages as a raw `string` rather than a proto
  `enum`, inconsistent with this repo's own established precedent for exactly this kind of
  discriminator.** `.claude/rules/session-creation-registry.md` documents `SessionType` as a
  proto `enum SessionType { SESSION_TYPE_DIRECTORY = 1; ... }` for the structurally identical
  problem (a session-creation-mode discriminator). Introducing a second, string-typed
  convention for `TriggerType` at the API boundary loses client-side compile-time safety the
  existing convention already provides. **Remediation**: add `enum TriggerType` to
  `proto/session/v1/types.proto` mirroring `SessionType`'s shape, confirmed at Task 8.1.1a's
  consolidated proto pass if not addressed earlier.

- [ ] **Task 5.2.1a — `CallbackDispatcher.Dispatch`'s bounded-retry loop (`go func(){ for
  attempt := 0; attempt < 3; ...; time.Sleep(backoff) }()`) has no injectable clock or
  synchronous test hook.** Task 5.2.1e's own test plan ("assert the calling transition returns
  well before the dispatcher's 5s timeout elapses") only tests the non-blocking property, not
  the "3 attempts, bounded retry" property — asserting the latter deterministically requires
  either sleeping through real backoff delays in tests (slow, and prone to timing flakiness —
  exactly the class of bug `.claude/rules/fix-flaky-tests-dont-defer.md` warns about) or a
  seam to control timing. **Remediation**: accept a `sleepFunc func(time.Duration)` (or a
  minimal clock interface) as a constructor parameter, defaulting to `time.Sleep`, so tests can
  inject a no-op/instant version and assert attempt counts deterministically and fast.

- [ ] **No AC/task explicitly guards against trigger-created sessions receiving elevated
  auto-approve relative to manually created sessions**, per the prompt-injection risk the
  pitfalls research flagged (see "Specific verification asks" above). The plan's prompt-
  injection mitigation (inert-data-block framing in `RenderTriggerPrompt`) addresses the
  *prompt content* vector but not the *approval-rule matching* vector — e.g. an existing
  `ApprovalRule` that auto-approves broadly by session type/directory could apply equally to a
  webhook-triggered session with attacker-influenced prompt content, since nothing in the plan
  differentiates `ApprovalRule` evaluation by trigger origin. **Remediation**: add an explicit
  AC to Epic 1.3 or Phase 2/3 (wherever `FireTrigger`'s created session is finalized) stating
  trigger-origin sessions are subject to the same-or-stricter `ApprovalRule` set as manual
  sessions, plus a regression test asserting a broad auto-approve rule doesn't silently apply
  more permissively to a `WorkflowId`-attributed session than it would to a manual one.

- [ ] **`EventFilter`/`LabelFilter` (Task 1.1.1a) are plain optional strings with untyped
  "empty means no filter" semantics** — the distinction between "not configured" (any event/
  label matches) and "configured but requires an exact empty match" is carried only by
  convention/comment, not the type. Minor compared to the other findings, but a
  `type EventFilter struct{ enabled bool; value string }`-shaped value object (or simply
  `*string`, nil vs. empty-string being unambiguous) would remove the ambiguity Task 2.3.1c's
  matching logic currently has to hand-document ("no match when `label_filter` set but payload
  has no `labels` field").

## Nitpicks

- `DeliveryID` (dedup key) is a bare `string` throughout — low-risk given its narrow, purely
  internal use (SHA-256 digest or `X-GitHub-Delivery` passthrough), but a newtype would be
  free insurance against it being confused with `session_id` (`TriggerFireEvent` has both as
  adjacent string fields per Task 1.2.1a).
- `maxChainDepth`'s config-vs-constant question is correctly flagged as an Unresolved Question
  in plan.md already — no new finding, just confirming it should be resolved before Task
  6.3.1a, not deferred into implementation.
- Task 6.2.1c reuses `BuildSessionInitialPrompt(item, priorSessions)` for chain-prompt
  interpolation rather than inventing new summarization plumbing — good, matches the
  build-vs-buy research's explicit recommendation; no action needed, noted as a positive.
