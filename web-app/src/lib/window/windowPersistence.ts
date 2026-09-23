import { loadPaneLayout } from "@/lib/pane/usePaneLayout";
import { generateWindowId } from "./windowUtils";
import type { NamedWindow, PersistedWindowLayoutV2, WindowRevision } from "./windowTypes";

const WINDOW_LAYOUT_KEY = "cockpit.windowLayout";
const LAST_FOCUSED_WINDOW_KEY = "cockpit.lastFocusedWindowId";

/** Shape-validate a parsed v2 payload before trusting it. */
function isValidV2Layout(value: unknown): value is PersistedWindowLayoutV2 {
  if (!value || typeof value !== "object") return false;
  const candidate = value as Partial<PersistedWindowLayoutV2>;
  return (
    candidate.version === 2 &&
    typeof candidate.revision === "number" &&
    Array.isArray(candidate.windows)
  );
}

/** Plain write-through helper shared by migration and the guarded save. */
function saveWindowLayoutRaw(layout: PersistedWindowLayoutV2): void {
  try {
    localStorage.setItem(WINDOW_LAYOUT_KEY, JSON.stringify(layout));
  } catch {
    // localStorage may be unavailable (private mode, quota exceeded, SSR)
  }
}

/**
 * Wrap the existing v1 `cockpit.paneLayout` (if any) into a single "Window 1"
 * and write it through to the v2 key. Never touches/deletes `cockpit.paneLayout`
 * (ADR-003) so the migration is non-destructive even though it is irreversible.
 */
const PANE_LAYOUT_KEY = "cockpit.paneLayout";

function migrateFromV1(): PersistedWindowLayoutV2 | null {
  const legacy = loadPaneLayout();
  if (!legacy) {
    // Distinguish "nothing to migrate" (fresh user, no log needed) from
    // "a v1 blob exists but failed validation" (corrupt, fail closed + log).
    if (localStorage.getItem(PANE_LAYOUT_KEY) !== null) {
      console.error("windowPersistence: cockpit.paneLayout is present but failed validation, migration fell back to null");
    }
    return null;
  }

  const migrated: PersistedWindowLayoutV2 = {
    version: 2,
    revision: 0,
    windows: [
      {
        id: generateWindowId(),
        name: "Window 1",
        paneState: {
          root: legacy.root,
          focusedPaneId: legacy.focusedPaneId,
          zoomedPaneId: legacy.zoomedPaneId,
        },
      },
    ],
  };
  saveWindowLayoutRaw(migrated);
  return migrated;
}

/**
 * Resolution order (Migration Plan): a shape-valid v2 key wins outright;
 * anything else (missing, corrupt, unrecognized version) falls through to a
 * one-time v1 migration; if that also has nothing, the caller gets `null`.
 */
export function loadWindowLayout(): PersistedWindowLayoutV2 | null {
  let parsed: unknown;
  try {
    const stored = localStorage.getItem(WINDOW_LAYOUT_KEY);
    if (!stored) return migrateFromV1();
    parsed = JSON.parse(stored);
  } catch {
    console.error("windowPersistence: failed to parse cockpit.windowLayout, falling back to migration");
    return migrateFromV1();
  }

  const version = (parsed as { version?: unknown } | null)?.version;
  switch (version) {
    case 2:
      if (isValidV2Layout(parsed)) {
        return parsed;
      }
      console.error("windowPersistence: cockpit.windowLayout has version 2 but invalid shape, falling back to migration");
      return migrateFromV1();
    default:
      console.error(`windowPersistence: unrecognized cockpit.windowLayout version (${String(version)}), falling back to migration`);
      return migrateFromV1();
  }
}

/**
 * Optimistic Offline Lock: re-reads the current stored value fresh and only
 * writes if `lastKnownRevision` still matches, so a concurrent tab's write
 * can never be silently clobbered (ADR-002).
 */
export function saveWindowLayout(
  windows: NamedWindow[],
  lastKnownRevision: WindowRevision
): { status: "ok"; revision: WindowRevision } | { status: "conflict"; latest: PersistedWindowLayoutV2 } | { status: "error" } {
  try {
    const stored = localStorage.getItem(WINDOW_LAYOUT_KEY);
    if (stored) {
      const parsed = JSON.parse(stored);
      if (isValidV2Layout(parsed) && parsed.revision !== lastKnownRevision) {
        console.error(
          `windowPersistence: stale save rejected (lastKnownRevision=${lastKnownRevision}, stored=${parsed.revision})`
        );
        return { status: "conflict", latest: parsed };
      }
    }

    const nextRevision = lastKnownRevision + 1;
    const layout: PersistedWindowLayoutV2 = { version: 2, revision: nextRevision, windows };
    saveWindowLayoutRaw(layout);
    return { status: "ok", revision: nextRevision };
  } catch {
    return { status: "error" };
  }
}

/** Single scalar, last-write-wins by construction — bypasses the revision guard entirely. */
export function getLastFocusedWindowId(): string | null {
  try {
    return localStorage.getItem(LAST_FOCUSED_WINDOW_KEY);
  } catch {
    return null;
  }
}

export function setLastFocusedWindowId(id: string): void {
  try {
    localStorage.setItem(LAST_FOCUSED_WINDOW_KEY, id);
  } catch {
    // localStorage may be unavailable (private mode, quota exceeded, SSR)
  }
}
