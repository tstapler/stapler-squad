# Cross-artifact consistency: notification-actions-and-agent-messaging

Date: 2026-10-17. Method: `sdd/skills/4-validate/cross-artifact-consistency-prompt.md` (coverage gaps, scope drift, UX-plan misalignment, terminology drift, contradictions). Inputs read in full: `requirements.md`, `implementation/plan.md`, `design/ux.md`, `decisions/ADR-001..008`. Severity per the prompt: BLOCKER = contradiction or coverage gap; CONCERN = scope drift or terminology; NITPICK = UX alignment and minor. A few UX-alignment items are raised to CONCERN where the missing piece is a testable acceptance criterion with no owner (noted inline).

Totals: **4 BLOCKER, 13 CONCERN, 9 NITPICK**.

## Verified counts (commands run in the project directory)

| Claim | Where stated | Command | Result |
|---|---|---|---|
| 6 epics, 45 stories, 137 tasks | plan.md round-3 repair log | `grep -c "^# Epic"`, `grep -c "^### Story"`, `grep -c "^##### Task"` on plan.md | 6 / 45 / 137 (VERIFIED) |
| Per-epic tasks match the effort table | Effort table | per-epic `grep -cE "^##### Task N\."` | 22 / 14 / 25 / 29 / 39 / 8 = 137; table rows sum identically (VERIFIED) |
| 8 ADRs | plan header | `ls decisions/ADR-*.md \| wc -l` | 8 (VERIFIED) |
| Glossary term count 39 | plan "Term count: 39" | rows between `## Domain Glossary` and `Term count` starting with a backtick | 39 (VERIFIED) |
| 58 UX acceptance criteria | ux.md last line | `grep -c "^- UX-AC" design/ux.md` | 58 (VERIFIED) |
| Raw total 10,970k | Effort epic-subtotal table | summed the per-epic raw cells 1,115+925+2,130+3,060+3,385+410 | **11,025, not 10,970** (see C12) |
| CU total 10,363k; with verification 18,137k | same | summed CU cells; 10,363 x 1.75 | consistent with 11,025 raw x 0.94 = 10,363 (VERIFIED) |
| `ScheduleWakeup` per-file map | Tech Debt row and Story 4.7 | both lists compared | identical, sum 18 (VERIFIED, text only; the repo counts were not re-run) |
| Five new MCP tools | ADR-007 rule 1, Story 4.5 | compared lists | `schedule_wake`, `list_wakes`, `cancel_wake`, `request_help`, `answer_help` in both (VERIFIED) |
| Four flags and scope vocabularies | requirements, plan Risk Control, ADR-005/008 | compared | identical names and scopes (VERIFIED) |
| 7 spikes | plan Epic 0 | `grep` Story 0.1-0.7 | 7 (VERIFIED) |

---

## BLOCKER

### B1. ADR-001 decision 3 still specifies "all healthy" copy (ADR vs plan, UX)
`decisions/ADR-001` Decision 3: roll-up reads "N background sessions, all healthy. Last activity <t>." Plan Story 1.3, glossary `BackgroundRollup` ("copy never says 'healthy'"), `ActivityKind.stopped_unknown`, `design/ux.md` section 1.1 and UX-AC1 ("never 'healthy'") all reject that wording because absence of a failure record is not proof of health. The plan's repair log claims it updated `design/ux.md`; it never updated ADR-001, and ADR-001 is the authoritative decision record.
**Resolution**: amend ADR-001 Decision 3 to the `no_failures_reported` wording and add the `finished` vs `stopped_unknown` rule.

### B2. Non-Claude agent sessions cannot use any messaging or wake tool (requirements vs plan Story 4.9)
`requirements.md` Users names "other supported harnesses: Aider, Codex/OpenCode/Gemini/Antigravity/pi" as agent sessions that "ask, schedule, message", and success metric 4 says "a session can (a)...(c)". Story 4.9's spawn-path matrix mints a proof only for Claude-family tmux programs; every other harness gets `identity_unverified` from all five tools, fail closed. The plan's Vision table says wakes use "our own durable wake layer" per harness, but no harness other than Claude can call `schedule_wake`/`request_help` at all. This is a deliberate fail-closed choice that appears nowhere in requirements and is not an Unresolved Question for the owner.
**Resolution**: either record Claude-family-only as a v1 limit in requirements (and in Success Metrics, with Tyler's confirmation in Unresolved Questions), or add a task giving OpenCode/pi a proof path.

### B3. Stopped owner with `allow_revive=false`: three artifacts, two answers
Story 4.3 AC: a stopped non-archived owner is revived only when `allow_revive` AND `Admit`; "otherwise the row is `needs_revive` while within `expires_at`, then `missed`". ADR-007 rule 5: "Otherwise the trigger becomes `missed`/`skipped`". ADR-006 rule 2 and Story 4.1 define `needs_revive` as a claimed wake of a stopped owner "with `allow_revive=true` waiting on `Admit`". With `allow_revive=false` (the default) the plan's state is `needs_revive`, which ADR-006 reserves for the opt-in case, and the row would sit there until `expires_at` and be retried by re-admission that can never succeed.
**Resolution**: state in ADR-006 and Story 4.3 that `allow_revive=false` plus stopped owner goes `claimed -> skipped(reason=owner_stopped)` with an operator notification (the transition exists), and keep `needs_revive` for `allow_revive=true` only.

### B4. Operator "Remind me" is defeated by the wake guards it must bypass (UX S6 / Story 4.6c vs Story 4.3 guards)
Remind me is offered on finished or stopped `review:*`/`diagnose:*` rows and creates a `notify_operator` trigger whose `owner_session_uuid` is that hidden session. Story 4.3 guards skip wakes whose owner is archived, one-shot (`backlog:review`/`backlog:triage`) or whose item is terminal, and those are exactly the rows Remind me is offered on. The guards are not scoped by `WakeAction` anywhere (ADR-006, ADR-007 rule 5, Story 4.3), and UX 6.3 expects a "Session no longer exists" notification at fire time, which implies the guard does not apply.
**Resolution**: scope the archived/one-shot/terminal-item/revive guards to `action_kind=steer_text` in ADR-007 rule 5 and Story 4.3, and add an AC that a `notify_operator` trigger on an archived owner is delivered as an INFO notification.

---

## CONCERN

### C1. "kind" means three things; `ActivityRow` has no session-role field
ADR-001 Decision 1: row "kind (review/diagnose/triage/other from tags)". Plan glossary `ActivityKind`: `exception_failure | exception_needs_human | running | finished | stopped_unknown | departed` (a state). Observability `background_activity_rows{kind}` and requirements "Background row counts by kind" use `ActivityKind`. UX 1.3 "Hidden session with unknown kind ... chip 'other'" means role. Story 1.2's fixture says "10 review, 1 diagnose", yet no `ActivityRow` field carries review/diagnose.
**Resolution**: rename the state sum type `ActivityState` (or role `SessionRole`), add `role` to `ActivityRow`, and use `state` in the metric label.

### C2. UX still uses "Healthy" as a group label
`design/ux.md` wireframe 1.1 ("Healthy (10)", chips FINISHED/STOPPED) and section 7 ("Disclosures (`Answer`, Healthy, Waiting on)") conflict with the same file's no-failures variant ("No failures reported (11)") and Story 1.3 (distinct "Stopped, outcome unknown" chip). Plan Task 1.3a also reuses test ids `background-empty-healthy`/`background-empty-none`, so a "healthy" token persists in test ids.
**Resolution**: rename the disclosure to "No failures reported (N)", update the wireframe chip to "STOPPED, OUTCOME UNKNOWN", and name the test ids in Task 1.3a as an acknowledged legacy.

### C3. "S1..S7" names two different things
Plan spikes S1-S7 (Epic 0) and UX surfaces S1-S7 (`design/ux.md` section 0) share names. Plan Story 4.10 AC says "Answer for them opens the S3 card" (UX surface S3 = guidance card) in a document where S3 is the Channels spike; `design/ux.md` 8.1 "S1 session-list poll failure" vs plan spike S1.
**Resolution**: rename UX surfaces `UX-S1..UX-S7` (or the card "GuidanceQuestionForm") in both documents.

### C4. `terminal_detect` is a source value that no ADR or glossary defines
Story 3.6 AC: `GuidanceRequest(source=ask_user_question_hook or terminal_detect)`. ADR-003 and glossary `QuestionSource` allow exactly `agent_mcp | ask_user_question_hook | peer`. Consequences if it ships as written: no pending-cap class, no coalescing key, not covered by the `GuidanceSessionReaper` scope (hook and peer only), and `NewGuidanceRequest` has no rule for it.
**Resolution**: either drop `terminal_detect` (non-Claude harness rows use `ask_user_question_hook` with a profile name field) or add it to ADR-003, the glossary, the cap-class list and the reaper scope.

### C5. `question_id` vs `guidance_request_id` vs `QuestionID`
Glossary: `QuestionID` is a newtype over the `GuidanceRequest` id, but `TargetKey` lists `question_id` and `guidance_request_id` as separate keys, Story 3.3 says notification metadata carries both, and Story 2.2's example derives `question_id=q1` from a `PendingQuestionStore` entry (a different id space that exists before and without a mirrored row). ADR-002 Decision 1 lists keys `question_id`, `approval_id`, `item_id` and omits `guidance_request_id`/`ANSWER_GUIDANCE`.
**Resolution**: state that `question_id` is the `PendingQuestionStore` key for unmirrored prompts and equals `guidance_request_id` once mirrored, or drop one key; fix ADR-002's key list.

### C6. Sampler give-up marks a still-open dialog `stale` with copy that says it was handled
Story 3.3: after 10 minutes with the dialog still matching, sampling stops and the row is `stale`. UX delivery-state table: `stale` copy is "Already handled in the session. No action needed." A human who is away for 10+ minutes (the normal case for an unattended session) then sees a false "handled" message and, per Story 3.3, `STALE` on answer attempt, so the question that is still on screen can no longer be answered from the pane. This conflicts with the requirement's value (answer from the pane) and the Remote Control finding that questions stay open indefinitely.
**Resolution**: keep the row open and unsampled after 10 minutes (re-verify via `DialogMatch` at answer time, which Story 3.3 already requires) and reserve `stale` for a confirmed mismatch, or give the 10-minute state its own `DeliveryState` and UX copy.

### C7. Peer-help UX (UX S4) has no plan owner for most states
UX-AC31 (informational row with asker, answerer, question, age, state; no badge before escalation), UX-AC33 ("Not sent: {reason}" row), UX-AC34 ("Answered by {session}" row) have no story or task. The plan lists peer requests only inside "Waiting on" (Story 4.6) and its self-review says peers appear "only in Waiting on and Background rows (no notification)", while UX places them in the Background feed and Recent activity. Rejected requests (self-target, archived, one-shot, cross-workspace) are rejected at tool-call time and create no row, so UX 4.3 "Not sent" has no data to render. The 10-minute stall in Story 4.10's AC ("no reply in 10 min ... escalates ... appears in Needs you") has no task that detects it or emits the Needs-you notification (Task 4.3e covers wake failures only; ADR-003 gives peer rows a 24 h `expires_at`, not 10 min).
**Resolution**: add a task for the peer escalation timer and notification emitter (and its gate-class table entry), decide whether rejected requests persist a row, and add ACs for UX-AC31/33/34.

### C8. Delivery-state copy differs between Story 3.7 and the UX source
Story 3.7's first AC says copy "matches `research/ux.md` s5" and quotes "Saved; will be delivered when the session resumes" and "Session ended; recorded only"; its own example below asserts "Your answer is saved and will be delivered if the session resumes". `design/ux.md` section 3.1 and UX-AC23 use "...if the session resumes." and "Session ended before this was answered." The plan's Story 3.4 archived copy is a fragment ("will not be delivered unless you resume") of the UX sentence.
**Resolution**: make `design/ux.md` section 3.1 the single copy source and have Story 3.7 and 3.4 cite it by UX-AC23.

### C9. `status_transition` is used as a valid trigger kind and as the rejected example
Story 4.3 coalescing AC fires on "10 `status_transition` events for item X". Story 4.4 AC and ADR-006 rule 4 use bare `status_transition` as the kind `schedule_wake` rejects with `no_reconcile_predicate`. The full `(trigger_type, kind)` table with predicates is never enumerated (Task 4.4d builds it), and ADR-006 also calls `verdict_recorded` "as an edge" rejectable while Story 4.4 accepts it with a predicate.
**Resolution**: enumerate the supported `(type, kind, predicate)` rows in Story 4.4 and make the Story 4.3 coalescing example use one of them.

### C10. Requirement (c) of success metric 4 is conditional in the plan
Requirements: schedule by "backlog/PR/CI event". Story 4.4 and Spike S4 allow `pr_event`/`ci_event` to ship as a typed `unavailable` error. The plan acknowledges the fallback; requirements and Success Metrics do not, so metric 4 can be reported unmet by design.
**Resolution**: add "PR/CI triggers depend on spike S4" to requirements Feasibility Risks, or state the measurable minimum (backlog event + `at`/`cron` + session_state + human answer).

### C11. Tier classification is missing for `ANSWER_GUIDANCE` and the cancel kinds
ADR-004 enumerates Tier A as `answer_question` (durable route), `backlog_decision`, `approve`/`deny`, `open`, and Tier B as keystroke answers and steers. `ANSWER_GUIDANCE`, `CANCEL_WAKE`, `CANCEL_PEER_HELP` are in the glossary and ADR-002 4a but only the cancels are called Tier A (ADR-002 4a); `ANSWER_GUIDANCE` has no tier anywhere, and Story 2.8 tests only `ANSWER_QUESTION` and `BACKLOG_DECISION` on the remote chain.
**Resolution**: add `ANSWER_GUIDANCE` (Tier A) and both cancels to ADR-004's tier list and a remote-chain AC in Story 2.8.

### C12. Effort arithmetic: raw total and the Epic 4 checkpoint
(1) Epic subtotal table lists raw total 10,970k; the six epic cells sum to 11,025k (and 11,025 x 0.94 = 10,363, which is the CU total shown). The round-3 repair log repeats "10,915k -> 10,970k" while the same section's size band says round 3 added 110k (0.7c S + 4.9f S = 110k). (2) The overrun checkpoint uses Epic 4 "3,130k CU" and 6,847k; the table's Epic 4 CU is 3,182k (3,182 x 1.75 x 1.25 = 6,961k). 3,130k predates Task 4.9f.
**Resolution**: set the raw total to 11,025k in the table and round-3 log, and the Epic 4 checkpoint to 6,961k.

### C13. UX acceptance criteria with no plan AC or task (raised from NITPICK because each is independently testable)
No plan text exists for: inline Deny-reason and Merge confirm in the row (UX-AC15; plan only covers the "Approve anyway" confirm), `RATE_LIMITED` countdown and re-enable (UX-AC17), `INVALID` -> Refresh copy, field validation with `aria-invalid` and focus to first invalid group (UX-AC22), "View full prompt in session" for unrenderable shapes (UX-AC28), Cancel-wake inline confirm and "Already fired at {time}" (UX-AC36, 37), the >100-row and >20-wait-row caps ("Show all (N)"), Remind-me 10 s Undo (UX-AC40), "Handled automatically by rule {name}" STALE variant. grep of `plan.md` for each phrase returned 0 hits (RATE_LIMITED appears once, only as an enum).
**Resolution**: add one AC per item to Stories 2.5/2.6/3.7/4.6/4.10, or mark them "UX-only, verified by Story 5.3 checklist".

---

## NITPICK

- **N1. Requirements wording lags the plan.** Success Metrics say "a meaningful state when all is healthy"; Observability says wakes "scheduled/fired/missed" while plan/ADR-006 retire `fired` for `claimed`/`delivered`. Update the two phrases.
- **N2. UX cites "plan Unresolved Question 5" for Remind me.** It is the 6th bullet (the 5th is the MCP role allowlist). Fix the number in `design/ux.md` section 6.
- **N3. "7 interactive surfaces (S1-S6 plus S7 condensed)".** `design/ux.md` footer and the surface table type S7 as "Condensed", so there are 6 interactive surfaces plus 1 cross-cutting entry.
- **N4. ADR-005 vocabulary.** Consequences say mismatch yields `STALE_PROMPT`; the plan and ADR-002 outcome is `STALE` (confirm `STALE_PROMPT` is the legacy hidden-reply enum and say so). ADR-005 rung 4 says archived sessions get "no write", while Story 3.4 stores the answer (`answered_at` set, `saved_for_later`); add "no pane write".
- **N5. Wave and dependency notes.** Dependency Visualization says Epic 4 "needs S3,S4 for 4.4a"; S3 (Channels) informs Task 4.3c, not 4.4a. Task 4.10d reuses the card from Story 3.7 but both are in W6; Task 4.9f says it reuses the Task 3.3c seam but 4.9 is in W1 and 3.3 in W3 (its "or the existing session-delete cleanup" alternative should be the default).
- **N6. Requirements In Scope items 2 and 3** (current-state inventory, competitive survey) and the vision doc in item 4 are delivered by `research/*.md` and the plan's Vision section, with no story; add a line to the plan mapping them so the coverage check is explicit.
- **N7. ADR-004 uses lower-case action names** (`answer_question`, `backlog_decision`, `approve`) where the plan and proto use `ActionKind` enum names (`ANSWER_QUESTION`, ...).
- **N8. Tab names.** Story 1.3 AC says focus on the "Notifications" tab; `design/ux.md` and `research/ux.md` use "All / Needs you / Background". The current component (`NotificationPanel.tsx:785`) renders "Notifications" and "Background". Align UX wording or note that "Needs you" is a tier, not a tab.
- **N9. ADR-001 consequence "departed handling already exists per tab"** vs plan Task 1.2c, which builds `departed` as new selector behavior. Reword the ADR.

---

## Coverage tables

### Requirements -> stories (In Scope and success metrics)
| Requirement | Stories | Status |
|---|---|---|
| Scope 1 Background diagnose/fix, feed + pinned, empty state | Epic 1 (1.1-1.6) | covered (B1, C1, C2) |
| Scope 2 inventory, Scope 3 survey, Scope 4 vision + ADRs | research/*.md, plan Vision, ADR-001..008 | covered by artifacts, no story (N6) |
| Scope 5 phased flag-gated implementation | Epics 0-5 | covered |
| Metric 1 Background 0 -> 11 rows | 1.2, 1.4 | covered |
| Metric 2 one-interaction approve/question/decision | 2.2, 2.3, 2.5, 2.9 | covered (C13 for confirm steps) |
| Metric 3 structured question, answered latency | 3.1-3.7 | covered (C6) |
| Metric 4 (a) human help, (b) peer help, (c) triggers incl. restart survival, zero new poll loops | 4.1-4.9, 4.7b | covered with gaps (B2, B3, C10) |
| Observability list and the two alerts | Observability Plan, 5.1 | covered |
| Constraints (isolated instance, reuse guidance/gate, live flags, mobile+desktop, noarchivedrevival) | 5.3, 3.1, 0.8, 2.6, 4.3f | covered |
| Performance SLO: pane renders <= 200 ms from cached state | UX flow 1.2 only | **no plan AC or test** (p95 < 1 s action SLO is covered in 2.3); add a render-time assertion to Story 1.3 or 2.5 |

### Stories -> requirements (scope drift check)
No story lacks a requirement. Stories that serve an unrequested capability but are justified by requirements wording: 3.6c harness-native channels (requirements Rabbit Holes), 4.6c/4.10b Remind me (not in requirements In Scope; added from UX research and kept "optional-to-order"; see plan Unresolved Questions), 4.10c Resume wakes (UX gap, not in requirements), 4.9 credential (requirements Security: "caller-identity checks"). All four are acknowledged in the plan; flagged only because Remind me and Resume wakes have no requirements line (recommend adding them to Scope for traceability).

### UX surfaces -> plan stories
S1 -> 1.2, 1.3, 1.6; S2 -> 2.2, 2.3, 2.5-2.8; S3 -> 3.4, 3.5, 3.7; S4 -> 4.5, 4.10d only (C7); S5 -> 4.6, 4.10a/c; S6 -> 4.6c, 4.10b; S7 -> 1.6b, 2.6b, 0.8. Keyboard/ARIA section 7 -> 1.3b, 2.6a. UX gaps 8.1/8.2 are owned (plan "UX gaps" table verified against story numbers).

## Repair status (Phase 4 repair, 2026-10-17)

Docs-only repair. The counts in the table above were true when written (6 / 45 / 137, 39 glossary terms, 58 UX-ACs); after the repair they are **6 epics, 47 stories, 154 tasks, 8 ADRs, 44 glossary terms, 59 UX acceptance criteria** (recounted with grep). The raw effort total in the plan is now 12,265k (the 10,970k figure criticised in C12 was wrong; the cells then summed to 11,025k). Full mapping: `implementation/plan.md` "Review repair log, Phase 4".

| Item | Status | Where |
|---|---|---|
| B1 ADR-001 "all healthy" | Resolved | ADR-001 decisions 3 and 6 |
| B2 non-Claude harness messaging | Resolved by design change: proof minting extends to every harness whose per-session MCP config ssq controls (Spike S5, new Task 0.5e; Task 4.9g), fail-closed otherwise; requirements reworded | Stories 0.5, 4.9; ADR-007 rule 0; `requirements.md` |
| B3 `needs_revive` vs `skipped` | Resolved: single decision table in ADR-006 rule 2; `allow_revive=false` is `skipped(owner_stopped)` | ADR-006, ADR-007 rule 5, Story 4.3 |
| B4 Remind me defeated by guards | Resolved, plus an extra finding: the hidden-session gate would also have suppressed the INFO reminder, so it is a system-level notification with no session binding | ADR-006 rule 2, ADR-007 rule 5, Stories 4.3, 4.6 |
| C1 "kind" | Resolved | Glossary note, `ActivityState`, `SessionRole` |
| C2 UX "Healthy" label | Resolved | `design/ux.md` |
| C3 S1-S7 clash | Resolved: UX surfaces are `UX-S1` to `UX-S7` | `design/ux.md`, plan |
| C4 `terminal_detect` | Resolved: dropped | ADR-003, Story 3.6 |
| C5 `question_id` vs `guidance_request_id` | Resolved | Glossary, ADR-002 |
| C6 sampler `stale` | Resolved (pre-mortem P1-b) | Story 3.3, UX-AC59 |
| C7 peer-help UX owner and escalation emitter | Resolved: Story 4.11; rejected requests create no row (decision) | Story 4.11 |
| C8 delivery-state copy | Resolved: `design/ux.md` 3.1 is the single source | Stories 3.4, 3.7 |
| C9 `status_transition` | Resolved: `(type, kind, predicate)` table in Story 4.4 | Story 4.4, ADR-006 rule 4 |
| C10 PR/CI conditional | Resolved | `requirements.md` |
| C11 tiers | Resolved | ADR-004, Story 2.8 |
| C12 effort arithmetic | Resolved and recomputed | Plan Effort estimate |
| C13 UX-ACs without a plan AC | Resolved: ACs added and a 59-row ownership trace | Plan "UX acceptance criteria trace" |
| N1, N2, N3, N4, N5, N7, N8, N9 | Resolved in the same edits (wording, bullet reference, surface count, `STALE` vs `STALE_PROMPT`, 4.9f seam default, enum names in ADR-004, tab names, "departed" reworded) | `requirements.md`, `design/ux.md`, ADR-001/004/005, plan |
| N6 requirements scope items 2-4 with no story | Not changed: delivered by `research/*.md`, the plan Vision and ADR-001..008 | - |
| Coverage gap: 200 ms pane SLO | Resolved: Story 2.5 (Task 2.5d), Story 1.3, `notification_pane_open_ms`; p95 < 1 s measured in Story 2.9 (Task 2.9c) | Plan |

New totals after the repair: **0 open BLOCKERs, 0 open CONCERNs; N6 left as is.** Not re-reviewed by an independent pass (the pre-mortem and consistency reviewers did not re-run); the self-review below is the only check.
