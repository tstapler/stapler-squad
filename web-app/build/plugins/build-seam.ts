// Selects the Bun implementations of the two build seams and refuses `next`, so a half-migrated
// import fails the build instead of shipping a second router (ADR-006).
import type { BunPlugin } from "bun";
import path from "node:path";

export class ForbiddenImportError extends Error {
  constructor(
    readonly specifier: string,
    readonly importer: string,
  ) {
    super(`forbidden import of ${specifier} from ${importer || "<entry>"}: the Bun build must not bundle next`);
    this.name = "ForbiddenImportError";
  }
}

export function buildSeamPlugin(srcDir: string): BunPlugin {
  return {
    name: "build-seam",
    setup(b) {
      // `next` and `next/*` only: a bare `^next` would also catch packages like `next-themes`.
      b.onResolve({ filter: /^next(\/|$)/ }, (a) => {
        throw new ForbiddenImportError(a.path, a.importer);
      });
      b.onResolve({ filter: /^@\/lib\/nav\/impl$/ }, () => ({ path: path.join(srcDir, "lib/nav/impl.router.tsx") }));
      b.onResolve({ filter: /^@\/lib\/lazy\/impl$/ }, () => ({ path: path.join(srcDir, "lib/lazy/impl.bun.tsx") }));
    },
  };
}
