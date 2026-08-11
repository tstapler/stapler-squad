import type { Timestamp } from "@bufbuild/protobuf/wkt";

export type DateFilter = "all" | "today" | "week" | "month";

export const formatTimeAgo = (timestamp: Timestamp | undefined): string => {
  if (!timestamp) return "N/A";
  const now = Date.now();
  const date = new Date(Number(timestamp.seconds) * 1000);
  const seconds = Math.floor((now - date.getTime()) / 1000);

  if (seconds < 60) return "just now";
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m ago`;
  if (seconds < 86400) return `${Math.floor(seconds / 3600)}h ago`;
  if (seconds < 604800) return `${Math.floor(seconds / 86400)}d ago`;
  return date.toLocaleDateString();
};

export const formatDate = (timestamp: Timestamp | undefined): string => {
  if (!timestamp) return "N/A";
  const date = new Date(Number(timestamp.seconds) * 1000);
  return date.toLocaleString();
};

export const truncateMiddle = (str: string, maxLength: number): string => {
  if (str.length <= maxLength) return str;
  const ellipsis = "...";
  const charsToShow = maxLength - ellipsis.length;
  const frontChars = Math.ceil(charsToShow / 2);
  const backChars = Math.floor(charsToShow / 2);
  return str.substring(0, frontChars) + ellipsis + str.substring(str.length - backChars);
};

export const getDateGroup = (timestamp: Timestamp | undefined): string => {
  if (!timestamp) return "Unknown";
  const date = new Date(Number(timestamp.seconds) * 1000);
  const now = new Date();
  const today = new Date(now.getFullYear(), now.getMonth(), now.getDate());
  const yesterday = new Date(today.getTime() - 86400000);
  const weekAgo = new Date(today.getTime() - 7 * 86400000);
  const monthAgo = new Date(today.getTime() - 30 * 86400000);

  if (date >= today) return "Today";
  if (date >= yesterday) return "Yesterday";
  if (date >= weekAgo) return "This Week";
  if (date >= monthAgo) return "This Month";
  return "Older";
};

export const isWithinDateFilter = (timestamp: Timestamp | undefined, filter: DateFilter): boolean => {
  if (filter === "all" || !timestamp) return true;
  const date = new Date(Number(timestamp.seconds) * 1000);
  const now = new Date();
  const today = new Date(now.getFullYear(), now.getMonth(), now.getDate());

  switch (filter) {
    case "today":
      return date >= today;
    case "week":
      return date >= new Date(today.getTime() - 7 * 86400000);
    case "month":
      return date >= new Date(today.getTime() - 30 * 86400000);
    default:
      return true;
  }
};
