// +feature: notification-inline-actions
"use client";

import { useState } from "react";
import { copyToClipboard } from "@/lib/clipboard";
import { COPY, characterCount } from "@/lib/copy/notificationCopy";
import { commandMeta, commandRegion, commandScroll, commandText, actionButton } from "./ActionRow.css";

/** Past either limit the command moves into a labelled scroll region with a count and Copy. */
export const LONG_COMMAND_LINES = 8;
export const LONG_COMMAND_CHARS = 400;

export function isLongCommand(text: string): boolean {
  return text.split("\n").length > LONG_COMMAND_LINES || text.length > LONG_COMMAND_CHARS;
}

interface ApprovalCommandProps {
  tool?: string;
  /** Complete command or argument text; never truncated. */
  text?: string;
}

/** The whole command the operator is about to approve (ux.md 2.4); wrapped, never ellipsized. */
export function ApprovalCommand({ tool, text }: ApprovalCommandProps) {
  const [copied, setCopied] = useState(false);
  if (!text) return null;
  const long = isLongCommand(text);
  const copyButton = (
    // analytics-exempt
    <button
      type="button"
      className={actionButton}
      onClick={() => {
        void copyToClipboard(text).then(setCopied);
      }}
    >
      {copied ? COPY.copied : COPY.copy}
    </button>
  );
  return (
    <div className={commandRegion} data-testid="approval-command">
      {tool && <span>{tool}</span>}
      <pre
        className={`${commandText} ${long ? commandScroll : ""}`}
        // A keyboard user must be able to scroll the region.
        tabIndex={long ? 0 : undefined}
        aria-label={long ? COPY.commandRegionLabel : undefined}
        role={long ? "region" : undefined}
        data-testid="approval-command-text"
      >
        {text}
      </pre>
      {long && (
        <div className={commandMeta}>
          <span data-testid="approval-command-count">{characterCount(text.length)}</span>
          {copyButton}
        </div>
      )}
    </div>
  );
}
