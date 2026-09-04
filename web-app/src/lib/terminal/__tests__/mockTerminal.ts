/**
 * Shared MockTerminal test double for TerminalStreamManager.test.ts and
 * pipeline-integration.test.ts. Both files previously defined byte-for-byte
 * identical `class MockTerminal implements ITerminal` bodies (jscpd flagged
 * the duplication) — this is the single source of truth for it.
 *
 * The method surface here is the union of what each test file needs
 * (getWrittenItems/getRefreshCalls/wasScrolledToBottom are only used by
 * TerminalStreamManager.test.ts; getAllWrittenText only by
 * pipeline-integration.test.ts) so both can import the same class unmodified.
 */

import type { ITerminal } from '../TerminalStreamManager';

export class MockTerminal implements ITerminal {
  rows = 24;
  cols = 80;
  private written: Array<{ data: string; callback?: () => void }> = [];
  private cleared = false;
  private refreshed: Array<{ start: number; end: number }> = [];
  private scrolledToBottom = false;

  write(data: string | Uint8Array, callback?: () => void): void {
    const str = typeof data === 'string' ? data : new TextDecoder().decode(data);
    this.written.push({ data: str, callback });
    // Auto-invoke callback to simulate xterm.js processing
    callback?.();
  }

  clear(): void {
    this.cleared = true;
  }

  refresh(start: number, end: number): void {
    this.refreshed.push({ start, end });
  }

  scrollToBottom(): void {
    this.scrolledToBottom = true;
  }

  get buffer() {
    return {
      active: { cursorY: 0, viewportY: 0, length: 0 },
      normal: { length: 0 },
    };
  }

  // Test helpers
  getWrittenData(): string[] {
    return this.written.map(w => w.data);
  }

  getWrittenItems(): Array<{ data: string; callback?: () => void }> {
    return [...this.written];
  }

  getAllWrittenText(): string {
    return this.written.map(w => w.data).join('');
  }

  wasCleared(): boolean {
    return this.cleared;
  }

  getRefreshCalls(): Array<{ start: number; end: number }> {
    return [...this.refreshed];
  }

  wasScrolledToBottom(): boolean {
    return this.scrolledToBottom;
  }

  resetTracking(): void {
    this.written = [];
    this.cleared = false;
    this.refreshed = [];
    this.scrolledToBottom = false;
  }
}
