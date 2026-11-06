# ADR-003: Direct replacement with compatibility fallback

**Status**: Accepted
**Date**: 2026-09-23

## Context

The user selected direct replacement rather than a feature-flag rollout. CLI and server can nevertheless be temporarily mixed during install, crash, or rollback. Current settings also contain duplicate classifier entries.

## Decision

Start/health-check the new server protocol before atomically normalizing all recognized Stapler hook commands to one canonical entry. Every request negotiates protocol and instance identity. Failure or incompatibility uses a verified instance-bound rule cache, then emits no decision (Claude defer). Never reopen the primary database as fallback.

Installer keeps an atomic settings backup. Rollback restores the prior binary/settings and removes only files owned by the new listener/cache. New files and DB behavior are additive, so no database restoration is required.

## Alternatives rejected

- Feature flag/staged cohort: explicitly declined.
- Hard deny on incompatibility: contradicts selected fallback.
- Route to any available instance: violates isolation.

## Consequences

Direct cutover remains reversible, but compatibility, crash-point, and old/new matrix tests become shipping blockers. Correctness alerts trigger immediate rollback.