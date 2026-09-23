# Build vs. Buy — shipped-snapshot-display (Agent 6)

## Question

The original ask (map `shipped_*` backend fields into the frontend fallback display) is
already implemented and shipped on `main` (commit `aedd80648`, 2026-07-18). The only
candidate follow-up is a DOM-level regression test for the render path. Is any new
tool/library needed, or does the repo already have everything required?

## 1. Test infra already present (no new dependency)

`web-app/package.json` already has, and `web-app/jest.config.js` already configures:

- `jest` (`^30.2.0`)
- `ts-jest` (`^29.4.11`)
- `@testing-library/react` (`^16.3.0`)
- `@testing-library/jest-dom` (`^6.9.1`)
- `@testing-library/user-event` (`^14.5.2`)

No installation needed — "buy" here is free; it's just reusing what's wired up.

## 2. Existing component-test pattern already covers this exact path

Not just a nearby pattern to fork — **the DOM-level regression test already exists**:

- `web-app/src/components/shared/VcsWidget.test.tsx` (~lines 200–230) renders `<VcsWidget>`
  with `shipped: true` fixtures through RTL and asserts the fallback copy
  ("No history captured for this item — it shipped before detailed tracking was added.")
  in both `mode="full"` and `mode="compact"`.
- `web-app/src/components/shared/vcs-widget/VcsWidgetGithubRow.test.tsx` and
  `web-app/src/components/backlog/detail/VersionControlSection.test.tsx` both build
  `VcsWidgetData`-shaped fixtures (`makeData()` helpers) and render the real component,
  confirming the pattern (fixture builder + `render()` + `screen.getBy...`) is the
  established convention across this component family, not a one-off.

So the "fork and adapt" option isn't hypothetical — for the shipped-fallback path itself,
it's already done in `VcsWidget.test.tsx`.

## Recommendation

No external tool or library is warranted. "Reuse existing jest + RTL setup" is correct,
and there is nothing left to fork-and-adapt: `VcsWidget.test.tsx` already exercises the
rendered DOM for the `shipped` fallback in both display modes. If a gap remains, it would
be narrow (e.g. asserting on `shipped_snapshot_capture_failed` specifically, or the raw
`shipped_file_stats` rendering) and should extend `VcsWidget.test.tsx`'s existing
`makeData({ shipped: true, ... })` pattern — not introduce a new test file or dependency.
