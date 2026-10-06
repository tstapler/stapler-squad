# Implementation Plan: import-external-session

**Feature**: Let a user promote an already-running external Claude/Antigravity session (ssq-mux-wrapped IDE terminal, or a plain tmux/terminal pane) into a fully-managed stapler-squad `Instance` with complete conversation history, then optionally end the original process only after explicit confirmation.
**Date**: 2026-07-16
**Status**: Ready for implementation
**ADRs**: ADR-001-import-external-session-rpc-shape (see `project_plans/import-external-session/decisions/ADR-001-import-external-session-rpc-shape.md`)

---

## Step 0.5 — Alternatives Considered (Creative Pass)

Three high-level shapes were evaluated for how the client talks to the backend across both discovery paths (ssq-mux, plain-tmux) and all three lifecycle steps (preview → commit → confirm-kill):

**A. Single `ImportExternalSession` service** — one small RPC surface (`PreviewImportExternalSession`, `CommitImportExternalSession`, `ConfirmKillExternalSession`, plus `BatchImportExternalSessions` added in Phase 3) with a shared `ExternalSessionCandidate.SourceKind` discriminant, handlers delegating to per-source-kind pure functions rather than one branchy function.
Strength: one client integration surface and one place to enforce the safety-critical ordering invariant (persist+start before any kill) for every discovery path, instead of re-implementing that ordering per path.
Weakness: risks becoming a god-handler if the per-source-kind delegation discipline slips.

**B. Separate RPCs per discovery path** (`ImportMuxSession` / `ImportPlainTmuxSession`, each with their own preview/commit/kill).
Strength: each RPC's request/response shape is honestly minimal and specific to what that path actually has (mux has socket+PID; plain-tmux has a pane target and no socket).
Weakness: duplicates the safety-critical prepare→verify→commit→confirm-kill sequencing across two independent handlers, and batch import (Phase 3) would need dispatch logic to route each candidate to the right RPC — the "never leave two writers on one JSONL" invariant now has two enforcement points to keep in sync instead of one.

**C. Extend the existing `CreateSession` RPC with an import-mode flag.**
Strength: reuses an RPC clients already call; no new proto service.
Weakness: conflates a destructive, multi-step, confirmation-gated workflow (correlate → verify → commit → confirm-kill) with a call designed to be one-shot and fire-and-forget; `CreateSession`'s path-guard and `SessionType` switch would grow import-only branches they have no business knowing about, and there is no way to express "preview without committing" inside a call whose contract is "create now." This directly contradicts the architecture research's two-phase recommendation.

**Decision: A.** Recorded in the Pattern Decisions table below with B and C as rejected alternatives. See ADR-001 for the durable record of this decision (it is the one architecturally significant, hard-to-reverse choice in this plan — everything else is composition of existing packages).

---

## Domain Glossary

| Term | Definition | Notes |
|------|-----------|-------|
| `ExternalSessionCandidate` | An unmanaged, discovered process/pane eligible for import, before any `Instance` is constructed for it. Carries `SourceKind`, `Path`, `Program`, `PID`, optional `TmuxSession`/`SocketPath`. | Produced by either `mux.Discovery.Scan()` (ssq-mux path) or a new plain-tmux enumerator (Phase 2); never persisted. |
| `ImportSourceKind` | Enum `{MuxDiscovered, PlainTmux}` tagging where a candidate came from. Determines which correlation inputs are available (socket+PID vs. pane-only) and which kill primitive applies. | Not a `session.SessionType` — see ADR-001 §"why no new SessionType". |
| `ImportPreview` | Read-only summary returned by `PreviewImportExternalSession`: program, path, detected `ConversationUUID` (if any), turn count, last-message excerpt, `CorrelationResult`. Nothing is mutated to produce it. | Maps to the "preview with counts" UX pattern (browser bookmark import analog). |
| `CorrelationResult` | Discriminated outcome of running `HistoryFileDetector` against a candidate: `Resolved{UUID, Confidence}` \| `Ambiguous{Candidates []HistoryFileInfo}` \| `NotFound`. | Exhaustively switched on by every caller — see Pattern Decisions row 3. Never silently collapsed to "pick most recent." |
| `CorrelationConfidence` | Enum `{PIDExact, PathHeuristic}` attached to a `Resolved` correlation, surfaced in the UI so the user can see *why* a match was made. | `PIDExact` = `HistoryFileDetector.Detect(pid)`; `PathHeuristic` = `DetectByPath` with exactly one candidate. |
| `CorrelationAmbiguity` | The specific `Ambiguous` case of `CorrelationResult`: more than one JSONL/history file could plausibly belong to this candidate. Forces `DisambiguationChoice` before commit is allowed. | Never auto-resolved — must-not-happen #5 in pitfalls research. |
| `DisambiguationChoice` | User's explicit selection of one `HistoryFileInfo` out of a `CorrelationAmbiguity`'s candidate list, supplied back to `CommitImportExternalSession`. | Required input, not optional, whenever `CorrelationResult` was `Ambiguous`. |
| `PIDIdentity` | Value type `{PID int32, CreateTimeMs int64}` used to re-verify "is this still the same OS process" immediately before any destructive action (kill). | Guards the PID-reuse TOCTOU called out in pitfalls research; built on `procinfo.ProcessInspector.IsAlive`. |
| `AdoptedInstance` | The freshly-constructed `InstanceTypeManaged` `Instance` produced by `CommitImportExternalSession` — a normal `Instance`, not a new subtype; the term exists only to talk about "the Instance that resulted from an import" in code comments/logs. | No new Go type — reuses `session.Instance` as-is (avoids smell #6, struct-wraps-struct). |
| `ImportOutcome` | Per-candidate result of a commit attempt: `{CandidateRef, Status: Success\|Failed, InstanceID *string, Error *string}`. | One per item, always — batch import never collapses to a single boolean (must-not-happen #6). |
| `KillConfirmation` | The explicit, separately-issued user confirmation (a distinct RPC call, `ConfirmKillExternalSession`, never a flag bundled into commit) required before any original external process/session is terminated. | Mirrors the Slack "sign out other devices" UX pattern — always separate, always named. |
| `KillOutcome` | Result of a confirm-kill attempt: `{Status: Killed\|AlreadyGone\|Failed, Error *string}`. | `Failed` must surface loudly to the user, never silently swallowed (must-not-happen #3). |
| `BatchImportRequest` / `BatchImportResult` | Phase-3 wrapper types: a list of candidate refs (+ any `DisambiguationChoice`s) in, a list of `ImportOutcome` out. | Thin fan-out over the same `CommitImportExternalSession` logic used for single import — no new orchestration primitive. |
| `ExternalSessionRow` (frontend) | React-side view-model for one row in the discovery table: candidate + preview + selection + per-row status (`idle`/`loading`/`imported`/`failed`/`needs-disambiguation`/`kill-pending`/`kill-failed`). | Lives in `web-app/src/components/sessions/ImportExternalSessionsPanel.tsx` (new). |

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| Overall RPC shape | Single `ImportExternalSession` service, thin handlers delegating per `SourceKind` | Type-driven design | (B) Separate RPCs per discovery path; (C) `CreateSession` import-mode flag | See Step 0.5 — duplicated safety sequencing (B) and conflating one-shot creation with a confirmation-gated workflow (C) |
| Discovery aggregation (mux + plain-tmux) | Tagged struct `ExternalSessionCandidate{SourceKind, ...}` + plain `switch` at the few call sites that need source-specific behavior | Type-driven design (Go idiom, per `interface-pollution-checklist.md`) | `CandidateSource` interface with pluggable `Discover()` implementations | Only two sources will ever exist for the life of this feature; a runtime-pluggable interface is speculative generality (smell #1) — a discriminant field is simpler and the repo's own idiom |
| Import lifecycle orchestration | Two-phase command: `Prepare` (side-effect-free) → `Commit` (persist + start) → separate `ConfirmKill` | Command pattern (GoF), already the shape of destructive flows elsewhere in the repo | Single synchronous `ImportSession` call doing discovery+commit+kill in one round trip | Violates "verify first, kill on confirmation"; loses the preview step the UX research identifies as a hard requirement |
| Correlation ambiguity handling | Sum-type-shaped `CorrelationResult` (`Resolved`/`Ambiguous`/`NotFound`), exhaustively switched | Type-driven design | `(uuid string, err error)` with a sentinel "ambiguous" error | Conflates a legitimate "needs user input" branch with a true failure; callers could log-and-swallow instead of surfacing to the UI (must-not-happen #5) |
| Kill dispatch (mux-attach vs. plain-tmux vs. native process) | Table-driven dispatch on `ImportSourceKind` calling existing concrete functions (`Instance.KillExternalSession`, new `KillPlainTmuxPane`) directly | Type-driven design | New `Killable` interface implemented by tmux-kill and native-kill wrappers | Only two call sites exist and are already known statically; an interface here is a speculative abstraction whose sole purpose is to satisfy a switch that a concrete enum already expresses (smell #1) |
| Batch orchestration | Fan-out over `CommitImportExternalSession`, independent per-item result, no shared transaction | Type-driven design / PoEAA "Unit of Work" explicitly rejected | All-or-nothing transactional batch (single DB transaction wrapping N `Create`s) | Requirements mandate independent per-item outcomes; the underlying operations (DB row, tmux start, process kill) already aren't transactional across 3 subsystems, so a transactional *DB* wrapper would create a false sense of atomicity |
| Frontend discovery table + batch picker | Reuse existing concrete component shapes (`ApprovalAnalyticsPanel.tsx` checkbox table, `ResumeSessionModal.tsx` focus-trap dialog) copied and adapted | Composition over generalization | New generic `DataTable<T>` abstraction shared across features | Single call site; premature generalization (interface-pollution smell #5) |
| Instance promotion (external candidate → managed `Instance`) | Route all field/state changes through existing actor entry points (`Instance` construction + `finishInstanceConstruction`, existing status-transition machinery) | Actor / message-passing (already established in `session/instance_hibernate.go`) | Direct field mutation on a partially-built `Instance` for speed | Avoids a second, ad hoc write path racing the actor's serialized one — explicit reuse guidance from pitfalls research |
| PID re-verification before kill | Double-checked pattern: read `PIDIdentity` at preview time, re-read and compare immediately before signaling, always act on the freshly-read value | Existing repo pattern (`.claude/rules/go-double-checked-locking.md`, `procinfo.IsAlive`) | Trust the PID captured at discovery/preview time | Directly the TOCTOU class documented in pitfalls research (PID reuse, container-adoption bugs, gdb attach-to-wrong-process) |

---

## Migration Plan

No ent schema changes. Imported sessions persist through the existing `EntRepository.Create` path (`session/ent_repository.go`) using the existing `SessionType` column (always `directory` for imports) and the existing `ConversationUUID`/`HistoryFilePath` columns already populated for any resumed session — no new column, no new migration. `InstanceType` remains an in-memory-only distinction (as it is today) and is never persisted; a promoted `Instance` is, from the DB's perspective, indistinguishable from any other `SessionTypeDirectory` session created via `--resume`.

## Observability Plan

- **Logs**: structured log line per import attempt at each phase boundary — `import.preview` (candidate source kind, path, PID, correlation result), `import.commit` (target instance ID, resume UUID, success/failure, duration), `import.kill` (target PID/tmux session, confirmation timestamp, `KillOutcome`, duration). Batch imports additionally log `import.batch.summary` (counts by outcome). All logged via the existing structured logger used elsewhere in `session/` and `server/services/`.
- **Metrics**: count of import attempts by `SourceKind` and outcome (`Success`/`Failed`/`Ambiguous`); count of kill attempts by outcome; no new dashboards required for v1 — reuse whatever counter/log-based mechanism other session-lifecycle actions already emit through (no new alerting infra needed, per requirements' Observability Requirements section which explicitly says "no new oncall alert").
- **Alerts**: none — per requirements.md, failures surface synchronously to the initiating user via RPC error/UI state, not to oncall.

## Risk Control

- **Feature flag**: `STAPLER_SQUAD_ENABLE_SESSION_IMPORT` (env var, default `false` during Phase 1 rollout, following the existing `STAPLER_SQUAD_USE_CONTROL_MODE`-style env var convention). Gates both the new RPC handlers (return `CodeUnimplemented` when disabled) and the frontend panel's visibility.
- **Rollback procedure**: flip the env var back to `false` / unset it — no data migration to reverse, since imported sessions are ordinary `SessionTypeDirectory` rows indistinguishable from manually-created ones. Worst case for an in-flight import at rollback time: a `CommitImportExternalSession` call that already persisted a row but hasn't started tmux yet — same orphaned-row failure mode already handled by Story 1.2.3's compensating delete, not a new rollback hazard.
- **Staged rollout**: Phase 1 (ssq-mux single import) ships and soaks first, behind the flag, before Phase 2 (plain-tmux/manual) begins implementation — matches the phase sequencing below and the requirements' explicit instruction to cut scope rather than slip the deadline if all three phases don't fit.

## Unresolved Questions

- [ ] Should `ConfirmKillExternalSession` for the plain-tmux path default to killing the *pane's shell* or the *Claude process PID specifically* inside it, given the pitfalls research's finding that SIGHUP-on-shell-exit behavior differs across macOS/Linux/shell configs? — blocks Story 2.2.2 — owner: implementer, resolve via a spike measuring actual behavior on both platforms before writing `KillPlainTmuxPane`.
- [ ] Exact Antigravity running-process detection layout (`~/.gemini/antigravity-cli/`) needs confirmation against a real installed Antigravity instance before Story 3.2.1 can be estimated precisely — blocks Story 3.2.1 — owner: implementer, verify path during Phase 3 kickoff.
- [ ] Whether batch import's kill-confirmation is one dialog per session or one dialog naming all successfully-imported sessions in the batch — blocks Story 3.1.3 — owner: product/UX call during Phase 3, default to "one dialog per session" (matches Slack analog most closely) unless user feedback during Phase 1/2 soak indicates otherwise.

## Dependency Visualization

```
Phase 1: ssq-mux single-session import (smallest shippable slice)
  Epic 1.1 Backend: candidate model + preview RPC
        │
  Epic 1.2 Backend: commit RPC (promote DiscoveredSession -> managed Instance)
        │
  Epic 1.3 Backend: confirm-kill RPC (mux path only, reuses KillExternalSession)
        │
  Epic 1.4 Frontend: discovery panel + preview + confirm-import + confirm-kill dialogs
        │
        ▼
Phase 2: manual / plain-tmux import (adds a second discovery+kill path)
  Epic 2.1 Backend: plain-tmux candidate enumeration (session/pty_discovery.go extension)
        │
  Epic 2.2 Backend: KillPlainTmuxPane primitive + wiring into ConfirmKillExternalSession
        │
  Epic 2.3 Frontend: manual pointer entry (path/UUID) + source-kind-aware UI branching
        │
        ▼
Phase 3: batch import + multi-program (Antigravity)
  Epic 3.1 Backend+Frontend: BatchImportExternalSessions + per-row batch UI
        │
  Epic 3.2 Backend: Antigravity running-process detector + AgyAdapter wiring into commit path
```

Phase 2 depends on Phase 1's `ExternalSessionCandidate`/`ImportPreview`/`CorrelationResult` types and the `PreviewImportExternalSession`/`CommitImportExternalSession` RPC shapes being stable; it only adds a second `SourceKind` branch, never touches Phase 1's mux branch. Phase 3 depends on both prior phases' single-import commit path existing and being correct before fan-out orchestration is layered on top.

---

## Phase 1: ssq-mux Single-Session Import

### Epic 1.1: Candidate model and read-only preview

**Goal**: Given an already-discovered `mux.DiscoveredSession`, produce a read-only `ImportPreview` with zero side effects — no Instance created, nothing persisted, nothing killed.

#### Story 1.1.1: Define `ExternalSessionCandidate` and `ImportSourceKind`
**As a** backend developer, **I want** a small, source-agnostic type describing an importable candidate, **so that** preview/commit/kill logic can be written once and branch only where the two sources genuinely differ.
**Acceptance Criteria**:
- A new `session.ExternalSessionCandidate` struct exists with fields `SourceKind ImportSourceKind`, `Path string`, `Program string`, `PID int32`, `TmuxSession string`, `SocketPath string` (empty for plain-tmux).
  - *Given* a `mux.DiscoveredSession{Metadata: {Cwd: "/Users/x/proj", PID: 4821, TmuxSession: "ssq-mux-4821", Command: "claude"}}`, *When* `NewCandidateFromDiscovered(ds)` is called, *Then* it returns `ExternalSessionCandidate{SourceKind: MuxDiscovered, Path: "/Users/x/proj", Program: "claude", PID: 4821, TmuxSession: "ssq-mux-4821", SocketPath: ds.SocketPath}`.
- `ImportSourceKind` is a typed `int` enum with `MuxDiscovered` and `PlainTmux` (the latter unused until Phase 2, defined now so the type doesn't need a breaking change later).
**Files**: `session/import_candidate.go` (new), `session/import_candidate_test.go` (new)

##### Task 1.1.1a: Create `ImportSourceKind` enum and `ExternalSessionCandidate` struct (~3 min)
- Add `type ImportSourceKind int` with `MuxDiscovered ImportSourceKind = iota`, `PlainTmux`, and a `String()` method for logging.
- Add `ExternalSessionCandidate` struct per acceptance criteria.
- Files: `session/import_candidate.go`

##### Task 1.1.1b: Add `NewCandidateFromDiscovered(ds *mux.DiscoveredSession) ExternalSessionCandidate` (~4 min)
- Pure mapping function, no I/O.
- Files: `session/import_candidate.go`

##### Task 1.1.1c: Unit tests for the mapping function (~4 min)
- Table-driven test covering a normal `DiscoveredSession` and one with an empty `TmuxSession`.
- Files: `session/import_candidate_test.go`

#### Story 1.1.2: `CorrelationResult` sum type and one-shot correlation entry point
**As a** backend developer, **I want** a single function that runs `HistoryFileDetector` against a candidate and returns an exhaustive `Resolved`/`Ambiguous`/`NotFound` result, **so that** preview and commit share identical, non-silent ambiguity handling.
**Acceptance Criteria**:
- `CorrelationResult` is defined with fields making the three cases distinguishable without a stringly-typed status: `Kind CorrelationKind` (`Resolved`/`Ambiguous`/`NotFound`), `UUID string`, `Confidence CorrelationConfidence`, `Candidates []session.HistoryFileInfo`.
  - *Given* PID `4821` whose open files resolve to exactly one JSONL via `HistoryFileDetector.Detect(4821)`, *When* `CorrelateCandidate(candidate)` runs, *Then* it returns `CorrelationResult{Kind: Resolved, UUID: "<uuid>", Confidence: PIDExact}`.
  - *Given* PID lookup fails (process has no open Claude file) but `DetectByPath("/Users/x/proj")` finds two JSONL files modified within the same session, *When* `CorrelateCandidate(candidate)` runs, *Then* it returns `CorrelationResult{Kind: Ambiguous, Candidates: [file1, file2]}` and never picks one automatically.
  - *Given* neither PID nor path correlation finds anything, *When* `CorrelateCandidate(candidate)` runs, *Then* it returns `CorrelationResult{Kind: NotFound}` (a valid state, not an error — the JSONL may not exist yet).
- This is a **one-shot** call — it does not register the candidate with `HistoryLinker` or start any backoff/polling (that only happens after commit, in Story 1.2.2).
**Files**: `session/import_correlate.go` (new), `session/import_correlate_test.go` (new)

##### Task 1.1.2a: Define `CorrelationResult`/`CorrelationKind`/`CorrelationConfidence` types (~3 min)
- Files: `session/import_correlate.go`

##### Task 1.1.2b: Implement `CorrelateCandidate(candidate ExternalSessionCandidate) (CorrelationResult, error)` calling `HistoryFileDetector.Detect`/`DetectByPath` directly (~5 min)
- PID path first (`detector.Detect(candidate.PID)`); on failure/nil, fall back to `detector.DetectByPath(candidate.Path)`; if `DetectByPath` returns more than one plausible match (needs a small extension to `HistoryFileDetector` — see Task 1.1.2c — to return the full candidate list instead of silently picking most-recent), map to `Ambiguous`.
- Files: `session/import_correlate.go`

##### Task 1.1.2c: Extend `HistoryFileDetector.DetectByPath` with a `DetectAllByPath` variant returning all matches instead of the single most-recent (~5 min)
- Additive, non-breaking: existing `DetectByPath` callers (the background `HistoryLinker`) keep their current behavior; only the new import path calls `DetectAllByPath`.
- Files: `session/history_detector.go`, `session/history_detector_test.go`

##### Task 1.1.2d: Unit tests for `CorrelateCandidate` covering Resolved/Ambiguous/NotFound (~5 min)
- Files: `session/import_correlate_test.go`

#### Story 1.1.2b: Correlation-feasibility go/no-go spike (resolves pre-mortem Failure #2) (~30 min)
**As a** backend developer, **I want** to measure `CorrelateCandidate`'s real resolve rate against genuinely unmanaged (never previously started by stapler-squad) Claude processes before building `PreviewImportExternalSession`/`CommitImportExternalSession` on top of it, **so that** the plan doesn't ship a full safety-engineered import pipeline around a correlation step that mostly returns `Ambiguous`/`NotFound` in practice.
**Acceptance Criteria**:
- Against a sample of at least 10 genuinely unmanaged Claude CLI processes (started directly in a terminal, never through stapler-squad, with realistic multi-session working directories — i.e. not synthetic single-JSONL fixtures), `CorrelateCandidate` must resolve `Kind: Resolved` for at least 80% of cases. This threshold and the sample methodology are recorded as a comment header in `session/import_correlate.go` alongside the spike's measured rate.
- If the measured rate is below threshold, this story is a **hard gate**: Story 1.1.3 (`PreviewImportExternalSession`) may not proceed until either the correlation heuristic is revised (e.g. broadening `DetectByPath`'s search window, or adding a working-directory + recent-mtime heuristic) and re-measured, or the plan is amended via a fresh pre-mortem/architecture pass.
- If the measured rate clears threshold, record the result and proceed to Story 1.1.3 unchanged.
**Files**: none (research spike; finding recorded as a header comment in `session/import_correlate.go`)

##### Task 1.1.2b-1: Run the correlation-feasibility spike against ≥10 unmanaged Claude processes on the developer's own machine, record resolve/ambiguous/not-found counts (~20 min)
- Files: none (spike)

##### Task 1.1.2b-2: Record the measured rate and go/no-go decision as a header comment in `session/import_correlate.go`; if below threshold, do not proceed to Story 1.1.3 until resolved (~5 min)
- Files: `session/import_correlate.go`

#### Story 1.1.3: `PreviewImportExternalSession` RPC (read-only)
**As a** user, **I want** to see a preview of an external session before importing it, **so that** I can confirm it's the right conversation before anything changes.
**Acceptance Criteria**:
- New proto RPC `PreviewImportExternalSession(PreviewImportExternalSessionRequest) returns (PreviewImportExternalSessionResponse)` in a new `session/v1/import.proto`, taking a `SourceKind` + `SocketPath`/`PID`/`Path` identifying the candidate, returning program, path, `CorrelationResult` (as proto), turn count, last-message excerpt (via `ClaudeAdapter.Import` read against the resolved JSONL, when `Resolved`), and a `pid_identity { pid: int32, create_time_ms: int64 }` field captured via `procinfo.ProcessInspector` at preview time.
  - *Given* a request naming the mux-discovered candidate at PID 4821, *When* correlation resolves to UUID `abc-123` with 42 turns, *Then* the response has `correlation.kind = RESOLVED`, `correlation.uuid = "abc-123"`, `turn_count = 42`, `last_message_excerpt` containing the tail of the last `CanonicalTurn`, and `pid_identity = {pid: 4821, create_time_ms: <observed>}`.
  - *Given* the feature flag `STAPLER_SQUAD_ENABLE_SESSION_IMPORT` is unset/false, *When* `PreviewImportExternalSession` is called, *Then* the handler returns `connect.CodeUnimplemented`.
- No `Instance`, DB row, or tmux session is created or mutated by this RPC under any input.
- **`PIDIdentity` provenance (resolves architecture-review Blocker 1)**: `PreviewImportExternalSessionResponse` is the *only* place `PIDIdentity` is minted (read once via `procinfo.ProcessInspector` against `candidate.PID`). The client is responsible for holding that value in row state and passing it back verbatim on both `CommitImportExternalSessionRequest` (see Story 1.2.1) and `ConfirmKillExternalSessionRequest` (see Story 1.3.1) — no other RPC re-derives it from scratch, so there is exactly one source of truth for "what process did we observe at preview time."
**Files**: `proto/session/v1/import.proto` (new), `server/services/import_service.go` (new), `server/server.go` (register handler)

##### Task 1.1.3a: Write `proto/session/v1/import.proto` with `ImportService`, `PreviewImportExternalSession` message shapes, and a shared `PIDIdentity` proto message (`pid: int32`, `create_time_ms: int64`) reused by preview/commit/kill request-response pairs (~6 min)
- Files: `proto/session/v1/import.proto`

##### Task 1.1.3b: `make proto-gen` and commit generated bindings (~2 min)
- Files: `session/gen/session/v1/import*.go`, `web-app/src/gen/session/v1/import_pb.ts`

##### Task 1.1.3c: Implement `ImportService.PreviewImportExternalSession` handler calling `CorrelateCandidate` + `ClaudeAdapter.Import` + `procinfo.ProcessInspector` (~6 min)
- Guard on feature flag first; construct `ExternalSessionCandidate` from request; call `CorrelateCandidate`; if `Resolved`, read turns via `ReadCanonicalTurnsFromFile` (not a full `Instance` — see Story 1.1.4) to compute `turn_count`/excerpt; always read `PIDIdentity{PID: candidate.PID, CreateTimeMs: procinfo.ProcessInspector.CreateTime(candidate.PID)}` and set it on the response regardless of correlation outcome, since kill needs it even when correlation is `Ambiguous`/`NotFound`.
- Files: `server/services/import_service.go`

##### Task 1.1.3d: Register `ImportService` in `server/server.go` and `server/dependencies.go` (~3 min)
- Files: `server/server.go`, `server/dependencies.go`

##### Task 1.1.3e: Handler unit tests: resolved/ambiguous/not-found/flag-disabled (~5 min)
- Files: `server/services/import_service_test.go`

#### Story 1.1.4: Read canonical turns without a live `Instance`
**As a** backend developer, **I want** to call `ClaudeAdapter.Import` against a bare JSONL path (no `Instance`), **so that** preview doesn't require constructing a half-built `Instance` just to read history.
**Acceptance Criteria**:
- `ClaudeAdapter.Import` currently takes `(ctx, *Instance)`; add a small, additive helper `ReadCanonicalTurnsFromFile(path string) ([]CanonicalTurn, error)` on the adapter (or a package-level function) that both the existing `Import` and the new preview path delegate to, rather than duplicating JSONL-parsing logic.
  - *Given* a JSONL file with 3 well-formed lines and a 4th truncated line (simulating a mid-write read), *When* `ReadCanonicalTurnsFromFile` runs, *Then* it returns the 3 well-formed `CanonicalTurn`s and does not error on the trailing partial line (per pitfalls research's "tolerate trailing partial lines" requirement).
**Files**: `session/claude_adapter.go`

##### Task 1.1.4a: Extract JSONL-parsing body of `ClaudeAdapter.Import` into `ReadCanonicalTurnsFromFile(path string)` (~5 min)
- `Import(ctx, inst)` becomes a thin wrapper calling `ReadCanonicalTurnsFromFile(inst.HistoryFilePath())`.
- Files: `session/claude_adapter.go`

##### Task 1.1.4b: Make trailing-partial-line tolerance explicit with a test (~4 min)
- Files: `session/claude_adapter_test.go`

---

### Epic 1.2: Commit — promote a `DiscoveredSession` into a managed `Instance`

**Goal**: Given a candidate + resolved (or user-disambiguated) correlation, create a real, persisted `InstanceTypeManaged` `Instance` that resumes the correlated conversation — without touching the original external session, and without ever running two live writers against the same conversation JSONL.

#### Story 1.2.0a: Extract `CreateManagedInstance` shared domain function (resolves architecture-review Blocker 2)
**As a** backend developer, **I want** the core "resolve path, build `Instance`, persist, start tmux/`--resume`" logic that `SessionService.CreateSession` currently implements inline to live in a plain domain function, **so that** `CommitImportExternalSession` can call that logic directly instead of one connect handler invoking another handler's generated method.
**Acceptance Criteria**:
- A new function `session.CreateManagedInstance(ctx context.Context, params CreateManagedInstanceParams) (*Instance, error)` is extracted from `server/services/session_service.go`'s `CreateSession` (roughly lines ~1104-1350: path resolution, `SessionType` switch, `Instance` construction + start, `EntRepository.Create`), taking a plain struct (`Path`, `SessionType`, `ResumeID`, etc. — no `connect.Request`/`connect.Response` in its signature) and returning a real `*Instance` or an error.
  - *Given* `SessionService.CreateSession`'s connect handler receives a normal create request, *When* it unwraps the request into `CreateManagedInstanceParams` and calls `session.CreateManagedInstance`, *Then* the resulting `*Instance`/error is marshaled into the existing `CreateSessionResponse` shape with no behavior change from today.
  - *Given* `session.CommitImportExternalSession` (Story 1.2.1) needs to create the same kind of `Instance` for an import, *When* it calls `session.CreateManagedInstance` directly, *Then* it never imports `server/services` or references `SessionService`'s connect handler/`connect.Request` types — the dependency points the correct direction (RPC layer → domain function), never handler → handler.
- `SessionService.CreateSession`'s existing tests continue to pass unmodified (behavior-preserving refactor).
**Files**: `session/create_managed_instance.go` (new), `server/services/session_service.go`, `server/services/session_service_test.go`

##### Task 1.2.0a-1: Define `CreateManagedInstanceParams` and extract `CreateManagedInstance` from `SessionService.CreateSession` (~6 min)
- Files: `session/create_managed_instance.go`

##### Task 1.2.0a-2: Rewire `SessionService.CreateSession` to unwrap its `connect.Request` into `CreateManagedInstanceParams`, call `session.CreateManagedInstance`, and marshal the result — no remaining inline creation logic in the handler (~5 min)
- Files: `server/services/session_service.go`

##### Task 1.2.0a-3: Run existing `SessionService.CreateSession` tests unmodified to prove the refactor is behavior-preserving; add one regression test asserting the handler contains no direct `EntRepository`/tmux calls (only delegates) (~4 min)
- Files: `server/services/session_service_test.go`

#### Story 1.2.0b: Spike — verify `claude --resume <uuid>` behavior with a concurrently-live original process (resolves adversarial-review Blocker 4, part 1)
**As a** backend developer, **I want** to know, before implementing commit, whether the Claude CLI has any lock/refusal behavior when `--resume <uuid>` is invoked while another process already has that conversation's JSONL open, **so that** the commit ordering can rely on real CLI behavior instead of an untested assumption.
**Acceptance Criteria**:
- A written finding (recorded as a code comment header in `session/import_commit.go` plus a short note in this plan's Unresolved Questions resolution) states, for both macOS and Linux dev environments: (a) does `claude --resume <uuid>` fail/refuse/warn when the same UUID's JSONL already has an open writer, and (b) does the CLI use any advisory/O_EXCL-style lock file. This spike must complete and its answer must be reflected in Task 1.2.1b before that task is implemented — it is not deferred to Phase 3 like the Antigravity spike.
- If no CLI-level protection exists (the expected outcome based on pitfalls research), the mitigation in Story 1.2.1's Task 1.2.1e is mandatory, not optional.
**Files**: none (research spike; finding recorded in `session/import_commit.go` header comment)

##### Task 1.2.0b-1: Run `claude --resume <uuid>` against a JSONL with a live writer on macOS and Linux (or a Linux container/VM), record actual behavior (~5 min)
- Files: none (spike)

#### Story 1.2.1: `CommitImportExternalSession` RPC — happy path
**As a** user, **I want** to commit an import after previewing it, **so that** a fully-managed session appears with the same conversation history, without ever leaving two live processes writing the same JSONL.
**Acceptance Criteria**:
- New RPC `CommitImportExternalSession(CommitImportExternalSessionRequest) returns (CommitImportExternalSessionResponse)` accepting the same candidate identity as preview, the `pid_identity` returned by preview (see Story 1.1.3), the `correlation` result shown to the user in preview (`expected_correlation` — see Task 1.2.1f), plus an optional `disambiguation_choice` (history file path), and returning `{instance_id, outcome, pid_identity}` (the response echoes a freshly-re-read `pid_identity` so the frontend's row state stays current for the later kill-confirm call).
  - *Given* a candidate at path `/Users/x/proj` with `CorrelationResult{Kind: Resolved, UUID: "abc-123"}`, *When* `CommitImportExternalSession` is called, *Then* it internally builds `CreateManagedInstanceParams{SessionType: SESSION_TYPE_DIRECTORY, Path: "/Users/x/proj", ResumeID: "abc-123"}`, calls `session.CreateManagedInstance` (Story 1.2.0a — never `SessionService.CreateSession`'s connect handler), and returns `{instance_id: "<new-id>", outcome: {status: SUCCESS}}` with a new row visible via `ListSessions`.
  - *Given* the same candidate but `CorrelationResult{Kind: Ambiguous}` and no `disambiguation_choice` supplied, *When* `CommitImportExternalSession` is called, *Then* it returns `connect.CodeInvalidArgument` ("disambiguation required") and creates nothing.
- The original `DiscoveredSession`/mux socket/tmux session is not signaled/killed by this RPC under any input — but see Task 1.2.1e for the explicit write-suspension guard this RPC *does* apply before starting the resumed process.
**Files**: `server/services/import_service.go`, `session/import_commit.go` (new)

##### Task 1.2.1a: Add `disambiguation_choice`, `pid_identity` (input, from preview), `expected_correlation` (input, from preview) fields to `CommitImportExternalSessionRequest`, and `pid_identity` (output, freshly re-read) to `CommitImportExternalSessionResponse`; regenerate (~4 min)
- Files: `proto/session/v1/import.proto`, generated bindings

##### Task 1.2.1b: Implement `CommitImportExternalSession(ctx, candidate, pidIdentity, expectedCorrelation, disambiguation) (instanceID string, freshIdentity PIDIdentity, err error)` in `session/import_commit.go`, delegating to `session.CreateManagedInstance` (Story 1.2.0a) (~6 min)
- Re-run `CorrelateCandidate` inside commit (never trust a preview result the client cached — re-verify immediately before acting, per the double-checked pattern). If `disambiguation_choice` is supplied, still re-run `CorrelateCandidate` and assert the choice is a member of the fresh `Ambiguous` result's `Candidates`; return `connect.CodeInvalidArgument` if it is not (closes architecture-review's disambiguation-bypass concern).
- Files: `session/import_commit.go`

##### Task 1.2.1c: Wire `ImportService.CommitImportExternalSession` handler to call `session.CommitImportExternalSession` (~4 min)
- Files: `server/services/import_service.go`

##### Task 1.2.1d: Handler + unit tests for happy path and ambiguous-without-choice (~5 min)
- Files: `server/services/import_service_test.go`, `session/import_commit_test.go`

##### Task 1.2.1e: Suspend the original process before starting the resumed one; resume it on commit failure (resolves adversarial-review Blocker 4, part 2) (~6 min)
- Immediately before calling `session.CreateManagedInstance`, send `SIGSTOP` to `pidIdentity.PID` (via a new `session.SuspendOriginalProcess(pid int32) error`, built on the existing `procinfo` package) after re-verifying `procinfo.ProcessInspector.IsAlive(pidIdentity.PID, pidIdentity.CreateTimeMs)` — this guarantees the original process cannot write to the JSONL while the resumed process starts, closing the dual-writer window identified in adversarial-review Blocker 1, independent of whatever Story 1.2.0b's spike finds about CLI-level locking.
- **Durable record before suspending (resolves pre-mortem Failure #1)**: before sending `SIGSTOP`, persist a `SuspendedProcessRecord{PID, CreateTimeMs, CandidateRef, InstanceID}` row via a new small store (`session/suspended_process_store.go`, backed by the same `config.json`-style JSON persistence used elsewhere in `config/` — not the ent DB, since this is transient operational state, not domain data). Remove the record only when the process is later `SIGCONT`'d (kill-confirmed, cancelled, or reconciled on startup — see Task 1.2.1h).
- If `session.CreateManagedInstance` (or the subsequent start) fails, call `session.ResumeOriginalProcess(pid int32) error` (`SIGCONT`) so the user's original session is left exactly as it was before the failed commit attempt (compensating action, mirrors Story 1.2.3's compensating delete), and remove the `SuspendedProcessRecord`.
- If it succeeds, the original process remains stopped (`SIGSTOP`'d) until `ConfirmKillExternalSession` (Story 1.3.1) either kills it outright or the user cancels — see Story 1.3.3 for the cancel-path resume guard. The `SuspendedProcessRecord` remains persisted until one of those two outcomes (or Task 1.2.1h's startup reconciliation) removes it.
- Files: `session/import_commit.go`, `session/process_suspend.go` (new), `session/suspended_process_store.go` (new)

##### Task 1.2.1h: Startup reconciliation pass — `SIGCONT` any PID left suspended from a prior server incarnation (resolves pre-mortem Failure #1) (~5 min)
- On server startup, `session.ReconcileSuspendedProcesses(ctx)` reads every persisted `SuspendedProcessRecord`, re-verifies `procinfo.ProcessInspector.IsAlive(record.PID, record.CreateTimeMs)`, calls `session.ResumeOriginalProcess(record.PID)` for each still-alive match (the row's own commit/kill outcome was never confirmed before the prior process died, so the safest default is to unfreeze the user's original session rather than leave it stopped indefinitely), and removes the record. If the matching `InstanceID` still exists as an uncommitted/partial row, apply Story 1.2.3's compensating delete to it as well, since a reconciled-resume recreates the same abandon-without-cleanup shape Story 1.3.3 guards against.
  - *Given* a `SuspendedProcessRecord` for PID 4821 persisted before a server restart, and PID 4821's create-time still matches on the process table after restart, *When* `ReconcileSuspendedProcesses` runs at startup, *Then* it `SIGCONT`s PID 4821 and removes the record — the user's terminal is unfrozen even though the server never received a kill-confirm or cancel.
  - *Given* a persisted record whose PID/create-time no longer matches any live process (already exited or reused), *When* reconciliation runs, *Then* it just removes the stale record without signaling anything.
- Call this from the same startup path that initializes other session-recovery logic (see `session/storage.go`'s existing startup load, if present, for the established pattern).
- Files: `session/suspended_process_store.go`, `server/server.go` (call at startup), `session/suspended_process_store_test.go` (new)

##### Task 1.2.1f: Reject commit if re-correlation drifted from the previewed correlation (resolves adversarial-review Blocker 2 / "Correlation drift") (~5 min)
- After re-running `CorrelateCandidate`, compare its `Kind`+`UUID` against the `expected_correlation` echoed back from the client (which is exactly what preview returned and the user reviewed in the preview dialog). If they differ in `Kind` or `UUID`, abort before any `Instance` is created — do not call `CreateManagedInstance` — and return `connect.CodeFailedPrecondition` with message `"the matched conversation changed since preview — please re-preview"` (surfaced verbatim by the frontend, not swallowed).
  - *Given* preview returned `Resolved{UUID: "abc-123"}` and commit's re-correlation now resolves `Resolved{UUID: "def-456"}` (a second JSONL appeared in between), *When* `CommitImportExternalSession` is called with `expected_correlation = {Kind: Resolved, UUID: "abc-123"}`, *Then* it returns `CodeFailedPrecondition` and creates nothing.
- Files: `session/import_commit.go`, `session/import_commit_test.go`

##### Task 1.2.1g: Tests: suspend-then-resume-on-failure, suspend-then-left-stopped-on-success, correlation-drift abort (~6 min)
- Files: `session/import_commit_test.go`

#### Story 1.2.2: Register the promoted `Instance` with `HistoryLinker`
**As a** backend developer, **I want** the newly-committed `Instance` to be handed to `HistoryLinker.AddInstance` immediately after creation, **so that** ongoing correlation (e.g. a later `/clear`) keeps working exactly like any other managed session, with no competing polling loop.
**Acceptance Criteria**:
- After a successful commit, `HistoryLinker.AddInstance(newInstance)` is called exactly once.
  - *Given* a successful `CommitImportExternalSession` call, *When* the commit completes, *Then* `HistoryLinker`'s internal instance set contains the new instance's title, and no separate/duplicate correlation loop was started for it.
**Files**: `session/import_commit.go`

##### Task 1.2.2a: Call `HistoryLinker.AddInstance` at the end of the commit function, guarded so it only fires once (~3 min)
- Files: `session/import_commit.go`

##### Task 1.2.2b: Test asserting `AddInstance` is called exactly once on success and zero times on a failed commit (~4 min)
- Files: `session/import_commit_test.go`

#### Story 1.2.3: Compensating delete on partial commit failure
**As a** user, **I want** a failed import to never leave an orphaned, half-created session behind, **so that** the session list doesn't accumulate broken entries after a failed import.
**Acceptance Criteria**:
- If `EntRepository.Create` succeeds but starting the new tmux session / `--resume` fails, the just-created DB row is deleted and the RPC returns a failure `ImportOutcome`, not a partial success.
  - *Given* `EntRepository.Create` succeeds and the subsequent `Instance.Start()` call returns an error (e.g. tmux server unreachable), *When* `CommitImportExternalSession` observes the start failure, *Then* it calls `EntRepository.Delete(instanceID)` and returns `ImportOutcome{Status: Failed, Error: "<start error>"}` with no visible row in `ListSessions`.
**Files**: `session/import_commit.go`, `session/import_commit_test.go`

##### Task 1.2.3a: Wrap the create-then-start sequence with a deferred compensating delete on start failure (~5 min)
- Files: `session/import_commit.go`

##### Task 1.2.3b: Test simulating a start failure and asserting the row is gone afterward (~4 min)
- Files: `session/import_commit_test.go`

#### Story 1.2.4: Pre-commit worktree/path-collision guard (resolves adversarial-review Blocker 3 — "No worktree/path-collision check")
**As a** user, **I want** an import to refuse to proceed if the target path already belongs to another managed `Instance`, **so that** I never end up with two managed sessions pointed at the same working directory/worktree.
**Acceptance Criteria**:
- Before calling `session.CreateManagedInstance` (Story 1.2.0a), `CommitImportExternalSession` checks `candidate.Path` (and, if it resolves to a git worktree, the worktree root) against every existing `Instance`'s `WorkingDirectory()`/worktree path in the current session registry (`session/storage.go`'s in-memory instance set, the same source `ListSessions` reads from).
  - *Given* an existing managed `Instance` already has `WorkingDirectory() == "/Users/x/proj"`, *When* `CommitImportExternalSession` is called with `candidate.Path == "/Users/x/proj"`, *Then* it returns `connect.CodeAlreadyExists` with message `"this path is already managed by session <existing-instance-id>"` and creates nothing — no `Instance`, no DB row.
  - *Given* `candidate.Path` resolves (via the existing worktree-resolution helper used elsewhere in `session/git/`) to a worktree already registered to a *different* instance's git worktree metadata, *When* commit is called, *Then* the same `CodeAlreadyExists` outcome applies, even if the raw path strings differ (e.g. candidate points at a worktree subdirectory).
  - *Given* no existing `Instance` owns the path/worktree, *When* commit is called, *Then* this check passes silently and commit proceeds normally.
- This check runs regardless of `SourceKind` (mux or plain-tmux) and regardless of correlation outcome — it is a structural safety check, not a correlation concern.
**Files**: `session/import_commit.go`, `session/import_commit_test.go`

##### Task 1.2.4a: Implement `CheckPathNotAlreadyManaged(path string, registry SessionRegistry) error` consulting the existing in-memory instance set (~5 min)
- Files: `session/import_commit.go`

##### Task 1.2.4b: Call the check at the top of `CommitImportExternalSession`, before suspension (Task 1.2.1e) and before `CreateManagedInstance` (~3 min)
- Files: `session/import_commit.go`

##### Task 1.2.4c: Tests: exact-path collision, worktree-subpath collision, no-collision passthrough (~5 min)
- Files: `session/import_commit_test.go`

---

### Epic 1.3: Confirm-kill (mux path)

**Goal**: After a verified-successful commit, let the user explicitly end the original ssq-mux-discovered session — reusing the existing `KillExternalSession` primitive, with a PID/identity re-check immediately before signaling. Note: since Task 1.2.1e already `SIGSTOP`s the original process at commit time, this epic's "kill" is really "finalize the termination of an already-suspended process," not a kill of a live writer — the dual-writer window is closed before this epic ever runs.

#### Story 1.3.1: `ConfirmKillExternalSession` RPC (mux source kind)
**As a** user, **I want** to end the original external session only after I've verified the import worked, **so that** I never lose a conversation to a premature kill.
**Acceptance Criteria**:
- New RPC `ConfirmKillExternalSession(ConfirmKillExternalSessionRequest) returns (ConfirmKillExternalSessionResponse)` taking the *original* candidate's identity (not the new instance's) plus the `pid_identity` returned by `CommitImportExternalSessionResponse` (Story 1.2.1 — the freshest re-read, not the one from preview, since more time has elapsed since preview than since commit), returning `KillOutcome`.
  - *Given* a mux-discovered candidate whose tmux session `ssq-mux-4821` is still alive (currently `SIGSTOP`'d per Task 1.2.1e) and whose PID 4821's create-time matches the value captured at commit, *When* `ConfirmKillExternalSession` is called, *Then* it calls the existing `Instance.KillExternalSession()` path (which sends the terminating signal regardless of the process's stopped state) and returns `KillOutcome{Status: Killed}`.
  - *Given* the same candidate but PID 4821 has since exited and PID 4821 was reused by an unrelated process with a different create-time, *When* `ConfirmKillExternalSession` is called, *Then* it detects the create-time mismatch via `procinfo.ProcessInspector.IsAlive(4821, expectedCreateTimeMs)`, does **not** signal the unrelated process, and returns `KillOutcome{Status: AlreadyGone}`.
  - *Given* `tmux kill-session` fails for any reason (e.g. permission, session already torn down mid-call), *When* `ConfirmKillExternalSession` is called, *Then* it returns `KillOutcome{Status: Failed, Error: "<detail>"}`, the failure is logged at warn level with the target PID/session name (never silently swallowed, per must-not-happen #3), and the original process is left in its current `SIGSTOP`'d state (not resumed) so the user can retry the kill rather than the process silently resuming and racing the (already-committed) resumed instance.
**Files**: `proto/session/v1/import.proto`, `server/services/import_service.go`, `session/import_kill.go` (new)

##### Task 1.3.1a: Add `ConfirmKillExternalSessionRequest`/`Response` + `KillOutcome` messages to proto, request takes `pid_identity` (from commit response, not preview); regenerate (~4 min)
- Files: `proto/session/v1/import.proto`, generated bindings

##### Task 1.3.1b: Implement `ConfirmKillExternalSession(ctx, candidate, expectedIdentity PIDIdentity) (KillOutcome, error)` re-verifying identity via `procinfo.ProcessInspector.IsAlive` immediately before dispatching to `Instance.KillExternalSession()` for `MuxDiscovered` candidates (~5 min)
- Files: `session/import_kill.go`

##### Task 1.3.1c: Wire the RPC handler (~3 min)
- Files: `server/services/import_service.go`

##### Task 1.3.1d: Tests: kill success, PID-reuse-detected-skip, tmux-kill-failure-surfaced-and-left-suspended (~5 min)
- Files: `session/import_kill_test.go`, `server/services/import_service_test.go`

#### Story 1.3.2: Feature flag plumbing
**As an** operator, **I want** the entire import feature gated behind an env var, **so that** it can be rolled out safely and rolled back without code changes.
**Acceptance Criteria**:
- `STAPLER_SQUAD_ENABLE_SESSION_IMPORT` (default `false`) is read once at server startup and threaded into `ImportService`.
  - *Given* the env var is unset, *When* any of the three import RPCs are called, *Then* each returns `connect.CodeUnimplemented` without touching discovery/correlation/commit/kill code paths.
  - *Given* the env var is `"true"`, *When* the same RPCs are called, *Then* they execute normally.
**Files**: `config/config.go` (or existing env-flag location), `server/dependencies.go`, `server/services/import_service.go`

##### Task 1.3.2a: Add flag read + threading through dependency construction (~4 min)
- Files: `config/config.go`, `server/dependencies.go`

##### Task 1.3.2b: Guard all three handlers on the flag; test both states (~4 min)
- Files: `server/services/import_service.go`, `server/services/import_service_test.go`

#### Story 1.3.3: Abandon the entire import — compensating-delete the committed `Instance`, then resume the original process (resolves pre-mortem Failure #3)
**As a** user, **I want** cancelling the kill step to fully undo the import (not just un-suspend my original terminal), **so that** I never end up with both the original process and the newly-committed managed `Instance` writing the same conversation JSONL at once.
**Acceptance Criteria**:
- **"Abandon kill" is redefined as "abandon the entire import."** A `CancelPendingKill(candidateRef) error` path (called when the user dismisses the row / navigates away without confirming kill, or explicitly clicks a "Keep original session running" affordance) must, in order: (1) stop the committed `Instance` and delete its DB row via the same compensating-delete mechanism Story 1.2.3 uses for a failed start (`EntRepository.Delete(instanceID)` + tmux/process teardown for the *new* resumed process, not the original), (2) only then call `session.ResumeOriginalProcess` (`SIGCONT`, built in Task 1.2.1e) on the suspended original PID, and (3) remove the corresponding `SuspendedProcessRecord` (Task 1.2.1e/1.2.1h). Resuming the original before or without deleting the committed `Instance` is the exact bug this story exists to prevent — the acceptance tests must assert the delete happens first.
  - *Given* a row is in `imported`/`kill-pending` state (original process `SIGSTOP`'d since commit, a managed `Instance` already running and writing the resumed JSONL) and the user dismisses the row without confirming kill, *When* the dismissal handler runs, *Then* the managed `Instance`'s DB row is deleted and its tmux/resume process is torn down *before* `ResumeOriginalProcess(pid)` is called, so at no point are both the original and the (now-deleted) managed instance simultaneously live and writing.
  - *Given* the compensating delete of the managed `Instance` itself fails (e.g. tmux teardown error), *When* `CancelPendingKill` observes that failure, *Then* it does **not** call `ResumeOriginalProcess` — the original stays `SIGSTOP`'d and the error surfaces to the user loudly (must-not-happen #3: never silently swallow a failed kill/cleanup outcome) rather than risking a live dual-writer state.
  - *Given* the delete-then-resume sequence completes successfully and the user later wants the conversation back, *When* they start a new session against the same path, *Then* correlation/commit runs fresh exactly as it would for any never-imported candidate — there is no special "previously abandoned" state to recover from.
- This closes both the "left suspended indefinitely" gap called out in Story 1.2.1's Task 1.2.1e note and the dual-writer recreation risk identified in pre-mortem Failure #3.
**Files**: `session/import_kill.go`, `session/import_kill_test.go`, `web-app/src/components/sessions/ImportExternalSessionsPanel.tsx`

##### Task 1.3.3a: Implement `CancelPendingKill` performing compensating-delete of the committed `Instance` (reusing Story 1.2.3's delete path) before calling `ResumeOriginalProcess`, and removing the `SuspendedProcessRecord` on completion (~6 min)
- Files: `session/import_kill.go`

##### Task 1.3.3b: Wire row-dismissal in the frontend panel to call it via a small RPC or local cleanup call; update row-state copy so "Keep original session running" clearly implies the import itself is undone, not just the kill step (~4 min)
- Files: `web-app/src/components/sessions/ImportExternalSessionsPanel.tsx`

##### Task 1.3.3c: Tests: delete-runs-before-resume ordering, resume-is-skipped-when-delete-fails, kill-still-works-if-user-never-cancels (~5 min)
- Files: `session/import_kill_test.go`

---

### Epic 1.4: Frontend — discovery panel, preview, confirm-import, confirm-kill

**Goal**: A user can see ssq-mux-discovered external sessions in the UI, preview one, import it, and separately confirm ending the original — all with accessible, testable markup.

#### Story 1.4.1: Discovery list panel
**As a** user, **I want** to see a list of externally-discovered Claude sessions, **so that** I can choose one to import.
**Acceptance Criteria**:
- `ImportExternalSessionsPanel.tsx` renders a `<table>` of `ExternalSessionRow`s (one per `DiscoveredSession` currently visible), each with `data-testid="discovered-session-row-{pid}"`, columns for path, program, last-active, and an "Import" button.
  - *Given* the backend reports one discovered session at `/Users/x/proj`, PID 4821, program `claude`, *When* the panel mounts, *Then* a row with `data-testid="discovered-session-row-4821"` and accessible name `"Claude session in /Users/x/proj, PID 4821"` is rendered.
  - *Given* zero discovered sessions, *When* the panel mounts, *Then* an empty state renders text "No unmanaged Claude or Antigravity sessions found..." linking to `ssq-mux` setup docs, not a bare blank table.
  - *Given* discovery updates live (a new session appears), *Then* an `aria-live="polite"` region announces the new count.
**Files**: `web-app/src/components/sessions/ImportExternalSessionsPanel.tsx` (new), `web-app/src/components/sessions/ImportExternalSessionsPanel.css.ts` (new)

##### Task 1.4.1a: Scaffold `ImportExternalSessionsPanel.tsx` with table markup + `data-testid` rows, modeled on `ApprovalAnalyticsPanel.tsx` (~5 min)
- Files: `web-app/src/components/sessions/ImportExternalSessionsPanel.tsx`

##### Task 1.4.1b: `ImportExternalSessionsPanel.css.ts` using vanilla-extract + `vars.xxx` tokens only, no raw `var()` strings (~4 min)
- Files: `web-app/src/components/sessions/ImportExternalSessionsPanel.css.ts`

##### Task 1.4.1c: Empty state + `aria-live` count region (~4 min)
- Files: `web-app/src/components/sessions/ImportExternalSessionsPanel.tsx`

##### Task 1.4.1d: Jest/RTL tests for row rendering, empty state, live region text (~5 min)
- Files: `web-app/src/components/sessions/ImportExternalSessionsPanel.test.tsx` (new)

#### Story 1.4.2: Preview + confirm-import dialog
**As a** user, **I want** to see a preview before committing an import, **so that** I can verify it's the right conversation.
**Acceptance Criteria**:
- Clicking "Import" on a row opens a `role="dialog" aria-modal="true"` preview dialog (modeled on `ResumeSessionModal.tsx`), showing program, path, turn count, last-message excerpt, and correlation confidence (`"Matched by process ID"` vs. `"Matched by best guess — verify this is correct"`).
  - *Given* a row whose preview resolves with `Confidence: PIDExact`, *When* the dialog opens, *Then* it displays "Matched by process ID" text (never silently omits the confidence signal).
  - *Given* a row whose preview resolves `Ambiguous` with 2 candidate files, *When* the dialog opens, *Then* it renders a sub-list of the 2 candidates (each labeled with last-modified time + excerpt) and disables the "Confirm Import" button until one is selected.
  - *Given* the user clicks "Confirm Import" after preview, *When* the commit RPC succeeds, *Then* the dialog closes, the row's status becomes `imported`, and a new session appears in the main session list without a page reload.
- **(Resolves Phase 4 triad-review blocker — SIGSTOP-at-commit disclosure gap)** The dialog must disclose, before "Confirm Import" is clicked, that the original terminal will briefly pause during commit (Task 1.2.1e sends `SIGSTOP` immediately on commit, ahead of any separate "End original session" step), and the discovery row for that session must show a visible "paused" indicator from the moment "Confirm Import" is clicked until the process is resumed (cancel/abandon) or the kill step completes.
  - *Given* the preview dialog is open, *Then* it renders copy to the effect of "Your original terminal session will briefly pause while we complete the import" near the "Confirm Import" button, not only in a tooltip or help text.
  - *Given* the user clicks "Confirm Import" and the commit RPC is in flight or has succeeded but kill has not yet been confirmed, *When* the discovery row for that session renders, *Then* it shows a distinct "Paused" status (visually and via accessible name, not color alone) rather than its normal pre-import state.
  - *Given* the user cancels the pending kill (`CancelPendingKill`) or the commit fails and the original process is `SIGCONT`'d, *When* the row next renders, *Then* the "Paused" indicator is cleared.
**Files**: `web-app/src/components/sessions/ImportPreviewDialog.tsx` (new), `web-app/src/components/sessions/ImportPreviewDialog.css.ts` (new)

##### Task 1.4.2a: Scaffold dialog with focus trap + restoration, following `ResumeSessionModal.tsx`'s pattern (~5 min)
- Files: `web-app/src/components/sessions/ImportPreviewDialog.tsx`

##### Task 1.4.2b: Render correlation confidence text and disambiguation sub-list (~5 min)
- Files: `web-app/src/components/sessions/ImportPreviewDialog.tsx`

##### Task 1.4.2c: Wire "Confirm Import" to `CommitImportExternalSession` via a new `useImportSessionService.ts` hook (~5 min)
- Files: `web-app/src/lib/hooks/useImportSessionService.ts` (new)

##### Task 1.4.2c-1: Add SIGSTOP-disclosure copy to the confirm-import dialog and a "Paused" row status from click-through-commit until resume/kill (resolves Phase 4 triad-review blocker: SIGSTOP-at-commit disclosure gap) (~5 min)
- Files: `web-app/src/components/sessions/ImportPreviewDialog.tsx`, `web-app/src/components/sessions/ImportExternalSessionsPanel.tsx`

##### Task 1.4.2d: Thread `pid_identity` and the previewed `correlation` from `PreviewImportExternalSessionResponse` into row state, and pass both back on `CommitImportExternalSessionRequest` as `pid_identity`/`expected_correlation`; on commit success, overwrite row state's `pid_identity` with the freshly-returned value from `CommitImportExternalSessionResponse` for use by the later `ConfirmKillExternalSession` call (resolves architecture-review Blocker 1's frontend half) (~5 min)
- Files: `web-app/src/components/sessions/ImportExternalSessionsPanel.tsx`, `web-app/src/lib/hooks/useImportSessionService.ts`

##### Task 1.4.2e: Jest/RTL tests: confidence text, disambiguation gating, successful commit closes dialog, `pid_identity` round-trips from preview response into commit request and is updated from commit response afterward (~6 min)
- Files: `web-app/src/components/sessions/ImportPreviewDialog.test.tsx` (new)

#### Story 1.4.3: Confirm-kill dialog (separate, named target)
**As a** user, **I want** a separate, explicit confirmation before the original session is ended, **so that** I never end the wrong process by accident.
**Acceptance Criteria**:
- After a successful import, the row shows an "End original session" button (not auto-offered before import succeeds). Clicking it opens a dialog with heading naming the exact target, e.g. `aria-labelledby` pointing to text "End external session in /Users/x/proj (PID 4821)?" — never a generic "Are you sure?".
  - *Given* the import for this row has not yet succeeded, *Then* "End original session" is not rendered at all (not just disabled).
  - *Given* the dialog is open, *When* it first renders, *Then* initial focus is on "Cancel", not the destructive action button (guards against accidental Enter).
  - *Given* the user confirms, *When* `ConfirmKillExternalSession` returns `KillOutcome{Status: Killed}`, *Then* the row shows a "Session ended" state; *When* it returns `Status: Failed`, *Then* the row shows a dismissible error "Import complete. Could not end the original session — you may need to close it manually," and the row's `imported` status is unaffected.
- **(Resolves Phase 4 triad-review repair-loop residual gap — kill-failed × Paused reconciliation)** When `ConfirmKillExternalSession` returns `KillOutcome{Status: Failed}`, the row's "Paused" indicator (from Story 1.4.2's Task 1.4.2c-1) must remain visible alongside the kill-failed error, since Task 1.3.1 leaves the original process `SIGSTOP`'d (not resumed) on kill failure — clearing the indicator would incorrectly imply the process resumed.
**Files**: `web-app/src/components/sessions/ConfirmKillDialog.tsx` (new), `web-app/src/components/sessions/ImportExternalSessionsPanel.tsx`

##### Task 1.4.3a: Scaffold `ConfirmKillDialog.tsx` with named-target heading, initial focus on Cancel, `--error`/`--error-bg` token usage (~5 min)
- Files: `web-app/src/components/sessions/ConfirmKillDialog.tsx`

##### Task 1.4.3b: Wire row status machine: `idle → loading → imported → (kill-pending → killed | kill-failed)` in the panel (~5 min)
- Files: `web-app/src/components/sessions/ImportExternalSessionsPanel.tsx`

##### Task 1.4.3c: Jest/RTL tests: button hidden pre-import, initial focus, kill-failed leaves imported status intact, Paused indicator stays `true` when kill `Status: Failed` (resolves Phase 4 triad-review repair-loop residual gap: kill-failed × Paused reconciliation) (~5 min)
- Files: `web-app/src/components/sessions/ConfirmKillDialog.test.tsx` (new)

#### Story 1.4.4: E2E coverage + feature registry
**As a** maintainer, **I want** the mux-import happy path covered end-to-end and registered, **so that** CI catches regressions and the feature registry stays accurate.
**Acceptance Criteria**:
- `tests/e2e/import-external-session.spec.ts` starts with `// @feature session:import, session:import-kill`, uses only `data-testid`/ARIA locators, contains no `waitForTimeout`, and exercises: discover → preview → confirm import → confirm kill, against a real ssq-mux-wrapped test fixture session.
  - *Given* the e2e test server has one ssq-mux test session running, *When* the spec runs `page.getByTestId('discovered-session-row-<pid>').click()` then completes the flow, *Then* the new session appears in the session list and the discovery row disappears (or shows "Session ended") within an `expect(...).toHaveText(...)` assertion, never a fixed `waitForTimeout`.
- `docs/registry/features/backend/import-external-session-preview.json`, `.../commit.json`, `.../kill.json` and a frontend entry `docs/registry/features/frontend/import-external-sessions-panel.json` exist with `tested: true` and `testIds` pointing at the new e2e spec's test names; `make registry-generate` run and its output committed.
**Files**: `tests/e2e/import-external-session.spec.ts` (new), `tests/e2e/pages/ImportSessionsPage.ts` (new), `docs/registry/features/backend/*.json`, `docs/registry/features/frontend/*.json`

##### Task 1.4.4a: Add `ImportSessionsPage.ts` page helper with locators for the panel/dialogs (~5 min)
- Files: `tests/e2e/pages/ImportSessionsPage.ts`

##### Task 1.4.4b: Write the e2e spec for the mux happy path (~5 min)
- Files: `tests/e2e/import-external-session.spec.ts`

##### Task 1.4.4c: Add per-feature registry JSON entries with `// +api:`/`// +feature:` markers in the corresponding source files (~4 min)
- Files: `server/services/import_service.go`, `web-app/src/components/sessions/ImportExternalSessionsPanel.tsx`, `docs/registry/features/backend/*.json`, `docs/registry/features/frontend/*.json`

##### Task 1.4.4d: Run `make registry-generate`; commit regenerated aggregate files (~2 min)
- Files: `docs/registry/backend-features.json`, `docs/registry/frontend-features.json`, `docs/registry/coverage-gaps.json`

---

## Phase 2: Manual / Plain-Tmux Import

### Epic 2.1: Plain-tmux candidate enumeration

**Goal**: Discover candidates for import from plain tmux panes running `claude`/`agy` with no ssq-mux wrapper, reusing `session/pty_discovery.go`'s batching pattern.

#### Story 2.1.1: Enumerate plain-tmux Claude/Antigravity panes
**As a** user, **I want** to see plain tmux panes running Claude Code even without ssq-mux, **so that** I can import sessions I started manually.
**Acceptance Criteria**:
- A new function `session.DiscoverPlainTmuxCandidates(ctx) ([]ExternalSessionCandidate, error)` calls the existing `batchPTYInfo`/`batchIsClaudeProcess` machinery, filters to panes already known to `pty_discovery.go` as `externalClaude`, and excludes any pane already tracked by `ExternalSessionDiscovery` (mux-wrapped) to avoid double-listing the same process under two source kinds.
  - *Given* two tmux panes exist, one running `claude` with no ssq-mux env markers and one running `ssq-mux` (already surfaced via mux discovery), *When* `DiscoverPlainTmuxCandidates` runs, *Then* it returns exactly one candidate (`SourceKind: PlainTmux`) for the first pane and excludes the second.
- Uses a single batched `tmux list-panes -a` + one `ps` call for N panes (no N sequential subprocess calls), per the existing `batchPTYInfo` pattern.
**Files**: `session/pty_discovery.go`, `session/import_candidate.go`, `session/import_candidate_test.go`

##### Task 2.1.1a: Add `DiscoverPlainTmuxCandidates` calling existing `batchPTYInfo`/`batchIsClaudeProcess`, with a dedupe check against `ExternalSessionDiscovery.sessions` (~5 min)
- Files: `session/pty_discovery.go`

##### Task 2.1.1b: Map filtered PTY results to `ExternalSessionCandidate{SourceKind: PlainTmux, TmuxSession: <pane target>, PID: <pane_pid>}` (~4 min)
- Files: `session/import_candidate.go`

##### Task 2.1.1c: Tests: mixed mux/plain panes produce correct exclusion; multiple plain panes in the same directory both surface as separate candidates (not merged) (~5 min)
- Files: `session/import_candidate_test.go`

#### Story 2.1.2: PID-based correlation is primary for plain-tmux (no path-heuristic-first shortcut)
**As a** backend developer, **I want** plain-tmux candidates to always attempt PID-based correlation first, **so that** the multi-process-per-directory ambiguity flagged in pitfalls research is minimized before falling back to path heuristics.
**Acceptance Criteria**:
- `CorrelateCandidate` (Story 1.1.2) already does PID-first; this story adds a test proving plain-tmux candidates (which always have a real PID from `tmux list-panes -F '#{pane_pid}'`) exercise the PID path, not the path-fallback, whenever the process has a resolvable open file.
  - *Given* a plain-tmux candidate with PID 9911 whose open files resolve via `HistoryFileDetector.Detect(9911)`, *When* `CorrelateCandidate` runs, *Then* the result is `Resolved{Confidence: PIDExact}`, never falling through to `DetectByPath`.
**Files**: `session/import_correlate_test.go`

##### Task 2.1.2a: Add plain-tmux-specific correlation test case (~4 min)
- Files: `session/import_correlate_test.go`

---

### Epic 2.2: Kill primitive for plain-tmux sessions

**Goal**: A kill path for candidates that were never `InstanceTypeExternal` and have no `ExternalMetadata.TmuxSessionName` for `KillExternalSession()` to use.

#### Story 2.2.1: `KillPlainTmuxPane` primitive
**As a** backend developer, **I want** a kill function scoped to plain-tmux panes, **so that** confirm-kill for the manual import path doesn't misuse the mux-specific `KillExternalSession()`.
**Acceptance Criteria**:
- `KillPlainTmuxPane(paneTarget string, expectedIdentity PIDIdentity) (KillOutcome, error)` re-verifies `procinfo.ProcessInspector.IsAlive(expectedIdentity.PID, expectedIdentity.CreateTimeMs)` immediately before acting, then sends the resolved unresolved-question's chosen signal target (default: `tmux kill-session -t <paneTarget>`, per the resolved Unresolved Question in this plan — see note below), never a raw `-pid` process-group kill (must-not-happen #2).
  - *Given* `paneTarget = "plain-claude:0.0"` and the expected identity matches the live process, *When* `KillPlainTmuxPane` runs, *Then* it shells out to `tmux kill-session -t plain-claude` (the session containing that pane) and returns `KillOutcome{Status: Killed}`.
  - *Given* the expected identity does not match (create-time mismatch), *When* `KillPlainTmuxPane` runs, *Then* it returns `KillOutcome{Status: AlreadyGone}` without sending any signal.
- Never issues `syscall.Kill(-pgid, ...)` against a plain-tmux target — group-kill risk explicitly excluded per pitfalls research (could hit the user's shell/tmux server).
**Files**: `session/import_kill.go`, `session/import_kill_test.go`

##### Task 2.2.1a: Implement `KillPlainTmuxPane` using `tmux kill-session` (session-level, not pane-level, to match `KillExternalSession`'s existing precedent) with the pre-kill identity re-check (~5 min)
- Files: `session/import_kill.go`

##### Task 2.2.1b: Tests: kill success, identity-mismatch skip, tmux-command-failure surfaced (~5 min)
- Files: `session/import_kill_test.go`

#### Story 2.2.2: Wire `ConfirmKillExternalSession` to dispatch on `SourceKind`
**As a** backend developer, **I want** the existing confirm-kill RPC to route to `KillExternalSession()` for mux candidates and `KillPlainTmuxPane` for plain-tmux ones, **so that** one RPC serves both discovery paths without a speculative `Killable` interface.
**Acceptance Criteria**:
- `ConfirmKillExternalSession`'s implementation is a table-driven `switch candidate.SourceKind { case MuxDiscovered: ...; case PlainTmux: ... }`.
  - *Given* a `PlainTmux` candidate, *When* `ConfirmKillExternalSession` is called, *Then* `KillPlainTmuxPane` is invoked and `Instance.KillExternalSession()` is not.
**Files**: `session/import_kill.go`, `session/import_kill_test.go`

##### Task 2.2.2a: Add the switch dispatch (~3 min)
- Files: `session/import_kill.go`

##### Task 2.2.2b: Test asserting correct dispatch per `SourceKind` (~4 min)
- Files: `session/import_kill_test.go`

---

### Epic 2.3: Frontend — manual pointer entry

**Goal**: Let a user who has no ssq-mux wrapper point stapler-squad at a directory/UUID/plain-tmux pane and run the same preview/commit/kill flow.

#### Story 2.3.1: Manual candidate entry form
**As a** user, **I want** to manually point at a plain tmux pane or directory to import, **so that** sessions started without ssq-mux are still importable.
**Acceptance Criteria**:
- A new "Add manual candidate" affordance in `ImportExternalSessionsPanel.tsx` lets the user either pick from the plain-tmux enumeration list (Story 2.1.1's candidates, shown automatically) or type a directory path directly; typed paths get previewed through the same `PreviewImportExternalSession` RPC with `SourceKind: PlainTmux` and no PID (path-only correlation).
  - *Given* the plain-tmux enumeration already found a candidate at `/Users/x/proj2`, *When* the panel renders, *Then* that candidate appears in the same table as ssq-mux candidates, visually distinguished (e.g. a "Plain tmux" badge) rather than a separate list — matches the git-GUI "untracked files visually distinguished" pattern from UX research.
- No change to the preview/commit/kill dialogs from Phase 1 — they already branch on `SourceKind` from the response.
**Files**: `web-app/src/components/sessions/ImportExternalSessionsPanel.tsx`, `web-app/src/components/sessions/ImportExternalSessionsPanel.css.ts`

##### Task 2.3.1a: Add "Plain tmux" source badge to row rendering (~3 min)
- Files: `web-app/src/components/sessions/ImportExternalSessionsPanel.tsx`

##### Task 2.3.1b: Add manual-path entry input + submit wiring to preview RPC (~5 min)
- Files: `web-app/src/components/sessions/ImportExternalSessionsPanel.tsx`

##### Task 2.3.1c: Jest/RTL tests: badge rendering, manual entry triggers preview with `SourceKind: PlainTmux` (~5 min)
- Files: `web-app/src/components/sessions/ImportExternalSessionsPanel.test.tsx`

#### Story 2.3.2: E2E coverage for plain-tmux path
**As a** maintainer, **I want** the plain-tmux import path covered end-to-end, **so that** the second discovery path doesn't regress silently.
**Acceptance Criteria**:
- `tests/e2e/import-external-session-plain-tmux.spec.ts` (or an added `describe` block in the Phase 1 spec) covers: plain-tmux pane appears with "Plain tmux" badge → preview → confirm import → confirm kill using `KillPlainTmuxPane`, with the same locator/no-waitForTimeout conventions.
**Files**: `tests/e2e/import-external-session.spec.ts`, `tests/e2e/pages/ImportSessionsPage.ts`

##### Task 2.3.2a: Extend the e2e spec with the plain-tmux case (~5 min)
- Files: `tests/e2e/import-external-session.spec.ts`

##### Task 2.3.2b: Update feature registry entries for the plain-tmux kill primitive (~3 min)
- Files: `docs/registry/features/backend/import-external-session-kill.json`

---

## Phase 3: Batch Import + Multi-Program (Antigravity)

### Epic 3.1: Batch import orchestration

**Goal**: Import N candidates in one user action with independent per-item outcomes, never a single aggregate boolean.

#### Story 3.1.1: `BatchImportExternalSessions` RPC
**As a** user, **I want** to import several discovered sessions at once, **so that** I don't have to repeat the preview/confirm flow one at a time.
**Acceptance Criteria**:
- New RPC `BatchImportExternalSessions(BatchImportExternalSessionsRequest) returns (BatchImportExternalSessionsResponse)` taking a list of `(candidate, disambiguation_choice)` pairs, returning a list of `ImportOutcome`, one per input item, in the same order.
  - *Given* a batch of 3 candidates where candidate 2 has an unresolved `CorrelationAmbiguity` and no `disambiguation_choice`, *When* `BatchImportExternalSessions` runs, *Then* candidates 1 and 3 each get an independent `ImportOutcome{Status: Success}` (assuming no other failure) and candidate 2 gets `ImportOutcome{Status: Failed, Error: "disambiguation required"}` — items 1 and 3 are unaffected by item 2's failure.
- Internally, this is a fan-out loop calling the exact same `CommitImportExternalSession` function used by the single-import RPC — no new commit logic, no shared transaction.
**Files**: `proto/session/v1/import.proto`, `server/services/import_service.go`, `session/import_batch.go` (new)

##### Task 3.1.1a: Add `BatchImportExternalSessionsRequest`/`Response` proto messages; regenerate (~4 min)
- Files: `proto/session/v1/import.proto`, generated bindings

##### Task 3.1.1b: Implement `BatchImportExternalSessions` as a sequential (not parallel — avoid N concurrent tmux/DB writes racing) loop over `CommitImportExternalSession`, collecting one `ImportOutcome` per item (~5 min)
- Files: `session/import_batch.go`

##### Task 3.1.1c: Wire RPC handler (~3 min)
- Files: `server/services/import_service.go`

##### Task 3.1.1d: Tests: mixed success/failure batch, order preservation, no rollback of successes (~5 min)
- Files: `session/import_batch_test.go`

#### Story 3.1.2: Batch kill-confirmation — only successfully-imported sessions offered
**As a** user, **I want** batch kill-confirmation to only ever list sessions that actually imported successfully, **so that** I can never accidentally kill a session whose import failed.
**Acceptance Criteria**:
- The frontend's post-batch summary only renders "End original session" affordances for rows with `ImportOutcome.Status == Success`; failed rows show their error and no kill option at all.
  - *Given* a batch result of 4 successes and 1 failure, *When* the summary renders, *Then* exactly 4 "End original session" buttons are present and the failed row has no such button, matching must-not-happen #6.
- Per Unresolved Question in this plan, ship with **one confirm dialog per session** (not one batch dialog) for Phase 3 v1, revisit if soak feedback indicates otherwise.
**Files**: `web-app/src/components/sessions/ImportExternalSessionsPanel.tsx`, `web-app/src/components/sessions/BatchImportSummary.tsx` (new)

##### Task 3.1.2a: Build `BatchImportSummary.tsx` per-row result list with "N of M imported" header, modeled on the email-import-wizard pattern from UX research (~5 min)
- Files: `web-app/src/components/sessions/BatchImportSummary.tsx`

##### Task 3.1.2b: Gate kill affordance rendering on `Status == Success` per row (~3 min)
- Files: `web-app/src/components/sessions/BatchImportSummary.tsx`

##### Task 3.1.2c: Jest/RTL test: failed row never renders a kill button (~4 min)
- Files: `web-app/src/components/sessions/BatchImportSummary.test.tsx` (new)

#### Story 3.1.3: Batch selection UI (checkbox table)
**As a** user, **I want** to select multiple discovered sessions via checkboxes, **so that** I can trigger a batch import in one action.
**Acceptance Criteria**:
- Header checkbox with `aria-label="Select all discovered sessions"`, tri-state `indeterminate` set via DOM property when some-but-not-all rows selected, modeled directly on `ApprovalAnalyticsPanel.tsx`'s existing "Select all" pattern.
  - *Given* 3 of 5 rows are checked, *When* the header checkbox renders, *Then* its `indeterminate` DOM property is `true` and `aria-checked="mixed"`.
  - *Given* ≥1 row is checked, *Then* an "Import N selected" bulk action bar renders and its enabled-state change is announced via `aria-live="polite"`.
**Files**: `web-app/src/components/sessions/ImportExternalSessionsPanel.tsx`

##### Task 3.1.3a: Add header checkbox + tri-state logic (~5 min)
- Files: `web-app/src/components/sessions/ImportExternalSessionsPanel.tsx`

##### Task 3.1.3b: Add bulk action bar + `aria-live` selection-count announcement (~4 min)
- Files: `web-app/src/components/sessions/ImportExternalSessionsPanel.tsx`

##### Task 3.1.3c: Jest/RTL tests: indeterminate state, bulk bar visibility, announced count (~5 min)
- Files: `web-app/src/components/sessions/ImportExternalSessionsPanel.test.tsx`

---

### Epic 3.2: Antigravity (multi-program) support

**Goal**: Extend both discovery and commit to handle non-Claude programs already supported by `AgyAdapter`, without refactoring `PortSessionHistory` (per build-vs-buy's finding that it assumes an already-managed `Instance`).

#### Story 3.2.1: Antigravity running-process detector
**As a** backend developer, **I want** a `HistoryFileDetector`-equivalent for Antigravity's on-disk layout, **so that** running Antigravity processes can be correlated to their history file the same way Claude processes are.
**Acceptance Criteria**:
- A new `AntigravityHistoryDetector` (or an extension to the existing detector abstraction) resolves a PID/path to Antigravity's sqlite-backed history file (per `history_transfer.go:85`'s `~/.gemini/antigravity-cli/history.jsonl`-style layout — path confirmed during Phase 3 kickoff per the Unresolved Questions section).
  - *Given* a candidate with `Program: "agy"` and a resolvable PID, *When* correlation runs, *Then* `CorrelateCandidate` dispatches to the Antigravity detector instead of `HistoryFileDetector`, based on `candidate.Program`, and returns the same `CorrelationResult` shape as the Claude path.
**Files**: `session/agy_history_detector.go` (new), `session/agy_history_detector_test.go` (new), `session/import_correlate.go`

##### Task 3.2.1a: Confirm actual Antigravity on-disk layout against a real installation (spike, per Unresolved Questions) (~5 min)
- Files: none (research task; findings recorded as a comment in the next task's file)

##### Task 3.2.1b: Implement `AntigravityHistoryDetector.Detect(pid)`/`DetectByPath(path)` mirroring `HistoryFileDetector`'s signature (~5 min)
- Files: `session/agy_history_detector.go`

##### Task 3.2.1c: Dispatch on `candidate.Program` inside `CorrelateCandidate` (~4 min)
- Files: `session/import_correlate.go`

##### Task 3.2.1d: Tests for the new detector (~5 min)
- Files: `session/agy_history_detector_test.go`

#### Story 3.2.1b: Read Antigravity canonical entries without a live `Instance` (resolves architecture-review Blocker 3)
**As a** backend developer, **I want** an `AgyAdapter` equivalent of `ReadCanonicalTurnsFromFile` (Story 1.1.4), **so that** the Antigravity commit path never needs to construct a synthetic/partial `Instance` just to read history — the same antipattern Story 1.1.4 eliminated for Claude must not be reintroduced here.
**Acceptance Criteria**:
- `AgyAdapter.Import` currently takes `(ctx, *Instance)` and calls real `*Instance` methods (`GetClaudeConversationUUID`-equivalent, `GetWorkingDirectory`, etc.). Add a small, additive helper `ReadCanonicalEntriesFromFile(path string) ([]CanonicalTurn, error)` (or, if Story 3.2.1a's spike finds a sqlite-backed store rather than JSONL, `ReadCanonicalEntriesFromStore(path string) ([]CanonicalTurn, error)` reading the sqlite file directly) that both the existing `AgyAdapter.Import` and the new commit path delegate to — mirroring the `ReadCanonicalTurnsFromFile` refactor exactly, never building a partial `Instance`.
  - *Given* an Antigravity history file/store containing 3 entries, *When* `ReadCanonicalEntriesFromFile`/`ReadCanonicalEntriesFromStore` runs against a bare path, *Then* it returns 3 `CanonicalTurn`s with no `*Instance` constructed anywhere in the call chain.
- This story is a hard prerequisite for Story 3.2.2 — Story 3.2.2 may not proceed until this extraction exists, exactly as Story 1.1.4 was a hard prerequisite for Story 1.1.3.
**Files**: `session/agy_adapter.go`, `session/agy_adapter_test.go`

##### Task 3.2.1b-1: Extract the entry-reading body of `AgyAdapter.Import` into `ReadCanonicalEntriesFromFile`/`ReadCanonicalEntriesFromStore` (path/format depends on Story 3.2.1a's spike finding) (~5 min)
- Files: `session/agy_adapter.go`

##### Task 3.2.1b-2: Make `AgyAdapter.Import(ctx, inst)` a thin wrapper calling the new bare-path function with `inst.HistoryFilePath()` (~4 min)
- Files: `session/agy_adapter.go`

##### Task 3.2.1b-3: Unit tests for the bare-path reader (~4 min)
- Files: `session/agy_adapter_test.go`

#### Story 3.2.2: Commit path calls `AgyAdapter` directly via bare-path reads (not `PortSessionHistory`, not a synthetic `Instance`)
**As a** backend developer, **I want** import's commit step to call the bare-path `HistoryAdapter` reader directly for non-Claude programs, **so that** I don't need to refactor `PortSessionHistory`'s already-managed-`Instance` assumption, and I don't reintroduce the synthetic-`Instance` antipattern Story 1.1.4/3.2.1b exist specifically to prevent.
**Acceptance Criteria**:
- `CommitImportExternalSession` selects `srcAdapter := AdapterFor(candidate.Program)`, calls the bare-path reader (`srcAdapter.ReadCanonicalEntriesFromFile(candidate.HistoryFilePath)` for Claude, `ReadCanonicalEntriesFromFile`/`ReadCanonicalEntriesFromStore` for Agy — never `srcAdapter.Import(ctx, syntheticSourceInstance)`) to get `[]CanonicalTurn`, then `dstAdapter.Export(ctx, turns, newManagedInstance)` against the real, already-persisted destination `Instance` — never calling `PortSessionHistory` and never constructing a synthetic source `Instance`.
  - *Given* a candidate with `Program: "agy"`, *When* commit runs, *Then* `AgyAdapter.ReadCanonicalEntriesFromFile`/`...FromStore` is called (not `ClaudeAdapter.Import`, not `AgyAdapter.Import` against a synthetic `Instance`) and the resulting turns are exported into the new, real instance via the destination adapter selected by the new instance's configured program.
- Depends on Story 3.2.1b existing first.
**Files**: `session/import_commit.go`, `session/import_commit_test.go`

##### Task 3.2.2a: Add adapter-selection-by-program branch to commit, calling each adapter's bare-path reader rather than `Import(ctx, *Instance)` (~5 min)
- Files: `session/import_commit.go`

##### Task 3.2.2b: Test committing an Antigravity candidate exercises `AgyAdapter.ReadCanonicalEntriesFromFile`/`...FromStore`, never a synthetic `Instance`, never `PortSessionHistory` (~5 min)
- Files: `session/import_commit_test.go`

#### Story 3.2.3: E2E + registry closeout
**As a** maintainer, **I want** the full feature set registered and covered, **so that** the feature registry has no net-new coverage gap by the end of Phase 3.
**Acceptance Criteria**:
- All new backend RPCs and frontend components from Phases 1–3 have entries in `docs/registry/features/{backend,frontend}/` with `tested: true`; `make registry-generate` shows no net increase in `docs/registry/coverage-gaps.json` count relative to the pre-feature baseline.
**Files**: `docs/registry/features/backend/*.json`, `docs/registry/features/frontend/*.json`, `docs/registry/coverage-gaps.json`

##### Task 3.2.3a: Add remaining registry entries for batch RPC and Antigravity detector (~4 min)
- Files: `docs/registry/features/backend/*.json`

##### Task 3.2.3b: Run `make registry-generate`; verify no coverage-gap regression; commit (~3 min)
- Files: `docs/registry/backend-features.json`, `docs/registry/frontend-features.json`, `docs/registry/coverage-gaps.json`

---

## Repair Log (Iteration 1)

Two independent reviews (`implementation/architecture-review.md`, `implementation/adversarial-review.md`) found 6 BLOCKER issues. Each is resolved by a structural change to the plan below, not a caveat sentence:

1. **Architecture Blocker — `ConfirmKillExternalSession` depends on a `PIDIdentity` no earlier RPC returns.** Fixed by adding a `pid_identity` field to `PreviewImportExternalSessionResponse` (Story 1.1.3, Task 1.1.3a/1.1.3c) as the single source of truth for the value, echoing a freshly-re-read `pid_identity` back out of `CommitImportExternalSessionResponse` (Story 1.2.1, Task 1.2.1a), and adding an explicit frontend task (Story 1.4.2, Task 1.4.2d) to thread it through row state into `ConfirmKillExternalSessionRequest` (Story 1.3.1, Task 1.3.1a).

2. **Architecture Blocker — handler-to-handler RPC call (`ImportService` invoking `SessionService.CreateSession`'s connect handler).** Fixed by inserting new Story 1.2.0a, which extracts a plain domain function `session.CreateManagedInstance(ctx, params) (*Instance, error)` out of `SessionService.CreateSession`'s inline logic; both `SessionService.CreateSession`'s connect handler and `session.CommitImportExternalSession` (Story 1.2.1, Task 1.2.1b) now call this shared function — no handler calls another handler.

3. **Architecture Blocker — Antigravity commit path reintroduces the synthetic-partial-`Instance` antipattern Story 1.1.4 eliminated.** Fixed by inserting new Story 3.2.1b, which extracts `ReadCanonicalEntriesFromFile`/`ReadCanonicalEntriesFromStore` out of `AgyAdapter.Import` (mirroring Story 1.1.4's `ReadCanonicalTurnsFromFile` exactly), and rewriting Story 3.2.2 so commit calls that bare-path reader directly instead of `srcAdapter.Import(ctx, syntheticSourceInstance)`.

4. **Adversarial Blocker — commit-before-kill ordering risks dual JSONL writers.** Fixed by inserting new Story 1.2.0b (a mandatory pre-implementation spike into `claude --resume`'s concurrent-open behavior) and new Task 1.2.1e in Story 1.2.1, which `SIGSTOP`s the original process immediately before starting the resumed one (re-verified via `procinfo.ProcessInspector.IsAlive` first), `SIGCONT`s it back on commit failure, and leaves it suspended until kill is confirmed or cancelled. New Story 1.3.3 adds the resume-on-abandon guard so a suspended-but-never-killed process doesn't stay frozen forever.

5. **Adversarial Blocker — no defined behavior for correlation drift between preview and commit.** Fixed by adding `expected_correlation` as a required input to `CommitImportExternalSessionRequest` (Story 1.2.1, Task 1.2.1a) and new Task 1.2.1f, which compares commit-time re-correlation against the previewed value and aborts with `connect.CodeFailedPrecondition` ("the matched conversation changed since preview — please re-preview") on any mismatch, before any `Instance` is created.

6. **Adversarial Blocker — no worktree/path-collision check before commit.** Fixed by inserting new Story 1.2.4, which checks `candidate.Path` (and its resolved worktree root) against every existing `Instance`'s working directory/worktree before commit proceeds, returning `connect.CodeAlreadyExists` and creating nothing when a collision is found.

## Repair Log (Iteration 2 — Phase 4 readiness gate, pre-mortem P1s)

`implementation/pre-mortem.md` identified 3 P1 failure modes as part of Phase 4 validation. Per the readiness-gate rule for pre-mortem P1 items, each is resolved by a structural change below rather than left as an open risk:

1. **Pre-mortem P1 #1 — suspended-PID state has no durable record; a server restart while a process is `SIGSTOP`'d leaves it frozen forever.** Fixed by extending Task 1.2.1e to persist a `SuspendedProcessRecord{PID, CreateTimeMs, CandidateRef, InstanceID}` (via a new `session/suspended_process_store.go`) before sending `SIGSTOP`, removing it on resume/kill, and adding new Task 1.2.1h — a startup reconciliation pass (`session.ReconcileSuspendedProcesses`) that `SIGCONT`s any PID left suspended from a prior server incarnation and compensating-deletes any orphaned partial `Instance`.

2. **Pre-mortem P1 #2 — `HistoryFileDetector` correlation has never been validated against genuinely unmanaged processes; the "one-click import" premise may not hold in practice.** Fixed by inserting new Story 1.1.2b, a mandatory correlation-feasibility go/no-go spike (≥10 real unmanaged processes, 80% resolve-rate threshold) that gates Story 1.1.3 — `PreviewImportExternalSession` may not be implemented until the spike clears threshold or the correlation heuristic is revised and re-measured.

3. **Pre-mortem P1 #3 — "abandon kill" only `SIGCONT`s the original process while the committed `Instance` stays alive, recreating the dual-writer hazard `SIGSTOP` was built to prevent.** Fixed by rewriting Story 1.3.3 so `CancelPendingKill` means "abandon the entire import": it now compensating-deletes the committed `Instance` (reusing Story 1.2.3's delete path) *before* resuming the original process, and refuses to resume at all if that delete fails — closing the exact race the failure mode describes.

All 3 P1 items from `pre-mortem.md` are now addressed structurally in the plan above; see that file's checklist for the original failure descriptions.
