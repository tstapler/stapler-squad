"use client";
// +feature: history-search history-list

import { useState, useEffect, useRef, useCallback } from "react";
import { usePageView } from "@/lib/analytics/usePageView";
import { useRouter } from "next/navigation";
import { SessionService } from "@/gen/session/v1/session_pb";
import { ClaudeHistoryEntry, ClaudeMessage } from "@/gen/session/v1/session_pb";
import { createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { getApiBaseUrl } from "@/lib/config";
import {
  HistorySearchResults, HistoryFilterBar, HistoryGroupView,
  HistoryDetailPanel, HistoryMessagesModal,
} from "@/components/history";
import { useHistoryFullTextSearch, SearchResultItem } from "@/lib/hooks/useHistoryFullTextSearch";
import { useHistoryFilters, GroupingStrategyLabels } from "@/lib/hooks/useHistoryFilters";
import { useHistoryGrouping } from "@/lib/hooks/useHistoryGrouping";
import { useAnalytics } from "@/lib/contexts/AnalyticsContext";
import * as styles from "./history.css";

export default function HistoryBrowserPage() {
  usePageView();
  const { track } = useAnalytics();
  const router = useRouter();

  // Core state
  const [entries, setEntries] = useState<ClaudeHistoryEntry[]>([]);
  const [nextPageToken, setNextPageToken] = useState<string>("");
  const [loadingMore, setLoadingMore] = useState(false);
  const [selectedEntry, setSelectedEntry] = useState<ClaudeHistoryEntry | null>(null);
  const [selectedIndex, setSelectedIndex] = useState(-1);
  const [messages, setMessages] = useState<ClaudeMessage[]>([]);
  const [isMessagesOpen, setShowMessages] = useState(false);
  const [loadingMessages, setLoadingMessages] = useState(false);
  const [previewMessages, setPreviewMessages] = useState<ClaudeMessage[]>([]);
  const [loadingPreview, setLoadingPreview] = useState(false);
  const [loading, setLoading] = useState(true);
  const [searching, setSearching] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [resuming, setResuming] = useState(false);
  const [messageSearchQuery, setMessageSearchQuery] = useState("");
  const [resumeTarget, setResumeTarget] = useState<ClaudeHistoryEntry | null>(null);
  const [resumeTitle, setResumeTitle] = useState("");

  // Hooks
  const { filterState, setters, derived, actions } = useHistoryFilters(entries);
  const { searchQuery, selectedModel, dateFilter, sortField, sortOrder, groupingStrategy, searchMode } = filterState;
  const { setSearchQuery, setSelectedModel, setDateFilter, setSortField, setSortOrder, setGroupingStrategy, setSearchMode } = setters;
  const { uniqueModels, filteredEntries, hasActiveFilters } = derived;
  const { clearFilters, cycleGroupingStrategy } = actions;
  const fullTextSearch = useHistoryFullTextSearch({ debounceMs: 300, autoSearch: true });
  const { groupedEntries, flatEntries } = useHistoryGrouping(filteredEntries, groupingStrategy);

  // Refs
  const clientRef = useRef<ReturnType<typeof createClient<typeof SessionService>> | null>(null);
  const searchInputRef = useRef<HTMLInputElement>(null);
  const entryListRef = useRef<HTMLDivElement>(null);

  // Initialize ConnectRPC client and load data
  useEffect(() => {
    const transport = createConnectTransport({ baseUrl: getApiBaseUrl() });
    clientRef.current = createClient(SessionService, transport);
  }, []);

  // Data loading callbacks
  const loadHistory = useCallback(async (query?: string) => {
    if (!clientRef.current) return;
    try {
      setLoading(true); setError(null);
      const response = await clientRef.current.listClaudeHistory({ pageSize: 100, searchQuery: query });
      setEntries(response.entries);
      setNextPageToken(response.nextPageToken);
    } catch (err) { setError(`Failed to load history: ${err}`); }
    finally { setLoading(false); }
  }, []);

  useEffect(() => { loadHistory(); }, [loadHistory]);

  const loadMoreHistory = useCallback(async () => {
    if (!clientRef.current || !nextPageToken || loadingMore) return;
    try {
      setLoadingMore(true); setError(null);
      const response = await clientRef.current.listClaudeHistory({ pageSize: 100, pageToken: nextPageToken });
      setEntries(prev => [...prev, ...response.entries]);
      setNextPageToken(response.nextPageToken);
    } catch (err) { setError(`Failed to load more history: ${err}`); }
    finally { setLoadingMore(false); }
  }, [nextPageToken, loadingMore]);

  const loadEntryDetail = useCallback(async (id: string) => {
    if (!clientRef.current) return;
    try {
      setError(null); setLoadingPreview(true); setPreviewMessages([]);
      const [detailResponse, messagesResponse] = await Promise.all([
        clientRef.current.getClaudeHistoryDetail({ id }),
        clientRef.current.getClaudeHistoryMessages({ id, limit: 5, tail: true }),
      ]);
      if (detailResponse.entry) setSelectedEntry(detailResponse.entry);
      if (messagesResponse.messages) setPreviewMessages([...messagesResponse.messages].reverse());
    } catch (err) { setError(`Failed to load entry details: ${err}`); }
    finally { setLoadingPreview(false); }
  }, []);

  const handleSearch = useCallback(async () => {
    setSearching(true);
    try { await loadHistory(searchQuery || undefined); }
    finally { setSearching(false); }
  }, [searchQuery, loadHistory]);

  const loadMessages = useCallback(async (id: string) => {
    if (!clientRef.current) return;
    try {
      setLoadingMessages(true); setError(null);
      const response = await clientRef.current.getClaudeHistoryMessages({ id });
      setMessages(response.messages); setShowMessages(true); setMessageSearchQuery("");
    } catch (err) { setError(`Failed to load messages: ${err}`); }
    finally { setLoadingMessages(false); }
  }, []);

  // Event handlers
  const selectEntry = useCallback((entry: ClaudeHistoryEntry, index: number) => {
    setSelectedIndex(index); loadEntryDetail(entry.id);
  }, [loadEntryDetail]);

  const handleSearchResultClick = useCallback((result: SearchResultItem) => {
    const existingEntry = entries.find(e => e.id === result.sessionId);
    if (existingEntry) {
      const index = flatEntries.indexOf(existingEntry);
      setSelectedIndex(index >= 0 ? index : 0);
      loadEntryDetail(existingEntry.id);
    } else { loadEntryDetail(result.sessionId); }
    setSearchMode("metadata");
  }, [entries, flatEntries, loadEntryDetail, setSearchMode]);

  const handleCopyId = useCallback(async (id: string) => {
    try { await navigator.clipboard.writeText(id); }
    catch (err) { console.error("Failed to copy ID:", err); }
  }, []);

  const handleExportEntry = useCallback(async (entry: ClaudeHistoryEntry) => {
    if (!clientRef.current) return;
    try {
      const response = await clientRef.current.getClaudeHistoryMessages({ id: entry.id });
      const exportData = {
        name: entry.name, id: entry.id, project: entry.project, model: entry.model,
        messageCount: entry.messageCount,
        createdAt: entry.createdAt ? new Date(Number(entry.createdAt.seconds) * 1000).toISOString() : null,
        updatedAt: entry.updatedAt ? new Date(Number(entry.updatedAt.seconds) * 1000).toISOString() : null,
        messages: response.messages.map((msg: ClaudeMessage) => ({
          role: msg.role, content: msg.content, model: msg.model,
          timestamp: msg.timestamp ? new Date(Number(msg.timestamp.seconds) * 1000).toISOString() : null,
        })),
      };
      const blob = new Blob([JSON.stringify(exportData, null, 2)], { type: "application/json" });
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = `${entry.name.replace(/[^a-z0-9]/gi, "_")}_${entry.id.substring(0, 8)}.json`;
      document.body.appendChild(a); a.click(); document.body.removeChild(a);
      URL.revokeObjectURL(url);
    } catch (err) { setError(`Failed to export: ${err}`); }
  }, []);

  const openResumeModal = useCallback((entry: ClaudeHistoryEntry) => {
    if (!entry.project) { setError("Cannot resume: No project path recorded for this session"); return; }
    setResumeTarget(entry);
    setResumeTitle(entry.name.substring(0, 60));
  }, []);

  const handleResumeSession = useCallback(async () => {
    if (!clientRef.current || !resumeTarget) return;
    if (!resumeTarget.project) { setError("Cannot resume: Project path is required"); return; }
    const title = resumeTitle.trim() || resumeTarget.name.substring(0, 60);
    try {
      setResuming(true); setError(null);
      track({ name: "history_resume_session", category: "user_action" });
      const response = await clientRef.current.createSession({
        title, path: resumeTarget.project, resumeId: resumeTarget.id, category: "Resumed",
      });
      if (response.session) { setResumeTarget(null); router.push("/"); }
    } catch (err) { setError(`Failed to resume session: ${err}`); }
    finally { setResuming(false); }
  }, [router, resumeTarget, resumeTitle, track]);

  // Keyboard navigation
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      const isInInput = document.activeElement?.tagName === "INPUT" || document.activeElement?.tagName === "TEXTAREA";
      if (resumeTarget) {
        if (e.key === "Escape") { e.preventDefault(); setResumeTarget(null); }
        return;
      }
      if (isMessagesOpen) {
        if (e.key === "Escape") { e.preventDefault(); setShowMessages(false); }
        return;
      }
      if (e.key === "/" && !isInInput) { e.preventDefault(); searchInputRef.current?.focus(); return; }
      if (e.key === "Escape") {
        if (isInInput) { (document.activeElement as HTMLElement)?.blur(); return; }
        if (searchQuery) { setSearchQuery(""); return; }
      }
      if (isInInput) return;
      if (e.key === "ArrowDown" || e.key === "j") {
        e.preventDefault();
        const newIndex = Math.min(selectedIndex + 1, flatEntries.length - 1);
        if (newIndex >= 0 && flatEntries[newIndex]) selectEntry(flatEntries[newIndex], newIndex);
        return;
      }
      if (e.key === "ArrowUp" || e.key === "k") {
        e.preventDefault();
        const newIndex = Math.max(selectedIndex - 1, 0);
        if (flatEntries[newIndex]) selectEntry(flatEntries[newIndex], newIndex);
        return;
      }
      if (e.key === "Enter" && selectedEntry) { e.preventDefault(); loadMessages(selectedEntry.id); return; }
      if (e.key === "g" || e.key === "G") { e.preventDefault(); cycleGroupingStrategy(); return; }
      if (e.key === "?") { e.preventDefault(); return; }
    };
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [resumeTarget, isMessagesOpen, searchQuery, selectedIndex, selectedEntry, flatEntries, selectEntry, loadMessages, cycleGroupingStrategy, setSearchQuery]);

  return (
    <main id="main-content" className={styles.container}>
      <div className={styles.header}>
        <h1 className={styles.title}>📚 Claude History Browser</h1>
        <div className={styles.groupingIndicator}>
          📊 {GroupingStrategyLabels[groupingStrategy]}
          <span className={styles.shortcutHint}>(Press G to cycle)</span>
        </div>
      </div>

      {error && (
        <div className={styles.errorBanner}>
          <div className={styles.errorContent}>
            <span className={styles.errorIcon}>⚠️</span>
            <div>
              <div className={styles.errorTitle}>Error</div>
              <div className="text-muted">{error}</div>
            </div>
          </div>
          <button onClick={() => loadHistory(searchQuery || undefined)} className="btn btn-secondary btn-sm">Retry</button>
          <button onClick={() => setError(null)} className="btn btn-ghost btn-sm" aria-label="Dismiss error">✕</button>
        </div>
      )}

      <HistoryFilterBar
        searchQuery={searchQuery} selectedModel={selectedModel} dateFilter={dateFilter}
        sortField={sortField} sortOrder={sortOrder} groupingStrategy={groupingStrategy} searchMode={searchMode}
        setSearchQuery={setSearchQuery} setSelectedModel={setSelectedModel} setDateFilter={setDateFilter}
        setSortField={setSortField} setSortOrder={setSortOrder} setGroupingStrategy={setGroupingStrategy}
        setSearchMode={setSearchMode} uniqueModels={uniqueModels} hasActiveFilters={hasActiveFilters}
        searching={searching} onSearch={handleSearch} onClearFilters={clearFilters}
        searchInputRef={searchInputRef} fullTextSearch={fullTextSearch}
      />

      <div className={styles.content}>
        <div className={styles.entryList} ref={entryListRef}>
          {searchMode === "fulltext" ? (
            <>
              <h2 className={styles.sectionTitle}>Full-Text Search</h2>
              <HistorySearchResults
                results={fullTextSearch.results} totalMatches={fullTextSearch.totalMatches}
                queryTimeMs={fullTextSearch.queryTimeMs} hasMore={fullTextSearch.hasMore}
                loading={fullTextSearch.loading} error={fullTextSearch.error} query={fullTextSearch.query}
                onResultClick={handleSearchResultClick} onLoadMore={fullTextSearch.loadMore}
              />
            </>
          ) : (
            <>
              <h2 className={styles.sectionTitle}>
                History ({filteredEntries.length} of {entries.length} entries{nextPageToken ? "+" : ""})
              </h2>
              <HistoryGroupView
                groupedEntries={groupedEntries} flatEntries={flatEntries} selectedEntry={selectedEntry}
                enrichedEntry={selectedEntry}
                loading={loading} entriesCount={entries.length} filteredCount={filteredEntries.length}
                hasActiveFilters={hasActiveFilters} groupingStrategy={groupingStrategy}
                onSelectEntry={selectEntry} onClearFilters={clearFilters}
              />
              {nextPageToken && (
                <div className={styles.loadMoreContainer}>
                  <button
                    onClick={loadMoreHistory}
                    disabled={loadingMore}
                    className="btn btn-secondary"
                  >
                    {loadingMore ? "Loading..." : "Load more"}
                  </button>
                </div>
              )}
            </>
          )}
        </div>
        <HistoryDetailPanel
          entry={selectedEntry} previewMessages={previewMessages} loadingPreview={loadingPreview}
          loadingMessages={loadingMessages} resuming={resuming} onResume={openResumeModal}
          onViewMessages={loadMessages} onExport={handleExportEntry} onCopyId={handleCopyId}
        />
      </div>

      <div className={styles.keyboardHints}>
        <span><kbd>/</kbd> Search</span>
        <span><kbd>↑↓</kbd> Navigate</span>
        <span><kbd>Enter</kbd> View Messages</span>
        <span><kbd>G</kbd> Cycle Grouping</span>
        <span><kbd>Esc</kbd> Clear/Close</span>
      </div>

      <HistoryMessagesModal
        open={isMessagesOpen} messages={messages} messageSearchQuery={messageSearchQuery}
        onSearchChange={setMessageSearchQuery} onClose={() => setShowMessages(false)}
      />

      {resumeTarget && (
        <div className={styles.modalOverlay} onClick={(e) => { if (e.target === e.currentTarget) setResumeTarget(null); }}>
          <div className={styles.resumeModal} role="dialog" aria-modal="true" aria-labelledby="resume-modal-title">
            <h2 id="resume-modal-title" className={styles.resumeModalTitle}>Resume Session</h2>
            <p className={styles.resumeModalSubtitle}>
              This will start a new session continuing the conversation with Claude.
            </p>
            <div className={styles.resumeModalField}>
              <label htmlFor="resume-title" className={styles.resumeModalLabel}>Session name</label>
              <input
                id="resume-title"
                type="text"
                className={styles.resumeModalInput}
                value={resumeTitle}
                onChange={(e) => setResumeTitle(e.target.value)}
                onKeyDown={(e) => { if (e.key === "Enter") handleResumeSession(); if (e.key === "Escape") setResumeTarget(null); }}
                autoFocus
                maxLength={100}
              />
            </div>
            <div className={styles.resumeModalField}>
              <span className={styles.resumeModalLabel}>Directory</span>
              <code className={styles.resumeModalPath}>{resumeTarget.project}</code>
            </div>
            <div className={styles.resumeModalActions}>
              <button onClick={() => setResumeTarget(null)} className="btn btn-secondary">Cancel</button>
              <button
                onClick={handleResumeSession}
                disabled={resuming || !resumeTitle.trim()}
                className="btn btn-primary"
              >
                {resuming ? "Starting..." : "▶️ Resume"}
              </button>
            </div>
          </div>
        </div>
      )}
    </main>
  );
}
