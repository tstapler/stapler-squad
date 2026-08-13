# Build vs. Buy: TOML-Based Detector Plugin Loading

Research for `project_plans/detector-plugins/requirements.md` (user-extensible
agent detector plugins via TOML files in `~/.stapler-squad/detectors/`, with
validation, merge-with-built-ins, and fsnotify hot-reload).

## Current repo state (confirmed via go.mod / grep)

- `github.com/fsnotify/fsnotify v1.9.0` is **already a direct dependency**,
  used in 12+ files (`main.go`, `daemon/daemon.go`, `session/history_watcher.go`,
  `session/unfinished/watcher.go`, `session/mux/autodiscover.go`, etc.). Directory
  watching is an established, repo-idiomatic pattern here — not a new
  integration risk.
- **No TOML library is currently a dependency** (`grep -i toml go.sum` — no
  hits). One must be added regardless of the loader architecture chosen.
- `github.com/xeipuuv/gojsonschema` exists as a (transitive, via buf tooling)
  dependency but isn't used for runtime config validation elsewhere; not a
  natural fit for TOML anyway (would require TOML→JSON round-tripping).
- `github.com/go-viper/mapstructure/v2` is present but only as an **indirect**
  transitive dependency (pulled in by something else, likely buf/atlas
  tooling) — not evidence that Viper itself is already integrated.
- `dtypes.BinaryDetector` is a 3-method interface (`Name() string`,
  `Patterns() StatusPatterns`, `FilterContent(string) string`); `PatternSet`
  (`session/detection/pattern_set.go`) compiles `StatusPatterns` into
  `*regexp.Regexp` slices per category using stdlib `regexp.Compile` and
  returns a wrapped error naming the pattern on failure — this is exactly the
  validation hook a plugin loader needs to reuse unmodified.

## 1. Existing OSS library for the plugin-loading mechanism itself

The full requirement — watch a directory of independently-authored TOML
files, validate each one against a domain schema, merge results into a
registry, hot-reload on change, log-and-skip on a single bad file — is not
one thing any config library ships. It decomposes into three separable
concerns, evaluated separately below (parsing, watching) plus this one:
**config-aggregation frameworks (Viper, koanf)**.

**Viper (`spf13/viper`)** and **koanf (`knadh/koanf`)** both support:
TOML parsing, multi-source merging, and file-watching with reload callbacks.
On paper they look like a fit. In practice neither matches this domain:

- Both are built around **merging N sources into one flat/nested config
  tree**, read once into a single struct — not **N independently-identified
  plugin documents**, each producing its own domain object (a
  `BinaryDetector`) with **per-file validation and per-file error isolation**.
  Getting "file B has a bad regex, so skip only file B, keep A and C, and
  keep all built-ins" out of Viper/koanf's merge model means fighting the
  library's assumptions rather than using them — you'd still hand-write the
  per-file iteration, per-file error wrapping, and id/binary_names collision
  detection.
- Viper's known TOML-correctness issue — it force-lowercases keys, which
  breaks the TOML spec for case-sensitive keys — is a real footgun for a
  user-authored plugin format where `binary_names` values are literal binary
  names (potentially case-sensitive on some platforms).
- Both pull in non-trivial dependency trees (Viper especially: afero, pflag,
  etc.) for a feature this repo needs a thin slice of.
- Neither reduces the amount of domain-specific code needed: constructing a
  `dtypes.BinaryDetector` from parsed TOML, mapping `status` strings to
  `StatusPatterns` fields, and enforcing "user plugin overrides built-in
  by binary name" are unavoidably custom regardless of the aggregation layer.

**Verdict: Not recommended.** Neither Viper nor koanf's core value
proposition (multi-source config merging into one tree) matches this
domain's shape (N independent, individually-validated plugin documents). Adopting
either would add a dependency and an impedance mismatch to work around,
without removing any of the actual custom code (TOML→domain-object mapping,
per-file error isolation, override-by-binary-name) that has to be written
either way.

## 2. SaaS / managed API

Not applicable. This is a local file-loading feature reading from
`~/.stapler-squad/detectors/` on the same machine the process runs on; there
is no external service to call, no data to sync, and the requirements
explicitly scope out remote/distributed manifest fetching (issue #178
territory) as a non-goal. No further evaluation needed.

## 3. LLM-generated implementation vs. battle-tested library, per concern

### 3a. TOML parsing — use a library, do not hand-roll

**`github.com/pelletier/go-toml/v2`** is the recommended choice.

- It is the modern, actively maintained TOML library for Go; `BurntSushi/toml`
  (the older community-standard default) is now effectively unmaintained, and
  its own community has been migrating to go-toml v2.
- go-toml v2 benchmarks 1.7x–5.1x faster than BurntSushi/toml on typical
  unmarshal workloads (not performance-critical here, but it's the strictly
  better default with no downside).
- API is `encoding/json`-shaped (`toml.Unmarshal([]byte, &struct{})`), so a
  detector struct with `toml:"..."` tags mirrors the existing pattern used for
  `StatusPatterns`/`StatusPattern` JSON tags elsewhere in `dtypes.go` — low
  friction to adopt.
- Alternative considered: `BurntSushi/toml` — simpler API, but unmaintained
  upstream; not worth adopting for new code in 2026.

Hand-rolling a TOML parser is not a serious option: TOML has a real grammar
(nested tables, arrays of tables — `[[patterns]]` is exactly an array-of-tables
construct — inline tables, multiple string/datetime forms) and a hand-rolled
parser would be a correctness and security liability for zero benefit over an
existing, widely-used library already provided as a small, dependency-light
import.

**Verdict: Recommended — add `github.com/pelletier/go-toml/v2` as a new
direct dependency.**

### 3b. Regex compilation/matching — stdlib `regexp` is correct, keep it

`session/detection/pattern_set.go` already compiles all patterns with Go's
stdlib `regexp.Compile` (RE2 engine). This is the right choice to continue
for plugin-supplied patterns, and arguably more load-bearing here than for
built-in patterns, because plugin regexes are **untrusted user input**:

- RE2 guarantees **linear-time matching** with no backtracking — it cannot
  exhibit catastrophic backtracking (the classic ReDoS vector) regardless of
  the pattern or input, because RE2 does not implement backreferences or the
  backtracking-based matching that makes exponential blowup possible in PCRE-style
  engines. This is exactly the safety property the requirements doc's
  non-functional requirement #3 ("regex compilation must not be able to hang
  the process") asks for — and stdlib already provides it for free.
  `PatternSet.compile()`'s existing `regexp.Compile` + wrapped-error-on-failure
  pattern needs zero changes to be reused for plugin-sourced patterns; the
  requirements doc's own suggestion ("a max-pattern-length guard") is the only
  incremental hardening worth adding, purely to bound memory/compile time for
  pathologically large pattern strings, not because RE2 can hang.

**Verdict: Recommended — continue using stdlib `regexp`, unchanged. No new
regex engine needed or wanted.**

### 3c. fsnotify-based hot reload — fsnotify is correct, already proven in this repo

Go's stdlib has no directory-watch primitive (no `os.Watch`-equivalent), so a
kernel-notification library is required for anything better than polling.

- `github.com/fsnotify/fsnotify` is already a **direct dependency at v1.9.0**
  (current as of the May 2026 release) and is used for exactly this kind of
  job in 12+ places in this codebase already (`session/unfinished/watcher.go`,
  `session/history_watcher.go`, `session/mux/autodiscover.go`, etc.) — there's
  an established, working idiom in this repo to follow, not a new integration
  to design from scratch.
- There is no meaningful "fsnotify v2 fork" to consider for this project:
  `fsnotify/fsnotify` itself is the actively maintained, canonical project
  (last release May 2026); the numbering the requirements doc's phrasing
  hints at doesn't correspond to a separate maintained fork worth switching to.
- A hand-rolled poll loop (stat the directory on a timer, diff mtimes) would
  need to reinvent debouncing, rename/atomic-write handling (editors often
  write-then-rename), and cross-platform behavior that fsnotify has already
  solved and this repo already trusts elsewhere — strictly worse for equal or
  more code.

**Verdict: Recommended — reuse the existing `fsnotify/fsnotify` dependency,
following the pattern in `session/unfinished/watcher.go`. No new dependency,
no hand-rolled poll loop.**

## 4. Fork or adapt an existing "TOML plugin directory with hot reload" project

Looked at how comparable tools structure drop-in plugin directories:

- **Caddy** (`caddyserver/caddy`) — plugin system is Go-native (build-time
  `plugin.go` registration via blank imports / `caddy.RegisterModule`), not a
  runtime-loaded config-file plugin format at all. Nothing to adapt — the
  entire mechanism is a different shape (compile-time module registration,
  not runtime file drop-in).
- **Vector.dev** — configuration is TOML/YAML/JSON, but its "plugins" are
  compiled-in components selected by config, not third-party runtime-loaded
  detector logic; same shape mismatch as Caddy.
- **Grafana** — plugin system (data source/panel plugins) is a full
  process-isolated plugin architecture (separate binaries communicating over
  gRPC via `hashicorp/go-plugin`), built for a much larger surface (arbitrary
  plugin *behavior*, not just declarative pattern matching) — explicitly out
  of scope per this project's non-goals (no code execution, no Lua/scripting
  runtime). Adapting it would import far more machinery (process
  supervision, RPC framing, plugin discovery protocol) than this feature
  needs or wants.

None of these projects' plugin-loading code is adoptable as-is: their plugin
*shape* (compiled modules, or fully sandboxed subprocess plugins) doesn't
match this feature's shape (a declarative TOML document parsed into a fixed,
narrow schema — `id` + `binary_names` + `[[patterns]]` — with no behavior
beyond regex matching). The herdr-style pattern this requirements doc already
cites as its model (TOML manifest, no scripting) is closer in spirit to
existing prior art in this codebase's own git history/roadmap references than
to any of Caddy/Vector/Grafana's plugin systems.

The actual custom surface is small and domain-specific enough that adapting
someone else's loader wouldn't save meaningful effort over building directly:

- Parse TOML → typed struct (few dozen lines with go-toml v2 + struct tags).
- Validate: required fields non-empty, regexes compile, `status` string maps
  to a known `StatusPatterns` field, `id`/`binary_names` uniqueness against
  other *user* plugins (~50–80 lines).
- Construct a `dtypes.BinaryDetector`-satisfying value from the validated
  struct, feeding `PatternSet` exactly as built-ins do (~30–40 lines, mostly
  a `status → StatusPatterns` field switch).
- fsnotify watch loop on the directory, debounced re-scan on write/create/
  remove/rename events, log-and-skip per file (~60–80 lines, closely mirroring
  `session/unfinished/watcher.go`'s existing structure).
- Merge step in `registry.go`: iterate loaded user detectors, register after
  built-ins so `DetectorRegistry.Register`'s last-write-wins (confirm this
  semantics in `registry.go` during implementation) gives user plugins
  override precedence by binary name.

Total estimate: ~200–300 lines across `plugins.go` (new) + a small edit to
`registry.go`, consistent with the requirements doc's own files-touched list.

**Verdict: Not recommended to fork/adapt anything — build directly on
go-toml v2 + stdlib regexp + fsnotify.** No candidate project's plugin
mechanism matches this feature's shape closely enough to save work; adapting
any of them would mean stripping away more machinery than it would save.

## Summary Table

| Option | Pros | Cons | Verdict |
|---|---|---|---|
| Viper / koanf as the loader | Built-in TOML parsing + file-watch reload | Wrong domain shape (N-source merge into 1 tree vs. N independently-validated plugin docs); Viper lowercases keys (TOML-spec-breaking); doesn't remove any custom code that must be written anyway | Not recommended |
| SaaS/managed API | — | Not applicable — local file feature, no external service, explicitly out of scope | N/A |
| `pelletier/go-toml/v2` for parsing | Modern, fast, maintained, `encoding/json`-shaped API | New direct dependency (small cost) | **Recommended** |
| `BurntSushi/toml` for parsing | Simple, historically the default | Effectively unmaintained in 2026; community migrating away | Viable but not preferred |
| stdlib `regexp` (RE2) for patterns | Already used in `pattern_set.go`; linear-time guarantee is exactly the ReDoS protection untrusted plugin regexes need; zero new code to compile plugin patterns | None material | **Recommended** |
| `fsnotify/fsnotify` for hot-reload | Already a direct dependency at current version; repo has 12+ existing usages/idioms to follow; solves debounce/rename edge cases already | None material | **Recommended** |
| Hand-rolled poll loop for hot-reload | No new dependency (already zero, since fsnotify is already a dep) | Reinvents debouncing/atomic-write handling fsnotify already solves; strictly more code for less capability | Not recommended |
| Fork/adapt Caddy / Vector.dev / Grafana plugin systems | Might shortcut design work | Shape mismatch — compiled-module or full process-isolated plugin architectures, not declarative-TOML-only; would import unneeded machinery (RPC, process supervision) explicitly excluded by this project's non-goals | Not recommended |
| Build the ~200–300 line loader directly | Matches exact domain shape (BinaryDetector construction, StatusPatterns mapping, per-file isolation); reuses existing `PatternSet`/`fsnotify` idioms already proven in this repo | Is genuinely new code to write and test | **Recommended** |

## Overall Recommendation

Build the loader directly. Add one new dependency
(`github.com/pelletier/go-toml/v2`) for TOML parsing; reuse the two
dependencies already in `go.mod` that fit perfectly
(`fsnotify/fsnotify` for hot-reload, stdlib `regexp` via the existing
`PatternSet` for pattern compilation). No config-aggregation framework
(Viper/koanf) and no existing plugin-system project (Caddy/Vector/Grafana)
matches this feature's shape closely enough to adopt or fork — the
domain-specific 200–300 lines (TOML struct → validation → `BinaryDetector`
construction → registry merge → fsnotify watch loop) is the actual
deliverable regardless of which supporting libraries are chosen underneath it.
