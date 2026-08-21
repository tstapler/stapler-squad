# Research Plan: Web Push Notifications

Created: 2026-04-16
Input: project_plans/web-push-notifications/requirements.md

## Subtopics and Search Strategy

### 1. Stack — Safari Web Push + FCM/APNs forwarding
File: `findings-stack.md`
Focus: Safari 16+ standard Web Push compatibility; what FCM/APNs-compatible payloads look like;
whether self-hosted VAPID can forward to FCM as a hub for RN delivery.
Search cap: 4 queries
Trade-off axes: browser compatibility, VAPID vs FCM/APNs, self-hosted vs hosted hub, payload portability

### 2. Features — Rich notification payloads + UX patterns
File: `findings-features.md`
Focus: What data to include in push payloads for a developer tool (session deep-links, output snippets,
action buttons); best-practice UX for permission prompts and subscribe/unsubscribe toggles.
Search cap: 4 queries
Trade-off axes: payload size limits, action button browser support, deep-link URL scheme design,
permission prompt timing (immediate vs. deferred)

### 3. Architecture — Multi-target push subscriber + preference storage
File: `findings-architecture.md`
Focus: How to structure a push delivery layer that today sends Web Push and tomorrow forwards
to FCM/APNs; where to store per-user notification preferences in a single-user Go app;
whether to extend existing EventBus subscriber pattern or introduce a dispatcher abstraction.
Search cap: 3 queries
Trade-off axes: extensibility vs. complexity, in-process vs. webhook-based delivery, JSON file
vs. config-based preference storage

### 4. Pitfalls — Known failure modes and production gotchas
File: `findings-pitfalls.md`
Focus: VAPID key rotation (what breaks when keys change); service worker update lifecycle
(SW versioning, skipWaiting race, cache invalidation); push subscription expiry handling
(410 Gone from push service); permission revocation detection; Safari 16+ quirks;
the specific mutex bug already identified in `PushService.Subscribe()`.
Search cap: 4 queries
Trade-off axes: n/a — enumerate failure modes with mitigations

## Output files
research/findings-stack.md
research/findings-features.md
research/findings-architecture.md
research/findings-pitfalls.md
research/synthesis.md (written after all four complete)
