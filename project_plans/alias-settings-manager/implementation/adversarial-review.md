# Adversarial Review: alias-settings-manager
**Date**: 2026-06-21
**Verdict**: CONCERNS

## Blockers

_(None)_

---

## Concerns

- [ ] **No Jest/RTL unit tests for AliasesManager.tsx** — Story 2.2.2 only runs `make quick-check` and checks existing tests aren't broken; `alias-manager.json` in the registry explicitly marks `"tested": false`. The feature-testing-registry rule (`rules/feature-testing-registry.md`) requires every new UI feature to have at least one Jest or Playwright test before the registry entry marks it tested. The acceptance criteria in Phase 2 are all stated as manual observations ("when the form renders, then…") with no Jest test function backing them. This leaves the component entirely unverified by automation, and the coverage gap count will increase. Recommendation: add a Jest/RTL test file (`AliasesManager.test.tsx`) covering at minimum: mount → list renders, Save with empty name shows inline error, successful save calls `upsertAlias`.

- [ ] **`window.confirm` is untestable and breaks accessibility** — `handleDelete` gates deletion on `window.confirm(...)` (Story 2.1.4a). This function is not available in jsdom (Jest will get `undefined` back unless mocked globally), blocks the browser's main thread, and cannot be styled or keyboard-navigated per WCAG. Every other destructive action in Settings uses inline confirmation (e.g., a "Confirm Delete?" state toggle on the row). Using `window.confirm` here will cause CI lint/a11y failures if Axe or Lighthouse runs on the settings page, and makes `handleDelete` untestable without a global mock. Recommendation: replace with an inline row confirmation state (`deletingName: string | null`) matching the pattern in `DirectoryRulesManager`.

- [ ] **No e2e test story** — The feature-registry rule requires every new user-facing feature to have at least one Playwright e2e test in `tests/e2e/`. No story, task, or file is listed in the plan for `tests/e2e/alias-settings.spec.ts`. The `alias-manager.json` registry entry correctly marks `"tested": false`, but the plan has no path to flipping that to `true`. Without this, `coverage-gaps.json` grows by one entry, violating the CI gate in Story 3.1.2 acceptance criteria ("coverage gap count has not grown"). Recommendation: add Story 2.3.3 to create a minimal Playwright spec covering the create → edit → delete cycle.

- [ ] **Client-side uniqueness check is case-insensitive on check but case-sensitive on storage** — `handleSave` compares `a.name.toLowerCase() === trimmedName.toLowerCase()` for collision detection, but the Go slice-scan upsert matches on `a.Name == alias.Name` (exact, case-sensitive). A user can create `"MyProj"` then create `"myproj"` — the client blocks it, but if the omnibar invokes `@myproj` vs `@MyProj` the two are treated as separate entries in `config.json`. The inconsistency also means editing `"MyProj"` while in create mode would be blocked by the client's case-insensitive guard. Recommendation: either make the Go upsert case-insensitive (normalize to lowercase on write), or make the client check exact-case only, documenting that alias names are case-sensitive.

- [ ] **Race condition when form is open during a concurrent config write** — The plan explicitly acknowledges "last-write-wins" and defers a package-level mutex to a follow-up. However, `handleSave` does load → mutate → save with no check that the loaded state is still current. If the user has the form open in two browser tabs, or if the omnibar creates a session that triggers a config write concurrently, one write silently clobbers the other. This is called out in requirements as a known risk but the plan's only mitigation ("document in a code comment") is invisible to the user. Recommendation: at minimum, `handleSave` should refresh `loadAliases` before building the envVarsMap and performing the RPC, so the client is sending a full snapshot of the latest state; this won't eliminate the race but reduces the window dramatically.

- [ ] **`as unknown as AliasProto` cast suppresses type safety on the RPC payload** — Story 2.1.3 builds the upsert payload as a plain object literal and casts it with `as unknown as AliasProto`. This bypasses TypeScript's structural check that all required proto fields are present, meaning a field rename in the proto would silently compile but fail at runtime. The `ProfilesManager` equivalent uses the generated proto constructor or a properly typed partial. Recommendation: use `create(UpsertAliasRequestSchema, { alias: { ... } })` (buf-connect pattern) or at minimum `{ alias: { name: trimmedName, ... } } satisfies PartialMessage<UpsertAliasRequest>` to keep the compiler honest.

- [ ] **EnvVar key uniqueness is not enforced in the UI** — `handleSave` silently drops duplicate env var keys (the `for` loop last-write-wins into `envVarsMap`). The form lets a user add two rows with `KEY=FOO` and `KEY=BAR`; only one survives the save. There is no warning, no deduplication UI, and the acceptance criteria don't cover this case. Recommendation: add a client-side check before save that flags duplicate keys with an inline error, or deduplicate with a visible warning.

---

## Minors

- `aliasNameRE = /^[\w-]+$/` in Go uses `\w` which matches Unicode word characters in some regex engines but Go's `regexp` package uses RE2 semantics where `\w` is ASCII-only (`[0-9A-Za-z_]`). The frontend `ALIAS_NAME_RE` also uses `\w`. Both will behave consistently (ASCII-only), but the doc comment says "letters, digits, hyphens, underscores only" — which is accurate for ASCII. Worth noting that non-ASCII alias names are silently rejected with no localization-friendly message.

- `setTimeout(() => setSuccess(null), 3000)` in `handleSave` and `handleDelete` creates a timer that fires after component unmount if the user navigates away, calling `setState` on an unmounted component. React 18 no longer throws on this but it is still a leaked timer. Use a `useEffect` cleanup ref or `useTimeout` hook pattern to cancel on unmount.

- `profile` field in the form uses a plain `<input>` (Story 2.1.5b note: "for simplicity, use a plain text `<input>` for `profile`"). This is fine short-term, but if a user types a profile name that doesn't exist, the alias silently references a non-existent profile. No validation or dropdown hint is planned. Minor UX gap; acceptable for the stated appetite.

- `formCard` is placed at the bottom of the list (bottom-anchored), but on a long alias list the form will be scrolled off-screen. `ProfilesManager` has the same behavior. Minor; not a blocker.

- Registry entry `docs/registry/features/backend/alias/list.json` retroactively registers the pre-existing `ListAliases` RPC with `"tested": false`. This increases `coverage-gaps.json` by one extra entry beyond the new feature's own gap, making the net coverage delta +2 (list + manage) rather than +1. The plan's CI acceptance criteria say "coverage gap count has not grown" — this will fail unless `ListAliases` also gets a test in Story 1.2.4. Recommendation: either add a `TestListAliases_*` test in Story 1.2.4 and flip `list.json` to `"tested": true`, or omit `list.json` from this PR entirely since `ListAliases` is pre-existing.
