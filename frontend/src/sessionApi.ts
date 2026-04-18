import { apiUrl } from "./apiBase";
import type { ChatMessage, SessionSummary, StoredChat } from "./sessionStore";

export type ModelInfo = {
  id: string;
  name: string;
};

export async function fetchModels(): Promise<ModelInfo[]> {
  const res = await fetch(apiUrl("/api/models"));
  if (!res.ok) {
    throw new Error(`Failed to list models (${res.status})`);
  }
  const data = (await res.json()) as { models?: ModelInfo[] };
  return Array.isArray(data.models) ? data.models : [];
}

export async function fetchSessionSummaries(): Promise<SessionSummary[]> {
  const res = await fetch(apiUrl("/api/sessions"));
  if (!res.ok) {
    throw new Error(`Failed to list sessions (${res.status})`);
  }
  const data = (await res.json()) as { sessions?: SessionSummary[] };
  return Array.isArray(data.sessions) ? data.sessions : [];
}

export async function fetchSession(id: string): Promise<StoredChat> {
  const res = await fetch(apiUrl(`/api/sessions/${encodeURIComponent(id)}`));
  if (!res.ok) {
    throw new Error(`Failed to load session (${res.status})`);
  }
  return (await res.json()) as StoredChat;
}

export async function createSession(init?: {
  modelId?: string;
}): Promise<StoredChat> {
  const res = await fetch(apiUrl("/api/sessions"), {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify(init ?? {}),
  });
  if (!res.ok) {
    throw new Error(`Failed to create session (${res.status})`);
  }
  return (await res.json()) as StoredChat;
}

export async function deleteSession(id: string): Promise<void> {
  const res = await fetch(apiUrl(`/api/sessions/${encodeURIComponent(id)}`), {
    method: "DELETE",
  });
  if (res.status === 404) return;
  if (!res.ok) {
    const text = await res.text().catch(() => "");
    throw new Error(text || `Failed to delete session (${res.status})`);
  }
}

export type SessionPatch = {
  messages?: ChatMessage[];
  modelId?: string;
};

export async function updateSession(
  id: string,
  patch: SessionPatch,
): Promise<StoredChat> {
  if (patch.messages === undefined && patch.modelId === undefined) {
    throw new Error("Nothing to update");
  }
  const res = await fetch(apiUrl(`/api/sessions/${encodeURIComponent(id)}`), {
    method: "PATCH",
    headers: { "content-type": "application/json" },
    body: JSON.stringify(patch),
  });
  if (!res.ok) {
    const text = await res.text().catch(() => "");
    throw new Error(text || `Failed to save session (${res.status})`);
  }
  return (await res.json()) as StoredChat;
}
