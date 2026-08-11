# Seneschal Brand Guidelines

> Condensed brand guide — specific enough that a contractor who has never worked with this brand can produce on-brand work from this document alone.
>
> For foundational strategy, positioning, and the estate metaphor vocabulary, see `product-marketing-context.md`.

---

## 1. Color System

### Semantic Palette

All color decisions should reference semantic roles, not raw hex. Match these tokens to the CSS variables in `globals.css`.

#### Foundation (Backgrounds)

| Role | Token | Hex | Usage |
|------|-------|-----|-------|
| Canvas | `--background` | `#0f1117` | Page background, deepest layer |
| Surface | `--card-background` | `#161b22` | Cards, panels, drawers |
| Surface raised | `--hover-background` | `#1e2530` | Hover states on surfaces |
| Border | `--border-color` | `#1e293b` | Dividers, input borders |

#### Terminal Layer

| Role | Token | Hex | Usage |
|------|-------|-----|-------|
| Terminal canvas | `--terminal-background` | `#1e1e1e` | xterm.js background |
| Terminal chrome | `--terminal-surface-header` | `#252526` | Terminal header/tab bar |
| Terminal text | `--terminal-foreground` | `#d4d4d4` | Terminal output text |
| Terminal accent | `--terminal-accent` | `#4d8bf7` | Highlights, cursor |

#### Text

| Role | Token | Hex | Usage |
|------|-------|-----|-------|
| Primary | `--text-primary` | `#e2e8f0` | Body text, headings |
| Secondary | `--text-secondary` | `#94a3b8` | Labels, metadata, captions |
| Muted | `--text-muted` | `#7d8ea8` | Placeholders, disabled labels |
| Disabled | `--text-disabled` | `#9a9a9a` | Inactive controls |

#### Action

| Role | Token | Hex | Usage |
|------|-------|-----|-------|
| Primary | `--primary` | `#8B1A3B` | Buttons, icons, active indicators — deep burgundy |
| Primary hover | `--primary-hover` | `#A82248` | Button hover |
| Primary active | `--primary-active` | `#6D1530` | Button pressed |
| Gold accent | `--gold` | `#C9A84C` | Taglines, decorative accents, blade bits, seal rim |
| Gold hover | `--gold-hover` | `#D4B866` | Gold element hover states |
| Purple whisper | `--purple` | `#5B3678` | Used sparingly — one accent per composition |
| Primary text | `--primary-text` | `#EDE0D0` | Text on dark — warm cream, not pure white |

#### Status

| Role | Token | Hex | Usage |
|------|-------|-----|-------|
| Success | `--success` | `#22c55e` | Completed, approved, healthy |
| Warning | `--warning` | `#f59e0b` | Attention needed, uncommitted changes |
| Error | `--error` | `#ef4444` | Failed, denied, critical |
| Processing | `--processing` | `#4338ca` | In-progress, running |

#### Session Status Badges

These are the inline status chips on session cards. Use the full bg + fg pair — never mix them.

| Status | Background | Foreground | When |
|--------|-----------|-----------|------|
| Approval pending | `#fecaca` | `#991b1b` | Agent has a petition awaiting assent — highest urgency |
| Input needed | `#dbeafe` | `#1e40af` | Agent is waiting on user input |
| Complete | `#dcfce7` | `#166534` | Task finished cleanly |
| Uncommitted | `#fef3c7` | `#92400e` | Worktree has unmerged changes |
| Idle | `#f3f4f6` | `#374151` | Running, no action needed |

### Usage Rules

- **Never use raw hex in component code.** Always reference the CSS variable. This makes theme changes propagate correctly.
- **Dark-first.** There is no light theme. Do not design for it.
- **Indigo is authority, not decoration.** Use `--primary` for interactive elements and the active state of selected items only. Do not use it for decorative highlights.
- **Red means stop.** `--error` and the approval-pending badge are the only red elements in the UI. Do not use red for anything non-critical.
- **Terminal surfaces are their own world.** Do not mix terminal surface colors (`#1e1e1e`) with UI surface colors (`#161b22`). They are intentionally distinct layers.

---

## 2. Typography

### Typeface Stack

The project uses four fonts loaded via `next/font`. Each has a defined role — do not substitute.

| Typeface | Variable | Role | Why |
|----------|----------|------|-----|
| **Cinzel** | `--font-cinzel` | Display / Logo / Estate headings | Modeled on classical Roman monumental inscriptions — directly evokes the Seneschal name's historical roots. Authority without ornamentation. |
| **Inter** | `--font-inter` | UI body / labels / descriptions | The standard for legible, dense information UI. Neutral enough to carry any content. |
| **Rajdhani** | `--font-rajdhani` | Technical section headings | Geometric sans with a slightly industrial edge — appropriate for labeling technical structures (sessions, branches, tags). |
| **JetBrains Mono** | `--font-mono` | Code / terminal / paths / commands | Best-in-class developer monospace. All code-adjacent content uses this. |

### Type Scale

| Role | Font | Size | Weight | Line-height | Usage |
|------|------|------|--------|-------------|-------|
| Display | Cinzel | 32px | 700 | 1.2 | Logo wordmark, hero headline, marketing h1 |
| Heading 1 | Cinzel | 24px | 700 | 1.3 | Page titles, modal titles |
| Heading 2 | Rajdhani | 18px | 600 | 1.4 | Section headings, panel headers |
| Heading 3 | Rajdhani | 15px | 600 | 1.4 | Subsection labels, grouped item headers |
| Body | Inter | 14px | 400 | 1.6 | All descriptive text, form labels, README body |
| Body strong | Inter | 14px | 600 | 1.6 | Emphasis within body text |
| Small | Inter | 12px | 400 | 1.5 | Captions, metadata, timestamps, tag labels |
| Small strong | Inter | 12px | 600 | 1.5 | Status labels, badge text |
| Code inline | JetBrains Mono | 13px | 400 | 1.6 | Inline `code`, paths, commands |
| Code block | JetBrains Mono | 13px | 400 | 1.7 | Code blocks, terminal output |
| Terminal | JetBrains Mono | 13px | 400 | 1.5 | Live terminal content in xterm pane |

### Typography Rules

- **Cinzel is for the estate, not the engine.** Use it for display text, brand headings, and estate-vocabulary callouts. Do not use it for form labels, error messages, or dense UI text.
- **All code-adjacent content is JetBrains Mono.** This includes: inline code, command examples, file paths, branch names, session IDs, git hashes, and terminal output.
- **Minimum body size is 12px.** Nothing smaller in production UI.
- **No italic in the UI.** Italics are reserved for marketing copy and the estate metaphor voice examples. Use weight variation (400→600) instead of italic for emphasis in the product.
- **Letter-spacing on Cinzel display**: add `letter-spacing: 0.05em` at 24px+ for proper monumental spacing.

---

## 3. Logo System

### Mark Components

**Wordmark**: `SENESCHAL` in Cinzel Bold (700), uppercase, letter-spacing 0.08em. The full caps and spacing evoke Roman inscription — intentional.

**Icon**: A skeleton key with a grid of three parallel horizontal bars in the bow (the ring at the top of the key). The bars represent parallel agent sessions under unified authority. The key represents the seneschal's literal role — keeper of the keys to the estate.

**Combination mark**: Icon to the left of wordmark, separated by 1× the icon height in space. Icon and wordmark optically centered on the horizontal axis.

### Sizing & Clear Space

- **Minimum size**: 120px wide for the combination mark; 24px for icon-only
- **Clear space**: Equal to the cap-height of the `S` on all four sides of the full mark
- **Don't place the logo on a background lighter than `#1e293b`** — the mark is designed for dark surfaces

### Approved Variations

| Variant | When to use |
|---------|-------------|
| Full mark on dark (`#0f1117`) | Primary — README, website, presentations |
| Full mark on indigo (`#6366f1`) | Secondary — badges, social cards |
| Wordmark only (no icon) | Tight horizontal spaces, breadcrumbs |
| Icon only | Favicons, app icons, 32px and under |
| Mono white | Single-color contexts (embroidery, engraving) |

### Prohibited Uses

- No color gradients on the mark or icon
- No drop shadows, glows, or bloom effects
- No stretching or non-uniform scaling
- No placement on backgrounds with insufficient contrast
- No recoloring the icon to anything except `#6366f1`, `#818cf8`, or white
- No outlines or strokes added to the letterforms
- No AI-generated or playful variations of the icon (no robots, brains, sparkles, circuit boards)

---

## 4. Voice & Tone Matrix

### Personality Anchors

**Pragmatic** — Says exactly what is needed. No filler.
**Technically precise** — Uses the right word, not the accessible word.
**Quietly intellectual** — The estate metaphor is present but never forced.
**Unpretentious** — Expert-first without condescension.

### By Context

#### README / GitHub Hero
*Goal: Convince a developer in 60 seconds.*
- Declarative statements, not questions
- Estate metaphor is welcome here — this is the one place it gets full expression
- Numbers over adjectives ("42 built-in rules" beats "powerful approval engine")
- The aha moment is always the approval gate

> ✅ "Your agentic estate, managed. Run five AI coding agents in parallel — every petition heard, every holding visible."
>
> ❌ "Seneschal is a powerful AI agent management platform that helps developers streamline their workflow."

#### Onboarding / First Run
*Goal: Orient without hand-holding.*
- Warmer than the README, not warm
- Acknowledge the user knows what they're doing
- Frame first steps as taking possession of the estate

> ✅ "Your first holding is ready. Seneschal manages the routine — you'll only be summoned when a decision matters."
>
> ❌ "Welcome! Let's get you started with your first session! Here's how it works..."

#### Approval / Petition Interface
*Goal: Fast, unambiguous decision-making.*
- Formal and precise — this is the court
- Agent name + tool name + what it will do. Nothing more.
- The two options are always "Assent" and "Deny" (or "Approve" / "Deny" in technical labels)

> ✅ "claude-code is petitioning to delete `dist/` (3 files). Assent or deny?"
>
> ❌ "Your AI assistant would like permission to perform a file deletion operation. Would you like to allow this?"

#### Error Messages
*Goal: State what happened and what to do.*
- No apology. No "oops" or "uh oh".
- Cause first, remedy second.
- One sentence each.

> ✅ "Session failed to start. Verify tmux is installed and the path exists."
>
> ❌ "Oops! We couldn't start your session. Please make sure tmux is installed correctly and try again."

#### Empty States
*Goal: Acknowledge absence without drama.*
- Brief. Matter-of-fact. One line when possible.
- A dry hint is acceptable; never cheerful.

> ✅ "No holdings. Start a session to put your first agent to work."
>
> ❌ "You don't have any sessions yet! Click the button below to create your first one 🚀"

#### Notifications / Alerts
*Goal: Interrupt only when necessary; be scannable.*
- Session name first, status second, action third
- Never alert for informational events — only for petitions, failures, and completions that require human attention

> ✅ "Petition pending — `feat/payments` is waiting on your assent."
>
> ❌ "You have a new notification from one of your AI coding agent sessions!"

### Words to Use
`session`, `holding`, `petition`, `assent`, `deny`, `standing order`, `review queue`, `worktree`, `isolated`, `real-time`, `approval rule`, `coverage`, `estate`

### Words to Avoid
`AI-powered`, `revolutionize`, `seamlessly`, `effortlessly`, `unlock`, `supercharge`, `cutting-edge`, `empower`, `smart`, `intelligent`, `magic`, `oops`, `uh oh`, `please`

---

## 5. Imagery & Screenshot Standards

### What to Show
- **Real UI at full resolution** — screenshots of the actual Seneschal dashboard, terminal panes, approval drawer
- **Dense, active states** — show 3–5 sessions running simultaneously; empty states are not marketing material
- **The approval gate in action** — the petition interface is the product's most differentiating moment; it should appear in the README hero area
- **Terminal content** — real code output, real diffs, real agent interactions (sanitized, no credentials)

### What Not to Show
- No stock photography
- No abstract "AI" imagery (neural nets, glowing brains, circuit board patterns)
- No light-mode screenshots — the product is dark-first; always capture in dark theme
- No castle, estate, or medieval imagery — the estate metaphor lives in copy, not visuals
- No AI-generated illustrations

### Demo GIF Standards
- Maximum 30 seconds
- Show the aha moment: agent running → petition appears → user assents → agent continues
- Record at 1440×900 minimum
- Optimize to under 5MB (use `gifsicle` or equivalent)
- Caption the key moments with Rajdhani 15px labels

---

## 6. Layout & Spacing Principles

### Density Philosophy
Seneschal is a professional tool for developers who have many sessions running at once. **Information density is a feature, not a design failure.** Do not add whitespace to "breathe" — add it only to create clear visual hierarchy.

### Grid
- Base unit: 4px
- Common spacings: 4, 8, 12, 16, 24, 32, 48px
- Session cards: minimum 280px wide, no fixed maximum
- Sidebar: 240px fixed; 56px collapsed
- Terminal pane: fills available space; minimum 400px wide

### Hierarchy Rules
- Use weight (400→600) to create emphasis within a type role before changing size
- Use color (`--text-primary` → `--text-secondary` → `--text-muted`) to de-emphasize supporting content
- Status badges are the loudest element on a session card — they should be immediately scannable without reading anything else

### Borders & Radius
- Card borders: 1px `--border-color` (`#1e293b`)
- Border radius: 6px for cards, 4px for inputs, 2px for badges, 0 for terminal panes
- No rounded corners on terminal panes — the terminal is a raw surface

---

## 7. Quick Audit Checklist

Use this before shipping any external-facing asset:

- [ ] Colors reference CSS tokens, not raw hex
- [ ] No color lighter than `#1e293b` used as a background
- [ ] Cinzel is used only for display text and estate-vocabulary headings
- [ ] All code, paths, and commands are in JetBrains Mono
- [ ] No "oops", "seamlessly", "AI-powered", or emoji in UI copy
- [ ] Approval interface uses "Assent" / "Deny" language
- [ ] Empty states are one line, matter-of-fact
- [ ] Screenshots are dark-theme, real UI, active state with multiple sessions
- [ ] Logo has proper clear space; no drop shadows or gradients
- [ ] Red (`--error`, approval-pending badge) is used only for genuinely critical states

---

## Related Skills

| Skill | When to apply |
|-------|-------------|
| `pm-brand-strategy` | Source of truth for positioning, estate metaphor, and name vetting |
| `ui-frontend-design` | Executing these guidelines in React/vanilla-extract components |
| `ui-logo-designer` | Generating logo concepts from the mark description above |
| `ui-design-system` | Building or auditing the design token system |
