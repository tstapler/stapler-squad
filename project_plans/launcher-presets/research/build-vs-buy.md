# Build vs. Buy: Launcher Presets

Research question: should the launcher-presets config loader (JSON schema + validation +
optional live reload) be built from scratch or sourced from an existing library?

## 1. Existing OSS library / framework

**JSON schema validation** — `github.com/xeipuuv/gojsonschema v1.2.0` is already a direct
dependency (`go.mod:38`) and is used exactly once in the codebase, in
`config/claude.go:250-253` (`ClaudeConfigManager.ValidateJSON`), to validate Claude's own
`settings.json` against a large embedded JSON-Schema string (`settingsJSONSchema`). That is a
case where a formal schema earns its keep: the schema is externally defined (mirrors Claude
Code's actual settings format), large, and nested.

The launcher-presets schema is the opposite shape: `{ version, presets: [{ id, label, argv,
program?, default_path? }] }` — five flat fields, three of them required. Every other config
loader in this repo (`config/config.go:847` `LoadConfigFromPath`, `config/state.go:167`
`LoadState`) validates by hand: `encoding/json.Unmarshal` into a struct, then a handful of
explicit `if` checks (non-empty required fields, uniqueness), and neither uses gojsonschema.

**File watching** — `github.com/fsnotify/fsnotify v1.9.0` is already a direct dependency
(`go.mod:20`, confirmed exact version in `go.sum:128-129`) and already has a working reference
implementation for exactly this "watch a config file, reload without restart, fall back
gracefully" scenario: `session/unfinished/watcher.go`. Its `NewWatchDirWatcher`
(`watcher.go:22-36`) tries `fsnotify.NewWatcher()` and falls back to nil (later handled by a
`periodicReWalk` poll) if fsnotify itself fails to initialize — a pattern worth copying
directly for the Nice-to-Have live-reload requirement rather than reinventing.

**Preset/profile launch-config libraries** — no existing Go library was found (or is already
vendored) that models "named launch profile → program + argv + cwd." This is a narrow,
project-specific concept; nothing off-the-shelf models it, and pulling in a general-purpose
config/profile framework (e.g. something built around Viper's precedence-layering model) would
solve problems this feature doesn't have (env-var overrides, multi-format support, remote
config sources).

**Verdict: plain `encoding/json` + hand-written validation is sufficient. Do not add
gojsonschema to this feature's code path** — Recommended: reuse it only if Phase 3 explicitly
wants a formal JSON-Schema artifact for other reasons (e.g. IDE autocomplete for hand-edited
preset files); otherwise it's unjustified ceremony for a 5-field schema. Reuse `fsnotify`
directly, mirroring `session/unfinished/watcher.go`'s try-then-fallback pattern, if live reload
is confirmed as in-scope. Recommended.

## 2. SaaS / managed API

Not applicable. Launcher presets are a local, user-authored JSON file
(`~/.stapler-squad/launcher-presets.json`) read by the same process that serves the Omnibar —
there is no remote service boundary to outsource to, no multi-tenant config-management need,
and no case for a hosted config/feature-flag SaaS (LaunchDarkly-style) for a single-user,
single-machine preference file. Not recommended / not applicable.

## 3. LLM-generated implementation vs. battle-tested library

Confirmed existing convention: `config/config.go` and `config/state.go` (the two closest
analogues — both load/save JSON config from `~/.stapler-squad/`) both hand-roll loading via
`os.ReadFile` + `json.Unmarshal` (`config/config.go:847-856`, `config/state.go` `LoadState`)
and hand-roll saving via `json.MarshalIndent` + write-to-temp + atomic rename
(`config/config.go:806-838`). Neither uses Viper or Koanf. `go.mod` confirms neither is a
direct dependency — `github.com/go-viper/mapstructure/v2` and `github.com/spf13/{cast,pflag}`
present are transitive dependencies of `spf13/cobra` (the CLI flag library), not of Viper
itself; there is no actual config-management framework in this codebase to match against.

For this feature's actual surface area — parse a small JSON file, unmarshal into a `Presets`
struct, check `len(argv) > 0` and `id` uniqueness by hand, reject the whole file on any error
— hand-written Go carries negligible correctness risk: this is exactly the same shape of code
already proven correct in `config/config.go`/`config/state.go`, and it keeps the feature
consistent with the rest of the config package (a reviewer already knows this pattern). Pulling
in Viper or Koanf here would mean:
- A new direct dependency solving problems this feature doesn't have (multi-format, env
  overlays, remote backends, live sub-key watching abstractions).
- Inconsistency with every other loader in `config/`, raising future-maintenance cost (two
  config idioms in one package).

The one piece worth reusing as a library, not hand-rolling, is `fsnotify` for live reload —
but that's already a dependency and already has a proven wrapper pattern in this codebase
(`session/unfinished/watcher.go`), so "reuse the existing pattern" is the recommendation, not
"introduce a new abstraction."

**Verdict: hand-written `encoding/json` + manual validation, matching `config/config.go` and
`config/state.go` — Recommended.** Do not add Viper/Koanf. Do not add gojsonschema to this
feature. Do reuse `fsnotify` via the same try-then-fallback wrapper shape as
`session/unfinished/watcher.go` if/when live reload ships.

## 4. Fork or adapt

`herdr-web`'s Rust implementation (`bridge/src/launcher_presets.rs`, referenced in
`project_plans/launcher-presets/requirements.md:16-19`) is not available in this environment —
it's a different repo, not checked out locally, and no fetch was attempted (a different
language/stack per the task framing, so this is design-reference-only territory, not a porting
candidate). The requirements doc already extracted the one design idea worth carrying over
conceptually: presets store `argv` as an explicit string array rather than a shell string,
specifically to avoid quoting ambiguity for paths with spaces and nested-quote remote-exec
commands (`ssh -t host '...'`) — that constraint is already captured in this project's
requirements (`requirements.md:47-48`) and should be preserved in the Go schema (`argv
[]string`, never a single command string that gets shell-split later).

**Verdict: no fork/port needed — the one transferable design decision (argv-as-array) is
already captured in requirements; everything else (JSON shape, validation errors, Go
idioms) should follow this repo's own `config/` conventions rather than mirror Rust
structures.** Viable (as a conceptual reference only, already absorbed into requirements).

## Summary Table

| Option | Verdict |
|---|---|
| gojsonschema for preset validation | Not recommended (already in go.mod, but wrong tool for a 5-field schema; reserve for `config/claude.go`'s existing large-schema use case) |
| fsnotify for live reload | Recommended (already a dependency; reuse `session/unfinished/watcher.go`'s pattern) |
| SaaS/managed config service | Not applicable (local single-user file, no remote boundary) |
| Hand-written `encoding/json` + manual validation | Recommended (matches `config/config.go`/`config/state.go` convention exactly) |
| Viper / Koanf | Not recommended (not already a dependency; solves problems this feature doesn't have; would fragment `config/` package conventions) |
| Fork/port herdr-web's Rust implementation | Not recommended as code; Viable as design reference only — argv-as-array decision already captured in requirements.md |
