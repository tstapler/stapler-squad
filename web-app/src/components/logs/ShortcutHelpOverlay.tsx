import { overlay, panel, title, table, row, keyCell, descCell, kbd, closeButton } from "./ShortcutHelpOverlay.css";

const SHORTCUTS = [
  { key: "/", desc: "Focus search" },
  { key: "Esc", desc: "Clear search" },
  { key: "g", desc: "Scroll to top" },
  { key: "G", desc: "Scroll to bottom" },
  { key: "=", desc: "Toggle live tail" },
  { key: "?", desc: "Toggle this help" },
];

interface ShortcutHelpOverlayProps {
  isOpen: boolean;
  onClose: () => void;
}

export function ShortcutHelpOverlay({ isOpen, onClose }: ShortcutHelpOverlayProps) {
  if (!isOpen) return null;

  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-label="Keyboard shortcuts"
      className={overlay}
      onClick={(e) => {
        // Close when clicking the backdrop
        if (e.target === e.currentTarget) onClose();
      }}
    >
      <div className={panel}>
        <h2 className={title}>Keyboard Shortcuts</h2>
        <table className={table}>
          <tbody>
            {SHORTCUTS.map((s) => (
              <tr key={s.key} className={row}>
                <td className={keyCell}>
                  <kbd className={kbd}>{s.key}</kbd>
                </td>
                <td className={descCell}>{s.desc}</td>
              </tr>
            ))}
          </tbody>
        </table>
        <button onClick={onClose} aria-label="Close shortcuts help" className={closeButton}>
          ×
        </button>
      </div>
    </div>
  );
}
