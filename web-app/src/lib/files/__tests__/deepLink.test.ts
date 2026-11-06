import { renderHook } from "@testing-library/react";
import { clampLine, fileHash, parseDeepLink } from "../deepLink";
import { useFilesDeepLink } from "@/lib/hooks/useFilesDeepLink";

describe("parseDeepLink", () => {
  it("deepLink_should_ParseFileAndLine_When_QueryOrHash", () => {
    expect(parseDeepLink("?file=src/main.go&line=42", "")).toEqual({ file: "src/main.go", line: 42 });
    expect(parseDeepLink("?file=a%2Fb.go", "")).toEqual({ file: "a/b.go" });
    expect(parseDeepLink("", "#file=a%2Fb.go")).toEqual({ file: "a/b.go" });
    expect(parseDeepLink("?file=x.go&line=0", "")).toEqual({ file: "x.go" });
    expect(parseDeepLink("?file=x.go&line=abc", "")).toEqual({ file: "x.go" });
    expect(parseDeepLink("?line=3", "")).toBeNull();
    expect(parseDeepLink("", "#other")).toBeNull();
    expect(parseDeepLink("?file=", "")).toBeNull();
  });

  it("deepLink_should_ClampLineIntoRange_When_GoToLineOverflowsOrUnderflows", () => {
    expect(clampLine(250, 300)).toEqual({ line: 250, clamped: false });
    expect(clampLine(9999, 300)).toEqual({ line: 300, clamped: true });
    expect(clampLine(0, 300)).toEqual({ line: 1, clamped: true });
    expect(clampLine(5, 0)).toEqual({ line: 1, clamped: true });
  });

  it("deepLink_should_BuildHash_When_PathOrNull", () => {
    expect(fileHash("a b/c.go")).toBe("#file=a%20b%2Fc.go");
    expect(fileHash(null)).toBe("");
  });
});

describe("useFilesDeepLink", () => {
  const tree = { current: { revealPath: jest.fn(), collapseAll: jest.fn() } };
  beforeEach(() => {
    window.history.replaceState(null, "", "/session/s1");
    tree.current.revealPath.mockClear();
  });

  it("useFilesDeepLink_should_PreserveHistoryState_When_SelectionChangesAfterPushState", () => {
    const { rerender } = renderHook(({ selected }) => useFilesDeepLink({ initialSelectedPath: null, tree, selectedPath: selected }), {
      initialProps: { selected: null as string | null },
    });
    window.history.pushState({ pane: "editor" }, "", window.location.href);
    rerender({ selected: "src/a.go" });
    expect(window.history.state).toEqual({ pane: "editor" });
    expect(window.location.hash).toBe("#file=src%2Fa.go");
    rerender({ selected: null });
    expect(window.history.state).toEqual({ pane: "editor" });
    expect(window.location.hash).toBe("");
    expect(window.location.pathname).toBe("/session/s1");
  });

  it("useFilesDeepLink_should_OpenPinnedAtLine_When_QueryDeepLinkOnMount", () => {
    window.history.replaceState(null, "", "/session/s1?file=src/main.go&line=42");
    const onJump = jest.fn();
    const { rerender } = renderHook(() => useFilesDeepLink({ initialSelectedPath: null, tree, selectedPath: null, onJump }));
    expect(onJump).toHaveBeenCalledTimes(1);
    expect(onJump).toHaveBeenCalledWith("src/main.go", 42);
    rerender();
    expect(onJump).toHaveBeenCalledTimes(1);
  });

  it("useFilesDeepLink_should_RevealCrossLinkedPathInTree_When_InitialPathChanges", () => {
    const { rerender } = renderHook(({ p }) => useFilesDeepLink({ initialSelectedPath: p, tree, selectedPath: null }), { initialProps: { p: "a.go" as string | null } });
    expect(tree.current.revealPath).toHaveBeenCalledWith("a.go");
    rerender({ p: "b.go" });
    expect(tree.current.revealPath).toHaveBeenLastCalledWith("b.go");
  });

  it("useFilesDeepLink_should_BeTheOnlyWriterOfFileHash_When_SessionDetailViewRendered", () => {
    // The old SessionDetailView effect called history.replaceState(null, ...) on every selection change.
    const src = require("fs").readFileSync(require("path").resolve(__dirname, "../../../components/sessions/SessionDetailView.tsx"), "utf8") as string;
    expect(src).not.toMatch(/history\.replaceState/);
    expect(src).not.toMatch(/`#file=/);
  });
});
