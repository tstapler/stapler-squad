# Dispatch sessions over MCP and get a link to the UI

Let an agent on the same machine start sessions through the stapler-squad MCP server and hand back a clickable link to each one.

## Prerequisites

- stapler-squad running on the machine where the agent runs (default `http://localhost:8543`).
- The UI reachable from your browser at some URL (a port-forward, reverse proxy or workspace app URL).

## Steps

1. Register the MCP server with the agent. It speaks streamable HTTP at `/mcp`:

   ```json
   { "mcpServers": { "stapler-squad-dispatch": { "type": "http", "url": "http://localhost:8543/mcp" } } }
   ```

   Use a name other than `stapler-squad`: sessions that stapler-squad launches get a per-session entry with that name and a credential header.

2. Set `STAPLER_SQUAD_PUBLIC_URL` to the URL your browser uses for the UI. The durable place is a line in `~/.stapler-squad/service.env`, which `scripts/install-service.sh` folds into the unit on every install, so re-run `make install-service` afterwards:

   ```
   STAPLER_SQUAD_PUBLIC_URL=https://ssq.example.com
   ```

   To apply it without a reinstall on Linux, use a systemd drop-in at `~/.config/systemd/user/stapler-squad.service.d/public-url.conf` (the installer does not touch drop-ins):

   ```ini
   [Service]
   Environment=STAPLER_SQUAD_PUBLIC_URL=https://ssq.example.com
   ```

   Then `systemctl --user daemon-reload && systemctl --user restart stapler-squad`.

3. Call `create_session` (or `create_session_for_pr`). The result now includes `ui_url`, for example `https://ssq.example.com/?session=<stable id>`.

## Open the UI through a reverse proxy

A proxy on the same machine reaches `localhost:8543` with its own `Host` header, and the local listener's host guard answers `403 Forbidden` (log line `local listener request rejected ... reason=host_not_allowed`). List the proxy's hostname in `STAPLER_SQUAD_TRUSTED_PROXY_HOSTS` (comma-separated, set it like `STAPLER_SQUAD_PUBLIC_URL` above) and restart:

```
STAPLER_SQUAD_TRUSTED_PROXY_HOSTS=ssq.example.com
```

- Entries must be plain DNS names; schemes, ports, paths, wildcards, IP literals and `localhost` are rejected and logged.
- The listener behind the proxy is unauthenticated, so the proxy must authenticate callers itself. Do not list a hostname that anyone untrusted can reach.
- Requests that arrive with proxy headers are still refused for `Reply` (answering a session's question), which is the strictest write. Enable `require_local_auth` if you need it through the proxy.

## Behavior and limits

- `ui_url` is omitted when the variable is unset, is not an `http(s)` URL, has no hostname, or contains credentials (`user:pass@`).
- A path prefix in the variable is kept: `https://gw.example.com/ssq` gives `https://gw.example.com/ssq/?session=<id>`. Query and fragment are dropped.
- The link carries the session's stable ID, which the UI resolves first. The variable is read once at startup.
- `create_session` and `create_session_for_pr` return the link, including when the session is still being created (`still_creating`).
- By default the local listener (`localhost:8543`) is unauthenticated, so any process on that machine can call `create_session`. With `require_local_auth` enabled, callers must send the local API token (`<config dir>/auth/local-api-token`). Only the tools in `identityGatedTools` (`server/mcp/caller_identity_gate.go`) verify the calling session; the rest trust the claimed session UUID.
- Do not forward or proxy `localhost:8543` to other hosts. Reach `/mcp` remotely only through the authenticated `--remote-access` HTTPS listener.
- The link is only as private as the URL you configure. If it sits behind a login, only people who can sign in can open it.
