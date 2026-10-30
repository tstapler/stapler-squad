/**
 * Answers the two terminal queries ghostty-web 0.4.0's WASM parses but never replies to:
 * DECRQM for synchronized output (`CSI ? 2026 $ p`) and the Kitty keyboard query (`CSI ? u`).
 * The renderer does not enable Kitty keyboard flags, so reporting flags 0 is accurate.
 * Plain string scan, so a query split across two chunks is carried over rather than missed.
 */

const SYNC_OUTPUT_MODE = "2026";
const QUERY_OR_MODE = /\x1b\[\?(?:u|([\d;]+)(\$p|[hl]))/g;
const INCOMPLETE_TAIL = /\x1b(?:\[(?:\?[\d;]*\$?)?)?$/;

export interface QueryResponder {
  /** Feeds one output chunk; returns the bytes the terminal should send back to the host (usually ""). */
  feed(chunk: string): string;
}

export function createQueryResponder(): QueryResponder {
  let carry = "";
  let syncOutputOn = false;
  return {
    feed(chunk) {
      const text = carry + chunk;
      const tail = INCOMPLETE_TAIL.exec(text);
      carry = tail ? tail[0] : "";
      const scan = tail ? text.slice(0, tail.index) : text;
      let reply = "";
      for (const m of scan.matchAll(QUERY_OR_MODE)) {
        const [seq, params, op] = m;
        if (seq === "\x1b[?u") {
          reply += "\x1b[?0u";
          continue;
        }
        const modes = params.split(";");
        if (!modes.includes(SYNC_OUTPUT_MODE)) continue;
        if (op === "$p") reply += `\x1b[?${SYNC_OUTPUT_MODE};${syncOutputOn ? 1 : 2}$y`;
        else syncOutputOn = op === "h";
      }
      return reply;
    },
  };
}
