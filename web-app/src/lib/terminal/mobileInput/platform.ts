/**
 * Two predicates, two questions. `isTouchPrimary()` is the generic coarse-pointer test (link-tap menu,
 * selection handles, gestures on every touch device). `shouldAttachMobileInput()` is the narrow
 * Android-class test for attaching the input layer and choosing the renderer; nothing else may use it.
 */

function matches(query: string): boolean {
  try {
    return typeof window !== "undefined" && typeof window.matchMedia === "function" && window.matchMedia(query).matches;
  } catch {
    return false;
  }
}

/** Coarse primary pointer, on any platform. */
export function isTouchPrimary(): boolean {
  return matches("(pointer: coarse)");
}

/**
 * UA-CH `platform` exists only in secure contexts (absent on plain-HTTP LAN origins) and may be null or empty,
 * so anything but an explicit "Android" falls back to the UA string. Chrome's "Request desktop site" drops
 * Android from the UA and opts out.
 */
export function isAndroid(): boolean {
  if (typeof navigator === "undefined") return false;
  const platform = (navigator as Navigator & { userAgentData?: { platform?: string | null } }).userAgentData?.platform;
  if (platform === "Android") return true;
  return /Android/.test(navigator.userAgent ?? "");
}

/** Android, touch-primary, no hover: attach the layer. iOS, iPadOS and touch laptops with a keyboard never attach. */
export function shouldAttachMobileInput(): boolean {
  return isTouchPrimary() && !matches("(hover: hover)") && isAndroid();
}
