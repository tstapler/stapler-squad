# Architecture Review (re-review after repair): files-viewer-revamp
**Date**: 2026-10-16
**Verdict**: CONCERNS (0 blockers, 2 new concerns, 1 new nitpick; all 2 prior blockers and 9 prior concerns RESOLVED, 6 prior nitpicks RESOLVED)

Method: re-read `plan.md` (1118 lines, incl. "Review repair log"), `ADR-031` in full, grepped `ADR-032/034`, and checked code. Scope: only the previously reported findings plus new blockers introduced by the repairs. `docs/adr/ADR-000-architecture-constitution.md` does not exist, so there are no constitution violations.

Code facts re-checked:
- `session/types.go:317-355`: `WorktreeDir` is `""` when the session works directly in `RepoRoot`; `ExistingDir` is `ActiveDir` if it exists, else `RepoRoot`. So `ActiveDir == ExistingDir` is false exactly when the fallback fired. The repaired write gate is correct and does not read raw `i.Path`.
- `server/services/session_service.go:948` builds `NewFileService(workspaceSvc)` with one dependency, so a separate `FileWriter` constructed beside it is a clean extraction.
- `web-app/node_modules` is absent in this worktree, so CodeMirror behavior below is INFERRED from CM6 knowledge, not run.

## Prior findings

### Blockers
- **Conflict payload not a sum type** — RESOLVED. `TheirsVersion = FullText{content, token} | Unavailable{reason}` in glossary, ADR-031, Task 2.1.2a/c. Server sends `Unavailable` and never partial content when the disk file is over 1 MB, binary, non-UTF-8, Mixed EOL, symlink or deleted. ACs added in 2.1.2, 2.3.1 and 2.3.2 (Merge and Overwrite disabled, `merge3` never called). Matches the remediation requested.
- **EOL/BOM ownership** — RESOLVED. Single owner is the server: client sends LF-normalized text, `line_ending`/`has_bom` removed from `WriteFileRequest`, server derives both from the token-verified disk bytes, rejects `\r`/NUL (`InvalidArgument`), `merge3` does not re-apply. AC "no-op save of CRLF+BOM is byte-identical, no `\r\r\n`" is in 2.1.2 and 2.1.4; ADR-031 Encoding bullet updated. See new concern N1 for a gap this design leaves.

### Concerns
- **`WorktreeDir != ""` gate** — RESOLVED. Gate is `ActiveDir != "" && ActiveDir == ExistingDir`; both session shapes have ACs (paused session rejected, directory-mode session saves). Owner-veto path (`NoDedicatedWorktree`) recorded as an Unresolved Question.
- **`.git` denial lexical** — RESOLVED. Task 0.2.1a does per-component `Lstat` for writes plus post-resolution recheck; `linkdir -> .git/hooks` and `gitlink -> .git` are in the 2.1.4 matrix.
- **Token fast path** — RESOLVED. Always hash; mismatch may short-circuit only toward `Conflict`; AC with `os.Chtimes`-pinned same-size rewrite.
- **Newest-wins registry** — RESOLVED. Mutex-guarded map with swap-and-cancel in the lock, `SearchKey{SessionID, ClientID}`, process-wide semaphore (2); ACs for two devices, 50-goroutine race and bounded walks.
- **Primitive obsession** — RESOLVED. `SearchOptions` with `MatchMode/CaseMode/WordMode`, `Scope`, `Limits`, parsed once at the boundary; `FileRef` and `AccessMode`; nested proto messages.
- **`Buffer` model** — RESOLVED. `Loading|Failed|Loaded` union; `dirty` boolean only; dirty implies pinned with a reducer property test; draft text read from `EditorState` on a debounce.
- **`MergeBase` after draft restore** — RESOLVED. `Available|Unavailable`; equal token gives `Available(disk text)`, changed token gives a 2-way flow with Merge disabled and a stated reason; no base persisted.
- **Write-path dependencies on `FileService`** — RESOLVED. Separate `FileWriter` with ports `FilesFlags`, `WriteGuard`, `StatusInvalidator`, `WorkspaceProvider` (Task 2.1.2b, wired in `SessionService`); mutex key `(ActiveDir, ResolvedPath)`, refcounted.
- **`CodeEditor.tsx` / `FilesTab.tsx` growth** — RESOLVED. `extensions/*.ts` modules and hook seams (Task 1.2.2d), 350-line budget asserted by a jest check, edit ordering in the wave plan.
- **Tech Debt Disposition gaps** — RESOLVED. Rows added for `session_service_delegates.go`/`SessionService`, `workspace_service.go`, session ownership (mapped to the hidden-session guard) and `LocalFileBrowser` reachability; new delegates go in `files_delegates.go`.

### Nitpicks
All RESOLVED: `fs.WalkDir(root.FS())`; refcounted mutex registry; test-only `git check-ignore` comment; single reject channel (Connect errors with typed `ReadOnlyReason` detail, explicit code table); bursty 3-message test stream in 0.1.1a; threat model in ADR-031.

## Verification of the named repair areas
- **`FullText|Unavailable`**: sound. One ordering gap, see N2.
- **EOL/BOM server ownership**: sound for LF and CRLF files. One gap, see N1.
- **`FileWriter` extraction with ports**: sound. Ports are consumer-defined, `FileService` constructor unchanged, and Task 2.1.2b is the explicit target for the 0.1.3b recon.
- **`ActiveDir == ExistingDir` write gate**: correct against `session/types.go`. Directory-mode editing makes `.git` a real directory inside the root, which the symlink-safe deny covers.
- **Buffer union**: sound; no new illegal states found. The `Loaded` arm carries `mergeBase` (up to 1 MB per editable buffer, at most 10 on mobile), which is acceptable.

## New findings

### Blockers
None.

### Concerns
- [ ] **N1. Lone `\r` is silently rewritten to `\n` by the client-normalization design (Task 0.2.3b, Story 2.1.4)** — INFERRED, not run (no `node_modules`). CodeMirror's default document split treats `\r\n`, `\r` and `\n` all as line breaks, so a file that is otherwise LF but holds a stray lone `\r` (or old-Mac line endings) loads with those `\r` turned into `\n`. After the first edit anywhere, the save request is LF-clean, the server derives `LF`, and the on-disk bytes change in places the user never touched. The plan's `MixedEol` rule and the corpus (`crlf`, `crlf_bom`, `mixed_eol`, `bom`, `no_trailing_newline`, `exec.sh`) only cover LF vs CRLF mixes. Fix before 0.2.3b: define `detectLineEnding` so any lone `\r` (a `\r` not followed by `\n`) classifies the file `MixedEol` (read-only), and add a `lone_cr.txt` fixture to the 2.1.4 corpus asserting `MixedEol` and unchanged bytes. Cheap, but without it the "byte-identical" guarantee is incomplete.
- [ ] **N2. Deleted-file ordering in the `WriteFile` pipeline (Task 2.1.2c vs Story 2.1.2 / 2.3.1)** — The pipeline runs "`Lstat` not-symlink/not-dir/exists" before the token compare, but the ACs expect a deleted file to yield `Conflict{Unavailable{Deleted}}`. As written, a missing file hits the `Lstat` step first and returns a Connect error (likely `NotFound`) that the client has no mapped `Rejected` reason for. State the order explicitly: for a missing leaf with a valid token, return `Conflict{Unavailable{Deleted}}`; for a missing parent directory, a typed refusal. Add the case to 2.1.2e.

### Nitpicks
- `ReadOnlyReason` (glossary line 32) and `Unavailable.reason` (line 45) overlap but are different enums (`Deleted` exists only in the second; `Truncated`/`FlagOff` only in the first). Reuse one enum with a `Deleted` value, or say in the proto comment why they differ, so the client does not maintain two parallel switches.

## Lens summary
- **Structural**: `FileWriter` extraction and the extension/hook seams remove the two structural weak spots from the first review. No new coupling found.
- **Type-level**: `TheirsVersion`, `MergeBase`, `Buffer`, `AccessMode` and `SearchOptions` close the earlier gaps. N1 is a domain-rule gap (EOL classification), not a type gap.
- **Patterns**: Ports and adapters for the writer, Optimistic Offline Lock with always-hash compare, single `EditorView` with per-tab state. None over-applied.
- **Workspace paths**: Reads use `ExistingDir`, writes use `ActiveDir` with the equality gate, mutex key uses `ActiveDir`; consistent with `instance-lock-free-reads.md`.
- **Build-vs-buy**: unchanged; consistent with `research/build-vs-buy.md`.
