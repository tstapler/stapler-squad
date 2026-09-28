# Research: UX — backlog-diagnose-and-nudge

Companion to `requirements.md`. Covers comparable-product patterns, Tyler's mental
model (inferred from the shipped sibling feature), accessibility, error/edge-case UX,
and jobs-to-be-done for the "Diagnose" action and its fully-autonomous nudge path.

## 0. Existing codebase conventions this feature must match

Direct inspection, not inference:

- **`web-app/src/components/backlog-stuck/StuckItemDetail.tsx`** — the accordion detail
  panel pattern this feature extends. Every reason variant follows the same shape:
  a `data-testid`-tagged `actionCopy` `<p>` stating literally what happened and what
  (if anything) to do about it, an inline form for the one available remediation
  (`onReworkCapOverride`, `onApprovePlan`) with three states (`idle` / `pending` /
  `error`), and a `role="alert"` inline error string on failure — never a silent
  no-op or a toast that could be missed. `AUTONOMOUS_STUCK`'s copy
  (`stuck-item-autonomous-stuck-copy`, lines 227–233) is the closest existing analog
  to "an autonomous agent stopped and here's what to do" — it reads: *"Autonomous mode
  stopped without a completion signal. Open the session to see what it accomplished,
  then either give it a manual instruction or use 'Reopen for Revision' / re-trigger
  triage to let it try again."* This is the tone/structure to imitate: name the
  mechanism, point at where to look, name the recovery lever.
- **`project_plans/backlog-stuck-item-visibility/design/ux.md`** (the sibling's UX
  design doc) — establishes house style: color+text-paired chips (never color-only),
  reassuring/filter-aware empty-state copy, degraded states get an explicit banner
  rather than a silent empty list (`DeviceAuthBanner` precedent), non-exclusive
  accordions so two items can be compared side by side, focus returns to the
  toggle on collapse (never dropped to `<body>`), `aria-live="polite"` for routine
  background updates vs `role="alert"` reserved for actionable/urgent signals, and a
  hard rule that **no state may silently upgrade toward "healthy" on stale data**
  (`pr_status_unknown` only clears on a fresh successful check). This last rule is
  the single most important transferable precedent for Diagnose/nudge: a nudge
  outcome must never silently read as "handled" until it's confirmed, and a stale
  "last diagnosed" timestamp must be visibly stale, not silently trusted.
- **`web-app/src/components/backlog/detail/SessionDiagnosticPanel.tsx`** +
  `web-app/src/lib/backlog/sessionKind.ts`** — the codebase already has a fully
  general mechanism for exactly this feature's core UX problem: presenting a
  DB-only, non-interactive, agent-authored record as a first-class item in the
  Sessions list. `classifySessionKind` already recognizes a `"headless_diagnostic"`
  kind (role `"triage"` or a `headless-` session-id prefix) that is **not**
  steerable (`isSteerable` returns false — no PTY, nothing to attach to) but still
  renders a structured, read-only summary (`TriageReviewPanel` or `GateVerdictBox`,
  both `readOnly`) plus a one-line `role="status"` summary. **This is the load-bearing
  precedent for the Diagnose dispatch**: a dispatched diagnostic session should
  register as a `headless_diagnostic`-classified Synthetic Session (e.g. a
  `headless-diagnose-*` session-id prefix slots straight into the existing
  `startsWith("headless-")` check with zero new classification code) so it appears
  in the existing Sessions list, gets the existing icon/label treatment, and never
  needs a bespoke "diagnosis history" surface built from scratch.
- **`web-app/src/components/backlog/detail/ActivityLogSection.tsx`** — the existing
  free-form, timestamped, author-attributed note feed (`post_backlog_update`),
  visually distinct from official progress/review marks specifically so an informal
  note is never confused with a formal verdict. This is the right existing surface
  for the diagnostic agent's inconclusive-outcome note (requirement: "post a
  diagnostic note when inconclusive") — reuse it rather than inventing a new note
  type. Author attribution already supports a session UUID/title, so a
  diagnose-dispatched session's notes attribute correctly with no new plumbing.
- **`GateVerdictBox.tsx` / `TriageReviewPanel.tsx`** — both already support
  `readOnly` rendering of a structured verdict with a summary line and a
  pass/fail-per-criterion breakdown. A nudge/bug-filed/skipped outcome is
  structurally the same shape (one summary line + a small number of named facts:
  which safety check passed/failed, cap remaining, cooldown remaining) — reusing
  this component family keeps Diagnose visually consistent with two already-shipped
  "an agent made a structured judgment call" surfaces instead of introducing a third
  visual language for the same job.

## 1. Comparable UX patterns — diagnose/auto-remediate in dev tools

Patterns that work well, and the mechanism behind why:

- **GitHub Actions "Re-run failed jobs" + run history.** The action itself is one
  click, but the payoff is the persistent, timestamped run list showing exactly
  which jobs re-ran, their new conclusion, and a link to the log — the click is
  cheap because the audit trail after it is complete and permanent. Applies here:
  the "Diagnose" button being trivial to invoke is only trustworthy because the
  resulting record (which agent ran, what it decided, what it did) is equally easy
  to find afterward, in the same place every time (Sessions list), not a
  fire-and-forget toast.
- **PagerDuty / Datadog auto-remediation runbooks.** The convention that earns
  trust is not the automation itself but the **three-part event record**: (1) what
  triggered it, (2) what decision the runbook made and why (including "decided not
  to act — condition X wasn't met"), (3) what changed as a result, if anything.
  Critically, "the runbook ran and did nothing" is always a distinct, logged event
  — never indistinguishable from "the runbook didn't run." This maps directly onto
  the requirement's three-way outcome split (nudged / skipped-unsafe /
  bug-filed-instead): all three must be equally visible, not just the "did
  something" cases.
- **Dependabot / Renovate auto-merge.** Two trust levers dev tools converge on:
  (a) a visible **capability/config surface** ("auto-merge is enabled for
  patch-level only") so the user always knows the blast radius before anything
  happens, matching this feature's feature-flag-gated nudge capability — the UI
  needs an equivalent, always-visible "nudging: enabled / disabled" indicator, not
  something only discoverable by reading a config file; and (b) every autonomous
  merge gets its own permanent, linkable record (the merge commit + a bot comment
  explaining why), not just a log line buried in CI output.
- **Kubernetes self-healing (liveness-probe restarts) + `kubectl describe`
  Events.** The interaction users rely on is "describe" — a chronological event
  list scoped to *this one resource*, showing every automated action taken against
  it, each with a reason string and a timestamp, retained across several restarts
  (not just the most recent). This is the strongest single precedent for "surface
  after-the-fact what the agent did and why": Diagnose's outcome should be an
  **item-scoped, chronological, multi-event history**, not a single "last diagnosis"
  field that overwrites itself — Tyler needs to see a *pattern* of nudge attempts
  over time to judge whether the automation is trustworthy, not just the latest one.
- **Anti-pattern to avoid: silent auto-retry with no visible counter.** Several
  CI systems' "flaky test auto-retry" features got redesigned after users
  discovered retries were masking a real, worsening problem — because the retry
  count wasn't surfaced anywhere near the pass/fail badge. Directly relevant to the
  nudge cap: the cap-hit event and the running "N nudges so far" count must be
  visible on the item itself, not only derivable from digging through logs,
  otherwise repeated nudging on a genuinely broken item reads as "it's fine" right
  up until the cap silently triggers a bug-filed fallback.

## 2. Tyler's mental model (inferred from the shipped sibling feature)

The sibling `backlog-stuck-item-visibility` feature is the strongest evidence of
what Tyler expects, because its UX design doc encodes decisions he presumably
reviewed or at least the assistant made under his standing instructions:

- **"Triage, not investigation" as the default posture** — glance-level facts
  (reason, duration, one identifying count) with full detail one click away, never
  a wall of text by default. Diagnose's outcome should follow the same
  glance/detail split: a compact chip/line on the item ("Diagnosed 2m ago — nudged")
  at a glance, full reasoning behind one click.
- **Zero tolerance for a state that "looks fine" when it might not be.** The
  entire `pr_status_unknown` design (never silently show 🟢 on stale data) and the
  nav badge's "never a misleading zero" rule (AC 24) show a consistent instinct:
  Tyler would rather see an explicit "couldn't confirm" than a false-positive
  "handled." This is the single most important transferable constraint for the
  no-approval-gate nudge: an inconclusive or skipped outcome must be **visually as
  loud as** a successful one — never demoted to a quieter treatment just because
  nothing "happened."
- **Snooze, not delete; visibility control, not remediation, kept explicit and
  separate.** The sibling feature's snooze flow deliberately requires an explicit
  duration pick (not a single ambiguous click) specifically because muting a signal
  has real consequences. Diagnose/nudge inverts this shape but for the same
  underlying value: since there's no approval gate on the *write*, the equivalent
  "friction point" has to move to the *read* side — the after-the-fact record must
  make the write impossible to miss or misread, since there is no moment where
  Tyler consciously agreed to it.
- **Group/severity language is handled with unusual rigor.** The sibling doc goes
  out of its way to state group ordering is "actionability, not severity" and is
  never color-coded as danger. This suggests Tyler cares about UI copy/ordering not
  accidentally implying a confidence level the system doesn't actually have — the
  Diagnose UI should avoid, e.g., a green "all good, nudged successfully" chip that
  implies the underlying stuck condition is resolved when a nudge is only ever a
  best-effort attempt to unstick a session, not a fix.
- **Existing precedent already generalizes cleanly (Section 0)**: the
  `headless_diagnostic` Synthetic Session concept, `ActivityLogSection`, and the
  `readOnly` `GateVerdictBox`/`TriageReviewPanel` family show the codebase was
  already extended once before (`backlog-operator-feedback-loop`'s ADR-002, per
  `sessionKind.ts`'s doc comment) specifically to let non-interactive agent
  judgment calls appear in the UI as first-class, structured records. This is a
  strong signal Tyler prefers **extending an existing structured-record mechanism**
  over inventing a bespoke "diagnosis log" UI for this feature.

## 3. Accessibility requirements

Baseline is the sibling feature's already-shipped AC list (§13–20, 26–30 of
`project_plans/backlog-stuck-item-visibility/design/ux.md`), which this feature must
match rather than regress:

- **Diagnose button**: a real `<button>`, reachable via Tab, activated via
  Enter/Space, with a descriptive `aria-label` (e.g. `"Diagnose this stuck item"`,
  not bare "Diagnose") since the visible label alone may be ambiguous out of
  context for a screen-reader user tabbing through a dense card.
- **Busy/dispatch-in-flight state**: while the diagnostic session is being
  dispatched, the button must expose `aria-busy="true"` and a disabled state (not
  just a CSS spinner with no semantic signal), and the button label should change
  to reflect state (e.g. "Diagnosing…") rather than relying on a spinner icon
  alone — mirrors the existing `overrideState === "pending"` pattern in
  `StuckItemDetail.tsx`'s rework-cap form.
- **Outcome announcement**: because there is no approval step, the outcome of a
  dispatch (queued / diagnosing / nudged / skipped / bug-filed / inconclusive /
  dispatch-failed) is exactly the kind of state change a screen-reader user could
  otherwise miss entirely (nothing prompted them to look). The result region should
  use `aria-live="polite"` for the routine settle (matching the sibling's count
  region convention) — **except** the "dispatch failed" and "nudge attempted but
  errored" cases, which should use `role="alert"` (or `aria-live="assertive"`),
  matching `StuckItemDetail.tsx`'s existing `role="alert"` treatment for
  `overrideState === "error"`. Successful/neutral outcomes (nudged, skipped-safe,
  bug-filed, inconclusive) are informational, not urgent — `polite` is correct
  there per the sibling's `role="alert"`-is-for-urgent-only rule; a failed dispatch
  or an internal error is the urgent case.
- **Keyboard-only full flow**: click/Enter/Space to trigger Diagnose → focus stays
  on (or moves predictably to) the result region as it updates → the item's history
  entries (Section 4 below) are keyboard-navigable list items, each individually
  focusable/expandable if they carry further detail (matching the existing
  `ActivityLogSection`'s `role="list"`/`role="listitem"` pattern).
- **Color independence**: each of the four-plus outcome states (nudged / skipped /
  bug-filed / inconclusive / dispatch-failed) pairs an icon/color with a text label,
  per the sibling's "removing all color still leaves state legible" test (AC 18) —
  do not rely on green/yellow/red alone to distinguish "nudged" from "skipped" from
  "failed."
- **Contrast**: new chip/badge text-on-background pairs for the outcome states meet
  WCAG AA 4.5:1, consistent with the existing Axe Core CI gate on `web-app/src/`.
- **Feature-flag-off state**: when nudge capability is disabled, the Diagnose
  button/affordance for "will nudge" must not be presented as available-but-broken
  (e.g., a control that's clickable but silently no-ops) — either omit the nudge
  branch of the explanation entirely or state plainly, as visible text (not just a
  disabled-looking control with no explanation), that nudging is off and diagnosis
  will only ever result in a note or a filed bug. This mirrors the sibling's
  "no action available" literal-copy convention (`pr_status_unknown`'s required
  on-screen string) rather than leaving the limitation to be inferred.

## 4. Error / edge-case UX

Following the sibling feature's house rule — **every degraded state gets its own
distinct, explicit UI, never folded into a generic error or a silent no-op** — this
feature needs at minimum five distinguishable outcome states, each with its own
copy and its own icon+label pairing (never color-only):

| Outcome | What happened | Required on-screen copy (literal-string convention, per sibling AC 23/25) | Icon/chip |
|---|---|---|---|
| **Dispatch failed** | Could not create/start the diagnostic session at all (e.g. MCP server unreachable) | *"Couldn't start diagnosis — \<reason if known\>. Try again."* + Retry | ⚠ / neutral-error color, distinct from all "ran successfully" outcomes |
| **Diagnosing (in flight)** | Session dispatched, agent still running | *"Diagnosing…"* with `aria-busy` | ⟳ spinner |
| **Nudged** | Safety gates passed; nudge sent | *"Diagnosed \<time\> ago — nudged the session."* + link to the diagnostic session's record | 🟢 or a dedicated "acted" color |
| **Skipped (safety gate)** | Diagnosis ran, decided a nudge was warranted, but the pre-write idle/identity re-check failed | *"Diagnosed \<time\> ago — nudge skipped (session wasn't idle / identity check failed)."* — state the *specific* gate that failed, not a generic "skipped" | 🟡 / neutral, explicitly **not** styled as a failure (declining to act unsafely is the gate working correctly) |
| **Bug filed instead** | Agent judged the root cause wasn't nudge-fixable | *"Diagnosed \<time\> ago — filed \<bug link\> instead of nudging."* with a direct link to the filed bug | 🐛 / info color |
| **Inconclusive** | Agent couldn't determine a clear next step | *"Diagnosed \<time\> ago — inconclusive."* + link to the posted diagnostic note (`ActivityLogSection` entry, §0) | ⚪ / muted, same "couldn't determine" family as the sibling's `pr_status_unknown` chip |
| **Nudging disabled (flag off)** | Diagnose ran or is available, but nudge capability is globally off | *"Nudging is currently disabled — diagnosis will file a bug or post a note, but won't act on the session directly."* | ⚙ / informational, shown proactively (before dispatch) as a standing notice, not only after the fact |

Additional edge cases, matching the sibling's "no dead ends" rule:

- **Nudge cap/cooldown hit mid-flow**: this is a sixth flavor of "skipped," and
  must name the specific limit ("nudge cap reached: 3/3 this window" or "cooldown
  active, next eligible \<time\>") rather than a generic "skipped" — otherwise a
  legitimately-firing safety control looks identical to an unexplained failure,
  eroding exactly the trust this after-the-fact surface exists to build.
  Requirements.md's "Configurable nudge cap/cooldown" implies these are two
  distinct counters; the UI should be able to say which one fired.
  Recommendation: reuse `formatReworkCapOverride`'s "N / cap" numeric-display
  convention already in `StuckItemDetail.tsx` for a visually consistent counter.
- **Diagnostic session itself crashes/times out after dispatch succeeded**: distinct
  from "dispatch failed" (which fails before a session exists) — this is a
  `headless_diagnostic` Synthetic Session that ended without a completion signal,
  which the existing `AUTONOMOUS_STUCK` copy and detection already model almost
  exactly (`session/instance actor` — the copy "stopped without a completion
  signal" is directly reusable/adaptable here). Recommendation: don't invent new
  handling — a diagnostic session that stalls should itself become visible via the
  existing stuck-item machinery (ironic but correct: the diagnoser can get stuck
  too, and should be diagnosable in principle, though re-triggering Diagnose on a
  Diagnose session is out of scope per requirements.md unless a future iteration
  decides otherwise).
- **Two Diagnose dispatches racing** (user clicks twice, or an automated re-check
  fires while a manual one is in flight): button must be disabled while
  `aria-busy`, and if a duplicate dispatch is attempted anyway, the server-side
  response should be treated as "already diagnosing" (reuse the in-flight state)
  rather than surfacing a second, contradictory outcome for the same item.
- **Result arrives after the user has navigated away and come back**: because there
  is no approval step, Tyler may open this page well after the nudge already
  happened. The outcome display must be sourced from durable, persisted state (the
  structured logging event / Synthetic Session row), not client-side-only state —
  matching the sibling's "duration/timestamps always sourced from persisted fields,
  never process-uptime" rule (its traceability table, final row). A page refresh
  must show the same outcome as was live moments earlier.
- **History, not just latest**: per the Kubernetes-Events precedent (§1), a single
  "last diagnosis" field is not enough — an item that's been diagnosed and nudged
  three times in a row (each skipped or ineffective) needs its *history* visible,
  not just the most recent line, so a pattern of "nudging isn't working on this
  item" is discoverable without grepping logs. Minimum bar: extend
  `ActivityLogSection`'s existing feed (or the Sessions list, since each dispatch
  is its own `headless_diagnostic` row) rather than a single overwriting field.

## 5. Jobs-to-be-done

**Functional jobs:**
- Get a stuck item unstuck (or correctly triaged toward the right remediation —
  bug filed, human attention flagged) without Tyler personally re-deriving why it's
  stuck from raw session state each time.
- Reduce Tyler's manual polling of stuck/stalled sessions — the diagnostic agent
  does the "is this actually stuck or just slow" judgment call he'd otherwise make
  by eyeballing scrollback.
- Provide a queryable record of *why* each autonomous action happened, sufficient
  for Tyler to answer "did the system do the right thing here?" without replaying
  the whole session.

**Emotional jobs (the dominant ones, given the no-approval-gate risk control):**
- **Trust-building for an action he didn't approve in the moment.** This is the
  central emotional job named in the task brief. The mechanism the research above
  converges on (Kubernetes Events, PagerDuty runbook records, Dependabot's
  always-linkable merge record) is consistent: trust in unattended automation is
  built by **cheap, reliable, structured after-the-fact legibility**, not by making
  the action itself harder to trigger. Every nudge needs to be as easy to audit,
  after the fact, as an approval gate would have made it easy to review beforehand.
- **Relief from "is something silently wrong right now" anxiety** — the same
  emotional job the stuck-item-visibility feature itself was built to solve
  (per `MEMORY.md`'s `backlog_stuck_item_visibility` context: "solved *seeing*"
  but declined to *act*). Diagnose closes the loop, but only fulfills this job if
  its own outcomes are at least as visible as the stuck state that triggered it —
  an invisible or hard-to-find nudge outcome would just relocate the anxiety one
  level down ("did the automation even try?").
- **Avoiding a *new* anxiety: "is the automation making things worse without me
  noticing."** Because there's no approval gate, a nudge that repeatedly fires on
  a fundamentally broken item (rather than escalating to a filed bug) is a
  plausible failure mode Tyler would want caught by the cap/cooldown — the UI's
  job is to make that cap's operation visible enough that Tyler trusts it's
  working, not just trust that it exists in code he hasn't re-read recently.

**Social/organizational jobs:** single-user product (per requirements.md's "Users /
Consumers"), so there's no peer-facing job here in the traditional PM sense. The one
adjacent job: **future-Tyler as an audience for present-Tyler's automation
decisions** — the structured logging + UI surface doubles as documentation he can
point to later (e.g., in a `docs/explanation/` note, or when deciding whether to
raise the nudge cap) for *why* the nudge-cap/cooldown values were set where they
were, without re-deriving that reasoning from raw log greps.

## Traceability

| Requirement / risk (requirements.md) | Addressed in |
|---|---|
| Three distinct outcomes (nudged / skipped-unsafe / bug-filed) | §4 table |
| "Dispatch failed" distinct from "ran and found nothing" | §4 table, first two rows |
| Nudge cap/cooldown configurable, UI must name which fired | §4 "Nudge cap/cooldown hit" |
| Nudging disabled (flag OFF) is its own distinct state | §3 "Feature-flag-off state", §4 last row |
| Structured logging *is* the audit trail (no approval gate) | §1 (Kubernetes Events precedent), §4 "Result arrives after navigating away", §5 emotional jobs |
| Match sibling feature's visual/interaction conventions | §0, §2 |
