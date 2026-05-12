"use strict";

const { RuleTester } = require("eslint");
const rule = require("../require-omnibar-dispatch");

const ruleTester = new RuleTester({
  languageOptions: {
    ecmaVersion: 2020,
    sourceType: "module",
  },
});

ruleTester.run("require-omnibar-dispatch", rule, {
  valid: [
    // case with track() call (function declaration form)
    {
      code: `
        function dispatchOmnibarAction(action, deps) {
          switch (action.type) {
            case "navigate_session":
              deps.navigate(action.sessionId);
              analytics.track({ name: "navigate" });
              return;
          }
        }
      `,
    },
    // case with analytics-exempt comment
    {
      code: `
        function dispatchOmnibarAction(action, deps) {
          switch (action.type) {
            // analytics-exempt
            case "navigate_session":
              deps.navigate(action.sessionId);
              return;
          }
        }
      `,
    },
    // arrow function form with track()
    {
      code: `
        const dispatchOmnibarAction = (action, deps) => {
          switch (action.type) {
            case "create_session":
              analytics.track({ name: "session_created" });
              deps.createSession(action);
              return;
          }
        };
      `,
    },
    // nested switch inside case — should NOT flag inner switch cases
    {
      code: `
        function dispatchOmnibarAction(action, deps) {
          switch (action.type) {
            case "navigate_session":
              analytics.track({ name: "navigate" });
              switch (action.subtype) {
                case "internal":
                  deps.navigateInternal();
                  return;
                case "external":
                  deps.navigateExternal();
                  return;
              }
              return;
          }
        }
      `,
    },
    // default case is skipped
    {
      code: `
        function dispatchOmnibarAction(action, deps) {
          switch (action.type) {
            case "navigate_session":
              analytics.track({ name: "navigate" });
              return;
            default:
              return;
          }
        }
      `,
    },
    // not named dispatchOmnibarAction — not enforced
    {
      code: `
        function handleAction(action) {
          switch (action.type) {
            case "foo":
              doFoo();
              return;
          }
        }
      `,
    },
  ],

  invalid: [
    // case with no track call (function declaration form)
    {
      code: `
        function dispatchOmnibarAction(action, deps) {
          switch (action.type) {
            case "navigate_session":
              deps.navigate(action.sessionId);
              return;
          }
        }
      `,
      errors: [{ messageId: "missingTrack" }],
    },
    // arrow function form with no track
    {
      code: `
        const dispatchOmnibarAction = (action, deps) => {
          switch (action.type) {
            case "create_session":
              deps.createSession(action);
              return;
          }
        };
      `,
      errors: [{ messageId: "missingTrack" }],
    },
    // multiple cases, only one missing track
    {
      code: `
        function dispatchOmnibarAction(action, deps) {
          switch (action.type) {
            case "navigate_session":
              analytics.track({ name: "navigate" });
              deps.navigate(action.sessionId);
              return;
            case "delete_session":
              deps.deleteSession(action.sessionId);
              return;
          }
        }
      `,
      errors: [{ messageId: "missingTrack" }],
    },
  ],
});
