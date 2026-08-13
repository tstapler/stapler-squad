# Web UI Session Creation - Future Enhancements

This document outlines planned enhancements for the session creation flow that require backend support or additional complexity.

## Completed Features (MVP)

✅ **Multi-step wizard** - 4-step process (Basic Info, Repository, Configuration, Review)
✅ **Form validation** - Real-time validation with clear error messages
✅ **Review step** - Configuration summary before creation
✅ **Custom commands** - Support for custom AI assistant commands
✅ **Error handling** - User-friendly error messages and loading states
✅ **Enhanced validation** - Better validation messages for all fields

## Planned Enhancements

### 1. Smart Path Discovery (High Priority)

**Backend RPC Required**: Path suggestion and validation service

```protobuf
// Suggested proto additions
message SuggestPathsRequest {
  string partial_path = 1;
  bool include_git_repos = 2;
  int32 max_results = 3;
}

message SuggestPathsResponse {
  repeated PathSuggestion suggestions = 1;
}

message PathSuggestion {
  string path = 1;
  bool is_git_repository = 2;
  string repository_name = 3;
  string current_branch = 4;
}
```

**Features**:
- Real-time path autocomplete as user types
- Highlight git repositories in suggestions
- Show recent/favorite directories
- Validate path existence and permissions

### 2. Git Repository Discovery (High Priority)

**Backend RPC Required**: Git metadata service

```protobuf
message DetectGitRepositoryRequest {
  string path = 1;
}

message DetectGitRepositoryResponse {
  bool is_git_repo = 1;
  string repo_name = 2;
  repeated string branches = 3;
  string current_branch = 4;
  string default_branch = 5;
}
```

**Features**:
- Automatic git repository detection when path is entered
- List available branches for selection
- Show current branch and default branch
- Suggest branch names based on session title

### 3. Branch Selection UI (Medium Priority)

**Dependencies**: Git repository discovery backend

**Features**:
- Radio button list of available branches
- Search/filter branches
- "Create new branch" option with base branch selection
- Visual indication of current vs default branch
- Branch validation (already exists, invalid characters)

### 4. Session Templates (Medium Priority)

**Backend RPC Required**: Template storage and retrieval

**Features**:
- Save current configuration as template
- Load existing templates
- Common templates (Claude Code, Aider Ollama, etc.)
- Template management (edit, delete)

### 5. Path History and Favorites (Low Priority)

**Backend Required**: User preferences storage

**Features**:
- Track recently used repository paths
- Star/favorite frequently used repositories
- Quick selection from favorites dropdown

### 6. Validation Enhancements (Low Priority)

**Backend RPC Required**: Async validation services

**Features**:
- Async path validation (exists, accessible, permissions)
- Branch name conflict detection
- Session name uniqueness check
- Git worktree availability check

### 7. Keyboard Navigation (Low Priority)

**No Backend Required** - Pure frontend enhancement

**Features**:
- Tab navigation through form fields
- Enter to advance to next step
- Escape to cancel
- Arrow keys in dropdowns
- Keyboard shortcuts (Ctrl+Enter to submit)

### 8. State Persistence (Low Priority)

**Frontend LocalStorage** - No backend required

**Features**:
- Save draft configuration to localStorage
- Resume incomplete session creation
- Clear draft after successful creation
- Auto-save as user types

## Implementation Priority

### Phase 1 (Backend Required)
1. Path suggestion RPC
2. Git repository detection RPC
3. Branch listing RPC

### Phase 2 (Frontend Enhancements)
1. Implement path autocomplete component
2. Implement git branch selector
3. Add keyboard navigation
4. Add draft persistence

### Phase 3 (Advanced Features)
1. Session templates
2. Path favorites
3. Advanced validation

## Technical Notes

### Debouncing Strategy
- Path suggestions: 300ms debounce
- Async validation: 500ms debounce
- Auto-save: 1000ms debounce

### Performance Considerations
- Cache git repository metadata (5 minutes)
- Limit path suggestions to 10 results
- Use virtual scrolling for branch lists with >100 items

### Error Handling
- Graceful degradation if backend services unavailable
- Fallback to manual input if suggestions fail
- Clear error messages for validation failures

## Related Documentation

- `/web-app/src/components/sessions/SessionWizard.tsx` - Main wizard implementation
- `/web-app/src/lib/validation/sessionSchema.ts` - Validation schema
- `/proto/session/v1/session.proto` - Backend RPC definitions
- `/docs/direct-claude-interface-integration.md` - Integration documentation
