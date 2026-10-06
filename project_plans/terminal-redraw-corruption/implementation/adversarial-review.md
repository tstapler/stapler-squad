# Adversarial Review: terminal-redraw-corruption

**Date**: 2026-08-06
**Verdict**: CLEAN (no blockers, no concerns; 1 minor)
**Round**: 9 (re-review after Round 8 fixes — byte-order gate added to Task 1.3.1c/Story 2.1.1, known-gap note + candidate regex added to Task 2.1.1a, Task 1.1.1a wording corrected, status header updated)

## Round 8 Fixes — Verification

All five claimed edits were checked against the literal current text of plan.md, not taken on the summary's word.

### 1. Task 1.3.1c gains an independent byte-order bullet — FIXED
plan.md:374-397 adds a new bullet, separate from the existing "which repositioning idiom"
bullet (plan.md:361-373), titled "Second, independent hard-gate dimension." It explicitly
asks: "does the erase sequence come before or after the repositioning sequence in the
captured bytes?" and states "Do not infer the order from which idiom is present — record
it as its own explicit observation." This is exactly the blind spot Round 8 identified —
the old wording ("...idiom the live redraw actually uses...relative to the erase
sequence") let an implementer describe the idiom without ever checking which side of the
erase it falls on. The new bullet carries the same Phase-5-blocking consequence as the
idiom-recognition gate ("this blocks Phase 5 sign-off on Story 2.1.1..."). Genuinely fixed.

### 2. Story 2.1.1 Acceptance Criteria gains an analogous gating bullet — FIXED
plan.md:451-459 adds a bullet parallel to the existing live-repro-gating bullet
(plan.md:444-450), stating the classifier "hard-codes reposition-then-erase byte order,"
naming `research/pitfalls.md` §6 item 2 as the source, and gating sign-off on Task 1.3.1c's
byte-order observation — "if it observes erase-then-reposition, the classifier must gain a
second alternation branch...before this story can be signed off for Phase 5." Consistent
with bullet 1's wording, correctly cross-referenced. Genuinely fixed.

### 3. Task 2.1.1a gains a "known, research-named gap" note with a candidate regex — FIXED, and independently checked for correctness
plan.md:520-541 adds the note, explicitly labeled "Round 8 adversarial review, BLOCKER,"
explaining the reposition-then-erase hard-coding, and proposing the second alternation
branch: `(?:\x1b\[[0-2]?K|\x1b\[[0-3]?J)(?:\x1b\[\d+A|\r|\x1b\[\d+;\d+H|\x1b\[H)`.

Checked this fragment against the actual `ansi-escapes` idiom research names (pitfalls.md
§6 item 2): `ansi-escapes`'s `eraseLines(count)` emits `eraseLine (\x1b[2K)` followed by
`cursorUp(1) (\x1b[1A)`, repeated per line, i.e. exactly erase-then-cursor-up at the start
of the sequence — this fragment's shape (an erase form immediately followed by one of the
three repositioning forms) matches that idiom correctly. The character-class ranges
(`[0-2]?K`, `[0-3]?J`) are copy-consistent with the existing branch and with the EL/ED
parameter ranges used everywhere else in the plan (EL has no `3`, ED does).

Checked the narrowness argument the note leans on: the new branch still requires one of
the three named repositioning forms to immediately follow the erase — it is not "any bare
erase," so it doesn't reopen the Round 7 hole (an ordinary non-redraw chunk that merely
starts with an erase byte still fails to match unless it also happens to be immediately
followed by cursor-up/CR/absolute-CUP, the same "three concrete idioms" argument Round 7
accepted for the existing branch). This holds up under the same skepticism Round 7's
reviewer applied to the optional-prefix regression. Genuinely fixed, and independently
verified correct — not just present.

### 4. Task 1.1.1a wording corrected — FIXED
plan.md:132-136 now reads "this window is the cursor-repositioning prefix — cursor-up,
CR, or absolute CUP, per Round 6/7's widening, not 'cursor-up sequence' alone as an
earlier draft of this task said — plus the immediately-following erase sequence," with an
inline "(Round 8 review finding (Minor, fixed here))" marker. Matches the Round 8 minor's
required rewording exactly. Genuinely fixed.

### 5. Status header updated — FIXED
plan.md:6 accurately summarizes the Round 8 blocker/minor fixes and states "round 9
re-review pending." Consistent with the actual diff.

## Blockers

None.

## Concerns

None.

## Minors

### Task 1.1.1a's footprint-scan-window wording will go stale again if the contingent Task 2.1.1a second branch is ever actually added, and nothing currently flags that follow-up
This is a new observation, not a re-check of a prior finding. Task 2.1.1a's Round 8 fix
(plan.md:520-541) documents a *candidate* second alternation branch — erase-then-reposition
— but only wires it in contingently, gated on Task 1.3.1c's live-repro byte-order finding.
If that gate ever fires and a Phase 5 implementer adds the branch, `isFullRedraw`'s leading
structural window would then legitimately have two possible internal orderings (prefix
then erase, or erase then prefix), but Task 1.1.1a's just-corrected wording
(plan.md:132-136) still describes the window as "the cursor-repositioning prefix...plus the
immediately-following erase sequence" — a description that is only accurate for the
current, order-1-only regex. No task or acceptance criterion in the Task 2.1.1a "known gap"
note, nor in Story 2.1.1's new gating bullet, instructs a future implementer to also revisit
Task 1.1.1a's window description if the second branch lands — the exact same class of
staleness Round 8 itself caught and fixed (a description of `isFullRedraw`'s shape drifting
out of sync with the regex after a widening) is set up to recur a second time, silently,
the moment this contingent branch is exercised.

In practice this is likely harmless in effect (per Task 1.1.1a's own text,
`summarizeEraseFootprint` is a presence test for erase labels within a bounded leading
window, not an order-sensitive match, so it would probably still correctly detect the
erase regardless of which side of the prefix it falls on) — the risk is purely that the
*prose* would misdescribe the window's shape for a future reader, the same low-severity
"could mislead a Phase 5 implementer" framing Round 8 used for the original instance of
this issue. Low severity, no fix required now since the branch is not yet added: worth a
one-line addition to Task 2.1.1a's "known gap" note (e.g. "if this branch is added, also
update Task 1.1.1a's window description to cover both orderings") so the contingency is
pre-empted rather than rediscovered in a Round 10.
