# Mikoto 视频渠道实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 新增默认禁用的 Mikoto task-only 视频渠道，通过现有 Ark SDK 接口调用 Mikoto 的 Sora 和 Seedance `/v1/videos` 协议。

**Architecture:** 在 `relay/channel/task/newapivideo` 中增加一个 Mikoto protocol profile，并按上游模型选择 Sora 或 Seedance 请求编码器。复用现有 Bearer 鉴权、异步任务持久化、轮询、Ark 响应转换、计费结算、退款和敏感信息清理；HTML 中的模型只用于协议方言判定，不生成正式价格或启用的模型模板。

**Tech Stack:** Go 1.22、Gin、GORM、`common.Marshal/Unmarshal`、`httptest`、`testify`、React 19、TypeScript、Bun、i18next、Ark 视频任务协议。

---

## 文件边界

**Mikoto 专属文件：**

- Create: `relay/channel/task/newapivideo/mikoto_request.go`
- Create: `relay/channel/task/newapivideo/mikoto_request_test.go`
- Create: `relay/channel/task/newapivideo/mikoto_response_test.go`
- Create: `e2e/mikoto_upstream_e2e_test.go`
- Modify: `relay/channel/task/newapivideo/profile.go`
- Modify: `relay/channel/task/newapivideo/adaptor.go`
- Modify: `relay/channel/task/newapivideo/dto.go`
- Modify: `relay/channel/task/newapivideo/response.go`

**共享后端注册文件：**

- Modify: `constant/channel.go`
- Modify: `constant/channel_test.go`
- Modify: `relay/relay_adaptor.go`
- Modify: `relay/seedance_task.go`
- Modify: `service/seedance_task_response.go`
- Modify: `relay/relay_task.go`
- Modify: `relay/relay_task_seedance_test.go`
- Modify: `relay/video_route_contract.go`
- Modify: `relay/video_route_contract_test.go`
- Modify: `relay/cost_accounting_adaptor_test.go`
- Modify: `relay/relay_task_billing_test.go`
- Modify: `controller/channel.go`
- Modify: `controller/channel-test.go`
- Modify: `controller/channel_test_internal_test.go`
- Modify: `service/config_import_stage.go`

**管理端与国际化：**

- Modify: `web/src/features/channels/constants.ts`
- Modify: `web/src/features/channels/lib/channel-type-config.ts`
- Modify: `web/src/features/channels/lib/channel-form.ts`
- Modify: `web/src/features/channels/lib/channel-utils.ts`
- Modify: `web/tests/channel-type-config.test.ts`
- Modify: `web/src/i18n/static-keys.ts`
- Modify: `web/src/i18n/locales/{en,zh,zh-TW,fr,ru,ja,vi}.json`

不修改两份上游 HTML，不增加 `CH-MIKOTO` 配置导入映射，不从 HTML 生成模型价格或成本规则。

## Task 1: Mikoto profile 与请求方言

**Files:**

- Modify: `relay/channel/task/newapivideo/profile.go`
- Create: `relay/channel/task/newapivideo/mikoto_request.go`
- Create: `relay/channel/task/newapivideo/mikoto_request_test.go`
- Modify: `relay/channel/task/newapivideo/adaptor.go`

- [x] **Step 1: 写 profile 和模型分流失败测试**

在 `mikoto_request_test.go` 添加 `TestMikotoProfileUsesDocumentedTaskContract`，断言：

```go
adaptor := NewMikotoTaskAdaptor()
profile := adaptor.activeProfile()
assert.Equal(t, ChannelNameMikoto, adaptor.GetChannelName())
assert.Empty(t, adaptor.GetModelList())
assert.Equal(t, "/v1/videos", profile.submitPath)
assert.Equal(t, "/v1/videos/{task_id}", profile.pollPath)
assert.Equal(t, videoRequestDialectMikoto, profile.requestDialect)
assert.Equal(t, mikotoDialectSora, mikotoRequestDialect("sora-v3-pro"))
assert.Equal(t, mikotoDialectSeedance, mikotoRequestDialect("seedance-fast-720p"))
assert.Equal(t, mikotoDialectUnknown, mikotoRequestDialect("unverified-model"))
```

- [x] **Step 2: 运行测试并确认因符号不存在而失败**

Run: `go test ./relay/channel/task/newapivideo -run 'TestMikotoProfile' -count=1`

Expected: FAIL，错误包含 `undefined: NewMikotoTaskAdaptor`。

- [x] **Step 3: 添加 profile 的最小实现**

在 `profile.go` 增加：

```go
const ChannelNameMikoto = "Mikoto"

const videoRequestDialectMikoto videoRequestDialect = "mikoto"

func mikotoProtocolProfile() protocolProfile {
	return protocolProfile{
		channelName:        ChannelNameMikoto,
		modelList:          []string{},
		submitPath:         "/v1/videos",
		pollPath:           "/v1/videos/{task_id}",
		contentType:        "application/json",
		requestDialect:     videoRequestDialectMikoto,
		allowEmbeddedMedia: true,
	}
}

func NewMikotoTaskAdaptor() *TaskAdaptor {
	return &TaskAdaptor{profile: mikotoProtocolProfile()}
}
```

- [x] **Step 4: 写 Sora 精确请求和边界失败测试**

添加表测试 `TestBuildMikotoSoraRequest`，使用 Ark 请求覆盖：纯文本、单首帧、首尾帧、9 图片、3 视频、3 音频，并精确断言以下上游结构：

```json
{
  "model": "sora-v3-pro",
  "prompt": "keep the subject consistent",
  "seconds": "10",
  "aspect_ratio": "16:9",
  "resolution": "720p",
  "image_url": "https://8.8.8.8/first.png",
  "reference_image_urls": ["https://8.8.4.4/last.png"],
  "video_config": {"reference_mode": "start_end"}
}
```

参考视频和参考音频使用独立的 `reference_mode=auto` 用例；`start_end` 不得与参考视频混用。

添加 `TestBuildMikotoSoraRequestRejectsDocumentedInvalidInputs`，逐项断言 `duration=3/16`、非 `720p`、比例 `2:1`、10 图片、4 视频、4 音频、总素材超过 12、音频无图片、`start_end` 混用视频、data URI、私网 URL、`seed`、`watermark` 和 `draft` 返回对应 `InvalidParameter.*`。

- [x] **Step 5: 运行 Sora 测试并确认因编码器不存在而失败**

Run: `go test ./relay/channel/task/newapivideo -run 'TestBuildMikotoSora' -count=1`

Expected: FAIL，错误包含 `undefined: buildMikotoRequest`。

- [x] **Step 6: 写 Seedance 精确请求和边界失败测试**

添加 `TestBuildMikotoSeedanceRequest`，断言字段名保持大小写，显式 `generate_audio: false` 不被省略：

```json
{
  "model": "seedance-2.0-720p",
  "prompt": "follow the references",
  "duration": 8,
  "aspect_ratio": "9:16",
  "images": ["https://8.8.8.8/ref.png"],
  "reference_mode": "media",
  "referenceVideos": ["data:video/mp4;base64,AAAA"],
  "referenceAudios": ["data:audio/mpeg;base64,AAAA"],
  "generate_audio": false
}
```

同一测试断言缺省 `generate_audio`、媒体和比例时这些字段完全省略，且不发送 `resolution`。添加 `TestBuildMikotoSeedanceRequestRejectsDocumentedInvalidInputs`，覆盖时长边界、比例、数量、错误角色、错误 data URI、单个 data URI 解码后超过 50 MB、未知模型和 Sora/Seedance 字段混用。

- [x] **Step 7: 运行 Seedance 测试并确认失败原因正确**

Run: `go test ./relay/channel/task/newapivideo -run 'TestBuildMikotoSeedance' -count=1`

Expected: FAIL，原因是 Mikoto 请求编码尚未实现，而不是测试 JSON 或 fixture 错误。

- [x] **Step 8: 实现两个请求 DTO、验证和编码**

在 `mikoto_request.go` 定义以下稳定接口；可选标量全部使用指针和 `omitempty`，JSON 通过 `common.Marshal`：

```go
type mikotoDialect string

const (
	mikotoDialectUnknown  mikotoDialect = ""
	mikotoDialectSora     mikotoDialect = "sora"
	mikotoDialectSeedance mikotoDialect = "seedance"
)

func mikotoRequestDialect(modelName string) mikotoDialect
func validateMikotoRequest(request arkRequest, upstreamModel string) error
func buildMikotoRequest(request arkRequest, upstreamModel string) ([]byte, error)
```

Sora DTO 使用 `Seconds *string`、`ImageURL *string`、`ReferenceImageURLs []string`、`ReferenceVideos []string`、`AudioURL any` 和 `VideoConfig *mikotoSoraVideoConfig`；Seedance DTO 使用 `Duration *int`、`Images []string`、`ReferenceVideos []string` 的 `json:"referenceVideos,omitempty"`、`ReferenceAudios []string` 的 `json:"referenceAudios,omitempty"` 与 `GenerateAudio *bool`。使用 `relaycommon.ParseTaskMediaURL` 区分 HTTP 与 data URI；Sora 仅接受公开 HTTPS，Seedance 接受公开 HTTPS 和媒体类型匹配的 data URI。data URI 大小由 Base64 解码长度计算，超过 50 MB 返回 `InvalidParameter.content`。

- [x] **Step 9: 把 Mikoto 接入验证与请求构建分支**

在 `TaskAdaptor.ValidateRequestAndSetAction` 的 provider 分支中调用 `validateMikotoRequest` 并将 `ProviderValidationComplete` 设为 true；在 `BuildRequestBody` 的 Ark switch 中加入：

```go
case videoRequestDialectMikoto:
	state, stateErr := getRequestState(c)
	if stateErr != nil || state.ARK == nil {
		return nil, fmt.Errorf("ARK request state is missing")
	}
	if !state.ProviderValidationComplete {
		return nil, fmt.Errorf("Mikoto provider validation is incomplete")
	}
	body, err = buildMikotoRequest(*state.ARK, modelName)
```

- [x] **Step 10: 格式化并运行请求测试**

Run:

```powershell
gofmt -w relay/channel/task/newapivideo/profile.go relay/channel/task/newapivideo/mikoto_request.go relay/channel/task/newapivideo/mikoto_request_test.go relay/channel/task/newapivideo/adaptor.go
go test ./relay/channel/task/newapivideo -run 'TestMikoto|TestBuildMikoto|TestZ5API|TestCangyuan' -count=1
```

Expected: PASS。

- [x] **Step 11: 提交请求方言**

```powershell
git add relay/channel/task/newapivideo/profile.go relay/channel/task/newapivideo/adaptor.go relay/channel/task/newapivideo/mikoto_request.go relay/channel/task/newapivideo/mikoto_request_test.go
git commit -m "feat(mikoto): add video request dialects"
```

## Task 2: Mikoto 响应投影

**Files:**

- Modify: `relay/channel/task/newapivideo/dto.go`
- Modify: `relay/channel/task/newapivideo/response.go`
- Create: `relay/channel/task/newapivideo/mikoto_response_test.go`

- [x] **Step 1: 写顶层 `content_url` 失败测试**

添加 `TestMikotoDirectTaskResponseProjection`，覆盖：

```go
tests := []struct{ body, wantStatus, wantURL string }{
	{`{"id":"m1","status":"queued"}`, "QUEUED", ""},
	{`{"id":"m1","status":"processing","progress":40}`, "IN_PROGRESS", ""},
	{`{"id":"m1","status":"completed","content_url":"https://assets.example/seedance.mp4"}`, "SUCCESS", "https://assets.example/seedance.mp4"},
	{`{"id":"m1","status":"completed","video_url":"https://assets.example/sora.mp4"}`, "SUCCESS", "https://assets.example/sora.mp4"},
}
```

另断言失败响应中的 Mikoto 私有任务 ID 和 API key 被替换，成功响应缺少 URL 时返回错误。

- [x] **Step 2: 运行测试并确认 `content_url` 用例失败**

Run: `go test ./relay/channel/task/newapivideo -run 'TestMikotoDirectTaskResponseProjection' -count=1`

Expected: FAIL，成功任务因没有识别顶层 `content_url` 而报 `has no result URL`。

- [x] **Step 3: 添加最小响应字段和 URL 优先级**

在 `directTask` 增加：

```go
ContentURL string `json:"content_url"`
```

在 `directTaskVideoURL` 的候选数组中按以下顺序读取：

```go
for _, value := range []string{
	task.ContentURL,
	task.VideoURL,
	task.URL,
	task.ResultURL,
} {
```

保持现有 `object`、`output`、`metadata`、`content` 和 `data` 兼容分支不变。

- [x] **Step 4: 格式化并运行响应回归测试**

Run:

```powershell
gofmt -w relay/channel/task/newapivideo/dto.go relay/channel/task/newapivideo/response.go relay/channel/task/newapivideo/mikoto_response_test.go
go test ./relay/channel/task/newapivideo -run 'TestMikoto|TestParse.*Task|TestConvertToArkVideoTask|TestZ5API' -count=1
```

Expected: PASS。

- [x] **Step 5: 提交响应投影**

```powershell
git add relay/channel/task/newapivideo/dto.go relay/channel/task/newapivideo/response.go relay/channel/task/newapivideo/mikoto_response_test.go
git commit -m "feat(mikoto): normalize video task responses"
```

## Task 3: 注册后端渠道类型 212

**Files:**

- Modify: `constant/channel.go`
- Modify: `constant/channel_test.go`
- Modify: `relay/relay_adaptor.go`
- Modify: `relay/seedance_task.go`
- Modify: `service/seedance_task_response.go`
- Modify: `relay/relay_task.go`
- Modify: `relay/relay_task_seedance_test.go`
- Modify: `relay/video_route_contract.go`
- Modify: `relay/video_route_contract_test.go`
- Modify: `relay/cost_accounting_adaptor_test.go`
- Modify: `controller/channel.go`
- Modify: `controller/channel-test.go`
- Modify: `controller/channel_test_internal_test.go`
- Modify: `service/config_import_stage.go`

- [ ] **Step 1: 写渠道注册失败测试**

先添加以下可观察契约：

```go
require.Equal(t, 212, constant.ChannelTypeMikoto)
require.Equal(t, 213, constant.ChannelTypeDummy)
require.Equal(t, "https://api.mikoto.vip", constant.ChannelBaseURLs[constant.ChannelTypeMikoto])
require.Equal(t, "Mikoto", constant.GetChannelTypeName(constant.ChannelTypeMikoto))
require.False(t, mapped)
adaptor := GetTaskAdaptor(constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeMikoto)))
require.NotNil(t, adaptor)
assert.Equal(t, "Mikoto", adaptor.GetChannelName())
assert.True(t, isSeedanceTaskPlatform(constant.TaskPlatform("212")))
```

在 `controller/channel_test_internal_test.go` 断言 generic channel test 不支持 Mikoto；在 controller 增加测试，断言新增 Mikoto 渠道被强制设为手动禁用。

- [ ] **Step 2: 运行注册测试并确认失败**

Run: `go test ./constant ./relay ./controller -run 'TestMikoto|TestSupportsGenericChannelTest' -count=1`

Expected: FAIL，原因是 `ChannelTypeMikoto` 尚未定义。

- [ ] **Step 3: 添加常量、默认 URL、名称和 TaskAdaptor**

在 `constant/channel.go` 分配：

```go
ChannelTypeMikoto = 212 // Mikoto Sora and Seedance task protocol
ChannelTypeDummy  = 213 // this one is only for count, do not add any channel after this
```

设置 `ChannelBaseURLs[ChannelTypeMikoto] = "https://api.mikoto.vip"` 和名称 `Mikoto`。在 `relay/relay_adaptor.go` 返回 `newapivideo.NewMikotoTaskAdaptor()`；不要加入 `common.ChannelType2APIType`。

- [ ] **Step 4: 注册任务生命周期、计费能力和默认禁用行为**

把 Mikoto 加入：

```text
service.IsSeedanceTaskPlatform
service.SeedanceTaskPlatformValues
relay.seedanceTaskPayload 的 Ark converter 强制集合
relay.relay_task 的 Ark 响应转换集合
controller.supportsGenericChannelTest 的排除集合
controller.isPreAcceptanceVideoChannel
service.supportsImportedTaskChannelType
```

在 `relay/cost_accounting_adaptor_test.go` 断言支持 `ValidatedRequest`、`UpstreamActual`、`UpstreamUsage`，保持现有任务账务实现不变。

- [ ] **Step 5: 写并实现模型能力路由契约**

先在 `relay/video_route_contract_test.go` 添加表测试：Sora 只允许 `720p`、4 至 15 秒、最多 9/3/3 且总数 12；四个 Seedance 模型按名称要求 480p、720p 或 1080p，允许 4 至 15 秒和最多 9/3/3；未知模型返回 `route_contract_model`。

在 `relay/video_route_contract.go` 增加：

```go
case constant.ChannelTypeMikoto:
	return validateMikotoVideoRoute(target)
```

`validateMikotoVideoRoute` 必须复用 `routeDurationWithin`、`allRouteResolutions` 和 `routeReferenceTotalMax`，不读取 HTML，不写价格。

- [ ] **Step 6: 格式化并运行后端注册测试**

Run:

```powershell
gofmt -w constant/channel.go constant/channel_test.go relay/relay_adaptor.go relay/seedance_task.go service/seedance_task_response.go relay/relay_task.go relay/relay_task_seedance_test.go relay/video_route_contract.go relay/video_route_contract_test.go relay/cost_accounting_adaptor_test.go controller/channel.go controller/channel-test.go controller/channel_test_internal_test.go service/config_import_stage.go
go test ./constant ./relay ./controller ./service -run 'TestMikoto|TestSeedanceTask|TestSupportsGenericChannelTest|TestVideoRouteContract|TestCostAccounting' -count=1
```

Expected: PASS。

- [ ] **Step 7: 提交后端注册**

```powershell
git add constant/channel.go constant/channel_test.go relay/relay_adaptor.go relay/seedance_task.go service/seedance_task_response.go relay/relay_task.go relay/relay_task_seedance_test.go relay/video_route_contract.go relay/video_route_contract_test.go relay/cost_accounting_adaptor_test.go controller/channel.go controller/channel-test.go controller/channel_test_internal_test.go service/config_import_stage.go
git commit -m "feat(mikoto): register task-only channel"
```

## Task 4: 管理端配置与七种语言

**Files:**

- Modify: `web/src/features/channels/constants.ts`
- Modify: `web/src/features/channels/lib/channel-type-config.ts`
- Modify: `web/src/features/channels/lib/channel-form.ts`
- Modify: `web/src/features/channels/lib/channel-utils.ts`
- Modify: `web/tests/channel-type-config.test.ts`
- Modify: `web/src/i18n/static-keys.ts`
- Modify: `web/src/i18n/locales/{en,zh,zh-TW,fr,ru,ja,vi}.json`

- [ ] **Step 1: 加载 `i18n-translate` skill**

在编辑 locale 前完整读取 `.agents/skills/i18n-translate/SKILL.md`，按其流程同步所有语言，不只修改英文和中文。

- [ ] **Step 2: 写渠道配置失败测试**

在 `web/tests/channel-type-config.test.ts` 增加：

```ts
expect(CHANNEL_TYPES[212]).toBe('Mikoto')
expect(CHANNEL_TYPE_OPTIONS).toContainEqual({ value: 212, label: 'Mikoto' })
expect(getChannelTypeIcon(212)).toBe('NewAPI')
expect(TASK_ONLY_CHANNEL_TYPES.has(212)).toBe(true)
expect(GENERIC_CHANNEL_TEST_UNSUPPORTED_TYPES.has(212)).toBe(true)
expect(MODEL_FETCHABLE_TYPES.has(212)).toBe(false)
expect(getChannelTypeConfig(212)).toMatchObject({
  id: 212,
  name: 'Mikoto',
  defaultBaseUrl: 'https://api.mikoto.vip',
  supportedModels: [],
})
```

并断言新建 Mikoto 渠道的初始状态是手动禁用，手工代理 URL不被默认 URL 覆盖。

- [ ] **Step 3: 运行前端测试并确认失败**

Run: `bun test tests/channel-type-config.test.ts`

Working directory: `web`

Expected: FAIL，原因是 type 212 尚未注册。

- [ ] **Step 4: 注册管理端渠道配置**

在 `constants.ts`、`channel-type-config.ts`、`channel-form.ts` 和 `channel-utils.ts` 中增加 type 212，配置如下：

```ts
212: {
  id: 212,
  name: CHANNEL_TYPES[212],
  icon: 'NewAPI',
  defaultBaseUrl: 'https://api.mikoto.vip',
  supportedModels: [],
  hints: {
    baseUrl: 'Default: https://api.mikoto.vip',
    key: 'Enter the raw API key issued by Mikoto',
    models: 'Map client-visible Ark model names to verified Mikoto upstream models',
  },
},
```

将 212 加入 task-only、generic-test unsupported、managed default URL 和 pre-acceptance disabled 集合，不加入 model-fetchable 集合。

- [ ] **Step 5: 添加静态 i18n key 和七种翻译**

登记以下英文源 key：`Mikoto`、`Default: https://api.mikoto.vip`、`Enter the raw API key issued by Mikoto`、`Map client-visible Ark model names to verified Mikoto upstream models`、`Mikoto is task-only. Enable it only after real upstream contract acceptance.`。七个 locale 都必须有对应值；品牌和 URL 保留原文，其余使用自然的目标语言表达。

- [ ] **Step 6: 同步并验证前端**

Run:

```powershell
bun run i18n:sync
bun test tests/channel-type-config.test.ts
bun run typecheck
bun run lint
bun run build
```

Working directory: `web`

Expected: 全部退出码为 0；非英文 locale 中不保留新增英文提示的回退值。

- [ ] **Step 7: 提交管理端配置**

```powershell
git add web/src/features/channels/constants.ts web/src/features/channels/lib/channel-type-config.ts web/src/features/channels/lib/channel-form.ts web/src/features/channels/lib/channel-utils.ts web/tests/channel-type-config.test.ts web/src/i18n/static-keys.ts web/src/i18n/locales
git commit -m "feat(web): add Mikoto channel configuration"
```

## Task 5: Ark 生命周期、路由和计费 E2E

**Files:**

- Create: `e2e/mikoto_upstream_e2e_test.go`
- Modify: `relay/relay_task_billing_test.go`

- [ ] **Step 1: 写双方言 mock upstream 生命周期失败测试**

创建 `httptest.Server`，记录 method、path、Authorization 和 body。Sora fixture 必须收到 `seconds/resolution/video_config`，Seedance fixture 必须收到 `duration/images/referenceVideos/referenceAudios/generate_audio`。两条链路均通过：

```text
POST /api/v3/contents/generations/tasks
GET /v1/videos/{private_task_id}
GET /api/v3/contents/generations/tasks/{public_task_id}
GET /api/v3/contents/generations/tasks
```

断言上游只见映射后的模型，用户只见公共任务 ID和客户端模型；上游 key、私有任务 ID和 Mikoto 原始错误不出现在 Ark 单查和列表。

- [ ] **Step 2: 运行 E2E 并确认渠道尚未完整注册而失败**

Run: `go test ./e2e -run 'TestMikoto' -count=1 -v`

Expected: FAIL，失败点是 Mikoto 渠道注册或请求契约，不允许测试绕过真实 relay 链路直接调用编码器。

- [ ] **Step 3: 增加成功、失败和本地拒绝场景**

E2E 必须覆盖 `queued -> processing -> completed`，Sora 的 `video_url`、Seedance 的 `content_url`，以及 `failed` 只退款一次。非法 duration、10 图片、Sora data URI 和未知上游模型应在本地返回 400，mock POST 计数保持 0；上游 401、429 和 500 使用现有错误结构且不泄露 key。

- [ ] **Step 4: 加入计费矩阵**

在 `relay/relay_task_billing_test.go` 添加 Mikoto 的按请求、按时长、终态成功结算和失败退款用例。duration/seconds 均通过现有 `MaxTaskDurationSeconds` 与 checked quota 路径；测试只通过正式 `PriceData.AddOtherRatio` 或既有 fixture 配置倍率，不直接写 `OtherRatios`。

- [ ] **Step 5: 运行 E2E 与计费回归**

Run:

```powershell
gofmt -w e2e/mikoto_upstream_e2e_test.go relay/relay_task_billing_test.go
go test ./e2e -run 'TestMikoto' -count=1 -v
go test ./relay -run 'TestMikoto|TestTaskBilling|TestVideoRouteContract' -count=1 -v
```

Expected: PASS。

- [ ] **Step 6: 提交生命周期覆盖**

```powershell
git add e2e/mikoto_upstream_e2e_test.go relay/relay_task_billing_test.go
git commit -m "test(mikoto): cover Ark lifecycle and billing"
```

## Task 6: 完整验证与交付门禁

**Files:**

- Modify: `docs/superpowers/plans/2026-08-11-mikoto-channel.md`，仅更新已执行步骤的复选框

- [ ] **Step 1: 加载完成前验证 skill**

完整读取 `superpowers:verification-before-completion`，所有完成声明必须引用本轮实际命令输出。

- [ ] **Step 2: 运行后端目标测试**

Run:

```powershell
go test ./relay/channel/task/newapivideo ./constant ./relay ./controller ./service ./router ./e2e -count=1
go vet ./...
go build ./...
```

Expected: 全部退出码为 0。

- [ ] **Step 3: 运行前端验证**

Run:

```powershell
bun run format:check
bun run typecheck
bun run lint
bun run build
```

Working directory: `web`

Expected: 全部退出码为 0。

- [ ] **Step 4: 检查变更范围和秘密**

Run:

```powershell
git diff --check
git status --short
git diff --stat HEAD~4..HEAD
rg -n "api\.mikoto\.vip.*token=|YOUR_API_KEY|Bearer [A-Za-z0-9_-]{20,}" --glob '!docs/new-channels/*.html'
```

Expected: 无空白错误、无本地数据库/日志/构建产物、无新增真实 key；只保留 HTML 原始资料中既有的保存来源 URL。

- [ ] **Step 5: 记录真实验收边界**

没有 Mikoto 凭据时，交付说明必须明确“mock 契约与本地生命周期通过，真实上游未验收，渠道保持 disabled”。只有从本机安全环境取得凭据并实际完成文本与多模态提交、轮询、结果下载、失败退款后，才创建脱敏的中文验收报告并允许管理员手动启用渠道。

## 自检矩阵

| 设计要求 | 计划任务 |
| --- | --- |
| Sora 与 Seedance 两套请求字段 | Task 1 |
| Bearer、提交和查询路径 | Task 1、Task 5 |
| 图片、视频、音频与 data URI 边界 | Task 1、Task 5 |
| `video_url` 与 `content_url` 状态投影 | Task 2、Task 5 |
| Ark 提交、单查和列表兼容 | Task 3、Task 5 |
| 模型目录不从 HTML 生成 | 文件边界、Task 3、Task 4 |
| 默认禁用与不可通用测试 | Task 3、Task 4 |
| 路由能力、计费、退款与饱和安全 | Task 3、Task 5 |
| 七种语言管理端配置 | Task 4 |
| 无凭据时不宣称真实验收 | Task 6 |

## 回滚

渠道默认禁用。发生上游协议不一致时，先禁用所有 type 212 渠道；代码回滚按提交逆序撤销 Mikoto E2E、管理端注册、后端注册、响应投影和请求方言。不要删除用户已有渠道或任务记录，保留失败测试和脱敏证据用于修正协议。
