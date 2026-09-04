# UX Design: GitHub Provenance Display + Two-Way Sync Controls

Design phase for `backlog-github-two-way-sync`. Grounded in `requirements.md` (AC0-AC7),
`research/ux.md` (patterns, a11y, JTBD), and `implementation/plan.md` Phase 4 (Epics 4.1-4.3,
the surfaces actually scoped to be built). This doc does not introduce any surface beyond what
Phase 4 already plans — it specifies layout, flow, and acceptance criteria for exactly those
surfaces, plus the error/edge states research flagged as needing a home.

Four surfaces:
1. Backlog item card — provenance badge (Epic 4.1)
2. Backlog item detail view — Source section (Epic 4.2)
3. Settings > Backlog Sources — sync-direction toggles + warning (Epic 4.3, Story 4.3.1)
4. Settings > Backlog Sources — persistent row-level failure warning (Epic 4.3, Story 4.3.2)

---

## Surface 1: Backlog Item Card — Provenance Badge

### Wireframe

```
┌────────────────────────────────────────────┐
│ Fix flaky retry timer            P2  ready │  ← cardHeader (existing)
│                                             │
│ 3/5 done          [Mark Ready]   [ Ⓖ #42 ] │  ← cardFooter: AcSummary, action button,
└────────────────────────────────────────────┘     NEW provenance badge (only if externalUrl set)

Card with NO linked issue (locally created):
┌────────────────────────────────────────────┐
│ Refactor session storage layer    P3  idea  │
│                                             │
│ 0/2 done                    [Mark Ready]   │  ← no badge rendered at all
└────────────────────────────────────────────┘
```

Badge detail (desktop hover / mobile tap-and-hold shows native title tooltip):

```
[ Ⓖ #42 ]
  ↑  ↑
  │  └─ visible text: "#<externalId>"
  └──── lucide-react Github icon, aria-hidden, 12px (matches compact-card budget)
```

### Interaction Flow

| Step | User action | System response |
|---|---|---|
| 1 | Card renders, item has `externalUrl` | Badge appears in `cardFooter`, right-aligned after the action button |
| 2 | User clicks the badge | `e.stopPropagation()` fires (badge carries `data-action-button="true"`, covered by the card's existing `handleCardClick` guard at `BacklogItemCard.tsx:133`); browser navigates to `externalUrl` in a new tab (`target="_blank" rel="noopener noreferrer"`) — the card's own `onClick` (open detail panel) does **not** fire |
| 3 | User tabs to the badge via keyboard | Badge is a real `<a href>`, natively focusable; visible focus ring per repo's existing focus-visible styles; `Enter` activates it identically to a click |
| 4 | Screen reader encounters the badge | Announces "Imported from GitHub issue #42, link" (from `aria-label`) — icon itself is not separately announced (`aria-hidden="true"`) |
| 5 | Item has no `externalUrl` | Nothing renders — no placeholder, no empty badge shell |

### Error / Edge Cases

| Case | Behavior |
|---|---|
| `externalUrl` present but issue was deleted/transferred on GitHub (404) | Badge still renders (this surface has no live-fetch, so it can't know the link is dead without the backward-sync poll cycle running first). Once backward-sync's next poll detects the 404/410 (see Surface 3/4 below), a muted/struck-through badge variant should apply — **flagged for plan follow-up**: Phase 4's plan text (Epic 4.1) does not currently spec a "broken link" visual variant; this is a gap between the plan and research §4's "muted/struck-through variant" recommendation. Recommend adding `item.externalLinkBroken?: boolean` (or equivalent) as a follow-up story if AC2's "no dead-looking clickable link" bar is meant to be met precisely — noting it here rather than silently assuming it's covered. |
| Very long repo/issue number width overflow on narrow mobile card | Badge text is fixed-format `#<n>` (short, numeric) so it does not risk the same overflow class as long titles; no truncation needed |
| Touch target size on mobile | Badge must meet the 44×44px (iOS) / 48×48dp (Android) minimum tap target even though its visual content is small — pad the `<a>` box via CSS (not just icon+text bounding box) so a thumb tap doesn't miss it or collide with the adjacent action button |

### UX Acceptance Criteria

- **AC-1.1**: User can identify that a card represents an imported GitHub issue in 0 additional clicks (badge is always visible when applicable, no expand/hover required to discover its existence).
- **AC-1.2**: User can open the source GitHub issue in exactly 1 click/tap, without accidentally opening the item's detail panel instead.
- **AC-1.3**: Badge and card action button both meet the 44×44px minimum touch target on mobile, with no overlapping hit regions.
- **AC-1.4**: Badge is keyboard-reachable via Tab, in the card's existing tab order, and activates on Enter.
- **AC-1.5**: Screen reader announces "Imported from GitHub issue #<n>" — verified via `aria-label`, never relying on icon-only or visual-only conveyance (icon is `aria-hidden`).
- **AC-1.6**: Badge background/border/text colors meet ≥4.5:1 contrast against the card background in both light and dark themes (verified against `vars.color.*` tokens already contrast-audited per `.claude/rules/css-architecture.md`, not a new hardcoded color).
- **AC-1.7**: No badge (empty or placeholder) renders for locally-created items with no `externalUrl` — confirmed absence, not just visual emptiness.

---

## Surface 2: Backlog Item Detail — "Source" Section

### Wireframe

```
┌──────────────────────────────────────────────────────────┐
│  Fix flaky retry timer                                    │
│  ready · P2                                                │
│  ──────────────────────────────────────────────────────   │
│  ▸ Acceptance Criteria (3/5 done)                          │  ← existing CollapsibleSection
│  ▸ Progress History                                        │  ← existing
│  ▾ Source                                          [ – ]   │  ← NEW CollapsibleSection,
│     Ⓖ Issue #42 — "Retry timer fires twice on reconnect"   │     collapsed by default
│     [bug] [p1] [flaky-test]                                │  ← label chips, GitHubIssuePicker
│  ▸ Workflow History                                        │     labelBadge style
└──────────────────────────────────────────────────────────┘

Item with no linked issue: "Source" section header does not render at all
(same guard-at-call-site pattern as PullRequestSection — parent decides, not the section).
```

### Interaction Flow

| Step | User action | System response |
|---|---|---|
| 1 | Detail view opens for an item with `externalUrl` | "Source" section appears in the section list, collapsed by default (`defaultExpanded={false}`) alongside other `CollapsibleSection`s |
| 2 | User clicks/taps the section header | Expands to show issue link + label chips; state persists per the existing `CollapsibleSection` persistence convention (`sectionKey="source"`) |
| 3 | User clicks the issue link | Opens `externalUrl` in a new tab; link is a real `<a>`, not a click-handler div |
| 4 | User hovers/focuses a label chip | Native `title={label}` tooltip shows full label text (guards against any future truncation) |
| 5 | Item has no `externalUrl` | Section is entirely absent from the section list — no "Source (not linked)" placeholder cluttering non-imported items |

### Error / Edge Cases

| Case | Behavior |
|---|---|
| Issue deleted/transferred (backward sync's next poll gets 404/410) | Sync history logs a distinct message: *"issue not found — may have been deleted or transferred; backward sync paused for this item"* (per research §4). The Source section's link should carry a `title` attribute noting this (e.g. `title="This issue may have been deleted or moved on GitHub"`) once that signal is available — same follow-up gap noted in Surface 1 (plan's Epic 4.2 doesn't yet spec the wiring for this flag; flagging rather than assuming). |
| Labels array empty (`labels: []`) | Label-chip row does not render at all; only the issue link shows — no "no labels" placeholder |
| Very long issue title | Truncate with ellipsis + `title` attribute holding the full string (standard pattern, matches existing card title handling) |
| Forward-sync is on, item transitions to `done`, but `externalUrl` is empty (no linked issue) | Nothing to sync — the item has no Source section to show in the first place (already covered by the guard). Sync history should still log a no-op entry: *"nothing to sync — no linked GitHub issue"* (research §4) so the skip is visible/auditable rather than silent, per this repo's own "document AI decisions in edge cases" convention. |

### UX Acceptance Criteria

- **AC-2.1**: User can view the full source issue title, link, and all labels in ≤2 clicks (1 to open detail view if not already open, 1 to expand "Source").
- **AC-2.2**: User can navigate to the GitHub issue from the detail view via a real, keyboard-activatable `<a href>` — no click-only `<div>`.
- **AC-2.3**: Section is completely absent (not just empty) for items with no linked issue — verified via DOM query, not just visual inspection.
- **AC-2.4**: Label chips reuse the existing `labelBadge` token styling (`GitHubIssuePicker.css.ts`) — no new color/shape introduced for the same semantic concept.
- **AC-2.5**: No dead end: if the section is expanded and the link turns out to be broken (issue deleted upstream), the sync-history log (reachable from Settings, not buried) explains why, rather than the user hitting a silent 404 with no explanation anywhere in the product.

---

## Surface 3: Settings > Backlog Sources — Sync-Direction Toggles

### Wireframe

```
┌───────────────────────────────────────────────────────────────┐
│ acme/widget-app                              github_issues  ⏻ │  ← existing row header
│ Last synced: 2026-08-03 09:14 AM                                │
│ [Sync now]  [View history]                                     │
│ ┌─ Sync with GitHub ─────────────────────────────────────────┐│  ← NEW sub-heading + group
│ │ ⏻  Close GitHub issues when I finish here                  ││
│ │     When a backlog item is marked done, close its linked   ││
│ │     GitHub issue.                                          ││
│ │     Label to apply on close (optional): [___________]      ││  ← visible only if toggle ON
│ │                                                              ││
│ │ ⏻  Reflect GitHub status back here                         ││
│ │     When the linked GitHub issue is closed or relabeled,   ││
│ │     update this backlog item to match.                     ││
│ │                                                              ││
│ │ ⚠ Both directions are enabled — closing this item's issue   ││  ← only if BOTH toggles ON
│ │   may be observed and re-applied by backward sync. Verify   ││
│ │   this doesn't create a loop for items you also edit        ││
│ │   manually.                                                  ││
│ └──────────────────────────────────────────────────────────────┘│
└───────────────────────────────────────────────────────────────┘
```

Mobile (narrow viewport — toggle group stacks full-width, same content, no truncation):

```
┌─────────────────────────────┐
│ acme/widget-app          ⏻  │
│ github_issues                │
│ Last synced: 09:14 AM        │
│ [Sync now]                   │
│ [View history]                │
│ ── Sync with GitHub ──────── │
│ ⏻ Close issues when done     │
│   (helper text wraps)        │
│   Close label: [_________]   │
│ ⏻ Reflect GitHub status back │
│   (helper text wraps)        │
│ ⚠ Both directions enabled…   │
│   (warning text wraps)       │
└─────────────────────────────┘
```

### Interaction Flow

| Step | User action | System response |
|---|---|---|
| 1 | User opens Settings > Backlog Sources | Existing source rows render; each shows the new "Sync with GitHub" group with both toggles OFF by default (per AC3/AC4 — nothing changes until explicit opt-in) |
| 2 | User clicks "Close GitHub issues when I finish here" toggle | `role="switch" aria-checked` flips to `true`; calls `setForwardSyncEnabled`; on success, the close-label text input becomes visible/enabled |
| 3 | User types into the close-label input | Local state updates; value is persisted via `setForwardSyncCloseLabel` (debounced or on-blur — plan doesn't specify; recommend on-blur to avoid a write-per-keystroke) |
| 4 | User clicks "Reflect GitHub status back here" toggle | Same pattern; calls `setBackwardSyncEnabled` |
| 5 | Both toggles now ON | Inline warning appears immediately (pure client-side derived state — no round trip needed since both values are already in local state) |
| 6 | User turns either toggle back OFF | Warning disappears immediately; close-label input hides again if forward toggle turned off (its value is preserved in state, not discarded, in case re-enabled) |

### Error / Edge Cases

| Case | Behavior |
|---|---|
| Toggle write fails (network error, backend rejects) | Toggle should NOT optimistically flip and stay flipped on failure — revert to previous state and surface the existing `lastError` banner (`errorMessage` styles.errorMessage), consistent with how `handleToggleEnabled`'s existing `enabled` toggle already handles a failed update (it awaits and only updates local state via `refresh()` after success) |
| First-enable of backward sync on a source with many pre-existing imported items | Per plan Unresolved Question #3, this ships as a static inline warning only — no preview/dry-run of which items would bulk-transition. This is a known, documented gap (not silently assumed safe); UX-wise, recommend the warning copy explicitly says so if a future iteration adds bulk-transition, to avoid a surprise mass-status-change the first time backward sync runs. Flagged, not fixed, in this design pass — matches the plan's own scoping. |
| User enables forward sync but leaves close-label blank | Valid state — "optional" per the input's placeholder copy; forward sync should still close the issue, just without applying a label |

### UX Acceptance Criteria

- **AC-3.1**: User can enable either sync direction in exactly 1 click per direction (no confirmation modal for turning a single direction on — modals are reserved for genuinely destructive/irreversible actions, and this is reversible by toggling back off).
- **AC-3.2**: Both toggles default to OFF/`false` on first load for every existing source — verified against fetched `source.forwardSyncEnabled`/`backwardSyncEnabled`, never hardcoded true client-side.
- **AC-3.3**: When both toggles are ON, the warning text is visible without scrolling past the toggles themselves (i.e., appears directly beneath, not in a separate collapsed panel).
- **AC-3.4**: Toggling either switch and having the write fail results in the switch visually reverting to its prior state — never a toggle that appears "on" while the backend still has it "off" (no lying UI).
- **AC-3.5**: Both toggles use `role="switch" aria-checked` (not a checkbox or custom div) — screen reader announces "Close GitHub issues when I finish here, switch, off" / "on" correctly.
- **AC-3.6**: Toggle labels use plain language ("Close GitHub issues when I finish here" / "Reflect GitHub status back here") — no "forward sync"/"backward sync" jargon exposed in the UI copy (research §2).
- **AC-3.7**: Close-label input meets 4.5:1 contrast for its placeholder and entered text; is keyboard-tabbable in document order immediately after its toggle.
- **AC-3.8**: No dead end: every toggle state (on/off, warning shown/hidden) is reversible by the user with no unrecoverable configuration.

---

## Surface 4: Settings > Backlog Sources — Persistent Row-Level Failure Warning

### Wireframe

```
┌───────────────────────────────────────────────────────────────┐
│ acme/widget-app  ⚠ Auth error — reconnect this source     ⏻  │  ← NEW warning inline with
│ github_issues                                                  │     displayName, same row,
│ Last synced: 2026-08-03 09:14 AM                                │     no expand needed
│ [Sync now]  [View history]                                     │
└───────────────────────────────────────────────────────────────┘

Expanded history (user clicks "View history") shows the underlying detail:
│ ┌─ History ──────────────────────────────────────────────────┐│
│ │ 2026-08-03 09:14 — created 0, updated 0, skipped 0,         ││
│ │   errored 3 (401 Unauthorized — token may be revoked)        ││
│ └────────────────────────────────────────────────────────────┘│
```

Transient (rate-limit) failure — no persistent row warning, only visible on expand:

```
┌───────────────────────────────────────────────────────────────┐
│ acme/widget-app                                            ⏻  │  ← no persistent warning;
│ github_issues                                                  │     transient errors only
│ Last synced: 2026-08-03 09:14 AM                                │     show in history log
│ [Sync now]  [View history]                                     │
└───────────────────────────────────────────────────────────────┘
│ ┌─ History (expanded) ──────────────────────────────────────┐│
│ │ 2026-08-03 09:14 — errored 1 (rate limited, retry in 12m)   ││
│ └────────────────────────────────────────────────────────────┘│
```

### Interaction Flow

| Step | User action | System response |
|---|---|---|
| 1 | Source's most recent sync event has an `errorMessage` matching a non-transient pattern (401/403/"revoked") | Row header shows a persistent warning icon + short text next to `displayName`, visible without expanding history |
| 2 | Source's most recent sync event has a transient error (rate limit, timeout) | No persistent row warning — only visible if the user expands "View history" |
| 3 | User clicks the row-level warning (or an adjacent "Fix" affordance, if added) | Recommend: clicking navigates/scrolls to the token-entry field or opens a re-auth flow if one exists — **this exact remediation action isn't specified in Phase 4's plan text**; at minimum, the warning's `title`/tooltip should say "Reconnect this source" so the user knows the next step even if no direct action button exists yet |
| 4 | User re-authenticates (re-enters token) and the next sync succeeds | Persistent warning clears on next successful sync (no error to show) |

### Error / Edge Cases

| Case | Behavior |
|---|---|
| No sync history yet (brand new source) | No warning — absence of history is not treated as an error state |
| Auth error persists across multiple sync attempts | Warning persists (re-derived from most recent event each time, not "sticky" past a real fix) |
| Ambiguous error message (doesn't clearly match 401/403/revoked pattern) | Falls back to transient-only treatment (history-only, no persistent row warning) — false negatives (missing a warning) are preferred over false positives (crying wolf on a transient blip), consistent with "no dead ends" but also "don't alarm-fatigue the user" |

### UX Acceptance Criteria

- **AC-4.1**: User can identify an auth-type sync failure without expanding history — the warning is visible in the collapsed row state.
- **AC-4.2**: Error state shows a specific message (not generic "Error occurred") and the row-level warning's tooltip/title offers a specific next action ("Reconnect this source" or equivalent) — no dead end.
- **AC-4.3**: Transient errors (rate limit) do NOT trigger the persistent row-level warning — only surfaced in the expandable history, to avoid alarm fatigue on self-resolving issues.
- **AC-4.4**: Warning uses the existing warning-weight color token (matching `errorMessage`'s existing visual weight) — no new ad hoc red/orange introduced.
- **AC-4.5**: Warning disappears automatically once a subsequent sync succeeds — never requires a manual "dismiss" that could mask a still-broken source.
- **AC-4.6**: Warning icon/text is screen-reader announced as part of the row's accessible name (e.g. `aria-label` on the row extended to include the warning, or a separate `aria-live="polite"` region) — not conveyed by color alone.

---

## Cross-Surface Accessibility Checklist (WCAG AA / POUR)

| Requirement | Applies to | Verification |
|---|---|---|
| Keyboard reachable, standard activation (Enter/Space) | Badge (S1), issue link (S2), both toggles (S3), close-label input (S3) | Tab through each surface with mouse disconnected; confirm focus visible and order logical |
| Color contrast ≥4.5:1 (text), ≥3:1 (UI components/icons) | Badge, label chips, warning banners, toggle switch states | Check against both light and dark theme token values, not just one |
| No information conveyed by color alone | Row-level warning (S4), both-directions warning (S3) | Icon + text always accompanies color, never a bare colored dot/border |
| `aria-label`/`aria-checked`/`role="switch"` present and correct | All toggles (S3) | Screen reader smoke test (VoiceOver/NVDA) announces state changes |
| Real semantic elements (`<a>`, `<button>`), not click-handler `<div>`s | Badge, issue link, toggles, remove/sync buttons | Code review + axe-core CI gate (already required per this repo's CI: "Axe Core... blocks on WCAG AA violations") |
| Touch targets ≥44×44px / 48×48dp | Badge (S1), toggles (S3), row action buttons (S4) | Manual mobile viewport check (Playwright device emulation or physical device) |
| No dead ends — every error state has a visible, specific next step | S2 (broken link explanation in history), S3 (revertible toggle failure), S4 (reconnect hint) | Walk each error path manually; confirm no state leaves the user without an explanation or an exit |

---

## Summary of Gaps Flagged (not silently assumed covered)

1. **Broken-link visual variant** (muted/struck-through badge/link when GitHub returns 404/410) — recommended by research §4 but not currently specified as a task in Phase 4's plan text (Epics 4.1/4.2). Needs a `externalLinkBroken`-style signal threaded from backward sync's 404 detection before the frontend can render it; flagged as a follow-up story, not fabricated as already planned.
2. **Remediation action for the row-level auth warning** (Surface 4, step 3) — Story 4.3.2 specifies the warning affordance but not a concrete "fix it" click target (e.g., jump to token field, open re-auth flow). Recommend at minimum a descriptive `title`/tooltip until a dedicated action exists.
3. **First-enable-of-backward-sync blast radius** (Surface 3) — explicitly deferred per plan's own Unresolved Question #3; this design only specifies the static warning copy already scoped, not a preview/dry-run.

These three gaps are called out per-surface above and repeated here so they aren't lost in a single subsection; none of them block the acceptance criteria written for what Phase 4 does specify — they are recommendations for what to scope next, not defects in the current plan.
