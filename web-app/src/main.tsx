// Entry of the Bun build; dead code until PR 5a selects it. Mirrors Next's persistent root layout: RootShell sits
// inside the router (its hooks need it) and outlives every navigation. The html theme class, FOUC script and
// fonts that layout.tsx emits belong to index.html and the build, not to this file.
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
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
