// +feature: command-bar
"use client";

import { useCallback, useEffect, useId, useMemo, useReducer, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { registry } from "@/lib/shortcuts/shortcutRegistry";
import { chordMatches, formatLeader, getLeaderConfig, LEADER_CONFIG_EVENT } from "@/lib/shortcuts/leaderConfig";
import { useFocusTrap } from "@/lib/hooks/useFocusTrap";
import { commandBarController } from "./commandBarController";
import {
  barReducer,
  childrenAt,
  CLOSED_STATE,
  filterRows,
  interpretKey,
  isFuzzy,
  paletteRows,
  pathLabel,
  ROW_HEIGHT_PX,
  LIST_MAX_HEIGHT_PX,
  virtualWindow,
  VIRTUALIZE_THRESHOLD,
  type PaletteRow,
  type Row,
} from "./commandBarState";
import * as styles from "./CommandBar.css";

/**
 * The notification tray is a permanent, non-modal role="dialog" (closed it stays
 * in the DOM with data-state="closed"; open it deliberately leaves the page
 * usable), so it never blocks the leader.
 */
const NON_BLOCKING_DIALOG = '[data-testid="notification-tray"]';

function isBlockingDialog(d: HTMLElement): boolean {
  if (d.hidden || d.matches(NON_BLOCKING_DIALOG)) return false;
  if (d.getAttribute("data-state") === "closed") return false;
  if (d.closest('[aria-hidden="true"], [inert]')) return false;
  return typeof d.checkVisibility === "function" ? d.checkVisibility() : true;
}

function hasOpenDialog(): boolean {
  const dialogs = document.querySelectorAll<HTMLElement>('[role="dialog"], [role="alertdialog"]');
  return Array.from(dialogs).some(isBlockingDialog);
}

/**
 * Leader-key command bar: which-key menu after the leader, switching to a fuzzy
 * palette when a non-bound character is typed. Both views are generated from the
 * shortcut registry. Mount once (CockpitShell).
 *
 * The leader listener runs in the capture phase and consumes the event, so a
 * focused terminal (xterm) never sees the chord or any key while the bar is open.
 */
export function CommandBar() {
  const [state, dispatch] = useReducer(barReducer, CLOSED_STATE);
  const [version, setVersion] = useState(0);
  const [mounted, setMounted] = useState(false);
  const [scrollTop, setScrollTop] = useState(0);
  const dialogRef = useRef<HTMLDivElement>(null);
  const listRef = useRef<HTMLUListElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);
  const triggerRef = useRef<HTMLElement | null>(null);
  const stateRef = useRef(state);
  stateRef.current = state;
  const rowsRef = useRef<Row[]>([]);
  const listboxId = useId();

  useEffect(() => {
    setMounted(true);
    // Readiness marker: the capture listener below is attached by this point.
    document.body.dataset.commandBarReady = "true";
    return () => {
      delete document.body.dataset.commandBarReady;
    };
  }, []);

  const openBar = useCallback((path: string[]): boolean => {
    if (stateRef.current.open) return true;
    if (hasOpenDialog()) return false;
    const active = document.activeElement;
    triggerRef.current = active instanceof HTMLElement && active !== document.body ? active : null;
    if (path.length > 0) registry.resolveLazy(path);
    dispatch({ type: "open", path });
    return true;
  }, []);

  const closeBar = useCallback(() => dispatch({ type: "close" }), []);

  useEffect(() => commandBarController.attach(openBar), [openBar]);

  useEffect(() => {
    if (!state.open) registry.releaseLazy();
  }, [state.open]);

  const fuzzy = isFuzzy(state);
  const tree = useMemo(() => registry.getTree(), [state.open, state.path, version]); // eslint-disable-line react-hooks/exhaustive-deps

  const rows: Row[] = useMemo(() => {
    if (!state.open) return [];
    if (fuzzy) {
      return filterRows(paletteRows(registry.getPalette()), state.query).map((r: PaletteRow) => ({
        id: r.id, label: r.label, group: r.group, keys: r.keys, run: r.run,
      }));
    }
    return childrenAt(tree, state.path).map((n) => ({
      id: `${state.path.join(".")}.${n.key}`,
      label: n.label,
      group: "",
      keys: [n.key],
      branchKey: n.shortcut ? undefined : n.key,
      run: n.shortcut?.action,
    }));
  }, [state.open, state.path, state.query, fuzzy, tree]);
  rowsRef.current = rows;

  const execute = useCallback((row: Row) => {
    if (row.branchKey !== undefined) {
      registry.resolveLazy([...stateRef.current.path, row.branchKey]);
      setVersion((v) => v + 1);
      dispatch({ type: "descend", key: row.branchKey });
      return;
    }
    dispatch({ type: "close" });
    // After close commits, so focus is restored before the action (which may open its own dialog).
    if (row.run) setTimeout(row.run, 0);
  }, []);

  // Capture-phase keyboard handling: one listener for arm + all in-bar keys.
  useEffect(() => {
    const consume = (e: KeyboardEvent) => {
      e.preventDefault();
      e.stopPropagation();
      e.stopImmediatePropagation();
    };

    const onKeyDown = (e: KeyboardEvent) => {
      const cfg = getLeaderConfig();
      const s = stateRef.current;

      if (!s.open) {
        if (!cfg.enabled || !chordMatches(e, cfg.chord)) return;
        // IME composition or an open modal: the leader is a no-op and the key passes through.
        if (e.isComposing || e.keyCode === 229 || hasOpenDialog()) return;
        consume(e);
        openBar([]);
        return;
      }

      if (e.isComposing || e.keyCode === 229) return; // let the IME reach the input
      consume(e);
      if (e.repeat && e.key === "Escape") return;
      if (cfg.enabled && chordMatches(e, cfg.chord)) {
        dispatch({ type: "close" });
        return;
      }

      const outcome = interpretKey(e, s, rowsRef.current, registry.getTree());
      if (outcome.type === "action") dispatch(outcome.action);
      else if (outcome.type === "execute") execute(outcome.row);
      else if (outcome.type === "seedQuery") {
        registry.resolveLazy([]);
        setVersion((v) => v + 1);
        dispatch({ type: "setQuery", query: s.query + outcome.char });
      }
    };

    const onLeaderConfigChange = () => setVersion((v) => v + 1);
    document.addEventListener("keydown", onKeyDown, true);
    window.addEventListener(LEADER_CONFIG_EVENT, onLeaderConfigChange);
    return () => {
      document.removeEventListener("keydown", onKeyDown, true);
      window.removeEventListener(LEADER_CONFIG_EVENT, onLeaderConfigChange);
    };
  }, [openBar, execute]);

  useFocusTrap(dialogRef, state.open && mounted, triggerRef);

  // The combobox input owns focus (aria-activedescendant points from it); the
  // trap would otherwise land on the Back button when opened at a sub-level.
  useEffect(() => {
    if (state.open && mounted) inputRef.current?.focus();
  }, [state.open, mounted, state.path]);

  // Keep the active row visible (also required once the list is virtualised).
  useEffect(() => {
    const el = listRef.current;
    if (!el || !state.open) return;
    const top = state.activeIndex * ROW_HEIGHT_PX;
    if (top < el.scrollTop) el.scrollTop = top;
    else if (top + ROW_HEIGHT_PX > el.scrollTop + el.clientHeight && el.clientHeight > 0) {
      el.scrollTop = top + ROW_HEIGHT_PX - el.clientHeight;
    }
  }, [state.activeIndex, state.open, rows.length]);

  if (!mounted || !state.open) return null;

  const total = rows.length;
  const virtual = total > VIRTUALIZE_THRESHOLD;
  const win = virtualWindow(total, scrollTop, LIST_MAX_HEIGHT_PX, ROW_HEIGHT_PX);
  const visible = rows.slice(win.start, win.end);
  const optionId = (i: number) => `${listboxId}-opt-${i}`;
  const breadcrumb = state.path.length > 0 ? `${pathLabel(tree, state.path)} › ${state.path.join(" ")}` : "";
  const leaderCfg = getLeaderConfig();

  return createPortal(
    <div
      className={styles.backdrop}
      data-testid="command-bar-backdrop"
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) closeBar();
      }}
    >
      <div
        ref={dialogRef}
        role="dialog"
        aria-modal="true"
        aria-label="Command bar"
        className={styles.dialog}
        data-testid="command-bar"
        data-mode={fuzzy ? "fuzzy" : "whichkey"}
      >
        <div className={styles.header}>
          {state.path.length > 0 && !fuzzy && (
            <button type="button" className={styles.iconButton} aria-label="Back one level" onClick={() => dispatch({ type: "back" })}>
              ←
            </button>
          )}
          <input
            ref={inputRef}
            className={styles.searchInput}
            role="combobox"
            aria-expanded="true"
            aria-controls={listboxId}
            aria-activedescendant={total > 0 ? optionId(state.activeIndex) : undefined}
            aria-autocomplete="list"
            aria-label="Search commands"
            placeholder={fuzzy ? "" : "Press a key, or type to search…"}
            value={state.query}
            onChange={(e) => dispatch({ type: "setQuery", query: e.target.value })}
            data-testid="command-bar-input"
            autoComplete="off"
            spellCheck={false}
          />
          <button type="button" className={styles.iconButton} aria-label="Close command bar" onClick={closeBar} data-testid="command-bar-close">
            ✕
          </button>
        </div>
        {breadcrumb && <div className={styles.breadcrumb} data-testid="command-bar-path">{breadcrumb}</div>}
        {total === 0 ? (
          <div className={styles.empty} role="status">No matching commands</div>
        ) : (
          <ul
            ref={listRef}
            id={listboxId}
            role="listbox"
            aria-label={fuzzy ? "Matching commands" : "Next keys"}
            className={styles.list}
            onScroll={virtual ? (e) => setScrollTop(e.currentTarget.scrollTop) : undefined}
            data-testid="command-bar-list"
          >
            {virtual && win.padTop > 0 && <li aria-hidden="true" style={{ height: win.padTop }} />}
            {visible.map((row, i) => {
              const index = win.start + i;
              return (
                <li
                  key={row.id}
                  id={optionId(index)}
                  role="option"
                  aria-selected={index === state.activeIndex}
                  aria-setsize={total}
                  aria-posinset={index + 1}
                  className={styles.row}
                  data-testid="command-bar-item"
                  data-action-id={row.id}
                  onMouseMove={() => index !== stateRef.current.activeIndex && dispatch({ type: "setActive", index })}
                  onClick={() => execute(row)}
                >
                  <span className={styles.keys}>
                    {row.keys.map((k, ki) => (
                      <kbd key={ki} className={styles.keyCap}>{k}</kbd>
                    ))}
                  </span>
                  <span className={styles.rowLabel}>{row.label}</span>
                  {row.group && <span className={styles.rowGroup}>{row.group}</span>}
                  {row.branchKey !== undefined && <span aria-hidden="true">›</span>}
                </li>
              );
            })}
            {virtual && win.padBottom > 0 && <li aria-hidden="true" style={{ height: win.padBottom }} />}
          </ul>
        )}
        {state.helpOpen && (
          <div className={styles.help} data-testid="command-bar-help" role="note">
            <div>Keys at this level: {rows.map((r) => `${r.keys.join(" ")} ${r.label}`).join(", ") || "none"}.</div>
            <div>j / k or ↑ / ↓ move · Enter runs · Esc goes back (closes at the top) · type to search · ? toggles this help.</div>
            <div>Leader: {formatLeader(leaderCfg)}</div>
          </div>
        )}
        <div className={styles.srOnly} aria-live="polite" role="status">
          {fuzzy ? `${total} matching commands` : breadcrumb ? `${breadcrumb}, ${total} keys` : `${total} groups`}
        </div>
      </div>
    </div>,
    document.body,
  );
}
