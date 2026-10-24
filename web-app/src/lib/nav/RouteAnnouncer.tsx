import { useEffect, useState } from "react";
import { ANNOUNCE_CLEAR_MS, ANNOUNCE_DELAY_MS } from "./policy";
import { usePathnameChange } from "./usePathnameChange";

const VISUALLY_HIDDEN = {
  position: "absolute",
  width: 1,
  height: 1,
  margin: -1,
  padding: 0,
  border: 0,
  overflow: "hidden",
  clip: "rect(0 0 0 0)",
  whiteSpace: "nowrap",
} as const;

/** Polite live region that speaks `document.title` after each pathname change (Next's own announcer is assertive; ADR-001). */
export function RouteAnnouncer() {
  const [message, setMessage] = useState("");
  const [generation, setGeneration] = useState(0);

  usePathnameChange(() => {
    // Clear now and set later in a separate task, so an identical title still counts as a change.
    setMessage("");
    setGeneration((value) => value + 1);
  });

  useEffect(() => {
    if (generation === 0) return;
    const announce = setTimeout(() => setMessage(document.title), ANNOUNCE_DELAY_MS);
    const clear = setTimeout(() => setMessage(""), ANNOUNCE_DELAY_MS + ANNOUNCE_CLEAR_MS);
    return () => {
      clearTimeout(announce);
      clearTimeout(clear);
    };
  }, [generation]);

  return (
    <div role="status" aria-live="polite" aria-atomic="true" data-testid="route-announcer" style={VISUALLY_HIDDEN}>
      {message}
    </div>
  );
}
