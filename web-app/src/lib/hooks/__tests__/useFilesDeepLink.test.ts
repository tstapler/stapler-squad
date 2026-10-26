import { renderHook } from "@testing-library/react";
import { useFilesDeepLink } from "../useFilesDeepLink";

const tree = { current: null };
const FILE = "a/b.go";

type Props = { selectedPath: string | null; active?: boolean };
const setup = (initial: Props) =>
  renderHook(
    ({ selectedPath, active }: Props) => useFilesDeepLink({ initialSelectedPath: null, tree, selectedPath, active }),
    { initialProps: initial },
  );

describe("useFilesDeepLink hash ownership", () => {
  afterEach(() => window.history.replaceState(null, "", "/"));

  it("writes the hash on selection and clears it when the viewer closes", () => {
    const { rerender } = setup({ selectedPath: null });
    rerender({ selectedPath: FILE });
    expect(window.location.hash).toBe("#file=a%2Fb.go");
    rerender({ selectedPath: null });
    expect(window.location.hash).toBe("");
  });

  it("clears its hash on unmount", () => {
    const { rerender, unmount } = setup({ selectedPath: null });
    rerender({ selectedPath: FILE });
    unmount();
    expect(window.location.hash).toBe("");
  });

  it("clears its hash when the tab becomes inactive (tab switch or pane blur)", () => {
    const { rerender } = setup({ selectedPath: FILE, active: true });
    expect(window.location.hash).toBe("#file=a%2Fb.go");
    rerender({ selectedPath: FILE, active: false });
    expect(window.location.hash).toBe("");
  });

  it("an inactive tab never writes the hash", () => {
    const { rerender } = setup({ selectedPath: null, active: false });
    rerender({ selectedPath: FILE, active: false });
    expect(window.location.hash).toBe("");
  });

  it("does not clear a hash another pane wrote", () => {
    const { rerender } = setup({ selectedPath: FILE, active: true });
    window.history.replaceState(null, "", "/#file=other.go");
    rerender({ selectedPath: FILE, active: false });
    expect(window.location.hash).toBe("#file=other.go");
  });

  it("claims the hash when an inactive tab with a selection becomes active", () => {
    const { rerender } = setup({ selectedPath: FILE, active: false });
    expect(window.location.hash).toBe("");
    rerender({ selectedPath: FILE, active: true });
    expect(window.location.hash).toBe("#file=a%2Fb.go");
  });

  it("leaves a foreign hash intact when it becomes active with nothing selected", () => {
    window.history.replaceState(null, "", "/#file=other.go");
    const { rerender } = setup({ selectedPath: null, active: false });
    rerender({ selectedPath: null, active: true });
    expect(window.location.hash).toBe("#file=other.go");
  });

  it("hands the hash from one pane to the next", () => {
    const a = setup({ selectedPath: FILE, active: true });
    const b = setup({ selectedPath: "c/d.go", active: false });
    a.rerender({ selectedPath: FILE, active: false });
    b.rerender({ selectedPath: "c/d.go", active: true });
    expect(window.location.hash).toBe("#file=c%2Fd.go");
  });
});
