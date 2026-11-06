/** Shared suite body: asserts each vector's result against the renderer's recorded baseline. */
import { loadBaseline, loadVectors, vectorPasses, type Outcome, type Vector } from "./vectors";

export type RenderVector = (v: Vector) => Promise<Outcome>;

export function describeConformance(renderer: "xterm" | "ghostty", render: RenderVector): void {
  const baseline = loadBaseline(renderer);
  const vectors = loadVectors();

  it("has at least 12 vectors", () => {
    expect(vectors.length).toBeGreaterThanOrEqual(12);
  });

  it.each(vectors.map((v) => [v.name, v] as const))("%s", async (_name, v) => {
    const got = await render(v);
    const passes = vectorPasses(v, got);
    if (baseline.knownFailing.includes(v.name)) {
      // A fixed gap must be removed from baseline.json so a later regression is caught.
      expect({ vector: v.name, passes }).toEqual({ vector: v.name, passes: false });
    } else {
      expect({ vector: v.name, screen: got.screen, response: v.expectedResponse === undefined ? undefined : got.response })
        .toEqual({ vector: v.name, screen: v.expectedScreen, response: v.expectedResponse });
    }
  });
}
