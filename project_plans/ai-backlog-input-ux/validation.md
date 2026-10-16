# Validation: AI Backlog Input UX

**Date:** 2026-10-11

## Scope Check

All four defects are addressed without touching the backend:

| Defect | Fix | Files Touched |
|---|---|---|
| Text discard | `onCreateBacklog` sets `backlogComposeText(input.trim())` | `Omnibar.tsx` |
| No attachment in compose | `BacklogComposeWidget` has inline attach button | new `BacklogComposeWidget.tsx` |
| Blocking submission | Close omnibar immediately; fire `createBacklogItemFromChat` in background; notify | `BacklogComposeWidget.tsx` |
| Hidden entry point | Compose mode opens via existing result-list button (already visible) | `Omnibar.tsx` |

## Risk Matrix

| Risk | Likelihood | Severity | Mitigation |
|---|---|---|---|
| Wrong `notificationType` value causes type error | Low | Low | Read `NotificationData` type before implementing; use existing constant |
| `createBacklogItemFromChat` RPC returns null on network error; error notification confuses user | Medium | Low | Error notification gives actionable text; retry is a follow-up |
| `jscpd` threshold exceeded by attachment upload snippet | Low | Low | Snippet is ~15 lines; jscpd minLines=20, so it won't flag; if it does, extract to `useBacklogAttachmentUpload` hook |
| Compose mode and review mode both active | Very low | Medium | Add mutual-exclusion guard in state setters |
| Mobile: file picker doesn't open on iOS | Low | Medium | `<input type="file" accept="image/*">` is the correct mobile pattern; no workaround needed |

## Omnibar Registry

Per `docs/reference/feature-testing-registry.md`, new Omnibar capabilities need both:
- An `OmnibarAction` union entry (if it's a user-triggerable action via the dispatcher)
- A `DetectorRegistry` entry (if it auto-detects input patterns)

**Verdict**: The compose widget is not a new _detected_ input pattern — it's triggered by the existing button click path (`onCreateBacklog`). It does not require a new `DetectorRegistry` entry. It is not an `OmnibarAction` in the dispatcher sense either (it's a UI mode toggle, not a session creation action). No registry changes needed.

The `// +feature: backlog:compose-widget` annotation in `BacklogComposeWidget.tsx` covers the feature registry (`make registry-generate`).

## Session Creation Registry

The compose widget does not create a _session_ — it creates a _backlog item_. The 7-touchpoint session-creation registry (`docs/reference/session-creation-registry.md`) does not apply.

## Minimal Viable Scope

If background processing (AC-3) is too complex to ship in one PR, it can be split:
- **PR 1**: AC-1 (text preservation) + AC-2 (compose widget with attachment, synchronous review) + AC-4 (button) — replaces `setInput("backlog: ")` with compose mode; still shows review form before creating.
- **PR 2**: AC-3 (background processing via `createBacklogItemFromChat` + notification).

The ACs are ordered by user pain: AC-1 is a simple one-line fix; AC-3 is the most architectural change.

## Estimated Complexity

Frontend only. No proto changes. No new RPC. One new component (~120 lines), one new CSS file, ~30 lines changed in `Omnibar.tsx`, one new test file (~80 lines). This is small-to-medium frontend work.

Cost class: single wave, one agent, frontend only. Parallelism opportunity: `BacklogComposeWidget` can be written independently of the `Omnibar.tsx` wiring (two sub-tasks that can be reviewed in parallel).
