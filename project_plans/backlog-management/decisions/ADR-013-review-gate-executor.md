# ADR-013: Review Gate Executor

**Status**: Accepted
**Date**: 2026-05-10

## Context

When a session completes (detected via the existing `EventExited` lifecycle hook), a review gate must evaluate the session's output against the backlog item's acceptance criteria and produce a structured verdict before the item is allowed to transition to `done`. The verdict must be:

- **Structured**: per-criterion `PASS | FAIL | UNVERIFIABLE`, not a free-form opinion
- **Auditable**: reasoning and evidence must be preserved for human inspection
- **Actionable**: the verdict must land in the database without the Go server needing to parse LLM output text
- **Extensible**: the gate should be able to run static analysis tools (gosec, test suite) in addition to LLM review

Four execution strategies were evaluated. The primary concerns are reliability, auditability, and avoiding fragile LLM text parsing in Go.

## Decision

Spawn a dedicated short-lived `one_shot` Stapler Squad session tagged `backlog:review` to run the review gate. The session calls the `submit_review_verdict` MCP tool to write its verdict directly to the database.

**Execution flow**:

1. `EventExited` fires for a session linked to a backlog item in `in_progress` state.
2. The `BacklogLifecycleListener` (a `LifecycleListener`) enqueues a review gate job.
3. The Go server spawns a new session with:
   - `one_shot = true` (auto-exits after completing its task)
   - Working directory: the worktree of the completed session (so `git diff`, `go test`, and `gosec` run against the actual code)
   - Tag: `backlog:review` (filtered from the default session list view)
   - Initial prompt: a structured template containing the git diff summary, AC list, and instructions to call `submit_review_verdict`
   - MCP server URL injected so the session can reach `submit_review_verdict`
4. The review session runs `git diff <base>..HEAD`, optionally runs `go test ./...` and `gosec`, then calls:

```
submit_review_verdict(
  item_id:    "<uuid>",
  session_id: "<uuid>",
  verdicts: [
    { criterion_index: 0, outcome: "PASS",         evidence: "Tests added in foo_test.go:42–67" },
    { criterion_index: 1, outcome: "FAIL",         evidence: "No DB migration found for schema change" },
    { criterion_index: 2, outcome: "UNVERIFIABLE", evidence: "Cannot run infra integration tests locally" },
  ],
  summary: "Overall: PARTIAL — 1 of 3 criteria unmet"
)
```

5. `submit_review_verdict` is an MCP tool implemented in `server/mcp/tools_backlog.go`. It writes the `ReviewVerdict` record to the database and transitions the item from `in_progress` to `review`.
6. The review session exits (`one_shot`). `EventExited` fires for the review session; the `BacklogLifecycleListener` detects `session_role = "review"` and does not trigger another gate (preventing infinite recursion).
7. The human receives a notification with the verdict summary and approve/reopen actions.

The Go server does not parse any LLM output text. All structured data flows through the typed `submit_review_verdict` MCP tool call.

## Alternatives Considered

**Option A: Go goroutine calling the Anthropic LLM API directly**

A Go function constructs a review prompt, calls the Anthropic API, and parses the JSON response in the Go request path. This is faster (no session startup overhead) and simpler (no tmux, no process management).

Rejected for the following reasons:
- Adds an Anthropic API client dependency to the Go server, which currently has none. This increases the attack surface and binary size.
- LLM calls in a goroutine either block the request path (unacceptable for a long-running review) or require a complex async pipeline with goroutine lifecycle management, error handling, and cancellation.
- Produces no transcript or audit trail. If the verdict is wrong, there is no record of the reasoning used to reach it.
- The review prompt must be iterated on as false positive/negative rates are observed. With the goroutine approach, every prompt change requires a binary rebuild and service restart.
- Error handling (rate limits, timeouts, context exhaustion) must be implemented from scratch rather than relying on Claude Code's existing retry and compaction logic.
- The review gate should be able to run tools (gosec, test suite) before calling the LLM. A goroutine cannot spawn subprocesses in the worktree as safely as a full session.

**Option B: Inline goroutine in the session exit hook**

Similar to Option A but triggered directly from the `EventExited` handler rather than from a queue. The handler spawns a goroutine that does the review inline.

Rejected: same objections as Option A, plus the additional problem that the `EventExited` handler is called synchronously in the lifecycle event loop. Blocking it (or spawning unbounded goroutines from it) creates resource leaks under high session churn. The queued approach in the chosen design avoids this.

**Option C: Human-only review**

The gate produces no automated verdict; humans must manually read the diff and tick AC items.

Rejected: the review gate is a core requirement (US-8) and one of the key differentiators from Gastown/Beads. Manual-only review does not scale and does not produce machine-readable `ReviewVerdict` records that drive state transitions. Human override remains available on top of the automated verdict.

## Consequences

**Positive**

- Reuses all existing session infrastructure (tmux, worktree, lifecycle events, MCP injection, one-shot flag). No new execution primitives required.
- Produces an auditable transcript: the review session's tmux scrollback preserves the reasoning, tool calls, and evidence the agent used to reach each verdict. Humans can inspect it for any item.
- No LLM text parsing in Go: the `submit_review_verdict` MCP tool is strongly typed. Invalid or missing fields return a structured error; the review session can retry or escalate.
- The review agent can run static analysis tools (gosec, `go test`) before calling the LLM. Tool results become evidence in the verdict, not just prose.
- The review prompt can be iterated on without rebuilding the binary: it is a text template in the session's initial prompt, managed as a versioned config value.
- Rate limiting and context window management are Claude Code's responsibility, not the Go server's.

**Negative**

- Approximately 3–8 seconds of session startup overhead per review. Acceptable given that review gates run post-session, not on the critical path.
- The review session is visible in the session list (mitigated by the `backlog:review` tag and default filter). Users unfamiliar with the feature may be confused by short-lived sessions appearing and disappearing.
- One-shot sessions accumulate as historical records. Implement auto-hide of stopped `backlog:review` sessions after 24 hours in the UI.
- If the review session itself crashes or times out, the item stays in `in_progress`. Mitigation: the `BacklogLifecycleListener` reconciliation pass detects items whose review session has been stopped for more than N minutes without a verdict and either retries or transitions to `review` with an `auto_timeout` note.
- Recursive gate prevention requires the `BacklogLifecycleListener` to check `session_role` before enqueueing a gate job. This logic must be tested explicitly.
