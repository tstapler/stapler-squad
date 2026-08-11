# ADR-002: Hook Interception Strategy (Wrapper Approach)

**Status**: Accepted
**Date**: 2026-04-06
**Project**: permissions-extension-mdd

## Context
We need to intercept calls to tools like `gemini` and `open-code` to perform a permissions check before execution. Two approaches were considered:
1. **Shell Alias**: Simple, but easily bypassed.
2. **Wrapper Binary (Prefixing $PATH)**: The tool is replaced by a wrapper binary or script that appears earlier in the user's `$PATH`.

## Decision
We will use the **Wrapper Binary** approach, where the `ssq-hooks` binary acts as a proxy for the target tool.

## Rationale
- More robust than shell aliases.
- Works across different shells (bash, zsh, fish).
- Matches the pattern used by `direnv` and `asdf`.
- Allows for a single binary (`ssq-hooks`) to handle multiple tools based on its `argv[0]`.

## Consequences
- Requires modifications to the user's shell configuration (adding `~/.stapler-squad/bin` to the front of `$PATH`).
- Must handle signal passing and terminal state correctly.
- Must find the "real" underlying binary to avoid infinite recursion.

## Patterns Applied
- **Proxy Pattern**: Intercepting and controlling access to an object.
- **Decorator Pattern**: Adding behavior (permissions check) before the original execution.
