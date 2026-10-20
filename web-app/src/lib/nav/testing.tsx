import { render } from "@testing-library/react";
import type { RenderOptions, RenderResult } from "@testing-library/react";
import type { ReactElement } from "react";
import { MemoryNavProvider } from "./impl.memory";

/** Renders under the in-memory nav so `push`/`replace`/`useSearchParams` behave like a router. */
export function renderWithNav(
  ui: ReactElement,
  { initialEntries, ...options }: { initialEntries?: string[] } & Omit<RenderOptions, "wrapper"> = {},
): RenderResult {
  return render(ui, {
    wrapper: ({ children }) => <MemoryNavProvider initialEntries={initialEntries}>{children}</MemoryNavProvider>,
    ...options,
  });
}
