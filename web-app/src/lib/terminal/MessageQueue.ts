/**
 * MessageQueue - Async-iterable queue for outgoing terminal messages.
 *
 * Bridges React callbacks to a ConnectRPC bidirectional stream.
 * The queue implements the async iterator protocol so it can be
 * directly passed to `client.streamTerminal(queue)`.
 *
 * Usage:
 * ```typescript
 * const queue = new MessageQueue();
 * // Producer side
 * queue.push(new TerminalData({ sessionId, data: { case: "input", value: ... } }));
 * // Consumer side (ConnectRPC stream)
 * const stream = client.streamTerminal(queue);
 * // Shutdown
 * queue.close();
 * ```
 */

import { TerminalData, TerminalDataSchema } from "@/gen/session/v1/events_pb";
import { create } from "@bufbuild/protobuf";

export class MessageQueue {
  private queue: TerminalData[] = [];
  private resolve: ((value: TerminalData) => void) | null = null;
  private closed = false;

  push(msg: TerminalData) {
    if (this.closed) return;

    if (this.resolve) {
      this.resolve(msg);
      this.resolve = null;
    } else {
      this.queue.push(msg);
    }
  }

  async *[Symbol.asyncIterator]() {
    while (!this.closed || this.queue.length > 0) {
      if (this.queue.length > 0) {
        yield this.queue.shift()!;
      } else {
        const msg = await new Promise<TerminalData>((resolve) => {
          this.resolve = resolve;
        });
        // Don't yield sentinel messages (empty messages used to unblock iterator)
        if (msg.sessionId !== "" || msg.data.case !== undefined) {
          yield msg;
        }
      }
    }
  }

  close() {
    this.closed = true;
    if (this.resolve) {
      // Force unblock the iterator with a sentinel message
      // This message will be filtered out by the iterator and not sent to the server
      this.resolve(create(TerminalDataSchema, { sessionId: "", data: { case: undefined } }));
      this.resolve = null;
    }
  }

  isClosed(): boolean {
    return this.closed;
  }
}
