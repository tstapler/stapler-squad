# UX Design: terminal-multi-connection-streaming

Phase 3 design artifact. Builds directly on `research/ux.md`'s recommendations (comparable
patterns, accessibility conventions, error-state taxonomy) and `implementation/plan.md`'s
Epic 4.2 (Stories 4.2.1–4.2.2). No new UX exploration happens here — this document turns the
already-decided direction into wireframes, interaction flows, and testable acceptance criteria.

## Surface inventory

Per `research/ux.md`'s explicit scope note, this backend/architecture redesign has exactly
three user-facing surfaces, none of which is a new screen or modal:

| # | Surface | Type | Treatment below |
|---|---|---|---|
| 1 | Connection-count indicator (+ tooltip/expanded state) | Interactive (hover/tap, screen-reader-announced) | Full wireframe + flow + AC |
| 2 | Hub-can't-start error banner | Non-interactive status display (reuses existing `hardFailedBanner`) | Condensed entry |
| 3 | Feature-flag / path state in logs (`PathLegacyPerConnection` vs `PathHubOwned`) | Non-interactive (operator-facing log/observability output, not UI) | Condensed entry |

Surface 3 is included because requirements.md's Observability Requirements call for
feature-flag state to be "visible in logs per session" — it's a legitimate user-facing (operator-
facing) surface even though it never renders in the browser. It gets the condensed treatment
per this task's instructions for non-interactive surfaces.

---

## Surface 1: Connection-count indicator (full treatment)

### Wireframe — default state (single connection, indicator hidden)

```
┌─ Terminal chrome ────────────────────────────────────────────────┐
│ [toolbar icons...]                                    [🔧][⛶][x] │
├────────────────────────────────────────────────────────────────┤
│                                                                    │
│   $ claude                                                        │
│   ...agent output...                                              │
│                                                                    │
└────────────────────────────────────────────────────────────────┘
```
No indicator renders. `connection_count <= 1` (or absent, for `PathLegacyPerConnection`
sessions) → nothing mounts. This is the overwhelmingly common case (single operator, single tab)
and must add zero visual noise to it.

### Wireframe — two connections attached, collapsed

```
┌─ Terminal chrome ────────────────────────────────────────────────┐
│ [toolbar icons...]                          [👥 2 ●] [🔧][⛶][x] │
├────────────────────────────────────────────────────────────────┤
│   $ claude                                                        │
│   ...agent output...                                              │
└────────────────────────────────────────────────────────────────┘
```
`[👥 2 ●]` is `ConnectionCountIndicator` — icon `aria-hidden="true"` (per
`DeepLinkErrorBanner.tsx:127-129`'s icon convention), visible text "2", `role="status"`
`aria-live="polite"`, `aria-label="2 connections active"`. Neutral color (not red/amber) —
this is not an error state.

### Wireframe — hover/tap expanded (tooltip), resize-mismatch case

```
┌─ Terminal chrome ────────────────────────────────────────────────┐
│ [toolbar icons...]                          [👥 2 ●] [🔧][⛶][x] │
│                                              ┌──────────────────┐ │
│                                              │ 2 connections    │ │
│                                              │ active           │ │
│                                              │                  │ │
│                                              │ Another          │ │
│                                              │ connection has   │ │
│                                              │ this session     │ │
│                                              │ open at a        │ │
│                                              │ different size.  │ │
│                                              └──────────────────┘ │
├────────────────────────────────────────────────────────────────┤
│   $ claude                                                        │
│   ...agent output...                                              │
└────────────────────────────────────────────────────────────────┘
```
Second paragraph appears only when this tab's `ResizeVote` lost the `NegotiatedSize`
negotiation. When both/all tabs got their requested size, the tooltip shows just the first
line ("2 connections active") — no fabricated explanation when nothing is actually mismatched.

### Interaction flow

| Step | User action | System response |
|---|---|---|
| 1 | Operator opens a second tab (or ssq-mux attaches) on a session already open in tab A | Hub's `SubscriberCount()` goes from 1→2. Both tabs receive `connection_count: 2` on their next stream message. |
| 2 | Tab A's `connection_count` transitions 1→2 | `ConnectionCountIndicator` mounts, screen reader announces "2 connections active" exactly once (change-triggered, not mount-triggered — see AC below). |
| 3 | Operator hovers/taps the indicator in tab A | Tooltip expands showing count + (conditionally) the resize-mismatch sentence. No new `aria-live` announcement fires — the tooltip's content is available to assistive tech via the already-associated `aria-describedby`/on-focus reveal, not a second live region. |
| 4 | Operator closes tab B | Hub's `SubscriberCount()` goes 2→1. Tab A receives `connection_count: 1`. |
| 5 | Tab A's `connection_count` transitions 2→1 | Indicator unmounts. Screen reader announces the change once ("1 connection active" or equivalent unmount announcement — implementer's choice, but must not go silent without any signal, since a screen-reader user was told about the second connection and deserves to be told it's gone). |
| 6 | Operator resizes tab A's window while tab B is still attached | If tab A's resize vote loses the negotiation, the pane redraws cleanly at the negotiated (possibly smaller) size — **not garbled** — and the tooltip's mismatch sentence becomes available on next hover. No new unprompted announcement (per `research/ux.md` §4b, this is folded into the existing indicator, not a second live region). |

### Error and edge-case handling

| Case | What the user sees | Why |
|---|---|---|
| `connection_count` absent (`PathLegacyPerConnection`, flag off) | Nothing — indicator does not render | Avoids fabricating a count from the `activeControlModeStreams` generation counter, which isn't a live count (requirements.md, Story 4.2.1 AC2) |
| `connection_count` drops to 0 transiently (e.g. mid-reconnect race) | Indicator does not render (same as `<= 1`) | 0 is not a meaningful state to announce to the remaining viewer; avoid flicker |
| Rapid count oscillation (e.g. flaky reconnect flapping 1↔2 repeatedly) | Indicator implementation should debounce announcements the same way `InputDropBadge.tsx`'s episode-coalescing pattern does, so a screen-reader user doesn't get a burst of "2 connections active / 1 connection active / 2 connections active" | `research/ux.md` §3 names this precedent explicitly |
| Tooltip requested but `NegotiatedSize` data hasn't arrived yet | Tooltip shows just "N connections active" (first line only) — never a blank or loading tooltip | No dead-end/empty state |

### UX acceptance criteria (surface 1)

1. With `connection_count <= 1` or absent, `ConnectionCountIndicator` does not render — zero added visual surface for the single-tab case (mirrors plan.md Story 4.2.1 AC2 and Story 4.2.2 AC1).
2. Operator can determine "another connection is attached" in **0 extra steps** — the indicator is always visible near the terminal chrome once `connection_count > 1`, requiring no navigation or menu.
3. Operator can see the resize-mismatch explanation in **1 step** (hover or tap the indicator).
4. The indicator uses `role="status"` + `aria-live="polite"` and never `role="alert"` — verified by DOM inspection / component test, matching `TerminalOutput.tsx:1789-1791`'s reconnecting-banner convention exactly.
5. Screen reader announces a connection-count change exactly once per change, and does not announce on initial mount if the count was already `>1` at mount time (changes-only, per plan.md Story 4.2.2 AC1's `Given`/`When`/`Then`).
6. The icon glyph (if used) is `aria-hidden="true"`; the announced/visible text carries all meaning (e.g. "2 connections active"), per `DeepLinkErrorBanner.tsx:127-129`'s precedent.
7. Color contrast of the indicator's text and icon against its background is ≥ 4.5:1 in both light and dark themes (WCAG AA, normal text).
8. The indicator is keyboard-navigable: reachable via Tab in the terminal chrome's existing tab order, and its tooltip is revealable via keyboard focus (not hover-only) — a mouse-only interaction would fail keyboard accessibility.
9. No dead ends: there is no state where the indicator is visible but unexplainable — every visible state (bare count, count + mismatch tooltip) has a plain-language, non-technical explanation, never raw internals like "hub" or "transport."
10. The resize-mismatch sentence in the tooltip appears **only** when this tab's vote actually lost the negotiation — never shown speculatively or when sizes matched, preventing a false "something's wrong" signal (research/ux.md §4b: don't announce a non-event).
11. A rapid sequence of count changes (flapping) is coalesced/debounced so a screen-reader user receives at most one announcement per user-perceptible state, not one per underlying network message.

---

## Surface 2: Hub-can't-start error banner (condensed)

Reuses the existing `hardFailedBanner` / `role="alert"` pattern verbatim
(`TerminalOutput.tsx:1797-1799`) — no new banner chrome, no new copy beyond what already ships:

```tsx
<div className={styles.hardFailedBanner} role="alert">
  Connection lost — <button onClick={handleHookReconnect}>Retry</button>
</div>
```

This fires identically whether the underlying cause is today's control-mode failure or a
post-redesign hub-start failure — the user-facing contract does not change, only the internal
trigger condition (`isHardFailed` becomes true on hub-start failure in addition to its existing
triggers).

**Acceptance criteria:**
- Given the hub fails to start for a `PathHubOwned` session, when the frontend's connection
  attempt exhausts retries, then the existing `hardFailedBanner` renders with `role="alert"` and
  the same "Connection lost — Retry" copy already used today (no new/different message).
- The Retry button re-triggers the same reconnect path as today's hard-failure case — no new
  dead end introduced by the hub redesign.
- No new copy needs translation/localization review — it's the existing string, unchanged.
- Component tests already covering `hardFailedBanner`'s render conditions are extended (not
  replaced) to include a hub-start-failure trigger, per plan.md's testability NFR.

---

## Surface 3: Feature-flag / path state in logs (condensed)

Not a UI element — an operator-facing structured log line, per requirements.md's Observability
Requirements ("Feature-flag state... must be visible in logs per session").

Representative sample (structured log, one line per session-stream start):

```json
{"level":"info","msg":"terminal stream started","session_id":"abc123","stream_path":"PathHubOwned","subscriber_count":2,"time":"2026-08-20T10:15:00Z"}
```

**Acceptance criteria:**
- Every session-stream start logs `stream_path` as one of the two `StreamPath` enum values
  (`PathLegacyPerConnection` or `PathHubOwned`), never omitted, so a dark-launch rollout is
  auditable from logs alone without code inspection.
- `subscriber_count` at hub creation and at each attach/detach is present in the corresponding
  log line, matching requirements.md's "structured logging at hub creation, subscriber
  attach/detach... events" requirement.
- The pre-existing `420584566` overlap-detection WARN either stops firing entirely (because the
  race is structurally impossible under `PathHubOwned`) or is converted into a hard invariant
  check per requirements.md — this is an architecture decision tracked in `implementation/plan.md`,
  not a UX decision, but the *visibility* requirement (operator can tell which happened) is in
  scope here: a log line must state which of the two outcomes applies for a given session.
- No PII or session content appears in these log lines — session identifiers and counts only.

---

## Summary

3 surfaces designed (1 full interactive treatment, 2 condensed). 11 UX acceptance criteria
written for the interactive surface (connection-count indicator), plus 4 for the error banner
and 4 for the log-visibility surface — 19 acceptance criteria total across all three surfaces.
