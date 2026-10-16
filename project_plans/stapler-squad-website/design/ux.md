# UX Design: stapler-squad-website

SDD Phase 3 design artifact. Builds on `project_plans/stapler-squad-website/research/ux.md`
(comparable patterns, mental models, a11y requirements, JTBD) and
`project_plans/stapler-squad-website/implementation/plan.md` (Astro + Starlight, the
`FeatureCard`/`ContentManifest`/`ReleaseAssetRef` data shapes, the 11 curated how-to pages,
the 404/search/asset-health build-time checks already planned as Stories 4.4.1/4.4.2).
This document does not re-litigate those decisions — it specifies the screens, flows, and
testable UX acceptance criteria an implementer and a reviewer can check the shipped site against.

## Surface Inventory

| # | Surface | User-facing? | Treatment |
|---|---|---|---|
| 1 | Landing page (hero, feature grid, demo media, install CTA) | Yes — primary conversion path | Full (wireframe + flow + error table) |
| 2 | Docs sidebar nav + search modal | Yes — primary self-serve path | Full (wireframe + flow + error table) |
| 3 | Individual docs page template | Yes | Full (folded into §2, shares its nav chrome) |
| 4 | 404 page | Yes, but single static state, no decision tree | Condensed |
| 5 | Demo-media-failed-to-load fallback | Yes, but a cross-cutting contract, not a standalone screen | Condensed (cross-referenced from §1/§2 error tables) |
| 6 | Deploy workflow status badge | No — repo maintainer artifact, not end-user-facing | Condensed |
| 7 | `prefers-reduced-motion` state for hero video | Yes, but a single conditional render, no interaction | Condensed |

Total: 7 surfaces designed (2 full interactive flows covering 3 screens, 4 condensed entries).

---

## 1. Landing Page

### 1.1 Wireframe — desktop (≥768px)

```
┌──────────────────────────────────────────────────────────────────────┐
│  [Stapler Squad logo]                              [Docs]  [GitHub ↗]│ ← persistent header
├──────────────────────────────────────────────────────────────────────┤
│                                                                        │
│   Stapler Squad                                                       │
│   Mission control for running multiple AI coding agents               │
│   (Claude Code, Codex, Gemini, Aider) at once — real-time dashboard,  │
│   automatic approval rules, structured review queue.                  │
│                                                                        │
│   ┌────────────────────────────────────────────────────┐  [Copy 📋]  │ ← install block,
│   │ $ brew tap tstapler/stapler-squad && brew install..│             │   copy-to-clipboard
│   └────────────────────────────────────────────────────┘             │
│                                                                        │
│   [ Install ▸ ]   [ View on GitHub ]           CI ✓   Release v2.x    │ ← primary/secondary
│                                                                        │   CTA + trust badges
│   ┌──────────────────────────────────────────────────────────────┐   │
│   │                                                                │   │
│   │        <video autoplay loop muted controls poster=...>        │   │ ← hero demo, ABOVE
│   │                 (full product demo, scrubbable)                │   │   the fold, no
│   │                                                                │   │   click-to-play wall
│   └──────────────────────────────────────────────────────────────┘   │
├──────────────────────────────────────────────────────────────────────┤
│  Features                                                              │
│  ┌───────────┐ ┌───────────┐ ┌───────────┐ ┌───────────┐             │
│  │[screenshot]│ │[screenshot]│ │[screenshot]│ │[screenshot]│  ...      │ ← FeatureCard grid,
│  │ Dashboard  │ │  Review    │ │  History   │ │ Isolated   │          │   4-up desktop,
│  │ Real-time..│ │  Queue     │ │  Search    │ │ Workspace  │          │   category-grouped
│  └───────────┘ └───────────┘ └───────────┘ └───────────┘             │
│  (17 cards total: 8 with screenshot, 9 text-only, grouped by the 5   │
│   README categories — Visibility / Organisation / Review & Approval /│
│   History & Debugging / Infrastructure)                              │
├──────────────────────────────────────────────────────────────────────┤
│  Need help getting unstuck?  → [Browse the docs]                      │ ← unstuck exit,
│                                                                        │   reachable without
│  [GitHub] [Docs] [Report an issue]                        CI ✓ badge │   scrolling past install
└──────────────────────────────────────────────────────────────────────┘
```

### 1.2 Wireframe — mobile (375px)

```
┌───────────────────────┐
│ [logo]      [≡ menu]  │ ← header collapses Docs/GitHub into a menu
├───────────────────────┤
│ Stapler Squad          │
│ Mission control for    │
│ running multiple AI    │
│ coding agents at once. │
│                         │
│ ┌─────────────────────┐│
│ │ $ brew tap ...      ││ ← single column, no
│ │           [Copy 📋] ││   horizontal scroll
│ └─────────────────────┘│
│                         │
│ [ Install ▸ ]          │ ← stacked CTAs, full-
│ [ View on GitHub ]     │   width tap targets
│                         │   (≥48x48dp)
│ ┌─────────────────────┐│
│ │  <video poster=...> ││ ← full-width, 16:9,
│ │                     ││   controls visible
│ └─────────────────────┘│   without pinch-zoom
├───────────────────────┤
│ Features                │
│ ┌─────────────────────┐│
│ │ [screenshot]        ││ ← single-column card
│ │ Dashboard            ││   stack, not a cropped
│ │ Real-time status...  ││   4-up grid
│ └─────────────────────┘│
│ ┌─────────────────────┐│
│ │ ...                 ││
│ └─────────────────────┘│
├───────────────────────┤
│ → Browse the docs      │
│ [GitHub][Docs][Issue]  │
└───────────────────────┘
```

### 1.3 Interaction Flow

| Step | User action | System response |
|---|---|---|
| 1 | Lands on `/` (from README link, search engine, or shared link) | Hero renders synchronously (zero-JS-by-default per Astro/Starlight) — headline + value prop + install block visible without scroll on a 1080p viewport |
| 2 | Reads headline (≤5s per `research/ux.md` §2) | No action required — answers "what is this, is it for me" before any interaction |
| 3 | Watches/scrubs the hero `<video>` | Native HTML5 controls; `autoplay loop muted` on load, pauses on manual scrub like any `<video>` |
| 4 | Clicks the copy icon next to the install command | Clipboard API fires; button transitions to a "Copied ✓" state for ~2s, then reverts; the command text itself remains visibly selectable even if the click fails (see Error States) |
| 5 | Scrolls to the feature grid | Cards lazy-render (native `loading="lazy"` on images — no custom JS scroll-observer needed) |
| 6 | Clicks "View on GitHub" (secondary CTA) | Opens `github.com/tstapler/stapler-squad` in the same tab (outbound, not same-page) |
| 7 | Scrolls to footer without finding an answer | Sees "Browse the docs" link and GitHub issue link — no need to scroll back up to the header nav |

### 1.4 Error & Edge-Case States

| Trigger | What the user sees | Exit path |
|---|---|---|
| Hero `<video>` source 404s (release asset or committed `demo.webm` missing) | Browser falls back to the `poster="screenshot.png"` static frame — a real product screenshot, not a blank box | Page remains fully readable; install CTA and feature grid are unaffected |
| Feature-card `<img>` screenshot 404s | Browser renders the descriptive `alt` text in place of the broken image icon (e.g. "Dashboard showing status badges, diff stats, tags, and approvals for all agents at a glance") | Card's heading/description text still communicates the feature; no blank card |
| Clipboard API blocked (insecure context, permission denied, or no JS) | Copy button click does nothing visible, but the install command remains a plain, triple-click-selectable `<pre>`/`<code>` block | User can still select-and-copy manually — command text is never JS-gated behind the button |
| User on `prefers-reduced-motion: reduce` | Hero video does not receive the `autoplay` attribute (checked client-side before set); shows the poster frame with visible play controls instead | User can still press play manually — no motion forced on anyone who opted out |
| JS fails to load entirely | Static HTML (headline, install block, feature grid, footer links) still renders — only the copy-button polish and the reduced-motion check degrade | No dead page; core content is server-rendered, not client-hydrated |

---

## 2. Docs Section: Sidebar Nav + Search

### 2.1 Wireframe — desktop

```
┌──────────────────────────────────────────────────────────────────────┐
│  [logo]  Docs                           [ 🔍 Search...      ⌘K ]  [GitHub ↗]│ ← header, search
├───────────────────┬────────────────────────────────────────────────────┤
│ ▸ Getting Started  │  # Getting Started                                 │
│   • Homebrew       │                                                     │
│   • Manual install │  Both Homebrew and manual installation install    │
│   • Build from src │  Stapler Squad as `ssq` on your system.            │
│                     │                                                     │
│ ▸ Features          │  ## Homebrew                                       │
│   • Overview (17)   │  ```                                               │
│                     │  brew tap tstapler/stapler-squad                   │
│ ▾ Troubleshooting   │  brew install stapler-squad                        │
│   • Debug with logs │  ```                                               │
│   • Manage notifs   │  ...                                               │
│   • Manage systemd  │                                                     │
│   • Monitor ext.    │  ← [Prev: Overview]      [Next: Debug w/ Logs →]  │ ← page pager, always
│   • ...(11 total)   │                                                     │   present, never a
│                     │                                                     │   dead end at either
│   (current page     │                                                     │   end of a section
│    highlighted,     │                                                     │
│    group expanded)  │                                                     │
└───────────────────┴────────────────────────────────────────────────────┘
```

### 2.2 Search modal (⌘K or click search box)

```
┌──────────────────────────────────────────────────────────┐
│  🔍  tmux session|                                   [Esc] │ ← focus trapped here on open
├──────────────────────────────────────────────────────────┤
│  Monitor External Terminal Sessions                        │
│  ...tmux session ...ssq-mux for IDE terminals...            │
│                                                              │
│  Manage systemd Service                                    │
│  ...tmux server restart kills every live tmux session...   │
└──────────────────────────────────────────────────────────┘

Zero-results state (query "zzqqxyz"):
┌──────────────────────────────────────────────────────────┐
│  🔍  zzqqxyz|                                        [Esc] │
├──────────────────────────────────────────────────────────┤
│  No results for "zzqqxyz"                                  │
│  → Browse all docs        → File an issue on GitHub        │ ← two exits, never blank
└──────────────────────────────────────────────────────────┘
```

### 2.3 Wireframe — mobile (375px)

```
┌───────────────────────┐
│ [≡]  Docs    [🔍]      │ ← hamburger opens sidebar as
├───────────────────────┤   a full-screen overlay drawer;
│ # Getting Started      │   search icon opens the same
│                         │   modal as desktop, full-width
│ Both Homebrew and       │
│ manual installation...  │
│                         │
│ [← Prev]   [Next →]    │
└───────────────────────┘

Sidebar drawer (after tapping ≡):
┌───────────────────────┐
│ [✕ close]              │ ← focus trapped in drawer;
│ ▸ Getting Started      │   Escape or ✕ closes, focus
│ ▸ Features             │   returns to the ≡ button
│ ▾ Troubleshooting      │
│   • Debug with logs    │
│   • ...                │
└───────────────────────┘
```

### 2.4 Interaction Flow

| Step | User action | System response |
|---|---|---|
| 1 | Arrives at `/help/getting-started/` (bookmark, landing-page link, or search result) | Sidebar renders with the current page's group expanded and the page itself highlighted — no extra click needed to see "where am I" |
| 2 | Presses `Cmd/Ctrl+K` (desktop) or taps the search icon (mobile) | Modal opens; focus moves into the input immediately; page-behind is inert (`aria-hidden`/`inert`) so Tab can't escape into it |
| 3 | Types a query | Results update live, client-side, same-origin only (Pagefind index — no third-party network call) |
| 4 | Clicks/Enters a result | Modal closes, page navigates, focus moves to the new page's `<h1>` (not left floating or reset to `<body>`) |
| 5 | Presses `Escape` with no selection | Modal closes, focus returns to the search trigger it was opened from |
| 6 | Wants troubleshooting help from any page | Clicks "Troubleshooting" in the always-visible sidebar — reachable in 1 click from any docs page, 2 clicks max from the landing page (landing → Docs → Troubleshooting) |
| 7 | Reaches the end of a section's last page | Uses the "Next"/"Prev" pager, which always points somewhere real — on the very last page, "Next" is omitted rather than pointing at nothing (never a disabled dead button) |

### 2.5 Error & Edge-Case States

| Trigger | What the user sees | Exit path |
|---|---|---|
| Search query matches nothing | "No results for '{query}'" + a link to the docs home + a "File a GitHub issue" link, inside the still-open modal | User can retype, click either link, or press Escape |
| A curated page links to an excluded/internal doc (should never ship — caught by `BrokenLinkCheck` in CI per the implementation plan's Story 4.4.1) | If a link slips through anyway: standard browser navigation to the branded 404 page (§4.1), not a raw GitHub 404 | 404 page's "docs home" and search links |
| Search modal opened, then user resizes below 768px mid-session | Modal re-renders at mobile full-width layout, same focus-trap behavior, no layout break | Same Escape/close affordance |
| JS fails to load (search is inherently client-side) | Search box/⌘K hint simply doesn't respond — sidebar nav (plain links) and page content remain fully usable | Static nav tree is the fallback path to every page; search is additive, not the only way to reach content |
| User tabs through the sidebar tree | Each group header and link receives a visible focus ring (reusing the product's `vars.color.inputFocusBorder` token so the site reads as the same product family) | Standard keyboard traversal, no focus trap outside the search modal |

---

## 3. Individual Docs Page Template

Shares the chrome from §2 (header, sidebar, search). Per-page contract:

- `<h1>` matches the frontmatter `title` derived from the source doc's first `# ` heading (per the plan's `sync-content.mjs` behavior) — never missing, never duplicated.
- Every page has a visible "Prev"/"Next" pager, omitting either end at the start/end of the sidebar's flattened page order (never a disabled or dead-pointing button).
- Code blocks (install commands, config snippets) get the same copy-to-clipboard affordance as the landing page's install block, for consistency — a user shouldn't learn two different copy interactions on one site.
- Images carry real alt text sourced from the original doc's surrounding prose (per `research/ux.md` §3), never a bare filename.

---

## 4. Condensed Surfaces

### 4.1 404 Page

Representative sample:

```
┌──────────────────────────────────────────────────────────┐
│  [logo]  Docs                                   [GitHub ↗]│ ← same header as every page
├──────────────────────────────────────────────────────────┤
│                                                             │
│              404 — Page not found                          │
│                                                             │
│   The page you're looking for doesn't exist or moved.      │
│                                                             │
│   [ Go to the docs home ]     [ 🔍 Search the docs ]        │
│                                                             │
└──────────────────────────────────────────────────────────┘
```

Acceptance criteria:
- Renders the site's own header/nav/branding, never GitHub Pages' generic default 404 HTML.
- Offers at least two exit paths: a link to the docs home and a way to search (reusing the same search modal as every other page).
- Reachable at any unmatched path under the `/stapler-squad/` base path, including a typo'd docs slug.
- Passes the same Lighthouse accessibility ≥90 / axe-core critical-violation gate as every other page (no exemption for "it's just an error page").

### 4.2 Deploy Workflow Status Badge (non-interactive, maintainer-facing)

Representative sample: a standard GitHub Actions badge image, e.g.
`[![Deploy Website](https://github.com/tstapler/stapler-squad/actions/workflows/deploy-website.yml/badge.svg)](...)`, placed in `README.md` next to the existing CI badge.

Acceptance criteria:
- Badge reflects the live status of the most recent `deploy-website.yml` run (pass/fail/in-progress), matching the existing `build.yml` badge's established visual convention — no new badge style introduced.
- Links through to the workflow's run history on click.
- Not part of any end-user flow — excluded from the Lighthouse/axe-core gates that apply to the public site itself (it lives in `README.md`, not `website/`).
- No design review burden beyond "does it match the existing badge's placement and style" — this is config, not a UI surface a visitor encounters.

### 4.3 Demo-Media-Failed-to-Load Fallback (cross-cutting contract)

This is not a standalone screen — it is the shared contract every `<video>`/`<img>` embed on the site must honor, referenced from §1.4 and §3. Stated once here to avoid repeating it per-surface:

- Every `<video>` has a real `poster` attribute pointing at a committed, small, in-repo image (never depending on the same Release-asset tier it would otherwise fall back to — the fallback must not share a failure mode with the thing it's falling back for).
- Every `<img>` has descriptive `alt` text sourced from the surrounding doc/feature prose, so a 404 degrades to readable text, never a blank box or a bare filename.
- `AssetHealthCheck` (already planned in Epic 2.2) is the build-time backstop that should make a live 404 rare — this UX contract is the defense-in-depth for the case it still happens (stale cache, a CDN edge miss, a user on a flaky connection).

### 4.4 `prefers-reduced-motion` State (non-interactive conditional render)

Representative sample: hero `<video>` renders with `loop muted controls` but without `autoplay` when the media query matches; shows the `poster` frame at rest with visible native play controls.

Acceptance criteria:
- No motion plays without an explicit user action when `prefers-reduced-motion: reduce` is set.
- The static poster frame is a real, legible product screenshot — not a blank or loading placeholder.
- Controls remain fully visible and operable so the user can still choose to play the video.

---

## 5. UX Acceptance Criteria

### Task completion

1. A first-time visitor can identify "what is this, is it for me" from the hero section alone, with zero scrolling, on a 1080p desktop viewport and a 375px mobile viewport.
2. A visitor can copy the install command in **1 click** (the copy button) or **1 selection gesture** (manual select, if JS/clipboard is unavailable) — no scrolling required to find it on either viewport.
3. A visitor can reach the docs home from the landing page in **1 click** (header "Docs" link, or the footer "Browse the docs" link) without first scrolling past the install section.
4. An existing user can reach the Troubleshooting section from **any** docs page in **1 click** (persistent sidebar group, not nested under Getting Started).
5. A user can go from the landing page to the Troubleshooting section in **2 clicks maximum** (landing → Docs → Troubleshooting, or landing → Docs → any page → Troubleshooting via sidebar).
6. A user can perform a docs search and reach a relevant result page in **2 actions** (open search, click/Enter a result) from any page on the site.
7. No page's "Next"/"Prev" pager ever points at a nonexistent page — the pager omits the control at either end of the flattened page order rather than disabling or dead-linking it.

### Error states / no dead ends

8. Every error/edge state documented in §1.4, §2.5, and §4 resolves to at least one visible, working exit path (a link, a button, or a still-readable fallback) — **no state leaves the user with nothing to click and nothing to read.**
9. The docs search zero-results state shows the exact copy pattern "No results for '{query}'" plus a link to the docs home and a link to file a GitHub issue — never a blank results panel.
10. A broken demo video/image degrades to its `poster`/`alt` fallback, never a blank box, broken-image icon with no surrounding text, or a layout-shifting empty space.
11. The 404 page renders the site's own branded chrome (header, nav, footer) and offers a link to the docs home and a way to search — never GitHub Pages' bare default 404.
12. A stale/excluded internal link is caught by `BrokenLinkCheck` at build time (per the implementation plan) before it ever reaches a live visitor; the 404 page is the documented fallback for the case that check is bypassed or a link rots post-deploy.

### Accessibility

13. **Keyboard navigation**: every interactive element (sidebar links, search trigger, search modal input and result list, pager links, copy buttons, feature-grid links) is reachable and operable via Tab/Shift+Tab/Enter alone, with no keyboard trap outside the intentional search-modal focus trap (which itself must be exitable via Escape).
14. **Focus indication**: every focusable element shows a visible focus ring on `:focus-visible`, styled from the product's existing `vars.color.inputFocusBorder` token (or an equivalently contrasting color against the site's background) — not the browser default left unstyled, and not suppressed.
15. **Screen-reader labels**: every `<img>` has either descriptive `alt` text (informational images: hero poster, feature screenshots) or `alt=""` (purely decorative images only); every icon-only button (search trigger, copy button, hamburger, modal close) has an `aria-label` describing its action, not just a visual glyph.
16. **Color contrast ≥ 4.5:1** for normal text and **≥ 3:1** for large text (≥24px, or ≥18.66px bold) and UI components, per WCAG 2.2 AA (criteria 1.4.3/1.4.11). Specifically: before shipping, verify the actual CTA-button text/background pair and link-text/background pair the site adopts — reusing the product's existing `primary: #0070f3` (light) / `primary: #2d9cdb` (dark) tokens is the intended baseline, but **do not assume it passes without checking**; `theme.css.ts` already has two documented precedents in this codebase (`#ff2d78`→`#cc245f`, `#6366f1`→`#5457ef`) where a brand color needed a post-hoc contrast fix after shipping. Run the check before merge via the planned `@axe-core/playwright` gate (implementation plan Epic 6.2), not after.
17. **Motion**: `prefers-reduced-motion: reduce` suppresses `autoplay` on every video embed site-wide (hero and any feature-card clips), per §4.4 — verified, not assumed, since this is easy to wire on one embed and forget on others.
18. **Non-text content fallback**: every `<video>` has a `poster`; the demo clips themselves carry no audio (confirmed per requirements — scripted, non-narrated Playwright captures), so no caption track is required under WCAG 1.2.2, but the feature-grid card's heading/description must always accompany the clip as its text alternative (never an unlabeled floating video).
19. **Automated gate**: Lighthouse accessibility score ≥ 90 (blocking, not warn-only — a stricter, independent threshold from `web-app/lighthouserc.json`'s existing 0.80 warn-only gate) and zero `critical`/`serious` axe-core violations on the landing page and at least one docs page template, enforced in CI before every deploy.

---

## Cross-References

- Landing-page copy sourcing (hero tagline verbatim from `README.md:3`, install command verbatim, feature taxonomy reconciliation): implementation plan Epics 3.1–3.3.
- Docs sidebar grouping and the 11-page Troubleshooting include/exclude list: implementation plan Story 2.1.2 / Epic 4.3.
- Search implementation (Pagefind, zero-results copy), `BrokenLinkCheck`, 404 page: implementation plan Epic 4.4.
- `AssetHealthCheck` / `ReleaseAssetRef` build-time 404 prevention: implementation plan Epic 2.2.
- Lighthouse/axe-core CI gates referenced in §5's accessibility criteria: implementation plan Epics 6.1–6.2.
- Color-contrast precedent and theme-token reuse: `research/ux.md` §3, `web-app/src/styles/theme.css.ts`.
