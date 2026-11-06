# Research: Feature Landscape — files-viewer-revamp

Confidence labels: VERIFIED = read/grepped in this worktree; INDUSTRY = background knowledge of the named tools (not re-fetched this session); INFERRED = my reasoning.

## 1. Existing code to reuse or learn from (VERIFIED unless noted)

| Asset | What it gives us | Gap / caveat |
|---|---|---|
| `web-app/src/components/sessions/FileContentViewer.tsx` (693 lines) | CodeMirror 6, read-only (`EditorState.readOnly.of(true)`), hand-rolled `vimKeymap` (`gg`, `/`, `n`, `N`), wrap compartment, change-gutter marks from `parseDiff.ts` | Not `@replit/codemirror-vim`; no edit state, no `gotoLine`, no hunk nav |
| `FilesTab.tsx` / `FileTree.tsx` (970) / `QuickOpenPalette.tsx` | Two-pane layout, tree sort/filter/git badges, filename quick-open, `recentPaths` | Recents are `useState` only (FilesTab.tsx:46) — lost on reload; single open file |
| `server/services/file_service.go` (889) | `ListFiles`, `GetFileContent`, `SearchFiles` (name only), `.gitignore` matcher, `os.Lstat`, symlinks listed non-expandable, binary detection (ext allowlist, MIME, null-byte), 10 MB reject / 1 MB truncate | Resolves root via `ws.ExistingDir` (lines 131, 298, 460, 723) — see Risk R1 |
| `web-app/src/components/files/LocalFileBrowser.tsx` (509) + `server/services/local_file_service.go` (176) | Plain HTTP `GET /api/local/files/list?path=`, `GET /api/local/serve/<abs path>`; 2000-entry cap | No CodeMirror, no vim, no session context; arbitrary absolute paths (outside the session-confinement model) |
| `components/backlog/BacklogFileBrowserModal.tsx` (113) | Small modal file picker with focus-return tests | Picker, not a viewer; reuse only its focus-return pattern |
| `components/sessions/DiffViewer.tsx`, `components/shared/DiffRenderer.css.ts`, `lib/utils/parseDiff.ts` | Unified-diff rendering + parsing already exist | Needed for the conflict dialog (save-vs-disk diff) — reuse `DiffRenderer`, don't build a new one |
| `lib/hooks/useKeyboard.ts` (102), `components/ui/KeyboardShortcutOverlay.tsx`, `app/settings/KeyboardShortcutsTab.tsx` | App-level shortcut hook, `?` overlay, settings tab | New viewer shortcuts must register here or they will be invisible in the overlay and may collide with page-level keys while CodeMirror has focus |
| `@codemirror/search` ^6.6.0, `@codemirror/commands` already in `web-app/package.json` | `gotoLine`, in-file search, history/undo | `@codemirror/vim`/`@replit/codemirror-vim`, `@codemirror/autocomplete`, `@codemirror/merge` are NOT installed (grep of package.json) |
| `SessionVcsContext.tsx` + `onNavigateToFile` (per ux-review.md) | Diff-panel to file cross-link | Reuse as a "open at line" entry point |
| `rg` | Present on this host (`/home/linuxbrew/.linuxbrew/bin/rg`) | Not guaranteed on all hosts (requirements already flag this) |

Prior plans (read): `project_plans/files-tab-ux-review/ux-review.md` already lists the same gaps (no jump-to-line, no outline, no cross-file content search, no hunk nav), so those are confirmed owner pain points, not new inventions. `better-file-tree/` and `mobile-ux-improvements/` research dirs exist (architecture/features/pitfalls/stack; decisions/) — planning should cross-check them for already-decided tree/mobile choices rather than re-deciding.

## 2. Industry landscape (INDUSTRY)

- **VS Code web / github.dev**: global search panel (grouped by file, collapsible, regex/case/whole-word toggles, include/exclude globs, "replace"), tabs with preview (italic, single-click reuses tab) vs pinned, breadcrumbs + outline, Ctrl+P quick-open with `:line` and `@symbol` suffixes, dirty dot on tab, "file changed on disk" prompt with Compare/Overwrite, auto-save option, local-history/undo. github.dev is edit-and-commit only (no terminal), the closest analogue to this scope.
- **code-server**: full VS Code; heavy, poor on phones (tiny targets, soft-keyboard issues) — matches the requirements' "Alternatives Considered" rejection.
- **Mobile editors (Working Copy, Textastic, Termux+vim, Replit mobile)**: the consistent pattern is an *extra-key row above the soft keyboard* (Esc, Tab, brackets, arrows, undo/redo, Ctrl) because soft keyboards lack these; Working Copy/Textastic put file switching in a swipe/drawer rather than a persistent tree. Modal vim on a soft keyboard is a known pain (autocorrect/IME composition fights `hjkl`; no Esc key) — Termux users rely on a toolbar Esc or `jk`/Ctrl-[ remaps.
- **Vim**: `:e`, `Ctrl-^` (alternate file), `Ctrl-o/Ctrl-i` jumplist, `gf`, `:N` goto line, `*`/`#`, `[c`/`]c`-style change nav, `:grep`/quickfix list (`:cn`/`:cp`) — quickfix is exactly the "search results panel + next/prev hit" model.
- **ranger**: Miller columns, preview pane, `/` filter, bookmarks (`m`/`'`), tabs, `gh`/`gg` jumps — relevant idea: preview-on-select and bookmarks; not worth copying columns on a phone.

## 3. Unstated needs beyond the explicit requirements (INFERRED, grounded in the above)

1. **Dirty-buffer indicator and unsaved-change guard** (tab dot, `beforeunload`, confirm on tab close / session switch).
2. **Preview vs pinned tabs** so single-click browsing doesn't spawn dozens of tabs on a phone.
3. **Search results as a first-class, keyboardable list** (quickfix-style next/prev hit, grouped by file with match counts, truncated-results banner, "searching..." cancel). Needs request cancellation/streaming-or-paged shape in the proto, not one giant response.
4. **Staleness awareness for read-only buffers too**: agents write constantly, so an *open, unedited* file should refresh (or show "changed on disk — reload") — otherwise the viewer silently shows old content; the requirements only cover staleness on save.
5. **Soft-keyboard extra-key row** (Esc, Tab, undo/redo, arrows, `:`-command entry) — the Vim toggle is unusable on phone without Esc.
6. **Persisted workspace state** per session: open tabs, active file, cursor/scroll, recents (currently lost on reload).
7. **Deep links**: `?file=path&line=N` so the mobile app, notifications, and the diff panel can open a hit directly.
8. **Save feedback and undo**: toast with bytes written, and keep CodeMirror undo history across a successful save.
9. **Line-ending and trailing-newline preservation** as an explicit, tested contract (see section 4).
10. **Read-only reason display**: tell the user *why* editing is disabled (flag off, binary, truncated, too large, paused session, symlink).

## 4. Edge cases and failure modes the design must handle

| # | Case | Required behaviour |
|---|---|---|
| R1 | **Deleted worktree on paused/archived session.** `ExistingDir` falls back to the repo root when the worktree dir is gone (rule file `instance-lock-free-reads.md`: "two sessions in separate cleaned-up worktrees share an ExistingDir"). Existing read RPCs use `ExistingDir` (file_service.go:131,298,460,723). | The write RPC must NOT use `ExistingDir`; resolve against `Workspace().WorktreeDir`/`ActiveDir`, and refuse (FailedPrecondition) if that dir is absent or the session is paused/archived. Otherwise an edit lands in the **main checkout**. This is the highest-severity finding. Reads should also label "showing repo root, worktree removed". |
| R2 | Agent writes mid-edit | Optimistic concurrency token returned by `GetFileContent` and echoed by `WriteFile`. Prefer content hash (sha256 of bytes) + size over mtime alone: mtime has coarse granularity and changes on `touch`/checkout without content change (false conflicts), and an agent can rewrite within the same mtime tick (missed conflicts). Compare-and-write must be atomic w.r.t. other writers as far as possible: write to temp file in same dir + `rename`, re-stat/hash immediately before rename. Residual TOCTOU window cannot be closed without cooperative locking from the agent — document it. |
| R3 | Conflict UX | Block save; show 3-way-ish view: my buffer vs current disk (reuse `DiffRenderer`); actions: overwrite, reload (discard mine), keep mine as unsaved copy. "Merge" in the requirements implies a real merge UI — `@codemirror/merge` is not installed; scope decision for planning. |
| R4 | Atomic write semantics | Temp+rename loses permissions/ownership/xattrs and breaks hard links/symlink-as-target unless explicitly preserved (copy mode bits; write through resolved target for symlinks). Existing `settings_file_atomic_write.go` is a precedent to read before writing a second implementation. |
| R5 | Symlinks | Today: listed, non-expandable (file_service.go:193-238). Write path: reject writing through a symlink whose resolved target escapes the worktree (`EvalSymlinks` + prefix check after resolving, and re-check immediately before write to narrow TOCTOU); also reject `..` and absolute paths. Decide whether in-tree symlink targets are editable (recommend: read-only with reason, simplest safe default). |
| R6 | Binary / non-UTF-8 | Binary detection exists and returns metadata only. Editing must be disabled for binary, truncated (`is_truncated`), and >limit files — otherwise saving a 1 MB-truncated buffer **destroys the rest of the file**. Non-UTF-8 text (Latin-1, UTF-16): response currently says "UTF-8 file content"; invalid bytes would be replaced on round trip. Safest: detect non-UTF-8 and open read-only, or carry original bytes/encoding. Preserve BOM. |
| R7 | CRLF / mixed endings / no trailing newline | CodeMirror normalizes line breaks to `\n` internally; saving must re-apply the detected original EOL (and handle mixed files, ideally by refusing or preserving per-line). Otherwise every save of a CRLF file produces a whole-file git diff. |
| R8 | Huge files on phone | 1 MB cap is ok for read; for editing set a lower cap (suggest <=1 MB, tunable) and disable syntax highlighting/wrapping above a threshold. Long single lines (minified JS) freeze CodeMirror even when small — add a max-line-length guard. Search results: cap and show "N more not shown". |
| R9 | Unsaved changes on pause/archive/kill/session switch/service restart | Pause deletes the worktree (rule file). Client must warn on in-flight dirty buffers when the session state changes; consider a local draft (IndexedDB) so a closed tab or service restart doesn't lose work. Server-side: write RPC rejects with a clear code, client keeps the buffer. |
| R10 | Search: ignored/binary/huge | Default respect `.gitignore` (requirement); skip binary and >N MB files; cap per-file matches and total; regex DoS — with `rg` (finite automata) safe, with an in-process `regexp` (RE2) also safe; never use a backtracking engine. Timeout and context cancellation on client disconnect. Don't search `.git/`. Query text must not be logged at info level (requirement). |
| R11 | ripgrep availability | Check at startup; fall back to in-process walk + RE2 (the existing `SearchFiles` already walks with a gitignore matcher). The repo's `prefer-go-git-over-subshells` skill argues against subprocesses, but go-git has no content grep over the *working tree* — the in-process walker is the fallback, `rg` the fast path; planning must decide and justify. |
| R12 | Git index / agent interaction | Saving only touches the working tree; no `git add`. Concern is the agent's own tools: Claude Code's Edit requires it to have Read the file recently and fails if mtime changed — a user save may make the agent's next edit fail once and retry (benign, but worth a user-visible note). File watchers (fsnotify) used elsewhere may fire on save — verify no feedback loop with the viewer's own refresh. |
| R13 | Permissions | Read-only file (mode 0444) or no write permission: surface EACCES as a clear error, not a generic failure. |
| R14 | Multi-device | Phone and desktop editing the same file: the same optimistic-concurrency token covers it; tab state persisted in localStorage is per-device (server-side persistence would make "continue on phone" work — see Decision D2). |
| R15 | Auth / ownership | Requirements say "respect session ownership"; mirror whatever `FileService` does today and add the write RPC to the same interceptor path; remote-access (`:8444`, private CA) means writes go over the network — keep auth identical to existing RPCs. |

## 5. Decision: unify the two file browsers?

**Recommendation: do not merge the UIs now; share the plumbing, and stop investing in LocalFileBrowser.**

Evidence (VERIFIED): they differ in model, not just look.
- Session Files tab: ConnectRPC (`ListFiles`/`GetFileContent`/`SearchFiles`), scoped to a session worktree, gitignore-aware, git-status badges, CodeMirror viewer.
- `LocalFileBrowser`: raw HTTP `/api/local/*`, arbitrary absolute paths, no session, no CodeMirror (per ux-review.md and file listing; 509 lines).

Reasons not to unify:
1. The security model is opposite: the revamp's write RPC *depends* on confinement to a worktree; LocalFileBrowser is deliberately unconfined (read-only). Folding them together risks leaking the unconfined path model into the write path.
2. Requirements scope is "session Files tab"; unifying would add an open-ended migration.
3. Cheap sharing is available: extract the tree rows, file-icon, size/format helpers, and the CodeMirror viewer component into `components/files/` shared pieces so `/files` can later render file content through the same viewer (read-only) — a follow-up, not part of this epic.

Follow-up option (INFERRED): if the owner rarely uses `/files`, consider deprecating it in favor of "browse any directory" as a read-only root mode of the session viewer. That needs an owner call; flag as Open Question O1.

## 6. Recommended answers to the requirements' open questions (inputs for planning, not final)

- Content search backend: `rg --json` fast path with in-process RE2 walker fallback (R10, R11); both behind one Go interface.
- Concurrency token: content hash + size (R2). Return it on every `GetFileContent`; `WriteFile` requires it; no token = reject.
- Vim mode: default off on mobile (no Esc), user toggle persisted; on desktop default off unless the owner states otherwise (existing Files tab already ships vim-style `gg`, `/`, `n` keys, which suggests a Vim-leaning owner) — owner decision, flagged as a wall-clock blocker. Adding real vim means a new dependency (`@replit/codemirror-vim`); verify bundle size impact and lazy-load it.
- Tab state: localStorage per session for v1 (cheap, matches "per-viewer convenience"); server-side only if "continue on phone" is wanted (D2). Wrap storage in try/catch.
- Create/rename/delete: out of scope; revisit only after write RPC and confinement tests exist — the path-confinement code is the same, so marginal cost is mostly UI and git-status refresh, but delete is the data-loss-prone one.

## 7. Open items for the planner
- O1 Owner: keep or retire `/files` LocalFileBrowser?
- O2 Owner: Vim default, and extra-key toolbar contents.
- O3 Does "merge" in conflict resolution mean true 3-way (add `@codemirror/merge`) or just overwrite/reload/keep-mine?
- O4 Verify before implementing: how `ExistingDir` vs `WorktreeDir` is exposed on `Workspace()` (session/types.go) for the write path.
- O5 Cross-check `better-file-tree/decisions` and `mobile-ux-improvements/decisions` for settled choices.
