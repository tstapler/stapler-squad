/**
 * CircularBuffer - Fixed-size buffer with O(1) operations
 *
 * Provides constant memory usage for terminal output history by maintaining
 * a fixed-capacity buffer with automatic wraparound. Ideal for preventing
 * memory leaks in long-running terminal sessions.
 *
 * Features:
 * - O(1) append operation
 * - O(1) indexed access
 * - O(n) iteration
 * - Thread-safe for single-threaded environments
 *
 * @example
 * ```typescript
 * const buffer = new CircularBuffer<TerminalLine>(10000);
 * buffer.push({ text: "Hello", sequence: 1, timestamp: Date.now() });
 * const line = buffer.get(0); // Get first line
 * ```
 */
export class CircularBuffer<T> {
  private buffer: (T | undefined)[];
  private head = 0; // Points to next write position
  private size = 0; // Current number of items

  /**
   * Create a new circular buffer with fixed capacity
   * @param capacity Maximum number of items to store
   */
  constructor(private capacity: number) {
    if (capacity <= 0) {
      throw new Error("Capacity must be positive");
    }
    this.buffer = new Array(capacity);
  }

  /**
   * Add an item to the buffer (O(1) operation)
   * If buffer is full, oldest item is overwritten
   * @param item Item to add
   */
  push(item: T): void {
    this.buffer[this.head] = item;
    this.head = (this.head + 1) % this.capacity;

    if (this.size < this.capacity) {
      this.size++;
    }
  }

  /**
   * Get item at index (O(1) operation)
   * Index 0 is the oldest item, size-1 is the newest
   * @param index Index of item (0-based, relative to oldest item)
   * @returns Item at index, or undefined if index out of bounds
   */
  get(index: number): T | undefined {
    if (index < 0 || index >= this.size) {
      return undefined;
    }

    // Calculate actual buffer position
    // If buffer is full, oldest item is at head position
    // Otherwise, oldest item is at position 0
    const start = this.size === this.capacity ? this.head : 0;
    const actualIndex = (start + index) % this.capacity;

    return this.buffer[actualIndex];
  }

  /**
   * Iterate over all items in the buffer (oldest to newest)
   * @param callback Function to call for each item
   */
  forEach(callback: (item: T, index: number) => void): void {
    for (let i = 0; i < this.size; i++) {
      const item = this.get(i);
      if (item !== undefined) {
        callback(item, i);
      }
    }
  }

  /**
   * Get all items as an array (oldest to newest)
   * Warning: This creates a new array and can be expensive for large buffers
   * @returns Array of all items
   */
  toArray(): T[] {
    const result: T[] = [];
    this.forEach((item) => result.push(item));
    return result;
  }

  /**
   * Get the current number of items in the buffer
   */
  getSize(): number {
    return this.size;
  }

  /**
   * Get the maximum capacity of the buffer
   */
  getCapacity(): number {
    return this.capacity;
  }

  /**
   * Check if the buffer is full
   */
  isFull(): boolean {
    return this.size === this.capacity;
  }

  /**
   * Check if the buffer is empty
   */
  isEmpty(): boolean {
    return this.size === 0;
  }

  /**
   * Clear all items from the buffer
   */
  clear(): void {
    this.buffer = new Array(this.capacity);
    this.head = 0;
    this.size = 0;
  }

  /**
   * Get the most recent N items (newest first)
   * @param count Number of items to retrieve
   * @returns Array of most recent items
   */
  getRecent(count: number): T[] {
    const actualCount = Math.min(count, this.size);
    const result: T[] = [];

    for (let i = 0; i < actualCount; i++) {
      const index = this.size - actualCount + i;
      const item = this.get(index);
      if (item !== undefined) {
        result.push(item);
      }
    }

    return result;
  }

  /**
   * Search for items matching a predicate
   * @param predicate Function to test each item
   * @returns Array of matching items
   */
  filter(predicate: (item: T, index: number) => boolean): T[] {
    const result: T[] = [];
    this.forEach((item, index) => {
      if (predicate(item, index)) {
        result.push(item);
      }
    });
    return result;
  }

  /**
   * Find first item matching a predicate
   * @param predicate Function to test each item
   * @returns First matching item, or undefined
   */
  find(predicate: (item: T) => boolean): T | undefined {
    for (let i = 0; i < this.size; i++) {
      const item = this.get(i);
      if (item !== undefined && predicate(item)) {
        return item;
      }
    }
    return undefined;
  }
}

/**
 * Terminal line metadata for circular buffer storage
 */
export interface TerminalLine {
  /**
   * Plain text content of the line (ANSI codes stripped)
   */
  text: string;

  /**
   * Sequence number for ordering (monotonically increasing)
   */
  sequence: number;

  /**
   * Unix timestamp when line was received (milliseconds)
   */
  timestamp: number;

  /**
   * Optional: Line number in the terminal buffer
   */
  lineNumber?: number;

  /**
   * Optional: Tags or metadata for searching/filtering
   */
  tags?: string[];
}
