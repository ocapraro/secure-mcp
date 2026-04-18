function normalizeApiBase(): string {
  const raw = import.meta.env.VITE_API_BASE;
  if (typeof raw !== "string") return "";
  const trimmed = raw.trim().replace(/\/+$/, "");
  return trimmed.length > 0 ? trimmed : "";
}

const resolvedBase = normalizeApiBase();

/**
 * Build the URL for a same-origin API path, or an absolute origin when `VITE_API_BASE` is set.
 * A separate origin in dev avoids the browser’s per-host connection cap sharing one pool with Vite.
 */
export function apiUrl(path: string): string {
  if (!path.startsWith("/")) {
    throw new Error(`apiUrl: path must start with /, got ${path}`);
  }
  return resolvedBase ? `${resolvedBase}${path}` : path;
}
