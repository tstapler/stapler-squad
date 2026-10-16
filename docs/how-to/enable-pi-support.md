# Enabling pi Support

Stapler Squad can manage sessions running the [pi coding agent](https://github.com/earendil-works/pi-coding-agent) (`@earendil-works/pi-coding-agent`) alongside Claude Code: resume across restarts, a first-class program-picker entry, live status in the session list, and approval-rules enforcement parity with Claude Code's hook. Disabled by default — everything below is opt-in.

## Enable the feature flag

Turn on the `pi-support` flag from **Settings → Features**, or via the API:

```bash
curl -X POST http://localhost:8543/api/session.v1.SessionService/UpdateFeatureFlag \
  -H "Content-Type: application/json" \
  -d '{"name": "pi-support", "enabled": true}'
```

With the flag on, "pi" appears in the session-creation Program picker (Advanced Options), and pi sessions get resume-across-restart support and live status detection.

Disabling the flag again does **not** remove an already-installed approval extension (below) — it only hides the picker entry and stops new resume/status wiring. A settings warning names the residual effect and links back to the uninstall step.

## Install the approval extension

Approval-rules parity with Claude Code (the same `RulesService`-backed auto-approve/deny logic) requires installing a small TypeScript extension into pi's **global** extension directory — global, not per-project, so a fresh worktree is covered immediately instead of needing a manual per-directory trust prompt:

```bash
ssq-hooks install pi
```

This writes `~/.pi/agent/extensions/ssq-approval.ts` and copies the `ssq-hooks` binary to `~/.local/bin/ssq-hooks`. Safe to re-run — it's idempotent. Restart any running `pi` process for the extension to take effect.

To remove it later:

```bash
ssq-hooks install pi --uninstall
```

The extension intercepts every pi tool call and POSTs it to `/api/hooks/permission-request` — the same endpoint Claude Code's hook uses — so an existing rule that blocks a tool call for Claude Code also blocks it for pi. On a network error or any unexpected exception, it **fails closed** (denies the tool call) rather than letting it through unenforced.

## Reading the health badge

Because the extension and the enforcement it provides live outside the stapler-squad process (loaded by pi itself, at pi's discretion), a pi session card shows a health badge reporting whether the extension is confirmed loaded — never assumed:

| State | Badge | Meaning |
|---|---|---|
| Unknown | `◌` pi — status unknown | No load ping has been received yet. Shown on every fresh session until the first ping arrives or the grace window elapses. Never rendered as "loaded" before a real signal. |
| Loaded | 🛡️ pi — loaded, tool calls are enforced | The extension pinged `/api/hooks/pi-extension-loaded` within the grace window. Approval rules are being enforced for this session. |
| Failed | ⚠️ pi — not loaded, tool calls are unenforced | The grace window elapsed with no ping — most likely `ssq-hooks install pi` was never run, or pi's per-project trust gate skipped the extension. Tool calls for this session run **without** rule enforcement. Run `ssq-hooks install pi` (or check pi's trust settings) and restart the session. |

Treat a `Failed` badge as if approval rules aren't installed at all for that session — it's the enforcement fact, not a session-liveness indicator (a Failed session can otherwise be running normally).
