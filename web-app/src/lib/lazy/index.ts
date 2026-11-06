import { lazyComponent as impl } from "./impl.bun";
import type { LazyComponentFn } from "./impl.types";

export type { LazyLoader, LazyOptions } from "./impl.types";

/** Code-split a component; stale-chunk recovery lives in impl.bun. */
export const lazyComponent: LazyComponentFn = impl;
