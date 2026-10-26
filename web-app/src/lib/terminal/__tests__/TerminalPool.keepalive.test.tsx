/**
 * Story 4.1.2 terminal-pool policy: hidden retained windows keep their terminals docked, so the pool
 * may exceed maxSize by the retained terminals and nothing hidden is reclaimed (accepted, documented).
 */

import React from "react";
import { renderHook, act } from "@testing-library/react";
import { TerminalPoolProvider, useTerminalPool } from "../TerminalPool";

jest.mock("@/components/sessions/XtermTerminal", () => {
  const ReactLib = require("react");
  const XtermTerminal = ReactLib.forwardRef((_props: unknown, ref: unknown) => {
    ReactLib.useImperativeHandle(ref, () => ({ terminal: { cols: 80, rows: 24 }, focus: jest.fn(), fit: jest.fn() }));
    return ReactLib.createElement("div");
  });
  return { XtermTerminal };
});

const MAX_SIZE = 4;
const WINDOWS = ["w1", "w2", "w3"];
const PANES_PER_WINDOW = 4;

function wrapper({ children }: { children: React.ReactNode }) {
  return <TerminalPoolProvider maxSize={MAX_SIZE}>{children}</TerminalPoolProvider>;
}

describe("TerminalPool keep-alive policy", () => {
  it("pool_should_KeepHiddenWindowTerminalsDocked_And_ExceedMaxSizeByRetained_When_3x4TerminalsMounted", () => {
    const { result } = renderHook(() => useTerminalPool(), { wrapper });
    const anchor = document.createElement("div");
    document.body.appendChild(anchor);
    const ids = WINDOWS.flatMap((w) => Array.from({ length: PANES_PER_WINDOW }, (_, i) => `${w}-s${i}`));

    const entries = new Map<string, unknown>();
    act(() => {
      for (const id of ids) {
        entries.set(id, result.current.getOrCreateEntry(id));
        result.current.dock(id, anchor); // hidden windows are never undocked (windowActive gating is Story 4.1.3)
      }
    });

    expect(ids).toHaveLength(12);
    expect(ids.length).toBeGreaterThan(MAX_SIZE);
    // Re-fetching returns the same object for every id: nothing was reclaimed to honor maxSize.
    act(() => {
      for (const id of ids) expect(result.current.getOrCreateEntry(id)).toBe(entries.get(id));
    });
    document.body.removeChild(anchor);
  });
});
