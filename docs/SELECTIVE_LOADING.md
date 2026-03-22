# Selective Data Loading in Stapler Squad

## Overview

Stapler Squad's SQLite repository now supports selective data loading through `LoadOptions`. This allows you to control exactly what child data is loaded for each session, optimizing performance and memory usage.

## Performance Impact

| Operation | Before | After (LoadSummary) | After (LoadMinimal) | Improvement |
|-----------|--------|---------------------|---------------------|-------------|
| List() | 31 MB | ~5 KB | ~2 KB | 99.98% |
| Get() | Full | Full | Custom | Unchanged |
| Memory per session | 1-25 MB | 1-2 KB | ~100 bytes | 99.5% |

## LoadOptions Presets

### LoadMinimal
Loads only core session fields without any child data.

```go
sessions, err := repo.ListWithOptions(ctx, session.LoadMinimal)
// Loaded: Title, Path, Status, Timestamps, etc.
// NOT loaded: Worktree, DiffStats, Tags, ClaudeSession
```

**Use when:**
- Quick session counts
- Status checks only
- Minimal memory footprint needed

**Memory:** ~100 bytes per session

### LoadSummary (Default for List operations)
Loads lightweight child data suitable for list views.

```go
sessions, err := repo.ListWithOptions(ctx, session.LoadSummary)
// Loaded: Worktree, DiffStats counts, Tags, ClaudeSession
// NOT loaded: Diff content (can be 1-25 MB)
```

**Use when:**
- Displaying session lists (TUI, Web UI)
- Session overview pages
- Navigation and filtering

**Memory:** ~1-2 KB per session

### LoadFull (Default for Get operations)
Loads all data including full diff content.

```go
sessionData, err := repo.GetWithOptions(ctx, "my-session", session.LoadFull)
// Loaded: Everything including full diff content
```

**Use when:**
- Viewing individual session details
- Displaying diff preview
- Full session inspection

**Memory:** Can be 1-25 MB per session (depends on diff size)

### LoadDiffOnly
Loads only diff-related data for preview panes.

```go
sessionData, err := repo.GetWithOptions(ctx, "my-session", session.LoadDiffOnly)
// Loaded: Worktree, DiffStats, Diff content
// NOT loaded: Tags, ClaudeSession
```

**Use when:**
- Diff preview pane
- Change analysis
- Git operations

### LoadForReviewQueue
Optimized for review queue operations.

```go
sessions, err := repo.ListWithOptions(ctx, session.LoadForReviewQueue)
// Loaded: Worktree, DiffStats counts, Tags
// NOT loaded: Diff content, ClaudeSession
```

**Use when:**
- Review queue listing
- Change notifications
- Activity tracking

## Custom LoadOptions

You can create custom options for specific use cases:

```go
// Load only what you need
customOptions := session.LoadOptions{
    LoadWorktree:      true,  // Need branch info
    LoadDiffStats:     true,  // Need change counts
    LoadDiffContent:   false, // Don't need full diff
    LoadTags:          true,  // Need tags for filtering
    LoadClaudeSession: false, // Don't need Claude session
}

sessions, err := repo.ListWithOptions(ctx, customOptions)
```

## Builder Methods

LoadOptions supports fluent builder methods:

```go
// Start with minimal and add what you need
options := session.LoadMinimal.
    WithTags().
    WithDiffContent()

// Or remove what you don't need
options := session.LoadFull.
    WithoutDiffContent()
```

## Migration Guide

### Old Code
```go
// Loads everything including 31 MB of diff content
sessions, err := repo.List(ctx)

// Also loads everything
sessionData, err := repo.Get(ctx, "my-session")
```

### New Code (Optimized)
```go
// Use default (summary - no diff content)
sessions, err := repo.List(ctx)  // Now uses LoadSummary internally

// Or be explicit about what you need
sessions, err := repo.ListWithOptions(ctx, session.LoadMinimal)

// Get still loads everything for detail views
sessionData, err := repo.Get(ctx, "my-session")  // Uses LoadFull internally

// Or customize for specific needs
sessionData, err := repo.GetWithOptions(ctx, "my-session", session.LoadDiffOnly)
```

## Common Use Cases

### 1. Session List (TUI/Web UI)
```go
// Use default List() which now uses LoadSummary
sessions, err := repo.List(ctx)
// Shows: Title, branch, diff counts, tags, status
// Memory: ~1-2 KB per session instead of 1-25 MB
```

### 2. Session Detail View with Diff
```go
// Use default Get() which loads everything
sessionData, err := repo.Get(ctx, sessionTitle)
// Shows: Full session details including diff content
// Memory: Full data as needed
```

### 3. Quick Status Check
```go
// Minimal loading for fast status checks
sessions, err := repo.ListWithOptions(ctx, session.LoadMinimal)
for _, s := range sessions {
    fmt.Printf("%s: %v\n", s.Title, s.Status)
}
// Memory: ~100 bytes per session
```

### 4. Tag-Based Filtering
```go
// Load sessions with specific tag, summary data only
sessions, err := repo.ListByTag(ctx, "Frontend")
// Uses LoadSummary internally - no diff content
```

### 5. Review Queue
```go
// Optimized for review queue with only needed data
options := session.LoadForReviewQueue
sessions, err := repo.ListByStatusWithOptions(ctx, session.Ready, options)
```

## Performance Best Practices

1. **Use presets when possible** - They're optimized for common scenarios
2. **Avoid loading diff content in lists** - Can be 1-25 MB per session
3. **Load diff content only when viewing** - Use Get() for detail views
4. **Customize for specific needs** - Don't load data you won't use
5. **Use LoadMinimal for counts** - When you only need session counts

## API Summary

```go
// Repository interface methods
Get(ctx, title) (*InstanceData, error)                                    // LoadFull
GetWithOptions(ctx, title, options) (*InstanceData, error)                // Custom

List(ctx) ([]InstanceData, error)                                         // LoadSummary
ListWithOptions(ctx, options) ([]InstanceData, error)                     // Custom

ListByStatus(ctx, status) ([]InstanceData, error)                         // LoadSummary
ListByStatusWithOptions(ctx, status, options) ([]InstanceData, error)     // Custom

ListByTag(ctx, tag) ([]InstanceData, error)                               // LoadSummary
ListByTagWithOptions(ctx, tag, options) ([]InstanceData, error)           // Custom
```

## Backward Compatibility

All existing code continues to work without changes:
- `List()` now uses `LoadSummary` internally (still fast, no breaking changes)
- `Get()` still loads full data as expected
- New `WithOptions` methods are opt-in for optimization

## Implementation Notes

The selective loading is implemented through a helper method:
```go
func (r *SQLiteRepository) loadChildDataWithOptions(
    ctx context.Context,
    sessionID int64,
    data *InstanceData,
    options LoadOptions,
) error
```

This ensures consistent behavior across all loading methods and makes it easy to add new child data types in the future.
