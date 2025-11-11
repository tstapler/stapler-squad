import { Session } from "@/gen/session/v1/types_pb";

/**
 * Grouping strategy options for organizing sessions.
 * Mirrors the TUI grouping strategies in ui/list.go
 */
export enum GroupingStrategy {
  Category = "category",
  Tag = "tag",
  Branch = "branch",
  Path = "path",
  Program = "program",
  Status = "status",
  SessionType = "session_type",
  None = "none",
}

/**
 * Display labels for grouping strategies
 */
export const GroupingStrategyLabels: Record<GroupingStrategy, string> = {
  [GroupingStrategy.Category]: "Category",
  [GroupingStrategy.Tag]: "Tags",
  [GroupingStrategy.Branch]: "Branch",
  [GroupingStrategy.Path]: "Path",
  [GroupingStrategy.Program]: "Program",
  [GroupingStrategy.Status]: "Status",
  [GroupingStrategy.SessionType]: "Session Type",
  [GroupingStrategy.None]: "None (Flat List)",
};

/**
 * Grouped session data structure
 */
export interface GroupedSessions {
  groupKey: string;
  displayName: string;
  sessions: Session[];
}

/**
 * Group sessions by the selected strategy with multi-membership support.
 * Mirrors the TUI OrganizeByStrategy() method in ui/list.go:959
 *
 * Implementation Notes:
 * - Tag strategy enables multi-membership: sessions appear in multiple groups simultaneously
 * - Uses Map<string, Session[]> for O(1) group lookups and membership checks
 * - Returns sorted array with special groups (Uncategorized, Untagged) at the end
 * - Preserves session references (no cloning) for efficient memory usage
 *
 * Performance:
 * - Time Complexity: O(n * g) where n = session count, g = average groups per session
 * - Space Complexity: O(n * g) for multi-membership scenarios (tags)
 * - Sorting: O(k log k) where k = unique group count
 *
 * @param sessions - Array of Session instances to organize
 * @param strategy - Grouping strategy to apply (Category, Tag, Branch, etc.)
 * @returns Sorted array of grouped sessions with display metadata
 */
export function groupSessions(
  sessions: Session[],
  strategy: GroupingStrategy
): GroupedSessions[] {
  // Early exit: No grouping returns single flat group
  if (strategy === GroupingStrategy.None) {
    return [
      {
        groupKey: "all",
        displayName: "All Sessions",
        sessions: [...sessions],
      },
    ];
  }

  // Map for O(1) group lookups and membership checks
  const grouped = new Map<string, Session[]>();

  // Phase 1: Extract group keys and build membership map
  sessions.forEach((session) => {
    let groupKeys: string[] = [];

    // Extract group keys based on selected strategy
    // Most strategies return single group (single-membership)
    // Tag strategy returns multiple groups (multi-membership)
    switch (strategy) {
      case GroupingStrategy.Category:
        // Single-membership: One category per session
        groupKeys = [session.category || "Uncategorized"];
        break;

      case GroupingStrategy.Tag:
        // Multi-membership: Session appears in ALL its tag groups
        // Example: session with tags ["Frontend", "React"] appears in both groups
        // This enables multi-dimensional organization without duplication
        if (session.tags && session.tags.length > 0) {
          groupKeys = session.tags;
        } else {
          groupKeys = ["Untagged"];
        }
        break;

      case GroupingStrategy.Branch:
        // Single-membership: One branch per session
        groupKeys = [session.branch || "No Branch"];
        break;

      case GroupingStrategy.Path:
        // Single-membership: One path per session
        groupKeys = [session.path || "No Path"];
        break;

      case GroupingStrategy.Program:
        // Single-membership: One program per session
        groupKeys = [session.program || "No Program"];
        break;

      case GroupingStrategy.Status:
        // Single-membership: One status per session
        groupKeys = [getStatusDisplayName(session.status)];
        break;

      case GroupingStrategy.SessionType:
        // Single-membership: One type per session
        groupKeys = [getSessionTypeDisplayName(session.sessionType)];
        break;

      default:
        // Fallback for unknown strategies
        groupKeys = ["Uncategorized"];
    }

    // Phase 2: Add session to all its groups (enables multi-membership)
    // For most strategies, this adds to single group
    // For Tag strategy, this adds to multiple groups
    groupKeys.forEach((key) => {
      if (!grouped.has(key)) {
        grouped.set(key, []);
      }
      // Session reference is shared across groups (no cloning for efficiency)
      grouped.get(key)!.push(session);
    });
  });

  // Phase 3: Convert Map to sorted array
  // Sort logic ensures consistent ordering with special groups at the end
  const sortedGroups = Array.from(grouped.keys()).sort((a, b) => {
    // Special groups (empty/missing fields) always appear at the end
    const specialGroups = ["Uncategorized", "Untagged", "No Branch", "No Path", "No Program"];
    if (specialGroups.includes(a)) return 1;
    if (specialGroups.includes(b)) return -1;
    // Regular groups sorted alphabetically
    return a.localeCompare(b);
  });

  // Phase 4: Build final result array with metadata
  return sortedGroups.map((key) => ({
    groupKey: key,
    displayName: key,
    sessions: grouped.get(key)!,
  }));
}

/**
 * Get human-readable status display name
 */
function getStatusDisplayName(status: number): string {
  switch (status) {
    case 1:
      return "Running";
    case 2:
      return "Ready";
    case 3:
      return "Loading";
    case 4:
      return "Paused";
    case 5:
      return "Needs Approval";
    default:
      return "Unknown";
  }
}

/**
 * Get human-readable session type display name
 */
function getSessionTypeDisplayName(sessionType: number): string {
  switch (sessionType) {
    case 1:
      return "Directory";
    case 2:
      return "New Worktree";
    case 3:
      return "Existing Worktree";
    default:
      return "Unknown";
  }
}

/**
 * Get the next grouping strategy in the cycle.
 * Mirrors the TUI CycleGroupingStrategy() method in ui/list.go
 */
export function cycleGroupingStrategy(current: GroupingStrategy): GroupingStrategy {
  const strategies = Object.values(GroupingStrategy);
  const currentIndex = strategies.indexOf(current);
  const nextIndex = (currentIndex + 1) % strategies.length;
  return strategies[nextIndex];
}
