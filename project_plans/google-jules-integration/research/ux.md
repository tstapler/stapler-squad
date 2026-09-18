# UX Research: Google Jules Integration

Agent 5 (UX Research), SDD Phase 2, `google-jules-integration`.

## 0. Existing patterns to reuse (web-app inventory)

stapler-squad already has three UI primitives that are the right building
blocks for a Jules indicator — reuse and extend them; do not invent a
parallel status system.

- **`InstanceType.EXTERNAL`** (`proto/session/v1/types.proto:446-453`) — the
  session model already distinguishes "fully managed by claude-squad" from
  "discovered externally, limited interaction." `SessionCard.tsx:380,554-565`
  renders a `🔗 <sourceTerminal>` pill (`externalBadge`, primary-color,
  `role="img"` + descriptive `aria-label`/`title`) for external sessions.
  **Caveat:** `EXTERNAL` today still assumes a real PTY exists somewhere
  (ssq-mux discovers an actual tmux pane) — `SessionDetailView.tsx:580` still
  shows a "Terminal" tab for `EXTERNAL` sessions, and only the "Switch
  workspace" button is suppressed (`SessionDetailView.tsx:833`). A Jules
  session has **no PTY at all**, so it cannot reuse `EXTERNAL` verbatim; it
  needs either a third `InstanceType` value or a separate "backend kind"
  discriminator the architecture research should size (see `SessionCard.tsx`
  comment `// +feature: remote-host-badge` for where this class of feature
  gets marked).
- **`RemoteConnectionIndicator`** (`web-app/src/components/sessions/RemoteConnectionIndicator.tsx`)
  — built for SSH-bastion remote workspaces (Epic 6.2), this is the closest
  existing analog to "work happening on infrastructure I don't directly
  control." Pattern worth copying wholesale for a Jules status chip:
  - Redux-driven, no polling/fetch of its own inside the component.
  - Dot/spinner + text label, never color alone (see §3).
  - A persistent `aria-live="polite"` region for state transitions
    (connecting/reconnecting), plus a separate `role="alert"` element fired
    only on the failure transition — polite for routine updates, assertive
    only for "this needs your attention."
  - Renders nothing (not a default/neutral badge) until a real state is
    known — avoids a misleading "connected" flash before the first event.
- **`SessionSummaryPanel`** (`web-app/src/components/sessions/SessionSummaryPanel.tsx:32-51`)
  — the PR-summary card from `docs/jules-feature-adoption.md`'s candidate #1
  is already shipped for local sessions. Its `Phase` enum
  (`loading | empty | ready | error | error-stale | transport-error`) is the
  right shape to copy for Jules error states (§4) — it already distinguishes
  "never got data" from "had data, then a poll failed" (stale-but-shown, not
  wiped) from "generation itself failed," with plain-language copy per
  failure stage (`ERROR_STAGE_COPY`) instead of raw enum strings or stack
  traces.
- **`GitHubBadge`** (`web-app/src/components/shared/GitHubBadge.tsx`) — the
  PR chip Jules-produced PRs should flow into unchanged, per the
  requirements doc's success metric ("pollable via the existing PR review
  path, not a separate surface"). No new PR-status UI needed if `WorktreePRPoller`
  ingests Jules' PR the same way it does any other agent's.

## 1. Comparable UX patterns: remote/opaque job vs. local/inspectable process

Surveying CI/deploy status UIs (GitHub Actions run view, Vercel/Netlify
deploys) against what stapler-squad already does for local tmux sessions,
the distinguishing cues cluster into four categories:

| Cue | Local/inspectable (tmux) | Remote/opaque (CI, Jules) |
|---|---|---|
| **Primary content** | Live character stream (xterm.js) | Discrete, timestamped event list ("Started," "Ran tests," "Opened PR") |
| **Interaction affordance** | Type into it, attach, steer | Read-only until terminal; at most "cancel"/"open in provider" |
| **Progress signal** | Implicit — you watch tokens/output scroll | Explicit — a phase name or step counter ("2 of 4 steps"), because there's no scroll-to-infer-activity |
| **Iconography** | Terminal/monitor glyph | Cloud/globe glyph, provider logo mark |

GitHub Actions and Vercel both solve "no PTY" the same way: a **vertical
timeline of named steps**, each collapsible, each with a duration and a
terminal icon (success/fail/running), plus a persistent top-level status
pill that summarizes the whole run without requiring the user to open it.
Neither tries to fake a live terminal for a process that doesn't expose one
— they lean into the discreteness rather than smoothing over it with a fake
character stream. That is the model to follow for Jules' Activities API,
not an attempt to synthesize a pseudo-terminal from activity messages.

## 2. Replacing "watch the terminal": recommended mental model

A stapler-squad user's default expectation, reinforced by every other
session type in this codebase, is "click the card → see a live terminal."
Silently omitting the Terminal tab for a Jules session (as a bare disabled
tab) would read as broken, not as a different kind of session. Recommend:

- **A dedicated "Activity" tab** replaces "Terminal" in the tab strip for
  Jules-backed sessions (reusing the `tabs` array/`disabled` pattern already
  in `SessionDetailView.tsx:579-589`, not a new tab-bar component) — a
  reverse-chronological list of Jules `Activity` events (plan proposed, plan
  approved, file changed, command run, PR opened, session completed/failed).
  Each row: icon + one-line summary + relative timestamp, expandable for the
  raw activity payload (mirrors `SessionLogsTab`'s expand-for-detail
  pattern already in the codebase, not a new interaction).
- **A status badge**, not a full timeline, is the *summary* surface — on
  `SessionCard`/`BacklogItemBadge` where space is tight (`BacklogItemBadge.tsx`
  is explicitly capped at single-line/260px per its Story 5.1.2 comment — a
  full activity feed has no room there). Badge states: Queued → Running →
  Needs Review (PR opened) → Done → Failed, mirroring the vocabulary Jules'
  own product uses, so a user who also uses jules.google.com directly isn't
  learning two vocabularies.
- **Do not attempt a live "typing" simulation** of Jules activity text —
  the Activities API is polled, not streamed; presenting it as if it were a
  live PTY (e.g., animating text char-by-char) would misrepresent latency
  and erode the "I can trust this status" job (§5's emotional job).
- The existing `isSessionTerminal(status)`-gated "Summary" tab
  (`SessionDetailView.tsx:588`) applies unchanged once a Jules session
  reaches a terminal state — the PR-summary/diff-summary generation
  pipeline doesn't care what produced the diff.

## 3. Accessibility

- **Never encode local-vs-Jules (or queued/running/done) in color alone.**
  Every existing status/badge component already follows this — `StatusBadge.tsx`
  pairs a color variant with a distinct emoji icon *and* a text label for
  every state (🔒 Approval Pending, ⚠️ Error, ✅ Complete, etc.), and
  `RemoteConnectionIndicator.tsx` pairs its color class with a dot/spinner
  shape difference plus a text label. A Jules badge must do the same: an
  icon shape (e.g., ☁️ or a cloud glyph, distinct from 🔗 external and 🖥️
  host) + text label + color, never color-only, and never icon-only (screen
  reader users need the label; the aria-label pattern in every badge above —
  `role="img"` + explicit `aria-label` — is the baseline to match, not
  `title` alone, which isn't reliably exposed to AT).
- **`role="status"` / `aria-live="polite"` for state transitions**, matching
  `RemoteConnectionIndicator`'s persistent live region — a Queued→Running or
  Running→Needs Review transition should be announced without requiring
  focus to be on the card, since Jules sessions run unattended by design
  (that's the point of offloading).
- **`role="alert"` only for failure**, not for routine progress — reuse
  `RemoteConnectionIndicator`'s split between a polite region (routine) and
  an assertive one fired only on the disconnect-equivalent (Jules session
  failed / API unreachable), so failures interrupt a screen reader user the
  way a sighted user's eye is drawn to a red badge, without every routine
  status tick doing the same.
- **Activity timeline keyboard/AT structure**: use a semantic list
  (`<ul>`/`<li>` or `role="list"`), each item with a clear heading-level
  summary before the expandable detail, so AT users can scan the timeline
  the way sighted users scan a vertical list of icons — do not rely on
  visual indentation/connector lines alone to convey sequence.
- **Color contrast**: existing badges use token-driven colors
  (`vars.color.primary`/`primaryText`, `vars.color.surfaceSubtle`/`textSecondary`)
  which should already meet WCAG AA in both themes per this repo's design
  system — a new Jules badge variant should pull from the same token set
  rather than a one-off hex value, and get the same dark-mode check the rest
  of the design system gets.

## 4. Error states

Following `SessionSummaryPanel`'s `Phase` model (§0) — distinguish "never
had data" from "had data, now stale" from "the operation itself failed" —
recommend four distinct Jules error surfaces, each with plain-language copy
(never a raw HTTP status or stack trace as the primary message, detail
available on expand/tooltip):

1. **API key missing/invalid** — not a per-session error at all; this is a
   precondition. Surface it where the user would try to create a Jules
   session (backlog item's session-creation UI / omnibar), disabling the
   Jules option with inline copy ("Add a Jules API key in Settings to enable
   cloud sessions") rather than letting the user create a session that
   immediately fails. If a stored key is later revoked/expired mid-session,
   surface it as a distinct badge state ("Jules: Reconnect required") not
   folded into generic "Failed," since the fix (re-enter a key) differs from
   a genuine task failure.
2. **Jules session failed** (the task itself errored on Google's side) —
   terminal state, red/error badge variant, activity timeline's last entry
   shows Jules' own failure message verbatim (attributed, since
   stapler-squad didn't generate it — mirrors `stageSentence()`'s pattern of
   a plain lead sentence, but here the detail *is* Jules' own text, not
   ours). Offer "View on jules.google.com" as an escape hatch — Jules' own
   UI will have more diagnostic depth than stapler-squad will ever mirror,
   and MVP is explicitly fire-and-forget (no re-invocation loop), so a deep
   local retry UI would overpromise capability that doesn't exist yet.
3. **Jules API unreachable / rate-limited** — this is a *poller* failure,
   not a session failure; the last-known session status must stay visible
   (stale-but-labeled, exactly like `SessionSummaryPanel`'s `error-stale`
   phase) with a small "Last updated Nm ago, retrying…" indicator rather
   than the badge flipping to an error/failed state — a transient poll
   failure must never be visually indistinguishable from the Jules task
   itself having failed, since those require completely different user
   reactions (wait vs. investigate).
4. **Jules PR import/link failure** (PR created on Jules' side but
   stapler-squad's poller can't attach it) — surface as a distinct
   secondary notice on the session, not a full session failure, since the
   underlying work likely succeeded; this is the scenario most likely to
   erode trust silently if unsurfaced (per the user's global instinct doc,
   `feedback_document_ai_decisions_in_edge_cases`: don't let automation act
   or fail silently — post a visible notice).

## 5. Job-to-be-done

- **Functional job**: offload a backlog item to Jules' cloud VM when local
  capacity/CPU/RAM is the constraint, or when the user wants execution to
  continue while their own machine is off/asleep — distinct from local
  agents, whose functional job is tight iteration loops with the developer
  present.
- **Emotional job**: confidence that work is progressing without requiring
  the user to babysit a terminal — this is the same emotional job local
  sessions already serve via status badges/notifications when a user tabs
  away, but stronger for Jules because there is no "just glance at the
  terminal to reassure myself" fallback. This is *why* the activity
  timeline + live-region status transitions (§2, §3) matter more here than
  for local sessions: the only trust signal available is the UI's own
  status reporting, so it has to be honest about staleness (§4.3) rather
  than defaulting to an optimistic "still running" that silently rots.
- **Social job**: limited but real — a PR badge/attribution that shows a
  PR came from Jules (vs. a local Claude Code/Aider session) lets a
  teammate reviewing the PR calibrate scrutiny appropriately (cloud agent
  on Google's infra with its own guardrails, vs. a local session running
  with whatever permissions the user granted) and lets the user
  demonstrate they're using available infra efficiently rather than
  saturating their own machine. This is secondary to the functional/
  emotional jobs and shouldn't drive scope — a small provenance badge on
  the PR/session (reusing `GitHubBadge`'s existing badge-row slot) is
  sufficient; no dedicated "ran on Jules" marketing surface is warranted.

## Summary of concrete UI recommendations for Phase 3 (plan)

1. New Jules status badge component, modeled on `RemoteConnectionIndicator.tsx`
   (dot/icon + label + `role="status"`/`aria-live` split), not a copy of
   `externalBadge`/`hostBadge` verbatim (those imply an underlying PTY that
   doesn't exist for Jules).
2. New "Activity" tab replacing "Terminal" in `SessionDetailView.tsx`'s tab
   array when the session is Jules-backed, reusing the disabled-tab
   mechanism already there — timeline list, not a simulated terminal.
3. Reuse `SessionSummaryPanel`'s `Phase` state shape for Jules
   create/poll/session-failure error handling, with the stale-vs-failed
   distinction preserved (§4.3) — this is the most likely place a naive
   implementation collapses "poller hiccup" and "task failed" into one
   error state, which would misinform the user in exactly the scenario
   (unattended cloud work) where trust matters most.
4. Reuse `GitHubBadge` unchanged for the resulting PR; add only a small
   provenance marker (icon or text) distinguishing a Jules-originated PR,
   not a parallel PR UI.
5. Confirm with architecture research (their call, not UX's) whether Jules
   gets a third `InstanceType` enum value or a separate backend-kind field —
   either way, the UI badge/tab logic above should key off whatever
   discriminator they choose, not off `InstanceType.EXTERNAL` (semantically
   wrong: EXTERNAL still implies a real PTY, per §0).
