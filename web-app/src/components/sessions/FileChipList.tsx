// +feature: file-chip-list
import React from "react";
import {
  File as FileIcon,
  FileCode,
  FileText,
  FileArchive,
  ImageIcon,
} from "lucide-react";
import type { LucideIcon } from "lucide-react";
import {
  chipList,
  chip,
  chipName,
  chipThumbnail,
  chipIconWrapper,
  chipRemove,
} from "./FileChipList.css";

export interface AttachedFile {
  file: File;
  path: string;
  previewUrl?: string; // only for image/* files
  name: string;
  size: number;
}

function iconForMimeType(mimeType: string): LucideIcon {
  if (mimeType.startsWith("image/")) return ImageIcon;
  if (
    mimeType.startsWith("text/") ||
    /\/(javascript|typescript|json|xml|yaml|toml)/.test(mimeType) ||
    /x-(python|go|rust|c|java|ruby|sh)/.test(mimeType)
  ) {
    return FileCode;
  }
  if (
    mimeType === "application/pdf" ||
    mimeType.includes("word") ||
    mimeType.includes("document") ||
    mimeType.includes("spreadsheet") ||
    mimeType.includes("presentation")
  ) {
    return FileText;
  }
  if (
    mimeType.includes("zip") ||
    mimeType.includes("tar") ||
    mimeType.includes("gzip") ||
    mimeType.includes("bzip") ||
    mimeType.includes("7z") ||
    mimeType.includes("rar") ||
    mimeType.includes("archive")
  ) {
    return FileArchive;
  }
  return FileIcon;
}

interface FileChipListProps {
  files: AttachedFile[];
  onRemove: (index: number) => void;
}

export function FileChipList({ files, onRemove }: FileChipListProps) {
  if (files.length === 0) return null;

  return (
    <div className={chipList} role="list" aria-label="Attached files">
      {files.map((f, i) => {
        const isImage = f.file.type.startsWith("image/") && !!f.previewUrl;
        const Icon = iconForMimeType(f.file.type);

        return (
          <div key={f.path} className={chip} role="listitem" title={f.name}>
            {isImage ? (
              // eslint-disable-next-line @next/next/no-img-element
              <img
                src={f.previewUrl}
                alt={f.name}
                className={chipThumbnail}
              />
            ) : (
              <span className={chipIconWrapper} aria-hidden="true">
                <Icon size={16} />
              </span>
            )}
            <span className={chipName}>{f.name}</span>
            <button
              type="button"
              className={chipRemove}
              onClick={() => onRemove(i)}
              aria-label={`Remove ${f.name}`}
            >
              ×
            </button>
          </div>
        );
      })}
    </div>
  );
}
