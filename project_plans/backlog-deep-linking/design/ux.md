# UX Design: backlog-deep-linking

SDD Phase 3. Inputs: `requirements.md`, `research/ux.md` (Phase 2), `implementation/plan.md`
(Story 2.3, Epic 5/Story 5.1). This doc turns the Phase 2 research recommendations into
concrete wireframes, interaction flows, and testable acceptance criteria for implementation.

## Step 1: Surface Inventory

| # | Surface | Interactive? | Plan.md story |
|---|---|---|---|
| 1 | Copy Link / Copy ID buttons (`BacklogItemDetail.tsx`) | Yes | 2.3 |
| 2 | Same-host `ssq://`/`https://` link resolution | Yes | 2.1, 2.2 |
| 3 | Cross-host link resolution (live handoff) | Yes | 2.2, 3.3 |
| 4 | Error state: item deleted/archived | Yes | 5.1 |
| 5 | Error state: host unreachable (registered, offline) | Yes | 5.1 |
| 6 | Error state: host not registered | Yes | 5.1 |
| 7 | Error state: malformed link | Yes | 5.1 |
| 8 | Error state: version-mismatch link | Yes | 5.1 |
| 9 | `--open-url` CLI subcommand | No (CLI/OS-invoked) | 4.1 |
| 10 | Linux `.desktop` scheme registration | No (install-time) | 4.2 |
| 11 | Server-side deep-link resolver route | No (API) | 2.2 |
| 12 | Structured logging for link generation/resolution | No (ops-facing) | Observability Plan |

Surfaces 1–8 (a human clicks or reads them in the web UI) get full wireframe + flow + error
treatment in Step 2/3 below. Surfaces 9–12 get condensed treatment.

---

## Step 2 & 3 — Interactive Surfaces

### Surface 1: Copy Link / Copy ID buttons

This is an in-place upgrade of existing UI (`BacklogItemDetail.tsx:1257-1277`), not new
construction — per `research/ux.md` §0 and plan.md's "Copy-link UI" pattern decision, GitHub's
inline-icon-swap pattern is already implemented and validated; the only changes are (a) the
URL value built at line 1271, and (b) making the copy confirmation audible to screen readers.

**Wireframe (sticky header, unchanged layout):**

```
┌─────────────────────────────────────────────────────────────────┐
│ [P1] Fix flaky test in scanbuf          Created 2d ago · Updated 3h ago │
│                                                                   │
│ bl_01J9X7QK3M8N2P4R6S8T0V2W4Y  [Copy ID]  [Copy Link]           │
│ └──────────── id text ────────┘  └──────┘  └───────┘            │
│                (visually hidden aria-live region, see below)     │
├─────────────────────────────────────────────────────────────────┤
│  [●Connected]                              [Edit]  [×]          │
└─────────────────────────────────────────────────────────────────┘
```

Post-click state (either button, ~1.5s window):

```
bl_01J9X7QK3M8N2P4R6S8T0V2W4Y  [✓ Copied]  [Copy Link]
```

**Interaction flow:**

1. User clicks **Copy Link** (mouse) or tabs to it and presses `Enter`/`Space` (keyboard).
2. Synchronously (no network round-trip, no spinner): the handler builds
   `ssq://<this-host>/backlog/v1/<public_id>` — falling back to the item's legacy UUID `id`
   if `public_id` is empty (pre-backfill row) — and copies it via the existing
   `copyToClipboard()` helper.
3. Button label flips to `✓ Copied`; a visually-hidden `aria-live="polite"` region (or a
   dynamic `aria-label`, see accessibility below) announces "Link copied to clipboard."
4. After 1.5s (`copyTimerRef`), label reverts to `Copy Link`. Re-clicking mid-window resets
   the timer rather than double-firing (existing guard, unchanged).
5. **Copy ID** follows the identical flow, copying the bare `bl_01J...` (or legacy UUID)
   string, not a URL.

**Edge cases:**

- **Clipboard write fails** (both `navigator.clipboard.writeText` and the `execCommand`
  fallback fail — rare, locked-down browser configs): `handleCopy`'s existing
  `if (!ok || !mountedRef.current) return;` guard no-ops today, meaning the button silently
  doesn't change and no error is announced. **This is a design gap carried forward from
  research** — the button must not claim success it didn't achieve, but a silent no-op is
  indistinguishable from "I forgot to click." Fix: on failure, flip the label to `Copy
  failed` for 1.5s (reusing the same timer/state machinery, no new component) and log a
  console warning; do not show a blocking error banner for a low-stakes clipboard failure.
- **Item has no `public_id` yet** (pre-backfill legacy row): Copy Link must still produce a
  working link — build it from the legacy UUID (`ssq://<host>/backlog/v1/<uuid>`), since the
  resolver's dual-ID dispatcher (Story 1.3) accepts both forms. Never block or disable the
  button waiting on backfill.
- **Rapid double-click** on either button: idempotent — same value copied both times, one
  confirmation cycle, not two.

**UX acceptance criteria:**

- AC1: Clicking **Copy Link** places `ssq://<hostname>/backlog/v1/<id>` on the clipboard in
  exactly 1 click, with no intermediate dialog, spinner, or page navigation.
- AC2: The copied value is directly pasteable into a browser address bar or Slack message
  with no user edits required (well-formed scheme, no encoding artifacts).
- AC3: Button label reads `Copy Link` at rest, `✓ Copied` for 1.5s post-click, then reverts —
  identical timing/behavior to the existing `Copy ID` button.
- AC4: On clipboard-write failure, the button shows a distinct `Copy failed` state (not a
  false `✓ Copied`) for 1.5s, and a warning is logged to the console.
- AC5: **Keyboard**: both buttons are reachable via standard `Tab` order and activatable via
  `Enter` or `Space` — no custom `tabindex`, no keyboard trap. Focus remains on the button
  after activation (no focus loss).
- AC6: **Screen reader**: activating Copy Link via keyboard triggers an audible "Link copied
  to clipboard" announcement within the same interaction — implemented via a visually-hidden
  `aria-live="polite"` region (`<span className="sr-only" aria-live="polite">{copiedField === "link" ? "Link copied to clipboard" : ""}</span>`) co-located with the button, since a
  static `aria-label` does not re-announce on inner-text-only changes.
- AC7: **Contrast**: button text/icon against its background meets WCAG AA 4.5:1 in both
  light and dark theme (verify against this repo's existing design tokens per
  `.claude/docs/css-architecture.md` — no new colors introduced).
- AC8: A legacy item with no `public_id` still produces a working, correctly-formed link
  (UUID form) — verified by a test case, not just visual inspection.

---

### Surface 2: Same-host link resolution

**Flow:**

```
User clicks ssq://myhost/backlog/v1/bl_01J... (Slack, terminal, address bar)
        │
        ▼
OS scheme handler (Epic 4) or same-tab https:// navigation
        │
        ▼
┌───────────────────────────────┐
│ Web app already open on       │   Web app not open
│ myhost?                        │───────────────┐
└───────────────────────────────┘                │
        │ yes                                     │ no
        ▼                                          ▼
Route parses URL client-side,           Browser/OS opens myhost:8543,
calls GET /api/deep-link/resolve         app boots, then same resolve
        │                                          │
        ▼◄─────────────────────────────────────────┘
Item exists locally?
   │ yes                              │ no (see Surfaces 4–8)
   ▼
Backlog detail panel opens/scrolls
to the item, focus moves to the
panel's heading — no interstitial,
no confirmation click
```

**Interaction flow (mirrors the existing `?item=<uuid>` behavior the requirements doc
requires this to match exactly):**

1. Link opened. If the web app tab is already open, the router intercepts client-side; if
   not, the OS opener launches/focuses the browser at the resolved local URL first.
2. The resolver (`GET /api/deep-link/resolve`) runs. Because this is same-host, no registry
   lookup or network hop is needed — result returns effectively immediately (no perceptible
   loading state should be shown for the common case; if a real render delay is unavoidable,
   a skeleton state of the detail panel itself is acceptable, but never a separate
   "resolving..." interstitial screen per `research/ux.md` §2).
3. On success, the backlog board/list view (whatever is already the entry route) opens the
   item's detail panel directly, matching today's `?item=<uuid>` deep-link behavior exactly.
4. Focus moves to the detail panel's heading (`item.title`) so keyboard/screen-reader users
   land in the right place without needing to tab past the whole board.

**Edge cases:** covered by Surfaces 4–8 (every non-success outcome).

**UX acceptance criteria:**

- AC1: Opening a same-host link when the app is already open in a tab navigates to the item
  with **zero additional clicks** — no confirmation, no interstitial page.
- AC2: Opening a same-host link when the app is not running launches it and lands on the
  item in one continuous flow — the user does not have to re-click the link or re-navigate
  after the app finishes booting.
- AC3: Focus lands on the item detail panel's heading after resolution, verified via a
  screen-reader/keyboard test (not just visual placement).
- AC4: An old-format `?item=<uuid>` link opened after this feature ships behaves identically
  to before — no banner, no delay difference, no "legacy" labeling (permanence promise,
  `research/ux.md` §2/§4).
- AC5: No loading spinner/interstitial is shown for the same-host path under normal
  (sub-200ms local resolve) conditions.

---

### Surface 3: Cross-host resolution (live handoff)

**Wireframe — successful handoff banner (shown briefly before/during redirect):**

```
┌───────────────────────────────────────────────────────────────┐
│ ℹ  This item lives on "otherhost" — opening it there…          │
│    [Open on otherhost now →]                                    │
└───────────────────────────────────────────────────────────────┘
```

**Flow:**

```
Link hostname ("otherhost") != this instance's HostIdentity
        │
        ▼
Resolver queries Workspace Host Registry for "otherhost"
        │
        ├─ known + live (liveness check passes) ──► auto-redirect to
        │                                            AdvertisedAddress,
        │                                            banner shown briefly
        │                                            as a transition cue
        │
        ├─ known but liveness check times out ────► Surface 5 (unreachable)
        │
        └─ not in registry at all ────────────────► Surface 6 (not registered)
```

**Interaction flow:**

1. Resolver returns a resolved `AdvertisedAddress` for a live peer.
2. Client shows the transition banner (`role="status"`, polite) naming the target host, then
   automatically navigates (e.g. `window.location.href = advertisedAddress + ...`) after a
   brief, perceivable moment — not instant, so the user isn't confused by an unannounced tab
   navigation, but fast enough it doesn't feel like a dead stop. A manual "Open on X now"
   link is always present so the user isn't forced to wait even that briefly.
3. If the auto-navigation is blocked (e.g. popup/cross-origin restrictions in some browser
   contexts), the manual link is the fallback path — never a dead end.

**Edge cases:**

- Auto-redirect fails silently in some browser security contexts (e.g. blocked programmatic
  navigation) → the manual button must always be present and focused by default so `Enter`
  immediately completes the handoff.
- User has multiple tabs/instances open across hosts → this is out of scope to detect;
  handoff always targets exactly the one `AdvertisedAddress` the registry returned.

**UX acceptance criteria:**

- AC1: A successful cross-host resolution never requires more than **1 click** (the manual
  "Open on X now" link) to complete, even if auto-redirect is blocked.
- AC2: The banner names the actual target host by hostname, never a bare "elsewhere" or
  "another instance."
- AC3: Banner uses `role="status"` (polite), not `role="alert"` — per `research/ux.md` §3,
  this is a normal navigational event, not a failure.
- AC4: The manual link's accessible name includes the target host
  (`aria-label="Open this item on otherhost"`), not just visible adjacent text.
- AC5: Keyboard-only: the manual link is reachable via `Tab` and activatable via `Enter`
  without needing to interact with anything else first.

---

### Surfaces 4–8: Resolver failure states

All five share one component pattern per plan.md Story 5.1: a `DeepLinkErrorBanner`
extending the existing `InlineError.tsx` / `TriageErrorBanner.tsx` conventions already used
elsewhere in this codebase (block-style container, icon + headline + body + action button(s),
`role="status"` for informational states vs `role="alert"` only where a true failure — see
per-case table). No new banner shape is invented; this reuses `InlineError`'s `blockContainer`
variant with a new `type` union member set (or a sibling component with matching CSS classes,
implementer's choice at build time) so it inherits contrast-checked, theme-aware styling for
free.

**Shared wireframe shape:**

```
┌───────────────────────────────────────────────────────────────┐
│ ✕/ℹ  <Headline>                                                 │
│      <Body text — specific, names the host/reason>              │
│      [Primary action]   [Secondary action]                      │
└───────────────────────────────────────────────────────────────┘
```

| # | Case | `role` | Headline | Body | Primary action | Secondary action |
|---|---|---|---|---|---|---|
| 4 | Item deleted/archived | `alert` | "This backlog item no longer exists" (or "…has been archived" if soft-deleted/restorable — resolver's error payload distinguishes the two) | "It may have been completed, archived, or deleted since this link was shared." | "Go to backlog board" (navigates to the unfiltered board) | none |
| 5 | Host unreachable (registered, offline) | `status` | `This item lives on "otherhost"` | `otherhost isn't reachable right now.` + `Last seen 2h ago` if the registry has a timestamp, omitted if genuinely unknown | "Retry" (re-runs the liveness check) | "Copy host address" if an `AdvertisedAddress` is known, for manual out-of-band access |
| 6 | Host not registered | `status` | `This item lives on "otherhost"` | `"otherhost" hasn't been seen by this instance yet — it may not be running, or the two instances haven't discovered each other.` | "Retry" | none — no address to offer |
| 7 | Malformed link | `alert` | "This link isn't valid" | "It may have been cut off when copied or pasted. Try copying the link again from its source." | "Go to backlog board" | none |
| 8 | Version-mismatch link | `alert` | "This link needs a newer version of stapler-squad" | "This link uses a format ({version}) this instance doesn't understand yet. Update stapler-squad to open it." | "Go to backlog board" | none |

**Rationale per case** (from `research/ux.md` §4, carried forward unchanged): deleted/archived
must never be conflated with "elsewhere" (would send the user hunting other hosts for
something that's simply gone); unreachable/not-registered both evoke the "wrong workspace"
model (name the host, never a bare 404); malformed and version-mismatch are both
recovery-actionable ("re-copy the link" / "update the binary") rather than raw parse-error
dumps.

**Why `alert` vs `status` differs by case:** deleted/malformed/version-mismatch are genuine
dead ends for *this* link as given — the user's expectation was violated (a link that should
work, doesn't), which is exactly what `role="alert"` (assertive) is for. Unreachable/
not-registered are not failures of the link itself — the item is real and the link is
correctly formed, resolution is just pending on network conditions — so `role="status"`
(polite) matches, consistent with Surface 3's non-error handoff banner using the same role.

**Edge case common to all five:** every banner must offer a way out that isn't "go back" (no
guaranteed browser history entry exists if the link was opened directly, e.g. from Slack) —
"Go to backlog board" / "Retry" satisfy this; no banner is a true dead end.

**UX acceptance criteria (apply to all 5, plus per-row specifics above):**

- AC1: Each of the 5 failure reasons produces **visibly and textually distinct** copy — no
  two cases share the same headline+body pair (testable: a snapshot/assertion per reason in
  the component test named in plan.md Story 5.1 Task 3).
  - Cross-check: 5 reasons above, so this drives **5 distinct assertions**, not fewer.
- AC2: None of the 5 states requires a browser "Back" navigation to escape — each has at
  least one actionable, keyboard-reachable control that leads somewhere (never a dead end).
- AC3: The unreachable-host and not-registered banners name the literal hostname from the
  link, never a generic "an instance" or "elsewhere."
- AC4: Deleted/archived messaging never reuses cross-host phrasing ("lives on host X") — the
  two must remain visually and textually distinguishable at a glance.
- AC5: Malformed-link and version-mismatch messaging never surfaces a raw parse error, stack
  trace, or the malformed URL string itself verbatim as the primary message (the underlying
  detail is still logged per the Observability Requirements — just not shown to the user).
- AC6: All 5 banners are announced to assistive technology without requiring the user to have
  already focused them — `alert`-role banners interrupt (assertive), `status`-role banners
  are announced at the next polite opportunity, per the role assignment table above.
- AC7: Any actionable button/link in any of the 5 states has an accessible name reflecting
  its actual destination/effect (e.g. `aria-label="Go to backlog board"`, not a bare icon with
  no label).
- AC8: Contrast of banner text/icons against background meets WCAG AA — 4.5:1 body text, 3:1
  for icons/large text — reusing `InlineError.tsx`'s existing token-based styling rather than
  introducing new colors (verify against both light and dark theme).
- AC9: Retry actions (unreachable/not-registered) are idempotent and safe to click repeatedly
  — no double-submission bug, no accumulating duplicate banners on repeated failure.

---

## Condensed Surfaces (non-interactive / backend-facing)

### Surface 9: `--open-url` CLI subcommand

```
$ stapler-squad --open-url ssq://myhost/backlog/v1/bl_01J9X7QK3M8N2P4R6S8T0V2W4Y
```
On success: shells to `open`/`xdg-open` against the translated local URL, process exits 0,
no stdout noise beyond what the OS opener itself prints. On a malformed input URL: prints a
one-line error to stderr and exits non-zero — never a Go panic/stack trace to the terminal.

**Acceptance criteria:**
- Exits 0 and hands off to the OS opener for any URL `ParseDeepLink` (Story 2.1) accepts.
- Exits non-zero with a single human-readable stderr line (no stack trace) for a malformed
  input, mirroring Surface 7's message tone ("This link isn't valid").
- Never blocks waiting on the opened browser process (fire-and-forget shell-out).

### Surface 10: Linux `.desktop` scheme registration

```
$ cat ~/.local/share/applications/stapler-squad.desktop
[Desktop Entry]
Type=Application
Name=Stapler Squad
Exec=stapler-squad --open-url %u
MimeType=x-scheme-handler/ssq;
NoDisplay=true
```

**Acceptance criteria:**
- `make install-service` on Linux installs/updates this file and runs
  `update-desktop-database`/`xdg-mime default` idempotently — re-running produces no
  duplicate `.desktop` entries and no error on a machine where it's already registered.
- After registration, clicking an `ssq://` link anywhere on the desktop (browser, terminal
  emulator hyperlink, file manager) invokes Surface 9 without further user setup.
- macOS gets no equivalent in this pass (deferred per ADR-003) — not a UX regression since
  in-app resolution (Surfaces 2–8) is unaffected; only the outside-the-browser click path is
  unavailable on macOS until the follow-up ships.

### Surface 11: Server-side deep-link resolver route

```
GET /api/deep-link/resolve?url=ssq%3A%2F%2Fmyhost%2Fbacklog%2Fv1%2Fbl_01J...
→ 200 { "kind": "local", "item": { ... } }
→ 200 { "kind": "handoff", "advertisedAddress": "http://otherhost:8543" }
→ 404 { "kind": "not-found", "reason": "deleted" | "archived" }
→ 409 { "kind": "unreachable", "reason": "not-registered" | "unreachable", "lastSeenAt"?: "..." }
→ 400 { "kind": "invalid", "reason": "malformed" | "version-mismatch" }
```

**Acceptance criteria:**
- Every one of the 5 failure reasons from Surfaces 4–8 maps to a distinct `reason` value the
  frontend can switch on directly — no string-matching a free-text message.
- Response shape is stable enough for both the web UI and the `--open-url` subcommand
  (Surface 9) to consume without duplicating resolution logic (per plan.md's "Link resolution
  surface" decision).

### Surface 12: Structured logging for link generation/resolution

```
level=info msg="deep_link.resolved" host=myhost item_id=bl_01J... kind=local
level=warn msg="deep_link.resolve_failed" host=otherhost reason=unreachable last_seen="2h ago"
```

**Acceptance criteria:**
- Every resolver outcome (success and all 5 failure reasons) is logged at info/warn per the
  Observability Plan, visible in `~/.stapler-squad/logs/stapler-squad.log` without needing a
  live repro.
- `host_advertisement.sent`/`received` events stay at debug level (not info) to avoid log
  spam at gossip frequency, per the Observability Plan's explicit callout.

---

## Summary

- **Surfaces designed:** 12 (8 full interactive treatments — Copy Link/ID, same-host
  resolution, cross-host handoff, and 5 distinct failure states — plus 4 condensed
  non-interactive surfaces).
- **UX acceptance criteria written:** 37 (Surface 1: 8, Surface 2: 5, Surface 3: 5,
  Surfaces 4-8 shared table: 9, Surface 9: 3, Surface 10: 3, Surface 11: 2, Surface 12: 2).
