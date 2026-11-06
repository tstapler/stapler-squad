import { forwardRef, useCallback, useMemo, useRef } from "react";
import { Link as RouterLink, Navigate, useLocation, useNavigate, useSearchParams as useRouterSearchParams } from "react-router";
import { pushHistoryMarkerInHistory, pushTrayEntryInHistory, replaceHashInHistory } from "./history";
import type { NavigateOptions, NavImpl, NavLinkProps, NavRouter, NavSearchParams, RedirectProps } from "./impl.types";
import { normalizePathname } from "./normalize";

// React Router's own keys in history.state; every other top-level key (the tray marker, ...) is foreign.
const ROUTER_STATE_KEYS = new Set(["usr", "key", "idx"]);

const keepsScroll = (options?: NavigateOptions) => (options?.resetScroll === false ? true : undefined);

export function useRouter(): NavRouter {
  const navigate = useNavigate();
  return useMemo(
    () => ({
      push: (href, options) => void navigate(href, { preventScrollReset: keepsScroll(options) }),
      // RR pushes by default, so replace is always explicit.
      replace: (href, options) => void navigate(href, { replace: true, preventScrollReset: keepsScroll(options) }),
      back: () => void navigate(-1),
      forward: () => void navigate(1),
    }),
    [navigate],
  );
}

export function useSearchParams(): NavSearchParams {
  const [params] = useRouterSearchParams();
  return params;
}

/** RR keeps the trailing slash of `/x/` in `pathname` even though it matches `/x`. */
export function usePathname(): string {
  return normalizePathname(useLocation().pathname);
}

export const Link = forwardRef<HTMLAnchorElement, NavLinkProps>(function Link({ href, replace, resetScroll, ...rest }, ref) {
  // Same-page hash links (the skip link) belong to the browser; RR would push a router entry for them.
  if (href.startsWith("#")) return <a ref={ref} href={href} {...rest} />;
  return <RouterLink ref={ref} to={href} replace={replace} preventScrollReset={keepsScroll({ resetScroll })} {...rest} />;
});

/** `<Navigate>` pushes by default, which would trap Back on the redirecting URL. */
export function Redirect({ to }: RedirectProps) {
  return <Navigate to={to} replace />;
}

function foreignState(): Record<string, unknown> {
  const state = (window.history.state ?? {}) as Record<string, unknown>;
  return Object.fromEntries(Object.entries(state).filter(([key]) => !ROUTER_STATE_KEYS.has(key)));
}

// An RR replace rewrites history.state to {usr,key,idx}, dropping markers such as ssqTray.
function restoreForeignState(foreign: Record<string, unknown>): void {
  if (Object.keys(foreign).length > 0) window.history.replaceState({ ...window.history.state, ...foreign }, "");
}

export function useReplaceSearch(): (search: string) => void {
  const navigate = useNavigate();
  const pathnameRef = useRef("");
  pathnameRef.current = useLocation().pathname;
  return useCallback(
    (search) => {
      const query = search === "" || search.startsWith("?") ? search : `?${search}`;
      const foreign = foreignState();
      const done = navigate(`${pathnameRef.current}${query}`, { replace: true, preventScrollReset: true });
      // A data router commits (and rewrites history.state) asynchronously, a memory router synchronously.
      if (done) void done.then(() => restoreForeignState(foreign));
      else restoreForeignState(foreign);
    },
    [navigate],
  );
}

export const replaceHash = replaceHashInHistory;
export const pushTrayEntry = pushTrayEntryInHistory;
export const pushHistoryMarker = pushHistoryMarkerInHistory;

export const routerImpl = {
  useRouter,
  useSearchParams,
  usePathname,
  Link,
  Redirect,
  useReplaceSearch,
  replaceHash,
  pushTrayEntry,
  pushHistoryMarker,
} satisfies NavImpl;
