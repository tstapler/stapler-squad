# Research: Pitfalls — Permissions Extension (MDD)

**Status**: Draft | **Phase**: 1 — Research
**Created**: 2026-04-03

This document identifies known failure modes, gotchas, and risks associated with the command classification and AST parsing logic for the Permissions Extension.

## 1. Performance Overhead

### 1.1. Synchronous Execution Latency
- **Risk**: Hooks in Gemini and other CLIs often run **synchronously**. The agent loop pauses and waits for every matching hook to complete before proceeding.
- **Impact**: A slow shell script or a network call inside a hook will make the AI feel laggy or unresponsive.
- **Mitigation**: Keep hook logic lightweight. Use specific "matchers" to ensure hooks only run for relevant tools/events.

### 1.2. AST Parsing Overhead
- **Risk**: Parsing every command before execution can introduce latency, especially for large scripts or complex commands.
- **Impact**: Noticeable delay between the AI generating a command and its execution.
- **Mitigation**: Use efficient parsers like `mvdan/sh`. Avoid full AST parsing if simple tokenization (e.g., `google/shlex`) is sufficient for the classification level.

### 1.3. Memory Overhead
- **Risk**: Large ASTs can be memory-intensive. Each token and node is a struct with position information.
- **Impact**: Potential memory leaks or high memory usage in long-running processes.
- **Mitigation**: Ensure AST references are not leaked. Use streaming parsers where possible.

## 2. Shell-Specific Parsing Issues

### 2.1. Parser Differentials
- **Risk**: Multiple parsers (e.g., the security validator vs. the actual shell) may interpret the same command differently.
- **Impact**: An attacker could "hide" malicious commands in the gap between interpretations (e.g., using carriage returns `\r` as word separators if the validator doesn't recognize them but the shell does).
- **Mitigation**: Use a single, authoritative parser (like `tree-sitter-bash` or `mvdan/sh`) for both validation and execution.

### 2.2. Arithmetic Ambiguity (`mvdan/sh`)
- **Risk**: The `mvdan/sh` parser assumes `((` and `$((` always start arithmetic expressions.
- **Impact**: Fails on valid but ambiguous Bash code like `((foo); (bar))`.
- **Mitigation**: Add spaces to disambiguate: `( (foo); (bar))`.

### 2.3. Keyword Promotion (`mvdan/sh`)
- **Risk**: `mvdan/sh` treats `export`, `let`, and `declare` as keywords rather than built-ins.
- **Impact**: If these names are used as regular commands or in complex aliases, the static parser might misinterpret the structure.
- **Mitigation**: Be aware of this behavior when parsing complex scripts.

### 2.4. Non-Standard Shell Behavior
- **Risk**: Different shells (Bash, Zsh, Fish, Dash, POSIX sh) have subtle differences in expansion rules and syntax.
- **Impact**: A command classified as safe in one shell might be dangerous in another.
- **Mitigation**: Target a specific shell (e.g., POSIX sh or Bash) and ensure the parser matches that shell's behavior.

## 3. Security Risks

### 3.1. Command Injection in Hooks
- **Risk**: Hook scripts taking arguments from the agent (like a filename or search string) and passing them directly to a shell command without sanitization.
- **Impact**: Vulnerable to command injection (e.g., a filename named `; rm -rf /`).
- **Mitigation**: Treat all `tool_input` as untrusted. Use language-specific libraries for safe execution rather than raw string interpolation.

### 3.2. Context Poisoning (AI-Specific)
- **Risk**: Malicious instructions in files (e.g., `CLAUDE.md`) can survive context compression and be "laundered" into what the model treats as a genuine user directive.
- **Impact**: The AI generates dangerous shell commands based on malicious instructions.
- **Mitigation**: Implement deep security analysis of generated commands before execution.

### 3.3. Environment Leakage
- **Risk**: Hooks inherit the CLI's environment variables, which may include sensitive API keys.
- **Impact**: Sensitive information could be exposed to third-party hook scripts.
- **Mitigation**: Redact or filter environment variables passed to hooks.

## 4. Integration & Reliability

### 4.1. Stdout Pollution
- **Risk**: Printing anything to `stdout` other than the final JSON object in a hook script.
- **Impact**: Breaks the JSON handshake with the CLI, causing it to fail.
- **Mitigation**: Redirect all logs, debug info, and errors to `stderr`.

### 4.2. CI-Mode Interactivity Block
- **Risk**: Environment variables starting with `CI_` (e.g., `CI_TOKEN`) can cause underlying UI libraries to disable the terminal interface.
- **Impact**: The CLI fails to enter interactive mode.
- **Mitigation**: Unset `CI_` variables before launching the CLI in local development environments.

### 4.3. Terminal UI (PTY) Corruption
- **Risk**: Running interactive apps (like `vim` or `htop`) inside the CLI can cause UI layout breaks or inconsistent terminal states.
- **Impact**: "UI going haywire"—text overlaps, layout breaks.
- **Mitigation**: Use robust PTY handling and avoid complex terminal multiplexers if possible.

### 4.4. Path Contamination and Version Drift
- **Risk**: Different tools (e.g., `direnv`, `pre-commit`) modifying the `PATH` differently.
- **Impact**: Version drift where the version of a tool used by the IDE differs from the version used by the hook.
- **Mitigation**: Ensure consistent environment management across tools.

## 5. Standalone Binary Constraints

### 5.1. Configuration Management
- **Risk**: Running independently of `stapler-squad` means the binary needs its own configuration and state management.
- **Impact**: Potential for configuration drift or duplication of logic.
- **Mitigation**: Share domain logic and configuration structures between the main project and the standalone binary.

### 5.2. Dependency Management
- **Risk**: The standalone binary must be able to run without the full `stapler-squad` system.
- **Impact**: "Hidden" dependencies (requiring specific tools to be installed) can make the binary hard to use.
- **Mitigation**: Minimize external dependencies and bundle necessary logic within the binary.
