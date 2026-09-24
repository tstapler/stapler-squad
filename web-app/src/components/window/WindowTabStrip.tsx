"use client";

import { forwardRef, memo, useImperativeHandle, useRef, useState } from "react";
import type { NamedWindow, WindowId } from "@/lib/window/windowTypes";
import { GESTURE_MOVEMENT_THRESHOLD_PX } from "@/lib/window/windowGestureConstants";
import { useWindowSwipe } from "@/lib/window/useWindowSwipe";
import {
  windowTabStrip,
  windowTabList,
  windowTabWrapper,
  windowTabButton,
  windowTabCloseButton,
  windowTabInput,
  windowAddButton,
} from "@/styles/window/windowTabStrip.css";

/**
 * Imperative handle exposed via `ref` so a caller outside the component
 * (the leader-key `,` follow-up in useWindowShortcuts) can trigger the same
 * inline rename editor double-click uses, instead of a separate UI like a
 * native `window.prompt`.
 */
export interface WindowTabStripHandle {
  beginEdit: (id: WindowId, name: string) => void;
}

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
      data-testid={`window-tab-close-${id}`}
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
  showClose: boolean;
  onSwitch: (id: WindowId) => void;
  onClose: (id: WindowId) => void;
  onBeginEdit: (id: WindowId, name: string) => void;
  longPress: ReturnType<typeof useLongPress>;
}

// A <div role="tab"> rather than a native <button> — the close "×" control
// must live inside the tab's own DOM subtree (nesting it as a sibling makes
// it a direct, non-"tab"-role child of the parent role="tablist", which
// fails axe's aria-required-children rule) and a real <button> cannot be
// nested inside another <button>. `aria-label={w.name}` keeps this
// element's own accessible name pinned to just the window name rather than
// accumulating the nested close button's "Close <name>" text via
// name-from-content. The nested real <button> still trips axe's separate
// nested-interactive rule; this mirrors ShellTabLabel's identical
// tab-with-inline-actions structure (SessionDetailView.tsx's shell tab
// button), an established, pre-existing pattern in this codebase rather
// than a new tradeoff introduced here.
function WindowTabButton({ window: w, isActive, showClose, onSwitch, onClose, onBeginEdit, longPress }: WindowTabButtonProps) {
  const handleKeyDown = (e: React.KeyboardEvent) => {
    // Ignore keys that bubbled up from the nested close button — only the
    // tab itself (not its child) should respond to these.
    if (e.target !== e.currentTarget) return;
    if (e.key === "F2") {
      onBeginEdit(w.id, w.name);
    } else if (e.key === "Delete") {
      onClose(w.id);
    } else if (e.key === "Enter" || e.key === " ") {
      // A native <button> auto-activates on Enter/Space; this element is a
      // <div> (see doc comment above), so that activation is wired up here.
      e.preventDefault();
      onSwitch(w.id);
    }
  };

  return (
    <div
      role="tab"
      aria-selected={isActive}
      aria-label={w.name}
      tabIndex={isActive ? 0 : -1}
      data-testid={`window-tab-${w.id}`}
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
      {showClose && <WindowTabCloseButton id={w.id} name={w.name} isActive={isActive} onClose={onClose} />}
    </div>
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
        showClose={showClose}
        onSwitch={onSwitch}
        onClose={onClose}
        onBeginEdit={onBeginEdit}
        longPress={longPress}
      />
    </div>
  );
}

const WindowTabStripInner = forwardRef<WindowTabStripHandle, WindowTabStripProps>(
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

    useImperativeHandle(ref, () => ({ beginEdit }), [beginEdit]);

    // Container ref for useWindowSwipe to attach touch listeners to
    // (Task 3.1.1b) — internal only; the forwarded ref exposes `beginEdit`
    // instead of this DOM node (nothing outside this component needs the
    // node directly).
    const containerRef = useRef<HTMLDivElement>(null);

    useWindowSwipe(containerRef, {
      onSwipe: (dir) => {
        const idx = windows.findIndex((w) => w.id === currentWindowId);
        const nextIdx = dir === "next" ? (idx + 1) % windows.length : (idx - 1 + windows.length) % windows.length;
        onSwitch(windows[nextIdx].id);
      },
    });

    return (
      <div ref={containerRef} className={windowTabStrip}>
        {/* role="tablist" wraps ONLY the tabs — the "+" button is a sibling
            outside this element's DOM subtree, not a tablist child, since
            aria-required-children mandates role="tab" for every direct
            (flattened) child of role="tablist". */}
        <div className={windowTabList} role="tablist" aria-label="Window switcher">
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
        </div>
        <button
          className={windowAddButton}
          data-testid="window-add-button"
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
