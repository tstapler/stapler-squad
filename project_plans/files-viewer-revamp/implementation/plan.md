# Implementation Plan: files-viewer-revamp

**Feature**: Turn the read-only, single-file, filename-only session Files tab into a Vim/VS Code-style project explorer: content search, multi-file tabs and navigation, safe in-browser editing (conflict diff + 3-way merge), and a phone-first layout.
**Date**: 2026-10-16
**Status**: Ready for implementation (Phase 3, Phase 4, pre-mortem, triad, triad round 2 and triad round 3 repairs applied, and the owner-approved ADR-036 transport and editor-engine integration applied; see "Review repair log" at the end)
**Size**: 14 epics, 65 stories, 196 tasks (recounted by script after the ADR-036 integration; 27.71M CU, band XL; the base plan before ADR-036 was 12 epics, 47 stories, 133 tasks, 18.04M CU; the contingent custom editor engine, Epic 1.7, is costed separately and not in this total: 6 stories, 26 tasks, 4.59M CU)
**Flag naming**: earlier drafts (requirements.md, research) said `files_editing` / `files_viewer_v2`; this plan uses `files:editing` / `files:viewer-v2` to match the newer `area:name` flags (e.g. `programs:cli-flag-probe`). Rename only, not drift; `files:editing` / `files:viewer-v2` is the one spelling in every artifact (the `knownFeatureFlags` convention). Both flags are **global-only** (see Risk Control).
**ADRs**: ADR-030 (content search), ADR-031 (WriteFile safety), ADR-032 (buffer/tab state), ADR-033 (vim and input profile), ADR-034 (3-way merge), ADR-035 (analytics taxonomy, privacy rules, typed helper), ADR-036 (pluggable range-read and delta-write file transport, with the editor-engine port; Accepted 2026-10-16) in `project_plans/files-viewer-revamp/decisions/`

---

## Step 0.5: Alternatives considered (CREATIVE pass)

| Approach | Key strength | Key weakness | Verdict |
|---|---|---|---|
| A. Extend the existing Files tab behind seams: `useOpenBuffers` hook, reusable `CodeEditor`, new server files beside `file_service.go` | Reuses CodeMirror 6, react-arborist, Connect plumbing, the existing unit/e2e regression net; read-only value ships first and editing is flag-gated | `FilesTab`/`FileContentViewer` are already large; needs disciplined seams so they do not grow | **Chosen** |
| B. Embed code-server / openvscode / Monaco per session | Full feature set (search, vim, merge) for free | Not touch-first, hundreds of MB per workspace, breaks session identity/flags/git badges, contradicts "no IDE" scope | Rejected |
| C. Rewrite as a new `/files-v2` page with its own state and RPCs, leaving the old tab | No regression risk to the old tab | Duplicates tree/viewer (jscpd gate, component forks), two UIs to maintain, delays value | Rejected |

---

## Domain Glossary
*(Exact names are used in code, tests and comments.)*

| Term | Definition | Notes |
|---|---|---|
| `SessionId` | Existing id of a session whose worktree is being browsed | Existing string; server-side params travel in `FileRef{SessionID, Path}` / `SearchOptions` arg structs, never as adjacent bare strings |
| `WorktreeRoot` | Absolute directory a request is confined to, wrapping an `*os.Root` | Reads use `Workspace().ExistingDir`; writes use `Workspace().ActiveDir` and require `ActiveDir != ""` and `ActiveDir == ExistingDir` (directory exists, so no repo-root fallback); `WorktreeDir` may be `""` (directory-mode / existing-worktree sessions are editable) |
| `ResolvedPath` | A client path that passed `resolveInWorktree`: relative, canonical, no `..`/NUL, no `.git` component before or after symlink resolution, not an escaping symlink | Newtype; returned canonical to the client, which keys buffers by it |
| `FileVersionToken` | Opaque `v1:<size>:<mtime_ns>:<sha256hex>`; hash is `-` for reads that are not editable (truncated/binary/over cap, and, under ADR-036, range reads whose whole-file `Analyze` pass is still pending; the format is unchanged). **The sha256 segment is the identity; size and mtime are hints** (an mtime-only difference with equal hash is "no change"; ADR-031) | Newtype; `Parse` accepts a well-formed `-` token as `NonEditable` and `WriteFile` refuses it with `FailedPrecondition` (`Truncated`; `Binary`/`TooLarge` per the live classification of the file); only an unparseable string is `InvalidArgument` |
| `LineEnding` | Enum `LF` / `CRLF` / `Mixed` detected on read | Sum type; `Mixed` is read-only. `Mixed` includes any file with both CRLF and bare LF **and** a lone `\r` (not followed by `\n`) in an otherwise-LF file, so it maps to `ReadOnlyReason=MixedEol` |
| `ReadOnlyReason` | Enum `None` / `FlagOff` / `Binary` / `Truncated` / `TooLarge` / `NonUtf8` / `MixedEol` / `Symlink` / `WorktreeMissing` / `HiddenSession` / `GitInternal` | Proto enum + TS union; drives the "why can't I edit" chip. `WorktreeMissing` = the session's active directory no longer exists (paused/cleaned up), never "has no dedicated worktree". **One enum for both uses**: also carries `Deleted` (file gone from disk; used only inside `Unavailable`). `Unavailable.reason` is this same `ReadOnlyReason` type restricted to the subset `{Deleted, TooLarge, Binary, NonUtf8, MixedEol, Symlink}`; the server never emits another value there and the client treats any other as a bug (exhaustive switch default asserts) |
| `ContentSearcher` | Go interface: `Search(ctx, root, SearchQuery, emit) (SearchStats, error)` | Domain-owned; in-process impl now, `rg` impl optional later |
| `SearchOptions` | Value object built once at the proto boundary: `Pattern`, `Match MatchMode{Literal,Regex}`, `Case CaseMode{Insensitive,Sensitive}`, `Word WordMode{Any,Whole}`, `Scope{IncludeGlobs, ExcludeGlobs, IncludeIgnored}`, `Limits{MaxResults, ContextLines}` | `NewSearchOptions(proto)` validates (length cap, RE2 compile); no bool/string piles below the boundary. `SearchQuery` in other places means this type |
| `SearchMatch` | One hit: path, 1-based line, line text (clamped), match ranges in UTF-16 code units | UTF-16 because CodeMirror positions are UTF-16 |
| `SearchBatch` | One streamed message: grouped `FileMatches` | Final message carries `SearchStats` |
| `SearchStats` | `files_scanned`, `binary_skipped`, `truncated`, `duration_ms` | Logged without query text. No `root_is_fallback`: search never runs against the repo-root fallback (Story 1.1.2) |
| `Buffer` | Open file in the client, a discriminated union: `Loading{sessionId, path}` / `Failed{sessionId, path, error}` / `Loaded{sessionId, path, baseToken, mergeBase, dirty, mode, ViewState, lastAccess, kind}`; **`sessionId` is captured when the buffer is opened and never re-read from props** (a save, stat or draft write uses `buffer.sessionId`) | Reducer state; one `EditorState` per buffer, one mounted `EditorView`. The reducer holds `dirty: boolean` only; draft text is read from the `EditorState` by `draftStore` on a debounce (no per-keystroke string copy through React state) |
| `BufferKind` | `preview` (replaced by next single-click) or `pinned` | Sum type. **Entering Edit pins the buffer** (triad round 3: not the first keystroke, so a clean buffer in Edit cannot be evicted or replaced, U-EDIT-9/17); invariant: a dirty buffer is always `pinned`; `open` of another file never replaces a pinned or dirty buffer |
| `ViewState` | Per-buffer scrollSnapshot + selection | Persisted in localStorage; drafts bounded separately |
| `OpenBuffers` | Reducer state `{buffers, activeIndex, history, historyPos}` owned by `useOpenBuffers` | Replaces `selectedPath` + `recentPaths` as the source of truth |
| `NavEntry` | One jumplist record `{path, line, col}` | Back/forward steps NavEntries, not tab order |
| `EditorMode` | `Read` / `Edit`, **per buffer** (the `mode` field of `Loaded`) | Read by default on open and reload; not persisted across reload; each tab keeps its own mode; `Edit` is representable only when `ReadOnlyReason=None` and `files:editing` is on, so an uneditable file shows Edit as a disabled control with the reason (ux.md 8.1), not as a mode |
| `SaveOutcome` | `Saved(newToken)` / `Conflict(TheirsVersion)` / `Rejected(reason)` / `Unknown` | Client sum type with exhaustive handling. Wire shape: `WriteFileResponse` oneof has only `Saved` and `Conflict`; every refusal is a Connect error carrying a typed `ReadOnlyReason` detail, which the client maps to `Rejected(reason)`. `Unknown` (lost response) forces a re-stat before next save |
| `TheirsVersion` | Wire sum type `FullText{content (LF-normalized, BOM stripped), token}` / `Unavailable{reason: ReadOnlyReason, restricted to Deleted\|TooLarge\|Binary\|NonUtf8\|MixedEol\|Symlink}` | Carried by `Conflict`. `Unavailable` means the disk file cannot be shown or safely overwritten: Merge and Overwrite are disabled (never truncate `theirs`) |
| `ConflictResolution` | `Overwrite` / `Reload` / `KeepEditing` / `Merge` | Exhaustive switch in `ConflictDialog` |
| `MergeBase` | `Available(text)` / `Unavailable` | `Available` = LF-normalized content of the version the buffer was opened from, in memory only, editable (<= 1 MB) buffers. `Unavailable` after a draft restore whose token differs from disk; the conflict flow is then 2-way only (Merge disabled with reason) |
| `MergeResult` | `{text, conflicts[], autoMergedHunks[]}` from `merge3(base, mine, theirs)`; each auto-merged hunk carries `nearTheirs` (overlaps or lies within 1 line of a theirs hunk) | Pure function, property-tested; `nearTheirs` drives the mandatory review step (Story 2.3.2) |
| `Draft` | Unsaved dirty edits persisted per (session, path, baseToken) behind one `DraftBackend` interface; **since ADR-036 the primary form is the composed pending ChangeSet (`ChangeSet.toJSON`) plus selection, so the size caps below bound edit volume, not file size; a text snapshot is kept only for an engine that cannot produce a ChangeSet**: localStorage tier <= 256 KB, **IndexedDB tier for 256 KB up to the 1 MB edit cap (max 5 entries; triad round 3)**; 24 h TTL; **no base text stored** | See ADR-032 and ux.md 11.3; when neither tier can hold a dirty buffer the priority-1 "Draft not stored" banner shows. On restore: token == disk -> `MergeBase = Available(disk text now)` (disk equals the original base); token != disk -> `MergeBase = Unavailable`, 2-way conflict flow |
| `PaneState` | Mobile nav-stack: `files` / `editor` / `search` | Existing `mobilePane` generalised |
| `FilesLayout` / `chooseFilesLayout` | `mobile` / `desktop`, chosen by the pure function `chooseFilesLayout({containerWidth, pointerCoarse, screenShortSide})`: `mobile` when `containerWidth < 768` **or** (`pointerCoarse` and `screenShortSide < 600`), else `desktop` | Triad round 3 (ux.md section 0): a landscape phone (844x390) keeps the mobile layout and the 7.1a ladder, a tablet (short side >= 600) gets the desktop layout, and `screen` (not the visual viewport) makes the answer stable when the keyboard opens; Task 1.5.1c |
| `InputProfile` | `HardwareKeyboard` / `Touch` | Gates vim default (read **only at Edit entry**, never flipped live mid-edit) and ExtraKeyBar. Resolution order (ADR-033, ux.md section 8.8): persisted manual override (Auto/Touch/Hardware) > conservative auto-detect over **trusted** events only (primary `(pointer: fine) and (hover: hover)`; or a Ctrl/Meta chord; or Escape/F-key/arrow/paging key with a non-empty `code`; or three `Key<L>` letters whose `code` matches `key`; Enter/Backspace/Space/Tab/digits never count; the flip is session-only) > `Touch` (the default). A persisted explicit vim toggle always wins; vim is never auto-enabled on a touch-resolved device |
| `AccessibleView` | Plain, fully rendered, read-only paged view of the active buffer (<= 500 lines per page, line numbers as text, pager, Go to line), toggled per device (Overflow, palette, skip link) | The guaranteed screen-reader read path; CM6 virtualizes so its DOM cannot be assumed reachable (ux.md 8.10, Stories 0.1.6 and 1.5.4) |
| `EditLayoutBudget` | Pure function `editLayoutBudget(viewport, keyboard, inputProfile, flags)` returning which rows show in Edit mode (top bar, 44 px banner row, key row, vim row, status bar, fixed Save/Stop cluster) so the editor keeps >= 120 px (hard floor 64 px); `inputProfile` is the resolved `InputProfile` plus "Show key bar", because the key row and the cluster exist only while the key bar is shown (a Hardware profile never auto-hides the top bar) | ux.md 7.1a ladder; Task 2.4.2c |
| `Reachability` | `Online` / `Offline` (browser says so) / `Unreachable` (two consecutive RPC or heartbeat failures while the browser reports online) | Disables Save with a visible reason; clears on the next successful RPC or `online` event (ux.md 8.11; Task 2.2.3a) |
| Buffer display states | `clean` / `dirty` / `stale` / `conflict` / `offline-dirty` (ux.md section 2) | Derived, not stored: `dirty` is the reducer boolean; `stale` comes from `useBufferFreshness`; `conflict` is local `ConflictDialog` state; `offline-dirty` is `dirty && !useOnlineStatus()`. No extra reducer fields |
| `ExtraKeyBar` | Row above soft keyboard: Esc, Tab, sticky Ctrl/Alt, arrows, brackets, undo/redo, Save | Reuses terminal supplemental-key styling |
| `StickyModifier` | One-shot (tap) or locked (double-tap) Ctrl/Alt | Visible state |
| `QuickfixCursor` | Index into the last search results for next/prev hit stepping | Survives opening a hit |
| `FilesEditingFlag` | Feature flag `files:editing`, default off, **global-only** | Enforced server-side in `WriteFile` |
| `FilesViewerV2Flag` | Feature flag `files:viewer-v2`, default on (kill switch), **global-only** | Gates `SearchContent` and the new UI |
| `FilesMergePhoneFlag` | Feature flag `files:merge-phone`, default on, **global-only** | Per-form-factor kill switch: when off (or the container is < 360px), Merge on a phone degrades to the 2-way conflict flow; desktop merge unaffected (ux.md section 10) |
| `FilesEvent` / `FilesAnalytics` | Typed registry of `files.*` analytics events (name, category, enum-bounded label types) and the `useFilesAnalytics()` helper that is the only code allowed to call `track` with a `files.` name | ADR-035. Label values are string-literal unions or bucket functions; there is no `string`-typed label. Stamps `form`, `input`, `editing`, `v2` on every event |
| `FindFlowMeter` | Client counter of user interactions from opening search to opening a hit (requirements.md counting protocol), emitted as the `interactions` bucket on `files.search.open_hit` | Backs the find-and-open usage metric; the Playwright test stays the ship gate |
| `FilesTelemetry` | Server port (`RecordWrite`, `RecordSearch`) taking enum-typed outcome structs with no path, query or glob field; adapter maps to `analytics.AnalyticsProvider.Record` | Injected into `FileWriter` and the search handler; no-op default; complements the `slog` lines |
| `FileWriter` | Server component (own file/constructor) owning the write path; ports `FilesFlags`, `WriteGuard`, `StatusInvalidator`, `WorkspaceProvider`, `FilesTelemetry` | `FileService` stays read-only |
| `SearchRegistry` | Mutex-guarded `map[SearchKey]cancelFn` with swap-and-cancel inside the lock; `SearchKey{SessionID, ClientID}` | Newest search wins per client, never across devices |
| `IgnoreMatcher` | Streaming, ctx-aware gitignore matcher confined to the `os.Root`; loads each directory's `.gitignore` as the walker enters that directory (Task 1.1.1e) | Replaces the `collectAllGitignorePatterns` pre-pass for search; `SearchFiles` keeps its own call |
| `SessionBufferHold` | Tab-lifetime in-memory map `(sessionId, path) -> {text, baseToken}` of dirty buffers whose `FilesTab` unmounted (session switch); no size cap beyond the 1 MB editable cap (T0 text; for a windowed T1 buffer the hold stores the composed ChangeSet, ADR-036), max 5 entries | Complements the 256 KB localStorage `Draft`; consumed by the restore prompt (Task 2.2.2c) |
| `FileTransport` | Client port (TS) the buffer, save and freshness hooks use instead of the generated Connect client: `stat`, `read` (all / lines / bytes), optional `readStream`, optional `patch`, `writeFull`; adapters `full`, `connect_unary`, `connect_stream`, `ws_stream`, `http_range` | ADR-036; Story 1.2.3. Adapters translate bytes and framing only, no safety logic |
| `FileTransportRouter` | Resolves each op (`read`, `patch`, `stat`, `writeFull`) independently from `GetFileTransportCapabilities`; ladder `ws_stream` > `connect_stream` > `connect_unary` > `full`; three consecutive transport-class failures demote one rung for the page lifetime, application errors never do | Pins `full` on `Unimplemented` or a newer server major; refetches on a changed `caps_epoch` |
| `FileOps` / `FileBackend` | Server application service holding every file policy (`resolveInWorktree`/`os.Root`, `.git` and symlink denial, flags, hidden-session guard, per-path mutex, token check, idempotency, limits, audit) over a storage port (`osRootBackend`, in-memory test backend) | Story 0.2.6. `FileWriter` (Story 2.1.2) is its write-side collaborator and consumes its policy and backend |
| `FileTransportCapabilities` | Reply of the new read-only RPC: `protocol_version`, `features`, `ops`, per-listener `wires`, `limits`, `caps_epoch` | `:8543` and `:8444` differ truthfully; flag flips bump `caps_epoch` |
| Size tier `T0` / `T1` / `T2` | T0 up to 1 MiB (whole-file load, full or delta save); T1 over 1 MiB up to `hash_cap_bytes` (proposed 64 MiB; windowed read, delta-only edit behind `files:edit-large`); T2 above that up to the read ceiling (proposed 1 GiB; windowed, view-only, `ReadOnlyReason=TooLarge`) | Constants confirmed by S-T2 |
| `Buffer.source` | `Full` or `Windowed{windowStartLine, lineCount?, analysis}` on `Loaded` | Persisted tab state stores `(path, anchorLine)` only, never chunk text |
| `EditorEngine` / `Highlighter` / `MergeSurface` | Ports the Files tab depends on instead of CodeMirror: lazy `VirtualDoc` source, `onNeedRange/provideRange/evictRange`, `onEdits/takePending/ack` in absolute line/col_utf16, `features{vim, mergeView, lazyDoc, irc}`; `CmEngine` is the shipping adapter; `runEngineConformance` gates any engine | ADR-036 decisions 19-20; Story 1.2.1 (ports), Story 1.6.4 (`cm6LazyDoc` gap-line module), contingent Epic 1.7 (custom engine) |
| `Edit` / `ApplyFilePatch` | Ordered, non-overlapping splice `{start{line,col_utf16}, end{...}, insert, deleted_crc32c}` in **base-document coordinates**, sent with `base_token`, `client_id`, `seq`, `expect_new_size` and checksums; applied or refused atomically | Server converts line/col to bytes and owns EOL/BOM; deltas are never rebased, documents are (ADR-034 amended) |
| `PatchRejectReason` | Typed refusal detail beside `ReadOnlyReason`: `OffsetMismatch`, `ResultMismatch`, `OutOfOrder`, `AnalysisPending`, `PatchTooLarge` | Task 2.1.5a |
| `files:range-read` / `files:delta-write` / `files:edit-large` | Global-only, live-settable flags (no scopes), **all default off** (rollout order: range-read after Task 1.6.7c, delta-write after Gate G2, edit-large after Gate G3) | Task 0.2.6 / 0.2.2a; the defaults are an owner-flagged choice recorded in ADR-036 |

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|---|---|---|---|---|
| Read-side additions (`SearchContent`, `StatFile`) | Service Layer: methods on existing `FileService`, logic in new files | Fowler PoEAA | Transaction Script inside `file_service.go` | `file_service.go` is 889 lines and gocognit/funlen gates apply to modified functions |
| Write path (`WriteFile`) | Separate `FileWriter` (own file + constructor) with consumer-defined ports `FilesFlags`, `WriteGuard`, `StatusInvalidator`, `WorkspaceProvider`; wired in `SessionService`; delegate in new `files_delegates.go` | Ports and adapters | Methods on `FileService` | `FileService` has one dependency (`WorkspaceProvider`, `workspace_service.go:30-32`); the write path needs flag, guard, VCS invalidator and a mutex registry, which would widen it into an interface-pollution magnet |
| Search engine | Strategy behind `ContentSearcher` (in-process default, `rg` optional later) | GoF | Direct `exec.Command("rg")` | `rg` not guaranteed on every host; repo prefers in-process; ignore semantics must match tree |
| Search transport | Server-streaming RPC with final stats message | Connect | Unary with cap | First-result < 2 s only meetable with streaming; unary cannot become streaming without a breaking RPC |
| Path confinement | Single `resolveInWorktree` returning `ResolvedPath` over `os.Root` | Parse-don't-validate | Per-handler `HasPrefix` checks | Lexical check is the classic bug; one helper for read/write/search |
| Atomic save | Template: temp-in-same-dir (rename must stay on one filesystem), fsync, chmod, rename, dir fsync | Pragmatic (precedent `settings_file_atomic_write.go`) | In-place truncate+write; `renameio` dep | Torn reads by agents; hand-rolled matches the repo precedent and avoids a dependency |
| Concurrency | Optimistic concurrency token (always sha256-compared) + per-path mutex keyed by `(ActiveDir, ResolvedPath)`, refcounted | PoEAA (Optimistic Offline Lock) | mtime-only; git blob id; file locks | mtime misses same-tick edits; agents ignore locks; a relative-path-only key would serialize two sessions editing `README.md` against each other |
| `FileVersionToken`, `ResolvedPath`, `WorktreeRoot` | Newtypes | type-driven-design | raw `string` | Prevent passing an unvalidated path or a truncated-read token to a write |
| `ReadOnlyReason`, `LineEnding`, `SaveOutcome`, `ConflictResolution`, `BufferKind`, `EditorMode`, `PaneState`, `InputProfile` | Sum types / proto enums / TS discriminated unions with exhaustive switch | type-driven-design | booleans (`isReadOnly`, `isPreview`) | Compiler-enforced handling, one place that says why editing is disabled |
| Client buffer state | Reducer + hook (`useOpenBuffers`), pure `bufferReducer`; `Buffer` is `Loading|Failed|Loaded` union with `dirty` boolean (text lives in `EditorState`) | PoEAA Service Layer on client | Global store / React context; flat record with `loaded`/`draft` fields | Single owner (BFT decision); illegal states unrepresentable; no per-keystroke 1 MB string copies through React |
| Editor | One `EditorView`, `view.setState()` per tab | CM6 documented pattern | One view per tab | Phone memory (M5); composition safety (M3) |
| `CodeEditor` read/edit/vim/merge toggles | Compartments (GoF Strategy by configuration) | CM6 | Re-create view on mode change | Re-creating during composition cancels IME |
| `FilesTab` / `CodeEditor` | Facade / composition root over hooks and extension modules (`useFilesShortcuts`, `useFilesDeepLink`, `extensions/{degrade,searchHighlight,vim,merge,readOnlyFacet}.ts`), line budget 350 each | GoF | Keep inline state / effects | Nine stories touch these two files; the seams stop them becoming the new `FileContentViewer` |
| Mobile vs desktop layout | One state model, two view components | UX research | Responsive CSS only | Keeps e2e symmetric, avoids hidden-DOM duplication |
| Conflict + merge | Strategy per `ConflictResolution` | GoF | Single modal with ad-hoc branches | Exhaustive handling |
| File transport | Ports and adapters: client `FileTransport` + `FileTransportRouter`, server `FileOps` over `FileBackend`; wire handlers are thin | Hexagonal (ADR-036) | Safety logic per wire; one fixed RPC set | A new adapter cannot weaken ADR-031; adapters are swappable, measurable and demotable |
| Editor engine | Port `EditorEngine` + `Highlighter` + `MergeSurface`; CodeMirror 6 as `CmEngine` with a lazy gap-line document | Hexagonal / Strategy (ADR-036 decisions 19-20) | CM6 types in UI code; Monaco; custom engine now | Laziness is a data-layer property and CM6 already provides IME, a11y hooks, vim and merge; custom engine only if the S-T3 phone gate fails |
| Delta writes | Base-coordinate line/col_utf16 splice ops, stop-and-wait client, server streaming splice, layered verification, replay store; explicit Save only | ADR-036 decisions 8-13 | OT/CRDT, byte offsets, in-place patching, append journal, server piece table | One human writer, agents overwrite whole files, so reconciliation stays the 3-way merge |

---

## Tech Debt Disposition

| Area | Existing Issue | Disposition | Justification |
|---|---|---|---|
| `server/services/file_service.go` (889 lines) | One file holds all file RPCs; `resolveAndValidatePath` (`:106-116`) is lexical-only (symlinks readable today); three handlers repeat the session/path/workspace preamble | **Isolate via seam** + **Refactor-first** for the resolver | New code in `content_search.go`, `file_write.go`, `file_codec.go`, `path_resolver.go`; swap resolver and preamble in existing handlers as one separate commit (Story 0.2.1); no relocation of unrelated code |
| `GetFileContent` (~140 lines) | Gocognit/funlen risk; mixes truncation, binary sniff, encoding | **Isolate via seam** | Extract helpers into `file_codec.go`; additive response fields only; do not grow the function |
| `FilesTab.tsx` (296) | Single `selectedPath`, MRU, palette, mobile pane inline | **Isolate via seam** | `useOpenBuffers` hook (Story 1.2.2) makes it a thin composition root |
| `FileContentViewer.tsx` (693) | `CodeMirrorViewer` ~300-line effect-driven component mixed with media viewers | **Refactor-first (narrow)** | Extract `CodeEditor` (Story 1.2.1) as its own commit before any edit/vim/merge work; leave image/PDF/video branches alone |
| `FileTree.tsx` (970) | Large, but this feature needs only `revealPath` | **Extend as-is** | No new violation added; separate debt, not in this plan |
| Mobile layout / `mobilePane` | Two-pane switching with known pitfalls | **Extend** | Reuse `ViewportProvider`/`--viewport-height`; no second viewport listener |
| `LocalFileBrowser` + `local_file_service.go` | Unconfined absolute-path HTTP model | **Extend as-is (untouched)** | Opposite security model; not merged (features.md section 5); retirement is an owner question, non-blocking. It is a sibling of the new write RPC but is not reachable from the new editing UI (no link, no shared component path); stated in ADR-031 |
| `session_service_delegates.go` (973 lines) and the ~160-RPC `SessionService` in `session.proto` | Shared choke points the plan adds RPCs to | **Extend as-is, one delegate file per feature** | New delegates go in `files_delegates.go` (search, stat, write) so the 973-line file does not grow; proto edits are sequenced one commit each. No split of `SessionService` here (separate debt) |
| `workspace_service.go` (`vcsStatusCache`: field `:82`, load `:160`, store `:225`, the one existing `Delete` `:447`) | Status cache lives here and in `session/git_worktree_manager.go`, not in `FileService`; only `SwitchWorkspace` evicts an entry today (VERIFIED), so no invalidation entry point exists for a save | **Extend with one method** | `InvalidateVCSStatus(workDir)` (a `vcsStatusCache.Delete`) is added to `workspace_service.go` in Task 2.1.3a; `FileWriter` reaches it only through the `StatusInvalidator` port (Task 0.1.3b confirms nothing richer exists on `Instance`) |
| Session ownership NFR (requirements Security: "respect session ownership") | No per-owner model exists in this single-user tool | **Map to existing guard** | Ownership == the hidden-session write guard + authenticated access (authorization equals terminal write access via `WriteToSession`, so no escalation); recorded in ADR-031; requirements coverage row added |
| `hardSkipDirs` (`build`, `dist`, `vendor`) hides searchable dirs | Search blind spot | **Extend as-is** + `IncludeIgnored` toggle | `.git` stays hard-denied always |
| `ServeFileRaw` (`server/services/file_service.go`, streams media with `http.ServeContent`) | A GET route that returns file bytes; whether it passes the Host/Origin rebinding verdict on the unauthenticated `:8543` listener is UNVERIFIED (reads are not in `guardedProcedures`) | **Audit in Task 0.1.3b; guard in Task 2.6.3a** | ADR-036 decision 16: any GET route returning file bytes must pass the rebinding verdict or not ship; the audit result is recorded before the `http_range` adapter is built |

---

## Migration Plan
None. No schema or stored-data changes. Proto changes are additive (new RPCs, new response fields). Rollback needs no data steps.

## Observability Plan
- **Logs**: one `slog` Info line per `SearchContent` (`session`, `query_len`, `duration_ms`, `results`, `files_scanned`, `truncated`, `backend`) and per `WriteFile` (`session`, `path`, `bytes`, `outcome` = `ok|conflict_rejected|denied|disabled|error`). Never file contents or query text at Info. Error paths log error plus context.
- **Metrics**: `files_search_duration_seconds` (histogram), `files_search_total{outcome}`, `files_write_duration_seconds`, `files_write_total{outcome}`, `files_stat_total`, via the existing metrics pattern identified in Task 0.1.3b.
- **Analytics (long-term usage understanding, ADR-035)**: client `files.*` events through the typed `useFilesAnalytics()` helper into the existing analytics system (`POST /api/analytics` -> `analytics.db`), plus server `files.server.write|search|search_first_result` via the `FilesTelemetry` port. Labels are enums, booleans and buckets only; never paths, file names, contents or query text (privacy test with canary strings, Story 0.2.5). Viewed at `/analytics/files` and via `GET /api/analytics/files` (Story 2.5.3); success metrics are derived queries over these events (ADR-035 section 5).
- **Transport (ADR-036)**: one `slog` line per `ReadFileRange` (`session`, `path`, `kind`, `bytes`, `lines`, `duration_ms`, `outcome`, `adapter`) and per `ApplyFilePatch` (`session`, `path`, `edits`, `bytes_inserted`, `bytes_deleted`, `size_before`, `size_after`, `outcome` = `saved|conflict|offset_mismatch|result_mismatch|rejected`, `adapter`, `duration_ms`), never content; metrics `files_range_duration_seconds`, `files_range_total{outcome}`, `files_patch_total{outcome}`, `files_analyze_duration_seconds` via the Task 0.1.3b pattern; analytics labels `transport`, `fallback_reason`, `payload`, `ops`, `tier` and events `files.transport.select|fallback`, `files.read.window`, `files.edit.patch_rejected`, server `files.server.patch` (ADR-035 "Transport extension"; no paths, offsets, lines or text).
- **Alerts**: no new alerts required (single-user tool).

## Risk Control
- **Feature flags** (live-settable via `GetFeatureFlags`/`UpdateFeatureFlag`, **global only**, no env vars; names follow the newer `area:name` style). VERIFIED: per-scope overrides exist only for hidden-session kinds (`gateFlagScopes()` at `server/services/feature_flag_mutation.go:30-36` is built from `deliverygate.ScopableKinds` = review/diagnose/other, `server/deliverygate/flagcache.go:27-33`; `GetFeatureFlagScoped` at `config/config.go:1915-1925` takes that same key). A repo- or session-scoped override would need a new scope model and is not built here. Owner-visible deviation from requirements.md ("global + per-scope override"); requirements.md updated.
  - `files:editing`, default **off**. Enforced server-side in `WriteFile` and read by the client to hide Edit. Because it is global, flipping it on enables editing for every session on that instance; the server-side guards (hidden-session guard, `.git` deny, token check) are what bound the blast radius, and the flip is a deliberate owner action after Story 2.5.2.
  - `files:viewer-v2`, default **on** (kill switch for `SearchContent`, tabs, search UI).
  - **ADR-036 transport flags (all global-only, no scopes, default off; the defaults are an owner-flagged choice)**: `files:range-read` (range reads, windowed viewer, large-file search; flip after Task 1.6.7c), `files:delta-write` (`ApplyFilePatch`; flip after Gate G2 = Task 2.5.4a signed, then the Task 2.5.5b T0 steps), `files:edit-large` (T1 delta-only editing of files over 1 MiB; flip after Gate G3 = the Task 2.5.5b T1 steps). Each is a kill switch: `files:delta-write` off routes saves to `writeFull` (T0) and makes T1 buffers read-only with their ChangeSet kept as a draft; `files:range-read` off returns the base-plan truncated read-only behavior; a per-adapter kill is a boolean in the same family (the flag RPC's verified shape is boolean). The server enforces each in `FileOps`, and the capability RPC advertises only what is on.
  - `files:merge-phone`, default **on**, global-only (per-form-factor kill switch for the phone 3-way merge view; off, or a container < 360px, degrades Merge to the 2-way conflict flow on phones only). Added by the triad repair; the owner dogfoods the phone merge (Task 2.5.4a) before the `files:editing` flip and may turn this off if it is not good enough.
- **Rollback**: flip `files:editing` off (editor drops to read-only without losing the buffer); flip `files:viewer-v2` off (falls back to the legacy single-file layout); flip `files:delta-write` / `files:edit-large` / `files:range-read` off (ADR-036; saves and reads return to the whole-file path, drafts are kept); otherwise standard revert commit. No data steps.
- **Kill-switch retirement**: the legacy single-file layout path (Task 1.2.2c) and the `files:viewer-v2` flag are removed in the first release after the flag has been on for 4 weeks with no rollback, tracked as a backlog follow-up created in Story 2.5.1. Until then the flag-off path is covered by one smoke test only (open file, view content), not the full matrix.
- **Staged rollout**: read-only epics (1.1-1.5) ship first; editing (2.x) ships dark behind the global `files:editing`. Rollout = after the Story 2.5.2 sign-off on the isolated manual instance, the owner flips the global flag on the live instance. No per-scope step exists.

### Accepted risks (pre-mortem P3 and deferred P2; each cites `implementation/pre-mortem.md`)
- **Symlink/`.git` TOCTOU (#14, P3)**: Task 0.2.1a `Lstat`s each intermediate component and then opens through `os.Root`; an agent could swap a directory for a symlink to `.git/hooks` between the check and the open. `os.Root` still confines to the root, and authorization equals terminal write access (no escalation). Accepted; recorded in ADR-031. Hardening (decide `.git` on the opened parent handle via `fstat` dev/ino) is a follow-up if the owner wants it.
- **Rename-based save drops file identity (#15, P3)**: hard links, xattrs/ACLs/SELinux labels and non-owner ownership are lost on save; file watchers see create+rename; a crash leaves `.x.ssq-tmp-*` an agent's `git add -A` can commit. Accepted and listed in ADR-031 and the how-to (Task 2.5.1b); refusing `nlink > 1` is a one-line follow-up.
- **Mis-tap writes into the main checkout (#16, P3)**: directory-mode editing stays allowed by default with the "Editing the main checkout" chip (Unresolved Questions). The owner is asked to decide the veto before Story 2.1.2 ships rather than at Story 2.5.2; the fallback is one reason value (`NoDedicatedWorktree`).
- **Agent overwrites a saved edit; client does not learn (#7, P2, PARTLY MITIGATED by the triad repair)**: the dedicated "Overwritten by another process / Re-apply my version" banner is still not built. The triad repair adds a 10 s visible-tab `StatFile` poll for the **active** buffer (Story 1.3.4, ux.md section 8.7), so an agent overwrite of a just-saved (now clean) active buffer shows the ordinary "Changed on disk - Reload" banner within 10 s; non-active tabs are checked on activation; the how-to documents both directions of the agent race (Task 2.5.1b). Revisit after first use.
- **Real-device keyboard and viewport behavior** (#10 residual): the Story 0.1.5 spike and Task 1.5.3c reduce but do not eliminate it; the final owner check stays at Story 2.5.2.
- **Delta-write silent mangling (ADR-036; P1 risk, controlled not accepted away)**: a client encoder bug can produce a valid but wrong edit whose client-computed checksums derive from the same wrong position. Controls: the S-T4 prototype and the Task 2.2.5a/2.2.5c property tests against the independent full-replace path, shared Go/TS golden vectors, `FuzzApplyPatch`, the layered server verification before rename (base sha256, per-edit `deleted_crc32c`, resulting size, region CRCs, T0 `expect_result_sha256`), the periodic `StatFile{want_sha256}` reconcile with `Diverged` forcing `writeFull`, explicit Save only, and the `files:delta-write` kill switch. Residual: a T1 file has no whole-document end-to-end hash from the client (it holds only windows), so T1 relies on the size and region checks; `files:edit-large` therefore ships last and separately.
- **Unknown outcome after a server restart in T1 (ADR-036 decision 12)**: the replay store dies with the process and a T1 client knows only `expect_new_size`; the documented resolution is Reload (check the file) with the ChangeSet kept as a draft. Accepted.
- **Scroll/gap interplay inside CM6 on iOS (S-T3 residual)**: the gap-line model is gated by the extended spike on desktop, the WebKit project and the owner's phone; failure selects window swap or the contingent custom engine (Epic 1.7). UNVERIFIED until Task 0.1.4d.

## Unresolved Questions
- [ ] Which transport does `SearchContent` use in production, and does it stream (not buffer) through the plain `:8543` listener, the `:8444`/`:62872` remote-access TLS listener and a phone? `WatchSessions` is the wrong stand-in: production `Watch*` RPCs ride `StreamingWSBridge` (WebSocket), `SearchContent` is not a `Watch*` RPC and will use plain Connect fetch streaming unless the spike shows it buffers. Suspects, in order (ET-6): the `otelhttp` writer wrapper, then off-process hops; response compression is exempt for `/api/` (VERIFIED `server/middleware/gzip.go:155-163`). Remedy order: fix the buffering layer in-process, then the WS bridge (exact-path registration plus a transport-selector entry), then a unary wrapper. Blocks Story 1.1.2 and 1.4.1. Owner: Story 0.1.1 (agent).
- [ ] Connect `ReadMaxBytes`/`WriteMaxBytes` vs the 1 MB editable cap (the whole-file path; ADR-036's patch and range caps are separate and set in Tasks 2.1.5e and 1.6.2c); hidden-session write guard registration; **remote-access auth tier coverage of `/api` write RPCs** (can a read-scoped remote caller reach `WriteFile`?); existence of a public VCS-status invalidation on `Instance`. Blocks Stories 2.1.2, 2.1.3. Owner: Task 0.1.3b (agent).
- [x] **DECIDED 2026-10-17 (owner): directory-mode editing is ALLOWED, default on, with the "Editing the main checkout" chip.** Owner decision (non-blocking default; requested before Story 2.1.2 ships): editing is allowed in directory-mode sessions, i.e. directly in the main checkout (`WorktreeDir == ""`). Default is allow with a status-bar chip "Editing the main checkout"; a stricter `NoDedicatedWorktree` read-only reason is a one-line change if the owner vetoes. Owner: tstapler.
- [ ] diff3 library choice (license, size, API) for 3-way merge. Blocks Task 2.3.2b. Owner: Task 2.3.2a spike (agent).
- [x] **DECIDED 2026-10-17 (owner): draft persistence in localStorage and IndexedDB is ALLOWED (no veto); Story 2.2.2 is unblocked, still behind Gate G1.** Drafts persisted in localStorage (<= 256 KB) and, since triad round 3, in IndexedDB (256 KB to 1 MB) may contain secrets (ADR-032 chooses bounded drafts with explicit restore; both tiers have the same plaintext-at-rest exposure). Owner may veto either or both; a veto leaves the priority-1 "Draft not stored" banner as the standing state for dirty buffers. Blocks Story 2.2.2. Owner: tstapler.
- [ ] Real-device iOS PWA verification of keyboard bar, visualViewport and vim best-effort. Blocks Story 2.5.2 sign-off. Owner: tstapler.
- [ ] Retire `/files` LocalFileBrowser? Non-blocking. Owner: tstapler.
- [ ] **S-T3 phone gate (Task 0.1.4d)**: owner records PASS/FAIL for the gap-line and window-swap models on the phone; selects the engine path (Story 1.6.4, Task 1.6.4d, or contingent Epic 1.7). Blocks Story 1.6.4. Owner: tstapler (same sitting as Tasks 0.1.5b, 0.1.5d, 0.1.6b).
- [ ] **Remote-tier reach of `ApplyFilePatch` (Task 0.1.3b)**: can a read-scoped remote caller reach write RPCs? The capability design handles either answer; if yes, Task 2.1.5e gates the RPC by scope. Blocks Task 2.1.5e. Owner: Task 0.1.3b (agent).
- [ ] **ADR-036 constants**: `hash_cap_bytes` 64 MiB and read ceiling 1 GiB, 4,096-line checkpoints, window sizes, 8/64 MiB chunk cache and the limiter caps are proposals; S-T2 and Task 1.6.7b confirm or replace them. Owner: Tasks 0.1.7b, 1.6.7b (agent).
- [ ] **Transport flag defaults and the whole-file mode for 1-8 MB on desktop**: all three flags default off with the flip order above (planner's choice, not stated by ADR-036); ADR-036 open question 3 (a normal whole-document mode for 1-8 MB on desktop) is not built: T1 files use the windowed document. Owner may override either. Non-blocking. Owner: tstapler.
- [ ] **Phase D enablement (Epic 2.6)**: gated on the S-T1/S-T5 and Task 1.6.7b outcomes recorded in ADR-036; decides whether the stream adapters are advertised on by default. Owner: Tasks 0.1.8b, 1.6.7b (agent); owner may overrule.

## Deferred / Out of Scope (recorded)
- **Split view** (requirements.md In Scope: "optional split view"): explicitly **deferred**, no story in this plan. Rationale: the Files tab is phone-first; a second editor pane needs a second `EditorView` (ADR-032 chooses one view with `setState` for phone memory and IME safety) and a second layout path in `MobileFilesLayout`. The tree/search panel keyboard resize that UX U-A11Y-4 requires does **not** exist today (verified: `TreeResizeHandle.tsx` and `useResizablePanel.ts` have no `role`, `aria-*` or key handling); it is built in Story 1.3.6 and needs no split view. Revisit after the first release as a desktop-only follow-up; requirements.md updated to mark it deferred so verify does not read it as missing coverage.
- File create/rename/delete/move: out of scope (unchanged).
- **Deferred by ADR-036**: a two-phase staged commit for oversized pastes (T1 refuses; T0 falls back to `writeFull`); a `v2` token with `ctime_ns`/inode (the `v1` token and write-time hashing stay); a Merkle/chunk-digest token; incremental extension of the line index; autosave to disk (explicit Save only); OT/CRDT and a bidirectional WebSocket multiplex are rejected, not deferred.

## Dependency Visualization

```
Phase 0                       Phase 1 (read-only, ships first)                         Phase 2 (editing, flag-gated, last)

0.1.1 stream spike ----------> 1.1.1 proto+walker --> 1.1.2 handler --> 1.1.3 bench gate
0.1.2 bench harness ---------> 1.1.1                         |                |
0.1.3 platform recon ----------------------------------------|----------------|------------> 2.1.1 atomic write
0.1.4 CM6 swap spike --> 1.2.1 CodeEditor extract --> 1.2.2 useOpenBuffers --+                |
                                  |                          |               |                v
0.2.1 resolveInWorktree --> 0.2.3 codec+tokens+StatFile      |               |           2.1.2 WriteFile --> 2.1.3 invalidation
        |                         |        |                 v               |                |                 |
        |                         |        +---------> 1.3.4 staleness       |                v                 |
        v                         v                          |               |           2.1.4 corpus  <--------+
0.2.2 flags ----------------------+--> 1.3.1 tab strip --> 1.3.2 nav/jumplist/deeplink                |
                                  |         |                   |                                      v
                                  |         v                   v                          2.2.1 edit mode --> 2.2.2 guards/drafts --> 2.2.3 offline
                                  |    1.3.3 outline       1.3.5 status/palette/shortcuts          |                                 |
                                  |                                |                               v                                 v
1.1.2 --> 1.4.1 search client --> 1.4.2 search panel --> 1.4.3 search e2e                    2.3.1 ConflictDialog --> 2.3.2 3-way merge
                                         |                                                         |
                                         v                                                         v
                          1.5.1 mobile layout --> 1.5.2 mobile read mode --> 1.5.3 mobile e2e      2.4.1 vim --> 2.4.2 ExtraKeyBar --> 2.4.3 edit e2e
                                                                                                                       |
                                                                                                                       v
                                                                                                   2.5.1 gates+docs --> 2.5.2 manual instance + owner device check

Gate edges added by the engineering triad repair (ET-1); the edges above are unchanged:

  1.5.1 -> 1.5.2 -> 1.5.3 (1.5.3a, 1.5.3b, then 1.5.3c = GATE G1, the owner phone smoke)
                                |
                                v   nothing in Epics 2.2, 2.3 or 2.4 starts before G1 is recorded
  2.2.1 -> 2.2.2, 2.2.3, 2.2.4 (2.2.4 also needs 1.5.1 and 1.3.7)
  2.2.1 -> 2.3.1 -> 2.3.2 (2.3.2e also needs 0.2.2)
  2.2.1 -> 2.4.1 (2.4.1d also needs 1.5.1) -> 2.4.2 -> 2.4.3
  2.2.x, 2.3.x, 2.4.x -> 2.5.3 (metrics), 2.5.4 (owner scripts) -> 2.5.2 (walkthrough, flag-flip recommendation)
  Epic 2.1 (server write, dark) is not behind G1: it runs in Waves 4-5.

Serial edge added by triad round 2 (TR2-E3; `FilesTab.tsx` is the shared file in Wave 5):

  1.3.5 (status bar, palette; no FilesTab edit) -> 1.3.7a -> 1.3.7b (FilesTab) -> 1.4.2b (FilesTab)
        one worker, in that order; 1.4.1, 1.4.2a and 1.5.4 are independent of it and run in parallel.

Edges added by triad round 3 (E2; these were only wave prose before):

  0.1.6 (W1; R1/R2 pass or the view is redesigned) --> 1.5.4a --> 1.5.4b --> 1.5.3c (GATE G1)
  1.5.1c (W6) --> 1.5.4b                  (MobileFilesLayout.tsx, FilesTab.tsx)
  1.5.4 (whole story) before 1.5.3c       (the owner smoke at G1 includes the Accessible view and
                                           the screen-reader quick check; 1.5.3b/1.5.3a do not wait for it)
  1.5.4 --> 2.2.1b (Edit hand-off from the Accessible view, moved out of 1.5.4b), 1.5.4 --> 2.5.4b (R1-R4 re-run)
  1.1.2d (conditional, W4) --> Wave 4 gate (server.go length probe) --> 0.2.4f (conditional carve-out)
        --> 2.1.2d --> 2.1.2f --> 2.1.2g (conditional) --> 2.1.3a ; 2.5.3a (W9) edits server.go last
  0.1.3b (W1 remote-access finding) --> 2.1.2g ; 0.1.1b (W1 transport finding) --> 1.1.2d
  2.2.2a --> 2.2.2d (IndexedDB tier) --> 2.2.2b --> 2.2.2c --> 2.2.2e (Save all)
  2.3.2c --> 2.3.2e --> 2.3.2f (2.3.2f consumes the `phoneMergeAvailable` predicate 2.3.2e defines)
  0.1.5b --> 0.1.5d (time-to-answer baseline, same sitting) --> 0.1.6b

Edges added by the ADR-036 integration (transport and editor-engine port; Phases A-D; waves in the Effort estimate):

  Phase 0 spikes (W1)   0.1.7a -> 0.1.7b -> 0.1.8a -> 0.1.8b -> 0.1.4d      (the ADR-036 "Spike outcomes" editors, in this order)
                        0.1.4c (S-T3 prototypes) --> 0.1.4d (owner phone gate, appended after 0.1.6b)
                              --verdict--> 1.6.4a-c (a2 passes) | 1.6.4d (a2 fails, a1 passes) | contingent Epic 1.7 (both fail)
                        0.1.3b (remote-tier reach of write RPCs + ServeFileRaw audit) --> 2.1.5e, 2.6.3a
                        0.1.7b (S-T2 constants) --> 1.6.1 ; 0.1.8a (encoder + golden-vector schema) --> 2.2.5a, 2.1.5f
  Phase A (W2-W3)       0.2.2a (flags) --> 0.2.6a (proto, after 0.2.3a) --> 0.2.6b --> 0.2.6c (file_service.go after 0.2.3c) --> 0.2.6d
                        1.2.1a --> 1.2.1f (ports + CmEngine) --> 1.2.1b ;  1.2.1c --> 1.2.1g (engine conformance)
                        1.2.2b + 0.2.6a --> 1.2.3a --> 1.2.3b --> 1.2.3c --> 1.2.2c ;  0.2.5a, 0.2.5c --> 0.2.5d
  Phase B (W6; read-only value, before any editing epic)
                        0.2.6c, 0.2.3c --> 1.6.1a --> 1.6.1b ;  1.6.2a (session.proto after 1.1.1a) --> 1.6.2b --> 1.6.2c --> 1.6.2d
                        1.6.2a --> 1.6.3a --> 1.6.3b --> 1.6.3c ;  0.1.4d + 1.2.1f --> 1.6.4a --> 1.6.4b --> 1.6.4c (--> 1.6.4d if a1)
                        1.6.3b + 1.6.4c (+ 1.5.2b on CodeEditor.tsx) --> 1.6.5a --> 1.6.5b (FilesTab lane after 1.5.4b) ;  1.5.4b --> 1.6.5c
                        1.1.1a (proto fields) --> 1.6.6a ;  1.5.1b + 1.4.2 --> 1.6.6b
                        1.6.5, 1.6.6, 1.5.3b --> 1.6.7a --> 1.6.7b --> 1.6.7c (owner phone check; flips files:range-read)
  Phase C (code after G1; flag flips after G2/G3)
                        2.1.2g (W5), 1.6.1b, 0.2.6c --> 2.1.5a (after 1.6.2a) --> 2.1.5b --> 2.1.5c --> 2.1.5d --> 2.1.5e --> 2.1.5f
                        2.1.5a --> 2.3.3a (PreflightFilePatch)
                        2.2.1, 2.2.2d, 2.2.3a (W7) + 2.1.5 + 1.6.3a + 1.6.4c + 0.1.8a --> 2.2.5a --> 2.2.5b --> 2.2.5c --> 2.2.5d --> 2.2.5e --> 2.2.5f
                        2.3.2f (W8) + 2.2.5b + 2.3.3a --> 2.3.3b ;  2.3.2f --> 2.3.3c
                        2.2.5, 2.3.3, 2.4.3a --> 2.5.5a ;  2.5.4b --> 2.5.5b --> 2.5.2a ;  2.5.1b, 2.5.3c --> 2.5.5c
                        Gate G2 = Task 2.5.4a signed --> flip files:delta-write ;  Gate G3 = Task 2.5.5b T1 steps --> flip files:edit-large
  Phase D (W10, last; Gate G-D = S-T1, S-T5 and 1.6.7b recorded in ADR-036)
                        2.1.5a --> 2.6.1a --> 2.6.1b (server.go after 2.5.3a) ;  1.6.3 + 1.2.3a --> 2.6.2a, 2.6.2b --> 2.6.2c
                        0.1.3b --> 2.6.3a --> 2.6.3b --> 2.6.3c (how-to after 2.5.5c)
  Contingent Epic 1.7 (custom engine): starts only if 0.1.4d records FAIL for a2 and a1; replaces 1.6.4 in Wave 6 and is not in the base totals.
```

Added by the pre-mortem repair: 0.1.5 (real-device spike, Wave 1, before Epics 1.5/2.4), 0.2.1d (symlink inventory, before 0.2.1c), 0.2.4 (per-wave gate checkpoints, Wave 2), 1.1.1e (`IgnoreMatcher`, before 1.1.1c), 1.3.2c (single URL owner, with 1.3.2), 1.5.3c (owner phone smoke = Gate G1, before Epics 2.2/2.3/2.4 start), 2.2.2c (`SessionBufferHold`).

Added by the earlier repair: 0.1.4b (WebKit CI check) runs in Wave 1 and decides the gating mobile project for 1.5.3/2.4.3; 2.1.2b (`FileWriter` ports) precedes 2.1.2c-e and consumes the 0.1.3b recon.

Added by the triad repair: 0.2.5 (analytics helper, Wave 3; every emitting story depends on it), 1.3.7 (banner stack and feedback states, Wave 5; 2.2.x banners register into it), 2.2.4 (Save affordances and read-mode hint, after 2.2.1, 1.5.1 and Gate G1), 2.3.2e (phone merge kill switch, after 0.2.2 and 2.3.1), 2.4.1d (input-profile precedence), 2.5.3 (metrics endpoint and page, after all emitters), 2.5.4 (owner scripts, before 2.5.2 and before the `files:editing` flip).

Added by the engineering triad repair: 0.2.1e (tree listing and tree-search retrofit, split from 0.2.1c; after 0.2.1c), 0.2.4c (race-suite baseline and BUG-067 note, first thing in Wave 2), 0.2.4d (conditional package relocation, only if the race budget is exceeded), 0.2.4e (test harness factory and jscpd ledger, Wave 2), 1.1.1f (per-file scanner, split from 1.1.1c; before it), 1.1.2c (open-stream limiter, before 1.1.2a wires it), 1.2.1e (`extensions/*` stubs, split from 1.2.1a; before it).

Added by triad round 3: 0.1.5d (time-to-answer baseline, Wave 1), 1.1.2d (conditional WS-bridge registration, Wave 4), 2.2.2d (IndexedDB backup tier for 256 KB to 1 MB dirty buffers, Wave 7), 2.2.2e (Close others "Save all" sequence, Wave 7); re-classes: 1.5.1c S to M, 1.5.4b S to M, 2.1.2f XS to S.

Added by triad round 2: 0.1.5c (input-event capture and detector fixtures, with 0.1.5), 0.1.6 (screen-reader read-path spike, Wave 1; its Accessible-view finding gates Story 1.5.4's scope), 0.2.4f (conditional `server/server.go` length probe at the Wave 4 gate, before 2.1.2d/2.1.2f touch it), 1.3.5c (chord denylist test and per-browser shortcut spec), 1.4.2d (first-tap activation with the keyboard open and the idle state), 1.5.4 (Accessible view, Wave 6, before Gate G1), 2.1.2g (conditional remote-tier deny task, from the 0.1.3b finding), 2.3.2f (phone merge states), 2.4.2c (edit layout budget and fixed Save/Stop cluster).

Added by the ADR-036 integration (owner decisions 2026-10-16): spikes 0.1.4c/0.1.4d (S-T3 extension), Story 0.1.7 (S-T1, S-T2), Story 0.1.8 (S-T4, S-T5); Phase A: Story 0.2.6, Task 0.2.5d, Story 1.2.3, Tasks 1.2.1f/1.2.1g; Phase B: Epic 1.6 (Stories 1.6.1-1.6.7, with conditional Task 1.6.4d); Phase C: Stories 2.1.5, 2.2.5, 2.3.3, 2.5.5; Phase D: Epic 2.6 (Stories 2.6.1-2.6.3); contingent Epic 1.7.

Parallel lanes after Phase 0: (server search 1.1.x) || (frontend seams 1.2.x then tabs 1.3.x) ; 2.1.x (server write) can start once 0.2.x and 0.1.3 are done, in parallel with Phase 1 frontend, but it ships dark.

---

# Phase 0: De-risking and foundations

## Epic 0.1: Spikes (riskiest unknowns first)
**Goal**: Replace the five UNVERIFIED research claims with measured facts before building on them. Outcomes are recorded in the PR description and in the "Spike outcomes" section of the relevant ADR.

#### Story 0.1.1: `SearchContent` streaming on its real production path
**As a** developer, **I want** proof that a short-lived server-streaming RPC (with abort) streams incrementally over the exact transport `SearchContent` will use in production, **so that** `SearchContent` can be streaming from day one.
**Why the stand-in changed (pre-mortem #5, VERIFIED)**: `server/server.go:499-509` routes only `Watch*` RPCs through `StreamingWSBridge`, and `createSessionWatchTransport` (`web-app/src/lib/transport/watch-ws-transport.ts:223-230`) uses native Connect streaming only when `NEXT_PUBLIC_CONNECTRPC_NATIVE_STREAMING=true` and the base URL is `https://`. `TestEveryServerStreamingWatchRPCHasAWebSocketBridge` (`server/watch_ws_bridge_registration_test.go`) only checks `Watch*`, so it will not flag `SearchContent`. A spike on `WatchSessions` therefore validates the WebSocket bridge, which `SearchContent` will not use by default.
**Acceptance Criteria**:
- The transport is chosen first and recorded.
  - *Given* the production transport selection (`useFileService.ts` builds `getConnectTransport()`; the Watch selector is `createSessionWatchTransport`), *When* the spike starts, *Then* ADR-030 states which of three paths `SearchContent` uses: (a) plain Connect fetch streaming through `getConnectTransport()` (default for a short-lived, abortable stream; holds one HTTP/1.1 connection only while a search runs), (b) the WS bridge by exact-path registration plus a selector entry, (c) unary wrapper `SearchContentUnary`.
- A throwaway test-only streaming RPC on the chosen path is consumed incrementally.
  - *Given* a test-only server-streaming handler (registered in the isolated e2e/manual server through the same `handler` stack and middleware chain as `SessionService`, sending 3 messages 100 ms apart, then a final message) and the web client built exactly as `useFileService` builds it, *When* a Playwright check against the isolated e2e server and a jest test with a fake streaming transport read the stream, *Then* messages arrive one by one (not buffered to completion) and the first arrives before the stream ends.
- The in-process middleware chain is cleared first, with the suspect list stated (engineering triad ET-6).
  - *Given* `localChain()` and `remoteChain()` (`server/server.go:1896-1915` and `:1933`, wrapped by `otelhttp` at `:1536-1541` and `:1965-1969`) serving a throwaway streaming handler on an `/api/...` path that sends 3 messages 100 ms apart, *When* a Go test client reads the response body incrementally with `Accept-Encoding: gzip, zstd`, *Then* each arrival gap is >= 80 ms (not one burst at the end); on failure the test bisects by rebuilding the chain with one layer removed at a time and names the buffering layer.
  - Suspects, ordered, with evidence labels. (1) **Response compression is not a suspect on `/api/` paths** (VERIFIED: `server/middleware/gzip.go:155-163` returns `next` untouched for every `/api/` request, and the wrapper flushes its encoder on `Flush()` anyway, `:93-103`); `SearchContent` is served under `/api` (`server.go:497`), so it is never wrapped by `compressResponseWriter`. It becomes relevant only if a streaming route is ever mounted outside `/api/`; the first remedy there is an exempt-path entry in `Compress`, before any transport change. (2) The `otelhttp` response-writer wrapper with `WithMessageEvents(ReadEvents, WriteEvents)`: it should preserve `http.Flusher` through `httpsnoop` (`go.mod:144`), INFERRED, not tested here, so it is the first thing the chain test exercises. (3) `middleware.Logging` implements `Flush` (VERIFIED `logging.go:33-37`); `CORS` and `HostGuard` do not wrap the writer (VERIFIED by reading `cors.go`, `hostguard.go`), so they are cleared. (4) Off-process: the `:8444` TLS listener most likely serves HTTP/2 (INFERRED), and the phone's LAN/Tailscale path may add a buffering proxy or a browser without fetch-stream support; both are UNVERIFIED until measured below.
  - Remedy order if buffering is observed: fix the identified layer in-process (Flusher passthrough or exempt path), then path (b) WS bridge, then path (c) unary.
- The remote and phone paths are measured, not assumed.
  - *Given* the manual isolated instance (`PORT=62871`, `--remote-port 62872`), *When* the same stream is opened over `:62871`, over `:62872`, and from the owner's phone over LAN or Tailscale, *Then* the observed first-message time and whether batches were delivered incrementally are recorded in ADR-030; if the phone or `:62872` path buffers, path (b) is tried, then (c) becomes the default.
- Connection-slot impact is measured.
  - *Given* a search stream held open on the plain HTTP/1.1 listener, *When* a second unary RPC is issued, *Then* it completes (the 6-connection-per-origin limit is not exhausted by one or two concurrent searches); recorded in ADR-030.
- Abort cancels the server context.
  - *Given* the test stream open, *When* the client aborts via `AbortController.signal`, *Then* the server handler observes `ctx.Done()` within one second (asserted in a Go test using a fake stream).
- Decision recorded.
  - *Given* the results, *When* ADR-030 is updated, *Then* it names the transport, any registration needed (and, for path (b), that the WS-bridge registration test is extended so a future `SearchContent`-like RPC cannot be missed), and the unary fallback shape if (c).
**Files**: `web-app/src/lib/hooks/useFileService.ts` (locate with Glob first), `web-app/src/lib/hooks/__tests__/streamingSpike.test.ts`, `server/streaming_chain_test.go`, `server/services/streaming_spike_test.go`, `tests/e2e/files-viewer.spec.ts` (spike section), `project_plans/files-viewer-revamp/decisions/ADR-030-in-process-streaming-content-search.md`

##### Task 0.1.1a: Locate the transport and write the streaming/abort test (S)
- Glob for `useFileService`, read transport construction and `createSessionWatchTransport`; write a jest test with a fake streaming transport asserting incremental delivery and abort propagation, using a test stream that sends 3 messages 100 ms apart. Do not use `WatchSessions` (WebSocket bridge, long-lived, low-rate).
- Files: `web-app/src/lib/hooks/__tests__/streamingSpike.test.ts`

##### Task 0.1.1b: Server abort test and real-path check on `:62871`, `:62872` and the phone (S)
- Go test: a fake `ServerStream` handler returns on `ctx.Done()`. First write the in-process chain test (`server/streaming_chain_test.go`, package `server`, beside `server_probeguard_test.go`, which already builds `localChainWith`/`remoteChain`) that reads a 3-messages-100-ms-apart `/api/` stream through the real chain and asserts the arrival gaps (AC "in-process middleware chain"); keep it as a regression test (it is cheap and uses no tmux or storage). Then register the throwaway test-only streaming handler behind the same middleware chain, run it on the isolated manual instance (ports 62871/62872, `STAPLER_SQUAD_TMUX_SOCKET=ssq-manual-1`), stream over both ports and from the phone, and record buffering and connection-slot behavior in ADR-030. Remove the throwaway handler before merging.
- Files: `server/streaming_chain_test.go`, `server/services/streaming_spike_test.go`, `project_plans/files-viewer-revamp/decisions/ADR-030-in-process-streaming-content-search.md`

#### Story 0.1.2: 50k-file search benchmark harness
**As a** developer, **I want** a reproducible 50k-file fixture and prototype walker benchmark, **so that** the 2 s first-result SLO is measured, not assumed.
**Acceptance Criteria**:
- Fixture is deterministic.
  - *Given* seed `42`, *When* `genFixtureTree(t, 50000)` runs, *Then* it creates 50,000 files across about 2,000 directories incl. 5% binary, a `node_modules/` with 20,000 ignored files and a `.gitignore`, byte-identical across runs.
- Benchmark reports first-result and total latency.
  - *Given* the fixture and a literal query `TODO`, *When* `go test -bench BenchmarkContentSearch -run ^$ ./server/services &` runs, *Then* it prints `first_result_ms` and `total_ms` as custom metrics.
- The fixture can expose many-`.gitignore` and cold-cache cost (pre-mortem #4).
  - *Given* a second fixture variant `genFixtureTree(t, 50000, WithGitignores(200))` (200 nested `.gitignore` files, a monorepo shape), *When* the benchmark runs on it, *Then* the same custom metrics are printed for it separately, and a documented cold-cache procedure (`sync; echo 3 > /proc/sys/vm/drop_caches` as root on Linux, or a fresh checkout of the generated tree on another host; skipped with a printed reason when neither is possible) is available to the benchmark runner.
**Files**: `server/services/content_search_bench_test.go`, `server/services/content_search_fixture_test.go`

##### Task 0.1.2a: Fixture generator (S)
- Deterministic generator using `math/rand` seed 42; writes under `b.TempDir()`; option `WithGitignores(n)` spreads `n` small `.gitignore` files across the tree (default 1).
- Files: `server/services/content_search_fixture_test.go`

##### Task 0.1.2b: Prototype walker benchmark (throwaway core) (S)
- `filepath.WalkDir` + `regexp` single-thread baseline and an `errgroup` worker-pool variant; benchmarks only. Remember: benchmarks run with `&` per `docs/reference/benchmarks.md`.
- Files: `server/services/content_search_bench_test.go`

#### Story 0.1.3: Platform recon for the write path
**As a** developer, **I want** verified answers on `os.Root` behavior, Connect message limits, guard registration and status invalidation, **so that** ADR-031 rests on facts.
**Acceptance Criteria**:
- `os.Root` semantics are pinned by tests.
  - *Given* a temp worktree with `link_out -> /etc` and `link_in -> sub/real.txt`, *When* `root.Open("link_out/passwd")` and `root.Lstat("link_in")` run (Go 1.26.6), *Then* the first returns an escape error and the second reports `ModeSymlink`; `root.OpenFile(".t", O_CREATE|O_EXCL)`, `root.Rename`, and directory `Sync` all work.
- Message limit is known (VERIFIED in triad round 2: `server/` sets no `connect.WithReadMaxBytes`; `grep -rn ReadMaxBytes server/` finds only a comment at `server/services/backlog_service_lifecycle.go:1054` calling the absence a pre-existing repo-wide gap).
  - *Given* the Connect handler options in `server/server.go` (`sessionOpts`, `:495`), *When* a body of 1.5 MiB + 1 byte is posted in a test, *Then* the recon records that Connect's default accepts it (no limit), which is why Task 2.1.2f installs a `WriteFile`-scoped body limit unconditionally rather than "if it must be raised".
- Guard and invalidation facts are recorded.
  - *Given* `server/services/guards_flag.go` and `session/git_worktree_watcher.go`, *When* grepped, *Then* ADR-031 states whether write RPCs register per-RPC with the hidden-session guard and names the exact `Instance`/VCS-cache invalidation call (VERIFIED: only `SwitchWorkspace` evicts `vcsStatusCache`, `server/services/workspace_service.go:447`, so Task 2.1.3a adds `InvalidateVCSStatus` there).
- The local-listener guard map is named before the handler is built (pre-mortem #1, VERIFIED).
  - *Given* `server/server.go:1879-1888` (`guardedProcedures`: `ProbeProgram`, `NudgeSessionForPR`, `PruneHiddenSessionNotifications`, `UpdateFeatureFlag`, `ReplyToPendingQuestion`) and `localChainWith` at `:1901-1915`, *When* the recon runs, *Then* ADR-031 "Spike outcomes" records that the unauthenticated `:8543` listener only enforces POST-only plus the Origin/rebinding verdict (`middleware.ProfileRebinding`, `server/middleware/callerguard.go:21-24`) for procedures present in that map, and that `WriteFile` must be registered there (enforced by Task 2.1.2d).
- Remote-access exposure of the write RPC is known before the handler is built.
  - *Given* the `/api` remote-access middleware and auth tiers in `server/`, *When* a Go test posts to an existing mutating RPC (e.g. `WriteToSession`) as an unauthenticated remote caller and, if a read-only/limited tier exists, as that tier, *Then* ADR-031 "Spike outcomes" records which tiers can reach a write RPC and whether a per-RPC deny entry is required for `WriteFile`; Task 2.1.2d only implements what this records.
- ADR-036 additions to the recon (owner decisions 2026-10-16; the capability design works for either answer, but the answer decides the wiring of Tasks 2.1.5e and 2.6.3a).
  - *Given* the same remote-tier test that exercises `WriteToSession`, *When* it is extended to POST a stand-in unary write procedure with a patch-sized body (up to 2 MiB) and to GET a route that returns file bytes, *Then* ADR-036 "Spike outcomes" records whether a read-scoped remote caller can reach write RPCs (the input to `ApplyFilePatch` on `:8444`: if yes, Task 2.1.5e gates the RPC by scope; if no, `PATCH` is advertised for write-scoped remote callers) and whether the plain `:8543` listener applies the Host/Origin rebinding verdict to GET routes at all.
  - *Given* `ServeFileRaw` (`server/services/file_service.go`, VERIFIED to stream media with `http.ServeContent` and no size cap), *When* a foreign-`Origin` GET and a rebinding-`Host` GET are sent through the real `localChain`, *Then* the result (served or rejected) is recorded; Task 2.6.3a adds the guard if it is served.
**Files**: `server/services/os_root_semantics_test.go`, `project_plans/files-viewer-revamp/decisions/ADR-031-writefile-atomicity-confinement-concurrency.md`

##### Task 0.1.3a: `os.Root` semantics test matrix (S)
- Table test over escape symlink, in-tree symlink, `..`, absolute path, rename, `O_EXCL`, dir fsync; record anything platform-specific (macOS work machine) as skipped-with-reason.
- Files: `server/services/os_root_semantics_test.go`

##### Task 0.1.3b: Recon of Connect limits, guards, remote-access tiers, invalidation, metrics pattern (S)
- Also (ADR-036): answer the remote-tier question for patch-sized write bodies and record whether `ServeFileRaw`'s GET passes the rebinding verdict (see the story's ADR-036 AC). Read `server/server.go` handler options, the `guardedProcedures` map and `localChainWith` (`:1879-1915` (map 1879-1888, `localChain` 1896-1899, `localChainWith` 1901-1915)), `guards_flag.go`, the remote-access/auth middleware and any read-only tier, `session/git_worktree_watcher.go`, existing metrics registration (`server/services/file_metrics.go` does NOT exist; find the real pattern elsewhere in `server/` and `session/`); write findings into ADR-031 "Spike outcomes", including the remote-access answer above.
- Files: `project_plans/files-viewer-revamp/decisions/ADR-031-writefile-atomicity-confinement-concurrency.md`, `project_plans/files-viewer-revamp/decisions/ADR-036-pluggable-range-and-delta-file-transport.md`

#### Story 0.1.4: CodeMirror state-swap spike (+ extended S-T3 engine gate, ADR-036)
**As a** developer, **I want** confirmation that one `EditorView` with per-tab `EditorState` keeps scroll, selection and undo, **so that** the buffer model (ADR-032) holds.
**Acceptance Criteria**:
- State swap preserves position.
  - *Given* two `Buffer`s of 2,000 lines each, scrolled to line 900 and 1400 with a selection, *When* the test calls `view.setState(other)` then back, *Then* `scrollSnapshot` restore returns `scrollTop` within 1 line height and selection ranges and undo depth are unchanged.
- Composition safety.
  - *Given* a view with `view.composing === true`, *When* a tab switch is requested, *Then* the switch is deferred until `compositionend`.
- Extended S-T3 (ADR-036, owner decision 2026-10-16): the editor engine is chosen by measurement, not assumed. CodeMirror 6 stays the shipping engine unless both lazy-document models fail on the owner's phone.
  - *Given* a throwaway page holding a virtual 50 MB / 1,000,000-line file served by a fake `VirtualDoc` source, *When* the gap-line model (a2: loaded lines plus one block-replace placeholder per unloaded gap, gap table in a `StateField`, non-history load/evict transactions, `changeFilter` and `atomicRanges` over placeholders) and the window-swap model (a1: one window per `EditorState`, edge-triggered `setState`, line-offset gutter) each run in desktop Chromium and in the WebKit project of Task 0.1.4b, *Then* each model records pass/fail for: scroll anchoring when a gap above the viewport loads (viewport top line moves by at most one line height), undo history intact across 100 load/evict cycles, the pending-ChangeSet-to-absolute-line/col conversion equal to a full-replace oracle on 1,000 random edits, a heap ceiling under the phone budget (8 MiB of chunk text plus structures), and no dispatch while `view.composing`.
  - *Given* the owner's phone (the Task 0.1.4d sitting, with Tasks 0.1.5b, 0.1.5d and 0.1.6b), *When* the owner scrolls fast, jumps to a far line and types in each model, *Then* the owner records PASS or FAIL for a2 and for a1 in the checklist and in ADR-036 "Spike outcomes"; **a2 PASS selects the gap-line module (Story 1.6.4); a2 FAIL with a1 PASS selects window swap (Task 1.6.4d) and the single-editable-window UX; both FAIL activates the contingent custom engine (contingent Epic 1.7, costed and excluded from the base total) and until it ships T1 and T2 files are view-only through the Accessible-view pager.**
**Files**: `web-app/src/components/sessions/__tests__/codeEditorStateSwap.test.tsx`, `tests/e2e/files-viewer.spec.ts`, `web-app/src/components/sessions/__tests__/cm6LazyDocSpike.test.tsx`, `tests/e2e/files-lazydoc-spike.spec.ts` (extended S-T3, ADR-036)

##### Task 0.1.4a: Playwright state-swap spike (kept as regression test) (S)
- Add a spike spec section to `tests/e2e/files-viewer.spec.ts` using `data-testid` locators; header `// @feature files:tabs`.
- Files: `tests/e2e/files-viewer.spec.ts`, `tests/e2e/pages/files-tab.page.ts`

##### Task 0.1.4b: Verify WebKit/mobile Playwright projects can run in CI (Wave 1) (S)
- `tests/e2e/playwright.config.ts` today defines only the visual-theme and `chromium` (Desktop Chrome) projects. In a throwaway branch/CI run, add a `mobile-webkit` project running one trivial spec; confirm WebKit installs on the CI runner (mind `docs/how-to/fix-playwright-chromium-install-stall.md`) and the run is green. Record the outcome in the PR description. Decision rule: if WebKit cannot run reliably on CI, `mobile-chromium` (Pixel-class, 390x844, touch) becomes the **gating** project and `mobile-webkit` is informational only (`continue-on-error`); Stories 1.5.3 and 2.4.3 then reference the gating project.
- Files: `tests/e2e/playwright.config.ts`, `.github/workflows/` (only if a browser-install step is needed)

##### Task 0.1.4c: Extended S-T3 prototypes: gap-line (a2) and window-swap (a1) with the full-replace oracle (M; ADR-036)
- Throwaway `VirtualDoc` fake (50 MB virtual text generated from a seeded PRNG), the a2 prototype (`StateField` gap table, placeholder widget with `estimatedHeight`, gutter through the table, load/evict transactions annotated `addToHistory.of(false)`, `changeFilter` and `atomicRanges` guards) and the a1 prototype, plus a jest oracle (`pending.map(loadChange)` converted to absolute line/col versus applying the same edits to a fully materialized document) and a Playwright spec that runs both models in desktop Chromium and in the Task 0.1.4b WebKit project, measuring scroll jitter, history survival, heap and composition safety. Results (numbers and pass/fail) go into ADR-036 "Spike outcomes" in the Task 0.1.4d order. Kept as the regression test of Story 1.6.4. The spike page is its own route so it does not join the Story 0.1.5 page chain.
- Files: `web-app/src/components/sessions/__tests__/cm6LazyDocSpike.test.tsx`, `web-app/src/app/spike-lazydoc/` (throwaway page, removed before merge), `tests/e2e/files-lazydoc-spike.spec.ts`

##### Task 0.1.4d: Owner phone gate for S-T3: record PASS/FAIL for a2 and a1 (XS; wall-clock blocker)
- Owner, same sitting as Tasks 0.1.5b, 0.1.5d and 0.1.6b (appended last, after 0.1.6b); no code. Records the engine verdict (a2 / a1 / contingent custom engine) in `manual-verification-checklist.md` and in ADR-036 "Spike outcomes". Blocks Story 1.6.4 (and with it Epic 1.6's windowed viewer); Stories 1.6.1 to 1.6.3 (server index, `ReadFileRange`, transport) do not wait for it.
- Files: `project_plans/files-viewer-revamp/implementation/manual-verification-checklist.md`, `project_plans/files-viewer-revamp/decisions/ADR-036-pluggable-range-and-delta-file-transport.md`

#### Story 0.1.5: Real-device input and viewport spike (before Epics 1.5 and 2.4)
**As the** owner, **I want** the riskiest iOS/Android behaviors checked on my phone in Phase 0, **so that** the mobile layout and editing epics are not built on emulator assumptions (pre-mortem #10).
**Acceptance Criteria**:
- Throwaway page on the isolated instance.
  - *Given* a throwaway page (CodeMirror 6 with a long document, a fixed bottom bar, and a bar pinned to `visualViewport`, using the `ViewportProvider` pattern) served from the manual instance on `:62871`, *When* the owner opens it on the phone over LAN or Tailscale, *Then* the owner records in the PR description whether the pinned bar stays above the soft keyboard, whether focus-scroll jumps the caret out of view, and whether composition (`keyCode 229`) input works; a failing result changes the Epic 1.5/2.4 design before those epics start.
- Keyboard-detection event shapes are captured, not guessed (triad round 2; the first detector, "first non-229 non-modifier keydown", false-positives on soft keyboards).
  - *Given* the throwaway page's capture panel (Task 0.1.5c), *When* the owner types a sentence with the phone's soft keyboard (letters, Enter, Backspace, Space, Tab where offered, digits, punctuation, an IME composition, an emoji), *Then* the panel lists for each `keydown` its `key`, `code`, `keyCode`, `isComposing` and `isTrusted` (only on screen, never sent anywhere), the owner exports them as JSON into the PR description, and the same is done with a Bluetooth keyboard attached (letters, Esc, Ctrl+key, arrows, F-keys); the recorded fixtures become `inputProfile.test.ts` cases.
  - *Given* those fixtures, *When* `detectHardwareKeyboard(events, env)` (the pure S1-S4 function of ADR-033) is run, *Then* every soft-keyboard sequence resolves to `Touch` with zero flips (including Enter, Backspace, Space and Tab) and every Bluetooth sequence resolves to `HardwareKeyboard` after at most one Escape/arrow/F-key/Ctrl-chord or three matching letters; a mismatch changes the rule in ADR-033 before Story 2.4.1 starts. The same page records the real soft-keyboard height at 320x568 (the 260 px assumption of ux.md 7.1a) and whether `visualViewport` shrinks when a hardware keyboard is attached and the editor is focused (signal S4).
- Autocorrect-hostile attributes are explicit, not assumed.
  - *Given* the `CodeEditor` content element, *When* a jest test inspects `EditorView.contentAttributes`, *Then* `autocorrect="off"`, `autocapitalize="off"` and `spellcheck="false"` are set explicitly (whether CodeMirror 6 sets them by default is UNVERIFIED: `node_modules` is not installed here), and the owner checklist (Story 2.5.2) has a line "type `--` and `"x"`, save, then diff the bytes: no em dash or curly quotes".
- Time-to-answer baseline is measured, not asserted (triad round 3; the success metric "<= 3 interactions" and the adoption review need a comparator, and the pain is asserted rather than measured).
  - *Given* the owner's current phone workaround (SSH or terminal on the phone, or any other route actually used), *When* the owner finds three different strings in a real session worktree and reaches each hit's line, once each, *Then* the seconds and the interaction count (requirements.md counting protocol) of each trial, the route used, the device and the network are recorded in the checklist spike notes and the PR description (Task 0.1.5d); the median of three is the baseline the find-and-open metric and the 4-week review are compared against, and a trial the owner cannot complete on the phone is recorded as "not possible" (that is the "baseline: impossible" claim, now evidenced).
**Files**: `web-app/src/components/sessions/__tests__/CodeEditorContentAttributes.test.tsx`, `web-app/src/lib/files/__tests__/inputProfile.test.ts` (fixtures; the function lands in Task 2.4.1d), `project_plans/files-viewer-revamp/implementation/manual-verification-checklist.md` (spike notes section), throwaway page under `web-app/src/app/` removed before merge

##### Task 0.1.5a: Throwaway spike page and contentAttributes assertion (S)
- Build the page behind a temporary route; add the `contentAttributes` assertion as a kept regression test (applied for real in Task 1.2.1a).
- Files: `web-app/src/components/sessions/__tests__/CodeEditorContentAttributes.test.tsx`

##### Task 0.1.5c: Key-event capture panel and detector fixtures (S)
- On the Task 0.1.5a throwaway page add the on-screen `keydown` capture panel (field list above, copy-as-JSON button) and commit the recorded fixtures as `inputProfile.test.ts` data once the owner has exported them (Task 0.1.5b); the panel is removed with the page. Files stay within the 3-5 limit by sharing the page file with 0.1.5a.
- Files: `web-app/src/app/` (throwaway spike page, same file as Task 0.1.5a), `web-app/src/lib/files/__tests__/inputProfile.test.ts`

##### Task 0.1.5b: Owner phone check of the spike page, recorded (XS)
- Wall-clock blocker (owner). Serve from the isolated instance on `:62871`; results go in the PR description and `manual-verification-checklist.md`; the page is deleted afterwards.
- Files: `project_plans/files-viewer-revamp/implementation/manual-verification-checklist.md`

##### Task 0.1.5d: Owner time-to-answer baseline on the phone, recorded (XS)
- Wall-clock blocker (owner), same sitting as Tasks 0.1.5b and 0.1.6b; no code. Three timed find-a-string-and-reach-the-line trials with today's workaround, recorded as in the Story 0.1.5 baseline AC. Ordered after Task 0.1.5b and before Task 0.1.6b in `manual-verification-checklist.md` (three tasks append to the same spike-notes section).
- Files: `project_plans/files-viewer-revamp/implementation/manual-verification-checklist.md`

#### Story 0.1.6: Screen-reader read path on CodeMirror 6 (before Story 1.5.4 and Epic 2.4; triad round 2)
**As the** owner and a potential assistive-technology user, **I want** to know in Phase 0 whether VoiceOver and TalkBack can read a long document in CodeMirror 6 at all, **so that** the architecture includes a non-virtual read path where it cannot, instead of discovering it in Wave 9 after the design is built.
**Why**: CM6 mounts only the lines near the viewport, so explore-by-touch and the virtual cursor cannot reach the rest of a file, and "focus at the last tap position" is meaningless for a user who has not tapped. The previous plan deferred this to Task 2.5.4b, an owner-only script with no pass criteria, run in Wave 9.
**Acceptance Criteria** (pass/fail criteria are fixed here so the spike has an answer):
- The spike page offers both views of the same 3,000-line document (generated, each line `L<n> text`).
  - *Given* the Story 0.1.5 throwaway page extended with (a) the production-shaped CodeMirror 6 read-only view and (b) a prototype of the Accessible view (ux.md 8.10: pages of 500 lines in `<pre tabindex=0>`, line numbers as text, pager with Previous/Next page buttons and a Go-to-line input), *When* a Playwright test reads Chromium's accessibility tree for each, *Then* the CM6 view exposes at most the viewport-adjacent lines (the count is recorded, expected well under 3,000) and the Accessible view exposes exactly 500 lines per page with the page label "Lines 501-1000 of 3000"; this is automated and kept as a regression test of the Accessible view's structure.
- **Owner pass criteria on the phone** (recorded as pass/fail per row with the device, OS and screen-reader version in the PR description):
  - *R1 (Accessible view, required)*: with VoiceOver (iOS) and TalkBack (Android), swiping next from the first line of a page reads 50 consecutive lines in order with no skipped or repeated line and each announcement begins with the line number; the Next page button is reachable by swipe; Go to line 1,234 lands on the page containing it and reads "Line 1234" first.
  - *R2 (Accessible view, required)*: heading/landmark or rotor navigation reaches the pager and the code region; the skip link "Switch to accessible view" is the first element reached inside the editor region by swipe from the file name.
  - *R3 (CM6 Read mode, informational)*: swiping reaches line 1, a line in the viewport and then stops at the last mounted line; the record states whether off-screen lines are ever reachable (expected: no). A pass here would not remove the Accessible view; a fail confirms it is required.
  - *R4 (CM6 Edit, informational; feeds Task 2.5.4b)*: typing letters echoes each character, moving the caret announces the character or line, IME composition does not duplicate text, and Delete/Backspace announce the removed character.
- Decision rule recorded in ADR-033's spike-outcome note and ux.md 8.10: R1 and R2 must pass or the Accessible view is redesigned (for example one `<ol>` of 100-line pages) before Story 1.5.4 starts; R3 and R4 results only set the wording of the known-limits section of the how-to (Task 2.5.1b) and the Task 2.5.4b script.
**Files**: `web-app/src/app/` (throwaway spike page, same page as Story 0.1.5), `web-app/src/components/sessions/__tests__/accessibleViewStructure.test.tsx`, `tests/e2e/files-viewer.spec.ts` (spike section, same file as Task 0.1.4a, header `// @feature files:tabs`), `project_plans/files-viewer-revamp/implementation/manual-verification-checklist.md` (spike notes section)

##### Task 0.1.6a: Prototype Accessible view, accessibility-tree test and spike page variant (S)
- Build the throwaway prototype (it becomes the seed of Task 1.5.4a), the jest structure test (500 lines per page, line-number text, pager labels) and the Playwright accessibility-tree check described above (spike section of `files-viewer.spec.ts`).
- Files: `web-app/src/app/` (throwaway spike page), `web-app/src/components/sessions/__tests__/accessibleViewStructure.test.tsx`, `tests/e2e/files-viewer.spec.ts`

##### Task 0.1.6b: Owner VoiceOver/TalkBack check against criteria R1-R4, recorded (XS)
- Wall-clock blocker (owner), run in the same sitting as Task 0.1.5b; results into the checklist spike notes and ADR-033.
- Files: `project_plans/files-viewer-revamp/implementation/manual-verification-checklist.md`

#### Story 0.1.7: Transport measurement spikes S-T1 and S-T2 (ADR-036; before Epic 1.6)
**As a** developer, **I want** compression, hash and splice costs measured on the real stack, **so that** the size tiers, chunk sizes and the 64 MiB / 1 GiB ceilings of ADR-036 are facts and not proposals.
**Acceptance Criteria**:
- S-T1: what a range response costs on the wire (VERIFIED input: `middleware.Compress` passes every `/api/` path through uncompressed, `server/middleware/gzip.go:155-163`; whether connect-web negotiates Connect-level gzip is UNVERIFIED).
  - *Given* a throwaway unary RPC on the real handler stack returning a 128 KiB and a 1 MiB text payload, *When* called through `getConnectTransport()` over `:62871`, `:62872` and the owner's phone path (Tailscale or LAN), *Then* ADR-036 "Spike outcomes" records the response size on the wire, whether any `content-encoding` was negotiated, and the latency, and states the decision: Connect-level gzip as is, per-RPC compression for range responses over 1 KiB (without touching the `/api/` bypass), or none; the same is recorded for the WebSocket path (permessage-deflate availability in the WS library, UNVERIFIED until run).
- S-T2: throughput of the index-plus-sha256 pass and of the streaming splice (decides `hash_cap_bytes` and the read ceiling).
  - *Given* generated files of 1, 16, 64 and 256 MB (LF, CRLF and a 200-char-line variant, deterministic seed), *When* the Go benchmark runs the prototype single-pass index+sha256 and the prototype streaming splice (read old, write prefix, insert, suffix to a temp file, hash old and new in one pass), *Then* ADR-036 records ns/op, MB/s and `B/op` for each size on the named host, the free-space and temp-file behavior, a 64 MiB literal scan of the `SearchContent` scanner with the cap raised, and the proposed constants are confirmed or replaced (`hash_cap_bytes` 64 MiB, read ceiling 1 GiB, scan time budget); benchmarks run with `&` per repo rule and the numbers are never asserted in CI.
**Files**: `server/services/transport_spike_test.go`, `server/services/transport_bench_test.go`, `web-app/src/lib/files/__tests__/transportCompressionSpike.test.ts`, `project_plans/files-viewer-revamp/decisions/ADR-036-pluggable-range-and-delta-file-transport.md`

##### Task 0.1.7a: S-T1 compression and wire-cost spike (S)
- Throwaway unary handler (same registration route and middleware chain as `SessionService`, like the Task 0.1.1b streaming handler) plus a jest test that inspects the negotiated `content-encoding` and response size through the real transport factory; the WS permessage-deflate probe is recorded from the library docs and one live run. Outcome recorded in ADR-036; the throwaway handler is deleted before merge.
- Files: `server/services/transport_spike_test.go`, `web-app/src/lib/files/__tests__/transportCompressionSpike.test.ts`, `project_plans/files-viewer-revamp/decisions/ADR-036-pluggable-range-and-delta-file-transport.md`

##### Task 0.1.7b: S-T2 index/hash/splice throughput and 64 MiB scan benchmarks (S)
- `transport_bench_test.go`: generators, prototype single-pass index+sha256, prototype streaming splice, `-benchmem`, run with `&`; the 64 MiB scan measures the scanner with the per-file cap lifted. Runs after Task 0.1.7a (both append to `transport_spike_test.go` and ADR-036). Findings set the constants Story 1.6.1 and Story 2.1.5 build to.
- Files: `server/services/transport_bench_test.go`, `server/services/transport_spike_test.go`, `project_plans/files-viewer-revamp/decisions/ADR-036-pluggable-range-and-delta-file-transport.md`

#### Story 0.1.8: Delta encoder and connection-pool contention spikes S-T4 and S-T5 (ADR-036; before Epic 1.6 and Story 2.2.5)
**As a** developer, **I want** the ChangeSet-to-edit encoder proven against a full-replace oracle and the HTTP/1.1 connection-pool contention measured before any server code depends on them, **so that** the delta wire and the choice of extra adapters rest on measurements.
**Acceptance Criteria**:
- S-T4: encoder prototype (the principal residual risk of ADR-036: a valid but wrong edit whose client-computed checksums derive from the same wrong position).
  - *Given* a seeded property harness (fast-check) generating base texts (LF, CRLF, BOM, multibyte, combining marks, astral characters) and random CM6 `ChangeSet`s (including adjacent edits, edits at 0 and EOF, insert-only and delete-only), *When* the prototype encoder converts each `ChangeSet` to `Edit{start{line,col_utf16}, end{...}, insert, deleted_crc32c}` in base coordinates and a reference applier (a byte-level TS function mirroring the server rules: line/col to bytes, surrogate-pair rejection, EOL and BOM owned by the applier) applies them, *Then* the result equals the full-replace result (`applyChanges(base)` then EOL/BOM re-applied) bytewise for 10,000 cases, a column that splits a surrogate pair is rejected, and the failing seed is printed.
- S-T5: connection-pool contention (the assumption of 6 HTTP/1.1 connections per origin is UNVERIFIED per browser).
  - *Given* a Playwright run (desktop Chromium and the Task 0.1.4b mobile project) against the isolated server holding open the Task 0.1.1 test stream, two other tabs prefetching ranges (the S-T1 throwaway unary endpoint) and one tab issuing the unary calls of a save, *When* up to 8 prefetches are in flight, *Then* the p50/p95 latency of the save-shaped unary call and the number of queued requests are recorded in ADR-036; the decision rule is written there: if p95 of the save-shaped call exceeds 2 s under the planned caps (2 ranges per buffer, 4 per tab, unary only), the Phase D adapters (Epic 2.6) are enabled by default in the capability ladder, otherwise they ship but are advertised off (Epic 2.6 gate). The real `SearchContent` and range load is re-measured by Task 1.6.7b.
**Files**: `web-app/src/lib/files/__tests__/changeSetEncoderSpike.test.ts`, `web-app/src/lib/files/transport/encodeChanges.spike.ts`, `web-app/src/lib/files/__tests__/poolContentionSpike.test.ts`, `tests/e2e/files-transport-spike.spec.ts`, `project_plans/files-viewer-revamp/decisions/ADR-036-pluggable-range-and-delta-file-transport.md`

##### Task 0.1.8a: S-T4 ChangeSet-to-edits encoder prototype and property harness (S)
- Prototype `encodeChanges.spike.ts` (promoted to `encodeChanges.ts` by Task 2.2.5a), the TS reference applier and the fast-check harness; outcome and the seed corpus recorded in ADR-036; the golden-vector JSON schema for `testdata/transport/*.json` is fixed here and consumed by Task 2.1.5f.
- Files: `web-app/src/lib/files/__tests__/changeSetEncoderSpike.test.ts`, `web-app/src/lib/files/transport/encodeChanges.spike.ts`, `project_plans/files-viewer-revamp/decisions/ADR-036-pluggable-range-and-delta-file-transport.md`

##### Task 0.1.8b: S-T5 connection-pool contention spike (S)
- A separate spec (`files-transport-spike.spec.ts`, kept as the Phase D regression test) so it does not join the Wave 1 `files-viewer.spec.ts` order; uses the throwaway endpoints of Tasks 0.1.1b and 0.1.7a; results and the decision rule go into ADR-036. Runs after Task 0.1.7a (it reuses that endpoint) and last among the ADR-036 editors of Wave 1 before Task 0.1.4d.
- Files: `tests/e2e/files-transport-spike.spec.ts`, `web-app/src/lib/files/__tests__/poolContentionSpike.test.ts`, `project_plans/files-viewer-revamp/decisions/ADR-036-pluggable-range-and-delta-file-transport.md`

## Epic 0.2: Shared server foundations (refactor-first)
**Goal**: One symlink-safe resolver, one request preamble, flags, a tested read/write codec, and the per-wave gate harness before any new RPC exists.

#### Story 0.2.1: `resolveInWorktree` and read-RPC retrofit (separate commit)
**As a** security-conscious owner, **I want** every file RPC confined by one symlink-safe resolver, **so that** neither reads nor writes can escape the worktree.
**Acceptance Criteria**:
- Symlink escape is rejected on read (closes the latent hole at `file_service.go:106-116`).
  - *Given* a worktree containing `leak -> /etc/hostname` and `SessionId` `s1`, *When* `GetFileContent(s1, "leak")` is called, *Then* it returns `PermissionDenied` with a typed `ReadOnlyReason=Symlink` detail that carries the link's target string (a `Readlink`, metadata only), and no content bytes.
- Legitimate symlinks keep working (pre-mortem #3: read regression from the retrofit).
  - *Given* in-tree links `docs/alias.md -> ../README.md` and `AGENTS.md -> CLAUDE.md` (both resolve inside the root and never through `.git`), *When* `GetFileContent` and `ListFiles` run, *Then* the file content is returned with `ReadOnlyReason=Symlink` (viewable, not editable; the tree marks it as a link), and an in-tree directory symlink `shared -> pkg/shared` lists and reads through `os.Root` without escaping; a link that resolves outside the root or into `.git` is the only case refused.
- The policy is chosen from the owner's real worktrees, not from fixtures.
  - *Given* the implementer's inventory (Task 0.2.1d) of symlinks in the live worktrees, *When* the retrofit lands, *Then* every link class found there is covered by a named test case or listed in ADR-031 with its outcome.
- `.git` and path edge cases are denied.
  - *Given* paths `.git/hooks/pre-commit`, `a/../../x`, `/etc/passwd`, `a\x00b`, *When* `resolveInWorktree` runs, *Then* each returns `InvalidArgument` or `PermissionDenied`; `src/a.go` returns `ResolvedPath("src/a.go")`.
- Existing behavior is unchanged for legit paths.
  - *Given* the existing `file_service_test.go` fixtures, *When* the suite runs, *Then* every pre-existing test passes unchanged.
**Files**: `server/services/path_resolver.go`, `server/services/path_resolver_test.go`, `server/services/file_service.go`, `server/services/file_service_test.go`

##### Task 0.2.1a: `path_resolver.go` with `WorktreeRoot` and `ResolvedPath` (M)
- `OpenWorktreeRoot(dir) (*WorktreeRoot, error)` wrapping `os.OpenRoot`; `resolveInWorktree(root, rel) (ResolvedPath, error)` rejecting NUL, `..`, absolute, `.git` component (any depth, incl. worktree `.git` pointer file); `Lstat` leaf and surface `IsSymlink`; `#nosec G304` justified comments.
- **`.git` denial is symlink-safe, not lexical only**: `os.Root` allows symlinks that stay inside the root, so `linkdir -> .git/hooks` then `linkdir/pre-commit` passes a component check. For write mode, `Lstat` every intermediate path component and reject any intermediate symlink (reads may follow in-root symlinks, but the post-resolution path is re-checked for a `.git` component). This matters now that directory-mode sessions are editable and `.git` is a real directory inside the root. Add `linkdir -> .git/hooks` and `linkdir -> .git` to the Story 2.1.4 symlink matrix.
- Files: `server/services/path_resolver.go`, `server/services/path_resolver_test.go`

##### Task 0.2.1b: Shared request preamble (S)
- `resolveFileRequest(ctx, ref FileRef, mode AccessMode)` where `FileRef{SessionID, Path}` is an arg struct (parsed once at the proto boundary) and `AccessMode` is a typed enum `{Read, Write}`; returns `{root, resolved, workspace}`. `Read` uses `ExistingDir` (tree and file reads may fall back to the repo root and report it; `SearchContent` additionally refuses that fallback, see Story 1.1.2). `Write` uses `ActiveDir` and requires `ActiveDir != ""` **and** `ActiveDir == ExistingDir`, else `FailedPrecondition` `WorktreeMissing`. Removes the triple-repeated preamble (dupl gate).
- **Why not `WorktreeDir != ""`** (reviews' concern verified against `session/types.go:325-355`): `WorktreeDir` is `""` for a session working directly in `RepoRoot` (directory mode or created from a pre-existing worktree), so requiring it would make those sessions permanently non-editable with a misleading `WorktreeMissing`. The hazard it was meant to stop is `ExistingDir` silently falling back to the repo root after a worktree is cleaned up; that fallback is exactly the case where `ActiveDir != ExistingDir` (ExistingDir doc: "ActiveDir when that directory exists on disk, else RepoRoot"), so `ActiveDir == ExistingDir` already blocks it. Raw `i.Path` is never read (`Workspace()`/`Snapshot()` only, per `instance-lock-free-reads.md`). Editing directly in the main checkout is allowed by default (see Unresolved Questions).
- Files: `server/services/path_resolver.go`, `server/services/path_resolver_test.go`

##### Task 0.2.1d: Symlink inventory, allowed-in-tree policy and regression tests (S)
- Before Tasks 0.2.1c and 0.2.1e: inventory symlinks in the owner's live worktrees from the manual instance (`find <ActiveDir> -type l`, classify each by `readlink` as in-tree, escaping, or into `.git`; read-only commands, nothing modified); write the classes into ADR-031. Policy: in-tree links (including directory links) read normally and are shown read-only with `ReadOnlyReason=Symlink`; escaping links and links into `.git` return `PermissionDenied` with the `Symlink` detail plus target string and never any bytes outside the root. Add table tests for each class (in-tree file link, in-tree dir link, chained in-tree links, escaping link, link into `.git`, dangling link) against `GetFileContent`, `ListFiles` and the resolver.
- Files: `server/services/path_resolver_test.go`, `server/services/file_service_test.go`, `project_plans/files-viewer-revamp/decisions/ADR-031-writefile-atomicity-confinement-concurrency.md`

##### Task 0.2.1c: Retrofit `GetFileContent` and raw-serve to the resolver (own commit) (M)
- Runs after Task 0.2.1d's inventory and policy and after Task 0.2.4b's extraction (ET-2: order on `file_service.go` is 0.2.4b -> 0.2.1c -> 0.2.1e -> 0.2.3c). Split from the original four-site retrofit (ET-4: it was upper-M/L): this task takes the two byte-returning paths, where the symlink hole actually leaks content. Replace the `resolveAndValidatePath` call sites at `file_service.go:303` (`GetFileContent`) and `:729` (`ServeFileRaw`) with the preamble (VERIFIED by grep in this worktree: the resolver has exactly three production call sites, `:141`, `:303`, `:729`; the earlier `:131,298,460,723` numbers were stale); open files via `root.Open`; add symlink-escape and `.git` read tests for both. Commit message: `fix(files): confine file-content reads with symlink-safe resolver`.
- Files: `server/services/file_service.go`, `server/services/file_service_test.go`

##### Task 0.2.1e: Retrofit `ListFiles` and the tree `SearchFiles` walk to the resolver and `os.Root` (own commit) (S)
- Split from Task 0.2.1c (ET-4). `ListFiles` swaps its `resolveAndValidatePath` call (`file_service.go:141`) for the preamble and lists through the `os.Root` (in-tree directory links list, escaping or `.git` links are refused, per the Story 0.2.1 policy); `SearchFiles` (`:444`) has no resolver call today (it walks `ws.ExistingDir` with `filepath.WalkDir`), so it opens a `WorktreeRoot` for `basePath` and walks `root.FS()` so the tree search cannot follow a link out of the root; `collectAllGitignorePatterns` stays unchanged (separate debt). `session_service_delegates.go` is not edited: it only forwards to `fileSvc` (VERIFIED `session_service_delegates.go:640-653`). Commit message: `fix(files): confine tree listing and tree search with os.Root`.
- Files: `server/services/file_service.go`, `server/services/file_service_test.go`

#### Story 0.2.2: Feature flags
**As an** owner, **I want** `files:editing` (off), `files:viewer-v2` (on) and `files:merge-phone` (on) live-settable, **so that** rollout and rollback need no deploy.
**Acceptance Criteria**:
- Defaults are correct and live-read.
  - *Given* a fresh config, *When* `FilesEditingEnabled(ctx)` and `FilesViewerV2Enabled(ctx)` are read per request, *Then* they return `false` and `true`; after `UpdateFeatureFlag("files:editing", true)` the next call returns `true` without restart.
- Flags are global-only.
  - *Given* the registry entries for `files:editing` and `files:viewer-v2`, *When* `UpdateFeatureFlag` is called with a scope for either (e.g. `kind:review`), *Then* it is rejected (these flags declare no scopes; helper signatures take no session input, matching `ProgramCLIFlagProbeEnabled`). Rollout is one global flip, bounded by the server-side `WriteFile` guards.
- Client sees flags.
  - *Given* `useFilesFlags()` mounted with a mocked `GetFeatureFlags` returning `files:editing=true`, *When* it renders, *Then* it returns `{status: "ready", editing: true, viewerV2: true}`.
- Loading is distinct from off (pre-mortem #12, VERIFIED: `FeatureFlagsContext.tsx:72-96` fetches once on mount; `flags[...]` is `undefined` until then and on error).
  - *Given* the context is still loading, *When* `FilesTab` renders, *Then* `useFilesFlags()` returns `status: "loading"` and `FilesTab` mounts neither the new nor the legacy layout (a neutral skeleton), so there is no legacy-layout flash and no remount; *Given* the fetch errors, *Then* `status: "error"` resolves to the server defaults (`viewerV2: true`, `editing: false`).
  - *Given* a mounted `FilesTab`, *When* `files:viewer-v2` flips, *Then* the layout is not remounted (buffer state is kept); the flag is applied at the next mount, and a `SearchContent` refusal shows the "Search is disabled" state.
- Flags refresh (the context today never re-fetches).
  - *Given* `files:editing` turned off server-side, *When* the window regains focus or visibility (throttled to once per 5 s), or any `WriteFile`/`SearchContent` call returns a `FlagOff` refusal (a typed `ReadOnlyReason=FlagOff` detail, also on `SearchContent`), *Then* `FeatureFlagsContext.refresh()` re-reads `GetFeatureFlags` and the editor drops to read-only within that one refresh, with no page reload.
- Phone-merge kill switch (triad repair).
  - *Given* the registry entry `files:merge-phone` (global-only, default on, no scopes), *When* `FilesMergePhoneEnabled(ctx)` is read and `UpdateFeatureFlag("files:merge-phone", false)` is called, *Then* the next read returns `false` without restart; `useFilesFlags()` also returns `mergePhone` (fails open to `true` on error, because turning merge off is the safe-degrade direction only when the owner chose it).
- Transport flags (ADR-036; all global-only, default off).
  - *Given* the registry entries `files:range-read`, `files:delta-write` and `files:edit-large` (no scopes), *When* `FilesRangeReadEnabled(ctx)`, `FilesDeltaWriteEnabled(ctx)` and `FilesEditLargeEnabled(ctx)` are read per request on a fresh config, *Then* each returns `false`, `UpdateFeatureFlag` flips each live without restart, a scoped update is rejected, and `useFilesFlags()` also returns `rangeRead`, `deltaWrite` and `editLarge` (each failing closed to `false` on a flag-fetch error, because turning these off is the safe direction).
**Files**: `server/services/feature_flag_service.go`, `server/services/feature_flag_service_test.go`, `web-app/src/lib/hooks/useFilesFlags.ts`, `web-app/src/lib/hooks/__tests__/useFilesFlags.test.ts`

##### Task 0.2.2a: Server registry + helpers (S)
- Add the six entries (`files:editing`, `files:viewer-v2`, `files:merge-phone`, and the ADR-036 `files:range-read`, `files:delta-write`, `files:edit-large`, all default off except `viewer-v2` and `merge-phone`) to `knownFeatureFlags` with no scopes, helper funcs mirroring `ProgramCLIFlagProbeEnabled` (global read, no session argument); update registry-content assertions in `feature_flag_service_test.go`.
- Files: `server/services/feature_flag_service.go`, `server/services/feature_flag_service_test.go`

##### Task 0.2.2b: `useFilesFlags` hook and `FeatureFlagsContext.refresh()` (S)
- Wrap existing `GetFeatureFlags` plumbing; return a `status` of `loading | ready | error` with `editing`/`viewerV2` and the ADR-036 `rangeRead`/`deltaWrite`/`editLarge` (the three fail closed to false on error; editing fails closed to false on error; `viewerV2` takes the server default `true`). Add `refresh()` to `FeatureFlagsContext` (a `fetchFlags` re-read that does not set `isLoading`), call it on window `focus`/`visibilitychange` (5 s throttle) and expose a helper that `useSaveBuffer`/`useContentSearch` call when they receive a `FlagOff` refusal. Existing `FeatureFlagsContext` consumers stay unchanged.
- Files: `web-app/src/lib/contexts/FeatureFlagsContext.tsx`, `web-app/src/lib/hooks/useFilesFlags.ts`, `web-app/src/lib/hooks/__tests__/useFilesFlags.test.ts`

#### Story 0.2.3: Read-side codec, version token, StatFile
**As a** user, **I want** every file read to carry a version token, line-ending/BOM facts and an explicit read-only reason, **so that** the UI can edit safely and explain why not.
**Acceptance Criteria**:
- Token and metadata on read.
  - *Given* a 120-byte UTF-8 CRLF file with BOM at `docs/a.md`, *When* `GetFileContent` returns, *Then* `version_token` parses as `v1:120:<mtime_ns>:<64-hex>`, `line_ending=CRLF`, `has_bom=true`, `read_only_reason=None`, and `content` has the BOM stripped.
- Non-editable reads carry no editable token.
  - *Given* a 2 MB text file, *When* read, *Then* `is_truncated=true`, `read_only_reason=Truncated`, token hash segment is `-`, and the cut is on a UTF-8 rune boundary.
- Reasons are exhaustive.
  - *Given* fixtures binary (NUL), latin-1 bytes `0xE9`, mixed LF/CRLF, an otherwise-LF file with one lone `\r` (`lone_cr.txt`), in-tree symlink, *When* read, *Then* reasons are `Binary`, `NonUtf8`, `MixedEol`, `MixedEol` (lone `\r`), `Symlink` respectively.
- `StatFile` is cheap.
  - *Given* `docs/a.md` unchanged, *When* `StatFile` is called, *Then* it returns the same token without reading content beyond hashing, and a changed file returns a different token.
- Later extensions are additive (ADR-036).
  - *Given* `StatFileRequest`, *When* Task 0.2.6a adds `want_analysis` and `want_sha256` (and `StatFileResponse.analysis`), *Then* this story's `StatFile` behavior and tests are unchanged (the new fields default to the cheap path); `GetFileContent` is delegated to `FileOps` by Task 0.2.6c after Task 0.2.3c with byte-identical responses.
**Files**: `proto/session/v1/session.proto`, `server/services/file_codec.go`, `server/services/file_codec_test.go`, `server/services/file_service.go`, `server/services/files_delegates.go`

##### Task 0.2.3a: Proto additions (S)
- `GetFileContentResponse`: `version_token`, `line_ending`, `has_bom`, `read_only_reason` (enum). New `StatFile(StatFileRequest{session_id,path}) returns (StatFileResponse{version_token,size,read_only_reason})`. `make proto-gen`; commit only the `.proto`.
- Files: `proto/session/v1/session.proto`

##### Task 0.2.3b: `file_codec.go` pure functions (M)
- `NewFileVersionToken`, `ParseFileVersionToken`, `detectLineEnding` (a lone `\r` not followed by `\n` anywhere makes the file `Mixed`, hence read-only `MixedEol`, so the LF-normalize/re-apply round trip can never change bytes), `detectBOM`, `classifyReadOnly`, `truncateOnRuneBoundary`, strict `validateEditableUTF8` (NUL + `utf8.Valid`). Table + fuzz tests.
- Files: `server/services/file_codec.go`, `server/services/file_codec_test.go`

##### Task 0.2.3c: Wire into `GetFileContent` and add `StatFile` (M)
- Add through helpers only (no function growth); delegate `StatFile` in the new `files_delegates.go`; `// +api: files:stat` marker; run `make registry-generate`.
- Files: `server/services/file_service.go`, `server/services/files_delegates.go`, `docs/registry/features/files-stat.json`

#### Story 0.2.4: Per-wave gate checkpoints (pre-mortem #13)
**As a** maintainer, **I want** the jscpd, complexity and size gates to run at the end of every wave, **so that** a failure is found while the owning story's context is fresh, not after about 90 tasks and about 20 heavily mocked new test files.
**Acceptance Criteria**:
- One command runs the gates.
  - *Given* the repo, *When* `make files-viewer-gates` runs, *Then* it runs `pnpm run lint:duplicates` (in `web-app/`, jscpd threshold 0.14%), `make ready-complexity-gate` (dupl, gocognit, funlen over the new-from-`origin/main` diff) and the 350-line budget test, and prints each result; the end of every wave in the wave plan (Waves 2 to 10) runs it and records the output in the PR description.
- Test-mock duplication is controlled from the first file.
  - *Given* the first new jest test file, *When* it needs a fake Connect transport or `CodeEditor` fixture, *Then* it imports them from shared fixture modules under `web-app/src/lib/files/__tests__/fixtures/` (transport, editor, session) rather than repeating `jest.mock(...)` blocks per file.
- The jscpd margin is thin and is managed with one harness, a guard test and a per-wave ledger (engineering triad ET-9; the 0.14% ratchet is `web-app/.jscpd.json`, and `CLAUDE.md` records that heavily mocked test files grow the baseline by a small, irreducible amount each time; the plan names 30 distinct jest test files, several heavily mocked; the current measured percentage is UNVERIFIED here because `web-app/node_modules` is not installed, so Task 0.2.4e records it first).
  - *Given* `createFilesTestHarness(overrides)` in `fixtures/filesHarness.ts` (one factory that wires the fake Connect transport, the `CodeEditor` mock, the session and `FeatureFlagsContext` providers and the fake analytics provider of the privacy harness), *When* any new jest test under `web-app/src/lib/files/**` or `web-app/src/components/sessions/files/**` needs those collaborators, *Then* it calls the factory; a guard test `noDuplicateMocks.test.ts` fails when a new test file under those paths contains a `jest.mock(` of a module the harness already mocks (the list is a constant in the fixture).
  - *Given* each wave gate, *When* `make files-viewer-gates` runs, *Then* it prints the jscpd percentage and the headroom to 0.14% and the PR description keeps a one-line-per-wave ledger; if headroom is below 0.02 percentage points at any gate, the next wave's first task is a fixture extraction, and raising the threshold is last resort and an explicit owner decision (it has been raised twice already, per `CLAUDE.md`), never a silent edit.
- Go race-suite budget is protected (engineering triad ET-9; BUG-067, `docs/bugs/open/BUG-067-server-services-race-suite-exceeds-150s-ci-timeout.md`, still open with root cause "Unknown").
  - Facts. VERIFIED: the bug's 150 s ceiling is stale for the MCP workflow: `.github/workflows/mcp-integration.yml` now runs the `server/services` leg **without** `-race` at `-timeout=1200s` (its comment cites BUG-083/BUG-099 and about 2.9 s per `createTestStorage` call under `-race`). The live exposure is `.github/workflows/build.yml:275` (`-race -short -timeout=22m`, affected packages or the full-suite fallback on every PR) and `Makefile:689-690` (`test-race`, `-timeout=20m`). The bug's 2026-08-27 entry records a 320.7 s CI run of `server/services` against about 74 s locally (whether that run was race-enabled is INFERRED). The plan adds 12 new `_test.go` files to `server/services` (13 with the hygiene guard of Task 0.2.4c) (the grep of the plan's Files lines: `content_search_test`, `content_search_handler_test`, `content_search_fixture_test`, `content_search_bench_test`, `file_atomic_test`, `file_codec_test`, `file_write_test`, `file_write_corpus_test`, `files_telemetry_test`, `os_root_semantics_test`, `path_resolver_test`, `streaming_spike_test`; the last is throwaway, two are fixture/benchmark helpers) plus extensions to `file_service_test.go` and `feature_flag_service_test.go`, with `-race` concurrency tests (50-goroutine registry race, `goleak`, 50k-file cancellation). Impact: every one of them is compiled into the same `server/services` test binary and run under the same `-race -short` budget that BUG-067 says is marginal, so unscoped growth is the risk, not any single test.
  - *Given* the new Go tests, *When* they are written, *Then* none calls `createTestStorage`, starts tmux or a session driver (they use `t.TempDir()` worktrees, fake ports and the in-process `ServerStream` fake, per the Test Stack in validation.md); every large-fixture test uses a 5,000-file variant of the same shape when `testing.Short()` is true (CI runs `-short`) and the full 50k fixture only in the benchmark and `make files-viewer-perf` (non-race); and `make files-viewer-go-tests` runs only the files-viewer tests with `-race` through a `-run` regex (a fixed list of name prefixes kept in the Makefile), so a worker iterates without paying for the other 280 test files.
  - *Given* Task 0.2.4c's baseline (`go test -race -short ./server/services` wall-clock and the five slowest tests from `gotestsum` output, measured on `origin/main` at the branch's merge base, so the Wave 1 spike tests already count in the delta), *When* each wave gate runs, *Then* it prints the package wall-clock and the delta to that baseline; the budget is that the files-viewer tests add no more than 10% to the baseline (a proposed threshold, INFERRED, to be confirmed or replaced by the measured baseline); if the Wave 4 or Wave 5 gate exceeds it, conditional Task 0.2.4d moves the pure, SessionService-independent units (`path_resolver.go`, `file_codec.go`, `file_atomic.go`, the `content_search*.go` walker, matcher and ignore files, with their tests) into a new leaf package `server/services/filesfs` so `go test ./...` runs them in parallel with `server/services` instead of inside its binary.
  - *Given* the baseline and the Wave 5 numbers, *When* Task 0.2.4c finishes, *Then* a dated note is appended to BUG-067 stating the stale facts above and the measurement (collateral-debt hygiene; the bug stays open, this plan does not claim to fix its unknown root cause).
- `server/server.go` length is probed at the Wave 4 gate, before three tasks add lines to it (triad round 2).
  - *Facts (VERIFIED)*: `server/server.go` is 2,172 lines; `.golangci.yml:47-51` enables `revive(file-length-limit)` at 1,000 lines (skipComments, skipBlankLines) and `.github/workflows/lint.yml` scopes it to new issues; tasks 2.1.2d (guard entry and route), 2.1.2f (body-limit registration) and 2.5.3a (analytics route) edit `server/server.go` in Waves 5 and 9. Whether an edit to a file already over the limit counts as a "new issue" is UNVERIFIED.
  - *Given* the Wave 4 gate, *When* `make files-viewer-gates` runs, *Then* it records the non-blank, non-comment line count of `server/server.go` and runs `golangci-lint run --enable revive --new-from-rev=origin/main ./server/` after a trivial one-line comment edit to `server/server.go` (reverted), printing whether `file-length-limit` is reported; if it is, conditional Task 0.2.4f runs before Wave 5 and the later tasks edit the new file instead.
- `GetFileContent` headroom is known before it is extended.
  - *Given* `GetFileContent` (about 140 lines against funlen 150 and statements 100, `.golangci.yml:38-43`), *When* Task 0.2.4b runs, *Then* its current funlen/gocognit headroom is recorded, and the extraction of helpers into `file_codec.go` is done before Task 0.2.3c adds any code to it.
**Files**: `Makefile`, `web-app/src/lib/files/__tests__/fixtures/`, `server/services/file_service.go` (extraction only), `docs/bugs/open/BUG-067-server-services-race-suite-exceeds-150s-ci-timeout.md` (dated note only)

##### Task 0.2.4a: `make files-viewer-gates` target and shared jest fixture modules (S)
- Thin Make target composing the existing gates (no new tooling); shared fixtures module skeleton that later stories extend.
- Files: `Makefile`, `web-app/src/lib/files/__tests__/fixtures/`

##### Task 0.2.4b: Record `GetFileContent` headroom and extract before extending (XS)
- Run the linters on `GetFileContent`, write the numbers into the PR description, and extract the truncation, binary-sniff and encoding blocks into `file_codec.go` helpers as a behavior-preserving commit. Runs first on `file_service.go` in Wave 2 (before Task 0.2.1c; ET-2).
- Files: `server/services/file_service.go`, `server/services/file_codec.go`

##### Task 0.2.4c: Race-suite baseline, scoped Make targets and the BUG-067 note (S)
- Measure on a clean checkout of `origin/main` at the branch's merge base (Wave 1 already adds Go tests, so the branch itself is not a baseline): record `go test -race -short ./server/services` wall-clock and the five slowest tests (use `gotestsum --format testname` timing or `go test -json`); add `make files-viewer-go-tests` (`-race -short -run` over the fixed prefix list) and `make files-viewer-perf` (non-race, full 50k fixtures) to the Makefile; extend `make files-viewer-gates` to print package wall-clock and the delta to the baseline; append the dated note to BUG-067.
- Also add `server/services/files_test_hygiene_test.go`, a guard that scans the fixed list of files-viewer test files for `createTestStorage(`, tmux or session-driver construction and for an unguarded 50k fixture call (validation REQ-35).
- Files: `Makefile`, `docs/bugs/open/BUG-067-server-services-race-suite-exceeds-150s-ci-timeout.md`, `server/services/files_test_hygiene_test.go`

##### Task 0.2.4d: Relocate the pure files code to its own package if the race budget is exceeded (S, conditional)
- Runs only if a wave gate shows the files-viewer tests adding more than the Task 0.2.4c budget. Mechanical `git mv` plus package rename of the already written pure units into `server/services/filesfs` (exported names only where `SessionService` or `FileService` consume them), imports updated, no behavior change; if the move would touch more than five files it is split per directory (resolver and codec; search; atomic write). Skipped, and recorded as skipped in the PR description, when the budget holds. Counted in the totals so the plan does not under-cost the branch.
- Files: `server/services/path_resolver.go`, `server/services/file_codec.go`, `server/services/file_atomic.go`, `server/services/content_search.go`, `server/services/content_search_match.go`

##### Task 0.2.4f: Carve `guardedProcedures` out of `server/server.go` if the length probe reports (S, conditional)
- Only if the Wave 4 probe shows `revive` reporting `file-length-limit` for an edit to `server/server.go`: move `guardedProcedures`, the `*ProcedurePath` constants and `localChain`/`localChainWith` as a pure, behavior-preserving move (own commit) into `server/guarded_procedures.go` so Tasks 2.1.2d and 2.1.2f add their one-line entries there and `server/server.go` is touched only for route registration; skipped and recorded as skipped when the probe passes. Counted in the totals (same convention as Task 0.2.4d).
- Files: `server/server.go`, `server/guarded_procedures.go`, `server/server_probeguard_test.go`

##### Task 0.2.4e: Shared test harness factory, mock-guard test and the jscpd ledger (S)
- Measure and record the current jscpd percentage (`pnpm run lint:duplicates` in `web-app/`) before the first new test file; add `fixtures/filesHarness.ts` (`createFilesTestHarness`), `fixtures/index.ts` exporting transport/editor/session/flags/analytics fakes, and `noDuplicateMocks.test.ts`; wire the percentage and headroom into `make files-viewer-gates` output. Later stories import the factory instead of writing `jest.mock(` blocks (Story 0.2.5b's privacy harness is an input to it, not a copy).
- Files: `web-app/src/lib/files/__tests__/fixtures/filesHarness.ts`, `web-app/src/lib/files/__tests__/fixtures/index.ts`, `web-app/src/lib/files/__tests__/noDuplicateMocks.test.ts`, `Makefile`


#### Story 0.2.5: Files analytics taxonomy, typed helper and privacy guard (ADR-035)
**As the** owner, **I want** the viewer to emit a small, typed, privacy-safe set of events into the existing analytics system, **so that** I can understand adoption, conflicts, mobile vs desktop use and latency over the long term without ever recording what I searched for or which files I opened.
**Acceptance Criteria**:
- One typed entry point (no stringly-typed names).
  - *Given* the registry in `web-app/src/lib/files/analytics/events.ts`, *When* a call site passes an unknown event name, a label value outside its union, or a `string`-typed label, *Then* `tsc` fails (a `*.typetest.ts` file with `// @ts-expect-error` lines proves each); *Given* the ESLint override for `web-app/src/components/sessions/**` and `web-app/src/lib/files/**`, *When* a file calls `track({ name: "files.` directly, *Then* lint fails; only `web-app/src/lib/files/analytics/` may. The existing `analytics/require-on-click` rule is checked against `useFilesAnalytics().emit` (Task 0.2.5b) so handlers do not need `// analytics-exempt`.
- No PII, proven with canary strings.
  - *Given* canary values (path `secret/dir/Canary.go`, file name `Canary.go`, query `CANARYQUERY`, glob `**/Canary*`, draft text `CANARYDRAFT`, branch `canary-branch`), *When* the privacy test drives every emitter and the real components of Epics 1.3-2.4 (search, open, edit, save, conflict, merge, extra-key flows) against a fake provider, *Then* the JSON of every tracked event (name, labels, component, page, sessionId) contains none of them, every label value matches `^[a-z0-9_+-]{1,24}$`, every event has <= 8 labels and only registry keys, and every label value is a member of its declared enum or bucket set.
- Stamped dimensions.
  - *Given* the helper at a 1280px viewport with a mouse, *When* any event is emitted, *Then* it carries `form=desktop`, `input=hardware`, `editing`, `v2`; *When* the viewport crosses 768px, the `InputProfile` changes or a flag flips, *Then* the next event carries the new values without a remount.
- High-frequency events are capped, and drops are visible.
  - *Given* 100 `files.tab.switch` within 100 ms, *Then* at most one event is emitted (250 ms coalescer); *Given* 50 emits of one name in a minute, *Then* 30 are tracked and one `files.meta.dropped{event,count}` is emitted for the rest; *Given* 40 `extra_key_used` taps of one key class in 30 s, *Then* one aggregated event with a `count` bucket is emitted at window close or `pagehide`. All timing uses injected fake timers.
- Find-and-open interaction meter.
  - *Given* Search opened (1), a query typed (1) and a hit tapped (1), *Then* `files.search.open_hit` carries `interactions=3`; *Given* an extra group-expand tap, *Then* `4`; *Given* the search cancelled or the panel closed, *Then* the meter resets and no `open_hit` is emitted.
- Analytics cannot break the viewer.
  - *Given* `provider.track` throws or no `AnalyticsContext` provider is mounted (as in isolated component tests), *When* any Files action runs, *Then* the action completes and the viewer renders; analytics never blocks, awaits or throws into UI code.
- Server port and handler guard.
  - *Given* the `FilesTelemetry` no-op default, *Then* services construct without an analytics provider; *Given* the adapter over a fake `analytics.AnalyticsProvider`, *When* `RecordWrite(conflict_rejected)` is called, *Then* exactly one `analytics.Event{EventName:"files.server.write", EventCategory:"rpc", SessionID, DurationMs, Labels}` is recorded with enum/bucket labels only, a `Record` error is logged at Warn and not returned, and the call uses a 500 ms detached context; *Given* `POST /api/analytics` with a batch containing a `files.edit.save` event whose label value is `src/a.go` (or with 9 labels, or an uppercase/space value), *Then* that event is dropped, the other events in the batch are recorded, the response is `204`, and the Warn log carries counts but no label values; non-`files.` events behave exactly as before (existing `analytics_handler_test.go` stays green).
- Transport labels and events (ADR-036 section 16, ADR-035 "Transport extension"; Task 0.2.5d).
  - *Given* the registry, *When* it is compiled, *Then* it contains the closed-enum labels `transport` {full,connect_unary,connect_stream,ws_stream,http_range}, `fallback_reason` {network,unavailable,deadline,malformed,capability,version}, `payload` {lt1k,1k-10k,10k-100k,100k-1m,ge1m}, `ops` {1,2-9,10-99,100-999,1000+} and `tier` {t0,t1,t2}, and the events `files.transport.select`, `files.transport.fallback`, `files.read.window` (client, sampled), `files.edit.patch_rejected`, `files.edit.preflight` and the server event `files.server.patch` (unthrottled, mirrors `files.server.write`); *When* the canary privacy test drives a windowed read, a patch save, a fallback, a rejected patch and a preflight, *Then* no tracked event contains a path, offset, line number, byte count, text or query, every value matches `^[a-z0-9_+-]{1,24}$`, and a `string`-typed label or a numeric offset label fails `tsc` (type tests).
**Files**: `web-app/src/lib/files/analytics/events.ts`, `web-app/src/lib/files/analytics/filesAnalytics.ts`, `web-app/src/lib/files/analytics/sampling.ts`, `web-app/src/lib/files/analytics/findFlowMeter.ts`, `web-app/src/lib/files/analytics/__tests__/`, `web-app/.eslintrc*` (override), `server/services/files_telemetry.go`, `server/services/files_telemetry_test.go`, `server/handlers/analytics_handler.go`, `server/handlers/analytics_handler_test.go`

##### Task 0.2.5a: Event registry, bucket functions, `useFilesAnalytics()`, sampling and `FindFlowMeter` (M)
- `events.ts` is generated by hand once and is the single source (the ADR-035 table is checked against it by a jest test that parses the ADR); `filesAnalytics.ts` wraps `useAnalytics()` (memoised, never throws); `sampling.ts` has the token bucket, coalescer and window aggregator with injected clock; `extClass(path)` returns only a class.
- Files: `web-app/src/lib/files/analytics/events.ts`, `web-app/src/lib/files/analytics/filesAnalytics.ts`, `web-app/src/lib/files/analytics/sampling.ts`, `web-app/src/lib/files/analytics/findFlowMeter.ts`

##### Task 0.2.5b: Type tests, canary privacy harness (shared fixture), sampling tests and ESLint compatibility (S)
- Privacy harness lives under `web-app/src/lib/files/__tests__/fixtures/` (Story 0.2.4a) so later stories import it instead of duplicating `jest.mock` blocks; ESLint `no-restricted-syntax` override; verify `analytics/require-on-click` accepts the helper (extend the rule's callee list in `web-app/eslint-plugin-analytics/rules/` if not, rather than adding `// analytics-exempt` comments).
- Files: `web-app/src/lib/files/analytics/__tests__/`, `web-app/src/lib/files/__tests__/fixtures/`, `web-app/eslint-plugin-analytics/rules/`

##### Task 0.2.5c: `FilesTelemetry` port, adapter, wiring and the `files.` label-shape guard in the analytics handler (M)
- Port and adapter in `files_telemetry.go` (enum-typed outcome structs; no-op default; adapter calls `AnalyticsProvider.Record` after the response is decided with a 500 ms detached context); wired through `SessionService` dependencies into `FileWriter` (Task 2.1.2b port list gains `FilesTelemetry`) and the search handler; the handler guard drops violating `files.*` events (not the whole batch) and logs counts only. Serialize on `server/server.go` / dependencies with 2.1.2d.
- Files: `server/services/files_telemetry.go`, `server/services/files_telemetry_test.go`, `server/handlers/analytics_handler.go`, `server/handlers/analytics_handler_test.go`, `server/dependencies.go`

##### Task 0.2.5d: Transport labels, events and the server patch outcome in the registry (S; ADR-036)
- Adds the labels and events of the AC above to `events.ts` (bucket functions `bucketPayload`, `bucketOps`; `tier` from the buffer source, also stamped on `files.perf.open_ms`), the ADR-035 "Transport extension" table (checked against `events.ts` by the existing jest test that parses the ADR), the `PatchOutcomeEvent` struct and `RecordPatch` method on the `FilesTelemetry` port (enum-typed, no path/offset field, no-op default; the adapter emits `files.server.patch` with `outcome` {ok,conflict,offset_mismatch,result_mismatch,out_of_order,analysis_pending,too_large,rejected,disabled,error}, `tier`, `transport`, `payload`, `ops`), and the `files.` label-shape guard needs no change (values fit the existing shape). Runs after Tasks 0.2.5a and 0.2.5c (same files); emitting call sites arrive in Epics 1.6 and 2.x.
- Files: `web-app/src/lib/files/analytics/events.ts`, `web-app/src/lib/files/analytics/__tests__/`, `server/services/files_telemetry.go`, `server/services/files_telemetry_test.go`, `project_plans/files-viewer-revamp/decisions/ADR-035-files-analytics-taxonomy-and-privacy.md`

#### Story 0.2.6: `FileOps`/`FileBackend` ports, `GetFileTransportCapabilities` and transport flags (ADR-036 Phase A; behavior-identical)
**As a** developer, **I want** every file policy (confinement, flags, guards, token rules, limits, audit) held in one application service over a storage port, with a capability RPC that tells each client what this listener really supports, **so that** transports are swappable adapters that cannot weaken ADR-031 and a stale client falls back to today's behavior.
**Acceptance Criteria**:
- Behavior-identical refactor (Phase A ships inside Phase 0 with no visible change).
  - *Given* the existing `file_service_test.go`, the Story 0.2.3 token/reason corpus and the Story 0.2.1 symlink corpus, *When* `GetFileContent` and `StatFile` run through `FileOps` over `osRootBackend`, *Then* every existing assertion passes unchanged and the responses are byte-identical to the pre-refactor ones; `FileService` keeps its constructor and the policy code (resolver call, flags, guard, token, audit) exists once, in `FileOps`.
- The storage port is substitutable.
  - *Given* `memBackend`, *When* `FileOpsConformance` (scaffold here, cases added by Stories 1.6.2 and 2.1.5) runs over both backends, *Then* both pass the same cases; *Given* a `FileOps` constructed with a backend that panics on any path outside the resolver's output, *Then* no test reaches it with an unresolved path (confinement is single-sourced).
- Capabilities are truthful per listener and per flag.
  - *Given* the stamped request record for `ListenerLocal` and for `ListenerRemote` (VERIFIED mechanism: `remoteChain` calls `stampRequest(services.ListenerRemote, ...)`), *When* `GetFileTransportCapabilities` is called with `files:range-read`, `files:delta-write`, `files:edit-large` and `files:editing` in each combination, *Then* `ops` and `wires` list exactly what that listener serves today (this story: `STAT`, `READ_ALL`, `WRITE_FULL` when `files:editing` is on, wire `CONNECT_UNARY`; no `PATCH` on `:8444` until Task 0.1.3b decides remote write reachability, and when that recon says a read-scoped remote caller can reach writes, `PATCH` is also omitted for that tier), `limits` carries `max_write_full_bytes=1.5 MiB` and `hash_cap_bytes=1 MiB` until Task 1.6.2b raises the range limits and `hash_cap_bytes` when `files:range-read` is on, and `caps_epoch` increases when any of the four flags changes; an advertised op works and an unadvertised op is refused with `FailedPrecondition` (capability honesty test).
  - *Given* an older server that returns `Unimplemented` for the RPC, *Then* the client treats it as `{ops: [STAT, READ_ALL, WRITE_FULL]}` (tested in Story 1.2.3); the RPC is read-only and not in `guardedProcedures` (it mutates nothing; asserted by the map-membership test staying unchanged).
- Flags are registered global-only, live-settable, off by default (owner-flagged default; see ADR-036).
  - *Given* a fresh config, *When* `FilesRangeReadEnabled(ctx)`, `FilesDeltaWriteEnabled(ctx)` and `FilesEditLargeEnabled(ctx)` are read, *Then* each returns `false` without restart-required wiring and `UpdateFeatureFlag` flips each live; a scoped update is rejected (no scopes declared; Task 0.2.2a).
- Server analytics and logs are unchanged in this story.
  - *Given* the Phase A refactor, *When* `GetFileContent` runs, *Then* the existing slog lines and `FilesTelemetry` events are emitted exactly once as before (no duplicate from the new layer).
**Files**: `proto/session/v1/session.proto`, `server/services/file_backend.go`, `server/services/file_ops.go`, `server/services/file_ops_capabilities.go`, `server/services/file_ops_conformance_test.go`

##### Task 0.2.6a: `GetFileTransportCapabilities` proto (S)
- Messages `FileTransportCapabilities{protocol_version, features[], ops[], wires[], limits{max_range_bytes, max_range_lines, preferred_chunk_bytes, max_patch_ops, max_patch_body_bytes, max_insert_bytes, max_write_full_bytes, hash_cap_bytes, max_concurrent_ranges}, caps_epoch}` and enums `Op`, `Wire`; unary RPC `GetFileTransportCapabilities(session_id)`; additive fields on `StatFileRequest` (`want_analysis`, `want_sha256`) and `StatFileResponse` (`analysis` state) so the later stories need no second proto edit of Story 0.2.3's messages. `make proto-gen`; commit only the `.proto`. Ordered after Task 0.2.3a in `session.proto`.
- Files: `proto/session/v1/session.proto`

##### Task 0.2.6b: `FileBackend` port, `osRootBackend` and `memBackend` (M)
- Go interface `FileBackend` (`Open` read-only non-symlink handle over the `os.Root`, `Stat`, `ReadAt`, later `Analyze` and `Splice`), `osRootBackend` wrapping the Story 0.2.1 `WorktreeRoot`/`os.Root` calls and the Story 2.1.1 atomic helpers by composition (no policy inside), and an in-memory backend for tests with injectable faults (short read, ENOSPC, rename failure) used by the later fault-injection rows. No behavior change.
- Files: `server/services/file_backend.go`, `server/services/file_backend_osroot.go`, `server/services/file_backend_mem.go`, `server/services/file_backend_test.go`

##### Task 0.2.6c: `FileOps` application service, capability computation and thin wire handlers (M)
- `FileOps` holds `FilePolicy` (flags port, hidden-session `WriteGuard`, `WorkspaceProvider`, `FilesTelemetry`, resolver, per-path mutex registry from Task 2.1.1b when it lands) and the read/stat operations moved out of `file_service.go` behind helpers; `GetFileContent`, `StatFile` and the new capabilities handler become thin wire adapters in `files_delegates.go`; capabilities are computed from the stamped listener and the live flags with `caps_epoch` derived from a flag-version counter. `file_service.go` is edited for delegation only, after Task 0.2.3c (the last editor of that file in Wave 3). Story 2.1.2's `FileWriter` becomes the write-side collaborator of `FileOps` (it consumes this policy and backend instead of declaring its own ports; Task 2.1.2b is amended).
- Files: `server/services/file_ops.go`, `server/services/file_ops_capabilities.go`, `server/services/files_delegates.go`, `server/services/file_service.go`, `server/services/file_ops_test.go`

##### Task 0.2.6d: `FileOpsConformance` scaffold, capability honesty and per-listener tests (S)
- `FileOpsConformance(t, newOps)` runs table cases over both backends (this task adds the read-all/stat/capability cases and the confinement corpus reuse of Story 0.2.1; Story 1.6.2 adds range cases and Story 2.1.5 adds patch cases); per-listener capability tests drive the real `localChain` and `remoteChain` stamps.
- Files: `server/services/file_ops_conformance_test.go`, `server/services/file_ops_capabilities_test.go`


---

# Phase 1: Read-only value (ships first)

## Epic 1.1: Content search backend
**Goal**: Project-wide regex/literal search, streaming, capped, cancellable, honoring gitignore.

#### Story 1.1.1: Proto, `ContentSearcher`, in-process walker
**As a** user, **I want** grep-like search across the worktree, **so that** I can find where something is used.
**Acceptance Criteria**:
- Literal and regex matching with UTF-16 columns.
  - *Given* `SearchOptions{Pattern:"héllo", Match:Literal, Case:Insensitive}` and file `a.go` line 3 `x := "HÉLLO" // 😀 héllo`, *When* searched, *Then* two `SearchMatch` on line 3 with ranges in UTF-16 units (the second offset accounts for the 2-unit emoji).
- Ignore and skip semantics match the tree.
  - *Given* `.gitignore` containing `dist/` and files `dist/x.js`, `src/x.js`, `bin.dat` with NUL in the first 8 KB, *When* searched with `Scope.IncludeIgnored=false`, *Then* only `src/x.js` can match; with `Scope.IncludeIgnored=true` `dist/x.js` matches; `.git/` never matches; `binary_skipped=1`.
- Caps and truncation.
  - *Given* 5,000 matching lines across 200 files and `Limits.MaxResults=1000`, *When* searched, *Then* exactly 1000 matches are emitted, at most 50 per file, `truncated=true`, and any matched line longer than 512 bytes is clamped with the range clamped inside.
- Invalid query.
  - *Given* pattern `(` with `Match=Regex`, *When* `NewSearchOptions` runs, *Then* it returns an `InvalidArgument` error naming the RE2 syntax problem; pattern longer than 1 KB is rejected; a 1-char pattern is rejected.
- Cancellation.
  - *Given* a running search over the 50k fixture, *When* `ctx` is cancelled after 50 ms, *Then* `Search` returns `context.Canceled` within 200 ms and no goroutine leaks (`goleak`); the same holds on the 200-`.gitignore` fixture variant, i.e. cancellation is honoured while ignore rules are still being loaded (no non-cancellable pre-pass; pre-mortem #4, VERIFIED `file_service.go:597-636` ignores `ctx`).
- Ignore rules are loaded incrementally and confined to the root.
  - *Given* nested `.gitignore` files in `a/` and `a/b/`, *When* the walker enters each directory, *Then* that directory's `.gitignore` is read through the `os.Root` handle at that moment (a `.gitignore` that is a symlink escaping the root is ignored, not followed), rules apply with the correct domain prefix, the first match is emitted before the tree has been fully enumerated, and the result equals the `git check-ignore` oracle.
- Worker count leaves room for terminal streaming.
  - *Given* `GOMAXPROCS=8`, *When* a search runs, *Then* at most `min(4, GOMAXPROCS/2)` scan workers run for it (and at least 1).
- Memory is bounded and independent of file count (NFR, engineering triad ET-8).
  - *Bound (derived, not measured)*: scan buffers are at most `workers x per-file cap` = 4 x 2 MB = 8 MB per walk, and the process-wide walk semaphore (limit 2, Story 1.1.2) makes that at most 16 MB across all searches; results are capped at 1000 matches x at most 512 bytes of clamped line text, under 1 MB per search; nothing scales with the 50,000 files (the walk is streaming, no `[]string` of paths is built, the ignore stack is O(directory depth), and files are read through a pooled buffered reader, not collected).
  - *Given* the 50k fixture (the 5k variant under `-short`, Story 0.2.4) and a literal query that matches in every file, *When* the search runs with a heap sampler (`runtime.ReadMemStats` every 10 ms) in a Go test, *Then* the peak `HeapAlloc` growth over the pre-search baseline stays below 64 MB (4x the derived bound, a generous CI tolerance), and `BenchmarkContentSearch -benchmem` prints `B/op` so a regression in allocation shape is visible; a file larger than 2 MB is never read past the cap (asserted with a counting reader).
**Files**: `proto/session/v1/session.proto`, `server/services/content_search.go`, `server/services/content_search_match.go`, `server/services/content_search_ignore.go`, `server/services/content_search_test.go`

##### Task 1.1.1a: `SearchContent` proto (S)
- `rpc SearchContent(SearchContentRequest) returns (stream SearchContentResponse)`. Request: `session_id, client_id (per-browser-tab random id, used only to key newest-wins), options { pattern, match_mode, case_mode, word_mode, scope { include_globs, exclude_globs, include_ignored }, limits { max_results, context_lines } }` (nested messages and enums instead of 10 flat scalar fields). Response: `oneof { SearchBatch batch; SearchStats stats; Queued queued }` (`Queued{ahead}` sent while waiting for a walk slot). **ADR-036 additions, defined here so `session.proto` is not edited again in Wave 6**: `SearchOptions.large_files` (bool, default false; Task 1.6.6a implements it), `SearchMatch.byte_offset` (raw-file offset of the matched line start), `SearchStats.large_files_skipped`. Commit `.proto` only.
- Files: `proto/session/v1/session.proto`

##### Task 1.1.1b: Domain types and interface (S)
- `ContentSearcher` (`Search(ctx, root, SearchOptions, emit)`), `SearchOptions` with typed `MatchMode`/`CaseMode`/`WordMode` enums and `Scope`/`Limits` sub-structs (+ `NewSearchOptions(proto)` as the single parse-at-boundary validation point: length cap, RE2 compile), `SearchMatch`, `SearchStats`; no engine detail in `FileService`; no adjacent same-typed bool/string parameters anywhere below the boundary (`primitive-obsession-checklist`).
- Files: `server/services/content_search.go`

##### Task 1.1.1e: Streaming, ctx-aware, `os.Root`-confined `IgnoreMatcher` (M; built before Task 1.1.1c, which consumes it)
- `collectAllGitignorePatterns` is **not** reused for search (it is a full second `filepath.WalkDir` over `rootPath`, takes no `ctx`, bypasses `os.Root`, and must finish before any file is scanned). Build an `IgnoreMatcher` that is handed each directory as the walker enters it: it opens `<dir>/.gitignore` via `root.Open`, parses lines with `gitignore.ParsePattern(line, domain)`, pushes the patterns onto a per-directory stack that is popped on leaving, checks `ctx.Err()` per directory, honours `hardSkipDirs` at any depth, and never follows a symlinked `.gitignore`. The `SearchFiles` tree RPC keeps calling `collectAllGitignorePatterns` unchanged (separate debt); a shared parser helper is extracted only if it removes duplication (dupl gate).
- **go-git reuse, VERIFIED against the pinned fork source** (`github.com/tstapler/go-git/v5 v5.19.3-ssq.1`, the `replace` in `go.mod:231`; `plumbing/format/gitignore/`): `ParsePattern(p string, domain []string) Pattern` (`pattern.go:41`) and `NewMatcher(ps []Pattern) Matcher` (`matcher.go:14`) are exported and usable without a billy filesystem; `Matcher.Match(path []string, isDir bool)` scans patterns last to first (`matcher.go:22-30`), so pushing root patterns first and deeper directories' patterns later gives git's precedence; `Pattern.Match` requires the full root-relative path split into components and the pattern's `domain` to be its directory's components (`pattern.go:66-80`), so the matcher is called with `strings.Split(rel, "/")` and each directory's patterns are parsed with that directory's components as `domain`. Three behaviors the reuse must replicate or accept: `ParsePattern` does not skip blank lines or `#` comments (the upstream reader does, `dir.go:35-36`; the matcher must do the same), the fork's `ReadPatterns` also reads `.git/info/exclude` (which search never enters, so excluded as a documented divergence along with global excludes), and a `**` inside a path segment (`a**b`) never matches (`pattern.go` `globMatch`). The `git check-ignore` oracle test (Task 1.1.1d) runs on generated repos with `core.excludesFile=/dev/null`, and any divergence it finds is either fixed in `IgnoreMatcher` or recorded in ADR-030 with the pattern; lines longer than the `bufio.Scanner` default (64 KB) are skipped, not fatal.
- Files: `server/services/content_search_ignore.go`, `server/services/content_search_test.go`

##### Task 1.1.1f: Per-file scanner: NUL sniff, 2 MB cap, line matching, UTF-16 ranges, clamps (S; built before Task 1.1.1c, which calls it)
- Split from Task 1.1.1c (engineering triad ET-4: the walker task covered traversal, ignore wiring, globs, a worker pool and the scanner, which is upper-M/L). Pure, filesystem-independent functions over an `io.Reader`: 8 KB NUL sniff (`binary_skipped`), 2 MB per-file cap enforced by a counting limit reader, line splitting through a pooled buffered reader (never `os.ReadFile`, which is what makes the Story 1.1.1 memory bound hold), literal/regex/word/case matching, UTF-16 range conversion, per-file (50) and line-length (512 bytes) clamps, `SearchMatch` construction with the clamped range inside the clamped line.
- Files: `server/services/content_search_match.go`, `server/services/content_search_test.go`

##### Task 1.1.1c: Walker implementation (M)
- Reuse the `searchFilesInWorktree` skeleton: traverse with `fs.WalkDir(root.FS(), ".")` so traversal and reads share the one `os.Root` confinement object (no symlink-swap race between walk and open), prune at `SkipDir`, `hardSkipDirs` unless `Scope.IncludeIgnored`, ignore decisions from the `IgnoreMatcher` of Task 1.1.1e (incremental, not a pre-pass), include/exclude globs, a bounded `errgroup` pool of `min(4, GOMAXPROCS/2)` workers (at least 1; leaves CPU for terminal streaming goroutines in the same process) that call the Task 1.1.1f scanner, ctx checks per directory and per file, `root.Open(rel)` for reads. No symlinked-dir following.
- **Glob library, VERIFIED**: `go.mod:126` has `github.com/bmatcuk/doublestar v1.3.4 // indirect` (the v1 import path, pulled into the module graph by `entgo.io/ent` and `ariga.io/atlas`; `go mod why -m` reports the main module does not import it), and no source file imports any doublestar. So "promote from indirect to direct" would bind the walker to the v1 API (`doublestar.Match(pattern, name) (bool, error)`, `PathMatch`, `Glob`; no pattern validator). Decision: use **v4** (`github.com/bmatcuk/doublestar/v4`; `v4.10.0` is already in the module graph via `tailscale.com@v1.102.4` and in the local module cache, but has no `go.sum` entry yet, so the task runs `go get github.com/bmatcuk/doublestar/v4@v4.10.0`). v4 provides `ValidatePattern(s string) bool` (`validate.go:12`), used in `NewSearchOptions` to reject a malformed include/exclude glob with `InvalidArgument` at the boundary, and `Match(pattern, name string) (bool, error)` (`match.go:52`) applied to slash-separated, root-relative paths (never `PathMatch`, which uses the OS separator). The v1 line stays untouched as ent's indirect dependency.
- Files: `server/services/content_search.go`, `go.mod`, `go.sum`

##### Task 1.1.1d: Tests incl. `-race`, ignore oracle, goleak (M)
- Table tests for the criteria above; ignore oracle compares to `git check-ignore` on a generated repo (a test-only subprocess, acceptable under `prefer-go-git-over-subshells`; say so in a comment); `-race`; ordering is deterministic per file group; the memory-bound heap-sampler test and the counting-reader cap test from the Story 1.1.1 ACs; large-fixture cases use the 5k variant under `-short` (Story 0.2.4, race-suite budget).
- Files: `server/services/content_search_test.go`

#### Story 1.1.2: `SearchContent` handler, delegation, observability
**As a** user, **I want** results to stream in as they are found, **so that** the first hits show quickly even in large repos.
**Acceptance Criteria**:
- Streaming shape.
  - *Given* 3 matching files and batch size 50, *When* the stream is consumed, *Then* at least one `batch` message precedes exactly one final `stats` message.
- Newest search wins, per client.
  - *Given* a search in flight for `SearchKey{s1, clientA}`, *When* a second `SearchContent` starts for the same key, *Then* the first stream ends with `Canceled` and only one walk runs for that key.
- Two devices do not cancel each other.
  - *Given* a desktop (`clientA`) and a phone (`clientB`) both searching `s1`, *When* each starts a search, *Then* both run to completion with no `Canceled`; two `clientA` searches race-started from 50 goroutines under `-race` leave exactly one surviving.
- Total walks are bounded.
  - *Given* 6 concurrent searches across 3 sessions and a process-wide walk limit of 2, *When* run, *Then* at most 2 walks execute at once (the rest wait on ctx) and all complete or cancel cleanly with no goroutine leak.
- Queued state is observable.
  - *Given* both walk slots are held, *When* a third search starts, *Then* the stream immediately sends a `SearchStats`-sibling `Queued{ahead=n}` message (proto `oneof` arm, sent once on enqueue and again if position changes), then `batch` messages after the slot is acquired; cancelling (client abort or newest-wins) while queued releases the wait with `Canceled` and never runs a walk.
- Open search streams are capped per origin and a queued stream cannot pin a connection forever (NFR, engineering triad ET-8).
  - *Why*: on the plain `:8543` HTTP/1.1 listener each open server stream holds one of the browser's 6 connections per origin, and those 6 are shared by every tab of that browser (6 per origin is the figure the Story 0.1.1 connection-slot check assumes; the exact count is browser-dependent and UNVERIFIED here). The 2-walk semaphore bounds walks, not open streams: a queued search holds its connection while it waits. Search streams sent over HTTP/2 (the `:8444` TLS listener most likely negotiates it, INFERRED) multiplex and are not limited this way, but the same cap applies so behavior does not depend on the listener.
  - *Given* named constants `maxSearchStreamsPerOrigin = 3` and `maxSearchStreamsTotal = 8` (a proposed split, INFERRED: 3 of a browser's 6 leaves at least 3 for unary RPCs; both adjustable) and `maxQueueWait = 20 s`, *When* a fourth `SearchContent` stream opens from the same remote host (different `client_id` values from tabs or windows of one browser; the key is the request's remote IP, not the client-supplied id), *Then* it is refused immediately with Connect `ResourceExhausted` and a typed `ReadOnlyReason`-style detail `SearchBusy` (no walk, no `Queued` message, no registry entry), and a phone on another IP is unaffected; *Given* the same host holding three streams, *When* one ends (`stats`, error or abort), *Then* a new one is accepted; *Given* a search queued behind the walk semaphore for longer than `maxQueueWait` (fake clock), *Then* its stream ends with `ResourceExhausted` `SearchBusy` and releases its slot; *Given* 50 goroutines opening and closing streams under `-race`, *Then* the counters never go negative or exceed the caps and no goroutine leaks (`goleak`).
  - *Client behavior when refused*: the search hook enters a `busy` state and shows "Too many searches are running in other tabs or windows - close one, or retry" with **Retry**; it never auto-retries in a loop, and it aborts its own stream when the search panel closes, the tab unmounts or the document is hidden for more than 30 s, so abandoned streams do not hold slots. Absorbed in Tasks 1.4.1a and 1.4.2b; the analytics `outcome` enums gain `busy` (ADR-035 `files.search.results` and `files.server.search`).
- Flag kill switch.
  - *Given* `files:viewer-v2` set to `false`, *When* `SearchContent` is called, *Then* it returns `FailedPrecondition` "viewer v2 disabled" with a typed `ReadOnlyReason=FlagOff` detail, which the client treats as the trigger for `FeatureFlagsContext.refresh()` (Story 0.2.2).
- Never searches the repo-root fallback.
  - *Given* a paused or archived session whose worktree directory is gone (`ActiveDir != ExistingDir`, `ExistingDir` fell back to `RepoRoot`), *When* `SearchContent` is called, *Then* it returns `FailedPrecondition` with `ReadOnlyReason=WorktreeMissing`, emits no `batch`, and walks nothing (the same wrong-checkout hazard as writes); the UI shows "Session paused; worktree unavailable" with Resume (ux.md S1, U-SEARCH-6). A directory-mode session (`ActiveDir == ExistingDir == RepoRoot`) searches normally.
- No sensitive logging.
  - *Given* query `AKIA123`, *When* logged, *Then* the Info line contains `query_len=7` and not the text.
- Server analytics outcome (ADR-035, complements the slog line).
  - *Given* a search that returns 27 results, truncated, with a fake `FilesTelemetry`, *When* the handler returns, *Then* exactly one `RecordSearch` call carries `Outcome=Truncated`, `Results` and `Scale` buckets, `Queued=false`, `Backend=Walker`, and the total duration; a cancelled search records `Cancelled`, an invalid regex `Invalid`, a `FlagOff` refusal `Disabled`; the first batch records one `files.server.search_first_result`; the event struct has no query, glob or path field, and a telemetry error or panic-free no-op default never changes the RPC result (Story 0.2.5 adapter test asserts the mapped `analytics.Event`).
**Files**: `server/services/content_search_handler.go`, `server/services/content_search_handler_test.go`, `server/services/content_search_limits.go`, `server/services/files_delegates.go`, `server/services/file_metrics.go` (new file; it does not exist today)

##### Task 1.1.2a: Handler with `SearchRegistry` and walk semaphore (M)
- Streaming handler using `resolveFileRequest(ctx, FileRef, Read)` and then refusing with `FailedPrecondition WorktreeMissing` when `ActiveDir != ExistingDir` (the same predicate as the write gate; reuse one helper), batches of up to 50 files. `SearchRegistry` is one mutex-guarded `map[SearchKey]cancelFn`, with swap-and-cancel performed inside the lock (a `sync.Map` `Load` then `Store` is not atomic and lets two concurrent searches both survive; per `instinct_check_then_reserve_needs_process_wide_lock`). `SearchKey{SessionID, ClientID}`, so newest-wins applies per client, not per session. A process-wide `golang.org/x/sync/semaphore` (limit 2) bounds concurrent walks across all sessions, acquired with the request ctx; before blocking, the handler sends `Queued{ahead}` on the stream so the UI can show the wait. Response oneof is `{batch; stats; queued}`.
- Files: `server/services/content_search_handler.go`, `server/services/content_search_handler_test.go`

##### Task 1.1.2c: Open-stream limiter and bounded queue wait (S)
- `streamLimiter` keyed by remote IP with the per-origin and total caps and a `maxQueueWait` context deadline around the semaphore acquire, injected clock; the handler of Task 1.1.2a acquires a stream slot before registering in `SearchRegistry` and releases it in a `defer`; refusal and queue timeout map to `ResourceExhausted` with the `SearchBusy` detail; counters exposed through `file_metrics.go` (Task 1.1.2b) as `files_search_total{outcome="busy"}`. Built before Task 1.1.2a's handler wires it (same worker, small file).
- Files: `server/services/content_search_limits.go`, `server/services/content_search_handler_test.go`

##### Task 1.1.2d: WS-bridge registration for `SearchContent` (S, conditional contingency from Task 0.1.1b; triad round 3)
- Runs only if ADR-030 records path (b): exact-path registration of `SessionServiceSearchContentProcedure` beside the three `Watch*` registrations (`server/server.go:499-509`), the transport-selector entry in `web-app/src/lib/transport/watch-ws-transport.ts`, and the extension of `TestEveryServerStreamingWatchRPCHasAWebSocketBridge` so a future `SearchContent`-like RPC cannot be missed. It edits `server/server.go` in **Wave 4**, so it is ordered before the Wave 4 `server.go` length probe (the probe then measures the file including these lines) and before Task 0.2.4f, 2.1.2b/d/f/g and 2.5.3a. Skipped and recorded as skipped when ADR-030 chooses path (a) (the expected default) or (c).
- Files: `server/server.go`, `server/watch_ws_bridge_registration_test.go`, `web-app/src/lib/transport/watch-ws-transport.ts`, `web-app/src/lib/hooks/useFileService.ts`

##### Task 1.1.2b: Delegate, register, observability, registry (S)
- Delegate from the new `files_delegates.go` (so `session_service_delegates.go` does not grow past 973 lines); `// +api: files:content-search`; slog + metrics in the new `file_metrics.go` (create it, following the pattern found in Task 0.1.3b); `make registry-generate` and commit `docs/registry/features/files-content-search.json`.
- Files: `server/services/files_delegates.go`, `server/services/file_metrics.go`, `docs/registry/features/files-content-search.json`

#### Story 1.1.3: Benchmark gate
**As the** owner, **I want** the 2 s / 50k-file SLO measured on the real implementation, **so that** `rg` is added only if the data says so.
**Acceptance Criteria**:
- SLO measured.
  - *Given* the 50k-file fixture and query `TODO`, *When* `BenchmarkContentSearch &` runs on the dev host, *Then* `first_result_ms < 2000` is recorded in ADR-030 with the host description; if not met, a tuning task is run (worker count, early flush) and re-measured.
- Cold-cache, many-`.gitignore` and cancel-latency numbers are recorded, not just the warm single-`.gitignore` case (pre-mortem #4).
  - *Given* the Story 0.1.2 fixtures (50k files with 1 `.gitignore`, the same with 200 nested `.gitignore` files, and the cold-cache procedure), *When* the benchmark runs on each, *Then* ADR-030 records `first_result_ms`, `total_ms` and the observed `ctx`-cancel latency when cancelling during the first 100 ms, for warm and cold runs; `first_result_ms < 2000` must hold on the 200-`.gitignore` fixture too, and cancel latency must stay under 200 ms.
- rg decision recorded.
  - *Given* the numbers, *When* ADR-030 is updated, *Then* it states "rg accelerator: not needed" or opens a follow-up backlog item with the measured gap.
**Files**: `server/services/content_search_bench_test.go`, `project_plans/files-viewer-revamp/decisions/ADR-030-in-process-streaming-content-search.md`

##### Task 1.1.3a: Run benchmark on the real walker, tune, record (M)
- Swap benchmark from prototype to production walker; run in background (warm, 200-`.gitignore` and cold-cache variants); tune if needed; update ADR-030. Also re-check batch cadence through the real handler on `:62871`, `:62872` and the phone, confirming the Story 0.1.1 transport decision holds for the production `SearchContent` stream (the spike used a test-only stream on the same path).
- Files: `server/services/content_search_bench_test.go`, `project_plans/files-viewer-revamp/decisions/ADR-030-in-process-streaming-content-search.md`

## Epic 1.2: Frontend seams (refactor-first)
**Goal**: Make the viewer and tab state extensible without growing the large files.

#### Story 1.2.1: Extract `CodeEditor` from `FileContentViewer`
**As a** developer, **I want** a reusable CodeMirror component with swappable state, **so that** tabs, edit, vim and merge build on one place.
**Acceptance Criteria**:
- Behavior-preserving extraction.
  - *Given* the existing `FileContentViewer.test.tsx` and `tests/e2e/local-file-browser.spec.ts`, *When* run after the extraction, *Then* all pass with unchanged `data-testid`s.
- Read-only by default, switchable.
  - *Given* `<CodeEditor mode="Read">`, *When* the test switches to `mode="Edit"`, *Then* typing `x` changes the doc only in Edit mode and the view instance is the same object (Compartment reconfigure, no re-create).
- Imperative API.
  - *Given* a mounted editor, *When* `handle.gotoLine(250)` is called, *Then* the selection head is at the start of line 250 and it is scrolled into view centered.
- Extension seams and size budget.
  - *Given* the extracted `CodeEditor.tsx`, *When* `wc -l` runs, *Then* it is <= 350 lines and the read-only facet, degrade profile, search highlight, vim and merge behaviors live in separate modules under `web-app/src/components/sessions/files/extensions/` (`readOnlyFacet.ts`, `degrade.ts`, `searchHighlight.ts`, `vim.ts`, `merge.ts`; stubs with a single exported `Extension` factory are fine until their stories land) that the editor composes via Compartments.
- Open-latency SLO is measured.
  - *Given* a 1 MB text fixture (about 25,000 lines), *When* a jest/Playwright timing test opens it in Read mode after the RPC response is delivered, *Then* time to first painted content is recorded and asserted < 300 ms on desktop with a generous CI tolerance (fail at 3x = 900 ms), and the number is logged to the PR description; on `pointer: coarse` the degrade profile applies (Story 1.5.2) and the same fixture is measured informationally (manual on device if CI cannot emulate).
- Analytics: open latency (ADR-035).
  - *Given* the open-latency hook of Task 1.2.1d, *When* a buffer finishes first paint, *Then* `files.perf.open_ms` (category `performance`, `durationMs` = RPC-return to first paint) is emitted once with labels `size` (bucket) and `degraded` (bool) and no path or file name.
- The editor sits behind an engine port (ADR-036 decisions 19-20, owner decision 2026-10-16: CodeMirror 6 stays, but it is an adapter).
  - *Given* the Files tab code, *When* `rg "@codemirror/|@replit/codemirror-vim|@lezer/"` runs over `web-app/src/components/sessions/**` and `web-app/src/lib/files/**` excluding `web-app/src/components/sessions/files/engine/`, `web-app/src/components/sessions/files/extensions/` and the existing `CodeEditor.tsx` facade imports of the engine factory, *Then* it finds nothing (an ESLint `no-restricted-imports` override and a jest import-boundary test enforce it, mirroring the `depguard` style); `FileContentViewer` and `FilesTab` import `EditorEngine`, `Highlighter` and `MergeSurface` only.
  - *Given* `runEngineConformance(makeEngine)` (typing; IME composition does not dispatch while `view.composing`; `provideRange`/`evictRange` keep the scroll anchor; edits map to the right absolute line/col across a gap; read-only; accessible name and role), *When* it runs against `CmEngine` with `features.lazyDoc=false` (this story) , *Then* every case passes and the lazy-doc cases are reported as skipped-with-reason, not silently absent; Story 1.6.4 turns them on.
  - *Given* `EditorEngine.features` (`vim`, `mergeView`, `lazyDoc`, `irc`), *When* the UI needs to know whether Vim keys, the merge view or lazy windows exist, *Then* it reads the flags and never tests which engine is mounted.
**Files**: `web-app/src/components/sessions/CodeEditor.tsx`, `web-app/src/components/sessions/CodeEditor.css.ts`, `web-app/src/components/sessions/files/extensions/*.ts`, `web-app/src/components/sessions/files/engine/` (ADR-036 ports and `CmEngine`), `web-app/src/components/sessions/FileContentViewer.tsx`, `web-app/src/components/sessions/__tests__/CodeEditor.test.tsx`

##### Task 1.2.1e: `extensions/*` module stubs (S; built before Task 1.2.1a, which composes them)
- Split from Task 1.2.1a (engineering triad ET-3: the original task listed seven files, two over the 3-5 file limit). Create the five modules under `web-app/src/components/sessions/files/extensions/`, each exporting a single `Extension` factory with a no-op default and a one-line comment naming the story that fills it in: `readOnlyFacet.ts` (Story 2.2.1), `degrade.ts` (Story 1.5.2), `searchHighlight.ts` (Story 1.4.2), `vim.ts` (Story 2.4.1), `merge.ts` (Story 2.3.2). Wave 2 ordering: this task first, then 1.2.1a.
- Files: `web-app/src/components/sessions/files/extensions/readOnlyFacet.ts`, `web-app/src/components/sessions/files/extensions/degrade.ts`, `web-app/src/components/sessions/files/extensions/searchHighlight.ts`, `web-app/src/components/sessions/files/extensions/vim.ts`, `web-app/src/components/sessions/files/extensions/merge.ts`

##### Task 1.2.1a: `CodeEditor` component and handle (M)
- Move `CodeMirrorViewer` logic; the editor is a thin composition root over the `extensions/*` modules created by Task 1.2.1e (later stories add behavior in those modules, not in `CodeEditor.tsx`); **ADR-036: after Task 1.2.1f this component is the React facade over `EditorEngine`, and the CM6 view creation and Compartment wiring written here move into `CmEngine` (Task 1.2.1f runs right after this task and before Task 1.2.1b)**; Compartments for readOnly/wrap/vim/merge; `setBufferState(EditorState)`, `getState()`, `gotoLine`, `scrollSnapshot`; never replace doc from props; never dispatch while `view.composing`; `EditorView.contentAttributes` (`aria-label="File: <path>"`, `aria-readonly`, and explicit `autocorrect="off"`, `autocapitalize="off"`, `spellcheck="false"` so soft-keyboard smart punctuation cannot corrupt source; asserted by the Task 0.1.5a test); font >= 16px on `pointer: coarse`.
- Files: `web-app/src/components/sessions/CodeEditor.tsx`, `web-app/src/components/sessions/CodeEditor.css.ts`

##### Task 1.2.1b: `FileContentViewer` consumes `CodeEditor` (own commit) (M)
- Delete the inline CodeMirror effect; keep image/PDF/video branches and test ids; replace the hand-rolled `vimKeymap` temporarily with existing `gg`,`/`,`n`,`N` parity (removed in Story 2.4.1).
- Files: `web-app/src/components/sessions/FileContentViewer.tsx`

##### Task 1.2.1c: Unit tests (S)
- Mode switch, gotoLine, state swap, composition guard. One shared fixture module to keep jscpd under 0.14%.
- Files: `web-app/src/components/sessions/__tests__/CodeEditor.test.tsx`

##### Task 1.2.1d: Open-latency measurement on a 1 MB fixture (S)
- Generate the 1 MB fixture deterministically in the test, time open-to-first-paint with `performance.now()` around `setBufferState`, assert the threshold in the AC above, and print the measured value. Records the Requirements SLO (open < 1 MB file in < 300 ms after RPC return) as an executable check rather than a claim.
- Files: `web-app/src/components/sessions/__tests__/CodeEditorOpenLatency.test.tsx`

##### Task 1.2.1f: `EditorEngine`, `Highlighter` and `MergeSurface` ports, the `CmEngine` adapter and the import boundary (M; ADR-036)
- Interfaces exactly as research/transport.md section 11.1 (`setSource(VirtualDoc)`, `onNeedRange/provideRange/evictRange`, `onEdits/takePending/ack` in absolute line/col_utf16, `reveal`, `getViewport`, `setReadOnly`, `setMode`, `features`, `focus`, `destroy`); `Highlighter.tokenize(range, languageId)`; `MergeSurface.show3Way(base, mine, theirs)`. `CmEngine` takes over the CM6 view creation and Compartment wiring that Task 1.2.1a wrote inside `CodeEditor.tsx`, which becomes the React facade over the port (still <= 350 lines); the `extensions/*` modules become engine-internal. `lazyDoc:false`, `mergeView` per the merge module. ESLint `no-restricted-imports` override keeps `@codemirror/*` inside `engine/` and `extensions/`. Runs after Task 1.2.1a and before Task 1.2.1b.
- Files: `web-app/src/components/sessions/files/engine/EditorEngine.ts`, `web-app/src/components/sessions/files/engine/Highlighter.ts`, `web-app/src/components/sessions/files/engine/MergeSurface.ts`, `web-app/src/components/sessions/files/engine/CmEngine.ts`, `web-app/.eslintrc*` (override)

##### Task 1.2.1g: `runEngineConformance` suite and the import-boundary test (S; ADR-036)
- The shared suite under `engine/__tests__/` plus `CmEngine.conformance.test.tsx` running it, and `importBoundary.test.ts` asserting the grep rule above; the suite is the gate any later engine (the contingent custom engine of Epic 1.7) must pass. One shared fixture module, no new `jest.mock` blocks (Story 0.2.4).
- Files: `web-app/src/components/sessions/files/engine/__tests__/engineConformance.ts`, `web-app/src/components/sessions/files/engine/__tests__/CmEngine.conformance.test.tsx`, `web-app/src/components/sessions/files/engine/__tests__/importBoundary.test.ts`

#### Story 1.2.2: `useOpenBuffers` hook and thin `FilesTab`
**As a** user, **I want** several files open at once with preserved position, **so that** I can follow a trail across files.
**Acceptance Criteria**:
- Preview tab semantics.
  - *Given* `OpenBuffers` with a preview `Buffer` `a.go`, *When* the user single-opens `b.go`, *Then* `b.go` replaces `a.go` as preview (still 1 tab); double-open pins it and the next single-open adds a second tab.
- A dirty buffer is never silently replaced.
  - *Given* preview `a.go` (clean), *When* its buffer enters Edit mode (`setMode(Edit)`), *Then* it is promoted to `pinned` in the same reducer step, before any keystroke (triad round 3: otherwise a clean buffer in Edit could be the one the next single-click or the tab cap replaces; U-EDIT-9); *When* `b.go` is then single-opened, *Then* `b.go` opens as a new tab and `a.go` remains; a first edit on a buffer that is somehow still preview (restored draft path) also promotes it. A reducer property test asserts no action sequence leaves a dirty buffer in the preview slot or removes it without an explicit `close` that passed the discard guard.
- Illegal states are unrepresentable.
  - *Given* the `Buffer` type, *When* it is compiled with exhaustive switches, *Then* `Loading`, `Failed` and `Loaded` are separate union arms (no `loaded: boolean` plus optional fields), and the reducer state holds `dirty: boolean` but not the draft text (the text is read from `EditorState` by `draftStore` on a debounce).
- Eviction never touches pinned or dirty tabs, one cap rule everywhere (triad repair; was "least-recently-used clean buffer").
  - *Given* the cap (10 when `pointer: coarse` or viewport < 768px, 30 otherwise) and a full set where `x.go` is dirty, `p.go` is pinned and `c.go` is a clean preview, *When* one more opens, *Then* only `c.go` is evicted, `x.go` and `p.go` stay, a polite `aria-live` message "Closed c.go to stay within N tabs" is announced and an **Undo** toast of at least 8 s (paused while hovered or focused, dismissible; ux.md section 0) is offered (Undo reopens `c.go` with its saved `ViewState`; the reducer keeps a bounded closed-tab stack of 10 `{path, ViewState}` entries, no contents).
  - *Given* no buffer is an eligible clean preview (all pinned or dirty) at the cap, *When* one more opens, *Then* the open is **refused** (no buffer is closed), an inline "Too many tabs open - close one" shows and the tab list opens (mobile sheet / desktop `[v]` list); a reducer property test asserts no action sequence closes a pinned or dirty buffer without an explicit `close` that passed the discard guard, and eviction never selects a `Loading` buffer or one with a pending save.
- Edit mode is per buffer (ux.md 8.1, U-EDIT-17).
  - *Given* two `Loaded` editable buffers `a.go` (Edit) and `b.go` (Read), *When* the user switches tabs and back, *Then* each keeps its own `mode`; *When* Stop editing runs on `a.go`, *Then* `b.go` is unaffected; *When* a buffer is reloaded or reopened after a page reload, *Then* it starts in `Read`; a reducer test asserts `Edit` is unreachable for a buffer whose read-only reason is not `None` (exhaustive union), and the flag-off action sets every buffer to `Read` without clearing `dirty`.
- Analytics: tab restore (ADR-035).
  - *Given* a remount with 3 persisted tabs, *When* restore finishes, *Then* one `files.tab.restore` is emitted with `outcome` {ok, partial, storage_unavailable} and a `tabs` bucket, never paths.
- Persistence is bounded.
  - *Given* 3 open buffers in session `s1`, *When* the hook unmounts and remounts, *Then* paths, active index and `ViewState` are restored from localStorage key `ssq.files.s1.<device>`; no file contents are stored; a throwing `localStorage` leaves a working in-memory hook.
- Back-compat.
  - *Given* `initialSelectedPath="src/main.go"`, *When* `FilesTab` mounts, *Then* that buffer is active and `onSelectedPathChange` fires with the same path.
- A buffer is bound to the session it was opened in (pre-mortem #2, VERIFIED: `SessionDetailView.tsx:1208` renders `FilesTab` with no `key`, and `:496-498` already clears the selection on session change, so reuse across sessions is real).
  - *Given* a `Loaded` buffer opened with `sessionId="s1"`, *When* the `sessionId` prop becomes `s2` without a remount, *Then* the buffer still carries `s1`, every stat, save and draft write for it uses `buffer.sessionId`, and no `s2` request is ever made for `s1`'s path; *Given* `SessionDetailView` switches session, *Then* `FilesTab` is remounted via `key={session.id}` so the reducer starts empty for `s2` (dirty buffers of `s1` are handed to `SessionBufferHold` first, Story 2.2.2).
- Buffers load through the transport port and may be windowed (ADR-036; ADR-032 amended).
  - *Given* a `Loaded` buffer, *When* the reducer type is compiled, *Then* it carries `source: Full | Windowed{windowStartLine, lineCount?, analysis}` (Task 1.6.5a adds the `Windowed` arm; this story ships `Full` only) and the load path goes through `useFileTransport()` (Story 1.2.3), never the generated client; persisted tab state still stores paths, active index, `ViewState` and (for `Windowed`) an anchor line, never text.
**Files**: `web-app/src/lib/files/bufferReducer.ts`, `web-app/src/lib/hooks/useOpenBuffers.ts`, `web-app/src/components/sessions/FilesTab.tsx`, `web-app/src/components/sessions/SessionDetailView.tsx`, `web-app/src/lib/files/__tests__/bufferReducer.test.ts`, `web-app/src/lib/hooks/__tests__/useOpenBuffers.test.ts`

##### Task 1.2.2a: Pure `bufferReducer` (M)
- Actions: open(preview/pinned), close, activate, pin/unpin, navBack/Forward, recordViewState, markLoaded/markFailed, markDirty (promotes preview to pinned, defensive), setMode(Read|Edit) per buffer (rejects `Edit` when the read-only reason is not `None`; `setMode(Edit)` promotes preview to pinned), flagOff (all to Read), evict (clean previews only); MRU derivation. `Buffer = Loading | Failed | Loaded` discriminated union with exhaustive switch; every arm carries `sessionId`, set by the `open` action from the hook's session at open time and never updated afterwards. Reducer test: an `open` for `s1` followed by actions dispatched while the hook's session is `s2` leaves the buffer's `sessionId` as `s1`.
- Files: `web-app/src/lib/files/bufferReducer.ts`, `web-app/src/lib/files/__tests__/bufferReducer.test.ts`

##### Task 1.2.2b: `useOpenBuffers` with persistence (S)
- Hook wraps reducer; try/catch localStorage; debounce writes; paths + active + viewState only.
- Files: `web-app/src/lib/hooks/useOpenBuffers.ts`, `web-app/src/lib/hooks/__tests__/useOpenBuffers.test.ts`

##### Task 1.2.2c: `FilesTab` becomes composition root (M)
- (ADR-036: buffers load through `useFileTransport()`, Task 1.2.3c, which runs before this task.) `selectedPath` derived; `RecentFilesSection` fed from MRU; keep `onSelectedPathChange`/`initialSelectedPath`; add `key={session.id}` to the `<FilesTab>` element in `SessionDetailView.tsx` (the one-line edit there); gate new layout on `files:viewer-v2` (render a neutral skeleton while `useFilesFlags().status === "loading"`, Story 0.2.2) (flag off = legacy path preserved, covered by one smoke test; retirement plan in Risk Control: the legacy path and flag are deleted after 4 flag-on weeks).
- Files: `web-app/src/components/sessions/FilesTab.tsx`, `web-app/src/components/sessions/RecentFilesSection.tsx`, `web-app/src/components/sessions/SessionDetailView.tsx`

##### Task 1.2.2d: Hook seams for `FilesTab` and line budget (S)
- Move the keyboard shortcut registration, deep-link handling, staleness wiring and draft-guard wiring out of `FilesTab.tsx` into `useFilesShortcuts`, `useFilesDeepLink` (and later `useBufferFreshness`, `useDraftGuard`) so later stories add hooks, not inline effects. `FilesTab.tsx` <= 350 lines; asserted by a tiny repo test (`wc -l` style) in the jest suite so it fails loudly when a later story breaches it.
- Files: `web-app/src/components/sessions/FilesTab.tsx`, `web-app/src/lib/hooks/useFilesShortcuts.ts`, `web-app/src/lib/hooks/useFilesDeepLink.ts`

#### Story 1.2.3: `FileTransport` client port, `FileTransportRouter` and `FullFileTransport` (ADR-036 Phase A; behavior-identical)
**As a** developer, **I want** the buffer, save and freshness hooks to talk to a transport port with capability-driven routing and a `full` adapter that behaves exactly like today's RPCs, **so that** range reads, deltas and extra wires are added as adapters later without touching the hooks or the safety rules.
**Acceptance Criteria**:
- Behavior-identical floor.
  - *Given* `FullFileTransport` over `GetFileContent`/`StatFile` (and `WriteFile` once Story 2.1.2 lands), *When* the existing Story 1.2.2 hook tests and `FileContentViewer.test.tsx` run with the transport injected, *Then* they pass unchanged; `read({kind:"all"})` returns the same text, token, line ending and read-only reason as the direct RPC, the truncated-read refusal stays on this adapter, and `capabilities().limits.hash_cap_bytes` is 1 MiB.
- Capability-driven per-op routing.
  - *Given* a fake capabilities response listing `STAT, READ_ALL, WRITE_FULL, READ_LINES, PATCH` and wires `CONNECT_UNARY`, *When* `FileTransportRouter` resolves the ops, *Then* `read` and `patch` pick `connect_unary` (once that adapter exists; here the fake registers it), `stat` and `writeFull` resolve independently, and an op the server does not advertise is never called (the call is routed to the next rung or refused with a typed reason).
  - *Given* `Unimplemented` from the capability RPC, or `protocol_version` greater than the client's supported major, *Then* the router pins `FullFileTransport` for every op and emits `files.transport.select{op, transport=full, reason=version|capabilities}` once.
  - *Given* a response whose `caps_epoch` differs from the cached value, *Then* the router refetches capabilities once (live flag flips are noticed within one response) and re-resolves; unknown `features`/`ops` strings are ignored.
- Demotion ladder (application errors never demote).
  - *Given* three consecutive transport-class failures of the `read` op (network error, `Unavailable`, `DeadlineExceeded`, malformed envelope) on `ws_stream`, *Then* that op is demoted one rung (`ws_stream` > `connect_stream` > `connect_unary` > `full`) for the page lifetime, a quiet status-bar chip "Using simpler file mode" (text, not color-only) is requested through the banner stack, and `files.transport.fallback{op, from, to, fallback_reason}` is emitted once; *Given* `Conflict`, `FailedPrecondition` or `PermissionDenied` ten times in a row, *Then* nothing is demoted; promotion happens only on page reload.
- Hooks depend on the port, never on the generated client.
  - *Given* `useOpenBuffers`, `useBufferFreshness` and `useSaveBuffer`, *When* inspected and exercised through `createFilesTestHarness`, *Then* they obtain a `FileTransport` from `useFileTransport()` and import no generated `*_connect` client; the harness's fake transport implements the port and runs `runTransportConformance` (scaffold here; Stories 1.6.3, 2.2.5 and 2.6.2 add cases).
- Analytics (ADR-035, ADR-036).
  - *Given* a page load against a server advertising `connect_unary`, then a forced demotion, *Then* exactly one `files.transport.select` per op change and one `files.transport.fallback` are emitted with closed-enum labels only.
**Files**: `web-app/src/lib/files/transport/fileTransport.ts`, `web-app/src/lib/files/transport/router.ts`, `web-app/src/lib/files/transport/fullFileTransport.ts`, `web-app/src/lib/hooks/useFileTransport.ts`, `web-app/src/lib/files/transport/__tests__/transportConformance.ts`

##### Task 1.2.3a: `FileTransport` port types and `FileTransportRouter` (M)
- Runs after Task 0.2.6a (the generated capability types). `fileTransport.ts` (the interface of research/transport.md 1.1: `id`, `capabilities()`, `stat`, `read` with `ReadRequest` all/lines/bytes, optional `readStream`, optional `patch`, `writeFull`, `FileRef`, `ReadResult` with validator, served range and `chunkCrc32c`, typed `Rejected(reason)` mirroring `ReadOnlyReason`), `router.ts` (capability fetch and cache, `caps_epoch` refetch, per-op resolution, demotion ladder, version pinning), unit tests with fake adapters and fake timers.
- Files: `web-app/src/lib/files/transport/fileTransport.ts`, `web-app/src/lib/files/transport/router.ts`, `web-app/src/lib/files/transport/__tests__/router.test.ts`

##### Task 1.2.3b: `FullFileTransport` over the existing RPCs (S)
- Wraps `GetFileContent` (`read` kind `all`), `StatFile` and, when present, `WriteFile` (`writeFull`; the binding is exercised by Task 2.2.1a); maps proto fields to `ReadResult`/`Rejected`; advertises today's limits; no new RPC.
- Files: `web-app/src/lib/files/transport/fullFileTransport.ts`, `web-app/src/lib/files/transport/__tests__/fullFileTransport.test.ts`

##### Task 1.2.3c: `useFileTransport`, hook seams and the `runTransportConformance` scaffold (M)
- `useFileTransport()` (context with the router, memoised; the production provider builds it from `useFileService` and the capability RPC), the fake transport in `fixtures/fakeTransport.ts` (tests pass it through `createFilesTestHarness({transport})`; the harness files are not edited), the load path of `useOpenBuffers` (Task 1.2.2b) switched to the port, the conformance scaffold (capability honesty for read-all/stat/writeFull, error mapping to typed reasons, abort/cancel) run against `FullFileTransport`. `useBufferFreshness` (Task 1.3.4a) and `useSaveBuffer` (Task 2.2.1a) are written against the port from the start. Runs after Task 1.2.2b and before Task 1.2.2c.
- Files: `web-app/src/lib/hooks/useFileTransport.ts`, `web-app/src/lib/hooks/useOpenBuffers.ts`, `web-app/src/lib/hooks/__tests__/useOpenBuffers.test.ts`, `web-app/src/lib/files/__tests__/fixtures/fakeTransport.ts`, `web-app/src/lib/files/transport/__tests__/transportConformance.ts`

## Epic 1.3: Tabs and navigation UI
**Goal**: VS Code tabs plus a Vim-style jumplist, with staleness awareness.

#### Story 1.3.1: Tab strip
**As a** desktop user, **I want** a tab strip with preview/pinned tabs, **so that** I can switch files without losing place.
**Acceptance Criteria**:
- Accessible tabs.
  - *Given* 3 `Buffer`s open with `b.go` active, *When* ArrowRight is pressed on the focused tab, *Then* focus and activation move to `c.go`; roles are `tablist`/`tab`/`tabpanel` with `aria-selected` and `aria-controls`; Delete closes the tab and focuses its neighbor.
- Dirty state is announced.
  - *Given* `b.go` with `dirty = true`, *When* rendered, *Then* its accessible name is `b.go, unsaved` and a dot is shown.
- Touch target.
  - *Given* desktop and mobile viewports, *When* the close button is measured, *Then* its hit area is >= 44x44 CSS px on `pointer: coarse` (and >= 24x24 clear on fine).
- Overflow.
  - *Given* 20 tabs, *When* the 20th activates, *Then* it scrolls into view.
- Eviction is announced, undoable and never silent for pinned/dirty tabs (ux.md section 5, U-TAB-8).
  - *Given* the cap is reached with one clean preview tab, *When* another file opens, *Then* a toast "Closed <file> to stay within N tabs" with **Undo** shows (at least 8 s, paused on hover/focus, dismissible) and an `aria-live="polite"` announcement is made; *Given* all tabs are pinned/dirty, *Then* the open is refused with the message and the tab list opens; pinned tabs offer **Unpin** in the context menu and, on touch, in the tab sheet row's actions menu (Task 1.5.1b; ux.md 5/6, U-TAB-9/U-TAB-10), and pinned/preview state is in the accessible name (", pinned"/", preview") with a pin glyph, not italic alone.
- Analytics (ADR-035).
  - *Given* a tab opens from search, closes dirty via the button, switches via the sheet, is evicted and the eviction undone, *When* each happens, *Then* `files.tab.open` (`kind`, `source`, `ext_class`, `size`, `open_tabs`), `files.tab.close` (`dirty`, `method`), `files.tab.switch` (`via`; coalesced within 250 ms), `files.tab.evict` (`outcome`) and `files.tab.evict_undo` are emitted with enum/bucket labels only (privacy test, Story 0.2.5).
**Files**: `web-app/src/components/sessions/files/OpenTabsBar.tsx`, `web-app/src/components/sessions/files/OpenTabsBar.css.ts`, `web-app/src/components/sessions/__tests__/OpenTabsBar.test.tsx`

##### Task 1.3.1a: `OpenTabsBar` (M)
- Preview in italics plus the ", preview" name suffix, middle-click/**`Alt+W`** close (Ctrl+W is browser-reserved, ux.md 14.1), move left/right items in the context menu (the keyboard alternative to Alt+Shift+Arrow), context menu (close others, copy path, reveal in tree via `FileTreeHandle.revealPath`, Unpin), `data-testid="files-tab-<path>"`; eviction toast with Undo and the refuse-and-open-list path.
- Files: `web-app/src/components/sessions/files/OpenTabsBar.tsx`, `web-app/src/components/sessions/files/OpenTabsBar.css.ts`

##### Task 1.3.1b: Tests (S)
- Files: `web-app/src/components/sessions/__tests__/OpenTabsBar.test.tsx`

#### Story 1.3.2: Jumplist, go-to-line, deep links
**As a** user, **I want** back/forward and `:N`, **so that** I can return to where I was.
**Acceptance Criteria**:
- Jumplist.
  - *Given* a `NavEntry` stack `[a.go:10, b.go:40, c.go:7]` at position 3, *When* **Alt+[** twice then **Alt+]** once (Alt+Left/Right are the browser's history shortcuts and are not intercepted, ux.md 14.1), *Then* the active buffer is `b.go`, cursor at line 40, and each buffer restores its own `ViewState`.
- Go to line.
  - *Given* `a.go` with 300 lines, *When* the user presses Ctrl+G and enters `250`, *Then* the cursor is on line 250; entering `9999` clamps to line 300.
- Deep link.
  - *Given* URL `...?file=src/main.go&line=42`, *When* the Files tab loads, *Then* `src/main.go` opens as a pinned buffer at line 42.
- Shortcuts do not hijack the browser.
  - *Given* focus outside the Files tab, *When* Ctrl+Shift+F is pressed, *Then* the browser default is not prevented.
- One Back model (triad round 2, ux.md 7.4, U-NAV-6/U-NAV-7).
  - *Given* the mobile search pane with results scrolled to row 40 and the editor on `x.go`, *When* the user opens a hit in `a.go`, then presses the top-bar `<` (or system Back), *Then* the search pane returns with the same scroll offset, query, toggles and collapsed groups and the jumplist is unchanged by that pop; *When* instead the bottom-bar Back is pressed after opening the hit, *Then* the active buffer is `x.go` at its earlier line (the previous location) and no pane is popped; `<` and the bottom-bar Back are labelled "Back to results"/"Previous location" respectively when the previous pane is search.
  - *Given* a quickfix chain of hits, *When* F4 or the chip steps through them, *Then* no pane entry is pushed and the jumplist records the origin location only for the first hit; Back/Forward at the ends are `aria-disabled` with "No earlier location"/"No later location".
- One owner of URL state (pre-mortem #11, VERIFIED: `SessionDetailView.tsx:486-498` calls `history.replaceState(null, "", ...)` on every selection change, which discards `history.state` and so wipes the pane entries Story 1.5.1 pushes).
  - *Given* the mobile pane stack has pushed `history.pushState({pane: "editor"}, ...)`, *When* the selected file changes, *Then* the URL hash is updated with `history.replaceState(history.state, "", ...)` so the pushed state object survives; the `#file=` effect in `SessionDetailView.tsx` is removed and `useFilesDeepLink` is the only code that writes `#file=` / `?file=&line=` and `history` state.
  - *Given* the phone (gating mobile project): open a hit from results, press Back twice, *Then* the pane order is editor -> results -> files and the URL stays inside the session; a deep link `?file=&line=` still opens at the line after Back/forward.
- Analytics (ADR-035).
  - *Given* Back, Forward and a go-to-line of 9999 on a 300-line file, *When* each runs, *Then* `files.nav.back`/`files.nav.forward` (`via`, `depth` bucket) and `files.nav.jump_line` (`via`, `clamped=true`) are emitted, with no line number, path or file name.
**Files**: `web-app/src/lib/files/bufferReducer.ts`, `web-app/src/components/sessions/FilesTab.tsx`, `web-app/src/components/sessions/SessionDetailView.tsx`, `web-app/src/lib/hooks/useFilesDeepLink.ts`, `web-app/src/lib/files/deepLink.ts`, `web-app/src/lib/files/__tests__/deepLink.test.ts`

##### Task 1.3.2a: Jumplist + goto-line wiring (S)
- Includes the Back-model rules above (first-hit-only origin entry, labels, ends disabled) in the reducer, and the Alt+[ / Alt+] bindings registered through `useFilesShortcuts` (never Alt+Left/Right).
- Files: `web-app/src/lib/files/bufferReducer.ts`, `web-app/src/components/sessions/FilesTab.tsx`

##### Task 1.3.2b: Deep-link parse/apply and tests (S)
- `?file=&line=` via existing router; reuse `onNavigateToFile` from `SessionVcsContext` for diff-panel entry.
- Files: `web-app/src/lib/files/deepLink.ts`, `web-app/src/lib/files/__tests__/deepLink.test.ts`

##### Task 1.3.2c: Single URL-state owner: reconcile the existing hash sync with pane history (S)
- Delete the `replaceState(null, ...)` hash effect (and its session-change clear effect, which `key={session.id}` from Task 1.2.2c makes redundant) from `SessionDetailView.tsx`; move hash/query writes into `useFilesDeepLink`, which always passes `history.state` through `replaceState`. Keep `initialSelectedPath` / `onSelectedPathChange` working for existing callers (`FilesTab` back-compat AC). Test: a jest test that a selection change after `pushState({pane})` leaves `history.state.pane` intact; the mobile Back-Back e2e is added in Task 1.5.3b.
- Files: `web-app/src/components/sessions/SessionDetailView.tsx`, `web-app/src/lib/hooks/useFilesDeepLink.ts`, `web-app/src/lib/files/__tests__/deepLink.test.ts`

#### Story 1.3.3: Outline / symbols
**As a** user, **I want** a symbol outline, **so that** I can jump to functions without LSP.
**Acceptance Criteria**:
- Outline from Lezer tree.
  - *Given* `main.go` with `func A()` at line 5 and `type B struct` at line 20, *When* the outline opens, *Then* it lists `A (5)` and `B (20)` and selecting `B` jumps to line 20.
- Large file degrade.
  - *Given* a file above the highlight-off threshold, *When* the outline is requested, *Then* it shows "Outline unavailable for large files".
- Analytics (ADR-035).
  - *Given* the outline opens, a symbol is chosen, and another open finds no symbols, *When* each completes, *Then* `files.nav.outline` is emitted with `outcome` {jumped, empty, unavailable_large, closed} and a `symbols` bucket, never symbol names.
**Files**: `web-app/src/lib/files/outline.ts`, `web-app/src/components/sessions/files/OutlinePanel.tsx`, `web-app/src/lib/files/__tests__/outline.test.ts`

##### Task 1.3.3a: `outline.ts` per-language node maps (go, ts/js, py, rust, java, markdown headings) (S)
- Files: `web-app/src/lib/files/outline.ts`, `web-app/src/lib/files/__tests__/outline.test.ts`

##### Task 1.3.3b: `OutlinePanel` (dropdown desktop, sheet mobile) (S)
- Files: `web-app/src/components/sessions/files/OutlinePanel.tsx`

#### Story 1.3.4: Staleness banner for clean buffers
**As a** user watching agent edits, **I want** to know when an open file changed on disk, **so that** I never read stale content unknowingly.
**Acceptance Criteria**:
- Banner on change.
  - *Given* a clean `Buffer` `a.go` with `baseToken T1`, *When* the window regains focus and `StatFile` returns `T2`, *Then* a `role="status"` banner "Changed on disk - Reload" appears and content is not replaced.
- Reload keeps position.
  - *Given* the banner, *When* Reload is clicked, *Then* the new content loads, `baseToken=T2`, and cursor line and scroll are preserved.
- No focus theft.
  - *Given* typing focus in the search box, *When* the banner appears, *Then* focus does not move.
- Concurrent agent writes are detected while the tab is open, not only on focus (triad repair, ux.md section 8.7, U-EDIT-16).
  - *Given* a visible, focused Files tab with clean active buffer `a.go` and fake timers, *When* 10 s elapse and `StatFile` returns a different hash segment on two consecutive checks (or the same different hash persists 1 s), *Then* exactly one banner shows (updated in place if the token changes again while open); *Given* the tab is hidden, offline or `files:viewer-v2` is off, *Then* no poll runs; *Given* 3 consecutive `StatFile` failures, *Then* the interval backs off to 30 s; only the active buffer is polled, other tabs are checked on activation; at most one `StatFile` is in flight.
  - *Given* the banner and a running session, *When* it renders, *Then* the suffix reads "(session is running; this may be the agent)"; *Given* a non-running session, *Then* "(source unknown)". The text never states who changed the file. A dirty active buffer gets the "Changed on disk - review diff" banner by the same rule.
  - *Given* the user saved `a.go` (clean, new token) and an agent then overwrites it, *When* the next poll runs, *Then* the same banner shows within 10 s (covers the previously deferred pre-mortem #7 case for the active tab).
- An mtime-only change is not a change (pre-mortem #6: a `touch`, formatter no-op or `git checkout` rewriting identical bytes).
  - *Given* a clean `Buffer` with `baseToken T1 = v1:S:M1:H` and `StatFile` returns `v1:S:M2:H` (same hash), *When* the window regains focus, *Then* no banner is shown and `baseToken` silently becomes the new token (reducer action `adoptToken`); *Given* a differing hash, *Then* the banner appears. Tokens whose hash segment is `-` compare as whole strings.
- Analytics (ADR-035).
  - *Given* the banner shows and the user chooses Reload, *When* it happens, *Then* `files.stale.shown` (`dirty`, `actor` {agent_likely, unknown}) is emitted once per banner and `files.stale.action` (`action` {reload, review, dismiss}) on the choice; `agent_likely` is emitted only when the session is running.
**Files**: `web-app/src/lib/hooks/useBufferFreshness.ts`, `web-app/src/components/sessions/files/StaleBanner.tsx`, `web-app/src/lib/hooks/__tests__/useBufferFreshness.test.ts`

##### Task 1.3.4a: `useBufferFreshness` (focus + visibilitychange + 10 s visible-tab poll of the active buffer with 1 s / two-check debounce and failure back-off; compares the hash segment, adopts mtime-only changes silently) (M)
- Written against `FileTransport.stat` (Story 1.2.3), not the generated client. ADR-036: while a windowed buffer's `analysis` is pending its token is `v1:<size>:<mtime_ns>:-`, so the freshness check compares size and mtime only until the hash is ready (a size or mtime change raises the banner; once the hash is known the usual hash-segment rule applies); a validator change reported by any range response raises the same banner.
- Files: `web-app/src/lib/hooks/useBufferFreshness.ts`, `web-app/src/lib/hooks/__tests__/useBufferFreshness.test.ts`

##### Task 1.3.4b: `StaleBanner` + reload action (S)
- Files: `web-app/src/components/sessions/files/StaleBanner.tsx`, `web-app/src/components/sessions/FilesTab.tsx`

#### Story 1.3.5: Status bar, command palette, shortcut registration
**As a** keyboard user, **I want** a status bar and a palette, **so that** state and actions are discoverable.
**Acceptance Criteria**:
- Status bar content.
  - *Given* `a.md` CRLF UTF-8 read-only because Truncated, *When* rendered, *Then* it shows `Ln 1, Col 1`, `CRLF`, `UTF-8`, `Markdown`, and a chip "Read-only: showing first 1 MB of 3.2 MB".
- Palette.
  - *Given* QuickOpen open, *When* the user types `>go to line`, *Then* "Go to line" is the highlighted `role="option"` (`combobox` + `listbox`, `aria-activedescendant`) and Enter runs it.
- Shortcuts documented.
  - *Given* the `?` overlay, *When* opened, *Then* Files shortcuts (Ctrl+P, Ctrl+Shift+F, Ctrl+G, Alt+[ / Alt+], Alt+W, Ctrl+S, F4/Shift+F4) are listed, plus Esc-then-Tab and, with vim on, Esc Esc and Leave editor; no entry is a browser-reserved chord.
- No browser-reserved chords (triad round 2, ux.md 14.1, U-CHORD-1).
  - *Given* the registered shortcut table, *When* `reservedChords.test.ts` compares it to the denylist (Ctrl/Cmd+W/T/N, Ctrl/Cmd+Shift+W/T/N, Ctrl+Tab and Ctrl+Shift+Tab, Ctrl+PageUp/Down, Ctrl/Cmd+1..9, Alt+Left/Right/Home, Ctrl/Cmd+L/K/E/D/R, F5/F6/F11/F12, Ctrl/Cmd+Shift+I/J/C/M/Delete, Ctrl/Cmd+Q), *Then* the test fails on any overlap and on any chord without a visible button or menu equivalent; *When* `files-shortcuts.spec.ts` runs on desktop Chromium, Firefox and WebKit, *Then* each documented chord reaches its handler with focus inside the Files tab and is `defaultPrevented` exactly once, and with focus outside the tab it is not prevented. Playwright injects events below the browser's accelerator layer, so real delivery is not proven by CI; the owner's per-browser manual check (Chrome, Firefox, Safari) is a checklist line in Story 2.5.2.
- Non-US layouts and AltGr (triad round 3, ux.md 14.1, U-CHORD-2).
  - *Given* the chord matcher, *When* it receives `Alt+BracketLeft` (no Ctrl/Meta), *Then* jumplist Back runs and the event is `defaultPrevented`; *When* it receives an AltGr composition (`altKey` and `ctrlKey` both true, or `getModifierState('AltGraph')`, as for AltGr+8 which types `[` on a German layout), *Then* nothing runs and the event is not prevented, so a typed bracket is never swallowed.
  - *Given* a stubbed `navigator.keyboard.getLayoutMap()` mapping `BracketLeft` to `ü`, *When* the shortcut overlay and tooltips render, *Then* the chord is shown as `Alt+Ü`; *Given* the API is absent (Firefox, Safari), *Then* the overlay shows `Alt+[` followed by "(the key at the US [ position)"; the toolbar and bottom-bar buttons remain the primary route.
- Analytics (ADR-035).
  - *Given* the palette in file, command, line and outline modes, *When* a command is run or the palette is cancelled, *Then* `files.nav.palette` is emitted with `mode`, `outcome` and, for command mode only, `command` from the fixed palette command-id enum (`none` for every other mode); typed palette text is never a label.
**Files**: `web-app/src/components/sessions/files/StatusBar.tsx`, `web-app/src/components/sessions/QuickOpenPalette.tsx`, `web-app/src/lib/hooks/useKeyboard.ts`, `web-app/src/components/ui/KeyboardShortcutOverlay.tsx`, `web-app/src/components/sessions/__tests__/StatusBar.test.tsx`, `web-app/src/lib/hooks/__tests__/reservedChords.test.ts`, `tests/e2e/files-shortcuts.spec.ts`

##### Task 1.3.5a: `StatusBar` incl. read-only reason chip and `aria-live` message region (S)
- Files: `web-app/src/components/sessions/files/StatusBar.tsx`, `web-app/src/components/sessions/__tests__/StatusBar.test.tsx`

##### Task 1.3.5b: Command mode in `QuickOpenPalette` + shortcut registration (M)
- Audit existing palette against combobox/listbox ARIA; add `>` command mode; register shortcuts in `useKeyboard.ts` and the overlay, using the ux.md 14.1 chord table (matching `Alt+[`, `Alt+]`, `Alt+W` by `event.code`, refusing AltGr compositions, and rendering layout-resolved physical-key labels through `chordLabels.ts`).
- Files: `web-app/src/components/sessions/QuickOpenPalette.tsx`, `web-app/src/lib/hooks/useKeyboard.ts`, `web-app/src/components/ui/KeyboardShortcutOverlay.tsx`, `web-app/src/lib/hooks/chordLabels.ts`, `web-app/src/lib/hooks/__tests__/chordLabels.test.ts`

##### Task 1.3.5c: Reserved-chord denylist test and the per-browser shortcut spec (S)
- `reservedChords.test.ts` (denylist constant, shortcut-table cross-check, visible-equivalent check, the AltGr non-match case and the layout-label cases of the Story 1.3.5 AC) and `tests/e2e/files-shortcuts.spec.ts` (header `// @feature files:tabs`; ARIA-role locators; chord injected with focus inside and outside the tab); add informational `desktop-firefox` and `desktop-webkit` projects limited to this spec to `playwright.config.ts` (WebKit availability per Task 0.1.4b; Firefox install is checked in CI the same way, `continue-on-error` if unreliable, Chromium gating). Runs after Task 1.3.5b.
- Files: `web-app/src/lib/hooks/__tests__/reservedChords.test.ts`, `tests/e2e/files-shortcuts.spec.ts`, `tests/e2e/playwright.config.ts`

#### Story 1.3.6: Keyboard-operable resizable panes
**As a** keyboard or screen-reader user, **I want** to resize the Explorer|Search panel without a mouse, **so that** no function is drag-only (ux.md U-A11Y-4).
**Acceptance Criteria**:
- Separator semantics (VERIFIED gap: `TreeResizeHandle.tsx` and `useResizablePanel.ts` today expose only pointer handlers; no `role`, `aria-*`, `tabIndex` or key handling).
  - *Given* the panel is expanded, *When* the handle is inspected, *Then* it has `role="separator"`, `aria-orientation="vertical"`, `tabIndex=0`, an accessible name "Resize file panel", and `aria-valuenow`/`aria-valuemin`/`aria-valuemax` equal to the current, minimum and maximum panel width in px.
- Arrow keys resize, Home/End jump.
  - *Given* the handle is focused at width 300, *When* ArrowRight is pressed, *Then* the width becomes 316 and `aria-valuenow=316`; Shift+ArrowRight adds 64; ArrowLeft/Shift+ArrowLeft subtract; Home sets the minimum and End the maximum (both clamped to the same bounds as dragging); the new width persists via the existing localStorage key.
- Existing collapse unaffected.
  - *Given* Ctrl+B and the collapse button, *When* used, *Then* they behave as before and the handle is not rendered while collapsed; `useResizablePanel.test.ts` passes unchanged.
**Files**: `web-app/src/components/sessions/TreeResizeHandle.tsx`, `web-app/src/lib/hooks/useResizablePanel.ts`, `web-app/src/lib/hooks/useResizablePanel.test.ts`, `tests/e2e/files-a11y.spec.ts`

##### Task 1.3.6a: Keyboard support in `useResizablePanel` and `TreeResizeHandle` (S)
- Add `step`/`largeStep` and a `keyboardHandlerProps` (role, aria values, `tabIndex`, `onKeyDown`) to the hook's `handleProps` and widen `TreeResizeHandleProps` to pass them through, so `FilesTab.tsx` (which spreads `{...panel.handleProps}`) needs no edit and stays out of the ordering list. Unit tests extend `useResizablePanel.test.ts`.
- Files: `web-app/src/lib/hooks/useResizablePanel.ts`, `web-app/src/components/sessions/TreeResizeHandle.tsx`, `web-app/src/lib/hooks/useResizablePanel.test.ts`

##### Task 1.3.6b: Separator e2e in the new a11y spec (S)
- Start `tests/e2e/files-a11y.spec.ts` (header `// @feature files:tabs`; ARIA-role locators only; axe cases for U-A11Y-1/2/3/5 are added to the same file by Story 1.5.3b): focus the separator, press ArrowRight/Home/End, assert `aria-valuenow` and the rendered width.
- Files: `tests/e2e/files-a11y.spec.ts`, `tests/e2e/pages/files-tab.page.ts`


#### Story 1.3.7: Banner stack, loading states and success feedback (triad repair)
**As a** user, **I want** every wait to look like a wait, every failure to say what happened and how to leave it, and the most important banner never to be hidden, **so that** I am never staring at a blank pane or missing a "Save rejected".
**Acceptance Criteria**:
- Banner priority and the never-hidden rule (ux.md section 8.2, U-EDIT-15).
  - *Given* four active banners including "Save rejected" and three informational ones, *When* the stack renders, *Then* at most two show, sorted by the documented priority (rejected/unknown > deleted > flag-off > stale-with-edits > offline > permanent read-only > beyond-loaded > stale-clean > informational), "Save rejected" is visible and not inside "+N more", "+N more" is a button opening the full list, and the blocking banner is `role="alert"` with no Dismiss while every other banner is `role="status"` (the earlier convention line "banners use role=status" was corrected in ux.md section 0); a jest property test over random banner sets asserts a priority-1 banner is always in the visible set.
- Collapsed blocking banner keeps its actions (triad round 3, ux.md 7.1a step 2 / 8.2, U-MOB-14).
  - *Given* a priority-1 banner (Save rejected, Save outcome unknown, or Draft not stored) and a `collapsed` stack (short Edit viewport), *When* it renders at 320px, *Then* the stack is one 44 px row with the one-line text (tap expands it) and **Copy my version** and **Retry Save** (**Retry backup** for Draft not stored) as two 44x44 buttons, never moved into a menu and never collapsed into "+N more"; the "N notices" chip has a 44x44 hit area; "Draft not stored while dirty" is a priority-1 `BannerKind` registered by Task 2.2.2b.
- Loading and failure states for every wait (ux.md section 8.5, U-STATE-1).
  - *Given* `GetFileContent` pending longer than 150 ms (fake timers), *Then* the tab shows a spinner and "Opening b.go..." with `aria-busy`, and on failure "Could not open b.go: <class>" with **Retry** and **Close tab**; *Given* the same call failing for an open dirty buffer (reload/restore), *Then* the buffer, its content and its dirty state are unchanged and a banner offers Retry and Copy my version; *Given* tree load, tab restore, outline parse, an explicit `StatFile` (Save/Reload) and the merge library load, *Then* each shows its listed pending state after 150 ms and its listed failure with an exit; *Given* a draft check slower than 150 ms, *Then* the editor is already editable and the Restore prompt appears when ready.
- Analytics (ADR-035).
  - *Given* an open that fails as not found, too large, binary, non-UTF-8, worktree unavailable or an RPC error, *When* the failure state renders, *Then* one `files.open.failed{reason}` is emitted with the enum reason only (no path or error text).
- Success feedback (ux.md section 8.6, U-STATE-2).
  - *Given* Saved, Merge saved, Reload, Draft restored, Overwrite and Undo-eviction, *When* each completes, *Then* a polite message is announced in the status-bar region and, on mobile, a toast shows above the bar without covering the Save control (Playwright mobile bounding-box check); every toast stays at least 8 s, pauses while hovered or focused, has a dismiss control and uses no motion under `prefers-reduced-motion` (fake-timer and `emulateMedia` tests; ux.md section 0, U-TOAST-1).
**Files**: `web-app/src/components/sessions/files/BannerStack.tsx`, `web-app/src/lib/files/bannerPriority.ts`, `web-app/src/components/sessions/files/PendingState.tsx`, `web-app/src/lib/hooks/useFilesFeedback.ts`, `web-app/src/lib/files/__tests__/bannerPriority.test.ts`, `web-app/src/components/sessions/__tests__/BannerStack.test.tsx`

##### Task 1.3.7a: `bannerPriority.ts` and `BannerStack` (replaces direct `StaleBanner` placement) (M)
- Typed `BannerKind` union with a total priority function; later stories register their banners (Save rejected, Draft not stored, Offline, Flag off...) into the same stack; `StaleBanner` becomes a kind; a `collapsed` mode renders the single 44 px row with the two action buttons of a priority-1 banner (consumed by the Task 2.4.2c ladder).
- Files: `web-app/src/lib/files/bannerPriority.ts`, `web-app/src/components/sessions/files/BannerStack.tsx`, `web-app/src/components/sessions/files/StaleBanner.tsx`, `web-app/src/components/sessions/FilesTab.tsx`

##### Task 1.3.7b: `PendingState`, failure states and `useFilesFeedback` success messages (M)
- 150 ms threshold helper with injected clock; wires the open, tree, restore, outline, `StatFile` and merge-load states; success messages routed to the status bar region and the mobile toast. One-line registration in `FilesTab.tsx`.
- Files: `web-app/src/components/sessions/files/PendingState.tsx`, `web-app/src/lib/hooks/useFilesFeedback.ts`, `web-app/src/components/sessions/FilesTab.tsx`

## Epic 1.4: Search UI
**Goal**: The flagship flow: search, jump, step through hits, go back.

#### Story 1.4.1: Streaming search client
**As a** developer, **I want** a debounced, abortable streaming client, **so that** typing never leaves stale results.
**Acceptance Criteria**:
- Debounce and min length.
  - *Given* the user types `f`,`fo`,`foo` within 300 ms, *When* the debounce fires, *Then* exactly one request with `foo` is sent; a 1-character query sends none.
- Abort on new query.
  - *Given* an in-flight stream for `foo`, *When* the query becomes `foob`, *Then* the first stream is aborted, its late batches are ignored (request-id guard), and only `foob` results render.
- Queued state and cancel.
  - *Given* the stream's first message is `Queued{ahead=1}`, *When* the hook handles it, *Then* state is `queued` (distinct from `searching`) with position, and Cancel aborts the stream and returns to idle; the first `batch` moves state to `searching`.
- Busy refusal is a state, not a retry loop (engineering triad ET-8, Story 1.1.2 stream cap).
  - *Given* a `ResourceExhausted` `SearchBusy` refusal (stream cap or queue-wait timeout), *When* the hook handles it, *Then* state is `busy` (distinct from `error`), prior results stay, no automatic retry is scheduled (asserted with fake timers advancing 60 s), **Retry** starts exactly one new request, and the stream is aborted when the panel closes, the hook unmounts or the document has been hidden for 30 s.
- Invalid regex keeps prior results.
  - *Given* results for `foo`, *When* `(` with regex on returns `InvalidArgument`, *Then* an inline error is set and prior results stay.
- Analytics and latency (ADR-035).
  - *Given* typing `foo` quickly, *When* the debounced request is sent, *Then* exactly one `files.search.run` is emitted (`query_len` bucket, `regex`, `case_sensitive`, `whole_word`, `has_globs`, `include_ignored`) and the pattern text is not in any event; *Given* the first `batch` and the final `stats`, *Then* `files.perf.search_first_result_ms` (`scale`, `queued`) and `files.perf.search_total_ms` (`outcome`, `scale`) carry `durationMs`; *Given* a queued state or a cancel, *Then* `files.search.queued` (`ahead`) and `files.search.cancel` (`phase`, `results` bucket) are emitted; typing that is aborted before the debounce fires emits nothing.
**Files**: `web-app/src/lib/files/searchClient.ts`, `web-app/src/lib/hooks/useContentSearch.ts`, `web-app/src/lib/files/__tests__/searchClient.test.ts`

##### Task 1.4.1a: `searchClient.ts` + `useContentSearch` (M)
- Uses the generated client with `AbortSignal`; accumulates batches; applies the unary fallback if ADR-030 recorded one; maps `ResourceExhausted` `SearchBusy` to the `busy` state and aborts on unmount or after 30 s hidden (Story 1.1.2 stream cap).
- Files: `web-app/src/lib/files/searchClient.ts`, `web-app/src/lib/hooks/useContentSearch.ts`

##### Task 1.4.1b: Tests (shared transport fixture) (S)
- Files: `web-app/src/lib/files/__tests__/searchClient.test.ts`

#### Story 1.4.2: Search panel and results
**As a** user, **I want** grouped results with highlighted hits and next/prev stepping, **so that** I can review usages quickly.
**Acceptance Criteria**:
- Grouped, virtualized results with counts.
  - *Given* 27 matches in 6 files, *When* results render, *Then* a header announces "27 results in 6 files" via `aria-live="polite"`, file groups are collapsible, and 5,000 rows render via `@tanstack/react-virtual` without mounting all.
- Open at line, results persist.
  - *Given* a hit `src/a.go:42`, *When* Enter is pressed on it, *Then* a preview `Buffer` opens at line 42 with the match highlighted, the results panel stays visible, and `Esc` returns focus to the editor.
- Quickfix stepping.
  - *Given* a `QuickfixCursor` at 3 of 27, *When* F4 is pressed, *Then* hit 4 opens; Shift+F4 returns to 3.
- Queued state is shown.
  - *Given* `useContentSearch` state `queued`, *When* the header renders, *Then* it reads "Waiting for other searches (1 ahead)..." with a **Cancel** button (`data-testid=search-cancel`), prior results stay dimmed-visible, and `aria-live=polite` announces the state once.
- Empty/truncated states.
  - *Given* zero results with case-sensitive+regex toggles on, *When* rendered, *Then* "No matches for `foo` (case-sensitive, regex)" and a "Clear filters" button appear; truncated shows "Showing first 1000 - refine your query".
- Beyond-truncation hit.
  - *Given* a hit at line 40,000 in a file whose loaded portion ends at line 20,000, *When* opened, *Then* a banner "Line 40000 is beyond the loaded portion" shows and the view clamps to the last line.
- Cancelled, failure and refine affordances (ux.md S1, U-SEARCH-5/7).
  - *Given* a search cancelled after 312 results, *Then* "Search cancelled - 312 results so far" shows with the partial results usable; *Given* an RPC failure, *Then* "Search failed - check your connection" with **Retry** shows and prior results stay; *Given* a truncated set, *Then* the "Add filters" link focuses the include/exclude fields.
- Not colour-only.
  - *Given* a result row, *When* inspected, *Then* the match is in a `<mark>` element.
- Valid, virtualization-safe ARIA (triad round 2, ux.md 3.5, U-SEARCH-15; a `listbox` cannot hold collapsible groups).
  - *Given* 5,000 rows virtualized, *When* any window of rows is mounted, *Then* the container is `role="tree"` with `aria-label="Search results"`, every mounted group header is a `treeitem` with `aria-level=1`, `aria-expanded`, `aria-setsize`/`aria-posinset` over the groups, every hit row is a `treeitem` with `aria-level=2` and `aria-setsize`/`aria-posinset` within its group, the hit row's accessible name is "<path> line N, <text>, match k of m, result r of t" (plus the unsaved-buffer marker when applicable), `aria-activedescendant` on the query input points at the active treeitem on desktop, rows are focusable (roving `tabindex=-1`) on `pointer: coarse`, and axe reports no `aria-required-*`, `aria-valid-attr-value` or nested-interactive violation (jest-axe in the component test plus the Playwright axe case of Task 1.5.3b).
- Idle state is never blank (ux.md 3.4, U-SEARCH-16).
  - *Given* the panel opened with no query, *Then* the helper block ("Search file contents in <session name>", "Type at least 2 characters", toggle legend) shows and no request is sent; *Given* retained results, *Then* they show dimmed with "Results for `foo` - edit to search again".
- First tap with the keyboard open (ux.md 3.5, U-SEARCH-14).
  - *Given* the query input focused with the soft keyboard up (viewport stubbed 844 -> 500 in Playwright mobile), *When* one tap lands on result row 3, *Then* row 3 opens exactly once (the row identity is captured on `pointerdown`, the input is blurred, activation happens on `pointerup`, no re-hit-test after relayout); *When* the pointer moves more than 10 px between down and up, *Then* nothing opens (scroll). Real-device behavior is a checklist line in Story 2.5.2.
- Search with a dirty open buffer (triad repair, ux.md section 3.4, U-SEARCH-13).
  - *Given* a hit in `a.go` whose open buffer is dirty, *When* results render, *Then* the row shows "buffer has unsaved changes; line may differ" (also in its accessible name); *When* it is activated and the matched text is not on the buffer's hit line, *Then* the view jumps to the nearest line within 200 lines containing the matched text and announces "Moved to line N (buffer has unsaved changes)", or announces "Match not found in your unsaved version" and clamps to the hit line; the buffer text is never modified or re-read from disk.
- Analytics (ADR-035).
  - *Given* a completed search and a hit opened in 3 interactions (open Search, type, tap hit), *When* the hit opens, *Then* `files.search.results` (`outcome`, `results`, `files`, `queued`) was emitted on the final stats and `files.search.open_hit` carries `interactions=3`, `rank`, `via`, `reused_tab` from the `FindFlowMeter`; a flow cancelled before a hit emits no `open_hit`.
**Files**: `web-app/src/components/sessions/files/SearchPanel.tsx`, `web-app/src/components/sessions/files/SearchResults.tsx`, `web-app/src/components/sessions/files/SearchPanel.css.ts`, `web-app/src/components/sessions/FilesTab.tsx`, `web-app/src/components/sessions/__tests__/SearchPanel.test.tsx`

##### Task 1.4.2a: `SearchResults` virtualized grouped list (M)
- Renders the `tree`/`treeitem` structure with explicit `aria-level`/`aria-setsize`/`aria-posinset` (virtualization removes siblings from the DOM) and the accessible-name format above; touch rows are 44 px and focusable.
- Files: `web-app/src/components/sessions/files/SearchResults.tsx`, `web-app/src/components/sessions/files/SearchPanel.css.ts`

##### Task 1.4.2b: `SearchPanel` (query, toggles `Aa`/word/`.*`, include/exclude, include-ignored) + Explorer|Search header (M)
- Ctrl+Shift+F, retains tree and results state; `data-testid`s; paused-session empty state on `WorktreeMissing` (no root label); `busy` message with Retry (Story 1.4.1).
- Files: `web-app/src/components/sessions/files/SearchPanel.tsx`, `web-app/src/components/sessions/FilesTab.tsx`

##### Task 1.4.2d: Idle helper state and first-tap activation with the keyboard open (S)
- `SearchIdleState` (small component, inside `SearchPanel.tsx` or its own file if the panel nears the line budget) and a `useFirstTapActivation(rowRef)` hook implementing the pointerdown-capture / pointerup-activate / 10 px scroll threshold rule; jest tests with synthetic pointer events and the Playwright mobile case with the stubbed viewport (added to Task 1.5.3b's spec file list, not this task's).
- Files: `web-app/src/components/sessions/files/SearchPanel.tsx`, `web-app/src/lib/hooks/useFirstTapActivation.ts`, `web-app/src/components/sessions/__tests__/SearchPanel.test.tsx`

##### Task 1.4.2c: Quickfix chip + highlight on open + tests (M)
- Highlight via a transient CodeMirror decoration; `QuickfixCursor` in `useContentSearch`; dirty-buffer marker and text-match fallback jump (`jumpToMatchNear`, in `extensions/`); `FindFlowMeter` wiring for `files.search.open_hit`.
- Files: `web-app/src/components/sessions/files/SearchResults.tsx`, `web-app/src/components/sessions/CodeEditor.tsx`, `web-app/src/components/sessions/__tests__/SearchPanel.test.tsx`

#### Story 1.4.3: Desktop search + tabs e2e
**As the** owner, **I want** CI to prove the flagship flow, **so that** regressions are caught.
**Acceptance Criteria**:
- Search to line in <= 3 interactions.
  - *Given* the isolated e2e server with a seeded worktree containing `needle_fn` in `pkg/a.go:12`, *When* the test clicks Search, types `needle_fn`, presses Enter on the hit, *Then* the editor shows `pkg/a.go` with the cursor on line 12.
- Two buffers keep position.
  - *Given* `a.go` scrolled to line 900 and `b.go` open, *When* switching back to `a.go`, *Then* line 900 is still visible.
- Conventions.
  - *Given* the new specs, *When* CI's convention checks run, *Then* they have the `// @feature` header, no `waitForTimeout`, and only `data-testid`/ARIA locators.
**Files**: `tests/e2e/files-search.spec.ts`, `tests/e2e/files-viewer.spec.ts`, `tests/e2e/pages/files-tab.page.ts`

##### Task 1.4.3a: Page object and specs (M)
- Header `// @feature files:content-search, files:tabs`; seed via test-dir worktree; add `docs/registry/features/files-tabs.json` via `make registry-generate`.
- Files: `tests/e2e/pages/files-tab.page.ts`, `tests/e2e/files-search.spec.ts`, `tests/e2e/files-viewer.spec.ts`, `docs/registry/features/files-tabs.json`

## Epic 1.5: Mobile
**Goal**: All read-only flows one-handed with 44px targets and no layout break when the keyboard opens.

#### Story 1.5.1: Three-pane mobile layout
**As a** phone user, **I want** a nav-stack of Files, Editor, Search with a thumb-zone action bar, **so that** I can navigate one-handed.
**Acceptance Criteria**:
- Panes and back.
  - *Given* viewport 390x844 and `PaneState=files`, *When* a file is tapped, *Then* `PaneState=editor` and the top-bar back arrow returns to `files`; browser back does the same without leaving the session.
- Bottom action bar.
  - *Given* the editor pane, *When* rendered, *Then* the bar shows Files | Search | Quick-open | Go-to-line | Back/Forward, each >= 44px tall, and `padding-bottom: env(safe-area-inset-bottom)`.
- Tab switcher sheet.
  - *Given* 3 open buffers, *When* the `[3]` button is tapped, *Then* a sheet lists 3 rows of >= 44px with close buttons, focus is trapped, Esc/back dismisses, and `prefers-reduced-motion` disables the slide animation.
  - *Given* a pinned row, a preview row and a dirty row, *When* the sheet renders, *Then* each row's accessible name carries "pinned", "preview" or "unsaved" with a glyph, each row has a 44x44 actions button (also opened by long-press) with Pin/Unpin, Close, Close others and Copy path, and Unpin turns a pinned row into a preview row that the next open may replace (U-TAB-9); *Given* the cap refused an open, *Then* the sheet opens with the refusal message at its top and Close highlighted on eligible rows only (U-TAB-10).
- Tablet (>= 768px, coarse pointer; ux.md 7.3, U-TABLET-1).
  - *Given* 820x1180 with `pointer: coarse`, *When* `FilesTab` renders, *Then* the desktop layout shows (no FAB, no bottom bar, no tab sheet), all controls are >= 44px, the breadcrumb-row toolbar shows **Quick open**, **Go to line** and **Search** buttons >= 44px (the Touch profile has no Ctrl chords and no bottom bar; with the Hardware profile they are the 24px desktop size; U-TABLET-1), and the `InputProfile`-keyed pieces (key bar, vim default) follow the resolved profile rather than the breakpoint (jest with a stubbed `matchMedia`; the Playwright tablet case is added to Task 1.5.3b).
- One layout rule (triad round 3, ux.md section 0, U-MOB-16).
  - *Given* `chooseFilesLayout`, *When* it is evaluated over the table {container 390, 767, 768, 844 x coarse/fine x screen short side 390, 600, 820}, *Then* `mobile` is returned exactly when `containerWidth < 768` or (`pointerCoarse` and `screenShortSide < 600`): 844 wide coarse with short side 390 (a landscape phone) is `mobile`, 820 or 1180 wide coarse with short side 820 is `desktop` and stays `desktop` when the container height drops to 470 because the soft keyboard opened (the rule reads `screen`, not the visual viewport), and rotating a phone never changes the answer; a layout change preserves the buffers and does not remount the `EditorView`. Playwright checks 844x390 (mobile layout, mobile editing ladder) and 1180x820 coarse (desktop layout) in Task 1.5.3b.
- Search pane results persistence.
  - *Given* results for `foo`, *When* a hit is opened and Back pressed, *Then* the results list returns with scroll position preserved.
- Narrow bottom bar and floating-control stacking (triad repair, ux.md section 7.1, U-MOB-9/10).
  - *Given* viewport 320x568, *When* the editor pane renders, *Then* the bottom bar shows Files | Search | Quick-open | Back | More with every target >= 44px and `scrollWidth <= innerWidth`, Go-to-line and Forward are reachable in <= 2 taps via More, and at >= 360px all five groups show with Back/Forward as a split control.
  - *Given* the quickfix chip and the Edit FAB (Task 2.4.2b) both shown at 390x844 and 320x568, *When* measured, *Then* the chip sits 8px above the bar, the FAB 8px above the chip, neither overlaps the other, the bar or the extra-key bar, the editor's bottom padding equals the stack height, and dismissing the chip lowers the FAB. `MobileFilesLayout` owns one `FloatingControls` stack container that both register into (so they cannot overlap by construction).
- Android system Back with the soft keyboard up (triad round 3, ux.md 7.4 rule 6, U-MOB-4).
  - *Given* the editor pane with the keyboard up (stubbed `visualViewport` shrink), *When* the viewport resizes back to full height without a `popstate`, *Then* the pane, the Edit mode and the buffer are unchanged and no history entry was pushed or popped by the resize; *When* `popstate` then arrives, *Then* the pane pops as in the table. Whether Android Chrome and the installed PWA deliver Back this way is UNVERIFIED and is a line of the Task 2.5.4a script.
- Analytics (ADR-035).
  - *Given* a tap on a file, the bottom-bar Search button and browser Back, *When* each changes the `PaneState`, *Then* `files.mobile.pane_switch` is emitted with `from`, `to` and `via` {bottom_bar, back_arrow, system_back, tap_hit}.
**Files**: `web-app/src/components/sessions/files/MobileFilesLayout.tsx`, `web-app/src/components/sessions/files/MobileFilesLayout.css.ts`, `web-app/src/components/sessions/files/TabSwitcherSheet.tsx`, `web-app/src/components/sessions/FilesTab.tsx`

##### Task 1.5.1a: `MobileFilesLayout` + top bar + bottom bar (M)
- Reuse `ViewportProvider`/`--viewport-height`; hide bottom bar while keyboard open; `history.pushState` for pane stack (guarded against router races); < 360px bottom-bar collapse with a More popover; `FloatingControls` stack container.
- Files: `web-app/src/components/sessions/files/MobileFilesLayout.tsx`, `web-app/src/components/sessions/files/MobileFilesLayout.css.ts`

##### Task 1.5.1b: `TabSwitcherSheet` and search full-screen pane (M)
- The sheet rows carry the pin glyph, the state words in the accessible name and the per-row actions menu (Pin/Unpin, Close, Close others, Copy path; long-press opens the same menu).
- Files: `web-app/src/components/sessions/files/TabSwitcherSheet.tsx`, `web-app/src/components/sessions/files/SearchPanel.tsx`

##### Task 1.5.1c: `chooseFilesLayout`, wire `FilesTab` between layouts, tablet toolbar buttons (M; was S before triad round 3)
- Pure `chooseFilesLayout` (rule in the Story 1.5.1 AC and ux.md section 0) with its table test, `FilesTab` consuming it instead of a bare 768px test (container width from the existing `ResizeObserver`/`ViewportProvider`, `screen` read once), and the desktop-layout breadcrumb-row **Quick open / Go to line / Search** buttons (shown at 44px under the Touch profile, 24px under Hardware). One-line registrations in `FilesTab.tsx`; the pure function and toolbar live in their own files.
- Files: `web-app/src/components/sessions/FilesTab.tsx`, `web-app/src/lib/files/chooseFilesLayout.ts`, `web-app/src/lib/files/__tests__/chooseFilesLayout.test.ts`, `web-app/src/components/sessions/__tests__/FilesTabToolbar.test.tsx`

#### Story 1.5.2: Mobile read mode and large-file degradation
**As a** phone user, **I want** tapping to read to not raise the keyboard and big files not to freeze, **so that** reading is smooth.
**Acceptance Criteria**:
- No keyboard in Read mode.
  - *Given* `EditorMode=Read` on a touch device, *When* the user taps code, *Then* the editor content is not focused/`inputmode="none"` and `visualViewport.height` is unchanged.
- Large-file degrade.
  - *Given* a 600 KB file, *When* opened on `pointer: coarse`, *Then* syntax highlight, bracket matching, active-line highlight and fold gutter are off and a one-line notice shows; a 4,000-char single-line file falls back to wrapping off with horizontal scroll.
- Font stepper.
  - *Given* the overflow menu, *When* "A+" is pressed, *Then* editor font size increases by 1px steps and is >= 16px on coarse pointers.
- Windowing replaces degrading for large files (ADR-036; amended by Task 1.6.5c).
  - *Given* `files:range-read` on and a transport advertising `READ_LINES`, *When* a file over 1 MiB opens on a phone, *Then* it is served windowed (Story 1.6.5) instead of being degraded whole, the 600 KB degrade rule still applies to T0 files under 1 MiB on `pointer: coarse`, and the profile is applied through `EditorEngine.setMode` rather than by reaching into CM6.
**Files**: `web-app/src/components/sessions/CodeEditor.tsx`, `web-app/src/lib/files/degradeProfile.ts`, `web-app/src/lib/files/__tests__/degradeProfile.test.ts`

##### Task 1.5.2a: `degradeProfile` (thresholds as named constants, adjustable) (S)
- Files: `web-app/src/lib/files/degradeProfile.ts`, `web-app/src/lib/files/__tests__/degradeProfile.test.ts`

##### Task 1.5.2b: Apply profile + read-mode input suppression + font stepper in `CodeEditor` (M)
- Files: `web-app/src/components/sessions/CodeEditor.tsx`

#### Story 1.5.3: Mobile e2e and a11y
**As the** owner, **I want** mobile viewport tests in CI, **so that** layout regressions are caught without my phone.
**Acceptance Criteria**:
- Open, search, jump, back on mobile.
  - *Given* the gating Playwright mobile project (`mobile-webkit`, or `mobile-chromium` per Task 0.1.4b) at 390x844, *When* the test opens a file, searches `needle_fn`, taps the hit, then Back, *Then* it lands on `pkg/a.go:12` and returns to results.
- Back order and URL (pre-mortem #11).
  - *Given* the gating mobile project, *When* the test opens a hit from results, then presses Back twice, *Then* it lands on editor -> results -> files in that order and the URL stays inside the session (no exit to the session list); the deep link `?file=pkg/a.go&line=12` still lands on line 12 after one Back and forward.
- Owner phone smoke of the read-only flows before editing work starts.
  - *Given* Epic 1.5 merged on the isolated instance (`:62871`), *When* the owner opens, searches, jumps and rotates on the phone (read-only; no `files:editing`), *Then* any layout, keyboard-viewport or history defect is filed before Epics 2.2 and 2.4 begin (Task 1.5.3c; the Story 0.1.5 spike already de-risked the primitives). **Gate G1 includes the Accessible view (Story 1.5.4, which is merged before the gate) and a screen-reader quick check** (triad round 3): in the same sitting the owner turns the Accessible view on from the skip link, opens a file of at least 1,000 lines, and with VoiceOver (iOS) or TalkBack (Android) confirms R1 and R2 of Story 0.1.6 against the built component (swipe reads consecutive numbered lines, the pager and Go to line are reachable, the skip link is first); a failure is filed and fixed before Wave 7.
- Small screen reflow.
  - *Given* 320x568, *When* the Files tab renders, *Then* no horizontal page scroll exists (`document.scrollingElement.scrollWidth <= innerWidth`).
- Keyboard layout (stubbed).
  - *Given* `visualViewport.height` stubbed from 844 to 500 with a resize event, *When* the editor is focused, *Then* the bottom bar is hidden and the editor container height equals 500.
- Accessibility.
  - *Given* axe-core on the Files tab (desktop and mobile), *When* run, *Then* zero WCAG AA violations.
- Automated hooks for reflow, non-text contrast and mobile focus order (triad round 2, ux.md U-A11Y-2/5/8).
  - *Given* 320x568 with root `font-size: 200%`, *When* the Files tab renders in Read and Edit states, *Then* `document.scrollingElement.scrollWidth <= innerWidth`, no primary control's bounding box leaves the viewport horizontally, and no two primary controls overlap (helper `assertReflow(page)` in `tests/e2e/helpers/a11y.ts`).
  - *Given* the dirty dot, pin glyph, diff `+`/`-` glyph, focus ring and a pressed toggle (found by `data-testid`), *When* `assertNonTextContrast(page, testIds, 3)` computes each indicator's contrast against its computed background under `prefers-color-scheme: light` and `dark`, *Then* every ratio is >= 3:1 (axe does not check non-text contrast, so this is a custom helper; the judgment cases stay in the manual spot check).
  - *Given* 390x844 and 320x568, *When* the test presses Tab repeatedly from the top bar, *Then* the sequence of accessible names is back, file name, tab count, Save, Stop editing (Edit mode only), overflow, banners, pane content, floating controls, key bar, bottom bar (U-A11Y-8), and a sheet traps focus until closed.
  - *Given* the tablet viewport 820x1180 with `hasTouch` and a stubbed soft keyboard, *Then* the U-TABLET-1 assertions hold (desktop layout, 44 px targets, Quick open / Go to line / Search buttons in the toolbar, key bar above the keyboard); *Given* 1180x820 `hasTouch` with the viewport height stubbed to 470, *Then* the layout stays desktop; *Given* 844x390 `isMobile` and `hasTouch`, *Then* the mobile layout renders (U-MOB-16).
**Files**: `tests/e2e/files-mobile.spec.ts`, `tests/e2e/files-a11y.spec.ts`, `tests/e2e/playwright.config.ts`, `tests/e2e/pages/files-tab.page.ts`

##### Task 1.5.3a: Mobile Playwright projects (S)
- Add `mobile-webkit` and `mobile-chromium` projects limited to `files-mobile.spec.ts`; which one gates is decided by the Task 0.1.4b result (the other runs informationally).
- Files: `tests/e2e/playwright.config.ts`

##### Task 1.5.3c: Owner phone smoke of read-only mobile flows on the isolated instance (XS)
- **Gate G1, a wall-clock blocker (owner; engineering triad ET-1).** Wave 6 ends here, and no task of Epic 2.2, 2.3 or 2.4 starts until the owner has recorded the smoke result (or filed the defects it found) in `manual-verification-checklist.md`. The smoke covers the read-only flows **and the Accessible view with a screen-reader quick check** (R1/R2 against the built component, Story 1.5.3 AC; Story 1.5.4 is merged before this task, triad round 3). Epic 2.1 (server write path, ships dark) is not blocked. Stories 1.5.1-1.5.3 and the e2e tasks of Wave 6 run before it, so the smoke tests the merged Epic 1.5. ADR-036: if Epic 1.6 is merged and `files:range-read` is on in the isolated instance, the same sitting adds an informational line (scroll and jump in a 50 MB file); its full owner check is Task 1.6.7c and does not block G1.
- Files: `project_plans/files-viewer-revamp/implementation/manual-verification-checklist.md`

##### Task 1.5.3b: `files-mobile.spec.ts` incl. axe and visualViewport stub (M)
- Includes the open-hit, Back, Back pane-order and URL assertion, the first-tap-with-keyboard case (U-SEARCH-14), the tablet case, and the `assertReflow` / `assertNonTextContrast` / focus-order helpers in `tests/e2e/helpers/a11y.ts` (listed in this task's Files). Does not claim real soft-keyboard behavior; header `// @feature files:mobile, files:content-search`. Also extends `files-a11y.spec.ts` (created in Task 1.3.6b) with the axe and keyboard-only cases for U-A11Y-1/2/3/5.
- Files: `tests/e2e/files-mobile.spec.ts`, `tests/e2e/files-a11y.spec.ts`, `tests/e2e/helpers/a11y.ts`, `tests/e2e/pages/files-tab.page.ts`

#### Story 1.5.4: Accessible view, the non-virtual read path (triad round 2; before Gate G1)
**As a** screen-reader user, **I want** a plain, fully rendered, paged view of the file I have open, **so that** I can read all of it without depending on CodeMirror's virtualized DOM.
**Acceptance Criteria**:
- Structure (design from ux.md 8.10; shape confirmed by Story 0.1.6).
  - *Given* a 1,200-line buffer, *When* the Accessible view is on, *Then* a `region` labelled "<file>, accessible view" holds a pager ("Lines 1-500 of 1200", Previous/Next page buttons >= 44 px, a Go-to-line input) and a `<pre tabindex="0">` with exactly the 500 lines of the current page as text, each prefixed by its line number; changing page announces "Lines 501-1000 of 1200" politely and moves focus to the new `<pre>`.
- Navigation integration.
  - *Given* a search hit or Go to line for line 1,034, *When* activated, *Then* the page containing it is shown and the line's element is focused and announced "Line 1034"; the quickfix chip and F4 step through hits the same way; jumplist Back restores the previous page and line.
- Reachable before any code is read.
  - *Given* a file open, *When* a keyboard or screen-reader user moves into the editor region, *Then* the first focusable element is the skip link "Switch to accessible view" (and "Switch to code editor" in the other view), the toggle is also in the mobile overflow menu, the desktop toolbar and the palette, and the choice persists per device.
- Read-only; the Edit hand-off is a seam here and is built in Wave 7 (triad round 3: Edit mode does not exist in Wave 6, so the earlier AC here could not be satisfied).
  - *Given* Wave 6 (no Edit mode, `files:editing` off), *When* the Accessible view renders, *Then* no Edit control is rendered, `AccessibleView` exposes an `onEditRequested?(line)` prop that the parent does not pass yet, and a stub assertion proves that passing a callback renders a 44 px **Edit** button that calls it once with the line last focused (and that omitting it renders nothing); the text comes from the buffer's `EditorState` for `Full` buffers (no second copy of the document); for `Windowed` buffers (ADR-036, Task 1.6.5c) each 500-line page is one `FileTransport.read(lines)` call, so the view never depends on the editor engine and works for unloaded ranges. The real hand-off (CodeMirror mounts in Edit at that line, "Editing b.go, line N" announced, dirty state and tab mode shared, preview tab pinned) is asserted in Story 2.2.1 / Task 2.2.1b, where Edit mode is built.
- A dirty buffer is shown as its draft (triad round 3, ux.md 8.10, U-EDIT-22).
  - *Given* a dirty buffer, *When* the Accessible view is on, *Then* it renders the `EditorState` text (the unsaved edits), the region's accessible name ends ", unsaved changes", the pager row shows a visible "Unsaved changes" label and a 44 px **Save** button on the single save path (reasons for a disabled Save per ux.md 7.1), and the banner stack renders above the region; *Given* a clean buffer, *Then* neither the label nor Save is rendered.
- Cost and degrade.
  - *Given* a 1 MB file, *When* the Accessible view is on, *Then* at most one page of lines is in the DOM and page change completes within 100 ms with a generous CI tolerance; binary, truncated and over-cap states render their usual banners.
- Accessibility checks.
  - *Given* axe-core on the view, *Then* zero WCAG AA violations; the `accessibleViewStructure.test.tsx` structure test from Story 0.1.6 is kept and extended to the production component.
- Analytics (ADR-035).
  - *Given* the view toggled on from the skip link and off from the toolbar, *Then* `files.a11y.accessible_view` is emitted with `to`, `via` and `size` only.
**Files**: `web-app/src/components/sessions/files/AccessibleView.tsx`, `web-app/src/components/sessions/files/AccessibleView.css.ts`, `web-app/src/lib/hooks/useAccessibleView.ts`, `web-app/src/components/sessions/__tests__/AccessibleView.test.tsx`, `web-app/src/components/sessions/files/MobileFilesLayout.tsx`, `web-app/src/components/sessions/QuickOpenPalette.tsx`

##### Task 1.5.4a: `AccessibleView` component, pager, Go-to-line, draft/dirty rendering and tests (M)
- Grown from the Task 0.1.6a prototype (and its structure test); pure `paginateLines(text, pageSize=500)` helper (written over a `PageSource` interface so Task 1.6.5c can swap the text source for `FileTransport.read(lines)` without changing the component); focus and announcement handling; reads the active buffer text (the draft) through the `CodeEditor` handle (`getState()`), never from a second copy; the dirty label and Save button of the AC above; the `onEditRequested` seam (stub assertion only; wired in Task 2.2.1b).
- Files: `web-app/src/components/sessions/files/AccessibleView.tsx`, `web-app/src/components/sessions/files/AccessibleView.css.ts`, `web-app/src/components/sessions/__tests__/AccessibleView.test.tsx`

##### Task 1.5.4b: Toggle, skip link, persistence, Overflow and palette entries, navigation routing (M; was S before triad round 3)
- `useAccessibleView` (per-device persisted flag, try/catch), skip link, the mobile Overflow item and the desktop toolbar toggle, the palette command `toggle_accessible_view` (already a member of the ADR-035 `files.nav.palette` command enum, so `events.ts` is not edited; the palette command list in `QuickOpenPalette.tsx` gains the entry), the `files.a11y.accessible_view` emit, hit / Go-to-line / quickfix routing into the page-and-line focus, jumplist Back restoring page and line; one-line registrations in `FilesTab.tsx` and `MobileFilesLayout.tsx` (ordered after Task 1.5.1c, which also edits both, in Wave 6). **The Edit hand-off is not in this task** (moved to Task 2.2.1b; this task only keeps the `onEditRequested` seam unwired). Re-classed M: it touches the palette, Overflow, the desktop toolbar, routing and analytics, not one hook.
- Files: `web-app/src/lib/hooks/useAccessibleView.ts`, `web-app/src/components/sessions/files/MobileFilesLayout.tsx`, `web-app/src/components/sessions/FilesTab.tsx`, `web-app/src/components/sessions/QuickOpenPalette.tsx`, `web-app/src/components/sessions/__tests__/AccessibleView.test.tsx`

## Epic 1.6: Range reads and the windowed viewer (ADR-036 Phase B; flag `files:range-read`; ships before any editing epic)
**Goal**: Open, scroll, jump to a line and search files far beyond 1 MiB (T1: 1 MiB to `hash_cap_bytes`, T2: up to the read ceiling, view-only) without reading them whole, on desktop and on the owner's phone, with the `full` adapter as the always-working floor. Constants (64 MiB `hash_cap_bytes`, 1 GiB read ceiling, 4,096-line index checkpoints, 400-line / 128 KiB default windows, 4,000-line / 1 MiB hard caps) are the ADR-036 proposals, confirmed or replaced by the S-T2 outcome (Story 0.1.7) before Story 1.6.1 starts. Everything is dark until `files:range-read` is flipped after the Task 1.6.7c owner check.

#### Story 1.6.1: Server sparse line index and background `Analyze`
**As a** user opening a 50 MB log, **I want** the server to know where line N starts and what the file's line endings and hash are, **so that** any window is one small read and editing can wait for a trustworthy token.
**Acceptance Criteria**:
- One pass produces everything.
  - *Given* a generated file with 2,000,000 lines (LF), *When* `Analyze` runs, *Then* it returns a sparse index with a checkpoint every 4,096 lines (about 490 checkpoints), counts of `\n`, `\r\n`, lone `\r` and NUL, the maximum line length, the whole-file sha256 and the line count, and line-to-byte for line 1,234,567 equals the byte offset found by a naive scan (property test on random line-length distributions, including a file with no trailing newline, an empty file and a single 300 KiB line).
- Cached by validator, background, bounded.
  - *Given* the analysis cache keyed by `(path, size, mtime_ns, inode)` (LRU of 32 entries), *When* a file is read by range for the first time, *Then* a background `Analyze` starts under a process-wide semaphore of 1 (configurable 1-2) with ctx cancellation when the last interested request goes away, `analysis=pending` is reported until it finishes and `ready` after; *When* the file is appended to or rewritten (validator change), *Then* the entry is invalidated and rebuilt in full (no incremental extension); `StatFile{want_analysis}` returns the state and `StatFile{want_sha256}` returns the sha256 once ready (else `analysis=pending`).
- Token behavior is unchanged.
  - *Given* a T1 file with analysis pending, *When* a range read returns, *Then* its token is `v1:<size>:<mtime_ns>:-` and non-editable; *Given* analysis ready, *Then* tokens carry the sha256 and equal `NewFileVersionToken` over the same bytes (the ADR-031 format is untouched).
- Failure and cancellation.
  - *Given* a 256 MB file and `ctx` cancelled at 50 ms, *Then* `Analyze` returns `context.Canceled` within 200 ms and `goleak` is clean; *Given* the file changes during the pass (an agent appends), *Then* the result is discarded and marked stale (never cached under the new validator).
**Files**: `server/services/file_analyze.go`, `server/services/file_lineindex.go`, `server/services/file_analysis_cache.go`, `server/services/file_analyze_test.go`, `server/services/file_analysis_cache_test.go`

##### Task 1.6.1a: `Analyze`, the sparse line index and pure helpers (M)
- Pure, backend-agnostic functions: single streaming pass with a pooled buffered reader (index checkpoints, EOL/NUL/max-line statistics, sha256), `LineToByte(index, line)` (nearest checkpoint then bounded scan, at most 4,096 lines), `ByteToLine`, BOM awareness (offsets stay raw), `FuzzAnalyze` and table tests. Constants named at the top of `file_analyze.go`.
- Files: `server/services/file_analyze.go`, `server/services/file_lineindex.go`, `server/services/file_analyze_test.go`

##### Task 1.6.1b: Analysis cache, scheduler and `StatFile` analysis fields (M)
- `file_analysis_cache.go` (LRU, validator key, semaphore, ctx ownership by reference count, stale-discard on change), `FileOps.Stat` fills `analysis` and `sha256` per `want_*` (edit in `file_ops.go` after Task 0.2.6c), tests with the in-memory backend's fault injection (change during pass, cancel, eviction), `-race`, `goleak`.
- Files: `server/services/file_analysis_cache.go`, `server/services/file_analysis_cache_test.go`, `server/services/file_ops.go`

#### Story 1.6.2: `ReadFileRange` RPC and `FileOps.ReadRange`
**As a** client, **I want** line-aligned windows with enough metadata to place them, join them and detect staleness, **so that** the viewer never needs the whole file.
**Acceptance Criteria**:
- Line window.
  - *Given* a 50 MB LF file, *When* `ReadFileRange{lines, anchor_line=1_000_000, dir=around, want_lines=400, max_bytes=128 KiB}` is called, *Then* the response holds a contiguous, line-aligned window around line 1,000,000 (its text LF-normalized), `start_line`, `end_line`, `start_byte`, `end_byte` (raw-file offsets), `bof=false`, `eof=false`, `line_count_known` per the analysis state, the validator and token, a CRC32C of the served bytes and `torn=false`; `want_lines` above 4,000 or `max_bytes` above 1 MiB are clamped, and a window never exceeds either cap.
- Byte anchor.
  - *Given* a byte anchor inside a multibyte rune, *Then* it snaps back over at most three continuation bytes to a rune start; *Given* an anchor in the last window, *Then* `eof=true`.
- BOM, CRLF, binary, non-UTF-8, long lines.
  - *Given* a CRLF+BOM file, *Then* window text is LF-normalized, the first window sets `has_bom` and strips it, and every offset stays a raw-file offset (the BOM never shifts them); *Given* NUL in the first 8 KB, *Then* `is_binary=true`; *Given* NUL after the first 8 KB, *Then* the window containing it sets `contains_nul_at`; *Given* invalid UTF-8 in a window, *Then* `non_utf8=true`; *Given* a line over 256 KiB, *Then* the region switches to byte-window mode (`long_line=true`, `partial_last_line` possible, read-only).
- Torn and stale reads.
  - *Given* the file changes between the pre-read `fstat` and the post-read `fstat`, *Then* the response sets `torn=true`; *Given* a request carrying `if_validator` that no longer matches, *Then* the response reports the new validator so the client can drop its cache.
- First-chunk shortcut.
  - *Given* `prefer_full_if_le=1 MiB` and a 300 KB file, *Then* one response carries the whole file (T0 behavior in one round trip) with the normal token (hash computed inline because the file is small), and for a 5 MB file it returns the first window plus `analysis=pending`.
- Policy lives in `FileOps` (ADR-036 decision 1).
  - *Given* `files:range-read=false`, a hidden session, a path escaping via symlink, `.git/config`, and a session whose `ActiveDir != ExistingDir`, *When* `ReadFileRange` is called, *Then* each returns the same typed refusal as `GetFileContent` would (`FailedPrecondition FlagOff`, `PermissionDenied`, `WorktreeMissing`) and nothing is read.
- Limits and observability.
  - *Given* a fifth concurrent range call from one client (4 allowed) or a ninth from one remote IP (8 allowed), *Then* `ResourceExhausted` with a `RangeBusy` detail; *Given* any call, *Then* one slog line (session, path, kind, bytes, lines, duration, outcome) and one `RecordRead`-style metric without file content; the shared limiter seam is `content_search_limits.go`'s (Story 1.1.2).
**Files**: `proto/session/v1/session.proto`, `server/services/file_ops_range.go`, `server/services/file_ops_range_test.go`, `server/services/files_delegates.go`, `server/services/content_search_limits.go`

##### Task 1.6.2a: `ReadFileRange` proto (S)
- `ReadFileRangeRequest{ref, oneof anchor {lines{anchor_line, dir, want_lines}, bytes{offset, length, dir}}, max_bytes, prefer_full_if_le, if_validator, caps_epoch}` and response with the fields named in the AC (`start_line`, `end_line`, `start_byte`, `end_byte`, `bof`, `eof`, `line_count_known`, `line_count`, `validator`, `version_token`, `chunk_crc32c`, `torn`, `eol_class`, `has_bom`, `is_binary`, `contains_nul_at`, `non_utf8`, `long_line`, `partial_last_line`, `analysis`, `caps_epoch`). `make proto-gen`; commit only the `.proto`. Ordered after Task 1.1.1a in `session.proto`.
- Files: `proto/session/v1/session.proto`

##### Task 1.6.2b: `FileOps.ReadRange` (M)
- Line/byte windows over the backend and the analysis cache (sequential reads from line 1 and byte anchors work before analysis), UTF-8 snapping, LF normalization with raw offsets, 8 KB NUL sniff, long-line mode, CRC32C (stdlib `hash/crc32` Castagnoli), pre/post `fstat` torn detection, `prefer_full_if_le`, flag and policy checks via `FilePolicy`; no function above funlen/gocognit limits (helpers). The capability reply gains `READ_LINES`/`READ_BYTES`, the range limits and the raised `hash_cap_bytes` and `max_concurrent_ranges` when `files:range-read` is on (edit in `file_ops_capabilities.go`, the Wave 6 single editor; Task 2.1.5e edits it again in Wave 7).
- Files: `server/services/file_ops_range.go`, `server/services/file_ops_range_test.go`, `server/services/file_ops_capabilities.go`

##### Task 1.6.2c: Delegate, range limiter, metrics, registry (S)
- Thin handler in `files_delegates.go`; `rangeLimiter` (4 per client, 8 per IP, the same remote-address keying and caveat as ADR-030 decision 6) added beside `streamLimiter` in `content_search_limits.go`; metrics in `file_metrics.go`; `// +api: files:read-range`; `docs/registry/features/files-read-range.json` via `make registry-generate`. `ReadFileRange` is a read RPC and is not added to `guardedProcedures` (reads are not guarded today; asserted so the decision is explicit).
- Files: `server/services/files_delegates.go`, `server/services/content_search_limits.go`, `server/services/file_metrics.go`, `docs/registry/features/files-read-range.json`

##### Task 1.6.2d: `FileOpsConformance` read cases and the range edge corpus (M)
- Adds range cases to the Story 0.2.6 suite over both backends: BOF/EOF, UTF-8 boundary, CRLF/BOM, binary-after-8 KB, long line, torn read, stale validator, flag off, confinement corpus reuse (Story 2.1.4's symlink and `.git` matrix applied to `ReadFileRange`), limiter refusals, and capability honesty for `READ_LINES`/`READ_BYTES`.
- Files: `server/services/file_ops_conformance_test.go`, `server/services/testdata/files_range/`, `server/services/file_ops_range_corpus_test.go`

#### Story 1.6.3: `ConnectUnaryTransport` reads, chunk cache and prefetch
**As a** user scrolling a large file, **I want** the next window to be there before I reach it and a changed file to be noticed, **so that** big files feel like normal ones without holding connections open.
**Acceptance Criteria**:
- Reads through the port.
  - *Given* a server advertising `READ_LINES`, *When* the router resolves `read` to `connect_unary`, *Then* `read({kind:"lines", anchorLine, dir, wantLines, maxBytes})` issues one unary `ReadFileRange`, verifies the CRC32C of the decoded bytes (a mismatch is a transport-class failure and retries once), and returns the served range and validator; `read({kind:"all"})` on a file at most 1 MiB uses `prefer_full_if_le` so a T0 open is still one round trip.
- Staleness and torn windows.
  - *Given* a response whose validator differs from the cached one, *Then* every cached chunk of that file is dropped and a "changed on disk" event is raised to the Story 1.3.4 banner path; *Given* `torn=true`, *Then* the client retries once and, if still torn, treats the window as stale (no partial text is shown as current).
- Chunk cache.
  - *Given* an 8 MiB budget (`pointer: coarse`) or 64 MiB (desktop), *When* 200 windows of 128 KiB are read, *Then* the cache holds at most the budget (LRU by bytes), keyed by `(sessionId, path, validator)`, and a validator change empties the file's entries; a jest memory test with a fake transport asserts the byte accounting.
- Prefetch is bounded and unary.
  - *Given* viewport window W, *Then* W plus one window ahead and one behind are kept; *Given* scroll velocity above one window per second, *Then* two windows are prefetched in the scroll direction; at most 2 ranges are in flight per buffer and 4 per tab; a direction change or a jump aborts in-flight prefetches (`AbortSignal`); no stream is opened (HTTP/1.1 pool, ADR-030 decision 6).
- Demotion and conformance.
  - *Given* three consecutive network failures of `read`, *Then* the op demotes to `full` (files over 1 MiB then show the existing truncated read-only state with the "Using simpler file mode" chip), application errors never demote, and `runTransportConformance` passes for `full` and `connect_unary` (read side).
- Analytics.
  - *Given* a window read, *Then* `files.read.window{transport, payload, outcome, cache, direction}` is emitted (sampled by the Story 0.2.5 rate cap) with `durationMs`, and no offset, line or path.
**Files**: `web-app/src/lib/files/transport/connectUnaryTransport.ts`, `web-app/src/lib/files/transport/chunkCache.ts`, `web-app/src/lib/files/transport/prefetch.ts`, `web-app/src/lib/files/transport/crc32c.ts`, `web-app/src/lib/files/transport/__tests__/connectUnaryTransport.test.ts`

##### Task 1.6.3a: `ConnectUnaryTransport` read and stat ops (M)
- `read` for lines/bytes/all, CRC32C verification (table-driven CRC32C in `crc32c.ts`, also used by Story 2.2.5's encoder), validator/staleness hook, torn retry, `prefer_full_if_le`, typed `Rejected` mapping, `stat` with `want_analysis`/`want_sha256`, capability registration with the router (patch comes in Task 2.2.5c).
- Files: `web-app/src/lib/files/transport/connectUnaryTransport.ts`, `web-app/src/lib/files/transport/crc32c.ts`, `web-app/src/lib/files/transport/__tests__/connectUnaryTransport.test.ts`

##### Task 1.6.3b: `ChunkCache` and prefetch policy (M)
- LRU by bytes with the coarse/desktop budgets as named constants, validator-keyed invalidation, window stitching (several cached chunks fill one window), prefetch scheduler (one ahead/behind, two at speed, caps 2 per buffer / 4 per tab, abort on direction change), fake-timer and fake-transport tests.
- Files: `web-app/src/lib/files/transport/chunkCache.ts`, `web-app/src/lib/files/transport/prefetch.ts`, `web-app/src/lib/files/transport/__tests__/chunkCache.test.ts`, `web-app/src/lib/files/transport/__tests__/prefetch.test.ts`

##### Task 1.6.3c: Read-side conformance, demotion tests and transport analytics (S)
- Runs the read half of `runTransportConformance` against `full` and `connect_unary` (capability honesty, range edges via a fake server built on the Go golden windows, staleness, typed error mapping, abort), the demotion ladder tests, and the `files.transport.*` / `files.read.window` emits through `useFilesAnalytics()`.
- Files: `web-app/src/lib/files/transport/__tests__/transportConformance.ts`, `web-app/src/lib/files/transport/__tests__/connectUnaryTransport.conformance.test.ts`, `web-app/src/lib/files/transport/__tests__/demotion.test.ts`

#### Story 1.6.4: `cm6LazyDoc` gap-line module in `CmEngine` (gated on Task 0.1.4d)
**As a** user scrolling a 1,000,000-line file, **I want** one continuous scrollable document with a proportional scrollbar and working line numbers, **so that** it behaves like a normal editor even though most of it is not loaded.
**Acceptance Criteria**:
- One document, placeholders for gaps.
  - *Given* a `VirtualDoc` of 1,000,000 lines with lines 1-400 loaded, *When* the engine mounts it, *Then* the document holds the loaded lines plus one placeholder line for the gap, drawn as a block-replace widget whose `estimatedHeight` is `gapLines * lineHeight` (so the scrollbar is proportional to the whole file), the gutter shows true absolute line numbers through the gap table (line 401 after a loaded window shows 401, a window starting at 500,000 shows 500,000), and the placeholder cannot be typed into, deleted into or partly selected for deletion (`changeFilter` and `atomicRanges`).
- Loads and evictions are invisible to history and dirty state.
  - *Given* 100 load/evict cycles interleaved with 20 user edits, *Then* undo depth equals the number of user edits (load/evict transactions are `addToHistory.of(false)` and never mark the buffer dirty), undoing returns exactly the user's text, and evicting is refused for any range that contains an unsaved edit.
- Scroll anchoring.
  - *Given* a gap above the viewport loads while the user reads, *Then* the top visible line moves by at most one line height (desktop Chromium and the WebKit project); *Given* `reveal(line 750,000)` from line 1, *Then* the target window loads, the line is centered within one line height and the intermediate gap stays a placeholder.
- Absolute coordinates for edits across gaps.
  - *Given* edits in two loaded regions separated by a gap, *When* `takePending()` is called, *Then* the pending edits are expressed in absolute base line/col_utf16 and equal the full-replace oracle on 1,000 seeded random edit scripts (the Story 0.1.4c oracle promoted to a regression test); chunk loads that happen between edits are mapped (`pending.map(loadChange)`).
- Memory.
  - *Given* a virtual 50 MB file and the phone budget, *When* the viewport sweeps the whole file, *Then* resident chunk text stays at or under 8 MiB (clean chunks far from the viewport are evicted back to placeholders) and no listener or timer leaks (jest leak check).
- Engine conformance.
  - *Given* `runEngineConformance`, *When* `features.lazyDoc` is true, *Then* the lazy cases (range provide/evict keeps the scroll anchor; edits across a gap map to absolute positions) run and pass; highlighting beyond 1 MiB of loaded text is off (the degrade profile of Story 1.5.2) and in-editor find sees loaded text only (project find is `SearchContent`).
**Files**: `web-app/src/components/sessions/files/engine/cm6LazyDoc/gapTable.ts`, `web-app/src/components/sessions/files/engine/cm6LazyDoc/loadEvict.ts`, `web-app/src/components/sessions/files/engine/cm6LazyDoc/scrollAnchor.ts`, `web-app/src/components/sessions/files/engine/CmEngine.ts`, `web-app/src/components/sessions/files/engine/__tests__/cm6LazyDoc.test.tsx`

##### Task 1.6.4a: Gap table, placeholder widget and absolute gutter (M)
- `StateField` holding the gap table (placeholder document position to absolute line range), the placeholder block widget with `estimatedHeight`, `lineNumbers({formatNumber})` reading the table, pure helpers `absoluteLine(pos)` and `gapAt(line)` with table tests.
- Files: `web-app/src/components/sessions/files/engine/cm6LazyDoc/gapTable.ts`, `web-app/src/components/sessions/files/engine/cm6LazyDoc/placeholderWidget.ts`, `web-app/src/components/sessions/files/engine/cm6LazyDoc/gutter.ts`, `web-app/src/components/sessions/files/engine/__tests__/cm6LazyDoc.test.tsx`

##### Task 1.6.4b: Load/evict transactions, edit guards and scroll anchoring (M)
- `loadRange`/`evictRange` as annotated non-history, non-dirty transactions, `changeFilter` plus `atomicRanges` protecting placeholders, refuse eviction of dirty ranges, scroll anchoring for loads above the viewport (scroll snapshot/`scrollIntoView` effect), `onNeedRange` driven by the viewport with a debounce; all with the composition rule (no dispatch while `view.composing`; loads queue until `compositionend`).
- Files: `web-app/src/components/sessions/files/engine/cm6LazyDoc/loadEvict.ts`, `web-app/src/components/sessions/files/engine/cm6LazyDoc/guards.ts`, `web-app/src/components/sessions/files/engine/cm6LazyDoc/scrollAnchor.ts`, `web-app/src/components/sessions/files/engine/__tests__/cm6LazyDoc.test.tsx`

##### Task 1.6.4c: `CmEngine` lazy integration and the lazy conformance cases (M)
- `CmEngine.setSource(VirtualDoc)`, `features.lazyDoc=true`, `reveal` to far lines, `getViewport`, the registration line in `CodeEditor.tsx` (after Task 1.5.2b), the lazy cases of `runEngineConformance`, the 50 MB memory sweep, and the Playwright regression of Task 0.1.4c's spec against the production module (desktop and the gating mobile project).
- Files: `web-app/src/components/sessions/files/engine/CmEngine.ts`, `web-app/src/components/sessions/CodeEditor.tsx`, `web-app/src/components/sessions/files/engine/__tests__/engineConformance.ts`, `tests/e2e/files-lazydoc-spike.spec.ts`

##### Task 1.6.4d: Window-swap fallback mode (a1) (S, conditional on Task 0.1.4d recording a2 FAIL and a1 PASS)
- Implements the a1 model behind the same `EditorEngine` interface (one window per `EditorState`, edge-triggered `setState`, anchor line preserved, line-offset gutter, single editable window with an "Edit this window" affordance, windows pinned while dirty) and flips `features.lazyDoc` semantics to "windowed"; skipped, and recorded as skipped in the PR description, when a2 passes. Counted in the totals so the plan does not under-cost the branch.
- Files: `web-app/src/components/sessions/files/engine/cm6LazyDoc/windowSwap.ts`, `web-app/src/components/sessions/files/engine/CmEngine.ts`, `web-app/src/components/sessions/files/engine/__tests__/cm6WindowSwap.test.tsx`

#### Story 1.6.5: Windowed viewer, large-file states and the Accessible view through the transport
**As a** user, **I want** a large file to open at the right line with clear loading, indexing and read-only states, **so that** I know what I am looking at and why I cannot edit it.
**Acceptance Criteria**:
- Tier detection and buffer source (ADR-032 amended: `Buffer.Loaded.source: Full | Windowed{windowStartLine, lineCount?, analysis}`; persisted tab state stores `(path, anchorLine)` only, never chunk text).
  - *Given* `files:range-read` on and a 300 KB file, *Then* it opens as `Full` (T0, one round trip, identical to the base plan); *Given* a 5 MB file, *Then* it opens `Windowed` (T1) at line 1 (or at the requested line) with the first window painted from one range read; *Given* a 200 MB file, *Then* it opens `Windowed` and read-only with `ReadOnlyReason=TooLarge` (T2) shown in the reason chip (a T1 file with `files:edit-large` off or `files:delta-write` off is read-only with the same `TooLarge` reason); *Given* `files:range-read` off or a server without `READ_LINES`, *Then* the base-plan behavior holds (truncated 1 MiB read-only) with no error.
  - *Given* a persisted tab set containing a T1 file at anchor line 812,345, *When* the page reloads, *Then* the buffer reopens at that line from one range read and no chunk text was stored.
- Open at a far line (deep link, search hit, Go to line).
  - *Given* a hit at line 4,100,000 of a 300 MB file, *When* it is opened, *Then* one `read(lines, around)` call (not a scan from line 1) paints the window within the Story 1.2.1 SLO tolerance, the cursor is on the hit line, and the status bar reads "Line 4,100,000 of about 4.2 million" until the analysis finishes, then the exact count; *Given* Go to line beyond the end, *Then* it clamps to the last line and says so.
- Visible states (ux.md 8.12; each has an exit path).
  - *Given* a gap that is loading, *Then* the placeholder reads "Loading lines A-B..." (polite announcement after 500 ms); *Given* a failed load, *Then* "Couldn't load lines A-B. Retry" with a 44 px Retry; *Given* analysis pending, *Then* an "Indexing file..." chip; *Given* a transport demotion, *Then* the quiet "Using simpler file mode" chip; *Given* the file changed on disk while open, *Then* the Story 1.3.4 banner, and Reload re-anchors the same line.
- Accessible view reads through the transport (Story 1.5.4 amended).
  - *Given* a 5 MB file and the Accessible view on, *Then* each 500-line page is one `read(lines)` call, the pager reads "Lines 1-500 of about N" until the count is exact, a failed page announces "Couldn't load lines 501-1000. Retry" politely, and a T0 file still pages from the buffer text with no extra call.
- Degrade profile (Story 1.5.2 amended).
  - *Given* a 700 KB file on `pointer: coarse`, *Then* the degrade profile of Story 1.5.2 applies as before (windowing is not used under 1 MiB); *Given* a 5 MB file on a phone, *Then* it is windowed (not degraded whole) and highlighting follows the loaded-text rule of Story 1.6.4.
- Analytics.
  - *Given* an open of a T1 file, *Then* `files.tab.open` carries `size=gt1m` and `files.perf.open_ms` carries `degraded=false` plus the `tier` label; no offset or line is emitted.
**Files**: `web-app/src/lib/files/bufferReducer.ts`, `web-app/src/lib/files/windowedSource.ts`, `web-app/src/components/sessions/FileContentViewer.tsx`, `web-app/src/components/sessions/files/LargeFileStates.tsx`, `web-app/src/components/sessions/files/AccessibleView.tsx`

##### Task 1.6.5a: Buffer `source`, tier detection and the `VirtualDoc` bridge (M)
- Reducer arm `source: Full | Windowed{...}` and `windowedSource.ts` (builds the engine's `VirtualDoc` from the router's `read`, the chunk cache and the analysis state; T0/T1/T2 classification from size and `hash_cap_bytes`), `FileContentViewer` opens through the port, `CodeEditor.tsx` one-line registration of the lazy engine mode (after Task 1.6.4c; ordered after Task 1.5.2b), `ReadOnlyReason=TooLarge` for T2; persisted tab state stores anchor line only. Runs after Tasks 1.6.3b and 1.6.4c.
- Files: `web-app/src/lib/files/bufferReducer.ts`, `web-app/src/lib/files/windowedSource.ts`, `web-app/src/components/sessions/FileContentViewer.tsx`, `web-app/src/components/sessions/CodeEditor.tsx`, `web-app/src/lib/files/__tests__/windowedSource.test.ts`

##### Task 1.6.5b: Large-file states, status bar copy and far-jump progress (M)
- `LargeFileStates.tsx` (placeholder copy and retry, "Indexing file..." chip, "about N lines" text, Go-to-line progress with Cancel for far jumps, T2 reason chip, fallback chip, stale re-anchor), status-bar segments in `StatusBar.tsx`, banner priorities registered in `bannerPriority.ts` (indexing and fallback are low priority; load failure is a status banner), `aria-live` messages through the Story 1.3.7 feedback hook; component tests with the fake transport. It is mounted by `FileContentViewer.tsx` (edited in Task 1.6.5a), so `FilesTab.tsx` is not touched by Epic 1.6.
- Files: `web-app/src/components/sessions/files/LargeFileStates.tsx`, `web-app/src/components/sessions/files/LargeFileStates.css.ts`, `web-app/src/components/sessions/files/StatusBar.tsx`, `web-app/src/lib/files/bannerPriority.ts`, `web-app/src/components/sessions/__tests__/LargeFileStates.test.tsx`

##### Task 1.6.5c: Accessible view via `read(lines)` and the windowing/degrade rule (S)
- `AccessibleView` takes its page text from the transport for `Windowed` buffers (pager copy "of about N", loading/failed announcements), keeps the buffer-text path for `Full`; `degradeProfile` stops degrading whole files over 1 MiB when a transport with `READ_LINES` is active. Ordered after Task 1.5.4b (same view and test file).
- Files: `web-app/src/components/sessions/files/AccessibleView.tsx`, `web-app/src/lib/files/degradeProfile.ts`, `web-app/src/components/sessions/__tests__/AccessibleView.test.tsx`, `web-app/src/lib/files/__tests__/degradeProfile.test.ts`

#### Story 1.6.6: Large-file search: request-scoped file cap and hit `byte_offset` (ADR-030 amended)
**As a** user, **I want** content search to find matches in big files and open them at the right place, **so that** "huge files searchable" is true.
**Acceptance Criteria**:
- The per-file cap is request-scoped (ADR-030 decision 1 amended; the default stays 2 MB).
  - *Given* a 40 MB log with a match at line 900,000 and `SearchOptions.large_files=false`, *Then* the file is skipped past 2 MB as before; *Given* `large_files=true`, *Then* the scanner reads up to `hash_cap_bytes` (64 MiB) under a per-file time budget and a worker count lowered to `min(2, GOMAXPROCS/2)`, the match is emitted, the scan buffers stay at `workers x pooled reader` (the 64 MB heap ceiling test of Story 1.1.1 still holds), and a file past the cap or the budget is reported in `SearchStats` (`large_files_skipped`), never silently missed.
- Hits carry a byte offset.
  - *Given* any hit, *Then* `SearchMatch.byte_offset` is the raw-file offset of the matched line start (not PII, not logged, not sent to analytics) and opening the hit issues `read(lines, around line)` (works before the index exists: a byte-anchored read renumbers from `start_line`).
- One limiter family.
  - *Given* the Story 1.1.2 open-stream limiter and the Story 1.6.2 range limiter, *When* a search and a prefetch storm run together, *Then* they share the per-IP accounting seam (range calls have their own higher cap) and a refused range call never cancels a search.
- UI toggle.
  - *Given* the search panel, *When* "Include large files" is turned on (default off, per device), *Then* the request carries `large_files=true`; hits in files over 2 MB show a "large file" badge, and the zero-results copy lists the toggle among the active filters.
- Analytics (ADR-035).
  - *Given* a search with large files, *Then* `files.search.run` carries `large_files` (bool) and `files.server.search` carries `large` (bool); no path or offset.
**Files**: `server/services/content_search.go`, `server/services/content_search_match.go`, `server/services/content_search_test.go`, `web-app/src/components/sessions/files/SearchPanel.tsx`, `web-app/src/lib/files/searchClient.ts`

##### Task 1.6.6a: Request-scoped per-file cap, `byte_offset` and the shared limiter (M)
- Proto fields `large_files` and `SearchMatch.byte_offset` ride Task 1.1.1a (added there so `session.proto` is not edited again in Wave 6); this task implements the scanner mode (cap raised to `hash_cap_bytes` under the time budget and lower worker count), fills `byte_offset` in `content_search_match.go`, `SearchStats.large_files_skipped`, and the limiter accounting; tests include the counting-reader assertion adapted to the raised cap and the heap sampler.
- Files: `server/services/content_search.go`, `server/services/content_search_match.go`, `server/services/content_search_test.go`

##### Task 1.6.6b: Large-file toggle, hit opening via `read(lines)` and badge (S)
- `searchClient.ts` passes `large_files`; `SearchPanel.tsx` toggle and badge (after Task 1.5.1b); opening a hit routes through the transport (Story 1.6.5a). Tests extend `SearchPanel.test.tsx` with the shared harness.
- Files: `web-app/src/components/sessions/files/SearchPanel.tsx`, `web-app/src/lib/files/searchClient.ts`, `web-app/src/components/sessions/__tests__/SearchPanel.test.tsx`

#### Story 1.6.7: Phase B verification: e2e matrix, benchmarks and the owner phone check
**As the** owner, **I want** the large-file read path measured and checked on my phone before it is switched on, **so that** `files:range-read` is flipped on evidence.
**Acceptance Criteria**:
- E2E matrix over `full` and `connect_unary` (CI; desktop and the gating mobile project).
  - *Given* generated 5 MB and 50 MB fixtures on the isolated e2e server, *When* the spec scrolls, jumps to line 800,000, opens a search hit in the 50 MB file and opens a 200 MB file (T2), *Then* the viewport shows the target line within the Story 1.2.1 tolerance, the JS heap stays under the phone-class budget asserted via `performance.memory` where available (else the chunk-cache byte counter), no single response exceeds 1 MiB (network assertion), the T2 file is read-only with the reason chip, and the `full` adapter run shows the base-plan truncated read-only state with the same files.
- Benchmarks are recorded, not gated.
  - *Given* the harness (run with `&`), *When* it runs, *Then* ADR-036 "Spike outcomes" records: Go `Analyze`/index at 1/16/64/256 MB, `ReadRange` latency at 128 KiB windows, a 64 MiB large-file search, TS window-swap/load time and chunk-cache memory on the phone budget, and the payload comparison (window versus whole-file read of the same file); S-T5 is re-measured with the real `SearchContent` and range load against the Epic 2.6 decision rule.
- Owner phone check (wall-clock blocker; flips `files:range-read`).
  - *Given* the isolated instance on `:62871` with a 50 MB fixture, *When* the owner scrolls fast, jumps to a far line, opens a search hit, uses the Accessible view and backgrounds/foregrounds the PWA on the phone, *Then* pass/fail per step is recorded in the checklist; only after it passes does the owner set `files:range-read` on the live instance.
**Files**: `tests/e2e/files-large.spec.ts`, `tests/e2e/helpers/largeFile.ts`, `server/services/file_ops_range_bench_test.go`, `web-app/src/lib/files/__tests__/windowBench.test.ts`, `project_plans/files-viewer-revamp/implementation/manual-verification-checklist.md`

##### Task 1.6.7a: Playwright matrix with 5 MB, 50 MB and 200 MB fixtures (M)
- `tests/e2e/files-large.spec.ts` (`// @feature files:range-read`, `data-testid`/ARIA locators, no `waitForTimeout`), a deterministic fixture generator written directly into the e2e worktree, page-object additions in `tests/e2e/pages/files-tab.page.ts` (after Task 1.5.3b), network response-size and heap assertions. Runs over `full` and `connect_unary` (the project selects the adapter through the capabilities flags of the isolated server).
- Files: `tests/e2e/files-large.spec.ts`, `tests/e2e/helpers/largeFile.ts`, `tests/e2e/pages/files-tab.page.ts`

##### Task 1.6.7b: Benchmark runs and recorded constants (S)
- Go `-benchmem` benchmarks (`Analyze` 1/16/64/256 MB, `ReadRange`, large-file search), TS window/cache benchmarks, S-T5 re-measurement with the real endpoints, all with `&`; the numbers and the confirmed constants are written into ADR-036 "Spike outcomes" and the `hash_cap_bytes` / ceiling constants adjusted in one place each if they differ.
- Files: `server/services/file_ops_range_bench_test.go`, `web-app/src/lib/files/__tests__/windowBench.test.ts`, `project_plans/files-viewer-revamp/decisions/ADR-036-pluggable-range-and-delta-file-transport.md`

##### Task 1.6.7c: Owner phone check of windowed large-file reading, recorded (XS; wall-clock blocker)
- No code; appended to the checklist after Task 1.5.3c. Blocks the live `files:range-read` flip, not any later story's code.
- Files: `project_plans/files-viewer-revamp/implementation/manual-verification-checklist.md`


---

# Phase 2: Editing (flag-gated, last)

## Epic 2.1: `WriteFile` backend
**Goal**: A save that cannot silently lose data, escape the worktree, or corrupt encoding.

#### Story 2.1.1: Atomic write primitive
**As a** developer, **I want** a tested atomic replace helper, **so that** agents never read a torn file.
**Acceptance Criteria**:
- Atomic replace with mode preserved.
  - *Given* `run.sh` mode `0755` containing `old`, *When* `atomicReplace(root, "run.sh", []byte("new"))` runs, *Then* the file reads `new`, mode is `0755`, and no `.run.sh.ssq-tmp-*` remains.
- Orphaned temp files are swept and kept out of git.
  - *Given* a stale `.run.sh.ssq-tmp-abc` (mtime 15 min old, as left by a crash between create and rename) in the target directory, *When* any write to that directory runs, *Then* the stale temp is removed (younger temps, which may belong to a concurrent writer, are left). The helper never writes outside the `os.Root` (no `info/exclude` or other git-metadata write); the residual crash window is documented in ADR-031.
- Failure cleanup.
  - *Given* a simulated write error after temp creation, *When* the helper returns, *Then* the original file is intact and the temp file is removed.
- Serialization.
  - *Given* two goroutines saving the same `ResolvedPath` concurrently, *When* run under `-race`, *Then* writes serialize through the per-path mutex and the final content is one of the two inputs, never interleaved.
**Files**: `server/services/file_atomic.go`, `server/services/file_atomic_test.go`

##### Task 2.1.1a: `atomicReplace` with `os.Root`, temp in same dir, fsync file and dir, chmod, rename (M)
- Temp name `.<name>.ssq-tmp-<rand>`, created in the **same directory** as the target (rename atomicity requires the same filesystem; writing temps outside the worktree would usually cross filesystems or be non-atomic, so that option is rejected). Exclude the pattern from tree and search; `defer` cleanup; before each write, best-effort sweep of `.*.ssq-tmp-*` older than 10 minutes in that one directory (single `ReadDir`, no recursion); **no `info/exclude` write**: the shared git common-dir lives outside the worktree `os.Root` and is not in the threat model, so it is dropped. Residual (documented, accepted): between a crash and the next write to that directory the temp file exists on disk and an agent's `git add -A` could commit it; the window is one crash plus no later save in that directory, and the file is visible in the tree filter's hidden-count and in `git status`. The earlier claim that temps could be hidden from `git status` by the tree filter alone is dropped.
- Files: `server/services/file_atomic.go`, `server/services/file_atomic_test.go`

##### Task 2.1.1b: Per-path mutex registry (process-wide) (S)
- Keyed by `(ActiveDir, ResolvedPath)` (not the relative path alone, or two sessions editing `README.md` would serialize against each other); refcounted entries removed when idle, so the registry cannot grow without bound (unlike `settingsFileLocks`); documented limit: cannot serialize against agent processes.
- Files: `server/services/file_atomic.go`

#### Story 2.1.2: `WriteFile` RPC
**As a** user, **I want** save to refuse anything unsafe and conflict with changed files, **so that** nothing is silently overwritten.
**Acceptance Criteria**:
- Happy path.
  - *Given* `files:editing=true` and `a.go` read with `FileVersionToken T1`, *When* `WriteFile(s1,"a.go",content,T1)` is called with LF-normalized `content`, *Then* the response is `Saved` with token `T2 != T1`, disk bytes equal the content with the file's own EOL/BOM re-applied exactly, and the slog line has `outcome=ok bytes=<n>` without content.
- Conflict returns current content as a sum type.
  - *Given* an agent rewrote `a.go` after the read (token now `T3`), *When* `WriteFile` is called with `T1`, *Then* the response is `Conflict{FullText{content (LF-normalized, BOM stripped), token=T3}}` and the file is unchanged.
  - *Given* the agent grew `a.go` past 1 MB, replaced it with latin-1 bytes, made it binary, or made it Mixed EOL, *When* `WriteFile` is called with `T1`, *Then* the response is `Conflict{Unavailable{reason=TooLarge|NonUtf8|Binary|MixedEol}}` with no content field and no token usable for a write; a deleted file yields `Conflict{Unavailable{Deleted}}` (not a Connect error, including when it is deleted between the `Lstat` and the token compare).
- Wire shape for refusals is one thing: Connect errors.
  - *Given* the response type `WriteFileResponse`, *When* inspected, *Then* its oneof has exactly two arms, `Saved` and `Conflict`; every refusal is a Connect error with a typed `ReadOnlyReason` detail, mapped as: `FlagOff`, `Binary`, `Truncated`, `TooLarge`, `NonUtf8`, `MixedEol`, `Symlink`, `WorktreeMissing` -> `FailedPrecondition`; `HiddenSession`, `GitInternal`, path escape -> `PermissionDenied`; an **unparseable** token string, NUL, or any `\r` in `content` -> `InvalidArgument`; a request body over the 1.5 MiB wrapper limit (Task 2.1.2f) -> `ResourceExhausted` from Connect itself, with no detail (the handler never runs), mapped by the client to `Rejected(TooLarge)`. A well-formed token whose hash segment is `-` (every truncated, binary or over-cap read yields one) parses, then is refused as `FailedPrecondition` with `ReadOnlyReason=Truncated` (or `Binary`/`TooLarge` when the file's live classification says so); the two sets are mutually exclusive. The client maps error+detail to `Rejected(reason)`, and the e2e `failed_precondition` assertion in Story 2.4.3 follows from this table.
- Flag enforced server-side.
  - *Given* `files:editing=false`, *When* a direct Connect call is made to `WriteFile`, *Then* it returns `FailedPrecondition` (reason `FlagOff`) "editing disabled" and nothing is written.
- Request size is capped before the handler (triad round 2; unconditional; ADR-031 "Request-size cap").
  - *Given* the real `localChain` **and** the real `remoteChain` (both are built over the same `srv.mux`; `guardedProcedures` is consulted only by `localChainWith`, so a limit registered beside that map would leave `:8444` unbounded; VERIFIED in `server/server.go`), *When* a `WriteFile` POST whose body is 1.5 MiB + 1 byte is sent through each, *Then* it is refused before the handler runs and nothing is read into memory beyond the limit; the refusal is Connect `ResourceExhausted` (connect-go v1.20.0 turns an `*http.MaxBytesError` into `CodeResourceExhausted` while reading the body, `connect@v1.20.0/error.go:452-466`, so the typed `ReadOnlyReason=TooLarge` detail cannot be attached to this refusal and the wire contract is "over the body limit -> `ResourceExhausted`", which the client maps to `Rejected(TooLarge)` for `WriteFile`; asserted with the real Connect client against both chains); a body of exactly 1 MiB of content (about 1.34 MiB base64 in the JSON encoding) is accepted and reaches the handler, and a content string over 1 MiB but under the body limit reaches the handler and gets `FailedPrecondition` + `TooLarge` from the validation pipeline; the limit applies to the `WriteFile` path only (another procedure's large body is unaffected, asserted).
- Archived and paused sessions never get revived by a save (triad round 2, ux.md 7.2, U-EDIT-21; `.claude/rules/noarchivedrevival.md`).
  - *Given* a session with `ArchivedAt != nil` and a client holding a dirty buffer for it, *When* `WriteFile` is called, *Then* it returns the refusal Task 0.1.3b recorded for archived sessions (`PermissionDenied HiddenSession` if the hidden-session guard covers them, else `FailedPrecondition WorktreeMissing`), writes nothing, and the test asserts the instance's lifecycle state and `ArchivedAt` are unchanged and `Start`/`RecoverFromStopped` were never invoked (fake `WorkspaceProvider`/guard spies).
- Registered in the local-listener write guard (pre-mortem #1, VERIFIED `server/server.go:1879-1888`: every comparable write RPC, `ProbeProgram`, `NudgeSessionForPR`, `PruneHiddenSessionNotifications`, `UpdateFeatureFlag`, `ReplyToPendingQuestion`, is a `guardedProcedures` member; an unregistered procedure gets no POST-only or Origin/rebinding verdict on the unauthenticated `:8543` listener).
  - *Given* `"/api"+sessionv1connect.SessionServiceWriteFileProcedure` in `guardedProcedures` with `middleware.ProfileRebinding`, *When* a Go test (modelled on `localChainWith`'s test hook and the existing guard tests, run over the real `guardedProcedures` map, not a substitute) sends a POST with a foreign `Origin`, a POST with a rebinding `Host`, and a GET, *Then* each is rejected by the guard before reaching the handler and nothing is written; a same-origin POST with a valid `Host`/`Origin` reaches the handler (and gets `FailedPrecondition FlagOff` when the flag is off).
  - *Given* the set of write RPCs added by this plan (`WriteFile`), *When* a map-membership test runs, *Then* it fails if `WriteFile` is removed from `guardedProcedures`.
- Refusals.
  - *Given* each of: truncated-read token (well-formed, hash `-`: `FailedPrecondition Truncated`), unparseable token string (`InvalidArgument`), binary target, non-UTF-8 target, content with NUL or `\r`, symlink leaf, `.git/hooks/pre-commit`, a session whose active directory is gone, hidden session, content over 1 MB, *When* `WriteFile` is called, *Then* each returns the specific error code and `ReadOnlyReason` from the table above and writes nothing.
- Never the repo root by fallback; directory-mode sessions are editable.
  - *Given* a paused worktree session whose `ExistingDir` fell back to the repo root (`ActiveDir != ExistingDir`), *When* `WriteFile` is called, *Then* it returns `FailedPrecondition WorktreeMissing` and the repo-root file is untouched.
  - *Given* a directory-mode session (`WorktreeDir == ""`, `ActiveDir == ExistingDir == RepoRoot`), *When* `WriteFile` is called with a valid token, *Then* it saves (editing the repo root is allowed by default).
- Token comparison always hashes.
  - *Given* a file rewritten to the same size with different bytes and its mtime pinned back with `os.Chtimes`, *When* `WriteFile` is called with the old token, *Then* it returns `Conflict` (size+mtime equality never accepts without comparing sha256).
- The hash is the identity; an mtime-only change is not a conflict (pre-mortem #6). The fast-path rule above still holds: a size or mtime mismatch may only trigger hashing, never acceptance on its own.
  - *Given* `a.go` read at token `v1:S:M1:H` and then `touch`ed forward with `os.Chtimes` (bytes unchanged, so the on-disk hash equals `H`), *When* `WriteFile` is called with the old token, *Then* the response is `Saved` with a new token (not `Conflict`), because the hashed on-disk content equals the content the buffer was opened from; *Given* a size mismatch, *Then* the hashes necessarily differ and `Conflict` follows; *Given* a different hash with a moved mtime, *Then* `Conflict`.
- Re-verify before rename.
  - *Given* the token check passed and an agent modifies the file just before rename, *When* the injected pre-rename hook changes the file's bytes (and mtime), *Then* the write aborts with `Conflict`; *When* the hook changes only the mtime (bytes identical), *Then* the re-check re-hashes and the write proceeds (residual TOCTOU documented in ADR-031).
- Remote-access exposure matches the recon.
  - *Given* the Task 0.1.3b findings, *When* `WriteFile` is called as each remote caller tier that can reach `/api`, *Then* the outcome equals what ADR-031 "Spike outcomes" recorded (denied for any read-only tier).
- Server analytics outcome (ADR-035, complements the slog line).
  - *Given* a fake `FilesTelemetry` and saves that end `ok`, `conflict_rejected`, `denied` (hidden-session or confinement), `disabled` (flag off) and `error`, *When* each `WriteFile` returns, *Then* exactly one `RecordWrite` per call carries the same `outcome` enum as the slog line, a `reason` (the `ReadOnlyReason` or `none`), `size` bucket, `eol`, `bom` and the write duration; there is no path or content field on the event; a recorder error is logged at Warn and never alters the `WriteFile` response.
- Cap scope under ADR-036 (the caps in this story are the `writeFull` caps).
  - *Given* the 1.5 MiB body cap, the 1 MiB content cap and the truncated-read refusal above, *When* `ApplyFilePatch` exists (Story 2.1.5), *Then* these rules still govern `WriteFile`/`writeFull` and the `full` adapter unchanged; `ApplyFilePatch` has its own limits (2 MiB body, 2,000 edits, 1 MiB inserted text) and a T1 file is edited by delta only, never by `WriteFile` (a `WriteFile` for a file over 1 MiB is still refused `TooLarge`).
**Files**: `proto/session/v1/session.proto`, `server/server.go`, `server/middleware/bodylimit.go`, `server/middleware/bodylimit_test.go`, `server/server_probeguard_test.go`, `server/services/file_writer.go`, `server/services/file_write.go`, `server/services/file_write_test.go`, `server/services/files_delegates.go`

##### Task 2.1.2a: Proto `WriteFile` (S)
- Request `{session_id, path, content (bytes, LF-normalized UTF-8), expected_token}` only. **No `line_ending` or `has_bom` request fields** (they were redundant, let the client claim `CRLF` for an `LF` file, and could carry `Mixed`): the server derives EOL and BOM from the on-disk file it just token-verified. Response `oneof { Saved{token}; Conflict{ oneof theirs { FullText{content, token}; Unavailable{reason} } } }`. Rejects are Connect errors with a typed `ReadOnlyReason` detail (no `Rejected` arm on the wire). Commit `.proto` only.
- Files: `proto/session/v1/session.proto`

##### Task 2.1.2b: `FileWriter` skeleton, ports and wiring (S)
- New `file_writer.go`: `FileWriter` with its own constructor and consumer-defined ports `FilesFlags` (global flag read), `WriteGuard` (hidden-session guard), `StatusInvalidator` (VCS cache), `WorkspaceProvider` (existing), `FilesTelemetry` (analytics outcome port from Task 0.2.5c; no-op default), plus the refcounted per-path mutex registry from Task 2.1.1b. **ADR-036: `FileWriter` is the write-side collaborator of `FileOps` (Task 0.2.6c): the flag, guard, workspace and telemetry ports and the `FileBackend` are the ones `FileOps` already holds in its `FilePolicy` (they are not redeclared here), `FileWriter` keeps only the write-specific ports (`StatusInvalidator`) and the mutex registry, and `WriteFile` is reached as `FileOps.WriteFull`; behavior and tests are unchanged.** Wired in `SessionService` construction; `FileService` stays read-only and its constructor is unchanged. Target for the Task 0.1.3b findings.
- Files: `server/services/file_writer.go`, `server/server.go`

##### Task 2.1.2c: Validation pipeline in `file_write.go` (M)
- (Pre-mortem #6 refinement, applies to the compare and the pre-rename re-check below: treat the sha256 as the identity. On any size or mtime difference, hash the on-disk bytes; if the hash equals the token's hash segment, accept and return the new token, otherwise `Conflict`. Size and mtime never accept on their own. The pre-rename re-check re-hashes the same way.)
- Ordered small functions: flag, hidden-session guard, `resolveFileRequest(ctx, FileRef, Write)` (symlink-safe `.git` deny, see Task 0.2.1a), `Lstat` not-symlink/not-dir (a missing file is **not** an error here: it falls through to the token compare), parse token (unparseable -> `InvalidArgument`; hash `-` parses as `NonEditable` and is refused `FailedPrecondition` with the reason from the live `classifyReadOnly`, `Truncated` by default), reject `\r`/NUL in content, size cap, `validateEditableUTF8`, token compare under the mutex, **folding the existence check into it**: if the file is gone (`ErrNotExist`) the compare yields `Conflict{Unavailable{Deleted}}`, never a Connect `NotFound`/error, and the same applies if it vanishes between `Lstat` and the compare (**always compare sha256**; size or mtime mismatch may short-circuit only toward `Conflict`; size+mtime equality still hashes, about 1 ms at the 1 MB cap), then derive EOL and BOM from the verified on-disk bytes and apply them to the LF content, pre-rename re-check, `atomicReplace`, new token. On a token mismatch build `Conflict`: `FullText` (LF-normalized, BOM stripped) when the current file passes `classifyReadOnly` and is <= 1 MB, else `Unavailable{reason}` and never partial content. No function above funlen/gocognit limits.
- Files: `server/services/file_write.go`

##### Task 2.1.2d: Delegate, `guardedProcedures` registration, remote-access enforcement, logs, metrics, registry (S)
- Delegate from `files_delegates.go`; add `writeFileProcedurePath = "/api" + sessionv1connect.SessionServiceWriteFileProcedure` and register it as `middleware.ProfileRebinding` in `guardedProcedures` (`server/server.go`, beside `replyProcedurePath`; follow that constant's doc-comment pattern); implement whatever Task 0.1.3b recorded for remote-access tiers (this task enforces, it does not discover); `// +api: files:write`; `docs/registry/features/files-editing.json`.
- Files: `server/server.go`, `server/services/files_delegates.go`, `server/services/file_metrics.go`, `docs/registry/features/files-editing.json`

##### Task 2.1.2e: Handler tests (flag, conflict sum type, refusal table, paused vs directory-mode session, same-size pinned-mtime rewrite, touched-mtime-identical-bytes `Saved`, guard rejection of foreign-Origin POST / rebinding Host / GET over the real `guardedProcedures`) (M)
- Files: `server/services/file_write_test.go`, `server/server_probeguard_test.go`

##### Task 2.1.2f: `WriteFile`-scoped request body limit on both listeners (S; was XS before triad round 3; unconditional since triad round 2)
- VERIFIED that `server/` has no `connect.WithReadMaxBytes` (only a comment at `backlog_service_lifecycle.go:1054`), so Connect's default is unlimited and the 1 MB cap was enforced only after the body was in memory. Add `middleware.BodyLimit(path, 1.5 MiB)` (`http.MaxBytesReader`, precedent `server/handlers/telemetry_handler.go:79`) in the new `server/middleware/bodylimit.go`. **Where it is wired (triad round 3, VERIFIED)**: not beside the `guardedProcedures` entry (that map feeds only `localChainWith`, so `remoteChain` on `:8444` would stay unbounded) but **around the session Connect handler itself** at `server/server.go:~495-497`, i.e. `handler = middleware.BodyLimit("/session.v1.SessionService/WriteFile", 1.5<<20)(handler)` before the `StreamingWSBridge` and `RegisterConnectHandler(apiPath, http.StripPrefix("/api", handler))` lines, matching the path as the handler sees it (the `/api` prefix is stripped) so one wrapper covers `:8543` and `:8444`. **Error shape**: connect-go v1.20.0 converts the `*http.MaxBytesError` into `CodeResourceExhausted` itself before any handler or interceptor runs (`error.go:452-466`), so the refusal is `ResourceExhausted` without the typed detail; Task 2.2.1a maps `ResourceExhausted` on `WriteFile` to `Rejected(TooLarge)`. Tests (`bodylimit_test.go` for the wrapper alone; `file_write_test.go` through the real local and remote chains with a Connect client): over-limit refused on both chains, 1 MiB of content accepted, other procedures unaffected. One line in `server/server.go` (or in `server/guarded_procedures.go` if Task 0.2.4f ran and moved the handler wiring there). Does not change the repo-wide absence of read limits for other RPCs (recorded in ADR-031 as a separate gap). ADR-036: Task 2.1.5e registers a second path (`ApplyFilePatch`, 2 MiB) with the same wrapper on both listeners. Re-classed S: two chains, a verified error contract and a client mapping.
- Files: `server/middleware/bodylimit.go`, `server/middleware/bodylimit_test.go`, `server/server.go`, `server/services/file_write_test.go`

##### Task 2.1.2g: Remote-tier deny entry for `WriteFile` (S, conditional contingency from Task 0.1.3b)
- Runs only if the Task 0.1.3b recon records that a read-only or limited remote-access tier can reach a write RPC: add the per-RPC deny (or tier requirement) for `WriteFile` at the point 0.1.3b identified, a Go test per tier (the REQ-12 integration row), and an ADR-031 update. Skipped and recorded as skipped when the recon finds no such tier. Costed and counted here (it was previously an uncosted "possible extra task" in the wall-clock list) so the Wave 5 figure includes the contingency; it runs in Wave 5 before Story 2.1.3.
- Files: `server/server.go`, `server/services/file_writer.go`, `server/services/file_write_test.go`

#### Story 2.1.3: Post-save status invalidation
**As a** user, **I want** git-status badges to update after save, **so that** the tree is not stale for 15 s.
**Acceptance Criteria**:
- Cache invalidated.
  - *Given* `a.go` clean in `vcsStatusCache`, *When* `WriteFile` saves a change, *Then* the next `GetVcsStatus`-equivalent call reports `a.go` modified without waiting for the 15 s fingerprint poll.
- Frontend refresh.
  - *Given* `SaveOutcome=Saved`, *When* handled, *Then* `refreshStatus()` from `useSessionVcsContext()` is called once.
**Files**: `server/services/file_writer.go`, `server/services/workspace_service.go` (new `InvalidateVCSStatus`), `session/git_worktree_watcher.go` (only if the recon finds a watcher fingerprint to bump), `server/services/file_write_test.go`

##### Task 2.1.3a: Call or add the invalidation hook (per Task 0.1.3b) (S)
- Implemented behind the `StatusInvalidator` port of `FileWriter`; respect `instance-lock-free-reads.md` (use `Snapshot()`); no `git` subprocess in production code. VERIFIED (triad round 2): `vcsStatusCache` is a `sync.Map` field on `WorkspaceService` (`server/services/workspace_service.go:82`, read `:160`, written `:225`, with 15 s TTL `:60`) and its only eviction is `SwitchWorkspace` (`:447`); the port's adapter therefore calls a new `WorkspaceService.InvalidateVCSStatus(workDir string)` (`ws.vcsStatusCache.Delete(workDir)`, keyed the same way as `GetVCSStatus`), which this task adds. `session/git_worktree_watcher.go` is touched only if Task 0.1.3b finds a watcher-side fingerprint that must also be bumped.
- Files: `server/services/file_writer.go`, `server/services/workspace_service.go`, `session/git_worktree_watcher.go`, `server/services/file_write_test.go`

#### Story 2.1.4: Round-trip and security corpus
**As the** owner, **I want** a byte-exact round-trip test over nasty files, **so that** saving cannot corrupt content.
**Acceptance Criteria**:
- Byte-identical no-op round trip.
  - *Given* fixtures `crlf.txt`, `crlf_bom.txt`, `mixed_eol.txt`, `lone_cr.txt` (LF file with one bare `\r`), `bom.txt`, `no_trailing_newline.txt`, `exec.sh` (0755), *When* each is read, LF-normalized by the client function, then written back unchanged, *Then* `bytes.Equal(before, after)` (a CRLF+BOM file is byte-identical after a no-op save, with no `\r\r\n`) and mode is unchanged (`mixed_eol.txt` and `lone_cr.txt` are rejected `MixedEol` on both read and `WriteFile`, never rewritten).
- A zero-byte file round-trips (triad round 2, ux.md 8.9, U-EDIT-19).
  - *Given* `empty.txt` (0 bytes), *When* it is read, *Then* the response is editable (token hash = sha256 of zero bytes, not `-`; `LineEnding=LF` default, no BOM); *When* it is written back unchanged, *Then* it is still 0 bytes and the mode is unchanged; *When* `"a\nb"` is written, *Then* the file is `a\nb` with LF and no BOM; *When* a CRLF file is edited down to `""` and saved, *Then* the file is 0 bytes.
- Latin-1 stays read-only.
  - *Given* `latin1.txt` with byte `0xE9`, *When* `WriteFile` is attempted, *Then* it returns `NonUtf8` and the file is unchanged.
- Symlink-escape matrix.
  - *Given* `leak -> /tmp/outside`, `dir_leak/ -> /tmp/outside_dir`, `linkdir -> .git/hooks` and `gitlink -> .git` inside a directory-mode checkout, *When* writing `leak`, `dir_leak/x`, `linkdir/pre-commit` and `gitlink/config`, *Then* all are rejected (`PermissionDenied`) and `/tmp/outside*` and `.git/` are unchanged.
- Race.
  - *Given* 20 concurrent saves with the same token, *When* run with `-race`, *Then* exactly one `Saved` and 19 `Conflict`.
- The corpus also runs through the delta path (ADR-036; executed by Task 2.1.5f).
  - *Given* the fixtures above, *When* each is edited by `ApplyFilePatch` with an empty edit list or an identity edit, *Then* CRLF+BOM files are byte-identical, `mixed_eol.txt` and `lone_cr.txt` are refused `MixedEol`, `latin1.txt` is refused `NonUtf8`, and the symlink-escape matrix is rejected for patches exactly as for `WriteFile`; a 20-concurrent-patch race with the same base token yields exactly one `Saved` and 19 `Conflict`.
**Files**: `server/services/testdata/files_roundtrip/`, `server/services/file_write_corpus_test.go`

##### Task 2.1.4a: Fixtures and corpus tests (M)
- Files: `server/services/testdata/files_roundtrip/`, `server/services/file_write_corpus_test.go`

#### Story 2.1.5: `ApplyFilePatch` server: splice, layered verification, replay store and guards (ADR-036 Phase C; after Gate G1)
**As the** owner, **I want** a save to send only my edits and the server to refuse anything it cannot prove correct, **so that** a one-character save of a big file is small and a wrong offset can never silently mangle a file.
**Acceptance Criteria**:
- Equality with the full-replace path (the property that makes deltas safe).
  - *Given* the shared golden vectors `server/services/testdata/transport/*.json` (`{base bytes, eol, bom, edits, expected bytes}`: empty file, no trailing newline, CRLF, BOM, astral characters, combining marks, edits at 0 and EOF, adjacent edits, insert-only, delete-only, invalid column that splits a surrogate pair), *When* `ApplyFilePatch` applies each case in the T0 (in-memory) and T1 (streaming splice) paths, *Then* the resulting bytes equal the expected bytes and equal `WriteFile` of the same final LF text bytewise, a no-op patch of a CRLF+BOM file is byte-identical (ADR-031's no-op property), and the invalid-column vector is rejected `InvalidArgument` with nothing written.
  - *Given* `FuzzApplyPatch` (random base text over LF/CRLF/BOM, random ordered non-overlapping edits including astral and combining text), *Then* `applyPatch == writeFull(applyChanges(base))` for every input in both paths, no panic, no partial write and the temp file is cleaned up on every failure.
- Verification layers run before rename; a failure leaves the original untouched (ADR-036 decision 11).
  - *Given* a patch whose `deleted_crc32c` is wrong for edit 2, *Then* `OffsetMismatch` and the file is unchanged; *Given* `expect_new_size` off by one, *Then* `ResultMismatch`; *Given* a wrong `region_crc32c`, *Then* `ResultMismatch`; *Given* T0 with a wrong `expect_result_sha256`, *Then* `ResultMismatch`; *Given* a base token whose sha256 no longer matches (an agent rewrote the file), *Then* the existing `Conflict` outcome (T0 `FullText`, T1 `Unavailable{TooLarge}`), never a partial apply; *Given* a T1 patch while `analysis` is pending, *Then* `AnalysisPending` (no write).
  - *Given* a successful patch, *Then* the response carries `new_token`, `new_size`, `new_line_count`, `applied_edits` and `verified{size, regions, result_sha}` so the UI and logs show exactly what was checked.
- Streaming splice (T1) is O(1) memory and as atomic as ADR-031.
  - *Given* a 64 MiB file and a 1-character edit in the middle, *When* the splice runs, *Then* peak `HeapAlloc` growth stays under 16 MB (pooled buffers), old and new sha256 come from the same pass, a disk-space preflight (free space at least file size plus slack, else `ResourceExhausted`) runs first, a process-wide semaphore of 1 serializes large rewrites, the pre-rename re-check follows ADR-031 (a content change aborts with `Conflict`, an mtime-only change re-hashes equal and proceeds), mode is preserved and no `.x.ssq-tmp-*` remains; fault injection (kill between temp write and rename, ENOSPC, rename failure, the agent writing during the pass) leaves the original file intact.
- Idempotency and ordering (decision 12).
  - *Given* the same `(client_id, seq)` and payload digest twice, *Then* the second returns the recorded outcome without reapplying; *Given* the same key with a different digest, *Then* `InvalidArgument`; *Given* `seq != last_acked + 1`, *Then* `OutOfOrder`; the store keeps 256 entries / 10 minutes per `(session, path)` and a restart empties it (the lost-response rule of Story 2.2.3 then applies).
- Limits (decision 15).
  - *Given* a 2 MiB + 1 byte body through the real `localChain` and `remoteChain`, *Then* both refuse with `ResourceExhausted` before the handler (the same connect-go behavior as Task 2.1.2f; the client maps it to `PatchTooLarge`); *Given* 2,001 edits or 1 MiB + 1 byte of inserted text, *Then* `PatchTooLarge` with a typed detail and nothing written; *Given* a third patch to one path within one second, *Then* `ResourceExhausted` (2 per second per path); `writeFull` keeps its 1.5 MiB cap.
- Security and policy (decision 16).
  - *Given* `ApplyFilePatch` and `PreflightFilePatch` registered in `guardedProcedures` with `ProfileRebinding`, *When* the Story 2.1.2 guard tests run over the real map, *Then* a foreign-Origin POST, a rebinding Host and a GET are rejected before the handler and a map-membership test fails if either is removed; *Given* `files:editing=false`, a hidden or archived session, a symlink leaf, `.git/hooks/x`, a paused worktree (`ActiveDir != ExistingDir`), *Then* each is refused exactly as `WriteFile` refuses it (single policy in `FileOps`); *Given* `files:delta-write=false`, *Then* `FailedPrecondition FlagOff` and the client routes saves to `writeFull`; *Given* a T1 file and `files:edit-large=false`, *Then* `FailedPrecondition` with reason `TooLarge`.
  - *Given* the Task 0.1.3b finding on remote-tier write reachability, *When* `ApplyFilePatch` is called on `:8444`, *Then* the outcome equals what ADR-031/ADR-036 "Spike outcomes" recorded: if a read-scoped remote caller can reach write RPCs, the RPC is denied for that scope (Task 2.1.5e) and the capabilities omit `PATCH` for it; if not, `PATCH` is advertised on `:8444` for write-scoped callers. Either answer passes the same conformance cases.
- Observability.
  - *Given* any patch, *Then* exactly one slog line (session, path, edits, bytes inserted/deleted, size before/after, outcome, adapter, duration; no content) and exactly one `files.server.patch` outcome event (enum/bucket labels only) are emitted, and a telemetry error never alters the response.
**Files**: `proto/session/v1/session.proto`, `server/services/file_patch.go`, `server/services/file_splice.go`, `server/services/patch_replay_store.go`, `server/services/file_ops_patch.go`, `server/server.go`

##### Task 2.1.5a: `ApplyFilePatch` / `PreflightFilePatch` proto and the typed refusal reasons (S)
- `Position{line, col_utf16}`, `Edit{start, end, insert, deleted_crc32c}`, `ApplyFilePatchRequest{ref, base_token, client_id, seq, edits[], expect_new_size, expect_result_sha256, region_crc32c[], caps_epoch}`, response `oneof {Saved{new_token,new_size,new_line_count,applied_edits,verified}; Conflict{theirs}}` (same sum type as `WriteFile`), `PreflightFilePatchRequest/Response`, and a sibling enum `PatchRejectReason {OffsetMismatch, ResultMismatch, OutOfOrder, AnalysisPending, PatchTooLarge}` carried as a typed Connect error detail (`ReadOnlyReason` stays the one read-only enum); the code mapping is in the Story 2.1.5 ACs (state-dependent refusals `FailedPrecondition`, malformed input `InvalidArgument`, limits `ResourceExhausted`). `make proto-gen`; commit only the `.proto`. Ordered after Task 1.6.2a.
- Files: `proto/session/v1/session.proto`

##### Task 2.1.5b: Patch validation, line/col to byte conversion and the T0 in-memory splice (M)
- `file_patch.go`: ordered/non-overlapping validation and caps, base-coordinate single forward pass, EOL and BOM owned by the server (LF-normalized inserts re-expanded for CRLF), verification layers (a)-(e) as small functions; `file_patch_position.go`: line/col_utf16 to bytes through the line index plus decoding only the touched line, surrogate-split rejection, columns past EOL rejected; table tests over the golden vectors.
- Files: `server/services/file_patch.go`, `server/services/file_patch_position.go`, `server/services/file_patch_test.go`, `server/services/file_patch_position_test.go`

##### Task 2.1.5c: Streaming splice for T1 with preflight, semaphore and re-check (M)
- `FileBackend.Splice` on `osRootBackend` (and the in-memory backend with fault injection): read old, write prefix/inserts/suffix to the same-directory temp via the Story 2.1.1 helpers, hash old and new in one pass, verify before rename, fsync file and directory, disk-space preflight, process-wide semaphore of 1, `analysis=ready` required, `files:edit-large` gate; reuse `atomicReplace` internals without growing its signature.
- Files: `server/services/file_splice.go`, `server/services/file_splice_test.go`, `server/services/file_backend_osroot.go`, `server/services/file_atomic.go`

##### Task 2.1.5d: Replay store, ordering and per-path rate limit (S)
- Bounded `(session, path)` store keyed `(client_id, seq)` with payload digest (256 entries, 10 minutes, injected clock), `seq == last_acked + 1`, replay returns the recorded outcome, 2 per second per path limiter; `-race` and fake-clock tests.
- Files: `server/services/patch_replay_store.go`, `server/services/patch_replay_store_test.go`

##### Task 2.1.5e: `FileOps.ApplyPatch`, delegate, `guardedProcedures` on both chains, body limit, remote-tier handling and telemetry (M)
- `file_ops_patch.go` orchestrates policy (flags, hidden/archived session guard, confinement, per-path mutex, replay store, T0/T1 routing), the delegate in `files_delegates.go`, `ApplyFilePatch` and `PreflightFilePatch` added to `guardedProcedures` with `middleware.ProfileRebinding` and a 2 MiB `middleware.BodyLimit` around the session handler on both listeners (the Task 2.1.2f wrapper, one more path; in `server/guarded_procedures.go` if Task 0.2.4f ran), the remote-tier decision from Task 0.1.3b implemented here (deny entry for read-scoped callers if the recon says they reach writes, and `PATCH` omitted from `:8444` capabilities accordingly), slog line, `RecordPatch`, `// +api: files:patch`, registry JSON. This task edits `server/server.go` after Task 2.1.2g and before Task 2.5.3a.
- Files: `server/services/file_ops_patch.go`, `server/services/files_delegates.go`, `server/server.go`, `server/services/file_ops_capabilities.go`, `docs/registry/features/files-patch.json`

##### Task 2.1.5f: Patch conformance, golden vectors, `FuzzApplyPatch`, fault injection and guard tests (M)
- Adds the patch cases to `FileOpsConformance` (capability honesty, idempotent replay, out-of-order refusal, body-limit refusals on both chains, confinement corpus on patches, fallback of `files:delta-write` off), the golden vectors under `testdata/transport/` (schema fixed by Task 0.1.8a, consumed by Task 2.2.5c), `FuzzApplyPatch`, the fault-injection list of research/transport.md section 7, and the real-`guardedProcedures` tests for both RPCs (in `file_patch_guard_test.go`, modelled on the Story 2.1.2 guard tests), plus the delta-path run of the Story 2.1.4 corpus (`file_write_corpus_test.go`).
- Files: `server/services/file_ops_conformance_test.go`, `server/services/file_patch_fuzz_test.go`, `server/services/testdata/transport/`, `server/services/file_patch_guard_test.go`, `server/services/file_write_corpus_test.go`

## Epic 2.2: Editing frontend
**Goal**: Read-only by default, explicit Edit, never lose a draft, never save blindly.

#### Story 2.2.1: Edit mode and save
**As a** user, **I want** to edit and save with Ctrl+S, **so that** I can fix a line without a terminal.
**Acceptance Criteria**:
- Edit toggle gated.
  - *Given* `files:editing=false`, *When* a file opens, *Then* no Edit button exists; with `true` and `ReadOnlyReason=None` the Edit button is visible; with any non-`None` reason (Truncated, TooLarge, Binary, NonUtf8, MixedEol, Symlink, WorktreeMissing, HiddenSession, GitInternal) it is rendered `aria-disabled` and never absent, with the reason as visible text ("Read-only: Truncated") on desktop and, on mobile, a disabled FAB whose tap opens a popover with the reason and the available action (Download); table-driven over the enum (U-EDIT-18). Edit mode is per buffer (Story 1.2.2).
- Entering Edit pins the tab; Edit hands off from the Accessible view (triad round 3, ux.md 8.1 / 8.10, U-EDIT-9, U-EDIT-17).
  - *Given* a clean preview tab `a.go` and the Edit control activated, *Then* the tab is pinned before any keystroke and a following single-click on `b.go` opens a new tab instead of replacing it.
  - *Given* the Accessible view on an editable buffer with `onEditRequested` now wired, *When* the 44 px Edit button is activated with line 1,034 last focused, *Then* the CodeMirror view mounts in Edit mode at that line, "Editing b.go, line 1034" is announced, the buffer, dirty state and tab mode are shared, no second copy of the document is made, and the Accessible view is off for that tab until toggled back (the stub assertion of Story 1.5.4 is replaced by this real one).
- Zero-byte file (ux.md 8.9, U-EDIT-19).
  - *Given* an empty file, *When* it opens, *Then* it shows the "Empty file" placeholder (not part of the document), status `0 B` and "LF (none detected)", Edit is enabled, typing makes it dirty, and Save sends LF-normalized text (including `""`); the server corpus case is Story 2.1.4.
- Save success.
  - *Given* a `Loaded` `Buffer` `a.go` in Edit with `dirty = true` (doc text `"x"`), *When* Ctrl+S is pressed, *Then* `WriteFile` is called with the LF-normalized doc text and `baseToken`, on `Saved(T2)` `dirty=false`, `baseToken=T2`, `mergeBase` becomes the saved text, undo history is preserved, a "Saved" status message is announced, and `refreshStatus()` runs.
- EOL/BOM preserved with a single owner (the server).
  - *Given* a `CRLF` file with BOM, *When* it is loaded, *Then* the client normalizes the text to LF once (`normalizeToLf`) and the status bar shows `CRLF` informationally; *When* saved, *Then* the request carries LF-normalized `content` only (no `line_ending`/`has_bom` fields) and the editor's `lineSeparator` is left at its default (never `\r\n`), so the doc can never contain `\r`. A no-op save of a CRLF+BOM file is byte-identical on disk (asserted in Story 2.1.4).
- Save in flight.
  - *Given* a pending save, *When* Ctrl+S is pressed again, *Then* no second request is sent and Save shows a spinner.
- Flag flipped mid-session.
  - *Given* a dirty buffer and the global `files:editing` turned off, *When* flags refresh (window focus/visibility via `FeatureFlagsContext.refresh()`, or immediately after a `FlagOff` refusal of a save, Story 0.2.2), *Then* the editor becomes read-only, the draft is kept, and a banner explains.
- A save can only target the session its buffer was opened in (pre-mortem #2).
  - *Given* a dirty `Loaded` buffer opened with `sessionId="s1"` and a `FilesTab` whose `sessionId` prop is then changed to `s2` (simulated in a jest test by re-rendering without the production `key`), *When* Ctrl+S is pressed, *Then* the `WriteFile` request carries `session_id="s1"` (the Buffer's) or is blocked with "Session changed - reopen the file"; it is never sent with `s2`. The same holds for `StatFile` re-checks and conflict-resolution re-sends.
  - *Given* the status bar in Edit mode, *Then* it shows the session name and branch (always on mobile, where the tab strip is hidden) so the owner can see which checkout is being edited.
- Analytics (ADR-035).
  - *Given* Edit entered via the FAB, a Ctrl+S that returns `Saved`, a save that returns `Conflict`, and a refusal with `ReadOnlyReason=Symlink`, *When* each resolves, *Then* `files.edit.enter` (`via`, `ext_class`), `files.edit.save` (`outcome`, `via`, `size`) with `files.perf.save_ms` (`outcome`, `size`; `durationMs` = request round trip), and for the refusal `files.edit.save_rejected` (`reason`) are emitted; leaving Edit emits `files.edit.exit` (`via`, `dirty`); no path, file name or content is present; the offline path emits `files.edit.save{outcome=offline_blocked}`.
**Files**: `web-app/src/components/sessions/CodeEditor.tsx`, `web-app/src/lib/files/bufferReducer.ts`, `web-app/src/lib/hooks/useSaveBuffer.ts`, `web-app/src/lib/hooks/__tests__/useSaveBuffer.test.ts`, `web-app/src/components/sessions/files/StatusBar.tsx`

##### Task 2.2.1a: `useSaveBuffer` with `SaveOutcome` handling (M)
- Exhaustive switch over `Saved/Conflict/Rejected/Unknown`; `Unknown` forces a `StatFile` re-check before next save. The hook takes the `Buffer` and uses `buffer.sessionId` for `WriteFile`/`StatFile`; it never reads a session id from props or context. Jest test `useSaveBuffer_should_UseBufferSessionId_When_SessionPropChangesBeforeSave` (dirty buffer, change the prop, Ctrl+S, assert the request's `session_id` is the original or that the save is blocked). A `FlagOff` refusal calls `FeatureFlagsContext.refresh()`; a Connect `ResourceExhausted` on `WriteFile` (the body-limit refusal of Task 2.1.2f, which carries no `ReadOnlyReason` detail) maps to `Rejected(TooLarge)`, with a jest case. ADR-036: the hook calls `FileTransport.writeFull` (the `full` binding of Task 1.2.3b) and leaves the patch route to Task 2.2.5b; the single save path, `SaveOutcome` and the one-request-in-flight rule do not change.
- Files: `web-app/src/lib/hooks/useSaveBuffer.ts`, `web-app/src/lib/hooks/__tests__/useSaveBuffer.test.ts`

##### Task 2.2.1b: Edit mode in `CodeEditor` + reducer `dirty` flag + Edit button (M)
- Compartment reconfigure only; doc text is LF-normalized on load and sent LF-normalized (`EditorState.lineSeparator` is NOT set from the detected EOL; the server re-applies EOL/BOM from the token-verified disk file); `indentWithTab` not unconditional (Esc then Tab leaves the editor); Save command; entering Edit dispatches `setMode(Edit)` (pins a preview tab, triad round 3) and the first edit dispatches `markDirty`. Also wires the **Edit hand-off from the Accessible view** (moved here from Task 1.5.4b, which is built in Wave 6 before any Edit mode exists): activating Edit in `AccessibleView` calls the `onEditRequested` seam that Task 1.5.4a left as a stub, mounts CodeMirror in Edit at the line last focused and announces "Editing b.go, line N".
- Files: `web-app/src/components/sessions/CodeEditor.tsx`, `web-app/src/lib/files/bufferReducer.ts`, `web-app/src/components/sessions/files/AccessibleView.tsx`, `web-app/src/components/sessions/__tests__/AccessibleView.test.tsx`

##### Task 2.2.1c: Status bar dirty/save states, read-only banners and directory-mode chip (S)
- Chip "Editing the main checkout" (ux.md copy; shown when `WorktreeDir == ""` in Edit mode); asserted in `StatusBar.test.tsx`. Status bar also shows session name and branch in Edit mode (asserted in the same test).
- Files: `web-app/src/components/sessions/files/StatusBar.tsx`, `web-app/src/components/sessions/FilesTab.tsx`

#### Story 2.2.2: Unsaved guards and drafts
**As a** user, **I want** a prompt before losing work and a way to restore it, **so that** a closed tab or paused session does not lose edits.
**Acceptance Criteria**:
- Close prompt.
  - *Given* dirty `a.go`, *When* its tab close is clicked, *Then* a dialog offers Cancel / Discard / Save with **Cancel as the initial focus and Esc/back target** (Enter on it changes nothing; Save is primary-styled but never the default, because Enter on a default Save would write to disk from a dialog opened to close a tab; ux.md 11, U-TAB-7), focus is trapped and restored; the Restore-draft dialog's initial focus is **Not now**.
- beforeunload.
  - *Given* any dirty buffer, *When* the page unloads, *Then* `beforeunload` is requested.
- Bounded draft.
  - *Given* a dirty `a.go` (40 KB) with `baseToken T1` and the session paused externally, *When* the buffer is reopened later, *Then* a "Restore draft?" prompt offers it; drafts over 256 KB or older than 24 h are not stored/offered (the over-cap case is covered by the in-memory hold and the "Not backed up" chip below).
- Close others with several dirty tabs (triad round 3, ux.md 11.1a, U-TAB-11).
  - *Given* `a.go`, `b.go` and `c.go` dirty and the sheet's or strip's Close others, *When* it is activated, *Then* one Discard dialog lists the three (default focus Cancel) with Cancel, Discard all and Save all; *When* Save all is chosen and `b.go` returns `Conflict`, *Then* saves ran sequentially in tab order (never in parallel), `a.go` saved and closed, `b.go` is activated with its conflict dialog open, `c.go` was not saved and stays open and dirty, and a toast "Saved 1 of 3; b.go needs attention" (>= 8 s) shows; *When* Stop is pressed during `a.go`'s save, *Then* no further save starts after it returns and nothing is discarded; offline disables Save all with the reason.
- Draft vs unchanged file keeps Merge possible.
  - *Given* a stored draft for `T1` and the file still at `T1`, *When* restored, *Then* the draft opens in an Edit buffer with `mergeBase = Available(disk text at restore time)` (disk equals the original base because the tokens match), so a later conflict can still offer Merge.
- Dirty buffers above the localStorage cap are backed up in IndexedDB, and an unprotected dirty buffer is an explicit high-priority warning (pre-mortem #8; rebuilt in triad round 3 because iOS discards a backgrounded PWA and an in-memory hold alone loses everything then; ux.md 11.3, 8.2, U-DRAFT-3, U-DRAFT-6, ADR-032).
  - *Given* a dirty buffer of 600 KB (above the 256 KB localStorage tier, below the 1 MB edit cap) and a fake `DraftBackend` over IndexedDB, *When* the edit lands, *Then* the status chip reads "Backing up..." until the debounced (2 s) write resolves, then clears; a write is also started on `visibilitychange` (to hidden) and `pagehide`; the entry is keyed (session, path, baseToken), expires after 24 h, and the store keeps at most 5 entries (the oldest is dropped with a toast naming the file); *When* the buffer is reopened within 24 h, *Then* the same Restore prompt (default Not now) offers it.
  - *Given* IndexedDB unavailable, blocked or over quota (the fake rejects), or the owner's veto of draft persistence, *When* a buffer is dirty, *Then* the priority-1 banner "Draft not stored: <reason>. Your edits exist only in this tab." (`role="alert"`, never collapsed, no Dismiss) shows with **Save** (emphasised), **Copy my version** and **Retry backup**, and it clears when a backup write succeeds or the buffer is saved; `files.edit.draft_not_stored{reason}` is emitted once per buffer per session with `reason` in {too_large, storage_full, backup_unavailable}.
  - *Given* a 1 MB dirty buffer, *When* the localStorage quota is also nearly full, *Then* the layout/tab keys still write (the large draft never goes to localStorage) and typing does not block (the write is asynchronous).
  - *Given* a dirty buffer of any size, *When* `visibilitychange` (to hidden) or `pagehide` fires, *Then* the draft is flushed synchronously (not only on the debounce) to `draftStore` when under the cap, so the last seconds of typing survive an iOS tab eviction.
- Session switch does not lose or misdirect edits (pre-mortem #2).
  - *Given* dirty buffers in session `s1` and the parent switches the displayed session to `s2` (`FilesTab` remounts via `key={session.id}`), *When* the old `FilesTab` unmounts, *Then* no blocking dialog is shown (ux.md 11.1 was corrected to match this story; the parent has no veto point and automation must not be blocked), a toast "Unsaved changes in b.go kept - reopen this session to restore" (at least 8 s, dismissible) shows in the new view (U-DRAFT-5), and each dirty buffer's text is synchronously stored in `SessionBufferHold` keyed `(s1, path)` (any size up to the 1 MB cap, max 5 entries, evicting the oldest with a toast) and, when under the cap, also in `draftStore`; *When* the user returns to `s1`, *Then* the "Restore draft?" prompt offers the held text first, and restoring opens it with `baseToken` and the original `sessionId`; nothing from `s1` is ever written to `s2`.
- Draft vs changed file is a 2-way flow.
  - *Given* a stored draft for `T1` and the file now at `T2`, *When* restoring, *Then* the draft opens with `mergeBase = Unavailable` and the conflict dialog shows draft-vs-disk with Keep editing / Reload / Overwrite / Copy my version; Merge is disabled with the reason "original version not available after reload". No base text is stored in localStorage (it would double the secrets exposure the owner may veto).
- Analytics (ADR-035).
  - *Given* a restore prompt resolved by Restore (token equal), Restore (token differs), Discard draft, Not now, and a draft refused as too large, *When* each resolves, *Then* `files.edit.draft_restore` (`outcome` {restored, restored_conflict, discarded, not_now, missing_file}, `age` bucket) or `files.edit.draft_not_stored` (`reason`) is emitted, never draft text.
- Drafts become ChangeSets (ADR-036, ADR-032 amended; built by Task 2.2.5d, not here).
  - *Given* this story's snapshot drafts and tiers, *When* Story 2.2.5 lands, *Then* the draft payload is the composed pending ChangeSet plus selection (the 256 KB localStorage cap and the 256 KB to 1 MB IndexedDB tier then bound edit volume, not file size), snapshots remain for an engine without a ChangeSet, and the keys, 24 h TTL, 5-entry limit, Restore prompt and the priority-1 banner of this story are unchanged.
**Files**: `web-app/src/lib/files/draftStore.ts`, `web-app/src/lib/files/draftBackends.ts`, `web-app/src/lib/files/sessionBufferHold.ts`, `web-app/src/lib/hooks/useDraftGuard.ts`, `web-app/src/lib/hooks/useCloseOthers.ts`, `web-app/src/components/sessions/files/DiscardDialog.tsx`, `web-app/src/lib/files/__tests__/draftStore.test.ts`

##### Task 2.2.2a: `draftStore` (try/catch, size/TTL caps, key `(session,path,baseToken)`) (S)
- Draft text is read from the `EditorState` (debounced), not from reducer state; no base text is persisted. Adds an immediate `flush()` called from `visibilitychange`/`pagehide`. Introduces the `DraftBackend` interface (`put/get/delete/list`, async-capable) with the localStorage backend (<= 256 KB, synchronous flush); the IndexedDB backend is Task 2.2.2d.
- Files: `web-app/src/lib/files/draftStore.ts`, `web-app/src/lib/files/__tests__/draftStore.test.ts`

##### Task 2.2.2b: `DiscardDialog` (incl. Close others shell), `beforeunload`, restore prompt, "Backing up..." chip and the "Draft not stored" banner (M)
- Includes the "Backing up..." chip, the priority-1 "Draft not stored" banner with Save / Copy my version / Retry backup (registered into the Story 1.3.7 banner stack at priority 1) and Save emphasis (test: a >256 KB dirty buffer with a failing backend shows the banner; with a working backend shows "Backing up..." then clears), and the Close others dialog shell (Cancel / Discard all / Save all buttons; the Save all sequence is Task 2.2.2e). Consumes the backend result of Task 2.2.2d.
- Files: `web-app/src/components/sessions/files/DiscardDialog.tsx`, `web-app/src/components/sessions/FilesTab.tsx`, `web-app/src/components/sessions/files/StatusBar.tsx`

##### Task 2.2.2d: IndexedDB draft backend for 256 KB to 1 MB dirty buffers (S; triad round 3)
- `IndexedDbDraftBackend` implementing `DraftBackend` (database `ssq-files-drafts`, one object store keyed (session, path, baseToken), 24 h TTL sweep on open, max 5 entries, oldest dropped), selection by size in `draftStore` (<= 256 KB localStorage, else IndexedDB), a 2 s debounce plus a started-on-`pagehide` write, every call wrapped (open failure, blocked upgrade, quota, private mode => resolves `unavailable`, never throws into UI). Tests use an in-memory `DraftBackend` double and a small hand-written `indexedDB` stub in the shared fixtures (no new `fake-indexeddb` dev dependency, so `package.json` stays out of this wave's collisions; adding that dependency instead would be an explicit change to the Wave 7/8 `package.json` order). Runs after Task 2.2.2a and before Task 2.2.2b.
- Files: `web-app/src/lib/files/draftBackends.ts`, `web-app/src/lib/files/draftStore.ts`, `web-app/src/lib/files/__tests__/draftStore.test.ts`

##### Task 2.2.2e: Close others "Save all" sequence (S; triad round 3)
- `useCloseOthers`: a sequential runner over the dirty tabs (injected `save(buffer)` returning `SaveOutcome`, injected `activate(tab)`, a `stop()` flag checked between saves), closing each tab that saved, pausing at the first `Conflict`/`Rejected`/`Unknown` by activating that tab, emitting `files.edit.save{via=save_all}` per save and one `files.tab.close{method=close_others}`; each save uses `buffer.sessionId`; the tab being saved is locked against other close triggers. Wired to the dialog shell of Task 2.2.2b. Runs after Task 2.2.2b (both edit `DiscardDialog.tsx`).
- Files: `web-app/src/lib/hooks/useCloseOthers.ts`, `web-app/src/components/sessions/files/DiscardDialog.tsx`, `web-app/src/lib/hooks/__tests__/useCloseOthers.test.ts`

##### Task 2.2.2c: `SessionBufferHold` and unmount flush on session switch (S)
- In-memory module-level map (tab lifetime, never persisted), written synchronously in the `FilesTab` unmount cleanup via a `useDraftGuard` hook (the seam from Task 1.2.2d); the restore prompt reads it before `draftStore`. Jest test: dirty buffer in `s1`, rerender the parent with `s2` (key change), assert nothing is sent for `s1`'s path with `s2`, return to `s1`, assert the held text is offered and restores with `sessionId=s1`.
- Files: `web-app/src/lib/files/sessionBufferHold.ts`, `web-app/src/lib/hooks/useDraftGuard.ts`, `web-app/src/lib/files/__tests__/sessionBufferHold.test.ts`

#### Story 2.2.3: Offline and unknown-outcome saves
**As a** phone user, **I want** saves to refuse safely when offline, **so that** a queued write never lands on a changed file.
**Acceptance Criteria**:
- Offline and unreachable are detected beyond `navigator.onLine` (triad round 2, ux.md 8.11, U-EDIT-20).
  - *Given* `navigator.onLine=false`, *Then* `Reachability=Offline`, Save is disabled and the banner reads "You are offline - edits kept locally" with Retry now; *Given* `navigator.onLine=true` but two consecutive Files-tab RPC network failures (fetch `TypeError`, Connect `unavailable`/`deadline_exceeded`) or two consecutive failed 10 s heartbeat `StatFile` calls (the poll runs for dirty buffers too), *Then* `Reachability=Unreachable`, Save is disabled and the banner reads "Server unreachable - edits kept locally"; *Given* one failed explicit Save or explicit `StatFile`, *Then* the state is also `Unreachable`; nothing is queued in any case; the next successful RPC or an `online` event clears it, and **Retry now** runs `StatFile` immediately. Fake-timer tests cover: captive-portal style (onLine true, fetch rejects), flapping (one failure does not flip), recovery, and the poll back-off (30 s after 3 failures, 10 s after a success).
  - *Given* any disabled-Save state, *Then* the reason string is visible (banner or status chip), is the button's `aria-describedby`, and tapping or pressing Enter on the disabled Save announces it politely (nothing silent; U-EDIT-20).
- Reconnect re-check.
  - *Given* the connection returns, *When* the next save is attempted, *Then* `StatFile` runs first and a changed token routes to the conflict dialog.
- Lost response.
  - *Given* a save request whose response never arrived (`SaveOutcome=Unknown`), *When* the user saves again, *Then* the client first re-stats and compares to detect whether the earlier write landed.
- Lost response with deltas (ADR-036 decision 12; exercised by Task 2.2.5b).
  - *Given* a patch whose response never arrived, *When* the user saves again, *Then* the client re-stats first; in T0 "landed" means the disk sha256 equals the `expect_result_sha256` it sent, in T1 (where it knows only `expect_new_size`) an unknown outcome is resolved by Reload with the ChangeSet kept as a draft, and a retry with the same `(client_id, seq)` and payload digest returns the recorded outcome if the server still holds it.
**Files**: `web-app/src/lib/hooks/useSaveBuffer.ts`, `web-app/src/lib/hooks/useOnlineStatus.ts`, `web-app/src/lib/hooks/useReachability.ts`, `web-app/src/lib/hooks/__tests__/useSaveBuffer.test.ts`, `web-app/src/lib/hooks/__tests__/useReachability.test.ts`

##### Task 2.2.3a: `useOnlineStatus` + `useReachability` (RPC-failure and heartbeat signals) + offline/unknown handling (M; was S before triad round 2)
- Reachability is a pure reducer over `{onLine, rpcFailures, heartbeatFailures}` events fed by a transport interceptor (network-class errors only) and `useBufferFreshness`; the disabled-Save reason wiring lives with the top bar (Task 2.2.4a).
- Files: `web-app/src/lib/hooks/useOnlineStatus.ts`, `web-app/src/lib/hooks/useReachability.ts`, `web-app/src/lib/hooks/useSaveBuffer.ts`


#### Story 2.2.4: Save affordances and read-mode editing feedback (triad repair)
**As a** phone or desktop user, **I want** Save always reachable while my buffer is dirty and clear feedback when I type into a read-only view, **so that** I never lose a save behind a dismissed keyboard or wonder why typing does nothing.
**Acceptance Criteria**:
- Mobile Save is reachable with the keyboard dismissed (ux.md section 7.1, U-MOB-11).
  - *Given* viewport 390x844, a dirty `Loaded` buffer and the soft keyboard dismissed (stubbed full-height `visualViewport`), *When* the top bar renders, *Then* a Save button >= 44px with accessible name "Save, unsaved changes" is visible, tapping it calls the single `useSaveBuffer.save()` once (no second request while pending, spinner shown), and the overflow menu also lists Save; *Given* a clean buffer in Read mode, *Then* Save is hidden; in Edit mode it is shown disabled with "Save, no changes".
  - *Given* Edit mode on a phone, *When* the top bar renders, *Then* **Stop editing** is a 44 px icon button immediately right of Save with the accessible name "Stop editing" ("Stop editing (unsaved)" when dirty), the two never reorder, and activating it returns that buffer to Read keeping the draft (ux.md 7.1).
  - *Given* 320px width with a long file name, *When* the top bar crowds, *Then* `[N]` moves into the overflow menu first and Save and Stop editing never do (fixed targets back 44 + `[N]` 44 + Save 44 + Stop 44 + overflow 44 = 220px plus a >= 72px file name fit in 320px; the file name gets 100px before any move).
  - *Given* a disabled Save (clean, saving, offline, unreachable, read-only, paused, archived), *Then* its reason is visible and exposed through `aria-describedby`, and tapping it announces the reason politely (ux.md 7.1, U-EDIT-20).
- A dirty state stays visible and savable in Read mode.
  - *Given* a dirty buffer and the user leaves Edit, *When* Read mode renders, *Then* the dirty dot and the Save affordance remain, and a polite message "Stopped editing; N unsaved changes kept. Save to write them." is announced.
- Leaving Edit is not called "Done" (ux.md section 8.1, U-EDIT-14).
  - *Given* Edit mode, *When* the toolbar renders, *Then* the control reads "Stop editing" ("Stop editing (unsaved)" when dirty); no UI string says "Done" for this action.
- Typing in Read mode gives feedback (ux.md section 8.1, U-EDIT-13).
  - *Given* `files:editing` on, an editable file, desktop, Read mode, vim off, *When* a printable key, Enter or Backspace is pressed in the editor, *Then* the inline hint "Read-only. Press E or click Edit to start editing" shows (`role="status"`, no focus move, at most once per 10 s, auto-hide 4 s) and `E` enters Edit mode without inserting the triggering character; *Given* the file is not editable or the flag is off, *Then* the hint shows the reason chip text; *Given* vim on, *Then* vim keys behave as vim and the hint is suppressed.
- Analytics (ADR-035).
  - *Given* the hint is shown and `E` pressed, then another hint dismissed, *Then* `files.edit.read_hint{outcome}` is emitted as `shown` / `entered_edit` / `dismissed` and `files.edit.enter{via=read_hint}` follows; a top-bar Save emits `files.edit.save{via=top_bar}`.
**Files**: `web-app/src/components/sessions/files/MobileFilesLayout.tsx`, `web-app/src/components/sessions/files/StatusBar.tsx`, `web-app/src/components/sessions/files/extensions/readModeHint.ts`, `web-app/src/components/sessions/__tests__/MobileFilesLayout.test.tsx`, `web-app/src/components/sessions/__tests__/ReadModeHint.test.tsx`

##### Task 2.2.4a: Top-bar and overflow Save, persistent dirty affordance in Read mode, "Stop editing" label (S)
- One save path; spinner and `aria-disabled` states; 320px priority rule (`[N]` yields to Save).
- Files: `web-app/src/components/sessions/files/MobileFilesLayout.tsx`, `web-app/src/components/sessions/files/StatusBar.tsx`

##### Task 2.2.4b: Read-mode typing hint extension and `E` to enter Edit (S)
- CodeMirror extension in `extensions/readModeHint.ts` (input handler on the read-only state, rate-limited), registered by one line in `CodeEditor.tsx`.
- Files: `web-app/src/components/sessions/files/extensions/readModeHint.ts`, `web-app/src/components/sessions/CodeEditor.tsx`

#### Story 2.2.5: Delta encoder, stop-and-wait sync loop, ChangeSet drafts and delta UX states (ADR-036 Phase C)
**As the** owner on a phone link, **I want** Save to send only what I changed and to fall back to a whole-file save the moment anything looks wrong, **so that** saves are small and I can never lose or mangle a file because of the shortcut.
**Acceptance Criteria**:
- The encoder is engine-neutral and correct (the residual risk: a valid but wrong edit with matching checksums).
  - *Given* `EditorEngine.takePending()` after typing, deleting, undo and redo (collapsed to the net edit), *When* the encoder produces `Edit[]` in base coordinates (absolute line, UTF-16 column) with `deleted_crc32c`, `region_crc32c`, `expect_new_size` and, for T0, `expect_result_sha256`, *Then* a TS reference applier (byte-level, mirroring the server rules) applied to the golden vectors of Task 2.1.5f yields the full-replace bytes for LF/CRLF/BOM/multibyte/astral inputs, the fast-check property holds for 10,000 seeded cases, and edits that span a gap in the lazy document are converted through the gap table to the same absolute positions as a full-replace oracle.
- Stop-and-wait, no OT.
  - *Given* Save with a pending edit, *Then* exactly one `ApplyFilePatch` is in flight (a second Ctrl+S shows the existing spinner and sends nothing); edits made while it is in flight accumulate on top of the in-flight result and become the next patch based on the `new_token` of the ack; `client_id` is stable per page, `seq` increments by one per acked patch, and the client never sends a patch whose base token it has not received.
- Verified fallbacks keep the file safe.
  - *Given* a `Diverged` signal (the reconcile `StatFile{want_sha256}` after every 20 patches or on blur does not equal the client's hash of its document), *Then* the buffer stops using deltas, a banner says so, and the next save goes through `writeFull` (T0 only; a T1 `Diverged` blocks Save with "Reload to continue" and keeps the ChangeSet as a draft); *Given* `OffsetMismatch` or `ResultMismatch`, *Then* nothing was written, the buffer stays dirty, and T0 offers "Save whole file" (`writeFull`) while T1 offers Copy my edits as patch and Reload (keeping edits); *Given* `files:delta-write` turned off mid-session (caps_epoch changes), *Then* the next save uses `writeFull` for T0 and a T1 buffer becomes read-only with its ChangeSet kept as a draft.
  - *Given* a paste of more than 1 MiB or a change of more than 2,000 edits in a T0 file, *Then* the client sends `writeFull` (the 1.5 MiB cap still applies); in T1 the save is refused with the "too large to send as a patch" message and nothing is split across requests.
- ChangeSet drafts (ADR-032 amended).
  - *Given* a dirty buffer, *When* the draft backend writes, *Then* it stores the composed pending ChangeSet (`ChangeSet.toJSON`) plus selection keyed `(session, path, baseToken)`, so the 256 KB localStorage cap bounds edit volume and a 900 KB file with a few edits is backed up in the small tier (a snapshot of the same buffer needed the IndexedDB tier); the IndexedDB tier (256 KB to 1 MB of serialized edits) and the 24 h / 5 entry rules are unchanged; a snapshot draft is used only when the engine cannot produce a ChangeSet.
  - *Given* a stored ChangeSet draft whose token equals the disk token, *When* restored, *Then* the base is loaded and `ChangeSet.fromJSON` replays the edits (a base-length mismatch fails closed with "Draft doesn't match the file" and offers Copy my edits as patch), and `MergeBase = Available(disk text now)` for T0 exactly as before; with a changed token the 2-way flow applies and T1 uses the same-position rebase of Story 2.3.3.
- States are visible and have exits (ux.md 8.13): "Preparing file for editing" until `analysis=ready`; "Checking file..." during reconcile; patch-rejected, diverged, too-large, T1 unknown-outcome ("Can't tell if the last save landed - Reload to check"), offline with a ChangeSet draft ("N changes kept locally").
- Transport conformance for the patch side.
  - *Given* `runTransportConformance`, *When* run against `connect_unary` with a fake server built on the golden vectors, *Then* capability honesty, idempotent replay, out-of-order refusal, the `ResourceExhausted`-to-`PatchTooLarge` mapping, typed error mapping and abort all pass.
- Analytics (ADR-035, ADR-036).
  - *Given* a patch save, a fallback to `writeFull`, a rejected patch and a divergence, *Then* `files.edit.save` carries `transport`, `payload` and `ops` buckets plus the existing `outcome`/`via`/`size`, `files.edit.patch_rejected{reason}` fires for rejections and `files.transport.fallback` for demotions; no path, offset, line or text appears (canary test).
**Files**: `web-app/src/lib/files/transport/encodeChanges.ts`, `web-app/src/lib/files/syncLoop.ts`, `web-app/src/lib/hooks/useSaveBuffer.ts`, `web-app/src/lib/files/draftStore.ts`, `web-app/src/components/sessions/files/PatchStates.tsx`

##### Task 2.2.5a: ChangeSet encoder in `CmEngine` and the engine pending API (M)
- Promotes the Task 0.1.8a prototype to `encodeChanges.ts`; implements `onEdits/takePending/ack` in `CmEngine` (compose pending, map over non-history loads, absolute conversion through the gap table at flush), `deleted_crc32c`/`region_crc32c` using `crc32c.ts` (Task 1.6.3a), engine-conformance edit cases, the property test and oracle. Runs after Tasks 1.6.4c and 2.2.1b.
- Files: `web-app/src/lib/files/transport/encodeChanges.ts`, `web-app/src/components/sessions/files/engine/CmEngine.ts`, `web-app/src/components/sessions/files/engine/cm6LazyDoc/pendingToAbsolute.ts`, `web-app/src/lib/files/transport/__tests__/encodeChanges.test.ts`

##### Task 2.2.5b: Stop-and-wait sync loop, reconcile, `Diverged` and the `writeFull` fallbacks (M)
- `syncLoop.ts` (pure state machine: idle / inflight / pending, ack adoption, reconcile counter, `Diverged`, kill-switch and cap fallbacks) driven by `useSaveBuffer.ts` (after Tasks 2.2.1a and 2.2.3a, which also edit it); unknown-outcome rule per tier (T0 compares `expect_result_sha256` to a re-stat, T1 resolves by Reload); fake-timer tests.
- Files: `web-app/src/lib/files/syncLoop.ts`, `web-app/src/lib/hooks/useSaveBuffer.ts`, `web-app/src/lib/files/__tests__/syncLoop.test.ts`, `web-app/src/lib/hooks/__tests__/useSaveBuffer.test.ts`

##### Task 2.2.5c: `ConnectUnaryTransport.patch`/`preflight`, golden-vector and property tests (M)
- Adds `patch` and `preflight` to the adapter (after Task 1.6.3a), error mapping (`PatchRejectReason`, `ResourceExhausted` to `PatchTooLarge`), the Jest consumer of `server/services/testdata/transport/*.json`, the fast-check property test against the reference applier.
- Files: `web-app/src/lib/files/transport/connectUnaryTransport.ts`, `web-app/src/lib/files/transport/__tests__/goldenVectors.test.ts`, `web-app/src/lib/files/transport/__tests__/encodeChanges.property.test.ts`

##### Task 2.2.5d: ChangeSet drafts on the `DraftBackend` tiers (S)
- `draftStore` writes ChangeSet drafts (size by serialized edits), snapshot fallback, restore with `fromJSON` and fail-closed length check, "Copy my edits as patch"; reconciles with the Task 2.2.2d IndexedDB tier (selection by serialized size). Runs after Task 2.2.2d (same files).
- Files: `web-app/src/lib/files/draftStore.ts`, `web-app/src/lib/files/draftBackends.ts`, `web-app/src/lib/files/__tests__/draftStore.test.ts`

##### Task 2.2.5e: Delta UX states: rejected, diverged, reconcile, too large, preparing, offline draft (M)
- `PatchStates.tsx` (banners and chips per ux.md 8.13 with copy, exits and `aria-live` messages), `bannerPriority.ts` registrations, status-bar segments, the disabled-Save reasons (`AnalysisPending`, `PatchTooLarge`, `Diverged`) wired to the Task 2.2.4a reason mechanism, component tests with the shared harness.
- Files: `web-app/src/components/sessions/files/PatchStates.tsx`, `web-app/src/components/sessions/files/PatchStates.css.ts`, `web-app/src/lib/files/bannerPriority.ts`, `web-app/src/components/sessions/files/StatusBar.tsx`, `web-app/src/components/sessions/__tests__/PatchStates.test.tsx`

##### Task 2.2.5f: Patch-side transport conformance and delta analytics (S)
- The patch half of `runTransportConformance` plus `connectUnaryTransport.conformance.test.ts` additions, and the privacy/canary test rows for the new `files.edit.*` labels.
- Files: `web-app/src/lib/files/transport/__tests__/transportConformance.ts`, `web-app/src/lib/files/transport/__tests__/connectUnaryTransport.conformance.test.ts`, `web-app/src/lib/files/analytics/__tests__/transportEvents.test.ts`

## Epic 2.3: Conflict resolution and 3-way merge
**Goal**: Block, show a diff, offer explicit choices; merge when both sides changed.

#### Story 2.3.1: Conflict dialog
**As a** user, **I want** a clear diff when the file changed, **so that** I choose Overwrite / Reload / Keep editing / Merge consciously.
**Acceptance Criteria**:
- Block and diff.
  - *Given* a save returning `Conflict{FullText{content:"B", token:T3}}` for draft `"A"`, *When* handled, *Then* the dialog "File changed on disk" shows a unified diff of mine vs disk (via dynamic-imported `@codemirror/merge` `unifiedMergeView`), with +/- gutter glyphs (not colour alone), and Keep editing is the primary action.
- Unavailable disk version disables Merge and Overwrite.
  - *Given* a save returning `Conflict{Unavailable{reason}}` (agent grew the file past 1 MB, replaced it with latin-1, made it binary or Mixed EOL), *When* the dialog opens, *Then* it states the reason, shows no diff, offers only **Keep editing**, **Copy my version** and **Reload from disk** (reload shows what the read path can show, read-only), and Merge and Overwrite are disabled (`aria-disabled` with the reason): the client cannot overwrite a file it cannot show, and a deliberate overwrite of a large file stays impossible (ADR-031). `Unavailable{Deleted}` keeps the existing deleted-file variant.
- Overwrite is explicit.
  - *Given* a `FullText` conflict, *When* Overwrite is chosen and confirmed with the path `src/a.go` displayed, *Then* `WriteFile` is re-sent with `expected_token=T3` and succeeds.
- Reload offers a copy.
  - *Given* dirty edits, *When* Reload is chosen, *Then* "Copy my version" is offered first, then the buffer reloads with `baseToken=T3` and `dirty=false`.
- Mobile presentation.
  - *Given* 390px width, *When* shown, *Then* it renders as a full-screen sheet with focus trap and a hunk next/prev control.
- A change detected while the dialog is open refreshes it in place (triad round 2, ux.md 8.9, U-CONF-10).
  - *Given* the conflict dialog open on `theirs` token `T3`, *When* the 10 s poll sees `T4` on two consecutive checks, *Then* the diff recomputes against `T4`, a polite "Disk changed again - diff updated" shows, focus does not move, the underlying stale banner is not shown (and not shown after close if the dialog already reflects `T4`), the Overwrite re-send uses `T4`, and in the merge view resolutions for unchanged regions are kept (jest with fake timers).
- Overlay layering (triad repair, ux.md section 9.1a, U-MOB-12).
  - *Given* a phone, *When* the conflict sheet opens Merge, the Reload copy-first step, the Overwrite confirm and the Discard-merge-choices prompt, *Then* Merge swaps the content of the same sheet (not stacked), Reload's copy step and the Overwrite confirm are inline steps of that sheet, only the Discard prompt is an alert dialog over it (maximum depth: one sheet plus one alert dialog); a sheet never opens over a sheet; Esc/back closes only the top layer; a table-driven jest test counts layers for each path.
  - *Given* the merge view S8 inside the sheet, *When* Esc, system back or the top-bar `<` is pressed (triad round 3, ux.md section 10, U-CONF-9), *Then* the sheet **never closes**: with no resolutions made it returns to S7; with resolutions it opens the "Discard merge choices?" alert (default **Keep merging**, which stays in S8) and **Discard** clears the choices and returns to S7; the **Back to diff** button returns to S7 keeping the choices (choosing Merge... again restores them); Esc from S7 is Keep editing. A jest test walks all four paths and asserts the layer count and the retained/cleared resolutions.
- Very large diffs cannot be overwritten blind (triad round 3, ux.md 9.3, U-CONF-11).
  - *Given* a `FullText` conflict with more than 5,000 changed lines, *When* the dialog opens, *Then* it shows the summary (counts, both sizes, the first 200 changed lines) with Overwrite... and Merge... `aria-disabled` and the visible reason "Show the full diff to enable this", no text mentions a download of both versions, Keep editing and Reload stay enabled, and **Show full diff** renders the whole diff in chunks (a slow-device warning on a coarse pointer) and then enables both.
- Analytics (ADR-035).
  - *Given* a conflict, Overwrite confirmed, Reload with copy-first, *When* each happens, *Then* `files.edit.save_conflict` (`theirs` {full_text, deleted, too_large, binary, non_utf8, mixed_eol, symlink}, `merge_available`), `files.edit.overwrite` (`outcome` {confirmed, cancelled}) and `files.edit.reload` (`source`, `copied_first`) are emitted without any diff text or path.
**Files**: `web-app/src/components/sessions/files/ConflictDialog.tsx`, `web-app/src/components/sessions/files/ConflictDialog.css.ts`, `web-app/src/components/sessions/__tests__/ConflictDialog.test.tsx`

##### Task 2.3.1a: `pnpm add @codemirror/merge` (pinned) + dynamic-import wrapper (XS)
- `pnpm add` in `web-app/`; commit `pnpm-lock.yaml`; never npm.
- Files: `web-app/package.json`, `web-app/pnpm-lock.yaml`

##### Task 2.3.1b: `ConflictDialog` (M)
- Exhaustive handling of `ConflictResolution` and of `TheirsVersion` (`FullText` | `Unavailable`); changed-hunks collapse; Merge also disabled when `MergeBase` is `Unavailable` (restored draft against a changed file); the > 5,000-changed-line summary state with Show full diff gating Overwrite and Merge; the Back rules of the layering AC above.
- Files: `web-app/src/components/sessions/files/ConflictDialog.tsx`, `web-app/src/components/sessions/files/ConflictDialog.css.ts`

##### Task 2.3.1c: Tests (S)
- Files: `web-app/src/components/sessions/__tests__/ConflictDialog.test.tsx`

#### Story 2.3.2: 3-way merge
**As a** user, **I want** to merge my edits with the agent's, **so that** neither is lost.
**Acceptance Criteria**:
- Clean auto-merge.
  - *Given* `MergeBase` `"a\nb\nc\n"`, mine `"a\nB\nc\n"`, theirs `"a\nb\nc\nd\n"`, *When* `merge3` runs, *Then* `MergeResult.text` is `"a\nB\nc\nd\n"` with zero conflicts.
- A merge is never saved unreviewed (pre-mortem #9: diff3 merges adjacent but semantically coupled edits cleanly).
  - *Given* a clean `merge3` result, *When* the user chooses Merge, *Then* the dialog opens the result pane with auto-merged hunks labelled "from you" / "from disk" and does not call `WriteFile`; Save is a separate explicit action (no auto-save after merge).
  - *Given* any auto-merged hunk with `nearTheirs` (overlapping a theirs hunk or within 1 line of one), *When* the result pane is shown, *Then* Save stays disabled until the user ticks "Review merged result" (and the first such hunk is scrolled into view); a merge with no `nearTheirs` hunk needs only the explicit Save.
  - *Given* a property test over random edit pairs, *When* two edits are adjacent (touching or one line apart), *Then* `merge3` returns an `autoMergedHunk` with `nearTheirs=true` for them.
- Conflicts are surfaced.
  - *Given* base `"x\n"`, mine `"y\n"`, theirs `"z\n"`, *When* `merge3` runs, *Then* one conflict region with `mine="y"`, `theirs="z"` is returned and the UI requires choosing Mine / Theirs / Both before Save is enabled.
- Identity laws.
  - *Given* any base and text `t`, *When* `merge3(base, t, base)` and `merge3(base, base, t)` run, *Then* both return `t` (property test, 200 random cases).
- Save against theirs token.
  - *Given* a resolved merge (only reachable from `FullText` with `MergeBase = Available`), *When* Save is pressed, *Then* `WriteFile` sends the LF-normalized merged text with `expected_token=T3`, and on success the buffer's `baseToken` and `MergeBase` become the saved text.
- Never merges against partial or missing data.
  - *Given* `TheirsVersion=Unavailable` or `MergeBase=Unavailable`, *When* the dialog renders, *Then* Merge is disabled with the reason and `merge3` is never called.
- Mobile.
  - *Given* 390px width, *When* the merge view opens, *Then* it uses the unified presentation with a per-hunk action row of >= 44px targets.
- Phone kill switch and runtime degrade (triad repair, ux.md section 10, U-MOB-13).
  - *Given* a 390px viewport and `files:merge-phone=false` (or a container < 360px, or the merge library failing to load), *When* a conflict dialog opens, *Then* Merge... is disabled with "Merge is not available on this screen" and the 2-way flow (Keep editing / Reload / Overwrite / Copy my version) is fully usable; *Given* a 1280px viewport with the same flag off, *Then* Merge is still available. The flag is read through `useFilesFlags().mergePhone` and re-evaluated on flag refresh without losing resolutions already made.
- Phone merge states (triad round 2, ux.md 10.1, U-MOB-15).
  - *Given* 390px width, a merge base, mine and theirs producing 2 conflicts and 3 `nearTheirs` areas, *When* the merge opens, *Then* the Overview shows "3 auto-merged areas to review" and "2 conflicts left" with Save merged disabled and its visible reason; each conflict is a one-card Resolve screen with two base context lines above and below (expander "N unchanged lines"), Yours / Disk / Both buttons >= 44 px, a collapsed summary with **Change** after choosing, and **Undo last choice** on the Overview (unlimited in-memory stack, polite "Conflict 1 reopened"); after the last conflict a Review stepper shows only the `nearTheirs` areas ("Area k of N", Previous/Next >= 44 px) and the "I reviewed all N areas" checkbox stays disabled until every flagged area has been displayed once, with Save merged disabled and the visible reason "Review N areas first"; a merge with no flagged area skips the stepper.
  - *Given* a merge in progress with resolutions made, *When* the container becomes narrower than 360px, *Then* the pane is replaced by "Merge needs a wider screen. Your choices are kept." with **Back to diff**, all choices are retained in memory, widening restores the pane, and nothing is saved.
- Analytics (ADR-035).
  - *Given* a merge opened with 2 conflicts and saved after the required review, *When* each happens, *Then* `files.edit.merge_open` (`hunks`, `conflicts` buckets) and `files.edit.merge_saved` (`reviewed`, `conflicts_resolved` bucket, `auto_only`) are emitted.
**Files**: `web-app/src/lib/files/merge3.ts`, `web-app/src/lib/files/__tests__/merge3.test.ts`, `web-app/src/components/sessions/files/MergeView3Way.tsx`, `web-app/src/components/sessions/files/ConflictDialog.tsx`

##### Task 2.3.2a: Library spike: license, size, API of diff3 candidates; record in ADR-034 (S)
- Evaluate `node-diff3` (and alternatives) for license, bundle size (dynamic import), maintenance; do not hand-roll a diff algorithm. Decision gate for 2.3.2b.
- Files: `project_plans/files-viewer-revamp/decisions/ADR-034-three-way-merge-for-save-conflicts.md`

##### Task 2.3.2b: `merge3.ts` wrapper and property tests (M)
- Pure function returning `MergeResult` (including `autoMergedHunks[]` with `nearTheirs`, derived from the diff3 hunk ranges); operates on LF-normalized text only (inputs `base`, `mine`, `theirs` are already LF from the client load function and the server `FullText`); it does not apply EOL or BOM. The server re-applies EOL/BOM from the token-verified on-disk file on save (single owner).
- Files: `web-app/src/lib/files/merge3.ts`, `web-app/src/lib/files/__tests__/merge3.test.ts`, `web-app/package.json`, `web-app/pnpm-lock.yaml`

##### Task 2.3.2c: `MergeView3Way` UI and `Merge` action wiring (M)
- Editable result pane with conflict hunk controls (Mine/Theirs/Both), hunk-origin labels, the "Review merged result" checkbox gating Save when any hunk is `nearTheirs`, dynamic import, a11y labels. Merge never triggers a save. ADR-036: the merge UI is mounted through the `MergeSurface` port by Task 2.3.3c (after this task and Task 2.3.2f); until then it mounts `@codemirror/merge` directly inside the `engine/` boundary.
- Files: `web-app/src/components/sessions/files/MergeView3Way.tsx`, `web-app/src/components/sessions/files/ConflictDialog.tsx`

##### Task 2.3.2d: Component tests (S)
- Files: `web-app/src/components/sessions/__tests__/MergeView3Way.test.tsx`

##### Task 2.3.2f: Phone merge flow: Overview, one-card Resolve, Review stepper, shrink fallback (M)
- **Ordered after Task 2.3.2e** (triad round 3): 2.3.2e defines the `phoneMergeAvailable` predicate (flag x container width x library state) that this task's "wider screen" fallback consumes, so the two must not run in parallel and 2.3.2e merges first (list order here is historical; the dependency is 2.3.2c -> 2.3.2e -> 2.3.2f). `MergeViewPhone.tsx` (the unified per-hunk presentation of Task 2.3.2c split out for the phone: Overview, Resolve card with base context and Change/Undo, Review stepper gated by `nearTheirs`, "wider screen" fallback keeping the resolution state in the parent dialog), component tests over the states in ux.md 10.1 and the reducer for the undo stack. Desktop keeps the editable result pane of Task 2.3.2c.
- Files: `web-app/src/components/sessions/files/MergeViewPhone.tsx`, `web-app/src/components/sessions/files/MergeView3Way.tsx`, `web-app/src/components/sessions/__tests__/MergeViewPhone.test.tsx`

##### Task 2.3.2e: Phone merge kill switch and runtime degrade to the 2-way flow (S)
- Read `useFilesFlags().mergePhone` plus a container-width check (< 360px) and the merge-library load failure in `ConflictDialog`; disabled-with-reason Merge; table test over flag x width x library state; exports the `phoneMergeAvailable({flag, containerWidth, libraryState})` predicate that Task 2.3.2f consumes. Runs before Task 2.3.2f.
- Files: `web-app/src/components/sessions/files/ConflictDialog.tsx`, `web-app/src/lib/hooks/useFilesFlags.ts`, `web-app/src/components/sessions/__tests__/ConflictDialog.test.tsx`

#### Story 2.3.3: Delta conflicts: `PreflightFilePatch`, same-position rebase and the `MergeSurface` seam (ADR-036 Phase C; ADR-034 amended)
**As the** owner editing a big file an agent also touches, **I want** a conflict to keep my edits and never guess where they belong, **so that** a rebase is either provably the same edit or not done.
**Acceptance Criteria**:
- Documents are merged, deltas are not rebased (ADR-034 amended).
  - *Given* a T0 `Conflict{FullText}`, *Then* the flow is exactly Story 2.3.1/2.3.2 (the client's `MergeBase` and `merge3` over documents); the pending ChangeSet is never transformed against the other side's change.
- T1 conflicts (`theirs = Unavailable{TooLarge}`).
  - *Given* a T1 `Conflict`, *When* the dialog opens, *Then* Merge and Overwrite are disabled with the reason "File is too large to compare", and Keep editing, Reload (keeping my edits as a ChangeSet draft) and **Rebase if lines unchanged** are offered.
  - *Given* `PreflightFilePatch(new_base_token, edits)`, *When* each edit's `deleted_crc32c` and the CRC of its full touched line(s) still match at the same line numbers in the new file, *Then* the response is `ok` (no write happens), the client adopts the new token and re-sends the same patch; *When* an agent inserted lines above (lines moved) or any CRC differs, *Then* the response is `lines_moved`/`mismatch`, the client fails closed with "The file changed around your edits - Reload and re-apply", and nothing is written or guessed (no diff-and-guess rebase exists in the code).
- `MergeSurface` seam.
  - *Given* `MergeView3Way` mounted through the `MergeSurface` port (`CmMergeSurface` wrapping `@codemirror/merge` and `merge3`), *When* `features.mergeView` is false (an engine without merge), *Then* the Merge action is hidden/disabled with a reason and the 2-way flow still ships; with CM6 the Story 2.3.2 tests pass unchanged.
- Server contract.
  - *Given* `PreflightFilePatch`, *Then* it applies the same flag, confinement, hidden-session and rate-limit policy as `ApplyFilePatch`, is registered in `guardedProcedures`, reads at most the touched lines through the index, never writes, and is covered by `FileOpsConformance`.
- Analytics.
  - *Given* a rebase attempt, *Then* the existing `files.edit.save_conflict{theirs=too_large, merge_available=false}` and the new `files.edit.preflight{outcome}` (ADR-035 "Transport extension") are emitted with enum labels only.
**Files**: `server/services/file_ops_preflight.go`, `web-app/src/components/sessions/files/ConflictDialog.tsx`, `web-app/src/lib/files/rebase.ts`, `web-app/src/components/sessions/files/engine/CmMergeSurface.ts`, `web-app/src/components/sessions/files/MergeView3Way.tsx`

##### Task 2.3.3a: `PreflightFilePatch` server (S)
- `file_ops_preflight.go`: verifies per-edit `deleted_crc32c` and touched-line CRC against the new file through the line index (reads only the touched lines), returns `ok | lines_moved | mismatch | analysis_pending`, never writes, shares the patch rate limit; handler in `files_delegates.go` (after Task 2.1.5e); its conformance cases run from `file_ops_preflight_test.go` through the shared `FileOpsConformance` helper.
- Files: `server/services/file_ops_preflight.go`, `server/services/file_ops_preflight_test.go`, `server/services/files_delegates.go`

##### Task 2.3.3b: T1 conflict flow and same-position rebase (M)
- `rebase.ts` (preflight call, token adoption, re-send, fail-closed copy), `ConflictDialog.tsx` T1 variant (disabled Merge/Overwrite with reason, Rebase and Reload-keeping-draft actions; after Task 2.3.2e), `useSaveBuffer` wiring (after Task 2.2.5b), component and hook tests.
- Files: `web-app/src/components/sessions/files/ConflictDialog.tsx`, `web-app/src/lib/files/rebase.ts`, `web-app/src/lib/hooks/useSaveBuffer.ts`, `web-app/src/components/sessions/__tests__/ConflictDialog.test.tsx`, `web-app/src/lib/files/__tests__/rebase.test.ts`

##### Task 2.3.3c: `MergeSurface` implementation and the capability-gated Merge action (S)
- `CmMergeSurface` implements the port, `MergeView3Way.tsx` mounts through it (after Task 2.3.2f), Merge hidden when `features.mergeView` is false; tests with a fake surface.
- Files: `web-app/src/components/sessions/files/engine/CmMergeSurface.ts`, `web-app/src/components/sessions/files/MergeView3Way.tsx`, `web-app/src/components/sessions/__tests__/MergeView3Way.test.tsx`

## Epic 2.4: Vim and mobile editing input
**Goal**: Real vim on desktop with a hardware keyboard; usable touch editing on phones.

#### Story 2.4.1: Lazy vim mode
**As a** vim user, **I want** real vim with `:w`, **so that** muscle memory works.
**Acceptance Criteria**:
- Lazy and gated by InputProfile.
  - *Given* `InputProfile=HardwareKeyboard` at the moment a buffer **enters Edit** (the default is read only at Edit entry, never re-evaluated live; primary `(pointer: fine) and (hover: hover)` matches, or a trusted strong keydown was seen, per ADR-033/ux.md 8.8; a touchscreen laptop with a mouse is therefore `HardwareKeyboard`, a phone with a Bluetooth keyboard becomes `HardwareKeyboard` after an Escape/arrow/F-key/Ctrl-chord keydown or three matching letters, not after Enter/Backspace/Space/Tab), *When* a file opens in Edit, *Then* `@replit/codemirror-vim` is dynamically imported and `-- NORMAL --` shows in the status bar (`role="status"` with `aria-live="off"`, so ordinary mode changes are not announced; toggling vim on/off announces "Vim mode on, Normal" politely, and focusing the segment reads the current mode; U-STATUS-3); with `InputProfile=Touch` vim stays off unless toggled and the main bundle contains no vim code.
- `:w` uses the one save path.
  - *Given* vim on and a dirty buffer, *When* `:w` is run, *Then* `useSaveBuffer.save()` is invoked (conflict-checked), not a second implementation.
- Jumplist.
  - *Given* vim on, *When* Ctrl-o / Ctrl-i are pressed, *Then* they step `NavEntry` back/forward.
- Old keymap removed.
  - *Given* the former hand-rolled vimKeymap with `H`/`L` selection, *When* the build completes, *Then* it is deleted and `H`/`L` have real vim semantics.
- Escape from the editor.
  - *Given* focus in the editor, *When* Esc then Tab is pressed (vim off), *Then* focus leaves the editor (no keyboard trap).
- Vim cannot consume the dedicated escape, and it is not a browser-reserved chord (triad round 2: Ctrl+Shift+M is Chrome's profile menu and Firefox's Responsive Design Mode).
  - *Given* vim on and in each of Normal, Insert, Visual and a pending `:` command-line, *When* Esc is pressed until Normal and then Esc, Esc again within 500 ms with no pending operator, count or register (and, separately, the status-bar "Leave editor" control, the primary documented exit, is activated, and separately `:leave` is run), *Then* focus leaves the editor to the status bar in every mode, and `:map`/`:nmap`/`:imap <Esc>`, `Vim.map` or `Vim.defineEx` cannot override it: the Esc, Esc handler is a capture-phase keydown listener on the editor DOM registered after the vim extension and re-registered on every vim Compartment reconfigure, `:leave` is defined after the user's rc, with tests that execute `:nmap <Esc> x` and `:imap <Esc> x` and assert the escape still works; a single Esc, and Esc followed by another key within the window, only behave as vim. All three are listed in the shortcut overlay; no Ctrl/Alt chord is used for this action.
  - *Given* the armed double Esc (triad round 3, ux.md 8.4 table, U-EDIT-8, U-EDIT-23), *When* the first Esc arrives in Normal mode with nothing pending, *Then* the status bar shows the chip "Esc again to leave the editor" (1.5 s, announced politely) and a second Esc while the chip shows leaves; *When* the first Esc arrives in Insert or Visual (it returns to Normal), *Then* no chip shows and a second Esc within 500 ms is a plain Normal-mode Esc; *When* the per-device setting "Esc, Esc leaves the editor" is off, *Then* the chip never shows and only `:leave`, the control and the pointer route leave; *When* focus is on the Leave editor control, *Then* Esc, Enter or Space returns focus to the editor with cursor, scroll and Normal mode intact; *When* an overlay (dialog, sheet, palette) is open, *Then* a double Esc closes that layer and its second press is swallowed (never closes a layer and then acts on the editor); *When* vim is turned on for the first time, *Then* a one-time status "Press Esc twice, or type :leave, to leave the editor" shows. With vim on, Tab from inside the editor is a vim key and is asserted *not* to reach the status bar (the corrected claim in ux.md 8.4).
- Vim default is evaluated only at Edit entry and never flips live (ADR-033).
  - *Given* a buffer already in Edit on a `Touch` profile with vim off, *When* auto-detect later resolves `HardwareKeyboard`, *Then* the key bar hides, the one-time status shows, vim stays off, and a chip "Hardware keyboard detected - turn vim on?" offers **Turn on** / **Not now**; *Given* a buffer in Edit with vim on, *When* the profile changes to `Touch`, *Then* vim stays on; *Given* a persisted explicit vim toggle, *Then* it wins at every Edit entry.
- Input profile precedence and Bluetooth keyboards (triad repair, ux.md section 8.8, U-INPUT-1; ADR-033 amended).
  - *Given* a phone with `(pointer: coarse)`, no override, *When* a soft-keyboard sequence of the Story 0.1.5 fixtures (letters, Enter, Backspace, Space, Tab, digits, punctuation, IME composition, synthetic key-bar events) is replayed, *Then* the resolved profile stays `Touch` with zero flips; *When* a Bluetooth sequence of the fixtures arrives (an Escape, arrow, F-key or Ctrl-chord keydown, or three `Key<L>` letters with matching `code`), *Then* the resolved profile becomes `HardwareKeyboard` for the session, the key bar hides, a one-time status "Hardware keyboard detected - key bar hidden. Change in Overflow > Keyboard" shows, and nothing is persisted; *Given* a persisted override `Touch`, *When* a physical key is pressed, *Then* the profile stays `Touch` (manual override always wins) and the key bar stays; *Given* a persisted override `Hardware` on a coarse device, *Then* the key bar is hidden. Precedence order: override > auto-detect > `Touch`; a persisted explicit vim toggle always wins over the profile default, and vim is never auto-enabled when the resolved profile is `Touch`.
- Analytics (ADR-035).
  - *Given* vim toggled on from the status bar and Esc, Esc (from Insert, through Normal) leaving the editor, *When* each happens, *Then* `files.vim.toggle` (`to`, `source`) and `files.vim.escape` (`via`={esc_esc,status_control,ex_leave}, `mode`) are emitted, and a profile change emits `files.input.profile_change` (`to`, `source`); ordinary vim mode changes emit nothing.
- Vim is an engine capability (ADR-036).
  - *Given* `EditorEngine.features.vim` false (an engine without Vim keys, such as the contingent custom engine), *When* the vim toggle or the `:`-commands would be offered, *Then* the toggle is disabled with "Vim keys unavailable for this engine", the Esc, Esc handler is not armed, and no `@replit/codemirror-vim` import exists outside `CmEngine`; with `CmEngine` every AC of this story holds unchanged.
**Files**: `web-app/src/lib/files/vimLoader.ts`, `web-app/src/lib/files/inputProfile.ts`, `web-app/src/components/sessions/CodeEditor.tsx`, `web-app/src/lib/files/__tests__/inputProfile.test.ts`

##### Task 2.4.1a: `pnpm add @replit/codemirror-vim` + `inputProfile.ts` + `vimLoader.ts` (M)
- Files: `web-app/package.json`, `web-app/pnpm-lock.yaml`, `web-app/src/lib/files/inputProfile.ts`, `web-app/src/lib/files/vimLoader.ts`

##### Task 2.4.1b: Compartment wiring, `Vim.defineEx` for `:w`/`:q`/`:leave`, mode indicator, jumplist maps, armed Esc, Esc handler and setting, delete old keymap (M)
- Also builds `extensions/escLeave.ts` (the armed double-Esc handler with the hint chip, the per-device setting and its Overflow/overlay entry, `:leave`, the one-time vim-on status); `escLeave.ts` is a new module registered by one line in `CodeEditor.tsx`. First suspect for the Wave 8 overrun checkpoint.
- Files: `web-app/src/components/sessions/CodeEditor.tsx`, `web-app/src/components/sessions/FileContentViewer.tsx`, `web-app/src/components/sessions/files/StatusBar.tsx`, `web-app/src/components/sessions/files/extensions/escLeave.ts`

##### Task 2.4.1c: Persisted per-device vim toggle + tests (S)
- Files: `web-app/src/lib/files/inputProfile.ts`, `web-app/src/lib/files/__tests__/inputProfile.test.ts`

##### Task 2.4.1d: Input-profile precedence, conservative keyboard detection and the Overflow "Keyboard: Auto / Touch / Hardware" override (S)
- Pure `detectHardwareKeyboard(events, env)` (S1-S4 of ADR-033, trusted events only, tested against the Story 0.1.5 fixtures) and `resolveInputProfile({override, detected})` plus session-only detection state; the override is the only persisted input; unit tests cover every precedence combination including a Bluetooth keyboard on a coarse device and a disconnected keyboard.
- Files: `web-app/src/lib/files/inputProfile.ts`, `web-app/src/lib/files/__tests__/inputProfile.test.ts`, `web-app/src/components/sessions/files/MobileFilesLayout.tsx`

#### Story 2.4.2: Extra-key bar for phone editing
**As a** phone user, **I want** Esc/Tab/Ctrl/brackets above the keyboard, **so that** I can edit and use vim best-effort.
**Acceptance Criteria**:
- Pinned above keyboard.
  - *Given* `visualViewport` height 500 with offsetTop 0, *When* edit starts, *Then* the bar's bottom aligns to `offsetTop + height`, keys are 44x44, row scrolls horizontally.
- Sticky modifiers.
  - *Given* Ctrl tapped once then `d`, *When* handled, *Then* Ctrl+D is dispatched once and Ctrl returns to idle; double-tap Ctrl locks until tapped again; state is visibly distinct.
- Esc first and emphasised in vim.
  - *Given* vim on, *When* rendered, *Then* Esc is the first key and Save shows dirty state and is disabled when clean.
- Save and Stop editing are never inside the scrolling row (triad round 2, ux.md 7.1a/12, U-KEY-5, U-MOB-14).
  - *Given* the key bar, *When* it renders, *Then* Save and Stop editing are two 44x44 buttons in a fixed cluster at the right end, outside the horizontally scrolling key list, and the list scrolls in the remaining width (jest layout assertion plus the Playwright bounding-box check: the cluster stays at the same x while the list scrolls).
- Edit layout budget and fallback ladder (ux.md 7.1a, U-MOB-2/7/14).
  - *Given* `editLayoutBudget(viewport, keyboard, inputProfile, flags)` over the matrix {XS-568 (308 px visual viewport), S-844 (584), L-390 (190), XS-L (120)} x {vim on, off} x {0, 1, 2 banners, one of them priority 1} x {keyboard 260 px, 320 px tall} x {inputProfile Touch, Hardware, Hardware with "Show key bar"}, *When* evaluated, *Then* the result lists the rows shown in the priority order top bar > one banner row > editor > key row > vim row > status bar, applies the ladder (merge the vim row into the main row; fold banners to one 44 px row that, for a priority-1 banner, keeps **Copy my version** and **Retry Save** as two 44x44 buttons, or to an "N notices" chip with a 44x44 hit area; auto-hide the top bar and move Save/Stop into the key bar's fixed cluster **only while the key bar is shown** (a Hardware profile without the key bar never auto-hides the top bar); hide the status bar and quickfix chip; keep only Esc/Tab/arrows plus the cluster with the one-time "Little room" hint) until the editor region is >= 120 px, never goes below the 64 px floor by removing Save or Stop editing, and is a pure function with a table test; at least one of Save and Stop editing is visible in every row of the matrix whenever a buffer is dirty in Edit.
  - *Given* the Playwright mobile project with `visualViewport` stubbed to each named viewport and keyboard state (including 844x390, which is a phone and gets the mobile layout by the `chooseFilesLayout` rule), *When* an Edit session is dirty, *Then* exactly one Save and one Stop editing control (>= 44 px) are visible, the editor region meets the height rule above, and dismissing the keyboard restores the top bar; *Given* a dirty buffer with a priority-1 banner at 320x568, *Then* the collapsed banner row shows Copy my version and Retry Save as >= 44 px buttons.
  - *Given* the top bar auto-hidden (cluster in the key bar) and the resolved profile flipping to `Hardware` (detection or override), *When* the layout recomputes, *Then* the top bar with Save and Stop editing is visible in the same render in which the key bar is removed (no frame with neither; U-MOB-17), and flipping back to `Touch` re-applies the ladder.
- Composition-safe.
  - *Given* `view.composing === true`, *When* a bar key is tapped, *Then* it queues until `compositionend`.
- Vim row only with vim on (resolves the ux.md section 12 contradiction with the owner decision).
  - *Given* `InputProfile=Touch` with vim off, *When* the bar renders, *Then* it shows no vim row and Esc is not emphasized; *Given* the user opted into vim from Overflow, *Then* the second row (`:` `/` `h` `j` `k` `l`) appears and Esc is first and emphasized; vim is never turned on by merely opening the bar.
- Analytics (ADR-035).
  - *Given* 40 arrow taps within 30 s and one Save-key tap, *When* the window closes (or `pagehide`), *Then* one `files.mobile.extra_key_used{key_class=arrow,count=21+}` and one `{key_class=save,count=1}` are emitted (never per keystroke, never the key's character); `files.mobile.keyboard_open` (`trigger`, `bar_visible`) is coalesced within 2 s.
**Files**: `web-app/src/components/sessions/files/ExtraKeyBar.tsx`, `web-app/src/components/sessions/files/ExtraKeyBar.css.ts`, `web-app/src/lib/files/editLayoutBudget.ts`, `web-app/src/components/sessions/__tests__/ExtraKeyBar.test.tsx`, `web-app/src/lib/files/__tests__/editLayoutBudget.test.ts`, `tests/e2e/files-mobile.spec.ts`

##### Task 2.4.2a: Locate and reuse terminal supplemental-key styles; build `ExtraKeyBar` (M)
- Files: `web-app/src/components/sessions/files/ExtraKeyBar.tsx`, `web-app/src/components/sessions/files/ExtraKeyBar.css.ts`

##### Task 2.4.2c: `editLayoutBudget` ladder and the fixed Save/Stop cluster (M)
- Pure `editLayoutBudget.ts` (rows by priority with the 44 px banner row, the five-step ladder, hard floor, `inputProfile` input and the same-pass restore of the top bar when the profile flips to Hardware), the fixed non-scrolling Save/Stop cluster in `ExtraKeyBar`, the matrix table test and the Playwright named-viewport cases in `tests/e2e/files-mobile.spec.ts` (the spec file that Task 1.5.3b created in Wave 6; this task adds a describe block, so it runs after Wave 6's edits, trivially); `files.mobile.layout_fallback` analytics (coalesced 2 s). Ordered after Task 2.4.2a and before 2.4.2b mounts the bar.
- Files: `web-app/src/lib/files/editLayoutBudget.ts`, `web-app/src/components/sessions/files/ExtraKeyBar.tsx`, `web-app/src/lib/files/__tests__/editLayoutBudget.test.ts`, `tests/e2e/files-mobile.spec.ts`

##### Task 2.4.2b: Mobile Edit button, bar mounting, landscape mode, tests (M)
- Floating 44px Edit button (disabled with the reason popover when the file cannot be edited, Task 2.2.1b/c); the bottom bar is hidden while the keyboard is up; the top bar auto-hides **only** when `editLayoutBudget` says the editor would be < 120 px, in which case a visible 44px-tall top-edge reveal handle opens the overflow menu (U-MOB-7) and Save/Stop live in the key bar's fixed cluster (Task 2.4.2c); jest asserts the handle is present when the top bar is hidden. On tablets (>= 768px, coarse pointer) the bar mounts above the keyboard in the desktop layout (U-TABLET-1).
- Files: `web-app/src/components/sessions/files/MobileFilesLayout.tsx`, `web-app/src/components/sessions/__tests__/ExtraKeyBar.test.tsx`

#### Story 2.4.3: Editing e2e
**As the** owner, **I want** CI proof of save/conflict/merge on desktop and mobile, **so that** the dangerous path is guarded.
**Acceptance Criteria**:
- Save round trip.
  - *Given* the e2e server with the global `files:editing` set via `UpdateFeatureFlag`, *When* the test edits `pkg/a.go` line 12 and presses Ctrl+S, *Then* the file on disk equals the edit byte-for-byte and the tab dot clears.
- Conflict.
  - *Given* the test writes `pkg/a.go` directly on disk after the editor loaded, *When* the user saves, *Then* the "File changed on disk" dialog appears and the disk file is unchanged until Overwrite is confirmed.
- Merge.
  - *Given* disjoint edits on both sides, *When* Merge is chosen, *Then* the merged file contains both edits.
- Flag off.
  - *Given* the global `files:editing=false`, *When* a direct RPC `WriteFile` is posted to `/api/session.v1.SessionService/WriteFile` with a **valid `Host` (the e2e server's own `localhost:<port>`) and a matching `Origin`** (so it passes the Story 2.1.2 `guardedProcedures` rebinding guard and reaches the handler), *Then* it fails with `failed_precondition` (Connect error, `ReadOnlyReason=FlagOff`, per the Story 2.1.2 table). *Given* the same call with a foreign `Origin`, *Then* it is refused by the guard (403, not `failed_precondition`); the test asserts both so a guard refusal cannot masquerade as the flag-off result.
- Mobile.
  - *Given* the gating mobile project (`mobile-webkit`, or `mobile-chromium` if Task 0.1.4b found WebKit unreliable in CI), *When* Edit is tapped, *Then* the ExtraKeyBar is visible and Save works; *and* at 320x568 (visual viewport 308), 390x844 (584) and 844x390 (190) with a dirty buffer, exactly one Save and one Stop editing control are visible and the editor region follows the 7.1a rule (U-MOB-14).
**Files**: `tests/e2e/files-editing.spec.ts`, `tests/e2e/pages/files-tab.page.ts`

##### Task 2.4.3a: Specs and helpers (direct-disk write helper in the test dir) (M)
- Header `// @feature files:editing, files:conflict`; no `waitForTimeout`; `data-testid` locators.
- Files: `tests/e2e/files-editing.spec.ts`, `tests/e2e/pages/files-tab.page.ts`

## Epic 2.5: Hardening and rollout
**Goal**: Gates green, flag-flip procedure documented, owner device check.

#### Story 2.5.1: Gates, registry, docs
**As a** maintainer, **I want** every repo gate green, **so that** CI does not block the PR.
**Acceptance Criteria**:
- Gates.
  - *Given* the complete diff, *When* `make ready`, `pnpm run lint:duplicates` (in `web-app/`) and `make ready-complexity-gate` run, *Then* all pass (jscpd <= 0.14%, dupl/gocognit/funlen clean for new and modified functions).
- Registry current.
  - *Given* new RPCs and `// +feature:` markers, *When* `make registry-diff` runs, *Then* it shows no changes.
- Size budgets and follow-ups.
  - *Given* the final diff, *When* `wc -l` runs on `CodeEditor.tsx` and `FilesTab.tsx`, *Then* each is <= 350 lines; and a backlog item "retire `files:viewer-v2` kill switch and legacy single-file layout after 4 weeks flag-on" exists, carrying the numeric adoption review rule of ADR-035 section 5 (verdict from the `/api/analytics/files` `review` block; inconclusive below `adoptionMinEvents` server events or `adoptionMinActiveDays` active days) **and the low-adoption action of that section** (triad round 3: on `editing_tried_not_adopted`, `retire_or_rework` or two consecutive `inconclusive` verdicts, flip the global `files:editing` flag off, keep the read-only features, open a follow-up to delete the write path, merge, key bar and draft stores if editing is still unused 8 weeks later, and retire v2 only if search and tabs are also unused); the backlog item also records that the thresholds are owner-editable proposals to be edited before the verdict is read.
- Docs.
  - *Given* the how-to, *When* read, *Then* it states how to enable the global `files:editing` flag (no per-scope override exists), the conflict model (including `Unavailable` disk versions and that an mtime-only change is not a conflict), the directory-mode editing default, the residual agent-race window including that a UI save can make a Claude agent's next Edit fail with "modified since read" and that a non-Claude agent holding an old copy can overwrite a saved edit, and the known rename-save limits (hard links, xattrs/ACLs/SELinux labels, file watchers see create+rename; pre-mortem #7, #15), the reserved-chord table (ux.md 14.1), the keyboard override and the Accessible view with the supported / best-effort / unsupported screen-reader matrix from Task 2.5.4b.
- The wave gates already ran.
  - *Given* Story 0.2.4's per-wave `make files-viewer-gates` output in the PR description for Waves 2 to 10, *When* this story runs the full gates, *Then* any new failure is attributable to this wave's diff alone.
- Transport gates (ADR-036; executed in Story 2.5.5 and Epic 2.6).
  - *Given* the new RPCs (`GetFileTransportCapabilities`, `ReadFileRange`, `ApplyFilePatch`, `PreflightFilePatch`, and the Phase D stream/GET routes), *When* this story's gates run, *Then* the registry is current for them (Task 2.5.5c), `FileOpsConformance` and `runTransportConformance` and `runEngineConformance` are green for every shipped backend, adapter and engine, the guard map-membership tests list both patch RPCs, and `make files-viewer-gates` includes the transport test prefixes.
**Files**: `docs/how-to/use-files-search-and-editing.md`, `docs/registry/features/`, `CLAUDE.md`

##### Task 2.5.1a: Run gates, fix findings, regenerate registry (M)
- Files: `docs/registry/features/`

##### Task 2.5.1b: How-to doc and CLAUDE.md reference-index row (S)
- Files: `docs/how-to/use-files-search-and-editing.md`, `CLAUDE.md`

#### Story 2.5.2: Isolated manual verification and owner device check
**As the** owner, **I want** a safe way to try it on my phone, **so that** I can sign off without touching the live service.
**Acceptance Criteria**:
- Isolated instance.
  - *Given* a build at `~/.stapler-squad/manual-builds/manual-1/stapler-squad` on `PORT=62871`, `--remote-port 62872`, `STAPLER_SQUAD_TMUX_SOCKET=ssq-manual-1`, *When* the walkthrough runs, *Then* the live `:8543` service and its tmux sessions are untouched (`ss -ltnp` shows no conflicts).
- Owner checklist.
  - *Given* the phone over LAN/Tailscale, *When* the owner opens, searches, jumps, edits, saves, hits a conflict and rotates the device, *Then* each step is signed off or filed as a bug.
- Spike and session-switch lines on the checklist (pre-mortem #2, #10, #12).
  - *Given* the checklist, *Then* it includes: Story 0.1.5 spike results; "type `--` and `"x"`, save, diff the bytes (no em dash or curly quotes)"; "dirty buffer, switch to another session and back: the Restore prompt offers the held text and a save lands in the original session's checkout"; "flip `files:editing` on, then off, with a dirty buffer: editor goes read-only within one focus/refresh, draft kept"; "touch the file from a terminal, then save: no conflict dialog"; "a 600 KB dirty buffer shows 'Not backed up - save soon'".
- Directory-mode line on the checklist.
  - *Given* a session with no dedicated worktree (editing the main checkout directly, `WorktreeDir == ""`), *When* the owner reviews the checklist, *Then* it has an explicit line: "Edit and save in a directory-mode session; confirm the status-bar chip 'Editing the main checkout' shows, the save lands in the checkout, and decide keep-allowed or veto", signed off or the default is flipped before the global flag is turned on.
- Browser-chord, screen-reader-read and tap lines on the checklist (triad round 2).
  - *Given* the checklist, *Then* it includes: a per-browser chord table (Chrome, Firefox, Safari on the owner's desktop) where the owner presses each chord of ux.md 14.1 inside the Files tab and records delivered/swallowed, with the menu or button route noted for any swallowed chord; the Story 0.1.6 R1-R4 results; "with the soft keyboard open in search, one tap on a result row opens it (U-SEARCH-14)"; "Bluetooth keyboard: typing Enter/Backspace on the soft keyboard never hides the key bar; pressing Esc on the Bluetooth keyboard does; the override wins"; and the Edit-layout check at the three named phone viewports (portrait, landscape, keyboard up and dismissed); on a non-US keyboard layout (for example German), AltGr+8 types `[` in the editor and does not trigger a Files chord, and the overlay shows the layout's key label for Alt+[ (U-CHORD-2).
- Triad-repair lines on the checklist.
  - *Given* the checklist, *Then* it also includes the signed Phone dogfood script (Task 2.5.4a, including the `files:merge-phone` keep/turn-off decision) and the VoiceOver/TalkBack script (Task 2.5.4b); the global `files:editing` flip is blocked until the dogfood script is signed.
- Transport flag flips are separate, ordered owner actions (ADR-036).
  - *Given* the checklist, *Then* it lists the flips in order with a rollback line each: `files:range-read` after Task 1.6.7c; `files:editing` after Tasks 2.5.4a/2.5.4b (as before); `files:delta-write` only after Gate G2 (Task 2.5.4a signed on the Wave 7-8 build) and the T0 steps of Task 2.5.5b; `files:edit-large` only after Gate G3 (the T1 steps of Task 2.5.5b); none of the three is flipped by the `files:editing` sign-off, and the walkthrough records the live `GetFileTransportCapabilities` reply for `:62871` and `:62872` (which wires and ops each advertises, including whether `PATCH` is present on the remote listener per Task 0.1.3b).
**Files**: `project_plans/files-viewer-revamp/implementation/manual-verification-checklist.md`

##### Task 2.5.2a: Build the isolated instance and run the walkthrough with screenshots (S)
- Never `make install-service`.
- Files: `project_plans/files-viewer-revamp/implementation/manual-verification-checklist.md`


#### Story 2.5.3: Files metrics view and derived queries (ADR-035 section 5)
**As the** owner, **I want** one place to see how the Files viewer is used and how it performs against its SLOs, **so that** I can judge adoption and spot regressions over months.
**Acceptance Criteria**:
- Derived metrics endpoint.
  - *Given* an in-memory `analytics.db` seeded with: 10 `files.search.run`, 6 `files.search.open_hit` with `interactions` {2,3,3,3,4,5+}, 20 `files.edit.save` of which 3 `conflict` and 2 `files.edit.overwrite{confirmed}`, 4 `files.edit.merge_open` and 3 `merge_saved`, a desktop/mobile mix via `form`, `files.perf.search_first_result_ms` values {300, 900, 2500}, `files.perf.open_ms` values {120, 200, 450}, and 5 non-`files.` events, *When* `GET /api/analytics/files?from&to` is called, *Then* the JSON reports `find_and_open{within_3:4,total:6,rate}`, `adoption{weeks[{week,saves,searches,edit_enters,active_days}]}`, `save_conflict{saves:20,conflicts:3,rate:0.15,overwrites:2,merge_completion}`, `form_split{search,save,tab_open:{mobile,desktop}}`, `search_latency{p50,p95,share_within_2s,by_scale}`, `open_latency{p50,p95,share_within_300ms,by_size}`, `agent_race{stale_shown,active_days,per_active_day}`, and `review{window_days:28, server_events, active_days, searches, saves, edit_enters, dropped, verdict}` where `verdict` is `inconclusive|keep_v2|editing_proven|editing_tried_not_adopted|retire_or_rework` from the owner-editable constants of ADR-035 section 5 (`adoptionMinEvents`, `adoptionMinActiveDays`, `adoptionKeepActiveDays`, `adoptionKeepSearches`, `adoptionEditSaves`); adoption counts for saves and searches come from the unthrottled server events `files.server.write{ok}` and `files.server.search`, client events are the lower-bound cross-check, `dropped` is the summed `files.meta.dropped` count, and with fewer than `adoptionMinEvents` the verdict is `inconclusive` and no percentage is shown; only `files.*` rows are read (the non-`files.` events change nothing); an empty database returns zeroes and `200`, never `500`; at most the platform's 100k retained rows are scanned.
- Page.
  - *Given* `/analytics/files` (a sibling of `/analytics/escape`, following its route pattern and the `require-page-analytics` rule), *When* it renders against the endpoint with a 7/28/90-day picker, *Then* it shows a review card (verdict, the five thresholds and their current values, the `dropped` count and the caveat that one user's small counts are inconclusive), a card per metric with the SLO line drawn (2 s search, 300 ms open), a mobile vs desktop split, an empty state "No Files-viewer events yet" and a loading/error state with Retry; it is usable at 390px; a link to it is added wherever `/analytics/escape` is linked.
- Raw queries documented and verified.
  - *Given* the how-to section "Files analytics queries", *When* its SQL is run against the seeded fixture in `TestFilesMetricsSQL_ShouldMatchEndpoint`, *Then* each documented query returns the same number as the endpoint field it mirrors; the how-to states the `analytics.db` path (`~/.stapler-squad/<instance>/analytics.db`, ADR-013), the privacy rules and the no-PII guarantee.
- Transport series (ADR-035 "Transport extension", ADR-036; absorbed in Tasks 2.5.3a/2.5.3b).
  - *Given* a seeded fixture with `files.transport.select` (by `transport`), `files.transport.fallback` (by `fallback_reason`), `files.edit.save` (by `payload` and `transport`), `files.edit.patch_rejected` (by `reason`) and server `files.server.patch` (by `outcome`, `tier`), *When* the endpoint is called, *Then* the response carries a `transport` block with the adapter mix, the fallback rate, the share of saves sent as patches versus whole files, the median `payload` bucket per path, the patch-rejection rate by reason and the server patch outcomes; the page renders it, and the documented SQL for each returns the same numbers (`TestFilesMetricsSQL_ShouldMatchEndpoint`). No path, offset or text exists in any of these series.
**Files**: `server/handlers/analytics_files_handler.go`, `server/handlers/analytics_files_handler_test.go`, `server/server.go`, `web-app/src/app/analytics/files/page.tsx`, `web-app/src/components/analytics/FilesAnalyticsPage.tsx`, `web-app/src/components/analytics/__tests__/FilesAnalyticsPage.test.tsx`, `docs/how-to/use-files-search-and-editing.md`, `docs/registry/features/`

##### Task 2.5.3a: `GET /api/analytics/files` derived-metrics handler and Go tests (M)
- (ADR-036 absorbed item: the `transport` block of the Story 2.5.3 AC is part of this task and Task 2.5.3b; first suspect if the Wave 9 overrun checkpoint trips.)
- Reads `AnalyticsEvent` rows with `event_name LIKE 'files.%'` in the window via the ent client (same client the summary handler uses), computes the metrics in Go from the label map; percentiles reuse the summary handler's percentile helper; register in `server/server.go`; `// +api: analytics:files`.
- Files: `server/handlers/analytics_files_handler.go`, `server/handlers/analytics_files_handler_test.go`, `server/server.go`

##### Task 2.5.3b: `/analytics/files` page and component with jest tests (M)
- Files: `web-app/src/app/analytics/files/page.tsx`, `web-app/src/components/analytics/FilesAnalyticsPage.tsx`, `web-app/src/components/analytics/__tests__/FilesAnalyticsPage.test.tsx`

##### Task 2.5.3c: How-to "Files analytics queries", SQL-equals-endpoint test, registry and CLAUDE.md index row (S)
- Extends `docs/how-to/use-files-search-and-editing.md` (Task 2.5.1b owns the file; this task appends its section after 2.5.1b merges); `make registry-generate`; one row in the CLAUDE.md reference index.
- Files: `docs/how-to/use-files-search-and-editing.md`, `server/handlers/analytics_files_handler_test.go`, `docs/registry/features/`, `CLAUDE.md`

#### Story 2.5.4: Owner dogfood and screen-reader scripts (before editing ships; triad repair)
**As the** owner, **I want** scripted walkthroughs for the riskiest phone and assistive-technology paths, **so that** I verify them deliberately before turning editing on instead of discovering them in daily use.
**Acceptance Criteria**:
- Phone dogfood script (must be signed before the global `files:editing` flip).
  - *Given* the manual checklist, *When* the owner follows the "Phone dogfood" section on the isolated instance, *Then* each numbered step has an expected result and a pass/fail box covering: open from search, edit with the soft keyboard, dismiss the keyboard and Save from the top bar, a Bluetooth keyboard attached mid-edit (key bar hides; override works), vim opt-in, an agent edit during editing (stale banner within 10 s, no actor claimed), Save into a conflict, Merge on the phone (resolve, review, save), Reload and Overwrite paths, going offline mid-edit, tab-cap eviction with Undo, rotate to landscape, and a draft restore; plus the triad round 3 lines: Android system Back with the soft keyboard up (first Back dismisses the keyboard without leaving the pane, second pops it; U-MOB-4), a 600 KB dirty buffer switched to the background for a minute and the page killed or reloaded, then reopened (the IndexedDB backup is offered, or the priority-1 "Draft not stored" banner was shown; U-DRAFT-6), Close others with three dirty tabs and Save all (one conflicting), and the armed Esc, Esc with a Bluetooth keyboard (vim on: chip, leave, Esc back); it ends with an explicit keep/turn-off decision for `files:merge-phone`.
- Screen-reader script (U-A11Y-7).
  - *Given* iOS VoiceOver, Android TalkBack and a desktop screen reader, *When* the owner follows the script, *Then* it re-runs the Story 0.1.6 pass criteria R1-R4 (Accessible view required, CM6 Read and Edit informational) and covers open file, reading code in Read mode (via the Accessible view and CM6), entering Edit mode and typing (CodeMirror's touch content element and IME), saving, the conflict dialog, the merge view, the tab sheet and search results, with the vim mode indicator verified not to announce each mode change; failures are filed as bugs and the script result is recorded in the checklist.
**Files**: `project_plans/files-viewer-revamp/implementation/manual-verification-checklist.md`

##### Task 2.5.4a: Write the phone dogfood script and the sign-off gate (S)
- The script is written in Wave 9 and the owner runs it on a build containing the Wave 7 and Wave 8 merges (Epics 2.2, 2.3, 2.4), before Task 2.5.2a's flag-flip recommendation; includes the `files:merge-phone` decision line.
- Files: `project_plans/files-viewer-revamp/implementation/manual-verification-checklist.md`

##### Task 2.5.4b: Write the VoiceOver/TalkBack manual script for CodeMirror on touch (S)
- Not a from-scratch script any more: it re-runs the Story 0.1.6 criteria R1-R4 against the built Accessible view (Story 1.5.4) and CM6, each numbered step with an expected result and pass/fail box, plus the open file, Edit entry and typing, save, conflict dialog, merge view, tab sheet and search-results steps; the Accessible view must pass R1/R2 before the global `files:editing` flip, CM6 Edit with a screen reader is recorded as supported / best-effort / unsupported in the how-to.
- Files: `project_plans/files-viewer-revamp/implementation/manual-verification-checklist.md`

#### Story 2.5.5: Delta rollout: e2e matrix, registry/docs for the new RPCs and the large-edit dogfood (ADR-036 Phase C close)
**As the** owner, **I want** the delta path proven end to end against the real server and signed off on my phone before `files:delta-write` and `files:edit-large` are switched on, **so that** each flag is flipped on evidence, one at a time.
**Acceptance Criteria**:
- E2E over `full` and `connect_unary`, desktop and the gating mobile project, with 5 MB and 50 MB fixtures.
  - *Given* the isolated e2e server with `files:editing`, `files:range-read` and `files:delta-write` on, *When* the spec edits one character in a 5 MB file and saves, *Then* the network request body is under 4 KiB (asserted from the recorded request; the same edit on the `full` adapter sends the whole file), the file on disk equals the expected bytes, the token changes and the tab goes clean; *When* an agent-style direct-disk write intervenes before the save, *Then* the conflict flow of Story 2.3.3 appears and nothing is written; *When* `files:delta-write` is turned off mid-session, *Then* the next save uses `writeFull` for a T0 file and a T1 buffer goes read-only with its draft kept.
  - *Given* `files:edit-large` on and a 50 MB file, *When* the spec edits in two regions separated by thousands of lines and saves, *Then* one patch is sent, the disk bytes equal the expected bytes with CRLF/BOM preserved, and Reload after a conflict keeps the pending edits as a draft; with `files:edit-large` off, the same file is view-only with the reason chip.
- Registry, docs and gates for the new RPCs.
  - *Given* `GetFileTransportCapabilities`, `ReadFileRange`, `ApplyFilePatch` and `PreflightFilePatch`, *When* `make registry-generate` runs, *Then* the feature-registry files are current; the how-to `docs/how-to/use-files-search-and-editing.md` gains the sections "Large files", "How saves are sent" and "Transport fallback and kill switches" (what each flag does, the flip order, how to read the fallback chip), and `make files-viewer-gates` includes the new test-name prefixes in `files-viewer-go-tests`.
- Owner large-file edit dogfood (wall-clock blocker; flips `files:delta-write` and `files:edit-large`, in that order, on separate sittings).
  - *Given* the checklist section "Large-file editing", *When* the owner follows it on the isolated instance, *Then* each step records pass/fail: edit one character in a 5 MB file on the phone link and save; edit and save in a 50 MB file; save into a conflict; the same-position rebase succeeds when only a distant line changed and fails closed when lines shifted; the kill switch routes saves to the whole-file path; offline with a ChangeSet draft restores; `files:delta-write` may be flipped only after Gate G2 (Task 2.5.4a signed: full-content editing proven on the Wave 7-8 build) and this script's T0 steps pass, and `files:edit-large` only after its T1 steps also pass (Gate G3).
**Files**: `tests/e2e/files-large-edit.spec.ts`, `tests/e2e/pages/files-tab.page.ts`, `docs/how-to/use-files-search-and-editing.md`, `Makefile`, `project_plans/files-viewer-revamp/implementation/manual-verification-checklist.md`

##### Task 2.5.5a: Delta e2e matrix with direct-disk assertions (M)
- `files-large-edit.spec.ts` (`// @feature files:editing, files:delta-write`), page-object additions (after Task 2.4.3a), the recorded-request payload assertion, kill-switch and conflict scenarios, helpers reused from Task 1.6.7a.
- Files: `tests/e2e/files-large-edit.spec.ts`, `tests/e2e/pages/files-tab.page.ts`, `tests/e2e/helpers/largeFile.ts`

##### Task 2.5.5b: Large-file editing dogfood script and flag-flip gates (S)
- Writes the checklist section, the flip order (`files:range-read` after Task 1.6.7c; `files:delta-write` after Gate G2 and the T0 steps; `files:edit-large` after Gate G3), and the rollback line for each flag; ordered after Task 2.5.4b and before Task 2.5.2a in the checklist.
- Files: `project_plans/files-viewer-revamp/implementation/manual-verification-checklist.md`

##### Task 2.5.5c: Registry, how-to and gate-target updates for the transport RPCs (S)
- Regenerate/commit registry files, extend the how-to (after Tasks 2.5.1b and 2.5.3c, which write the same file), extend `files-viewer-go-tests` prefixes and the CLAUDE.md index row text if the how-to title changes.
- Files: `docs/how-to/use-files-search-and-editing.md`, `docs/registry/features/`, `Makefile`


## Epic 2.6: Optional transport adapters, Phase D (ADR-036; owner-selected, sequenced last, gated on the spike results)
**Goal**: Add `connect_stream`, `ws_stream` (reads) and the byte-only `http_range` GET adapter behind the same `FileTransport` port, each passing the same conformance suites, and handle HTTP/1.1 connection-pool contention. **Gate G-D**: the S-T1 and S-T5 outcomes and the Task 1.6.7b benchmarks must be recorded in ADR-036 before this epic starts; they decide the default ladder, not whether the code exists: if S-T5 meets its decision rule (p95 of the save-shaped call over 2 s under the planned caps) the stream adapters are advertised by default, otherwise they ship but the server advertises them off (a capability, flippable without a deploy) and the unary ladder stays the default. `http_range` never ships without its rebinding guard (Task 2.6.3a) and never becomes the default for line windows. No adapter weakens ADR-031: all policy stays in `FileOps`.

#### Story 2.6.1: Server stream wires: `ReadFileRangeStream` and the WebSocket bridge entry
**As a** client on a long scroll, **I want** a sequence of line-aligned chunks from one request, **so that** read-ahead needs one round trip where prefetch misses its target.
**Acceptance Criteria**:
- Same policy, same bytes.
  - *Given* `ReadFileRangeStream{lines, anchor, want_lines=4000}`, *When* the handler streams, *Then* it emits contiguous line-aligned chunks of at most 128 KiB each with the same fields, CRC32C and validator as the unary call, stops at `eof`, honors ctx cancellation within 200 ms (`goleak` clean), and every refusal (flag off, hidden session, symlink escape, `.git`) is identical to the unary path because it calls the same `FileOps.ReadRange`.
- Limits.
  - *Given* the shared limiter, *Then* an open range stream counts toward the per-IP stream cap of ADR-030 decision 6 (not only the range cap), a queued stream times out at 20 s with `ResourceExhausted`, and abandoned streams are aborted.
- WebSocket exposure (reads only).
  - *Given* `ReadFileRangeStream` registered through the exact-path `StreamingWSBridge` entry and the transport-selector entry, *When* `TestEveryServerStreamingWatchRPCHasAWebSocketBridge` runs, *Then* it passes with the new RPC listed; permessage-deflate is used if the S-T1 outcome found the library supports it.
**Files**: `proto/session/v1/session.proto`, `server/services/file_ops_stream.go`, `server/services/file_ops_stream_test.go`, `server/server.go`, `web-app/src/lib/transport/watch-ws-transport.ts`

##### Task 2.6.1a: `ReadFileRangeStream` RPC and handler (M)
- Proto (ordered after Task 2.1.5a), `FileOps.ReadRangeStream` over `ReadRange`, handler in `files_delegates.go`, ctx-aware flush per chunk, limiter integration, conformance cases (`READ_STREAM` capability honesty).
- Files: `proto/session/v1/session.proto`, `server/services/file_ops_stream.go`, `server/services/file_ops_stream_test.go`, `server/services/files_delegates.go`

##### Task 2.6.1b: WebSocket bridge registration and selector entry (S)
- Exact-path registration beside the other bridge entries, the `watch-ws-transport.ts` selector entry, the extended registration test; `server/server.go` is edited after Task 2.5.3a (Wave 9, the last earlier editor), so the file order is by wave.
- Files: `server/server.go`, `web-app/src/lib/transport/watch-ws-transport.ts`, `server/watch_ws_bridge_registration_test.go`

#### Story 2.6.2: `ConnectStreamTransport`, `WsStreamTransport` and pool-contention handling
**As a** user on a phone with several tabs, **I want** read-ahead that cannot starve my saves of connections, **so that** the stream adapters help rather than hurt.
**Acceptance Criteria**:
- Adapters pass the same suites.
  - *Given* `runTransportConformance`, *When* run against `connect_stream` and `ws_stream` (read side), *Then* both pass (capability honesty, range edges, staleness, typed errors, abort, demotion); a malformed envelope is a transport-class failure.
- Ladder.
  - *Given* a server advertising all wires, *Then* `read` resolves `ws_stream` > `connect_stream` > `connect_unary` > `full`, `patch`/`stat`/`writeFull` stay unary, and three consecutive transport-class failures demote one rung with the quiet chip (Story 1.2.3).
- HTTP/1.1 pool contention (S-T5).
  - *Given* a tab holding a range stream, a `SearchContent` stream and a prefetching second tab, *When* the user presses Save, *Then* the save's unary call is not queued behind more than one other request (a `PoolBudget` caps long-lived streams at 2 per tab, aborts a range stream when the document is hidden for 30 s or when a search starts and the budget is exhausted, and falls back to unary prefetch), measured by the Task 0.1.8b spec re-run with the stream adapters on; `ws_stream` does not count against the pool.
- Analytics (ADR-035, ADR-036).
  - *Given* the stream adapters selected or demoted, *Then* `files.transport.select` and `files.transport.fallback` carry `transport` values `connect_stream`/`ws_stream` and the closed `fallback_reason` enum, once per change; no path, offset or line.
**Files**: `web-app/src/lib/files/transport/connectStreamTransport.ts`, `web-app/src/lib/files/transport/wsStreamTransport.ts`, `web-app/src/lib/files/transport/poolBudget.ts`, `web-app/src/lib/files/transport/router.ts`, `tests/e2e/files-transport-spike.spec.ts`

##### Task 2.6.2a: `ConnectStreamTransport` (M)
- `readStream` over the Connect server-stream, chunk stitching into the cache, backpressure, abort, malformed-envelope handling; tests with a fake streaming transport.
- Files: `web-app/src/lib/files/transport/connectStreamTransport.ts`, `web-app/src/lib/files/transport/__tests__/connectStreamTransport.test.ts`

##### Task 2.6.2b: `WsStreamTransport` over `watch-ws-transport` (M)
- Reuses `fromWebSocket`/the Connect-envelope codec of `watch-ws-transport.ts`; reads only; reconnect and auth behavior through the existing bridge; a probe that downgrades to `connect_stream` when the bridge path fails (the Story 0.1.1-style probe); tests with a fake WebSocket.
- Files: `web-app/src/lib/files/transport/wsStreamTransport.ts`, `web-app/src/lib/transport/watch-ws-transport.ts`, `web-app/src/lib/files/transport/__tests__/wsStreamTransport.test.ts`

##### Task 2.6.2c: Stream conformance, ladder tests and `PoolBudget` (M)
- Conformance runs for both adapters, ladder/demotion tests, `poolBudget.ts` and its router integration (edit after Task 1.2.3a), and the Playwright re-run of the S-T5 spec with the adapters enabled.
- Files: `web-app/src/lib/files/transport/poolBudget.ts`, `web-app/src/lib/files/transport/router.ts`, `web-app/src/lib/files/transport/__tests__/streamConformance.test.ts`, `tests/e2e/files-transport-spike.spec.ts`

#### Story 2.6.3: `http_range` GET adapter behind the DNS-rebinding guard, and the `ServeFileRaw` audit
**As the** owner, **I want** raw byte ranges (media, binary preview) served cacheably without opening a new hole on the unauthenticated local listener, **so that** a GET that returns file bytes passes the same Host/Origin verdict as the write RPCs.
**Acceptance Criteria**:
- Guarded GET, byte-only.
  - *Given* the `http_range` route (a GET returning `206` with `Content-Range`, ETag equal to the version token, `Accept-Ranges: bytes`), *When* a request has a foreign `Origin` or a rebinding `Host` on the `:8543` listener, *Then* it is rejected by the same `ProfileRebinding` verdict before any byte is read; a same-origin request with a valid `Host` is served through `FileOps` (confinement, flags, hidden-session and `.git`/symlink rules identical to every other wire), `If-Range`/`If-None-Match` follow the token, and no line numbers, LF normalization or BOM stripping happen (raw bytes only).
- `ServeFileRaw` gets the same check.
  - *Given* the existing `ServeFileRaw` route (VERIFIED to stream media with `http.ServeContent`), *When* the same foreign-Origin and rebinding-Host GETs are sent, *Then* they are rejected, and the pre-existing same-origin media behavior (PDF/video range playback) is unchanged (the existing tests stay green); the audit result (what the route did before) is recorded in ADR-036 "Spike outcomes".
- Selection rule.
  - *Given* a media/binary preview or the raw/hex view, *Then* the router picks `http_range`; *Given* a line window request, *Then* it is never chosen; the conformance suite runs its byte-only subset (capability honesty, range edges, `If-Range` staleness, refusal mapping).
- Analytics.
  - *Given* a byte-range preview, *Then* `files.transport.select{op=read, transport=http_range}` is emitted once; no path or offset.
**Files**: `server/services/file_http_range.go`, `server/services/file_http_range_test.go`, `server/services/file_service.go`, `server/server.go`, `web-app/src/lib/files/transport/httpRangeTransport.ts`

##### Task 2.6.3a: `http_range` GET route through `FileOps` with the rebinding guard (M)
- Route handler (thin) in `file_http_range.go`, registration with `middleware.ProfileRebinding` for GET (the GET guard path from Task 0.1.3b's finding) on the local chain and the auth tier on the remote chain, `ServeFileRaw` gains the same guard (edit after Task 0.2.1c/0.2.1e/0.2.3c in `file_service.go`), ETag/`If-Range` from the token, capability `HTTP_RANGE` advertised per listener.
- Files: `server/services/file_http_range.go`, `server/services/file_http_range_test.go`, `server/services/file_service.go`, `server/server.go`

##### Task 2.6.3b: `HttpRangeTransport` (byte-only) and the selection rule (S)
- Byte-only adapter (`read` kind `bytes`, no lines, no patch), router selection for media/binary preview, conformance byte-only subset.
- Files: `web-app/src/lib/files/transport/httpRangeTransport.ts`, `web-app/src/lib/files/transport/__tests__/httpRangeTransport.test.ts`

##### Task 2.6.3c: Guard tests over the real chains, registry and docs (S)
- Foreign-Origin and rebinding-Host GET tests for both routes over the real local chain, remote-chain auth behavior, feature-registry file, how-to section (after Task 2.5.5c), ADR-036 audit note.
- Files: `server/services/file_http_range_test.go`, `docs/registry/features/files-http-range.json`, `docs/how-to/use-files-search-and-editing.md`


---

# Contingent (excluded from the base totals): custom engine, built only if both CodeMirror 6 lazy models fail

*Activation rule (owner decision 2026-10-16, ADR-036 decision 20): this epic is built only if Task 0.1.4d records FAIL for the gap-line model (a2) **and** the window-swap model (a1) on the owner's phone. Until then it is not started, its tasks are not in the 196-task base count and its cost is reported separately in the Effort estimate ("Contingent engine"). If activated, it runs as an additional `EditorEngine` for T1/T2 only (CM6 stays the T0 editor), replacing Story 1.6.4 in the wave plan, and ships behind the existing `files:range-read`/`files:edit-large` flags with `features.vim=false` and `features.mergeView=false` until reimplemented. It is an alternative to the Epic 1.6 CM6 work, so activating it removes Story 1.6.4's tasks from the base (not modeled as a negative: the base total assumes CM6 passes).*

## Contingent Epic 1.7: Custom rope/piece-table editor engine with a virtualized renderer
**Goal**: A native-lazy document (chunks plus a piece table of original-versus-added buffers), a virtualized line renderer, a text-input layer with IME/soft-keyboard support, an accessibility tree for the viewport and standalone Lezer highlighting, behind `EditorEngine`, passing `runEngineConformance`.

#### Story 1.7.1: Piece-table `VirtualDoc` model
**As a** developer, **I want** a lazy document model whose edits map directly to `Edit{line,col}`, **so that** the delta wire falls out of the model without a ChangeSet conversion.
**Acceptance Criteria**:
- *Given* a `VirtualDoc` of 1,000,000 lines with two loaded chunks, *When* 1,000 seeded random edits are applied to the loaded regions, *Then* `text(range)` equals a reference string model after every edit, line/column to offset conversion is exact for astral and combining text, and edits touching an unloaded gap are refused.
- *Given* `takePending()`, *Then* the net edits since the last ack are returned in absolute base line/col_utf16 and equal the full-replace oracle on 10,000 seeded scripts.
**Files**: `web-app/src/components/sessions/files/engine/custom/pieceTable.ts`, `web-app/src/components/sessions/files/engine/custom/lineIndex.ts`, `web-app/src/components/sessions/files/engine/custom/pending.ts`, `web-app/src/components/sessions/files/engine/custom/__tests__/pieceTable.test.ts`

##### Task 1.7.1a: Piece table with chunk-backed original buffer (M)
- Files: `web-app/src/components/sessions/files/engine/custom/pieceTable.ts`, `web-app/src/components/sessions/files/engine/custom/__tests__/pieceTable.test.ts`

##### Task 1.7.1b: Line index and absolute coordinate mapping across gaps (M)
- Files: `web-app/src/components/sessions/files/engine/custom/lineIndex.ts`, `web-app/src/components/sessions/files/engine/custom/__tests__/lineIndex.test.ts`

##### Task 1.7.1c: Pending edits as `Edit[]` in base coordinates (M)
- Files: `web-app/src/components/sessions/files/engine/custom/pending.ts`, `web-app/src/components/sessions/files/engine/custom/__tests__/pending.test.ts`

##### Task 1.7.1d: Fuzz against a reference string model (S)
- Files: `web-app/src/components/sessions/files/engine/custom/__tests__/pieceTable.fuzz.test.ts`

#### Story 1.7.2: Virtualized line renderer
**As a** user, **I want** smooth scrolling and correct selection rendering over a huge document, **so that** the custom engine feels like an editor.
**Acceptance Criteria**:
- *Given* a 1,000,000-line virtual document, *When* the viewport scrolls, *Then* only visible rows plus an overscan are in the DOM (at most 200 nodes), scroll position is stable across row-height measurement changes (anchor line preserved), and wrapped long lines measure correctly.
- *Given* a selection across two chunks and a gap, *Then* caret and selection rects render without gaps in the visible rows and the gutter shows absolute line numbers.
**Files**: `web-app/src/components/sessions/files/engine/custom/renderer.ts`, `web-app/src/components/sessions/files/engine/custom/selectionLayer.ts`, `web-app/src/components/sessions/files/engine/custom/gutter.ts`, `web-app/src/components/sessions/files/engine/custom/__tests__/renderer.test.tsx`

##### Task 1.7.2a: Row virtualization with measured heights and scroll anchoring (M)
- Files: `web-app/src/components/sessions/files/engine/custom/renderer.ts`, `web-app/src/components/sessions/files/engine/custom/__tests__/renderer.test.tsx`

##### Task 1.7.2b: Caret and selection layer with wrapping (M)
- Files: `web-app/src/components/sessions/files/engine/custom/selectionLayer.ts`, `web-app/src/components/sessions/files/engine/custom/__tests__/selectionLayer.test.tsx`

##### Task 1.7.2c: Gutter, decoration layers and highlight overlay (M)
- Files: `web-app/src/components/sessions/files/engine/custom/gutter.ts`, `web-app/src/components/sessions/files/engine/custom/decorations.ts`, `web-app/src/components/sessions/files/engine/custom/__tests__/gutter.test.tsx`

##### Task 1.7.2d: Bidirectional text, tabs and very long lines (S)
- Files: `web-app/src/components/sessions/files/engine/custom/renderer.ts`, `web-app/src/components/sessions/files/engine/custom/__tests__/renderer.longlines.test.tsx`

#### Story 1.7.3: Text-input layer for touch and keyboard
**As a** phone user, **I want** composition, autocorrect-safe input and selection handles to work, **so that** typing is not the weak spot of a custom editor.
**Acceptance Criteria**:
- *Given* a hidden input proxy, *When* an IME composition runs (`keyCode 229` sequences recorded in Task 0.1.5c), *Then* no document dispatch happens until `compositionend`, the result equals the composed text, and `autocorrect="off"`, `autocapitalize="off"`, `spellcheck="false"` are set.
- *Given* the soft keyboard opening, *Then* the caret stays visible (the `visualViewport` rules of Story 1.5.1 apply) and long-press selection with handles, tap-to-place and double-tap word selection work in the gating mobile project.
**Files**: `web-app/src/components/sessions/files/engine/custom/inputProxy.ts`, `web-app/src/components/sessions/files/engine/custom/clipboard.ts`, `web-app/src/components/sessions/files/engine/custom/touch.ts`, `web-app/src/components/sessions/files/engine/custom/__tests__/inputProxy.test.tsx`

##### Task 1.7.3a: Input proxy and IME composition (M)
- Files: `web-app/src/components/sessions/files/engine/custom/inputProxy.ts`, `web-app/src/components/sessions/files/engine/custom/__tests__/inputProxy.test.tsx`

##### Task 1.7.3b: Soft keyboard and viewport handling (M)
- Files: `web-app/src/components/sessions/files/engine/custom/inputProxy.ts`, `web-app/src/components/sessions/files/engine/custom/viewport.ts`, `web-app/src/components/sessions/files/engine/custom/__tests__/viewport.test.tsx`

##### Task 1.7.3c: Clipboard, drag-drop and keybinding map (M)
- Files: `web-app/src/components/sessions/files/engine/custom/clipboard.ts`, `web-app/src/components/sessions/files/engine/custom/keymap.ts`, `web-app/src/components/sessions/files/engine/custom/__tests__/clipboard.test.tsx`

##### Task 1.7.3d: Touch gestures, selection handles and inertia scrolling (M)
- Files: `web-app/src/components/sessions/files/engine/custom/touch.ts`, `web-app/src/components/sessions/files/engine/custom/__tests__/touch.test.tsx`

##### Task 1.7.3e: Input-event capture fixtures replayed against the proxy (S)
- Files: `web-app/src/components/sessions/files/engine/custom/__tests__/inputFixtures.test.tsx`

#### Story 1.7.4: Accessibility tree and standalone Lezer highlighting
**As a** screen-reader user, **I want** the visible rows exposed with names and announcements, **so that** the custom engine is not less accessible than CodeMirror.
**Acceptance Criteria**:
- *Given* the viewport rows, *Then* they expose roles, line numbers and the selection to the accessibility tree (axe clean), edits and loads are announced politely, and the Accessible view (Story 1.5.4) remains the guaranteed read path.
- *Given* the `Highlighter` port implemented with standalone Lezer parsers over visible ranges, *Then* spans equal the CM6 highlighter's token classes for the supported languages on a 5,000-line fixture and highlighting is off beyond the loaded-text rule.
**Files**: `web-app/src/components/sessions/files/engine/custom/a11y.ts`, `web-app/src/components/sessions/files/engine/custom/lezerHighlighter.ts`, `web-app/src/components/sessions/files/engine/custom/find.ts`, `web-app/src/components/sessions/files/engine/custom/__tests__/a11y.test.tsx`

##### Task 1.7.4a: Viewport accessibility tree and announcements (M)
- Files: `web-app/src/components/sessions/files/engine/custom/a11y.ts`, `web-app/src/components/sessions/files/engine/custom/__tests__/a11y.test.tsx`

##### Task 1.7.4b: Standalone Lezer `Highlighter` over visible ranges (M)
- Files: `web-app/src/components/sessions/files/engine/custom/lezerHighlighter.ts`, `web-app/src/components/sessions/files/engine/custom/__tests__/lezerHighlighter.test.ts`

##### Task 1.7.4c: Find in loaded text and go-to (M)
- Files: `web-app/src/components/sessions/files/engine/custom/find.ts`, `web-app/src/components/sessions/files/engine/custom/__tests__/find.test.tsx`

##### Task 1.7.4d: Screen-reader criteria R1-R4 run against the engine (S)
- Files: `web-app/src/components/sessions/files/engine/custom/__tests__/a11yCriteria.test.tsx`

#### Story 1.7.5: History, `CustomEngine` adapter, conformance and capability flags
**As the** owner, **I want** the engine to pass the same conformance suite and to say honestly what it cannot do, **so that** the UI never offers Vim keys or a merge view that do not exist.
**Acceptance Criteria**:
- *Given* `runEngineConformance(makeCustomEngine)`, *Then* every case passes including the lazy cases and edits across gaps; `features` reports `vim=false`, `mergeView=false`, `lazyDoc=true`.
- *Given* the UI with the custom engine active, *Then* Vim toggles show "Vim keys unavailable for this engine" and the conflict flow for T1 uses Reload/same-position rebase only (already the T1 rule).
**Files**: `web-app/src/components/sessions/files/engine/CustomEngine.ts`, `web-app/src/components/sessions/files/engine/custom/history.ts`, `web-app/src/components/sessions/files/engine/custom/__tests__/CustomEngine.conformance.test.tsx`, `web-app/src/components/sessions/files/StatusBar.tsx`

##### Task 1.7.5a: Undo/redo history over the piece table (M)
- Files: `web-app/src/components/sessions/files/engine/custom/history.ts`, `web-app/src/components/sessions/files/engine/custom/__tests__/history.test.ts`

##### Task 1.7.5b: `CustomEngine` adapter implementing `EditorEngine` (M)
- Files: `web-app/src/components/sessions/files/engine/CustomEngine.ts`, `web-app/src/components/sessions/files/engine/custom/index.ts`

##### Task 1.7.5c: Full `runEngineConformance` run and edit-mapping oracle (M)
- Files: `web-app/src/components/sessions/files/engine/custom/__tests__/CustomEngine.conformance.test.tsx`, `web-app/src/components/sessions/files/engine/__tests__/engineConformance.ts`

##### Task 1.7.5d: Capability-gated UI for vim and merge (M)
- Files: `web-app/src/components/sessions/files/StatusBar.tsx`, `web-app/src/components/sessions/files/extensions/vim.ts`, `web-app/src/components/sessions/__tests__/engineCapabilities.test.tsx`

##### Task 1.7.5e: Keyboard shortcut parity table (S)
- Files: `web-app/src/components/sessions/files/engine/custom/keymap.ts`, `web-app/src/components/sessions/files/engine/custom/__tests__/keymap.test.ts`

#### Story 1.7.6: Integration, performance and the phone gate
**As the** owner, **I want** the custom engine selected only for T1/T2 and checked on my phone, **so that** its risk never touches the T0 editor.
**Acceptance Criteria**:
- *Given* `files:range-read` on and a T1/T2 file, *When* the router selects the engine, *Then* the custom engine mounts and T0 files still use `CmEngine`; a 50 MB file stays within the phone memory budget and scroll jank stays within the Story 1.6.7a thresholds.
- *Given* the owner's phone, *When* the owner types, selects, pastes, scrolls fast and uses the soft keyboard, *Then* pass/fail is recorded before the engine is enabled.
**Files**: `web-app/src/components/sessions/files/engine/engineFactory.ts`, `web-app/src/lib/files/windowedSource.ts`, `tests/e2e/files-custom-engine.spec.ts`, `project_plans/files-viewer-revamp/implementation/manual-verification-checklist.md`

##### Task 1.7.6a: Engine selection by tier (M)
- Files: `web-app/src/components/sessions/files/engine/engineFactory.ts`, `web-app/src/lib/files/windowedSource.ts`, `web-app/src/components/sessions/files/engine/__tests__/engineFactory.test.ts`

##### Task 1.7.6b: Phone-budget performance and memory tests (M)
- Files: `web-app/src/components/sessions/files/engine/custom/__tests__/perf.test.tsx`, `web-app/src/components/sessions/files/engine/custom/renderer.ts`

##### Task 1.7.6c: Playwright matrix for the custom engine (S)
- Files: `tests/e2e/files-custom-engine.spec.ts`, `tests/e2e/pages/files-tab.page.ts`

##### Task 1.7.6d: Owner phone gate for the custom engine (XS; wall-clock blocker)
- Files: `project_plans/files-viewer-revamp/implementation/manual-verification-checklist.md`


---

## Effort estimate

*(Bands and weights INFERRED per `ESTIMATION.md`; recalibrate after the first implementer run. Task token midpoints: XS 20k, S 55k, M 130k. Assumed mix: output ~12% of tokens, ~70% of input served from cache, so about 0.93 CU per raw token. Story CU includes the x1.75 verification multiplier.)*

| Story | Tasks | Classes | Raw tokens | Output share | CU (x1.75 verify) | Wall-clock blocker |
|---|---|---|---|---|---|---|
| 0.1.1 `SearchContent` streaming on its real production path (+ phone/remote check) | 2 | S,S | 110k | ~12% | 179k | CI run |
| 0.1.2 50k-file search benchmark harness | 2 | S,S | 110k | ~12% | 179k | CI run |
| 0.1.3 Platform recon for the write path | 2 | S,S | 110k | ~12% | 179k | CI run |
| 0.1.4 CodeMirror state-swap spike (+ WebKit CI check; + extended S-T3 gap-line/window-swap prototypes 0.1.4c and the owner phone gate 0.1.4d, ADR-036) | 4 | S,S,M,XS | 260k | ~12% | 423k | CI run; owner S-T3 phone gate (tstapler, same sitting as 0.1.5b) |
| 0.1.5 Real-device input and viewport spike (+ key-event capture panel and detector fixtures, Task 0.1.5c; + owner time-to-answer baseline, Task 0.1.5d) | 4 | S,S,XS,XS | 150k | ~12% | 244k | Owner phone check and time-to-answer baseline (tstapler) |
| 0.1.6 Screen-reader read path on CM6: Accessible-view prototype, accessibility-tree test, owner VoiceOver/TalkBack check (R1-R4) | 2 | S,XS | 75k | ~12% | 122k | Owner VoiceOver/TalkBack check (tstapler), same sitting as 0.1.5b |
| 0.1.7 Transport measurement spikes S-T1 (compression, wire cost) and S-T2 (index/hash/splice throughput) (ADR-036) | 2 | S,S | 110k | ~12% | 179k | CI run; benchmark runs on the real host |
| 0.1.8 Delta encoder and connection-pool contention spikes S-T4, S-T5 (ADR-036) | 2 | S,S | 110k | ~12% | 179k | CI run |
| 0.2.1 `resolveInWorktree` and read-RPC retrofit (separate commit; 0.2.1c split into 0.2.1c + 0.2.1e) | 5 | M,S,S,M,S | 425k | ~12% | 692k | CI run |
| 0.2.2 Feature flags | 2 | S,S | 110k | ~12% | 179k | CI run |
| 0.2.3 Read-side codec, version token, StatFile | 3 | S,M,M | 315k | ~12% | 513k | CI run |
| 0.2.4 Per-wave gate checkpoints (+ race-suite budget, conditional package move, test harness and jscpd ledger, conditional `server/server.go` carve-out 0.2.4f) | 6 | S,XS,S,S,S,S | 295k | ~12% | 480k | CI run |
| 0.2.5 Files analytics taxonomy, typed helper, privacy guard (ADR-035; + transport labels and events, Task 0.2.5d) | 4 | M,S,M,S | 370k | ~12% | 602k | CI run |
| 0.2.6 `FileOps`/`FileBackend` ports, `GetFileTransportCapabilities`, transport flags (ADR-036 Phase A) | 4 | S,M,M,S | 370k | ~12% | 602k | CI run |
| 1.1.1 Proto, `ContentSearcher`, in-process walker (+ scanner split from the walker) | 6 | S,S,M,S,M,M | 555k | ~12% | 903k | CI run |
| 1.1.2 `SearchContent` handler, delegation, observability (+ open-stream limiter; + conditional WS-bridge registration 1.1.2d) | 4 | M,S,S,S | 295k | ~12% | 480k | CI run |
| 1.1.3 Benchmark gate | 1 | M | 130k | ~12% | 212k | CI run |
| 1.2.1 Extract `CodeEditor` from `FileContentViewer` (+ seams, latency SLO, `extensions/*` stubs split out; + `EditorEngine`/`Highlighter`/`MergeSurface` ports, `CmEngine` and engine conformance, Tasks 1.2.1f/1.2.1g, ADR-036) | 7 | S,M,M,S,S,M,S | 610k | ~12% | 993k | CI run |
| 1.2.2 `useOpenBuffers` hook and thin `FilesTab` (+ hook seams) | 4 | M,S,M,S | 370k | ~12% | 602k | CI run |
| 1.2.3 `FileTransport` port, router and `FullFileTransport` (ADR-036 Phase A) | 3 | M,S,M | 315k | ~12% | 513k | CI run |
| 1.3.1 Tab strip | 2 | M,S | 185k | ~12% | 301k | CI run |
| 1.3.2 Jumplist, go-to-line, deep links | 3 | S,S,S | 165k | ~12% | 269k | CI run |
| 1.3.3 Outline / symbols | 2 | S,S | 110k | ~12% | 179k | CI run |
| 1.3.4 Staleness banner for clean buffers (+ 10 s active-buffer poll, debounce, actor wording) | 2 | M,S | 185k | ~12% | 301k | CI run |
| 1.3.5 Status bar, command palette, shortcut registration (+ reserved-chord denylist and per-browser spec, Task 1.3.5c) | 3 | S,M,S | 240k | ~12% | 391k | CI run; owner per-browser chord check at 2.5.2 |
| 1.3.6 Keyboard-operable resizable panes | 2 | S,S | 110k | ~12% | 179k | CI run |
| 1.3.7 Banner stack, loading states, success feedback | 2 | M,M | 260k | ~12% | 423k | CI run |
| 1.4.1 Streaming search client | 2 | M,S | 185k | ~12% | 301k | CI run |
| 1.4.2 Search panel and results (+ `tree` ARIA, idle state, first-tap activation, Task 1.4.2d) | 4 | M,M,S,M | 445k | ~12% | 724k | CI run |
| 1.4.3 Desktop search + tabs e2e | 1 | M | 130k | ~12% | 212k | CI run |
| 1.5.1 Three-pane mobile layout (+ `chooseFilesLayout`, tablet toolbar buttons; 1.5.1c re-classed S to M) | 3 | M,M,M | 390k | ~12% | 635k | CI run |
| 1.5.2 Mobile read mode and large-file degradation | 2 | S,M | 185k | ~12% | 301k | CI run |
| 1.5.3 Mobile e2e and a11y | 3 | S,XS,M | 205k | ~12% | 334k | Owner phone smoke, Gate G1 (tstapler); CI run |
| 1.5.4 Accessible view: non-virtual paged read path for assistive technology (1.5.4b re-classed S to M; Edit hand-off moved to 2.2.1b) | 2 | M,M | 260k | ~12% | 423k | CI run; owner screen-reader check at Gate G1 and 2.5.4b |
| 1.6.1 Server sparse line index and background `Analyze` (ADR-036 Phase B) | 2 | M,M | 260k | ~12% | 423k | CI run |
| 1.6.2 `ReadFileRange` RPC and `FileOps.ReadRange` | 4 | S,M,S,M | 370k | ~12% | 602k | CI run |
| 1.6.3 `ConnectUnaryTransport` reads, chunk cache and prefetch | 3 | M,M,S | 315k | ~12% | 513k | CI run |
| 1.6.4 `cm6LazyDoc` gap-line module in `CmEngine` (1.6.4d is the conditional a1 window-swap fallback) | 4 | M,M,M,S | 445k | ~12% | 724k | S-T3 owner phone gate (Task 0.1.4d); CI run |
| 1.6.5 Windowed viewer, large-file states, Accessible view through the transport | 3 | M,M,S | 315k | ~12% | 513k | CI run |
| 1.6.6 Large-file search: request-scoped file cap and hit `byte_offset` (ADR-030 amended) | 2 | M,S | 185k | ~12% | 301k | CI run |
| 1.6.7 Phase B verification: e2e matrix, benchmarks, owner phone check | 3 | M,S,XS | 205k | ~12% | 334k | Owner phone check (tstapler) flips `files:range-read`; benchmark runs; CI run |
| 2.1.1 Atomic write primitive | 2 | M,S | 185k | ~12% | 301k | CI run |
| 2.1.2 `WriteFile` RPC (+ `FileWriter` ports, unconditional body limit on both listeners 2.1.2f re-classed XS to S, conditional remote-tier deny 2.1.2g) | 7 | S,S,M,S,M,S,S | 535k | ~12% | 871k | CI run |
| 2.1.3 Post-save status invalidation | 1 | S | 55k | ~12% | 90k | CI run |
| 2.1.4 Round-trip and security corpus | 1 | M | 130k | ~12% | 212k | CI run |
| 2.1.5 `ApplyFilePatch` server: splice, layered verification, replay store, guards (ADR-036 Phase C) | 6 | S,M,M,S,M,M | 630k | ~12% | 1025k | Task 0.1.3b remote-tier finding; CI run |
| 2.2.1 Edit mode and save | 3 | M,M,S | 315k | ~12% | 513k | CI run |
| 2.2.2 Unsaved guards and drafts (+ IndexedDB backup tier 2.2.2d, Close others Save all 2.2.2e) | 5 | S,M,S,S,S | 350k | ~12% | 570k | Owner veto on draft persistence (localStorage and IndexedDB tiers) |
| 2.2.3 Offline, unreachable and unknown-outcome saves (+ `useReachability`, M since triad round 2) | 1 | M | 130k | ~12% | 212k | CI run |
| 2.2.4 Save affordances and read-mode editing feedback | 2 | S,S | 110k | ~12% | 179k | CI run |
| 2.2.5 Delta encoder, stop-and-wait sync loop, ChangeSet drafts, delta UX states | 6 | M,M,M,S,M,S | 630k | ~12% | 1025k | CI run |
| 2.3.1 Conflict dialog | 3 | XS,M,S | 205k | ~12% | 334k | CI run |
| 2.3.2 3-way merge (+ phone kill switch and degrade 2.3.2e, phone merge flow 2.3.2f) | 6 | S,M,M,S,M,S | 555k | ~12% | 903k | CI run |
| 2.3.3 Delta conflicts: `PreflightFilePatch`, same-position rebase, `MergeSurface` seam | 3 | S,M,S | 240k | ~12% | 391k | CI run |
| 2.4.1 Lazy vim mode (+ input-profile precedence, Task 2.4.1d) | 4 | M,M,S,S | 370k | ~12% | 602k | CI run |
| 2.4.2 Extra-key bar for phone editing (+ `editLayoutBudget` ladder and fixed Save/Stop cluster, Task 2.4.2c) | 3 | M,M,M | 390k | ~12% | 635k | CI run |
| 2.4.3 Editing e2e | 1 | M | 130k | ~12% | 212k | CI run |
| 2.5.1 Gates, registry, docs | 2 | M,S | 185k | ~12% | 301k | CI run |
| 2.5.2 Isolated manual verification and owner device check | 1 | S | 55k | ~12% | 90k | Owner phone check (tstapler); CI queue |
| 2.5.3 Files metrics view and derived queries (ADR-035) | 3 | M,M,S | 315k | ~12% | 513k | CI run |
| 2.5.4 Owner dogfood and screen-reader scripts | 2 | S,S | 110k | ~12% | 179k | Owner phone and screen-reader checks (tstapler) |
| 2.5.5 Delta rollout: e2e matrix, registry/docs for the new RPCs, large-edit dogfood | 3 | M,S,S | 240k | ~12% | 391k | Owner large-file edit dogfood (tstapler); Gates G2 and G3; CI run |
| 2.6.1 Server stream wires: `ReadFileRangeStream` and the WebSocket bridge entry (ADR-036 Phase D) | 2 | M,S | 185k | ~12% | 301k | Gate G-D (spike results recorded in ADR-036); CI run |
| 2.6.2 `ConnectStreamTransport`, `WsStreamTransport` and pool-contention handling (Phase D) | 3 | M,M,M | 390k | ~12% | 635k | Gate G-D; CI run |
| 2.6.3 `http_range` GET adapter behind the rebinding guard and the `ServeFileRaw` audit (Phase D) | 3 | M,S,S | 240k | ~12% | 391k | Task 0.1.3b GET-guard finding; CI run |

**Totals**: 14 epics, 65 stories, 196 tasks (recounted by script after the ADR-036 integration: +2 epics, 1.6 and 2.6; +18 stories; +63 tasks, of which Task 1.6.4d is conditional and counted so the plan does not under-cost the branch; the base before it was 12 epics, 47 stories, 133 tasks, 18.04M CU). Raw task tokens 17.03M; price-weighted 15.83M CU before verification; **27.71M CU** (sum of rows) after the x1.75 verification multiplier. Size band per `ESTIMATION.md`: **XL (> 15M CU)**, about 85% above the Large band's 15M ceiling (18.04M, about 20% above, before this integration; 17.44M and 16.02M before the earlier repairs). One line, no question: proceeding at full scope; the owner approved the ADR-036 scope and phasing (2026-10-16) and selected every adapter.

**ADR-036 integration delta (+5.95M raw tokens, +9.68M CU; recomputed by script from the task classes, cost classes only)**:

| ADR-036 part | Tasks | Raw tokens | CU (x1.75 verify) |
|---|---|---|---|
| Spikes and engine gate (0.1.4c/d, Stories 0.1.7, 0.1.8) | 6 | 370k | 602k |
| Phase A: seam, `full` adapter, capabilities, engine ports (Stories 0.2.6, 1.2.3, Tasks 0.2.5d, 1.2.1f/g) | 10 | 925k | 1505k |
| Phase B: range reads, windowed viewer, lazy document, large-file search (Epic 1.6) | 21 | 2095k | 3410k |
| Phase C: delta writes, drafts, conflicts, rollout (Stories 2.1.5, 2.2.5, 2.3.3, 2.5.5) | 18 | 1740k | 2832k |
| Phase D: stream, WebSocket and `http_range` adapters (Epic 2.6, sequenced last) | 8 | 815k | 1326k |
| **Total added** | 63 | 5945k | 9675k |

The owner approved about +7.2M CU (research/transport.md section 8.1's +6.2M plus about 1.0M for the engine seam and lazy document in section 11.5; top-down, INFERRED). The bottom-up total above is about 2.5M CU higher (about 34%): 63 tasks of 3-5 files against about 36 in the top-down estimate (Phase B 21 against 16, Phase C 18 against 10, Phase D 8 against 5), the full conformance, property, fuzz, fault-injection and e2e suites, and the owner's decision to include every adapter. It is reported, not trimmed (full scope is the default); `research/transport.md` section 8.1 and 11.5 now carry these figures. Absorbed in existing task classes rather than costed as new tasks (first suspects if the Wave 3 or Wave 6 overrun checkpoint trips): the three transport flag entries in Task 0.2.2a and `useFilesFlags`, the move of `FileWriter`'s ports onto `FileOps` (Task 2.1.2b), the `PageSource` seam in Task 1.5.4a, the pending-analysis compare in Task 1.3.4a, the `writeFull` binding in Task 2.2.1a, the `MergeSurface` note in Task 2.3.2c, the patch-body and `ServeFileRaw` recon questions in Task 0.1.3b, the `transport` block of the metrics endpoint (Tasks 2.5.3a/b), the proto fields added early in Tasks 0.2.6a and 1.1.1a, and the Story 1.4.2/1.6.6 hit `byte_offset` plumbing.

**Contingent custom engine (Epic 1.7; excluded from every figure above, costed separately)**: 6 stories, 26 tasks (19 M, 6 S, 1 XS), raw 2.82M tokens, 2.62M CU before verification, **4.59M CU** after the x1.75 multiplier; this task-level figure replaces the "about 6M CU" of ADR-036 and `research/transport.md` (top-down, INFERRED). Built only if Task 0.1.4d records FAIL for both CM6 lazy models; if built it replaces Story 1.6.4's CM6 lazy tasks 1.6.4a-c (and 1.6.4d), so the net addition to the base is the contingent figure minus about 0.64M CU. If it is activated the base totals above are restated by script.

*Contingent Epic 1.7 (excluded from the base totals; activation rule in the epic's header)*

| Story | Tasks | Classes | Raw tokens | CU (x1.75 verify) |
|---|---|---|---|---|
| 1.7.1 | 4 | M,M,M,S | 445k | 724k |
| 1.7.2 | 4 | M,M,M,S | 445k | 724k |
| 1.7.3 | 5 | M,M,M,M,S | 575k | 936k |
| 1.7.4 | 4 | M,M,M,S | 445k | 724k |
| 1.7.5 | 5 | M,M,M,M,S | 575k | 936k |
| 1.7.6 | 4 | M,M,S,XS | 335k | 545k |
| **Total** | 26 | | 2820k | 4589k |

**Critical path in agent waves** (stories on the same row run concurrently; ten waves: nine after the engineering triad re-sequencing, ET-1, which moved Epics 2.2-2.4 behind the owner phone smoke, plus Wave 10 for ADR-036 Phase D):
1. Wave 1: 0.1.1 (real-path streaming spike, now with the in-process chain test), 0.1.2, 0.1.3 (incl. the remote-access and `guardedProcedures` recon), 0.1.4 (incl. 0.1.4b WebKit CI check), 0.1.5 (phone input/viewport spike with the key-event capture panel, Task 0.1.5c; owner check and the Task 0.1.5d time-to-answer baseline are wall-clock blockers in one sitting), 0.1.6 (screen-reader read-path spike; the owner VoiceOver/TalkBack check runs in the same sitting as 0.1.5b) (spikes, parallel). **ADR-036 additions**: 0.1.7 (S-T1 compression and wire cost, S-T2 index/hash/splice throughput), 0.1.8 (S-T4 encoder prototype, S-T5 pool contention) and 0.1.4c (extended S-T3 gap-line and window-swap prototypes, own spike route), all in parallel with the other spikes; the owner S-T3 phone gate 0.1.4d is appended to the same owner sitting as 0.1.5b, 0.1.5d and 0.1.6b (it is last, after 0.1.6b); Task 0.1.3b additionally answers the remote-tier patch-body question and the `ServeFileRaw` GET-guard question; every ADR-036 editor of Wave 1 is ordered in the serialization scan below.
2. Wave 2: 0.2.4 (0.2.4c baseline first, measured on the `origin/main` merge base; then 0.2.4b, the `GetFileContent` extraction, which must precede the retrofit), 0.2.1 (0.2.1d inventory first, then resolver; `file_service.go` order is 0.2.4b -> 0.2.1c -> 0.2.1e), 1.2.1 (1.2.1e stubs, then 1.2.1a-d), 0.2.2 (flags), 0.2.4a/0.2.4e (gate target, shared fixtures, harness factory and jscpd ledger, before any Wave 3 test file). **ADR-036**: Task 1.2.1f (engine ports, `CmEngine`, import-boundary lint) runs after 1.2.1a and before 1.2.1b, Task 1.2.1g (engine conformance suite) after 1.2.1c; Task 0.2.2a carries the three transport flags.
3. Wave 3: 0.2.3 (codec; needs resolver and the 0.2.4b extraction; 0.2.3c is the last `file_service.go` edit), 1.2.2 (buffers; needs CodeEditor), 0.2.5 (analytics helper and privacy harness: 0.2.5a/b are new frontend files; 0.2.5c serializes on `server/dependencies.go` and the analytics handler, after 0.2.1c; every later story emits events through it). **ADR-036 Phase A (behavior-identical)**: 0.2.6 (0.2.6a proto after 0.2.3a; 0.2.6b new files; 0.2.6c delegates `GetFileContent`/`StatFile` through `FileOps` and so edits `file_service.go` and `files_delegates.go` after 0.2.3c; 0.2.6d conformance scaffold), 1.2.3 (1.2.3a -> 1.2.3b -> 1.2.3c after 1.2.2b, and before 1.2.2c, because `FilesTab` loads through the port) and 0.2.5d (after 0.2.5a and 0.2.5c). The plan has no Wave 0; ADR-036's "lands inside Waves 0-1" is read as Phase 0 and Phase 1 of this plan (the seam needs the resolver of Wave 2 and the codec of Wave 3 to wrap), so Phase A lands in Wave 3, before any read-only epic ships.
4. Wave 4: 1.1.1 (1.1.1e, 1.1.1f, then 1.1.1c) then 1.1.2 (1.1.2c limiter, then 1.1.2a, 1.1.2b, and conditional 1.1.2d WS-bridge registration first on `server/server.go` if ADR-030 chose path (b); server search) || 1.3.1/1.3.2 (incl. 1.3.2c)/1.3.3/1.3.4/1.3.6 (UI; 1.3.4 needs 0.2.3; 1.3.6 touches only the resize hook and handle), and 2.1.1 (atomic write) starts here.
5. Wave 5: 1.1.3, 1.3.5 (incl. 1.3.5c), 1.3.7 (banner stack and feedback states; needs 1.3.4 and 1.3.5), 1.4.1 then 1.4.2 (incl. 1.4.2d), 2.1.2 (2.1.2d -> 2.1.2f -> 2.1.2g on `server/server.go`) then 2.1.3, 2.1.4. **`FilesTab.tsx` is the shared file and forces one serial worker lane: 1.3.5 -> 1.3.7a -> 1.3.7b -> 1.4.2b** (1.3.7 needs 1.3.5, and 1.4.2b follows 1.3.7b in the FilesTab order); 1.4.1, 1.4.2a, 1.4.2d and the Go lane (1.1.3, 2.1.x) run in parallel with it. The Task 0.2.4d relocation, if triggered, runs at the end of Wave 4 or Wave 5 before more Go tests land. **Wave 4 gate extra (triad round 2)**: the gate also runs the `server/server.go` length probe of Story 0.2.4; if `revive(file-length-limit)` reports, Task 0.2.4f (the carve-out) runs before Wave 5 starts, because 2.1.2d, 2.1.2f and 2.5.3a add lines to that file.
6. Wave 6: 1.4.3, then 1.5.1 -> 1.5.2 -> 1.5.3 (1.5.3a, 1.5.3b, and 1.5.3c as the last task), with 1.5.4 (Accessible view; 1.5.4a in parallel, 1.5.4b after 1.5.1c on `MobileFilesLayout.tsx`/`FilesTab.tsx`) before 1.5.3c so the smoke includes it. **Gate G1 (wall-clock, owner): the Task 1.5.3c phone smoke on the merged Epic 1.5 (including the Accessible view and its screen-reader quick check) must be recorded before Wave 7 starts.** Nothing from Epics 2.2, 2.3 or 2.4 shares a wave with Epic 1.5 any more. **ADR-036 Phase B (read-only value, before any editing epic; five lanes, nothing from Epic 1.6 edits `FilesTab.tsx`)**: Go lane 1.6.1 -> 1.6.2 (1.6.2a after 1.1.1a/2.1.2a in `session.proto`) -> 1.6.6a; transport lane 1.6.3 (new files); engine lane 1.6.4 (needs the Task 0.1.4d verdict; 1.6.4c edits `CodeEditor.tsx` after 1.5.2b; 1.6.4d only if a1); UI lane 1.6.5a (`CodeEditor.tsx` after 1.6.4c, `FileContentViewer.tsx`), 1.6.5b, 1.6.5c (`AccessibleView.tsx` after 1.5.4b), 1.6.6b (`SearchPanel.tsx` after 1.5.1b); verification lane 1.6.7a (`files-tab.page.ts` after 1.5.3b), 1.6.7b, and the owner Task 1.6.7c appended after 1.5.3c. G1 does not wait for Epic 1.6 (informational line only); `files:range-read` stays off until Task 1.6.7c passes.
7. Wave 7 (after G1): 2.2.1 then 2.2.2 (2.2.2a -> 2.2.2d -> 2.2.2b -> 2.2.2c -> 2.2.2e)/2.2.3/2.2.4 (2.2.4 needs 1.5.1 for the top bar and 1.3.7 for the banner stack, both merged in earlier waves), and 2.3.1 after 2.2.1 (it consumes the `Conflict` outcome of `useSaveBuffer`). **ADR-036 Phase C server (dark; the Go lane is idle in this wave)**: 2.1.5a-f (2.1.5a `session.proto` after 1.6.2a; 2.1.5e on `server.go` after 2.1.2g; Phase C starts only after G1, and `files:delta-write` is not flipped before Gate G2), 2.3.3a (`PreflightFilePatch`, after 2.1.5a; `files_delegates.go` after 2.1.5e).
8. Wave 8: 2.3.2 (incl. 2.3.2e and the phone flow 2.3.2f; needs 0.2.2 and 2.3.1), 2.4.1 (incl. 2.4.1d; needs 2.2.1b and 1.5.1) then 2.4.2 (2.4.2a -> 2.4.2c -> 2.4.2b). `web-app/package.json` and `pnpm-lock.yaml` are touched by 2.3.2b and 2.4.1a, so those two tasks run back to back on one worker (2.3.2b first). **ADR-036 Phase C client**: 2.2.5a -> 2.2.5b -> 2.2.5c -> 2.2.5d -> 2.2.5e -> 2.2.5f (needs Epic 2.2 of Wave 7, Story 2.1.5, 1.6.3a, 1.6.4c and 0.1.8a; `StatusBar.tsx` 2.4.1b before 2.2.5e; `useSaveBuffer.ts` after 2.2.3a).
9. Wave 9: 2.4.3, 2.5.1, 2.5.3 (needs the events from every earlier wave; 2.5.3c appends to the how-to after 2.5.1b), 2.5.4 (2.5.4a/b write the owner scripts; the owner runs them on the Wave 7-8 build), then 2.5.2 (the walkthrough runs the signed 2.5.4 scripts; the global `files:editing` flip is blocked until the dogfood script is signed). **ADR-036**: 2.3.3b (`ConflictDialog.tsx` after 2.3.2e; `useSaveBuffer.ts` after 2.2.5b) and 2.3.3c (`MergeView3Way.tsx` after 2.3.2f), 2.5.5a (after 2.4.3a on `files-tab.page.ts`), 2.5.5b (checklist between 2.5.4b and 2.5.2a), 2.5.5c (docs after 2.5.3c). **Gate G2** (Task 2.5.4a signed on the Wave 7-8 build: full-content editing proven) must precede the live `files:delta-write` flip, and **Gate G3** (the T1 steps of Task 2.5.5b) the `files:edit-large` flip.
10. Wave 10 (ADR-036 Phase D, last; **Gate G-D**: the S-T1 and S-T5 outcomes and the Task 1.6.7b benchmarks are recorded in ADR-036): Epic 2.6, 2.6.1a -> 2.6.1b (`server.go` and `watch-ws-transport.ts` after every earlier editor), 2.6.2a, 2.6.2b (`watch-ws-transport.ts` after 2.6.1b), 2.6.2c, 2.6.3a -> 2.6.3b -> 2.6.3c (`server.go` after 2.6.1b; `file_service.go` last editor; docs after 2.5.5c). The spike results decide whether the stream adapters are advertised on by default (rule in Story 0.1.8 and the Epic 2.6 header), not whether the code is built. Wave 10 ends with its own `make files-viewer-gates` run.
**Re-verification of the wave table (engineering triad ET-1)**: every dependency edge was checked against its target's wave: 2.2.4 (needs 1.5.1 W6, 1.3.7 W5, 2.2.1 W7) lands in W7; 2.3.2e (needs 0.2.2 W2, 2.3.1 W7) lands in W8; 2.4.1d (needs 2.4.1a W8, 1.5.1 W6) lands in W8 and `MobileFilesLayout.tsx` order is 1.5.1a (W6), 2.2.4a (W7), 2.4.1d (W8), 2.4.2b (W8); 2.5.3 (needs events from W7-W8 emitters) and 2.5.4 (needs the editing, merge and key-bar features) land in W9; 0.1.5 (W1) precedes Epics 1.5 and 2.4; G1 (end of W6) precedes Epics 2.2, 2.3 and 2.4. The previous plan put 1.5.x and 2.2.x in the same Wave 6, so the phone smoke could not precede 2.2.1; that contradiction is removed.
**Re-verification of triad round 2 placements**: 0.1.6 (W1) precedes 1.5.4 (W6) and Epic 2.4; 1.5.4b (needs 1.5.1c W6 and 0.1.6 W1) lands in W6 before G1; 1.3.5c (needs 1.3.5b W5 and 0.1.4b W1 for the Playwright projects) lands in W5; 1.4.2d (needs 1.4.2b W5) lands in W5; 2.1.2g (needs 0.1.3b W1 and 2.1.2d W5) lands in W5 before 2.1.3; 2.2.3a (reachability; needs 1.3.4a W4 for the heartbeat) lands in W7; 2.3.2f (needs 2.3.2c W8) and 2.4.2c (needs 2.4.2a W8) land in W8; 0.2.4f (needs the Wave 4 gate) lands between W4 and W5.
**Re-verification of the ADR-036 placements**: every edge was checked against its target's wave. 0.1.4c/0.1.7/0.1.8 (W1) precede every consumer; 0.1.4d (W1, owner) precedes 1.6.4 (W6); 0.2.6 (needs 0.2.1 W2, 0.2.2 W2, 0.2.3 W3) lands in W3 after 0.2.3c; 1.2.3 (needs 1.2.2b W3) lands in W3 before 1.2.2c; 1.2.1f/g (need 1.2.1a W2) land in W2; 1.6.1 (needs 0.2.6c W3, 0.1.7b W1) and 1.6.2 (needs 1.1.1a W4 for the shared proto order, 2.1.2a W5, 1.6.1) land in W6; 1.6.3 (needs 1.6.2a, 1.2.3 W3) lands in W6; 1.6.4 (needs 1.2.1f W2, 0.1.4d W1) lands in W6; 1.6.5 (needs 1.6.3b, 1.6.4c, 1.5.2b, 1.5.4b, all W6) lands in W6 after them; 1.6.6a (needs 1.1.1 W4) and 1.6.6b (needs 1.4.2 W5, 1.5.1b W6) land in W6; 1.6.7 (needs 1.6.5/1.6.6, 1.5.3b) closes W6. 2.1.5 (needs 2.1.2 W5, 1.6.1b W6, 0.2.6c W3, and G1) lands in W7; 2.3.3a (needs 2.1.5a) in W7; 2.2.5 (needs 2.2.1 W7, 2.2.2d W7, 2.2.3a W7, 2.1.5, 1.6.3a/1.6.4c W6, 0.1.8a W1) lands in W8; 2.3.3b/c (need 2.3.2e/f W8, 2.2.5b) land in W9; 2.5.5 (needs 2.2.5, 2.3.3, 2.4.3a) lands in W9; 2.6.x (needs 2.1.5a, 1.6.3, 1.2.3a, 0.1.3b, and Gate G-D) lands in W10. The shipped order is therefore A (W3) -> B (W6, before editing) -> C (W7-W9, after G1) -> D (W10, last), matching the owner's phasing; the flag flips are separate owner actions after Task 1.6.7c, Gate G2 and Gate G3.
**Gate checkpoint (every wave, Story 0.2.4)**: each wave ends by running `make files-viewer-gates` (jscpd with headroom, `ready-complexity-gate`, 350-line budget, `server/services` race wall-clock delta) and pasting the output into the PR description; a failing gate is fixed in the owning story's files before the next wave starts. Epic-closing stories (1.1.3, 1.3.6, 1.4.3, 1.5.3, 1.6.7, 2.1.4, 2.1.5, 2.2.3, 2.2.5, 2.3.2, 2.3.3, 2.4.3, 2.5.5, 2.6.3) inherit that run. After Wave 6 the owner runs the Task 1.5.3c phone smoke (Gate G1) before Epics 2.2/2.3/2.4 begin. **ADR-036 gates**: the three conformance suites join the gate from the wave in which their first case lands, and a failing suite blocks the wave exactly like a failing jscpd or complexity check: Go `FileOpsConformance` (scaffold W3, read cases W6, patch and preflight cases W7), TS `runTransportConformance` (scaffold W3, read cases W6, patch cases W8, stream and byte cases W10), TS `runEngineConformance` (scaffold W2, lazy cases W6, edit cases W8), plus the shared golden vectors and `FuzzApplyPatch` (W7); an adapter, backend or engine ships only when its suite is green. Gate G2 (Task 2.5.4a signed) and Gate G3 (Task 2.5.5b T1 steps) are owner gates on flag flips, not on code merging.
**Shared-file serialization points** (scanned by script over every task's `Files:` line, grouped by the new wave; engineering triad ET-2). Cross-story collisions in one wave, each with its required order:
- `server/services/file_service.go`: Wave 2 **0.2.4b -> 0.2.1c -> 0.2.1e**, then Wave 3 0.2.3c (0.2.3c is the only later editor). Tests: `file_service_test.go` is touched by 0.2.1d, 0.2.1c, 0.2.1e in that order. 0.2.4b is in Story 0.2.4 and 0.2.1c in Story 0.2.1, so two workers must not run them concurrently.
- `session.proto`: 0.2.3a (W3), 1.1.1a (W4), 2.1.2a (W5), one commit each. New delegates go in `files_delegates.go` (0.2.3c W3, 1.1.2b W4, 2.1.2d W5); `session_service_delegates.go` is not edited by any task (it only forwards; VERIFIED `:640-653`). `server/server.go` is touched by 2.1.2d, 2.1.2f and 2.1.2g (W5, in that order; 2.1.2b also wires `FileWriter` there) and 2.5.3a (W9), all after the Wave 4 length probe (or in `server/guarded_procedures.go` after Task 0.2.4f); `server/services/workspace_service.go` only by 2.1.3a; `server/dependencies.go` and `analytics_handler.go` only by 0.2.5c.
- `SessionDetailView.tsx`: 1.2.2c (W3, add `key={session.id}`) then 1.3.2c (W4, remove the hash effect), nothing else.
- `FilesTab.tsx` (1.2.2c, 1.2.2d W3; 1.3.2a, 1.3.4b W4; 1.3.7a, 1.3.7b, 1.4.2b W5; 1.5.1c, 1.5.4b W6; 2.2.1c, 2.2.2b W7) and `CodeEditor.tsx` (1.2.1a W2, 1.4.2c W5, 1.5.2b W6, 2.2.1b, 2.2.4b W7, 2.4.1b W8) are touched by 5+ stories each; see the merge-conflict note below.
- Wave 6: `tests/e2e/pages/files-tab.page.ts` is touched by 1.4.3a and 1.5.3b (different stories, same wave): order **1.4.3a -> 1.5.3b**. `playwright.config.ts` is touched by 1.3.5c (W5, desktop-firefox/desktop-webkit projects) then 1.5.3a (W6) in that order; `files-a11y.spec.ts` and `tests/e2e/helpers/a11y.ts` have a single editor in W6 (1.5.3b). `MobileFilesLayout.tsx`: 1.5.1a -> 1.5.4b (W6) -> 2.2.4a (W7) -> 2.4.1d -> 2.4.2b (W8).
- Wave 7: `StatusBar.tsx` by 2.2.1c -> 2.2.2b -> 2.2.4a (three stories; one worker, in that order, `FilesTab.tsx` ordered 2.2.1c -> 2.2.2b); `useSaveBuffer.ts` by 2.2.1a -> 2.2.3a; `CodeEditor.tsx` by 2.2.1b -> 2.2.4b.
- Wave 8: `web-app/package.json` and `pnpm-lock.yaml` by 2.3.2b -> 2.4.1a (lockfile conflicts are resolved by re-running `pnpm install`, never by hand-merging); `inputProfile.ts` by 2.4.1a -> 2.4.1c -> 2.4.1d (one story); `MobileFilesLayout.tsx` by 2.4.1d -> 2.4.2b; `ExtraKeyBar.tsx` by 2.4.2a -> 2.4.2c (one story); `ConflictDialog.tsx` by 2.3.2c -> 2.3.2e (one story); `MergeView3Way.tsx` by 2.3.2c -> 2.3.2f (one story).
- Wave 9: `docs/how-to/use-files-search-and-editing.md`, `CLAUDE.md` and `docs/registry/features/` by 2.5.1a/b -> 2.5.3c (2.5.1b first); `manual-verification-checklist.md` by 2.5.4a -> 2.5.4b -> 2.5.2a (the walkthrough appends last); `analytics_files_handler_test.go` by 2.5.3a -> 2.5.3c.
- Same-story same-file overlaps (one worker, task order as numbered unless stated) were also listed by the scan and need no cross-worker rule: `path_resolver(_test).go` (0.2.1a, 0.2.1b, 0.2.1d), `content_search.go` (1.1.1b, 1.1.1c), `content_search_test.go` (1.1.1d, 1.1.1e, 1.1.1f), `file_atomic.go` (2.1.1a, 2.1.1b), `deepLink.test.ts` (1.3.2b, 1.3.2c), `server/server.go`/`file_writer.go`/`file_write_test.go` inside Story 2.1.2/2.1.3, `SearchResults.tsx` (1.4.2a, 1.4.2c).
- Directory-valued entries (`extensions/*.ts`, `docs/registry/features/`) cannot be matched by the scan: `extensions/*` is created by 1.2.1e (W2), then filled by one story each in different waves (`searchHighlight.ts` W5, `degrade.ts` W6, `readOnlyFacet.ts` W7, `readModeHint.ts` W7 by 2.2.4b, `merge.ts`/`vim.ts` W8), so no two workers share a module in a wave.
**Full serialization scan, all waves (triad round 3, E1; supersedes the Wave 2-9-only coverage above where they overlap)**: a script (`scan.py` in the repair scratchpad, not committed) parsed every task's `Files:` line and every story's `**Files**` line, assigned each story to its wave from the list above, and reported every file named by more than one task (62 files) and every file named by more than one **story in the same wave** (24 collisions, the ones that can put two workers on one file). Wave 1 was missing from the earlier scan entirely. Each collision below has an explicit order; a file touched in different waves is ordered by the waves and is listed only where the order is not obvious. New tasks 0.1.5d, 1.1.2d, 2.2.2d and 2.2.2e are included (133 tasks).
- **Wave 1 (parallel spikes; one worker per file at a time)**: `tests/e2e/files-viewer.spec.ts` is named by Story 0.1.1 (its Playwright streaming check), Task 0.1.4a and Task 0.1.6a: order **0.1.1 -> 0.1.4a -> 0.1.6a** (all append a describe block; 0.1.1b's spec section is the first). `tests/e2e/pages/files-tab.page.ts` has one Wave 1 editor (0.1.4a). The throwaway spike page under `web-app/src/app/` is one file edited by **0.1.5a -> 0.1.5c -> 0.1.6a** (page and Playwright route first, capture panel second, the two 3,000-line variants third; the owner sitting for 0.1.5b/0.1.5d/0.1.6b follows 0.1.6a). `manual-verification-checklist.md` (spike notes section): **0.1.5b -> 0.1.5d -> 0.1.6b** (all owner, one sitting, appended in that order). ADR-030 is edited by 0.1.1b in Wave 1 and 1.1.3a in Wave 5; ADR-031 by 0.1.3b (W1) then 0.2.1d (W2); `content_search_bench_test.go` by 0.1.2b (W1) then 1.1.3a (W5); `inputProfile.test.ts` fixtures by 0.1.5c (W1) then 2.4.1c/2.4.1d (W8). No two of these are in one wave on different workers except the three above.
- **Wave 2**: `file_service.go`: 0.2.4b -> 0.2.1c -> 0.2.1e (then W3 0.2.3c); `file_service_test.go`: 0.2.1d -> 0.2.1c -> 0.2.1e; `path_resolver.go`/`_test.go`: 0.2.1a -> 0.2.1b -> 0.2.1d (one story); `file_codec.go` is created by the 0.2.4b extraction (W2) and then extended by 0.2.3b (W3); conditional 0.2.4d moves `path_resolver.go`, `file_codec.go`, `file_atomic.go` and the `content_search*.go` files at the end of Wave 4 or 5, after every edit to them in Waves 2-4 and before any Wave 5 task that touches them, so it needs no ordering with Wave 2 tasks beyond the waves; `Makefile`: 0.2.4a -> 0.2.4c -> 0.2.4e (one story); `useFilesFlags.ts`: 0.2.2b (W2) then 2.3.2e (W8).
- **Wave 3**: `session.proto` 0.2.3a; `files_delegates.go` 0.2.3c (then 1.1.2b W4, 2.1.2d W5); `bufferReducer.ts` 1.2.2a (then 1.3.2a W4, 2.2.1b W7); `SessionDetailView.tsx` 1.2.2c then 1.3.2c (W4); `useFilesDeepLink.ts` 1.2.2d then 1.3.2c; `fixtures/` 0.2.4a (W2) then 0.2.5b (W3). No same-wave cross-story collision in Wave 3.
- **Wave 4**: `FilesTab.tsx` **1.3.2a -> 1.3.4b** (one worker); `server/server.go`: **1.1.2d (conditional) first**, then nothing else until the Wave 4 gate; `content_search_test.go` **1.1.1e -> 1.1.1f -> 1.1.1d**, `content_search.go` 1.1.1b -> 1.1.1c, `content_search_handler_test.go` **1.1.2c -> 1.1.2a**, `file_atomic.go` 2.1.1a -> 2.1.1b, `files-a11y.spec.ts` and `files-tab.page.ts` 1.3.6b (single W4 editor), `StaleBanner.tsx` 1.3.4b (then 1.3.7a W5), `file_metrics.go` 1.1.2b (then 2.1.2d W5).
- **Wave 5**: `FilesTab.tsx` **1.3.7a -> 1.3.7b -> 1.4.2b** (the FilesTab lane above); `SearchResults.tsx` 1.4.2a -> 1.4.2c; `SearchPanel.tsx` and `SearchPanel.test.tsx` **1.4.2b -> 1.4.2d -> 1.4.2c** (heading order of Story 1.4.2); `CodeEditor.tsx` 1.4.2c; `playwright.config.ts` 1.3.5c (then 1.5.3a W6); `server/server.go` **2.1.2b -> 2.1.2d -> 2.1.2f -> 2.1.2g**; `server_probeguard_test.go` 0.2.4f (if run) -> 2.1.2e; `file_writer.go` **2.1.2b -> 2.1.2g -> 2.1.3a**; `file_write_test.go` **2.1.2e -> 2.1.2f -> 2.1.2g -> 2.1.3a** (Story 2.1.3 starts after Story 2.1.2 finishes); `workspace_service.go` only 2.1.3a; `QuickOpenPalette.tsx` 1.3.5b (then 1.5.4b W6).
- **Wave 6**: `FilesTab.tsx` **1.5.1c -> 1.5.4b**; `MobileFilesLayout.tsx` **1.5.1a -> 1.5.4b**; `SearchPanel.tsx` 1.5.1b (after the Wave 5 edits); `files-tab.page.ts` **1.4.3a -> 1.5.3b**; `files-mobile.spec.ts` and `a11y.ts` 1.5.3b only; `AccessibleView.tsx`/`.test.tsx` **1.5.4a -> 1.5.4b**; `QuickOpenPalette.tsx` 1.5.4b; `chooseFilesLayout.ts` 1.5.1c; `playwright.config.ts` 1.5.3a; `manual-verification-checklist.md` 1.5.3c (the gate, last).
- **Wave 7**: `CodeEditor.tsx` **2.2.1b -> 2.2.4b**; `AccessibleView.tsx`/`.test.tsx` 2.2.1b (after the Wave 6 edits); `FilesTab.tsx` **2.2.1c -> 2.2.2b**; `StatusBar.tsx` **2.2.1c -> 2.2.2b -> 2.2.4a**; `useSaveBuffer.ts` and its test **2.2.1a -> 2.2.3a**; `draftStore.ts` and `draftStore.test.ts` **2.2.2a -> 2.2.2d**; `DiscardDialog.tsx` **2.2.2b -> 2.2.2e**; the Story 2.2.2 order is **2.2.2a -> 2.2.2d -> 2.2.2b -> 2.2.2c -> 2.2.2e** (the banner in 2.2.2b consumes 2.2.2d's backend result; 2.2.2e wires the dialog shell of 2.2.2b); `ConflictDialog.tsx` and its test 2.3.1b/2.3.1c (then 2.3.2c/2.3.2e W8); `web-app/package.json` and `pnpm-lock.yaml` 2.3.1a (Wave 7, one task).
- **Wave 8**: `package.json` and `pnpm-lock.yaml` **2.3.2b -> 2.4.1a** (after 2.3.1a of Wave 7); `ConflictDialog.tsx` **2.3.2c -> 2.3.2e**; `MergeView3Way.tsx` **2.3.2c -> 2.3.2f**; **2.3.2e -> 2.3.2f** (the `phoneMergeAvailable` predicate); `MobileFilesLayout.tsx` **2.4.1d -> 2.4.2b**; `ExtraKeyBar.tsx` 2.4.2a -> 2.4.2c; `files-mobile.spec.ts` 2.4.2c (one W8 editor, after Wave 6's 1.5.3b); `inputProfile.ts` and its test 2.4.1a -> 2.4.1c -> 2.4.1d; `CodeEditor.tsx`, `StatusBar.tsx`, `FileContentViewer.tsx` and new `extensions/escLeave.ts` 2.4.1b only (after the Wave 7 edits).
- **Wave 9**: as listed above, plus `server/server.go` 2.5.3a **last** (after the Wave 4 length probe and every Wave 5 edit), `files-tab.page.ts` 2.4.3a (after 1.5.3b), `manual-verification-checklist.md` **2.5.4a -> 2.5.4b -> 2.5.2a**.

**ADR-036 serialization scan (rerun after the integration; same method as the full scan: a script over every task's `Files:` line with the wave of its story, now 196 tasks: 95 files named by more than one task and 39 same-wave cross-story collisions across the whole plan (the earlier scan found 62 and 24 over 133 tasks); this block supersedes the earlier ones where they overlap and lists every collision that involves an ADR-036 task, each with its order)**:
- **Wave 1**: the ADR-036 "Spike outcomes" section is edited by **0.1.3b -> 0.1.7a -> 0.1.7b -> 0.1.8a -> 0.1.8b -> 0.1.4d** (the engine verdict summary is written last); `manual-verification-checklist.md` **0.1.5b -> 0.1.5d -> 0.1.6b -> 0.1.4d**; `transport_spike_test.go` 0.1.7a -> 0.1.7b; the new spikes use their own specs and routes (`files-lazydoc-spike.spec.ts`, `files-transport-spike.spec.ts`, `web-app/src/app/spike-lazydoc/`), so they do not join the `files-viewer.spec.ts` order **0.1.1 -> 0.1.4a -> 0.1.6a** or the spike-page order **0.1.5a -> 0.1.5c -> 0.1.6a**.
- **Wave 2**: `CodeEditor.tsx` 1.2.1a, then 1.2.1f creates `engine/CmEngine.ts` and the ports and edits the ESLint override (single editor of each); `engine/__tests__/engineConformance.ts` 1.2.1g only; `FileContentViewer.tsx` 1.2.1b after 1.2.1f; Task 0.2.2a (amended to add the three transport flags) remains the single Wave 2 editor of `feature_flag_service.go`.
- **Wave 3**: `session.proto` **0.2.3a -> 0.2.6a**; `file_service.go` and `files_delegates.go` **0.2.3c -> 0.2.6c** (0.2.6c is the last Wave 3 editor of both); `useOpenBuffers.ts` **1.2.2b -> 1.2.3c**; the Story 1.2.2 / 1.2.3 order is **1.2.2a, 1.2.2b -> 1.2.3a -> 1.2.3b -> 1.2.3c -> 1.2.2c -> 1.2.2d**; `events.ts`, `analytics/__tests__/`, `files_telemetry.go`, `files_telemetry_test.go` **0.2.5a/b/c -> 0.2.5d**; `fixtures/` 0.2.4a (W2) -> 0.2.5b, and 1.2.3c adds one new file `fakeTransport.ts` there (no edit to `index.ts` or `filesHarness.ts`); `useOpenBuffers.test.ts` **1.2.2b -> 1.2.3c**; `file_ops*.go` and `file_backend*.go` are new files with a single Wave 3 editor each (0.2.6b, 0.2.6c).
- **Wave 6**: `session.proto` has one Wave 6 editor, 1.6.2a (the cross-wave order is **0.2.3a -> 0.2.6a (W3) -> 1.1.1a (W4) -> 2.1.2a (W5) -> 1.6.2a (W6) -> 2.1.5a (W7) -> 2.6.1a (W10)**, one commit each); `files_delegates.go`, `content_search_limits.go`, `file_metrics.go` 1.6.2c (after their Wave 4-5 editors); `file_ops.go` 1.6.1b; `file_ops_capabilities.go` 1.6.2b; `file_ops_conformance_test.go` 1.6.2d; `content_search*.go` 1.6.6a (after Waves 4-5); `CodeEditor.tsx` **1.5.2b -> 1.6.4c -> 1.6.5a**; `CmEngine.ts` **1.6.4c -> 1.6.4d** (d only if a1 won; they are alternatives, never both); `engineConformance.ts` 1.6.4c; `bufferReducer.ts` and `FileContentViewer.tsx` 1.6.5a (single W6 editor); `StatusBar.tsx` and `bannerPriority.ts` 1.6.5b (single W6 editor); `AccessibleView.tsx` and its test **1.5.4a -> 1.5.4b -> 1.6.5c**; `degradeProfile.ts` and its test **1.5.2a -> 1.6.5c**; `SearchPanel.tsx` **1.5.1b -> 1.6.6b** (`searchClient.ts` and `SearchPanel.test.tsx` single W6 editor 1.6.6b); `tests/e2e/pages/files-tab.page.ts` **1.4.3a -> 1.5.3b -> 1.6.7a**; `manual-verification-checklist.md` **1.5.3c -> 1.6.7c**; `connectUnaryTransport.ts` 1.6.3a and `transportConformance.ts` 1.6.3c (single W6 editors); the ADR-036 file 1.6.7b.
- **Wave 7**: `session.proto` 2.1.5a; `files_delegates.go` **2.1.5e -> 2.3.3a**; `server/server.go` 2.1.5e (after 2.1.2g, before 2.5.3a); `file_write_corpus_test.go` **2.1.4a (W5) -> 2.1.5f**; `file_ops_conformance_test.go` 2.1.5f; `file_atomic.go` 2.1.5c (an exported helper only, after 2.1.1); `file_backend_osroot.go` 2.1.5c; `file_ops_capabilities.go` 2.1.5e; Epic 2.2 orders as before (`AccessibleView.tsx` 2.2.1b after 1.6.5c).
- **Wave 8**: `StatusBar.tsx` **2.4.1b -> 2.2.5e**; `bannerPriority.ts` 2.2.5e; `CmEngine.ts` 2.2.5a (after 1.6.4c); `useSaveBuffer.ts` and its test 2.2.5b (after 2.2.3a); `draftStore.ts`, `draftBackends.ts`, `draftStore.test.ts` 2.2.5d (after 2.2.2d); `connectUnaryTransport.ts` 2.2.5c (after 1.6.3a); `transportConformance.ts` and the conformance test 2.2.5f (after 1.6.3c); `package.json`/`pnpm-lock.yaml` unchanged (**2.3.2b -> 2.4.1a**; no ADR-036 task adds a dependency except the contingent epic's Lezer parsers, which would append after 2.4.1a).
- **Wave 9**: `ConflictDialog.tsx` and its test **2.3.2e -> 2.3.3b**; `MergeView3Way.tsx` and its test **2.3.2f -> 2.3.3c**; `useSaveBuffer.ts` 2.3.3b (after 2.2.5b); `files-tab.page.ts` **2.4.3a -> 2.5.5a**; `tests/e2e/helpers/largeFile.ts` 2.5.5a (after 1.6.7a); `use-files-search-and-editing.md` **2.5.1b -> 2.5.3c -> 2.5.5c**; `docs/registry/features/` **2.5.1a -> 2.5.3c -> 2.5.5c**; `CLAUDE.md` 2.5.1b -> 2.5.3c; `Makefile` 2.5.5c (after 0.2.4e); `manual-verification-checklist.md` **2.5.4a -> 2.5.4b -> 2.5.5b -> 2.5.2a**; `server/server.go` 2.5.3a remains the last editor of Waves 1-9.
- **Wave 10**: `server/server.go` **2.6.1b -> 2.6.3a**; `watch-ws-transport.ts` **2.6.1b -> 2.6.2b**; `watch_ws_bridge_registration_test.go` 2.6.1b (after 1.1.2d); `session.proto` and `files_delegates.go` 2.6.1a (last editor of both); `file_service.go` 2.6.3a (last editor; `ServeFileRaw` guard); `file_http_range_test.go` **2.6.3a -> 2.6.3c**; `router.ts` 2.6.2c (after 1.2.3a); `files-transport-spike.spec.ts` 2.6.2c (after 0.1.8b); `use-files-search-and-editing.md` 2.6.3c (after 2.5.5c).
- Same-story same-file overlaps (one worker, task order as numbered): `cm6LazyDoc.test.tsx` (1.6.4a, 1.6.4b), `transport_spike_test.go`, `file_patch*.go` inside Story 2.1.5, `file_http_range_test.go`, contingent Epic 1.7's `custom/*` files.

**Merge-conflict avoidance for `CodeEditor.tsx` / `FilesTab.tsx`**: (1) 1.2.1e creates the `extensions/*.ts` modules and 1.2.2d creates the hook seams *before* any later story starts, so 1.4.2c, 1.5.2b, 2.2.1b and 2.4.1b add code to `extensions/*` and hooks, leaving only a one-line registration in the two files; (2) stories that touch the same file are never in the same wave on different workers: `FilesTab.tsx` edits are ordered 1.2.2c -> 1.2.2d -> 1.3.2a -> 1.3.4b -> 1.3.7a -> 1.3.7b -> 1.4.2b -> 1.5.1c -> 2.2.1c -> 2.2.2b (1.3.2a and 1.3.4b are both Wave 4: the same worker does them back to back, and each is limited to a one-line registration of `useFilesDeepLink` / `useBufferFreshness`) and `CodeEditor.tsx` edits 1.2.1a -> 1.2.1f (moves the CM6 view creation into `CmEngine.ts`; `CodeEditor.tsx` becomes the facade) -> 1.4.2c -> 1.5.2b -> 1.6.4c -> 1.6.5a -> 2.2.1b -> 2.2.4b -> 2.4.1b, each rebased on the previous (2.2.4b adds `extensions/readModeHint.ts` and a one-line registration); (3) each worker uses its own worktree and merges `main` before starting; (4) both files carry a 350-line budget enforced by a jest check (AC of 1.2.1 and 2.5.1). Use separate worktrees per parallel worker (per project memory on parallel workers).

**Overrun checkpoint**: if cumulative spend on completed stories exceeds the table value by more than 25% (about +0.25x of the running subtotal, checked at the end of Wave 3, Wave 5, Wave 6 (Phase B) and Wave 8 (Phase C)), pause and report the delta and cause. Informational, not a cut trigger. The ADR-036 integration is checked against its own subtotal as well: if the completed Phase A-C stories exceed the Phase table above by more than 25%, the report names whether the top-down estimate or the task splits were wrong.

**Wall-clock blockers (with owners)** (every owner-dependent item that can stall a wave, engineering triad ET-5):
- Early phone check of the Story 0.1.5 spike page (including the key-event capture, Task 0.1.5c) and the Story 0.1.1 streaming path, plus the Task 0.1.5d time-to-answer baseline (three timed trials of today's workaround) and the Story 0.1.6 VoiceOver/TalkBack check R1-R4 in the same sitting (tstapler; during Wave 1, before Epics 1.5 and 2.4 start; a failed R1/R2 redesigns the Accessible view before Story 1.5.4).
- Owner per-browser chord check (Chrome, Firefox, Safari; ux.md 14.1) at Story 2.5.2.
- **Gate G1**, the Task 1.5.3c read-only phone smoke including the Accessible view and its screen-reader quick check (tstapler; end of Wave 6; blocks Wave 7 and with it every task of Epics 2.2, 2.3 and 2.4).
- RESOLVED 2026-10-17 (owner: allowed, default on): Owner decision on **directory-mode editing** (veto or keep "Editing the main checkout"; tstapler; requested before Story 2.1.2 ships in Wave 5, final sign-off line at Story 2.5.2). If the veto arrives late, `NoDedicatedWorktree` is a one-reason change in Task 0.2.1b/2.1.2c plus a test row, so a late veto reworks Wave 5 output instead of blocking it.
- **The Task 0.1.3b remote-access finding** (agent, Wave 1): which remote-access tiers can reach a write RPC and whether `WriteFile` needs a per-RPC deny entry. It blocks Stories 2.1.2 and 2.1.3 (Wave 5); if the finding is "a read-scoped remote caller can reach writes", conditional **Task 2.1.2g** (S, costed and counted in the table since triad round 2; the earlier "uncosted possible extra task" wording is withdrawn) runs in Wave 5 before the flag can ever be flipped.
- Remote-access path streaming check needs the manual isolated instance on `:62872` (agent, Task 0.1.1b), plus the owner's phone for the same stream; ADR-030's fallback order is decided here and blocks Stories 1.1.2 and 1.4.1.
- RESOLVED 2026-10-17 (owner: approved, no veto): Owner veto or approval of bounded draft persistence, now covering the localStorage tier and the IndexedDB tier for 256 KB to 1 MB buffers (tstapler, before Story 2.2.2 in Wave 7; a veto of both leaves the priority-1 "Draft not stored" banner as the standing state for dirty buffers).
- **Owner phone dogfood (Task 2.5.4a)**, including the `files:merge-phone` keep/turn-off decision, and the VoiceOver/TalkBack script (Task 2.5.4b), run on the Wave 7-8 build (tstapler; before the global `files:editing` flip).
- **Real-device iOS/Android PWA check** of keyboard bar, `visualViewport` and best-effort vim (tstapler, at Story 2.5.2); this is the one check CI cannot stand in for.
- Global flag flip of `files:editing` after sign-off (tstapler).
- **ADR-036 wall-clock blockers (added by the integration)**:
  - **S-T3 owner phone gate (Task 0.1.4d)**: PASS/FAIL for the gap-line (a2) and window-swap (a1) models on the owner's phone, in the same Wave 1 sitting as Tasks 0.1.5b, 0.1.5d and 0.1.6b (tstapler). Blocks Story 1.6.4 (windowed viewer) and decides whether contingent Epic 1.7 is built; Stories 1.6.1-1.6.3 do not wait.
  - **The `ApplyFilePatch` remote decision (Task 0.1.3b, agent, Wave 1)**: whether a read-scoped remote-access caller can reach write RPCs; blocks Task 2.1.5e and the shape of `GetFileTransportCapabilities` on `:8444` (the design works for either answer; if yes, the RPC is scope-gated). The same recon records whether `ServeFileRaw`'s GET passes the rebinding verdict (input to Task 2.6.3a).
  - **S-T2 constants confirmation (Task 0.1.7b, agent, benchmark run on the real host)**: confirms or replaces `hash_cap_bytes` 64 MiB and the 1 GiB read ceiling before Story 1.6.1.
  - **Task 1.6.7c owner phone check of windowed reading (tstapler, end of Wave 6)**: the live `files:range-read` flip waits for it; it does not block any later story's code.
  - **Gate G2 (Task 2.5.4a signed on the Wave 7-8 build; tstapler)**: full-content editing is proven; precedes the live `files:delta-write` flip. **Gate G3 (Task 2.5.5b T1 steps; tstapler)**: precedes the live `files:edit-large` flip. Three separate owner flips, in the order `files:range-read`, `files:delta-write`, `files:edit-large`.
  - **Gate G-D (Wave 10 start)**: the S-T1, S-T5 and Task 1.6.7b outcomes are recorded in ADR-036 (agent); the owner may overrule the resulting default for the stream adapters. Benchmark runs (`&`) and the Phase B/C e2e runs on the 50 MB and 200 MB fixtures are CI-queue/host wall-clock items.
  - **Owner confirmation of two planner choices** (non-blocking, defaults stated): the three transport flags default off with the flip order above, and the whole-document mode for 1-8 MB on desktop (ADR-036 open question 3) is not built.
- Conditional: if the Wave 4 or Wave 5 gate shows the race-suite budget exceeded, Task 0.2.4d runs before more Go tests land (agent; delays Wave 5 by one worker run, no owner needed). Conditional: if the Wave 4 `server/server.go` length probe reports, Task 0.2.4f runs before Wave 5 (agent, no owner). Conditional: Task 2.1.2g per the 0.1.3b finding.
- CI queue runs for e2e (mobile gating project, WebKit or Chromium per Task 0.1.4b) and `make ci`; benchmarks run in background with `&`.

**Basis**: bands and weights INFERRED; recalibrate after the first implementer run.

---

## Review repair log

Repair pass over `adversarial-review.md` (BLOCKED) and `architecture-review.md` (BLOCKED). Code facts re-verified before editing: per-scope flags (`server/services/feature_flag_mutation.go:30-36`, `:94-95`; `server/deliverygate/flagcache.go:27-33`; `config/config.go:1915-1925`), `Workspace` field semantics (`session/types.go:317-355`), `file_metrics.go` absent (`ls`: no such file). Counts recounted by script: 12 epics, 37 stories, 85 tasks.

| Finding | Summary | What changed |
|---|---|---|
| ADV Blocker 1 | Per-scope flag override by repo path does not exist | Confirmed. Flags are global-only: Risk Control, Story 0.2.2 (AC now asserts scoped updates are rejected; a flag with no scopes already rejects them at `feature_flag_mutation.go:94-95`), glossary, rollout, 2.4.3/2.5.1 text, ADR-031, requirements.md ("global + per-scope" amended, owner-visible deviation). Extending the scope model was not cheap, so not done |
| ARCH Blocker 2 | `Conflict` payload capped at 1 MB with no partial marker | `TheirsVersion = FullText | Unavailable{reason}` sum type (glossary, ADR-031, Tasks 2.1.2a/c); Stories 2.1.2, 2.3.1, 2.3.2 ACs added (agent grows past 1 MB, latin-1, binary, mixed EOL, deleted); Merge and Overwrite disabled for `Unavailable`; ux.md S7 and U-CONF-8 updated |
| ARCH Blocker 3 | EOL/BOM ownership | Client sends LF-normalized text, server derives EOL/BOM from the token-verified disk file, `line_ending`/`has_bom` removed from `WriteFileRequest`, server rejects `\r`/NUL, `merge3` does not re-apply; Tasks 2.1.2a/c, 2.2.1b, 2.3.2b, Story 2.2.1 AC, Story 2.1.4 corpus (adds `crlf_bom.txt`), ADR-031 Encoding, ADR-034. Also resolves ADV minor (BOM lost after merge) |
| ARCH concern: `WorktreeDir != ""` | Makes directory-mode sessions non-editable | Confirmed against `session/types.go:325-355` (`WorktreeDir` is `""` when working in `RepoRoot`). Decision: write gate is `ActiveDir != "" && ActiveDir == ExistingDir` (blocks exactly the repo-root fallback case); directory-mode editing allowed by default with a status chip and an explicit owner-veto note (Unresolved Questions, ADR-031). Task 0.2.1b, Story 2.1.2 AC pair |
| ARCH concern: `.git` deny lexical | Symlink bypass | Task 0.2.1a: per-component `Lstat` and post-resolution recheck for writes; `linkdir -> .git/hooks` and `gitlink -> .git` added to Story 2.1.4 matrix |
| ADV + ARCH: token fast path | size+mtime acceptance without hash | Always compare sha256; fast path only short-circuits toward Conflict; Story 2.1.2 AC with `os.Chtimes`-pinned same-size rewrite; ADR-031; Task 2.1.2c |
| ADV + ARCH: search cancel keyed per session / racy `sync.Map` | Devices cancel each other | `SearchKey{SessionID, ClientID}` (proto `client_id`), single mutex-guarded registry with swap-and-cancel in the lock, process-wide walk semaphore (limit 2); ACs for two clients, 50-goroutine race, bounded walks; ADR-030 |
| ARCH concern: primitive obsession | `NewSearchQuery`, `resolveFileRequest` arg piles | `SearchOptions` with typed `MatchMode/CaseMode/WordMode`, `Scope`, `Limits`, parsed once at the boundary; `FileRef{SessionID,Path}` and `AccessMode` enum; nested proto messages (glossary, Tasks 1.1.1a/b, 0.2.1b) |
| ADV concern: draft restore has no merge base | Merge offered with no base | `MergeBase = Available | Unavailable`; restore with equal token -> base = disk text at restore time (Merge possible); changed token -> 2-way flow, Merge disabled with reason; no base persisted (secrets exposure); Story 2.2.2, 2.3.1/2.3.2, ADR-032/034, ux.md S7/S9/U-DRAFT-2 |
| ADV concern: remote-access check last | Network write primitive examined after handler | Moved into Story 0.1.3 AC and Task 0.1.3b (recon of tiers, per-RPC deny); Task 2.1.2d only enforces; ADR-031 spike outcomes and threat model |
| ADV concern + ARCH nit: `Rejected` modelled two ways | Wire shape ambiguity | Chosen: oneof has only `Saved`/`Conflict`; all refusals are Connect errors with typed `ReadOnlyReason` detail, with an explicit code table (`FlagOff` -> `FailedPrecondition` etc.); client maps to `Rejected`. Glossary, Story 2.1.2, Task 2.1.2a, 2.4.3 |
| ADV concern: split view | No story or deferral | Explicit deferral recorded in plan "Deferred / Out of Scope" and requirements.md |
| ADV concern: open-latency SLO | No measurement | Story 1.2.1 AC + new Task 1.2.1d (1 MB fixture, <300 ms with 3x CI tolerance) |
| ADV concern: orphaned `.ssq-tmp-*` | Files left in worktree | Same-dir temp kept (cross-FS rename is not atomic, so outside-worktree rejected); stale-temp sweep (>10 min, target dir) before each write; the `info/exclude` append was later dropped (see repair row R2-1); unsupported "git-status noise" claim dropped; Story 2.1.1 AC, Task 2.1.1a, ADR-031 |
| ADV concern: two UI paths behind `files:viewer-v2` | Indefinite kill switch | Retirement condition: removed after 4 flag-on weeks; flag-off path covered by a smoke test only; backlog follow-up AC in Story 2.5.1; Risk Control |
| ADV concern: mobile-webkit CI unverified | Assumption | New Task 0.1.4b (Wave 1) with fallback rule: `mobile-chromium` gates if WebKit is unreliable; 1.5.3a and 2.4.3 reference the gating project |
| ARCH concern: `Buffer` invalid combinations; dirty preview replaced; draft string in reducer | Flat record | `Loading | Failed | Loaded` union, `dirty: boolean`, dirty implies pinned, reducer property test; draft text read from `EditorState` on a debounce. Glossary, Story 1.2.2 ACs, Tasks 1.2.2a/2.2.1b, ADR-032 |
| ARCH concern: write path on `FileService` | Widened dependencies | Separate `FileWriter` (own file/constructor, ports `FilesFlags`, `WriteGuard`, `StatusInvalidator`, `WorkspaceProvider`), new Task 2.1.2b, wired in `SessionService`; mutex key `(ActiveDir, ResolvedPath)`, refcounted (also ARCH nit on leaking registry). Pattern table, Tasks 2.1.1b, 2.1.3a |
| ARCH concern: tech-debt table gaps | `session_service_delegates.go`, `SessionService`, `workspace_service.go`, ownership, `LocalFileBrowser` | Four rows added/extended; delegates go in new `files_delegates.go`; session-ownership NFR mapped to the hidden-session guard (single-user, no per-owner model; also resolves ADV minor on ownership) |
| ARCH concern: `CodeEditor.tsx` / `FilesTab.tsx` growth | 5+ stories each | `extensions/*.ts` modules (Story 1.2.1 AC), new Task 1.2.2d hook seams, 350-line budget with jest check (ACs of 1.2.1 and 2.5.1), merge-conflict-avoidance note and ordered edits in the wave plan |
| ADV minor: `file_metrics.go` listed as existing | Does not exist | Verified missing; marked new in Story 1.1.2 files, Tasks 1.1.2b/2.1.2d, Task 0.1.3b |
| UX: vim escape can be consumed | Keyboard trap risk | AC in Story 2.4.1 (`Prec.highest`, all vim modes, `:nmap <C-S-m> x` test), ADR-033, ux.md section 8.4, U-EDIT-8, risk 3. **Superseded by TR2-B4**: the chord is now Esc, Esc plus the status-bar control, tested with `:nmap <Esc> x` |
| Effort table | Consistency | Rows 0.1.4, 1.2.1, 1.2.2, 2.1.2 updated; totals recomputed by script: raw 7.53M tokens, 7.00M CU, 12.26M CU after x1.75; still Large; cost classes only |
| ADV minor: flag names differ from requirements | Rename not noted | Note added in plan header |
| ADV minor: spike 0.1.1 stand-in / cadence | `WatchSessions` is not bursty | Task 0.1.1a uses a 3-messages-100-ms-apart test stream; Task 1.1.3a re-checks cadence through the real handler on the remote path |
| ARCH nits: `fs.WalkDir(root.FS())`, test-only `git check-ignore` comment, `doublestar` direct dep, ADR-031 cite `settings_file_atomic_write.go` and threat model | Small items | Applied in Tasks 1.1.1c/1.1.1d and ADR-030/031 |
| Disputed | none | Every review claim checked held up. One refinement: ARCH suggested `WriteRejectReason` enum arm as simpler; ADV recommended Connect errors. Chose Connect errors because the existing flag-off AC and e2e assertion already expect `failed_precondition`, and a typed detail keeps `SaveOutcome` exhaustive on the client |

| R2-1 | Temp hiding wrote the shared git common-dir `info/exclude`: outside `os.Root`, not in the threat model | Dropped the exclude write (Story 2.1.1 AC, Task 2.1.1a, ADR-031); kept same-dir temp, pre-write sweep, cleanup on error; residual crash window documented and accepted |
| R2-2 | No UI for a search waiting on the 2-walk semaphore | Server sends `Queued{ahead}`; Stories 1.1.2, 1.4.1, 1.4.2 ACs (queued state, Cancel path); ux.md S1 section 3.4 row, U-SEARCH-12; ADR-030 |
| R2-3 | Directory-mode editing missing from owner sign-off | Story 2.5.2 AC line added |
| R2-4 | Lone `\r` in an otherwise-LF file | `detectLineEnding` classifies as `Mixed` -> `MixedEol` (read-only); `lone_cr.txt` in Story 2.1.4 corpus and Story 0.2.3 reasons AC |
| R2-5 | Task 2.1.2c ordering: deleted file could surface as a Connect error | Existence check folded into the token compare; deleted -> `Conflict{Unavailable{Deleted}}` (Task 2.1.2c, Story 2.1.2 AC) |
| R2-6 | `ReadOnlyReason` vs `Unavailable.reason` enums overlapped | One enum: `ReadOnlyReason` gains `Deleted`; `Unavailable.reason` is a documented subset (glossary, ADR-031) |

### Phase 4 repair (consistency.md, `implementation/consistency.md`)

Counts recounted by script: 12 epics, 38 stories, 88 tasks; effort rows parsed (class weights XS 20k / S 55k / M 130k, x0.93 x1.75): raw 7,660k, CU 12,470k.

| ID | Summary | What changed |
|---|---|---|
| P4-B1 | Paused-session search: plan searched the repo root, ux showed an empty state | Decision: search never runs against the repo-root fallback (same wrong-checkout hazard as writes). Story 1.1.2 AC replaced: `FailedPrecondition WorktreeMissing` when `ActiveDir != ExistingDir`, no `batch`; `root_is_fallback` removed from `SearchStats` (glossary, Task 1.1.2a/1.4.2b, preamble note); ux.md "Searching <root>" row removed and U-SEARCH-6 tightened; ADR-030 |
| P4-B2 | Truncated-read token had two codes | Mutually exclusive rule: well-formed token with hash `-` parses and is refused `FailedPrecondition` (`Truncated`, or `Binary`/`TooLarge` from live classification); only an unparseable string is `InvalidArgument`. Glossary, Story 2.1.2 table + refusals AC, Task 2.1.2c, ADR-031 |
| P4-B3 | Duplicate `U-EDIT-8`/`U-EDIT-9` in ux.md | Second pair renumbered `U-EDIT-11` (stale banner) and `U-EDIT-12` (rejected save); `U-EDIT-8` stays the vim escape and `U-EDIT-9` the dirty-preview promotion, so the existing citations (repair log, ADR-033, ux 8.4) were already correct; `validation.md` 8a/8b/9a/9b mapped to 8/11/9/12 |
| P4-B4 | U-A11Y-4 keyboard resize had no story; plan wrongly said it existed | VERIFIED no `role`/`aria-*`/key handling in `TreeResizeHandle.tsx` and `useResizablePanel.ts`. New Story 1.3.6 (Tasks 1.3.6a, 1.3.6b, new `files-a11y.spec.ts`); Deferred paragraph corrected; ux U-A11Y-4 made concrete; effort row added; Wave 4 |
| P4-C1 | Flag spelling | `files:viewer-v2` / `files:editing` everywhere (ux.md, requirements.md, plan header note) |
| P4-C2 | Write-log outcome | `ok|conflict_rejected|denied|disabled|error` (plan Observability, requirements, ux U-OBS-1) |
| P4-C3 | Directory-mode chip | One string, "Editing the main checkout" (ux.md owns copy; plan Unresolved Questions; Task 2.2.1c now asserts the chip) |
| P4-C4 | InputProfile rule | OR (`pointer: fine` or non-229 keydown) in Story 2.4.1 AC, matching ADR-033 and the glossary |
| P4-C5 | Tab cap | One rule: 10 on mobile / 30 on desktop, evict LRU clean tab with polite announcement; all-dirty -> toast with Close others. Story 1.2.2 AC, ux section 5, ADR-032. **SUPERSEDED by TR-B8 (triad repair) and TR2-U13 (triad round 2)**: only a clean *preview* is ever evicted (with an Undo toast of at least 8 s); pinned and dirty tabs are never closed automatically and an all-pinned/dirty set refuses the open and opens the tab list; the "all-dirty -> toast with Close others" wording here is stale and ADR-032 now says the same as ux.md section 5 |
| P4-C6 | Buffer display states | Accepted: states are derived (glossary row), no reducer fields; the "stale with unsaved edits -> Review diff" banner is an accepted follow-up for Story 2.3.1 implementation, tracked in consistency.md |
| P4-C7 | `ReadMaxBytes` had no owner | Conditional Task 2.1.2f (XS); Story 0.1.3 AC points at it |
| P4-C8 | `FilesTab.tsx` ordering | 1.3.2a and 1.3.4b added to the ordered list and the touched-by list; same worker in Wave 4 |
| P4-C9 | Mobile reveal handle | Added to Task 2.4.2b text with a jest assertion |
| P4-C10 | Palette `:`/`@` modes | Accepted follow-up: Story 1.3.5 ships `>` commands only; ux S12 `:`/`@` modes and numeric go-to-line are post-release polish, noted in consistency.md |
| P4-C11 | Search UI extras | Folded cancelled / RPC-failure Retry / Add-filters into one Story 1.4.2 AC; selection prefill and Ctrl+Enter pin accepted as follow-ups |

### Pre-mortem repair (`implementation/pre-mortem.md`)

Code facts re-verified in this worktree before editing: `server/server.go:1879-1915` (map 1879-1888, `localChainWith` 1901-1915) (`guardedProcedures`, `localChainWith`), `server/middleware/callerguard.go:21-24` (`ProfileRebinding` = POST only plus rebinding verdict), `SessionDetailView.tsx:486-498` (hash `replaceState(null, ...)` effect, clear-on-session-change effect) and `:1208-1213` (`<FilesTab>` has no `key`), `file_service.go:597-636` (`collectAllGitignorePatterns`: second full `filepath.WalkDir`, no `ctx`, `os.Open` not `os.Root`), `FeatureFlagsContext.tsx:72-96,100` (single `useEffect(fetchFlags)`, `isLoading`, no refresh), `server.go:499-509` and `watch-ws-transport.ts:223-230` (WS bridge only for `Watch*`). Counts recounted by script: 12 epics, 40 stories, 97 tasks; raw 8,160k tokens, CU 13,285k.

| ID | Sev | Summary | What changed |
|---|---|---|---|
| PM-1 | P1 | `WriteFile` not in `guardedProcedures` | Story 2.1.2 AC (registered `ProfileRebinding`; Go test over the real map: foreign-Origin POST, rebinding Host, GET rejected; membership test), Task 2.1.2d registers it in `server/server.go`, Task 2.1.2e tests, Story 0.1.3 AC + Task 0.1.3b name the map, Story 2.4.3 flag-off e2e sends valid Host/Origin and also asserts the foreign-Origin 403 so a guard refusal cannot pass as `failed_precondition`; validation.md rows |
| PM-2 | P1 | Save path bound to live `sessionId` prop; `FilesTab` unkeyed | `Buffer` captures `sessionId` at open (glossary, Task 1.2.2a, ADR-032); `key={session.id}` on `FilesTab` (Task 1.2.2c); `useSaveBuffer` uses `buffer.sessionId` + jest test that a prop change cannot redirect a save (Story 2.2.1, Task 2.2.1a); dirty buffers survive a session switch via `SessionBufferHold` + `useDraftGuard` + restore prompt (Story 2.2.2, new Task 2.2.2c); status bar shows session name and branch in Edit mode (Task 2.2.1c); validation.md rows |
| PM-3 | P2 | Symlink retrofit regresses legitimate reads | Story 0.2.1 ACs (in-tree links read normally, shown read-only `Symlink`; escaping link still `PermissionDenied` with `Symlink` detail and target string but no bytes; policy from real inventory), new Task 0.2.1d (inventory + regression tests), ADR-031. Partly declined: the pre-mortem suggested opening escaping links read-only; reading bytes outside the worktree is the hole the retrofit closes, so only the target string is shown |
| PM-4 | P2 | Ignore pre-pass is a second non-cancellable, non-`os.Root` walk; benchmark too kind | New Task 1.1.1e (`IgnoreMatcher`: incremental per-directory `.gitignore` load via `root.Open`, ctx-checked); Task 1.1.1c consumes it with `min(4, GOMAXPROCS/2)` workers; Story 1.1.1 ACs (cancel during ignore loading, incremental load, worker cap); Story 0.1.2 gets a 200-`.gitignore` variant and a cold-cache procedure; Story 1.1.3 records warm, cold, 200-`.gitignore` and first-100-ms cancel latency; ADR-030 |
| PM-5 | P2 | Streaming spike used `WatchSessions` (WS bridge), not `SearchContent`'s path | Story 0.1.1 rewritten (transport chosen up front, throwaway streaming RPC on the same handler stack, measured on `:62871`, `:62872` and the phone, connection-slot check, ADR-030 records the choice and the WS-bridge/unary fallbacks); Task 1.1.3a re-checks with the real handler; Unresolved Questions |
| PM-6 | P2 | mtime-only change gives empty-diff conflicts | Hash is the identity: Story 2.1.2 ACs (touched file with identical bytes -> `Saved`; the ADR-031 fast-path rule stands: size/mtime mismatch only triggers hashing, never acceptance; pre-rename re-check re-hashes), Task 2.1.2c; Story 1.3.4 (banner silent and `adoptToken` when only mtime moved); glossary; ADR-031 |
| PM-7 | P2 | Agent overwrites a saved edit; client never learns | DEFERRED: no post-save poll. Documented both directions of the agent race in the how-to (Story 2.5.1 AC, Task 2.5.1b); listed under Accepted risks |
| PM-8 | P2 | 256 KB..1 MB dirty buffers have no backup | Story 2.2.2 ACs: "Not backed up - save soon" chip + Save emphasis, flush on `visibilitychange`/`pagehide`, in-memory `SessionBufferHold` for any size up to 1 MB; Tasks 2.2.2a/b/c; ADR-032 |
| PM-9 | P2 | Auto-merge saved unreviewed | Story 2.3.2 ACs (merge never saves; hunk-origin labels; "Review merged result" required when any auto-merged hunk is within 1 line of a theirs hunk; property test), `MergeResult.autoMergedHunks[].nearTheirs`, Tasks 2.3.2b/c; ADR-034 |
| PM-10 | P2 | Real-device mobile checks only at the very end | New Story 0.1.5 (throwaway page on `:62871`, owner phone check, explicit `autocorrect/autocapitalize/spellcheck` attributes + assertion test) before Epics 1.5/2.4; Task 1.5.3c owner smoke of the read-only flows before Epics 2.2/2.4; Task 1.2.1a attributes; checklist lines in Story 2.5.2 |
| PM-11 | P2 | `replaceState(null, ...)` hash sync wipes pushed pane entries | Story 1.3.2 AC + new Task 1.3.2c (`useFilesDeepLink` is the single URL owner, always `replaceState(history.state, ...)`; hash effect removed from `SessionDetailView.tsx`); Back-Back e2e in Story 1.5.3 / Task 1.5.3b |
| PM-12 | P2 | Flags fetched once; loading reads as off | Story 0.2.2 ACs (`status: loading|ready|error`, neutral skeleton while loading, no remount on mid-session `viewer-v2` flip, `refresh()` on focus/visibility and on any `FlagOff` refusal, typed `FlagOff` detail also on `SearchContent`), Task 0.2.2b (XS -> S), Story 2.2.1 flag-flip AC, checklist line |
| PM-13 | P2 | CI gates discovered at the end | New Story 0.2.4 (`make files-viewer-gates`, shared jest fixtures, `GetFileContent` headroom + extraction before 0.2.3c); wave plan runs the gates at the end of every wave and epic-closing story; Story 2.5.1 AC |
| PM-14 | P3 | Symlink/`.git` TOCTOU | ACCEPTED (Accepted risks) |
| PM-15 | P3 | Rename-based save loses hard links/xattrs/ACLs; stray temp | ACCEPTED (Accepted risks; limits listed in the how-to and ADR-031) |
| PM-16 | P3 | Directory-mode mis-tap into main checkout | ACCEPTED (Accepted risks); owner veto requested before Story 2.1.2 ships |

### Triad repair (Phase 4 product/UX lens plus the owner's analytics request)

Scope: (A) tie the feature into the existing user analytics system; (B) fix the UX triad-lens gaps. Each B finding was verified against `design/ux.md` before editing (all confirmed present as stated; the evidence column cites the section that was missing or wrong). Counts recounted by script: 12 epics, 45 stories, 111 tasks; raw 9.46M tokens, 15.39M CU.

| Id | Finding / request | Verified how | What changed |
|---|---|---|---|
| TR-A1 | Owner: long-term understanding via the existing analytics system | Read `web-app/src/lib/analytics/types.ts`, `AnalyticsContext.tsx`, `HttpAnalyticsProvider.ts` (200-event drop-oldest queue), `server/analytics/provider.go`, `server/handlers/analytics_handler.go` (1000/min limiter, 100/batch, **no label validation**), `project_plans/analytics-system/` (pitfalls 7.2) | ADR-035 (taxonomy of 44 events, categories, enum-bounded labels, stamped `form`/`input`/`editing`/`v2`, privacy rules, sampling, metrics as queries); Story 0.2.5 (typed `useFilesAnalytics`, sampling, `FindFlowMeter`, canary privacy test, ESLint restriction, server `FilesTelemetry` port + `files.` label-shape guard); per-story "Analytics" AC lines in 1.1.2, 1.2.1, 1.2.2, 1.3.1-1.3.5, 1.4.1, 1.4.2, 1.5.1, 2.1.2, 2.2.1, 2.2.2, 2.3.1, 2.3.2, 2.4.1, 2.4.2, 2.2.4; Observability Plan; glossary rows; requirements.md and validation.md updated |
| TR-A2 | Where to view the metrics | No generic analytics page exists (`web-app/src/app/analytics/` has only `escape/`; `/insights` is cost data); `/api/analytics/summary` returns the global top 10 only with no label grouping | Story 2.5.3: `GET /api/analytics/files` derived-metrics handler, `/analytics/files` page, documented SQL verified equal to the endpoint by a test |
| TR-B1 | Mobile Save unreachable with the keyboard dismissed | ux.md 8.1/12: Save only on the extra-key bar (U-KEY-3) | ux.md 7.1 top-bar Save + overflow Save, 12 row, U-MOB-11; Story 2.2.4, Task 2.2.4a |
| TR-B2 | Loading and success states undefined | ux.md had only "Searching..." and Save spinner | ux.md 8.5 (9 waits with pending/failure/exit), 8.6 (success feedback), U-STATE-1/2; Story 1.3.7, Task 1.3.7b |
| TR-B3 | Banner priority; blocking banner can be hidden | ux.md 8.2 had "at most 2 visible, rest +N more" with no order | ux.md 8.2 priority list and never-hidden rule, U-EDIT-15; Story 1.3.7, Task 1.3.7a |
| TR-B4 | Concurrent agent-write journey unmapped | Plan 1.3.4 was focus/visibility only; pre-mortem #7 deferred | ux.md 8.7 (10 s poll, 1 s / two-check debounce, back-off, actor wording "may be the agent"/"source unknown", never asserted), U-EDIT-16; Story 1.3.4 AC, Task 1.3.4a (S to M); Accepted-risk #7 now partly mitigated for the active tab |
| TR-B5 | Search with a dirty buffer | ux.md S1 said nothing; hits are disk-based | ux.md 3.4 row, U-SEARCH-13; Story 1.4.2 AC, Task 1.4.2c |
| TR-B6 | Typing in Read mode; "Done" implies saved | ux.md 8.1 had no feedback and the label "Done" | ux.md 8.1 (inline hint, `E`, "Stop editing"), U-EDIT-13/14; Story 2.2.4, Task 2.2.4b |
| TR-B7 | InputProfile precedence; section 12 vim row contradicts owner decision | ux.md header said hardware-keyboard-only vim; section 12 showed a vim row and "Esc first with vim on" for mobile with no gate; ADR-033 said "persisted override wins" without precedence | ux.md header, 8.8, 12 edits, U-INPUT-1; Story 2.4.1 AC + Task 2.4.1d; Story 2.4.2 AC (vim row only when vim is on); ADR-033 amended; glossary `InputProfile` |
| TR-B8 | LRU could close pinned tabs silently | ux.md section 5 evicted "clean (preview or pinned)"; plan 1.2.2 "least-recently-used clean buffer" | Eviction touches only clean previews, announces and offers Undo, refuses when none eligible; ux.md 5, U-TAB-8; Stories 1.2.2, 1.3.1 ACs |
| TR-B9 | 320px bottom bar; Edit FAB vs quickfix chip | ux.md 7.1 listed six bottom-bar targets with no narrow rule; section 4 (chip, floating above the bar, right) and 8.1 (FAB, bottom-right above the bar) anchor the same corner | ux.md 7.1 narrow rule and `FloatingControls` stack, U-MOB-9/10; Story 1.5.1 ACs, Task 1.5.1a |
| TR-B10 | Phone 3-way merge risk; sheet-over-sheet | ux.md section 10 had no kill switch; section 6 put the Discard dialog over the tab sheet and S7 -> S8 -> Reload/Overwrite dialogs had no depth bound | `files:merge-phone` flag (Story 0.2.2, Risk Control), runtime degrade (Task 2.3.2e), owner dogfood (Task 2.5.4a), layering rule (ux.md 9.1a, U-MOB-12/13; Story 2.3.1 AC) |
| TR-B11 | A11y: screen-reader script; vim `role=status` chatter | ux.md 13 made the mode segment `role="status"` (announces every change) | ux.md 13 `aria-live="off"` + explicit-change announcements, U-STATUS-3; manual VO/TalkBack script (Task 2.5.4b, U-A11Y-7); Story 2.4.1 AC |

**Declined / adjusted**: none declined. Adjustments: (1) the vim row contradiction is resolved by requiring explicit opt-in on touch (not by removing the row, since the owner wanted vim best-effort on phone); (2) the dedicated post-save "Overwritten by another process" banner stays unbuilt, the 10 s poll reuses the ordinary stale banner instead; (3) analytics ACs are absorbed into existing task classes, not costed as separate tasks (stated in the Totals paragraph); (4) the server label guard cannot catch a short lowercase query word (shape checks only), so the privacy boundary is the typed registry plus the canary test, recorded in ADR-035.

### Engineering triad repair (Phase 4 engineering lens)

Each finding was checked against the plan and the code before editing; the evidence column says how. Counts recounted by script: 12 epics, 45 stories, 118 tasks; raw 9.84M tokens, 9.15M CU before verification, 16.02M CU after the x1.75 multiplier, band XL (> 15M CU), about 7% above the Large ceiling. Cost classes only; no human-time units.

| Id | Finding | Verified how | What changed |
|---|---|---|---|
| ET-1 | Wave plan contradicted the Task 1.5.3c gate (Epic 1.5 and Story 2.2.1 shared Wave 6) | Read the old wave list (Wave 6 held 1.5.1-1.5.3 and 2.2.1-2.2.4) and Task 1.5.3c's own "before editing stories start" text: confirmed | Re-sequenced to nine waves: Wave 6 is 1.4.3 and Epic 1.5 and ends at **Gate G1** (the owner phone smoke); Wave 7 is 2.2.x plus 2.3.1; Wave 8 is 2.3.2 (with 2.3.2e) and 2.4.1 (with 2.4.1d) then 2.4.2; Wave 9 is 2.4.3, 2.5.1, 2.5.3, 2.5.4, then 2.5.2. Dependency diagram gained gate edges; every newly placed story (2.2.4, 2.3.2e, 2.4.1d, 2.5.3, 2.5.4) was re-checked against its prerequisites' waves (listed in the wave section); `MobileFilesLayout.tsx` order and the gate wording in Tasks 1.5.3c and 2.5.4a, Story 0.2.4 and Story 2.5.1 ("Waves 2 to 9"), validation.md and the wall-clock list updated |
| ET-2 | Serialization list omitted Task 0.2.4b (same wave as 0.2.1c, both edit `file_service.go`) | Ran a script over every task's `Files:` line grouped by wave: confirmed, and it found further cross-story collisions | `file_service.go` order is **0.2.4b -> 0.2.1c -> 0.2.1e -> 0.2.3c**. Also listed with orders: `files-tab.page.ts` (1.4.3a -> 1.5.3b, Wave 6), `StatusBar.tsx` (2.2.1c -> 2.2.2b -> 2.2.4a), `useSaveBuffer.ts`, `CodeEditor.tsx`, `FilesTab.tsx` (Wave 7), `package.json`/`pnpm-lock.yaml` (2.3.2b -> 2.4.1a), `MobileFilesLayout.tsx` (Wave 8), `how-to`/`CLAUDE.md`/`docs/registry/features/`/`manual-verification-checklist.md`/`analytics_files_handler_test.go` (Wave 9). `session_service_delegates.go` is no longer listed as edited (VERIFIED: it only forwards, `:640-653`); the resolver has three production call sites, `file_service.go:141,303,729` (the plan's `131,298,460,723` were stale) |
| ET-3 | Task 1.2.1a listed seven files | Counted the `Files:` entries: confirmed (CodeEditor.tsx, css, five `extensions/*.ts`) | New **Task 1.2.1e** (S) creates the five extension stubs first; Task 1.2.1a keeps two files |
| ET-4 | Tasks 0.2.1c and 1.1.1c sized M but upper-M/L | 0.2.1c: three call sites plus `SearchFiles`' separate walk plus tests; 1.1.1c: traversal, ignore wiring, globs, a worker pool, scanner, UTF-16 and caps in one task: confirmed | Split, not justified away: **0.2.1c** (`GetFileContent` and raw-serve, M) + new **0.2.1e** (`ListFiles` and the tree-search walk, S); **1.1.1c** (walker, globs, pool, M) + new **1.1.1f** (per-file scanner, S) |
| ET-5 | Effort section's wall-clock list incomplete | Compared the list with the plan's owner items: directory-mode veto, the 0.1.3b finding, 2.5.4a dogfood and the real-device check were missing or only implicit | Wall-clock blockers rewritten with owners and what each blocks; adds Gate G1, directory-mode veto (with the cost of a late veto), the Task 0.1.3b finding (noting an uncosted possible extra task if remote callers can reach writes), 2.5.4a/2.5.4b, the real-device check, and the conditional Task 0.2.4d |
| ET-6 | ADR-030 streaming spike: name compression as prime suspect | Read `server/middleware/gzip.go` and `server/server.go:1896-1915`, `:1933` and `:1960-1969` | **Premise not confirmed.** `Compress` returns the handler untouched for every `/api/` path (VERIFIED `gzip.go:155-163`), and `SearchContent` is mounted under `/api`; Logging flushes (VERIFIED), CORS and HostGuard do not wrap the writer (VERIFIED). So compression is recorded as *not* a suspect, with the exempt-path remedy kept only for a streaming route mounted outside `/api/`. Prime suspects become the `otelhttp` wrapper (INFERRED) and off-process hops (UNVERIFIED). Story 0.1.1 gained an in-process chain AC and test (`server/streaming_chain_test.go`, also pins the exemption); ADR-030 decision 4 states the suspect order and remedy order (fix the layer, then WS bridge, then unary) |
| ET-7 | `doublestar` is v1.3.4 indirect; state the API or use v4; verify go-git `ParsePattern` reuse | `go.mod:126`, `go mod why -m` (main module does not import it), `go mod graph` (pulled in by ent and atlas; v4.10.0 present via tailscale), read the v4 and v1 sources and the pinned go-git fork (`v5.19.3-ssq.1`) | Task 1.1.1c uses **v4** (`go get github.com/bmatcuk/doublestar/v4@v4.10.0`; `ValidatePattern` at the boundary, `Match` on slash paths); `go.mod`/`go.sum` added to its files. Task 1.1.1e records the VERIFIED go-git facts (`ParsePattern` `pattern.go:41`, `NewMatcher` `matcher.go:14`, last-to-first matching, full-path-with-domain contract) and three behaviors to replicate or accept (no comment/blank skipping in `ParsePattern`, `.git/info/exclude`, mid-segment `**`) |
| ET-8 | Missing NFRs: search memory bound; open-stream cap on HTTP/1.1 | Derived the bound from the plan's own constants; read the Story 0.1.1 connection-slot AC and the walk semaphore (it bounds walks, not open streams, so a queued search pins a connection) | requirements.md NFRs; ADR-030 decisions 5 and 6; Story 1.1.1 AC (8 MB per walk, 16 MB across the 2-walk limit, heap-sampler test under 64 MB, `-benchmem`); Story 1.1.2 AC and new **Task 1.1.2c** (3 per IP, 8 total, 20 s queue wait, `ResourceExhausted` `SearchBusy`); Story 1.4.1 AC (`busy` state, no auto-retry, abort when hidden 30 s); ADR-035 outcome enums gain `busy`; validation.md REQ-33 and REQ-34 with 8 rows |
| ET-9 | Open BUG-067 and the thin jscpd margin | Found `docs/bugs/open/BUG-067-...md`; read `mcp-integration.yml`, `build.yml:275`, `Makefile:689-690`, `.jscpd.json` | BUG-067's 150 s ceiling is **stale** (the MCP workflow now runs the `server/services` leg without `-race` at 1200 s; BUG-083/BUG-099), but the bug is still open and the live exposure is `build.yml`'s `-race -short -timeout=22m` on every PR. Impact stated on the 12 (13) new `server/services` test files. Story 0.2.4 ACs and new tasks: **0.2.4c** (baseline, scoped `make files-viewer-go-tests`/`files-viewer-perf`, 5k fixture under `-short`, no `createTestStorage`, hygiene guard test, dated BUG-067 note), conditional **0.2.4d** (relocate pure code to `server/services/filesfs` if a wave gate shows more than 10% over baseline), **0.2.4e** (one `createFilesTestHarness` factory, `noDuplicateMocks.test.ts`, jscpd percentage and headroom printed every wave; raising the threshold is an explicit owner decision); validation.md REQ-35 |

**Declined / adjusted**: (1) ET-6's requested prime suspect was not adopted because the code exempts `/api/` from compression; the requested remedy order is kept as the contingency. (2) ET-7: v4 chosen instead of documenting the v1 API (v1 has no pattern validator and nothing imports it). (3) ET-9: BUG-067 is not claimed fixed (root cause still "Unknown"); the plan only proves it does not make it worse and appends a dated note. The relocation is conditional rather than upfront because moving the pure files would rewrite about 60 `Files:` references in a plan that has just been reviewed; the trigger and the mechanical move are specified instead. (4) The stream caps (3/8/20 s), the 10% race budget and the 64 MB heap ceiling are proposed constants, labelled INFERRED, with tests that pin whatever value is chosen. (5) Only the two M tasks the finding named were audited for the split; the other M tasks (for example 2.1.2c and 2.3.2b) were not re-audited and are the first suspects if the Wave 3/5 overrun checkpoint trips. (6) The current jscpd percentage could not be measured here (`web-app/node_modules` is absent), so Task 0.2.4e records it before the first new test file.

### Triad round-2 repair (fresh Product/UX/Engineering agents; Phase 4)

Every finding was checked against the docs and, for engineering items, against the code before editing; the evidence column says how. Counts recounted by script (`count.py` over the `Task`/`Story` headings): 12 epics, 47 stories, 129 tasks; raw 10.71M tokens, 9.96M CU before verification, 17.44M CU after the x1.75 multiplier, band XL (> 15M CU), about 16% above the Large ceiling. Cost classes only; no human-time units. New ids: Stories 0.1.6 and 1.5.4; Tasks 0.1.5c, 0.1.6a, 0.1.6b, 0.2.4f, 1.3.5c, 1.4.2d, 1.5.4a, 1.5.4b, 2.1.2g, 2.3.2f, 2.4.2c; ux ids U-SEARCH-14/15/16, U-NAV-6/7, U-TAB-9/10, U-MOB-14/15, U-TABLET-1, U-EDIT-17..21, U-OFFLINE-1, U-CONF-10, U-DRAFT-5, U-KEY-5, U-CHORD-1, U-TOAST-1, U-A11Y-8/9; REQ-36 and REQ-37 in validation.md.

| Id | Finding | Verified how | What changed |
|---|---|---|---|
| TR2-B1 | Screen-reader plan for CM6 on touch not testable; CM6 virtualizes so "focus at last tap" is meaningless | Re-read ux.md U-A11Y-7 and Task 2.5.4b (owner-only, Wave 9, no pass criteria): confirmed | New Story 0.1.6 (Wave 1 spike beside 0.1.5, criteria R1-R4 fixed up front, automated accessibility-tree structure test, owner check in the same sitting); new Story 1.5.4 (Accessible view: paged `<pre>` of <= 500 numbered lines, pager, Go to line, skip link, persisted toggle, Edit hand-off) designed into the architecture before Gate G1; ux.md 8.10 (S16), 8.1 edit-focus wording, U-A11Y-7/9; Task 2.5.4b re-based on R1-R4; ADR-033/035 (`files.a11y.accessible_view`, palette command); how-to known-limits AC |
| TR2-B2 | Mobile Stop editing / Save placement contradict (ux 12 vs 7.1); landscape Save at the end of a scrolling row; no min editor height | Re-read ux 7.1, 12, U-MOB-7/11: confirmed | ux.md 7.1 top bar now has Save and Stop editing (fixed order, 220px fixed targets at 320), 7.1a named viewports (XS-568 308px, S-844 584px, L-390 190px, XS-L 120px; keyboard height 260/200 is an assumption the 0.1.5 spike replaces), minimum 120px (floor 64px), a five-step fallback ladder, Save/Stop in a fixed non-scrolling cluster on the key bar when the top bar auto-hides; U-MOB-2/7/11 rewritten, U-MOB-14, U-KEY-5; Task 2.4.2c (`editLayoutBudget` pure function with matrix test), 2.4.2b, 2.2.4a, Story 2.4.3 viewport cases |
| TR2-B3 | Hardware-keyboard auto-detect false-positives on Enter/Backspace | Reasoned from the old rule ("first non-229 non-modifier keydown"): soft keyboards emit those with ordinary key codes | Detector redefined in ux.md 8.8 / ADR-033: trusted events only; primary `(pointer: fine) and (hover: hover)`; or Ctrl/Meta chord; or Escape/F-key/arrow/paging key with a non-empty `code`; or three `Key<L>` letters with matching `code`; Enter/Backspace/Space/Tab/digits never count; S4 viewport signal only as a supporting vote; default Touch; override wins and persists; vim default read only at Edit entry and never flips live (explicit "turn vim on?" chip instead); Story 0.1.5 gains the capture panel (Task 0.1.5c) and fixture AC; Story 2.4.1/Task 2.4.1d; U-INPUT-1; `files.input.profile_change` event |
| TR2-B4 | Ctrl+W, Alt+Left, Ctrl+Shift+M are browser-reserved or colliding | Browser behavior is documented knowledge, not measured here: marked UNVERIFIED per browser in ux.md 14.1 | Chord policy (ux.md 14.1): Alt+W (close), Alt+[ / Alt+] (history; Alt+Left/Right not intercepted), Esc, Esc plus the status-bar Leave editor control (primary) replace the three; denylist jest test and per-browser Playwright spec (Task 1.3.5c; desktop Chromium gating, Firefox/WebKit informational); owner per-browser manual check in the 2.5.2 checklist; ADR-033, Story 2.4.1, U-EDIT-8, U-NAV-1, U-CHORD-1, overlay list updated. Playwright cannot prove real delivery (injects below the accelerator layer), stated plainly |
| TR2-U1 | Tablet matrix (>= 768px, coarse, soft keyboard) vs key-bar inventory | ux.md had no tablet row; the key bar was described only under mobile | ux.md 7.3 matrix (desktop layout + touch sizing + Touch profile; key bar keyed on `InputProfile`, not width), U-TABLET-1; Stories 1.5.1, 1.5.3 (tablet viewport case), 2.4.2 |
| TR2-U2 | Vertical-space budget at 320x568 not named in U-MOB-2 | ux.md U-MOB-2 named no viewport | Folded into TR2-B2: named viewports in 7.1a and U-MOB-2 |
| TR2-U3 | Touch pin/unpin and pinned marker; LRU vs "Too many tabs"; stale P4-C5 | Re-read ux 5/6, ADR-032 (line 14) and repair row P4-C5: ADR-032 and P4-C5 still said "least-recently-used clean buffer" and "all dirty -> toast", contradicting TR-B8 | ux.md 5/6 (sheet row actions menu, long-press, pin glyph + "pinned"/"preview" in the name, one cap rule, explicit "Unpin and close the oldest pinned tab" only as a user choice), U-TAB-9/10, ADR-032 reconciled, P4-C5 row marked superseded, Stories 1.2.2/1.3.1/1.5.1 |
| TR2-U4 | Three Back controls need one spec, incl. returning to results after a hit | ux 7.1/7.2 and Story 1.3.2 described them separately | ux.md 7.4 (jumplist Back, pane back, browser back: scopes, rules for opening a hit and quickfix stepping, labels), U-NAV-6/7, Story 1.3.2 AC and Task 1.3.2a |
| TR2-U5 | Edit-mode scope; disabled-with-reason Edit | ux 8.1 and Story 2.2.1 left scope implicit; plan showed a chip only for Truncated | Edit mode is per buffer (not per session or global; not persisted across reload): ux 8.1, glossary `EditorMode`, ADR-032, Stories 1.2.2/2.2.1, U-EDIT-17; Edit is disabled, never absent, with visible reason for every `ReadOnlyReason` (mobile FAB popover), U-EDIT-18 |
| TR2-U6 | Phone 3-way merge wireframe/states | ux 10 had one mobile sketch and a kill switch only | ux.md 10.1 (Overview / Resolve card / Review stepper wireframes, base context lines, Change and Undo last choice, `nearTheirs` stepper gating Save, below-360px behavior including mid-merge shrink keeping choices), U-MOB-15, Story 2.3.2 AC and Task 2.3.2f |
| TR2-U7 | Offline detection beyond `navigator.onLine`; visible reason for disabled Save | ux/Story 2.2.3 used `navigator.onLine` or "a failed RPC" without a rule | ux.md 8.11 (Offline vs Unreachable from RPC network failures and the 10 s heartbeat, two-failure rule, recovery, Retry now), 7.1 (disabled-Save reason visible + `aria-describedby` + announced on tap), U-EDIT-20, U-OFFLINE-1; Story 2.2.3 and Task 2.2.3a (S to M, `useReachability`) |
| TR2-U8 | ux 11.1 (session switch => Discard dialog) vs plan `SessionBufferHold` | Plan Story 2.2.2 and ADR-032 vs ux 11.1: contradiction confirmed | **Plan wins**: the parent switches sessions with no veto point (including automated switches), so a blocking dialog cannot be guaranteed; ux.md 11.1 corrected, toast "Unsaved changes in b.go kept..." (>= 8 s), U-DRAFT-5, Story 2.2.2, ADR-032 |
| TR2-U9 | Toast durations; "banners role=status" vs priority-1 `role=alert` | ux section 0 said all banners `role="status"` while 8.2 made priority 1 `role="alert"`; toasts were 3 s and 5 s | ux.md section 0 corrected; all confirmation/success/Undo toasts >= 8 s, pause on hover/focus, dismissible, no motion under reduced-motion, Undo toasts persist longer; ux 5/8.6/13, U-TAB-8, U-STATUS-2, U-STATE-2, U-TOAST-1; Stories 1.2.2, 1.3.1, 1.3.7 |
| TR2-U10 | Search results ARIA, mobile rows, first tap with keyboard open, idle state | ux 3.1 used "option row" under collapsible groups (invalid for `listbox`) | ux.md 3.4 idle row and 3.5 (`role="tree"`, `aria-level`/`setsize`/`posinset` on every mounted row for virtualization, accessible-name format, pointerdown-capture / pointerup-activate / 10 px scroll rule), U-SEARCH-14/15/16; Story 1.4.2 ACs, Tasks 1.4.2a, 1.4.2d |
| TR2-U11 | Discard dialog default must be the safest option | ux 11 and U-TAB-7 made Save the default | Cancel is the default focus and Esc target; Save primary-styled but not default; Restore dialog default Not now; ux section 0 dialog rule, ux 11, U-TAB-7, Story 2.2.2, validation row |
| TR2-U12 | Automated hooks for 200% zoom / 320px reflow, non-text contrast, mobile focus order | U-A11Y-2/5 were manual or axe-only (axe does not check non-text contrast) | `assertReflow`, `assertNonTextContrast` (>= 3:1, both color schemes) and a Tab-sequence focus-order check in `tests/e2e/helpers/a11y.ts`; U-A11Y-2/5 amended, U-A11Y-8; Story 1.5.3, Task 1.5.3b |
| TR2-U13 | Missing states: zero-byte file, stale banner while conflict dialog open, archived session with dirty buffer | None of the three appeared in ux 8/9 | ux.md 8.9 and 7.2 rows, 9.3 row, U-EDIT-19/21, U-CONF-10; Stories 2.1.2 (archived refusal, no revival per `noarchivedrevival.md`), 2.1.4 (`empty.txt`), 2.2.1, 2.3.1; the archived-session server reason (`HiddenSession` vs `WorktreeMissing`) is taken from the Task 0.1.3b recon rather than guessed |
| TR2-E1 | ~1.5 MB WriteFile request-size cap must be unconditional | `grep -rn ReadMaxBytes server/`: only a comment at `server/services/backlog_service_lifecycle.go:1054`; no `WithReadMaxBytes` exists, so Task 2.1.2f's condition could never be true; precedent `http.MaxBytesReader` at `server/handlers/telemetry_handler.go:79` | Task 2.1.2f unconditional (`middleware.BodyLimit`, 1.5 MiB, `WriteFile` path only, typed `TooLarge`), Story 2.1.2 AC, Story 0.1.3 AC, ADR-031 "Request-size cap" |
| TR2-E2 | `workspace_service.go` missing from Task 2.1.3a Files | Read `server/services/workspace_service.go`: `vcsStatusCache` field `:82`, load `:160`, store `:225`, only eviction `:447` (`SwitchWorkspace`) | Task 2.1.3a/Story 2.1.3 Files list now include `workspace_service.go` with a new `InvalidateVCSStatus`; tech-debt row 103 corrected |
| TR2-E3 | FilesTab-driven serial edge 1.3.5 -> 1.3.7 -> 1.4.2b missing from the diagram/wave row | Plan FilesTab order list already had 1.3.7a/b -> 1.4.2b and 1.3.7 "needs 1.3.5" | Diagram note and the Wave 5 row now state the one-worker serial lane |
| TR2-E4 | Stream limiter keys on remote IP: behind a proxy/loopback all devices share a key | `server/middleware/hostguard.go:23` states the local listener sees a loopback `RemoteAddr` for same-box proxies | ADR-030 decision 6 records it (`X-Forwarded-For` ignored, total cap 8 still bounds the process); test `TestStreamLimiter_ShouldKeyOnRemoteAddr_And_IgnoreForwardedHeaders` with two `RemoteAddr` values; validation row |
| TR2-E5 | Possible extra Wave-5 task is uncosted | Wall-clock list said "the cost of that task is not in the table" | Conditional Task 2.1.2g (S) added, counted, placed in Wave 5; blocker wording withdrawn |
| TR2-E6 | Wave 4 gate check for `server/server.go` length (revive) | `wc -l server/server.go` = 2,172; `.golangci.yml:47-51` `file-length-limit` max 1000; whether edits to an already-over file count as new issues is UNVERIFIED | Story 0.2.4 AC (probe at the Wave 4 gate), conditional Task 0.2.4f (carve `guardedProcedures` into `server/guarded_procedures.go`), Task 2.1.2f/ADR-031 keep the server.go edit to one line |
| TR2-E7 | Re-grep `guardedProcedures` line refs | `grep -n` on `server/server.go`: map `:1879-1888` (was cited `:1879-1891`), `localChain` `:1896-1899`, `localChainWith` `:1901-1915` (was `:1900-1915`), `remoteChain` `:1933`, `otelhttp` `:1536` and `:1965` (unchanged), Watch bridge `:499-509` (was `:498-509`) | Updated in plan.md, ADR-031 and pre-mortem.md |
| TR2-P1 | Numeric decision rule for the 4-week adoption review | requirements.md Adoption bullet and ADR-035 section 5 had none | ADR-035 section 5 "Adoption review rule" with five owner-editable constants and verdicts (inconclusive / keep v2 / editing proven / tried-not-adopted / retire-or-rework), minimum volume caveat for one user, endpoint `review` block (Task 2.5.3a), page review card, requirements.md, Story 2.5.1 |
| TR2-P2 | Analytics coalescing (30/min) vs adoption counts | ADR-035 section 4 (token bucket, coalescers, 200-event drop-oldest queue) | Adoption counts for saves and searches use unthrottled server events; client events are lower bounds with the `dropped` count shown (ADR-035 section 5, Task 2.5.3a) |

**Declined / adjusted**: nothing declined. Adjustments: (1) the brief's "make Esc-then-Tab the primary vim escape" cannot work with vim on (Esc is the mode switch and Tab is a vim motion in Normal mode), so the primary documented exit is the status-bar Leave editor control reached by Esc, Esc, with Esc-then-Tab kept for vim off; (2) browser-reserved status of chords is recorded as UNVERIFIED knowledge and proven only by the owner's per-browser manual check, because Playwright injects events below the accelerator layer; (3) the Wave 4 `server.go` item is a probe with a conditional carve-out (Task 0.2.4f) rather than an unconditional move, to avoid a mechanical move that the lint may not require; (4) the archived-session server refusal reason is deferred to the Task 0.1.3b recon; (5) assistive-technology use is not auto-detected (browsers do not expose it reliably), so the Accessible view is an explicit toggle with a skip link; (6) the 260 px / 200 px keyboard heights behind the layout budget are assumptions until the Story 0.1.5 spike measures them.

### Triad round-3 repair (final triad-repair pass; plan, ux, ADR and validation edits only, no code)

Every finding was checked against the docs and, for engineering items, against the code before editing; the evidence column says how. Counts recounted by script (`eff.py` over the effort-table rows and `scan.py` over the `Task`/`Story` headings and `Files` lines): 12 epics, 47 stories, 133 tasks; raw 11.08M tokens, 10.30M CU before verification, 18.04M CU after the x1.75 multiplier, band XL (> 15M CU), about 20% above the Large ceiling. Cost classes only; no human-time units. New task ids: 0.1.5d, 1.1.2d (conditional), 2.2.2d, 2.2.2e; re-classes: 1.5.1c S to M, 1.5.4b S to M, 2.1.2f XS to S; new ux ids: U-MOB-16, U-MOB-17, U-TAB-11, U-EDIT-22, U-EDIT-23, U-CONF-11, U-DRAFT-6, U-CHORD-2 (123 `U-*` ids now); new validation id: REQ-38.

| Id | Finding | Verified how | What changed |
|---|---|---|---|
| TR3-E1 | Wave 1 shared files unserialized; scan covered Waves 2-9 only | Ran a script over every task's `Files:` line and every story's `**Files**` line with waves from the wave list: 62 files named by more than one task, 24 same-wave cross-story collisions, among them `files-viewer.spec.ts` (0.1.1/0.1.4a/0.1.6a), the spike page (0.1.5a/0.1.5c/0.1.6a) and the checklist (0.1.5b/0.1.6b) in Wave 1 | New "Full serialization scan, all waves" block in the wave section with an explicit order for every collision in every wave (Wave 1 added: 0.1.1 -> 0.1.4a -> 0.1.6a; 0.1.5a -> 0.1.5c -> 0.1.6a; 0.1.5b -> 0.1.5d -> 0.1.6b; plus Wave 4-8 orders the earlier list lacked, e.g. `file_writer.go`, `file_write_test.go`, `SearchPanel.tsx`, `draftStore.ts`, `DiscardDialog.tsx`, `AccessibleView.tsx`) |
| TR3-E2 | Dependency diagram lacks the 0.1.6 -> 1.5.4 -> G1, 0.2.4f and 2.1.2g edges and 1.5.4-before-1.5.3c | Diagram showed only the G1 edge and the FilesTab lane; the rest was wave prose | "Edges added by triad round 3" block inside the diagram: 0.1.6 -> 1.5.4a -> 1.5.4b -> 1.5.3c (G1); 1.5.1c -> 1.5.4b; 1.5.4 before 1.5.3c; 1.1.2d -> Wave 4 gate -> 0.2.4f -> 2.1.2d -> 2.1.2f -> 2.1.2g -> 2.1.3a; 0.1.3b -> 2.1.2g; 2.2.2 and 2.3.2 task chains |
| TR3-E3 | 1.5.3c / Story 1.5.3 do not mention the Accessible view and screen-reader quick check G1 includes | Read Story 1.5.3 AC, Task 1.5.3c and the Wave 6 prose: only the prose said it | Story 1.5.3 AC and Task 1.5.3c state that G1 includes the Accessible view and an R1/R2 quick check against the built component; wall-clock blockers updated |
| TR3-E4 | 1.5.4b includes the Edit hand-off, but Edit mode is built in Wave 7; 1.5.4b also touches palette, ADR-035 enum, Overflow, routing | Read Story 1.5.4, Task 1.5.4b and 2.2.1b: Edit mode (2.2.1b) is Wave 7, so the Wave 6 AC was unsatisfiable; `toggle_accessible_view` is already in ADR-035's palette `command` enum (line 46), so `events.ts` needs no edit | Hand-off AC moved to Story 2.2.1 / Task 2.2.1b (Wave 7); Story 1.5.4 keeps a stub assertion (`onEditRequested` seam renders a button only when passed); Task 1.5.4b re-classed S to M, Files list now includes `QuickOpenPalette.tsx` and `AccessibleView.test.tsx`; effort table row updated (260k raw) |
| TR3-E5 | 2.1.2f must cover both `localChain` and `remoteChain` and say how a `MaxBytesReader` error becomes the typed refusal | Read `server/server.go`: `guardedProcedures` is used only by `localChainWith` (`LocalWriteGuard`), `remoteChain` never applies it; both chains share `srv.mux`; the handler is wrapped at `:~495-497` and registered with `http.StripPrefix("/api", handler)`; read connect-go v1.20.0 `error.go:452-466`: `*http.MaxBytesError` becomes `CodeResourceExhausted` before any handler or interceptor runs | Wrapper moves to the handler level (one line covers both listeners, path matched as the stripped path); **contract changed**: over-limit body -> `ResourceExhausted` with no detail (a typed `TooLarge` detail cannot be attached because the handler never runs), client maps it to `Rejected(TooLarge)`; Story 2.1.2 ACs, refusal table, Task 2.1.2f (XS to S, new `bodylimit_test.go`), Task 2.2.1a, ADR-031 updated |
| TR3-E6 | ADR-030 path (b) WS-bridge registration edits `server/server.go` in Wave 4 before the 0.2.4f probe, unowned | Read ADR-030 decision 4 and the Wave 4 gate: path (b) needs an exact-path registration at `server.go:499-509` plus the selector entry; no task listed it | Conditional Task 1.1.2d (S, Wave 4, first on `server.go`, before the length probe and 0.2.4f); ADR-030 decision 4 and the effort table updated |
| TR3-E7 | Order 2.3.2e before 2.3.2f; spec file for 2.4.2c Playwright named-viewport cases | 2.3.2f's "wider screen" fallback needs the `phoneMergeAvailable` predicate defined in 2.3.2e; 2.4.2c's Files line lacked any spec although validation puts the named-viewport e2e in `files-mobile.spec.ts` | 2.3.2e exports the predicate and runs before 2.3.2f (stated in both tasks and the diagram); Task 2.4.2c Files gains `tests/e2e/files-mobile.spec.ts` |
| TR3-U1 | Breakpoint contradiction: 768px container rule gives the landscape phone the desktop layout but 7.1a/U-MOB-2/7/14 test L-390 with the mobile ladder | Read ux section 0 vs 7.1a: confirmed | One rule (`chooseFilesLayout`): mobile when container < 768 **or** (coarse pointer and `min(screen.width, screen.height)` < 600). `screen`, not the visual viewport, so a tablet's keyboard does not flip it; threshold 600 is an INFERRED adjustable constant. ux sections 0, 7.1a, 7.2, 7.3, U-MOB-2/7/16, U-TABLET-1; Story 1.5.1 ACs and Task 1.5.1c (S to M); Story 1.5.3 Playwright cases; glossary |
| TR3-U2 | Collapsed priority-1 banner (36 px single line) must keep Copy my version / Retry Save at 320 px with >= 44 px targets | Read 7.1a step 2: it collapsed the banner to text only | Banner row is 44 px (rows list and both table rows recomputed: S-844 452 px, XS-568 vim+2 banners 132 px); collapsed priority-1 row keeps two 44x44 buttons; "N notices" chip has a 44x44 hit area; U-MOB-14, U-EDIT-15, Story 2.4.2 AC |
| TR3-U3 | Esc direction contradiction and collisions with vim Esc / Esc,Esc | ux section 2 and 7.4 rule 5 said Esc goes editor -> results; 3.3 step 6 and U-SEARCH-9 said Esc goes results -> editor | One Esc table in ux 8.4 by focus location and vim state; editor -> results is **Ctrl+Shift+F**, results -> editor is Esc; layers swallow the second Esc; sections 2, 3.3, 7.4, 8.4, 15.2, U-SEARCH-9, U-EDIT-8/10/23, ADR-033 |
| TR3-U4 | 9.3 "Diff too large" row half-edited and offers Overwrite without a visible diff | Read the row: stray "Download both versions is not available", Overwrite listed | Summary state with Overwrite and Merge disabled until **Show full diff** is used; stray text removed; U-CONF-11, Story 2.3.1 AC and Task 2.3.1b |
| TR3-U5 | Esc,Esc trap-escape fires on habitual double-Esc; 8.4 "reachable by Tab order" wrong with vim on | Reasoned from ADR-033 (Tab is a vim key in Normal mode) | Armed gesture: only from vim Normal with nothing pending and a visible chip "Esc again to leave the editor"; per-device setting to disable; leaving is undone by one Esc/Enter; `:leave` ex command as a second keyboard route; false Tab-order claim replaced by the real route list; one-time status when vim is first turned on; Story 2.4.1 ACs, Task 2.4.1b (new `extensions/escLeave.ts`), ADR-033, ADR-035 `ex_leave` |
| TR3-U6 | Back from S8: Discard merge choices returns to S7 or closes the sheet, 9.1a vs 10.1 | Read 9.1a, section 10 table, 10.1, U-CONF-9 | Esc/system back/`<` from S8 never closes the sheet: no resolutions -> S7; resolutions -> Discard prompt, Discard returns to S7; Back to diff keeps choices; 9.1a, section 10, U-CONF-9, Story 2.3.1 AC |
| TR3-U7 | `editLayoutBudget` must take the resolved InputProfile; mid-edit flip to Hardware with the top bar auto-hidden | Read 7.1a step 3 and 8.8: the cluster lives on the key bar, which a Hardware profile hides | Signature `editLayoutBudget(viewport, keyboard, inputProfile, flags)`; step 3 only while the key bar is shown; same-pass top-bar restore on a flip; matrix dimension and U-MOB-17; ux 7.1a, 8.8, 12; Story 2.4.2, Task 2.4.2c, glossary |
| TR3-U8 | Accessible view with a dirty buffer; copy R1-R4 into ux.md | Read 8.10 and Story 1.5.4: silent on dirty state; R1-R4 only in plan | Renders the draft, "unsaved changes" name and label, 44 px Save, banners above; R1-R4 copied into ux 8.10; U-EDIT-22, U-A11Y-9, Story 1.5.4 AC, Task 1.5.4a |
| TR3-U9 | Clean preview tab in Edit can be LRU-evicted | Read 8.1/8.4/Story 1.2.2: promotion happened on the first edit | Edit entry pins; ux 8.1/8.4, U-EDIT-9/17, glossary `BufferKind`, Story 1.2.2 AC, Tasks 1.2.2a/2.2.1b, ADR-032 |
| TR3-U10 | Dirty buffers over 256 KB are memory-only and iOS discards backgrounded PWAs | Read ADR-032, Story 2.2.2, ux 11.1: the over-cap case was a chip plus the in-memory hold | **Decision**: two storage tiers behind `DraftBackend`: localStorage (<= 256 KB, synchronous flush) and a new IndexedDB tier for 256 KB up to the 1 MB cap (async, 2 s debounce plus a `pagehide` start, 5 entries, 24 h); priority-1 "Draft not stored" banner replaces the chip when neither tier works; justification: localStorage is synchronous, small-quota and shared with tab-layout keys; the owner's veto covers both tiers. New Task 2.2.2d; Story 2.2.2 ACs, ux 8.2/11.1/11.3, U-DRAFT-3/6, ADR-032, ADR-035 enum, validation |
| TR3-U11a | Section 2 buffer-status list omits Loading/Failed | Read ux section 2 | Buffer arm line added |
| TR3-U11b | Overlay should show physical-key labels for non-US layouts / AltGr | Reasoned from `event.code` matching; AltGr is Ctrl+Alt on many Windows layouts | AltGr guard in the matcher and layout-resolved labels (`getLayoutMap()`, Chromium only; fallback text elsewhere); ux 14.1/15.2, U-CHORD-2, Story 1.3.5 AC, Tasks 1.3.5b/1.3.5c |
| TR3-U11c | Tablet touch profile lacks Quick-open and Go-to-line entries | Read 7.3: no bottom bar and no Ctrl chords on a soft keyboard | Breadcrumb-row toolbar buttons; ux 7.3, U-TABLET-1, Story 1.5.1 AC, Task 1.5.1c |
| TR3-U11d | Close others with several dirty tabs: Save semantics | Read 11.1: one dialog, no Save rule | Sequential Save all, pause at first failure, Stop; ux 11.1a, U-TAB-11, Story 2.2.2 AC, new Task 2.2.2e |
| TR3-U11e | Android system Back with soft keyboard up | Reasoned from platform behavior; UNVERIFIED on the owner's device | ux 7.4 rule 6, U-MOB-4, Story 1.5.1 AC, owner script line (Task 2.5.4a) |
| TR3-U11f | State the < 360 px no-merge limit in ux.md and requirements | Read section 10 and requirements | ux section 19 item 8; requirements.md Constraints |
| TR3-P1 | Stale "(original list)" open questions in requirements.md | Read requirements.md lines 93-102: all five were resolved by Phase 2 | Block deleted; "Still open" list corrected (the 2 s benchmark is Story 1.1.3, the Connect size limit is answered by Task 0.1.3b / 2.1.2f) |
| TR3-P2 | Name the second risky assumption; alternatives; low-adoption action | Read requirements "Feasibility Risks" and "Alternatives Considered": one assumption named, two alternatives, no action | Second assumption (browser editing preferred over terminal Vim/IDE on desktop) named with mitigation; SSH/terminal-on-phone and Tailscale code-server added with the reason this scope is chosen (labelled INFERRED where not evidenced); verdict-to-action table (flag editing off, keep read-only features, delete write path later, retire v2 last) in ADR-035 and Story 2.5.1 |
| TR3-P3 | Rationale for adoption thresholds; time-to-answer baseline in the 0.1.5 spike | ADR-035 section 5 gave constants without sourcing | Rationale paragraph (judgment, not measurement; edit before reading the verdict) in ADR-035 and requirements; new Task 0.1.5d (XS owner baseline of three timed trials) and Story 0.1.5 AC |

**Declined / adjusted**: nothing declined. Adjustments: (1) TR3-E5 changed an earlier contract, not just a location: the over-limit refusal cannot carry the typed `TooLarge` detail because Connect rejects it before the handler runs, so the wire contract is `ResourceExhausted` plus a client mapping; (2) TR3-E4 moved the Edit hand-off instead of faking it in Wave 6, and kept a stub assertion so the seam is tested early; (3) TR3-U10 adds IndexedDB only for the 256 KB to 1 MB range (the synchronous localStorage tier is kept for the pagehide-safe small case) and uses a hand-written `indexedDB` stub rather than a new dev dependency; (4) TR3-U1's 600 px short-side threshold, the 1.5 s chip, the 2 s backup debounce and the 5-entry store are INFERRED constants, adjustable in one place each; (5) TR3-U5 keeps Esc, Esc (owner-requested earlier) but arms and undoes it rather than removing it, and a user can turn it off; (6) Android Back with the keyboard up, `getLayoutMap()` coverage outside Chromium and iOS `pagehide` IndexedDB timing are UNVERIFIED and are owner-script lines, not CI claims.

### ADR-036 integration (owner decisions 2026-10-16; plan, ux, ADR, validation, requirements, pre-mortem and research edits only, no code)

Owner decisions integrated: (1) large-file editing (T1, delta-only) is included; (2) CodeMirror 6 with a lazy gap-line document behind the `EditorEngine` port, with a custom engine only if CM6 fails the S-T3 phone gate; (3) every adapter (`full`, `connect_unary`, `connect_stream`, `ws_stream`, `http_range` behind the rebinding guard) and `ApplyFilePatch` on `:8444` subject to Task 0.1.3b; (4) cost approved with phasing A, B, C, then D last and gated on the spike results. Counts recounted by script (`count2.py` over the headings, `verify.py` over the effort table and Files lines, `scan.py` over every task's `Files:` line): **14 epics, 65 stories, 196 tasks; raw 17.03M tokens, 15.83M CU before verification, 27.71M CU after the x1.75 multiplier; band XL**; +2 epics (1.6, 2.6), +18 stories, +63 tasks (one conditional, Task 1.6.4d), +5.95M raw tokens, +9.68M CU. The contingent Epic 1.7 (6 stories, 26 tasks, 4.59M CU) is excluded from every base figure. Cost classes only; no human-time units. Validation: 46 requirement ids (REQ-39..REQ-46 added), 325 mapping rows plus the REQ-09 placeholder, 145 `U-*` ids with 145 UX rows (parity between ux.md section 18 and validation.md checked by script). Review artifacts `adversarial-review.md`, `architecture-review.md` and `consistency.md` are historical records and were not edited.

| Id | What changed | Where |
|---|---|---|
| A36-1 | ADR-036 marked **Accepted**, the four owner decisions recorded inline, phasing and plan-effects text updated to the integrated ids, consequences cost corrected (+9.7M CU integrated against the +6.2M top-down; contingent engine about 4.6M, not 6M), open questions split into resolved and still-open, "Spike outcomes" section added | `decisions/ADR-036-...md` |
| A36-2 | Plan front matter: status, **Size** (14/65/196, 27.71M CU), ADR list; Domain Glossary +11 rows (`FileTransport`, `FileTransportRouter`, `FileOps`/`FileBackend`, `FileTransportCapabilities`, size tiers, `Buffer.source`, engine ports, `Edit`/`ApplyFilePatch`, `PatchRejectReason`, the three flags) and amended `FileVersionToken` and `Draft` rows; Pattern Decisions +3 rows; Tech Debt +1 row (`ServeFileRaw`); Observability Plan transport line; Risk Control: transport flags (global-only, default off), rollback, three accepted/controlled risks (delta-write silent mangling, T1 unknown outcome after restart, CM6 scroll/gap on iOS); Unresolved Questions +5; Deferred +1 | `plan.md` header to "Deferred" |
| A36-3 | Dependency diagram: an "ADR-036 edges" block for Phases 0, A, B, C, D and the contingent epic, plus the "Added by the ADR-036 integration" id list | `plan.md` Dependency Visualization |
| A36-4 | Phase 0: Story 0.1.3 AC and Task 0.1.3b (remote-tier patch-body question, `ServeFileRaw` GET-guard audit); Story 0.1.4 AC and **Tasks 0.1.4c (M), 0.1.4d (XS, owner)** (extended S-T3); **Stories 0.1.7 (S-T1, S-T2) and 0.1.8 (S-T4, S-T5)** with Tasks 0.1.7a/b, 0.1.8a/b | `plan.md` Epic 0.1 |
| A36-5 | Phase A: Story 0.2.2 AC and Tasks 0.2.2a/b (three transport flags); Story 0.2.3 pointer AC; Story 0.2.5 AC and **Task 0.2.5d**; **Story 0.2.6** (Tasks 0.2.6a-d); Story 1.2.1 AC and **Tasks 1.2.1f, 1.2.1g** (+ Task 1.2.1a note); Story 1.2.2 AC and Task 1.2.2c; **Story 1.2.3** (Tasks 1.2.3a-c) | `plan.md` Epics 0.2, 1.2 |
| A36-6 | Amendments to existing stories (no new classes): Task 1.1.1a (proto fields `large_files`, `byte_offset`, `large_files_skipped` added early), Task 1.3.4a (pending-analysis compare), Story 1.5.2 AC (windowing replaces degrade over 1 MiB), Task 1.5.3c (informational large-file line), Story 1.5.4 AC and Task 1.5.4a (`PageSource`), Story 2.1.2 (cap-scope AC, Task 2.1.2b ports now on `FileOps`, Task 2.1.2f second path), Story 2.1.4 (delta corpus AC), Task 2.2.1a (`transport.writeFull`), Story 2.2.2 (ChangeSet drafts AC), Story 2.2.3 (lost response with deltas), Task 2.3.2c (`MergeSurface` note), Story 2.4.1 (capability-gated vim AC), Story 2.5.1 (transport gates AC), Story 2.5.2 (ordered flag flips AC), Story 2.5.3 and Task 2.5.3a (transport metrics block) | `plan.md` Epics 1.1-2.5 |
| A36-7 | Phase B: **Epic 1.6** (Stories 1.6.1-1.6.7, 21 tasks: sparse line index and `Analyze`, `ReadFileRange` and `FileOps.ReadRange`, `ConnectUnaryTransport` reads with chunk cache and prefetch, `cm6LazyDoc` gap-line module (+ conditional window-swap Task 1.6.4d), windowed viewer and states and the Accessible view through the transport, large-file search, e2e/benchmarks/owner check). A separate `Analyze`-benchmark task from the first outline was folded into Task 1.6.7b | `plan.md` after Epic 1.5 |
| A36-8 | Phase C: **Stories 2.1.5** (`ApplyFilePatch` server, 6 tasks), **2.2.5** (encoder, sync loop, ChangeSet drafts, delta UX, 6 tasks), **2.3.3** (`PreflightFilePatch`, same-position rebase, `MergeSurface`, 3 tasks), **2.5.5** (e2e matrix, registry/docs, large-edit dogfood, 3 tasks) | `plan.md` Epics 2.1, 2.2, 2.3, 2.5 |
| A36-9 | Phase D: **Epic 2.6** (Stories 2.6.1-2.6.3, 8 tasks: `ReadFileRangeStream` and WS bridge entry, `ConnectStreamTransport`, `WsStreamTransport`, `PoolBudget`, `http_range` GET route behind the rebinding guard and the `ServeFileRaw` guard); Gate G-D | `plan.md` after Story 2.5.5 |
| A36-10 | **Contingent Epic 1.7** (custom rope/piece-table engine: 6 stories, 26 tasks with cost classes and Files lines, activation rule, 4.59M CU), clearly marked and excluded from the base totals | `plan.md` after Epic 2.6 |
| A36-11 | Effort estimate: table rows regenerated for all 65 stories (rows 0.1.4, 0.2.5, 1.2.1 re-costed, 18 added), Totals paragraph rewritten with the by-phase delta table, the comparison with the approved top-down estimate and the absorbed-items list, contingent table; waves 1-9 extended and **Wave 10** added; re-verification paragraph for the ADR-036 placements; gate paragraph (three conformance suites join the gate; Gates G2, G3, G-D); overrun checkpoint extended; **ADR-036 serialization scan** (95 files named by more than one task, 39 same-wave cross-story collisions, an explicit order for every one that involves an ADR-036 task, plus the cross-wave `session.proto` order); merge-conflict lanes for `CodeEditor.tsx`; wall-clock blockers (S-T3 phone gate, `ApplyFilePatch` remote decision, S-T2 constants, Task 1.6.7c, Gates G2/G3/G-D, two planner-choice confirmations) | `plan.md` Effort estimate |
| A36-12 | ADR amendments: ADR-030 decision 7 (request-scoped cap, `byte_offset`, shared limiter), ADR-031 (editable cap qualified; amendment bullet), ADR-032 (`Buffer.source`, drafts as ChangeSets, edit-volume caps), ADR-033 (vim is an engine capability), ADR-034 (documents rebased, not deltas; T1 same-position rebase; `MergeSurface`), ADR-035 section 7 "Transport extension" (labels, six events, `RecordPatch`, `files.perf.open_ms` `tier`) | `decisions/ADR-030..035` |
| A36-13 | ux.md: S6 row, state-model lines, 8.10 windowed Accessible view, **8.12 large-file states**, **8.13 delta-save states**, **9.4 T1 conflict**, 11.3 ChangeSet drafts, 13 status-bar segments, 17 exit-path rows, 18 new "Large files and delta saves" criteria (**U-LARGE-1..11, U-PATCH-1..10**) and **U-OBS-5**, 19 risks 10-12 | `design/ux.md` |
| A36-14 | validation.md: REQ-39..REQ-46, 64 mapping rows (golden vectors, property, fuzz, conformance, fault injection, benchmark and e2e rows), 22 UX rows, Test Stack and Coverage Targets additions, recounted Counts, Gaps item 15 | `implementation/validation.md` |
| A36-15 | requirements.md: owner-decisions section, large-file success metric, Size (current figure and the 18.04M history), wall-clock blockers, constraints (flags, limits), NFR performance note, scope, risks, observability, risk control, open questions, the 31% editing-share figures recomputed (29%, 8.08M of 27.71M); pre-mortem.md rows 17-21 and two P1 items; research/transport.md: status note, section 8.1 and 11.5 CU figures replaced by the integrated ones (top-down kept in a column), section 8.2 ids, section 10 answers | `requirements.md`, `implementation/pre-mortem.md`, `research/transport.md` |
| A36-16 | Self-review (below) | all |

**Self-review results (one adversarial pass, scripted where possible)**: no `@@` placeholders; no duplicate story or task ids; effort-table rows equal the story headings for task counts, class lists, raw tokens and CU (65 of 65, sum 27,714k); every `Task x.y.zA` and `Story x.y.z` reference in the plan, ux, validation, requirements, ADRs, pre-mortem and transport.md resolves to a heading; no task has more than 5 files in its `Files:` line; U-id parity between ux.md section 18 and validation.md (145 and 145); REQ definitions 46 and rows for 46. Fixed during the pass: a duplicated `**Files**`/task header from an insertion, a wrong `SearchPanel.tsx` path (it lives under `components/sessions/files/`), analytics label names that disagreed between the plan and ADR-035 (`fallback_reason`, `reason` values, an invented `conflict_resolve` event replaced by the existing `save_conflict`), a stale "197-task" count, "Waves 2 to 9" in two ACs and the validation REQ-28 row, the 31% editing-share figures, and 1 MB edit-cap statements that needed qualification (ADR-031, ADR-032, ADR-034, validation REQ-13, the glossary and the `SessionBufferHold` row).

**Declined / adjusted / flagged**: (1) **"Inside Waves 0-1"** is read as Phase 0 and Phase 1 of this plan (the plan has no Wave 0, and the seam must wrap the Wave 2 resolver and Wave 3 codec), so Phase A lands in Wave 3 (engine ports in Wave 2); recorded in Wave 3 and ADR-036. (2) **Flag defaults**: ADR-036 was silent on `files:range-read` and `files:delta-write`; all three default **off** with a stated flip order (Task 1.6.7c, Gate G2, Gate G3) - a planner choice the owner may change (Unresolved Questions). (3) **Phase D gating** is implemented as "spike results decide default enablement of the stream adapters, the code is built either way" because the owner selected the adapters and said not to downscale; the original ADR text ("only if the spikes show a miss") would have made them optional; `http_range` never ships without its guard. (4) **Cost**: the bottom-up total is about 2.5M CU (about 34%) above the approved top-down estimate; reported in the Totals paragraph, not trimmed. (5) Phase C **code starts after Gate G1** (as instructed) but the flag flips wait for **Gate G2** (the Task 2.5.4a dogfood signed), because the client delta path hooks into the Wave 7 editing code and "full-content editing proven" needs the Wave 7-8 build; the ADR's wording "Gate G1 dogfood" conflated the two gates. (6) S-T5 runs in Phase 0 on throwaway endpoints (the real `SearchContent` and range load do not exist yet) and is re-measured in Task 1.6.7b; S-T3 is split into a CI prototype (Task 0.1.4c) and an owner gate (Task 0.1.4d). (7) `FileWriter` is reconciled with `FileOps` as its write-side collaborator rather than replaced (Task 2.1.2b amended), to keep the Story 2.1.2 ACs and tests intact. (8) Patch refusals use a sibling enum `PatchRejectReason` rather than extending `ReadOnlyReason` (which describes why a file cannot be edited, not why a request was refused). (9) The `large_files`/`byte_offset` proto fields are added in Task 1.1.1a (Wave 4) so `session.proto` is not edited a fourth time in Wave 6. (10) Three whole-document-mode and staged-commit questions of ADR-036 were not asked of the owner and are recorded as defaults (not built). **UNVERIFIED** (no code run in this pass, by design): every CodeMirror capability of the gap-line model (INFERRED from public APIs until Task 0.1.4c/0.1.4d), compression behavior of connect-web (S-T1), throughput and the 64 MiB / 1 GiB constants (S-T2), pool contention (S-T5), the remote-tier answer for `ApplyFilePatch` and the `ServeFileRaw` GET-guard state (Task 0.1.3b), and the connect-go `MaxBytesError` mapping for the new 2 MiB path (carried over from Task 2.1.2f's VERIFIED reading, not re-run for the new path).
