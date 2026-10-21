/** Strips trailing slashes (Next's `trailingSlash` export reports `/x/`) but keeps the root `/`. */
export function normalizePathname(pathname: string): string {
  const trimmed = pathname.replace(/\/+$/, "");
  return trimmed === "" ? "/" : trimmed;
}
