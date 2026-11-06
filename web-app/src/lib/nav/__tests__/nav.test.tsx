/**
 * Contract tests for the navigation facade: `impl.memory.tsx`, the search-param parsers and
 * the pathname normaliser. The global "@/lib/nav" jest stub is bypassed on purpose (requireActual).
 */
import { act, fireEvent, render, renderHook, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import type * as MemoryImplModule from "../impl.memory";
import type { NavImpl } from "../impl.types";
import { normalizePathname } from "../normalize";
import { enumParam, intParam, stringParam } from "../search-params";

const memoryImpl = jest.requireActual<typeof MemoryImplModule>("../impl.memory");

beforeEach(() => {
  window.history.replaceState(null, "", "/");
});

describe("normalizePathname", () => {
  it.each([
    ["/review-queue/", "/review-queue"],
    ["/review-queue", "/review-queue"],
    ["/", "/"],
    ["", "/"],
    ["/a/b//", "/a/b"],
  ])("%s -> %s", (input, expected) => {
    expect(normalizePathname(input)).toBe(expected);
  });
});

describe("search-param parsers", () => {
  const params = new URLSearchParams("tab=prs&n=12&bad=1x&empty=");
  it("stringParam", () => {
    expect(stringParam(params, "tab")).toBe("prs");
    expect(stringParam(params, "empty")).toBeUndefined();
    expect(stringParam(params, "missing")).toBeUndefined();
  });
  it("intParam", () => {
    expect(intParam(params, "n")).toBe(12);
    expect(intParam(params, "bad")).toBeUndefined();
    expect(intParam(params, "missing")).toBeUndefined();
  });
  it("enumParam", () => {
    expect(enumParam(params, "tab", ["prs", "stuck"] as const)).toBe("prs");
    expect(enumParam(params, "n", ["prs", "stuck"] as const)).toBeUndefined();
  });
});

describe.each([["impl.memory", () => memoryImpl.memoryImpl as NavImpl]])("%s history helpers keep foreign history.state", (_name, impl) => {
  it("useReplaceSearch changes the query and keeps history.state", () => {
    window.history.replaceState({ idx: 3, ssqTray: true }, "", "/unfinished");
    const { result } = renderHook(() => impl().useReplaceSearch());
    act(() => result.current("?tab=b"));
    expect(window.location.search).toBe("?tab=b");
    expect(window.history.state).toEqual({ idx: 3, ssqTray: true });
  });

  it("replaceHash sets and clears the hash, keeping state", () => {
    window.history.replaceState({ keep: 1 }, "", "/x?y=1");
    impl().replaceHash("#file=a%2Fb");
    expect(window.location.hash).toBe("#file=a%2Fb");
    impl().replaceHash("");
    expect(window.location.href).toBe("http://localhost/x?y=1");
    expect(window.history.state).toEqual({ keep: 1 });
  });

  it("pushTrayEntry pushes a marked entry and, with replace, reuses the current one", () => {
    window.history.replaceState({ keep: 1 }, "", "/");
    const before = window.history.length;
    impl().pushTrayEntry();
    expect(window.history.length).toBe(before + 1);
    expect(window.history.state).toEqual({ keep: 1, ssqTray: true });
    impl().pushTrayEntry(true);
    expect(window.history.length).toBe(before + 1);
  });

  it("pushHistoryMarker adds a keyed marker to the current state; replace reuses the entry", () => {
    window.history.replaceState({ keep: 1 }, "", "/");
    const before = window.history.length;
    impl().pushHistoryMarker("depth", 2);
    expect(window.history.length).toBe(before + 1);
    expect(window.history.state).toEqual({ keep: 1, depth: 2 });
    impl().pushHistoryMarker("depth", 3, true);
    expect(window.history.length).toBe(before + 1);
    expect(window.history.state).toEqual({ keep: 1, depth: 3 });
  });
});

describe("impl.memory navigation", () => {
  const wrapper = ({ children }: { children: ReactNode }) => (
    <memoryImpl.MemoryNavProvider initialEntries={["/start/?a=1"]}>{children}</memoryImpl.MemoryNavProvider>
  );

  it("reports a normalised pathname and stable search params", () => {
    const { result, rerender } = renderHook(() => ({ p: memoryImpl.usePathname(), s: memoryImpl.useSearchParams() }), { wrapper });
    expect(result.current.p).toBe("/start");
    expect(result.current.s.get("a")).toBe("1");
    const firstParams = result.current.s;
    rerender();
    expect(result.current.s).toBe(firstParams);
  });

  it("push adds an entry, replace swaps it, back returns", () => {
    const { result } = renderHook(() => ({ r: memoryImpl.useRouter(), p: memoryImpl.usePathname() }), { wrapper });
    act(() => result.current.r.push("/one"));
    expect(result.current.p).toBe("/one");
    act(() => result.current.r.replace("/two"));
    expect(result.current.p).toBe("/two");
    act(() => result.current.r.back());
    expect(result.current.p).toBe("/start");
    act(() => result.current.r.forward());
    expect(result.current.p).toBe("/two");
  });

  it("Link click navigates via push, or replace with the replace prop", () => {
    const Probe = () => (
      <>
        <memoryImpl.Link href="/pushed" data-testid="push">p</memoryImpl.Link>
        <memoryImpl.Link href="/replaced" replace data-testid="replace">r</memoryImpl.Link>
        <span data-testid="path">{memoryImpl.usePathname()}</span>
      </>
    );
    render(<Probe />, { wrapper });
    fireEvent.click(screen.getByTestId("push"));
    expect(screen.getByTestId("path").textContent).toBe("/pushed");
    fireEvent.click(screen.getByTestId("replace"));
    expect(screen.getByTestId("path").textContent).toBe("/replaced");
  });

  it("Redirect replaces the location", () => {
    const Probe = () => <span data-testid="path">{memoryImpl.usePathname()}</span>;
    render(<><memoryImpl.Redirect to="/settings" /><Probe /></>, { wrapper });
    expect(screen.getByTestId("path").textContent).toBe("/settings");
  });

  it("without a provider the hooks report `/` and navigation goes nowhere", () => {
    const { result } = renderHook(() => ({ r: memoryImpl.useRouter(), p: memoryImpl.usePathname() }));
    act(() => result.current.r.push("/ignored"));
    expect(result.current.p).toBe("/");
  });
});
