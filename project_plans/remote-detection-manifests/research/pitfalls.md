# Research: Known Pitfalls — Remote-Fetched Detection Manifests (issue #178 pattern)

Scope: risk analysis for the proposed remote-fetch-and-cache layer described in
`project_plans/remote-detection-manifests/requirements.md`, which would sit on top of the
already-shipped `detector-plugins` local TOML loader (merged 2026-08-02, commits `3c25e94f9`/
`005e75827`). This is **research only** — the sibling project's 90-day demand-validation
checkpoint (target ~2026-10-31, currently 4 days elapsed) is unresolved, not failed, and
`requirements.md` already states this document should not be read as a build recommendation.

## 1. Supply-chain / spoofing risk

The manifest format under discussion carries patterns for `needs_approval` and
`input_required` status categories (`session/detection/pattern_set.go`) — the exact signal
stapler-squad uses to decide whether to pause and require a human before letting an agent
proceed. A remote manifest is therefore not "config," it's **security-relevant control data**:
an attacker (or a compromised/expired-domain host, or a MITM'd plain-HTTP fetch) who can
influence the fetched content can rewrite or delete the `needs_approval`/`input_required`
patterns so stapler-squad never pauses — silently converting a human-in-the-loop system into
an unsupervised one. This is a strictly worse failure mode than a bad `detector-plugins` local
file (ADR-004's threat model: "resource consumption only, never privilege escalation" — that
conclusion was reached for *local, user-authored* files the user explicitly placed on their own
disk; it does not transfer to content fetched from a third party over a network).

**Standard mitigation shapes, in increasing cost order:**

| Mitigation | What it buys | Cost |
|---|---|---|
| TLS-only (HTTPS enforced, reject `http://` even via config override) | Blocks passive MITM/on-path tampering | Near zero — one `if !strings.HasPrefix(url, "https://")` check |
| Size/shape limits (max bytes, schema validation via the existing `detector-plugins` validator) | Bounds blast radius of a malformed or oversized payload; catches structural corruption | Near zero — the ADR-004 caps (`maxPluginFileSize`, `maxPatternsPerPlugin`, `maxRegexLength`) already exist and are the correct reuse target | 
| Pinned commit SHA (fetch `raw.githubusercontent.com/.../<sha>/manifest.toml`, not `.../main/manifest.toml`) | Removes the floating-branch trust problem — no single `git push` (or compromised maintainer account) can change what a pinned client fetches without the client also updating the pin | Requires a version-to-SHA mapping mechanism, and someone (Tyler) manually bumping the pin — real but small ongoing cost |
| Content hash verification (e.g. manifest ships alongside a `.sha256`, or the SHA is embedded in a signed index file) | Detects tampering/corruption between publish and fetch even over a compromised CDN edge | Needs a second trusted channel to carry the hash — either baked into the client binary (defeats "update without a release") or itself fetched, which just moves the trust problem one hop |
| Cryptographic signing (GPG/minisign/sigstore over the manifest, public key baked into the client binary) | The only mitigation that survives a fully compromised transport/host — verifies authenticity independent of TLS | Real engineering cost: key generation, key rotation story, signing step in the publish workflow, verification code, and now a *key compromise* is also a threat to plan for |

**What's proportionate here:** for a personal-scale tool with one maintainer and no untrusted
publishers, TLS-only + reuse of the existing `detector-plugins` structural validator (ADR-003)
is the honest floor — anything less (plain HTTP, no schema check) is not acceptable per
`requirements.md`'s own Security NFR. Pinned-commit-SHA is a reasonable middle tier if the
source is `raw.githubusercontent.com` on this same repo (as `Open Questions #2` suggests is the
likely choice) — it costs almost nothing extra since the fetch code needs *some* version
identifier already, and it directly closes the "floating branch, anyone with push access changes
detection behavior for every running instance on next fetch" gap. **Full signing (GPG/sigstore)
is overkill for this project** — it's the shape of mitigation that makes sense for a plugin
*marketplace* with many untrusted publishers (exactly what `requirements.md`'s own "Rabbit
Holes" section already rules out: "Designing a full plugin marketplace / signing infrastructure
— far beyond what a personal-scale tool with no telemetry needs"). The asymmetry worth naming
explicitly: the pinned-SHA approach and the "just edit the file by hand" local-plugin path
(already shipped) have the *same* actual trust root — Tyler's own GitHub push access — so signing
would be protecting against a threat (a compromised publish pipeline) that doesn't materially
differ from the threat already accepted for every other part of this repo's release process.

## 2. Availability / blocking risk (startup-time fetch)

**Root-cause shape of the footgun:** a network call with no timeout, or a timeout applied only
to the HTTP round-trip and not to DNS resolution/TCP connect, can hang far longer than any
reasonable user will wait — DNS resolution against a broken resolver, or a TCP SYN into a
black-holed corporate proxy, can hang for the OS-level connect timeout (order of minutes), not
whatever `http.Client{Timeout: ...}` was set to, if the timeout wasn't wired through a
`context.WithTimeout` that also bounds connection establishment.

**What this codebase already does for comparable network-touching startup/background paths**
(confirmed via `Grep`, not assumed): every existing outbound HTTP client in this repo
(`github/http_client.go:17` — `ghHTTPClient = &http.Client{Timeout: 30 * time.Second}`;
`session/backlog_plugin_github.go:187,331,397` — same 30s pattern) uses a flat 30-second
`http.Client.Timeout`, and `session/repo_path.go` wraps git-network operations in
`context.WithTimeout(ctx, 60*time.Second)` / `120*time.Second` for clone. **None of these are on
the session-startup hot path** — they're triggered by explicit user actions (importing a GitHub
issue, polling PR status) or background pollers, which is the opposite of what a manifest fetch
gated on session/app startup would be. There is no existing precedent in this codebase for a
network call gating startup — which is itself informative: nothing here today makes app startup
wait on the network, and `requirements.md`'s own Non-Functional Requirements section already
calls this out ("fetch must not add user-perceptible latency to session/app startup... async
background refresh... is preferable to a startup-blocking fetch").

**herdr's actual behavior (VERIFIED via `WebFetch` against
`raw.githubusercontent.com/ogulcancelik/herdr/master/src/detect/manifest_update.rs`, and
cross-checked against a secondary source, GitHub issue
[ogulcancelik/herdr#677](https://github.com/ogulcancelik/herdr/issues/677), which independently
reports the same curl flags):**

- Fetch is **not** startup-gated in the way "fetch before first detection" would imply — it runs
  on a recurring **30-minute background cycle** (issue #677: cycles land near `:01`/`:31` of each
  hour), not synchronously blocking any particular session start.
- Network layer uses curl-equivalent parameters: `--connect-timeout 5` (5s to establish TCP),
  `--max-time 15` (15s hard ceiling on the whole request), `--max-filesize 262144` (256KB cap —
  notably the *same* 256 KiB figure this repo's own `detector-plugins` ADR-004 independently
  chose for `maxPluginFileSize`), and `--retry 2`.
- **Fallback: on fetch failure, herdr keeps the previously cached manifest, logs a warning, and
  continues** — it does not block, crash, or fall further back to some degraded no-detection
  state. This is the correct shape and matches what `requirements.md` already specifies
  ("falls back silently to the last-known-good cached or bundled manifest on any fetch
  failure/timeout").
- Caveat/unverified: herdr's own `--connect-timeout 5`/`--max-time 15` bounds a *single* curl
  invocation; I did not verify whether the surrounding Rust code wraps the *entire* multi-agent
  fetch loop (index + up to ~15 individual agent manifests, per issue #677's list) in an outer
  deadline, which matters if per-file timeouts could still sum to multiple hung minutes on a
  degraded network. Flagged as unverified rather than assumed either way.

**Recommended contract for this project, if it proceeds:** background-only fetch (never
startup-blocking, per the requirements doc's own preference), an outer `context.WithTimeout`
bounding the *entire* fetch-plus-parse operation (not just the HTTP round trip) at something in
the 5-15s range consistent with both herdr's numbers and this repo's own precedent, and a
guarantee — provable by a test that kills network access entirely — that detection still works
using bundled/cached manifests with zero user-visible effect on session startup latency.

## 3. Cache staleness / corruption risk

- **Partial writes during a crash mid-fetch.** This repo already has a strong, consistent
  convention for this exact hazard: `config/config.go:833`, `config/claude.go:211`,
  `config/state.go:300`, and `config/workspace_meta.go:57,89` all write to a temp file and
  `os.Rename` it into place atomically — never write-in-place to the live path. A manifest cache
  writer must follow this same pattern (and per
  `.claude/rules/prefer-go-git-over-subshells.md`'s general spirit of reusing what the repo
  already has, should call the existing atomic-write helper rather than reinventing it).
  herdr's own implementation does the equivalent (VERIFIED via the raw-source fetch above):
  writes to a temp path suffixed with PID + nanosecond timestamp, then renames, with a
  best-effort parent-directory `fsync` for durability (gracefully skipped on filesystems that
  don't support it, e.g. Windows) — the same shape this repo's `config/` package already
  converged on independently.
- **Version-comparison logic that gets stuck.** herdr's version type (VERIFIED) does
  dotted-numeric segment comparison (`"1.2.0" == "1.2"`, missing segments treated as zero) and
  explicitly **rejects both downgrades and version-unchanged-but-content-changed payloads** —
  i.e., it refuses to silently swap in different pattern content under an unbumped version
  number, and refuses to move backward. Both are worth adopting directly: a "never downgrades"
  rule prevents a bug where a broken or malicious manifest with a *lower* version number could
  otherwise still be accepted if the comparison only checked "is this a valid version string"
  rather than "is this version >= what I already have," and a "content must match version"
  rule closes the gap where a compromised host serves different bytes under an unchanged
  version tag to evade update-conflict detection.
- **A bad manifest bricking detection for ALL agents.** This is the sharpest version of the
  "blast radius" question, and the repo already has the right structural answer at hand: the
  `detector-plugins` requirement #3 (log-and-skip, not fail-fast for the whole process) and the
  copy-on-write registry swap already recommended in `project_plans/detector-plugins/research/
  pitfalls.md` §4 (build the new `PatternSet`/registry fully off to the side, validate it, and
  only then swap it into an `atomic.Pointer` — `session/detection/detector.go:49` already uses
  `atomic.Pointer[PatternSet]` for exactly this reason) generalize cleanly to remote manifests:
  a fetch that produces an invalid or unparseable manifest for agent X should fail *that agent's*
  merge step only, leaving X's last-known-good (cached or bundled) pattern set live, and must
  never be able to corrupt or block the load of any other agent's detector. The one *new* failure
  mode remote fetch adds beyond what `detector-plugins` already handles: a manifest that is
  *structurally valid* (passes the schema validator) but semantically wrong (e.g., an
  `needs_approval` pattern quietly deleted or narrowed) — this is not caught by any validator,
  structural or otherwise; it's the same "syntactically valid, semantically dead" class the
  detector-plugins pitfalls doc already flagged from fail2ban prior art (§5), except here the
  stakes are "silently stops asking for human approval" rather than "one plugin doesn't badge
  correctly." No purely automated check closes this gap; it argues for the pinned-SHA/manual-
  review mitigation in §1 over pure trust-on-first-use.

## 4. Operational/maintenance risk specific to this project

`detector-plugins`' `requirements.md` names "no server fleet" as an explicit design constraint
this repo already operates under, and this item's own `requirements.md` (§Constraints) already
concedes the point directly: *"Any new remote endpoint/CDN is new infrastructure this
personal-scale project would need to stand up and maintain indefinitely (availability, TLS cert
renewal, abuse/cost exposure) — unlike `detector-plugins`, which added zero new runtime
infrastructure."*

Weighing this against what it buys: `detector-plugins` already gives Tyler — the sole
target user this item names (`requirements.md` §Users/Consumers: "Solo/small-team desktop tool
— no server fleet, no enterprise fleet-management use case, no telemetry") — a fix path that is
strictly faster than anything a remote-fetch layer could offer: **edit
`~/.stapler-squad/detectors/*.toml` by hand, save, and hot-reload picks it up with no restart**
(`detector-plugins` requirement #5, already shipped and tested). A remote-fetch layer's entire
value proposition is "push a detection fix to a *running instance you are not currently at your
keyboard for*, without a release." For a single-operator desktop tool where the operator is
present at the machine whenever they'd notice detection is broken (they're the one watching the
session that broke), that scenario — fix needed on a machine Tyler isn't actively using, urgently
enough to not wait for the next `go build`/`make install-service` — is narrow, and even in that
scenario, `git pull && go build` on a second machine addresses it without any new infrastructure
either, since this is Tyler's own repo he already has push access to and no other party needs to
consume the manifest for it to be "shipped." Even genuinely counting the good case (a
detection break that affects a machine Tyler isn't at, that Tyler wants fixed before a rebuild is
convenient), the maintenance liability — a static host that must keep resolving DNS, keep a
valid TLS cert, and keep serving correctly-signed/pinned content *indefinitely*, forever, for a
tool with zero other consumers who'd notice or report an outage — is a standing cost paid on
every day nothing goes wrong, in exchange for a benefit that only materializes on days something
does go wrong *and* Tyler isn't at the affected machine *and* isn't willing to `git pull`. That
asymmetry (constant carrying cost vs. narrow, already-otherwise-solvable benefit) is exactly the
shape of risk `requirements.md`'s own Constraints section is naming, and it's larger, not
smaller, than it first appears once the *already-shipped* local alternative is priced in as the
actual counterfactual, rather than "no fix mechanism at all."

## 5. Premature-build risk

Yes — this is a named, general category ("building infrastructure before validating anyone hit
the problem it solves"), and this item is close to a textbook instance of it, not just
adjacent to the pattern. The direct citation, from `project_plans/detector-plugins/requirements.md`
§Success Metric (the section this item's own `requirements.md` already quotes in its "Critical
prior-art finding" section, lines 39-42):

> If a new agent *is* onboarded in that window and it still ships as a `binaries/*.go` PR
> instead of a `.toml` file, the demand assumption below was wrong and the feature should not
> be extended further (e.g. **the deferred remote-manifest/issue-#178 work should not proceed**,
> and the cheaper alternative below should be tried instead).

and §Risky Assumption (same document):

> **Named, not yet validated:** that there exists real (not hypothetical) demand for detecting
> non-built-in agents, sufficient to justify a TOML schema, validator, and hot-reload watcher
> over the alternative of "just PR a `binaries/*.go` file, it's a ~40-line addition."... there
> is no usage data, user request thread, or count of "I run a private agent CLI" reports backing
> it.

This item (remote-manifest distribution) is a *second-order* extension of a *first-order*
feature (`detector-plugins`) whose own demand is explicitly unvalidated, on an explicit 90-day
clock that is 4 days elapsed as of this research (2026-08-06; checkpoint target ~2026-10-31).
Building the remote layer now would mean: (a) adding new indefinite-maintenance infrastructure
(§4) and a new security-relevant trust boundary (§1) (b) on top of a foundation
(`detector-plugins`) whose own justification is still an open question, (c) to serve a
distribution problem — "push a fix without a release" — that the foundation already solves for
the sole named user via a 30-second manual file edit (§4), making the marginal problem this item
would solve narrower than the problem `detector-plugins` itself was scoped to solve. Every
signal points the same direction: this is not a case where the premature-build risk is a vague
category concern to flag defensively — it is the specific, named, already-documented risk this
exact item was called out against by its own predecessor's requirements doc, four days into the
window meant to resolve it.

## Summary

1. **Spoofing (§1):** TLS-only + reuse of the existing `detector-plugins` structural validator
   is the honest floor; pinned-commit-SHA is proportionate and cheap if the source is this
   repo's own `raw.githubusercontent.com`; full cryptographic signing is overkill for a
   single-publisher, no-marketplace project (the project's own "Rabbit Holes" section already
   says so).
2. **Blocking (§2):** No existing network call in this codebase gates startup; keep it that way.
   herdr's verified behavior (30-min background cycle, 5s connect/15s max/256KB cap via curl
   flags, cache-and-continue on failure) is a reasonable reference contract — background-only,
   never startup-blocking, matching this item's own NFR.
3. **Staleness/corruption (§3):** Reuse this repo's existing atomic-write convention
   (temp-file + `os.Rename`, already used in `config/`) and the `detector-plugins`
   copy-on-write registry-swap pattern (`atomic.Pointer[PatternSet]`) for blast-radius
   containment; adopt herdr's verified never-downgrade + version-must-match-content rules.
   No automated check catches a structurally-valid-but-semantically-dead approval-gate pattern —
   that residual risk is exactly what pinning/review (§1) exists to cover, not validation.
4. **Operational burden (§4):** Larger than it first appears once `detector-plugins`'
   already-shipped local-edit path is priced in as the real counterfactual — a standing,
   indefinite maintenance cost (DNS/TLS/hosting) paid every day, for a benefit (fix a machine
   Tyler isn't at, without `git pull`) that is narrow for a single-operator tool.
5. **Premature build (§5):** This item is a second-order extension of a first-order feature
   whose own demand is explicitly unvalidated on an active, named 90-day checkpoint (4/90 days
   elapsed) — the strongest and most concrete finding in this research pass, directly citable to
   `project_plans/detector-plugins/requirements.md`'s own Success Metric and Risky Assumption
   sections.

## Sources

- [Issue #677 — Document and make remote agent-detection manifest updates configurable · ogulcancelik/herdr](https://github.com/ogulcancelik/herdr/issues/677) — secondary source, independently corroborates the curl flags and 30-minute cycle
- `raw.githubusercontent.com/ogulcancelik/herdr/master/src/detect/manifest_update.rs` — primary source, fetched and summarized via `WebFetch` (2026-08-06); not read byte-for-byte by this agent, so treat granular claims (exact struct/field names) as WebFetch-summarized, not directly verified line-by-line

## Repo files referenced

- `project_plans/remote-detection-manifests/requirements.md`
- `project_plans/detector-plugins/requirements.md` (§Success Metric, §Risky Assumption, §Scope)
- `project_plans/detector-plugins/decisions/ADR-004-plugin-trust-boundary-and-resource-caps.md`
- `project_plans/detector-plugins/research/pitfalls.md` (§4 copy-on-write registry-swap pattern)
- `session/detection/pattern_set.go`, `session/detection/detector.go:49` (`atomic.Pointer[PatternSet]`)
- `github/http_client.go:17`, `session/backlog_plugin_github.go:187,331,397` (existing 30s HTTP timeout convention)
- `session/repo_path.go:222,254,421,461` (existing `context.WithTimeout` convention for network-touching git ops)
- `config/config.go:833`, `config/claude.go:211`, `config/state.go:300`, `config/workspace_meta.go:57,89` (existing atomic temp-file + `os.Rename` write convention)
