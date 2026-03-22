# Session Explorer Enhancement Plan

## Overview

This document outlines the plan to enhance the Stapler Squad session explorer by removing the 10-session limit and improving the session management experience. The enhancements focus on better organization, navigation, and searchability of sessions.

## Goals

1. Remove the artificial session limit
2. Implement session categorization and grouping
3. Add session search functionality
4. Enhance UI for better navigation with many sessions
5. Improve overall session management workflow

## Implementation Phases

### Phase 1: Increase Session Limit and Basic Grouping

- Update `GlobalInstanceLimit` constant in `app/app.go`
- Extend `session.Instance` struct to add category/group fields
- Update UI rendering to support session grouping
- Implement collapsible groups in session list
- Modify storage mechanism to persist categories

**Key Files:**
- `app/app.go` - Update session limit constant
- `session/instance.go` - Add categorization fields
- `ui/list.go` - Update rendering for groups
- `session/storage.go` - Update persistence

### Phase 2: Add Search Functionality

- Create SearchableList component using existing fuzzy search implementation
- Add keyboard shortcuts for activating search
- Implement search by session title, content, branch, and other metadata
- Add search result highlighting to InstanceRenderer

**Key Files:**
- `ui/list.go` - Add search functionality
- `ui/fuzzy/fuzzy.go` - Leverage existing fuzzy search
- `app/app.go` - Add key bindings for search

### Phase 3: Enhance Organization with Tags

- Implement tag-based filtering system for sessions
- Update SessionSetupOverlay to include tag selection
- Add UI for managing tags on existing sessions
- Enable filtering sessions by tag combinations

**Key Files:**
- `session/instance.go` - Add tags field
- `ui/overlay/sessionSetup.go` - Add tag input
- `ui/list.go` - Implement tag filtering

### Phase 4: Polish and Optimize

- Add user preferences for view options
- Optimize performance for large numbers of sessions
- Improve keyboard navigation
- Add help documentation for new features

**Key Files:**
- `app/app.go` - Performance optimizations
- `ui/menu.go` - Update help documentation
- `ui/list.go` - View options and optimizations

## Technical Details

### Session Categorization

Sessions will be organized using a combination of:
1. **Groups/Categories**: Primary organization method
2. **Tags**: Flexible, multi-faceted categorization
3. **Search**: Dynamic filtering by content

### Search Implementation

The search functionality will leverage the existing fuzzy search implementation in `ui/fuzzy/fuzzy.go`:
- Keyboard shortcut to activate search mode
- Incremental search with real-time results
- Highlighting of matching terms
- Search across multiple fields (title, content, branch, tags)

### UI Updates

- Collapsible groups with visual indentation
- Color coding for different categories
- Improved status indicators
- Keyboard shortcuts for navigation between groups

## Future Considerations

- Session templates for quick creation
- Session archiving for long-term storage
- Advanced filtering by date ranges or activity
- Session statistics and metrics