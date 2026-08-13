# UX Patterns Research: Terminal Toolbar Best Practices

## Popular Terminal Emulator Toolbar Ordering

### VS Code Integrated Terminal
Toolbar left-to-right: **New Terminal | Split Terminal** (creation) → **Kill Terminal** (destructive) → **Clear** → **Maximize** → **More Actions ▾** (overflow kebab)

Key insight: VS Code puts the most-used creation/management actions first, Clear is prominent, and infrequently used actions go in "More Actions." There is a hard visual separator between creation and management groups.

### iTerm2 (macOS)
Does not use a persistent toolbar; actions are in the Session menu and context menu. The status bar is user-configurable (drag-to-reorder). High-frequency items (split pane, new tab) are in the tab bar, not a per-terminal toolbar.

Key insight: When toolbars are configurable, users always move copy/paste and clear to primary positions.

### Hyper Terminal
Minimal toolbar, no per-terminal controls. New/close tabs via keyboard shortcuts. Philosophy: the toolbar is for session-level actions only (new, close), not terminal editing actions (copy, paste).

Key insight: Hyper separates "session control" (new/close tab) from "terminal editing" (copy/paste via keyboard) entirely. Editing actions require keyboard shortcuts.

### Warp Terminal
Top-right cluster: **Settings | New Tab | Split** (session controls). Command history and AI search are first-class. No per-terminal copy/paste buttons — selection copy is automatic.

Key insight: Modern terminal apps increasingly remove editing toolbar buttons in favor of auto-copy-on-select. However, stapler-squad serves an AI-agent use case where Paste (inserting file paths) and Copy (sharing output) are genuinely primary actions.

### tmux / screen (keyboard-first reference)
No GUI toolbar. Standard workflow: prefix + c (new), prefix + d (detach), prefix + [ (copy mode). Copy is the highest-frequency terminal action after typing.

---

## Button Frequency Classification

Based on cross-terminal UX research and the AI-agent use case of stapler-squad:

### Tier 1 — High Frequency (primary bar, leftmost)
- **Copy** — Users copy terminal output to paste into prompts, issues, chat. In an AI-agent terminal, this is the #1 output action.
- **Paste** — Critical for inserting file paths (from upload), commands from the AI, code snippets. In AI terminal workflows, Paste is called frequently.
- **Clear** — Used after a command sequence completes or when the terminal is cluttered. Medium-high frequency.
- **Bottom (scroll-to-bottom)** — Essential in AI terminals where output streams continuously. Very high frequency.

### Tier 2 — Medium Frequency (secondary position)
- **Gallery / Files** — Uploading files to AI context is a core stapler-squad workflow. Medium frequency but important to discover.
- **Mouse mode toggle** — Used when switching to vim/tmux. Low-medium frequency; needed when needed.

### Tier 3 — Low Frequency (de-emphasize or separate)
- **Resize** — Manual resize. Near-redundant with auto-fit. Low frequency.
- **Record** — Session recording. Occasional. Dev/power user.
- **Camera** (mobile only) — Niche upload path.

### Tier 4 — Dev/Diagnostic (hide or collapse)
- **Debug** — Developer-only. Not needed by end users.
- **Log Stream** — Developer-only remote debug tool.
- **Raw streaming mode select** — Developer-only protocol selector.

---

## Standard Pattern for Separating Dev Tools

### Pattern A: Right-aligned visual group with border separator
Used by browser DevTools (Sources tab toolbar), VS Code editor toolbar. Dev actions are right-aligned behind a `|` separator or `margin-left: auto` push. Always visible but spatially demoted.

Pros: Always reachable, no state management, clear visual hierarchy.
Cons: Still takes horizontal space; 13 → ~10 buttons (doesn't meet the ≤8 target).

### Pattern B: Collapsible "Dev ⚙" toggle group
Used by browser extension toolbars, Firefox DevTools "Customize" mode. A single toggle button expands an inline section.

Pros: Meets the ≤8 target, discoverable via labeled button, no portal/z-index issues.
Cons: Two-click access to dev tools; requires new state and CSS.

### Pattern C: Overflow/kebab "..." menu
Used by VS Code "More Actions ▾", Chrome DevTools panel "⋮" menu. All low-frequency actions hidden behind a three-dot button.

Pros: Cleanest primary bar, meets ≤8 target.
Cons: Lowest discoverability, requires absolute-positioned dropdown + createPortal (per CSS architecture rules).

### Pattern D: Dev mode activation gate (localStorage-based)
Dev buttons only appear when `localStorage['debug-terminal'] === 'true'`. Already used for the `debugMode` state in the component.

Pros: Zero noise for end users, perfectly clean toolbar.
Cons: Discoverability problem — new developers may not know the feature exists; support burden when something goes wrong.

---

## Evidence-Based Button Order Recommendation

Based on AI-terminal use frequency and cross-tool research, the recommended new order is:

```
PRIMARY BAR (always visible, ≤8 buttons):
[Copy] [Paste] [Bottom] [Clear] [Gallery] [Files] [Mouse] [Resize] | [Dev ⚙]

DEV GROUP (expanded inline when Dev toggle clicked):
[Debug] [Log Stream] [Record] [Raw▾]
```

Rationale:
1. Copy and Paste first — the two most frequent terminal actions in an AI-agent context
2. Bottom before Clear — scrolling to the end is more frequent than clearing
3. Gallery/Files before Mouse — file uploads are a core stapler-squad feature; mouse toggle is situational
4. Resize last in primary — near-redundant utility
5. Dev group behind a single toggle — meets ≤8 requirement, keeps all tools reachable

### Is "Mouse" primary-worthy?
Yes, but barely. Mouse mode is needed when a user opens vim, tmux, or any full-screen TUI inside the AI terminal. It's medium-frequency for power users and almost never needed by casual users. Recommendation: keep in primary bar but at the end (position 7/8).

### Should "Resize" be removed?
Not removed — demote to last position in primary bar. Auto-fit covers 95% of cases, but manual resize is a useful escape hatch when the container has been resized and the terminal hasn't caught up. Keeping it costs one button slot.

### The "Mouse" + "Resize" tradeoff
If the ≤8 target is strict and both Mouse and Resize must be present, consider grouping them with a labeled separator as "⚙ Controls" alongside dev tools, keeping the primary bar at 6 buttons (Copy, Paste, Bottom, Clear, Gallery, Files).
