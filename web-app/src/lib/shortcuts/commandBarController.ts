/**
 * Imperative handle to the mounted command bar, so non-React callers (the
 * Alt+W alias, the Settings "Open command bar" button) can open it without
 * prop-drilling. `open` returns false when no bar is mounted or a modal dialog
 * is already open, letting callers fall back to their own behaviour.
 */
type OpenFn = (path: string[]) => boolean;

let opener: OpenFn | null = null;

export const commandBarController = {
  attach(fn: OpenFn): () => void {
    opener = fn;
    return () => {
      if (opener === fn) opener = null;
    };
  },
  open(path: string[] = []): boolean {
    return opener ? opener(path) : false;
  },
};
