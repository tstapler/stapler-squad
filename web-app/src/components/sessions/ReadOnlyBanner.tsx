"use client";

import { useEffect } from "react";
import { useAnnounce } from "@/lib/hooks/useAnnounce";
import { READ_ONLY_BANNER_PRIMARY, readOnlySecondaryText } from "./readOnlyCopy";
import * as styles from "./ReadOnlyBanner.css";

interface ReadOnlyBannerProps {
  /** True while the Reply card (Story 5.6) is shown below the banner. */
  replyCardPresent?: boolean;
}

/**
 * Banner for a hidden session's read-only view. Announced once on mount through
 * the Announcer; the element carries no live role of its own (RO-8).
 */
export function ReadOnlyBanner({ replyCardPresent = false }: ReadOnlyBannerProps) {
  const { announce } = useAnnounce();
  const secondary = readOnlySecondaryText(replyCardPresent);

  useEffect(() => {
    announce(`${READ_ONLY_BANNER_PRIMARY}. ${secondary}`, "polite", "readonly-banner");
    // On mount only: a copy change while open (Reply card appearing) is not re-announced.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  return (
    <div className={styles.banner} data-testid="readonly-banner">
      <span className={styles.icon} aria-hidden="true">
        👁
      </span>
      <div className={styles.text}>
        <span className={styles.primary}>{READ_ONLY_BANNER_PRIMARY}</span>
        <span className={styles.secondary} data-testid="readonly-banner-secondary">
          {secondary}
        </span>
      </div>
    </div>
  );
}
