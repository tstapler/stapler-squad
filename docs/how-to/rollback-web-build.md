# Rolling Back the Web Build (Next/Bun Cutover)

Story 8.5 (`project_plans/web-app-bun-build/implementation/plan.md`) runbook
for reversing the Bun web-build cutover (PR 8) or its minification follow-up
(PR 8b). Read this in full before running any step in a real rollback — the
window where a rollback is even possible shrinks as PR 9 approaches (see
"After PR 9" below).

**Every step that restarts the deployed service depends on one precondition:
the deployed unit must pass `--tmux-keep-server`.** Without it, restarting
the service during a rollback destroys every live tmux session — including
any Claude Code session the restart runs from. See
`docs/explanation/tmux-keep-server-on-restart.md` and the CLAUDE.md WARNING.
`make install-service` now enforces this itself (a hard preflight, not just
this document — see "Enforcement" below), but confirm it manually first so
a failed preflight isn't a surprise mid-rollback.

## Rollback ladder (PR 8 cutover)

Run these in order.

### 1. Confirm `--tmux-keep-server`

```bash
# Linux
systemctl --user cat stapler-squad | grep -- --tmux-keep-server

# macOS
grep -A1 -- '--tmux-keep-server' ~/Library/LaunchAgents/*stapler-squad*.plist
```

Either must print the flag. Real output against this deployment, 2026-10-22
(read-only; the service was never restarted or touched to produce this):

```
$ systemctl --user cat stapler-squad | grep -- --tmux-keep-server
ExecStart=/home/tstapler/Programming/stapler-squad/stapler-squad --remote-access --tmux-keep-server --profile --profile-port 6060
```

Present. See `project_plans/web-app-bun-build/research/cutover-gate-results.md`
for the recorded Story 8.5 precondition check. If it's missing when you run
this for real, stop here: fix `scripts/install-service.sh`'s generator (see
the tmux-keep-server doc's "How to re-verify" section) and redeploy before
touching anything below, or explicitly accept session loss by setting
`ACCEPT_SESSION_LOSS=1` on the `make install-service` invocation in step 3.

### 2. Flip `WEB_BUILD` back to `next`, or revert PR 8

Two equivalent options, in increasing order of invasiveness:

- **Flip the default** (valid only until PR 9 merges — see "After PR 9"
  below): edit the `Makefile`'s `WEB_BUILD ?= bun` line back to
  `WEB_BUILD ?= next`, commit, and merge/deploy that change.
- **Revert PR 8 outright**: `git revert` the cutover PR's merge commit. Use
  this once PR 8's diff has accumulated commits on top of it that the simple
  default flip can't cleanly undo (e.g. after PR 8b — see the PR 8b section
  below).

### 3. `make install-service` in a planned window

```bash
make install-service
```

Run this only in a planned window, never to try out a change (use a
ManualInstance for that — see `CLAUDE.md`'s "Manual/interactive testing"
section). This restarts the live `:8543` service.

**Enforcement, not just documentation (Task 8.5d).** `make install-service`
now depends on `install-service-preflight`, a Makefile guard that fails
*before* building anything if the deployed unit lacks `--tmux-keep-server`
and `ACCEPT_SESSION_LOSS=1` is not set:

```bash
$ make -n install-service-preflight INSTALL_SERVICE_LINUX_UNIT=<fake-unit-without-flag> UNAME_S=Linux
Makefile:506: *** install-service preflight: the deployed stapler-squad unit does not pass --tmux-keep-server. Restarting it now would destroy every live tmux session, including any session this command runs from -- see docs/explanation/tmux-keep-server-on-restart.md. Re-deploy with the flag first, or re-run with ACCEPT_SESSION_LOSS=1 to explicitly accept the loss.  Stop.
```

With the flag present (the real deployed unit, verified above), or with
`ACCEPT_SESSION_LOSS=1` explicitly set, the preflight passes silently and
`install-service` proceeds normally. The guard is implemented as Make
`$(error ...)`/`$(shell ...)` expansions rather than a shell conditional
inside the recipe, specifically so it fires even under `make -n` (a Make
dry-run still expands recipe text, including `$(error)`, before printing
it) — this is what lets `scripts/install-service-preflight.test.sh` exercise
the real guard logic with fake unit files instead of the live one. Run it
with `sh scripts/install-service-preflight.test.sh`.

This replaces "or record the decision to accept loss" as the *only* escape
hatch that existed before this story: the decision must now be an explicit,
typed `ACCEPT_SESSION_LOSS=1` on the command line, not an unenforced note.

### 4. No aggregate gate to edit

The `web-build-gate` aggregate job and its `WEB_BUILD_GATE_MEMBERS` variable
were removed (`main` has no branch protection, so nothing required it). A
rollback needs no CI-gate change; if branch protection is added later, require
the specific jobs (for example `e2e (Bun build)`) directly.

### 5. Post-checks

After the restart:

- Watch the browser console for `Unexpected token <` (a classic
  wrong-bundler-output symptom).
- Confirm login static assets 200 (not 404s from a stale asset manifest).
- Confirm the HTTP server itself answers (`curl -s -o /dev/null -w '%{http_code}\n' http://localhost:8543/` should print `200`).
- Confirm tmux sessions survived: `tmux list-sessions` before and after
  should show the same session names with unchanged `created` timestamps
  (see the ManualInstance rehearsal below for what this looks like in
  practice).

## PR 8b rollback (minification revert)

PR 8b (Story 8.6) switches minification on after the first Soak window. Its
revert set is four pieces — revert all of them together, not piecemeal:

1. **`Makefile`**: restore the `WEB_MINIFY ?= 0` line (Task 8.6a removed it).
2. **CI default**: restore the CI job default that Task 8.6a changed (the
   same PR as #1, so reverting the PR covers both).
3. **`verify-output` assertion**: restore its explicit-flag-path assertion
   (`stats.minify === false`, `stats.minifyExplicit === true`) — the state
   the restored Makefile line produces.
4. **Service restart**: `make install-service` in a planned window, subject
   to the same `--tmux-keep-server` preflight as step 3 above.

After this revert, CI continues enforcing byte budgets on its own,
separately-built minified artifact — so budgets are checked against
minified bytes while the *deployed* bytes are unminified. This is expected,
not a gap: say so explicitly if anyone asks why a budget check and the
running deployment disagree on byte size.

## After PR 9 merges: no rollback, only a forward fix

Once PR 9 merges (the Next-removal PR), there is **no rollback path back to
Next** — only a forward fix of whatever broke. The tag `web-dual-build-last`
captures the last commit where both builds existed side by side, but
restoring from it is an emergency measure, not a rollback: doing so discards
every commit merged after PR 9 (new pages that never got Next's `metadata`
export, `layout.tsx` files deleted in the Next-removal cleanup that would
conflict on restore, etc.). The rollback window therefore effectively
expires when PR 9 merges; Story 8.4's Soak exists specifically to make PR 9
safe to merge in the first place.

## ManualInstance rehearsal (Task 8.5e)

Rehearsed once, 2026-10-22, on a private tmux socket — never against the
live `:8543` deployment. Steps and real timings:

1. **Build with `WEB_BUILD=bun`** (cold — proto-gen, ent-gen, and the Go
   build all ran): `env -u npm_config_user_agent WEB_BUILD=bun make build`.
   Wall time: **38.6 s** (the Bun web-build step itself:
   `web-build: ok (unminified, 1324 ms) -> out`; the rest was proto/ent/Go).
2. **Run the ManualInstance**: copied the binary to
   `~/.stapler-squad/manual-builds/manual-story85/stapler-squad`, then:
   ```bash
   mkdir -p ~/.stapler-squad/instances/story85-manual
   PORT=62871 STAPLER_SQUAD_INSTANCE=story85-manual \
     STAPLER_SQUAD_TMUX_SOCKET=ssq-manual-story85 \
     ~/.stapler-squad/manual-builds/manual-story85/stapler-squad --tmux-keep-server &
   ```
   Confirmed up: `curl -s -o /dev/null -w '%{http_code}\n' http://localhost:62871/` -> `200`.
3. **Stand in for a live session**: `tmux -L ssq-manual-story85 new-session -d -s story85-rehearsal-session`.
   Session list before the flip:
   ```
   staplersquad_keepalive: 1 windows (created Thu Oct 22 04:35:24 2026)
   story85-rehearsal-session: 1 windows (created Thu Oct 22 04:35:46 2026)
   ```
4. **Flip to `WEB_BUILD=next` and rebuild** (incremental — proto/ent already
   generated, so this measured mostly the Next.js production build itself):
   `env -u npm_config_user_agent WEB_BUILD=next make build`. Wall time:
   **89.9 s**.
5. **Restart the ManualInstance**: copied the new binary over, `kill`ed the
   running app process (not the tmux server), confirmed the process was
   gone, then relaunched the identical command from step 2 against the same
   `STAPLER_SQUAD_TMUX_SOCKET`. Came back up: `curl` -> `200` again.
6. **Confirm sessions survived**: session list after the restart —
   ```
   staplersquad_keepalive: 1 windows (created Thu Oct 22 04:35:24 2026)
   story85-rehearsal-session: 1 windows (created Thu Oct 22 04:35:46 2026)
   ```
   Identical names and **unchanged `created` timestamps** versus step 3 —
   the sessions were never torn down and recreated, which is exactly what
   `--tmux-keep-server` guarantees across an app-process restart.
7. **Teardown**: killed the ManualInstance process,
   `tmux -L ssq-manual-story85 kill-server`, removed the manual build and
   instance-state directories. Port `62871` and the `ssq-manual-story85`
   tmux socket were confirmed free before starting and after cleanup.

These build timings are from one rehearsal on this machine under ordinary
load, not a benchmark — they're recorded here to show the flip-and-rebuild
step is on the order of a minute or two, not to set an SLA. Story 8.4's Soak
runbook is the place for measured production-equivalent build-time numbers.
