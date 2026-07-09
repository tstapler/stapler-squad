/**
 * Strips ANSI escape sequences (SGR + OSC hyperlinks) without an npm dependency.
 *
 * The CSI branch's final-byte class is [@-~] (0x40-0x7E per ECMA-48), not just
 * letters — CSI sequences can terminate on '@' (Insert Character), '~', and
 * other non-letter bytes. A letter-only class leaves those sequences
 * unstripped, leaking raw escape bytes into the displayed text (BUG-025).
 */
export function stripAnsi(str: string): string {
  return str
    .replace(/\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)/g, "") // OSC sequences (hyperlinks, etc.)
    .replace(/\x1b[@-Z\\-_]|\x1b\[[0-9;]*[@-~]/g, ""); // CSI / single-char escapes
}
