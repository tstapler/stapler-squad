# UX Research: Session Completion Summary

Agent 5 (UX Research). Grounded in requirements.md (backlog `59bbff11-ee8b-418c-8484-64307cb14244`) and the existing `SessionDetailView.tsx` / `SessionDetail.tsx` tab implementation.

## Codebase grounding (read first — drives the rest of this doc)

- Tab strip: `web-app/src/components/sessions/SessionDetailView.tsx:283-291` — `tabs` array is `{ id, label, icon, disabled? }[]`. Existing disabled example is Browser (`disabled: !isBrowserAvailable`), rendered with `aria-disabled` + a `title` tooltip explaining *why* (`SessionDetailView.tsx:582,585`). `SessionDetailTab` union type lives in `SessionDetail.tsx:28` and must gain `"summary"`.
- Tab strip a11y is already solid: `role="tablist"` container (`:558`) with roving `ArrowLeft`/`ArrowRight` focus management (`:559-572`), each tab `role="tab"` + `aria-selected` (`:580-581`). A new Summary tab gets this for free — no new keyboard-nav work needed.
- Existing `aria-live="polite"` precedent at `:495` (queue position) and `:697` — confirms the codebase already uses live regions for async status text, which the Summary tab's PENDING→READY/ERROR transition should follow.
- Markdown rendering is already a dependency: `react-markdown@^10.1.0` + `remark-gfm@^4.0.1` (`web-app/package.json:87,91`), currently used in `DescriptionSection.tsx:3-4,24` (`<ReactMarkdown remarkPlugins={[remarkGfm]}>`). The Summary tab should reuse this exact pattern rather than introducing a second markdown renderer.
- Existing copy-to-clipboard pattern (`SessionDetailView.tsx:320-329`, used at `:829,1042,1066,1108,1223`): `navigator.clipboard.writeText` → `setCopiedField(field)` → glyph swap (📋 → ✓) for 1.5s via `setTimeout`, with only a `console.warn` on failure (no user-facing error, no `aria-live` announcement, feedback is `title` attribute only). **This is a gap worth fixing, not copying verbatim** — see Accessibility section below.

## 1. Comparable UX patterns for async-generated reports

| Tool | Pattern |
|---|---|
| GitHub Actions run summary | Page renders immediately on run start with a live log stream; the "Summary" section (job-level markdown from `$GITHUB_STEP_SUMMARY`) only appears once the job that writes it completes — until then that section of the page simply isn't present, not shown as a skeleton. |
| GitHub Copilot PR description generation | Button-triggered, shows an inline spinner in place of the (yet-empty) description textarea; textarea populates in place once done, no separate loading page. |
| CI report pages (CircleCI, Buildkite artifacts/test-summary tabs) | Tab is visible immediately alongside Tests/Artifacts tabs; tab content area shows a spinner + "Generating report…" until the post-build step finishes, then swaps to the rendered report in place — no tab-appears-later pattern once the run itself is visible. |
| Async LLM summarization UIs generally (e.g. incident postmortem generators, meeting-notes bots) | Skeleton-block placeholders (2-4 gray bars mimicking paragraph shape) rather than a bare spinner, because the wait is several seconds to tens of seconds — a spinner alone at that duration reads as "stuck," skeleton blocks read as "actively composing." |

Takeaway for FR-5/FR-7: the field is split roughly two ways — (a) tab/section is invisible until content exists (GH Actions job summary), or (b) tab is visible from the moment it becomes *reachable* and shows an in-progress state (CI report tabs, Copilot). Given FR-3 (durable, must be reachable even after the Session row is gone) and FR-6 (must always end in a valid READY doc), pattern (b) is the better fit here — see §2.

## 2. User mental model — when should the Summary tab appear, and in what state?

**Non-terminal (running) session:** Summary tab should be **visible but disabled**, following the exact Browser-tab precedent (`disabled: !isBrowserAvailable` + `title` tooltip). Rationale: a user scanning the tab strip while a session is running should be able to see "this exists, it's just not ready yet" rather than have tabs materialize unpredictably — consistent with how Diff/VCS/Files are always present even when currently empty. Tooltip text: `"Summary is generated after the session ends"`.

**Session just transitioned to terminal (EventExited/EventStopped fired, generation not yet started or in PENDING/GENERATING):** Tab becomes enabled immediately (not gated on generation completing) and defaults to showing a **generating state** if the user clicks into it — spinner + skeleton blocks (per §1) + text `"Generating summary…"`. Do NOT make the user wait to even open the tab; FR-5 explicitly requires non-blocking teardown, and hiding the tab until READY would contradict that by coupling tab visibility to async completion.

**Why not delay tab appearance until READY:** the user's mental model at the moment a session ends is "the session just stopped, where's the record of what happened" — exactly when they're most likely to click. A tab that isn't there yet reads as a missing feature, not as "still working on it." A visible tab in a GENERATING state answers the implicit question ("is this coming?") for free.

**Once READY:** tab shows the document. **Once ERROR:** tab shows the error state (§4) — never silently fall back to hiding the tab or reverting to a blank pane.

**Confirmed pattern to use:** add `disabled: !isSessionTerminal` (or equivalent) to the `summary` tab entry in the `tabs` array at `SessionDetailView.tsx:283-291`, mirroring the Browser tab's `disabled` + `title` convention exactly.

## 3. Accessibility

- **Tab strip / keyboard nav:** No new work — `role="tablist"`/`role="tab"`/`aria-selected`/roving arrow-key focus at `SessionDetailView.tsx:558-591` already covers a new "summary" entry in the `tabs` array with zero additional logic.
- **Disabled-tab tooltip:** Reuse the `title` pattern at `:585`, extended with a `summary`-specific case (`tab.disabled && tab.id === "summary" ? "Summary is generated after the session ends" : ...`). Note `title` alone is a weak a11y signal (not reliably exposed by all AT, not keyboard-discoverable without focus+wait) — the existing Browser tab already has this limitation, so this is a pre-existing gap, not one this feature should be blamed for introducing, but also not one to expand elsewhere.
- **State transition announcements (PENDING/GENERATING → READY/ERROR):** wrap the tab panel's status text in `aria-live="polite"` (same idiom as `:495` and `:697` already in this file) so a screen reader user who opened the tab while it was generating hears the transition without having to re-poll. Use `aria-busy="true"` on the panel container while GENERATING, matching the ARIA authoring practice for regions with pending async content.
- **Copy-to-clipboard button — do not copy the existing pattern verbatim:**
  - Accessible name: the existing buttons rely on `title="Copy to clipboard"` with a bare 📋/✓ emoji as content — no `aria-label`, so a screen reader announces only "button" plus whatever the emoji glyph happens to expose (inconsistent across AT). The Summary tab's copy button should have an explicit `aria-label="Copy summary as Markdown"` and, on success, `aria-label="Copied"` (or a separate `aria-live="polite"` status node) rather than relying on the icon swap alone — the icon swap is a *visual-only* signal today, and the existing code's `console.warn`-on-failure is silent to the user entirely.
  - Failure should be user-visible, not just logged: on `navigator.clipboard.writeText` rejection, surface the failure in the same `aria-live` region used for success ("Copy failed — try selecting the text manually") rather than the current silent `console.warn`.
  - This is a genuine upgrade over the pattern at `SessionDetailView.tsx:320-329`, worth calling out explicitly in the plan phase since a reviewer may otherwise "match existing style" and reintroduce the gap.
- **Markdown content itself:** `react-markdown` output must still respect heading hierarchy (don't jump from panel `h2` straight to content `h1`/`h4` — pick a level and increment consistently) and any code blocks (diff stat, token counts) should not rely on color alone to convey approved/denied/pending status — pair color with text/icon (this matters for the approval-decision breakdown, FR-2).

## 4. Error states and edge cases

**ERROR state (FR-5 — deterministic-stage failure):**
- Show *what* failed, not just "an error occurred." At minimum: which stage failed (e.g. "Failed to compute diff stat" vs. "Failed to generate narrative") if the generation pipeline has stages, plus a timestamp of the failed attempt.
- Primary action: **Regenerate** button, prominent, not buried — this is the only recovery path per FR-5.
- Secondary information: if a *prior* successful generation exists (e.g. Regenerate itself failed but an earlier READY doc is still stored), show the stale document with a banner ("Showing summary from the last successful generation, dated X — regeneration failed, see error above") rather than replacing a working doc with a bare error. This avoids the failure mode where a user loses a previously-good summary because a manual regenerate attempt failed.
- Avoid raw stack traces / internal error strings in the primary error text — collapse them behind a "Details" disclosure for anyone who wants to file a bug, but lead with a plain-language sentence a non-engineer teammate (social-job scenario, §5) wouldn't be confused by if they saw it over a shoulder-share.

**FR-7 idempotency UI:** while GENERATING (including from a Regenerate click), the Regenerate button must be disabled and show a spinner/label change (e.g. "Regenerating…"), matching the disabled-button-during-async-action convention used elsewhere for form submits in this codebase. Do not merely debounce the click — the button must be inert for the full duration, since FR-7 explicitly calls out repeated clicks must not spawn overlapping generations.

**FR-6 empty-state copy (minimal-activity session, e.g. started and stopped immediately):** every section renders with explicit text, never a blank/omitted block. Proposed wording, matching the terse-but-informative tone of the rest of the app's empty states:

| Section | Empty-state text |
|---|---|
| Narrative | "This session ended before any work was recorded." |
| Changes / diff | "No files were changed." |
| Approval decisions | "No approval requests occurred during this session." |
| Timeline | Always populated (start/stop/duration are always known) — never empty, but if start==stop essentially, still show "Duration: <1s" rather than "0s" or blank. |
| Token usage / cost | "No tokens were used." (if literally zero) — distinguish from "Cost data unavailable" if the failure is data-collection rather than genuinely-zero-usage; these are different states and should not share copy. |

Each empty-state string should render in the same visual slot the populated content would use (not a smaller/grayed-out variant) so the document reads as complete and intentional, not broken — directly serves FR-6's "not blank/omitted sections" requirement and the trust/accountability emotional job (§5).

## 5. Jobs-to-be-done

- **Functional job** — "tell me what the agent did without me reading scrollback." Served by: diff stat + narrative + approval breakdown all being visible without expanding/scrolling past the fold if possible; this argues for a compact structured layout (stat line, then collapsible full diff link — not the full diff inlined) rather than a wall of markdown.
- **Emotional job** — confidence/accountability ("did it do something I didn't approve? did anything silently fail?"). Served directly by the approval-decision breakdown (FR-2) being unmissable, not buried under the narrative — put it near the top, not at the bottom, since it's the section most likely to be scanned specifically for red flags (denied/still-open items). A session with any `still-open` review-queue items or denials should visually distinguish that from a clean run (e.g. a small status indicator at the tab level itself, similar to how the Browser/shell tabs already carry state), though that's a stretch goal beyond the FRs as written — flag for the planning phase rather than assume it's in scope.
- **Social job** — pasting into a PR body / Slack. This is the strongest argument for FR-4's "GFM markdown, reusable as-is" requirement: the raw markdown export is the primary artifact for this job, not the in-app rendering. But the in-app view still needs to look presentable on its own because a user may screenshot the tab directly (common in Slack) rather than copy-export first — so the in-app rendering should not rely on interactive-only affordances (tooltips, hover states, collapsed-by-default sections that need a click to reveal content) for anything that matters to a skimming viewer. Practical implication: keep the approval-decision breakdown and diff stat expanded by default in the rendered view, not behind an accordion — an accordion is fine for the *full diff*, not for the summary counts.

## Open questions to carry into planning (not resolved here)

- Whether the Summary tab should carry a lightweight status badge (dot/icon) visible on the tab label itself when ERROR or when a still-open/denied item exists, independent of clicking into the tab — flagged in §5 as a possible enhancement beyond the literal FR text.
- Exact wording/threshold for distinguishing "zero tokens used" vs. "cost data unavailable" in the empty-state table above depends on backend data-availability guarantees Agent 5 doesn't have visibility into — needs backend research agent input.
