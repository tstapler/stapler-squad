"use client";

import { forwardRef, memo, useCallback, useEffect, useImperativeHandle, useRef, useState } from "react";
import type { NamedWindow, WindowId } from "@/lib/window/windowTypes";
import { WindowPreviewCard, type SessionTitleLookup } from "./WindowLayoutPreview";
import * as overviewStyles from "@/styles/window/windowOverview.css";
import { GESTURE_MOVEMENT_THRESHOLD_PX } from "@/lib/window/windowGestureConstants";
import { useWindowSwipe } from "@/lib/window/useWindowSwipe";
import type { WindowSwitchTrigger } from "@/lib/window/useWindowFocusMemory";
import { windowPanelDomId, windowTabDomId } from "@/lib/window/windowDomIds";
import {
  windowTabStrip,
  windowTabList,
  windowTabWrapper,
  windowTabButton,
  windowTabCloseButton,
  windowTabEditorWrapper,
  windowTabInput,
  windowAddButton,
  windowOnboardingHint,
  windowOnboardingHintDismiss,
} from "@/styles/window/windowTabStrip.css";

/** Surface 10 (design/ux.md): permanently dismisses the tmux-vocabulary onboarding hint. */
const HINT_DISMISSED_KEY = "cockpit.windowHintDismissed";

function readHintDismissed(): boolean {
  try {
    return localStorage.getItem(HINT_DISMISSED_KEY) === "true";
  } catch {
    return false;
  }
}

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
  onSwitch: (id: WindowId, trigger?: WindowSwitchTrigger) => void;
  onCreate: () => void;
  onClose: (id: WindowId) => void;
  onRename: (id: WindowId, name: string) => void;
  /**
   * Hover-preview inputs. Callbacks (not data) because arePropsEqual below
   * deliberately ignores pane-tree changes; reading through them at hover time
   * always sees the latest layout. Omit either to disable the preview.
   */
  getWindows?: () => NamedWindow[];
  getSessionTitle?: SessionTitleLookup;
  onOpenOverview?: () => void;
}

const HOVER_PREVIEW_DELAY_MS = 300;
const HOVER_PREVIEW_CLOSE_MS = 150;

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
    if (
      dx > GESTURE_MOVEMENT_THRESHOLD_PX ||
      dy > GESTURE_MOVEMENT_THRESHOLD_PX
    ) {
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
  onSwitch: (id: WindowId, trigger?: WindowSwitchTrigger) => void;
  onClose: (id: WindowId) => void;
  onBeginEdit: (id: WindowId, name: string) => void;
  onDraftChange: (value: string) => void;
  onCommit: (id: WindowId) => void;
  onCancel: () => void;
  longPress: ReturnType<typeof useLongPress>;
  tabRef: (id: WindowId, el: HTMLDivElement | null) => void;
}

interface WindowTabEditorProps {
  id: WindowId;
  draftName: string;
  onDraftChange: (value: string) => void;
  onCommit: (id: WindowId) => void;
  onCancel: () => void;
}

function WindowTabEditor({
  id,
  draftName,
  onDraftChange,
  onCommit,
  onCancel,
}: WindowTabEditorProps) {
  // Select once on mount; an inline ref callback re-runs every render and would
  // re-select the text on each keystroke, so each character overwrites the last.
  const inputRef = useRef<HTMLInputElement>(null);
  useEffect(() => {
    inputRef.current?.focus();
    inputRef.current?.select();
  }, []);

  return (
    <div className={windowTabEditorWrapper}>
      <input
        ref={inputRef}
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

function WindowTabCloseButton({
  id,
  name,
  isActive,
  onClose,
}: WindowTabCloseButtonProps) {
  return (
    <button
      className={windowTabCloseButton({ active: isActive })}
      data-testid={`window-tab-close-${id}`}
      // Roving tabindex (WAI-ARIA APG "tabs" pattern) means the tablist is a
      // single stop in page Tab order; a native button defaults to tabIndex
      // 0 and would add a second, per-tab stop. Closing is still reachable
      // via mouse/touch here or via the tab's own Delete key (handleKeyDown).
      tabIndex={-1}
      aria-label={`Close ${name}`}
      title={`Close ${name}`}
      onClick={(e) => {
        e.stopPropagation();
        onClose(id);
      }}
    >
      {/* analytics-exempt */}
      &times;
    </button>
  );
}

/** `detail === 0` is a synthesized activation (assistive tech "activate"), treated like Enter/Space. */
function clickTrigger(e: React.MouseEvent): WindowSwitchTrigger {
  const pointerType = (e.nativeEvent as Partial<PointerEvent>).pointerType;
  if (pointerType === "touch" || pointerType === "pen") return "touch";
  return e.detail === 0 ? "tab-key" : "mouse";
}

interface WindowTabButtonProps {
  window: NamedWindow;
  isActive: boolean;
  showClose: boolean;
  onSwitch: (id: WindowId, trigger?: WindowSwitchTrigger) => void;
  onClose: (id: WindowId) => void;
  onBeginEdit: (id: WindowId, name: string) => void;
  longPress: ReturnType<typeof useLongPress>;
  tabRef: (id: WindowId, el: HTMLDivElement | null) => void;
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
function WindowTabButton({
  window: w,
  isActive,
  showClose,
  onSwitch,
  onClose,
  onBeginEdit,
  longPress,
  tabRef,
}: WindowTabButtonProps) {
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
      onSwitch(w.id, "tab-key");
    }
  };

  const handleClick = (e: React.MouseEvent) => {
    onSwitch(w.id, clickTrigger(e));
  };

  return (
    <div
      ref={(el) => tabRef(w.id, el)}
      role="tab"
      id={windowTabDomId(w.id)}
      aria-selected={isActive}
      // Only the selected tab's panel is guaranteed to be in the DOM (the keep-alive host adds the rest).
      aria-controls={isActive ? windowPanelDomId(w.id) : undefined}
      aria-label={w.name}
      tabIndex={isActive ? 0 : -1}
      data-testid={`window-tab-${w.id}`}
      className={windowTabButton({ active: isActive })}
      title={w.name}
      onClick={handleClick}
      onDoubleClick={() => onBeginEdit(w.id, w.name)}
      onKeyDown={handleKeyDown}
      onTouchStart={(e) => longPress.onTouchStart(e, w.id, w.name)}
      onTouchMove={longPress.onTouchMove}
      onTouchEnd={longPress.onTouchEnd}
    >
      {w.name}
      {showClose && (
        <WindowTabCloseButton
          id={w.id}
          name={w.name}
          isActive={isActive}
          onClose={onClose}
        />
      )}
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
  tabRef,
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
        tabRef={tabRef}
      />
    </div>
  );
}

const WindowTabStripInner = forwardRef<
  WindowTabStripHandle,
  WindowTabStripProps
>(function WindowTabStrip(
  {
    windows,
    currentWindowId,
    onSwitch,
    onCreate,
    onClose,
    onRename,
    getWindows,
    getSessionTitle,
    onOpenOverview,
  },
  ref,
) {
  const [editingId, setEditingId] = useState<WindowId | null>(null);
  const [draftName, setDraftName] = useState("");
  const [previewWindows, setPreviewWindows] = useState<NamedWindow[] | null>(null);
  const hoverTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

  const clearHoverTimer = () => {
    if (hoverTimer.current !== null) clearTimeout(hoverTimer.current);
    hoverTimer.current = null;
  };
  const scheduleHover = (open: boolean) => {
    clearHoverTimer();
    hoverTimer.current = setTimeout(
      () => setPreviewWindows(open && getWindows ? getWindows() : null),
      open ? HOVER_PREVIEW_DELAY_MS : HOVER_PREVIEW_CLOSE_MS,
    );
  };
  useEffect(() => clearHoverTimer, []);

  const beginEdit = useCallback((id: WindowId, name: string) => {
    setEditingId(id);
    setDraftName(name);
  }, []);

  const commitRename = (id: WindowId) => {
    clearHoverTimer();
    const trimmed = draftName.trim();
    if (trimmed.length > 0) {
      onRename(id, trimmed);
    }
    setEditingId(null);
  };

  const cancelRename = () => setEditingId(null);
  const longPress = useLongPress(beginEdit);

  const [hintDismissed, setHintDismissed] = useState(readHintDismissed);
  const dismissHint = () => {
    setHintDismissed(true);
    try {
      localStorage.setItem(HINT_DISMISSED_KEY, "true");
    } catch {
      // Degraded but acceptable (design/ux.md Surface 10): the hint may
      // reappear next session if localStorage is unavailable/full.
    }
  };
  const showHint = windows.length >= 2 && !hintDismissed;

  useImperativeHandle(ref, () => ({ beginEdit }), [beginEdit]);

  // Container ref for useWindowSwipe to attach touch listeners to
  // (Task 3.1.1b) — internal only; the forwarded ref exposes `beginEdit`
  // instead of this DOM node (nothing outside this component needs the
  // node directly).
  const containerRef = useRef<HTMLDivElement>(null);

  useWindowSwipe(containerRef, {
    onSwipe: (dir) => {
      const idx = windows.findIndex((w) => w.id === currentWindowId);
      const nextIdx =
        dir === "next"
          ? (idx + 1) % windows.length
          : (idx - 1 + windows.length) % windows.length;
      onSwitch(windows[nextIdx].id, "swipe");
    },
  });

  // Roving-tabindex support (WAI-ARIA APG "tabs" pattern, automatic
  // activation): tab DOM nodes are keyed by id here so ArrowLeft/Right/
  // Home/End can move both the switched-to window AND actual keyboard
  // focus in one keystroke — the nodes themselves persist across a
  // currentWindowId change (only their tabIndex/aria-selected re-render),
  // so focusing by id works without waiting on the re-render to land.
  const tabNodesRef = useRef<Map<WindowId, HTMLDivElement>>(new Map());
  const setTabRef = (id: WindowId, el: HTMLDivElement | null) => {
    if (el) tabNodesRef.current.set(id, el);
    else tabNodesRef.current.delete(id);
  };

  const handleTabListKeyDown = (e: React.KeyboardEvent<HTMLDivElement>) => {
    if (
      e.target !== e.currentTarget &&
      (e.target as HTMLElement).getAttribute("role") !== "tab"
    )
      return;
    const idx = windows.findIndex((w) => w.id === currentWindowId);
    if (idx === -1) return;
    let targetIdx: number | null = null;
    if (e.key === "ArrowRight") targetIdx = (idx + 1) % windows.length;
    else if (e.key === "ArrowLeft")
      targetIdx = (idx - 1 + windows.length) % windows.length;
    else if (e.key === "Home") targetIdx = 0;
    else if (e.key === "End") targetIdx = windows.length - 1;
    if (targetIdx === null) return;
    e.preventDefault();
    const target = windows[targetIdx];
    onSwitch(target.id, "tab-key");
    tabNodesRef.current.get(target.id)?.focus();
  };

  return (
    <>
      <div
        ref={containerRef}
        className={windowTabStrip}
        // Mouse only: a touch tap synthesizes mouseenter, which would pop the preview
        // open on every tap. Touch users get the overview button instead.
        onPointerEnter={(e) => e.pointerType === "mouse" && scheduleHover(true)}
        onPointerLeave={(e) => e.pointerType === "mouse" && scheduleHover(false)}
        onKeyDown={(e) => {
          if (e.key === "Escape") setPreviewWindows(null);
        }}
      >
        {/* role="tablist" wraps ONLY the tabs — the "+" button is a sibling
            outside this element's DOM subtree, not a tablist child, since
            aria-required-children mandates role="tab" for every direct
            (flattened) child of role="tablist". */}
        <div
          className={windowTabList}
          role="tablist"
          aria-label="Window switcher"
          onKeyDown={handleTabListKeyDown}
        >
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
              tabRef={setTabRef}
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
          {/* analytics-exempt */}
          +
        </button>
        {onOpenOverview && (
          <button
            className={windowAddButton}
            data-testid="window-overview-button"
            onClick={() => {
              setPreviewWindows(null);
              onOpenOverview();
            }}
            title="Window overview"
            aria-label="Window overview"
          >
            {/* analytics-exempt */}
            ⊞
          </button>
        )}
        {previewWindows && editingId === null && getSessionTitle && (
          <div
            className={overviewStyles.popover}
            data-testid="window-hover-preview"
            onMouseEnter={clearHoverTimer}
          >
            {previewWindows.map((w, i) => (
              <WindowPreviewCard
                key={w.id}
                window={w}
                position={i + 1}
                isActive={w.id === currentWindowId}
                getSessionTitle={getSessionTitle}
                className={overviewStyles.popoverCard}
                onSelect={(id) => {
                  setPreviewWindows(null);
                  onSwitch(id, "mouse");
                }}
              />
            ))}
          </div>
        )}
      </div>
      {showHint && (
        <div
          className={windowOnboardingHint}
          data-testid="window-onboarding-hint"
          role="note"
        >
          <span>
            ⓘ Windows group your panes into separate workspaces — like a tmux
            window, but the &quot;sessions&quot; here are your agent panes, not
            tmux sessions.
          </span>
          <button className={windowOnboardingHintDismiss} onClick={dismissHint}>
            {/* analytics-exempt */}
            Got it ✕
          </button>
        </div>
      )}
    </>
  );
});

WindowTabStripInner.displayName = "WindowTabStrip";

// Compares `windows` by the derived {id, name}[] list rather than by
// reference/full deep-equality — windowReducer's PANE_ACTION case rebuilds
// the entire `windows` array (including every other window's pane tree) on
// every pane action, so comparing full objects would defeat memoization on
// every keystroke/resize in the active window (pre-mortem.md P2 #4).
// Callback props (onSwitch/onCreate/onClose/onRename) are deliberately not
// compared: page.tsx passes fresh inline arrows every render, but each one
// only forwards to the hook's already-stable functions.
function arePropsEqual(
  prev: WindowTabStripProps,
  next: WindowTabStripProps,
): boolean {
  if (prev.currentWindowId !== next.currentWindowId) return false;
  if (prev.windows.length !== next.windows.length) return false;
  for (let i = 0; i < prev.windows.length; i++) {
    if (prev.windows[i].id !== next.windows[i].id) return false;
    if (prev.windows[i].name !== next.windows[i].name) return false;
  }
  return true;
}

export const WindowTabStrip = memo(WindowTabStripInner, arePropsEqual);
