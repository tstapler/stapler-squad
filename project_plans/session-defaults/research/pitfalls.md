# Pitfalls Research: Session Defaults

## Config Migration Risks

**Current state:** No versioning system in `config.go`. `json.Unmarshal` silently drops unknown/missing fields. Post-load nil checks are done ad-hoc (e.g., `KeyCategories` nil check at `config.go:428-430`, `CategoryExpanded` nil check in `state.go:232-236`).

**Risks:**
1. Old config.json lacks `SessionDefaults` → field is zero-value empty struct, not nil. Must initialize sub-collections (maps, slices) in `LoadConfig()` or callers will nil-panic.
2. Nested collection fields (`Profiles map[string]Profile`, `DirectoryDefaults []DirectoryDefaults`) are `nil`, not empty. Every caller must nil-check before ranging.
3. No `config_version` field → no migration path for future restructuring. Silent data loss if a field is renamed.
4. Concurrent feature branches adding config fields → whichever lands first initializes defaults; the other's never apply retroactively to existing configs.

**Mitigation:** Add `config_version: 1` field. Wrap `LoadConfig()` to initialize all new collections after unmarshal (match the `KeyCategories` pattern). Mark new fields `omitempty`.

---

## Precedence Edge Cases

**Stale profile references:**
- User deletes profile "Work" after sessions reference it.
- `GetSessionDefaults("Work")` returns nil → silently falls through to global. No visible error.
- Fix: validate on delete (list affected sessions), or cascade-clear profile references.

**Ambiguous directory matching:**
- Monorepo: `/workspace/frontend` and `/workspace/frontend/src` both registered.
- Which wins when cwd is `/workspace/frontend/src`? Nearest? Longest path?
- Decision required: **longest matching prefix wins** (most specific). Must be documented and enforced consistently.

**Explicit-empty vs unset:**
- Global default: `program: "claude"`.
- Per-directory rule wants empty program (force user to choose).
- JSON `""` and `null`/absent are indistinguishable in a plain string field.
- Fix: use pointer fields (`*string`) or a sentinel value for explicit-clear, or document that empty string means "inherit".

**No defaults at all:**
- cwd matches nothing, global defaults empty → dialog shows blank program field.
- User can't tell if this is intentional or broken.
- Fix: show a subtle "no defaults configured" hint in the form.

---

## Per-Directory Detection Failure Modes

**cwd unavailable:** `os.Getwd()` fails (deleted symlink target) → silently falls back to global. No log, no UI indicator. Fix: log warning, surface fallback in UI.

**Symlink resolution mismatch:**
- `/home/user/projects/myrepo` (symlink) vs `/mnt/ssd/code/myrepo` (real path).
- Current code hashes the string as-is → different hash, different workspace, different defaults.
- Fix: call `filepath.EvalSymlinks()` before path comparison and hashing.

**Manual path entry in create dialog:**
- User types a custom path that differs from cwd.
- Per-directory detection uses `os.Getwd()`, ignoring the entered path.
- Fix: use the form's `path` field for directory detection, not server-side cwd.

**Absolute path staleness:**
- Stored as `{ "/Users/tyler/projects/web": {...} }`.
- User renames or moves directory → defaults silently stop matching.
- No cleanup or repair path.

**Workspace detection race:**
- Directory deleted between `os.Getwd()` and config read → workspace dir absent → falls to global without warning.

---

## UI / UX Pitfalls

**Hidden default origin:**
- Form shows `program: "claude"` but user can't see if it's from global, profile, or directory.
- "Save as default" then writes to an unknown scope.
- Fix: show per-field source badge (tooltip: "from global defaults"). "Save" dialog asks which scope.

**Editing pre-populated fields then saving:**
- User changes pre-populated tags from `["work"]` to `["personal"]`, clicks "Save as default".
- If it saves the entire merged result, it overwrites previously independent layers.
- Fix: save only the diff (fields the user explicitly edited) or clearly ask scope.

**Profile change clears user edits:**
- User partially fills form, then switches profile selector.
- Should un-edited fields update to new profile values? Should edited fields be preserved?
- Fix: track per-field `{ value, source, edited }` state; only re-populate unedited fields on profile change.

**Tags merge semantics undefined:**
- Global tags `["work"]` + profile tags `["urgent"]` → union `["work","urgent"]` or override `["urgent"]`?
- Must be decided and documented before implementation. Recommend: **union** for tags (additive), **override** for scalar fields (program, cli_flags).

**Invalid default combinations:**
- Global default includes empty required field (e.g., `API_KEY: ""`).
- Dialog pre-populates with invalid value; session creation fails with cryptic error.
- Fix: validate defaults at save-time; warn user at pre-population time if defaults produce invalid state.

**Cancel ambiguity:**
- User opens dialog, sees pre-populated fields, clicks Cancel.
- Next open shows same pre-populated fields — user mistakes defaults for remembered choices.
- Fix: subtle "Pre-filled from defaults" notice in the dialog header.

---

## Concurrency & Persistence Risks

**config.json has no file lock:**
- `state.go` uses `flock` for `state.json`, but `config.go` does not lock `config.json`.
- Two browser tabs saving settings simultaneously → last write wins, other changes silently lost.
- Fix: apply the `flock` pattern from `state.go` to `config.go`.

**No temp-file cleanup:**
- `state.go:290-299` cleans up abandoned `.tmp` files; `config.go` does not.
- Disk-full mid-write leaves orphaned temp files indefinitely.
- Fix: match `state.go` cleanup pattern; scan for orphaned `.tmp` files on startup.

**Stale defaults across browser tabs:**
- Tab A fetches defaults at dialog open; Tab B saves new defaults.
- Tab A creates session with stale defaults.
- Fix: fetch `ResolveDefaults` fresh each time the create-session dialog opens (not cached in component state across renders).

---

## Recommended Mitigations (Prioritized)

### Critical — before Phase 5 implementation

| # | Risk | Fix |
|---|---|---|
| 1 | No config versioning | Add `config_version: 1`; init collections in `LoadConfig()` |
| 2 | config.json unprotected | Apply `flock` pattern from `state.go` |
| 3 | Precedence algorithm unspecified | Document: longest-prefix for directory; union for tags; scalar override for program/flags |
| 4 | Symlink mismatch | `filepath.EvalSymlinks()` before all path comparisons |

### High priority — during Phase 5

| # | Risk | Fix |
|---|---|---|
| 5 | Default origin hidden | Per-field source tracking in React form state |
| 6 | Stale profile references | Cascade delete or UI warning on profile delete |
| 7 | cwd failure silent | Log + return `used_fallback: true` in `ResolveDefaultsResponse` |
| 8 | Tags merge undefined | Union for tags, override for scalars; env var: layer wins (highest precedence) |

### Medium priority — polish pass

| # | Risk | Fix |
|---|---|---|
| 9 | Temp file cleanup | Startup scan for `.tmp` orphans |
| 10 | Stale tab defaults | Fetch `ResolveDefaults` on each dialog open |
| 11 | Manual path ignored | Use form `path` field for directory detection, not server cwd |
| 12 | Invalid default combinations | Validate at save-time; warn at pre-populate time |
