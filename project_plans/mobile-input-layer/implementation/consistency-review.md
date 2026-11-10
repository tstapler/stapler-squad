# Cross-artifact consistency review: mobile-input-layer

Scope: requirements.md, implementation/plan.md, design/ux.md, decisions/ADR-001..008 (research/ux.md consulted for UX-2). Method: `sdd/skills/4-validate/cross-artifact-consistency-prompt.md` (coverage gaps, scope drift, UX-plan misalignment, terminology drift, contradictions). plan.md was not edited.

Summary: 3 BLOCKER, 13 CONCERN, 9 NITPICK.

## Verified clean (no finding)

- Task and story counts. `grep` of plan.md gives 37 stories and 87 tasks (XS 13, S 62, M 12). The Effort table has 37 rows summing to 87 tasks. Raw 5230k = 13x20 + 62x55 + 12x130; CU 7740k = 5230 x 1.48; verified 13,546k is 7740 x 1.75 (13,545 before rounding).
- Delivery Slicing PR-A..PR-E against the Dependency Visualization. Contents and `Depends on` agree, including the chrome hook shell (Task 1.4.1e in PR-A, extended by B/C/D) and `SendResult` plumbing (Tasks 1.2.4a/b in PR-A).
- Sub-flag names. `mobile_input_queue`, `mobile_input_diagnostics`, `mobile_input_compose` and `mobile_input_edit_context` are spelled identically in plan Delivery Slicing, Risk Control, Phases 3/4/6, ADR-003 and ADR-004. No file introduces a second spelling.
- Synthetic fixture count: 14 scenarios + 5 Story 1.2.4 fixtures = 19 (+1 `disconnect-autocorrect-reconnect` in PR-B). ADR-004's six replay cases = 5 (PR-A) + 1 (PR-B).
- Glossary terms. `DeliveryEpoch`, `SendResult` (`sent|queued|dropped`), invalidate reasons (`session|generation|pool_reuse|visibility|queue_discard|transport`) and `MobileInputController` members (`focus, blur, dispose, active, bindTapTarget, invalidate`) are used identically in the Glossary, Stories 1.1.2/1.2.2/1.2.3/1.2.4/3.1.3/3.1.4, ADR-002, ADR-004 and ADR-006.
- PR-A "no visible drop notice" is stated consistently in plan header, Story 1.2.4, ADR-004 and ux.md section 0 line 13. ux.md S3 is tagged Phase 3.
- ux.md S2 "no preview chrome" agrees with ADR-002 and the plan's Pattern Decisions and UX table. See NITPICK N1 for the stale research/ux.md UX-2 text.

## BLOCKER

### B1. Flag default: requirements.md says "on once verified"; plan and ADR-003 merge ON before device verification
- Artifacts: requirements.md Risk Control (line 110: "default on for touch devices once verified, off elsewhere initially") vs plan.md header "Flag default" paragraph, Delivery Slicing and Story 1.3.1 (`defaultValue: true` at merge; Story 5.1.4 is a post-redeploy check), and ADR-003 (amended 2026-11-08).
- The two are opposite orderings (verify, then default on, vs default on, then verify). Requirements.md was never amended.
- Resolution: amend requirements.md Risk Control to the ADR-003 policy (server default ON at merge, effective only on the Android class, rollback by flipping the flag, two page loads to apply), or reverse the plan. Keep one wording across all three.

### B2. Scalability NFR: "layer attaches to the focused terminal only" contradicts attach-on-visible/docked
- Artifacts: requirements.md NFR line 57 vs plan Story 1.4.1 (attach when visible/docked, not on focus; several layers can exist, focus only moves between them) and ADR-006.
- The plan deliberately changed this so `bindTapTarget` exists before the first tap. It means N hidden textareas and listener sets for N docked terminals, which the NFR forbids.
- Resolution: rewrite the requirement to "at most one layer owns focus/inputmode; unfocused docked layers hold only a hidden element and a tap binding; hidden terminals dispose." Optionally add a test or note bounding per-terminal cost for many-terminal pages.

### B3. ux.md S9 requires `reportToTrace` from the diagnostics report; plan, ADR-007 and ADR-008 say the report cannot become a trace
- Artifacts: design/ux.md S9 Acceptance ("`reportToTrace` produces a fixture the corpus accepts except `expectedBytes`") vs plan Story 4.1.1 (report is content-free, "cannot be converted into a replayable trace", only `reportToSkeleton`), Story 4.1.3 (`captureToTrace` from the separate capture artifact), ADR-007, ADR-008. adversarial-review.md items 1 and 3 already resolved this in the plan, but ux.md was not updated and the "Plan alignment" table in ux.md does not list it.
- Resolution: change S9 to "report contains no typed text; `reportToSkeleton` yields a shape summary only; replayable fixtures come from capture-for-fixture (`captureToTrace`)". Add a row to the ux.md alignment table.

## CONCERN

### C1. Scope drift: Phases 3-4 deliverables are not in requirements.md In Scope
- Plan Phase 3 (atomic-unit queue), Phase 4.1 (diagnostics sheet, capture-for-fixture), 4.2 (compose bar, IME-fallback detector), Story 1.2.4 (delivery ledger) and Story 1.1.3 (paste/bracketed paste) have no matching In Scope bullet. In Scope only lists the counter and visible indicator, an "optional client beacon", and the test corpus. The queue, compose bar and capture mode trace to owner decisions in ADR-004/008 and research, not to requirements.md.
- Resolution: add the owner-approved items to In Scope in requirements.md, or mark them as ADR-sourced scope in the plan.

### C2. "Visible indicator for dropped input" is unmet by default and not delivered in PR-A
- Requirements Success Metrics (line 36) and In Scope require a visible indicator. The plan delivers the chip in PR-B behind sub-flag `mobile_input_queue`, default off "until owner approves ADR-004". PR-A retains `console.warn` and shows nothing new. ux.md section 0 only discusses `mobile_input_layer`, so ux.md does not say the chip is dark by default (ux.md "Plan alignment" omits `mobile_input_queue`).
- Resolution: state in requirements.md that the indicator ships in PR-B and is off until the owner enables the sub-flag, or default `mobile_input_queue` on at PR-B merge. Add the sub-flag to ux.md section 0 and S3.

### C3. Renderer gate scope: requirements say "touch devices", plan gates the Android class only
- requirements.md In Scope and Risk Control ("on touch devices it falls back to xterm.js unless the input layer is active") vs plan `resolveRenderer` (`{true,false,false}` -> ghostty; iOS and touch laptops keep ghostty with no layer). Out of Scope's "iOS parity beyond not regressing" partly covers this, but an iPhone with `ghostty_renderer` on keeps the same dead-soft-keyboard risk the gate exists to prevent.
- Resolution: reword requirements.md to "Android touch-primary class" and add an iOS-under-ghostty line to Out of Scope or the follow-up issues (Story 6.1.2 already files an iOS parity item).

### C4. Performance NFR and batching have no covering task or test
- requirements.md NFR ("no added keystroke-to-send latency beyond one animation frame", "batching with existing `terminal-input-batching` behavior preserved", desktop zero added work) has no story, AC or benchmark in plan.md. Story 1.2.2 only asserts "zero listeners on desktop". Atomic edits that bypass `PASTE_CHUNK_SIZE` (Story 3.1.1b, PR-B) touch batching.
- Resolution: add an AC (e.g. a test that the reducer-to-`onData` path is synchronous per event and that atomic units interleave correctly with existing batching), or record it as covered by the Pixel 7 e2e and owner acceptance.

### C5. Alert condition in requirements vs "no paging alert" in plan
- requirements.md Observability lists an "Alert condition"; plan Observability Plan says "no paging alert" and gives a documented diagnostic query only.
- Resolution: change requirements to "documented diagnostic condition", or add an alert rule task.

### C6. Requirements say "entry in the existing actuator view"; plan adds a new endpoint
- Plan Story 2.1.3 adds `GET /actuator/terminal-input`. Resolution: edit the requirement text to "a new actuator endpoint beside `/actuator/ws-unary`" (ADR-005 already says so).

### C7. `OnData` signature drift: `seq` missing in Task 1.1.1a and ADR-001
- Glossary `MobileInputOptions` and Story 1.2.2 define `onData: (bytes: TerminalBytes, seq: UnitSeq) => SendResult | void`. Task 1.1.1a defines `OnData = (bytes: TerminalBytes) => SendResult | void` "defined once", Task 1.2.4b says to use "the `OnData` type from Task 1.1.1a", and ADR-001 shows `onData: (bytes) => SendResult | void`. A worker following 1.1.1a will omit `seq` and break the ledger.
- Resolution: change Task 1.1.1a's `OnData` to include `seq: UnitSeq` (and `UnitSeq` in `types.ts`); update ADR-001's signature text.

### C8. Story 1.1.2 AC depends on machinery created in Story 1.2.4 (wave ordering)
- Story 1.1.2 (W3) has an AC about `dropped` results and `invalidate('transport')`; Task 1.2.4c (W6) modifies `composition.ts` and creates the ledger. Story 1.1.4 (W4) asserts a count of "19 synthetic" fixtures, five of which are authored in Task 1.2.4d (W6). Also Task 1.2.4e's sessionId-change jsdom test needs 1.4.1a (W7) while the critical-path text puts Story 1.2.4 in W6.
- Resolution: mark the dropped/invalidate AC of 1.1.2 and the "19" figure in 1.1.4 as "completed by Story 1.2.4", or move 1.2.4c's reducer part into 1.1.2.

### C9. `InputPath` enum has five values but Story 2.1.2 lists four instrumented sites
- Glossary `InputPath = cm | hub | shell | capture | stream`; Story 2.1.2 and ADR-005 list the shared read loop (cm), capture-pane, shell, and `session_service_stream_terminal.go`, then say "each of the five sites". `hub` maps to no listed site. Either a site is missing (silent blind transport) or the enum has a dead value.
- Resolution: add the `hub` site to the Story 2.1.2 table or remove it from the enum; correct "five sites" to the real count.

### C10. Compose bar offline behavior: ux.md S5 vs plan 4.2.1 vs ADR-004 plain-text rule
- ux.md S5: "Disconnected on Send: text is queued through the same path". Plan 4.2.1: queued only if plain text with no newline; multi-line refused. ADR-004: any `\x1b` or `\r` makes a unit unqueueable, so a bracketed-paste-wrapped compose unit (mode 2004 on) and Send+Enter can never be queued. The plan does not say whether wrapping happens before or after the queue, and the chip copy for the multi-line refusal is not in ux.md. The limit is "4096 chars" in the plan/ux but the queue capacity is 4096 bytes.
- Resolution: specify that compose queues the unwrapped plain text and applies wrapping at flush (or refuses offline Send in 2004 mode), add the refusal state to ux.md S5, and unify chars vs bytes.

### C11. ux.md S6 vs plan 4.2.2: auto-open vs offer
- ux.md S6: the notice appears and the compose bar opens focused; "Got it" leaves it open; "Turn off" closes it; notice auto-dismisses after 8 s. Plan 4.2.2: "Offered once with a notice", opt-out persistence only; no auto-open, no 8 s timer, no "Turn off closes compose" AC. ux.md S1 also has a trigger ("2nd tap with no focus event") with no detector in the plan.
- Resolution: add the auto-open, 8 s dismiss, and "keyboard fails to open" trigger to Story 4.2.2 or remove them from ux.md.

### C12. ux.md internal contradiction on flag-off and diagnostics; S8 apply-timing vs two page loads
- ux.md section 0: with the flag off "none of the new surfaces render". S7 and S8: the diagnostics sheet still opens with header "layer: off" / "renderer: xterm (fallback: mobile_input_layer off)" (also plan Story 4.1.2 AC). S8: flag toggle "takes effect on next terminal mount"; plan Risk Control/ADR-003: takes two page loads (ADR-003 itself says both "next page load or terminal mount" and "two page loads").
- Resolution: carve the diagnostics sheet (own sub-flag) out of the section 0 "none render" rule; replace the S8 timing text and the ADR-003 "or terminal mount" phrase with the two-load statement.

### C13. ux.md S1 hardware-keyboard dedupe "in a short window" contradicts ADR-002
- ux.md S1 flow step 4 says the `beforeinput` path "dedupes in a short window (R 4.6)". ADR-002 rejects timing-window dedupe (Codeman 50 ms) as non-deterministic; plan Story 1.2.2 handles double-send structurally.
- Resolution: reword ux.md S1 step 4 to the structural rule (physical `keydown` with `keyCode != 229` is handled and `beforeinput` for it is suppressed), no timing window.

## NITPICK

- N1. research/ux.md UX-2 ("Default text path is preview-then-commit") is superseded by ADR-002 and design/ux.md S2 but carries no supersession note. Separately, "UX-2" now names two different items (research/ux.md vs design/ux.md "Autocomplete pick...").
- N2. Chip copy differs between ux.md and plan. ux.md: "Keys are not sent while disconnected" vs plan AC substring "keys not sent"; ux.md "Reconnected - sent 12 typed characters." vs plan "Sent N chars"; ux.md has per-cause discard copy ("offline for more than 30 s", "the session changed") vs plan/ADR-004 single "Discarded N chars typed while offline". Pick one string set.
- N3. ux.md S3 has no "Enter not sent" state in its table or UX-9, though plan 3.1.3 and the ux.md alignment table mention it.
- N4. ux.md section 0 says one polite live region is shared; S3 says discard notices use `role="alert"` (assertive). Plan 3.1.5 says only "announced once".
- N5. ux.md section 0 refers to "existing ADR-003 dvh/visualViewport" (the mobile-ux-improvements ADR) while this project's ADR-003 is the flag policy. Name the source as "mobile-ux-improvements ADR-003".
- N6. Metric mapping covers 2 of the requirements names. ADR-004 also names `input_queue_discarded`, and the plan's `cm.input` events `queued_flushed`/`queue_discarded` are unmapped. Read-only/control/enter drops have no `cm.input` event (they would be counted as `dropped_disconnected` or not at all). Add them to the Story 2.2.3 mapping table.
- N7. Plan puts Story 5.1.4 in PR-A, yet Task 5.1.4a (device fixtures) can only run after PR-C. Mark 5.1.4a as post-PR-C in Delivery Slicing. Similarly Story 6.1.2 (file issues) is listed under PR-E though it is not a PR and not conditional.
- N8. ux.md S1/S2 edge tables mention the S3 chip as if present; the chip is PR-B behind `mobile_input_queue`. Add "(PR-B)" like section 0 line 13 does.
- N9. Count unit mismatch: Story 3.1.3 "Discarded 4106 chars" uses bytes for a chars message; plan Glossary `InputDropInfo.chars`. Define `chars` as UTF-16/grapheme count or the bytes figure consistently.

## Highest-severity finding

B1: requirements.md still says `mobile_input_layer` defaults on only "once verified", while plan, ADR-003 and Story 1.3.1 merge it ON before the owner device check.
