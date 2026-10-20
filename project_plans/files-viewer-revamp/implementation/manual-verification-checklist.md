# Manual verification checklist: files-viewer-revamp

Owner-run checks that CI cannot stand in for. The Phase 0 sitting below runs once, on the owner's phone, against an
isolated instance. Record every result in the PR description as well as here.

## Spike notes (Phase 0 owner sitting)

Order of the sitting: Task 0.1.5b, Task 0.1.5d, Task 0.1.6b, Task 0.1.4d. All four use the same throwaway
instance. Nothing here touches the live service on `:8543`.

### Setup (once)

1. From the worktree root, build the web UI and a binary into the manual-build directory (never `./stapler-squad`, never
   `make install-service`):
   ```bash
   make web-build
   mkdir -p ~/.stapler-squad/manual-builds/manual-1
   go build -o ~/.stapler-squad/manual-builds/manual-1/stapler-squad .
   ```
2. Check the ports are free (must print nothing):
   ```bash
   ss -ltnp | grep -E '62871|62872'
   ```
3. Start the isolated instance:
   ```bash
   PORT=62871 STAPLER_SQUAD_INSTANCE=claude-manual-test STAPLER_SQUAD_TMUX_SOCKET=ssq-manual-1 \
     ~/.stapler-squad/manual-builds/manual-1/stapler-squad --tmux-keep-server --remote-access --remote-port 62872 &
   ```
4. On the phone, open the instance over LAN or Tailscale. Use the same host name and certificate path you use for the
   `:8444` app, with port `62872` (TLS, private CA). On the desktop the same pages are on `http://localhost:62871`.
   Spike pages (all throwaway, no data leaves the page):
   - Input and viewport page: `/spike-files/` (same as `/spike-files/?v=input`)
   - Accessible view and CodeMirror 6 read view: `/spike-files/?v=a11y`
   - Lazy-document models: `/spike-lazydoc/?m=a2` (gap-line) and `/spike-lazydoc/?m=a1` (window swap)
5. When the sitting is done: `kill %1; tmux -L ssq-manual-1 kill-server`, then delete `web-app/src/app/spike-files/` and
   `web-app/src/app/spike-lazydoc/` (the spike pages are removed after the sitting, not before).

### Task 0.1.5b: phone check of the input and viewport page

> **PROVISIONAL, EMULATED (not an owner phone verdict).** Only the `--` and `"x"` ASCII check (row 5 of the table) was
> exercised, with synthetic key events in emulated Chromium. It passed (both stayed plain ASCII). Soft-keyboard height,
> GREEN/RED bar behaviour, caret visibility, IME (`keyCode 229`), Bluetooth-keyboard signals and the fixture exports are NOT
> verified; the table rows below stay blank until the owner fills them on the phone. The `inputProfile` `it.skip` fixtures
> are untouched. The owner can still overturn or complete this.

Open `/spike-files/`. The page shows a live readout line (`innerHeight`, `vv.height`, `vv.offsetTop`, `keyboard`,
`maxKeyboard`), a CodeMirror 6 editor with a 5,000-line document, a RED bar fixed at `bottom: 0` and a GREEN bar pinned
to the visual viewport, and the key-event capture panel.

1. Tap inside the editor so the soft keyboard opens. Record: does the GREEN bar stay fully above the keyboard? Does the
   RED bar hide behind it? Does the caret stay in view, or does focus-scroll jump it out of view?
2. Write down the readout's `maxKeyboard` value (the real soft-keyboard height in px) and the `size=` value.
   Repeat at a 320x568 viewport if you have a small device or use the browser's responsive mode on the desktop (the plan
   assumes 260 px).
3. Type `--` and then `"x"` in the editor. Record whether the text became an em dash or curly quotes. This is the line
   for Story 2.5.2: type `--` and `"x"`, save, then diff the bytes; there must be no em dash and no curly quotes.
4. Start an IME composition (for example Japanese or Chinese keyboard, or predictive text on a composing keyboard) and
   type a few characters. Record whether text appears correctly and whether the capture panel shows `keyCode 229`.
5. Capture soft-keyboard events: tap Clear in the capture panel, then type this sentence in the editor with the soft
   keyboard: `Hello, world 123.` then Enter, Backspace, Space, Tab (if the keyboard offers it), one emoji, and one IME
   composition. Tap "Copy as JSON" (if the clipboard is blocked, long-press the JSON box and copy). Paste the JSON into the
   PR description under "soft keyboard fixtures". Each entry has `key`, `code`, `keyCode`, `isComposing`, `isTrusted`, and
   the modifier flags.
6. Attach the Bluetooth keyboard. With the editor focused, record whether `maxKeyboard` / `keyboard` in the readout
   shrinks (signal S4) or stays near 0.
7. Clear the panel and type with the Bluetooth keyboard: letters, Esc, Ctrl+key (for example Ctrl+S), the arrow keys, and
   F-keys. Export the JSON the same way and paste it under "Bluetooth keyboard fixtures".
8. Record the device, OS and browser version.

Result table (fill in):

| Check | Result |
|---|---|
| GREEN bar above keyboard | |
| RED bar covered | |
| Caret stays visible on focus | |
| Soft keyboard height (px), device, viewport | |
| `--` and `"x"` stay plain ASCII | |
| IME input works, `keyCode 229` seen | |
| Readout keyboard value with Bluetooth keyboard attached | |
| Device / OS / browser | |

The two JSON exports become the `inputProfile.test.ts` fixtures. The test file currently carries synthetic placeholders
and an `it.skip` named `inputProfile_should_ReplayOwnerCapturedFixtures_When_ExportedFromPhone`; replace the placeholders
with the exports and un-skip it.

### Task 0.1.5d: time-to-answer baseline

Same sitting, no spike page needed. With the workaround you actually use today (SSH or a terminal on the phone, or any
other route), find three different strings in a real session worktree and reach each hit's line, once each. Time each
trial with a stopwatch and count interactions with the requirements.md counting protocol.

| Trial | String searched | Route used | Seconds | Interactions | Completed? |
|---|---|---|---|---|---|
| 1 | | | | | |
| 2 | | | | | |
| 3 | | | | | |

Record the device and network. The median of the three completed trials is the baseline for the find-and-open metric and
the 4-week review. A trial you cannot complete on the phone is recorded as "not possible".

### Task 0.1.6b: VoiceOver / TalkBack check (criteria R1 to R4)

> **PROVISIONAL, EMULATED (not an owner phone verdict).** An emulated Pixel 7 Chromium run checked the R1/R2 analogue in DOM
> and ARIA terms only: lines are sequential and numbered; Go to line 1234 shows "Lines 1001-1500 of 3000" and "Line 1234";
> the skip link, headings and the Pager landmark are present. Real VoiceOver/TalkBack was NOT used, so the R1-R4 table below
> stays blank (R3 and R4 not run). The owner can still overturn it.

Open `/spike-files/?v=a11y`. The page has a region "Editor" holding the skip link "Switch to accessible view", a
CodeMirror 6 read-only view, and the Accessible view (pages of 500 lines, a pager, a Go to line field). Turn on VoiceOver
(iOS) or TalkBack (Android). Record device, OS and screen-reader version.

- R1 (Accessible view, required): swipe next from the first line of a page; it must read 50 consecutive lines in order
  with no skipped or repeated line, and each announcement begins with the line number. The Next page button must be
  reachable by swipe. Enter 1234 in Go to line and activate Go: it must land on the page containing line 1234 ("Lines
  1001-1500 of 3000") and announce "Line 1234" first.
- R2 (Accessible view, required): heading, landmark or rotor navigation reaches the pager and the code region. The skip
  link "Switch to accessible view" must be the first element reached inside the editor region when swiping from the file
  name.
- R3 (CodeMirror 6 read view, informational): swipe through the CodeMirror 6 view. Record whether you reach line 1, a line
  in the viewport, and then stop at the last mounted line, and whether off-screen lines are ever reachable (expected: no).
- R4 (CodeMirror 6 edit, informational): on `/spike-files/` (the editable view) type letters (each character echoes),
  move the caret (character or line announced), do an IME composition (no duplicated text), and press Backspace (removed
  character announced).

| Row | Pass / fail | Notes |
|---|---|---|
| R1 | | |
| R2 | | |
| R3 | | |
| R4 | | |

Decision rule: R1 and R2 must pass, or the Accessible view is redesigned (for example one `<ol>` of 100-line pages) before
Story 1.5.4 starts. R3 and R4 only set the wording of the known-limits section of the how-to and the Task 2.5.4b script.

### Task 0.1.4d: engine verdict for the lazy-document models

> **PROVISIONAL, EMULATED (not an owner phone verdict).** Emulated Pixel 7 Chromium, real touch flings, 6x CPU throttle,
> 250 ms latency. a2 gap-line: PASS (0 blank bands in 150 samples; typing and scrolling OK; a far jump lands within a few
> thousand lines of the target, as expected). a1 window swap: PASS, and it refuses a swap with dirty edits as designed.
> Provisional selection: **a2 gap-line module for Story 1.6.4**; Epic 1.7 (custom engine) is not built. Real-finger feel on
> iOS Safari / Android Chrome (S-T3 residual) is NOT verified. The owner can still overturn it by recording a different
> verdict in the table below.

Open `/spike-lazydoc/?m=a2`, then `/spike-lazydoc/?m=a1`. Each page shows a virtual 1,000,000-line file (about 50 MB) and a
stats line. For each model:

1. Scroll fast with a flick, from the top to well past the middle. Record whether you see jumps, blank areas that stay
   blank, or the viewport sliding while chunks load.
2. Enter 500000 (and then 999000) in Go to line and tap Go. Record whether it lands near the line and whether it settles
   without flicker. (a2 may land a few thousand lines off the requested line and then fill in; a1 swaps windows.)
3. Tap in the text and type a few characters. Record whether typing works, whether the keyboard opening moves the view, and
   whether scrolling afterwards still works.
4. a1 only: after typing, scroll to the window edge and record what happens (a1 refuses to swap a window with unsaved
   edits).
5. Record PASS or FAIL for each model.

| Model | Fast scroll | Far jump | Typing | Verdict |
|---|---|---|---|---|
| a2 gap-line | | | | PASS / FAIL |
| a1 window swap | | | | PASS / FAIL |

Verdict rule from the plan: a2 PASS selects the gap-line module (Story 1.6.4); a2 FAIL with a1 PASS selects window swap
(Task 1.6.4d) and the single-editable-window UX; both FAIL activates the contingent custom engine (contingent Epic 1.7)
and until it ships T1 and T2 files are view-only through the Accessible-view pager. Record the verdict here and in
ADR-036 "Spike outcomes".

## Phone-only checks still unverified

Everything below needs the owner's real phone, real assistive technology or a real keyboard. Emulated runs (see the
PROVISIONAL notes above) do not close any of these. Plan line numbers refer to `plan.md`.

| Check | Task / source | Status |
|---|---|---|
| Real soft-keyboard height; GREEN bar above and RED bar covered; caret visible on focus | Task 0.1.5b (plan 410) | Unverified |
| IME composition (`keyCode 229`), no duplicated text; soft-keyboard and Bluetooth `inputProfile` fixtures exported (un-skip `inputProfile_should_ReplayOwnerCapturedFixtures_When_ExportedFromPhone`) | Tasks 0.1.5b, 0.1.5c (plan 407, 410) | Unverified |
| Bluetooth keyboard signals (S4 readout shrink), Esc / Ctrl+key / F-keys; Enter/Backspace on the soft keyboard never hides the key bar, Esc on Bluetooth does | Task 0.1.5b; Story 2.5.2 (plan 2108) | Unverified |
| Android Back with the keyboard open; `inputmode=none` in Read mode | Stories 0.1.5, 1.5.1 / 1.5.2 (plan 163, 1247) | Unverified |
| Real iOS PWA keyboard bar, `visualViewport`, vim best-effort | Open question (plan 163); Story 2.5.2 (plan 2096-2102) | Unverified |
| Time-to-answer baseline: three timed find-a-string-and-reach-the-line trials | Task 0.1.5d (plan 414) | Not done |
| VoiceOver / TalkBack R1-R4 (R1/R2 required; R3/R4 informational) on the spike page | Task 0.1.6b (plan 436) | DOM/ARIA analogue only (emulated) |
| S-T3 real-finger fling feel and far-jump / typing feel on the phone (a2, a1) | Task 0.1.4d (plan 384; open question plan 165) | Emulated PASS only; owner verdict open |
| S-T1 leg: response size and latency from the phone over `:62872` / Tailscale | Task 0.1.7a (ADR-036 "Spike outcomes") | PENDING-OWNER |
| Real-path streaming check on the phone | Task 0.1.1b (plan 311) | Unverified |
| Gate G1 phone smoke of the read-only flows (open, search, jump, rotate) with the Accessible view and a VoiceOver/TalkBack quick check of R1/R2 against the built component | Task 1.5.3c (plan 1272); Story 1.5.4 | Not due yet; blocks Epic 2.2/2.3/2.4 |
| Informational: scroll and jump in a 50 MB file when `files:range-read` is on in the isolated instance | Task 1.5.3c (plan 1273) | Not due yet |
| Windowed large-file reading on the phone (fast scroll, far jump, search hit, Accessible view, background/foreground the PWA); gates the live `files:range-read` flip | Task 1.6.7c (plan 1502); Story 1.6.5 on-device large file | UNVERIFIED on a phone. CI side done (Tasks 1.6.7a/b: emulated mobile-chromium e2e and host benchmarks, ADR-036 "Phase B benchmarks"); real fast-scroll feel, far jump, search hit, Accessible view and PWA background/foreground, plus phone-side wire size and latency, remain for Task 1.6.7c |
| Phone dogfood script and sign-off (incl. `files:merge-phone` decision); gates the global `files:editing` flip | Task 2.5.4a (plan 2154) | Not due yet |
| VoiceOver / TalkBack script for CodeMirror on touch (R1-R4 re-run) | Task 2.5.4b (plan 2158) | Not due yet |
| Per-browser chord table (Chrome, Firefox, Safari), non-US layout AltGr+8, Edit-layout at three phone viewports | Story 2.5.2 (plan 2108) | Not due yet |
| Large-file editing script on the phone link (5 MB and 50 MB edit/save, conflict, rebase, kill switch, offline draft); gates `files:delta-write` / `files:edit-large` | Story 2.5.5 (plan 2171) | Not due yet |
| Custom-engine phone gate | Task 1.7.6d (plan 2386) | Only if Epic 1.7 is activated (not expected) |
