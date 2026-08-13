# Pitfalls Research — BUG-006 doc entry (env-var-inheritance gap)

## Scope of this research

This item's only remaining code-adjacent work is a documentation addition:
a new `BUG-006` entry in `docs/tasks/completed/system-service-autostart.md`
describing the env-inheritance gap left behind by `baca1c7c` (removal of
`~/.zshrc` sourcing from the generated launchd plist / systemd unit). No
code changes are in scope. This research exists to make sure the doc entry
is accurate, doesn't overstate/understate risk, doesn't get read as
"resolved," and doesn't accidentally recommend the anti-pattern this
backlog item exists to eliminate.

## 1. Existing doc format/conventions (BUG-001..BUG-005, lines 566-648)

Confirmed structure, `docs/tasks/completed/system-service-autostart.md`:

```
### BUG-00N: <Title> [SEVERITY: High|Medium|Low]

**Description:** <what goes wrong, mechanism, user-visible symptom>

**Mitigation:**
- <bullet> (mitigations that ARE already implemented as part of this story)
- <bullet>

**Files Affected:**
- `path/to/file`

**Prevention:** <one sentence: what test/check would catch a regression>

---
```

Important nuance: every existing BUG-00N's "Mitigation" section describes
mitigations that were **already built** as part of the `system-service-autostart`
story (this file lives under `docs/tasks/completed/`, i.e. the story is done
and these bugs were pre-empted, not just documented). **BUG-006 breaks this
pattern** — it must NOT use "Mitigation:" to describe already-done work,
because nothing has been done. The new entry needs an explicit status
signal so it can't be read as "already handled like BUG-001..005 were."
Recommend a `**Status:** Deferred — not implemented this session` line
immediately under the description, and reframe the mitigation-shaped
section as "**Possible Future Mitigations (not implemented):**" so the
bullet list isn't mistaken for shipped work. This directly serves
acceptance criterion 5's requirement that the gap be "explicitly documented
... not silently declared resolved."

Severity scale observed: High (BUG-001, breaks session creation with no
visible error), Medium (BUG-002 stale path, BUG-003 launchctl deprecation),
Low (BUG-004 systemd unavailable, BUG-005 port conflict). BUG-006 should
land at **Medium**: real and silent-ish failure mode, but conditionally
mitigated (see §2) rather than universally breaking, so it doesn't meet
BUG-001's "High" bar of unconditionally breaking core functionality.

## 2. Real-world risk / existing fallbacks (config/config.go, github/http_client.go, server/services/session_service.go)

Checked how `ANTHROPIC_API_KEY` and `GITHUB_TOKEN` are actually resolved in
this codebase — this materially changes how severe the doc entry should
sound:

- **`GITHUB_TOKEN` already has a non-shell-rc fallback.**
  `github/http_client.go:31-58` (`getGHToken`): precedence is
  `GITHUB_TOKEN` env → `GH_TOKEN` env → `gh auth token` (shelled out,
  cached 1h via singleflight). `gh auth token` reads gh's own credential
  store (`~/.config/gh/hosts.yml`), which is populated by `gh auth login`
  and is **independent of shell rc files** — it was never reachable via
  `.zshrc` sourcing in the first place unless the user explicitly
  `export`ed a PAT there. So for most users (anyone who ran `gh auth
  login` rather than exporting a raw PAT in `.zshrc`), this specific token
  is **not actually broken** by `baca1c7c`. This is worth stating in the
  doc so BUG-006 isn't overstated for the GitHub half.

- **`ANTHROPIC_API_KEY` has a fallback for the app's own internal AI
  feature, but NOT for spawned agent sessions.**
  `server/services/session_service.go:206-219`: the service's own
  "AI rule generation" feature (`NewBestAvailableAIClient`) falls back
  from `ANTHROPIC_API_KEY` env → `claude` CLI → `gemini` CLI → `opencode`
  CLI, and logs a clear message (`"AI rule generation unavailable: set
  ANTHROPIC_API_KEY or install claude/gemini/opencode CLI"`) rather than
  crashing — this internal feature degrades gracefully.
  **However**, this fallback chain is specific to that one internal
  feature. The actual concern in the backlog item — the `claude`/`aider`
  processes spawned *inside the tmux sessions the service creates for the
  user* — inherit environment from the tmux session, which inherits from
  the service process's environment (now the minimal
  `HOME`/`PATH`-only env baked into the plist/unit by `baca1c7c`, not the
  user's full interactive shell env). There is no equivalent fallback
  chain for that path; `config/config.go:490` only reads
  `ANTHROPIC_API_KEY` from the service process's own env at config-load
  time, once, into `cfg.AnthropicAPIKey`.

- **De-risking factor worth noting**: the `claude` CLI itself commonly
  authenticates via a persisted OAuth session/credential file (from
  `claude login`, e.g. Pro/Max subscription auth) rather than requiring
  `ANTHROPIC_API_KEY` in the environment at all. For users on that auth
  path, losing shell-rc-sourced env vars is a **non-issue**. The risk is
  concentrated on users who authenticate via a raw API key exported only
  in `.zshrc`/`.zprofile` (e.g. console/API billing rather than a
  subscription) — for them, spawned `claude` sessions inside the service's
  tmux panes will silently fall back to a login/auth prompt (or fail)
  inside a pane the user may not be watching, which is a UX failure mode
  structurally identical to BUG-001 (session appears to start, but the
  agent inside is non-functional with no visible error in the web UI).

- No 1Password or other secret-store integration exists in this codebase
  for `stapler-squad` itself (grep of `config/`, `session/`, `server/`
  found none) — the dotfiles repo's 1Password role is unrelated to this
  application. So there is currently no automatic non-shell-rc secret
  source for the app; this is a real, currently-unmitigated gap for the
  API-key-auth subset of users.

**Net for the doc**: describe this as conditionally-impactful (not
universal), name the two token types differently (GITHUB_TOKEN mostly
de-risked via `gh auth token`; ANTHROPIC_API_KEY genuinely gapped for
API-key-auth users), and avoid alarmist "will break for everyone" language
since it wouldn't be accurate.

## 3. The anti-pattern to avoid recommending

The whole point of `baca1c7c` was removing `~/.zshrc`/`~/.zprofile`
sourcing from the launchd `ProgramArguments` / systemd `ExecStart` because
interactive-shell rc files can hang, prompt, or emit `stty`/tty-ioctl
errors when run headless (no controlling terminal) by launchd/systemd.
**The doc entry's "future mitigation" ideas must not suggest re-adding any
form of shell-rc sourcing** (`zsh -c 'source ~/.zshrc && exec ...'`,
`bash -lc`, `. ~/.profile`, etc.) as a fix path — that would silently
reintroduce the exact bug this backlog item closed. Safe-to-suggest
future-mitigation directions (for the "not implemented" bullet list only,
clearly marked as unimplemented ideas, not a plan): a dedicated
non-interactive env file read at install time (e.g.
`~/.stapler-squad/env`, sourced via `EnvironmentFile=` on systemd, which is
POSIX `KEY=VALUE` parsing with **no shell execution** — safe — or via
explicit per-key `EnvironmentVariables` entries in the plist), or a
1Password CLI (`op`) integration at request time. Both avoid invoking an
interactive shell.

## 4. Secondary pitfall: baking secrets into the generated unit/plist file

If a future fix reads `ANTHROPIC_API_KEY`/`GITHUB_TOKEN` at
`install-service.sh` install time and bakes the literal value into the
generated `~/.config/systemd/user/stapler-squad.service` or
`~/Library/LaunchAgents/*.plist` (the same way `PATH` is currently baked
in per `baca1c7c`), that introduces two new problems worth flagging
alongside the deferred-gap entry so a future implementer doesn't walk into
them:

- **Staleness**, same shape as existing BUG-002 (stale binary path): if
  the user rotates the API key, the service keeps using the old value
  until `make install-service` is re-run — silent auth failures after a
  routine key rotation.
- **At-rest secret exposure**: generated unit/plist files are plain text
  world-readable by default (systemd user units under
  `~/.config/systemd/user/`, launchd plists under
  `~/Library/LaunchAgents/`) and are exactly the kind of file people
  accidentally commit, sync via dotfile managers, or attach to bug
  reports/support requests — worse exposure surface than a `.zshrc` export
  the user already treats as sensitive-ish and controls directly. This
  should be flagged as a reason NOT to bake raw secret values into the
  generated service file if/when this gap is eventually fixed.

## 5. Wording risk: don't let the entry read as "already resolved"

The backlog acceptance criterion is explicit: the gap must be "explicitly
documented ... as deferred-pending-maintainer-decision, not silently
declared resolved." Concrete wording pitfalls to avoid when drafting
BUG-006:

- Don't reuse the exact "**Mitigation:**" heading with bullets describing
  fixes as if done — every reader's prior (BUG-001..005, all in a
  `docs/tasks/completed/` file) is that a "Mitigation" bullet list means
  "this was built." Use `**Status:** Deferred` + a differently-labeled,
  explicitly-hedged bullet list (see §1).
- Don't title the entry in a way that reads as past-tense/fixed (e.g. "Env
  vars now missing" reads worse than "Env vars no longer reachable via
  shell rc — no replacement path yet"). Prefer a title that names the gap
  as ongoing, e.g. "Service Process No Longer Inherits Shell-RC-Exported
  Secrets (ANTHROPIC_API_KEY, GITHUB_TOKEN)".
- Don't omit the causal link to `baca1c7c` — the entry should state
  plainly that this is a side effect of that commit's fix, not an
  unrelated pre-existing bug, so a reader understands why it's now present
  and why simply reverting isn't an acceptable answer either.
- Do state, per §2, that impact is conditional (subscription/OAuth
  `claude login` users and `gh auth login` users are largely unaffected;
  raw-API-key/PAT-in-shell-rc users are affected) rather than claiming a
  blanket "the service can't reach these secrets at all" — that's not
  quite true for `GITHUB_TOKEN` and overstates for `ANTHROPIC_API_KEY`
  users on OAuth auth.
