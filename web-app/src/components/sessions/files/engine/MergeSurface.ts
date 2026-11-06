// Port so the 3-way merge UI (ADR-034) is not welded to @codemirror/merge.
export interface MergeSurface {
  show3Way(base: string, mine: string, theirs: string): void;
  /** The merged text as it currently stands in the surface. */
  getResult(): string;
  destroy(): void;
}
