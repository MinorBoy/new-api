# ZZone 渠道接入实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 新增默认禁用的 ZZone task-only 渠道，通过现有 Ark SDK 任务接口完成多模态视频提交、轮询、公开结果投影和内容下载，并以 Mock HTTP 契约测试代替当前不可执行的真实上游验收。

**Architecture:** 为 ZZone 分配独立 channel type，但复用 `relay/channel/task/newapivideo` 的任务、计费和 Ark 转换内核。ZZone 专属代码只负责 profile、请求 dialect、无结果 URL 的终态和 `/content` 下载；模型目录来自配置导入的 `CH-ZZONE`，不硬编码 HTML 示例模型。

**Tech Stack:** Go 1.22+、Gin、GORM、`httptest`、`testify`、React 19、TypeScript、Bun、i18next。

---

## 文件职责

- `constant/channel.go`：ZZone channel type、名称和默认 Base URL。
- `relay/channel/task/newapivideo/profile.go`：ZZone protocol profile 和 adaptor 构造函数。
- `relay/channel/task/newapivideo/zzone_request.go`：Ark 到 ZZone JSON 的校验与编码。
- `relay/channel/task/newapivideo/response.go`：允许 ZZone 成功轮询不带结果 URL。
- `relay/relay_adaptor.go`：按 ZZone task platform 创建共享 task adaptor。
- `relay/relay_task.go`、`relay/seedance_task.go`、`service/seedance_task_response.go`：把 ZZone 纳入 Ark 公开任务投影、列表和轮询平台集合。
- `controller/video_proxy.go`：使用私有上游任务 ID 和 Bearer Key 代理 ZZone `/content`。
- `controller/channel.go`、`controller/channel-test.go`：默认禁用 ZZone，并禁止通用聊天渠道测试。
- `service/config_import_stage.go`、`relay/video_route_contract.go`：识别 `CH-ZZONE` 并约束导入路由能力。
- `web/src/features/channels/*`：管理端渠道枚举、受管 Base URL、task-only 提示和默认禁用。
- `web/src/channel-config-converter/document.ts`：把 `CH-ZZONE` 转换为 ZZone channel type。
- `web/src/i18n/locales/*.json`：ZZone 管理端文案的七种语言翻译。
- `e2e/zzone_upstream_e2e_test.go`：Mock 上游的 Ark 生命周期、失败退款和无副作用验收。
- `docs/superpowers/reports/2026-08-11-zzone-channel-acceptance.md`：记录测试证据和真实 Canary 缺口。

### Task 1: 注册独立 ZZone 渠道和任务平台

**Files:**
- Modify: `constant/channel.go`
- Modify: `constant/channel_test.go`
- Modify: `relay/channel/task/newapivideo/profile.go`
- Modify: `relay/relay_adaptor.go`
- Modify: `relay/relay_task.go`
- Modify: `relay/seedance_task.go`
- Modify: `relay/relay_task_seedance_test.go`
- Modify: `service/seedance_task_response.go`
- Modify: `service/seedance_task_response_test.go`
- Modify: `controller/channel.go`
- Modify: `controller/channel-test.go`
- Modify: `controller/channel_test_internal_test.go`

- [ ] **Step 1: 写渠道常量、adaptor 可发现性和默认禁用的失败测试**

在现有表驱动测试中加入以下可观察契约：

```go
func TestZZoneChannelConstants(t *testing.T) {
	require.Equal(t, 212, constant.ChannelTypeZZone)
	require.Equal(t, 213, constant.ChannelTypeDummy)
	require.Equal(t, "https://zzone.cc.cd", constant.ChannelBaseURLs[constant.ChannelTypeZZone])
	require.Equal(t, "ZZone", constant.GetChannelTypeName(constant.ChannelTypeZZone))
	_, mapped := common.ChannelType2APIType(constant.ChannelTypeZZone)
	require.False(t, mapped)
}
```

同时断言：

```go
require.NotNil(t, relay.GetTaskAdaptor(constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeZZone))))
require.True(t, service.IsSeedanceTaskPlatform(constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeZZone))))
require.False(t, supportsGenericChannelTest(constant.ChannelTypeZZone))
require.True(t, isPreAcceptanceVideoChannel(constant.ChannelTypeZZone))
```

- [ ] **Step 2: 运行失败测试并确认缺少 ZZone 符号/注册**

Run:

```powershell
go test ./constant ./relay ./service ./controller -run 'TestZZone|TestSupportsGenericChannelTestRejectsDimensio'
```

Expected: FAIL，错误包含 `undefined: constant.ChannelTypeZZone` 或 ZZone 注册断言失败。

- [ ] **Step 3: 实现最小渠道常量和平台注册**

在 `constant/channel.go` 使用保留编号并顺延哨兵：

```go
ChannelTypeZ5API = 211
ChannelTypeZZone = 212 // ZZone Seedance task protocol
ChannelTypeDummy = 213 // this one is only for count, do not add any channel after this
```

同步加入：

```go
baseURLs[ChannelTypeZZone] = "https://zzone.cc.cd"
ChannelTypeZZone: "ZZone",
```

在 `relay/channel/task/newapivideo/profile.go` 同步定义最小可构造 profile，保证注册提交保持可编译：

```go
const ChannelNameZZone = "ZZone"

const videoRequestDialectZZone videoRequestDialect = "zzone"

func zzoneProtocolProfile() protocolProfile {
	return protocolProfile{
		channelName:                   ChannelNameZZone,
		modelList:                     []string{},
		submitPath:                    "/v1/videos",
		pollPath:                      "/v1/videos/{task_id}",
		contentType:                   "application/json",
		requestDialect:                videoRequestDialectZZone,
		requirePublicHTTPMedia:        true,
		untypedImagesAreReferences:    true,
		allowEmptyReferenceMediaRoles: true,
		allowAudioWithoutVisual:       true,
	}
}

func NewZZoneTaskAdaptor() *TaskAdaptor {
	return &TaskAdaptor{profile: zzoneProtocolProfile()}
}
```

在 `relay.GetTaskAdaptor` 注册：

```go
case constant.ChannelTypeZZone:
	return newapivideo.NewZZoneTaskAdaptor()
```

在 `relay/relay_task.go` 的 Ark 转换分支、`relay/seedance_task.go` 的强制 converter 分支，以及 `service.IsSeedanceTaskPlatform` / `SeedanceTaskPlatformValues` 中加入：

```go
constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeZZone))
```

在 `controller/channel-test.go` 的 `unsupportedChannelTypes` 和 `controller/channel.go` 的 `isPreAcceptanceVideoChannel` 中加入 `constant.ChannelTypeZZone`。把 `constant/channel_test.go` 中所有旧的 `ChannelTypeDummy == 212` 断言更新为 `213`。

- [ ] **Step 4: 运行 focused tests**

Run:

```powershell
go test ./constant ./relay ./service ./controller -run 'TestZZone|TestSupportsGenericChannelTestRejectsDimensio|TestSeedance'
```

Expected: PASS。

- [ ] **Step 5: 提交渠道身份和注册**

```powershell
git add constant/channel.go constant/channel_test.go relay/channel/task/newapivideo/profile.go relay/relay_adaptor.go relay/relay_task.go relay/seedance_task.go relay/relay_task_seedance_test.go service/seedance_task_response.go service/seedance_task_response_test.go controller/channel.go controller/channel-test.go controller/channel_test_internal_test.go
git commit -m "feat(zzone): register task channel"
```

### Task 2: 用 TDD 实现 ZZone 请求 dialect

**Files:**
- Create: `relay/channel/task/newapivideo/zzone_request.go`
- Create: `relay/channel/task/newapivideo/zzone_request_test.go`
- Modify: `relay/channel/task/newapivideo/adaptor.go`

- [ ] **Step 1: 写 profile 和请求编码失败测试**

测试必须断言空模型目录和文档协议，而不是示例模型：

```go
func TestZZoneProfileUsesDocumentedContract(t *testing.T) {
	adaptor := NewZZoneTaskAdaptor()
	profile := adaptor.activeProfile()
	assert.Equal(t, ChannelNameZZone, adaptor.GetChannelName())
	assert.Empty(t, adaptor.GetModelList())
	assert.Equal(t, "/v1/videos", profile.submitPath)
	assert.Equal(t, "/v1/videos/{task_id}", profile.pollPath)
	assert.Equal(t, videoRequestDialectZZone, profile.requestDialect)
	assert.True(t, profile.requirePublicHTTPMedia)
}
```

添加一个完整多模态用例，期望精确 JSON：

```go
request, err := parseARKRequest([]byte(`{
  "model":"client-model",
  "content":[
    {"type":"text","text":"city at night"},
    {"type":"image_url","role":"reference_image","image_url":{"url":"https://8.8.8.8/a.png"}},
    {"type":"video_url","role":"reference_video","video_url":{"url":"https://8.8.4.4/a.mp4"}},
    {"type":"audio_url","role":"reference_audio","audio_url":{"url":"https://1.1.1.1/a.mp3"}}
  ],
  "duration":15,
  "ratio":"9:16"
}`), zzoneProtocolProfile())
require.NoError(t, err)
body, err := buildZZoneRequest(request, "imported-zzone-model")
require.NoError(t, err)
assert.JSONEq(t, `{
  "model":"imported-zzone-model",
  "prompt":"city at night",
  "seconds":"15",
  "aspect_ratio":"9:16",
  "images":["https://8.8.8.8/a.png"],
  "videos":["https://8.8.4.4/a.mp4"],
  "audios":["https://1.1.1.1/a.mp3"]
}`, string(body))
```

表驱动失败用例必须包含：第 5 张图片、第 4 个视频、第 2 个音频、私网 URL、本地路径、`data:` URI、`21:9`、超限时长，以及显式的 `resolution`、`seed`、`watermark:false`、`generate_audio:false`、`service_tier:"default"`、`draft:false`、`tools:[]`。

- [ ] **Step 2: 运行测试并确认 dialect 尚未实现**

Run:

```powershell
go test ./relay/channel/task/newapivideo -run 'TestZZone'
```

Expected: FAIL，错误包含 `undefined: buildZZoneRequest` 或 ZZone 校验断言失败。

- [ ] **Step 3: 实现精确请求结构、上限和 unsupported-field 校验**

在 `zzone_request.go` 定义：

```go
type zzoneRequest struct {
	Model       string   `json:"model"`
	Prompt      string   `json:"prompt"`
	Seconds     *string  `json:"seconds,omitempty"`
	AspectRatio *string  `json:"aspect_ratio,omitempty"`
	Images      []string `json:"images,omitempty"`
	Videos      []string `json:"videos,omitempty"`
	Audios      []string `json:"audios,omitempty"`
}

var zzoneRatios = map[string]struct{}{
	"16:9": {}, "9:16": {}, "1:1": {},
}
```

`validateZZoneRequest` 先调用 `validateARKSemantics(request, zzoneProtocolProfile())`，再拒绝所有未声明字段，检查比例和 `4/3/1` 数量上限。`buildZZoneRequest` 用 `strconv.Itoa(*request.Duration)` 构造字符串 `seconds`，按 content 类型分别 append URL，最后调用 `common.Marshal`。

在 `adaptor.go` 的三个阶段加入同一 dialect：

```go
if profile.requestDialect == videoRequestDialectZZone {
	state, err := getRequestState(c)
	if err != nil || state.ARK == nil {
		return service.TaskErrorWrapperLocal(fmt.Errorf("ARK request state is missing"), "InvalidParameter", http.StatusBadRequest)
	}
	if err := validateZZoneRequest(*state.ARK); err != nil {
		var requestErr *arkRequestError
		if errors.As(err, &requestErr) {
			return service.TaskErrorWrapperLocal(err, requestErr.Code, http.StatusBadRequest)
		}
		return service.TaskErrorWrapperLocal(err, "InvalidParameter", http.StatusBadRequest)
	}
	return nil
}
```

在 `BuildRequestBody` 的 switch 中调用：

```go
case videoRequestDialectZZone:
	state, stateErr := getRequestState(c)
	if stateErr != nil || state.ARK == nil {
		return nil, fmt.Errorf("ARK request state is missing")
	}
	body, err = buildZZoneRequest(*state.ARK, modelName)
```

- [ ] **Step 4: 运行请求包测试并提交**

Run:

```powershell
go test ./relay/channel/task/newapivideo -run 'TestZZone|TestParseARKRequest'
```

Expected: PASS。

```powershell
git add relay/channel/task/newapivideo/adaptor.go relay/channel/task/newapivideo/zzone_request.go relay/channel/task/newapivideo/zzone_request_test.go
git commit -m "feat(zzone): encode Ark video requests"
```

### Task 3: 支持无 URL 成功轮询和安全内容代理

**Files:**
- Create: `relay/channel/task/newapivideo/zzone_response_test.go`
- Modify: `relay/channel/task/newapivideo/response.go`
- Create: `controller/video_proxy_zzone_test.go`
- Modify: `controller/video_proxy.go`

- [ ] **Step 1: 写无 URL 终态和公开身份失败测试**

```go
func TestZZoneParseTaskResultAllowsContentEndpointSuccess(t *testing.T) {
	result, err := NewZZoneTaskAdaptor().ParseTaskResult([]byte(`{
	  "id":"zzone-private","status":"completed","progress":100,"seconds":"15"
	}`))
	require.NoError(t, err)
	assert.Equal(t, string(model.TaskStatusSuccess), result.Status)
	assert.Empty(t, result.Url)
	assert.True(t, result.DurationPresent)
	assert.Equal(t, 15, result.DurationSeconds)
}
```

再覆盖 queued、processing、failed、expired 和未知状态；失败信息必须清除上游任务 ID 与 Bearer 字符串。构造 `model.Task`，令 `PrivateData.ResultURL` 为本地公开 `/content` URL，断言 `ConvertToArkVideoTask` 只输出公开 task ID、客户端模型名和公开 URL。

- [ ] **Step 2: 写内容代理失败测试**

以 `controller/video_proxy_eightyes_test.go` 为 fixture，新建 ZZone 测试并断言：

```go
assert.Equal(t, "/v1/videos/upstream%2Fprivate/content", upstreamPath)
assert.Equal(t, "Bearer zzone-key", upstreamAuthorization)
assert.Equal(t, "mp4-data", recorder.Body.String())
assert.Empty(t, recorder.Header().Get("Set-Cookie"))
assert.Empty(t, recorder.Header().Get("X-Provider-Request-Id"))
```

额外建立跨 origin redirect，断言重定向目标收不到 `Authorization`。

- [ ] **Step 3: 运行失败测试**

Run:

```powershell
go test ./relay/channel/task/newapivideo ./controller -run 'TestZZone'
```

Expected: FAIL；响应解析报 `successful new-api video task has no result URL`，代理测试报空 URL 或未命中上游。

- [ ] **Step 4: 允许 ZZone 无 URL 成功并实现 `/content` 代理**

在 `response.go` 改为按 dialect 判断：

```go
dialect := a.activeProfile().requestDialect
allowMissingSuccessURL := dialect == videoRequestDialectEightYes || dialect == videoRequestDialectZZone
```

在 `VideoProxy` switch 中加入：

```go
case constant.ChannelTypeZZone:
	videoURL = fmt.Sprintf(
		"%s/v1/videos/%s/content",
		strings.TrimRight(baseURL, "/"),
		url.PathEscape(task.GetUpstreamTaskID()),
	)
	req.Header.Set("Authorization", "Bearer "+channel.Key)
	client = publicMediaHTTPClient("Authorization")
```

保持现有 SSRF、60 秒超时、媒体类型白名单和公开响应头过滤逻辑不变。

- [ ] **Step 5: 运行测试并提交**

Run:

```powershell
go test ./relay/channel/task/newapivideo ./controller -run 'TestZZone'
```

Expected: PASS。

```powershell
git add relay/channel/task/newapivideo/response.go relay/channel/task/newapivideo/zzone_response_test.go controller/video_proxy.go controller/video_proxy_zzone_test.go
git commit -m "feat(zzone): proxy completed video content"
```

### Task 4: 接入配置导入和路由能力契约

**Files:**
- Modify: `service/config_import_stage.go`
- Modify: `service/config_import_stage_test.go`
- Modify: `relay/video_route_contract.go`
- Modify: `relay/video_route_contract_test.go`

- [ ] **Step 1: 写 `CH-ZZONE` 绑定和能力边界失败测试**

添加绑定 fixture：

```go
fixture := configImportBindingLineFixture{
	lineRef: "zzone-main", channelRef: "CH-ZZONE",
	channelType: constant.ChannelTypeOpenAI,
	protocol: "task", models: []string{"imported-zzone-model"},
}
```

断言该行只能绑定 `ChannelTypeZZone`，且 task protocol 校验通过。路由表测试至少覆盖：

```go
{
	name: "zzone accepts documented references", channelType: constant.ChannelTypeZZone,
	target: videoContractTarget("imported-zzone-model", []string{"720p"}, 1, 15,
		[]modelrouting.InputMode{modelrouting.InputModeText, modelrouting.InputModeOmniReference},
		modelrouting.ReferenceLimits{Images: 4, Videos: 3, Audios: 1}),
},
```

并分别断言 5 张图片、4 个视频、2 个音频、空 upstream model、超出 `MaxTaskDurationSeconds` 和 minimum 大于 maximum 时返回对应 `route_contract_*` 错误。

- [ ] **Step 2: 运行失败测试**

Run:

```powershell
go test ./service ./relay -run 'Test.*ZZone|TestValidateVideoRouteTargetContract'
```

Expected: FAIL，`CH-ZZONE` 仍解析为 OpenAI 或能力限制未被拒绝。

- [ ] **Step 3: 实现绑定归一化和能力检查**

在 `normalizedConfigImportBindingChannelType` 中加入：

```go
if channelRef == "CH-ZZONE" && sourceType == constant.ChannelTypeOpenAI {
	return constant.ChannelTypeZZone
}
```

在 `isConfigImportTaskChannelType` 中加入 `constant.ChannelTypeZZone`。在 `ValidateVideoRouteTargetContract` 增加 ZZone 分支，并实现：

```go
func validateZZoneVideoRoute(target modelrouting.Target) error {
	if strings.TrimSpace(target.UpstreamModel) == "" {
		return newVideoRouteContractError("route_contract_model", "ZZone mapped upstream model is required")
	}
	if !routeDurationWithin(target.Constraints.Durations, 1, relaycommon.MaxTaskDurationSeconds) {
		return newVideoRouteContractError("route_contract_duration", "ZZone route duration exceeds the task protocol limit")
	}
	limits := target.Constraints.ReferenceLimits
	minimums := target.Constraints.ReferenceMinimums
	if limits.Images > 4 || limits.Videos > 3 || limits.Audios > 1 ||
		minimums.Images > limits.Images || minimums.Videos > limits.Videos || minimums.Audios > limits.Audios {
		return newVideoRouteContractError("route_contract_references", "ZZone route reference limits exceed the documented protocol")
	}
	return nil
}
```

- [ ] **Step 4: 运行测试并提交**

Run:

```powershell
go test ./service ./relay -run 'Test.*ZZone|TestValidateVideoRouteTargetContract'
```

Expected: PASS。

```powershell
git add service/config_import_stage.go service/config_import_stage_test.go relay/video_route_contract.go relay/video_route_contract_test.go
git commit -m "feat(zzone): validate imported route contracts"
```

### Task 5: 完成 Mock Ark 生命周期与计费回归

**Files:**
- Create: `e2e/zzone_upstream_e2e_test.go`
- Modify: `relay/relay_task_billing_test.go`
- Modify: `relay/cost_accounting_adaptor_test.go`

- [ ] **Step 1: 写 Mock 上游生命周期失败测试**

以 `e2e/z5api_upstream_e2e_test.go` 的数据库和路由 fixture 为基础，Mock 必须实现：

```go
switch {
case request.Method == http.MethodPost && request.URL.Path == "/v1/videos":
	response = `{"id":"zzone-private-task","status":"queued","seconds":"15"}`
case request.Method == http.MethodGet && request.URL.Path == "/v1/videos/zzone-private-task":
	response = pollResponses[min(pollIndex, len(pollResponses)-1)]
case request.Method == http.MethodGet && request.URL.Path == "/v1/videos/zzone-private-task/content":
	writer.Header().Set("Content-Type", "video/mp4")
	_, _ = writer.Write([]byte("zzone-mp4"))
	return
}
```

生命周期断言包括：

- Ark 提交体精确变成 ZZone JSON，并带 `Bearer mock-zzone-key`。
- 轮询 `processing -> completed` 后任务成功且 `ResultURL` 是公开 `/v1/videos/{public_id}/content`。
- 单任务和列表接口只出现公开 ID/客户端模型，不出现私有 ID、Key、upstream model、`channel_id` 或 `user_id`。
- 失败任务只退款一次；400/401/429/500 创建错误不会重试或创建任务。
- 超限媒体和不支持字段在发出上游请求前返回 400，用户 quota 和日志无副作用。

- [ ] **Step 2: 运行 E2E 并确认平台尚未完整贯通**

Run:

```powershell
go test ./e2e -run 'TestZZone' -count=1
```

Expected: FAIL，失败点应是尚未贯通的注册、请求映射、轮询或公开 URL；不能通过放宽断言规避。

- [ ] **Step 3: 补齐共享计费能力断言**

在计费表驱动测试中加入 ZZone，期望与其他 `newapivideo` profile 相同：

```go
{
	name: "zzone", channel: constant.ChannelTypeZZone,
	expected: []types.CostMeterSource{
		types.CostMeterValidatedRequest,
		types.CostMeterUpstreamActual,
		types.CostMeterUpstreamUsage,
	},
}
```

为 duration=15 的 ZZone 请求复用现有预扣/结算 fixture，断言 quota 为正、失败退款归零、重复轮询不重复结算。

- [ ] **Step 4: 运行 E2E、计费和轮询测试**

Run:

```powershell
go test ./e2e ./relay ./service -run 'TestZZone|TestCostAccounting|Test.*Billing|Test.*Polling' -count=1
```

Expected: PASS。

- [ ] **Step 5: 提交 Mock 生命周期**

```powershell
git add e2e/zzone_upstream_e2e_test.go relay/relay_task_billing_test.go relay/cost_accounting_adaptor_test.go
git commit -m "test(zzone): cover mock Ark lifecycle"
```

### Task 6: 接入管理端、配置转换和 i18n

**Files:**
- Modify: `web/src/features/channels/constants.ts`
- Modify: `web/src/features/channels/lib/channel-type-config.ts`
- Modify: `web/src/features/channels/lib/channel-form.ts`
- Modify: `web/src/features/channels/lib/__tests__/new-api-channel.test.ts`
- Modify: `web/src/channel-config-converter/document.ts`
- Modify: `web/src/channel-config-converter/__tests__/v1.test.ts`
- Modify: `web/src/i18n/locales/en.json`
- Modify: `web/src/i18n/locales/zh.json`
- Modify: `web/src/i18n/locales/zh-TW.json`
- Modify: `web/src/i18n/locales/fr.json`
- Modify: `web/src/i18n/locales/ru.json`
- Modify: `web/src/i18n/locales/ja.json`
- Modify: `web/src/i18n/locales/vi.json`

- [ ] **Step 1: 加载 `i18n-translate` skill 后写前端失败测试**

测试必须断言：

```ts
assert.deepEqual(
  CHANNEL_TYPE_OPTIONS.find((item) => item.value === 212),
  { value: 212, label: 'ZZone' }
)
assert.equal(getChannelTypeConfig(212).defaultBaseUrl, 'https://zzone.cc.cd')
assert.deepEqual(getChannelTypeConfig(212).supportedModels, [])
assert.equal(getStatusOnChannelTypeChange(1, 212, 1), 2)
assert.equal(MODEL_FETCHABLE_TYPES.has(212), false)
```

在 V1 converter 测试 fixture 中增加 `CH-ZZONE`，断言 `channel_type === 212`，且转换结果的模型完全来自 fixture 行，不出现 `video-ds-2.0`、`video-ds-2.0-fast` 或 `as-sd2.0-fast`。

- [ ] **Step 2: 运行前端失败测试**

Run:

```powershell
bun test src/features/channels/lib/__tests__/new-api-channel.test.ts src/channel-config-converter/__tests__/v1.test.ts
```

Working directory: `web/`

Expected: FAIL，ZZone 配置或 `CH-ZZONE` 映射不存在。

- [ ] **Step 3: 实现渠道表单和转换映射**

在 `constants.ts` 的 `CHANNEL_TYPES`、显示顺序、`GENERIC_CHANNEL_TEST_UNSUPPORTED_TYPES`、`TASK_ONLY_CHANNEL_TYPES`、`TYPE_TO_KEY_PROMPT` 和 `CHANNEL_TYPE_WARNINGS` 中加入 212。警告 key 固定为：

```ts
212: 'ZZone is task-only. Enable it only after real upstream contract acceptance.'
```

在 `channel-type-config.ts` 增加：

```ts
212: {
  id: 212,
  name: CHANNEL_TYPES[212],
  icon: 'NewAPI',
  defaultBaseUrl: 'https://zzone.cc.cd',
  supportedModels: [],
  hints: {
    baseUrl: 'Default: https://zzone.cc.cd',
    key: 'Enter the raw API key issued by ZZone',
    models: 'Import ZZone models from channel configuration or add verified mappings manually',
  },
},
```

把 212 加入 `MANAGED_DEFAULT_BASE_URL_TYPES` 和 `PRE_ACCEPTANCE_DISABLED_CHANNEL_TYPES`。在 `document.ts` 的 `V1_CHANNEL_TYPES` 增加：

```ts
'CH-ZZONE': 212,
```

- [ ] **Step 4: 补齐七种语言并运行 i18n 校验**

使用项目 i18n 工具同步下列英文 source keys：`ZZone`、ZZone task-only 警告、Key 提示和模型导入提示。所有 locale 必须存在非空值；`zh.json` 的警告翻译为：

```json
"ZZone is task-only. Enable it only after real upstream contract acceptance.": "ZZone 仅支持任务接口，请仅在真实上游契约验收通过后启用。"
```

Run:

```powershell
bun run i18n:sync
bun test src/features/channels/lib/__tests__/new-api-channel.test.ts src/channel-config-converter/__tests__/v1.test.ts
```

Working directory: `web/`

Expected: i18n sync 无缺失 key，focused tests PASS。

- [ ] **Step 5: 提交管理端和配置转换**

```powershell
git add web/src/features/channels/constants.ts web/src/features/channels/lib/channel-type-config.ts web/src/features/channels/lib/channel-form.ts web/src/features/channels/lib/__tests__/new-api-channel.test.ts web/src/channel-config-converter/document.ts web/src/channel-config-converter/__tests__/v1.test.ts web/src/i18n/locales/en.json web/src/i18n/locales/zh.json web/src/i18n/locales/zh-TW.json web/src/i18n/locales/fr.json web/src/i18n/locales/ru.json web/src/i18n/locales/ja.json web/src/i18n/locales/vi.json
git commit -m "feat(zzone): add managed channel configuration"
```

### Task 7: 完成分层验证和验收报告

**Files:**
- Create: `docs/superpowers/reports/2026-08-11-zzone-channel-acceptance.md`
- Verify only: `web/scripts/channel-model-template/conversion-rules.json`

- [ ] **Step 1: 运行 provider 与注册层测试**

```powershell
go test ./constant ./relay/channel/task/newapivideo ./controller ./service -run 'TestZZone|TestSupportsGenericChannelTestRejectsDimensio|TestValidateVideoRouteTargetContract' -count=1
```

Expected: PASS。

- [ ] **Step 2: 运行 Ark 生命周期、计费和路由测试**

```powershell
go test ./e2e ./relay ./router ./service -run 'TestZZone|Test.*Seedance|Test.*Billing|Test.*Polling|TestVideo' -count=1
```

Expected: PASS。

- [ ] **Step 3: 运行前端测试、i18n 和生产构建**

```powershell
bun test src/features/channels/lib/__tests__/new-api-channel.test.ts src/channel-config-converter/__tests__/v1.test.ts
bun run i18n:sync
bun run build
```

Working directory: `web/`

Expected: focused tests、i18n sync 和 production build 均成功，并生成 `web/dist`。

- [ ] **Step 4: 重新运行全量 Go 基线与静态检查**

```powershell
go test ./...
git diff --check
git status --short
```

Expected: `go test ./...` PASS；`git diff --check` 无输出；status 只包含本任务预期文件。若根包仍报 `web/dist` 缺失，报告实际 `bun run build` 结果，不把全量测试写成通过。

- [ ] **Step 5: 校验模型来源和敏感信息边界**

```powershell
rg -n 'video-ds-2\.0|video-ds-2\.0-fast|as-sd2\.0-fast' constant relay controller service web/src
rg -n 'mock-zzone-key|Authorization: Bearer' --glob '!**/*_test.go' --glob '!docs/new-channels/cn-zzone.html'
rg -n '"14": "CH-ZZONE"' web/scripts/channel-model-template/conversion-rules.json
```

Expected: 生产代码没有 HTML 示例模型或测试 Key；转换规则仍准确包含 `"14": "CH-ZZONE"`。

- [ ] **Step 6: 写简体中文验收报告**

报告必须列出：每条命令、退出码、Mock 覆盖矩阵、默认 disabled 证据、模型来源检查、`git diff --check` 结果，以及以下明确结论：

```markdown
## 真实上游验收

未执行。当前未提供 ZZone API Key，因此没有验证真实提交、状态枚举、内容下载、鉴权错误或限流行为。Mock 契约通过不能替代真实上游 Canary；渠道继续保持 disabled。
```

- [ ] **Step 7: 提交验收报告**

```powershell
git add docs/superpowers/reports/2026-08-11-zzone-channel-acceptance.md
git commit -m "docs(zzone): record mock acceptance results"
```

## 完成标准

- ZZone 渠道类型为 212，`ChannelTypeDummy` 为 213，默认 Base URL 为 `https://zzone.cc.cd`。
- 管理端和后端创建/切换 ZZone 时都强制默认 disabled，通用渠道测试不可用。
- Ark 请求严格编码为 ZZone 的 `seconds` 字符串、`aspect_ratio` 和 `images/videos/audios` 数组，并执行公网 URL、`4/3/1`、时长和 unsupported-field 校验。
- Mock 上游覆盖提交、轮询成功/失败、未知状态、HTTP 错误、公开任务列表/详情、计费退款和安全内容代理。
- ZZone 模型只来自 `CH-ZZONE` 配置导入或管理员显式映射，生产代码没有 HTML 示例模型目录。
- focused Go tests、Ark 生命周期、计费测试、前端测试与构建通过；真实 Canary 明确保留为未完成并且渠道未启用。
