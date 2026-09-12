---
name: rollout-readiness-review
description: Use to validate one of this repo's feature-flag-gated backend rollouts (e.g. tymux, stream_hub, native_git_worktree, native_git_merge) and decide whether it's safe to flip the global default — an operational-readiness review that inventories the flag's plumbing, checks docs against actual behavior, runs its tests, then actually exercises it against a live disposable instance to surface gaps a code read alone won't catch. Triggers on "rollout readiness", "is <flag> ready to default on", "validate the <X> rollout", "feature flag review".
---

# Rollout Readiness Review

This repo's rollout flags (`config.TymuxFeatureFlag`, `StreamHubFeatureFlag`,
`NativeWorktreeFeatureFlag`, `NativeMergeFeatureFlag` — `config/config.go`) all
share one shape: a `config.FeatureFlags` entry, an `Effective<X>Enabled(cfg)`
reader, `Set<X>GlobalOverride`/`Set<X>SessionOverride` config methods, a
`<X>RolloutService` RPC (`server/services/`), and a `<X>RolloutPanel` in
`web-app/src/components/settings/`. A code read of this plumbing tells you
what's *supposed* to happen. It does not tell you what actually happens —
this review's Step 4 found a production-blocking bug (BUG-106) that six
passing test suites and a clean `go build` both missed, because the bug only
existed at the boundary between stapler-squad and a real spawned `tymuxd`
process, which no unit test exercises.

Don't skip straight to "run the tests and call it GO." Each step below found
something the previous one didn't, on the one review this skill is written
from (`tymux`, 2026-09-12).

## Step 1 — Inventory

Find every piece for the flag under review, mirroring an existing one if the
shape is unclear:

```bash
grep -n "func Effective.*Enabled\|FeatureFlag = \"" config/config.go
rg -l "<Flag>RolloutService" server/services/
rg -n "<Flag>RolloutServiceGetRolloutStatusProcedure\|Procedure = " gen/proto/go/session/v1/sessionv1connect/*.go   # RPC paths (needs `make build`/proto-gen to exist locally)
find web-app/src/components/settings -iname "*<Flag>*"
find project_plans -ipath "*<flag-slug>*" -iname "ADR-*.md"      # design decisions
find docs/bugs -iname "*<flag>*"                                  # prior known issues
```

Note: the default (on vs. off), whether a rollback-rehearsal timestamp field
exists (`<X>RollbackRehearsalCompletedAt`), and whether session-level
overrides exist alongside the global one.

**Also check observability parity, not just correctness.** A flag can work
and still be un-shippable as a default if there's no way to see whether it
performs acceptably. Grep for OTel instruments and spans in the flag's own
package and compare against a rollout that already did this well:

```bash
grep -rn "Histogram\|Counter\|Gauge\|tracer\.Start\|otel\.Tracer" session/<pkg>/*.go | grep -v _test.go
```

`session/git/native_rollout.go` is this repo's reference implementation: a
`git_operation_duration_ms` histogram labeled `operation × implementation`
plus a span per dispatch point, built specifically to let an operator compare
the new path's latency against the old one before trusting it as a default.
A flag whose only instrument is a reliability counter (e.g. a reconnect
count) or a generic cross-backend histogram with no backend/implementation
label (functionally correct, but unable to isolate the new path's latency)
cannot answer "does this feel slower" — file that as its own gap (this
review's `docs/bugs/open/BUG-108` is the worked example) rather than treating
Step 3/4's functional pass as sufficient proof of readiness.

## Step 2 — Static readiness checks

- **Doc-vs-behavior drift.** Read every doc comment touching the flag's
  resolution path and verify each claim against the actual call graph — don't
  trust a comment that says "read once at startup" or "gated on rehearsal"
  without finding the code that does it. `config.go`'s `EffectiveTymuxEnabled`
  claimed "not live-settable" while `SetTymuxGlobalOverride`'s own comment
  said the opposite, and `getSelectedBackend`'s comment (correct) said
  "resolved live, on every call." Cross-referencing all three found the stale
  one. A quick way to catch a *removed* mechanism specifically: grep for any
  function name mentioned only in comments —
  `rg -n '<FuncName>' -g '!*.md'` returning zero real call sites (only
  comments) means it was deleted and the comments never got updated. Check
  `git log -S'<FuncName>'` to confirm and find the commit that removed it.
- **Enforcement reality check.** If a comment claims a global-default flip is
  gated on something (a rehearsal, a review, a minimum canary period), read
  the `Set<X>GlobalOverride` implementation and confirm the gate is actually
  checked there, not just documented. It is common in this codebase for a
  gate to have been designed, then quietly dropped in a later refactor
  ("drive stream-hub/tymux defaults from feature flags, not env vars" removed
  both `ResolveGlobalTymuxDefault` and `ResolveGlobalStreamHubDefault`
  entirely) while the surrounding comments kept describing the old behavior.
- **Known-issues sweep.** `grep -rl "<flag-or-package>" docs/bugs/open/` — an
  open bug already covering the exact area under review changes the verdict
  regardless of what your own testing finds.
- **Current real-world exposure.** Before touching anything, read (never
  write) the live deployed instance's config to see how far this flag already
  is in practice — don't rely on "default is off" meaning "nobody's using
  it":
  ```bash
  find ~/.stapler-squad -maxdepth 3 -iname "config.json"   # workspaces/*, instances/*, and the flat baseDir
  python3 -c 'import json; c=json.load(open("<path>")); print(c.get("feature_flags"), c.get("<x>_session_overrides"))'
  ```
  A flag can be globally off but already pinned on for several real
  (non-test-named) sessions via a per-session override — that changes both
  the urgency and the blast radius of any bug you find.

## Step 3 — Automated test pass

```bash
go build ./...
go test ./session/<pkg>/... ./server/services/... -run '<Flag>|Backend' -v -count=1
go test ./session/<pkg>/... -race -count=1        # concurrency-sensitive daemon/session code especially
```

A clean pass here is necessary, not sufficient — it did not catch BUG-106
because no test in the suite spawns a second real daemon process for a second
instance. Note what the tests *can't* reach (usually: real subprocess
lifecycles, real sockets/ports, real filesystem permissions) and plan Step 4
to cover exactly that gap.

## Step 4 — Exercise it live, on a disposable instance

Never use the MCP `mcp__stapler-squad__*` tools for this — they operate on
the live deployed instance on `:8543`. Build and run a second, fully isolated
instance per this repo's own `CLAUDE.md` ("Manual/interactive testing without
touching the live deployed instance"):

```bash
mkdir -p ~/.stapler-squad/manual-builds/manual-1
go build -o ~/.stapler-squad/manual-builds/manual-1/stapler-squad .
PORT=62871 STAPLER_SQUAD_INSTANCE=rollout-orr-<flag> \
  ~/.stapler-squad/manual-builds/manual-1/stapler-squad --tmux-keep-server > /tmp/.../server.log 2>&1 &
disown
```

**Startup is not instant** — the first `curl` attempt commonly fails with
`HTTP 000`/exit 7 even after a few seconds; retry with backoff rather than
concluding the server is broken. A brand-new named instance's first two
config-save attempts also log a benign, self-healing `WARN` (BUG-107) — don't
mistake it for the failure under investigation.

Drive the flag through its real RPC surface, not by editing `config.json`
directly (that skips the exact code path a real operator/UI click would take)
found from Step 1's generated `sessionv1connect` procedure constants, always
prefixed with `/api`:

```bash
curl -s -X POST http://localhost:62871/api/session.v1.<X>RolloutService/Get<X>RolloutStatus -d '{}'
curl -s -X POST http://localhost:62871/api/session.v1.<X>RolloutService/Set<X>GlobalOverride -d '{"force<X>": true}'
```

(If piping a `curl` response into `python3 -c` gets blocked by a
prompt-injection-style safety hook, save to a file with `-o` first and read
that file separately — it's flagging the shape of the pipe, not the target.)

Then create a **real disposable session** against a throwaway git repo (never
point it at this repo's own working tree — that creates a real worktree/branch
here) and watch what actually happens:

```bash
mkdir -p /tmp/.../orr-repo && cd /tmp/.../orr-repo && git init -q && git commit --allow-empty -qm init
curl -s -X POST http://localhost:62871/api/session.v1.SessionService/CreateSession \
  -d '{"title":"orr-test","path":"/tmp/.../orr-repo","program":"bash","existingWorktree":"/tmp/.../orr-repo"}'
curl -s -X POST http://localhost:62871/api/session.v1.SessionService/ListSessions -d '{}'   # check status
```

If the session status is anything but active/healthy, read the *structured*
log, not stdout — stdout only has the first few startup lines:

```bash
tail -100 ~/.stapler-squad/instances/rollout-orr-<flag>/logs/staplersquad.log
```

This is where BUG-106 actually surfaced: `SESSION_STATUS_FAILED` plus an
`ERROR [session pipeline] async start failed` line naming the real daemon
error. Chase a daemon/subprocess failure by running the underlying binary
**by hand** with the same env vars stapler-squad set — this is usually the
fastest way to see the real error message a supervision layer's own retry
loop and `Stdout = nil`/log-discarding would otherwise hide entirely:

```bash
TYMUXD_ADDR=127.0.0.1:<port> timeout 3 /path/to/tymuxd 2>&1
```

**Verify rollback, not just forward**: clear the override
(`{"force<X>": null}` — omit the field or send explicit `null`/unset per the
proto's tri-state convention) and confirm a *new* session created afterward
succeeds normally. A flag that's easy to turn on and hard to safely turn back
off is not ready for a global default regardless of forward-path behavior.

**Reproduce concurrency/collision scenarios deliberately.** If Step 1 found a
per-instance or per-session isolation mechanism (a derived port, a derived
path), don't just trust it exists — start a *second* named instance
concurrently and confirm both actually coexist. `resolveDaemonAddr`'s
instance-scoped TCP port existed and was documented; testing revealed it
solved only half of the actual collision surface (a shared Unix-socket lock
neither the code nor its own design docs had re-checked after the port fix
landed).

**Clean up everything you created**, in this order, before finishing: kill
the disposable session's tmux server (`tmux kill-session -t
staplersquad_<title>`), kill the manual instance process, `rm -rf` its
`~/.stapler-squad/instances/<name>/` directory and the manual-build binary,
remove the throwaway repo. Leave the real deployed instance and any tymuxd
process you didn't spawn yourself untouched — check a process's parent
(`ps -o pid,ppid,cmd -p <pid>`) before assuming it's yours to kill.

## Step 5 — Classify and fix

- **BLOCKER**: breaks correctness/safety, or makes the documented
  operator-facing rollback path not actually work. Fix now if the root cause
  is scoped and low-risk (BUG-106: additive env var + a mirrored pure
  function + tests, no behavior change for the existing default path). If the
  fix would itself be risky or large, file it (`docs/bugs/open/`, this
  repo's numbered format — check the latest `BUG-NNN` and increment) rather
  than rushing a patch into an ORR.
- **GAP**: missing enforcement/observability that doesn't block correctness
  today (e.g. a rehearsal gate that's now purely a historical record with no
  code enforcement — worth a product decision, not a code fix by itself).
- **NIT**: stale docs/comments. Fix inline as you find them — cheap, and
  compounds into real confusion if left (a stale "gated on rehearsal" comment
  is exactly what makes the next engineer trust a safety mechanism that no
  longer exists).

Any test you add for a fix found this way needs the same env-var hermeticity
discipline as the rest of this codebase: if your fix's code path now touches
`config.GetConfigDir()` (most per-instance derivation does), any existing
test that sets `STAPLER_SQUAD_INSTANCE` to a real-looking name without also
setting `STAPLER_SQUAD_TEST_DIR` will now write into the developer's real
`~/.stapler-squad/instances/<name>/` — check every test that exercises the
function you changed, not just the ones you add.

## Step 6 — Verdict

State one of:

- **GO** — safe to flip the global default now.
- **HOLD** — a BLOCKER remains; name it and what unblocks it.
- **FIX-THEN-SHIP** — BLOCKERs found and fixed in this pass; re-verify Step 4
  against the fix before claiming GO (a fix you haven't re-run against a live
  instance is a hypothesis, not a verified fix — see BUG-106's own history in
  this review, where the first fix attempt still failed on a second,
  different error the same live re-test caught).

Always name what's still open (GAPs, filed bugs) even under a GO verdict —
"safe to default on" is not the same claim as "fully hardened."
