"use strict";

module.exports = {
  rules: {
    "require-on-click":         require("./rules/require-on-click"),
    "require-omnibar-dispatch": require("./rules/require-omnibar-dispatch"),
    "require-page-analytics":   require("./rules/require-page-analytics"),
    "require-rpc-analytics":    require("./rules/require-rpc-analytics"),
    "no-direct-files-track":    require("./rules/no-direct-files-track"),
  },
};
