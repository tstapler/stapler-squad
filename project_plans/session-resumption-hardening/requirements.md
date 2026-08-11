# Requirements: Session Resumption Hardening

**Status**: Draft | **Phase**: 1 — Ideation complete
**Created**: 2026-04-15
**Predecessor plan**: `project_plans/session-resumption/` (Phase 1 largely implemented; Phases 2-3 outlined)

---

## Problem Statement

Session resumption infrastructure exists but is fragile. Phase 1 of the original plan was built but never fully closed: the HistoryLinker is not wired into server startup (so UUID detection never runs in production), cold restore has no integration test (so regressions go undetected), CaptureCurrentState is not called on shutdown (so WorkingDir is stale at cold restore time), and proto fields for history/UUID are not populated in the session adapter (so the web UI never sees them).

Beyond the gaps, Phase 2 is unstarted: users cannot create checkpoints (named bookmarks of session state), and there is no fork capability. The goal of this iteration is to make session resumption actually work end-to-end — from first-run UUID detection through graceful shutdown state capture through cold restore — and to deliver checkpoint creation as the foundation for future fork work.

Primary user: solo developer running multiple Claude Code sessions across projects who needs zero conversation loss on restart.

---

## Success Criteria

1. **Zero conversation loss**: After any restart (SIGTERM, crash, kill of tmux server), all running sessions resume with their original Claude conversation UUID passed via `--resume`. No manual re-establishment of context.
2. **UUID detection observable**: The web UI displays `claude_conversation_uuid` on a session card within 10 seconds of Claude starting a new conversation.
3. **Cold restore tested in CI**: `session/instance_cold_restore_test.go` passes under `go test -race` — no manual testing required for the cold restore path.
4. **Checkpoint creation working**: A user can create a named checkpoint on any running session via the web UI; checkpoints appear in the session detail with label, timestamp, and git SHA.
5. **All paths tested**: New code has companion `_test.go` files; `make quick-check` stays green.

---

## Scope

### Must Have (this iteration)

**Phase 1 gap closure:**
- Wire `HistoryLinker` into `server/dependencies.go` startup so UUID detection actually runs
- Call `Instance.CaptureCurrentState()` during graceful shutdown to persist WorkingDir before tmux dies
- Write `session/instance_cold_restore_test.go` integration test covering the cold restore branch
- Populate `history_file_path` and `claude_conversation_uuid` in `server/adapters/instance_adapter.go` so proto fields reach the web UI

**Phase 2 — Checkpoint creation:**
- `Checkpoint` struct, `InstanceData.Checkpoints`, and serialization (backward-compatible JSON)
- `Instance.CreateCheckpoint(label)` service method (captures scrollback seq, git SHA, conversation UUID)
- ConnectRPC endpoints: `CreateCheckpoint`, `ListCheckpoints`, `DeleteCheckpoint`
- Web UI: "Create Checkpoint" button on session card with label input, checkpoint list in session detail

### Out of Scope (this iteration)

- **Fork from checkpoint** (Phase 2 Epic 2.1) — depends on checkpoint creation being stable first
- **Adopted bridge improvements** (Phase 2 Epic 2.2) — separate concern, separate iteration
- **Phase 3**: OSC 7 CWD tracking, VT state snapshots, read-only external process discovery
- **Multi-user or shared sessions** — single-user, local only
- **Cloud backup / remote sync** — file system persistence only
- **Non-Claude programs** — checkpoint/resume targets Claude sessions only for this iteration

### Explicitly preserved

- Existing `sessions.json` / `InstanceData` must load without migration — backward-compatible JSON changes only
- claude-mux protocol and socket format unchanged
- No new language runtimes, no new databases, no external services

---

## Constraints

| Constraint | Detail |
|---|---|
| **Backward compatibility** | Existing sessions.json loads without a migration step. New fields use `omitempty`. |
| **CI must stay green** | All new code ships with tests. `make quick-check` passes. Integration tests marked `//go:build integration` if they require tmux; unit tests run unconditionally. |
| **macOS-first, Linux must build** | `session/procinfo/` is darwin-specific via build tags. Linux gets stub implementations that degrade gracefully (return `ErrNotSupported`). |
| **No new external services** | File system + tmux + SQLite only. No Redis, no remote APIs, no additional binaries beyond what already ships. |
| **Go only** | Backend changes in Go. Web UI changes in React/TypeScript. No new language runtimes. |

---

## Context

### Current state (as of 2026-04-15)

| Component | Status | Notes |
|---|---|---|
| `session/procinfo/` | ✅ Built | Process inspector with darwin + stub builds |
| `session/history_detector.go` | ✅ Built | Detects JSONL files open by PID; `tryExtractConversationUUID()` |
| `session/history_watcher.go` | ✅ Built | fsnotify watcher on `~/.claude/projects/` |
| `session/history_linker.go` | ✅ Built | Background correlation service |
| `session/checkpoint.go` | ✅ Built (struct only) | Checkpoint struct exists; service methods may be partial |
| HistoryLinker startup wiring | ❌ Missing | `server/dependencies.go` does not start HistoryLinker |
| `CaptureCurrentState` on shutdown | ❌ Missing | Shutdown path does not capture WorkingDir |
| Cold restore integration test | ❌ Missing | `session/instance_cold_restore_test.go` not written |
| Proto fields populated | ❌ Missing | `history_file_path` / `claude_conversation_uuid` not set in adapter |
| Checkpoint creation service | ❓ Verify | `CreateCheckpoint` method may be a stub |
| Checkpoint RPC endpoints | ❓ Verify | Service handlers may not be implemented |
| Checkpoint web UI | ❌ Missing | No UI for checkpoint creation or listing |

PR #70 (foundational fix) merged to main: `ClaudeProjectDirName` encoding fixed, signal handling refactored to context cancellation.

### Predecessor artifacts

- `project_plans/session-resumption/implementation/plan.md` — detailed task breakdown for Epics 1.1–1.3; still valid reference
- `project_plans/session-resumption/implementation/validation.md` — test coverage map; should be updated post-implementation
- `project_plans/session-resumption/decisions/` — ADR-001 (Session Identity), ADR-002 (Two-Tier Resume), ADR-003 (Checkpoint Storage) — binding decisions, do not revisit

### Stakeholders

- Tyler Stapler (sole developer and user)

---

## Research Dimensions Needed

- [ ] **Stack** — verify current state of each "❓ Verify" item above; check if `CreateCheckpoint` and RPC handlers are stubs or real; confirm what startup wiring exists in `server/dependencies.go`
- [ ] **Features** — survey how competing tools handle checkpoint UX (tmux-resurrect bookmarks, VSCode workspace restore, JetBrains session save) — inform checkpoint label/list UI design
- [ ] **Architecture** — design the missing wiring: where exactly does HistoryLinker start, what shutdown hook model works with the current context-cancellation pattern, how does `CaptureCurrentState` fit into the graceful shutdown sequence
- [ ] **Pitfalls** — known risks: race between HistoryLinker startup scan and session serve; scrollback LatestSequence read while session is writing; cold restore to deleted WorkingDir; gopsutil CGo on CI (already documented in KI-001)
