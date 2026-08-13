# Feature Testing Registry

This project has two complementary frontend registries for omnibar capabilities. Every new feature that touches the omnibar must be registered in the relevant one(s) and must have corresponding tests.

---

## Registry 1: OmnibarAction Discriminated Union

**Files**: `web-app/src/lib/omnibar/actions/`

### How it works

`types.ts` defines a static discriminated union of every action. `dispatch.ts` routes each via an exhaustive `switch` — missing a case is a **compile error**, the architectural guard against silent omissions.

**Special case: `create_session` with `sessionType: "one_off"`** — One-off sessions use `oneOff: true` flag rather than a new action type. The dispatch case maps `sessionType === "one_off"` to `{ oneOff: true, sessionType: undefined }`.

### When to add a new action type

Add to the `OmnibarAction` union when:
- There is a new user-triggerable omnibar operation that has no existing action type to extend
- The operation has distinct payload fields (not just a flag on `create_session`)

Do NOT add a new action type for:
- Variations of existing operations (use a flag/field on the existing type instead)
- Session creation modes (use `sessionType` string or `oneOff`/similar bool flags)

### Test pattern

Every action type must have a `describe` block in `dispatch.test.ts`. Test name convention: `dispatchOmnibarAction_should_<effect>_When_<action>`

### Registration checklist for a new OmnibarAction

- [ ] Add variant to `OmnibarAction` union in `types.ts`
- [ ] Add `case "<type>":` to `dispatch.ts` switch
- [ ] Add `describe("<type>")` block with ≥1 test in `dispatch.test.ts`
- [ ] Run `cd web-app && npx jest --no-coverage --testPathPatterns="dispatch.test"` to verify

---

## Registry 2: DetectorRegistry

**Files**: `web-app/src/lib/omnibar/detector.ts`, `detector.test.ts`

### How it works

`DetectorRegistry` holds a priority-sorted list of `Detector` implementations. Each detector returns a `DetectionResult` or `null`; first match wins. `createDefaultRegistry()` is the single authoritative list, in priority order:

| Priority | Detector | Matches | Registered |
|---|---|---|---|
| 5 | `CommandDetector` | `>command` VS Code-style prefix | `createDefaultRegistry()` |
| 10 | `GitHubPRDetector` | `https://github.com/.../pull/N` | `createDefaultRegistry()` |
| 20 | `GitHubBranchDetector` | `https://github.com/.../tree/branch` | `createDefaultRegistry()` |
| 25 | `WorkflowDetector` | `@workflow-slug` | dynamic — `OmnibarContext.tsx` effect |
| 30 | `GitHubRepoDetector` | `https://github.com/owner/repo` | `createDefaultRegistry()` |
| 35 | `NewSessionDetector` | `new:<path>` shorthand | `createDefaultRegistry()` |
| 36 | `AliasDetector` | `@alias-name` | dynamic — `OmnibarContext.tsx` effect |
| 40 | `GitHubShorthandDetector` | `owner/repo` shorthand | `createDefaultRegistry()` |
| 50 | `PathWithBranchDetector` | `/path:branch` | `createDefaultRegistry()` |
| 100 | `LocalPathDetector` | `/absolute/path` or `~/path` | `createDefaultRegistry()` |
| 200 | `SessionSearchDetector` | everything else (search fallback) | `createDefaultRegistry()` |

**Dynamic detectors** (`WorkflowDetector`, `AliasDetector`) require runtime-fetched data and are registered/unregistered in `OmnibarContext.tsx` effects, NOT in `createDefaultRegistry()`. Add a data-driven detector there, not in `detector.ts`.

### When to add a new detector

Add a new `Detector` class when:
- A new input pattern should trigger a distinct behavior (e.g., a new URL scheme, a shorthand syntax)
- The pattern is not covered by any existing detector
- The detection logic is non-trivial and deserves isolation

### How to add a detector

1. Implement `Detector` interface in `detector.ts` (`name`, `priority`, `detect`)
2. Register in `createDefaultRegistry()`
3. Add tests in `detector.test.ts` — naming: `DetectorName_should_<effect>_When_<condition>` with test IDs `T-UNIT-TS-NNN` (unit) / `T-PITFALL-NNN` (pitfall guards)

### Registration checklist for a new Detector

- [ ] Class implements `Detector` interface (name, priority, detect)
- [ ] Registered in `createDefaultRegistry()` at the correct priority
- [ ] Tests cover: positive match, negative (returns null), edge cases
- [ ] Test IDs assigned (`T-UNIT-TS-NNN`)
- [ ] Run `cd web-app && npx jest --no-coverage --testPathPatterns="detector.test"` to verify

---

## Decision tree: which registry does a new feature need?

```
New omnibar feature
        │
        ├─ Triggers via a user action (navigate, create, pause, etc.)?
        │         └── YES → OmnibarAction union + dispatch case + dispatch test
        │
        ├─ Auto-detects a new input pattern (URL, shorthand, etc.)?
        │         └── YES → New Detector class + createDefaultRegistry() + detector test
        │
        ├─ New session creation mode?
        │         └── YES → also see .claude/rules/session-creation-registry.md (7 touchpoints)
        │
        └─ None of the above?
                  └── May only need changes to OmnibarCreationPanel + Omnibar form state
```

