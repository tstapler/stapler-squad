# ADR-014: Local ESLint Plugin for Analytics Coverage Enforcement

## Status
Accepted

## Context

FR-3 requires that four categories of callsites in the React frontend must always be instrumented with analytics:

1. `onClick` props on `<button>`, `<a>`, and `[role=button]` elements
2. Every `case` in the `dispatchOmnibarAction` switch statement
3. Top-level page/route components (files matching `app/**/page.tsx`)
4. Every call site of hooks returned by `useSessionService`

The enforcement mechanism must run in CI as part of `make quick-check` and block PRs that add un-instrumented callsites. All four rules must support a `// analytics-exempt: <reason>` escape hatch for intentional exceptions.

The features research (section 5) and stack research (section 2) explored the available enforcement mechanisms in detail. The project already uses ESLint's `no-restricted-syntax` rule extensively (`.eslintrc.json` lines 28–58) to ban `100vh`, `100dvh`, and hardcoded hex values. The question is whether that same mechanism is sufficient or whether a custom plugin is necessary.

The critical finding: **all four analytics rules enforce the _presence_ of an adjacent call, not the _presence_ of a forbidden pattern**. `no-restricted-syntax` can only match and ban syntax that is present in the AST; it cannot express "this node must have a sibling call expression of type X." A custom rule using `context.report()` only when a required pattern is absent is structurally impossible with `no-restricted-syntax`.

The project currently uses ESLint v9 (package.json `"eslint": "^9"`) with a legacy `.eslintrc.json` format. The local plugin must be compatible with that configuration.

## Decision

Implement the four analytics enforcement rules as a **local ESLint plugin** at `web-app/eslint-plugin-analytics/`, structured as a `file:` workspace package referenced from `web-app/package.json`.

**Package structure:**

```
web-app/
  eslint-plugin-analytics/
    index.js                          # plugin entry: { rules: { ... } }
    rules/
      require-on-click.js
      require-omnibar-dispatch.js
      require-page-analytics.js
      require-rpc-analytics.js
    rules/__tests__/
      require-on-click.test.js
      require-omnibar-dispatch.test.js
      require-page-analytics.test.js
      require-rpc-analytics.test.js
    package.json                      # { "name": "eslint-plugin-analytics", "main": "index.js" }
```

`web-app/package.json` references it as a local file dependency:

```json
{
  "devDependencies": {
    "eslint-plugin-analytics": "file:./eslint-plugin-analytics"
  }
}
```

`.eslintrc.json` enables the rules:

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

Rules are written in CommonJS (no build step needed) and tested with ESLint's `RuleTester`. This is compatible with both the legacy `.eslintrc.json` format and a future migration to `eslint.config.mjs` (flat config), where the plugin would be imported directly instead of referenced by name.

**Key implementation decisions for each rule:**

`require-on-click`: Uses the `'JSXAttribute[name.name="onClick"]'` AST selector. Checks `node.parent` (the `JSXOpeningElement`) for the element tag name and any `role="button"` sibling attribute. Critically, checks for a `JSXSpreadAttribute` sibling — if spread props are present, the rule is suppressed (spread may carry `onClick` that the AST cannot see) to avoid false positives on polymorphic button wrappers. The `// analytics-exempt` comment is read from `context.getSourceCode().getCommentsBefore(node.parent.parent)` (the `JSXElement`).

`require-omnibar-dispatch`: Matches `SwitchCase` nodes inside a function named `dispatchOmnibarAction`, supporting both `FunctionDeclaration` and `VariableDeclarator > ArrowFunctionExpression` forms so a future refactor from `function` to `const` arrow does not silently break the rule. Scans each `case` body recursively for a `CallExpression` where the callee matches `track` or `analytics.track`. Does not recurse into nested `SwitchStatement` nodes.

`require-page-analytics`: Gates on `context.getFilename()` matching `/app\/.*\/page\.tsx?$/` — the rule is a no-op for all other files. On matching files, checks the entire source AST body (not just the default export) for a call to `usePageView()` or `useAnalytics().track('page_view', ...)`.

`require-rpc-analytics`: Matches `CallExpression` nodes where the callee name matches hooks from `useSessionService` (detected via import source tracking). Walks up to the enclosing component function using scope analysis (`context.getScope().upper`), then checks whether any `track()` call exists within that same function scope — including inside `useCallback` closures. Excludes provider component files (`OmnibarContext.tsx`, `SessionServiceContext.tsx`) and custom hook files (`useSessionActions.ts`, `useSessionService.ts` itself) via a filename exclusion list to avoid false positives on infrastructure code that calls hooks but is not a UI component.

## Alternatives Considered

**`no-restricted-syntax` with AST selectors**

The project already uses `no-restricted-syntax` effectively for banning specific patterns (hardcoded colors, viewport units). It requires zero additional infrastructure — just JSON entries in `.eslintrc.json`.

Rejected for all four analytics rules because: `no-restricted-syntax` matches and reports on nodes that are _present_. All four rules need to report on nodes that are _missing_ an adjacent construct. For example, "an `onClick` attribute that lacks a sibling `track()` call in the enclosing function" cannot be expressed as an esquery selector that matches the `onClick` attribute alone — the selector would match every `onClick` attribute regardless of whether `track()` is present.

`no-restricted-syntax` remains useful for one complementary purpose: banning direct imports of the legacy `lib/telemetry.ts` module once `useAnalytics()` is in place:

```json
{
  "selector": "ImportDeclaration[source.value='@/lib/telemetry']",
  "message": "Use useAnalytics().track() from @/lib/analytics instead."
}
```

This single `no-restricted-syntax` rule will be added to `.eslintrc.json` alongside the plugin rules.

**TypeScript compiler plugin**

A TypeScript language service plugin or transformer (`ts-morph`, `ttypescript`) could enforce analytics at the type level — e.g., making `track()` calls required by the type signature of onClick handlers. Rejected because:
- TypeScript plugins run in the language service, not in CI lint pipelines, without additional tooling (`ts-patch`, custom build pipeline)
- Type-level enforcement of "this callback must call a specific function" requires encoding runtime behavior in the type system, which TypeScript does not support natively without complex conditional types
- The 4-rule scope is well within what ESLint's AST visitor API handles cleanly; adding a compiler plugin is significant infrastructure for this use case

**`eslint-plugin-local-rules` npm package**

The `eslint-plugin-local-rules` package allows defining rules inline in the ESLint config or in a project-local directory without creating a separate `package.json`. Rejected because:
- With four rules requiring independent test files using `RuleTester`, the `file:` workspace package provides cleaner organization (each rule in its own file, tests colocated)
- `eslint-plugin-local-rules` has limited support for ESLint v9 flat config; the workspace package approach works with both legacy and flat config formats
- A real `package.json` in `eslint-plugin-analytics/` makes the plugin's identity explicit in `npm ls` output and allows version tracking in the lock file

**Inline rules in `eslint.config.mjs` (flat config migration)**

ESLint v9's flat config format allows importing local plugins directly without the `file:` package indirection:

```javascript
import analyticsPlugin from "./eslint-plugin-analytics/index.js";
export default [{ plugins: { analytics: analyticsPlugin } }];
```

This is the cleanest long-term approach. However, migrating from `.eslintrc.json` to `eslint.config.mjs` mid-feature would expand scope significantly. The `file:` workspace package approach is forward-compatible — the plugin directory structure is identical; only the registration changes when the migration happens.

## Consequences

**Positive:**
- All four rules run as part of `make quick-check` via `next lint`, blocking PRs with un-instrumented callsites before merge
- Each rule is independently testable with ESLint's `RuleTester` — tests cover valid (instrumented), invalid (not instrumented), and exempt (with `// analytics-exempt` comment) cases
- The plugin is tracked in git under `web-app/eslint-plugin-analytics/` — rule changes have full diff history and code review
- Adding a fifth rule (e.g., for a new callsite category) requires adding one file under `rules/` and one entry in `index.js` — no changes to the main codebase ESLint config
- The `// analytics-exempt: <reason>` escape hatch is handled uniformly in each rule via `context.getSourceCode().getCommentsBefore()`, with the reason string preserved as documentation

**Negative / Trade-offs:**
- `npm install` must be run after any change to `eslint-plugin-analytics/package.json` to update the symlink in `node_modules/` — adding a step to the onboarding docs
- The `require-rpc-analytics` rule requires a hardcoded list of hook names from `useSessionService` and an exclusion list of provider/hook files; both lists drift silently if the service hooks are renamed or new provider files are added
- Initial enablement of the rules will require retrofitting `// analytics-exempt` comments on all currently un-instrumented callsites before the rules can run in CI without failing — this is a one-time migration cost

**Pitfall mitigations (from pitfalls research, section 1):**

- **Spread-props false positives** (`require-on-click`): The rule checks for `JSXSpreadAttribute` siblings on the same `JSXOpeningElement` and suppresses the error when found. This prevents false positives on every polymorphic `<Button as="div" {...rest}>` wrapper component. The trade-off is potential missed coverage when `onClick` arrives via spread; the documentation notes this limitation and recommends the `// analytics-exempt` comment on spread-prop wrappers with an explicit reason.

- **File-path gating** (`require-page-analytics`): The rule reads `context.getFilename()` and exits immediately for any file that does not match `app/**/page.tsx?`. This prevents false positives on shared components, layouts, and server components that happen to call `useAnalytics()`.

- **Provider and hook file exclusions** (`require-rpc-analytics`): A static exclusion list (`OmnibarContext.tsx`, `SessionServiceContext.tsx`, `useSessionActions.ts`) prevents the rule from firing on infrastructure code that calls `useSessionService` hooks in a non-component context. This list is documented in the rule source and must be updated when new provider files are added.

- **Arrow function detection** (`require-omnibar-dispatch`): The rule matches both `FunctionDeclaration[id.name="dispatchOmnibarAction"]` and `VariableDeclarator[id.name="dispatchOmnibarAction"] > ArrowFunctionExpression` so the rule continues to work if `dispatchOmnibarAction` is ever refactored from a named function declaration to a `const` arrow function assignment.
