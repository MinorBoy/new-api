# Endpoints

new-api is compatible with both **OpenAI-style** and **Anthropic-style** APIs. The Base URL is `https://<your-domain>`, and every endpoint below is relative to it.

## Base URL and authentication

Every request carries the API key in an `Authorization` header:

```http
Authorization: Bearer <YOUR_API_KEY>
```

> Anthropic-style endpoints also accept `Authorization: Bearer ...`. When using the official Claude SDK, set `base_url` to `https://<your-domain>`.

## OpenAI-style endpoints

| Endpoint                   | Method    | Description                   |
| -------------------------- | --------- | ----------------------------- |
| `/v1/chat/completions`     | POST      | Chat completion (core)        |
| `/v1/completions`          | POST      | Text completion (legacy)      |
| `/v1/responses`            | POST      | Responses API (next-gen chat) |
| `/v1/embeddings`           | POST      | Text embeddings               |
| `/v1/images/generations`   | POST      | Image generation              |
| `/v1/images/edits`         | POST      | Image editing                 |
| `/v1/audio/transcriptions` | POST      | Speech to text                |
| `/v1/audio/translations`   | POST      | Speech translation            |
| `/v1/audio/speech`         | POST      | Text to speech                |
| `/v1/rerank`               | POST      | Reranking                     |
| `/v1/moderations`          | POST      | Content moderation            |
| `/v1/realtime`             | WebSocket | Realtime multimodal chat      |
| `/v1/models`               | GET       | List models                   |

## Anthropic-style endpoints

| Endpoint       | Method | Description                              |
| -------------- | ------ | ---------------------------------------- |
| `/v1/messages` | POST   | Claude chat completion                   |
| `/v1/messages` | GET    | Fetch a message (streaming continuation) |

> When calling `/v1/messages`, the server auto-detects the OpenAI / Anthropic / Gemini protocol from the request headers — no extra configuration needed.

## Gemini-style endpoints

| Endpoint                                 | Method | Description               |
| ---------------------------------------- | ------ | ------------------------- |
| `/v1beta/models`                         | GET    | List Gemini models        |
| `/v1beta/models/{model}:generateContent` | POST   | Gemini content generation |

## Async task endpoints

Dedicated endpoints for image, video, and other async generation tasks:

| Endpoint                  | Method | Description              |
| ------------------------- | ------ | ------------------------ |
| `/v1/videos`              | POST   | Create a video task      |
| `/v1/videos/{id}/content` | GET    | Fetch video content      |
| `/v1/video/generations`   | POST   | Unified video generation |
| `/mj/submit/{action}`     | POST   | Submit a Midjourney task |
| `/mj/task/{id}/fetch`     | GET    | Query a Midjourney task  |
| `/suno/submit/{action}`   | POST   | Submit a Suno music task |
| `/suno/fetch/{id}`        | GET    | Query a Suno task        |
| `/kling/v1/videos/*`      | POST   | Kling native video API   |

## Playground

An in-browser playground for testing (requires login):

| Endpoint               | Method | Description                                 |
| ---------------------- | ------ | ------------------------------------------- |
| `/pg/chat/completions` | POST   | Playground chat (uses your logged-in quota) |

## Next steps

- For the full model list and prices, see [Models & Pricing](/docs/pricing).
- When a request fails, match it against [Error Codes](/docs/error-codes).
