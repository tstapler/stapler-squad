"use strict";

/**
 * require-abort-signal
 *
 * Hooks and contexts (src/lib/hooks/**, src/lib/contexts/**) that call a
 * ConnectRPC client method must pass a { signal } option, tied to an
 * AbortController aborted on unmount/re-run (see useAbortableRequest).
 *
 * Root incident: useSessionVcs fired a fresh getVCSStatus/getSessionDiff
 * call per session switch with no cancellation. Every prior in-flight
 * request (and everything its closure held: sessionId, setState, the
 * component's fiber) stayed alive until it resolved or hit its deadline.
 * Measured 2026-09-04: ~44MB -> ~154MB JS heap over 138 rapid session
 * switches in one profiling run, traced to exactly this pattern.
 *
 * A `// abort-signal-exempt` comment on the call suppresses this for a
 * genuinely fire-and-forget call with no mount-tied loading state (e.g. a
 * one-shot mutation triggered by a user action, not a render/effect).
 */

function isClientObject(node, createdClientNames) {
  if (!node) return false;
  // client.method(...) where `client` was assigned from createClient(...)
  if (node.type === "Identifier" && createdClientNames.has(node.name)) {
    return true;
  }
  // getClient().method(...) — this repo's convention for a memoized client.
  if (
    node.type === "CallExpression" &&
    node.callee.type === "Identifier" &&
    node.callee.name === "getClient"
  ) {
    return true;
  }
  return false;
}

function hasSignalOption(args) {
  if (args.length < 2) return false;
  const opts = args[1];
  if (!opts || opts.type !== "ObjectExpression") return false;
  return opts.properties.some(
    (p) =>
      p.type === "Property" &&
      ((p.key.type === "Identifier" && p.key.name === "signal") ||
        (p.key.type === "Literal" && p.key.value === "signal")) ||
      p.type === "SpreadElement"
  );
}

module.exports = {
  meta: {
    type: "suggestion",
    docs: {
      description:
        "RPC client calls in hooks/contexts must pass { signal } (see useAbortableRequest) for cancellation on unmount/re-run",
    },
    messages: {
      missingSignal:
        "RPC call in a hook/context must pass { signal } (use useAbortableRequest()) or be marked // abort-signal-exempt",
    },
    schema: [],
  },

  create(context) {
    const filename = context.getFilename();
    if (!/\/lib\/(hooks|contexts)\//.test(filename)) return {};
    if (/\.(test|spec)\.[tj]sx?$/.test(filename)) return {};

    const createdClientNames = new Set();

    // Deliberately NOT AST-based (no getCommentsBefore/parent-walk): that
    // approach worked under plain espree in manual testing but silently
    // failed to find an otherwise-correctly-placed exempt comment when
    // linted for real via `next lint` (@typescript-eslint/parser) — the two
    // parsers attach comments to different node boundaries for the same
    // source text. Scanning raw comments by line number sidesteps parser-
    // specific comment/node attachment entirely.
    function isAbortExempt(node) {
      const sourceCode = context.getSourceCode();
      const allComments = sourceCode.getAllComments
        ? sourceCode.getAllComments()
        : [];
      const callLine = node.loc.start.line;
      return allComments.some(
        (c) =>
          c.value.trim() === "abort-signal-exempt" &&
          c.loc.end.line <= callLine &&
          c.loc.end.line >= callLine - 6
      );
    }

    return {
      VariableDeclarator(node) {
        if (
          node.init &&
          node.init.type === "CallExpression" &&
          node.init.callee.type === "Identifier" &&
          node.init.callee.name === "createClient" &&
          node.id.type === "Identifier"
        ) {
          createdClientNames.add(node.id.name);
        }
      },
      CallExpression(node) {
        const callee = node.callee;
        if (callee.type !== "MemberExpression") return;
        if (!isClientObject(callee.object, createdClientNames)) return;
        if (hasSignalOption(node.arguments)) return;
        if (isAbortExempt(node)) return;

        context.report({ node, messageId: "missingSignal" });
      },
    };
  },
};
