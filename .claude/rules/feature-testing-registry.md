# Feature Testing Registry

This project has two complementary frontend registries for omnibar capabilities. Every new feature that touches the omnibar must be registered in the relevant one(s) and must have corresponding tests.

---

## Registry 1: OmnibarAction Discriminated Union

**Files**: `web-app/src/lib/omnibar/actions/`

### How it works

`types.ts` defines a static discriminated union of every action the omnibar can perform:

```typescript
export type OmnibarAction =
  | { type: "navigate_session"; sessionId: string; label: string }
  | { type: "create_session"; path: string; sessionType: string; ... }
  | { type: "clone_session"; ... }
  | { type: "pause_session"; sessionId: string; label: string }
  | { type: "resume_session"; sessionId: string; label: string }
  | { type: "delete_session"; sessionId: string; label: string };
```

`dispatch.ts` routes each action via an exhaustive `switch`. TypeScript's type system **prevents compilation** if a union variant has no case — this is the architectural guard against silent omissions.

```typescript
export function dispatchOmnibarAction(action: OmnibarAction, deps: ActionDeps): void {
  switch (action.type) {
    case "navigate_session": ... return;
    case "create_session": ...  return;
    // missing case → compile error ✅
  }
}
```

### Special case: `create_session` with `sessionType: "one_off"`

One-off sessions use a flag (`oneOff: true`) rather than a new action type. The `create_session` case in `dispatch.ts` maps `sessionType === "one_off"` to `{ oneOff: true, sessionType: undefined }`:

```typescript
case "create_session": {
  const isOneOff = action.sessionType === "one_off";
  void deps.createSession({
    ...
    sessionType: isOneOff ? undefined : action.sessionType as ...,
    oneOff: isOneOff,
  });
}
```

### When to add a new action type

Add to the `OmnibarAction` union when:
- There is a new user-triggerable omnibar operation that has no existing action type to extend
- The operation has distinct payload fields (not just a flag on `create_session`)

Do NOT add a new action type for:
- Variations of existing operations (use a flag/field on the existing type instead)
- Session creation modes (use `sessionType` string or `oneOff`/similar bool flags)

### Test pattern

Every action type must have a `describe` block in `dispatch.test.ts`. Test name convention:

```
dispatchOmnibarAction_should_<effect>_When_<action>
```

Example:
```typescript
describe("create_session (one-off)", () => {
  it("dispatchOmnibarAction_should_setOneOffTrue_When_sessionTypeIsOneOff", () => {
    ...
    expect(deps.createSession).toHaveBeenCalledWith(
      expect.objectContaining({ oneOff: true, sessionType: undefined })
    );
  });
});
```

### Registration checklist for a new OmnibarAction

- [ ] Add variant to `OmnibarAction` union in `types.ts`
- [ ] Add `case "<type>":` to `dispatch.ts` switch
- [ ] Add `describe("<type>")` block with ≥1 test in `dispatch.test.ts`
- [ ] Run `cd web-app && npx jest --no-coverage --testPathPatterns="dispatch.test"` to verify

---

## Registry 2: DetectorRegistry

**Files**: `web-app/src/lib/omnibar/detector.ts`, `detector.test.ts`

### How it works

`DetectorRegistry` holds a priority-sorted list of `Detector` implementations. Each detector tries to match the omnibar input and returns a `DetectionResult` or `null`.

```typescript
export class DetectorRegistry {
  register(detector: Detector): void { ... }   // inserts sorted by priority
  detect(input: string): DetectionResult { ... } // first match wins
  detectAll(input: string): DetectionResult[] { ... }
}
```

`createDefaultRegistry()` is the single authoritative list of all registered detectors, in priority order:

| Priority | Detector | Matches |
|---|---|---|
| 10 | `GitHubPRDetector` | `https://github.com/.../pull/N` |
| 20 | `GitHubBranchDetector` | `https://github.com/.../tree/branch` |
| 30 | `GitHubRepoDetector` | `https://github.com/owner/repo` |
| 35 | `NewSessionDetector` | `new:<path>` shorthand |
| 40 | `GitHubShorthandDetector` | `owner/repo` shorthand |
| 50 | `PathWithBranchDetector` | `/path:branch` |
| 100 | `LocalPathDetector` | `/absolute/path` or `~/path` |
| 200 | `SessionSearchDetector` | everything else (search fallback) |

Lower priority number = checked first.

### When to add a new detector

Add a new `Detector` class when:
- A new input pattern should trigger a distinct behavior (e.g., a new URL scheme, a shorthand syntax)
- The pattern is not covered by any existing detector
- The detection logic is non-trivial and deserves isolation

### How to add a detector

1. Implement the `Detector` interface in `detector.ts`:
   ```typescript
   class MyDetector implements Detector {
     name = "MyDetector";
     priority = 45; // pick a priority slot
     detect(input: string): DetectionResult | null { ... }
   }
   ```
2. Register in `createDefaultRegistry()`:
   ```typescript
   registry.register(new MyDetector());
   ```
3. Add tests in `detector.test.ts` using the naming convention:
   ```
   DetectorName_should_<effect>_When_<condition>
   ```
   Assign a test ID from the next available slot:
   - Unit tests: `T-UNIT-TS-NNN`
   - Pitfall guards: `T-PITFALL-NNN`

### Test pattern

```typescript
describe("MyDetector", () => {
  it("MyDetector_should_returnGitHubPR_When_validPRUrl", () => {
    // T-UNIT-TS-012
    const registry = createDefaultRegistry();
    const result = registry.detect("https://...");
    expect(result?.type).toBe(InputType.GitHubPR);
  });
});
```

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

---

## One-Off Session: Reference Registration

The one-off session feature (2026-04-24) is the canonical example of:
- A creation mode that uses a **flag** on `create_session` rather than a new action type
- A creation mode that has **no detector** (accessed only through the creation form radio button)
- A creation mode that needs special handling in the `create_session` dispatch case

| Registry | Registered? | How |
|---|---|---|
| OmnibarAction union | no new type | Uses existing `create_session` with `sessionType: "one_off"` |
| `dispatch.ts` | ✅ | `isOneOff` guard → `oneOff: true, sessionType: undefined` |
| `dispatch.test.ts` | ✅ | `describe("create_session (one-off)")` |
| DetectorRegistry | intentionally absent | One-off is UI-only, not auto-detected from input |
| Session creation registry | ✅ | See `.claude/rules/session-creation-registry.md` |
