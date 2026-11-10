"use client";

import type { ReactNode } from "react";
import type { ChromeBudget } from "@/lib/terminal/mobileInput/chromeBudget";
import { stack, stackItem, terminalSlot } from "./MobileInputChrome.css";

export interface MobileInputChromeStackProps {
  budget: ChromeBudget;
  terminal: ReactNode;
  /** Chrome surfaces in bottom-up order: toolbar, compose bar, chip. Each is a non-shrinking flex child. */
  chrome: ReactNode[];
}

/** Terminal above, chrome below, all in flow (never position: fixed) so the soft keyboard cannot cover them. */
export function MobileInputChromeStack({ budget, terminal, chrome }: MobileInputChromeStackProps) {
  return (
    <div className={stack} data-testid="mobile-input-chrome-stack" data-density={budget.density}>
      <div className={terminalSlot}>{terminal}</div>
      {chrome.map((node, i) => (
        <div key={i} className={stackItem}>
          {node}
        </div>
      ))}
    </div>
  );
}
