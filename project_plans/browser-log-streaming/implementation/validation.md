# Browser Log Streaming — Test Validation Plan

_Status_: Draft  
_Created_: 2026-05-02  
_Source plan_: `plan.md`  
_Requirements_: `../requirements.md`

---

## Requirement-to-Test Traceability Matrix

| Req ID | Description (abbreviated) | Test IDs |
|--------|---------------------------|----------|
| FR-1 | Capture console.{log,warn,error,debug}, onerror, unhandledrejection | UT-F-02, UT-F-03, UT-F-04, UT-F-05, UT-F-11, UT-F-12 |
| FR-2a | Buffer up to 50 entries; flush at cap | UT-F-06 |
| FR-2b | Flush on 5 s timer | UT-F-07 |
| FR-2c | Message body capped at 200 chars with `…` | UT-F-04, UT-B-10 |
| FR-2d | Max 1 flush per 5 s (rate) | UT-F-08 |
| FR-2e | `sendBeacon` flush on page unload | UT-F-16 |
| FR-3 | POST /api/v1/client-logs, 204 on success | UT-B-01, UT-B-02, UT-B-03, IT-01 |
| FR-3 | 400 on malformed JSON | UT-B-05 |
| FR-3 | Inherits auth middleware (no extra wiring) | IT-02 |
| FR-4 | Server-side `[client-log]` prefix and level routing | UT-B-08, UT-B-09 |
| FR-4 | UserAgent truncated to 80 chars | UT-B-11 |
| FR-5 | Remote Debug button in devOnly cluster | UT-UI-01 |
| FR-5 | Hidden on ≤768px | UT-UI-02 |
| FR-5 | localStorage key `stapler-squad-remote-debug` | UT-UI-03, UT-UI-04 |
| FR-5 | Default off | UT-UI-05 |
| FR-5 | Green indicator when on | UT-UI-06 |
| FR-6 | Feature off → no console patching, no HTTP calls | UT-F-01, AC-1 |
| FR-6 | POST failure silently discarded | UT-F-09 |
| AC-1 | Remote Debug OFF: no patches, no HTTP | UT-F-01 |
| AC-2 | Remote Debug ON: console.error in browser → log line ≤ 10 s | IT-03 |
| AC-3 | >200 char message truncated | UT-F-04, UT-B-10 |
| AC-4 | >50 entries in 5 s → 1 HTTP call (50 entries) | UT-F-06, UT-F-08 |
| AC-5 | Toggle OFF removes patches, stops HTTP | UT-F-10, UT-F-14, UT-F-15 |
| AC-6 | Remote Debug hidden ≤768px | UT-UI-02 |
| AC-7 | onerror / unhandledrejection appear in server log | UT-B-06, UT-F-11, UT-F-12 |
| AC-8 | POST returns 204; handler does not crash on malformed JSON | UT-B-01, UT-B-05 |

Coverage: **8/8 functional requirements covered**, **8/8 acceptance criteria covered**.

---

## Test Suite

### Go Unit Tests — `server/handlers/browser_log_handler_test.go`

| ID | Function | What it tests |
|----|----------|---------------|
| UT-B-01 | `TestBrowserLog_ValidSingleEntry` | Single valid entry → 204 No Content |
| UT-B-02 | `TestBrowserLog_ValidBatch` | 50-entry valid batch → 204 |
| UT-B-03 | `TestBrowserLog_EmptyEntries` | Zero entries → 204 (no-op, not error) |
| UT-B-04 | `TestBrowserLog_MethodNotAllowed` | GET request → 405 |
| UT-B-05 | `TestBrowserLog_MalformedJSON` | Broken JSON body → 400 |
| UT-B-06 | `TestBrowserLog_OversizedEntries` | 201 entries → 400 |
| UT-B-07 | `TestBrowserLog_OversizedBody` | Body > 64 KB → 400 (MaxBytesReader) |
| UT-B-08 | `TestBrowserLog_RateLimit` | 101st request in same window → 429 |
| UT-B-09 | `TestBrowserLog_LogInjection_Newline` | Message with `\n` → sanitized before logging |
| UT-B-10 | `TestBrowserLog_LogInjection_CarriageReturn` | Message with `\r` → sanitized |
| UT-B-11 | `TestBrowserLog_MessageTruncation` | Message 300 chars → truncated to 200 + `…` in log |
| UT-B-12 | `TestBrowserLog_UAShortened` | UserAgent 200 chars → truncated to 80 + `…` in log |
| UT-B-13 | `TestBrowserLog_ErrorLevelRoutesToErrorLog` | `level: "error"` → logged at ErrorLog |
| UT-B-14 | `TestBrowserLog_OtherLevelRoutesToInfoLog` | `level: "log"/"warn"/"debug"` → InfoLog |
| UT-B-15 | `TestBrowserLog_MissingOptionalFields` | Entry without sessionId/url → does not crash |

**Count**: 15 Go unit tests

#### Notes on testability

`logEntry` writes directly to `log.InfoLog` / `log.ErrorLog`. To assert on log output, the tests
can either:
- (a) Capture the log output by setting `log.InfoLog.SetOutput(buf)` in the test and checking
  `buf.String()` contains the expected substring (same technique that can be used for telemetry
  handler logging); or
- (b) Test the public HTTP surface (status codes) and treat log content as out-of-band for unit
  tests, relying on integration tests for end-to-end log assertion.

Recommended: tests UT-B-11 through UT-B-14 should use approach (a) with a `bytes.Buffer` writer
to assert on the sanitized log output. All other tests need only assert on the HTTP response code.

---

### Frontend Hook Unit Tests — `web-app/src/lib/hooks/__tests__/useBrowserLogStream.test.ts`

| ID | Test name | What it tests |
|----|-----------|---------------|
| UT-F-01 | `disabled_should_not_patch_console_methods` | `enabled: false` → console unchanged (ref equality) |
| UT-F-02 | `enabled_should_patch_all_four_console_methods` | `console.{log,warn,error,debug}` are replaced |
| UT-F-03 | `enabled_should_call_through_to_originals` | Patched methods invoke the captured originals |
| UT-F-04 | `enqueue_should_truncate_message_at_200_chars` | 300-char message → 200 chars + `…` in POST body |
| UT-F-05 | `enqueue_should_include_level_url_userAgent_timestamp` | Entry fields populated correctly |
| UT-F-06 | `buffer_at_cap_should_trigger_immediate_flush` | 50th entry → `fetch` called without waiting for timer |
| UT-F-07 | `flush_timer_should_fire_after_5000ms` | `jest.advanceTimersByTime(5000)` → `fetch` called |
| UT-F-08 | `overflow_beyond_cap_should_not_send_extra_calls_in_window` | 60 entries → only 1 `fetch` call |
| UT-F-09 | `fetch_failure_should_be_silently_swallowed` | Rejected `fetch` → no unhandled rejection, no console.error |
| UT-F-10 | `toggle_off_should_restore_original_console_methods` | Unmount/disable → `console.log === original` |
| UT-F-11 | `window_onerror_should_be_intercepted` | `window.onerror` replaced; error event → enqueued |
| UT-F-12 | `unhandledrejection_should_be_intercepted` | `addEventListener` called; event → enqueued |
| UT-F-13 | `reentrancy_guard_prevents_infinite_loop` | `console.error` inside flush → not re-enqueued |
| UT-F-14 | `toggle_off_should_cancel_pending_flush_timer` | `clearTimeout` called on disable |
| UT-F-15 | `toggle_off_should_clear_buffer` | Pending entries discarded on disable |
| UT-F-16 | `beforeunload_should_trigger_sendBeacon` | `window.dispatchEvent(new Event('beforeunload'))` → `navigator.sendBeacon` called |
| UT-F-17 | `session_id_included_in_posted_entry` | `sessionId` from options appears in `entries[0].sessionId` |
| UT-F-18 | `circular_reference_arg_falls_back_to_string` | Circular object in args → `String()` fallback, no throw |
| UT-F-19 | `args_limited_to_5_per_entry` | 10-arg `console.log` → message derived from first 5 only |
| UT-F-20 | `disabled_should_not_make_http_calls` | `enabled: false` the whole time → `fetch` never called |

**Count**: 20 frontend hook unit tests

#### Test setup requirements

```ts
beforeEach(() => {
  jest.useFakeTimers();
  mockFetch = jest.fn().mockResolvedValue({ ok: true } as Response);
  global.fetch = mockFetch;
  // Stub navigator.sendBeacon
  Object.defineProperty(navigator, 'sendBeacon', {
    writable: true, configurable: true,
    value: jest.fn().mockReturnValue(true),
  });
});
afterEach(() => {
  // Restore console methods (safety net in addition to hook cleanup)
  console.log = origLog; console.warn = origWarn;
  console.error = origError; console.debug = origDebug;
  window.onerror = null;
  jest.useRealTimers();
  jest.restoreAllMocks();
});
```

---

### Frontend Component Tests — `TerminalOutput.logstream.test.tsx`

| ID | Test name | What it tests |
|----|-----------|---------------|
| UT-UI-01 | `renders_log_stream_button_in_expanded_toolbar` | Button present with text "Log Stream" |
| UT-UI-02 | `log_stream_button_has_devOnly_class` | `devOnly` CSS class applied (hidden on mobile by CSS) |
| UT-UI-03 | `toggle_on_calls_localStorage_setItem_with_correct_key` | `localStorage.setItem('stapler-squad-remote-debug', 'true')` |
| UT-UI-04 | `toggle_off_calls_localStorage_removeItem` | `localStorage.removeItem('stapler-squad-remote-debug')` |
| UT-UI-05 | `default_state_is_off_when_localStorage_empty` | Button shows "Log Stream" (not "Log Stream ON") initially |
| UT-UI-06 | `active_state_shows_ON_label_and_green_style` | Text "Log Stream ON"; `style.backgroundColor === '#2a4'` |
| UT-UI-07 | `hook_called_with_sessionId_prop` | `useBrowserLogStream` mock receives `sessionId` matching prop |
| UT-UI-08 | `initializes_from_localStorage_true` | Seed `localStorage['stapler-squad-remote-debug'] = 'true'` → ON |

**Count**: 8 component tests

Note: `TerminalOutput` has significant dependencies (xterm.js, ConnectRPC hooks, etc.). These tests
should mock `useBrowserLogStream` at the module boundary and mock `useTerminalStream` to avoid
xterm initialization. Use the same mocking strategy that any existing `TerminalOutput` test uses.

---

### Integration Tests

| ID | Description | How |
|----|-------------|-----|
| IT-01 | `POST /api/v1/client-logs` with valid payload returns 204 | `httptest.NewServer` + real handler; no mocking |
| IT-02 | Auth middleware passes through to handler when cookie present | `httptest.NewServer` wired with auth + handler; send valid cookie |
| IT-03 | End-to-end: browser `console.error` → server log entry | Run actual server (`STAPLER_SQUAD_INSTANCE=e2e-local`); Playwright test triggers `console.error`; assert log file contains `[client-log] error` |

**Count**: 3 integration tests (2 Go, 1 E2E Playwright)

#### IT-03 Playwright spec outline

```ts
// tests/e2e/browser-log-streaming.spec.ts
// @feature session:browser-log-streaming

test.describe('browser-log-streaming', () => {
  test('browser_console_error_should_appear_in_server_log', async ({ page }) => {
    await page.goto('/');
    // Enable remote debug via localStorage before reload
    await page.evaluate(() => {
      localStorage.setItem('stapler-squad-remote-debug', 'true');
    });
    await page.reload();
    // Trigger a console.error in the browser context
    await page.evaluate(() => { console.error('e2e-test-log-streaming'); });
    // Wait for the batch flush (5s + buffer)
    await page.waitForTimeout(6000);
    // Assert server log contains the entry
    // (read log file via API or a dedicated test endpoint)
    const logContent = /* read ~/.stapler-squad/logs/stapler-squad.log */ '';
    expect(logContent).toContain('[client-log] error');
    expect(logContent).toContain('e2e-test-log-streaming');
  });
});
```

_Note_: `waitForTimeout` is banned by project E2E conventions. Replace with a poll on a test
endpoint that exposes the last N log lines, or use `expect.poll` with a deterministic log-tail API.
This is noted as a pre-merge task: the E2E test must not use `waitForTimeout`.

---

## Test Count Summary

| Layer | Count | Files |
|-------|-------|-------|
| Go unit tests (handler) | 15 | `server/handlers/browser_log_handler_test.go` |
| Frontend hook unit tests | 20 | `web-app/src/lib/hooks/__tests__/useBrowserLogStream.test.ts` |
| Frontend component tests | 8 | `web-app/src/components/sessions/__tests__/TerminalOutput.logstream.test.tsx` |
| Go integration tests | 2 | `server/handlers/browser_log_handler_test.go` (integration subset) |
| E2E Playwright tests | 1 | `tests/e2e/browser-log-streaming.spec.ts` |
| **Total** | **46** | 4 test files |

---

## Requirements Coverage Fraction

| Requirement type | Total | Covered | Fraction |
|---|---|---|---|
| Functional requirements (FR-1 … FR-6) | 6 | 6 | **6/6 (100%)** |
| Acceptance criteria (AC-1 … AC-8) | 8 | 8 | **8/8 (100%)** |
| Non-functional (perf, privacy, security, bundle) | 4 | 2 direct† | 2/4 (50%)† |

† NFR coverage notes:
- **Security** (log injection prevention): covered by UT-B-09, UT-B-10.
- **Privacy** (no PII beyond URL/UA): covered by UT-F-05 (verifies only expected fields are sent).
- **Performance** (< 1 ms overhead): not covered by automated tests — would require a micro-benchmark; deferred.
- **Bundle size** (< 2 KB gzipped): not covered by automated tests — verified as a CI step via
  `next build` bundle analysis; deferred to a separate size-limit check (e.g. `bundlewatch`).

---

## Pre-Merge Checklist

- [ ] UT-B tests: assert on log output using `bytes.Buffer` writer for tests UT-B-09 through UT-B-14
- [ ] IT-03 E2E test: replace `waitForTimeout` with `expect.poll` or a log-tail test endpoint
- [ ] `useBrowserLogStream.test.ts`: verify `jest.useFakeTimers()` works with `fetch` mock (some environments require `jest.runAllTimers()` after advancing fake timers)
- [ ] Confirm `navigator.sendBeacon` is stubable in jsdom (requires `Object.defineProperty` or `jest.spyOn`)
- [ ] Run `cd web-app && npx jest --no-coverage --testPathPatterns="useBrowserLogStream"` before PR
- [ ] Run `go test ./server/handlers/...` before PR
- [ ] `TerminalOutput` component tests: confirm `useBrowserLogStream` is importable as a mock via `jest.mock('@/lib/hooks/useBrowserLogStream')`
