# known_gap: what deliberately does not round-trip

These files are replayed through the daemon so we prove it swallows them cleanly, but their
screen diffs are logged, not failed. Everything else the daemon can snapshot lives in the other
areas and must match xterm.js exactly. A case only belongs here if round-tripping it is
impossible or wrong, not merely unimplemented.

| Case | Why it can't (or shouldn't) round-trip |
|---|---|
| `sixel_images` | Sixel (DCS q) and kitty graphics (APC G) carry pixel data, not cells. The emulator has no decoder and `@xterm/headless` has no image addon, so there is nothing to snapshot or compare. |
| `bell_and_window_ops` | BEL is an event, not screen state; the raw attach stream delivers it live. `CSI t` window manipulation (resize/iconify/report) is a request to the host window, which the daemon doesn't have. |
| `terminal_replies` | DA, DSR/CPR, DECRQM and XTVERSION are queries answered by the *attached terminal* (xterm.js) back into the pty. If tymuxd answered too, every query would get two replies. Whether a headless daemon should answer when no client is attached is an open product decision, not a fidelity gap. |

State that *can* be restored (title, modes, mouse, paste, alt screen, scroll region, tab stops,
charsets, cursor shape/visibility/position) is covered by `modes/`, `charset/` and `cursor/`.
