"use client";

import { createContext, useContext } from "react";
import type { Session } from "@/gen/session/v1/types_pb";
import type { PaneState, PaneAction } from "@/lib/pane/paneTypes";

export interface PaneContextValue {
  state: PaneState;
  dispatch: React.Dispatch<PaneAction>;
  sessions: Session[];
  pickerPendingSession: Session | null;
  triggerPicker: (session: Session, tab?: string) => void;
  triggerPickerForceNew: (session: Session, tab?: string) => void;
  cancelPicker: () => void;
}

export const PaneContext = createContext<PaneContextValue | null>(null);

export function usePaneContext(): PaneContextValue {
  const ctx = useContext(PaneContext);
  if (!ctx) throw new Error("usePaneContext must be used inside PaneContext.Provider");
  return ctx;
}
