# MiniMax H3 视频生成 API 接入文档

本文档面向下游用户，描述如何调用本站的 MiniMax H3 视频生成接口。

## 概述

- **模型 ID**：`MiniMax-H3`（大小写不敏感，`minimax-h3` 等价；两者会被规范化为同一身份）
- **提交端点**：`POST /v1/video/generations`（推荐）或 `POST /v1/videos`（兼容别名）
- **查询端点**：`GET /v1/videos/{task_id}` 或 `GET /v1/video/generations/{task_id}`
- **内容下载**：`GET /v1/videos/{task_id}/content`
- **任务模式**：异步。提交后返回 `task_id`，客户端轮询到终态（`completed` / `failed`）
- **鉴权**：`Authorization: Bearer <API Key>`（标准 token 鉴权）

本站根据请求参数（清晰度、时长、比例、参考素材数量）自动选择最合适的上游渠道。下游**始终只使用 `MiniMax-H3` 这一个模型 ID**，不需要（也不能）指定具体渠道或上游模型。

## 请求地址

```text
POST /v1/video/generations
POST /v1/videos                      # 兼容别名，行为一致

GET  /v1/videos/{task_id}
GET  /v1/video/generations/{task_id} # 查询别名

GET  /v1/videos/{task_id}/content    # 下载视频内容
```

> 注意：ARK 原生入口 `/api/v3/contents/generations/**` 仅服务 Seedance 系列模型，请求 `MiniMax-H3` 会被拒绝（`400 InvalidParameter.model`）。

## 认证

```http
Authorization: Bearer sk-你的令牌
Content-Type: application/json
```

请先确认令牌所属分组已开通 `MiniMax-H3`；可通过 `GET /v1/models` 确认模型在列。

## 请求体

请求体使用 **content 数组结构**（与 OpenAI 的 `prompt` 平铺格式不同；仅传 `prompt` 字段会被拒绝）。

### 顶层字段

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `model` | string | 是 | 固定填 `MiniMax-H3` |
| `content` | array | 是 | 内容数组，至少 1 项，必须恰好包含 1 个 `text` 项 |
| `resolution` | string | 否 | 清晰度：`720p` / `768p` / `2k`；缺省按路由策略默认（当前 `2k`） |
| `duration` | integer | 否 | 时长秒数：4–15；缺省按路由策略默认（当前 15） |
| `ratio` | string | 否 | 画幅：`auto` / `1:1` / `16:9` / `9:16` / `21:9` / `3:4` / `4:3`；缺省 `16:9` |

### content 数组项

| type | 角色（`role`） | 字段 | 说明 |
| --- | --- | --- | --- |
| `text` | —（不填） | `text` | 提示词，非空 |
| `image_url` | `reference_image` 或缺省 | `image_url.url` | 参考图，公网 HTTP(S) URL |
| `image_url` | `first_frame` | `image_url.url` | 首帧图 |
| `image_url` | `last_frame` | `image_url.url` | 尾帧图（必须与首帧同时出现，且位于首帧之后） |
| `video_url` | `reference_video`（必填） | `video_url.url` | 参考视频，公网 HTTP(S) URL |
| `audio_url` | `reference_audio`（必填） | `audio_url.url` | 参考音频，公网 HTTP(S) URL |

### 生成模式自动推导

不需要传任何模式字段，模式由内容角色推导：

| 内容组合 | 推导模式 |
| --- | --- |
| 仅文本 | 文生视频 |
| 文本 + 参考图/视频/音频 | 多模态参考 |
| 文本 + 首帧（或首尾帧） | 首尾帧 |

### 能力约束

- 清晰度仅支持 `720p` / `768p` / `2k`，其他值返回 `400`
- 时长 4–15 秒，越界返回 `400`
- 比例仅支持上表枚举，`adaptive` 等其他值返回 `400`
- 参考素材上限：图片 9、视频 3、音频 3，总计不超过 15
- 音频不能单独出现，必须伴随至少 1 张图片或 1 个视频
- 首尾帧不能与参考素材混用
- 媒体 URL 必须是公网可访问的 HTTP(S) 地址（不支持 data: 内嵌、本地路径）

### 示例请求

文生视频：

```json
{
  "model": "MiniMax-H3",
  "content": [
    { "type": "text", "text": "一只柯基在海边奔跑，电影感镜头，阳光洒在海面上" }
  ],
  "resolution": "2k",
  "duration": 5,
  "ratio": "16:9"
}
```

参考图 + 音频（多模态参考）：

```json
{
  "model": "MiniMax-H3",
  "content": [
    { "type": "text", "text": "保持参考图中人物的外观，按音频节奏推进镜头" },
    { "type": "image_url", "role": "reference_image", "image_url": { "url": "https://example.com/person.jpg" } },
    { "type": "audio_url", "role": "reference_audio", "audio_url": { "url": "https://example.com/rhythm.mp3" } }
  ],
  "resolution": "768p",
  "duration": 10,
  "ratio": "16:9"
}
```

首尾帧：

```json
{
  "model": "MiniMax-H3",
  "content": [
    { "type": "text", "text": "从首帧过渡到尾帧，镜头缓慢推进" },
    { "type": "image_url", "role": "first_frame", "image_url": { "url": "https://example.com/first.jpg" } },
    { "type": "image_url", "role": "last_frame", "image_url": { "url": "https://example.com/last.jpg" } }
  ],
  "duration": 8,
  "ratio": "16:9"
}
```

## 响应体

### 提交任务响应（HTTP 200）

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

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `id` / `task_id` | string | 任务 ID（两字段同值，`task_id` 为规范字段） |
| `object` | string | 固定 `video` |
| `model` | string | 请求时的模型 ID |
| `status` | string | 初始 `queued` |
| `progress` | int | 进度百分比 |
| `created_at` | int64 | 创建时间（Unix 秒） |

保存 `task_id` 用于轮询。

### 查询任务响应

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
  "metadata": {
    "url": "https://cdn.example.com/video.mp4?签名参数"
  }
}
```

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `status` | string | `queued` / `in_progress` / `completed` / `failed` |
| `progress` | int | 进度百分比（估算值） |
| `completed_at` | int64 | 完成时间（终态时返回） |
| `metadata.url` | string | 成功时的视频下载 URL（带签名，有时效，建议尽快转存） |
| `error` | object | 失败时返回，`error.message` 为原因，`error.code` 为错误码 |

### 错误响应

提交阶段参数错误（HTTP 400）：

```json
{
  "error": {
    "code": "InvalidParameter.resolution",
    "message": "resolution is invalid"
  }
}
```

任务失败（查询返回，HTTP 200 + `status: failed`）：

```json
{
  "status": "failed",
  "error": {
    "message": "video result storage failed",
    "code": "task_failed"
  }
}
```

常见错误码：

| code | 含义 |
| --- | --- |
| `InvalidParameter.resolution` | 清晰度不在 720p/768p/2k |
| `InvalidParameter.duration` | 时长越界（4–15） |
| `InvalidParameter.ratio` | 比例不受支持 |
| `InvalidParameter.content` | 内容数组结构错误（缺 text、角色错、素材超限等） |
| `InvalidParameter.model` | ARK 原生入口请求了 H3，或模型不可用 |
| `no_compatible_route` | 请求参数没有匹配的可用渠道（检查清晰度/时长/比例组合） |
| `compatible_channel_unavailable` | 有匹配渠道但当前不可用，稍后重试 |
| `task_failed` | 上游生成失败（见 `error.message`） |

## 完整调用示例

```bash
#!/bin/bash
BASE="https://你的站点地址"
KEY="sk-你的令牌"

# 1. 提交
RESP=$(curl -sS -X POST "$BASE/v1/video/generations" \
  -H "Authorization: Bearer $KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "MiniMax-H3",
    "content": [{"type": "text", "text": "一只柯基在海边奔跑，电影感镜头"}],
    "resolution": "2k",
    "duration": 5,
    "ratio": "16:9"
  }')

TASK_ID=$(echo "$RESP" | jq -r '.task_id')
echo "task_id=$TASK_ID"

# 2. 轮询（建议 5 秒间隔，总等待 ≥10 分钟）
for i in $(seq 1 120); do
  RESULT=$(curl -sS "$BASE/v1/videos/$TASK_ID" -H "Authorization: Bearer $KEY")
  STATUS=$(echo "$RESULT" | jq -r '.status')
  case "$STATUS" in
    completed)
      echo "$RESULT" | jq -r '.metadata.url'
      exit 0 ;;
    failed)
      echo "$RESULT" | jq '.error'
      exit 1 ;;
    *) sleep 5 ;;
  esac
done
echo "轮询超时，请保留 task_id 稍后继续查询" >&2
exit 2
```

## 计费说明

- 按次计费（per-request）：一次成功生成的视频记一次费用，与时长无关
- 生成失败（`status: failed`）不扣费，预扣额度自动全额返还
- 价格以模型广场展示为准；不同清晰度对应不同售价档位

## 常见问题

**Q: 为什么我用 `prompt` 字段提交返回 400？**
A: H3 使用 content 数组结构。把提示词放进 `content` 数组的 `text` 项即可，参见示例。

**Q: 能否指定走某个渠道（如 paipu / secure）？**
A: 不能也不需要。渠道选择由本站按请求参数自动路由；下游只使用 `MiniMax-H3`。

**Q: 提交后多久出片？**
A: 通常 1–6 分钟，取决于渠道负载与时长；轮询直到终态即可。

**Q: 视频地址能保存多久？**
A: 结果 URL 带签名有时效（通常 24 小时），建议完成后尽快下载或转存；也可随时用 `GET /v1/videos/{task_id}/content` 重新获取内容。
