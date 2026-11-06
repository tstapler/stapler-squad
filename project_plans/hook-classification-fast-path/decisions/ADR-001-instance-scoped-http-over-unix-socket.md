# ADR-001: Instance-scoped HTTP/JSON over Unix sockets

**Status**: Accepted
**Date**: 2026-09-23

## Context

Per-tool repository startup averages 38,661 ms. The resident instance already owns initialized rules and storage. Transport must support default, workspace, named manual, and test state without port collisions or cross-instance routing.

## Decision

Use HTTP/1.1 with compact JSON over a Unix domain socket. Derive a short endpoint from the resolved config-directory fingerprint, with explicit endpoint override taking precedence. Validate protocol version and instance fingerprint before classification. Isolated namespaces never fall back to the shared endpoint.

## Alternatives rejected

- Lightweight direct SQLite: retains startup/contention and duplicated state.
- ConnectRPC/gRPC: unnecessary code generation/framing for one small local JSON round trip.
- Dedicated `ssq-hooks serve` daemon: duplicates main-server lifecycle and instance ownership.
- Fixed localhost port: collision and weaker namespace binding.

## Consequences

Standard-library implementation and easy handler tests; requires secure socket lifecycle, compatibility negotiation, stale-socket handling, and explicit managed-session endpoint injection. Protocol is transport-neutral enough for a future remote adapter.