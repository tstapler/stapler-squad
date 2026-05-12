"use strict";

const { RuleTester } = require("eslint");
const rule = require("../require-on-click");

const ruleTester = new RuleTester({
  languageOptions: {
    ecmaVersion: 2020,
    parserOptions: { ecmaFeatures: { jsx: true } },
    sourceType: "module",
  },
});

ruleTester.run("require-on-click", rule, {
  valid: [
    // button with track() call in enclosing function
    {
      code: `
        function MyComponent() {
          const { track } = useAnalytics();
          return <button onClick={() => { track({ name: "click" }); }}>Click</button>;
        }
      `,
    },
    // div with onClick — not an interactive element
    {
      code: `
        function MyComponent() {
          return <div onClick={handler}>Not a button</div>;
        }
      `,
    },
    // spread props on button — skip (can't analyze statically)
    {
      code: `
        function MyComponent() {
          return <button {...props} onClick={handler}>Click</button>;
        }
      `,
    },
    // anchor with track() call
    {
      code: `
        function MyComponent() {
          const analytics = useAnalytics();
          return <a onClick={() => analytics.track({ name: "link_click" })} href="#">Link</a>;
        }
      `,
    },
    // track() called at component level (not inside onClick) — still valid
    {
      code: `
        function MyComponent() {
          const { track } = useAnalytics();
          track({ name: "render" });
          return <button onClick={handler}>Click</button>;
        }
      `,
    },
    // role="button" with track()
    {
      code: `
        function MyComponent() {
          const { track } = useAnalytics();
          return <div role="button" onClick={() => track({ name: "click" })}>Click</div>;
        }
      `,
    },
  ],

  invalid: [
    // bare button with onClick, no track
    {
      code: `
        function MyComponent() {
          return <button onClick={noop}>Click</button>;
        }
      `,
      errors: [{ messageId: "missingTrack" }],
    },
    // bare anchor with onClick, no track
    {
      code: `
        function MyComponent() {
          return <a onClick={handler} href="#">Link</a>;
        }
      `,
      errors: [{ messageId: "missingTrack" }],
    },
    // role="button" without track
    {
      code: `
        function MyComponent() {
          return <div role="button" onClick={handler}>Click</div>;
        }
      `,
      errors: [{ messageId: "missingTrack" }],
    },
    // button inside arrow function component, no track
    {
      code: `
        const MyComponent = () => {
          return <button onClick={() => doSomething()}>Click</button>;
        };
      `,
      errors: [{ messageId: "missingTrack" }],
    },
  ],
});
