import { useEffect } from "react";
import type { ReactNode } from "react";
import { render, screen, within } from "@testing-library/react";
import { WindowsProvider, WindowHostSlot, useWindowsContext, type WindowHostInputs } from "../WindowsProvider";
import type { NamedWindow } from "../windowTypes";
import { defined } from "@/lib/utils/defined";

let mockPathname = "/";
let mockKeepAlive = true;
jest.mock("@/lib/nav", () => ({ usePathname: () => mockPathname }));
jest.mock("@/lib/hooks/useFeatureFlagValue", () => ({ useFeatureFlagValue: () => mockKeepAlive }));

const mounts: string[] = [];
const activity: Record<string, boolean> = {};

function Marker({ id, active }: { id: string; active: boolean }) {
  useEffect(() => {
    mounts.push(id);
  }, [id]);
  activity[id] = active;
  return <span data-testid={`marker-${id}`} />;
}

const windows = ["w1", "w2", "w3"].map((id) => ({
  id,
  name: id,
  paneState: { root: { type: "leaf", id: `p-${id}`, viewKind: "session-detail", sessionId: null, activeTab: "info" }, focusedPaneId: `p-${id}` },
})) as unknown as NamedWindow[];
const focusMemory = { record: jest.fn(), recall: jest.fn(() => undefined) } as never;

function inputs(currentWindowId: string): WindowHostInputs {
  return {
    windows,
    currentWindowId,
    focusMemory,
    externalAssign: null,
    dispatchPane: jest.fn(),
    renderWindow: (w, { active }) => <Marker id={w.id} active={active} />,
    wrap: (host: ReactNode) => <div data-testid="wrapped">{host}</div>,
  };
}

function Page({ current }: { current: string }) {
  return <WindowHostSlot inputs={inputs(current)} />;
}

function Harness({ path, current = "w1" }: { path: string; current?: string }) {
  mockPathname = path;
  return <WindowsProvider>{path === "/" ? <Page current={current} /> : <span>backlog</span>}</WindowsProvider>;
}

// The slot is detached from the document off-route, so query it directly rather than through `screen`.
function panelFor(slot: HTMLElement, id: string) {
  return within(slot).getByTestId(`marker-${id}`).closest('[role="tabpanel"]') as HTMLElement;
}

beforeEach(() => {
  mounts.length = 0;
  mockKeepAlive = true;
  for (const k of Object.keys(activity)) delete activity[k];
});

describe("WindowsProvider", () => {
  it("windowsProvider_should_NotRemountContainers_When_NavigateHomeBacklogHome", () => {
    const { rerender } = render(<Harness path="/" />);
    rerender(<Harness path="/" current="w2" />);
    rerender(<Harness path="/" current="w3" />);
    rerender(<Harness path="/" current="w1" />);
    expect([...mounts].sort()).toEqual(["w1", "w2", "w3"]);
    const slot = screen.getByTestId("window-host-slot");

    rerender(<Harness path="/backlog" />);
    for (const id of ["w1", "w2", "w3"]) {
      expect(panelFor(slot, id).hidden).toBe(true);
      expect(panelFor(slot, id).hasAttribute("inert")).toBe(true);
      expect(activity[id]).toBe(false);
    }

    rerender(<Harness path="/" />);
    expect([...mounts].sort()).toEqual(["w1", "w2", "w3"]);
    expect(panelFor(slot, "w1").hidden).toBe(false);
    expect(activity.w1).toBe(true);
    expect(panelFor(slot, "w2").hidden).toBe(true);
  });

  it("windowsProvider_should_ParkSlotInsidePagePlaceholder_And_DetachItOffRoute", () => {
    const { rerender } = render(<Harness path="/" />);
    const slot = screen.getByTestId("window-host-slot");
    expect(screen.getByTestId("window-host-placeholder").contains(slot)).toBe(true);
    rerender(<Harness path="/backlog" />);
    expect(document.body.contains(slot)).toBe(false);
    rerender(<Harness path="/" />);
    expect(screen.getByTestId("window-host-slot")).toBe(slot);
  });

  it("windowsProvider_should_RenderNoHost_When_KeepAliveOff", () => {
    mockKeepAlive = false;
    render(<Harness path="/" />);
    expect(screen.getByTestId("window-host-slot").childElementCount).toBe(0);
    expect(screen.queryByTestId("wrapped")).toBeNull();
  });

  it("windowsProvider_should_FocusNothing_When_FocusActiveDetailCalledOffRoute", () => {
    let focus: (() => void) | undefined;
    let detailRef: ReturnType<NonNullable<ReturnType<typeof useWindowsContext>>["controller"]["detailRefFor"]> | undefined;
    function Probe() {
      const ctx = useWindowsContext();
      focus = ctx?.controller.focusActiveDetail;
      detailRef = ctx?.controller.detailRefFor("w1");
      ctx?.controller.syncCurrentWindow("w1");
      return null;
    }
    const el = document.createElement("div");
    el.tabIndex = -1;
    document.body.appendChild(el);
    const view = (path: string) => {
      mockPathname = path;
      return (
        <WindowsProvider>
          <Probe />
        </WindowsProvider>
      );
    };
    const { rerender } = render(view("/backlog"));
    defined(detailRef).current = el;
    defined(focus)();
    expect(document.activeElement).not.toBe(el);
    rerender(view("/"));
    defined(focus)();
    expect(document.activeElement).toBe(el);
    el.remove();
  });
});
