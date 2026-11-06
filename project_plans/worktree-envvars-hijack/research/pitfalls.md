# Pitfalls Research: worktree-envvars-hijack

Research Agent 4 (Pitfalls), SDD Phase 2. Read-only research; no source changed.

## 1. Git history: prior fixes for this exact bug class

This repo has fixed the "silent worktree-creation-skip → session lands in the
bare repo path" shape at least three times before. All three are directly
relevant precedent, not just analogy:

- **`9dbda9d2e`** — *"fix(session): fall back to repo root when worktree path
  is missing from disk"* (`session/instance_worktree.go`). Introduced the
  exact mechanism whose *legitimate* form is now baked into `Workspace()`:
  `ExistingDir` falls back to `RepoRoot` when the worktree directory doesn't
  exist on disk (e.g. `pause_session` deleted it). This fallback is by design
  for `ExistingDir` — but it is precisely the shape the reporter observed
  (`workingDir`/`activeDir`/`existingDir` all resolving to the bare repo
  path). The follow-up refactor `eb91981b2` (below) later found that
  `ActiveDir` had *also* been affected by conflating these two fallback
  semantics (see §6, `#801`).
- **`9037ed5e0`** — *"fix(worktree): stop silently fabricating disconnected
  repos on missing paths"*. `findGitRepoRoot` used to `os.MkdirAll` +
  `git.PlainInit` + a fake "Initial commit" whenever `repoPath` didn't exist,
  instead of erroring — silently producing a worktree with history
  disconnected from the real repo. Same *class*: a missing-precondition path
  that should error instead silently substituted a plausible-looking but
  wrong value.
- **`43be58416`** — *"fix(session): stop backlog worktrees from silently
  branching off ambient HEAD"*. `CreateBacklogWorktree` silently fell back to
  `repoPath`'s ambient checked-out HEAD whenever origin-branch resolution
  failed for *any* reason, branching a new session off whatever branch a
  concurrent process last left the shared checkout on. Same root shape as
  this bug: an error path silently substitutes a "good enough" value instead
  of surfacing failure, and the substituted value depends on unrelated
  concurrent state.

None of these three is *the* fix for this bug (the baseline already read the
current `resolveSessionType`/`NewInstance`/`setupFirstTimeWorktree` code and
found no direct `EnvVars`-conditioned branch), but they establish the
project's own recurring failure mode: **worktree-path resolution silently
substitutes the bare repo path instead of erroring, repeatedly, across three
separate incidents.** Any fix here should be checked against not reintroducing
a fourth instance of the same shape.

- **`eb91981b2`** — *"refactor(session): name the four session path
  concepts distinctly" (#805)* — post-dates the above three. Its commit
  message documents `#801`/`WorkspacePeersPanel` reporting **false
  collisions between properly isolated sessions** because callers compared
  the fallback-prone field instead of the disk-existence-agnostic one. That
  PR is the origin of today's `ActiveDir`/`ExistingDir` split
  (`.claude/rules/instance-lock-free-reads.md` documents the same split and
  the same underlying confusion). Relevant here as a **cautionary tale in the
  opposite direction**: `#801`'s own commit message *misattributed* its root
  cause (blamed `session.path`, when the actual carrier was
  `Workspace().EffectivePath`'s fallback) — evidence that this exact
  code path has burned even careful review before, and that this bug's
  root-cause hypothesis needs to be pinned to an actual field/line, not
  inferred from symptom-shape alone.

## 2. TODO/FIXME/known-gap comments near the relevant code

`grep -n "TODO\|FIXME\|HACK\|known.gap\|not yet\|race"` across
`session/instance_worktree.go`, `session/instance_tmux.go`, and
`server/services/session_service.go` turned up no direct hit *naming this bug*,
but several adjacent "known gap" comments worth flagging to Phase 3 planning:

- `server/services/session_service.go:634` — "an actually-slow GitHub host
  resolution end-to-end" is called out as a **known gap**, not tested
  end-to-end. Relevant because the reporter's Request B never went through
  GitHub-URL resolution (no `program` custom/GitHub source), so this
  particular gap is likely not implicated — but it establishes that the
  background pipeline's phase coverage is known-incomplete, not
  exhaustively tested.
- `server/services/session_service.go:3038` — "existing remote sessions'
  relays on server restart is a known gap." Not implicated (Request A/B are
  not remote), but same pattern of an acknowledged, undocumented-elsewhere
  gap living only as an inline comment.
- `server/services/session_service.go:4166` — a comment noting that whether
  GitHubResolution was ever recorded on the instance "is a known gap" not
  otherwise surfaced — another inline-only known gap.
- `session/instance_worktree.go:199` — "`create_if_missing` is not yet
  supported for remote Directory sessions" inside `setupFirstTimeWorktree`'s
  `default:` branch (see §3a below) — the same branch that silently no-ops
  for any session type it doesn't recognize.

None of these is a smoking gun for the `EnvVars` divergence itself, but they
confirm the background pipeline and worktree-setup code both carry
precedent for "acknowledged as incomplete, silent rather than loud."

## 3. Common pitfall classes, evaluated against this codebase

### (a) Error-swallowing in the background goroutine — PARTIALLY RULES OUT, finds a stronger candidate

`runBackgroundResolutionPipeline` (`server/services/session_creation_pipeline.go:79`)
funnels every exit path through one `terminal()` closure (line 105), and
`p.instance.Start(true)` failing **does** surface as `terminal(pipelineOutcome{session.Failed, "StartupError", ...})`
(`session_creation_pipeline.go:258-263`). `Start()` calls `finishFirstTimeSetup()`
→ `setupFirstTimeWorktree()` (`session/instance.go:1283`), and
`setupFirstTimeWorktree`'s `SessionTypeNewWorktree` case does return a wrapped
error on `newWorktreeFromResolvedBase()` failure
(`session/instance_worktree.go:71-74`). So a *loud* worktree-creation failure
in that specific case **would** reach `status: Failed`, contradicting the
`status: ACTIVE` the reporter observed — this specific mechanism is not the
culprit for an error that occurred inside the `SessionTypeNewWorktree` case.

**However**, `setupFirstTimeWorktree`'s `switch i.SessionType` has a
`default:` arm (`session/instance_worktree.go:194-210`):

```go
default: // SessionTypeDirectory and unknown types → no worktree
    log.Info("directory session, no git worktree", "session", i.Title, "path", i.Path)
    ...
    i.gitManager.SetWorktree(nil)
    i.Branch = ""
```

This is a **silent no-op**, not an error, for `SessionTypeDirectory` *and any
session type the switch doesn't otherwise recognize*. If `i.SessionType` were
ever anything other than the four explicitly-cased values at this point
(including a zero-value/miscategorized type), the session proceeds straight
to `Active` with `i.gitManager`'s worktree left `nil` — which is exactly what
makes `Workspace().ActiveDir` (per the `types.go` doc comment: "`WorktreeDir`
if it has one, else `RepoRoot`") collapse to the bare repo path with **no
error at all**, not even a log line above `Info`.

A structurally identical second layer exists one level up:
`resolveSessionType` (`server/services/session_service.go:3108-3132`) has its
own `default:` inside the explicit-`SessionType` switch
(line 3121-3122) that returns `session.SessionTypeDirectory` for *any*
unrecognized `sessionv1.SessionType` enum value — silently, with no
logging or error at all. **Two independent silent-fallback-to-Directory
layers are stacked**: if anything upstream (proto (de)serialization,
enum-value skew, or a code path this investigation's baseline hasn't yet
ruled out) ever produces an off-nominal `SessionType`, both layers convert it
to "just a directory session" without a trace — the same silent-substitution
shape as `9037ed5e0`/`43be58416`/`9dbda9d2e` above, just at a different
layer, and one this investigation's baseline read (per requirements.md) has
not yet examined specifically for `EnvVars`-driven divergence at the proto
layer.

**This is the strongest concrete lead this research task turned up.** Phase 3
should confirm or rule out whether Request B's `SessionType` was actually
`SESSION_TYPE_NEW_WORKTREE` by the time it reached
`setupFirstTimeWorktree`'s switch, vs. having been silently coerced to
Directory somewhere in the `resolveSessionType` → `NewInstance` →
`setupFirstTimeWorktree` chain.

### (b) Stale-snapshot-read race (actor-model async write vs. concurrent RPC read)

`.claude/rules/instance-lock-free-reads.md` documents this exact class for a
different field (`i.Path` read unguarded, raced against
`setGitHubResolutionLocked`'s `i.mu.Lock()` write). The current code already
routes `GetEffectiveRootDir`/`Workspace()` through `ActiveDir()` →
`Snapshot()` (`session/instance_worktree.go:435-500`), so a **freshly-written**
`WorktreeDir` should be race-free by the rule's own reasoning — *provided*
`setupFirstTimeWorktree`'s writes (`i.gitManager.SetWorktree(...)`,
`i.Branch = ...`) are followed by a `buildSnapshot`/`i.snapshot.Store` republish
before any RPC (`GetSession`) can observe the instance, which the
`SessionTypeNewWorktree` case does explicitly (lines 81-89) but the `default:`
case does **not** — it mutates `i.gitManager`/`i.Branch` with no `i.mu.Lock()`
and no snapshot republish at all (lines 191-192, 208-209). This is a second,
independent gap in the same function: even in the cases that don't produce a
wrong value, the write pattern is inconsistent about whether it republishes
the snapshot, which is exactly the ingredient `instance-lock-free-reads.md`
warns is required for `GetSession` (a concurrent RPC reader) to see the
correct state rather than a stale pre-mutation one.

### (c) Directory/path collision between concurrent sessions

Confirmed by direct grep: only `Title` uniqueness is enforced before session
creation (`server/services/session_service.go:2328`,
`connect.CodeAlreadyExists` on duplicate title). **No check anywhere in
`CreateSession` compares a newly-resolved `resolvedPath`/worktree path against
any already-running session's `ActiveDir`.** So if worktree creation silently
no-ops (as in 3a above), nothing downstream would catch or reject the
resulting collision with an already-live session's directory — it is neither
detected nor rejected, it simply happens.

Compounding this: `session/history_detector.go:154`'s `DetectByPath` scans
`~/.claude/projects/<encoded-path>/` for "the most recently modified" JSONL
for a given path with **no per-session ownership check** — it cannot
distinguish which live `Instance` a path "belongs to," because nothing
enforces that only one does. If Request B's `ActiveDir` collapsed onto
Request A's (or any other live session's) directory, and `buildLaunchCommand`
(`session/instance_tmux.go:690`) or the launch-time `claudeSessionID`
resolution consulted this path-keyed detection, it would attribute the
*other* live session's most-recent conversation to the new session —
this is the concrete mechanism that would explain the reporter's "tmux pane
showed that other session's conversation content, including turns
postdating the new session's creation." This is the same failure shape
`project_plans/cold-start-uuid-loss/requirements.md` already documents for a
different trigger (`DetectByPath`'s "Ambiguous JSONL selection" Rabbit Hole:
"if a path has been reused across genuinely different conversations... blindly
resuming the newest file could resume the wrong conversation instead of
losing one" — written for a *recycled* worktree, but the mechanism is
identical for two *concurrently live* sessions sharing a path).

## 4. `docs/explanation/concurrency-patterns.md` / `docs/reference/state-isolation.md`

- `docs/reference/state-isolation.md:16` documents a directly analogous prior
  incident: **"A per-cwd auto-isolated workspace being the default previously
  caused sessions to silently 'disappear' when the binary was started from an
  unusual cwd."** The fix was to make workspace isolation opt-in rather than
  cwd-inferred. Same underlying lesson as this investigation: **isolation
  that's implicit/inferred rather than explicit and verified is where this
  codebase's silent-collision bugs keep originating.**
- `docs/explanation/concurrency-patterns.md` was reviewed for worktree- or
  `EnvVars`-specific gotchas; it covers double-checked-locking-style patterns
  generally but has no entry specific to worktree-path resolution or
  `CreateSession`'s pipeline — this is itself a documentation gap: the
  `instance-lock-free-reads.md` rule (glob-scoped, not indexed from this doc)
  is the closest existing guidance and is scoped only to `session/instance*.go`
  field reads, not to the broader "silent path-fallback" class this bug and
  the three git-history precedents in §1 all share.

## 5. Existing test coverage gap

Searched `server/services/*_test.go` and `session/*_test.go` for tests
asserting worktree path ≠ bare repo path for `SESSION_TYPE_NEW_WORKTREE`,
and specifically for the three-way combination the requirements doc calls out
(`SESSION_TYPE_NEW_WORKTREE` + non-empty `EnvVars` + built-in `program=claude`).

**No such test exists.** Concretely:

- `TestCreateSession_ThreadsEnvVars_WhenSetInRequest`
  (`server/services/session_service_envvars_test.go:16`) is the only full
  `CreateSession`-pipeline test combining `program: "claude"` with non-empty
  `EnvVars` — but it passes no `Branch` and no explicit `SessionType`, so
  `resolveSessionType` (branch == "" and `SessionType` unspecified) resolves
  to `session.SessionTypeDirectory`, not `SessionTypeNewWorktree`. It asserts
  only `inst.EnvVars["FOO"] == "bar"`, never `inst.ActiveDir()` /
  `Workspace().ActiveDir` against the repo root. **It cannot have caught this
  bug even if it exists, because it never exercises the worktree-creation
  code path at all.**
- `TestResolveSessionType_ExplicitNewWorktree`,
  `TestResolveSessionType_UnspecifiedBranchInfersNewWorktree`, etc.
  (`server/services/session_service_create_test.go:34-76`) are pure
  unit tests of `resolveSessionType()` in isolation — they never construct an
  `EnvVars`-bearing request, and never run the result through
  `setupFirstTimeWorktree`/`Start()`.
- `TestWorkspace_ActiveDirAndExistingDir_Diverge_WhenWorktreeMissing` and
  `TestWorkspace_ActiveDir_KeepsWorktreeDir_WhenMissingFromDisk`
  (`session/instance_worktree_test.go:336`, `:357`) test `Workspace()`
  directly against a hand-built `*Instance` with `gitManager.SetWorktree(...)`
  already called — they never go through `resolveSessionType` →
  `NewInstance` → `setupFirstTimeWorktree` → `Start()`, so they cannot catch
  a bug in that chain producing a wrong `SessionType` or a silently-skipped
  worktree creation.

**The gap, precisely stated:** no test exercises the full path
`CreateSession(SESSION_TYPE_NEW_WORKTREE, EnvVars: {...}, Program: "claude")`
→ `runBackgroundResolutionPipeline` → `Start()` → `setupFirstTimeWorktree()`
and then asserts `inst.Workspace().ActiveDir != inst.Workspace().RepoRoot`
(or equivalently, that `gitManager.HasWorktree()` is true). This is the
integration-level test this bug's root cause — whatever it turns out to be —
needs, regardless of which specific mechanism in §3 is confirmed.

## Designed-against checklist for Phase 3 planning

Any fix for this bug must be designed to satisfy all of the following,
regardless of which exact mechanism (§3a/b/c) turns out to be the confirmed
root cause:

1. **No silent `default:`/unrecognized-value fallback anywhere in the
   session-type resolution chain.** Both `resolveSessionType`'s proto-enum
   `default:` (`server/services/session_service.go:3121-3122`) and
   `setupFirstTimeWorktree`'s Go-switch `default:`
   (`session/instance_worktree.go:194-210`) currently convert *any*
   unrecognized/off-nominal input into "plain directory session, no error, no
   log above Info" — a fix must make an unexpected `SessionType` reaching
   either switch a loud, `Failed`-status error, not a quiet substitution.
2. **Every write inside `setupFirstTimeWorktree` republishes the snapshot
   before returning**, not just the `SessionTypeNewWorktree` case — so
   `GetSession`/`FindLiveInstance` readers can never observe a stale
   pre-worktree state for *any* session type, closing the same class of gap
   `instance-lock-free-reads.md` already fixed for a different field.
3. **A background-pipeline error always reaches `status: Failed`, never a
   silent `Active`** — already true for `Start()`'s own returned error
   (`session_creation_pipeline.go:258-263`), but confirm it stays true for
   whatever specific mechanism is found; do not let a fix accidentally
   convert a `setupFirstTimeWorktree` failure into a swallowed one.
4. **Add resolvedPath/`ActiveDir` collision detection (or, at minimum, a
   loud warning) against already-live sessions before or immediately after
   worktree setup completes** — today nothing but `Title` is checked for
   uniqueness (`session_service.go:2328`); a session ending up with the same
   `ActiveDir` as another live session should never be silently possible.
5. **`DetectByPath`/`HistoryLinker` path-keyed conversation lookup must not
   attribute a live session's conversation to a different live session**
   sharing the same path — either by using something more specific than "most
   recently modified JSONL for this path" (mirrors the open concern in
   `project_plans/cold-start-uuid-loss/requirements.md`'s "Ambiguous JSONL
   selection" Rabbit Hole) or by making the §4 collision check above strong
   enough that two live sessions can never legitimately share a path in the
   first place.
6. **Add the missing integration test** (§5): `CreateSession` with
   `SESSION_TYPE_NEW_WORKTREE` + non-empty `EnvVars` + `Program: "claude"`,
   run through the real pipeline (not a hand-built `*Instance`), asserting
   `Workspace().ActiveDir != Workspace().RepoRoot` and
   `gitManager.HasWorktree() == true` post-creation. This is the test that
   would have caught this bug family regardless of which exact mechanism
   caused it.
7. **Don't repeat `9dbda9d2e`/`9037ed5e0`/`43be58416`'s exact shape a fourth
   time**: any new fallback-on-failure logic introduced by this fix must
   itself error rather than silently substitute the bare repo path — the
   project has now fixed this precise substitution pattern three times in
   three different call sites.
