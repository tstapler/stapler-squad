"use client";

import { createContext, forwardRef, useContext, useEffect, useMemo, useSyncExternalStore } from "react";
import type { MouseEvent, ReactNode } from "react";
import { pushTrayEntryInHistory, replaceHashInHistory, replaceSearchInHistory } from "./history";
import type { NavImpl, NavLinkProps, NavRouter, NavSearchParams, RedirectProps } from "./impl.types";
import { normalizePathname } from "./normalize";

/** In-memory navigation shared by Jest and Storybook; the single fake, there is no second one. */
export interface MemoryNavStore {
  router: NavRouter;
  getLocation(): string;
  subscribe(listener: () => void): () => void;
}

export function createMemoryNav(initialEntries: string[] = ["/"], { frozen = false } = {}): MemoryNavStore {
  let entries = [...initialEntries];
  let index = entries.length - 1;
  const listeners = new Set<() => void>();
  const move = (apply: () => void) => {
    if (frozen) return;
    apply();
    listeners.forEach((listener) => listener());
  };
  const router: NavRouter = {
    push: (href) => move(() => { entries = [...entries.slice(0, index + 1), href]; index = entries.length - 1; }),
    replace: (href) => move(() => { entries = [...entries.slice(0, index), href]; index = entries.length - 1; }),
    back: () => move(() => { index = Math.max(0, index - 1); }),
    forward: () => move(() => { index = Math.min(entries.length - 1, index + 1); }),
  };
  return {
    router,
    getLocation: () => entries[index],
    subscribe: (listener) => { listeners.add(listener); return () => listeners.delete(listener); },
  };
}

// Without a provider the hooks see `/` with no query and a router that goes nowhere.
const detachedStore = createMemoryNav(["/"], { frozen: true });
const NavContext = createContext<MemoryNavStore>(detachedStore);

export function MemoryNavProvider({ initialEntries, children }: { initialEntries?: string[]; children: ReactNode }) {
  const store = useMemo(() => createMemoryNav(initialEntries), [initialEntries]);
  return <NavContext.Provider value={store}>{children}</NavContext.Provider>;
}

function useLocation(): string {
  const store = useContext(NavContext);
  return useSyncExternalStore(store.subscribe, store.getLocation, store.getLocation);
}

const splitLocation = (location: string) => {
  const [withoutHash] = location.split("#");
  const queryAt = withoutHash.indexOf("?");
  return queryAt === -1
    ? { pathname: withoutHash, search: "" }
    : { pathname: withoutHash.slice(0, queryAt), search: withoutHash.slice(queryAt) };
};

export function useRouter(): NavRouter {
  return useContext(NavContext).router;
}

export function useSearchParams(): NavSearchParams {
  const { search } = splitLocation(useLocation());
  return useMemo(() => new URLSearchParams(search), [search]);
}

export function usePathname(): string {
  return normalizePathname(splitLocation(useLocation()).pathname);
}

export const Link = forwardRef<HTMLAnchorElement, NavLinkProps>(function Link(
  { href, replace, resetScroll, onClick, target, ...rest },
  ref,
) {
  const router = useRouter();
  const handleClick = (event: MouseEvent<HTMLAnchorElement>) => {
    onClick?.(event);
    const plainLeftClick = event.button === 0 && !(event.metaKey || event.ctrlKey || event.shiftKey || event.altKey);
    if (event.defaultPrevented || !plainLeftClick || (target && target !== "_self")) return;
    event.preventDefault();
    (replace ? router.replace : router.push)(href, { resetScroll });
  };
  return <a ref={ref} href={href} target={target} onClick={handleClick} {...rest} />;
});

export function Redirect({ to }: RedirectProps): null {
  const router = useRouter();
  useEffect(() => router.replace(to), [router, to]);
  return null;
}

export function useReplaceSearch(): (search: string) => void {
  const router = useRouter();
  const pathname = usePathname();
  return useMemo(
    () => (search) => {
      replaceSearchInHistory(search);
      router.replace(`${pathname}${search === "" || search.startsWith("?") ? search : `?${search}`}`);
    },
    [router, pathname],
  );
}

export const replaceHash = replaceHashInHistory;
export const pushTrayEntry = pushTrayEntryInHistory;

export const memoryImpl = {
  useRouter,
  useSearchParams,
  usePathname,
  Link,
  Redirect,
  useReplaceSearch,
  replaceHash,
  pushTrayEntry,
} satisfies NavImpl;
