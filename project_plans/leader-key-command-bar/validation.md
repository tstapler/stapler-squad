# Validation: Leader-Key Command Bar

## Go/No-Go Assessment

**Verdict: GO — with three preconditions**

The plan is sound. The codebase already has all the primitives needed:
- `ShortcutRegistry` singleton with a single dispatch loop
- `useWindowShortcuts` proves the leader pattern works and its disarm lifecycle is correct
- `useFocusTrap` handles modal focus management
- vanilla-extract CSS architecture supports the command bar styling

Three items must be resolved before implementation starts:

1. **Byte probe (AC-1):** The default leader chord cannot be hardcoded until a human runs the browser/OS matrix test. T2 should use a clearly-marked placeholder (`TODO: replace after byte probe`) and the chord must be configurable from day one.

2. **`w` branch / `useWindowShortcuts` coexistence decision:** The plan leaves open whether `Alt+W` stays as a standalone leader or becomes an alias. This must be decided in T4 to avoid the P1-3 duplicate-registration bug. **Recommended:** keep `Alt+W` as a direct shortcut that opens the command bar pre-navigated to the `w` branch (not a second independent leader). This simplifies the leader lifecycle and removes the P1-3 risk entirely.

3. **Reply actions registration strategy (P2-4):** Lazy registration on `n`-branch navigation (not at mount) must be implemented in T5, not deferred.

## AC Coverage Check

| AC | Covered in plan? | Task(s) |
|---|---|---|
| AC-1: Chord probe + configurable default | Partially (T2 + T10; probe is human spike) | T2, T10 |
| AC-2: Leader arm/disarm safety | Yes | T3 |
| AC-3: Which-key mode | Yes | T3, T6 |
| AC-4: Fuzzy palette | Yes | T6, T7 |
| AC-5: Registry integration | Yes | T1, T4, T5 |
| AC-6: Terminal safety | Yes | T3 (capture phase) + T11 test 5 |
| AC-7: Accessibility | Yes | T6 (dialog, focus trap, reduced-motion, 44px) |
| AC-8: Playwright coverage (5 tests) | Yes | T11 |
| AC-9: No regression on existing shortcuts | Yes | T12 |

## Task Sizing Review

| Task | Estimate | Concern? |
|---|---|---|
| T1 (registry extend) | 1.5 h | None — backward-compatible optional fields |
| T2 (leader config) | 0.5 h | None |
| T3 (useLeaderMode) | 2 h | The capture-phase/disarm complexity is real; 2 h may be tight — buffer to 3 h |
| T4 (fold windowShortcuts) | 1 h | Depends on coexistence decision above |
| T5 (notif actions) | 1 h | Lazy registration adds complexity; buffer to 1.5 h |
| T6 (CommandBar) | 3 h | Virtual list + a11y; 3 h is tight — buffer to 4 h |
| T7 (fuzzy filter) | 1 h | None |
| T8 (wire into app) | 0.5 h | None |
| T9 (settings UI) | 1 h | None |
| T10 (probe docs) | 0.5 h | Human spike — can block shipping |
| T11 (Playwright) | 2 h | 5 tests; PTY spy test is novel — buffer to 2.5 h |
| T12 (unit tests) | 1 h | None |
| **Total** | **~18 h** | With buffers |

## Dependencies on external PRs / features

- None. This feature is self-contained to `web-app/`.

## Rollout / feature flag recommendation

Wrap `useLeaderMode` under a feature flag (`NEXT_PUBLIC_LEADER_KEY_ENABLED`) so it can be shipped dark and tested without affecting users. Remove the flag once AC-1 (probe) is complete and the default chord is confirmed safe.

## Documentation

- `docs/reference/leader-key-chord-probe.md` (T10) — mandatory before GA.
- Update `docs/reference/feature-testing-registry.md` with the new command-bar OmnibarAction (if added to the union).
- Update `KeyboardShortcutsTab.tsx` so the rendered shortcuts table shows sequence bindings.
