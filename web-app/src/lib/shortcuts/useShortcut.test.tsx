import { renderHook } from "@testing-library/react";
import { registry } from "./shortcutRegistry";
import { useShortcut } from "./useShortcut";

const def = { key: "t", modifiers: { ctrl: true }, label: "x", context: "terminal" as const, action: () => {} };

describe("useShortcut enabled flag", () => {
  it("registers once when several instances share an id and only one is enabled, without warning", () => {
    const warn = jest.spyOn(console, "warn").mockImplementation(() => {});
    const a = renderHook(() => useShortcut("dup.id", def, true));
    const b = renderHook(() => useShortcut("dup.id", def, false));
    const c = renderHook(() => useShortcut("dup.id", def, false));
    expect(warn).not.toHaveBeenCalled();
    c.unmount(); // a disabled instance unmounting must not delete the enabled one's registration
    b.unmount();
    expect(registry.getAll().terminal.some((s) => s.label === "x")).toBe(true);
    a.unmount();
    expect(registry.getAll().terminal.some((s) => s.label === "x")).toBe(false);
    warn.mockRestore();
  });
});
