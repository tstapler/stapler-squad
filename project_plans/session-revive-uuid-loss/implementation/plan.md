# Implementation Plan: session-revive-uuid-loss

**Feature**: Attempt on-disk conversation-UUID recovery before the cold-restore
resume/fresh decision is baked into the launch command, and make a forced
fresh-start after a failed recovery durably visible to the user.
**Date**: 2026-08-06
**Status**: Ready for implementation
**ADRs**: None — see "Step 3: ADR judgment call" below for the explicit reasoning.

---

## Step 0.5: Creative pass — alternatives considered

1. **(a) Move the recovery attempt earlier + reorder `initTmuxSession()`.**
   Strength: minimal, localized change — reuses `tryExtractConversationUUID`'s
   existing fallback body unchanged, and keeps `initTmuxSession()`'s single
   responsibility ("build a command from already-decided state") intact.
   Weakness: requires restructuring the call order in two near-duplicate
   ~90-line blocks (`startLocked` and `start`), so the extraction has to be
   done carefully to avoid drift (exactly the failure mode AC4 already warns
   about).

2. **(b) Make `initTmuxSession()` itself resume-aware** by re-resolving the
   UUID inside it, right before building the command, instead of trusting
   whatever `i.claudeSession.ConversationUUID` already holds.
   Strength: structurally guarantees the invariant "the launch command always
   reflects the freshest known UUID" at a single choke point — impossible to
   forget at a call site.
   Weakness: `initTmuxSession()` runs unconditionally on **every** start path
   (first-time setup, hot restore, cold restore) and currently knows nothing
   about `firstTimeSetup`/`pm().IsAlive()`/`HasClaudeSession()`. Folding a
   `DetectByPath` scan in there would run it even when Goal 2/AC2 requires
   zero added work (first-time setup), and would require duplicating the
   `!firstTimeSetup && !IsAlive() && !HasClaudeSession()` guard inside a
   function whose job is "build a command," not "decide whether to recover" —
   exactly the guard-duplication risk pitfalls.md's design-against-checklist
   item 1 warns against.

3. **(c) Two-phase: always launch with `--resume` if a UUID is *ever* found
   via recovery, reconcile after the fact if Claude actually started a new
   conversation anyway.**
   Strength: sidesteps needing to move anything before `initTmuxSession()` at
   all — keep today's call order, just always attempt `--resume` speculatively.
   Weakness: this is exactly the shape of bug `recoverFromStaleResume`
   (`session/instance_claude.go:78-96`) already exists to break — blindly
   trying `--resume` with a UUID that might not be valid causes Claude to
   exit immediately with `"No conversation found with session ID"`, and it
   still doesn't solve AC1's core case (a UUID that was **never** captured at
   all) because there's nothing to speculate with before recovery has
   actually run once.

**Chosen: (a).** Rejected alternatives and reasons are also recorded in the
Pattern Decisions table below (required by these instructions, not just
listed here).

---

## Domain Glossary

| Term | Definition | Notes |
|------|-----------|-------|
| **ColdRestore** | The code branch taken inside `startLocked`/`start` when `!firstTimeSetup && !i.pm().IsAlive()` — the tmux pane is confirmed dead and a new tmux session must be created to either resume or start fresh. | Existing concept (`session/instance.go:878-921`, `:1068-1127`); not renamed, just named here for the GWT examples below. |
| **RecoverySuppressed** | A one-shot `bool` field on `Instance`, set to `true` inside `ClearConversationState()` whenever an intentional clear happens (stale-`--resume` rejection, leaving the Claude/Antigravity program family, or the user-initiated `ClearConversationState` RPC). Read-and-reset to `false` exactly once by the next `prepareColdRestore()` call. | New. Prevents the new recovery call from immediately re-discovering the same UUID/JSONL a deliberate clear just walked away from (pitfalls.md Risk 2). |
| **EverHadConversationHistory** | A durable `bool` field on `Instance`, persisted alongside `HistoryFilePath`. Set to `true` the first time a non-empty `ConversationUUID` is ever captured for this instance (via `SetHistoryInfo` or a successful recovery). Reset to `false` only by `ClearConversationState()` — an intentional "start over" also resets this bookkeeping, not just the live UUID pointer. | New. This is the field that lets Goal 2/AC3 distinguish "genuinely first conversation" (never true) from "had one, recovery couldn't find it" (true, recovery came back empty) — set at capture/clear time, never inferred from field emptiness (pitfalls.md finding 3/4). |
| **prepareColdRestore** | New unexported `(*Instance) prepareColdRestore() coldRestoreOutcome` method, called from both `startLocked` and `start` inside the `ColdRestore` branch, **before** `initTmuxSession()` runs. Encapsulates: honor `RecoverySuppressed`, else attempt the existing `tryExtractConversationUUID`'s path-only fallback body, and compute this cycle's `ReviveOutcome`. | New. The single shared helper AC4 requires — see Pattern Decisions. |
| **coldRestoreOutcome** | The small return struct from `prepareColdRestore`: `{ Resume bool; Outcome ReviveOutcome }`. `Resume == true` means the caller should expect `i.claudeSession.ConversationUUID` to be populated before calling `initTmuxSession()`. | New, unexported (single-package use). |
| **ReviveOutcome** | A new 4-value enum (Go string-const type + mirrored proto enum) recording the outcome of the most recent start/restart cycle: `RESUME_LIVE` (had a UUID already, no recovery needed), `RESUME_RECOVERED` (path-fallback found a UUID missing from memory), `FRESH_EXPECTED` (legitimate fresh start — first-time setup, recovery suppressed, or genuinely no prior history), `FRESH_LOST_HISTORY` (recovery ran, found nothing, but `EverHadConversationHistory` was true — the AC3 signal). | New. Persisted on `Instance.LastReviveOutcome`, exposed via `proto/session/v1/types.proto`, and threaded into the `EventStarted` lifecycle-event reason string. |
| **ReasonColdRestoreLostHistory** | The `string` reason constant (`"cold-restore-fresh-lost-history"`) passed to `i.fireLifecycleEvent(EventStarted, reason)` when `ReviveOutcome == FRESH_LOST_HISTORY`. | New. Follows the existing `"reconcile-session-revived"` / `"reconcile-session-hibernated-but-alive"` reason-string precedent in `session/review_queue_poller.go`. |
| **onColdRestoreLostHistory** | New `SessionService` method, same shape as `onRateLimitRecovery` (`server/services/session_service.go:4001`), that publishes an `events.NewNotificationEvent(...)` (WARNING/MEDIUM) when it observes `EventStarted` with `reason == ReasonColdRestoreLostHistory`, gated on `!inst.Hidden`. | New. Wired as a `session.LifecycleListener` in `wireCallbacks`, mirroring `autoArchiveListener`'s registration pattern. |
| **RevivedContextBadge** | New frontend component: a small aria-labeled badge rendered on `SessionCard`/`SessionRow`, sibling to the existing `StatusBadge`, shown when `session.reviveOutcome === ReviveOutcome.FRESH_LOST_HISTORY`. | New. Follows `ConnectionIndicator.tsx`'s accessibility shape (`role="status"`, `aria-live="polite"`, `aria-hidden` icon, full-sentence `aria-label`). |

9 glossary terms.

---

## Step 3: Technology / pattern validation and ADR judgment call

Every pattern below reuses an existing mechanism already present in this
codebase (actor-serialized `Instance` methods, `LifecycleEvent`/reason
strings, `pkg/events.NewNotificationEvent`, proto enum + TS `assertNever`
exhaustiveness, `sessionStorage`-scoped dismissal, JSON-tagged persisted
struct fields). No new dependency, no new cross-cutting architectural
direction, no new subsystem — see `research/build-vs-buy.md`'s "build,
nothing new" conclusion. Per the instructions' own guidance ("likely no
formal ADR needed for a bugfix this scoped, but say so explicitly"): **this
plan explicitly decides not to write a formal ADR.** The one genuinely new
piece of vocabulary — the `ReviveOutcome` enum — is scoped, small, and
follows the exact structural precedent of `DetectedStatus`/`AttentionReason`
(existing proto enums with an `assertNever`-exhaustive frontend switch), so
it doesn't rise to "decision that needs to survive independently of the
code" the way e.g. ADR-009 (vanilla-extract) or the workspace-isolation
model do.

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| Recovery call ordering | Move the recovery attempt before `initTmuxSession()`, restructuring both cold-restore blocks' call order | Step 0.5 (a) | (b) Make `initTmuxSession()` itself resume-aware | `initTmuxSession()` runs unconditionally on every start path including first-time-setup and hot-restore; folding the `DetectByPath` scan in there blurs its single responsibility and duplicates the `!firstTimeSetup && !IsAlive() && !HasClaudeSession()` guard in a place not designed to hold it |
| Recovery call ordering | (same) | | (c) Always speculatively launch with `--resume`, reconcile after the fact | Reintroduces exactly the stale-resume loop `recoverFromStaleResume` already exists to break, and still doesn't solve AC1 for the "never captured any UUID" case since there is nothing to speculate with before recovery has run once |
| Shared logic between `startLocked`/`start` | One unexported `prepareColdRestore` helper called from both | architecture.md recommendation; AC4 | Two independently patched copies of the decision logic | AC4 explicitly forbids duplicated divergent logic; the two existing blocks already show comment drift from being maintained independently (research's stack.md L906-909 vs L1107-1111) — the exact failure mode that let this bug live in only one of the two paths' documentation |
| Intentional-clear vs. accidental-loss | One-shot `RecoverySuppressed` flag set **inside** `ClearConversationState()` itself | pitfalls.md Risk 2 | Per-call-site flags set individually by `recoverFromStaleResume`/`SwitchProgram`/the `ClearConversationState` RPC | Centralizing in `ClearConversationState()` covers all three existing callers (including the RPC-triggered "start over") with one write, instead of requiring every future caller to remember to also set a flag — the same DRY principle AC4 already applies to the two cold-restore call sites |
| "Had history before" signal | Durable `EverHadConversationHistory` bool set at capture/clear time | pitfalls.md finding 3/4 — "set at the point of loss, not inferred after the fact" | Infer "had history" from `HistoryFilePath == ""` at decision time | All 3 existing `ClearConversationState()` call sites already leave `HistoryFilePath` empty for legitimate fresh-start cases (Goal 4) — inferring from emptiness cannot distinguish those from the unexpected-loss case this fix targets |
| Decision-time UUID recovery mechanism | Reuse `tryExtractConversationUUID`'s exact path-fallback body via the shared helper, gated by the same "only when `HasClaudeSession()` is false" precondition it already has | stack.md; pitfalls.md design-against-checklist #1 | New `Resolver`/`Recovery` interface type | `.claude/rules/interface-pollution-checklist.md` — one concrete method set already fits the need; no second implementation exists or is imminent |
| User-visible signal transport | Reuse `LifecycleEvent`/`fireLifecycleEvent` reason string + `pkg/events.NewNotificationEvent`, mirroring `onRateLimitRecovery`'s exact shape | architecture.md; ux.md | Inventing a new session-level event/notice primitive | No existing generic mechanism was found, but the closest analog (`onRateLimitRecovery`) already solves an architecturally identical problem (auto-heal action needs a `Hidden`-gated, best-effort, durable notification) — a new mechanism would duplicate it for no benefit |
| Durable field shape | Single `ReviveOutcome` enum (proto + Go string-const type) | ux.md "resumed vs. lost-and-fresh" distinction; pitfalls.md (c)'s asymmetry note (resumed-via-fallback deserves a lighter note too) | Two independent booleans (`resumedViaFallback`, `lostHistory`) | An enum makes the 4 mutually-exclusive states exhaustively switchable on the frontend (matches `DetectedStatus`/`AttentionReason`'s existing `assertNever` pattern in `StatusBadge.tsx`) instead of requiring callers to reason about invalid boolean combinations (e.g. `resumedViaFallback && lostHistory` should never both be true) |
| Frontend surface | Toast (existing `Notifier`/`NotificationToast` pipeline, zero new frontend code) + a durable aria-labeled badge on `SessionCard`/`SessionRow` | ux.md recommended composite pattern (layers 1+2) | Also building the full `MemoryPressureCallout`-style dismissible in-detail-panel banner (ux.md layer 3) now | AC3's "at minimum" bar is met by toast + badge; the banner is ux.md's fully layered recommendation but adds real component/test surface beyond what any AC requires — flagged as a fast-follow in Unresolved Questions instead of expanding this plan's scope |
| Same-directory `DetectByPath` ambiguity | Accept as an inherited, documented limitation (matches `session-resume-uuid-fix`'s own R3 stance); add an explicit test naming it, no new disambiguation logic | pitfalls.md §b; architecture.md Risk 1 | Add `HistoryFilePath`-match cross-check or content-level `cwd` verification before trusting a recovered UUID | Out of scope for this bugfix's ACs (none of AC1-AC6 mention cross-session disambiguation); research explicitly frames this as a pre-existing, already-accepted limitation, not a regression introduced by this fix — a stronger fix is a candidate follow-up item, not blocking here |

---

## Migration Plan

Additive-only. Three new persisted/wire fields, each with a safe zero-value
default so existing sessions (created before this fix ships) round-trip
correctly with no backfill:

- `Instance.recoverySuppressed bool` (in-memory only, not persisted — always
  starts `false`, which is correct for every existing session).
- `Instance.everHadConversationHistory bool` and `Instance.LastReviveOutcome
  ReviveOutcome` — both added to the JSON-persisted struct in
  `session/storage.go` with `,omitempty`-style tags, same pattern as the
  existing `HistoryFilePath string \`json:"history_file_path,omitempty"\`` at
  `session/storage.go:111`. An existing session loaded from disk with no
  `ever_had_conversation_history` key deserializes to `false` (Go zero
  value) — this is a conservative default: such a session's first cold
  restart after this fix ships will be treated as "no known prior history"
  even if it did in fact have a conversation, so it will not spuriously fire
  a `FRESH_LOST_HISTORY` notification for pre-existing sessions until the
  next real conversation capture sets the flag. No migration script needed.
- `revive_outcome` proto field (72) on `Session` — proto3 fields are
  optional-by-default; old clients reading a `Session` from a server that
  hasn't shipped this field see the zero value (`REVIVE_OUTCOME_UNSPECIFIED`)
  and unrecognized field skipping is already proto3's default behavior.

---

## Observability Plan

- **Logs**: `prepareColdRestore` logs one line per invocation (`log.Info`
  when `Outcome == RESUME_RECOVERED`, `log.Warn` when `FRESH_LOST_HISTORY`,
  `log.Debug` otherwise) with `session`, `path`, and `outcome` fields —
  extends the existing `log.Info("cold restoring with --resume", ...)` /
  `log.Warn("cold start: ... starting fresh", ...)` lines already at
  `instance.go:882/884` rather than adding a parallel logging path.
- **Metrics**: none added. Out of scope per requirements.md's Non-goals
  ("fixing the upstream restart churn itself... out of scope here"). The
  persisted `LastReviveOutcome` field plus the notification-history table
  (`server/services/notification_service.go`) already give an operator a way
  to grep/count `FRESH_LOST_HISTORY` occurrences after the fact if a future
  item wants to quantify restart-churn frequency — no new dashboard is built
  as part of this fix.
- **Alerts**: none added. This is a self-hosted single-user deployment
  (`CLAUDE.md`'s "Architecture Overview") with no existing alerting
  infrastructure to hook into; the in-app notification (toast + badge) is
  the alert.

## Risk Control

- **Feature flag**: none added. This is a pure correctness fix restoring the
  behavior the pre-existing code already intended (`log.Info("cold restoring
  with --resume", ...)` already claims to do this — the fix makes that claim
  true). The `RecoverySuppressed` one-shot flag is itself the built-in safety
  valve that prevents the new recovery path from firing on the three
  already-tested "must stay fresh" flows (see Unresolved Question #4 for the
  optional belt-and-suspenders config toggle considered and deferred).
- **Rollback procedure**: revert the commit. All three new persisted/wire
  fields are additive with safe zero-value defaults (see Migration Plan), so
  rolling back the binary to a pre-fix version does not corrupt on-disk
  session state — the old binary simply ignores JSON keys it doesn't know
  about (Go's `encoding/json` already does this by default; no custom
  unmarshal logic exists for `session/storage.go`'s persisted struct today).
- **Staged rollout**: not applicable. `stapler-squad` is a single-binary,
  self-hosted service deployed via `make install-service`
  (`.claude/rules/systemd-user-service.md`) — there is no gradual-rollout or
  canary mechanism in this architecture to stage through.

## Unresolved Questions

- [ ] Should `EverHadConversationHistory` resetting to `false` on
      `ClearConversationState()` still allow a **later, unrelated**
      cold-restore (after a legitimate "start over" and a subsequent short
      real conversation that itself loses its UUID before capture) to
      correctly re-earn `FRESH_LOST_HISTORY` rather than staying
      `FRESH_EXPECTED` forever? — blocks Story 1.2.1 only in the sense that
      the chosen design (`EverHadConversationHistory` re-set to `true` the
      moment `SetHistoryInfo`/recovery next captures a UUID) already handles
      this correctly by construction; flagged here only because it's a
      genuinely non-obvious interaction worth a reviewer's explicit sign-off,
      not because the design is incomplete — owner: PR reviewer.
- [ ] Should the ux.md layer-3 in-session banner
      (`MemoryPressureCallout.tsx`-style, dismissible, `sessionStorage`-scoped)
      be built now or deferred as a fast-follow? — AC3's "at minimum" bar is
      met by Phase 3 (toast + badge) as scoped; ux.md frames the banner as
      "the surface most likely to actually be read" but it isn't required by
      any acceptance criterion — blocks nothing in this plan — owner:
      requirements owner / product call during implementation review.
- [ ] Should a stronger `DetectByPath` disambiguation (matching against a
      last-known `HistoryFilePath`, or a cheap content-level `cwd` check) be
      scoped as its own follow-up item, given pitfalls.md's finding that a
      same-directory collision between two never-linked sessions is *worse*
      than today's silent-fresh-start outcome (a wrong-but-plausible resumed
      conversation, not a visibly fresh one)? — blocks nothing in this plan
      (Task 4.1.1f documents the limitation with an explicit test) — owner:
      backlog triage, candidate new item.
- [ ] Is the optional `DisableColdRestoreRecovery`-style config kill-switch
      worth building, or is `RecoverySuppressed` plus the existing/new
      integration tests sufficient confidence for a fix this size? — no task
      in this plan builds it; default is **skip unless a reviewer asks for
      it** during Story 1.4.1/1.5.1's PR review — owner: implementer +
      reviewer, decided at review time.

---

## Dependency Visualization

```
Phase 1 — Recovery ordering (backend, required for AC1/AC2/AC4/AC6)
  Epic 1.1 (types/fields) ──┬─→ Epic 1.2 (wire into ClearConversationState/
                             │            SetHistoryInfo/tryExtractConversationUUID)
                             │                       │
                             └─→ Epic 1.3 (prepareColdRestore) ←┘
                                          │
                             ┌────────────┴────────────┐
                             ▼                          ▼
                  Epic 1.4 (startLocked reorder)  Epic 1.5 (start reorder — mirror)
                             │                          │
                             └────────────┬─────────────┘
                                          ▼
Phase 2 — Durable signal (backend, required for AC3)
  Epic 2.1 (proto enum + field) ──→ Epic 2.2 (LifecycleEvent reason) ──→ Epic 2.3 (notification listener)
                                          │
                                          ▼
Phase 3 — Frontend surface (ux.md)
  Epic 3.1 (RevivedContextBadge + SessionCard/SessionRow wiring, toast verify)
                                          │
                                          ▼
Phase 4 — Tests (required for AC5, covers all of Phase 1-3)
  Epic 4.1 (backend unit + integration) ──→ Epic 4.2 (frontend component test)
```

Phase 1 must land before Phase 2 (the `ReviveOutcome` value Phase 2 persists
and publishes is computed by Phase 1's `prepareColdRestore`). Phase 3 depends
on Phase 2's proto field existing. Phase 4 tasks are written per-epic
alongside their Phase 1-3 counterparts below, not deferred to the end, but
grouped here for visibility into total test coverage.

---

## Phase 1: Recovery-before-decision ordering

### Epic 1.1: New domain types and fields
**Goal**: Introduce `ReviveOutcome`, `RecoverySuppressed`, and
`EverHadConversationHistory` as first-class `Instance` state before any
control-flow change touches them.

#### Story 1.1.1: Add ReviveOutcome type and Instance fields
**As a** developer implementing the cold-restore fix, **I want** the new
domain types to exist before I wire them into control flow, **so that** the
control-flow change in Epic 1.3/1.4/1.5 is a small, reviewable diff on top of
already-compiling types.
**Acceptance Criteria**:
- `ReviveOutcome` is a `string`-backed type with 4 named constants, matching
  the Domain Glossary definition.
  - *Given* the new `session/instance_claude.go` const block, *When* code
    elsewhere references `session.ReviveOutcomeFreshLostHistory`, *Then* it
    resolves to the string value `"fresh_lost_history"`.
- `Instance` has three new fields: `recoverySuppressed bool`,
  `everHadConversationHistory bool`, `LastReviveOutcome ReviveOutcome`.
**Files**: `session/instance_claude.go`, `session/instance.go`

##### Task 1.1.1a: Add `ReviveOutcome` type + constants (~3 min)
- In `session/instance_claude.go`, near the existing `staleResumePattern`
  const (line 19), add:
  ```go
  type ReviveOutcome string

  const (
  	ReviveOutcomeUnspecified      ReviveOutcome = ""
  	ReviveOutcomeResumeLive       ReviveOutcome = "resume_live"
  	ReviveOutcomeResumeRecovered  ReviveOutcome = "resume_recovered"
  	ReviveOutcomeFreshExpected    ReviveOutcome = "fresh_expected"
  	ReviveOutcomeFreshLostHistory ReviveOutcome = "fresh_lost_history"
  )
  ```
- Files: `session/instance_claude.go`

##### Task 1.1.1b: Add new fields to the `Instance` struct (~3 min)
- In `session/instance.go`, next to the existing `HistoryFilePath string`
  field (line 230) and under the `claudeSessionMu` comment (line 301), add:
  `recoverySuppressed bool`, `everHadConversationHistory bool`, and
  `LastReviveOutcome ReviveOutcome` (exported — needs to be read by
  `server/adapters/instance_adapter.go` in Phase 2). Doc-comment each: "guarded
  by claudeSessionMu, same lock order as HistoryFilePath."
- Files: `session/instance.go`

##### Task 1.1.1c: Persist the two durable fields in storage (~3 min)
- In `session/storage.go`, next to the existing
  `HistoryFilePath string \`json:"history_file_path,omitempty"\`` field
  (line 111), add `EverHadConversationHistory bool
  \`json:"ever_had_conversation_history,omitempty"\`` and
  `LastReviveOutcome string \`json:"last_revive_outcome,omitempty"\`` (stored
  as the underlying string, not a Go type alias, matching how other enum-ish
  persisted fields in this file are stored). Wire the load/save mapping
  functions in this file (wherever `HistoryFilePath` is copied to/from the
  persisted struct) to also copy these two fields.
- Files: `session/storage.go`

##### Task 1.1.1d: Expose `LastReviveOutcome` on `InstanceSnapshot` (~3 min)
- In `session/instance_snapshot.go`, add `LastReviveOutcome ReviveOutcome` to
  the `InstanceSnapshot` struct (near `HistoryFilePath string` at line 117)
  and copy it in `buildSnapshot` (near line 198's
  `HistoryFilePath: i.HistoryFilePath,`).
- Files: `session/instance_snapshot.go`

---

### Epic 1.2: Wire the new fields into existing mutators
**Goal**: `ClearConversationState`, `SetHistoryInfo`, and
`tryExtractConversationUUID` become the single source of truth for
`RecoverySuppressed`/`EverHadConversationHistory`, so no other code ever has
to infer these signals after the fact (Pattern Decisions row 4).

#### Story 1.2.1: ClearConversationState sets RecoverySuppressed and resets EverHadConversationHistory
**As a** session that is being intentionally reset (stale-resume rejection,
program-family switch, or user "start over"), **I want** the next cold
restart to skip path-based recovery and treat me as a clean slate, **so
that** I don't loop back onto the exact UUID/JSONL I was just deliberately
disconnected from (Goal 4 / AC6).
**Acceptance Criteria**:
- Every call to `ClearConversationState()` sets `recoverySuppressed = true`
  and `everHadConversationHistory = false` under the same lock order
  (`claudeSessionMu` outer, `i.mu` inner) it already uses for
  `ConversationUUID`/`HistoryFilePath`.
  - *Given* an `Instance` with `everHadConversationHistory == true` (a prior
    conversation was captured) and `recoverySuppressed == false`, *When*
    `recoverFromStaleResume()` calls `i.ClearConversationState()`, *Then*
    immediately after that call `i.recoverySuppressed == true` and
    `i.everHadConversationHistory == false`.
**Files**: `session/instance_claude.go`

##### Task 1.2.1a: Set the two flags inside ClearConversationState (~3 min)
- In `session/instance_claude.go`'s `ClearConversationState()` (line 278),
  inside the existing `i.mu.Lock()`/`i.mu.Unlock()` section (alongside the
  `i.claudeSession.ConversationUUID = ""` / `i.HistoryFilePath = ""` writes),
  add `i.recoverySuppressed = true` and `i.everHadConversationHistory =
  false`. Update the function's doc comment to mention the new fields.
- Files: `session/instance_claude.go`

##### Task 1.2.1b: Set EverHadConversationHistory=true in SetHistoryInfo (~3 min)
- In `session/instance_claude.go`'s `SetHistoryInfo()` (line 464), inside
  the existing `i.mu.Lock()` section, add: when the new `conversationUUID`
  argument is non-empty, set `i.everHadConversationHistory = true`.
- Files: `session/instance_claude.go`

##### Task 1.2.1c: Set EverHadConversationHistory=true in tryExtractConversationUUID's direct-mutation path (~3 min)
- In `session/instance_claude.go`'s `tryExtractConversationUUID()` (line
  308), at the existing direct-mutation block (lines 356-362, "Set the
  fields directly (caller holds stateMutex)"), add
  `i.everHadConversationHistory = true` alongside the existing
  `i.claudeSession.ConversationUUID = info.ConversationUUID` write. This
  function does not take `claudeSessionMu` today (documented pre-existing
  inconsistency, architecture.md) — do not introduce a new lock here; match
  the existing unlocked-write style exactly, per architecture.md's explicit
  instruction not to add a third, differently-ordered locking path.
- Files: `session/instance_claude.go`

---

### Epic 1.3: The `prepareColdRestore` shared helper
**Goal**: One function, callable from both `startLocked` and `start`, that
performs the entire recovery-then-decide computation this bug is about.

#### Story 1.3.1: Implement prepareColdRestore and coldRestoreOutcome
**As a** developer wiring this helper into `startLocked`/`start`, **I want**
one function that returns "should I resume, and what's the durable outcome
signal," **so that** both call sites share identical logic (AC4) and neither
call site has to re-derive the `HasClaudeSession()`/`RecoverySuppressed`/
`DetectByPath`/`EverHadConversationHistory` interaction itself.
**Acceptance Criteria**:
- `prepareColdRestore()` never calls `DetectByPath` when
  `HasClaudeSession()` is already true (Pattern Decisions row 5 / pitfalls.md
  design-against-checklist #1 — do not "double-check" an existing UUID).
  - *Given* an `Instance` with `i.claudeSession.ConversationUUID ==
    "550e8400-e29b-41d4-a716-446655440000"`, *When* `prepareColdRestore()`
    runs, *Then* it returns `coldRestoreOutcome{Resume: true, Outcome:
    ReviveOutcomeResumeLive}` without touching the filesystem.
- `prepareColdRestore()` honors and consumes `RecoverySuppressed` before
  attempting any recovery.
  - *Given* an `Instance` with `i.claudeSession == nil` and
    `i.recoverySuppressed == true` (just set by `ClearConversationState()`),
    *When* `prepareColdRestore()` runs, *Then* it returns
    `coldRestoreOutcome{Resume: false, Outcome: ReviveOutcomeFreshExpected}`,
    performs no `DetectByPath` scan, and leaves `i.recoverySuppressed ==
    false` afterward (consumed).
**Files**: `session/instance_claude.go`

##### Task 1.3.1a: Add the coldRestoreOutcome struct and function signature (~3 min)
- In `session/instance_claude.go`, add:
  ```go
  // coldRestoreOutcome is prepareColdRestore's result: whether the caller
  // should expect --resume to be embedded in the launch command it is about
  // to build, and the durable ReviveOutcome signal for this start cycle.
  type coldRestoreOutcome struct {
  	Resume  bool
  	Outcome ReviveOutcome
  }

  // prepareColdRestore attempts on-disk UUID recovery — path-fallback only,
  // since the caller has already confirmed the tmux pane is dead — BEFORE
  // initTmuxSession() reads i.claudeSession.ConversationUUID to build the
  // launch command (session-revive-uuid-loss Goal 1). Must be called from
  // inside startLocked/start's actor-serialized cold-restore branch, after
  // i.pm().IsAlive() is confirmed false, and its result must be consumed by
  // calling initTmuxSession() before any other code reads
  // i.claudeSession.ConversationUUID for this start cycle.
  func (i *Instance) prepareColdRestore() coldRestoreOutcome {
  	// body added in Task 1.3.1b
  }
  ```
- Files: `session/instance_claude.go`

##### Task 1.3.1b: Implement the decision body (~5 min)
- Body logic, in order:
  1. `if i.HasClaudeSession() { return coldRestoreOutcome{Resume: true,
     Outcome: ReviveOutcomeResumeLive} }`
  2. `if i.recoverySuppressed { i.recoverySuppressed = false; return
     coldRestoreOutcome{Resume: false, Outcome: ReviveOutcomeFreshExpected}
     }`
  3. Call `i.tryExtractConversationUUID()` (already does exactly the
     path-fallback scan needed — the live-PID fast path inside it is a
     guaranteed no-op here since the caller already confirmed
     `!i.pm().IsAlive()`; Task 1.2.1c already made it set
     `everHadConversationHistory = true` on success).
  4. `if i.HasClaudeSession() { return coldRestoreOutcome{Resume: true,
     Outcome: ReviveOutcomeResumeRecovered} }`
  5. `if i.everHadConversationHistory { return coldRestoreOutcome{Resume:
     false, Outcome: ReviveOutcomeFreshLostHistory} }`
  6. `return coldRestoreOutcome{Resume: false, Outcome:
     ReviveOutcomeFreshExpected}`
- Files: `session/instance_claude.go`

---

### Epic 1.4: Restructure `startLocked`'s call ordering
**Goal**: `initTmuxSession()` reads a `ConversationUUID` that recovery has
already had a chance to populate, for the `startLocked` (production) path.

#### Story 1.4.1: Move initTmuxSession() to run after prepareColdRestore in the cold-restore branch
**As a** session whose tmux pane died and whose in-memory UUID was never
captured, **I want** `startLocked` to attempt recovery before it builds the
`claude` launch command, **so that** a resumable JSONL that exists on disk
is actually used instead of silently discarded (AC1).
**Acceptance Criteria**:
- `i.initTmuxSession()` is no longer called unconditionally at the top of
  `startLocked`; it is called once per branch, after that branch's decision
  is fully known.
  - *Given* a `ColdRestore` for a session whose effective root dir is
    `/home/user/.stapler-squad/worktrees/proj-42` and a real JSONL exists at
    `~/.claude/projects/-home-user--stapler-squad-worktrees-proj-42/550e8400-e29b-41d4-a716-446655440000.jsonl`,
    with `i.claudeSession == nil`, *When* `startLocked` runs, *Then*
    `i.prepareColdRestore()` populates
    `i.claudeSession.ConversationUUID == "550e8400-e29b-41d4-a716-446655440000"`
    **before** `i.initTmuxSession()` is called, so
    `i.LaunchCommand` contains the substring `--resume
    550e8400-e29b-41d4-a716-446655440000`.
**Files**: `session/instance.go`

##### Task 1.4.1a: Remove the unconditional initTmuxSession() call at the top (~2 min)
- Delete `i.initTmuxSession()` at `session/instance.go:858` (immediately
  after the `i.Title == ""` check, before `i.pm().ResetExitOnce()`). Leave
  `i.pm().ResetExitOnce()` / `i.pm().SetOnExitCallback(...)` (lines 860-861)
  in place — they operate on exit-tracking state independent of the
  `tmux.TmuxSession` object `initTmuxSession()` builds.
- Files: `session/instance.go`

##### Task 1.4.1b: Call prepareColdRestore + initTmuxSession in the cold-restore branch (~5 min)
- In the `if !i.pm().IsAlive() {` block (line 879), immediately after
  entering it and before the existing `startPath :=
  i.resolveStartPath(...)` line, add:
  ```go
  outcome := i.prepareColdRestore()
  i.initTmuxSession()
  i.LastReviveOutcome = outcome.Outcome
  ```
  Then replace the existing `if i.HasClaudeSession() { ... } else { ... }`
  log branch (lines 881-885) with `if outcome.Resume { ... } else { ... }` —
  same log messages, just driven by the already-computed `outcome.Resume`
  instead of re-calling `HasClaudeSession()` (which would now
  trivially match `outcome.Resume` but re-deriving it is redundant and
  slightly misleading — `outcome` is the actual decision that shaped
  `initTmuxSession()`'s output).
- Files: `session/instance.go`

##### Task 1.4.1c: Call initTmuxSession() in the hot-restore and first-time-setup branches (~3 min)
- Add `i.initTmuxSession()` as the first line of the `else` branch (hot
  restore, line 922) and as the first line of the outer `else` branch
  (first-time setup, line 936) — both are safe no-ops today only because
  `initTmuxSession()`'s early-return (`if i.pm().HasSession() { return }`)
  wasn't previously relied upon for these paths; calling it explicitly here
  makes each branch self-contained instead of depending on a call that used
  to live unconditionally at the top. Verify via `go build ./session/...`
  that this compiles and no other code path assumed `i.LaunchCommand` was
  already set before reaching these branches.
- Files: `session/instance.go`

##### Task 1.4.1d: Update the cold-restore comment block to describe the new order (~2 min)
- Update the existing comment above `i.tryExtractConversationUUID()` at
  line 914-920 ("Re-detect immediately from the freshly-started process's
  open files...") — this call still runs, unchanged, **after** `Start()`
  (it re-derives from the live process's open files now that it's running,
  which can differ from the pre-launch `DetectByPath` guess if `--resume`
  itself created a new conversation). Add one sentence noting that
  `prepareColdRestore()` (new, above) already attempted path-based recovery
  before the launch command was built — this second call is not redundant
  with it, it's the live-process confirmation pass.
- Files: `session/instance.go`

---

### Epic 1.5: Mirror the restructuring into `start()`
**Goal**: Identical fix in `Instance.start` (reachable via
`StartWithCleanup`), satisfying AC4's symmetry requirement even though this
path has no production caller today (research/features.md — every non-test
caller is in `*_test.go`).

#### Story 1.5.1: Apply the identical restructuring to Instance.start
**As a** maintainer relying on AC4 ("no duplicated divergent logic between
the two call sites"), **I want** `start()`'s cold-restore branch to call the
exact same `prepareColdRestore()` helper in the exact same position relative
to `initTmuxSession()`, **so that** the two paths cannot drift apart the way
they already had before this fix (stack.md's observed comment drift).
**Acceptance Criteria**:
- `start()`'s cold-restore branch produces byte-identical `coldRestoreOutcome`
  behavior to `startLocked`'s for the same fixture state.
  - *Given* two otherwise-identical instances — one driven through
    `Instance.Start(false)` → `startLocked`, one through
    `Instance.StartWithCleanup(false)` → `start` — both with `i.claudeSession
    == nil` and the same resumable JSONL on disk, *When* each is invoked,
    *Then* both call `i.prepareColdRestore()` (verified via `grep -c "func
    (i \*Instance) prepareColdRestore" session/instance_claude.go` equals
    `1`, i.e. one shared implementation, not two), and both end with
    `i.LastReviveOutcome == ReviveOutcomeResumeRecovered` and `--resume
    <uuid>` present in `i.LaunchCommand`.
**Files**: `session/instance.go`

##### Task 1.5.1a: Remove the unconditional initTmuxSession() call in start() (~2 min)
- Delete `i.initTmuxSession()` at `session/instance.go:1040` (mirror of Task
  1.4.1a). Leave `i.pm().ResetExitOnce()` / `SetOnExitCallback(...)` (lines
  1046-1047) in place.
- Files: `session/instance.go`

##### Task 1.5.1b: Call prepareColdRestore + initTmuxSession in start()'s cold-restore branch (~5 min)
- Mirror of Task 1.4.1b inside the `if !i.pm().IsAlive() {` block at line
  1069: call `outcome := i.prepareColdRestore()`, then `i.initTmuxSession()`,
  then `i.LastReviveOutcome = outcome.Outcome`, then replace the
  `if i.HasClaudeSession() { ... } else { ... }` log branch (lines
  1072-1080) with `if outcome.Resume { ... } else { ... }`.
- Files: `session/instance.go`

##### Task 1.5.1c: Call initTmuxSession() in start()'s hot-restore and first-time-setup branches (~3 min)
- Mirror of Task 1.4.1c: add `i.initTmuxSession()` at the top of the `else`
  (hot restore, line ~1128) and the outer first-time-setup branch in
  `start()`.
- Files: `session/instance.go`

##### Task 1.5.1d: Update start()'s cold-restore comment block (~2 min)
- Mirror of Task 1.4.1d for the comment block at lines 1122-1126.
- Files: `session/instance.go`

---

## Phase 2: Durable revive-outcome signal (Goal 3, AC3)

### Epic 2.1: Proto enum and wire field
**Goal**: `ReviveOutcome` is visible to the frontend over the existing
`Session` proto message, alongside `history_file_path`/
`claude_conversation_uuid`.

#### Story 2.1.1: Add ReviveOutcome enum and revive_outcome field to proto
**As a** frontend developer, **I want** `session.reviveOutcome` available on
the `Session` message the same way `session.historyFilePath` already is,
**so that** `RevivedContextBadge` (Phase 3) can read it with no extra RPC.
**Acceptance Criteria**:
- `proto/session/v1/types.proto` declares a top-level `enum ReviveOutcome`
  (5 values, `_UNSPECIFIED = 0` first, matching `DetectedStatus`'s existing
  style) and a `ReviveOutcome revive_outcome = 72;` field on `Session`
  (next available field number, confirmed via
  `awk '/^message Session {/,/^}/' proto/session/v1/types.proto | grep -oE
  "= [0-9]+;" | sort -n | tail -1` → `71`, so `72` is free).
  - *Given* the regenerated Go/TS bindings, *When* a Go caller sets
    `protoSession.ReviveOutcome =
    sessionv1.ReviveOutcome_REVIVE_OUTCOME_FRESH_LOST_HISTORY`, *Then* a TS
    client reading the same `Session` sees
    `session.reviveOutcome === ReviveOutcome.FRESH_LOST_HISTORY`.
**Files**: `proto/session/v1/types.proto`

##### Task 2.1.1a: Add the enum and field to types.proto (~4 min)
- Add, near `enum DetectedStatus` (line 380):
  ```protobuf
  enum ReviveOutcome {
    REVIVE_OUTCOME_UNSPECIFIED = 0;
    REVIVE_OUTCOME_RESUME_LIVE = 1;
    REVIVE_OUTCOME_RESUME_RECOVERED = 2;
    REVIVE_OUTCOME_FRESH_EXPECTED = 3;
    REVIVE_OUTCOME_FRESH_LOST_HISTORY = 4;
  }
  ```
  Add inside `message Session { ... }`, after `string workspace_key = 71;`:
  `ReviveOutcome revive_outcome = 72;`
- Files: `proto/session/v1/types.proto`

##### Task 2.1.1b: Regenerate bindings (~2 min)
- Run `make proto-gen`. Verify `session/gen/session/v1/*.go` and
  `web-app/src/gen/session/v1/*_pb.ts` picked up the new enum/field (`grep
  -c ReviveOutcome session/gen/session/v1/types.pb.go` > 0).
- Files: `session/gen/session/v1/*.go` (generated), `web-app/src/gen/session/v1/*_pb.ts` (generated)

##### Task 2.1.1c: Map Go ReviveOutcome to proto enum in the adapter (~4 min)
- In `server/adapters/instance_adapter.go`, near the existing
  `protoSession.HistoryFilePath = snap.HistoryFilePath` line (117), add a
  small `reviveOutcomeToProto(o session.ReviveOutcome)
  sessionv1.ReviveOutcome` switch function (same shape as the existing
  `statusToProto`/`sessionTypeToProto` functions in this file) and set
  `protoSession.ReviveOutcome =
  reviveOutcomeToProto(snap.LastReviveOutcome)`.
- Files: `server/adapters/instance_adapter.go`

---

### Epic 2.2: Lifecycle event reason
**Goal**: `EventStarted` carries a defined reason string when this cycle's
outcome was `FRESH_LOST_HISTORY`, following the existing reason-string
precedent.

#### Story 2.2.1: Fire EventStarted with ReasonColdRestoreLostHistory
**As a** listener subscribed to `Instance` lifecycle events, **I want** to
distinguish "started normally" from "forced fresh after a failed recovery"
without inspecting session state myself, **so that** the notification
listener in Epic 2.3 can be a thin, reason-string-driven filter (matching
`review_queue_poller.go`'s existing pattern).
**Acceptance Criteria**:
- `i.fireLifecycleEvent(EventStarted, reason)` is called with
  `ReasonColdRestoreLostHistory` exactly when `outcome.Outcome ==
  ReviveOutcomeFreshLostHistory` for this start cycle, and with `""` (as
  today) otherwise.
  - *Given* a `ColdRestore` where `prepareColdRestore()` returned
    `coldRestoreOutcome{Resume: false, Outcome:
    ReviveOutcomeFreshLostHistory}`, *When* `startLocked` reaches its
    existing `i.fireLifecycleEvent(EventStarted, "")` call (line 999),
    *Then* the reason argument is `"cold-restore-fresh-lost-history"`
    instead of `""`.
**Files**: `session/instance_controller.go`, `session/instance.go`

##### Task 2.2.1a: Add the ReasonColdRestoreLostHistory constant (~2 min)
- In `session/instance_controller.go`, near the `LifecycleEvent` consts
  (line 73-88), add:
  ```go
  // ReasonColdRestoreLostHistory is passed to fireLifecycleEvent(EventStarted, ...)
  // when a cold restore was forced fresh after prepareColdRestore's recovery
  // attempt found nothing, despite the session having captured a real
  // conversation UUID at some point before this cycle (session-revive-uuid-loss
  // Goal 3 / AC3).
  const ReasonColdRestoreLostHistory = "cold-restore-fresh-lost-history"
  ```
- Files: `session/instance_controller.go`

##### Task 2.2.1b: Pass the reason at both fireLifecycleEvent(EventStarted, ...) call sites (~3 min)
- In `session/instance.go`, change `i.fireLifecycleEvent(EventStarted, "")`
  at line 999 (`startLocked`) and line 1225 (`start`) to compute a `reason`
  local (`""` unless `i.LastReviveOutcome ==
  ReviveOutcomeFreshLostHistory`, in which case
  `ReasonColdRestoreLostHistory`) and pass it instead of the literal `""`.
- Files: `session/instance.go`

---

### Epic 2.3: Notification listener
**Goal**: The reason string from Epic 2.2 reaches a durable, user-visible
notification, mirroring `onRateLimitRecovery`'s exact shape.

#### Story 2.3.1: onColdRestoreLostHistory SessionService method and wiring
**As a** user whose session lost its previous conversation on revive, **I
want** a notification (not just a log line) telling me this happened, **so
that** I don't act on a false assumption that the agent remembers our prior
conversation (Goal 3 / AC3; ux.md's "tell me my mental model of this session
is wrong" job-to-be-done).
**Acceptance Criteria**:
- `SessionService.onColdRestoreLostHistory` publishes exactly one
  `events.NewNotificationEvent` per firing, type WARNING (8), priority
  MEDIUM (2), gated on `!inst.Hidden` — same gating as `onRateLimitRecovery`.
  - *Given* a non-`Hidden` `Instance` titled `"my-worktree-session"` whose
    `EventStarted` fires with `reason ==
    "cold-restore-fresh-lost-history"`, *When*
    `coldRestoreOutcomeListener.OnLifecycleEvent` receives it, *Then*
    `s.eventBus.Publish(events.NewNotificationEvent(...))` is called with
    `notificationType = 8` (WARNING), `priority = 2` (MEDIUM), and a title
    containing `"my-worktree-session"`.
  - *Given* the same event but `inst.Hidden == true` (e.g. a headless
    backlog review session), *When* the listener receives it, *Then* no
    notification is published (matches `onRateLimitRecovery`'s existing
    `Hidden` gate).
**Files**: `server/services/session_service.go`

##### Task 2.3.1a: Add coldRestoreOutcomeListener type (~3 min)
- In `server/services/session_service.go`, near `autoArchiveListener` (line
  3865-3875), add:
  ```go
  type coldRestoreOutcomeListener struct {
  	svc  *SessionService
  	inst *session.Instance
  }

  func (l *coldRestoreOutcomeListener) OnLifecycleEvent(event session.LifecycleEvent, reason string) {
  	if event == session.EventStarted && reason == session.ReasonColdRestoreLostHistory {
  		l.svc.onColdRestoreLostHistory(l.inst)
  	}
  }
  ```
- Files: `server/services/session_service.go`

##### Task 2.3.1b: Add onColdRestoreLostHistory method (~4 min)
- Add, mirroring `onRateLimitRecovery` (line 4001-4029):
  ```go
  func (s *SessionService) onColdRestoreLostHistory(inst *session.Instance) {
  	if !inst.Hidden {
  		linkedItemID := s.rateLimitLinkedItemID(inst) // reuse: same best-effort ItemSession lookup
  		notifID := fmt.Sprintf("cold-restore-lost-history-%s", inst.UUID)
  		s.eventBus.Publish(events.NewNotificationEvent(
  			inst.UUID, inst.Title, notifID,
  			int32(8), // NotificationType_WARNING
  			int32(2), // NotificationPriority_MEDIUM
  			fmt.Sprintf("Session %q started fresh — previous conversation could not be resumed", inst.Title),
  			"The session's tmux pane restarted and the previous conversation history could not be found on disk. Earlier context is not available.",
  			events.SessionScopedMetadata(nil, linkedItemID),
  		))
  	}
  }
  ```
  Note: reusing `s.rateLimitLinkedItemID` (line 3955) as-is since its
  behavior (best-effort, bounded, `""` on any failure) is exactly what's
  needed here too — do not duplicate it.
- Files: `server/services/session_service.go`

##### Task 2.3.1c: Register the listener in wireCallbacks (~2 min)
- In `wireCallbacks` (line 985), add
  `inst.RegisterLifecycleListener(&coldRestoreOutcomeListener{svc: s, inst:
  inst})` alongside the other `wire*`/`RegisterLifecycleListener` calls.
- Files: `server/services/session_service.go`

---

## Phase 3: Frontend surface

### Epic 3.1: RevivedContextBadge
**Goal**: A durable, accessible badge on the session list/card that survives
the toast auto-closing (ux.md's core finding: a toast alone is insufficient
because restarts typically happen while the user is away).

#### Story 3.1.1: RevivedContextBadge component
**As a** user browsing my session list, **I want** to see, at a glance and
without opening the session, that a session's context was lost on its last
restart, **so that** I know not to trust "continue where we left off"
before I've even opened it (ux.md job-to-be-done).
**Acceptance Criteria**:
- `RevivedContextBadge` renders only when `session.reviveOutcome ===
  ReviveOutcome.FRESH_LOST_HISTORY`; renders nothing (`null`) for the other
  3 enum values.
  - *Given* a `Session` proto value with `reviveOutcome ===
    ReviveOutcome.FRESH_LOST_HISTORY`, *When* `<RevivedContextBadge
    session={session} />` renders, *Then* the DOM contains an element with
    `role="status"`, `aria-label="This session lost its previous
    conversation and started fresh"`, and a decoration icon marked
    `aria-hidden="true"` — following `ConnectionIndicator.tsx`'s exact
    accessibility shape (`role="status"`, `aria-live="polite"`, hidden icon,
    full-sentence label).
  - *Given* a `Session` with `reviveOutcome ===
    ReviveOutcome.RESUME_RECOVERED` (or any other non-`FRESH_LOST_HISTORY`
    value), *When* `<RevivedContextBadge session={session} />` renders,
    *Then* it returns `null` (no DOM output).
**Files**: `web-app/src/components/sessions/RevivedContextBadge.tsx`, `web-app/src/components/sessions/RevivedContextBadge.css.ts`

##### Task 3.1.1a: Create RevivedContextBadge.css.ts (~3 min)
- New vanilla-extract style file per `.claude/rules/css-architecture.md`,
  colocated with the component. Import tokens from `theme.css.ts` (e.g.
  `vars.color.statusWarning`/equivalent existing warning token — check
  `web-app/src/app/globals.css` for the closest existing `--warning*`
  token per `.claude/rules/css-architecture.md`'s CSS Modules section, or
  the vanilla-extract theme contract if this file already uses one).
  Minimal: a `wrapper` style (inline-flex, small gap) and an `icon` style
  (matching `ConnectionIndicator.css.ts`'s `dots`/`label` shape).
- Files: `web-app/src/components/sessions/RevivedContextBadge.css.ts`

##### Task 3.1.1b: Create RevivedContextBadge.tsx (~4 min)
- New component, structurally identical to `ConnectionIndicator.tsx`:
  ```tsx
  "use client";
  import { ReviveOutcome, Session } from "@/gen/session/v1/types_pb";
  import { icon, wrapper } from "./RevivedContextBadge.css";

  interface RevivedContextBadgeProps { session: Session; }

  export function RevivedContextBadge({ session }: RevivedContextBadgeProps) {
    if (session.reviveOutcome !== ReviveOutcome.FRESH_LOST_HISTORY) return null;
    return (
      <span
        className={wrapper}
        role="status"
        aria-live="polite"
        aria-label="This session lost its previous conversation and started fresh"
        data-testid="revived-context-badge"
      >
        <span className={icon} aria-hidden="true">⚠</span>
        Context lost
      </span>
    );
  }
  ```
- Files: `web-app/src/components/sessions/RevivedContextBadge.tsx`

##### Task 3.1.1c: Wire into SessionCard.tsx (~3 min)
- In `web-app/src/components/sessions/SessionCard.tsx`, next to the
  existing `<StatusBadge detectedStatus={detectedStatus} .../>` call (line
  538), add `<RevivedContextBadge session={session} />`. Import the new
  component alongside the existing `StatusBadge` import (line 7).
- Files: `web-app/src/components/sessions/SessionCard.tsx`

##### Task 3.1.1d: Wire into SessionRow.tsx (~3 min)
- Same wiring as Task 3.1.1c, in `web-app/src/components/sessions/SessionRow.tsx`,
  next to its existing status-badge rendering, and extend the row's own
  `aria-label` string (line 210) to append `", context: lost"` when
  `session.reviveOutcome === ReviveOutcome.FRESH_LOST_HISTORY` — matching
  ux.md's explicit accessibility recommendation to extend the existing
  card-level `aria-label` rather than adding a second, separately-announced
  landmark.
- Files: `web-app/src/components/sessions/SessionRow.tsx`

##### Task 3.1.2a: Verify the existing toast pipeline needs no changes (~2 min)
- `NotificationToast.tsx` already renders any `NewNotificationEvent`
  generically by type/priority (confirmed: `onRateLimitRecovery` already
  uses WARNING/MEDIUM and INFO/MEDIUM with no per-notification-type
  component code). Run the existing frontend test suite filtered to
  `NotificationToast` (`cd web-app && npx jest --no-coverage
  --testPathPatterns="NotificationToast"`) to confirm nothing needs to
  change here — this task is a verification, not a code change.
- Files: none (verification only)

---

## Phase 4: Tests

### Epic 4.1: Backend tests
**Goal**: Cover the recovery-before-decision ordering itself (not just the
log line), the intentional-clear guard, the durable signal, and the known
same-directory limitation — per AC5 and pitfalls.md's flakiness guidance.

#### Story 4.1.1: prepareColdRestore unit tests
**As a** reviewer of this fix, **I want** unit tests at the smallest testable
unit (`prepareColdRestore` itself, not a full `startLocked` run), **so that**
the ordering logic is verified without the flakiness/heaviness of real tmux
sessions (pitfalls.md §e).
**Acceptance Criteria**:
- All 6 branches of `prepareColdRestore` (HasClaudeSession-true,
  RecoverySuppressed, recovery-succeeds, recovery-fails+EverHadHistory,
  recovery-fails+never-had-history, same-directory collision) are covered
  by a distinct test.
  - *Given* an `Instance` with `historyDetector` pointed at a temp
    `$HOME/.claude/projects/<encoded-path>/` directory containing a single
    valid JSONL `550e8400-e29b-41d4-a716-446655440000.jsonl`, and
    `i.claudeSession == nil`, *When* `i.prepareColdRestore()` is called,
    *Then* it returns `coldRestoreOutcome{Resume: true, Outcome:
    ReviveOutcomeResumeRecovered}` and `i.everHadConversationHistory ==
    true`.
**Files**: `session/instance_claude_test.go` or a new `session/instance_cold_restore_decision_test.go`

##### Task 4.1.1a: TestPrepareColdRestore_RecoversUUID_When_JSONLExistsButUUIDNeverCaptured (~5 min)
- Follow `TestTryExtractConversationUUID_PathFallbackRepopulatesAfterClear`'s
  exact fixture pattern (`instance_workspace_test.go:151-177`): temp home
  dir, `ClaudeProjectDirName`-encoded project dir, a written `.jsonl` file.
  Assert `coldRestoreOutcome{Resume: true, Outcome:
  ReviveOutcomeResumeRecovered}` and `inst.everHadConversationHistory ==
  true` (AC1).
- Files: `session/instance_cold_restore_decision_test.go` (new)

##### Task 4.1.1b: TestPrepareColdRestore_StartsFresh_When_NoJSONLExists (~4 min)
- Same fixture shape but no JSONL directory created at all. Assert
  `coldRestoreOutcome{Resume: false, Outcome: ReviveOutcomeFreshExpected}`
  and `inst.everHadConversationHistory == false` (AC2).
- Files: `session/instance_cold_restore_decision_test.go`

##### Task 4.1.1c: TestPrepareColdRestore_SkipsRecovery_When_RecoverySuppressed (~4 min)
- `inst.recoverySuppressed = true`, JSONL present on disk (so a naive
  implementation *would* recover it). Assert the result is
  `ReviveOutcomeFreshExpected` (not `ResumeRecovered`), `DetectByPath` is
  never reached (verify via a `historyDetector` test double that fails the
  test if `DetectByPath` is called — reuse `mockProcessInspector`-style
  pattern from `history_detector_test.go`), and
  `inst.recoverySuppressed == false` after the call (consumed).
- Files: `session/instance_cold_restore_decision_test.go`

##### Task 4.1.1d: TestPrepareColdRestore_SignalsFreshLostHistory_When_RecoveryFailsButEverHadHistory (~4 min)
- `inst.everHadConversationHistory = true`, no JSONL on disk (simulates: had
  a real conversation before, the file/dir is now gone or the encoded path
  changed). Assert `coldRestoreOutcome{Resume: false, Outcome:
  ReviveOutcomeFreshLostHistory}` (AC3).
- Files: `session/instance_cold_restore_decision_test.go`

##### Task 4.1.1e: TestPrepareColdRestore_NoSignal_When_GenuinelyNeverHadHistory (~3 min)
- `inst.everHadConversationHistory = false` (default), no JSONL. Assert
  `ReviveOutcomeFreshExpected`, not `FreshLostHistory` — the AC2/AC6
  negative case distinguishing "never had history" from "had it, lost it."
- Files: `session/instance_cold_restore_decision_test.go`

##### Task 4.1.1f: TestPrepareColdRestore_SameDirectoryCollision_DocumentsKnownLimitation (~5 min)
- Two `Instance`s pointed at the same encoded project dir via
  `historyDetector`, both `claudeSession == nil`. Write one JSONL file, pin
  its mtime with `os.Chtimes` (per pitfalls.md §e / `history_linker_test.go`
  pattern — do not rely on write-order-implies-mtime-order). Call
  `prepareColdRestore()` on both instances; assert **both** recover the
  **same** UUID (documents the accepted limitation from Pattern Decisions'
  last row — this test exists to make the limitation visible and regression
  proof, not to fix it, per pitfalls.md design-against-checklist #5: "at
  minimum this risk should be named as accepted/out-of-scope... rather than
  silently untested").
- Files: `session/instance_cold_restore_decision_test.go`

#### Story 4.1.2: Integration-level ordering test proving the launch command is actually fixed
**As a** reviewer skeptical that this fix does more than change a log line
(pitfalls.md's central warning), **I want** a real-tmux test asserting
`--resume <uuid>` is actually present in the launched command, **so that**
the fix is proven against the actual bug, not just against
`prepareColdRestore`'s isolated return value.
**Acceptance Criteria**:
- A new test in `instance_cold_restore_test.go`, siblings to
  `TestColdRestore_WithUUID`/`TestColdRestore_WithoutUUID`, drives a real
  cold restore with no in-memory UUID but a real on-disk JSONL, and inspects
  `inst.LaunchCommand` (not just `inst.Status`) for the `--resume` flag.
  - *Given* `inst.claudeSession == nil` and a real JSONL file written to
    `inst.historyDetector`'s configured home dir under `inst.Path`'s encoded
    project directory, *When* `inst.StartWithCleanup(false)` runs (dead tmux
    pane, simulating post-reboot state), *Then*
    `strings.Contains(inst.LaunchCommand, "--resume")` is `true` — this
    specific assertion is what would have failed against the pre-fix code
    (pre-fix, `tryExtractConversationUUID` ran after `initTmuxSession()` had
    already built the command without `--resume`).
**Files**: `session/instance_cold_restore_test.go`

##### Task 4.1.2a: TestColdRestore_RecoversUUIDBeforeLaunch_When_UUIDNeverCapturedButJSONLExists (~5 min)
- Add to `session/instance_cold_restore_test.go`, following
  `TestColdRestore_WithUUID`'s exact tmux-fixture setup (lines 44-97), but:
  set `inst.historyDetector` to a real `HistoryFileDetector` pointed at a
  temp home dir (`NewHistoryFileDetectorWithHomeDir`) containing a written
  JSONL under `ClaudeProjectDirName(inst.Path)`, and do **not** call
  `inst.SetClaudeSession(...)`. After `inst.StartWithCleanup(false)`,
  assert `strings.Contains(inst.LaunchCommand, "--resume")`.
- Files: `session/instance_cold_restore_test.go`

##### Task 4.1.2b: TestColdRestore_Mirror_start_RecoversUUIDBeforeLaunch (~3 min)
- Same as Task 4.1.2a — already exercises `start()` since
  `StartWithCleanup` calls `i.start(...)` directly (line 1015), not
  `startLocked`. Confirms AC4's symmetry for the actual entry point this
  test file already uses. If time-boxing requires only one integration test,
  this task documents that 4.1.2a already covers the `start()` mirror and
  can be a no-op verification rather than a new test — note this explicitly
  in the PR description rather than silently skipping AC4 coverage.
- Files: `session/instance_cold_restore_test.go`

#### Story 4.1.3: Regression guards for the intentional-clear flows
**As a** reviewer verifying Goal 4/AC6, **I want** an explicit test proving
`recoverFromStaleResume` does not loop back onto the same stale UUID via the
new recovery path, **so that** this fix cannot silently reintroduce the bug
`recoverFromStaleResume` already fixed.
**Acceptance Criteria**:
- After `recoverFromStaleResume()` runs (which calls
  `ClearConversationState()` then `Start(false)`), the resulting
  `ColdRestore` does not resume with the same UUID that was just rejected as
  stale, even though its JSONL file is still the newest file in the
  directory.
  - *Given* an `Instance` whose `claudeSession.ConversationUUID` was
    `"aaaaaaaa-...-stale"` and whose on-disk JSONL for that UUID is still
    present (untouched — `recoverFromStaleResume` doesn't delete files),
    *When* `recoverFromStaleResume()` runs and the subsequent cold restart's
    `prepareColdRestore()` executes, *Then* the outcome is
    `ReviveOutcomeFreshExpected` (recovery was suppressed for this one
    cycle), not `ReviveOutcomeResumeRecovered` with the stale UUID.
**Files**: `session/instance_claude_test.go`

##### Task 4.1.3a: TestRecoverFromStaleResume_DoesNotRediscoverSameStaleUUID_OnNextColdRestore (~5 min)
- Write the JSONL for the stale UUID to disk (simulating that
  `recoverFromStaleResume` doesn't delete it), call
  `inst.recoverFromStaleResume()` inline (or replicate its two calls:
  `ClearConversationState()` then a fixture-friendly equivalent of the
  cold-restore path), and assert `prepareColdRestore()`'s result is
  `ReviveOutcomeFreshExpected`, not a resume of the stale UUID.
- Files: `session/instance_claude_test.go`

##### Task 4.1.3b: Verify TestTryExtractConversationUUID_PathFallbackRepopulatesAfterClear still passes unmodified (~2 min)
- Run `go test ./session/... -run
  TestTryExtractConversationUUID_PathFallbackRepopulatesAfterClear -v` after
  all Phase 1 changes land. This test must pass with zero modifications
  (AC5) — it exercises `tryExtractConversationUUID()` directly, which
  Task 1.2.1c only adds one line to (`everHadConversationHistory = true`)
  without changing its externally-observable UUID/HistoryFilePath behavior.
- Files: none (verification only)

### Epic 4.2: Frontend tests

#### Story 4.2.1: RevivedContextBadge tests
**As a** reviewer of the frontend change, **I want** a Jest/RTL test
confirming the badge only renders for `FRESH_LOST_HISTORY` and carries the
correct accessible name, **so that** the accessibility contract from ux.md
(hidden icon, full-sentence `aria-label`, `role="status"`) is enforced by CI,
not just code review.
**Acceptance Criteria**:
- Test file covers both the positive-render and null-render cases from
  Story 3.1.1's acceptance criteria.
  - *Given* a mock `Session` object with `reviveOutcome:
    ReviveOutcome.FRESH_LOST_HISTORY`, *When* `render(<RevivedContextBadge
    session={mockSession} />)` runs, *Then*
    `screen.getByTestId("revived-context-badge")` exists and has
    `aria-label="This session lost its previous conversation and started
    fresh"`.
**Files**: `web-app/src/components/sessions/__tests__/RevivedContextBadge.test.tsx`

##### Task 4.2.1a: RevivedContextBadge.test.tsx — positive and null render (~4 min)
- New test file, following `StatusBadge.test.tsx`'s existing structure
  (`web-app/src/components/sessions/__tests__/StatusBadge.test.tsx`). Two
  cases: `reviveOutcome === FRESH_LOST_HISTORY` renders the badge with
  correct `aria-label`/`role`/hidden icon; each of the other 3 enum values
  renders `null`.
- Files: `web-app/src/components/sessions/__tests__/RevivedContextBadge.test.tsx`

##### Task 4.2.1b: Run the full frontend suite for this directory (~2 min)
- `cd web-app && npx jest --no-coverage
  --testPathPatterns="RevivedContextBadge|SessionCard|SessionRow"` — confirm
  no regressions in the two files Task 3.1.1c/3.1.1d modified.
- Files: none (verification only)
