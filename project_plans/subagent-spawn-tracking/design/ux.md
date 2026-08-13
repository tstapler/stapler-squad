# UX Design: subagent-spawn-tracking

Source: `project_plans/subagent-spawn-tracking/requirements.md`,
`project_plans/subagent-spawn-tracking/implementation/plan.md`. Component under
extension: `web-app/src/components/sessions/SubStatusChip.tsx` (+
`SubStatusChip.css.ts`), consumed by
`web-app/src/components/sessions/SessionRow.tsx` (line 258).

This is a **passive status indicator**, not an interactive control. There is no
click/tap flow to design. The surface is: what text renders inside the existing
`chipWaitingForAgent` chip, under what conditions, and what the assistive-tech
equivalent of that text is. Per requirements, explicitly **not** designed here:
tooltip/expandable subagent list, real-subagent-vs-shell/monitor distinction,
aggregate `WORKING` state — each is flagged as deferred where relevant below,
not built.

## Ground truth confirmed by reading the code before designing

- `SubStatusChip` today renders a single literal string `⏳ Waiting for Agents`
  for `SubStatus.WAITING_FOR_AGENT`, with `role="status"`,
  `aria-label="Waiting for agents"`, and `title="Claude is waiting for
  background agents to finish"` (`SubStatusChip.tsx:34-44`). All three
  attributes are candidates for count interpolation — the plan updates the
  visible label but explicitly leaves `aria-label` and `title` unchanged
  (Pattern Decisions table, "Frontend badge rendering" row, and Story 5.1.1's
  Given/When/Then: *"the chip's `aria-label` remains `'Waiting for agents'`
  (unchanged, so no `aria-live` announcement spam on count-only churn)"*).
  **This is a UX gap** — see AC-4 below, which pushes back on that decision.
- `SessionRow.tsx:253-259` filters the chip out entirely for
  `UNSPECIFIED`/`READY`/`IDLE` substatus, but **`WAITING_FOR_AGENT` survives
  the filter** — confirmed by reading the exclusion list directly. The chip
  reliably renders whenever the backend reports this substatus, so the count
  surface is not at risk of being silently dropped by an unrelated filter.
- The chip lives inside `pathLine` (`SessionRow.css.ts:43-49`): a flex row
  with `overflow: hidden`, holding the abbreviated path text (which has its
  own `overflow: hidden; textOverflow: ellipsis` truncation) followed by the
  chip and then `GitHubBadge`. The chip itself uses `whiteSpace: "nowrap"` and
  `flexShrink: 0` (`SubStatusChip.css.ts:10-21`, the shared `chip` base
  style). This matters directly for the "layout shift" acceptance criterion
  below: the chip does not wrap or shrink, so growing digit count invisibly
  eats into the path text's already-truncating space rather than reflowing
  the row grid or changing row height.
- `subagentCount` arrives as a plain `number` prop, optional
  (`subagentCount?: number`), sourced from a proto3 `int32` scalar
  (`Session.subagent_count`). Proto3 scalars cannot distinguish "unset" from
  "explicitly zero" — both arrive as JS `0` — so the frontend has no wire
  signal to distinguish "no subagents" from "field not populated yet by an
  older/mid-rollout server." This directly shapes the "loading/stale" state
  design below: the only safe default is graceful degradation to today's
  plain chip, never a literal `"0 Agents"` or `"undefined Agents"`.

## Step 1 — Surfaces and states

| # | State | Trigger |
|---|---|---|
| 1 | Count > 1 (plural) | `subagentCount = 2..N`, `subStatus === WAITING_FOR_AGENT` |
| 2 | Count == 1 (singular) | `subagentCount = 1` |
| 3 | Count == 0 / absent | `subagentCount = 0` or `undefined` — renders today's plain chip |
| 4 | Rapid transition / flicker | `subagentCount` changes value on consecutive polls while `WAITING_FOR_AGENT` persists |
| 5 | Field present but stale/not-yet-populated | Proto field defaults to `0` before a real detection pass has run, or on a server that predates this feature (mid-rollout) |
| 6 | Out-of-range value (regex misparse) | Negative or absurdly large `subagentCount` from a malformed capture |
| 7 | Status leaves `WAITING_FOR_AGENT` | Turn completes, goes idle, errors, etc. |

Six surfaces plus the exit transition (#7, which is really state #3 reached a
different way — no distinct rendering, called out separately because it's the
one users most rely on for "did it finish").

## Step 2 — Wireframes and interaction notes

All states below render inside the same `pathLine` flex row shown in context;
only the chip itself changes.

### State 1 — Plural (N > 1)

```
┌──────────────────────────────────────────────────────────┐
│ ● my-feature-branch                                       │
│   ~/code/proj  ⏳ Waiting for 3 Agents   [PR #42 badge]    │
└──────────────────────────────────────────────────────────┘
        ^path (truncates)   ^chip (nowrap, fixed content)
```
- `role="status"`, `aria-label="Waiting for 3 agents"` (see AC-4 — this
  changes from the plan's "leave aria-label unchanged" decision),
  `title="Claude is waiting for 3 background agents to finish"`.
- No interaction affordance beyond the existing `title=` native tooltip on
  hover (unchanged mechanism — a browser-native tooltip, not a custom
  overlay). Per scope, this native tooltip text should include the count but
  must **not** attempt to list what the agents are doing — that's the
  deferred JSONL/per-subagent-description work.

### State 2 — Singular (N == 1)

```
   ⏳ Waiting for 1 Agent   ← "Agent", not "Agents"
```
- Same attributes, singular noun. This is the highest-risk copy bug in the
  feature (a template literal that forgets the singular branch reads "1
  Agents" — grammatically jarring and a classic Krug "makes me think" moment,
  however minor). Plan's Task 5.1.1.2 already branches on
  `subagentCount === 1 ? "Agent" : "Agents"` — confirmed correct in the plan
  text; AC-1 below exists to make sure this doesn't regress silently.

### State 3 — Count 0 or absent (today's behavior, unchanged)

```
   ⏳ Waiting for Agents   ← no number, exactly as it renders today
```
- `aria-label="Waiting for agents"`, `title="Claude is waiting for background
  agents to finish"` — the pre-existing, un-numbered copy. This is also the
  fallback for the stale/not-yet-populated case (state 5) and the
  out-of-range case (state 6, see below) — one degraded state serves three
  triggers, which is deliberately simple rather than three different error
  copies for a cosmetic feature.

### State 4 — Rapid transition (2 → 3 → 1 across consecutive polls)

```
 t0: ⏳ Waiting for 2 Agents
 t1: ⏳ Waiting for 3 Agents   (chip widens ~6px, no row reflow)
 t2: ⏳ Waiting for 1 Agent    (chip narrows, singular noun swaps in)
```
- No debounce exists anywhere in this detection→proto→UI pipeline today for
  any field (confirmed in plan's Pattern Decisions, "Debounce for count
  flicker" row) — this feature does not introduce one either, matching
  existing precedent for `DetectedStatus` itself. The visual consequence:
  the chip's pixel width and label re-render on every poll tick where the
  count changes, same cadence as every other live-updating chip field
  already in this UI (spinner state, elapsed time, etc.).
  Design position: **acceptable as-is** for a first version — flag as
  Unresolved Question #4 (already listed in the plan) rather than building
  smoothing logic nobody asked for yet.
- Because the chip's `whiteSpace: nowrap` / `flexShrink: 0` never wraps or
  compresses, digit-count growth cannot break the row grid — worst case it
  eats a few more characters of the already-ellipsizing path text next to it.
  No row-height jank, no other row jumping.

### State 5 — Stale/not-yet-populated proto field

```
   ⏳ Waiting for Agents   ← identical rendering to State 3
```
- Because proto3 cannot express "unset" for a scalar `int32`, and the plan's
  own render logic treats `subagentCount == null || subagentCount <= 0` as
  "no count," a server that hasn't populated the field yet, a client bundle
  that predates the field, and a genuinely-zero count are all
  indistinguishable — and **should be**, because the correct behavior in all
  three cases is identical: show the pre-existing chip, never a literal `"0
  Agents"` or `"NaN Agents"` or `"undefined Agents"`.

### State 6 — Out-of-range value (negative or absurd count)

```
 subagentCount = -1  →  ⏳ Waiting for Agents   (treated same as 0)
 subagentCount = 847 →  ⏳ Waiting for 847 Agents  (rendered verbatim — see below)
```
- Negative: per Task 1.2.1.1's backend guard (`if n, convErr :=
  strconv.Atoi(m[1]); convErr == nil && n > 0`), the backend already clamps
  negative/zero to `0` before the value ever reaches the proto. The frontend
  should defensively treat any `subagentCount <= 0` the same way (not trust
  the backend guard alone) — this is cheap insurance against a future
  backend regression re-exposing a negative value over the wire.
- Absurdly large (e.g. a regex mis-capturing a timestamp or PID as the
  count): there is no upstream validation of an upper bound anywhere in the
  plan. The design position here is to render the number verbatim rather
  than silently clamping or hiding it. Rationale: a wildly wrong number
  (`"Waiting for 847 Agents"`) is an obviously-broken, self-diagnosing
  signal a user or developer can screenshot and report; silently clamping to
  something plausible (e.g. capping display at 99) would hide the exact bug
  symptom needed for root-cause triage, and this feature's own risk framing
  is "miscounting is cosmetic, not functionally harmful." Flagging this as
  an explicit open question rather than a shipped decision — see AC-3.

### State 7 — Exit transition (leaves WAITING_FOR_AGENT)

```
 t0: ⏳ Waiting for 3 Agents
 t1: (chip disappears — SubStatus is now PROCESSING, READY, SUCCESS, etc.)
```
- No fade/animation is specified or expected — chip presence is driven
  purely by `subStatus` equality checks in `SessionRow.tsx`, same mechanism
  as every other substatus chip already. The count is never "left behind" —
  because the count is recomputed fresh every detection pass and is Go
  zero-value `0` whenever the winning status isn't `WaitingForAgent` (plan's
  reset-semantics Pattern Decision), there is no code path that can show a
  stale count from a *previous* WAITING_FOR_AGENT episode once the chip
  reappears for a *new* one — each occurrence starts from a fresh detection
  pass, not a carried-over frontend value.

## Step 3 — UX acceptance criteria (human-testable)

1. **Singular/plural correctness.** With `subagentCount = 1`, the chip reads
   exactly `Waiting for 1 Agent` (no trailing "s"). With `subagentCount = 2`
   through at least `9`, it reads `Waiting for N Agents`. Verify by rendering
   `<SubStatusChip subStatus={WAITING_FOR_AGENT} subagentCount={1}>` and
   `subagentCount={2}` side by side and reading the text — do not rely on
   test assertions alone, visually confirm no orphaned "s".

2. **No layout shift/jank as digit count grows.** In a session list with
   mixed counts (1, 2, 3, and a hypothetical multi-digit count like 12),
   confirm: (a) row height is identical across all rows regardless of digit
   count — the grid's row height is driven by `minHeight: 38px` on `.row`,
   not by chip content: `SessionRow.css.ts:19`; (b) the chip itself never
   wraps to a second line (`whiteSpace: nowrap` holds); (c) growing chip
   width is absorbed by the path text truncating further (already-ellipsized
   text, not a jarring reflow) rather than by the chip, GitHub badge, or
   action buttons moving position. Test by resizing the browser to a narrow
   column width where the path is already tight, and comparing a 1-digit vs.
   2-digit count.

3. **No numerically-impossible or misleading value ever reached the DOM.**
   Confirm the frontend guard treats `subagentCount` as "has a count" only
   when it is a finite number strictly greater than `0`
   (`typeof subagentCount === "number" && subagentCount > 0`, not just
   truthiness — this also correctly excludes `NaN`, which is falsy-adjacent
   but not caught by a plain `> 0` check unless `NaN > 0` is confirmed
   `false` in the guard, which it is in JS). Specifically test: `undefined`
   → plain chip; `0` → plain chip; `-1` → plain chip (never a literal "-1
   Agents"); `NaN` → plain chip (never "NaN Agents"). This is the concrete,
   testable form of "no dead ends" for a status-only display: the failure
   mode isn't a UI dead end, it's a chip that shows something false — and
   every one of these inputs must degrade to the known-good unnumbered chip,
   never render literal `"undefined"`/`"NaN"`/a negative number as user-facing
   text.

4. **Accessibility parity between the visible label and its assistive-tech
   equivalent — this is a change from the plan's current default.** The
   implementation plan (Story 5.1.1's Given/When/Then) explicitly leaves
   `aria-label="Waiting for agents"` unchanged when a count is present, to
   avoid `aria-live` announcement spam on count-only churn. **UX position:
   the `title=` attribute (a static native tooltip, not `aria-live`, and
   therefore carries no announcement-spam risk) should include the count**,
   e.g. `title="Claude is waiting for 3 background agents to finish"` —
   sighted users hovering get the same numeric information visible users get
   from the label text, closing the gap described in requirements' framing
   of "users cannot see how many." The `aria-label` staying unnumbered is a
   defensible tradeoff (chip has `role="status"`, not `aria-live="polite"`,
   so it isn't auto-announced on every poll regardless — confirm this in
   implementation: if `role="status"` in this codebase's live-region setup
   *is* wired to auto-announce, revisit updating `aria-label` too, since the
   spam concern would then apply equally to the visible label re-rendering).
   Test by inspecting the rendered DOM's `title` attribute at `subagentCount
   = 3` and confirming it contains "3". Confirm color contrast is unchanged
   by diffing the rendered class name — it must still be exactly
   `chipWaitingForAgent` (no new class, no inline color override), per the
   plan's explicit "extend text only" decision.

5. **The chip disappears cleanly when the underlying substatus changes**, and
   reappearing later never shows a leftover count from the prior occurrence
   before a fresh value is computed. Test by watching a session transition
   `WAITING_FOR_AGENT (3)` → `PROCESSING` → back to `WAITING_FOR_AGENT` with a
   different real count (e.g. `1`) and confirming the second occurrence
   shows `1`, not a flash of stale `3`.

6. **Out-of-range values render as a visible, un-clamped anomaly rather than
   being silently hidden or clamped to a plausible-looking number**, so a
   miscount is diagnosable rather than invisible. (Deferred decision, flagged
   for product sign-off — see State 6 above — but the default behavior
   should be "show the raw number," not "clamp.")

## Deferred (explicitly not designed here, per requirements scope)

- Tooltip/expandable list of individual subagent descriptions.
- Visually or textually distinguishing real Task-tool subagents from
  background shells/monitors (all three collapse into one `subagentCount`
  per the plan's "winning line wins" decision).
- An aggregate `WORKING` state spanning parent + subagents.
- Any debounce/smoothing of rapid count changes (State 4) — flagged as an
  open question, not built, matching the plan's own Unresolved Questions #4.
- An upper-bound clamp/cap on displayed count (State 6) — flagged for
  product feedback, not resolved here.
