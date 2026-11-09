"use client";

import { useMemo, useRef, type ReactElement } from "react";
import { createAnnouncer, type Announcer, type Region } from "@/lib/terminal/mobileInput/announce";
import { visuallyHidden } from "./MobileInputChrome.css";

export interface MobileInputLiveRegions {
  announcer: Announcer;
  /** Render once per terminal mount; both regions exist before any message is written. */
  regions: ReactElement;
}

/**
 * Per-terminal polite (`role="status"`) and assertive (`role="alert"`) regions plus the one `announcer`
 * that writes to them. Writes go straight to the DOM so clear-then-set is not batched away by React.
 */
export function useMobileInputLiveRegions(): MobileInputLiveRegions {
  const polite = useRef<HTMLDivElement>(null);
  const assertive = useRef<HTMLDivElement>(null);
  return useMemo(() => {
    const announcer = createAnnouncer({
      write(region: Region, text: string) {
        const el = (region === "polite" ? polite : assertive).current;
        if (el) el.textContent = text;
      },
    });
    const regions = (
      <>
        <div ref={polite} role="status" aria-live="polite" aria-atomic="true" className={visuallyHidden} data-testid="mobile-input-live-polite" />
        <div ref={assertive} role="alert" aria-live="assertive" aria-atomic="true" className={visuallyHidden} data-testid="mobile-input-live-assertive" />
      </>
    );
    return { announcer, regions };
  }, []);
}
