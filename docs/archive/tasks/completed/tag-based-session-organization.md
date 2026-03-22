# Tag-Based Session Organization with Dynamic Grouping

## Epic Overview

**Goal**: Transform the rigid single-category session organization into a flexible tag-based system with dynamic group-by modes, enabling users to view and organize sessions by any attribute (category, branch, program, tags, status, etc.).

**Value Proposition**:
- **Multi-Dimensional Organization**: Sessions can belong to multiple groups via tags
- **Flexible Viewing**: Switch between different organizational perspectives instantly
- **Enhanced Discoverability**: Find sessions by any attribute without rigid categorization
- **Backward Compatible**: Existing category-based organization continues to work
- **Scalability**: Better organization as session count grows
- **User Workflow Alignment**: Group sessions by project phase, priority, client, technology stack, etc.

**Success Metrics**:
- Seamless backward compatibility with existing Category field (0% migration failures)
- <100ms group-by mode switching latency
- Support for 5+ grouping strategies (Category, Branch, Path, Program, Tag, Status, None)
- Tag-based multi-membership (sessions appear in multiple tag groups)
- 100% preservation of existing UI navigation patterns
- Comprehensive test coverage (>85%) for grouping engine

**Current State**:
- ✅ Single `Category` field in `session.Instance`
- ✅ Basic nested category support via "/" delimiter
- ✅ Hard-coded `OrganizeByCategory()` in `ui/list.go`
- ✅ Category expansion/collapse functionality
- ❌ No tag support for multi-dimensional organization
- ❌ No dynamic grouping strategy system
- ❌ No UI controls for switching grouping modes
- ❌ Limited to one organizational view at a time

**Target State**:
- Tags field in session data model with backward-compatible migration
- Generic grouping engine with pluggable strategies
- UI hotkey ('G') to cycle through grouping modes
- Tag editor overlay for tag management
- Multi-level grouping (primary + secondary strategies)
- Tag-based filtering integration with existing search

---

## Story 1: Data Model Enhancement - Tags Foundation (Week 1: 2 days)

**Objective**: Add Tags field to session data model with backward-compatible storage migration, preserving existing Category field for compatibility.

**Value**: Establishes the foundational data structure for multi-dimensional organization without disrupting existing functionality.

**Dependencies**: None (foundational)

### Task 1.1: Add Tags Field to Instance Struct (2h) - Small

**Scope**: Extend `Instance` and `InstanceData` structs with Tags field and migration logic.

**Files**:
- `session/instance.go` (modify) - Add Tags field to Instance struct
- `session/storage.go` (modify) - Add Tags to InstanceData and migration logic
- `session/types.go` (modify) - Add tag-related constants if needed

**Context**:
- Instance struct already has Category field (line 66)
- InstanceData is the serializable form for persistence
- ToInstanceData() method handles conversion (line 106)
- Need to preserve Category for backward compatibility
- Tags should be []string for flexibility
- Empty tags should serialize as omitempty

**Implementation**:
```go
// session/instance.go - Add to Instance struct (around line 66)
type Instance struct {
	// ... existing fields ...

	// Category is used for organizing sessions into groups (legacy)
	Category string

	// Tags enables multi-dimensional organization and grouping
	// Sessions can have multiple tags for flexible classification
	Tags []string

	// ... remaining fields ...
}

// session/storage.go - Add to InstanceData struct (around line 30)
type InstanceData struct {
	// ... existing fields ...

	// Category for backward compatibility
	Category   string   `json:"category,omitempty"`

	// Tags for flexible multi-dimensional organization
	Tags       []string `json:"tags,omitempty"`

	// ... remaining fields ...
}

// Update ToInstanceData() method
func (i *Instance) ToInstanceData() InstanceData {
	data := InstanceData{
		// ... existing fields ...
		Category:   i.Category,
		Tags:       i.Tags,
		// ... remaining fields ...
	}
	return data
}

// Update FromInstanceData() method (if exists)
func (i *Instance) FromInstanceData(data InstanceData) {
	// ... existing field assignments ...
	i.Category = data.Category
	i.Tags = data.Tags
	// ... remaining assignments ...
}
```

**Success Criteria**:
- Tags field added to Instance struct as []string
- Tags field added to InstanceData with json:"tags,omitempty"
- ToInstanceData() includes Tags in serialization
- FromInstanceData() restores Tags from storage
- Category field preserved and functional
- No breaking changes to existing session loading

**Testing**:
- Unit test: Instance with Tags serializes correctly
- Unit test: Instance without Tags (legacy) loads without error
- Unit test: Category field unaffected by Tags addition
- Integration test: Save and load session with Tags

**Dependencies**: None

**Status**: ⏳ Pending

---

### Task 1.2: Implement Tag Migration for Existing Categories (2h) - Small

**Scope**: Create migration logic to convert existing Category values to Tags on load for enhanced organization.

**Files**:
- `session/storage.go` (modify) - Add migration logic in LoadInstances()
- `session/storage_test.go` (modify) - Add migration test cases

**Context**:
- LoadInstances() method loads persisted sessions
- Migration should run automatically on load
- Category should be preserved (don't delete)
- Category → Tags mapping: "Work/Frontend" becomes tags ["Work", "Frontend"]
- Migration should be idempotent (safe to run multiple times)
- Default tag "Uncategorized" if no Category exists

**Implementation**:
```go
// session/storage.go - Add migration function
func migrateInstanceTags(data *InstanceData) {
	// Skip if Tags already populated
	if len(data.Tags) > 0 {
		return
	}

	// Migrate from Category field
	if data.Category != "" && data.Category != "Uncategorized" {
		// Split nested categories into individual tags
		parts := strings.Split(data.Category, "/")
		data.Tags = parts
	} else {
		// Default tag for uncategorized sessions
		data.Tags = []string{"Uncategorized"}
	}
}

// Update LoadInstances() to call migration
func (s *Storage) LoadInstances() ([]*Instance, error) {
	instancesData := s.state.GetInstances()
	instances := make([]*Instance, 0, len(instancesData))

	for _, data := range instancesData {
		// Run migration on each instance
		migrateInstanceTags(&data)

		// ... existing instance creation logic ...
	}

	return instances, nil
}
```

**Success Criteria**:
- Migration runs automatically on LoadInstances()
- Category "Work/Frontend" generates Tags ["Work", "Frontend"]
- Empty Category generates Tags ["Uncategorized"]
- Category field preserved after migration
- Migration is idempotent (no duplicate tags on reload)
- Existing sessions with Tags unchanged

**Testing**:
```go
func TestMigrateInstanceTags(t *testing.T) {
	tests := []struct {
		name           string
		inputCategory  string
		inputTags      []string
		expectedTags   []string
	}{
		{
			name:          "Simple category to tags",
			inputCategory: "Work",
			inputTags:     nil,
			expectedTags:  []string{"Work"},
		},
		{
			name:          "Nested category to multiple tags",
			inputCategory: "Work/Frontend/React",
			inputTags:     nil,
			expectedTags:  []string{"Work", "Frontend", "React"},
		},
		{
			name:          "Existing tags preserved",
			inputCategory: "Work",
			inputTags:     []string{"Custom", "Tag"},
			expectedTags:  []string{"Custom", "Tag"},
		},
		{
			name:          "Uncategorized default",
			inputCategory: "",
			inputTags:     nil,
			expectedTags:  []string{"Uncategorized"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := InstanceData{
				Category: tt.inputCategory,
				Tags:     tt.inputTags,
			}

			migrateInstanceTags(&data)

			if !reflect.DeepEqual(data.Tags, tt.expectedTags) {
				t.Errorf("Expected tags %v, got %v", tt.expectedTags, data.Tags)
			}
		})
	}
}
```

**Dependencies**: Task 1.1 (Tags field must exist)

**Status**: ⏳ Pending

---

### Task 1.3: Add Tag Management Methods to Instance (1h) - Micro

**Scope**: Implement helper methods for tag manipulation on Instance struct.

**Files**:
- `session/instance.go` (modify) - Add tag management methods

**Context**:
- Instance struct needs convenient tag operations
- Thread-safe access via stateMutex (already exists)
- Methods should be idempotent and defensive
- Common operations: Add, Remove, Has, Clear, Set

**Implementation**:
```go
// session/instance.go - Add tag management methods

// AddTag adds a tag to the instance if not already present
func (i *Instance) AddTag(tag string) {
	i.stateMutex.Lock()
	defer i.stateMutex.Unlock()

	// Skip empty tags
	if tag == "" {
		return
	}

	// Check if tag already exists
	for _, existingTag := range i.Tags {
		if existingTag == tag {
			return
		}
	}

	i.Tags = append(i.Tags, tag)
}

// RemoveTag removes a tag from the instance
func (i *Instance) RemoveTag(tag string) {
	i.stateMutex.Lock()
	defer i.stateMutex.Unlock()

	newTags := make([]string, 0, len(i.Tags))
	for _, existingTag := range i.Tags {
		if existingTag != tag {
			newTags = append(newTags, existingTag)
		}
	}
	i.Tags = newTags
}

// HasTag checks if the instance has a specific tag
func (i *Instance) HasTag(tag string) bool {
	i.stateMutex.RLock()
	defer i.stateMutex.RUnlock()

	for _, existingTag := range i.Tags {
		if existingTag == tag {
			return true
		}
	}
	return false
}

// GetTags returns a copy of the instance's tags
func (i *Instance) GetTags() []string {
	i.stateMutex.RLock()
	defer i.stateMutex.RUnlock()

	tagsCopy := make([]string, len(i.Tags))
	copy(tagsCopy, i.Tags)
	return tagsCopy
}

// SetTags replaces all tags with a new set
func (i *Instance) SetTags(tags []string) {
	i.stateMutex.Lock()
	defer i.stateMutex.Unlock()

	i.Tags = make([]string, len(tags))
	copy(i.Tags, tags)
}

// ClearTags removes all tags from the instance
func (i *Instance) ClearTags() {
	i.stateMutex.Lock()
	defer i.stateMutex.Unlock()

	i.Tags = []string{}
}
```

**Success Criteria**:
- AddTag adds tag if not present, idempotent
- RemoveTag removes tag if present
- HasTag correctly checks tag presence
- GetTags returns defensive copy (not reference)
- SetTags replaces all tags atomically
- All methods are thread-safe via stateMutex

**Testing**:
```go
func TestInstanceTagManagement(t *testing.T) {
	instance := &Instance{Tags: []string{"Initial"}}

	// Test AddTag
	instance.AddTag("New")
	assert.True(t, instance.HasTag("New"))
	instance.AddTag("New") // Idempotent
	assert.Equal(t, 2, len(instance.GetTags()))

	// Test RemoveTag
	instance.RemoveTag("Initial")
	assert.False(t, instance.HasTag("Initial"))

	// Test SetTags
	instance.SetTags([]string{"A", "B", "C"})
	assert.Equal(t, 3, len(instance.GetTags()))

	// Test GetTags returns copy
	tags := instance.GetTags()
	tags[0] = "Modified"
	assert.NotEqual(t, "Modified", instance.GetTags()[0])

	// Test ClearTags
	instance.ClearTags()
	assert.Equal(t, 0, len(instance.GetTags()))
}
```

**Dependencies**: Task 1.1 (Tags field must exist)

**Status**: ⏳ Pending

---

## Story 2: Grouping Engine - Dynamic Strategy System (Week 1-2: 3 days)

**Objective**: Implement flexible grouping engine that can organize sessions by any attribute using pluggable strategies.

**Value**: Core infrastructure enabling multiple organizational views without duplicating grouping logic.

**Dependencies**: Story 1 (Tags data model must exist)

### Task 2.1: Define GroupingStrategy Type and Constants (1h) - Micro

**Scope**: Create type-safe enumeration of grouping strategies.

**Files**:
- `ui/grouping.go` (create) - Grouping strategy types and constants
- `ui/grouping_test.go` (create) - Basic strategy tests

**Context**:
- List struct currently hard-codes category grouping
- Need extensible strategy system for future additions
- Strategies should be type-safe (not magic strings)
- Each strategy extracts different session attributes

**Implementation**:
```go
// ui/grouping.go
package ui

// GroupingStrategy determines how sessions are organized in the list
type GroupingStrategy int

const (
	// GroupByCategory organizes sessions by their Category field (default)
	GroupByCategory GroupingStrategy = iota

	// GroupByBranch organizes sessions by git branch name
	GroupByBranch

	// GroupByPath organizes sessions by repository path
	GroupByPath

	// GroupByProgram organizes sessions by program (claude, aider, etc.)
	GroupByProgram

	// GroupByTag creates separate groups for each tag (multi-membership)
	GroupByTag

	// GroupByStatus organizes sessions by status (Running, Paused, etc.)
	GroupByStatus

	// GroupBySessionType organizes by session type (directory, worktree, etc.)
	GroupBySessionType

	// GroupByNone disables grouping (flat list)
	GroupByNone
)

// String returns the human-readable name of the grouping strategy
func (g GroupingStrategy) String() string {
	switch g {
	case GroupByCategory:
		return "Category"
	case GroupByBranch:
		return "Branch"
	case GroupByPath:
		return "Path"
	case GroupByProgram:
		return "Program"
	case GroupByTag:
		return "Tag"
	case GroupByStatus:
		return "Status"
	case GroupBySessionType:
		return "Session Type"
	case GroupByNone:
		return "None (Flat)"
	default:
		return "Unknown"
	}
}

// GetGroupingStrategies returns all available grouping strategies
func GetGroupingStrategies() []GroupingStrategy {
	return []GroupingStrategy{
		GroupByCategory,
		GroupByBranch,
		GroupByPath,
		GroupByProgram,
		GroupByTag,
		GroupByStatus,
		GroupBySessionType,
		GroupByNone,
	}
}
```

**Success Criteria**:
- GroupingStrategy is type-safe enum
- All strategies have String() representation
- GetGroupingStrategies() returns complete list
- Strategies cover all major session attributes
- Code compiles without errors

**Testing**:
```go
func TestGroupingStrategy(t *testing.T) {
	// Test String() representation
	assert.Equal(t, "Category", GroupByCategory.String())
	assert.Equal(t, "Branch", GroupByBranch.String())

	// Test GetGroupingStrategies() completeness
	strategies := GetGroupingStrategies()
	assert.Equal(t, 8, len(strategies))
	assert.Contains(t, strategies, GroupByTag)
}
```

**Dependencies**: None

**Status**: ⏳ Pending

---

### Task 2.2: Implement getGroupKeys() Method for Key Extraction (3h) - Medium

**Scope**: Create method that extracts group keys from sessions based on active strategy.

**Files**:
- `ui/grouping.go` (modify) - Add getGroupKeys() method
- `ui/grouping_test.go` (modify) - Comprehensive extraction tests

**Context**:
- Different strategies extract different attributes
- Tag strategy returns multiple keys (multi-membership)
- Other strategies return single key
- Need defensive handling for nil/empty values
- Path strategy should abbreviate paths for readability
- Branch strategy should handle detached HEAD

**Implementation**:
```go
// ui/grouping.go - Add to grouping.go

// getGroupKeys extracts group key(s) for a session based on the strategy
// Returns multiple keys for multi-membership strategies (e.g., tags)
func (l *List) getGroupKeys(instance *session.Instance, strategy GroupingStrategy) []string {
	switch strategy {
	case GroupByCategory:
		// Use existing category path logic
		categoryPath := instance.GetCategoryPath()
		if len(categoryPath) == 0 {
			return []string{"Uncategorized"}
		}

		// Support nested categories (e.g., "Work/Frontend")
		if len(categoryPath) == 1 {
			return []string{categoryPath[0]}
		}
		return []string{strings.Join(categoryPath, "/")}

	case GroupByBranch:
		if instance.Branch != "" {
			return []string{instance.Branch}
		}
		return []string{"No Branch"}

	case GroupByPath:
		if instance.Path != "" {
			// Abbreviate path for readability
			return []string{abbreviatePath(instance.Path)}
		}
		return []string{"No Path"}

	case GroupByProgram:
		if instance.Program != "" {
			return []string{instance.Program}
		}
		return []string{"Unknown Program"}

	case GroupByTag:
		// Multi-membership: session appears in multiple groups
		tags := instance.GetTags()
		if len(tags) == 0 {
			return []string{"Untagged"}
		}
		return tags

	case GroupByStatus:
		return []string{instance.Status.String()}

	case GroupBySessionType:
		return []string{instance.SessionType.String()}

	case GroupByNone:
		// Flat list: all sessions in one group
		return []string{"All Sessions"}

	default:
		return []string{"Unknown"}
	}
}

// abbreviatePath shortens long paths for display
// Examples:
//   /Users/bob/projects/my-app -> ~/projects/my-app
//   /very/long/path/to/project -> .../to/project
func abbreviatePath(path string) string {
	// Replace home directory with ~
	homeDir, err := os.UserHomeDir()
	if err == nil && strings.HasPrefix(path, homeDir) {
		path = "~" + strings.TrimPrefix(path, homeDir)
	}

	// Truncate very long paths
	const maxLength = 50
	if len(path) > maxLength {
		parts := strings.Split(path, "/")
		if len(parts) > 3 {
			return ".../" + strings.Join(parts[len(parts)-2:], "/")
		}
	}

	return path
}
```

**Success Criteria**:
- Each strategy correctly extracts appropriate keys
- GroupByTag returns multiple keys for multi-membership
- Nil/empty values return sensible defaults
- Paths are abbreviated for readability
- Method handles all GroupingStrategy enum values
- No panics on edge cases (nil instance, missing fields)

**Testing**:
```go
func TestGetGroupKeys(t *testing.T) {
	list := &List{}

	tests := []struct {
		name     string
		instance *session.Instance
		strategy GroupingStrategy
		expected []string
	}{
		{
			name: "Category grouping",
			instance: &session.Instance{Category: "Work"},
			strategy: GroupByCategory,
			expected: []string{"Work"},
		},
		{
			name: "Nested category grouping",
			instance: &session.Instance{Category: "Work/Frontend"},
			strategy: GroupByCategory,
			expected: []string{"Work/Frontend"},
		},
		{
			name: "Tag multi-membership",
			instance: &session.Instance{Tags: []string{"Urgent", "Client", "Frontend"}},
			strategy: GroupByTag,
			expected: []string{"Urgent", "Client", "Frontend"},
		},
		{
			name: "Empty tags default",
			instance: &session.Instance{Tags: []string{}},
			strategy: GroupByTag,
			expected: []string{"Untagged"},
		},
		{
			name: "Branch grouping",
			instance: &session.Instance{Branch: "feature/new-ui"},
			strategy: GroupByBranch,
			expected: []string{"feature/new-ui"},
		},
		{
			name: "Program grouping",
			instance: &session.Instance{Program: "claude"},
			strategy: GroupByProgram,
			expected: []string{"claude"},
		},
		{
			name: "Status grouping",
			instance: &session.Instance{Status: session.Running},
			strategy: GroupByStatus,
			expected: []string{"Running"},
		},
		{
			name: "No grouping",
			instance: &session.Instance{},
			strategy: GroupByNone,
			expected: []string{"All Sessions"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keys := list.getGroupKeys(tt.instance, tt.strategy)
			assert.Equal(t, tt.expected, keys)
		})
	}
}
```

**Dependencies**: Task 2.1 (GroupingStrategy must exist)

**Status**: ⏳ Pending

---

### Task 2.3: Refactor OrganizeByCategory to OrganizeByStrategy (3h) - Medium

**Scope**: Generalize existing category organization to support any grouping strategy.

**Files**:
- `ui/list.go` (modify) - Refactor OrganizeByCategory() to OrganizeByStrategy()
- `ui/list_test.go` (modify) - Update tests for new method

**Context**:
- Current OrganizeByCategory() at line 828 is hard-coded
- categoryGroups map[string][]*session.Instance already exists
- Method already handles performance optimization (categoriesNeedUpdate)
- Need to preserve existing behavior for GroupByCategory
- Multi-membership support for GroupByTag (session in multiple groups)
- Maintain expansion state (groupExpanded map)

**Implementation**:
```go
// ui/list.go - Replace OrganizeByCategory() method

// OrganizeByStrategy organizes sessions into groups based on active strategy
// Supports both single-membership (Category, Branch) and multi-membership (Tag)
func (l *List) OrganizeByStrategy() {
	// Only reorganize if needed (performance optimization)
	if !l.categoriesNeedUpdate {
		return
	}

	// Reset category groups (reusing existing field name for compatibility)
	l.categoryGroups = make(map[string][]*session.Instance)

	// Group instances by active strategy
	for _, instance := range l.items {
		// Skip paused sessions if hidePaused is true
		if l.hidePaused && instance.Status == session.Paused {
			continue
		}

		// Extract group key(s) based on strategy
		groupKeys := l.getGroupKeys(instance, l.groupingStrategy)

		// Add instance to each group (enables multi-membership for tags)
		for _, groupKey := range groupKeys {
			// Initialize the group if it doesn't exist
			if _, exists := l.categoryGroups[groupKey]; !exists {
				l.categoryGroups[groupKey] = []*session.Instance{}

				// Initialize expansion state if it doesn't exist
				if _, expanded := l.groupExpanded[groupKey]; !expanded {
					// Default to expanded for new groups
					l.groupExpanded[groupKey] = true
				}
			}

			// Add instance to this group
			l.categoryGroups[groupKey] = append(l.categoryGroups[groupKey], instance)
		}
	}

	// Mark categories as updated
	l.categoriesNeedUpdate = false
	// Invalidate viewport cache since group count may have changed
	l.maxVisibleCacheValid = false
}
```

**Success Criteria**:
- Method supports all GroupingStrategy values
- Multi-membership works for GroupByTag (session in multiple groups)
- Single-membership works for other strategies
- Existing performance optimizations preserved (categoriesNeedUpdate)
- Expansion state (groupExpanded) maintained correctly
- No breaking changes to existing UI navigation

**Testing**:
```go
func TestOrganizeByStrategy(t *testing.T) {
	// Test single-membership grouping
	t.Run("Category strategy single membership", func(t *testing.T) {
		list := &List{
			items: []*session.Instance{
				{Category: "Work", Title: "Session 1"},
				{Category: "Work", Title: "Session 2"},
				{Category: "Personal", Title: "Session 3"},
			},
			groupingStrategy: GroupByCategory,
			categoriesNeedUpdate: true,
			categoryGroups: make(map[string][]*session.Instance),
			groupExpanded: make(map[string]bool),
		}

		list.OrganizeByStrategy()

		assert.Equal(t, 2, len(list.categoryGroups))
		assert.Equal(t, 2, len(list.categoryGroups["Work"]))
		assert.Equal(t, 1, len(list.categoryGroups["Personal"]))
	})

	// Test multi-membership grouping
	t.Run("Tag strategy multi membership", func(t *testing.T) {
		list := &List{
			items: []*session.Instance{
				{Tags: []string{"Urgent", "Frontend"}, Title: "Session 1"},
				{Tags: []string{"Frontend", "Client"}, Title: "Session 2"},
			},
			groupingStrategy: GroupByTag,
			categoriesNeedUpdate: true,
			categoryGroups: make(map[string][]*session.Instance),
			groupExpanded: make(map[string]bool),
		}

		list.OrganizeByStrategy()

		// Session 1 appears in both "Urgent" and "Frontend"
		assert.Contains(t, list.categoryGroups["Urgent"], list.items[0])
		assert.Contains(t, list.categoryGroups["Frontend"], list.items[0])

		// Session 2 appears in both "Frontend" and "Client"
		assert.Contains(t, list.categoryGroups["Frontend"], list.items[1])
		assert.Contains(t, list.categoryGroups["Client"], list.items[1])

		// Frontend group should have both sessions
		assert.Equal(t, 2, len(list.categoryGroups["Frontend"]))
	})

	// Test filter integration
	t.Run("Respects hidePaused filter", func(t *testing.T) {
		list := &List{
			items: []*session.Instance{
				{Status: session.Running, Category: "Work"},
				{Status: session.Paused, Category: "Work"},
			},
			groupingStrategy: GroupByCategory,
			hidePaused: true,
			categoriesNeedUpdate: true,
			categoryGroups: make(map[string][]*session.Instance),
			groupExpanded: make(map[string]bool),
		}

		list.OrganizeByStrategy()

		// Only running session should be grouped
		assert.Equal(t, 1, len(list.categoryGroups["Work"]))
	})
}
```

**Dependencies**: Task 2.2 (getGroupKeys must exist)

**Status**: ⏳ Pending

---

### Task 2.4: Add GroupingStrategy Field to List Struct (1h) - Micro

**Scope**: Add groupingStrategy field to List struct with default initialization.

**Files**:
- `ui/list.go` (modify) - Add groupingStrategy field and getter/setter

**Context**:
- List struct needs to track active strategy
- Default should be GroupByCategory for backward compatibility
- NewList() constructor needs to initialize field
- Changing strategy should trigger reorganization

**Implementation**:
```go
// ui/list.go - Add to List struct (around line 50)
type List struct {
	// ... existing fields ...

	// groupingStrategy determines how sessions are organized
	groupingStrategy GroupingStrategy

	// ... remaining fields ...
}

// NewList constructor - add initialization
func NewList(items []*session.Instance, width, height int, autoyes bool) *List {
	list := &List{
		items:  items,
		width:  width,
		height: height,
		autoyes: autoyes,

		// Initialize grouping with default strategy
		groupingStrategy: GroupByCategory,
		categoryGroups: make(map[string][]*session.Instance),
		groupExpanded: make(map[string]bool),
		categoriesNeedUpdate: true,

		// ... existing initializations ...
	}

	return list
}

// GetGroupingStrategy returns the current grouping strategy
func (l *List) GetGroupingStrategy() GroupingStrategy {
	return l.groupingStrategy
}

// SetGroupingStrategy changes the grouping strategy and triggers reorganization
func (l *List) SetGroupingStrategy(strategy GroupingStrategy) {
	if l.groupingStrategy != strategy {
		l.groupingStrategy = strategy
		l.categoriesNeedUpdate = true
		l.invalidateVisibleCache()
	}
}

// CycleGroupingStrategy advances to the next grouping strategy
func (l *List) CycleGroupingStrategy() {
	strategies := GetGroupingStrategies()
	currentIndex := -1

	// Find current strategy index
	for i, s := range strategies {
		if s == l.groupingStrategy {
			currentIndex = i
			break
		}
	}

	// Cycle to next strategy (wrap around)
	nextIndex := (currentIndex + 1) % len(strategies)
	l.SetGroupingStrategy(strategies[nextIndex])
}
```

**Success Criteria**:
- groupingStrategy field added to List struct
- NewList() initializes with GroupByCategory default
- SetGroupingStrategy() triggers reorganization
- CycleGroupingStrategy() rotates through all strategies
- Strategy changes invalidate caches appropriately

**Testing**:
```go
func TestListGroupingStrategy(t *testing.T) {
	list := NewList([]*session.Instance{}, 100, 50, false)

	// Test default strategy
	assert.Equal(t, GroupByCategory, list.GetGroupingStrategy())

	// Test SetGroupingStrategy
	list.SetGroupingStrategy(GroupByBranch)
	assert.Equal(t, GroupByBranch, list.GetGroupingStrategy())
	assert.True(t, list.categoriesNeedUpdate)

	// Test CycleGroupingStrategy
	initialStrategy := list.GetGroupingStrategy()
	list.CycleGroupingStrategy()
	assert.NotEqual(t, initialStrategy, list.GetGroupingStrategy())

	// Test cycling through all strategies returns to start
	strategies := GetGroupingStrategies()
	for i := 0; i < len(strategies); i++ {
		list.CycleGroupingStrategy()
	}
	// Should wrap back to GroupByBranch (where we started)
	assert.Equal(t, GroupByBranch, list.GetGroupingStrategy())
}
```

**Dependencies**: Task 2.1 (GroupingStrategy must exist)

**Status**: ⏳ Pending

---

### Task 2.5: Update All OrganizeByCategory Calls to OrganizeByStrategy (1h) - Micro

**Scope**: Replace all OrganizeByCategory() calls with OrganizeByStrategy() throughout codebase.

**Files**:
- `ui/list.go` (modify) - Replace 4 method calls

**Context**:
- OrganizeByCategory() called at lines 548, 903, 1756 (from grep results)
- Need to maintain exact same behavior initially
- This is a pure refactoring (no behavior change)
- Easy to verify with existing tests

**Implementation**:
```bash
# Find and replace all occurrences
grep -n "OrganizeByCategory" ui/list.go
# Line 548, 903, 1756 - replace with OrganizeByStrategy

# Before:
l.OrganizeByCategory()

# After:
l.OrganizeByStrategy()
```

**Success Criteria**:
- All OrganizeByCategory() calls replaced
- No compilation errors
- Existing tests pass without modification
- UI behavior unchanged (still defaults to GroupByCategory)

**Testing**:
- Run existing test suite: `go test ./ui -run TestList`
- Manual UI verification: Build and navigate through sessions
- Verify category organization still works correctly

**Dependencies**: Task 2.3 (OrganizeByStrategy must exist)

**Status**: ⏳ Pending

---

## Story 3: UI Integration - Hotkey and Visual Feedback (Week 2: 2 days)

**Objective**: Add UI controls for cycling grouping modes with visual feedback showing active strategy.

**Value**: Makes the new grouping capabilities discoverable and usable through intuitive hotkey interface.

**Dependencies**: Story 2 (Grouping engine must exist)

### Task 3.1: Add 'G' Key Binding for Grouping Mode Cycling (2h) - Small

**Scope**: Wire up 'G' key in app event handler to cycle grouping strategies.

**Files**:
- `app/app.go` (modify) - Add key handler in Update() method
- `app/help.go` (modify) - Add help documentation for 'G' key

**Context**:
- Key handling in Update() method around line 400-600
- Uses switch statement for key handling
- Need to call list.CycleGroupingStrategy()
- Should be available in stateDefault mode
- Follow existing pattern for other keys (f, s, etc.)

**Implementation**:
```go
// app/app.go - Add to Update() method key switch

case "G": // Cycle grouping strategy
	if m.state == stateDefault {
		m.list.CycleGroupingStrategy()

		// Log the change for debugging
		strategy := m.list.GetGroupingStrategy()
		log.InfoLog.Printf("Grouping strategy changed to: %s", strategy.String())

		// Visual feedback: show brief status message
		m.statusMessage = fmt.Sprintf("Grouping by: %s", strategy.String())
		m.statusMessageTimeout = time.Now().Add(2 * time.Second)

		return m, nil
	}
```

```go
// app/help.go - Add to help text
const helpText = `
...existing help...

View Controls:
  f         Toggle paused sessions filter
  s         Search sessions
  G         Cycle grouping mode (Category/Branch/Tag/etc.)

...remaining help...
`
```

**Success Criteria**:
- 'G' key cycles through grouping strategies
- Visual feedback shows current strategy
- Help text documents 'G' key functionality
- Key only active in stateDefault mode
- No conflicts with existing key bindings

**Testing**:
- Manual: Press 'G' repeatedly and verify groups reorganize
- Manual: Verify status message shows current strategy
- Unit test: Verify key handler calls CycleGroupingStrategy()

**Dependencies**: Task 2.4 (CycleGroupingStrategy must exist)

**Status**: ⏳ Pending

---

### Task 3.2: Add Grouping Mode Indicator to Title Bar (2h) - Small

**Scope**: Display current grouping strategy in list title bar for visibility.

**Files**:
- `ui/list.go` (modify) - Update String() method title generation

**Context**:
- List title generation in String() method around line 520
- Currently shows filter status (Active Only, Search)
- Should show grouping mode when not default
- Format: "Instances (Category) (Active Only)"
- Only show when strategy != GroupByCategory

**Implementation**:
```go
// ui/list.go - Update title generation in String() method

func (l *List) String() string {
	// Build dynamic title with filter status
	titleText := " Instances"
	var filters []string

	// Add grouping mode indicator (if not default)
	if l.groupingStrategy != GroupByCategory {
		groupingText := fmt.Sprintf("📊 %s", l.groupingStrategy.String())
		filters = append(filters, groupingText)
	}

	// Add search filter info with progress indicator
	if l.searchMode && l.searchQuery != "" {
		searchText := fmt.Sprintf("🔍 %s", l.searchQuery)
		if l.searchLoading && l.searchStage != "" {
			searchText += fmt.Sprintf(" (%s...)", l.searchStage)
		}
		filters = append(filters, searchText)
	}

	// Add paused filter info
	if l.hidePaused {
		filters = append(filters, "Active Only")
	}

	// Construct title with filters
	if len(filters) > 0 {
		titleText += fmt.Sprintf(" (%s)", strings.Join(filters, " | "))
	}
	titleText += " "

	// ... rest of method unchanged ...
}
```

**Success Criteria**:
- Title shows grouping mode when not default
- Format matches existing filter indicators
- Icon (📊) provides visual distinction
- Default mode (Category) doesn't show indicator
- Multiple indicators separated by " | "

**Testing**:
```go
func TestListTitleWithGrouping(t *testing.T) {
	list := NewList([]*session.Instance{}, 100, 50, false)

	// Default: no indicator
	title := list.String()
	assert.NotContains(t, title, "📊")

	// Change strategy: should show indicator
	list.SetGroupingStrategy(GroupByTag)
	title = list.String()
	assert.Contains(t, title, "📊 Tag")

	// Multiple indicators
	list.hidePaused = true
	title = list.String()
	assert.Contains(t, title, "📊 Tag")
	assert.Contains(t, title, "Active Only")
}
```

**Dependencies**: Task 2.4 (groupingStrategy field must exist)

**Status**: ⏳ Pending

---

### Task 3.3: Add Bottom Menu Entry for 'G' Key (1h) - Micro

**Scope**: Add 'G' key to bottom menu bar showing available commands.

**Files**:
- `ui/menu.go` (modify) - Add grouping command to menu options

**Context**:
- Bottom menu shows available commands (n, D, g, q, etc.)
- Menu is context-aware (changes based on state)
- Should show 'G' in default state
- Format: "G:Group" or "G:Grouping"

**Implementation**:
```go
// ui/menu.go - Add to menu options for default state

// GetDefaultStateCommands returns commands available in default state
func GetDefaultStateCommands() []MenuCommand {
	return []MenuCommand{
		{Key: "n", Description: "New Session"},
		{Key: "D", Description: "Delete"},
		{Key: "f", Description: "Filter"},
		{Key: "s", Description: "Search"},
		{Key: "G", Description: "Group"}, // Add this line
		{Key: "g", Description: "Git"},
		{Key: "q", Description: "Quit"},
		// ... existing commands ...
	}
}
```

**Success Criteria**:
- 'G' key appears in bottom menu
- Description is concise ("Group")
- Menu updates when key becomes available
- No conflicts with existing menu items

**Testing**:
- Manual: Verify 'G' appears in bottom menu
- Manual: Press 'G' and verify it cycles grouping
- Verify menu text matches key binding documentation

**Dependencies**: Task 3.1 (G key handler must exist)

**Status**: ⏳ Pending

---

## Story 4: Tag Editor UI - Tag Management Interface (Week 2-3: 2 days)

**Objective**: Create interactive tag editor overlay for adding/removing tags on sessions.

**Value**: Enables users to apply multi-dimensional organization by managing session tags through a friendly UI.

**Dependencies**: Story 1 (Tags data model), Story 3 (UI framework)

### Task 4.1: Create Tag Editor Overlay Component (3h) - Medium

**Scope**: Build modal overlay for tag editing following existing overlay patterns.

**Files**:
- `ui/overlay/tag_editor.go` (create) - Tag editor overlay component
- `ui/overlay/tag_editor_test.go` (create) - Unit tests

**Context**:
- Follow existing overlay pattern (see `ui/overlay/textInput.go`)
- Overlay system uses BubbleTea Model/Update/View
- Should show current tags with checkboxes
- Allow adding new tags via text input
- Allow removing tags by unchecking/deleting
- Submit with Enter, cancel with Esc

**Implementation**:
```go
// ui/overlay/tag_editor.go
package overlay

import (
	"stapler-squad/session"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// TagEditor is an overlay for managing session tags
type TagEditor struct {
	instance       *session.Instance
	tags           []string // Working copy of tags
	selectedIndex  int      // Currently selected tag
	inputMode      bool     // True when adding new tag
	inputBuffer    string   // Buffer for new tag input
	width          int
	height         int
}

// NewTagEditor creates a new tag editor overlay
func NewTagEditor(instance *session.Instance, width, height int) *TagEditor {
	return &TagEditor{
		instance:      instance,
		tags:          instance.GetTags(),
		selectedIndex: 0,
		inputMode:     false,
		inputBuffer:   "",
		width:         width,
		height:        height,
	}
}

// Init initializes the tag editor
func (t *TagEditor) Init() tea.Cmd {
	return nil
}

// Update handles key input for tag management
func (t *TagEditor) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if t.inputMode {
			return t.handleInputMode(msg)
		}
		return t.handleNavigationMode(msg)
	}
	return t, nil
}

// handleInputMode processes keys when adding new tag
func (t *TagEditor) handleInputMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		// Add new tag if not empty
		if strings.TrimSpace(t.inputBuffer) != "" {
			newTag := strings.TrimSpace(t.inputBuffer)
			// Check if tag already exists
			exists := false
			for _, tag := range t.tags {
				if tag == newTag {
					exists = true
					break
				}
			}
			if !exists {
				t.tags = append(t.tags, newTag)
			}
		}
		t.inputMode = false
		t.inputBuffer = ""

	case "esc":
		// Cancel input
		t.inputMode = false
		t.inputBuffer = ""

	case "backspace":
		if len(t.inputBuffer) > 0 {
			t.inputBuffer = t.inputBuffer[:len(t.inputBuffer)-1]
		}

	default:
		// Append character to buffer
		t.inputBuffer += msg.String()
	}

	return t, nil
}

// handleNavigationMode processes keys when navigating tags
func (t *TagEditor) handleNavigationMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if t.selectedIndex > 0 {
			t.selectedIndex--
		}

	case "down", "j":
		if t.selectedIndex < len(t.tags)-1 {
			t.selectedIndex++
		}

	case "a", "n":
		// Enter input mode to add new tag
		t.inputMode = true
		t.inputBuffer = ""

	case "d", "x", "delete":
		// Remove selected tag
		if len(t.tags) > 0 && t.selectedIndex < len(t.tags) {
			t.tags = append(t.tags[:t.selectedIndex], t.tags[t.selectedIndex+1:]...)
			if t.selectedIndex >= len(t.tags) && t.selectedIndex > 0 {
				t.selectedIndex--
			}
		}

	case "enter":
		// Save tags and close overlay
		t.instance.SetTags(t.tags)
		return t, tea.Quit

	case "esc":
		// Cancel without saving
		return t, tea.Quit
	}

	return t, nil
}

// View renders the tag editor overlay
func (t *TagEditor) View() string {
	var b strings.Builder

	// Title
	title := "Edit Tags"
	b.WriteString(lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#ffffff")).
		Render(title))
	b.WriteString("\n\n")

	// Current tags list
	if len(t.tags) == 0 {
		b.WriteString(lipgloss.NewStyle().
			Foreground(lipgloss.Color("8")).
			Render("No tags yet. Press 'a' to add."))
	} else {
		for i, tag := range t.tags {
			cursor := "  "
			style := lipgloss.NewStyle()

			if i == t.selectedIndex {
				cursor = "> "
				style = style.Background(lipgloss.Color("#007acc"))
			}

			line := fmt.Sprintf("%s %s", cursor, tag)
			b.WriteString(style.Render(line))
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")

	// Input field for adding new tag
	if t.inputMode {
		b.WriteString(lipgloss.NewStyle().
			Foreground(lipgloss.Color("#00ff00")).
			Render("New tag: "))
		b.WriteString(t.inputBuffer)
		b.WriteString("_\n")
		b.WriteString(lipgloss.NewStyle().
			Foreground(lipgloss.Color("8")).
			Render("(Enter to add, Esc to cancel)\n"))
	} else {
		// Help text
		b.WriteString(lipgloss.NewStyle().
			Foreground(lipgloss.Color("8")).
			Render("a:Add  d:Delete  Enter:Save  Esc:Cancel\n"))
	}

	// Wrap in border
	content := b.String()
	boxWidth := min(60, t.width-4)
	boxHeight := min(20, t.height-4)

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#007acc")).
		Padding(1, 2).
		Width(boxWidth).
		Height(boxHeight).
		Render(content)
}

// HandleKeyPress is required by overlay interface
func (t *TagEditor) HandleKeyPress(key string) bool {
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
	_, _ = t.Update(msg)
	return true // Handled
}
```

**Success Criteria**:
- Tag editor displays current tags in list
- Arrow keys navigate tag list
- 'd' key deletes selected tag
- 'a' key enters input mode for new tag
- Enter saves changes, Esc cancels
- Overlay follows existing pattern (modal, bordered)
- No duplicate tags allowed

**Testing**:
```go
func TestTagEditor(t *testing.T) {
	instance := &session.Instance{
		Tags: []string{"Existing", "Tag"},
	}

	editor := NewTagEditor(instance, 80, 24)

	// Test initial state
	assert.Equal(t, 2, len(editor.tags))
	assert.False(t, editor.inputMode)

	// Test adding new tag
	editor.inputMode = true
	editor.inputBuffer = "NewTag"
	editor.Update(tea.KeyMsg{Type: tea.KeyEnter})
	assert.Equal(t, 3, len(editor.tags))
	assert.Contains(t, editor.tags, "NewTag")

	// Test deleting tag
	editor.selectedIndex = 0
	editor.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	assert.Equal(t, 2, len(editor.tags))

	// Test duplicate prevention
	editor.inputMode = true
	editor.inputBuffer = editor.tags[0] // Duplicate
	editor.Update(tea.KeyMsg{Type: tea.KeyEnter})
	assert.Equal(t, 2, len(editor.tags)) // No duplicate added
}
```

**Dependencies**: Story 1 (Tags data model)

**Status**: ⏳ Pending

---

### Task 4.2: Integrate Tag Editor into App State Machine (2h) - Small

**Scope**: Wire tag editor overlay into app event handling and state management.

**Files**:
- `app/app.go` (modify) - Add stateTagEditor state and handlers

**Context**:
- App uses state machine pattern (stateDefault, stateNew, statePrompt, etc.)
- Need new state: stateTagEditor
- Activate with 't' key on selected session
- Overlay system already exists in app
- Follow pattern from other overlays (textInput, confirmModal)

**Implementation**:
```go
// app/app.go - Add to state constants
const (
	stateDefault = iota
	stateNew
	statePrompt
	stateTagEditor // Add this
	// ... existing states ...
)

// Add to Model struct
type Model struct {
	// ... existing fields ...

	tagEditor *overlay.TagEditor

	// ... remaining fields ...
}

// Add key handler in Update() method
case "t": // Edit tags
	if m.state == stateDefault {
		selectedInstance := m.list.Selected()
		if selectedInstance != nil {
			m.tagEditor = overlay.NewTagEditor(
				selectedInstance,
				m.width,
				m.height,
			)
			m.state = stateTagEditor
			return m, nil
		}
	}

// Add state handler in Update() method
case stateTagEditor:
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "enter":
			// Save tags and return to default state
			m.state = stateDefault
			m.tagEditor = nil
			m.list.InvalidateCategoryCache() // Trigger reorganization
			return m, nil

		case "esc":
			// Cancel and return to default state
			m.state = stateTagEditor
			m.tagEditor = nil
			return m, nil

		default:
			// Forward to tag editor
			m.tagEditor.HandleKeyPress(key.String())
			return m, nil
		}
	}

// Add to View() method
func (m Model) View() string {
	// ... existing view logic ...

	// Overlay tag editor if active
	if m.state == stateTagEditor && m.tagEditor != nil {
		baseView := m.baseView() // Existing view
		overlayView := m.tagEditor.View()
		return overlay.PlaceOverlay(baseView, overlayView, m.width, m.height)
	}

	// ... rest of view logic ...
}
```

**Success Criteria**:
- 't' key opens tag editor on selected session
- Tag editor overlay appears over main view
- Enter saves tags and closes overlay
- Esc cancels without saving
- Session list reorganizes after tag changes
- No crashes when no session selected

**Testing**:
- Manual: Press 't' on selected session
- Manual: Add/remove tags and verify persistence
- Manual: Verify list reorganizes with GroupByTag
- Unit test: Verify state transitions

**Dependencies**: Task 4.1 (TagEditor component)

**Status**: ⏳ Pending

---

### Task 4.3: Add Tag Editor to Help and Menu (1h) - Micro

**Scope**: Document tag editor in help text and add to bottom menu.

**Files**:
- `app/help.go` (modify) - Add tag editor help
- `ui/menu.go` (modify) - Add 't' key to menu

**Context**:
- Help text documents all key bindings
- Bottom menu shows available commands
- Should appear in default state
- Format: "t:Tags" in menu

**Implementation**:
```go
// app/help.go - Add to help text
const helpText = `
...existing help...

Session Management:
  n         Create new session
  D         Delete session
  t         Edit tags
  r         Restart session

...remaining help...
`

// ui/menu.go - Add to menu options
func GetDefaultStateCommands() []MenuCommand {
	return []MenuCommand {
		{Key: "n", Description: "New"},
		{Key: "t", Description: "Tags"}, // Add this
		{Key: "D", Description: "Delete"},
		// ... existing commands ...
	}
}
```

**Success Criteria**:
- Help text documents 't' key for tag editing
- Bottom menu shows 't:Tags' option
- Documentation matches actual behavior

**Testing**:
- Manual: Press '?' and verify 't' key documented
- Manual: Verify 't' appears in bottom menu

**Dependencies**: Task 4.2 (Tag editor integration)

**Status**: ⏳ Pending

---

## Story 5: Advanced Features - Filtering and Search Integration (Week 3: 2 days)

**Objective**: Integrate tags with existing filtering and search systems for powerful discovery.

**Value**: Leverages existing UI patterns to make tag-based organization immediately useful for finding sessions.

**Dependencies**: Story 1 (Tags), Story 4 (Tag editor)

### Task 5.1: Add Tag-Based Filtering to Search (2h) - Small

**Scope**: Extend search functionality to include tag matching.

**Files**:
- `ui/search_index.go` (modify) - Add tag indexing
- `ui/list.go` (modify) - Include tags in search results

**Context**:
- Existing search system uses fuzzy matching
- Search indexes Title, Path, Branch, Program
- Need to add Tags to searchable fields
- Search should match individual tags
- Tag matches should boost relevance score

**Implementation**:
```go
// ui/search_index.go - Add to buildSearchIndex()

func (l *List) buildSearchIndex() {
	l.searchIndex = make([]searchEntry, len(l.items))

	for i, instance := range l.items {
		// Build searchable text including tags
		searchableText := strings.Join([]string{
			instance.Title,
			instance.Path,
			instance.Branch,
			instance.Program,
			strings.Join(instance.GetTags(), " "), // Add tags
		}, " ")

		l.searchIndex[i] = searchEntry{
			instance:       instance,
			searchableText: searchableText,
			tags:           instance.GetTags(), // Store for relevance boosting
		}
	}
}

// ui/list.go - Update search scoring to boost tag matches

func (l *List) calculateSearchScore(entry searchEntry, query string) float64 {
	baseScore := fuzzyMatch(entry.searchableText, query)

	// Boost score for direct tag matches
	queryLower := strings.ToLower(query)
	for _, tag := range entry.tags {
		if strings.Contains(strings.ToLower(tag), queryLower) {
			baseScore *= 1.5 // 50% boost for tag match
			break
		}
	}

	return baseScore
}
```

**Success Criteria**:
- Search matches tags in addition to other fields
- Tag matches boost relevance scores
- Search results include tag-matched sessions
- Fuzzy matching works on tags
- Performance remains acceptable (<100ms)

**Testing**:
```go
func TestSearchIncludesTags(t *testing.T) {
	list := NewList([]*session.Instance{
		{Title: "Session 1", Tags: []string{"Frontend", "Urgent"}},
		{Title: "Session 2", Tags: []string{"Backend"}},
	}, 100, 50, false)

	// Search for tag
	list.Search("Frontend")
	results := list.GetSearchResults()

	assert.Equal(t, 1, len(results))
	assert.Equal(t, "Session 1", results[0].Title)

	// Test relevance boosting
	list2 := NewList([]*session.Instance{
		{Title: "Frontend Code", Tags: []string{"Backend"}},
		{Title: "Backend Code", Tags: []string{"Frontend"}},
	}, 100, 50, false)

	list2.Search("Frontend")
	results2 := list2.GetSearchResults()

	// Session with Frontend tag should rank higher despite title mismatch
	assert.Equal(t, "Backend Code", results2[0].Title)
}
```

**Dependencies**: Story 1 (Tags field)

**Status**: ⏳ Pending

---

### Task 5.2: Add Tag Filter Command (2h) - Small

**Scope**: Implement tag-based filtering similar to paused session filter.

**Files**:
- `ui/list.go` (modify) - Add tag filter logic
- `app/app.go` (modify) - Add 'T' key for tag filter

**Context**:
- Existing filter: hidePaused (toggle with 'f')
- Need similar filter: filterByTag (toggle with 'T')
- When active, shows only sessions with specific tag
- Prompt user to select tag to filter by
- Show tag count in title when filtering

**Implementation**:
```go
// ui/list.go - Add filter fields

type List struct {
	// ... existing fields ...

	filterByTag    string // If non-empty, filter to this tag only
	availableTags  []string // All unique tags across sessions

	// ... remaining fields ...
}

// Add filter method
func (l *List) SetTagFilter(tag string) {
	l.filterByTag = tag
	l.invalidateVisibleCache()
	l.categoriesNeedUpdate = true
}

// Add tag collection
func (l *List) GetAvailableTags() []string {
	tagSet := make(map[string]bool)

	for _, instance := range l.items {
		for _, tag := range instance.GetTags() {
			tagSet[tag] = true
		}
	}

	tags := make([]string, 0, len(tagSet))
	for tag := range tagSet {
		tags = append(tags, tag)
	}

	// Sort alphabetically
	sort.Strings(tags)
	return tags
}

// Update getVisibleItems to respect tag filter
func (l *List) getVisibleItems() []*session.Instance {
	// ... existing filter logic ...

	// Apply tag filter
	if l.filterByTag != "" {
		filtered := make([]*session.Instance, 0)
		for _, item := range visible {
			if item.HasTag(l.filterByTag) {
				filtered = append(filtered, item)
			}
		}
		visible = filtered
	}

	return visible
}

// app/app.go - Add key handler
case "T": // Filter by tag
	if m.state == stateDefault {
		// Get available tags
		tags := m.list.GetAvailableTags()

		if len(tags) == 0 {
			m.statusMessage = "No tags to filter by"
			return m, nil
		}

		// Show tag selection overlay (reuse existing overlay)
		m.state = stateTagFilter
		m.tagFilterSelector = overlay.NewSelector(
			"Filter by Tag",
			tags,
			m.width,
			m.height,
		)
		return m, nil
	}
```

**Success Criteria**:
- 'T' key opens tag selection overlay
- Selecting tag filters list to that tag only
- Title shows active tag filter
- Filter works with other filters (hidePaused)
- Clearing filter restores all sessions

**Testing**:
```go
func TestTagFilter(t *testing.T) {
	list := NewList([]*session.Instance{
		{Title: "A", Tags: []string{"Work", "Urgent"}},
		{Title: "B", Tags: []string{"Personal"}},
		{Title: "C", Tags: []string{"Work"}},
	}, 100, 50, false)

	// Filter by "Work"
	list.SetTagFilter("Work")
	visible := list.getVisibleItems()

	assert.Equal(t, 2, len(visible))
	assert.Contains(t, []string{"A", "C"}, visible[0].Title)

	// Clear filter
	list.SetTagFilter("")
	visible = list.getVisibleItems()
	assert.Equal(t, 3, len(visible))
}
```

**Dependencies**: Story 1 (Tags), existing overlay system

**Status**: ⏳ Pending

---

### Task 5.3: Update Title Bar with Tag Filter Indicator (1h) - Micro

**Scope**: Show active tag filter in title bar like other filters.

**Files**:
- `ui/list.go` (modify) - Update title generation

**Context**:
- Title already shows filter indicators
- Add tag filter indicator when active
- Format: "Tag: Frontend" or "🏷️ Frontend"
- Clear visual distinction from other filters

**Implementation**:
```go
// ui/list.go - Update String() method

func (l *List) String() string {
	// Build dynamic title with filter status
	titleText := " Instances"
	var filters []string

	// ... existing filters ...

	// Add tag filter info
	if l.filterByTag != "" {
		tagFilterText := fmt.Sprintf("🏷️ %s", l.filterByTag)
		filters = append(filters, tagFilterText)
	}

	// ... rest of method ...
}
```

**Success Criteria**:
- Title shows tag filter when active
- Icon (🏷️) provides visual distinction
- Filter indicator removed when cleared
- Works with other filter indicators

**Testing**:
- Manual: Activate tag filter, verify title
- Manual: Clear filter, verify indicator removed

**Dependencies**: Task 5.2 (Tag filter)

**Status**: ⏳ Pending

---

## Story 6: Testing and Documentation (Week 3: 1 day)

**Objective**: Comprehensive testing coverage and user-facing documentation.

**Value**: Ensures feature quality and helps users discover and use new capabilities.

**Dependencies**: All previous stories

### Task 6.1: Integration Tests for Grouping Engine (2h) - Small

**Scope**: End-to-end tests for all grouping strategies.

**Files**:
- `ui/grouping_integration_test.go` (create) - Integration tests

**Context**:
- Test all GroupingStrategy values
- Verify multi-membership for tags
- Test strategy switching
- Test with filters active
- Performance benchmarks

**Implementation**:
```go
// ui/grouping_integration_test.go

func TestGroupingIntegration(t *testing.T) {
	// Create diverse test data
	instances := []*session.Instance{
		{
			Title:   "Frontend Work",
			Tags:    []string{"Work", "Frontend", "Urgent"},
			Branch:  "feature/ui",
			Program: "claude",
			Status:  session.Running,
		},
		{
			Title:   "Backend API",
			Tags:    []string{"Work", "Backend"},
			Branch:  "feature/api",
			Program: "aider",
			Status:  session.Paused,
		},
		{
			Title:   "Personal Project",
			Tags:    []string{"Personal", "Learning"},
			Branch:  "main",
			Program: "claude",
			Status:  session.Running,
		},
	}

	list := NewList(instances, 100, 50, false)

	// Test each grouping strategy
	t.Run("GroupByCategory", func(t *testing.T) {
		list.SetGroupingStrategy(GroupByCategory)
		list.OrganizeByStrategy()
		// Verify groups...
	})

	t.Run("GroupByTag Multi-membership", func(t *testing.T) {
		list.SetGroupingStrategy(GroupByTag)
		list.OrganizeByStrategy()

		// First instance should appear in 3 groups
		workGroup := list.categoryGroups["Work"]
		frontendGroup := list.categoryGroups["Frontend"]
		urgentGroup := list.categoryGroups["Urgent"]

		assert.Contains(t, workGroup, instances[0])
		assert.Contains(t, frontendGroup, instances[0])
		assert.Contains(t, urgentGroup, instances[0])
	})

	t.Run("GroupByBranch", func(t *testing.T) {
		list.SetGroupingStrategy(GroupByBranch)
		list.OrganizeByStrategy()

		assert.Equal(t, 3, len(list.categoryGroups))
		assert.Contains(t, list.categoryGroups, "feature/ui")
		assert.Contains(t, list.categoryGroups, "feature/api")
	})

	// ... tests for other strategies ...
}

func BenchmarkOrganizeByStrategy(b *testing.B) {
	// Create large dataset
	instances := make([]*session.Instance, 1000)
	for i := 0; i < 1000; i++ {
		instances[i] = &session.Instance{
			Title:  fmt.Sprintf("Session %d", i),
			Tags:   []string{"Tag1", "Tag2", "Tag3"},
			Branch: fmt.Sprintf("branch-%d", i%10),
		}
	}

	list := NewList(instances, 100, 50, false)

	b.Run("Category", func(b *testing.B) {
		list.SetGroupingStrategy(GroupByCategory)
		for i := 0; i < b.N; i++ {
			list.categoriesNeedUpdate = true
			list.OrganizeByStrategy()
		}
	})

	b.Run("Tag", func(b *testing.B) {
		list.SetGroupingStrategy(GroupByTag)
		for i := 0; i < b.N; i++ {
			list.categoriesNeedUpdate = true
			list.OrganizeByStrategy()
		}
	})
}
```

**Success Criteria**:
- All grouping strategies tested
- Multi-membership verified for tags
- Performance benchmarks included
- Edge cases covered (empty tags, nil values)
- Tests pass consistently

**Testing**:
```bash
go test ./ui -run TestGroupingIntegration
go test ./ui -bench=BenchmarkOrganizeByStrategy
```

**Dependencies**: Story 2 (Grouping engine)

**Status**: ⏳ Pending

---

### Task 6.2: Update CLAUDE.md with Tag System Documentation (2h) - Small

**Scope**: Document tag system usage, grouping modes, and key bindings.

**Files**:
- `CLAUDE.md` (modify) - Add tag system section

**Context**:
- CLAUDE.md documents project architecture and features
- Need comprehensive tag system documentation
- Include examples and best practices
- Document all key bindings

**Implementation**:
```markdown
## Tag-Based Session Organization

Stapler Squad supports flexible session organization through tags and dynamic grouping strategies.

### Grouping Modes

Press **G** to cycle through grouping strategies:

- **Category** (Default): Organize by category field, supports nested categories
- **Tag**: Multi-dimensional organization - sessions appear in multiple tag groups
- **Branch**: Group by git branch name
- **Path**: Group by repository path
- **Program**: Group by program (claude, aider, etc.)
- **Status**: Group by session status (Running, Paused, etc.)
- **Session Type**: Group by session type (directory, worktree, etc.)
- **None**: Flat list with no grouping

The current grouping mode is shown in the title bar (e.g., "📊 Tag").

### Managing Tags

1. **Add Tags**: Press **t** on a selected session to open the tag editor
2. **Navigate Tags**: Use arrow keys or j/k to navigate the tag list
3. **Add New Tag**: Press **a** to enter input mode, type tag name, press Enter
4. **Delete Tag**: Select tag and press **d** or **x**
5. **Save Changes**: Press Enter to save, Esc to cancel

### Tag-Based Filtering

Press **T** to filter sessions by tag:
- Shows tag selection overlay with all available tags
- Select a tag to show only sessions with that tag
- Clear filter to restore all sessions
- Works with other filters (paused sessions, search)

### Search Integration

Tags are automatically included in search:
- Search query matches against session tags
- Tag matches boost relevance scores
- Use search (press **s**) to find sessions by tag

### Best Practices

**Multi-dimensional Organization**:
```
Example session tags: ["Frontend", "Urgent", "Client-Work", "React"]
- Appears in all 4 tag groups when grouped by tag
- Searchable by any tag
- Can filter to any single tag
```

**Tag Naming Conventions**:
- Use PascalCase or kebab-case for consistency
- Keep tags concise (1-2 words)
- Avoid redundant tags (e.g., "Work" + "Work-Project")
- Common tag categories:
  - Priority: Urgent, Low-Priority, Backlog
  - Type: Frontend, Backend, Infrastructure, DevOps
  - Client: Client-A, Client-B, Internal
  - Technology: React, Go, Python, Docker
  - Phase: Planning, Development, Review, Maintenance

**Backward Compatibility**:
- Existing Category field preserved and functional
- Categories auto-migrate to tags on first load
- Nested categories (e.g., "Work/Frontend") split into individual tags
```

**Success Criteria**:
- Complete documentation of tag system features
- Key bindings documented
- Examples and best practices included
- Backward compatibility explained

**Testing**:
- Review with fresh eyes (comprehensible to new users)
- Verify all documented features actually exist
- Test examples work as described

**Dependencies**: All previous stories

**Status**: ⏳ Pending

---

### Task 6.3: Add Inline Code Comments for Grouping Logic (1h) - Micro

**Scope**: Add comprehensive comments to grouping engine code.

**Files**:
- `ui/grouping.go` (modify) - Add detailed comments
- `ui/list.go` (modify) - Comment grouping-related methods

**Context**:
- Complex grouping logic needs clear documentation
- Explain multi-membership behavior
- Document performance optimizations
- Clarify design decisions

**Implementation**:
```go
// ui/grouping.go - Add comments throughout

// getGroupKeys extracts group key(s) for a session based on the active strategy.
//
// Multi-membership support: Some strategies (notably GroupByTag) return multiple
// keys, causing the session to appear in multiple groups simultaneously. This
// enables powerful multi-dimensional organization where a single session can be
// categorized along multiple axes.
//
// Single-membership strategies (Category, Branch, Path, etc.) return exactly one
// key, maintaining traditional hierarchical organization.
//
// Edge case handling: All strategies provide sensible defaults for missing or
// nil values (e.g., "Untagged", "No Branch") to ensure sessions always appear
// in exactly one group (or more for multi-membership strategies).
func (l *List) getGroupKeys(instance *session.Instance, strategy GroupingStrategy) []string {
	// ... implementation with inline comments ...
}

// OrganizeByStrategy organizes sessions into groups based on the active grouping strategy.
//
// Performance optimization: This method is only executed when categoriesNeedUpdate is true,
// avoiding expensive reorganization on every render. The flag is set when:
// - Sessions are added/removed from the list
// - The grouping strategy changes
// - Filter state changes (hidePaused, filterByTag)
//
// Multi-membership: Sessions can appear in multiple groups when GroupByTag is active.
// This is implemented by iterating over all keys returned by getGroupKeys() and adding
// the session to each corresponding group.
//
// State preservation: Group expansion state (groupExpanded map) is preserved across
// reorganizations, maintaining user interaction state even when grouping changes.
func (l *List) OrganizeByStrategy() {
	// ... implementation with inline comments ...
}
```

**Success Criteria**:
- All public methods have doc comments
- Complex logic explained with inline comments
- Design decisions documented
- Performance optimizations explained

**Testing**:
- Run `go doc ui.getGroupKeys` to verify doc comments
- Code review for comment clarity

**Dependencies**: Story 2 (Grouping engine)

**Status**: ⏳ Pending

---

## Progress Tracking

### Story Completion Status

- **Story 1: Data Model Enhancement** - ⏳ Pending (0/3 tasks)
- **Story 2: Grouping Engine** - ⏳ Pending (0/5 tasks)
- **Story 3: UI Integration** - ⏳ Pending (0/3 tasks)
- **Story 4: Tag Editor UI** - ⏳ Pending (0/3 tasks)
- **Story 5: Advanced Features** - ⏳ Pending (0/3 tasks)
- **Story 6: Testing and Documentation** - ⏳ Pending (0/3 tasks)

**Total Progress**: 0/20 tasks completed (0%)

### Dependency Visualization

```
Story 1 (Data Model)
├─ Task 1.1 (2h) ──┐
├─ Task 1.2 (2h) ──┼─→ Story 2 (Grouping Engine)
└─ Task 1.3 (1h) ──┘     ├─ Task 2.1 (1h) ──┐
                         ├─ Task 2.2 (3h) ──┼─→ Story 3 (UI)
                         ├─ Task 2.3 (3h) ──┤    ├─ Task 3.1 (2h)
                         ├─ Task 2.4 (1h) ──┤    ├─ Task 3.2 (2h)
                         └─ Task 2.5 (1h) ──┘    └─ Task 3.3 (1h)
                                                        │
Story 1 + Story 3 ───────────────────→ Story 4 (Tag Editor)
                                        ├─ Task 4.1 (3h)
                                        ├─ Task 4.2 (2h)
                                        └─ Task 4.3 (1h)
                                              │
Story 1 + Story 4 ───────────────────→ Story 5 (Advanced)
                                        ├─ Task 5.1 (2h)
                                        ├─ Task 5.2 (2h)
                                        └─ Task 5.3 (1h)
                                              │
All Stories ─────────────────────────→ Story 6 (Testing)
                                        ├─ Task 6.1 (2h)
                                        ├─ Task 6.2 (2h)
                                        └─ Task 6.3 (1h)
```

### Parallel Execution Opportunities

**Week 1 (Foundation)**:
- Story 1: Tasks 1.1, 1.2, 1.3 (sequential - 5 hours total)
- Story 2: Task 2.1 can start in parallel with Story 1
- After Story 1: Tasks 2.2-2.5 can proceed (sequential - 8 hours)

**Week 2 (UI Integration)**:
- Story 3: All tasks can execute sequentially (6 hours)
- Story 4: Task 4.1 can start after Story 1 completes
- Tasks 4.2 and 4.3 depend on 4.1 (sequential - 6 hours)

**Week 3 (Advanced + Testing)**:
- Story 5: All tasks sequential (5 hours)
- Story 6: Task 6.1 can start early, 6.2-6.3 after all features complete

**Critical Path**: Story 1 → Story 2 → Story 3 → Story 4 → Story 5 → Story 6 (35 hours total)

### Time Estimates

- **Story 1**: 5 hours (2 days)
- **Story 2**: 9 hours (3 days)
- **Story 3**: 5 hours (2 days)
- **Story 4**: 6 hours (2 days)
- **Story 5**: 5 hours (2 days)
- **Story 6**: 5 hours (1 day)

**Total Epic Effort**: 35 hours (approximately 3 weeks at 2-3 hours/day)

---

## Risk Assessment

### Technical Risks

**Medium Risk: Multi-Membership Navigation Complexity**
- **Issue**: Sessions appearing in multiple groups (GroupByTag) complicates cursor tracking
- **Mitigation**: Reuse existing navigation logic, track global index not group index
- **Task Impact**: Task 2.3 may need additional testing

**Low Risk: Performance with Large Tag Sets**
- **Issue**: 1000+ sessions with 10+ tags each could slow grouping
- **Mitigation**: Performance optimization flag (categoriesNeedUpdate) already exists
- **Task Impact**: Task 6.1 benchmarks will validate performance

**Low Risk: Backward Compatibility**
- **Issue**: Existing sessions must load without errors
- **Mitigation**: Category field preserved, migration is additive only
- **Task Impact**: Task 1.2 handles migration carefully

### User Experience Risks

**Low Risk: Feature Discoverability**
- **Issue**: Users may not discover new grouping capabilities
- **Mitigation**: Visual indicator in title, help documentation, bottom menu
- **Task Impact**: Story 3 and 6 address discoverability comprehensively

**Low Risk: Tag Management Learning Curve**
- **Issue**: Tag editor may be unfamiliar to users
- **Mitigation**: Follow familiar vim-style navigation, clear help text
- **Task Impact**: Task 4.1 includes comprehensive help text

---

## Success Validation

### Acceptance Criteria

**Data Model**:
- ✅ Tags field added without breaking existing sessions
- ✅ Migration runs automatically on load
- ✅ Category field preserved and functional

**Grouping Engine**:
- ✅ All 8 grouping strategies implemented and working
- ✅ Multi-membership works for GroupByTag
- ✅ Strategy switching < 100ms latency
- ✅ No visual glitches during reorganization

**UI Integration**:
- ✅ 'G' key cycles through strategies smoothly
- ✅ Title bar shows current strategy
- ✅ Bottom menu documents 'G' key

**Tag Editor**:
- ✅ 't' key opens editor on selected session
- ✅ Tags can be added, removed, edited
- ✅ Changes persist across restarts

**Advanced Features**:
- ✅ Search includes tags with relevance boosting
- ✅ Tag filter works with other filters
- ✅ Filter indicator appears in title

**Testing**:
- ✅ >85% test coverage for grouping engine
- ✅ All grouping strategies have integration tests
- ✅ Performance benchmarks show acceptable latency

**Documentation**:
- ✅ CLAUDE.md documents all features
- ✅ Help text includes all new key bindings
- ✅ Code comments explain complex logic

### User Validation Scenarios

**Scenario 1: Organize by Project Phase**
1. User creates tags: Planning, Development, Review, Done
2. Tags sessions according to current phase
3. Groups by Tag to see all sessions in each phase
4. Uses tag filter to focus on "Development" phase only

**Scenario 2: Multi-Project Development**
1. User works on multiple client projects simultaneously
2. Tags sessions: Client-A, Client-B, Internal, Frontend, Backend
3. Groups by Tag to see all Client-A work across frontend/backend
4. Switches to GroupByProgram to see tool distribution
5. Searches for "Client-A Frontend" to find specific sessions

**Scenario 3: Priority Management**
1. User tags sessions: Urgent, High-Priority, Low-Priority, Backlog
2. Groups by Tag during daily standup to discuss urgent items
3. Filters by "Urgent" tag to focus on critical work
4. Switches to GroupByStatus to see what's actively running

---

## Notes for Implementer

### Code Style Conventions

- **Go Idioms**: Follow existing codebase patterns (error handling, struct initialization)
- **BubbleTea Patterns**: Use Model/Update/View consistently for UI components
- **Thread Safety**: Use existing stateMutex for concurrent access to Instance fields
- **Defensive Programming**: Check for nil/empty before accessing slice/map elements

### Testing Strategy

- **Unit Tests**: Test each method in isolation with table-driven tests
- **Integration Tests**: Test story-level functionality end-to-end
- **Benchmarks**: Measure performance for grouping operations (target <100ms)
- **Manual Testing**: Build and run app after each story for real-world validation

### Development Workflow

1. **Start with Story 1**: Complete all data model tasks before moving to Story 2
2. **Test incrementally**: Run tests after each task completion
3. **Commit frequently**: Small commits per task for easy rollback
4. **Validate backward compatibility**: Load existing sessions after each story
5. **Performance check**: Run benchmarks after Story 2 completion

### Key Files Reference

- **Data Model**: `session/instance.go`, `session/storage.go`
- **Grouping Logic**: `ui/grouping.go` (new), `ui/list.go`
- **UI Integration**: `app/app.go`, `app/help.go`, `ui/menu.go`
- **Overlays**: `ui/overlay/tag_editor.go` (new)
- **Testing**: `ui/grouping_test.go` (new), `ui/grouping_integration_test.go` (new)

### Common Pitfalls to Avoid

❌ **Don't**: Modify Category field behavior (breaking change)
✅ **Do**: Add Tags alongside Category for backward compatibility

❌ **Don't**: Create new grouping data structure
✅ **Do**: Reuse existing categoryGroups map for all strategies

❌ **Don't**: Break existing navigation logic
✅ **Do**: Preserve selectedIdx tracking across strategy changes

❌ **Don't**: Forget to invalidate caches when state changes
✅ **Do**: Call invalidateVisibleCache() and set categoriesNeedUpdate = true

❌ **Don't**: Assume fields are non-nil
✅ **Do**: Use defensive checks (if len(tags) > 0, if instance != nil)

---

## Appendix: Alternative Approaches Considered

### Approach 1: Separate Tag Storage File
**Decision**: Store tags in Instance struct
**Rationale**: Simpler data model, easier migration, atomic saves with session data

### Approach 2: Hard-Coded Grouping Functions
**Decision**: Generic strategy-based engine
**Rationale**: Extensible, testable, reduces code duplication

### Approach 3: Tag Hierarchy
**Decision**: Flat tag structure
**Rationale**: Simpler UI, easier to understand, sufficient for most use cases

### Approach 4: Auto-Generated Tags
**Decision**: Manual tag management only
**Rationale**: Explicit user control, no surprising behavior, clearer organization
