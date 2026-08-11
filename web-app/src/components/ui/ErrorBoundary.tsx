"use client";

import React, { Component, ErrorInfo, ReactNode } from "react";
import { ErrorState } from "./ErrorState";

interface Props {
  children: ReactNode;
  fallback?: ReactNode;
  onError?: (error: Error, errorInfo: ErrorInfo) => void;
}

interface State {
  hasError: boolean;
  error: Error | null;
  errorInfo: ErrorInfo | null;
}

export class ErrorBoundary extends Component<Props, State> {
  constructor(props: Props) {
    super(props);
    this.state = {
      hasError: false,
      error: null,
      errorInfo: null,
    };
  }

  static getDerivedStateFromError(error: Error): Partial<State> {
    return {
      hasError: true,
      error,
    };
  }

  componentDidCatch(error: Error, errorInfo: ErrorInfo) {
    console.error("Error caught by boundary:", error, errorInfo);

    // ChunkLoadError means the browser has a stale build cached. Reload once to get fresh chunks.
    if (error.name === "ChunkLoadError" || error.message.includes("Loading chunk")) {
      try {
        const key = "chunkload_reload_attempted";
        if (!sessionStorage.getItem(key)) {
          sessionStorage.setItem(key, "1");
          window.location.reload();
          return;
        }
      } catch {
        // sessionStorage unavailable (e.g. SecurityError in hardened contexts) — fall through to error UI
      }
    }

    this.setState({
      errorInfo,
    });

    // Call optional error handler
    this.props.onError?.(error, errorInfo);
  }

  handleReset = () => {
    this.setState({
      hasError: false,
      error: null,
      errorInfo: null,
    });
  };

  render() {
    if (this.state.hasError) {
      // Use custom fallback if provided
      if (this.props.fallback) {
        return this.props.fallback;
      }

      // Default error display
      return (
        <ErrorState
          error={this.state.error}
          title="Something went wrong"
          message="An unexpected error occurred. Please try again."
          onRetry={this.handleReset}
          showDetails={process.env.NODE_ENV === "development"}
          errorInfo={this.state.errorInfo}
        />
      );
    }

    return this.props.children;
  }
}
