# Architecture Research: Core Domain Decomposition

Research/verification only, performed by direct code inspection (no application files modified).

## 1. `server/services/` already follows an established decomposition pattern

Confirmed by reading `project_service.go` and `github_service.go` in full:

```go
// project_service.go
type ProjectService struct {
    storage *session.Storage
}
func NewProjectService(storage *session.Storage) *ProjectService { ... }

// session_service.go's constructor (around line 309):
projectSvc: NewProjectService(concStorage),
```

`SessionService.CreateProject` is a 6-line delegate:
```go
func (s *SessionService) CreateProject(ctx context.Context, req *connect.Request[...]) (*connect.Response[...], error) {
    return s.projectSvc.CreateProject(ctx, req)
}
```

This is the Facade pattern (`code-architecture-best-practices`'s Decision Guide: "Simplify complex subsystem → Facade (named `service`)") applied at the ConnectRPC boundary: `SessionService` is the single type ConnectRPC's generated interface requires, but the actual logic for 24+ of its ~26 conceptual responsibility clusters already lives in dedicated, constructor-injected service types it composes. **New work in this project should extend this exact pattern, not invent a different one.**

## 2. What's NOT yet following the pattern (verified by body-line-count)

| Method(s) | Lines | Current home | Recommendation |
|---|---|---|---|
| `CreateCheckpoint`, `ListCheckpoints`, `ClearConversationState` | 34, 23, ~40 | inline in `session_service.go` | Extract `CheckpointService` |
| `registerDriver`, `stopAndDeregisterDriver`, `StopDriverForSession`, `buildTurnCallback`, `StartAutonomousDriverForInstance`, `StartAutonomousDriverWithTimeout`, `onAutonomousDriverComplete`, `wireRateLimitCallbacks` (48 lines), `wireAutoArchiveCallback`, `wireSessionExitedPublisher`, `wireStatusChangeCallback`, `wireClaudeSessionIDCallback` | ~250 total | inline | Extract `AutonomousOrchestrationService` |
| `GetProviderLimits` | 76 | inline | Evaluate individually — may be a legitimate one-off |
| `GetDetectionEvents` | 36 | inline | Evaluate individually |
| ~30 `Set*`/`Get*` wiring methods | 1-8 each | inline | Audit per-method: late-binding necessity vs. convertible to constructor injection |

Session lifecycle itself (`CreateSession`, `UpdateSession`, `DeleteSession`, `HibernateSession`, `ResumeHibernatedSession`, `ListSessions`, `GetSession`, `RenameSession`, `RestartSession`, `ArchiveSession`, `ForkSession`, `BatchCreateSessions`, `RunOneShot`, `WatchSessions`) is **not** a decomposition target — it's the cluster that gives `SessionService` its name and its legitimate aggregate-root responsibility. `UpdateSession` alone is ~220 lines (read in full earlier this session during the `instance-actor-concurrency` investigation) because session mutation genuinely has many independent optional fields (title, category, tags, program, working dir, rate-limit flag, autonomous mode, steering, status) — this is inherent complexity in the RPC's contract, not a missed extraction opportunity.

## 3. `AutonomousOrchestrationService` — proposed shape

```go
// server/services/autonomous_orchestration_service.go
type AutonomousOrchestrationService struct {
    headlessPool  *headless.Pool
    eventBus      *events.EventBus
    driverCtx     func() context.Context   // avoids importing SessionService itself
    onComplete    func(sessionTitle string, outcome session.AutonomousDriverOutcome)
    drivers       map[string]*session.AutonomousDriver
    driversMu     sync.Mutex // small, narrow — driver registry only, not Instance state
}

func NewAutonomousOrchestrationService(pool *headless.Pool, bus *events.EventBus) *AutonomousOrchestrationService
func (a *AutonomousOrchestrationService) StartForInstance(inst *session.Instance) { ... }
func (a *AutonomousOrchestrationService) StartWithTimeout(inst *session.Instance, d time.Duration) { ... }
func (a *AutonomousOrchestrationService) Stop(sessionTitle string) { ... }
func (a *AutonomousOrchestrationService) WireCallbacks(inst *session.Instance) { ... } // consolidates the 5 wireXCallback methods
```

`SessionService` keeps a `autonomousSvc *AutonomousOrchestrationService` field and delegates `StartAutonomousDriverForInstance`/`StartAutonomousDriverWithTimeout`/`StopDriverForSession` to it — same shape as `projectSvc`.

**Caution**: `onAutonomousDriverComplete` currently writes several `Instance` fields directly (`AutonomousMode`, `AutonomousTurn`, `AutonomousMaxTurns`, `AutonomousOutcome` — confirmed unguarded in the `instance-actor-concurrency` investigation's requirements.md background). Extracting this into a new service does **not** fix that race — it only relocates the same unguarded write. Do not treat this extraction as a substitute for `instance-actor-concurrency`'s Epic 5 (which converts these exact writes to actor commands); this project's job here is purely moving the method to a better-homed type, with the concurrency fix landing separately per the Sequencing section in `requirements.md`.

## 4. `CheckpointService` — proposed shape

```go
// server/services/checkpoint_service.go
type CheckpointService struct {
    storage *session.Storage
}
func NewCheckpointService(storage *session.Storage) *CheckpointService
func (c *CheckpointService) Create(ctx context.Context, req *connect.Request[...]) (*connect.Response[...], error)
func (c *CheckpointService) List(ctx context.Context, req *connect.Request[...]) (*connect.Response[...], error)
func (c *CheckpointService) ClearConversationState(ctx context.Context, req *connect.Request[...]) (*connect.Response[...], error)
```

Verify during implementation whether `CreateCheckpoint`'s existing logic calls `Instance.CreateCheckpoint()` (the session-package method already migration-sensitive per `instance-actor-concurrency`'s Epic 4 atomicity gate) — if so, this service becomes a thin RPC-to-domain-method adapter, which is exactly the intended shape and requires no coordination with the other project beyond not changing that call signature.

## 5. Tmux-content normalization (Item 2)

`session/detection/normalizer.go` (confirmed via `normalizer_test.go`) already implements `stripANSI` and CR-collapse (`PTYNormalizer`). Confirmed by direct read: `approval_handler.go` calls `inst.Preview()` (`session/instance_terminal.go:105`), which returns either `ClaudeController.GetRecentOutput()` (in-memory, already-processed-to-some-degree PTY buffer) or, as a fallback, `i.pm().CapturePaneContent()` — **raw tmux output, not passed through `PTYNormalizer`**. `approval_handler.go` does not import or call `session/detection`'s normalizer at all (confirmed: no `Normalizer`/`stripANSI`/`detection.` reference in `approval_handler.go`).

**Recommendation**: `Preview()`'s fallback path should normalize via the same `PTYNormalizer` the detection package already uses, before returning content to any caller — this fixes the coupling for every consumer of `Preview()` at once (including `approval_handler.go`), not just the one file the audit flagged, and reuses existing, already-tested code rather than writing new normalization logic.

**`pkg/classifier/classifier.go`**: read in full during this research — it has **no direct tmux dependency and no call to `Preview()`/pane-capture APIs** found. Its `.Output()` calls are `safeexec`-wrapped `git`/`jj` subprocess calls unrelated to tmux. The audit's reported co-change (3 shared commits, ratio 0.30) could not be independently confirmed as a causal dependency from this reading — **treat this as an unconfirmed, weaker signal; do not force a fix here without further evidence.** Possible explanations not requiring code coupling: coincidental commits that touched both files for unrelated reasons in a small sample (3 commits over 2 months is a thin signal), or a shared test fixture. Recommend: no action on `classifier.go` in this project unless implementation-time investigation finds an actual dependency.

## 6. Relationship to `instance-actor-concurrency`

Read `research/architecture.md` §1.2 in that project: `InstanceSnapshot` is already defined as one flat struct with fields *grouped by comment* (GitHub PR/URL block, autonomous-mode block, `ReviewState` embedded value type) but not yet split into named sub-structs. This project's `Instance`-adjacent scope is limited to: once that snapshot exists, extract its already-identified groups into embedded named types (`GitHubIntegration`, `AutonomousModeState`) — polish on an existing seam, not a competing decomposition. This is explicitly sequenced to land after that project's Epic 1 (additive snapshot infra) per `requirements.md`'s Sequencing section.
