# UX Design: session-classifier-pipeline

Grounded against the current implementation (not invented): `web-app/src/components/sessions/SessionCard.tsx`
(tag pills are bare `<span role="listitem">` inside a `role="list" aria-label="Session tags"` wrapper, with
**zero** per-tag `title`/`aria-label` today — the `Category` chip is the only per-item element with an
`aria-label` pattern, `` `Category: ${session.category}` ``, that this design extends), `TagEditor.tsx` (modal,
focus-trapped, Enter-to-add, immediate no-confirmation `×`-to-remove, save is deferred/batched via `onSave`),
`ApprovalRulesPanel.tsx` (source-filter tabs `all/user/seed/...`, fire-count column tooltip
`title="Number of times this rule fired in the last 7 days"`, per-row `aria-label`s
`` `Edit rule ${rule.name}` ``/`` `Delete rule ${rule.name}` ``/`(Enable|Disable) rule`, built-in badge
`title="Built-in rules cannot be disabled"`, **delete has no confirmation dialog today**), and
`RuleBuilderForm.tsx` (shared `fieldInput`/`fieldSelect`/`fieldTextarea`/`fieldLabel` row pattern, **no
`aria-invalid`/`aria-describedby` pattern exists yet anywhere in this file or `SuggestedRuleCard.tsx`**).

**Important correction to the implementation plan's assumption:** plan.md Task 6.2.2a asks to "mirror any
existing two-step-confirm pattern... check `ImportRulesModal.tsx`/`SuggestedRuleCard.tsx` for a precedent."
None exists — grepped for `confirm`/`Confirm`/`window.confirm` across the sessions/rules components: zero
hits. Surface 3 below is therefore a genuinely new interaction pattern, designed from WCAG/HCI first
principles rather than mirrored from an existing repo convention, and Surface 4/5's inline-regex-error
pattern is likewise new (not a `saveError`-reuse — `saveError` today is a bare unassociated error string).

## Surface inventory

| # | Surface | Treatment |
|---|---|---|
| 1 | Auto-tag pill provenance (SessionCard) | Full |
| 2 | `Unclassified` distinct visual state | Full |
| 3 | "May reappear" removal confirmation (TagEditor) | Full |
| 4 | Tagging-rule CRUD tab (ApprovalRulesPanel + RuleBuilderForm) | Full |
| 5 | Regex validation inline error (rule builder pattern field) | Full |
| 6 | Fire-count column reused for tagging rules | Condensed |

---

## Surface 1 — Auto-tag pill provenance

### Wireframe

```
SessionCard
┌──────────────────────────────────────────────────┐
│ fix-flaky-tests                          [● Live] │
│ bugfix/pr-poller · claude                         │
│                                                    │
│ Tags: ┌────────┐ ┌──────────┐ ┌────────┐          │
│       │ Bugfix │ │ Frontend │ │ MyTag  │          │
│       └────────┘ └──────────┘ └────────┘          │
│          ▲                                        │
│          focus/hover → native tooltip:            │
│          "Applied by rule: Bugfix branch"         │
│                                       [Edit Tags] │
└──────────────────────────────────────────────────┘
```

Same pill styling as today (no new color/border) — per `research/ux.md`'s "provenance on demand, no
permanent visual tax" finding. `Frontend`/`MyTag` (no provenance) show no tooltip.

### Interaction flow

1. Pill renders with `title` set only when the session's `ruleTagProvenance` map has an entry for that
   tag value — `title="Applied by rule: {rule.Name}"` for a `TaggingRule.ID`-keyed entry, or
   `title="Applied by AI classification"` for the `"llm"` sentinel. Tags with no provenance entry get no
   `title` (unchanged from today).
2. Sighted mouse user: hovers pill → browser-native tooltip after the OS/browser's standard hover delay.
3. Keyboard user: Tabs into the tag list (each pill becomes a focusable element, a change from today's
   non-interactive `<span>` — implemented as `tabIndex={0}` + `role="listitem"`, not a `<button>`, since
   there's no click action here) → same native tooltip appears on focus, no extra keystroke.
4. Screen reader user: every pill (auto and manual) gets an `aria-label`: `` `Tag: ${name}` `` for manual
   tags, `` `Tag: ${name} (auto-applied by rule)` `` or `` `Tag: ${name} (auto-applied by AI)` `` for
   provenance-carrying ones — read on focus, no hover needed.

### Error / edge cases

| Case | Behavior |
|---|---|
| Provenance entry points at a rule ID that no longer exists (rule deleted after tag applied) | Tooltip degrades to `"Applied automatically — originating rule no longer exists"`, never blank/`undefined`. |
| Rule name is very long | Native tooltip wraps/truncates per browser default — no custom truncation logic needed. |
| Session has 10+ tags (long list) | No change to existing wrap/overflow behavior of the tag row — out of scope for this project. |

### Acceptance criteria

- **AC1**: Every tag pill is keyboard-focusable via Tab, in existing tab order, before the "Edit Tags" button.
- **AC2**: A provenance-carrying pill's accessible name (via `aria-label`) includes "auto-applied" text; a
  manual tag's does not — verifiable by an axe/screen-reader check without needing to hover.
- **AC3**: Hovering or focusing a provenance pill shows the tooltip within one interaction (hover-dwell or
  focus); a manual tag shows none.
- **AC4**: No dead end — the tooltip is purely informational (no action can be taken from it, nothing to
  escape from incorrectly); it dismisses on blur/mouse-out with no lingering state.
- **AC5**: Pill text/border contrast remains ≥ 4.5:1 in both light and dark themes (no new color introduced
  by this surface, so this is a non-regression check against the existing pill style).

---

## Surface 2 — `Unclassified` distinct visual state

### Wireframe

```
Tags: ┌ ─ ─ ─ ─ ─ ─ ─ ┐
      ╎ ? Unclassified ╎   (dashed border, muted gray — no × control)
      └ ─ ─ ─ ─ ─ ─ ─ ┘
         ▲ title: "LLM classification failed or timed out —
                    will retry next poll cycle"
```

The `?` glyph (or equivalent icon) is required, not optional — WCAG 1.4.1 (use of color) forbids the
dashed border alone from being the *only* signal, since border-style differences are easy to miss and
some users override borders via forced-colors/high-contrast modes.

### Interaction flow

1. `Unclassified` appears automatically (LLM poller failure/timeout path) — no user action creates it.
2. User can view it in `SessionCard` (read-only, per §1's affordances) or open `TagEditor`, where it is
   listed like any other tag row **except with no `×` remove button rendered** for that specific value.
3. It disappears automatically the moment any real tag (sync-rule- or LLM-derived) is applied — no user
   action removes it, per Epic 3.4's `dropUnclassifiedIfOtherTagsPresentLocked` shared rule.

### Error / edge cases

| Case | Behavior |
|---|---|
| User opens `TagEditor` while `Unclassified` is present | Row renders with a small caption instead of a remove button: `"Removed automatically once classification succeeds"` — never a silently-missing control with no explanation. |
| LLM keeps failing indefinitely (misconfigured API key, etc.) | `Unclassified` persists indefinitely; tooltip's "will retry next poll cycle" remains accurate (it does keep retrying) — no separate "give up" state designed, matching the requirements' non-critical/no-alert framing. |
| A user manually types `"Unclassified"` into the add-tag input | Out of scope to special-case — it behaves like any manually-added tag with no provenance (gets a remove `×` normally); the auto-managed no-remove-button behavior only applies when the tag has no user-authored provenance and was placed by the LLM-failure path. Flag as an implementation nuance: distinguish by provenance/sentinel presence, not by string value alone, to avoid this edge case. |

### Acceptance criteria

- **AC6**: `Unclassified` is visually distinguishable via *both* a non-solid border and an icon glyph — not
  color alone (WCAG 1.4.1), passable in a forced-colors browser mode.
- **AC7**: `Unclassified` has no remove (`×`) control in `TagEditor`; the row instead shows explanatory
  caption text — never a plain missing button with no context.
- **AC8**: Tooltip text explicitly frames the state as transient ("will retry"), not a permanent verdict.

---

## Surface 3 — "May reappear" removal confirmation (new pattern; no existing precedent)

### Wireframe

```
TagEditor — default row:
┌─────────────────────────────────────────────┐
│ Bugfix                                   [×] │
└─────────────────────────────────────────────┘

Click [×] on "Bugfix" (has rule provenance) → row expands in place:
┌───────────────────────────────────────────────────────────────┐
│ Bugfix   Applied by rule "Bugfix branch" — may reappear.       │
│                                  [ Keep ]  [ Remove anyway ]   │
└───────────────────────────────────────────────────────────────┘
                 ▲ focus moves here on expand (safe default)

Click [×] on "MyTag" (no provenance) → removed immediately, unchanged from today.
```

Deliberately an **inline row expansion**, not a nested modal-in-modal (TagEditor is already a modal
dialog; stacking a second modal dialog inside it fails WAI-ARIA's single-active-dialog convention and is
heavier than the risk warrants).

### Interaction flow

1. User clicks/activates the `×` on a tag row.
2. If the tag has a `ruleTagProvenance` entry: the row expands into the confirmation state shown above.
   Focus moves programmatically to the **`Keep`** button (the non-destructive default — matches standard
   destructive-confirmation convention of defaulting focus away from the destructive action).
3. `Remove anyway` — removes the tag from the editor's local (unsaved) list, exactly like today's
   immediate-removal path, plus marks it for suppression (persisted on `Save Tags`, per `SuppressedRuleTags`).
4. `Keep` or `Escape` — collapses the row back to its default state, no change.
5. If the tag has no provenance entry: `×` removes immediately, identical to today's behavior — this is
   the only path that stays exactly as-is.
6. Opening a second row's confirmation while one is already expanded auto-collapses the first (only one
   confirmation open at a time, to avoid a cluttered stack of warnings in the small modal).

### Error / edge cases

| Case | Behavior |
|---|---|
| Provenance rule ID no longer resolves to a live rule (deleted) | Confirmation text falls back to `"This tag was applied automatically and may reappear — remove anyway?"` (no rule name available), never a blank/`undefined` name. |
| User presses `×` then immediately closes the whole `TagEditor` modal without confirming | No side effect — the pending confirmation state is local/unsaved, discarded with the rest of the unsaved edits (consistent with the editor's existing "Save Tags" batching). |
| User confirms removal, then re-adds the same tag name before saving | Re-adding clears the pending-suppression flag for that value in the same session (mirrors `AddTag`'s existing suppression-clear behavior) — the tag is fair game for a rule to reclaim on the next fixpoint pass. |

### Acceptance criteria

- **AC9**: Removing a provenance tag takes exactly 2 actions (activate `×`, then activate `Remove anyway`)
  — never removed on the first click/keypress.
- **AC10**: Removing a plain user tag takes exactly 1 action — zero regression to existing behavior.
- **AC11**: The confirmation names the specific rule when resolvable, and degrades to generic wording when
  not — never renders `undefined`/blank text.
- **AC12**: `Keep` and `Escape` both fully cancel with no side effects; every state in this flow has an
  exit (no dead end).
- **AC13**: The entire flow is keyboard-operable: `×`, `Keep`, and `Remove anyway` are real `<button>`
  elements in Tab order; focus is programmatically moved to `Keep` when the confirmation expands (WCAG 2.4.3).

---

## Surface 4 — Tagging-rule CRUD tab

### Wireframe — rule list

```
Rules Page
┌─────────────────────────────────────────────────────────────────┐
│ [ All ] [ User ] [ Seed ] [ Tagging Rules ]   ← existing tab row │
├─────────────────────────────────────────────────────────────────┤
│ Name            Match           Output Tag  Pri  Fires(7d)      │
│ Bugfix branch   branch:^(bug…   Bugfix        50    12   [E][D] │
│ Claude program  program:claude  Claude       100     0   [E][D] │  ← 0 fires = dead-rule signal (§6)
│ ⓘ Built-in                                                       │
│                                            [+ Add Tagging Rule]  │
└─────────────────────────────────────────────────────────────────┘

Empty state (all tagging rules removed/disabled by user):
┌─────────────────────────────────────────────────────────────────┐
│              No tagging rules configured.                       │
│         Sessions will rely on LLM classification only.          │
│                       [+ Add Tagging Rule]                       │
└─────────────────────────────────────────────────────────────────┘

Loading state (tab just opened): existing skeleton/spinner row pattern from ApprovalRulesPanel, reused as-is.
```

### Wireframe — rule builder (add/edit)

```
┌─ Add Tagging Rule ────────────────────────────────────────┐
│ Name:            [ Bugfix branch                        ]  │
│ Match against:   [ Branch ▾ ]  (Name / Branch / Path / Program) │
│ Pattern:         [ ^(bugfix|fix)/                        ]  │
│ Requires tags:   [ (optional, comma-separated)           ]  │
│ Output tag:      [ Bugfix                                ]  │
│ Priority:        [ 50 ]        Enabled: [x]                 │
│                                                               │
│                                    [ Cancel ]  [ Save Rule ] │
└──────────────────────────────────────────────────────────┘
```

Scoped per the implementation plan (Task 6.1.1b): one "match against" selector + one pattern field, not
four simultaneous pattern boxes — the domain model (`TaggingRule`) supports all four patterns ANDed
together, but the initial UI only exposes single-field matching; multi-field rules remain editable via the
MCP/ConnectRPC API directly if ever needed. This is a stated scope decision, not a UI gap.

### Interaction flow

1. Click the "Tagging Rules" tab → list loads (existing loading-skeleton reused), then shows the table or
   the empty state.
2. Click "+ Add Tagging Rule" → builder form opens (same inline-expand shape `ApprovalRulesPanel` already
   uses for approval rules).
3. Fill Name / Match-against / Pattern / (optional) Requires-tags / Output tag / Priority / Enabled.
4. Pattern field validates client-side on blur/change (see Surface 5) — `Save Rule` remains enabled at all
   times (never disabled-on-invalid — disabling buttons hides *why* they can't be clicked, which is worse
   for discoverability than a rejected submit); clicking it while the pattern is invalid keeps the form
   open, moves focus to the pattern field, and surfaces the same inline error.
5. On successful save: row appears/updates in the table immediately, form closes, no full page reload.
6. Edit/Delete/Enable-Disable buttons mirror `ApprovalRulesPanel`'s exact `aria-label` convention:
   `` `Edit tagging rule ${rule.name}` ``, `` `Delete tagging rule ${rule.name}` ``,
   `` `${enabled ? "Disable" : "Enable"} tagging rule` ``.
7. Delete: **no confirmation dialog**, matching today's approval-rule delete behavior exactly (a known,
   pre-existing repo convention this project carries forward rather than fixes — flagged below, not a
   regression this project introduces).

### Error / edge cases

| Case | Behavior |
|---|---|
| Invalid regex on save (see Surface 5) | Inline error, save blocked, focus moves to pattern field. |
| Save RPC fails (network/server error) | Inline banner at top of the form: `"Failed to save rule: {message}"` + a `Try again` button — form stays open with entered values intact, never silently discarded. |
| `Requires tags` references a tag no rule currently outputs | Non-blocking inline hint: `"No rule currently produces the tag '{X}' — this rule may never fire"` (per `research/ux.md` §4 option 2). Marked **P2/stretch** per the plan's own characterization — not required for ship, but designed here so it's a drop-in addition later. |
| Attempting to delete/disable a seeded (`Source: "seed"`) rule | Same built-in-badge treatment as approval rules: `title="Built-in rules cannot be disabled"`, Delete/Disable controls disabled — consistency with the existing seed-rule convention. |
| Two rules share the same Output tag | Allowed, no error — multiple rules legitimately producing the same tag is expected (redundancy/chaining), not a conflict. |

### Acceptance criteria

- **AC14**: A user can create a tagging rule (5 required fields + `Save Rule`) and see it live in the list
  in ≤ 6 interactions, no page reload.
- **AC15**: An invalid pattern is caught and explained before the save round-trip, not only after a server
  rejection.
- **AC16**: Every control (tab, add, edit, delete, enable/disable, save, cancel) is Tab-reachable and
  Enter/Space-operable.
- **AC17**: Edit/Delete/Enable-Disable `aria-label`s exactly match the existing approval-rule naming
  convention (verified: this repo's e2e locators are ARIA-only, so a mismatched label breaks page-helper
  reuse, not just cosmetics).
- **AC18**: No dead end — a failed save offers `Try again`; `Cancel` returns to the list with no orphaned
  partial row.

---

## Surface 5 — Regex validation inline error (rule builder pattern field)

### Wireframe

```
Pattern:  [ ^(unterminated[                              ]
          ⚠ Invalid regex: Unterminated character class
```

### Interaction flow

1. On blur (and on every keystroke after the first blur, to give live feedback while correcting) the
   field runs `new RegExp(value)` in a `try/catch`.
2. On throw: input gets `aria-invalid="true"` and `aria-describedby="tagging-rule-pattern-error"`; an
   error `<p id="tagging-rule-pattern-error">` renders directly below the field with the caught error's
   message (or a friendlier paraphrase — implementer's call, but must not be silently swallowed).
3. On a subsequent valid edit: `aria-invalid` is removed, the error paragraph unmounts.
4. Clicking `Save Rule` while invalid re-validates and re-shows the same error (idempotent — no double
   error text, no flicker).

### Error / edge cases

| Case | Behavior |
|---|---|
| Empty pattern | Treated as "no error" (empty is not attempted as a regex) unless the field is otherwise required by "Match against" selection — if required and empty, a distinct message: `"Pattern is required for {field} matching"`. |
| Pattern valid regex syntax but pathologically slow (e.g. catastrophic backtracking) | Out of scope for client-side detection — no ReDoS linting in this pass; flagged as a known gap, not silently ignored. |
| Screen reader user | `aria-invalid`/`aria-describedby` ensure the error is announced immediately on the field gaining/losing the invalid state, not only visually. |

### Acceptance criteria

- **AC19**: An invalid regex shows field-level error text within the same interaction, before any submit
  attempt.
- **AC20**: The error is programmatically associated to the field via `aria-invalid` + `aria-describedby`
  — not just visually adjacent text.
- **AC21**: Correcting the pattern removes the error state and its ARIA association in the same render
  pass — no stale error left announced.

---

## Surface 6 — Fire-count column reused for tagging rules (condensed, non-interactive display)

Representative sample (same table as Surface 4, this is one column of it):

```
Name             Pri   Fires(7d)
Bugfix branch     50      12        (title="Number of times this rule fired in the last 7 days")
Claude program   100       0        ← same column/tooltip; zero fires flags a dead rule passively
```

Acceptance criteria:

- Tooltip text is reused **verbatim** from the approval-rule column (`"Number of times this rule fired in
  the last 7 days"`) — no new copy to write or translate.
- A 0-fire tagging rule uses no shaming/red styling — passive, scannable/sortable signal only, consistent
  with the approval-rule column's existing tone.
- No new route/surface — this is purely a column addition to Surface 4's existing table.
- Fire-count data source is whatever mechanism already aggregates approval-rule fire counts, extended to
  the `"tagging rule fired"` log line from the plan's Observability Plan — a backend-implementer decision,
  not a new UX surface.

---

## Cross-cutting accessibility requirements (all surfaces)

- **Keyboard**: every interactive element introduced by this project (pill focus target, `×`/`Keep`/`Remove
  anyway` buttons, tab row, rule-builder fields, `Save Rule`/`Cancel`) is reachable via Tab in a logical
  order and operable via Enter/Space, per WCAG 2.1.1.
- **Screen reader labels**: every new/modified element has an explicit `aria-label`, `aria-describedby`, or
  accessible text content — never a bare icon-only control with no name.
- **Color contrast**: no surface here introduces a new color; all pill/border/text combinations must meet
  ≥ 4.5:1 in both the existing light and dark themes (regression check against the current design-system
  tokens, not a new palette).
- **Use of color (WCAG 1.4.1)**: `Unclassified`'s distinction (Surface 2) uses border-style + icon, not
  color alone.
- **No dead ends**: every error/confirmation state introduced (Surfaces 2, 3, 4, 5) has an explicit exit —
  retry, cancel, keep, or auto-resolution — enumerated in each surface's edge-case table above.
- **ARIA-only e2e locators**: per this repo's `e2e-test-conventions` skill, every new control must be
  reachable by `data-testid` or ARIA role/label — none of the interactions above rely on a CSS class as the
  only selector.

## Summary of testable UX acceptance criteria

21 acceptance criteria (AC1–AC21) across 5 full-treatment interactive surfaces, plus 4 condensed criteria
for the non-interactive fire-count column (Surface 6) — 25 total UX acceptance criteria over 6 surfaces.
