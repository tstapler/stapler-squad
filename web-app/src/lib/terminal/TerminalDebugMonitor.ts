import type { ITerminal } from './TerminalStreamManager';

/** True when verbose terminal debug logging is enabled via localStorage. */
function isDebugTerminalEnabled(): boolean {
  return typeof window !== "undefined" && localStorage.getItem("debug-terminal") === "true";
}

/**
 * TerminalDebugMonitor - optional write/refresh monkey-patching that logs
 * timing and call-site info when the `debug-terminal` localStorage flag is
 * set. Debug-only instrumentation, not on the hot write path.
 *
 * Split out of TerminalStreamManager.ts, which owns one instance per manager
 * and drives it purely through install()/restore().
 */
export class TerminalDebugMonitor {
  private originalWrite: ITerminal['write'] | null = null;
  private originalRefresh: ITerminal['refresh'] | null = null;
  private lastWriteTime = 0;
  private writeCount = 0;
  private installed = false;

  /** Wrap terminal.write/refresh to log timing/call-site info. Idempotent. */
  install(terminal: ITerminal): void {
    if (this.installed) return;

    this.originalWrite = terminal.write.bind(terminal);
    this.originalRefresh = terminal.refresh.bind(terminal);

    terminal.write = this.wrapWrite(terminal);
    terminal.refresh = this.wrapRefresh(terminal);

    this.installed = true;
    if (isDebugTerminalEnabled()) {
      console.log('[TerminalStreamManager] Refresh and write monitoring installed');
    }
  }

  /** Restore the original terminal.write/refresh captured by install(). Idempotent. */
  restore(terminal: ITerminal): void {
    if (!this.installed || !this.originalWrite || !this.originalRefresh) return;

    terminal.write = this.originalWrite;
    terminal.refresh = this.originalRefresh;
    this.installed = false;

    // Null out refs to terminal internals to prevent GC retention (Pitfall #7):
    // when the terminal is disposed while the manager is still alive, these refs
    // hold closed terminal objects preventing garbage collection.
    this.originalWrite = null;
    this.originalRefresh = null;
  }

  private wrapWrite(terminal: ITerminal): ITerminal['write'] {
    return (data: string | Uint8Array, callback?: () => void) => {
      const now = performance.now();
      const timeSinceLastWrite = now - this.lastWriteTime;
      this.lastWriteTime = now;
      this.writeCount++;

      if (isDebugTerminalEnabled()) {
        console.log('[XtermWrite]', {
          writeCount: this.writeCount,
          dataLength: typeof data === 'string' ? data.length : data.byteLength,
          timeSinceLastWrite: `${timeSinceLastWrite.toFixed(2)}ms`,
          cursorY: terminal.buffer.active.cursorY,
          timestamp: new Date().toISOString()
        });
      }

      return this.originalWrite?.(data, callback);
    };
  }

  private wrapRefresh(terminal: ITerminal): ITerminal['refresh'] {
    return (start: number, end: number) => {
      if (isDebugTerminalEnabled()) {
        const stackTrace = new Error().stack;
        const caller = stackTrace?.split('\n')[2]?.trim() || 'unknown';
        const timeSinceLastWrite = performance.now() - this.lastWriteTime;

        console.log('[XtermRefresh] Refresh called', {
          start,
          end,
          rows: terminal.rows,
          timeSinceLastWrite: `${timeSinceLastWrite.toFixed(2)}ms`,
          recentWrites: this.writeCount,
          caller: caller.replace(/^at /, ''),
          possibleRaceCondition: timeSinceLastWrite < 50,
          timestamp: new Date().toISOString()
        });

        this.writeCount = 0;
      }

      return this.originalRefresh?.(start, end);
    };
  }
}
