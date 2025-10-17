# Zombie Process Leak Fix

**Date**: 2025-10-09
**Issue**: claude-squad was leaking zombie tmux processes, reaching system process limit (10,662/10,666)
**Root Cause**: Daemon process started with `cmd.Start()` but never reaped
**Fix**: Added `cmd.Process.Release()` to properly release daemon process

## What Was Fixed

The `LaunchDaemon()` function in `daemon/daemon.go` was starting a daemon process but never calling `cmd.Wait()` or `cmd.Process.Release()`. This caused the daemon to become a zombie process when it exited, gradually accumulating thousands of zombies until the system ran out of available processes.

**Fixed Code** (daemon/daemon.go:371-378):
```go
// Release the process so it won't become a zombie when it exits
// This tells the OS that the parent won't wait for the child
if err := cmd.Process.Release(); err != nil {
    log.WarningLog.Printf("failed to release daemon process (may become zombie on exit): %v", err)
}

// Don't wait for the child to exit, it's detached and released
return nil
```

## Immediate Cleanup Steps

### 1. Stop All claude-squad Processes
```bash
# Find all claude-squad processes
ps aux | grep claude-squad | grep -v grep

# Kill all running claude-squad instances (including zombies)
killall -9 claude-squad

# Verify all are killed
ps aux | grep claude-squad | grep -v grep
```

### 2. Clean Up Zombie Processes
```bash
# Check for remaining zombie processes
ps aux | grep 'Z' | grep tmux

# Zombies will be cleaned up automatically when parent is killed
# Verify zombies are gone
ps aux | awk '$8=="Z"' | wc -l
# Should show 0
```

### 3. Verify System Recovery
```bash
# Check current process count
ps aux | wc -l

# Should be much lower than 10,662
# Normal system process count is typically 300-1000
```

### 4. Rebuild and Restart
```bash
cd /Users/tylerstapler/IdeaProjects/claude-squad

# Build with fix
go build .

# Start claude-squad
./claude-squad
```

## Verification Steps

### Monitor for Zombie Accumulation
After restarting claude-squad, monitor for any new zombies:

```bash
# Monitor zombie count (should stay at 0)
watch -n 5 'ps aux | awk '\''$8=="Z"'\'' | wc -l'

# Check claude-squad child processes
ps aux | grep -E 'claude-squad|tmux' | grep -v grep
```

### Check Daemon Launches
```bash
# Watch logs for daemon launches
tail -f ~/.claude-squad/logs/claude-squad.log | grep daemon

# Should see:
# [INFO] started daemon child process with PID: XXXXX
# No zombie warnings should appear
```

### Long-term Monitoring
```bash
# Create a monitoring script
cat > ~/check-zombies.sh << 'EOF'
#!/bin/bash
ZOMBIE_COUNT=$(ps aux | awk '$8=="Z"' | wc -l)
if [ "$ZOMBIE_COUNT" -gt 0 ]; then
    echo "⚠️  WARNING: $ZOMBIE_COUNT zombie processes detected!"
    ps aux | awk '$8=="Z"'
else
    echo "✅ No zombie processes"
fi
EOF
chmod +x ~/check-zombies.sh

# Run periodically
~/check-zombies.sh
```

## How Process.Release() Works

**Before Fix**:
1. Parent process calls `cmd.Start()` to launch daemon
2. Daemon runs as detached process
3. When daemon exits, it sends SIGCHLD to parent
4. Parent never calls `Wait()` to reap the process
5. Process remains as zombie (Z state) indefinitely
6. Thousands of zombies accumulate over time

**After Fix**:
1. Parent process calls `cmd.Start()` to launch daemon
2. Parent calls `cmd.Process.Release()` immediately
3. OS takes over responsibility for process cleanup
4. When daemon exits, OS automatically reaps it
5. No zombie created - process is fully cleaned up

## Technical Details

### Why Daemons Need Special Handling

Daemon processes are intentionally detached from their parent:
- Parent doesn't wait for daemon to exit (daemon runs indefinitely)
- Parent exits while daemon continues running
- Without `Process.Release()`, parent's exit handler still tries to track the daemon
- This creates a zombie when the daemon eventually exits

### Process.Release() vs Wait()

**Process.Release()**:
- Tells OS: "I won't wait for this process"
- OS takes responsibility for cleanup
- Used for long-running detached processes (daemons)
- Prevents zombie creation

**cmd.Wait()**:
- Parent actively waits for child to exit
- Parent reaps child when it terminates
- Used for foreground or managed processes
- Would block if daemon runs indefinitely

## Prevention

To prevent similar issues in the future:

1. **Always call Process.Release() for detached processes**:
```go
cmd := exec.Command("daemon-process")
cmd.Start()
cmd.Process.Release() // ← Critical!
```

2. **Use Wait() for managed processes**:
```go
cmd := exec.Command("short-lived-process")
cmd.Start()
go func() {
    cmd.Wait() // Reap when done
}()
```

3. **Monitor zombie process count**:
```bash
# Add to monitoring/alerting
ps aux | awk '$8=="Z"' | wc -l
```

4. **Static analysis for process leaks**:
```bash
# Grep for Start() without Release() or Wait()
grep -r "cmd.Start()" --include="*.go" . | grep -v "Wait\|Release"
```

## Related Files

- `daemon/daemon.go` - Fixed daemon launch function
- `main.go` - Calls LaunchDaemon() on exit
- `executor/executor.go` - Properly uses cmd.Run() and cmd.Output()
- `executor/timeout_executor.go` - Properly uses cmd.Wait() in goroutines

## Conclusion

The fix is minimal but critical - a single call to `cmd.Process.Release()` prevents thousands of zombie processes from accumulating. The daemon functionality remains unchanged, but now properly integrates with OS process management.

**Status**: ✅ FIXED - Deployed in daemon/daemon.go:371-378
