# Research: Build vs. Buy — deferred `ANTHROPIC_API_KEY`/`GITHUB_TOKEN` env-inheritance gap

**Scope note:** this research is *not* for work done in this session. This
session only adds a documentation entry (`BUG-006` in
`docs/tasks/completed/system-service-autostart.md`) recording the gap as
deferred. This file exists so that entry can gesture accurately at what a
future fix would look like, without this session designing or implementing
that fix. See `requirements.md` Non-goals.

## The gap

`baca1c7c` replaced the `.zshrc`-sourcing shell wrapper in
`scripts/install-service.sh` with direct `Program`/`ProgramArguments`
(macOS) and `ExecStart` (Linux) invocations, with `PATH`/`HOME` supplied
explicitly via `EnvironmentVariables` (plist) / `Environment=` (systemd
unit) — both generated once, at install time, by the installer script
(`install_linux()` at `scripts/install-service.sh:75-140`,
`install_macos()` at `scripts/install-service.sh:143-266`). That was the
correct fix for the non-deterministic-startup bug, but `.zshrc` sourcing
was also the *only* path by which shell-rc-exported secrets
(`ANTHROPIC_API_KEY`, `GITHUB_TOKEN`, etc.) reached the service process.
Removing it silently dropped secret inheritance too.

## 1. Existing OSS pattern: native env-file loading (not shell-sourcing)

Both service managers have a **native, non-shell mechanism** for exactly
this — loading a file of `KEY=VALUE` pairs into a process's environment —
which is the standard, idiomatic replacement for shell-sourcing secrets
into a unit/plist. Neither requires a custom parser.

**systemd — `EnvironmentFile=`** (directive on the `[Service]` section):
```ini
EnvironmentFile=-%h/.stapler-squad/env
```
- Reads `KEY=VALUE` lines (dotenv-like: `#`-comments, blank lines ignored,
  optional quoting) directly into the unit's environment, merged with any
  `Environment=` lines already present (`Environment=PATH=...` today).
- Read fresh **at service start**, not baked into the unit file — a maintainer
  could add this directive once, and future secret rotation would need only a
  service restart (`systemctl --user restart stapler-squad`), not a
  `make install-service` re-run.
- Leading `-` makes a missing file non-fatal (matches this codebase's
  existing tolerance for missing optional config).
- This is a widely-used, first-class systemd feature (`man systemd.exec`),
  not a bespoke pattern — e.g. Docker, Gitea, and many other self-hosted
  services ship an `EnvironmentFile=` pointing at an `/etc/default/*` or
  `~/.config/*/env` file for exactly this purpose.

**launchd — `EnvironmentVariables` dict, with no file-include equivalent:**
- launchd's plist format has **no directive analogous to `EnvironmentFile=`**.
  There is no `include`, no path reference — `EnvironmentVariables` must be a
  literal `<dict>` of `<key>/<string>` pairs baked directly into the plist
  XML.
- Consequently, on macOS the *only* place a generation-time env-file load can
  happen is inside the installer script itself, exactly where `PATH` is
  already resolved and inlined today (`scripts/install-service.sh:159-161`,
  `197-206`). The script would need to read `~/.stapler-squad/env` and
  emit additional `<key>/<string>` pairs into the same `EnvironmentVariables`
  dict, at plist-generation time — not at service-start time. Any secret
  rotation would require re-running `make install-service` to regenerate
  the plist, unlike the systemd path.
- This asymmetry (systemd re-reads a file at every restart; launchd must
  bake values into the generated plist) is a real platform difference the
  future fix would need to document, not an oversight to fix.

Both are **the same class of mechanism the codebase already uses for
`PATH`/`HOME`**: install-time generation of static environment values, no
shell interpreter invoked at service-start. Extending it to an env file is
consistent with the existing architecture, not a new pattern.

## 2. SaaS/managed alternative: does this repo already integrate 1Password?

**No.** Grepped the full repo (`.go`, `.sh`, `.md`) for `1password`,
`op read`, `op://`, `op run`:
- No Go code references 1Password or the `op` CLI at all (checked
  `executor/managed_process_test.go` and `session/pr_status_poller.go`,
  the only two non-doc files matching `1password` in a broad grep — both
  are unrelated string coincidences, not actual integration code).
- The only repo-wide mentions of 1Password are in **planning/ADR
  documents** (`project_plans/immortal-migration/plan.md`,
  `project_plans/*/research/*.md`, `docs/reviews/fuel-forge-archaeology.md`)
  — these discuss 1Password as a general secrets-management idea in other
  contexts, not as a shipped or in-progress integration for this service.
- The user's global tooling (`~/.claude/RTK.md`, Ansible `secrets` role in
  the separate dotfiles repo) does install/use 1Password CLI, but that's
  infrastructure for the *developer's* machine setup, unrelated to how
  `stapler-squad`'s own systemd/launchd service sources its runtime env.

So there is no existing 1Password integration to extend as "the intended
real fix." Introducing one would be new build/adopt work, not a rewire of
something already half-built. If it were adopted, the natural shape would
be `op run --env-file=~/.stapler-squad/env.tpl -- $bin_path ...` — but
that reintroduces a **wrapper process** at service-start time (similar
shape to the shell-wrapper anti-pattern this item just removed), and
`op run`/`op read` need an unlocked, authenticated 1Password session or a
configured **service account token** to run non-interactively — the latter
is itself a secret that would need to reach the headless service somehow,
which doesn't eliminate the bootstrapping problem, it moves it one level
up.

## 3. Reasons not to build a custom env-file loader

- **systemd already has one, natively.** `EnvironmentFile=` is a built-in
  directive — writing a custom parser/loader to inject `KEY=VALUE` pairs
  into `Environment=` lines at generation time would be strictly worse
  than just adding one line to the unit template and letting systemd do
  the reading, since the native directive also gets rotation-without-
  reinstall for free.
- **launchd has no include mechanism, so there's nothing to "build" beyond
  what the installer script already does for `PATH`.** The only lever
  available is generation-time inlining into `EnvironmentVariables` — which
  is not a custom loader, it's the same string-templating the script
  already performs. A "loader" would just be: read file, split on `=`,
  append `<key>/<string>` pairs. Trivial, but still means plist regen on
  every secret rotation — an inherent launchd limitation, not something a
  cleverer implementation can route around.
- **Secret handling in a world-readable-by-default file is a real risk**
  either way: plists in `~/Library/LaunchAgents` and files referenced by
  `EnvironmentFile=` are both subject to normal Unix file permissions —
  whichever design is chosen, the future fix needs to set the env file (and,
  for launchd, the plist itself) to `0600`, which isn't automatic today for
  the plist writer (`scripts/install-service.sh:163` uses default `cat >`
  permissions, currently fine only because it holds no secrets).

## 4. Recommendation for the future backlog item

The future fix should almost certainly be:

**A generation-time-loaded `~/.stapler-squad/env` dotenv file, read via
each platform's native mechanism** — `EnvironmentFile=` for systemd,
generation-time inlining into `EnvironmentVariables` for launchd (the
same pattern already used for `PATH`) — **not** a 1Password/SaaS
integration and **not** a custom parser/loader.

Rationale to briefly gesture at in the deferred-doc entry, without
over-committing to specifics a maintainer hasn't signed off on:
- It reuses a mechanism each service manager already provides natively
  (`EnvironmentFile=`) or a pattern the installer script already applies
  today (plist inlining for `PATH`/`HOME`) — no new dependency, no shell
  interpreter reintroduced.
- A 1Password-based fix is not "already half-built here" — it would be new
  integration work, and it trades one bootstrapping problem (secrets not
  reaching the service) for another (an unlocked/authenticated `op` session
  reaching a headless background service), so it's a heavier lift for the
  same outcome unless a maintainer specifically wants centralized secret
  rotation across machines.
- File permissions (`0600` on the env file, and on the plist once it can
  contain secrets) are a prerequisite either way and should be called out
  as an implementation requirement, not assumed.

This is a recommendation for future work, not a decision — the actual
backlog item should still get maintainer sign-off on file location/format
before implementation.
