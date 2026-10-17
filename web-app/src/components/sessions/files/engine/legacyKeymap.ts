import { keymap, type EditorView } from "@codemirror/view";
import type { Extension } from "@codemirror/state";
import {
  cursorCharLeft, cursorCharRight, cursorLineDown, cursorLineUp,
  selectCharLeft, selectCharRight, selectLineDown, selectLineUp,
  cursorLineBoundaryBackward, cursorLineBoundaryForward,
  selectLineBoundaryBackward, selectLineBoundaryForward,
  cursorDocStart, cursorDocEnd, cursorPageDown, cursorPageUp,
  selectPageDown, selectPageUp,
} from "@codemirror/commands";
import { openSearchPanel, findNext, findPrevious } from "@codemirror/search";

// Vim-style "gg" needs two "g" presses in a short window; a lone "g" is swallowed.
export const DOUBLE_G_WINDOW_MS = 400;

export function isDoubleGPress(lastGPressAt: number, now: number, windowMs: number = DOUBLE_G_WINDOW_MS): boolean {
  return now - lastGPressAt < windowMs;
}

// Temporary hand-rolled hjkl keymap; Story 2.4.1 replaces it with @replit/codemirror-vim.
export function legacyVimKeymap(): Extension {
  let lastGPress = 0;
  return keymap.of([
    { key: "h", run: cursorCharLeft },
    { key: "l", run: cursorCharRight },
    { key: "j", run: cursorLineDown },
    { key: "k", run: cursorLineUp },
    { key: "0", run: cursorLineBoundaryBackward },
    { key: "$", run: cursorLineBoundaryForward },
    {
      key: "g",
      run: (v: EditorView) => {
        const now = Date.now();
        const isDoubleG = isDoubleGPress(lastGPress, now);
        lastGPress = isDoubleG ? 0 : now;
        return isDoubleG ? cursorDocStart(v) : true;
      },
    },
    { key: "G", run: cursorDocEnd },
    { key: "Ctrl-d", run: cursorPageDown },
    { key: "Ctrl-u", run: cursorPageUp },
    { key: "Shift-h", run: selectCharLeft },
    { key: "Shift-l", run: selectCharRight },
    { key: "Shift-j", run: selectLineDown },
    { key: "Shift-k", run: selectLineUp },
    { key: "Shift-0", run: selectLineBoundaryBackward },
    { key: "Shift-$", run: selectLineBoundaryForward },
    { key: "Shift-Ctrl-d", run: selectPageDown },
    { key: "Shift-Ctrl-u", run: selectPageUp },
    {
      key: "y",
      run: (v: EditorView) => {
        const { from, to } = v.state.selection.main;
        const selected = v.state.sliceDoc(from, to);
        if (selected) void navigator.clipboard.writeText(selected);
        return true;
      },
    },
    { key: "/", run: openSearchPanel },
    { key: "n", run: findNext },
    { key: "Shift-n", run: findPrevious },
  ]);
}
