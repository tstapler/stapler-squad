# Cross-artifact consistency review: bazel-conversion

**Date**: 2026-11-01
**Artifacts checked**: `requirements.md`, `implementation/plan.md` (1845 lines), `decisions/ADR-001..008`. No `design/ux.md` exists (infrastructure project), so the UX-plan check is not applicable.
**Totals**: 1 BLOCKER, 8 CONCERN, 4 NITPICK.

## Verified consistent (no finding)

- Windows = cross-compile only: requirements.md Constraints, ADR-005, plan Epic 6 and the plan header's correction note agree. `target_compatible_with` handling matches.
- Remote cache = self-hosted `bazel-remote` on onyx: requirements.md Constraints, ADR-001 row 2, ADR-004 and plan Story 0.12/7.4 agree. Ports `:9092`/`:9093`, tags `tag:ci-pr`/`tag:ci-main`, the `bazel-cache-write` environment and the credential helper match across plan and ADR.
- Forks allowed: requirements.md Constraints, ADR-008 and plan Task 0.14b agree on `git_override`/`archive_override` pinning and the fork repo owner step. The ADR-008 cost matrix classes equal the plan's fallback-matrix classes (bun.lock, embed, ent, connect-go, tmux).
- cgo resolved: requirements.md Open Questions, ADR-006 and plan Task 1.1b/1.1c agree (modernc in production, mattn in 2 test files, 3 darwin cgo files).
- End state = Bazel build of record with make wrappers: requirements.md, ADR-001, plan Epics 8-10 agree. Rollback via `BUILD_SYSTEM=make` for one release cycle matches Story 10.1.
- Numbering and counts (recomputed by script): plan header "12 epics, 70 stories, 168 tasks" equals the actual count. Per-story task counts in the Effort table equal the actual `##### Task` counts for all 70 stories. Raw tokens 13060k and CU 14.38M x 1.75 = 25.16M recompute correctly.
- ADR cross-references that resolve: Task 0.14c, Story 7.3, 7.4, 7.7, 8.4, 8.5, 11.7, 11.8, Task 1.1b, Task 7.4b, spike ids S1-S12. Stale-path check: no BUILD file in the plan sits under `build/` or `third_party/tmux`; the only `build/` hits are `web-app/build/build.ts` and the `build/windows_amd64` output name, both legitimate.

## BLOCKER

### B1. ADR-003 and plan spike S4 disagree on what "default" and "fallback" mean for the Bun web build
- **Conflict**: ADR-003 vs plan Fallback matrix row S4 and Story 0.6.
- ADR-003: the default is the repo-owned Bun repository rule, and if it "fails hermeticity or cacheability" the fallback is a pinned third-party `rules_bun` ("Workaround 2"); if both fail, fork `aspect_rules_js`.
- Plan S4 row: the Bun repository rule is the **Workaround path** column, the fork is the Fork path, `rules_bun` is listed as a second workaround, and "Default if Pass fails" is again "Bun repository rule (ADR-003)". Story 0.6 turns a failed parity or sandbox-cost check into `FallbackWorkaround`, whose resulting action (consuming actions tagged `no-sandbox`) still uses the repository rule, not `rules_bun`.
- Effect: a spike that tests the repository rule (Task 0.6a) cannot both "Pass" and be the "Workaround", and the plan's failure path never reaches `rules_bun`, which ADR-003 says is the next step. The implementer cannot tell which verdict selects which path, and Story 0.14 cannot validate it mechanically.
- **Resolution**: make one vocabulary. Either the repository rule is `Pass` and `rules_bun` is `FallbackWorkaround` (ADR-003 wording; update the plan row), or amend ADR-003 so the sandbox-cost and delta failures map to `no-sandbox` on the same rule, and only hermeticity or cache failures reach `rules_bun`.

## CONCERN

### C1. ADR-004 cites a task that does not exist
- ADR-004 Decision (D-PROTECT bullet) says "Before the write credential is created (Task 0.12pre)". Plan has Tasks 0.12a, 0.12b, 0.12e, 0.12c, 0.12d only; the prerequisite is Task 0.12e. **Resolution**: change `0.12pre` to `0.12e`.

### C2. "Three regex lists" wording: requirements and ADR-001 vs the plan's correction
- requirements.md (Problem Statement, Metric 1) and ADR-001 (row 5: "three regex lists remain"; Decision: "retires all three lists") say three regex lists. Plan Tech Debt row (line 83) verified they are three different mechanisms (one `grep -qE` regex in `build.yml:97`, `detect-feature-changes.sh`, native `paths:` filters in `mcp-integration.yml`), and Story 7.3 and Task 10.2c are written to that. **Resolution**: amend requirements.md and ADR-001 to "three hand-maintained selection mechanisms", or add a one-line note that the plan corrected the count.

### C3. requirements.md says tmux is built from the `third_party/tmux` submodule; plan and ADR-007 build from a pinned release tarball
- requirements.md In Scope: "tmux build from the `third_party/tmux` submodule via a bzlmod-compatible rule". Plan Tech Debt, Story 0.7/3.3 and ADR-007: Bazel targets live at `tools/bzl/tmux/`, source is an `http_archive`, the submodule is a version pin compared against `tools/bzl/tmux/PIN`. The change was a deliberate review repair (arch B1) but requirements.md was never amended. **Resolution**: add a one-line requirements.md amendment (decision recorded, reason: a gitlink cannot hold tracked BUILD files).

### C4. requirements.md "Open Questions" is stale relative to the plan, and the plan's "Unresolved Questions" does not carry them
- Open in requirements.md: (a) which lint gates become Bazel targets versus wrapped, (b) can `aspect_rules_js` consume `bun.lock`, (c) `tools/lint` separate-module handling "still to be spiked".
- (a) is answered by ADR-005 and Story 8.3; (b) is answered by ADR-003/ADR-008 ("no", default Bun repo rule, pending S4); (c) is Task 0.2c/S-gomod. None appears in plan `## Unresolved Questions`, and requirements.md is not struck through. **Resolution**: strike through (a) and (b) with pointers (ADR-005/Story 8.3; ADR-003/S4), and mark (c) as pending S-gomod; no new plan entries needed.

### C5. `godebug netdns=go` is a listed rabbit hole with no plan coverage
- requirements.md Rabbit Holes names the `godebug netdns=go` setting. `go.mod` line 14 declares it. `grep -c 'godebug|netdns' plan.md` is 0. Nothing verifies that rules_go-built binaries honour the `go.mod` `godebug` directive, which affects DNS resolution of the pure-Go build. **Resolution**: add a check to Task 3.4 or P5 (`compare-binary.sh`): the binary's `runtime/debug` build settings or a behaviour probe equals the make binary.

### C6. Homebrew `Formula/` is in requirements.md scope but unmentioned in the plan
- requirements.md In Scope: "release-please/release artifacts, Formula/". `grep -c Formula plan.md` is 0. `Formula/stapler-squad.rb` is updated by `.goreleaser.yaml` (`brews.directory: Formula`, line 52), so Story 3.7 (goreleaser) probably covers it implicitly, but no AC checks that the formula update still works with Bazel-built or `prebuilt` artifacts. **Resolution**: add one AC line to Story 3.7 / Task 3.7a that `goreleaser-check.yml` (or a dry run) still renders the formula; severity is CONCERN because the gap is plausible but unconfirmed.

### C7. "D-PROTECT" names two different owner decisions
- Plan Unresolved Questions line 112 (cache-write environment gate, needed in Epic 0, matches ADR-004) and line 113 (main-branch required checks at the Story 7.6 flip) both use the label D-PROTECT. ADR-004 uses it for the first only. **Resolution**: rename the second (for example D-PROTECT-MERGE) so Story 7.6 and Task 0.12e cite unambiguous ids.

### C8. Custom Gazelle extension stance differs between requirements and plan/ADR-008
- requirements.md Constraints: "write custom Gazelle extensions where a pattern repeats." Plan Pattern Decisions: "Custom Gazelle extension / repository rules by default" is the rejected alternative; ADR-008 rule 5: extensions "only after directives plus a macro demonstrably fail". S11 does keep an extension as the explicit fallback, so the owner's allowance is preserved, but the default order is reversed from the requirement's tone. **Resolution**: add a sentence to ADR-008 rule 5 noting this is a sequencing preference within the owner's allowance, not a prohibition.

### C9. Plan status says "Ready for implementation" while four ADRs are Proposed
- ADR-002, ADR-003, ADR-007 are `Proposed`; ADR-004's reachability, read-only enforcement and redaction parts are `Proposed`. Plan text (Pattern Decisions rows for generated code and embeds, Task 0.2e's `hand-build-files.txt` naming `tools/bzl/tmux|flags|ent|proto|windows`) already assumes their outcomes, before Story 0.14 flips them. The plan does gate this correctly (Epic 0 gates everything; Task 0.14a checks `Status: Accepted`), so there is no conflict, only a header that reads as stronger than the state. **Resolution**: change the plan header Status to "Ready for Epic 0; ADR-002/003/004/007 become Accepted at Story 0.14".

## NITPICK

### N1. Effort table story titles are truncated
- Plan Effort table rows (for example "7.3 Affected-target selection replacing the three re", "0.3 Spike S1 - proto Go + connect-go + TS through Ba") cut titles at ~50 characters, and the 7.3 row still says "three re..." while the story title was changed to "hand-maintained selection logic". **Resolution**: regenerate titles in full.

### N2. Task ids are out of lexical order in several stories
- File order: 0.2e before 0.2d, 0.3e before 0.3d, 0.6e before 0.6d, 0.12e before 0.12c, 7.5e before 7.5d, 10.2c before 10.2b, and Story 4.10 sits between 4.7 and 4.8. These are intentional (execution order) but a reader scanning for `X.Yd` will not find it where expected. **Resolution**: leave, or add "(runs before Xd)" to the later task headings.

### N3. Task 9.1c is a pointer task counted as work
- Task 9.1c "(moved to Task 7.5e)" is counted in the 168 tasks and the Story 9.1 estimate (1M, 1S, 2XS). It does docs-only work (`grep -c bazel-manual-instance` check), which is legitimate, but the heading reads as a tombstone. **Resolution**: retitle it "Point docs at scripts/bazel-manual-instance.sh".

### N4. Story 2.6 AC is narrower than requirements Metric 2
- requirements.md Metric 2: `bazel build //...` works on a fresh clone. Story 2.6 AC: exit 0 "for all non-tag-gated, non-Windows, non-embed-dependent targets". The full `//...` claim is only asserted later by P3 (Story 7.5), which is where the metric is finally met, so it is consistent end-to-end. **Resolution**: add "(full `//...` is asserted by P3 after Epic 3)" to Story 2.6.

## Coverage and scope-drift summary

- Coverage: every requirements.md In Scope bullet maps to at least one story, except `Formula/` (C6, implicit) and the `godebug` rabbit hole (C5). The feature-registry "if applicable" generated input appears only in the affected-targets story; it is not a gitignored output, so treated as not applicable (no finding).
- Scope drift (plan stories with no requirement): Story 7.7 (determinism and cache-poisoning guard), Story 8.7 (test summary instead of gotestsum), Story 11.6 (reviewability of BUILD changes), Story 11.8 (upgrade canary) are derived from requirements Risk Control, Feasibility Risks and agent-authored-repo risk rather than named in In Scope. Reasonable and traceable; no finding.
