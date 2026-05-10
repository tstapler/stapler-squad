"use strict";

/**
 * require-on-click
 *
 * Requires that interactive elements (button, a, role="button") with an onClick
 * handler also call useAnalytics().track() somewhere in the enclosing component.
 *
 * Guards:
 *  - Spread prop guard: if any sibling is a JSXSpreadAttribute, skip (can't statically analyze)
 *  - analytics-exempt comment: suppresses the error
 */

module.exports = {
  meta: {
    type: "suggestion",
    docs: {
      description:
        "onClick handlers on interactive elements must call useAnalytics().track() or be marked analytics-exempt",
    },
    messages: {
      missingTrack:
        "onClick handlers on interactive elements must call useAnalytics().track() or be marked // analytics-exempt",
    },
    schema: [],
  },

  create(context) {
    /**
     * Walk all nodes in a subtree looking for a call expression where
     * callee.property.name === "track" (handles analytics.track(...), obj.track(...), etc.)
     * or callee.name === "track".
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
     * Walk up to the nearest enclosing function component body and check for track().
     */
    function enclosingFunctionContainsTrack(node) {
      let current = node.parent;
      while (current) {
        if (
          current.type === "FunctionDeclaration" ||
          current.type === "FunctionExpression" ||
          current.type === "ArrowFunctionExpression"
        ) {
          const body = current.body;
          if (body && body.type === "BlockStatement") {
            return containsTrackCall(body);
          }
          // Arrow function with expression body
          if (body) {
            return containsTrackCall(body);
          }
        }
        current = current.parent;
      }
      return false;
    }

    /**
     * Check whether a JSXElement node has an analytics-exempt comment.
     * Supports both:
     *   - Leading JS comment: // analytics-exempt
     *   - JSX comment child: {/\* analytics-exempt *\/}
     */
    function isAnalyticsExempt(jsxElementNode) {
      const sourceCode = context.getSourceCode();

      // Check leading comments on the JSXElement
      const comments = sourceCode.getCommentsBefore
        ? sourceCode.getCommentsBefore(jsxElementNode)
        : [];
      for (const comment of comments) {
        if (comment.value.trim() === "analytics-exempt") return true;
      }

      // Check JSX children for {/* analytics-exempt */}
      // In JSX, comments are JSXEmptyExpression nodes with attached comments.
      const children = jsxElementNode.children || [];
      for (const child of children) {
        if (child.type === "JSXExpressionContainer") {
          // String literal form: {"analytics-exempt"} (kept for backward compat)
          if (
            child.expression.type === "Literal" &&
            typeof child.expression.value === "string" &&
            child.expression.value.trim() === "analytics-exempt"
          ) {
            return true;
          }
          // Comment form: {/* analytics-exempt */} → JSXEmptyExpression with leading comment
          if (child.expression.type === "JSXEmptyExpression") {
            const comments = context.getSourceCode().getCommentsBefore(child.expression);
            if (comments.some((c) => c.value.trim() === " analytics-exempt" || c.value.trim() === "analytics-exempt")) {
              return true;
            }
          }
        }
      }
      return false;
    }

    return {
      'JSXAttribute[name.name="onClick"]'(node) {
        const openingElement = node.parent; // JSXOpeningElement
        if (!openingElement || openingElement.type !== "JSXOpeningElement") {
          return;
        }

        // Determine if this is an interactive element
        const elementNameNode = openingElement.name;
        let elementName = null;
        if (elementNameNode.type === "JSXIdentifier") {
          elementName = elementNameNode.name;
        }

        const siblings = openingElement.attributes || [];

        // Check for spread props — can't analyze statically, skip
        const hasSpread = siblings.some(
          (attr) => attr.type === "JSXSpreadAttribute"
        );
        if (hasSpread) return;

        // Determine if interactive: button, a, or role="button"
        const isButton = elementName === "button";
        const isAnchor = elementName === "a";
        const hasRoleButton = siblings.some(
          (attr) =>
            attr.type === "JSXAttribute" &&
            attr.name.name === "role" &&
            attr.value &&
            ((attr.value.type === "Literal" &&
              attr.value.value === "button") ||
              (attr.value.type === "JSXExpressionContainer" &&
                attr.value.expression.type === "Literal" &&
                attr.value.expression.value === "button"))
        );

        if (!isButton && !isAnchor && !hasRoleButton) return;

        // Walk up to the JSXElement (parent of JSXOpeningElement)
        const jsxElement = openingElement.parent;
        if (!jsxElement || jsxElement.type !== "JSXElement") return;

        // Check analytics-exempt
        if (isAnalyticsExempt(jsxElement)) return;

        // Check enclosing function for track() call
        if (enclosingFunctionContainsTrack(node)) return;

        context.report({
          node,
          messageId: "missingTrack",
        });
      },
    };
  },
};
