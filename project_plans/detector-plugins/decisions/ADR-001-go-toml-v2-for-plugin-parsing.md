# ADR-001: `github.com/pelletier/go-toml/v2` for Detector Plugin Parsing

## Status
Accepted — 2026-08-01

## Context

The detector-plugins feature requires parsing user-authored TOML files from
`~/.stapler-squad/detectors/*.toml` into a Go struct. The repo has **no TOML
library today** — `grep -iE "toml|viper" go.mod go.sum` returns only
`github.com/go-viper/mapstructure/v2`, an indirect transitive dependency
(Viper itself is not in `go.mod`). Existing structured-config parsing uses
`gopkg.in/yaml.v3` (see the `yaml:"..."` struct tags on
`session/detection/dtypes/dtypes.go:7-26`) and plain `encoding/json` for
`config.json` / `sessions.json`.

So this is a genuinely new direct dependency, and `deps_guard_test.go` shows
the repo takes dependency-graph hygiene seriously (it has a hard test guard
against reintroducing `github.com/shoenig/go-m1cpu` and
`github.com/shirou/gopsutil/v3`).

The requirements fix TOML as the format (`requirements.md` §Out of Scope:
"TOML only, per the herdr pattern this item adopts"), so "just use YAML like
the rest of the repo" is not an option on the table.

## Options Considered

| Option | Maintained | Unknown-field detection | Perf | Verdict |
|---|---|---|---|---|
| `github.com/pelletier/go-toml/v2` | Yes, actively | `Decoder.DisallowUnknownFields()` — one flag | 1.7x–5.1x faster than BurntSushi on unmarshal | **Chosen** |
| `github.com/BurntSushi/toml` | Effectively unmaintained | Opt-in `MetaData.Undecoded()` — caller must write the check | Baseline | Rejected |
| `github.com/spf13/viper` | Yes | N/A (merges sources) | N/A | Rejected |
| `github.com/knadh/koanf` | Yes | N/A (merges sources) | N/A | Rejected |
| Hand-rolled TOML subset parser | — | — | — | Rejected |

## Decision

Add `github.com/pelletier/go-toml/v2` as a direct dependency in `go.mod`, and
decode plugin files with a `toml.Decoder` configured with
`DisallowUnknownFields()`.

### Why go-toml v2 over BurntSushi

- BurntSushi/toml is effectively unmaintained; go-toml's own documentation
  points new consumers at v2.
- **`DisallowUnknownFields()` is the deciding factor**, not speed. Users will
  typo `binary_name` for `binary_names` and `staus` for `status`. With
  BurntSushi those are silent no-ops that produce a detector that loads
  cleanly and matches nothing — the single worst failure mode for this
  feature (see ADR-004 and `research/pitfalls.md` item 5). go-toml v2 turns
  them into a loud load error with one decoder flag.
- Speed is a secondary benefit but not irrelevant: hot-reload means
  re-parsing the whole directory on every settled filesystem event, not once
  at startup.
- go-toml v2 supports TOML 1.0; the schema here (strings, string arrays,
  array-of-tables) uses nothing near the spec edges.

### Why not Viper / koanf

Both are **N-source-merge-into-one-config-tree** frameworks. This feature is
the inverse shape: **N independently-validated documents each producing a
distinct domain object with per-file error isolation** (requirement #3
mandates that one bad file must not sink the others — a merge framework
structurally cannot express that). Viper additionally force-lowercases keys,
which would silently break case-sensitive `binary_names` values. Both would
also import large amounts of machinery (env-var binding, remote KV backends,
their own file watching) this feature explicitly does not want.

### Why not hand-rolled

TOML has enough surface (multiline strings, escapes, array-of-tables,
comments) that a subset parser would diverge from what users' editors and
linters tell them is valid TOML. Not worth ~400 lines to avoid one
well-maintained dependency.

## Consequences

- One new direct `require` line in `go.mod`. No conflicting version is
  already pinned in the module graph, so no minimum-version-selection
  surprises.
- `deps_guard_test.go`'s `forbiddenDeps` list is **not** modified — go-toml
  is a wanted dependency, not a banned one. No change to that file is part of
  this work.
- License: MIT — compatible with the repo's existing dependency set.
- Security surface: a pure-Go text parser with no cgo, no network, no
  subprocess. Its blast radius on malformed input is a parse error, which is
  exactly what the loader wants.
- If go-toml v2 is ever abandoned, the swap surface is confined to one
  function (`parsePluginFile` in `session/detection/plugins.go`) because the
  TOML struct is a DTO that is immediately converted to
  `dtypes.StatusPatterns` (see ADR-002).
