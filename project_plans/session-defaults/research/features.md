# Features Research: Session Defaults

## Tool Survey

### tmux sessionizer

Directory-based session discovery — scans configured paths for git repos and opens them. No profile/template system beyond the path itself. No per-field defaults, no env var support, no "save as default" UX.

**Gaps for Stapler Squad:** No named profiles, no pre-population beyond path.

---

### Wezterm

`config.lua` defines named "multiplexer domains" as profile-like constructs:

```lua
multiplexer_domains = {
  { name = "dev",  default_prog = {"zsh"}, env = { EDITOR = "vim" } },
  { name = "work", default_prog = {"bash"}, env = { EDITOR = "nvim" } },
}
```

- Global: `default_prog`, `launch_menu`
- Per-domain: default shell/program, environment variables, initial working directory
- Selection UX: user selects domain at attach time (`wezterm connect multiplexer`)
- Each domain persists its own config independently

**Pattern worth adopting:** named domains with env vars, selected at session create time.

---

### Zellij

Layouts stored as `.kdl` files in `~/.config/zellij/layouts/`:

```kdl
// work.kdl
default_shell "zsh"
env {
  EDITOR = "vim"
  PROJECT = "my-project"
}
```

- Global config: `~/.config/zellij/config.kdl` sets default layout, shell, theme
- Named layouts: select at creation with `--layout myprofile`
- Per-layout env vars passed to launched process
- No "save as default" from within a session; configs are files only

**Pattern worth adopting:** per-directory `.stapler-squad.json` config file (analogous to layout file checked into repo).

---

### iTerm2 Profiles (gold standard)

Best-in-class profile UX. Key traits:

**Data model:**
- Global "Default" profile — applies to all new windows unless overridden
- Named profiles: working directory, shell command, environment variables, badge/color, keyboard shortcuts
- Variable expansion in paths: `\(user.name)`, `\(local.hostname)`

**Selection UX:**
- Profile dropdown in new terminal dialog
- "Edit Session" right-click → save current session values as new profile
- Badges (colored labels) visually identify active profile
- Remembers last-used profile per workspace

**"Save as Default" flow:**
1. Create terminal with specific settings
2. Right-click → Edit Profile
3. Modal pre-populated with current values
4. Add name + description; checkbox: "Set as default for new terminals"
5. Save → immediately usable for future sessions

---

## UX Patterns Worth Adopting

### 1. Profile selector dropdown at session creation

```
[Select Profile: Work ▼]  [+ New Profile]
```

- Lists all named profiles + "None"
- Selecting profile auto-fills form fields
- User can override any field after selection
- "+" opens lightweight creation modal

### 2. Per-field source indicator

When form is pre-populated, show where each value came from:

```
Program:  claude        ← from "Work" profile
Tags:     [Work][Active] ← from "Work" profile
Branch:   main          ← from project .stapler-squad.json 📍
```

Subtle tooltip or badge per field. Builds mental model of the precedence chain without cluttering the form.

### 3. "Save as Profile" button in Review step

```
[Back]  [Cancel]  [Save as Profile…]  [Create Session]
```

- Opens modal pre-populated with current form values
- User adds name + description
- Optional "Set as global default" checkbox
- After save: profile immediately available; user stays in their workflow

### 4. Precedence chain: directory > profile > global

Matches Zellij + Wezterm conventions. Highest-priority wins per field. Tags: union. Scalars (program, flags): override.

---

## Field Inventory

Fields comparable tools expose as configurable defaults:

| Field | tmux-sessionizer | Wezterm | Zellij | iTerm2 | Stapler Squad |
|---|---|---|---|---|---|
| Program/shell | ✅ | ✅ | ✅ | ✅ | ✅ must-have |
| Working directory | ✅ | ✅ | ✅ | ✅ | ✅ must-have |
| Environment variables | ❌ | ✅ | ✅ | ✅ | ✅ must-have |
| Tags/labels | ❌ | ❌ | ❌ | ✅ badges | ✅ must-have |
| Initial prompt | ❌ | ❌ | ❌ | ✅ | ✅ should-have |
| Auto-yes / auto-approve | ❌ | ❌ | ❌ | ❌ | ✅ should-have |
| Branch default | ❌ | ❌ | ❌ | ❌ | ✅ should-have |
| Session type | ❌ | ❌ | ❌ | ❌ | ✅ should-have |
| Category | ❌ | ❌ | ❌ | ✅ folder | nice-to-have |

**Not recommended as defaults:**
- `path` — too project-specific; keep user-entered
- `title` — unique per session; not generalizable
- `existing_worktree` — path-specific; not generalizable

---

## Recommended Defaults Fields (Tier 1 scope)

```json
{
  "name": "Work Profile",
  "program": "claude",
  "tags": ["Work", "Active"],
  "auto_yes": false,
  "env": { "EDITOR": "vim" },
  "branch": "main",
  "session_type": "new_worktree",
  "prompt": "",
  "working_dir": ""
}
```

## Per-Directory File Schema (`.stapler-squad.json`)

Analogous to Zellij layout files checked into repos:

```json
{
  "program": "aider",
  "tags": ["Project-X", "Frontend"],
  "working_dir": "packages/core",
  "env": {
    "PROJECT_NAME": "Project X"
  }
}
```

Keeps directory-specific defaults portable and version-controllable. Complements (not replaces) the registered-path approach in config.json.

---

## Top 3 Recommendations for Stapler Squad

1. **Named profiles with dropdown selector** — profile at the top of the session wizard, before any other fields. Pre-fills everything; user overrides what they need.

2. **Visible source indicators on pre-populated fields** — tooltip or small badge (📍 or "from profile") so users understand the precedence hierarchy without reading docs.

3. **"Save as Profile" shortcut in Review step** — lets users capture a working session config as a reusable template without leaving the create flow. iTerm2 pattern with highest adoption rate.

---

## Known Pitfalls from Comparable Tools

| Pitfall | Tool | Mitigation |
|---|---|---|
| Profile proliferation (dozens of stale profiles) | iTerm2 | Soft delete / archive; show "last used" date |
| Outdated env vars (stale API keys in profiles) | Wezterm | Don't store secrets; note in description |
| User forgets which layer a value came from | All | Per-field source indicators |
| Directory config not distributed to teammates | Zellij | Suggest committing `.stapler-squad.json` for team repos |
| No migration path for config format | All | Version field + migration handler in `config.go` |
