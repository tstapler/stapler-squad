# UX research: backlog-make-it-true (workstreams A-status, B, C, D-legibility)

Date: 2026-10-17. Paths are relative to `web-app/src/`. Labels: VERIFIED = opened the file/ran grep this session; UNVERIFIED = recalled knowledge or not opened. External comparables are UNVERIFIED (no web lookup done).

## 1. Jobs-to-be-done

| # | When the operator... | They want to... | Surface |
|---|---|---|---|
| J1 | enables autonomy and walks away | know in one glance which automation is actually acting (on/off, and *why* a flag that is "On" does nothing) | Features page |
| J2 | tunes quota | see the effective gate state, the budget in use, and what 0 means before saving | new Quota/Budget section |
| J3 | sees a PR stalled | know whether repo auto-merge could have worked, and if not, why, without opening GitHub | StuckItemDetail |
| J4 | edits stages | know whether the edit changes real transitions (enforced), is only being compared (shadow), or is inert (off) | Backlog Stages page |
| J5 | rolls back at 2am | one live toggle to revert, no deploy, no env var | all of the above |

## 2. Existing patterns to reuse (all VERIFIED unless noted)

**Features page** (`app/settings/features/page.tsx`)
- Flag row = name + On/Off badge + description + `statusDetail` line + toggle (`page.tsx:151-199`). `statusDetail` is a free-text string from the server (`lib/contexts/FeatureFlagsContext.tsx:15`, `:40`). It is the cheapest place to render "On, but inert: requires diagnose_nudge_enabled" without a proto change, but it is untyped text, so the UI cannot style severity.
- Per-flag extras are bolted on by `name ===` checks (`page.tsx:165-183`). A third and fourth special case would be a smell; a small `flagExtras` registry keyed by flag name is cheaper (UNVERIFIED design suggestion).
- `FEATURE_META` supplies labels (`page.tsx:38-54`). `backlog:sdd-default-pipeline` is there; `diagnose_nudge_enabled` and `autonomous_stall_nudge_enabled` are not in the map (grep of the map shown), so they would render with the raw key as label if they appear in `flagList` at all. UNVERIFIED whether they are returned by the flag RPC; they live in `config/config.go:555/561`, not as `GetFeatureFlag` flags.
- Live note pattern: `LIVE_FLAG_NOTE` "Takes effect without reload" (`page.tsx:36`, `:165-169`). Reuse for B and flags.
- Live status line pattern: `GateStatusLine` polls a stats RPC and renders one sentence, rendering **nothing on failure** (`GateStatusLine.tsx:51-65`). Good for shadow counters but wrong for "unknown vs failed" (see section 5): silent absence hides failure.
- Scoped tri-state override pattern (Inherit/On/Off + "On - override"/"Off - from global" effective text): `GateKindOverrides.tsx:29-45`. Directly applicable to "global + per-scope" live-settable flags demanded by requirements. Segments are `minHeight/minWidth 2.75rem` (`page.css.ts:152-153`), i.e. 44px targets.
- Global force-on/off/clear + rehearsal: `components/settings/RolloutStatusSection.tsx:41-90`; used by StreamHub/Tymux panels (`page.tsx:204-205`). Precedent for a rollout "mode" panel; reuse for the A shadow/enforce flag.

**Defects in the current toggle (relevant because we add flags to it)**
- Visual toggle is 2.75rem x 1.5rem = 44x24px (`page.css.ts:59-67`): width passes, height is 24px, below the 44px touch target the repo already uses elsewhere (`page.css.ts:124,152`). Fix with a larger hit area (padding or pseudo-element), not a bigger visual.
- It uses `aria-pressed` plus an `aria-label` that flips between "Enable X"/"Disable X" (`page.tsx:190-191`). Screen readers then announce a pressed state *and* an inverted verb, which is ambiguous. The stages page uses the correct `role="switch"` + `aria-checked` (`backlog-stages/page.tsx:224-229`) with a similarly flipping label. Recommend `role="switch"`, `aria-checked`, a stable label, and `aria-describedby` pointing at the description/statusDetail node. UNVERIFIED that no test depends on the current attributes (`features/page.test.tsx` not read).
- The knob colour is set via inline style from `vars.color.primary` vs `borderColor` (`page.tsx:187-189`); On/Off is also in the text badge, so colour is not the only signal (good, WCAG 1.4.1).

**Numeric / select fields** (for B)
- `LivenessRowEditor.tsx:94-97`: `<label htmlFor>` + `<input type="number" min>` + `data-testid`; errors as `role="alert"` span (`:266`); hints via `styles.hint` (`:196`). Reuse this field shape.
- `GlobalDefaultsForm.tsx:327,348-352` uses `type="number"` and `inputMode="numeric"` for the mobile numeric keypad (VERIFIED lines; the context of the two inputs not read).
- `StuckItemDetail.css.ts:63-72`: 44px `minHeight` input. Match it.
- Stages page load/error/retry: `role="alert"` banner with Retry button (`backlog-stages/page.tsx:161-168`), per-row scoped errors (`:215-219`), "Loading…" text (`:193`). Reuse verbatim for B's settings load.

## 3. Comparable patterns (UNVERIFIED, from general knowledge)

- **Feature-flag consoles (LaunchDarkly, Unleash, GrowthBook)**: show the *effective* value and its source ("on - from rule X", "off - default") rather than just the stored value; the repo's `effectiveText` (`GateKindOverrides.tsx:36-40`) already does this. Dependent/prerequisite flags are shown as a blocked state with the prerequisite named. That is the right model for `autonomous_stall_nudge_enabled` AND-ed with `diagnose_nudge_enabled` (requirements.md:89).
- **Rollout stage vocabulary**: Off / Shadow (evaluate, log, do not act) / Enforce. Same tri-state the git-backend cohort soak uses (`docs/how-to/flip-git-backend-cohort.md`, cited by requirements.md:114, not opened). Reuse that word set so the operator learns one vocabulary.
- **Quota/budget settings (cloud billing budgets, API usage limits)**: a budget form shows (a) current usage vs limit, (b) thresholds as percentages with a "what happens at each" sentence, (c) a distinct "no budget set" state that is explicit rather than a 0 field. A numeric 0 meaning "disabled" is a known footgun; use an explicit control ("Proactive budget signal: Off / On with N tokens") instead of a magic zero.
- **Dry-run / audit modes (AWS Config, OPA, Kubernetes admission "warn")**: show "would have denied N transitions in 24h" to give evidence before enforcing. Maps to the divergence counter in requirements.md:108.

## 4. Mental models

- Operator's model: "if the UI says it, it is happening." Every state must be one of **Enforced / Shadow / Off / Inert (blocked by prerequisite) / Unknown**. The current Features page collapses everything to On/Off and so cannot express Inert or Shadow.
- A saved setting is not an applied setting. After a save, read back and show the effective value (consistent with global instruction: read a mutation back). Show "Saved" only after the read-back matches; otherwise show a role="alert" mismatch.
- Rollback must look as easy as enabling: one visible control per section, same place, same wording ("Revert to Off").

## 5. Per-surface recommendations

### 5.1 Features page: the three automation flags
1. Group under a heading "Autonomous backlog" so the AND relationship is visible spatially. Order: `diagnose_nudge_enabled` first, `autonomous_stall_nudge_enabled` indented beneath it.
2. State model for the dependent flag, rendered as text, not colour alone:
   - parent Off, child toggled On -> badge "On (inactive)" + line "Has no effect until Diagnose & Nudge is On." Toggle stays operable (the stored value is real) but the effective state is explicit.
   - both On -> "On".
3. Each flag row gets a default line: "Default: Off. Why: <one sentence>. Flip when: <exit criteria>. Rollback: toggle Off." Content comes from the workstream-D decision record; link to the doc. Do not hard-code criteria in JSX if they can be served from `description`/`statusDetail` (they would drift).
4. Keep `LIVE_FLAG_NOTE` on these three. Use `role="status"` (polite) for the post-toggle confirmation line; use `role="alert"` only for failures.
5. Add labels to `FEATURE_META` for the two nudge flags if they are surfaced here (`page.tsx:38`).
6. Fix the toggle a11y/target defects in section 2 before adding three more consumers. This is collateral debt but directly in the touched component.

### 5.2 Quota gate + soft budget (workstream B)
Placement: a section on the Features page (flags + live note live there) or a new `settings/backlog-quota` route (UNVERIFIED which; the Features page is already 212 lines of special cases, and `status_detail` text is the only current quota signal, `page.tsx:170`). Recommendation: new section component under `components/settings/`, linked from Features, mirroring `BacklogSourcesSettings`/`JulesSettings` placement.

Fields, in order:
1. **Quota gate** (switch). Helper: current effective state sentence from the server ("Gate is Off: sessions start regardless of quota").
2. **Assumed window token budget** (number, `inputMode="numeric"`, `min=0`, `step` coarse). Do not leave raw 0 as the off switch: show a second line that is always visible: "0 = proactive headroom signal disabled" and, when value is 0 and gate is On, a `role="status"` warning "Gate is on but no budget is set, so the proactive signal never fires." (requirements.md:88 hidden coupling). Prefer a separate "Proactive signal" switch if the proto change is acceptable (decision for plan, not UX).
3. **Soft-budget threshold** (number or percent; unit must be in the label, e.g. "Warn at (% of budget)" or "(tokens)"). Validation inline: required, integer, `>= 0`, and, if percent, `<= 100`; if tokens, warn when `> budget`. Error text names the constraint ("Enter a whole number from 1 to 100"), `aria-invalid`, `aria-describedby` to the error id, error `role="alert"`.
4. Save behaviour: either per-field blur-save with row-scoped status (stages toggle precedent, `backlog-stages/page.tsx:119-140`) or one Save button. Recommendation: single Save + Revert for a 3-field form so cross-field validation runs once; on success read back and show "Applied" with the effective values.
5. Effective-values line: "Effective now: gate On, budget 5,000,000 tokens, warn at 80%." Sourced from the server read-back, not local state.
6. Open question 4 (requirements.md:123) (derive budget from observed usage): if derivable, show the observed figure as a read-only "Observed last window: N" next to the input and offer "Use observed" as a button, never auto-fill silently. UNVERIFIED feasibility.

### 5.3 Repo auto-merge line in StuckItemDetail (workstream C)
Current: `autoMergeLine` returns `"Repo auto-merge: unknown"` whenever the optional bool is unset (`StuckItemDetail.tsx:54-60`), styled italic muted (`StuckItemDetail.css.ts:51-54`), rendered only when `isPrReady` (`StuckItemDetail.tsx:352-361`). Wire field: `optional bool allow_auto_merge = 11` (`proto/session/v1/backlog.proto:2144`). Proto optionality gives only 3 states (true/false/unset), but the requirement needs a 4th, "lookup failed", distinct from "not looked up / no PR repo". The UI cannot tell these apart today; the plan must add a status field (enum or error string), e.g. `auto_merge_lookup_status`. Do not overload unset.

Proposed copy (all with text, not colour only):

| State | Line | Secondary |
|---|---|---|
| true | "Repo auto-merge: allowed" | "GitHub setting only. Branch protection or required checks can still block it." (requirements.md:87) |
| false | "Repo auto-merge: not allowed on this repo" | "Merge this PR by hand, or enable Allow auto-merge in the repo settings." + link to repo settings (new tab, `rel="noopener"`) |
| not looked up (no PR repo / item pre-PR) | omit the line entirely | n/a: do not show "unknown" for a question never asked |
| lookup pending / stale | "Repo auto-merge: checking…" | n/a |
| lookup failed | "Repo auto-merge: couldn't check (<reason>)" | reason is short: "rate limited, retry after HH:MM", "no access to repo", "GitHub unreachable"; show "Last tried <ago>" using `formatAgo` (`stuckReason.ts`, used at `StuckItemDetail.tsx:348`) and a Retry button if a refresh RPC exists |
| value cached older than N | append "as of <ago>" | reuse `PR_STATUS_STALE_THRESHOLD_MS` idea (`stuckReason.ts:240-248`) |

Rules:
- "unknown" as a word should disappear from the product; it is ambiguous between never asked, in flight, and failed (the stuck-reason panel already has a proper "Couldn't check this PR's status" message, `StuckItemDetail.tsx:213-217`; align tone with it).
- Remove the raw `(allow_auto_merge: true)` debug suffix (`StuckItemDetail.tsx:59`); it leaks a field name to an operator.
- Failed-lookup line gets `role="status"`, not alert (informational, not blocking). Make it persistent text, never a toast.
- The "off" state should come with an action, matching the panel's existing pattern of an `actionCopy` paragraph (`:203-207`) rather than a bare fact.
- Pre-PR warning (requirements.md:73): show the same line, with the "not allowed" variant promoted to a `role="status"` warning in the PR-creation context. Warnings that block nothing must say so ("PR will still be created").

### 5.4 Stages settings: enforced vs shadow vs off (workstream A status)
Current page has no engine-mode indicator at all (`backlog-stages/page.tsx:144-159` header is title + description only), and the description ("Configure workflow stages, the transitions between them, and the gates...", `:151-154`) *claims* enforcement, which VERIFIED is false today (requirements.md:11). Until A ships, this copy is itself misleading.

Recommendation: a persistent **status banner directly under the h1**, always rendered (never hidden when Enforced), role="status":
- Enforced: "Enforced: edits here change which transitions and gates the backlog allows."
- Shadow: "Shadow: edits are compared against the built-in rules but are **not enforced**. N differences in the last 24h. [View differences]. Enforcement is changed in Feature Flags." Use warning styling plus the word "not enforced".
- Off: "Not in use: the built-in 9-stage workflow is active. Edits are saved but have no effect."
- Unknown/failed to load mode: "Couldn't determine whether these settings are enforced. Treat edits as not applied." + Retry; never default to the Enforced wording on failure (fail-closed, matching pi-support precedent `features/page.tsx:81-101`).
- Show mode as text plus an icon; do not put the control on this page (one place to flip: Features page), but link both ways ("Change in Feature Flags", and from the flag row "Edit stages").
- Save confirmation in `StageForm` must carry the mode: "Saved. Not enforced yet (shadow)" vs "Saved and enforced". UNVERIFIED how `StageForm` reports success (`StageForm.tsx` 504 lines, not read past label grep).
- The empty state "No stages configured — the built-in 9-stage workflow is active by default." (`page.tsx:196`) is already honest; keep, and align wording with the Off banner.
- Divergence counter display: reuse `GateStatusLine` shape but, unlike it, render a failed-fetch message instead of nothing (`GateStatusLine.tsx:60-61`).

## 6. Unknown vs failed vs loading: wording standard

Define once and apply to all four surfaces:

| Condition | Meaning | Copy pattern | ARIA |
|---|---|---|---|
| Loading | request in flight | "Checking…" / "Loading…" (stages uses "Loading…", `page.tsx:193`; detail uses "Checking current override…", `StuckItemDetail.tsx:71`) | `aria-busy` on region; no live announcement |
| Not applicable | question not asked (no PR repo, no PR yet) | omit the row | n/a |
| Known | value + provenance + age | "<value> (as of <ago>)" | `role="status"` on change only |
| Failed | asked, no answer | "Couldn't check <thing>: <short reason>. Last tried <ago>. [Retry]" | `role="status"` if informational, `role="alert"` if it blocks an action |
| Stale | answer older than threshold | value + "(as of <ago>, may be out of date)" | none |

Never use "unknown" alone. Never render a failed fetch as an absent element (current `GateStatusLine` behaviour is acceptable only for a purely decorative stat).
The existing ReworkCap line shows the right instinct (`StuckItemDetail.tsx:63-76`: separates "not loaded" from "confirmed unset"); copy that discipline.

## 7. Accessibility (WCAG 2.2 AA)

- 1.4.1 Use of colour: every state is text + badge; colour supplements only. Existing badge pattern complies (`features/page.tsx:155-160`).
- 1.4.3/1.4.11 contrast: muted italic text (`unknownNote` uses `vars.color.textMuted`, `StuckItemDetail.css.ts:51-54`) must meet 4.5:1 against the panel background in both themes. UNVERIFIED value; check tokens before reusing muted style for the new failed/stale states, which carry more important information than "unknown" did. Failed state should not be muted.
- 2.1.1/2.4.7 keyboard: all controls native `<button>`/`<input>`/`<select>`; visible `:focus-visible` ring (UNVERIFIED present in `page.css.ts`; grep before relying). Disabled-while-pending should use `aria-disabled` (as StuckItemDetail does, `:162-165`, to keep focus) rather than `disabled`, so focus is not lost when a save starts.
- 2.5.8 target size: 24px minimum, 44px repo convention. Fix flag toggle height (section 2).
- 3.3.1/3.3.3 errors: inline text, associated by `aria-describedby`, `aria-invalid`; summary `role="alert"` on submit.
- 3.3.7 redundant entry: do not clear fields on a failed save.
- 4.1.2 name/role/value: `role="switch"` + `aria-checked` + stable accessible name; description via `aria-describedby`.
- 4.1.3 status messages: confirmations via `role="status"`; do not move focus on success.
- Banner on stages page: place before the list in DOM order so screen-reader users hear the mode before editing.
- Dialogs (if any confirm "Enable enforcement"): follow `PiDisableWarningDialog` pattern (`features/page.tsx:207-209`); focus trap and Escape (UNVERIFIED implementation). Enabling enforcement is the one action warranting a confirm; shadow and quota edits do not.

## 8. Mobile and desktop (user requirement)

- Layout: rows already `flex-wrap` (`page.css.ts:117-123`); keep label above input at narrow widths, inline at wide. Quota form: single column under ~640px, two-column grid above (UNVERIFIED breakpoint tokens; `StuckItemDetail.css.ts`/`page.css.ts` use no `@media` that I saw: only the grep output with `media` matched none in `features/page.css.ts`).
- Touch: 44px targets for switches, segments, buttons, inputs (`overrideInput` already 44px).
- Mobile keyboard: numeric fields `inputMode="numeric"` + `pattern="[0-9]*"`, `enterKeyHint="done"`; do not use `type="number"` spinners as the only affordance (they are tiny on touch and scroll-wheel-change values on desktop; UNVERIFIED but widely documented). Keep `min` for validation. Keep `autoComplete="off"`.
- Save bar: sticky bottom bar on mobile can be covered by the on-screen keyboard; prefer an inline Save after the last field (UNVERIFIED how the app shell handles this; `page.css.ts:4-12` notes nested `overflow:hidden` shells pinned to viewport height, so sticky positioning inside the page is risky).
- Long copy: the status banners and "Default / Flip when / Rollback" lines must wrap, with no horizontal scroll and no truncation of the "not enforced" phrase.
- Tooltips are not allowed to carry essential info (no hover on touch). The "0 means disabled" and "AND-ed with" explanations must be visible text.
- Stage-form transitions editor (`TransitionRow.tsx`, 327 lines) was not reviewed for mobile; flag as UNVERIFIED and out of scope except for the banner.
- Test both form factors with Playwright viewport variants following `e2e-test-conventions` (data-testid / ARIA locators, no `waitForTimeout`). Existing selectors: `stuck-item-auto-merge` (`StuckItemDetail.tsx:357`), `feature-flag-row/-name/-status` (`page.tsx:151-158`), `backlog-stages-load-error` (`backlog-stages/page.tsx:162`).

## 9. State matrix to implement and test

For each of Features-flag row, Quota form, Auto-merge line, Stages banner: loading, known/enforced, shadow/inert variant, off, failed (with retry), stale, save-pending, save-failed, save-mismatch (read-back differs), plus keyboard-only and 360px viewport passes. Each cell needs a test (supports success metric 5, requirements.md:38).

## 10. Open items for the plan

1. Proto/API needs implied by UX: lookup-status field for auto-merge (4th state); engine mode + 24h divergence count; effective quota values read-back; whether the two nudge flags are exposed through the flag RPC or need their own endpoint (UNVERIFIED).
2. Decide quota placement (Features section vs own route) and Save model (single Save vs per-field).
3. Decide whether to split the "0 = off" budget into an explicit switch (needs config/proto change).
4. Copy for "Default / Flip when / Rollback" depends on the owner decision in workstream D (requirements.md:44).
5. Verify before building: focus-visible styles, text-muted contrast, `features/page.test.tsx` coupling to `aria-pressed`, `StageForm` success messaging.
