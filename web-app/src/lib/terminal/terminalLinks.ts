/** Mirrors @xterm/addon-web-links' default handler (window.open + null opener) so every renderer opens links the same way. */
export function openLinkInNewWindow(uri: string): void {
  const newWindow = window.open();
  if (!newWindow) return;
  try {
    newWindow.opener = null;
  } catch {
    // Some browsers throw when reassigning opener; navigation still proceeds.
  }
  newWindow.location.href = uri;
}

