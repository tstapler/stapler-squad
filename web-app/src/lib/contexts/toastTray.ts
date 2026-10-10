import type { MutableRefObject } from "react";
import type { NotificationData } from "@/lib/types/notification";
import type { ToastQueue, ToastQueueAction } from "@/lib/hooks/useToastQueue";
import type { ToastTimerRegistry } from "@/lib/hooks/useToastTimers";
import { readUndoWindowMs } from "@/lib/utils/deckSettings";

/**
 * "Move all to tray": a demote-only deck action. It takes toasts off the screen
 * and nothing else; it must stay free of history and RPC imports, which a test
 * enforces by reading this file. Marking read or deleting history is a different
 * command, in a different place.
 */

export const MOVE_UNDO_TIMER_ID = "move-all-to-tray";

export interface MovedBatch {
  toasts: NotificationData[];
}

interface Deps {
  queueRef: MutableRefObject<ToastQueue>;
  dispatch: (action: ToastQueueAction) => void;
  timers: ToastTimerRegistry;
  movedRef: MutableRefObject<MovedBatch | null>;
  setMoved: (batch: MovedBatch | null) => void;
  announce: (message: string) => void;
}

export function createToastTrayCommands(deps: Deps) {
  const { queueRef, dispatch, timers, movedRef, setMoved, announce } = deps;

  const finalize = () => {
    timers.cancel(MOVE_UNDO_TIMER_ID);
    setMoved(null);
  };

  return {
    /** Returns the ids that left the deck. */
    moveAllToTray(): string[] {
      const toasts = queueRef.current;
      if (toasts.length === 0) return [];
      const ids = toasts.map((t) => t.id);
      ids.forEach((id) => timers.cancel(id));
      dispatch({ type: "remove", ids: new Set(ids) });
      setMoved({ toasts: [...toasts] });
      timers.register(MOVE_UNDO_TIMER_ID, "undo-window", readUndoWindowMs(), () => setMoved(null));
      announce(`${toasts.length} moved to tray`);
      return ids;
    },

    undoMoveToTray(): void {
      const batch = movedRef.current;
      if (!batch) return;
      finalize();
      dispatch({ type: "restore", toasts: batch.toasts });
      announce(`Restored ${batch.toasts.length} notifications`);
    },

    /** The undo window ends early, e.g. because the tray opened. */
    finalizeMove: finalize,
  };
}
