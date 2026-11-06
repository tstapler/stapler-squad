/** createAppRouter over createMemoryRouter: shell persistence, failures, pending UI, title, announcer, focus. */
import "./fetch-globals";
import { act, fireEvent, render, screen } from "@testing-library/react";
import { useEffect, useRef } from "react";
import type { ReactNode } from "react";
import type { AppRoute, ShellProps } from "../app-types";
import { createAppRouter, AppRouterProvider } from "../entry";
import { Redirect } from "../impl.router";
import { page } from "@/lib/lazy/stale-chunk";

jest.mock("@/lib/lazy/stale-chunk", () => ({
  ...jest.requireActual("@/lib/lazy/stale-chunk"),
  // Skip the reload-once recovery: tests drive the "second failure" path directly.
  recoverFromChunkError: (error: unknown) => Promise.reject(error),
  page: { reload: jest.fn() },
}));

const titles: Record<string, string> = {
  "/": "Stapler Squad Sessions",
  "/history": "History - Stapler Squad",
  "/settings": "Settings - Stapler Squad",
  "/unfinished": "Up Next - Stapler Squad",
  "/field": "Field - Stapler Squad",
};
const titleFor = (pathname: string) => titles[pathname] ?? "Page not found - Stapler Squad";

let shellMounts = 0;

function Shell({ children, mainRef }: ShellProps) {
  useEffect(() => {
    shellMounts += 1;
  }, []);
  return (
    <div data-testid="shell">
      <nav>
        <a href="#main-content" className="skip-link" data-testid="skip">Skip</a>
        <button data-testid="nav-link" type="button">History nav</button>
      </nav>
      <main id="main-content" ref={mainRef} tabIndex={-1} data-testid="layout-main">
        {children}
      </main>
    </div>
  );
}

const named = (name: string) => ({ default: () => <h1>{name}</h1> });

function Field() {
  const ref = useRef<HTMLInputElement>(null);
  useEffect(() => ref.current?.focus(), []);
  return <input ref={ref} data-testid="field" />;
}

let release: (() => void) | undefined;
const routes: AppRoute[] = [
  { path: "/", load: async () => named("Home") },
  { path: "/history", load: async () => named("History") },
  { path: "/settings", load: async () => named("Settings") },
  { path: "/unfinished", load: async () => named("Unfinished") },
  { path: "/field", load: async () => ({ default: Field }) },
  { path: "/via", load: async () => ({ default: () => <Redirect to="/settings" /> }) },
  { path: "/config", redirect: "/settings?tab=config-files", keepQuery: false },
  { path: "/legacy", redirect: "/unfinished", keepQuery: true },
  { path: "/boom", load: async () => Promise.reject(new Error("render failed")) },
  { path: "/stale", load: async () => Promise.reject(new TypeError("Failed to fetch dynamically imported module: /x.js")) },
  { path: "/slow", load: () => new Promise((resolve) => (release = () => resolve(named("Slow")))) },
  { path: "/guarded", load: async () => named("Guarded"), wrap: ({ children }: { children: ReactNode }) => <section data-testid="guard">{children}</section> },
];

function setup(url = "/") {
  const router = createAppRouter(
    {
      routes,
      shell: Shell,
      pending: <p data-testid="skeleton">loading</p>,
      errorElement: ({ error, retry }) => {
        const retryButton = (
          // analytics-exempt
          <button onClick={retry}>Retry</button>
        );
        return (
          <div role="alert">
            <span>{error instanceof Error ? error.message : "unknown"}</span>
            {retryButton}
          </div>
        );
      },
      notFound: <h1>Page not found</h1>,
      titleFor,
    },
    { initialEntries: [url] },
  );
  render(<AppRouterProvider router={router} />);
  const go = (to: string | number) => act(async () => void (await router.navigate(to as string)));
  return { router, go };
}

// Failing-page tests make React and RR log the error they were asked to handle; jsdom has no scrollTo.
beforeEach(() => {
  jest.spyOn(console, "error").mockImplementation(() => undefined);
  window.scrollTo = jest.fn();
  shellMounts = 0;
  release = undefined;
  jest.mocked(page.reload).mockClear();
});

const announcer = () => screen.getByTestId("route-announcer");

describe("createAppRouter", () => {
  it("keeps one shell instance across navigations", async () => {
    const { go } = setup();
    expect(await screen.findByText("Home")).toBeTruthy();
    await go("/history");
    expect(await screen.findByText("History")).toBeTruthy();
    await go("/");
    expect(await screen.findByText("Home")).toBeTruthy();
    expect(shellMounts).toBe(1);
  });

  it("wraps a page in its guard", async () => {
    setup("/guarded");
    expect((await screen.findByText("Guarded")).closest("[data-testid=guard]")).not.toBeNull();
  });

  it("renders unknown paths inside the shell, keeps the URL, and uses the not-found title", async () => {
    const { router } = setup("/nonexistent");
    expect(await screen.findByText("Page not found")).toBeTruthy();
    expect(screen.getByTestId("layout-main").contains(screen.getByText("Page not found"))).toBe(true);
    expect(router.state.location.pathname).toBe("/nonexistent");
    expect(document.title).toBe("Page not found - Stapler Squad");
  });

  it("matches /x/ like /x", async () => {
    setup("/history/");
    expect(await screen.findByText("History")).toBeTruthy();
    expect(document.title).toBe("History - Stapler Squad");
  });

  it("keeps the shell, skip link and nav when a page fails, with a Retry inside main", async () => {
    const { go } = setup();
    await screen.findByText("Home");
    await go("/boom");
    expect(await screen.findByText("render failed")).toBeTruthy();
    expect(screen.getByTestId("skip")).toBeTruthy();
    expect(screen.getByTestId("nav-link")).toBeTruthy();
    expect(screen.getByTestId("layout-main").contains(screen.getByRole("alert"))).toBe(true);
    expect(shellMounts).toBe(1);
  });

  it("Retry reloads the document for a chunk-load error, because a rejected import is memoised", async () => {
    const { go } = setup();
    await screen.findByText("Home");
    await go("/stale");
    fireEvent.click(await screen.findByText("Retry"));
    expect(page.reload).toHaveBeenCalledTimes(1);
  });

  it("Retry for other errors re-navigates instead of reloading", async () => {
    const { go, router } = setup();
    await screen.findByText("Home");
    await go("/boom");
    const before = router.state.location.key;
    await act(async () => void fireEvent.click(await screen.findByText("Retry")));
    expect(page.reload).not.toHaveBeenCalled();
    expect(router.state.location.pathname).toBe("/boom");
    expect(router.state.location.key).not.toBe(before);
  });
});

describe("redirect routes", () => {
  it("drops the query by default and lands once on the target", async () => {
    const { router } = setup("/config?x=1");
    expect(await screen.findByText("Settings")).toBeTruthy();
    expect(`${router.state.location.pathname}${router.state.location.search}`).toBe("/settings?tab=config-files");
  });

  it("keeps the query when asked", async () => {
    const { router } = setup("/legacy?tab=prs");
    expect(await screen.findByText("Unfinished")).toBeTruthy();
    expect(router.state.location.search).toBe("?tab=prs");
  });

  it("does not leave the redirecting URL in history", async () => {
    const { go, router } = setup();
    await screen.findByText("Home");
    await go("/config");
    await screen.findByText("Settings");
    await go(-1);
    expect(router.state.location.pathname).toBe("/");
  });
});

describe("pending UI", () => {
  beforeEach(() => jest.useFakeTimers());
  afterEach(() => jest.useRealTimers());

  it("shows the skeleton in main only after 150 ms, with aria-busy, and the shell stays", async () => {
    const { router } = setup();
    await act(async () => void (await Promise.resolve()));
    await act(async () => void (await router.navigate("/slow")));
    expect(screen.queryByTestId("skeleton")).toBeNull();
    await act(async () => void jest.advanceTimersByTime(140));
    expect(screen.queryByTestId("skeleton")).toBeNull();
    await act(async () => void jest.advanceTimersByTime(20));
    expect(screen.getByTestId("skeleton")).toBeTruthy();
    expect(screen.getByTestId("layout-main").getAttribute("aria-busy")).toBe("true");
    expect(screen.getByTestId("layout-main").contains(screen.getByTestId("skeleton"))).toBe(true);
    expect(screen.getByTestId("skeleton").closest("[role=status],[role=alert],[aria-live]")).toBeNull();

    await act(async () => void release?.());
    await act(async () => void jest.advanceTimersByTime(0));
    expect(screen.getByText("Slow")).toBeTruthy();
    expect(screen.queryByTestId("skeleton")).toBeNull();
    expect(screen.getByTestId("layout-main").hasAttribute("aria-busy")).toBe(false);
  });

  it("never flashes the skeleton for a load that settles inside 150 ms", async () => {
    const { router } = setup();
    await act(async () => void (await Promise.resolve()));
    await act(async () => void (await router.navigate("/history")));
    await act(async () => void jest.advanceTimersByTime(500));
    expect(screen.queryByTestId("skeleton")).toBeNull();
  });
});

describe("title", () => {
  it("follows the route on every client navigation and ignores search-only changes", async () => {
    const { go } = setup();
    await screen.findByText("Home");
    expect(document.title).toBe("Stapler Squad Sessions");
    await go("/history");
    expect(document.title).toBe("History - Stapler Squad");
    await go("/unfinished");
    await go("/unfinished?tab=a");
    expect(document.title).toBe("Up Next - Stapler Squad");
    await go("/unfinished?tab=b");
    expect(document.title).toBe("Up Next - Stapler Squad");
    expect(document.querySelectorAll("title")).toHaveLength(1);
  });
});

describe("RouteAnnouncer", () => {
  beforeEach(() => jest.useFakeTimers());
  afterEach(() => jest.useRealTimers());
  const flush = (ms: number) => act(async () => void jest.advanceTimersByTime(ms));

  it("is a polite status region that stays silent on first load", async () => {
    setup();
    await act(async () => void (await Promise.resolve()));
    expect(announcer().getAttribute("role")).toBe("status");
    expect(announcer().getAttribute("aria-live")).toBe("polite");
    await flush(2000);
    expect(announcer().textContent).toBe("");
  });

  it("announces the new title after a pathname change, and not for search-only changes", async () => {
    const { router } = setup();
    await act(async () => void (await Promise.resolve()));
    await act(async () => void (await router.navigate("/history")));
    await flush(150);
    expect(announcer().textContent).toBe("History - Stapler Squad");
    await flush(1500);
    await act(async () => void (await router.navigate("/history?tab=b")));
    await flush(150);
    expect(announcer().textContent).toBe("");
  });

  it("announces an identical title again by clearing first (set only in a later task)", async () => {
    const { router } = setup();
    await act(async () => void (await Promise.resolve()));
    const seen: string[] = [];
    new MutationObserver(() => seen.push(announcer().textContent ?? "")).observe(announcer(), { childList: true, characterData: true, subtree: true });
    await act(async () => void (await router.navigate("/history")));
    await flush(150);
    await act(async () => void (await router.navigate("/")));
    await act(async () => void (await router.navigate("/history")));
    expect(announcer().textContent).toBe("");
    await flush(150);
    expect(announcer().textContent).toBe("History - Stapler Squad");
    expect(seen.filter((text) => text === "History - Stapler Squad")).toHaveLength(2);
  });

  it("announces a redirect once, for the final location, and Back is announced like any navigation", async () => {
    const { router } = setup();
    await act(async () => void (await Promise.resolve()));
    const seen: string[] = [];
    new MutationObserver(() => seen.push(announcer().textContent ?? "")).observe(announcer(), { childList: true, characterData: true, subtree: true });
    await act(async () => void (await router.navigate("/config")));
    await flush(150);
    expect(seen.filter(Boolean)).toEqual(["Settings - Stapler Squad"]);
    await flush(1500);
    await act(async () => void (await router.navigate(-1)));
    await flush(150);
    expect(announcer().textContent).toBe("Stapler Squad Sessions");
  });

  it("announces a page-level <Redirect> once, for the final location", async () => {
    const { router } = setup();
    await act(async () => void (await Promise.resolve()));
    const seen: string[] = [];
    new MutationObserver(() => seen.push(announcer().textContent ?? "")).observe(announcer(), { childList: true, characterData: true, subtree: true });
    await act(async () => void (await router.navigate("/via")));
    await flush(150);
    expect(router.state.location.pathname).toBe("/settings");
    expect(seen.filter(Boolean)).toEqual(["Settings - Stapler Squad"]);
  });

  it("announces the not-found title", async () => {
    const { router } = setup();
    await act(async () => void (await Promise.resolve()));
    await act(async () => void (await router.navigate("/nope")));
    await flush(150);
    expect(announcer().textContent).toBe("Page not found - Stapler Squad");
  });
});

describe("focus on navigation", () => {
  it("moves focus to the layout main when the pathname changes, not on first load or search-only changes", async () => {
    const { go } = setup();
    await screen.findByText("Home");
    expect(document.activeElement).toBe(document.body);
    screen.getByTestId("nav-link").focus();
    await go("/unfinished");
    expect(document.activeElement).toBe(screen.getByTestId("layout-main"));
    screen.getByTestId("nav-link").focus();
    await go("/unfinished?tab=a");
    expect(document.activeElement).toBe(screen.getByTestId("nav-link"));
  });

  it("does not steal focus a page already took inside main", async () => {
    const { go } = setup();
    await screen.findByText("Home");
    await go("/field");
    expect(document.activeElement).toBe(await screen.findByTestId("field"));
  });

  it("leaves a hash link to the browser: no default prevented, no router navigation", async () => {
    const { router } = setup();
    await screen.findByText("Home");
    const key = router.state.location.key;
    expect(fireEvent.click(screen.getByTestId("skip"))).toBe(true);
    expect(router.state.location.key).toBe(key);
  });
});
