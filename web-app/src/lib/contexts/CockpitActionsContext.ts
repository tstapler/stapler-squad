"use client";

import { createContext, useContext } from "react";
import type { Session, CheckpointProto } from "@/gen/session/v1/types_pb";

export interface CockpitActions {
  onSessionClick: (session: Session) => void;
  onDeleteSession: (sessionId: string) => Promise<void> | void;
  onPauseSession: (sessionId: string) => void;
  onResumeSession: (session: Session) => void;
  onDirectResumeSession: (session: Session) => void;
  onCloneSession: (sessionId: string) => void;
  onNewWorkspaceSession: (sessionId: string) => void;
  onRenameSession: (sessionId: string, newTitle: string) => Promise<boolean>;
  onRestartSession: (sessionId: string) => Promise<boolean>;
  onUpdateTags: (sessionId: string, tags: string[]) => void;
  onNewSession: () => void;
  onCreateCheckpoint: (sessionId: string, label: string) => Promise<boolean>;
  onListCheckpoints: (sessionId: string) => Promise<CheckpointProto[]>;
  onForkFromCheckpoint: (sessionId: string, checkpointId: string, newTitle: string) => Promise<Session | null>;
  onRunOneShot: (sessionId: string) => Promise<void>;
  onSetRateLimitEnabled: (sessionId: string, enabled: boolean) => void;
  onClearConversationState: (sessionId: string) => Promise<boolean>;
  onListSessions: () => void;
}

const CockpitActionsContext = createContext<CockpitActions | null>(null);

export function useCockpitActions(): CockpitActions {
  const ctx = useContext(CockpitActionsContext);
  if (!ctx) throw new Error("useCockpitActions must be inside CockpitActionsProvider");
  return ctx;
}

export const CockpitActionsProvider = CockpitActionsContext.Provider;
