# UX Design: ssh-remote-workspaces

**Date**: 2026-08-06
**Inputs**: `project_plans/ssh-remote-workspaces/requirements.md`,
`project_plans/ssh-remote-workspaces/research/ux.md`,
`project_plans/ssh-remote-workspaces/implementation/plan.md` (Phase 3 Epic 3.3,
Phase 4 Epic 4.3, Phase 6 Epics 6.1–6.4)
**Status**: Pre-implementation design artifact — read before starting Phase 6 work
(Epics 6.1–6.2), and before Phase 3's `TestRemoteConnection`/`TrustRemoteHostKey`
RPC shapes are finalized (Epic 3.3), since Surface 3 below constrains that contract.

This doc covers every user-facing surface the plan introduces, in the order a user
would actually encounter them: configure a remote once (Settings) → create a
session against it (Omnibar) → observe it (SessionCard) → recover when something
goes wrong (error states). All surfaces are designed for both desktop and mobile
per this project's standing UX requirement (`feedback_mobile_desktop_ux`).

**Conventions reused throughout** (grounded in `research/ux.md` and direct
inspection of the current codebase):
- Status badges: `role="img"` + `aria-label` pairing text with color, never color
  alone (`SessionCard.tsx:459-469`, `:507-513`, `:524-531`).
- Live-region announcements: a persistent, empty-when-idle
  `role="status" aria-live="polite"` span, visually hidden but always in the DOM
  (`SessionCard.tsx:792`), so NVDA/VoiceOver announce on content change without
  requiring focus on the card.
- Terminal failures needing user action use `role="alert"` (assertive), matching
  `inlineEditError` (`SessionCard.tsx:441`).
- Detail-on-demand goes in a `Tooltip` (`SessionCard.tsx:492`, `:506`), not inline
  text — keep badge labels terse.
- Modals/overlays use `createPortal(..., document.body)` (`NewShellDialog.tsx`,
  `QuickOpenPalette.tsx`, `SessionPeekModal.tsx`) — never `position: fixed` without
  a portal, per `.claude/rules/css-architecture.md`.
- Styling is vanilla-extract `.css.ts` with `vars.*` tokens; existing components
  already define mobile breakpoints via `"@media"` in their `.css.ts` files
  (`SessionCard.css.ts` has 9 media-query blocks) — new components follow the same
  pattern rather than inventing a new responsive approach.

---

## Surface inventory

| # | Surface | Type |
|---|---|---|
| 1 | Settings → Remotes list | Screen (empty + populated states) |
| 2 | Add Remote form | Screen/panel (+ loading state during Test Connection) |
| 3 | First-time host-key trust dialog | Modal |
| 4 | Omnibar remote selector | Form control (composes with existing creation flow) |
| 5 | SessionCard host badge | Badge |
| 6 | SessionCard connection status indicator | Badge + live region (connected/reconnecting/disconnected) |
| 7 | Error: remote unreachable at session creation | Inline form error |
| 8 | Error: SSH auth failure | Inline error (Settings) + inline error (creation) |
| 9 | Error: mid-session disconnect | Terminal pane overlay |

**9 surfaces designed.**

---

## Surface 1 — Settings → Remotes list

### Empty state (desktop, ≥768px)

```
┌─ Settings ──────────────────────────────────────────────────┐
│  General   Notifications   Approval Rules   ▸Remotes         │
├────────────────────────────────────────────────────────────┤
│                                                                │
│   No remotes configured yet.                                  │
│                                                                │
│   Register a remote host to run sessions on a dedicated       │
│   Linux box instead of this machine.                          │
│                                                                │
│              [ + Add remote ]                                 │
│                                                                │
└────────────────────────────────────────────────────────────┘
```

### Populated state (desktop)

```
┌─ Settings ──────────────────────────────────────────────────┐
│  General   Notifications   Approval Rules   ▸Remotes         │
├────────────────────────────────────────────────────────────┤
│  [ + Add remote ]                                              │
│                                                                │
│  ┌────────────────────────────────────────────────────────┐ │
│  │ ● prod-box              tyler@prod.example.com          │ │
│  │   /srv/workspaces                        [Test] [Edit] [Delete] │
│  │   Connected · trusted host key                          │ │
│  └────────────────────────────────────────────────────────┘ │
│  ┌────────────────────────────────────────────────────────┐ │
│  │ ◐ gpu-box                tyler@10.0.1.40                │ │
│  │   /home/tyler/work                       [Test] [Edit] [Delete] │
│  │   Reconnecting… last seen 3 min ago                     │ │
│  └────────────────────────────────────────────────────────┘ │
└────────────────────────────────────────────────────────────┘
```

### Mobile (<600px) — stacked, actions collapse into overflow menu

```
┌─ Remotes ──────────┐
│ [ + Add remote ]    │
│                      │
│ ┌──────────────────┐│
│ │ ● prod-box       ⋮││  ⋮ opens Test/Edit/Delete
│ │ tyler@prod...     ││
│ │ Connected         ││
│ └──────────────────┘│
│ ┌──────────────────┐│
│ │ ◐ gpu-box        ⋮││
│ │ Reconnecting…     ││
│ └──────────────────┘│
└────────────────────┘
```

### Interaction flow
1. User navigates Settings → Remotes tab (new tab in existing settings nav).
2. Empty state shows one primary CTA: **Add remote**. No decoys.
3. Populated state lists each `RemoteConfig` as a card: name, `user@host`,
   `base_path`, live connection status (reuses Surface 6's badge vocabulary),
   and row actions (Test connection / Edit / Delete).
4. **Delete** is destructive — confirm via a lightweight inline confirm
   ("Remove prod-box? Sessions already running there keep running; only future
   session creation is affected. [Cancel] [Remove]"), not a silent delete,
   since it also implicitly forgets the trusted host key relationship for that
   *entry* (the on-disk `known_hosts` record itself is left alone, matching
   the additive-only migration posture in plan.md).
5. Each row's status text is exactly the vocabulary in Surface 6 — this list is
   the "at rest" view of the same state a SessionCard shows "in context."

### Empty / loading / error
- **Empty**: single CTA, explanatory one-liner, no table chrome rendered for zero rows.
- **Loading** (list fetch): skeleton row shimmer, not a spinner overlay — avoid
  layout shift once remotes load.
- **Row-level Test-connection loading**: button becomes `Testing… [disabled]`,
  no page-level blocking.

---

## Surface 2 — Add Remote form

### Desktop

```
┌─ Add remote ───────────────────────────────────────────────┐
│  Name*          [ prod-box                              ]    │
│  Host*          [ prod.example.com                       ]    │
│  User*          [ tyler                                  ]    │
│  Port           [ 22                                     ]    │
│  Base path*     [ /srv/workspaces                        ]    │
│                                                                │
│  A new SSH keypair will be generated for this remote and       │
│  stored in your OS keychain. You'll need to add the public     │
│  key below to the remote's authorized_keys.                    │
│                                                                │
│              [ Cancel ]        [ Test connection ]             │
└────────────────────────────────────────────────────────────┘
```

After key generation (on first "Test connection" attempt):

```
┌─ Add remote ───────────────────────────────────────────────┐
│  ... (fields as above, now read-only until Cancel/retry) ...  │
│                                                                │
│  Add this to ~/.ssh/authorized_keys on prod.example.com:      │
│  ┌──────────────────────────────────────────────────────┐   │
│  │ command="...",restrict,pty ssh-ed25519 AAAA... ssq    │   │
│  └──────────────────────────────────────────────────────┘   │
│                                          [ Copy ]              │
│  Stapler Squad cannot verify this line was applied — test      │
│  the connection once it's in place.                            │
│                                                                │
│              [ Cancel ]        [ Test connection ]             │
└────────────────────────────────────────────────────────────┘
```

### Mobile — full-screen panel (not a small modal — too many fields)

```
┌─ Add remote        ✕ ┐
│ Name*                 │
│ [ prod-box          ] │
│ Host*                 │
│ [ prod.example.com  ] │
│ User*                 │
│ [ tyler             ] │
│ Base path*             │
│ [ /srv/workspaces   ] │
│                        │
│ [ Test connection ]   │  ← full-width primary action, thumb-reachable
└────────────────────────┘
```

### Interaction flow
1. User fills name/host/user/port(optional)/base_path. Client-side validation:
   required fields, `host` must not itself contain `user@` (User is a separate
   field — avoid the `user@host` string-parsing ambiguity baked into the CLI
   form of this feature).
2. Clicking **Test connection** triggers `TestRemoteConnection`:
   - Generates the per-remote Ed25519 keypair (Story 3.2.2) on first attempt,
     shows the `authorized_keys` line with a Copy button and the ADR-004
     "cannot verify this was applied" caveat verbatim.
   - Dials the remote. Three outcomes:
     a. **Success, known host key** → remote is saved, form closes, list
        updates with `Connected` status (Surface 1).
     b. **Success, unknown host key** → Surface 3 (host-key trust dialog)
        opens; remote is *not yet saved*.
     c. **Failure** (unreachable / auth failure) → Surface 7/8 inline error,
        form stays open and editable, nothing is persisted.
3. **Cancel** at any point discards the form. If a keypair was generated but
   the remote was never saved, the orphaned keychain entry is deleted (no
   dangling credential left behind) — this is a backend responsibility this
   design assumes; call it out to implementation if not already covered by
   Epic 3.2.

### Empty / loading / error
- **Loading**: `Test connection` button shows a spinner and `Testing…` label;
  all fields become read-only for the duration (prevents a race where the
  user edits `host` mid-dial). Bounded timeout (matches `research/ux.md` §4's
  "bounded retry/timeout, not an indefinite spinner") — surfaced as Surface 7
  if exceeded.
- **Error**: see Surfaces 7 and 8 — always inline in this form, never a toast,
  because the user is mid-flow and needs to fix something right here.

---

## Surface 3 — First-time SSH host-key trust dialog

Modal, triggered from Surface 2's "Test connection" when `TestRemoteConnection`
returns `host_key_unknown: true`. Per `research/ux.md` §1 (VS Code precedent):
this is a **modal dialog**, not a terminal-style prompt, and not a silent
"trust on first use" — the user must explicitly act.

### Desktop

```
        ┌─ Verify host identity ───────────────────────┐
        │                                                │
        │  Stapler Squad has not connected to this host  │
        │  before. Verify the fingerprint matches what    │
        │  the remote's administrator (you) expects.      │
        │                                                │
        │  Host:   prod.example.com:22                    │
        │  Key type: ED25519                              │
        │  Fingerprint:                                   │
        │    SHA256:k3jd93kfDJKS93kdjfKSJDF93kdjf93k       │
        │                                                │
        │        [ Cancel ]     [ Trust and connect ]     │
        └────────────────────────────────────────────────┘
```

### Mobile — bottom sheet, same content, full-width buttons stacked

```
┌────────────────────────┐
│ ▔▔▔▔▔ (drag handle)     │
│ Verify host identity     │
│                          │
│ Host: prod.example.com   │
│ SHA256:k3jd93kfDJKS93... │
│ (tap to expand/copy)     │
│                          │
│ [ Trust and connect ]   │
│ [ Cancel ]               │
└────────────────────────┘
```

### Interaction flow
1. Dialog opens with focus trapped inside it (portal-rendered, per
   `HostKeyTrustDialog.tsx` in the plan) — focus lands on **Cancel** by
   default (not "Trust and connect"), so a stray Enter keypress doesn't
   silently trust an unverified host.
2. Fingerprint is displayed in the OpenSSH `SHA256:<base64>` format, matching
   what a user would see running `ssh-keygen -lf` on the remote directly, so
   they can cross-check out-of-band (e.g. via a cloud provider's console) if
   they choose to.
3. **Trust and connect** → `TrustRemoteHostKey` RPC with that exact
   fingerprint (server rejects if the fingerprint doesn't match what it
   computed — "defense against blindly trusting a different key," per Task
   3.3.2d) → dialog closes → Surface 2 continues to "remote saved" success.
4. **Cancel** → dialog closes, remote is not saved, Surface 2 form remains
   open and editable (no dead end — user can retry Test connection, e.g.
   after confirming the fingerprint against the box directly).
5. Dismissing via Escape key or backdrop click == Cancel (never == Trust).

### Empty / loading / error
- No empty state (dialog only renders when there's a fingerprint to show).
- If a **previously-trusted** host's key changes (MITM or box reprovisioned),
  this is a materially different message — not "first-time," but "changed" —
  and must use stronger language ("This host's key has changed since you last
  connected — this could mean the host was reconfigured, or something is
  intercepting the connection") with the old and new fingerprints both shown.
  This scenario isn't explicitly scoped in the plan's Epic 3.3 acceptance
  criteria (which only covers "never seen" vs "previously trusted, matches");
  flagging it here as a design requirement so it isn't silently treated the
  same as first-time trust — the risk profile is inverted.

---

## Surface 4 — Omnibar remote selector

Per ADR-001 / `research/ux.md` §2: **not a 6th session-type radio option** — an
orthogonal selector, hidden entirely when zero remotes are configured, composing
with whichever `sessionType` is already selected. Renders adjacent to the
existing "Autonomous mode" checkbox (`OmnibarCreationPanel.tsx` ~line 462).

### Desktop — zero remotes configured (today's behavior, unchanged)

```
┌─ New session ──────────────────────────────────────────────┐
│  Session type:  ( ) New worktree  (•) Existing folder  ( )... │
│  Path: [ /home/tyler/code/foo                            ]    │
│  ☐ 🤖 Autonomous mode (Beta)                                   │
│                                          [ Cancel ] [ Create ] │
└────────────────────────────────────────────────────────────┘
```

### Desktop — ≥1 remote configured

```
┌─ New session ──────────────────────────────────────────────┐
│  Session type:  (•) New worktree  ( ) Existing folder  ( )... │
│  Branch: [ feature-x                                      ]    │
│  Run on: [ This machine ▾ ]                                    │
│           ┌──────────────────┐                                │
│           │ ✓ This machine    │                                │
│           │   prod-box  ●     │ ← live status dot, same         │
│           │   gpu-box   ◐     │   vocabulary as Surface 6        │
│           └──────────────────┘                                │
│  ☐ 🤖 Autonomous mode (Beta)                                   │
│                                          [ Cancel ] [ Create ] │
└────────────────────────────────────────────────────────────┘
```

### Mobile

```
┌─ New session      ✕ ┐
│ Session type         │
│ [ New worktree    ▾] │
│ Branch                │
│ [ feature-x         ] │
│ Run on                │
│ [ This machine    ▾] │  ← native <select> on mobile, same
│                        │    options, avoids custom dropdown
│ ☐ 🤖 Autonomous mode  │    touch-target issues
│                        │
│      [ Create ]       │
└────────────────────────┘
```

### Interaction flow
1. Selector defaults to **"This machine"** — zero new decisions for the
   common (local) case, matching `research/ux.md` §2's explicit
   recommendation.
2. Selecting a remote does **not** change or hide any other field in the
   form — session type, branch, working-dir fields stay exactly as they'd
   behave locally (composability, per Story 4.3.2's acceptance criteria).
3. Each remote in the dropdown shows its live connection-status dot inline
   (reusing Surface 6's icon+color, though the text label is omitted here for
   density — the dropdown option's accessible name still includes it, e.g.
   `aria-label="prod-box, connected"`).
4. Selecting a remote that is currently `disconnected` is **not blocked** —
   session creation is attempted, and any failure surfaces via Surface 7, so
   the user isn't prevented from retrying a create right as a flaky remote
   comes back up. (Blocking here would be a dead end if the status is stale.)
5. Submitting calls `CreateSession` with `remote.remote_name` set; on success,
   the new session's card immediately shows Surfaces 5+6.

### Empty / loading / error
- **Empty** (zero remotes): selector is entirely absent from the DOM — not
  present-but-disabled — per Story 4.3.2's acceptance criteria
  (`queryByTestId("remote-selector")` returns null).
- **Loading**: none — the dropdown's option list is sourced from
  already-fetched Redux state (`remotesSlice`, push-driven), not a per-open
  fetch.
- **Error**: handled entirely by Surface 7 after submission, not at
  selection time.

---

## Surface 5 — SessionCard host badge

Mirrors the existing `externalBadge` pattern exactly (`SessionCard.tsx:459-469`).

```
┌─ my-feature-branch ──────────────────────────────────┐
│  🖥 prod-box   ● Connected   ✓ Running                 │
│  feature-x                                              │
│  +42 -13                                                 │
└──────────────────────────────────────────────────────┘
```

- Renders only when `session.remoteName` is set; absent entirely for local
  sessions (no "Local" badge added for symmetry — per `research/ux.md` §2,
  a badge that's meaningless for the common case dilutes the signal for
  where it matters).
- `role="img"`, `aria-label="Running on prod-box"` — icon (🖥 or similar) is
  `aria-hidden`, the label carries the meaning, per the `externalBadge`
  precedent.
- Tapping/clicking the badge is inert (informational only) — it is not a
  secondary navigation target that competes with the card's primary click
  target (opening the session). If a future iteration wants "click host
  badge → jump to Settings → Remotes → prod-box," that's an explicit
  follow-up, not assumed here.

---

## Surface 6 — Connection status indicator

Distinct badge from the session's own lifecycle status (`getStatusText`) — a
session can be `PAUSED` while its remote connection is `connected`, or `ACTIVE`
while `reconnecting`. Per `research/ux.md` §3, these must never be merged into
one badge.

### States

```
● Connected        (green, statusConnected)
◐ Reconnecting…     (amber, statusReconnecting, pulsing per existing spinner pattern)
○ Disconnected      (red, statusDisconnected)
```

### In context on a card

```
┌─ my-feature-branch ──────────────────────────────────┐
│  🖥 prod-box   ◐ Reconnecting…    ⏸ Paused             │
│  feature-x                                              │
└──────────────────────────────────────────────────────┘
   ^host badge   ^connection status    ^session lifecycle status
   (Surface 5)   (this surface)        (existing, unchanged)
```

### Persistent live region (visually hidden, always in DOM)

```html
<span role="status" aria-live="polite" id="remote-status-{sessionId}">
  Connection to prod-box lost, reconnecting…
</span>
```

For a terminal failure (auth failure, exhausted reconnect attempts):

```html
<span role="alert" id="remote-status-{sessionId}">
  Disconnected from prod-box — SSH auth failed. Check the key in Settings → Remotes.
</span>
```

### Interaction flow
1. State is driven entirely by push events (`RemoteHealthProber` →
   `NewRemoteHealthChangedEvent` → `remotesSlice`) — the component issues no
   network requests itself (Story 6.2.2's acceptance criteria).
2. `connected → reconnecting`: badge updates, `aria-live="polite"` region
   announces "Connection to {host} lost, reconnecting…" — expected,
   self-healing, not alarming.
3. `reconnecting → connected`: badge updates back, live region announces
   "Connection to {host} restored." (symmetry — a user who stepped away and
   comes back should get the same clarity for recovery as for loss).
4. `reconnecting → disconnected` (terminal, e.g. auth failure or exhausted
   backoff): badge updates, **and** the announcement upgrades to
   `role="alert"` (assertive) with an actionable message naming the specific
   cause and where to fix it, per `research/ux.md` §4.
5. Tooltip on the badge (reusing the existing `Tooltip` component) carries
   detail: last-connected timestamp, specific error text — badge label stays
   terse ("Reconnecting…"), detail is on-demand.

### Empty / loading / error
- **Empty**: badge absent for local sessions (same rule as Surface 5 — no
  "Connected" badge on a local session card).
- No separate loading state — `connecting` (initial dial, before any prior
  state existed) renders identically to `reconnecting` visually, since both
  communicate "not yet connected, working on it." (The plan's Unresolved
  Questions section flags whether Coder's finer-grained `connecting`/`timeout`
  states are needed for v1 — this design's position: the 3-state model is
  sufficient for the badge/announcement text, but implementation should keep
  the `RemoteConnectionState` type extensible so a `connecting` sub-state can
  be added later without a UI contract change.)
- **Error** (terminal `disconnected`): see interaction step 4 above and
  Surface 9 for how this manifests inside an open terminal view specifically.

---

## Surface 7 — Error: remote unreachable at session creation

Applies to Surface 4's Create action when the target remote can't be dialed
(network partition, remote host down) — as opposed to Surface 8's auth-specific
failure.

```
┌─ New session ──────────────────────────────────────────────┐
│  ... (form fields unchanged, still editable) ...              │
│                                                                │
│  ⚠ Couldn't reach prod-box. Check that the host is up, or      │
│    pick a different remote.                    [ Retry ]      │
│                                          [ Cancel ] [ Create ] │
└────────────────────────────────────────────────────────────┘
```

### Interaction flow
1. Reuses the existing `error: string | null` prop already threaded through
   `OmnibarCreationPanel` (`error` rendered at the bottom of the form,
   `OmnibarCreationPanel.tsx:816-817`) — no new error-surfacing mechanism.
2. Message names the specific remote, states the likely cause in plain
   language, and gives two explicit exits: **Retry** (re-attempt the same
   request) or change the "Run on" selection back to "This machine" / a
   different remote and hit **Create** again. Form is never locked.
3. Bounded timeout on the creation attempt (no indefinite spinner) — per
   `research/ux.md` §4's Codespaces-derived guidance.

### Empty / loading / error
- Not applicable beyond the error text itself — this *is* the error state.

---

## Surface 8 — Error: SSH auth failure

Two distinct places this can surface, per `research/ux.md` §4's recommendation
to catch it early (at configuration) rather than only at creation time:

### 8a. At Settings → Test connection (Surface 2)

```
┌─ Add remote ───────────────────────────────────────────────┐
│  ... (fields, still editable) ...                             │
│                                                                │
│  ⚠ Auth failed for prod-box. The remote rejected the           │
│    generated key — confirm the authorized_keys line above      │
│    was added correctly, then test again.                       │
│                                                                │
│              [ Cancel ]        [ Test connection ]             │
└────────────────────────────────────────────────────────────┘
```

### 8b. At session creation (e.g. key rotated remotely after the remote was
already configured and working)

```
┌─ New session ──────────────────────────────────────────────┐
│  ⚠ Auth failed for prod-box — check the SSH key in             │
│    Settings → Remotes.              [ Open Settings → Remotes ]│
│                                          [ Cancel ] [ Create ] │
└────────────────────────────────────────────────────────────┘
```

### Interaction flow
1. Both cases name the remote explicitly and point at the exact place to fix
   it (`research/ux.md` §4: "the message naming the remote and the specific
   failure... so the user knows exactly where to go fix it, not just that
   something broke").
2. Case 8b includes a direct navigation action (**Open Settings → Remotes**)
   rather than making the user remember where to go — this is the one place
   in this design where an inline form error also carries a cross-surface
   navigation affordance, justified because the fix genuinely lives
   elsewhere (credentials, not anything in the creation form itself).
3. Neither case blocks the user from cancelling out and trying a different
   remote or local execution instead.

### Empty / loading / error
- Same as Surface 7 — this is itself the error-state design.

---

## Surface 9 — Error: mid-session disconnect (terminal pane)

Applies once a session is open and its remote connection drops while the user
is actively viewing the terminal (xterm.js view).

```
┌─ my-feature-branch ─ prod-box ─────────────────────────────┐
│  $ npm test                                                    │
│  ...                                                            │
│  PASS  src/foo.test.ts                                         │
│  ┌──────────────────────────────────────────────────────┐    │
│  │        ◐ Reconnecting to prod-box…                      │    │
│  │        Last output above is preserved.                  │    │
│  └──────────────────────────────────────────────────────┘    │
│  (last-rendered frame stays visible underneath, dimmed)        │
└────────────────────────────────────────────────────────────┘
```

After exhausting reconnect attempts (terminal failure):

```
┌─ my-feature-branch ─ prod-box ─────────────────────────────┐
│  $ npm test                                                    │
│  ...  (scrollback preserved, dimmed)                            │
│  ┌──────────────────────────────────────────────────────┐    │
│  │  ○ Disconnected from prod-box                           │    │
│  │  SSH auth failed — check the key in Settings → Remotes. │    │
│  │                              [ Retry ] [ Open Settings ]│    │
│  └──────────────────────────────────────────────────────┘    │
└────────────────────────────────────────────────────────────┘
```

### Interaction flow
1. On disconnect, the terminal does **not** clear or blank — the last-rendered
   frame stays visible (dimmed to signal "stale, not live") underneath a
   small overlay banner, per `research/ux.md` §4's explicit requirement
   ("don't clear scrollback... only fall back to a hard 'Disconnected' empty
   state after a bounded number of reconnect attempts").
2. The overlay uses the same `Reconnecting…`/`Disconnected` vocabulary and
   icon/color as Surfaces 5/6 — a user who glances at the card and then opens
   the session sees the identical status language, no re-learning.
3. This state is **not** conflated with the session-lifecycle status
   (`PAUSED`/`STOPPED`) — per `research/ux.md` §4, mixing "network is down"
   into that vocabulary would misrepresent whether the agent itself needs
   attention vs. whether the network does. The two badges (Surface 5 host,
   Surface 6 connection, existing lifecycle status) all remain visible and
   independently readable even while this overlay is showing.
4. Input typed into the terminal while disconnected is not silently dropped —
   either buffered with a visible "will send on reconnect" indicator, or the
   input box is disabled with a clear reason ("Reconnecting — input paused"),
   whichever the terminal component's existing architecture supports more
   naturally. This decision is flagged for implementation to confirm against
   `PtyFactory`'s actual buffering behavior (Epic 4.4) — this design's
   requirement is only that it must be **one or the other, visibly**, never
   silent data loss.
5. **Retry** and **Open Settings** exits mirror Surface 8b — no dead end even
   in the terminal, deepest-nested error state in the whole feature.

### Empty / loading / error
- This surface *is* the error/degraded state for an otherwise-normal
  in-progress terminal session; there is no separate empty state (a session
  with no output yet behaves like today's existing "Starting session..."
  creation-progress state, unrelated to this surface).

---

## UX Acceptance Criteria

Each criterion below is human-testable against a running build.

### Task completion
1. A user with zero remotes configured can add a remote and reach "Connected"
   status in ≤5 steps: open Settings → Remotes → Add remote → fill 4 required
   fields → Test connection (→ Trust and connect, only if host key is
   unknown) → remote appears in the list as Connected.
2. A user with ≥1 configured, connected remote can create a session against
   it in ≤4 interactions from the Omnibar: open New session → pick session
   type (if not already default) → select the remote from "Run on" → Create.
3. From any error state in Surfaces 7, 8, or 9, a user can reach a working
   next step (retry, change remote, or navigate to the exact Settings screen
   that fixes it) in **1 click/tap** — no error state requires more than one
   action to find the recovery path.

### Error states
4. Remote-unreachable-at-creation (Surface 7) shows the specific remote name
   and the phrase "Couldn't reach {remote}" and offers a **Retry** action
   without leaving the creation form or losing any already-entered field
   values (session type, branch, path).
5. Auth failure (Surface 8, both locations) shows the specific remote name,
   the phrase "Auth failed for {remote}", and names **Settings → Remotes** as
   the place to fix it — generic messages like "Something went wrong" or
   "Connection error" are BLOCKER-level failures of this criterion.
6. Mid-session disconnect (Surface 9) never blanks or clears terminal
   scrollback — the last-rendered output frame remains visible under the
   overlay at all times during `reconnecting` and `disconnected` states.
7. First-time host-key trust (Surface 3) never auto-trusts — a fingerprint is
   always shown and requires an explicit "Trust and connect" click before any
   remote is persisted to config; hitting Escape or clicking the backdrop is
   equivalent to Cancel, never to Trust.
8. A host key that changes after being previously trusted produces a visibly
   different (more alarming) message than first-time trust, showing both the
   previously-trusted and newly-seen fingerprints.

### No dead ends
9. Every error state defined in Surfaces 7, 8, and 9 has at least one visible,
   enabled exit action (Retry, Cancel, or a navigation link) — none can leave
   the user stuck with only a disabled form or a blank screen.
10. Cancelling out of the host-key trust dialog (Surface 3) returns focus to
    the Add Remote form (Surface 2) in an editable state, not to a dead form
    or a closed flow that must be restarted from Settings.

### Composability (session-creation registry integrity)
11. With ≥1 remote configured, selecting a remote in the Omnibar (Surface 4)
    does not hide, disable, or alter the behavior of the session type radio
    group, branch field, or any other existing creation-form field — a
    `new_worktree` session created against a remote produces the same
    worktree/branch semantics as one created locally, differing only in
    where it runs.
12. With zero remotes configured, no remote-related UI (selector, host badge,
    connection indicator) is present anywhere in the DOM — verified as an
    absence, not a disabled/greyed-out presence.

### Accessibility
13. Every status badge introduced by this feature (host badge — Surface 5;
    connection status — Surface 6) pairs `role="img"` with an `aria-label`
    that fully states the meaning in words; no badge conveys state through
    color or icon alone.
14. Connection-state transitions (`connected`↔`reconnecting`) are announced
    via a persistent `role="status" aria-live="polite"` region without
    requiring the session card or terminal to have focus; terminal failure
    transitions (`→ disconnected` due to an unrecoverable cause) upgrade to
    `role="alert"` (assertive).
15. All interactive elements introduced (Add remote form fields, Test
    connection button, Trust/Cancel dialog buttons, remote selector dropdown,
    row actions in the Remotes list) are reachable and operable via keyboard
    alone (Tab/Shift+Tab, Enter/Space, Escape to dismiss modals), with a
    visible focus indicator at every stop.
16. The host-key trust dialog (Surface 3) traps focus while open and returns
    focus to the triggering control (Test connection button) on close.
17. Text and icon colors used for `statusConnected` / `statusReconnecting` /
    `statusDisconnected` meet WCAG AA contrast (≥4.5:1) against their
    background in both light and dark themes — verified against the actual
    token values once assigned in `RemoteConnectionIndicator.css.ts` (this
    doc does not hardcode colors; it names the token family and required
    contrast, per `.claude/rules/css-architecture.md`'s "no hardcoded hex"
    rule).
18. All form inputs in the Add Remote form (Surface 2) have associated
    `<label>` elements (not placeholder-only labeling), and the required-field
    indicator (`*`) is conveyed both visually and via `aria-required`.

### Mobile/desktop parity
19. Every surface in this document has a defined mobile layout (≤600px) that
    preserves all information present in the desktop layout — no field,
    badge, or status text is desktop-only. Row actions that collapse into an
    overflow menu on mobile (Surface 1) remain individually reachable, not
    merged or dropped.
20. All interactive targets on mobile layouts (buttons, dropdown, overflow
    menu trigger) meet a minimum 44×44px touch target per the existing
    mobile+desktop UX standard already applied elsewhere in this codebase.

---

## Open items for implementation to confirm

These are called out inline above but repeated here since they affect scope,
not just visuals:

- **Host-key-changed scenario** (Surface 3, item 8): not explicitly covered by
  Epic 3.3's stated acceptance criteria (`known host, matching key` vs.
  `never-seen host`) — needs a third `KnownHostsStore.Verify` outcome
  (`ErrHostKeyMismatch`, distinct from `ErrUnknownHostKey`) for the frontend
  to render the correct (more alarming) variant of this dialog.
- **Terminal input during disconnect** (Surface 9, step 4): whether input is
  buffered-and-flushed or blocked-with-message depends on `PtyFactory`'s
  actual behavior post-Epic-4.4 generalization — pick one, but don't ship
  silent input loss.
- **`RemoteConnectionState` granularity**: this design uses the plan's
  3-state model (`connected`/`reconnecting`/`disconnected`) throughout, per
  the plan's own Unresolved Questions item on this — if a `connecting`
  sub-state is added later, badge text and live-region copy in Surface 6
  extend without a structural change (type left open on purpose).
