# Omni Bar Quick Navigation — Pitfalls & Risks Research

**Research Date**: April 2026  
**Scope**: Codebase-specific risks and known pitfalls for Omni Bar enhancement (keyboard navigation, inline creation, action registry, TUI removal)

---

## PART 1: CODEBASE-SPECIFIC RISKS

### 1. TUI Audit & Deletion Risks

**[HIGH]** **BubbleTea/TUI Infrastructure Still Exists**
- Location: `/cmd/commands/navigation.go`, `/cmd/commands/session.go`, etc.
- Status: BubbleTea (tea) imports exist in at least 14 Go files, including test utilities
- Files with tea imports:
  - `cmd/commands/system.go`, `cmd/commands/git.go`, `cmd/commands/vc.go`
  - `cmd/commands/session.go`, `cmd/commands/organization.go`, `cmd/commands/pty.go`
  - `cmd/commands/navigation.go`
  - `testutil/teatest_test.go`, `testutil/teatest_helpers.go`
  - `terminal/signals.go`, `terminal/size.go`, etc.
  - `cmd/migration.go`
- **Risk**: Navigation handlers return `(tea.Model, tea.Cmd)` tuples — fully removing TUI requires:
  1. Stub/remove all `NavigationHandlers` callback functions
  2. Remove `SetNavigationHandlers()` which is a global registration point
  3. Delete tea-dependent test utilities (`TUITestConfig`, `CreateTUITest`, etc.)
  4. Audit CI/CD pipelines for TUI-specific test runs

**[HIGH]** **Navigation Handlers Are Globally Registered Callbacks**
- Location: `cmd/commands/navigation.go:22-26`
- Pattern: Global `navigationHandlers` variable with setter; handlers return tea.Model/Cmd
- Functions: `UpCommand`, `DownCommand`, `LeftCommand`, `RightCommand`, `PageUpCommand`, `PageDownCommand`, `SearchCommand`, `NextReviewCommand`, `PreviousReviewCommand`, `ToggleReviewQueueCommand`
- **Risk**: These are active integration points. Removing without porting features to web UI breaks:
  - Up/Down/Left/Right navigation in any TUI implementation
  - Page Up/Page Down scrolling
  - Search mode entry
  - Review queue navigation
  - These must be ported as keyboard shortcuts in web UI before deletion
  - Verify no CLI scripts or automation relies on these navigation commands

**[MEDIUM]** **ForkScrollback Functionality Exists**
- Location: `session/scrollback/fork.go`, `session/scrollback/fork_test.go`
- Status: Fully implemented (copies scrollback entries up to a sequence number)
- Tests: Comprehensive fork tests exist (`session_service_fork_test.go`)
- **Risk**: Fork feature is mature and tested. If removing "fork/clone/duplicate" actions from UI, ensure:
  1. Backend `ForkSession` RPC (proto/session/v1/session.proto) is NOT removed
  2. Only UI action is removed; backend capability stays
  3. Test coverage for fork operations must remain active

**[MEDIUM]** **CLI Flags & Test Mode Infrastructure**
- Location: `main.go` lines 36-51
- Existing flags: `--daemon`, `--mcp`, `--test-mode`, `--discovery-mode`, `--profile`, `--trace`, etc.
- **Risk**: No TUI-specific CLI flags found (good sign), but:
  - Removing BubbleTea code may affect test infrastructure
  - `testutil/teatest_helpers.go` provides TUI test config — this entire module can be deleted once TUI is gone
  - CI/CD may have `make test` targets that depend on TUI tests

---

### 2. Keyboard Shortcut Conflict Analysis

**[MEDIUM]** **Cmd+N (Meta+N) Collision Risk**
- Current search: No existing Cmd+N handlers found in web-app codebase
- Good news: Appears safe to use for "create new session" mode
- **Risk**: Global hotkey might be captured by browser/OS before React handles it
  - Example: Chrome might interpret Cmd+N as "new window"
  - Mitigation: Use `e.preventDefault()` BEFORE event bubbles
  - Test on macOS, Windows, Linux to verify browser doesn't intercept

**[MEDIUM]** **Tab Key Interception in Modal**
- Location: `Omnibar.tsx:399-419` — Tab key already intercepted in dropdown context
- Current behavior:
  - Tab accepts highlighted completion OR extends to LCP of multiple entries
  - When no dropdown, Tab reverts to browser focus management
- **Risk**: Proposed "Tab to cycle session types" conflicts with:
  1. Browser native Tab (focus management) if dropdown is hidden
  2. Form field Tab cycling (users expect Tab to move focus, not cycle types)
  3. Accessibility contract — screen reader users expect Tab to navigate form
- **Mitigation**: Use a different key (e.g., `Alt+T`, `Cmd+T`, `Ctrl+Shift+T`) or context-specific cycling only in discovery mode

**[MEDIUM]** **Escape Key's Two-Escape Behavior Already Implemented**
- Location: `Omnibar.tsx:436-447`
- Current: First Escape dismisses dropdown, second Escape in discovery closes omnibar
- **Risk**: Proposed Escape behavior (Esc from creation → discovery, then Esc again → close) mirrors this pattern
- Safe to extend: The `.stopImmediatePropagation()` logic prevents double-firing
- **BUT**: Ensure new inline creation addon panel respects this escape ladder

**[LOW]** **No IME (Input Method Editor) Handling**
- Current state: No composition event listeners found
- Risk: Low — IME conflicts only matter if Tab/Escape are intercepted during composition
- Mitigation: Defer keyboard handlers until `e.nativeEvent.isComposing === false` if Tab becomes a custom key

---

### 3. State Management Complexity

**[MEDIUM]** **Current State Count: 14 `useState` Calls**
- Existing state variables (Omnibar.tsx lines 57-84):
  1. `input` (string)
  2. `detection` (DetectionResult | null)
  3. `sessionName` (string)
  4. `program` (string)
  5. `category` (string)
  6. `autoYes` (boolean)
  7. `showAdvanced` (boolean)
  8. `sessionType` (string union)
  9. `branch` (string)
  10. `useTitleAsBranch` (boolean)
  11. `existingWorktree` (string)
  12. `workingDir` (string)
  13. `isSubmitting` (boolean)
  14. `error` (string | null)
  15. `dropdownIndex` (number)
  16. `dropdownDismissed` (boolean)
  17. `mode` (OmnibarMode: "discovery" | "creation")
  18. `resultHighlightIndex` (number)

**Proposed additions for inline creation + mode badge:**
- `inline creationPanelOpen` (boolean)
- `creationPanelFields` (merged form state OR fold into existing fields)
- `modeLabel` (string) — redundant if mode already exists

**[HIGH]** **State Explosion Risk**
- Each new feature (Tab cycling types, inline panel, action registry) tempts new state
- Current pattern: Related state is scattered (e.g., `sessionType`, `branch`, `useTitleAsBranch` live independently)
- **Risk**: 25+ useState calls make refactoring hard, increase re-render surface
- **Recommendation**:
  1. Group related form fields into a single state object: `const [formState, setFormState] = useState({ sessionType, branch, useTitleAsBranch, ... })`
  2. Consolidate mode-related state: `const [uiState, setUiState] = useState({ mode, dropdownIndex, showAdvanced, inlineCreationOpen })`
  3. This reduces useState from 18 to ~5 and makes dependencies clearer
  4. **Before implementation**: Consider `useReducer` for form state management (one dispatch, less fragmentation)

**[MEDIUM]** **Detection Debounce Hidden State**
- Location: `Omnibar.tsx:88-90`
- Refs: `debounceRef`, `lastSuggestedNameRef`, `prevDetectionTypeRef`, `handleSubmitRef`
- **Risk**: Four useRef calls store hidden state not visible in component render
- If adding more debounce/cache logic (e.g., for action registry lookup), risk of ref-based state diverging from rendered state
- **Safe for now**, but document that refs are timing/cache, not reactive

---

### 4. Focus Management & Modal Interactions

**[HIGH]** **Focus Management When Switching Modes**
- Location: `Omnibar.tsx:293-297` (focus on open), `Omnibar.tsx:300-322` (reset on close)
- Current: `inputRef.current?.focus()` when omnibar opens
- **Risk**: When transitioning discovery → creation mode:
  1. Form fields are inserted into DOM but focus stays on main input
  2. User expects form fields to be tabbable, but focus trap isn't set up
  3. First Tab moves focus to first form field (good), but then what?
  4. If adding inline creation panel INSIDE the modal, need focus management:
     - Should Tab from input → form fields → buttons?
     - Or should creation panel be a separate modal overlay (focus trap)?

**Implementation concern**: Read `SessionCard.tsx:159-172` for focus trap example
- Uses `useFocusTrap()` hook for dialogs
- If inline creation panel is a _sub-form_ inside omnibar (not a separate dialog), focus trap is overkill
- But if it's a floating panel, needs its own trap

**[MEDIUM]** **Global Document Listener for Escape**
- Location: `Omnibar.tsx:469-479`
- Pattern: Document-level keydown listener, only active when `isOpen`
- **Risk**: Multiple event listeners on document can cause conflicts
  - If adding another global listener for Cmd+N, must dedup or coordinate
  - stopImmediatePropagation() in modal handler prevents document listener from firing
  - Safe if modal always handles key first

**[MEDIUM]** **Scrollback/Scrollbar in Result List**
- Location: `OmnibarResultList` component (not shown, but referenced)
- **Risk**: When result list grows (5 sessions + 8 repos = 13 items), overflow may hide highlights
- Ensure `scrollIntoView()` is called when highlighting items with keyboard
- If not already done, add `element.scrollIntoView({ behavior: 'smooth', block: 'nearest' })` to Arrow key handlers

---

### 5. `new/` Prefix Collision Risk

**[MEDIUM]** **LocalPathDetector Does NOT Special-Case `new/` Prefix**
- Location: `detector.ts:211-254` (LocalPathDetector)
- Current logic:
  - `isAbsolute` = starts with `/`
  - `hasTilde` = starts with `~/`
  - `isRelative` = starts with `./` or `../`
  - `hasMultipleSlashes` = contains more than 1 `/`
- Test cases: No test for `new/` prefix (does not start with `/`, `~`, `.`, and has only 1 `/`)
- **Risk**:
  1. User types `new/my-feature` → Does NOT match LocalPathDetector (only 1 slash)
  2. Falls through to SessionSearchDetector → treated as session search query
  3. **This is actually CORRECT** — `new/` should NOT be detected as a path
  4. But if Cmd+N mode auto-fills input with `new/`, the detector must not misinterpret it

**Mitigation**: 
- If Cmd+N prefills `new/` and awaits user completion (e.g., `new/feature-name`):
  - Once user types second `/`, it becomes `new/feature/` → 2 slashes → LocalPathDetector fires ✓
  - Inline validation badge should confirm path exists/is being created
- If `new/` is just a UI hint (not literal input), no risk

**[LOW]** **Path Completion with `new/` Prefix**
- Location: `usePathCompletions` hook (not shown)
- **Risk**: If `new/` is auto-completed, the completions backend might:
  1. Fail to find `~/new/` on disk (directory doesn't exist yet)
  2. Or find `~/.someexistingdir/new/subdir` if path happens to exist
- **Mitigation**: Document that `new/` prefix in creation mode does NOT use live path completions (or show "path will be created" hint)

---

### 6. Session Clone/Fork Action Consolidation

**[MEDIUM]** **Fork & Duplicate Are Already Implemented Backend**
- Location: `server/services/session_service_fork_test.go` (comprehensive tests)
- Backend RPC: `ForkSession` exists in proto (line 156 not shown, but inferred from tests)
- Frontend: `handleDuplicateSession` exists in `app/page.tsx`, `onDuplicate` callback in SessionCard
- **Risk**: If consolidating fork/duplicate → "Clone" action in omnibar:
  1. SessionCard already exposes `onDuplicate` — don't break this
  2. `onForkFromCheckpoint` is a separate callback (forks from a saved checkpoint, not latest state)
  3. Omnibar's new "Clone" action would be a shorthand for creating a new session with same path/program
  4. This is NOT the same as ForkSession — it's a fresh creation, not a fork of state
  5. **Decision needed**: Is omnibar "Clone" the same as SessionCard "Duplicate" or different?

**Implementation clarification**:
- SessionCard "Duplicate" (from `handleDuplicateSession`): Likely uses `?duplicate=<id>` query param to pre-fill wizard
- SessionCard "Fork from Checkpoint": Uses `ForkSession` RPC to fork from a saved point in time
- Omnibar "Clone": Proposed as quick re-creation of same config (path + program + settings)
- **These are three different operations** — clarify scope before implementing

**[LOW]** **No Naming Conflict in RPC**
- `ForkSession` RPC exists and is stable
- No `CloneSession` RPC found (good — no namespace collision)
- Safe to add omnibar "Clone" action without touching backend

---

## PART 2: KNOWN PITFALLS IN THIS PROBLEM DOMAIN

### 7. Command Palette Keyboard Navigation Bugs

**[HIGH]** **Focus Trap Escaping**
- **Problem**: User presses Escape in modal → should close modal, not activate browser back button or global shortcuts
- **This codebase handles it**: Uses `stopImmediatePropagation()` correctly
- **Risk for new code**: Any new keyboard handler must also call `stopImmediatePropagation()` BEFORE async operations
  - Example: If Tab opens a loading indicator, the Escape key fired during load might not propagate correctly
  - **Mitigation**: Prevent default SYNCHRONOUSLY, not inside async callback

**[HIGH]** **Scroll-Into-View for Highlighted Items**
- **Problem**: User presses Arrow Down, highlight moves to item #10, but item #10 is below viewport fold
- **Result**: User sees highlight index increase but no visual feedback of where they are
- **This codebase**: No `scrollIntoView()` found in OmnibarResultList (need to check SessionCard usage)
- **Risk**: If result list grows beyond viewport, Arrow key navigation becomes "blind"
- **Mitigation**: On highlight change, call:
  ```tsx
  const highlighted = document.querySelector(`[id="${getHighlightedItemId(...)}"]`);
  highlighted?.scrollIntoView({ behavior: 'instant', block: 'nearest' });
  ```

**[MEDIUM]** **Z-Index & Portal Layering**
- **Problem**: Dropdown appears below modal, or modal appears below page content
- **This codebase**: Omnibar uses a single modal div (no portal), dropdown is inline
- **Risk**: If action registry uses a portal for panels, ensure z-index doesn't conflict
- **Mitigation**: Use CSS custom properties for z-index (e.g., `--z-modal: 1000`, `--z-dropdown: 1001`)

**[MEDIUM]** **Highlight Index Boundary Bugs**
- **Problem**: User presses Arrow Down at last item → index becomes `totalCount`, but array has indices 0..count-1
- **This codebase**: Uses `Math.min(i + 1, totalResultCount - 1)` correctly (line 368)
- **Risk for new code**: If adding Tab cycling for session types, ensure index never exceeds array bounds
- **Test case**: Last item, press Arrow Down → should stay at last item (not wrap to first)

---

### 8. Inline Form-Within-Modal Patterns

**[HIGH]** **Modal Height Grows When Form Appears**
- **Problem**: User opens omnibar in discovery mode (compact). Switches to creation mode → form fields appear → modal height doubles → modal jumps/re-centers
- **Result**: Disorienting UX; focused input loses visual position
- **This codebase**: No form height animation (form appears instantly)
- **Risk for inline creation panel**: If panel expands modal height, must:
  1. Pre-compute height (CSS `grid-auto-rows` or flex sizing)
  2. OR use `max-height` + `overflow: auto` to contain growth
  3. OR animate height with `transition: height 200ms ease-in-out`

**[MEDIUM]** **Overflow & Scrolling in Nested Containers**
- **Problem**: Path completion dropdown is inside modal; result list is inside modal; form fields are inside modal → nested scrollbars
- **This codebase**: Modal has `.modal` class; dropdown has dedicated container
- **Risk for inline panel**: If panel adds a scrollable sub-section, must clarify:
  1. Does panel scroll independently? (requires `overflow: auto; max-height: X`)
  2. Or does entire modal scroll? (requires modal to be flex container with `overflow: auto`)
  3. Mixing breaks keyboard navigation (Arrow Down in list might scroll parent instead of moving highlight)

**[MEDIUM]** **Form Label & Input Alignment**
- **Problem**: Session Name field label is visible. Existing Worktree selector is visible. User focuses on field → focus ring appears → layout shift
- **This codebase**: Uses `fieldInput` class with padding/border, labels are positioned above (no side-by-side)
- **Risk**: No vertical growth issues (all fields stack), but ensure form width is locked to prevent reflow

---

### 9. Plugin/Registry Pattern Pitfalls in React

**[HIGH]** **Registration Order & Lifecycle Mismatch**
- **Problem**: Actions are registered at module load time. If action depends on a hook (useContext, useSelector), registration fails
- **Example**: `register({ name: 'Clone', handler: () => useSession() })` → Error: "hooks can't be called outside render"
- **This codebase**: No action registry exists yet; needs careful design
- **Risk**: If registry stores handler functions (not components), they can't use hooks
- **Mitigation**:
  1. Store handlers as component factories or async functions, not hook-dependent code
  2. OR: Inject dependencies at registration time: `register({ name: 'Clone', handler: (deps) => deps.selectSession() })`
  3. Prefer component-based actions where handlers are JSX

**[MEDIUM]** **Unregistered Actions Cause Silent Failures**
- **Problem**: Developer registers action `{ name: 'Clone' }` but forgets `handler`. At runtime, clicking button does nothing.
- **Result**: No error, no console warning — silent failure
- **Mitigation**: Add validation to registry:
  ```tsx
  register(action) {
    if (!action.handler) throw new Error(`Action ${action.name} missing handler`);
    // ...
  }
  ```

**[MEDIUM]** **Tree-Shaking & Unused Action Registration**
- **Problem**: If actions are registered via side effects (`import './actions/clone'`), bundler can't tree-shake
- **Result**: All actions (even unused) are included in bundle
- **This codebase**: Should use explicit registration (not import side effects)
- **Mitigation**: Register actions in a central `registerOmnibarActions.ts` file that is explicitly imported, not auto-loaded

**[MEDIUM]** **Memory Leaks from Persistent Registrations**
- **Problem**: Action handler closes over state that never gets garbage collected
- **Example**: `register({ name: 'Clone', handler: () => sessionCache[sessionId] })` — sessionCache grows unbounded
- **Mitigation**: Attach cleanup to registration:
  ```tsx
  register(action, { cleanup: () => sessionCache.clear() })
  // Call cleanup on omnibar unmount
  ```

**[LOW]** **Duplicate Registration Under Different Conditions**
- **Problem**: Component mounts twice (dev StrictMode), handler is registered twice
- **Result**: Clicking action runs handler twice
- **Mitigation**: Use React's `useEffect` cleanup or check `if (alreadyRegistered)` before registering

---

### 10. Tab Key Overloading Accessibility Risks

**[CRITICAL]** **Screen Reader Navigation Breaks**
- **Problem**: Tab key usually moves focus between focusable elements. If Tab is overloaded to "cycle session types," screen readers get confused.
- **User flow**: Screen reader user presses Tab → expects focus to move to next field → instead types cycles type selector → user thinks Tab is broken
- **Result**: Product is inaccessible to screen reader users
- **This codebase**: Uses proper `aria-label`, `aria-controls`, `aria-expanded` (good foundation)
- **Risk**: Adding Tab cycling without managing focus trap correctly violates WCAG 2.1 Level AA

**[HIGH]** **Focus Visible Ring Disappears**
- **Problem**: When Tab cycles types (without moving focus), focus ring stays on same input → user doesn't see that something changed
- **Mitigation**: If Tab must cycle types:
  1. Move focus to the cycled item: `element.focus()` after cycling
  2. OR: Use Ctrl+Tab (never overloads Tab)
  3. OR: Use arrow keys in a listbox widget (not Tab)

**[MEDIUM]** **Keyboard Trap in Modal**
- **Problem**: User presses Tab repeatedly → focus cycles: input → button1 → button2 → back to input (correct). But if modal intercepts Tab, cycle might break.
- **This codebase**: Tab is already intercepted for path completion (line 399)
- **Risk**: Ensure focus trap EXITS through first/last tabbable element, not loops internally

**Accessibility checklist for Tab cycling**:
- [ ] Tab does NOT become the only way to access feature (must have button/menu alternative)
- [ ] Tab with Shift reverses (Shift+Tab)
- [ ] Focus ring is visible (CSS `outline: 2px solid` or `box-shadow`)
- [ ] Screen reader announces changes ("Session type changed to: new_worktree")
- [ ] Focus moves to newly activated element (not stays on trigger)

---

### 11. TUI Removal Testing & Integration Points

**[HIGH]** **Test Coverage Before Deletion**
- **Current state**: `testutil/teatest_helpers.go` and `testutil/teatest_test.go` provide TUI testing framework
- **Risk**: If tests use `CreateTUITest()` and render BubbleTea app, deletion breaks tests
- **Scope of TUI tests**:
  - Navigation handlers (Up/Down/Left/Right)
  - Search mode entry
  - Terminal output rendering in TUI
  - Session list display in TUI
  - All of these MUST be migrated to web UI before TUI code is deleted
- **Mitigation**:
  1. Before deleting TUI: Identify all `*_test.go` files that import tea
  2. Port each test to web UI test (using Playwright or Jest)
  3. Only then delete TUI code

**[HIGH]** **Documentation & Examples**
- **Risk**: README, CLAUDE.md, or Getting Started guide might reference TUI commands
  - Example: "Press `j` to move down" (vi-like TUI keybinding)
  - Example: "Run `stapler-squad` to start TUI"
- **Search for**: `/TUI|terminal UI|bubbletea|keybinding|j/k|vim/` in docs
- **Mitigation**: Before deletion, update all docs to reference web UI only

**[HIGH]** **CI/CD Pipelines**
- **Risk**: GitHub Actions or local Makefile might have TUI-specific targets
  - Example: `make test-tui` or `make test-cli`
  - Example: `make lint-tui-commands`
- **Grep for**: `tea\|TUI\|buble\|tui` in `Makefile`, `.github/workflows/`, `scripts/`
- **Mitigation**: Remove TUI test jobs from CI before removing code

**[MEDIUM]** **CLI Scripts & Hooks**
- **Risk**: If users have scripts that invoke `stapler-squad <tui-command>`, they will break
- **Example**: `stapler-squad navigate-up` or `stapler-squad search`
- **Search for**: Command definitions in `cmd/commands/` that are exported for CLI
- **Mitigation**: Deprecate with warning message before deletion; provide migration path

**[MEDIUM]** **Dependency Cleanup**
- After deletion, run:
  ```bash
  go mod tidy
  grep -r "github.com/charmbracelet/bubbletea" .
  ```
  Should return zero results. Check `go.mod` — BubbleTea should no longer be listed.

---

### 12. State Synchronization Pitfalls

**[MEDIUM]** **Ref Drift vs. State Drift**
- **Problem**: `lastSuggestedNameRef` stores a string that gets out of sync with `sessionName` state
- **Current logic** (line 268-270): If `sessionName === lastSuggestedNameRef`, update suggestion
- **Risk for new code**: If adding more ref-based caches (e.g., `lastModeRef`, `lastDetectionRef`), easy to accidentally desync
- **Mitigation**: Document which refs are caches (not source of truth) vs. which are memoization

**[MEDIUM]** **Async State Updates in Handlers**
- **Problem**: User opens omnibar, types path, detection fires (debounced 150ms). If user closes omnibar before debounce fires, state update still happens
- **Current code** (line 285-289): Cleans up debounce timer on unmount ✓
- **Risk for new inline panel**: If panel has async operations (e.g., `onCreateCheckpoint`), ensure cleanup on close
- **Mitigation**: Store cleanup functions and call on unmount

---

## SUMMARY TABLE

| Pitfall | Severity | Type | Mitigation |
|---------|----------|------|-----------|
| TUI deletion without porting | HIGH | Codebase | Audit all tea imports; port navigation handlers to web |
| Cmd+N keyboard capture | MEDIUM | Codebase | Test on macOS/Windows; use preventDefault() |
| Tab key overloading | CRITICAL | Domain | Use different key; provide non-Tab alternative |
| State explosion (18+ useState) | HIGH | Codebase | Refactor to useReducer; group related state |
| Focus trap in modal | HIGH | Domain | Use `useFocusTrap`; test Arrow+Tab navigation |
| new/ prefix collision | MEDIUM | Codebase | Verify detector doesn't misclassify; test with path completion |
| Screen reader Tab override | CRITICAL | Domain | Never override Tab for non-focus-movement; use Ctrl+Tab |
| Scroll-into-view missing | MEDIUM | Domain | Add scrollIntoView() on highlight change |
| Form height growth | MEDIUM | Domain | Use max-height or animate height transitions |
| Action registry unregistered handlers | MEDIUM | Domain | Add validation; throw on missing handler |
| Tree-shaking actions | MEDIUM | Domain | Use central registration file; avoid import side effects |
| TUI test migration before deletion | HIGH | Codebase | Identify tea-dependent tests; port to web UI |
| Ref drift from state | MEDIUM | Codebase | Document ref purpose; add comments |

---

## RECOMMENDATIONS

### Before Implementation
1. **Run full TUI audit**: `grep -r "bubbletea\|tea\." --include="*.go"` → identify all files to modify
2. **Consolidate state**: Convert 18 useState to ~5 with useReducer for form state
3. **Finalize Tab behavior**: Decide if Tab cycles types or uses Ctrl+Tab
4. **Design action registry**: Decide if handlers are functions, components, or RPC calls
5. **Accessibility review**: Have screen reader user test Tab behavior before commit

### During Implementation
1. Add `scrollIntoView()` to all Arrow key handlers
2. Add test for `new/` prefix path completion
3. Keep ForkSession RPC intact; only remove UI buttons
4. Add cleanup for async operations in inline panel
5. Use `stopImmediatePropagation()` consistently in all keyboard handlers

### After Implementation
1. Delete BubbleTea imports and test files
2. Update CLAUDE.md and README to reference web UI only
3. Run `go mod tidy` to remove unused dependencies
4. Verify no GitHub Actions jobs reference TUI
5. Test with screen reader (VoiceOver on macOS)

