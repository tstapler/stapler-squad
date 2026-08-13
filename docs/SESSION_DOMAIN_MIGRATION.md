# Session Domain Model Migration Guide

This document describes the migration from the legacy Instance/InstanceData types to the new Session domain model with optional contexts.

## Overview

The new Session domain model provides:
- **Cleaner separation of concerns** via optional contexts
- **Support for multiple deployment scenarios** (local, cloud, ephemeral)
- **Optimized query performance** through selective context loading
- **Domain-driven design** with clear bounded contexts

## Architecture

### Type Hierarchy

```
┌─────────────────────────────────────────────────────────────────┐
│                      Legacy Types (Keep)                        │
├─────────────────────────────────────────────────────────────────┤
│  Instance          - Runtime object with tmux/git operations    │
│  InstanceData      - Persistence/serialization DTO              │
│  LoadOptions       - Selective loading for InstanceData         │
└─────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────┐
│                      New Types (Session Model)                  │
├─────────────────────────────────────────────────────────────────┤
│  Session           - Core domain entity (minimal required data) │
│  ContextOptions    - Selective loading for Session              │
│  GitContext        - Git repository, branch, PR integration     │
│  FilesystemContext - Paths, worktree detection                  │
│  TerminalContext   - Dimensions, tmux configuration             │
│  UIPreferences     - Categories, tags, display settings         │
│  ActivityTracking  - Timestamps, output signatures              │
│  CloudContext      - Cloud provider, API configuration          │
└─────────────────────────────────────────────────────────────────┘
```

### When to Use Each Type

| Type | Use Case |
|------|----------|
| `Instance` | Runtime operations: starting tmux, capturing output, executing commands |
| `InstanceData` | Persistence with InstanceStorage, JSON serialization |
| `Session` | New deployment scenarios, cloud sessions, API clients |

## Migration Guide

### For New Code

Use the Session-based API for new features:

```go
// Query with selective context loading
ctx := context.Background()
opts := session.ContextUIView  // Load UI + Activity + Git + Tags

sessions, err := repo.ListSessions(ctx, opts)
for _, s := range sessions {
    fmt.Printf("%s: branch=%s, category=%s\n",
        s.Title,
        s.GetBranch(),      // Safe accessor (returns "" if no Git context)
        s.GetCategory())    // Safe accessor (returns "" if no UI context)
}

// Create a cloud session (no local git/filesystem)
cloudSession := session.NewSession("my-api-session", "claude")
cloudSession.WithCloudContext(&session.CloudContext{
    Provider:    "aws",
    APIEndpoint: "https://api.example.com",
})
err = repo.CreateSession(ctx, cloudSession)
```

### For Existing Code

Existing code using `Instance` and `InstanceData` continues to work. The adapter methods enable interoperability:

```go
// Convert Instance to Session
inst := &session.Instance{Title: "my-session", ...}
s := inst.ToSession()  // Or session.InstanceToSession(inst)

// Convert Session back to Instance
inst2 := session.SessionToInstance(s)

// Convert LoadOptions to ContextOptions
oldOpts := session.LoadSummary
newOpts := session.FromLoadOptions(oldOpts)

// Convert ContextOptions to LoadOptions
ctxOpts := session.ContextUIView
loadOpts := ctxOpts.ToLoadOptions()
```

### Preset Mapping

| Old Preset (LoadOptions) | New Preset (ContextOptions) |
|--------------------------|----------------------------|
| `LoadMinimal` | `ContextMinimal` |
| `LoadSummary` | `ContextUIView` |
| `LoadFull` | `ContextFull` |
| `LoadDiffOnly` | `ContextTerminalView.WithDiffContent()` |
| `LoadForReviewQueue` | `ContextForReviewQueue` |

### Repository Methods

The Repository interface now supports both APIs:

```go
// Legacy API (InstanceData)
repo.Get(ctx, title)                        // Full load
repo.GetWithOptions(ctx, title, LoadFull)   // Selective load
repo.List(ctx)                              // Summary load
repo.ListWithOptions(ctx, LoadMinimal)      // Selective load

// New API (Session)
repo.GetSession(ctx, title, ContextUIView)  // Selective contexts
repo.ListSessions(ctx, ContextMinimal)      // List with contexts
repo.CreateSession(ctx, session)            // Create from Session
repo.UpdateSession(ctx, session)            // Update from Session
```

## Context Loading Presets

| Preset | Memory | Use Case |
|--------|--------|----------|
| `ContextMinimal` | ~500B | Basic metadata only |
| `ContextUIView` | ~2-3KB | List/card display |
| `ContextTerminalView` | ~5-10KB | Terminal preview |
| `ContextDetailView` | ~10-20KB | Session detail panel |
| `ContextFull` | 1-25MB | Complete data with diffs |
| `ContextCloudSession` | ~1-2KB | Remote/API sessions |
| `ContextForSearch` | ~1-2KB | Search operations |
| `ContextForReviewQueue` | ~3-5KB | Review queue operations |

## Database Schema

The schema (v3) includes normalized context tables:

```sql
-- Core session (always loaded)
sessions (id, title, status, program, auto_yes, prompt, created_at, updated_at, ...)

-- Optional contexts (loaded based on ContextOptions)
git_context (session_id, branch, base_commit_sha, pr_number, pr_url, ...)
filesystem_context (session_id, project_path, working_dir, is_worktree, ...)
terminal_context (session_id, height, width, tmux_session_name, ...)
ui_preferences (session_id, category, is_expanded, grouping_strategy, ...)
activity_tracking (session_id, last_terminal_update, last_meaningful_output, ...)
cloud_context (session_id, provider, region, api_endpoint, ...)
```

## Deployment Scenarios

The Session model supports various deployment modes:

### Local Development (Traditional)
```go
// Full local context: git + filesystem + terminal + UI
opts := ContextDetailView
s, _ := repo.GetSession(ctx, title, opts)
// s.Git, s.Filesystem, s.Terminal all populated
```

### Ephemeral Containers
```go
// Minimal context: just terminal streaming
s := NewSession("ephemeral-worker", "claude")
s.WithTerminalContext(&TerminalContext{
    Height: 24, Width: 80,
})
// No Git, Filesystem, or Cloud context
```

### Cloud API Sessions
```go
// Cloud context: API-based, no local resources
s := NewSession("api-session", "claude")
s.WithCloudContext(&CloudContext{
    Provider:       "aws",
    APIEndpoint:    "https://bedrock.us-east-1.amazonaws.com",
    CloudSessionID: "sess-123",
})
// No Git, Filesystem, or Terminal context
```

### Hybrid Mode
```go
// Local git + cloud API
s := NewSession("hybrid", "claude")
s.WithGitContext(&GitContext{Branch: "feature/x"})
s.WithCloudContext(&CloudContext{Provider: "anthropic"})
```

## Safe Accessors

Session provides nil-safe accessors that return sensible defaults:

```go
s.GetBranch()             // "" if no Git context
s.GetPath()               // "" if no Filesystem context
s.GetCategory()           // "" if no UI context
s.GetTags()               // []string{} if no UI context
s.GetTmuxSessionName()    // "" if no Terminal context
s.GetLastViewed()         // time.Time{} if no Activity context
s.NeedsReviewQueueAttention() // false if no Activity context
s.IsCloudConfigured()     // false if no Cloud context or not configured
```

## Context Checkers

Verify context availability before accessing detailed fields:

```go
if s.HasGitContext() {
    fmt.Printf("PR #%d: %s\n", s.Git.PRNumber, s.Git.PRURL)
}

if s.HasCloudContext() && s.Cloud.IsConfigured() {
    // Use cloud-specific features
}
```

## Builder Pattern

Build sessions fluently:

```go
s := NewSession("my-session", "claude").
    WithGitContext(&GitContext{Branch: "main"}).
    WithTerminalContext(&TerminalContext{Width: 120, Height: 40}).
    WithUIPreferences(&UIPreferences{Category: "Development"})
```

## Migration Checklist

For gradually migrating existing code:

- [ ] New features use `Session` and `ContextOptions`
- [ ] Existing Instance/InstanceData code continues to work
- [ ] Use adapters (`InstanceToSession`, `SessionToInstance`) at boundaries
- [ ] Replace `LoadOptions` presets with `ContextOptions` equivalents
- [ ] Use safe accessors instead of direct context field access
- [ ] Add context presence checks before accessing nested fields
