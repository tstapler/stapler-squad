// @feature website-demos
// Caption timeline behind <name>.transcript.json (R19).

export interface TranscriptEntry {
  tMs: number;
  text: string;
}

/** Collects captions on the clip's own timeline: 0 is the first frame viewers see. */
export class CaptionLog {
  private readonly entries: TranscriptEntry[] = [];
  private zero: number;

  /**
   * @param now clock source, injectable for tests
   * @param offsetMs time viewers see before the recorded footage starts (the 1.5 s title card)
   */
  constructor(
    private readonly now: () => number = Date.now,
    private readonly offsetMs = 0,
  ) {
    this.zero = now();
  }

  /** Marks the moment page loading ends; everything before it is trimmed from the clip. */
  markReady(): void {
    this.zero = this.now();
    this.entries.length = 0;
  }

  add(text: string): void {
    this.entries.push({ tMs: Math.round(this.now() - this.zero + this.offsetMs), text });
  }

  /** Milliseconds from ready to now, plus the offset: the clip length so far. */
  elapsedMs(): number {
    return Math.round(this.now() - this.zero + this.offsetMs);
  }

  toJSON(): TranscriptEntry[] {
    return [...this.entries];
  }
}

const logs = new WeakMap<object, CaptionLog>();

export function attachCaptionLog(page: object, log: CaptionLog): void {
  logs.set(page, log);
}

export function captionLogOf(page: object): CaptionLog | undefined {
  return logs.get(page);
}
