# Stack Research: Analytics System

**Domain**: Library/Framework Choices for the Analytics Provider Adapter Pattern  
**Date**: 2026-05-09

---

## Q1: OpenFeature Provider Interface Contract → Analytics Provider Translation

### OpenFeature Provider Interface (TypeScript)

OpenFeature's `Provider` interface (from `@openfeature/web-sdk`) defines the following contract:

```typescript
interface Provider {
  readonly metadata: ProviderMetadata;           // { name: string }
  readonly runsOn?: "client" | "server";
  hooks?: Hook[];

  // Lifecycle hooks (optional)
  initialize?(context?: EvaluationContext): Promise<void>;
  onClose?(): Promise<void>;
  onContextChange?(oldCtx: EvaluationContext, newCtx: EvaluationContext): Promise<void>;

  // Core resolution methods (one per type)
  resolveBooleanEvaluation(flagKey: string, defaultValue: boolean, context: EvaluationContext, logger: Logger): Promise<ResolutionDetails<boolean>>;
  resolveStringEvaluation(...): Promise<ResolutionDetails<string>>;
  resolveNumberEvaluation(...): Promise<ResolutionDetails<number>>;
  resolveObjectEvaluation<T extends JsonValue>(...): Promise<ResolutionDetails<T>>;
}
```

**Key design patterns from OpenFeature to adopt:**

1. **Global singleton registration**: `OpenFeature.setProvider(new MyProvider())` — a single line to swap the active implementation. Our `AnalyticsContext` should mirror this with a `setProvider()` function or a context value.
2. **`initialize()` / `onClose()` lifecycle hooks**: Providers handle async setup (connecting, loading config) before the first call arrives.
3. **Metadata object**: Every provider carries `{ name: string }` so logging/debugging can identify which provider is active.
4. **No inheritance**: The interface is purely structural (TypeScript `interface`), not a class hierarchy.

### Recommended `AnalyticsProvider` TypeScript Interface

Model the interface closely after OpenFeature's structural contract:

```typescript
// web-app/src/lib/analytics/types.ts

export interface AnalyticsEvent {
  name: string;
  category: "user_action" | "performance" | "navigation" | "rpc";
  durationMs?: number;
  sessionId?: string;
  page?: string;
  component?: string;
  labels?: Record<string, string>;
}

export interface AnalyticsProviderMetadata {
  readonly name: string;
}

export interface AnalyticsProvider {
  readonly metadata: AnalyticsProviderMetadata;

  /** Called once when the provider is registered. Set up connections, batch queues, etc. */
  initialize?(): Promise<void>;

  /** Called on teardown (page unload, test cleanup). Flush any pending events. */
  onClose?(): Promise<void>;

  /**
   * Record a single analytics event. Must be fire-and-forget (non-blocking).
   * Implementations must never throw — errors are swallowed or queued for retry.
   */
  track(event: AnalyticsEvent): void;

  /**
   * Flush any buffered events. Returns a Promise that resolves when the flush completes.
   * Optional: not all providers buffer (e.g., ConsoleAnalyticsProvider is synchronous).
   */
  flush?(): Promise<void>;
}
```

**Two concrete providers:**

```typescript
// ConsoleAnalyticsProvider — dev-mode
export class ConsoleAnalyticsProvider implements AnalyticsProvider {
  readonly metadata = { name: "ConsoleAnalyticsProvider" };
  track(event: AnalyticsEvent): void {
    console.debug("[analytics]", event.name, event);
  }
}

// HttpAnalyticsProvider — posts to /api/analytics with internal batching
export class HttpAnalyticsProvider implements AnalyticsProvider {
  readonly metadata = { name: "HttpAnalyticsProvider" };
  private queue: AnalyticsEvent[] = [];
  private flushTimer?: ReturnType<typeof setTimeout>;
  private readonly BATCH_SIZE = 25;
  private readonly FLUSH_INTERVAL_MS = 2000;

  track(event: AnalyticsEvent): void {
    this.queue.push(event);
    if (this.queue.length >= this.BATCH_SIZE) {
      void this.flush();
    } else if (!this.flushTimer) {
      this.flushTimer = setTimeout(() => void this.flush(), this.FLUSH_INTERVAL_MS);
    }
  }

  async flush(): Promise<void> {
    clearTimeout(this.flushTimer);
    this.flushTimer = undefined;
    if (this.queue.length === 0) return;
    const batch = this.queue.splice(0);
    await fetch("/api/analytics", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ events: batch }),
    }).catch(() => {}); // fire-and-forget
  }
}
```

**Global registration pattern** (mirrors `OpenFeature.setProvider()`):

```typescript
// web-app/src/lib/analytics/AnalyticsContext.tsx
let _provider: AnalyticsProvider = new ConsoleAnalyticsProvider();

export function setAnalyticsProvider(p: AnalyticsProvider): void {
  _provider = p;
}

export function useAnalytics(): AnalyticsProvider {
  return useContext(AnalyticsContext);
}
```

**Verdict**: The OpenFeature pattern maps cleanly. Key differentiator: analytics has a single `track()` method vs. OpenFeature's 4 typed resolution methods. Adopt the `metadata`, `initialize()`, `onClose()`, and `flush()` optional methods verbatim.

---

## Q2: ESLint Custom Rules — Local Plugin Approach for Next.js/TypeScript

### Local Plugin vs. Published Package

The project currently uses ESLint v9 (flat config era, per `"eslint": "^9"` in `package.json`) but the existing `.eslintrc.json` is the **legacy format**. This creates an important constraint.

**Two viable approaches:**

#### Option A: Local plugin as a workspace package (Recommended)

Create `web-app/eslint-plugin-analytics/` as a local Node package:

```
web-app/
  eslint-plugin-analytics/
    index.js          ← plugin entry point
    rules/
      require-on-click.js
      require-omnibar-dispatch.js
      require-page-analytics.js
      require-rpc-analytics.js
    tests/
      require-on-click.test.js
      ...
    package.json      ← { "name": "eslint-plugin-analytics", "main": "index.js" }
```

In `web-app/package.json`, add a local file reference:
```json
{
  "devDependencies": {
    "eslint-plugin-analytics": "file:./eslint-plugin-analytics"
  }
}
```

Then in `.eslintrc.json`:
```json
{
  "plugins": ["analytics"],
  "rules": {
    "analytics/require-on-click": "error",
    "analytics/require-omnibar-dispatch": "error",
    "analytics/require-page-analytics": "error",
    "analytics/require-rpc-analytics": "error"
  }
}
```

**Advantages**: clean separation, proper `package.json`, testable with `RuleTester`, no build step needed (CommonJS), works with both legacy and flat config formats, tracked in git, symlinked by npm in `node_modules`.

#### Option B: eslint-plugin-local

The `eslint-plugin-local` package allows inline rule definitions directly in `.eslintrc.json` without a separate package. However: it does not work cleanly with ESLint v9's flat config, and mixing legacy `.eslintrc.json` with local rules creates maintenance friction. **Not recommended** for 4 rules with tests.

### Rule-Writing APIs: AST Selectors

All rules use the ESLint `RuleContext` API with AST selectors. Key selectors for each rule:

#### `require-on-click` — JSX onClick on buttons/anchors

```javascript
// Selector: catch JSXAttribute named "onClick" on relevant elements
module.exports = {
  create(context) {
    return {
      // AST selector for JSX onClick attributes
      'JSXAttribute[name.name="onClick"]'(node) {
        // node.parent is JSXOpeningElement
        const elementName = node.parent.name.name; // "button", "a", etc.
        const roleAttr = node.parent.attributes.find(
          a => a.type === "JSXAttribute" && a.name?.name === "role"
        );
        const isButton = elementName === "button" || elementName === "a" ||
          roleAttr?.value?.value === "button";
        if (!isButton) return;

        // Check for analytics-exempt comment
        const sourceCode = context.getSourceCode();
        const comments = sourceCode.getCommentsBefore(node.parent.parent); // JSXElement
        if (comments.some(c => c.value.includes("analytics-exempt"))) return;

        // Walk up to the enclosing function component, check for useAnalytics().track(...)
        // Use context.getScope() and scope analysis, or check the JSX body for CallExpression
        // with callee matching useAnalytics().track
        if (!componentCallsTrack(node, context)) {
          context.report({ node, message: "onClick handlers must call useAnalytics().track()" });
        }
      }
    };
  }
};
```

**Important**: For JSX comments (`{/* analytics-exempt */}`), use `getCommentsBefore` on the JSXElement node or check `JSXExpressionContainer` with string literal.

#### `require-omnibar-dispatch` — Switch cases in dispatchOmnibarAction

```javascript
// Selector: SwitchStatement inside the specific function, then each SwitchCase
'FunctionDeclaration[id.name="dispatchOmnibarAction"] SwitchCase'(node) {
  // Check that node.consequent contains a CallExpression for track(...)
  const hasTrack = node.consequent.some(stmt => containsTrackCall(stmt));
  if (!hasTrack) { context.report(...) }
}
// Or with arrow function:
'VariableDeclarator[id.name="dispatchOmnibarAction"] SwitchCase'(node) { ... }
```

#### `require-page-analytics` — Files matching app/**/page.tsx

```javascript
// Use context.getFilename() to gate the rule
create(context) {
  if (!/app\/.*\/page\.tsx?$/.test(context.getFilename())) return {};
  return {
    'ExportDefaultDeclaration'(node) {
      // Check if file contains usePageView() or useAnalytics().track('page_view'...)
      const body = context.getSourceCode().ast.body;
      if (!fileContainsPageViewCall(body)) {
        context.report({ node, message: "Page components must call usePageView()" });
      }
    }
  };
}
```

#### `require-rpc-analytics` — Hooks from useSessionService

```javascript
// Match calls to hooks like useCreateSession, useListSessions, etc.
'CallExpression[callee.name=/^use[A-Z]/]'(node) {
  // Determine if callee is from useSessionService (check import source)
  // If yes, verify sibling track() call in same component
}
```

### Testing Rules with RuleTester

```javascript
const { RuleTester } = require("eslint");
const rule = require("../rules/require-on-click");

const tester = new RuleTester({
  parserOptions: { ecmaVersion: 2020, ecmaFeatures: { jsx: true } }
});

tester.run("require-on-click", rule, {
  valid: [
    { code: `<button onClick={() => { useAnalytics().track("click"); }} />` },
    { code: `{/* analytics-exempt: intentional */}\n<button onClick={noop} />` }
  ],
  invalid: [
    { code: `<button onClick={noop} />`, errors: [{ messageId: "missingTrack" }] }
  ]
});
```

### Integration with make quick-check

The `lint` script in `package.json` runs `next lint`. To include the local plugin in `make quick-check`, add to `package.json`:
```json
"lint": "next lint && npm run lint:css && npm run lint:css-vars"
```
The local plugin will be picked up automatically once added to `.eslintrc.json` plugins array and installed via `npm install`.

**Note**: ESLint v9 with flat config (`eslint.config.js`) is cleaner for local plugins — consider migrating `.eslintrc.json` to `eslint.config.mjs` as part of this feature. With flat config, local plugins are imported directly without the `file:` package reference:
```javascript
import analyticsPlugin from "./eslint-plugin-analytics/index.js";
export default [{ plugins: { analytics: analyticsPlugin }, rules: { ... } }];
```

---

## Q3: ent ORM vs. Raw SQLite for AnalyticsEvent Entity

### Recommendation: Use ent ORM (consistent with existing setup)

The project already uses ent for `ErrorEvent`, `ClassificationAnalytics`, `Session`, `Worktree`, and other entities. Adding `AnalyticsEvent` as an ent entity is strongly preferred for consistency.

### Correct Generate Command

From `session/ent/generate.go` (line 3), the authoritative command is:

```bash
go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./schema
```

**Run from `session/ent/` directory**, or using `go generate`:
```bash
go generate ./session/ent/...
```

The `--feature sql/upsert` flag is **critical** — omitting it breaks `UpsertRule`, `OnConflict`, and similar upsert methods used elsewhere in the codebase. This is documented in `CLAUDE.md`.

### AnalyticsEvent Schema (ent)

Following the pattern established by `ClassificationAnalytics` and `ErrorEvent`:

```go
// session/ent/schema/analytics_event.go
package schema

import (
    "time"
    "entgo.io/ent"
    "entgo.io/ent/schema/field"
    "entgo.io/ent/schema/index"
)

type AnalyticsEvent struct{ ent.Schema }

func (AnalyticsEvent) Fields() []ent.Field {
    return []ent.Field{
        field.UUID("id", uuid.UUID{}).Default(uuid.New).Immutable(),
        field.String("event_name").NotEmpty(),
        field.Enum("event_category").
            Values("user_action", "performance", "navigation", "rpc"),
        field.String("session_id").Optional(),
        field.Int64("duration_ms").Optional().Nillable(),
        field.String("page").Optional(),
        field.String("component").Optional(),
        field.JSON("labels", map[string]string{}).Optional(),
        field.Time("created_at").Default(time.Now).Immutable(),
    }
}

func (AnalyticsEvent) Indexes() []ent.Index {
    return []ent.Index{
        index.Fields("event_name"),
        index.Fields("event_category"),
        index.Fields("session_id"),
        index.Fields("created_at"),
    }
}
```

### Gotchas

1. **UUID field**: Requires importing `github.com/google/uuid`. The existing `Session` schema likely already uses it — verify with `grep -r "uuid" session/ent/schema/`.

2. **JSON field for labels**: `field.JSON("labels", map[string]string{})` serializes to SQLite TEXT as JSON. Query filtering on JSON keys is possible but requires SQLite JSON functions — the summary endpoint should read and aggregate in Go, not via SQL JSON queries, for portability.

3. **Enum values**: ent enums generate a Go type and a check constraint. If the set of categories expands, a new migration is needed. Alternative: use `field.String("event_category")` with a Go-level validator if you want flexibility without migrations.

4. **Retention policy (100k / 90 days)**: ent has no built-in TTL or row-count enforcement. Implement a background goroutine that runs `DELETE FROM analytics_events WHERE created_at < $cutoff OR id NOT IN (SELECT id FROM analytics_events ORDER BY created_at DESC LIMIT 100000)` on a timer. This query can be expressed with ent's `Delete().Where(...)` predicate.

5. **Commit all generated files together**: After running the generate command, commit everything under `session/ent/` in a single commit — `session/ent/client.go`, `session/ent/analytics_event.go`, `session/ent/analytics_event_create.go`, etc. Partial commits cause build failures.

6. **Migration**: The existing codebase uses ent's auto-migrate (`schema.Create()`). Confirm in `server/dependencies.go` or `server.go` that `entClient.Schema.Create(ctx)` is called at startup — the new entity's table will be created automatically.

### Raw SQLite Alternative (Rejected)

Raw SQLite (`database/sql` + `mattn/go-sqlite3` or `modernc.org/sqlite`) would require:
- Manual schema migration tracking
- Hand-written SQL for upsert, index creation, and batch insert
- Separate query layer inconsistent with the rest of the codebase
- Loss of ent's type-safe query builder for the summary endpoint aggregations

The only argument for raw SQL would be performance-critical bulk insert — but at 100 events/min maximum (see Q4), ent's overhead is negligible.

---

## Q4: Rate Limiter Scaling Analysis and Client-Side Batching

### Current Rate Limiter Analysis

The existing `rateLimiter` in `telemetry_handler.go`:
- **Algorithm**: Fixed window (not sliding window — comment says "sliding" but implementation resets a counter every minute)
- **Limit**: 100 requests per minute, **global** (shared across all clients via a single `rateLimiter` instance on `TelemetryHandler`)
- **No per-IP or per-session granularity**

**Current usage**: A single user performing normal activity generates ~5-15 telemetry events per minute (session attachments, RPC calls). 100/min is ample headroom.

### Event Volume with Analytics System Enabled

With all four event categories active:

| Source | Events/min (estimate) | Notes |
|--------|----------------------|-------|
| onClick handlers | 10–30 | User clicks on buttons/links |
| Web Vitals (CWV) | 5–10 | LCP, CLS, FID/INP, FCP, TTFB — fires once per page load |
| RPC timing | 20–50 | Every ConnectRPC call; polling RPCs can be frequent |
| Page views | 2–5 | Navigation events |
| **Total (single user)** | **37–95** | Near the 100/min limit |

**Problem**: 100 req/min is inadequate for a single active user with all categories enabled. RPC polling (e.g., session status checks every 2–5 seconds) alone can generate 12–30 RPCs/min. Combined with onClick and Web Vitals, a power user will hit the rate limit within the first minute.

### Recommendation: Client-Side Batching + Raised Backend Limit

**Primary fix: Client-side batching in `HttpAnalyticsProvider`**

Batch events before sending to the backend:
- Buffer up to 25 events OR flush every 2 seconds (whichever comes first)
- Use `navigator.sendBeacon()` for page-unload flush (guaranteed delivery)
- Result: 100 events/min → 4–8 HTTP requests/min, well within any reasonable limit

```typescript
// Already shown in Q1 HttpAnalyticsProvider — batch queue with 25-event threshold
// and 2000ms flush timer
```

**Secondary fix: Raise the backend limit + make it configurable**

The new `/api/analytics` endpoint should:
1. Accept a **batch** request body (`{ events: AnalyticsEvent[] }`) — one HTTP call per flush
2. Use a raised rate limit (e.g., 500 batches/min or 10,000 events/min)
3. Store the limit in `config.json` alongside the retention policy

**Tertiary fix: The new endpoint validates batch size, not request count**

```go
// analytics_handler.go
const maxBatchSize = 100

if len(req.Events) > maxBatchSize {
    http.Error(w, "batch too large", http.StatusBadRequest)
    return
}
```

### Backward Compatibility

The existing `/api/telemetry` with 100 req/min limit must remain unchanged (zero-regression requirement from FR-2 non-functional). The new `/api/analytics` is a separate endpoint with its own handler and rate limiter tuned for batched workloads.

**< 50ms p99 requirement**: SQLite write for a batch of 25 events via ent upsert is typically 1–5ms. The limit is achievable if the handler is synchronous (write → respond). For very high concurrency, add a channel-based write queue, but this is not needed for a single-user local app.

### Current Rate Limiter Bug

The comment says "sliding-window" but the implementation is a **fixed window** (resets `count = 0` when `now.After(r.resetAt)`). A fixed window allows burst of 200 requests across a window boundary. For the new analytics handler, document this accurately or use `golang.org/x/time/rate` (token bucket) which is already available in the Go module ecosystem and gives true rate limiting with burst control.

---

## Summary Table

| Question | Recommendation |
|----------|----------------|
| AnalyticsProvider interface | Mirror OpenFeature's structural contract: `metadata`, `initialize?()`, `onClose?()`, `track()`, `flush?()` — see interface above |
| ESLint local plugin | `web-app/eslint-plugin-analytics/` as `file:` workspace package; use `RuleTester` for tests; consider flat config migration |
| ent ORM for AnalyticsEvent | Use ent, follow existing `ClassificationAnalytics` pattern, run `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./schema` |
| Rate limiting | Current 100 req/min is insufficient; add client-side batching in `HttpAnalyticsProvider` (25 events / 2s flush); new `/api/analytics` accepts batch body |
