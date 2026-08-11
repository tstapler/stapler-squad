# Pitfalls & Risks — Completion Reviewer

Research for `project_plans/completion-reviewer/`. Grounded in this repo's existing code
(paths/line numbers cited); repo-wide `Grep`/`ast-grep` search, no external sources needed.

## 1. Goroutine-leak / unbounded-fanout on bulk `done` transitions

**Risk**: `BacklogStatusDone` can be reached for many items in a short window (bulk-close,
reconciliation sweep, a batch import all clearing review at once — see
`session/backlog_lifecycle.go:3256`, `:3280`, `:3649`, `:4462`, `:4907`, all of which call
`TransitionBacklogItemStatus(..., BacklogStatusDone, ...)` from different code paths). If the
completion-reviewer hook does a bare `go func() { spawnReviewSession(item) }()` per transition
with no cap, N items closing together spawns N concurrent restricted Claude sessions — CPU,
Claude API rate limits, and (if it reuses `session.Instance`) N tmux panes at once.

**Existing pattern to reuse — do not invent a new one**: `BacklogLifecycleListener` already
solved this exact problem for review-gate spawns. It holds a **bounded semaphore channel**,
sized by a package constant, guarded by the shutdown context:

```go
// session/backlog_lifecycle.go:300-302
const maxConcurrentReviewGates = 8
// session/backlog_lifecycle.go:813
reviewSem: make(chan struct{}, maxConcurrentReviewGates),
```

Acquired at the `done`-adjacent transition site (`session/backlog_lifecycle.go:1011-1022`):

```go
go func() {
    // Acquire the bounded semaphore to prevent unbounded goroutine fan-out
    // when many sessions exit simultaneously.
    select {
    case l.reviewSem <- struct{}{}:
    case <-l.shutdownCtx.Done():
        return
    }
    defer func() { <-l.reviewSem }()
    l.spawnReviewGate(item, is)
}()
```

The same two-line idiom (`chan struct{}` sized by a named constant + `select` against
`l.shutdownCtx`) is reused independently by `session/pr_status_poller.go:211`,
`session/review_queue_poller.go:520`, and `session/worktree_pr_poller.go:206` — this is the
repo's standing convention for bounding any per-item background fan-out, not a one-off. The
completion reviewer should add its own `completionReviewSem` (or share `reviewSem` if the two
workloads are judged interchangeable in cost) rather than spawn unguarded goroutines.

**Also note**: there is a separate, unrelated `MaxConcurrentBacklogWorkItems` config
(`config/config.go:308-311`, `MaxConcurrentBacklogWorkItemsOrDefault()` at `:618`) — that one
caps concurrent **in-progress work sessions** (the WIP limit the user's memory notes refer to),
not background review/reviewer fan-out. Do not conflate the two; the completion reviewer needs
its own cap of the `reviewSem` shape, not a reuse of the WIP-limit config.

## 2. Prompt injection via attacker-influenceable backlog content — code-level enforcement is mandatory, not optional

**Risk**: item title/description/AC text can originate from an imported GitHub issue
(`mcp__stapler-squad__import_github_issue` exists as a tool, confirming this ingestion path is
real) — i.e. arbitrary external text that will be interpolated into the reviewer's prompt.
If tool restriction is expressed only as a system-prompt instruction ("you may only write to
memory"), a crafted issue body can override or distract from that instruction (classic
prompt-injection), and the session — if merely told not to use other tools rather than
technically prevented — could still invoke terminal/delegation/approval tools if the harness
grants them.

**Concrete failure mode if enforcement is instruction-only**: the model complies with the
injected instruction instead of the system prompt's, and calls a tool that was never revoked at
the transport/CLI level — because nothing in the process actually removed it from the tool
list. There is no code-level backstop catching that call; it just executes.

**This repo already has the correct mechanism, in production, for exactly this class of
problem** — `session/headless` package:

- `caller.go:32-46`: `AllowedTools` / `DisallowedTools` fields, documented explicitly as
  belt-and-suspenders: *"AllowedTools as belt-and-suspenders — an explicit denylist of
  destructive [tools]"* (`caller.go:44`).
- `runner.go:52,54`: these compile to literal `--allowedTools` / `--disallowedTools` flags
  passed to the `claude` CLI subprocess (`runner_test.go:24-51` pins the exact flag shape and
  ordering) — i.e. enforcement happens in the CLI's own invocation surface, not in a prompt
  string the model reads.
- Critically, **the repo does not trust that this flag silently works** —
  `session/headless/capability_check.go` (`CodebaseReadCapabilitySelfCheck`) is a once-per-process
  smoke test that writes a unique marker file and verifies a real `WorkDir+AllowedTools+
  PermissionMode` call actually reads it back, specifically because (per its own doc comment,
  `capability_check.go:24-27`) an `AllowedTools`/`PermissionMode`-bearing call could otherwise
  "silently produce zero real [capability]" — i.e. the flag could be a no-op under some CLI/config
  combination and nothing would visibly fail. `server/services/backlog_service_triage.go:2706-2715`
  consumes this self-check and produces an explicit `"Review UNVERIFIABLE"` status rather than
  silently proceeding as if the restriction held.

**Implication for the completion reviewer**: it must (a) enforce the memory-write-only tool list
via `headless.Caller`'s `AllowedTools`+`DisallowedTools` flags (or the equivalent gate on
whatever session-builder it uses), not a system-prompt sentence, and (b) either reuse
`DefaultCapabilitySelfCheck`/an analogous self-check, or add its own, so a silently-broken
restriction degrades to a logged no-op rather than an unverified assumption of safety. Given the
requirements doc's explicit AC ("the restricted session literally cannot invoke a non-memory
tool"), the self-check pattern is the only thing in this codebase that has ever tried to *prove*
that property empirically rather than assume it from the flag being passed.

## 3. tmux/session lifecycle: hidden `Instance` reuse risk vs. headless subprocess

**Risk named in the requirements' Open Questions**: whether the restricted session is a real
tmux-backed `session.Instance` or a lighter headless CLI call matters a lot for leak risk. This
repo has two live precedents pulling in different directions:

- **Review gates now use a real, hidden `session.Instance`** — `session/backlog_lifecycle.go:1027`'s
  comment: *"Review now always happens in a real, hidden session.Instance (see
  ReviewGateRunner.Run / SpawnReviewSession) instead of a synchronous in-process headless LLM
  call"* — i.e. the project deliberately moved *away* from headless-only for review gates,
  presumably because a real session gives resumability/observability the headless call didn't.
  Anything that reuses `session.Instance` inherits every tmux lifecycle risk this repo has
  already documented and hit in production: `.claude/rules/tmux-keep-server-on-restart.md`
  (a service restart without `--tmux-keep-server` kills every live tmux session, confirmed
  losing in-flight session state) and `.claude/rules/service-restart-orphan-process.md` (orphaned
  server processes on macOS restart can race a new process over the same tmux server/session
  state, confirmed via `ps`/`lsof` — four separate stale processes found live simultaneously).
  A completion-reviewer session that hangs (e.g. the restricted agent loops without ever calling
  the memory-write tool) and is tmux-backed is exactly the kind of process that would be
  orphaned/leaked across a restart, per that second rule.
- **`session/headless` exists specifically to avoid this class of risk** for capability checks
  and codebase-read reviews — it shells a bounded, timeout-guarded subprocess
  (`capabilityCheckTimeout = 30 * time.Second`, `capability_check.go:21`) rather than allocating
  a tmux pane/session at all. No tmux server, no orphan-on-restart surface, trivially killable
  via context cancellation.

**Recommendation for this plan (not yet a decision, since it's explicitly an Open Question)**:
given the requirements' "never blocks the main workflow" and "failures logged not surfaced" ACs
point toward a fire-and-forget, cheap, bounded-lifetime call — not something that needs
resumability or a UI-visible pane — the `session/headless.Caller` shape (subprocess +
`context.WithTimeout` + `shutdownCtx` wiring, matching the semaphore-goroutine pattern in
Pitfall 1) is the lower-risk choice unless a concrete reason emerges that the reviewer needs
full `session.Instance` machinery (resumable scrollback, `submit_review_verdict`-style MCP
callback, etc., none of which the requirements ask for — the reviewer only needs to invoke one
tool once). Reusing hidden `session.Instance` inherits two already-documented incident classes
for no benefit the requirements call for.

## 4. Silent failure becoming invisible technical debt

**Risk**: the AC "failures are logged, never surfaced to the user or the backlog item" is
correct as a *don't block the user* requirement, but taken alone it also describes a reviewer
that could fail 100% of the time (e.g. `AllowedTools` self-check silently degraded, or the
memory-store dependency — not yet built — is down) with nobody ever finding out, since nothing
downstream ever looks at it.

**What this repo already treats as the minimum bar for "logged, not surfaced" without becoming
invisible** — two adjacent precedents:
- `backlog_service_triage.go:2706-2715`'s capability self-check doesn't just log-and-continue on
  failure — it produces a distinct, greppable status string (`"Review UNVERIFIABLE: ..."`)
  attached to the record, so a human or a later automated pass *can* find every occurrence, even
  though no user-facing alert fires at the time.
- `.claude/rules/fix-flaky-tests-dont-defer.md` names the general anti-pattern this maps to:
  "known pre-existing flake, unrelated" repeatedly cited across PRs without ever being fixed,
  specifically because nothing forced a re-look. The rule's fix is procedural (don't re-excuse),
  but the structural precondition that let it recur was the same one this pitfall describes: a
  failure that's *observable in principle* but never surfaced anywhere anyone checks routinely.

**Minimum bar recommended** (does not violate "never surfaced to the user/item"): a
process-lifetime counter (e.g. `atomic.Int64`, matching the concurrency-primitive idiom already
used for `CodebaseReadCapabilitySelfCheck.ok`/`checked` at `capability_check.go:34-35`) for
attempts vs. failures, exposed the same way other internal counters in this codebase are exposed
(check `--profile`/pprof debug endpoints per `.claude/docs/profiling.md`, or a log line at
INFO with a running total on each failure, e.g. `log.WarningLog.Printf("[completion-reviewer]
failed (%d/%d total)", failures, attempts)`) — cheap enough that it can't itself become a new
blocking dependency, but present enough that "is the reviewer silently dead" is answerable by
grep instead of institutional memory.

## 5. Memory-store race/consistency risk (concurrent reviewers on related items)

**Risk**: two backlog items that are related (e.g. a parent/child pair, or two items touching
the same subsystem) can transition to `done` close together, and with the semaphore from
Pitfall 1 allowing up to N concurrent reviewers, two reviewer sessions could append to the same
memory-store file/record concurrently. Because `OperatorMemory`/`MemoryStore` doesn't exist yet
(confirmed by the requirements doc's repo-wide grep), this plan cannot point at an existing
concurrency contract for it — but it can point at how this codebase handles the identical shape
of problem (multiple goroutines appending to one persisted store) elsewhere, so the eventual
interface seam is designed against a known-safe idiom instead of a new one:

- `.claude/rules/go-double-checked-locking.md` (canonical example: `session/git/worktree_git.go`
  `IsDirty`) — the standing rule in this repo for read-check-write races is: always return the
  locally-computed value after taking the write lock, never re-read the shared slot, to avoid a
  lost-update race silently handing one goroutine another goroutine's result.
- `.claude/rules/fix-flaky-tests-dont-defer.md` cites
  `TestWriteSettingsAtomic_ConcurrentWritesToSameSettingsPath_NeverProduceCorruptJSON`
  (`server/services/hook_injector_test.go`) as the worked example for concurrent-writers-to-one-
  file: the fix there was a unique `os.CreateTemp` name + atomic rename, not a mutex alone —
  i.e. this codebase's precedent for "many goroutines, one shared persisted file" is
  write-to-temp-then-atomic-rename, not read-modify-write in place.

**Implication for the interface seam this plan must define** (per the requirements' "define the
shape this item needs from #116's interface, not assume a specific one"): the memory-write
primitive the completion reviewer calls should be specified as requiring **either** an atomic
append operation the store guarantees itself (preferred — pushes the concurrency contract onto
#116, where it belongs) **or**, if the store can only expose a raw read-modify-write file/blob,
the reviewer-side seam should assume it needs the same temp-file+atomic-rename discipline
`hook_injector_test.go` already validates in this repo, not a bare mutex around an in-place
write. The two related-items-race scenario should be an explicit test case handed to whoever
builds #116, since this plan cannot itself verify it against a store that doesn't exist yet.

## Summary of reusable patterns identified

| Risk | Existing repo pattern to reuse | Location |
|---|---|---|
| Unbounded fan-out | Bounded semaphore `chan struct{}` sized by a named const, `select` against `shutdownCtx` | `session/backlog_lifecycle.go:300-302,813,1011-1022` (also `pr_status_poller.go:211`, `review_queue_poller.go:520`, `worktree_pr_poller.go:206`) |
| Tool restriction enforced in code | `headless.Caller`'s `AllowedTools`/`DisallowedTools` → real CLI flags, not prompt text | `session/headless/caller.go:32-46`, `runner.go:52-54`, `runner_test.go:24-51` |
| Restriction silently not holding | Once-per-process empirical self-check with a real marker-file round trip; explicit "UNVERIFIABLE" status on failure, not silent pass-through | `session/headless/capability_check.go`, `server/services/backlog_service_triage.go:2706-2715` |
| tmux orphan/leak on restart | Avoid tmux-backed `session.Instance` for short bounded-lifetime calls; use headless subprocess + timeout instead | `.claude/rules/tmux-keep-server-on-restart.md`, `.claude/rules/service-restart-orphan-process.md`, `session/headless/capability_check.go:21` (`capabilityCheckTimeout`) |
| Silent 100%-failure debt | Cheap counter/log-with-total, not a UI surface — matches the "logged not surfaced" AC while staying discoverable | `capability_check.go:34-35` (atomic counters), `.claude/rules/fix-flaky-tests-dont-defer.md` (names the anti-pattern) |
| Concurrent writers to one store | Atomic write (temp file + rename), not in-place read-modify-write; never re-read a shared slot after a race | `.claude/rules/go-double-checked-locking.md`, `hook_injector_test.go`'s `TestWriteSettingsAtomic_ConcurrentWritesToSameSettingsPath_NeverProduceCorruptJSON` |
