"use strict";

/**
 * no-direct-files-track
 *
 * Forbids `track({ name: "files.…" })` outside web-app/src/lib/files/analytics/ (ADR-035):
 * event names and labels must come from the typed registry via useFilesAnalytics().emit.
 */
const isFilesName = (node) =>
  (node.type === "Literal" && typeof node.value === "string" && node.value.startsWith("files.")) ||
  (node.type === "TemplateLiteral" && node.quasis[0].value.cooked.startsWith("files."));

module.exports = {
  meta: {
    type: "problem",
    docs: { description: "files.* analytics events must go through useFilesAnalytics()" },
    messages: { direct: "Use useFilesAnalytics().emit(...) instead of track({ name: \"files.…\" }) (ADR-035)." },
    schema: [],
  },
  create(context) {
    return {
      CallExpression(node) {
        const callee = node.callee;
        const isTrack =
          (callee.type === "Identifier" && callee.name === "track") ||
          (callee.type === "MemberExpression" && callee.property.name === "track");
        const arg = node.arguments[0];
        if (!isTrack || !arg || arg.type !== "ObjectExpression") return;
        const name = arg.properties.find((p) => p.type === "Property" && (p.key.name === "name" || p.key.value === "name"));
        if (name && isFilesName(name.value)) context.report({ node, messageId: "direct" });
      },
    };
  },
};
