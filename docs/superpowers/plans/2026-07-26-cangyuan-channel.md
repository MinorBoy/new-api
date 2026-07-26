# 苍原算力 Seedance 渠道实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 新增苍原算力 Seedance task-only 渠道，让现有 Ark SDK 通过创建、单查和列表接口完成文生视频调用，并对未确认的媒体能力明确失败。

**Architecture:** 复用 MegaByAI 计划完成的 `newapivideo` 端点 profile 与直接响应投影，增加一个结构化的文生 JSON 编码器。苍原 profile 使用 `/v1/videos`，把 Ark `ratio` 转为 `aspect_ratio`，其余任务、隐私和计费逻辑继续共享。

**Tech Stack:** Go 1.22+、Gin、GORM v2、testify、React 19、TypeScript、i18next、Bun。

**Design:** `docs/superpowers/specs/2026-07-26-cangyuan-channel-design.md`

**Ark Contract:** 用户代码不变，统一覆盖 `POST /api/v3/contents/generations/tasks`、`GET /api/v3/contents/generations/tasks/{id}` 和 `GET /api/v3/contents/generations/tasks`。

**Prerequisite:** 依次完成 `2026-07-26-lucen-channel.md` 和 `2026-07-26-megabyai-channel.md`。MegaByAI 占用类型 63 并把 Dummy 移到 64；本计划使用类型 64。

---

## 文件结构

### 新增文件

- `relay/channel/task/newapivideo/text_request.go`：可配置比例字段的文生 `/v1/videos` JSON 编码器。
- `relay/channel/task/newapivideo/text_request_test.go`：苍原文生请求和拒绝边界。
- `e2e/cangyuan_upstream_e2e_test.go`：Ark 到 mock 苍原的生命周期测试。

### 修改文件

- `relay/channel/task/newapivideo/profile.go`、`adaptor.go`：Cangyuan profile 和 text-only dialect 分派。
- `constant/channel.go`、`constant/channel_test.go`：类型 64 和默认地址。
- `relay/relay_adaptor.go`、`relay/seedance_task.go`、`relay/relay_task.go`、`relay/relay_task_seedance_test.go`、`relay/cost_accounting_adaptor_test.go`：任务注册和成本能力回归。
- `controller/channel-test.go`、`controller/channel_test_internal_test.go`：通用测试排除。
- `web/src/features/channels/constants.ts`、`lib/channel-type-config.ts`、`lib/channel-utils.ts`、`web/tests/channel-type-config.test.ts`：管理端配置。
- `web/src/i18n/locales/{en,zh,zh-TW,fr,ru,ja,vi}.json`：翻译。

---

### Task 1: 实现苍原文生 JSON profile

**Files:**
- Modify: `relay/channel/task/newapivideo/profile.go`
- Modify: `relay/channel/task/newapivideo/adaptor.go`
- Create: `relay/channel/task/newapivideo/text_request.go`
- Create: `relay/channel/task/newapivideo/text_request_test.go`

- [ ] **Step 1: 写失败测试**

```go
func TestCangyuanProfile(t *testing.T) {
	adaptor := NewCangyuanTaskAdaptor()
	assert.Equal(t, "Cangyuan", adaptor.GetChannelName())
	assert.Equal(t, []string{"seedance-2.0-720p"}, adaptor.GetModelList())
	assert.Equal(t, "/v1/videos", adaptor.activeProfile().submitPath)
}

func TestBuildCangyuanTextRequest(t *testing.T) {
	request, err := parseARKRequest([]byte(`{
		"model":"doubao-seedance-2-0-260128",
		"content":[{"type":"text","text":"雨夜霓虹，镜头推进"}],
		"duration":8,"ratio":"16:9","resolution":"720p"
	}`), cangyuanProtocolProfile())
	require.NoError(t, err)

	body, err := buildTextVideoRequest(request, "seedance-2.0-720p", cangyuanProtocolProfile())
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"model":"seedance-2.0-720p","prompt":"雨夜霓虹，镜头推进",
		"aspect_ratio":"16:9","duration":8,"resolution":"720p"
	}`, string(body))
}
```

表驱动测试必须覆盖图片、视频、音频、`draft_task`、`generate_audio`、`draft=true`、非空 tools 和非默认 service tier。每个用例都先通过 `NewCangyuanTaskAdaptor().ValidateRequestAndSetAction(...)`，断言 HTTP 400 和明确 `arkRequestError`；不得只测试编码器。省略 duration/ratio/resolution 时不擅自写默认值。

- [ ] **Step 2: 运行测试并确认失败**

```powershell
go test ./relay/channel/task/newapivideo -run 'TestCangyuan|TestBuildCangyuan' -count=1
```

Expected: FAIL，Cangyuan profile 和 text encoder 尚不存在。

- [ ] **Step 3: 扩展 profile 的文生方言配置**

在 `profile.go` 增加：

```go
const videoRequestDialectTextJSON videoRequestDialect = "text_json"

type textRequestProfile struct {
	ratioField          string
	minimumDuration     int
	maximumDuration     int
	allowedRatios       []string
	allowedResolutions  []string
	rejectExplicitServiceTier bool
}
```

给 `protocolProfile` 增加 `textRequest *textRequestProfile`，并定义：

```go
func cangyuanProtocolProfile() protocolProfile {
	return protocolProfile{
		channelName: "Cangyuan",
		modelList: []string{"seedance-2.0-720p"},
		submitPath: "/v1/videos",
		pollPath: "/v1/videos/{task_id}",
		contentType: "application/json",
		requestDialect: videoRequestDialectTextJSON,
		textRequest: &textRequestProfile{
			ratioField: "aspect_ratio",
			minimumDuration: 1,
			maximumDuration: relaycommon.MaxTaskDurationSeconds,
			rejectExplicitServiceTier: true,
		},
	}
}

func NewCangyuanTaskAdaptor() *TaskAdaptor {
	return &TaskAdaptor{profile: cangyuanProtocolProfile()}
}
```

本地资料没有给出更小的时长区间，因此只应用公共安全上限；路由策略负责业务能力限制。

- [ ] **Step 4: 实现结构化文生编码器**

创建 `text_request.go`：

```go
package newapivideo

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

type textVideoRequest struct {
	Model       string `json:"model"`
	Prompt      string `json:"prompt"`
	Duration    *int   `json:"duration,omitempty"`
	Ratio       string `json:"ratio,omitempty"`
	AspectRatio string `json:"aspect_ratio,omitempty"`
	Resolution  string `json:"resolution,omitempty"`
}

func validateTextVideoRequest(request arkRequest, upstreamModel string, profile protocolProfile) error {
	config := profile.textRequest
	if config == nil {
		return fmt.Errorf("text request profile is missing")
	}
	for _, item := range request.Content {
		if item.Type != "text" {
			return &arkRequestError{Code: "InvalidParameter.content", Message: profile.channelName + " currently supports text-only requests"}
		}
	}
	if request.GenerateAudio != nil || request.Draft != nil || request.Tools != nil ||
		(config.rejectExplicitServiceTier && request.ServiceTier != nil) {
		return &arkRequestError{Code: "InvalidParameter", Message: profile.channelName + " does not support the requested control fields"}
	}
	if request.Duration != nil && (*request.Duration < config.minimumDuration || *request.Duration > config.maximumDuration) {
		return &arkRequestError{Code: "InvalidParameter.duration", Message: "duration is outside the provider range"}
	}
	if !stringAllowed(request.Ratio, config.allowedRatios) || !stringAllowed(request.Resolution, config.allowedResolutions) {
		return &arkRequestError{Code: "InvalidParameter", Message: "ratio or resolution is unsupported"}
	}
	return nil
}

func buildTextVideoRequest(request arkRequest, upstreamModel string, profile protocolProfile) ([]byte, error) {
	if err := validateTextVideoRequest(request, upstreamModel, profile); err != nil {
		return nil, err
	}
	config := profile.textRequest
	out := textVideoRequest{Model: upstreamModel, Prompt: arkPrompt(request.Content), Duration: request.Duration, Resolution: request.Resolution}
	if strings.EqualFold(config.ratioField, "aspect_ratio") { out.AspectRatio = request.Ratio } else { out.Ratio = request.Ratio }
	return common.Marshal(out)
}

func stringAllowed(value string, allowed []string) bool {
	if value == "" || len(allowed) == 0 { return true }
	for _, candidate := range allowed { if value == candidate { return true } }
	return false
}
```

不要用 `map[string]any` 拼接请求；DTO 保证字段名和可选值语义可测试。编码器再次调用纯校验函数只用于防御内部绕过。

- [ ] **Step 5: 在预扣前校验，并在 adaptor 中分派**

扩展 `ValidateRequestAndSetAction` 的 Ark dialect switch：公共 `validateARKRequest` 成功并保存 request state 后，对 `videoRequestDialectTextJSON` 调用 `validateTextVideoRequest(*state.ARK, "", a.activeProfile())`。此处不需要上游模型即可验证苍原的文生边界，错误继续转换为 HTTP 400；执行点必须早于 `EstimateBilling`、价格计算和 `PreConsumeBilling`。

`BuildRequestBody` 的 dialect switch 增加：

```go
case videoRequestDialectTextJSON:
	body, err = buildTextVideoRequest(*state.ARK, modelName, a.activeProfile())
```

复用 MegaByAI 已实现的 profile submit/poll path，不另写网络逻辑。E2E 的非法媒体用例必须断言上游请求数、任务记录数和用户 quota 变化均为 0，证明没有发生预扣。

- [ ] **Step 6: 格式化、测试和提交**

```powershell
gofmt -w relay/channel/task/newapivideo/profile.go relay/channel/task/newapivideo/adaptor.go relay/channel/task/newapivideo/text_request.go relay/channel/task/newapivideo/text_request_test.go
go test ./relay/channel/task/newapivideo -run 'TestCangyuan|TestBuildCangyuan|TestMegaByAI|TestLucen' -count=1
git add relay/channel/task/newapivideo/profile.go relay/channel/task/newapivideo/adaptor.go relay/channel/task/newapivideo/text_request.go relay/channel/task/newapivideo/text_request_test.go
git commit -m "feat(video): add Cangyuan request profile"
```

Expected: PASS。

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
- Modify: `controller/channel-test.go`
- Modify: `controller/channel_test_internal_test.go`

- [ ] **Step 1: 写注册失败测试**

```go
func TestCangyuanChannelConstants(t *testing.T) {
	require.Equal(t, 64, constant.ChannelTypeCangyuan)
	require.Equal(t, 65, constant.ChannelTypeDummy)
	require.Equal(t, "https://ai.cangyuansuanli.cn", constant.ChannelBaseURLs[constant.ChannelTypeCangyuan])
	require.Equal(t, "Cangyuan", constant.GetChannelTypeName(constant.ChannelTypeCangyuan))
	_, success := common.ChannelType2APIType(constant.ChannelTypeCangyuan)
	require.False(t, success)
}
```

增加 adaptor、Ark converter、任务列表平台和通用渠道测试排除断言。

- [ ] **Step 2: 运行失败测试**

```powershell
go test ./constant ./relay ./controller -run 'TestCangyuan|TestSupportsGenericChannelTest' -count=1
```

Expected: FAIL。

- [ ] **Step 3: 注册类型 64**

在 MegaByAI 后增加 `ChannelTypeCangyuan = 64`，把 Dummy 调整为 65；追加默认 URL 和名称。`GetTaskAdaptor` 返回 `newapivideo.NewCangyuanTaskAdaptor()`。

把 Cangyuan 加入：

- Ark Seedance 单查/列表平台集合；
- `/v1/videos/:id` 的 Ark converter 分支；
- task cost accounting 覆盖；
- `supportsGenericChannelTest` 禁止列表。

- [ ] **Step 4: 增加公开 ID 回归**

构造 `data:[{"url":"https://x/video.mp4"}]` 的成功任务，断言 Ark 单查返回 `content.video_url`，但不返回私有上游 ID、上游模型或渠道信息。

- [ ] **Step 5: 格式化、测试和提交**

```powershell
gofmt -w constant/channel.go constant/channel_test.go relay/relay_adaptor.go relay/seedance_task.go relay/relay_task.go relay/relay_task_seedance_test.go relay/cost_accounting_adaptor_test.go controller/channel-test.go controller/channel_test_internal_test.go
go test ./constant ./relay ./controller -run 'TestCangyuan|TestSeedanceTask|TestSupportsGenericChannelTest' -count=1
git add constant/channel.go constant/channel_test.go relay/relay_adaptor.go relay/seedance_task.go relay/relay_task.go relay/relay_task_seedance_test.go relay/cost_accounting_adaptor_test.go controller/channel-test.go controller/channel_test_internal_test.go
git commit -m "feat(cangyuan): register Seedance task channel"
```

---

### Task 3: 增加管理端渠道配置

**Files:**
- Modify: `web/src/features/channels/constants.ts`
- Modify: `web/src/features/channels/lib/channel-type-config.ts`
- Modify: `web/src/features/channels/lib/channel-utils.ts`
- Modify: `web/tests/channel-type-config.test.ts`
- Modify: `web/src/i18n/locales/{en,zh,zh-TW,fr,ru,ja,vi}.json`

- [ ] **Step 1: 写失败测试**

断言类型 64 名称为 `Cangyuan`，默认 URL 为 `https://ai.cangyuansuanli.cn`，模型目录为 `['seedance-2.0-720p']`，且属于 task-only 和 generic-test-unsupported，不属于 model-fetchable。

- [ ] **Step 2: 运行测试并确认失败**

```powershell
Set-Location web
bun test tests/channel-type-config.test.ts
```

Expected: FAIL。

- [ ] **Step 3: 实现配置**

在类型常量、显示顺序、task-only 集合、测试禁止集合、Key 提示和 warning 中加入 64。`CHANNEL_TYPE_CONFIGS[64]` 使用：

```ts
{
  id: 64,
  name: CHANNEL_TYPES[64],
  icon: 'NewAPI',
  defaultBaseUrl: 'https://ai.cangyuansuanli.cn',
  supportedModels: ['seedance-2.0-720p'],
  hints: {
    baseUrl: 'Default: https://ai.cangyuansuanli.cn',
    key: 'Enter the raw API key issued by Cangyuan',
    models: 'The documented initial model is seedance-2.0-720p; administrators may add verified models manually',
  },
}
```

加入 managed default URL 类型；图标复用 `NewAPI`。

- [ ] **Step 4: 添加七语言翻译并验证**

简体中文 warning 使用“苍原算力仅支持任务接口，请通过 Ark /api/v3 任务 API 调用”；模型提示明确“初始文档模型为 seedance-2.0-720p，已验证的其他模型可由管理员手工添加”。其他六种语言提供实际翻译。

```powershell
bun run i18n:sync
bun test tests/channel-type-config.test.ts
bun run typecheck
Set-Location ..
git add web/src/features/channels/constants.ts web/src/features/channels/lib/channel-type-config.ts web/src/features/channels/lib/channel-utils.ts web/tests/channel-type-config.test.ts web/src/i18n/locales/en.json web/src/i18n/locales/zh.json web/src/i18n/locales/zh-TW.json web/src/i18n/locales/fr.json web/src/i18n/locales/ru.json web/src/i18n/locales/ja.json web/src/i18n/locales/vi.json
git commit -m "feat(web): add Cangyuan channel configuration"
```

Expected: PASS。

---

### Task 4: Ark E2E、计费和真实验收

**Files:**
- Create: `e2e/cangyuan_upstream_e2e_test.go`
- Modify: `relay/relay_task_billing_test.go`
- Create after real success: `docs/superpowers/reports/2026-07-26-cangyuan-channel-acceptance.md`

- [ ] **Step 1: 写 mock Ark E2E**

从 `/api/v3/contents/generations/tasks` 提交 8 秒、16:9、720p 文生请求。mock 必须断言上游收到 `aspect_ratio` 而不是 `ratio`，返回私有 task ID；查询先返回 `in_progress`，再返回：

```json
{"task_id":"private","status":"completed","data":[{"url":"https://x/cangyuan.mp4"}]}
```

断言 Ark 创建只返回公开 ID，单查/列表返回成功 URL。失败分支断言公共退款。

- [ ] **Step 2: 增加未支持媒体 E2E**

提交带任意 `image_url` 的 Ark 请求，断言 HTTP 400、错误码 `InvalidParameter.content`，mock 上游请求计数为 0，未产生任务记录或扣费。

- [ ] **Step 3: 覆盖按时长计费并运行测试**

把现有计费 fixture 扩展到 `ChannelTypeCangyuan`，冻结请求时长 8 秒；成功保持结算，失败退款。

```powershell
go test ./e2e -run 'TestCangyuan' -count=1 -v
go test ./relay -run 'TestCangyuan|TestTaskBilling' -count=1 -v
```

Expected: PASS。

- [ ] **Step 4: 提交自动化测试**

```powershell
git add e2e/cangyuan_upstream_e2e_test.go relay/relay_task_billing_test.go
git commit -m "test(cangyuan): cover Ark video lifecycle"
```

- [ ] **Step 5: 运行全量验证**

```powershell
go test ./relay/channel/task/newapivideo ./constant ./relay ./controller ./e2e -count=1
go vet ./relay/channel/task/newapivideo ./relay ./controller ./constant
go build ./...
Set-Location web
bun test tests/channel-type-config.test.ts
bun run typecheck
bun run build
Set-Location ..
```

Expected: 全部退出码 0。

- [ ] **Step 6: 执行真实文生验收**

凭据从 `CANGYUAN_API_KEY` 读取。通过 new-api Ark 入口提交 `seedance-2.0-720p` 文生任务，保存脱敏的创建、处理中、完成和失败响应，核对 MP4、时长、账单和公开 ID。验收前不宣称图片/视频/音频已支持。

- [ ] **Step 7: 写报告并提交**

```powershell
git add docs/superpowers/reports/2026-07-26-cangyuan-channel-acceptance.md
git commit -m "docs: record Cangyuan channel acceptance"
```

报告必须记录本次只验证文生、资料缺失边界以及扩展多模态前需先更新设计和计划。

---

## 自检

| 设计要求 | 实施任务 |
| --- | --- |
| Ark 零代码创建/单查/列表 | Task 2、Task 4 |
| `ratio -> aspect_ratio` | Task 1 |
| 未确认媒体明确失败 | Task 1、Task 4 |
| `data[0].url` 和公开 ID | Task 2、Task 4 |
| 管理端默认 URL 和模型 | Task 3 |
| 计费与失败退款 | Task 2、Task 4 |
| 真实契约验收 | Task 4 |

本计划使用类型 64，并将 Dummy 移到 65；派普计划从 65 继续。所有字段、失败路径和命令均为确定内容。
