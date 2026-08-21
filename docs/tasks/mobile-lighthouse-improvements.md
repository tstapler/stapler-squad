# Mobile-Friendliness and Lighthouse Improvements

**Date**: 2026-03-17
**Epic**: Make the Stapler Squad web application fully responsive, accessible, and performant across all devices and screen sizes.

---

## Epic Overview

**Goal**: Resolve all critical and high-priority mobile layout breakages, close WCAG 2.1 AA accessibility gaps, and systematically improve Lighthouse scores across Performance, Accessibility, Best Practices, and SEO categories.

**Value Proposition**:
- Enable full session monitoring and management on mobile devices
- Meet WCAG 2.1 AA compliance for all interactive components
- Improve Lighthouse scores to enable better discoverability and trust signals
- Reduce support burden caused by broken mobile layouts
- Establish responsive design patterns for all future components

**Success Metrics**:
- Lighthouse Accessibility score: 60 → 90+
- Lighthouse SEO score: 70 → 95+
- Lighthouse Best Practices score: 75 → 95+
- All touch targets meet 44x44px minimum
- No horizontal overflow on any page at 375px viewport width
- WCAG 2.1 AA compliance verified for all interactive elements
- Zero `console.log` statements in production bundle

**Estimated Lighthouse Score Improvements**:

| Category       | Current (est.) | Target | Key Drivers                                    |
|----------------|---------------|--------|------------------------------------------------|
| Accessibility  | ~60           | 90+    | ARIA labels, focus management, color contrast  |
| SEO            | ~70           | 95+    | Viewport meta tag, page meta descriptions      |
| Best Practices | ~75           | 95+    | console.log removal, touch targets, no overflow|
| Performance    | ~80           | 85+    | Reduced motion support, virtual scrolling      |

---

## Quick Wins (5 minutes or less each)

These items can be completed independently in a single sitting and have immediate Lighthouse impact.

| Item                          | File                                  | Time | Lighthouse Impact          |
|-------------------------------|---------------------------------------|------|----------------------------|
| Add viewport meta tag         | `web-app/src/app/layout.tsx`          | 2m   | SEO +10, Accessibility +5  |
| Add `aria-label` to `<nav>`   | `web-app/src/components/Header.tsx`   | 1m   | Accessibility +3           |
| Remove `console.log` calls    | `web-app/src/components/Header.tsx`   | 2m   | Best Practices +5          |
| Add `aria-hidden` to emoji    | Multiple components                   | 5m   | Accessibility +2           |
| Add `prefers-reduced-motion`  | `web-app/src/app/globals.css`         | 3m   | Accessibility +2           |

---

## Dependency Visualization

```
Phase 1 (Critical - unblocked)
├── Task 1.1: Viewport Meta Tag (2h) ──────────────────────────────→ unblocks SEO tasks
├── Task 1.2: Header Mobile Navigation (3h) ───────────────────────→ unblocks Task 2.4
├── Task 1.3: Session Card Button Overflow (2h) ───────────────────→ independent
└── Task 1.4: Filter Bar Mobile Collapse (3h) ─────────────────────→ independent

Phase 2 (High Priority - mostly independent)
├── Task 2.1: Skip-to-Content Link (1h) ───────────────────────────→ independent
├── Task 2.2: Main Landmark on All Pages (2h) ─────────────────────→ independent
├── Task 2.3: Nav aria-label (1h) ─────────────────────────────────→ independent
├── Task 2.4: Dark Mode Color Contrast (2h) ───────────────────────→ independent
├── Task 2.5: Modal Focus Trapping (4h) ───────────────────────────→ independent
├── Task 2.6: Filter Input aria-labels (1h) ───────────────────────→ independent
└── Task 2.7: Config Page Mobile Layout (2h) ──────────────────────→ independent

Phase 3 (Medium Priority)
├── Task 3.1: Touch Target Sizes (2h) ─────────────────────────────→ independent
├── Task 3.2: Page Meta Descriptions (1h) ─────────────────────────→ independent
├── Task 3.3: Loading Skeleton Component (2h) ─────────────────────→ independent
├── Task 3.4: Emoji to SVG Icons (3h) ─────────────────────────────→ independent
├── Task 3.5: Remove console.log (1h) ─────────────────────────────→ independent
└── Task 3.6: Modal Height Responsive Fix (1h) ────────────────────→ independent

Phase 4 (Low Priority)
├── Task 4.1: prefers-reduced-motion (1h) ─────────────────────────→ independent
├── Task 4.2: rem Font Sizes (2h) ─────────────────────────────────→ independent
├── Task 4.3: Design Token Alignment (2h) ─────────────────────────→ independent
├── Task 4.4: Hide Header on Login (1h) ───────────────────────────→ independent
└── Task 4.5: Virtual Scrolling for Large Lists (4h) ──────────────→ independent
```

---

## Phase 1: Critical Issues

These items cause visible layout breakages or block Lighthouse scores from reaching acceptable thresholds.

---

### Task 1.1: Add Viewport Meta Tag (2h) [Small]

**Scope**: Export the Next.js `viewport` metadata object from the root layout so the browser renders pages correctly on mobile devices.

**Files**:
- `web-app/src/app/layout.tsx` (modify)

**Context**:
- Next.js 13+ App Router uses exported `viewport` objects rather than direct `<meta>` tags
- The existing `metadata` export in layout.tsx is present but viewport is missing
- Without this tag, mobile browsers render at desktop width and scale down, causing Lighthouse SEO and Accessibility failures

**Implementation**:
```tsx
// Add alongside the existing metadata export in layout.tsx
export const viewport = {
  width: 'device-width',
  initialScale: 1,
  maximumScale: 5,
};
```

**Acceptance Criteria**:
- Chrome DevTools mobile emulation at 375px shows correct initial scale
- Lighthouse SEO audit passes "Has a viewport meta tag" check
- Page does not render at 980px width on mobile devices

**Lighthouse Impact**: SEO +10, Accessibility +5

**Status**: Pending

---

### Task 1.2: Header Navigation Mobile Hamburger Menu (3h) [Medium]

**Scope**: Replace the full horizontal nav link list with a hamburger menu below 768px that collapses into a slide-out or dropdown drawer.

**Files**:
- `web-app/src/components/Header.tsx` (modify)
- `web-app/src/components/Header.module.css` (modify)

**Context**:
- Current header renders all nav links horizontally; at 375px they overflow and push critical action buttons off screen
- The hamburger trigger must itself be a minimum 44x44px touch target
- Keep only the most critical action (e.g., new session button) always visible in the mobile header bar
- The nav drawer must trap focus when open and close on Escape

**Implementation**:
```tsx
// In Header.tsx
const [menuOpen, setMenuOpen] = useState(false);

// Render hamburger button below 768px (CSS class controls visibility)
<button
  className={styles.hamburger}
  aria-label="Open navigation menu"
  aria-expanded={menuOpen}
  aria-controls="mobile-nav"
  onClick={() => setMenuOpen(prev => !prev)}
>
  {/* SVG hamburger/close icon */}
</button>

// Nav links in a conditionally visible drawer
<nav
  id="mobile-nav"
  aria-label="Main navigation"
  className={`${styles.nav} ${menuOpen ? styles.navOpen : ''}`}
>
  {/* existing nav links */}
</nav>
```

```css
/* Header.module.css additions */
.hamburger {
  display: none;
  min-width: 44px;
  min-height: 44px;
  align-items: center;
  justify-content: center;
}

@media (max-width: 768px) {
  .hamburger { display: flex; }
  .nav { display: none; position: absolute; top: 100%; left: 0; right: 0; }
  .navOpen { display: flex; flex-direction: column; }
}
```

**Acceptance Criteria**:
- At 375px, header shows hamburger button and no overflowing nav links
- Hamburger button is 44x44px minimum touch target
- Menu opens and closes correctly
- Menu closes when Escape is pressed
- Lighthouse Accessibility audit passes touch target size check for nav

**Lighthouse Impact**: Accessibility +5, Best Practices +3

**Status**: Pending

---

### Task 1.3: Session Card Action Button Overflow on Mobile (2h) [Small]

**Scope**: Add responsive media queries to SessionCard so action buttons wrap into a grid or overflow menu instead of overflowing horizontally at 375px.

**Files**:
- `web-app/src/components/sessions/SessionCard.module.css` (modify)
- `web-app/src/components/sessions/SessionCard.tsx` (modify if needed)

**Context**:
- `SessionCard.module.css` currently has zero media queries
- Action buttons (approve, deny, pause, resume, delete, etc.) render in a single row; at mobile widths they overflow the card boundary
- All touch targets must be a minimum of 44x44px
- Button labels can be hidden on small screens with an `aria-label` providing the accessible name

**Implementation**:
```css
/* Add to SessionCard.module.css */
@media (max-width: 768px) {
  .actions {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(44px, 1fr));
    gap: 8px;
    width: 100%;
  }

  .actionButton {
    min-width: 44px;
    min-height: 44px;
    padding: 10px;
    justify-content: center;
  }

  .actionButtonLabel {
    display: none; /* hide text labels, keep aria-label */
  }
}
```

**Acceptance Criteria**:
- At 375px, no horizontal scrollbar appears within a session card
- All action buttons are at least 44x44px on mobile
- Action button icons render with descriptive `aria-label` attributes when labels are hidden
- Lighthouse Accessibility audit passes touch target check

**Lighthouse Impact**: Accessibility +4, Best Practices +2

**Status**: Pending

---

### Task 1.4: Filter Bar Mobile Collapse (3h) [Medium]

**Scope**: Collapse the session list filter bar behind a "Filters" toggle button on mobile so the filter controls do not overflow horizontally at 375px. The search input remains always visible.

**Files**:
- `web-app/src/components/sessions/SessionList.tsx` (modify)
- `web-app/src/components/sessions/SessionList.module.css` (modify)

**Context**:
- The filter bar renders status, category, tag, grouping, and sort controls inline; at 375px they overflow the viewport
- Search input should always be visible as the primary discovery mechanism
- Active filters should be shown as dismissible chips below the search bar to maintain awareness
- The "Filters" toggle button must itself be a 44x44px touch target

**Implementation**:
```tsx
// In SessionList.tsx
const [filtersOpen, setFiltersOpen] = useState(false);
const hasActiveFilters = status !== 'all' || category || tag;

// Render:
<div className={styles.filterBar}>
  <input type="search" ... /> {/* always visible */}
  <button
    className={styles.filterToggle}
    aria-expanded={filtersOpen}
    aria-label={`${hasActiveFilters ? 'Active filters, ' : ''}Toggle filters`}
    onClick={() => setFiltersOpen(prev => !prev)}
  >
    Filters {hasActiveFilters && <span className={styles.filterBadge} />}
  </button>
  <div className={`${styles.filterControls} ${filtersOpen ? styles.open : ''}`}>
    {/* status, category, tag, group-by, sort selects */}
  </div>
  {hasActiveFilters && (
    <div className={styles.activeFilterChips}>
      {/* dismissible chips for each active filter */}
    </div>
  )}
</div>
```

```css
/* SessionList.module.css additions */
@media (max-width: 768px) {
  .filterControls {
    display: none;
    flex-direction: column;
    width: 100%;
    gap: 8px;
    padding: 8px 0;
  }
  .filterControls.open { display: flex; }

  .filterToggle {
    min-height: 44px;
    padding: 0 16px;
  }
}
```

**Acceptance Criteria**:
- At 375px, no horizontal overflow in the filter bar area
- Search input is always visible
- "Filters" button shows a visual indicator when filters are active
- Active filter chips are visible and dismissible below the search bar
- Filter controls expand/collapse correctly
- All controls meet 44x44px touch target minimum

**Lighthouse Impact**: Best Practices +3, Accessibility +2

**Status**: Pending

---

## Phase 2: High Priority

Accessibility compliance gaps and layout issues that significantly degrade user experience but do not completely prevent use.

---

### Task 2.1: Skip-to-Content Link (1h) [Micro]

**Scope**: Add a visually-hidden skip link as the first focusable element on every page so keyboard users can bypass the repeated header navigation.

**Files**:
- `web-app/src/app/layout.tsx` (modify)
- `web-app/src/app/globals.css` (modify)

**Context**:
- WCAG 2.4.1 (Level A) requires a mechanism to skip blocks of repeated content
- The skip link should be visually hidden until focused, then appear as a visible button
- The target anchor (`id="main-content"`) must be added to the primary `<main>` element on each page

**Implementation**:
```tsx
// In layout.tsx, as first child of <body>
<a href="#main-content" className="skip-link">
  Skip to main content
</a>
```

```css
/* In globals.css */
.skip-link {
  position: absolute;
  top: -100%;
  left: 0;
  padding: 8px 16px;
  background: var(--background);
  color: var(--foreground);
  font-weight: 600;
  z-index: 9999;
  border: 2px solid var(--accent);
  border-radius: 0 0 4px 0;
  text-decoration: none;
}
.skip-link:focus {
  top: 0;
}
```

**Acceptance Criteria**:
- Pressing Tab on any page reveals the skip link as the first focused element
- Clicking/activating the skip link moves focus to `id="main-content"`
- Skip link is not visible to mouse users until focused
- Lighthouse Accessibility audit passes "Bypass Blocks" check

**Lighthouse Impact**: Accessibility +3

**Status**: Pending

---

### Task 2.2: Add `<main>` Landmark to All Pages (2h) [Small]

**Scope**: Ensure every page wraps its primary content in a `<main id="main-content">` element so screen readers and the skip link have a proper landmark to navigate to.

**Files**:
- `web-app/src/app/config/page.tsx` (modify)
- `web-app/src/app/login/page.tsx` (modify)
- Any other pages missing a `<main>` element (audit during implementation)

**Context**:
- WCAG 1.3.1 requires content to have correct semantic structure
- Pages that render only a `<div>` as root lack the `<main>` landmark
- The `id="main-content"` attribute ties to the skip link from Task 2.1 (can proceed independently)

**Implementation**:
```tsx
// Replace top-level <div> wrapper with <main>
export default function ConfigPage() {
  return (
    <main id="main-content" className={styles.container}>
      {/* existing content */}
    </main>
  );
}
```

**Acceptance Criteria**:
- Accessibility tree (Chrome DevTools) shows a `<main>` landmark on all pages
- Screen reader can navigate to main content via landmarks list
- Lighthouse Accessibility audit passes "Document has a main landmark" check

**Lighthouse Impact**: Accessibility +4

**Status**: Pending

---

### Task 2.3: Header Nav `aria-label` (1h) [Micro]

**Scope**: Add `aria-label="Main navigation"` to the `<nav>` element in Header.tsx so screen readers can distinguish it from other navigation regions on the page.

**Files**:
- `web-app/src/components/Header.tsx` (modify)

**Context**:
- WCAG 1.3.1 requires navigation landmarks to be distinguishable
- When multiple `<nav>` elements exist on a page, each must have an accessible label
- This is also required for the mobile nav drawer added in Task 1.2

**Implementation**:
```tsx
<nav aria-label="Main navigation" className={styles.nav}>
  {/* existing nav links */}
</nav>
```

**Acceptance Criteria**:
- Screen reader announces "Main navigation" when entering the nav landmark
- Lighthouse Accessibility audit passes "Navigation has accessible name" check

**Lighthouse Impact**: Accessibility +2

**Status**: Pending

---

### Task 2.4: Dark Mode Color Contrast Fix (2h) [Small]

**Scope**: Audit all text/background color combinations in dark mode and fix any that fail WCAG 1.4.3 AA contrast ratio (4.5:1 for normal text, 3:1 for large text). Start with `--text-disabled` which is known to fail.

**Files**:
- `web-app/src/app/globals.css` (modify)
- `web-app/src/components/sessions/SessionCard.module.css` (audit)
- `web-app/src/components/ui/NotificationPanel.module.css` (audit)

**Context**:
- WCAG 1.4.3 Contrast Minimum (Level AA) requires 4.5:1 ratio for body text
- `--text-disabled` in dark mode is known to fall below this threshold
- Use browser DevTools accessibility panel or a tool like axe to identify all failures
- Target value for `--text-disabled` in dark mode: minimum `#767676` on dark backgrounds

**Implementation**:
```css
/* In globals.css, dark mode overrides */
@media (prefers-color-scheme: dark) {
  :root {
    --text-disabled: #767676; /* was lower contrast value */
    /* audit and update other failing color pairs */
  }
}

[data-theme="dark"] {
  --text-disabled: #767676;
  /* ... */
}
```

**Acceptance Criteria**:
- All text/background pairs in dark mode meet 4.5:1 contrast ratio (WCAG AA)
- axe or Lighthouse reports zero contrast failures in dark mode
- Disabled state text remains visually distinct from enabled state

**Lighthouse Impact**: Accessibility +5

**Status**: Pending

---

### Task 2.5: Modal Focus Trapping (4h) [Large]

**Scope**: Implement proper focus trapping in all modal dialogs so keyboard users cannot tab outside an open modal, and focus returns to the trigger element when the modal closes.

**Files**:
- `web-app/src/components/ui/Modal.tsx` (modify or create shared hook)
- Any modal components that do not use the shared Modal component (audit)

**Context**:
- WCAG 2.4.3 (Focus Order) and 2.1.2 (No Keyboard Trap in positive sense) require modals to trap focus while open
- The modal must use `role="dialog"` and `aria-modal="true"`
- On open: focus must move to the first interactive element inside the modal
- On close: focus must return to the element that triggered the modal
- Tab and Shift+Tab must cycle through focusable elements inside the modal only

**Implementation**:
```tsx
// Shared useFocusTrap hook in web-app/src/lib/hooks/useFocusTrap.ts
export function useFocusTrap(ref: RefObject<HTMLElement>, isActive: boolean) {
  useEffect(() => {
    if (!isActive || !ref.current) return;
    const focusable = ref.current.querySelectorAll(
      'a[href], button:not([disabled]), input, select, textarea, [tabindex]:not([tabindex="-1"])'
    );
    const first = focusable[0] as HTMLElement;
    const last = focusable[focusable.length - 1] as HTMLElement;
    first?.focus();

    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key !== 'Tab') return;
      if (e.shiftKey) {
        if (document.activeElement === first) { e.preventDefault(); last?.focus(); }
      } else {
        if (document.activeElement === last) { e.preventDefault(); first?.focus(); }
      }
    };

    document.addEventListener('keydown', handleKeyDown);
    return () => document.removeEventListener('keydown', handleKeyDown);
  }, [isActive, ref]);
}

// In Modal.tsx
<div role="dialog" aria-modal="true" aria-labelledby={titleId} ref={dialogRef}>
  {/* content */}
</div>
```

**Acceptance Criteria**:
- Tab and Shift+Tab cycle only through elements inside an open modal
- Focus moves to first interactive element when modal opens
- Focus returns to the triggering element when modal closes
- Pressing Escape closes the modal
- Lighthouse Accessibility audit reports no focus management failures

**Lighthouse Impact**: Accessibility +6

**Status**: Pending

---

### Task 2.6: Filter Input Accessible Labels (1h) [Micro]

**Scope**: Add `aria-label` attributes to all filter and sort controls in the session list filter bar that currently lack visible or programmatic labels.

**Files**:
- `web-app/src/components/sessions/SessionList.tsx` (modify)

**Context**:
- WCAG 4.1.2 Name, Role, Value requires all form controls to have accessible names
- Controls currently rendered as unlabeled `<select>` or `<input>` elements fail this requirement
- Visible labels are preferred; if space is constrained, `aria-label` is acceptable

**Implementation**:
```tsx
<input
  type="search"
  aria-label="Search sessions"
  placeholder="Search..."
  ...
/>

<select aria-label="Filter by status" ...>
<select aria-label="Filter by category" ...>
<select aria-label="Filter by tag" ...>
<select aria-label="Group sessions by" ...>
<select aria-label="Sort sessions by" ...>
```

**Acceptance Criteria**:
- All filter controls have an accessible name visible to screen readers
- Lighthouse Accessibility audit passes form label checks for all filter inputs
- axe reports zero "Form elements must have labels" violations

**Lighthouse Impact**: Accessibility +3

**Status**: Pending

---

### Task 2.7: Config Page Mobile Layout (2h) [Small]

**Scope**: Add a responsive breakpoint to the config page so the file list and Monaco editor stack vertically on mobile instead of rendering side-by-side with overflow.

**Files**:
- `web-app/src/app/config/config.module.css` (modify)

**Context**:
- `config.module.css` currently has no responsive breakpoints
- The two-column layout (file tree + editor) breaks at mobile widths causing horizontal overflow
- Monaco editor has a minimum usable width; set to at least 300px
- On mobile, file list should appear above the editor in full-width column layout

**Implementation**:
```css
/* Add to config.module.css */
@media (max-width: 768px) {
  .configLayout {
    flex-direction: column;
  }

  .fileList {
    width: 100%;
    max-height: 200px;
    overflow-y: auto;
    border-right: none;
    border-bottom: 1px solid var(--border);
  }

  .editorContainer {
    width: 100%;
    min-width: 300px;
    flex: 1;
  }
}
```

**Acceptance Criteria**:
- At 375px, config page shows file list above the editor with no horizontal overflow
- Monaco editor renders at a minimum of 300px width
- File list is scrollable and shows all files
- No horizontal scrollbar appears on the config page at any viewport width above 320px

**Lighthouse Impact**: Best Practices +2

**Status**: Pending

---

## Phase 3: Medium Priority

Quality-of-life improvements and polish items that improve overall usability and production readiness.

---

### Task 3.1: Touch Target Sizes for Notification and Review Queue (2h) [Small]

**Scope**: Increase the touch target sizes of small interactive elements in the Notification Panel and Review Queue filter buttons to meet the 44x44px minimum.

**Files**:
- `web-app/src/components/ui/NotificationPanel.module.css` (modify)
- Review Queue filter button styles (identify file during implementation)

**Context**:
- `.removeButton` in NotificationPanel is currently 24px, failing the 44x44px touch target requirement
- On mobile, `.removeButton` opacity should be 1 (currently hidden until hover, which has no mobile equivalent)
- Review Queue filter buttons need padding increased to achieve 44px height

**Implementation**:
```css
/* NotificationPanel.module.css */
.removeButton {
  min-width: 44px;
  min-height: 44px;
  display: flex;
  align-items: center;
  justify-content: center;
}

@media (max-width: 768px) {
  .removeButton {
    opacity: 1; /* always visible on touch devices */
  }

  .approveButton,
  .denyButton {
    padding: 12px 16px;
    min-height: 44px;
  }
}

/* Review Queue filter buttons */
@media (max-width: 768px) {
  .filterButton {
    padding: 12px 16px;
    min-height: 44px;
  }
}
```

**Acceptance Criteria**:
- All interactive elements in NotificationPanel and Review Queue are at least 44x44px on mobile
- Remove buttons are visible (opacity: 1) on mobile without requiring hover
- Lighthouse Accessibility audit passes touch target size check for these components

**Lighthouse Impact**: Accessibility +3

**Status**: Pending

---

### Task 3.2: Page Meta Descriptions (1h) [Micro]

**Scope**: Add `metadata` exports with `description` fields to all pages that currently lack them.

**Files**:
- `web-app/src/app/config/page.tsx` (modify)
- `web-app/src/app/login/page.tsx` (modify)
- Any other app pages missing metadata (audit during implementation)

**Context**:
- Next.js App Router uses exported `metadata` objects for `<meta name="description">` tags
- Lighthouse SEO audit flags pages missing meta descriptions
- Descriptions should be 120-158 characters and accurately describe page content

**Implementation**:
```tsx
// config/page.tsx
export const metadata = {
  title: 'Configuration - Stapler Squad',
  description: 'Configure Stapler Squad settings including agent programs, tmux prefix, and log levels.',
};

// login/page.tsx
export const metadata = {
  title: 'Sign In - Stapler Squad',
  description: 'Sign in to Stapler Squad to manage your AI agent sessions.',
};
```

**Acceptance Criteria**:
- All pages have a unique, descriptive `<meta name="description">` tag
- Lighthouse SEO audit passes "Document has a meta description" check for all pages

**Lighthouse Impact**: SEO +5

**Status**: Pending

---

### Task 3.3: Loading Skeleton for Review Queue Suspense (2h) [Small]

**Scope**: Replace the unstyled `<div>Loading...</div>` Suspense fallback in the Review Queue page with a properly styled skeleton component that matches the application's loading patterns.

**Files**:
- Review Queue page component (identify path during implementation)
- `web-app/src/components/ui/Skeleton.tsx` (create if does not exist)

**Context**:
- Unstyled loading states create jarring layout shifts and a poor first impression
- The skeleton should match the approximate dimensions of the loaded content
- If a shared `Skeleton` component already exists, use it; otherwise create a minimal one

**Implementation**:
```tsx
// Skeleton.tsx (if not already present)
export function Skeleton({ className }: { className?: string }) {
  return <div className={`${styles.skeleton} ${className}`} aria-hidden="true" />;
}

// In review queue page
<Suspense fallback={<ReviewQueueSkeleton />}>
  <ReviewQueueContent />
</Suspense>
```

**Acceptance Criteria**:
- Review Queue Suspense boundary shows a styled skeleton instead of plain text
- Skeleton matches the approximate layout of the loaded content
- No layout shift occurs when content loads after the skeleton
- `aria-hidden="true"` on skeleton elements (purely visual, not meaningful to screen readers)

**Lighthouse Impact**: Best Practices +2, Performance +1

**Status**: Pending

---

### Task 3.4: Replace UI-Critical Emoji with SVG Icons (3h) [Medium]

**Scope**: Replace emoji used for UI affordance (navigation, status, actions) with SVG icons and add `aria-hidden="true"` to any remaining decorative emoji.

**Files**:
- `web-app/src/components/Header.tsx` (modify)
- `web-app/src/components/sessions/SessionCard.tsx` (modify)
- Other components using UI-critical emoji (audit during implementation)

**Context**:
- Emoji render inconsistently across platforms and OS versions
- UI-critical emoji (icons indicating actions or status) must have text alternatives
- Decorative emoji (purely visual flavor) should have `aria-hidden="true"` and nearby visible text
- Use inline SVG or an existing icon library already in the project

**Implementation**:
```tsx
// Before: emoji without label
<button>🛠️</button>

// After: SVG with accessible label
<button aria-label="Settings">
  <SettingsIcon aria-hidden="true" />
</button>

// Before: decorative emoji
<span>✅ Completed</span>

// After: decorative emoji hidden from screen readers
<span>
  <span aria-hidden="true">✅</span>
  <span> Completed</span>
</span>
```

**Acceptance Criteria**:
- All UI-critical emoji replaced with SVG icons that render consistently across platforms
- All decorative emoji have `aria-hidden="true"` with adjacent visible text
- No Lighthouse Accessibility warnings related to emoji usage
- Visual appearance is equivalent or improved across macOS, Windows, Android, iOS

**Lighthouse Impact**: Accessibility +2, Best Practices +2

**Status**: Pending

---

### Task 3.5: Remove console.log from Production Bundle (1h) [Micro]

**Scope**: Remove all `console.log` calls from onClick handlers and other production code in Header.tsx and any other component files.

**Files**:
- `web-app/src/components/Header.tsx` (modify - lines 33, 40, 47, 54, 61)

**Context**:
- Lighthouse Best Practices audit flags `console.log` in production code
- These log statements appear to be debugging artifacts from development
- If logging is needed in production, use a structured logger or remove entirely

**Implementation**:
Remove the `console.log` calls on Header.tsx lines 33, 40, 47, 54, and 61. If any logging is necessary for diagnostics, replace with a conditional logger that is stripped in production builds.

**Acceptance Criteria**:
- Zero `console.log` calls in production JavaScript bundle for navigation handlers
- Lighthouse Best Practices audit passes "Does not use console.log" check
- All existing navigation functionality continues to work correctly

**Lighthouse Impact**: Best Practices +5

**Status**: Pending

---

### Task 3.6: Modal Content Height Responsive Fix (1h) [Micro]

**Scope**: Increase the default modal height from 60vh to 80vh on desktop, and use `calc(100vh - 5rem)` on mobile so modal content is not unnecessarily constrained.

**Files**:
- `web-app/src/components/ui/Modal.tsx` or the relevant CSS module (modify)

**Context**:
- The current 60vh height causes content to scroll unnecessarily on desktop and clips content on mobile
- On mobile, modals should use nearly the full viewport height to maximize usable space
- Ensure the modal is still scrollable if content exceeds the available height

**Implementation**:
```css
.modalContent {
  max-height: 80vh;
  overflow-y: auto;
}

@media (max-width: 768px) {
  .modalContent {
    max-height: calc(100vh - 5rem);
    border-radius: 12px 12px 0 0; /* bottom sheet style on mobile */
  }
}
```

**Acceptance Criteria**:
- Desktop modals use up to 80vh height before scrolling
- Mobile modals use nearly full viewport height
- Modals are scrollable when content overflows
- No content is clipped by an unnecessarily small modal height

**Lighthouse Impact**: Best Practices +1

**Status**: Pending

---

## Phase 4: Low Priority

Foundational improvements and future-proofing items. These address technical debt and edge cases.

---

### Task 4.1: prefers-reduced-motion Support (1h) [Micro]

**Scope**: Add a global `prefers-reduced-motion` media query to `globals.css` that disables or reduces all CSS transitions and animations for users who have requested reduced motion in their OS settings.

**Files**:
- `web-app/src/app/globals.css` (modify)

**Context**:
- WCAG 2.3.3 (AAA) and Lighthouse Accessibility recommend respecting this system preference
- Vestibular disorders can cause nausea from excessive animation
- A single global rule is the safest and lowest-effort approach

**Implementation**:
```css
/* Add to globals.css */
@media (prefers-reduced-motion: reduce) {
  *,
  *::before,
  *::after {
    animation-duration: 0.01ms !important;
    animation-iteration-count: 1 !important;
    transition-duration: 0.01ms !important;
    scroll-behavior: auto !important;
  }
}
```

**Acceptance Criteria**:
- When OS reduced motion preference is enabled, all CSS animations and transitions are disabled
- Page content remains fully functional without animations
- Lighthouse Accessibility audit passes "Respects prefers-reduced-motion" check

**Lighthouse Impact**: Accessibility +2

**Status**: Pending

---

### Task 4.2: Convert Fixed px Font Sizes to rem (2h) [Small]

**Scope**: Audit all CSS files for fixed `px` font-size declarations and convert them to `rem` equivalents so user browser font size preferences are respected.

**Files**:
- `web-app/src/app/globals.css` (modify)
- `web-app/src/components/sessions/SessionCard.module.css` (modify)
- Other component CSS modules with `font-size` in `px` (audit during implementation)

**Context**:
- Fixed `px` font sizes override user browser zoom preferences and OS accessibility font settings
- `1rem` equals the browser default font size (typically 16px)
- Use `clamp()` for responsive type that scales with both viewport and user preference where appropriate

**Implementation**:
```css
/* Before */
.title { font-size: 16px; }
.label { font-size: 12px; }

/* After */
.title { font-size: 1rem; }
.label { font-size: 0.75rem; }

/* Responsive type with clamp() */
.heading { font-size: clamp(1rem, 2.5vw, 1.5rem); }
```

**Acceptance Criteria**:
- All `font-size` declarations use `rem` or `em` units (no raw `px` for font sizes)
- Increasing browser default font size to 20px causes all text to scale proportionally
- No layout breakage occurs when font size is increased 200%

**Lighthouse Impact**: Accessibility +1

**Status**: Pending

---

### Task 4.3: Notification Panel Design Token Alignment (2h) [Small]

**Scope**: Audit the CSS custom properties used in `NotificationPanel.module.css` and replace any locally-defined or drifted values with the corresponding global design tokens from `globals.css`.

**Files**:
- `web-app/src/components/ui/NotificationPanel.module.css` (modify)
- `web-app/src/app/globals.css` (reference)

**Context**:
- Design token drift causes inconsistent colors and spacing compared to the rest of the application
- Dark mode may not apply correctly if local variables override global tokens
- Goal is for NotificationPanel to respond correctly to theme changes without local overrides

**Implementation**:
Audit all CSS variable references in `NotificationPanel.module.css`. For each local override, determine whether the global token (e.g., `--background`, `--foreground`, `--border`, `--accent`) provides the correct value. Replace local overrides with global tokens and remove variables that duplicate globals.

**Acceptance Criteria**:
- NotificationPanel colors match the rest of the application in both light and dark mode
- No locally-scoped CSS variables that duplicate or conflict with global tokens
- Dark mode toggle correctly updates all NotificationPanel colors

**Lighthouse Impact**: Best Practices +1

**Status**: Pending

---

### Task 4.4: Conditionally Hide Header on Login Page (1h) [Micro]

**Scope**: Prevent the Header component from rendering on the login page since it exposes navigation links that are inaccessible or irrelevant to unauthenticated users.

**Files**:
- `web-app/src/app/layout.tsx` (modify)
- `web-app/src/app/login/page.tsx` (reference)

**Context**:
- The current layout renders Header on all pages including login
- On the login page, nav links pointing to authenticated routes create confusion
- Next.js App Router supports route groups `(group)` to apply different layouts to different page sets
- Alternatively, a pathname check in layout.tsx can conditionally suppress the Header

**Implementation**:
Option A (pathname check in Client Component wrapper):
```tsx
// In a client component wrapping the Header
'use client';
import { usePathname } from 'next/navigation';
export function ConditionalHeader() {
  const pathname = usePathname();
  if (pathname === '/login') return null;
  return <Header />;
}
```

Option B (Next.js route groups): Move the login page into a `(auth)` route group with its own layout that omits the Header.

**Acceptance Criteria**:
- Header is not rendered on the `/login` page
- All other pages continue to render the Header normally
- No flash of Header content before it is hidden on the login page

**Lighthouse Impact**: Best Practices +1

**Status**: Pending

---

### Task 4.5: Virtual Scrolling for Large Session Lists (4h) [Large]

**Scope**: Implement virtual scrolling using `react-window` (or similar) for the session list so rendering performance does not degrade when the list exceeds 50 sessions.

**Files**:
- `web-app/src/components/sessions/SessionList.tsx` (modify)
- `web-app/src/components/sessions/SessionCard.tsx` (verify fixed height or measure)
- `web-app/package.json` (add dependency)

**Context**:
- Rendering 100+ session cards simultaneously causes significant DOM size and paint time
- Virtual scrolling renders only visible items plus a small overscan buffer
- `react-window` is a lightweight, well-tested solution; `@tanstack/react-virtual` is a dependency-free alternative
- Session cards need a consistent height (or height measurement) for virtualization to work
- Grouped views (by tag, category, etc.) require `VariableSizeList` if group headers have different heights

**Implementation**:
```bash
npm install react-window @types/react-window
```

```tsx
import { FixedSizeList } from 'react-window';

<FixedSizeList
  height={window.innerHeight - HEADER_HEIGHT - FILTER_BAR_HEIGHT}
  itemCount={filteredSessions.length}
  itemSize={SESSION_CARD_HEIGHT}
  width="100%"
>
  {({ index, style }) => (
    <div style={style}>
      <SessionCard session={filteredSessions[index]} />
    </div>
  )}
</FixedSizeList>
```

**Acceptance Criteria**:
- Session list with 200+ sessions renders in under 100ms
- Scrolling through 200+ sessions is smooth (60fps)
- All existing filter, search, and grouping functionality works with virtualized list
- Lighthouse Performance score does not regress for lists under 50 sessions

**Lighthouse Impact**: Performance +3 (for large lists)

**Status**: Pending

---

## Summary

| Phase | Tasks | Total Est. Hours | Lighthouse Categories Affected        |
|-------|-------|-----------------|---------------------------------------|
| 1 (Critical)       | 4 | 10h | SEO, Accessibility, Best Practices   |
| 2 (High Priority)  | 7 | 13h | Accessibility                         |
| 3 (Medium)         | 6 | 10h | Accessibility, Best Practices, SEO    |
| 4 (Low Priority)   | 5 |  9h | Accessibility, Best Practices, Performance |
| **Total**          | **22** | **42h** |                                  |

**Recommended execution order for maximum early Lighthouse gain**:
1. Task 1.1 (Viewport Meta Tag) - 2h, biggest SEO gain
2. Task 2.3 (Nav aria-label) - 1h, quick accessibility win
3. Task 3.5 (Remove console.log) - 1h, quick Best Practices gain
4. Task 2.1 (Skip-to-Content) - 1h, WCAG Level A compliance
5. Task 2.6 (Filter aria-labels) - 1h, WCAG Level AA compliance
6. Task 2.2 (Main landmark) - 2h, screen reader foundation
7. Task 1.3 (Session card overflow) - 2h, visible mobile fix
8. Task 1.4 (Filter bar collapse) - 3h, visible mobile fix
9. Task 1.2 (Header hamburger) - 3h, largest mobile layout fix
10. Task 2.5 (Modal focus trapping) - 4h, critical WCAG compliance
