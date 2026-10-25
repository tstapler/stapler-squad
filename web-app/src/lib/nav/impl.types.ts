import type { AnchorHTMLAttributes, ComponentType, ForwardRefExoticComponent, RefAttributes } from "react";

/** Router-neutral navigation options; `resetScroll: false` keeps the scroll position (Next `scroll:false`, RR `preventScrollReset`). */
export interface NavigateOptions {
  resetScroll?: boolean;
}

export interface NavRouter {
  push(href: string, options?: NavigateOptions): void;
  replace(href: string, options?: NavigateOptions): void;
  back(): void;
  forward(): void;
}

/** Read-only view of the query string; mutate by navigating. */
export type NavSearchParams = Omit<URLSearchParams, "append" | "delete" | "set" | "sort">;

export interface NavLinkProps extends Omit<AnchorHTMLAttributes<HTMLAnchorElement>, "href"> {
  /** Accepts any string: call sites append query strings and ids, so this is not narrowed to `RoutePath`. */
  href: string;
  replace?: boolean;
  resetScroll?: boolean;
}

export interface RedirectProps {
  to: string;
}

/** The contract every `impl.*.tsx` satisfies; selected per build through the `@/lib/nav/impl` alias. */
export interface NavImpl {
  useRouter(): NavRouter;
  useSearchParams(): NavSearchParams;
  /** Trailing slash stripped except for `/`. */
  usePathname(): string;
  Link: ForwardRefExoticComponent<NavLinkProps & RefAttributes<HTMLAnchorElement>>;
  Redirect: ComponentType<RedirectProps>;
  /** Replaces the query string (`?a=b`), keeping foreign `history.state`. */
  useReplaceSearch(): (search: string) => void;
  /** Replaces the URL hash (`#x`, or `""` to clear), keeping foreign `history.state`. */
  replaceHash(hash: string): void;
  /** Pushes (or with `replace`, replaces) the mobile tray history entry marker. */
  pushTrayEntry(replace?: boolean): void;
  /** Pushes (or with `replace`, replaces) an entry whose state adds `[key]: value` to the current state; throws if History does. */
  pushHistoryMarker(key: string, value: unknown, replace?: boolean): void;
}
