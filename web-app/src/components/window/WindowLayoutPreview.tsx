"use client";

import type { PaneNode } from "@/lib/pane/paneTypes";
import type { NamedWindow } from "@/lib/window/windowTypes";
import * as styles from "@/styles/window/windowOverview.css";

export type SessionTitleLookup = (sessionId: string) => string | undefined;

interface PaneNodePreviewProps {
  node: PaneNode;
  focusedPaneId: string;
  getSessionTitle: SessionTitleLookup;
}

function leafLabel(
  node: Extract<PaneNode, { type: "leaf" }>,
  getSessionTitle: SessionTitleLookup,
): { text: string; empty: boolean } {
  if (node.viewKind === "session-list") return { text: "Session list", empty: false };
  if (!node.sessionId) return { text: "Empty", empty: true };
  return { text: getSessionTitle(node.sessionId) ?? node.sessionId.slice(0, 8), empty: false };
}

function PaneNodePreview({ node, focusedPaneId, getSessionTitle }: PaneNodePreviewProps) {
  if (node.type === "leaf") {
    const { text, empty } = leafLabel(node, getSessionTitle);
    const className = [
      styles.previewLeaf,
      node.id === focusedPaneId ? styles.previewLeafFocused : "",
      empty ? styles.previewLeafEmpty : "",
    ]
      .filter(Boolean)
      .join(" ");
    return (
      <div className={className} title={text}>
        <span className={styles.previewLeafLabel}>{text}</span>
      </div>
    );
  }
  // "vertical" splits sit left | right, "horizontal" top | bottom (see paneTypes.ts).
  const ratio = Math.min(Math.max(node.ratio, 0.05), 0.95);
  return (
    <div
      className={styles.previewFrame}
      style={{ flexDirection: node.direction === "vertical" ? "row" : "column" }}
    >
      <div style={{ flex: ratio, display: "flex", minWidth: 0, minHeight: 0 }}>
        <PaneNodePreview node={node.first} focusedPaneId={focusedPaneId} getSessionTitle={getSessionTitle} />
      </div>
      <div style={{ flex: 1 - ratio, display: "flex", minWidth: 0, minHeight: 0 }}>
        <PaneNodePreview node={node.second} focusedPaneId={focusedPaneId} getSessionTitle={getSessionTitle} />
      </div>
    </div>
  );
}

interface WindowPreviewCardProps {
  window: NamedWindow;
  position: number;
  isActive: boolean;
  getSessionTitle: SessionTitleLookup;
  onSelect: (id: string) => void;
  className?: string;
}

/** A clickable thumbnail of one window: its name plus a to-scale sketch of its pane layout. */
export function WindowPreviewCard({
  window: w,
  position,
  isActive,
  getSessionTitle,
  onSelect,
  className,
}: WindowPreviewCardProps) {
  const cardClass = [styles.card, isActive ? styles.cardActive : "", className ?? ""]
    .filter(Boolean)
    .join(" ");
  return (
    <button
      type="button"
      className={cardClass}
      data-testid={`window-preview-${w.id}`}
      aria-current={isActive ? "true" : undefined}
      onClick={() => onSelect(w.id)}
    >
      <span className={styles.cardTitle}>
        {position}. {w.name}
      </span>
      <div className={styles.cardPreview}>
        <PaneNodePreview
          node={w.paneState.root}
          focusedPaneId={w.paneState.focusedPaneId}
          getSessionTitle={getSessionTitle}
        />
      </div>
    </button>
  );
}
