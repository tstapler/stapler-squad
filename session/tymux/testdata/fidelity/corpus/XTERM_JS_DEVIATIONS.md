# Where xterm.js (the oracle) deviates from the spec

The goldens are what xterm.js shows, because that's what the browser shows. Where xterm.js is
itself wrong, a spec-following daemon can't match it, so the sequence is kept out of the corpus
instead of pinning the bug. Each entry names the spec reading and what xterm.js does.

| Sequence | Spec | xterm.js |
|---|---|---|
| `ESC ~`, `ESC }`, `ESC |` (LS1R/LS2R/LS3R) | Invoke G1/G2/G3 into **GR** only; GL is unchanged. | Treats them as GL shifts (`setgLevel`). |
| DECRC (`ESC 8`) after `?6h` + DECSC | Restores origin mode (DECOM). | Leaves DECOM as it was. |
| `ESC - A`, `ESC . A`, `ESC / A` (96-character sets) | Designate G1/G2/G3 as ISO Latin-1 supplement. | Maps them to the UK 94-set. |
| SGR 59 | Resets the underline color to default. | Sets it to white. |
| SGR 6 (rapid blink), `38;5;>255`, truncated color params, invalid `4:N` | Rapid blink = blink; invalid params ignored. | Inconsistent, or no-op. |
| `?5` DECSCNM, `?1005`, `?1015` | Reverse video / UTF-8 / urxvt mouse encodings. | Not implemented; the oracle observes the mode sequences itself (`oracle.mjs`). |
| SGR 4 inside an OSC 8 link | Underline as specified. | Forces every linked cell's underline to dashed. |
| Invalid UTF-8 (lone continuation, truncated/overlong sequences, surrogates, 0xFE/0xFF) | Print U+FFFD per ill-formed subsequence (Unicode ch. 3 "U+FFFD substitution"; RFC 3629). | Drops the bytes silently. Covered by the Rust unit tests in `crates/vt100` instead. |
| U+008D (RI) as a UTF-8 code point | Acts as Reverse Index. | Ignored. |

Not a deviation but out of schema: national replacement character sets (`ESC ( C`/`K`/`5`..., VT220 NRCS). The snapshot models ASCII, DEC graphics and UK only, so NRCS designations are kept out of the corpus.
