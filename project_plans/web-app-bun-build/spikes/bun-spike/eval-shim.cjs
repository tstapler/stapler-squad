// Replacement for the `eval` package: same contract, but uses createRequire instead of Module._load.
const vm = require("node:vm");
const { createRequire } = require("node:module");
module.exports = function (content, filename, scope, includeGlobals) {
  const req = createRequire(filename);
  const exports = {};
  const sandbox = { ...(includeGlobals ? globalThis : {}), ...scope, require: req, exports };
  sandbox.module = { exports, filename, id: filename, require: req };
  sandbox.global = sandbox;
  new vm.Script(content, { filename, displayErrors: false }).runInNewContext(sandbox);
  return sandbox.module.exports;
};
