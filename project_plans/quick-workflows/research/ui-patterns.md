# UI Patterns Research: Workflow Management Panel

Research date: 2026-06-11
Branch: `stapler-squad-quick-workflows`

---

## 1. Navigation / Sidebar Structure

### How navigation works

The sidebar is `DrawerNav` — a collapsible left-side navigation used on desktop (hidden on mobile, where `BottomNav` takes over). The nav items are **not** hardcoded inside `DrawerNav.tsx`; they are driven by a single array.

**Key files:**

| File | Role |
|------|------|
| `web-app/src/lib/nav-pages.ts` | Single source of truth for all nav items |
| `web-app/src/lib/routes.ts` | All route strings as typed constants |
| `web-app/src/components/layout/DrawerNav.tsx` | Renders the sidebar from `NAV_PAGES` |
| `web-app/src/components/layout/BottomNav.tsx` | Mobile nav, also driven by `NAV_PAGES` |

### `NavPage` interface

```typescript
// web-app/src/lib/nav-pages.ts
export interface NavPage {
  href: string;
  label: string;
  shortLabel?: string;         // for BottomNav (falls back to label)
  icon: LucideIcon;
  mobileNav?: boolean;         // false = desktop/hamburger only
  headerNav?: boolean;         // false = exclude from header nav row
  bottomNavPrimary?: boolean;  // true = shown in BottomNav primary bar
  featureFlag?: string;        // feature flag name to gate visibility
}
```

### How to add a new top-level nav entry

**Step 1:** Add a route constant to `routes.ts`:
```typescript
workflows: "/workflows",
```

**Step 2:** Add a `NavPage` entry to the `NAV_PAGES` array in `nav-pages.ts`:
```typescript
import { Zap } from "lucide-react";

{ href: routes.workflows, label: "Workflows", icon: Zap, headerNav: false },
```

Use `headerNav: false` to keep it in the drawer/hamburger but not the header row. Use `featureFlag: "quick-workflows"` to gate it behind the existing feature flag system.

The component `DrawerNav` renders `NAV_PAGES` in order with no additional configuration required.

---

## 2. Page Routing

The app uses **Next.js App Router** (Next.js 13+ file-system routing). Every page is a directory under `web-app/src/app/`.

### Existing page structure pattern

Each page follows this minimal structure:
```
web-app/src/app/<route>/
  layout.tsx      # Metadata (title, description) + thin wrapper
  page.tsx        # "use client" page component with Suspense boundary
  page.css.ts     # vanilla-extract styles colocated with the page
```

**`layout.tsx` template (minimal — matches rules/layout.tsx):**
```typescript
import type { Metadata } from "next";

export const metadata: Metadata = {
  title: "Workflows - Stapler Squad",
  description: "Manage quick workflow templates.",
};

export default function WorkflowsLayout({ children }: { children: React.ReactNode }) {
  return <>{children}</>;
}
```

**`page.tsx` template (matches rules/page.tsx):**
```typescript
"use client";
// +feature: workflows-management

import { Suspense } from "react";
import { WorkflowsPanel } from "@/components/workflows/WorkflowsPanel";
import * as styles from "./page.css";

function WorkflowsPageInner() {
  return (
    <div className={styles.page}>
      <main id="main-content" className={styles.main}>
        <WorkflowsPanel />
      </main>
    </div>
  );
}

export default function WorkflowsPage() {
  return (
    <Suspense fallback={<div style={{ padding: "2rem" }}>Loading…</div>}>
      <WorkflowsPageInner />
    </Suspense>
  );
}
```

### Where to add the new route/page file

Create:
```
web-app/src/app/workflows/
  layout.tsx
  page.tsx
  page.css.ts
```

The `page.css.ts` for the workflows page should copy the pattern from `web-app/src/app/rules/page.css.ts`, which defines a `page` (flex column, full-height) and `main` (max-width centered, padded, scrollable) — this is the canonical single-column content page layout.

---

## 3. Vanilla-Extract CSS Patterns

### Theme contract

The theme contract lives at `web-app/src/styles/theme-contract.css.ts` and exports `vars`, `breakpoints`, and `zIndex`. Always import from `@/styles/theme.css` (which re-exports), not the contract file directly.

```typescript
import { vars } from "@/styles/theme.css";
```

**Key token categories for a management panel:**

| Category | Tokens |
|----------|--------|
| Text | `vars.color.textPrimary`, `textSecondary`, `textMuted` |
| Surfaces | `vars.color.background`, `cardBackground`, `hoverBackground` |
| Borders | `vars.color.borderColor`, `borderSubtle`, `borderHover` |
| Actions | `vars.color.primary`, `primaryHover`, `primaryText` |
| Status | `vars.color.error`, `errorBg`, `success`, `successBg` |
| Input | `vars.color.inputBackground`, `inputText`, `inputBorder`, `inputFocusBorder` |
| Spacing | `vars.space["1"/"2"/"3"/"4"/"6"/"8"]` |
| Radii | `vars.radii.sm`, `md`, `lg` |
| Font | `vars.fontSize.xs/sm/base/lg`, `vars.fontWeight.normal/medium/semibold/bold` |
| Transitions | `vars.transition.fast/base/slow` |

**z-index slots** (never hardcode numbers):
```typescript
import { zIndex } from "@/styles/theme.css";
// Use: zIndex.modal, zIndex.dropdown, zIndex.raised, etc.
```

### Canonical recipe pattern

```typescript
// WorkflowsPanel.css.ts
import { style } from "@vanilla-extract/css";
import { recipe } from "@vanilla-extract/recipes";
import { vars } from "@/styles/theme.css";

export const container = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["4"],
});

export const workflowCard = recipe({
  base: {
    background: vars.color.cardBackground,
    border: `1px solid ${vars.color.borderColor}`,
    borderRadius: vars.radii.lg,
    padding: vars.space["4"],
    transition: vars.transition.fast,
  },
  variants: {
    selected: {
      true: {
        borderColor: vars.color.primary,
        background: vars.color.accentBg,
      },
      false: {},
    },
  },
  defaultVariants: { selected: false },
});
```

### Page layout pattern (from `rules/page.css.ts`)

```typescript
import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const page = style({
  display: "flex",
  flexDirection: "column",
  minHeight: "calc(var(--viewport-height, 100dvh) - var(--header-height))",
  background: vars.color.background,
});

export const main = style({
  flex: 1,
  maxWidth: "1200px",
  width: "100%",
  margin: "0 auto",
  padding: "2rem",
  overflowY: "auto",
  display: "flex",
  flexDirection: "column",
  gap: "2rem",
  "@media": {
    "screen and (max-width: 768px)": {
      padding: "1rem",
    },
  },
});
```

For a settings-style single-column form page (narrower), use `maxWidth: "960px"` (see `settings/settings.css.ts`).

---

## 4. Shared UI Components

All shared components are in `web-app/src/components/ui/` and exported via `web-app/src/components/ui/index.ts`.

### Available components

| Component | File | Key props |
|-----------|------|-----------|
| `Button` | `Button.tsx` | `intent` (primary/secondary/danger/ghost), `size` (sm/md/lg), `asChild` for Radix Slot |
| `Input` | `Input.tsx` | `label`, `error`, `inputSize` (sm/md/lg), `state` (default/error/disabled) |
| `Card` | `Card.tsx` | `variant` (default/elevated/bordered/interactive), `padding` (none/sm/md/lg) |
| `CardHeader` | `Card.tsx` | standard div with bottom border |
| `CardTitle` | `Card.tsx` | styled h3 |
| `CardDescription` | `Card.tsx` | styled p |
| `CardFooter` | `Card.tsx` | flex row, top border |
| `Modal` | `Modal.tsx` | Radix Dialog wrapper; `ModalTrigger`, `ModalContent`, `ModalTitle`, `ModalClose`, `ModalFooter` |
| `Badge` | `Badge.tsx` | status chips |
| `Skeleton` | `Skeleton.tsx` | loading placeholder |

**Import pattern:**
```typescript
import { Button, Input, Card, CardHeader, CardTitle, CardFooter, Modal, ModalContent, ModalTitle, ModalFooter, ModalClose } from "@/components/ui";
```

### Form field pattern (from OmnibarCreationPanel.tsx)

The omnibar form uses inline `<input>` elements styled with Omnibar.css.ts classes (`field`, `label`, `fieldInput`, `hint`). For the Workflows panel, use the `Input` component from `/ui/` instead — it wraps the label/error/input pattern automatically:

```tsx
<Input
  id="workflow-name"
  label="Workflow Name"
  error={errors.name}
  value={name}
  onChange={(e) => setName(e.target.value)}
  placeholder="My Workflow"
/>
```

### Textarea

There is no shared `Textarea` component. Use a native `<textarea>` styled inline or add a `Textarea.css.ts`/`Textarea.tsx` colocated with the `WorkflowsPanel`. The Omnibar panel uses a native `<textarea className={fieldInput}>` with `resize: vertical`.

### Select

There is no shared `Select` component. Use a native `<select>` with a colocated `select` recipe style, or Radix Select if you need custom styling. The Omnibar uses native `<select className={selectClass}>` (defined in `Omnibar.css.ts`).

---

## 5. ConnectRPC Hook Patterns

### Transport

Two transport types exist:

1. **`getConnectTransport()`** (`web-app/src/lib/api/transport.ts`) — singleton HTTP transport for all **unary** RPCs. Use this for CRUD operations.
2. **`createWatchTransport()`** — separate WebSocket transport for streaming Watch* RPCs only. Workflows does not need this.

### Canonical simple hook pattern (from `useApprovalRules.ts`)

This is the recommended pattern for a self-contained CRUD hook:

```typescript
"use client";

import { useEffect, useState, useCallback, useRef } from "react";
import { createClient } from "@connectrpc/connect";
import { SessionService } from "@/gen/session/v1/session_pb";
import { getConnectTransport } from "@/lib/api/transport";

interface UseWorkflowsReturn {
  workflows: WorkflowProto[];
  loading: boolean;
  error: Error | null;
  createWorkflow: (data: ...) => Promise<void>;
  updateWorkflow: (id: string, data: ...) => Promise<void>;
  deleteWorkflow: (id: string) => Promise<void>;
  refresh: () => Promise<void>;
}

export function useWorkflows(): UseWorkflowsReturn {
  const [workflows, setWorkflows] = useState<WorkflowProto[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<Error | null>(null);

  const clientRef = useRef<ReturnType<typeof createClient<typeof SessionService>> | null>(null);

  useEffect(() => {
    clientRef.current = createClient(SessionService, getConnectTransport());
  }, []);

  const fetchWorkflows = useCallback(async () => {
    if (!clientRef.current) return;
    setLoading(true);
    setError(null);
    try {
      const resp = await clientRef.current.listWorkflows({});
      setWorkflows(resp.workflows ?? []);
    } catch (err) {
      setError(err instanceof Error ? err : new Error("Failed to fetch workflows"));
    } finally {
      setLoading(false);
    }
  }, []);

  const refresh = useCallback(() => fetchWorkflows(), [fetchWorkflows]);

  useEffect(() => {
    refresh();
  }, [refresh]);

  const createWorkflow = useCallback(async (data: ...) => {
    if (!clientRef.current) return;
    await clientRef.current.createWorkflow({ ... });
    await refresh();
  }, [refresh]);

  const deleteWorkflow = useCallback(async (id: string) => {
    if (!clientRef.current) return;
    await clientRef.current.deleteWorkflow({ id });
    // Optimistic update:
    setWorkflows((prev) => prev.filter((w) => w.id !== id));
  }, []);

  return { workflows, loading, error, createWorkflow, deleteWorkflow, refresh };
}
```

**Key patterns to follow:**
- `clientRef` (not `useState`) for the client — avoids re-renders on initialization
- `useEffect` with `getConnectTransport()` to initialize the client after mount (required for SSR compatibility)
- `useCallback` on all exposed functions for stable references
- `setLoading(true)` + `setError(null)` before every fetch
- `setError` in catch; `setLoading(false)` in finally
- Optimistic deletes: update local state immediately, don't re-fetch after delete
- Non-optimistic creates/updates: re-fetch after mutation to get server-assigned fields (id, timestamps)

### Alternative: inline client in settings-style components

`GlobalDefaultsForm.tsx` creates the client directly in a `useEffect` (not via `getConnectTransport`):

```typescript
useEffect(() => {
  const transport = createConnectTransport({ baseUrl: getApiBaseUrl() });
  clientRef.current = createClient(SessionService, transport);
  loadDefaults();
}, [loadDefaults]);
```

This pattern works but creates a new transport per component mount. For the workflows hook, prefer `getConnectTransport()` singleton.

### Error handling

- Catch all errors in try/catch
- Store as `Error | null` in state
- Display inline in the component: `{error && <div className={errorStyle}>{error.message}</div>}`
- No global error dispatcher for CRUD hooks (unlike `useSessionService` which dispatches to Redux)

---

## 6. Settings/Config Page Form Patterns

### Reference: `GlobalDefaultsForm.tsx`

This is the most instructive example of a multi-field CRUD form. Key patterns:

1. **Local state** for each field (`useState`)
2. **`loadDefaults` async function** called on mount via `useEffect`
3. **`handleSave` async function** for form submission with `saving` + `error` + `success` state
4. **Sections** within the form separated by `<div className={field}>` wrappers with `<label>` + input + optional hint text
5. **Cancel/Save footer** buttons: secondary (cancel) + primary (save)

### Reference: `settings/page.tsx` + `settings.css.ts`

For a page with multiple sections or sub-views, use `@radix-ui/react-tabs` (already used in settings):

```typescript
import * as Tabs from "@radix-ui/react-tabs";

<Tabs.Root defaultValue="list">
  <Tabs.List className={tabList}>
    <Tabs.Trigger value="list" className={tab({})}>All Workflows</Tabs.Trigger>
    <Tabs.Trigger value="new" className={tab({})}>Create New</Tabs.Trigger>
  </Tabs.List>
  <Tabs.Content value="list" className={tabPanel}>...</Tabs.Content>
  <Tabs.Content value="new" className={tabPanel}>...</Tabs.Content>
</Tabs.Root>
```

The `tab` and `tabList` styles in `settings.css.ts` can be re-exported from a shared location or reproduced in the workflows page css file. Since there's no shared tab component yet, copy the pattern.

---

## 7. Concrete Recommendations

### File locations

```
web-app/src/app/workflows/
  layout.tsx                        # Metadata wrapper
  page.tsx                          # "use client" page with Suspense
  page.css.ts                       # page + main layout styles

web-app/src/components/workflows/
  WorkflowsPanel.tsx                # Main panel: list + create/edit forms
  WorkflowsPanel.css.ts
  WorkflowForm.tsx                  # Create/edit form
  WorkflowForm.css.ts

web-app/src/lib/hooks/
  useWorkflows.ts                   # ConnectRPC CRUD hook
```

### Adding the sidebar nav entry

1. Add `workflows: "/workflows"` to `routes.ts`
2. Add to `NAV_PAGES` in `nav-pages.ts`:
   ```typescript
   import { Zap } from "lucide-react";  // or whichever lucide icon fits
   
   { href: routes.workflows, label: "Workflows", icon: Zap, headerNav: false, featureFlag: "quick-workflows" }
   ```
   Place after the `rules` entry (secondary section). Set `mobileNav: false` if workflows is not a primary mobile workflow.

### Components to reuse

- `Button` from `@/components/ui` — for Create, Save, Cancel, Delete actions
- `Input` from `@/components/ui` — for workflow name, description, trigger fields
- `Card`, `CardHeader`, `CardTitle`, `CardFooter` from `@/components/ui` — for list item cards and the form container
- `Modal`, `ModalContent`, `ModalTitle`, `ModalFooter`, `ModalClose` from `@/components/ui` — for the delete confirmation dialog
- `@radix-ui/react-tabs` (already a dependency) — if the panel has list + create tabs

### vanilla-extract approach for WorkflowsPanel.css.ts

```typescript
import { style } from "@vanilla-extract/css";
import { recipe } from "@vanilla-extract/recipes";
import { vars } from "@/styles/theme.css";

// Page shell (copy from rules/page.css.ts)
export const page = style({
  display: "flex",
  flexDirection: "column",
  minHeight: "calc(var(--viewport-height, 100dvh) - var(--header-height))",
  background: vars.color.background,
});

export const main = style({
  flex: 1,
  maxWidth: "960px",    // narrower since this is a config page, not a list
  width: "100%",
  margin: "0 auto",
  padding: "2rem",
  overflowY: "auto",
  display: "flex",
  flexDirection: "column",
  gap: "2rem",
  "@media": {
    "screen and (max-width: 768px)": { padding: "1rem" },
  },
});

// Panel header with title + action button
export const panelHeader = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "space-between",
  marginBottom: vars.space["4"],
});

export const panelTitle = style({
  fontSize: vars.fontSize.xl,
  fontWeight: vars.fontWeight.bold,
  color: vars.color.textPrimary,
});

// Empty state
export const emptyState = style({
  padding: vars.space["8"],
  textAlign: "center",
  color: vars.color.textMuted,
  fontSize: vars.fontSize.sm,
});

// Workflow list item card variant
export const workflowCard = recipe({
  base: {
    background: vars.color.cardBackground,
    border: `1px solid ${vars.color.borderColor}`,
    borderRadius: vars.radii.lg,
    padding: vars.space["4"],
    transition: vars.transition.fast,
    display: "flex",
    alignItems: "flex-start",
    gap: vars.space["3"],
    selectors: {
      "&:hover": {
        borderColor: vars.color.borderHover,
        background: vars.color.hoverBackground,
      },
    },
  },
  variants: {
    editing: {
      true: {
        borderColor: vars.color.primary,
        background: vars.color.accentBg,
      },
      false: {},
    },
  },
  defaultVariants: { editing: false },
});

// Form field layout (matches settings components)
export const field = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["1"],
});

export const fieldLabel = style({
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.medium,
  color: vars.color.textSecondary,
});

export const fieldHint = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
});

// Footer with action buttons
export const formFooter = style({
  display: "flex",
  justifyContent: "flex-end",
  gap: vars.space["2"],
  paddingTop: vars.space["4"],
  borderTop: `1px solid ${vars.color.borderColor}`,
  marginTop: vars.space["4"],
});
```

### ConnectRPC hook pattern

Use `useWorkflows.ts` following the `useApprovalRules.ts` pattern (section 5 above):
- `clientRef` initialized in `useEffect` via `getConnectTransport()`
- `fetchWorkflows` called on mount
- `createWorkflow`, `updateWorkflow`, `deleteWorkflow` as `useCallback`s
- Local `loading`/`error` state (no Redux — workflows data is page-local, not global)
- Optimistic deletes; re-fetch after create/update

---

## 8. Feature Registry Requirements

Per `.claude/rules/feature-registry.md`, every new feature PR must:

1. Add a new entry to `docs/registry/backend-features.json` when the backend RPC is wired
2. Add a new entry to `docs/registry/frontend-features.json` for the page and panel component:
   ```json
   {
     "id": "workflows-management",
     "type": "frontend",
     "component": "WorkflowsPanel",
     "path": "web-app/src/components/workflows/WorkflowsPanel.tsx",
     "tested": false,
     "testIds": []
   }
   ```
3. Add at least one Playwright e2e test in `tests/e2e/workflows.spec.ts`
4. Add `// +feature: workflows-management` in the first 10 lines of `WorkflowsPanel.tsx`

Per `.claude/rules/feature-testing-registry.md`, if the Omnibar gains a "run workflow" action, add it to the `OmnibarAction` discriminated union in `web-app/src/lib/omnibar/actions/types.ts`.
