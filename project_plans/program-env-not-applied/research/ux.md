# UX Research: Program env not applied to spawned sessions

## Surface examined

`web-app/src/components/settings/ProgramsManager.tsx` — the "Program Configurations"
UI, per-program `Environment Variables` key/value editor (`envVars` state, lines
56-297, 440-473), submitting via `handleSubmit` to `client.upsertProgramConfig(...)`
(lines 220-256).

Two sibling surfaces with the identical pattern also exist and share the same gap:
`web-app/src/components/settings/AliasesManager.tsx` (per-alias env vars, ~line 598)
and `web-app/src/components/settings/GlobalDefaultsForm.tsx` (global-default env vars,
~line 256).

## 1. Does the UI give any feedback that registered env vars are actually in effect?

No. `handleSubmit` (`ProgramsManager.tsx:220-256`) only confirms the **write**:

```tsx
setSuccess(`Successfully ${isEditing ? "updated" : "created"} program "${formData.label.trim()}".`);
```

That message (and the `error`/`success` state it's paired with) reflects whether
`UpsertProgramConfig` persisted the config — it says nothing about whether a
subsequently spawned session's process environment actually contains those vars.
There is no session-detail view, terminal affordance, or session-creation
confirmation anywhere in `web-app/src` that surfaces a session's *resolved/effective*
environment (confirmed via repo-wide grep for `effective env`, `resolvedEnv`,
`environment variable` across `web-app/src` — the only hits are the three config-entry
forms themselves and an unrelated `GitHubPRsSection.tsx` string about token sourcing).

So the only way a user can confirm "my custom env var was applied" is to open the
spawned session's terminal and run `printenv`/`echo $VAR` by hand — exactly the path
that surfaced this bug. The UI's success toast is a **write acknowledgment**, not an
**effect acknowledgment**, and nothing in the product distinguishes the two for the
user. This is the real UX gap the bug lives in: the config form worked completely
correctly (persisted, round-tripped, editable) end-to-end, and a user had no signal
that the wiring past that point was broken.

## 2. Reasonable low-cost follow-up suggestion (out of scope for this bugfix)

Once `buildExtraEnv`/`resolveExtraEnvVars` (`session/instance_tmux.go`) actually
injects the configured vars via tmux `-e`, add a lightweight confirmation surface so
this class of "config saved but not applied" regression is visible to users without
a terminal probe:

- Minimal: on the session-detail/info panel (wherever session metadata — command,
  cwd, program — is already shown), add a collapsed "Environment" section listing the
  program's configured env var *keys* (not values, to avoid leaking secrets on
  screen) with a small "applied" indicator once the session has actually started.
- Higher-value but higher-cost: have the backend echo back the resolved env keys it
  actually passed to tmux (not just what's in config) as part of session-start
  telemetry/response, so the UI can show "N configured, N applied" and flag a
  mismatch instead of asserting success blindly.

This is a genuine gap, not just a nice-to-have — it's the exact feedback loop whose
absence let this bug ship silently. Recommend logging it as a follow-up backlog item
rather than folding it into the bugfix itself, since the requirements are explicit
that Program Configurations' fields/layout are not changing.

## 3. Scope note

No accessibility/WCAG review performed per task instructions — this is not a new UI
feature, and the existing form (labels, ARIA labels on remove buttons, etc.) is
unchanged by the backend fix. This file covers only the "did my config change take
effect" feedback-loop gap.
