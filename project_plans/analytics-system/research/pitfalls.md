# Analytics System — Pitfalls Research

Domain: Known failure modes, performance traps, and migration costs.

---

## 1. ESLint Custom Rule Pitfalls

### 1.1 JSX AST Traversal for `onClick`

**Relevant AST node types:**

- `JSXAttribute` with `name.name === "onClick"` — the prop itself
- `JSXOpeningElement` — parent; its `name` property is a `JSXIdentifier` (for `<button>`) or `JSXMemberExpression` (for `<Foo.Bar>`)
- `JSXIdentifier.name` — the tag name string (e.g. `"button"`, `"div"`, `"a"`)

**Distinguishing button from div:**
The rule must walk up from the `JSXAttribute` to its `JSXOpeningElement` parent and check:
- `node.parent.name.name` (for intrinsic elements) — allow if it equals `"button"` or `"a"`
- For `[role=button]`: a sibling `JSXAttribute` where `name.name === "role"` and `value` is `Literal("button")` — requires scanning all attributes on the same opening element

**Spread props (`{...props}`) — the biggest false-positive source:**
When a component uses `<div {...props}>`, ESLint cannot know whether `props` contains an `onClick`. The rule must:
- Check for a `JSXSpreadAttribute` sibling on the same `JSXOpeningElement`
- Either suppress the error (risk: missed coverage) or require the `// analytics-exempt` comment (correct approach)

Failing to handle spread will generate false positives on every component that forwards props — e.g., polymorphic `<Button as="div" {...rest}>` wrappers that are common in the existing `ui/` components.

**Inherited onClick detection (event bubbling):**
A `<div onClick>` wrapping a `<button>` is structurally invisible to an attribute-level rule. The rule cannot detect this pattern; it should only fire on elements that match the explicit target list.

### 1.2 Hook Call Detection for `require-rpc-analytics`

**Goal:** detect that `useSessionService()` call site (or calls to methods it returns) is in the same component scope as a `track()` call.

**AST approach:**

The rule needs to:
1. Find `CallExpression` where `callee.name === "useSessionService"` or where the call site destructures from a known service hook
2. Walk up to the enclosing `FunctionDeclaration` / `ArrowFunctionExpression` / `FunctionExpression` that represents the React component
3. Check whether any `CallExpression` with `callee.name === "track"` (or `callee.object.name === "analytics"` + `callee.property.name === "track"`) exists within that same function scope

**False-positive risks:**
- The hook is called inside `useSessionServiceContext()` (a context wrapper in `SessionServiceContext.tsx`), not directly. The ESLint rule must account for both `useSessionService` and `useSessionServiceContext` — or any hook that wraps them — otherwise it fires on every context consumer even when the consuming component has tracking.
- `useSessionService` is used in `OmnibarContext.tsx` (a provider) and `useSessionActions.ts` (a custom hook). Provider components and custom hook files are not React components in the UI sense — flagging them as missing analytics is a false positive. The rule needs a file-path exclusion or a component-detection heuristic.
- Tracking inside callbacks (e.g., `useCallback`): the rule must accept `track()` calls inside nested `useCallback` within the same component function, not only at the top level of the function body.

**Scope-walking trap:** ESLint's `Scope` analysis should be used (`context.getScope()` and `scope.upper`) rather than manual AST parent walking — it handles closures correctly.

### 1.3 Switch Case Enforcement for `require-omnibar-dispatch`

**Goal:** every `case` inside `dispatchOmnibarAction`'s switch must contain a `track()` call.

**AST path:**

```
FunctionDeclaration (name: "dispatchOmnibarAction")
  └─ BlockStatement
       └─ SwitchStatement
            └─ SwitchCase[]
                 └─ (consequent must contain CallExpression for track)
```

The rule needs to:
1. Match a `FunctionDeclaration` or `FunctionExpression` by name (`dispatchOmnibarAction`) — or match the exported arrow function assigned to that identifier
2. Find the `SwitchStatement` as a direct child of the function body
3. For each `SwitchCase`, scan `consequent` recursively for a qualifying `CallExpression`

**Traps:**
- `dispatchOmnibarAction` is exported as a named function declaration in `dispatch.ts`. If it is ever refactored to a `const dispatchOmnibarAction = (action, deps) => { ... }` arrow function, the rule's `FunctionDeclaration` selector breaks. Use `FunctionDeclaration, VariableDeclarator > ArrowFunctionExpression` and match on the declared identifier name.
- The current switch has a `// TypeScript exhaustiveness` comment in the tail but no `default` case. If a new case is added without tracking, it compiles fine but evades the rule if the rule only runs on existing cases. The rule should error on new `SwitchCase` nodes that lack a `track` call, regardless of whether they are new or old — which means initially, retrofitting all existing cases will be required before enabling the rule in CI.
- Nested switches (if a case body contains another switch) — the rule should not recurse into inner switches looking for `track` calls; it should only enforce at the direct children of the named function's switch.

---

## 2. SQLite Write Performance

### 2.1 Write Volume Estimate

With the four callsite categories enforced by ESLint rules firing in normal use:

| Source | Estimated frequency |
|--------|---------------------|
| onClick on buttons/links | ~5–20 per active minute (UI-heavy usage) |
| omnibar dispatch cases (create, navigate, etc.) | ~2–5 per minute |
| Page view transitions | ~1–3 per minute |
| RPC calls (createSession, listSessions, deleteSession, etc.) | ~5–30 per active minute |
| Web Vitals (CLS, LCP, FID) | ~3 per page load |
| RPC latency (rpcTiming.ts) | ~1 per RPC |

**Realistic peak:** ~50–100 analytics events per active minute per browser tab. At 1 user, that's <2 events/second. At 5 concurrent users, ~10 events/second.

### 2.2 SQLite Write Throughput

SQLite's WAL mode (already configured in `ent_repository.go` via `_journal_mode=WAL`) supports roughly:
- **10,000–50,000 simple INSERT/s** on SSD for serialized single-connection writes
- The existing config uses `db.SetMaxOpenConns(1)` — one writer at a time, which is correct for SQLite

**The real risk is not throughput but latency spikes:** each analytics write blocks the single connection briefly. If the analytics endpoint shares the same `*ent.Client` as session management, a burst of analytics writes will delay session list queries.

**Recommendation:** Use a dedicated SQLite file (`analytics.db`) separate from `sessions.db`. This allows each to have its own `MaxOpenConns(1)` connection pool without cross-blocking. The existing pattern in `ent_repository.go` makes this straightforward — instantiate a second `EntRepository` pointing to a different path.

### 2.3 Batching

The existing `RecordAnalytics` (for `ClassificationAnalytics`) writes one row per call without batching. For analytics events fired from the browser (network hop + JSON decode + DB write), individual writes are fine at low volume. However, if Web Vitals and RPC timing events fire in rapid bursts on page load, an in-process write queue with 100ms debounce flush would reduce write pressure. This is the same pattern used by many embedded analytics stores.

**Retention enforcement (`max 100k / 90 days`):** without a background cleanup job or a trigger, the table will grow unbounded. The `ClassificationAnalytics` entity has no retention policy either. An `ON DELETE TRIGGER` or a periodic `DELETE FROM analytics_events WHERE created_at < ?` in a goroutine must be added explicitly — ent does not handle this automatically.

---

## 3. Ent Schema Migration Risk

### 3.1 How ent Handles Migrations

The project uses **`client.Schema.Create(ctx)`** called on startup in `session/ent_repository.go:84`. This is ent's _automatic migration_ mode — it applies `ALTER TABLE ADD COLUMN` for new columns, creates new tables, and adds new indexes. It does **not** drop columns or indexes by default (controlled by `WithDropColumn` and `WithDropIndex` options, which are not passed here).

**Safe operations for `AnalyticsEvent`:**
- Adding the new table on first deploy: safe — `Schema.Create` creates missing tables
- Adding a new optional column later: safe — `Schema.Create` will `ALTER TABLE ADD COLUMN` with a `NULL`/default value
- Renaming a column: **not safe** — ent auto-migration does not detect renames; it adds the new column and leaves the old one orphaned

**Dangerous operations:**
- Changing a field type (e.g., `Int` → `Int64`): ent may attempt an `ALTER COLUMN` which SQLite does not support — it would need to create a new table, copy data, and drop the old one (the "12-step SQLite alter" workaround). With plain `Schema.Create`, this silently fails or panics.
- Making an optional field required: existing rows with `NULL` fail the constraint on reads

### 3.2 Atlas Involvement

The project has `ariga.io/atlas v0.32.1` in `go.mod` as an **indirect** dependency (pulled in by `entgo.io/ent v0.14.5`). Atlas is not configured as a migration tool here — it is not in the migration path. The project does not use `atlas migrate` CLI or Atlas Cloud; migrations are purely through `Schema.Create`.

**Risk:** Atlas's versioned migration feature (generates SQL diff files, applies them in order) is not in use. If the `AnalyticsEvent` schema changes post-deployment, there is no migration file, no rollback script, and no hash check. The only safety net is that `Schema.Create` is additive-only by default.

**Practical guidance for `AnalyticsEvent`:**
- Design the schema to be as stable as possible at first implementation
- Use `field.JSON("labels")` for extensible metadata rather than adding new columns per label type
- Mark all analytics-specific fields as `.Optional()` to allow safe `ALTER TABLE ADD COLUMN NULL` if the schema evolves

---

## 4. Rate Limiting Risk

### 4.1 Current Limit

`telemetry_handler.go` implements a sliding-window rate limiter at **100 requests per minute**, global (not per-session or per-IP). It resets every 60 seconds using a simple counter.

### 4.2 Projected Volume vs. Limit

At 50–100 events per active minute per user with **1 user**, the new `/api/analytics` endpoint will fire **at or near the limit**. With 2+ simultaneous users:
- 2 users at 60 events/min = 120 events/min → **rate limited after 100**
- The rate limiter is instance-global — it does not distinguish between users, tabs, or event types

**Critical:** The new analytics endpoint will need its own rate limiter with a substantially higher cap, or the limit must be per-session-ID or per-client-IP. A global 100/min limit was appropriate for the original telemetry endpoint (which only received 3 Web Vitals per page load) but is far too low for all-clickstream analytics.

### 4.3 Recommended Limit

For a single-user local tool at 100 events/min with burst headroom: **500–1000/min**. For multi-user: consider per-IP limiting at 200/min per client. The existing limiter structure is simple enough to extend with a per-key map.

---

## 5. React Render Loop Risk

### 5.1 The Problem

If `useAnalytics().track()` internally calls `setState` (e.g., storing tracking state in a React context), any component that calls `track()` inside an `onClick` handler will trigger a re-render of all context consumers. If the same component re-renders and the `onClick` function identity changes (not wrapped in `useCallback`), React may re-render children, causing a cascade.

### 5.2 How the Current `track()` is Safe

The existing `track()` in `web-app/src/lib/telemetry.ts` is a **pure function** (no React state, no hooks) — it just fires `fetch()` and returns. This is the correct pattern and must be preserved in `useAnalytics().track()`.

### 5.3 Risk Points in the New Design

The requirements call for `AnalyticsContext` (React context) exposing `useAnalytics()`. Risk:
- If the context value is a new object on every render of the provider, `useAnalytics()` consumers will re-render unnecessarily. Mitigation: `useMemo(() => ({ track: stableTrackFn }), [])` in the provider.
- If `track()` is implemented as `useCallback` referencing state, re-renders of the provider invalidate the function identity. Mitigation: use a `useRef` to hold the provider instance; return a stable wrapper.
- If the `ConsoleAnalyticsProvider` (dev mode) logs to the console inside a render (not inside an event handler), React StrictMode's double-invoke will log twice — not a bug but confusing. Track calls must only be in event handlers and effects.

### 5.4 Enforcement

The ESLint rule `analytics/require-on-click` should only flag `onClick` handlers, not render-time code — so by construction, `track()` will always be called from event handlers (safe). The `require-rpc-analytics` rule checking for `track()` in the same component scope does not restrict *where* the call is, so the rule documentation should note that `track()` must be inside the callback, not at the top level of the component body.

---

## 6. Telemetry Handler Tests — Current Coverage

The existing `server/handlers/telemetry_handler_test.go` has **6 tests**:

| Test | What it covers |
|------|----------------|
| `TestTelemetry_MissingEvent` | 400 when `event` field is absent |
| `TestTelemetry_MissingDuration` | 400 when `duration_ms` is 0 |
| `TestTelemetry_TooManyLabels` | 400 when `labels` map has >100 keys |
| `TestTelemetry_ValidRequest` | 204 happy path |
| `TestTelemetry_MethodNotAllowed` | 405 on GET |
| `TestTelemetry_RateLimit` | 429 after 100 requests in a window |

**Coverage gaps for the new `/api/analytics` endpoint that will need tests:**
- Persistence to SQLite: does the event actually appear in the DB after POST?
- Invalid `event_category` enum values
- Optional fields (`session_id`, `duration_ms`) being absent
- Retention enforcement (>100k rows or >90 days)
- Summary endpoint aggregation correctness (p50/p95/p99 calculation)
- Rate limiter reset after window expiry (the existing test does not test reset)
- Concurrent writes (goroutine safety of the new provider)

The telemetry handler currently has **no test for log-injection sanitization** — the production code sanitizes `\n`/`\r` from event names, but there is no test asserting this. This should be added when the handler is upgraded.

---

## 7. Privacy Risk Model

### 7.1 Session IDs as Identifiers

The requirements state that `session_id` in analytics events is an "opaque identifier." Session IDs in this codebase are UUIDs assigned per session (`uuid` field in the `sessions` table, type `string`). They are not user-identifiers; they map to a specific tmux session / worktree.

**Correlation risk:** A session ID can correlate all events within a session (page views, RPC calls, clicks) without revealing who the user is. In a single-user local tool, this is essentially self-correlation — no privacy risk. In a multi-user or shared deployment, session IDs could reveal usage patterns per session but not per-person identity.

**Risk verdict: Low.** Session IDs do not contain PII. They are scoped to the stapler-squad instance (local machine) and are not sent to external services.

### 7.2 Indirect PII in Labels or Event Names

The `labels` field is a JSON map with arbitrary keys/values. **Risk:** if a caller passes user-generated content as a label value (e.g., the session title, which might be a personal project name), that is stored as analytics data. The ESLint rules enforce where `track()` is called but cannot enforce what is passed in `labels`.

**Mitigation needed:** The `POST /api/analytics` handler should validate label values (e.g., max key/value length, no credential-like patterns). More importantly, the `useAnalytics().track()` API documentation and code examples must not demonstrate passing free-form user input as label values.

### 7.3 Path Fields

The `page` field stores the current route (e.g., `/sessions/my-project-name`). Project names embedded in paths could be considered quasi-identifying. Since this is a local-only tool (data never leaves the machine), this is low risk, but it should be documented in the schema as "may contain user-defined names."

---

## Summary of Top Pitfalls

1. **ESLint false positives from spread props and context wrappers** — the `require-on-click` rule will fire on every component using `{...props}` on a `<button>`, and `require-rpc-analytics` will fire on provider components and custom hooks that call `useSessionService` but are not themselves UI components. Both rules need explicit exclusion lists or heuristics before they can run in CI without overwhelming noise.

2. **Rate limit is 20x too low for full analytics instrumentation** — the existing 100/min global cap on the telemetry endpoint will reject events from 2+ concurrent users once all four callsite types are firing. The new `/api/analytics` endpoint needs its own higher limit (500–1000/min) or per-client rate limiting before enabling broad callsite coverage.

3. **Ent auto-migration is additive-only and has no rollback** — `Schema.Create` safely creates the new `AnalyticsEvent` table, but any post-deploy column rename, type change, or constraint tightening requires a manual SQLite table-rebuild workaround. Design the schema for maximum stability (use `labels JSON` for extensibility, mark all non-core fields `.Optional()`) and document this constraint explicitly to prevent future pain.
