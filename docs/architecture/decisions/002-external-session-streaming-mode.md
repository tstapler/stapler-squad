# ADR-002: Streaming Mode for External Sessions

## Status
Accepted

## Context
External sessions use `tmux capture-pane` for terminal content, which provides full snapshots rather than incremental byte streams. We need to decide how to transmit these snapshots through the unified ConnectRPC protocol.

### Current Behavior
- **Managed sessions**: Stream raw PTY bytes incrementally
- **External sessions**: Poll `tmux capture-pane` every 150ms for full terminal content

### Available Streaming Modes
The ConnectRPC handler supports multiple streaming modes:
1. **"raw"**: Direct byte streaming (used by managed sessions)
2. **"raw-compressed"**: LZMA compressed byte streaming
3. **"state"**: Complete terminal state with ANSI parsing
4. **"hybrid"**: Both raw and state for comparison

## Decision
External sessions will use **"state" streaming mode** with snapshot-based updates.

## Rationale

### Natural Fit for Snapshots
The state mode is designed to handle complete terminal states, which aligns perfectly with `tmux capture-pane` output.

### Efficient Updates
State mode includes:
- Sequence numbers for ordering
- Cursor position tracking
- Dimension management
- Optimized diff generation

### Compression Benefits
State mode can leverage LZMA compression for large terminal contents, reducing bandwidth significantly.

### Client Compatibility
The frontend `StateApplicator` already handles state-based updates, ensuring smooth rendering without flicker.

## Consequences

### Positive
- External sessions gain compression capabilities
- Consistent protocol envelope format
- Better handling of terminal resize events
- Reduced bandwidth for large terminals
- Client-side optimization opportunities

### Negative
- State generation adds processing overhead
- Cannot provide true incremental updates
- 150ms polling interval remains a limitation
- Potential for state synchronization issues

### Neutral
- Different internal processing paths for same protocol
- State sequence numbers independent from managed sessions

## Implementation Details

### State Generation for External Sessions
```go
func (h *ConnectRPCWebSocketHandler) streamExternalTerminal(stream *connectWebSocketStream, instance *session.Instance) error {
    // Use ExternalTmuxStreamer for polling
    streamer := h.tmuxStreamerManager.GetOrCreate(instance.ExternalMetadata.TmuxSessionName)
    
    // Configure state generator for snapshots
    stateGen := terminal.NewStateGenerator(cols, rows)
    
    // Consumer receives full terminal content
    consumer := func(content string) {
        state := stateGen.GenerateState([]byte(content))
        
        terminalData := &sessionv1.TerminalData{
            SessionId: instance.Title,
            Data: &sessionv1.TerminalData_State{
                State: state,
            },
        }
        
        // Send via ConnectRPC protocol
        sendMessage(stream, terminalData)
    }
    
    streamer.AddConsumer(consumer)
}
```

### Polling Configuration
- Maintain 150ms polling interval for responsiveness
- Consider adaptive polling based on activity
- Future: Investigate tmux hooks for event-driven updates

### Compression Strategy
- Enable LZMA for states >1KB
- Monitor compression ratios
- Fallback to uncompressed on error

## Monitoring

### Metrics to Track
- State generation time
- Compression ratio
- Message size distribution
- Sequence number gaps
- Client-side apply latency

## Future Improvements

### Event-Driven Updates
Investigate tmux control mode or hooks to receive change notifications instead of polling.

### Incremental State Diffs
Develop algorithm to compute minimal diffs between consecutive states.

### Adaptive Polling
Adjust polling frequency based on terminal activity patterns.

## References
- [Terminal State Protocol](https://github.com/claude-squad/proto/terminal_state.proto)
- [StateApplicator Implementation](https://github.com/claude-squad/web-app/StateApplicator.ts)
- [tmux capture-pane Documentation](https://man.openbsd.org/tmux#capture-pane)
