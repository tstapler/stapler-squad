# Research Synthesis: Session Resumption Hardening

**Date**: 2026-04-15
**Sources**: findings-stack.md, findings-features.md, findings-architecture.md, findings-pitfalls.md

---

## Decision Required

What targeted changes close the remaining session resumption gaps and deliver a working checkpoint creation UI, given that Phase 1 wiring is largely complete but the cold restore path is missing?

---

## Context

PR #70 (just merged) fixed the foundational `ClaudeProjectDirName` encoding bug and signal handling. The follow-on audit revealed that nearly all Phase 1 infrastructure is built and wired — HistoryLinker runs on startup, CaptureCurrentState runs on shutdown, checkpoint creation and RPC handlers exist, and ForkFromCheckpoint (Phase 2) is even partially implemented. The critical gap: when tmux is dead after a reboot or kill, `Instance.start(false)` calls `RestoreWithWorkDir` and errors out. No `--resume` fallback exists. Checkpoints also silently record `scrollbackSeq = 0` because `LatestSequence` was never added to scrollback storage.

---

## What's Already Done (no work needed)

| Component | Status | Evidence |
|---|---|---|
| HistoryLinker startup wiring | ✅ Complete | `server/server.go:98`, `dependencies.go:449-451` |
| CaptureCurrentState on shutdown | ✅ Complete | `server/server.go:117`, 4-second deadline with SaveInstances |
| CreateCheckpoint method | ✅ Complete | `session/instance.go:2053` — captures git SHA, conv UUID, conv line count |
| Checkpoint RPCs (Create/List/Delete) | ✅ Complete | `session_service.go:1699,1735,1767` |
| ForkFromCheckpoint (Phase 2) | ✅ Complete | `session/instance.go:2104`, fork test file exists |
| ClaudeSession.SessionId in proto | ✅ Complete | `instance_adapter.go:81` |
| External session HistoryLinker wiring | ✅ Complete | `dependencies.go:478` — AddInstance called on discovery |

---

## Options Considered

### Option A: Minimal gap closure (recommended)
Close only the 4 identified gaps — cold restore branch, LatestSequence, HistoryFilePath in adapter, cold restore test — then ship the existing checkpoint UI with a simple web component.

### Option B: Full cold restore + checkpoint UI polish
Same as A, plus the JetBrains-style popover UX with optimistic updates, unbounded-list protection (cap at 10), and delete-with-undo toast.

### Option C: Defer cold restore, ship checkpoint UI only
Skip the cold restore fix (it requires integration testing), deliver only the checkpoint creation UI.

---

## Dominant Trade-off

**Correctness vs. speed**: The cold restore gap means "zero conversation loss after restart" is not achievable without the `start(false)` fix. Option C delivers visible UI without closing the core reliability gap. Option A closes the gap with minimal scope. Option B adds UX polish that isn't strictly required for correctness.

---

## Recommendation

**Choose Option A (minimal gap closure) + checkpoint UI from Option B UX pattern.**

Because:
1. Cold restore is a 30-50 LOC change with clear placement — `session/instance.go:808`. The `HasClaudeSession()` and `resolveStartPath()` helpers already exist. The `initTmuxSession()` path already adds `--resume`. This is not risky.
2. LatestSequence is a trivially small addition (20 LOC) that fixes silent data corruption in checkpoint fork operations.
3. The checkpoint UI is the only user-visible new feature — using the JetBrains-style popover (pre-filled label, collapsible list with metadata pills) delivers good UX with minimal component complexity.
4. The integration test for cold restore is the most valuable single test we can add — it would have caught the current gap and any regression.

**Accept these costs**:
- Phase 3 (OSC 7, VT snapshots, read-only discovery) deferred
- Fork UX (Phase 2 Epic 2.1 web UI) deferred — the backend is built but no fork button in the UI
- Adopted bridge improvements deferred

**Reject these alternatives**:
- **Option C (skip cold restore)**: Rejected — shipping checkpoint UI without cold restore means users still lose conversation context on restart, which defeats the primary goal.
- **Adopted bridge / Phase 3 work**: Rejected for this iteration — out of scope per requirements.md.

---

## Implementation Order (feed directly into plan.md)

### Story 1: Cold Restore Fix (P0 — unblocks "zero conversation loss")

**S — session/instance.go:808**
- Add `!i.tmuxManager.DoesSessionExist() && i.HasClaudeSession()` check before `RestoreWithWorkDir`
- On true: call `i.tmuxManager.Start(resolveStartPath)` + `RestoreWithWorkDir` (PTY attach)
- Log: `"Cold restoring '%s' with --resume %s"`
- On false with dead tmux and no UUID: start fresh (log warning)
- Keep existing hot-restore path (`RestoreWithWorkDir` when tmux is alive) unchanged

**M — session/instance_cold_restore_test.go (new)**
- Build tag: `//go:build integration` (requires tmux)
- Test 1: UUID set + dead tmux → new session created with `--resume <uuid>` in program
- Test 2: No UUID + dead tmux → new session created without `--resume`
- Test 3: UUID set + alive tmux → existing session restored (no new session created)

### Story 2: LatestSequence (P1 — fixes silent checkpoint data corruption)

**S — session/scrollback/storage.go**
- Add `LatestSequence(sessionID string) (uint64, error)` — scan entries, return last `.Sequence`
- Consider file lock consistency (existing `getFileLock` pattern)

**XS — server/services/session_service.go:1720**
- Obtain `scrollbackSeq` from scrollback manager before calling `inst.CreateCheckpoint`

**XS — server/services/workspace_service.go:286**
- Same pattern: get real scrollback seq instead of 0

### Story 3: HistoryFilePath in Adapter (P2 — web UI completeness)

**XS — server/adapters/instance_adapter.go:55**
- Add `HistoryFilePath: inst.HistoryFilePath` to the `protoSession` struct literal
- Verify `history_file_path` field exists in types.proto; add if missing

### Story 4: Checkpoint Web UI (P2 — user-visible feature)

**M — web-app/src/components/sessions/CheckpointButton.tsx (new)**
- Bookmark icon button (🔖) on session card action bar
- Popover: single label input pre-filled with `"Checkpoint YYYY-MM-DD HH:MM"`, Create button
- On submit: call `CreateCheckpoint` RPC, optimistic UI update

**S — web-app/src/components/sessions/CheckpointList.tsx (new)**
- Collapsible section below session card
- Per entry: label (bold), relative time, `git:abc1234` pill, `conv:def5678` pill, ✕ delete
- Empty state: "No checkpoints yet — click 🔖 to save your place"
- Cap display at 10 most recent; "Show all (N)" expand

**XS — web-app/src/lib/hooks/useSessionService.ts**
- `createCheckpoint(sessionId, label)` → CreateCheckpoint RPC
- `listCheckpoints(sessionId)` → ListCheckpoints RPC
- `deleteCheckpoint(sessionId, checkpointId)` → DeleteCheckpoint RPC

---

## Open Questions Before Committing

- [ ] Does `history_file_path` proto field exist in `proto/session/v1/types.proto`? If not, `make proto-gen` required after adding it — check before Story 3. Blocks: Story 3 scope.
- [ ] Does `LatestSequence` need to hold the file lock for the entire read, or is last-line read sufficient? Blocks: Story 2 implementation detail (performance vs correctness).
- [ ] Should cold restore integration tests run unconditionally in CI or only in a separate integration job? Blocks: Story 1 test placement.

If these are answered quickly (via code read), no spike needed before writing plan.md.

---

## Sources

- `project_plans/session-resumption-hardening/research/findings-stack.md`
- `project_plans/session-resumption-hardening/research/findings-features.md`
- `project_plans/session-resumption-hardening/research/findings-architecture.md`
- `project_plans/session-resumption-hardening/research/findings-pitfalls.md`
- Direct codebase audit: `server/server.go`, `server/dependencies.go`, `session/instance.go`, `server/adapters/instance_adapter.go`, `server/services/session_service.go`
