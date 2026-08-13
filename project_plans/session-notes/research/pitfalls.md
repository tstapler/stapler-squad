# Pitfalls Research: Session Notes (markdown note on Session)

## 1. Markdown rendering / XSS

**Established safe convention exists — follow it exactly.** Every current
`react-markdown` usage in this codebase renders plain markdown with
`remarkGfm` only, and critically **none** pass `rehypePlugins={[rehypeRaw]}`
or otherwise re-inject raw HTML:

- `web-app/src/components/backlog/detail/DescriptionSection.tsx:27`
- `web-app/src/components/backlog/BacklogItemForm.tsx:536`
- `web-app/src/components/sessions/SessionSummaryPanel.tsx:460`
- `web-app/src/app/help/page.tsx:97-99`

`react-markdown`'s default behavior (no `rehype-raw`) strips/escapes raw
HTML tags rather than rendering them, so these call sites are already safe
against injected `<script>`/`<img onerror>` etc. without any extra
sanitization step. `rehype-raw` and `dangerouslySetInnerHTML` do exist
elsewhere in the codebase, but only for unrelated concerns — a `<script>`
FOUC-prevention snippet in `web-app/src/app/layout.tsx:56` and pre-rendered
syntax-highlighted HTML in `FileContentViewer.tsx:403` and
`SessionCard.tsx:833` (terminal snapshot HTML, not markdown). Neither is a
markdown-rendering path.

**Pitfall to avoid:** do not add `rehype-raw` "to support richer notes" —
it would be the first instance of raw-HTML pass-through for user-authored
markdown in this codebase and reintroduces XSS risk the moment a user (or
anything pasting external content) puts `<img src=x onerror=alert(1)>` in a
note. Match the existing pattern: `<ReactMarkdown remarkPlugins={[remarkGfm]}>`,
no `rehypePlugins`, styled via the shared `markdownBody.css` (already used
by `DescriptionSection.tsx` and `SessionSummaryPanel.tsx` — reuse it rather
than inventing new markdown-body CSS).

## 2. Save-on-blur vs explicit save UX

**No autosave/save-on-blur convention exists in this codebase today** —
searched for it explicitly (`onBlur` handlers, `useDebounce` usages) and
found no precedent for "edit a text field → autosave on blur/debounce."
`useDebounce.ts` (`web-app/src/lib/hooks/useDebounce.ts`) exists but is used
for search/autocomplete inputs, not for persisting form state. The closest
analog, `BacklogItemForm.tsx`'s markdown description field, uses an
**explicit submit** model (write/preview tabs, save via form submit button)
— not autosave.

Because there's no established save-on-blur pattern to reuse, the plan
should either:
- Follow `BacklogItemForm`'s explicit-save convention (edit → explicit
  "Save" action, e.g. a button or Cmd/Ctrl+Enter) — lowest risk, most
  consistent with existing UX, and avoids inventing new failure modes, **or**
- If autosave-on-blur is chosen anyway (goals doc doesn't require it),
  explicitly design for:
  - **Lost edits on navigation away**: blur doesn't fire on tab-close,
    hard navigation, or session switch via keyboard shortcut — a user who
    types a note and immediately closes the session detail panel (common
    workflow here, since sessions are frequently switched) can lose the
    edit if save is blur-only and the panel unmounts before the request
    resolves. Needs a `beforeunload`/unmount-triggered flush, not just
    `onBlur`.
  - **Race between autosave and manual navigation**: if a save RPC is
    in-flight when the user re-opens the note (e.g. rapid session-switch
    back), a stale response could overwrite newer local edits — need to key
    saves off session ID and ignore stale responses (compare session ID at
    resolve time, not just fire-and-forget).
  - **Race between two open tabs/panels**: this repo explicitly scoped out
    optimistic-concurrency revisioning (`expected_revision` guard) as a
    non-goal, accepting last-write-wins — fine per requirements, but note
    it explicitly in the plan so it isn't "discovered" as a bug later.

Given the non-goal already accepts last-write-wins with no revisioning, the
simplest and most consistent choice is explicit-save (matches
`BacklogItemForm`), not autosave — recommend defaulting to that unless the
plan phase decides otherwise.

## 3. ent schema migration pitfalls

**This repo uses ent's auto-migration, not versioned/generated migration
files.** Confirmed via:

```
server/analytics/db.go:46:  client.Schema.Create(ctx)
session/ent_repository.go:93: client.Schema.Create(context.Background())
```

`client.Schema.Create()` on startup diffs the current struct definitions
against the live SQLite schema and applies `ALTER TABLE`s automatically —
there is no `migrations/` directory or versioned migration runner to also
update. This means:

- Adding `field.String("notes").Optional()` to
  `session/ent/schema/session.go` is sufficient on the schema side; no
  separate migration file is needed.
- **Must be `.Optional()`** (and ideally `.Default("")` per the existing
  convention seen on `uuid`, `github_pr_url`, `session_artifacts`, etc. in
  the same file) — a non-optional/`NotEmpty()` field on a table with
  existing rows will fail auto-migration or (worse) silently produce
  invalid rows, since `Schema.Create` has no interactive backfill step. Model the new field directly on the pattern already
  used for `session_artifacts` (`field.String("session_artifacts").Optional().Default("")...`)
  — same shape: optional, empty-string default, free-form persisted blob.
- Regeneration command is mandatory and must use the flag from
  `.claude/rules/ent-schema-generation.md`:
  `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema`
  — omitting `--feature sql/upsert` silently breaks existing `UpsertRule`-style
  methods elsewhere in the generated code even though it compiles.
- `session/ent/schema/session.go` is only 179 lines (not the "~490+ lines"
  the requirements doc estimated — that figure likely refers to a different
  file, e.g. `session/instance.go` or the generated `session/ent/session.go`).
  Regardless of line count, `session.go`'s field list (18-133) is long and
  flat; add the new field near the other free-form/optional string fields
  (e.g. near `session_artifacts` at line 128) rather than at the end, to
  keep related fields grouped.
- After regen: `go build ./...` and commit all changed files under
  `session/ent/` together (generated code), per the existing repo
  convention referenced in `CLAUDE.md`'s ent-schema section.

## 4. CI / registry pitfalls specific to this repo

- **`docs/registry/features/`**: this is a new backend RPC (e.g.
  `UpdateSessionNote`) and a new frontend feature (the note panel/editor).
  Both need new per-feature JSON files — pattern to follow:
  `docs/registry/features/backend/<feature>.json` and
  `docs/registry/features/frontend/<feature>.json` (see existing files like
  `docs/registry/features/frontend/backlog-category-selector.json` for
  shape). Must add `// +api: session:update-note`-style marker in the Go
  handler and `// +feature: session-note-panel`-style marker in the first
  10 lines of the new React file (see `SessionSummaryPanel.tsx:2`'s
  `// +feature: session-summary-tab` for the exact convention), then run
  `make registry-generate` and commit the changed per-feature + aggregated
  files.
- **e2e test required**: per `.claude/rules/e2e-test-conventions.md`, needs
  a new spec (e.g. `tests/e2e/session-notes.spec.ts`) with a `// @feature
  session:update-note, session-note-panel`-style header comment, no
  `waitForTimeout`, and `data-testid`/ARIA-role locators only (e.g.
  `data-testid="session-note-editor"`, `data-testid="session-note-rendered"`,
  `data-testid="session-card-note-indicator"`). New page-interaction helpers
  belong in `tests/e2e/pages/`, not inlined in the spec.
- **7-touchpoint session-creation registry does NOT apply.** Confirmed by
  reading `.claude/rules/session-creation-registry.md` in full: it governs
  adding a *new session creation mode* (proto `SessionType` enum, creation
  handler switch, `Omnibar.tsx` radio group, etc.). A note field is a
  property of an *existing* session, edited from the session detail view
  after creation — it doesn't touch `CreateSessionRequest`, the
  `SessionType` enum, or any of the 7 touchpoints. This is a plain "new API
  endpoint" + "new UI feature" per the `CLAUDE.md` "Adding New Features"
  section (proto RPC → `make proto-gen`; handler in
  `server/services/session_service.go` or a sibling service file), not a
  creation-mode change. No action needed against that rule — just note
  this in the plan so a reviewer doesn't ask for it.
- **Omnibar/detector registries also don't apply** — per
  `.claude/rules/feature-testing-registry.md`'s own decision tree, this
  isn't a user-triggerable omnibar action or an auto-detected input
  pattern; it's UI reachable only from the session detail view. Skip both
  `OmnibarAction` and `DetectorRegistry` changes.
- `make registry-generate` output (`coverage-gaps.json`) must not grow net
  new gaps — the new backend/frontend entries need `tested: true` +
  populated `testIds` once the e2e test (and any Go/Jest unit tests) land,
  not left `tested: false`.

## 5. vanilla-extract CSS pitfalls

Per `.claude/rules/css-architecture.md`, any new component (note editor
panel, read-mode markdown display, `SessionCard` indicator) must use a
colocated `.css.ts` file with `style`/`recipe` from `@vanilla-extract/css`,
importing tokens from `vars` (`web-app/src/styles/theme-contract.css.ts` /
`theme.css.ts`) — no new `.module.css` files, no `var(--undefined-token)`
strings.

Concretely for this feature:

- **Reuse `markdownBody.css.ts`** (`web-app/src/components/backlog/markdownBody.css.ts`,
  already imported by `DescriptionSection.tsx` and `SessionSummaryPanel.tsx`
  as `import * as markdownStyles from "@/components/backlog/markdownBody.css"`)
  for the rendered-markdown display, rather than writing new markdown-body
  CSS — this is the established shared style for `ReactMarkdown` output in
  this repo.
- **`SessionCard.tsx` indicator**: follow the existing badge pattern already
  in that file (`ReviewQueueBadge`, `GitHubBadge`, `autonomousBadge`,
  `memoryBadge` — see `SessionCard.tsx:44-89` imports) — a small badge
  component or `className` variant, not an inline `style={{}}` (inline
  layout styles are explicitly disallowed by the CSS rule; use `data-*` +
  `selectors` if state-driven styling is needed).
- **No hardcoded `zIndex`** if the editor is any kind of overlay/expanding
  panel — must add a named slot to the shared `zIndex` contract in
  `theme-contract.css.ts` and reference `zIndex.xxx`, per the explicit
  "Never Do" list in the CSS rule.
- **No `position: fixed`/`absolute` overlay without `createPortal`** if the
  note editor is presented as a modal/popover rather than inline in the
  session detail panel — required per the same rule to avoid breaking under
  transformed ancestors.
- **Scroll convention**: if the note panel lives in a scrollable section of
  the session detail view, check whether that container already opts into
  the `height: "100%"` + `overflowY: "auto"` pattern
  (`.claude/rules/css-architecture.md`'s "Page Scroll Convention") or
  whether the new panel needs its own scroll region for long notes.
- **Mobile/desktop**: per `feedback_mobile_desktop_ux.md` (memory), the note
  editor's textarea needs adequate touch target sizing and must not trigger
  layout issues when the mobile on-screen keyboard opens (a fixed-height
  panel with a textarea at the bottom of viewport is the common failure
  mode — verify the panel scrolls the textarea into view rather than being
  covered by the keyboard).

## Summary of key file references

| Concern | File |
|---|---|
| Safe markdown pattern to copy | `web-app/src/components/backlog/detail/DescriptionSection.tsx:27` |
| Shared markdown body CSS | `web-app/src/components/backlog/markdownBody.css.ts` |
| Explicit-save form analog | `web-app/src/components/backlog/BacklogItemForm.tsx` (description field, lines ~474-540) |
| ent schema to edit | `session/ent/schema/session.go` (add near `session_artifacts`, line 128) |
| Auto-migration call site | `session/ent_repository.go:93` |
| `// +feature:` marker convention | `web-app/src/components/sessions/SessionSummaryPanel.tsx:2` |
| SessionCard badge patterns to follow | `web-app/src/components/sessions/SessionCard.tsx:44-89, 461-595` |
| Session-creation 7-touchpoint rule (confirmed N/A) | `.claude/rules/session-creation-registry.md` |
| e2e conventions | `.claude/rules/e2e-test-conventions.md` |
| CSS architecture rules | `.claude/rules/css-architecture.md` |
