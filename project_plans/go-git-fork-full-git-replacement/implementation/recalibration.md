# Re-baselined calendar and effort (Story 0.2.9)

Date: 2026-10-08. **Provisional: no gate date here is binding.** Basis: spike evidence in `gates.md` and the story and task counts in `plan.md`. S1 to S7 have all reported, so Story 0.2.9's precondition holds, but G3 (writes, edge probes), G4 (linked worktrees and most of Story 2.3.1's bar) and G5 (live hosts) are only partly covered, which widens the ranges below.

## 1. Honest limits of these numbers

- **There is no measured velocity.** Nothing in the spikes times how long implementation of a story takes. Phase 0 ran as a burst of agent work in one session; its wall-clock is not recorded in any spike file, so it cannot calibrate the estimates.
- **The plan's "~5 min" per task is an agent run time**, not a schedule. 114 tasks at 5 minutes is under 10 hours, which ignores review, CI, debugging, shadow windows and dogfood. I did not use it.
- Estimates below are **effort-days of agent-assisted work by one maintainer reviewing** (one stream), sized S/M/L/XL by story content and by what the spikes already built. They are my judgement, labelled GUESS or LOW/MEDIUM confidence. Calendar = effort divided by a parallelism factor of **1.3 to 2.0** (the plan itself assumed two agents in parallel; most of the work touches `session/git` and `session/vc`, which limits it), then fixed windows added.
- Fixed windows come from the plan, not from evidence, and are provisional (O-12): 7-day `refs` shadow window with at least 500 shadowed calls per op family; 14-day write-cohort dogfood; one full release cycle before Epic 6.2 (release-please cadence is not measured here).

## 2. Plan size (VERIFIED: `grep` over `plan.md`)

56 stories and 114 tasks. Original-appetite scope (the MVP slice of plan section 0.3.1) is 44 stories and 90 tasks (Epics 0.1, 0.2, 1.1, 1.2, 1.3, 2.1, 2.2, 4.1, 5.1, 5.2 read/worktree part, 5.3, 6.1 to 6.3, with Epic 4.2 at zero patches). The stretch scope Tyler added (O-13) is Epic 2.3 (2 stories, 8 tasks), Epic 3.1 (3, 6), Epic 3.2 (3, 5), plus resolver v2 (inside Story 1.3.4) and the write and network variants of the soak and rollout. Story counts understate the stretch scope: Story 2.3.1 alone carries the lock layer, journal and token recovery, reflog writer, fault injection and a macOS plus Linux CI matrix.

## 3. Effort and calendar by epic (full scope)

Effort in single-stream effort-days. Evidence column cites what the spike already de-risked.

| Epic / item | Effort (days) | Confidence | Evidence and reasoning |
|---|---|---|---|
| Phase 0 remainder: Task 0.2.3b edge/index v3-v4 probes, write benchmarks (worktree add, commit), real fork creation and CI check (Story 1.2.0/0.2.1c) | 1.5 to 3 | Medium | G3 and G1 "not verified" lists; the probes are small harnesses on top of S3 |
| 1.1 Seam, topology, router, CLI backend, lint rule, wiring (6 stories, 14 tasks) | 6 to 10 | Low | S0: 62 sites in 29 files, 30 importers; S2: go-git import graph is closed (no third-party importer); topology (1.1.0) is a verified blocker; no prototype exists |
| 1.2 Fork repo and `replace` (4 stories) | 1 to 2 | Medium | S1: mechanics proven end to end, 0 rebase conflicts |
| 1.3 Redactor, counters, oracle harness, capability preflight, **resolver v1** (4 stories, 12 tasks) | 5 to 8 | Low | S5 supplies the ssh_config probe and `Match` rule; resolver v1 is conservative route-to-CLI; no code exists |
| 2.1 `refs` cohort (3 stories, 8 tasks) | 4 to 7 | Medium | S3: Head/ResolveRevision/MergeBase pass; S7: predicate, `ErrRefDangling`, retry; S6: retry wrapper. Plus the 7-day shadow window (calendar, overlaps) |
| 2.2 `diffstatus` with status/diff on CLI (2 stories) | 2 to 3 | Medium | S3: `IsDirty` stays on the existing fast path; destructive-intent fail-closed per S0 |
| 4.1 `worktree` residue (3 stories, 6 tasks) | 3 to 5 | Low | Reuses native code (plan 0.1); linked-worktree locking untested in S4 |
| 5.1 Fixture migration (223 test sites, 3 stories) | 8 to 15 | Low | S0: 1,006 spawns, 61 to 72 s; part done in #955/#956; 121 sites under `session/` |
| 5.2 Soak, read and worktree variants | 2 to 4 | Low | S6/S7 harness code is reusable |
| 5.3 Zero-spawn gate and no-git container (2 stories, 5 tasks) | 4 to 7 | Low | S5 spawn shim approach verified; 20 `Setenv("PATH"` and 29 skip sites to handle |
| 6.1 Staged rollout (read cohorts) | 2 to 3 | Low | no evidence yet |
| 6.3 Fork runbook and rebase burden (2 stories) | 2 to 3 | Medium | S1 F6 gives the first rebase number |
| **Subtotal: original-appetite scope** | **40.5 to 70** | | |
| Resolver v2 (full git-compatible resolver, own tokenizer, `hasconfig`) | 5 to 9 | Low | Plan Story 1.3.4 design; S5 F3/F6 show how easy it is to be silently wrong |
| 2.3 `localwrite` (lock layer, ref writer, reflog, journal, fault injection, hooks preflight) | 15 to 30 | **Low (widest range)** | S4 core: 250-line prototype passes lost-update, partial-read, read-your-writes (9 ops), abort consistency. Unrun: linked worktrees, ref delete, `packed-refs.lock`, `HEAD.lock`, `config.lock`, reflog, journal/xattr recovery, fault injection, index v3/v4. Several of those could change the design |
| 3.1 Credentials and transport (3 stories, 6 tasks) | 5 to 9 | Medium | S5 prototype: 24 tests, `-race` green; F1 to F8 known. Gaps: live hosts, real keychain injection |
| 3.2 `network` cohort (3 stories, 5 tasks) | 5 to 9 | Low | S5 fallback and `insteadOf` verified; `file://` in-process server is an open alternative |
| 5.2 write and network soak variants, plus extra CI | 2 to 4 | Low | S4 T13/T6 are the standing tests |
| 6.1 extra promotion evidence for write and network cohorts | 2 to 4 | Low | plan Story 6.1.1 evidence rules |
| 6.2 Flag and fallback removal | 1 to 2 | Medium | small; happens one release later |
| **Subtotal: stretch scope added by O-13** | **35 to 67** | | |
| **Total (full scope)** | **75.5 to 137** | | |

**Not in the total (no evidence, would need new spikes):**
- In-process status list, diff text and numstat (decision, section 5): GUESS 8 to 20 effort-days. S3 shows only that go-git `Status` and `Patch.Stats` are 240x and 3.6x to 9x slower and that `Status` returns 896 entries where the CLI returns 2. No wrapper was prototyped.
- Eliminating the O-14 conflict items (a) to (e) (in-process `Checkout`, `Reset(Hard|Merge)`, `Restore(Worktree)`, `Remove`/`Move`, local-path remotes, hooked and signed repos, ref deletes): **not estimated.** S4 measured that go-git `Checkout` and `Reset(Hard)` diverge from CLI semantics (and `Reset(Hard)` deletes untracked files), so these are rewrites with parity risk, not wrappers.
- Whole-repo go-git v6 migration: not needed (G2).

### Calendar (weeks from W0 = the day Tyler answers G7/G7a)

Calendar = effort / (1.3 to 2.0), 5 days per week, then fixed windows. All GUESS.

| Segment | Effort | Calendar |
|---|---|---|
| Original-appetite scope (MVP slice, through `refs`, `diffstatus`, worktree residue, fixtures, zero-spawn gate, read rollout) | 40.5 to 70 | **4 to 11 weeks** |
| Stretch scope (resolver v2, `localwrite`, credentials, network, extra soak and rollout) | 35 to 67 | 3.5 to 10.5 weeks, after GL |
| Write-cohort dogfood (14 days, plan) | n/a | +2 weeks, mostly at the end |
| **Full scope** | 75.5 to 137 | **about 10 to 24 weeks** (central guess 15 to 16) |
| Epic 6.2 flag removal | 1 to 2 | one release cycle after default-flip: cadence not measured, **no estimate** |

## 4. What the 3 to 6 week appetite covered versus what the full scope adds

- The original appetite (plan Status line, section 0.3.1) was 3 to 6 weeks (15 to 30 working days) for the MVP slice. Applying the same method to that slice gives **4 to 11 weeks**, so the low end of the appetite (3 weeks) is not supported by this evidence even for the MVP, and 6 weeks sits near the middle of my range, not the top. The two likeliest reasons for the gap are the fixture migration (8 to 15 days, 223 test sites) and the seam and topology work (6 to 10 days); both are GUESSes with no prototype.
- The full scope adds about **35 to 67 effort-days, or 3.5 to 10.5 calendar weeks, plus 2 weeks of dogfood**, dominated by `localwrite` (15 to 30 days, widest range). It roughly doubles the MVP estimate.
- The spikes changed estimates in both directions: they de-risked F1 and F2 (no fork patches, so no Epic 4.2 work), credentials and fallback (S5 prototype), and the repack and `refs` retry designs; they added work for status/diff (S3), retry cost and batching (S6), `ErrRefDangling` (S7), preflight additions (S5 F3), and the unrun S4 items.

### Gates re-dated (weeks from W0; all provisional, not binding)

| Gate | Plan said | Now (range) | Basis |
|---|---|---|---|
| G7a confirm-only | end of week 1 | W0 (Tyler's answer), then Phase 0 remainder 1 to 2 weeks (Task 0.2.3b, write benchmarks, real fork) | G1/G3 open items |
| T3 re-plan tripwire (resolver v1 green, `refs` entered shadow, GS-1) | end of week 3 | **week 3 to 6** | Phase 0 remainder + 1.1 + 1.3 v1 = 13.5 to 23 effort-days at 1.3 to 2.0 parallelism, about 1.5 to 3.5 weeks, plus overlap |
| GS-1 (spawns down 30%: 1,006 to 704 or fewer) | end of week 3 | week 3 to 6 | S0 baseline; fixtures start week 1 per plan |
| GS-2 (spawns down 60%: 402 or fewer) | end of week 5 | week 5 to 11 | fixtures 8 to 15 effort-days |
| GL (MVP shipped, GS-2 met, resolver v1 routing share, S4/S5 outcomes) | after MVP | **week 5 to 12** | MVP slice 4 to 11 weeks, plus the 7-day shadow window |
| Full-scope done (excluding Epic 6.2) | not stated | week 10 to 24 | section 3 |

The 0.25% soak ceiling (G8) and the "retry at least 3 attempts" rule (G6) are measured numbers and replace the plan's provisional 1% and "retry once".

## 5. Decisions Tyler must make (G7 and G7a)

| ID | Question | Evidence that bears on it | Default the plan uses if silent | Schedule effect |
|---|---|---|---|---|
| **G7a** | Confirm the empty pinned fork F0 as the deliverable | `fork-necessity.md`: no spike proved a patch necessary (F1 and F2 out, F4 unproven) | Confirmed | none |
| **G7** | Overall checkpoint on the evidence and this calendar: accept it, or re-plan scope? | sections 3 and 4 | needs an answer | the whole calendar |
| **F4 / status-and-diff (new, from S3)** | In-process status list, diff text and numstat, or keep as CLI carve-outs? | G3: 240x, 3.6x to 9x slower, 896 vs 2 entries; no wrapper prototyped | Keep CLI, list under O-14 (new item i) | +8 to 20 effort-days if in-process (GUESS) |
| **O-2** (ADR-006) | Hooked or signed repos: route to CLI (default) or implement in-process? | S3 probe of hooks/`Commit(Amend)` not run; S4: go-git `Commit` runs no hooks | Route to CLI | blocks Epic 2.3; in-process adds an unmeasured amount |
| **O-10** | Does `gh pr create` stay as a carve-out, or move to the GitHub REST API? | not spiked | Carve-out (tier 2): the server spawns `gh`, which runs git itself | blocks the wording of Story 5.3.2; REST replacement is small to medium, unestimated |
| **O-11** | `session/vc` vs `session/vcs`: merge, delete or leave | S0: both live, disjoint capabilities, `vcs` untested; recommendation is merge after the Epic 2 facade, not delete | Merge into `vc` after the facade; leave until then | affects Epic 2.1 migration order |
| **O-12** | Provisional thresholds (GS-1 30%, GS-2 60%, resolver v2 trigger 10%, 500 shadowed calls, 20 status calls per repo, 10 green runs, 14-day dogfood) | GS thresholds are now concrete: 704 and 402 spawns for the `session` package (S0, a floor); soak ceiling 0.25% (S7) | Keep as proposed | none if kept |
| **O-14** | For each carve-out that spawns git from the server (a to e, plus new i), eliminate or keep and list | S4: `Checkout` and `Reset(Hard)` diverge from CLI semantics in go-git; S5 F7: local transport spawns git, in-process `file` server is a candidate; S7/S6: `object_missing`/`object_not_found` routes remain | Keep (a) to (e) and (i) as counted, ratcheted carve-outs | each elimination adds unestimated work and likely new spikes |
| **O-15** | HTTPS-to-SSH fallback default on or opt-in | S5: mechanism verified; token never goes to the SSH host by construction; GitHub's bad-token status unverified | Default off | none |
| New (S1) | Owner and name of the public fork (Story 1.2.0) | S1 used a local stand-in; no repo created | needs an answer before Story 1.2.0 | blocks Epic 1.2 |
| New (S0) | Fix the pause/stop `Remove()`-after-`IsDirty`-error path as a small standalone change? | S0 audit D2/D3 | Recommend yes, independent of this project | S, about 0.5 to 1 day |

**Accept this calendar, or re-plan scope?**

I stop here. No code, plan.md, ADR, repository or GitHub change was made by this pass; the plan edits implied are listed in `fork-necessity.md` section 4 for your approval.
