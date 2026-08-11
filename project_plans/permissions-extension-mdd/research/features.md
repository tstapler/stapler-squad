# Research: Features Dimension — Permissions Extension (MDD)

**Status**: Final | **Phase**: 1 — Research complete
**Created**: 2026-04-03

## Overview

This document surveys comparable tools, libraries, and approaches for command classification, security hooks, and AST-based analysis. The goal is to inform the feature set of the `permissions-extension-mdd` standalone hooks binary, ensuring it meets the requirements for security, portability, and ease of use across different LLM-driven CLIs (Claude, Gemini, Open Code).

## Comparable Tools Survey

### 1. Shell Hook Systems

| Tool | Mechanism | Security Model | Key Feature |
| :--- | :--- | :--- | :--- |
| **direnv** | Shell prompt hooks (`PROMPT_COMMAND`, `precmd`) | Hash-based allowlisting (`direnv allow`) | Automatic environment loading/unloading based on directory. |
| **pre-commit** | Native Git hooks wrapper (`.git/hooks/pre-commit`) | Exit code based control (0=Success, Non-zero=Abort) | Environment isolation (virtualenv, npm) for hook execution. |
| **asdf / mise** | Shell shims and hooks | Version-specific execution | Transparent tool version management via shims. |

**Takeaway for MDD**: Use shell-native hooks (like `direnv`) for transparent interception, but consider a "shim" approach for specific CLIs to ensure the classifier runs before any command execution.

### 2. Permission & Policy Engines

| Tool | Classification Method | Policy Language | Key Feature |
| :--- | :--- | :--- | :--- |
| **sudo / doas** | `Cmnd_Alias` and path patterns | Custom `sudoers` syntax | Fine-grained privilege elevation with argument restriction. |
| **OPA (Open Policy Agent)** | Structured JSON input | Rego | Decoupled policy logic; supports RBAC and complex logic. |
| **HashiCorp Boundary** | Grant Strings (RBAC) | HCL / API | Identity-aware session brokering and least-privilege access. |
| **Polkit** | Action-based authorization | JavaScript (rules) | System-wide privilege management for unprivileged processes. |

**Takeaway for MDD**: A structured policy approach (like OPA) is more robust than simple regex. MDD should classify commands into "Actions" (e.g., `read`, `write`, `execute`, `network`) and apply policies based on these classifications.

### 3. Security Analysis & AST Parsing

| Tool | Method | Strength | Weakness |
| :--- | :--- | :--- | :--- |
| **ShellCheck** | AST + Heuristics | Deep understanding of shell pitfalls (quoting, `eval`). | Limited cross-file data flow analysis. |
| **Semgrep** | AST + Pattern Matching | Highly customizable; good for "taint analysis." | Bash support can be experimental; misses complex redirections. |
| **Tree-sitter** | Incremental AST | Extremely fast; used for real-time validation. | Vulnerable to "parser differentials" if not aligned with `bash`. |

**Takeaway for MDD**: AST parsing is essential to distinguish between `echo "rm -rf /"` (safe) and `rm -rf /` (dangerous). However, MDD must mitigate "parser differentials" by normalizing input and potentially using multiple parsers.

## AI-Specific Security Tools (Emerging 2026)

- **Microsoft Agent Governance Toolkit**: Uses OPA/Cedar policies to intercept agent actions with sub-millisecond latency.
- **NVIDIA OpenShell**: Provides kernel-level isolation and declarative YAML policies for agent runtimes.
- **Bashlet / ERA**: Uses WASM (Wasmer) or micro-VMs (Firecracker) to provide hardware-level isolation for shell access.
- **MintMCP Gateway**: A security proxy specifically for Model Context Protocol (MCP) tool invocations.

## Key Feature Recommendations for MDD

Based on the survey, the following features are recommended for the `permissions-extension-mdd` project:

### 1. Standalone Hooks Binary
- **Transparent Interception**: Provide a mechanism to hook into shells (Bash, Zsh, Fish) similar to `direnv`.
- **CLI-Specific Wrappers**: Support wrapping specific binaries (e.g., `claude`, `gemini`) to ensure the classifier is always invoked.

### 2. Advanced Command Classification
- **Action-Based Mapping**: Instead of just "allow/deny", map commands to high-level actions (e.g., `FilesystemRead`, `NetworkEgress`, `SystemModification`).
- **Deep AST Analysis**: Use AST parsing (e.g., via `mvdan.cc/sh` or `tree-sitter`) to inspect command structure, handle obfuscation, and detect nested execution (e.g., `python -c "..."`).

### 3. Security Mechanisms
- **Hash-Based Approval**: Implement a "trust" mechanism similar to `direnv allow` for known-safe scripts or commands.
- **Human-in-the-Loop (HITL)**: Provide a standard interface for requesting user approval for "high-risk" classifications.
- **Policy Engine**: Use a simplified version of OPA-like logic (or integrate with OPA) to allow users to define custom security rules.

### 4. Portability & Performance
- **Zero-Dependency Binary**: Compile to a single static Go binary for easy installation.
- **Sub-millisecond Latency**: Ensure the classification logic is fast enough to be imperceptible during interactive use.

## Risks & Pitfalls to Avoid

- **Parser Differentials**: Ensure the AST parser matches the behavior of the target shell (e.g., handling of `\r`, backslashes, and quoting).
- **Argument Injection**: Be wary of patterns that allow arbitrary argument injection (the "sudo" pitfall).
- **Context Poisoning**: Protect against LLMs being tricked into generating commands that bypass the classifier's logic.

## Conclusion

The `permissions-extension-mdd` project should position itself as a "security-first hook system" for LLM-driven CLIs. By combining the transparent interception of `direnv`, the policy flexibility of `OPA`, and the deep analysis of `ShellCheck`, it can provide a robust security layer that is currently missing from most AI agent implementations.
