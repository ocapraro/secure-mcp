import http from "node:http";
import { randomUUID } from "node:crypto";

const PORT = Number(process.env.PORT ?? "8787");
const OLLAMA_BASE_URL =
  process.env.OLLAMA_BASE_URL ??
  "http://llm-endpoint.bonobo-pineapplefish.ts.net:11434";
const OLLAMA_MODEL = process.env.OLLAMA_MODEL ?? "llama3.2:1b";
/** Second model exposed in GET /api/models (override with OLLAMA_ALT_MODEL). */
const OLLAMA_ALT_MODEL = process.env.OLLAMA_ALT_MODEL ?? "gemma4:e4b";
const MAX_SESSIONS = 50;

function configuredModelIds(): string[] {
  return [...new Set([OLLAMA_MODEL, OLLAMA_ALT_MODEL])];
}

function pickChatModel(body: object): string {
  const allowed = new Set(configuredModelIds());
  const raw = (body as { model?: unknown }).model;
  if (typeof raw === "string" && allowed.has(raw)) return raw;
  return OLLAMA_MODEL;
}

function pickSessionModelId(body: object): string {
  const allowed = new Set(configuredModelIds());
  const raw = (body as { modelId?: unknown }).modelId;
  if (typeof raw === "string" && allowed.has(raw)) return raw;
  return OLLAMA_MODEL;
}

function normalizeOptionalModelId(raw: unknown): string | undefined {
  const allowed = new Set(configuredModelIds());
  if (typeof raw === "string" && allowed.has(raw)) return raw;
  return undefined;
}

type ChatMessage = {
  role: "user" | "assistant" | "system";
  content: string;
};

type Session = {
  id: string;
  title: string;
  updatedAt: number;
  messages: ChatMessage[];
  modelId: string;
};

const sessions = new Map<string, Session>();

function deriveTitle(messages: ChatMessage[]): string {
  const first = messages.find((m) => m.role === "user");
  if (!first) return "New chat";
  const t = first.content.trim().replace(/\s+/g, " ");
  if (!t) return "New chat";
  return t.length > 52 ? `${t.slice(0, 49)}…` : t;
}

function pruneToLatest() {
  const sorted = [...sessions.values()].sort(
    (a, b) => b.updatedAt - a.updatedAt,
  );
  const keep = new Set(sorted.slice(0, MAX_SESSIONS).map((s) => s.id));
  for (const id of sessions.keys()) {
    if (!keep.has(id)) sessions.delete(id);
  }
}

function sendJson(
  res: http.ServerResponse,
  status: number,
  body: unknown,
  extraHeaders?: http.OutgoingHttpHeaders,
) {
  const payload = JSON.stringify(body);
  res.writeHead(status, {
    "content-type": "application/json; charset=utf-8",
    "content-length": Buffer.byteLength(payload),
    ...extraHeaders,
  });
  res.end(payload);
}

function setCors(res: http.ServerResponse) {
  res.setHeader("access-control-allow-origin", "*");
  res.setHeader(
    "access-control-allow-headers",
    "content-type, authorization",
  );
  res.setHeader(
    "access-control-allow-methods",
    "GET,POST,PATCH,DELETE,OPTIONS",
  );
}

async function readJsonBody(req: http.IncomingMessage): Promise<unknown> {
  const chunks: Buffer[] = [];
  for await (const chunk of req) {
    chunks.push(chunk as Buffer);
  }
  const raw = Buffer.concat(chunks).toString("utf8");
  if (!raw) return null;
  return JSON.parse(raw) as unknown;
}

function handleListModels(res: http.ServerResponse) {
  const models = configuredModelIds().map((id) => ({ id, name: id }));
  sendJson(res, 200, { models });
}

function normalizeMessages(raw: unknown): ChatMessage[] | null {
  if (!Array.isArray(raw)) return null;
  const normalized: ChatMessage[] = [];
  for (const m of raw) {
    if (!m || typeof m !== "object") continue;
    const role = (m as { role?: unknown }).role;
    const content = (m as { content?: unknown }).content;
    if (role !== "user" && role !== "assistant" && role !== "system") continue;
    if (typeof content !== "string") continue;
    normalized.push({ role, content });
  }
  return normalized;
}

function handleListSessions(res: http.ServerResponse) {
  const list = [...sessions.values()]
    .sort((a, b) => b.updatedAt - a.updatedAt)
    .map((s) => ({
      id: s.id,
      title: s.title,
      updatedAt: s.updatedAt,
      modelId: s.modelId,
    }));
  sendJson(res, 200, { sessions: list });
}

function handleGetSession(res: http.ServerResponse, id: string) {
  const s = sessions.get(id);
  if (!s) {
    sendJson(res, 404, { error: "Session not found" });
    return;
  }
  sendJson(res, 200, s);
}

async function handleCreateSession(
  req: http.IncomingMessage,
  res: http.ServerResponse,
) {
  let body: object = {};
  try {
    const raw = await readJsonBody(req);
    if (raw && typeof raw === "object") body = raw as object;
  } catch {
    sendJson(res, 400, { error: "Invalid JSON body" });
    return;
  }
  const id = randomUUID();
  const now = Date.now();
  const modelId = pickSessionModelId(body);
  const s: Session = {
    id,
    title: "New chat",
    updatedAt: now,
    messages: [],
    modelId,
  };
  sessions.set(id, s);
  pruneToLatest();
  sendJson(res, 201, s);
}

async function handlePatchSession(
  req: http.IncomingMessage,
  res: http.ServerResponse,
  id: string,
) {
  let body: unknown;
  try {
    body = await readJsonBody(req);
  } catch {
    sendJson(res, 400, { error: "Invalid JSON body" });
    return;
  }
  if (!body || typeof body !== "object") {
    sendJson(res, 400, { error: "Expected JSON object body" });
    return;
  }
  const bodyObj = body as Record<string, unknown>;
  const hasMessages = "messages" in bodyObj && bodyObj.messages !== undefined;
  const hasModelId = "modelId" in bodyObj && bodyObj.modelId !== undefined;
  if (!hasMessages && !hasModelId) {
    sendJson(res, 400, { error: "Expected messages and/or modelId" });
    return;
  }

  const existing = sessions.get(id);
  if (!existing) {
    sendJson(res, 404, { error: "Session not found" });
    return;
  }

  let messages: ChatMessage[];
  if (hasMessages) {
    const normalized = normalizeMessages(bodyObj.messages);
    if (normalized === null) {
      sendJson(res, 400, { error: "Expected messages array" });
      return;
    }
    messages = normalized;
  } else {
    messages = existing.messages;
  }

  let modelId = existing.modelId;
  if (hasModelId) {
    const nextModel = normalizeOptionalModelId(bodyObj.modelId);
    if (nextModel === undefined) {
      sendJson(res, 400, { error: "Invalid modelId" });
      return;
    }
    modelId = nextModel;
  }

  const next: Session = {
    ...existing,
    messages,
    modelId,
    title: deriveTitle(messages),
    updatedAt: Date.now(),
  };
  sessions.set(id, next);
  pruneToLatest();
  sendJson(res, 200, next);
}

function handleDeleteSession(res: http.ServerResponse, id: string) {
  if (!sessions.has(id)) {
    sendJson(res, 404, { error: "Session not found" });
    return;
  }
  sessions.delete(id);
  res.writeHead(204);
  res.end();
}

async function handleChat(
  req: http.IncomingMessage,
  res: http.ServerResponse,
) {
  let body: unknown;
  try {
    body = await readJsonBody(req);
  } catch {
    sendJson(res, 400, { error: "Invalid JSON body" });
    return;
  }

  if (!body || typeof body !== "object") {
    sendJson(res, 400, { error: "Expected JSON object body" });
    return;
  }

  const normalized = normalizeMessages((body as { messages?: unknown }).messages);
  if (!normalized || normalized.length === 0) {
    sendJson(res, 400, { error: "messages must contain at least one entry" });
    return;
  }

  const model = pickChatModel(body as object);

  const upstream = await fetch(new URL("/api/chat", OLLAMA_BASE_URL), {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({
      model,
      messages: normalized,
      stream: true,
    }),
  });

  if (!upstream.ok) {
    const text = await upstream.text().catch(() => "");
    sendJson(res, 502, {
      error: "Ollama request failed",
      status: upstream.status,
      detail: text.slice(0, 4000),
    });
    return;
  }

  if (!upstream.body) {
    sendJson(res, 502, { error: "Ollama response had no body" });
    return;
  }

  res.writeHead(200, {
    "content-type": "application/x-ndjson; charset=utf-8",
    "cache-control": "no-cache",
    connection: "close",
  });

  const reader = upstream.body.getReader();
  try {
    while (true) {
      const { value, done } = await reader.read();
      if (done) break;
      if (value.byteLength) res.write(Buffer.from(value));
    }
  } catch {
    // If the client disconnects mid-stream, ignore.
  } finally {
    if (!res.writableEnded) res.end();
  }
}

const server = http.createServer(async (req, res) => {
  setCors(res);

  if (req.method === "OPTIONS") {
    res.writeHead(204);
    res.end();
    return;
  }

  const host = req.headers.host ?? "127.0.0.1";
  const pathname = new URL(req.url ?? "/", `http://${host}`).pathname;

  if (req.method === "GET" && pathname === "/health") {
    sendJson(res, 200, { ok: true });
    return;
  }

  if (req.method === "GET" && pathname === "/api/models") {
    handleListModels(res);
    return;
  }

  const sessionMatch = pathname.match(/^\/api\/sessions\/([^/]+)$/);

  if (req.method === "GET" && pathname === "/api/sessions") {
    handleListSessions(res);
    return;
  }

  if (req.method === "GET" && sessionMatch) {
    handleGetSession(res, sessionMatch[1]!);
    return;
  }

  if (req.method === "POST" && pathname === "/api/sessions") {
    await handleCreateSession(req, res);
    return;
  }

  if (req.method === "PATCH" && sessionMatch) {
    await handlePatchSession(req, res, sessionMatch[1]!);
    return;
  }

  if (req.method === "DELETE" && sessionMatch) {
    handleDeleteSession(res, sessionMatch[1]!);
    return;
  }

  if (req.method === "POST" && pathname === "/api/chat") {
    await handleChat(req, res);
    return;
  }

  sendJson(res, 404, { error: "Not found" });
});

server.listen(PORT, () => {
  console.log(
    `Dummy LLM API listening on http://127.0.0.1:${PORT} → Ollama ${OLLAMA_BASE_URL} (${OLLAMA_MODEL})`,
  );
});
