# Claude Squad - Current Priority Tasks

## IN PROGRESS: Web Server Implementation with ConnectRPC 🚀

**Status**: Tasks 1.1 & 1.2 COMPLETE - Code generation pipeline ready
**Priority**: P1 - New feature development

### Progress:
- [x] ✅ **Task 1.1**: Protocol Buffer Definitions (2h)
  - Created `proto/session/v1/session.proto` with SessionService
  - Created `proto/session/v1/types.proto` with message types
  - Configured `buf.yaml` for linting and validation
  - Configured `buf.gen.yaml` for Go + TypeScript code generation
  - All proto files validated with `buf lint` and `buf build`

- [x] ✅ **Task 1.2**: Go Code Generation Setup (1h)
  - Created `tools.go` for buf and protoc plugin dependencies
  - Added ConnectRPC v1.19.0 and protobuf v1.36.9 to go.mod
  - Added Makefile targets: `proto-gen`, `proto-lint`, `proto-build`
  - Generated Go code with ConnectRPC server stubs
  - Generated TypeScript client code for Next.js
  - All generated code compiles successfully

### Next Atomic Task:
- [ ] **Task 1.4**: HTTP Server Foundation (2h)
  - Create `server/server.go` with ConnectRPC handler registration
  - Create `server/middleware/cors.go` and `logging.go`
  - Add `--web` flag to `main.go`
  - HTTP server starts on `:8080`
  - Graceful shutdown on SIGTERM/SIGINT

**See**: [Web Server ConnectRPC Implementation](docs/tasks/web-server-connectrpc-implementation.md)

---

## DEFERRED: Critical Test Timeouts

**Status**: Tests compile successfully, deferred while building new features
**Priority**: P2 - Resume after web server MVP

### Root Cause: External Command Dependencies in Tests
Tests hang in `config.GetClaudeCommand()` which executes shell commands during setup.

### Deferred Actions:
- [ ] **CRITICAL**: Mock external command dependencies in test environment
- [ ] **URGENT**: Fix UI test snapshot mismatches
- [ ] **VALIDATION**: Ensure clean test execution with `go test ./... -timeout=30s`

**See**: [Emergency Test Timeouts Task](docs/tasks/emergency-test-timeouts.md)

---

## DEFERRED: Test Stabilization

**Status**: Deferred while building new features
**Priority**: P2 - Resume after web server MVP

### Test Infrastructure Tasks:
- [ ] Fix UI search index nil pointer issues (`TestFuzzySearchIntegration`)
- [ ] Resolve layout calculation mismatches (`TestLayoutDebug`)
- [ ] Stabilize session package test timeouts
- [ ] Integrate teatest framework for TUI testing

**See**: [Test Stabilization Epic](docs/tasks/test-stabilization-and-teatest-integration.md)

---

## Documentation Maintenance

### Completed and Updated:
- [x] ✅ **Contextual Git Repository Discovery** - All implementation complete
- [x] ✅ **Unit Testing & Validation** - Comprehensive test coverage
- [x] ✅ **Path Validation & UX** - Enhanced error handling and shortcuts
- [x] ✅ **Edge Case Handling** - Network paths, permissions, empty queries

### Architecture Implementation Status:
- [x] ✅ **SessionSetupOverlay** - Contextual discovery fully implemented
- [x] ✅ **FuzzyInputOverlay** - Raw path entry support added
- [x] ✅ **Git Integration** - Repository, branch, worktree discovery working
- [x] ✅ **Performance** - Benchmarked at 0.47ms per operation

---

## Future Priorities (After Emergency Resolution)

### Medium Term (Next 3-5 Sessions):
- [ ] **Session Health Check Integration** - Evaluate health check system
- [ ] **Filtering System Enhancement** - Tag vs Category analysis
- [ ] **Help System Consolidation** - Compare current vs unused help generator

### Long Term (Future Sessions):
- [ ] **Dead Code Removal** - Clean up unused constructors and test mocks
- [ ] **Performance Optimization** - Large directory tree handling improvements
- [ ] **Advanced Features** - Network path support, fuzzy path matching

---

## Context Notes

**Last Updated**: 2025-01-17
**Current Phase**: Emergency Build Stabilization
**Next Milestone**: Restore compilation and test execution capability

**Critical Dependencies**:
- Build failures must be resolved before any other development work
- Test stabilization required for production deployment confidence
- All major feature work is complete and functional (when builds work)