// Node half of the vanilla-extract oracle: prints { [file]: output } for the given files.
const { compile, processVanillaFile } = require("@vanilla-extract/integration");

(async () => {
  const out = {};
  for (const filePath of process.argv.slice(2)) {
    try {
      const { source } = await compile({ filePath, cwd: process.cwd(), identOption: "short" });
      out[filePath] = await processVanillaFile({ source, filePath, identOption: "short" });
    } catch (e) {
      out[filePath] = `ERROR: ${e}`;
    }
  }
  process.stdout.write(JSON.stringify(out));
})();
