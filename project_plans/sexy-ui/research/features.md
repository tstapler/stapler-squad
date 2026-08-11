# Features Research: stapler-squad UI Redesign

_Research date: 2026-05-14_

---

## 1. Dense List Views in Developer Tools

### Reference Implementations

**Linear issue list**
Linear's 2024 redesign (documented at linear.app/now/how-we-redesigned-the-linear-ui) is the canonical reference for the stapler-squad target aesthetic. Key decisions from their redesign:
- Tabs and headers were made more compact — rounded corners, smaller icon sizing, no full-width spans
- Icon usage was reduced, scale was brought down, colored background treatments removed
- The current view, available actions, and meta properties are presented with less visual noise
- Information density is preserved without the interface feeling overwhelming — their phrase is "calm without losing density"
- Row interactions: hover reveals inline actions; no persistent action chrome per row

**Material Design density scale**
Three tiers: default, comfortable, compact. Compact density is recommended for data-rich applications where more information must fit in less vertical space. Target: 32–36px per row for compact density.

**VS Code Explorer panel**
- File rows are 22px tall at default density (single icon + label, no metadata)
- Key insight: metadata (modified date, size) is relegated to hover tooltip, not visible in the row
- Active/focused row uses a high-contrast background band, not border decoration
- Selection is shown via background color only — no checkboxes, no wasted left gutter

**GitHub PR list**
- Rows are ~52px (two lines: title + metadata). Denser than cards but looser than Linear.
- Inline status badges (Draft, Open, Merged) are colored dots at the row's left edge
- Actions (Review, Close) appear only on hover as a right-aligned icon cluster

**Raycast result list**
- ~40px per row: icon (16px) + label + subtitle, all on one line
- Right side shows keyboard shortcut hint or action label on hover
- Active row is highlighted with a 4px rounded-rect background, not a full-width band

### Recommendations for REQ-2 (Session List)

**Row structure (36–40px target)**
```
[8px] [status dot 8px] [12px] [branch/name bold 14px flex-1] [agent icon 16px] [8px] [path mono 12px 200px truncate] [8px] [elapsed 11px muted] [→ hover actions]
```

- Status dot: 8px circle, no ring/border — colored fill only (green=#22c55e running, amber=#f59e0b paused, slate=#475569 idle)
- Branch/name: 14px Inter semibold, truncate with ellipsis. Highest visual weight in the row.
- Agent icon: 16px monochrome icon (Claude, Aider, etc.), muted color until hover
- Path: 12px JetBrains Mono, #475569 muted, max-width ~220px, right-truncated with ellipsis
- Elapsed: 11px, #475569, right-aligned — consider relative format ("3m", "2h") not absolute
- Total visible metadata: 5 data points on one line without wrapping

**Hover state (no layout shift)**
- Reveal 3–4 icon buttons (pause/resume, open terminal, delete) right-aligned, replacing the elapsed time column
- Use `position: absolute` or `display: grid` column swap — never insert/remove DOM that causes reflow
- Transition: opacity 0→1, 100ms ease — not a slide

**Group headers (REQ-2)**
- 24px row: 10px uppercase muted label + item count badge
- No divider line — use 8px vertical gap above the header to signal the boundary
- Background same as session rows — no distinct section background

**Scannability tactics (from data density research)**
- Consistent left-edge alignment of the status dot creates a "status column" the eye can scan vertically
- Monotone icon set reduces color noise — reserve color for status dots and the accent (indigo)
- 14px body / 20px line-height is the sweet spot for dense readable lists (confirmed by Linear and Material guidelines)
- Use tabular numbers (font-variant-numeric: tabular-nums) for elapsed time so values don't jitter width on update

---

## 2. Keyboard Shortcut Cheatsheet UI Patterns

### Reference Implementations

**VS Code (Ctrl+K Ctrl+S)**
- Opens a full-panel editor (not a modal) that replaces the active editor area
- Table format: Command | Keybinding | When | Source
- Live search bar at top filters the list as you type; can search by command name OR by key combination
- Verdict for stapler-squad: full-panel is overkill; the searchable table model is excellent

**Figma (Ctrl+/)**
- Opens a floating overlay, roughly 480×600px, centered
- Categorized sections with sticky category headers
- Not searchable in the overlay itself — the `/` shortcut opens the Quick Actions command palette instead
- Verdict: the modal format works well; lack of search is a weakness to avoid

**Linear**
- Linear does not have a dedicated shortcut cheatsheet modal; shortcuts are discoverable via the command palette (⌘K) where each command shows its bound key to the right
- This is elegant but requires users to already know what they're looking for

**GitHub**
- `?` key opens a full-page modal overlay listing shortcuts grouped by page context (Issues, PRs, Code, etc.)
- Not searchable — pure reference list
- Keyboard-navigable: Tab moves through sections
- Simple two-column layout: Action | Shortcut — dense and scannable

### Recommendations for REQ-5 (Shortcut Cheatsheet)

**Pattern: `?`-triggered floating modal, categorized + searchable**

Adopt the GitHub `?` trigger with Linear's inline-shortcut-hint aesthetics. Add search (GitHub's weakness).

**Structural design:**
- Modal: 520px wide, max-height 70vh, scrollable body
- Header: "Keyboard Shortcuts" title + live search input
- Sections: Global | Session List | Terminal | Omnibar — sticky section headers within the scrollable list
- Each row: left = action label (14px) | right = `<kbd>` styled key combination (12px mono, rounded badge)
- Filter: as user types, hide non-matching rows and collapse empty sections
- Dismiss: Escape key, click outside, or the `?` key again

**Single source of truth:**
- Define all shortcuts in one TypeScript file: `web-app/src/lib/shortcuts/registry.ts`
- Export: `{ id, label, category, keys: string[], handler }[]`
- The cheatsheet modal and the settings Keyboard Shortcuts tab both consume this registry
- The onboarding step 4 picks the top 6 most important shortcuts from the same registry

**Implementation:** Use `cmdk` (the library powering Linear and Raycast's command palettes) for the filter input + list rendering if the omnibar already uses it; otherwise a simple filtered `<ul>` with Radix UI Dialog for the modal is sufficient.

---

## 3. First-Run Onboarding Flows

### Reference Implementations

**Raycast onboarding**
- Install → immediate modal asking to set the global hotkey (the single most important action)
- Permissions flow is embedded in the onboarding wizard, not deferred
- Key design decision: the very first action the user takes IS the product (they set the hotkey, then immediately press it)
- No "tour" of the UI — the onboarding teaches by doing, not by describing

**Warp terminal**
- Multi-step wizard at first launch covering: login/auth, AI features, team sharing (Warp Drive)
- Docker's use case: onboarding time cut from 3 days to 1 day via shared Warp Notebooks (runbooks as onboarding artifacts)
- Lesson: shareable, structured guides embedded in the product are more effective than passive walkthroughs

**Vercel dashboard**
- Progressive disclosure pattern: shows "Getting Started" checklist in the sidebar until all items are complete, then the checklist disappears
- Items have checkmarks that fill as actions are taken — completion ceremony (confetti/green check)
- Key insight: the checklist persists across sessions so the user can return; it doesn't have to be completed in one sitting

**Progressive disclosure framework (best practice 2025)**
- Layer 1 (first session): single guided action, no overload — teaches the most critical path
- Layer 2 (first week): contextual hints that appear in situ as user encounters new screens
- Layer 3 (earned): power features surfaced only after consistent usage patterns

**Anti-patterns to avoid:**
- "Take a tour" carousels that just point at UI elements — users skip these; the UI is self-explanatory
- Requiring completion before accessing the product
- Showing all configuration options immediately

### Recommendations for REQ-4 (Onboarding)

**Pattern: 4-step modal wizard with skip-always + persistent re-trigger**

```
Step 1 — "What is stapler-squad"
  Headline: "One place for all your AI coding sessions"
  Body (2 sentences): what sessions are, why they're isolated
  Visual: ASCII diagram showing tmux session ↔ worktree ↔ repo relationship

Step 2 — "Sessions + Worktrees"
  Headline: "Each session is isolated"
  Body: each session gets its own git worktree, so agents can't step on each other
  Visual: simple diagram with two branches forking from main

Step 3 — "The Omnibar (⌘K)"
  Headline: "Create or navigate sessions in one keystroke"
  Body: what ⌘K opens, what you can type
  CTA: "Try it now" button that closes the modal and opens the omnibar

Step 4 — "Key Shortcuts"
  Inline shortcut reference (top 6 from the shortcuts registry):
    ⌘K  Open omnibar | ?  Shortcut cheatsheet | ⌘[  Focus session list
    ⌘]  Focus terminal | ⌘P  Pause session      | ⌘D  Delete session
  CTA: "View all shortcuts" link opens the cheatsheet modal
  Checkbox: "Don't show this again" (default: checked after first viewing)

Dismiss always visible: "Skip" text button top-right on every step
```

**Implementation details:**
- localStorage key: `stapler-squad:onboarded` — set to `true` after final step CTA or "Skip"
- If key is absent → show on mount after 800ms delay (avoids flash during app init)
- Re-trigger: Settings > Help tab has "Show onboarding again" button; also `?`-then-`o` shortcut
- No "next" in step 3's "Try it now" flow — the omnibar itself IS the completion of that step; return to the modal only if user dismisses omnibar without acting

---

## 4. Settings Page Consolidation Patterns

### Reference Implementations and Research Findings

**Tab navigation (≤7 sections)**
- Tabs work well when: content areas are sibling-level (not hierarchical), and there are 3–7 of them
- Break down: if one tab has 30+ fields, it likely should be a sidebar sub-section instead
- Anti-pattern: nested tabs (tabs inside tabs) — causes disorientation

**Sidebar sections (>7 sections or hierarchical)**
- Persistent sidebar lists all categories; selecting one replaces the right pane
- macOS System Settings, VS Code Settings, Linear Settings all use this pattern
- Allows deeper hierarchy without nesting — sidebar can have sub-items under a parent

**For stapler-squad (20–50 fields, 4 sections per REQ-3)**
- 4 sections (General | Sessions | Appearance | Keyboard Shortcuts) is squarely in the "tabs" sweet spot
- Tabs across the top of the settings page; single scrollable content area below
- Within each tab, use labeled subsections (bold label + light divider) rather than nested tabs
- Each subsection groups 5–8 related fields — the accordion pattern is appropriate if any subsection grows large

**Accessibility requirements:**
- Tab must be keyboard-navigable (Left/Right arrows switch tabs per ARIA `tablist` pattern)
- Active tab must have visible focus indicator that meets WCAG AA contrast
- Settings persist on change (no "Save" button) or have a clearly visible "Save" CTA — not both

### Recommendations for REQ-3 (Settings Consolidation)

**Layout:** Top-aligned tab strip (4 tabs) + scrollable content area, no sidebar needed at this scale.

```
/settings
  ┌─ General ─┬─ Sessions ─┬─ Appearance ─┬─ Keyboard Shortcuts ─┐
  │                                                                 │
  │  [subsection: Workspace]                                        │
  │    Instance name  ________________                              │
  │    Workspace mode [ ] enabled                                   │
  │                                                                 │
  │  [subsection: Defaults]                                         │
  │    Default session type  [dropdown]                             │
  │    One-off base directory  ________________  [Browse]           │
  └─────────────────────────────────────────────────────────────────┘
```

**Keyboard Shortcuts tab:** Embeds the same shortcut list as the cheatsheet modal (same registry source), but in a non-modal in-page table. Allows editing keybindings in a future iteration.

**Persistence:** Auto-save on input blur/change. Show a subtle "Saved" toast (2s) after each field save. No full-page save button.

---

## 5. In-App Documentation Hubs

### Reference Implementations

**Linear in-app help**
- `?` opens a help panel (slide-in from right, ~320px wide)
- Shows recent searches + contextual articles based on the current view
- Clicking an article opens it in the panel (not a new tab)
- Search is instant, client-side filtered from a pre-loaded article index

**Notion help**
- `?` or help icon opens a floating command-palette-style panel
- Categorized: Getting started | Keyboard shortcuts | Community | Contact support
- Articles open in a side panel that overlaps the main content

**Retool docs**
- In-app user documentation is drafted in Markdown, rendered inline in the app
- Simple pattern: markdown files → parse → render as HTML

**Docusaurus / custom embedded docs**
- Common pattern for developer tools: pre-build a search index (Algolia DocSearch or Fuse.js index) at deploy time from markdown files
- Client loads the index JSON, all search is local, zero server round-trip
- react-markdown renders the selected article

### Recommendations for REQ-6 (Documentation Hub)

**Pattern: `/help` route with two-column layout and client-side search**

```
/help
  ┌─ Search: [_______________] ─────────────────────────────────────┐
  │                                                                   │
  │  Left column (240px fixed)          Right column (flex)          │
  │  ─ Getting Started                  [Article content rendered    │
  │    · What is stapler-squad           from markdown file]         │
  │    · Session types                                               │
  │    · Worktrees                                                   │
  │  ─ Omnibar                                                       │
  │    · Usage                                                       │
  │    · Keyboard shortcuts                                          │
  │  ─ Configuration                                                 │
  │    · Config reference                                            │
  │  ─ Integrations                                                  │
  │    · tmux                                                        │
  │    · claude-mux                                                  │
  └───────────────────────────────────────────────────────────────────┘
```

**Search implementation:**
- Use **Fuse.js** (lightweight, fuzzy, no build step for index) over FlexSearch — simpler API, sufficient for <100 doc pages
- At route load: fetch all markdown files, build Fuse index over `{ title, slug, content }` entries
- As user types: filter sidebar nav and main content list; highlight matching terms with `<mark>`
- Zero server round-trip; all in-browser

**Markdown rendering:**
- `react-markdown` with `remark-gfm` plugin for tables and code blocks
- Code blocks: use the same monospace font as the terminal (JetBrains Mono)
- Heading anchors allow deep-linking from onboarding flow ("Learn more" links to `/help#omnibar`)

**Doc source files:**
- `web-app/src/docs/` — one `.md` file per article
- Vite's `import.meta.glob` loads all docs at build time → bundled, no HTTP round-trip at runtime
- Shortcut list article is auto-generated from the same registry as the cheatsheet modal

---

## 6. React Component Library Candidates

### Evaluation Criteria

The project uses vanilla-extract (build-time CSS-in-TypeScript, no Tailwind). The ideal library is:
1. **Unstyled / headless** — brings behavior and accessibility, no default styles to override
2. **Radix UI primitives** — accessible, composable, well-maintained, used in production by Linear/Vercel
3. **No Tailwind dependency** — Tailwind + vanilla-extract creates two competing class systems

### Recommendations by Component Need

**Dialog / Modal (onboarding wizard, shortcut cheatsheet)**
- **Radix UI `@radix-ui/react-dialog`** — WAI-ARIA dialog role, focus trap, scroll lock, Escape handling, fully unstyled. Style with vanilla-extract `recipe()` variants.
- No alternative needed; this is the clear choice.

**Tabs (settings page)**
- **Radix UI `@radix-ui/react-tabs`** — ARIA `tablist`/`tab`/`tabpanel` roles, keyboard Left/Right navigation, fully unstyled.
- Apply vanilla-extract `style()` for the tab strip and active indicator.

**Session list rows**
- **No headless library needed** — the session list is a custom virtualized list, not a combobox or select.
- Use **TanStack Virtual** (`@tanstack/react-virtual`) for row virtualization if the session count can grow to 100+. For <50 sessions a simple `<ul>` renders fine.
- Row hover interactions handled with CSS `(:hover)` via vanilla-extract; no JS state needed.

**Command palette / Omnibar**
- **`cmdk`** — the library already powering Linear and Raycast's command palettes. Headless, composable, unstyled. Built-in fuzzy search, keyboard navigation, grouping.
- Pair with `@radix-ui/react-dialog` for the modal shell.
- Style everything via vanilla-extract.

**Keyboard shortcut `<kbd>` badges**
- Plain HTML `<kbd>` elements, styled via vanilla-extract `style()` with a small border-radius, border, and monospace font.
- No library needed.

**Tooltip (row hover metadata)**
- **Radix UI `@radix-ui/react-tooltip`** — accessible, configurable delay, portal-based positioning.

**Dropdown menus (session actions)**
- **Radix UI `@radix-ui/react-dropdown-menu`** — keyboard navigable, ARIA compliant, unstyled.

### vanilla-extract Compatibility

Radix UI primitives are fully compatible with vanilla-extract:
- Radix provides no CSS — you provide all styles
- vanilla-extract `recipe()` variants map cleanly to component states (open/closed, active/inactive)
- Radix exposes `data-state` attributes (`data-state="open"`) that vanilla-extract can target via `selectors`

```ts
// Example: styling Radix Dialog overlay with vanilla-extract
export const overlay = style({
  selectors: {
    '&[data-state="open"]': { opacity: 1 },
    '&[data-state="closed"]': { opacity: 0 },
  },
  transition: 'opacity 150ms ease',
});
```

**Do NOT use:**
- shadcn/ui — tightly coupled to Tailwind CSS; the CLI copies components pre-wired to Tailwind utilities. Not compatible with vanilla-extract as the sole CSS system.
- Headless UI (by Tailwind Labs) — designed for Tailwind, limited component count, slower release cadence.
- MUI / Chakra / Mantine — ship their own styling solutions; heavyweight for this use case.

---

## Summary: Recommended Decisions Per Feature

| Feature | Pattern Decision | Key Library |
|---|---|---|
| Session list rows | 36–40px single-line, `grid` layout, hover reveals icon actions, TanStack Virtual if >50 sessions | `@tanstack/react-virtual` |
| Group headers | 24px, uppercase muted label, gap-based separation (no dividers) | vanilla-extract only |
| Shortcut cheatsheet | `?` trigger, Radix Dialog modal, 2-col table, live filter via Fuse.js | `@radix-ui/react-dialog`, Fuse.js |
| Shortcut registry | Single TS file `shortcuts/registry.ts`, consumed by cheatsheet + settings + onboarding | — |
| Onboarding wizard | 4-step Radix Dialog modal, skip always present, localStorage flag, step 3 hands off to omnibar | `@radix-ui/react-dialog` |
| Settings page | Radix Tabs, 4 tabs (General / Sessions / Appearance / Keyboard Shortcuts), auto-save | `@radix-ui/react-tabs` |
| Docs hub | `/help` route, Fuse.js client search, `react-markdown` + `remark-gfm`, Vite glob import | `react-markdown`, Fuse.js |
| Omnibar | `cmdk` headless + Radix Dialog shell, groups, keyboard nav | `cmdk`, `@radix-ui/react-dialog` |
| Tooltips | Radix Tooltip, 300ms delay, portal positioning | `@radix-ui/react-tooltip` |
| Dropdown menus | Radix DropdownMenu | `@radix-ui/react-dropdown-menu` |

---

## Sources

- [How we redesigned the Linear UI (part II)](https://linear.app/now/how-we-redesigned-the-linear-ui)
- [A calmer interface for a product in motion — Linear](https://linear.app/now/behind-the-latest-design-refresh)
- [Designing for Data Density — Paul Wallas / Medium](https://paulwallas.medium.com/designing-for-data-density-what-most-ui-tutorials-wont-teach-you-091b3e9b51f4)
- [App Settings UI Design — Setproduct](https://www.setproduct.com/blog/settings-ui-design)
- [Tabs UX Best Practices — LogRocket](https://blog.logrocket.com/ux-design/tabs-ux-best-practices/)
- [Progressive Disclosure Onboarding — userTourKit](https://usertourkit.com/blog/progressive-disclosure-onboarding)
- [Warp customers: Docker onboarding case study](https://www.warp.dev/customers/docker)
- [Raycast onboarding flow — Pageflows](https://pageflows.com/post/desktop-web/onboarding/raycast/)
- [cmdk — Fast, unstyled command menu for React](https://github.com/dip/cmdk)
- [Radix UI Primitives](https://www.radix-ui.com/primitives)
- [From Tailwind to vanilla-extract — Gabriel Moyano](https://gafemoyano.com/en/posts/from-tailwind-to-vanilla-extract/)
- [Fuse.js — Lightweight Fuzzy-Search](https://www.fusejs.io/)
- [React UI libraries comparison 2025 — Makers' Den](https://makersden.io/blog/react-ui-libs-2025-comparing-shadcn-radix-mantine-mui-chakra)
- [Top Headless UI libraries for React in 2026 — GreatFrontend](https://www.greatfrontend.com/blog/top-headless-ui-libraries-for-react-in-2026)
- [Using Material Density on the Web — Google Design / Una Kravets](https://medium.com/google-design/using-material-density-on-the-web-59d85f1918f0)
- [Data Table Design UX Patterns — Pencil & Paper](https://www.pencilandpaper.io/articles/ux-pattern-analysis-enterprise-data-tables)
- [Implementing client-side search with Fuse.js — Daily.co](https://www.daily.co/blog/implementing-client-side-search-in-a-react-app-with-fuse-js/)
