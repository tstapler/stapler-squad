/** Build identifier for diagnostics: the content-hashed entry script's file name, else "unknown". */
export function readBuildId(doc: Document | undefined = typeof document === "undefined" ? undefined : document): string {
  try {
    const src = doc?.querySelector<HTMLScriptElement>('script[type="module"][src], script[src*="/_next/static/"]')?.getAttribute("src");
    const name = src?.split("?")[0].split("/").pop();
    return name || "unknown";
  } catch {
    return "unknown";
  }
}
