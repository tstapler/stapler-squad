"use client";

import { useState, useEffect, useRef } from "react";
import { SessionService } from "@/gen/session/v1/session_connect";
import { ClaudeHistoryEntry, ClaudeMessage } from "@/gen/session/v1/session_pb";
import { createPromiseClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import styles from "./history.module.css";

export default function HistoryBrowserPage() {
  const [entries, setEntries] = useState<ClaudeHistoryEntry[]>([]);
  const [selectedEntry, setSelectedEntry] = useState<ClaudeHistoryEntry | null>(null);
  const [messages, setMessages] = useState<ClaudeMessage[]>([]);
  const [showMessages, setShowMessages] = useState(false);
  const [loadingMessages, setLoadingMessages] = useState(false);
  const [searchQuery, setSearchQuery] = useState("");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const clientRef = useRef<any>(null);

  // Initialize ConnectRPC client
  useEffect(() => {
    const transport = createConnectTransport({
      baseUrl: window.location.origin,
    });
    clientRef.current = createPromiseClient(SessionService, transport);
  }, []);

  // Load history on mount
  useEffect(() => {
    loadHistory();
  }, []);

  const loadHistory = async (query?: string) => {
    if (!clientRef.current) return;

    try {
      setLoading(true);
      setError(null);
      const response = await clientRef.current.listClaudeHistory({
        limit: 100,
        searchQuery: query,
      });
      setEntries(response.entries);
    } catch (err) {
      setError(`Failed to load history: ${err}`);
    } finally {
      setLoading(false);
    }
  };

  const loadEntryDetail = async (id: string) => {
    if (!clientRef.current) return;

    try {
      setError(null);
      const response = await clientRef.current.getClaudeHistoryDetail({ id });
      if (response.entry) {
        setSelectedEntry(response.entry);
      }
    } catch (err) {
      setError(`Failed to load entry details: ${err}`);
    }
  };

  const handleSearch = () => {
    loadHistory(searchQuery || undefined);
  };

  const loadMessages = async (id: string) => {
    if (!clientRef.current) return;

    try {
      setLoadingMessages(true);
      setError(null);
      const response = await clientRef.current.getClaudeHistoryMessages({ id });
      setMessages(response.messages);
      setShowMessages(true);
    } catch (err) {
      setError(`Failed to load messages: ${err}`);
    } finally {
      setLoadingMessages(false);
    }
  };

  const formatDate = (timestamp: any) => {
    if (!timestamp) return "N/A";
    const date = new Date(Number(timestamp.seconds) * 1000);
    return date.toLocaleString();
  };

  return (
    <div className={styles.container}>
      <h1 className={styles.title}>
        📚 Claude History Browser
      </h1>

      {error && (
        <div className="alert alert-error">
          {error}
        </div>
      )}

      {/* Search bar */}
      <div className={styles.searchBar}>
        <input
          type="text"
          placeholder="Search history..."
          value={searchQuery}
          onChange={(e) => setSearchQuery(e.target.value)}
          onKeyDown={(e) => e.key === "Enter" && handleSearch()}
          className="input"
          style={{ flex: 1 }}
        />
        <button
          onClick={handleSearch}
          className="btn btn-primary"
        >
          Search
        </button>
        <button
          onClick={() => {
            setSearchQuery("");
            loadHistory();
          }}
          className="btn btn-secondary"
        >
          Clear
        </button>
      </div>

      <div className={styles.content}>
        {/* Entry list */}
        <div className={styles.entryList}>
          <h2 className={styles.sectionTitle}>
            History ({entries.length} entries)
          </h2>
          {loading ? (
            <div className={styles.loadingContainer}>
              <div className="spinner" />
              <div className={styles.loadingTitle}>
                Loading Claude History...
              </div>
              <div className="text-muted" style={{ fontSize: "14px" }}>
                {entries.length === 0
                  ? "This may take a few moments on first load while scanning conversation files..."
                  : "Refreshing..."}
              </div>
            </div>
          ) : (
            <div className={styles.entryCards}>
              {entries.map((entry) => (
                <div
                  key={entry.id}
                  onClick={() => loadEntryDetail(entry.id)}
                  className={`card ${selectedEntry?.id === entry.id ? styles.selected : ""}`}
                  style={{ cursor: "pointer", transition: "all 0.2s" }}
                >
                  <div className={styles.entryName}>
                    {entry.name}
                  </div>
                  <div className="text-secondary" style={{ fontSize: "13px" }}>
                    {formatDate(entry.updatedAt)} • {entry.model} • {entry.messageCount}{" "}
                    messages
                  </div>
                  {entry.project && (
                    <div
                      className="text-muted"
                      style={{
                        fontSize: "12px",
                        marginTop: "5px",
                        fontFamily: "monospace",
                      }}
                    >
                      📁 {entry.project}
                    </div>
                  )}
                </div>
              ))}
            </div>
          )}
        </div>

        {/* Detail panel */}
        <div className={styles.detailPanel}>
          {selectedEntry ? (
            <div>
              <h2 className={styles.sectionTitle}>
                Entry Details
              </h2>
              <div className={styles.detailFields}>
                <div className={styles.detailField}>
                  <div className={styles.fieldLabel}>Name:</div>
                  <div className="text-primary">{selectedEntry.name}</div>
                </div>
                <div className={styles.detailField}>
                  <div className={styles.fieldLabel}>ID:</div>
                  <div className="text-muted" style={{ fontFamily: "monospace", fontSize: "12px" }}>
                    {selectedEntry.id}
                  </div>
                </div>
                {selectedEntry.project && (
                  <div className={styles.detailField}>
                    <div className={styles.fieldLabel}>Project:</div>
                    <div style={{ fontFamily: "monospace", fontSize: "13px", color: "var(--primary)" }}>
                      {selectedEntry.project}
                    </div>
                  </div>
                )}
                <div className={styles.detailField}>
                  <div className={styles.fieldLabel}>Model:</div>
                  <div className="text-primary">{selectedEntry.model}</div>
                </div>
                <div className={styles.detailField}>
                  <div className={styles.fieldLabel}>Message Count:</div>
                  <div className="text-primary">{selectedEntry.messageCount}</div>
                </div>
                <div className={styles.detailField}>
                  <div className={styles.fieldLabel}>Created:</div>
                  <div className="text-secondary" style={{ fontSize: "13px" }}>
                    {formatDate(selectedEntry.createdAt)}
                  </div>
                </div>
                <div className={styles.detailField}>
                  <div className={styles.fieldLabel}>Last Updated:</div>
                  <div className="text-secondary" style={{ fontSize: "13px" }}>
                    {formatDate(selectedEntry.updatedAt)}
                  </div>
                </div>
                <button
                  onClick={() => loadMessages(selectedEntry.id)}
                  disabled={loadingMessages}
                  className="btn btn-primary"
                  style={{ marginTop: "10px" }}
                >
                  {loadingMessages ? "Loading..." : "View Messages"}
                </button>
              </div>
            </div>
          ) : (
            <div className={styles.emptyState}>
              Select an entry to view details
            </div>
          )}
        </div>
      </div>

      {/* Messages Modal */}
      {showMessages && (
        <div
          className="modal-overlay"
          onClick={() => setShowMessages(false)}
        >
          <div
            className="modal"
            onClick={(e) => e.stopPropagation()}
          >
            {/* Modal Header */}
            <div className="modal-header">
              <h2 style={{ fontSize: "20px", fontWeight: "bold", margin: 0 }}>
                Conversation Messages ({messages.length})
              </h2>
              <button
                onClick={() => setShowMessages(false)}
                style={{
                  background: "none",
                  border: "none",
                  fontSize: "24px",
                  cursor: "pointer",
                  padding: "0 10px",
                  color: "var(--text-primary)",
                }}
              >
                ×
              </button>
            </div>

            {/* Messages List */}
            <div className="modal-content">
              {messages.map((msg, idx) => (
                <div
                  key={idx}
                  className={msg.role === "user" ? styles.messageUser : styles.messageAssistant}
                >
                  <div className={styles.messageHeader}>
                    <div
                      style={{
                        fontWeight: "600",
                        color: msg.role === "user" ? "var(--primary)" : "var(--text-secondary)",
                        textTransform: "capitalize",
                      }}
                    >
                      {msg.role}
                    </div>
                    <div className="text-muted" style={{ fontSize: "12px" }}>
                      {formatDate(msg.timestamp)}
                    </div>
                  </div>
                  <div className={styles.messageContent}>
                    {msg.content}
                  </div>
                  {msg.model && (
                    <div
                      className="text-muted"
                      style={{
                        fontSize: "11px",
                        marginTop: "8px",
                        fontFamily: "monospace",
                      }}
                    >
                      Model: {msg.model}
                    </div>
                  )}
                </div>
              ))}
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
