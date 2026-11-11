import { resolveRenderer, type RendererInputs } from "./rendererPolicy";

const table: Array<[RendererInputs, "xterm" | "ghostty"]> = [
  [{ ghosttyFlag: false, mobileLayerFlag: false, attachable: false }, "xterm"],
  [{ ghosttyFlag: false, mobileLayerFlag: false, attachable: true }, "xterm"],
  [{ ghosttyFlag: false, mobileLayerFlag: true, attachable: false }, "xterm"],
  [{ ghosttyFlag: false, mobileLayerFlag: true, attachable: true }, "xterm"],
  [{ ghosttyFlag: true, mobileLayerFlag: false, attachable: false }, "ghostty"], // desktop/iOS unchanged
  [{ ghosttyFlag: true, mobileLayerFlag: false, attachable: true }, "xterm"], // Android class without the layer
  [{ ghosttyFlag: true, mobileLayerFlag: true, attachable: false }, "ghostty"],
  [{ ghosttyFlag: true, mobileLayerFlag: true, attachable: true }, "ghostty"],
];

describe("resolveRenderer", () => {
  it.each(table)("resolveRenderer_should_Return_%j", (input, want) => {
    expect(resolveRenderer(input)).toBe(want);
  });

  it("covers all 8 combinations", () => {
    expect(new Set(table.map(([i]) => JSON.stringify(i))).size).toBe(8);
  });
});
