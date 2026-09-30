# GitHub Webhooks — Scoping Public Reachability

Enabling `webhook_triggers` registers `POST /webhooks/github`
(`server/server.go`, `server/services/github_webhook_handler.go`) so GitHub can push
delivery notifications to your stapler-squad instance instead of it having to poll.
GitHub calls this endpoint from GitHub's own servers, not from your LAN — the instance
must be reachable from the public internet at that path for deliveries to arrive.

This one route now serves two independent features that share the same path and secret:

- `push` deliveries (pre-existing) — matched against `github_push`-type `Workflow` rows
  to fire a new session.
- `check_run`/`workflow_run`/`pull_request_review`/`issue_comment` deliveries (the
  `pr_event_webhooks` feature) — reconcile a tracked `pr_pending` backlog item
  immediately instead of waiting for `PRStatusPoller`'s next ~60s tick.

GitHub multiplexes the event type via the `X-GitHub-Event` header, not the URL, so
**no per-event path-scoping is needed** — one tunnel/proxy rule for `/webhooks/github`
covers both features.

## Why this is different from every other `/api/hooks/*` route

Every `/api/hooks/*` receiver (`/api/hooks/permission-request`, `/api/hooks/stop`,
`/api/hooks/slack-interactive`, etc.) is documented elsewhere (see
[`slack-phase2-public-reachability.md`](slack-phase2-public-reachability.md) for the
Slack case). `/webhooks/github` is the same shape of problem: a genuinely
internet-facing endpoint that needs its own scoped tunnel, not a side effect of
exposing the whole app. Signature verification (`VerifyGitHubSignature`, HMAC-SHA256
over the raw body against the secret stored on the matching `github_push` `Workflow`
row) is necessary, but exposing more of the app than this one path to get there
defeats the point of having it.

## Do not tunnel the whole port

Applied naively to stapler-squad (`localhost:8543`), tunneling the whole port exposes
the dashboard, the ConnectRPC session API, and every other `/api/hooks/*` receiver to
the internet alongside `/webhooks/github` — none of which are meant to be
internet-facing.

**Wrong** — tunnels everything on `:8543`:
```bash
ngrok http 8543
```

**Right** — scope the tunnel to exactly one path, the same way Slack Phase 2 already
does for `/api/hooks/slack-interactive`. If you're running both features, add one more
`location` block to the *same* nginx config already serving the Slack path — one tunnel
process, one nginx instance, two (or more) scoped paths — rather than standing up a
second proxy.

### ngrok: path-restricted via a local reverse proxy

```nginx
# /etc/nginx/conf.d/webhooks-only.conf — listens on 8999, forwards only these paths
server {
    listen 127.0.0.1:8999;

    location = /webhooks/github {
        proxy_pass http://127.0.0.1:8543;
    }

    # Add alongside an existing Slack Phase 2 block in the same file, e.g.:
    # location = /api/hooks/slack-interactive {
    #     proxy_pass http://127.0.0.1:8543;
    # }

    location / {
        return 404;
    }
}
```

```bash
ngrok http 8999
```

Register `https://<your-ngrok-subdomain>.ngrok-free.app/webhooks/github` as the Webhook
URL in your GitHub repo's Settings → Webhooks configuration. For local development only,
[smee.io](https://smee.io) is a lighter-weight convenience for relaying GitHub webhook
deliveries to a local port — it is not the production guidance above, which still
applies once you move off a dev proxy.

### Reverse proxy (e.g. an existing Caddy/nginx box with a real domain)

```nginx
server {
    listen 443 ssl;
    server_name webhooks.example.com;

    location = /webhooks/github {
        proxy_pass http://127.0.0.1:8543;
    }

    location / {
        return 404;
    }
}
```

## Checklist

- [ ] The public hostname/tunnel forwards **only** `/webhooks/github` — verify by
      requesting any other path (e.g. `/` or `/api/hooks/permission-request`) through
      the public URL and confirming it 404s or is refused, not served.
- [ ] A `github_push`-type `Workflow` row exists for the repo you want PR-fix reactions
      on, with its webhook secret set. This is the **only** secret-storage source the
      signature-verification loop checks — it's required for `push` deliveries already,
      and `pr_event_webhooks` reuses the exact same loop, so a repo with no
      `github_push` `Workflow` row will have every PR-fix delivery 401 (fail closed)
      with no stored secret to verify against, even if `pr_event_webhooks` is enabled.
- [ ] `webhook_triggers` is `true` and the service has been restarted (route
      registration is boot-time only — flipping the flag alone does not register the
      route on a running instance).
- [ ] `pr_event_webhooks` is set to your intended state. It is independently
      toggleable from `webhook_triggers`, but **has no effect unless `webhook_triggers`
      is also enabled** — enabling `pr_event_webhooks` alone leaves `/webhooks/github`
      unregistered, so every PR-fix delivery silently 404s. Enabling `pr_event_webhooks`
      does **not** require a restart on its own (it's checked per-request, not at
      route-registration time) — only `webhook_triggers` does.
- [ ] The repo's webhook configuration on GitHub (Settings → Webhooks) has the
      **Content type** set to `application/json`, a secret matching the `github_push`
      Workflow row's, and, if you want `pr_event_webhooks` reactions, the individual
      events subscribed: **Check runs**, **Workflow runs**, **Pull request reviews**,
      and **Issue comments** (in addition to **Pushes**, if you also use the push
      trigger).
- [ ] **Verify reachability, not just setup**: after completing the tunnel/proxy setup,
      trigger one real delivery of each event type you enabled and confirm the
      corresponding one-time log line appears in the service log:
      `"[GitHubWebhookHandler] first verified <event-type> delivery received —
      /webhooks/github reachability confirmed"`. The easiest way to trigger one without
      waiting for real CI/review activity is GitHub's own **Recent Deliveries** panel
      (repo Settings → Webhooks → your webhook → Recent Deliveries) — pick a past
      delivery of the event type you want to verify and click **Redeliver**. Route
      registration succeeding and the feature flags being on are not proof a real
      GitHub delivery ever reached the instance — this step is what confirms it did.
