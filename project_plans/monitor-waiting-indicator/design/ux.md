# UX Design: monitor-waiting-indicator

## Scope note (read this first)

This is a **backend counting fix**, not a new UI build. `implementation/plan.md` and
`research/ux.md` both conclude AC2's "distinct visual indicator" requirement is already satisfied
by `SubStatusChip.tsx`'s `SubStatus.WAITING_FOR_AGENT` case — no new component is planned, and
this design doc does not invent one. Its job is to document the *existing* surface as the one
that satisfies AC2, verify it meets the UX acceptance bar (including accessibility), and show
what actually changes for the user once the backend fix (Epic 1.1 in the plan) lands: the number
in an already-existing chip, not a new element.

Source files verified directly for this doc:
- `web-app/src/components/sessions/SubStatusChip.tsx:38-64` (chip markup/logic)
- `web-app/src/components/sessions/SubStatusChip.css.ts:111-120` (`chipWaitingForAgent` styling)
- `web-app/src/components/sessions/SessionCard.tsx:787-792` (card-view mount point)
- `web-app/src/components/sessions/SessionRow.tsx:349-361` (row/list-view mount point)
- `web-app/src/lib/utils/deriveWorkingState.ts:12,19,34,49` (status-grouping bucket mapping)

## Step 1: User-facing surfaces

| # | Surface | Type | Treatment |
|---|---|---|---|
| 1 | `SubStatusChip` "Waiting for N Tasks" chip, card view | Visible status indicator (non-interactive: `role="status"` span, no click target) | Full (wireframe + flow + edge cases) |
| 2 | Same chip, row/list view | Visible status indicator | Condensed (layout diff from #1 only) |
| 3 | Detection input → `SubagentCount` output (the actual code change) | Non-interactive: regex over PTY scrollback text | Condensed |
| 4 | Status-grouping bucket placement (`WorkingState`) | Non-interactive: internal categorization consumed by tag-organization's Status grouping | Condensed |

There is no modal, error dialog, or empty state introduced by this feature — the chip either
renders or it doesn't (`SubStatus.UNSPECIFIED` / `IDLE` → chip omitted entirely by the parent's
render guard, `SessionCard.tsx:788-789`, `SessionRow.tsx:349-350`), and detection failure modes
resolve to "chip doesn't show" rather than a visible error, per the codebase's `assertNever`
forward-compat pattern (`SubStatusChip.tsx:189-198`).

---

## Step 2: Surface 1 — `SubStatusChip` (card view), full treatment

### Wireframe — before the fix (today, undercounted)

```
┌─ Session Card ───────────────────────────────────────────┐
│ my-feature-branch                              [●●●]      │
│ tstapler/stapler-squad · worktree: triage-...             │
│                                                            │
│  ⏳ Waiting for 1 Task            🟠 Stale (if applicable) │
│  ^^^^^^^^^^^^^^^^^^^^                                     │
│  WRONG: input line was "1 shell, 1 monitor still          │
│  running" (2 outstanding), but shells_still_running        │
│  only captured the monitor → count=1                       │
└────────────────────────────────────────────────────────────┘
```

### Wireframe — after the fix

```
┌─ Session Card ───────────────────────────────────────────┐
│ my-feature-branch                              [●●●]      │
│ tstapler/stapler-squad · worktree: triage-...             │
│                                                            │
│  ⏳ Waiting for 2 Tasks           🟠 Stale (if applicable) │
│  ^^^^^^^^^^^^^^^^^^^^^                                     │
│  CORRECT: 1 shell + 1 monitor summed, mirroring how        │
│  the short footer form already sums both groups.           │
└────────────────────────────────────────────────────────────┘
```

Chip anatomy (unchanged by this fix — shown for completeness):

```
┌──────────────────────────────┐
│ ⏳  Waiting for 2 Tasks        │   role="status"
└──────────────────────────────┘   aria-label="Waiting for agents"
    ^                               title="Claude is waiting for 2
    aria-hidden glyph, decorative    background tasks to finish"
```

### Interaction flow

This is a passive, read-only indicator — there is no user input step. The "interaction" is Claude
Code's own output changing and the UI reflecting it:

1. **Claude's turn ends with outstanding background work.** The CLI emits a line like
   `"✻ Cogitated for 1m 6s · done 3:06 PM · 1 shell, 1 monitor still running"` into the tmux pane.
2. **Backend detects it.** `session/detection` (Surface 3) parses the line into
   `DetectedStatus == WAITING_FOR_AGENT`, `SubagentCount == 2` (post-fix), regardless of tmux
   control mode vs. legacy polling (`ClaudeController.GetStatusAndIdleInfo` reads the same PTY tail
   either way — no mode branch exists, `claude_controller.go:1124-1138`).
3. **Count reaches the client.** `server/adapters/instance_adapter.go:252-256` maps
   `SubagentCount` onto the proto unconditionally; the web client receives `session.subagentCount`.
4. **Chip renders.** `SessionCard.tsx`/`SessionRow.tsx` render `<SubStatusChip subagentCount={2} />`
   the next time the session list refreshes (existing polling/streaming cadence — no new
   round-trip added by this fix).
5. **User glances at the session list.** Sees `⏳ Waiting for 2 Tasks` instead of a plain Idle or
   Ready chip, and (per `deriveWorkingState.ts:34`) the session sorts into the "Processing/Active"
   bucket for any Status-based grouping view, never "Idle" — see Surface 4.
6. **Background work finishes.** A subsequent, more-recent line with no shell/monitor mention
   (e.g. a plain `"❯ "` prompt) becomes the most-recent line in the scrollback.
   `detectFromLines`'s backward-scan (`detector.go:596-651`) resolves to that line's status
   instead, `SubagentCount` drops to `0`, and the chip switches to whatever `SubStatus` applies
   next (typically `IDLE`/`READY`, which suppresses the chip in the row view per the
   `subStatus !== SubStatus.READY` guard, `SessionRow.tsx:349`).

No click, no dead end to design for — the user's only "action" here is choosing not to kill/restart
a session that shows this chip, which is a judgment call the chip's label already supports (see
Step 3, AC-UX-3).

### Error and edge-case handling

| Case | What the user sees | Source |
|---|---|---|
| Count is `0`/negative/`NaN`/`undefined` | Chip falls back to unnumbered `"⏳ Waiting for Agents"` rather than a wrong or missing number | `SubStatusChip.tsx:41-44` (`hasCount` guard) |
| Singular count (`1`) | `"Waiting for 1 Task"` (singular), not "1 Tasks" | `SubStatusChip.tsx:45,61` |
| Detector doesn't recognize the line at all (unrecognized future CLI format) | No chip for this state — falls through to whatever the next-most-recent recognized line says, or plain Idle. Fails *quiet*, not misleading-wrong: this matches every other `SubStatusChip` case's `default:` behavior (render nothing, `console.warn`, never throw) | `SubStatusChip.tsx:189-198` |
| Session list hasn't refreshed yet after the underlying line changed | Chip briefly shows the previous count/state until the next poll/stream tick — no new staleness risk introduced by this fix beyond what every other chip already has | Existing polling cadence, unchanged by this plan |
| A monitor/shell that never resolves ("stuck forever") | Chip keeps showing `"⏳ Waiting for N Tasks"` indefinitely — **explicitly out of scope** per `implementation/plan.md`'s Out-of-Scope list and `research/ux.md` §4's staleness-threshold idea; not a gap in this feature, a deferred one | `implementation/plan.md` "Out of Scope" |

---

## Step 2 (cont.): Surface 2 — row/list view, condensed

Same component, same props, different surrounding layout (`SessionRow.tsx:349-361` vs.
`SessionCard.tsx:787-792`) and a slightly different visibility guard: the row view additionally
suppresses `SubStatus.READY` (`!== SubStatus.READY`, `SessionRow.tsx:349`) where the card view
suppresses `IDLE`/`UNSPECIFIED` via a different condition (`SessionCard.tsx:788-789`) — both
already correctly *never* suppress `WAITING_FOR_AGENT`, so the chip's presence is consistent
between the two mount points; only its neighboring badges (path, stale indicator) differ.

```
Row:  my-feature-branch  ⏳ Waiting for 2 Tasks   🟠 Stale   [worktree path]
```

No separate acceptance criteria needed beyond Surface 1's — same component, same a11y attributes,
same fallback behavior.

---

## Step 2 (cont.): Surface 3 — detection input/output, condensed

Non-interactive: a regex match over PTY scrollback text, not something a user clicks or types
into. Representative sample (post-fix):

```
input:  "✻ Cogitated for 1m 6s · done 3:06 PM · 1 shell, 1 monitor still running"
output: DetectedStatus = WAITING_FOR_AGENT, PatternName = "shells_still_running", SubagentCount = 2
```

Acceptance criteria (maps to requirements.md AC1/AC5/AC6):
- Comma-joined `"N shell(s), M monitor(s) still running"` sums both counts, matching the short
  footer form's existing summation behavior (`detector.go:40-61`).
- Monitor-only lines (no shell mention) continue to produce the correct single count — no
  regression.
- Singular and plural phrasing (`shell`/`shells`, `monitor`/`monitors`) both match.
- A zero-sum match is treated as no-match (falls through), never surfaced as
  `WAITING_FOR_AGENT` with count `0`.
- Full `session/detection/...` and `session/...` test suites pass unchanged (AC6) — see
  `implementation/plan.md` Task 1.1.1d.

---

## Step 2 (cont.): Surface 4 — status-grouping bucket placement, condensed

Non-interactive/internal, but directly answers the open question `research/ux.md` §2 raised
("verify a session in WAITING_FOR_AGENT sorts into an active/busy bucket, not an idle one").
**Verified, already correct** — `deriveWorkingState.ts`:

```
SubStatus.WAITING_FOR_AGENT       → WorkingState.PROCESSING   (line 34)
DetectedStatus.WAITING_FOR_AGENT  → WorkingState.ACTIVE        (line 49, UNSPECIFIED-subStatus fallback path)
```

Neither path maps to `WorkingState.IDLE`. This closes the loop the research doc flagged as a risk
("could be misread as idle... at the aggregation layer") — no code change needed here; recorded
for traceability since it was an open question, not a settled fact, going into this design pass.

Acceptance criteria:
- A session with `SubStatus.WAITING_FOR_AGENT` never appears in an "Idle" bucket of any
  Status-grouped view (Tag Organization's Status strategy).

---

## Step 3: UX acceptance criteria

1. **Correct count, at a glance, zero extra steps.** A user viewing the session list sees the
   true combined outstanding-work count (shells + monitors) in the existing chip with **0 clicks**
   — it renders automatically, no expansion/hover required to see the number.
2. **Singular/plural grammar is correct.** `"Waiting for 1 Task"` for count `1`,
   `"Waiting for N Tasks"` for count ≠ 1 — verified by existing chip logic
   (`SubStatusChip.tsx:45,61`), unchanged by this fix, still true after it.
3. **The chip is distinguishable from Idle/Ready/NeedsApproval by three independent signals**,
   not color alone (WCAG 1.4.1): distinct label text ("Waiting for N Tasks" vs. "Idle" vs.
   "Approve Tool Use"), distinct icon glyph (⏳ vs. ● vs. ⚠), and a distinct CSS class
   (`chipWaitingForAgent` vs. `chipIdle` vs. `chipNeedsApproval`,
   `SubStatusChip.css.ts:23-30,72-80,111-120`).
4. **Screen-reader accessible.** `role="status"` triggers a polite live-region announcement on
   change; `aria-label="Waiting for agents"` carries the state independent of the emoji glyph
   (which is not separately wrapped in `aria-hidden` today — see Gap below); `title` gives
   hover users the fuller "Claude is waiting for N background tasks to finish" text.
5. **No dead end.** There is no error state to exit from — the chip either shows a number or
   falls back to an unnumbered but still-accurate "Waiting for Agents" label
   (`SubStatusChip.tsx:41-44,58`); it never shows a wrong count or a broken/blank chip.
6. **The count clears correctly.** Once the most-recent scrollback line no longer reports
   outstanding shells/monitors, the chip stops showing `WAITING_FOR_AGENT` on the session's very
   next status refresh (requirements.md AC3) — testable by a human as: run a command that
   completes, wait for the next poll tick, confirm the chip changes.
7. **Consistent across surfaces.** The same count and label appear whether the session is
   viewed as a card or a list row (Surfaces 1/2) — testable by switching view modes and comparing
   the chip text.
8. **No misclassification into "idle."** In any Status-grouped view, a session showing this chip
   never lands in the same visual bucket as a genuinely idle session (Surface 4).
9. **Color contrast ≥ 4.5:1.** `chipWaitingForAgent` reuses the same `vars.color.primary` /
   `vars.color.accentBg` token pair as `chipProcessing` (`SubStatusChip.css.ts:32-39,111-120`),
   which is the established, already-shipped "Thinking…" chip — no new token combination is
   introduced by this fix, so no new contrast risk. Not independently re-measured in this pass
   (see Gap below).

## Gaps found (informational, not new tasks — see reasoning below)

Two items came up during this pass that are genuine observations but are **not** being proposed
as new work, because the plan and research already considered and explicitly deferred them:

- **Emoji glyph not wrapped in `aria-hidden="true"`.** `research/ux.md` §3 states the repo's
  established pattern wraps the icon glyph in its own `aria-hidden` span, but
  `SubStatusChip.tsx:61`'s `⏳ Waiting for...` renders the emoji as plain text content inside the
  `role="status"` span, not in a separate `aria-hidden` wrapper — same as every other case in this
  file (`✔`, `⚠`, `●`, etc. at lines 87, 99, 111, etc.), so this isn't specific to
  `WAITING_FOR_AGENT`. Screen readers generally either skip or mispronounce the glyph while still
  reading the surrounding text, so the `aria-label` already carries full meaning independent of
  it — functionally fine, just not literally matching the pattern description. Pre-existing
  across the whole component, out of scope for a fix scoped to backend counting.
- **No live re-measurement of rendered contrast ratio.** AC-UX-9 above reasons by token reuse
  (same tokens as the already-shipped `chipProcessing`) rather than pulling actual computed
  colors and running a contrast calculation. If a stricter check is wanted, that's a quick
  standalone a11y audit of `SubStatusChip.css.ts`'s whole chip family, not something specific to
  this feature.

Per this task's scope instructions, neither is added as a new implementation task — they predate
this feature and aren't caused or worsened by the backend regex/count fix.
