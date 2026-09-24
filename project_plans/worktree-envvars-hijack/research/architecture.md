# Architecture / Root-Cause Research: worktree-envvars-hijack

Research Agent 3 (Architecture). All line numbers verified against the worktree
checkout at the time of writing (branch `tstapler/triage-a4d287ba-f050-4931-a9c9-59c7ea387b14`).

## Q1: Full trace of `sessionType`/`i.SessionType` reassignment sites

`resolveSessionType(req.Msg, branch)` (`server/services/session_service.go:3108-3132`):

```go
func resolveSessionType(msg *sessionv1.CreateSessionRequest, branch string) session.SessionType {
	if msg.SessionType != sessionv1.SessionType_SESSION_TYPE_UNSPECIFIED {
		switch msg.SessionType {
		...
		case sessionv1.SessionType_SESSION_TYPE_NEW_WORKTREE:
			return session.SessionTypeNewWorktree
		...
		}
	}
	if msg.ExistingWorktree != "" { return session.SessionTypeExistingWorktree }
	if branch != "" { return session.SessionTypeNewWorktree }
	return session.SessionTypeDirectory
}
```

Because both repro requests set `sessionType: SESSION_TYPE_NEW_WORKTREE` explicitly,
this returns `SessionTypeNewWorktree` immediately, short-circuiting the
`branch`-based inference entirely. This is the `session_service.go:2537` call site.

Every subsequent reassignment of the local `sessionType` variable between
`:2537` and the `instanceOpts := session.InstanceOptions{...}` literal at
`:2835` is gated on `req.Msg.SessionType == sessionv1.SessionType_SESSION_TYPE_UNSPECIFIED`:

- `:2538-2540` (alias session type fallback): `if aliasSessionType != config.SessionTypeDefault && req.Msg.SessionType == sessionv1.SessionType_SESSION_TYPE_UNSPECIFIED`
- `:2553-2556` (deferred GitHub URL force-worktree): `if deferredGitHubURL && req.Msg.SessionType == sessionv1.SessionType_SESSION_TYPE_UNSPECIFIED && req.Msg.ExistingWorktree == ""`
- `:2560-2563` (resume force-directory): `if req.Msg.ResumeId != "" && req.Msg.ForkSourceId == "" && req.Msg.SessionType == sessionv1.SessionType_SESSION_TYPE_UNSPECIFIED`

Since `req.Msg.SessionType` is `SESSION_TYPE_NEW_WORKTREE` (not `UNSPECIFIED`) for
**both** Request A and Request B, none of these three blocks can fire for either
request. The remote-target block (`:2652-2831`) only runs when `remoteRequested`
is true, which the repro (local macOS paths) does not exercise.

There is exactly one other reassignment: `:2543-2545` (`SessionTypeOneOff` →
`SessionTypeDirectory`), also inapplicable since `sessionType` is
`SessionTypeNewWorktree`, not `SessionTypeOneOff`, at that point.

**`instanceOpts.SessionType = sessionType` (`:2848`) is therefore identical for A
and B: `SessionTypeNewWorktree`.**

I additionally grepped every assignment to the *live* `Instance.SessionType`
field (not the local var) across the whole `session` package:

```
session/ent_repository.go:1354:  data.SessionType = SessionType(sess.SessionType)   // load-from-storage path, not create
session/session.go:353:          i.SessionType = s.Filesystem.SessionType           // import/deserialize path, not create
```

Neither is reachable from `CreateSession`'s synchronous path or
`runBackgroundResolutionPipeline`. `session.NewInstance()` copies
`opts.SessionType` onto `instance.SessionType` once, unconditionally
(confirms/extends the triager's existing VERIFIED note), and nothing else in
the create path ever touches it again before `setupFirstTimeWorktree()`'s
`switch i.SessionType` reads it.

**Conclusion: Q1 is now fully closed, not just partially eliminated by the
baseline.** `i.SessionType` is `SessionTypeNewWorktree` for both A and B at the
moment `setupFirstTimeWorktree()` runs. A silent SessionType downgrade is not
the mechanism.

## Q2: `config.ResolveProgramConfig` — full definition

`config/defaults.go:202-227`:

```go
// ResolvedProgram holds the resolved executable command, CLI flags, and environment variables for a program ID.
type ResolvedProgram struct {
	Command  string
	CLIFlags string
	EnvVars  map[string]string
	IsCustom bool
}

func ResolveProgramConfig(cfg *Config, program string) ResolvedProgram {
	if prog := FindProgramConfig(cfg, program); prog != nil {
		env := ExpandEnvVars(prog.Env)
		return ResolvedProgram{Command: prog.Command, CLIFlags: prog.CLIFlags, EnvVars: env, IsCustom: true}
	}
	return ResolvedProgram{Command: program, CLIFlags: "", EnvVars: nil, IsCustom: false}
}
```

`ResolvedProgram` has **no `Path`/`WorkingDir` field at all** — the struct only
carries `Command`, `CLIFlags`, `EnvVars`, `IsCustom`. The call site
(`session_service.go:2521-2531`) only ever writes into `instanceEnvVars` (a
plain env-var merge map that feeds `InstanceOptions.EnvVars`); it never reads
or writes `resolvedPath`/`instanceOpts.Path`/`instanceOpts.WorkingDir`.

**Conclusion: Q2's hypothesis is definitively ruled out.** A custom program
cannot carry a path override that diverges the two requests' `resolvedPath` —
the type doesn't have anywhere to put one.

## Q3: `newWorktreeFromResolvedBase()` and the full worktree-creation chain

Read in full, plus everything it calls transitively, looking specifically for a
silently-swallowed error that would leave `gitManager.HasWorktree()==true` with
`GetWorktreePath()` resolving to the bare repo path (or `""`, which
`GetEffectiveRootDir()`/`ActiveDir()` would then fall back from to `i.Path` —
same visible symptom).

`session/instance_worktree.go:219-242`:

```go
func (i *Instance) newWorktreeFromResolvedBase() (*git.GitWorktree, string, error) {
	branchName := git.ResolveBranchName(i.Branch, i.Title)
	resolvedRepo, err := ResolveMainRepoRoot(i.Path)
	if err != nil {
		resolvedRepo = i.Path   // only affects the divergence-warning computation below
	}
	defaultBranch, baseSHA, resolveErr := git.ResolveWorktreeBaseCommit(resolvedRepo)
	if resolveErr != nil {
		return nil, "", fmt.Errorf("resolve default branch: %w", resolveErr)   // real, propagated error
	}
	if baseSHA == "" {
		// unborn repo
		return git.NewGitWorktreeWithBranch(i.Path, i.Title, branchName, git.WithCommandRunner(runner))
	}
	if diverged, ambientBranch := git.AmbientHEADDivergesFromBase(resolvedRepo, baseSHA); diverged {
		i.CreationWarning = git.FormatAmbientDivergenceWarning(...)   // warning only, not an error
	}
	return git.NewGitWorktreeFromCommitSHA(i.Path, i.Title, branchName, baseSHA, git.WithCommandRunner(runner))
}
```

Both terminal constructors (`git.NewGitWorktreeWithBranch` →
`NewGitWorktreeWithBranchAndExecutor`, and `git.NewGitWorktreeFromCommitSHA`,
`session/git/worktree.go:145-191,264-331`) either return `(nil, "", err)` on a
real failure or a fully-populated non-nil `*GitWorktree` — there is no
nil-tree/nil-err combination. `setupFirstTimeWorktree`'s `SessionTypeNewWorktree`
case (`session/instance_worktree.go:71-74`) propagates any error from this call
as a real error (`fmt.Errorf("failed to create git worktree: %w", err)`), which
`finishFirstTimeSetup` → `startLocked` → `Start()` →
`runBackgroundResolutionPipeline` all propagate straight through to a terminal
`Failed` write (`session_creation_pipeline.go:258-263`). This branch is
inconsistent with the reported `status: ACTIVE, no error`.

**The only reuse-from-existing-worktree logic in the construction path**
(`NewGitWorktreeWithBranchAndExecutor`, `session/git/worktree.go:290-310`, taken
only on the unborn-repo branch) calls `nativeFindExistingWorktreeForBranch`,
which in turn calls `nativeListWorktrees(repoPath)`
(`session/git/native_worktree_list.go:40-63`). That function **only reads
`repoPath/.git/worktrees/<name>/`** — the linked-worktree admin directories.
Git's own main/primary checkout (the bare repo itself) never has an entry
there; it is structurally invisible to this lookup. So this reuse path can
return, at most, a *different* linked worktree's path — never the bare repo
path itself.

The second stage — `GitWorktreeManager.Setup()` (`session/git_worktree_manager.go:195-207`)
→ `GitWorktree.Setup()`/`setupLocked()` (`session/git/worktree_ops.go:107-177`) —
runs later, inside `startLocked`'s `firstTimeSetup` branch
(`session/instance.go:1589-1600`), and is what actually executes `git worktree
add`/native equivalent on disk. I read this chain in full:

- `setupLocked` branches on `branchExists` (a real, repo-wide go-git ref
  lookup — not scoped to `.git/worktrees/`). If the branch already exists →
  `setupFromExistingBranch()`; else → `nativeSetupNewWorktreeWithSelfHeal()`.
- `setupFromExistingBranch` (`worktree_ops.go:200-243`): reuses in place if
  `worktreeAlreadyRegisteredForBranch()` (git-reported path+branch match against
  `g.worktreePath`, the freshly-computed timestamped target — not the bare repo)
  is true; otherwise removes any stale worktree at that target path and runs
  `git worktree add <g.worktreePath> <branchName>`. **If that fails** (e.g.
  because `branchName` is already checked out somewhere git considers "already
  checked out" — including the *main* working tree, which `git worktree add`
  does check against even though it has no `.git/worktrees/` entry), the
  self-heal `findLiveWorktreeForBranch()` → `nativeFindLiveWorktreeForBranch()`
  (`worktree_ops.go:356-397`) is tried — but this **also** only enumerates
  `nativeListWorktrees(g.repoPath)`, i.e. `.git/worktrees/` admin dirs, so it
  is equally blind to the main checkout. In the specific scenario where the
  branch collision is with the *main* checkout (not a linked worktree), this
  self-heal fails to find anything and the function returns a **real,
  propagated error** (`worktree_ops.go:236`) — not a silent bare-path success.
- `nativeSetupNewWorktreeWithSelfHeal`/`nativeSetupNewWorktree`
  (`worktree_ops.go:418-431`, `native_worktree_add.go:67-101`): the plain-add
  path used when the branch is genuinely new. Every step (`OpenRepo`,
  `resolveNativeAddBaseCommit`, `AllocateAdminDirName`,
  `writeNativeWorktreeAdminFiles`, `checkoutNativeWorktree`) either succeeds or
  returns a real error; `g.worktreePath` itself was already computed via
  `joinWithinDir(worktreeDir, sanitizedName) + "_" + timestamp`
  (`session/git/worktree.go:313-318`), and `joinWithinDir`
  (`session/git/util.go:141-153`) explicitly rejects any result that would
  escape `worktreeDir` — it can never resolve to the bare repo path.

**Conclusion: I found no silent-swallow path anywhere in worktree
construction or `Setup()`.** Every failure mode I traced either (a) propagates
a real error that would surface as a Failed session (contradicting the
reported `ACTIVE`/no-error state), or (b) is structurally incapable of
resolving `g.worktreePath`/`GetWorktreePath()` to the bare repo path, because
every path-selection function in this chain (fresh-create, existing-worktree
reuse, self-heal reuse) either builds a path strictly under the configured
worktrees directory or looks *only* inside `.git/worktrees/`, which never
contains the main checkout. This significantly narrows the search: the bug is
unlikely to be a "worktree creation quietly no-ops" bug in this specific
chain. (It leaves open a *related*, narrower gap — `git worktree add` failing
because of a real collision with the *main* checkout's currently-checked-out
branch is NOT self-healed and does NOT match the reported symptom, since it
fails loudly — worth fixing on its own merits but not this bug.)

## Q4: `Instance.Workspace()`/`ActiveDir`/`ExistingDir` — snapshot race?

`session/instance_worktree.go:446-502`:

```go
func (i *Instance) ActiveDir() string {
	if i.gitManager.HasWorktree() {
		if p := i.gitManager.GetWorktreePath(); p != "" {
			return p
		}
	}
	return i.GetPath()
}

func (i *Instance) Workspace() Workspace {
	repoRoot := i.GetPath()
	worktreeDir := ""
	if i.gitManager.HasWorktree() {
		worktreeDir = i.gitManager.GetWorktreePath()
	}
	activeDir := worktreeDir
	if activeDir == "" { activeDir = repoRoot }
	existingDir := activeDir
	if existingDir != repoRoot {
		if _, err := os.Stat(existingDir); err != nil { existingDir = repoRoot }
	}
	return Workspace{RepoRoot: repoRoot, WorktreeDir: worktreeDir, ActiveDir: activeDir, ExistingDir: existingDir}
}
```

Both `i.gitManager.HasWorktree()` and `i.gitManager.GetWorktreePath()` call
through to `GitWorktreeManager.GetWorktree()`
(`session/git_worktree_manager.go:120-124`), which does a live
`gm.mu.RLock()`-guarded read of `gm.worktree` — **not** a value cached in
`Instance.snapshot`. And `GetSession`'s response is built by
`adapters.InstanceToProto` (`server/adapters/instance_adapter.go:61-74`):

```go
ws := inst.Workspace()
...
Path:        ws.ExistingDir,
WorkingDir:  ws.ActiveDir,
...
ActiveDir:   ws.ActiveDir,
ExistingDir: ws.ExistingDir,
```

This calls `inst.Workspace()` fresh, synchronously, on every conversion — it
never reads `Instance.snapshot` (the `atomic.Pointer[InstanceSnapshot]`) for
these fields at all.

**Conclusion: Q4's race hypothesis is ruled out.** `GetSession` cannot be
observing a stale pre-worktree snapshot for `workingDir`/`activeDir`/
`existingDir` — every call recomputes them live against the actual
`GitWorktreeManager` state at request time. Given this, the reported bare path
was not a transient read — it reflects a **persistent** state where
`gitManager.HasWorktree()` was false (or `GetWorktreePath()` returned `""`) for
this instance even after creation completed. Combined with Q3's finding that
the `SessionTypeNewWorktree` case (once entered) cannot silently leave the
worktree unset without also returning a propagated error, this means either
(a) `setupFirstTimeWorktree()`'s `SessionTypeNewWorktree` case was never
actually entered for this instance despite `i.SessionType` being
`SessionTypeNewWorktree` at construction (ruled out by Q1 — nothing changes it
in between), which is a contradiction I could not resolve from static reading
alone, or (b) the actual live `CreateSessionRequest` the server received for
Request B differed from the reporter's description in some field this
backend-only trace cannot see (e.g. a client-side proto-construction bug
specific to the envVars/advanced-options code path in the web-app or an MCP
tool, not yet audited by this agent — see "Blind spot" below).

## Q5: Tmux launch — fresh path or stale reference?

`session/instance.go:1585-1667` (the `firstTimeSetup` branch of `startLocked`):

```go
} else {
	// firstTimeSetup: ...
	basePath := i.Path
	if i.gitManager.HasWorktree() {
		if i.SessionType != SessionTypeExistingWorktree {
			if err := i.gitManager.Setup(); err != nil { ...; return setupErr }
		}
		basePath = i.gitManager.GetWorktreePath()
	}
	i.initTmuxSession()
	startPath := i.resolveStartPath(basePath)
	...
	if err := i.pm().Start(startPath); err != nil { ... }
```

`basePath`/`startPath` here are computed fresh, *after* `gitManager.Setup()`
has actually run — this is not the stale-reference bug the question
hypothesized; if `HasWorktree()` were true with a real worktree path, tmux
would launch there, not at a stale bare path.

**However, I did find a real, separate stale-path bug** one layer up, in
`server/services/session_service.go` and
`server/services/session_creation_pipeline.go`, which the Q5 hypothesis's
framing ("could the launch command use a stale i.Path captured earlier")
correctly anticipated in shape, just not in the exact location:

`session_service.go:2956-2963` (captured **before** `Start()` runs — this is
right after `CreateManagedInstance`, which explicitly does *not* start
tmux/worktree setup):

```go
instanceTitle := instance.Title
instanceRootDir := instance.GetEffectiveRootDir()   // <-- pre-worktree-creation, i.e. still the bare repo path
```

This is threaded into `creationPipelineParams.instanceRootDir`
(`session_creation_pipeline.go:34-51`) and used as the local
`instanceRootDir` (`:90`). It is **only refreshed** inside the
`deferredGitHubURL` branch (`:172-200`, specifically `:198`:
`instanceRootDir = p.instance.GetEffectiveRootDir()`). For a plain
(non-GitHub-URL) `SessionTypeNewWorktree` session — the shape of **both**
Request A and Request B — this branch never runs, so `instanceRootDir` stays
at its pre-`Start()` value (the bare repo path) for the rest of the pipeline,
and is used, un-refreshed, at:

```go
} else if err := InjectHookConfig(instanceRootDir, p.instanceTitle); err != nil {   // :296
	...
}
...
session.StartSessionDriver(p.instance, instanceRootDir)   // :322
```

This is a real, confirmed bug: the Claude Code HTTP hook config
(`InjectHookConfig`) gets written into the **bare repo directory**, not the
freshly-created worktree, for every plain `SessionTypeNewWorktree` session —
and the session driver (which types the initial prompt) is told to operate
against the bare repo path too. If another, unrelated live session is already
running `claude` in that same bare repo directory (as the reporter's scenario
describes), this new session's `InjectHookConfig` call **overwrites that other
session's hook config file** (same path, same filename), and
`StartSessionDriver`'s directory-scoped work targets the wrong directory.

**This bug is present identically for Request A and Request B** (both are
plain `SessionTypeNewWorktree` sessions with no deferred GitHub URL) — it is
not gated on `envVars`/`program`, so it cannot by itself be *the* A/B
differentiator, but it is a real, independently-fixable defect, and it is a
plausible **contributing/compounding** factor to the reported symptom's
severity (hook-config collision with the other live session) even if it isn't
the primary mechanism that made `workingDir`/`activeDir`/`existingDir` report
the bare path in `GetSession` (Q4 already showed those are computed
elsewhere, live, and unaffected by this stale local variable).

## Blind spot / scope boundary

I traced the entire backend Go path from `CreateSession`'s synchronous prefix
through `runBackgroundResolutionPipeline`, `Instance.Start()`,
`setupFirstTimeWorktree()`, the full git-worktree construction/Setup chain
(both the legacy-existing-worktree-reuse path and the native go-git
implementation), and `Instance.Workspace()`/`GetSession`'s proto conversion. I
found **no envVars- or program-conditioned branch anywhere in this backend
trace**, and closed off every mechanism the requirements doc asked me to
check (Q1, Q2, Q4 fully; Q3's worktree-creation chain exhaustively, finding no
silent-swallow path; Q5 partially, finding a real but non-differentiating
stale-path bug one layer up from where the question pointed).

I did **not** audit how the web-app Omnibar or an MCP tool client constructs
the `CreateSessionRequest` proto when the user sets `envVars` (e.g. via an
"advanced options" panel) — a quick grep of `server/mcp/tools_lifecycle.go`
shows the MCP path maps `session_type` independently of `envVars`/`program`
with no coupling, but I did not check the web-app TypeScript request-builder
code, which is out of this backend-architecture agent's traced scope and
better suited to whichever research agent covers the frontend/request-shape
angle. Given how cleanly Q1/Q2/Q4 close off the backend mechanisms the
reporter's own hypotheses point at, **I now suspect the actual differentiator
is not backend control flow conditioned on envVars/program at all**, but
either (a) a client-side request-construction bug specific to the
envVars-bearing code path that changes some *other* field (branch, or
`session_type` itself) in a way this trace can't observe from the Go side, or
(b) a live git-state coincidence (Request B's branch happening to collide with
whatever branch the pre-existing "other live session" had checked out) that
would require the actual repro payloads/logs to confirm.

## Top 3 root-cause hypotheses (ranked)

### 1. Client-side request divergence when `envVars` is set (SPECULATIVE, highest structural fit)

**Hypothesis:** The web-app (or an MCP tool) constructs a subtly different
`CreateSessionRequest` when the user's envVars/advanced-options panel is
populated — e.g. `session_type` or `branch` ends up unset/different from what
the reporter believes was sent — causing `resolveSessionType` to fall through
to `SessionTypeDirectory` (via the `branch == ""` path,
`session_service.go:3128-3131`) for Request B specifically, entirely upstream
of everything this agent traced in the Go backend.

**Evidence:** Q1 proves the backend's `sessionType` resolution is airtight
*given* `req.Msg.SessionType == SESSION_TYPE_NEW_WORKTREE`; Q3 proves the
worktree-creation chain cannot silently produce a bare path once
`SessionTypeNewWorktree` is genuinely entered; Q4 proves `GetSession` isn't
reading stale state. Everything the backend *can* do with the described
inputs has been checked and doesn't reproduce the symptom — which points
upstream of the inputs themselves.

**Confidence:** SPECULATIVE — I did not read the client code that builds this
request.

**Repro test to confirm/deny:** Capture (or add temporary logging for) the
actual wire-level `CreateSessionRequest` proto bytes/JSON the server received
for the failing Request B — specifically `session_type` and `branch` as the
server actually parsed them, not as the UI displayed them. If `session_type`
was actually `UNSPECIFIED` or `branch` was actually empty, this hypothesis is
confirmed and the fix is client-side (or a defensive server-side validation
that treats an inconsistent envVars+worktree combination as an error rather
than silently downgrading).

### 2. Branch-name collision with the pre-existing session's checked-out branch, hitting the un-self-healed "main checkout" gap (PLAUSIBLE)

**Hypothesis:** Request B's resolved branch name coincided with a branch
already checked out in the bare repo itself (where the pre-existing "other
live session" was running, e.g. as a `SessionTypeDirectory` session on that
branch). `git worktree add` then fails (git refuses to check out a branch
that's already checked out elsewhere, including the main working tree), and —
per Q3's finding — the self-heal (`findLiveWorktreeForBranch`) cannot find it
either, since it's blind to the main checkout. Under this reading the failure
**should** propagate as a real error (contradicting "ACTIVE, no error"), so
this hypothesis requires an *additional*, not-yet-located gap where that
specific error class is being swallowed somewhere between
`setupFromExistingBranch` and the pipeline's terminal-write — I did not find
that gap, so this hypothesis is currently incomplete on the "no error"
half of the symptom.

**Evidence:** `worktree_ops.go:200-243`'s error path (verbatim, cited above);
`nativeListWorktrees`'s `.git/worktrees/`-only scope
(`native_worktree_list.go:40-63`).

**Confidence:** PLAUSIBLE for the "why would `git worktree add` even fail"
half; UNVERIFIED for the "why wouldn't that failure surface as an error"
half.

**Repro test to confirm/deny:** Reproduce with a repo where the target branch
is already checked out in the main working tree, run
`setupFirstTimeWorktree()` directly, and check whether the error actually
propagates all the way to a `Failed` status (expected, per my reading) or
gets lost somewhere (would newly confirm a real gap). If it propagates
correctly, this hypothesis is fully ruled out and can be dropped.

### 3. Stale `instanceRootDir` in the creation pipeline (VERIFIED, but explains only a compounding side-effect, not the core symptom)

**Hypothesis:** `session_service.go:2958`'s `instanceRootDir :=
instance.GetEffectiveRootDir()`, captured before `Start()` runs and never
refreshed for a non-GitHub-URL session, causes `InjectHookConfig`
(`session_creation_pipeline.go:296`) and `StartSessionDriver` (`:322`) to
operate against the bare repo path instead of the freshly-created worktree.

**Evidence:** Full call chain cited above (`session_service.go:2956-2963`,
`session_creation_pipeline.go:34-51,90,172-200,283-298,318-322`).

**Confidence:** VERIFIED as a real bug via direct code read. **Ruled out** as
the primary mechanism because (a) it applies identically to Request A, which
worked correctly, so it cannot be the A/B differentiator, and (b) `GetSession`
does not read this value at all (Q4) — it reads `Instance.Workspace()` live.
Worth fixing independently (re-derive `instanceRootDir` from
`p.instance.GetEffectiveRootDir()` unconditionally after `Start()` returns,
not only in the `deferredGitHubURL` branch), since it is a genuine hook-config
misdirection bug and a plausible aggravating factor when two sessions do end
up sharing a directory (e.g. under hypothesis #1 or #2 above) — but it does
not by itself explain how the two sessions came to share a directory in the
first place.

**Repro test to confirm/deny:** Not needed to confirm the bug exists (already
verified by reading), but a regression test would assert
`InjectHookConfig`/`StartSessionDriver` receive the post-`Start()`
`GetEffectiveRootDir()` value (the worktree path), not the pre-`Start()`
capture, for a plain `SessionTypeNewWorktree` session.
