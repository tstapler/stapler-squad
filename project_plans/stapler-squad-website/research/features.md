# Research: Features (Agent 2)

Scope: landing page, demo media, help docs — feature-landscape patterns, edge cases, unstated user needs, and raw-material gap check against `docs/features.md` + `assets/demo-recording/`.

## 1. Comparable OSS dev-tool sites — structural patterns

Checked three terminal/web-hybrid dev tools in the same category (local tool + rich UI), via WebFetch of the live sites:

**k9s** (k9scli.io — plain site, no visible SSG chrome):
- Header/hero → one-line tagline + a static screenshot
- "Who Let The Pods Out?" intro paragraph
- Features: one long nested bullet list (no cards/grid)
- Sponsor/support call-to-action
- **Documentation section is just a link list** (`/topics/install`, `/topics/rbac`, etc.) — the landing page doesn't try to *be* docs, it indexes them
- Previews: one asciinema recording (clickable SVG thumbnail linking out to asciinema.org, not an inline embed) + five captioned static PNG screenshots
- No visible nav bar, search, or theme toggle on the landing page itself
- Takeaway: **demo media doesn't have to be inline-hosted** — a thumbnail link to an external player (asciinema) avoids repo bloat and works fine for a dev audience that's comfortable clicking through.

**Posting** (posting.sh — MkDocs Material, inferred from "Initializing search" + trailing-slash URLs):
- Top nav: Home / Guide / Roadmap / Changelog / FAQ
- Hero tagline → install command (with copy button) → "Read the guide" + "GitHub" CTAs
- A large **terminal-mockup demo** (HTML/CSS art, not an image/video) directly in the hero
- "Keyboard first" feature section styled as a **rotating carousel** ("1 of 7") cycling through feature call-outs
- Built-in search ("Initializing search" placeholder = MkDocs Material's search-index loader)
- No simple dark/light toggle — instead an 8-option **theme picker** (galaxy, aurora, catppuccin-latte, etc.), only one of which is light
- Takeaway: a rendered/mocked terminal UI in the hero (rather than a GIF) sidesteps media-hosting entirely for the *above-the-fold* demo, with real recordings reserved for feature-specific sections further down.

**Starship** (starship.rs — VitePress, confirmed via `#VPContent`/"Appearance" toggle signatures):
- Hero: tagline + logo + single "Get Started →" CTA linking straight into `/guide/`
- Three short feature blurbs (Compatibility / Performance / Customizable) — no screenshots at all on the landing page
- Docs live under the **same domain, different path** (`/guide/`, `/config/`), not a subdomain — directly relevant since this project's constraint is one GitHub Pages origin
- Standard VitePress chrome: ⌘K search, locale switcher, light/dark "Appearance" toggle, "Skip to content" link
- No demo GIF/video anywhere — the product (a shell prompt) is visual but they chose not to show it; feature descriptions carry the weight instead

**VHS** (charmbracelet/vhs — the generation side, not a landing-page example, but directly relevant to this project's "automated, higher-production-value" media constraint):
- Demos are written as a plain-text `.tape` script (`Type`, `Enter`, `Sleep`, `Wait` commands) and rendered headlessly — exactly the "no owner recording/narration" model this project already committed to via Playwright
- Ships a first-party `vhs-action` GitHub Action specifically to "keep GIFs up to date" from CI — same shape as this project's existing `demo-publish.yml`
- Has a **dedicated external hosting path**: `vhs publish demo.gif` uploads to `vhs.charm.sh` and hands back a stable URL + ready-made `<img>`/Markdown embed snippet. Conceptually the same pattern this repo already uses (GitHub Release assets with a stable `releases/download/<tag>/<name>` URL) — confirms Release-asset hosting is the standard answer in this tool category, not a one-off workaround.

**Cross-cutting pattern across all three landing pages:**
| Pattern | k9s | Posting | Starship |
|---|---|---|---|
| Docs reached via link-out, not embedded in landing page | ✅ | ✅ | ✅ |
| Docs on same domain/subpath (not separate repo/subdomain) | unclear | ✅ | ✅ |
| Real recorded demo media | asciinema link-out | mockup, not real capture | none |
| Dark/light or multi-theme | not visible | 8-theme picker | toggle |
| Search | not visible | yes (MkDocs search) | yes (⌘K) |

Net: none of the three inline a large committed GIF/video directly in the page — they either link out to an external player, fake the demo with markup, or skip it. This supports the project's existing constraint (Release assets, not committed binaries) and suggests the landing page should **link/lazy-load** media rather than ship it as part of the page bundle.

## 2. Edge cases and failure modes to design for

- **Pages build failures blocking deploy.** The repo's Pages config is already in a broken state (`build_type: workflow`, no workflow, live 404 — confirmed in requirements.md). The new workflow must not just "add a job" but be verified end-to-end against a real deploy; a failed `actions/deploy-pages` step should fail the workflow loudly (it does by default) rather than leave the previous (404) state looking unchanged.
- **Broken links when `docs/` source files get renamed/moved.** The site will curate/adapt specific `docs/how-to/*.md` files (per requirements' "curated subset" scope). Any internal doc-to-doc link or site-generator reference by filename will silently 404 if the source file is later renamed in `docs/how-to/` without a corresponding site update — there is no coupling mechanism today (no test/lint enforces it). Recommend either: (a) a CI check that diffs curated filenames against `docs/how-to/` existence, or (b) treating curation as **copy-and-adapt** (already implied by "adapted for a public audience") rather than live-including, with an explicit curated-list file reviewed on every `docs/how-to/` change — this is exactly the kind of cross-reference-rot problem `docs:prune`/`docs:update`-style tooling exists for.
- **Release-asset URLs for demo media going stale/deleted.** `demo-publish.yml` already uses a **rolling** `demo-gifs-latest` release tag (overwritten on each manual run), so the download URL shape is stable — but if a future run is skipped or the release is deleted, every embedded `<img>`/`<video src>` pointing at `releases/download/demo-gifs-latest/<name>.gif` 404s silently with no visible break in the HTML (just a broken-image icon). The site build should ideally verify (HEAD request) that referenced asset URLs resolve at build time, failing the deploy loudly instead of shipping broken media.
- **Mobile responsiveness.** All three comparable sites (k9s, Posting, Starship) are plain content pages, not app shells — a dedicated docs SSG (VitePress/Docusaurus/MkDocs Material) gets this for free. A hand-rolled option or a `web-app`-reused Next.js static export would need this designed in explicitly; the product's own `web-app` is a dashboard UI, not built mobile-first, so its layout patterns aren't a safe default to copy wholesale for a marketing page.
- **Dark/light mode.** The product's own web-app (`web-app/src/app/globals.css`) already migrated *away* from a pure `@media (prefers-color-scheme: dark)` block toward an explicit `data-theme` switcher (Story 1.5.3, per the file's own comments) and supports user-added custom themes (`docs/how-to/add-user-themes.md`). The marketing site doesn't need to reuse those exact CSS variables (a separate SSG has its own theming system per the examples above), but should at minimum support light+dark to match user expectation set by the product itself, and should use the same brand colors/logo (`web-app/public/logo.svg`, `logos/concepts/*.svg`) for visual continuity rather than inventing a new palette.
- **Video/GIF autoplay and accessibility.** The current README embeds `<video autoplay loop muted controls>` for the full demo — autoplaying video should still respect `prefers-reduced-motion` and have accessible pause controls on the new site, not just carry the README's embed verbatim.

## 3. Unstated user needs beyond the explicit requirements

- **"Is this actively maintained?"** A prospective adopter's first filter. The README already surfaces this partially via CI and Release badges (`build.yml` badge, `img.shields.io/github/v/release/...`). The landing page should surface the same signal (recent release version/date, commit activity) rather than making the visitor dig through GitHub Insights — e.g. a "latest release" badge/link near the install CTA, which is cheap since it's already a shields.io badge in the README.
- **"Will this work on my OS?"** Confirmed from `README.md`: install paths are Homebrew (`brew tap tstapler/stapler-squad ... && brew install ...`), a manual pre-built-binary `install.sh` (self-detects shell: zsh/bash/fish/ash, auto-adds `~/.local/bin` to `PATH`), and build-from-source (needs Go, Node, buf — auto-installed via Homebrew by `make build`). There is **no Brewfile in this repo** (that's a dotfiles-repo convention, not applicable here) — the only package-manager path is the external `tstapler/homebrew-stapler-squad` tap referenced inline. The landing page's install CTA should surface OS coverage explicitly (macOS/Linux via Homebrew tap and binary; Windows is not mentioned anywhere in `install.sh`'s shell detection, which only branches on zsh/bash/fish/ash — i.e. **Windows/WSL support is unconfirmed and should not be implied** by the install CTA without checking further, since `install.sh`'s `$SHELL` branches don't cover `cmd`/PowerShell).
- **"What does it look like before I install it?"** This is the explicit demo-media requirement, but the unstated corollary is that a visitor wants this *before* scrolling to install instructions, not after — all three comparable sites put visual proof (screenshot, mockup, or demo link) in or immediately below the hero, not buried under a "Features" wall of text.
- **Existing users want a fast path to *their specific error*, not a manual.** This is a search/findability requirement, not just a content-curation one — of the three comparables, the two with real docs sites (Posting/MkDocs, Starship/VitePress) both ship built-in full-text search (⌘K-style or MkDocs Material's search index) specifically because link-list navigation alone doesn't answer "I got error X." This pushes toward an SSG with first-class search over hand-rolled HTML, which is a relevant input to the Phase 2 tooling-choice research (the SSG decision owned by a different research dimension, but this requirement should weigh toward whichever option ships search for free).

## 4. Does existing raw material (`docs/features.md` + `assets/demo-recording/*.png`) cover a feature-highlights section with no new work?

**No — there are real gaps.** Specifics:

- **Confirmed broken reference today:** `docs/features.md` line 117 embeds `../assets/demo-recording/09-unfinished.png` for the "Unfinished Work" section, but `assets/demo-recording/` only contains `01-dashboard.png` through `08-final.png` — **`09-unfinished.png` does not exist on disk** (verified via `ls assets/demo-recording/`). This is already a broken image link in the current repo's own `docs/features.md`, not something the new site would introduce — but if the site curates this file as-is, it inherits a 404'd image. Needs either regenerating that screenshot or dropping/reordering the section.
- **Image coverage is partial, not complete.** Of `docs/features.md`'s 17 feature sections, only 9 have an accompanying screenshot (Dashboard, Instant Search, Status Filtering, Tag-Based Grouping, Review Queue, Auto-Approval Rules, Custom Rules, Unfinished Work (image broken, see above), Isolated Git Workspaces). The other 8 — **Live Terminal Streaming, Diff Viewer, Notifications, Workspace Switcher, Bulk Actions, Approval Analytics, History Search, Logs Viewer, Config Viewer** — have text only, no visual. A "feature highlights" section built straight from this file would be text-heavy for roughly half its entries, undercutting the "what does it look like" need identified in §3.
- **Screenshots are static PNGs, not the demo GIF/video the requirements call for.** `assets/demo-recording/*.png` are stills (confirmed via `ls -la`, ~100-155KB each, dated 2026-04-30 per `git log`), suitable for a "features" page but not for the hero-level "demo videos/GIFs" requirement — that's a separate, not-yet-existing asset class the Phase 3 plan already scopes as new work (the automated Playwright recording pipeline). The two don't substitute for each other.
- **README's `Highlights` section (the other named source, per requirements line 45) is richer than `docs/features.md` in grouping** — it clusters the same ~17 features into 5 named categories (Visibility, Organisation, Review & Approval, History & Debugging, Infrastructure) with punchier one-line descriptions, but has **zero images** of its own; it relies entirely on the single top-of-README `assets/demo.gif`.
- **Net assessment:** the existing material is a legitimate *starting point and content source* (headings, descriptions, groupings, and 9 real screenshots are reusable as-is) but not sufficient on its own to build a complete, visually-consistent feature-highlights section. Phase 3 should scope: (1) fixing/regenerating the missing `09-unfinished.png`, (2) deciding whether the 8 screenshot-less sections get new captures (likely folded into the same new automated-recording pipeline work, since it's already scoped to touch Playwright e2e fixtures) or ship text-only on the marketing site, and (3) reconciling README's 5-category grouping vs. `docs/features.md`'s flat 17-section list into one structure for the new page — right now they're two different taxonomies of the same feature set.

## Sources (external)

- [k9s](https://k9scli.io/) — landing page structure (WebFetch)
- [Posting](https://posting.sh/) — landing page structure, MkDocs Material inference (WebFetch)
- [Starship](https://starship.rs/) — landing page structure, VitePress confirmation (WebFetch)
- [charmbracelet/vhs](https://github.com/charmbracelet/vhs) — scripted demo-GIF generation and external-hosting pattern (WebFetch)
