# Slack Phase 2 — Scoping Public Reachability

Enabling `cfg.Slack.ApprovalEnabled` registers `POST /api/hooks/slack-interactive`
(`server/server.go`, `server/services/slack_interactive_handler.go`) so a click on an
outbound Slack message's Approve/Deny button resolves the pending approval. Slack calls
this endpoint from Slack's own servers, not from your LAN — the stapler-squad instance
must be reachable from the public internet at that path for the buttons to work at all.

## Why this is different from every other `/api/hooks/*` route

Every other hook receiver (`/api/hooks/permission-request`, `/api/hooks/stop`,
`/api/hooks/pre-tool-use`, etc.) is only ever called by Claude Code running on the same
machine — the existing trust boundary is "localhost only," the same one
`validateLocalhostOrigin`/`validateLocalhostAddr` (`server/services/notification_service.go`)
establish for other local-only surfaces. `/api/hooks/slack-interactive` cannot rely on that:
it is a new, genuinely internet-facing HTTP endpoint. Signature verification
(`verifySlackSignature`, HMAC-SHA256 over the raw body with your Slack app's signing
secret, `hmac.Equal` comparison, 5-minute replay window) is necessary, but exposing more of
the app than this one path to get there defeats the point of having it.

## Do not tunnel the whole port

The default quick-start for both ngrok and most reverse-proxy setups tunnels or proxies an
entire port. Applied naively to stapler-squad (`localhost:8543`), that exposes every other
`/api/hooks/*` receiver, the dashboard, and the ConnectRPC session API to the internet — none
of which are signature-verified or otherwise designed to be internet-facing.

**Wrong** — tunnels everything on `:8543`:
```bash
ngrok http 8543
```

**Right** — scope the tunnel to exactly one path.

### ngrok: path-restricted via a local reverse proxy

ngrok itself has no built-in per-path filter, so front it with a minimal local reverse proxy
that only forwards the one path, and point ngrok at *that* proxy instead of at stapler-squad
directly. A one-line example using `nginx` (any reverse proxy that can match a path works
the same way):

```nginx
# /etc/nginx/conf.d/slack-interactive-only.conf — listens on 8999, forwards only this path
server {
    listen 127.0.0.1:8999;

    location = /api/hooks/slack-interactive {
        proxy_pass http://127.0.0.1:8543;
    }

    location / {
        return 404;
    }
}
```

```bash
ngrok http 8999
```

Register `https://<your-ngrok-subdomain>.ngrok-free.app/api/hooks/slack-interactive` as the
Interactivity Request URL in your Slack app's configuration.

### Reverse proxy (e.g. an existing Caddy/nginx box with a real domain)

Same principle, applied directly instead of through a tunnel — an explicit `location`/route
block for the one path, with everything else left unreachable from that public vhost:

```nginx
server {
    listen 443 ssl;
    server_name slack-hooks.example.com;

    location = /api/hooks/slack-interactive {
        proxy_pass http://127.0.0.1:8543;
    }

    location / {
        return 404;
    }
}
```

## Checklist

- [ ] The public hostname/tunnel forwards **only** `/api/hooks/slack-interactive` — verify
      by requesting any other path (e.g. `/` or `/api/hooks/permission-request`) through the
      public URL and confirming it 404s or is refused, not served.
- [ ] `cfg.Slack.SigningSecretEncrypted` (or the `SLACK_SIGNING_SECRET` env override) is set
      to your Slack app's actual signing secret — an empty secret makes the handler reject
      every request (fails closed), so buttons will silently not work if this is skipped.
- [ ] `cfg.Slack.ApprovalEnabled` is `true` and the service has been restarted (route
      registration is boot-time only — flipping the flag alone does not register the route
      on a running instance).
- [ ] The Interactivity Request URL configured in your Slack app points at the public
      URL's `/api/hooks/slack-interactive` path, not the bare host.
