/**
 * @jest-environment node
 */
import { createViewportSettle } from "../viewportSettle";

describe("createViewportSettle without a DOM", () => {
  it("viewportSettle_should_ThrowClearError_When_NoVisualViewportAndNoWindow", () => {
    const scheduler = { request: jest.fn(), cancel: jest.fn(), now: () => 0 };
    expect(() =>
      createViewportSettle(null, scheduler, { stableFrames: 3, maxWaitMs: 600, onSettled: jest.fn() })
    ).toThrow(/pass options\.win/);
  });
});
