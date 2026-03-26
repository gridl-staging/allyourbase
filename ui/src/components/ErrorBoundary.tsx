import { Component, Fragment, type ErrorInfo, type ReactNode } from "react";

type ErrorBoundaryProps = {
  children: ReactNode;
};

type ErrorBoundaryState = {
  hasError: boolean;
  resetKey: number;
};

export class ErrorBoundary extends Component<ErrorBoundaryProps, ErrorBoundaryState> {
  state: ErrorBoundaryState = {
    hasError: false,
    resetKey: 0,
  };

  static getDerivedStateFromError(): Pick<ErrorBoundaryState, "hasError"> {
    return { hasError: true };
  }

  componentDidCatch(error: Error, errorInfo: ErrorInfo): void {
    console.error("Unhandled render crash", error, errorInfo);
  }

  private handleRetry = (): void => {
    this.setState((currentState) => ({
      hasError: false,
      resetKey: currentState.resetKey + 1,
    }));
  };

  private handleReload = (): void => {
    window.location.reload();
  };

  render() {
    if (this.state.hasError) {
      return (
        <div className="flex min-h-screen items-center justify-center bg-gray-50 p-6 dark:bg-gray-950">
          <div className="w-full max-w-md rounded-lg border border-red-200 bg-white p-6 shadow-sm dark:border-red-900/60 dark:bg-gray-900">
            <h1 className="text-xl font-semibold text-red-700 dark:text-red-300">Something went wrong</h1>
            <p className="mt-2 text-sm text-gray-600 dark:text-gray-300">
              The dashboard hit an unexpected rendering error.
            </p>
            <div className="mt-4 flex gap-3">
              <button
                type="button"
                onClick={this.handleRetry}
                className="rounded-md bg-red-600 px-4 py-2 text-sm font-medium text-white hover:bg-red-700 dark:bg-red-500 dark:hover:bg-red-400"
              >
                Retry
              </button>
              <button
                type="button"
                onClick={this.handleReload}
                className="rounded-md border border-gray-300 px-4 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50 dark:border-gray-600 dark:text-gray-200 dark:hover:bg-gray-800"
              >
                Reload
              </button>
            </div>
          </div>
        </div>
      );
    }

    return <Fragment key={this.state.resetKey}>{this.props.children}</Fragment>;
  }
}
