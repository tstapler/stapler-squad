# TUI-006 - Text Locators Implementation

## Story
**As a** test writer
**I want** to find text elements in the terminal buffer
**So that** I can write assertions against specific content in TUI applications

## Acceptance Criteria
- [ ] **Given** text content in terminal buffer **When** I search for exact text **Then** all matching positions are returned
- [ ] **Given** text search with options **When** I use case-insensitive matching **Then** matches ignore case differences
- [ ] **Given** partial text **When** I search with partial matching **Then** substrings are found correctly

## Technical Requirements
### Implementation Details
- [ ] Implement `TextSelector` struct with matching options
- [ ] Add exact text matching algorithm
- [ ] Support case-sensitive and case-insensitive matching
- [ ] Implement partial text matching (contains)
- [ ] Add multi-line text search capabilities

### Files to Create/Modify
- [ ] `pkg/locator/text.go` - TextSelector implementation
- [ ] `pkg/locator/text_test.go` - Text locator tests
- [ ] `pkg/locator/locator.go` - Main Locator struct
- [ ] `pkg/locator/locator_test.go` - Locator tests
- [ ] `pkg/types/position.go` - Position utilities
- [ ] `pkg/types/position_test.go` - Position tests

### Dependencies
- **Depends on**: TUI-005 (ANSI Renderer)
- **Blocks**: TUI-007 (Basic Expectations)

## Definition of Done
- [ ] Text search finds all matching positions in buffer
- [ ] Case sensitivity options work correctly
- [ ] Partial matching works for substrings
- [ ] Multi-line text can be searched
- [ ] Performance is acceptable for large buffers
- [ ] Unicode text is handled correctly
- [ ] Unit tests cover all search scenarios and edge cases

## Estimate
**Story Points**: 3
**Time Estimate**: 4-6 hours

## Notes
- Consider Unicode normalization for text matching
- Implement efficient search algorithms for large buffers
- Handle whitespace and formatting differences
- Support line-aware vs character-stream searching

## Validation
### Test Cases
1. **Test Case**: Exact text matching
   - **Steps**: Create buffer with known text, search for exact matches
   - **Expected**: All exact matches found at correct positions

2. **Test Case**: Case-insensitive search
   - **Steps**: Search for text with different case
   - **Expected**: Matches found regardless of case

3. **Test Case**: Multi-line text search
   - **Steps**: Search for text spanning multiple lines
   - **Expected**: Multi-line matches found correctly

---
**Created**: 2025-01-17
**Phase**: Phase 1
**Priority**: Medium
**Status**: Not Started