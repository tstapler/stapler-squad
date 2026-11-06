import { GlobalRegistrator } from "@happy-dom/global-registrator";
import { plugin } from "bun";
GlobalRegistrator.register();
const root = import.meta.dir;
// replaces jest moduleNameMapper
plugin({ name: "css-mock", setup(b) {
  b.onLoad({ filter: /\.css\.ts$/ }, () => ({ contents: `module.exports = require("${root}/src/__mocks__/styleMock.js")`, loader: "js" }));
  b.onLoad({ filter: /\.css$/ }, () => ({ contents: `module.exports = require("${root}/src/__mocks__/styleMock.js")`, loader: "js" }));
}});
require("./jest.setup.js");
