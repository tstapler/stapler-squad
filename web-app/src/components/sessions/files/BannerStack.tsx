"use client";

import { useState, type ReactNode } from "react";
import { isBlocking, pickVisible, type Banner } from "@/lib/files/bannerPriority";
import { banner as bannerClass, bannerAction } from "./StaleBanner.css";
import { collapsedRow, collapsedText, moreButton, stack } from "./BannerStack.css";

export interface BannerAction { label: string; onClick: () => void }

export interface BannerSpec extends Banner {
  body: ReactNode;
  /** Blocking banners: the first two are Copy my version and Retry; they stay on the collapsed row. */
  actions?: BannerAction[];
  /** Ignored for blocking banners, which have no Dismiss. */
  onDismiss?: () => void;
  testId?: string;
}

const MAX_VISIBLE = 2;
const TARGET: React.CSSProperties = { minWidth: 44, minHeight: 44 };

/** One banner: `role="alert"` and no Dismiss when blocking, `role="status"` otherwise; it never takes focus. */
export function BannerItem({ spec }: { spec: BannerSpec }) {
  const blocking = isBlocking(spec.kind);
  return (
    <div className={bannerClass} role={blocking ? "alert" : "status"} data-testid={spec.testId ?? `banner-${spec.kind}`}>
      <span>{spec.body}</span>
      {spec.actions?.map((a) => (
        <button key={a.label} type="button" className={bannerAction} onClick={a.onClick}>{a.label}</button>
      ))}
      {!blocking && spec.onDismiss && (
        <button type="button" className={bannerAction} aria-label="Dismiss" onClick={spec.onDismiss}>×</button>
      )}
    </div>
  );
}

interface BannerStackProps {
  banners: readonly BannerSpec[];
  /** Short Edit viewport: one 44 px row for the top blocking banner (ux.md 7.1a step 2). */
  collapsed?: boolean;
}

/** At most two banners by priority; a blocking banner is always shown and "+N more" opens the rest. */
export function BannerStack({ banners, collapsed = false }: BannerStackProps) {
  const [expanded, setExpanded] = useState(false);
  const [opened, setOpened] = useState(false);
  if (banners.length === 0) return null;
  const { visible, hidden } = pickVisible(banners, MAX_VISIBLE);
  const top = visible[0];

  if (collapsed && !opened && isBlocking(top.kind)) {
    const [copy, retry] = top.actions ?? [];
    return (
      <div className={stack}>
        <div className={collapsedRow} data-testid="banner-collapsed-row" style={{ minHeight: 44 }}>
          <div role="alert" className={collapsedText}>
            <button type="button" className={bannerAction} style={{ ...TARGET, textAlign: "left" }} onClick={() => setOpened(true)}>
              {top.body}
            </button>
          </div>
          {[copy, retry].map((a) => a && (
            <button key={a.label} type="button" className={bannerAction} style={TARGET} onClick={a.onClick}>{a.label}</button>
          ))}
          {banners.length > 1 && (
            <button type="button" className={moreButton} style={TARGET} onClick={() => setOpened(true)}>{banners.length - 1} notices</button>
          )}
        </div>
      </div>
    );
  }

  const shown = expanded ? [...visible, ...hidden] : visible;
  return (
    <div className={stack}>
      {shown.map((spec) => <BannerItem key={spec.id} spec={spec} />)}
      {hidden.length > 0 && (
        <button type="button" className={moreButton} aria-expanded={expanded} onClick={() => setExpanded((e) => !e)}>
          {expanded ? "Show fewer" : `+${hidden.length} more`}
        </button>
      )}
    </div>
  );
}
