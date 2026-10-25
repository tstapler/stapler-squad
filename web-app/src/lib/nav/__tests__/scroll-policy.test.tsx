/**
 * Scroll reset/restore for an app whose window never scrolls (gate S4): a shell scroller that persists across
 * routes, and a per-page scroller that is remounted. jsdom has no layout, so scrollTop is a plain settable number.
 */
import "./fetch-globals";
import { act, fireEvent, render, screen } from "@testing-library/react";
import type { AppRoute, ShellProps } from "../app-types";
import { AppRouterProvider, createAppRouter } from "../entry";

function Shell({ children, mainRef }: ShellProps) {
  return (
    <div id="shell-scroller" data-testid="shell-scroller">
      <main id="main-content" ref={mainRef} tabIndex={-1}>
        {children}
      </main>
    </div>
  );
}

const pageWithScroller = (name: string) => ({
  default: () => (
    <div data-testid={`page-scroller-${name}`}>
      <h1>{name}</h1>
    </div>
  ),
});

const routes: AppRoute[] = [
  { path: "/a", load: async () => pageWithScroller("a") },
  { path: "/b", load: async () => pageWithScroller("b") },
];

function setup() {
  const router = createAppRouter(
    { routes, shell: Shell, pending: null, errorElement: () => null, notFound: null, titleFor: () => "t" },
    { initialEntries: ["/a"] },
  );
  render(<AppRouterProvider router={router} />);
  return router;
}

const scrollTo = (element: HTMLElement, top: number) => {
  element.scrollTop = top;
  fireEvent.scroll(element);
};
const go = (router: ReturnType<typeof setup>, to: string | number, options?: { preventScrollReset?: boolean }) =>
  act(async () => void (await router.navigate(to as string, options)));

beforeEach(() => {
  window.scrollTo = jest.fn();
});

describe("ScrollPolicy", () => {
  it("scrolls a persistent scroller to the top on forward navigation to a new pathname", async () => {
    const router = setup();
    await screen.findByText("a");
    const shell = screen.getByTestId("shell-scroller");
    scrollTo(shell, 400);
    await go(router, "/b");
    expect(shell.scrollTop).toBe(0);
    expect(window.scrollTo).toHaveBeenCalledWith(0, 0);
  });

  it("restores the position of the entry on Back", async () => {
    const router = setup();
    await screen.findByText("a");
    const shell = screen.getByTestId("shell-scroller");
    scrollTo(shell, 400);
    await go(router, "/b");
    scrollTo(shell, 90);
    await go(router, -1);
    expect(shell.scrollTop).toBe(400);
    await go(router, 1);
    expect(shell.scrollTop).toBe(90);
  });

  it("restores a page-level scroller that was remounted", async () => {
    const router = setup();
    const first = await screen.findByTestId("page-scroller-a");
    scrollTo(first, 250);
    await go(router, "/b");
    await screen.findByTestId("page-scroller-b");
    await go(router, -1);
    const remounted = await screen.findByTestId("page-scroller-a");
    expect(remounted).not.toBe(first);
    expect(remounted.scrollTop).toBe(250);
  });

  it("leaves the position alone for a search-only change", async () => {
    const router = setup();
    await screen.findByText("a");
    const shell = screen.getByTestId("shell-scroller");
    scrollTo(shell, 400);
    await go(router, "/a?tab=b");
    expect(shell.scrollTop).toBe(400);
  });

  it("keeps the position when the navigation asks not to reset scroll", async () => {
    const router = setup();
    await screen.findByText("a");
    const shell = screen.getByTestId("shell-scroller");
    scrollTo(shell, 400);
    await go(router, "/b", { preventScrollReset: true });
    expect(shell.scrollTop).toBe(400);
  });

  it("retries a restore until the content is tall enough, and stops as soon as the user scrolls", async () => {
    jest.useFakeTimers();
    try {
      const router = setup();
      await act(async () => void (await Promise.resolve()));
      const shell = screen.getByTestId("shell-scroller");
      scrollTo(shell, 400);
      await go(router, "/b");
      // A scroller that cannot reach the target yet, as when a lazy page has not grown it.
      const attempts = jest.fn();
      Object.defineProperty(shell, "scrollTop", { configurable: true, get: () => 0, set: attempts });
      await go(router, -1);
      await act(async () => void jest.advanceTimersByTime(120));
      expect(attempts.mock.calls.length).toBeGreaterThan(1);
      expect(attempts).toHaveBeenLastCalledWith(400);

      fireEvent.wheel(window);
      const seen = attempts.mock.calls.length;
      await act(async () => void jest.advanceTimersByTime(1000));
      expect(attempts).toHaveBeenCalledTimes(seen);
    } finally {
      jest.useRealTimers();
    }
  });
});
