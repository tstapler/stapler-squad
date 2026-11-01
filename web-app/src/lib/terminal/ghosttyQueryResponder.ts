/**
 * Answers the two terminal queries ghostty-web 0.4.0's WASM parses but never replies to:
 * DECRQM for synchronized output (`CSI ? 2026 $ p`) and the Kitty keyboard query (`CSI ? u`).
 * The renderer does not enable Kitty keyboard flags, so reporting flags 0 is accurate.
 * Hand-rolled scan instead of a regex: scrollback replays arrive as multi-MB chunks, and a
 * global regex over them overflowed the stack in the browser. A query split across chunks is carried over.
 */

const SYNC_OUTPUT_MODE = "2026";
const MAX_PARAM_LEN = 32;

export interface QueryResponder {
  /** Feeds one output chunk; returns the bytes the terminal should send back to the host (usually ""). */
  feed(chunk: string): string;
}

const isParamChar = (c: string): boolean => (c >= "0" && c <= "9") || c === ";";

export function createQueryResponder(): QueryResponder {
  let carry = "";
  let syncOutputOn = false;
  return {
    feed(chunk) {
      const text = carry + chunk;
      carry = "";
      let reply = "";
      let i = text.indexOf("\x1b");
      while (i !== -1) {
        // Each guard below either rejects this ESC (resume at the next one) or, if the text ends mid-sequence, carries it over.
        const prefix = "\x1b[?";
        let j = i;
        while (j < text.length && j - i < prefix.length && text[j] === prefix[j - i]) j++;
        if (j === text.length) {
          carry = text.slice(i);
          break;
        }
        if (j - i < prefix.length) {
          i = text.indexOf("\x1b", i + 1);
          continue;
        }
        let k = j;
        while (k < text.length && k - j <= MAX_PARAM_LEN && isParamChar(text[k])) k++;
        if (k === text.length) {
          carry = text.slice(i);
          break;
        }
        const params = text.slice(j, k);
        const final = text[k];
        let next = k + 1;
        if (final === "u" && params === "") {
          reply += "\x1b[?0u";
        } else if (final === "$") {
          if (k + 1 === text.length) {
            carry = text.slice(i);
            break;
          }
          if (text[k + 1] === "p") {
            next = k + 2;
            if (params === SYNC_OUTPUT_MODE) reply += `\x1b[?${SYNC_OUTPUT_MODE};${syncOutputOn ? 1 : 2}$y`;
          }
        } else if ((final === "h" || final === "l") && params.split(";").includes(SYNC_OUTPUT_MODE)) {
          syncOutputOn = final === "h";
        }
        i = text.indexOf("\x1b", next);
      }
      return reply;
    },
  };
}
