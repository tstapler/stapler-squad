import dynamic from "next/dynamic";
import type { LazyComponentFn } from "./impl.types";

// ssr:false is a Next-only concern (xterm and recharts need the browser), so it lives here, not at call sites.
export const lazyComponent: LazyComponentFn = (loader, options) =>
  dynamic(loader, { ssr: false, loading: options?.loading }) as never;
