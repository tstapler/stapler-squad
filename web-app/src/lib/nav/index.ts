// `@/lib/nav/impl` is aliased per build (next.config.ts, tsconfig paths, jest moduleNameMapper); everything outside this directory imports from here.
export {
  Link,
  pushHistoryMarker,
  pushTrayEntry,
  Redirect,
  replaceHash,
  useReplaceSearch,
  usePathname,
  useRouter,
  useSearchParams,
} from "@/lib/nav/impl";
export type { AppRoute, AppRouterSpec, MainRef, PageRoute, RedirectRoute, ShellProps } from "./app-types";
export type { NavigateOptions, NavImpl, NavLinkProps, NavRouter, NavSearchParams, RedirectProps } from "./impl.types";
export type { RoutePath } from "./paths";
export { enumParam, intParam, stringParam } from "./search-params";
