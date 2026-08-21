"use strict";

/**
 * require-rpc-analytics
 *
 * React components that call RPC hooks (createSession, listSessions, deleteSession,
 * pauseSession, resumeSession, etc. from useSessionService) must also call
 * track() somewhere in the enclosing component function body.
 *
 * File-path exclusions: lib/contexts/ and lib/hooks/ paths are skipped.
 *
 * An // analytics-exempt comment near the RPC call suppresses the error.
 */

// RPC hook names that trigger the rule.
const RPC_HOOK_NAMES = new Set([
  "createSession",
  "listSessions",
  "deleteSession",
  "pauseSession",
  "resumeSession",
  "updateSession",
  "getSession",
  "startSession",
  "stopSession",
]);

module.exports = {
  meta: {
    type: "suggestion",
    docs: {
      description:
        "Components calling RPC hooks must also call track() or be marked // analytics-exempt",
    },
    messages: {
      missingTrack:
        "Components calling RPC hooks must call useAnalytics().track() or mark the call // analytics-exempt",
    },
    schema: [],
  },

  create(context) {
    const filename = context.getFilename();

    // Skip files in lib/contexts/ and lib/hooks/ — these are the hook/provider definitions
    if (/\/lib\/contexts\//.test(filename) || /\/lib\/hooks\//.test(filename)) {
      return {};
    }

    /**
     * Check whether a node is an RPC hook call (direct call or destructured from hook).
     * Matches: createSession(...), useSessionService().createSession(...), etc.
     */
    function isRpcCall(node) {
      if (node.type !== "CallExpression") return false;
      const callee = node.callee;
      // Direct call: createSession(...)
      if (callee.type === "Identifier" && RPC_HOOK_NAMES.has(callee.name)) {
        return true;
      }
      // Member call: something.createSession(...)
      if (
        callee.type === "MemberExpression" &&
        RPC_HOOK_NAMES.has(callee.property.name)
      ) {
        return true;
      }
      return false;
    }

    /**
     * Recursively walk a node looking for a track() call.
     */
    function containsTrackCall(node) {
      if (!node || typeof node !== "object") return false;
      if (
        node.type === "CallExpression" &&
        ((node.callee.type === "MemberExpression" &&
          node.callee.property.name === "track") ||
          (node.callee.type === "Identifier" && node.callee.name === "track"))
      ) {
        return true;
      }
      for (const key of Object.keys(node)) {
        if (key === "parent") continue;
        const child = node[key];
        if (Array.isArray(child)) {
          for (const item of child) {
            if (item && typeof item === "object" && item.type) {
              if (containsTrackCall(item)) return true;
            }
          }
        } else if (child && typeof child === "object" && child.type) {
          if (containsTrackCall(child)) return true;
        }
      }
      return false;
    }

    /**
     * Walk up to the enclosing React component function.
     * A component is a function whose name starts with a capital letter,
     * or is an arrow function / function expression assigned to a capital-letter variable,
     * or is a default export.
     */
    function getEnclosingComponentBody(node) {
      let current = node.parent;
      while (current) {
        if (
          current.type === "FunctionDeclaration" ||
          current.type === "FunctionExpression" ||
          current.type === "ArrowFunctionExpression"
        ) {
          // Check if it looks like a React component
          const name = getFunctionName(current);
          if (name && /^[A-Z]/.test(name)) {
            return current.body;
          }
          // Default export: export default function() {} or export default () => {}
          if (
            current.parent &&
            current.parent.type === "ExportDefaultDeclaration"
          ) {
            return current.body;
          }
        }
        current = current.parent;
      }
      return null;
    }

    function getFunctionName(node) {
      if (node.type === "FunctionDeclaration" && node.id) {
        return node.id.name;
      }
      if (
        (node.type === "FunctionExpression" ||
          node.type === "ArrowFunctionExpression") &&
        node.parent
      ) {
        if (
          node.parent.type === "VariableDeclarator" &&
          node.parent.id &&
          node.parent.id.type === "Identifier"
        ) {
          return node.parent.id.name;
        }
        if (node.parent.type === "AssignmentExpression") {
          const left = node.parent.left;
          if (left.type === "Identifier") return left.name;
          if (left.type === "MemberExpression") return left.property.name;
        }
      }
      return null;
    }

    /**
     * Check for // analytics-exempt comment near the node.
     */
    function isAnalyticsExempt(node) {
      const sourceCode = context.getSourceCode();
      const comments = sourceCode.getCommentsBefore
        ? sourceCode.getCommentsBefore(node)
        : [];
      for (const comment of comments) {
        if (comment.value.trim() === "analytics-exempt") return true;
      }
      return false;
    }

    return {
      CallExpression(node) {
        if (!isRpcCall(node)) return;

        // Check analytics-exempt comment
        if (isAnalyticsExempt(node.parent || node)) return;

        // Find the enclosing component body
        const componentBody = getEnclosingComponentBody(node);
        if (!componentBody) return; // Not inside a component

        // Check if the component body contains a track() call
        if (containsTrackCall(componentBody)) return;

        context.report({
          node,
          messageId: "missingTrack",
        });
      },
    };
  },
};
