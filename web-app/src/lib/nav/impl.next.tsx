"use client";

import NextLink from "next/link";
import {
  redirect,
  usePathname as useNextPathname,
  useRouter as useNextRouter,
  useSearchParams as useNextSearchParams,
} from "next/navigation";
import { forwardRef, useCallback, useMemo } from "react";
import { pushHistoryMarkerInHistory, pushTrayEntryInHistory, replaceHashInHistory, replaceSearchInHistory } from "./history";
import type { NavigateOptions, NavImpl, NavLinkProps, NavRouter, NavSearchParams, RedirectProps } from "./impl.types";
import { normalizePathname } from "./normalize";

const toNextOptions = (options?: NavigateOptions) => (options?.resetScroll === false ? { scroll: false } : undefined);

export function useRouter(): NavRouter {
  const router = useNextRouter();
  return useMemo(
    () => ({
      push: (href, options) => router.push(href, toNextOptions(options)),
      replace: (href, options) => router.replace(href, toNextOptions(options)),
      back: () => router.back(),
      forward: () => router.forward(),
    }),
    [router],
  );
}

export function useSearchParams(): NavSearchParams {
  return useNextSearchParams();
}

export function usePathname(): string {
  return normalizePathname(useNextPathname() ?? "/");
}

/** Prefetch stays off: it triggers CSS preload warnings (the reason the NavLink this replaces existed). */
export const Link = forwardRef<HTMLAnchorElement, NavLinkProps>(function Link({ resetScroll, ...props }, ref) {
  return <NextLink ref={ref} prefetch={false} scroll={resetScroll === false ? false : undefined} {...props} />;
});

/** Next's render-time `redirect()`: the behaviour the two redirect-only pages have today. */
export function Redirect({ to }: RedirectProps): null {
  redirect(to);
}

export function useReplaceSearch(): (search: string) => void {
  return useCallback((search) => replaceSearchInHistory(search), []);
}

export const replaceHash = replaceHashInHistory;
export const pushTrayEntry = pushTrayEntryInHistory;
export const pushHistoryMarker = pushHistoryMarkerInHistory;

export const nextImpl = {
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
