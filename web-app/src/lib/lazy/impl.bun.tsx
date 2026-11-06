import { createElement, lazy, Suspense } from "react";
import type { ComponentType } from "react";
import { markLoadSucceeded, recoverFromChunkError } from "./stale-chunk";
import type { LazyComponentFn } from "./impl.types";

export const lazyComponent: LazyComponentFn = (loader, options) => {
  const Lazy = lazy(() =>
    loader().then(
      (mod) => {
        markLoadSucceeded();
        return mod;
      },
      (err: unknown) => recoverFromChunkError<Awaited<ReturnType<typeof loader>>>(err)
    )
  ) as unknown as ComponentType<Record<string, unknown>>;

  const LazyBoundary = (props: Record<string, unknown>) =>
    createElement(Suspense, { fallback: options?.loading?.() ?? null }, createElement(Lazy, props));
  return LazyBoundary as never;
};
