# Z5API Seedance 渠道接入设计

**日期：** 2026-08-04
**状态：** 已确认方案，待实施

## 目标

新增 Z5API Seedance 视频渠道，使用户继续通过 Ark SDK 的 `/api/v3/contents/generations/tasks/*` 完成提交、查询和任务结果获取，无需修改客户端代码。Z5API 上游协议使用 `https://z5api.com/v1/videos`，渠道默认保持禁用，真实上游验收通过后才允许启用。

## 协议事实

来源：`docs/new-channels/cn-z5api.html`。HTML 是协议来源，模型目录以 `sd收录.xlsx`、渠道模板或配置导入快照为准。

| 项目 | Z5API 约定 | 代码处理 |
|---|---|---|
| 默认地址 | `https://z5api.com` | 新增渠道常量和默认 Base URL，允许管理员覆盖 |
| 认证 | `Authorization: Bearer <API Key>` | 使用现有 task adaptor 认证，不记录密钥 |
| 创建 | `POST /v1/videos` | Ark 请求转换为 Z5API JSON |
| 查询 | `GET /v1/videos/{task_id}` | 由现有任务轮询调用 |
| 请求主体 | `model`、`prompt`、`media[]`、`parameters{}` | 顶层 Ark `content` 转换为上游字段 |
| 参数 | `resolution`、`ratio`、`duration` | 放入 `parameters`；兼容字段 `size`/`seconds` 使用公共 Ark 解析 |
| 媒体 | `first_frame`、`last_frame`、`reference_image`、`reference_video`、`reference_voice` | 保留角色语义，公网 HTTP(S) URL，拒绝本地/私网地址 |
| 创建状态 | `pending`、`processing`、`completed`、`failed` | 映射到内部排队、运行、成功、失败 |
| 成功结果 | 查询响应 `object` 为视频 URL | 投影为 Ark `content.video_url` |
| 时长 | 查询响应 `seconds` 字符串 | 解析并限制在 `MaxTaskDurationSeconds` 内，参与上游实际计费 |
| 结果保留 | 约 24 小时 | 不把上游私有任务数据暴露给用户 |

文档声明模型为 `sd-2-fast`、`sd-2-c1` 至 `sd-2-c6`，但实现不硬编码这组名称作为最终模型目录。模型映射必须由渠道模板/导入配置提供；上游请求只使用映射后的模型 ID。

## 能力与边界矩阵

| 能力 | 文档结论 | 首次接入验收 |
|---|---|---|
| 文生视频 | 支持 | Ark 提交与成功轮询 |
| 首帧/首尾帧 | 支持 | `first_frame`、`last_frame` 顺序与角色测试 |
| 参考图片 | 支持，通常最多 9 张 | 9 张通过，10 张本地 400 |
| 参考视频 | 支持，通常最多 3 个 | 3 个通过，4 个本地 400 |
| 参考音频 | `reference_voice`，通常最多 3 个 | 3 个通过，4 个本地 400 |
| 多模态混合 | 支持 | 图片、视频、音频同一请求的编码测试 |
| 分辨率 | 文档推荐 720p | 使用导入模型能力校验，不猜模型 ID |
| 宽高比 | `1:1`、`16:9`、`9:16`、`4:3`、`3:4` | 非法比例本地拒绝 |
| 时长 | 文档默认 10 秒，具体范围未给出 | 使用公共最大值；真实范围待上游验收确认 |
| 流式 | 未声明 | 不加入 `streamSupportedChannels` |
| 删除任务 | 文档未声明 | 不新增删除上游调用；Ark 删除能力按现有内部语义处理 |

未在文档明确的上游行为标记为待实测，不通过代码推断补齐。

## 架构设计

在 `relay/channel/task/newapivideo` 中新增 Z5API profile 和专用请求编码/校验函数，复用 `TaskAdaptor`、Ark 请求解析、任务轮询、响应投影、计费和安全工具：

1. `ValidateRequestAndSetAction` 使用 Z5API 方言校验媒体角色、URL、数量、比例、时长和不支持字段。
2. `BuildRequestBody` 将 Ark `content` 转成：

   ```json
   {
     "model": "<mapped model>",
     "prompt": "<text>",
     "media": [{"type": "first_frame", "url": "https://..."}],
     "parameters": {"resolution": "720p", "ratio": "16:9", "duration": 10}
   }
   ```

   缺省可选字段省略；`duration` 等上游可选标量使用指针，显式 `0` 不得因 `omitempty` 静默丢失，而应在校验阶段明确接受或拒绝。Z5API 文档未声明 `watermark`、`seed`、`generate_audio` 等 Ark 字段，显式传入时返回参数错误。媒体数组只发送 URL 和协议声明的 type。
3. 查询响应解析复用通用任务投影，增加 `object` 视频 URL 和 `seconds` 时长字段的兼容分支；失败消息经过敏感信息清理。
4. adaptor 实现 `channel.ArkVideoTaskConverter`，加入任务平台白名单和注册表，使单查、列表和生命周期响应保持 Ark 结构并隔离上游 ID、Key、渠道信息。
5. 计费沿用现有 Seedance token/时长计费链路。所有上游时长和 token 用量经过已有边界与 `Quota*Checked` 饱和处理，不新增本地比例或裸 `int` 转换。

## 注册与管理端

- 分配下一个保留的渠道类型 ID，`ChannelTypeDummy` 顺延；同步常量测试、任务平台列表、配置导入绑定和成本计量测试。
- 管理端新增 Z5API 名称、默认 Base URL 和 task-only 标记；不加入通用聊天模型拉取。
- 模型目录由渠道模板/配置导入维护，不把 HTML 示例模型直接写入生产常量。
- 新渠道默认 disabled；未完成真实验收时，管理端提示保持“仅在验收通过后启用”。

## 测试设计

先写失败测试，再实现：

- profile、默认地址、认证头和 task adaptor 注册；
- 文生、首帧、首尾帧和图片/视频/音频混合请求的精确 JSON；
- 缺省字段省略、显式 `duration: 0` 不被静默省略，以及 `watermark: false` 等未支持字段明确返回参数错误；
- 媒体角色、数量、HTTP(S)/私网 URL、比例、时长和不支持字段的 400 错误码；
- `pending -> processing -> completed/failed` 状态、`object` URL、`seconds` 时长和错误投影；
- Ark 单查、列表和用户模型/公开任务 ID 隔离；
- 预扣、成功结算、失败退款和上游实际时长计费；
- 管理端配置、导入绑定、默认禁用和多语言文案。

使用 `httptest` 验证上游请求，不在测试快照中写真实 Key。无凭据时不宣称真实上游验收通过；真实验收需单独记录请求模型、状态、视频 URL 可读性、时长、计费和失败退款。

## 风险与停止条件

- 文档没有明确 Z5API 的精确时长范围、删除接口和错误 HTTP 状态，计划中以公共上限和待实测项处理；如果真实响应与假设不符，先更新设计再改代码。
- 文档模型 ID 仅作示例，禁止绕过渠道模板/导入表硬编码模型目录。
- 不复制 Paipu 的请求 profile；Z5API 的 `parameters` 嵌套、frame/voice 角色和 `object/seconds` 响应必须保持独立。
- 没有 API Key 时可以完成 mock、契约和本地测试，但必须把真实验收列为阻塞项，不能启用渠道。
