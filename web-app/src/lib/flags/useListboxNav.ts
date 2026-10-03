import { useCallback, useState } from "react";

export interface ListboxState {
  open: boolean;
  /** -1 until the user arrows to an option; the first option is never auto-active. */
  active: number;
}

export interface ListboxKeyResult {
  state: ListboxState;
  /** Index to accept (Enter, or Tab after arrowing), else null. */
  accept: number | null;
  /** True when the caller should preventDefault. */
  handled: boolean;
}

export const CLOSED: ListboxState = { open: false, active: -1 };

/** Pure key-to-state reducer shared by combobox-style listboxes. */
export function listboxKey(
  state: ListboxState,
  key: string,
  altKey: boolean,
  count: number,
): ListboxKeyResult {
  const unhandled = { state, accept: null, handled: false };
  if (count === 0) return unhandled;
  const hasActive = state.open && state.active >= 0 && state.active < count;
  switch (key) {
    case "ArrowDown":
      if (altKey) return { state: { open: true, active: state.active }, accept: null, handled: true };
      return {
        state: { open: true, active: state.open ? (state.active + 1) % count : 0 },
        accept: null,
        handled: true,
      };
    case "ArrowUp": {
      const active = !state.open || state.active <= 0 ? count - 1 : state.active - 1;
      return { state: { open: true, active }, accept: null, handled: true };
    }
    case "Escape":
      return state.open ? { state: CLOSED, accept: null, handled: true } : unhandled;
    case "Enter":
      return hasActive ? { state: CLOSED, accept: state.active, handled: true } : unhandled;
    case "Tab":
      if (hasActive) return { state: CLOSED, accept: state.active, handled: true };
      return state.open ? { state: CLOSED, accept: null, handled: false } : unhandled;
    default:
      return unhandled;
  }
}

export function useListboxNav() {
  const [state, setState] = useState<ListboxState>(CLOSED);
  const onKey = useCallback(
    (key: string, altKey: boolean, count: number): ListboxKeyResult => {
      const result = listboxKey(state, key, altKey, count);
      if (result.state !== state) setState(result.state);
      return result;
    },
    [state],
  );
  const open = useCallback(() => setState({ open: true, active: -1 }), []);
  const close = useCallback(() => setState(CLOSED), []);
  return { ...state, onKey, openList: open, close };
}
