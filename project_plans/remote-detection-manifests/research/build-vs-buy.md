# Build vs. Buy: Remote Fetch/Cache Layer for Detection Manifests

Research for `project_plans/remote-detection-manifests/requirements.md` — the
herdr-inspired remote-fetch capability (issue #178) layered on top of the
already-shipped `detector-plugins` local TOML loader (commits `3c25e94f9` /
`005e75827`, merged 2026-08-02). Scope here is **only** the remote-fetch/cache
piece — parsing, validation, hot-reload, and the trust-boundary decisions for
plugin *content* are already settled by `detector-plugins`' own build-vs-buy
(`project_plans/detector-plugins/research/build-vs-buy.md`) and its ADRs, and
are not re-litigated.

**Framing note, load-bearing for every verdict below**: `requirements.md`
documents that this item is explicitly gated on a 90-day demand-validation
checkpoint owned by the sibling `detector-plugins` project (target
~2026-10-31), and that as of this triage (2026-08-06) the checkpoint is 4
days into that window — effectively unstarted, no signal either way yet. That
fact changes the shape of a build-vs-buy analysis: the strongest "buy"
option available right now is *not building anything at all*. Option 5 below
makes that explicit rather than leaving it implied.

## Repo state checked

- `go.mod` has no HTTP client library beyond stdlib `net/http` usage
  elsewhere in the codebase, no `google/go-github`, no `hashicorp/go-getter`,
  no feature-flag SDK (LaunchDarkly, Flagsmith, Unleash, etc.), and no
  generic remote-config library (Viper's remote-provider plugin, koanf's
  provider ecosystem) — confirmed via `grep -iE "go-github|go-getter|viper|
  koanf|launchdarkly|flagsmith|unleash" go.mod`, only match is an indirect,
  unrelated `go-viper/mapstructure` transitive dependency (same one
  `detector-plugins`' own research already noted is not evidence of Viper
  integration).
- `detector-plugins`' ADR-003 (plugin TOML schema: `id`, `binary_names`,
  `version`, `[[patterns]]`) and ADR-004 (trust boundary: RE2-only regex, no
  code execution, resource caps, no sandbox) are the two decisions this
  remote layer must extend without weakening — a compromised/spoofed fetch
  endpoint is explicitly named in `requirements.md` as a *new* trust boundary
  ADR-004 never had to consider, since ADR-004 only reasoned about
  locally-authored files a user deliberately placed on their own disk.

## 1. Existing OSS library for "fetch versioned config from a URL, cache, fall back on failure"

Looked for a Go library that does the actual shape of this job: fetch a
small versioned document from a URL, compare against a cached copy, write
the new one atomically, and silently keep using the old one on any failure.

- **`google/go-github`** (or any GitHub-API-client SDK) — wrong tool for the
  job. `requirements.md` §Out of Scope explicitly names the target as "GitHub
  raw content or an equivalently zero-maintenance static host, not a new
  service" — i.e. a plain `https://raw.githubusercontent.com/.../manifest.toml`
  GET, not an authenticated GitHub API call (which would need a token, hit
  API rate limits shared with every other `gh`/API use on the machine, and
  pull in an SDK sized for the entire GitHub REST/GraphQL surface for a
  single unauthenticated file fetch). A raw-content URL is a plain HTTP GET;
  no GitHub-specific client adds value over `net/http`.
- **`hashicorp/go-getter`** — designed for fetching and unpacking full
  artifacts (tarballs, git repos, S3/GCS objects) with a plugin-per-protocol
  architecture. Correctly sized for Terraform module fetching; wrong shape
  here (single small file, no unpacking, no multi-protocol needs) and would
  import a materially larger dependency surface than the job requires.
- **Viper's remote provider / koanf's provider ecosystem** — both support
  "fetch config from etcd/Consul/S3/HTTP and merge it in," which sounds
  adjacent, but `detector-plugins`' own build-vs-buy already rejected
  Viper/koanf for the *local* loader on a domain-shape mismatch (N
  independently-validated plugin documents vs. one merged config tree). That
  mismatch is unchanged — arguably worse — for the remote case: these
  providers are built to poll/merge into a single struct, not to "fetch one
  file, version-compare it against a cached copy, and fall back to the last
  good one on any error," which is the actual state machine this item needs.
- **A generic "remote config" library in general** — no widely-used,
  actively maintained Go library exists purpose-built for exactly this
  narrow shape (versioned single-document fetch + local cache + silent
  fallback). The libraries that exist either solve a bigger problem
  (multi-source config aggregation, artifact fetching) or a narrower one
  (raw HTTP GET, which is stdlib already).

**What the job actually requires**, per `requirements.md`'s own NFRs: an
`http.Client` with a short timeout (herdr's own reference point, cited in
requirements.md, is a 2s fallback), an HTTP GET against a configured URL,
version comparison against the cached file (reusing the `version` field
already in ADR-003's schema), an atomic write to
`~/.stapler-squad/detection-manifests/` (or the existing
`~/.stapler-squad/detectors/` dir — open question #3 in requirements.md),
and passing the result through the exact validator `detector-plugins`
already built and tested. Every piece of that is either stdlib
(`net/http`, `os.Rename` for atomic write) or already-shipped code
(`ADR-003` schema, the ADR-004-governed validator). This is a well-scoped
~50–100 line addition, not a gap any library fills better than hand-rolling.

**Verdict: Not recommended (no library adopted) — a hand-rolled fetcher
using stdlib `net/http` + the existing TOML validator is genuinely the
simplest option, if/when this proceeds.** No candidate library removes
custom code that must be written anyway (version comparison against the
existing schema, atomic cache write, fallback-on-any-error), and each
candidate considered adds either the wrong domain shape (Viper/koanf,
already rejected once for the sibling project) or a larger dependency
surface than a single unauthenticated file GET needs (go-github,
go-getter).

## 2. SaaS / managed config-distribution service

Considered feature-flag/config-distribution SaaS (LaunchDarkly, Flagsmith,
Unleash, ConfigCat) and generic CDN-with-versioning options (Cloudflare
Workers KV, a Cloudinary-style asset CDN with cache-busting).

**Assessment: overkill, and not just by default-caution — the specific
reasons are:**

- **No fleet to manage.** `requirements.md`'s own Users/Consumers section
  reiterates the same posture `detector-plugins` established: "solo/small-
  team desktop tool — no server fleet, no enterprise fleet-management use
  case, no telemetry." Feature-flag SaaS exists to solve targeted rollout
  and kill-switches *across many running instances you don't have shell
  access to* — the entire value proposition assumes a fleet. A single
  developer's local `stapler-squad` instance polling its own laptop has
  nothing for that value prop to act on.
- **New account, new secret, new failure mode, for a personal tool.**
  Every SaaS option here means: sign up, manage an API key/SDK key (a new
  secret to rotate and to accidentally leak), depend on that vendor's
  uptime for a codepath the NFRs explicitly require to degrade silently to
  cached/bundled content — which then raises the question of why depend on
  a paid vendor at all if the entire design point is "must work exactly the
  same when it's unreachable."
- **Cost asymmetry.** Free tiers exist for most of these (LaunchDarkly,
  ConfigCat, Unleash open-source self-hosted), so raw dollar cost isn't the
  blocker — the blocker is operational: standing up and maintaining an
  account/project for a single static TOML file that already has a
  zero-maintenance alternative (`requirements.md`'s own stated target,
  "GitHub raw content ... not a new service") is pure overhead with no
  corresponding benefit for this use case.
- **A raw file host already satisfies every stated NFR.** GitHub raw
  content (or an equivalent static host) gives versioning for free (git
  history / commit SHA), doesn't require a new account beyond one the
  project maintainer already has, and matches `requirements.md`'s explicit
  Out-of-Scope line ruling out "hosting/ops for a bespoke CDN."

**Verdict: Not recommended.** Not a reflexive "SaaS is always overkill" —
specifically: no fleet to target, an existing zero-maintenance static-host
option already satisfies every NFR in requirements.md, and every SaaS
option adds a new secret/account/vendor-uptime dependency to a codepath
whose entire design goal is graceful degradation to *not needing* that
dependency.

## 3. The cheaper alternative already named by the sibling project

`detector-plugins/requirements.md`'s Risky Assumption section names this
explicitly as the alternative to try before building *any* user-extensible
detection system, local or remote: **"keep detection Go-only, and lower the
bar for landing a new built-in detector PR (faster review, a documented
template)."** Per that same document, this alternative was **deliberately
not tried before `detector-plugins` shipped** — it is not "already
attempted and found insufficient," it is genuinely still on the table and
unexplored.

**What it would concretely look like**, inferred from the existing
`session/detection/binaries/{claude,aider,gemini,opencode}.go` structure
(each a small file implementing `dtypes.BinaryDetector` — `Name()`,
`Patterns()`, `FilterContent()`):

- A `binaries/TEMPLATE.go` or a documented "adding a new detector" section
  in the repo's contribution docs, showing the exact shape a new
  `binaries/*.go` file needs (the three-method interface, an example
  `StatusPatterns` block, a pointer to `pattern_set.go`'s compile/validate
  path).
- A committer note (CI check or PR-template checklist) that a PR touching
  only `binaries/*.go` — no `session/detection` core logic — gets an
  expedited review path: e.g. any one maintainer can approve/merge without
  waiting for full review-queue cycling, since the blast radius of a wrong
  regex is bounded by ADR-004's own threat-model reasoning (regex-only, no
  code execution) — the same safety argument ADR-004 already made for
  *plugin* content applies just as well to a *PR adding* that same content
  as a built-in.
- Optionally, a lightweight test harness/fixture (sample terminal output →
  expected status) so a contributor proposing a new detector can validate
  it locally without needing a live session of the target agent CLI running.

**Effort/risk comparison against building fetch infrastructure:**

| | Lower-friction PR template | Remote-fetch infrastructure |
|---|---|---|
| New runtime infra | None | Yes — endpoint to keep reachable indefinitely (requirements.md's own Constraints section flags this as new maintenance burden `detector-plugins` never incurred) |
| New trust boundary | None — still a reviewed PR merged by a human | Yes — spoofed/compromised endpoint pushing malicious detector definitions, explicitly called out in requirements.md §Constraints |
| Code to write | A template file + doc section + PR-checklist note (hours) | Fetch/cache/version-compare/fallback logic + config for the endpoint + tests for offline/timeout/corrupt-response paths (the few-days estimate requirements.md itself gives) |
| Solves the actual stated problem? | Yes, directly — "detection-pattern fixes can ship faster than a full release" is equally solved by same-day PR merge + a fast release cadence for that file, without any new fetch path | Only if release cadence is genuinely the bottleneck, which is unconfirmed |
| Validates or invalidates demand | Is literally the mechanism the 90-day checkpoint is measuring (checkpoint asks: did a new agent land as `binaries/*.go` or as `.toml`?) | Does not touch the checkpoint at all — orthogonal to demand validation |

This option is strictly cheaper and lower-risk, and — critically — it is
*the same lever the checkpoint itself is measuring*. Making the built-in PR
path faster and better-documented doesn't just compete with the remote-fetch
approach on cost; it actively helps produce a cleaner signal for the
checkpoint (a contributor who finds the PR path easy and fast has less
reason to reach for a local `.toml` plugin or ask for remote distribution,
making a subsequent "no one adopted `.toml` either" reading more meaningful).

**Verdict: Recommended as the near-term action, if any action is taken at
all right now.** Available, unexplored, and net cheaper than every
technical option in sections 1–2 by a wide margin — hours of documentation
work vs. days of new fetch/cache/trust-boundary infrastructure.

## 4. Fork or adapt herdr directly

`requirements.md` cites herdr's `src/detect/manifest_update.rs` as the
originating design reference (issue #178) and states herdr's own fallback
timing (2s) as a design data point.

**I do not have internet/repo access in this task to fetch herdr's actual
source** (`ogulcancelik/herdr`) — everything below is bounded strictly to
what `requirements.md` itself already states about herdr, not independent
verification of herdr's implementation. I am not fabricating details about
`manifest_update.rs`'s actual logic, retry behavior, caching strategy, or
manifest source beyond what's quoted in requirements.md.

What can be assessed without reading herdr's source:

- **Language boundary rules out literal code porting.** herdr is Rust
  (`.rs` file extension confirms this); stapler-squad is Go. There is no
  "fork the file" option — any reuse is necessarily a from-scratch Go
  reimplementation of whatever design herdr's approach represents, not an
  adaptation of its actual code.
- **The manifest *format* is not being forked from herdr either, by
  requirements.md's own explicit design choice.** §Scope states: "A
  versioned manifest format compatible with the existing `detector-plugins`
  TOML schema (`id`, `binary_names`, `version`, `[[patterns]]`) — not a
  competing JSON format; reuse, don't fork the schema the sibling project
  just shipped and tested." So even the *data shape* herdr may use is
  explicitly not being adopted — this item extends `detector-plugins`'
  already-shipped ADR-003 schema, which was designed independently of
  herdr's own manifest format (unverified here what that format actually
  is).
- **What genuinely could be adapted, if the source were reviewed**: the
  *design reference* herdr represents — versioned manifest, remote fetch,
  local cache, fallback-on-failure, per requirements.md's paraphrase — is
  already fully captured in this document's §1 analysis (the state machine:
  fetch → version-compare → atomic write → fallback) without needing
  herdr's source, because that state machine is generic enough to describe
  from the one-paragraph summary in the originating issue. Reading herdr's
  actual Rust would only be valuable for edge-case handling specifics (exact
  retry/backoff behavior, exact cache invalidation logic) that
  requirements.md doesn't currently claim to need beyond the single "2s
  fallback" data point it already quotes.

**Verdict: Not viable to fork/port literally (language mismatch); the
design-reference value herdr provides is already fully captured in
requirements.md's own paraphrase and this document's §1 state-machine
description, so a from-scratch Go implementation is required regardless of
whether herdr's source is ever read.** If this item proceeds past the
checkpoint, reading herdr's actual `manifest_update.rs` before implementation
is worth an hour compared to going in blind — but that's a due-diligence
step for the *implementation* phase, not a build-vs-buy input that changes
which of sections 1–2's options is chosen.

## 5. Do-nothing / defer until the checkpoint resolves

Separate from *which* technical approach (sections 1–4) would be used if
this proceeds, is *whether to proceed at all right now* — a build-vs-buy
axis of its own, since "buy nothing, build nothing, wait" is a legitimate
answer to a build-vs-buy question, not an abdication of one.

**Pros:**
- Directly matches `requirements.md`'s own explicit framing: "this document
  does not recommend building it now," and lists "building this before the
  `detector-plugins` 90-day demand checkpoint resolves" as Out of Scope.
- The checkpoint is designed to produce exactly the signal needed to answer
  sections 1–4 with confidence instead of speculation: whether a real
  contributor, given the *already-shipped* local `.toml` plugin path, still
  chooses to file a `binaries/*.go` PR instead. If they do, per
  `detector-plugins/requirements.md`'s own stated consequence, "the deferred
  remote-manifest/issue-#178 work should not proceed" — i.e. building this
  now risks building infrastructure for a demand signal that may resolve
  negative in the next ~86 days.
- Avoids taking on new maintenance surface (an endpoint to keep available,
  a new trust boundary per requirements.md §Constraints) before there is
  evidence anyone needs it.
- Zero cost today. The absolute cheapest option among all five considered.

**Cons:**
- If demand is real and urgent (e.g. a built-in agent's UI breaks detection
  in a way that can't wait for a release), deferring means that fix still
  has to go through a full release cycle in the meantime — unless §3's
  cheaper-PR-process alternative is adopted in parallel, which has no
  dependency on the checkpoint and could ship immediately. That latency risk
  is bounded, not eliminated, by inaction.
- 90 days is a long window; if the checkpoint's own tracking (per
  `detector-plugins/requirements.md`, self-tracked via a backlog item) lapses
  without anyone checking it, deferral risks becoming permanent-by-neglect
  rather than a deliberate re-evaluation — a process risk, not a technical
  one, but real given this is a personal-scale project without a PM forcing
  function.

**Verdict: Deferred pending checkpoint — this is the correct answer to
"should the remote-fetch capability be built" right now, independent of
which technical option in §1 or §2 would be chosen if it does proceed.**
This is not a default non-answer; it's the same conclusion
`requirements.md` itself already reaches, restated here as a first-class
build-vs-buy option because "wait for evidence" competing directly against
"build now" is exactly what this section of research is supposed to weigh.

## Summary Table

| Option | Pros | Cons | Verdict |
|---|---|---|---|
| OSS remote-config library (go-github, go-getter, Viper/koanf remote providers) | Off-the-shelf fetch/merge machinery | Wrong domain shape (multi-source merge vs. single versioned file) or wrong scale (GitHub API client, artifact-unpacking tool) for "GET one small file, cache it, fall back on failure"; removes no custom code that must be written anyway | Not recommended |
| Hand-rolled fetcher (stdlib `net/http` + existing TOML validator) | ~50–100 lines; reuses `detector-plugins`' already-shipped/tested schema and validator; no new dependency | Is new code to write and test (fetch, version-compare, atomic cache write, fallback paths) | Viable (if/when the item proceeds) |
| SaaS config-distribution / feature-flag service | Versioning, rollout controls out of the box | No fleet to target the value prop at; new account/secret/vendor-uptime dependency on a codepath designed to not need one; a free static host already satisfies every NFR | Not recommended |
| Lower-friction built-in-detector PR process (§3) | Directly solves the stated problem (fast-turnaround detection fixes) without new infra or trust boundary; is literally the mechanism the demand checkpoint measures; hours of work | Doesn't give community/non-maintainer contributors a self-serve path (still requires a maintainer to merge) | **Recommended near-term action** |
| Fork/port herdr directly | Existing design reference already informs requirements.md | Rust → Go language boundary rules out literal porting; manifest format explicitly not being forked per requirements.md's own scope; herdr's actual source unverified in this research | Not viable as a port; worth a read for implementation-detail due diligence only, not a build-vs-buy input |
| Defer until 90-day checkpoint resolves (§5) | Zero cost now; avoids building infra for unvalidated demand; matches requirements.md's own explicit framing; checkpoint is designed to produce exactly this signal | Any urgent detection-pattern fix in the meantime still needs a full release unless §3 is adopted in parallel; risk of the checkpoint lapsing unchecked over 90 days | **Recommended — current default verdict** |

## Overall Recommendation

Two recommendations at different time horizons, not in conflict:

1. **Right now**: defer the remote-fetch capability itself (§5) — this
   matches `requirements.md`'s own explicit conclusion and is the correct
   build-vs-buy answer to "should this be built today." In parallel, pursue
   §3 (lower-friction built-in-detector PR template + expedited review note)
   as the immediately actionable, near-zero-cost step that both solves the
   underlying "detection fixes ship slowly" problem today *and* sharpens the
   very signal the 90-day checkpoint is trying to measure.
2. **If/when the checkpoint validates proceeding** (~2026-10-31 or later):
   build a hand-rolled fetcher directly on stdlib `net/http` plus the
   already-shipped `detector-plugins` TOML schema/validator (§1's
   conclusion) — no OSS remote-config library and no SaaS service fits this
   narrow "fetch one small versioned file, cache, fall back silently" shape
   better than ~50–100 lines of new code reusing infrastructure that
   already exists and is already tested. herdr's source is worth a read at
   that point for implementation-detail reference, but is not something to
   fork or port given the Rust/Go language boundary.
