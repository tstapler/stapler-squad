# Tag-Based Session Organization

Sessions support multi-dimensional organization through tags and 8 grouping strategies.

## Grouping Modes (Web UI "Group by" dropdown)

| Mode | Groups by |
|---|---|
| **Category** (default) | Category field; supports nested `Work/Frontend` → separate tags |
| **Tag** | Multi-membership — session appears in all its tag groups |
| **Branch** | Git branch name |
| **Path** | Repository path (abbreviated) |
| **Program** | claude, aider, etc. |
| **Status** | Running, Paused, Ready, etc. |
| **Session Type** | directory, worktree, etc. |
| **None** | Flat list |

## Tag Management (Web UI)

- Tags appear as blue pills on session cards
- "Add Tags" / "Edit Tags" button opens Tag Editor Modal
- Enter to add, × to remove; duplicates and empty tags prevented
- Tag Filter Dropdown: filter to sessions with a specific tag
- Combine with status and category filters

## Tag Naming Conventions

- Use **PascalCase** or **kebab-case**; keep tags 1-2 words
- Common categories: `Urgent`/`Low-Priority` (priority), `Frontend`/`Backend` (type), `Client-A`/`Internal` (client), `React`/`Go` (tech), `Planning`/`Development`/`Review` (phase)

## Backward Compatibility

- Existing `Category` field preserved; auto-migrates to tags on first load
- Nested categories (e.g. `Work/Frontend`) split into individual tags `["Work", "Frontend"]`
- `GroupByCategory` remains the default grouping strategy
- Migration is idempotent; no data loss

## Technical Details

- Tags stored in `tagIndex` map for O(1) lookup; prefix matching for partial queries
- `Tags []string` field in `session.Instance` struct with thread-safe methods (`AddTag`, `RemoveTag`, `HasTag`, `SetTags`)
- Tags serialized in JSON persistence and Protobuf schema
- Strategy pattern for grouping engine; expansion state preserved across strategy changes

## Example Use Cases

**By Project Phase:** tag `Planning`/`Development`/`Review` → Group By Tag → filter to `Development`

**Multi-Project:** tag `Client-A`/`Frontend` → Group By Tag for client view → Group By Program for tool view

**Priority:** tag `Urgent`/`Backlog` → Group By Tag → Tag Filter → `Urgent` for daily standups
