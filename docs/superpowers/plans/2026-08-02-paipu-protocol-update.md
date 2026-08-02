# Paipu 渠道协议更新实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将 Paipu 从旧的纯文本请求方言升级为 Ark SDK 零代码兼容的多模态数组协议，同时让模型、能力和成本继续完全由配置导入驱动，并禁止创建任务自动重试。

**Architecture:** 在 `relay/channel/task/newapivideo` 增加独立的 Paipu request dialect 和 DTO，只负责 Ark 字段到 Paipu 协议的转换及 9/3/3 全局安全上限。路由 target 继续提供模型级时长、分辨率、素材上下限和成本模式；Paipu 后端及前端静态模型目录改为空。controller 的任务重试策略对 Paipu 创建请求 fail closed，任何上游错误都不执行第二次提交。

**Tech Stack:** Go 1.22、Gin、testify、httptest、GORM task fixtures、React 19、TypeScript、Bun、i18next、现有 modelrouting 与 channel-config converter。

---

## 执行工作树

在既有 `.worktrees/seedance-paipu` 中执行。该 worktree 当前分支 `feat/seedance-paipu` 已是 `ysr` 的祖先且工作树干净。实施前先把它快进到最新 `ysr`：

```powershell
git -C .worktrees/seedance-paipu merge --ff-only ysr
```

所有实现提交都落在 `feat/seedance-paipu`。验证完成后在主工作树将其 fast-forward 合并到本地 `ysr`，再推送 `fork/ysr`。不得提交主工作树中用户已有的 `AGENTS.md`、`CLAUDE.md` 和 `docs/superpowers/plans/2026-08-01-video-metadata-load-test.md` 修改。

## 文件职责

| 文件 | 职责 |
| --- | --- |
| `relay/channel/task/newapivideo/paipu_request.go` | Paipu 专用 DTO、协议校验、素材 URL/MIME 校验和请求编码 |
| `relay/channel/task/newapivideo/paipu_request_test.go` | Paipu profile、精确 JSON、字段省略、素材边界和错误契约 |
| `relay/channel/task/newapivideo/profile.go` | 注册 Paipu dialect，静态模型列表保持为空 |
| `relay/channel/task/newapivideo/native.go` | 仅增加 Paipu 所需的共享语义开关，不写模型分支 |
| `relay/channel/task/newapivideo/adaptor.go` | 在预扣前和编码前调度 Paipu provider validation |
| `relay/video_route_contract.go` | 校验导入的 Paipu route target 是否超出协议边界 |
| `controller/relay.go` | Paipu 创建任务禁止自动重试 |
| `e2e/paipu_upstream_e2e_test.go` | Ark 多模态创建、轮询、隐私、结算退款和单次上游提交 |
| `web/src/features/channels/lib/channel-type-config.ts` | Paipu 空静态模型目录和配置导入提示 |
| `web/src/channel-config-converter/__tests__/v1.test.ts` | 导入数据不受空静态目录影响 |

### Task 1: 用失败测试定义 Paipu 多模态请求契约

**Files:**
- Modify: `relay/channel/task/newapivideo/paipu_request_test.go`

- [ ] **Step 1: 把旧的静态模型断言改为动态 profile 断言**

```go
func TestPaipuProfileUsesDynamicModelsAndArrayDialect(t *testing.T) {
	adaptor := NewPaipuTaskAdaptor()
	profile := adaptor.activeProfile()
	assert.Equal(t, ChannelNamePaipu, adaptor.GetChannelName())
	assert.Empty(t, adaptor.GetModelList())
	assert.Equal(t, "/v1/videos", profile.submitPath)
	assert.Equal(t, "/v1/videos/{task_id}", profile.pollPath)
	assert.Equal(t, videoRequestDialectPaipuMediaArrays, profile.requestDialect)
	assert.True(t, profile.allowEmbeddedMedia)
	assert.True(t, profile.requirePublicHTTPMedia)
}
```

- [ ] **Step 2: 写精确 Ark 到 Paipu JSON 的失败测试**

```go
func TestBuildPaipuRequestPreservesMultimodalArrays(t *testing.T) {
	request, err := parseARKRequest([]byte(`{
		"model":"doubao-seedance-2-0-260128",
		"content":[
			{"type":"text","text":"人物跟拍"},
			{"type":"image_url","role":"reference_image","image_url":{"url":"https://8.8.8.8/ref.png"}},
			{"type":"video_url","role":"reference_video","video_url":{"url":"https://8.8.4.4/ref.mp4"}},
			{"type":"audio_url","role":"reference_audio","audio_url":{"url":"data:audio/wav;base64,UklGRg=="}}
		],
		"duration":5,"ratio":"16:9","resolution":"720p"
	}`), paipuProtocolProfile())
	require.NoError(t, err)
	body, err := buildPaipuRequest(request, "imported-paipu-model")
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"model":"imported-paipu-model","prompt":"人物跟拍",
		"duration":5,"aspect_ratio":"16:9","resolution":"720p",
		"images":["https://8.8.8.8/ref.png"],
		"videos":["https://8.8.4.4/ref.mp4"],
		"audios":["data:audio/wav;base64,UklGRg=="]
	}`, string(body))
}
```

- [ ] **Step 3: 写字段省略和任意导入模型名测试**

输入只有文本，映射模型使用 `vendor-model-from-import-v9`，精确断言：

```json
{"model":"vendor-model-from-import-v9","prompt":"minimal"}
```

不得检查该模型是否存在于代码白名单。

- [ ] **Step 4: 写素材边界、角色和 URL 安全测试**

用确定性 table test 覆盖 9/10 图片、3/4 视频、3/4 音频。分别断言 `first_frame`、`last_frame`、错误 MIME data URI、`asset://`、`file://`、私网 HTTP URL、多个文本项返回 `InvalidParameter.content`。合法 HTTP(S) 和匹配 MIME 的 data URI 必须通过。

- [ ] **Step 5: 写不支持标量测试**

覆盖 `generate_audio`、`watermark`、`seed`、`callback_url`、`draft=true`、非空 `tools`、`draft_task`、非默认 `service_tier`。每个用例通过 `ValidateRequestAndSetAction` 断言 HTTP 400 和准确错误码。

- [ ] **Step 6: 运行测试并确认 RED**

```powershell
go test ./relay/channel/task/newapivideo -run 'TestPaipu|TestBuildPaipu' -count=1
```

Expected: FAIL，原因是 Paipu dialect 和 builder 尚不存在，且当前解析器仍拒绝多模态；不能是测试语法错误。

### Task 2: 实现 Paipu 专用 dialect 和预扣前校验

**Files:**
- Create: `relay/channel/task/newapivideo/paipu_request.go`
- Modify: `relay/channel/task/newapivideo/profile.go`
- Modify: `relay/channel/task/newapivideo/native.go`
- Modify: `relay/channel/task/newapivideo/adaptor.go`
- Modify: `relay/channel/task/newapivideo/paipu_request_test.go`

- [ ] **Step 1: 注册独立 dialect 和动态 profile**

在 `profile.go` 增加 dialect，并删除 `paipuModels` 与 Paipu 的 `textRequestProfile`：

```go
const videoRequestDialectPaipuMediaArrays videoRequestDialect = "paipu_media_arrays"

func paipuProtocolProfile() protocolProfile {
	return protocolProfile{
		channelName:                    ChannelNamePaipu,
		modelList:                      []string{},
		submitPath:                     "/v1/videos",
		pollPath:                       "/v1/videos/{task_id}",
		contentType:                    "application/json",
		requestDialect:                 videoRequestDialectPaipuMediaArrays,
		allowEmbeddedMedia:             true,
		requirePublicHTTPMedia:         true,
		untypedImagesAreReferences:     true,
		allowEmptyReferenceMediaRoles: true,
		allowAudioWithoutVisual:        true,
	}
}
```

在 `protocolProfile` 增加：

```go
untypedImagesAreReferences     bool
allowEmptyReferenceMediaRoles bool
allowAudioWithoutVisual       bool
```

- [ ] **Step 2: 只通过 profile 开关放宽共享 Ark 语义**

在图片角色 switch 中，当角色为空且 `untypedImagesAreReferences=true` 时增加 `referenceImageCount`，不要增加 `firstCount`。这样多个无角色 Paipu 图片不会被共享首尾帧规则误判。

视频和音频角色允许空值的逻辑必须写成 `allowEmptyReferenceMediaRoles` 条件；`audioCount > 0` 且没有图片/视频的限制也只在 `allowAudioWithoutVisual=false` 时执行。其他 provider 的现有语义保持不变。

- [ ] **Step 3: 创建 Paipu DTO 和素材 URL 校验**

```go
type paipuRequest struct {
	Model       string   `json:"model"`
	Prompt      string   `json:"prompt"`
	Duration    *int     `json:"duration,omitempty"`
	AspectRatio *string  `json:"aspect_ratio,omitempty"`
	Resolution  *string  `json:"resolution,omitempty"`
	Images      []string `json:"images,omitempty"`
	Videos      []string `json:"videos,omitempty"`
	Audios      []string `json:"audios,omitempty"`
}

func validPaipuMediaURL(value, mediaPrefix string) bool {
	media, err := relaycommon.ParseTaskMediaURL(value)
	if err != nil {
		return false
	}
	if media.Kind == relaycommon.TaskMediaURLData {
		return strings.HasPrefix(strings.ToLower(media.Value), "data:"+mediaPrefix+"/")
	}
	if media.Kind != relaycommon.TaskMediaURLHTTP {
		return false
	}
	return validMediaURL(media.Value, paipuProtocolProfile())
}
```

该 helper 拒绝 `asset://`、错误 MIME 和 SSRF 目标。

- [ ] **Step 4: 实现 provider 校验**

新增 `validatePaipuRequest(request arkRequest, upstreamModel string) error`。先调用 `validateARKSemantics`，然后校验：映射模型非空时只要求 `TrimSpace` 后非空；图片角色只允许空或 `reference_image`；媒体 URL MIME 匹配；数量不超过 9/3/3；所有设计中列出的不支持字段返回确定性错误。不得读取静态模型列表。

- [ ] **Step 5: 实现精确请求编码**

```go
func buildPaipuRequest(request arkRequest, upstreamModel string) ([]byte, error) {
	if err := validatePaipuRequest(request, upstreamModel); err != nil {
		return nil, err
	}
	result := paipuRequest{
		Model: upstreamModel, Prompt: arkPrompt(request.Content),
		Duration: request.Duration, AspectRatio: request.Ratio, Resolution: request.Resolution,
	}
	for _, item := range request.Content {
		switch item.Type {
		case "image_url":
			result.Images = append(result.Images, item.ImageURL.URL)
		case "video_url":
			result.Videos = append(result.Videos, item.VideoURL.URL)
		case "audio_url":
			result.Audios = append(result.Audios, item.AudioURL.URL)
		}
	}
	return common.Marshal(result)
}
```

- [ ] **Step 6: 接入两阶段 provider validation**

`ValidateRequestAndSetAction` 使用空上游模型执行协议校验；`ValidateBillingRequest` 使用最终 `info.UpstreamModelName` 再校验并设置 `ProviderValidationComplete=true`；`BuildRequestBody` 只有看到该标记才能调用 `buildPaipuRequest`。用户错误必须在预扣前返回。

- [ ] **Step 7: 运行 focused tests 并确认 GREEN**

```powershell
gofmt -w relay/channel/task/newapivideo/profile.go relay/channel/task/newapivideo/native.go relay/channel/task/newapivideo/adaptor.go relay/channel/task/newapivideo/paipu_request.go relay/channel/task/newapivideo/paipu_request_test.go
go test ./relay/channel/task/newapivideo -run 'TestPaipu|TestBuildPaipu|TestOmegaAI|TestFourSToken|TestEightYes' -count=1
```

Expected: PASS，其他 array dialect 无回归。

- [ ] **Step 8: 提交请求方言**

```powershell
git add relay/channel/task/newapivideo/profile.go relay/channel/task/newapivideo/native.go relay/channel/task/newapivideo/adaptor.go relay/channel/task/newapivideo/paipu_request.go relay/channel/task/newapivideo/paipu_request_test.go
git diff --cached --check
git commit -m "feat(paipu): support Ark multimodal requests"
```

### Task 3: 让导入路由成为模型能力唯一来源

**Files:**
- Modify: `relay/video_route_contract.go`
- Modify: `relay/video_route_contract_test.go`
- Modify: `service/config_import_stage_test.go`

- [ ] **Step 1: 写动态模型 route target 失败测试**

用任意 `vendor-model-from-import-v9`、`InputModeOmniReference`、全局最大时长和 9/3/3 引用上限替换旧的 Paipu 纯文本用例。新增 10 图、4 视频、4 音频、空上游模型、超长时长及最低数量大于上限的失败用例。

- [ ] **Step 2: 运行测试并确认 RED**

```powershell
go test ./relay -run TestValidateVideoRouteTargetContract -count=1
```

Expected: FAIL，因为 Paipu 仍调用 `validateTextOnlyVideoRoute`。

- [ ] **Step 3: 实现动态路由契约**

```go
func validatePaipuVideoRoute(target modelrouting.Target) error {
	if strings.TrimSpace(target.UpstreamModel) == "" {
		return newVideoRouteContractError("route_contract_model", "Paipu mapped upstream model is required")
	}
	if !routeDurationWithin(target.Constraints.Durations, 1, relaycommon.MaxTaskDurationSeconds) {
		return newVideoRouteContractError("route_contract_duration", "Paipu route duration exceeds the task protocol limit")
	}
	limits := target.Constraints.ReferenceLimits
	minimums := target.Constraints.ReferenceMinimums
	if limits.Images > 9 || limits.Videos > 3 || limits.Audios > 3 ||
		minimums.Images > limits.Images || minimums.Videos > limits.Videos || minimums.Audios > limits.Audios {
		return newVideoRouteContractError("route_contract_references", "Paipu route reference limits exceed the protocol")
	}
	return nil
}
```

Paipu switch 调用该函数，不校验模型白名单或模型名分辨率后缀。

- [ ] **Step 4: 增加配置导入回归断言**

在 Paipu staging fixture 中使用不在代码列表中的模型名，断言映射、route target 和 cost rule 原样保留。只改测试，不修改 `e2e/testdata/channel-config-v1.json` 业务数据。

- [ ] **Step 5: 运行测试并提交**

```powershell
gofmt -w relay/video_route_contract.go relay/video_route_contract_test.go service/config_import_stage_test.go
go test ./relay ./service -run 'TestValidateVideoRouteTargetContract|TestConfigImport.*Paipu' -count=1
git add relay/video_route_contract.go relay/video_route_contract_test.go service/config_import_stage_test.go
git diff --cached --check
git commit -m "feat(paipu): validate imported route capabilities"
```

### Task 4: 禁止 Paipu 创建任务自动重试

**Files:**
- Create: `controller/relay_task_retry_internal_test.go`
- Modify: `controller/relay.go`
- Modify: `e2e/paipu_upstream_e2e_test.go`

- [ ] **Step 1: 写 controller 重试策略失败测试**

使用 `package controller` 直接测试未导出的函数：

```go
func TestShouldRetryTaskRelayNeverRetriesPaipuSubmit(t *testing.T) {
	for _, status := range []int{
		http.StatusTooManyRequests,
		http.StatusTemporaryRedirect,
		http.StatusInternalServerError,
		http.StatusBadGateway,
	} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			common.SetContextKey(c, constant.ContextKeyChannelType, constant.ChannelTypePaipu)
			taskErr := service.TaskErrorWrapper(errors.New("upstream uncertain"), "upstream_error", status)
			assert.False(t, shouldRetryTaskRelay(c, constant.ChannelTypePaipu, taskErr, 3))
		})
	}
}
```

增加对照用例：Cangyuan 的 500 在剩余重试次数大于零时仍返回 `true`。

- [ ] **Step 2: 运行测试并确认 RED**

```powershell
go test ./controller -run TestShouldRetryTaskRelay -count=1
```

Expected: FAIL，当前 Paipu 429、307 和 5xx 会进入重试。

- [ ] **Step 3: 实现渠道级禁止重试策略**

在 `shouldRetryTaskRelay` 的 nil 判断之后、所有状态码判断之前加入：

```go
if common.GetContextKeyInt(c, constant.ContextKeyChannelType) == constant.ChannelTypePaipu {
	// Paipu task creation is non-idempotent and must never be submitted twice.
	return false
}
```

保持逻辑内联，不新增单调用者 helper，也不改变其他渠道。

- [ ] **Step 4: 扩展 E2E mock 保护单次提交**

给 `paipuE2EMock` 增加可配置 submit status/body 和受 mutex 保护的 submit 次数。新增 429、500 两个 table case，通过真实 Ark 创建入口调用后断言：

```go
assert.Equal(t, 1, env.mock.submitCount())
assert.Empty(t, env.mock.pollRequests())
```

- [ ] **Step 5: 运行测试并提交**

```powershell
gofmt -w controller/relay.go controller/relay_task_retry_internal_test.go e2e/paipu_upstream_e2e_test.go
go test ./controller -run TestShouldRetryTaskRelay -count=1
go test ./e2e -run 'TestPaipu.*NoRetry' -count=1 -v
git add controller/relay.go controller/relay_task_retry_internal_test.go e2e/paipu_upstream_e2e_test.go
git diff --cached --check
git commit -m "fix(paipu): prevent duplicate task submission"
```

### Task 5: 更新 Ark 多模态生命周期和账务回归测试

**Files:**
- Modify: `e2e/paipu_upstream_e2e_test.go`
- Modify: `relay/relay_task_billing_test.go`
- Modify: `relay/channel/task/newapivideo/response_test.go`

- [ ] **Step 1: 将旧的纯文本 E2E 改为多模态请求**

把 `TestPaipuARKLifecycleAndTextRequestE2E` 改名为 `TestPaipuARKMultimodalLifecycleE2E`，提交文本、图片、视频、音频、8 秒、16:9 和 720p。mock 必须精确断言上游收到：

```json
{
  "model":"imported-paipu-model",
  "prompt":"paipu multimodal acceptance",
  "duration":8,
  "aspect_ratio":"16:9",
  "resolution":"720p",
  "images":["https://8.8.8.8/ref.png"],
  "videos":["https://8.8.4.4/ref.mp4"],
  "audios":["data:audio/wav;base64,UklGRg=="]
}
```

断言上游 JSON 不包含 `ratio` 或 Ark `content`。

- [ ] **Step 2: 用真实协议错误替换“媒体必须失败”的旧用例**

将旧测试替换为 10 图片、frame role 和私网 URL 三个 table case。每个 case 断言 HTTP 400、上游 submit 次数为 0、无新 task row、用户 quota 和账务日志无变化。

- [ ] **Step 3: 增加文档响应 fixture**

在 `response_test.go` 覆盖：

```json
{"id":"paipu-private","task_id":"paipu-private","status":"queued","progress":0}
```

以及：

```json
{"id":"paipu-private","status":"completed","content":{"video_url":"https://assets.example/paipu.mp4"}}
```

任务 ID 一致时通过，冲突时返回 `invalid_response`；完成态解析出 URL。不得新增 Paipu 私有 response parser。

- [ ] **Step 4: 扩展 billing fixture**

在 `relay_task_billing_test.go` 的 Paipu case 中加入一张 `reference_image`，保持请求时长 8 秒和导入上游模型。继续断言 per-request 成功保持预扣、per-duration 使用请求时长、失败退款一次及重复轮询幂等。

- [ ] **Step 5: 运行生命周期测试并提交**

```powershell
gofmt -w e2e/paipu_upstream_e2e_test.go relay/relay_task_billing_test.go relay/channel/task/newapivideo/response_test.go
go test ./e2e -run TestPaipu -count=1 -v
go test ./relay/channel/task/newapivideo -run 'TestPaipu|TestParse.*Task' -count=1
go test ./relay -run 'TestPaipu|TestTaskBilling' -count=1
git add e2e/paipu_upstream_e2e_test.go relay/relay_task_billing_test.go relay/channel/task/newapivideo/response_test.go
git diff --cached --check
git commit -m "test(paipu): cover multimodal Ark lifecycle"
```

### Task 6: 清空前端静态模型目录并保护配置导入

**Files:**
- Modify: `web/tests/channel-type-config.test.ts`
- Modify: `web/src/features/channels/lib/channel-type-config.ts`
- Modify: `web/src/channel-config-converter/__tests__/v1.test.ts`
- Modify: `web/src/i18n/locales/en.json`
- Modify: `web/src/i18n/locales/zh.json`
- Modify: `web/src/i18n/locales/zh-TW.json`
- Modify: `web/src/i18n/locales/fr.json`
- Modify: `web/src/i18n/locales/ru.json`
- Modify: `web/src/i18n/locales/ja.json`
- Modify: `web/src/i18n/locales/vi.json`

- [ ] **Step 1: 写空目录和导入提示失败测试**

```ts
expect(getChannelTypeConfig(206)).toMatchObject({
  id: 206,
  name: 'Paipu',
  icon: 'NewAPI',
  defaultBaseUrl: 'https://api.paipu.net',
  supportedModels: [],
})
expect(getChannelModelOptions(206, [], [])).toEqual([])
expect(getChannelTypeHints(206).models).toBe(
  'Import Paipu models from channel configuration or add verified mappings manually'
)
```

保留 task-only、默认 disabled、禁止 generic channel test 和自定义代理 URL 不被覆盖的断言。

- [ ] **Step 2: 增加 converter 不过滤导入数据的断言**

在 `v1.test.ts` 中断言 `CH-PAIPU` 仍为 206，现有 Paipu target 的 `upstream_model`、`reference_limits` 以及对应 cost draft 的 `cost_mode` 原样保留。不要编辑 `e2e/testdata/channel-config-v1.json`。

- [ ] **Step 3: 运行测试并确认 RED**

```powershell
Set-Location web
bun test tests/channel-type-config.test.ts src/channel-config-converter/__tests__/v1.test.ts
```

Expected: FAIL，因为 Paipu 仍声明 24 个静态模型和旧提示。

- [ ] **Step 4: 清空目录并更新提示**

```ts
supportedModels: [],
hints: {
  baseUrl: 'Default: https://api.paipu.net',
  key: 'Enter the raw API key issued by Paipu',
  models:
    'Import Paipu models from channel configuration or add verified mappings manually',
},
```

不要改变类型 ID、Base URL、task-only 集合和默认 disabled 门禁。

- [ ] **Step 5: 补齐七语言翻译**

新增英文 key `Import Paipu models from channel configuration or add verified mappings manually`，翻译如下：

| Locale | 文案 |
| --- | --- |
| en | Import Paipu models from channel configuration or add verified mappings manually |
| zh | 从渠道配置导入 Paipu 模型，或手动添加已验证的模型映射 |
| zh-TW | 從渠道設定匯入 Paipu 模型，或手動新增已驗證的模型對應 |
| fr | Importez les modèles Paipu depuis la configuration des canaux ou ajoutez manuellement des mappages vérifiés |
| ru | Импортируйте модели Paipu из конфигурации каналов или добавьте проверенные сопоставления вручную |
| ja | チャネル設定から Paipu モデルをインポートするか、検証済みのモデルマッピングを手動で追加してください |
| vi | Nhập các mô hình Paipu từ cấu hình kênh hoặc thêm thủ công các ánh xạ đã xác minh |

只有在 `rg` 确认无调用者后，才删除旧的 24 模型提示 key。

- [ ] **Step 6: 运行前端验证并提交**

```powershell
Set-Location web
bun run i18n:sync
bun test tests/channel-type-config.test.ts src/channel-config-converter/__tests__/v1.test.ts
bun run typecheck
bunx oxlint src/features/channels/lib/channel-type-config.ts tests/channel-type-config.test.ts src/channel-config-converter/__tests__/v1.test.ts
bunx oxfmt --check src/features/channels/lib/channel-type-config.ts tests/channel-type-config.test.ts src/channel-config-converter/__tests__/v1.test.ts
Set-Location ..
git add web/src/features/channels/lib/channel-type-config.ts web/tests/channel-type-config.test.ts web/src/channel-config-converter/__tests__/v1.test.ts web/src/i18n/locales/en.json web/src/i18n/locales/zh.json web/src/i18n/locales/zh-TW.json web/src/i18n/locales/fr.json web/src/i18n/locales/ru.json web/src/i18n/locales/ja.json web/src/i18n/locales/vi.json
git diff --cached --check
git commit -m "feat(web): use imported Paipu model catalog"
```

### Task 7: 更新旧文档权威关系并执行完成验证

**Files:**
- Modify: `docs/superpowers/specs/2026-07-26-paipu-channel-design.md`
- Modify: `docs/superpowers/plans/2026-07-26-paipu-channel.md`
- Create after real success: `docs/superpowers/reports/2026-08-02-paipu-protocol-update-acceptance.md`

- [ ] **Step 1: 在旧设计和计划顶部增加替代说明**

```markdown
> **协议更新说明：** Paipu 的请求编码、动态模型目录和创建重试语义已由 `2026-08-02-paipu-protocol-update-design.md` 与 `2026-08-02-paipu-protocol-update.md` 替代。本文仍保留最初接入背景和共享 Ark 生命周期设计，但旧的纯文本请求方言、24 模型静态目录及自动重试假设不再有效。
```

- [ ] **Step 2: 运行后端完整验证**

```powershell
go test ./... -count=1
go build ./...
```

Expected: 两条命令 exit code 0。

- [ ] **Step 3: 运行前端构建验证**

```powershell
Set-Location web
bun run typecheck
bun run build
Set-Location ..
```

Expected: exit code 0。全仓 `bun test` 和全仓 lint 若仍存在既有失败，只记录准确结果，不修改无关文件。

- [ ] **Step 4: 运行安全和工作树检查**

```powershell
rg -n 'api.?key|authorization|bearer|upstream_task_id|provider-secret' relay/channel/task/newapivideo/paipu_request* e2e/paipu_upstream_e2e_test.go docs/superpowers/specs/2026-08-02-paipu-protocol-update-design.md
git diff --check
git status --short
```

确认命中内容只包含固定测试值或安全说明，没有真实 Key、完整签名 URL、用户素材或私有任务 ID 泄漏。

- [ ] **Step 5: 使用真实凭据执行发布门禁**

只从本机读取 `PAIPU_API_KEY` 和可选 `PAIPU_BASE_URL`。通过网关 Ark 接口提交一个最低成本文本+图片请求和一个已验证支持的视频+音频请求，确认上游只收到一次 submit、使用导入模型、公共 ID 隔离、结果 MP4 可读及失败退款一次。

未提供真实 Key 时保持 Paipu disabled，不生成虚假验收报告。报告不得记录 Key、完整签名 URL 或私有任务 ID。

- [ ] **Step 6: 提交旧文档替代说明**

```powershell
git add docs/superpowers/specs/2026-07-26-paipu-channel-design.md docs/superpowers/plans/2026-07-26-paipu-channel.md
git diff --cached --check
git commit -m "docs(paipu): supersede legacy protocol assumptions"
```

真实验收成功后将报告单独提交：

```powershell
git add docs/superpowers/reports/2026-08-02-paipu-protocol-update-acceptance.md
git diff --cached --check
git commit -m "docs(paipu): record protocol acceptance"
```

- [ ] **Step 7: fast-forward 合并到本地 ysr 并推送 fork**

确认 Paipu worktree 干净且所有提交都基于最新 `ysr`：

```powershell
git -C .worktrees/seedance-paipu status --short --branch
git merge-base --is-ancestor ysr feat/seedance-paipu
```

然后在主工作树执行：

```powershell
git merge --ff-only feat/seedance-paipu
git push fork ysr
git rev-parse HEAD
git rev-parse fork/ysr
```

Expected: 两个 hash 完全一致；用户已有未提交文件仍保持原状。

## 计划自检

| 设计要求 | 实施任务 |
| --- | --- |
| Ark SDK 零代码多模态调用 | Task 1、Task 2、Task 5 |
| API 协议与模型目录解耦 | Task 2、Task 3、Task 6 |
| 导入模型和成本保持权威 | Task 3、Task 5、Task 6 |
| `aspect_ratio` 和媒体数组 | Task 1、Task 2、Task 5 |
| 9/3/3 协议安全上限 | Task 1、Task 2、Task 3 |
| 创建任务永不自动重试 | Task 4 |
| 公共/私有任务 ID 隔离 | Task 5 |
| 真实验收前默认 disabled | Task 6、Task 7 |
| 旧文档不再误导实施者 | Task 7 |

计划不修改受保护的项目身份信息，不写入真实凭据，不根据 HTML 文档硬编码 Paipu 模型或价格。
