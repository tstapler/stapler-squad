# UX Design: app-scrollback-forwarding

SDD Phase 3 (UX design pass), project `app-scrollback-forwarding`. Grounded in
`requirements.md`, `research/ux.md`, and `implementation/plan.md` — every
component name, response shape, and copy string below matches what the plan
already committed to build (`ScrollSourceIndicator`, `ScrollForwardOutcome`
enum `DELIVERED`/`AT_TOP`/`NO_CAPABILITY`/`BLOCKED`, the single-client-only
`Blocked` gate). This document does not introduce new engineering surface —
it specifies the copy, layout, timing, and accessibility wiring for surfaces
the plan already scoped.

## Surface inventory

| # | Surface | Type | Plan reference |
|---|---------|------|-----------------|
| 1 | Scroll-forward in-progress (loading) | Interactive | Story 1.3.2 (`RedrawQuiescence`), research/ux.md §0/§4c |
| 2 | `ScrollSourceIndicator` banner (`DELIVERED`) | Interactive | Story 1.4.2 |
| 3 | Blocked — 2+ clients connected (`BLOCKED`) | Interactive | Story 1.4.3, Risk Control's multi-client decision |
| 4 | Reached the hard limit (`AT_TOP`, Phase-1-scoped to the app-forwarded path only — see Scope boundary below) | Interactive | Story 1.4.4, Task 1.4.4b |
| 5 | No known scroll command (`NO_CAPABILITY`) | Interactive (degenerate — renders as today's baseline) | Story 1.4.4 |
| 6 | Feature flags (`terminal:app-scrollback-forwarding:*`) | Non-interactive (config) | Story 1.5.1 |
| 7 | Observability — logs & counters | Non-interactive (ops output) | Epic 1.5, requirements' Observability Requirements |
| 8 | `scrollForwardKeybindingCanary` warning | Non-interactive (ops output) | Story 1.5.2 |

Design tokens cited below (`vars.color.*`) are read from
`web-app/src/styles/theme-contract.css.ts`; layout/positioning precedents are
read from `web-app/src/components/sessions/TerminalOutput.css.ts`'s existing
`reconnectingBanner`/`hardFailedBanner`/`unavailableOverlay` rules and
`InputDropBadge.tsx`'s portal-toast pattern — every surface below reuses one
of these three existing shapes rather than inventing a fourth.

---

## Surface 1 — Scroll-forward in-progress

### Why this needs a design at all
`research/ux.md` §0 confirms the *existing* tmux-native paged-scrollback
fetch has **no loading indicator today** (`isFetchingScrollbackRef` is set
but never read by any render path). §2 argues this reads as chat-style
infinite-scroll-up to users, who expect a spinner the way Slack/WhatsApp
give one. The new forwarded-scroll path adds a strictly longer wait — up to
`waitForRedrawQuiescence`'s 2s deadline (Story 1.3.2) versus a simple network
round-trip — so shipping it with zero loading affordance would make an
already-marginal gap actively worse. This surface is in scope for *both*
paths sharing one implementation, per the same "close the gap once, not
twice" principle the plan applies to Surface 4.

### Wireframe (desktop, wheel/trackpad scroll)

```
┌─────────────────────────────────────────────────────────────┐
│ ╭─────────────────────────────────────────╮  <- inline pill,  │
│ │  ⟳ Loading Claude Code's history…        │     not a full    │
│ ╰─────────────────────────────────────────╯     overlay        │
│                                                                 │
│  claude> Let me check the test file first.                     │
│  ...existing visible transcript content, unobscured...         │
│                                                                 │
│  > _                                                            │
└─────────────────────────────────────────────────────────────┘
```

- Reuses `reconnectingBanner`'s exact shape: absolutely positioned pill,
  `top: vars.space["2"]`, horizontally centered, `background:
  vars.color.modalBackground`, `color: vars.color.textPrimary`,
  `role="status"`, `aria-live="polite"`. Not a full-screen
  `loadingOverlay` (that pattern is reserved for the initial-content case,
  `isLoadingInitialContent` — this is a mid-session paged fetch, and the
  user's already-visible content must stay visible and interactive
  underneath, matching how a chat app's "loading older messages" spinner
  never blocks the messages already on screen).
- Label reads `Loading Claude Code's history…` — program name interpolated
  from `AppScrollbackResponse.program`, generic `Loading more…` for the
  unchanged tmux-native path (this surface's implementation is shared, but
  the two paths render slightly different copy since one has a known
  program name and the other doesn't).

### Interaction flow

| Step | User action | System response |
|------|-------------|------------------|
| 1 | Scrolls up (wheel/trackpad, or a touch drag) at the top of the viewport. **Plain-shell sessions** use the existing `viewportY < 200` DOM-scroll trigger (`TerminalOutput.tsx:1047-1058`), unchanged. **Alt-screen sessions** (the case this whole feature exists for) use Story 1.4.0's independent trigger instead — a `wheel` listener plus `useTerminalGestures`'s `SCROLLING` state — since `viewportY` never moves for an alt-screen pane and the DOM `scroll` event this row originally assumed may never fire at all for one (`research/architecture.md` §3; this superseded an earlier draft of this document that assumed the same trigger covered both cases identically, per research/ux.md §4d's now-corrected claim — see plan.md's Story 1.4.0) | Client sends `ScrollbackRequest`; if `AppScrollGate` will pass server-side, `isFetchingScrollbackRef`-equivalent state flips true |
| 2 | (waits) | Loading pill appears after a **150ms delay**, not instantly — avoids a flash-of-spinner for the common case where `RedrawQuiescence` settles fast (mirrors the standard "don't show a spinner for sub-200ms operations" heuristic; keeps the surface calm for the majority-fast case while still covering the up-to-2s worst case) |
| 3 | (still waits) | Server sends `AppScrollbackResponse` (`DELIVERED`, `AT_TOP`, `NO_CAPABILITY`, or `BLOCKED`) |
| 4 | — | Loading pill is replaced by the outcome-specific surface (2, 3, 4, or 5 below); pill never persists past outcome delivery |
| 5 (timeout) | User keeps the tab open past `RedrawQuiescence`'s 2s deadline with no server response at all (network stall, not app-level `deadlineExceeded` — that case already returns a result per Story 1.3.2) | After **8s** total with no response, pill copy changes to `Still trying to load Claude Code's history…` (reassures the user this isn't stuck silently) and a **Cancel** action appears (returns to the live view without waiting further) — this is the "no dead ends" exit path per UX-AC-1 below |

### Error / edge cases
- **Mid-flight scroll-down (user changes their mind)**: scrolling back down while the pill is showing does not cancel the in-flight request (nothing to gain from racing a cancel against a request that's already in the pipe) — the pill simply disappears once the response arrives and is applied at the top of the buffer, where the user is no longer looking. No error, no toast.
- **New live output arrives while forwarding is in flight**: per `research/ux.md` §4c and the plan's `ScrollLease`, this is a server-side ordering problem (queue live output for the lease's duration), not a client rendering problem — the client only ever sees the loading pill, then one coherent outcome; it never renders an interleaved/torn frame. Called out here because it's the reason the loading pill must not be dismissed early on partial data.

---

## Surface 2 — `ScrollSourceIndicator` banner (`DELIVERED`)

### Wireframe

```
┌─────────────────────────────────────────────────────────────┐
│ ╭─────────────────────────────────────────────────────────╮ │
│ │  📜 Viewing Claude Code's own history                     │ │
│ ╰─────────────────────────────────────────────────────────╯ │
│                                                                 │
│  claude> Earlier, I ran the migration before the schema        │
│  change — here's the sequence of commands...                   │
│  ...forwarded transcript content...                            │
│                                                                 │
└─────────────────────────────────────────────────────────────┘
        ▲ mobile: same banner, full-width, sits above the
          custom left-side scrollbar track (does not overlap it)
```

- Same positioning family as Surface 1's pill but **persistent** (does not
  auto-dismiss on a timer) — it stays up for as long as the `ScrollLease` is
  held by this client, i.e. for as long as the viewport shows forwarded
  content instead of the live tail.
- Icon (📜) is `aria-hidden="true"`, decorative only — the text label alone
  carries the meaning, consistent with `ConnectionCountIndicator`'s `👥`
  treatment.
- Copy: `Viewing <Program>'s own history` — the plan's exact string for
  Claude Code (`Viewing Claude Code's own history`); `<Program>` comes from
  `AppScrollbackResponse.program`, not hardcoded, so Phase 2's pi adapter
  gets `Viewing pi's own history` for free with zero new client code.

### Interaction flow

| Step | User action | System response |
|------|-------------|------------------|
| 1 | Scroll-up completes with `outcome=DELIVERED` (Surface 1's pill was showing) | Pill is replaced by this persistent banner; `aria-live="polite"` region announces the banner's text exactly once (mount-time announcement, not on every re-render — same one-shot discipline `ConnectionCountIndicator` already uses) |
| 2 | User keeps scrolling up | Additional forwarded pages stream in under the same banner; banner text does not change or re-announce per page (only on state transitions: none → shown, shown → gone) |
| 3 | User scrolls back down toward the live tail, past the point where the server resumes sending live frames | Banner unmounts; no exit announcement is required here (unlike `ConnectionCountIndicator`'s departure-hold — losing a "you were viewing history" state when you asked to return to live is the expected, requested outcome, not a surprising background event a screen-reader user needs proactively told about) |

### Error / edge cases
- **Banner shown but a `BLOCKED` or `AT_TOP` response arrives for a subsequent page**: banner does not flicker off between pages — Surface 3/4's own affordance renders *alongside* the still-current banner (the user is still "viewing history," they've just hit a boundary on it), not as a replacement for it.
- **Session ends / disconnects while the banner is showing**: banner unmounts as part of the existing reconnect/teardown flow (Surface 1 from `TerminalOutput.tsx`'s `showReconnectBanner`) — no special-case handling needed; reconnection resets to the live tail per existing behavior, so a stale "Viewing history" banner never survives a reconnect.

---

## Surface 3 — Blocked (`BLOCKED`, keyed on `ScrollBlockedReason`)

This is the surface the parallel adversarial reviewer is specifically
checking, and it is the one place this design deliberately layers **two**
signals — an ambient one that exists before the user ever tries to scroll,
and a reactive one delivered at the moment the attempt is actually blocked —
because a single reactive toast alone still leaves a user who dismisses it
too fast, or who has a screen reader configured to skip toasts, without any
standing explanation for *why* scroll-up is unavailable.

**Three distinct causes, three distinct messages** (adversarial-review
BLOCKER fix — an earlier draft collapsed all three into one hardcoded
"another viewer is connected" toast, which is factually false for a solo
user whose session happens to be on `PathLegacyPerConnection`, unconditionally
`Blocked` there regardless of actual viewer count; see plan.md's `Instance.
ForwardScroll`/`ScrollBlockedReason` and Story 1.4.3):

| `ScrollBlockedReason` | Toast copy | When it's true |
|---|---|---|
| `MULTIPLE_VIEWERS` | "Can't browse `<Program>`'s history right now — another viewer is connected. Close other tabs or sessions viewing this session to enable it." | A real second subscriber is attached (`PathHubOwned`, `SubscriberCount() >= 2`) |
| `UNSUPPORTED_STREAMING_PATH` | "Scroll-forwarding isn't available for this session yet." | The session is on `PathLegacyPerConnection`, which can't report an accurate viewer count and is blocked unconditionally, solo user or not |
| `LEASE_CONTENTION` | "Still loading — try again in a moment." | This client's own rapid repeat scroll raced its prior in-flight request |

Only the `MULTIPLE_VIEWERS` case pairs with the `ConnectionCountIndicator`
pulse below — the other two reasons have no connected-viewer state to point
to, so their toasts stand alone with no ambient signal.

### Signal 1 (ambient, pre-existing, reused as-is): `ConnectionCountIndicator`
Already shipped (commit `0ce32e531`'s "system status banner" family;
component at `web-app/src/components/sessions/ConnectionCountIndicator.tsx`).
Renders `👥 2` the moment a second connection joins — **before** the user
scrolls at all. This document adds no new code for this signal; it is called
out because it changes the framing of Signal 2 below from "the app just
refused my scroll for no visible reason" to "the badge I can already see
explains it."

### Signal 2 (new, reactive): blocked toast

The wireframe and interaction table below illustrate the `MULTIPLE_VIEWERS`
case specifically (the only reason that pairs with Signal 1's ambient badge).
`UNSUPPORTED_STREAMING_PATH` and `LEASE_CONTENTION` reuse the identical toast
mechanism and timing, swapping in their own copy from the table above, with
no `ConnectionCountIndicator` pulse (there is no viewer-count state relevant
to either case).

### Wireframe

```
Ambient state (visible the whole time a 2nd tab is open, top-right of the
session header, per ConnectionCountIndicator's existing placement):

┌─────────────────────────────────────────────────────────────┐
│  claude-migration-fix              👥 2  [settings] [⋮]        │
├─────────────────────────────────────────────────────────────┤
│                                                                 │
│  ...terminal content...                                        │
│                                                                 │
└─────────────────────────────────────────────────────────────┘

User scrolls to top → blocked toast appears (portal-rendered, matches
InputDropBadge's fixed-position toast shape, bottom-of-viewport):

┌─────────────────────────────────────────────────────────────┐
│  claude-migration-fix          [👥 2] ← briefly pulses         │
├─────────────────────────────────────────────────────────────┤
│                                                                 │
│  ...terminal content, scroll position unchanged...             │
│                                                                 │
│              ╭───────────────────────────────────────────╮   │
│              │ ⚠ Can't browse Claude Code's history right │   │
│              │   now — another viewer is connected.       │   │
│              │   Close other tabs or sessions viewing     │   │
│              │   this session to enable it.        [Got it]│  │
│              ╰───────────────────────────────────────────╯   │
└─────────────────────────────────────────────────────────────┘
```

- Toast styling: `background: vars.color.warningBg`, `color:
  vars.color.warningText` — deliberately **not** `errorDark`/`textInverse`
  (the `hardFailedBanner` treatment). This is a policy boundary the app is
  enforcing by design, not a failure — the plan's own Story 1.4.3 language
  is explicit: "not a silent no-op and not an error state." `role="status"`
  + `aria-live="polite"`, not `role="alert"`, for the same reason (an
  `alert` role interrupts screen-reader users immediately and implies
  something has gone wrong; this is an expected, self-correcting condition).
- `[Got it]` dismisses the toast early; otherwise it auto-dismisses after
  `DEFAULT_TOAST_MS` (`web-app/src/lib/notification-policy.ts`, the same
  constant `InputDropBadge` already uses — no new timing constant to invent
  or tune).
- The `ConnectionCountIndicator` badge briefly gets a `pulse` CSS class
  (a 600ms outline-flash, reusing the badge's existing `expanded`/tooltip
  visual language, no new component) at the same moment the toast mounts —
  a lightweight visual link between "here's why" (toast) and "here's the
  standing state that caused it" (badge), so a user who later closes the
  second tab can look at the same badge to confirm the block has cleared,
  rather than having to re-attempt the scroll blind.

### Interaction flow

| Step | User action | System response |
|------|-------------|------------------|
| 1 | A second client (tab, `ssq-mux` attach) connects to the session | `ConnectionCountIndicator` mounts, announces "2 connections active" once (existing behavior, unmodified) |
| 2 | User on the first client scrolls to the top | Client sends `ScrollbackRequest`; server's `AppScrollGate` evaluates `subscriberCount == 1` as false, returns `BLOCKED` before any PTY write (per plan's Story 1.3.1 acceptance criteria — no keystroke is ever sent for this outcome) |
| 3 | — | Surface 1's loading pill (if it had appeared) is replaced by the blocked toast; `ConnectionCountIndicator` badge pulses |
| 4a | User closes the second tab, then scrolls to the top again | `ConnectionCountIndicator` unmounts (count back to 1); the retry succeeds and Surface 1 → Surface 2 (`DELIVERED`) proceeds normally — **this is the path forward**, and it requires no in-app action beyond "close the other tab," which the toast copy states explicitly |
| 4b | User leaves the second tab open and scrolls to the top again | Toast reappears (same copy) — **not rate-limited/suppressed on repeat attempts**, because each attempt is a distinct, deliberate user gesture and the copy is the honest answer every time; this differs from `InputDropBadge`'s episode-coalescing (that badge exists to prevent an unbounded string of *automatic* drop events from spamming the user — a blocked *user-initiated* scroll is not the same failure shape) |

### Error / edge cases
- **`PathLegacyPerConnection` sessions** (plan's Risk Control notes these can't support an accurate subscriber count): `AppScrollGate` still returns the same `ScrollForwardOutcome.Blocked` verdict as a 2+-subscriber `PathHubOwned` session (always `Blocked`, unconditionally) — but as of the adversarial-review fix, the two cases carry **different** `ScrollBlockedReason` values and therefore **different** toast copy (see the table above): `UNSUPPORTED_STREAMING_PATH`'s "isn't available for this session yet," never the "another viewer is connected" claim. This reverses this document's earlier position (unifying the copy "since the user does not need to know which streaming architecture their session is on") — that reasoning held only if the copy were true for both cases, and it isn't: a solo user on the legacy path has no other viewer to close, so telling them to would be actively misleading, not merely an implementation detail leaking through.
- **Both clients belong to the same person** (two tabs open by one user, no "other viewer" in the social sense, `MULTIPLE_VIEWERS` case only): the copy still says "another viewer is connected" rather than trying to distinguish "another person" from "your own second tab" — the server has no reliable way to know these are the same human, and the instruction ("close other tabs or sessions") is correct and actionable regardless of who owns the other connection.
- **No path forward exists** (e.g., the second connection is a monitoring/automation client the user doesn't control, such as a backlog-automation session attaching via `ssq-mux`): the toast's instruction becomes technically un-followable by this user in this moment. This is accepted as a known limitation, not a dead end in the WCAG/Krug sense — the toast still tells the truth about *why*, which is the requirement (`research/ux.md`'s core complaint is silence, not that every block has a one-click fix). Recorded as a UX limitation alongside the plan's own Unresolved Question about whether single-client-only should be a permanent constraint.

---

## Surface 4 — Reached the hard limit (`AT_TOP`)

### Scope boundary (cross-artifact-consistency BLOCKER, resolved)
An earlier draft of this surface unified the `AT_TOP` affordance with the
pre-existing tmux-native "exhausted scrollback" case (`metadata.hasMore ===
false`) as one shared implementation, rendered identically regardless of
path. That contradicts `requirements.md`'s Risk Control section ("the
existing tmux-native pagination path is unchanged and remains the fallback
for every session this feature doesn't cover") and its Out-of-Scope line
("Rebuilding tmux-native scrollback delivery... out of scope") — it would
change visible behavior for every session, including ones with this
feature's flag off or running a non-Claude program, which never asked for a
"no more history" affordance and never had one before. **This surface is
scoped to the `AT_TOP` outcome only for Phase 1** (`plan.md`'s Task 1.4.4b,
`hasMoreAppScrollbackRef`, distinct from the tmux-native path's own
`hasMoreScrollbackRef`, which this feature does not read or write). The
wireframe, copy, and accessibility treatment below still apply — only the
"same component serves both paths" claim is withdrawn. Unifying the two is
recorded as a follow-up outside this feature's scope (`plan.md`, Task
1.4.4b's follow-up note), not abandoned, just not Phase 1's to do.

### Wireframe (desktop)

```
┌─────────────────────────────────────────────────────────────┐
│  ── No more history available ──                    (static,  │
│                                                        muted,   │
│  claude> This is the earliest message in this          no icon)│
│  session's Claude Code transcript.                              │
│  ...transcript content continues downward...                    │
│                                                                 │
└─────────────────────────────────────────────────────────────┘
```

### Wireframe (mobile — optional enhancement, not required for MVP)

```
     ╭─╮
     │ │  <- custom scrollbar thumb (scrollThumbRef) gets a
     ╰─╯     small rubber-band bounce animation on hitting the
             AT_TOP boundary, mirroring native iOS/Android
             overscroll bounce — research/ux.md §4d flags this
             as a mobile-specific opportunity, not a requirement
```

- Rendered as a thin static line at the current scroll-top position, `color:
  vars.color.textMuted`, no background pill, no icon, no `role="status"`
  live announcement on mount — this is a **calm, expected** boundary (the
  research doc's own framing: functionally identical to a plain shell
  hitting the top of real scrollback), not a novel event needing an
  interruption. It announces once via the existing `aria-live="polite"`
  region so a screen-reader user scrolling up gets *some* signal that
  further scroll attempts won't do anything (closing the exact "silent
  no-op" gap `research/ux.md` §0/§2 identifies as the core problem this
  whole feature exists to fix) — but it does not use a toast or a
  visually loud treatment, because "you've reached the end" is not an error
  or a policy block, it's just information.
- **Phase 1 scope** (Task 1.4.4b, see Scope boundary above): this component
  renders only when `hasMoreAppScrollbackRef.current` goes `false` via
  `AT_TOP`. A session that instead exhausts tmux-native scrollback
  (`metadata.hasMore === false`) does not render this affordance in Phase 1
  — that path's behavior is unchanged by this feature. `research/ux.md` §4b's
  original recommendation (a user should never be able to tell which path
  served them) remains the right long-term target and is exactly what the
  follow-up story unifying the two is for; it is just not built in Phase 1.

### Interaction flow

| Step | User action | System response |
|------|-------------|------------------|
| 1 | User scrolls up repeatedly until the app's own transcript has no more content to reveal (app-forwarded session, this feature's path) | Server returns `AT_TOP`; `hasMoreAppScrollbackRef.current` is set `false` and the "No more history available" line renders at the current top-of-viewport position |
| 1b | (For contrast, not this feature's behavior change) A plain-shell/flag-off/non-Claude session exhausts tmux-native scrollback (`metadata.hasMore=false`) | Unchanged from today: no affordance renders, `hasMoreScrollbackRef.current` is set `false` exactly as it already is — Phase 1 does not touch this path (see Scope boundary above) |
| 2 | User scrolls up again anyway (app-forwarded session) | No new request is sent (guarded by `hasMoreAppScrollbackRef`, the app-forwarded analog of the ref that already prevents redundant tmux-native fetches today) — scrolling simply stops producing new content, exactly like reaching the top of a plain shell's scrollback, which is the intentionally boring, well-understood terminal-UX convention this surface is matching |

### Error / edge cases
- **`AT_TOP` arrives mid-way through a page that also contains new content** (the redraw settled but is byte-identical to the last-captured frame, per Story 1.3.2's outcome computation): the content already delivered stays rendered; only the trailing "No more history" line is added — never a full-page replacement that could discard content already shown to the user.
- **False `AT_TOP` from `scrollForwardKeybindingCanary` ambiguity** (Surface 8): if the canary flags a redraw as ambiguous rather than confidently "scrolled," the server-side outcome computation (Story 1.3.2) still returns a definite outcome to the client — the canary is a maintainer-facing log signal, not a client-visible state, so this never surfaces a fourth "maybe" state to the user. Confirmed by plan's enum having exactly four values with no `Ambiguous` member.

---

## Surface 5 — No known scroll command (`NO_CAPABILITY`)

**Condensed** — per the plan (Story 1.4.4's acceptance criteria), this
outcome is not reachable in practice for Phase 1: `AppScrollGate` refuses to
attempt forwarding at all when `resolveScrollAdapter(program) == nil`, so
the server never sends `NO_CAPABILITY` for a genuinely unregistered program
today — it only exists as a forward-compatible enum value for a future
adapter registered with an explicitly unconfirmed capability.

Representative client handling (Task 1.4.4a):
```ts
case ScrollForwardOutcome.NO_CAPABILITY:
  // Falls back to the pre-existing silent no-op — today's baseline,
  // not a regression. No banner, no toast, no loading pill left showing.
  clearAppScrollbackLoadingState();
  break;
```

Acceptance criteria:
- Receiving `NO_CAPABILITY` never throws or leaves Surface 1's loading pill
  stuck showing — it is cleared unconditionally.
- No banner, toast, or "reached the top" line renders for this outcome —
  behavior is byte-for-byte identical to today's baseline for an
  unsupported program (this is a defensive branch, not a new user-facing
  affordance, and adding one here would contradict the plan's own scoping:
  Phase 3's agy adapter dropping out of scope entirely is an accepted
  outcome per requirements.md's Rabbit Holes).
- A session whose program is a genuinely unsupported agent CLI (e.g. agy,
  pre-Phase-3) behaves identically whether this feature's flag is on or
  off — verifiable by comparing scroll-up behavior with the flag toggled,
  per Story 1.5.1's flag-gating tests.

---

## Surface 6 — Feature flags (config)

**Condensed** — non-interactive from an end-user perspective; consumed via
the existing flag panel/RPC, not a bespoke UI.

Representative flag registration (Task 1.5.1a/b):
```go
const FeatureAppScrollForwardingClaude = "terminal:app-scrollback-forwarding:claude"
// + terminal:app-scrollback-forwarding:pi (Phase 2)
// + terminal:app-scrollback-forwarding:agy (Phase 3, not registered until researched)
```

Acceptance criteria:
- Each flag defaults to `false` and appears in the existing flag panel's
  list with a human-readable label distinguishing it from unrelated flags
  (e.g. "App scrollback forwarding — Claude Code") — reuses the panel's
  existing list/toggle UI, no new screen.
- Toggling a flag off mid-session degrades a session already mid-forward
  gracefully: an in-flight `ForwardScroll` call is unaffected (flag is
  checked once per request, at Task 1.5.1c's call site, not polled inside
  the orchestration), and the *next* scroll-up request after the toggle
  uses the tmux-native path with no client-visible error.
- Per-adapter scoping is visible in the panel — a user/operator can tell at
  a glance that Claude's flag and pi's flag are independent toggles, not one
  combined switch (directly satisfies requirements.md's "scoped per-adapter
  if practical" Risk Control ask).
- No new confirmation dialog or destructive-action warning is needed for
  toggling this flag — flipping it off is the intended, safe rollback path
  (per plan's Risk Control section), not a hazardous action.

---

## Surface 7 — Observability: logs & counters

**Condensed** — ops-facing, not end-user-facing.

Representative log line (Epic 1.5's Observability Plan):
```
level=warn msg="scroll adapter not registered for program" session_id=abc123 program=agy
level=info msg="scroll forward served" session_id=abc123 adapter=claude outcome=DELIVERED path=app_forwarded
```

Representative counter:
```
scroll_forward_attempts_total{adapter="claude",outcome="blocked"} 3
scroll_forward_capture_duration_ms{adapter="claude"} histogram
```

Acceptance criteria:
- Every `Instance.ForwardScroll` call produces exactly one `log.Info`/`Warn`
  line and one counter increment, labeled by adapter and outcome — no
  request is unaccounted for (satisfies requirements' "log which scrollback
  path served a given scroll-up request").
- A session whose program has no registered `ScrollAdapter` produces the
  coverage-gap warning at most once per session (not once per scroll
  attempt) — avoids log-flooding a long-running unattended session that a
  user repeatedly scrolls in.
- Metric labels use a bounded, known set of adapter names and outcome
  values (the `ScrollForwardOutcome` enum) — never a raw, unbounded program
  string — so the metric's cardinality can't grow unexpectedly as new
  program variants are launched.

---

## Surface 8 — `scrollForwardKeybindingCanary` warning

**Condensed** — ops-facing drift-detection signal, not end-user-facing (see
Surface 4's edge case above for why this never leaks into client state).

Representative log line (Story 1.5.2):
```
level=warn msg="scroll forward canary: redraw ambiguous, may indicate keybinding drift" \
  session_id=abc123 adapter=claude before_excerpt="..." after_excerpt="..."
```

Acceptance criteria:
- Fires only for the ambiguous case (redraw changed but doesn't look
  scroll-consistent) — never for a clean `DELIVERED` or a clean
  byte-identical `AT_TOP`, so its signal-to-noise ratio stays high enough
  for a maintainer to act on it (mirrors `compactingCanary`'s existing
  discipline).
- Logs at most once per session (guarded by `scrollForwardCanaryLogged`,
  same shape as `compactingCanaryLogged`) — a maintainer sees "this session
  hit drift" once, not a line per scroll gesture.
- The logged excerpt is truncated (not the full captured pane) — keeps log
  volume bounded per this repo's existing log-volume-reduction guidance
  (`docs/how-to/debug-with-logs.md`).

---

## Accessibility summary

| Surface | Mechanism | Notes |
|---------|-----------|-------|
| 1. Loading pill | `role="status"` + `aria-live="polite"`, reusing `XtermTerminal.tsx:1325`'s pattern | Announced once per fetch start, not per poll tick of `RedrawQuiescence` |
| 2. `ScrollSourceIndicator` | `role="status"` + `aria-live="polite"`, mount/unmount-only announcement | Icon `aria-hidden="true"`; text alone carries meaning |
| 3. Blocked toast | `role="status"` (not `alert`) + `aria-live="polite"`; `[Got it]` button keyboard-focusable and dismissible with Enter/Space | `ConnectionCountIndicator`'s pulse is `aria-hidden` (purely visual reinforcement; the toast text already carries the full explanation) |
| 4. "No more history" line | `aria-live="polite"`, single announcement on the `false` transition, silent afterward | No live-region spam on repeated scroll attempts past the limit |
| 3/2 combined | Every new banner/toast/indicator this feature adds reuses the **existing** `visuallyHidden` + `aria-live="polite"` pattern (`web-app/src/styles/a11y.css.ts`, already used by `ReviewQueuePanel`/`TriggersPanel`/`TriggerFormModal`/`CallbackSettings` and precedented in this exact component) — no parallel announcer mechanism is introduced anywhere in this design | Per the task brief's explicit instruction |
| Keyboard | The scroll-up gesture itself is DOM-scroll-position-triggered (wheel, trackpad, touch-drag on the custom thumb), not a new keyboard shortcut — this feature adds no new keyboard-only interaction surface. The Blocked toast's `[Got it]` dismiss button is the one new focusable element this design introduces, and it must be independently tab-reachable per WCAG 2.1 SC 2.1.1, following the existing `Scroll to bottom` button's precedent (`XtermTerminal.tsx:1573-1578`) | If a future iteration adds a toolbar "Load older history" button (mentioned as a fallback affordance in research/ux.md §3 for cases where passive scroll-detection proves ambiguous), it must be independently keyboard-operable the same way |
| Color contrast | Blocked toast: `vars.color.warningBg`/`warningText`. Loading pill / `ScrollSourceIndicator`: `vars.color.modalBackground`/`textPrimary` (matches `reconnectingBanner`, already shipped and presumably already contrast-checked). "No more history" line: `vars.color.textMuted` — **must be verified ≥ 4.5:1 against the terminal background in both light and dark theme** before ship, since `textMuted` is used here on a plain terminal background rather than inside a pill with its own background, which is a new usage context for that token | Flagged as a pre-ship check, not assumed |
| Canvas/a11y-tree boundary | Per `research/ux.md` §3: forwarded transcript *content* flows through xterm.js's existing accessibility-tree mirror automatically (it's just terminal buffer content from xterm's point of view). Only the four chrome affordances above (pill, banner, toast, "no more history" line) live outside that mirror and needed explicit ARIA treatment in this design — confirmed no fifth mechanism was needed | — |

---

## UX Acceptance Criteria

**Task completion**
- UX-AC-1: A user whose scroll-up succeeds sees the outcome (banner, "no
  more history" line, or blocked toast) within one scroll gesture — no
  additional click/tap is required to reveal what happened. (0 extra steps
  beyond the scroll gesture itself.)
- UX-AC-2: A user who wants to return from viewing forwarded history to the
  live session view can do so with the same single gesture (scroll down)
  that already returns to live in the tmux-native path — no new "exit
  history mode" button is required, and none is added.
- UX-AC-3: A user blocked by a second connection can reach a working scroll
  in ≤ 2 steps once the other connection is actually closed: (1) close the
  other tab/session, (2) scroll up again. No page reload, no manual
  "retry" button click is required — the very next scroll gesture just
  works.

**Error states**
- UX-AC-4: The `Blocked` state shows the message matching its actual
  `ScrollBlockedReason` — never a one-size-fits-all string. `MULTIPLE_VIEWERS`
  shows "Can't browse `<Program>`'s history right now — another viewer is
  connected. Close other tabs or sessions viewing this session to enable it."
  (matching Story 1.4.3's exact wording, extended with an actionable next
  step) and is paired with the pre-existing `ConnectionCountIndicator`
  pulsing at the same moment, giving the user both the explanation and the
  standing signal that confirms when the block will clear.
  `UNSUPPORTED_STREAMING_PATH` shows "Scroll-forwarding isn't available for
  this session yet" with no viewer-count claim and no badge pulse — a solo
  user on `PathLegacyPerConnection` must never see the `MULTIPLE_VIEWERS`
  copy (adversarial-review BLOCKER; regression-tested by Task 1.4.3b).
- UX-AC-5: The stalled-loading case (Surface 1, step 5) offers an explicit
  **Cancel** action after 8s — no state in this design leaves the user
  staring at an indefinite spinner with no way out.
- UX-AC-6: No error state in this design is silent. Every one of the four
  `ScrollForwardOutcome` values produces a distinguishable, honest signal to
  the user except `NO_CAPABILITY`, which is explicitly scoped (Surface 5) to
  match today's pre-existing baseline rather than invent new copy for a case
  the plan says should not occur in practice for Phase 1 — this is a
  deliberate, documented exception, not an oversight.

**No dead ends**
- UX-AC-7: Every surface in this document that represents a stopping point
  (Blocked toast, "no more history" line, stalled-loading Cancel) has a
  concrete next action available to the user: retry after closing another
  tab (Blocked), scroll back down to return to live (all surfaces), or
  cancel and return to the live view (stalled loading). None require a page
  reload or a support request to escape.
- UX-AC-8: The one surface with a real, acknowledged limitation (Blocked,
  when the other connection is not user-controllable) still tells the truth
  about *why* rather than presenting a false "retry" affordance that can
  never succeed — recorded explicitly as a known limitation, not hidden
  behind misleading copy.

**Accessibility**
- UX-AC-9: Every new status surface (pill, banner, toast, "no more history"
  line) is reachable by a screen reader via the existing
  `visuallyHidden`+`aria-live="polite"` pattern already used in this exact
  component (`XtermTerminal.tsx:1325`) — verified by a screen-reader smoke
  test (VoiceOver or NVDA) confirming each of the four outcomes is announced
  exactly once per transition, not zero times and not repeatedly.
- UX-AC-10: The one new focusable element this design introduces (the
  Blocked toast's `[Got it]` dismiss button) is keyboard-reachable via Tab
  and activatable via Enter/Space, verified by keyboard-only navigation
  through a session with the flag enabled and a second tab open.
- UX-AC-11: All four new/modified text surfaces meet ≥ 4.5:1 contrast
  against their background in both light and dark theme, verified against
  `web-app/src/styles/theme-contract.css.ts`'s token values before ship —
  explicitly including the `textMuted`-on-plain-terminal-background usage
  flagged above as a new usage context for that token.
- UX-AC-12: No surface in this design relies on color alone to convey
  meaning (Blocked uses both a distinct warning color *and* explanatory
  text *and* the pulsing badge; "no more history" uses text, not just a
  muted-color line with no label).

---

## Cross-cutting notes for implementation

- **Mobile vs. desktop trigger, corrected**: an earlier draft of this
  document (citing research/ux.md §4d) claimed the scroll *trigger* is
  identical on both platforms (DOM scroll-position based) for every session.
  That claim held for **plain-shell** sessions but not for **alt-screen**
  ones — `research/architecture.md` §3 found the DOM `scroll` event may never
  fire at all for an alt-screen pane, which is exactly the case this feature
  targets. The plan's Story 1.4.0 (added during architecture review) resolves
  this with a separate trigger: a `wheel` listener for desktop, and a new
  outcome inside `useTerminalGestures`'s existing `SCROLLING` touch state for
  mobile — not a new touch listener (see the next bullet). Every surface
  above (loading pill, banner, toast, "no more history" line) still renders
  identically regardless of which trigger fired; only the *triggering
  condition* differs by session type, not the presentation. The one
  remaining mobile-specific visual opportunity (Surface 4's scrollbar-thumb
  bounce) is unaffected and stays optional — if in doubt, ship Surface 4's
  static line identically on both platforms and skip the bounce animation.
- **Do not add a third touchmove handler.** Restated from research/ux.md §0
  and honored by Story 1.4.0's design (it extends `useTerminalGestures`'s
  existing `SCROLLING` state with a new outcome, rather than registering a
  second `touchstart`/`touchmove` pair) — this remains the single most
  tempting wrong turn for anyone extending Surface 4's optional mobile
  bounce or any other touch-adjacent affordance: hook into the existing
  gesture/scroll-position signal, never a new `touchmove` listener.
- **Copy strings are load-bearing for tests.** Story 1.4.2/1.4.3/1.4.4's
  acceptance criteria quote exact banner/toast text — this document's
  wireframe copy matches those quotes verbatim so implementation and test
  assertions don't drift from each other.
