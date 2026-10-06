# Research: Pitfalls & Risks — backlog-scaffolding-gitignore

Agent 4 of SDD research phase. Scope: risks specific to the `addWorktreeExcludes`
`--git-dir` → `--git-common-dir` fix, the new dirty-paths-listing feature, and their
regression tests.

## 1. Silent-failure pitfall — CONFIRMED, name it explicitly

**Hypothesis confirmed.** `setupTestGitRepo` (`session/backlog_commands_test.go:20-45`)
builds its fixture with a single `git init` — a **plain repo**, never a linked worktree
(`git worktree add`). Every existing test that exercises `addWorktreeExcludes` /
`selfHealWorktreeScaffolding` (`TestWriteBacklogContextFile_UntracksPreviouslyCommittedContextFile`,
`TestWriteSlashCommands_UntracksPreviouslyCommittedSlashCommandFiles`, etc., all at
`session/backlog_commands_test.go:336-478`) runs against this plain-repo fixture. None of
them assert the *actual effect* the function exists to have (`git status` reporting the
worktree clean) — they only assert the untrack-from-index behavior of the *sibling*
function `UntrackScaffolding`, never `addWorktreeExcludes`'s exclude-file write itself.

**Why this matters mechanically:** in a plain repo, `git rev-parse --git-dir` and
`git rev-parse --git-common-dir` both resolve to the same path (`.git`), so a test built
on `git init` cannot distinguish "resolves the right directory" from "resolves the wrong
one" — the bug is invisible by construction. Only in a linked worktree
(`git worktree add`) do the two diverge: `--git-dir` → `<common>/.git/worktrees/<name>`
(worktree-private admin dir, whose `info/exclude` git does **not** consult), vs.
`--git-common-dir` → `<common>/.git` (the one `git status` actually reads). This is
exactly how the bug shipped once already undetected (per requirements.md's "prior work"
note) and is precisely the gap acceptance criterion #1 exists to close.

**Named pitfall — "topology-blind git-dir fixture":** *Any Go test asserting behavior of
git-tooling code that depends on `--git-dir` vs. `--git-common-dir` (or more generally,
any git plumbing command whose output differs between a plain repo and a linked worktree)
must build its fixture via `git worktree add` from a real base repo, not `git init` alone.
A `git init`-only fixture is topology-blind: it will pass regardless of which of the two
directories the code under test resolves to, so it provides zero regression coverage for
this exact class of bug.* This should be stated as a hard requirement on the new
regression test (acceptance criterion #1), not left implicit — plan/implementation phases
must use a `git worktree add` fixture, and the test should assert on the *observable
effect* (`git status --porcelain` / `IsWorktreeDirty` reporting clean) rather than merely
that the exclude file was written to some path.

## 2. `info/exclude` / `--git-common-dir` version risk — LOW, confirmed low

- `--git-common-dir` has been available since git 2.5 (2015); no realistic risk from that
  alone.
- No pinned git version anywhere in this repo: grepped `.github/workflows/*.yml` — every
  job that checks out code runs `runs-on: ubuntu-latest` (`build.yml:49,80,121,272,325,364`,
  `backlog-scaffolding-guard.yml:30`) with no explicit git install/version-pin step found.
  `ubuntu-latest` GitHub-hosted runners ship a current git (>=2.4x as of 2026), far above
  the 2.5 floor.
- `--git-common-dir` is not a new dependency for this codebase — it's already used in three
  other places: `session/repo_path.go:401` (`GetMainRepoPath`), `session/git/util.go:153`
  (doc comment at :148 explicitly names it "the shared .git directory a worktree points
  at"), confirming the semantics this fix relies on are already trusted and exercised
  elsewhere in production. This significantly de-risks the fix: it's reusing an existing,
  working pattern rather than introducing new git-version-dependent behavior.
- No other `--git-dir`/`--git-common-dir` semantic divergence beyond the worktree-private
  vs. shared-admin-dir split was found relevant here; `info/exclude` read behavior (only
  the common dir's copy is honored by `git status` in a linked worktree) is the single
  divergence this bug depends on, and it's been stable git behavior since worktrees were
  introduced (git 2.5).

## 3. Race / idempotency risk — REAL but low-severity (duplicate lines, not corruption)

- `session/git/scaffolding.go:11-24` defines `untrackMu sync.Map` (per-worktree-path
  `sync.Mutex`), documented (`:11-18`) as serializing **`UntrackScaffolding`'s** index
  read-modify-write only — explicitly scoped to go-git's `Storer.Index()`/`SetIndex()`
  calls, which don't participate in git's own `index.lock` protocol.
- `addWorktreeExcludes` (`session/backlog_commands.go:241-277`) is **not** covered by this
  lock at all — it does its own unlocked read-then-append against `info/exclude`:
  `os.ReadFile` (existing-content check, :262-263) followed by
  `os.OpenFile(O_APPEND|O_CREATE|O_WRONLY)` (:265) with a `strings.Contains` guard against
  the file content read *before* opening. This is a classic TOCTOU window: two concurrent
  callers can both read the file before either has appended, both conclude a pattern isn't
  present, and both append it — resulting in duplicate lines in `info/exclude`.
- **Concrete impact if triggered:** benign in git-behavior terms — `info/exclude` lines are
  idempotent as far as `git status` filtering, so duplicate entries don't break exclusion,
  they just make the file untidy and (if it happens repeatedly, e.g. every session
  respawn) grow unboundedly over the worktree's lifetime.
- **Caller-concurrency check:** grepped every call site of `WriteSlashCommands` and
  `WriteBacklogContextFile` (the two callers of `selfHealWorktreeScaffolding`, which is the
  only caller of `addWorktreeExcludes`): `server/services/backlog_service_sync.go:94,98`
  and `server/services/backlog_service_triage.go:1152,1155`. In both files the two calls
  are sequential, same-goroutine, within a single function body — not concurrent with each
  other. No call site was found that spawns concurrent goroutines calling either function
  for the *same* worktree path. So under the current call graph this is a **latent** race,
  not one with a known concrete trigger today (e.g., a periodic sync tick overlapping an
  on-demand triage respawn on the same item/worktree would be the theoretical trigger, but
  no such overlap was found wired up).
- **Recommendation for the plan phase:** given severity is low (cosmetic duplicate lines,
  not lost writes or corruption) and no known concurrent trigger exists today, this is
  worth a one-line note in the implementation plan (e.g., extend `addWorktreeExcludes` to
  take the same per-worktree lock `lockForWorktree` already exposes, or de-dupe on write)
  but should **not** block the fix — it's a pre-existing latent property of the function,
  not something the `--git-common-dir` change introduces or worsens. Do not conflate
  fixing it with fixing the resolution-path bug; if addressed, do so as a clearly separated
  , small follow-on inside the same PR at most.

## 4. Message-formatting / truncation pitfall — needs an explicit cap

- Precedent found: `server/mcp/tools_backlog.go:360-361` caps `message` at 2000 chars
  (`"message must be <= 2000 characters"`) and `:369` caps `verification_notes` at 4000
  chars — both are **input** validation caps on MCP tool parameters, not precedent for
  capping a server-generated message, but they establish the codebase's general posture
  that unbounded text fields flowing through this tool surface are treated as a
  correctness/UX risk worth bounding.
- The new dirty-paths rejection message (replacing the blanket `git add -A && git commit`
  instruction at `server/mcp/tools_backlog.go:388-389`) has no existing analog to copy
  size-limiting behavior from, because no function currently enumerates dirty paths
  (`GetWorktreeDirtyPaths` doesn't exist yet — confirmed via grep, see acceptance
  criterion #2).
- **Risk if uncapped:** a worktree with hundreds of dirty files (e.g. an accidental
  `node_modules` untrack, or a large auto-generated directory getting swept into `git
  status --porcelain` output) would produce a message of unbounded length fed back through
  the MCP `request_review` tool response — same class of "flows into an LLM prompt/tool
  result" concern the 2000/4000-char caps exist to guard against, plus a plain UX problem
  (an agent can't usefully read a 500-line file list).
- **Recommendation:** the new `formatDirtyPathsRejectionMessage` (or equivalent) should
  cap the number of *listed* paths (e.g. first N with a "...and N more" suffix) rather than
  truncating mid-string — path lists truncate more usefully by count than by character
  offset (a byte-length cap risks cutting a path in half, which the 2000/4000-char input
  caps don't need to worry about since those are opaque free-text fields, not structured
  lists). This should be an explicit decision in the plan phase, not left to chance in
  implementation.

## 5. Backward-compatibility pitfall — CLEAR, no test breaks on message-wording change

- Grepped every `*_test.go` file in the repo for the literal rejection message strings
  (`"the worktree has uncommitted changes"`, `"git add -A && git commit"`). The **only**
  hit was `server/services/hook_receiver_drift_test.go:46`:
  ```go
  if !isGitCommitOrPushCommand(`git add -A && git commit -m "x" && git push`) {
  ```
  This is an unrelated test of `isGitCommitOrPushCommand` (a shell-command-shape detector
  used elsewhere in hook processing) using that string only as a *sample shell command* to
  classify — it has no relationship to `tools_backlog.go`'s rejection message and would be
  unaffected by any wording change to the `request_review` rejection text.
- **No test in the repo string-matches the exact current rejection message
  (`server/mcp/tools_backlog.go:388-389`).** Changing its wording/format to list specific
  dirty paths (acceptance criterion #3) is safe from a backward-compat test-breakage
  standpoint — there is nothing pinning the current exact phrasing.
- One adjacent thing worth flagging for the plan phase (not a blocker, just scope
  awareness): `server/mcp/tools_backlog.go:381` has a `// Belt-and-suspenders layer 1`
  comment implying there's a "layer 2" elsewhere — worth locating during planning to
  confirm the new dirty-paths message doesn't need to be duplicated/kept in sync with a
  second call site using similar wording.

## Summary of design constraints these findings impose on the plan phase

1. The regression test for acceptance criterion #1 **must** use a `git worktree add`
   fixture (not `git init` alone) and assert on `git status`/`IsWorktreeDirty` clean state,
   per the "topology-blind git-dir fixture" pitfall (§1) — this is the single most
   important constraint from this research, since it's the exact gap that let the bug ship
   silently once already.
2. No git-version gating or compatibility shim is needed for `--git-common-dir` (§2).
3. The TOCTOU gap in `addWorktreeExcludes` (§3) is pre-existing, low-severity, and not
   introduced by this fix — note it, optionally fix it as a small separated change, but
   don't let it block or scope-creep the primary fix.
4. `GetWorktreeDirtyPaths`'s message formatter should cap the *count* of listed paths with
   a "...and N more" style truncation, not a raw byte-length cut (§4).
5. No existing test pins the current rejection message's exact wording — free to change it
   (§5).
