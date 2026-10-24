/** impl.router over a real `createBrowserRouter` in jsdom, so history length, state and popstate are the browser's own. */
import "./fetch-globals";
import { act, render, screen } from "@testing-library/react";
import type { ReactElement } from "react";
import { createBrowserRouter, RouterProvider } from "react-router";
import * as routerModule from "../impl.router";
import { routerImpl } from "../impl.router";
import { navContract } from "./nav-contract";

type BrowserRouter = ReturnType<typeof createBrowserRouter>;

let routers: BrowserRouter[] = [];
let current: BrowserRouter;

function mount(ui: ReactElement, url: string) {
  window.history.pushState(null, "", url);
  current = createBrowserRouter([{ path: "*", element: ui }]);
  routers.push(current);
  render(<RouterProvider router={current} />);
}

const settle = () => act(async () => void (await new Promise((resolve) => setTimeout(resolve, 20))));

afterEach(() => {
  routers.forEach((router) => router.dispose());
  routers = [];
});

navContract({
  name: "impl.router",
  impl: routerImpl,
  mount,
  keptScroll: () => current.state.preventScrollReset === true,
  redirectedTo: (to) => `${window.location.pathname}${window.location.search}` === to,
  observes: { linkClick: true, replaceSearchHooks: true, scrollOption: true },
});

describe("impl.router specifics", () => {
  it("does not leave an entry for the redirecting URL: Redirect replaces", async () => {
    const before = window.history.length;
    mount(<routerImpl.Redirect to="/settings" />, "/start");
    await settle();
    expect(window.location.pathname).toBe("/settings");
    expect(window.history.length).toBe(before + 1); // mount's own entry; the redirect added none
  });

  it("keeps the ssqTray marker through useReplaceSearch even though RR rewrites history.state", async () => {
    const seen: string[] = [];
    function Probe() {
      const replaceSearch = routerModule.useReplaceSearch();
      const params = routerModule.useSearchParams();
      seen.push(params.toString());
      return (
        // analytics-exempt
        <button onClick={() => replaceSearch("?tab=b")}>go</button>
      );
    }
    mount(<Probe />, "/unfinished?tab=a");
    window.history.replaceState({ ...window.history.state, ssqTray: true }, "");
    act(() => screen.getByText("go").click());
    await settle();
    expect(seen[seen.length - 1]).toBe("tab=b");
    expect(window.history.state).toEqual(expect.objectContaining({ ssqTray: true }));
  });

  it("an RR push on top of a tray entry and back keeps the marker on the earlier entry", async () => {
    mount(<routerImpl.Link href="/next" data-testid="go">go</routerImpl.Link>, "/start");
    routerImpl.pushTrayEntry();
    const marked = window.history.state;
    expect(marked).toEqual(expect.objectContaining({ ssqTray: true }));
    act(() => screen.getByTestId("go").click());
    await settle();
    expect(window.location.pathname).toBe("/next");
    act(() => window.history.back());
    await act(async () => void (await new Promise((resolve) => setTimeout(resolve, 20))));
    expect(window.history.state).toEqual(expect.objectContaining({ ssqTray: true }));
  });

  it("Back from a tray entry stays on the route: RR neither navigates nor drops the marker", async () => {
    let renders = 0;
    function Probe() {
      renders += 1;
      return <output data-testid="path">{routerModule.usePathname()}</output>;
    }
    mount(<Probe />, "/start");
    const lengthBefore = window.history.length;
    routerImpl.pushTrayEntry();
    const rendersAfterPush = renders;
    expect(window.history.length).toBe(lengthBefore + 1);
    act(() => window.history.back());
    await act(async () => void (await new Promise((resolve) => setTimeout(resolve, 20))));
    expect(screen.getByTestId("path").textContent).toBe("/start");
    expect(window.history.state).not.toEqual(expect.objectContaining({ ssqTray: true }));
    expect(window.history.length).toBe(lengthBefore + 1);
    expect(renders - rendersAfterPush).toBeLessThanOrEqual(1);
    act(() => window.history.forward());
    await act(async () => void (await new Promise((resolve) => setTimeout(resolve, 20))));
    expect(window.history.state).toEqual(expect.objectContaining({ ssqTray: true }));
  });

  it("usePathname normalises a trailing slash RR keeps in location.pathname", async () => {
    function Path() {
      return <output data-testid="path">{routerModule.usePathname()}</output>;
    }
    mount(<Path />, "/review-queue/");
    expect(window.location.pathname).toBe("/review-queue/");
    expect(screen.getByTestId("path").textContent).toBe("/review-queue");
  });
});
