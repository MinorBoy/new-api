# MiniMax H3 视频生成

MiniMax H3 是异步视频生成模型：提交任务后立即返回 `task_id`，轮询到终态获取视频地址。本站按请求参数（清晰度、时长、比例、参考素材）自动选择上游渠道，你始终只使用 `MiniMax-H3` 一个模型 ID。

## 基本信息

| 项 | 值 |
| --- | --- |
| 模型 ID | `MiniMax-H3`（大小写不敏感，`minimax-h3` 等价） |
| 提交 | `POST /v1/video/generations`（推荐）或 `POST /v1/videos` |
| 查询 | `GET /v1/videos/{task_id}`（或 `GET /v1/video/generations/{task_id}`） |
| 下载 | `GET /v1/videos/{task_id}/content` |
| 任务模式 | 异步；终态 `completed` / `failed` |
| 鉴权 | `Authorization: Bearer <API Key>` |

> ARK 原生入口 `/api/v3/contents/generations/**` 仅服务 Seedance 系列模型，请求 `MiniMax-H3` 会被拒绝。

## 请求体

请求体使用 **content 数组结构**（不是 `prompt` 平铺格式；仅传 `prompt` 会被拒绝）。

### 顶层字段

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `model` | string | 是 | 固定 `MiniMax-H3` |
| `content` | array | 是 | 内容数组，必须恰好包含 1 个 `text` 项 |
| `resolution` | string | 否 | `720p` / `768p` / `2k`；缺省按路由默认（当前 `2k`） |
| `duration` | integer | 否 | 4–15 秒；缺省按路由默认（当前 15） |
| `ratio` | string | 否 | `auto` / `1:1` / `16:9` / `9:16` / `21:9` / `3:4` / `4:3`；缺省 `16:9` |

### content 数组项

| type | role | 字段 | 说明 |
| --- | --- | --- | --- |
| `text` | 不填 | `text` | 提示词，非空 |
| `image_url` | `reference_image` 或缺省 | `image_url.url` | 参考图 |
| `image_url` | `first_frame` | `image_url.url` | 首帧图 |
| `image_url` | `last_frame` | `image_url.url` | 尾帧图（必须与首帧同现） |
| `video_url` | `reference_video`（必填） | `video_url.url` | 参考视频 |
| `audio_url` | `reference_audio`（必填） | `audio_url.url` | 参考音频 |

生成模式由内容自动推导，无需传任何模式字段：纯文本为文生；带参考图/视频/音频为多模态参考；带首尾帧角色为首尾帧模式。

### 能力约束

- 清晰度仅 `720p` / `768p` / `2k`
- 时长 4–15 秒
- 参考素材上限：图片 9、视频 3、音频 3，总计 ≤ 15
- 音频不能单独出现，必须伴随至少 1 张图片或 1 个视频
- 首尾帧不能与参考素材混用
- 媒体 URL 必须是公网可访问的 HTTP(S) 地址

## 示例请求

文生视频：

```curl
curl -X POST "https://<你的域名>/v1/video/generations" \
  -H "Authorization: Bearer sk-你的令牌" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "MiniMax-H3",
    "content": [
      { "type": "text", "text": "一只柯基在海边奔跑，电影感镜头，阳光洒在海面上" }
    ],
    "resolution": "2k",
    "duration": 5,
    "ratio": "16:9"
  }'
```

参考图 + 音频（多模态参考）：

```curl
curl -X POST "https://<你的域名>/v1/video/generations" \
  -H "Authorization: Bearer sk-你的令牌" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "MiniMax-H3",
    "content": [
      { "type": "text", "text": "保持参考图中人物的外观，按音频节奏推进镜头" },
      { "type": "image_url", "role": "reference_image", "image_url": { "url": "https://example.com/person.jpg" } },
      { "type": "audio_url", "role": "reference_audio", "audio_url": { "url": "https://example.com/rhythm.mp3" } }
    ],
    "resolution": "768p",
    "duration": 10,
    "ratio": "16:9"
  }'
```

首尾帧：

```curl
curl -X POST "https://<你的域名>/v1/video/generations" \
  -H "Authorization: Bearer sk-你的令牌" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "MiniMax-H3",
    "content": [
      { "type": "text", "text": "从首帧过渡到尾帧，镜头缓慢推进" },
      { "type": "image_url", "role": "first_frame", "image_url": { "url": "https://example.com/first.jpg" } },
      { "type": "image_url", "role": "last_frame", "image_url": { "url": "https://example.com/last.jpg" } }
    ],
    "duration": 8,
    "ratio": "16:9"
  }'
```

## 响应

提交成功（HTTP 200）：

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

查询（`GET /v1/videos/{task_id}`）：

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
  "metadata": { "url": "https://cdn.example.com/video.mp4?签名参数" },
  "error": null
}
```

- `status`：`queued` / `in_progress` / `completed` / `failed`
- 成功后视频地址在 `metadata.url`（带签名、有时效，建议尽快转存；也可用 `/v1/videos/{task_id}/content` 重新获取）
- 失败时 `error.message` 为原因

常见错误码：

| code | 含义 |
| --- | --- |
| `InvalidParameter.resolution` | 清晰度不在 720p/768p/2k |
| `InvalidParameter.duration` | 时长越界（4–15） |
| `InvalidParameter.ratio` | 比例不受支持 |
| `InvalidParameter.content` | 内容数组结构错误 |
| `InvalidParameter.model` | ARK 入口请求 H3，或模型不可用 |
| `no_compatible_route` | 参数组合没有可用渠道 |
| `task_failed` | 上游生成失败 |

## 完整轮询示例

```bash
BASE="https://<你的域名>"
KEY="sk-你的令牌"

RESP=$(curl -sS -X POST "$BASE/v1/video/generations" \
  -H "Authorization: Bearer $KEY" \
  -H "Content-Type: application/json" \
  -d '{"model":"MiniMax-H3","content":[{"type":"text","text":"一只柯基在海边奔跑"}],"resolution":"2k","duration":5,"ratio":"16:9"}')

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

## 计费

按次计费：一次成功生成记一次费用，与时长无关。生成失败不扣费，预扣额度自动全额返还。不同清晰度对应不同售价档位，以模型广场为准。

## 常见问题

**为什么用 `prompt` 提交返回 400？** H3 使用 content 数组结构，把提示词放进 `content` 的 `text` 项即可。

**能否指定渠道？** 不能也不需要；渠道由本站按请求参数自动路由。

**多久出片？** 通常 1–6 分钟，轮询直到终态。
