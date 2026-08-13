/**
 * Webpack loader that mocks vanilla-extract .css.ts files in Storybook.
 *
 * The VanillaExtractPlugin uses a child webpack compiler to process .css.ts
 * files, but that child compiler conflicts with next/dist/compiled/webpack
 * (the webpack bundled inside Next.js that @storybook/nextjs injects), causing:
 *   "Cannot read properties of undefined (reading 'tap')"
 *
 * This loader replaces .css.ts module output with a JS Proxy that returns
 * empty strings for any property access.  Styles won't render, but component
 * trees compile and render correctly for Storybook snapshot/a11y testing.
 */
module.exports = function cssTsMockLoader() {
  // recipe() exports are functions that return class strings; styleVariants
  // exports are objects indexed by variant key.  A plain string proxy would
  // crash both at runtime (recipe is called as a function, styleVariants is
  // indexed).  Return a recursive Proxy that is both callable (returns '') and
  // indexable (returns another callable proxy), covering all vanilla-extract
  // export shapes without needing to know the exact export type.
  return `
    function makeProxy() {
      return new Proxy(function() { return ''; }, {
        get: (_, key) => typeof key === 'string' ? makeProxy() : undefined,
        apply: () => '',
      });
    }
    module.exports = makeProxy();
  `;
};
