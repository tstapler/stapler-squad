# UX Design: pi-support

SDD Phase 3 design artifact. Builds on `requirements.md` and `research/ux.md`;
scoped to the concrete surfaces `implementation/plan.md` already decided
(Phase 3 Epic 3.1 / Stories 3.1.1–3.1.2, Phase 4 Epic 4.2 Story 4.2.2). This
doc does not re-open those decisions — it specifies wireframes, flows, error
states, and testable acceptance criteria for them.

## Surface inventory (Step 1)

| # | Surface | Interactive? | Plan reference |
|---|---|---|---|
| 1 | Program picker entry (`PROGRAMS`) | Yes — user clicks/selects | Story 3.1.1 |
| 2 | Session-creation panel capability warning | Yes — passive-then-alert on program change | Story 3.1.2 |
| 3 | Session-card health badge | Yes — hoverable/focusable, but no click action | Story 4.2.2 |
| 4 | Resume behavior | No — silent, matches Claude | Phase 2 (`ClaudeCommandBuilder`-equivalent), Epic 2.2 |
| 5 | `ssq-hooks install pi` CLI output | No — CLI/log output | Story 4.1.2 |
| 6 | Health-ping endpoint / server log | No — internal signal, no UI of its own | Story 4.2.1 |

Resume (surface 4) is explicitly **not** a user-facing decision point: per
`research/ux.md` §2, "resume works the same way it does for Claude —
silently." No confirmation dialog, no visible flag — a user who reconnects to
a stopped pi session just sees their prior conversation continue, exactly as
with Claude Code. The only place this becomes *visible* is indirectly, via
surface 3 (the health badge) if the resumed session's extension fails to
reload — that failure path is covered under surface 3's error states below,
not as a separate surface.

---

## Non-interactive surfaces (condensed)

### Surface 5: `ssq-hooks install pi`

Installs the pi approval extension (`ssq-approval.ts`). Representative output (mirrors `installOpenCode()`'s existing print format):

```
$ ssq-hooks install pi
Detected pi version: 0.4.2
Installed ~/.local/bin/ssq-hooks
Wrote ~/.pi/agent/extensions/ssq-approval.ts
```

Acceptance criteria:
- Exits 0 and prints the extension's absolute path on success; a re-run with
  no changes prints identical output (idempotent, per Story 4.1.2 AC).
- If pi isn't installed/found, prints `pi not found on PATH — install pi
  first (see https://pi.dev/docs)` to stderr and exits non-zero rather than
  silently writing a dead extension file.
- If `~/.pi/agent/extensions/` can't be created (permissions), prints the
  underlying OS error, not a generic "install failed."
- Output never claims "enforcement active" — installing the file is
  necessary but not sufficient (see Surface 3); wording stays scoped to
  "installed," matching the loaded/installed distinction in requirements.md.

### Surface 6: Health-ping endpoint / server log

Representative log line (`staplersquad.log`, JSON-lines):

```json
{"level":"WARN","msg":"pi extension health: no ping within grace window","session_id":"sess_abc123","program":"pi","grace_window_s":10}
```

Acceptance criteria:
- A `Loaded` transition and a `Failed` transition each produce one log line
  at session scope, greppable by `session_id`.
- The grace-window timeout is stated in the log line (not just in code),
  so a user debugging via `docs/how-to/debug-with-logs.md` doesn't need to
  read source to know the window.
- No log spam: one line per state transition, not one per missed ping.

---

## Interactive surfaces (Step 2 + Step 3)

### Surface 1: Program picker entry

**This entry is flag-gated.** The "pi" `<option>` renders only when
`pi-support` is on (Story 3.1.1's `getPickerPrograms(piSupportEnabled)`
helper). With the flag off, the picker looks and behaves exactly as it does
today — no "pi" row, no disabled/greyed entry, nothing — matching this
doc's own Cross-surface "Opt-in invisibility" criterion below. The
wireframe below shows the flag-on state.

**Wireframe** (Advanced Options section, `OmnibarCreationPanel.tsx:916-936`),
**with `pi-support` on**:

```
┌ Program ──────────────────────────────────┐
│ [ Claude Code                          ▾ ] │
│   Claude Code                              │
│   Claude Code (proxy)                      │
│   pi                          ← new, here, only when pi-support is on │
│   Aider                                    │
│   Aider (Ollama)                           │
│   OpenCode                                 │
│   Gemini CLI                               │
│   Antigravity                              │
│   Terminal                                 │
└─────────────────────────────────────────────┘
```

This is a native `<select>` — no new widget. pi is inserted as one more
`<option>`, positioned per Story 3.1.1's AC: after both Claude entries,
before Aider — and present in the rendered list only while the flag is on.

**Interaction flow**:

1. User opens the session-creation panel (Omnibar), expands "Advanced
   Options" if collapsed.
2. User opens the Program `<select>` (click, or `Space`/`Enter` via
   keyboard focus — native control, free).
3. User selects "pi" (click, or arrow keys + `Enter`).
4. System sets `program = "pi"` via `setFormField("program", ...)`. No
   network call yet — this is client-side form state.
5. Downstream reactions fire synchronously in the same render:
   - `isProgramRecognized` re-evaluates (`pi` is now in `availablePrograms`,
     so no "not found in PATH" warning — unless pi genuinely isn't
     installed, see error states below).
   - `isAutoApproveSupported("pi")` is `false` (pi isn't in
     `AUTO_APPROVE_SUPPORTED_AGENTS`) → Auto-approve checkbox disables with
     its existing hint text, per current behavior for any unsupported agent.
   - `isApprovalExtensionSupported("pi")` is `true` → the Surface 2 warning
     area becomes live (see below) rather than hidden.
6. User proceeds to create the session as normal (Create button), unchanged
   flow.

**Error / edge cases**:

| Condition | What the user sees | Exit path |
|---|---|---|
| pi selected but not actually on `PATH`/installed | `isProgramRecognized` still returns `true` (it's a known preset, not a PATH probe) for the picker itself — the "not found in PATH" warning is driven by `useAvailablePrograms()`'s runtime probe, so if that probe also checks pi specifically and it's missing, the existing `preset-program-warning` span appears: `"pi" not found in PATH — check it's installed` | User installs pi or picks a different program; Create is not blocked (matches existing free-text-program precedent — a warning, not a hard stop) |
| User types a pi-like string manually (e.g. `"pi --flag"`) instead of using the picker | Same `preset-program-warning` treatment as any unrecognized free-text program today — no pi-specific regression | Pick from dropdown, or proceed anyway |

**UX acceptance criteria**:
- AC1: User can select pi in ≤ 2 actions from an already-open creation panel
  (expand Advanced Options if needed, then one select interaction) —
  matches selecting any other program.
- AC2: `<select id="omnibar-program">` keeps its existing `<label
  htmlFor="omnibar-program">` association — screen readers announce "Program,
  combo box, pi" (or current selection) with no additional markup needed.
- AC3: Focus order is unchanged (native `<select>` insertion doesn't alter
  tab order elsewhere in the form).
- AC4: No dead end — picking pi and changing your mind is a single
  re-selection, same as any other program; nothing about picking pi commits
  the user irreversibly.
- AC5: With `pi-support` off, the rendered `<option>` list has no "pi" entry
  at all (not disabled, not hidden-but-present in the DOM) — testable via
  `getPickerPrograms(false)` excluding it, per Story 3.1.1's AC.

---

### Surface 2: Session-creation panel capability warning

**Wireframe** — three states, same DOM location under the Program field:

```
State A — program != "pi", or pi + healthy/not-yet-applicable:
┌ Program ──────────────────────────────────┐
│ [ pi                                   ▾ ] │
└─────────────────────────────────────────────┘
(no warning row)

State B — program == "pi", extension health = unhealthy (failed):
┌ Program ──────────────────────────────────┐
│ [ pi                                   ▾ ] │
└─────────────────────────────────────────────┘
⚠ Approval extension not loaded for pi — tool calls           [role="alert"]
  will run WITHOUT rule enforcement for this session.

State C — program == "pi", health unknown (pre-grace-window):
┌ Program ──────────────────────────────────┐
│ [ pi                                   ▾ ] │
└─────────────────────────────────────────────┘
(no warning shown at creation time — health isn't known until the session
 starts; see "Session-creation time" row in error table below for the
 alternative failure this state maps to: trust-gate block)
```

**Interaction flow**:

1. Continues from Surface 1, step 5: `isApprovalExtensionSupported("pi")`
   evaluates `true`, which *enables* the warning area (vs. e.g. OpenCode,
   where the check is `false` and the area never renders at all — it is not
   "supported but always healthy," it is "eligible to show a health-based
   warning").
2. Phase 3 ships this as a stubbed prop (Task 3.1.2c); Phase 4 (Task 4.2.2c)
   wires it to the real per-session health signal. From the user's
   perspective this is invisible — the same UI, just backed by a real signal
   once Phase 4 lands.
3. At creation time there is no live session yet, so "health" here reflects
   either (a) a cached last-known-bad state for a resumed/duplicated
   configuration, or (b) nothing (state C, most common — new session, health
   isn't knowable pre-launch). The warning is therefore mostly a
   **post-creation** signal in practice; see Surface 3 for the live version
   that actually matters once the session is running. This panel-level
   warning exists per Story 3.1.2's AC primarily for the resumed/duplicated
   case, where a prior session's known-bad health is available before the
   new session starts.
4. User can still click Create with the warning showing — it is a warning,
   not a submit-blocker (consistent with the existing `preset-program-warning`
   pattern's non-blocking precedent, but escalated to `role="alert"` per
   research/ux.md §3 because the stakes — unenforced tool calls — are higher
   than a typo).

**Error / edge cases**:

| Condition | What the user sees | Exit path |
|---|---|---|
| Extension confirmed failed (health = Failed) before creating | `role="alert"` banner: `"Approval extension not loaded for pi — tool calls will run WITHOUT rule enforcement for this session."` | Proceed anyway (explicit informed choice) or switch program |
| pi-support flag itself is off | No warning area rendered at all — the capability check and badge are gated behind the flag per Story 4.2.2's AC; a user who hasn't opted in never sees pi-specific UI | Enable the flag in settings if they want pi |
| pi selected, flag on, but pi isn't installed anywhere on the machine | Combines with Surface 1's `preset-program-warning` (PATH warning) — the two warnings can co-occur; both are non-blocking, stacked in the same field's warning area | Install pi, or pick another program |

**UX acceptance criteria**:
- AC1: When health = Failed, the alert text is exactly: `"Approval extension
  not loaded for pi — tool calls will run WITHOUT rule enforcement for this
  session."` (concrete text, not a placeholder) — mirrors the badge's
  `aria-label` wording from Story 4.2.2 so the two surfaces are internally
  consistent for a user who sees both.
- AC2: The warning uses `role="alert"` (not the passive `programWarning`
  span) — screen-reader users are interrupted/notified, not left to
  discover it visually only, closing the exact gap `research/ux.md` §3
  flags in the pre-existing pattern.
- AC3: No dead end — the alert never blocks the Create button; the user's
  exit path (proceed anyway, or switch program via Surface 1) is always
  available in the same view, no modal, no separate page.
- AC4: Contrast: warning text/icon combination must meet ≥ 4.5:1 against the
  panel background in both light and dark themes — verify against the
  existing `--error`/`--warning` CSS custom properties used elsewhere in
  this file (e.g. `inlineEditError`'s `var(--error)` at `SessionCard.tsx:586`)
  rather than introducing a new ad hoc color.

---

### Surface 3: Session-card health badge

**Wireframe** — badge row, alongside the existing external-session (🔗) and
remote-host (🖥️) badges at `SessionCard.tsx:604-626`:

```
┌ Session Card ────────────────────────────────────────────┐
│ my-pi-session               🔗 iTerm  🖥️ dev-box  🛡️ pi   │
│                                                             │
│ [conversation preview / status pill]                       │
└─────────────────────────────────────────────────────────┘
```

Three badge states (icon + color are illustrative; exact glyph/color chosen
during implementation, but the *shape* — icon + text label, not icon alone —
is fixed by this design):

```
Loaded (healthy):     🛡️ pi              — neutral/green tone
Failed (unhealthy):   ⚠️ pi               — warning/red tone
Unknown (pending):     ◌ pi               — muted/gray tone
```

Placement rule: shown only when (a) the `pi-support` flag is on, and (b) the
session's resolved `program` is pi — never for Claude or other agents, so
the badge row's meaning stays legible ("this row is pi-specific safety
status," not a generic health concept applied everywhere).

**Interaction flow**:

1. Session card renders; `program === "pi"` and flag is on →
   the badge component mounts, initial render state = Unknown (per Story
   4.2.2's AC: never defaults to Loaded).
2. Server-side: pi process starts, extension attempts load. Within the
   grace window (~10s, tunable), either:
   - a ping arrives at `/api/hooks/pi-extension-loaded` → tracker flips to
     `Loaded` → next poll/subscription update re-renders the badge to the
     healthy state.
   - no ping arrives → tracker flips to `Failed` at grace-window expiry →
     badge re-renders to the failed state.
3. User hovers (mouse) or focuses (keyboard, if the badge is made
   tabbable/has a `title`) the badge → sees the full-text tooltip (mirroring
   the existing `title` attribute pattern used by the external-session and
   host badges at lines 608/619).
4. No click action — this is a status indicator, not a control (consistent
   with existing badges in this row, none of which are interactive).
5. If a running session's extension later crashes or is unloaded mid-session
   (e.g. pi subprocess restarts without reinjecting the extension), the
   health tracker should treat the absence of a *repeat* ping the same way
   as initial failure — this doc treats that as the same `Failed` state and
   the same badge, not a fourth state; Phase 4's implementation should
   confirm whether pings are one-shot ("loaded once") or periodic
   ("still loaded") — if one-shot, the plan should note this as a known gap
   (badge can go stale-green after a later mid-session extension crash)
   rather than silently presenting it as fully covered.

**Error / edge cases**:

| Condition | What the user sees | Exit path |
|---|---|---|
| Extension failed to load (trust-gate blocked, or file missing) | Badge shows Failed state; `aria-label="pi approval extension: not loaded — tool calls are unenforced"` (exact text from Story 4.2.2's AC) | User can open a terminal/session view and run `ssq-hooks install pi` again, or check trust-gate status; badge is diagnostic, not itself an action button — exit path is "go fix it outside this card," which is acceptable since this is a status indicator, not a wizard |
| pi subprocess crashes mid-session (process dies entirely) | This is a **lifecycle** failure, not an extension-health failure — per research/ux.md §4, it should surface via the existing `SessionStatus`-driven status pill (`statusCrashed` or equivalent), not the health badge. The health badge for a dead session should not claim any state — recommend hiding the badge (or freezing it at last-known state with a "session ended" qualifier) once `SessionStatus` indicates the process is gone, so a stale "Loaded" badge doesn't imply an extension is still running when nothing is | Restart/resume the session (existing session-lifecycle affordances) |
| The separate, status-only `pi --mode json` subprocess dies while the interactive pi session is still alive and healthy (Story 5.2.3's `detection.StatusUnavailable`) — distinct from the row above: the extension is still loaded and enforcing, only the *status-reporting* channel is down | This is neither an extension-health failure nor a full session crash — the health badge is unaffected (still shows Loaded/Failed per the extension's own ping), but the session-list **status pill** shows a distinct "status unavailable" state (not `StatusIdle`, per Story 5.2.3's AC) once bounded relaunch retries are exhausted, so a quiet session isn't confused with one whose status feed just went dark | No user action required while retries are in flight (automatic relaunch); if retries are exhausted, the session itself is still usable via its terminal — the status pill's "unavailable" state is diagnostic, same non-blocking treatment as the health badge's Failed state |
| pi-support flag on, program is pi, but pi itself isn't installed at all | Session creation should have already surfaced this via Surface 1/2's PATH warning; if the user proceeded anyway, the session likely fails to start entirely — surfaced via `statusCreationFailed` (existing status), not the health badge, since there's no process to report extension health for | Fix PATH/install pi, retry session creation |
| Health flips Loaded → then a late/duplicate ping arrives after Failed | Per plan's Task 4.2.1e intended behavior ("still flips to Loaded"), badge transitions back to healthy — this is correct UX: a late-but-successful load is still real enforcement, and the badge should reflect current truth, not the worst historical state | N/A — this is a recovery, not an error |

**UX acceptance criteria**:
- AC1: Badge never renders in the "Loaded" (healthy-looking) state before a
  positive signal arrives — testable: mount a fresh pi session's card,
  assert badge state is Unknown, not Loaded, in the first render.
- AC2: `role="img"` + exact `aria-label` text per state:
  - Failed: `"pi approval extension: not loaded — tool calls are unenforced"`
  - Loaded: `"pi approval extension: loaded — tool calls are enforced"`
  - Unknown: `"pi approval extension: status unknown"`
  (Loaded/Unknown text is this design's proposal, filling the placeholder
  the plan left open for the non-Failed states — pick this exact wording
  during implementation unless a reviewer objects, since Failed's wording is
  the only one the plan pins verbatim.)
- AC3: Color is never the *only* signal distinguishing states — text label
  ("pi") plus distinct icon glyph per state, so the badge is legible without
  color (contrast/colorblind-safe), consistent with `research/ux.md` §3's
  requirement that the visual glyph be decorative and the label carry
  semantics.
- AC4: Contrast ≥ 4.5:1 for the badge's text against its background in all
  three states, both light and dark themes.
- AC5: No dead end for the Failed state — the tooltip/label text itself
  names the consequence ("tool calls are unenforced"), giving the user
  enough information to decide to stop the session rather than leaving them
  to guess what "failed" means.
- AC6: Badge is keyboard-discoverable: if implemented as a focusable element
  (like the existing badges' `title` attribes suggest mouse-hover tooltip
  behavior only), it must also expose its text to keyboard/screen-reader
  users without requiring hover — the existing `aria-label` on the wrapping
  span already satisfies this for AT users; no separate keyboard action is
  needed since there's no click behavior (AC per Surface 3, interaction
  flow step 4).

---

## Cross-surface acceptance criteria

- **Consistency**: the Failed-state wording used in Surface 2's alert and
  Surface 3's badge `aria-label` describe the same consequence ("tool calls
  … unenforced") in near-identical language, so a user encountering both
  doesn't have to reconcile two different claims about what's wrong.
- **No dead ends anywhere**: every error/edge-case row across all three
  surfaces above has a stated exit path; none require contacting support or
  leave the user with no next action.
- **Opt-in invisibility**: with the `pi-support` flag off, none of these
  three surfaces render anything beyond the status quo — zero-diff for
  existing Claude-only users (per requirements.md's "existing Claude-only
  workflows are unaffected until explicitly enabled").
- **Accessibility floor**: every new interactive element either reuses an
  existing labeled-control pattern (`<label htmlFor>` for the picker) or
  follows the `role="img"`/`role="alert"` + explicit `aria-label` pattern
  already established in `SessionCard.tsx` — no new bare icon-only or
  div-as-button controls are introduced by this design.
- **CI backstop**: all three surfaces live under `web-app/src/`, so Axe Core
  (WCAG AA, blocking) and Lighthouse CI (score < 70 warning) already gate
  any regression per the top-level `CLAUDE.md`'s CI note — this design's
  accessibility criteria above are the pre-CI bar, not a substitute for it.
