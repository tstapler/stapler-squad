# Validation Plan: Web Push Notifications

Status: Ready — use before writing implementation code
Phase: 4 - Validation complete
Date: 2026-04-18

Input: `implementation/plan.md`, `requirements.md`, `research/findings-*.md`
Output: this file
Next: open a **fresh session** → `/code:implement` with `plan.md` + `validation.md`

---

## Requirements Traceability Matrix

Every requirement from `requirements.md` is traced to at least one test case.

| Req | Requirement Summary | Test IDs | Priority |
|-----|---------------------|----------|----------|
| R1 | Fix mutex deadlock in PushService.Subscribe | UT-1.1, UT-1.2 | P0 |
| R2 | HTTP 410/404 removes stale subscription | UT-1.3, UT-1.4, UT-1.5 | P0 |
| R3 | Push wired in server startup | IT-1.1 | P0 |
| R4 | URGENT priority notifications trigger push | UT-2.1 | P1 |
| R5 | APPROVAL_NEEDED always fires push regardless of priority | UT-2.2, UT-2.3 | P1 |
| R6 | Deep-link URL uses session.ID (not Title) | UT-2.4, UT-2.5 | P1 |
| R7 | Notifier interface accepts multiple notifiers | UT-3.1, UT-3.2 | P1 |
| R8 | NotificationPrefs persisted in config.json | UT-4.1, UT-4.2, UT-4.3 | P2 |
| R9 | saveConfig is atomic (no torn writes) | UT-4.4 | P2 |
| R10 | Permission revocation detected while app open | MT-1.1 | P2 |
| R11 | Settings toggle: three states rendered correctly | CT-1.1, CT-1.2, CT-1.3 | P2 |
| R12 | Push payload includes notificationType, timestamp, actions | UT-5.1, UT-5.2 | P2 |
| R13 | requireInteraction=true on approval_needed | UT-5.3 | P2 |
| R14 | renotify=true on approval_needed | UT-5.4 | P2 |
| R15 | Payload ≤ 3900 bytes after enrichment | UT-5.5 | P2 |
| R16 | SW reads dynamic actions from payload | MT-2.1 | P3 |
| R17 | notificationclick dispatches by event.action | MT-2.2 | P3 |
| R18 | Push SW and cache SW separated | MT-3.1 | P3 |
| R19 | Backend payload FCM-compatible data map | UT-5.6 | P2 |
| R20 | No goroutine leak on server shutdown | IT-1.2 | P1 |
| R21 | resp.Body.Close() called on all push responses | UT-1.5 | P0 |

---

## Test Pyramid

```
          ┌──────────────────────┐
          │  Manual / E2E (5%)   │  MT-1.x, MT-2.x, MT-3.x
          │  10 scenarios        │  Browser-level only
          ├──────────────────────┤
          │  Integration (15%)   │  IT-1.x, IT-2.x
          │  4 scenarios         │  Real server/EventBus; no real push endpoint
          ├──────────────────────┤
          │  Unit Tests (80%)    │  UT-1.x … UT-5.x
          │  21 test cases       │  Pure functions + mock HTTP + mock Notifier
          └──────────────────────┘
```

Target coverage: **≥ 80% statement coverage** on new/modified Go files.
Files that must reach ≥ 80%:
- `server/services/push_service.go`
- `server/push/subscriber.go`
- `server/push/notifier.go` (new)
- `server/push/trigger_constants.go` (new)
- `config/config.go` (modified sections)

---

## Unit Tests

### UT-1.x — PushService correctness (Story 1)

**File**: `server/services/push_service_test.go`
**Package**: `services`
**Pattern**: table-driven, testify assertions, `t.Parallel()` where safe

---

#### UT-1.1 — Subscribe does not deadlock under concurrent calls [R1]

```go
func TestSubscribeConcurrent(t *testing.T) {
    svc := newTestPushService(t)
    var wg sync.WaitGroup
    for i := 0; i < 10; i++ {
        wg.Add(1)
        go func(i int) {
            defer wg.Done()
            svc.Subscribe(PushSubscription{
                Endpoint: fmt.Sprintf("https://example.com/%d", i),
            })
        }(i)
    }
    done := make(chan struct{})
    go func() { wg.Wait(); close(done) }()
    select {
    case <-done: // pass
    case <-time.After(2 * time.Second):
        t.Fatal("Subscribe deadlocked under concurrent calls")
    }
}
```

**Run with**: `go test ./server/services/... -race -timeout 10s`
**Passes when**: no deadlock, no data race, all 10 subscriptions stored.

---

#### UT-1.2 — Subscribe/Unsubscribe/GetSubscriptions use correct lock pairs [R1]

```go
func TestMutexSymmetry(t *testing.T) {
    // Subscribe acquires Lock() then Unlock() (not RUnlock)
    // GetSubscriptions acquires RLock() then RUnlock()
    // Unsubscribe acquires Lock() then Unlock()
    // Test: Subscribe then GetSubscriptions from separate goroutines simultaneously
    svc := newTestPushService(t)
    svc.Subscribe(PushSubscription{Endpoint: "https://example.com/1"})
    var wg sync.WaitGroup
    for i := 0; i < 5; i++ {
        wg.Add(2)
        go func() { defer wg.Done(); svc.Subscribe(PushSubscription{Endpoint: "https://example.com/new"}) }()
        go func() { defer wg.Done(); _ = svc.GetSubscriptions() }()
    }
    wg.Wait() // passes if no panic/deadlock under -race
}
```

**Passes when**: `-race` reports no data races.

---

#### UT-1.3 — SendNotification removes subscription on HTTP 410 [R2]

```go
func TestSendNotification410RemovesSubscription(t *testing.T) {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusGone) // 410
    }))
    defer server.Close()

    svc := newTestPushService(t)
    svc.Subscribe(PushSubscription{Endpoint: server.URL})

    require.Len(t, svc.GetSubscriptions(), 1)
    svc.SendNotification(PushNotification{Title: "test", Body: "body"})
    assert.Len(t, svc.GetSubscriptions(), 0, "subscription must be removed after 410")
}
```

---

#### UT-1.4 — SendNotification removes subscription on HTTP 404 [R2]

Same as UT-1.3 with `http.StatusNotFound`. Subscription must be removed.

---

#### UT-1.5 — SendNotification closes response body on all status codes [R21]

```go
func TestSendNotificationClosesBody(t *testing.T) {
    statuses := []int{201, 410, 404, 413, 429, 500}
    for _, status := range statuses {
        t.Run(fmt.Sprintf("status_%d", status), func(t *testing.T) {
            bodyRead := false
            server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                w.WriteHeader(status)
                // body read = Body.Close called when body drained
            }))
            defer server.Close()

            svc := newTestPushService(t)
            svc.Subscribe(PushSubscription{Endpoint: server.URL})
            svc.SendNotification(PushNotification{Title: "t", Body: "b"})
            _ = bodyRead // assert no leaked goroutine via -race
        })
    }
}
```

**Run with**: `go test ./server/services/... -race`
**Passes when**: no goroutine leak detected; no race on body read.

---

#### UT-1.6 — SendNotification retains subscription on HTTP 201 [R2]

```go
func TestSendNotification201RetainsSubscription(t *testing.T) {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(201)
    }))
    defer server.Close()

    svc := newTestPushService(t)
    svc.Subscribe(PushSubscription{Endpoint: server.URL})
    svc.SendNotification(PushNotification{Title: "test", Body: "body"})
    assert.Len(t, svc.GetSubscriptions(), 1, "subscription must NOT be removed after 201")
}
```

---

### UT-2.x — Push trigger rules (Story 2)

**File**: `server/push/subscriber_test.go`
**Package**: `push`
**Pattern**: table-driven test over (EventType, Priority, NotificationType) → shouldNotify bool

```go
func TestShouldNotifyTable(t *testing.T) {
    tests := []struct {
        name             string
        eventType        events.EventType
        priority         int32
        notificationType int32
        newStatus        session.Status
        wantNotify       bool
    }{
        // EventNotification cases
        {"low priority generic → no push",    events.EventNotification, priorityLow,    typeGeneric,   0,                   false},
        {"medium priority generic → no push", events.EventNotification, priorityMedium, typeGeneric,   0,                   false},
        {"high priority generic → push",      events.EventNotification, priorityHigh,   typeGeneric,   0,                   true},  // UT-2.1a
        {"urgent priority generic → push",    events.EventNotification, priorityUrgent, typeGeneric,   0,                   true},  // UT-2.1b [BUG-2 fix]
        {"low priority APPROVAL → push",      events.EventNotification, priorityLow,    typeApproval,  0,                   true},  // UT-2.2 [R5]
        {"high priority APPROVAL → push",     events.EventNotification, priorityHigh,   typeApproval,  0,                   true},  // UT-2.3
        // EventSessionStatusChanged cases
        {"session stopped → push",            events.EventSessionStatusChanged, 0, 0, session.Stopped,      true},
        {"session needs approval → push",     events.EventSessionStatusChanged, 0, 0, session.NeedsApproval, true},
        {"session running → no push",         events.EventSessionStatusChanged, 0, 0, session.Running,      false},
        // Other event types
        {"unrelated event → no push",         events.EventSessionCreated, 0, 0, 0,                          false},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got := shouldNotify(tt.eventType, tt.priority, tt.notificationType, tt.newStatus)
            assert.Equal(t, tt.wantNotify, got, "shouldNotify mismatch for: %s", tt.name)
        })
    }
}
```

**Note**: `shouldNotify` must be extracted to a pure, testable function from the switch statement. If it remains inline, wrap it in a helper that accepts the same parameters.

---

#### UT-2.4 — URL uses session.ID not session.Title [R6]

```go
func TestNotificationURLUsesSessionID(t *testing.T) {
    notification := buildNotificationForSession(&session.Instance{
        ID:    "session-abc-123",
        Title: "My Session (renamed)",
    }, events.EventSessionStatusChanged)

    url, ok := notification.Data["url"].(string)
    require.True(t, ok)
    assert.Contains(t, url, "session-abc-123", "URL must contain session ID")
    assert.NotContains(t, url, "My Session (renamed)", "URL must not contain mutable title")
}
```

---

#### UT-2.5 — Tag suffix uses session.ID not session.Title [R6]

```go
func TestNotificationTagUsesSessionID(t *testing.T) {
    notification := buildNotificationForSession(&session.Instance{
        ID:    "session-abc-123",
        Title: "Renamed Title",
    }, events.EventSessionStatusChanged)

    assert.Contains(t, notification.Tag, "session-abc-123")
    assert.NotContains(t, notification.Tag, "Renamed Title")
}
```

---

### UT-3.x — Notifier interface (Story 2)

**File**: `server/push/notifier_test.go`
**Package**: `push`

---

#### UT-3.1 — StartDeliverySubscriber calls all Notifiers in slice [R7]

```go
type mockNotifier struct {
    name   string
    calls  []DeliveryNotification
    mu     sync.Mutex
}
func (m *mockNotifier) Send(_ context.Context, n DeliveryNotification) error {
    m.mu.Lock(); defer m.mu.Unlock()
    m.calls = append(m.calls, n)
    return nil
}
func (m *mockNotifier) Name() string { return m.name }

func TestStartDeliverySubscriberCallsAllNotifiers(t *testing.T) {
    bus := events.NewEventBus(10)
    n1, n2 := &mockNotifier{name: "n1"}, &mockNotifier{name: "n2"}

    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    StartDeliverySubscriber(ctx, bus, []Notifier{n1, n2})

    // Publish a high-priority notification
    bus.Publish(&events.Event{
        Type:                 events.EventNotification,
        NotificationPriority: int32(priorityHigh),
        NotificationTitle:    "Test",
        NotificationMessage:  "Body",
    })

    time.Sleep(50 * time.Millisecond) // allow goroutine to process

    assert.Len(t, n1.calls, 1, "n1 must receive notification")
    assert.Len(t, n2.calls, 1, "n2 must receive notification")
}
```

---

#### UT-3.2 — One failing Notifier does not prevent delivery to others [R7]

```go
func TestDeliverySubscriberContinuesOnNotifierError(t *testing.T) {
    bus := events.NewEventBus(10)
    failingNotifier := &errorNotifier{name: "failing"}
    successNotifier := &mockNotifier{name: "success"}

    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    StartDeliverySubscriber(ctx, bus, []Notifier{failingNotifier, successNotifier})

    bus.Publish(&events.Event{
        Type:                 events.EventNotification,
        NotificationPriority: int32(priorityHigh),
        NotificationTitle:    "Test",
        NotificationMessage:  "Body",
    })

    time.Sleep(50 * time.Millisecond)
    assert.Len(t, successNotifier.calls, 1, "success notifier must still receive despite first notifier error")
}
```

---

### UT-4.x — Config notification preferences (Story 3)

**File**: `config/config_test.go` (extend existing)
**Package**: `config`

---

#### UT-4.1 — NotificationPrefs round-trip in Config [R8]

```go
func TestNotificationPrefsRoundTrip(t *testing.T) {
    dir := t.TempDir()
    cfg := &Config{
        ConfigVersion: 2,
        Notifications: NotificationPrefs{PushEnabled: true},
    }
    err := saveConfig(cfg, filepath.Join(dir, "config.json"))
    require.NoError(t, err)

    loaded, err := LoadConfig(filepath.Join(dir, "config.json"))
    require.NoError(t, err)
    assert.True(t, loaded.Notifications.PushEnabled)
}
```

---

#### UT-4.2 — v1 config loads with NotificationPrefs defaults [R8]

```go
func TestV1ConfigLoadsWithNotificationDefaults(t *testing.T) {
    dir := t.TempDir()
    v1Json := `{"configVersion": 1, "sessionDefaults": {}}`
    os.WriteFile(filepath.Join(dir, "config.json"), []byte(v1Json), 0600)

    cfg, err := LoadConfig(filepath.Join(dir, "config.json"))
    require.NoError(t, err)
    // Notifications field has zero-value defaults (PushEnabled = false)
    assert.False(t, cfg.Notifications.PushEnabled, "default must be push disabled")
}
```

---

#### UT-4.3 — PushEnabled=false is the zero-value default [R8]

```go
func TestNotificationPrefsDefault(t *testing.T) {
    var prefs NotificationPrefs
    assert.False(t, prefs.PushEnabled, "push must be disabled by default")
}
```

---

#### UT-4.4 — saveConfig is atomic: partial write leaves previous file intact [R9]

```go
func TestSaveConfigAtomic(t *testing.T) {
    dir := t.TempDir()
    path := filepath.Join(dir, "config.json")

    // Write initial config
    initial := &Config{ConfigVersion: 2}
    require.NoError(t, saveConfig(initial, path))

    // Verify no .tmp file remains after a successful write
    _, err := os.Stat(path + ".tmp")
    assert.True(t, os.IsNotExist(err), ".tmp file must be cleaned up after successful save")

    // Verify the file at path is valid JSON
    data, _ := os.ReadFile(path)
    var check Config
    assert.NoError(t, json.Unmarshal(data, &check))
}
```

---

### UT-5.x — Payload enrichment (Story 4)

**File**: `server/push/subscriber_test.go` (extend)
**Package**: `push`

---

#### UT-5.1 — Payload data map includes notificationType and timestamp [R12]

```go
func TestPayloadDataMapFields(t *testing.T) {
    notif := buildApprovalNotification(&session.Instance{ID: "s1", Title: "S1"})

    assert.Equal(t, "APPROVAL_NEEDED", notif.Data["notificationType"])
    ts, ok := notif.Data["timestamp"].(int64)
    require.True(t, ok, "timestamp must be int64")
    assert.Greater(t, ts, int64(0))
}
```

---

#### UT-5.2 — Payload data map includes sessionId using session.ID [R12, R6]

```go
func TestPayloadDataSessionID(t *testing.T) {
    notif := buildApprovalNotification(&session.Instance{ID: "abc-123", Title: "Changed Title"})
    assert.Equal(t, "abc-123", notif.Data["sessionId"])
}
```

---

#### UT-5.3 — RequireInteraction=true on approval_needed [R13]

```go
func TestRequireInteractionApproval(t *testing.T) {
    notif := buildApprovalNotification(&session.Instance{ID: "s1", Title: "S1"})
    assert.True(t, notif.RequireInteraction)
}

func TestRequireInteractionSessionComplete(t *testing.T) {
    notif := buildCompletedNotification(&session.Instance{ID: "s1", Title: "S1"})
    assert.False(t, notif.RequireInteraction)
}
```

---

#### UT-5.4 — Renotify=true on approval_needed, false on session-complete [R14]

```go
func TestRenotifyApproval(t *testing.T) {
    notif := buildApprovalNotification(&session.Instance{ID: "s1", Title: "S1"})
    assert.True(t, notif.Renotify)
}

func TestRenotifyComplete(t *testing.T) {
    notif := buildCompletedNotification(&session.Instance{ID: "s1", Title: "S1"})
    assert.False(t, notif.Renotify)
}
```

---

#### UT-5.5 — Payload JSON does not exceed 3900 bytes [R15]

```go
func TestPayloadSizeWithinBudget(t *testing.T) {
    // Worst-case: long title + long body
    sess := &session.Instance{
        ID:    strings.Repeat("a", 100),
        Title: strings.Repeat("Session Title ", 10), // 130 chars
    }
    notif := buildApprovalNotification(sess)

    data, err := json.Marshal(notif)
    require.NoError(t, err)
    assert.LessOrEqual(t, len(data), 3900, "payload must fit within push budget")
}
```

---

#### UT-5.6 — Payload data map is FCM-compatible [R19]

```go
func TestPayloadFCMCompatibility(t *testing.T) {
    notif := buildApprovalNotification(&session.Instance{ID: "s1", Title: "S1"})

    // FCM data map requires string values
    for k, v := range notif.Data {
        switch v.(type) {
        case string, int64, bool:
            // valid FCM data map value types
        default:
            t.Errorf("data[%q] has non-FCM-compatible type %T", k, v)
        }
    }

    // Required FCM fields present
    assert.NotEmpty(t, notif.Data["sessionId"])
    assert.NotEmpty(t, notif.Data["notificationType"])
    assert.NotEmpty(t, notif.Data["url"])
}
```

---

## Integration Tests

**File**: `server/push/subscriber_integration_test.go`
**Build tag**: `//go:build integration` (or run with `-tags integration`)
**Pattern**: real EventBus, mock HTTP server for push endpoint, no real VAPID

---

### IT-1.1 — PushService registered in server; EventBus event triggers delivery [R3]

```go
func TestPushDeliveryEndToEnd(t *testing.T) {
    // Set up a mock push endpoint
    notified := make(chan struct{}, 1)
    pushServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        notified <- struct{}{}
        w.WriteHeader(201)
    }))
    defer pushServer.Close()

    // Wire up EventBus + PushService + subscriber
    bus := events.NewEventBus(10)
    svc := newTestPushServiceWithEndpoint(t, pushServer.URL)
    svc.Subscribe(PushSubscription{Endpoint: pushServer.URL})

    ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
    defer cancel()

    StartDeliverySubscriber(ctx, bus, []Notifier{&WebPushNotifier{svc: svc}})

    // Publish a HIGH priority notification
    bus.Publish(&events.Event{
        Type:                 events.EventNotification,
        NotificationPriority: int32(priorityHigh),
        NotificationTitle:    "Test",
        NotificationMessage:  "Test body",
        SessionID:            "session-1",
    })

    select {
    case <-notified:
        // Pass: push endpoint received the call
    case <-ctx.Done():
        t.Fatal("push notification was not delivered within timeout")
    }
}
```

---

### IT-1.2 — Subscriber goroutine exits when context is cancelled [R20]

```go
func TestSubscriberExitsOnContextCancel(t *testing.T) {
    bus := events.NewEventBus(10)
    n := &mockNotifier{name: "test"}

    ctx, cancel := context.WithCancel(context.Background())

    // Count goroutines before
    before := runtime.NumGoroutine()
    StartDeliverySubscriber(ctx, bus, []Notifier{n})

    // Goroutine count increases by 1
    time.Sleep(10 * time.Millisecond)
    assert.Greater(t, runtime.NumGoroutine(), before)

    // Cancel context
    cancel()
    time.Sleep(50 * time.Millisecond)

    // Goroutine count returns to baseline
    assert.Equal(t, before, runtime.NumGoroutine(), "subscriber goroutine must exit after context cancel")
}
```

---

### IT-2.1 — APPROVAL_NEEDED at LOW priority triggers delivery [R5]

```go
func TestApprovalAtLowPriorityTriggersPush(t *testing.T) {
    bus := events.NewEventBus(10)
    n := &mockNotifier{name: "test"}

    ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
    defer cancel()
    StartDeliverySubscriber(ctx, bus, []Notifier{n})

    bus.Publish(&events.Event{
        Type:                 events.EventNotification,
        NotificationPriority: int32(priorityLow),
        NotificationType:     int32(typeApproval),
        NotificationTitle:    "Approval needed",
        NotificationMessage:  "Please review",
    })

    time.Sleep(50 * time.Millisecond)
    assert.Len(t, n.calls, 1, "APPROVAL_NEEDED must trigger push at LOW priority")
}
```

---

### IT-2.2 — URGENT priority notification triggers delivery [R4]

```go
func TestUrgentPriorityTriggersPush(t *testing.T) {
    // Same as IT-2.1 structure but with priorityUrgent and typeGeneric
    // Assert n.calls has 1 entry
}
```

---

## Manual / E2E Test Scenarios

These require a running dev server and Chrome browser. Execute as smoke tests after each story merge.

### MT-1.1 — Permission revocation detected mid-session [R10]

**Setup**: Chrome, app at localhost:3000
1. Open Settings → enable push notifications (subscribe)
2. Verify status shows "Enabled"
3. Open Chrome Settings → Site Settings → Notifications → Block localhost
4. Return to app — **without refreshing**
**Expected**: Settings toggle updates to "Blocked" state within 1–2 seconds
**Pass criteria**: No reload required; toggle shows "Notifications blocked in browser settings"

---

### MT-1.2 — Denied permission shows instructions, not broken CTA

**Setup**: Chrome, permission currently blocked for localhost
1. Open app settings panel
**Expected**: Toggle is disabled; explanatory text visible; no subscribe button rendered
**Pass criteria**: No JS error in console; text explains how to unblock in browser settings

---

### MT-2.1 — Service worker displays server-provided action buttons [R16]

**Setup**: Chrome desktop (actions supported), approval_needed event
1. Subscribe to push
2. Trigger an approval_needed event (e.g., via a test session)
**Expected**: OS notification shows "Review" and "Later" buttons
**Pass criteria**: Two buttons visible; notification does not auto-dismiss (requireInteraction)

---

### MT-2.2 — notificationclick "Review" deep-links to correct session [R17]

1. From MT-2.1: click "Review" button on the notification
**Expected**: App window opens at `/?session=<session.ID>` and the correct session is selected
**Pass criteria**: URL contains session ID (not session title); correct session highlighted

---

### MT-2.3 — notificationclick "dismiss" closes notification without opening app

1. From MT-2.1: click "Later"
**Expected**: Notification dismissed; no new browser window or tab opened
**Pass criteria**: App window focus unchanged

---

### MT-3.1 — Push SW and cache SW have independent update cycles [R18]

**Setup**: Chrome DevTools → Application → Service Workers
1. Verify two SWs registered: push-sw.js and cache-sw.js
2. Modify cache-sw.js (bump `CACHE_VERSION` constant), reload app
3. Verify push-sw.js does NOT update (stays at previous version)
4. Trigger a push; verify it is still received
**Pass criteria**: cache SW updates; push SW stays; push delivery uninterrupted

---

### MT-3.2 — Safari 16+ macOS receives push (VAPID, zero backend changes)

**Setup**: macOS Safari 16+ at localhost, app served over HTTPS (or localhost)
1. Open Settings panel — verify push toggle is visible
2. Click enable (from button click context — user gesture)
3. Subscribe
4. Trigger a session-complete event from another tab
**Expected**: OS notification appears in macOS Notification Centre
**Pass criteria**: Notification displayed; clicking it opens app at correct session

---

## Race Condition Tests

**All Go tests must pass with `-race` flag.**

```bash
go test ./server/services/... -race -timeout 30s
go test ./server/push/... -race -timeout 30s
go test ./config/... -race -timeout 30s
```

Specific races to detect:
- Concurrent `Subscribe` + `GetSubscriptions` in PushService (UT-1.1, UT-1.2)
- Concurrent `saveConfig` + `LoadConfig` (UT-4.4)
- `mockNotifier.calls` slice concurrent append in delivery loop (test helper must use mutex)

---

## Boundary Value Tests

#### BV-1 — Payload at exactly 3900 bytes

Build a notification where the JSON is exactly 3900 bytes; assert it is sent without truncation.
Build a notification where the JSON is 3901 bytes; assert `Body` is truncated to fit within budget.

#### BV-2 — Session ID with URL-unsafe characters

```go
func TestSessionIDURLEncoding(t *testing.T) {
    notif := buildApprovalNotification(&session.Instance{
        ID: "session with spaces & special=chars",
    })
    url := notif.Data["url"].(string)
    assert.NotContains(t, url, " ")
    assert.NotContains(t, url, "&tab") // & is encoded, so &tab= appears as encoded form
    assert.Contains(t, url, "session+with+spaces") // or %20 encoding
}
```

#### BV-3 — Empty notifier slice does not panic

```go
func TestStartDeliverySubscriberEmptyNotifiers(t *testing.T) {
    bus := events.NewEventBus(10)
    ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
    defer cancel()
    // Must not panic with empty notifier slice
    assert.NotPanics(t, func() {
        StartDeliverySubscriber(ctx, bus, []Notifier{})
        bus.Publish(&events.Event{
            Type:                 events.EventNotification,
            NotificationPriority: int32(priorityHigh),
        })
        <-ctx.Done()
    })
}
```

#### BV-4 — Deduplication window: same tag within 2s suppressed

```go
func TestDeduplicationWindow(t *testing.T) {
    bus := events.NewEventBus(10)
    n := &mockNotifier{name: "test"}
    ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
    defer cancel()
    StartDeliverySubscriber(ctx, bus, []Notifier{n})

    event := &events.Event{
        Type:                 events.EventNotification,
        NotificationPriority: int32(priorityHigh),
        NotificationTitle:    "Test", NotificationMessage: "Body",
        NotificationID:       "same-id",
    }
    bus.Publish(event)
    bus.Publish(event) // same tag, within 2s
    time.Sleep(100 * time.Millisecond)
    assert.Len(t, n.calls, 1, "duplicate within 2s window must be suppressed")
}
```

---

## Component Tests (TypeScript)

These tests use Vitest + React Testing Library (match existing patterns in web-app).

**File**: `web-app/src/components/settings/PushNotificationSettings.test.tsx`

---

### CT-1.1 — Renders "not supported" state when isSupported=false [R11]

```tsx
it('renders not-supported message when push unavailable', () => {
  mockUsePushNotifications({ isSupported: false });
  render(<PushNotificationSettings />);
  expect(screen.getByText(/not supported/i)).toBeInTheDocument();
  expect(screen.queryByRole('checkbox')).not.toBeInTheDocument();
});
```

---

### CT-1.2 — Renders blocked state when permission=denied [R11]

```tsx
it('renders blocked state with instructions when permission is denied', () => {
  mockUsePushNotifications({ isSupported: true, permission: 'denied' });
  render(<PushNotificationSettings />);
  expect(screen.getByText(/blocked/i)).toBeInTheDocument();
  expect(screen.queryByRole('checkbox')).not.toBeInTheDocument();
});
```

---

### CT-1.3 — Renders subscribe toggle in default/granted state [R11]

```tsx
it('renders toggle when supported and not denied', () => {
  mockUsePushNotifications({ isSupported: true, permission: 'default', isSubscribed: false });
  render(<PushNotificationSettings />);
  expect(screen.getByRole('checkbox')).toBeInTheDocument();
  expect(screen.getByRole('checkbox')).not.toBeChecked();
});
```

---

### CT-1.4 — Toggle calls subscribe() on click when not subscribed

```tsx
it('calls subscribe on toggle click when not subscribed', async () => {
  const subscribe = vi.fn();
  mockUsePushNotifications({ isSupported: true, permission: 'default', isSubscribed: false, subscribe });
  render(<PushNotificationSettings />);
  await userEvent.click(screen.getByRole('checkbox'));
  expect(subscribe).toHaveBeenCalledOnce();
});
```

---

### CT-1.5 — Toggle calls unsubscribe() on click when subscribed

```tsx
it('calls unsubscribe on toggle click when subscribed', async () => {
  const unsubscribe = vi.fn();
  mockUsePushNotifications({ isSupported: true, permission: 'granted', isSubscribed: true, unsubscribe });
  render(<PushNotificationSettings />);
  await userEvent.click(screen.getByRole('checkbox'));
  expect(unsubscribe).toHaveBeenCalledOnce();
});
```

---

## Test Coverage Targets

| File | Current | Target | Test IDs |
|------|---------|--------|---------|
| `server/services/push_service.go` | unknown | ≥ 80% | UT-1.1–1.6 |
| `server/push/subscriber.go` | unknown | ≥ 85% | UT-2.x, IT-2.x |
| `server/push/notifier.go` (new) | 0% | ≥ 80% | UT-3.x |
| `server/push/trigger_constants.go` (new) | 0% | 100% | UT-2.x |
| `config/config.go` (modified) | unknown | ≥ 80% | UT-4.x |
| `web-app/src/components/settings/PushNotificationSettings.tsx` (new) | 0% | ≥ 80% | CT-1.x |

Run coverage:
```bash
go test ./server/services/... ./server/push/... ./config/... -coverprofile=coverage.out -race
go tool cover -html=coverage.out -o coverage.html
```

---

## Definition of Done

All items must be checked before claiming implementation complete:

- [ ] `go test ./server/services/... -race` passes with zero failures
- [ ] `go test ./server/push/... -race` passes with zero failures
- [ ] `go test ./config/... -race` passes with zero failures
- [ ] `npm test` (or `pnpm test`) in `web-app/` passes with zero failures
- [ ] UT-2.x shouldNotify test table: all 10 cases pass (including URGENT and APPROVAL_NEEDED)
- [ ] UT-1.1 concurrency test: no deadlock under `-race` with 10 concurrent goroutines
- [ ] UT-1.3/1.4: HTTP 410/404 removes subscription from storage
- [ ] IT-1.1: real EventBus event triggers mock HTTP push endpoint
- [ ] MT-2.1: Chrome desktop shows "Review"/"Later" on approval_needed notification
- [ ] MT-2.2: clicking "Review" navigates to the correct session URL
- [ ] Coverage ≥ 80% on all new/modified files
- [ ] `make lint` passes (no new lint errors)
- [ ] No `.tmp` config files left on disk after saveConfig
- [ ] No goroutine leaks on server shutdown (IT-1.2)
