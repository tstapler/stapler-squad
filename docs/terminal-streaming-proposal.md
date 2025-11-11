# Terminal Streaming Architecture Proposal

## Problem Statement

Current MOSH-style state synchronization approach causes issues:
- Lines scroll off screen incorrectly
- `terminal.clear()` on every update disrupts scrollback
- Fighting against xterm.js's design (incremental ANSI stream processing)
- Complex state generation/application logic with bugs

## Proposed Solution: Raw PTY Streaming

Go back to traditional terminal architecture that works with xterm.js:

### 1. Server Side (Go)

**Remove:** State generation, line splitting, complete state snapshots

**Keep:** Raw PTY byte stream

```go
// server/services/connectrpc_websocket.go

func (h *ConnectRPCWebSocketHandler) streamTerminal(stream *connectWebSocketStream) error {
    // ... setup code ...

    // Simple: Read from PTY, send raw bytes to WebSocket
    go func() {
        buffer := make([]byte, 4096)
        for {
            n, err := ptyReader.Read(buffer)
            if err != nil {
                // Handle error
                return
            }

            if n > 0 {
                // Send raw PTY bytes directly
                terminalData := &sessionv1.TerminalData{
                    SessionId: sessionID,
                    Data: &sessionv1.TerminalData_Output{
                        Output: &sessionv1.TerminalOutput{
                            Data: buffer[:n], // Raw bytes
                        },
                    },
                }

                dataBytes, _ := proto.Marshal(terminalData)
                envelope := protocol.CreateEnvelope(0, dataBytes)
                stream.WriteMessage(websocket.BinaryMessage, envelope)
            }
        }
    }()

    // Input: Read from WebSocket, write to PTY
    go func() {
        for {
            _, message, err := stream.conn.ReadMessage()
            if err != nil {
                return
            }

            // Parse input and write to PTY
            var incomingData sessionv1.TerminalData
            proto.Unmarshal(envelope.Data, &incomingData)

            if input := incomingData.GetInput(); input != nil {
                ptyWriter.Write(input.Data)
            }
        }
    }()

    return nil
}
```

### 2. Client Side (TypeScript)

**Remove:** StateApplicator, state-based rendering, line positioning

**Keep:** Raw write to xterm

```typescript
// web-app/src/lib/hooks/useTerminalStream.ts

export function useTerminalStream(config: TerminalStreamConfig) {
    // ... setup ...

    const connect = useCallback(async () => {
        const stream = client.streamTerminal(request);

        for await (const msg of stream) {
            if (msg.data.case === "output") {
                // RAW APPROACH: Just write bytes directly
                const bytes = msg.data.value.data;
                const text = textDecoder.decode(bytes);

                const terminal = getTerminal?.();
                if (terminal) {
                    terminal.write(text); // xterm handles everything!
                }
            }
        }
    }, []);

    return { connect, sendInput, resize, ... };
}
```

### 3. Benefits

✅ **Simplicity:** PTY → WebSocket → xterm (no intermediate processing)
✅ **Reliability:** Standard terminal emulation (decades of proven design)
✅ **Scrollback:** xterm.js handles automatically
✅ **Cursor:** ANSI codes in content position cursor correctly
✅ **Colors/Formatting:** All ANSI codes work natively
✅ **Input:** Bidirectional stream just works
✅ **Performance:** No state generation overhead

### 4. What About Features?

**Q: What about scrollback loading?**
A: xterm.js maintains scrollback buffer automatically. For older history, use `tmux capture-pane -S -1000` on-demand.

**Q: What about connection recovery?**
A: Send `tmux capture-pane` output on reconnect to restore current screen.

**Q: What about compression?**
A: Use WebSocket compression (permessage-deflate) - transparent to application.

**Q: What about latency?**
A: Raw streaming has LOWER latency than state generation (no batching/processing delay).

## Migration Path

### Phase 1: Add Raw Mode (Parallel)
- Add `TerminalOutput` field to protobuf
- Keep existing state-based mode
- Add flag to switch between modes

### Phase 2: Test Raw Mode
- Use test harness (tests/terminal-harness.html)
- Verify rendering, input, scrollback, resize

### Phase 3: Switch Default
- Make raw mode default
- Keep state mode as fallback

### Phase 4: Remove State Mode
- Delete StateApplicator
- Delete state generation code
- Simplify protobuf schema

## Implementation

See:
- `tests/terminal-harness.html` - Test/validation tool
- `docs/terminal-streaming-proposal.md` - This document
