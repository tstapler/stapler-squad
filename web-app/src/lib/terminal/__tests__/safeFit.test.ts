import { FitAddon } from "@xterm/addon-fit";
import type { Terminal } from "@xterm/xterm";
import { safeFit, safeProposeDimensions, type FitLike } from "../safeFit";

// A terminal that is attached to the DOM but whose renderer does not exist yet (or was just
// swapped out): the state a reload with several terminals mounting at once hits.
function terminalWithoutRenderer(): Terminal {
  return { element: { parentElement: {} }, _core: {} } as unknown as Terminal;
}

describe("safeFit", () => {
  it("reproduces the xterm failure it guards against", () => {
    const fit = new FitAddon();
    fit.activate(terminalWithoutRenderer());
    expect(() => fit.proposeDimensions()).toThrow(/reading 'dimensions'/);
  });

  it("reports no proposal instead of throwing when the renderer is missing", () => {
    const fit = new FitAddon();
    fit.activate(terminalWithoutRenderer());
    expect(safeProposeDimensions(fit)).toBeUndefined();
    expect(safeFit(fit)).toBe(false);
  });

  it("does not call fit() when there is nothing to measure", () => {
    const fit: FitLike = { proposeDimensions: () => undefined, fit: jest.fn() };
    expect(safeFit(fit)).toBe(false);
    expect(fit.fit).not.toHaveBeenCalled();
  });

  it("fits when the terminal can be measured", () => {
    const fit: FitLike = { proposeDimensions: () => ({ cols: 80, rows: 24 }), fit: jest.fn() };
    expect(safeFit(fit)).toBe(true);
    expect(fit.fit).toHaveBeenCalledTimes(1);
  });

  it("survives the renderer disappearing between the proposal and the fit", () => {
    const fit: FitLike = {
      proposeDimensions: () => ({ cols: 80, rows: 24 }),
      fit: () => {
        throw new TypeError("Cannot read properties of undefined (reading 'dimensions')");
      },
    };
    expect(safeFit(fit)).toBe(false);
  });

  it("rejects non-finite proposals", () => {
    const fit: FitLike = { proposeDimensions: () => ({ cols: NaN, rows: 24 }), fit: jest.fn() };
    expect(safeFit(fit)).toBe(false);
    expect(fit.fit).not.toHaveBeenCalled();
  });
});
