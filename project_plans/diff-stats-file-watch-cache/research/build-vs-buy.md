# Build vs. Buy: recursive-watch + gitignore-filter + debounce

## Context

The feature needs: (1) recursive directory watching layered on `fsnotify` v1.9.0
(already a direct dep, `go.mod:28`), (2) `.gitignore`-aware filtering, (3)
new-subdirectory detection, (4) event debouncing — to invalidate
`GetSessionDiff`'s diff-stats cache and `GetVCSStatus`'s `vcsStatusCache`.

This repo already hand-rolls direct `fsnotify` usage (no wrapper) in
`session/unfinished/watcher.go` (`fsnotify.NewWatcher()` at line 30,
`event.Has(fsnotify.Write)`-style event handling at line 156) and
`session/unfinished/scanner.go`. `go.mod` lists ~53 direct dependencies —
no unusually strict cap, but the repo's ADRs show a repeated preference for
zero-new-dependency solutions when one is workable: `docs/adr/012-react-virtuoso-log-viewer.md`
explicitly weighs "custom lightweight virtual scroller (zero new dependency)"
as an option, and `docs/adr/ADR-024-no-new-diff-rendering-library.md` rejects
a library specifically because "it adds a new dependency." No document states
a hard rule against new deps, but the pattern favors avoiding one when the
existing primitive already covers most of the need — which is the case here
since `fsnotify` is already vendored and already used directly, unwrapped, in
an analogous file-watching subsystem.

## 1. Existing OSS library/framework

| Library | Last push | Stars/issues | License | Recursive? | gitignore? | Debounce? |
|---|---|---|---|---|---|---|
| `github.com/rjeczalik/notify` | 2026-06-01 | 936★ / 46 open issues | MIT | Yes — native OS recursive watch on macOS (FSEvents)/Windows (ReadDirectoryChangesW); watchpoint-tree-managed manual recursion on Linux (inotify)/kqueue | No | No |
| `github.com/farmergreg/rfsnotify` | 2024-08-25 | 44★ / 1 open issue | MIT | Yes — thin wrapper adding `AddRecursive`/`RemoveRecursive` on top of fsnotify | No | No |
| `github.com/sean9999/go-fsnotify-recursively` | 2024-03-19 | 3★ / 0 issues | MIT | Yes, plus glob filtering | No (glob only, not `.gitignore` semantics) | No |

(Verified via `gh api repos/<owner>/<repo>` for `pushed_at`/`archived`/`license`/`stargazers_count`/`open_issues` — not archived, MIT-licensed for all three.)

None of these solve the gitignore-filtering requirement — that's a separate,
orthogonal problem all three leave to the caller. `rjeczalik/notify` is the
only one under active maintenance (pushed within the last quarter as of this
research) and has meaningfully broader adoption; the other two are small,
single-maintainer, largely dormant wrappers (rfsnotify) or near-abandoned
(go-fsnotify-recursively, 3 stars, no activity since March 2024).

Even `rjeczalik/notify`, the most viable candidate, only replaces the manual
`Add()`-per-directory walk — it doesn't touch gitignore filtering, debounce,
or new-subdirectory detection, all of which still need hand-written code on
top regardless of which recursion mechanism is chosen. Its recursion is also
platform-inconsistent: true native recursive watches only on macOS/Windows;
on Linux (this project's primary dev platform per `CLAUDE.md`'s "Manjaro/Ubuntu
Linux primary") it still walks and adds inotify watches per-directory
internally — the same fd-budget exposure as hand-rolling, just hidden behind
an API. Adopting it swaps one dependency for the *same* correctness burden
(gitignore, debounce, new-subdir handling) plus a new API surface + a second
low-level watcher abstraction living alongside the repo's existing direct
`fsnotify` usage.

**Verdict: Not recommended** for any of the three. `rjeczalik/notify`
specifically: **Viable but not recommended** — solves less than half the
problem (recursion mechanics only) while adding an architectural
inconsistency with `session/unfinished/watcher.go`'s established direct-fsnotify
convention, for a benefit (native recursion on non-Linux) that doesn't matter
much for a Linux-primary dev tool.

## 2. SaaS/managed API

Not applicable — this is a local filesystem watch on a developer's own
worktree; no managed cloud file-watching service can observe local disk
events for an internal dev tool with no network exposure, so there is no
real SaaS option to evaluate here.

## 3. LLM-generated implementation vs. battle-tested library

The pitfalls research (per this task's brief) flags three real correctness
traps for hand-rolled recursive fsnotify: `Add()` failing partway through a
deep tree walk (partial-watch state), watch descriptor leaks when a watched
directory is deleted out from under the watcher, and event-coalescing edge
cases (rapid create+write+rename sequences). These are genuine risks whether
the recursion mechanics are hand-rolled on raw `fsnotify` or delegated to
`rjeczalik/notify` — none of the candidates in §1 handle gitignore filtering
or debouncing, which is most of this feature's actual complexity, so "adopt a
library" doesn't buy safety against the two traps that matter here
(gitignore-aware pruning during the walk, debounce coalescing) and only
partially buys safety against the third (watch-descriptor lifecycle on
delete, and even there `rjeczalik/notify` has its own documented Windows gap
around deleted-directory events).

Given:
- The recursion+debounce+gitignore-filter code is a bounded, well-scoped
  problem (single package, no external state, testable with a temp-dir
  fixture) — not the kind of open-ended surface where hand-rolling
  correctness is hard to verify.
- The repo already has both building blocks needed for gitignore filtering
  (`go-git`'s `gitignore.Matcher`/`gitignore.ReadPatterns`, used in
  `session/unfinished/gogit_vcs_reader.go`) and direct fsnotify usage
  (`session/unfinished/watcher.go`) to model the new watcher after — this
  is an extension of an established in-repo pattern, not a fresh design.
- No candidate library removes the two traps most specific to *this*
  feature (gitignore pruning, debounce), so the "adopt a library" case
  reduces to "let a dependency replace `filepath.WalkDir` + `Add()` calls" —
  a small slice of the total logic.

**Verdict:** Hand-rolling on raw `fsnotify`, matching
`session/unfinished/watcher.go`'s convention, is **Recommended**. The
correctness traps are real but addressable with targeted unit tests (delete a
watched subdir mid-test and assert no descriptor leak; nested-gitignore
fixture; rapid-fire event burst asserting single coalesced invalidation) —
cheaper than absorbing a second file-watching abstraction whose only
advantage (native OS recursion on macOS/Windows) doesn't cover this feature's
harder problems and don't matter for a Linux-primary tool.

## 4. Fork or adapt

No single-file, no-framework recursive-watch helper worth vendoring emerged
from the search — `rfsnotify` and `go-fsnotify-recursively` are already thin
(a few hundred lines each) but neither is meaningfully smaller or safer than
writing the walk+`Add()` loop directly against `fsnotify`, and vendoring a
near-abandoned single-maintainer repo (no activity since 2024, 44 and 3 stars
respectively) trades a small amount of boilerplate for an update/security
burden with no upstream maintenance to rely on. No explicit written policy in
this repo caps dependency count or forbids forking third-party code (beyond
the general "don't commit generated code" and ADR pattern favoring
zero-new-dependency where workable) — this wasn't pursued further per the
task's own guidance not to over-invest here.

**Verdict: Not recommended.**

## Final Recommendation

Build the recursive-watch + gitignore-filter + debounce logic directly on
the already-adopted `fsnotify` v1.9.0, following the direct-usage convention
in `session/unfinished/watcher.go` (no wrapper). Concretely:
- Recursive watch: `filepath.WalkDir` + per-directory `watcher.Add()`, with
  `fsnotify.Create` events on directories triggering incremental `Add()` for
  new subdirectories (the "new-subdirectory detection" requirement).
- Gitignore filtering: reuse `go-git`'s `gitignore.Matcher`/`ReadPatterns`
  already wired up in `session/unfinished/gogit_vcs_reader.go`, applied both
  when walking to decide which directories to `Add()` and when filtering
  incoming events before they trigger cache invalidation.
- Debounce: a per-worktree timer (reset on each qualifying event, invalidate
  caches once quiescent) — the same pattern documented in the community
  fsnotify debounce tutorials, no library needed.
- Zero new dependencies; add targeted tests for the three documented pitfalls
  (deep-tree partial `Add()` failure, deleted-directory watch-descriptor
  leak, rapid-event coalescing) rather than relying on a third-party library
  to have solved them generically.

No new dependency is added.
