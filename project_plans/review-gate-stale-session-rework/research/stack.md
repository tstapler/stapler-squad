# Stack Research: review-gate-stale-session-rework

**Date**: 2026-07-24

## Summary

This is a localized fix within the existing stapler-squad Go backend + React/TypeScript frontend — no new libraries, frameworks, or dependency versions are needed. All required infrastructure already exists and is in active use elsewhere in the codebase.

## Existing stack components this fix touches

- **Go backend**: `server/services/backlog_service_triage.go` (triage/rework orchestration), `session/backlog_lifecycle.go` (reconcile-tick detectors), `session/ent_repository_backlog.go` (ent-based persistence for `BacklogStuckState`), `session/domain/backlog.go` (pure domain types, no external deps — see the package doc comment: "imports only standard library packages").
- **ent ORM**: `BacklogStuckState` is already a defined ent schema entity (implied by `MarkStuck`/`FindOpenStuckStates`/`MarkStuckNotified` repository methods) — no schema changes anticipated unless Phase 3 adds a new `StuckReason` enum value, in which case it's a Go-level enum extension (string-backed, validated via `IsValid()`), not an ent schema migration, since `StuckReason` is stored as a plain string column.
- **ConnectRPC + protobuf**: `proto/session/v1/*.proto` defines the `StuckReason` proto enum consumed by `web-app/src/gen/session/v1/backlog_pb`. Only touched if Phase 3 adds a new reason value — requires `make proto-gen` per this repo's standard proto workflow (`.claude/rules` / CLAUDE.md's "New API Endpoints" section).
- **React/TypeScript + vanilla-extract**: `web-app/src/components/backlog-stuck/*` (already-shipped components: `StuckItem.tsx`, `StuckItemDetail.tsx`, `StuckItemsSection.tsx`, `stuckReason.ts` + `.css.ts`). No new component library or pattern needed — the generic `Record<StuckReason, T>` label/icon/class map pattern (`stuckReason.ts:15-60`) already accommodates a new enum value with a compile-time-enforced entry (TypeScript errors on a missing key), which is the correct mechanism to extend if a new reason is added.
- **Existing task-protocol prompt text**: `session/backlog_context.go`'s `taskProtocolBlock` — plain Go string template (`fmt.Sprintf`), no templating engine or external dependency; item D's fix is a pure text edit.

## Versions / dependencies

No new dependencies. This repo's existing `go.mod`/`web-app/package.json` versions are sufficient — verify via `go build ./...` and `cd web-app && npx jest --no-coverage` during implementation, not during planning.

## Community-recommended patterns

Not applicable — this is an internal architectural consistency fix (reuse an existing internal pattern correctly) rather than a greenfield technology choice. See `build-vs-buy.md` for the explicit "build vs. reuse-existing-internal-pattern" framing.
