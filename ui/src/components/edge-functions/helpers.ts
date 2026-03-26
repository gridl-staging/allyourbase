export function formatTimeout(nanoseconds: number): string {
  const ms = Math.round(nanoseconds / 1_000_000);
  if (ms >= 1000) return `${(ms / 1000).toFixed(1)}s`;
  return `${ms}ms`;
}

export function formatLastInvoked(lastInvokedAt?: string | null): string {
  if (!lastInvokedAt) return "Never";
  return new Date(lastInvokedAt).toLocaleString();
}

export const DEFAULT_SOURCE = `export default function handler(req) {
  return {
    statusCode: 200,
    body: JSON.stringify({ message: "Hello from edge function!" }),
    headers: { "Content-Type": "application/json" },
  };
}
`;
