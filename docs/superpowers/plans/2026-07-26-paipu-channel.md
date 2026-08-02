# 派普 Seedance 渠道实施计划

> **协议更新说明：** Paipu 的请求编码、动态模型目录和创建重试语义已由 `2026-08-02-paipu-protocol-update-design.md` 与 `2026-08-02-paipu-protocol-update.md` 替代。本文仍保留最初接入背景和共享 Ark 生命周期设计，但旧的纯文本请求方言、24 模型静态目录及自动重试假设不再有效。

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 新增派普 Seedance task-only 渠道，使现有 Ark SDK 可通过创建、单查和列表接口调用派普的 24 个 `/v1/videos` 模型，并在上游契约尚未补齐时只开放可验证的文生能力。

**Architecture:** 复用苍原计划建立的 `newapivideo` 文生 JSON 编码器以及 MegaByAI 的直接任务响应投影。派普 profile 使用同名 `ratio` 字段和默认 Base URL `https://api.paipu.net`，并在请求出站前校验带分辨率后缀的上游模型与 Ark `resolution` 一致。

**Tech Stack:** Go 1.22+、Gin、GORM v2、testify、React 19、TypeScript、i18next、Bun。

**Design:** `docs/superpowers/specs/2026-07-26-paipu-channel-design.md`

**Ark Contract:** 用户代码不变，统一覆盖 `POST /api/v3/contents/generations/tasks`、`GET /api/v3/contents/generations/tasks/{id}` 和 `GET /api/v3/contents/generations/tasks`。

**Prerequisite:** 依次完成 `2026-07-26-lucen-channel.md`、`2026-07-26-megabyai-channel.md` 和 `2026-07-26-cangyuan-channel.md`。苍原占用类型 64 并把 Dummy 移到 65；本计划使用类型 65。

---

## 文件结构

### 新增文件

- `relay/channel/task/newapivideo/paipu_request_test.go`：派普 profile、24 个模型、文生请求和拒绝边界。
- `e2e/paipu_upstream_e2e_test.go`：Ark 到 mock 派普的完整任务生命周期。
- `docs/superpowers/reports/2026-07-26-paipu-channel-acceptance.md`：真实契约验收通过后创建的脱敏报告。

### 修改文件

- `relay/channel/task/newapivideo/profile.go`、`text_request.go`：派普 profile、模型目录和分辨率后缀校验。
- `constant/channel.go`、`constant/channel_test.go`：注册类型 65，Dummy 移到 66。
- `relay/relay_adaptor.go`、`relay/seedance_task.go`、`relay/relay_task.go`、`relay/relay_task_seedance_test.go`、`relay/cost_accounting_adaptor_test.go`：注册 task-only 提交、轮询和 Ark 查询。
- `controller/channel-test.go`、`controller/channel_test_internal_test.go`：通用聊天测试排除。
- `web/src/features/channels/constants.ts`、`lib/channel-type-config.ts`、`lib/channel-utils.ts`、`web/tests/channel-type-config.test.ts`：管理端类型和 24 模型目录。
- `web/src/i18n/locales/{en,zh,zh-TW,fr,ru,ja,vi}.json`：管理端文案。
- `relay/relay_task_billing_test.go`：按次、按时长和失败退款回归。

---

### Task 1: 增加派普文生 profile 和请求边界

**Files:**
- Modify: `relay/channel/task/newapivideo/profile.go`
- Modify: `relay/channel/task/newapivideo/adaptor.go`
- Modify: `relay/channel/task/newapivideo/text_request.go`
- Create: `relay/channel/task/newapivideo/paipu_request_test.go`

- [ ] **Step 1: 写 profile 和模型目录失败测试**

```go
var expectedPaipuModels = []string{
	"lec-sz-seedance-2-0-480p",
	"lec-gongteng-seedance-2-0-720p",
	"lec-gongteng-seedance-2-0-fast-720p",
	"lec-gongteng-seedance-2-0-1080p",
	"lec-seedance-2-0",
	"lec-feituo-seedance-2-0-hn-fast-720p",
	"lec-feituo-seedance-2-0-hn-720p",
	"lec-feituo-seedance-2-0-xh-fast-933-720p",
	"lec-feituo-seedance-2-0-xh-pro-933-720p",
	"lec-feituo-seedance-2-0-ld-cvk-2",
	"lec-feituo-seedance-2-0-limited-720p",
	"lec-feituo-seedance-2-0-my-fast-upscaled-1080p",
	"lec-feituo-seedance-2-0-my-upscaled-1080p",
	"lec-seedance-videos-standard",
	"lec-seedance-videos-face-standard",
	"lec-seedance-videos-face-fast",
	"lec-seedance-videos-stable",
	"lec-seedance-videos-stable-fast",
	"lec-seedance-videos-stable-mini",
	"lec-seedance-videos-stable-720p",
	"lec-seedance-videos-fast-720p",
	"lec-seedance-videos-mini-720p",
	"lec-seedance-videos-fast",
	"lec-seedance-videos-mini",
}

func TestPaipuProfile(t *testing.T) {
	adaptor := NewPaipuTaskAdaptor()
	assert.Equal(t, "Paipu", adaptor.GetChannelName())
	assert.Equal(t, expectedPaipuModels, adaptor.GetModelList())
	assert.Equal(t, "/v1/videos", adaptor.activeProfile().submitPath)
	assert.Equal(t, "/v1/videos/{task_id}", adaptor.activeProfile().pollPath)
	assert.Equal(t, "ratio", adaptor.activeProfile().textRequest.ratioField)
}
```

- [ ] **Step 2: 写文生翻译和省略字段测试**

```go
func TestBuildPaipuTextRequest(t *testing.T) {
	request, err := parseARKRequest([]byte(`{
		"model":"doubao-seedance-2-0-260128",
		"content":[{"type":"text","text":"海边日落，固定机位"}],
		"duration":8,"ratio":"16:9","resolution":"720p"
	}`), paipuProtocolProfile())
	require.NoError(t, err)
	body, err := buildTextVideoRequest(request, "lec-gongteng-seedance-2-0-720p", paipuProtocolProfile())
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"model":"lec-gongteng-seedance-2-0-720p","prompt":"海边日落，固定机位",
		"duration":8,"ratio":"16:9","resolution":"720p"
	}`, string(body))
}

func TestBuildPaipuTextRequestOmitsAbsentScalars(t *testing.T) {
	request, err := parseARKRequest([]byte(`{
		"model":"doubao-seedance-2-0-260128",
		"content":[{"type":"text","text":"minimal"}]
	}`), paipuProtocolProfile())
	require.NoError(t, err)
	body, err := buildTextVideoRequest(request, "lec-seedance-2-0", paipuProtocolProfile())
	require.NoError(t, err)
	assert.JSONEq(t, `{"model":"lec-seedance-2-0","prompt":"minimal"}`, string(body))
}
```

不得给派普补写资料中没有出现的默认时长、比例或分辨率。

- [ ] **Step 3: 写分辨率后缀和拒绝边界测试**

```go
tests := []struct {
	name       string
	model      string
	resolution string
	wantErr    bool
}{
	{name: "480p matches", model: "lec-sz-seedance-2-0-480p", resolution: "480p"},
	{name: "480p rejects 720p", model: "lec-sz-seedance-2-0-480p", resolution: "720p", wantErr: true},
	{name: "720p matches", model: "lec-feituo-seedance-2-0-hn-720p", resolution: "720p"},
	{name: "720p rejects 1080p", model: "lec-feituo-seedance-2-0-hn-720p", resolution: "1080p", wantErr: true},
	{name: "1080p matches", model: "lec-feituo-seedance-2-0-my-upscaled-1080p", resolution: "1080p"},
	{name: "unsuffixed accepts route resolution", model: "lec-seedance-videos-standard", resolution: "720p"},
}
```

另用表驱动请求覆盖 `image_url`、`video_url`、`audio_url`、`draft_task`、`generate_audio`、`draft=true`、非空 `tools` 和非默认 `service_tier`。每项必须通过 `NewPaipuTaskAdaptor().ValidateRequestAndSetAction(...)` 得到 HTTP 400 的 `arkRequestError`；媒体项错误码为 `InvalidParameter.content`。分辨率后缀用 `ValidateBillingRequest` 测试，因为只有模型映射完成后才有权威 `UpstreamModelName`。

- [ ] **Step 4: 运行测试并确认失败**

```powershell
go test ./relay/channel/task/newapivideo -run 'TestPaipu|TestBuildPaipu' -count=1
```

Expected: FAIL，`NewPaipuTaskAdaptor`、`paipuProtocolProfile` 和后缀校验尚不存在。

- [ ] **Step 5: 定义派普 profile**

在 `profile.go` 给 `textRequestProfile` 增加 `enforceModelResolutionSuffix bool`，把 Step 1 的 24 个值定义为 `paipuModels`，并增加：

```go
func paipuProtocolProfile() protocolProfile {
	return protocolProfile{
		channelName: "Paipu", modelList: append([]string(nil), paipuModels...),
		submitPath: "/v1/videos", pollPath: "/v1/videos/{task_id}",
		contentType: "application/json", requestDialect: videoRequestDialectTextJSON,
		textRequest: &textRequestProfile{
			ratioField: "ratio", minimumDuration: 1,
			maximumDuration: relaycommon.MaxTaskDurationSeconds,
			enforceModelResolutionSuffix: true,
			rejectExplicitServiceTier: false,
		},
	}
}

func NewPaipuTaskAdaptor() *TaskAdaptor {
	return &TaskAdaptor{profile: paipuProtocolProfile()}
}
```

- [ ] **Step 6: 在预扣前校验请求和映射后的分辨率后缀**

Paipu 的文生、控制字段、时长、比例和分辨率允许值沿用苍原计划中 `ValidateRequestAndSetAction` 对 `validateTextVideoRequest` 的调用，发生在价格计算前。分辨率后缀依赖模型映射，因此给 `TaskAdaptor.ValidateBillingRequest` 的 text dialect 分支在映射完成后、价格计算和预扣前调用同一个校验函数；在 `validateTextVideoRequest` 中加入：

```go
if config.enforceModelResolutionSuffix && request.Resolution != "" && upstreamModel != "" {
	requiredResolution := ""
	switch {
	case strings.HasSuffix(upstreamModel, "-1080p"):
		requiredResolution = "1080p"
	case strings.HasSuffix(upstreamModel, "-720p"):
		requiredResolution = "720p"
	case strings.HasSuffix(upstreamModel, "-480p"):
		requiredResolution = "480p"
	}
	if requiredResolution != "" && request.Resolution != requiredResolution {
		return nil, &arkRequestError{
			Code: "InvalidParameter.resolution",
			Message: fmt.Sprintf("model %s requires resolution %s", upstreamModel, requiredResolution),
		}
	}
}
```

`ValidateBillingRequest` 从 context 读取已解析的 `state.ARK`，并把 `info.UpstreamModelName` 传给 `validateTextVideoRequest`。把 `arkRequestError` 原样包装为 HTTP 400；profile 配置缺失才返回 500。Paipu 的 `rejectExplicitServiceTier=false` 只允许公共解析器已确认无副作用的 `service_tier="default"`，非默认值仍由公共 Ark 校验拒绝。无分辨率后缀的模型由路由 capability 约束，不在 adaptor 中猜测。`buildTextVideoRequest` 保留同一调用作为防御性复验，但正常 relay 路径不得到编码阶段才首次发现错误。

- [ ] **Step 7: 格式化、测试并提交**

```powershell
gofmt -w relay/channel/task/newapivideo/profile.go relay/channel/task/newapivideo/adaptor.go relay/channel/task/newapivideo/text_request.go relay/channel/task/newapivideo/paipu_request_test.go
go test ./relay/channel/task/newapivideo -run 'TestPaipu|TestBuildPaipu|TestCangyuan|TestMegaByAI|TestLucen' -count=1
git add relay/channel/task/newapivideo/profile.go relay/channel/task/newapivideo/adaptor.go relay/channel/task/newapivideo/text_request.go relay/channel/task/newapivideo/paipu_request_test.go
git commit -m "feat(video): add Paipu request profile"
```

Expected: PASS，苍原的 `aspect_ratio` 和派普的 `ratio` 各自保持正确。

---

### Task 2: 注册后端 task-only 渠道

**Files:**
- Modify: `constant/channel.go`
- Modify: `constant/channel_test.go`
- Modify: `relay/relay_adaptor.go`
- Modify: `relay/seedance_task.go`
- Modify: `relay/relay_task.go`
- Modify: `relay/relay_task_seedance_test.go`
- Modify: `relay/cost_accounting_adaptor_test.go`
- Modify: `controller/channel.go`
- Modify: `controller/channel-test.go`
- Modify: `controller/channel_test_internal_test.go`

- [ ] **Step 1: 写常量和 task-only 注册失败测试**

```go
func TestPaipuChannelConstants(t *testing.T) {
	require.Equal(t, 65, constant.ChannelTypePaipu)
	require.Equal(t, 66, constant.ChannelTypeDummy)
	require.Equal(t, "https://api.paipu.net", constant.ChannelBaseURLs[constant.ChannelTypePaipu])
	require.Equal(t, "Paipu", constant.GetChannelTypeName(constant.ChannelTypePaipu))
	_, success := common.ChannelType2APIType(constant.ChannelTypePaipu)
	require.False(t, success)
}
```

在 relay 测试断言 `GetTaskAdaptor("65")` 非空、名称为 `Paipu`、实现 `channel.ArkVideoTaskConverter` 与 `channel.TaskCostAccountingAdaptor`，并且 `seedanceTaskPlatformValues()` 包含 `"65"`。controller 测试断言类型 65 不支持通用聊天渠道测试；`validateChannel` 把 ` https://override.paipu.example/// ` 规范化为 `https://override.paipu.example`。

- [ ] **Step 2: 运行失败测试**

```powershell
go test ./constant ./relay ./controller -run 'TestPaipu|TestSupportsGenericChannelTest' -count=1
```

Expected: FAIL，类型和 adaptor 尚未注册。

- [ ] **Step 3: 注册类型、默认地址和任务平台**

在苍原后增加 `ChannelTypePaipu = 65`，把 Dummy 调整为 66；给 `ChannelBaseURLs[65]` 增加 `https://api.paipu.net`，给 `ChannelTypeNames` 增加 `Paipu`。`GetTaskAdaptor` 返回 `newapivideo.NewPaipuTaskAdaptor()`。

把 Paipu 加入 `isSeedanceTaskPlatform`、`seedanceTaskPlatformValues`、Ark converter 强制分支、task cost accounting 测试矩阵和 `supportsGenericChannelTest` 禁止列表。

在 `validateChannel` 中对非空 Paipu 覆盖值执行：

```go
if channel.Type == constant.ChannelTypePaipu && channel.BaseURL != nil {
	normalized := strings.TrimRight(strings.TrimSpace(*channel.BaseURL), "/")
	channel.BaseURL = &normalized
}
```

空值继续由 `ChannelBaseURLs[65]` 提供已确认默认地址。

- [ ] **Step 4: 增加公开响应隔离测试**

平台 65 的成功任务使用私有 ID `paipu-private-id`、上游模型 `lec-gongteng-seedance-2-0-720p` 和：

```json
{"task_id":"paipu-private-id","status":"completed","data":[{"url":"https://media.example/paipu.mp4"}]}
```

断言 Ark 单查和列表只返回公开 `task_*` ID、用户原始模型和 `content.video_url`，不返回私有 ID、上游模型、渠道 ID、Key、quota 或 routing 字段。

- [ ] **Step 5: 格式化、测试并提交**

```powershell
gofmt -w constant/channel.go constant/channel_test.go relay/relay_adaptor.go relay/seedance_task.go relay/relay_task.go relay/relay_task_seedance_test.go relay/cost_accounting_adaptor_test.go controller/channel.go controller/channel-test.go controller/channel_test_internal_test.go
go test ./constant ./relay ./controller -run 'TestPaipu|TestSeedanceTask|TestSupportsGenericChannelTest' -count=1
git add constant/channel.go constant/channel_test.go relay/relay_adaptor.go relay/seedance_task.go relay/relay_task.go relay/relay_task_seedance_test.go relay/cost_accounting_adaptor_test.go controller/channel.go controller/channel-test.go controller/channel_test_internal_test.go
git commit -m "feat(paipu): register Seedance task channel"
```

Expected: PASS。Paipu 不进入 `ChannelType2APIType`。

---

### Task 3: 增加 24 模型的管理端配置

**Files:**
- Modify: `web/src/features/channels/constants.ts`
- Modify: `web/src/features/channels/lib/channel-type-config.ts`
- Modify: `web/src/features/channels/lib/channel-utils.ts`
- Modify: `web/tests/channel-type-config.test.ts`
- Modify: `web/src/i18n/locales/{en,zh,zh-TW,fr,ru,ja,vi}.json`

- [ ] **Step 1: 写管理端失败测试**

```ts
test('configures Paipu as task-only with all documented models', () => {
  expect(CHANNEL_TYPES[65]).toBe('Paipu')
  expect(getDefaultBaseUrl(65)).toBe('https://api.paipu.net')
  expect(getChannelTypeConfig(65).supportedModels).toEqual([
    'lec-sz-seedance-2-0-480p',
    'lec-gongteng-seedance-2-0-720p',
    'lec-gongteng-seedance-2-0-fast-720p',
    'lec-gongteng-seedance-2-0-1080p',
    'lec-seedance-2-0',
    'lec-feituo-seedance-2-0-hn-fast-720p',
    'lec-feituo-seedance-2-0-hn-720p',
    'lec-feituo-seedance-2-0-xh-fast-933-720p',
    'lec-feituo-seedance-2-0-xh-pro-933-720p',
    'lec-feituo-seedance-2-0-ld-cvk-2',
    'lec-feituo-seedance-2-0-limited-720p',
    'lec-feituo-seedance-2-0-my-fast-upscaled-1080p',
    'lec-feituo-seedance-2-0-my-upscaled-1080p',
    'lec-seedance-videos-standard',
    'lec-seedance-videos-face-standard',
    'lec-seedance-videos-face-fast',
    'lec-seedance-videos-stable',
    'lec-seedance-videos-stable-fast',
    'lec-seedance-videos-stable-mini',
    'lec-seedance-videos-stable-720p',
    'lec-seedance-videos-fast-720p',
    'lec-seedance-videos-mini-720p',
    'lec-seedance-videos-fast',
    'lec-seedance-videos-mini',
  ])
  expect(TASK_ONLY_CHANNEL_TYPES.has(65)).toBe(true)
  expect(GENERIC_CHANNEL_TEST_UNSUPPORTED_TYPES.has(65)).toBe(true)
  expect(MODEL_FETCHABLE_TYPES.has(65)).toBe(false)
})
```

- [ ] **Step 2: 运行测试并确认失败**

```powershell
Set-Location web
bun test tests/channel-type-config.test.ts
Set-Location ..
```

Expected: FAIL，类型 65 不存在。

- [ ] **Step 3: 注册渠道目录**

在管理端类型常量、显示顺序、`TASK_ONLY_CHANNEL_TYPES`、`GENERIC_CHANNEL_TEST_UNSUPPORTED_TYPES` 和 Key 提示中增加 65；不要加入 `MODEL_FETCHABLE_TYPES`。

`CHANNEL_TYPE_CONFIGS[65]` 的 `supportedModels` 使用 Step 1 的完整数组，其他字段为：

```ts
{
  id: 65,
  name: CHANNEL_TYPES[65],
  icon: 'NewAPI',
  defaultBaseUrl: 'https://api.paipu.net',
  supportedModels: paipuModels,
  hints: {
    baseUrl: 'Default: https://api.paipu.net',
    key: 'Enter the raw API key issued by Paipu',
    models: 'Select from the 24 documented Paipu /v1/videos models',
  },
}
```

把 65 加入 `MANAGED_DEFAULT_BASE_URL_TYPES`，在 `channel-utils.ts` 将图标映射为 `NewAPI`。管理员可以覆盖默认地址；复用现有 `/v1` 尾缀警告。

- [ ] **Step 4: 添加七语言翻译**

| English key | zh | zh-TW |
| --- | --- | --- |
| `Paipu` | 派普 | 派普 |
| `Default: https://api.paipu.net` | 默认：https://api.paipu.net | 預設：https://api.paipu.net |
| `Enter the raw API key issued by Paipu` | 输入派普签发的原始 API Key | 輸入派普簽發的原始 API Key |
| `Select from the 24 documented Paipu /v1/videos models` | 从文档列出的 24 个派普 /v1/videos 模型中选择 | 從文件列出的 24 個派普 /v1/videos 模型中選擇 |
| `Paipu is task-only. Enable it only after real upstream contract acceptance.` | 派普仅支持任务接口。仅在真实上游契约验收通过后启用。 | 派普僅支援任務介面。僅在真實上游契約驗收通過後啟用。 |

`en.json` 映射为自身；`fr`、`ru`、`ja`、`vi` 提供实际翻译，不保留英文原文。

- [ ] **Step 5: 运行管理端验证并提交**

```powershell
Set-Location web
bun test tests/channel-type-config.test.ts
bun run i18n:sync
bun run format:check
bun run lint
bun run typecheck
bun run build
Set-Location ..
git add web/src/features/channels/constants.ts web/src/features/channels/lib/channel-type-config.ts web/src/features/channels/lib/channel-utils.ts web/tests/channel-type-config.test.ts web/src/i18n/locales/en.json web/src/i18n/locales/zh.json web/src/i18n/locales/zh-TW.json web/src/i18n/locales/fr.json web/src/i18n/locales/ru.json web/src/i18n/locales/ja.json web/src/i18n/locales/vi.json
git commit -m "feat(web): add Paipu channel configuration"
```

Expected: 全部退出码 0。不要暂存无关的 `_reports` 文件。

---

### Task 4: 证明 Ark 生命周期、计费和发布门槛

**Files:**
- Create: `e2e/paipu_upstream_e2e_test.go`
- Modify: `relay/relay_task_billing_test.go`
- Create after real success: `docs/superpowers/reports/2026-07-26-paipu-channel-acceptance.md`

- [ ] **Step 1: 写 mock Ark 生命周期测试**

创建类型 65 渠道，Base URL 指向 mock server，映射 `client-video -> lec-gongteng-seedance-2-0-720p`。通过 `POST /api/v3/contents/generations/tasks` 提交：

```json
{
  "model":"client-video",
  "content":[{"type":"text","text":"Paipu lifecycle"}],
  "duration":8,"ratio":"16:9","resolution":"720p"
}
```

mock 断言收到 `POST /v1/videos`、Bearer Key 和精确 JSON；创建返回 `{"task_id":"paipu-private","status":"queued"}`，轮询先返回 `in_progress`，再返回：

```json
{"task_id":"paipu-private","status":"completed","data":[{"url":"https://media.example/paipu.mp4"}]}
```

断言创建响应只含公开 ID；Ark 单查和列表返回 `model: client-video`、`status: succeeded` 和结果 URL，且不包含私有 ID、上游模型、Key、渠道 ID或 quota。

- [ ] **Step 2: 写失败退款和媒体拒绝测试**

失败 fixture 为：

```json
{"task_id":"paipu-private-failed","status":"failed","error":{"code":"provider_rejected","message":"request rejected"}}
```

断言只退款一次。再提交带 `reference_image` 的 Ark 请求，断言 HTTP 400、`InvalidParameter.content`、上游 POST 计数为 0、没有新任务和扣费记录。

- [ ] **Step 3: 扩展计费矩阵并提交测试**

把 `ChannelTypePaipu` 加入共享 task billing fixture：按次成功保持预扣、按时长使用请求的 8 秒、失败全额退款。duration 受 `relaycommon.MaxTaskDurationSeconds` 保护。

```powershell
gofmt -w e2e/paipu_upstream_e2e_test.go relay/relay_task_billing_test.go
go test ./e2e -run 'TestPaipu' -count=1 -v
go test ./relay -run 'TestPaipu|TestTaskBilling' -count=1 -v
git add e2e/paipu_upstream_e2e_test.go relay/relay_task_billing_test.go
git commit -m "test(paipu): cover Ark video lifecycle"
```

Expected: PASS。

- [ ] **Step 4: 运行全量验证和静态审计**

```powershell
go test ./relay/channel/task/newapivideo ./constant ./relay ./controller ./e2e -count=1
go test ./... -count=1
go vet ./...
go build ./...
Set-Location web
bun test tests/channel-type-config.test.ts
bun run i18n:sync
bun run typecheck
bun run build
Set-Location ..
rg -n 'ChannelTypePaipu|Paipu' constant relay controller web/src/features/channels web/tests e2e
rg -n 'int\(.*quota|int\(math\.|OtherRatios\[' relay/channel/task/newapivideo relay/relay_task_billing_test.go
git diff --check
git status --short
```

Expected: 全部退出 0；新增计费路径没有 bare quota cast 或直接写 `OtherRatios`。

- [ ] **Step 5: 执行真实上游契约验收**

只从本机环境变量读取凭据：

```powershell
$env:PAIPU_BASE_URL = 'https://api.paipu.net'
$env:PAIPU_API_KEY = '本机临时凭据'
```

通过 new-api Ark 入口验证一个 720p 模型的创建、处理中、成功和失败；确认 `/v1/videos/{private_task_id}` 查询路径、MP4 可读性、时长、分辨率、结算、退款和公开 ID。媒体请求必须在本地 400 且不访问上游。如真实字段或响应不同，先更新设计和本计划，再改实现；验收前渠道保持禁用。

- [ ] **Step 6: 固化 fixture 和验收报告**

把 Key、签名参数、用户数据和上游任务 ID 替换为固定测试值，再加入 `paipu_request_test.go` 或 `response_test.go`。报告只记录验证模型、字段契约、状态映射和限制。

```powershell
git add relay/channel/task/newapivideo/paipu_request_test.go docs/superpowers/reports/2026-07-26-paipu-channel-acceptance.md
git diff --cached --quiet; if ($LASTEXITCODE -eq 0) { throw 'No acceptance changes to commit' }
git commit -m "docs(paipu): record upstream contract acceptance"
```

如果 fixture 放在 `response_test.go`，只在该文件实际变化时一并暂存。

---

## 自检

| 设计要求 | 实施任务 |
| --- | --- |
| Ark SDK 创建、单查、列表零代码改动 | Task 2、Task 4 |
| 24 个模型完整目录 | Task 1、Task 3 |
| 默认 Base URL `https://api.paipu.net` | Task 2、Task 3 |
| 初期只开放文生，媒体明确 400 | Task 1、Task 4 |
| 分辨率后缀与请求一致 | Task 1 |
| 公开 ID、状态和 URL 投影 | Task 2、Task 4 |
| 按次/按时长计费与失败退款 | Task 4 |
| 真实请求/响应契约验收是启用门槛 | Task 4 |

本计划使用类型 65，并将 `ChannelTypeDummy` 移到 66。Secure 计划从类型 66 继续，不得与本计划并行执行渠道编号变更。
