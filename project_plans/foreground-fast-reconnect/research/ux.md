# Research: UX Value of Foreground Fast Reconnect

Scope: is a shorter connect timeout for the terminal the user just switched to
(1200-1500ms vs 3500ms) a change the user will actually perceive, given what
this app's UI already does while a terminal is disconnected/reconnecting.

## 1. Is 1200ms vs 3500ms perceptible?

Nielsen's three response-time limits (Nielsen Norman Group, "Response Times:
The 3 Important Limits" — grounded in Miller 1968 / Card, Robertson & Newell
1991, the standard citation for this threshold set):

- **0.1s** — feels instantaneous, no feedback needed.
- **1.0s** — the limit for the user's flow of thought staying uninterrupted;
  beyond this the user notices the delay, though no progress indicator is
  strictly required up to this point.
- **10s** — the limit for keeping the user's attention on the task at all;
  beyond this users context-switch away. A progress indicator is required for
  anything in the 1s-10s band.

Both proposed values (1200-1500ms *and* 3500ms) are **past the 1.0s
threshold** — neither reads as "instant." Both fall in the band where a
progress indicator is expected and where the user consciously registers "the
computer is doing something." The proposed change is a difference of degree
*within* the same perceptual tier (both "noticeable, needs a spinner"), not a
difference of category (e.g. "instant" vs "noticeable"). The 2s+ savings is
real relative to the alternative, but it does not cross the line into feeling
instantaneous — a user going from 3.5s to 1.3s still perceives a pause, just a
shorter one.

**Important architectural caveat that changes what this number even measures**:
nothing in the current codebase implements a "connect timeout" concept today.
`web-app/src/lib/utils/backoff.ts`'s `BackoffState`/`jitteredDelay` control the
**delay before the next retry attempt**, not how long an in-flight attempt is
allowed to hang before being aborted. `useTerminalStream`'s `connect()`
(`web-app/src/lib/hooks/useTerminalStream.ts:162-361`) has no timeout at all —
it awaits the WebSocket stream indefinitely until it either receives a
message, errors, or the socket closes. So "connect timeout" as scoped by the
requirements is a wholly new mechanism, not a tightened version of something
that exists. In the common case (server reachable, WS handshake completes in
tens of milliseconds), a connect-timeout of 1200ms vs 3500ms changes *nothing
observable* — the handshake finishes well under either value. The timeout only
does work when the network/server is degraded enough that the handshake is
genuinely hanging, in which case the user-facing win is "gives up and retries
sooner," not "connects faster." This narrows the population of users who would
ever notice the foreground-vs-background distinction to those hitting a slow
or hanging handshake — the acceptance criteria should be read as "how fast do
we abandon a stalled foreground attempt," not "how fast is a normal foreground
reconnect."

## 2. What does the user actually see today while disconnected/reconnecting?

Traced through `web-app/src/components/sessions/TerminalOutput.tsx`:

- **Terminal content is not cleared on disconnect.** The only `terminal.clear()`
  call (`TerminalOutput.tsx:1272`) is wired to the manual "Clear" toolbar
  button, not to any connection-state transition. When a terminal disconnects,
  the last-rendered scrollback stays exactly as-is on screen — there is no
  blank/frozen-spinner state to "fix" by reconnecting faster.
- **Status text updates immediately** (`TerminalOutput.tsx:1389-1396`): the
  toolbar's status dot/text flips from "Connected" to "Disconnected" the
  instant `isConnected` goes false — this is synchronous with the WS close
  event, unaffected by any connect-timeout value.
- **The "Reconnecting terminal…" banner is gated behind its own fixed 2000ms
  timer**, independent of backoff/connect-timeout values
  (`TerminalOutput.tsx:783-809`, "Story 3.2.1"): `bannerTimerRef` only flips
  `showReconnectBanner` true after 2s of continuous disconnection. This means:
  - **In the foreground-fast case (~1200-1500ms target), the whole
    reconnect cycle can complete *before this banner ever appears***. A
    foreground reconnect that succeeds within 1.5s would show only the
    momentary "Disconnected" status-text flip, then flip straight back to
    "Connected" — no banner, no visible spinner overlay, no user-facing
    "reconnecting" state at all.
  - In the background/slow case (~3500ms), the 2s banner *does* fire and is
    visible for roughly the last 1.5s of the reconnect.
  - This reframes the actual visible benefit: it isn't "spinner shows for
    1.3s instead of 3.5s," it's closer to **"no reconnecting UI appears at
    all" vs "a 'Reconnecting terminal…' banner briefly appears."** That is a
    legitimate, concrete UX improvement (less flicker/less alarming UI on the
    session the user is actively watching), but it is a different claim than
    "the user perceives a faster reconnect" — the user may not perceive a
    reconnect happened at all in the fast path.
- **A manual "🔄 Reconnect" button only appears after 5000ms**
  (`TerminalOutput.tsx:764-770`, and only on the legacy/V2-off path — see
  §4), well outside either proposed timeout value, so it's not affected by
  this change either way.
- **The loading overlay/spinner** (`TerminalOutput.tsx:1663-1670`,
  `loadingOverlay`/`loadingSpinner`) is gated on `isLoadingInitialContent`,
  which is an initial-mount/session-switch concept, not a
  disconnect/reconnect concept — it does not re-trigger on a mid-session
  disconnect (confirmed no `setIsLoadingInitialContent(true)` on the
  `wasConnected && !isConnected` transition at `TerminalOutput.tsx:759-770`;
  that block explicitly sets it to `false` so the user sees frozen content +
  "Disconnected" text instead of a stuck spinner).

Net: today's reconnect UX is "status text flips, content freezes in place,
and — only past 2s — a small banner appears." A faster foreground reconnect
mostly acts on that 2s banner's visibility, not on any spinner/loading state
the user is staring at, because there isn't one for this scenario.

## 3. Accessibility

No accessibility interaction found, and none expected. The reconnect banner
has no `aria-live`/`role="status"` wiring visible in the excerpt reviewed
(`TerminalOutput.tsx:1653-1661` — plain `<div>`s), unlike the resizing overlay
which does use `role="status" aria-label="Terminal resizing"`
(`TerminalOutput.tsx:1681-1688`). A screen reader user gets no announcement
either way, at either timeout value — so a shorter connect timeout changes
nothing for that population, and (out of scope for this feature, but adjacent)
the existing reconnect banner is itself not screen-reader-announced regardless
of speed.

## 4. Job to be done, and is it already served

The plausible JTBD here is: *"don't make me wait when I just switched to the
terminal I actually care about right now."* Two things push back on whether
this feature is the mechanism that delivers that job today:

- **The existing UI already mostly serves this job by not showing the user
  anything is wrong** (§2) — the frozen scrollback plus a same-instant status
  flip is a low-drama disconnect experience already. The 2s-gated banner is
  the one thing that would visibly interrupt the user, and (per §2) a
  sub-1.5s foreground reconnect already dodges it under the *current* 2000ms
  banner threshold, with no change needed.
- **The feature may not be reachable by default users at all.** Reconnect
  logic is split across two parallel, mutually-exclusive code paths gated on
  `NEXT_PUBLIC_RECONNECT_V2` (default OFF per the requirements doc's own
  "Current state" section, confirmed by the flag checks at
  `useTerminalStream.ts:331` and `TerminalOutput.tsx:862`):
  - **V2 ON** (opt-in): `useTerminalStream`'s own `terminalBackoffRef`
    (`BackoffState(1000, 30_000)`) drives reconnection
    (`useTerminalStream.ts:331-352`), plus a separate visibility-triggered
    reconnect listener (`useTerminalStream.ts:433-458`, "Story 3.1.3").
  - **V2 OFF** (default): `useTerminalStream`'s hook-level retry loop never
    runs (guarded by the same flag) — reconnection instead comes entirely
    from `TerminalOutput.tsx`'s own effect (`TerminalOutput.tsx:860-874`,
    "Auto-reconnect with exponential backoff"), which uses a different,
    ad-hoc backoff formula (`Math.min(1000 * 2^(attempts-1), 10000)`) that
    doesn't touch `BackoffState`/`terminalBackoffRef` at all.

  A connect-timeout implemented only inside `useTerminalStream`'s hook-level
  backoff path would be **invisible to default-configuration users**, since
  that path doesn't execute unless `NEXT_PUBLIC_RECONNECT_V2=true`. This is
  the concrete reason acceptance criterion 5 ("plan must state explicitly
  which reconnect path(s) this applies to") matters here — whoever writes the
  implementation plan needs to either target both paths, or state plainly
  that the feature ships dark until V2 is flipped on, or make flipping on V2
  part of this change. Confidence: VERIFIED by reading the flag checks
  directly (paths above); not verified whether V2 has been enabled in
  production config elsewhere (out of scope for this file — a plan-phase
  question, not a UX one).

**Bottom line on JTBD**: the job ("don't make me wait on the terminal I care
about") is real, but the current implementation of "waiting" that the user
experiences is dominated by (a) the 2s banner-delay threshold and (b) which of
two independent reconnect code paths is even active — not by the underlying
per-attempt connect latency this feature targets. The connect-timeout change
is a legitimate, low-risk refinement, but its user-visible payoff is smaller
and different in kind than "feels 2.3s faster": mostly it avoids a
transient banner from appearing for a class of quick, in-view reconnects, and
only actually shortens a *stalled* handshake case — provided it lands on the
code path the user's build is actually running.
