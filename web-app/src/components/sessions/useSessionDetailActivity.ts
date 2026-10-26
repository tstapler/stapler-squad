import type { PollMode } from "@/lib/hooks/pollMode";

export type { PollMode };

export interface SessionDetailActivity {
  pollMode: PollMode;
  keysEnabled: boolean;
  measureEnabled: boolean;
}

interface Inputs {
  /** Focused pane (the existing `isActive` prop). */
  isFocused: boolean;
  /** Window is the visible one. */
  windowActive: boolean;
}

/** The single place `isFocused` and `windowActive` combine. A visible unfocused pane keeps polling slowly. */
export function useSessionDetailActivity({ isFocused, windowActive }: Inputs): SessionDetailActivity {
  return {
    pollMode: windowActive ? (isFocused ? "fast" : "slow") : "off",
    keysEnabled: windowActive && isFocused,
    measureEnabled: windowActive,
  };
}
