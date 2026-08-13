import { ErrorInfo } from "react";
import {
  container,
  content,
  icon,
  title,
  message,
  retryButton,
  details,
  detailsSummary,
  detailsContent,
  errorBlock,
  errorText,
  stackTrace,
} from "./ErrorState.css";

interface ErrorStateProps {
  error?: Error | null;
  title?: string;
  message?: string;
  onRetry?: () => void;
  showDetails?: boolean;
  errorInfo?: ErrorInfo | null;
  actionLabel?: string;
}

export function ErrorState({
  error,
  title = "Error",
  message = "An error occurred",
  onRetry,
  showDetails = false,
  errorInfo,
  actionLabel = "Try Again",
}: ErrorStateProps) {
  return (
    <div className={container}>
      <div className={content}>
        <div className={icon}>
          <svg
            width="64"
            height="64"
            viewBox="0 0 24 24"
            fill="none"
            xmlns="http://www.w3.org/2000/svg"
          >
            <circle
              cx="12"
              cy="12"
              r="10"
              stroke="currentColor"
              strokeWidth="2"
            />
            <path
              d="M12 8V12"
              stroke="currentColor"
              strokeWidth="2"
              strokeLinecap="round"
            />
            <circle cx="12" cy="16" r="1" fill="currentColor" />
          </svg>
        </div>

        <h2 className={title}>{title}</h2>
        <p className={message}>{message}</p>

        {showDetails && error && (
          <details className={details}>
            <summary className={detailsSummary}>Error Details</summary>
            <div className={detailsContent}>
              <div className={errorBlock}>
                <strong>Error:</strong>
                <pre className={errorText}>{error.message}</pre>
              </div>

              {error.stack && (
                <div className={errorBlock}>
                  <strong>Stack Trace:</strong>
                  <pre className={stackTrace}>{error.stack}</pre>
                </div>
              )}

              {errorInfo?.componentStack && (
                <div className={errorBlock}>
                  <strong>Component Stack:</strong>
                  <pre className={stackTrace}>
                    {errorInfo.componentStack}
                  </pre>
                </div>
              )}
            </div>
          </details>
        )}

        {onRetry && (
          <button className={retryButton} onClick={onRetry}>
            {actionLabel}
          </button>
        )}
      </div>
    </div>
  );
}
