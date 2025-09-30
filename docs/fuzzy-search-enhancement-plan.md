# Fuzzy Search Enhancement Plan

## Overview
Upgrade claude-squad's session search to provide best-in-class fuzzy search functionality similar to FZF, VS Code's Ctrl+P, or other modern fuzzy finders.

## Current State Analysis

### Existing Infrastructure ✅
- **Sophisticated fuzzy search engine** in `ui/fuzzy/fuzzy.go`:
  - Debounced input handling (300ms)
  - Async loading with goroutines
  - Score-based ranking with match highlighting
  - Thread-safe operations
  - Prefix, contains, and fuzzy pattern matching

- **Active usage** in repository/branch/directory selection components
- **Basic session search** limited to title-only matching

### Identified Limitations
1. Session search only searches titles (`SearchByTitle()`)
2. Uses simple `strings.Contains()` instead of fuzzy matching
3. No real-time dynamic filtering
4. Missing multi-field search across session properties

## Implementation Plan

### Phase 1: Enhanced Session Fuzzy Search (Primary Goal)

#### 1.1 Session Search Adapter
Create `SessionSearchItem` struct implementing `SearchItem` interface:

```go
type SessionSearchItem struct {
    Instance *session.Instance
}

func (s SessionSearchItem) GetSearchText() string {
    // Combine searchable fields with appropriate weighting:
    // - Title (primary, highest priority)
    // - Path (repository path)
    // - Program (claude, aider, etc.)
    // - Category (project organization)
    // - Branch (git branch name)
    // - WorkingDir (subdirectory)
}
```

#### 1.2 Fuzzy Search Integration
Replace `SearchByTitle()` with fuzzy search engine:
- Maintain existing keybindings (`s`, `/`)
- Preserve UI state management
- Add debounced real-time filtering
- Include match highlighting in results

#### 1.3 Multi-Field Search Priority
Search field weighting (high to low priority):
1. **Title** - Primary identifier, exact/prefix matches heavily weighted
2. **Category** - Project organization context
3. **Program** - Tool being used (claude, aider)
4. **Branch** - Git branch name
5. **Path** - Repository path
6. **WorkingDir** - Subdirectory context

### Phase 2: Filesystem Search Interface (Future Enhancement)

#### 2.1 Filesystem Search Command
- New keybinding for filesystem search (e.g., `Ctrl+O`)
- Reuse existing `directoryBrowser.go` with fuzzy search
- Integration with session creation workflow

#### 2.2 Search Scope Options
- Recent directories
- Git repositories
- Bookmarked locations
- Current working directory tree

## Technical Architecture

### SearchItem Interface Implementation
```go
// Session adapter for fuzzy search
type SessionSearchItem struct {
    Instance *session.Instance
}

func (s SessionSearchItem) GetSearchText() string {
    parts := []string{
        s.Instance.Title,           // Highest priority
        s.Instance.Category,        // Organization context
        s.Instance.Program,         // Tool context
        s.Instance.Branch,          // Git context
        filepath.Base(s.Instance.Path), // Repo name
        s.Instance.WorkingDir,      // Directory context
    }
    return strings.Join(parts, " ")
}

func (s SessionSearchItem) GetDisplayText() string {
    // Use existing session display formatting
    return formatSessionForDisplay(s.Instance)
}

func (s SessionSearchItem) GetID() string {
    return s.Instance.Title
}
```

### Integration Points
- **UI Layer**: `ui/list.go` - Replace `SearchByTitle()` method
- **Search Engine**: `ui/fuzzy/fuzzy.go` - Existing, no changes needed
- **Display**: Maintain existing session list rendering
- **State Management**: Preserve current search mode handling

## Success Criteria

### Functional Requirements
- [x] Multi-field search across all session properties
- [x] Real-time fuzzy matching with debouncing
- [x] Score-based result ranking
- [x] Match highlighting for visual feedback
- [x] Maintain existing keybindings and UI patterns

### Performance Requirements
- Search responds within 50ms for typical session lists (20-50 items)
- Handles large session lists (100+ items) smoothly
- No memory leaks from search operations
- UI remains responsive during search

### Quality Requirements
- Feels as responsive as VS Code Ctrl+P or FZF
- All session fields searchable with intuitive matching
- Results update dynamically without freezing
- Backward compatible with existing workflows

## Risk Mitigation

### Technical Risks
1. **Performance degradation**: Existing fuzzy engine already optimized
2. **Search confusion**: Weight title matches highest, clear field priorities
3. **UI compatibility**: Maintain same keybindings and flow

### Rollback Strategy
- Current simple search remains as fallback
- Incremental deployment possible
- Can revert to `strings.Contains()` if needed

## Testing Strategy

### Unit Tests
- SessionSearchItem implementation
- Multi-field search text generation
- Fuzzy score validation
- Edge cases (empty queries, special chars)

### Integration Tests
- UI search interaction
- Performance with large session lists
- Memory usage validation
- Search state persistence

### User Acceptance
- Search feels natural and fast
- Results are relevant and well-ranked
- Existing muscle memory preserved

## Implementation Timeline

### Phase 1: Core Enhancement (2-3 hours)
1. **Hour 1**: SessionSearchItem implementation and testing
2. **Hour 2**: Integration with existing search UI
3. **Hour 3**: Testing, refinement, and documentation

### Phase 2: Filesystem Search (Future - 1 hour)
1. Add filesystem search command
2. Integrate with directory browser
3. Test and document

## Dependencies
- **None**: Leverages existing fuzzy search infrastructure
- **Minimal risk**: Well-tested components being enhanced
- **No external libraries**: Pure Go implementation

## Expected Impact
- **High user satisfaction**: Modern search experience
- **Improved productivity**: Find sessions faster across multiple fields
- **Future extensibility**: Foundation for filesystem search and other fuzzy features
- **Maintains performance**: Built on existing optimized engine

---

*This plan leverages claude-squad's existing excellent fuzzy search infrastructure to provide a best-in-class session search experience while maintaining the application's performance and usability standards.*