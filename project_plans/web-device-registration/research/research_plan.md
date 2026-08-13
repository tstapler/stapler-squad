# Research Plan: Web Device Registration

**Project**: web-device-registration  
**Requirements**: `project_plans/web-device-registration/requirements.md`  
**Date**: 2026-04-21

## Subtopics

### 1. Stack
**Focus**: Audit existing `server/auth/` backend to determine exactly what new endpoints and backend changes are needed.  
**Strategy**: Code archaeology of the existing codebase — no web search needed.  
**Search cap**: 0 web searches (codebase is the source of truth)  
**Key axes for trade-off matrix**: Reuse vs new code, complexity, alignment with existing patterns  
**Output**: `research/findings-stack.md`

### 2. Features
**Focus**: How comparable tools (Tailscale, Vaultwarden, Bitwarden, GitHub) handle multi-device enrollment UX.  
**Strategy**: Web search + training knowledge  
**Search cap**: 4 searches max  
**Key axes**: UX flow, security model, token lifetime, CA cert bootstrapping  
**Output**: `research/findings-features.md`

### 3. Architecture
**Focus**: Design the new HTTP endpoints, credential list/revoke API, `/account` React page structure, and QR code serving approach.  
**Strategy**: Code archaeology of existing handlers + training knowledge on WebAuthn credential management patterns  
**Search cap**: 2 web searches  
**Key axes**: Endpoint design, auth requirements per endpoint, React component hierarchy  
**Output**: `research/findings-architecture.md`

### 4. Pitfalls
**Focus**: CSRF risks on invite generation, token replay attacks, credential orphaning on revoke, CA cert bootstrapping for new devices.  
**Strategy**: Training knowledge + targeted web search on WebAuthn security  
**Search cap**: 3 searches max  
**Key axes**: Attack surface, mitigation availability, severity  
**Output**: `research/findings-pitfalls.md`

## Synthesis Target

After all four findings files are complete:  
→ Parent synthesizes into `research/synthesis.md` (ADR-Ready format)  
→ Synthesis feeds into `/plan:adr` and `/plan:feature`
