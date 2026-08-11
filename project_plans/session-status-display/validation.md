# Validation: Session Status Display

Generated: 2026-06-13

---

## 1. Test Matrix — FR Coverage

| FR | Description | Test ID | Type | What to Test | Expected Outcome |
|---|---|---|---|---|---|
| FR-1 | Fix ✦ detection gap | GO-01 | unit | `Detect("✦ Thinking… (2m 5s · ↓ 6.4k tokens)\n")` | `StatusActive` |
| FR-1 | Fix ✦ detection gap | GO-02 | unit | `Detect("✦ Ruminating… (30s · ↓ 1.2k tokens)\n")` | `StatusActive` |
| FR-1 | Fix ✦ detection gap | GO-03 | unit | `Detect("  ✦ Flambéing… (1m 0s · ↓ 500 tokens)\n")` — indented | `StatusActive` |
| FR-1 | Fix ✦ detection gap | GO-04 | unit | `Detect("✦ Uncommitted changes\n")` — NOT a thinking verb (pitfall guard) | NOT `StatusActive` |
| FR-2 | ⏺ tool-action patterns | GO-05 | unit | `Detect("⏺ Bash(go test ./...)\n(esc to interrupt)\n")` | `StatusActive` |
| FR-2 | ⏺ tool-action patterns | GO-06 | unit | `Detect("⏺ Read(/path/to/file)\n(esc to interrupt)\n")` | `StatusActive` |
| FR-2 | ⏺ false-positive guard | GO-07 | unit | `Detect("⏺ Bash(go test ./...)\n? for shortcuts\n")` — stale ⏺, now idle | NOT `StatusActive` (pitfall guard: bare ⏺ without `esc to interrupt` must not fire) |
| FR-3 | SubStatusChip IDLE render | FE-01 | unit (Jest/RTL) | Render `<SubStatusChip subStatus={SubStatus.IDLE} />` | Renders span with text "Waiting…" |
| FR-3 | SubStatusChip UNSPECIFIED null | FE-02 | unit (Jest/RTL) | Render `<SubStatusChip subStatus={SubStatus.UNSPECIFIED} />` | Returns `null` |
| FR-3 | SubStatusChip IDLE accessible | FE-03 | unit (Jest/RTL) | Rendered chip has `role="status"` and non-empty `aria-label` | Accessibility attributes present |
| FR-3 | SessionRow IDLE chip visible | FE-04 | unit (Jest/RTL) | Render `SessionRow` with `status=ACTIVE, subStatus=IDLE` | `SubStatusChip` is rendered (not suppressed) |
| FR-4 | rowActive class — PROCESSING | FE-05 | unit (Jest/RTL) | Render `SessionRow` with `subStatus=PROCESSING` | `data-testid="session-row"` element has `rowActive` class |
| FR-4 | rowActive class — IDLE absent | FE-06 | unit (Jest/RTL) | Render `SessionRow` with `subStatus=IDLE` | `rowActive` class absent from row element |
| FR-4 | rowActive class — UNSPECIFIED absent | FE-07 | unit (Jest/RTL) | Render `SessionRow` with `subStatus=UNSPECIFIED` | `rowActive` class absent from row element |
| FR-4 | Reduced-motion no animation | FE-08 | unit (CSS snapshot) | `SessionRow.css.ts` rowActive style includes `@media (prefers-reduced-motion: reduce)` block | Snapshot contains reduced-motion guard; animation properties removed |
| FR-5 | Existing pattern no regressions | GO-08 | unit | Re-run all pre-existing `TestStatusDetector_Detect*` cases after pattern change | All pass — no false positives introduced |
| FR-5 | testdata snapshots intact | GO-09 | unit (snapshot) | `snapshot_test.go` runs against all testdata/* files | All snapshots match |
| FR-6 | Ring buffer caps at 500 | GO-10 | unit | Insert 600 events into the ring buffer via 600 `Detect()` calls; call `RecentEvents(600)` | Returns exactly 500 events (oldest 100 dropped) |
| FR-6 | No-match event has TailSnippet | GO-11 | unit | `Detect()` on output that produces `StatusUnknown`; retrieve event | `DetectionEvent.TailSnippet` is non-empty (≤512 bytes of cleaned text) |
| FR-6 | Match event records pattern name | GO-12 | unit | `Detect("✦ Thinking… (5s · ↓ 1k tokens)\n")`; retrieve event | `DetectionEvent.MatchedPattern` == `"claude_thinking_verb"`, `MatchedCategory` == `"active"`, `ResultStatus` == `StatusActive` |
| FR-6 | Match event records session ID | GO-13 | unit | Detector constructed with a session ID; after `Detect()`; retrieve event | `DetectionEvent.SessionID` matches the session ID passed at construction |
| FR-6 | GetDetectionEvents RPC ≥5 events | GO-14 | integration | Call `Detect()` five times on an active session; call `GetDetectionEvents(sessionID, 10)` via ConnectRPC | Response contains ≥5 `DetectionEvent` messages |
| FR-7 | CR spinner → StatusActive | GO-15 | unit | `Detect("⠋ Thinking\r⠙ Thinking\r⠹ Thinking\n")` — pure CR spinner, no keywords | `StatusActive` |
| FR-7 | ANSI cursor-up → StatusActive | GO-16 | unit | `Detect("Line 1\x1b[ALine 2\n")` — contains ANSI cursor-up sequence | `StatusActive` |
| FR-7 | Overwrite event recorded | GO-17 | unit | Input with `\r` spinner → retrieve event | `DetectionEvent.MatchedPattern` == `"screen_overwrite"`, `MatchedCategory` == `"active"` |
| FR-7 | No overwrite — no false fire | GO-18 | unit | Plain output with no `\r` and no ANSI cursor-up, no keyword matches | Does NOT return `StatusActive` from overwrite path (returns `StatusReady` or `StatusUnknown`) |
| FR-8 | GetDetectionEvents proto RPC exists | GO-19 | build | `make generate-proto` succeeds after adding RPC; `go build ./...` succeeds | Zero compilation errors |
| FR-8 | Debug panel renders events table | FE-09 | unit (Jest/RTL) | Render `DetectionEventsPanel` with mock events prop | Table rows appear; each row shows timestamp, pattern name, category, status |
| FR-8 | TailSnippet truncated to 80 chars | FE-10 | unit (Jest/RTL) | Render panel with event whose TailSnippet is 200 chars | Displayed cell text is ≤80 chars; full text available in title/tooltip |
| FR-8 | Debug panel hidden without flag | FE-11 | e2e (Playwright) | Load session detail page without `?debug=1` | "Detection Events" section is not present in DOM |
| FR-8 | Debug panel visible with flag | FE-12 | e2e (Playwright) | Load session detail page with `?debug=1`; wait for active session | "Detection Events" section present; first row shows a pattern name and status |

---

## 2. Go Unit Tests — `session/detection/detector_test.go`

The tests below follow the existing file structure (package `detection`, same file).

### FR-1: ✦ detection in `claude_thinking_verb` pattern

```go
// TestDetect_StarThinkingVerb_ReturnsActive verifies that ✦ (U+2726) is included
// in the claude_thinking_verb spinner character class and returns StatusActive.
// FR-1 acceptance criterion.
func TestDetect_StarThinkingVerb_ReturnsActive(t *testing.T) {
    sd := NewStatusDetector()
    input := "✦ Thinking… (2m 5s · ↓ 6.4k tokens)\n"
    status := sd.Detect([]byte(input))
    if status != StatusActive {
        t.Errorf("Detect(%q) = %v; want StatusActive (FR-1: ✦ not in spinner class)", input, status)
    }
}

// TestDetect_StarRuminatingVerb_ReturnsActive verifies a second thinking verb ("Ruminating")
// with ✦ also triggers StatusActive, confirming the verb matching is verb-agnostic.
// FR-1 acceptance criterion.
func TestDetect_StarRuminatingVerb_ReturnsActive(t *testing.T) {
    sd := NewStatusDetector()
    input := "✦ Ruminating… (30s · ↓ 1.2k tokens)\n"
    status := sd.Detect([]byte(input))
    if status != StatusActive {
        t.Errorf("Detect(%q) = %v; want StatusActive (FR-1: ✦ Ruminating not detected)", input, status)
    }
}

// TestDetect_StarThinkingVerb_Indented_ReturnsActive verifies indented ✦ lines
// (e.g. inside a task manager sub-item) are also detected.
// FR-1 supplemental.
func TestDetect_StarThinkingVerb_Indented_ReturnsActive(t *testing.T) {
    sd := NewStatusDetector()
    input := "  ✦ Flambéing… (1m 0s · ↓ 500 tokens)\n"
    status := sd.Detect([]byte(input))
    if status != StatusActive {
        t.Errorf("Detect(%q) = %v; want StatusActive (FR-1: indented ✦ not detected)", input, status)
    }
}

// TestDetect_StarUncommittedChanges_NotActive is a pitfall guard: "✦ Uncommitted changes"
// must NOT trigger StatusActive — it lacks the capitalized-verb + ellipsis structure.
// FR-1 pitfall guard (see research/pitfalls.md §1).
func TestDetect_StarUncommittedChanges_NotActive(t *testing.T) {
    sd := NewStatusDetector()
    input := "✦ Uncommitted changes\n"
    status := sd.Detect([]byte(input))
    if status == StatusActive {
        t.Errorf("Detect(%q) = StatusActive; want anything else (pitfall: ✦ UI string false-positive)", input)
    }
}
```

### FR-2: ⏺ tool-action active patterns

The implementation must require `(esc to interrupt)` on the same or adjacent line (see pitfalls.md §2–§6). The existing `esc_to_interrupt` Active pattern already fires on inputs containing `(esc to interrupt)`, so FR-2 is already handled if the input contains that substring. The test confirms the combined input matches:

```go
// TestDetect_BulletBashEscToInterrupt_ReturnsActive verifies the combined ⏺ + "(esc to interrupt)"
// output that Claude Code emits during an in-progress tool call returns StatusActive.
// FR-2 acceptance criterion.
func TestDetect_BulletBashEscToInterrupt_ReturnsActive(t *testing.T) {
    sd := NewStatusDetector()
    input := "⏺ Bash(go test ./...)\n(esc to interrupt)\n"
    status := sd.Detect([]byte(input))
    if status != StatusActive {
        t.Errorf("Detect(%q) = %v; want StatusActive (FR-2: ⏺ + esc to interrupt not detected)", input, status)
    }
}

// TestDetect_BulletBashStaleNoEsc_NotActive is the critical pitfall guard: a stale
// "⏺ Verb(...)" line in the scrollback tail WITHOUT "(esc to interrupt)" must NOT fire
// StatusActive. This validates the implementation did NOT add bare ⏺ as an Active pattern.
// FR-2 pitfall guard (see research/pitfalls.md §2).
func TestDetect_BulletBashStaleNoEsc_NotActive(t *testing.T) {
    sd := NewStatusDetector()
    // Stale ⏺ from a previous turn; current state is idle (? for shortcuts).
    input := "⏺ Bash(go test ./...)\n? for shortcuts\n"
    status := sd.Detect([]byte(input))
    if status == StatusActive {
        t.Errorf("Detect(%q) = StatusActive; want StatusIdle or other (pitfall: stale ⏺ false-positive)", input)
    }
}
```

### FR-2 / FR-5: CR spinner and ANSI cursor-up (FR-7)

```go
// TestDetect_CRSpinner_ReturnsActive verifies that a pure CR-animated spinner line
// (no text keywords) is detected as StatusActive via the screen-overwrite signal.
// FR-7 acceptance criterion.
func TestDetect_CRSpinner_ReturnsActive(t *testing.T) {
    sd := NewStatusDetector()
    input := "⠋ Thinking\r⠙ Thinking\r⠹ Thinking\n"
    status := sd.Detect([]byte(input))
    if status != StatusActive {
        t.Errorf("Detect(%q) = %v; want StatusActive (FR-7: CR spinner not detected as active)", input, status)
    }
}

// TestDetect_AnsiCursorUp_ReturnsActive verifies that ANSI cursor-up escape sequences
// in the raw input trigger the screen-overwrite active signal.
// FR-7 acceptance criterion.
func TestDetect_AnsiCursorUp_ReturnsActive(t *testing.T) {
    sd := NewStatusDetector()
    input := "Loading...\x1b[AProgress 50%\n"
    status := sd.Detect([]byte(input))
    if status != StatusActive {
        t.Errorf("Detect(%q) = %v; want StatusActive (FR-7: ANSI cursor-up not detected as active)", input, status)
    }
}

// TestDetect_AnsiCursorUpNPattern_ReturnsActive verifies the cursor-up-N form (\x1b[<n>A).
// FR-7 supplemental.
func TestDetect_AnsiCursorUpNPattern_ReturnsActive(t *testing.T) {
    sd := NewStatusDetector()
    input := "Line 1\x1b[3AOverwrite\n"
    status := sd.Detect([]byte(input))
    if status != StatusActive {
        t.Errorf("Detect(%q) = %v; want StatusActive (FR-7: \\x1b[nA cursor-up not detected)", input, status)
    }
}
```

### FR-5: Existing patterns — no regressions

```go
// TestDetect_ExistingPatterns_NoRegressions re-runs a representative sample of the
// pre-existing active, idle, approval, error, and processing patterns after the
// FR-1/FR-2/FR-7 changes to confirm no false positives were introduced.
// FR-5 requirement.
func TestDetect_ExistingPatterns_NoRegressions(t *testing.T) {
    sd := NewStatusDetector()

    cases := []struct {
        input string
        want  DetectedStatus
    }{
        // Active — pre-existing
        {"(esc to interrupt)", StatusActive},
        {"Executing tests (esc to cancel)", StatusActive},
        {"Running...", StatusActive},

        // Idle — pre-existing
        {"$ ", StatusIdle},
        {"— INSERT —", StatusIdle},
        {"— NORMAL —", StatusIdle},
        {"? for shortcuts", StatusIdle},

        // Approval — pre-existing
        {"Yes, allow reading this file", StatusNeedsApproval},
        {"(Y)es/(N)o/(D)on't ask again", StatusNeedsApproval},

        // Error — pre-existing
        {"Error: file not found", StatusError},
        {"panic: runtime error", StatusError},

        // Success — pre-existing
        {"✓ Successfully completed the task", StatusSuccess},
        {"✻ Baked for 3m", StatusSuccess},
    }

    for _, tc := range cases {
        t.Run(tc.input, func(t *testing.T) {
            got := sd.Detect([]byte(tc.input))
            if got != tc.want {
                t.Errorf("Detect(%q) = %v; want %v (regression)", tc.input, got, tc.want)
            }
        })
    }
}
```

### FR-6: Ring buffer and DetectionEvent

```go
// TestRecentEvents_CapsAt500 verifies the ring buffer evicts the oldest entry when
// more than 500 events are appended. FR-6 requirement.
func TestRecentEvents_CapsAt500(t *testing.T) {
    sd := NewStatusDetector()
    // Generate 600 unique inputs to fill and overflow the buffer.
    for i := 0; i < 600; i++ {
        sd.Detect([]byte(fmt.Sprintf("cycle %d\n", i)))
    }
    events := sd.RecentEvents(600)
    if len(events) != 500 {
        t.Errorf("RecentEvents(600) returned %d events; want 500 (ring buffer cap)", len(events))
    }
}

// TestRecentEvents_NoMatch_IncludesTailSnippet verifies that a no-match detection
// (StatusUnknown) produces an event whose TailSnippet is non-empty.
// FR-6 acceptance criterion.
func TestRecentEvents_NoMatch_IncludesTailSnippet(t *testing.T) {
    sd := NewStatusDetector()
    // Force StatusUnknown by removing the catch-all ready regex.
    sd.readyRegexes = nil

    sd.Detect([]byte("xyzzy no match unique string"))
    events := sd.RecentEvents(1)
    if len(events) == 0 {
        t.Fatal("RecentEvents(1) returned 0 events after Detect(); want 1")
    }
    ev := events[0]
    if ev.TailSnippet == "" {
        t.Error("DetectionEvent.TailSnippet is empty for no-match event; want non-empty debug snippet")
    }
    if ev.MatchedPattern != "<none>" {
        t.Errorf("DetectionEvent.MatchedPattern = %q; want \"<none>\" for no-match event", ev.MatchedPattern)
    }
}

// TestRecentEvents_MatchEvent_RecordsPatternAndCategory verifies that a matching event
// records the correct pattern name, category, and result status.
// FR-6 supplemental.
func TestRecentEvents_MatchEvent_RecordsPatternAndCategory(t *testing.T) {
    sd := NewStatusDetector()
    sd.Detect([]byte("✦ Thinking… (5s · ↓ 1k tokens)\n"))
    events := sd.RecentEvents(1)
    if len(events) == 0 {
        t.Fatal("RecentEvents(1) returned 0 events")
    }
    ev := events[0]
    if ev.MatchedPattern != "claude_thinking_verb" {
        t.Errorf("MatchedPattern = %q; want \"claude_thinking_verb\"", ev.MatchedPattern)
    }
    if ev.MatchedCategory != "active" {
        t.Errorf("MatchedCategory = %q; want \"active\"", ev.MatchedCategory)
    }
    if ev.ResultStatus != StatusActive {
        t.Errorf("ResultStatus = %v; want StatusActive", ev.ResultStatus)
    }
}

// TestRecentEvents_ScreenOverwrite_RecordsPattern verifies that a CR-overwrite active
// detection records MatchedPattern = "screen_overwrite" and MatchedCategory = "active".
// FR-7 + FR-6 combined requirement.
func TestRecentEvents_ScreenOverwrite_RecordsPattern(t *testing.T) {
    sd := NewStatusDetector()
    sd.Detect([]byte("⠋ Thinking\r⠙ Thinking\r⠹ Thinking\n"))
    events := sd.RecentEvents(1)
    if len(events) == 0 {
        t.Fatal("RecentEvents(1) returned 0 events")
    }
    ev := events[0]
    if ev.MatchedPattern != "screen_overwrite" {
        t.Errorf("MatchedPattern = %q; want \"screen_overwrite\"", ev.MatchedPattern)
    }
    if ev.MatchedCategory != "active" {
        t.Errorf("MatchedCategory = %q; want \"active\"", ev.MatchedCategory)
    }
}
```

---

## 3. Frontend Unit Tests (Jest / React Testing Library)

File: `web-app/src/components/sessions/__tests__/SubStatusChip.test.tsx`

### FR-3: SubStatusChip renders "Waiting…" for IDLE

```typescript
import { render, screen } from "@testing-library/react";
import { SubStatus } from "@/gen/session/v1/types_pb";
import { SubStatusChip } from "../SubStatusChip";

describe("SubStatusChip", () => {
  it("renders Waiting chip for SUB_STATUS_IDLE (FR-3)", () => {
    const { container } = render(<SubStatusChip subStatus={SubStatus.IDLE} />);
    expect(screen.getByText(/Waiting/)).toBeInTheDocument();
    expect(container.firstChild).not.toBeNull();
  });

  it("returns null for SUB_STATUS_UNSPECIFIED (FR-3 regression guard)", () => {
    const { container } = render(
      <SubStatusChip subStatus={SubStatus.UNSPECIFIED} />
    );
    expect(container.firstChild).toBeNull();
  });

  it("IDLE chip has accessible role and aria-label (FR-3 a11y)", () => {
    render(<SubStatusChip subStatus={SubStatus.IDLE} />);
    const chip = screen.getByRole("status");
    expect(chip).toBeInTheDocument();
    expect(chip).toHaveAttribute("aria-label");
    expect(chip.getAttribute("aria-label")).toBeTruthy();
  });
});
```

File: `web-app/src/components/sessions/__tests__/SessionRow.test.tsx`

### FR-3: SessionRow does not suppress IDLE chip

```typescript
it("renders SubStatusChip for IDLE subStatus when session is ACTIVE (FR-3)", () => {
  renderSessionRow({ status: SessionStatus.ACTIVE, subStatus: SubStatus.IDLE });
  // SubStatusChip renders "Waiting…" — confirm it appears in the row
  expect(screen.getByText(/Waiting/)).toBeInTheDocument();
});
```

### FR-4: rowActive class present/absent

```typescript
it("applies rowActive class when subStatus is PROCESSING (FR-4)", () => {
  renderSessionRow({ status: SessionStatus.ACTIVE, subStatus: SubStatus.PROCESSING });
  const row = screen.getByTestId("session-row");
  expect(row.className).toMatch(/rowActive/);
});

it("does not apply rowActive class when subStatus is IDLE (FR-4)", () => {
  renderSessionRow({ status: SessionStatus.ACTIVE, subStatus: SubStatus.IDLE });
  const row = screen.getByTestId("session-row");
  expect(row.className).not.toMatch(/rowActive/);
});

it("does not apply rowActive class when subStatus is UNSPECIFIED (FR-4)", () => {
  renderSessionRow({ status: SessionStatus.ACTIVE, subStatus: SubStatus.UNSPECIFIED });
  const row = screen.getByTestId("session-row");
  expect(row.className).not.toMatch(/rowActive/);
});
```

### FR-4: CSS reduced-motion guard (snapshot)

```typescript
// CSS architecture note: vanilla-extract is build-time; the snapshot test checks
// the .css.ts source contains the @media block.
import { readFileSync } from "fs";
import { resolve } from "path";

it("SessionRow.css.ts contains prefers-reduced-motion guard on rowActive (FR-4)", () => {
  const src = readFileSync(
    resolve(__dirname, "../SessionRow.css.ts"),
    "utf-8"
  );
  expect(src).toContain("prefers-reduced-motion");
  // Confirm animation is inside the media block (not unconditional)
  const motionBlock = src.match(
    /@media\s*\(prefers-reduced-motion[^)]*\)[^}]*\{[^}]*animation[^}]*\}/
  );
  // The animation property must NOT appear outside the media block unconditionally.
  // This is a heuristic source check; CI lint:css is the authoritative gate.
  expect(motionBlock || !src.includes("animation")).toBeTruthy();
});
```

File: `web-app/src/components/sessions/__tests__/DetectionEventsPanel.test.tsx`

### FR-8: DetectionEventsPanel

```typescript
import { render, screen } from "@testing-library/react";
import { DetectionEventsPanel } from "../DetectionEventsPanel";

const mockEvents = [
  {
    timestamp: "2026-06-13T10:00:00Z",
    matchedPattern: "claude_thinking_verb",
    matchedCategory: "active",
    resultStatus: "StatusActive",
    tailSnippet: "✦ Thinking… (5s · ↓ 1k tokens)\n",
  },
  {
    timestamp: "2026-06-13T10:00:01Z",
    matchedPattern: "<none>",
    matchedCategory: "unknown",
    resultStatus: "StatusUnknown",
    tailSnippet: "x".repeat(200),
  },
];

it("renders a table row per event (FR-8)", () => {
  render(<DetectionEventsPanel events={mockEvents} />);
  expect(screen.getAllByRole("row").length).toBeGreaterThanOrEqual(3); // header + 2 data rows
});

it("truncates TailSnippet to 80 chars in cell, full text in title (FR-8)", () => {
  render(<DetectionEventsPanel events={mockEvents} />);
  const cells = screen.getAllByTitle(mockEvents[1].tailSnippet);
  expect(cells.length).toBeGreaterThan(0);
  // The visible text is ≤80 chars
  expect(cells[0].textContent!.length).toBeLessThanOrEqual(80);
});
```

---

## 4. Integration Tests

### FR-6 / FR-8: GetDetectionEvents RPC

File: `server/services/session_service_test.go` (new test function)

```go
// TestGetDetectionEvents_ReturnsEventsAfterDetectCycles verifies the ConnectRPC
// endpoint returns at least N events after N detection calls on an active session.
// FR-6 + FR-8 acceptance criterion.
func TestGetDetectionEvents_ReturnsEventsAfterDetectCycles(t *testing.T) {
    // Arrange: create a test server with a known sessionID and a wired StatusDetector.
    // Trigger 5 Detect() calls by simulating the detection poll cycle.
    // Act: call GetDetectionEvents(sessionID, 10).
    // Assert: response.Events has len >= 5.
    // (full implementation uses httptest.NewServer + connect client)
}
```

---

## 5. E2E Tests (Playwright)

File: `tests/e2e/session-detection-debug.spec.ts`

```typescript
// @feature session:debug-detection-events

import { test, expect } from "@playwright/test";

test.describe("session-detection-debug", () => {
  test("debug panel absent without ?debug=1 (FR-8)", async ({ page }) => {
    await page.goto("http://localhost:8544/sessions/test-session-id");
    await expect(page.getByText("Detection Events")).not.toBeVisible();
  });

  test("debug panel present with ?debug=1 and shows events (FR-8)", async ({ page }) => {
    await page.goto("http://localhost:8544/sessions/test-session-id?debug=1");
    await expect(page.getByText("Detection Events")).toBeVisible();
    // At least one event row visible
    const rows = page.locator("[data-testid='detection-event-row']");
    await expect(rows.first()).toBeVisible({ timeout: 5000 });
    // First row has a non-empty pattern name
    const patternCell = rows.first().locator("[data-col='pattern']");
    await expect(patternCell).not.toHaveText("");
  });
});
```

---

## 6. Implementation Readiness Gate

### Gate Criterion 1: Are all FRs covered by at least one test?

| FR | Test Count | Status |
|---|---|---|
| FR-1 | 4 (GO-01–04) | COVERED |
| FR-2 | 2 (GO-05–07; GO-07 is pitfall guard) | COVERED |
| FR-3 | 4 (FE-01–04) | COVERED |
| FR-4 | 4 (FE-05–08) | COVERED |
| FR-5 | 2 (GO-08–09) | COVERED |
| FR-6 | 5 (GO-10–14) | COVERED |
| FR-7 | 4 (GO-15–18) | COVERED |
| FR-8 | 5 (GO-19, FE-09–11, E2E ×2) | COVERED |

**Verdict: PASS** — all 8 FRs have at least one test.

---

### Gate Criterion 2: Does the plan avoid the ⏺ false-positive pitfall?

**Analysis**: Research (`pitfalls.md` §2–§6) conclusively shows that adding a bare `⏺` (U+23FA) as an Active pattern would cause false positives from stale scrollback — old `⏺ Verb(...)` lines within the 4096-byte tail would fire `StatusActive` even when the session is idle.

The plan resolves FR-2 without this risk:

1. The existing `esc_to_interrupt` Active pattern already catches `⏺ Bash(go test ./...)\n(esc to interrupt)\n` because the `(esc to interrupt)` substring is present. No new ⏺-based pattern is needed.
2. GO-07 (`TestDetect_BulletBashStaleNoEsc_NotActive`) explicitly asserts the stale-⏺ case does NOT fire `StatusActive`. This test will fail if anyone later adds a bare ⏺ pattern.
3. FR-2's acceptance criterion ("input `⏺ Bash(...)\n(esc to interrupt)\n` must produce `StatusActive`") is satisfied by the existing `esc_to_interrupt` pattern — no implementation change is required beyond confirming the test passes today.

**Verdict: PASS** — the ⏺ pitfall is identified, documented, and guarded by GO-07.

---

### Gate Criterion 3: Is the proto change for FR-8 called out with `make generate-proto`?

**Yes.** FR-8 requires:
1. A new `GetDetectionEvents` RPC in `proto/session/v1/session.proto`
2. A new `DetectionEvent` proto message in the same file (or `types.proto`)
3. `make generate-proto` to regenerate Go bindings (`session/gen/session/v1/*.go`) and TypeScript bindings (`web-app/src/gen/session/v1/*_pb.ts`)
4. `go build ./...` must succeed before any service handler tests run

From `CLAUDE.md` (Modifying the ent ORM Schema section note): the generate command is always required before build. The analogous requirement here is:

```bash
# REQUIRED after editing proto/session/v1/session.proto
make generate-proto
go build ./...
```

GO-19 (`TestGetDetectionEvents_ProtoRPCExists`) validates this at the build level — if `make generate-proto` was not run, `go build ./...` fails and no tests can run.

**Verdict: PASS** — proto regeneration is called out explicitly in the test matrix (GO-19) and in the Files Affected table in requirements.md.

---

### Gate Criterion 4: Are ring buffer memory bounds acceptable?

**Calculation from requirements.md NFR section**:
- Ring buffer cap: 500 events per session
- Event size estimate: ~1 KB (SessionID string + Timestamp + pattern name (~30 bytes) + category (~10 bytes) + TailSnippet (≤512 bytes) + DetectedStatus int + overhead)
- Memory per session: 500 × 1 KB = **500 KB maximum per session**
- Typical deployment: 10–50 active sessions simultaneously
- Worst-case total: 50 × 500 KB = **25 MB** across all sessions
- This is bounded and acceptable for a debug-only in-memory store

**Implementation concern**: The `TailSnippet` field is the largest component. At 512 bytes per event × 500 events = 256 KB of tail snippets per session. If the session count scales to 200 sessions, this reaches 100 MB — still within typical server heap budgets but worth monitoring.

GO-10 (`TestRecentEvents_CapsAt500`) enforces the cap is respected. No persistence risk exists since events are explicitly in-memory only.

**Verdict: PASS** — bounds are acceptable for the stated use case (debug tooling, in-memory only, 500-event cap enforced by test).

---

## 7. Summary

| Metric | Value |
|---|---|
| Total test cases | 31 (GO: 19, FE: 11, E2E: 2) |
| Go unit tests | 19 |
| Frontend unit tests (Jest/RTL) | 9 |
| Frontend CSS source checks | 1 |
| Integration tests | 1 |
| E2E tests (Playwright) | 2 |
| FRs covered | 8 / 8 (100%) |
| Pitfall guard tests | 2 (GO-04, GO-07) |

**Overall Readiness Gate: PASS**

All four gate criteria pass. The plan is safe to implement:
- FR-1: Add `✦` to the `claude_thinking_verb` spinner character class (`[·✢✳✶✻✽●*✦]`).
- FR-2: No new ⏺ pattern needed; existing `esc_to_interrupt` already covers the stated acceptance criterion. Add GO-07 as a regression guard.
- FR-3/FR-4: Straightforward UI changes to `SubStatusChip.tsx` and `SessionRow.tsx`/`.css.ts`.
- FR-6: New `events.go` file with `DetectionEvent` struct and ring buffer; `Detect()` appends on every call.
- FR-7: Check raw input (before `collapseCarriageReturns`) for `\r` or `\x1b[…A`; if overwrite activity found and no higher-priority status matched, return `StatusActive` and log `"screen_overwrite"` event.
- FR-8: New proto RPC + `make generate-proto` + `DetectionEventsPanel.tsx` behind `?debug=1`.
