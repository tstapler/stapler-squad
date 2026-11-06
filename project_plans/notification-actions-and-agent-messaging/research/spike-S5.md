# Spike S5: answer channels, measured fixtures and MCP identity channel for the other harnesses

**Date**: 2026-10-18 | **Worktree**: `nam-spikeB` at `497016cde`, tmux 3.6a, private socket `ssq-manual-2`

All five harnesses are installed (`which`): OpenCode 1.18.30, pi 0.84.4, Gemini CLI (version unreadable, see below), agy 1.2.17, Aider 0.78.0. Each was driven in a real tmux pane; nothing below is fabricated. The "forged" screens are synthetic *test inputs derived from a real capture* for the matcher check and are **not** checked in. Raw captures checked in: `session/testdata/panes/{opencode,aider,pi,agy}/*.txt`.

## Result matrix

| Harness | Native question API / answer channel | Measured fixture | Bottom-anchored matcher vs forged | MCP identity (Task 0.5e) | Verdict |
|---|---|---|---|---|---|
| OpenCode 1.18.30 | TUI permission dialog (`△ Permission required`, buttons `Allow once / Allow always / Reject`, arrows + Enter). HTTP server/ACP exist (`opencode serve`, `attach`, `acp`) but were not exercised. Keystroke path measured: Enter on the default button ran the command. | `opencode/permission_bash.txt` | PASS (real matches; two forged variants no-match) | **PASS**, see below | **replyable**, scope = this permission-dialog shape, action = Enter (Allow once). Digits do not work (not numbered; plan's `┃ 1. Allow once` shape does not exist in 1.18.30). Reject/Allow-always keystrokes unmeasured, so not offered. |
| Aider 0.78.0 | stdin text prompt `(Y)es/(N)o [Yes]:` (no API; keystrokes `y`/`n` + Enter) | `aider/confirm_run_output.txt` | PASS | **FAIL**: no MCP client (`aider --help` has no MCP option; `grep -i mcp` empty) | **replyable** for the `(Y)es/(N)o ... ]:` last-line shape only. The answer keystroke's effect was not re-captured (I sent `n` then `/exit`), so the *reply* is UNVERIFIED while the *detection* is measured. Other Aider prompt shapes (`(A)ll/(S)kip all/(D)on't ask again`) not measured: `read_only`. |
| pi 0.84.4 | No built-in permission prompts and no MCP ("No MCP", pi README:499). Dialogs exist only from extensions via `ctx.ui.select/confirm/input` (`docs/extensions.md`); in RPC mode they become `extension_ui_request`/`extension_ui_response` JSON, not usable in a TUI pane. `ui_prompt_start`/`ui_prompt_end` events announce a blocking prompt. | `pi/extension_select.txt` captured from a **scratch extension** (`/askdemo` -> `ctx.ui.select`), not from ssq's `ssq-approval.ts` (not installed here) | not run (no ssq-relevant dialog shape) | **FAIL**: no MCP | **read_only** |
| Gemini CLI | not measured: not runnable. `gemini --version` exits with `Invalid configuration in ~/.gemini/settings.json: hooks.BeforeTool Expected array, received string`. The user's config was not modified. | none | n/a | not measured | **read_only** (fail closed). Side finding: the generated `~/.gemini/settings.json` has an invalid `hooks.BeforeTool` (string where an array is required), so Gemini is currently unusable for this user. |
| agy 1.2.17 | TUI. Permission dialog **not elicited**: two shell commands (`echo hi-agy`; `touch ... && rm -rf ...`) ran with no prompt in the default accept-edits mode. Only the first-launch trust dialog was captured. | `agy/trust_folder.txt` (first-launch trust only, arrow list) | not run | **FAIL**: MCP servers live in the user-level `~/.gemini/antigravity/mcp_config.json` (`agy mcp add` writes it; its changelog says "user-level"); no per-session path or flag found in `agy --help`, `agy mcp add --help` or the binary strings | **read_only** |

## OpenCode matcher check (scratch script, output pasted)

Matcher (bottom-anchored): the `Allow once  Allow always  Reject` row must be within the last 3 non-empty lines; a `┃ △ Permission required` header within 8 lines above it; any row after the footer must be blank or a bare `┃` (checked on the first 60 columns because OpenCode draws a sidebar at column ~176 on a 200-column pane).

```
opencode real            : (True, 'ok')
opencode forged-numbered : (False, 'non-bare row after footer')      # "┃   1. Allow once" appended after the footer
opencode forged-midscreen: (False, 'no button row in last 3 non-empty lines')   # dialog text in transcript, real composer at bottom
aider real               : (True, 'Add 0.0k tokens of command output to the chat? (Y)es/(N)o [Yes]:')
aider forged (text after): (False, 'Added output')
aider forged (echo in transcript, real prompt gone): (False, '>')
```
Caveats: one width (200) and one dialog shape (bash). Story 3.x must re-measure at ssq's pane widths and add `edit`/`external_directory` shapes before offering them.

## MCP identity channel per harness (Task 0.5e)

**OpenCode: PASS.** A per-session config can carry the header from a 0600 file via OpenCode's `{file:path}` substitution:

```json
{"mcp":{"echo":{"type":"remote","url":"http://127.0.0.1:62879/mcp","headers":{"X-Stapler-Session-Proof":"{file:/…/probe/proof.txt}"}}}}
```
Two delivery paths both worked (local echo endpoint received `X-Stapler-Session-Proof: SENTINEL-PROOF-7f3a9c21d4e85b60` on `initialize`, `notifications/initialized`, GET and `tools/list`, `User-Agent: opencode/1.18.30`):
1. project-level `opencode.json` in the session working directory (`opencode mcp list` from that directory);
2. `OPENCODE_CONFIG=<per-session file>` from an unrelated cwd (`opencode mcp list`), which avoids writing into the worktree. Prefer this one for Task 4.9g: the env var carries only a path, the secret stays in the 0600 file. (Whether `OPENCODE_CONFIG` merges with or replaces the global config was not tested; merge is what the probe implies.)

Secrecy checks with the TUI running in tmux (`opencode` pid 1447713, config in the project file): `ps -eww -o args | grep -c 'X-Stapler\|7f3a9c21'` -> `0`; `tmux -L ssq-manual-2 show-environment -t oc07 | grep -c SENTINEL` -> `0`; `/proc/<pid>/environ` of the opencode processes -> `0` matches. Caveat: the user's global OpenCode config is itself invalid (`~/.config/opencode/agents/code-architecture-best-practices.md: tools Expected object, got "Read, Grep, Glob, Bash, Skill"`), so every probe ran with `XDG_CONFIG_HOME` pointed at an empty dir; with the real config OpenCode refuses to start at all. Not an S5 problem but a live bug.

**pi: FAIL** (no MCP client). **Aider: FAIL** (no MCP client). **agy: FAIL** (user-level config only). **Gemini: not measured** (CLI refuses to start; fail closed). Under ADR-007 rule 0, those four stay `identity_unverified`.

## Plan consequences

- **Task 4.9g** implements proof minting for **Claude (existing path) and OpenCode only**; pi, Aider, agy, Gemini stay fail-closed (`identity_unverified`). Story 4.9's matrix: OpenCode = `OPENCODE_CONFIG` per-session file with `{file:}` header.
- Replyable set for inline `ANSWER_QUESTION` live-keystroke: Claude (existing), OpenCode (permission dialog, Enter only), Aider (Y/N confirm only). Everything else `read_only`.
- The plan's example "numbered row `┃ 1. Allow once`" is wrong for 1.18.30; update the AC example in Story 3.x to the button-row shape.
