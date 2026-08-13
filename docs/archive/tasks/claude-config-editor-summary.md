# Claude Config Editor Implementation Summary

## Quick Reference

**Feature Plan**: `/Users/tylerstapler/IdeaProjects/stapler-squad/docs/tasks/claude-config-editor.md`  
**Epic ID**: FEATURE-001  
**Estimated Effort**: 3-4 weeks  
**Status**: Planning Complete

## Key Deliverables

### 1. Backend Services (Go)

**New Files to Create**:
- `/Users/tylerstapler/IdeaProjects/stapler-squad/config/claude.go` - ClaudeConfigManager
- `/Users/tylerstapler/IdeaProjects/stapler-squad/config/claude_test.go` - Unit tests
- `/Users/tylerstapler/IdeaProjects/stapler-squad/session/history.go` - ClaudeHistoryParser
- `/Users/tylerstapler/IdeaProjects/stapler-squad/session/history_test.go` - Unit tests
- `/Users/tylerstapler/IdeaProjects/stapler-squad/session/conversation.go` - ConversationStore
- `/Users/tylerstapler/IdeaProjects/stapler-squad/session/conversation_test.go` - Unit tests

**Files to Modify**:
- `/Users/tylerstapler/IdeaProjects/stapler-squad/proto/session/v1/session.proto` - Add new RPCs
- `/Users/tylerstapler/IdeaProjects/stapler-squad/proto/session/v1/types.proto` - Add new message types
- `/Users/tylerstapler/IdeaProjects/stapler-squad/server/services/session_service.go` - Implement new RPC handlers

### 2. TUI Components (Go)

**New Files to Create**:
- `/Users/tylerstapler/IdeaProjects/stapler-squad/ui/overlay/configEditorOverlay.go` - Config editor overlay
- `/Users/tylerstapler/IdeaProjects/stapler-squad/ui/overlay/configEditorOverlay_test.go` - Unit tests
- `/Users/tylerstapler/IdeaProjects/stapler-squad/ui/overlay/sessionHistoryBrowserOverlay.go` - History browser
- `/Users/tylerstapler/IdeaProjects/stapler-squad/ui/overlay/sessionHistoryBrowserOverlay_test.go` - Unit tests
- `/Users/tylerstapler/IdeaProjects/stapler-squad/ui/overlay/historyDetailOverlay.go` - Conversation viewer
- `/Users/tylerstapler/IdeaProjects/stapler-squad/ui/overlay/historyDetailOverlay_test.go` - Unit tests
- `/Users/tylerstapler/IdeaProjects/stapler-squad/ui/syntax/highlighter.go` - Syntax highlighting (if needed)

**Files to Modify**:
- `/Users/tylerstapler/IdeaProjects/stapler-squad/app/app.go` - Register new overlays with UI coordinator
- `/Users/tylerstapler/IdeaProjects/stapler-squad/keys/keys.go` - Add key bindings (C for config, H for history)
- `/Users/tylerstapler/IdeaProjects/stapler-squad/keys/help.go` - Add key help entries

### 3. Web UI Components (TypeScript/React)

**New Files to Create**:
- `/Users/tylerstapler/IdeaProjects/stapler-squad/web-app/src/components/config/ConfigEditor.tsx`
- `/Users/tylerstapler/IdeaProjects/stapler-squad/web-app/src/components/config/ConfigEditor.module.css`
- `/Users/tylerstapler/IdeaProjects/stapler-squad/web-app/src/components/config/ConfigEditor.test.tsx`
- `/Users/tylerstapler/IdeaProjects/stapler-squad/web-app/src/components/sessions/SessionHistoryBrowser.tsx`
- `/Users/tylerstapler/IdeaProjects/stapler-squad/web-app/src/components/sessions/SessionHistoryBrowser.module.css`
- `/Users/tylerstapler/IdeaProjects/stapler-squad/web-app/src/components/sessions/SessionHistoryBrowser.test.tsx`
- `/Users/tylerstapler/IdeaProjects/stapler-squad/web-app/src/components/sessions/ConversationViewer.tsx`
- `/Users/tylerstapler/IdeaProjects/stapler-squad/web-app/src/components/sessions/ConversationViewer.module.css`
- `/Users/tylerstapler/IdeaProjects/stapler-squad/web-app/src/app/config/page.tsx` - Config editor page
- `/Users/tylerstapler/IdeaProjects/stapler-squad/web-app/src/app/history/page.tsx` - History browser page
- `/Users/tylerstapler/IdeaProjects/stapler-squad/web-app/src/lib/hooks/useClaudeConfig.ts` - Config management hook
- `/Users/tylerstapler/IdeaProjects/stapler-squad/web-app/src/lib/hooks/useSessionHistory.ts` - History management hook

### 4. Test Files

**Integration Tests**:
- `/Users/tylerstapler/IdeaProjects/stapler-squad/tests/integration/config_integration_test.go`
- `/Users/tylerstapler/IdeaProjects/stapler-squad/tests/integration/history_integration_test.go`

**E2E Tests** (Playwright):
- `/Users/tylerstapler/IdeaProjects/stapler-squad/tests/e2e/configEditor.spec.ts`
- `/Users/tylerstapler/IdeaProjects/stapler-squad/tests/e2e/sessionHistory.spec.ts`

**Test Fixtures**:
- `/Users/tylerstapler/IdeaProjects/stapler-squad/tests/fixtures/claude/CLAUDE.md`
- `/Users/tylerstapler/IdeaProjects/stapler-squad/tests/fixtures/claude/settings.json`
- `/Users/tylerstapler/IdeaProjects/stapler-squad/tests/fixtures/claude/history.jsonl`
- `/Users/tylerstapler/IdeaProjects/stapler-squad/tests/fixtures/claude/history_large.jsonl`
- `/Users/tylerstapler/IdeaProjects/stapler-squad/tests/fixtures/claude/history_corrupted.jsonl`
- `/Users/tylerstapler/IdeaProjects/stapler-squad/tests/fixtures/claude/__store.db`

## Architecture Patterns to Follow

### 1. Overlay Pattern (Existing)
- Extend `BaseOverlay` in `/Users/tylerstapler/IdeaProjects/stapler-squad/ui/overlay/base.go`
- Follow pattern from existing overlays like `sessionSetup.go`
- Register with `UICoordinator` in app.go

### 2. gRPC/Connect-RPC (Existing)
- Add RPCs to `proto/session/v1/session.proto`
- Follow naming convention: `GetClaudeConfig`, `UpdateClaudeConfig`, etc.
- Implement handlers in `server/services/session_service.go`

### 3. Key Management (Existing)
- Add keys to `keys/keys.go` KeyName enum
- Map keys in `GlobalKeyStringsMap`
- Add help text in `keys/help.go` KeyHelpMap
- Register in app.go key switch statement

### 4. Atomic File Operations
- Use temp file + atomic rename pattern
- Create backups before modifications
- Transaction-safe error handling

### 5. Streaming Parsers
- Use bufio.Scanner for large files
- Implement pagination for API responses
- Avoid loading entire files into memory

## Key Integration Points

### Session Management
```go
// Launch session from history
// File: session/instance.go
func NewInstanceFromHistory(history *SessionHistorySummary) (*Instance, error) {
    opts := InstanceOptions{
        Title:   history.GenerateTitle(),
        Path:    history.ProjectPath,
        Program: history.Program,
        Category: history.Category,
    }
    return NewInstance(opts)
}
```

### Config Directory
```go
// Reuse existing config directory resolution
// File: config/config.go
configDir, err := GetConfigDir() // Returns ~/.stapler-squad
claudeDir := filepath.Join(os.UserHomeDir(), ".claude")
```

### UI Coordinator
```go
// Register overlay with coordinator
// File: app/app.go
case keys.KeyConfig:
    h.uiCoordinator.ShowConfigEditor(configFile)
case keys.KeyHistory:
    h.uiCoordinator.ShowHistoryBrowser()
```

## Critical Implementation Details

### 1. File Locking (Prevent Concurrent Edit Conflicts)
```go
import "github.com/gofrs/flock"

lockFile := filepath.Join(claudeDir, ".config.lock")
lock := flock.New(lockFile)
if err := lock.Lock(); err != nil {
    return fmt.Errorf("config locked: %w", err)
}
defer lock.Unlock()
```

### 2. JSON Schema Validation
```go
import "github.com/xeipuuv/gojsonschema"

func ValidateSettings(content string) error {
    schema := gojsonschema.NewStringLoader(settingsSchema)
    document := gojsonschema.NewStringLoader(content)
    result, err := gojsonschema.Validate(schema, document)
    if err != nil {
        return err
    }
    if !result.Valid() {
        return formatValidationErrors(result.Errors())
    }
    return nil
}
```

### 3. Streaming History Parser
```go
func (p *ClaudeHistoryParser) ParseStreaming(offset, limit int) ([]*HistoryEntry, error) {
    file, _ := os.Open(p.historyPath)
    defer file.Close()
    
    scanner := bufio.NewScanner(file)
    scanner.Buffer(make([]byte, 1024*1024), 10*1024*1024) // 10MB buffer
    
    entries := make([]*HistoryEntry, 0, limit)
    lineNum := 0
    
    for scanner.Scan() {
        if lineNum < offset {
            lineNum++
            continue
        }
        if len(entries) >= limit {
            break
        }
        
        var entry HistoryEntry
        if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
            log.WarningLog.Printf("Skipped corrupted line %d: %v", lineNum, err)
            continue
        }
        
        entries = append(entries, &entry)
        lineNum++
    }
    
    return entries, scanner.Err()
}
```

### 4. SQLite Conversation Access
```go
import "database/sql"
import _ "github.com/mattn/go-sqlite3"

func (s *ConversationStore) GetByProject(projectPath string) ([]*Conversation, error) {
    db, err := sql.Open("sqlite3", s.dbPath)
    if err != nil {
        return nil, err
    }
    defer db.Close()
    
    // Query structure depends on actual __store.db schema
    // This is a placeholder - inspect actual schema first
    rows, err := db.Query(`
        SELECT id, project_id, timestamp, content
        FROM conversations
        WHERE project_id LIKE ?
        ORDER BY timestamp DESC
    `, "%"+projectPath+"%")
    // ... parse results
}
```

### 5. Virtual Scrolling (Web UI)
```typescript
import { FixedSizeList } from 'react-window';

export function SessionHistoryBrowser() {
    return (
        <FixedSizeList
            height={600}
            itemCount={sessions.length}
            itemSize={80}
            width="100%"
        >
            {({ index, style }) => (
                <SessionHistoryItem 
                    session={sessions[index]} 
                    style={style} 
                />
            )}
        </FixedSizeList>
    );
}
```

## Testing Commands

```bash
# Backend unit tests
go test ./config -v -cover
go test ./session -v -cover

# TUI overlay tests
go test ./ui/overlay -v -cover

# Integration tests
go test ./tests/integration -v

# Web UI tests
cd web-app && npm test

# E2E tests
cd tests/e2e && npm test

# Performance benchmarks
go test -bench=. -benchmem ./session -timeout=10m

# Test with large fixtures
go test ./session -run TestHistoryParser_LargeFile -v
```

## Dependencies to Add

### Backend (Go)
```bash
go get github.com/gofrs/flock  # File locking
go get github.com/xeipuuv/gojsonschema  # JSON validation
go get github.com/mattn/go-sqlite3  # SQLite driver
```

### Frontend (Web)
```bash
cd web-app
npm install @monaco-editor/react  # Code editor
npm install react-window  # Virtual scrolling
npm install react-markdown  # Markdown rendering
```

## Documentation Updates

**Files to Update**:
- `/Users/tylerstapler/IdeaProjects/stapler-squad/CLAUDE.md` - Add config editor and history viewer usage
- `/Users/tylerstapler/IdeaProjects/stapler-squad/README.md` - Add feature overview
- `/Users/tylerstapler/IdeaProjects/stapler-squad/docs/architecture.md` - Add architecture diagrams (if exists)

## Key Risks & Mitigation

1. **Claude Code Schema Changes**
   - Risk: history.jsonl or __store.db schema changes break parser
   - Mitigation: Schema version detection, multi-version support
   - Test: Multiple Claude Code versions

2. **Large File Performance**
   - Risk: 100MB+ files cause memory/performance issues
   - Mitigation: Streaming, pagination, virtual scrolling
   - Test: Benchmark with 100MB fixtures

3. **Concurrent Edit Conflicts**
   - Risk: Multiple instances editing same config simultaneously
   - Mitigation: File locking, version checking
   - Test: Integration test with concurrent edits

4. **Path Traversal Security**
   - Risk: Malicious client reads files outside ~/.claude
   - Mitigation: Path validation, whitelist
   - Test: Security test with path traversal attempts

## Success Criteria

- ✅ Can view Claude configs in TUI and Web UI
- ✅ Can edit configs with validation
- ✅ Can browse session history by project
- ✅ Can launch new session from history
- ✅ Can view conversation details (if available)
- ✅ All tests passing (>80% coverage)
- ✅ Performance benchmarks met (<500ms history parsing)
- ✅ No critical bugs in first month of production

## Next Steps

1. **Review this plan** with engineering team
2. **Create GitHub issues** for each story
3. **Set up project board** with 4 milestones (phases)
4. **Begin Phase 1** (Backend foundation)
5. **Weekly check-ins** to track progress

---

**For detailed implementation guidance, see the full feature plan**:  
`/Users/tylerstapler/IdeaProjects/stapler-squad/docs/tasks/claude-config-editor.md`
