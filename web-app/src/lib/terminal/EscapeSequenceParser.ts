/**
 * EscapeSequenceParser - Ensures ANSI escape sequences are not split across writes
 *
 * Terminal control codes (ANSI escape sequences) must be written atomically to xterm.js.
 * Splitting them mid-sequence causes terminal corruption. This parser detects partial
 * escape sequences at the end of data chunks and buffers them for the next write.
 *
 * Supported sequences:
 * - CSI: \x1b[ ... (letter)  - Cursor positioning, colors, attributes
 * - OSC: \x1b] ... \x07|\x1b\\ - Operating system commands
 * - Simple: \x1b(letter) - Single-char escapes
 *
 * References:
 * - xterm.js Flow Control Guide: https://xtermjs.org/docs/guides/flowcontrol/
 * - xterm.js Terminal API: https://xtermjs.org/docs/api/terminal/classes/terminal/
 * - ANSI Escape Sequences: https://en.wikipedia.org/wiki/ANSI_escape_code
 * - VT100 Control Sequences: https://vt100.net/docs/vt100-ug/chapter3.html
 */

export class EscapeSequenceParser {
  private partialSequence: string = "";

  /**
   * Process data chunk and ensure escape sequences are not split.
   * Returns the complete data that can be safely written, buffering any
   * partial escape sequence for the next call.
   *
   * @param data - New data chunk to process
   * @returns Complete data safe to write (includes buffered partial from previous call)
   */
  public processChunk(data: string): string {
    // Prepend any buffered partial sequence from previous call
    const fullData = this.partialSequence + data;
    this.partialSequence = "";

    // Check if data ends with a partial escape sequence
    const partial = this.findPartialEscapeAtEnd(fullData);

    if (partial.length > 0) {
      // Buffer the partial sequence for next call
      this.partialSequence = partial;

      // Return data up to (but not including) the partial sequence
      return fullData.substring(0, fullData.length - partial.length);
    }

    // No partial sequence - safe to write all data
    return fullData;
  }

  /**
   * Get any buffered partial escape sequence (for debugging/testing)
   */
  public getBuffered(): string {
    return this.partialSequence;
  }

  /**
   * Clear any buffered data (call on disconnect/error)
   */
  public reset(): void {
    this.partialSequence = "";
  }

  /**
   * Detect partial escape sequence at end of string.
   * Returns the partial sequence string if found, empty string otherwise.
   *
   * Detection strategy:
   * 1. Scan backwards from end for ESC character (\x1b)
   * 2. Validate if this starts a complete or partial sequence
   * 3. CSI/OSC sequences need terminator, simple escapes need one more char
   */
  private findPartialEscapeAtEnd(data: string): string {
    if (data.length === 0) return "";

    // Maximum length to scan backward (escape sequences rarely exceed 20 bytes)
    const scanLength = Math.min(20, data.length);
    const startIndex = data.length - scanLength;

    // Scan backward for last ESC character
    let lastEscIndex = -1;
    for (let i = data.length - 1; i >= startIndex; i--) {
      if (data.charCodeAt(i) === 0x1b) {
        lastEscIndex = i;
        break;
      }
    }

    if (lastEscIndex === -1) {
      return ""; // No escape sequence at end
    }

    // Extract potential escape sequence from last ESC to end
    const potentialSeq = data.substring(lastEscIndex);

    // Check if this is a complete sequence or partial
    if (this.isCompleteEscapeSequence(potentialSeq)) {
      return ""; // Complete sequence - no buffering needed
    }

    // Partial sequence - buffer it
    return potentialSeq;
  }

  /**
   * Check if escape sequence is complete (has terminator).
   *
   * ANSI escape sequence patterns:
   * - CSI: ESC [ ... (letter A-Z, a-z)
   * - OSC: ESC ] ... BEL(\x07) or ESC\
   * - Simple: ESC (letter)
   * - C1: ESC (special char in 0x40-0x5F range)
   */
  private isCompleteEscapeSequence(seq: string): boolean {
    if (seq.length === 0 || seq.charCodeAt(0) !== 0x1b) {
      return true; // Not an escape sequence
    }

    if (seq.length === 1) {
      return false; // Just ESC - definitely partial
    }

    const secondChar = seq[1];

    // CSI sequence: ESC [
    if (secondChar === '[') {
      // CSI ends with a letter (A-Z, a-z) after optional parameters
      // Examples: ESC[31m (red), ESC[2K (clear line), ESC[10;20H (cursor position)
      return this.hasCSITerminator(seq);
    }

    // OSC sequence: ESC ]
    if (secondChar === ']') {
      // OSC ends with BEL (\x07) or ESC\ (\x1b\x5c)
      return this.hasOSCTerminator(seq);
    }

    // Simple escape: ESC + single char
    // If we have 2+ characters and second is not [ or ], it's likely complete
    // Examples: ESC 7 (save cursor), ESC 8 (restore cursor), ESC M (reverse index)
    if (seq.length === 2) {
      return true; // Simple 2-char escape is complete
    }

    // C1 control codes: ESC + char in 0x40-0x5F range
    const secondCharCode = seq.charCodeAt(1);
    if (secondCharCode >= 0x40 && secondCharCode <= 0x5F) {
      return true; // C1 sequence is complete
    }

    // Unknown sequence type - assume complete to avoid infinite buffering
    return true;
  }

  /**
   * Check if CSI sequence has terminator (letter A-Z, a-z).
   * CSI format: ESC [ (params) (letter)
   */
  private hasCSITerminator(seq: string): boolean {
    if (seq.length < 3) return false; // Need at least ESC[X

    // Scan for terminator (letter)
    for (let i = 2; i < seq.length; i++) {
      const code = seq.charCodeAt(i);

      // Terminators are letters A-Z (0x41-0x5A) or a-z (0x61-0x7A)
      if ((code >= 0x41 && code <= 0x5A) || (code >= 0x61 && code <= 0x7A)) {
        return true; // Found terminator
      }

      // Valid intermediate characters: digits, semicolon, ? , etc.
      // If we hit an invalid character, sequence is malformed
      const isValidIntermediate =
        (code >= 0x30 && code <= 0x3F) || // 0-9, :, ;, <, =, >, ?
        (code >= 0x20 && code <= 0x2F);   // space, !, ", #, $, %, &, ', (, ), *, +, comma, -, ., /

      if (!isValidIntermediate) {
        // Unexpected character - treat as complete to avoid blocking
        return true;
      }
    }

    // No terminator found yet - partial sequence
    return false;
  }

  /**
   * Check if OSC sequence has terminator (BEL or ESC\).
   * OSC format: ESC ] (params) BEL or ESC ]  (params) ESC \
   */
  private hasOSCTerminator(seq: string): boolean {
    if (seq.length < 3) return false; // Need at least ESC]X

    // Check for BEL terminator (\x07)
    if (seq.indexOf('\x07') !== -1) {
      return true;
    }

    // Check for ESC\ terminator (\x1b\x5c or \x1b\\)
    if (seq.indexOf('\x1b\\') !== -1 || seq.indexOf('\x1b\x5c') !== -1) {
      return true;
    }

    // No terminator found - partial sequence
    return false;
  }
}

/**
 * Example usage:
 *
 * const parser = new EscapeSequenceParser();
 *
 * // Chunk 1: Ends with partial CSI sequence
 * const chunk1 = "Hello \x1b[31";
 * const toWrite1 = parser.processChunk(chunk1); // Returns "Hello " (buffers "\x1b[31")
 *
 * // Chunk 2: Completes the CSI sequence
 * const chunk2 = "mWorld";
 * const toWrite2 = parser.processChunk(chunk2); // Returns "\x1b[31mWorld" (prepends buffered)
 */
