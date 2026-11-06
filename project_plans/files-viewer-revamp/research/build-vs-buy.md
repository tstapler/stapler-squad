# Build vs Buy: files-viewer-revamp

Date: 2026-10-16. Confidence labels: VERIFIED (read in this repo or run), INFERRED (training knowledge / search snippet, not opened at primary source), UNVERIFIED.

## Starting position (VERIFIED from `web-app/package.json`, `go.mod`)

Already dependencies: `codemirror ^6.0.2`, `@codemirror/{view,state,commands,language,search}`, 8 `@codemirror/lang-*` packages, `@lezer/highlight`, `@tanstack/react-virtual`, `react-arborist ^3.4.3`, `@monaco-editor/react` + `monaco-editor ^0.55.1` (Monaco is imported in `app/config/ConfigPageContent.tsx`, not in the Files tab), React 19, Next 15.
Not present: `@replit/codemirror-vim`, `@codemirror/merge`, any gitignore/ripgrep wrapper.
Go: `go-git/v5` (replaced by the `tstapler/go-git` fork), `sergi/go-diff v1.4.0`, `fsnotify`, `bmatcuch/doublestar v1` (indirect only). `rg` exists on this dev host (`/home/linuxbrew/.linuxbrew/bin/rg`) but nothing guarantees it on every deployment host.
Existing Files tab: `FilesTab.tsx` (296 lines), `FileTree.tsx` (970), `FileContentViewer.tsx` (693); proto has `GetFileContent` and `SearchFiles` only (no content search, no write).

## 1. Existing OSS libraries

| Option | Pros | Cons | Verdict |
|---|---|---|---|
| `@replit/codemirror-vim` (vim mode for CM6) | Only serious CM6 vim; targets CM6 only; active (Socket lists ~21K weekly downloads, 6.2.1 to 6.3.0 published; INFERRED from search snippets, check npm for the exact current version). MIT (INFERRED). Small, loaded as an extension in a `Compartment`, so toggling is trivial. Already have `gg` hand-rolled in viewer, which it replaces. | Modal editing on iOS/Android soft keyboards is poor (no Esc key, IME composition quirks); must pair with a touch toolbar (Esc, Ctrl, arrows). Fidelity gaps vs real Vim (macros, some ex commands). Adds weight to desktop-only path. | **Recommended** for desktop, default off, lazy-loaded via dynamic import; mobile gets standard keymap + toolbar |
| `@codemirror/search` | Already a dependency. In-file find/replace panel, regex/case/word. | Single-document only; default panel is not touch-sized, needs custom panel theme. | **Recommended** for in-file find (wrap panel styling) |
| `@codemirror/merge` (`MergeView`, `unifiedMergeView`) | Official CM6 package, same maintainers as CM; gives side-by-side and unified diff with revert controls: exactly the save-conflict UI (mine vs on-disk). Version reports conflict across snippets (6.10 to 6.12.x), so pin after checking npm. | Side-by-side is unusable at phone width; use `unifiedMergeView` on mobile. Adds one dep (small). Not a 3-way merge; "merge" = pick hunks. | **Recommended** for conflict diff (unified on mobile) |
| `react-arborist` | Already a dependency and imported by `FileTree.tsx`. Virtualized, keyboard nav. | Touch/long-press support is weak; tree already works, so no reason to swap. | **Recommended** to keep (no change) |
| `@tanstack/react-virtual` | Already a dependency; right tool for virtualizing the search-results list. | None material. | **Recommended** for results panel |
| Monaco (`monaco-editor`, `@monaco-editor/react`) | Already in tree; built-in find, diff editor, minimap, multi-cursor. | Multi-MB bundle, workers needed; Monaco officially does not support mobile browsers (INFERRED, long-standing upstream statement); would make CM and Monaco both loaded in Files tab; vim needs separate `monaco-vim`. | **Not recommended** for Files tab (mobile is the primary driver) |
| `@codemirror/lsp-client` | Official LSP client. | LSP is out of scope per requirements. | Not recommended (out of scope) |
| ripgrep subprocess | Fastest by far on 50k files, gitignore semantics correct, binary detection, JSON output (`--json`). | Not guaranteed installed on host; repo has an explicit preference for go-git over shelling out (`prefer-go-git-over-subshells` skill); subprocess lifecycle/cancellation/arg-injection hardening needed (user query must go after `--`/`-e`). | **Viable** as an optional accelerator, not the only path |
| In-process Go walker (`filepath.WalkDir` + `regexp` + go-git gitignore matcher) | No external binary; works everywhere; cancellable via ctx; fits repo preference. go-git ships `plumbing/format/gitignore` (a maintained matcher implementing gitignore pattern semantics, INFERRED from library knowledge; confirm in the fork's vendor/module cache). Go's RE2 `regexp` guarantees linear time (no ReDoS). | Slower than rg (single-threaded unless a worker pool is added); must hand-roll binary sniffing, size caps, symlink policy, parallelism. 2 s on 50k files needs a worker pool and probably a cap on files/bytes (UNVERIFIED, must benchmark). | **Recommended** as the primary/baseline backend |
| `go-git` worktree grep (`Worktree.Grep`) | In-library. | Greps tracked files at HEAD or the index via git objects, not the dirty working tree; agents write uncommitted files constantly, so results would be stale/wrong (INFERRED; confirm). | **Not recommended** |
| `git grep` subprocess | Respects gitignore for tracked files, `--untracked` option. | Same subprocess objection; `--untracked` semantics differ from rg. | Not recommended (rg or in-process cover it) |

Recommended search architecture: one `Searcher` interface with an in-process implementation (always available) and an optional ripgrep implementation selected when `rg` is on PATH. Planning decides whether the second backend earns its cost; the in-process one must be built regardless because rg may be absent.

## 2. SaaS / managed API

Hosted editors (StackBlitz WebContainers, CodeSandbox, GitHub Codespaces/github.dev, Sourcegraph Cloud search) require the worktree contents to leave the machine or require a GitHub-hosted repo. Sessions run on uncommitted local worktrees written by agents, and the owner reaches the app over LAN/Tailscale. Cannot index uncommitted local state. **Not recommended.** (The requirements list no formal data residency rule, so the objection is functional, not compliance: the product cannot see local dirty files.)

## 3. LLM-generated code vs battle-tested library

Reckless-to-hand-roll list (use a library or a vetted primitive; LLM-written code is acceptable only as glue and must be test-driven):

| Concern | Position | Why |
|---|---|---|
| gitignore semantics (negation, `**`, dir-only patterns, nested `.gitignore`, `.git/info/exclude`, global excludes) | **Use library**: go-git `plumbing/format/gitignore` (already a dependency tree), or defer to `rg` | Semantics are subtle (last-match-wins, parent-dir exclusion cannot be re-included). A hand-written glob matcher will silently over- or under-include. Add table tests that compare against `git check-ignore` as an oracle. |
| Path confinement for write RPC (traversal, symlink escape) | **Use stdlib primitive**: `filepath.EvalSymlinks` on the parent + prefix check, or Go 1.24+ `os.Root` (module is on Go 1.26.6, VERIFIED `go.mod`), which refuses escapes at the syscall layer | A string-prefix `strings.HasPrefix` check is the classic bug. `os.Root` removes the TOCTOU class. Security-sensitive: review adversarially. |
| Atomic write (temp file in same dir, fsync, rename, preserve mode, preserve symlink target) | **Small custom code over a vetted pattern**, or `google/renameio` (INFERRED: maintained, Linux/macOS-focused; verify before adopting) | About 40 lines; the failure modes (cross-device rename, mode loss, partial write, follow symlink) are known and testable. Custom is acceptable if tests cover mode preserved, crash-between-write-and-rename leaves original intact, symlink handling. Windows is not a target. |
| Concurrency token | **Custom, simple**: content hash (SHA-256) of the bytes the client loaded, compared at save time under a per-file lock; also return size+mtime for display | mtime alone misses same-second agent writes; git blob id equals a hash anyway and costs a subprocess/go-git call. Note the check-then-write window: do compare-and-rename under one process-wide per-path mutex (`instinct_check_then_reserve_needs_process_wide_lock` in memory). Agents writing outside the server can still race between check and rename; the residual window must be stated, not hidden. |
| Diff for conflict UI | **Use library**: `@codemirror/merge` client-side (it diffs in the browser); server only supplies on-disk content. Go `sergi/go-diff` is already present if a server-side diff is wanted. | Diff algorithms are well-trodden; do not write one. |
| Regex engine for search | **Use stdlib `regexp` (RE2)** | Linear-time guarantee; avoids ReDoS from user-supplied patterns. |
| Binary/encoding detection, CRLF round-trip | **Custom, but minimal and conservative**: NUL-byte sniff in the first 8 KB (git's heuristic) plus UTF-8 validity; detect dominant line ending on load and re-apply on save; refuse to edit non-UTF-8 | Cheap to write; the risk is silent corruption, so refuse rather than guess. Property test: load then save without edits yields identical bytes. |
| Search walker (concurrency, cancellation, caps, result streaming) | **Custom glue** over the primitives above | No library fits the RPC shape; LLM-assisted code is fine but needs `-race` tests, ctx-cancel tests, and a benchmark on a large tree. |
| Tab state persistence, UI wiring | Custom | Low risk. |

Rule of thumb for this project: custom is reckless wherever a wrong answer is silent and security- or data-loss-relevant (ignore matching, path confinement, regex engine, diff). Custom is fine where failures are loud and covered by round-trip/property tests (atomic rename wrapper, line-ending preservation, hash token).

## 4. Fork or adapt

| Option | Pros | Cons | Verdict |
|---|---|---|---|
| code-server / openvscode-server / VS Code web embedded per session | Full-featured editor, search, vim extension, git UI. | Separate Node server process per workspace or shared; hundreds of MB; auth/proxy through the existing remote-access path; VS Code web is not touch-first and mobile support is limited (INFERRED); contradicts requirements' scope (no extensions/IDE) and own "Alternatives Considered". Breaks tight integration with session identity, git-status badges, feature flags. | **Not recommended** |
| github.dev / vscode.dev | Zero hosting. | Needs GitHub-hosted repo; cannot see local dirty worktree (see section 2). | Not recommended |
| Adapt existing `FilesTab` / `FileTree` / `FileContentViewer` | CM6 + tree + virtualization already in bundle; existing unit tests (`__tests__/FileTree.test.tsx`, `FileContentViewer.test.tsx`) and e2e give a regression net (success metric 5); incremental, shippable behind flags. | `FileTree.tsx` (970 lines) and `FileContentViewer.tsx` (693) are already large; adding tabs/buffers/editing there will worsen them, so extract a buffer-store hook and an editor component rather than growing the files. Single-file state is baked in and needs a multi-buffer model. | **Recommended** (extend, with refactor-as-you-go) |

## Recommendation

Build on existing code, buy small CM6 extensions, hand-write only the glue.

- Frontend: keep CodeMirror 6 + react-arborist + react-virtual; add `@replit/codemirror-vim` (lazy, desktop toggle) and `@codemirror/merge` (conflict diff, unified on mobile). Do not use Monaco or an embedded IDE.
- Backend: new `SearchContent` (in-process RE2 walker with go-git gitignore matcher as baseline; optional ripgrep fast path) and `WriteFile` (hash-token compare-and-swap, `os.Root` confinement, atomic rename). Hand-written code is limited to this glue and carries race, cancel, traversal and round-trip tests.
- Open validation items for planning: benchmark in-process search against the 2 s / 50k-file target; confirm `go-git/plumbing/format/gitignore` handles nested `.gitignore` and info/exclude in the forked go-git; confirm exact latest versions and licenses of `@replit/codemirror-vim` and `@codemirror/merge` on npm; real-device check of vim mode on iOS (Esc key).

## Sources

- Repo: `web-app/package.json`, `go.mod`, `proto/session/v1/session.proto` (lines 302-310), `web-app/src/components/sessions/`.
- [@replit/codemirror-vim on Socket](https://socket.dev/npm/package/@replit/codemirror-vim/overview/^6.0.3), [README 6.3.0](https://cdn.jsdelivr.net/npm/@replit/codemirror-vim@6.3.0/README.md) (search snippets only; versions not confirmed at npm).
- [@codemirror/merge on npm](https://npmjs.com/package/@codemirror/merge), [repo](https://github.com/codemirror/merge) (search snippets; version listings conflicted).
