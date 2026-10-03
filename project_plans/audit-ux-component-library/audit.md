# UX audit and component library

Method: each of the 39 `web-app/src/app/**/page.tsx` routes and its delegate component was read by a
read-only review pass (findings cite `file:line` under `web-app/src/`); I re-read the code for every
finding I fixed. Not covered: tab bodies of `/settings` (e.g. `ProfilesManager`), the internals of
`/account`'s modals, loading/empty states in `/sessions/import` children. Nothing was run in a
browser; verification is jest + `tsc` + `build-storybook`.

## Fixed in this change (each with a local test)
| Route | Problem | Fix | Test |
|---|---|---|---|
| `/help` | Docs fetch failure swallowed; page ended on "Select a topic" with no error (`app/help/page.tsx`) | `role=alert` error + Retry | `app/help/__tests__/page.test.tsx` |
| `/sessions/summary` | Missing `sessionId` rendered a blank page | Explanatory status message | `app/sessions/summary/__tests__/page.test.tsx` |
| `/account` | Add-device input had only a placeholder | `aria-label="Device name"` | `app/account/__tests__/page.test.tsx` |
| `/history` | Error banner not announced | `role="alert"` | `app/history/__tests__/page.test.tsx` |
| History fork modal, Trigger form | Directory fields were plain inputs | `RepoPathInput` | `TriggerFormModal.test.tsx` (existing suite, now with hook mocks); guard test |
| `RepoPathInput` | No accessible-name or Enter hook, so migrated fields lost `aria-label`/Enter-to-add (found in verify) | new `aria-label` + `onEnter` props | `UnfinishedSourcesSettings.test.tsx` (name + Enter) |
| `/settings/unfinished` | Pinned-repo field was a plain input | `RepoPathInput` | `UnfinishedSourcesSettings.test.tsx` |
| Settings → Aliases | Alias path was a plain input | `RepoPathInput` | `AliasesManager.test.tsx` |

## Path fields (AC3)
| Field | Status |
|---|---|
| Session/backlog/workflow/shell/Jules/global-defaults/directory-rules/watch-dir/file-browser | already `RepoPathInput` (10 consumers) |
| Alias path, pinned repo | **migrated** here |
| `AddRemoteForm` base path (`add-remote-base-path`) | **Exception**: path on the *remote* host; `RepoPathInput` completes local filesystem paths and would suggest wrong dirs |
| `ProgramsManager` "Executable Command / Path" | **Exception**: a command line (may include args/PATH names), not a directory |
| Omnibar title, `WorkspaceSwitchModal` filter | **Exception**: not path values (guard-test allowlist, with reasons) |
| `OmnibarCreationPanel` working dir | **Exception**: relative subdirectory of the chosen repo |
Guard: `components/ui/__tests__/pathInputGuard.test.ts` fails on any new `<input>` whose id/placeholder/aria-label looks path-like unless it is in its `ALLOWLIST`. It is a heuristic over attribute text, so a path field with no path-like attribute would escape it.

## Shared-component adoption (AC4) — recorded as follow-up
Counts from `grep` over `web-app/src` excluding tests/stories:
- `Button`: 3 importing files vs 217 files containing raw `<button`.
- `Input`: 0 importing files vs 89 files containing `<input`.
- `Modal`: 3 users vs 37 files with hand-rolled `role="dialog"`/`aria-modal`; `/`, `/history`, `/backlog`, `/review-queue` hand-roll modals (focus trap/return inconsistent).
- `ErrorState`: no page uses it. `Skeleton`: insights + one backlog component; most pages use a bare "Loading…" div.
- Many error banners lack `role=alert` (`/errors`, `/login`, `/workflows`, `/insights`, `/settings/backlog-sources`, `/rules`, `/files`, `CallbackSettings`).
Follow-ups (not done: each is a broad mechanical migration needing its own review): migrate hand-rolled modals to `Modal`;
adopt `Skeleton`/`ErrorState`/`InlineNotice` for loading/error states; migrate `<input>`/`<button>` to `Input`/`Button`;
add a lint rule against raw dialogs.

## Per-page findings (AC0)
Severity: H/M/L. "Fixed" = see table above; otherwise open follow-up.
| Route | Findings |
|---|---|
| `/` | Hand-rolled delete modal, no focus trap/return (`app/page.tsx:566-606`) M; `error` from hook (`:93`) has no obvious render — verify M; no h1 L |
| `/backlog` | Hand-rolled create modal (`:870-878`) M; no disconnected-stream notice (`:244-248`) M; import error `<p>` lacks role=alert (`:936-939`) M; dead `{false && …}` import form (`:944-957`) L; inline-styled loading (`:732`) L. Row keyboard/aria-sort: good |
| `/backlog/board` | No Suspense fallback (`:170`), no h1/main L |
| `/review-queue` | Help overlay hand-rolled without focus trap (`:391-402`) M; skeleton good |
| `/history` | Error banner role — **fixed**; resume dialog hand-rolled + plain input (`:405-418`) M; `${err}` stringified (`:81,94,108,157`) L |
| `/logs` | Good semantics; tabs lack aria-controls/tabpanel (`:122-143`) L |
| `/help` | Silent fetch failure — **fixed**; text-only loading L; root `div#main-content` not `<main>` L |
| `/errors` | Error div unroled (`ErrorDashboard.tsx:89-91`) M; refresh has no busy state L; empty `<th>` L |
| `/files` | Errors styled as empty states, no role (`LocalFileBrowser.tsx:425,458`) M; bare loading L |
| `/notifications` | Native `window.confirm` for Clear read (`:206`) M; emoji loading L |
| `/rules` | Rules panel error no role=alert (`ApprovalRulesPanel.tsx:359-363`) M; bare Suspense fallback L; no h1 L |
| `/triggers` | `CallbackSettings` load error no role=alert (`:104-105`) L; no h1 L |
| `/workflows` | Load error unroled (`WorkflowsPanel.tsx:202`) M; no Suspense fallback L |
| `/sessions/import` | commit error has role=alert; children not audited |
| `/sessions/new` | redirect shim, fine |
| `/sessions/summary` | Missing id — **fixed**; strong phased states |
| `/resolve` | Good; loading div lacks role=status L |
| `/login` | Error `<p>` no role=alert (`:104-106`) M |
| `/account` | Unlabelled input — **fixed**; errors lack role=alert (`:104,338-339`) M; hand-rolled modals, 10 raw buttons M; no h1 L |
| `/config`, `/settings/defaults` | redirects (defaults drops tab context) |
| `/analytics/escape` | Good; tabpanel `aria-labelledby` hard-coded to `tab-per_session` (`:146`) — verify L |
| `/insights` | Fetch error shown twice, unroled (`InsightsDashboard.tsx:171,176,182`) M; best loading handling |
| `/insights/session-detail` | Good |
| `/settings` | Active tab not reflected in `?tab=` (`page.tsx:32,38`) M; bare fallbacks L |
| `/settings/backlog-sources` | Page error unroled (`BacklogSourcesSettings.tsx:305-311`) M |
| `/settings/backlog-stages`, `/pipeline-modes`, `/remotes`, `/jules`, `/tagging-classifier` | Good a11y/error handling; plain-text loading L; redundant list roles in tagging-classifier L |
| `/settings/features` | "Please refresh" error with no Retry (`:100-107`) M |
| `/settings/unfinished` | Path field **fixed**; load failure bare text, no retry (`UnfinishedSourcesSettings.tsx:19-20`) M |
| `/unfinished` | "All repos are clean" shown before scan/on failure (`UnfinishedTab.tsx:168-171`) M; h1 "Up Next" vs title mismatch L |
| `/debug/escape-codes`, `/test/escape-codes`, `/test/layout-overlap`, `/test/terminal-stress` | Dev/test fixtures; unlabelled controls in test fixtures L. Whether `/test/*` ships in production builds is unchecked |

## Component library (AC2)
`pnpm storybook` (port 6006) / `pnpm build-storybook` in `web-app/`. New stories: Button, Badge, Card, Input,
Skeleton, Tooltip, RadioGroup, ErrorState, Modal, RepoPathInput, InlineNotice (plus the 3 existing).
`web-app/src/components/ui/__tests__/storyCatalog.test.tsx` renders every story and fails if a component in
`components/ui` or `components/common` has neither a story nor a reasoned entry in its `UNCATALOGED` map.

## Usage report (reproducible)
`pnpm run report:component-usage` (script: `web-app/scripts/component-usage-report.mjs`) lists consumers per component in
`components/ui` + `components/common`; output saved in `component-usage.md`. Flags UNUSED: `Card`, `Input`, `Navigation`
(no production importers; `Card`/`Navigation` cross-checked with grep). No duplicate component names found.

## Verification (sdd:6-verify)
- Layer 1 (idiom) and Layer 2 (architecture) review agents ran on the diff. Fixed: pinned-repo/watch-dir a11y + Enter regression
  (new `RepoPathInput` props), RadioGroup story hooks, wrong `useGitHubEnterpriseHosts` mock shape, vacuous story-render assertion,
  `UNCATALOGED` ratchet (max 25), digit-bearing component filenames. Not done: shared hook-mock helper, per-story Redux store,
  moving Provider decorators into `.storybook/preview.tsx` (would let several `UNCATALOGED` entries be cataloged).
- Layer 3: jest 498 suites / 6000 tests pass; `tsc --noEmit` clean; `lint:duplicates` exit 0; `build-storybook` exit 0.
  `next lint` exits 1 with 5 `analytics/*` errors in `insights/session-detail/page.tsx`, `RestartWithSummaryButton.tsx`,
  `SessionBoard.tsx`, none touched by this branch.
- Layer 4 / Playwright a11y specs: not run (needs the Go binary + browsers). Storybook dev server not launched.
- Stories now exist for every component in `components/ui` + `components/common` (38 story files; `UNCATALOGED` is empty, cap 0).
  Some stories are narrower than default/disabled/error/edge: Navigation has a Default story only (feature flag context not exported);
  FlagCombobox/AutocompleteInput show the closed list state; null-rendering components carry a text label. Seen only under jest
  and `build-storybook`, not in a browser.
- Open against the backlog list: bulk migration of raw button/input/modal (recorded above as follow-up); screenshots as before/after evidence.
