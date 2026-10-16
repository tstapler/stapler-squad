import { bottomRightFootprint, deckAnchor } from "@/components/ui/toastDeckAnchor";

const footprint = bottomRightFootprint({ width: 1280, height: 800 }, { width: 360, height: 200 }, { right: 16, bottom: 24 });

describe("deckAnchor", () => {
  it("computes the bottom-right footprint from the viewport and insets", () => {
    expect(footprint).toEqual({ left: 904, right: 1264, top: 576, bottom: 776 });
  });

  it("stays bottom-right when the cursor is clear of the deck", () => {
    const cursor = { kind: "rect", rect: { left: 100, top: 400, width: 8, height: 16 } } as const;
    expect(deckAnchor(cursor, footprint)).toBe("bottom-right");
  });

  it("moves to the top-right when the cursor cell is under the deck", () => {
    const cursor = { kind: "rect", rect: { left: 1000, top: 700, width: 8, height: 16 } } as const;
    expect(deckAnchor(cursor, footprint)).toBe("top-right");
  });

  it("clears a cursor that sits just outside the deck gap", () => {
    const cursor = { kind: "rect", rect: { left: 100, top: 776 + 9, width: 8, height: 16 } } as const;
    expect(deckAnchor(cursor, footprint)).toBe("bottom-right");
  });

  it("uses the top-right when a mounted terminal's cursor cannot be read", () => {
    expect(deckAnchor({ kind: "unreadable" }, footprint)).toBe("top-right");
  });

  it("stays bottom-right when there is no terminal on the page", () => {
    expect(deckAnchor({ kind: "no-terminal" }, footprint)).toBe("bottom-right");
  });
});
