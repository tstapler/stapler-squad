# Feature Plan: History Page UX Improvements

**Status**: Planning - Ready for Implementation
**Created**: 2025-12-05
**Epic**: History Browser UX Overhaul
**Product Value**: Bring History page to feature parity with Sessions page, improving usability, accessibility, and power user efficiency

---

## Executive Summary

The History Browser page (`/history`) currently lacks many features already implemented in the Sessions page, creating an inconsistent user experience. This feature plan addresses 23 identified usability issues across Nielsen's 10 Usability Heuristics and WCAG POUR principles (Perceivable, Operable, Understandable, Robust).

**Key Problems**:
- **No keyboard navigation** - Power users cannot use arrow keys, j/k, /, or Escape
- **Missing filtering and grouping** - Flat 100-entry list vs Sessions' 8 grouping modes
- **Poor error handling** - No retry buttons, unclear empty states
- **Accessibility gaps** - Modal lacks focus trap, ARIA attributes
- **Information overload** - Cards cram too much text, poor visual hierarchy

**Impact**:
- **Power Users Blocked**: No keyboard shortcuts frustrate experienced users
- **Context Switching Cost**: Inconsistency with Sessions page increases cognitive load
- **Accessibility Barriers**: Screen reader and keyboard-only users cannot use effectively
- **Scalability Issues**: 100-entry flat list overwhelming, no pagination

**Success Metrics**:
- 100% keyboard navigable (matches Sessions page)
- WCAG 2.1 AA compliance (audit verified)
- Grouping and filtering parity with Sessions page
- < 200ms search latency with loading feedback
- Error recovery without page refresh

---

## Table of Contents

1. [Usability Heuristics Violated](#usability-heuristics-violated)
2. [Epic Overview](#epic-overview)
3. [Story Breakdown](#story-breakdown)
4. [Atomic Task Decomposition](#atomic-task-decomposition)
5. [Testing Strategy](#testing-strategy)
6. [Known Issues and Mitigation](#known-issues-and-mitigation)
7. [Dependencies and Timeline](#dependencies-and-timeline)
8. [Context Preparation Guides](#context-preparation-guides)

---

## Usability Heuristics Violated

### Nielsen's 10 Usability Heuristics

**H1 - Visibility of System Status**:
- VIOLATED: No loading indicator during search (Issue #2)
- VIOLATED: No indication of 100-entry truncation (Issue #13)

**H3 - User Control and Freedom**:
- VIOLATED: No Escape key to close modal (Issue #5)
- VIOLATED: No retry button after errors (Issue #4)

**H4 - Consistency and Standards**:
- VIOLATED: Inconsistent with Sessions page features (Issues #6, #7)
- VIOLATED: Different keyboard behavior than rest of app

**H7 - Flexibility and Efficiency of Use**:
- VIOLATED: No keyboard navigation (Issue #1)
- VIOLATED: No bulk actions (Issue #9)

**H8 - Aesthetic and Minimalist Design**:
- VIOLATED: High information density on cards (Issue #8)
- VIOLATED: Verbose absolute dates vs relative time

**H9 - Help Users Recognize, Diagnose, and Recover from Errors**:
- VIOLATED: Poor empty state guidance (Issue #3)
- VIOLATED: No error recovery actions (Issue #4)

**H10 - Help and Documentation**:
- VIOLATED: No keyboard shortcuts help (Issue #19)

### WCAG POUR Principles

**Perceivable**:
- VIOLATED: Missing ARIA labels on modal (Issue #5)
- VIOLATED: No loading state feedback (Issue #2)
- VIOLATED: Poor contrast in dark mode (Issue #21)

**Operable**:
- VIOLATED: No keyboard navigation (Issue #1)
- VIOLATED: No focus trap in modal (Issue #5)
- VIOLATED: Missing skip links and focus management

**Understandable**:
- VIOLATED: Unclear empty states (Issue #3)
- VIOLATED: Error messages without recovery (Issue #4)

**Robust**:
- VIOLATED: Missing semantic HTML in modal
- VIOLATED: Screen reader support incomplete

---

## Epic Overview

**Epic Goal**: Achieve feature parity with Sessions page while maintaining accessibility and power user efficiency.

**Epic Value Proposition**:
- **Consistency**: Same keyboard shortcuts, filtering, grouping across all pages
- **Accessibility**: WCAG 2.1 AA compliant, screen reader friendly
- **Efficiency**: Power users can navigate without mouse, bulk actions reduce repetitive work
- **Scalability**: Pagination and grouping handle 1000+ history entries

**Epic Success Metrics**:
1. All keyboard shortcuts from Sessions page work in History page
2. Pass WCAG 2.1 AA automated audit (axe-core, Lighthouse)
3. Search latency < 200ms with loading feedback
4. User can recover from errors without page refresh
5. Mobile responsive at 375px, 768px, 1024px breakpoints

**Epic Completion Criteria**:
- [ ] All 5 stories completed (23 tasks)
- [ ] Manual accessibility audit passed
- [ ] Automated tests for keyboard navigation
- [ ] E2E tests for critical flows (search, filter, modal)
- [ ] Design review approval

---

## Story Breakdown

### Story 1: Keyboard Navigation and Accessibility (CRITICAL)

**Story Goal**: Enable full keyboard navigation and fix accessibility violations

**Story Value**: Unblocks power users, achieves WCAG 2.1 AA compliance

**Tasks**: 5 tasks (Issues #1, #2, #5, #19, #23)
- Task 1.1: Add keyboard navigation hook (arrow keys, j/k, /)
- Task 1.2: Implement loading states and feedback
- Task 1.3: Fix modal accessibility (focus trap, ARIA, Escape)
- Task 1.4: Add keyboard shortcuts help modal
- Task 1.5: Mobile responsive breakpoints

**Dependencies**: None (can start immediately)

**Estimated Effort**: 10 hours

---

### Story 2: Error Handling and Empty States (CRITICAL)

**Story Goal**: Provide clear error recovery and contextual guidance

**Story Value**: Reduces user frustration, improves error recovery time

**Tasks**: 2 tasks (Issues #3, #4)
- Task 2.1: Implement contextual empty states
- Task 2.2: Add error retry functionality

**Dependencies**: None (can start immediately)

**Estimated Effort**: 4 hours

---

### Story 3: Filtering and Grouping (HIGH PRIORITY)

**Story Goal**: Add multi-dimensional organization matching Sessions page

**Story Value**: Makes large history lists manageable, reduces cognitive load

**Tasks**: 7 tasks (Issues #6, #7, #11, #17)
- Task 3.1: Add filter bar component (model, date range, status)
- Task 3.2: Implement grouping strategies (8 modes matching Sessions)
- Task 3.3: Add autocomplete search with suggestions
- Task 3.4: Persist filter state in localStorage
- Task 3.5: Add clear all filters button
- Task 3.6: Implement "Group by" dropdown UI
- Task 3.7: Add filter combination logic

**Dependencies**: Story 1 (keyboard navigation for filter controls)

**Estimated Effort**: 14 hours

---

### Story 4: Information Architecture and Bulk Actions (HIGH PRIORITY)

**Story Goal**: Reduce information density and enable bulk operations

**Story Value**: Improves scannability, reduces repetitive actions

**Tasks**: 5 tasks (Issues #8, #9, #10, #12, #13)
- Task 4.1: Redesign history card with visual hierarchy
- Task 4.2: Implement bulk selection mode
- Task 4.3: Add action toolbar to detail panel
- Task 4.4: Add message search in modal
- Task 4.5: Implement pagination controls

**Dependencies**: Story 1 (keyboard navigation for selection)

**Estimated Effort**: 12 hours

---

### Story 5: Enhanced Features (MEDIUM/LOW PRIORITY)

**Story Goal**: Polish and additional features for power users

**Story Value**: Improves long-term usability, reduces friction

**Tasks**: 4 tasks (Issues #14, #15, #16, #18, #20, #21, #22)
- Task 5.1: Add read/unread indicators
- Task 5.2: Implement export functionality (JSON/Markdown)
- Task 5.3: Add progress count to loading state
- Task 5.4: Truncate UUIDs with copy button
- Task 5.5: Add syntax highlighting for code messages
- Task 5.6: Audit and fix dark mode contrast
- Task 5.7: Preserve scroll position on navigation

**Dependencies**: Stories 1-4 (foundation features)

**Estimated Effort**: 10 hours

---

## Atomic Task Decomposition

### Story 1: Keyboard Navigation and Accessibility

#### Task 1.1: Add Keyboard Navigation Hook (3h)

**Scope**: Implement `useKeyboard` hook with arrow navigation, j/k, /, Escape support

**Files** (4 files):
- `web-app/src/lib/hooks/useKeyboard.ts` (create)
- `web-app/src/app/history/page.tsx` (modify - integrate hook)
- `web-app/src/components/history/HistoryCard.tsx` (create - keyboard focus states)
- `web-app/src/app/history/history.module.css` (modify - focus styles)

**Context**:
- Reference: `web-app/src/components/sessions/SessionList.tsx` (lines 200-250 - keyboard handling)
- Pattern: Event listener with key mapping, focus management
- Integration: Hook returns `{ selectedIndex, handleKeyDown }` for component

**Implementation**:
```typescript
// useKeyboard.ts structure
export function useKeyboard(
  items: HistoryEntry[],
  onSelect: (entry: HistoryEntry) => void,
  onSearch: () => void
) {
  const [selectedIndex, setSelectedIndex] = useState(0);

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      switch (e.key) {
        case 'ArrowDown':
        case 'j':
          setSelectedIndex(prev => Math.min(prev + 1, items.length - 1));
          break;
        case 'ArrowUp':
        case 'k':
          setSelectedIndex(prev => Math.max(prev - 1, 0));
          break;
        case 'Enter':
          onSelect(items[selectedIndex]);
          break;
        case '/':
          e.preventDefault();
          onSearch();
          break;
        case 'Escape':
          // Close modal or clear search
          break;
      }
    };

    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [items, selectedIndex]);

  return { selectedIndex, setSelectedIndex };
}
```

**Success Criteria**:
- [ ] Arrow keys and j/k navigate through history entries
- [ ] Enter key opens selected entry detail modal
- [ ] / key focuses search input
- [ ] Escape key closes modal or clears search
- [ ] Keyboard focus visible with 2px outline
- [ ] Navigation wraps at top/bottom of list

**Testing**: Unit tests for key handlers, E2E test for full navigation flow

**Dependencies**: None

**Status**: Pending

---

#### Task 1.2: Implement Loading States and Feedback (2h)

**Scope**: Add loading indicator during search, disabled button states, skeleton loaders

**Files** (3 files):
- `web-app/src/app/history/page.tsx` (modify - add `isSearching` state)
- `web-app/src/app/history/history.module.css` (modify - loading animations)
- `web-app/src/components/shared/LoadingSpinner.tsx` (create or reuse existing)

**Context**:
- Reference: `web-app/src/components/sessions/SessionList.tsx` (lines 100-120 - loading states)
- Pattern: Boolean state flag, conditional rendering, CSS animations
- Nielsen H1: Visibility of system status

**Implementation**:
```typescript
// In page.tsx
const [isSearching, setIsSearching] = useState(false);

const handleSearch = async (query: string) => {
  setIsSearching(true);
  try {
    const results = await searchHistory(query);
    setEntries(results);
  } finally {
    setIsSearching(false);
  }
};

// In JSX
<button disabled={isSearching}>
  {isSearching ? <Spinner /> : 'Search'}
</button>

{isSearching && <p className={styles.searchingFeedback}>Searching...</p>}
```

**Success Criteria**:
- [ ] Spinner appears within 100ms of search initiation
- [ ] Search button disabled and shows "Searching..." text
- [ ] Loading state clears on success or error
- [ ] Skeleton loaders shown during initial fetch
- [ ] Loading progress shows entry count if available

**Testing**: Unit test state transitions, visual regression test

**Dependencies**: None

**Status**: Pending

---

#### Task 1.3: Fix Modal Accessibility (3h)

**Scope**: Add focus trap, ARIA attributes, Escape key handler, semantic HTML

**Files** (3 files):
- `web-app/src/app/history/page.tsx` (modify - modal structure)
- `web-app/src/lib/hooks/useFocusTrap.ts` (create)
- `web-app/src/app/history/history.module.css` (modify - modal overlay)

**Context**:
- WCAG 2.1: Dialog pattern (ARIA Authoring Practices)
- Reference: `web-app/src/components/sessions/SessionDetail.tsx` (check for existing modal patterns)
- Focus trap library: `focus-trap-react` (if not already in project)

**Implementation**:
```typescript
// Modal structure
<div
  role="dialog"
  aria-modal="true"
  aria-labelledby="modal-title"
  aria-describedby="modal-description"
>
  <FocusTrap>
    <div className={styles.modalContent}>
      <h2 id="modal-title">{entry.sessionId}</h2>
      <div id="modal-description">
        {/* Modal content */}
      </div>
      <button onClick={onClose} aria-label="Close modal">×</button>
    </div>
  </FocusTrap>
</div>

// Escape key handler
useEffect(() => {
  const handleEscape = (e: KeyboardEvent) => {
    if (e.key === 'Escape') onClose();
  };
  document.addEventListener('keydown', handleEscape);
  return () => document.removeEventListener('keydown', handleEscape);
}, [onClose]);
```

**Success Criteria**:
- [ ] Focus trapped within modal (Tab/Shift+Tab cycles through modal elements)
- [ ] First focusable element receives focus on open
- [ ] Focus returns to trigger element on close
- [ ] Escape key closes modal
- [ ] `role="dialog"`, `aria-modal="true"` present
- [ ] Modal title has `aria-labelledby`
- [ ] Screen reader announces modal open/close

**Testing**: Automated accessibility audit (axe-core), manual screen reader test

**Dependencies**: None

**Status**: Pending

---

#### Task 1.4: Add Keyboard Shortcuts Help Modal (1h)

**Scope**: Create help modal showing all keyboard shortcuts, triggered by ? key

**Files** (2 files):
- `web-app/src/components/history/KeyboardShortcutsHelp.tsx` (create)
- `web-app/src/app/history/page.tsx` (modify - integrate help modal)

**Context**:
- Reference: Sessions page help system (if exists)
- Pattern: Modal overlay with keyboard shortcuts table
- Nielsen H10: Help and documentation

**Implementation**:
```typescript
// KeyboardShortcutsHelp.tsx
const shortcuts = [
  { key: '↑/k', description: 'Navigate up' },
  { key: '↓/j', description: 'Navigate down' },
  { key: 'Enter', description: 'Open entry details' },
  { key: '/', description: 'Focus search' },
  { key: 'Esc', description: 'Close modal/clear search' },
  { key: '?', description: 'Show this help' },
  { key: 'g', description: 'Cycle grouping strategies' },
];

export function KeyboardShortcutsHelp({ isOpen, onClose }: Props) {
  return (
    <Modal isOpen={isOpen} onClose={onClose} title="Keyboard Shortcuts">
      <table>
        {shortcuts.map(({ key, description }) => (
          <tr key={key}>
            <td><kbd>{key}</kbd></td>
            <td>{description}</td>
          </tr>
        ))}
      </table>
    </Modal>
  );
}
```

**Success Criteria**:
- [ ] ? key opens help modal
- [ ] All keyboard shortcuts documented
- [ ] Modal accessible (focus trap, ARIA)
- [ ] Shortcuts grouped by category
- [ ] Visual key styling (kbd element)

**Testing**: Manual verification, E2E test

**Dependencies**: Task 1.3 (modal accessibility pattern)

**Status**: Pending

---

#### Task 1.5: Mobile Responsive Breakpoints (1h)

**Scope**: Fix 400px detail panel for mobile, add responsive breakpoints at 375px, 768px, 1024px

**Files** (2 files):
- `web-app/src/app/history/history.module.css` (modify)
- `web-app/src/app/history/page.tsx` (modify - conditional rendering for mobile)

**Context**:
- Reference: `web-app/src/components/sessions/SessionList.module.css` (responsive patterns)
- Breakpoints: 375px (mobile), 768px (tablet), 1024px (desktop)
- Issue #23: Fixed 400px detail panel breaks on mobile

**Implementation**:
```css
/* history.module.css */
.detailPanel {
  width: 400px;
}

@media (max-width: 768px) {
  .detailPanel {
    width: 100%;
    position: fixed;
    top: 0;
    left: 0;
    height: 100vh;
    z-index: 1000;
  }

  .historyList {
    display: none; /* Hide list when detail panel open on mobile */
  }
}

@media (max-width: 375px) {
  .historyCard {
    padding: 12px;
    font-size: 14px;
  }
}
```

**Success Criteria**:
- [ ] Detail panel full-width on mobile (< 768px)
- [ ] List hidden when detail panel open on mobile
- [ ] Back button added to detail panel on mobile
- [ ] Touch-friendly target sizes (min 44px)
- [ ] Horizontal scroll eliminated on all breakpoints

**Testing**: Visual regression tests at 375px, 768px, 1024px

**Dependencies**: None

**Status**: Pending

---

### Story 2: Error Handling and Empty States

#### Task 2.1: Implement Contextual Empty States (2h)

**Scope**: Distinguish "no history" vs "no search results", provide recovery guidance

**Files** (3 files):
- `web-app/src/components/history/EmptyState.tsx` (create)
- `web-app/src/app/history/page.tsx` (modify - integrate empty states)
- `web-app/src/app/history/history.module.css` (modify - empty state styling)

**Context**:
- Nielsen H9: Help users recognize, diagnose, and recover from errors
- WCAG Understandable: Clear messaging
- Issue #3: No distinction between error types

**Implementation**:
```typescript
// EmptyState.tsx
type EmptyStateType = 'no-history' | 'no-results' | 'error';

export function EmptyState({ type, onAction }: Props) {
  const content = {
    'no-history': {
      icon: '📋',
      title: 'No History Yet',
      message: 'Start a session to see it appear here',
      action: { label: 'Create Session', onClick: () => router.push('/sessions/new') },
    },
    'no-results': {
      icon: '🔍',
      title: 'No Matching Results',
      message: 'Try a different search term or clear filters',
      action: { label: 'Clear Search', onClick: onAction },
    },
    'error': {
      icon: '⚠️',
      title: 'Failed to Load History',
      message: 'Something went wrong loading your history',
      action: { label: 'Retry', onClick: onAction },
    },
  }[type];

  return (
    <div className={styles.emptyState}>
      <span className={styles.icon}>{content.icon}</span>
      <h3>{content.title}</h3>
      <p>{content.message}</p>
      <button onClick={content.action.onClick}>{content.action.label}</button>
    </div>
  );
}
```

**Success Criteria**:
- [ ] Different empty states for no-history, no-results, error
- [ ] Clear action button for each state
- [ ] Contextual messaging explains situation
- [ ] Icon and styling differentiate states
- [ ] Empty state centered and visually balanced

**Testing**: Unit tests for each state, visual regression test

**Dependencies**: None

**Status**: Pending

---

#### Task 2.2: Add Error Retry Functionality (2h)

**Scope**: Add retry button to error messages, preserve search query and selection

**Files** (2 files):
- `web-app/src/app/history/page.tsx` (modify - error handling logic)
- `web-app/src/lib/api/historyApi.ts` (modify - retry wrapper)

**Context**:
- Nielsen H3: User control and freedom
- Pattern: Exponential backoff, preserve user context
- Issue #4: No retry button, must refresh page

**Implementation**:
```typescript
// In page.tsx
const [error, setError] = useState<string | null>(null);
const [retryAttempt, setRetryAttempt] = useState(0);

const fetchHistory = async (preserveQuery = false) => {
  setError(null);
  setIsLoading(true);

  try {
    const results = await historyApi.fetch(preserveQuery ? searchQuery : '');
    setEntries(results);
    setRetryAttempt(0);
  } catch (err) {
    setError(err.message);
  } finally {
    setIsLoading(false);
  }
};

const handleRetry = () => {
  setRetryAttempt(prev => prev + 1);
  fetchHistory(true); // Preserve search query
};

// In JSX
{error && (
  <div className={styles.errorBanner}>
    <p>{error}</p>
    <button onClick={handleRetry}>
      Retry {retryAttempt > 0 && `(Attempt ${retryAttempt})`}
    </button>
  </div>
)}
```

**Success Criteria**:
- [ ] Retry button appears on error
- [ ] Search query preserved across retries
- [ ] Selection state preserved
- [ ] Retry attempt count shown if > 0
- [ ] Error message persists until successful retry
- [ ] Exponential backoff for multiple retries (optional)

**Testing**: Unit test error scenarios, E2E test retry flow

**Dependencies**: Task 2.1 (empty state component)

**Status**: Pending

---

### Story 3: Filtering and Grouping

#### Task 3.1: Add Filter Bar Component (3h)

**Scope**: Create filter bar with dropdowns for model, date range, status

**Files** (4 files):
- `web-app/src/components/history/HistoryFilters.tsx` (create)
- `web-app/src/components/history/HistoryFilters.module.css` (create)
- `web-app/src/app/history/page.tsx` (modify - integrate filters)
- `web-app/src/lib/hooks/useHistoryFilters.ts` (create)

**Context**:
- Reference: `web-app/src/components/sessions/SessionList.tsx` (lines 131-170 - filter UI)
- Pattern: Dropdown selects with onChange handlers, combine filter criteria
- Issue #6: Missing filtering (Sessions page has all these)

**Implementation**:
```typescript
// HistoryFilters.tsx
export function HistoryFilters({ onFiltersChange }: Props) {
  const [filters, setFilters] = useState<FilterState>({
    model: 'all',
    dateRange: 'all',
    status: 'all',
  });

  const handleFilterChange = (key: keyof FilterState, value: string) => {
    const newFilters = { ...filters, [key]: value };
    setFilters(newFilters);
    onFiltersChange(newFilters);
  };

  return (
    <div className={styles.filterBar}>
      <select value={filters.model} onChange={e => handleFilterChange('model', e.target.value)}>
        <option value="all">All Models</option>
        <option value="claude">Claude</option>
        <option value="aider">Aider</option>
        <option value="other">Other</option>
      </select>

      <select value={filters.dateRange} onChange={e => handleFilterChange('dateRange', e.target.value)}>
        <option value="all">All Time</option>
        <option value="today">Today</option>
        <option value="week">This Week</option>
        <option value="month">This Month</option>
      </select>

      <select value={filters.status} onChange={e => handleFilterChange('status', e.target.value)}>
        <option value="all">All Status</option>
        <option value="completed">Completed</option>
        <option value="failed">Failed</option>
        <option value="running">Running</option>
      </select>

      <button onClick={() => handleClearFilters()}>Clear All</button>
    </div>
  );
}
```

**Success Criteria**:
- [ ] Dropdowns for model, date range, status
- [ ] Filter changes trigger immediate result update
- [ ] "Clear All" button resets to defaults
- [ ] Active filters visually distinguished
- [ ] Filter count badge shows number of active filters
- [ ] Keyboard accessible (Tab, Enter, Arrow keys)

**Testing**: Unit tests for filter logic, E2E test for filter combinations

**Dependencies**: None

**Status**: Pending

---

#### Task 3.2: Implement Grouping Strategies (4h)

**Scope**: Add 8 grouping modes matching Sessions page (Date, Project, Model, Status, etc.)

**Files** (4 files):
- `web-app/src/lib/grouping/historyGrouping.ts` (create)
- `web-app/src/components/history/GroupedHistoryList.tsx` (create)
- `web-app/src/app/history/page.tsx` (modify - integrate grouping)
- `web-app/src/components/history/GroupSelector.tsx` (create)

**Context**:
- Reference: `web-app/src/components/sessions/SessionList.tsx` (grouping logic)
- Issue #7: No grouping strategy - flat list overwhelming
- Sessions page has 8 modes: Category, Tag, Branch, Path, Program, Status, Type, None

**Implementation**:
```typescript
// historyGrouping.ts
export type GroupingStrategy =
  | 'date' | 'project' | 'model' | 'status'
  | 'branch' | 'tag' | 'type' | 'none';

export function groupEntries(
  entries: HistoryEntry[],
  strategy: GroupingStrategy
): GroupedEntries {
  switch (strategy) {
    case 'date':
      return groupByDate(entries); // Today, Yesterday, This Week, Older
    case 'project':
      return groupByProject(entries); // Group by repository
    case 'model':
      return groupByModel(entries); // Claude, Aider, Other
    case 'status':
      return groupByStatus(entries); // Completed, Failed, Running
    case 'branch':
      return groupByBranch(entries);
    case 'tag':
      return groupByTag(entries);
    case 'type':
      return groupByType(entries); // Worktree, Directory
    case 'none':
      return { ungrouped: entries };
  }
}

function groupByDate(entries: HistoryEntry[]): GroupedEntries {
  const now = new Date();
  const today = startOfDay(now);
  const yesterday = subDays(today, 1);
  const weekAgo = subDays(today, 7);

  return {
    'Today': entries.filter(e => isAfter(e.timestamp, today)),
    'Yesterday': entries.filter(e =>
      isAfter(e.timestamp, yesterday) && isBefore(e.timestamp, today)
    ),
    'This Week': entries.filter(e =>
      isAfter(e.timestamp, weekAgo) && isBefore(e.timestamp, yesterday)
    ),
    'Older': entries.filter(e => isBefore(e.timestamp, weekAgo)),
  };
}
```

**Success Criteria**:
- [ ] 8 grouping strategies implemented
- [ ] G key cycles through strategies (keyboard)
- [ ] Dropdown selector for strategy (mouse)
- [ ] Current strategy shown in title bar
- [ ] Groups collapsible with expand/collapse
- [ ] Entry count shown for each group
- [ ] Empty groups hidden

**Testing**: Unit tests for each grouping function, E2E test for strategy cycling

**Dependencies**: Task 1.1 (keyboard navigation for G key)

**Status**: Pending

---

#### Task 3.3: Add Autocomplete Search with Suggestions (2h)

**Scope**: Search field shows suggestions for project names, models, branches

**Files** (3 files):
- `web-app/src/components/history/AutocompleteSearch.tsx` (create)
- `web-app/src/lib/hooks/useAutocompleteSuggestions.ts` (create)
- `web-app/src/app/history/page.tsx` (modify - replace search input)

**Context**:
- Reference: `web-app/src/lib/hooks/useRepositorySuggestions.ts` (existing autocomplete pattern)
- Issue #11: Search lacks autocomplete
- Pattern: Debounced input, suggestion dropdown, arrow key navigation

**Implementation**:
```typescript
// AutocompleteSearch.tsx
export function AutocompleteSearch({ onSearch }: Props) {
  const [query, setQuery] = useState('');
  const [showSuggestions, setShowSuggestions] = useState(false);
  const suggestions = useAutocompleteSuggestions(query);

  const handleInputChange = (value: string) => {
    setQuery(value);
    setShowSuggestions(value.length >= 2);
  };

  const handleSuggestionSelect = (suggestion: string) => {
    setQuery(suggestion);
    setShowSuggestions(false);
    onSearch(suggestion);
  };

  return (
    <div className={styles.autocompleteContainer}>
      <input
        value={query}
        onChange={e => handleInputChange(e.target.value)}
        placeholder="Search history..."
      />

      {showSuggestions && suggestions.length > 0 && (
        <ul className={styles.suggestions}>
          {suggestions.map(s => (
            <li key={s} onClick={() => handleSuggestionSelect(s)}>
              {s}
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

// useAutocompleteSuggestions.ts
export function useAutocompleteSuggestions(query: string) {
  const [suggestions, setSuggestions] = useState<string[]>([]);

  useEffect(() => {
    if (query.length < 2) {
      setSuggestions([]);
      return;
    }

    const debounce = setTimeout(() => {
      // Extract unique values from history entries
      const projects = [...new Set(entries.map(e => e.project))];
      const models = [...new Set(entries.map(e => e.model))];
      const branches = [...new Set(entries.map(e => e.branch))];

      const allSuggestions = [...projects, ...models, ...branches];
      const filtered = allSuggestions.filter(s =>
        s.toLowerCase().includes(query.toLowerCase())
      ).slice(0, 5);

      setSuggestions(filtered);
    }, 200);

    return () => clearTimeout(debounce);
  }, [query]);

  return suggestions;
}
```

**Success Criteria**:
- [ ] Suggestions appear after 2 characters
- [ ] Dropdown shows top 5 matches
- [ ] Suggestions include projects, models, branches
- [ ] Arrow keys navigate suggestions
- [ ] Enter selects highlighted suggestion
- [ ] Click selects suggestion
- [ ] Escape closes suggestions
- [ ] Debounced to 200ms

**Testing**: Unit tests for suggestion logic, E2E test for full flow

**Dependencies**: Task 1.1 (keyboard navigation)

**Status**: Pending

---

#### Task 3.4: Persist Filter State in localStorage (1h)

**Scope**: Save filter, grouping, and sort state to localStorage, restore on page load

**Files** (2 files):
- `web-app/src/lib/hooks/useHistoryFilters.ts` (modify - add persistence)
- `web-app/src/app/history/page.tsx` (modify - initialize from localStorage)

**Context**:
- Issue #17: Filter state not persisted
- Pattern: useEffect to sync state to localStorage
- Storage key: `history-filters-v1`

**Implementation**:
```typescript
// useHistoryFilters.ts
export function useHistoryFilters() {
  const [filters, setFilters] = useState<FilterState>(() => {
    const stored = localStorage.getItem('history-filters-v1');
    return stored ? JSON.parse(stored) : defaultFilters;
  });

  useEffect(() => {
    localStorage.setItem('history-filters-v1', JSON.stringify(filters));
  }, [filters]);

  return { filters, setFilters };
}
```

**Success Criteria**:
- [ ] Filter state persisted on change
- [ ] State restored on page load
- [ ] Grouping strategy persisted
- [ ] Search query NOT persisted (privacy)
- [ ] Version key allows future migrations
- [ ] localStorage quota errors handled gracefully

**Testing**: Unit test persistence logic, manual test across page reloads

**Dependencies**: Task 3.1 (filter bar)

**Status**: Pending

---

#### Task 3.5: Add Clear All Filters Button (1h)

**Scope**: Single button to reset filters, grouping, and search to defaults

**Files** (2 files):
- `web-app/src/components/history/HistoryFilters.tsx` (modify)
- `web-app/src/app/history/page.tsx` (modify - reset handler)

**Context**:
- Issue #6 (continued): Need quick way to reset view
- Pattern: Button triggers reset of all filter state
- Visual: Badge showing active filter count

**Implementation**:
```typescript
// In HistoryFilters.tsx
const activeFilterCount = useMemo(() => {
  return Object.entries(filters).filter(([key, value]) =>
    value !== 'all' && value !== ''
  ).length;
}, [filters]);

const handleClearAll = () => {
  setFilters(defaultFilters);
  onFiltersChange(defaultFilters);
  onSearchClear();
  onGroupingReset();
};

return (
  <div className={styles.filterBar}>
    {/* Filter dropdowns */}

    <button
      onClick={handleClearAll}
      disabled={activeFilterCount === 0}
    >
      Clear All {activeFilterCount > 0 && <span>({activeFilterCount})</span>}
    </button>
  </div>
);
```

**Success Criteria**:
- [ ] Resets all filters to defaults
- [ ] Clears search query
- [ ] Resets grouping to default strategy
- [ ] Badge shows active filter count
- [ ] Disabled when no filters active
- [ ] Keyboard accessible (Tab + Enter)

**Testing**: Unit test reset logic, E2E test full clear flow

**Dependencies**: Task 3.1 (filter bar), Task 3.2 (grouping)

**Status**: Pending

---

#### Task 3.6: Implement Group By Dropdown UI (2h)

**Scope**: Visual dropdown for selecting grouping strategy, matches Sessions page UX

**Files** (3 files):
- `web-app/src/components/history/GroupSelector.tsx` (create)
- `web-app/src/components/history/GroupSelector.module.css` (create)
- `web-app/src/app/history/page.tsx` (modify - integrate selector)

**Context**:
- Reference: Sessions page grouping UI
- Issue #7: Need visual way to change grouping
- Pattern: Dropdown with icons for each strategy

**Implementation**:
```typescript
// GroupSelector.tsx
const groupingOptions = [
  { value: 'date', label: 'Date', icon: '📅' },
  { value: 'project', label: 'Project', icon: '📁' },
  { value: 'model', label: 'Model', icon: '🤖' },
  { value: 'status', label: 'Status', icon: '⚡' },
  { value: 'branch', label: 'Branch', icon: '🌿' },
  { value: 'tag', label: 'Tag', icon: '🏷️' },
  { value: 'type', label: 'Type', icon: '📋' },
  { value: 'none', label: 'None', icon: '📄' },
];

export function GroupSelector({ value, onChange }: Props) {
  return (
    <select
      value={value}
      onChange={e => onChange(e.target.value as GroupingStrategy)}
      className={styles.groupSelector}
    >
      {groupingOptions.map(opt => (
        <option key={opt.value} value={opt.value}>
          {opt.icon} {opt.label}
        </option>
      ))}
    </select>
  );
}
```

**Success Criteria**:
- [ ] Dropdown shows all 8 grouping strategies
- [ ] Icons distinguish each strategy
- [ ] Current strategy highlighted
- [ ] Change triggers immediate regroup
- [ ] Keyboard accessible
- [ ] Positioned next to filter bar

**Testing**: Visual regression test, E2E test strategy change

**Dependencies**: Task 3.2 (grouping logic)

**Status**: Pending

---

#### Task 3.7: Add Filter Combination Logic (1h)

**Scope**: Ensure filters, search, and grouping work together correctly

**Files** (2 files):
- `web-app/src/lib/hooks/useHistoryFilters.ts` (modify)
- `web-app/src/app/history/page.tsx` (modify - combine operations)

**Context**:
- Issue #4 (continued): Combined search + filter + sort
- Workflow: Filter → Search → Group → Display
- Performance: < 100ms for 1000 entries

**Implementation**:
```typescript
// In page.tsx
const displayedEntries = useMemo(() => {
  let result = entries;

  // Step 1: Apply filters
  result = applyFilters(result, filters);

  // Step 2: Apply search
  if (searchQuery.length >= 2) {
    result = searchEntries(result, searchQuery);
  }

  // Step 3: Apply grouping
  const grouped = groupEntries(result, groupingStrategy);

  return grouped;
}, [entries, filters, searchQuery, groupingStrategy]);
```

**Success Criteria**:
- [ ] Filters applied before search
- [ ] Search applied to filtered results
- [ ] Grouping applied to filtered + searched results
- [ ] Performance < 100ms for 1000 entries
- [ ] Empty state shows if no results after all operations
- [ ] Clear indication of which operations active

**Testing**: Unit test combination logic, performance benchmark

**Dependencies**: Tasks 3.1, 3.2, 3.3

**Status**: Pending

---

### Story 4: Information Architecture and Bulk Actions

#### Task 4.1: Redesign History Card with Visual Hierarchy (3h)

**Scope**: Reduce information density, add relative time, improve scannability

**Files** (3 files):
- `web-app/src/components/history/HistoryCard.tsx` (create - extract from page.tsx)
- `web-app/src/components/history/HistoryCard.module.css` (create)
- `web-app/src/app/history/page.tsx` (modify - use new card component)

**Context**:
- Reference: `web-app/src/components/sessions/SessionCard.tsx` (card design patterns)
- Issue #8: High information density
- Pattern: Visual hierarchy with typography and spacing

**Implementation**:
```typescript
// HistoryCard.tsx
export function HistoryCard({ entry, isSelected }: Props) {
  const relativeTime = formatRelativeTime(entry.timestamp); // "3h ago"
  const truncatedId = entry.sessionId.slice(0, 8); // Truncate UUID

  return (
    <div className={`${styles.card} ${isSelected ? styles.selected : ''}`}>
      <div className={styles.header}>
        <h3 className={styles.title}>{entry.title || 'Untitled Session'}</h3>
        <time className={styles.timestamp}>{relativeTime}</time>
      </div>

      <div className={styles.metadata}>
        <span className={styles.model}>{entry.model}</span>
        <span className={styles.separator}>•</span>
        <span className={styles.id} title={entry.sessionId}>{truncatedId}</span>
      </div>

      <div className={styles.context}>
        <span className={styles.project}>{entry.project}</span>
        {entry.branch && (
          <>
            <span className={styles.separator}>→</span>
            <span className={styles.branch}>{entry.branch}</span>
          </>
        )}
      </div>

      {entry.tags.length > 0 && (
        <div className={styles.tags}>
          {entry.tags.map(tag => (
            <span key={tag} className={styles.tag}>{tag}</span>
          ))}
        </div>
      )}
    </div>
  );
}

// formatRelativeTime utility
function formatRelativeTime(timestamp: Date): string {
  const now = new Date();
  const diffMs = now.getTime() - timestamp.getTime();
  const diffMins = Math.floor(diffMs / 60000);
  const diffHours = Math.floor(diffMins / 60);
  const diffDays = Math.floor(diffHours / 24);

  if (diffMins < 1) return 'Just now';
  if (diffMins < 60) return `${diffMins}m ago`;
  if (diffHours < 24) return `${diffHours}h ago`;
  if (diffDays < 7) return `${diffDays}d ago`;
  return timestamp.toLocaleDateString();
}
```

**Success Criteria**:
- [ ] Relative time shown ("3h ago" vs "2024-12-05 14:30")
- [ ] UUID truncated to 8 chars with full ID in tooltip
- [ ] Visual hierarchy: Title > Metadata > Context > Tags
- [ ] Typography: Bold title, muted metadata, smaller context
- [ ] Spacing: 16px padding, 8px gaps between sections
- [ ] Tags displayed as pills with max 3 visible
- [ ] Card height consistent regardless of content

**Testing**: Visual regression test, accessibility audit

**Dependencies**: None

**Status**: Pending

---

#### Task 4.2: Implement Bulk Selection Mode (3h)

**Scope**: Add select mode with checkboxes, select all, bulk delete/export

**Files** (4 files):
- `web-app/src/components/history/BulkActionToolbar.tsx` (create)
- `web-app/src/components/history/HistoryCard.tsx` (modify - add checkbox)
- `web-app/src/app/history/page.tsx` (modify - selection state)
- `web-app/src/lib/hooks/useSelection.ts` (create)

**Context**:
- Issue #9: Cannot select multiple entries
- Pattern: Checkbox on each card, toolbar appears when items selected
- Reference: Sessions page bulk actions (if exists)

**Implementation**:
```typescript
// useSelection.ts
export function useSelection<T>(items: T[]) {
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set());

  const toggleSelection = (id: string) => {
    setSelectedIds(prev => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  const selectAll = () => {
    setSelectedIds(new Set(items.map(item => item.id)));
  };

  const clearSelection = () => {
    setSelectedIds(new Set());
  };

  return {
    selectedIds,
    toggleSelection,
    selectAll,
    clearSelection,
    isSelected: (id: string) => selectedIds.has(id),
    selectedCount: selectedIds.size,
  };
}

// BulkActionToolbar.tsx
export function BulkActionToolbar({ selectedCount, onDelete, onExport }: Props) {
  return (
    <div className={styles.toolbar}>
      <span>{selectedCount} selected</span>
      <button onClick={onDelete}>Delete</button>
      <button onClick={onExport}>Export</button>
      <button onClick={onClear}>Clear Selection</button>
    </div>
  );
}
```

**Success Criteria**:
- [ ] Checkbox appears on each card
- [ ] Click checkbox toggles selection
- [ ] "Select All" button in toolbar
- [ ] Bulk action toolbar appears when items selected
- [ ] Delete confirmation modal for bulk delete
- [ ] Export downloads JSON/Markdown file
- [ ] Keyboard: Shift+click for range selection
- [ ] Visual: Selected cards highlighted

**Testing**: Unit tests for selection logic, E2E test bulk delete

**Dependencies**: Task 4.1 (card component)

**Status**: Pending

---

#### Task 4.3: Add Action Toolbar to Detail Panel (2h)

**Scope**: Add Export, Copy ID, Open Folder, Delete buttons to detail view

**Files** (3 files):
- `web-app/src/components/history/DetailPanelActions.tsx` (create)
- `web-app/src/app/history/page.tsx` (modify - integrate actions)
- `web-app/src/lib/api/historyApi.ts` (modify - add delete, export APIs)

**Context**:
- Issue #10: Detail panel lacks actions
- Pattern: Action toolbar in panel header
- APIs: Export to JSON/Markdown, delete single entry

**Implementation**:
```typescript
// DetailPanelActions.tsx
export function DetailPanelActions({ entry }: Props) {
  const handleExport = async () => {
    const data = await exportEntry(entry.id);
    downloadFile(data, `${entry.sessionId}.json`);
  };

  const handleCopyId = () => {
    navigator.clipboard.writeText(entry.sessionId);
    toast.success('ID copied to clipboard');
  };

  const handleOpenFolder = () => {
    // Open folder in system file explorer (if supported)
    window.open(`file://${entry.workingDir}`);
  };

  const handleDelete = async () => {
    if (confirm('Delete this history entry?')) {
      await deleteEntry(entry.id);
      onClose();
    }
  };

  return (
    <div className={styles.actions}>
      <button onClick={handleExport} title="Export to JSON">
        <ExportIcon /> Export
      </button>
      <button onClick={handleCopyId} title="Copy session ID">
        <CopyIcon /> Copy ID
      </button>
      <button onClick={handleOpenFolder} title="Open in file explorer">
        <FolderIcon /> Open Folder
      </button>
      <button onClick={handleDelete} className={styles.danger} title="Delete entry">
        <DeleteIcon /> Delete
      </button>
    </div>
  );
}
```

**Success Criteria**:
- [ ] 4 action buttons in detail panel header
- [ ] Export downloads JSON file
- [ ] Copy ID shows success toast
- [ ] Open Folder opens system file explorer
- [ ] Delete shows confirmation modal
- [ ] Buttons keyboard accessible
- [ ] Icons visually distinguish actions

**Testing**: Unit tests for each action, E2E test export/delete

**Dependencies**: None

**Status**: Pending

---

#### Task 4.4: Add Message Search in Modal (2h)

**Scope**: Search bar in modal header to filter displayed messages

**Files** (3 files):
- `web-app/src/components/history/MessageSearch.tsx` (create)
- `web-app/src/app/history/page.tsx` (modify - integrate search in modal)
- `web-app/src/components/history/MessageSearch.module.css` (create)

**Context**:
- Issue #12: Long conversations hard to navigate
- Pattern: Search input filters visible messages
- Highlight matching text in messages

**Implementation**:
```typescript
// MessageSearch.tsx
export function MessageSearch({ messages, onFilteredMessagesChange }: Props) {
  const [query, setQuery] = useState('');

  const filteredMessages = useMemo(() => {
    if (query.length < 2) return messages;

    return messages.filter(msg =>
      msg.content.toLowerCase().includes(query.toLowerCase()) ||
      msg.role.toLowerCase().includes(query.toLowerCase())
    );
  }, [messages, query]);

  useEffect(() => {
    onFilteredMessagesChange(filteredMessages);
  }, [filteredMessages]);

  return (
    <div className={styles.messageSearch}>
      <input
        type="text"
        placeholder="Search messages..."
        value={query}
        onChange={e => setQuery(e.target.value)}
      />
      <span className={styles.resultCount}>
        {filteredMessages.length} of {messages.length} messages
      </span>
    </div>
  );
}

// In modal rendering
{filteredMessages.map(msg => (
  <div key={msg.id} className={styles.message}>
    <span className={styles.role}>{msg.role}</span>
    <div className={styles.content}>
      {highlightText(msg.content, searchQuery)}
    </div>
  </div>
))}
```

**Success Criteria**:
- [ ] Search input in modal header
- [ ] Messages filtered as user types
- [ ] Matching text highlighted in results
- [ ] Result count shown ("5 of 42 messages")
- [ ] Search cleared when modal closed
- [ ] Debounced to 200ms
- [ ] Case-insensitive search

**Testing**: Unit test filter logic, E2E test modal search

**Dependencies**: Task 1.3 (modal structure)

**Status**: Pending

---

#### Task 4.5: Implement Pagination Controls (2h)

**Scope**: Replace 100-entry limit with pagination (Previous/Next, entries-per-page selector)

**Files** (3 files):
- `web-app/src/components/history/Pagination.tsx` (create)
- `web-app/src/app/history/page.tsx` (modify - pagination state)
- `web-app/src/lib/api/historyApi.ts` (modify - support offset/limit params)

**Context**:
- Issue #13: Fixed 100-entry limit, no pagination
- Pattern: Page numbers, Previous/Next buttons, entries-per-page dropdown
- API: Add `offset` and `limit` query params

**Implementation**:
```typescript
// Pagination.tsx
export function Pagination({
  currentPage,
  totalEntries,
  entriesPerPage,
  onPageChange,
  onEntriesPerPageChange
}: Props) {
  const totalPages = Math.ceil(totalEntries / entriesPerPage);

  return (
    <div className={styles.pagination}>
      <button
        disabled={currentPage === 1}
        onClick={() => onPageChange(currentPage - 1)}
      >
        Previous
      </button>

      <span>
        Page {currentPage} of {totalPages} ({totalEntries} total)
      </span>

      <button
        disabled={currentPage === totalPages}
        onClick={() => onPageChange(currentPage + 1)}
      >
        Next
      </button>

      <select
        value={entriesPerPage}
        onChange={e => onEntriesPerPageChange(Number(e.target.value))}
      >
        <option value={25}>25 per page</option>
        <option value={50}>50 per page</option>
        <option value={100}>100 per page</option>
        <option value={200}>200 per page</option>
      </select>
    </div>
  );
}

// In page.tsx
const [currentPage, setCurrentPage] = useState(1);
const [entriesPerPage, setEntriesPerPage] = useState(50);

const fetchPage = async (page: number, limit: number) => {
  const offset = (page - 1) * limit;
  const data = await historyApi.fetch({ offset, limit });
  setEntries(data.entries);
  setTotalCount(data.total);
};
```

**Success Criteria**:
- [ ] Previous/Next buttons navigate pages
- [ ] Current page and total pages shown
- [ ] Entries-per-page dropdown (25, 50, 100, 200)
- [ ] Pagination preserves filter/search/grouping state
- [ ] Keyboard accessible (Tab + Enter)
- [ ] Page number input for direct navigation (optional)
- [ ] Disabled state for first/last page buttons

**Testing**: Unit tests for page calculation, E2E test pagination flow

**Dependencies**: None (API change required)

**Status**: Pending

---

### Story 5: Enhanced Features

#### Task 5.1: Add Read/Unread Indicators (2h)

**Scope**: Visual indicator for entries not yet viewed in detail

**Files** (3 files):
- `web-app/src/components/history/HistoryCard.tsx` (modify - add indicator)
- `web-app/src/lib/hooks/useReadStatus.ts` (create)
- `web-app/src/app/history/page.tsx` (modify - track read state)

**Context**:
- Issue #14: No read/unread visual indicators
- Pattern: Blue dot for unread, mark as read when detail opened
- Storage: localStorage to persist read status

**Implementation**:
```typescript
// useReadStatus.ts
export function useReadStatus() {
  const [readEntries, setReadEntries] = useState<Set<string>>(() => {
    const stored = localStorage.getItem('history-read-entries-v1');
    return stored ? new Set(JSON.parse(stored)) : new Set();
  });

  const markAsRead = (id: string) => {
    setReadEntries(prev => {
      const next = new Set(prev);
      next.add(id);
      localStorage.setItem('history-read-entries-v1', JSON.stringify([...next]));
      return next;
    });
  };

  const isRead = (id: string) => readEntries.has(id);

  return { isRead, markAsRead };
}

// In HistoryCard.tsx
{!isRead && <span className={styles.unreadIndicator} />}
```

**Success Criteria**:
- [ ] Blue dot shown on unread entries
- [ ] Dot removed when entry detail opened
- [ ] Read status persisted in localStorage
- [ ] Unread count shown in page title
- [ ] Mark all as read button (optional)

**Testing**: Unit test read status logic, visual test

**Dependencies**: Task 4.1 (card component)

**Status**: Pending

---

#### Task 5.2: Implement Export Functionality (2h)

**Scope**: Export history entries to JSON or Markdown format

**Files** (3 files):
- `web-app/src/lib/export/historyExporter.ts` (create)
- `web-app/src/components/history/ExportModal.tsx` (create)
- `web-app/src/app/history/page.tsx` (modify - export trigger)

**Context**:
- Issue #15: No export functionality
- Formats: JSON (machine-readable), Markdown (human-readable)
- Export selected entries or all filtered results

**Implementation**:
```typescript
// historyExporter.ts
export function exportToJSON(entries: HistoryEntry[]): string {
  return JSON.stringify(entries, null, 2);
}

export function exportToMarkdown(entries: HistoryEntry[]): string {
  const lines = ['# Session History', ''];

  for (const entry of entries) {
    lines.push(`## ${entry.title || 'Untitled'}`);
    lines.push(`- **ID**: ${entry.sessionId}`);
    lines.push(`- **Timestamp**: ${entry.timestamp}`);
    lines.push(`- **Model**: ${entry.model}`);
    lines.push(`- **Project**: ${entry.project}`);
    if (entry.branch) lines.push(`- **Branch**: ${entry.branch}`);
    lines.push('');

    if (entry.messages.length > 0) {
      lines.push('### Messages');
      for (const msg of entry.messages) {
        lines.push(`**${msg.role}**: ${msg.content}`);
        lines.push('');
      }
    }

    lines.push('---');
    lines.push('');
  }

  return lines.join('\n');
}

export function downloadFile(content: string, filename: string, mimeType: string) {
  const blob = new Blob([content], { type: mimeType });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = filename;
  a.click();
  URL.revokeObjectURL(url);
}
```

**Success Criteria**:
- [ ] Export button opens format selection modal
- [ ] JSON format preserves all data
- [ ] Markdown format human-readable
- [ ] Exports selected entries or all filtered
- [ ] Filename includes timestamp
- [ ] Large exports don't freeze UI

**Testing**: Unit tests for export formats, manual download test

**Dependencies**: Task 4.2 (bulk selection)

**Status**: Pending

---

#### Task 5.3: Add Progress Count to Loading State (1h)

**Scope**: Show "Loading... (42 of 100 entries)" during fetch

**Files** (2 files):
- `web-app/src/app/history/page.tsx` (modify)
- `web-app/src/lib/api/historyApi.ts` (modify - streaming progress)

**Context**:
- Issue #16: Loading state shows no progress
- Pattern: Streaming fetch with progress callbacks
- Improves perceived performance for slow connections

**Implementation**:
```typescript
// In page.tsx
const [loadingProgress, setLoadingProgress] = useState<{ loaded: number; total: number } | null>(null);

const fetchHistory = async () => {
  setIsLoading(true);

  await historyApi.fetchWithProgress({
    onProgress: ({ loaded, total }) => {
      setLoadingProgress({ loaded, total });
    },
    onComplete: (entries) => {
      setEntries(entries);
      setLoadingProgress(null);
      setIsLoading(false);
    }
  });
};

// In JSX
{isLoading && (
  <div className={styles.loadingIndicator}>
    <Spinner />
    {loadingProgress && (
      <span>
        Loading... ({loadingProgress.loaded} of {loadingProgress.total} entries)
      </span>
    )}
  </div>
)}
```

**Success Criteria**:
- [ ] Progress count shown during load
- [ ] Updates in real-time as entries stream in
- [ ] Falls back to generic "Loading..." if progress unavailable
- [ ] Progress bar optional enhancement

**Testing**: Manual test with network throttling

**Dependencies**: Task 1.2 (loading states)

**Status**: Pending

---

#### Task 5.4: Truncate UUIDs with Copy Button (1h)

**Scope**: Show truncated UUID (8 chars) with copy button for full ID

**Files** (2 files):
- `web-app/src/components/history/HistoryCard.tsx` (modify)
- `web-app/src/components/shared/CopyButton.tsx` (create or reuse)

**Context**:
- Issue #18: Full UUID displayed unnecessarily
- Pattern: Truncated text with tooltip, copy button
- Improvement: Save space, reduce visual clutter

**Implementation**:
```typescript
// In HistoryCard.tsx
<div className={styles.idSection}>
  <span className={styles.truncatedId} title={entry.sessionId}>
    {entry.sessionId.slice(0, 8)}...
  </span>
  <CopyButton text={entry.sessionId} />
</div>

// CopyButton.tsx
export function CopyButton({ text }: Props) {
  const [copied, setCopied] = useState(false);

  const handleCopy = async () => {
    await navigator.clipboard.writeText(text);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <button
      onClick={handleCopy}
      className={styles.copyButton}
      title="Copy full ID"
    >
      {copied ? <CheckIcon /> : <CopyIcon />}
    </button>
  );
}
```

**Success Criteria**:
- [ ] UUID truncated to 8 chars + "..."
- [ ] Full UUID shown in tooltip on hover
- [ ] Copy button copies full UUID
- [ ] Visual feedback on successful copy (checkmark icon)
- [ ] Button keyboard accessible

**Testing**: Manual test copy functionality, visual test

**Dependencies**: Task 4.1 (card component)

**Status**: Pending

---

#### Task 5.5: Add Syntax Highlighting for Code Messages (2h)

**Scope**: Detect and highlight code blocks in message content

**Files** (3 files):
- `web-app/src/components/history/MessageContent.tsx` (create)
- `web-app/src/app/history/page.tsx` (modify - use in modal)
- `web-app/package.json` (add `react-syntax-highlighter` dependency)

**Context**:
- Issue #20: No syntax highlighting for code
- Library: `react-syntax-highlighter` with VS Code theme
- Pattern: Detect code blocks with markdown fences

**Implementation**:
```typescript
// MessageContent.tsx
import { Prism as SyntaxHighlighter } from 'react-syntax-highlighter';
import { vscDarkPlus } from 'react-syntax-highlighter/dist/esm/styles/prism';

export function MessageContent({ content }: Props) {
  const parts = parseContent(content); // Split into text and code blocks

  return (
    <div className={styles.messageContent}>
      {parts.map((part, i) =>
        part.type === 'code' ? (
          <SyntaxHighlighter
            key={i}
            language={part.language}
            style={vscDarkPlus}
          >
            {part.content}
          </SyntaxHighlighter>
        ) : (
          <p key={i}>{part.content}</p>
        )
      )}
    </div>
  );
}

function parseContent(content: string): ContentPart[] {
  const codeBlockRegex = /```(\w+)?\n([\s\S]*?)```/g;
  const parts: ContentPart[] = [];
  let lastIndex = 0;
  let match;

  while ((match = codeBlockRegex.exec(content)) !== null) {
    // Add text before code block
    if (match.index > lastIndex) {
      parts.push({
        type: 'text',
        content: content.slice(lastIndex, match.index),
      });
    }

    // Add code block
    parts.push({
      type: 'code',
      language: match[1] || 'text',
      content: match[2],
    });

    lastIndex = match.index + match[0].length;
  }

  // Add remaining text
  if (lastIndex < content.length) {
    parts.push({
      type: 'text',
      content: content.slice(lastIndex),
    });
  }

  return parts;
}
```

**Success Criteria**:
- [ ] Code blocks detected with markdown fences
- [ ] Syntax highlighting applied with VS Code dark theme
- [ ] Language auto-detected from fence info
- [ ] Inline code styled differently from blocks
- [ ] Copy button on code blocks (optional)

**Testing**: Visual test with code examples, performance test

**Dependencies**: Task 4.4 (modal message display)

**Status**: Pending

---

#### Task 5.6: Audit and Fix Dark Mode Contrast (1h)

**Scope**: Run accessibility audit, fix WCAG AA contrast violations

**Files** (2 files):
- `web-app/src/app/history/history.module.css` (modify - color values)
- `web-app/src/components/history/HistoryCard.module.css` (modify)

**Context**:
- Issue #21: Dark mode contrast not audited
- WCAG AA: 4.5:1 for normal text, 3:1 for large text
- Tools: Chrome DevTools Lighthouse, axe-core

**Implementation**:
```css
/* Before (potential violations) */
.metadata {
  color: #666; /* May fail contrast on dark bg */
}

/* After (WCAG AA compliant) */
.metadata {
  color: #999; /* 4.5:1 contrast ratio on #1a1a1a background */
}

/* Ensure all text elements meet contrast requirements */
.title {
  color: #fff; /* 21:1 - excellent */
}

.timestamp {
  color: #aaa; /* 7:1 - good */
}

.tag {
  background: #333;
  color: #e0e0e0; /* 5:1 - compliant */
}
```

**Success Criteria**:
- [ ] All text passes WCAG AA contrast (4.5:1)
- [ ] Large text passes 3:1 minimum
- [ ] Focus indicators visible (3:1 vs background)
- [ ] Automated audit (Lighthouse) shows 100% accessibility score
- [ ] Manual review with color blindness simulators

**Testing**: Lighthouse audit, axe-core, manual review

**Dependencies**: None

**Status**: Pending

---

#### Task 5.7: Preserve Scroll Position on Navigation (1h)

**Scope**: Restore scroll position when returning from detail view

**Files** (2 files):
- `web-app/src/app/history/page.tsx` (modify)
- `web-app/src/lib/hooks/useScrollRestoration.ts` (create)

**Context**:
- Issue #22: Scroll position not preserved
- Pattern: Save scroll offset before navigation, restore after
- Improves UX for browsing long lists

**Implementation**:
```typescript
// useScrollRestoration.ts
export function useScrollRestoration(key: string) {
  const scrollRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const savedPosition = sessionStorage.getItem(`scroll-${key}`);
    if (savedPosition && scrollRef.current) {
      scrollRef.current.scrollTop = parseInt(savedPosition, 10);
    }
  }, [key]);

  const saveScrollPosition = () => {
    if (scrollRef.current) {
      sessionStorage.setItem(`scroll-${key}`, String(scrollRef.current.scrollTop));
    }
  };

  return { scrollRef, saveScrollPosition };
}

// In page.tsx
const { scrollRef, saveScrollPosition } = useScrollRestoration('history-list');

const handleEntryClick = (entry: HistoryEntry) => {
  saveScrollPosition();
  setSelectedEntry(entry);
};

return (
  <div ref={scrollRef} className={styles.historyList}>
    {/* List content */}
  </div>
);
```

**Success Criteria**:
- [ ] Scroll position saved before opening detail
- [ ] Position restored when closing detail
- [ ] Works across page refreshes (sessionStorage)
- [ ] Smooth scroll restoration (no jump)
- [ ] Position cleared on page navigation away from history

**Testing**: Manual test navigation flow

**Dependencies**: None

**Status**: Pending

---

## Testing Strategy

### Unit Testing

**Coverage Goals**:
- 80% coverage for business logic (filtering, grouping, search)
- 70% coverage for UI components
- 100% coverage for utility functions (formatRelativeTime, exporters)

**Key Test Suites**:
1. **Keyboard Navigation** (`useKeyboard.test.ts`)
   - Arrow key navigation
   - j/k navigation
   - / focuses search
   - Escape closes modal
   - Enter opens detail

2. **Filter Logic** (`useHistoryFilters.test.ts`)
   - Individual filters work
   - Combined filters work
   - Clear all resets state
   - localStorage persistence

3. **Grouping Strategies** (`historyGrouping.test.ts`)
   - Each grouping strategy produces correct groups
   - Empty groups handled
   - Multi-membership for tags
   - Performance benchmarks

4. **Search and Autocomplete** (`useAutocompleteSuggestions.test.ts`)
   - Suggestions appear after 2 chars
   - Debouncing works
   - Fuzzy matching works
   - Performance with 1000 entries

### Integration Testing

**E2E Test Scenarios** (Playwright):

1. **Critical Path: Search and Resume**
   - User opens history page
   - Types search query
   - Navigates with arrow keys
   - Opens entry detail
   - Clicks resume button
   - Verify session resumes

2. **Filter and Group Workflow**
   - Apply model filter
   - Apply date range filter
   - Change grouping strategy
   - Verify filtered and grouped results
   - Clear filters
   - Verify reset to all entries

3. **Bulk Operations**
   - Enable select mode
   - Select multiple entries
   - Click bulk delete
   - Confirm deletion
   - Verify entries removed

4. **Error Recovery**
   - Simulate API error
   - Verify error message shown
   - Click retry button
   - Verify successful retry

5. **Keyboard Navigation Flow**
   - Open history page
   - Press j to navigate down
   - Press k to navigate up
   - Press / to focus search
   - Type query
   - Press Enter to open detail
   - Press Escape to close

### Accessibility Testing

**Automated Tools**:
- Lighthouse (CI pipeline)
- axe-core (unit tests with `jest-axe`)
- eslint-plugin-jsx-a11y (linting)

**Manual Tests**:
1. **Screen Reader** (VoiceOver on macOS, NVDA on Windows)
   - All content announced correctly
   - Modal open/close announced
   - Form labels clear
   - Button purposes clear

2. **Keyboard Only**
   - All functionality accessible via keyboard
   - Focus visible at all times
   - Tab order logical
   - No keyboard traps

3. **Color Blindness**
   - Status indicators distinguishable without color
   - Error messages have icons
   - Success feedback has text

4. **Zoom to 200%**
   - All content readable
   - No horizontal scroll
   - No overlapping elements

### Performance Testing

**Benchmarks**:
- Search latency < 200ms (1000 entries)
- Filter latency < 100ms (1000 entries)
- Group latency < 100ms (1000 entries)
- Modal open time < 100ms
- Page load time < 2s (50 entries)

**Load Testing**:
- 100 entries: All operations smooth
- 1000 entries: Search/filter/group < 200ms
- 10000 entries: Pagination required, 50 entries per page

---

## Known Issues and Mitigation

### Issue 1: API Pagination Not Yet Implemented

**Problem**: History API currently returns all entries, no offset/limit support

**Impact**: Task 4.5 (pagination) blocked until API updated

**Mitigation**:
- Implement client-side pagination as temporary solution
- File backend ticket for API pagination support
- Migrate to server-side pagination in follow-up sprint

**Timeline**: API update estimated 1 week, migrate client 2 hours

---

### Issue 2: Large Message Content Performance

**Problem**: Rendering 1000+ messages in modal may cause performance issues

**Impact**: Task 4.4 (message search) may need optimization

**Mitigation**:
- Implement virtual scrolling for message list (react-window)
- Lazy load message content (only render visible messages)
- Add "Show more" button for long conversations

**Timeline**: Virtual scrolling adds 3 hours to Task 4.4

---

### Issue 3: Export File Size Limits

**Problem**: Exporting 1000 entries to JSON may hit browser memory limits

**Impact**: Task 5.2 (export) may fail for large selections

**Mitigation**:
- Add warning for exports > 500 entries
- Stream export to file instead of in-memory string
- Offer to export in batches

**Timeline**: Streaming export adds 2 hours to Task 5.2

---

### Issue 4: localStorage Quota Exceeded

**Problem**: Storing read status for 10000 entries may exceed localStorage quota (5MB)

**Impact**: Task 5.1 (read/unread) may fail silently

**Mitigation**:
- Catch QuotaExceededError and fall back to in-memory storage
- Implement LRU cache for read status (keep only recent 1000)
- Warn user if quota exceeded

**Timeline**: Error handling adds 1 hour to Task 5.1

---

## Dependencies and Timeline

### External Dependencies

1. **Backend API Updates**
   - Pagination support (offset/limit) - Required for Task 4.5
   - Delete single/bulk entries API - Required for Tasks 4.2, 4.3
   - Export API (optional) - Improves Task 5.2 performance

2. **Library Dependencies**
   - `react-syntax-highlighter` - Required for Task 5.5
   - `focus-trap-react` - Required for Task 1.3 (or use existing)
   - `react-window` - Optional optimization for Task 4.4

### Story Dependencies

```
Story 1 (Keyboard & Accessibility) ──┐
                                      │
Story 2 (Error Handling)              ├──> Story 3 (Filtering & Grouping)
                                      │           │
                                      │           │
                                      └───────────┴──> Story 4 (Info Architecture & Bulk)
                                                              │
                                                              │
                                                              └──> Story 5 (Enhanced Features)
```

**Critical Path**:
- Story 1 → Story 3 → Story 4 → Story 5
- Estimated: 10h + 14h + 12h + 10h = **46 hours total**

**Parallel Work Opportunities**:
- Story 2 (4h) can run parallel to Story 1
- Tasks within each story are mostly independent

### Sprint Breakdown

#### Sprint 1 - Foundation (2 weeks, 20h)
**Goal**: Keyboard navigation, error handling, basic filtering

- Story 1: Tasks 1.1-1.5 (10h)
- Story 2: Tasks 2.1-2.2 (4h)
- Story 3: Tasks 3.1, 3.4, 3.5 (5h)

**Deliverables**:
- Full keyboard navigation
- Loading states and error recovery
- Basic filtering (model, date, status)
- localStorage persistence

#### Sprint 2 - Organization (2 weeks, 16h)
**Goal**: Grouping, autocomplete, filter combinations

- Story 3: Tasks 3.2, 3.3, 3.6, 3.7 (9h)
- Story 4: Task 4.1 (3h)
- Story 5: Task 5.6 (1h) - early accessibility fix

**Deliverables**:
- 8 grouping strategies
- Autocomplete search
- Redesigned history cards
- WCAG AA compliant dark mode

#### Sprint 3 - Actions (2 weeks, 14h)
**Goal**: Bulk actions, pagination, detail panel enhancements

- Story 4: Tasks 4.2-4.5 (9h)
- Story 5: Tasks 5.1, 5.4, 5.7 (4h)

**Deliverables**:
- Bulk selection and delete
- Pagination controls
- Action toolbar in detail panel
- Message search in modal
- Read/unread indicators

#### Sprint 4 - Polish (1 week, 6h)
**Goal**: Export, syntax highlighting, final enhancements

- Story 5: Tasks 5.2, 5.3, 5.5 (5h)
- Testing and bug fixes (variable)

**Deliverables**:
- Export to JSON/Markdown
- Syntax highlighting for code
- Progress indicators
- Final polish and bug fixes

### Timeline Summary

**Total Effort**: 46 hours (5.75 developer days)
**Calendar Time**: 7 weeks (with testing and reviews)
**Sprint Count**: 4 sprints

**Milestones**:
- Week 2: Keyboard navigation complete (Sprint 1)
- Week 4: Filtering and grouping complete (Sprint 2)
- Week 6: Bulk actions and pagination complete (Sprint 3)
- Week 7: Feature complete with polish (Sprint 4)

---

## Context Preparation Guides

### For Frontend Developers

**Before Starting Story 1**:
1. Review existing keyboard navigation in Sessions page
   - File: `web-app/src/components/sessions/SessionList.tsx`
   - Focus on: Key event handlers (lines 200-250)
   - Pattern: Custom hook for keyboard logic

2. Understand BubbleTea patterns in TUI (reference only)
   - File: `ui/list.go`
   - Focus on: Navigation methods (lines 766-866)
   - Pattern: State machine with keyboard events

3. Review WCAG 2.1 Dialog Pattern
   - Docs: https://www.w3.org/WAI/ARIA/apg/patterns/dialog-modal/
   - Focus on: Focus management, ARIA attributes

**Before Starting Story 3**:
1. Review existing filter implementation in Sessions
   - File: `web-app/src/components/sessions/SessionList.tsx`
   - Focus on: Filter bar UI (lines 131-170)
   - Pattern: useState for filter state, useMemo for filtered results

2. Study grouping strategies in TUI
   - File: `ui/list.go`
   - Focus on: Category organization (lines 1240-1278)
   - Pattern: Group sessions by property, maintain expansion state

3. Review autocomplete hooks
   - Files: `web-app/src/lib/hooks/useRepositorySuggestions.ts`, `useBranchSuggestions.ts`
   - Focus on: Debouncing, suggestion filtering
   - Pattern: useEffect with cleanup, debounced API calls

**Before Starting Story 4**:
1. Review SessionCard design patterns
   - File: `web-app/src/components/sessions/SessionCard.tsx`
   - Focus on: Visual hierarchy, typography, spacing
   - Pattern: CSS modules with BEM-like naming

2. Study selection patterns (if exists in codebase)
   - Search for: checkbox selection, bulk operations
   - Pattern: Set for selected IDs, toolbar appears on selection

3. Review export utilities (if exists)
   - Search for: JSON export, file download helpers
   - Pattern: Blob creation, URL.createObjectURL

### For Backend Developers

**API Changes Needed**:

1. **Pagination Support** (for Task 4.5)
   ```go
   // GET /api/history?offset=0&limit=50
   type HistoryListRequest struct {
     Offset int `json:"offset"`
     Limit  int `json:"limit"`
   }

   type HistoryListResponse struct {
     Entries []HistoryEntry `json:"entries"`
     Total   int            `json:"total"`
   }
   ```

2. **Delete Entry API** (for Tasks 4.2, 4.3)
   ```go
   // DELETE /api/history/:id
   // DELETE /api/history/bulk (body: { ids: string[] })
   ```

3. **Export API (Optional)** (for Task 5.2)
   ```go
   // GET /api/history/export?ids=id1,id2&format=json|markdown
   // Returns file download
   ```

**Files to Modify**:
- `server/services/session_service.go` - Add pagination, delete methods
- `server/adapters/instance_adapter.go` - Add export methods
- `proto/session/v1/session.proto` - Update protobuf definitions

### For QA/Testers

**Test Environment Setup**:
1. Create test history entries (100+)
   - Mix of different models (Claude, Aider, etc.)
   - Various dates (today, yesterday, last week, older)
   - Different statuses (completed, failed, running)
   - Multiple projects and branches

2. Install browser extensions
   - axe DevTools (accessibility testing)
   - React DevTools (component inspection)
   - Lighthouse (performance audits)

3. Set up screen readers
   - macOS: VoiceOver (Cmd+F5)
   - Windows: NVDA (free download)
   - Chrome extension: ChromeVox

**Test Data Script**:
```javascript
// Run in browser console to generate test data
const testEntries = [];
const models = ['claude', 'aider', 'gpt-4'];
const projects = ['project-a', 'project-b', 'project-c'];
const statuses = ['completed', 'failed', 'running'];

for (let i = 0; i < 100; i++) {
  testEntries.push({
    sessionId: `test-${i}-${Date.now()}`,
    title: `Test Session ${i}`,
    model: models[i % models.length],
    project: projects[i % projects.length],
    status: statuses[i % statuses.length],
    timestamp: new Date(Date.now() - i * 3600000), // 1 hour apart
    messages: [
      { role: 'user', content: `Test message ${i}` },
      { role: 'assistant', content: `Test response ${i}` },
    ],
  });
}

// Submit to API
await fetch('/api/history/bulk', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify(testEntries),
});
```

**Critical Test Paths**:
1. Keyboard-only navigation (no mouse)
2. Screen reader announcement accuracy
3. Search performance with 1000 entries
4. Bulk delete with 50 selected items
5. Export 100 entries to JSON/Markdown
6. Filter + search + group combination
7. Mobile responsive at 375px
8. Error recovery without page refresh

---

## Success Criteria Summary

### Definition of Done (Epic Level)

**Feature Complete**:
- [ ] All 23 tasks implemented (5 stories)
- [ ] Manual testing passed for all user flows
- [ ] E2E tests written and passing for critical paths
- [ ] Unit test coverage > 75%

**Accessibility**:
- [ ] WCAG 2.1 AA compliance verified (automated + manual)
- [ ] Screen reader test passed (VoiceOver, NVDA)
- [ ] Keyboard-only navigation tested
- [ ] Color contrast audit passed

**Performance**:
- [ ] Search latency < 200ms (1000 entries)
- [ ] Filter/group latency < 100ms (1000 entries)
- [ ] Page load time < 2s (50 entries)
- [ ] Lighthouse performance score > 90

**Consistency**:
- [ ] Keyboard shortcuts match Sessions page
- [ ] Filter UI matches Sessions page patterns
- [ ] Grouping strategies match Sessions page (8 modes)
- [ ] Design system compliance verified

**Documentation**:
- [ ] User guide updated with new features
- [ ] Keyboard shortcuts documented
- [ ] API changes documented (if any)
- [ ] Component Storybook stories added

### User Acceptance Criteria

**Power User Efficiency**:
- [ ] User can navigate history without mouse
- [ ] User can search and open entry in < 5 seconds
- [ ] User can bulk delete 20 entries in < 10 seconds
- [ ] User can filter by model + date in < 3 seconds

**Accessibility**:
- [ ] Screen reader user can complete all tasks
- [ ] Keyboard-only user can complete all tasks
- [ ] Color blind user can distinguish all states
- [ ] Zoom to 200% does not break layout

**Error Recovery**:
- [ ] User can retry failed operations without refresh
- [ ] User gets clear guidance on empty states
- [ ] User can recover from pagination errors
- [ ] User knows when operations are in progress

**Consistency**:
- [ ] History page feels like Sessions page
- [ ] Same keyboard shortcuts work
- [ ] Same filter patterns work
- [ ] Same grouping strategies work

---

## Appendix: Nielsen's Heuristics and WCAG Mapping

### Heuristic-to-Task Mapping

| Heuristic | Violated By | Fixed By |
|-----------|-------------|----------|
| H1 - Visibility of System Status | Issues #2, #13 | Tasks 1.2, 4.5, 5.3 |
| H3 - User Control and Freedom | Issues #4, #5 | Tasks 1.3, 2.2 |
| H4 - Consistency and Standards | Issues #6, #7 | Tasks 3.1, 3.2 |
| H7 - Flexibility and Efficiency | Issues #1, #9 | Tasks 1.1, 4.2 |
| H8 - Aesthetic and Minimalist Design | Issue #8 | Task 4.1 |
| H9 - Error Recovery | Issues #3, #4 | Tasks 2.1, 2.2 |
| H10 - Help and Documentation | Issue #19 | Task 1.4 |

### WCAG POUR Mapping

| Principle | Violated By | Fixed By |
|-----------|-------------|----------|
| Perceivable | Issues #2, #5, #21 | Tasks 1.2, 1.3, 5.6 |
| Operable | Issues #1, #5 | Tasks 1.1, 1.3 |
| Understandable | Issues #3, #4 | Tasks 2.1, 2.2 |
| Robust | Issue #5 | Task 1.3 |

---

## References

**Internal Documentation**:
- `/Users/tylerstapler/IdeaProjects/claude-squad/docs/tasks/session-search-and-sort.md` - Search patterns
- `/Users/tylerstapler/IdeaProjects/claude-squad/CLAUDE.md` - Development commands

**External Resources**:
- [WCAG 2.1 Guidelines](https://www.w3.org/WAI/WCAG21/quickref/)
- [ARIA Authoring Practices](https://www.w3.org/WAI/ARIA/apg/)
- [Nielsen's 10 Usability Heuristics](https://www.nngroup.com/articles/ten-usability-heuristics/)

**Codebase References**:
- `web-app/src/components/sessions/SessionList.tsx` - Filtering, grouping, keyboard nav
- `web-app/src/components/sessions/SessionCard.tsx` - Card design patterns
- `ui/list.go` - TUI keyboard navigation logic
- `web-app/src/lib/hooks/useRepositorySuggestions.ts` - Autocomplete pattern

---

**Last Updated**: 2025-12-05
**Author**: Project Coordination Specialist
**Status**: Ready for Sprint Planning
