# TokenLab Chat API (gpt-4.1)

Go-based chat API using TokenLab's `gpt-4.1` model with streaming, web search, per-user history, and Docker support.

## Features

| Feature | Details |
|---|---|
| Model | `gpt-4.1` (Vision, Tool Use, Web Search) |
| Streaming | SSE (Server-Sent Events) |
| History | Per-user sliding window (20 pairs) |
| CORS | Enabled for all origins |
| Deploy | Single Docker container |

## Environment Variables

| Variable | Required | Default | Description |
|---|---|---|---|
| `TOKENLAB_API_KEY` | ✅ Yes | — | Your TokenLab API key |
| `SYSTEM_PROMPT` | No | — | Custom system prompt |
| `PORT` | No | `8080` | Server port |

## Run with Docker

```bash
docker build -t tokenlab-chat .

docker run -p 8080:8080 \
  -e TOKENLAB_API_KEY=your_key_here \
  -e SYSTEM_PROMPT="তুমি নবিতা, একজন বাংলা AI সহকারী।" \
  tokenlab-chat
```

## API Endpoints

### POST /chat
Streaming response (SSE).

**Request:**
```json
{
  "user_id": "user123",
  "message": "বাংলাদেশের রাজধানী কোথায়?",
  "model": "gpt-4.1"
}
```

**Response:** `text/event-stream`
```
data: "ঢাকা"
data: " বাংলাদেশের"
data: " রাজধানী।"
data: [DONE]
```

---

### POST /chat/simple
Non-streaming JSON response.

**Request:**
```json
{
  "user_id": "user123",
  "message": "আজকের আবহাওয়া কেমন?",
  "model": "gpt-4.1"
}
```

**Response:**
```json
{
  "reply": "আজকের আবহাওয়া..."
}
```

---

### DELETE /history?user_id=user123
Clear conversation history for a user.

**Response:**
```json
{ "status": "cleared" }
```

---

### GET /health
Health check.

**Response:**
```json
{
  "status": "ok",
  "model": "gpt-4.1",
  "time": "2025-01-01T00:00:00Z"
}
```

## Clear History via Chat

Send `/clear` as message to reset history:
```json
{
  "user_id": "user123",
  "message": "/clear"
}
```

## Deploy on Sevalla

1. Push code to GitHub
2. Create new application in Sevalla
3. Set environment variables:
   - `TOKENLAB_API_KEY` = your key
   - `SYSTEM_PROMPT` = your prompt
4. Deploy — Sevalla auto-detects Dockerfile ✅
5. 
