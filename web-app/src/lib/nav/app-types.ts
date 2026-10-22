// App-level shapes for the router entry; no router import, so `src/app/routes.tsx` can use them.
import type { ComponentType, ReactNode, RefObject } from "react";

export type MainRef = RefObject<HTMLElement | null>;

export interface ShellProps {
  children: ReactNode;
  /** Attach to the layout `<main>`: it receives focus and `aria-busy` while a route chunk is pending. */
  mainRef: MainRef;
}

export interface PageRoute {
  path: string;
  load: () => Promise<{ default: ComponentType }>;
  /** Wraps the page, e.g. a feature-flag guard. */
  wrap?: ComponentType<{ children: ReactNode }>;
}

export interface RedirectRoute {
  path: string;
  redirect: string;
  keepQuery: boolean;
}

export type AppRoute = PageRoute | RedirectRoute;

export interface AppRouterSpec {
  routes: readonly AppRoute[];
  /** Persistent chrome and providers; stays mounted across navigations and route failures. */
  shell: ComponentType<ShellProps>;
  /** Skeleton shown inside the shell while a route chunk loads; the entry delays it so fast loads never flash. */
  pending: ReactNode;
  errorElement: (props: { error: unknown; retry: () => void }) => ReactNode;
  notFound: ReactNode;
  titleFor: (pathname: string) => string;
}
