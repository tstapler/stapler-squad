import { useCallback, useEffect } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { getLastFocusedWindowId, setLastFocusedWindowId } from "./windowPersistence";
import type { NamedWindow, WindowId } from "./windowTypes";

const WINDOW_PARAM = "window";

/**
 * Resolution order per ADR-001: URL `?window=` param (if it names an
 * existing window) → `lastFocusedWindowId` hint (if it names an existing
 * window) → the first window in array order.
 */
function resolveCurrentWindowId(
  paramWindowId: string | null,
  windows: NamedWindow[]
): WindowId {
  if (paramWindowId !== null && windows.some((w) => w.id === paramWindowId)) {
    return paramWindowId;
  }
  const lastFocused = getLastFocusedWindowId();
  if (lastFocused !== null && windows.some((w) => w.id === lastFocused)) {
    return lastFocused;
  }
  return windows[0]?.id ?? "";
}

/**
 * Resolves this tab's `currentWindowId` from the URL (ADR-001: the active
 * window is per-tab and derived from `?window=`, never a shared reducer
 * field), self-healing the URL when the param is missing or names a window
 * closed by another tab.
 *
 * `isRestored` must reflect `useWindowManager`'s own restore-completion flag,
 * not just "some windows array exists" — before restore, `windows` is a
 * throwaway placeholder (`initialWindowsState()`, a single window with a
 * freshly generated random id) that a real `?window=<id>` param will
 * legitimately never match. Without this guard, the self-heal effect below
 * would "correct" that placeholder mismatch by overwriting a perfectly valid
 * incoming URL param before the real windows list has even loaded — and once
 * it later loads, this tab is left resolved onto whatever `lastFocusedWindowId`
 * happens to be (e.g. another tab's most recent selection), not the window the
 * URL originally named.
 */
export function useWindowUrlSync(
  windows: NamedWindow[],
  isRestored: boolean
): {
  currentWindowId: WindowId;
  switchToWindow: (id: WindowId) => void;
} {
  const searchParams = useSearchParams();
  const router = useRouter();

  const paramWindowId = searchParams.get(WINDOW_PARAM);
  const currentWindowId = resolveCurrentWindowId(paramWindowId, windows);

  useEffect(() => {
    if (!isRestored) return;
    if (paramWindowId === currentWindowId) return;
    const params = new URLSearchParams(searchParams.toString());
    if (currentWindowId) {
      params.set(WINDOW_PARAM, currentWindowId);
    } else {
      params.delete(WINDOW_PARAM);
    }
    const query = params.toString();
    router.replace(query ? `/?${query}` : "/", { scroll: false });
    // eslint-disable-next-line react-hooks/exhaustive-deps -- searchParams identity changes every render; comparing paramWindowId/currentWindowId values is sufficient to avoid redundant replace calls
  }, [paramWindowId, currentWindowId, isRestored]);

  const switchToWindow = useCallback(
    (id: WindowId) => {
      setLastFocusedWindowId(id);
      const params = new URLSearchParams(searchParams.toString());
      params.set(WINDOW_PARAM, id);
      router.replace(`/?${params.toString()}`, { scroll: false });
    },
    [router, searchParams]
  );

  return { currentWindowId, switchToWindow };
}
