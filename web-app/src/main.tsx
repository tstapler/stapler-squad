// Entry of the Bun build (WEB_BUILD=bun). Mirrors Next's persistent root layout: RootShell sits inside the
// router (its hooks need it) and outlives every navigation. The html theme class and FOUC script that
// layout.tsx emits are injected into index.html by build/html.ts, not here.
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import "./app/fonts.css";
import "./app/globals.css";
import "@/styles/globalEffects.css";
import { NotFound } from "@/app/NotFound";
import { RootShell } from "@/app/RootShell";
import { RouteError } from "@/app/RouteError";
import { RoutePending } from "@/app/RoutePending";
import { titleForPath } from "@/app/route-meta";
import { appRoutes } from "@/app/routes";
import { AppRouterProvider, createAppRouter } from "@/lib/nav/entry";

export function mountApp(container: HTMLElement, options?: { initialEntries?: string[] }) {
  const router = createAppRouter(
    {
      routes: appRoutes,
      shell: RootShell,
      pending: <RoutePending />,
      errorElement: RouteError,
      notFound: <NotFound />,
      titleFor: titleForPath,
    },
    options,
  );
  const root = createRoot(container);
  root.render(
    <StrictMode>
      <AppRouterProvider router={router} />
    </StrictMode>,
  );
  return { router, unmount: () => root.unmount() };
}

const container = typeof document === "undefined" ? null : document.getElementById("root");
if (container) mountApp(container);
