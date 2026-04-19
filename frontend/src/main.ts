import "./style.css";
import DOMPurify from "dompurify";
import { marked } from "marked";
import { apiUrl } from "./apiBase";
import type { ChatMessage, SessionSummary, StoredChat } from "./sessionStore";
import {
  loadActiveSessionId,
  loadSelectedModelId,
  saveActiveSessionId,
  saveSelectedModelId,
} from "./sessionStore";
import {
  createSession,
  deleteSession,
  fetchModels,
  fetchSession,
  fetchSessionSummaries,
  type ModelInfo,
  updateSession,
} from "./sessionApi";

type OllamaChatStreamLine = {
  message?: { role?: string; content?: string };
  done?: boolean;
};

function requireEl<T extends HTMLElement>(selector: string): T {
  const el = document.querySelector<T>(selector);
  if (!el) throw new Error(`Missing required element: ${selector}`);
  return el;
}

const appEl = requireEl<HTMLElement>("#app");
const mainScrollEl = requireEl<HTMLElement>(".main-scroll");
const messagesEl = requireEl<HTMLElement>("#messages");
const emptyStateEl = requireEl<HTMLElement>("#emptyState");
const statusEl = requireEl<HTMLElement>("#status");
const formEl = requireEl<HTMLFormElement>("#composer");
const promptEl = requireEl<HTMLTextAreaElement>("#prompt");
const sendEl = requireEl<HTMLButtonElement>("#send");
const newChatEl = requireEl<HTMLButtonElement>("#newChat");
const modelSelectEl = requireEl<HTMLSelectElement>("#modelSelect");
const recentsListEl = requireEl<HTMLUListElement>("#recentsList");
const chatContextMenuEl = requireEl<HTMLDivElement>("#chatContextMenu");
const chatContextRenameEl = requireEl<HTMLButtonElement>("#chatContextRename");
const chatContextDeleteEl = requireEl<HTMLButtonElement>("#chatContextDelete");
const messageContextMenuEl = requireEl<HTMLDivElement>("#messageContextMenu");
const messageContextEditEl = requireEl<HTMLButtonElement>("#messageContextEdit");
const editMessageModalEl = requireEl<HTMLDivElement>("#editMessageModal");
const editMessageTextEl = requireEl<HTMLTextAreaElement>("#editMessageText");
const editMessageCloseEl = requireEl<HTMLButtonElement>("#editMessageClose");
const editMessageCancelEl = requireEl<HTMLButtonElement>("#editMessageCancel");
const editMessageSaveEl = requireEl<HTMLButtonElement>("#editMessageSave");
const sidebarToggleEl = document.querySelector<HTMLButtonElement>("#sidebarToggle");

let contextMenuSessionId: string | null = null;
let editingMessageIndex: number | null = null;

/** Messages per session (mutable arrays; same ref as `history` when that session is active). */
const sessionMessages = new Map<string, ChatMessage[]>();
/** Saved Ollama model id per session (aligned with server `modelId`). */
const sessionModelIds = new Map<string, string>();
/** Sessions with an in-flight LLM request. */
const inFlight = new Set<string>();

function modelOptionValues(): string[] {
  return [...modelSelectEl.options].map((o) => o.value);
}

function pickModelIdForSession(serverId: string | undefined): string {
  const opts = modelOptionValues();
  if (serverId && opts.includes(serverId)) return serverId;
  return opts[0] ?? serverId ?? "";
}

function ingestSessionFromServer(full: StoredChat) {
  sessionMessages.set(
    full.id,
    full.messages.map((m) => ({ ...m })),
  );
  sessionModelIds.set(full.id, pickModelIdForSession(full.modelId));
}

function applyModelToSelect(chatId: string | null) {
  if (!chatId) return;
  const mid = sessionModelIds.get(chatId);
  if (!mid) return;
  if (modelOptionValues().includes(mid)) {
    modelSelectEl.value = mid;
  }
}

marked.use({
  gfm: true,
  breaks: true,
});

DOMPurify.addHook("afterSanitizeAttributes", (node) => {
  if (node.tagName !== "A" || !(node instanceof HTMLAnchorElement)) return;
  node.setAttribute("target", "_blank");
  node.setAttribute("rel", "noopener noreferrer");
});

function renderAssistantHtml(markdown: string): string {
  const html = marked.parse(markdown, { async: false }) as string;
  return DOMPurify.sanitize(html);
}

let sessionSummaries: SessionSummary[] = [];
let activeChatId: string | null = null;
/** Alias for `sessionMessages.get(activeChatId)` after `bindActiveHistory()`. */
let history: ChatMessage[] = [];

function bindActiveHistory() {
  if (!activeChatId) {
    history = [];
    return;
  }
  if (!sessionMessages.has(activeChatId)) {
    sessionMessages.set(activeChatId, []);
  }
  history = sessionMessages.get(activeChatId)!;
}

function hideChatContextMenu() {
  contextMenuSessionId = null;
  chatContextMenuEl.hidden = true;
  chatContextMenuEl.setAttribute("aria-hidden", "true");
}

function hideMessageContextMenu() {
  messageContextMenuEl.hidden = true;
  messageContextMenuEl.setAttribute("aria-hidden", "true");
}

function showMessageContextMenuAt(clientX: number, clientY: number, messageIndex: number) {
  editingMessageIndex = messageIndex;
  messageContextMenuEl.hidden = false;
  messageContextMenuEl.setAttribute("aria-hidden", "false");
  messageContextMenuEl.style.left = `${clientX}px`;
  messageContextMenuEl.style.top = `${clientY}px`;
  requestAnimationFrame(() => {
    const rect = messageContextMenuEl.getBoundingClientRect();
    const pad = 6;
    let left = clientX;
    let top = clientY;
    if (rect.right > window.innerWidth - pad) {
      left = window.innerWidth - rect.width - pad;
    }
    if (rect.bottom > window.innerHeight - pad) {
      top = window.innerHeight - rect.height - pad;
    }
    if (left < pad) left = pad;
    if (top < pad) top = pad;
    messageContextMenuEl.style.left = `${left}px`;
    messageContextMenuEl.style.top = `${top}px`;
  });
}

function hideEditMessageModal() {
  editingMessageIndex = null;
  editMessageModalEl.hidden = true;
  editMessageModalEl.setAttribute("aria-hidden", "true");
}

function showEditMessageModal() {
  if (editingMessageIndex === null || editingMessageIndex >= history.length) return;
  const msg = history[editingMessageIndex];
  if (!msg) return;
  editMessageTextEl.value = msg.content;
  editMessageModalEl.hidden = false;
  editMessageModalEl.setAttribute("aria-hidden", "false");
  queueMicrotask(() => {
    editMessageTextEl.focus();
    editMessageTextEl.select();
  });
}

async function saveEditedMessage() {
  if (editingMessageIndex === null || !activeChatId) return;
  const msg = history[editingMessageIndex];
  if (!msg) return;

  const newContent = editMessageTextEl.value.trim();
  if (newContent === msg.content) {
    hideEditMessageModal();
    return;
  }

  try {
    msg.content = newContent;
    rerender();
    hideEditMessageModal();
    await persistActiveSession();
    setStatus("Message updated");
    queueMicrotask(() => setStatus(""));
  } catch (err) {
    const message = err instanceof Error ? err.message : "Failed to save message";
    setStatus(message, "error");
  }
}

function showChatContextMenuAt(clientX: number, clientY: number, sessionId: string) {
  contextMenuSessionId = sessionId;
  chatContextMenuEl.hidden = false;
  chatContextMenuEl.setAttribute("aria-hidden", "false");
  chatContextMenuEl.style.left = `${clientX}px`;
  chatContextMenuEl.style.top = `${clientY}px`;
  requestAnimationFrame(() => {
    const rect = chatContextMenuEl.getBoundingClientRect();
    const pad = 6;
    let left = clientX;
    let top = clientY;
    if (rect.right > window.innerWidth - pad) {
      left = window.innerWidth - rect.width - pad;
    }
    if (rect.bottom > window.innerHeight - pad) {
      top = window.innerHeight - rect.height - pad;
    }
    if (left < pad) left = pad;
    if (top < pad) top = pad;
    chatContextMenuEl.style.left = `${left}px`;
    chatContextMenuEl.style.top = `${top}px`;
  });
}

function applyModelOptions(models: ModelInfo[]) {
  modelSelectEl.replaceChildren();
  for (const m of models) {
    const opt = document.createElement("option");
    opt.value = m.id;
    opt.textContent = m.name;
    modelSelectEl.append(opt);
  }
  const first = models[0]?.id;
  if (!first) {
    modelSelectEl.disabled = true;
    return;
  }
  modelSelectEl.disabled = false;
  const saved = loadSelectedModelId();
  if (saved && models.some((m) => m.id === saved)) {
    modelSelectEl.value = saved;
  } else {
    modelSelectEl.value = first;
    saveSelectedModelId(first);
  }
}

modelSelectEl.addEventListener("change", () => {
  const v = modelSelectEl.value;
  saveSelectedModelId(v);
  if (!activeChatId) return;
  sessionModelIds.set(activeChatId, v);
  void (async () => {
    try {
      await updateSession(activeChatId, {
        messages: (sessionMessages.get(activeChatId) ?? []).map((m) => ({
          ...m,
        })),
        modelId: v,
      });
      await refreshSessionList();
    } catch (err) {
      const message = err instanceof Error ? err.message : "Failed to save model";
      setStatus(message, "error");
    }
  })();
});

async function refreshSessionList() {
  sessionSummaries = await fetchSessionSummaries();
  renderRecents();
}

/** PATCH session messages + model only (no list refetch — safe to run many in parallel). */
async function persistMessagesToServer(chatId: string, messages: ChatMessage[]) {
  const modelId = sessionModelIds.get(chatId) ?? modelSelectEl.value;
  await updateSession(chatId, {
    messages: messages.map((m) => ({ ...m })),
    modelId,
  });
}

async function persistActiveSession() {
  if (!activeChatId) return;
  await persistMessagesToServer(activeChatId, history);
  void refreshSessionList();
}

function renderRecents() {
  recentsListEl.replaceChildren();
  if (!sessionSummaries.length) {
    const li = document.createElement("li");
    const span = document.createElement("span");
    span.className = "recent-hint";
    span.textContent = "No chats yet";
    li.append(span);
    recentsListEl.append(li);
    return;
  }
  for (const s of sessionSummaries) {
    const li = document.createElement("li");
    const btn = document.createElement("button");
    btn.type = "button";
    btn.className = "recent-item";
    if (s.id === activeChatId) btn.classList.add("is-active");
    if (inFlight.has(s.id)) btn.classList.add("is-busy");
    btn.textContent = s.title;
    btn.title = s.modelId ? `${s.title} · ${s.modelId}` : s.title;
    btn.addEventListener("click", () => {
      void openChat(s.id);
    });
    btn.addEventListener("contextmenu", (e) => {
      e.preventDefault();
      showChatContextMenuAt(e.clientX, e.clientY, s.id);
    });
    li.append(btn);
    recentsListEl.append(li);
  }
}

async function openChat(id: string) {
  if (id === activeChatId) {
    if (window.matchMedia("(max-width: 860px)").matches) {
      appEl.classList.add("sidebar-collapsed");
    }
    return;
  }
  try {
    if (!inFlight.has(id)) {
      const full = await fetchSession(id);
      ingestSessionFromServer(full);
    }
    activeChatId = id;
    saveActiveSessionId(id);
    bindActiveHistory();
    applyModelToSelect(id);
    setStatus("");
    rerender();
    await refreshSessionList();
    if (window.matchMedia("(max-width: 860px)").matches) {
      appEl.classList.add("sidebar-collapsed");
    }
    promptEl.focus();
  } catch (err) {
    const message = err instanceof Error ? err.message : "Could not open chat";
    setStatus(message, "error");
  }
}

function setStatus(text: string, variant: "neutral" | "error" = "neutral") {
  statusEl.textContent = text;
  statusEl.classList.toggle("is-error", variant === "error");
}

function syncComposerSendState() {
  const busy = activeChatId ? inFlight.has(activeChatId) : false;
  sendEl.disabled = busy || promptEl.value.trim().length === 0;
}

function autosizePrompt() {
  promptEl.style.height = "auto";
  const h = Math.min(Math.max(promptEl.scrollHeight, 44), 200);
  promptEl.style.height = `${h}px`;
}

function syncEmptyState() {
  const hasMessages = history.length > 0;
  emptyStateEl.hidden = hasMessages;
  messagesEl.hidden = !hasMessages;
}

let scrollMainRaf: number | null = null;

/** Pixels from the bottom of `.main-scroll` still treated as “at bottom” for follow mode. */
const MAIN_SCROLL_BOTTOM_SLACK_PX = 80;

function isMainScrollNearBottom(): boolean {
  const el = mainScrollEl;
  return el.scrollHeight - el.scrollTop - el.clientHeight <= MAIN_SCROLL_BOTTOM_SLACK_PX;
}

/** When true, new tokens keep the view pinned to the bottom; set false when the user scrolls up. */
let stickMainScrollToBottom = true;

mainScrollEl.addEventListener(
  "scroll",
  () => {
    stickMainScrollToBottom = isMainScrollNearBottom();
  },
  { passive: true },
);

function scrollMainToBottom(behavior: ScrollBehavior = "auto") {
  mainScrollEl.scrollTo({ top: mainScrollEl.scrollHeight, behavior });
}

function scheduleScrollMainToBottom() {
  if (scrollMainRaf != null) return;
  scrollMainRaf = requestAnimationFrame(() => {
    scrollMainRaf = null;
    scrollMainToBottom("auto");
  });
}

/** Auto-scroll only while the user has not scrolled away from the latest messages. */
function scheduleScrollMainToBottomIfStuck() {
  if (scrollMainRaf != null) return;
  scrollMainRaf = requestAnimationFrame(() => {
    scrollMainRaf = null;
    if (stickMainScrollToBottom) scrollMainToBottom("auto");
  });
}

/** Live node for the streaming assistant bubble (rerender replaces children, so never cache across navigation). */
function activeStreamingAssistantBodyEl(): HTMLElement | null {
  const last = messagesEl.lastElementChild;
  if (!last?.classList.contains("assistant")) return null;
  const body = last.querySelector(".msg-body");
  return body instanceof HTMLElement ? body : null;
}

function renderMessage(msg: ChatMessage, index: number) {
  const wrap = document.createElement("div");
  wrap.className = `msg ${msg.role}`;
  wrap.style.cursor = "context-menu";
  wrap.setAttribute("data-message-index", String(index));

  const body = document.createElement("div");
  if (msg.role === "assistant") {
    body.className = "msg-body md";
    body.innerHTML = renderAssistantHtml(msg.content);
  } else {
    body.className = "msg-body";
    body.textContent = msg.content;
  }

  wrap.append(body);
  
  // Add context menu handler
  wrap.addEventListener("contextmenu", (e) => {
    e.preventDefault();
    const msgIndex = parseInt((e.currentTarget as HTMLElement).getAttribute("data-message-index") || "0", 10);
    showMessageContextMenuAt(e.clientX, e.clientY, msgIndex);
  });

  messagesEl.append(wrap);
}

function rerender() {
  messagesEl.replaceChildren();
  for (let i = 0; i < history.length; i++) {
    renderMessage(history[i]!, i);
  }
  syncEmptyState();
  stickMainScrollToBottom = true;
  scheduleScrollMainToBottom();
}

async function pumpOllamaNdjsonStream(
  body: ReadableStream<Uint8Array>,
  onDelta: (text: string) => void,
): Promise<void> {
  const reader = body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";

  while (true) {
    const { value, done } = await reader.read();
    if (done) break;
    buffer += decoder.decode(value, { stream: true });

    const lines = buffer.split("\n");
    buffer = lines.pop() ?? "";

    for (const line of lines) {
      const trimmed = line.trim();
      if (!trimmed) continue;
      let chunk: OllamaChatStreamLine;
      try {
        chunk = JSON.parse(trimmed) as OllamaChatStreamLine;
      } catch {
        continue;
      }
      const piece = chunk.message?.content ?? "";
      if (piece) onDelta(piece);
    }
  }

  const tail = buffer.trim();
  if (!tail) return;
  let chunk: OllamaChatStreamLine;
  try {
    chunk = JSON.parse(tail) as OllamaChatStreamLine;
  } catch {
    return;
  }
  const piece = chunk.message?.content ?? "";
  if (piece) onDelta(piece);
}

messageContextEditEl.addEventListener("click", () => {
  hideMessageContextMenu();
  showEditMessageModal();
});

editMessageSaveEl.addEventListener("click", () => {
  void saveEditedMessage();
});

editMessageCloseEl.addEventListener("click", () => {
  hideEditMessageModal();
});

editMessageCancelEl.addEventListener("click", () => {
  hideEditMessageModal();
});

editMessageTextEl.addEventListener("keydown", (e) => {
  if (e.key === "Enter" && e.ctrlKey) {
    e.preventDefault();
    void saveEditedMessage();
  }
});

chatContextRenameEl.addEventListener("click", () => {
  void (async () => {
    const id = contextMenuSessionId;
    hideChatContextMenu();
    if (!id) return;
    const current = sessionSummaries.find((s) => s.id === id)?.title ?? "";
    const newTitle = prompt("Rename chat:", current);
    if (!newTitle || newTitle.trim() === "" || newTitle.trim() === current) return;
    try {
      const modelId = sessionModelIds.get(id) ?? "";
      const messages = sessionMessages.get(id) ?? [];
      await updateSession(id, {
        title: newTitle.trim(),
        modelId,
        messages: messages.map((m) => ({ ...m })),
      });
      await refreshSessionList();
    } catch (err) {
      const message = err instanceof Error ? err.message : "Rename failed";
      setStatus(message, "error");
    }
  })();
});

chatContextDeleteEl.addEventListener("click", () => {
  void (async () => {
    const id = contextMenuSessionId;
    hideChatContextMenu();
    if (!id) return;
    if (!confirm("Delete this chat? This cannot be undone.")) return;
    try {
      inFlight.delete(id);
      sessionMessages.delete(id);
      sessionModelIds.delete(id);
      await deleteSession(id);
      if (id === activeChatId) {
        const summaries = await fetchSessionSummaries();
        if (summaries.length > 0) {
          const pick = summaries[0]!;
          activeChatId = pick.id;
          saveActiveSessionId(pick.id);
          if (!inFlight.has(pick.id)) {
            const full = await fetchSession(pick.id);
            ingestSessionFromServer(full);
          }
          bindActiveHistory();
          applyModelToSelect(activeChatId);
        } else {
          const s = await createSession({
            modelId: modelSelectEl.value,
          });
          activeChatId = s.id;
          saveActiveSessionId(s.id);
          ingestSessionFromServer(s);
          bindActiveHistory();
          applyModelToSelect(activeChatId);
        }
        setStatus("");
        rerender();
      }
      await refreshSessionList();
      syncComposerSendState();
    } catch (err) {
      const message = err instanceof Error ? err.message : "Delete failed";
      setStatus(message, "error");
    }
  })();
});

document.addEventListener(
  "pointerdown",
  (e) => {
    // Close message context menu if open
    if (!messageContextMenuEl.hidden && !messageContextMenuEl.contains(e.target as Node)) {
      hideMessageContextMenu();
    }
    // Close chat context menu if open
    if (!chatContextMenuEl.hidden && !chatContextMenuEl.contains(e.target as Node)) {
      hideChatContextMenu();
    }
    // Close edit modal if clicking the overlay
    if (!editMessageModalEl.hidden) {
      const modalContent = editMessageModalEl.querySelector(".modal-content");
      if (!modalContent?.contains(e.target as Node)) {
        hideEditMessageModal();
      }
    }
  },
  true,
);

document.addEventListener("keydown", (e) => {
  if (e.key === "Escape") {
    hideMessageContextMenu();
    hideChatContextMenu();
    hideEditMessageModal();
  }
});

newChatEl.addEventListener("click", () => {
  void (async () => {
    try {
      if (history.length > 0) {
        await persistActiveSession();
        const s = await createSession({
          modelId: modelSelectEl.value,
        });
        activeChatId = s.id;
        saveActiveSessionId(s.id);
        ingestSessionFromServer(s);
        bindActiveHistory();
        applyModelToSelect(activeChatId);
        rerender();
        await refreshSessionList();
      }
      setStatus("");
      syncComposerSendState();
      promptEl.focus();
    } catch (err) {
      const message = err instanceof Error ? err.message : "Failed to start chat";
      setStatus(message, "error");
    }
  })();
});

sidebarToggleEl?.addEventListener("click", () => {
  appEl.classList.toggle("sidebar-collapsed");
});

promptEl.addEventListener("input", () => {
  syncComposerSendState();
  autosizePrompt();
});

promptEl.addEventListener("keydown", (e) => {
  if (e.key !== "Enter" || e.shiftKey || e.isComposing) return;
  e.preventDefault();
  const text = promptEl.value.trim();
  if (!text || sendEl.disabled) return;
  formEl.requestSubmit(sendEl);
});

formEl.addEventListener("submit", (e) => {
  e.preventDefault();
  const text = promptEl.value.trim();
  if (!text) return;

  if (!activeChatId) {
    setStatus("No active session. Is the API server running?", "error");
    return;
  }

  const chatId = activeChatId;
  bindActiveHistory();
  const msgs = sessionMessages.get(chatId);
  if (!msgs) return;

  msgs.push({ role: "user", content: text });
  if (chatId === activeChatId) {
    renderMessage(msgs[msgs.length - 1]!, msgs.length - 1);
    syncEmptyState();
    stickMainScrollToBottom = true;
    scheduleScrollMainToBottom();
  }
  promptEl.value = "";
  autosizePrompt();

  inFlight.add(chatId);
  syncComposerSendState();
  if (chatId === activeChatId) setStatus("Thinking…");

  void (async () => {
    const modelId = modelSelectEl.value;
    sessionModelIds.set(chatId, modelId);
    try {
      const ac = new AbortController();
      let persistRejected: unknown = null;
      const persistP = persistMessagesToServer(chatId, msgs).catch((err) => {
        persistRejected = err;
        ac.abort();
        throw err;
      });
      const fetchP = fetch(apiUrl("/api/chat"), {
        method: "POST",
        headers: { "content-type": "application/json" },
        signal: ac.signal,
        body: JSON.stringify({
          messages: msgs,
          model: modelId,
        }),
      });

      let res: Response;
      try {
        const pair = await Promise.all([persistP, fetchP]);
        res = pair[1]!;
      } catch (err) {
        if (persistRejected) {
          throw persistRejected instanceof Error
            ? persistRejected
            : new Error("Failed to save message");
        }
        throw err instanceof Error ? err : new Error("Chat request failed");
      }

      void refreshSessionList();

      if (!res.ok) {
        const errText = await res.text().catch(() => "");
        throw new Error(errText || `Chat failed (${res.status})`);
      }

      if (!res.body) throw new Error("Missing response body");

      const assistant: ChatMessage = { role: "assistant", content: "" };
      msgs.push(assistant);

      if (chatId === activeChatId) {
        renderMessage(assistant, msgs.length - 1);
        syncEmptyState();
        scheduleScrollMainToBottomIfStuck();
      }

      await pumpOllamaNdjsonStream(res.body, (piece) => {
        assistant.content += piece;
        if (chatId === activeChatId) {
          const bodyEl = activeStreamingAssistantBodyEl();
          if (bodyEl) {
            bodyEl.classList.add("md");
            bodyEl.innerHTML = renderAssistantHtml(assistant.content);
          }
          scheduleScrollMainToBottomIfStuck();
        }
      });

      if (chatId === activeChatId) setStatus("");
      try {
        const modelId = sessionModelIds.get(chatId) ?? modelSelectEl.value;
        // Auto-name the chat after the first exchange (1 user + 1 assistant)
        const isFirstExchange = msgs.filter((m) => m.role === "user").length === 1;
        const firstUserMsg = msgs.find((m) => m.role === "user")?.content ?? "";
        const autoTitle = isFirstExchange
          ? firstUserMsg.slice(0, 60).trimEnd() + (firstUserMsg.length > 60 ? "…" : "")
          : undefined;
        await updateSession(chatId, {
          messages: msgs.map((m) => ({ ...m })),
          modelId,
          title: autoTitle,
        });
      } catch {
        /* e.g. session deleted while streaming */
      }
      void refreshSessionList();
      if (chatId === activeChatId) {
        queueMicrotask(() => {
          if (stickMainScrollToBottom) scrollMainToBottom("auto");
        });
      }
    } catch (err) {
      const message = err instanceof Error ? err.message : "Unknown error";
      if (chatId === activeChatId) {
        setStatus(message, "error");
      }
      const last = msgs[msgs.length - 1];
      if (last?.role === "assistant") {
        msgs.pop();
        msgs.pop();
      } else if (last?.role === "user") {
        msgs.pop();
      }
      if (chatId === activeChatId) rerender();
      try {
        await persistMessagesToServer(chatId, msgs);
      } catch {
        /* ignore */
      }
      void refreshSessionList();
    } finally {
      inFlight.delete(chatId);
      syncComposerSendState();
      if (chatId === activeChatId) {
        promptEl.focus();
      }
      void refreshSessionList();
    }
  })();
});

async function bootstrapApp() {
  try {
    const models = await fetchModels();
    applyModelOptions(models);

    let summaries = await fetchSessionSummaries();
    if (!summaries.length) {
      await createSession({ modelId: modelSelectEl.value });
      summaries = await fetchSessionSummaries();
    }
    let activeId = loadActiveSessionId();
    if (!activeId || !summaries.some((s) => s.id === activeId)) {
      activeId = summaries[0]!.id;
    }
    activeChatId = activeId;
    saveActiveSessionId(activeId);
    const full = await fetchSession(activeId);
    ingestSessionFromServer(full);
    bindActiveHistory();
    applyModelToSelect(activeId);
    sessionSummaries = summaries;
    rerender();
    renderRecents();
    setStatus("");
    syncComposerSendState();
    autosizePrompt();
    promptEl.focus();
  } catch (err) {
    const message = err instanceof Error ? err.message : "Could not load chats";
    setStatus(message, "error");
    sessionSummaries = [];
    activeChatId = null;
    sessionMessages.clear();
    sessionModelIds.clear();
    bindActiveHistory();
    rerender();
    renderRecents();
  }
}

if (window.matchMedia("(max-width: 860px)").matches) {
  appEl.classList.add("sidebar-collapsed");
}

void bootstrapApp();
