// Replays byte streams into xterm.js headless (the web terminal's engine) and prints each
// stream's final cell grid, so the Go fidelity test can diff what the browser shows live
// against what it shows after a snapshot redraw.
// stdin:  {"cols":80,"rows":24,"streams":{"<name>":"<base64 bytes>"}}
// stdout: {"<name>":{"grid":[[ [text, fg, bg, flags, underlineStyle, underlineColor, link], ... ], ...],
//                    "state":{title, modes, cursorHidden, cursorStyle, altScreen, scrollTop, scrollBottom,
//                             tabStops, charsets, cursor}}}
//   state uses the numeric conventions of tymux's PaneSnapshot proto. xterm.js has no DECSCNM (?5) or
//   UTF-8/urxvt mouse encodings (?1005/?1015), so the oracle observes those mode sequences itself.
//   link is "<uri>\u0000<id>" (xterm.js's internal numeric link ids aren't comparable across runs).
//   xterm.js forces every hyperlinked cell's underline to "dashed" whatever SGR said, so for linked
//   cells underline style/color/flag are reported as unknown ("", "default") and compared as such.
import xterm from "@xterm/headless";
import { Unicode11Addon } from "@xterm/addon-unicode11";
import { UnicodeGraphemesAddon } from "@xterm/addon-unicode-graphemes";

const { Terminal } = xterm;
const input = JSON.parse(await new Response(Bun.stdin.stream()).text());

function color(mode, value) {
  if (mode === 0) return "default";
  return mode === 0x03000000 ? `rgb:${value.toString(16).padStart(6, "0")}` : `p:${value}`;
}

const UNDERLINE_STYLES = ["", "single", "double", "curly", "dotted", "dashed"];

const MOUSE_MODES = { none: 0, x10: 1, vt200: 2, drag: 3, any: 4 };
const MOUSE_ENCODINGS = { DEFAULT: 0, UTF8: 1, SGR: 2 };

// SCS designation of one charset slot: 0 ASCII, 1 DEC special graphics, 2 UK.
function charsetKind(set) {
  if (!set) return 0;
  if (set.q) return 1; // DEC special graphics (q = horizontal line)
  if (Object.keys(set).length === 1 && set["#"]) return 2;
  return 0; // national replacement sets (Finnish, German, ...) aren't modeled by the snapshot
}

// DECSCUSR number from xterm.js's (shape, blink); an untouched cursor is 0.
function cursorStyle(modes) {
  if (!modes.cursorStyle) return 0;
  const base = { block: 1, underline: 3, bar: 5 }[modes.cursorStyle];
  return modes.cursorBlink ? base : base + 1;
}

// xterm.js's DECRC restores the active translation table (cs.charset) but not its slot array, so
// trust the active table for whichever slot GL points at.
function charsetState(cs) {
  const kinds = [0, 1, 2, 3].map((i) => charsetKind(cs._charsets[i]));
  kinds[cs.glevel] = charsetKind(cs.charset);
  return { g0: kinds[0], g1: kinds[1], g2: kinds[2], g3: kinds[3], gl: cs.glevel };
}

function collectState(term, observed, title) {
  const core = term._core;
  const buf = core.buffer;
  const m = term.modes;
  const dec = core.coreService.decPrivateModes;
  const wholeScreen = buf.scrollTop === 0 && buf.scrollBottom === input.rows - 1;
  const cs = core._inputHandler._charsetService;
  // xterm.js implements only SGR (?1006); the UTF-8 (?1005) and urxvt (?1015) encodings are
  // tracked from the sequences themselves, last one set wins.
  let mouseEncoding = observed.encoding || (MOUSE_ENCODINGS[core.coreMouseService.activeEncoding] ?? 0);
  return {
    title,
    modes: {
      appCursorKeys: m.applicationCursorKeysMode,
      appKeypad: m.applicationKeypadMode,
      bracketedPaste: m.bracketedPasteMode,
      focusEvents: m.sendFocusMode,
      mouseMode: MOUSE_MODES[m.mouseTrackingMode],
      mouseEncoding,
      reverseVideo: observed.reverseVideo,
      origin: m.originMode,
      noAutowrap: !m.wraparoundMode,
      insert: m.insertMode,
      newline: !!core.optionsService.options.convertEol,
    },
    cursorHidden: core.coreService.isCursorHidden,
    cursorStyle: cursorStyle(dec),
    altScreen: term.buffer.active.type === "alternate",
    scrollTop: wholeScreen ? 0 : buf.scrollTop + 1,
    scrollBottom: wholeScreen ? 0 : buf.scrollBottom + 1,
    tabStops: Object.keys(buf.tabs).map(Number).filter((c) => c !== 0).sort((a, b) => a - b),
    charsets: charsetState(cs),
    cursor: { row: buf.y, col: buf.x },
  };
}

// Records ?5, ?1005 and ?1015 set/reset; returns false so xterm.js still processes the sequence.
function observePrivateModes(term, observed) {
  for (const final of ["h", "l"]) {
    term.parser.registerCsiHandler({ prefix: "?", final }, (params) => {
      for (const p of params.flat()) {
        if (p === 5) observed.reverseVideo = final === "h";
        if (p === 1005 || p === 1015) {
          const enc = p === 1005 ? 1 : 3;
          if (final === "h") observed.encoding = enc;
          else if (observed.encoding === enc) observed.encoding = 0;
        }
        if (p === 1006 && final === "h") observed.encoding = 0; // xterm.js's own SGR state takes over
      }
      return false;
    });
  }
}

async function replay(b64) {
  const term = new Terminal({ cols: input.cols, rows: input.rows, allowProposedApi: true, scrollback: 0 });
  const observed = { reverseVideo: false, encoding: 0 };
  let title = "";
  term.onTitleChange((t) => (title = t));
  observePrivateModes(term, observed);
  // ORACLE_UNICODE: "11" (default, what the web terminal loads), "graphemes", or "6" (xterm.js default).
  const uni = process.env.ORACLE_UNICODE ?? "11";
  if (uni === "11") { term.loadAddon(new Unicode11Addon()); term.unicode.activeVersion = "11"; }
  else if (uni === "graphemes") { term.loadAddon(new UnicodeGraphemesAddon()); term.unicode.activeVersion = "15-graphemes"; }
  await new Promise((resolve) => term.write(Buffer.from(b64, "base64"), resolve));
  const buf = term.buffer.active;
  const links = term._core._oscLinkService;
  const grid = [];
  for (let y = 0; y < input.rows; y++) {
    const line = buf.getLine(y);
    const row = [];
    for (let x = 0; x < input.cols; x++) {
      const c = line.getCell(x);
      const urlId = c.extended.urlId;
      const linkData = urlId ? links.getLinkData(urlId) : undefined;
      // isUnderline() is the SGR 4 flag; xterm.js can lose the style bits (DECRC), so default to single.
      const ulStyle = urlId || !c.isUnderline() ? 0 : c.getUnderlineStyle() || 1;
      const flags = [
        c.isBold() && "bold", c.isDim() && "faint", c.isItalic() && "italic",
        ulStyle !== 0 && "underline", c.isInverse() && "reverse", c.isBlink() && "blink",
        c.isInvisible() && "conceal", c.isStrikethrough() && "strike", c.isOverline() && "overline",
      ].filter(Boolean).join(",");
      row.push([
        c.getWidth() === 0 ? "" : c.getChars() || " ",
        color(c.getFgColorMode(), c.getFgColor()),
        color(c.getBgColorMode(), c.getBgColor()),
        flags,
        UNDERLINE_STYLES[ulStyle],
        urlId || ulStyle === 0 ? "default" : color(c.getUnderlineColorMode(), c.getUnderlineColor()),
        linkData ? `${linkData.uri}\u0000${linkData.id ?? ""}` : "",
      ]);
    }
    grid.push(row);
  }
  const state = collectState(term, observed, title);
  term.dispose();
  return { grid, state };
}

const out = {};
for (const [name, b64] of Object.entries(input.streams)) out[name] = await replay(b64);
process.stdout.write(JSON.stringify(out));
