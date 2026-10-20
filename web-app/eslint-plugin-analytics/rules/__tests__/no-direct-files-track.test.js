"use strict";

const { RuleTester } = require("eslint");
const rule = require("../no-direct-files-track");
const onClick = require("../require-on-click");

const languageOptions = { ecmaVersion: 2020, parserOptions: { ecmaFeatures: { jsx: true } }, sourceType: "module" };
const ruleTester = new RuleTester({ languageOptions });

ruleTester.run("no-direct-files-track", rule, {
  valid: [
    { code: `track({ name: "omnibar.create_session", category: "user_action" });` },
    { code: `analytics.emit("files.tab.switch", { via: "strip" });` },
    { code: `track(event);` },
  ],
  invalid: [
    { code: `track({ name: "files.tab.open", category: "user_action" });`, errors: [{ messageId: "direct" }] },
    { code: "ctx.track({ name: `files.${x}` });", errors: [{ messageId: "direct" }] },
  ],
});

// Task 0.2.5b: the helper's emit satisfies analytics/require-on-click without an exempt comment.
new RuleTester({ languageOptions }).run("require-on-click accepts useFilesAnalytics().emit", onClick, {
  valid: [{
    code: `function Bar() { const files = useFilesAnalytics(); return <button onClick={() => files.emit("files.tab.evict_undo")}>Undo</button>; }`,
  }],
  invalid: [{
    code: `function Bar() { return <button onClick={() => go()}>Undo</button>; }`,
    errors: [{ messageId: "missingTrack" }],
  }],
});
