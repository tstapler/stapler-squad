# BUG-033: A Worktree Session's Captured Working Directory Was Never Validated to Still Be Inside the Worktree [SEVERITY: Critical]

**Status**: ✅ FIXED (2026-07-22)
**Discovered**: 2026-07-22 — live incident during a `sdd:fix-bug` session on BUG-032: the interactive session working in `/home/tstapler/Programming/stapler-squad` observed its own shared checkout's branch silently switch from `main` to `backlog/stapler-squad-add-cron-schedule-builder-widget` and receive two new commits, mid-session, with no action taken by the interactive session itself.
**Fixed**: 2026-07-22 — `session/instance_worktree.go`, `session/instance_terminal.go`
**Impact**: A backlog work session's isolated git worktree can be created successfully and never actually get used — the session's agent can end up running its real work (branch checkouts, commits) directly in the shared parent repo checkout instead, with `CaptureCurrentState()` silently persisting that escaped path as the session's own `WorkingDir` with no validation. Every other session sharing that same parent checkout — an interactive human session, or another backlog session degraded the same way — is exposed to branch switches and commits from a completely unrelated task.

## Live Incident Reproduction (2026-07-22)

While fixing BUG-032 in an interactive session at `/home/tstapler/Programming/stapler-squad` (branch `main` at session start), a routine `git status` after a `git stash pop` showed the branch had silently become `cron-schedule-widget`, with two unfamiliar commits ahead of `origin/cron-schedule-widget`. `git reflog` confirmed a `checkout: moving from main to cron-schedule-widget` had occurred mid-session, from an outside actor.

`search_sessions("cron")` identified the source: a live, `Active`, `["backlog:work","autonomous"]` session `stapler-squad-add-cron-schedule-builder-widget` (backlog item `fe94a7c1-d82d-4035-99a0-3601cecedba0`), `session_type: "existing_worktree"`. Direct process inspection proved the session's actual tmux pane process (PID confirmed via `tmux list-panes`) had its live OS-level `cwd` set to `/home/tstapler/Programming/stapler-squad` — the shared parent checkout, not an isolated directory.

**This was initially suspected to be a worktree-creation failure with a silent fallback to the parent directory (see this doc's original, since-corrected draft) — that theory does not hold up against the evidence:**

```
$ git worktree list
...
/home/tstapler/.stapler-squad/workspaces/d685c4b1a423cca3/worktrees/stapler-squad-add-cron-schedule-builder-widget_18c4ac95c784fad8    e7ec8f85 [backlog/stapler-squad-add-cron-schedule-builder-widget]
```

The session's own isolated worktree **exists**, is properly registered with git, is on the correct branch, and has a **clean working tree** with **none of the session's real work commits** (`git log` there stops at the same base commit the shared checkout started at). The two real feature commits from this session's 4.5+ hours of work exist only in the shared parent checkout's reflog — meaning the agent's actual development work happened with its shell `cd`'d into the parent repo, not its assigned worktree, and never went back.

## Root Cause

`resolveStartPath` (`session/instance_worktree.go`) already had — and still has — a documented guard for exactly this shape, with a comment naming the danger precisely: *"CaptureCurrentState() can persist the process CWD (e.g. the main repo path when Claude cd's there), which would otherwise bypass worktree isolation."* But that guard lived entirely on the **read side** (when a session is about to (re)start, decide what directory to use) and only fires when `i.gitManager.HasWorktree()` is already `true` at that exact moment — not guaranteed on every restart/reattach ordering.

The actual gap was on the **write side**. `CaptureCurrentState()` (`session/instance_terminal.go`) reads the tmux pane's live current directory via `GetPaneCurrentPath()` and persisted it into `i.WorkingDir` **unconditionally** — with no check that the captured path was still inside the session's own worktree. If an agent `cd`'d out of its worktree for any reason (live scrollback showed this session's own narration: *"mirrored the branch to the pre-provisioned ... branch so the automated pipeline can track it"* — apparently a self-directed step, not anything stapler-squad's own prompts instruct sessions to do) and a snapshot fired while the pane's cwd was still the parent repo, that escaped path was captured and stored as fact. The write-side gap meant bad state could always get *in*; the read-side guard was the only thing standing between that bad state and actually being used again, and it wasn't reliably positioned to catch every case.

## Fix Applied

- `session/instance_worktree.go`: extracted the escape-detection logic from `resolveStartPath` into a shared `pathEscapesRoot(root, candidate string) bool` helper (fails closed — a `filepath.Rel` error is treated as escaped, not allowed through). `resolveStartPath` now calls it as before (unchanged behavior, same read-side backstop).
- `session/instance_terminal.go`: `CaptureCurrentState` now calls `pathEscapesRoot` against the session's actual worktree path (`i.gitManager.GetWorktreePath()`) **before** persisting the captured pane path. An escaped path is logged and the capture is dropped (`WorkingDir` left at its last known-good value) instead of being written. This closes the gap at its source — a worktree session's `WorkingDir` can no longer be set to a path outside its own worktree at all, regardless of restart ordering or which code path happens to run first.

## Files Affected

- `session/instance_worktree.go` — new `pathEscapesRoot` helper; `resolveStartPath` refactored to use it
- `session/instance_terminal.go` — `CaptureCurrentState` gated on `pathEscapesRoot`

## Verification

- `TestPathEscapesRoot_should_returnTrue_When_CandidateIsParentRepo` — the direct regression check, using this exact live incident's real paths (the session's actual worktree path vs. the shared parent checkout path it escaped into).
- `TestPathEscapesRoot_should_returnFalse_When_CandidateIsRootItself`, `..._returnFalse_When_CandidateIsInsideRoot`, `..._returnTrue_When_CandidateIsSiblingWorktree`, `..._returnTrue_When_PathsAreUnrelated` — cover the boundary/sibling/fail-closed cases.
- `go test ./session/...` — full existing suite green, no regressions (including the 3 pre-existing `CaptureCurrentState` no-op tests, unaffected by this change).
- `go build ./...`, `golangci-lint run ./session/...` — clean.
- **Scoping note, same honesty standard as this session's other bug fixes**: `CaptureCurrentState`'s new gate is not covered by an end-to-end test with a real tmux-backed `Instance` — the function's early guards concrete-type-assert `i.processManager.(*TmuxBackend)`, and constructing a real (or realistically fake) tmux backend to drive a pane to a specific captured path is significantly heavier test infrastructure than this fix's scope justifies. The decision logic the gate depends on (`pathEscapesRoot`) is directly and thoroughly unit tested instead; the live incident itself, and this exact scenario's paths used verbatim in the regression test, are the evidence this fix addresses a real, reproduced defect rather than a hypothetical one.
- **Not verified**: this fix does not retroactively correct the *already-running* `stapler-squad-add-cron-schedule-builder-widget` session, whose pane cwd was still the shared parent checkout at the time this was written — it was still actively mid-task (open PR, running e2e tests) and was deliberately left alone rather than interrupted. Whether/how to recover that specific session is a separate, live-ops decision, not something this code fix resolves on its own.

## Reflection (Phase D — fix the class, not the instance)

**Classification**: API Contract Gap — `WorkingDir` implicitly carries the contract "always a path inside this session's operating root," but nothing in the type or the write path enforced it; only a downstream reader tried to defensively re-check an invariant that should have been guaranteed at the point of assignment.

**Earliest achievable enforcement**: The unit tests on `pathEscapesRoot` are the practical achievable level — this is inherently a runtime filesystem-path relationship, not something a static type can express in Go without a much heavier newtype-with-validated-constructor redesign of `WorkingDir` across the whole `Instance` struct (out of scope for this fix). Gating the single write site (`CaptureCurrentState`) that can introduce an invalid value is the correct-altitude fix: it makes the invariant hold by construction from that point forward, which is the spirit of "make illegal states unrepresentable" even without a type-system change.

**Recurring shape**: A textbook instance of "the guard existed, but only on the read side, so bad state could still be written and would only be caught if the read-side check happened to run under the right conditions" — closely related to (though a different specific mechanism from) BUG-030's swallowed-rollback-failure and BUG-029's wrong-session-selection: all three are cases where a piece of code correctly detects *something* is wrong, but the detection is positioned downstream of where the bad state was created rather than preventing its creation. Fifth bug found in this single session sharing the broader "an action's effect is trusted without being verified at its source" family — see `docs/tasks/backlog-feature-improvement.md` and the `backlog-feature-improvement` skill's "Prefer systemic fixes over instance patches" section. Worth a dedicated pass (flagged, not undertaken here) auditing every other `Instance` field that gets captured/persisted from live process state (not just `WorkingDir`) for the same write-side-validation gap.

## Related

- Backlog item `e1fb6825-39b2-4f06-9bf8-c9d1678a6824` ("Develop a system for our sessions to have awareness of other sessions working in the same workspace") is a related but distinct ask (peer *visibility*) — this fix addresses the actual isolation defect directly; peer-awareness would only have helped a human notice the collision sooner, not prevented it.
- This doc previously proposed a different root cause (a silent fallback in `resolveSessionPath`/`server/services/backlog_service_triage.go` when `CreateBacklogWorktree` fails) before the worktree was confirmed to exist and be valid. That fallback path is still real, still-unsafe code (a plain-directory fallback for a git-managed repo whose worktree creation genuinely fails would have the identical effect) — it just isn't what caused *this* incident. Tracked separately as a still-open, lower-confidence concern; worth a follow-up look at `resolveSessionPath`'s "not git-managed" vs. "worktree creation failed for an operational reason" distinction, same as this doc's original fix-approach section suggested, but not re-filed as its own bug doc since it wasn't reproduced.
