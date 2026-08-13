# Architecture Review: alias-settings-manager
**Date**: 2026-06-21
**Verdict**: CONCERNS (0 blockers, 5 concerns, 4 nitpicks)

## Constitution Violations

No `docs/adr/ADR-000-architecture-constitution.md` exists in this repository. Skipping this section.

---

## Blockers

None.

---

## Concerns

- [ ] **Story 2.1.3 (handleSave)** — `as unknown as AliasProto` type cast is unsound and will silently diverge if the proto schema changes. The plan constructs a plain object literal, then force-casts it to `AliasProto` (which is `Message<"session.v1.AliasProto"> & {...}`) — this is the same pattern used in `ProfilesManager` (`as unknown as ProfileDefaultsProto`), but it bypasses the `Message<>` brand entirely. The generated proto library exposes `create(AliasProtoSchema, { ... })` for exactly this purpose; use it instead. Remediation: `import { create } from "@bufbuild/protobuf"` and `create(AliasProtoSchema, { name: trimmedName, ... })` — this is type-safe and inherits correct default values for missing fields.

- [ ] **Story 1.2.1a — regex scope / DRY** — `aliasNameRE` is declared in `defaults_service.go` (Go backend) and `ALIAS_NAME_RE` is re-declared in `AliasesManager.tsx` (TypeScript frontend). Two independent regex definitions for the same validation rule will inevitably drift. The pattern already exists as a comment in `config.AliasConfig` (`// Name must match ^[\w-]+$`). Remediation: add a `// +api:` or doc comment co-locating both definitions with the constraint source in `config/config.go` and add a code comment in `AliasesManager.tsx` referencing the backend source. A test asserting the TS regex matches the Go regex (against a shared fixture table) would make divergence detectable at CI time.

- [ ] **Story 2.1.4a — `window.confirm` blocks the event loop and is untestable in Jest/jsdom** — `handleDelete` calls `window.confirm(...)` synchronously. `jsdom` returns `false` for `window.confirm` by default, which means any Jest test for `handleDelete` must explicitly stub `window.confirm` or the guard always suppresses the call. This is not mentioned in the test plan (Story 2.1.4a acceptance criteria assume `window.confirm` returns `true`/`false` without noting the stub requirement). `ProfilesManager` has the same pattern, but the plan adds no existing Jest tests for it either — extending the same known gap here is acceptable only if explicitly documented. Remediation: document the `jest.spyOn(window, 'confirm').mockReturnValue(true)` requirement in the test plan, or refactor to a callback prop / context-based confirmation pattern consistent with the "no modal" decision.

- [ ] **Story 1.2.4 — `defaults_service_test.go` file path and config injection gap** — The plan states "new file if it does not exist; otherwise extend it" but `server/services/defaults_service_test.go` does not currently exist in the repo (`defaults_service.go` is 361 lines with no accompanying test file). The test plan's acceptance criteria rely on `STAPLER_SQUAD_TEST_DIR` env injection for config isolation (documented as the established pattern in memory), but do not specify how to wire up a `DefaultsService` instance against a temp config directory. The plan silently assumes this pattern is self-evident from "other defaults tests," but there are none — `UpsertProfile` has no test file. A new implementer will lack the reference. Remediation: explicitly show the test setup scaffolding (temp dir creation, env var injection, `NewDefaultsService()` construction) in the task body, not just in the acceptance criteria.

- [ ] **Story 2.3.1 — Registry JSON schema mismatch** — The plan's proposed registry entries (`upsert.json`, `delete.json`, `list.json`) use a flat structure with top-level `service`, `method`, `protoFile`, `markerFound` fields. The existing schema (`docs/registry/schema.json`) requires a `backend` nested object for those fields ("`backend`: `{$ref: '#/definitions/BackendDetails'}`"). The existing `profile/upsert.json` entry uses the same flat structure, meaning either the schema or the actual files are out of sync — but plan entries should match the actual working format (flat), not the schema definition. The concern is that `make registry-generate` may overwrite or reject the hand-authored flat entries. Remediation: verify the flat format is what `make registry-generate` produces (compare against `profile/upsert.json`), and confirm that the plan's `markerFound: false` entries will survive a re-generate without being reset. Also: `markerFound` should be `true` for the two new RPCs since the plan should include `// +api: alias:upsert` and `// +api: alias:delete` markers in the handler files — no other `DefaultsService` method has such markers, but the plan calls out `markerFound: false` without explaining the omission relative to the `session_service.go` pattern where `// +api:` markers are present for all session RPCs.

---

## Nitpicks

- **Story 2.1.2 — `clientRef` initialization**: The plan initializes `clientRef` inside `useEffect` with `loadAliases` as a dependency. If `loadAliases` identity changes (e.g., on a re-render), the effect re-runs and recreates the transport. The `ProfilesManager` has the same pattern. Consider wrapping the client creation in a separate effect with `[]` deps to prevent unnecessary transport teardown — though this is a micro-optimization at this scale.

- **Story 2.1.5a — `aliasRow` naming**: The CSS class `profileRow` is renamed `aliasRow` in the plan, but the JSX specifies `profileInfo`/`profileActions` as well. Ensure the renaming is complete in both the `.css.ts` and the JSX so that no stale `profile*` class names leak into `AliasesManager` (IDE-discoverable but a readability nit).

- **Story 2.2.1 — section ordering**: The plan places Aliases below Directory Rules. The requirements say "alongside `ProfilesManager` and `DirectoryRulesManager`" without mandating order. Confirm the intended tab order (alphabetically: Aliases, Directory Rules, Profiles) is what the team wants, since the plan's ordering (Directory Rules first) differs.

- **Unresolved question — `alias:list` registry entry `tested: false`**: `list.json` is created with `tested: false` and empty `testIds`. This means `make registry-aggregate` will add it to `coverage-gaps.json`, growing the gap count. Story 3.1.2 acceptance criteria require "coverage gap count has not grown" — these two constraints contradict. Either add a `ListAliases` test (simplest: one passing call in the same test file), or explicitly mark the story as a known gap with justification in the PR description.
