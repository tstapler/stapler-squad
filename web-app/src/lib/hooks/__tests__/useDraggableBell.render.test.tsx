import { renderHook } from "@testing-library/react";
import { useDraggableBell } from "../useDraggableBell";

jest.mock("@/lib/hooks/useAnnounce", () => ({ useAnnounce: () => ({ announce: jest.fn() }) }));
jest.mock("@/lib/hooks/useHandedness", () => ({ useHandedness: () => ({ leftHanded: false }) }));

describe("useDraggableBell render purity", () => {
  it("does not append probe elements to document.body on re-render", () => {
    const ref = { current: null };
    const { rerender } = renderHook(() => useDraggableBell(ref, true));
    const spy = jest.spyOn(document.body, "appendChild");
    rerender();
    rerender();
    expect(spy).not.toHaveBeenCalled();
    spy.mockRestore();
  });
});
