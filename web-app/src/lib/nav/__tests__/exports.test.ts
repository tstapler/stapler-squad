/** The public surface of `@/lib/nav` is an allow-list, so a file dropped into this directory cannot leak router internals through index.ts. */
const facade = jest.requireActual<Record<string, unknown>>("../index");

const ALLOWED = [
  "Link",
  "Redirect",
  "enumParam",
  "intParam",
  "pushHistoryMarker",
  "pushTrayEntry",
  "replaceHash",
  "stringParam",
  "useReplaceSearch",
  "usePathname",
  "useRouter",
  "useSearchParams",
].sort();

const FORBIDDEN = ["createAppRouter", "AppRouterProvider", "createBrowserRouter", "RouterProvider", "Outlet"];

it("exports exactly the facade names and parsers", () => {
  expect(Object.keys(facade).sort()).toEqual(ALLOWED);
});

it.each(FORBIDDEN)("does not export %s", (name) => {
  expect(facade).not.toHaveProperty(name);
});
