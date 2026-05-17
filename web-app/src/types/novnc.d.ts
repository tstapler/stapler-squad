declare module '@novnc/novnc' {
  export default class RFB {
    constructor(target: HTMLElement, url: string, options?: Record<string, unknown>);
    scaleViewport: boolean;
    resizeSession: boolean;
    clipViewport: boolean;
    qualityLevel: number;
    compressionLevel: number;
    disconnect(): void;
    focus(): void;
    blur(): void;
    addEventListener(event: string, handler: (e: CustomEvent) => void): void;
    removeEventListener(event: string, handler: (e: CustomEvent) => void): void;
  }
}
