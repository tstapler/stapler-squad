# ADR-002: Record a claim inside `Storage.CreateBacklogItem`, not at each of its callers

## Status
Accepted

## Context

Requirements goal 4 says the dedup system must "work for any backlog item source (manual
import, NL-created, forward/backward-synced via `ItemSource`), not just the GitHub-issue-import
path." `research/features.md` §1b/§"Edge cases" confirms three independent code paths create
backlog items today with no shared helper: `ImportGitHubIssue`
(`server/services/backlog_service_sync.go:296`), the `ItemSource` sync loop
(`session/backlog_sync.go:346`), and NL/manual creation
(`server/services/backlog_service_lifecycle.go:218`, `server/services/backlog_service_chat.go`).
`research/features.md` frames this as a choice the maintainers must make: three call sites kept
in sync (the same registry-drift risk `docs/reference/session-creation-registry.md` /
`docs/reference/feature-testing-registry.md` exist to guard against elsewhere), or pushed down
into one shared function.

Verifying directly against this checkout (not assumed from research): every one of those three
paths, plus `server/mcp/tools_backlog.go`'s `import_github_issue`/manual-create MCP tools and
the debug seed/mutate handlers, calls exactly one function —
[`session.Storage.CreateBacklogItem`](../../../session/storage.go#L906), which delegates to
[`EntRepository.CreateBacklogItem`](../../../session/ent_repository_backlog.go#L425). There is
no fourth path that bypasses it (`grep -rn "CreateBacklogItem" --include=*.go .` outside tests
shows every call site funnels through `storage.CreateBacklogItem` or `h.storage.CreateBacklogItem`
on the same `*session.Storage`).

## Decision

**`Storage.CreateBacklogItem` records the claim itself, once, whenever `data.ExternalURL != ""`,
after the row is durably created.** No caller (`ImportGitHubIssue`, `SyncLoop`, the
`CreateBacklogItem` RPC, MCP tools, debug handlers) needs its own call to record a claim.

**Correction (post-review):** the call is made via a `session`-package-owned port,
`session.ClaimRecorder` (`RecordClaim(ctx, record ClaimRecord) error`), wired in through
`Storage.SetClaimRecorder(...)` — **not** via `crossHostClaimChecker`, which is defined in
`server/services` and cannot be called from `session.Storage` without an import cycle
(`server/services` already imports `session`). This mirrors the existing
`ItemChangePublisher`/`CallbackDispatcher` precedent exactly: both are ports defined in package
`session` (`session/backlog_item_change.go:84`, `session/callback_dispatcher.go:15`), with
`Storage.SetItemChangePublisher`/`Storage.SetCallbackDispatcher`
(`session/storage.go:284-292`) wiring in concrete implementations from
`server/dependencies.go:836,1076` — dependency inversion for the same "session-layer code needs a
services-layer capability" shape this decision has. See `plan.md` Story 1.3.1 for the concrete
task breakdown; `crossHostClaimChecker` (`server/services/claim_checker.go`) is narrowed to
`CheckClaim` only and never calls `RecordClaim`.

`ExternalURL != ""` is the same gate `GetBacklogItemByExternalURL`'s dedup check already uses
(`session/ent_repository_backlog.go`'s doc comment, cited in `research/features.md` §1a) — an
item with no external URL (a genuinely local-only manual item with nothing to dedupe against
across hosts) never enters the claim index at all, matching the requirements' claim-index key
choice (`ExternalURL`, not a synthetic key).

## Alternatives Rejected

**Instrument each of the three creation call sites individually.** Rejected: this is exactly
the multi-touchpoint registry-drift risk this codebase already has two dedicated `docs/reference/
*-registry.md` documents to guard against for *other* features (session creation modes,
Omnibar). A fourth backlog-item creation path added in the future (plausible — this repo adds
new `ItemSource`s and MCP tools regularly) would silently skip claim recording with no compile-
time or test signal, reproducing the exact "which call sites need to know about this" bug class
`research/features.md`'s edge-case section flags. Pushing the write down into the one function
every path already shares makes the omission structurally impossible instead of merely
documented.

**Record the claim asynchronously (a listener on backlog-item-created events) instead of inline
in `CreateBacklogItem`.** Considered and rejected for v1: no existing event-listener
infrastructure fires specifically on backlog-item creation (the closest analog,
`BacklogLifecycleListener`, reacts to session-exit/status-transition events, not creation), so
this would mean building new event-plumbing whose only consumer is this feature. `RecordClaim`
against the local `*session.ClaimIndex` is a fast, in-process, best-effort write (matching
`HostAdvertiser.BroadcastOnce`'s own "best-effort, an unreachable peer is skipped" philosophy)
— no justification for deferring it out of the synchronous creation path.

## Consequences

- `Storage.CreateBacklogItem`'s signature/behavior gains one more responsibility (best-effort
  local claim recording + gossip broadcast trigger) beyond the ent write it already does. This
  is judged acceptable rather than a new God-Function risk because the added call is a single,
  narrow interface method (`session.ClaimRecorder.RecordClaim`, see `plan.md`'s Domain
  Glossary/Pattern Decisions and Story 1.3.1) injected at construction via
  `Storage.SetClaimRecorder`, not inline claim-index logic — the same shape
  `Storage.SetItemChangePublisher`/`Storage.SetCallbackDispatcher` already establish for
  session-layer-needs-services-layer-capability seams elsewhere in this codebase.
- A failure to record a claim (e.g. `ClaimIndex` file lock timeout) must not fail the backlog
  item creation itself — `CreateBacklogItem`'s existing callers all expect creation to succeed
  independent of cross-host gossip health. `RecordClaim` failures are logged
  (`claim_index.record_failed`), never returned as the creation error.
