/**
 * Regression: more simultaneously-rendered consumers than the pool's maxSize.
 * getOrCreateEntry runs during render, before any consumer's pin/dock effect, so
 * the LRU sweep used to evict a sibling's not-yet-pinned entry. The sibling kept
 * its orphaned entry (no portal -> handleRef never populated) and fitOnShow gave
 * up with "handle ready: false".
 */
import React from "react";
import { render, act } from "@testing-library/react";
import { TerminalPoolProvider, usePooledTerminal } from "../TerminalPool";

jest.mock("@/components/sessions/XtermTerminal", () => {
  const ReactLib = require("react");
  const XtermTerminal = ReactLib.forwardRef((_props: unknown, ref: unknown) => {
    ReactLib.useImperativeHandle(ref, () => ({ terminal: { cols: 80, rows: 24 }, focus: jest.fn(), fit: jest.fn() }));
    return ReactLib.createElement("div", { "data-testid": "mock-xterm-terminal" });
  });
  return { XtermTerminal };
});

const rect = (w: number, h: number) =>
  ({ width: w, height: h, top: 0, left: 0, right: w, bottom: h, x: 0, y: 0, toJSON: () => ({}) }) as DOMRect;

const refs: Record<string, React.RefObject<unknown>> = {};

function Consumer({ id }: { id: string }) {
  const dockRef = React.useRef<HTMLDivElement | null>(null);
  const { xtermRef } = usePooledTerminal(id, dockRef, true);
  refs[id] = xtermRef;
  return <div ref={dockRef} />;
}

describe("TerminalPool over-committed (consumers > maxSize)", () => {
  beforeEach(() => {
    jest.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockReturnValue(rect(600, 400));
  });
  afterEach(() => jest.restoreAllMocks());

  it("every simultaneously-rendered consumer gets a ready handle", async () => {
    const ids = ["a", "b", "c", "d", "e"];
    render(
      <TerminalPoolProvider maxSize={2}>
        {ids.map((id) => <Consumer key={id} id={id} />)}
      </TerminalPoolProvider>
    );
    await act(async () => {});
    await act(async () => {});
    for (const id of ids) expect([id, !!refs[id].current]).toEqual([id, true]);
  });
});
