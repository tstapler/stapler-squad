// xterm modifier key sequences (CSI parameter convention: 2=Shift, 3=Alt, 5=Ctrl).
// Keyed by the unmodified sequence the toolbar sends; callers fall back to the
// input (or an ESC prefix, for Alt) when a key has no entry.

export const CTRL_KEY_MAP: Record<string, string> = {
  '\x1b[A': '\x1b[1;5A',  // Ctrl+Up
  '\x1b[B': '\x1b[1;5B',  // Ctrl+Down
  '\x1b[C': '\x1b[1;5C',  // Ctrl+Right (word forward)
  '\x1b[D': '\x1b[1;5D',  // Ctrl+Left (word back)
  '\x1b[H': '\x1b[1;5H',  // Ctrl+Home
  '\x1b[F': '\x1b[1;5F',  // Ctrl+End
  '\x1b[5~': '\x1b[5;5~', // Ctrl+PgUp
  '\x1b[6~': '\x1b[6;5~', // Ctrl+PgDn
  '/': '\x1f',             // Ctrl+/ (unit separator)
  '-': '\x1f',             // Ctrl+- (maps to Ctrl+_)
};

export const ALT_KEY_MAP: Record<string, string> = {
  '\x1b[A': '\x1b[1;3A',  // Alt+Up
  '\x1b[B': '\x1b[1;3B',  // Alt+Down
  '\x1b[C': '\x1b[1;3C',  // Alt+Right (word forward)
  '\x1b[D': '\x1b[1;3D',  // Alt+Left (word back)
  '\x1b[H': '\x1b[1;3H',  // Alt+Home
  '\x1b[F': '\x1b[1;3F',  // Alt+End
  '\x1b[5~': '\x1b[5;3~', // Alt+PgUp
  '\x1b[6~': '\x1b[6;3~', // Alt+PgDn
};

export const SHIFT_KEY_MAP: Record<string, string> = {
  '\t':      '\x1b[Z',      // Shift+Tab (backtab / dedent)
  '\x1b[A':  '\x1b[1;2A',  // Shift+Up
  '\x1b[B':  '\x1b[1;2B',  // Shift+Down
  '\x1b[C':  '\x1b[1;2C',  // Shift+Right
  '\x1b[D':  '\x1b[1;2D',  // Shift+Left
  '\x1b[H':  '\x1b[1;2H',  // Shift+Home
  '\x1b[F':  '\x1b[1;2F',  // Shift+End
  '\x1b[5~': '\x1b[5;2~',  // Shift+PgUp
  '\x1b[6~': '\x1b[6;2~',  // Shift+PgDn
};
