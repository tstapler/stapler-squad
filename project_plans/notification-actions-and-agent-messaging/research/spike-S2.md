# Spike S2: hook maximum blocking duration and failure modes

**Result: PASS.** A hook blocked 300.0 s without Claude killing it, then the native dialog appeared; a hook timeout, crash (exit 1), invalid JSON, HTTP 500 and empty 200 all fail open to the native dialog. One trap: command-hook `exit 2` denies the tool call. Results await owner review with the S1/S3 batch; the default (`hook_block_ms = 0`) proceeds.

- Date 2026-10-18, Claude Code `2.1.303`, `haiku`, interactive TUI in private tmux `nam-spikeA`, permission mode `auto`. Same scratch harness as `spike-S1.md`. HTTP hook = tiny python `http.server` on 127.0.0.1:62875.
- Documented timeout: `timeout` field, default 600 s for `command`/`http` hooks (https://code.claude.com/docs/en/hooks, hook-handler fields table, line 180 of the saved page); docs say non-2xx, connection failures and timeouts are non-blocking errors for HTTP hooks.

## Measurements (VERIFIED)

| Experiment | Server/script log | Observed in TUI |
|---|---|---|
| command hook, `sleep 30`, empty stdout | `elapsed: 30.00031352043152` | at t+15 s: `Forming… (running PreToolUse hooks… 2/3 · 14s`; no dialog; at t+40 s the numbered dialog `1. PG ... Enter to select · ↑/↓ to navigate` was drawn |
| HTTP hook, sleep 300, then 200 empty body (default timeout) | `{"event":"end","mode":"sleep300","elapsed":300.0004551410675,"code":200}` | spinner `running PreToolUse hooks… 2/3 · 2m 2s` at ~2 min; no dialog during the wait; native dialog drawn after the 300 s response |
| HTTP hook `"timeout":5`, server sleeps 30 | server logged `start` only at poll time (response still pending) | spinner at t+3 s (`3s`); at t+16 s the native dialog was already shown, i.e. Claude abandoned the hook at ~5 s and failed open |
| command hook exit 1 (stderr `boom`) | script `crash` mode | native dialog |
| command hook invalid JSON on stdout | `badjson` mode | native dialog |
| **command hook exit 2** (stderr text) | `exit2` mode | **tool denied**: `User declined to answer questions`; the agent then stopped retrying. No dialog |
| HTTP 500 | `code:500` logged | native dialog |
| HTTP 200, empty body | `code:200` | native dialog |
| HTTP 200 with allow + `updatedInput.answers` | `answer1` | dialog suppressed, `Which db? → SQLite` |

Not measured: connection-refused (docs say non-blocking), hook `timeout` above 600 s, behavior after the session is detached, other permission modes.

## Plan consequences

- Claude itself does not impose a ceiling below the measured 300 s; the real limit is operator-facing (terminal is not an answer path while blocked). ADR-005 now fixes `HOOK_BLOCK_MS_CEILING = 300000` (the longest measured block) and keeps default `hook_block_ms = 0`. Any non-zero value must also set the injected hook's `timeout` strictly larger than `hook_block_ms` (the hook's own empty-exit cap fires first) and below 600.
- The injected `ssq-hooks` must fail with exit 0 or 1, never 2: exit 2 denies the question tool call.
- Blocking mode is not dropped; Task 3.4c's bounded wait is feasible.
