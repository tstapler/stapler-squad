# Adversarial Review — Core Domain Decomposition (Fourth Pass)

**Verdict: CLEAN**

Both required fixes from the third review are genuinely closed, verified independently — not by trusting the patch's self-report. `ADR-002` now correctly enumerates all four new services this project adds, cross-checked line-by-line against `plan.md`'s actual Stories 1.4/1.5/1.6 (not just checked for internal consistency within the ADR itself). `validation.md`'s R1.8 row now correctly attributes the standing-rule governance to `ADR-001` and the specific-service enumeration to `ADR-002`, matching R1.8's exact wording in `requirements.md`. A full fresh read of all three ADRs' substance (not just a grep for the previously-flagged phrases) found no other stale claim of the same class — every file reference, line number, and design claim re-checked against the current repo state still holds. One new, pre-existing (not introduced by this patch), non-blocking documentation-precision nit was found: an internal inconsistency in how many "already-established" sibling service files exist (26 vs 24 vs the actual 25), present since the original planning pass. It does not affect any task's scope or correctness and does not require a fix before implementation — noted for completeness only, same treatment the second/third reviews gave the constructor-signature nit.

This review was performed by direct, independent reading of `requirements.md` and `plan.md` in full, all three ADRs in full, `validation.md` in full, fresh `grep`/line-number verification against `session_service.go`, `session/instance_terminal.go`, `session/response_stream.go`, `session/claude_controller.go`, `config/config.go`, `server/dependencies.go`, and `server/services/*_service.go`'s actual file list, and a fresh cross-check of `instance-actor-concurrency/implementation/plan.md`'s Epic 5/Story 5.3/Task 5.3b line numbers.

---

## 1. `ADR-002`'s amendment — verified correct and cross-checked against `plan.md`

`ADR-002`'s Decision section now reads: "This project extracts **four** new services in total," and enumerates:

- `CheckpointService` and `AutonomousOrchestrationService` (Stories 1.1-1.2, the original two) — matches `plan.md` Stories 1.1/1.2 exactly.
- `TerminalService` (`GetTerminalSnapshot`/`WriteToSession`, Story 1.5) and `FeatureFlagService` (`GetFeatureFlags`/`UpdateFeatureFlag`, Story 1.6) — matches `plan.md` Stories 1.5/1.6's method names and rationale ("no existing sibling service is a natural home") verbatim in substance.
- `ListBranches`→`WorkspaceService` and `ArchiveWorkflowSessions`/`DeleteWorkflowFailedSessions`→`WorkflowService`, explicitly called out as delegations to **already-existing** services, not new-service governance — matches `plan.md` Story 1.4's Tasks 1.4a/1.4b exactly (`s.workspaceSvc.ListBranches(...)`, `s.workflowSvc` for the workflow pair).

The Amendment paragraph correctly frames this as a scope correction ("post-third-adversarial-review scope correction... corrected below to enumerate the project's actual current scope of four new services"), the Context section's original two-cluster framing is left in place as explicitly-labeled pre-correction history (not silently deleted, and not asserted as current truth), and the Consequences/Positive section was updated in lockstep to mention `TerminalService`/`FeatureFlagService` and the workspace/workflow delegations alongside the original two. **No remaining discrepancy between this ADR and `plan.md`'s actual task breakdown.**

## 2. `validation.md`'s R1.8 row — verified correct

Now reads: *"Already satisfied — `decisions/ADR-001` documents the standing rule; `decisions/ADR-002` (scope-corrected 2026-07-01) documents this project's specific new services (`CheckpointService`/`AutonomousOrchestrationService`/`TerminalService`/`FeatureFlagService`) plus the `ListBranches`/`ArchiveWorkflowSessions`/`DeleteWorkflowFailedSessions` delegations to existing services."*

Checked against R1.8's exact wording in `requirements.md`: *"Governance: this decomposition needs an ADR (see Item 3) recording the established `server/services/`-one-file-per-cluster convention as a standing rule... plus the new services this project adds."* The row now correctly splits the two halves of R1.8 across the two ADRs that actually do each job, rather than crediting one ADR with both. **Closed, no remaining misattribution.**

---

## 3. Full ADR substance sweep (not just phrase-grepping) — no other staleness found

Read `ADR-001`, `ADR-002`, `ADR-003` in full and independently re-verified every concrete, checkable claim in each against the current repo state:

| Claim | ADR | Verified against | Result |
|---|---|---|---|
| `session_service.go` is 4,542 lines | ADR-001 | `wc -l server/services/session_service.go` | Exact match |
| 26 `server/services/*.go` files total (incl. `session_service.go`) exist, established delegate pattern | ADR-001 | `ls server/services/*_service.go` → 26 files total | Matches — but see §4 for a related, separate count inconsistency |
| `Preview()`'s buffer branch reads via `ClaudeController.GetRecentOutput` → `PTYAccess.GetBuffer()`, filled raw by `response_stream.go`'s `streamLoop` | ADR-003 Amendment | `session/claude_controller.go:774` (`GetRecentOutput`), `session/response_stream.go:259-284` | Matches; `chunk.Data` copied raw into circular buffer, escape-parser call is passthrough-only |
| "passthrough - doesn't modify data" comment at `response_stream.go:277` | ADR-003 | `grep -n` on that exact string | Exact line match |
| `ClaudeController`'s own status-detection tail reads at `claude_controller.go:619,877` | ADR-003 | Read those line ranges | Both are `GetRecentOutput`/`GetBuffer` tail-reads inside `GetCurrentStatus`/idle-detection, consistent with the claim these paths need raw ANSI |
| `Preview()` is currently lines 103-125, two-branch, unnormalized | ADR-003 / `plan.md` Task 2.1b | `session/instance_terminal.go:103-125` | Exact match — function unchanged since third review, still the two-branch shape both `plan.md` and `ADR-003` describe as the starting point |
| `instance-actor-concurrency`'s Epic 5 / Story 5.3 / Task 5.3b line numbers (~2005/2076/2113) | `plan.md` Story 1.2 coordination gate | `instance-actor-concurrency/implementation/plan.md` | Exact match (2001/2076/2113) — no drift since third review |
| `InstanceSnapshot`/`buildSnapshot` still don't exist; no back-references from that project to this one | `requirements.md` Sequencing, `plan.md` Epic 3 gate | `grep -rn "InstanceSnapshot\|buildSnapshot" session/*.go`, `grep -rl "core-domain-decomposition" project_plans/instance-actor-concurrency/` | Both greps empty — still true, no active conflict today |
| `session_service.go` method line numbers (`buildTurnCallback` 758, `StreamTerminal` 1942, `ListBranches` 2985, `GetTerminalSnapshot` 3575, `WriteToSession` 3622, `onAutonomousDriverComplete` 3682, `GetFeatureFlags` 4007, `UpdateFeatureFlag` 4035, `ArchiveWorkflowSessions` 4275, `DeleteWorkflowFailedSessions` 4330) | `plan.md`/`requirements.md` various citations | Fresh `grep -n "^func (s \*SessionService)"` | All match plan.md's/requirements.md's citations exactly — no drift since third review |
| `server/dependencies.go`'s staged builders (`BuildCoreDeps`/`BuildServiceDeps`/`BuildRuntimeDeps`) exist, referenced by R1.4 | `requirements.md` R1.4 | `grep -n "func Build"` in `server/dependencies.go` | All three exist |
| `config.LoadConfig()`/`SetFeatureFlag()` exist, referenced by Story 1.6 | `plan.md` Task 1.6a | `config/config.go` | Both exist |

No claim in any ADR was found to still assert the disproven "two services"/"fallback-branch-only" premises as current truth. Every remaining mention of the old framing is explicitly labeled historical/corrected context.

---

## 4. New finding (pre-existing, non-blocking): sibling-service count is internally inconsistent (26 / 25 / 24)

Not a rediscovery of the "two vs. four new services" bug (that was about this project's own new services; this is about the count of **already-existing** sibling services used as a consistency reference point) — a distinct, smaller documentation-precision issue, present since the original planning pass and not touched by this patch:

- `requirements.md` (twice) and `ADR-001` all assert **26** existing focused service files, then `requirements.md`'s own parenthetical actually lists exactly **25** file names.
- Direct count: `ls server/services/*_service.go` returns 26 files total — but one of those 26 is `session_service.go` itself (the monolith being decomposed, not one of the "already-decomposed siblings" the prose is describing). Excluding it, the actual sibling count is **25**, matching the 25 names listed, not the "26" the prose states.
- `requirements.md` R1.6 separately says "consistency with **24** existing examples," and `plan.md` Story 1.5/1.6 say "none of the **24** already-extracted services" / "any of the **24** existing services" — a third, different number for what should be the same fact.

Net: three different counts (26, 25-by-enumeration, 24) appear across `requirements.md`/`ADR-001`/`plan.md` for the same underlying fact (how many sibling service files already exist). This does not change any task's scope, does not affect what gets built, and isn't a candidate for the "old scope" failure mode this review chain exists to catch (it doesn't understate this project's own new-service count the way `ADR-002`'s bug did) — it's a minor arithmetic/proofreading slip in descriptive framing. **Not blocking implementation.** Fix opportunistically if convenient, not required.

## 5. Carried-over, still-non-blocking nit (unchanged, re-confirmed)

Task 1.5a/1.6a's illustrative constructor signatures still don't match the actual current code shape — re-confirmed this pass: `FeatureController` is actually a local interface defined inline in `server/services/session_service.go:54`, not a `features.FeatureController` package type as Task 1.6a's sketch suggests. Both tasks already say "verify exact constructor dependencies/types during implementation," so this remains fully hedged. Not re-flagging as a required fix.

---

## Summary

Both of the third review's required fixes are closed and independently re-verified against source, not just against each other. A full substance-level re-read of all three ADRs (not just a grep for previously-flagged phrases) found no further instance of the "old, disproven scope" failure mode — every file reference, line number citation, and design claim checked out against the current repo state. The only new observation (§4) is a minor, pre-existing, non-blocking descriptive-count inconsistency unrelated to this project's actual task scope.

**This is pass 4 and, on the evidence gathered, should be the final review before implementation.** `plan.md` is internally consistent, its task arithmetic is correct (3 epics, 9 stories, 25 tasks, re-confirmed), every requirement ID in `requirements.md` traces to at least one task and one validation row, all cross-project citations and coordination gates are current and re-verified against both projects' live state, and the two previously-open governance-documentation defects are closed. No design question needs reopening. Recommend proceeding to implementation starting with Epic 2 (as `validation.md` already recommends), per this plan as written.
