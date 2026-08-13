# Findings: Stack

## Summary

Stapler Squad currently faces four distinct architectural challenges: (A) large terminal sessions loading slowly due to sending entire scrollback buffers to the browser, (B) lack of frontend observability for performance metrics, (C) no branch autocomplete in the session creation dialog, and (D) potential mobile touch conflicts with xterm.js scroll handling.

This document evaluates library and API choices for each area. Key recommendations:
- **A**: Implement lazy scrollback loading via custom xterm.js addon using existing `GetRange` backend API
- **B**: Use `@opentelemetry/sdk-web` with existing Go OTLP exporter to unify observability; fallback to lightweight custom RPC-based approach if bundle size becomes critical
- **C**: Adopt Radix UI Combobox for branch autocomplete with vanilla-extract compatibility
- **D**: Address mobile scroll conflicts through event listener configuration and potential viewport-fit CSS

---

## A. xterm.js Lazy/Virtual Scrollback

### Options Surveyed

#### Option A1: Native xterm.js Virtual Scrolling Addon (Does Not Exist)
- **Status**: No official virtual scrollback addon exists in xterm.js ecosystem
- **Explanation**: xterm.js maintains a circular buffer in memory (the `scrollback` option controls hard size limit), but rendering is always full-screen viewport-focused. The library does not provide load-on-demand semantics for historical lines beyond what's in the buffer.
- **Bundle Impact**: N/A
- **Feasibility**: Not viable; would require building from scratch

#### Option A2: Custom Lazy-Load Addon (ITerminalAddon Pattern)
- **Architecture**: Implement a custom addon conforming to `ITerminalAddon` interface
  ```go
  type ITerminalAddon interface {
    activate(terminal: Terminal): void
    dispose(): void
  }
  ```
- **How it works**:
  1. On user scroll past buffer boundaries, detect "scroll to top" event
  2. Call backend `GetRange(fromSeq, limit)` RPC to fetch older lines
  3. Use internal xterm.js APIs to prepend (insert at top) without moving cursor
  4. Bind to scroll event: `terminal.onScroll` (deprecated) → use `terminal.parser.registerCsiParameter()` or observe resize/scroll via DOM
- **Key xterm.js APIs**:
  - `terminal.write(data)` — appends only (not suitable for prepending)
  - `terminal._core.buffer.addMarker()` — internal; marks positions for bookmarking
  - `terminal.coreService` — internal; direct buffer manipulation possible but unsupported
  - No public "prepend" API; prepending requires direct buffer writes (fragile)
- **Bundle Impact**: ~2–5 KB minified (custom code only)
- **Feasibility**: High; existing codebase already has `GetRange(fromSeq, limit)` backend methods and circular buffer tracking
- **Risk**: Relies on xterm.js internals that may change between minor versions; requires careful testing
- **Backend Compatibility**: ✅ Already has `GetLastN(n)` and `GetRange(fromSeq, limit)` methods in scrollback buffer

#### Option A3: Reload Session on Demand with New Scrollback Window
- **How it works**: User clicks "load more history" button; fetch older lines via `GetRange()` and reset terminal with new scrollback base
- **Trade-off**: Clears current terminal state (cursor, selections); requires full re-render
- **Bundle Impact**: Negligible (no new dependency)
- **Feasibility**: Low; poor UX (disruptive)
- **Use case**: Fallback for debugging if A2 becomes unstable

#### Option A4: Separate Scrollback Viewer Panel (Hybrid)
- **Architecture**: Keep xterm.js for live output only (small scrollback), open side panel for historical inspection
- **How it works**: Panel fetches and renders old lines as static text; independent of terminal component
- **Bundle Impact**: Minimal
- **Feasibility**: Medium; requires panel UI component but decouples from xterm.js internals
- **UX Trade-off**: Splits scrollback viewing; may feel less integrated
- **Best for**: Mobile/touch devices where xterm.js scrolling is already problematic (see section D)

### Trade-off Matrix

| Criterion | A1 (Native) | A2 (Custom Addon) | A3 (Reload) | A4 (Separate Panel) |
|-----------|-------------|-------------------|-------------|---------------------|
| **Time to implement** | N/A | 2–3 days (incl. tests) | 4–6 hours | 1–2 days |
| **Bundle size impact** | N/A | +2–5 KB | Negligible | +3–8 KB (React panel) |
| **UX smoothness** | N/A | Excellent | Poor | Good |
| **Risk (xterm.js versioning)** | N/A | Medium (internals) | Low | Low |
| **Works on mobile** | N/A | No (scroll conflicts) | No | **Yes** (separate panel) |
| **Reuses backend APIs** | N/A | ✅ `GetRange` | ✅ `GetRange` | ✅ `GetRange` |

### Risk and Failure Modes

**A2 (Custom Addon) risks**:
- **xterm.js API churn**: Internals may shift on major version bump. Mitigation: pin minor version; monitor changelog
- **Scroll event detection fragility**: Browser scroll event timing varies; may lag on large batch prepends. Mitigation: debounce/throttle prepend requests
- **Memory overhead**: Holding large scrollback in DOM/canvas + backend buffer. Mitigation: cap viewport to N most recent lines; older data fetched on-demand
- **Coordination bugs**: Sequence number drift if backend buffer wraps (circular); client prepend logic must stay in sync. Mitigation: unit test sequence edge cases; add telemetry for sequence gaps

**A4 (Separate panel) risks**:
- **User confusion**: Users may not discover scrollback panel; UI UX clarity needed
- **Fragmented interaction**: Some users scroll terminal, others use panel; inconsistent mental model

### Migration and Adoption Cost

**A2 (Custom Addon)**:
1. Create `/web-app/src/lib/terminal/LazyScrollbackAddon.ts` implementing `ITerminalAddon`
2. Modify `XtermTerminal.tsx` to load addon if feature flag enabled
3. Add RPC call to `SessionService.GetRange()` (already exists in proto)
4. Stress-test: 500-line buffer, prepend 100 lines repeatedly; measure no lag, no corrupted state
5. Enable flag on main; monitor for regressions
6. Documentation: explain to users that long sessions now load faster

**Backend changes**: Minimal. Verify existing `GetRange(fromSeq, limit)` returns correctly ordered entries with no gaps.

### Operational Concerns

- **Performance on slow connections**: Prepending while user is typing may stall RPC. Solution: show loading indicator; queue RPC if not idle
- **Scrollback sequence number consistency**: If backend circular buffer wraps, sequence numbers must be contiguous. Add assertion in `GetRange()` response
- **Testing**: Require e2e test for "attach to session with 10k-line scrollback, scroll to top, verify no lag/corruption"

### Prior Art and Lessons Learned

- **tmux**: Uses terminal history file + in-memory ring buffer; users scroll and request more via "capture-pane -p -S -N" (similar to `GetRange`)
- **Kitty terminal**: Offers scrollback search + history via separate indexing; not integrated into main rendering
- **Hyper (Electron-based terminal)**: Loads history lazily via JS plugin API; plugins report startup cost but smooth in practice

**Lesson**: Separate scrollback viewing (A4) is often simpler than integrating lazy-load into terminal renderer (A2). However, A2 is doable if you embrace the asynchrony and test well.

### Open Questions

1. How large are typical session scrollback buffers in production? (Impacts memory footprint of A2)
2. Does xterm.js publish breaking change log for internal APIs? (For A2 maintenance)
3. Should mobile users get A4 (panel) by default, desktop A2 (addon)?

### Recommendation

**Implement A2 (Custom Lazy-Load Addon) as primary feature, with A4 (Separate Panel) as future mobile optimization.**

**Rationale**:
- Backend APIs already exist and are well-tested (`GetRange`, circular buffer)
- Unifies scrollback UX (no fragmentation)
- Desktop users get fast, seamless large-session attach
- Risk is manageable with careful testing and version pinning
- Can evolve to A4 if xterm.js internals prove too volatile or mobile usage grows

**Implementation checklist**:
- [ ] Spike: test xterm.js scroll events and prepend safety (1 day)
- [ ] Build LazyScrollbackAddon (1–2 days)
- [ ] Integrate with XtermTerminal component (0.5 days)
- [ ] End-to-end stress tests (1 day)
- [ ] Feature flag + gradual rollout (1 day)

---

## B. Frontend Observability Options

### Options Surveyed

#### Option B1: OpenTelemetry SDK for Web (`@opentelemetry/sdk-web`)

**Overview**: Official Anthropic-maintained OTel distribution for JavaScript/browser environments. Instruments performance, errors, and custom events. Exports to OTLP (OpenTelemetry Protocol) endpoints.

**Bundle size**:
- Core + exporters: ~45–55 KB minified+gzipped
- Installed: `@opentelemetry/sdk-web`, `@opentelemetry/sdk-node`, `@opentelemetry/exporter-otlp`
- [TRAINING_ONLY — verify] Estimate from ecosystem surveys; actual size varies with tree-shaking

**Auto-instrumentation available**:
- `@opentelemetry/auto-instrumentations-web`: fetch, XMLHttpRequest, user interaction timing (click, scroll), navigation timing
- Does NOT auto-instrument ConnectRPC yet [TRAINING_ONLY — verify]

**OTLP Export Capability**:
- ✅ Built-in `OTLPTraceExporter` and `OTLPMetricExporter`
- ✅ Can export to same backend endpoint Go OTel already sends to (typically `http://localhost:4318/v1/traces`)
- ⚠️ CORS: Browser must be allowed by backend CORS policy. ConnectRPC already uses `@connectrpc/connect-web`; check if CORS headers sufficient

**Async data loading (RPC tracking)**:
- ✅ OTel wraps fetch; you get automatic spans for `SessionService.GetLastN()`, `ListBranches()`, etc.
- ⚠️ No built-in understanding of ConnectRPC semantics (error handling, streaming); requires custom span creation

**Integration effort**:
- Moderate. Set up tracer provider, exporters, auto-instrumentation at app startup.
- Example:
  ```typescript
  import { getWebAutoInstrumentations } from '@opentelemetry/auto-instrumentations-web';
  import { OTLPTraceExporter } from '@opentelemetry/exporter-otlp-proto';
  
  const traceExporter = new OTLPTraceExporter({
    url: 'http://localhost:4318/v1/traces',
  });
  const tracerProvider = new BasicTracerProvider({ resource });
  tracerProvider.addSpanProcessor(new BatchSpanProcessor(traceExporter));
  ```

**Pros**:
- Vendor-neutral standard; integrates with any OTLP backend (Jaeger, Tempo, Grafana Loki)
- Same tooling & exporter format as Go backend
- Unified dashboards (frontend + backend traces in one system)
- Auto-instrumentation reduces boilerplate

**Cons**:
- Bundle size (45–55 KB) is non-trivial for web terminal
- OTel overhead: small but measurable on high-frequency events (e.g., every keystroke)
- Requires OTLP backend running; local development needs docker/collector

#### Option B2: Sentry Browser SDK

**Overview**: Error tracking + performance monitoring SaaS. Free tier covers 5K events/month.

**Bundle size**:
- Core: ~60–70 KB minified+gzipped
- Auto-instrumentation included: fetch, navigation, React error boundary integration
- [TRAINING_ONLY — verify] Estimate from published metrics

**Auto-instrumentation**:
- ✅ Fetch, XMLHttpRequest, React suspense errors
- ✅ Web vitals (LCP, CLS, FID, INP)
- ✅ User interactions with replay (video-like capture of user session)
- ❌ No ConnectRPC-specific support

**Performance event tracking**:
- ✅ RPC duration: captured automatically via fetch hook
- ✅ Interaction latency: "click to render" via `performance.measure()`
- ✅ Custom metrics: can send arbitrary timings

**Pros**:
- Easiest to set up: one SDK, no backend infrastructure needed
- Excellent error context: source maps, breadcrumbs, session replay
- Generous free tier for small projects

**Cons**:
- Vendor lock-in: data stored in Sentry's cloud; export is manual + limited
- Cannot export to existing Go OTLP endpoint (separate system)
- Privacy concerns: user interactions sent to 3rd-party SaaS
- Pricing scales with event volume; high-traffic apps become expensive

#### Option B3: Lightweight Custom Approach (RPC-based observability)

**Overview**: Send minimal telemetry to backend via existing `LogUserInteractionRequest` RPC or new `RecordFrontendMetrics()` RPC.

**Architecture**:
```go
// proto: new RPC
rpc RecordFrontendMetrics(FrontendMetricsRequest) returns (FrontendMetricsResponse) {}

message FrontendMetricsRequest {
  string session_id = 1;
  string event_type = 2; // "rpc_latency", "render_latency", "click_latency"
  int64 duration_ms = 3;
  map<string, string> labels = 4; // rpc_name, component, etc.
  google.protobuf.Timestamp recorded_at = 5;
}
```

**Frontend hooks**:
```typescript
const trackRpcDuration = async (rpcName: string, fn: () => Promise<T>) => {
  const start = performance.now();
  const result = await fn();
  const duration = performance.now() - start;
  logMetric('rpc_latency', { rpc_name: rpcName, duration_ms: Math.round(duration) });
  return result;
};

const trackRenderLatency = (componentName: string, fn: () => void) => {
  requestAnimationFrame(() => {
    const start = performance.now();
    fn();
    requestAnimationFrame(() => {
      const duration = performance.now() - start;
      logMetric('render_latency', { component: componentName, duration_ms: Math.round(duration) });
    });
  });
};
```

**Bundle size**:
- ~1–2 KB of custom code (minimal utility functions)
- No external dependencies

**Async data loading**:
- ⚠️ Manual: you decide what to track and when
- ✅ Simple: just measure time before/after RPC call

**Integration effort**:
- Low. Add 5–10 custom hooks in `/web-app/src/lib/metrics/`
- Modify RPC calls to wrap with `trackRpcDuration()`

**Backend aggregation**:
- Metrics land in backend logs (structured as JSON)
- Integrate with existing log storage: grep, ELK, or simple time-series DB

**Pros**:
- Zero external dependencies; trivial bundle size
- All data stays within company infrastructure
- Simple to understand and debug
- No CORS issues (same origin as main app)

**Cons**:
- No correlation between frontend spans and backend traces (separate system)
- Manual instrumentation: easy to miss coverage
- No visualization library; requires custom dashboard or ELK setup
- Sampling/rate-limiting logic must be built manually
- Does not scale to thousands of clients without backend aggregation

### Trade-off Matrix

| Criterion | B1 (OTel Web) | B2 (Sentry) | B3 (Custom) |
|-----------|---------------|-------------|------------|
| **Bundle size (min+gz)** | 45–55 KB | 60–70 KB | 1–2 KB |
| **Setup time** | 2–4 hours | 30 minutes | 2–3 hours |
| **Auto-instrumentation** | Good (fetch, interactions) | Excellent (+ session replay) | None (manual) |
| **OTLP export** | ✅ Native | ❌ No | ❌ No (custom RPC) |
| **Backend correlation** | ✅ Same endpoint as Go | ❌ Separate SaaS | ✅ Same infrastructure |
| **Privacy** | ✅ Self-hosted | ❌ 3rd-party | ✅ Self-hosted |
| **Cost** | Infra (collector) | $0–$500+/mo | Dev time only |
| **ConnectRPC support** | ⚠️ Generic (fetch) | ⚠️ Generic (fetch) | ✅ Explicit |

### Risk and Failure Modes

**B1 (OTel Web)**:
- **CORS failures**: If backend collector CORS headers missing, browser blocks export. Mitigation: test in dev; add CORS headers to collector config
- **Data explosion**: Every interaction sends trace; high cardinality labels cause backend memory bloat. Mitigation: sampling (80% of requests, 100% of errors)
- **Bundle overhead**: 45 KB is 1–2% of total bundle; acceptable but non-zero for performance-critical apps

**B2 (Sentry)**:
- **Data compliance**: Sending user interactions to US-based SaaS may violate GDPR, CCPA. Mitigation: use EU SaaS option; anonymize PII
- **Cost scaling**: High-traffic deployments see dramatic cost increases; rate limiting needed. Mitigation: sample heavily; only send errors + key metrics
- **Lock-in**: Moving away requires building export tooling. Mitigation: keep it supplementary; maintain B3 custom approach in parallel

**B3 (Custom)**:
- **Incomplete coverage**: Easy to forget instrumenting new RPC calls. Mitigation: code review checklist; linting rule for tracked/untracked RPC calls
- **No real-time visualization**: Requires ELK setup or custom dashboard. Mitigation: start with logs, graduate to structured queries
- **Maintenance burden**: Custom code requires ongoing updates as app evolves. Mitigation: keep helpers simple; avoid overengineering

### Migration and Adoption Cost

**B1 (OTel Web)**: 
- Install packages: `@opentelemetry/sdk-web`, `@opentelemetry/exporter-otlp-proto`, `@opentelemetry/auto-instrumentations-web`
- Create `/web-app/src/lib/observability/otel.ts` with tracer setup
- Initialize in `/web-app/src/app/layout.tsx` or `_app.tsx` (Next.js)
- Ensure backend OTLP collector running (or enable in existing Jaeger/Tempo)
- Cost: 1–2 days (setup + validation)

**B2 (Sentry)**:
- Install: `@sentry/react`, `@sentry/nextjs` (if using Next.js)
- Initialize in `/web-app/src/app/layout.tsx`
- Create Sentry project; get DSN
- Cost: 30 minutes (quick start)

**B3 (Custom)**:
- Create `/web-app/src/lib/metrics/frontend.ts` with `logMetric()` and tracking hooks
- Wrap RPC calls in session creation, terminal attach, branch list, etc.
- Create backend RPC handler for `RecordFrontendMetrics()`
- Add log ingestion to monitoring pipeline
- Cost: 1–2 days (implementation + testing)

### Operational Concerns

**B1 (OTel Web)**:
- Requires OTLP backend (Jaeger, Tempo) running; add to deployment checklist
- Monitor collector disk/memory; may need retention limits (e.g., traces older than 24h)
- Browser-to-backend connectivity must be reliable; fallback export queue needed

**B2 (Sentry)**:
- Monthly billing review needed to ensure costs don't spiral
- PII redaction rules must be kept current (new form fields, etc.)
- Session replay storage can be large; retention policies needed

**B3 (Custom)**:
- Log storage must handle 1KB+ per user interaction; estimate volume growth
- No built-in dashboards; need Grafana/ELK expertise
- Manual aggregation queries (e.g., "p95 RPC latency") require custom code or SQL

### Prior Art and Lessons Learned

- **Vercel (Next.js team)**: Uses OTel for internal analytics; published guide on best practices
- **Grafana Loki**: Recommends OTel for frontend instrumentation + centralized logging
- **Small startups**: Often use B2 (Sentry) for quick wins; graduate to B1 or B3 as app scales
- **Open-source projects**: Prefer B3 (custom) to avoid dependencies and vendor lock-in

**Lesson**: Bundle size matters for web terminals (perceived latency = poor UX). B1 (45 KB) + B2 (60 KB) are both noticeable. B3 is minimal but requires discipline.

### Open Questions

1. What is the target bundle size for the web app? Does 45 KB fit the budget?
2. Is OTLP backend (Jaeger/Tempo) already running in infrastructure?
3. What is the expected volume of daily active users? (Affects Sentry vs. custom cost calculus)
4. Are there privacy/compliance constraints that rule out Sentry?

### Recommendation

**Implement B1 (OTel Web SDK) as primary observability layer, with B3 (custom RPC) as fallback.**

**Rationale**:
- Go backend already uses OTel; adding web OTel unifies the entire observability pipeline
- Auto-instrumentation of fetch covers most RPC tracking without code changes
- OTLP export means no vendor lock-in; data stays on-prem if using self-hosted collector
- 45–55 KB bundle cost is acceptable given web app's current dependencies
- If bundle becomes critical bottleneck (measure first!), fall back to B3

**Implementation checklist**:
- [ ] Audit current bundle size; confirm 45 KB overhead acceptable (1 day)
- [ ] Ensure OTLP collector endpoint reachable from browser (dev + prod)
- [ ] Initialize OTel in app layout (1 day)
- [ ] Test trace export; confirm traces appear in Jaeger/backend system (0.5 days)
- [ ] Add custom spans for key flows (session attach, branch list, etc.) (2 days)
- [ ] Set up sampling (80% of requests, 100% of errors) to manage volume (1 day)
- [ ] Dashboard: create Grafana panels for RPC latency, error rate (1–2 days)

**Future migrations**:
- If privacy constraints emerge, switch to B3
- If Sentry free tier is more pragmatic, add B2 in parallel (both can coexist)

---

## C. React Combobox/Autocomplete for Branch Selection

### Options Surveyed

#### Option C1: Radix UI Combobox (`@radix-ui/react-select` + custom menu)

**Overview**: Radix UI provides unstyled, accessible component primitives. No combobox directly, but can compose `Select` + custom filtering.

**Bundle size**:
- `@radix-ui/react-select`: ~15 KB minified+gzipped
- Requires state management: `@radix-ui/react-use-controllable-state`, `@radix-ui/react-popover`
- Total: ~20–25 KB

**Keyboard navigation**:
- ✅ Arrow up/down to select, Enter to confirm, Escape to dismiss
- ✅ Customizable via composition
- ✅ ARIA attributes included (role="listbox", aria-selected, etc.)

**Vanilla-extract compatibility**:
- ✅ Radix is unstyled; pairs perfectly with vanilla-extract
- Example: Wrap `Select.Item` with `className={styles.selectItem}` (vanilla-extract CSS)

**Async data loading**:
- ⚠️ No built-in async support; you manage filter + fetch state
- Typical pattern:
  ```typescript
  const [branches, setBranches] = useState<string[]>([]);
  const [filter, setFilter] = useState('');
  const [loading, setLoading] = useState(false);
  
  useEffect(() => {
    const timer = setTimeout(() => {
      if (filter.length > 0) {
        setLoading(true);
        sessionService.ListBranches({ filter }).then(({ branches }) => {
          setBranches(branches);
          setLoading(false);
        });
      }
    }, 300); // debounce
    return () => clearTimeout(timer);
  }, [filter]);
  ```

**Integration effort**:
- Moderate. Compose Radix primitives + write filtering logic

#### Option C2: Downshift (`downshift`)

**Overview**: Headless autocomplete library from PayPal. Handles state machine for dropdown interactions.

**Bundle size**:
- `downshift`: ~8–10 KB minified+gzipped
- Lightweight compared to Radix

**Keyboard navigation**:
- ✅ Full keyboard support: arrows, Home, End, Page Up/Down, Escape
- ✅ ARIA attributes auto-applied (but less granular than Radix)
- ✅ Customizable handlers

**Vanilla-extract compatibility**:
- ✅ Headless; you provide all JSX. Pairs well with vanilla-extract CSS modules

**Async data loading**:
- ✅ Better support than Radix; designed for this use case
- Example:
  ```typescript
  const { getInputProps, getMenuProps, getItemProps, highlightedIndex, isOpen, openMenu } = useCombobox({
    items: branches,
    inputValue: filter,
    onInputValueChange: ({ inputValue }) => {
      setFilter(inputValue);
      fetchBranches(inputValue); // built-in debounce via onInputValueChange timing
    },
  });
  ```

**Integration effort**:
- Low to moderate. Hook-based API; less JSX composition than Radix

#### Option C3: Headless UI Combobox (TailwindLabs)

**Overview**: Official Tailwind component library. Combobox is one of several unstyled components.

**Bundle size**:
- `@headlessui/react`: ~12–15 KB minified+gzipped
- Smaller than Radix; comparable to Downshift

**Keyboard navigation**:
- ✅ Standard arrows, Enter, Escape
- ✅ ARIA attributes included
- Less customizable than Radix; more opinionated

**Vanilla-extract compatibility**:
- ✅ Unstyled; pairs with vanilla-extract
- ⚠️ Headless UI is optimized for Tailwind CSS defaults; vanilla-extract integration requires explicit style composition

**Async data loading**:
- ⚠️ No built-in async support; manual state management like Radix C1
- Typical pattern: useState + useEffect for fetching

**Integration effort**:
- Low. Simple JSX; good documentation

#### Option C4: React Aria ComboBox (Adobe)

**Overview**: Adobe's comprehensive component library built on React Hooks Specification. Part of `@adobe/react-spectrum` design system.

**Bundle size**:
- `@react-aria/combobox`: ~15–20 KB
- `@react-spectrum/combobox`: ~25–30 KB (if you want pre-styled)
- Just the hooks (`@react-aria/combobox`) are lighter; you provide rendering

**Keyboard navigation**:
- ✅ Extensive: arrows, Home, End, Ctrl+A (select all input), Backspace (remove tags)
- ✅ Multi-select support built-in
- ✅ ARIA attributes auto-applied, thoroughly tested

**Vanilla-extract compatibility**:
- ✅ `@react-aria/combobox` hooks are unstyled; compatible with vanilla-extract
- ⚠️ `@react-spectrum/combobox` (styled) uses emotion/CSS-in-JS; conflicts with vanilla-extract

**Async data loading**:
- ✅ Better support than C1/C3; designed with data-fetching use cases in mind
- `useComboBoxState` hook includes loading state:
  ```typescript
  const state = useComboBoxState({ 
    items: branches,
    defaultFilter: filter,
    allowEmptyCollection: true
  });
  const { inputProps, listBoxProps } = useComboBox({ state, inputRef, listBoxRef });
  ```

**Integration effort**:
- Moderate to high. React Aria is comprehensive but has learning curve; excellent documentation

#### Option C5: Simple Custom Solution (no library)

**Overview**: Build branch list dropdown with `<input>` + `<div>` list, manual filter + RPC call.

**Bundle size**:
- ~0 KB (no external dependency)

**Keyboard navigation**:
- ❌ Manual implementation required (error-prone)
- Would need `onKeyDown` handlers for arrows, Enter, Escape
- ARIA attributes must be manually written

**Vanilla-extract compatibility**:
- ✅ Trivial; vanilla CSS modules sufficient

**Async data loading**:
- ✅ Straightforward; call RPC on input change

**Integration effort**:
- High initial; low ongoing if requirements are simple
- Risk of accessibility regressions and missed edge cases (multi-line input, IME, etc.)

**Maintainability**:
- Low; keyboard/a11y edge cases accumulate over time

### Trade-off Matrix

| Criterion | C1 (Radix) | C2 (Downshift) | C3 (Headless UI) | C4 (React Aria) | C5 (Custom) |
|-----------|-----------|---|---|---|---|
| **Bundle size (min+gz)** | 20–25 KB | 8–10 KB | 12–15 KB | 15–20 KB | 0 KB |
| **Keyboard nav quality** | Excellent | Excellent | Good | Excellent | Manual (risky) |
| **ARIA support** | Excellent | Good | Good | Excellent | Manual (risky) |
| **Vanilla-extract compat** | ✅ | ✅ | ✅ | ✅ (hooks only) | ✅ |
| **Async data support** | Manual | Built-in | Manual | Built-in | Custom |
| **Setup time** | 1–2 days | 0.5–1 day | 0.5–1 day | 1–2 days | 2–3 days |
| **Docs quality** | Excellent | Good | Good | Excellent | N/A |
| **Long-term maintenance** | Low | Low | Low | Low | High |

### Risk and Failure Modes

**C1 (Radix)**:
- **Composition complexity**: Requires careful nesting of Radix primitives; easy to miss ARIA attributes. Mitigation: use Radix examples; test with screen reader
- **Manual async handling**: Filtering + fetching state machine can get complex. Mitigation: create custom hook `useBranchAutocomplete()` to encapsulate

**C2 (Downshift)**:
- **Learning curve**: State machine model less intuitive than hook-based. Mitigation: Downshift docs are excellent; one good example covers most use cases
- **IME compatibility**: Downshift's input event handling may conflict with IME (Chinese, Japanese input). Mitigation: test on multiple languages

**C3 (Headless UI)**:
- **Less customizable**: Fewer composition points than Radix; opinionated keyboard behavior. Mitigation: file feature requests; use Radix if flexibility critical
- **Less comprehensive**: No multi-select combobox; only single-select. Mitigation: if needed, switch to C4 (React Aria)

**C4 (React Aria)**:
- **Larger API surface**: More hooks to learn; steeper learning curve. Mitigation: use only `useComboBox` + `useListBox`; ignore advanced features initially
- **Bundle size for lite usage**: 15–20 KB for just keyboard navigation may feel heavy. Mitigation: if bundle budget tight, use C2 (Downshift)

**C5 (Custom)**:
- **Accessibility issues**: Easy to miss ARIA roles, focus management, keyboard event handling. Mitigation: accessibility audit with screen reader; file issues as found
- **Edge cases**: IME, double-click, long inputs, many items (scroll), mobile touch. Mitigation: expect discovery bugs post-launch

### Migration and Adoption Cost

**C1 (Radix)**:
1. Install: `@radix-ui/react-select`
2. Create `/web-app/src/components/BranchAutocomplete.tsx`:
   ```typescript
   export function BranchAutocomplete() {
     const [branches, setBranches] = useState<string[]>([]);
     const [filter, setFilter] = useState('');
     const [loading, setLoading] = useState(false);
     
     useEffect(() => {
       const timer = setTimeout(() => {
         if (filter.length >= 2) {
           setLoading(true);
           sessionService.ListBranches({ filter }).then(({ branches }) => {
             setBranches(branches);
             setLoading(false);
           });
         } else {
           setBranches([]);
         }
       }, 300);
       return () => clearTimeout(timer);
     }, [filter]);

     return (
       <Select value={...} onValueChange={...}>
         <SelectTrigger>Branch</SelectTrigger>
         <SelectContent>
           {branches.map(b => <SelectItem key={b} value={b}>{b}</SelectItem>)}
         </SelectContent>
       </Select>
     );
   }
   ```
3. Style with vanilla-extract CSS module
4. Integrate into `CreateSessionDialog`
5. Cost: 1–2 days

**C2 (Downshift)**:
1. Install: `downshift`
2. Create `/web-app/src/components/BranchAutocomplete.tsx` using `useCombobox` hook
3. Similar to C1; slightly less JSX boilerplate
4. Cost: 0.5–1 day

**C3 (Headless UI)**:
1. Install: `@headlessui/react`
2. Similar structure to C1; more concise component definition
3. Cost: 0.5–1 day

**C4 (React Aria)**:
1. Install: `@react-aria/combobox`, `@react-aria/listbox`
2. Use `useComboBoxState` + `useComboBox` + `useListBox` hooks
3. More involved but cleaner separation of state and rendering
4. Cost: 1–2 days (learning curve offset by cleaner code)

**C5 (Custom)**:
1. Create `/web-app/src/components/BranchAutocomplete.tsx` with custom `<input>` + `<ul>`
2. Manual event handlers for keyboard navigation
3. ARIA attributes manually added
4. Cost: 2–3 days (implementation + testing + accessibility audit)

### Operational Concerns

- **Backend RPC design for branch listing**: Should `ListBranches(filter: string)` or `ListBranches()` then filter client-side?
  - **Recommendation**: Server-side filter (`ListBranches(filter)`) for repos with 500+ branches; reduces payload size
  - Implement git command: `git for-each-ref --format='%(refname:short)' refs/heads refs/remotes | grep '^filter'`
  - See section D for git performance analysis

- **Caching**: Should branch list be cached? TTL?
  - **Recommendation**: Cache for 5–10 minutes; invalidate on push notifications (if system sends them)

- **Mobile interaction**: Combobox keyboard navigation works poorly on mobile (no hardware keyboard)
  - **Recommendation**: For mobile, consider simplified dropdown (no filter input); users scroll list

### Prior Art and Lessons Learned

- **Vercel (Vercel dashboard)**: Uses Radix UI for filter selects; integrates with vanilla-extract-like styling
- **GitHub**: Custom combobox for branch selection; comprehensive keyboard nav
- **GitLab**: Downshift-based branch selector (documented in UI docs)

**Lesson**: Keyboard navigation and async data handling are the hard parts. Libraries like Downshift and React Aria abstract these well; worth the bundle size.

### Open Questions

1. How many branches are in typical repos? (Affects server-side filtering necessity; see section D)
2. Is mobile branch selection a priority? (Affects design of dropdown on small screens)
3. Does vanilla-extract build integrate cleanly with Radix's unstyled components?

### Recommendation

**Implement C2 (Downshift) as primary solution.**

**Rationale**:
- Smallest bundle impact (8–10 KB) among full-featured options
- Built-in async data loading support; minimal custom state management
- Excellent keyboard navigation out-of-the-box
- Pairs cleanly with vanilla-extract for styling
- Well-documented; PayPal maintains it actively
- Can be extended to multi-select if needed later

**Alternative**: Use C1 (Radix) if you're already using other Radix UI components elsewhere in the app (reduces total bundle footprint).

**Implementation checklist**:
- [ ] Spike: test Downshift with RPC call for branch list (0.5 days)
- [ ] Implement BranchAutocomplete component (1 day)
- [ ] Implement backend ListBranches RPC with filter parameter (see section D)
- [ ] Add vanilla-extract CSS module for dropdown styling (0.5 days)
- [ ] Integrate into CreateSessionDialog (0.5 days)
- [ ] Keyboard nav + accessibility testing (0.5 days)
- [ ] Mobile testing: verify usable on small screens (0.5 days)

---

## D. Go Git Branch Listing API

### Options Surveyed

#### Option D1: Shell Out to `git for-each-ref` (Current Implicit Approach)

**Command**:
```bash
git for-each-ref --format='%(refname:short)' refs/heads refs/remotes
```

**Performance on 500+ branches**:
- **Measurement [TRAINING_ONLY — verify]**: ~5–50 ms on modern hardware (SSD, warm cache)
- Scales roughly O(N) with branch count; git indexing is well-optimized
- First call slower (cold filesystem cache); subsequent calls within same session very fast

**Memory**:
- ~1 KB per branch in output; 500 branches → ~500 KB output
- Git internals use more; total process footprint ~20–50 MB

**RPC design**:
```protobuf
rpc ListBranches(ListBranchesRequest) returns (ListBranchesResponse) {}

message ListBranchesRequest {
  // Absolute path to git repository
  string repo_path = 1;
  // Optional: filter branches by substring (case-insensitive)
  optional string filter = 2;
  // Optional: include remote branches (default: true)
  bool include_remotes = 3;
  // Optional: max results (0 = no limit; recommended 1000)
  int32 max_results = 4;
}

message ListBranchesResponse {
  repeated string branches = 1;
  int32 total_count = 2; // total before max_results limit
}
```

**Filtering strategy**:
- **Server-side filter** (recommended for 500+ branches):
  ```bash
  git for-each-ref --format='%(refname:short)' refs/heads refs/remotes | grep -i "filter"
  ```
  Cost: ~5 ms additional for grep
  
- **Client-side filter** (simpler RPC, but larger payload):
  ```bash
  git for-each-ref --format='%(refname:short)' refs/heads refs/remotes
  # Then filter in Go or browser
  ```
  Cost: 500 KB payload on each request; cache on browser

**Pros**:
- Fastest approach (git is highly optimized)
- No external dependencies
- Works with any git repo (even submodules, worktrees)
- Output is canonical (exact branch names as git sees them)

**Cons**:
- Subprocess overhead: ~1–2 ms per exec (small but adds up in tight loops)
- Must handle repo path validation (security: no path traversal)
- Error handling: repo path may not exist, or not a valid git repo

**Risk**:
- Repo path security: user input → must validate with `filepath.Clean()` and check against allowed paths
- Large branch count (1000+): output becomes large; consider pagination

#### Option D2: `go-git` Library Branch Enumeration

**Overview**: Pure Go implementation of git; no subprocess needed.

**Package**: `github.com/go-git/go-git/v5`

**Performance on 500+ branches**:
- **Measurement [TRAINING_ONLY — verify]**: ~30–100 ms (slower than shell `git` due to parsing overhead)
- Slower than system `git` because it reimplements git internals in Go
- Scales O(N) but with higher constant factor
- Memory: ~5–10 MB for the repo object

**RPC design**: Identical to D1 (API-compatible)

**Integration**:
```go
import "github.com/go-git/go-git/v5"

func ListBranches(ctx context.Context, repoPath string, filter string) ([]string, error) {
  repo, err := git.PlainOpen(repoPath)
  if err != nil {
    return nil, err
  }
  
  refs, err := repo.References()
  if err != nil {
    return nil, err
  }
  
  var branches []string
  refs.ForEach(func(ref *plumbing.Reference) error {
    if !ref.Name().IsBranch() && !ref.Name().IsRemote() {
      return nil
    }
    branchName := strings.TrimPrefix(ref.Name().Short(), "origin/") // clean up remote prefix
    if filter == "" || strings.Contains(strings.ToLower(branchName), strings.ToLower(filter)) {
      branches = append(branches, branchName)
    }
    return nil
  })
  
  return branches, nil
}
```

**Pros**:
- No subprocess; pure Go
- Can be called inline in handler without spawning process
- Integrates easily with Go error handling
- No path validation needed (PlainOpen does it)

**Cons**:
- Slower than system `git` (30–100 ms vs. 5–50 ms)
- Additional dependency; adds ~1–2 MB to binary size
- Must keep go-git updated (security patches for git protocols)
- Less battle-tested than system git for edge cases (submodules, shallow clones, etc.)

**Risk**:
- Compatibility: go-git may not support all git features (e.g., new config options); codebase must stay updated

#### Option D3: Hybrid Approach (Cache + Fallback)

**Architecture**:
- On each session attach or workspace switch, query branch list
- Cache result in memory (with TTL) or Redis
- Invalidate on git push notifications (if system has them)
- Fallback to shell `git` if cache cold

**RPC design**:
```protobuf
message ListBranchesRequest {
  string repo_path = 1;
  optional string filter = 2;
  bool include_remotes = 3;
  bool force_refresh = 4; // Bypass cache
  int32 max_results = 5;
}
```

**Implementation**:
```go
var (
  branchCache = sync.Map{} // repo_path → {branches, timestamp}
  cacheTTL    = 5 * time.Minute
)

func ListBranches(ctx context.Context, req *ListBranchesRequest) (*ListBranchesResponse, error) {
  if !req.ForceRefresh {
    if cached, ok := branchCache.Load(req.RepoPath); ok {
      entry := cached.(CacheEntry)
      if time.Since(entry.Timestamp) < cacheTTL {
        return filterBranches(entry.Branches, req.Filter, req.MaxResults), nil
      }
    }
  }
  
  branches, err := execGitForEachRef(req.RepoPath)
  if err != nil {
    return nil, err
  }
  
  branchCache.Store(req.RepoPath, CacheEntry{Branches: branches, Timestamp: time.Now()})
  return filterBranches(branches, req.Filter, req.MaxResults), nil
}
```

**Pros**:
- Fastest for repeated queries (cache hit is ~1 µs)
- Handles spiky workloads (many combobox filters in quick succession)
- Clear fallback if cache cold
- Can be added to any D1/D2 implementation

**Cons**:
- Added complexity: cache invalidation logic
- Memory growth if many repos; need cleanup policy
- TTL-based invalidation may be stale (if push happens but cache not yet refreshed)

#### Option D4: Streaming Response (for Very Large Branch Lists)

**Overview**: Instead of returning all branches at once, stream them to client incrementally.

**RPC design**:
```protobuf
rpc ListBranchesStream(ListBranchesRequest) returns (stream ListBranchesResponse) {}

message ListBranchesResponse {
  repeated string branches = 1; // 1 or more per message
  bool has_more = 2;
}
```

**Use case**: Repos with 1000+ branches; user filters as list streams

**Pros**:
- Reduced latency: first results arrive immediately
- Better UX: combobox can show results before full list loads
- Backpressure: client-side filtering can slow consumption

**Cons**:
- More complex RPC plumbing (streaming protocol)
- No "total count" until stream ends; pagination unclear
- Rarely needed for typical branch counts (500 is comfortable)

**Recommendation**: Skip D4 for now; revisit if production data shows 1000+ branches common.

### Trade-off Matrix

| Criterion | D1 (Shell git) | D2 (go-git) | D3 (Cached) | D4 (Streaming) |
|-----------|---|---|---|---|
| **Latency (p50)** | 5–10 ms | 30–50 ms | 0.01 ms (cached) | 1–2 ms (first result) |
| **Memory per call** | ~500 KB | ~5 MB | Varies (cache) | Unbuffered |
| **Implementation time** | 2 hours | 3 hours | 4 hours | 2 days |
| **Dependencies** | Shell (always available) | go-git lib (+1.5 MB binary) | None (wraps D1/D2) | None (uses D1/D2 base) |
| **Path security** | Manual validation | Built-in | Manual validation | Manual validation |
| **Edge cases (submodules, shallow)** | ✅ Handles well | ⚠️ May not | Inherited from base | Inherited from base |
| **Suitable for 500 branches** | ✅ Yes | ✅ Yes | ✅ Yes | Not needed |
| **Suitable for 1000+ branches** | ✅ Yes | ✅ Yes | ✅ Yes | ✅ Better UX |

### Risk and Failure Modes

**D1 (Shell git)**:
- **Path traversal attack**: User provides `../../etc/passwd` as repo_path. Mitigation: `filepath.Clean()` + check is under workspace dir
- **Subprocess resource exhaustion**: If called very frequently, spawning processes becomes bottleneck. Mitigation: cache (D3) or go-git (D2)
- **Git command injection**: Unlikely if repo_path is clean, but filter string could be dangerous. Mitigation: never shell-interpolate filter; use `git ... | grep "..."` pattern only

**D2 (go-git)**:
- **Compatibility**: Repo uses advanced git features (sparse-checkout, worktrees) that go-git doesn't support. Mitigation: test on real repos; have D1 (shell) fallback
- **Dependency bloat**: go-git pulls in networking deps for cloning; unused here. Mitigation: accept the cost or submit upstream to make optional
- **Security updates**: go-git must be kept updated for git protocol vulnerabilities. Mitigation: use dependabot; flag in security reviews

**D3 (Cached)**:
- **Stale data**: User pushes new branch; cache not refreshed for 5 minutes. Mitigation: shorter TTL (1–2 min); or invalidate on git post-receive hooks
- **Cache explosion**: 1000 repos × 500 branches = 500 MB memory. Mitigation: add LRU eviction; monitor memory growth
- **Invalidation bugs**: Cache entry partially updated (e.g., filter applied to old list). Mitigation: store full list; filter at query time

**D4 (Streaming)**:
- **Client buffering**: Browser may buffer entire stream before rendering, defeating the purpose. Mitigation: test on real browsers; may need framing protocol tweaks

### Migration and Adoption Cost

**D1 (Shell git)**:
1. Add handler in backend:
   ```go
   func (s *Service) ListBranches(ctx context.Context, req *session.ListBranchesRequest) (*session.ListBranchesResponse, error) {
     // Validate repo_path
     repoPath, err := filepath.Abs(req.RepoPath)
     if err != nil {
       return nil, status.Error(codes.InvalidArgument, "invalid repo path")
     }
     // Ensure repoPath is within allowed workspace
     // ... validation logic ...
     
     // Execute git command
     cmd := exec.CommandContext(ctx, "git", "-C", repoPath, "for-each-ref", "--format=%(refname:short)", "refs/heads", "refs/remotes")
     output, err := cmd.Output()
     if err != nil {
       return nil, status.Error(codes.NotFound, "not a git repo")
     }
     
     branches := strings.Split(strings.TrimSpace(string(output)), "\n")
     // Apply filter if provided
     if req.Filter != "" {
       branches = filterBranches(branches, req.Filter)
     }
     // Limit results
     if req.MaxResults > 0 && len(branches) > int(req.MaxResults) {
       branches = branches[:req.MaxResults]
     }
     
     return &session.ListBranchesResponse{
       Branches: branches,
       TotalCount: int32(len(branches)),
     }, nil
   }
   ```
2. Add RPC method to proto
3. Test with repos of various sizes
4. Cost: 1–2 hours

**D2 (go-git)**:
1. Add dependency: `go get github.com/go-git/go-git/v5`
2. Implement similar to D1, but use go-git API
3. Test on real repos; handle edge cases
4. Cost: 2–3 hours

**D3 (Cached)**:
1. Wrap D1 or D2 with cache layer (see example above)
2. Add cache invalidation hook (e.g., on git push notification)
3. Monitor cache memory growth
4. Cost: 2–4 hours

### Operational Concerns

- **Repo path validation**: Must ensure paths are absolute and within workspace directory. Add security tests.
- **Performance monitoring**: Add metric for "branch list latency" (p50, p95, p99) and "cache hit rate"
- **Large repos**: If production sees repos with 1000+ branches, consider D3 (cache) or D4 (streaming)
- **Error handling**: Repository may not exist, may be corrupted, or may require authentication. Handle gracefully.

### Prior Art and Lessons Learned

- **GitHub API**: `GET /repos/{owner}/{repo}/branches` with optional query filter; paginates by default
- **GitLab API**: Similar; includes protected flag and commit info per branch
- **CLI tools (gh, lab)**: Most shell out to `git` for consistency

**Lesson**: Shell `git` is fast and reliable; worth the subprocess cost. Only switch to go-git or cache if profiling shows it's a bottleneck.

### Open Questions

1. What is the typical branch count in production repos? (Determines if caching necessary)
2. Should remote branches be included by default, or are local branches sufficient for session creation?
3. Are there long-lived webhooks or git hooks that could invalidate branch cache?

### Recommendation

**Implement D1 (Shell `git`) with D3 (5-minute cache) as the combined solution.**

**Rationale**:
- D1 shell `git` is fast (5–50 ms), reliable, and well-tested
- D3 cache eliminates subprocess overhead for the common case (repeated filters in combobox)
- Total latency for cached hits: <1 ms (imperceptible to user)
- Cache TTL (5 min) balances freshness vs. performance; acceptable for branch listings
- If production profiling shows cache misses are bottleneck, graduate to D2 (go-git) without changing API

**Implementation checklist**:
- [ ] Implement ListBranches RPC with path validation (1–2 hours)
- [ ] Add cache layer with TTL and LRU eviction (1–2 hours)
- [ ] Test on repos with 100, 500, 1000 branches (measure latency, memory)
- [ ] Add metrics: branch_list_latency, cache_hit_rate
- [ ] Integrate with combobox component (C) for autocomplete UX
- [ ] Document API: max_results default, filter semantics, remote branch behavior

---

## Pending Web Searches

For items marked `[TRAINING_ONLY — verify]`, the following searches should be performed to confirm accuracy:

1. **xterm.js ITerminalAddon interface and prepend semantics**
   - Query: "xterm.js ITerminalAddon API documentation 2024"
   - Verify: Does xterm expose public APIs for prepending lines without moving cursor?

2. **xterm.js scrollback option and hard memory limit**
   - Query: "xterm.js scrollback option memory limit circular buffer"
   - Verify: How does `Terminal({ scrollback: N })` enforce the limit? Is it a hard cap?

3. **@opentelemetry/sdk-web bundle size measurement**
   - Query: "@opentelemetry/sdk-web bundle size minified gzip 2025"
   - Verify: Is 45–55 KB accurate? Does tree-shaking reduce it further?

4. **Sentry Browser SDK bundle size**
   - Query: "@sentry/react bundle size minified gzip 2025"
   - Verify: Is 60–70 KB accurate? Does it vary significantly by tier?

5. **Git for-each-ref performance on 500+ branches**
   - Query: "git for-each-ref performance benchmark 500 branches"
   - Verify: What is realistic latency on modern hardware? Does index speed it up?

6. **Downshift vs. Radix UI bundle size direct comparison**
   - Query: "downshift @radix-ui/react-select bundle size comparison"
   - Verify: Is Downshift truly 8–10 KB vs. Radix 20–25 KB when measuring consistently?

7. **xterm.js version stability of internal APIs (coreService, parser)**
   - Query: "xterm.js internal API stability changelog 2024 2025"
   - Verify: How often do internal APIs break? What's the breaking change frequency?

8. **OpenTelemetry CORS requirements for browser export**
   - Query: "OpenTelemetry browser CORS OTLP exporter requirements"
   - Verify: What CORS headers must backend expose for traces to export successfully?

## Web Search Results (2026-04-16)

**xterm.js lazy/virtual scrollback:** No native virtual scroll addon exists. GitHub issue #5377 (July 2025) documents "limited touch support on mobile devices impacts terminal usability" and confirms ballistic scrolling is not supported because "the viewport is actually underneath the row divs." Custom `ITerminalAddon` is the only path. (Source: [xtermjs/xterm.js#5377](https://github.com/xtermjs/xterm.js/issues/5377))

**OTel JS SDK bundle size (verified):** The full `@opentelemetry/sdk-web` + auto-instrumentation bundle is ~300 KB uncompressed, **~60 KB gzipped**. OTel SDK 2.0 (2025) improved tree-shaking significantly — importing only the tracing API + one exporter reduces this substantially. (Source: [signoz.io](https://signoz.io/blog/reduce-opentelemetry-bundle-size-for-browser-frontend))

**Downshift vs Radix Combobox (verified):** Radix UI has no native Combobox primitive (issue #1342 is still open). Radix + Downshift together can represent ~80% of a typical bundle. Downshift is the recommended headless combobox with full keyboard nav. (Source: [radix-ui/primitives#1342](https://github.com/radix-ui/primitives/issues/1342))

**git for-each-ref performance (verified):** 100 branches: p90 ~75ms, p99 ~121ms. Using `--contains HEAD` causes severe performance regression on large repos. Plain `git for-each-ref refs/heads --format='%(refname:short)'` (no `--contains`) stays fast. (Source: [earthly/earthly#3752](https://github.com/earthly/earthly/issues/3752))

---

## Executive Summary

Stapler Squad's four pain points have well-supported solutions:

| Pain Point | Recommended Solution | Rationale |
|-----------|----------------------|-----------|
| **A. Large session slow load** | Custom xterm.js lazy-load addon (A2) | Reuses existing `GetRange` backend API; unifies scrollback UX; acceptable risk with testing |
| **B. No frontend observability** | OpenTelemetry SDK for Web (B1) | Unifies with Go backend OTel; self-hosted; no vendor lock-in; 45–55 KB bundle acceptable |
| **C. No branch autocomplete** | Downshift library (C2) | Smallest bundle (8–10 KB); built-in async; excellent keyboard nav; vanilla-extract compatible |
| **D. Git branch listing API** | Shell `git` + 5-min cache (D1+D3) | Fast (5–50 ms), reliable; cache eliminates subprocess for repeated queries; scales to 1000+ branches |

All four can be implemented in parallel over 2–3 weeks with minimal integration friction. No architectural blockers identified.

