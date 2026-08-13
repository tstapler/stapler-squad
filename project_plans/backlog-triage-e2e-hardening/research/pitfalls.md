# Pitfalls & Risks: backlog-triage-e2e-hardening

## 1. JSON Extraction Robustness in `ParseHeadlessTriageResult`

### Current state
The function (`session/backlog_triage.go:92-116`) strips only markdown code fences and then calls `json.Unmarshal` on the full trimmed string. Any preamble text before the JSON object causes an immediate parse error.

### Naive `{` / `}` scan is unsafe
Scanning for the first `{` and last `}` has two compounding problems:

1. **Preamble containing `{`** — The LLM prompt explicitly asks for research files with paths like `{a: 1}` or config-like text. Any `{` in preamble before the JSON would shift the start position to a non-JSON location, causing a parse error or, worse, silently extracting the wrong substring.

2. **Naive last-`}`** — JSON nesting means the first `}` after the opening `{` may close an inner object, not the root. Scanning for the *last* `}` is slightly safer for the root object but fails if there is any trailing text that contains `}` (unlikely but possible from the LLM).

### Correct approach: `json.Decoder` scan
Use `strings.Index(text, "{")` to find the first `{`, then pass `strings.NewReader(text[firstBrace:])` to a `json.Decoder`. Call `decoder.Decode(&result)` exactly once. The decoder handles nesting depth correctly and stops at the matching close brace, leaving any trailing content unread. This is the idiomatic Go pattern and is immune to nested `{` counts.

```go
idx := strings.Index(text, "{")
if idx < 0 {
    return HeadlessTriageResult{}, fmt.Errorf("ParseHeadlessTriageResult: no JSON object found")
}
dec := json.NewDecoder(strings.NewReader(text[idx:]))
var result HeadlessTriageResult
if err := dec.Decode(&result); err != nil { ... }
```

### JSON array top-level
The prompt template instructs the LLM to output an object (`{...}`), not an array. Scanning for `[` as a fallback would add complexity with no real benefit; if the LLM outputs an array, that is a prompt compliance failure that should surface as a parse error, not be silently recovered.

---

## 2. React `disabled` Button and Tooltip Pitfalls

### Current pattern in the codebase
The "Mark Ready" button (lines 580-589 in `BacklogItemDetail.tsx`) uses:
```tsx
disabled={actionLoading || item.acCriteria.length === 0}
aria-disabled={item.acCriteria.length === 0}
title={item.acCriteria.length === 0 ? "Add at least one AC criterion first" : undefined}
```

The `aria-disabled` attribute is redundant when `disabled` is also set (the browser propagates the accessible state). The `title` native tooltip **does** render on disabled buttons in Chrome, Firefox, and Safari — browsers bypass the `mouseenter` suppression for the `title` attribute specifically. So the native `title` tooltip approach is functional cross-browser.

### Codebase has a full `Tooltip` component
`web-app/src/components/ui/Tooltip.tsx` wraps Radix UI `@radix-ui/react-tooltip`. It accepts `children`, `label`, and `side`. Radix's `TooltipPrimitive.Trigger` uses `asChild` (line 19), which means it renders the child element directly. **Key pitfall**: Radix `Tooltip.Trigger asChild` on a `disabled` button suppresses pointer events, so the Radix tooltip will NOT appear when hovering a disabled `<button>` — this is a known Radix UI limitation (pointer-events: none on disabled buttons blocks the trigger).

### Recommended approach for `Trigger Triage` button
Use the existing `title` + `disabled` pattern (same as "Mark Ready") for the simplest and most consistent implementation. Do NOT wrap a disabled `<button>` in `<Tooltip>` from the UI component — the Radix trigger will not fire for disabled elements. If a richer tooltip is needed, wrap the disabled button in a `<span>` with `display: inline-block` and attach the Radix `<Tooltip>` to the span, not the button.

---

## 3. E2E Test State Isolation

### The empty-state tests are brittle by design
Three test suites (`Empty State`, `Item Creation and List`, `Status Transitions`) explicitly call `test.fail()` when items already exist in the backlog (`backlog.spec.ts` lines 31, 60, 95, 149, 196). A triage test that creates an item and does NOT clean up will break all of these on subsequent runs.

### Isolation strategies

| Strategy | Risk | Verdict |
|---|---|---|
| Unique timestamp title | Item persists; empty-state tests fail on next run | Not sufficient alone |
| Cleanup at end of test (`afterEach` delete) | Delete RPC must be called; if test fails mid-way, cleanup is skipped | Fragile |
| `test.afterEach` with try/catch cleanup | Safer than cleanup in test body; still leaves state on force-kill | Acceptable |
| Separate test instance (`STAPLER_SQUAD_INSTANCE=triage-e2e`) | Total isolation; requires second server process | Ideal for CI |
| Run triage test last (file ordering or `test.describe.serial`) | Still pollutes state for the next CI run | Not reliable |

**Best approach**: Create the triage test in a `test.describe.serial` block with `test.afterEach` that deletes the created item by calling the `DeleteBacklogItem` RPC (or via a page helper). Additionally, document in the test file header that `STAPLER_SQUAD_INSTANCE=e2e-local` must be a fresh instance when running `Empty State` tests. Use `Date.now()` in the item title for uniqueness within a run.

### Missing headless pool in CI
`TriggerTriage` returns `codes.Unimplemented` when `headlessPool == nil` (no `claude` binary). The e2e test must gracefully handle this. The codebase precedent is `test.fixme(true, '<reason>')` (line 413 in `backlog.spec.ts`) for features not yet accessible via UI, and `test.skip()` for runtime environment conditions (line 247). For the triage test, the right pattern is:

```typescript
const response = await page.waitForResponse(r => r.url().includes('TriggerTriage'));
const body = await response.json();
if (body.code === 'unimplemented') {
  test.skip(true, 'Headless triage pool unavailable in this environment (no claude binary)');
  return;
}
```

This surfaces as a skip (yellow) in Playwright reports rather than a failure, making CI green while accurately reporting coverage gaps.
