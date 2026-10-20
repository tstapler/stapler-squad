"use client";
// THROWAWAY (Task 0.1.6a): prototype of the non-virtual Accessible view (ux.md 8.10); seed of Task 1.5.4a.
import { useState } from "react";

export const ACCESSIBLE_PAGE_SIZE = 500;

export function pageLabel(page: number, total: number, size = ACCESSIBLE_PAGE_SIZE): string {
  const first = page * size + 1;
  const last = Math.min(total, (page + 1) * size);
  return `Lines ${first}-${last} of ${total}`;
}

const btn = { minHeight: 44, minWidth: 44 } as const;

export function AccessibleViewSpike({ lines }: { lines: string[] }) {
  const [page, setPage] = useState(0);
  const [goto, setGoto] = useState("");
  const [announce, setAnnounce] = useState("");
  const pages = Math.ceil(lines.length / ACCESSIBLE_PAGE_SIZE);
  const start = page * ACCESSIBLE_PAGE_SIZE;
  const label = pageLabel(page, lines.length);

  const go = () => {
    const n = Number.parseInt(goto, 10);
    if (Number.isFinite(n) && n >= 1 && n <= lines.length) {
      setPage(Math.floor((n - 1) / ACCESSIBLE_PAGE_SIZE));
      setAnnounce(`Line ${n}`);
    }
  };

  return (
    <section aria-label="Accessible view" data-testid="spike-accessible-view">
      <nav aria-label="Pager">
        <button type="button" style={btn} disabled={page === 0} onClick={() => setPage(page - 1)}>Previous page</button>
        <span data-testid="spike-page-label" aria-live="polite">{label}</span>
        <button type="button" style={btn} disabled={page >= pages - 1} onClick={() => setPage(page + 1)}>Next page</button>
        <label>
          Go to line <input type="number" inputMode="numeric" value={goto} onChange={(e) => setGoto(e.target.value)} style={{ minHeight: 44 }} />
        </label>
        <button type="button" style={btn} onClick={go}>Go</button>
      </nav>
      <div role="status" data-testid="spike-announce">{announce}</div>
      <pre tabIndex={0} aria-label={label} data-testid="spike-accessible-page" style={{ maxHeight: 300, overflow: "auto" }}>
        {lines.slice(start, start + ACCESSIBLE_PAGE_SIZE).map((text, i) => (
          <span key={start + i} data-testid="spike-accessible-line" style={{ display: "block" }}>
            {`${start + i + 1} ${text}`}
          </span>
        ))}
      </pre>
    </section>
  );
}
