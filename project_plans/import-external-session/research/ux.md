# UX Research: import-external-session

Agent 5 — Phase 2 (Research). Feature: discover unmanaged external Claude/Antigravity
sessions (plain terminal, raw tmux, or `ssq-mux`-wrapped IDE terminal), import them into
stapler-squad as managed `Instance`s with full history, then optionally kill the original
on explicit confirmation. Batch import and multi-program (`AgyAdapter`) support in scope.

## 0. Repo grounding (existing surfaces/conventions checked first)

- Backend already has `session.ExternalSessionDiscovery` (`session/external_discovery.go`)
  producing `*Instance` records with `InstanceType` external/discovered, wired through
  `server/dependencies.go`, `server/services/session_service.go`,
  `server/services/terminal_service.go`, `server/services/checkpoint_service.go`, and
  `server/services/external_websocket.go`. `session/instance_tmux.go:KillExternalSession`
  already exists — kill is not new plumbing, it needs a confirmed UI trigger.
  `session/external_approval.go` suggests an approval/consent concept already exists for
  externally-discovered sessions — reuse its vocabulary rather than inventing new terms.
- Proto (`web-app/src/gen/session/v1/types_pb.ts`) already has `InstanceType` distinguishing
  managed vs. "discovered externally (e.g. via ssq-mux) with limited interaction" and an
  `ExternalInstanceMetadata` message — the data model has a head start; there is currently
  **no frontend surface** rendering this data as a browsable/importable list (grep for
  "Discovered"/"external" in `web-app/src` only turns up terminal-stream and workspace-switch
  plumbing, not a discovery UI).
- Closest existing interaction patterns to imitate, not invent:
  - **Confirm/action modal shape**: `web-app/src/components/sessions/ResumeSessionModal.tsx`
    — `role="dialog" aria-modal="true" aria-labelledby="resume-modal-title"`, local form state,
    `isSubmitting` guard. This is the template for the "confirm import" and "confirm kill"
    dialogs.
  - **Batch checkbox table**: `web-app/src/components/sessions/ApprovalAnalyticsPanel.tsx` —
    header "select all" checkbox (`aria-label="Select all"`) driving row checkboxes, row click
    guarded with `stopPropagation` on the checkbox cell. This is the template for the batch
    import picker.
  - **Radio-style mode picker**: `OmnibarCreationPanel.tsx`'s `SESSION_TYPES` with
    `aria-checked` on pseudo-radio buttons — relevant if import becomes a session-creation mode
    per `.claude/rules/session-creation-registry.md`.
  - Modals/lists must be built with vanilla-extract `.css.ts` colocated files per
    `.claude/rules/css-architecture.md` — no hardcoded z-index (add a named slot to the shared
    `zIndex` contract), no ad-hoc hex colors, portal any overlay via `createPortal`.
  - All new interactive elements must use `data-testid` or ARIA role locators, per
    `.claude/rules/e2e-test-conventions.md` — this constrains the picker/dialog markup to
    testid-addressable rows/buttons rather than positional CSS selectors.

## 1. Comparable UX patterns ("discover unmanaged → select → confirm import → optionally destroy original")

| Product / flow | What it does well | What to avoid |
|---|---|---|
| **Docker Desktop "Import" / `docker container commit` + adopt-unmanaged-container flows** | Shows a clear list of things Docker didn't create itself (e.g. containers started via CLI outside Compose) with status badges before any action is taken. Never auto-destroys the source. | Docker's "prune" flows are the cautionary tale: broad, low-visibility destructive actions with a single confirm checkbox that people click through without reading. Don't model the kill-confirm on "Prune" — it under-communicates what's being deleted. |
| **Browser "Import bookmarks/passwords from another browser"** (Chrome/Firefox/Safari import wizard) | Multi-step wizard: (1) pick source, (2) **preview counts and categories** of what will be imported ("237 bookmarks, 12 passwords"), (3) confirm, (4) success summary with exact counts imported. Source browser/profile is *never* deleted or modified — it's copy-only. | Browsers deliberately have **no** "and now delete the original profile" step — that's a meaningfully different (harder) UX problem this feature must solve that the analog doesn't. Borrow the preview-with-counts step; don't borrow "no destructive step exists." |
| **git "adopt untracked files" (`git add` prompts in GUI clients like GitKraken/Fork)** | Untracked files are listed with clear visual distinction (different color/icon) from tracked ones before staging; you explicitly select which to adopt; nothing is deleted. | N/A — again, no destructive counterpart. Useful mainly for the "visually distinguish adoptable-but-not-yet-adopted items" pattern for the discovery list. |
| **IDE "Attach to running process" dialog** (IntelliJ/VS Code "Attach to Process") | Live-refreshing list of candidate processes with disambiguating metadata (PID, command line, cwd) so the user can tell processes apart when several look similar — directly analogous to the "multiple JSONL candidates for one process" ambiguity in this feature. Filterable/searchable list when there are many candidates. | These dialogs sometimes show a raw PID list with no human-readable label, forcing the user to know PIDs by heart — for this feature, always attach the derived label (project path, branch, first line of last prompt) rather than raw PID/socket path. |
| **Email client "Import mailbox" wizard** (Thunderbird/Outlook `.mbox`/`.pst` import) | Explicit **progress + per-item result** for batch operations — a list of items with a spinner→check/x state transition per item, not one aggregate spinner. Ends with a summary: "48 imported, 2 skipped (duplicate), 1 failed — [view error]." | Some implementations block the whole UI for the duration of a large batch import with no per-item feedback, so users can't tell if it's frozen or working — for this feature (likely seconds not hours, but still) show per-row state, not just one global spinner. |
| **Slack/Teams "sign out other sessions" / "log out all other devices"** | The closest analog to "confirm-before-kill of something running elsewhere": always a separate, explicit, named action from the primary flow, always requires typing/clicking a distinct confirm control, always names exactly what will be terminated (device name, location, last-active time). Never bundled silently into another action. | Never make killing the external session an implicit side effect of clicking "Import" — it must be its own step the user cannot reach without first seeing import succeed. |

**Synthesis of the good pattern to build**: browser-import's "preview with counts, copy-only" model
+ Slack's "separately confirmed, explicitly named target" model for kill + email-wizard's
"per-item batch progress and summary" model. That combination is the shape of this feature and
matches the requirements' "verify first, kill on confirmation" principle already stated in
`requirements.md`.

## 2. User mental models and expectations

- **Default assumption when a user says "import my session": the original is left alone unless
  they separately ask for it to be removed.** "Import" reads as *copy*, not *move*, by default
  in every analog above (browsers, email, git). The requirements doc already gets this right
  ("kill only after explicit confirmation") — the UX must reinforce it by **never defaulting the
  kill-confirmation checkbox/toggle to checked**, and by using verb "Import" for the primary
  action and a visually and temporally separate "End original session" as an opt-in follow-up,
  never a combined "Import & Close" primary button.
- **Amendment (Phase 4 triad-review repair — SIGSTOP-at-commit disclosure gap)**: the backend
  plan (Task 1.2.1e) `SIGSTOP`s the original process at the moment the user clicks "Confirm
  Import" — before any separate "End original session" step — to close a dual-writer race. This
  is a real, user-visible pause (the original terminal will stop echoing/updating) that happens
  *earlier* than the copy-only mental model above implies, so it must be disclosed, not left as a
  silent side effect the user has to infer from a frozen terminal:
  - The confirm-import dialog must show copy to this effect before the user clicks "Confirm
    Import," e.g. *"Your original terminal session will briefly pause while we complete the
    import."*
  - The original session's row/reference must show a visible "paused" state indicator from the
    moment `Confirm Import` is clicked until the process is either resumed (user cancels/abandons
    the kill step) or the kill step completes — so a user glancing at that row mid-import sees
    *why* the terminal looks frozen instead of assuming something broke.
  - This does not change the "Import" verb or the "copy, not move" framing — the process is
    suspended, not terminated, and remains fully resumable — but the pause itself must be as
    explicit as the later kill-confirmation step, not bundled silently into "Import."
- **Users expect a preview before committing**, especially because the object being imported
  (a Claude conversation with potentially hours of context) is expensive to lose and hard to
  visually verify after the fact. Minimum preview content per candidate: project path / working
  directory, program (Claude Code vs. Antigravity), approximate turn count or last-modified
  timestamp of the JSONL, and — if resolvable cheaply — the first few words of the most recent
  user prompt (a "the last thing this session was doing" cue people can pattern-match against
  their own memory of what they were doing in that pane).
- **Users expect the imported session to "pick up where it left off"** — i.e. after import, the
  managed Instance's conversation view should look identical to what they'd have seen in the raw
  terminal, and the terminal/tmux pane underneath should be attachable/resumable the same way any
  other managed session is. Any deviation (e.g. a subtly different working directory, a
  worktree that wasn't there before) needs to be surfaced explicitly in the preview, not
  discovered after the fact.
- **Ambiguity is scarier here than in most import flows** because there's a real chance of
  picking the *wrong* JSONL/process and then killing the *right* one — users will trust the UI's
  disambiguation cues over their own memory once several look similar, so those cues (path,
  timestamps, prompt snippet) must be accurate and prominent, not an afterthought in a tooltip.
- **Batch import mental model**: users likely think of "import all of these" as one intent but
  will still expect to see (and be able to deselect) individual items — the Chrome bookmark
  import model of "select which categories/items" rather than an unconfigurable all-or-nothing
  switch. They will not expect one failure to silently roll back successes elsewhere in the
  batch (see partial-success handling in §4).

## 3. Accessibility requirements (WCAG / ARIA / keyboard)

### Session-picker list/table (discovery view)
- Use a real `<table>` with `<th scope="col">` headers, or if a list-of-cards layout is used
  instead, wrap it in `role="table"`/`role="row"`/`role="cell"` (or simpler, `role="list"` /
  `role="listitem"` if it's not tabular data) — do not build a `<div>` soup with no semantic
  structure; screen readers need row/column relationships to announce "row 3 of 7."
  `ColumnPicker.tsx`'s `role="listbox"` and `ApprovalAnalyticsPanel.tsx`'s `<table>` are both
  acceptable precedents already in this codebase — reuse whichever shape fits (table for the
  discovery list, since it has several comparable columns: path, program, last-active, actions).
- Every row needs a stable `data-testid` (e.g. `discovered-session-row-{id}`) per
  `.claude/rules/e2e-test-conventions.md`, plus an accessible name that includes the
  disambiguating info (`aria-label="Claude session in ~/proj, last active 2m ago"`), so
  assistive tech and Playwright locators both work without relying on visual column position.
- Keyboard: full list must be operable without a mouse — `Tab`/`Shift+Tab` between rows'
  interactive elements, `Space`/`Enter` to toggle a row's checkbox, and if the list can grow long,
  arrow-key roving tabindex (`role="grid"` + `aria-rowindex`/`aria-colindex`) is preferable to
  forcing many Tab stops per row.
- Live region (`aria-live="polite"`) announcing count changes as discovery updates in real time
  (the backend actively polls `mux.Discovery`), e.g. "3 external sessions found" — otherwise a
  screen reader user has no way to know new rows appeared without re-scanning the whole table.
- Loading/empty/error states must not be conveyed by color or icon alone (WCAG 1.4.1) — always
  pair with text.

### Destructive confirm dialog (kill-after-import)
- `role="dialog" aria-modal="true"` + `aria-labelledby` pointing at a heading that names the
  specific target ("End external session in ~/proj (PID 48213)?") — generic titles like "Are you
  sure?" fail WCAG 2.4.6 (headings must describe purpose) and fail the Slack-analog principle in
  §1 of always naming what's being terminated.
- Focus must move into the dialog on open (`ResumeSessionModal.tsx` already does this via a ref
  + `useEffect`) and return to the triggering element on close/cancel (focus trap + restoration,
  WCAG 2.4.3).
- `Escape` closes/cancels; the destructive action itself must **never** be the initial-focus
  element — initial focus goes to Cancel, matching common destructive-dialog convention (this
  guards against accidental Enter-key confirmation) and is stricter than what
  `ResumeSessionModal.tsx` needs since that dialog's action isn't destructive.
- If a "type to confirm" or "hold to confirm" pattern is used for extra friction (reasonable
  given "no undo"), it must have a text label / instructions programmatically associated via
  `aria-describedby`, not conveyed by placeholder text alone.
- Color contrast: destructive button must meet 4.5:1 against its background per the existing
  `--error`/`--error-bg` tokens in `globals.css` — reuse those tokens, don't introduce a new red.

### Batch selection UI (checkboxes + bulk action)
- Header checkbox needs `aria-label="Select all discovered sessions"` (matches
  `ApprovalAnalyticsPanel.tsx`'s existing `aria-label="Select all"` convention) and must expose
  tri-state (`indeterminate` DOM property, not just `aria-checked="mixed"` alone — both should be
  set, since browsers/screen readers rely on different signals) when some-but-not-all rows are
  selected.
- Bulk action bar (e.g. "Import 3 selected") should only render/enable when ≥1 row is selected,
  and its enabled state change should be announced (`aria-live="polite"` region reporting
  "3 sessions selected") so keyboard/screen-reader users get the same feedback sighted users get
  from the count updating visually.
- Row checkboxes need programmatic association with their row's label (`aria-label` naming the
  session, not a bare unlabeled checkbox) — do not rely on visual adjacency alone.

## 4. Error states and edge cases

- **Import succeeds, kill fails**: never leave the user unsure which state they're in. The
  managed Instance must be shown as fully imported/healthy immediately (its own success is not
  contingent on kill), and the kill failure surfaces as a distinct, dismissible error tied to the
  *external* session's row/status, e.g. "Import complete. Could not end the original session
  (process no longer found / permission denied) — you may need to close it manually." Do not let
  a kill failure roll back or mark the import as failed; they are sequential but independent
  outcomes, matching the requirements doc's explicit ordering ("kill only after import verified
  successful"). Because a failed kill leaves the original process still `SIGSTOP`'d (it is not
  resumed on failure), the row's "Paused" indicator must remain visible alongside this error
  message — clearing it would wrongly suggest the process resumed on its own, when in fact it
  stays suspended until the user retries the kill.
- **No external sessions discovered (empty state)**: state clearly what was searched (ssq-mux
  socket discovery + any configured JSONL scan roots) and why nothing showed up — "No unmanaged
  Claude or Antigravity sessions found. Sessions must be running locally and, for IDE terminals,
  wrapped with `ssq-mux`." Include a link/hint to `ssq-mux` setup docs
  (`.claude/docs/pty-multiplexing.md`) rather than a bare "nothing here" message — this is a
  discoverability moment, not just an empty list.
- **Ambiguous correlation (multiple JSONL candidates for one process)**: do not auto-pick the
  most-recently-modified file silently — surface the row as "needs disambiguation" (a distinct
  visual/ARIA state, not just another row) and require the user to pick from a short sub-list of
  candidate JSONLs, each labeled with last-modified time and a short excerpt of the last message,
  before Import is enabled for that row. This mirrors the IDE "attach to process" pattern in §1 —
  more metadata, explicit choice, no guessing on the user's behalf for something this hard to
  undo.
- **Batch import, partial success**: default to **independent per-item outcomes**, not
  all-or-nothing — matches the email-wizard analog and avoids re-litigating already-succeeded
  imports. Show a per-row terminal state (imported / failed / skipped) plus a summary line
  ("4 of 5 imported — 1 failed, see below") and let the user retry just the failed row(s) without
  re-selecting or re-confirming the ones that already succeeded. This needs an explicit decision
  in Phase 3 planning (already flagged as an open question in requirements.md) but the UX default
  should be per-item independence unless a technical constraint (e.g. shared worktree creation)
  forces batching.
- **Batch import + kill interaction**: given partial success, kill-confirmation must only ever
  offer to end the external sessions whose import actually succeeded — never present a single
  "end all originals" control that could kill a session whose import failed (silent data loss).
  One confirmation per successfully-imported session, or a batch confirmation listing exactly
  those sessions by name/path (not a bare count) so the user can verify the list before
  confirming — bare counts ("End 4 sessions?") fail the "always name the target" principle from
  §1/§3.
- **Discovery list changing mid-selection**: if a row a user has selected disappears (process
  exited) or changes state while the picker is open, do not silently remove it from a submitted
  batch — show it as "no longer available" and let the user acknowledge before the batch proceeds
  without it.

## 5. Jobs-to-be-done

- **Functional job**: get a specific, already-in-progress AI coding conversation under
  stapler-squad's management (worktree tracking, backlog integration, review flows) without
  losing any history and without having to manually reconstruct the resume invocation.
- **Emotional job**: confidence, specifically *loss-aversion* confidence — the user needs to feel
  certain that (a) nothing from the conversation is missing after import and (b) they cannot
  accidentally kill the wrong pane/process. This is the dominant emotional driver and should
  shape every design decision that trades a small amount of extra friction (an explicit preview
  step, a named confirm dialog, disambiguation before enabling Import) for a large reduction in
  "did I just lose that" anxiety. Secondary emotional job: relief from the current manual
  workflow's tedium (hunting for a UUID in `~/.claude/projects/*.jsonl`) — the value here is
  "I don't have to think about file paths," which argues for surfacing human-readable labels
  (project name, last prompt) everywhere instead of raw paths/UUIDs/PIDs in the primary UI (raw
  identifiers can still appear as a secondary/tooltip detail for power users debugging
  correlation).
- **Social job**: minimal. This is a single-developer, local-machine action with no
  sharing/handoff dimension in the stated scope — no team visibility, no audit trail consumed by
  others (only the structured logs mentioned in requirements.md, which are for the same user's
  own debugging/oncall, not a social signal). Not a design priority for this feature.

## Recommendations rollup (for Phase 3 planning)

1. Build the discovery list as a table (reusing `ApprovalAnalyticsPanel.tsx`'s checkbox-column
   pattern) with columns: select, program (Claude/Antigravity icon+label), path, last-active,
   status (ready / needs disambiguation / importing / imported / failed), actions.
2. Import is single-step-preview → confirm; kill is a categorically separate, later, named
   confirmation — never combine into one button.
3. Disambiguation is a blocking sub-state per row, not a silent auto-resolution.
4. Batch = independent per-row outcomes with a summary line, not all-or-nothing.
5. Reuse existing tokens/components: `ResumeSessionModal.tsx` dialog shape, `--error`/`--error-bg`
   tokens, `ApprovalAnalyticsPanel.tsx` checkbox pattern, `zIndex` contract for the modal layer —
   avoids new CSS architecture violations under `.claude/rules/css-architecture.md`.
6. Every row/control needs a `data-testid` and accessible name up front, since e2e conventions
   forbid CSS-class locators — bake this into the component API from the first draft, not as a
   retrofit.
