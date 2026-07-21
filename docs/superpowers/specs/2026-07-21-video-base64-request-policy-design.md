# 视频生成 Base64 参考媒体请求策略设计

## 目标

为所有视频生成提交接口增加统一的请求保护策略：默认禁止客户端在 JSON 请求体中以内嵌 Base64 形式提交参考图片或参考视频，管理员可以通过全局配置显式放开。同时为视频生成 JSON 请求设置独立的请求体大小上限，在进入请求转换、渠道选择、计费和上游调用之前拒绝异常大请求。

该策略用于降低入口带宽、请求体缓存、JSON 解析、Base64 解码、上游转发、重试和 Go GC 带来的资源压力。正常 HTTP(S) 媒体 URL 和现有 multipart 文件上传继续可用。

## 决策摘要

- 视频生成 JSON 请求默认禁止 Base64 参考图片和参考视频。
- 管理员可通过全局开关放开 Base64 输入。
- 视频生成 JSON 请求默认最大 16 MB，管理员可在 1-128 MB 范围内调整。
- Base64 禁用错误返回 HTTP 400；请求体超限返回 HTTP 413。
- 策略在渠道选择、预扣费和上游调用前执行。
- multipart 文件上传不受视频 JSON 专用上限影响，继续使用现有全局限制。
- 查询任务、下载视频和上游响应中的 Base64 不在本策略范围内。

## 范围

策略覆盖以下视频生成提交入口：

- `POST /v1/video/generations`
- `POST /v1/videos`
- `POST /v1/videos/:video_id/remix`
- `POST /kling/v1/videos/text2video`
- `POST /kling/v1/videos/image2video`
- `POST /api/v3/contents/generations/tasks`
- Jimeng 官方接口中的 `CVSync2AsyncSubmitTask`

策略不覆盖：

- 视频任务查询请求；
- `GET /v1/videos/:task_id/content` 视频内容代理；
- 图片生成、聊天、音频或其他非视频端点；
- multipart 文件本身；
- 上游响应或已完成任务结果中的 Base64 视频。

Jimeng 官方接口使用 POST 同时承载提交和查询。`Action=CVSync2AsyncGetResult` 必须跳过本策略，仅 `Action=CVSync2AsyncSubmitTask` 视为视频生成提交。

## 当前链路约束

通用请求体由 `common.GetRequestBody` 和 `common.GetBodyStorage` 完整缓存，默认全局上限为 128 MB。磁盘缓存默认关闭；开启后只有达到配置阈值且磁盘容量允许的请求才使用磁盘。视频路由还存在 Kling、Seedance 和 Jimeng 请求转换器，其中 Kling 和 Jimeng 转换器会在认证、分发之前解析并缓存原始请求体。

因此，单纯在 `controller.RelayTask` 内校验已经太晚：请求体可能已经进入内存或磁盘，并被转换器解析。视频请求策略必须在任何会读取请求体的转换器之前执行，或在不读取请求体的转换器之后执行以保留协议上下文。

## 全局配置

新增 `setting/video_setting`，并注册到现有 `config.GlobalConfig`：

```go
type VideoSetting struct {
    Base64InputEnabled   bool `json:"base64_input_enabled"`
    JSONRequestBodyMaxMB int  `json:"json_request_body_max_mb"`
}
```

对应扁平配置键：

```text
video_setting.base64_input_enabled
video_setting.json_request_body_max_mb
```

默认值：

```text
base64_input_enabled = false
json_request_body_max_mb = 16
```

`json_request_body_max_mb` 只接受 1-128。管理接口在持久化前拒绝越界或非整数值。启动加载或其他内部路径遇到无效值时，运行时使用安全默认值 16 MB，不能把 0 或负数解释为无限制。

请求中间件通过只读的并发安全运行时快照获取设置。配置更新后同步快照并立即生效，避免请求处理与管理员更新之间的数据竞争。Base64 开关异常时保持默认关闭。

## 中间件边界

新增 `middleware.VideoRequestPolicy()`，只处理已知视频生成提交入口。路由顺序按现有转换器行为分别安排。

OpenAI 兼容视频路由不需要预转换，认证可先执行：

```text
RouteTag -> TokenAuth -> VideoRequestPolicy -> Distribute -> RelayTask
```

Kling 和 Jimeng 转换器会读取请求体，策略必须位于转换器之前：

```text
RouteTag -> VideoRequestPolicy -> RequestConvert -> TokenAuth -> Distribute -> RelayTask
```

Seedance 转换器只重写路径并设置原生协议上下文，不读取请求体，因此保留其在策略之前：

```text
RouteTag -> SeedanceRequestConvert -> VideoRequestPolicy -> TokenAuth -> Distribute -> RelayTask
```

这样 Seedance 被策略拒绝时仍可返回 ARK 原生错误结构。Jimeng 查询 Action 由策略直接识别并跳过，随后继续由现有转换器改写为 GET 查询。

对于必须在认证之前执行策略的 Kling 和 Jimeng 路由，未认证请求最多只会读取到视频 JSON 专用上限，不会进入通用 BodyStorage、请求转换、渠道选择或计费。这里的策略中间件是资源边界，TokenAuth 仍是权限边界。

## 请求处理流程

`VideoRequestPolicy` 按以下顺序处理：

1. 确认请求是受保护的视频生成提交；其他请求直接放行。
2. 检查 Content-Type。只有 `application/json` 及带参数的 JSON 类型进入专用策略；multipart 和其他类型交给现有链路。
3. 读取当前 `video_setting` 运行时快照。
4. 若 `Content-Length` 已知且大于上限，直接返回 413，不读取请求体。
5. 使用 `io.LimitReader` 或等价受限读取方式读取最多 `maxBytes+1`；这同时覆盖 chunked、缺少 Content-Length 和解压后长度更大的请求。
6. 实际读取超过上限时返回 413，不创建完整 BodyStorage。
7. 将未超限 body 放回 `c.Request.Body`，并写入现有可复用请求体缓存键，保证后续转换器和 `common.GetBodyStorage` 复用同一份字节。
8. Base64 开关已开启时跳过媒体检查，继续后续链路；独立请求体上限仍然生效。
9. Base64 开关关闭时，使用 `common.Unmarshal` 解析为结构化 JSON 并检查媒体字段。
10. JSON 语法错误不由策略层重新定义，body 保持可重读并交给现有分发器或适配器产生原有错误。
11. 命中禁止项时记录不含媒体内容的警告日志并终止请求。

全局 `MAX_REQUEST_BODY_MB` 和解压后限制继续保留。视频 JSON 上限是更严格的端点级保护，不替代全局保护。

## 媒体字段检测

检测采用结构化遍历，不对整个原始 JSON 做 `strings.Contains("data:")`。普通 prompt 或无关字符串中出现 `data:` 不应被拒绝。

### 受保护字段

检查现有视频格式中的图片和视频输入字段：

- 顶层 `image`、`video`、`image_tail`、`input_reference`；
- `images` 数组中的字符串元素；
- `image_url` 和 `video_url` 的字符串值；
- `image_url.url` 和 `video_url.url`；
- `content[]` 媒体项中的上述字段；
- `metadata` 以及其他嵌套对象中名称匹配上述媒体字段的值。

递归遍历可以覆盖 Kling、Jimeng 等把供应商原始参数保存在 metadata 的格式，但只有字段名具有明确图片或视频语义时才检查其字符串值。`audio`、`audio_url`、`prompt`、`text` 和普通 metadata 字符串不在检测范围。

字段名匹配采用大小写不敏感的标准化形式，并兼容 snake_case 与现有已知字段名。新增视频适配器如果引入新的参考图片或参考视频字段，必须将其加入受保护字段集合，并同时增加回归测试。

### Base64 分类

以下值判定为 Base64 媒体：

1. 受保护字段中的 `data:image/...;base64,...`；
2. 受保护字段中的 `data:video/...;base64,...`；
3. 明确媒体字段中的原始 Base64 字符串：不是允许的远程 URL，达到最小媒体候选长度，只包含标准或 URL-safe Base64 字符，并能按对应编码完整解码。

原始 Base64 检测只应用于媒体字段。短 ID、空字符串和普通文本不进入解码判断。检测只需确认编码格式，不校验媒体内容或长期保留解码结果；策略层不能为检测分配完整的解码后媒体副本。

HTTP 和 HTTPS URL 正常放行。其他由现有适配器明确支持的远程 URL scheme 继续放行；URL scheme 的合法性、安全性和下载限制仍由现有 URL/SSRF 校验负责，本策略不重复实现。

## 错误契约

### Base64 被禁用

OpenAI 兼容、Kling 和 Jimeng 路由返回 HTTP 400：

```json
{
  "error": {
    "message": "base64 reference media is disabled for video generation; use an HTTP(S) URL instead",
    "type": "invalid_request_error",
    "param": "content[0].image_url.url",
    "code": "video_base64_input_disabled"
  }
}
```

`param` 使用命中的结构化字段路径；不能包含字段值。

Seedance ARK 原生路由保持现有错误外形并返回 HTTP 400：

```json
{
  "error": {
    "code": "InvalidParameter.content",
    "message": "base64 reference media is disabled for video generation; use an HTTP(S) URL instead"
  }
}
```

### 视频 JSON 请求体超限

OpenAI 兼容、Kling 和 Jimeng 路由返回 HTTP 413：

```json
{
  "error": {
    "message": "video JSON request body exceeds the configured limit",
    "type": "invalid_request_error",
    "code": "video_request_body_too_large"
  }
}
```

Seedance ARK 原生路由保持 ARK 错误外形，状态码同样为 413，错误 code 使用 `InvalidParameter`。

两类错误都是本地请求错误：不进入 `Distribute`，不选择渠道，不预扣费，不创建任务，不触发重试，也不进入上游渠道自动禁用判断。

## 日志与数据保护

策略拒绝请求时使用警告日志，最多记录：

- 原始请求路径；
- 请求模型（仅在 JSON 已成功解析且字段为普通短字符串时）；
- Content-Length 或实际读取字节数；
- 命中的媒体字段路径；
- 生效的最大请求体配置。

日志不得记录 Base64 内容、媒体 URL 的查询参数、完整请求体、认证头或 token。该功能不新增数据库使用日志、计费日志或独立持久化审计表。

## 管理后台

在默认前端的“系统设置 -> 运维/性能”区域增加“视频请求保护”设置组：

- “允许视频生成使用 Base64 参考媒体”开关，默认关闭；
- “视频 JSON 请求体上限（MB）”数值输入，范围 1-128，默认 16。

关闭开关时显示简短说明：客户端应使用 HTTP(S) URL 或受支持的 multipart 上传。上限说明明确仅作用于视频 JSON 请求，Base64 开启后仍受该上限约束。

前端复用现有 Option 获取和更新接口、表单提交模式及设置组件。所有用户可见文本使用 `useTranslation()` 和英文源键，并同步 en、zh、fr、ru、ja、vi 六个项目支持的 locale 文件。

## 配置更新与兼容性

数据库中不存在新配置键时使用默认值，因此升级后立即默认禁止视频 Base64 输入并启用 16 MB JSON 上限。该行为是有意的安全默认变更，可能影响依赖 Base64 视频输入的现有客户端；管理员可在后台显式开启兼容开关，必要时同时调整上限。

开启 Base64 后不改变现有适配器的解码、请求转换或上游行为。若某个适配器原本不支持某种 Base64 形式，仍由该适配器返回原有请求错误。

multipart 上传、HTTP(S) URL、任务查询和视频下载保持兼容。无需数据库迁移；新键由现有 Option/config 机制持久化。

## 测试设计

新增或扩展的 Go 测试使用 `testify/require` 处理设置和致命断言，使用 `testify/assert` 检查非致命结果。测试必须是确定性的，不使用随机大输入、sleep 或性能时间比较。

### 中间件单元测试

- 默认配置拒绝 `data:image/...;base64,...`。
- 默认配置拒绝 `data:video/...;base64,...`。
- 默认配置拒绝已知媒体字段中的原始标准 Base64 和 URL-safe Base64。
- `content[].image_url.url`、`content[].video_url.url`、顶层 `image`、`image_tail`、`images[]` 和嵌套 metadata 均能定位准确字段路径。
- HTTP 和 HTTPS URL 正常放行。
- prompt 和普通 metadata 字符串中的 `data:` 不误判。
- 短 ID、空媒体值和普通非 Base64 字符串不误判。
- 开启 Base64 配置后，同一请求正常进入后续 handler。
- `Content-Length` 大于上限时不读取 body 并返回 413。
- 缺少 Content-Length 或 chunked body 超限时在 `limit+1` 处停止并返回 413。
- 恰好等于上限的请求可继续处理。
- 通过策略的 body 可被后续转换器和 `common.GetBodyStorage` 完整重读。
- malformed JSON 在未超限时交给后续 handler，不由策略更改错误语义。
- multipart 请求不进入 JSON 专用检查。

### 配置测试

- 未持久化配置时得到 `false` 和 16 MB。
- 合法配置更新立即反映到运行时快照。
- 0、负数、超过 128 和非整数上限不能通过管理接口保存。
- 启动加载到无效上限时运行时回退 16 MB。
- 并发读取设置时不直接读取正在被反射更新的配置结构。

### 路由与协议测试

- 所有范围内的视频提交路由都命中策略。
- 视频 GET 查询和内容下载不命中策略。
- Jimeng `CVSync2AsyncGetResult` POST 查询跳过策略。
- Kling 和 Jimeng 请求在转换器读取 body 前被拒绝。
- Seedance 策略错误保持 ARK 原生错误结构。
- multipart OpenAI 视频上传保持现有行为。
- 拒绝请求不会进入渠道选择、预扣费、上游请求、任务插入或重试。

### 前端测试与检查

- 表单默认值分别为关闭和 16。
- 上限范围和整数校验正确。
- 保存后使用精确的两个 `video_setting.*` 键。
- 配置重新加载后正确回显。
- 运行 `bun run i18n:sync`，确保六个目标 locale 不缺少新增键。
- 运行 `bun run build` 验证 TypeScript 和生产构建。

## 实现文件边界

预计新增或修改：

```text
setting/video_setting/config.go
setting/video_setting/config_test.go
middleware/video_request_policy.go
middleware/video_request_policy_test.go
router/video-router.go
model/option.go
controller/option.go
web/default/src/features/system-settings/types.ts
web/default/src/features/system-settings/operations/index.tsx
web/default/src/features/system-settings/maintenance/performance-section.tsx
web/default/src/i18n/locales/en.json
web/default/src/i18n/locales/zh.json
web/default/src/i18n/locales/fr.json
web/default/src/i18n/locales/ru.json
web/default/src/i18n/locales/ja.json
web/default/src/i18n/locales/vi.json
```

如果现有前端组件边界更适合拆出单独的 `video-request-protection-section.tsx`，实现计划可以采用该文件拆分，但不能改变本设计的配置键、默认值和行为边界。

## 验收标准

- 新安装和没有相关配置的升级实例默认拒绝视频 JSON 中的 Base64 参考图片和参考视频。
- 管理员开启全局开关后，Base64 输入恢复现有链路行为。
- 视频 JSON 请求默认超过 16 MB 返回 413，且不会先进入通用 BodyStorage。
- 管理员可把视频 JSON 上限设置为 1-128 MB，更新后立即生效。
- HTTP(S) 媒体 URL 和 multipart 上传保持可用。
- 所有视频提交格式得到与其协议一致的错误响应。
- 被策略拒绝的请求不选择渠道、不扣费、不重试、不创建任务。
- 日志和错误响应不包含 Base64 数据或完整请求体。
- 后端相关 Go 测试和默认前端生产构建通过。
