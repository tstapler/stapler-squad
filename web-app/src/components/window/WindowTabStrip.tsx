"use client";

import { forwardRef, memo, useRef, useState } from "react";
import type { NamedWindow, WindowId } from "@/lib/window/windowTypes";
import { GESTURE_MOVEMENT_THRESHOLD_PX } from "@/lib/window/windowGestureConstants";
import {
  windowTabStrip,
  windowTabWrapper,
  windowTabButton,
  windowTabCloseButton,
  windowTabInput,
  windowAddButton,
} from "@/styles/window/windowTabStrip.css";

interface WindowTabStripProps {
  windows: NamedWindow[];
  currentWindowId: WindowId;
  onSwitch: (id: WindowId) => void;
  onCreate: () => void;
  onClose: (id: WindowId) => void;
  onRename: (id: WindowId, name: string) => void;
}

const LONG_PRESS_MS = 500;

// Long-press touch state tracked in a ref rather than React state — it's
// write-heavy (every touchmove) and never needs to trigger a render itself.
interface LongPressState {
  startX: number;
  startY: number;
  timer: ReturnType<typeof setTimeout>;
}

// Shared 500ms-stationary-else-cancel long-press check: `onLongPress(id, name)`
// fires once if the touch stays within GESTURE_MOVEMENT_THRESHOLD_PX for the
// full duration; moving past the threshold cancels it so the gesture falls
// through to swipe/scroll handling instead (Epic 3.2).
function useLongPress(onLongPress: (id: WindowId, name: string) => void) {
  const stateRef = useRef<LongPressState | null>(null);

  const clear = () => {
    if (stateRef.current) {
      clearTimeout(stateRef.current.timer);
      stateRef.current = null;
    }
  };

  const onTouchStart = (e: React.TouchEvent, id: WindowId, name: string) => {
    const touch = e.touches[0];
    if (!touch) return;
    const timer = setTimeout(() => {
      onLongPress(id, name);
      stateRef.current = null;
    }, LONG_PRESS_MS);
    stateRef.current = { startX: touch.clientX, startY: touch.clientY, timer };
  };

  const onTouchMove = (e: React.TouchEvent) => {
    const state = stateRef.current;
    const touch = e.touches[0];
    if (!state || !touch) return;
    const dx = Math.abs(touch.clientX - state.startX);
    const dy = Math.abs(touch.clientY - state.startY);
    if (dx > GESTURE_MOVEMENT_THRESHOLD_PX || dy > GESTURE_MOVEMENT_THRESHOLD_PX) {
      clear();
    }
  };

  return { onTouchStart, onTouchMove, onTouchEnd: clear };
}

interface WindowTabProps {
  window: NamedWindow;
  isActive: boolean;
  isEditing: boolean;
  draftName: string;
  showClose: boolean;
  onSwitch: (id: WindowId) => void;
  onClose: (id: WindowId) => void;
  onBeginEdit: (id: WindowId, name: string) => void;
  onDraftChange: (value: string) => void;
  onCommit: (id: WindowId) => void;
  onCancel: () => void;
  longPress: ReturnType<typeof useLongPress>;
}

interface WindowTabEditorProps {
  id: WindowId;
  draftName: string;
  onDraftChange: (value: string) => void;
  onCommit: (id: WindowId) => void;
  onCancel: () => void;
}

function WindowTabEditor({ id, draftName, onDraftChange, onCommit, onCancel }: WindowTabEditorProps) {
  return (
    <div className={windowTabWrapper}>
      <input
        ref={(el) => {
          if (el) {
            el.focus();
            el.select();
          }
        }}
        className={windowTabInput}
        value={draftName}
        onChange={(e) => onDraftChange(e.target.value)}
        onBlur={() => onCommit(id)}
        onKeyDown={(e) => {
          if (e.key === "Enter") {
            e.preventDefault();
            onCommit(id);
          } else if (e.key === "Escape") {
            e.preventDefault();
            onCancel();
          }
        }}
      />
    </div>
  );
}

interface WindowTabCloseButtonProps {
  id: WindowId;
  name: string;
  isActive: boolean;
  onClose: (id: WindowId) => void;
}

function WindowTabCloseButton({ id, name, isActive, onClose }: WindowTabCloseButtonProps) {
  return (
    <button
      className={windowTabCloseButton({ active: isActive })}
      aria-label={`Close ${name}`}
      title={`Close ${name}`}
      onClick={(e) => {
        e.stopPropagation();
        onClose(id);
      }}
    >
      &times;
    </button>
  );
}

interface WindowTabButtonProps {
  window: NamedWindow;
  isActive: boolean;
  onSwitch: (id: WindowId) => void;
  onClose: (id: WindowId) => void;
  onBeginEdit: (id: WindowId, name: string) => void;
  longPress: ReturnType<typeof useLongPress>;
}

function WindowTabButton({ window: w, isActive, onSwitch, onClose, onBeginEdit, longPress }: WindowTabButtonProps) {
  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "F2") {
      onBeginEdit(w.id, w.name);
    } else if (e.key === "Delete") {
      onClose(w.id);
    }
  };

  return (
    <button
      role="tab"
      aria-selected={isActive}
      tabIndex={isActive ? 0 : -1}
      className={windowTabButton({ active: isActive })}
      title={w.name}
      onClick={() => onSwitch(w.id)}
      onDoubleClick={() => onBeginEdit(w.id, w.name)}
      onKeyDown={handleKeyDown}
      onTouchStart={(e) => longPress.onTouchStart(e, w.id, w.name)}
      onTouchMove={longPress.onTouchMove}
      onTouchEnd={longPress.onTouchEnd}
    >
      {w.name}
    </button>
  );
}

function WindowTab({
  window: w,
  isActive,
  isEditing,
  draftName,
  showClose,
  onSwitch,
  onClose,
  onBeginEdit,
  onDraftChange,
  onCommit,
  onCancel,
  longPress,
}: WindowTabProps) {
  if (isEditing) {
    return (
      <WindowTabEditor
        id={w.id}
        draftName={draftName}
        onDraftChange={onDraftChange}
        onCommit={onCommit}
        onCancel={onCancel}
      />
    );
  }

  return (
    <div className={windowTabWrapper}>
      <WindowTabButton
        window={w}
        isActive={isActive}
        onSwitch={onSwitch}
        onClose={onClose}
        onBeginEdit={onBeginEdit}
        longPress={longPress}
      />
      {showClose && <WindowTabCloseButton id={w.id} name={w.name} isActive={isActive} onClose={onClose} />}
    </div>
  );
}

const WindowTabStripInner = forwardRef<HTMLDivElement, WindowTabStripProps>(
  function WindowTabStrip({ windows, currentWindowId, onSwitch, onCreate, onClose, onRename }, ref) {
    const [editingId, setEditingId] = useState<WindowId | null>(null);
    const [draftName, setDraftName] = useState("");

    const beginEdit = (id: WindowId, name: string) => {
      setEditingId(id);
      setDraftName(name);
    };

    const commitRename = (id: WindowId) => {
      const trimmed = draftName.trim();
      if (trimmed.length > 0) {
        onRename(id, trimmed);
      }
      setEditingId(null);
    };

    const cancelRename = () => setEditingId(null);
    const longPress = useLongPress(beginEdit);

    return (
      <div ref={ref} className={windowTabStrip} role="tablist" aria-label="Window switcher">
        {windows.map((w) => (
          <WindowTab
            key={w.id}
            window={w}
            isActive={w.id === currentWindowId}
            isEditing={editingId === w.id}
            draftName={draftName}
            showClose={windows.length > 1}
            onSwitch={onSwitch}
            onClose={onClose}
            onBeginEdit={beginEdit}
            onDraftChange={setDraftName}
            onCommit={commitRename}
            onCancel={cancelRename}
            longPress={longPress}
          />
        ))}
        <button
          className={windowAddButton}
          onClick={onCreate}
          title="New window"
          aria-label="New window"
        >
          +
        </button>
      </div>
    );
  }
);

WindowTabStripInner.displayName = "WindowTabStrip";

// Compares `windows` by the derived {id, name}[] list rather than by
// reference/full deep-equality — windowReducer's PANE_ACTION case rebuilds
// the entire `windows` array (including every other window's pane tree) on
// every pane action, so comparing full objects would defeat memoization on
// every keystroke/resize in the active window (pre-mortem.md P2 #4).
// Callback props (onSwitch/onCreate/onClose/onRename) are deliberately not
// compared: page.tsx passes fresh inline arrows every render, but each one
// only forwards to the hook's already-stable functions.
function arePropsEqual(prev: WindowTabStripProps, next: WindowTabStripProps): boolean {
  if (prev.currentWindowId !== next.currentWindowId) return false;
  if (prev.windows.length !== next.windows.length) return false;
  for (let i = 0; i < prev.windows.length; i++) {
    if (prev.windows[i].id !== next.windows[i].id) return false;
    if (prev.windows[i].name !== next.windows[i].name) return false;
  }
  return true;
}

export const WindowTabStrip = memo(WindowTabStripInner, arePropsEqual);
