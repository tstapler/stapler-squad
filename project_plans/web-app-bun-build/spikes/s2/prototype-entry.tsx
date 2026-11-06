// Task 0.3a: prototype data-router entry shared by S2/S3/S4. Spike-only; nothing under web-app/src imports it.
// Routes come from the real ROUTE_META table (one lazy route per row); pages are stubs, the guard copies
// RequireFeatureFlag's logic (FeatureFlagsContext.tsx:179-200) with a controllable flag source.
// Behaviour knobs are read from window.__S4 so one bundle serves every measurement:
//   scroll: "window" | "main"   which element holds the tall content (default window)
//   restoration: boolean        render <ScrollRestoration/>
//   flags: Record<string, boolean>, flagDelayMs: number   feature-flag source for the three guard layouts
import { StrictMode, Suspense, useEffect, useLayoutEffect, useRef, useState, type ReactNode } from "react";
import { createRoot } from "react-dom/client";
import {
  createBrowserRouter,
  Link,
  Navigate,
  Outlet,
  RouterProvider,
  ScrollRestoration,
  useLocation,
  useNavigate,
  useSearchParams,
  type RouteObject,
} from "react-router";
import { ROUTE_META } from "../../../../web-app/src/app/route-meta";

type S4Config = {
  scroll?: "window" | "main";
  restoration?: boolean;
  flags?: Record<string, boolean>;
  flagDelayMs?: number;
};
declare global {
  interface Window {
    __S4?: S4Config;
    __s4log: { replaceCalls: number; announced: string[]; effectTitles: string[]; renders: number };
    __router: ReturnType<typeof createBrowserRouter>;
  }
}
const cfg = (): S4Config => window.__S4 ?? {};
window.__s4log = { replaceCalls: 0, announced: [], effectTitles: [], renders: 0 };

const normalize = (p: string) => (p.replace(/\/+$/, "") || "/");
const ROOT_TITLE = "Stapler Squad Sessions";
const titleFor = (pathname: string) => ROUTE_META[normalize(pathname)]?.title ?? ROOT_TITLE;

// Mirrors RequireFeatureFlag: null while the flag store loads, null + one router.replace("/") when the flag is off.
// The store is page-global and loads once per page load, like FeatureFlagsContext (not once per mount).
let flagsLoaded = false;
let flagsPromise: Promise<void> | null = null;
const loadFlags = () => (flagsPromise ??= new Promise<void>((resolve) => setTimeout(() => ((flagsLoaded = true), resolve()), cfg().flagDelayMs ?? 0)));

function Guard({ flag }: { flag: string }) {
  const navigate = useNavigate();
  const [loading, setLoading] = useState(!flagsLoaded);
  useEffect(() => {
    if (!flagsLoaded) void loadFlags().then(() => setLoading(false));
  }, []);
  const on = !!cfg().flags?.[flag];
  useEffect(() => {
    if (!loading && !on) {
      window.__s4log.replaceCalls++;
      navigate("/", { replace: true });
    }
  }, [loading, on, navigate]);
  if (loading || !on) return null;
  return <Outlet />;
}

function RouteAnnouncer() {
  const { pathname } = useLocation();
  const first = useRef(true);
  useEffect(() => {
    if (first.current) {
      first.current = false;
      return;
    }
    window.__s4log.announced.push(document.title);
  }, [pathname]);
  return <div id="announcer" aria-live="polite" role="status" />;
}

function Shell() {
  const { pathname } = useLocation();
  const title = titleFor(pathname);
  window.__s4log.renders++;
  useLayoutEffect(() => {
    window.__s4log.effectTitles.push(document.title);
  }, [pathname]);
  const mainScroll = cfg().scroll === "main";
  return (
    <>
      <title>{title}</title>
      <nav>
        <Link to="/" id="l-home">home</Link> <Link to="/history" id="l-history">history</Link>{" "}
        <Link to="/settings/" id="l-settings-slash">settings/</Link> <Link to="/history" replace id="l-history-replace">history (replace)</Link>{" "}
        <Link to="/unfinished?tab=a" id="l-unf">unfinished</Link> <Link to="/history" preventScrollReset id="l-history-keep">history keep</Link>
      </nav>
      <main
        id="main-content"
        style={{ display: "flex", flexDirection: "column", overflow: "hidden", height: mainScroll ? "70vh" : undefined }}
      >
        <Suspense fallback={<p id="pending">loading</p>}>
          <div id="scroller" style={mainScroll ? { overflowY: "auto", flex: 1 } : undefined}>
            <Outlet />
          </div>
        </Suspense>
      </main>
      <RouteAnnouncer />
      {cfg().restoration ? <ScrollRestoration /> : null}
    </>
  );
}

export function Page({ path }: { path: string }) {
  const [sp, setSp] = useSearchParams();
  return (
    <section data-route={path}>
      <h1>{path === "/settings" ? "Settings" : `Page ${path}`}</h1>
      <output id="tab">{sp.get("tab") ?? ""}</output>
      <button id="set-default" onClick={() => setSp({ tab: "b" })}>set</button>
      <button id="set-replace" onClick={() => setSp({ tab: "b" }, { replace: true })}>set replace</button>
      <output id="search">{sp.toString()}</output>
      <div style={{ height: 3000 }} id="tall">tall content</div>
    </section>
  );
}

const GUARDS: Record<string, string> = {};
for (const [path, meta] of Object.entries(ROUTE_META)) if (meta.guard) GUARDS[path] = meta.guard;

const leaf = (path: string): RouteObject => ({
  path,
  lazy: async () => ({ Component: () => <Page path={path} /> }),
});

function buildRoutes(): RouteObject[] {
  const plain: RouteObject[] = [];
  const guarded = new Map<string, RouteObject[]>();
  for (const path of Object.keys(ROUTE_META)) {
    const g = GUARDS[path];
    if (g) guarded.set(g, [...(guarded.get(g) ?? []), leaf(path)]);
    else plain.push(leaf(path));
  }
  const guardLayouts = [...guarded].map(([flag, children]) => ({ element: <Guard flag={flag} />, children }));
  return [
    {
      element: <Shell />,
      children: [...plain, ...guardLayouts, { path: "/__redir", element: <Navigate to="/history" /> }, { path: "/__redir-replace", element: <Navigate to="/history" replace /> }, { path: "*", element: <h1>Not found</h1> }],
    },
  ];
}

export function mount() {
  void loadFlags(); // like FeatureFlagsProvider: the flag store starts loading at app start, not at first guard mount
  const router = createBrowserRouter(buildRoutes());
  window.__router = router;
  createRoot(document.getElementById("root")!).render(
    <StrictMode>
      <RouterProvider router={router} />
    </StrictMode>,
  );
}
export const ROUTE_COUNT = Object.keys(ROUTE_META).length;
mount();
