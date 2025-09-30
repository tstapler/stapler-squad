# Web Server Implementation with ConnectRPC

## Epic Overview

### Goal
Expose claude-squad's session management functionality through a web interface, enabling browser-based access to all TUI features including session creation, monitoring, terminal viewing, and git diff inspection.

### Value Proposition
- **Multi-device Access**: Manage sessions from any browser without terminal access
- **Team Collaboration**: Share session views and collaborate on AI agent workflows
- **Enhanced Visibility**: Web-based dashboard with real-time updates and rich visualizations
- **Mobile Support**: Access and monitor sessions from mobile devices
- **Modern UX**: Leverage web technologies for improved user experience

### Success Metrics
- Web UI replicates 100% of TUI functionality
- Sub-100ms latency for session list operations
- Real-time terminal streaming with <50ms delay
- Support for 50+ concurrent browser sessions
- Mobile-responsive design working on iOS/Android

### Technical Approach
**ConnectRPC** as the communication layer provides:
- Type-safe, schema-driven API via Protocol Buffers
- HTTP/1.1, HTTP/2, and HTTP/3 support
- Bidirectional streaming for real-time updates
- Browser-native compatibility (no gRPC-web proxy needed)
- Excellent TypeScript code generation
- Built-in error handling and retries

---

## Story Breakdown

### Story 1: ConnectRPC Infrastructure & Core Session API (Week 1)
**Objective**: Establish ConnectRPC server with Protocol Buffer definitions and core session CRUD operations

**Value**: Foundation for all web functionality, enabling basic session management from browser

**Dependencies**: None (greenfield implementation)

---

### Story 2: Real-time Session Updates & Terminal Streaming (Week 1-2)
**Objective**: Implement bidirectional streaming for session status updates and live terminal output

**Value**: Real-time visibility into session state and terminal activity without polling

**Dependencies**: Story 1 (requires session service definitions)

---

### Story 3: Next.js Dashboard & Session Management UI (Week 2)
**Objective**: Build React-based dashboard with session list, filters, categories, and controls

**Value**: User-facing interface for browsing and managing sessions

**Dependencies**: Story 1 (requires session API), Story 2 (for real-time updates)

---

### Story 4: Terminal Viewer & Git Integration (Week 2-3)
**Objective**: Implement xterm.js-based terminal viewer and git diff display

**Value**: Core workflow features allowing terminal interaction and code review

**Dependencies**: Story 2 (terminal streaming), Story 3 (UI framework)

---

### Story 5: Advanced Features & Polish (Week 3)
**Objective**: Search, categories, session creation wizard, authentication, production hardening

**Value**: Production-ready feature parity with TUI

**Dependencies**: All previous stories

---

## Atomic Task Breakdown

---

### Story 1: ConnectRPC Infrastructure & Core Session API

#### Task 1.1: Protocol Buffer Definitions (2h - Small)

**Scope**: Define `.proto` files for session service contract

**Files**:
- `proto/session/v1/session.proto` (new)
- `proto/session/v1/types.proto` (new)
- `buf.yaml` (new) - Buf configuration
- `buf.gen.yaml` (new) - Code generation config

**Context**:
- Review `session/instance.go` for `Instance` struct fields
- Review `session/storage.go` for `InstanceData` serialization
- Understand ConnectRPC service method patterns
- Study Protocol Buffer best practices (google.api.annotations)

**Success Criteria**:
- `SessionService` with methods: ListSessions, GetSession, CreateSession, UpdateSession, DeleteSession
- Message types: Session, SessionStatus, CreateSessionRequest, UpdateSessionRequest
- Enums: SessionStatus (RUNNING, PAUSED, READY, LOADING, NEEDS_APPROVAL)
- `buf lint` passes with no errors
- `buf breaking` detects no breaking changes (first version)

**Testing**:
```bash
buf lint proto/session/v1
buf build proto/session/v1
```

**Dependencies**: None

**Implementation Notes**:
```protobuf
// proto/session/v1/session.proto
syntax = "proto3";

package session.v1;

import "google/protobuf/timestamp.proto";

service SessionService {
  rpc ListSessions(ListSessionsRequest) returns (ListSessionsResponse);
  rpc GetSession(GetSessionRequest) returns (GetSessionResponse);
  rpc CreateSession(CreateSessionRequest) returns (CreateSessionResponse);
  rpc UpdateSession(UpdateSessionRequest) returns (UpdateSessionResponse);
  rpc DeleteSession(DeleteSessionRequest) returns (DeleteSessionResponse);
}

message Session {
  string id = 1;
  string title = 2;
  string path = 3;
  string working_dir = 4;
  string branch = 5;
  SessionStatus status = 6;
  string program = 7;
  google.protobuf.Timestamp created_at = 8;
  google.protobuf.Timestamp updated_at = 9;
  string category = 10;
  bool is_expanded = 11;
  DiffStats diff_stats = 12;
}

enum SessionStatus {
  SESSION_STATUS_UNSPECIFIED = 0;
  SESSION_STATUS_RUNNING = 1;
  SESSION_STATUS_READY = 2;
  SESSION_STATUS_LOADING = 3;
  SESSION_STATUS_PAUSED = 4;
  SESSION_STATUS_NEEDS_APPROVAL = 5;
}

message DiffStats {
  int32 added = 1;
  int32 removed = 2;
  string content = 3;
}
```

---

#### Task 1.2: Go Code Generation Setup (1h - Micro)

**Scope**: Configure `buf generate` to produce Go ConnectRPC server code

**Files**:
- `buf.gen.yaml` (modify)
- `Makefile` (add generation target)
- `tools.go` (new) - Tool dependencies
- `go.mod` (update)

**Context**:
- Understand `buf generate` plugin system
- Review ConnectRPC Go plugin (`protoc-gen-connect-go`)
- Understand Go module tool dependency pattern

**Success Criteria**:
- `make proto-gen` generates Go code into `gen/proto/go/`
- Generated code includes `sessionv1connect` package
- Server stubs and client code available
- No manual modifications needed for generated code

**Testing**:
```bash
make proto-gen
go build ./gen/proto/go/...
```

**Dependencies**: Task 1.1 (requires .proto files)

**Implementation Notes**:
```yaml
# buf.gen.yaml
version: v2
plugins:
  - remote: buf.build/protocolbuffers/go
    out: gen/proto/go
    opt: paths=source_relative
  - remote: buf.build/connectrpc/go
    out: gen/proto/go
    opt: paths=source_relative
```

```makefile
# Makefile
.PHONY: proto-gen
proto-gen:
	buf generate proto
```

---

#### Task 1.3: TypeScript Code Generation Setup (1h - Micro)

**Scope**: Configure `buf generate` for TypeScript ConnectRPC client

**Files**:
- `buf.gen.yaml` (modify)
- `web/package.json` (update)
- `Makefile` (update generation target)

**Context**:
- Understand `protoc-gen-connect-es` plugin
- Review `@connectrpc/connect` npm package
- Understand ES modules in Next.js

**Success Criteria**:
- `make proto-gen` generates TypeScript code into `web/src/gen/`
- Generated code includes typed client for `SessionService`
- Type-safe request/response messages
- Compatible with Next.js app router

**Testing**:
```bash
make proto-gen
cd web && npm run type-check
```

**Dependencies**: Task 1.1 (requires .proto files)

**Implementation Notes**:
```yaml
# buf.gen.yaml (add to plugins)
  - remote: buf.build/connectrpc/es
    out: web/src/gen
    opt: target=ts
```

```json
// web/package.json
{
  "dependencies": {
    "@connectrpc/connect": "^2.3.0",
    "@connectrpc/connect-web": "^2.3.0"
  }
}
```

---

#### Task 1.4: HTTP Server Foundation (2h - Small)

**Scope**: Create HTTP server with ConnectRPC handler registration

**Files**:
- `server/server.go` (new)
- `server/middleware/cors.go` (new)
- `server/middleware/logging.go` (new)
- `main.go` (add --web flag)

**Context**:
- Review `connectrpc.com/connect` Go handler creation
- Understand `net/http` server setup
- Study CORS requirements for local development
- Review existing `main.go` flag handling

**Success Criteria**:
- HTTP server starts on `:8080` with `--web` flag
- ConnectRPC handlers registered at `/api.session.v1.SessionService/*`
- CORS middleware allows `localhost:3000` origin
- Request logging middleware captures all requests
- Graceful shutdown on SIGTERM/SIGINT

**Testing**:
```bash
go run . --web
curl -v http://localhost:8080/api.session.v1.SessionService/ListSessions
# Should return 200 with ConnectRPC response (even if empty)
```

**Dependencies**: Task 1.2 (requires generated Go code)

**Implementation Notes**:
```go
// server/server.go
package server

import (
    "context"
    "net/http"
    sessionv1 "claude-squad/gen/proto/go/session/v1"
    "connectrpc.com/connect"
)

type Server struct {
    sessionService sessionv1.SessionServiceHandler
}

func NewServer(sessionService sessionv1.SessionServiceHandler) *Server {
    return &Server{sessionService: sessionService}
}

func (s *Server) Start(ctx context.Context, addr string) error {
    mux := http.NewServeMux()

    // Register ConnectRPC handlers
    path, handler := sessionv1.NewSessionServiceHandler(s.sessionService)
    mux.Handle(path, handler)

    // Apply middleware
    handler := corsMiddleware(loggingMiddleware(mux))

    server := &http.Server{
        Addr:    addr,
        Handler: handler,
    }

    // Graceful shutdown
    go func() {
        <-ctx.Done()
        server.Shutdown(context.Background())
    }()

    return server.ListenAndServe()
}
```

---

#### Task 1.5: Session Service Implementation - List & Get (3h - Medium)

**Scope**: Implement ListSessions and GetSession RPC methods

**Files**:
- `server/services/session_service.go` (new)
- `server/adapters/instance_adapter.go` (new)
- `session/storage.go` (no changes, read only)

**Context**:
- Review `session.Storage.LoadInstances()` method
- Understand `session.Instance` to proto `Session` mapping
- Study ConnectRPC error handling patterns
- Review ConnectRPC interceptor patterns

**Success Criteria**:
- `ListSessions` returns all sessions from storage
- `GetSession` returns single session by ID (title as ID)
- Proper error codes: NotFound for missing sessions
- Instance-to-protobuf conversion helper function
- Unit tests for conversion logic
- Integration test calling RPC methods

**Testing**:
```bash
go test ./server/services -v
# Integration test with storage mock
```

**Dependencies**: Task 1.4 (requires server foundation)

**Implementation Notes**:
```go
// server/services/session_service.go
package services

import (
    "context"
    "claude-squad/session"
    sessionv1 "claude-squad/gen/proto/go/session/v1"
    "connectrpc.com/connect"
)

type SessionService struct {
    storage *session.Storage
}

func (s *SessionService) ListSessions(
    ctx context.Context,
    req *connect.Request[sessionv1.ListSessionsRequest],
) (*connect.Response[sessionv1.ListSessionsResponse], error) {
    instances, err := s.storage.LoadInstances()
    if err != nil {
        return nil, connect.NewError(connect.CodeInternal, err)
    }

    sessions := make([]*sessionv1.Session, len(instances))
    for i, inst := range instances {
        sessions[i] = toProtoSession(inst)
    }

    return connect.NewResponse(&sessionv1.ListSessionsResponse{
        Sessions: sessions,
    }), nil
}

func (s *SessionService) GetSession(
    ctx context.Context,
    req *connect.Request[sessionv1.GetSessionRequest],
) (*connect.Response[sessionv1.GetSessionResponse], error) {
    instances, err := s.storage.LoadInstances()
    if err != nil {
        return nil, connect.NewError(connect.CodeInternal, err)
    }

    for _, inst := range instances {
        if inst.Title == req.Msg.Id {
            return connect.NewResponse(&sessionv1.GetSessionResponse{
                Session: toProtoSession(inst),
            }), nil
        }
    }

    return nil, connect.NewError(connect.CodeNotFound,
        fmt.Errorf("session not found: %s", req.Msg.Id))
}
```

---

#### Task 1.6: Session Service Implementation - Create (4h - Large)

**Scope**: Implement CreateSession RPC with full session initialization

**Files**:
- `server/services/session_service.go` (modify)
- `server/services/session_manager.go` (new)
- `session/instance.go` (no changes, read only)

**Context**:
- Review `session.Instance.Start()` method and initialization flow
- Understand git worktree creation process
- Study tmux session lifecycle
- Review async session creation pattern in `app/handleAdvancedSessionSetup.go`
- Understand `session.Storage.SaveInstance()` method

**Success Criteria**:
- `CreateSession` validates input parameters
- Creates `session.Instance` with proper initialization
- Calls `Start()` method to initialize tmux and git worktree
- Saves instance to storage
- Returns created session with generated ID
- Proper error handling for invalid paths, git errors
- Unit tests for validation logic
- Integration test creating real session (with cleanup)

**Testing**:
```bash
go test ./server/services -v -run TestCreateSession
# Should create temp directory, initialize session, clean up
```

**Dependencies**: Task 1.5 (requires service foundation)

**Implementation Notes**:
```go
func (s *SessionService) CreateSession(
    ctx context.Context,
    req *connect.Request[sessionv1.CreateSessionRequest],
) (*connect.Response[sessionv1.CreateSessionResponse], error) {
    // Validate request
    if req.Msg.Title == "" {
        return nil, connect.NewError(connect.CodeInvalidArgument,
            errors.New("title is required"))
    }

    // Create instance
    instance := &session.Instance{
        Title:      req.Msg.Title,
        Path:       req.Msg.Path,
        WorkingDir: req.Msg.WorkingDir,
        Branch:     req.Msg.Branch,
        Program:    req.Msg.Program,
        Category:   req.Msg.Category,
        Status:     session.Loading,
        CreatedAt:  time.Now(),
        UpdatedAt:  time.Now(),
    }

    // Start session (initializes tmux + git)
    if err := instance.Start(); err != nil {
        return nil, connect.NewError(connect.CodeInternal, err)
    }

    // Save to storage
    if err := s.storage.SaveInstance(instance); err != nil {
        instance.Stop() // Cleanup on save failure
        return nil, connect.NewError(connect.CodeInternal, err)
    }

    return connect.NewResponse(&sessionv1.CreateSessionResponse{
        Session: toProtoSession(instance),
    }), nil
}
```

---

#### Task 1.7: Session Service Implementation - Update & Delete (3h - Medium)

**Scope**: Implement UpdateSession (pause/resume) and DeleteSession RPCs

**Files**:
- `server/services/session_service.go` (modify)
- `session/instance.go` (no changes, read only)

**Context**:
- Review `session.Instance.Pause()` and `Resume()` methods
- Review `session.Instance.Stop()` cleanup logic
- Understand `session.Storage.DeleteInstance()` method
- Study partial update patterns in Protocol Buffers

**Success Criteria**:
- `UpdateSession` supports status changes (pause/resume)
- Validates status transitions (can't resume stopped session)
- `DeleteSession` stops session and removes from storage
- Cleans up tmux sessions and git worktrees
- Proper error handling for invalid transitions
- Unit tests for all state transitions
- Integration tests with real sessions

**Testing**:
```bash
go test ./server/services -v -run TestUpdateSession
go test ./server/services -v -run TestDeleteSession
```

**Dependencies**: Task 1.6 (requires create functionality)

**Implementation Notes**:
```go
func (s *SessionService) UpdateSession(
    ctx context.Context,
    req *connect.Request[sessionv1.UpdateSessionRequest],
) (*connect.Response[sessionv1.UpdateSessionResponse], error) {
    instances, err := s.storage.LoadInstances()
    if err != nil {
        return nil, connect.NewError(connect.CodeInternal, err)
    }

    var instance *session.Instance
    for _, inst := range instances {
        if inst.Title == req.Msg.Id {
            instance = inst
            break
        }
    }

    if instance == nil {
        return nil, connect.NewError(connect.CodeNotFound,
            fmt.Errorf("session not found: %s", req.Msg.Id))
    }

    // Handle status change
    if req.Msg.Status == sessionv1.SessionStatus_SESSION_STATUS_PAUSED {
        if err := instance.Pause(); err != nil {
            return nil, connect.NewError(connect.CodeInternal, err)
        }
    } else if req.Msg.Status == sessionv1.SessionStatus_SESSION_STATUS_RUNNING {
        if err := instance.Resume(); err != nil {
            return nil, connect.NewError(connect.CodeInternal, err)
        }
    }

    if err := s.storage.SaveInstance(instance); err != nil {
        return nil, connect.NewError(connect.CodeInternal, err)
    }

    return connect.NewResponse(&sessionv1.UpdateSessionResponse{
        Session: toProtoSession(instance),
    }), nil
}
```

---

### Story 2: Real-time Session Updates & Terminal Streaming

#### Task 2.1: Session Event Protocol Buffer Definitions (2h - Small)

**Scope**: Define streaming RPC methods and event messages

**Files**:
- `proto/session/v1/session.proto` (modify)
- `proto/session/v1/events.proto` (new)
- `buf.gen.yaml` (regenerate)

**Context**:
- Understand ConnectRPC server-streaming patterns
- Review Protocol Buffer `oneof` for event types
- Study session state change events in `session/instance.go`

**Success Criteria**:
- `WatchSessions` server-streaming RPC defined
- `StreamTerminal` bidirectional streaming RPC defined
- `SessionEvent` message with `oneof` event types
- Event types: SessionCreated, SessionUpdated, SessionDeleted, StatusChanged
- Terminal event types: TerminalOutput, TerminalInput, TerminalResize
- Code regeneration successful

**Testing**:
```bash
make proto-gen
go build ./gen/proto/go/...
cd web && npm run type-check
```

**Dependencies**: Task 1.1 (extends proto definitions)

**Implementation Notes**:
```protobuf
// proto/session/v1/session.proto (add to SessionService)
service SessionService {
  // ... existing methods ...

  rpc WatchSessions(WatchSessionsRequest) returns (stream SessionEvent);
  rpc StreamTerminal(stream TerminalMessage) returns (stream TerminalMessage);
}

// proto/session/v1/events.proto
message SessionEvent {
  google.protobuf.Timestamp timestamp = 1;

  oneof event {
    SessionCreated session_created = 2;
    SessionUpdated session_updated = 3;
    SessionDeleted session_deleted = 4;
    SessionStatusChanged status_changed = 5;
  }
}

message SessionCreated {
  Session session = 1;
}

message SessionUpdated {
  Session session = 1;
}

message SessionDeleted {
  string session_id = 1;
}

message SessionStatusChanged {
  string session_id = 1;
  SessionStatus old_status = 2;
  SessionStatus new_status = 3;
}

message TerminalMessage {
  string session_id = 1;

  oneof message {
    TerminalOutput output = 2;
    TerminalInput input = 3;
    TerminalResize resize = 4;
  }
}

message TerminalOutput {
  bytes data = 1;
}

message TerminalInput {
  bytes data = 1;
}

message TerminalResize {
  int32 cols = 1;
  int32 rows = 2;
}
```

---

#### Task 2.2: Event Bus Implementation (3h - Medium)

**Scope**: Create pub/sub event bus for session state changes

**Files**:
- `server/events/bus.go` (new)
- `server/events/subscriber.go` (new)
- `server/events/types.go` (new)

**Context**:
- Review Go channel patterns for pub/sub
- Understand sync.Mutex for thread-safe subscriber management
- Study context cancellation for cleanup
- No external dependencies (use channels)

**Success Criteria**:
- `EventBus` with Subscribe/Unsubscribe methods
- Publish broadcasts to all subscribers
- Subscriber channels buffered to prevent blocking
- Context-based cleanup for canceled subscriptions
- Thread-safe subscriber management
- Unit tests for concurrent publish/subscribe
- Benchmark tests for throughput

**Testing**:
```bash
go test ./server/events -v -race
go test ./server/events -bench=. -benchmem
```

**Dependencies**: None (independent component)

**Implementation Notes**:
```go
// server/events/bus.go
package events

import (
    "context"
    "sync"
)

type EventBus struct {
    mu          sync.RWMutex
    subscribers map[string]chan interface{}
    bufferSize  int
}

func NewEventBus(bufferSize int) *EventBus {
    return &EventBus{
        subscribers: make(map[string]chan interface{}),
        bufferSize:  bufferSize,
    }
}

func (eb *EventBus) Subscribe(ctx context.Context) (<-chan interface{}, string) {
    ch := make(chan interface{}, eb.bufferSize)
    id := generateSubscriberID()

    eb.mu.Lock()
    eb.subscribers[id] = ch
    eb.mu.Unlock()

    // Cleanup on context cancellation
    go func() {
        <-ctx.Done()
        eb.Unsubscribe(id)
    }()

    return ch, id
}

func (eb *EventBus) Publish(event interface{}) {
    eb.mu.RLock()
    defer eb.mu.RUnlock()

    for _, ch := range eb.subscribers {
        select {
        case ch <- event:
        default:
            // Drop event if subscriber is slow
        }
    }
}

func (eb *EventBus) Unsubscribe(id string) {
    eb.mu.Lock()
    defer eb.mu.Unlock()

    if ch, exists := eb.subscribers[id]; exists {
        close(ch)
        delete(eb.subscribers, id)
    }
}
```

---

#### Task 2.3: WatchSessions Streaming Implementation (3h - Medium)

**Scope**: Implement server-streaming RPC for real-time session events

**Files**:
- `server/services/session_service.go` (modify)
- `server/services/session_watcher.go` (new)

**Context**:
- Review ConnectRPC server-streaming patterns
- Understand `connect.ServerStream` interface
- Study event bus subscription lifecycle
- Review proto `SessionEvent` message structure

**Success Criteria**:
- `WatchSessions` streams events to connected clients
- Sends initial snapshot of all sessions on connect
- Subscribes to event bus for real-time updates
- Converts Go events to proto events
- Handles client disconnection gracefully
- Cleans up subscriptions on stream close
- Integration test with multiple concurrent clients

**Testing**:
```bash
go test ./server/services -v -run TestWatchSessions
# Test with mock event bus, multiple clients
```

**Dependencies**: Task 2.2 (requires event bus), Task 2.1 (requires proto definitions)

**Implementation Notes**:
```go
func (s *SessionService) WatchSessions(
    ctx context.Context,
    req *connect.Request[sessionv1.WatchSessionsRequest],
    stream *connect.ServerStream[sessionv1.SessionEvent],
) error {
    // Send initial snapshot
    instances, err := s.storage.LoadInstances()
    if err != nil {
        return connect.NewError(connect.CodeInternal, err)
    }

    for _, inst := range instances {
        event := &sessionv1.SessionEvent{
            Timestamp: timestamppb.Now(),
            Event: &sessionv1.SessionEvent_SessionCreated{
                SessionCreated: &sessionv1.SessionCreated{
                    Session: toProtoSession(inst),
                },
            },
        }
        if err := stream.Send(event); err != nil {
            return err
        }
    }

    // Subscribe to real-time events
    eventCh, subID := s.eventBus.Subscribe(ctx)
    defer s.eventBus.Unsubscribe(subID)

    for {
        select {
        case <-ctx.Done():
            return nil
        case event := <-eventCh:
            protoEvent := toProtoEvent(event)
            if err := stream.Send(protoEvent); err != nil {
                return err
            }
        }
    }
}
```

---

#### Task 2.4: Terminal PTY Output Streaming (4h - Large)

**Scope**: Stream tmux PTY output to browser via ConnectRPC

**Files**:
- `server/services/terminal_streamer.go` (new)
- `session/tmux/tmux.go` (add reader method)
- `server/services/session_service.go` (implement StreamTerminal)

**Context**:
- Review `session/tmux/tmux.go` PTY management
- Understand `os.File.Read()` for PTY output
- Study ConnectRPC bidirectional streaming
- Review terminal escape sequence handling

**Success Criteria**:
- `StreamTerminal` sends PTY output as `TerminalOutput` events
- Handles PTY resize events from client
- Forwards client input to PTY (for interactive mode)
- Buffers output efficiently (1KB chunks)
- Handles tmux session detach/reattach
- Graceful cleanup on stream close
- Integration test with mock PTY

**Testing**:
```bash
go test ./server/services -v -run TestStreamTerminal
# Test with mock PTY, verify buffering and streaming
```

**Dependencies**: Task 2.1 (requires terminal proto), Task 1.5 (requires session retrieval)

**Implementation Notes**:
```go
// server/services/terminal_streamer.go
package services

import (
    "context"
    "io"
    sessionv1 "claude-squad/gen/proto/go/session/v1"
    "connectrpc.com/connect"
)

func (s *SessionService) StreamTerminal(
    ctx context.Context,
    stream *connect.BidiStream[sessionv1.TerminalMessage, sessionv1.TerminalMessage],
) error {
    // Get first message to determine session
    msg, err := stream.Receive()
    if err != nil {
        return err
    }

    sessionID := msg.SessionId
    instance := s.findInstance(sessionID)
    if instance == nil {
        return connect.NewError(connect.CodeNotFound,
            fmt.Errorf("session not found: %s", sessionID))
    }

    // Get PTY reader
    ptyReader, err := instance.GetPTYReader()
    if err != nil {
        return connect.NewError(connect.CodeInternal, err)
    }

    // Stream PTY output to client
    go func() {
        buf := make([]byte, 1024)
        for {
            n, err := ptyReader.Read(buf)
            if err != nil {
                if err != io.EOF {
                    log.Printf("PTY read error: %v", err)
                }
                return
            }

            outputMsg := &sessionv1.TerminalMessage{
                SessionId: sessionID,
                Message: &sessionv1.TerminalMessage_Output{
                    Output: &sessionv1.TerminalOutput{
                        Data: buf[:n],
                    },
                },
            }

            if err := stream.Send(outputMsg); err != nil {
                return
            }
        }
    }()

    // Handle client input and resize
    for {
        msg, err := stream.Receive()
        if err != nil {
            return err
        }

        switch m := msg.Message.(type) {
        case *sessionv1.TerminalMessage_Input:
            instance.WriteInput(m.Input.Data)
        case *sessionv1.TerminalMessage_Resize:
            instance.Resize(int(m.Resize.Cols), int(m.Resize.Rows))
        }
    }
}
```

---

#### Task 2.5: Storage Event Integration (2h - Small)

**Scope**: Publish events from session storage operations

**Files**:
- `session/storage.go` (modify)
- `server/events/session_events.go` (new)

**Context**:
- Review `session.Storage.SaveInstance()` and `DeleteInstance()` methods
- Understand event bus publish patterns
- Study Go struct composition for optional event bus

**Success Criteria**:
- `Storage` accepts optional `EventBus` in constructor
- `SaveInstance` publishes SessionCreated/SessionUpdated events
- `DeleteInstance` publishes SessionDeleted events
- Backward compatible (event bus optional)
- No changes to TUI usage of Storage
- Unit tests verify event publication

**Testing**:
```bash
go test ./session -v -run TestStorageEvents
# Test with mock event bus
```

**Dependencies**: Task 2.2 (requires event bus), Task 1.5 (modifies storage)

**Implementation Notes**:
```go
// session/storage.go (add field)
type Storage struct {
    state        config.InstanceStorage
    stateService *config.StateService
    eventBus     *events.EventBus // Optional
}

func NewStorageWithEvents(state config.InstanceStorage, eventBus *events.EventBus) (*Storage, error) {
    // ... existing initialization ...
    return &Storage{
        state:        state,
        stateService: stateService,
        eventBus:     eventBus,
    }, nil
}

func (s *Storage) SaveInstance(instance *Instance) error {
    // Check if instance exists
    exists := s.instanceExists(instance.Title)

    // ... existing save logic ...

    // Publish event
    if s.eventBus != nil {
        if exists {
            s.eventBus.Publish(events.SessionUpdated{Instance: instance})
        } else {
            s.eventBus.Publish(events.SessionCreated{Instance: instance})
        }
    }

    return nil
}
```

---

### Story 3: Next.js Dashboard & Session Management UI

#### Task 3.1: ConnectRPC Client Setup (2h - Small)

**Scope**: Configure ConnectRPC transport and clients in Next.js

**Files**:
- `web/src/lib/api/transport.ts` (new)
- `web/src/lib/api/clients.ts` (new)
- `web/.env.local` (new)
- `web/next.config.ts` (modify - API proxy)

**Context**:
- Review `@connectrpc/connect-web` transport options
- Understand Next.js API routes for proxying
- Study environment variable configuration
- Review generated TypeScript client from Task 1.3

**Success Criteria**:
- `createConnectTransport()` configured for browser
- `sessionClient` singleton for API calls
- Environment variable for API base URL
- Next.js API proxy to avoid CORS in development
- Type-safe client calls with generated types
- Error handling and retry configuration

**Testing**:
```bash
cd web && npm run dev
# Browser console should show successful API connection
```

**Dependencies**: Task 1.3 (requires generated TypeScript), Task 1.4 (requires running server)

**Implementation Notes**:
```typescript
// web/src/lib/api/transport.ts
import { createConnectTransport } from "@connectrpc/connect-web";

export const transport = createConnectTransport({
  baseUrl: process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080",
  useBinaryFormat: false, // Use JSON for easier debugging
});

// web/src/lib/api/clients.ts
import { createPromiseClient } from "@connectrpc/connect";
import { SessionService } from "@/gen/session/v1/session_connect";
import { transport } from "./transport";

export const sessionClient = createPromiseClient(SessionService, transport);
```

---

#### Task 3.2: React Query Setup & Session Hooks (3h - Medium)

**Scope**: Configure React Query with hooks for session operations

**Files**:
- `web/src/lib/query-client.ts` (new)
- `web/src/app/providers.tsx` (new)
- `web/src/hooks/use-sessions.ts` (new)
- `web/src/hooks/use-session-mutations.ts` (new)
- `web/src/app/layout.tsx` (modify)

**Context**:
- Review React Query setup patterns
- Understand query invalidation strategies
- Study optimistic updates for mutations
- Review ConnectRPC client usage from Task 3.1

**Success Criteria**:
- `QueryClientProvider` configured in app layout
- `useSessions()` hook for listing sessions
- `useSession(id)` hook for individual session
- `useCreateSession()`, `useUpdateSession()`, `useDeleteSession()` mutation hooks
- Automatic cache invalidation on mutations
- Optimistic updates for pause/resume
- Error handling with toast notifications
- Loading states for all operations

**Testing**:
```bash
cd web && npm run dev
# Browser console should show React Query dev tools
```

**Dependencies**: Task 3.1 (requires client setup)

**Implementation Notes**:
```typescript
// web/src/hooks/use-sessions.ts
import { useQuery } from "@tanstack/react-query";
import { sessionClient } from "@/lib/api/clients";

export function useSessions() {
  return useQuery({
    queryKey: ["sessions"],
    queryFn: async () => {
      const response = await sessionClient.listSessions({});
      return response.sessions;
    },
    refetchInterval: 5000, // Poll every 5s (will be replaced by streaming)
  });
}

// web/src/hooks/use-session-mutations.ts
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { sessionClient } from "@/lib/api/clients";
import { toast } from "sonner";

export function useCreateSession() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (req: CreateSessionRequest) => {
      return await sessionClient.createSession(req);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["sessions"] });
      toast.success("Session created successfully");
    },
    onError: (error) => {
      toast.error(`Failed to create session: ${error.message}`);
    },
  });
}
```

---

#### Task 3.3: Session List Component (3h - Medium)

**Scope**: Build session list with status indicators and actions

**Files**:
- `web/src/app/dashboard/page.tsx` (new)
- `web/src/components/sessions/SessionList.tsx` (new)
- `web/src/components/sessions/SessionCard.tsx` (new)
- `web/src/components/sessions/SessionStatus.tsx` (new)

**Context**:
- Review `ui/list.go` TUI implementation for feature parity
- Study session status rendering logic
- Review Next.js App Router layout patterns
- Understand React component composition

**Success Criteria**:
- Dashboard page at `/dashboard` route
- SessionList displays all sessions from `useSessions()` hook
- SessionCard shows title, path, branch, status, diff stats
- Status indicator with colors (green=running, gray=paused, orange=needs approval)
- Hover actions: pause/resume, delete
- Empty state when no sessions exist
- Loading skeleton while fetching
- Responsive grid layout

**Testing**:
```bash
cd web && npm run dev
# Navigate to http://localhost:3000/dashboard
```

**Dependencies**: Task 3.2 (requires React Query hooks)

**Implementation Notes**:
```tsx
// web/src/components/sessions/SessionCard.tsx
import { Session, SessionStatus } from "@/gen/session/v1/session_pb";
import { useUpdateSession, useDeleteSession } from "@/hooks/use-session-mutations";

export function SessionCard({ session }: { session: Session }) {
  const updateSession = useUpdateSession();
  const deleteSession = useDeleteSession();

  const isPaused = session.status === SessionStatus.SESSION_STATUS_PAUSED;

  const handleTogglePause = () => {
    updateSession.mutate({
      id: session.id,
      status: isPaused
        ? SessionStatus.SESSION_STATUS_RUNNING
        : SessionStatus.SESSION_STATUS_PAUSED,
    });
  };

  return (
    <div className="border rounded-lg p-4 hover:shadow-md transition">
      <div className="flex justify-between items-start">
        <div>
          <h3 className="font-semibold">{session.title}</h3>
          <p className="text-sm text-gray-500">{session.path}</p>
          <p className="text-xs text-gray-400">Branch: {session.branch}</p>
        </div>
        <SessionStatus status={session.status} />
      </div>

      {session.diffStats && (
        <div className="mt-2 text-sm">
          <span className="text-green-600">+{session.diffStats.added}</span>
          {" / "}
          <span className="text-red-600">-{session.diffStats.removed}</span>
        </div>
      )}

      <div className="mt-4 flex gap-2">
        <button
          onClick={handleTogglePause}
          disabled={updateSession.isPending}
          className="btn-secondary"
        >
          {isPaused ? "Resume" : "Pause"}
        </button>
        <button
          onClick={() => deleteSession.mutate({ id: session.id })}
          disabled={deleteSession.isPending}
          className="btn-danger"
        >
          Delete
        </button>
      </div>
    </div>
  );
}
```

---

#### Task 3.4: Category Organization UI (2h - Small)

**Scope**: Add category grouping with expand/collapse

**Files**:
- `web/src/components/sessions/SessionList.tsx` (modify)
- `web/src/components/sessions/CategoryGroup.tsx` (new)
- `web/src/lib/utils/group-sessions.ts` (new)

**Context**:
- Review TUI category grouping in `ui/list.go`
- Study accordion component patterns
- Understand session category field usage

**Success Criteria**:
- Sessions grouped by category (or "Uncategorized")
- Category headers with expand/collapse icons
- Persists expanded state in localStorage
- Shows session count per category
- Smooth expand/collapse animation
- Keyboard navigation support

**Testing**:
```bash
cd web && npm run dev
# Verify category groups collapse/expand, state persists on reload
```

**Dependencies**: Task 3.3 (modifies session list)

**Implementation Notes**:
```typescript
// web/src/lib/utils/group-sessions.ts
import { Session } from "@/gen/session/v1/session_pb";

export function groupSessionsByCategory(sessions: Session[]) {
  const groups = new Map<string, Session[]>();

  for (const session of sessions) {
    const category = session.category || "Uncategorized";
    if (!groups.has(category)) {
      groups.set(category, []);
    }
    groups.get(category)!.push(session);
  }

  return Array.from(groups.entries()).map(([category, sessions]) => ({
    category,
    sessions,
    count: sessions.length,
  }));
}
```

---

#### Task 3.5: Search & Filter UI (3h - Medium)

**Scope**: Implement fuzzy search and paused session filter

**Files**:
- `web/src/components/sessions/SearchBar.tsx` (new)
- `web/src/components/sessions/FilterBar.tsx` (new)
- `web/src/lib/utils/fuzzy-search.ts` (new)
- `web/src/app/dashboard/page.tsx` (modify)

**Context**:
- Review TUI search implementation in `ui/list.go`
- Study `sahilm/fuzzy` algorithm
- Understand React state for filters
- Review debounce patterns for search input

**Success Criteria**:
- Search bar with debounced input (300ms)
- Fuzzy matching on title, path, branch, working directory
- "Hide Paused" toggle filter
- Filter state persisted in URL query params
- Highlight matched text in results
- Clear search button when active
- Filter shows result count

**Testing**:
```bash
cd web && npm run dev
# Type in search, verify fuzzy matching works
# Toggle hide paused, verify filtered list
```

**Dependencies**: Task 3.4 (requires session list with categories)

**Implementation Notes**:
```typescript
// web/src/lib/utils/fuzzy-search.ts
import { Session } from "@/gen/session/v1/session_pb";
import Fuse from "fuse.js";

const fuseOptions = {
  keys: ["title", "path", "branch", "workingDir"],
  threshold: 0.3,
  includeMatches: true,
};

export function fuzzySearchSessions(sessions: Session[], query: string) {
  if (!query) return sessions;

  const fuse = new Fuse(sessions, fuseOptions);
  const results = fuse.search(query);

  return results.map((result) => ({
    session: result.item,
    matches: result.matches,
  }));
}
```

---

#### Task 3.6: Session Creation Modal (4h - Large)

**Scope**: Build multi-step session creation wizard

**Files**:
- `web/src/components/sessions/CreateSessionModal.tsx` (new)
- `web/src/components/sessions/CreateSessionForm.tsx` (new)
- `web/src/components/sessions/PathBrowser.tsx` (new)
- `web/src/app/dashboard/page.tsx` (modify - add "New Session" button)

**Context**:
- Review TUI session setup in `app/handleAdvancedSessionSetup.go`
- Study `ui/overlay/sessionSetup.go` wizard flow
- Understand form validation patterns
- Review Modal/Dialog component patterns

**Success Criteria**:
- Modal triggered by "New Session" button
- Form fields: title, path (with browser), category, program
- Path browser with directory navigation
- Branch selection for git repositories
- Form validation (required fields, valid paths)
- Loading state during creation
- Success feedback and automatic close
- Error handling with retry

**Testing**:
```bash
cd web && npm run dev
# Click "New Session", fill form, create session
# Verify new session appears in list
```

**Dependencies**: Task 3.2 (requires mutation hooks)

**Implementation Notes**:
```tsx
// web/src/components/sessions/CreateSessionModal.tsx
import { useState } from "react";
import { useCreateSession } from "@/hooks/use-session-mutations";
import { Dialog } from "@/components/ui/dialog";

export function CreateSessionModal({
  open,
  onClose
}: {
  open: boolean;
  onClose: () => void;
}) {
  const [formData, setFormData] = useState({
    title: "",
    path: "",
    category: "",
    program: "claude",
  });

  const createSession = useCreateSession();

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();

    try {
      await createSession.mutateAsync(formData);
      onClose();
    } catch (error) {
      // Error handling via toast in mutation hook
    }
  };

  return (
    <Dialog open={open} onClose={onClose}>
      <form onSubmit={handleSubmit} className="space-y-4">
        <h2 className="text-2xl font-bold">Create New Session</h2>

        <div>
          <label htmlFor="title">Session Title</label>
          <input
            id="title"
            value={formData.title}
            onChange={(e) => setFormData({ ...formData, title: e.target.value })}
            required
          />
        </div>

        <div>
          <label htmlFor="path">Working Directory</label>
          <input
            id="path"
            value={formData.path}
            onChange={(e) => setFormData({ ...formData, path: e.target.value })}
            required
          />
          {/* Path browser component */}
        </div>

        <div>
          <label htmlFor="category">Category (optional)</label>
          <input
            id="category"
            value={formData.category}
            onChange={(e) => setFormData({ ...formData, category: e.target.value })}
          />
        </div>

        <div className="flex justify-end gap-2">
          <button type="button" onClick={onClose} className="btn-secondary">
            Cancel
          </button>
          <button
            type="submit"
            disabled={createSession.isPending}
            className="btn-primary"
          >
            {createSession.isPending ? "Creating..." : "Create Session"}
          </button>
        </div>
      </form>
    </Dialog>
  );
}
```

---

### Story 4: Terminal Viewer & Git Integration

#### Task 4.1: xterm.js Integration (3h - Medium)

**Scope**: Set up xterm.js terminal emulator component

**Files**:
- `web/src/components/terminal/Terminal.tsx` (new)
- `web/src/components/terminal/use-terminal.ts` (new)
- `web/package.json` (add xterm dependencies)

**Context**:
- Review xterm.js documentation and API
- Study xterm addons (fit, webgl)
- Understand React ref patterns for DOM integration
- Review terminal theming options

**Success Criteria**:
- Terminal component renders xterm.js instance
- Fit addon resizes terminal to container
- WebGL addon for performance (optional)
- Dark/light theme support
- Configurable font size and family
- Proper cleanup on unmount
- Keyboard input handling

**Testing**:
```bash
cd web && npm run dev
# Terminal component displays with blinking cursor
```

**Dependencies**: None (independent component)

**Implementation Notes**:
```typescript
// web/src/components/terminal/Terminal.tsx
import { useEffect, useRef } from "react";
import { Terminal as XTerm } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import "@xterm/xterm/css/xterm.css";

export function Terminal({
  onData,
  onResize,
}: {
  onData?: (data: string) => void;
  onResize?: (cols: number, rows: number) => void;
}) {
  const terminalRef = useRef<HTMLDivElement>(null);
  const xtermRef = useRef<XTerm>();
  const fitAddonRef = useRef<FitAddon>();

  useEffect(() => {
    if (!terminalRef.current) return;

    const xterm = new XTerm({
      cursorBlink: true,
      fontSize: 14,
      fontFamily: "Menlo, Monaco, 'Courier New', monospace",
    });

    const fitAddon = new FitAddon();
    xterm.loadAddon(fitAddon);

    xterm.open(terminalRef.current);
    fitAddon.fit();

    // Handle user input
    xterm.onData((data) => {
      onData?.(data);
    });

    // Handle resize
    xterm.onResize((size) => {
      onResize?.(size.cols, size.rows);
    });

    xtermRef.current = xterm;
    fitAddonRef.current = fitAddon;

    // Resize observer
    const resizeObserver = new ResizeObserver(() => {
      fitAddon.fit();
    });
    resizeObserver.observe(terminalRef.current);

    return () => {
      resizeObserver.disconnect();
      xterm.dispose();
    };
  }, [onData, onResize]);

  // Expose write method to parent
  useEffect(() => {
    if (xtermRef.current) {
      // Store ref in parent component state or context
    }
  }, []);

  return <div ref={terminalRef} className="w-full h-full" />;
}
```

---

#### Task 4.2: ConnectRPC Terminal Streaming Client (3h - Medium)

**Scope**: Connect xterm.js to StreamTerminal RPC

**Files**:
- `web/src/hooks/use-terminal-stream.ts` (new)
- `web/src/components/terminal/Terminal.tsx` (modify)

**Context**:
- Review Task 2.4 StreamTerminal server implementation
- Study ConnectRPC bidirectional streaming client patterns
- Understand Uint8Array/text encoding for terminal data
- Review terminal resize message flow

**Success Criteria**:
- `useTerminalStream(sessionId)` hook manages streaming
- Receives terminal output and writes to xterm.js
- Sends user input to server
- Sends resize events on terminal dimension change
- Handles stream errors and reconnection
- Automatic cleanup on unmount
- Loading state while connecting

**Testing**:
```bash
cd web && npm run dev
# Click session, see live terminal output
# Type in terminal, verify input sent to server
```

**Dependencies**: Task 4.1 (requires Terminal component), Task 2.4 (requires StreamTerminal RPC)

**Implementation Notes**:
```typescript
// web/src/hooks/use-terminal-stream.ts
import { useEffect, useRef, useState } from "react";
import { createPromiseClient } from "@connectrpc/connect";
import { SessionService } from "@/gen/session/v1/session_connect";
import { transport } from "@/lib/api/transport";
import { TerminalMessage } from "@/gen/session/v1/session_pb";

export function useTerminalStream(sessionId: string) {
  const [connected, setConnected] = useState(false);
  const [error, setError] = useState<Error | null>(null);
  const streamRef = useRef<any>(null);
  const writeCallbackRef = useRef<((data: string) => void) | null>(null);

  useEffect(() => {
    const client = createPromiseClient(SessionService, transport);

    const startStream = async () => {
      try {
        const stream = client.streamTerminal();
        streamRef.current = stream;

        // Send initial message with session ID
        await stream.send({
          sessionId,
          message: { case: "output", value: { data: new Uint8Array() } },
        });

        setConnected(true);

        // Receive terminal output
        for await (const message of stream) {
          if (message.message.case === "output") {
            const text = new TextDecoder().decode(message.message.value.data);
            writeCallbackRef.current?.(text);
          }
        }
      } catch (err) {
        setError(err as Error);
        setConnected(false);
      }
    };

    startStream();

    return () => {
      streamRef.current?.close();
    };
  }, [sessionId]);

  const sendInput = (data: string) => {
    if (!streamRef.current) return;

    streamRef.current.send({
      sessionId,
      message: {
        case: "input",
        value: { data: new TextEncoder().encode(data) },
      },
    });
  };

  const sendResize = (cols: number, rows: number) => {
    if (!streamRef.current) return;

    streamRef.current.send({
      sessionId,
      message: {
        case: "resize",
        value: { cols, rows },
      },
    });
  };

  const registerWriteCallback = (callback: (data: string) => void) => {
    writeCallbackRef.current = callback;
  };

  return {
    connected,
    error,
    sendInput,
    sendResize,
    registerWriteCallback,
  };
}
```

---

#### Task 4.3: Git Diff Viewer Component (3h - Medium)

**Scope**: Display git diffs with syntax highlighting

**Files**:
- `web/src/components/git/DiffViewer.tsx` (new)
- `web/src/components/git/DiffStats.tsx` (new)
- `web/src/hooks/use-session-diff.ts` (new)

**Context**:
- Review git diff format (unified diff)
- Study diff syntax highlighting libraries (react-diff-view)
- Understand diff stats calculation
- Review proto DiffStats message structure

**Success Criteria**:
- DiffViewer displays unified diff with syntax highlighting
- Line-by-line coloring (green for additions, red for deletions)
- File headers with expand/collapse
- DiffStats shows added/removed line counts
- Loading state while fetching diff
- Empty state for no changes
- Responsive layout

**Testing**:
```bash
cd web && npm run dev
# View session with changes, verify diff displays correctly
```

**Dependencies**: Task 3.2 (requires React Query hooks)

**Implementation Notes**:
```typescript
// web/src/hooks/use-session-diff.ts
import { useQuery } from "@tanstack/react-query";
import { sessionClient } from "@/lib/api/clients";

export function useSessionDiff(sessionId: string) {
  return useQuery({
    queryKey: ["session-diff", sessionId],
    queryFn: async () => {
      const response = await sessionClient.getSession({ id: sessionId });
      return response.session?.diffStats;
    },
    enabled: !!sessionId,
  });
}

// web/src/components/git/DiffViewer.tsx
import { Diff, Hunk, parseDiff } from "react-diff-view";
import "react-diff-view/style/index.css";

export function DiffViewer({ diffContent }: { diffContent: string }) {
  const files = parseDiff(diffContent);

  return (
    <div className="diff-viewer">
      {files.map((file, index) => (
        <div key={index} className="diff-file">
          <div className="diff-file-header">
            {file.oldPath} → {file.newPath}
          </div>
          <Diff
            viewType="unified"
            diffType={file.type}
            hunks={file.hunks}
          >
            {(hunks) =>
              hunks.map((hunk) => (
                <Hunk key={hunk.content} hunk={hunk} />
              ))
            }
          </Diff>
        </div>
      ))}
    </div>
  );
}
```

---

#### Task 4.4: Session Detail Page with Tabs (3h - Medium)

**Scope**: Create detail view with terminal and diff tabs

**Files**:
- `web/src/app/dashboard/[sessionId]/page.tsx` (new)
- `web/src/components/sessions/SessionDetailTabs.tsx` (new)
- `web/src/components/sessions/SessionHeader.tsx` (new)

**Context**:
- Review TUI tabbed window in `ui/tabbed_window.go`
- Study Next.js dynamic routes
- Understand tab component patterns
- Review layout strategies for split views

**Success Criteria**:
- Dynamic route `/dashboard/[sessionId]`
- Session header with title, status, actions
- Tab navigation: Terminal, Diff, Info
- Terminal tab shows live terminal from Task 4.2
- Diff tab shows git diff from Task 4.3
- Info tab shows session metadata
- Keyboard shortcuts for tab switching (1, 2, 3)
- Back button to session list

**Testing**:
```bash
cd web && npm run dev
# Click session card, navigate to detail page
# Switch between tabs, verify content displays
```

**Dependencies**: Task 4.2 (terminal), Task 4.3 (diff), Task 3.3 (session data)

**Implementation Notes**:
```tsx
// web/src/app/dashboard/[sessionId]/page.tsx
"use client";

import { useParams } from "next/navigation";
import { useSession } from "@/hooks/use-sessions";
import { SessionDetailTabs } from "@/components/sessions/SessionDetailTabs";
import { Terminal } from "@/components/terminal/Terminal";
import { DiffViewer } from "@/components/git/DiffViewer";
import { useTerminalStream } from "@/hooks/use-terminal-stream";

export default function SessionDetailPage() {
  const params = useParams();
  const sessionId = params.sessionId as string;

  const { data: session, isLoading } = useSession(sessionId);
  const terminal = useTerminalStream(sessionId);

  if (isLoading) return <div>Loading...</div>;
  if (!session) return <div>Session not found</div>;

  return (
    <div className="flex flex-col h-screen">
      <SessionHeader session={session} />

      <SessionDetailTabs
        tabs={[
          {
            key: "terminal",
            label: "Terminal",
            content: (
              <Terminal
                onData={terminal.sendInput}
                onResize={terminal.sendResize}
              />
            ),
          },
          {
            key: "diff",
            label: "Diff",
            content: <DiffViewer diffContent={session.diffStats?.content || ""} />,
          },
          {
            key: "info",
            label: "Info",
            content: <SessionInfo session={session} />,
          },
        ]}
      />
    </div>
  );
}
```

---

### Story 5: Advanced Features & Polish

#### Task 5.1: Real-time Updates via WatchSessions (2h - Small)

**Scope**: Replace polling with streaming updates

**Files**:
- `web/src/hooks/use-sessions.ts` (modify)
- `web/src/hooks/use-watch-sessions.ts` (new)
- `web/src/app/dashboard/page.tsx` (modify)

**Context**:
- Review Task 2.3 WatchSessions server implementation
- Study ConnectRPC server-streaming client patterns
- Understand React Query cache updates
- Review event-driven state management

**Success Criteria**:
- `useWatchSessions()` hook subscribes to session events
- Updates React Query cache on SessionCreated/Updated/Deleted events
- Replaces 5-second polling from Task 3.2
- Automatic reconnection on disconnect
- Shows connection status indicator
- Graceful fallback to polling on error

**Testing**:
```bash
cd web && npm run dev
# Open two browser tabs, create session in one
# Verify other tab updates immediately without refresh
```

**Dependencies**: Task 2.3 (WatchSessions RPC), Task 3.2 (React Query setup)

**Implementation Notes**:
```typescript
// web/src/hooks/use-watch-sessions.ts
import { useEffect } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { createPromiseClient } from "@connectrpc/connect";
import { SessionService } from "@/gen/session/v1/session_connect";
import { transport } from "@/lib/api/transport";

export function useWatchSessions() {
  const queryClient = useQueryClient();

  useEffect(() => {
    const client = createPromiseClient(SessionService, transport);

    const startWatch = async () => {
      try {
        const stream = client.watchSessions({});

        for await (const event of stream) {
          if (event.event.case === "sessionCreated") {
            queryClient.setQueryData(
              ["sessions"],
              (old: any) => [...(old || []), event.event.value.session]
            );
          } else if (event.event.case === "sessionUpdated") {
            queryClient.setQueryData(["sessions"], (old: any) =>
              old.map((s: any) =>
                s.id === event.event.value.session.id
                  ? event.event.value.session
                  : s
              )
            );
          } else if (event.event.case === "sessionDeleted") {
            queryClient.setQueryData(["sessions"], (old: any) =>
              old.filter((s: any) => s.id !== event.event.value.sessionId)
            );
          }
        }
      } catch (err) {
        console.error("Watch stream error:", err);
        // Fallback to polling
      }
    };

    startWatch();
  }, [queryClient]);
}
```

---

#### Task 5.2: Authentication & Authorization (4h - Large)

**Scope**: Add JWT-based authentication

**Files**:
- `proto/auth/v1/auth.proto` (new)
- `server/auth/jwt.go` (new)
- `server/middleware/auth.go` (new)
- `web/src/lib/auth/auth-provider.tsx` (new)
- `web/src/app/login/page.tsx` (new)

**Context**:
- Review JWT best practices
- Study ConnectRPC interceptors for auth
- Understand Next.js middleware for protected routes
- Review secure token storage (httpOnly cookies)

**Success Criteria**:
- `AuthService` with Login/Logout RPCs
- JWT token generation and validation
- Protected routes require authentication
- Auth middleware on server validates tokens
- Client interceptor adds Authorization header
- Login page with username/password form
- Persistent auth state across sessions
- Token refresh mechanism

**Testing**:
```bash
# Start server with auth enabled
go run . --web --require-auth
curl -X POST http://localhost:8080/auth.v1.AuthService/Login \
  -d '{"username":"admin","password":"password"}'
```

**Dependencies**: Task 1.4 (server foundation)

**Implementation Notes**:
```protobuf
// proto/auth/v1/auth.proto
syntax = "proto3";

package auth.v1;

service AuthService {
  rpc Login(LoginRequest) returns (LoginResponse);
  rpc Logout(LogoutRequest) returns (LogoutResponse);
  rpc RefreshToken(RefreshTokenRequest) returns (RefreshTokenResponse);
}

message LoginRequest {
  string username = 1;
  string password = 2;
}

message LoginResponse {
  string access_token = 1;
  string refresh_token = 2;
  int64 expires_in = 3;
}
```

```go
// server/auth/jwt.go
package auth

import (
    "time"
    "github.com/golang-jwt/jwt/v5"
)

type JWTManager struct {
    secretKey     []byte
    tokenDuration time.Duration
}

func (m *JWTManager) Generate(username string) (string, error) {
    claims := jwt.MapClaims{
        "username": username,
        "exp":      time.Now().Add(m.tokenDuration).Unix(),
    }

    token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
    return token.SignedString(m.secretKey)
}

func (m *JWTManager) Verify(tokenString string) (*jwt.Token, error) {
    return jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
        return m.secretKey, nil
    })
}
```

---

#### Task 5.3: Mobile Responsive Design (3h - Medium)

**Scope**: Optimize UI for mobile devices

**Files**:
- `web/src/components/sessions/SessionList.tsx` (modify)
- `web/src/components/sessions/SessionCard.tsx` (modify)
- `web/src/app/dashboard/[sessionId]/page.tsx` (modify)
- `web/tailwind.config.ts` (add mobile breakpoints)

**Context**:
- Review responsive design patterns
- Study mobile-first CSS approaches
- Understand touch gestures vs. mouse events
- Review viewport meta tag configuration

**Success Criteria**:
- Session list stacks vertically on mobile
- Cards expand to full width on small screens
- Terminal resizes correctly on mobile
- Touch-friendly button sizes (min 44x44px)
- Mobile navigation menu (hamburger)
- Swipe gestures for tab navigation
- Virtual keyboard handling for terminal input
- Tested on iOS Safari and Android Chrome

**Testing**:
```bash
cd web && npm run dev
# Open in browser dev tools, test various device sizes
# Test on actual mobile devices
```

**Dependencies**: Task 3.3 (session list), Task 4.4 (detail page)

**Implementation Notes**:
```tsx
// web/src/components/sessions/SessionCard.tsx (responsive variants)
<div className="
  border rounded-lg p-4
  hover:shadow-md transition

  /* Mobile: full width, larger touch targets */
  w-full
  sm:w-auto

  /* Tablet: grid layout */
  md:flex md:flex-col

  /* Desktop: compact layout */
  lg:p-6
">
  <div className="
    flex flex-col gap-2
    sm:flex-row sm:justify-between sm:items-start
  ">
    {/* Session info */}
  </div>

  <div className="
    mt-4 flex flex-col gap-2
    sm:flex-row
  ">
    <button className="
      min-h-[44px] min-w-[44px]
      w-full sm:w-auto
    ">
      {isPaused ? "Resume" : "Pause"}
    </button>
  </div>
</div>
```

---

#### Task 5.4: Error Boundaries & Loading States (2h - Small)

**Scope**: Add comprehensive error handling and loading UX

**Files**:
- `web/src/components/errors/ErrorBoundary.tsx` (new)
- `web/src/components/loading/SessionListSkeleton.tsx` (new)
- `web/src/components/loading/TerminalSkeleton.tsx` (new)
- `web/src/app/error.tsx` (new)
- `web/src/app/loading.tsx` (new)

**Context**:
- Review React Error Boundary patterns
- Study skeleton loading states
- Understand Next.js error handling conventions
- Review user-friendly error messages

**Success Criteria**:
- ErrorBoundary catches React errors and displays fallback
- SessionListSkeleton shows while loading sessions
- TerminalSkeleton shows while connecting to stream
- Retry buttons on error states
- User-friendly error messages (no stack traces)
- Logging of errors to console (dev) or service (prod)
- Graceful degradation on feature failures

**Testing**:
```bash
cd web && npm run dev
# Simulate errors (disconnect server, invalid session ID)
# Verify error boundaries catch and display correctly
```

**Dependencies**: Task 3.3 (session list), Task 4.2 (terminal)

**Implementation Notes**:
```tsx
// web/src/components/errors/ErrorBoundary.tsx
"use client";

import { Component, ReactNode } from "react";

interface Props {
  children: ReactNode;
  fallback?: (error: Error, retry: () => void) => ReactNode;
}

interface State {
  hasError: boolean;
  error: Error | null;
}

export class ErrorBoundary extends Component<Props, State> {
  constructor(props: Props) {
    super(props);
    this.state = { hasError: false, error: null };
  }

  static getDerivedStateFromError(error: Error) {
    return { hasError: true, error };
  }

  componentDidCatch(error: Error, errorInfo: any) {
    console.error("ErrorBoundary caught error:", error, errorInfo);
  }

  retry = () => {
    this.setState({ hasError: false, error: null });
  };

  render() {
    if (this.state.hasError && this.state.error) {
      if (this.props.fallback) {
        return this.props.fallback(this.state.error, this.retry);
      }

      return (
        <div className="error-container">
          <h2>Something went wrong</h2>
          <p>{this.state.error.message}</p>
          <button onClick={this.retry}>Try Again</button>
        </div>
      );
    }

    return this.props.children;
  }
}
```

---

#### Task 5.5: Production Build & Docker Configuration (3h - Medium)

**Scope**: Prepare production deployment with Docker

**Files**:
- `Dockerfile` (modify - add web build stage)
- `docker-compose.yml` (new)
- `.dockerignore` (modify)
- `Makefile` (add docker targets)
- `web/.env.production` (new)

**Context**:
- Review Docker multi-stage builds
- Study Next.js standalone output
- Understand Go static binary compilation
- Review docker-compose service configuration

**Success Criteria**:
- Multi-stage Dockerfile builds both Go and Next.js
- Production-optimized Next.js build (standalone mode)
- Static Go binary with web assets embedded
- Docker image < 100MB compressed
- docker-compose.yml for local testing
- Environment variable configuration
- Health check endpoints
- Graceful shutdown support

**Testing**:
```bash
make docker-build
docker run -p 8080:8080 claude-squad-web
# Access http://localhost:8080, verify full functionality
```

**Dependencies**: All previous tasks (final integration)

**Implementation Notes**:
```dockerfile
# Dockerfile
# Stage 1: Build Next.js frontend
FROM node:20-alpine AS web-builder
WORKDIR /app/web
COPY web/package*.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# Stage 2: Build Go backend
FROM golang:1.24-alpine AS go-builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o claude-squad-web

# Stage 3: Production image
FROM alpine:latest
RUN apk --no-cache add ca-certificates tmux git
WORKDIR /app
COPY --from=go-builder /app/claude-squad-web .
COPY --from=web-builder /app/web/.next/standalone ./web
COPY --from=web-builder /app/web/.next/static ./web/.next/static
COPY --from=web-builder /app/web/public ./web/public

EXPOSE 8080
CMD ["./claude-squad-web", "--web"]
```

```yaml
# docker-compose.yml
services:
  claude-squad-web:
    build: .
    ports:
      - "8080:8080"
    volumes:
      - ./data:/root/.claude-squad
    environment:
      - LOG_LEVEL=info
      - PORT=8080
    healthcheck:
      test: ["CMD", "wget", "--spider", "http://localhost:8080/health"]
      interval: 30s
      timeout: 10s
      retries: 3
```

---

## Dependency Visualization

### Sequential Task Flow

```
Story 1: ConnectRPC Infrastructure
├─ Task 1.1: Proto Definitions (START)
│  └─ Task 1.2: Go Code Generation
│     └─ Task 1.4: HTTP Server
│        └─ Task 1.5: List & Get RPCs
│           └─ Task 1.6: Create RPC
│              └─ Task 1.7: Update & Delete RPCs
│
├─ Task 1.1: Proto Definitions
│  └─ Task 1.3: TypeScript Code Generation

Story 2: Real-time Streaming (Parallel with Story 1.5-1.7)
├─ Task 2.1: Event Proto (after 1.1)
│  └─ Task 2.2: Event Bus (independent)
│     └─ Task 2.3: WatchSessions RPC
│  └─ Task 2.4: Terminal Streaming (after 2.1)
│
└─ Task 2.5: Storage Events (after 2.2 + 1.5)

Story 3: Dashboard UI (Parallel after 1.5)
├─ Task 3.1: Client Setup (after 1.3 + 1.4)
│  └─ Task 3.2: React Query (after 3.1)
│     └─ Task 3.3: Session List (after 3.2)
│        └─ Task 3.4: Categories (after 3.3)
│           └─ Task 3.5: Search & Filter (after 3.4)
│
└─ Task 3.6: Create Modal (after 3.2, parallel with 3.3-3.5)

Story 4: Terminal & Git (Parallel with Story 3.3+)
├─ Task 4.1: xterm.js (independent)
│  └─ Task 4.2: Terminal Streaming (after 4.1 + 2.4)
│
├─ Task 4.3: Diff Viewer (after 3.2)
│
└─ Task 4.4: Detail Page (after 4.2 + 4.3 + 3.3)

Story 5: Polish (After all core features)
├─ Task 5.1: Real-time Updates (after 2.3 + 3.2)
├─ Task 5.2: Authentication (after 1.4, parallel with 5.3-5.4)
├─ Task 5.3: Mobile Responsive (after 3.3 + 4.4)
├─ Task 5.4: Error Handling (after 3.3 + 4.2)
└─ Task 5.5: Production Build (after ALL tasks)
```

### Parallel Work Opportunities

**Week 1:**
- **Developer A**: Tasks 1.1 → 1.2 → 1.4 → 1.5 (Backend foundation)
- **Developer B**: Tasks 2.1 → 2.2 (Event infrastructure, parallel after 1.1)
- **Developer C**: Tasks 1.3 → 3.1 (Frontend setup, parallel after 1.1)

**Week 2:**
- **Developer A**: Tasks 1.6 → 1.7 → 2.4 (Session mutations + terminal streaming)
- **Developer B**: Tasks 2.3 → 2.5 (Real-time updates)
- **Developer C**: Tasks 3.2 → 3.3 → 3.4 → 3.5 (Dashboard UI)

**Week 3:**
- **Developer A**: Tasks 5.2 (Authentication)
- **Developer B**: Tasks 4.1 → 4.2 (Terminal integration)
- **Developer C**: Tasks 3.6 → 4.3 → 4.4 (Session creation + git diff)

**Week 3-4:**
- **All Developers**: Tasks 5.1, 5.3, 5.4 (Polish)
- **DevOps Lead**: Task 5.5 (Production deployment)

---

## Context Preparation Guide

### For Backend Developers (Go)

**Essential Files to Read:**
1. `session/instance.go` - Core session management logic
2. `session/storage.go` - Persistence layer
3. `session/tmux/tmux.go` - Terminal session handling
4. `app/app.go` - TUI application structure (for feature parity)
5. `config/state.go` - State management patterns

**ConnectRPC Learning Path:**
1. Read ConnectRPC docs: https://connectrpc.com/docs/go/getting-started
2. Review Protocol Buffers guide: https://protobuf.dev/getting-started/gotutorial/
3. Study example server: https://github.com/connectrpc/examples-go

**Development Environment:**
```bash
# Install required tools
go install github.com/bufbuild/buf/cmd/buf@latest
go install connectrpc.com/connect/cmd/protoc-gen-connect-go@latest

# Run server in development mode
go run . --web --debug
```

---

### For Frontend Developers (TypeScript/React)

**Essential Files to Read:**
1. `web/src/gen/` - Generated TypeScript types (after proto generation)
2. `ui/list.go` - TUI list implementation (for UX parity)
3. `ui/overlay/sessionSetup.go` - Session creation flow
4. `session/storage.go` - Data structures (InstanceData, Status enums)

**ConnectRPC Learning Path:**
1. Read ConnectRPC-Web docs: https://connectrpc.com/docs/web/getting-started
2. Review React Query integration: https://tanstack.com/query/latest
3. Study xterm.js: https://xtermjs.org/docs/

**Development Environment:**
```bash
cd web
npm install
npm run dev  # Starts Next.js on :3000

# In another terminal, run backend
cd ..
go run . --web
```

---

## Testing Strategy

### Unit Tests
- **Go**: `go test ./server/... -v -race`
- **TypeScript**: `cd web && npm test`

### Integration Tests
- **API Tests**: Test ConnectRPC endpoints with real server
- **Component Tests**: React Testing Library for UI components
- **E2E Tests**: Playwright for full user workflows

### Performance Tests
- **Load Testing**: k6 for API endpoints (50+ concurrent clients)
- **Stream Performance**: Measure latency for terminal streaming
- **Frontend Performance**: Lighthouse scores (>90 for Performance)

### Acceptance Criteria
- ✅ All unit tests pass
- ✅ Integration tests cover happy path and error cases
- ✅ E2E tests validate critical user journeys
- ✅ Performance benchmarks meet targets
- ✅ Manual testing on desktop and mobile devices

---

## Success Metrics & Validation

### Feature Parity
- [ ] Session CRUD operations (create, read, update, delete)
- [ ] Real-time session status updates
- [ ] Live terminal viewing with input
- [ ] Git diff display
- [ ] Category organization with expand/collapse
- [ ] Fuzzy search across sessions
- [ ] Session filtering (hide paused)
- [ ] Mobile-responsive design

### Performance Targets
- [ ] Session list loads in <500ms (100 sessions)
- [ ] Terminal streaming latency <50ms
- [ ] WatchSessions updates within 100ms
- [ ] First Contentful Paint <1.5s
- [ ] Time to Interactive <3s
- [ ] Lighthouse Performance score >90

### Production Readiness
- [ ] Authentication and authorization
- [ ] Error boundaries and graceful degradation
- [ ] Docker deployment configuration
- [ ] Health check endpoints
- [ ] Logging and monitoring
- [ ] API documentation (generated from proto)

---

## Appendix: Technology Stack Summary

### Backend (Go)
- **Framework**: `net/http` + `connectrpc.com/connect`
- **Protocol Buffers**: `buf` + `protoc-gen-connect-go`
- **Existing Dependencies**: `charmbracelet/bubbletea`, `go-git/go-git`, `spf13/cobra`

### Frontend (TypeScript/React)
- **Framework**: Next.js 15.3.2 (App Router)
- **RPC Client**: `@connectrpc/connect-web`
- **State Management**: TanStack Query (React Query)
- **Terminal**: `@xterm/xterm` + addons
- **Diff Viewer**: `react-diff-view`
- **UI Components**: Headless UI or Radix UI
- **Styling**: Tailwind CSS

### DevOps
- **Build Tool**: `buf` for Protocol Buffers
- **Containerization**: Docker multi-stage builds
- **Orchestration**: docker-compose (local), Kubernetes (production)
- **Reverse Proxy**: nginx or Caddy (optional)

---

## Next Steps

1. **Phase 0: Setup** (Day 1)
   - Initialize `proto/` directory with buf configuration
   - Install development tools (buf, protoc-gen-connect-go)
   - Set up CI/CD for proto generation

2. **Phase 1: Core API** (Days 2-7)
   - Complete Story 1: Tasks 1.1 through 1.7
   - Validate API with manual testing (curl/Postman)

3. **Phase 2: Real-time Features** (Days 8-10)
   - Complete Story 2: Tasks 2.1 through 2.5
   - Integration test with multiple concurrent clients

4. **Phase 3: Dashboard** (Days 11-14)
   - Complete Story 3: Tasks 3.1 through 3.6
   - User acceptance testing on dashboard features

5. **Phase 4: Terminal & Git** (Days 15-17)
   - Complete Story 4: Tasks 4.1 through 4.4
   - Validate terminal streaming performance

6. **Phase 5: Production Ready** (Days 18-21)
   - Complete Story 5: Tasks 5.1 through 5.5
   - Security audit and performance optimization
   - Final deployment and documentation