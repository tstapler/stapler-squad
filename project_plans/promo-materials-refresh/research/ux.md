# UX research: /tours section and promo media

Note: public-facing names use "Grimdark" only; "WH40K" appears below solely when citing the current internal file `wh40k.css`.

Confidence labels: VERIFIED = read in this repo this session. GENERAL-KNOWLEDGE = established WCAG/web practice or recollection of how named products structure pages; no live web fetch was done, so verify specific product claims before citing them externally.

## 1. Repo facts that constrain the design (VERIFIED)

- `website/src/styles/wh40k.css` (83 lines) overrides Starlight tokens only: `--sl-color-*` neutrals, gold accent `#c0a020`, Cinzel for `h1-h4`/`.site-title`, dark only (`color-scheme: dark`, light mapped to same palette, theme picker removed via `NoThemeSelect.astro`). Body text stays sans. It mirrors `web-app/src/styles/theme.css.ts`; keep in sync.
- `HeroVideo.astro`: `<video loop muted controls poster>` with fallback link, never server-renders `autoplay`; `lib/reduced-motion-video.ts` adds autoplay client-side only if `prefers-reduced-motion` is not `reduce`, deferred to idle (2 s cap) for Lighthouse. Reuse this module for tour clips.
- `FeatureCard.astro`: clip tile = `<video controls muted loop playsinline preload="none" aria-label="<heading> demo: <alt>">`, `data-poster` set by script, `<noscript><img>` fallback; tile degrades to image, then text-only when neither exists. `alt` is required whenever `screenshot` is set.
- `FeatureGrid.astro`: grouped by 5 README categories, `minmax(260px,1fr)` grid, explicit 1-column at <=480px, `overflow-x: hidden`.
- `tests/e2e/website-demos/clips.ts`: clips are scripted scenes with **burned-in captions** (`caption(page, ...)`) and a title card; no `<track>`/VTT anywhere in `website/src` or `scripts` (grep found none). Clips are `.webm`.
- `src/pages/` contains only `404.astro`; the landing is `content/docs/index.mdx`, so `/tours` pages are either Starlight content-collection docs (get sidebar, search, a11y chrome for free) or custom `src/pages/tours/*.astro` (full layout control, but must hand-build nav/skip links). Recommendation below.

## 2. Comparable patterns (GENERAL-KNOWLEDGE)

- **Linear / Vercel / Zed landing pages**: one dominant claim per section, a looping muted clip or still next to 1-2 lines of copy, alternating left/right media. Short clips (5-15 s), no audio, poster visible immediately. Docs are separate from marketing.
- **Warp**: feature pages pair a short looping clip with a "what you do / what you get" caption; heavy use of dark-theme product shots matching the site theme.
- **Vercel docs / Stripe docs style tours**: numbered steps in prose, each step with a screenshot or code block, a persistent "on this page" TOC; steps are text first so the page is useful without media.
- **Takeaway for us**: the clip is the hook, the numbered steps are the substance. Do not make a tour a video-only page; the text steps carry SEO, accessibility, skimming and the no-clip fallback.

## 3. Recommended tour page structure

1. `h1` (Cinzel via existing rule) + one-sentence outcome ("See which of your agents is waiting on you in 10 seconds").
2. Meta row: who it is for (evaluator / existing user), time to read ("2 min"), prerequisites/feature flag if any.
3. Clip (`<video>` per HeroVideo pattern) with poster = first-step screenshot, visible controls, caption text below, "Transcript" `<details>`.
4. Numbered steps (`<ol>`): each step = imperative sentence, annotated screenshot (`<figure>` + `<figcaption>`), optional "Why it matters" line. Annotations: numbered gold badges that match the step numbers, drawn in the recorder (CSS overlay at capture time) so they stay in sync with the build; do not hand-edit images (no committed binaries).
5. "Try it" block: install command (`InstallCommand.astro`) and link to the in-app `/help` doc or website help page for the same feature (supports the registry docs-coverage gate).
6. Prev/next tour links and a link back to the index.

**Index page**: grid reusing `FeatureGrid`/`FeatureCard` look (same tokens), cards grouped by the same 5 categories, each card = poster, title, outcome line, duration. Tours ordered by evaluator value first (what is this -> dashboard -> approvals -> review), advanced tours later. Keep the card component shared so style drift is impossible.

**Authoring model**: one content entry per tour (frontmatter: `title`, `outcome`, `clip`, `steps[]` with `screenshot`, `alt`, `text`, `featureIds[]`) plus one clip spec; page template is generic. Matches the "adding a tour = one spec + one entry" requirement. The recorder already has a name -> file contract, so frontmatter `clip: <name>` joins on it. Embed the full tour contract in a typed schema (Astro content collection `z.object`) so a missing `alt` fails the build rather than shipping.

## 4. Accessibility (WCAG 2.2 AA; GENERAL-KNOWLEDGE unless noted)

- **Captions / alternatives for video (1.2.1, 1.2.2, 1.2.3, 1.2.5)**: clips are silent, so 1.2.1 (prerecorded audio-only/video-only) requires a text alternative or audio description track: provide the numbered steps plus a per-clip transcript `<details>`. The burned-in captions (VERIFIED in `clips.ts`) are a convenience, not a conformant substitute: they are not selectable, not resizable, not translatable. Cheap upgrade: emit a `.vtt` from the same `caption()` calls (timestamps are known at record time) and add `<track kind="captions">`/`kind="descriptions"`; also use the same data to generate the transcript. Flag as optional if CI complexity is a concern, but the transcript is the minimum.
- **Pause/stop/hide (2.2.2)**: any motion auto-playing >5 s needs a pause control. Native `controls` (already used) satisfy this; loop + autoplay only when `prefers-reduced-motion` is not `reduce`. Reuse `initHeroVideos`; never autoplay tour clips that are below the fold (`preload="none"`/`metadata`, play on intersection at most).
- **Reduced motion**: also disable CSS transitions/parallax on tour pages under `@media (prefers-reduced-motion: reduce)`; the poster remains the content.
- **Keyboard (2.1.1, 2.4.7, 2.4.3)**: native `<video controls>` is keyboard-operable; do not wrap it in a click-to-play div. Skip link and visible focus ring in gold (check contrast against `#0c0a08`); step images are not focusable unless they open a lightbox. If a lightbox is added it needs focus trap, Esc, return focus, which is cost for little benefit: prefer inline full-width images that are readable without zoom.
- **Alt text (1.1.1)**: describe the point of the screenshot, not its pixels ("Approval prompt for `git push` with Allow and Deny buttons; Allow is focused"), 1-2 sentences; decorative annotation overlays are baked into the image so the `figcaption` must restate what the numbered markers mean. Keep `alt` required in the schema (the card already requires it).
- **Contrast (1.4.3, 1.4.11)**: the CSS already notes textMuted `#a08870` WCAG choices; annotation badges (gold on near-black, near-black text on gold) must meet 4.5:1 for text and 3:1 for the shape; recording in the Grimdark theme guarantees this only if the badges use theme tokens.
- **Text reflow / zoom (1.4.4, 1.4.10)**: screenshots with embedded UI text will not reflow; ensure essential information is also in the step text.
- **Headings/landmarks**: one `h1`, steps as `h2`/`ol`; `<figure>` + `<figcaption>`; `aria-label` on each video matching the FeatureCard pattern.
- **Existing gates**: website a11y/Lighthouse checks already run; add tour URLs to the checked set so a missing alt or contrast regression blocks.

## 5. Mobile vs desktop

- Desktop: two-column step rows (text left, screenshot right) at >=1024px; clip full content width, 16:9 (`aspect-ratio` already used in HeroVideo).
- Mobile (375px class): single column; screenshot after step text; clip full width with native controls; `playsinline` required (iOS would otherwise go fullscreen), `muted` required for any autoplay. No horizontal scroll (FeatureGrid sets `overflow-x: hidden`, which can hide overflow bugs: use a real fix, and add a tour page to the existing mobile overflow check).
- Capture strategy (requirements say both form factors are considered): record clips at desktop 16:9 only; for mobile-relevant tours capture a separate **mobile viewport screenshot set** (e.g. 390x844) of the app's responsive layout and show it as a small phone-framed figure inside the step, not as a second video. Serve responsive images via `srcset` with a 2x capture only where text legibility needs it, to protect the Pages artifact size budget. Prefer `.webm` plus an `.mp4` (H.264) fallback only if Safari support for the chosen codec fails (GENERAL-KNOWLEDGE: Safari plays VP9 webm on recent versions but not reliably older ones; verify against the target matrix before dropping mp4).
- Bandwidth: `preload="none"` + poster on mobile; do not autoplay on data-saver. Lazy-load step images (`loading="lazy"` with explicit `width`/`height` to avoid CLS, as FeatureCard does).

## 6. Grimdark theme consistency (naming: "Grimdark" only in public copy)

Naming rule (requirements.md, Naming / IP Constraint): the theme is called only "Grimdark" in tour copy, alt text, captions, transcripts, media filenames, page titles and the recorded UI's theme label; no trademarked names or imagery. Current source still uses the old file name (`wh40k.css`, cited above as-is); the requirements put its rename in scope, so tour templates, recorder assertions and clip names must use Grimdark identifiers only. Alt text should describe colors/mood ("dark parchment-and-gold interface"), never the franchise. Avoid franchise iconography in tour chrome and annotations.

- Use existing tokens only (`--sl-color-*`, `--sl-font-display`); no new colors. Tour page headings inherit Cinzel from the global `h1-h4` rule; step numbers in Cinzel gold (`--sl-color-text-accent`) read as "chapter markers", consistent with the gothic theme without extra theming effort.
- Cards/figures: `--sl-color-gray-6` background, `--sl-color-gray-5` 1px border, radius matching HeroVideo's `0.75rem`, gold shadow tokens `--sl-shadow-*` for hover.
- Keep body copy sans (the CSS header says this is deliberate for legibility); do not set long step text in Cinzel.
- Recorded media in the app's Grimdark theme is cohesive with the site; add a recorder assertion that a theme token (e.g. computed background `rgb(12,10,8)` on `body`) matches before capture (already required by requirements), and wait for Cinzel to load (`document.fonts.ready`) to avoid fallback-serif frames.
- Avoid theme-heavy flourishes (ornate borders, iconography) on tour chrome: the media itself is dark and busy; chrome stays minimal.

## 7. Error and fallback states when a clip is missing

Current state (VERIFIED): `FeatureCard` degrades clip -> screenshot -> text-only; the old pipeline fell back to committed stale screenshots. Requirements now forbid silent fallback to stale media.

Recommended separation of **build-time** vs **run-time** behavior:

| Situation | Build/CI | Page rendering |
|---|---|---|
| Clip recorded | pass | video + poster + transcript |
| Clip failed, screenshots OK | **fail the freshness check** (required by observability section) | n/a, build blocked on main; on a PR preview, render steps + screenshots only |
| Screenshot missing for a step | fail the build (schema-required file existence) | n/a |
| Video fails to load in browser (network, codec) | n/a | `<video>` inner fallback: "Your browser can't play this clip. Download it" link (pattern already in HeroVideo) plus the poster, steps still intact |
| JS disabled | n/a | `<noscript>` poster image (FeatureCard pattern); no autoplay |

Principle: a missing clip must be a loud CI failure, never a quietly worse page; but a page that does render must never be an empty box, which the step text guarantees. Do not render "Clip coming soon" placeholders on production; an unrecorded tour is excluded from the index by the manifest instead (asset-health manifest already exists).

## 8. Job-to-be-done lens

| | Evaluator (lands from README/search) | Existing user |
|---|---|---|
| Job | "Decide in under 2 minutes whether this would help me run several AI agents without babysitting them." | "Learn a feature I have not used / remember how a flow works." |
| Entry | Landing hero, features grid, README link | In-app `/help`, website help, search |
| Needs | Outcome-first claim, real-looking data, proof it is current, install friction, what it does not do | Exact steps, flags/prereqs, keyboard shortcuts, links to reference |
| Best format | Short clip + 3-5 steps; index ordered by value | Steps + screenshots; deep link into the help doc |
| Failure mode | Stale or toy-looking UI, no obvious next step | Tour too shallow, no link to reference |

Design implications: (1) the tours index doubles as the evaluator's guided path, so give it a "Start here" 3-tour sequence (dashboard -> approvals -> review) with prev/next; (2) each tour ends with two CTAs: "Install" (evaluator) and "Read the help page" (existing user); (3) keep tours short (target <=2 min read, <=60 s clip, <=6 steps) and link out rather than expanding; (4) seeded data should look like a believable team's work (named repos, PR titles) but contain no real hostnames/paths (public classification); (5) show a visible "recorded from build <short sha>, <date>" line, which turns the freshness guarantee into evaluator trust and is cheap since the pipeline already knows the commit.

## 9. Open decisions for planning

1. Starlight content-collection pages (recommended: free nav/search/a11y, consistent theme) vs custom `src/pages/tours` (more layout control). Tour index card grid can still be a custom component inside an MDX/Astro page.
2. Transcript only, or also generated VTT tracks. Recommend transcript from step data now; VTT as a follow-up since captions are already authored in `clips.ts` and could be emitted by the recorder.
3. Mobile screenshot set scope: which tours get a phone-frame figure.
4. Whether landing hero embeds one tour clip (reuse `HeroVideo`) or stays on `demo.webm` replaced by a CI-recorded clip.
5. Where README embeds media: link to site tour pages (no binaries in repo, always current) vs release assets; UX favors links to the tour pages because they carry captions/steps and the freshness stamp.
