# UX Design: Async Session Creation (Creating → Running/Failed/Cancelled)

Design artifact for the plan in `project_plans/async-session-creation/implementation/plan.md`,
grounded in `research/ux.md` and `requirements.md`. Companion backend design docs cover the RPCs;
this document is the user-facing contract for those RPCs.

**New surfaces this feature touches**: Omnibar submit flow, SessionCard's Creating state (already
exists, extended), SessionCard's new Failed state, Cancel button (Creating), Retry button (Failed),
failure toast, and the (invisible to the user, but UX-relevant) stale-creation auto-transition.

---

## Surface Index

| # | Surface | Interactive? | Treatment |
|---|---------|-------------|-----------|
| 1 | Omnibar create-session submit | Yes | Full |
| 2 | SessionCard: Creating state | Partial (existing, minor extension) | Condensed |
| 3 | SessionCard: Failed state + Cancel/Retry buttons | Yes | Full |
| 4 | Failure toast | No (transient, no required action) | Condensed |
| 5 | Stale-creation auto-transition | No (system-driven, no user input) | Condensed |

---

## 1. Omnibar create-session submit (Full treatment)

### Wireframe / flow

```
┌─────────────────────────────────────────────────────────┐
│  Omnibar                                            [×]  │
│  ┌─────────────────────────────────────────────────────┐│
│  │ https://github.netflix.net/corp/repo/pull/123        ││
│  └─────────────────────────────────────────────────────┘│
│                                    [ Cancel ]  [ Create ]│
└─────────────────────────────────────────────────────────┘

  user clicks Create
        │
        ▼
┌─────────────────────────────────────────────────────────┐
│  Omnibar                                            [×]  │
│  ┌─────────────────────────────────────────────────────┐│
│  │ https://github.netflix.net/corp/repo/pull/123        ││
│  └─────────────────────────────────────────────────────┘│
│                          [ Cancel ]  [ Creating... ⏳ ]  │  <- button disabled, ~<500ms
└─────────────────────────────────────────────────────────┘
        │  RPC returns (fast: instance created, resolution not done)
        ▼
   (dialog closes — user is back on session list)
```

### Interaction flow

| Step | User action | System response |
|---|---|---|
| 1 | Types/pastes input, clicks Create | Create button disables immediately (idempotent-submit guard — prevents double-submit per UX research §4) |
| 2 | (no further action) | RPC returns in ≤500ms p99 with the new placeholder instance in `SESSION_STATUS_CREATING` |
| 3 | (no further action) | Omnibar closes automatically; user sees the session list with the new card already present, spinner active |
| 4a — success | (none) | Card silently transitions Creating → Active; no dialog reopens |
| 4b — synchronous fast-fail (duplicate title, missing path, invalid alias, resume_id format, fork source not found) | (none) | Omnibar does **not** close; inline validation error shown in the dialog exactly as today — no instance was created |
| 4c — background failure | (none) | Handled entirely off-dialog: toast (Surface 4) + card Failed state (Surface 3) |

### Error / edge-case handling

| Case | What the user sees |
|---|---|
| Fast-fail validation (still synchronous per Constraints) | Inline error text in the omnibar itself, dialog stays open, no card created — unchanged from today |
| Double-click Create | Second click is a no-op; only one card appears |
| Slow GHE clone (the motivating case) | Dialog closes within ~500ms regardless; user is never blocked waiting for the clone |

### UX acceptance criteria

- User can create any session type in exactly 1 click (paste/type + Create) with no additional step introduced by this feature.
- Dialog closes within 500ms of clicking Create for every session type, including slow GitHub-URL sessions — measured against the RPC's p99 SLO from requirements.md.
- Create button is disabled the instant it's clicked and stays disabled until the RPC responds, preventing duplicate submissions (no visible double-card regression).
- A synchronous fast-fail error (duplicate title, bad path, bad alias, malformed resume_id, missing fork source) keeps the dialog open with the existing inline error UI — zero behavior change here is itself a pass/fail criterion (regression check).
- No dead end: if the omnibar closes and the background pipeline later fails, the user is not required to have kept the dialog open to learn about it — the toast + card Failed state (Surfaces 3–4) are the exit path.

---

## 2. SessionCard: Creating state (Condensed — existing surface, minor extension only)

Already implemented (`SessionCard.tsx:235,951-961`, `SessionCard.css.ts:929-944`) — spinner, status
pill ("Starting…"), progress text, always-mounted `role="status" aria-live="polite"` region,
reduced-motion-safe animation. This feature's only change here is adding the Cancel button into the
same progress row (see Surface 3's button placement, which applies to both Creating and Failed).

Representative render:

```
┌───────────────────────────────────────────────────┐
│ ● my-feature-branch          [Starting…]           │
│   ⟳ Cloning repository...          [Cancel ✕]      │
└───────────────────────────────────────────────────┘
```

Acceptance criteria:
- Card appears in the list within ~1s of the user hitting Create (requirements.md success metric), for every session type.
- Progress text updates in place (no row re-mount/re-layout jump) as `creation_progress` advances through phases.
- Cancel button is present and clickable throughout Creating, not just after a delay.
- Existing reduced-motion and live-region behavior is unchanged (regression check only — no new gap introduced by adding the Cancel button to this row).

---

## 3. SessionCard: Failed state + Cancel/Retry buttons (Full treatment)

### Wireframe

**Creating → Cancel:**
```
┌───────────────────────────────────────────────────┐
│ ● my-feature-branch          [Starting…]           │
│   ⟳ Cloning repository...          [Cancel ✕]      │
└───────────────────────────────────────────────────┘
        │ user clicks Cancel
        ▼
   card removed from list (instance deleted)
   — OR, if cancel lost the race to success —
┌───────────────────────────────────────────────────┐
│ ● my-feature-branch          [Running]             │
└───────────────────────────────────────────────────┘
```

**Creating → Failed → Retry:**
```
┌───────────────────────────────────────────────────┐
│ ▲ my-feature-branch          [Failed]              │
│   ⚠ Failed to resolve GitHub URL: connection        │
│      timed out                    [Retry ↻]        │
└───────────────────────────────────────────────────┘
        │ user clicks Retry
        ▼
┌───────────────────────────────────────────────────┐
│ ● my-feature-branch          [Starting…]           │  <- same card, same position
│   ⟳ Resolving GitHub URL...        [Cancel ✕]      │
└───────────────────────────────────────────────────┘
```

**Stale/orphaned failure (system-detected, no clone error at all):**
```
┌───────────────────────────────────────────────────┐
│ ▲ my-feature-branch          [Failed]              │
│   ⚠ This session creation appears to have           │
│      stalled.                     [Retry ↻]        │
└───────────────────────────────────────────────────┘
```

### Interaction flow

| Step | User action | System response |
|---|---|---|
| 1 | Card is `Creating`, user clicks `Cancel session creation` | RPC `CancelSessionCreation` fires; button shows a brief pending state (no separate confirmation dialog — single-click per plan's default) |
| 2a | (cancel wins the race) | Card disappears from the list — instance and any partial clone/worktree removed |
| 2b | (cancel loses the race — pipeline already succeeded) | Card updates to `Running`/`Active` via the normal stream event; no flash of "Cancelled" |
| 3 | Card is `Failed`, user clicks `Retry creating session` | Button disables immediately (idempotent guard); RPC `RetrySessionCreation` fires |
| 4 | (retry accepted) | Same card, same position, same ID transitions `Failed → Creating` with fresh progress text |
| 5 | (retry itself fails again) | Card returns to `Failed` with a (possibly same) failure reason; user can retry again indefinitely |

### Error / edge-case handling — failure reason table

| `FailureReason` | Card message shown | Toast copy | User's exit path |
|---|---|---|---|
| `GitHubResolutionError` | "Failed to resolve GitHub URL: `<short error>`" | "Session '`<title>`' failed: couldn't resolve the GitHub URL." | Retry (if transient, e.g. network blip) or Cancel-equivalent (delete) if the URL itself was wrong — user must go fix the URL via a new creation, since Retry re-runs with the *same* stored input |
| `StartupError` | "Failed to start session: `<short error>`" | "Session '`<title>`' failed to start." | Retry |
| `Stale` | "This session creation appears to have stalled." (explicitly not phrased as a user error, per UX research §2/§4) | "Session '`<title>`' timed out during creation." | Retry |
| (Cancelled — not a `Failed` card at all) | N/A — instance is removed, not shown as Failed | No toast (user-initiated, already knows) | N/A — card simply disappears |

Additional edge cases:
- **Cancel racing success**: resolved server-side (plan Task 3.2.1c) before any UI decision is needed — the client only ever sees one deterministic outcome (`Active` or removed), never both.
- **Retry racing itself (double-click)**: client-side disables the button on first click; this is the only guard the UI needs — server-side dedup (plan Task 3.3.1d) is a backstop, not something the UI must also detect.
- **Repeated retry into the same failure**: not blocking for this phase (per requirements.md scope), but the card's message should not silently repeat the exact same generic text every time in a way that looks frozen — re-showing a fresh timestamp-free identical message is acceptable for v1; an escalation hint after N retries is a nice-to-have, not required.
- **No dead end**: every Failed card has exactly one action (Retry) plus the always-available session-delete action already on every card — a user who wants to give up entirely is not stuck with a Failed card they can't remove.

### UX acceptance criteria

- User can cancel a stuck Creating session in 1 click, with no confirmation dialog (per plan's default decision), and the card is gone (or correctly shows Running if the race was lost) within one stream-event round trip.
- User can retry a Failed session in 1 click, and the same card (verified by DOM node identity / same `data-testid` key, not just "a card with the same title") transitions Failed → Creating in place — never a second card.
- Failed state shows one of the three specific messages above (not a generic "Something went wrong") and always offers the Retry action alongside it.
- No dead ends: every Failed card has a working Retry button and remains deletable via the existing per-card delete action.
- Cancel and Retry are real `<button>` elements (not clickable `<div>`s), reachable via standard Tab order, each with an explicit `aria-label` (`"Cancel session creation"`, `"Retry creating session"`) distinct from the bare visible label — verified by axe-core / Playwright role query, not just visual inspection.
- Cancel/Retry buttons have visible focus rings matching the existing snapshot-toggle button precedent (`SessionCard.tsx:966-977`).
- Color contrast of the Failed status pill and its icon against both light and dark card backgrounds is ≥4.5:1 (WCAG AA, normal text) — verified with a contrast-checker tool against the actual `statusCreationFailed` token values, not eyeballed.
- Failed state is never visually or programmatically identical to `Crashed` or `Cancelled` — distinct icon, distinct color token (`statusCreationFailed`, not `statusCrashed`), distinct ARIA text.
- **Live region contract (explicitly verified, not assumed)**: the existing `role="status"` span at `SessionCard.tsx:951-954` is the *same* DOM node used for the Failed announcement — no second `aria-live` region is introduced anywhere in the Failed-state implementation. Verify by asserting in a test that exactly one element with `role="status"` exists on the card across the full Creating→Failed transition, and that its `aria-live` attribute value changes from `"polite"` to `"assertive"` via attribute mutation (not remount) at the moment of failure.
- Reduced-motion users get a static warning glyph for Failed (no animation), matching the existing static-ring fallback already used for Creating's spinner — verified under `prefers-reduced-motion: reduce`.
- Focus is never stolen from the user's current context (e.g. mid-typing in the omnibar for a different session) when a card they're not actively interacting with transitions to Failed — announcement only, no focus jump.

---

## 4. Failure toast (Condensed)

Routed through existing `NotificationToast`/`NotificationContext`/`notification-policy.ts` — a new
`NotificationData` variant, not a bespoke component.

Representative sample (visual shape matches existing toasts in the app):

```
┌──────────────────────────────────────────────┐
│ ⚠ Session "my-feature-branch" failed:         │
│   couldn't resolve the GitHub URL.       [×]  │
└──────────────────────────────────────────────┘
```

Acceptance criteria:
- Fires exactly once at the moment a session's status transitions to `FAILED`, regardless of which page/session the user is currently viewing.
- Uses failure-reason-specific copy (one of the three variants in Surface 3's table), never a generic "creation failed."
- Auto-close timing for this notification type is confirmed to be as long as or longer than routine toasts (higher stakes than a routine status update per UX research §4) — verified against the actual configured `toastAutoCloseMs` value for this variant, not assumed.
- Toast dismissing (by timeout or user click) never removes the corresponding information from the session card — the card's Failed state (Surface 3) is the durable record.
- Toast is dismissible with a visible, keyboard-reachable close control, consistent with every other toast in the app (no new interaction pattern introduced).

---

## 5. Stale-creation auto-transition (Condensed)

Not a UI a user clicks — a system-driven transition the user only observes as a side effect on the
card (Surface 3, `Stale` row) and toast (Surface 4, `Stale` row). Listed here for completeness of
the failure-reason taxonomy, not as its own interactive surface.

Representative sample: a session left in `Creating` for >10 minutes (default,
config-overridable) is flipped server-side to `Failed`/`Stale`; the client observes this exactly
like any other `Failed` transition via the `WatchSessions` stream — no separate client-side logic
needed to detect staleness.

Acceptance criteria:
- The stale-flip message is textually distinct from a resolution/startup error (already covered in Surface 3's table) so the user doesn't read a systemic timeout as something they personally misconfigured.
- No client-side timer/polling is needed to produce this state — it arrives as an ordinary status-changed stream event, so the card/toast code paths for `Stale` are identical to any other `FailureReason`, not a special case.
- A session that goes stale while the user's tab is closed still shows as Failed/Stale (with the persistent card) the next time they open the app — verified by relying on server-persisted status, not any client-side session/local state.

---

## Summary of Cross-Cutting Accessibility Verification (applies to Surfaces 2–4)

1. **One live region, not two.** `SessionCard.tsx:951-954`'s span is reused for both Creating-progress (`polite`) and Failed (`assertive`) — confirmed via the acceptance criterion in Surface 3 above. This directly follows the plan's explicit design decision (`Pattern Decisions` table, "Failed-state visual token" row and Epic 5.2.2) and the research's NVDA rationale (`research/ux.md` §0, §3).
2. **Icon + color, never color alone** (WCAG 1.4.1) for Failed vs. Crashed vs. Paused/Stopped.
3. **Real `<button>` elements with explicit `aria-label`s** for Cancel and Retry — not ambiguous bare text, not non-semantic clickable elements.
4. **Keyboard reachability and visible focus** for both new buttons, matching the existing snapshot-toggle precedent.
5. **Reduced-motion parity** for any new Failed-state visual (static icon, no unguarded animation).
6. **No focus theft** on background state transitions the user isn't actively watching.

---

## Sources

- `project_plans/async-session-creation/requirements.md` (success metrics, scope, baseline user quote).
- `project_plans/async-session-creation/research/ux.md` (comparable patterns, mental models, accessibility requirements, jobs-to-be-done, SessionCard.tsx baseline).
- `project_plans/async-session-creation/implementation/plan.md` (Epics 5.1–5.4, `FailureReason` taxonomy, Domain Glossary, Pattern Decisions table — `statusCreationFailed` token, live-region reuse decision, cancel/retry RPC semantics and race resolution in Epics 3.2–3.3).
