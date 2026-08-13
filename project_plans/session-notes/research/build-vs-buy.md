# Research: Build vs. Buy — Session Notes Markdown Editor

## Question

Should "attach a markdown note to a session" (plain-text edit + rendered
markdown preview, single field, no revisioning/attachments per
`requirements.md` non-goals) be built from scratch, sourced from an existing
OSS component, or adapted from an existing pattern already in this codebase?

## 1. Existing OSS rich markdown editor component

Candidates considered: `@uiw/react-md-editor`, `react-mde`, `tiptap` (+
markdown extension), `easymde`/SimpleMDE.

Checked `web-app/package.json` — **none of these are installed**. What *is*
already installed and relevant:

- `react-markdown@^10.1.0` + `remark-gfm@^4.0.1` — markdown-to-React renderer,
  already used in three places (`web-app/src/app/help/page.tsx`,
  `web-app/src/components/backlog/BacklogItemForm.tsx`,
  `web-app/src/components/sessions/SessionSummaryPanel.tsx`,
  `web-app/src/components/backlog/detail/DescriptionSection.tsx`).
- `@monaco-editor/react@^4.7.0`, `codemirror@^6.0.2` +
  `@codemirror/lang-markdown@^6.5.0` — present in `package.json`, but grepping
  actual usage (`FileContentViewer.tsx`, `app/config/ConfigPageContent.tsx`)
  shows they're wired up for **file/JSON/config editing**, not markdown notes.
  No existing call site treats markdown notes as a code-editor surface.

Pros of a rich editor library (`@uiw/react-md-editor` etc.):
- Toolbar (bold/italic/link buttons), live split-pane preview, syntax
  highlighting in the edit pane.
- Nicer authoring UX for longer notes.

Cons:
- New dependency (bundle size, maintenance surface, another vanilla-extract
  theming integration to get its toolbar/preview to match the app's dark/light
  theme — these libraries ship their own CSS that fights `.claude/rules/css-architecture.md`'s
  token system).
- Solves a problem the requirements doc explicitly scopes out: this is a
  single free-form field for short reminders ("left this waiting on X"), not
  a document-authoring surface. No revisioning, no attachments, no titles —
  v1 is intentionally minimal.
- Mobile UX (`feedback_mobile_desktop_ux.md` instinct) is harder to get right
  with a toolbar-heavy component than a plain `<textarea>`; split-pane preview
  doesn't fit small viewports without extra work.

**Verdict: Not recommended.** No new editor dependency is justified for this
scope.

## 2. SaaS / managed API

Not applicable — this is locally-persisted app data (one markdown field on a
session row in the existing ent/SQLite store per Goal 4), not content that
benefits from an external service. No further evaluation needed.

**Verdict: Not recommended (N/A).**

## 3. LLM-generated implementation vs. battle-tested library — markdown rendering specifically

For the **render** half (markdown → HTML), hand-rolling a parser/renderer
would mean reimplementing GFM tables, code fences, link handling, and XSS-safe
output — a well-known battle-tested-library problem (regex-based markdown
parsers are a classic source of both bugs and injection vulnerabilities).
`react-markdown` is already a direct dependency, already used for an
almost-identical case (`DescriptionSection.tsx` renders `item.description`
with `<ReactMarkdown remarkPlugins={[remarkGfm]}>`), and the requirements doc
explicitly calls this out: "react-markdown is already a dependency (per
source issue) — reuse it rather than adding a new markdown renderer."

**Verdict: Recommended — reuse `react-markdown` + `remark-gfm` for the render
half. Do not hand-roll markdown parsing.**

For the **edit** half (textarea vs. library), applying the "does stdlib/native
do it" ladder: a plain `<textarea>` is native HTML, requires zero dependencies,
handles mobile keyboards correctly by default (no custom key handling to get
wrong), and is exactly what the existing `NotesSection.tsx` (backlog item
notes) already uses for a structurally identical "click to edit free text,
save/cancel" flow — the only difference is that component doesn't currently
render its output as markdown (backlog notes are plain text), whereas session
notes need the render step. Given the explicit non-goals (no revisioning, no
attachments, single field, "simple start" per the source issue), a plain
textarea + toggle to a `ReactMarkdown` preview is sufficient and matches
existing UI conventions in this codebase.

**Verdict: Recommended — plain `<textarea>` for editing, no editor library
needed.**

## 4. Fork or adapt — existing reference implementation in this codebase

Two components in `web-app/src/components/backlog/detail/` together are
almost exactly this feature, split across two files:

- **`NotesSection.tsx`** — the edit/save/cancel interaction pattern: a
  `<textarea>` in edit mode (`styles.notesTextarea`, `data-testid`s,
  `aria-label`), a click-to-edit `<p>` in display mode (keyboard-accessible
  via `role="button"`/`onKeyDown`), Save/Cancel buttons with
  `actionLoading`/`aria-busy` pending-state handling. This is currently
  **plain-text** display (`{item.notes ?? "Click to add notes…"}`), not
  markdown-rendered.
- **`DescriptionSection.tsx`** — the markdown-render half:
  `<ReactMarkdown remarkPlugins={[remarkGfm]}>{item.description}</ReactMarkdown>`
  wrapped in `markdownStyles.markdownBody`, with an empty-state fallback
  (`styles.emptyText`).
- **`web-app/src/components/backlog/markdownBody.css.ts`** — an existing
  vanilla-extract stylesheet for rendered markdown (paragraph spacing, link
  color, code/pre blocks, image sizing) already wired to the shared
  `theme.css.ts` token contract. Directly reusable/extendable rather than
  writing new markdown CSS from scratch.

`herdr-web`'s reference (`PaneNote` model, mentioned in the requirements doc's
non-goals) was checked via repo grep — no `herdr-web` source is vendored in
this repo; only prior `project_plans/*/research/*.md` docs reference it by
name (e.g. `project_plans/detector-plugins/research/build-vs-buy.md`). It's
not available to adapt from directly and isn't needed given the in-repo
pattern above already covers the required shape (edit/display toggle) and the
non-goals explicitly exclude the multi-note/revisioning features that made
`PaneNote` more complex.

**Verdict: Recommended — adapt `NotesSection.tsx`'s edit/save/cancel
interaction pattern (add a markdown-render display state, replacing the plain
`<p>` with `DescriptionSection.tsx`'s `ReactMarkdown` + `markdownBody.css.ts`
approach) rather than designing a new interaction pattern from scratch.**

## Summary Table

| Option | Pros | Cons | Verdict |
|---|---|---|---|
| Rich markdown editor lib (`@uiw/react-md-editor`, `react-mde`, `tiptap`) | Toolbar, live preview, nicer long-form authoring | New dep, CSS theming fight, over-scoped for a single reminder field, worse mobile fit | **Not recommended** |
| SaaS/managed API | — | N/A — local persisted app data | **Not recommended (N/A)** |
| Hand-rolled markdown renderer | — | Reimplements GFM/XSS-safety that `react-markdown` already solves; requirements doc explicitly says reuse it | **Not recommended** |
| Plain `<textarea>` + `react-markdown` preview toggle | Zero new deps, native mobile keyboard behavior, matches existing conventions | Less rich authoring than a dedicated editor (acceptable per non-goals) | **Recommended** |
| Adapt `NotesSection.tsx` + `DescriptionSection.tsx` pattern | Interaction pattern, a11y (keyboard edit trigger, `aria-busy`), and CSS (`markdownBody.css.ts`) all already exist and are tested in production | None significant — need to add a display-mode markdown render to the notes pattern, which is a small merge of the two existing components | **Recommended** |

## Bottom line

Build on existing in-repo primitives, don't add a dependency: plain
`<textarea>` for editing (mirroring `NotesSection.tsx`'s interaction pattern)
+ `react-markdown`/`remark-gfm` for the read-mode render (mirroring
`DescriptionSection.tsx`, reusing `markdownBody.css.ts` for styling). No new
npm package is needed for this feature.
