# Architecture Research: backlog-scaffolding-gitignore

Agent 3 (Architecture). Root cause and existing architecture are pre-confirmed in
`requirements.md` — this doc answers the five integration-point questions from the
task brief, grounded in call-site tracing, not re-derivation of the bug.

## 1. Blast radius of `addWorktreeExcludes` / `selfHealWorktreeScaffolding`

Grep across `session/*.go` for both names (full results below) confirms this is a
**single-function fix**, no other call site independently resolves `--git-dir`:

```
session/backlog_commands.go:35   WriteSlashCommands        -> selfHealWorktreeScaffolding(worktreePath)
session/backlog_commands.go:179  WriteBacklogContextFile   -> selfHealWorktreeScaffolding(worktreePath)
session/backlog_commands.go:241  addWorktreeExcludes(worktreePath)          <- the buggy `--git-dir` resolution
session/backlog_commands.go:290  selfHealWorktreeScaffolding(worktreePath)  -> UntrackScaffolding(...) then addWorktreeExcludes(...)
```

`selfHealWorktreeScaffolding` has exactly 2 callers (`WriteSlashCommands`,
`WriteBacklogContextFile`), both in `session/backlog_commands.go`, both already routed
through the one `addWorktreeExcludes` definition. No other file in the repo greps for
`git rev-parse --git-dir` or duplicates this resolution logic — confirmed via a
repo-wide grep for `--git-dir` (only hit is inside `addWorktreeExcludes` itself) vs.
`--git-common-dir` (the one other hit is the pre-existing, correct precedent at
`session/git/util.go:153`, see §5). Fixing `addWorktreeExcludes`'s one `git rev-parse`
invocation is sufficient; no parallel fix is needed elsewhere.

## 2. `IsWorktreeDirty` callers and where a new function plugs in

Two callers only, both boolean-only "gate" checks that don't currently render a path
list to a human/agent in a way that would benefit from one at the same call site:

- `server/mcp/tools_backlog.go:385` — `request_review`'s belt-and-suspenders layer 1.
  **This is the one in scope** (requirements.md Scope item 2): its rejection message is
  the blanket `git add -A && git commit` string that needs to become path-specific.
- `session/review_gate.go:153` — belt-and-suspenders layer 2, inside the automated
  review-gate flow. It only sets a static `uncommittedWarning` string prefix
  (`"[WARNING: worktree has uncommitted changes — ...]\n"`) prepended to the diff shown
  to the reviewer LLM — no per-path detail is rendered here today, and
  requirements.md's Scope explicitly limits this item to `request_review`'s message.
  **Leave this call site as `IsWorktreeDirty` — do not touch.**

`session/review_queue_determiner.go`'s two "uncommitted changes" mentions
(`applyWorktreeCheck`, lines ~80 and ~247) are a **different mechanism entirely**: they
call `worktree.IsDirty()` on a `*git.GitWorktree` (`session/git/worktree_git.go:202`,
which shells to `git status --porcelain` through a TTL-cached/singleflight-coalesced
path, `IsDirtyWithHint`), not the free-function `session.IsWorktreeDirty`. This feeds
the omnibar attention-queue's informational "uncommitted changes ready to commit" flag
— unrelated to `request_review`'s rejection path and outside this item's scope per
requirements.md ("no behavior change... `IsWorktreeDirty`'s underlying mechanism").
**Recommendation: `GetWorktreeDirtyPaths` should be additive, not a replacement.**
`IsWorktreeDirty` keeps its two existing callers unchanged; the new function is called
only from `tools_backlog.go`'s `request_review` handler, alongside (not instead of) the
existing `IsWorktreeDirty` check — or `GetWorktreeDirtyPaths` could subsume the
existing `IsWorktreeDirty` call there (non-empty-slice implies dirty), but either way
`review_gate.go` and `review_queue_determiner.go` are untouched.

## 3. Where `GetWorktreeDirtyPaths` should live

**`session/backlog_review.go`, next to `IsWorktreeDirty`** — confirmed by the salvaged
`implementation/validation.md` (committed `c0bf31a69`), whose requirement→test mapping
table places `TestGetWorktreeDirtyPaths_*` tests in `session/backlog_review_test.go`
(package `session`, which already exists with 20 existing `Test*` functions, e.g.
`TestParseHeadlessVerdictResult_ValidJSON`, `TestBuildReviewPrompt_*`). This is also the
architecturally consistent choice: it's a same-package sibling of `IsWorktreeDirty`,
both operate on the same `worktreePath string` shape and both are consumed by the same
`request_review` caller. No reason to add a new `session/git/` export for this — the
existing `git` subpackage's boundary is "shells out to raw git plumbing for
worktree/commit lifecycle," while `IsWorktreeDirty`/`GetWorktreeDirtyPaths` are
review-flow-specific queries already living at the `session` package level.

**NUL-safe porcelain parsing already exists in this codebase — reuse it, don't
reimplement.** `session/vc/git_provider.go` has a fully NUL-safe, tested parser:
`GetChangedFiles()` runs `git status --porcelain=v2 -z --untracked-files=all` and
`parsePorcelainV2Z` (lines 213–312) parses it into `[]FileChange{Path, Status,
IsStaged, OldPath, ...}`, explicitly handling the rename/copy continuation-token case
(`"2 "` records followed by an extra NUL-delimited `origPath` token) that a naive
`strings.Fields`/newline-split parse gets wrong. `NewGitProvider(worktreePath)` +
`.GetChangedFiles()` is import-cycle-safe from the `session` package: `session/vc` has
no import of `session` or `session/git` (confirmed via grep), and is already imported
elsewhere (`server/services/search_service.go`, `session/workspace/*.go`), so adding it
to `session/backlog_review.go` introduces no cycle.

**This reuse recommendation is reinforced by direct evidence in the salvaged
validation.md**, not just architectural preference: its test-mapping table describes
`TestGetWorktreeDirtyPaths_HandlesRenamedFile` as covering "the bug fix called out
explicitly in the task brief (already patched once to fix a rename-parsing bug in
`GetWorktreeDirtyPaths`)" — i.e. the lost prior implementation apparently wrote its own
ad hoc parser and had to patch a rename bug in it. That failure already happened once;
reusing `session/vc`'s existing, tested `parsePorcelainV2Z`/`GetChangedFiles()` instead
of a second hand-rolled parser avoids repeating it. Recommended shape:

```go
// GetWorktreeDirtyPaths returns the specific list of dirty paths in worktreePath
// (staged + unstaged, deduplicated), or an empty slice for a clean worktree.
func GetWorktreeDirtyPaths(worktreePath string) ([]string, error) {
    provider, err := vc.NewGitProvider(worktreePath)
    if err != nil {
        return nil, fmt.Errorf("GetWorktreeDirtyPaths: %w", err)
    }
    changes, err := provider.GetChangedFiles()
    if err != nil {
        return nil, fmt.Errorf("GetWorktreeDirtyPaths: %w", err)
    }
    seen := make(map[string]struct{}, len(changes))
    var paths []string
    for _, c := range changes {
        if _, ok := seen[c.Path]; !ok {
            seen[c.Path] = struct{}{}
            paths = append(paths, c.Path)
        }
    }
    return paths, nil
}
```

(Dedup is needed because `GetChangedFiles` can emit two `FileChange` entries for the
same `Path` — one staged, one unstaged — per its own doc comment.) Note `GetChangedFiles`
also runs two `numstat` calls the message-formatter doesn't need; that's a minor,
acceptable cost for reusing tested parsing logic over hand-rolling a stats-free variant.

**Test fixture note for AC1/AC2 (real linked worktree):** no existing helper in
`session/*_test.go` creates a real `git worktree add` topology — `session/git/worktree_
creation_test.go`'s `setupTestRepo` and `session/git/worktree_git_stage_test.go`'s
fixtures are plain (non-worktree) repos. The closest existing precedent for a *linked*
worktree fixture is `server/services/backlog_service_triage_test.go` (`runGitTestCmd`
+ `git worktree add -b ...`, e.g. line 1490), but that's a different package. Within
`session`, `git.NewGitWorktree(repoPath, sessionName)` + `.Setup()` (`session/git/
worktree_ops.go:18`) performs a real `git worktree add` and is already imported by
`session/backlog_commands.go` as `git` — this is the natural building block for a new
`setupLinkedWorktree` test helper in `session/backlog_commands_test.go` or `session/
backlog_review_test.go`, rather than reinventing raw `git worktree add` shell-outs.

## 4. Where `formatDirtyPathsRejectionMessage` should live

**`server/mcp/tools_backlog.go`, unexported** — confirmed by the salvaged
validation.md's test mapping (`TestFormatDirtyPathsRejectionMessage_ListsEachPath` in
`server/mcp/tools_backlog_test.go`), and independently justified: it is a pure string
formatter with exactly one call site (`request_review`'s handler, the same file, lines
~381–392), consuming only `[]string` — it needs no access to `session` package
internals beyond the `[]string` `GetWorktreeDirtyPaths` already returns. Keeping it
unexported in the consuming file matches this repo's `interface-pollution-checklist.md`
guidance (don't export/relocate a helper with a single, same-file consumer). No
justification exists for a shared/exported location — nothing else in the codebase
needs a "dirty paths → rejection message" formatter today.

## 5. `prefer-go-git-over-subshells.md` — should `--git-dir`→`--git-common-dir` become go-git-native?

**No — keep it a subshell, and this repo already has a documented precedent for
exactly this call.** `session/git/util.go`'s `findMainRepoPathForWorktree` (lines
130–159) already resolves `--git-common-dir` via `safeexec.CommandContext("git",
"rev-parse", "--path-format=absolute", "--git-common-dir")` rather than go-git, with an
explicit doc comment explaining why: go-git's `PlainOpen` can succeed on a git-CLI-
created worktree while the subsequent `repo.Head()` call errors ("go-git struggling to
resolve HEAD for a git-CLI-created worktree" — "observed in production and reproduced
in a unit test"). That's precisely the failure-mode-naming bar
`prefer-go-git-over-subshells.md` itself sets for keeping a subshell ("don't fall back
'just in case' — name the specific failure the fallback exists for"). Since
`addWorktreeExcludes` needs the exact same `--git-common-dir` resolution against the
exact same linked-worktree topology, the safest and most consistent fix is to mirror
`findMainRepoPathForWorktree`'s existing subshell pattern (same flags, same
`--path-format=absolute` for good measure) rather than introduce a second, divergent
resolution path — one shells out, one uses go-git — for what should behave identically.
go-git v5.14.0 (this repo's pinned version, confirmed via `go.mod`/module cache) does
have some internal `CommonDir`-aware plumbing (`storage/filesystem/dotgit/
repository_filesystem.go`), so it is not flatly incapable — but nothing in this
repo exposes that through the public `go-git/v5` API surface (`Repository`/`Worktree`
methods) in a way that would let `addWorktreeExcludes` avoid a subprocess call, and
adopting it here would be inconsistent with the already-shipped, already-debugged
`findMainRepoPathForWorktree` precedent for no proven benefit. **Flagging for
cross-check with Agent 1 (Stack):** if Agent 1's research surfaces a specific go-git
v5.14 API that cleanly resolves common-dir without hitting the documented HEAD-
resolution bug, that would be new information worth reconciling with this
recommendation — but absent that, this repo's own prior debugging effort already
settled this question in favor of the subshell for this exact operation.

## Summary of recommended integration points

| Function | Location | Rationale |
|---|---|---|
| Fix (`--git-dir` → `--git-common-dir`) | `session/backlog_commands.go:244`, in place | Single call site, mirror `session/git/util.go:153`'s existing flags |
| `GetWorktreeDirtyPaths` | `session/backlog_review.go` (new func, next to `IsWorktreeDirty`) | Same package/consumer as `IsWorktreeDirty`; reuse `session/vc.NewGitProvider(...).GetChangedFiles()` for NUL-safe rename-correct parsing instead of reimplementing |
| `formatDirtyPathsRejectionMessage` | `server/mcp/tools_backlog.go` (new unexported func) | Single call site (`request_review` handler, same file), pure `[]string -> string` |
| Test fixture (`setupLinkedWorktree`) | `session/backlog_commands_test.go` or `session/backlog_review_test.go` (package `session`) | Build on `git.NewGitWorktree(...).Setup()` (`session/git/worktree_ops.go:18`), already imported as `git` in this package — no existing linked-worktree fixture in package `session` today |
| `review_gate.go` / `review_queue_determiner.go` | No changes | Out of scope per requirements.md; both use different mechanisms (`IsWorktreeDirty` static warning; `GitWorktree.IsDirty()` cached path) unrelated to `request_review`'s message |
