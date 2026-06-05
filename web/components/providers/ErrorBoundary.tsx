"use client";

import { Component, type ReactNode, type ErrorInfo } from "react";
import { InlineNotification, Button } from "@carbon/react";

interface Props {
  children: ReactNode;
  fallback?: ReactNode;
}

interface State {
  hasError: boolean;
  error: Error | null;
}

/**
 * React error boundary that shows a Carbon InlineNotification on caught errors.
 * Wrap each module page to prevent a single component error from crashing the shell.
 */
export class ErrorBoundary extends Component<Props, State> {
  constructor(props: Props) {
    super(props);
    this.state = { hasError: false, error: null };
  }

  static getDerivedStateFromError(error: Error): State {
    return { hasError: true, error };
  }

  override componentDidCatch(error: Error, info: ErrorInfo): void {
    console.error("[ErrorBoundary]", error, info.componentStack);
  }

  reset = () => {
    this.setState({ hasError: false, error: null });
  };

  override render(): ReactNode {
    if (this.state.hasError) {
      if (this.props.fallback) return this.props.fallback;

      return (
        <div style={{ padding: "2rem" }}>
          <InlineNotification
            kind="error"
            title="Something went wrong"
            subtitle={this.state.error?.message ?? "An unexpected error occurred."}
          />
          <Button
            kind="ghost"
            style={{ marginTop: "1rem" }}
            onClick={this.reset}
          >
            Try again
          </Button>
        </div>
      );
    }

    return this.props.children;
  }
}
