import { useState } from "react";
import { MOBILE_INPUT_LAYER_FLAG, preloadFlagValue } from "@/lib/features/flagStore";
import { shouldAttachMobileInput } from "./mobileInput/platform";

/** Layer flag AND the Android-class gate. Fixed for the page's lifetime like the renderer (flips apply on next load). */
export function useMobileInputLayerActive(): boolean {
  const [active] = useState(() => preloadFlagValue(MOBILE_INPUT_LAYER_FLAG) === true && shouldAttachMobileInput());
  return active;
}
