export type ChatRole = "user" | "assistant" | "system";

export type ChatMessage = {
  role: ChatRole;
  content: string;
};

export type StoredChat = {
  id: string;
  title: string;
  updatedAt: number;
  messages: ChatMessage[];
  /** Which Ollama model this chat uses (server-enforced id). */
  modelId?: string;
};

export type SessionSummary = Pick<
  StoredChat,
  "id" | "title" | "updatedAt" | "modelId"
>;

const ACTIVE_KEY = "llm-chat:active-id";
const MODEL_KEY = "llm-chat:model-id";

export function loadActiveSessionId(): string | null {
  try {
    const v = localStorage.getItem(ACTIVE_KEY);
    return v && v.length > 0 ? v : null;
  } catch {
    return null;
  }
}

export function saveActiveSessionId(id: string | null) {
  try {
    if (id) localStorage.setItem(ACTIVE_KEY, id);
    else localStorage.removeItem(ACTIVE_KEY);
  } catch {
    /* ignore */
  }
}

export function loadSelectedModelId(): string | null {
  try {
    const v = localStorage.getItem(MODEL_KEY);
    return v && v.length > 0 ? v : null;
  } catch {
    return null;
  }
}

export function saveSelectedModelId(id: string | null) {
  try {
    if (id) localStorage.setItem(MODEL_KEY, id);
    else localStorage.removeItem(MODEL_KEY);
  } catch {
    /* ignore */
  }
}
