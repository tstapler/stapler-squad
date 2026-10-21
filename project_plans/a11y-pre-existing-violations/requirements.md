# Requirements: a11y-pre-existing-violations

## Background

Story 7.8a extended `tests/e2e/a11y-route-sweep.spec.ts` to cover every `ROUTE_META` route with
WCAG 2.2 AA axe tags. Six routes had pre-existing serious/critical violations on the Next build.
They were added to the `KNOWN` allowlist (tagged `#1112`) rather than blocking CI. This project
removes each KNOWN entry by fixing the underlying violation.

## Violations to Fix (all tagged `#1112`)

| Route | Axe Rule | Nodes | Notes |
|---|---|---|---|
| `/analytics/escape` | `color-contrast` | 1 | `#tab-per_session` tab element |
| `/login` | `color-contrast` | 1 | Login card text using `vars.color.textMuted` / `textSecondary` |
| `/resolve` | `color-contrast` | 1 | DeepLink loading/error banner text |
| `/settings/backlog-sources` | `document-title` | — | Title is empty while `RequireFeatureFlag` redirects (flag=off path) |
| `/test/escape-codes` | `label` | 82 | Form controls lack associated `<label>` elements |
| `/test/escape-codes` | `select-name` | 4 | `<select>` elements missing accessible name |
| `/test/terminal-stress` | `color-contrast` | 22 | Multiple colored stat/metric elements |
| `/test/terminal-stress` | `select-name` | 1 | `<select>` missing accessible name |

## Scope

- Fix all 8 violation instances across the 6 routes
- Remove each corresponding `KNOWN` entry in `a11y-route-sweep.spec.ts` once fixed
- Do NOT add new KNOWN entries (ratchet policy: add only for violations present in Next build)
- `/test/*` pages are developer debug tools; minimal accessible UI is still required per WCAG AA

## Out of Scope

- Pre-existing KNOWN entries without `#1112` tag (those are separate debt)
- Theme-wide color token audit (fix only the specific failing nodes)
- Rewriting the test pages from scratch

## Acceptance Criteria

1. The axe sweep passes without any KNOWN entry for `/analytics/escape`.
2. The axe sweep passes without any KNOWN entry for `/login`.
3. The axe sweep passes without any KNOWN entry for `/resolve`.
4. The axe sweep passes without any KNOWN entry for `/settings/backlog-sources`.
5. The axe sweep passes without any KNOWN entry for `/test/escape-codes`.
6. The axe sweep passes without any KNOWN entry for `/test/terminal-stress`.
7. All 6 route entries tagged `#1112` are removed from the `KNOWN` map in `a11y-route-sweep.spec.ts`.
8. `make quick-check` passes (build + test + lint).
9. The a11y route sweep E2E test passes without introducing any new KNOWN violations.
