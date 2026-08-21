# Features Research: Analytics System

Domain: What do comparable open-source analytics projects do, and what patterns should we borrow?

---

## 1. OpenFeature Provider Interface — Mapping to an Analytics Provider

### What OpenFeature Does

OpenFeature defines a standardized `Provider` interface for feature-flag backends. The full contract (TypeScript) is:

```typescript
interface Provider {
  readonly metadata: ProviderMetadata;           // { name: string }

  // Lifecycle
  initialize(context?: EvaluationContext): Promise<void>;
  shutdown(): Promise<void>;

  // Evaluation (one per type)
  resolveBooleanValue(flagKey, defaultValue, context?): ResolutionDetails<boolean>;
  resolveStringValue(flagKey, defaultValue, context?):  ResolutionDetails<string>;
  resolveNumberValue(flagKey, defaultValue, context?):  ResolutionDetails<number>;
  resolveObjectValue<T>(flagKey, defaultValue, context?): ResolutionDetails<T>;

  // Optional hooks (tap into evaluation lifecycle stages)
  hooks?: Hook[];
}

type ResolutionDetails<T> = {
  value: T;
  variant?: string;         // e.g. "on" / "off"
  reason?: string;          // STATIC, TARGETING_MATCH, ERROR, …
  errorCode?: ErrorCode;
  errorMessage?: string;
};
```

Lifecycle rules worth borrowing:
- `initialize()` is called once by the SDK before the provider is used; it may throw/reject to signal a broken provider.
- `shutdown()` is idempotent — calling it twice has no effect.
- After `shutdown()`, the provider reverts to its "uninitialized" state; a fresh `initialize()` is required before reuse.
- Provider hooks (`before`, `after`, `error`, `finally`) tap into each evaluation stage — analogous to middleware.

### How This Maps to `AnalyticsProvider`

| OpenFeature concept | AnalyticsProvider equivalent |
|---|---|
| `initialize(context)` | `initialize(): Promise<void>` — open DB connection, start flush timer |
| `shutdown()` | `flush(): Promise<void>` + close connection — drain queue, dispose resources |
| `resolveXxxValue(key, default, ctx)` | `track(event: AnalyticsEvent): void` — single primary method |
| `hooks` (before/after evaluation) | middleware stack for sampling, redaction, enrichment |
| `ProviderMetadata.name` | `name: string` — used for logging/debugging |

**Recommended interface for this project:**

```typescript
// web-app/src/lib/analytics/provider.ts
export interface AnalyticsProvider {
  readonly name: string;
  initialize(): Promise<void>;                     // idempotent; called by AnalyticsContext on mount
  track(event: AnalyticsEvent): void;              // fire-and-forget at callsite
  flush(): Promise<void>;                          // drain queue; await on unmount / page hide
}
```

Swap pattern (mirrors `OpenFeature.setProvider()`):

```typescript
// In tests or storybook:
AnalyticsRegistry.setProvider(new ConsoleAnalyticsProvider());
```

Sources: [Provider spec](https://openfeature.dev/specification/sections/providers/) | [Provider concepts](https://openfeature.dev/docs/reference/concepts/provider/)

---

## 2. PostHog capture() API and Event Schema

### PostHog Capture Event Schema

Every event at the `/i/v0/e` (single) or `/batch` (bulk) endpoint requires:

```json
{
  "api_key": "phc_...",
  "distinct_id": "user-uuid-or-anon-id",   // required; uuidv7 recommended for anon
  "event": "clicked_button",               // required; event name
  "timestamp": "2026-05-09T12:00:00Z",     // ISO 8601; optional, defaults to server receipt time
  "properties": {
    "$current_url": "https://app.example.com/sessions",
    "$referrer": "https://app.example.com/",
    "component": "Omnibar",
    "session_type": "new_worktree",
    "$set": { "plan": "pro" },             // person property: overwrite on every event
    "$set_once": { "initial_referrer": "…" } // person property: write once, never overwrite
  }
}
```

Batch endpoint wraps events in `{ "batch": [...events] }`.

### TypeScript SDK Batching and Retry

The `posthog-node` v3+ SDK (TypeScript):
- Maintains an **in-memory queue** (`maxQueueSize` default 1000; oldest dropped when full)
- Flushes on a timer (`flushInterval` default 10s) OR when the queue reaches `flushAt` (default 20 events)
- `shutdown()` is awaitable — drains the queue before process exit
- Exponential backoff on network errors (from the feature-flag poller); the event pipeline retries on transient failures

**Configuration pattern worth borrowing:**

```typescript
const client = new PostHog(API_KEY, {
  flushAt: 20,           // batch size threshold
  flushInterval: 10_000, // ms between auto-flushes
  maxQueueSize: 1000,    // drop oldest when exceeded
});
// Await drain on shutdown
await client.shutdown();
```

### Borrowable Patterns for This Project

1. **Event schema**: our `AnalyticsEvent` schema closely mirrors PostHog's (name, timestamp, properties map). We can adopt the same field names for forward-compatibility if we ever add a PostHog adapter.
2. **Queue + flush timer**: `HttpAnalyticsProvider` should buffer events and POST them in batches. The existing `track()` in `telemetry.ts` does one `fetch` per event — replace with a queue draining to `/api/analytics` in batches.
3. **`$set` / `$set_once` distinction**: we don't need person properties (out of scope), but the pattern of differentiating "overwrite" vs "initialize-once" labels is useful for analytics metadata.
4. **`shutdown()` as `flush()`**: expose `flush(): Promise<void>` on the provider and call it from a `visibilitychange` / `pagehide` listener to drain before the browser tab closes.

Sources: [Capture API](https://posthog.com/docs/api/capture) | [Node.js SDK](https://posthog.com/docs/libraries/node) | [Event data model](https://posthog.com/docs/data/events)

---

## 3. Plausible Analytics — Privacy-First Event Model

### How Plausible Structures Events

Plausible's Events API (`POST /api/event`) is minimal by design:

```http
POST /api/event
Content-Type: application/json
User-Agent: Mozilla/5.0 …
X-Forwarded-For: <visitor-ip>

{
  "name":   "pageview",          // "pageview" is special; any other string = custom event
  "url":    "https://app.example.com/sessions",
  "domain": "app.example.com",
  "referrer": "https://app.example.com/",
  "props":  {                    // ≤30 key-value pairs; string values only
    "session_type": "new_worktree",
    "component": "Omnibar"
  }
}
```

No `distinct_id`, no user identity, no cross-request correlation. Unique visitor counting uses **IP + User-Agent hashing** (daily salt, never stored raw) — all PII is discarded before persistence.

### Privacy Patterns Applicable to Stapler-Squad

Stapler-squad's requirements say "no PII in events; session IDs are opaque identifiers only." The Plausible model gives us a blueprint:

| Plausible pattern | Stapler-squad equivalent |
|---|---|
| No persistent user ID | Session IDs are opaque UUIDs; no user identity |
| Props capped at 30 string values | `labels: Record<string, string>` map in `AnalyticsEvent` |
| All data isolated to a single day + single device | `created_at` indexed; retention policy (90d / 100k events) |
| `pageview` as a first-class event name | `event_category: "navigation"` + `event_name: "page_view"` |
| No cross-site or cross-device tracking | Local SQLite only; no external service |
| `url` field carries page context | `page` field in `AnalyticsEvent` schema |

**Key takeaway**: Plausible's minimal schema (name + url + props) is the floor. Our schema extends it with `duration_ms`, `component`, `session_id`, and `event_category` — all of which are operationally meaningful without being PII.

**Anonymous fingerprinting approach** (if we ever need session-scoped deduplication without identity):
```
daily_salt = SHA256(date + secret_key)
visitor_hash = SHA256(ip_address + user_agent + daily_salt)
```
The hash is stored, never the raw IP. Reset daily → no cross-day correlation.

Sources: [Events API](https://plausible.io/docs/events-api) | [Privacy policy](https://plausible.io/privacy-focused-web-analytics) | [GitHub](https://github.com/plausible/analytics)

---

## 4. ESLint Rules That Enforce Analytics Coverage

### Existing Art — No Dedicated Plugin Found

No widely-adopted npm package exists specifically for "enforce analytics at callsites." The pattern is consistently implemented as **project-local custom ESLint rules** using the standard `eslint.config.mjs` / `.eslintrc.json` plugin mechanism. VSCode's own codebase does this (see `microsoft/vscode` wiki on custom eslint rules).

The closest analog is `eslint-plugin-jsx-a11y` — it inspects JSX attributes (including `onClick`) using the same AST visitor pattern we need.

### Implementing Local ESLint Plugin

ESLint 8 / flat-config support a local plugin via `eslint-plugin-local-rules` (npm package, zero-dep) or by placing the plugin in a local `eslint-plugin-<name>/` directory and referencing it in config. The project already uses `eslint-plugin-boundaries` this way.

**Recommended project structure:**

```
web-app/
  eslint-plugin-analytics/
    index.js              # plugin entry: { rules: { ... } }
    rules/
      require-on-click.js
      require-omnibar-dispatch.js
      require-page-analytics.js
      require-rpc-analytics.js
    rules/__tests__/
      require-on-click.test.js
      …
```

**Rule skeleton (require-omnibar-dispatch):**

```javascript
// rules/require-omnibar-dispatch.js
module.exports = {
  meta: {
    type: "problem",
    docs: { description: "Each omnibar dispatch case must call track()" },
    schema: [],
  },
  create(context) {
    return {
      // Match: case "xxx": body inside dispatchOmnibarAction's switch
      SwitchCase(node) {
        const src = context.getSourceCode();
        // Check ancestor is the target switch; check body contains a CallExpression to track()
        const hasTrack = node.consequent.some(stmt => containsTrackCall(stmt));
        const hasExempt = src.getCommentsBefore(node).some(c =>
          c.value.includes('analytics-exempt')
        );
        if (!hasTrack && !hasExempt) {
          context.report({ node, message: "Add track() call or // analytics-exempt comment" });
        }
      }
    };
  }
};
```

AST explorer (astexplorer.net) is the standard tool for finding the right node types.

Sources: [ESLint Custom Rules](https://eslint.org/docs/latest/extend/custom-rules) | [ESLint Selectors](https://eslint.org/docs/latest/extend/selectors) | [local plugin pattern](https://medium.com/@ignatovich.dm/creating-and-using-custom-local-eslint-rules-with-eslint-plugin-local-rules-428d510db78f)

---

## 5. `no-restricted-syntax` vs Custom Plugin

### What `no-restricted-syntax` Can Do

`no-restricted-syntax` takes an array of `{ selector, message }` objects where `selector` is an [esquery](https://github.com/estools/esquery) CSS-like AST selector. It is already used heavily in this project (`.eslintrc.json` lines 28–58) to ban `100vh`, `100dvh`, and hardcoded hex values.

**Proven examples from the community:**

```json
// Ban setTimeout without second arg
{ "selector": "CallExpression[callee.name='setTimeout'][arguments.length!=2]",
  "message": "setTimeout must be called with two arguments." }

// Ban Array.prototype.reduce without initial value
{ "selector": "CallExpression[arguments.length=1][callee.property.name='reduce']",
  "message": "Provide an initialValue to .reduce()." }

// Enforce UTC usage
{ "selector": "CallExpression > Identifier[name='dayjs']",
  "message": "Use dayjs.utc() to avoid timezone issues." }
```

### Where `no-restricted-syntax` Falls Short for Analytics

| Requirement | `no-restricted-syntax` | Custom plugin |
|---|---|---|
| Ban a pattern that is MISSING (e.g., `onClick` without adjacent `track()`) | Cannot — it only bans present syntax | Required |
| Check that a `switch case` body contains a call | Cannot — it matches nodes, not their absence | Required |
| Exempt via comment (`// analytics-exempt`) | Not built-in — would need a separate `disable-` comment | Built-in via `context.getCommentsBefore()` |
| Cross-file or scope analysis | No | Possible but complex |
| Autofix | No | Supported via `fix()` in `meta` |

**Verdict**: `no-restricted-syntax` covers ~0% of the analytics rules in FR-3, because all four rules enforce the **presence of a call adjacent to another construct**, not the **presence of a forbidden pattern**. A custom local plugin is necessary for all four.

However, `no-restricted-syntax` is the right tool to **ban direct use of the old `track()` import** from `lib/telemetry.ts` once the new `useAnalytics().track()` API is in place:

```json
{
  "selector": "ImportDeclaration[source.value='@/lib/telemetry']",
  "message": "Use useAnalytics().track() instead of the legacy track() from lib/telemetry."
}
```

Sources: [no-restricted-syntax docs](https://eslint.org/docs/latest/rules/no-restricted-syntax) | [thoughtspile.github.io practical guide](https://thoughtspile.github.io/2021/06/02/eslint-restrict-syntax/) | [christopher.xyz examples](https://christopher.xyz/2021/05/16/eslint-ban-syntax.html)

---

## Summary of Recommended Patterns

### Interface design
Model `AnalyticsProvider` directly on OpenFeature's Provider contract: `initialize() / flush() / track()` with the same idempotency rules for `flush()` as OpenFeature's `shutdown()`.

### Event schema
Borrow PostHog's field names (`distinct_id` → not needed here; `event`, `timestamp`, `properties` map) and Plausible's privacy discipline (no PII, props capped at N key-value string pairs, page context via URL field, no persistent user identity).

### Queue + flush
Adopt PostHog's `flushAt` / `flushInterval` pattern in `HttpAnalyticsProvider`: buffer events in memory, POST in batches, expose `flush(): Promise<void>` called from a `pagehide` / `visibilitychange` listener.

### ESLint enforcement
All four FR-3 rules require a **local custom ESLint plugin** (`web-app/eslint-plugin-analytics/`); `no-restricted-syntax` cannot enforce "adjacent-call-required" patterns. Use `no-restricted-syntax` only for banning the legacy `lib/telemetry` import after migration.
