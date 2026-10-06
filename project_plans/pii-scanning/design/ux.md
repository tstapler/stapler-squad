# UX Design: PII Scanning Review-Queue Surface

Scope: concrete wireframes, interaction flows, and testable UX acceptance criteria for the `pii-scan` escalation category, built directly on `requirements.md` and `research/ux.md`'s conclusions and `implementation/plan.md`'s finalized wiring (Phases 3–5).

## Verdict up front

**No new UI component is needed.** This document concurs with `research/ux.md`: the feature is fully served by three map/union entries in existing generic components — `ESCALATION_REASON_EMOJI["pii-scan"]` and the `EscalationCategory` union entry (`web-app/src/components/sessions/ReviewQueuePanel.tsx`, `web-app/src/lib/sessions/escalationCategory.ts`), and `ESCALATION_CATEGORY_LABELS["pii-scan"]` (`web-app/src/components/sessions/ApprovalAnalyticsPanel.tsx`). `implementation/plan.md`'s Phase 5 (Tasks 5.1.1a, 5.2.1a) implements exactly this and nothing more — no dedicated stat tile, no config-toggle screen (config is JSON-only per Phase 2, no UI surface scoped), no masking/reveal interaction. This document does not invent scope beyond what those two phases build.

Three real user-facing surfaces exist. A fourth candidate (a deny-mode UI message) was investigated and ruled out as **not a web UI surface at all** — see Surface 4 below.

---

## Surface 1: Review Queue card — escalation reason line

**Where**: `ReviewQueuePanel.tsx:744-759`, inside each queue item's expanded body (`itemBody`), gated on `queueItem.metadata?.["pending_approval_id"]` being present (i.e., this item came from the approval-hook escalation path, not a static rule flag).

**What changes**: one line in the existing `ESCALATION_REASON_EMOJI` map (`:141-147`) and its doc comment (`:138-140`) — no JSX changes, no new element.

### Wireframe — queue item, PII escalation (expanded)

```
┌─────────────────────────────────────────────────────────────────┐
│ session-name-here                              [ P0 · Critical ] │ ← itemHeader / ReviewQueueBadge(compact)
├─────────────────────────────────────────────────────────────────┤
│ [ P0 · Critical ]                                                │ ← ReviewQueueBadge(full)
│                                                                   │
│ 🔒 Detected Social Security Number in command — escalated for    │ ← escalationReasonText, id/data-testid=
│    manual review.                                                │   escalation-reason-<sessionId>
│                                                                   │
│ ┌───────────────────────────────────────────────────────────┐   │
│ │ curl -d ssn=219-09-9999 http://internal                     │   │ ← <pre className={commandPreview}>
│ └───────────────────────────────────────────────────────────┘   │   raw, unredacted (see §4 rationale)
│                                                                   │
│ Directory: /repo/src                                             │ ← detailRow / detailLabel / detailValue
│                                                                   │
│           [ Approve ]   [ Deny ]   [ View diff ]                 │ ← existing action row (unchanged)
└─────────────────────────────────────────────────────────────────┘
```

### Wireframe — queue item, PII escalation from a Write (file content)

```
┌─────────────────────────────────────────────────────────────────┐
│ seed-fixtures-session                          [ P0 · Critical ] │
├─────────────────────────────────────────────────────────────────┤
│ [ P0 · Critical ]                                                │
│                                                                   │
│ 🔒 Detected Email address in content — escalated for manual      │
│    review.                                                       │
│                                                                   │
│ ┌───────────────────────────────────────────────────────────┐   │
│ │ INSERT INTO users VALUES ('john@company.com')                │   │ ← tool_input_file (Write content)
│ └───────────────────────────────────────────────────────────┘   │
│                                                                   │
│ Directory: /repo/src                                             │
│                                                                   │
│           [ Approve ]   [ Deny ]   [ View diff ]                 │
└─────────────────────────────────────────────────────────────────┘
```

Note the reason text names the **matched pattern** and the **field** (`command` vs `content`/`new_string`), per `FormatPIIEscalationReason(patternName, field)` (plan.md Task 1.2.3a) — this directly answers `research/ux.md` §2's requirement that PII's reason text read as "targeted and specific," distinct from the vaguer `❓ no rule matched`.

### Interaction flow

1. Agent issues a Bash command or Write/Edit tool call containing PII-shaped text, in a directory not covered by `skip_path_patterns`.
2. Approval hook (`ApprovalHandler.HandlePermissionRequest`) detects the pattern, builds a `PendingApproval` with `EscalationCategory: "pii-scan"`, and the item appears in the review queue exactly like any other escalation (no new polling/websocket path — reuses the existing `ReviewItem` broadcast).
3. Reviewer opens/expands the card (existing interaction — click or keyboard `Enter`/`Space` on the card per existing `itemHeader` handling) and sees the 🔒-prefixed reason line plus the raw command/content preview.
4. Reviewer reads the preview, judges fixture-vs-real, and clicks **Approve** or **Deny** (existing buttons, existing RPC — `pii-scan` items use the identical approve/deny action as every other `ReviewItem`; no PII-specific action was added).
5. On Approve: the underlying tool call proceeds; the item leaves the queue. On Deny: the tool call is rejected; the item leaves the queue. Both are the pre-existing generic queue-resolution flow — plan.md adds no new resolution branch.

### Error / edge-case handling for this surface

| Case | What the user sees | Source |
|---|---|---|
| `escalation_reason` metadata missing (item predates escalation-reason tracking) | Existing fallback text: `"Reason not recorded — this request predates escalation-reason tracking."` (no 🔒 prefix, no crash) | `ReviewQueuePanel.tsx:753` — pre-existing, unaffected by this feature; the `?? ""` guard in the emoji lookup (`:752`) means an unrecognized/missing category degrades to no emoji, never a broken render |
| Matched PII in a **very large file write** (Write/Edit content payload could exceed a comfortable preview) | The full `content`/`new_string` value is shown verbatim in `<pre className={commandPreview}>` with no truncation guard today — **this is a pre-existing gap the requirements doc flagged** (`research/ux.md` §4, final paragraph), not something Phase 5 fixes. `piiContentScanMaxBytes` (64 KB, plan.md) caps what is *scanned*, not what is *displayed* — the CSS `commandPreview` class presumably scrolls/wraps like any long `<pre>` block, so nothing breaks, but a reviewer must scroll to find the actual match with no highlighted/anchored excerpt. **Recommendation for a fast-follow, not blocking this plan**: byte-cap the *displayed* preview around the match position, matching the existing 64 KB scan cap, so the reviewer isn't scrolling a multi-MB file to eyeball one email address. |
| PII pattern matched but `queueItem.patternName` is unset (only `escalation_reason_category`/`escalation_reason` set, not the older `patternName` field used by static rule-flags) | The `Pattern: {patternName}` line (`:739-743`) simply does not render — no dead space, no error. This is existing conditional-rendering behavior; PII escalations populate the newer `escalation_reason`/`escalation_reason_category` metadata pair instead of the older `patternName` field, consistent with how `domain-age` escalations already work. |
| Reviewer denies a false-positive fixture match repeatedly | No dedicated "always allow this path" UI action exists today for any escalation category — the actual mitigation is the reviewer or an operator widening `pii_scanning.skip_path_patterns` in `config.json` (Risk Control section, plan.md). This is a config-file exit path, not a UI dead end: it is documented, reachable, and requires no code change — but it is **not self-service from the queue UI**. Flagging explicitly per the "no dead ends" criterion below: the exit path exists, but it is one hop outside the web UI. |
| Malformed custom regex in `pii_scanning.custom_patterns` (e.g. `"[invalid("`) | Never reaches this UI surface at all — `SetPIIScanningConfig` (plan.md Task 3.1.1a) skips the invalid entry at config-load time and logs a warning server-side; the queue simply never shows an escalation sourced from that broken pattern. No user-facing error state exists for this today (server logs only) — acceptable because it fails safe (skip, not crash) rather than fails loud, but note this as a gap if `pii_scanning` config ever gets a UI editor in a future iteration. |

---

## Surface 2: Analytics panel — Escalation Reasons table row

**Where**: `ApprovalAnalyticsPanel.tsx:329-360`, the existing generic table driven by `escalationReasonRows` (derived from `summary.escalationReasonCounts`).

**What changes**: one entry in `ESCALATION_CATEGORY_LABELS` (`:98-105`) — `"pii-scan": "PII detected in request"`. No new `<div>`, no new card, no new chart.

### Wireframe — Escalation Reasons table, with `pii-scan` present

```
┌─ Escalation Reasons ──────────────────────────────────────────────┐
│  Reason                                    Count   Frequency       │
├─────────────────────────────────────────────────────────────────  │
│  Rule explicitly flagged for review          12    ████████░░      │
│  PII detected in request                      4    ███░░░░░░░      │ ← new row, same shape as siblings
│  Newly-registered domain                       2    █░░░░░░░░░      │
│  No auto-approval rule matched                 1    ▏░░░░░░░░░      │
└─────────────────────────────────────────────────────────────────┘
```

Row ordering is `count`-descending (`escalationReasonRows` sort, `:147`) — `pii-scan` is not pinned to a fixed position; a quiet week with zero PII detections means the row simply does not render (filtered by `count > 0`, `:146`), falling into the table's existing empty-row filtering, not a dedicated empty state.

### Wireframe — zero PII detections in the selected window (whole-table empty state, pre-existing)

```
┌─────────────────────────────────────────────────────────────────┐
│  No escalations in this window.                                  │ ← `empty` class, :358
└─────────────────────────────────────────────────────────────────┘
```

This renders when **no** category (not just `pii-scan`) has a non-zero count for the selected 7/14/30/90-day window — pre-existing behavior, unaffected by this feature. There is no PII-specific empty state, by design — a single new row cannot have its own empty state independent of the table it lives in.

### Interaction flow

1. Team lead/compliance reviewer opens the Approval Analytics panel (existing navigation — unchanged).
2. Selects a time window (7/14/30/90 days) via the existing `WINDOW_OPTIONS` button group (`:157-168`) — no new control.
3. Scrolls to "Escalation Reasons"; if any `pii-scan` decisions occurred in the window, a row labeled **"PII detected in request"** appears with its count and a proportional inline bar (`Bar` component, reused).
4. That is the entire interaction — per `research/ux.md` §5's resolution, this is **count-only**; there is no click-through from this row to the underlying individual `ReviewItem`s or approver attribution (explicitly out of scope, plan.md line 79).

### Error / edge-case handling for this surface

| Case | What the user sees | Source |
|---|---|---|
| Analytics fetch fails entirely | Existing `error` banner: `"Failed to load analytics: {error.message}"` with a **Retry** button (`:175-179`) — applies to the whole panel, including the Escalation Reasons section; no PII-specific error path. | `ApprovalAnalyticsPanel.tsx:175-179`, pre-existing |
| `escalationReasonCounts` contains a category key the frontend doesn't recognize (e.g. a future backend category shipped before the frontend map is updated) | Falls back to the raw category string via `?? category` (`:346`) rather than rendering `"undefined"` — pre-existing guard, and per the Phase 4.2 sync-guard test (`escalationCategory.test.ts`), this specific drift is now caught by CI for `pii-scan` itself before it can ship un-mapped. | `ApprovalAnalyticsPanel.tsx:346`; `implementation/plan.md` Task 4.2.1b |
| Loading state while a window is being fetched | Existing `loading` state disables the refresh button and presumably shows the prior window's data or a loading indicator elsewhere in the panel (out of view in the excerpt reviewed) — unaffected by this feature; no PII-specific loading treatment needed since the row is just table data. | `ApprovalAnalyticsPanel.tsx:169` (`disabled={loading}`) |

---

## Surface 3: Config — confirmed no UI surface

`pii_scanning.enabled` / `custom_patterns` / `on_detection` / `skip_path_patterns` land in the existing JSON config file (`config/types.go`/`config/config.go`, Phase 2) with **no corresponding UI screen or toggle** — `implementation/plan.md` scopes this as backend-only (no task in Phase 5 or elsewhere touches a settings/config UI component). This matches `research/ux.md`'s open item ("Backend-only per requirements; no UI toggle specified/scoped yet") — planning resolved it by *not* building one. Operators change this by editing `config.json` and restarting the service (per Phase 3's `SetPIIScanningConfig` wiring), the same mechanism every other nested feature-config (`TmuxExecGateConfig`, `SessionRetentionConfig`) already uses without a UI. No wireframe applies here because there is no screen.

---

## Surface 4: Deny-mode message — confirmed not a web UI surface

Investigated because `on_detection: "deny"` (the opt-in stricter mode, plan.md §"Resolutions") produces a user-facing message (`FormatPIIDenyMessage`, Task 1.2.3a) analogous to the secret-scanner's existing `FormatSecretDenyMessage`. Traced the call site: `h.writeDecision(w, "deny", msg)` (`approval_handler.go:356`, mirroring the secret-scan branch at `:248`) writes the message into the **hook-specific JSON response returned to the calling agent's PreToolUse/PermissionRequest hook** — i.e., it surfaces in the agent's own tool-call rejection (visible in the agent's terminal/transcript output), not in any stapler-squad web page. This is the same surface secret-scan's existing (and unreviewed-by-any-UI) deny message already uses. **No wireframe needed**: there is no browser-rendered screen for this path, confirming `research/ux.md`'s scope conclusion extends to the deny mode as well, not just the default escalate mode.

---

## Accessibility

- **WCAG 1.4.1 (not color-only)**: confirmed/restated from `research/ux.md` §3 — the `🔒` emoji is a **prefix on the existing text paragraph** (`` `${emoji ?? ""} ${escalation_reason}`.trim() ``, `ReviewQueuePanel.tsx:752`), never a standalone colored badge or color-only signal. The reason text (`"Detected Social Security Number in command — escalated for manual review."`) fully carries the meaning independent of the emoji; a screen reader that reads `🔒` as "locked padlock," "lock," or skips it entirely (platform/AT-dependent) loses no information the text doesn't already state. This satisfies 1.4.1 by construction — no additional `aria-label` work is needed because the emoji is supplemental decoration, not the sole signal, exactly as `research/ux.md` concluded.
- **Keyboard navigation**: the queue card, its expand/collapse interaction, and the Approve/Deny buttons are all pre-existing keyboard-operable controls (native `<button>` elements, existing `itemHeader` click/keydown handling) — `pii-scan` items use these unmodified. No new interactive element is introduced by this feature, so no new keyboard path needs to be built or tested.
- **Screen-reader labels**: `id`/`data-testid={`escalation-reason-${queueItem.sessionId}`}` (`:748-749`) already exists and is category-agnostic — a screen reader landing on this paragraph reads the full "🔒 Detected..." sentence as one accessible text node. No per-category `aria-label` is needed since the category only changes the leading emoji + reason string, both already inside the same readable node.
- **Color contrast ≥ 4.5:1**: the reason text uses the existing `escalationReasonText` typography class (standard body text color against the card background) — unaffected by this feature, since no new colored element is introduced. The Escalation Reasons table row similarly reuses `td`/`row` styling. Both inherit whatever contrast ratio the existing classes already provide; this feature adds no new color token and therefore introduces no new contrast risk to verify.
- **Analytics table row**: the `Bar` component's fill color is a pre-existing `barRule` style shared by every category row (not PII-specific) — confirmed no new color/hue was introduced for `pii-scan` specifically (`ux.md` §1 recommended reusing `gapBadgeHigh`'s warning styling *if* a distinct badge were built; since no distinct badge was built, this is moot — the row uses the plain neutral `barRule` like every other category).

---

## UX Acceptance Criteria

Each is testable by a human exercising the running app (or, where noted, verifiable by reading the specific line the plan.md task touches).

### Surface 1 — Review Queue

1. **Recognition speed**: a reviewer scanning a mixed queue (containing `no-match`, `explicit-rule`, `domain-age`, and `pii-scan` items) can visually identify all `pii-scan` items in ≤ 2 seconds per item without reading the full reason sentence, by the 🔒 prefix alone — verify by confirming `🔒` is the *only* emoji mapped to `"pii-scan"` in `ESCALATION_REASON_EMOJI` (no collision with an existing category's emoji) and that it is visually distinct from `❓🛑🌐⚙️⚠️`.
2. **Task completion in ≤ 2 steps**: from the review queue landing state, a reviewer can (1) expand/open a `pii-scan` card and (2) click Approve or Deny — 2 total interactions, identical step count to every other escalation category, since no PII-specific extra confirmation step was added.
3. **Reason text is specific, not vague**: given a `pii-scan` item, the rendered reason text names both the matched pattern (`"Social Security Number"` / `"Email address"` / `"Credit card number"`) and the field (`"command"` / `"content"` / `"new_string"`) — verified directly against `FormatPIIEscalationReason`'s implementation (plan.md Task 1.2.3a) and the GWT example in Story 5.1.1.
4. **No dead ends**: every state reachable from a `pii-scan` queue item (expanded card, missing-reason fallback, missing-pattern-name fallback) has a visible next action — Approve, Deny, or (for the missing-reason fallback) the same Approve/Deny pair with a generic-but-present message, never a blank or stuck state. The one **documented, not-self-service** exit path (widening `skip_path_patterns` for a recurring false positive) requires a config-file edit outside the web UI — this is disclosed here as a known limitation, not silently treated as a dead end within the UI itself.
5. **No new component regressions**: `ReviewQueueBadge`, `itemContext`, `commandPreview`, `detailRow` and all other existing card elements render unchanged for `pii-scan` items exactly as they do for every other category — verify by confirming Task 5.1.1a's diff touches only `ESCALATION_REASON_EMOJI` and its adjacent comment, no JSX.

### Surface 2 — Analytics

6. **Task completion in ≤ 2 clicks**: from the Approval Analytics panel's default view, a team lead can see the `pii-scan` count for a given window by (1) optionally clicking a window-size button (7/14/30/90 days; default 7-day view requires 0 clicks) and (2) visually locating the "PII detected in request" row — no drill-down, no modal.
7. **Label clarity**: the label `"PII detected in request"` reads as a complete, non-technical sentence fragment consistent in tone with its siblings (`"Newly-registered domain"`, `"Plaintext secret detected"`) — verified by direct string comparison against `ESCALATION_CATEGORY_LABELS`'s existing entries (Task 5.2.1a).
8. **Correct empty behavior**: when `pii-scan` count is zero for the selected window but other categories are non-zero, no `pii-scan` row renders (filtered, not shown as "0") — matches every other category's existing zero-count behavior; when *all* categories are zero, the whole-table `"No escalations in this window."` message shows instead of an empty table shell.
9. **No dead ends**: the analytics panel's only failure mode (fetch error) shows the message + Retry button described in Surface 2's edge-case table — reachable recovery action present, not a blank/broken panel.

### Cross-cutting

10. **Accessibility — not color-only (WCAG 1.4.1)**: confirmed structurally (see Accessibility section above) — the `pii-scan` reason line's meaning survives with the emoji removed entirely (e.g., screen-reader-only rendering), satisfied by construction since Task 5.1.1a adds a map entry to an existing text-prefix pattern, not a new color-only element.
11. **Accessibility — keyboard navigable**: every control a reviewer needs to act on a `pii-scan` item (expand, Approve, Deny) is reachable and operable via keyboard alone, using the pre-existing, unmodified interaction handlers — no new custom widget was introduced that could regress keyboard support.
12. **Accessibility — color contrast ≥ 4.5:1**: no new color token was introduced by this feature (Surfaces 1 and 2 both reuse existing typography/table classes) — there is nothing new to contrast-check; the existing classes' contrast ratios are inherited unchanged.
13. **Consistency**: a `pii-scan` item is visually and interactively indistinguishable from a `domain-age` or `explicit-rule` item except for (a) the emoji prefix, (b) the reason text content, and (c) its analytics table label — confirmed by both wireframes above showing the identical card/table shape as the pre-existing categories they're modeled on.

---

## Summary of surfaces designed

| # | Surface | New component? | Wireframe | UX ACs |
|---|---|---|---|---|
| 1 | Review Queue card — escalation reason line (Bash + Write/Edit variants) | No — map entry only | Yes (2 variants) | 5 |
| 2 | Analytics — Escalation Reasons table row (+ zero-count/empty states) | No — map entry only | Yes (2 variants) | 4 |
| 3 | Config toggle | N/A — confirmed no UI exists | N/A | — |
| 4 | Deny-mode message | N/A — confirmed not a web UI surface (agent-side hook response) | N/A | — |

**3 real UI surfaces investigated, 2 confirmed as requiring design work** (both already fully covered by `implementation/plan.md` Phase 5's two map-entry tasks), **13 UX acceptance criteria** written across recognition speed, task-completion step counts, error/no-dead-end handling, and accessibility (WCAG 1.4.1, keyboard nav, contrast).
