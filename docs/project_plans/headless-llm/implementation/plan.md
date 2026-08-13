# Implementation Plan: headless-llm

**Feature**: Headless LLM interface — `session/headless` package with session pool, streaming, cache-optimized calling, and wiring into review gate + RunOneShot + new AI features + RunHeadlessCall RPC
**Date**: 2026-05-26
**Status**: Ready for implementation
**ADRs**: None (all choices validated against existing codebase patterns)

---

## Technology Validation

All choices are validated against the existing codebase. No novel dependencies introduced.

| Choice | Validation | Risk |
|---|---|---|
| `executor/safeexec.CommandContextPG` | Already used project-wide for background processes | None — canonical pattern |
| `executor.ManagedProcess` + `ScanLines` | Existing streaming primitive in `executor/managed_process.go` | None |
| `executor.ShortLivedCmd` for blocking calls | Used by `CLIAIClient.Complete()` | None |
| `executor/circuit_breaker.go` | Already implemented in codebase | None |
| ConnectRPC server-streaming pattern | WatchInsights / WatchSessions precedent | None |
| `go.uber.org/goleak` | Already in `go.sum` (transitive); needs explicit import in test | Low |
| `--output-format json` / `--output-format stream-json` | claude CLI documented flags; no existing usage → net-new | Low — verify against claude CLI version |
| `--exclude-dynamic-system-prompt-sections` | claude CLI documented flag | Low — same as above |
| `--system-prompt` flag | claude CLI documented flag | Low — same as above |
| OAuth via `~/.claude/` tokens | Documented claude CLI behavior; subprocess inherits env | None |

**Flag verification note**: Three new claude CLI flags (`--output-format`, `--exclude-dynamic-system-prompt-sections`, `--system-prompt`) have no existing usage in the codebase. They should be integration-tested against the actual claude binary during Epic 6. Add a smoke test that runs `claude --help` and greps for these flags at pool construction time.

**Circular import constraint**: `session/headless` must NEVER import `session` or `server` packages. All feature function parameters must be primitive types (string, []byte) only. If a future feature needs a type from `session/ent`, wrap it in the calling service before passing to headless, or define a DTO struct in `session/headless` that mirrors only the needed fields. Violating this constraint causes an import cycle that will break `go build`.

---

## Dependency Visualization

```
Epic 1 (session/headless package)
  ├── Story 1.1 (ClaudeRunner + ProcessRunner) ─────────────────────────┐
  ├── Story 1.2 (SessionState + Pool struct) ─────────────────────────┐ │
  ├── Story 1.3 (Pool.Call + acquireSession) ← depends on 1.1, 1.2   │ │
  ├── Story 1.4 (Pool.CallBlocking) ← depends on 1.3                 │ │
  └── Story 1.5 (FakeRunner + unit tests) ← depends on 1.1–1.4       │ │
                                                                       │ │
Epic 2 (feature AI services)                                           │ │
  └── Story 2.1 (feature functions + prompts) ← depends on Epic 1    ←┘ │
                                                                          │
Epic 3 (Replace SpawnReviewSession) ← depends on Epic 1              ←───┘
  └── Story 3.1 (remove ReviewGateSpawner, wire headless)
                                                                
Epic 4 (Replace RunOneShot) ← depends on Epic 1
  └── Story 4.1 (streaming upgrade, timeout, pool wiring)

Epic 5 (RunHeadlessCall RPC) ← depends on Epic 1
  ├── Story 5.1 (proto definitions + generate)
  ├── Story 5.2 (HeadlessService handler) ← depends on 5.1 + Epic 1
  └── Story 5.3 (server.go registration) ← depends on 5.2

Epic 6 (Wiring + tests) ← depends on all epics
  ├── Story 6.1 (server/dependencies.go wiring)
  └── Story 6.2 (integration tests + goleak)
```

---

## Phase 1: Core Headless Package

### Epic 1.1: `session/headless` Package Foundation
**Goal**: Implement the `ClaudeRunner` interface, `ProcessRunner` implementation, `SessionState`, `Pool` struct, and all pool logic — the complete headless calling infrastructure.

#### Story 1.1.1: Package scaffold and ClaudeRunner interface
**As a** developer, **I want** a `session/headless` package with a `ClaudeRunner` interface, **so that** the pool can accept real or fake subprocess runners.

**Acceptance Criteria**:
- `session/headless/` directory exists with `pool.go`, `caller.go`, `runner.go`
- `ClaudeRunner` interface has one method: `Run(ctx context.Context, args []string) (io.ReadCloser, error)`
- Package builds with `go build ./session/headless/...`
- No import of `session` package internals (no circular deps)

**Files**:
- `session/headless/runner.go`
- `session/headless/caller.go`
- `session/headless/pool.go`

##### Task 1.1.1a: Create `runner.go` with `ClaudeRunner` interface and `ProcessRunner` impl (~4 min)
- Create `session/headless/runner.go`
- Define `package headless`
- Define `StreamChunk` struct: `{ Text string; Err error; Done bool }`
- Define `ClaudeRunner` interface: `Run(ctx context.Context, args []string) (io.ReadCloser, error)`
- Implement `ProcessRunner` struct with `claudeBin string` field
- `ProcessRunner.Run()`: use `executor.ManagedProcess` (via `safeexec.CommandContextPG`) to start the process, return `p.Stdout()` as `io.ReadCloser` **and** a `func() error` stop function (i.e., `p.Stop`); on process start error return error
- **CRITICAL**: `ClaudeRunner` interface must be `Run(ctx, args) (stdout io.ReadCloser, stop func() error, err error)` so `Call()` holds the stop handle directly and can `defer stop()` unconditionally — prevents ManagedProcess leaks if the goroutine panics or the caller abandons the channel
- Add `var ErrClaudeNotFound = errors.New("claude binary not found in PATH")`
- Add error sentinel vars: `var ErrLLMError = errors.New("claude LLM error (exit 1)")`, `var ErrUsageError = errors.New("claude usage error (exit 2)")`, `var ErrInterrupted = errors.New("claude interrupted (exit 130)")`
- Files: `session/headless/runner.go`

##### Task 1.1.1b: Create `pool.go` stub with `FeatureKey` type and `SessionState` struct (~3 min)
- Create `session/headless/pool.go`
- Define `FeatureKey = string` type alias
- Define `SessionState` struct:
  ```go
  type SessionState struct {
      sessionID  string
      callCount  int
      procCtx    context.Context    // pool-level context for ManagedProcess lifetime
      procCancel context.CancelFunc
  }
  ```
- Define `PoolConfig` struct: `{ MaxCallsPerSession int; MaxConcurrentSessions int; DefaultModel string }`
- Define `Pool` struct with: `claudeBin string`, `cfg PoolConfig`, `runner ClaudeRunner`, `sessions map[FeatureKey]*SessionState`, `mu sync.Mutex` (global pool lock for map access), per-key `keyMu map[FeatureKey]*sync.Mutex`, `concurrencySem chan struct{}` (buffered to `MaxConcurrentSessions`; acquired before each subprocess start, released in goroutine defer)
- **Note**: `FeatureKey` must be defined as `type FeatureKey string` (a new named type), NOT `type FeatureKey = string` (alias) — the named type prevents accidental string injection at call sites
- Add `var defaultPoolMu sync.RWMutex` and `var defaultPool *Pool`; expose via `func DefaultPool() *Pool` getter (read-lock) and `func SetDefaultPool(p *Pool)` setter (write-lock) to avoid data races in tests
- Files: `session/headless/pool.go`

##### Task 1.1.1c: Create `caller.go` with `NewPool()` constructor (~3 min)
- Create `session/headless/caller.go`
- Implement `NewPool(cfg PoolConfig) (*Pool, error)`:
  - Call `exec.LookPath("claude")` once; return `ErrClaudeNotFound` on failure
  - Construct `ProcessRunner{claudeBin: bin}`
  - Initialize pool maps
  - Apply defaults: `MaxCallsPerSession=25`, `MaxConcurrentSessions=5`
- Implement `NewPoolWithRunner(cfg PoolConfig, runner ClaudeRunner) *Pool` for test injection (no LookPath)
- Files: `session/headless/caller.go`

---

#### Story 1.1.2: `Pool.acquireSession()` — first-call vs resumed-call logic
**As a** pool consumer, **I want** the pool to automatically establish a session on first use and resume it on subsequent calls, **so that** system-prompt prefix caching is achieved transparently.

**Acceptance Criteria**:
- First call per feature key uses `--output-format json` and captures `session_id` from JSON response
- Resumed calls use `--resume <session_id>` with plain output
- Session rotated when `callCount >= maxCalls` (default 25) or on non-zero exit
- Per-key mutex ensures serial access (no concurrent session sharing)

**Files**:
- `session/headless/caller.go`
- `session/headless/pool.go`

##### Task 1.1.2a: Implement `Pool.acquireSession()` with first-call JSON path (~5 min)
- In `caller.go`, implement `acquireSession(ctx context.Context, key FeatureKey, systemPrompt string, model string) (isFirstCall bool, args []string)`:
  - Lock per-key mutex (create if absent); read state; **release lock before returning** — the lock is only held long enough to read/write session state, NOT during subprocess execution (holding the lock during I/O would deadlock `rotateSession()`)
  - Check if `state.sessionID == ""` or `state.callCount >= cfg.MaxCallsPerSession`
  - **First call / rotation**: build args `["--output-format", "json", "--system-prompt", systemPrompt, "--exclude-dynamic-system-prompt-sections"]`; if model != "" add `["--model", model]`; set `isFirstCall=true`
  - **Resumed call**: build args `["--resume", state.sessionID, "--exclude-dynamic-system-prompt-sections"]`; set `isFirstCall=false`
  - Increment `callCount` under lock before releasing; return args (no error — errors come from subprocess)
  - After subprocess completes in `Call()`: re-acquire lock to store captured `session_id`
- Define `firstCallJSONResult` struct for JSON parsing: `{ SessionID string \`json:"session_id"\`; Result string \`json:"result"\`; CostUSD float64 \`json:"cost_usd"\` }`
- Files: `session/headless/caller.go`

##### Task 1.1.2b: Implement `rotateSession()` helper (~3 min)
- In `pool.go`, implement `(p *Pool) rotateSession(key FeatureKey)`:
  - Acquire per-key mutex
  - If state exists: call `state.procCancel()` to kill any running process
  - Replace map entry with fresh `SessionState{}`
- Called on error from `Call()` or `CallBlocking()` after 3 consecutive failures
- Add `consecutiveErrors int` counter to `SessionState`
- Files: `session/headless/pool.go`

---

#### Story 1.1.3: `Pool.Call()` — streaming output via channel
**As a** caller, **I want** `Pool.Call()` to return a `<-chan StreamChunk` that receives text as it's produced, **so that** the UI can display incremental progress.

**Acceptance Criteria**:
- `Call()` returns a buffered channel (buffer=16) and starts a goroutine that reads subprocess stdout line by line
- Context cancellation kills the subprocess and closes the channel
- Non-zero exit increments `consecutiveErrors`; at threshold 3, triggers `rotateSession()`
- First-call JSON path: parses JSON, sends `result` as a single chunk, captures `session_id`
- Goroutine uses `select { case ch <- chunk: case <-ctx.Done(): proc.Stop(); return }` to avoid blocking

**Files**:
- `session/headless/caller.go`

##### Task 1.1.3a: Implement `Pool.Call()` with streaming goroutine (~5 min)
- Implement `(p *Pool) Call(ctx context.Context, key FeatureKey, systemPrompt, userPrompt string) (<-chan StreamChunk, error)`:
  - Acquire session via `acquireSession()`
  - Build full args: acquired args + `[userPrompt]`
  - Start subprocess via `p.runner.Run(ctx, args)` → get `stdout io.ReadCloser`
  - Create buffered channel `ch := make(chan StreamChunk, 16)`
  - Acquire `p.concurrencySem` before starting subprocess (blocks if limit reached); release in goroutine defer
  - Start subprocess via `p.runner.Run(ctx, args)` → get `stdout io.ReadCloser, stop func() error`
  - Launch goroutine:
    - `defer close(ch)`
    - `defer stop()` — unconditional stop, prevents ManagedProcess leak on panic or channel abandon
    - `defer func() { <-p.concurrencySem }()` — release semaphore slot
    - `scanner := bufio.NewScanner(stdout)`
    - First call: accumulate full output into `[]byte`, parse JSON, send `result` as chunk, store `session_id` under per-key lock; on non-zero subprocess exit check stdout for error text and map exit code to `ErrLLMError`/`ErrUsageError`/`ErrInterrupted`
    - Resumed call: for each `scanner.Scan()`, `select { case ch <- StreamChunk{Text: line}: case <-ctx.Done(): stop(); return }`; on non-zero exit map exit code to error sentinel
    - On scanner error or process exit: send final `StreamChunk{Done: true, Err: err}` if err non-nil; increment `consecutiveErrors` under lock; rotate if threshold reached
  - Return `ch, nil`
- Files: `session/headless/caller.go`

##### Task 1.1.3b: Implement `Pool.CallBlocking()` convenience wrapper (~2 min)
- Implement `(p *Pool) CallBlocking(ctx context.Context, key FeatureKey, systemPrompt, userPrompt string) (string, error)`:
  - Call `p.Call()` → channel
  - Collect all `chunk.Text` into `strings.Builder`
  - Return on `chunk.Done || chunk.Err != nil`
  - Return collected text and first non-nil error
- Files: `session/headless/caller.go`

---

#### Story 1.1.4: `FakeRunner` and unit tests
**As a** developer, **I want** a `FakeRunner` for deterministic unit tests, **so that** tests do not require a real claude binary.

**Acceptance Criteria**:
- `FakeRunner` implements `ClaudeRunner`; configured with scripted responses
- Unit tests for `Pool.Call()`, `Pool.CallBlocking()`, session rotation, error circuit
- `goleak.VerifyTestMain(m)` present in `pool_test.go`
- All tests pass with `go test ./session/headless/...`

**Files**:
- `session/headless/fake_runner.go`
- `session/headless/pool_test.go`

##### Task 1.1.4a: Implement `FakeRunner` (~3 min)
- Create `session/headless/fake_runner.go`
- `FakeRunner` struct: `responses []string`, `errors []error`, `callCount int`, `mu sync.Mutex`
- `FakeRunner.Run(ctx, args) (io.ReadCloser, func() error, error)`: returns next scripted `strings.NewReader(response)` wrapped in `io.NopCloser`, a no-op stop function, or error; advances index
- **IMPORTANT**: FakeRunner must inspect `args` for `"--output-format"` + `"json"` to determine whether to return a JSON-formatted response or a plain-text response. Tests that exercise the first-call JSON path must ensure the scripted response is valid JSON matching `firstCallJSONResult` schema — document this requirement in test setup comments
- Helper `NewFakeRunner(responses ...string) *FakeRunner`
- Files: `session/headless/fake_runner.go`

##### Task 1.1.4b: Write unit tests for Pool (~5 min)
- Create `session/headless/pool_test.go`
- Add `TestMain(m *testing.M)` with `goleak.VerifyTestMain(m)`
- Test `TestPool_CallBlocking_FirstCall_CapturesSessionID`: FakeRunner returns valid JSON `{"session_id":"abc","result":"hello","cost_usd":0.001}`; assert returned text = "hello", session state has sessionID "abc"
- Test `TestPool_Call_ContextCancel_ClosesChannel`: cancel context mid-stream; assert channel closes without goroutine leak
- Test `TestPool_RotatesSession_AfterMaxCalls`: set `MaxCallsPerSession=2`; after 3 calls verify `sessionID` changed
- Test `TestPool_RotatesSession_AfterConsecutiveErrors`: FakeRunner returns errors; verify rotation after 3
- Test `TestPool_DifferentKeys_RunInParallel`: call two different feature keys concurrently; verify both complete
- Files: `session/headless/pool_test.go`

---

## Phase 2: Feature AI Services

### Epic 2.1: Thin Feature Wrappers
**Goal**: Implement four background AI feature functions as thin wrappers on `headless.Pool`.

#### Story 2.1.1: Feature functions and system prompts
**As a** service consumer, **I want** named functions for each AI feature, **so that** calling code does not embed prompts or know about feature keys.

**Acceptance Criteria**:
- `SummarizeBacklogItem`, `GenerateAcceptanceCriteria`, `DraftPRDescription`, `SuggestCommitMessage` compile and have unit tests
- Each function uses a stable system prompt (enables prefix caching across calls)
- Unit tests use `FakeRunner`; no real claude call required

**Files**:
- `session/headless/features.go`
- `session/headless/features_test.go`

##### Task 2.1.1a: Implement `features.go` (~5 min)
- Create `session/headless/features.go`
- Define feature key constants: `FeatureKeyReview = "review"`, `FeatureKeySummarize = "summarize"`, `FeatureKeyPRDescription = "pr-description"`, `FeatureKeyCommitMessage = "commit-message"`, `FeatureKeyCustom = "custom"`
- Implement `SummarizeBacklogItem(ctx context.Context, pool *Pool, title, description string) (string, error)`:
  - System prompt: `"You are a backlog analyst. Produce a one-paragraph summary and suggest up to 3 tags. Output JSON: {\"summary\":\"...\",\"tags\":[...]}"`
  - User prompt: `fmt.Sprintf("Title: %s\n\nDescription: %s", title, description)`
  - Call `pool.CallBlocking(ctx, FeatureKeySummarize, systemPrompt, userPrompt)`
- Implement `GenerateAcceptanceCriteria(ctx context.Context, pool *Pool, title, description string) ([]string, error)`:
  - System prompt: `"You are a product analyst. Output exactly 3-5 acceptance criteria as a JSON array of strings. Each criterion must be testable and specific."`
  - Parse JSON array from response
- Implement `DraftPRDescription(ctx context.Context, pool *Pool, diff, branchName string) (string, error)`:
  - System prompt: `"You are a technical writer. Draft a pull request description using Conventional Commit conventions. Format: ## Summary, ## Changes, ## Test plan."`
  - Truncate diff to 40,000 bytes if longer
- Implement `SuggestCommitMessage(ctx context.Context, pool *Pool, diff string) (string, error)`:
  - System prompt: `"You are a commit message expert. Output a single Conventional Commit message (type(scope): description). No extra text."`
  - Truncate diff to 20,000 bytes
- Files: `session/headless/features.go`

##### Task 2.1.1b: Unit tests for feature functions (~3 min)
- Create `session/headless/features_test.go`
- Test each function with a `FakeRunner` returning valid mock responses
- Test `GenerateAcceptanceCriteria` JSON parsing: mock response `["AC1","AC2","AC3"]`
- Test `DraftPRDescription` diff truncation: pass 50,000-byte diff, verify args contain ≤ 40,000 chars
- Files: `session/headless/features_test.go`

---

## Phase 3: Replace SpawnReviewSession

### Epic 3.1: Remove ReviewGateSpawner, Wire Headless Pool
**Goal**: Delete the heavyweight tmux-based review spawner and replace it with `headless.Pool.CallBlocking()`.

#### Story 3.1.1: Remove ReviewGateSpawner and update BacklogLifecycleListener
**As a** system, **I want** review gate calls to use the headless pool, **so that** review sessions no longer create tmux processes.

**Acceptance Criteria**:
- `session.ReviewGateSpawner` interface removed from `session/backlog_lifecycle.go`
- `BacklogLifecycleListener.sessionCreator` field replaced with `headlessPool *headless.Pool`
- `spawnReviewGate` uses `headlessPool.CallBlocking()` directly
- `SpawnReviewSession` method removed from `session_service.go`
- `make ci` passes

**Files**:
- `session/backlog_lifecycle.go`
- `server/services/session_service.go`
- `server/dependencies.go`

##### Task 3.1.1a: Remove `ReviewGateSpawner` interface and update `BacklogLifecycleListener` (~5 min)
- In `session/backlog_lifecycle.go`:
  - Delete `ReviewGateSpawner` interface definition
  - Replace `sessionCreator ReviewGateSpawner` field with `headlessPool *headless.Pool`
  - Update `NewBacklogLifecycleListenerWithSpawner(pool *headless.Pool, ...)` constructor signature
  - Update `NewBacklogLifecycleListener(pool *headless.Pool, ...)` (non-spawner constructor) likewise
- Files: `session/backlog_lifecycle.go`

##### Task 3.1.1b: Update `spawnReviewGate` to call headless pool (~4 min)
- In `session/backlog_lifecycle.go`, in `spawnReviewGate`:
  - Replace: `reviewSession, err := l.sessionCreator.SpawnReviewSession(ctx, item, is.ID.String(), prompt)`
  - With:
    ```go
    systemPrompt := buildReviewSystemPrompt()  // extract stable role+instructions from BuildReviewPrompt
    result, err := l.headlessPool.CallBlocking(ctx, headless.FeatureKeyReview, systemPrompt, prompt)
    ```
  - Remove the `reviewSession *session.Instance` reference; replace `reviewSession.ID` with a generated UUID for the `ItemSession` record
  - `SaveReviewVerdict` call remains unchanged
- Add `buildReviewSystemPrompt() string` function extracting the `## Your Role` + `## Instructions` sections from `BuildReviewPrompt`'s current implementation
- **CRITICAL context**: `spawnReviewGate` is launched as `go l.spawnReviewGate(item, is)`. The `ctx` passed into this goroutine must be derived from the listener's server-lifecycle context (e.g., `l.ctx` set at listener construction time from `server.Shutdown` signal), NOT from an RPC request context which will be cancelled when the triggering request completes. Ensure the listener constructor stores a server-lifecycle `context.Context` and `CancelFunc` for use in background goroutines.
- Files: `session/backlog_lifecycle.go`

##### Task 3.1.1c: Remove `SpawnReviewSession` from `session_service.go` (~2 min)
- In `server/services/session_service.go`:
  - Delete `SpawnReviewSession` method (currently at line ~464)
  - Remove `reviewGateSpawner ReviewGateSpawner` field from `SessionService` struct (if present; check current struct)
- Files: `server/services/session_service.go`

##### Task 3.1.1d: Update `NewBacklogLifecycleListenerWithSpawner` call in `server/dependencies.go` (~2 min)
- In `server/dependencies.go`:
  - Replace the `ReviewGateSpawner` argument in `NewBacklogLifecycleListenerWithSpawner` call with `deps.HeadlessPool`
  - Ensure `deps.HeadlessPool` is constructed before `BuildDependencies` calls this listener constructor
- Files: `server/dependencies.go`

---

## Phase 4: Replace RunOneShot

### Epic 4.1: Route RunOneShot Through Headless Package
**Goal**: Upgrade `RunOneShot` to use `headless.Pool.Call()` with streaming, raise timeout to 900s.

#### Story 4.1.1: Rewrite RunOneShot handler
**As a** user, **I want** `RunOneShot` to stream output and benefit from session resumption, **so that** calls are faster and more responsive.

**Acceptance Criteria**:
- `RunOneShot` uses `headless.DefaultPool.Call()` instead of raw `safeexec.CommandContext`
- Timeout default raised to 900s; configurable via `req.Msg.TimeoutSeconds`
- `exec.LookPath` call removed from handler
- Post-processing logic (PR URL extraction) preserved

**Files**:
- `server/services/session_service.go`

##### Task 4.1.1a: Rewrite `RunOneShot` to use headless pool (~5 min)
- In `server/services/session_service.go`, function `RunOneShot` (~line 2743):
  - Remove `exec.LookPath("claude")` call
  - Replace `context.WithTimeout` default from 300s to 900s
  - Replace `safeexec.CommandContext` + `CombinedOutput()` with:
    ```go
    output, err := s.headlessPool.CallBlocking(ctx, headless.FeatureKeyCustom, "", req.Msg.Prompt)
    ```
  - Pass `output` as the result string; preserve existing PR URL extraction logic
  - **Do NOT remove `workDir`** — existing `RunOneShot` sets `cmd.Dir = workDir` which git operations (PR URL extraction) depend on. Instead, add `WorkDir string` to an `options` variadic or a `CallOption` struct, and pass it through `Pool.Call()`/`Pool.CallBlocking()` to the subprocess. Update `PoolConfig` with a `workDir` field on `acquireSession` or add a `CallOptions` struct: `type CallOptions struct { Model string; WorkDir string; TimeoutSeconds int }`. Add `CallWithOptions(ctx, key, systemPrompt, userPrompt string, opts CallOptions)` variant.
- Files: `server/services/session_service.go`

##### Task 4.1.1b: Add `headlessPool` field to `SessionService` (~2 min)
- In `server/services/session_service.go`:
  - Add `headlessPool *headless.Pool` field to `SessionService` struct
  - Add `headlessPool` parameter to `NewSessionService(...)` constructor
- In `server/dependencies.go`: pass `deps.HeadlessPool` to `NewSessionService`
- Files: `server/services/session_service.go`, `server/dependencies.go`

---

## Phase 5: RunHeadlessCall RPC

### Epic 5.1: Proto Definitions
**Goal**: Define `RunHeadlessCallRequest`, `RunHeadlessCallResponse`, and `HeadlessService` in proto.

#### Story 5.1.1: Add proto definitions and regenerate
**As a** frontend developer, **I want** a `RunHeadlessCall` streaming RPC, **so that** the UI can trigger and display headless AI calls.

**Acceptance Criteria**:
- `proto/session/v1/headless.proto` defines `HeadlessService` with `RunHeadlessCall` RPC
- `make generate-proto` succeeds and produces Go + TypeScript bindings
- No existing proto files modified

**Files**:
- `proto/session/v1/headless.proto`
- (generated) `session/gen/session/v1/` (auto-generated)
- (generated) `web-app/src/gen/session/v1/` (auto-generated)

##### Task 5.1.1a: Write `headless.proto` (~3 min)
- Create `proto/session/v1/headless.proto`:
  ```protobuf
  syntax = "proto3";
  package session.v1;
  option go_package = "github.com/tstapler/stapler-squad/session/gen/session/v1;sessionv1";

  service HeadlessService {
    rpc RunHeadlessCall(RunHeadlessCallRequest) returns (stream RunHeadlessCallResponse) {}
  }

  message RunHeadlessCallRequest {
    string feature_key = 1;
    string system_prompt = 2;
    string user_prompt = 3;
    string model = 4;
    int32 timeout_seconds = 5;
  }

  message RunHeadlessCallResponse {
    string text = 1;
    bool done = 2;
    bool is_error = 3;
    string error_message = 4;
    double cost_usd = 5;
  }
  ```
- Files: `proto/session/v1/headless.proto`

##### Task 5.1.1b: Regenerate proto bindings (~2 min)
- Run `make generate-proto`
- Verify `session/gen/session/v1/sessionv1connect/headless.connect.go` exists
- Verify `web-app/src/gen/session/v1/headless_pb.ts` exists
- Commit generated files together with the proto source
- Files: all generated files under `session/gen/` and `web-app/src/gen/`

---

### Epic 5.2: HeadlessService Handler
**Goal**: Implement `HeadlessService` with `RunHeadlessCall` streaming handler.

#### Story 5.2.1: HeadlessService implementation
**As a** server, **I want** a `HeadlessService` handler that streams `Pool.Call()` output, **so that** the frontend receives real-time chunks.

**Acceptance Criteria**:
- `HeadlessService` implements `sessionv1connect.HeadlessServiceHandler` (compile-time check)
- `RunHeadlessCall` follows `WatchInsights` streaming pattern exactly
- Invalid/empty `feature_key` returns `connect.CodeInvalidArgument`
- Context cancellation stops the subprocess and returns `nil` from the handler
- Allowed feature keys: `"review"`, `"summarize"`, `"pr-description"`, `"commit-message"`, `"custom"`

**Files**:
- `server/services/headless_service.go`

##### Task 5.2.1a: Implement `server/services/headless_service.go` (~5 min)
- Create `server/services/headless_service.go`:
  ```go
  package services

  type HeadlessService struct { pool *headless.Pool }

  var _ sessionv1connect.HeadlessServiceHandler = (*HeadlessService)(nil)

  func NewHeadlessService(pool *headless.Pool) *HeadlessService { ... }

  func (s *HeadlessService) RunHeadlessCall(
      ctx context.Context,
      req *connect.Request[sessionv1.RunHeadlessCallRequest],
      stream *connect.ServerStream[sessionv1.RunHeadlessCallResponse],
  ) error {
      // validate feature_key
      // apply timeout from req.Msg.TimeoutSeconds (default 900s)
      ch, err := s.pool.Call(ctx, req.Msg.FeatureKey, req.Msg.SystemPrompt, req.Msg.UserPrompt)
      // forward chunks via WatchInsights pattern
  }
  ```
- Allowed keys validated against the `FeatureKey*` constants in `session/headless/features.go`
- Files: `server/services/headless_service.go`

---

### Epic 5.3: Server Registration
**Goal**: Register `HeadlessService` in `server.go`.

#### Story 5.3.1: Register handler in server.go
**As a** server, **I want** `HeadlessService` registered at startup, **so that** clients can connect to it.

**Acceptance Criteria**:
- `HeadlessService` registered in `server/server.go` with nil guard pattern (matching `InsightsService`)
- Optional `StreamingWSBridge` entry for SSE support (if browser streaming needed)
- `make build` succeeds

**Files**:
- `server/server.go`
- `server/dependencies.go`

##### Task 5.3.1a: Wire `HeadlessService` into `server/server.go` (~3 min)
- In `server/server.go`, add registration block:
  ```go
  if deps.HeadlessPool != nil {
      hlSvc := services.NewHeadlessService(deps.HeadlessPool)
      hlPath, hlHandler := sessionv1connect.NewHeadlessServiceHandler(hlSvc, ConnectOptions(deps.ErrorRegistry)...)
      srv.RegisterConnectHandler("/api"+hlPath, http.StripPrefix("/api", hlHandler))
  }
  ```
- Files: `server/server.go`

---

## Phase 6: Wiring and Tests

### Epic 6.1: Wire `headless.DefaultPool` into Dependencies
**Goal**: Construct and inject the headless pool at startup.

#### Story 6.1.1: Add HeadlessPool to ServerDependencies
**As a** server, **I want** `headless.DefaultPool` constructed at startup and available to all services, **so that** session state is shared across services.

**Acceptance Criteria**:
- `ServerDependencies.HeadlessPool *headless.Pool` field added
- `BuildDependencies()` constructs pool (non-fatal if claude not found — log warning, leave nil)
- `HeadlessPool` nil-guarded in all consumers
- `DefaultPool` package-level var set to the wired pool

**Files**:
- `server/dependencies.go`
- `session/headless/pool.go`

##### Task 6.1.1a: Add `HeadlessPool` to `ServerDependencies` and construct in `BuildDependencies` (~4 min)
- In `server/dependencies.go`:
  - Add `HeadlessPool *headless.Pool` to `ServerDependencies` struct
  - In `BuildDependencies()`, after config loading:
    ```go
    pool, err := headless.NewPool(headless.PoolConfig{
        MaxCallsPerSession:    25,
        MaxConcurrentSessions: 5,
        DefaultModel:          cfg.DefaultModel(),
    })
    if err != nil {
        log.Warn("headless pool disabled: claude binary not found", "err", err)
    } else {
        deps.HeadlessPool = pool
        headless.SetDefaultPool(pool)  // use thread-safe setter, not direct var assignment
    }
    ```
- Files: `server/dependencies.go`

---

### Epic 6.2: Integration Tests and Goroutine Leak Detection
**Goal**: End-to-end integration test for `RunHeadlessCall` RPC and goroutine safety verification.

#### Story 6.2.1: Integration test for RunHeadlessCall RPC
**As a** QA engineer, **I want** an integration test that calls `RunHeadlessCall` and receives chunks, **so that** the full pipeline is verified.

**Acceptance Criteria**:
- Integration test skipped if `CLAUDE_INTEGRATION_TESTS != "true"`
- Test calls `RunHeadlessCall` with `feature_key="custom"` and a simple prompt
- Test asserts at least one non-empty chunk received and `done=true` as final chunk
- `goleak.VerifyTestMain(m)` in headless package tests catches goroutine leaks

**Files**:
- `session/headless/integration_test.go`
- `server/services/headless_service_test.go`

##### Task 6.2.1a: Write integration test for pool (`integration_test.go`) (~4 min)
- Create `session/headless/integration_test.go` with `//go:build integration` tag (build tag alone is sufficient — do NOT also add a runtime `os.Getenv` skip, which is redundant)
- Test `TestPool_RealClaude_SimplePrompt`:
  - Create real `NewPool()`, call `CallBlocking(ctx, FeatureKeyCustom, "", "Say hello")`
  - Assert non-empty result
- Test `TestPool_RealClaude_SessionResumption`:
  - Call twice on same feature key; assert second call uses `--resume` flag (inspect args via a wrapping runner)
- Files: `session/headless/integration_test.go`

##### Task 6.2.1b: Write `HeadlessService` unit test with mock pool (~3 min)
- Create `server/services/headless_service_test.go`
- Test `TestHeadlessService_RunHeadlessCall_StreamsChunks`:
  - Use `FakeRunner` to return multi-line output
  - Call `RunHeadlessCall` via in-process server
  - Assert all chunks received in order
- Test `TestHeadlessService_RunHeadlessCall_InvalidFeatureKey`:
  - Expect `connect.CodeInvalidArgument` error
- Files: `server/services/headless_service_test.go`

##### Task 6.2.1c: Final CI validation (~2 min)
- Run `make ci` and confirm green
- Run `make quick-check`
- Verify `make build` produces no warnings about unused interfaces
- Fix any compilation errors from interface removal (ReviewGateSpawner references in test files)
- Files: any files with compile errors

---

## Summary

| Dimension | Count |
|---|---|
| Epics | 6 |
| Stories | 10 |
| Tasks | 22 |
| New files | 11 (`runner.go`, `caller.go`, `pool.go`, `fake_runner.go`, `pool_test.go`, `features.go`, `features_test.go`, `headless_service.go`, `headless_service_test.go`, `integration_test.go`, `headless.proto`) |
| Modified files | 5 (`backlog_lifecycle.go`, `session_service.go`, `dependencies.go`, `server.go`, and generated proto files) |

## Flagged Technology Choices

1. **`--output-format json` / `stream-json` / `--system-prompt` / `--exclude-dynamic-system-prompt-sections`** — Net-new claude CLI flags with no existing usage in the codebase. Flag availability should be verified at pool construction time (run `claude --help | grep "output-format"` in a smoke test). Add a `validateCLIFlags()` function called by `NewPool()` in non-test builds.

2. **`--output-format stream-json` for resumed calls** — The initial plan uses plain output for resumed calls (not `stream-json`). This means streaming resumed calls produce line-at-a-time plain text, not structured JSON events. This is intentional (matches the headless scripting wiki recommendations) but means cost tracking is only available on first calls.

3. **`goleak`** — Already in `go.sum` as a transitive dependency; adding a direct import is low-risk. Verify `go.uber.org/goleak` version matches expected API (`VerifyTestMain`).

## Implementation Order

Strictly sequential by Epic. Epic 1 must be complete (and `FakeRunner` working) before any other Epic begins. Epics 3, 4, 5 can proceed in parallel once Epic 1 is done. Epic 6 is last.
