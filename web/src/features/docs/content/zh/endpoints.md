# 接口地址

new-api 同时兼容 **OpenAI 风格**与 **Anthropic 风格**两类接口。Base URL 统一为 `https://<你的域名>`，下方所有端点均相对此域名。

## Base URL 与鉴权

所有接口在请求头中携带 API Key 进行鉴权：

```http
Authorization: Bearer <你的_API_KEY>
```

> Anthropic 风格接口同样接受 `Authorization: Bearer ...`；若使用官方 Claude SDK，设置 `base_url` 为 `https://<你的域名>` 即可。

## OpenAI 风格端点

| 端点                       | 方法      | 说明                        |
| -------------------------- | --------- | --------------------------- |
| `/v1/chat/completions`     | POST      | 对话补全（核心）            |
| `/v1/completions`          | POST      | 文本补全（旧版）            |
| `/v1/responses`            | POST      | Responses API（新一代对话） |
| `/v1/embeddings`           | POST      | 文本向量化                  |
| `/v1/images/generations`   | POST      | 图像生成                    |
| `/v1/images/edits`         | POST      | 图像编辑                    |
| `/v1/audio/transcriptions` | POST      | 语音转文字                  |
| `/v1/audio/translations`   | POST      | 语音翻译                    |
| `/v1/audio/speech`         | POST      | 文字转语音                  |
| `/v1/rerank`               | POST      | 重排序                      |
| `/v1/moderations`          | POST      | 内容审核                    |
| `/v1/realtime`             | WebSocket | 实时多模态对话              |
| `/v1/models`               | GET       | 模型列表                    |

## Anthropic 风格端点

| 端点           | 方法 | 说明                 |
| -------------- | ---- | -------------------- |
| `/v1/messages` | POST | Claude 对话补全      |
| `/v1/messages` | GET  | 获取消息（流式续传） |

> 在请求 `/v1/messages` 时，服务会根据请求头自动识别 OpenAI / Anthropic / Gemini 协议格式，无需额外配置。

## Gemini 风格端点

| 端点                                     | 方法 | 说明            |
| ---------------------------------------- | ---- | --------------- |
| `/v1beta/models`                         | GET  | Gemini 模型列表 |
| `/v1beta/models/{model}:generateContent` | POST | Gemini 内容生成 |

## 异步任务端点

图像、视频等异步生成任务的专用端点：

| 端点                      | 方法 | 说明                |
| ------------------------- | ---- | ------------------- |
| `/v1/videos`              | POST | 创建视频任务        |
| `/v1/videos/{id}/content` | GET  | 查询视频内容        |
| `/v1/video/generations`   | POST | 视频生成（统一）    |
| `/mj/submit/{action}`     | POST | Midjourney 任务提交 |
| `/mj/task/{id}/fetch`     | GET  | Midjourney 任务查询 |
| `/suno/submit/{action}`   | POST | Suno 音乐任务提交   |
| `/suno/fetch/{id}`        | GET  | Suno 任务查询       |
| `/kling/v1/videos/*`      | POST | Kling 视频原生接口  |

## Playground

内置 Playground 用于在浏览器中测试（需登录）：

| 端点                   | 方法 | 说明                              |
| ---------------------- | ---- | --------------------------------- |
| `/pg/chat/completions` | POST | Playground 对话（使用登录态额度） |

## 下一步

- 完整模型清单与价格见 [模型与计费](/docs/pricing)。
- 请求出错时对照 [错误码参考](/docs/error-codes) 排查。
