// `@/lib/lazy/impl` is aliased per build (next.config.ts, tsconfig paths, jest moduleNameMapper).
import { lazyComponent as impl } from "@/lib/lazy/impl";
import type { LazyComponentFn } from "./impl.types";

export type { LazyLoader, LazyOptions } from "./impl.types";

/** Code-split a component; the build-specific impl owns SSR and stale-chunk behaviour. */
export const lazyComponent: LazyComponentFn = impl;
