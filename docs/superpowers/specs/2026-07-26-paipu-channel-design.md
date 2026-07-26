# 派普 Seedance 渠道接入设计

**日期：** 2026-07-26  
**状态：** 已确认，进入接入计划阶段

## 1. 目标

为派普公开的 Seedance 模型增加独立任务渠道，使用户可继续通过标准 Ark SDK 的创建、单查和列表接口调用，用户代码不变。

本地派普资料只包含 `/v1/videos` 模型目录，不包含站点 Base URL、请求字段和响应样例。设计因此采用保守的 `/v1/videos` JSON 公共方言，并把真实上游契约验收设为发布前置条件，而不是虚构未确认的能力。

## 2. 已确认的模型目录

派普资料列出 24 个上游模型 ID：

```text
lec-sz-seedance-2-0-480p
lec-gongteng-seedance-2-0-720p
lec-gongteng-seedance-2-0-fast-720p
lec-gongteng-seedance-2-0-1080p
lec-seedance-2-0
lec-feituo-seedance-2-0-hn-fast-720p
lec-feituo-seedance-2-0-hn-720p
lec-feituo-seedance-2-0-xh-fast-933-720p
lec-feituo-seedance-2-0-xh-pro-933-720p
lec-feituo-seedance-2-0-ld-cvk-2
lec-feituo-seedance-2-0-limited-720p
lec-feituo-seedance-2-0-my-fast-upscaled-1080p
lec-feituo-seedance-2-0-my-upscaled-1080p
lec-seedance-videos-standard
lec-seedance-videos-face-standard
lec-seedance-videos-face-fast
lec-seedance-videos-stable
lec-seedance-videos-stable-fast
lec-seedance-videos-stable-mini
lec-seedance-videos-stable-720p
lec-seedance-videos-fast-720p
lec-seedance-videos-mini-720p
lec-seedance-videos-fast
lec-seedance-videos-mini
```

资料确认这些模型使用 `POST /v1/videos`。Base URL 未提供，因此管理端必须显式填写，后端不设置猜测默认值。

## 3. 架构决策

派普作为 `newapivideo` 的 `/v1/videos` JSON profile 接入：

1. 复用 MegaByAI 计划建立的直接任务提交、轮询和响应投影。
2. 初始请求只发送已被 `/v1/videos` 公共方言普遍定义的 `model`、`prompt`、`duration`、`ratio`、`resolution`。
3. 图片、视频、音频不会被静默删除；在真实契约证据补齐前，带媒体请求在访问上游前返回 `InvalidParameter.content`。
4. 24 个上游模型只作为管理员可选目录，不直接暴露成客户端标准模型；路由目标完成标准 Ark 模型到上游模型的映射。

备选方案中，直接复用 Lucen 多模态字段会把 `content/seconds` 契约强加给未确认的派普接口；独立复制 adaptor 又没有额外价值。因此均不采用。

## 4. Ark 请求语义

- `model` 和恰好一个非空文本项必填。
- `duration`、`ratio`、`resolution` 按 JSON 同名字段发送。
- 媒体内容、`draft_task`、`generate_audio`、`draft`、`tools` 和非默认 `service_tier` 初始均返回明确的 400。
- 可选控制字段不能在未确认语义时静默忽略。
- 分辨率后缀只作为路由能力和一致性校验线索。带明确 `480p/720p/1080p` 后缀的目标必须与 Ark 请求分辨率一致；没有分辨率后缀的模型由路由策略声明能力。
- Base URL 必填并在保存时去除尾部 `/`；用户误填以 `/v1` 结尾时继续使用现有管理端警告。

## 5. 响应、错误和隐私

- 创建响应接受 `id` 或 `task_id`，两者同时存在时必须一致。
- 查询使用 `GET /v1/videos/{private_task_id}`。
- 状态和 URL 使用 MegaByAI 计划扩展后的公共直接任务投影。
- 只有成功状态且结果 URL 非空才转换为 Ark `succeeded`。
- 上游私有 ID、上游模型、渠道 ID 和内部账单字段不得出现在公开 Ark 响应。
- 无法解析的 2xx 响应按网关错误处理；404/410 作为任务过期或不存在终态；5xx 保持可重试。

## 6. 发布门槛

由于资料缺少关键协议字段，以下验收必须在生产启用前完成：

1. 管理员提供真实 Base URL 和测试 API Key，但凭据不写入仓库。
2. 验证一个 720p 模型的创建、处理中、成功和失败响应。
3. 核对 `duration/ratio/resolution` 的字段名、类型和默认值。
4. 将脱敏响应固化为测试 fixture，并与通用解析器测试一致。
5. 如真实字段与本设计不同，先更新本设计和实施计划，再修改代码。

该门槛不使用模糊占位符或运行时猜测。代码可以支持管理员配置，但未通过门槛时渠道保持禁用状态。

## 7. 路由、计费和管理端

- 路由策略绑定具体渠道和上游模型，并声明文生、时长、比例和分辨率能力。
- 销售计费使用管理员配置的按次或按时长规则；缺少上游 usage 时不进行 Token 实际结算。
- 失败任务走公共退款路径。
- 管理端新增 task-only 派普渠道，Base URL 必填，模型目录提供 24 个选项，API Key 使用普通单 Key 输入。
- 不增加派普专用分组字段、多 Key 结构或模型自动抓取。

## 8. 测试策略

- 单测覆盖 24 个模型目录、Base URL 必填、模型/分辨率一致性、文生请求翻译和所有拒绝边界。
- mock E2E 覆盖 Ark 创建、单查、列表、公开 ID 隔离、成功结算和失败退款。
- 契约验收测试从脱敏 fixture 读取，不依赖持续可用的真实站点。
- 真实验收命令只从环境变量读取 Base URL 和 API Key，日志不得输出凭据。

## 9. 非目标

- 不在缺少证据时宣称派普支持多模态。
- 不猜测或硬编码派普站点域名。
- 不实现 Ark DELETE 和视频二进制代理下载。
