# ADR-003: Checkpoint Storage Model (Adjacency List in JSON)

**Status**: Accepted
**Date**: 2026-04-03
**Context**: Session Resumption feature — how to model session checkpoints for bookmarking and forking.

## Decision

Add a `Checkpoint` struct to the existing JSON session storage (`session/storage.go`). Use an adjacency list model (`ParentID` self-reference) to represent the checkpoint DAG. Do NOT introduce Ent ORM or any new database for checkpoints.

### Schema

```go
type Checkpoint struct {
    ID             string    `json:"id"`
    SessionID      string    `json:"session_id"`
    ParentID       string    `json:"parent_id,omitempty"`
    Label          string    `json:"label,omitempty"`
    ScrollbackSeq  int64     `json:"scrollback_seq"`
    ScrollbackPath string    `json:"scrollback_path"`
    ClaudeConvUUID string    `json:"claude_conv_uuid,omitempty"`
    GitCommitSHA   string    `json:"git_commit_sha,omitempty"`
    Timestamp      time.Time `json:"timestamp"`
}
```

### Storage Location

Checkpoints are stored as a slice on `InstanceData`:
```go
type InstanceData struct {
    // ... existing fields ...
    Checkpoints      []Checkpoint `json:"checkpoints,omitempty"`
    ActiveCheckpoint string       `json:"active_checkpoint,omitempty"`
    ForkedFromID     string       `json:"forked_from_id,omitempty"`
}
```

This keeps checkpoints co-located with the session they belong to.

## Alternatives Considered

1. **Ent ORM with dedicated Checkpoint table**: Rejected. The codebase uses a JSON-backed `Repository` interface for primary session storage. Introducing Ent for checkpoints alone would add unnecessary complexity.

2. **Separate JSONL checkpoint file per session**: Rejected. Adds file management complexity. Checkpoints are small (tens per session). Embedding in the session record is simpler and atomic.

3. **Materialized path (e.g., "root/ckpt-1/ckpt-2")**: Deferred. Adjacency list with `ParentID` is sufficient for the expected checkpoint depth.

## Consequences

- `InstanceData` gains 3 new fields (`Checkpoints`, `ActiveCheckpoint`, `ForkedFromID`)
- Checkpoint creation is non-blocking: captures `{scrollback_seq, git_sha, timestamp}` without process interruption
- Fork operation reads checkpoints from the source session, clones scrollback up to `ScrollbackSeq`, and creates a new session with `ForkedFromID` set
- Checkpoint list bounded implicitly by user behavior (manual creation). No automatic pruning in MVP.

## Risks

- If a session accumulates hundreds of checkpoints, JSON grows. Mitigation: each session is serialized independently; hundreds of small structs add at most a few KB.
- Checkpoint data references file paths that may be deleted. Mitigation: checkpoints are metadata pointers; the fork operation validates file existence before proceeding.
