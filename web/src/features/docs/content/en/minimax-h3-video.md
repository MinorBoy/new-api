# MiniMax H3 Video Generation

MiniMax H3 is an asynchronous video generation model: submit a task, receive a `task_id` immediately, and poll until a terminal state to get the video URL. The gateway picks the upstream channel automatically from your request parameters (resolution, duration, ratio, reference media) — you always use the single model ID `MiniMax-H3`.

## Basics

| Item | Value |
| --- | --- |
| Model ID | `MiniMax-H3` (case-insensitive; `minimax-h3` is equivalent) |
| Submit | `POST /v1/video/generations` (recommended) or `POST /v1/videos` |
| Poll | `GET /v1/videos/{task_id}` (or `GET /v1/video/generations/{task_id}`) |
| Download | `GET /v1/videos/{task_id}/content` |
| Task mode | Async; terminal states `completed` / `failed` |
| Auth | `Authorization: Bearer <API Key>` |

> The ARK native entry `/api/v3/contents/generations/**` only serves Seedance models; requesting `MiniMax-H3` there is rejected.

## Request body

Requests use the **content array structure** (not the flat `prompt` format; submitting only `prompt` is rejected).

### Top-level fields

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `model` | string | yes | Always `MiniMax-H3` |
| `content` | array | yes | Content items; exactly one `text` item is required |
| `resolution` | string | no | `720p` / `768p` / `2k`; defaults to the routing default (currently `2k`) |
| `duration` | integer | no | 4–15 seconds; defaults to the routing default (currently 15) |
| `ratio` | string | no | `auto` / `1:1` / `16:9` / `9:16` / `21:9` / `3:4` / `4:3`; defaults to `16:9` |

### Content items

| type | role | field | Description |
| --- | --- | --- | --- |
| `text` | none | `text` | Prompt, non-empty |
| `image_url` | `reference_image` or omitted | `image_url.url` | Reference image |
| `image_url` | `first_frame` | `image_url.url` | First frame |
| `image_url` | `last_frame` | `image_url.url` | Last frame (must appear with a first frame) |
| `video_url` | `reference_video` (required) | `video_url.url` | Reference video |
| `audio_url` | `reference_audio` (required) | `audio_url.url` | Reference audio |

The generation mode is derived from the content automatically — no mode field needed: text only means text-to-video; reference images/videos/audios mean multimodal reference; first/last frame roles mean frame interpolation.

### Constraints

- Resolution: `720p` / `768p` / `2k` only
- Duration: 4–15 seconds
- Reference media limits: 9 images, 3 videos, 3 audios, 15 total
- Audio cannot appear alone; it requires at least one image or video
- First/last frames cannot be mixed with reference media
- Media URLs must be publicly accessible HTTP(S) addresses

## Examples

Text to video:

```curl
curl -X POST "https://<your-domain>/v1/video/generations" \
  -H "Authorization: Bearer sk-your-key" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "MiniMax-H3",
    "content": [
      { "type": "text", "text": "A corgi running along the beach, cinematic shot, sunlight on the water" }
    ],
    "resolution": "2k",
    "duration": 5,
    "ratio": "16:9"
  }'
```

Reference image + audio (multimodal reference):

```curl
curl -X POST "https://<your-domain>/v1/video/generations" \
  -H "Authorization: Bearer sk-your-key" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "MiniMax-H3",
    "content": [
      { "type": "text", "text": "Keep the person from the reference image; advance the shot to the audio rhythm" },
      { "type": "image_url", "role": "reference_image", "image_url": { "url": "https://example.com/person.jpg" } },
      { "type": "audio_url", "role": "reference_audio", "audio_url": { "url": "https://example.com/rhythm.mp3" } }
    ],
    "resolution": "768p",
    "duration": 10,
    "ratio": "16:9"
  }'
```

First and last frames:

```curl
curl -X POST "https://<your-domain>/v1/video/generations" \
  -H "Authorization: Bearer sk-your-key" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "MiniMax-H3",
    "content": [
      { "type": "text", "text": "Transition from the first frame to the last, slow push-in" },
      { "type": "image_url", "role": "first_frame", "image_url": { "url": "https://example.com/first.jpg" } },
      { "type": "image_url", "role": "last_frame", "image_url": { "url": "https://example.com/last.jpg" } }
    ],
    "duration": 8,
    "ratio": "16:9"
  }'
```

## Responses

Submit success (HTTP 200):

```json
{
  "id": "task_xxx",
  "task_id": "task_xxx",
  "object": "video",
  "model": "MiniMax-H3",
  "status": "queued",
  "progress": 1,
  "created_at": 1790700660
}
```

Poll (`GET /v1/videos/{task_id}`):

```json
{
  "id": "task_xxx",
  "task_id": "task_xxx",
  "object": "video",
  "model": "MiniMax-H3",
  "status": "completed",
  "progress": 100,
  "created_at": 1790700660,
  "completed_at": 1790700849,
  "metadata": { "url": "https://cdn.example.com/video.mp4?signature" },
  "error": null
}
```

- `status`: `queued` / `in_progress` / `completed` / `failed`
- On success the video URL is in `metadata.url` (signed and time-limited; download promptly, or fetch again via `/v1/videos/{task_id}/content`)
- On failure `error.message` carries the reason

Common error codes:

| code | Meaning |
| --- | --- |
| `InvalidParameter.resolution` | Resolution outside 720p/768p/2k |
| `InvalidParameter.duration` | Duration out of range (4–15) |
| `InvalidParameter.ratio` | Unsupported ratio |
| `InvalidParameter.content` | Malformed content array |
| `InvalidParameter.model` | H3 requested on the ARK entry, or model unavailable |
| `no_compatible_route` | No channel matches the parameter combination |
| `task_failed` | Upstream generation failed |

## Full polling example

```bash
BASE="https://<your-domain>"
KEY="sk-your-key"

RESP=$(curl -sS -X POST "$BASE/v1/video/generations" \
  -H "Authorization: Bearer $KEY" \
  -H "Content-Type: application/json" \
  -d '{"model":"MiniMax-H3","content":[{"type":"text","text":"A corgi running along the beach"}],"resolution":"2k","duration":5,"ratio":"16:9"}')

TASK_ID=$(echo "$RESP" | jq -r '.task_id')

for i in $(seq 1 120); do
  RESULT=$(curl -sS "$BASE/v1/videos/$TASK_ID" -H "Authorization: Bearer $KEY")
  STATUS=$(echo "$RESULT" | jq -r '.status')
  case "$STATUS" in
    completed) echo "$RESULT" | jq -r '.metadata.url'; exit 0 ;;
    failed)    echo "$RESULT" | jq '.error'; exit 1 ;;
    *)         sleep 5 ;;
  esac
done
```

## Billing

Per-request billing: one successful generation is one charge, independent of duration. Failed generations are not billed; pre-deducted quota is fully refunded. Different resolutions map to different price tiers; see the model plaza.

## Differences from the MiniMax official v2 API

This station's request body deliberately mirrors the MiniMax official [video generation v2 API](https://platform.minimax.cn/docs/api-reference/video-generation-v2-create) (`POST /v2/video_generation`): the `content` array structure, role semantics, mode composition rules, and media limits are identical. If you already use the official v2 API, swap the base URL and credentials to migrate. Watch these differences:

| Item | MiniMax official v2 | This station |
| --- | --- | --- |
| Submit endpoint | `POST /v2/video_generation` | `POST /v1/video/generations` (or `POST /v1/videos`) |
| Submit response | `{"task_id": "..."}` | OpenAI video object (`id` / `task_id` / `object` / `status` / `progress`) |
| Poll endpoint | `GET /v2/video_generation/{task_id}` | `GET /v1/videos/{task_id}` |
| Poll response | `{"task": {...}}` with `queued` / `running` / `succeeded` / `failed` / `cancelled` | OpenAI video object with `queued` / `in_progress` / `completed` / `failed`; video URL in `metadata.url` |
| `ratio` | `adaptive` supported (and default); required for text-to-video, `adaptive` forbidden there | `adaptive` unsupported — use `auto`; not strictly required for text-to-video |
| `resolution` | required, `768P` / `2K` only | optional (routing default `2k`), supports `720p` / `768p` / `2k`; official uppercase spellings (`2K`/`768P`) are accepted |
| `duration` | required | optional (defaults to 15) |
| Models | `MiniMax-H3` / `MiniMax-H3-Max` | `MiniMax-H3` only |
| Single image without role | treated as first frame (image-to-video) | treated as reference image (multimodal reference); use explicit `role: first_frame` for frame input |
| `callback_url` webhook | supported (challenge verification, then status pushes) | unsupported — use polling |
| `aigc_watermark` | supported | unsupported |

Migration notes: don't send `adaptive` for `ratio` (use `auto`); parse responses for `task_id` and `metadata.url`; map `running`/`succeeded` to `in_progress`/`completed`. All other request fields carry over unchanged.

## FAQ

**Why is my `prompt`-only request rejected with 400?** H3 uses the content array structure; put the prompt in the `text` item of `content`.

**Can I pick a specific channel?** No, and you don't need to — routing is automatic based on request parameters.

**How long until the video is ready?** Usually 1–6 minutes; poll until a terminal state.
