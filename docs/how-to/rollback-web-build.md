# Rolling back the web build

Next.js was removed in PR 9, so there is **no rollback to Next, only a forward fix** or a revert of the commit that broke the Bun build. The git tag `web-dual-build-last` marks the last commit that had both builds, but restoring it discards every commit merged since (new pages without `metadata`, deleted `layout.tsx` files), so treat it as an emergency measure in a scratch worktree, never as a revert.

Every step that restarts the deployed service depends on one precondition: **the deployed unit must pass `--tmux-keep-server`**. Without it, restarting destroys every live tmux session, including any Claude Code session the restart runs from. See `docs/explanation/tmux-keep-server-on-restart.md` and the `CLAUDE.md` note.

## Ship a fixed or reverted build

### 1. Confirm `--tmux-keep-server`

```bash
# Linux
systemctl --user cat stapler-squad | grep -- --tmux-keep-server

# macOS
grep -A1 -- '--tmux-keep-server' ~/Library/LaunchAgents/*stapler-squad*.plist
```

Either must print the flag. If it is missing, stop: fix `scripts/install-service.sh`'s generator (see the tmux-keep-server doc's "How to re-verify" section) and redeploy before touching anything else, or accept session loss explicitly with `ACCEPT_SESSION_LOSS=1` in step 3.

### 2. Land the fix or the revert

`git revert` the offending commit, or forward-fix it, and merge to the branch you deploy from.

### 3. `make install-service` in a planned window

```bash
make install-service
```

Never run this to try out a change; use a manual instance (`CLAUDE.md`, "Manual/interactive testing"). It restarts the live `:8543` service.

`make install-service` depends on `install-service-preflight`, a Makefile guard that fails before building anything if the deployed unit lacks `--tmux-keep-server` and `ACCEPT_SESSION_LOSS=1` is not set. The guard is built from Make `$(error ...)`/`$(shell ...)` expansions so it also fires under `make -n`; `sh scripts/install-service-preflight.test.sh` exercises it with fake unit files.

### 4. Post-checks

- No `Unexpected token <` in the browser console (the classic wrong-bundler-output symptom).
- Login static assets return 200.
- `curl -s -o /dev/null -w '%{http_code}\n' http://localhost:8543/` prints `200`.
- `tmux list-sessions` before and after shows the same session names with unchanged `created` timestamps.

## Minification

The deployed build is unminified (`WEB_MINIFY ?= 0` in the `Makefile`). If minification is ever switched on and has to be reverted, restore that Makefile line and the matching `verify-output` assertion (`stats.minify === false`, `stats.minifyExplicit === true`), then restart as above. CI enforces byte budgets on a separately built minified artifact, so budgets are checked against minified bytes while the deployed bytes are unminified; that is expected.
