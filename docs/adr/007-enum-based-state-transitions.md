# ADR-007: Enum-Based State Transitions with Backward Compatibility

## Status
**Proposed** - Implementation in progress as part of app architecture refactoring

## Context

During the app architecture refactoring (Epic: App Architecture Refactoring, Task 1.2), we identified the need to improve state management by moving from magic strings to type-safe enums. The current implementation uses string-based menu states and direct state assignments, which are error-prone and difficult to refactor.

### Current Issues
1. **Magic Strings**: Menu states use strings like "Default", "CreatingInstance", "Search" with no compile-time validation
2. **Type Safety**: No compile-time checking for invalid state transitions
3. **Refactoring Risk**: String-based state references are fragile during code changes
4. **Serialization Concerns**: State persistence may break if enum names or order change

### Requirements
- **Type Safety**: Compile-time validation of state transitions
- **Backward Compatibility**: Existing serialized state data must remain valid
- **Forward Compatibility**: Future enum changes must not break existing data
- **Performance**: No significant runtime overhead
- **Developer Experience**: Clear, maintainable state management API

## Decision

We will implement **enum-based state transitions with serialization stability** using the following approach:

### 1. Enum Design with Stable Serialization

```go
// State represents application states with stable serialization
type State int

const (
    Default State = iota       // Serializes as 0 - NEVER CHANGE ORDER
    New                        // Serializes as 1
    Prompt                     // Serializes as 2
    Help                       // Serializes as 3
    Confirm                    // Serializes as 4
    CreatingSession            // Serializes as 5
    AdvancedNew                // Serializes as 6
    Git                        // Serializes as 7
    ClaudeSettings             // Serializes as 8
    ZFSearch                   // Serializes as 9
    // NEW STATES MUST BE ADDED AT THE END TO PRESERVE ORDER
)
```

### 2. Menu State Enum with String Mapping

```go
// MenuState represents UI menu states
type MenuState int

const (
    MenuDefault MenuState = iota
    MenuCreatingInstance
    MenuSearch
    MenuPrompt
    MenuHelp
)

// String provides human-readable representation
func (m MenuState) String() string {
    switch m {
    case MenuDefault:
        return "Default"
    case MenuCreatingInstance:
        return "CreatingInstance"
    // ... etc
    }
}

// FromString parses string back to enum (for legacy compatibility)
func MenuStateFromString(s string) MenuState {
    switch s {
    case "Default":
        return MenuDefault
    case "CreatingInstance":
        return MenuCreatingInstance
    // ... etc
    }
}
```

### 3. Backward Compatible Serialization

```go
// MarshalJSON provides stable serialization
func (s State) MarshalJSON() ([]byte, error) {
    return json.Marshal(struct{
        Value int    `json:"value"`
        Name  string `json:"name"`
    }{
        Value: int(s),
        Name:  s.String(),
    })
}

// UnmarshalJSON handles both old and new formats
func (s *State) UnmarshalJSON(data []byte) error {
    // Try new format first
    var obj struct {
        Value int    `json:"value"`
        Name  string `json:"name"`
    }
    if err := json.Unmarshal(data, &obj); err == nil {
        *s = State(obj.Value)
        return nil
    }

    // Fallback to old string format
    var str string
    if err := json.Unmarshal(data, &str); err != nil {
        return err
    }
    *s = StateFromString(str) // Legacy string conversion
    return nil
}
```

### 4. Transition Context with Enum Safety

```go
type TransitionContext struct {
    MenuState    MenuState  // Type-safe menu state
    OverlayName  string     // Keep as string (UI-specific)
    // ... other fields
}
```

### 5. Migration Strategy

**Phase 1: Parallel Implementation** (Current)
- Implement new enum system alongside existing string system
- Use mapping functions to maintain compatibility
- All new code uses enums, legacy code continues to work

**Phase 2: Gradual Migration**
- Replace string-based state references with enum equivalents
- Update serialization format with backward compatibility
- Maintain dual support during transition

**Phase 3: Legacy Cleanup** (Future)
- Remove string-based state system after confirmation of stability
- Keep backward-compatible deserialization for existing user data

## Implementation Benefits

### Type Safety
```go
// OLD: Runtime error possible
m.menu.SetState("Defalt") // Typo causes runtime issue

// NEW: Compile-time error
m.menu.SetState(state.Defalt) // Compile error - typo caught early
```

### Refactoring Safety
```go
// OLD: String search/replace required, error-prone
grep -r "CreatingInstance" # Manual search needed

// NEW: IDE-supported refactoring
// Rename symbol automatically updates all references
```

### Forward Compatibility
```go
// Adding new states preserves existing serialized data
const (
    Default State = iota // 0 - never changes
    New                  // 1 - never changes
    // ... existing states preserve their values
    NewFeatureState      // 10 - new state gets next available value
)
```

## Implementation Guidelines

### Enum Stability Rules
1. **NEVER reorder existing enum values**
2. **NEVER remove enum values** (mark as deprecated instead)
3. **ALWAYS add new values at the end**
4. **ALWAYS provide String() methods for debugging**
5. **ALWAYS implement backward-compatible unmarshaling**

### Serialization Best Practices
```go
// Store both value and name for debugging/validation
type SerializableState struct {
    Value int    `json:"value"`
    Name  string `json:"name,omitempty"`
}

// Validate on deserialization
func (s *State) UnmarshalJSON(data []byte) error {
    // ... unmarshal logic
    if !s.IsValid() {
        return fmt.Errorf("invalid state value: %d", int(*s))
    }
    return nil
}
```

### Testing Requirements
- **Serialization round-trip tests**: Ensure data survives serialize/deserialize cycles
- **Backward compatibility tests**: Verify old serialized data can be read
- **Forward compatibility tests**: Ensure unknown values are handled gracefully
- **Type safety tests**: Verify compile-time enum validation

## Consequences

### Positive
- **Compile-time safety**: Eliminates magic string errors
- **Better IDE support**: Auto-completion, refactoring, find usages
- **Maintainable**: Type-safe state transitions and clear APIs
- **Future-proof**: Serialization stability handles evolution
- **Performance**: Enum comparisons faster than string comparisons

### Negative
- **Migration complexity**: Dual system during transition period
- **Serialization overhead**: JSON format slightly larger due to dual value/name storage
- **Developer learning**: Team needs to understand enum stability rules

### Risks & Mitigations
- **Risk**: Accidental enum reordering breaks serialization
  - **Mitigation**: Automated tests, clear documentation, code review guidelines
- **Risk**: Legacy data becomes unreadable
  - **Mitigation**: Maintain backward-compatible deserialization indefinitely
- **Risk**: Performance regression during dual-system period
  - **Mitigation**: Profile critical paths, optimize conversion functions

## Implementation Status

**✅ Phase 1 Complete**: Enum system implemented in `app/state/` package
- State enum with stable ordering
- Menu state integration
- Backward-compatible helper methods
- Comprehensive test coverage

**🔄 Phase 2 In Progress**: BubbleTea integration
- Helper methods for state transitions
- Legacy state mapping for compatibility
- Partial migration of direct state assignments

**📅 Phase 3 Planned**: Full migration and cleanup
- Complete string-to-enum migration
- Remove legacy state system
- Performance optimization

## References
- [Epic: App Architecture Refactoring](../tasks/app-refactoring.md)
- [Go Enums Best Practices](https://blog.golang.org/constants)
- [JSON Compatibility Patterns](https://blog.golang.org/json)

---
**Decision Date**: 2025-01-19
**Decision Makers**: Claude Code Assistant, stapler-squad development team
**Review Date**: After Phase 2 completion