"use strict";

/**
 * require-omnibar-dispatch
 *
 * Every case in the dispatchOmnibarAction function's top-level switch must
 * either call track() or have an // analytics-exempt comment.
 *
 * Guards:
 *  - Nested switch guard: only enforces on the first-level switch inside
 *    dispatchOmnibarAction (switchDepth === 1) to avoid false positives.
 *  - Supports both function declaration and arrow function forms.
 */

module.exports = {
  meta: {
    type: "suggestion",
    docs: {
      description:
        "Every case in dispatchOmnibarAction must call track() or be marked // analytics-exempt",
    },
    messages: {
      missingTrack:
        "dispatchOmnibarAction switch case must call track() or have an // analytics-exempt comment",
    },
    schema: [],
  },

  create(context) {
    let inTargetFunction = false;
    let switchDepth = 0;

    /**
     * Recursively walk a list of statements/nodes looking for a track() call.
     */
    function containsTrackCall(nodes) {
      if (!nodes || !Array.isArray(nodes)) return false;
      for (const node of nodes) {
        if (!node || typeof node !== "object") continue;
        if (
          node.type === "ExpressionStatement" &&
          node.expression.type === "CallExpression" &&
          ((node.expression.callee.type === "MemberExpression" &&
            node.expression.callee.property.name === "track") ||
            (node.expression.callee.type === "Identifier" &&
              node.expression.callee.name === "track"))
        ) {
          return true;
        }
        // Check inside blocks, if-statements, etc.
        if (node.type === "BlockStatement" && containsTrackCall(node.body)) {
          return true;
        }
        if (node.type === "IfStatement") {
          if (
            containsTrackCall([node.consequent]) ||
            containsTrackCall([node.alternate])
          ) {
            return true;
          }
        }
        // Check expression statements that are calls
        if (node.type === "CallExpression") {
          if (
            (node.callee.type === "MemberExpression" &&
              node.callee.property.name === "track") ||
            (node.callee.type === "Identifier" && node.callee.name === "track")
          ) {
            return true;
          }
        }
        // Check variable declarations (const result = track(...))
        if (node.type === "VariableDeclaration") {
          for (const decl of node.declarations) {
            if (decl.init && isTrackCall(decl.init)) return true;
          }
        }
      }
      return false;
    }

    function isTrackCall(node) {
      if (!node) return false;
      return (
        node.type === "CallExpression" &&
        ((node.callee.type === "MemberExpression" &&
          node.callee.property.name === "track") ||
          (node.callee.type === "Identifier" && node.callee.name === "track"))
      );
    }

    /**
     * Check whether a SwitchCase has an analytics-exempt comment.
     */
    function isAnalyticsExempt(switchCaseNode) {
      const sourceCode = context.getSourceCode();
      const comments = sourceCode.getCommentsBefore
        ? sourceCode.getCommentsBefore(switchCaseNode)
        : [];
      for (const comment of comments) {
        if (comment.value.trim() === "analytics-exempt") return true;
      }

      // Also check comments inside the consequent
      for (const stmt of switchCaseNode.consequent || []) {
        const stmtComments = sourceCode.getCommentsBefore
          ? sourceCode.getCommentsBefore(stmt)
          : [];
        for (const comment of stmtComments) {
          if (comment.value.trim() === "analytics-exempt") return true;
        }
      }
      return false;
    }

    function enterTargetFunction() {
      inTargetFunction = true;
      switchDepth = 0;
    }

    function exitTargetFunction() {
      inTargetFunction = false;
      switchDepth = 0;
    }

    return {
      // Function declaration form: function dispatchOmnibarAction(...)
      "FunctionDeclaration[id.name='dispatchOmnibarAction']"() {
        enterTargetFunction();
      },
      "FunctionDeclaration[id.name='dispatchOmnibarAction']:exit"() {
        exitTargetFunction();
      },

      // Arrow function form: const dispatchOmnibarAction = (...) => { ... }
      // or: export const dispatchOmnibarAction = ...
      "VariableDeclarator[id.name='dispatchOmnibarAction'] > ArrowFunctionExpression"() {
        enterTargetFunction();
      },
      "VariableDeclarator[id.name='dispatchOmnibarAction'] > ArrowFunctionExpression:exit"() {
        exitTargetFunction();
      },

      // Also handle: export function dispatchOmnibarAction (ExportNamedDeclaration wrapping FunctionDeclaration)
      "ExportNamedDeclaration > FunctionDeclaration[id.name='dispatchOmnibarAction']"() {
        enterTargetFunction();
      },
      "ExportNamedDeclaration > FunctionDeclaration[id.name='dispatchOmnibarAction']:exit"() {
        exitTargetFunction();
      },

      SwitchStatement() {
        if (inTargetFunction) switchDepth++;
      },

      "SwitchStatement:exit"() {
        if (inTargetFunction) switchDepth--;
      },

      SwitchCase(node) {
        if (!inTargetFunction) return;
        if (switchDepth !== 1) return; // Nested switch guard

        // Skip the default case — it's a catch-all, not an action handler
        if (node.test === null) return;

        // Check for analytics-exempt comment
        if (isAnalyticsExempt(node)) return;

        // Check consequent for track() call
        if (containsTrackCall(node.consequent)) return;

        context.report({
          node,
          messageId: "missingTrack",
        });
      },
    };
  },
};
