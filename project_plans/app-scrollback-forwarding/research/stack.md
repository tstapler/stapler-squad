# Stack Research: app-scrollback-forwarding

Agent 1 (Stack Research), SDD Phase 2. Scope: what libraries/patterns/versions this
feature needs, what already exists in-repo to build on, and whether Claude Code /
Antigravity (agy) / pi actually expose a terminal-forwardable scroll command for their
own internal transcript views.

## TL;DR

- **No new dependencies are needed.** This feature is 100% composed of existing
  repo primitives: raw-byte PTY writes (`Instance.SendKeys`), the existing
  `Program`/adapter-detection machinery (`resolveHistoryAdapter`), and the existing
  `ScrollbackRequest`/`ScrollbackResponse` WebSocket plumbing. The work is protocol
  logic and per-CLI keystroke sequences, not a new library.
- **Claude Code confirmed: `Ctrl+O` (byte `0x0F`) toggles its transcript viewer**, and
  in **fullscreen** rendering a `[` keystroke dumps the full transcript into the
  terminal's *native* scrollback — a built-in escape hatch that may let us reuse the
  already-shipped tmux `capture-pane` path instead of parsing app-internal redraws.
  But Claude Code's fullscreen mode renders on the **alternate screen buffer** (like
  vim/htop) and virtualizes content — confirming the requirements doc's stated risk.
- **pi has the most explicit, cleanly documented scroll-forwarding surface** of the
  three: dedicated `tui.altScreen.*` keybinding actions (`pageUp`/`pageDown`,
  `lineUp`/`lineDown`, `top`/`bottom`) that target its alt-screen transcript region —
  by far the easiest adapter to prototype against.
- **agy (Antigravity CLI) has the same alt-screen/inline duality** as the other two,
  with `always`/`never`/`default`(adaptive) rendering modes — but its docs (fetched
  during this research pass) did not surface a specific scroll-keybinding ID list;
  that requires one more doc pass (its CLI Reference / default keymap page) before
  implementation, flagged as an open item below.

## 1. What already exists in this repo (no new dependencies)

### 1.1 Foreground-program detection is already solved

`Instance.GetProgram()` (`session/instance_terminal.go:38-48`) returns the running
program name via the lock-free `Snapshot()` accessor (see
`.claude/rules/instance-lock-free-reads.md` — this feature's own detection logic must
follow that rule, not read `i.Program` directly). This is the same string
`resolveHistoryAdapter(program string)` (`session/history_adapter.go:21-29`) already
uses to pick between `ClaudeAdapter` and `AgyAdapter` for history import/export.

```go
// session/claude_adapter.go:27
func (a *ClaudeAdapter) CanHandle(program string) bool {
	return strings.Contains(strings.ToLower(program), "claude")
}
```

`resolveHistoryAdapter` currently only wires up `claude` and `agy`
(`session/history_adapter.go:22-27`); `pi` is detected elsewhere via a standalone
`isPi(i.Program)` helper referenced at `session/instance.go:1828,2265`. **This means
the requirements doc's open question "can the foreground process be identified
without new app-specific process sniffing?" is already answered: yes** — the
`HistoryAdapter.CanHandle`-style pattern (or a new small interface following the same
shape, e.g. `ScrollForwarder.CanHandle(program string) bool` +
`ScrollGesture(direction) []byte`) is the natural place to add per-adapter scroll
support, mirroring the existing `HistoryAdapter` registry pattern rather than
inventing a new detection mechanism.

### 1.2 PTY input is raw bytes, not tmux CLI key-name syntax

Despite the name, `Instance.SendKeys` / `TmuxSession.SendKeys` does **not** shell out
to `tmux send-keys -t <session> "C-o"`. It writes raw bytes directly to the PTY master
file descriptor:

```go
// session/tmux/tmux.go:2011-2017
func (t *TmuxSession) SendKeys(keys string) (int, error) {
	file, err := t.GetPTY()
	if err != nil {
		return 0, err
	}
	return file.Write([]byte(keys))
}
```

`Instance.SendKeys` (`session/instance_tmux.go:1233-1238`) delegates to
`i.pm().SendKeys(keys)`. This means forwarding "Ctrl+O" to Claude Code is simply
`inst.SendKeys("\x0f")` (byte `0x0F`) — no tmux-specific key-name translation layer is
needed, and no new library. The `ProcessManager` interface
(`session/process_manager.go:34`) is the seam; both `TmuxBackend` and
`TymuxBackend` (`session/tmux_backend.go:65`, `session/backend_tymux.go:117`)
implement `SendKeys(keys string) (int, error)` identically, so a scroll-forwarding
helper written against `Instance.SendKeys` works across both backends without
branching.

One existing guardrail to reuse, not reinvent: `session/pane_submit.go:19-48`
documents (and a test enforces, `TestSessionPackage_NoDirectSendKeysPlusEnterConcatenation`)
that `SendKeys(content + EnterKeySequence)` in one write is a known bug pattern
(BUG-031) — multi-byte key sequences that must land as **separate** writes (e.g. an
escape sequence followed by a distinct keystroke) should follow that same
separate-write discipline if the eventual scroll gesture is more than one primitive
key.

### 1.3 The scrollback request/response wire format is already raw bytes end-to-end

`proto/session/v1/events.proto`:

```proto
// events.proto:175-177
message TerminalInput {
  bytes data = 1; // Raw input bytes (keystrokes, etc)
}

// events.proto:218-226
message ScrollbackRequest {
  uint64 from_sequence = 1; // Start from this sequence (0 for latest)
  int32 limit = 2;
}
message ScrollbackResponse {
  repeated ScrollbackChunk chunks = 1;
  bool has_more = 2;
}
```

Server-side, `handleScrollbackRequest` (`server/services/connectrpc_websocket.go:3116`)
and its shell-tab analog `handleShellScrollbackRequest` (:2415) both delegate the
actual data-fetch to an injected `onScrollbackRequest(startLine, endLine string)
(string, error)` closure (:3010, wired at :1343 and :2053), which today always calls
`Instance.GetScrollbackHistory` → `i.pm().CapturePaneContentWithOptions(...)` →
`tmux capture-pane` (`session/instance_tmux.go:1063-1069`). **This closure is the
integration point**: when the foreground program is one with app-managed scrollback,
swap this closure's implementation (or branch inside it) to send the forwarded scroll
keystroke + a capture, instead of a plain `tmux capture-pane` history slice. No proto
change is needed for the initial exchange shape — `ScrollbackRequest`/`Response` can
carry the app-scrollback "page" the same way it carries tmux history today, though a
follow-up proto field (e.g. a `source` enum: `TMUX_HISTORY` vs `APP_TRANSCRIPT`) may
be worth adding in the plan phase so the client can render a "viewing the app's own
scrollback" indicator per the requirements doc's open question.

Client-side, `web-app/src/lib/hooks/useTerminalStream.ts:81,730` exposes
`sendInput: (input: string) => void`, and
`web-app/src/components/sessions/TerminalOutput.tsx:1057` already calls
`requestScrollback(oldestSequenceReceivedRef.current, 500)` on scroll-up — the
forwarding logic is almost entirely server-side; the client's existing scroll-trigger
wiring does not need to change shape, only what the server returns for it to render.

### 1.4 Capture side: `CapturePaneContentWithOptions` / `CapturePaneContentRaw`

`session/instance_tmux.go` has several capture variants already
(`CapturePaneContent`, `CapturePaneContentPriority`, `CapturePaneContentRawPriority`,
`CapturePaneContentRaw`) — all backed by `tmux capture-pane`. After sending a forwarded
scroll gesture, the redraw needs to be captured with one of these (most likely
`CapturePaneContentRaw`/`RawPriority`, since the "raw" variant is documented as the
non-tmux-fallback path for control-mode streaming — see its doc comment at
`session/instance_tmux.go:953-956`) rather than introducing a new capture mechanism.

## 2. Claude Code: confirmed transcript-scroll keybindings

**Installed version in this environment: `claude --version` → `2.1.270 (Claude Code)`.**
(Ran directly; no `.claude/CLAUDE.md`/skill override needed — it's on `PATH`.)

From the official docs (`https://code.claude.com/docs/en/interactive-mode`, fetched
2026-09-14):

| Key | Byte(s) | Effect |
|---|---|---|
| `Ctrl+O` | `0x0F` | Toggle transcript viewer — "detailed tool usage and execution, with a timestamp and model used on each assistant message," expands collapsed MCP-call lines |
| `{` / `}` (while transcript open) | `0x7B` / `0x7D` | Jump to previous/next user prompt, vim-paragraph-motion style. **Requires fullscreen rendering.** |
| `[` (while transcript open) | `0x5B` | **"Write the full conversation to your terminal's native scrollback so Cmd+F, tmux copy mode, and other native tools can search it."** Requires fullscreen rendering. |
| `v` (while transcript open) | — | Write conversation to temp file, open in `$VISUAL`/`$EDITOR`. Requires fullscreen. |
| `q`, `Ctrl+C`, `Esc` | — | Exit transcript view |
| `?` (while transcript open) | — | Shortcut help panel (fullscreen only) |

The `[` binding is the standout finding: Claude Code ships a **built-in mechanism
specifically for dumping its virtualized transcript into the terminal's real,
tmux-capturable scrollback**. If feasible to drive headlessly (`Ctrl+O` then `[`),
this could let the "app scrollback" case reuse the *existing*, already-shipped
`tmux capture-pane` path (§1.4) almost unchanged, rather than needing to parse a
live re-rendered screen after each scroll step — worth prioritizing as the first
thing the plan phase's Claude Code prototype tries.

### 2.1 The virtualization risk is real and version-gated on rendering mode

Per `https://code.claude.com/docs/en/fullscreen` (fetched 2026-09-14):

> Fullscreen rendering... draws the interface on the terminal's alternate screen
> buffer, like `vim` or `htop`, and **only renders messages that are currently
> visible**.

Fullscreen mode is opt-in via `/tui fullscreen` or `CLAUDE_CODE_NO_FLICKER=1`, and the
docs explicitly flag **tmux** as one of the terminal contexts where fullscreen mode's
benefits (no flicker, flat memory) are "most noticeable" — meaning stapler-squad users
running Claude Code inside its tmux-backed sessions are a plausible population to have
this mode on, whether by their own choice or a future stapler-squad default. In
**classic** (non-fullscreen) rendering, Claude Code does not use the alternate screen,
so plain `tmux capture-pane` scrollback may already partially work there today for
what's been flushed to the primary screen — meaning **whether "scrolling does nothing"
reproduces depends on which renderer the user has selected**, an open item worth
confirming empirically in the validate/implement phase (check via `claude`'s
`/tui` output or `CLAUDE_CODE_NO_FLICKER`/`CLAUDE_CODE_DISABLE_ALTERNATE_SCREEN` env
vars, both mentioned in the fullscreen doc, rather than guessing).

Confirms the requirements doc's stated Feasibility Risk verbatim: "If an app's own
scrollback is virtualized (only ever renders a visible window, never writes full
history to the terminal even transiently), forwarding a scroll keystroke may only
reveal what the app chooses to redraw" — this is exactly fullscreen mode's behavior,
*except* for the `[` transcript-viewer export mechanism above, which is the one
documented escape hatch.

## 3. Antigravity CLI (agy): same alt-screen/inline duality, keybindings not yet located

Per `https://antigravity.google/docs/cli/settings/` (fetched 2026-09-14), agy's TUI
has the identical two-mode split as Claude Code and pi:

- **Alt-screen mode** (`altScreenMode: "always"`): "Integrated scrollback, mouse-wheel
  scrolling support, custom rendered scrollbar" — "Best used for: Standard local
  development sessions in advanced terminal emulators."
- **Inline mode** (`altScreenMode: "never"`): "Preserves entire session history inside
  your emulator's native scrollback buffer... Best used for: Remote SSH terminals,
  **terminal multiplexers like `tmux` or `screen`**."
- **Default** (`altScreenMode: "default"`): "Adaptive mode. Uses Alt-screen on
  advanced local terminals and degrades to inline mode over SSH."

Notably, agy's own docs recommend inline mode specifically for tmux — meaning agy
running inside a stapler-squad tmux session may, depending on how "adaptive" detects
the tmux TTY, already default to a mode where native `tmux capture-pane` scrollback
works correctly, unlike Claude Code's fullscreen mode. This should be verified
empirically (`agy --sandbox` in a throwaway tmux pane, check `/config`'s reported
`altScreenMode`) before assuming agy needs the same forwarding treatment as Claude
Code.

Keybindings are stored in `~/.gemini/antigravity-cli/keybindings.json`
(`cli.clear_screen`, `prompt.insert_newline`, `edit.open_editor` were the only example
action IDs shown on the settings page). **This research pass did not locate agy's
specific alt-screen scroll-action IDs** (the settings page didn't enumerate them,
unlike pi's dedicated keybindings reference) — the CLI Reference page
(`https://antigravity.google/docs/cli/reference`) or a live `agy` session's `?`/help
listing is the next place to check; flagged as an open item for the plan/implement
phase rather than guessed here.

## 4. pi: the most explicit scroll-forwarding surface of the three

Per `https://github.com/earendil-works/pi/blob/main/packages/coding-agent/docs/keybindings.md`
(fetched 2026-09-14), pi documents dedicated, individually-keybindable actions for
its "TUI Fullscreen Viewport" (alt-screen) transcript region, active under
`--tui-mode fullscreen`:

| Keybinding ID | Default key | Effect |
|---|---|---|
| `tui.altScreen.pageUp` / `pageDown` | `pageUp` / `pageDown` | Scroll transcript one page |
| `tui.altScreen.halfPageUp` / `halfPageDown` | *(unbound)* | Half-page scroll |
| `tui.altScreen.lineUp` / `lineDown` | *(unbound)* | Single-line scroll — bindable, useful for a smooth "load more" gesture |
| `tui.altScreen.top` / `bottom` | `home` / `end` | Jump to transcript start / follow-latest |
| `tui.altScreen.previousPrompt` / `nextPrompt` | `ctrl+shift+up`/`ctrl+up`, `ctrl+shift+down`/`ctrl+down` | Jump between marked messages (mirrors Claude Code's `{`/`}`) |
| `tui.altScreen.search` | `ctrl+shift+f` (`ctrl+f` on Win/WSL) | Search rendered transcript |

Outside fullscreen mode ("default mode" in pi's own terminology), these same keys
control the prompt editor instead — "Fullscreen transcript bindings take precedence
over editor bindings" only while `--tui-mode fullscreen` is active, so **whether a
forwarded `pageUp` reaches the transcript or the editor is entirely conditional on
which rendering mode the pi session is in** — the same detection problem as Claude
Code and agy, just with a much better-documented, individually-addressable action set
once you're in the right mode. Given pi's docs are the clearest of the three, and its
scroll actions are the most granular (single-line, half-page, full-page, all with
stable keybinding IDs a user could remap — which matters if stapler-squad ever needs
to detect a *customized* keymap rather than assuming defaults), **pi is a strong
second-choice prototype target after Claude Code**, ahead of agy pending the
keybinding-ID gap above.

## 5. Cross-cutting pattern and version notes

- **No new Go or TS dependencies.** `go.mod`/`package.json` need no changes — this is
  new logic over already-vendored PTY/proto/WebSocket plumbing (§1).
- **Adapter pattern to extend, not replace:** `HistoryAdapter`
  (`session/history_adapter.go:7-16`, `CanHandle(program string) bool` +
  Import/Export) is the existing precedent for "per-agent-CLI behavior selected by
  `Instance.GetProgram()`." A parallel small interface (e.g. `ScrollAdapter` with
  `CanHandle` + a method returning the byte sequence(s) to send for a scroll gesture)
  keeps detection logic in one place and avoids a second, divergent "is this Claude
  Code" check growing elsewhere in the codebase.
- **All three target CLIs gate their forwardable scroll surface behind an alt-screen
  ("fullscreen") rendering mode** that the user (or an env var) chooses, not
  something stapler-squad controls today. The plan phase needs a decision: detect the
  active rendering mode per adapter (env var / `/tui` output / `/config` query) before
  attempting to forward a scroll keystroke, versus documenting a rendering-mode
  prerequisite for this feature to work. This determines whether "detect foreground
  program" (§1.1, already solved) is sufficient, or whether "detect foreground
  program's *rendering mode*" becomes new required scope.
- **Version pinning is not a concern for this feature** — none of the findings above
  are from unstable/nightly docs; Claude Code's documented shortcuts note minimum
  versions per-shortcut only for recent additions (e.g. "`Ctrl+E` rebind... Requires
  Claude Code v2.1.208+"), well below the installed `2.1.270`.

## Sources

- `claude --version` (run in this environment) → `2.1.270 (Claude Code)`
- [Interactive mode – Claude Code Docs](https://code.claude.com/docs/en/interactive-mode) — keyboard shortcuts, transcript viewer table
- [Fullscreen rendering – Claude Code Docs](https://code.claude.com/docs/en/fullscreen) — alt-screen/virtualization behavior, `/tui` command, env vars
- [Settings, Rendering & Keybindings – Google Antigravity Docs](https://antigravity.google/docs/cli/settings/) — `altScreenMode` modes, keybindings file format
- [pi coding-agent keybindings.md (earendil-works/pi, main branch)](https://github.com/earendil-works/pi/blob/main/packages/coding-agent/docs/keybindings.md) — `tui.altScreen.*` action table
- In-repo: `session/instance_terminal.go:38-48`, `session/history_adapter.go`,
  `session/claude_adapter.go:27-29`, `session/agy_adapter.go:29`,
  `session/instance.go:1828,2265` (`isPi`), `session/tmux/tmux.go:2011-2017`,
  `session/instance_tmux.go:881,1063-1069,920-1054`, `session/process_manager.go:34`,
  `session/tmux_backend.go:65`, `session/backend_tymux.go:117`,
  `session/pane_submit.go:19-48`, `proto/session/v1/events.proto:175-226`,
  `server/services/connectrpc_websocket.go:1343,2053,3010,3116,2415`,
  `web-app/src/lib/hooks/useTerminalStream.ts:81,730`,
  `web-app/src/components/sessions/TerminalOutput.tsx:1057`
