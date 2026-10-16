# Cross-artifact consistency: go-git-fork-full-git-replacement

**Inputs**: `requirements.md`, `implementation/plan.md` (Revision 5), `decisions/ADR-001` to `ADR-006`. No `design/ux.md` exists (no UI surface), so the UX-Plan check is not applicable.
**Result**: 2 BLOCKER, 11 CONCERN, 7 NITPICK. Line numbers are in `plan.md` unless prefixed.

## BLOCKER

**B1. The no-git container run requires local-bare-remote clone and push that the plan routes to the CLI.** Conflict: plan Story 5.3.2 (lines 1323-1326) vs Story 3.2.1 (line 1081), ADR-003 tier 3, Story 5.3.1 carve-out table (line 1293).
- Story 5.3.2 AC 1: in a container with "no `git`", "clone from a local bare remote also succeeds"; AC 2: the flow includes "push to a local bare remote", with all backend spawn counters at 0.
- Story 3.2.1, ADR-003 and the gate table make local-path and `file://` remotes a permanent CLI carve-out (`capability_local_transport`), because go-git's `file` transport execs `git-upload-pack`.
- With no `git` on PATH those two steps cannot succeed, and the counters cannot be 0. Story 5.1.1 (line 1230, "helpers cover ... clone", shim count 0) has the same problem for fixtures.
- Resolution: the container flow clones and pushes over `git://` (or an in-process `PlainInit` remote) and lists local-path clone and push in `what-no-git-means.md` as tier-3 carve-outs. Reword 5.1.1 so the clone helper builds the remote in-process.

**B2. The "productionised" read-your-writes matrix still runs `Remove`, `Move`, `Reset(Hard)` and `Checkout` through `WithIndexLock`, but Revision 5 made them CLI-routed.** Conflict: Story 2.3.1 AC at line 973 vs the same story's AC at line 979, Task 2.3.2a (line 1017) and Revision 5 R3-2 (line 10).
- Line 973 requires the matrix (`Remove then Status`, `Move`, `Reset(Mixed|Hard)`, `Checkout`) to pass "through `WithIndexLock` in CI on every PR" with trees equal to the CLI twin's.
- Line 979 and Task 2.3.2a say those operations cannot be buffered, are "not in this list" and stay on `(operation, unsafe_worktree_write)`.
- An implementer cannot satisfy both. The S4 matrix (line 420) has the same list; it is acceptable as a feasibility probe but is not marked as such.
- Resolution: split the matrix. The production CI matrix covers only the promoted operations (`Commit(All|Amend)`, `Add`, `Restore(Staged)`, `Reset(Mixed|Soft)`, `AddWithOptions(All)`). The `Remove`/`RemoveGlob`/`Move`/`Reset(Hard)`/`Checkout` rows move to the fault-injection promotion test (line 427) and are labelled spike-only at line 420.

## CONCERN

**C1. Requirements are stale relative to decisions the plan has already made (requirements vs plan/ADR-003, ADR-005).**
- requirements.md Success Metrics 1 and 4 and Constraints state zero `git` spawns in the `session` test package, no `git` binary at runtime, and "every existing test passes with the CLI fallbacks removed". The plan re-scopes these to server-process-only (ADR-003 tiers, O-1/O-9) and keeps a permanent `cli` backend (ADR-004 item 7, Epic 6.2). `requirements.md` was not updated and O-1/O-9 are still open. Update the requirements, or record in the plan that they are overridden pending O-1.
- Constraint "must keep working with ... the ssh-fallback behaviour (HTTPS to SSH)" vs ADR-005 and Story 3.1.3: default off, opt-in, because research found the wrapper is a dotfiles convenience. The requirements' own Open Question is answered by research but the Constraint was never relaxed. Until O-5 resolves, the network cohort can regress the maintainer's HTTPS-only repos. Reword the Constraint or default the opt-in to on for the maintainer's instance.
- In Scope says "linked-worktree support ... in a go-git fork". The plan puts worktree code in-repo and expects an empty fork (plan lines 7, 93-102, O-6). That is flagged to Tyler at G7a but requirements.md still describes a patched fork.

**C2. Summed mutex delay is a requirement metric with no gate.** requirements.md Success Metric 3 requires wall time and summed mutex delay not to regress. Story 0.1.2 records mutex delay in the baseline; Story 5.1.2 (line 1240) and Story 6.1.1 gate only on wall time. Add the mutex-delay comparison to 5.1.2 and 6.1.1.

**C3. Observability requirement says "operation, call site"; plan and ADR-004 key on operation and reason.** Requirements Observability asks to count every CLI fallback by "operation, call site". The plan deliberately drops `callsite` (ADR-004 item 4, Observability Plan line 212) and keeps `file:line` only in the test dump. The reasoning is sound; record the departure in requirements.md so the traceability is explicit.

**C4. Promotion criteria assume shadow for every cohort, but `localwrite` and the write halves of `network` and `worktree` never shadow.** Story 6.1.1 (line 1348) requires "a cohort at stage `shadow`" with 7 days of zero `class="real"` mismatches. ADR-004 item 2 and Story 1.1.1 (line 563) force `localwrite=shadow` to resolve to `cli`. Risk Control Stage 0 says "shadow for read operations". There is no defined evidence step for `localwrite`, `worktree` and push/clone between "never shadowed" and "on gogit". Define a write-cohort promotion path (oracle plus soak plus dogfood window), or state that 6.1.1's shadow criterion applies only to `refs`, `diffstatus` and `network` reads.

**C5. Gate and story numbering for S7/G8 is off by one from the other spikes, and the glossary range is stale.**
- Spikes S1 to S6 map to Stories 0.2.1 to 0.2.6 and gates G1 to G6. S7 is Story 0.2.8 and gate G8, placed before Story 0.2.7 (the fork-necessity record, which carries G7a/G7). There is no spike S8.
- The glossary (line 156) says "G0..G7"; gates in use are G0 to G6, G7, G7a and G8.
- The dependency diagram (lines 273-277) omits the full G7 checkpoint.
- Resolution: fix the glossary to "G0..G8 plus G7a" and add G7 to the diagram. Renumbering is optional but the mapping S7-to-G8-to-Story-0.2.8 needs one explicit sentence in 0.2.

**C6. Epic 4.1 gate is stated inconsistently.** Header (line 1123): "conditional on G3". Dependency diagram (line 286): "4.1 worktree residue (G4)". Both gates probably apply (G3 for checkout perf, G4 because writing the admin files touches locks), but the two places disagree. State both in the header and the diagram.

**C7. Fork-patch IDs F1 to F5 collide with the fail-closed IDs F1 to F11.** Fork patches are F0 to F5 (lines 91-98; F3 "removed"). Story 1.3.4 (line 789) and ADR-006 reuse `F1`..`F11` for fail-closed cases, and (h) says "exactly F1 to F11 in (c)". "F1" now means an operation-level lock API in one place and an unreadable include target in another, and "F3" is both the removed index-v3 patch and include-depth. Rename the fail-closed set (for example `FC1`..`FC11`) in the plan and ADR-006.

**C8. The credential provider's reuse of `github/keychain.go` conflicts with Story 1.1.0's dependency acceptance test.** ADR-005 and Story 3.1.1 reuse the host-scoped token resolution in `github/keychain.go` from `session/git/backend/gogit/credential`. Verified: `go list -deps ./github` includes `config`, `executor/safeexec`, `session/git`, `session/tmux` and `session/lifecycle`. Story 1.1.0's AC requires `go list -deps ./session/git/backend/gogit` to list none of those. Importing the `github` package would fail that check. Extract the keychain and host-resolution code into a leaf package, or inject it as a function value as Task 2.3.1c does for the session registry.

**C9. ADR-005 and Story 3.1.1 disagree on when the credential-helper binary runs.** ADR-005: keychain then `gh` token first, the helper binary "as an opt-in fallback". Story 3.1.1 AC: "if neither is found it execs the configured `credential.helper` binary directly", which is not opt-in. Pick one. The requirements Constraint (system credential helpers) favours the always-on fallback.

**C10. Story 3.2.2 uses fallback reasons outside the closed enum.** Line 1101: "the CLI path runs (reason `error`/`capability`)". `capability` is not a `FallbackReason` (glossary line 139), and `error` is gate-blocking per Story 6.1.1, which would make every diverged `Pull` a cohort-flip blocker. A diverged or `pull.rebase` pull needs its own enum value (for example `unsupported_pull_mode`). Also, no precedence is defined when several reasons apply (a hooked repo running `Remove` is both `capability_hooks` and `unsafe_worktree_write`). State the evaluation order in Story 1.1.3.

**C11. Scope beyond the written requirements.** These stories have no requirement line and rest on derived requirements (parity, no user-visible change, credential security). Name them in requirements.md as derived scope, or cut them under the drop order at 0.3.1.
- The own git-config resolver, tokenizer and wildmatch (Story 1.3.4), roughly the largest single story.
- The lock journal, xattr tokens and stale-lock recovery (Story 2.3.1).
- The reflog writer (Task 2.3.1d).
- The `norawgitcli` and `norawgitpath` analyzers and the `spawngate` helpers.
- The package-topology extraction (Story 1.1.0).
They are justified by requirements Constraints ("must not weaken", "behaviour must match the CLI") but add well beyond the 3 to 6 week appetite; the calendar at 0.3.1 does not budget Story 1.3.4 or 2.3.1 explicitly.

## NITPICK

- **N1. "62" is used for two meanings.** ADR-004 (Context) calls it "62 `"git",` construction sites"; Pattern Decisions (line 167) says "Eliminates text parsing at 62 sites"; Tech Debt (line 183) says "62 scattered CLI sites". Story 1.1.4 (line 656) says 62 is the `"git",` literal count including argument tables and test helpers, and that the constructor-site count is 36 plus 25 runner sites. Use "62 `"git",` literals in 29 files (36 constructor and 25 runner sites)" once and refer to it.
- **N2. Counts reconcile.** 62 sites in 29 files, 223 test sites (121 under `session/`, 102 elsewhere, 223 - 121 = 102), 31 in `review_gate_test.go`, 30 go-git importers, 36 constructor sites in 19 files, 25 runner sites, 10 direct test `exec.Command("git"` sites, 20 PATH sites, 29 LookPath sites are consistent across the plan, ADR-001, ADR-004 and requirements. Requirements' "about 50 / about 150" is reconciled in Story 0.1.1.
- **N3. ADR-001 vs ADR-002 on the base tag.** ADR-001 says fork at `v5.19.3`; ADR-002 and Task 1.2.2a say run S1 at `v5.19.2-ssq.0` first and bump to v5.19.3 afterwards. Add "after the S1 bump sequence in ADR-002" to ADR-001's Decision.
- **N4. ADR-004 summary of native-to-`session/git` dependencies is shorter than the plan's.** ADR-004 names `WithRepoWorktreeLock`, `getHeadCommitSHA`, `FetchBranch` and `IsDirtyCleanCacheTTL`; the plan (Story 1.1.0, rule 8) lists eight, including `OpenRepo`, the two `*GitWorktree` methods, `MergeMainResult` and `gitignoreFSCache`. Not a contradiction; point ADR-004 at Story 1.1.0 for the full list.
- **N5. ADR-006 hook-gating examples are narrower than plan (e).** ADR-006 lists `Add`, `Remove`, `Restore`, `BranchRename`, `BranchDelete`, `SetUpstream`; the plan also gates `AddWorktree*`, `Fetch`, `Push`, `Pull`. ADR-006 omits the "I/O error reading a required file" case from its fail-closed list (plan (h)). Align.
- **N6. Unverified first-person claim left in plan text.** Task 2.3.2a (line 1017): "I did not re-open those lines in this pass" for `session/vcs/git.go:243-310` and `session/vc/git_provider.go:563,568`; line 987 also carries "I could not reproduce it". Replace with a VERIFIED or INFERRED label before Phase 5.
- **N7. Epic 1.2 prerequisites.** Header says "needs G1, G2" and "Runs only after G7a"; the dependency diagram (line 286) lists only G1, G2. Add G7a.

## Checks with no finding
- Terminology: cohort names (`refs`, `diffstatus`, `localwrite`, `network`, `worktree`), `Backend`/`RepoLocation`/`Local`/`Remote`/`Runner`, and `native`/`gitwiring` roles are used identically in ADR-004 and the plan. `Locate` is in `backend` everywhere it is mentioned (ADR-004, glossary, Story 1.1.0 rule 5, Story 1.1.5); no stale `gitwiring.Locate`.
- Index v3/v4 premise: no stale reference. F3 appears only struck through (line 96), in the changelog and in line 505 ("F3 was removed"); `capability_index_v3` is gone from the enum; ADR-006 carries the same correction.
- ADR numbers 1 to 6 are referenced consistently; no ADR-007. O-1 to O-11 are all defined (O-8 resolved); every ADR open-decision reference (O-1, O-2, O-4, O-5, O-10) resolves to a defined item.
- `FallbackReason` values used in the plan and ADRs all appear in the glossary enum except the two flagged in C10.
- Requirements coverage otherwise: worktree (Epic 4.1 plus existing native code), rev-parse family (2.1), diff/status (2.2), merge-base/log (2.1), network (3.x), fixtures (5.1), rollback (Risk Control, ADR-004), observability (1.3), no-git container (5.3.2), upstreaming optional (4.2.4), rebase burden (6.3).
