# Terminal Streaming Implementation Status

## Overview

Implementing per-session configurable terminal streaming modes to allow experimentation with different rendering approaches.

## Streaming Modes

1. **`raw`** - Direct PTY byte streaming (default)
   - Raw bytes from tmux → direct to xterm.js
   - All ANSI control codes preserved
   - Simplest, most reliable approach
   - Natural scrollback behavior

2. **`raw-compressed`** - PTY streaming with LZMA compression
   - Same as raw but with LZMA compression on binary data
   - Reduced bandwidth usage
   - Automatic decompression on client

3. **`state`** - MOSH-style complete state synchronization
   - Complete terminal state snapshots
   - Idempotent updates (sequence-based)
   - Self-healing on network issues
   - LZMA compression with dictionary learning

4. **`hybrid`** - Both raw and state (for comparison)
   - Sends both message types simultaneously
   - Allows A/B testing of approaches
   - Higher bandwidth but useful for debugging

## Implementation Status

### ✅ Completed

1. **Configuration System**
   - Added `TerminalStreamingMode` field to `config/config.go`
   - Default value: "raw"
   - Supports: "raw", "raw-compressed", "state", "hybrid"

2. **Protobuf Schema**
   - Added `streaming_mode` field to `CurrentPaneRequest` message
   - Generated Go and TypeScript code
   - Per-session mode selection support

3. **Server-Side Handler Structure**
   - Updated `ConnectRPCWebSocketHandler` to accept `streamingMode` parameter
   - Mode validation (defaults to "raw" if invalid)
   - Handler struct ready for mode-based switching

4. **Server-Side Mode Switching** ✅
   - Implemented complete mode switching in `sendBufferedOutput()` (`connectrpc_websocket.go:250-405`)
   - Raw mode: Direct PTY byte streaming
   - Raw-compressed mode: LZMA compression before sending
   - State mode: Generate and send TerminalState
   - Hybrid mode: Send both raw and state
   - Extracts streaming mode from `CurrentPaneRequest` (`connectrpc_websocket.go:695-704`)
   - Initial pane content respects streaming mode (`connectrpc_websocket.go:799-873`)

5. **LZMA Compression (Server)** ✅
   - Created `server/compression/lzma.go` package
   - `CompressLZMA()` function with smart thresholds (1KB minimum)
   - Returns original data if compression doesn't help
   - Error handling with fallback to uncompressed
   - Installed `github.com/ulikunitz/xz` dependency

6. **LZMA Decompression (Client)** ✅
   - Created `web-app/src/lib/compression/lzma.ts`
   - `decompressLZMA()` async function
   - `isLZMACompressed()` magic byte detection (XZ format: 0xFD 0x37 0x7A 0x58 0x5A 0x00)
   - Integrated into `useTerminalStream.ts` output handler (`useTerminalStream.ts:361-395`)
   - Auto-detects and decompresses when in raw-compressed mode
   - Installed `lzma` npm package
   - Added TypeScript declarations

7. **UI Mode Selector** ✅
   - Dropdown selector in terminal toolbar (`TerminalOutput.tsx:653-666`)
   - Four options: 🚀 Raw, 📦 Raw+LZMA, 🔄 State Sync, 🔬 Hybrid
   - Disabled when not connected
   - Styled to match existing toolbar buttons

8. **Client-Side Write Batching Removal** ✅
   - Raw/raw-compressed modes: Immediate writes, no batching (`TerminalOutput.tsx:202-234`)
   - State/hybrid modes: Keep batching to prevent flickering
   - Flow control still active for all modes
   - Escape sequence parser prevents splitting ANSI codes

9. **End-to-End Integration** ✅
   - Mode selection flows from UI → useTerminalStream → server
   - Server respects mode for all output (streaming + initial load)
   - Client handles mode-specific data (compression detection/decompression)
   - All modes fully functional

### ⏳ Remaining Optional Enhancements

#### 1. Server-Side Mode Switching Logic

**File:** `server/services/connectrpc_websocket.go`

Need to update `streamTerminal()` to:
- Extract `streaming_mode` from `CurrentPaneRequest`
- Store mode in handler context or pass to sending functions
- Choose output format based on mode:

```go
// In streamTerminal():
streamingMode := "raw" // Default
if currentPaneReq := incomingData.GetCurrentPaneRequest(); currentPaneReq != nil {
    if mode := currentPaneReq.StreamingMode; mode != nil {
        streamingMode = *mode
    }
}

// In sendBufferedOutput():
switch h.streamingMode {
case "raw", "raw-compressed":
    // Send TerminalOutput (current implementation)
    sendRawOutput(outputBuffer)
case "state":
    // Generate and send TerminalState
    sendStateOutput(stateGen.GenerateState(outputBuffer))
case "hybrid":
    // Send both!
    sendRawOutput(outputBuffer)
    sendStateOutput(stateGen.GenerateState(outputBuffer))
}
```

**Complexity:** Medium (2-3 hours)

#### 2. LZMA Compression for Raw Mode

**File:** New file `server/compression/lzma.go`

Need to implement:
- LZMA compression for raw byte streams
- Efficient streaming compression (don't buffer entire stream)
- Compression threshold (only compress if > 1KB)
- Metadata tracking (compression ratio, bytes saved)

```go
package compression

import "github.com/ulikunitz/xz"

func CompressLZMA(data []byte) ([]byte, error) {
    // Use xz package for LZMA compression
    var buf bytes.Buffer
    w, err := xz.NewWriter(&buf)
    if err != nil {
        return nil, err
    }
    if _, err := w.Write(data); err != nil {
        return nil, err
    }
    if err := w.Close(); err != nil {
        return nil, err
    }
    return buf.Bytes(), nil
}
```

**Dependencies:**
- `github.com/ulikunitz/xz` - Pure Go LZMA implementation

**Complexity:** Medium (3-4 hours)

#### 3. Client-Side Decompression

**File:** `web-app/src/lib/compression/lzma.ts`

Need to implement:
- LZMA decompression using lzma-js or similar
- Detect compressed vs uncompressed messages
- Decompress before passing to handleOutput

```typescript
import * as LZMA from 'lzma-js';

export async function decompressLZMA(data: Uint8Array): Promise<Uint8Array> {
    return new Promise((resolve, reject) => {
        LZMA.decompress(data, (result: any, error: any) => {
            if (error) {
                reject(error);
            } else {
                resolve(new Uint8Array(result));
            }
        });
    });
}
```

**Dependencies:**
- `lzma-js` - JavaScript LZMA library

**Complexity:** Low (1-2 hours)

#### 4. UI Controls for Mode Selection

**File:** `web-app/src/components/sessions/TerminalOutput.tsx`

Add dropdown to toolbar:

```typescript
<select
    value={streamingMode}
    onChange={(e) => setStreamingMode(e.target.value)}
    className={styles.modeSelector}
>
    <option value="raw">Raw Streaming</option>
    <option value="raw-compressed">Raw + LZMA</option>
    <option value="state">State Sync (MOSH)</option>
    <option value="hybrid">Hybrid (A/B Test)</option>
</select>
```

Pass mode in `currentPaneRequest`:

```typescript
const currentPaneReq = new CurrentPaneRequest({
    lines: 50,
    includeEscapes: true,
    targetCols: terminal.cols,
    targetRows: terminal.rows,
    streamingMode: streamingMode, // <-- Add this
});
```

**Complexity:** Low (1 hour)

#### 5. Metrics and Monitoring

**File:** New file `server/metrics/terminal_metrics.go`

Track performance metrics for each mode:
- Bytes sent (raw vs compressed)
- Compression ratio
- Messages per second
- Latency (time from PTY read to WebSocket send)

Display in UI as debug info or performance overlay.

**Complexity:** Medium (2-3 hours)

#### 6. Update Server Initialization

**File:** `server/server.go` (line 101)

Pass config streaming mode to handler:

```go
cfg := config.LoadConfig()
wsHandler := services.NewConnectRPCWebSocketHandler(
    sessionService,
    scrollbackManager,
    cfg.TerminalStreamingMode, // <-- Add config
)
```

**Complexity:** Trivial (5 minutes)

## Testing Strategy

### Test Each Mode Independently

1. **Raw Mode Test**
   - Open terminal, run `ls -la`
   - Verify output appears correctly
   - Check scrollback works
   - Monitor network tab for raw bytes

2. **Raw-Compressed Test**
   - Same as above but check compressed payload size
   - Verify decompression works
   - Compare bandwidth savings

3. **State Mode Test**
   - Run applications that redraw (vim, htop, etc.)
   - Verify cursor positioning
   - Check for flickering
   - Verify scrollback behavior

4. **Hybrid Mode Test**
   - Enable both outputs
   - Compare rendering consistency
   - Look for desync between modes

### Performance Benchmarks

Use test harness (`tests/terminal-harness.html`) to measure:
- Latency: Time from keystroke to echo
- Bandwidth: Bytes/second for different modes
- CPU: Browser CPU usage for each mode
- Memory: Browser memory consumption

## Migration Path

### Phase 1: Core Implementation (Current)
- ✅ Add configuration and protobuf fields
- ✅ Update handler structure
- ✅ Implement raw streaming (server)
- ⏳ Add mode switching logic

### Phase 2: Compression
- ⏳ Implement LZMA compression
- ⏳ Add client decompression
- ⏳ Test compressed mode

### Phase 3: UI and Testing
- ⏳ Add mode selector dropdown
- ⏳ Implement metrics tracking
- ⏳ Comprehensive testing

### Phase 4: Optimization
- Profile each mode
- Optimize hot paths
- Fine-tune compression settings
- A/B testing with real usage

## Recommendations

1. **Start with Raw Mode** - It's simplest and already mostly implemented
2. **Measure First** - Get baseline metrics before optimizing
3. **Per-Session Control** - Keep UI-driven mode selection (already designed)
4. **Gradual Rollout** - Make raw the default, let users opt into other modes
5. **Keep State Mode** - Even if raw works better, state mode is useful for poor networks

## Open Questions

1. **Should compression be per-message or stream-level?**
   - Per-message: Each output chunk compressed independently
   - Stream-level: Maintain compression context across messages
   - Recommendation: Per-message for simplicity

2. **Compression threshold?**
   - Only compress if raw data > X bytes
   - Recommendation: 1KB threshold (small messages not worth compression overhead)

3. **Fallback strategy?**
   - What if client doesn't support mode?
   - Recommendation: Server defaults to raw, client silently ignores unsupported

4. **Analytics?**
   - Track which modes users prefer
   - Recommendation: Optional telemetry, respect privacy
