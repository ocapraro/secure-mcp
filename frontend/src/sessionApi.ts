import { apiUrl } from "./apiBase";
import type { ChatMessage, SessionSummary, StoredChat } from "./sessionStore";

export type ModelInfo = {
  id: string;
  name: string;
};

type BackendModelsResponse = {
  models?: string[];
};

type BackendMessage = {
  role?: string;
  content?: string;
};

type BackendSession = {
  id?: number | string;
  title?: string;
  updated_at?: string;
  model?: string;
  messages?: BackendMessage[];
};

function toMs(ts: string | undefined): number {
  if (!ts) return Date.now();
  const n = Date.parse(ts);
  return Number.isNaN(n) ? Date.now() : n;
}

function toStoredChat(s: BackendSession): StoredChat {
  const id = s.id === undefined ? "" : String(s.id);
  const messages = Array.isArray(s.messages)
    ? s.messages
        .filter((m): m is Required<Pick<BackendMessage, "role" | "content">> => {
          return (
            (m.role === "user" || m.role === "assistant" || m.role === "system") &&
            typeof m.content === "string"
          );
        })
        .map((m) => ({ role: m.role, content: m.content }))
    : [];

  return {
    id,
    title: s.title ?? "New chat",
    updatedAt: toMs(s.updated_at),
    messages,
    modelId: s.model,
  };
}

function toSessionSummary(s: BackendSession): SessionSummary {
  return {
    id: s.id === undefined ? "" : String(s.id),
    title: s.title ?? "New chat",
    updatedAt: toMs(s.updated_at),
    modelId: s.model,
  };
}

export async function fetchModels(): Promise<ModelInfo[]> {
  const res = await fetch(apiUrl("/api/models"));
  if (!res.ok) {
    throw new Error(`Failed to list models (${res.status})`);
  }
  const data = (await res.json()) as BackendModelsResponse;
  return Array.isArray(data.models)
    ? data.models.map((name) => ({ id: name, name }))
    : [];
}

export async function fetchSessionSummaries(): Promise<SessionSummary[]> {
  const res = await fetch(apiUrl("/api/sessions"));
  if (!res.ok) {
    throw new Error(`Failed to list sessions (${res.status})`);
  }
  const data = (await res.json()) as BackendSession[];
  return Array.isArray(data) ? data.map(toSessionSummary) : [];
}

export async function fetchSession(id: string): Promise<StoredChat> {
  const res = await fetch(apiUrl(`/api/sessions/${encodeURIComponent(id)}`));
  if (!res.ok) {
    throw new Error(`Failed to load session (${res.status})`);
  }
  const data = (await res.json()) as BackendSession;
  return toStoredChat(data);
}

export async function createSession(init?: {
  modelId?: string;
}): Promise<StoredChat> {
  const payload = {
    title: "New chat",
    model: init?.modelId ?? "",
  };
  const res = await fetch(apiUrl("/api/sessions"), {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify(payload),
  });
  if (!res.ok) {
    throw new Error(`Failed to create session (${res.status})`);
  }
  const data = (await res.json()) as BackendSession;
  return toStoredChat(data);
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
  title?: string;
};

export async function updateSession(
  id: string,
  patch: SessionPatch,
): Promise<StoredChat> {
  if (patch.messages === undefined && patch.modelId === undefined) {
    throw new Error("Nothing to update");
  }

  const payload = {
    title: patch.title ?? "",
    model: patch.modelId ?? "",
    messages: (patch.messages ?? []).map((m) => ({
      role: m.role,
      content: m.content,
    })),
  };

  const res = await fetch(apiUrl(`/api/sessions/${encodeURIComponent(id)}`), {
    method: "PATCH",
    headers: { "content-type": "application/json" },
    body: JSON.stringify(payload),
  });
  if (!res.ok) {
    const text = await res.text().catch(() => "");
    throw new Error(text || `Failed to save session (${res.status})`);
  }
  const data = (await res.json()) as BackendSession;
  return toStoredChat(data);
}
