"use strict";

/**
 * require-page-analytics
 *
 * Files matching /\/app\/.*\/page\.tsx?$/ that export a default component
 * must call usePageView() or useAnalytics().track("page_view", ...) somewhere
 * in the file.
 *
 * A file-level // analytics-exempt comment (first comment) suppresses the error.
 */

module.exports = {
  meta: {
    type: "suggestion",
    docs: {
      description:
        "Next.js page files must call usePageView() or useAnalytics().track with 'page_view'",
    },
    messages: {
      missingPageView:
        "Next.js page components must call usePageView() or useAnalytics().track('page_view'). Add // analytics-exempt at the top of the file to suppress.",
    },
    schema: [],
  },

  create(context) {
    const filename = context.getFilename();

    // Gate on filename: applies to app/page.tsx (root) and app/.../page.tsx (nested)
    if (!/\/app\/(.*\/)?page\.tsx?$/.test(filename)) {
      return {};
    }

    /**
     * Recursively walk the AST body looking for usePageView() call or
     * useAnalytics().track with "page_view" as the first argument.
     * This performs a deep walk so it finds calls inside function bodies.
     */
    function containsPageAnalytics(nodes) {
      if (!Array.isArray(nodes)) return false;
      for (const node of nodes) {
        if (deepContainsPageAnalytics(node)) return true;
      }
      return false;
    }

    /**
     * Deep walk a single node for page analytics calls.
     */
    function deepContainsPageAnalytics(node) {
      if (!node || typeof node !== "object") return false;
      if (isPageAnalyticsCall(node)) return true;
      for (const key of Object.keys(node)) {
        if (key === "parent") continue;
        const child = node[key];
        if (Array.isArray(child)) {
          for (const item of child) {
            if (item && typeof item === "object" && item.type) {
              if (deepContainsPageAnalytics(item)) return true;
            }
          }
        } else if (child && typeof child === "object" && child.type) {
          if (deepContainsPageAnalytics(child)) return true;
        }
      }
      return false;
    }

    /**
     * Check if a node is:
     *   - usePageView()
     *   - useAnalytics().track("page_view", ...)
     *   - analytics.track("page_view", ...)
     */
    function isPageAnalyticsCall(node) {
      if (!node || node.type !== "CallExpression") return false;

      // usePageView()
      if (
        node.callee.type === "Identifier" &&
        node.callee.name === "usePageView"
      ) {
        return true;
      }

      // someObj.track("page_view", ...) — legacy string form
      if (
        node.callee.type === "MemberExpression" &&
        node.callee.property.name === "track" &&
        node.arguments.length > 0 &&
        node.arguments[0].type === "Literal" &&
        node.arguments[0].value === "page_view"
      ) {
        return true;
      }

      // someObj.track({ name: "page_view", ... }) — object form used by AnalyticsProvider
      if (
        node.callee.type === "MemberExpression" &&
        node.callee.property.name === "track" &&
        node.arguments.length > 0 &&
        node.arguments[0].type === "ObjectExpression"
      ) {
        const nameProp = node.arguments[0].properties.find(
          (p) =>
            p.type === "Property" &&
            ((p.key.type === "Identifier" && p.key.name === "name") ||
              (p.key.type === "Literal" && p.key.value === "name")) &&
            p.value.type === "Literal" &&
            p.value.value === "page_view"
        );
        if (nameProp) return true;
      }

      return false;
    }

    /**
     * Check if first comment in the file is // analytics-exempt
     */
    function isFileExempt() {
      const sourceCode = context.getSourceCode();
      const allComments = sourceCode.getAllComments
        ? sourceCode.getAllComments()
        : [];
      if (allComments.length > 0 && allComments[0].value.trim() === "analytics-exempt") {
        return true;
      }
      return false;
    }

    return {
      ExportDefaultDeclaration(node) {
        if (isFileExempt()) return;

        // Get all statements in the program body
        const programBody = context.getSourceCode().ast.body;

        if (containsPageAnalytics(programBody)) return;

        context.report({
          node,
          messageId: "missingPageView",
        });
      },
    };
  },
};
