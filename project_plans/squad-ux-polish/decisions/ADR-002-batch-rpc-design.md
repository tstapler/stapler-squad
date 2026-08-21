# ADR-002: Batch Session Creation RPC Design

**Status**: Accepted
**Date**: 2026-04-17
**Deciders**: Tyler Stapler

---

## Context

Story 2 requires creating N sessions from a single user interaction (paste N task lines → N sessions). The primary constraint is that `git worktree add` is not safe for concurrent invocations on the same repository. The pitfalls research confirmed that concurrent `worktree add` calls share `.git/objects/` and `.git/index.lock`, and a crash during any one call can leave a stale lock file that blocks all subsequent git operations on that repo.

The question is where to own the orchestration: server or client.

---

## Decision

Add a new `BatchCreateSessions` RPC to `SessionService`. The server implementation uses a bounded sequential worker pool (max 3 concurrent creates across all repos; serial within the same repo path) to prevent git worktree race conditions. Each item in the batch returns a `BatchCreateResult{id, title, success, error, session}`.

Client-side N calls are rejected.

---

## Options Considered

### Option A: Client-side N repeated CreateSession calls (rejected)

The UI fires N independent `CreateSession` RPCs.

**Rejected because**:
- No server-side throttle on concurrent `git worktree add` calls; concurrent calls on the same repo will corrupt `.git/index.lock`
- No structured partial-failure aggregation; UI must reconstruct success/failure state from N independent responses
- If the user navigates away during creation, some sessions are created and some are not, with no cleanup path
- Cannot enforce the max-20-per-batch limit at the API boundary

### Option B: CreateSession with batch_id field + server fanout (rejected)

Add `optional string batch_id = 14` to `CreateSessionRequest`. Server groups concurrent requests sharing a batch_id.

**Rejected because**:
- Requires streaming or polling to observe batch progress — awkward for a request-response protocol
- Coordination state must be stored server-side per batch_id with a TTL; adds complexity for marginal gain
- Semantics are ambiguous: what if two requests with the same batch_id arrive minutes apart?

### Option C: New BatchCreateSessions RPC (accepted)

New first-class RPC with `repeated BatchSessionRequest → BatchCreateSessionsResponse{repeated BatchCreateResult}`.

**Accepted because**:
- Server owns throttling: one place to enforce the worktree serialization invariant
- Partial success is first-class at the protocol level; each item carries its own `success` flag and `error` string
- Consistent with Google API Design Guide batch method pattern
- Reuses all existing `CreateSession` handler logic per item; no duplication of business rules
- Max batch size (20) enforced at the RPC boundary before any work begins

---

## Proto Design

```protobuf
rpc BatchCreateSessions(BatchCreateSessionsRequest)
    returns (BatchCreateSessionsResponse) {}

message BatchSessionRequest {
  string title         = 1;  // Optional; server generates from task_text if empty
  string path          = 2;
  string branch        = 3;
  string program       = 4;
  string initial_prompt = 5;
  string task_text     = 6;  // Raw task line from textarea; used as title if title empty
  repeated string tags = 7;
  string category      = 8;
  bool   auto_yes      = 9;
}

message BatchCreateSessionsRequest {
  repeated BatchSessionRequest sessions = 1;
  // Optional: max concurrent worktree operations (default 3, max 5)
  int32 max_concurrency = 2;
}

message BatchCreateSessionsResponse {
  repeated BatchCreateResult results = 1;
  int32 succeeded = 2;
  int32 failed    = 3;
}

message BatchCreateResult {
  string  task_text = 1;  // Echo of input task_text for correlation
  string  title     = 2;  // Assigned title (may differ if deduped)
  bool    success   = 3;
  string  error     = 4;  // Populated on failure
  Session session   = 5;  // Populated on success
}
```

---

## Consequences

**Positive**:
- Single atomic API call for the user action; clean partial-failure model
- Server controls resource pressure (tmux processes + git worktrees are expensive)
- Unique title suffix strategy (append `-N` or 6-char hex) prevents tmux naming collisions deterministically

**Negative / Accepted**:
- Sequential worktree creation within a repo means batch of 10 takes ~10x single session time
- New RPC adds protobuf schema + codegen step (`make generate-proto`)
- The RPC is synchronous (no streaming progress); for 20 sessions the user waits up to ~60s; a spinner with "N of M created" is sufficient UX

**Implementation notes**:
- Use a `semaphore` channel (buffered channel of size `maxConcurrency`) to bound parallelism globally
- Use a per-repo-path keyed mutex (`sync.Map` of `*sync.Mutex`) to serialize worktree creation per repo
- Title deduplication: append `-<2-digit-index>` suffix (e.g., "Fix auth -01", "Fix auth -02") before sanitization
- On partial failure: already-created sessions are NOT rolled back; `BatchCreateResult.error` explains each failure; UI offers "delete failed stubs" action via existing `DeleteSession` RPC
- Validate max 20 sessions before any creation begins; return `CodeInvalidArgument` if exceeded
