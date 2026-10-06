# UX Research: Async Session Creation (Creating → Running/Failed)

Scope: the frontend/UX side of `project_plans/async-session-creation/requirements.md`. Companion to backend/architecture research in the same directory.

## 0. Baseline: what SessionCard.tsx already does

`web-app/src/components/sessions/SessionCard.tsx` already renders a `SESSION_STATUS_CREATING`
state — this is the extension point, not a greenfield design.

- `isCreating` derived at `SessionCard.tsx:235` from `session.status === SessionStatus.CREATING`.
- Status pill: `getStatusColor` (`SessionCard.tsx:258-281`) maps `CREATING` to `statusLoading`
  (the same color token as legacy `LOADING`); `getStatusText` (`SessionCard.tsx:283-306`) renders
  the label `"Starting…"`.
- Progress row (`SessionCard.tsx:955-961`): a `creationSpinner` span (`aria-hidden`) plus the raw
  `session.creationProgress` text, falling back to `"Starting session..."`. Rendered only
  `isCreating && (...)`.
- Accessibility (`SessionCard.tsx:951-954`): a **separate**, visually-hidden
  `role="status" aria-live="polite"` span, always present in the DOM (not conditionally mounted),
  mirroring the same text. The comment explains why: NVDA and other screen readers only announce
  content *changes* inside a live region that was already in the DOM before the change — a region
  that mounts and immediately contains text is not guaranteed to be announced. This the correct,
  already-solved pattern to reuse for Failed-state announcements (do not reinvent).
- Visuals: `creationSpinner` (`SessionCard.css.ts:929-944`) is a 14px CSS border-spin animation,
  gated behind `@media (prefers-reduced-motion: no-preference)` — reduced-motion users get a
  static ring, not a spin. `statusCrashed` already exists as a color token
  (`SessionCard.tsx:276-277`) for the `CRASHED` status, which is the closest existing visual analog
  to the new `Failed` state (see §6).
- No status enum value for "Failed" exists yet in `proto/session/v1/types.proto` — the closest
  existing analog is `SESSION_STATUS_CRASHED = 10` (types.proto line 392), which today only
  applies to a session that died *after* running, not one that never started. Whether the new
  state is a genuinely new `SESSION_STATUS_FAILED` or a repurposed `CRASHED` semantic is an open
  question already flagged in requirements.md — from a pure UX standpoint they should look
  *different* (a session that ran and crashed carries salvageable output/worktree state a user
  may want to inspect; a session that never got past resolution has nothing to inspect, only
  retry/cancel), so a distinct visual state is recommended regardless of the backend enum choice.
- Toast infrastructure: `web-app/src/components/ui/NotificationToast.tsx` +
  `web-app/src/lib/contexts/NotificationContext.tsx` +
  `web-app/src/lib/notification-policy.ts` (`toastAutoCloseMs`, `toastAutoMinimizeMs`) is the
  existing, centralized toast system — auto-close/auto-minimize timing is explicitly centralized
  there per the comment at `NotificationToast.tsx:59-65` ("do not add dismissal logic here"). The
  failure toast required by requirements.md should be a new `NotificationData` variant routed
  through this system, not a bespoke one-off toast component.

**Net baseline assessment**: the Creating-state groundwork (spinner, progress text, live region,
reduced-motion handling) is already good and matches most of what the patterns below recommend.
The gap is entirely in the *Failed* state and its actions (retry/cancel), which don't exist yet.

## 1. Comparable patterns: async job/task creation with optimistic UI + live status

The shape of this problem — "user takes an action, a placeholder appears instantly, a background
process advances it through states, and it can fail" — is extremely well-trodden. Patterns that
consistently work well, and why:

- **GitHub Actions / CI run lists.** A workflow run appears in the list the instant it's queued,
  with a distinct "queued" (grey dot) → "in progress" (yellow spinning ring) → "success" (green
  check) / "failure" (red X) progression, each with its own icon *and* color (never color alone).
  Clicking a queued/in-progress run shows live-updating step-by-step log output — the equivalent
  of this feature's `creation_progress` phase text. Why it works: the user never has to guess
  whether "queued" means "about to start" or "stuck" — the spinner is a distinct icon from the
  static queued dot, so motion itself communicates liveness.
- **Cloud console resource provisioning** (AWS EC2 launch, GCP Cloud Run deploy, Vercel deploy).
  The resource row appears immediately in a "Pending"/"Provisioning" state with a spinner,
  optionally an inline sub-status ("Pulling image...", "Starting instance..." — directly
  analogous to `creation_progress`). A failed provision becomes a persistent red/error row with
  the failure reason inline and a "Retry" action *in the same row*, not requiring the user to
  re-open the original creation form. Vercel specifically keeps the failed deployment visible
  (not removed from the list) so the user has a record of what was attempted — directly relevant
  to this feature's requirement that the Failed card persists after the toast dismisses.
- **IDE background task indicators** (VS Code's "Installing extension...", JetBrains' background
  task progress in the status bar + notification). These favor a lightweight, low-chrome
  indicator for the *in-progress* state (spinner + one line of text, no modal, no blocking) but
  escalate to a more attention-grabbing surface (a toast/notification banner) only at *terminal*
  states — success is often silent or a subtle toast, but failure is always an explicit,
  dismissible notification with an action. This maps directly onto requirements.md's design:
  Creating is quiet (inline spinner in the list), Failed is loud (toast at the moment of failure)
  but also durable (persists on the card after the toast goes away).
- **Optimistic UI + reconciliation (general pattern, e.g. Linear, Notion).** The UI shows the
  user's intent immediately (item appears in the list) before the server confirms it, then
  reconciles to the server's authoritative state. The critical UX rule these products follow:
  the placeholder must be visually distinguishable from a "real" item (dimmed, spinner overlay,
  or explicit status badge) so the user is never confused about whether an action fully
  succeeded — this repo's `SESSION_STATUS_CREATING` pill + spinner already satisfies this.

**Common thread across all of these**: (a) instant list placement, (b) a distinct in-progress
visual (icon + motion, not just text), (c) inline sub-status text that updates in place rather
than replacing the whole row, (d) a terminal-failure state that is *persistent* in the list and
carries its own retry affordance, and (e) transient notifications (toasts) reserved for the
*moment* something changes, never as the sole record of it.

## 2. User mental models and expectations for a failable "Creating" state

What a user brings to this interaction, based on the patterns above and general async-UI research
(Nielsen Norman Group's guidance on system status visibility, progress indicators):

- **"Something is happening" must be visible within ~1 second of the action**, or the user
  assumes the click didn't register and will click again (see §4, double-submit). This is exactly
  requirements.md's success metric of the card appearing within ~1s.
- **A spinner alone reads as "please wait," not "here's what's happening."** Users tolerate
  indeterminate waits far better when given *phase* text (`creation_progress`: "Resolving GitHub
  URL...", "Cloning repository...") because it converts an opaque wait into a legible sequence —
  the user can distinguish "still working, on track" from "stuck on the same phase for a long
  time" (a signal power users will use to manually judge staleness even before an automated
  stale-detector fires).
- **"Failed" must not look like a dead end.** Once a user sees an error, their next question is
  "can I fix this and try again, or do I have to start over?" A Failed card with only an error
  message and no actions forces the user to mentally re-derive the original inputs (repo URL,
  branch, etc.) and re-enter the whole omnibar flow — exactly the friction requirements.md's
  Retry scope item exists to remove. Retry must be visually adjacent to the error, not buried in
  a menu.
- **Cancel is a distinct intent from Retry-after-failure and must be offered *while* Creating is
  still in progress**, not only after it fails. A user who realizes they made a mistake (wrong
  repo, wrong branch) mid-clone wants to stop it immediately, not wait out a 150s timeout to then
  retry with corrected inputs. This is the "stuck creation" case named in the requirements
  baseline ("There is no way to cancel a stuck creation").
- **Users read "stale/orphaned" failures as a system problem, not a user error**, and expect the
  messaging to say so plainly ("This session creation appears to have stalled" rather than a raw
  timeout/error string) — otherwise they'll assume they did something wrong and hesitate to retry
  when retry is in fact exactly the right action.
- **A toast is a *notice*, not a *record*.** Users expect toasts to be transient and don't expect
  to have to act on one before it disappears — if the toast is the *only* place the error and
  retry action live, a user who's looked away when it fires (a common case for background/async
  work) loses the information entirely. This is exactly why requirements.md specifies both a
  toast *and* a persistent Failed card state.

## 3. Accessibility requirements

- **WCAG 2.2 SC 4.1.3 (Status Messages, Level AA)** is the controlling criterion: a status change
  that doesn't shift keyboard focus must still be programmatically determinable via `role="status"`
  (polite) or `role="alert"` (assertive), so assistive tech announces it without the user needing
  to navigate to it. The existing `SessionCard.tsx:951-954` `role="status" aria-live="polite"` span
  for Creating-progress is the correct baseline pattern — it must be **extended**, not replaced,
  for the transition into Failed: the same persistent live-region element should have its text
  content updated to the failure message when status flips, so the DOM-mutation-based announcement
  behavior (content changes in an already-mounted region) keeps working. Do not mount a *new*,
  separate live region for Failed — that risks the same "mounts with text already present" gap the
  existing comment warns against.
- **Failure severity: `aria-live="assertive"` (or `role="alert"`) is more appropriate than
  `"polite"` for the Failed transition specifically.** A polite region waits for the screen reader
  to finish its current speech before announcing; a failure that the user should act on (retry
  decision) is time-sensitive enough to justify an assertive interruption, unlike routine "now
  cloning..." progress chatter, which should stay polite so it doesn't interrupt the user
  mid-task for every phase change. Recommend: keep the existing polite region for in-progress
  phase text, but use a *second*, assertive announcement (or toggle the existing region's
  `aria-live` value at the moment of failure — supported by all major screen readers when done via
  attribute mutation, not remount) specifically for the terminal Failed message.
- **Icon + color, never color alone (WCAG 1.4.1).** The existing `statusLoading`/`statusCrashed`
  color-token pattern (`SessionCard.tsx:258-281`) must pair the new Failed status with a distinct
  icon (e.g. an error/warning glyph) in addition to a distinct color — colorblind users and
  high-contrast-mode users need the icon shape to disambiguate Failed from, say, Paused or
  Stopped, which may render as visually similar hues under some OS accessibility themes.
- **Reduced motion (WCAG 2.3.3 / `prefers-reduced-motion`)**: already correctly handled for the
  spinner (`SessionCard.css.ts:936-943` gates the animation behind
  `@media (prefers-reduced-motion: no-preference)`). Any new Failed-state visual (e.g. a pulsing
  error icon) must follow the same gating — don't introduce a second unguarded animation.
- **Keyboard access for Retry/Cancel actions.** Both must be real `<button>` elements (not
  clickable `<div>`s) reachable via standard Tab order, with visible focus rings consistent with
  the rest of the card's interactive elements (see the existing snapshot-toggle button's
  `aria-expanded`/`aria-label` pattern at `SessionCard.tsx:966-977` as the precedent to match).
  Each needs an unambiguous accessible name — `aria-label="Retry creating session"` /
  `aria-label="Cancel session creation"` rather than bare "Retry"/"Cancel" text that's ambiguous
  out of visual context when read by a screen reader navigating by button list.
  Destructive-ish actions (Cancel discards in-progress work) should get the same confirmation
  affordance already used for session deletion (`isDeleting` state pattern at
  `SessionCard.tsx:221`) if a two-step confirm is judged necessary — though for Cancel-during-
  Creating specifically, a lower-friction single click may be preferable since the "damage" is
  just an aborted clone, not data loss (product decision, not purely an a11y one).
- **Focus management on state transition.** When a card the user is actively watching transitions
  from Creating to Failed, do not steal focus away from wherever the user currently is (e.g. if
  they're mid-typing in the omnibar for a *different* session) — the live region announcement
  handles notification without requiring a focus shift, consistent with SC 4.1.3's intent.

## 4. Error states and edge cases needing graceful UX

- **Stale/orphaned creation** (server restarted mid-goroutine, or resolution genuinely hung past
  threshold). UX must distinguish this from a "normal" failure: label it distinctly (e.g. "Creation
  timed out" vs. a specific clone/auth error) so the user doesn't waste time trying to interpret a
  stack-trace-shaped message for what is actually a systemic timeout. Because this can be detected
  well after the user has stopped watching (they may have closed the tab), the persistent Failed
  card is the *only* record they'll see — the toast will have long since fired and gone, if it
  fired at all while they were present. This reinforces requirements.md's design of persistent
  card state as the durable source of truth, toast as best-effort immediate notice only.
- **Rapid double-submit** (user clicks Create twice, e.g. because the first click's feedback
  wasn't visible fast enough — see §2's ~1s expectation). The omnibar/dialog's own submit button
  should disable immediately on click (standard idempotent-submit-guard pattern) so this is
  prevented at the source; the session list should never need to de-duplicate two Creating cards
  for what the user intended as one action. Confirm the RPC itself doesn't provide accidental
  idempotency on title alone in a way that would surface a confusing "duplicate title" fast-fail
  error to a user who only clicked once but the client double-fired the request (a client bug that
  would look like a backend bug from the user's seat).
- **Cancel racing with success** (user clicks Cancel just as background resolution completes and
  flips to Running). This must resolve deterministically and communicate the *actual* outcome to
  the user, not a stale one: if cancel loses the race, the card should show Running (not a
  confusing flash of "Cancelled" immediately followed by "Running"); if cancel wins, the resources
  should be fully torn down and the card should reflect a clean "Cancelled" (or removed) state, not
  a lingering Failed with a misleading error message about a clone that was intentionally aborted.
  A user-initiated cancel and a system-detected failure should never look identical in the UI —
  conflating them would make the user think something broke when they in fact chose to stop it.
- **Retry-in-place vs. duplicate.** From the user's perspective, clicking Retry must visibly
  reuse the *same* card/row (same position in the list, same identity) — if retry created a
  second, separate Creating card, the user would reasonably interpret that as a bug ("didn't I
  already have one of these?"). The card transitioning Failed → Creating → Running in place is the
  correct visible behavior, mirroring the requirements.md open question about whether retry needs
  a new `SessionCreatedEvent` (from a pure UX contract standpoint, it should not visibly re-appear
  as a "new" item).
- **Retrying repeatedly into the same failure** (e.g. bad credentials that will never resolve).
  Consider whether the UI should surface a hint after N consecutive retries on the same card
  ("still failing — check your GitHub credentials?") rather than silently allowing an infinite
  retry loop with no escalation; not explicitly in scope per requirements.md but worth flagging
  as a light-touch addition if cheap.
- **Toast dismissed before the user reads it** (auto-close per `toastAutoCloseMs` in
  `notification-policy.ts`). Since the persistent card is the durable fallback, this is already
  handled by design — but confirm the failure toast's auto-close timing is generous enough (or the
  failure toast type is configured for longer/no auto-close, similar to how approval-request
  toasts likely differ from routine ones) given the higher stakes of a failure notice vs. a routine
  status update.

## 5. Jobs-to-be-done: what immediate visible feedback fulfills

The user's own words (paraphrased from requirements.md's Baseline/Problem Statement): *"I'd
prefer a more graceful status... show in the list in a creating status with the right icon/spinner
to denote it's still not ready."*

- **Functional job**: "Let me keep working while this session sets itself up, and let me tell at a
  glance which of my sessions are usable right now vs. still coming online." The card list is
  already the user's primary at-a-glance dashboard for session state (that's why grouping by
  Status is one of the 8 organization strategies per `.claude/docs/tag-organization.md`) — a
  Creating session that's invisible until fully resolved breaks that dashboard's core promise for
  exactly the slow-path case (GHE clones) where the dashboard function matters most.
- **Functional job (secondary)**: "When something goes wrong, let me retry or clean it up without
  redoing all my input." This is the Retry/Cancel scope — converting a failure from a full restart
  into a one-click recovery.
- **Emotional job**: "Don't make me wonder if the app is broken." The current baseline — a frozen
  dialog with a static "Creating…" button label and total silence from the session list — reads as
  a hang, not a working system, because nothing visibly changes for up to 150 seconds. Immediate
  list placement + a moving spinner + advancing phase text converts an anxiety-inducing silence
  into legible, trust-building progress — this is the single biggest emotional-job payoff of the
  whole feature, independent of actual latency (even before the backend SLO drops to <500ms p99,
  *perceived* responsiveness improves the moment the omnibar stops blocking).
- **Emotional job (failure path)**: "Tell me plainly what happened and that it's not something I
  broke" (especially for stale/orphaned failures, which are a system issue, not user error) — and
  "don't make me feel punished for the failure by losing my inputs," which Retry-in-place directly
  addresses.
- **Social/collaborative job**: less central here since this is a single-user local dev tool (per
  requirements.md's Non-functional Requirements: "single-user-per-instance"), but still relevant
  in the sense that a user working across multiple concurrent sessions (a stated core workflow of
  this app) needs the list to be a trustworthy shared record of "what's actually running" they can
  glance at while attention is on a *different* session's terminal — the live region / toast
  distinction (announce once, persist visibly) exists precisely so a user who wasn't looking at
  this exact card when it changed still gets the information when they do look.

## 6. Recommendations for extending SessionCard.tsx (design consistency)

To keep the new Failed/Retry/Cancel states feeling like a natural extension of the existing
Creating treatment rather than a bolted-on addition:

1. **Reuse the color-token + label pattern exactly**: add a `SessionStatus.FAILED` (or whatever
   the chosen enum value is) case to both `getStatusColor` and `getStatusText`
   (`SessionCard.tsx:258-306`), following the existing switch-statement shape. Do not reuse
   `statusCrashed` for this — introduce a distinct token (e.g. `statusCreationFailed`) even if
   visually similar, since crashed-after-running and failed-before-running are different enough
   states that a future design pass may want to differentiate them, and reusing the token now
   would make that harder to discover later.
2. **Extend, don't duplicate, the live-region span** (`SessionCard.tsx:951-954`): keep it mounted
   unconditionally, and change its content/`aria-live` value based on status rather than adding a
   second region.
3. **Mirror the spinner's structure for a static failure icon**: same size/position as
   `creationSpinner` (`SessionCard.css.ts:929-944`) so the row's layout doesn't jump between
   Creating and Failed, just swap the spinning ring for a static warning glyph.
4. **Place Retry/Cancel where the snapshot-toggle button already sets precedent**
   (`SessionCard.tsx:966-977`): a real `<button>` with `aria-label`, positioned in the same
   progress-row area the spinner/text currently occupies (`SessionCard.tsx:955-961`), so the
   row's real estate is reused rather than adding new chrome elsewhere on the card.
5. **Route the failure toast through the existing `NotificationToast`/`NotificationContext`
   system** (`web-app/src/lib/contexts/NotificationContext.tsx`,
   `web-app/src/lib/notification-policy.ts`) as a new `NotificationData`/notification-type variant,
   rather than a bespoke toast — this gets auto-close/auto-minimize timing, styling, and dismissal
   behavior for free and consistent with every other toast in the app.

## Sources

- Direct code inspection (cited inline above) —
  `web-app/src/components/sessions/SessionCard.tsx`,
  `web-app/src/components/sessions/SessionCard.css.ts`,
  `web-app/src/components/ui/NotificationToast.tsx`,
  `proto/session/v1/types.proto`.
- `project_plans/async-session-creation/requirements.md` (problem statement, baseline, scope,
  user's own words in the Problem Statement/Baseline sections).
- General UX patterns: Nielsen Norman Group heuristic "Visibility of System Status"; WCAG 2.2
  Success Criteria 4.1.3 (Status Messages), 1.4.1 (Use of Color), 2.3.3 (Animation from
  Interactions) — cited from general web accessibility knowledge, not fetched from an external
  source in this pass; verify exact SC wording against w3.org/WAI/WCAG22 if precise compliance
  language is needed for the plan phase.
- Comparable-product patterns (GitHub Actions run list, cloud console provisioning states,
  VS Code/JetBrains background task indicators, Linear/Notion optimistic UI) are drawn from
  general familiarity with these products' documented/observable behavior, not a fresh web
  fetch in this research pass — flagged as INFERRED rather than freshly verified; low risk since
  the point being drawn from each is a widely-documented, stable interaction pattern rather than a
  specific claim that could have changed recently.
