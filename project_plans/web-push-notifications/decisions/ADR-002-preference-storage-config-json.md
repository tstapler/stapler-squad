# ADR-002: Notification Preferences Stored in config.json

Status: Accepted
Date: 2026-04-17

---

## Context

The settings UI requires persistent notification preferences (push enabled/disabled per-device)
that survive server restarts. Four storage options were evaluated:

- **Option A – Extend config.json**: add `NotificationPrefs` struct to the existing `Config`
- **Option B – Separate notification-prefs.json**: new file with its own load/save
- **Option C – Embed in NotificationHistoryStore**: attach prefs to notifications.json
- **Option D – In-memory only**: no persistence; fails the UX requirement

The existing `Config` struct already has version migration (`ConfigVersion`), nil-safe
field initialisation on load, and a `SaveConfig` function. The config directory
(`~/.stapler-squad/<workspace>/`) is the established location for all user-intent state.

The current `saveConfig` implementation uses `os.WriteFile` directly without a temp-rename
atomic write. Since the planned settings API will write prefs via a ConnectRPC handler,
a concurrent write + startup read race is possible. This must be hardened before new
API-writable fields are added.

---

## Decision

Adopt Option A: add `NotificationPrefs` to the `Config` struct.

```go
// config/config.go
type NotificationPrefs struct {
    PushEnabled bool `json:"pushEnabled"`
}

type Config struct {
    // ... existing fields ...
    Notifications NotificationPrefs `json:"notifications,omitempty"`
}
```

Harden `saveConfig` to use an atomic temp-rename write pattern before adding the new field:

```go
func saveConfig(cfg *Config, path string) error {
    data, err := json.MarshalIndent(cfg, "", "  ")
    if err != nil {
        return err
    }
    tmp := path + ".tmp"
    if err := os.WriteFile(tmp, data, 0600); err != nil {
        return err
    }
    return os.Rename(tmp, path)
}
```

Bump `ConfigVersion` (1 → 2). No migration logic needed; all new fields have zero-value
defaults that are semantically valid (push disabled by default).

---

## Consequences

**Positive**
- Zero new files; uses existing load/save/migrate infrastructure.
- Nil-safe on load: old config files without the field unmarshal to zero-value structs.
- Atomic write prevents torn config files under concurrent API writes.
- Single source of truth for user preferences; distinguishes intent (config.json) from
  live credential state (vapid-keys.json, push-subscriptions.json).

**Negative**
- Config file is now written by both the settings API handler and the server startup path;
  concurrency must be managed with the in-process config mutex (if one exists) or a new one.
- `ConfigVersion` bump requires careful handling if there are other in-flight schema changes.

**Mitigations**
- The atomic temp-rename write ensures file consistency even under concurrent writes.
- Use the existing `sync.RWMutex` pattern (or add one to `Config`) for in-process safety.

**Rejected alternatives**
- Option B (separate file): adds a second load/save pair for no architectural benefit in
  a single-user app.
- Option C (embed in history store): mixes user intent with live event data; violates
  single responsibility and complicates the history store.
- Option D (in-memory): fails the UX requirement for persistent subscribe/unsubscribe.

---

## Related

- findings-architecture.md: Q2 analysis
- config/config.go: existing Config struct and LoadConfig pattern
- notifications/store.go: atomic temp-rename write pattern to copy
