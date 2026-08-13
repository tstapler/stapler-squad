# Research: Stack (Agent 1)

## 1. Background-firing mechanism — reuse the existing EventBus subscriber pattern, don't hook `backlog_lifecycle.go` directly

There is already a **precedent-exact** pattern for "fire background work when an item transitions to `done`, off the main path": `server/services/backlog_github_forward_sync.go`.

```go
// StartBacklogGitHubForwardSyncSubscriber (server/services/backlog_github_forward_sync.go:48)
func StartBacklogGitHubForwardSyncSubscriber(ctx context.Context, bus *events.EventBus, registry *session.PluginRegistry, syncLoop *session.SyncLoop, storage *session.Storage) {
	ch, _ := bus.Subscribe(ctx)
	go func() {
		for {
			select {
			case event, ok := <-ch:
				if !ok { return }
				if event == nil || event.Type != events.EventBacklogItemChanged { continue }
				payload := event.BacklogItemPayload
				if payload == nil || payload.Kind != events.BacklogChangeStatusTransition || payload.NewStatus != string(session.BacklogStatusDone) {
					continue
				}
				handleForwardSyncClose(ctx, registry, syncLoop, storage, payload.Item)
			case <-ctx.Done():
				return
			}
		}
	}()
}
```

This is registered once at startup in `server/server.go:647`:
```go
services.StartBacklogGitHubForwardSyncSubscriber(serverCtx, deps.EventBus, deps.BacklogService.Registry(), deps.BacklogService.SyncLoopForForwardSync(), deps.Storage)
```

**Recommendation:** build `StartCompletionReviewerSubscriber(ctx, bus, storage, ...)` in the same shape — one long-lived subscriber goroutine, filtering on `payload.NewStatus != string(session.BacklogStatusDone)` (which already excludes archive/superseded/failed, satisfying the "not on archive, not on other terminal states" AC for free) — rather than adding a hook directly inside `session/backlog_lifecycle.go`. This:
- Matches the requirement's "modeled on... a fire-and-forget hook" intent without adding new coupling to the lifecycle state machine file itself.
- Already gets "never blocks the main workflow" for free: the event is published *after* the transition commits (events.EventBus is fire-and-forget pub/sub), and the subscriber loop processing one event slowly doesn't block the publisher or other subscribers (each subscriber gets its own channel via `bus.Subscribe`).
- Test file convention: `server/services/backlog_github_forward_sync_test.go` is the direct template to copy for `completion_reviewer_test.go` — subscribe, publish a synthetic event, assert on the side effect, using `waitWithTimeout`-style patterns already present in `session/backlog_lifecycle_test.go:27-34`.

For the failure-swallowing requirement ("failures are logged, never surfaced to the user or the backlog item"), follow `handleForwardSyncClose`'s log-and-continue posture (`log.Warn`/`log.Error`, no error return, no item mutation on failure) — this repo's established idiom for this exact class of background side-effect, not a `recover()`/panic-guard pattern (grep across `server/services/*.go` and `session/*.go` found no goroutine-wrapping panic-recovery helper already in use for this kind of fire-and-forget hook, so don't invent one beyond what the existing subscriber does).

**Alternative considered and rejected:** a bare `go func() { ... }()` inserted directly at the `TransitionBacklogItemStatus(... BacklogStatusDone ...)` call sites in `session/backlog_lifecycle.go` (there are 4: lines 3256, 3280, 3649, 4462, 4907 per grep). Rejected because those call sites are scattered across the lifecycle file and would require the hook to be duplicated or threaded through as a callback at every one of them — the EventBus already unifies all of them into a single `EventBacklogItemChanged` stream, so subscribing once is strictly less invasive and keeps `backlog_lifecycle.go` from growing a new concern.

No `errgroup`/worker-pool needed — `golang.org/x/sync v0.20.0` is already a go.mod dependency (used elsewhere for `singleflight`/`errgroup` per repo convention) but this fire-and-forget shape doesn't need result aggregation or bounded concurrency; a single subscriber goroutine processing events serially (like the GitHub forward-sync one) is the matching precedent. If concurrent completion reviews ever need bounding, `errgroup.Group.SetLimit()` is available, but nothing in the current pattern uses it for this class of hook.

## 2. Spawning a restricted Claude session — use `session/headless` (Pool/CallOptions), NOT a full tmux `Instance`

Two spawn mechanisms exist in this codebase:

1. **`session.Instance` (tmux-backed, full interactive agent harness)** — `session/instance.go`, launched via `session/instance_tmux.go`. Supports `--allowedTools` (`InstanceOptions.AllowedTools`), `--permission-mode` (`PermissionMode` constants: `auto`, `bypassPermissions`, `acceptEdits`, `manual`), and `--mcp-config` (`claudeMCPConfigArgs()`, `instance_tmux.go:235`) for injecting the stapler-squad MCP server's tools (this is how `create_backlog_item`, etc. are exposed to a spawned session).
2. **`session/headless` (`Pool.CallBlocking`, one-shot subprocess `claude -p`)** — used today for triage (`server/services/backlog_service_triage.go`) and the empty-diff review-gate codebase check (`session/backlog_review.go`'s `BuildReviewCallOptions`). No tmux session, no persistent state, no UI surface — exactly the "short-lived" characteristic the requirements ask for.

**Recommendation: use `session/headless`.** It directly answers the requirements doc's open question ("is this a real spawned tmux session or a lighter-weight direct API call?") — the review-gate feature already answered this exact question for a structurally identical problem (a short-lived, narrowly-scoped, no-user-visible-session Claude call that must not get broad tool access), and its answer was `headless.Pool.CallBlocking` with `CallOptions{WorkDir, AllowedTools, PermissionMode}`, not a tmux `Instance`.

### CRITICAL finding: `AllowedTools`/`DisallowedTools` are NOT real technical enforcement for tools with an execution surface (Bash)

`project_plans/backlog-already-implemented/decisions/ADR-001-workdir-plus-defensive-tool-flags-for-headless-review.md` (2026-07-15 Addendum #2, "Bash Grant Reverted — Empirically Disproven") documents a load-bearing empirical finding directly relevant to this feature's "enforce tool restriction in code not prompt" AC:

> Both sub-tests found no real enforcement. The explicitly unlisted `whoami` command executed freely and wrote a real file to disk; the chained command after an allowed prefix also executed in full. Under `bypassPermissions`, `--allowedTools`/`--disallowedTools` behave as pre-approval hints at best for Bash — they are not a hard technical filter.

The test proving this, `TestPool_RealClaude_UnlistedBashCommand_BlockedOrAllowed` (`session/headless/integration_test.go:167`), is retained specifically so this isn't rediscovered the hard way. The distinction the ADR draws (`features.go:380-384`):

> The structural difference that makes `Read,Grep,Glob` safe where a scoped Bash grant is not: Read/Grep/Glob have no arbitrary-execution surface — their worst case is reading a file inside `WorkDir`. Bash's worst case is running any command the underlying shell can run, and this test proved the CLI's scoping syntax does not actually constrain that at the process level under `bypassPermissions`.

**Implication for this feature:** the "memory-write only" tool restriction is only trustworthy if:
- The completion reviewer is **never granted `Bash`** (or any tool with an unbounded execution surface — `Task`/delegation is the other one the requirements explicitly call out to exclude).
- The memory-write capability is exposed as a **narrow, non-executing MCP tool** (analogous to `Read`/`Grep`/`Glob`'s "worst case is reading a file" property) — e.g. a `stapler-squad__write_memory(item_id, content)`-shaped MCP tool with no free-form command/shell parameter — via the same `--mcp-config` injection mechanism `session/instance_tmux.go:235`'s `claudeMCPConfigArgs()` already uses for the stapler-squad MCP server, or an equivalent headless-compatible variant.
- `AllowedTools` is then set to **exactly and only** that MCP tool's name (mirroring `CodebaseReadAllowedTools = "Read,Grep,Glob"`'s "narrow allowlist of tools with no execution surface" shape) — this is the empirically-verified-safe pattern, not the disproven Bash-scoping pattern.
- This should almost certainly get its own integration test in the same style as `TestPool_RealClaude_UnlistedBashCommand_BlockedOrAllowed`/`TestPool_RealClaude_WorkDirOnly_GrantsReadAccess` before being trusted: attempt to invoke a non-memory tool (e.g. ask the model to run Bash or spawn a session) from inside the restricted call and confirm it is actually refused, not merely un-instructed. Do not assume the allowlist enforces this without re-running that class of empirical check for whatever tool-injection mechanism (`--mcp-config` + `--allowedTools`) ends up used.

### `headless.CallOptions` (session/headless/caller.go:18) — the struct to extend/reuse

```go
type CallOptions struct {
	WorkDir         string
	Model           string
	TimeoutSecs     int
	AllowedTools    string // e.g. "Read,Grep,Glob" — only applied when WorkDir is set
	PermissionMode  string
	DisallowedTools string // general-purpose plumbing, currently unused in production
}
```
`Pool.CallBlocking(ctx, key, systemPrompt, userPrompt string, opts CallOptions) (string, float64, error)` (`caller.go:487`) is the consolidated single entry point (see ADR-001's "Pool blocking-method consolidation" note — do not add a new 4th blocking method).

If the memory-write tool needs to be exposed via MCP rather than a flag, note `--mcp-config` is currently only wired for the tmux `Instance` path (`instance_tmux.go`), not `session/headless`'s `ProcessRunner` — extending `headless.ProcessRunner`/`CallOptions` with an equivalent `MCPConfig` pass-through (mirroring the existing `WithToolAccess` pattern for `AllowedTools`/`PermissionMode`/`DisallowedTools`) is likely new plumbing this feature needs to add, not something that already exists for the headless path.

## 3. Test conventions for `session/backlog_lifecycle.go` / this class of feature

- Package: `session` (in-package tests, not `session_test`), or `services` for anything living in `server/services/`.
- Imports typically include `github.com/stretchr/testify/{assert,require}`, `github.com/google/uuid`, and this repo's own `log`, `session/domain`, `session/git` packages as needed.
- Naming: `TestBacklogLifecycleListener_<Scenario>_<Effect>_When_<Condition>` (e.g. `TestBacklogLifecycleListener_OnSessionExited_WorkSession_TransitionsToReview`).
- Goroutine/async tests use a `waitWithTimeout(t, done <-chan struct{})` helper (`session/backlog_lifecycle_test.go:27-34`, 2s timeout, `t.Fatal` on timeout) — reuse this pattern (or its equivalent in `server/services`) for asserting the background reviewer actually ran/didn't run, rather than a raw `time.Sleep`.
- **Direct template for this feature's subscriber test**: `server/services/backlog_github_forward_sync_test.go` — same shape (EventBus subscriber, `BacklogStatusDone` filter), so a new `completion_reviewer_test.go` can mirror its setup/assertions almost line-for-line, replacing the GitHub-close side effect with a memory-write assertion (once #116 lands) or a call-was-attempted assertion via a fake `headless.Pool`/runner in the interim.
- `session/headless` already has a `fake_runner.go` (`session/headless/fake_runner.go`) used by `pool_test.go`/`features_test.go` to fake the `claude` subprocess without shelling out — reuse this for unit-testing the reviewer's call construction (system prompt, `CallOptions`) without a real `claude` binary, and reserve the real-binary integration test (build-tag `integration`, `make test-integration`) for the actual tool-restriction empirical check described in section 2.

## 4. Relevant existing dependencies (go.mod)

- `golang.org/x/sync v0.20.0` — already present; provides `errgroup`/`singleflight` if concurrency bounding is ever needed, though the recommended EventBus-subscriber shape (section 1) doesn't require it.
- `github.com/google/uuid` — for tagging memory entries with the source backlog item ID (already the ID type used throughout `session/backlog*.go`).
- No existing background-job/scheduler library (e.g. no cron/queue dependency) — this repo's convention for "background, best-effort, fire on an event" is the bespoke goroutine+channel subscriber pattern in section 1, not a generic job-scheduling package. Don't introduce one for this feature.
- Structured failure logging: this repo's own `github.com/tstapler/stapler-squad/log` package (`log.Warn`, `log.Error`, `log.Info` — see usage throughout `backlog_github_forward_sync.go`), not a new logging dependency.

## Open items for other research agents / plan phase

- The MCP-tool-exposure mechanism for memory-write (new MCP endpoint on the stapler-squad MCP server, vs. some other injection path) is not yet designed — this is downstream of the operator-memory store landing (#116) and should be scoped as its own seam in the plan, per the requirements doc's "Hard Dependency" section.
- Confirm whether `session/headless.ProcessRunner` needs `--mcp-config` support added (currently only on the tmux `Instance` path) — this is a plan-phase task, not yet implemented.
