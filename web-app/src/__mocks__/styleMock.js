// Mock for CSS files in Jest. Returns callable functions so vanilla-extract
// recipe() imports (e.g. `button({ intent })`) don't throw in tests.
// The function returns the prop name string so className values stay readable.
//
// Recursive: each property access returns a proxied function that itself
// supports further property access (e.g. `vars.color.gitModified` from
// FileTree.tsx's module-scope theme lookups) instead of only one level deep.
// Without this, any real (unmocked) .tsx module that reads a chained
// `vars.x.y` token at module scope throws "Cannot read properties of
// undefined" during jest's import — this bit BacklogItemDetail.tsx via its
// FileTree.tsx -> theme.css import chain.
function makeLeaf(label) {
  const fn = (..._args) => label;
  fn.toString = () => label;
  fn.valueOf = () => label;
  return new Proxy(fn, {
    get: function (target, prop) {
      if (typeof prop === "symbol") return undefined;
      if (prop in target) return target[prop];
      return makeLeaf(`${label}.${String(prop)}`);
    },
  });
}

module.exports = new Proxy(
  {},
  {
    get: function (_, prop) {
      if (typeof prop === "symbol") return undefined;
      return makeLeaf(String(prop));
    },
  }
);
