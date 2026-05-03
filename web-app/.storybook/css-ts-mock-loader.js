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
  return `
    const handler = { get: (_, key) => typeof key === 'string' ? '' : undefined };
    const proxy = new Proxy({}, handler);
    module.exports = proxy;
  `;
};
