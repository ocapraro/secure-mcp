# API Documentation

Base URL: `http://127.0.0.1:8787`

## Endpoints

| Method | Path | Description | Request Body | Response |
|--------|------|-------------|--------------|----------|
| `GET` | `/health` | Health check | — | `{ ok: true }` |
| `GET` | `/api/models` | List available models | — | `{ models: [{ id, name }] }` |
| `GET` | `/api/sessions` | List all sessions (sorted by most recent) | — | `{ sessions: [{ id, title, updatedAt, modelId }] }` |
| `POST` | `/api/sessions` | Create a new session | `{ modelId?: string }` | `{ id, title, updatedAt, messages, modelId }` (201) |
| `GET` | `/api/sessions/:id` | Get a session by ID | — | `{ id, title, updatedAt, messages, modelId }` |
| `PATCH` | `/api/sessions/:id` | Update a session's messages and/or model | `{ messages?: ChatMessage[], modelId?: string }` | `{ id, title, updatedAt, messages, modelId }` |
| `DELETE` | `/api/sessions/:id` | Delete a session by ID | — | 204 No Content |
| `POST` | `/api/chat` | Send a chat request (streamed response) | `{ messages: ChatMessage[], model?: string }` | NDJSON stream |

## Types

### `ChatMessage`

| Field | Type | Values |
|-------|------|--------|
| `role` | `string` | `"user"` \| `"assistant"` \| `"system"` |
| `content` | `string` | Message text |

### `Session`

| Field | Type | Description |
|-------|------|-------------|
| `id` | `string` | UUID |
| `title` | `string` | Derived from first user message (max 52 chars) |
| `updatedAt` | `number` | Unix timestamp (ms) |
| `messages` | `ChatMessage[]` | Conversation history |
| `modelId` | `string` | ID of the model used |

## Notes

- All endpoints support CORS (`*`).
- `POST /api/chat` streams the Ollama response as NDJSON (`application/x-ndjson`).
- If an invalid or unknown `modelId` is provided, the default model is used silently.
- Sessions are capped at 50; the oldest (by `updatedAt`) are evicted automatically.
