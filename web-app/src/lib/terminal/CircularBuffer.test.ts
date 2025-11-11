import { CircularBuffer, TerminalLine } from './CircularBuffer';

describe('CircularBuffer', () => {
  describe('Basic Operations', () => {
    it('should create buffer with correct capacity', () => {
      const buffer = new CircularBuffer<number>(10);
      expect(buffer.getCapacity()).toBe(10);
      expect(buffer.getSize()).toBe(0);
      expect(buffer.isEmpty()).toBe(true);
      expect(buffer.isFull()).toBe(false);
    });

    it('should throw error for invalid capacity', () => {
      expect(() => new CircularBuffer<number>(0)).toThrow();
      expect(() => new CircularBuffer<number>(-1)).toThrow();
    });

    it('should push items correctly', () => {
      const buffer = new CircularBuffer<number>(3);

      buffer.push(1);
      expect(buffer.getSize()).toBe(1);
      expect(buffer.get(0)).toBe(1);

      buffer.push(2);
      expect(buffer.getSize()).toBe(2);
      expect(buffer.get(0)).toBe(1);
      expect(buffer.get(1)).toBe(2);

      buffer.push(3);
      expect(buffer.getSize()).toBe(3);
      expect(buffer.isFull()).toBe(true);
    });

    it('should handle wraparound correctly', () => {
      const buffer = new CircularBuffer<number>(3);

      // Fill buffer
      buffer.push(1);
      buffer.push(2);
      buffer.push(3);

      // Overwrite oldest items
      buffer.push(4); // Overwrites 1
      expect(buffer.getSize()).toBe(3);
      expect(buffer.get(0)).toBe(2); // Oldest is now 2
      expect(buffer.get(1)).toBe(3);
      expect(buffer.get(2)).toBe(4);

      buffer.push(5); // Overwrites 2
      expect(buffer.get(0)).toBe(3); // Oldest is now 3
      expect(buffer.get(1)).toBe(4);
      expect(buffer.get(2)).toBe(5);
    });

    it('should get items at correct indices', () => {
      const buffer = new CircularBuffer<number>(5);

      for (let i = 0; i < 5; i++) {
        buffer.push(i);
      }

      for (let i = 0; i < 5; i++) {
        expect(buffer.get(i)).toBe(i);
      }
    });

    it('should return undefined for out-of-bounds indices', () => {
      const buffer = new CircularBuffer<number>(3);
      buffer.push(1);
      buffer.push(2);

      expect(buffer.get(-1)).toBeUndefined();
      expect(buffer.get(2)).toBeUndefined();
      expect(buffer.get(10)).toBeUndefined();
    });
  });

  describe('Iteration', () => {
    it('should iterate over all items', () => {
      const buffer = new CircularBuffer<number>(5);
      const items = [1, 2, 3, 4, 5];

      items.forEach(item => buffer.push(item));

      const collected: number[] = [];
      buffer.forEach((item, index) => {
        collected.push(item);
        expect(index).toBe(collected.length - 1);
      });

      expect(collected).toEqual(items);
    });

    it('should iterate over wrapped buffer correctly', () => {
      const buffer = new CircularBuffer<number>(3);

      // Fill and wrap
      buffer.push(1);
      buffer.push(2);
      buffer.push(3);
      buffer.push(4); // Oldest is now [2, 3, 4]

      const collected: number[] = [];
      buffer.forEach(item => collected.push(item));

      expect(collected).toEqual([2, 3, 4]);
    });

    it('should convert to array correctly', () => {
      const buffer = new CircularBuffer<number>(5);
      const items = [1, 2, 3, 4, 5];

      items.forEach(item => buffer.push(item));

      expect(buffer.toArray()).toEqual(items);
    });
  });

  describe('Advanced Operations', () => {
    it('should get recent items', () => {
      const buffer = new CircularBuffer<number>(10);

      for (let i = 1; i <= 10; i++) {
        buffer.push(i);
      }

      expect(buffer.getRecent(3)).toEqual([8, 9, 10]);
      expect(buffer.getRecent(1)).toEqual([10]);
      expect(buffer.getRecent(15)).toEqual([1, 2, 3, 4, 5, 6, 7, 8, 9, 10]);
    });

    it('should filter items', () => {
      const buffer = new CircularBuffer<number>(10);

      for (let i = 1; i <= 10; i++) {
        buffer.push(i);
      }

      const evens = buffer.filter(item => item % 2 === 0);
      expect(evens).toEqual([2, 4, 6, 8, 10]);
    });

    it('should find first matching item', () => {
      const buffer = new CircularBuffer<number>(10);

      for (let i = 1; i <= 10; i++) {
        buffer.push(i);
      }

      expect(buffer.find(item => item > 5)).toBe(6);
      expect(buffer.find(item => item > 20)).toBeUndefined();
    });

    it('should clear buffer', () => {
      const buffer = new CircularBuffer<number>(5);

      for (let i = 1; i <= 5; i++) {
        buffer.push(i);
      }

      expect(buffer.getSize()).toBe(5);

      buffer.clear();

      expect(buffer.getSize()).toBe(0);
      expect(buffer.isEmpty()).toBe(true);
      expect(buffer.isFull()).toBe(false);
    });
  });

  describe('Performance', () => {
    it('should handle large buffers efficiently', () => {
      const buffer = new CircularBuffer<number>(10000);

      const start = performance.now();

      // Push 10,000 items
      for (let i = 0; i < 10000; i++) {
        buffer.push(i);
      }

      const pushTime = performance.now() - start;

      // Should complete in less than 10ms
      expect(pushTime).toBeLessThan(10);

      // Access items
      const accessStart = performance.now();
      for (let i = 0; i < 1000; i++) {
        buffer.get(i);
      }
      const accessTime = performance.now() - accessStart;

      // Should complete in less than 5ms
      expect(accessTime).toBeLessThan(5);
    });

    it('should maintain constant memory after filling', () => {
      const buffer = new CircularBuffer<number>(1000);

      // Fill buffer
      for (let i = 0; i < 1000; i++) {
        buffer.push(i);
      }

      // Overwrite many times
      for (let i = 0; i < 10000; i++) {
        buffer.push(i);
      }

      // Size should remain constant
      expect(buffer.getSize()).toBe(1000);
      expect(buffer.isFull()).toBe(true);
    });
  });

  describe('TerminalLine Integration', () => {
    it('should store terminal lines correctly', () => {
      const buffer = new CircularBuffer<TerminalLine>(100);

      const lines: TerminalLine[] = [
        { text: "Line 1", sequence: 1, timestamp: Date.now() },
        { text: "Line 2", sequence: 2, timestamp: Date.now() },
        { text: "Line 3", sequence: 3, timestamp: Date.now() },
      ];

      lines.forEach(line => buffer.push(line));

      expect(buffer.getSize()).toBe(3);
      expect(buffer.get(0)?.text).toBe("Line 1");
      expect(buffer.get(1)?.sequence).toBe(2);
      expect(buffer.get(2)?.text).toBe("Line 3");
    });

    it('should search terminal lines by content', () => {
      const buffer = new CircularBuffer<TerminalLine>(100);

      const lines: TerminalLine[] = [
        { text: "Error: Connection failed", sequence: 1, timestamp: Date.now() },
        { text: "Info: Starting server", sequence: 2, timestamp: Date.now() },
        { text: "Error: Timeout occurred", sequence: 3, timestamp: Date.now() },
        { text: "Info: Server started", sequence: 4, timestamp: Date.now() },
      ];

      lines.forEach(line => buffer.push(line));

      const errors = buffer.filter(line => line.text.startsWith("Error:"));
      expect(errors).toHaveLength(2);
      expect(errors[0].text).toBe("Error: Connection failed");
      expect(errors[1].text).toBe("Error: Timeout occurred");
    });

    it('should maintain line order with wraparound', () => {
      const buffer = new CircularBuffer<TerminalLine>(3);

      for (let i = 1; i <= 5; i++) {
        buffer.push({
          text: `Line ${i}`,
          sequence: i,
          timestamp: Date.now() + i,
        });
      }

      // Should contain lines 3, 4, 5 (oldest to newest)
      expect(buffer.getSize()).toBe(3);
      expect(buffer.get(0)?.sequence).toBe(3);
      expect(buffer.get(1)?.sequence).toBe(4);
      expect(buffer.get(2)?.sequence).toBe(5);
    });

    it('should support tagged lines', () => {
      const buffer = new CircularBuffer<TerminalLine>(100);

      const lines: TerminalLine[] = [
        { text: "Deploy started", sequence: 1, timestamp: Date.now(), tags: ["deploy", "production"] },
        { text: "Tests passed", sequence: 2, timestamp: Date.now(), tags: ["test"] },
        { text: "Deploy completed", sequence: 3, timestamp: Date.now(), tags: ["deploy", "production"] },
      ];

      lines.forEach(line => buffer.push(line));

      const deployLines = buffer.filter(line => line.tags?.includes("deploy"));
      expect(deployLines).toHaveLength(2);
    });
  });

  describe('Edge Cases', () => {
    it('should handle capacity of 1', () => {
      const buffer = new CircularBuffer<number>(1);

      buffer.push(1);
      expect(buffer.get(0)).toBe(1);

      buffer.push(2);
      expect(buffer.get(0)).toBe(2);
      expect(buffer.getSize()).toBe(1);
    });

    it('should handle empty buffer operations', () => {
      const buffer = new CircularBuffer<number>(10);

      expect(buffer.toArray()).toEqual([]);
      expect(buffer.getRecent(5)).toEqual([]);
      expect(buffer.filter(() => true)).toEqual([]);
      expect(buffer.find(() => true)).toBeUndefined();

      let callCount = 0;
      buffer.forEach(() => callCount++);
      expect(callCount).toBe(0);
    });

    it('should handle repeated wraparounds', () => {
      const buffer = new CircularBuffer<number>(3);

      // Write many more items than capacity
      for (let i = 0; i < 100; i++) {
        buffer.push(i);
      }

      // Should contain last 3 items
      expect(buffer.getSize()).toBe(3);
      expect(buffer.get(0)).toBe(97);
      expect(buffer.get(1)).toBe(98);
      expect(buffer.get(2)).toBe(99);
    });
  });
});
