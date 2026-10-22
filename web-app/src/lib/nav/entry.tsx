// The one place that builds the React Router tree. Only src/main.tsx and tests may import it (.eslintrc.json),
// and index.ts does not re-export it, so the Next build never bundles react-router.
import {
  createContext,
  createRef,
  lazy,
  Suspense,
  useContext,
  useEffect,
  useState,
} from "react";
import type { ComponentType, ReactNode } from "react";
import {
  createBrowserRouter,
  createMemoryRouter,
  Outlet,
  redirect,
  RouterProvider,
  useLocation,
  useNavigate,
  useRouteError,
} from "react-router";
import type { RouteObject } from "react-router";
import { isChunkLoadError, markLoadSucceeded, page, recoverFromChunkError } from "@/lib/lazy/stale-chunk";
import type { AppRouterSpec, MainRef, PageRoute, RedirectRoute } from "./app-types";
import { usePathname } from "./impl.router";
import { PENDING_DELAY_MS } from "./policy";
import { RouteAnnouncer } from "./RouteAnnouncer";
import { ScrollPolicy } from "./ScrollPolicy";
import { useFocusOnNavigate } from "./useFocusOnNavigate";

export type AppRouter = ReturnType<typeof createBrowserRouter>;

const MainRefContext = createContext<MainRef | null>(null);
const PendingContext = createContext<ReactNode>(null);

function PendingFallback({ children }: { children: ReactNode }) {
  const mainRef = useContext(MainRefContext);
  const [visible, setVisible] = useState(false);
  useEffect(() => {
    const main = mainRef?.current;
    const timer = setTimeout(() => {
      setVisible(true);
      main?.setAttribute("aria-busy", "true");
    }, PENDING_DELAY_MS);
    return () => {
      clearTimeout(timer);
      main?.removeAttribute("aria-busy");
    };
  }, [mainRef]);
  return visible ? children : null;
}

function NavigationEffects() {
  const mainRef = useContext(MainRefContext);
  if (!mainRef) throw new Error("NavigationEffects renders inside the app layout");
  useFocusOnNavigate(mainRef);
  return (
    <>
      <RouteAnnouncer />
      <ScrollPolicy />
    </>
  );
}

function AppLayout({ spec, mainRef }: { spec: AppRouterSpec; mainRef: MainRef }) {
  const pathname = usePathname();
  const Shell = spec.shell;
  return (
    <MainRefContext.Provider value={mainRef}>
      {/* React 19 hoists <title> to <head>; it follows the route table on every client navigation. */}
      <title>{spec.titleFor(pathname)}</title>
      <Shell mainRef={mainRef}>
        <Outlet />
      </Shell>
      <NavigationEffects />
    </MainRefContext.Provider>
  );
}

function RouteFailure({ spec }: { spec: AppRouterSpec }) {
  const error = useRouteError();
  const navigate = useNavigate();
  const location = useLocation();
  const retry = () => {
    // A rejected dynamic import is memoised, so only a reload re-requests the chunk.
    if (isChunkLoadError(error)) page.reload();
    else void navigate(`${location.pathname}${location.search}${location.hash}`, { replace: true, preventScrollReset: true });
  };
  return <>{spec.errorElement({ error, retry })}</>;
}

function pageRoute(route: PageRoute): RouteObject {
  const Page = lazy(() =>
    route.load().then(
      (module) => {
        markLoadSucceeded();
        return module;
      },
      (error: unknown) => recoverFromChunkError<{ default: ComponentType }>(error),
    ),
  );
  return { path: route.path, element: <PageBoundary route={route} Page={Page} /> };
}

function PageBoundary({ route, Page }: { route: PageRoute; Page: ComponentType }) {
  const Wrap = route.wrap;
  const pending = useContext(PendingContext);
  // Keyed so each route gets a fresh boundary: a transition would otherwise keep the old page instead of showing the skeleton.
  return (
    <Suspense key={route.path} fallback={<PendingFallback>{pending}</PendingFallback>}>
      {Wrap ? (
        <Wrap>
          <Page />
        </Wrap>
      ) : (
        <Page />
      )}
    </Suspense>
  );
}

function redirectRoute(route: RedirectRoute): RouteObject {
  return {
    path: route.path,
    element: null,
    // A loader redirect never commits the source URL, so Back is not trapped on it.
    loader: ({ request }) => {
      const { search } = new URL(request.url);
      const joiner = route.redirect.includes("?") ? "&" : "?";
      throw redirect(route.keepQuery && search ? `${route.redirect}${joiner}${search.slice(1)}` : route.redirect);
    },
  };
}

/** Pathless layout (shell, no errorElement) over a pathless child that owns the errorElement, so a failing page keeps the shell. */
export function createAppRouter(spec: AppRouterSpec, options?: { initialEntries?: string[] }): AppRouter {
  const mainRef: MainRef = createRef<HTMLElement>();
  const pages = spec.routes.map((route) => ("load" in route ? pageRoute(route) : redirectRoute(route)));
  const routes: RouteObject[] = [
    {
      element: (
        <PendingContext.Provider value={spec.pending}>
          <AppLayout spec={spec} mainRef={mainRef} />
        </PendingContext.Provider>
      ),
      // Redirect loaders make the first load async; render nothing rather than warn about a missing fallback.
      hydrateFallbackElement: <></>,
      children: [
        {
          errorElement: <RouteFailure spec={spec} />,
          children: [...pages, { path: "*", element: spec.notFound }],
        },
      ],
    },
  ];
  return options?.initialEntries ? createMemoryRouter(routes, { initialEntries: options.initialEntries }) : createBrowserRouter(routes);
}

export function AppRouterProvider({ router }: { router: AppRouter }) {
  return <RouterProvider router={router} />;
}

export type { AppRoute, AppRouterSpec, MainRef, PageRoute, RedirectRoute, ShellProps } from "./app-types";
