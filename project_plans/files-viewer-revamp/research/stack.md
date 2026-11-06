# Stack Research: files-viewer-revamp

Confidence labels: VERIFIED = command run or source read this session; INFERRED = reasoned, not measured.

## 1. Frontend (CodeMirror 6)

Already in `web-app/package.json` (VERIFIED, lines 45-58, 92): `codemirror ^6.0.2` (basicSetup), `@codemirror/{commands ^6.10.3, language ^6.12.3, search ^6.6.0, state ^6.6.0, view ^6.41.0}`, lang-{css,go,html,java,javascript,json,markdown,python,rust}, `@lezer/highlight 1.2.3`, `@tanstack/react-virtual ^3.13.25`, `@monaco-editor/react`/`monaco-editor` (not used by the Files viewer; do not extend), React 19, Next 15.3.2. `FileContentViewer.tsx` dynamically `import()`s CodeMirror pieces (lines 227-246), so new extensions should follow the same lazy-import pattern to keep them out of the main bundle.

Additions needed (versions from `npm view`, VERIFIED 2026-10-16):

| Package | Version | Purpose |
|---|---|---|
| `@replit/codemirror-vim` | 6.4.0 (2026-07-29) | Optional vim mode. Peer deps `@codemirror/{commands,language,search,state,view}` 6.x, all already present. |
| `@codemirror/merge` | 6.12.2 | `unifiedMergeView` / `MergeView` for the save-conflict diff (overwrite / reload / merge). |
| `@codemirror/autocomplete` | 6.20.3 | Optional; already a transitive dep of `codemirror` basicSetup. Only list explicitly if used. |
| `@codemirror/lint` | 6.9.7 | Not needed (LSP/diagnostics out of scope). |

Not recommended: `@uiw/react-codemirror` 4.25.12. The repo already drives `EditorView` imperatively with Compartments; a wrapper adds a second lifecycle for little gain.

Missing languages worth adding if wanted (INFERRED, not needed for MVP): `@codemirror/lang-yaml`, `lang-xml`, `lang-sql`, `@codemirror/legacy-modes` for shell/toml/dockerfile.

### Search, outline, navigation
- In-file find: `@codemirror/search` already present and used (`openSearchPanel`, `findNext`). Panel is keyboard-oriented; mobile needs a custom touch-sized panel via `search({createPanel})`.
- Outline/symbols: no new dependency. Walk the Lezer tree with `syntaxTree(state)` from `@codemirror/language` and match node names per language (FunctionDeclaration, ClassDeclaration, headings in markdown). No LSP.
- Tab/buffer state: keep one `EditorState` per open file and swap with `view.setState()` so scroll, selection, and undo history survive tab switches (state-level; scroll needs `EditorView.scrollSnapshot()` saved separately). Persist the open-tab list (paths + cursor line) in localStorage; do not persist buffer contents.
- Jump to line: `view.dispatch({selection, effects: EditorView.scrollIntoView(pos, {y:"center"})})`.

### Vim mode and mobile (research finding, mostly UNVERIFIED for this package)
- `@replit/codemirror-vim` registers a keymap with `Prec.high`, so it takes over keys including Esc. Issue #244 (open, Sep 2025) asks to run CM6 keymaps before vim.
- Issue #243 (open, Aug 2025): `{`/`}` motions fail on iOS with a Bluetooth keyboard. No reports found specific to soft keyboards in that tracker.
- Generic evidence from Obsidian: on Android, soft keyboards often cannot send Escape, and maintainers said the soft keyboard owns input text so it cannot be fixed in the vim layer. Sources: [replit/codemirror-vim issues](https://github.com/replit/codemirror-vim/issues), [Obsidian forum thread](https://forum.obsidian.md/t/on-android-using-vim-mappings-esc-sometimes-doesnt-take/101211), [CM Android widget keyboard bug](https://github.com/codemirror/codemirror.next/issues/602).
- Recommendation: vim mode is desktop-only and off by default (toggle in settings, loaded via dynamic import into a `Compartment`). On touch devices ship a toolbar (Esc, Tab, arrows, `{ } ( ) [ ] ;`, undo/redo, save, find) as the only "modal" affordance; never enable vim by default where `matchMedia("(pointer: coarse)")` is true. Keep an on-device check as the owner-blocker already noted in requirements.
- Vim ex commands `:w`/`:q` need explicit wiring via `Vim.defineEx("write", "w", cb)` to call the save RPC.
- iOS Safari keyboard layout: use `visualViewport` resize events (not `100vh`) to size the editor pane; `dvh` units as the CSS base. INFERRED from known iOS behaviour; verify in Playwright WebKit mobile emulation plus real device.

### Large files on mobile
Current server contract truncates at 1 MB and rejects over 10 MB (proto `GetFileContentResponse.is_truncated`). Editing a truncated buffer must be blocked (write would destroy the tail). For files over a threshold (INFERRED: ~300-500 KB on phones) open read-only, disable line wrapping and heavy extensions (bracket matching, highlight active line). CodeMirror virtualises rendering itself, so the cost is parse and document size, not DOM size.

## 2. Backend (Go)

go.mod (VERIFIED): `go 1.26.6`; `go-git/v5 v5.19.3` (replaced by `tstapler/go-git` fork, line 231); `fsnotify v1.10.1`; `golang.org/x/sync v0.22.0` (errgroup available); `bmatcuk/doublestar v1.3.4` (indirect, v1 API; glob filtering available without new deps); `lumberjack`. No ripgrep or renameio dependency.

Existing assets in `server/services/file_service.go` (VERIFIED by function listing): `resolveAndValidatePath` (line 106, traversal guard), `searchFilesInWorktree` (line 497) using go-git `gitignore.Pattern`s via `collectAllGitignorePatterns`/`loadGitignorePatterns` (lines 599, 644), `GetFileContent` (line 282). Atomic-write precedent: `server/services/settings_file_atomic_write.go` (`writeSettingsAtomic`, `lockSettingsPath`), reusable as a model.

### Content search: ripgrep vs in-process
- `rg` exists on this host (`/home/linuxbrew/.linuxbrew/bin/rg`, VERIFIED) but no Go code calls it (grep found none; the grep command errored on the glob flag so treat as UNVERIFIED for the whole tree), and the repo guidance prefers go-git over subprocesses. It is not guaranteed on other hosts (Brewfile/linux install is a separate step).
- Recommendation: hybrid behind one interface `ContentSearcher`.
  1. Primary: in-process walker reusing the existing gitignore pattern set and `doublestar` for glob include/exclude, parallelised with `errgroup` (bounded workers, e.g. GOMAXPROCS), `bufio.Scanner`/`bytes.Index` for literal and `regexp` (RE2, linear-time, safe against ReDoS) for regex, skipping files with NUL in first 8 KB and files over a size cap (e.g. 1-2 MB), streaming results with a hard cap on hits and wall time.
  2. Optional accelerator: if `exec.LookPath("rg")` succeeds, run `rg --json --max-count --max-filesize --glob` with `context.WithTimeout`, parse JSON lines. This gives the 50k-file < 2 s target far more reliably (INFERRED; neither path was benchmarked). Make it a swappable backend and log which backend ran.
  - Alternative for tracked files: `git grep` through a subprocess is also fast and ignore-aware, but violates the go-git preference and misses untracked agent output; not recommended as primary.
  - Streaming: ConnectRPC server-streaming (`stream SearchContentResponse`) lets the UI show first hits early and meets the "first results < 2 s" SLO even when the total scan is longer. Existing RPCs are unary, so check whether the Connect transport used by the web client handles server streams (INFERRED; it is standard for connect-es over HTTP/1.1 with `connect` protocol, but verify against current `useFileService` hook setup).
  - Never exec with the user's query as a shell string; pass as args after `--`/`-e`, and validate the regex with `regexp.Compile` first.

### Write path: atomic save
- Pattern: `resolveAndValidatePath` then `filepath.EvalSymlinks` on the target (and parent for new files) and re-check it is inside the worktree root; reject if final path escapes. Write to a temp file in the same directory (`os.CreateTemp`), `Sync`, `Chmod` to the original mode, `os.Rename` over the target, then fsync the directory. Same-dir rename keeps it atomic on POSIX.
- Alternative: `github.com/google/renameio/v2` implements exactly this (not in go.mod; optional, small). Hand-rolling matches the existing `writeSettingsAtomic` style and avoids a dependency.
- Caveat: rename replaces the inode, which breaks hard links, and changes ownership if the server runs as a different user than the file owner; preserve mode explicitly. Agent processes with the file open keep the old inode; acceptable, and arguably safer than in-place truncate-write.
- Encoding: round-trip bytes, not strings. Detect and preserve CRLF vs LF (CodeMirror normalises line breaks; set `EditorState.lineSeparator` from detection) and a UTF-8 BOM; reject non-UTF-8 and binary for editing. Existing proto reports `encoding` always "utf-8" and `is_binary`, so extend it with `line_ending` and `has_bom` or handle server-side on write.
- Post-save git interaction: writing a worktree file does not touch the index; git status badges only need a refresh of the existing status path. No index lock is taken, so no conflict with concurrent agent `git add`.

### Concurrency token options (for optimistic concurrency)
| Option | Pros | Cons |
|---|---|---|
| mtime + size | Free (a `stat`), no read | mtime granularity/clock quirks; same-size same-second edits missed; touched-but-unchanged false conflicts |
| Content hash (sha256 of bytes) | Exact; false conflicts impossible; works for untracked files | Needs full read (cheap under 10 MB cap) |
| git blob id (`git hash-object` equivalent) | Matches git's view | Same read cost as hash; sha1 vs sha256 repos; adds nothing for untracked files; go-git dependency for no gain |

Recommendation: return `version` (content sha256, hex) from `GetFileContent` and require `expected_version` in `WriteFile`. On write, under a per-path in-process mutex, re-read, hash, compare, then atomic-rename. Optionally also compare mtime+size as a fast-path precheck before the hash. This leaves a tiny TOCTOU window against external (agent) writers between hash and rename; unavoidable without OS file locks that agents will not honour. Document it and mention it in the conflict UX. Conflict response should return the current on-disk content and version so the client can render `@codemirror/merge` without a second round trip (cap response size).

## 3. Protocol surface (proto, `proto/session/v1/session.proto`, existing RPCs at lines 300-310)
New RPCs needed: `SearchContent` (streaming or unary with cap + `truncated`), `WriteFile` (`expected_version`, `content`, returns new version or `conflict` with current content). Extend `GetFileContentResponse` (currently content/encoding/is_binary/size/content_type/is_truncated, line ~2319-2335) with `version`, `line_ending`, `has_bom`. Regenerate via `make proto-gen`; update `docs/registry` via `make registry-generate`.

## 4. Open questions answered
- ripgrep vs in-process: in-process default with optional rg accelerator (see above); ignored files off by default, binary skipped by NUL sniff.
- Concurrency token: content sha256.
- Vim default: off; desktop-only toggle; mobile gets a toolbar.
- Tab persistence: localStorage for the tab list and cursor positions (no server round trip; single user); revisit server-side only if cross-device is wanted.
- Minimal create/rename/delete: out of scope here; the atomic-write helper makes create trivial, but delete/rename raise git-state and agent-race questions.

## 5. Gaps / not verified
- No benchmark of in-process vs rg on a 50k-file repo; first planning task should be a spike.
- Streaming RPC support through the existing web client transport not checked.
- Mobile vim behaviour inferred from third-party reports, not tested.
- `@codemirror/merge` bundle-size impact not measured; keep behind dynamic import.
