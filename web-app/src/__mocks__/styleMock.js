// Mock for CSS files in Jest. Returns callable functions so vanilla-extract
// recipe() imports (e.g. `button({ intent })`) don't throw in tests, and
// supports arbitrary-depth property chains (e.g. `vars.color.gitModified`,
// `vars.space["2"]`) so theme-contract token access doesn't throw either.
// The function/proxy stringifies to the dotted access path so className and
// CSS-value strings stay readable in test output.
function makeTokenProxy(path) {
  const fn = (..._args) => path;
  fn.toString = () => path;
  fn.valueOf = () => path;
  return new Proxy(fn, {
    get: function (target, prop) {
      if (typeof prop === "symbol") return undefined;
      if (prop === "toString" || prop === "valueOf") return target[prop];
      return makeTokenProxy(`${path}.${String(prop)}`);
    },
  });
}

module.exports = makeTokenProxy("mock");
