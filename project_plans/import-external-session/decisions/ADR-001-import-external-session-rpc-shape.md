# ADR-001: New `ImportExternalSession` RPC family instead of a new SessionType or a `CreateSession` flag

## Status
Accepted

## Context
`import-external-session` needs to let a user promote an already-running, unmanaged process (discovered via ssq-mux, or a plain tmux pane) into a fully-managed stapler-squad `Instance`, then optionally terminate the original process after explicit confirmation.

Three shapes were considered for where this logic lives in the RPC surface:

- **(A)** A new, small `ImportService` with `PreviewImportExternalSession` / `CommitImportExternalSession` / `ConfirmKillExternalSession` (+ `BatchImportExternalSessions` in Phase 3), sharing one `ExternalSessionCandidate` type discriminated by `SourceKind`.
- **(B)** Separate RPC pairs per discovery path (`ImportMuxSession*` / `ImportPlainTmuxSession*`).
- **(C)** Extend the existing `CreateSession` RPC with an "import mode" flag and route through the existing `SessionType` switch in `server/services/session_service.go`.

Two existing repo rules bear directly on this decision:
- `.claude/rules/session-creation-registry.md` defines a 7-touchpoint registry for any *new session creation mode*. Import is not a way a working directory comes into being — every import target already has a real, pre-existing working directory — so treating it as a `SessionType`/creation-mode variant would misuse that registry's model and force artificial touchpoint updates (proto enum, path-guard, `SessionType` switch, frontend radio group) for a concept that isn't actually a creation mode.
- Research (`research/architecture.md`) independently confirmed `InstanceType` (`Managed`/`External`) is never persisted to the ent schema — only `SessionType` is. This means "import" is fundamentally a *lifecycle promotion* of an in-memory-only distinction, not a database-level variant of session creation, reinforcing that it doesn't belong inside `CreateSession`'s contract.

Additionally, the feature is explicitly two-phase by requirement (`requirements.md`, `research/architecture.md`, `research/ux.md`): preview (read-only, no side effects) must happen before commit (persist + start), and killing the original process must be a separate, explicitly confirmed action that can never be silently bundled into commit.

## Decision
Adopt shape **(A)**: a new `ImportService` proto service (`proto/session/v1/import.proto`) with a small number of RPCs, each phase (preview / commit / confirm-kill / batch) as its own call, sharing one `ExternalSessionCandidate{SourceKind, ...}` discriminated struct rather than either duplicating RPCs per source (B) or overloading `CreateSession` (C).

`SourceKind` (`MuxDiscovered` | `PlainTmux`) is a plain Go enum, not a new `session.SessionType` proto value and not a new interface. The two source kinds differ only in what identity data is available at discovery time (socket+PID vs. pane-only) and which kill primitive applies (`Instance.KillExternalSession()` vs. new `KillPlainTmuxPane`) — a concrete discriminant switch expresses this correctly without introducing a speculative `Discoverer`/`Killable` interface with exactly two implementations (see `.claude/rules/interface-pollution-checklist.md` smell #1).

Handlers delegate to per-source-kind pure functions (`session/import_candidate.go`, `session/import_correlate.go`, `session/import_commit.go`, `session/import_kill.go`) rather than branching inline in the RPC handler, to avoid the god-handler failure mode identified as (A)'s main weakness during the creative pass.

## Consequences
- **No changes to the 7-touchpoint session-creation registry.** Import never appears in the `SessionType` proto enum, the `CreateSession` path-guard, or the frontend `OmnibarCreationPanel` radio group — those all remain scoped to "how does a working directory come into being," which import never answers (the directory always already exists).
- **No new ent schema/migration.** Committing an import writes an ordinary `SessionTypeDirectory` row via the existing `EntRepository.Create` path; nothing new is persisted to distinguish "was this session imported" after the fact.
- A new proto file (`proto/session/v1/import.proto`) and a new `server/services/import_service.go` must be registered in `server/server.go`/`server/dependencies.go` — this is the one new piece of RPC wiring the decision introduces, in exchange for keeping the existing `CreateSession` contract and the session-creation registry untouched.
- Batch import (Phase 3) is a straightforward additive RPC on the same service rather than a second service or a retrofit of `CreateSession`, because the two-phase/per-source-kind shape established in Phase 1 already generalizes to N candidates without modification.
- If a third discovery source is ever added (e.g., a future non-tmux process supervisor), it extends the `SourceKind` enum and adds one dispatch arm per function — no new RPCs, no new interfaces — consistent with this ADR's reasoning.

## Alternatives Rejected
- **(B) Separate RPCs per discovery path** — rejected because it duplicates the safety-critical prepare→verify→commit→confirm-kill sequencing across two independent handlers, and batch import would need its own dispatch logic to route each candidate to the correct RPC, doubling the surface where the "never leave two writers on one JSONL" invariant must be enforced correctly.
- **(C) `CreateSession` import-mode flag** — rejected because it conflates a destructive, multi-step, confirmation-gated workflow with an RPC whose contract is "create now, one shot," offers no way to express a side-effect-free preview inside that contract, and would force `CreateSession`'s path-guard and `SessionType` switch to grow import-only branches irrelevant to every other caller of that RPC.
