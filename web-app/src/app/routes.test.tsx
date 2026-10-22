import "@/lib/nav/__tests__/fetch-globals";
import * as fs from "fs";
import * as path from "path";
import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { FeatureFlagsProvider } from "@/lib/contexts/FeatureFlagsContext";
import { AppRouterProvider, createAppRouter } from "@/lib/nav/entry";
import { ROUTE_PATHS } from "@/lib/nav/paths";
import { NotFound } from "./NotFound";
import { RoutePending } from "./RoutePending";
import { NOT_FOUND_META, ROUTE_META, titleForPath } from "./route-meta";
import { appRoutes, routeTable } from "./routes";

// The real router impl, so FeatureGuard's replace and NotFound's back go through React Router.
jest.mock("@/lib/nav", () => ({
  ...jest.requireActual("@/lib/nav/impl.router"),
  ...jest.requireActual("@/lib/nav/search-params"),
}));
const mockGetFeatureFlags = jest.fn();
jest.mock("@/lib/api/transport", () => ({ getConnectTransport: () => ({}) }));
jest.mock("@connectrpc/connect", () => ({ createClient: () => ({ getFeatureFlags: mockGetFeatureFlags }) }));

// Plain strings: the global style mock hands out functions, which React rejects as a className.
jest.mock("./NotFound.css", () => ({ actions: "a", container: "c", goBack: "g", homeLink: "h", message: "m", pathText: "p", title: "t" }));
jest.mock("./RoutePending.css", () => ({ container: "c" }));
jest.mock("@/components/ui/Skeleton.css", () => ({ skeleton: "s", rectangular: "r", circular: "ci", text: "t" }));

jest.mock("./page", () => ({ __esModule: true, default: () => <h1>Sessions page</h1> }));
jest.mock("./settings/page", () => ({ __esModule: true, default: () => <h1>Settings page</h1> }));
jest.mock("./settings/jules/page", () => ({ __esModule: true, default: () => <h1>Jules page</h1> }));
jest.mock("./backlog/board/page", () => ({ __esModule: true, default: () => <h1>Board page</h1> }));
jest.mock("./changelog/page", () => ({ __esModule: true, default: () => <h1>Changelog page</h1> }));

const APP_DIR = __dirname;

function pageDirectories(dir: string = APP_DIR, found: string[] = []): string[] {
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    if (entry.isDirectory()) pageDirectories(path.join(dir, entry.name), found);
    else if (entry.name === "page.tsx") found.push("/" + path.relative(APP_DIR, dir).split(path.sep).join("/"));
  }
  return found.map((route) => (route === "/" || route === "" ? "/" : route));
}

function Shell({ children }: { children: ReactNode }) {
  return (
    <FeatureFlagsProvider>
      <main id="main-content">{children}</main>
    </FeatureFlagsProvider>
  );
}

function setup(url: string) {
  const router = createAppRouter(
    {
      routes: appRoutes,
      shell: Shell,
      pending: <RoutePending />,
      errorElement: () => <p>failed</p>,
      notFound: <NotFound />,
      titleFor: titleForPath,
    },
    { initialEntries: [url] },
  );
  render(<AppRouterProvider router={router} />);
  return router;
}

afterEach(() => jest.restoreAllMocks());

beforeEach(() => {
  window.scrollTo = jest.fn();
  mockGetFeatureFlags.mockReset();
  mockGetFeatureFlags.mockResolvedValue({ flags: [] });
});

describe("route table", () => {
  it("has the same key set as RoutePath, RouteMeta and the page.tsx directories", () => {
    const pages = pageDirectories().sort();
    expect(pages.length).toBeGreaterThan(40);
    expect(Object.keys(routeTable).sort()).toEqual(pages);
    expect(Object.keys(ROUTE_META).sort()).toEqual(pages);
    expect([...ROUTE_PATHS].sort()).toEqual(pages);
    expect(appRoutes.map((route) => route.path).sort()).toEqual(pages);
  });

  it("wraps exactly the RouteMeta-guarded paths and stores no title", () => {
    const guarded = Object.entries(ROUTE_META).filter(([, meta]) => meta.guard).map(([route]) => route).sort();
    expect(guarded).toEqual(["/backlog", "/backlog/board", "/changelog", "/settings/backlog-sources"]);
    const wrapped = appRoutes.filter((route) => "wrap" in route && route.wrap).map((route) => route.path).sort();
    expect(wrapped).toEqual(guarded);
    appRoutes.forEach((route) => expect(route).not.toHaveProperty("title"));
  });

  it("sets the RouteMeta title and loads the page chunk for /settings/jules", async () => {
    setup("/settings/jules");
    expect(await screen.findByText("Jules page")).toBeTruthy();
    expect(document.title).toBe("Jules - Settings - Stapler Squad");
  });
});

describe("redirects", () => {
  it("/config drops the query and lands on the config tab of /settings", async () => {
    const router = setup("/config?x=1");
    expect(await screen.findByText("Settings page")).toBeTruthy();
    expect(`${router.state.location.pathname}${router.state.location.search}`).toBe("/settings?tab=config-files");
  });

  it("/settings/defaults becomes /settings", async () => {
    const router = setup("/settings/defaults");
    expect(await screen.findByText("Settings page")).toBeTruthy();
    expect(router.state.location.pathname).toBe("/settings");
  });
});

describe("feature guards", () => {
  it("renders nothing while flags load, then replaces to / when the flag is off", async () => {
    let resolveFlags: (value: unknown) => void = () => undefined;
    mockGetFeatureFlags.mockReturnValue(new Promise((resolve) => (resolveFlags = resolve)));
    const router = setup("/");
    await screen.findByText("Sessions page");
    await act(async () => void (await router.navigate("/backlog/board")));
    expect(screen.queryByText("Board page")).toBeNull();
    expect(router.state.location.pathname).toBe("/backlog/board");

    await act(async () => resolveFlags({ flags: [{ name: "backlog", enabled: false, scopes: [] }] }));
    await waitFor(() => expect(router.state.location.pathname).toBe("/"));
    expect(screen.queryByText("Board page")).toBeNull();
    // The guarded entry was replaced, so Back is not trapped on it.
    await act(async () => void (await router.navigate(-1)));
    expect(router.state.location.pathname).not.toBe("/backlog/board");
  });

  it("renders the page when its flag is on", async () => {
    mockGetFeatureFlags.mockResolvedValue({ flags: [{ name: "changelog", enabled: true, scopes: [] }] });
    setup("/changelog");
    expect(await screen.findByText("Changelog page")).toBeTruthy();
  });
});

describe("not found", () => {
  it("shows the view inside the shell with the unknown path, a home link and Go back, and keeps the URL", async () => {
    jest.spyOn(window.history, "length", "get").mockReturnValue(3);
    const router = setup("/nonexistent");
    expect(await screen.findByRole("heading", { level: 1, name: "Page not found" })).toBeTruthy();
    expect(screen.getByText("/nonexistent")).toBeTruthy();
    expect(screen.getByRole("link", { name: "Go to Sessions" }).getAttribute("href")).toBe("/");
    expect(screen.getByRole("button", { name: "Go back" })).toBeTruthy();
    expect(document.getElementById("main-content")?.contains(screen.getByRole("heading", { level: 1 }))).toBe(true);
    expect(router.state.location.pathname).toBe("/nonexistent");
    expect(document.title).toBe(NOT_FOUND_META.title);
  });

  it("hides Go back when there is nothing to go back to", async () => {
    jest.spyOn(window.history, "length", "get").mockReturnValue(1);
    setup("/nonexistent");
    await screen.findByRole("heading", { level: 1, name: "Page not found" });
    expect(screen.queryByRole("button", { name: "Go back" })).toBeNull();
    expect(screen.getByRole("link", { name: "Go to Sessions" })).toBeTruthy();
  });

  it("Go back and the home link navigate", async () => {
    jest.spyOn(window.history, "length", "get").mockReturnValue(3);
    const router = setup("/settings");
    await screen.findByText("Settings page");
    await act(async () => void (await router.navigate("/nonexistent")));
    fireEvent.click(await screen.findByRole("button", { name: "Go back" }));
    await waitFor(() => expect(router.state.location.pathname).toBe("/settings"));
    await act(async () => void (await router.navigate("/nonexistent")));
    fireEvent.click(await screen.findByRole("link", { name: "Go to Sessions" }));
    expect(await screen.findByText("Sessions page")).toBeTruthy();
  });
});

describe("RoutePending", () => {
  it("contains no live region, so the announcer is the only thing a screen reader hears", () => {
    const { container } = render(<RoutePending />);
    expect(container.querySelector("[role=status], [role=alert], [aria-live]")).toBeNull();
  });

  it("the skeleton primitive only animates when motion is allowed", () => {
    const source = fs.readFileSync(path.join(APP_DIR, "../components/ui/Skeleton.css.ts"), "utf8");
    const animationAt = source.indexOf("animation:");
    expect(animationAt).toBeGreaterThan(-1);
    expect(source.lastIndexOf("prefers-reduced-motion: no-preference", animationAt)).toBeGreaterThan(-1);
    expect(source.slice(0, animationAt).split("animation:").length).toBe(1);
  });
});
