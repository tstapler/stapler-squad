# Frontend Quick Wins: Type Safety, Naming, Routes, Terminal Theme

## Epic Overview

**User Value**: These 5 independent improvements eliminate TypeScript escape hatches (`any`), reduce cognitive friction from inconsistent naming, and fix a usability gap (terminal unreadable in light mode). Each is small enough to complete in a single focused session, but collectively they raise baseline code quality across the frontend.

**Success Metrics**:
- Zero `any` types in non-library code
- Single modal state naming convention across all 6 affected files
- All 8 app routes represented in `lib/routes.ts`
- Terminal renders with correct contrast in both light and dark OS themes

**Scope**: 5 independent atomic tasks. No stories layer needed — all tasks can be parallelized.

**Constraints**:
- Must not break existing component APIs or prop signatures
- xterm.js theme change must not cause terminal flicker or require page reload

---

## Architecture Decision Record

### ADR-P3-01: Terminal Theme Detection Strategy

**Context**: `XtermTerminal.tsx` uses a hardcoded dark theme. CSS `prefers-color-scheme` media queries cannot drive xterm.js theming since xterm requires a JavaScript theme object, not CSS variables.

**Decision**: Use `window.matchMedia("(prefers-color-scheme: dark)")` with a `change` event listener to detect and reactively update the terminal theme at runtime.

**Rationale**: This is the only approach that works with xterm.js's imperative theme API. A React context-based theme system would also work but adds unnecessary complexity for a binary dark/light choice.

**Consequences**:
- Theme changes take effect immediately without reload
- The WebGL renderer's texture atlas may show stale colors after a dynamic theme change — mitigate by calling `terminal.refresh(0, terminal.rows - 1)` after theme update
- `lib/config/terminalConfig.ts` needs an `"auto"` mode that resolves at runtime

**Patterns Applied**: Observer (matchMedia event listener), Strategy (light/dark theme objects)

---

## Atomic Tasks (All Independent — Can Parallelize)

---

### Task P3-1: Standardize Modal/Panel Open State Naming [1h]

**Objective**: Apply a single boolean state naming convention for modal/panel open state across all frontend components.

**Context Boundary**:
- Primary: `web-app/src/components/sessions/SessionCard.tsx`
- Supporting: `web-app/src/components/layout/Header.tsx`, `web-app/src/components/ui/NotificationPanel.tsx`, `web-app/src/components/sessions/WorkspaceSwitchModal.tsx`, `web-app/src/app/page.tsx`, `web-app/src/app/history/page.tsx`
- ~600 lines total across affected files

**Current State (3 patterns in use)**:

| Pattern | Instances | Example |
|---------|-----------|---------|
| `showXxx` | ~12 | `showRenameDialog`, `showTagEditor`, `showHelp` |
| `isXxxOpen` | 4 | `isMobileMenuOpen`, `isPanelOpen` |
| `isXxxVisible` | 1 | `isDebugMenuOpen` |

**Decision**: Standardize on `isXxxOpen` (already used in `Header.tsx` and context files).

**Implementation**:
1. In `SessionCard.tsx`: rename `showRenameDialog` → `isRenameOpen`, `showTagEditor` → `isTagEditorOpen`, `showRestartConfirm` → `isRestartConfirmOpen`, `showDeleteConfirm` → `isDeleteConfirmOpen`
2. In `page.tsx`: rename `showHelp` → `isHelpOpen`
3. In `Header.tsx`: `isMobileMenuOpen` already correct ✅, `isDebugMenuOpen` → `isDebugOpen`
4. In `history/page.tsx`: rename any `showXxx` modal state to `isXxxOpen`
5. Verify all renamed variables are updated in JSX conditions and event handlers

**Validation**:
- `npm run build` in `web-app/` passes with no TypeScript errors
- All modals still open/close correctly (manual smoke test)
- No `showRenameDialog`, `showTagEditor`, `showHelp` variables remain in component files

**INVEST Check**:
- Independent: No dependencies on other tasks ✅
- Negotiable: Prefix choice flexible (`isXxxOpen` vs `showXxx`) ✅
- Valuable: Reduces cognitive friction for all future contributors ✅
- Estimable: 1 hour with high confidence ✅
- Small: Rename only, no logic changes ✅
- Testable: Build passes + manual smoke test ✅

---

### Task P3-2: Fix `any` Timestamp Types in history/page.tsx [1h]

**Objective**: Replace 4 `any` type annotations for protobuf Timestamp handling with proper types.

**Context Boundary**:
- Primary: `web-app/src/app/history/page.tsx` (lines 71, 84, 99, 115)
- Supporting: `web-app/src/gen/session/v1/types_pb.ts` (Timestamp import source)
- ~200 lines total

**Current State**: Four helper functions use `timestamp: any`:
```typescript
function formatTimestamp(timestamp: any): string { ... }
function compareTimestamps(a: any, b: any): number { ... }
```

**Target State**:
```typescript
import type { Timestamp } from "@bufbuild/protobuf";

function formatTimestamp(timestamp: Timestamp | undefined): string { ... }
function compareTimestamps(a: Timestamp | undefined, b: Timestamp | undefined): number { ... }
```

**Implementation**:
1. Add `import type { Timestamp } from "@bufbuild/protobuf"` to `history/page.tsx`
2. Replace all 4 `any` annotations with `Timestamp | undefined`
3. Add null guards where needed (timestamp may be undefined)
4. Verify `toDate()` method is available on Timestamp (it is — part of protobuf-ts API)

**Validation**:
- `npm run build` passes — no remaining `any` in history/page.tsx
- History page renders timestamps correctly in browser

**INVEST Check**: All criteria met, 1 hour, no dependencies ✅

---

### Task P3-3: Type ConnectRPC Client Refs [1h]

**Objective**: Replace `useRef<any>` for ConnectRPC client storage with properly typed refs.

**Context Boundary**:
- Primary files (6 total):
  - `web-app/src/lib/hooks/useSessionService.ts` (line 62)
  - `web-app/src/lib/hooks/useReviewQueue.ts` (line 104)
  - `web-app/src/lib/hooks/useAuditLog.ts` (line 27)
  - `web-app/src/app/history/page.tsx` (line 191)
  - `web-app/src/app/config/page.tsx` (line 58)
  - `web-app/src/components/sessions/TerminalOutput.tsx` (line 20)
- Supporting: `web-app/src/gen/session/v1/session_connect.ts` (service type definitions)

**Correct Pattern** (already used in `NotificationPanel.tsx`):
```typescript
import { createPromiseClient } from "@connectrpc/connect";
import { SessionService } from "@/gen/session/v1/session_connect";

const clientRef = useRef<ReturnType<typeof createPromiseClient<typeof SessionService>> | null>(null);
```

**Implementation**:
1. For each file, identify the service being called (SessionService, WorkspaceService, etc.)
2. Import the correct service type from gen/
3. Replace `useRef<any>` with `useRef<ReturnType<typeof createPromiseClient<typeof XxxService>> | null>`
4. Fix any downstream type errors (callers of `clientRef.current?.method()` may gain better inference)

**Validation**:
- `npm run build` passes — `tsc --noEmit` shows no remaining implicit any in hook files
- All API calls from hooks still function correctly

**INVEST Check**: All criteria met, 1 hour, no dependencies ✅

---

### Task P3-4: Complete Route Constants in lib/routes.ts [30m]

**Objective**: Ensure `lib/routes.ts` is the single source of truth for all app routes.

**Context Boundary**:
- Primary: `web-app/src/lib/routes.ts` (14 lines)
- Supporting: `web-app/src/components/layout/Header.tsx` (hardcoded route strings at lines 64-93)
- ~100 lines total

**Current State**:
```typescript
export const routes = {
  home: "/",
  sessionDetail: (id: string) => `/sessions/${id}`,
  sessionCreate: "/sessions/new",
  dashboard: "/dashboard",    // ← stale: no page exists
  settings: "/settings",     // ← stale: no page exists
} as const;
```

Missing: `/history`, `/logs`, `/review-queue`, `/config`

**Target State**:
```typescript
export const routes = {
  home: "/",
  sessionCreate: "/sessions/new",
  reviewQueue: "/review-queue",
  history: "/history",
  logs: "/logs",
  config: "/config",
  login: "/login",
  sessionDetail: (id: string) => `/sessions/${id}`,
} as const;
```

**Implementation**:
1. Update `lib/routes.ts` with the 6 active routes, remove 2 stale entries
2. Update `Header.tsx` to use `routes.history`, `routes.logs`, `routes.reviewQueue`, `routes.config` instead of hardcoded strings
3. Search for any other hardcoded route strings in components and update to use constants

**Validation**:
- `npm run build` passes
- All nav links in Header still work correctly

**INVEST Check**: All criteria met, 30 minutes, no dependencies ✅

---

### Task P3-5: Terminal Light Mode Theme Support [2h]

**Objective**: Make the terminal render with correct contrast in both light and dark OS themes using runtime `matchMedia` detection.

**Context Boundary**:
- Primary: `web-app/src/components/sessions/XtermTerminal.tsx` (446 lines)
- Supporting: `web-app/src/lib/config/terminalConfig.ts` (terminal theme configuration)
- Supporting: `web-app/src/app/globals.css` (CSS custom properties for theme reference)
- ~600 lines total

**Current State**: `XtermTerminal.tsx` passes a hardcoded dark theme object to xterm.js. Light mode users see white text on white/light backgrounds.

**Target State**: Terminal detects `prefers-color-scheme` on mount and subscribes to changes. Theme objects for light and dark modes are defined in `terminalConfig.ts`.

**Light Theme Object**:
```typescript
export const lightTerminalTheme: ITheme = {
  background: '#ffffff',
  foreground: '#1a1a1a',
  cursor: '#333333',
  selectionBackground: 'rgba(0, 112, 243, 0.3)',
  black: '#000000', red: '#c0392b', green: '#27ae60',
  yellow: '#e67e22', blue: '#2980b9', magenta: '#8e44ad',
  cyan: '#16a085', white: '#bdc3c7',
  brightBlack: '#7f8c8d', brightRed: '#e74c3c',
  brightGreen: '#2ecc71', brightYellow: '#f39c12',
  brightBlue: '#3498db', brightMagenta: '#9b59b6',
  brightCyan: '#1abc9c', brightWhite: '#ecf0f1',
};
```

**Implementation**:
1. Add `lightTerminalTheme` and `darkTerminalTheme` exports to `terminalConfig.ts`
2. In `XtermTerminal.tsx`, add `useEffect` that:
   - Detects initial theme from `window.matchMedia("(prefers-color-scheme: dark)")`
   - Subscribes to `change` events on the media query
   - Calls `terminal.options.theme = newTheme` on change
   - Calls `terminal.refresh(0, terminal.rows - 1)` to flush WebGL texture cache
   - Returns cleanup function to remove event listener
3. Pass resolved theme (not hardcoded) to xterm Terminal constructor

**Validation**:
- Terminal visible in both light and dark OS theme settings
- Switching OS theme (System Preferences) updates terminal immediately without reload
- No terminal flicker on theme change (verify WebGL refresh)

**INVEST Check**: All criteria met, 2 hours, no dependencies ✅

---

## Dependency Visualization

```
All 5 tasks are fully independent — run in parallel:

P3-1 Modal naming  ─────────────────────────────────────┐
P3-2 Timestamp any ─────────────────────────────────────┤
P3-3 Client ref any ────────────────────────────────────┤──► Build passes, all issues resolved
P3-4 Route constants ───────────────────────────────────┤
P3-5 Terminal theme ────────────────────────────────────┘
```

---

## Integration Checkpoint

After all 5 tasks:
- `npm run build` in `web-app/` passes with zero TypeScript errors
- `tsc --noEmit` shows no `any` usage in non-library code
- All 8 routes defined in `lib/routes.ts` and used from `Header.tsx`
- Terminal renders correctly in light and dark OS themes

---

## Context Preparation Guide

**For P3-1** (naming): Read `SessionCard.tsx` (find all `show` state variables), `Header.tsx` (check existing `isXxxOpen` pattern)

**For P3-2** (timestamps): Read `history/page.tsx` lines 60-130, `types_pb.ts` to find Timestamp import

**For P3-3** (client refs): Read `NotificationPanel.tsx` line 39 for the correct pattern, then replicate across 5 files

**For P3-4** (routes): Read `lib/routes.ts` and `Header.tsx` nav section

**For P3-5** (terminal): Read `XtermTerminal.tsx` lines 1-80 (constructor + theme setup), `terminalConfig.ts` (current theme definition)

---

## Success Criteria

- [ ] All 5 atomic tasks completed and validated
- [ ] `npm run build` passes cleanly
- [ ] No `any` types remaining in non-library frontend files
- [ ] Single `isXxxOpen` naming convention across all modal state
- [ ] All active routes in `lib/routes.ts`, stale entries removed
- [ ] Terminal theme switches dynamically with OS preference
