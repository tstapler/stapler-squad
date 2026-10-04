# Adversarial Review: worktree-base-branch-and-capture-dup-check

**Date**: 2026-09-24
**Verdict**: CONCERNS

**Post-review note**: since verdict was CONCERNS (not BLOCKED), the sdd:3-plan
repair loop does not require another re-review iteration before proceeding.
The coordinator nonetheless applied direct, targeted fixes to plan.md for all
2 open Concerns and both new Minors below (SkipReviewGate mechanism corrected
to an explicit stated assumption; hallucinated ADR-001 citation removed;
Branch 3's uncovered sub-case given an explicit no-executable-action caveat;
Epic 1.3 follow-ups given an owner note) rather than spinning another
subagent round for changes this small. Findings below are left as originally
written for the audit trail; plan.md itself now reflects the fixes.

## Blockers

None.

## Concerns

- [ ] **Item 1 (prior BLOCKER) — RESOLVED.** Story 1.1.2's AC5 (`plan.md:201-239`) now defines three Given-When-Then branches: linked/work-role → `report_duplicate` direct (transitions to `review`); unclaimed → `report_duplicate` dispatching to `reportDuplicateUnclaimed` (archives directly); linked/non-work-role (the likely case here) → `submit_triage_result` naming the duplicate finding and instructing the next session/human to run `/backlog/duplicate`. Task 1.1.2a (`plan.md:243-267`) routes among all three, Task 1.1.2b (`plan.md:269-304`) gives each branch a concrete terminal MCP call, and Task 1.1.2c (`plan.md:306-319`) reads back the resulting state for each branch — including the honest caveat that Branch 3's action defers rather than completes closure. Verified against code: `report_duplicate`'s role check (`server/mcp/tools_backlog.go:2212-2213`), `reportDuplicateUnclaimed`'s active-session guard (`:2419-2425`), and `submit_triage_result`'s existence + `Role != "triage"` rejection (`:2452`, `:2482`) all match the plan's citations exactly.

- [ ] **Item 2 (prior Concern 1) — PARTIALLY RESOLVED.** Task 1.1.2a (`plan.md:248-252`) now adds a pre-flight step: "Also read `SkipReviewGate` off that same `get_backlog_item` response... route to whatever `request_review`-based guidance the response gives instead." This is not actually executable as written: I read `getBacklogItem`'s full handler body (`server/mcp/tools_backlog.go:394-548`) end to end, and it never writes `SkipReviewGate` (or any variant) into the response text builder (`sb`) — confirmed by `awk 'NR==394,NR==549' server/mcp/tools_backlog.go | grep -ni skip`, zero hits besides an unrelated comment. `listBacklogItems` (`:857-960`) doesn't surface it either. So an agent literally cannot "read `SkipReviewGate` off the `get_backlog_item` response" — the field isn't there. The task's fallback, "or the backlog UI," does carry the value, but only inside `BacklogItemForm.tsx`'s edit-mode checkbox state (`web-app/src/components/backlog/BacklogItemForm.tsx:98,377,773-774`) — a mutation form, not a passive read the plan's phrasing implies — `BacklogItemDetail.tsx` never renders it as a visible read-only badge/label (`grep -n skipReviewGate web-app/src/components/backlog/BacklogItemDetail.tsx` shows only internal state plumbing, no rendered text).
  **Recommendation**: either (a) note in Task 1.1.2a that the MCP path can't surface `SkipReviewGate` and the check must go through opening the item's edit form in the backlog UI (or asking the operator) — a one-line correction — or (b) treat this as low-risk for this specific item (the requirements.md finding already establishes the item is a straightforward bug-report duplicate, not a `SkipReviewGate`-flagged item) and note that assumption explicitly rather than leaving a check step that can't be performed as described.

- [ ] **Item 3 (prior Concern 2) — RESOLVED.** Task 1.1.1d (`plan.md:156-178`) adds a live MCP repro: `create_session(session_type=new_worktree)` from a source repo deliberately diverged from `origin/<default-branch>`, then `run_command echo alive-check` + `read_session_output`, checking both the echoed output and the resolved-base HEAD SHA. It explicitly states the rationale (unit tests passed pre-fix too, since `AppendOutput` was previously only called from tests) and gives an explicit non-silent fallback: "If live MCP tool access is NOT available... say so explicitly in Task 1.1.1e's verification record... do not silently skip this task." Task 1.1.1e (`plan.md:180-188`) makes this the evidence gate before Story 1.1.2 proceeds, with a STOP-and-escalate path if anything fails. This closes the original blind spot.

## Minors

- New (this pass): Story 1.1.2's framing ("per ADR-001's 'always via the review rail unless genuinely unclaimed' rule," `plan.md:198`) cites a document that doesn't back this claim. This repo's actual `docs/adr/ADR-001-gemini-exit-code-contract.md` is about the unrelated Gemini/agy BeforeTool hook exit-code contract; `grep -rln "review rail" docs/adr/*.md` returns nothing. Likely a hallucinated/misattributed citation introduced by the fix pass. Harmless to execution (the rule described is still accurate to the code), but should be removed or replaced with a real source (or dropped — the branch-by-branch AC5 text already justifies itself without an ADR citation).
- New (this pass): Task 1.1.2a's Branch 3 (`plan.md:259-266`) bundles two different sub-cases under one action — "linked, non-work role" (where `submit_triage_result` works, since `resolveItemLink` succeeds and role is `triage`) and "claimed by a *different* work session than this one" (where, if this session has no link to the item at all, `submit_triage_result`'s own `resolveItemLink` check — `server/mcp/tools_backlog.go:2476-2481` — would reject it exactly like `report_duplicate` would, for the same reason). The plan's own evidence (worktree naming, artifact layout) makes the first sub-case the realistic one for this session, so this doesn't block current execution, but the second sub-case as written has no actually-executable terminal action. Worth a one-line caveat rather than implying `submit_triage_result` is a universal fallback.
- Carried forward, still accurate: `report_duplicate`'s "wrong mode" risk is well-guarded by code — a non-work-role session gets a clean `PERMISSION_DENIED` and never silently falls through to `reportDuplicateUnclaimed`'s archive path (`tools_backlog.go:2205-2214`). No fix needed.
- Carried forward, still accurate: the genuine-duplicate claim is well-supported — PR #837's title matches `4d856751`'s verbatim and its body links `c7466f05-...`; PR #833 has no backlog-item link, consistent with `requirements.md`'s account.
- Carried forward, still accurate: `project_plans/worktree-base-branch-and-capture-dup-check/decisions/` remains empty — harmless, consistent with the plan's "no contestable design decision" claim.
- Carried forward, still unaddressed: Epic 1.3's two deferred follow-ups (Tasks 1.1.3a/b, `plan.md:331-348`) say "file as a new, separate backlog item if/when picked up" but still name no owner/trigger for who actually files them — they risk being silently dropped once this session ends. Worth a one-line owner note.
