import type { ComponentType, ReactNode } from "react";

export interface LazyOptions {
  /** Rendered while the chunk loads. */
  loading?: () => ReactNode;
}

// Inferring the component type itself (not its props) keeps the loader's contextual return type from collapsing to ComponentType<never>.
// eslint-disable-next-line @typescript-eslint/no-explicit-any
export type LazyLoader<C extends ComponentType<any>> = () => Promise<{ default: C }>;

// eslint-disable-next-line @typescript-eslint/no-explicit-any
export type LazyComponentFn = <C extends ComponentType<any>>(loader: LazyLoader<C>, options?: LazyOptions) => C;
