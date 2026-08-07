# 视频生成结果 S3 对象存储转存设计

## 1. 背景与目标

标准视频任务完成后，上游渠道会返回可下载的视频 URL。部分上游自有域名会暴露渠道来源，需要按管理员配置将视频转存到兼容 S3 标准协议的对象存储，并向下游返回对象存储预签名 URL。

本次目标：

- 支持 AWS S3、MinIO、以及提供 S3 兼容协议的 OSS、COS、OBS 等对象存储。
- 管理后台提供独立的单页对象存储配置表单。
- 支持“必须转存域名白名单”和“不转存域名黑名单”。
- 命中白名单时必须转存；命中黑名单时不转存；未命中默认不转存；同时命中时黑名单优先。
- 转存失败时任务失败，不回退或返回上游 URL。
- 转存成功后，下游只收到新生成的对象存储预签名 URL。

本次不处理 Suno `Data` 中的音频、图片和视频多媒体列表，只处理标准视频任务轮询结果，包括 Seedance、Kling、Vidu、Gemini/Vertex 等 `TaskPollingAdaptor` 视频任务。

## 2. 总体架构

### 2.1 配置模块

新增 `setting/object_storage` 分层配置模块，注册为 `object_storage.*`。配置更新完成后生成不可变运行时快照，并通过原子值替换，避免任务轮询读到半套配置。

配置项：

| 字段 | 含义 | 默认值/约束 |
| --- | --- | --- |
| `enabled` | 是否启用视频结果转存 | `false` |
| `endpoint` | S3 API Endpoint，用于写入、查询和删除对象 | 启用时必填，HTTP(S) URL |
| `public_endpoint` | 生成预签名下载 URL 使用的 S3 兼容 Endpoint/访问域名 | 启用时必填，HTTP(S) URL |
| `region` | S3 Region | 默认 `us-east-1` |
| `bucket` | Bucket 名称 | 启用时必填 |
| `access_key_id` | Access Key ID | 启用时必填 |
| `secret_access_key` | Secret Access Key | 启用时必填，只写 |
| `use_path_style` | 是否使用 Path Style | 默认 `false` |
| `max_video_size_mb` | 单个视频最大转存大小 | 默认 `512`，范围 `1-2048` |
| `expires_seconds` | 预签名 GET URL 有效期 | 默认 `86400`，范围 `60-604800` |
| `transfer_domain_whitelist` | 必须转存的域名白名单 | 空列表 |
| `no_transfer_domain_blacklist` | 不转存的域名黑名单 | 空列表 |

`public_endpoint` 必须是对象存储服务能够接受 S3 SigV4 查询签名的访问地址。若使用自定义域名或 CDN，管理员必须保证该域名向对象存储转发时保留签名所需的 Host、路径和查询参数。

### 2.2 S3 客户端

新增独立对象存储实现包，职责包括：

- 根据运行时快照创建和缓存 S3 客户端与预签名客户端。
- 使用自定义 Endpoint、Region、静态凭据和 `use_path_style`。
- 提供 `HeadObject`、流式 `PutObject`/multipart upload、`DeleteObject` 和 `PresignGetObject`。
- 不主动设置对象 ACL。桶的读写策略由管理员在云厂商控制台配置。

项目已经使用 AWS SDK v2。本功能新增 S3 service 和 uploader/presigner 依赖，不引入厂商专用 SDK。

### 2.3 任务集成层

新增统一的视频结果终态处理函数，由以下两条路径共同调用：

1. 定时轮询路径：`service.updateVideoSingleTask`。
2. Gemini/Vertex 实时查询路径：`relay.tryRealtimeFetch`。

统一处理函数接收任务、上游结果 URL 和任务结果，依次完成域名决策、安全下载、S3 上传、对象键保存和终态状态转换。这样实时查询不能绕过转存策略，也不会在定时轮询之前泄漏上游 URL。

## 3. 域名规则

### 3.1 输入格式

管理页面以逐行输入方式编辑两张域名列表。每一项支持：

- 精确域名，例如 `provider.example.com`。
- 通配子域名，例如 `*.example.com`。

配置保存时使用 `url.Hostname()` 等价规则归一化：转小写、忽略端口、移除尾部点。配置项不允许包含 URL scheme、路径、查询参数或空白域名。

### 3.2 匹配优先级

对上游原始结果 URL 的 Host 执行一次决策：

1. 命中黑名单：不转存。
2. 未命中黑名单且命中白名单：必须转存。
3. 未命中任何列表：不转存。

是否转存只由上游直接返回的原始 URL 决定。下载发生重定向时，每一跳仍执行 SSRF 校验，但不会改变已经确定的转存决策。

不转存时保持现有行为，任务继续保存并返回上游 URL。管理员负责把需要隐藏的上游自有域名加入白名单。

## 4. 对象键与预签名 URL

### 4.1 对象键

对象键固定为：

```text
<本站模型ID>/<公开任务ID>.<安全扩展名>
```

示例：

```text
doubao-seedance-2-0-fast/task_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx.mp4
doubao-seedance-2-0/task_yyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyy.mp4
```

- 顶层目录来自 `Task.Properties.OriginModelName`，即本站请求模型 ID。
- 禁止使用上游模型名、上游 URL 路径或上游随机文件名。
- 模型 ID 仅保留字母、数字、点、下划线和连字符，其他字符归一化为 `_`。
- 文件名使用公开任务 ID，保证稳定、可审计且不暴露上游标识。
- 扩展名优先从安全的视频 Content-Type 推导，其次使用 URL 的已知视频后缀，无法判断时使用 `.mp4`。

确定性对象键使重叠轮询或多实例并发上传具备幂等目标。上传前后可使用 `HeadObject` 检查同键对象；发现已存在时直接复用。

### 4.2 任务持久化

预签名 URL 不能永久写入任务记录，否则 URL 过期后重新查询任务仍会得到失效链接。

`TaskPrivateData` 新增对象存储结果字段，至少包括：

- `ResultObjectKey`：对象键。
- `ResultObjectContentType`：视频 Content-Type，用于响应和审计。

这些字段保存在现有 `private_data` JSON 中，不新增数据库列，不需要方言相关迁移。旧任务继续使用 `ResultURL`，保证 SQLite、MySQL 和 PostgreSQL 数据兼容。

### 4.3 动态签名

当任务存在 `ResultObjectKey` 时，每次对外生成响应都调用 `PresignGetObject`，使用当前 `expires_seconds` 生成新的 S3 SigV4 GET URL。覆盖的对外出口包括：

- OpenAI Video 查询响应。
- ARK/Seedance 查询和列表响应。
- 用户任务列表与任务详情。
- 视频内容代理和 Gemini/Vertex 查询响应。
- 终态用户响应审计数据。

为了避免修改每个渠道适配器内部的解析逻辑，在顶层响应构建完成后按协议结构覆盖公共结果字段：OpenAI Video 使用 `metadata.url`，ARK 使用 `content.video_url`，任务 DTO 使用 `result_url`。适配器原始 `Data` 仍只用于管理员审计。

## 5. 转存流程

上游任务返回成功状态后执行：

1. 解析并验证原始视频 URL。
2. 按黑名单、白名单、默认规则判断是否转存。
3. 不转存：沿用原始 URL，继续现有成功终态和计费结算。
4. 必须转存：根据本站模型 ID 和公开任务 ID 生成对象键。
5. 使用现有 SSRF 防护能力创建下载请求；每次重定向重新校验目标地址。
6. 要求下载响应为 2xx，并验证 Content-Type、文件后缀和大小上限。
7. 流式上传到 S3，避免将完整视频载入内存。
8. `HeadObject` 验证对象存在，保存 `ResultObjectKey` 和 Content-Type。
9. 生成新的预签名 URL，进入任务成功终态 CAS、计费结算和用户响应审计。

对象存储禁用或 URL 未命中白名单时不会发起 S3 请求。禁用状态允许保存未填写完整的配置草稿；启用状态必须通过完整配置校验，否则拒绝保存。

## 6. 错误、重试与并发

### 6.1 安全失败

以下情况均视为转存失败：

- 原始 URL 非 HTTP(S) 或 SSRF 校验失败。
- 下载响应非 2xx。
- Content-Type/扩展名不能确认是安全视频。
- 视频超过 `max_video_size_mb`。
- S3 上传、校验或预签名失败。

失败时不得把原始 URL 写入公共结果字段。有限重试耗尽后，把任务转换为失败终态，沿用现有失败退款和异步计费失败链路。

### 6.2 重试

一次轮询内执行有限次数重试，并使用短指数退避且响应请求上下文取消。仅网络错误、5xx、限流和临时 S3 错误可重试；配置错误、SSRF 拒绝、文件过大和不安全 Content-Type 直接失败。

### 6.3 多实例并发

对象键是确定性的。并发实例可能重复发起上传，但写入同一个对象键，不产生多个公共对象。上传失败时再次 `HeadObject`；若其他实例已经完成上传，则按成功处理。最终任务状态继续使用现有 CAS，只有终态 CAS 获胜者执行结算、退款和终态审计。

## 7. 管理接口

新增管理员专用接口：

- `GET /api/object-storage/settings`：读取非敏感配置，返回 `secret_configured`，不返回 Secret Access Key。
- `PUT /api/object-storage/settings`：一次性校验并原子保存完整配置。
- `POST /api/object-storage/test`：使用请求中的未保存配置测试连接。

保存接口约定：

- Secret Access Key 留空表示保留已保存密钥。
- 明确的 `clear_secret=true` 才清除密钥。
- 启用配置时，如果保存后没有有效 Secret Access Key，则拒绝请求。
- 配置写入使用数据库事务更新 Option 记录，提交成功后再替换运行时快照。
- 通用 `GET /api/option/` 必须继续过滤 Secret Access Key，避免通过旧接口泄漏。

测试连接会在固定探针前缀下写入极小对象，执行 `HeadObject`、生成预签名 GET URL 后删除。测试接口不得自动保存配置，也不得在日志中输出凭据、完整签名 URL 或请求体。

## 8. 管理后台

导航位置：`系统设置 -> Operations -> Object Storage`。

采用独立单页表单，页面内按以下顺序分区：

1. 启用状态。
2. 连接：Endpoint、Public Endpoint、Region、Bucket、Path Style。
3. 凭据：Access Key ID、Secret Access Key、密钥已配置状态和清除操作。
4. 链接与限制：`ExpiresSeconds`、最大视频大小。
5. 域名规则：必须转存白名单、不转存黑名单。
6. 操作：测试连接、保存。

表单要求：

- 使用 React Hook Form 和 Zod。
- Secret 字段默认空白，不回填后端密钥。
- 启用时显示必填校验；禁用时允许保留现有配置。
- 域名列表使用逐行输入，保存前标准化并显示重复/冲突提示。
- 测试连接和保存分别显示加载、成功和失败状态。
- 所有新增文案通过 `useTranslation()`，同步 en、zh、zh-TW、fr、ja、ru、vi 七种语言。

## 9. 测试策略

### 9.1 后端

- 配置解析、运行时快照、`expires_seconds` 和大小范围。
- Secret 不返回、留空保留、明确清除、原子保存失败不更新运行时。
- 精确域名、通配子域名、大小写、端口、尾部点、黑名单优先和默认不转存。
- 对象键只使用本站模型 ID 和公开任务 ID，并正确清理不安全字符。
- SSRF 拒绝、重定向逐跳校验、Content-Type 和大小上限。
- S3 Put/Head/Delete/Presign 请求，Path Style 与自定义 Endpoint。
- 预签名 URL 每次查询重新生成并采用当前 `expires_seconds`。
- 白名单命中时上传成功并隐藏上游 URL。
- 黑名单和未命中时保持原始 URL且不访问 S3。
- 上传失败时任务失败、不泄漏上游 URL，并走退款/计费失败路径。
- 定时轮询和 Gemini/Vertex 实时查询使用同一转存策略。
- 重叠轮询使用同一对象键，只有 CAS 获胜者结算和审计。

新增或大幅改写的 Go 测试使用 `testify/require` 完成前置和致命断言，使用 `testify/assert` 完成非致命值断言。

### 9.2 前端

- 单页表单完整渲染和分区。
- 启用/禁用状态与条件必填。
- Secret 留空保留和明确清除。
- `ExpiresSeconds` 范围、最大视频大小范围和域名列表校验。
- 测试连接与保存的加载、成功、失败行为。
- 无效域名、重复域名和跨列表冲突提示。
- 七种语言的新增键完整性。

完成前执行受影响 Go 测试、前端 Vitest、`bun run typecheck`、涉及文件 lint 和 `bun run build`。

## 10. 非目标

- 不处理 Suno 多媒体输出。
- 不实现厂商专用 TOS/OSS/COS/OBS SDK。
- 不提供永久公开 URL 模式，始终使用预签名 GET URL。
- 不自动创建 Bucket、修改 Bucket Policy、配置 CDN 或对象生命周期规则。
- 不转存输入素材，只处理标准视频任务的最终视频结果。
