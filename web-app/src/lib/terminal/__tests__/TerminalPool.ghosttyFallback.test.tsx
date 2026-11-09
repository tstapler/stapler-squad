/**
 * When ghostty-web fails to load, the pool falls back to xterm.js instead of
 * leaving every pooled terminal blank.
 */
import React from "react";
import { render, act } from "@testing-library/react";
import { TerminalPoolProvider, usePooledTerminal } from "../TerminalPool";

jest.mock("@/lib/features/flagStore", () => ({
  GHOSTTY_RENDERER_FLAG: "ghostty_renderer",
  preloadFlagValue: () => true,
}));

jest.mock("@/components/sessions/GhosttyTerminal", () => {
  const ReactLib = require("react");
  const GhosttyTerminal = ReactLib.forwardRef(() => ReactLib.createElement("div", { "data-testid": "mock-ghostty" }));
  return { GhosttyTerminal, loadGhostty: () => Promise.reject(new Error("wasm blocked")) };
});

jest.mock("@/components/sessions/XtermTerminal", () => {
  const ReactLib = require("react");
  const XtermTerminal = ReactLib.forwardRef((_props: unknown, ref: unknown) => {
    ReactLib.useImperativeHandle(ref, () => ({ terminal: { cols: 80, rows: 24 }, focus: jest.fn(), fit: jest.fn() }));
    return ReactLib.createElement("div", { "data-testid": "mock-xterm-terminal" });
  });
  return { XtermTerminal };
});

let handle: React.RefObject<unknown>;
function Consumer() {
  const dockRef = React.useRef<HTMLDivElement | null>(null);
  handle = usePooledTerminal("s1", dockRef, true).xtermRef;
  return <div ref={dockRef} />;
}

describe("TerminalPool ghostty fallback", () => {
  it("falls back to xterm and readies the handle when ghostty fails to load", async () => {
    const errSpy = jest.spyOn(console, "error").mockImplementation(() => {});
    const { queryByTestId } = render(
      <TerminalPoolProvider>
        <Consumer />
      </TerminalPoolProvider>
    );
    for (let i = 0; i < 4; i++) await act(async () => {});
    expect(queryByTestId("mock-xterm-terminal")).not.toBeNull();
    expect(handle.current).not.toBeNull();
    expect(errSpy).toHaveBeenCalledWith(expect.stringContaining("falling back to xterm.js"), expect.any(Error));
    errSpy.mockRestore();
  });
});
