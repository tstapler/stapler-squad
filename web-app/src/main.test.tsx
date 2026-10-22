import "@/lib/nav/__tests__/fetch-globals";
import { act, screen, waitFor } from "@testing-library/react";
import { useEffect } from "react";
import type { ReactNode } from "react";
import { mountApp } from "./main";

// The real router impl: Providers' consumers read the pathname through it.
jest.mock("@/lib/nav", () => ({
  ...jest.requireActual("@/lib/nav/impl.router"),
  ...jest.requireActual("@/lib/nav/search-params"),
}));
jest.mock("@/components/providers/ViewportProvider", () => ({ ViewportProvider: ({ children }: { children: ReactNode }) => <>{children}</> }));
jest.mock("@/components/ui/ErrorBoundary", () => ({ ErrorBoundary: ({ children }: { children: ReactNode }) => <>{children}</> }));
jest.mock("@/lib/contexts/AuthContext", () => ({ AuthProvider: ({ children }: { children: ReactNode }) => <>{children}</> }));
jest.mock("@/app/layout.css", () => ({ mainContent: "main-content" }));
jest.mock("@/components/layout/CockpitShell", () => ({ CockpitShell: ({ children }: { children: ReactNode }) => <div data-testid="cockpit">{children}</div> }));
jest.mock("@/components/system/TmuxVersionMismatchBanner", () => ({ TmuxVersionMismatchBanner: () => null }));
jest.mock("@/components/system/ForkPressureStatusBanner", () => ({ ForkPressureStatusBanner: () => null }));
jest.mock("@/components/ui/TrayErrorBoundary", () => ({ TrayErrorBoundary: ({ children }: { children: ReactNode }) => <>{children}</> }));
jest.mock("@/components/ui/NotificationPanel", () => ({ NotificationPanel: () => <aside data-testid="notification-panel" /> }));
jest.mock("@/app/NotFound.css", () => ({ actions: "a", container: "c", goBack: "g", homeLink: "h", message: "m", pathText: "p", title: "t" }));
jest.mock("@/app/RoutePending.css", () => ({ container: "c" }));
jest.mock("@/components/ui/Skeleton.css", () => ({ skeleton: "s", rectangular: "r", circular: "ci", text: "t" }));
jest.mock("@/components/ui/ErrorState.css", () => new Proxy({}, { get: (_, key) => String(key) }));

const mockProviderMounts = jest.fn();
const mockPageEffects = jest.fn();
jest.mock("@/app/Providers", () => {
  const { usePathname } = jest.requireMock("@/lib/nav");
  function PathProbe() {
    return <output data-testid="provider-path">{usePathname()}</output>;
  }
  function Providers({ children }: { children: ReactNode }) {
    useEffect(() => {
      mockProviderMounts();
    }, []);
    return (
      <>
        <PathProbe />
        {children}
      </>
    );
  }
  return { Providers };
});

jest.mock("@/app/page", () => ({
  __esModule: true,
  default: function Home() {
    useEffect(() => {
      mockPageEffects();
    }, []);
    return <h1>Sessions</h1>;
  },
}));
jest.mock("@/app/history/page", () => ({ __esModule: true, default: () => <h1>History</h1> }));
jest.mock("@/app/unfinished/page", () => ({ __esModule: true, default: () => <h1>Unfinished</h1> }));
jest.mock("@/app/errors/page", () => ({
  __esModule: true,
  default: () => {
    throw new Error("page exploded");
  },
}));

let mounted: ReturnType<typeof mountApp> | undefined;

function start(url: string) {
  const container = document.body.appendChild(document.createElement("div"));
  act(() => {
    mounted = mountApp(container, { initialEntries: [url] });
  });
  return mounted as NonNullable<typeof mounted>;
}

beforeEach(() => {
  window.scrollTo = jest.fn();
  jest.spyOn(console, "error").mockImplementation(() => undefined);
  mockProviderMounts.mockClear();
  mockPageEffects.mockClear();
});

afterEach(() => {
  act(() => mounted?.unmount());
  mounted = undefined;
  document.body.innerHTML = "";
  jest.restoreAllMocks();
});

const go = (app: ReturnType<typeof mountApp>, to: string) => act(async () => void (await app.router.navigate(to)));

describe("main entry", () => {
  it("keeps providers mounted across navigation while pathname consumers inside the router follow it", async () => {
    const app = start("/");
    await screen.findByText("Sessions");
    await go(app, "/history");
    await screen.findByText("History");
    await go(app, "/");
    await screen.findByText("Sessions");
    expect(mockProviderMounts).toHaveBeenCalledTimes(2); // StrictMode runs a mount effect twice; navigation adds none
    await go(app, "/history");
    expect(screen.getByTestId("provider-path").textContent).toBe("/history");
  });

  it("runs a page's mount effect twice under StrictMode, as reactStrictMode does today", async () => {
    start("/");
    await screen.findByText("Sessions");
    expect(mockPageEffects).toHaveBeenCalledTimes(2);
  });

  it("renders the skip link and focuses the layout main after navigation", async () => {
    const app = start("/");
    await screen.findByText("Sessions");
    expect(screen.getByText("Skip to main content").getAttribute("href")).toBe("#main-content");
    await go(app, "/history");
    await screen.findByText("History");
    expect(document.activeElement).toBe(document.getElementById("main-content"));
    expect(document.getElementById("main-content")?.getAttribute("tabindex")).toBe("-1");
  });

  it("keeps the shell when a page throws, with Retry inside main", async () => {
    const app = start("/");
    await screen.findByText("Sessions");
    await go(app, "/errors");
    expect(await screen.findByRole("button", { name: "Retry" })).toBeTruthy();
    expect(screen.getByText("Skip to main content")).toBeTruthy();
    expect(screen.getByTestId("notification-panel")).toBeTruthy();
    expect(document.getElementById("main-content")?.contains(screen.getByRole("button", { name: "Retry" }))).toBe(true);
    expect(mockProviderMounts).toHaveBeenCalledTimes(2);
  });

  it("sets the title from the route table, keeps it for search-only changes, and ships one <title>", async () => {
    const app = start("/");
    await screen.findByText("Sessions");
    await go(app, "/history");
    await waitFor(() => expect(document.title).toBe("History - Stapler Squad"));
    await go(app, "/unfinished");
    await go(app, "/unfinished?tab=a");
    expect(document.title).toBe("Up Next - Stapler Squad");
    expect(document.querySelectorAll("title")).toHaveLength(1);
  });
});
