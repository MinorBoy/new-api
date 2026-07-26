# Lucen Seedance 渠道接入设计

**日期：** 2026-07-26  
**状态：** 已确认，进入接入计划阶段

## 1. 目标

为 Lucen 的 Seedance 视频模型增加独立渠道类型，使用户可以继续使用标准 Ark SDK 调用：

- `POST /api/v3/contents/generations/tasks`
- `GET /api/v3/contents/generations/tasks/{id}`
- `GET /api/v3/contents/generations/tasks`

用户不需要修改调用代码。Lucen 的图片、参考视频、参考音频多模态生成、固定秒计费和动态 Token 计费都必须可用。

删除任务接口不属于本次硬性要求，不实现，也不模拟 Lucen 上游取消。

## 2. 已确认的 Lucen 模型与 Key 配置

Lucen 上游要求两枚 API Key，分别对应两类计费模型。管理员创建两个普通 Lucen 渠道：

| 渠道用途 | API Key | 上游模型 |
| --- | --- | --- |
| 固定秒 | 固定秒 Key | `seedance-{480p,720p,1080p}-{5s,10s,15s}`，共 9 个 |
| 动态 Token | 动态 Token Key | `seedance-{480p,720p,1080p}-token`，共 3 个 |

完整模型 ID 如下：

```text
seedance-480p-5s
seedance-480p-10s
seedance-480p-15s
seedance-720p-5s
seedance-720p-10s
seedance-720p-15s
seedance-1080p-5s
seedance-1080p-10s
seedance-1080p-15s
seedance-480p-token
seedance-720p-token
seedance-1080p-token
```

不增加 `lucen_video_group` 或通用多 Key 配置。管理端使用普通模型选择器，管理员自行将 9 个固定秒模型绑定到固定秒渠道、将 3 个 Token 模型绑定到动态 Token 渠道。默认 Base URL 为 `https://lucen.asia`，同时保留可配置 Base URL。

## 3. 架构

采用“共享 `NewAPIVideo` 核心 + Lucen 协议配置”：

1. 新增独立 Lucen 渠道类型、品牌入口、默认地址和模型目录。
2. 复用 `relay/channel/task/newapivideo/` 的提交、轮询、私有/公开任务 ID、状态映射、Ark 响应转换和 usage 解析。
3. 通过 Lucen 协议配置选择请求字段和端点，不复制通用提交/轮询逻辑。
4. 路由策略仍是唯一的模型候选来源。策略目标绑定 `channel_id + upstream_model`，负责分辨率、时长、比例、多模态条件和成本/利润筛选；Lucen 渠道不新增一套参数范围或能力声明。
5. 用户请求中的标准 Ark 模型名与 Lucen 上游模型 ID 分离。上游模型 ID 只用于内部路由和上游请求，Ark 响应继续返回标准模型名。

### 请求数据流

```text
Ark SDK
  -> /api/v3/contents/generations/tasks
  -> 统一 Seedance 请求解析
  -> 现有模型路由策略生成候选目标
  -> 成本/利润路由在固定秒与 Token 渠道中选择
  -> Lucen NewAPIVideo 协议配置转换请求
  -> POST /v1/video/generations
  -> 本地任务保存上游 task_id
  -> GET /v1/video/generations/{task_id} 轮询
  -> Ark 标准任务响应
```

## 4. Ark 兼容与参数语义

### 4.1 支持的接口

必须支持创建、单任务查询和任务列表三个 Ark 接口。任务列表读取本地任务记录，保持现有 Ark 字段格式；删除任务不实现。

### 4.2 请求字段

- `model` 和 `content` 为必需字段。
- 参考图片、参考视频、参考音频均支持，保留 `first_frame`、`last_frame`、`reference_image`、`reference_video`、`reference_audio` 等角色。
- 媒体 URL 支持 HTTP(S)、`data:` Base64/Data URI 和 `asset://`，必须保留原始字符串并转发给 Lucen。
- Ark `duration: N` 转为 Lucen 要求的字符串字段 `"seconds": "N"`。
- Ark `generate_audio` 转为 Lucen 的驼峰字段 `generateAudio`。
- 所有可选标量使用指针或等价的存在性表示，显式 `false`、`0` 等值必须保留。
- 非必填且 Lucen 无法处理的 Ark 字段静默忽略，不返回参数错误，也不发送给上游。该规则适用于 `callback_url`、`return_last_frame`、`priority`、`execution_expires_after` 等可选控制字段。
- 影响生成能力或计费的字段不能静默丢弃；候选过滤或协议转换失败必须沿用现有任务错误链路。

### 4.3 比例和默认值

Lucen 不支持 `adaptive`，接入层不新增 `adaptive` 默认行为，也不在 Lucen 适配器中强行设置 `16:9`。请求事实和默认值由现有模型路由策略产生；路由策略不应把不支持的比例配置为 Lucen 目标。Lucen 适配器只转换已选中的请求，不自行扩展渠道级参数约束。

省略的分辨率和时长也由现有路由策略的默认值处理。对 Lucen 的固定秒模型，路由策略必须使 `5s`、`10s`、`15s` 目标与最终时长匹配；动态 Token 模型可参与其支持的时长范围。适配器不再复制一套固定秒筛选逻辑。

## 5. 多模态与路由解析

统一 Seedance 请求解析需要保留媒体原始地址，不能因 URL 不是 HTTP(S) 而在进入 Lucen 前丢失 `data:` 或 `asset://`。路由层继续提取媒体数量、角色等事实；只有可被现有媒体元数据服务读取的 HTTP(S) 参考视频才参与该服务的时长解析。Lucen 协议适配器将所有已接受的媒体地址原样转发。

该调整不改变其他渠道的上游协议语义：不支持某种地址格式的其他适配器仍按自身协议返回明确错误，不能静默删除媒体。

## 6. 路由与计费

- 路由策略目标绑定具体 Lucen 渠道和唯一上游模型 ID；渠道不声明参数取值范围。
- 固定秒模型只由策略目标在请求时长匹配时进入候选；动态 Token 模型和固定秒模型有重叠时，均交给现有成本/利润路由。
- 成本/利润路由根据管理员配置的上游成本、用户售价、最低利润阈值、可用性和重试状态选择渠道，不固定偏向固定秒或 Token 分组。
- 固定秒模型复用现有视频时长/分辨率计费规则。
- Token 模型在 Lucen 详细任务响应的 `usage.total_tokens` 或 `completion_tokens` 可用时，使用实际 usage 完成结算；创建阶段沿用现有预扣逻辑。
- 轮询终态缺少必要 usage、实际时长或视频地址时，进入现有计量未知、重试或失败关闭路径，不生成不可信账单。
- 非必填字段的静默忽略不参与路由、成本估算或计费。
- 预扣、结算、退款和 quota 饱和审计继续使用现有 billing expression、quota math 和任务结算安全机制。

## 7. 状态、错误和响应

- Lucen 创建和查询的 HTTP、JSON、业务错误转换为现有任务错误格式；上游原始错误信息仅保留在管理员可见日志中。
- `queued`、`in_progress` 映射为处理中。
- 只有 `succeeded`、进度完成且视频地址非空时才映射为成功。
- `failed` 映射为失败，并保留 Lucen 错误码和消息供诊断。
- 不以非零 `completed_at` 单独判断完成。
- 本地任务保存公开 Ark 任务 ID与私有 Lucen 上游 task ID，避免将上游 ID 直接暴露为用户模型信息。

## 8. 管理端行为

新增 Lucen 渠道类型，并提供：

- 默认 Base URL `https://lucen.asia`；
- API Key 输入；
- 12 个 Lucen 上游模型的普通选择项；
- 与现有渠道一致的启用、优先级、分组和计费配置。

不增加 Secure 风格的分组下拉框，不增加 Lucen 专用多 Key 或 `lucen_video_group` 字段。模型路由策略在独立配置界面维护标准模型到渠道/上游模型的目标关系。

## 9. 测试与验收

### 单元和适配器测试

- Lucen 创建请求字段和 Base URL/Authorization；
- Ark `duration` 到字符串 `seconds`；
- `generate_audio` 驼峰转换；
- 显式 `false`、`0` 的保留；
- 非必填未知字段静默忽略；
- 图片、视频、音频及 HTTP(S)、`data:`、`asset://` 地址保留和转发；
- 创建响应、处理中响应、成功响应、失败响应和详细 usage 解析。

### 路由和计费测试

- 两个 Lucen 渠道分别使用固定秒 Key 和 Token Key；
- 路由策略选择正确的固定秒模型和 Token 模型；
- 成本/利润条件重叠时按现有路由结果选择，不固定分组优先级；
- 固定秒实际时长和 Token usage 分别进入正确计费路径；
- 缺少 usage/视频地址时不产生错误结算；
- 预扣、结算、退款和 quota 饱和审计不回归。

### Ark 端到端测试

使用 mock Lucen 上游验证创建、单查、列表和错误流程，并覆盖图片 + 视频 + 音频组合。真实上游验收以 `lucen-seedance-video-api-test-report.md` 的结论为基准，至少复核多模态成功、`seconds` 字符串时长行为、音轨和 usage。

## 10. 非目标

- 不实现 Lucen 删除任务接口。
- 不复制 NewAPIVideo 的任务提交、轮询和计费核心。
- 不在 Lucen 渠道中加入独立参数范围、能力声明或多 Key 分组选择器。
- 不修改 Ark SDK 的调用方式。
- 不允许非必填字段的静默忽略扩展到模型、内容、时长等影响能力或计费的字段。
