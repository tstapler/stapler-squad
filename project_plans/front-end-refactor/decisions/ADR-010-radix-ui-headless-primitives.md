# ADR-010: Radix UI as Headless Primitive Library

## Status
Accepted

## Context

The stapler-squad web UI has no shared component primitive library. Every screen reimplements buttons, modals, inputs, and layout patterns with bespoke CSS Module files. This creates three compounding problems:

1. **Inconsistency**: Touch targets, focus management, and keyboard navigation are implemented differently per component — some correctly, many not.
2. **Accessibility gaps**: Modal implementations do not trap focus; custom select dropdowns have no ARIA roles; interactive elements lack `aria-label`.
3. **Migration friction**: The vanilla-extract migration (ADR-009) requires that new `.css.ts` files style something. Without a component primitive layer, every migrated component re-invents its own structure.

The primary device target (Pixel 9 Pro Fold) makes accessibility and keyboard navigation more important than on desktop — GBoard input, switch access, and TalkBack all depend on correct ARIA semantics.

### Requirements for a primitive library

- **No built-in styles**: Components must be unstyled or minimally styled so vanilla-extract provides all visual styling (ADR-009). Any library that bundles Tailwind or CSS-in-JS is incompatible.
- **React 19 / Next.js 15 compatibility**: The library must work with React 19's concurrent features and Next.js 15 App Router (Server + Client Components).
- **WCAG AA accessibility**: Focus management, keyboard navigation, ARIA roles — provided by the library, not hand-coded.
- **No Tailwind dependency**: ADR-009 adopts vanilla-extract. Tailwind conflicts at the styling layer.

### Options Evaluated

| Library | Styles | React 19 | Tailwind? | Primitives | Verdict |
|---------|--------|----------|-----------|------------|---------|
| **Radix UI Primitives** | Unstyled | Yes (verified) | No | Dialog, DropdownMenu, Tooltip, Select, Checkbox, Switch, Tabs, Accordion, Popover, ScrollArea, + 15 more | **Selected** |
| shadcn/ui | Tailwind (bundled) | Yes | Required | Same as Radix (it wraps Radix) | Rejected — Tailwind violates ADR-009 |
| Headless UI | Unstyled | Yes | No (but Tailwind-first docs) | Dialog, Combobox, Listbox, Tabs, Disclosure, Menu, Popover, RadioGroup, Switch, Transition | Rejected — inferior coverage, Tailwind-first ecosystem |
| Ark UI | Unstyled (Zag.js) | Yes | No | Comparable to Radix | Rejected — younger project, Zag.js dependency adds surface area |
| React Aria (Adobe) | Unstyled | Yes | No | Comprehensive | Viable alternative; rejected because Radix's simpler composition model better matches the team's existing mental model |

### Radix UI + Next.js App Router Constraint

All interactive Radix UI components require `"use client"` because they use browser APIs (`useEffect`, `useRef`, focus management, event listeners). This is expected and well-documented in the Radix SSR guide.

The pattern for handling this in Next.js 15 App Router is:

```tsx
// components/ui/Modal/Modal.tsx
"use client";
import * as Dialog from '@radix-ui/react-dialog';
// ... wrap Dialog.* with vanilla-extract styled elements
export function Modal({ open, onOpenChange, children, title }) {
  return (
    <Dialog.Root open={open} onOpenChange={onOpenChange}>
      <Dialog.Portal>
        <Dialog.Overlay className={overlay} />
        <Dialog.Content className={content}>
          <Dialog.Title>{title}</Dialog.Title>
          {children}
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}
```

Server Components that need to open a modal pass `open` and `onOpenChange` as props — they do not import Radix directly. This is the standard RSC composition pattern and adds negligible complexity.

## Decision

Adopt **Radix UI Primitives** (`@radix-ui/react-*`) as the headless component foundation for all interactive UI elements in `web-app/`.

### Scope of adoption

**Phase 1 (immediate)**: Install `@radix-ui/react-dialog` and `@radix-ui/react-slot`. Build 5 primitives: `Button` (uses Slot for `asChild`), `Badge`, `Input`, `Card`, `Modal` (Dialog wrapper).

**Phase 2 onwards (as needed)**: Add `@radix-ui/react-dropdown-menu`, `@radix-ui/react-select`, `@radix-ui/react-tooltip`, `@radix-ui/react-tabs` when building components that require them. Install each package individually — do not install the entire `@radix-ui` namespace upfront.

### File structure

```
web-app/src/components/ui/
  Button/
    Button.tsx        ← "use client"; wraps @radix-ui/react-slot
    Button.css.ts     ← recipe() with intent + size variants
    Button.test.tsx
  Modal/
    Modal.tsx         ← "use client"; wraps @radix-ui/react-dialog
    Modal.css.ts
    Modal.test.tsx
  index.ts            ← re-exports all primitives
```

### Usage rule

All new interactive components **must** compose from `components/ui/` primitives rather than using native HTML elements directly, unless the component is a thin semantic wrapper (e.g., a `<section>` layout element). This rule is enforced by code review, not by tooling.

## Consequences

### Positive
- Radix handles focus trapping, scroll locking, keyboard navigation, and ARIA semantics for all interactive patterns — these are no longer hand-implemented per component
- `asChild` prop (via Radix Slot) allows `Button` to render as `<a>`, `<Link>`, or any element without losing styles
- Installing packages individually keeps bundle impact proportional to what is actually used
- Zero Tailwind dependency — fully compatible with ADR-009

### Negative / Constraints
- Every interactive primitive requires a `"use client"` wrapper file — adds one file per primitive type (low overhead; 10–15 files total)
- Radix does not provide layout or data display primitives (tables, lists, grids) — those are vanilla HTML + vanilla-extract
- Upgrading Radix major versions may require updating wrapper components if the underlying prop API changes (mitigated by keeping wrappers thin)

## References
- Radix UI Primitives: https://www.radix-ui.com/primitives
- Radix SSR / Next.js App Router guide: https://www.radix-ui.com/primitives/docs/guides/server-side-rendering
- ADR-009: vanilla-extract for Type-Safe CSS (`docs/adr/009-vanilla-extract-type-safe-css.md`)
- Research: `project_plans/front-end-refactor/research/findings-stack.md`
