"use client";

// analytics-exempt
import { useState, useEffect, useCallback } from "react";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import type Fuse from "fuse.js";
import { loadDocs, buildFuseIndex } from "@/lib/docs/docLoader";
import type { DocEntry } from "@/lib/docs/docLoader";
import * as styles from "./help.css";

export default function HelpPage() {
  const [docs, setDocs] = useState<DocEntry[]>([]);
  const [filteredDocs, setFilteredDocs] = useState<DocEntry[]>([]);
  const [selectedSlug, setSelectedSlug] = useState<string | null>(null);
  const [query, setQuery] = useState("");
  const [loading, setLoading] = useState(true);
  const [fuseIndex, setFuseIndex] = useState<Fuse<DocEntry> | null>(null);

  useEffect(() => {
    let cancelled = false;
    loadDocs()
      .then((entries) => {
        if (cancelled) return;
        const index = buildFuseIndex(entries);
        setDocs(entries);
        setFilteredDocs(entries);
        setFuseIndex(index);
        if (entries.length > 0) {
          setSelectedSlug(entries[0].slug);
        }
      })
      .catch(() => {
        // silently handle fetch errors — filtered/docs remain empty
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const handleSearch = useCallback(
    (value: string) => {
      setQuery(value);
      if (!value.trim() || !fuseIndex) {
        setFilteredDocs(docs);
        return;
      }
      const results = fuseIndex.search(value);
      const matched = results.map((r) => r.item);
      setFilteredDocs(matched);
      if (matched.length > 0 && !matched.find((d) => d.slug === selectedSlug)) {
        setSelectedSlug(matched[0].slug);
      }
    },
    [docs, fuseIndex, selectedSlug]
  );

  const selectedDoc = docs.find((d) => d.slug === selectedSlug) ?? null;

  return (
    <div className={styles.pageRoot} id="main-content">
      {/* Sidebar */}
      <nav className={styles.sidebar} aria-label="Documentation navigation">
        <input
          type="search"
          className={styles.searchInput}
          placeholder="Search docs…"
          value={query}
          onChange={(e) => handleSearch(e.target.value)}
          aria-label="Search documentation"
        />
        {loading ? (
          <div className={styles.loadingContainer}>Loading…</div>
        ) : (
          filteredDocs.map((doc) => (
            <button
              key={doc.slug}
              className={styles.sidebarLink({ active: doc.slug === selectedSlug })}
              onClick={() => setSelectedSlug(doc.slug)}
              aria-current={doc.slug === selectedSlug ? "page" : undefined}
            >
              {doc.title}
            </button>
          ))
        )}
      </nav>

      {/* Article pane */}
      <main className={styles.articlePane}>
        {loading ? (
          <div className={styles.loadingContainer}>Loading documentation…</div>
        ) : selectedDoc ? (
          <article className={styles.markdownBody}>
            <ReactMarkdown remarkPlugins={[remarkGfm]}>
              {selectedDoc.content}
            </ReactMarkdown>
          </article>
        ) : (
          <div className={styles.loadingContainer}>
            {query ? "No results found." : "Select a topic from the sidebar."}
          </div>
        )}
      </main>
    </div>
  );
}
