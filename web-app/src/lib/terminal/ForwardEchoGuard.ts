/**
 * Recognizes the redraw echo of a scroll-forward (DELIVERED/AT_TOP) in the live
 * output stream, so TerminalOutput doesn't mistake it for a genuine live resume
 * (Ctrl+L) and drop the "viewing history" banner.
 *
 * ForwardScroll's PageUp send (session/instance_scroll_forward.go) causes a real
 * redraw that echoes back as a normal frame. Content comparison, not a time
 * window or frame count, is what's invariant: the echo's arrival timing isn't
 * guaranteed (some redraws never echo at all), but it always redraws the same
 * pane content that was just captured.
 */

// Generous headroom over any real single-pane capture.
const MAX_ECHO_BUFFER = 20000;

/**
 * Strips ANSI CSI/OSC sequences (full ECMA-48 grammar, not just digits/semicolons
 * -- narrower patterns miss e.g. DECSTR's `\x1b[!p`) and all whitespace, so two
 * redraws of the same pane compare equal despite differing line-wrap columns.
 * Deliberately not truncated: with no alt-screen scrollback, the server's capture
 * can start at a different offset into the same content than the live echo.
 */
export function contentSignature(raw: string): string {
  return raw
    .replace(/\x1b\[[0-?]*[ -/]*[@-~]/g, "")
    .replace(/\x1b\][^\x07]*(\x07|\x1b\\)/g, "")
    .replace(/\s+/g, "");
}

/**
 * Longest N such that signature.slice(0, N) is a contiguous substring of
 * haystack. Monotonic in N, so a binary search needs O(log N) includes() calls
 * rather than an O(N^2) character scan.
 */
export function longestSignaturePrefixContained(signature: string, haystack: string): number {
  let lo = 0;
  let hi = signature.length;
  while (lo < hi) {
    const mid = Math.ceil((lo + hi) / 2);
    if (haystack.includes(signature.slice(0, mid))) {
      lo = mid;
    } else {
      hi = mid - 1;
    }
  }
  return lo;
}

export class ForwardEchoGuard {
  private signature: string | null = null;
  // A redraw echo can legally arrive split across several frames, where no
  // single frame is a substring match but their concatenation is. Progress must
  // strictly increase each frame to keep accumulating; a frame that doesn't
  // extend the match is judged alone and the buffer resets, so a stale partial
  // match can't "stick" and misclassify a later genuine live resume.
  private buffer = "";
  private matchedPrefixLen = 0;

  /** Records the content of a just-delivered forward; empty content disarms the guard. */
  record(content: string): void {
    this.signature = content ? contentSignature(content) : null;
    this.buffer = "";
    this.matchedPrefixLen = 0;
  }

  /** Disarms the guard (e.g. after a full-pane snapshot makes the forward stale). */
  clear(): void {
    this.signature = null;
  }

  /** Whether `output` is recognizable as the echo of the last recorded forward. */
  isSelfEcho(output: string): boolean {
    const signature = this.signature;
    if (!signature) return false;
    const outSig = contentSignature(output);
    if (outSig.length === 0) return true; // no content to judge either way; assume still mid-echo

    const buffered = (this.buffer + outSig).slice(-MAX_ECHO_BUFFER);
    const matchedLen = longestSignaturePrefixContained(signature, buffered);

    if (matchedLen >= signature.length) {
      // Whole signature seen: resolved, so the next frame is judged fresh.
      this.buffer = "";
      this.matchedPrefixLen = 0;
      return true;
    }
    if (matchedLen > this.matchedPrefixLen) {
      // Genuine progress toward completing the same multi-frame echo.
      this.buffer = buffered;
      this.matchedPrefixLen = matchedLen;
      return true;
    }
    // No progress: judge this frame on its own content, not the stale buffer.
    const isEchoAlone = outSig.includes(signature) || signature.includes(outSig);
    this.buffer = isEchoAlone ? outSig : "";
    this.matchedPrefixLen = isEchoAlone ? longestSignaturePrefixContained(signature, outSig) : 0;
    return isEchoAlone;
  }
}
