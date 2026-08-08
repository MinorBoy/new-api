# MegaByAI Seedance 渠道实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 新增独立 MegaByAI Seedance 任务渠道，使现有 Ark SDK 创建、单查和列表调用可使用纯文本及图片/视频/音频参考，并建立后续 `/v1/videos` 上游复用的公共方言。

**Architecture:** 在 Lucen 计划建立的 `newapivideo` profile 机制上增加端点、请求方言和内容类型配置。MegaByAI profile 把 Ark `content` 转为引用数组；共享任务核心继续负责公开任务 ID、轮询、Ark 响应、计费和退款。

**Tech Stack:** Go 1.22+、Gin、GORM v2、testify、React 19、TypeScript、Base UI、i18next、Bun。

**Design:** `docs/superpowers/specs/2026-07-26-megabyai-channel-design.md`

**Ark Contract:** 用户代码不变，统一覆盖 `POST /api/v3/contents/generations/tasks`、`GET /api/v3/contents/generations/tasks/{id}` 和 `GET /api/v3/contents/generations/tasks`。

**Prerequisite:** 先完整执行 `docs/superpowers/plans/2026-07-26-lucen-channel.md`。该计划会创建 `relay/channel/task/newapivideo/profile.go` 并占用渠道类型 62；本计划从类型 63 继续。

---

## 文件结构

### 新增文件

- `relay/channel/task/newapivideo/mega_request.go`：MegaByAI Ark 请求校验与引用数组 JSON 编码。
- `relay/channel/task/newapivideo/mega_request_test.go`：请求字段、媒体角色和参数边界测试。
- `e2e/megabyai_upstream_e2e_test.go`：Ark 到 mock MegaByAI 的完整任务生命周期。

### 修改文件

- `relay/channel/task/newapivideo/profile.go`：端点、内容类型和 MegaByAI profile。
- `relay/channel/task/newapivideo/adaptor.go`：按 profile 构造创建/查询 URL、请求头和请求体。
- `relay/channel/task/newapivideo/dto.go`、`response.go`、`response_test.go`：解析 `/v1/videos` 直接响应的 URL 变体。
- `service/video_metadata_client.go`、`service/video_metadata_client_test.go`：供渠道硬边界复用的参考视频总时长入口。
- `service/reference_audio_duration.go`、`service/reference_audio_duration_test.go`：SSRF-safe、限流限大小的参考音频总时长解析。
- `constant/channel.go`、`constant/channel_test.go`：注册类型 63。
- `relay/relay_adaptor.go`、`relay/seedance_task.go`、`relay/relay_task.go`：注册提交、轮询、Ark 单查和列表。
- `relay/relay_task_seedance_test.go`、`relay/cost_accounting_adaptor_test.go`：平台与成本能力回归。
- `controller/channel-test.go`、`controller/channel_test_internal_test.go`：禁用通用聊天渠道测试。
- `web/src/features/channels/constants.ts`、`web/src/features/channels/lib/channel-type-config.ts`、`web/src/features/channels/lib/channel-utils.ts`：管理端渠道定义。
- `web/tests/channel-type-config.test.ts`：管理端配置测试。
- `web/src/i18n/locales/{en,zh,zh-TW,fr,ru,ja,vi}.json`：渠道表单文案。

---

### Task 1: 扩展共享 profile 的端点和请求方言

**Files:**
- Modify: `relay/channel/task/newapivideo/profile.go`
- Modify: `relay/channel/task/newapivideo/adaptor.go`
- Modify: `relay/channel/task/newapivideo/native.go`
- Create: `relay/channel/task/newapivideo/mega_request.go`
- Create: `relay/channel/task/newapivideo/mega_request_test.go`
- Modify: `service/video_metadata_client.go`
- Modify: `service/video_metadata_client_test.go`
- Create: `service/reference_audio_duration.go`
- Create: `service/reference_audio_duration_test.go`

- [ ] **Step 1: 写 MegaByAI profile 和请求翻译失败测试**

创建 `mega_request_test.go`，覆盖以下确定性用例：

```go
func TestMegaByAIProfile(t *testing.T) {
	adaptor := NewMegaByAITaskAdaptor()
	assert.Equal(t, "MegaByAI", adaptor.GetChannelName())
	assert.Equal(t, []string{"videos-standard", "videos-fast", "videos-mini"}, adaptor.GetModelList())
	assert.Equal(t, "/v1/videos", adaptor.activeProfile().submitPath)
	assert.Equal(t, "/v1/videos/{task_id}", adaptor.activeProfile().pollPath)
}

func TestBuildMegaByAIRequest(t *testing.T) {
	request, err := parseARKRequest([]byte(`{
		"model":"doubao-seedance-2-0-260128",
		"content":[
			{"type":"text","text":"镜头推进"},
			{"type":"image_url","role":"reference_image","image_url":{"url":"https://x/ref.jpg"}},
			{"type":"video_url","role":"reference_video","video_url":{"url":"https://x/ref.mp4"}},
			{"type":"audio_url","role":"reference_audio","audio_url":{"url":"https://x/ref.mp3"}}
		],
		"duration":8,"ratio":"16:9","resolution":"720p"
	}`), megaByAIProtocolProfile())
	require.NoError(t, err)

	body, err := buildMegaByAIRequest(request, "videos-mini")
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"model":"videos-mini","prompt":"镜头推进","duration":8,
		"ratio":"16:9","resolution":"720p",
		"referenceImages":["https://x/ref.jpg"],
		"referenceVideos":["https://x/ref.mp4"],
		"referenceAudios":["https://x/ref.mp3"]
	}`, string(body))
}
```

增加表驱动测试，精确断言：时长 3/16、比例 `4:3`、分辨率 `1440p`、`last_frame`、非 HTTP URL、纯音频、超过 9 图/3 视频/3 音频均返回对应 `arkRequestError`；单张无 role 或 `first_frame` 图片进入 `referenceImages`。另覆盖 `1080p`、`4k` 原样转发。视频 metadata 为 9000+6000 ms、音频 resolver 为 7000+8000 ms 时通过；任一类别为 15001 ms 时返回 `InvalidParameter.content`。

每个非法用例必须通过 `NewMegaByAITaskAdaptor().ValidateRequestAndSetAction(...)` 进入，而不是只直接调用编码器；断言返回 HTTP 400 和准确的 Ark 错误码，且尚未调用 `BuildRequestBody`。再保留一个直接调用 `buildMegaByAIRequest` 的防御性测试，证明内部调用方也不能绕过相同校验。

- [ ] **Step 2: 运行测试并确认失败**

```powershell
go test ./relay/channel/task/newapivideo ./service -run 'TestMegaByAI|TestBuildMega|TestResolveReferenceVideoDuration|TestReferenceAudioDuration' -count=1
```

Expected: FAIL，`NewMegaByAITaskAdaptor`、profile 字段和构造函数尚不存在。

- [ ] **Step 3: 给 profile 增加协议字段**

在 `profile.go` 定义请求方言并扩展结构。保留 Lucen 计划已有字段：

```go
type videoRequestDialect string

const (
	videoRequestDialectNewAPIGenerations videoRequestDialect = "newapi_generations"
	videoRequestDialectMegaReferenceArrays videoRequestDialect = "mega_reference_arrays"
)

type protocolProfile struct {
	channelName                        string
	modelList                          []string
	ignoreUnsupportedOptionalARKFields bool
	allowEmbeddedMedia                 bool
	useRoutingDurationDefault          bool
	submitPath                         string
	pollPath                           string
	contentType                        string
	requestDialect                     videoRequestDialect
}

func (p protocolProfile) normalized() protocolProfile {
	if p.submitPath == "" {
		p.submitPath = "/v1/video/generations"
	}
	if p.pollPath == "" {
		p.pollPath = "/v1/video/generations/{task_id}"
	}
	if p.contentType == "" {
		p.contentType = "application/json"
	}
	if p.requestDialect == "" {
		p.requestDialect = videoRequestDialectNewAPIGenerations
	}
	return p
}

func megaByAIProtocolProfile() protocolProfile {
	return protocolProfile{
		channelName: "MegaByAI",
		modelList: []string{"videos-standard", "videos-fast", "videos-mini"},
		submitPath: "/v1/videos",
		pollPath: "/v1/videos/{task_id}",
		contentType: "application/json",
		requestDialect: videoRequestDialectMegaReferenceArrays,
	}
}

func NewMegaByAITaskAdaptor() *TaskAdaptor {
	return &TaskAdaptor{profile: megaByAIProtocolProfile()}
}
```

让 `activeProfile()` 返回 `profile.normalized()`。不要改变 generic 和 Lucen 的默认行为。

- [ ] **Step 4: 实现 MegaByAI JSON 编码器**

创建 `mega_request.go`：

```go
package newapivideo

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
)

type megaByAIRequest struct {
	Model           string   `json:"model"`
	Prompt          string   `json:"prompt"`
	Duration        *int     `json:"duration,omitempty"`
	Ratio           string   `json:"ratio,omitempty"`
	Resolution      string   `json:"resolution,omitempty"`
	ReferenceImages []string `json:"referenceImages,omitempty"`
	ReferenceVideos []string `json:"referenceVideos,omitempty"`
	ReferenceAudios []string `json:"referenceAudios,omitempty"`
}

func validateMegaByAIRequest(request arkRequest) error {
	if request.Duration != nil && (*request.Duration < 4 || *request.Duration > 15) {
		return &arkRequestError{Code: "InvalidParameter.duration", Message: "MegaByAI duration must be between 4 and 15"}
	}
	if request.Ratio != "" && request.Ratio != "16:9" && request.Ratio != "9:16" && request.Ratio != "1:1" {
		return &arkRequestError{Code: "InvalidParameter.ratio", Message: "MegaByAI ratio is unsupported"}
	}
	if request.Resolution != "" && request.Resolution != "480p" && request.Resolution != "720p" && request.Resolution != "1080p" && request.Resolution != "4k" {
		return &arkRequestError{Code: "InvalidParameter.resolution", Message: "MegaByAI resolution is unsupported"}
	}
	for _, item := range request.Content {
		if item.Type == "image_url" && strings.TrimSpace(item.Role) == "last_frame" {
			return &arkRequestError{Code: "InvalidParameter.content", Message: "MegaByAI does not support last_frame"}
		}
	}
	return nil
}

func buildMegaByAIRequest(request arkRequest, upstreamModel string) ([]byte, error) {
	if err := validateMegaByAIRequest(request); err != nil {
		return nil, err
	}
	out := megaByAIRequest{Model: upstreamModel, Prompt: arkPrompt(request.Content), Duration: request.Duration, Ratio: request.Ratio, Resolution: request.Resolution}
	for _, item := range request.Content {
		switch item.Type {
		case "image_url": out.ReferenceImages = append(out.ReferenceImages, item.ImageURL.URL)
		case "video_url": out.ReferenceVideos = append(out.ReferenceVideos, item.VideoURL.URL)
		case "audio_url": out.ReferenceAudios = append(out.ReferenceAudios, item.AudioURL.URL)
		}
	}
	return common.Marshal(out)
}
```

`parseARKRequest` 已负责 HTTP URL、角色、计数、纯音频和互斥场景校验。Mega 测试必须证明这些校验实际使用 `megaByAIProtocolProfile()`，不能绕过公共验证。

- [ ] **Step 5: 建立视频和音频总时长解析入口**

在 `service/video_metadata_client.go` 增加复用现有 request-level metadata state 的薄入口：

```go
func ResolveReferenceVideoDurationMS(ctx context.Context, urls []string) (int64, error) {
	state := NewProfitRoutingRequestState(currentVideoMetadataClient(), urls)
	result, err := state.Metadata(ctx)
	if err != nil { return 0, err }
	return result.TotalDurationMS, nil
}
```

单测固定两个 URL 返回 9000 和 6000 ms，断言总和 15000；invalid media 保留 `VideoMetadataInvalidMedia`，不可用保留 `VideoMetadataUnavailable`，错误文本和捕获日志都不含签名 URL 查询串。

创建 `service/reference_audio_duration.go`，定义可注入的稳定接口：

```go
type ReferenceAudioDurationResolver interface {
	ResolveMS(ctx context.Context, urls []string) (int64, error)
}

func ResolveReferenceAudioDurationMS(ctx context.Context, urls []string) (int64, error)
func SetReferenceAudioDurationResolver(resolver ReferenceAudioDurationResolver)
```

生产 resolver 使用 `newProtectedFetchHTTPClient()`，共享 30 秒 deadline，最多并发 3 个 URL；每个响应必须是 2xx，累计读取上限 50 MiB，超限或无法识别 MP3/WAV/FLAC/M4A/OGG/Opus/AAC/AIFF 时返回不包含 URL 的 `ReferenceAudioInvalidMedia`。内容写入 `os.CreateTemp` 创建的单请求临时文件，用响应 Content-Type、Content-Disposition 或 URL path 的安全扩展名调用 `common.GetAudioDuration`，每条路径都关闭并删除临时文件。网络、超时和本机资源错误归类为 `ReferenceAudioMetadataUnavailable`。

`common.GetAudioDuration` 的 float 结果先拒绝 NaN、Inf 和非正值，再用 `decimal.NewFromFloat(seconds).Mul(decimal.NewFromInt(1000))` 与 15000 比较；一旦单项或累计超过上限直接返回 15001 sentinel，只有已证明位于 `[1,15000]` 的值才调用 `IntPart`。这样没有未界定的 float/int 转换，也不会让恶意媒体头造成算术溢出。

测试 helper 在 `t.TempDir()` 中生成确定性的 PCM16 WAV 文件，不向仓库提交二进制 fixture；覆盖 15000 ms、15001 ms、累计 50 MiB 上限、重定向后的 SSRF 拒绝、超时、未知格式、临时文件清理和错误/日志隐私。全局 setter 由 mutex 保护，并在测试 cleanup 中恢复默认 resolver；这些 setter 测试不调用 `t.Parallel()`。

- [ ] **Step 6: 在预扣前校验 profile 和媒体总时长**

修改 `adaptor.go`：

`ValidateRequestAndSetAction` 的 Ark 分支先调用 profile-aware `validateARKRequest` 解析并保存 state；成功后读取 `state.ARK`，按 dialect 调用 `validateMegaByAIRequest`。随后分别收集 `reference_video` 和 `reference_audio` 的 HTTP(S) URL，调用 Step 5 的两个入口；每类总时长大于 15000 ms 返回 HTTP 400 `InvalidParameter.content`，invalid media 返回 HTTP 400，metadata unavailable 返回 HTTP 503 `reference_media_metadata_unavailable`。用现有 `arkRequestError` 转换逻辑保留其他错误码。该方法由 `RelayTaskSubmit` 在模型价格计算和 `PreConsumeBilling` 之前调用，因此不得把这一步延迟到 `BuildRequestBody`。

媒体 resolver 不在编码器中重复下载；request state 增加 `ProviderValidationComplete bool`，只有上述校验全部成功后设为 true。`BuildRequestBody` 对 Mega dialect 要求该标记为 true，否则返回内部错误，避免内部调用绕过时长校验。`buildMegaByAIRequest` 中的纯字段校验仍作为防御性复验。E2E 非法请求测试必须额外断言上游请求数、任务记录数和用户 quota 变化均为 0。

- [ ] **Step 7: 按 profile 构造提交和查询**

修改 `adaptor.go`：

```go
func (a *TaskAdaptor) BuildRequestURL(_ *relaycommon.RelayInfo) (string, error) {
	return a.baseURL + a.activeProfile().submitPath, nil
}

func (a *TaskAdaptor) BuildRequestHeader(_ *gin.Context, req *http.Request, _ *relaycommon.RelayInfo) error {
	req.Header.Set("Content-Type", a.activeProfile().contentType)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	return nil
}
```

在 `BuildRequestBody` 的 Ark 分支读取 request state 后按 dialect 分派：

```go
switch a.activeProfile().requestDialect {
case videoRequestDialectMegaReferenceArrays:
	body, err = buildMegaByAIRequest(*state.ARK, modelName)
default:
	body, err = buildARKRequestBody(c, info, a.activeProfile())
}
```

若现有 `buildARKRequestBody` 仍由自身读取 state，则只向它增加 profile 参数，不重复解析 request body。

修改 `FetchTask` 使用安全模板替换：

```go
path := strings.Replace(a.activeProfile().pollPath, "{task_id}", url.PathEscape(taskID), 1)
requestURL := strings.TrimRight(baseURL, "/") + path
```

当模板不含 `{task_id}` 时返回错误，不能拼出错误 URL。

- [ ] **Step 8: 格式化、测试并提交**

```powershell
gofmt -w relay/channel/task/newapivideo/profile.go relay/channel/task/newapivideo/adaptor.go relay/channel/task/newapivideo/native.go relay/channel/task/newapivideo/mega_request.go relay/channel/task/newapivideo/mega_request_test.go service/video_metadata_client.go service/video_metadata_client_test.go service/reference_audio_duration.go service/reference_audio_duration_test.go
go test ./relay/channel/task/newapivideo ./service -run 'TestMegaByAI|TestBuildMega|TestLucen|TestARK|TestResolveReferenceVideoDuration|TestReferenceAudioDuration' -count=1
git add relay/channel/task/newapivideo/profile.go relay/channel/task/newapivideo/adaptor.go relay/channel/task/newapivideo/native.go relay/channel/task/newapivideo/mega_request.go relay/channel/task/newapivideo/mega_request_test.go service/video_metadata_client.go service/video_metadata_client_test.go service/reference_audio_duration.go service/reference_audio_duration_test.go
git commit -m "feat(video): add MegaByAI request profile"
```

Expected: PASS，且 Lucen/generic profile 回归保持通过。

---

### Task 2: 扩展直接任务响应投影

**Files:**
- Modify: `relay/channel/task/newapivideo/dto.go`
- Modify: `relay/channel/task/newapivideo/response.go`
- Modify: `relay/channel/task/newapivideo/response_test.go`

- [ ] **Step 1: 写 URL 位置和错误失败测试**

增加表驱动测试：

```go
func TestParseMegaByAIDirectTaskURLPrecedence(t *testing.T) {
	tests := []struct{ body, want string }{
		{`{"status":"completed","video_url":"https://x/video.mp4","url":"https://x/url.mp4"}`, "https://x/video.mp4"},
		{`{"status":"completed","url":"https://x/url.mp4"}`, "https://x/url.mp4"},
		{`{"status":"completed","metadata":{"content_url":"https://x/content.mp4"}}`, "https://x/content.mp4"},
		{`{"status":"completed","metadata":{"local_url":"https://x/local.mp4"}}`, "https://x/local.mp4"},
	}
	for _, tt := range tests {
		result, err := (&TaskAdaptor{}).ParseTaskResult([]byte(tt.body))
		require.NoError(t, err)
		assert.Equal(t, tt.want, result.Url)
	}
}
```

再断言 `completed` 且上述地址均为空时返回错误，`failed/error.code/error.message` 保持原值。

- [ ] **Step 2: 运行测试并确认失败**

```powershell
go test ./relay/channel/task/newapivideo -run 'TestParseMegaByAI' -count=1
```

Expected: FAIL，当前 `directTask` 不读取顶层 `video_url/url` 和 `metadata.content_url/local_url`。

- [ ] **Step 3: 扩展 DTO 和 URL 选择**

在 `dto.go` 扩展 `directTask`：

```go
type directTask struct {
	ID          string `json:"id"`
	TaskID      string `json:"task_id"`
	Status      string `json:"status"`
	Progress    int    `json:"progress"`
	CreatedAt   int64  `json:"created_at"`
	CompletedAt int64  `json:"completed_at"`
	URL         string `json:"url"`
	VideoURL    string `json:"video_url"`
	ResultURL   string `json:"result_url"`
	Metadata    *struct {
		URL        string `json:"url,omitempty"`
		ContentURL string `json:"content_url,omitempty"`
		LocalURL   string `json:"local_url,omitempty"`
	} `json:"metadata,omitempty"`
	Content *arkVideoContent `json:"content,omitempty"`
	Data    json.RawMessage  `json:"data,omitempty"`
	Usage   *tokenUsage      `json:"usage,omitempty"`
	Error   *upstreamError   `json:"error,omitempty"`
}
```

在 `response.go` 增加一个可直接测试的纯函数，按设计顺序读取地址，并兼容 `data` 对象和数组：

```go
func directTaskVideoURL(task directTask) string {
	for _, value := range []string{task.VideoURL, task.URL, task.ResultURL} {
		if strings.TrimSpace(value) != "" { return value }
	}
	if task.Metadata != nil {
		for _, value := range []string{task.Metadata.URL, task.Metadata.ContentURL, task.Metadata.LocalURL} {
			if strings.TrimSpace(value) != "" { return value }
		}
	}
	if task.Content != nil && strings.TrimSpace(task.Content.VideoURL) != "" { return task.Content.VideoURL }
	var object struct{ URL string `json:"url"` }
	if common.Unmarshal(task.Data, &object) == nil && strings.TrimSpace(object.URL) != "" { return object.URL }
	var list []struct{ URL string `json:"url"` }
	if common.Unmarshal(task.Data, &list) == nil && len(list) > 0 { return list[0].URL }
	return ""
}
```

仅在 `len(task.Data) > 0 && string(task.Data) != "null"` 时解析，避免空 `RawMessage` 错误。`parseDirectTask` 使用该函数。

- [ ] **Step 4: 运行完整包测试并提交**

```powershell
gofmt -w relay/channel/task/newapivideo/dto.go relay/channel/task/newapivideo/response.go relay/channel/task/newapivideo/response_test.go
go test ./relay/channel/task/newapivideo -count=1
git add relay/channel/task/newapivideo/dto.go relay/channel/task/newapivideo/response.go relay/channel/task/newapivideo/response_test.go
git commit -m "feat(video): parse direct upstream video results"
```

Expected: PASS。

---

### Task 3: 注册 MegaByAI task-only 渠道

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

- [ ] **Step 1: 写渠道注册失败测试**

在 `constant/channel_test.go` 增加：

```go
func TestMegaByAIChannelConstants(t *testing.T) {
	require.Equal(t, 63, constant.ChannelTypeMegaByAI)
	require.Equal(t, 64, constant.ChannelTypeDummy)
	require.Equal(t, "https://newapi.megabyai.cc", constant.ChannelBaseURLs[constant.ChannelTypeMegaByAI])
	require.Equal(t, "MegaByAI", constant.GetChannelTypeName(constant.ChannelTypeMegaByAI))
	_, success := common.ChannelType2APIType(constant.ChannelTypeMegaByAI)
	require.False(t, success)
}
```

在 relay 测试断言 `GetTaskAdaptor("63")` 返回 `NewMegaByAITaskAdaptor` 的能力：名称为 MegaByAI、实现 `ArkVideoTaskConverter` 和 `TaskCostAccountingAdaptor`。

- [ ] **Step 2: 运行测试并确认失败**

```powershell
go test ./constant ./relay ./controller -run 'TestMegaByAI|TestSupportsGenericChannelTest' -count=1
```

Expected: FAIL，渠道常量和注册不存在。

- [ ] **Step 3: 注册类型、默认地址和 adaptor**

在 Lucen 类型 62 后追加：

```go
ChannelTypeMegaByAI = 63 // MegaByAI Seedance via shared video task profiles
ChannelTypeDummy    = 64
```

在 `ChannelBaseURLs` 索引 63 追加 `https://newapi.megabyai.cc`，在 `ChannelTypeNames` 添加 `MegaByAI`。不要给 `ChannelType2APIType` 增加映射，task-only 渠道只能走 `GetTaskAdaptor`。

在 `GetTaskAdaptor` 增加：

```go
case constant.ChannelTypeMegaByAI:
	return newapivideo.NewMegaByAITaskAdaptor()
```

将 MegaByAI 加入 `isSeedanceTaskPlatform`、`seedanceTaskPlatformValues`、`RelayTaskFetch` 的 Ark converter 分支和 `supportsGenericChannelTest` 的禁止列表。注册成本能力源为 `ValidatedRequest`、`UpstreamActual`、`UpstreamUsage`，但没有 usage 时不得伪造值。

- [ ] **Step 4: 覆盖公开 ID 和列表平台**

在 `relay_task_seedance_test.go` 增加 MegaByAI 任务 fixture：`TaskID=task_public`、`PrivateData.UpstreamTaskID=videos-mini_secret`、上游 `Data` 带相同私有 ID。单查和列表必须只包含 `task_public`，并返回客户端原始模型。

- [ ] **Step 5: 格式化、测试并提交**

```powershell
gofmt -w constant/channel.go constant/channel_test.go relay/relay_adaptor.go relay/seedance_task.go relay/relay_task.go relay/relay_task_seedance_test.go relay/cost_accounting_adaptor_test.go controller/channel-test.go controller/channel_test_internal_test.go
go test ./constant ./relay ./controller -run 'TestMegaByAI|TestSeedanceTask|TestSupportsGenericChannelTest|TestTaskCostAccounting' -count=1
git add constant/channel.go constant/channel_test.go relay/relay_adaptor.go relay/seedance_task.go relay/relay_task.go relay/relay_task_seedance_test.go relay/cost_accounting_adaptor_test.go controller/channel-test.go controller/channel_test_internal_test.go
git commit -m "feat(megabyai): register Seedance task channel"
```

Expected: PASS。

---

### Task 4: 增加 MegaByAI 管理端配置

**Files:**
- Modify: `web/src/features/channels/constants.ts`
- Modify: `web/src/features/channels/lib/channel-type-config.ts`
- Modify: `web/src/features/channels/lib/channel-utils.ts`
- Modify: `web/tests/channel-type-config.test.ts`
- Modify: `web/src/i18n/locales/en.json`
- Modify: `web/src/i18n/locales/zh.json`
- Modify: `web/src/i18n/locales/zh-TW.json`
- Modify: `web/src/i18n/locales/fr.json`
- Modify: `web/src/i18n/locales/ru.json`
- Modify: `web/src/i18n/locales/ja.json`
- Modify: `web/src/i18n/locales/vi.json`

- [ ] **Step 1: 写前端配置失败测试**

```ts
describe('MegaByAI channel configuration', () => {
  test('registers task-only type 63', () => {
    expect(CHANNEL_TYPES[63]).toBe('MegaByAI')
    expect(TASK_ONLY_CHANNEL_TYPES.has(63)).toBe(true)
    expect(GENERIC_CHANNEL_TEST_UNSUPPORTED_TYPES.has(63)).toBe(true)
    expect(MODEL_FETCHABLE_TYPES.has(63)).toBe(false)
  })

  test('provides defaults and models', () => {
    expect(getChannelTypeConfig(63)).toMatchObject({
      defaultBaseUrl: 'https://newapi.megabyai.cc',
      supportedModels: ['videos-standard', 'videos-fast', 'videos-mini'],
    })
  })
})
```

- [ ] **Step 2: 运行测试并确认失败**

```powershell
Set-Location web
bun test tests/channel-type-config.test.ts
```

Expected: FAIL，类型 63 尚未定义。

- [ ] **Step 3: 添加类型、配置和图标**

在 `CHANNEL_TYPES`、显示顺序、`GENERIC_CHANNEL_TEST_UNSUPPORTED_TYPES`、`TASK_ONLY_CHANNEL_TYPES`、`TYPE_TO_KEY_PROMPT`、`CHANNEL_TYPE_WARNINGS` 添加类型 63。

在 `CHANNEL_TYPE_CONFIGS` 添加：

```ts
63: {
  id: 63,
  name: CHANNEL_TYPES[63],
  icon: 'NewAPI',
  defaultBaseUrl: 'https://newapi.megabyai.cc',
  supportedModels: ['videos-standard', 'videos-fast', 'videos-mini'],
  hints: {
    baseUrl: 'Default: https://newapi.megabyai.cc',
    key: 'Enter the raw API key issued by MegaByAI',
    models: 'Supported upstream models: videos-standard, videos-fast, videos-mini',
  },
},
```

将 63 加入 `MANAGED_DEFAULT_BASE_URL_TYPES`。图标复用现有 `NewAPI`，不新增图片资产。

- [ ] **Step 4: 添加七种语言文案**

至少添加并实际翻译以下 key：`MegaByAI is task-only. Call it through the Ark /api/v3 task API.`、`Enter the raw API key issued by MegaByAI`、模型提示和默认地址提示。简体中文分别使用“MegaByAI 仅支持任务接口，请通过 Ark /api/v3 任务 API 调用”和“输入 MegaByAI 签发的原始 API Key”；其他语言不得保留英文占位。

- [ ] **Step 5: 同步 i18n、测试并提交**

```powershell
Set-Location web
bun run i18n:sync
bun test tests/channel-type-config.test.ts
bun run typecheck
Set-Location ..
git add web/src/features/channels/constants.ts web/src/features/channels/lib/channel-type-config.ts web/src/features/channels/lib/channel-utils.ts web/tests/channel-type-config.test.ts web/src/i18n/locales/en.json web/src/i18n/locales/zh.json web/src/i18n/locales/zh-TW.json web/src/i18n/locales/fr.json web/src/i18n/locales/ru.json web/src/i18n/locales/ja.json web/src/i18n/locales/vi.json
git commit -m "feat(web): add MegaByAI channel configuration"
```

Expected: 所有命令退出码 0。不要暂存与本计划无关的翻译报告。

---

### Task 5: 验证 Ark 生命周期、计费和退款

**Files:**
- Create: `e2e/megabyai_upstream_e2e_test.go`
- Modify: `relay/relay_task_billing_test.go`

- [ ] **Step 1: 写 mock 上游 E2E**

测试服务器必须精确断言：

- `POST /v1/videos` 使用 Bearer Key 和 JSON；模型已映射为 `videos-mini`。
- Ark 图片/视频/音频分别进入引用数组。
- 视频和音频 metadata 各自 15000 ms 时允许提交；任一变为 15001 ms 时在预扣前返回 400，mock 上游调用数、任务数和 quota 变化均为 0；resolver 不可用时返回 503。
- 创建返回 `videos-mini_private`，客户端只得到 `task_public`。
- 第一次查询 `in_progress`，第二次查询 `completed` 且 `metadata.content_url` 非空。
- Ark 单查和列表都返回 `succeeded/content.video_url`，不包含私有 ID。

再增加失败场景，查询返回：

```json
{"task_id":"videos-mini_private","status":"failed","progress":100,"error":{"code":"unsupported_material","message":"素材格式不支持"}}
```

断言 Ark `error.code` 保持 `unsupported_material`，用户、Token 和渠道账本按公共任务退款规则恢复。

- [ ] **Step 2: 增加按时长计费平台 fixture**

把现有 `relay_task_billing_test.go` 的 NewAPIVideo/Lucen 固定时长用例扩展到 `ChannelTypeMegaByAI`。输入 duration=8，断言请求计费快照冻结 8 秒，成功保持合法结算，失败全额退款，`OtherRatios` 不重复加入 `seconds`。

- [ ] **Step 3: 运行 E2E 和计费测试**

```powershell
go test ./e2e -run 'TestMegaByAI' -count=1 -v
go test ./relay -run 'TestMegaByAI|TestTaskBilling' -count=1 -v
```

Expected: PASS。

- [ ] **Step 4: 提交 E2E**

```powershell
git add e2e/megabyai_upstream_e2e_test.go relay/relay_task_billing_test.go
git commit -m "test(megabyai): cover Ark video lifecycle"
```

---

### Task 6: 全量验证和真实上游验收

**Files:**
- Create after successful real test: `docs/superpowers/reports/2026-07-26-megabyai-channel-acceptance.md`

- [ ] **Step 1: 运行后端检查**

```powershell
go test ./relay/channel/task/newapivideo ./constant ./relay ./controller ./e2e -count=1
go vet ./relay/channel/task/newapivideo ./relay ./controller ./constant
go build ./...
```

Expected: 全部退出码 0。

- [ ] **Step 2: 运行前端检查**

```powershell
Set-Location web
bun test tests/channel-type-config.test.ts
bun run typecheck
bun run build
Set-Location ..
```

Expected: 全部退出码 0。

- [ ] **Step 3: 运行真实上游验收**

凭据只从环境变量 `MEGABYAI_API_KEY` 读取。通过 new-api 的 `/api/v3/contents/generations/tasks` 分别提交：

1. `videos-mini` 映射的纯文本 5 秒 720p 请求。
2. `videos-mini` 映射的一图加一段不超过 15 秒音频的 5 秒 720p 请求。
3. `videos-mini` 映射的图片+视频+音频混合请求，分别核对视频和音频总时长上限。

轮询公开 ID，核对 MP4 可访问、时长、Ark 单查/列表、账单和失败退款。报告只记录脱敏请求头；签名 URL 去除查询串后再写文档。

- [ ] **Step 4: 写验收报告并提交**

报告必须包含测试时间、模型映射、请求/响应、最终状态、媒体规格、计费快照、公开 ID 隔离和未实现 DELETE/内容代理边界。

```powershell
git add docs/superpowers/reports/2026-07-26-megabyai-channel-acceptance.md
git commit -m "docs: record MegaByAI channel acceptance"
```

---

## 自检

### 设计覆盖

| 设计要求 | 实施任务 |
| --- | --- |
| Ark 创建、单查、列表 | Task 3、Task 5 |
| 图片/视频/音频数组翻译 | Task 1 |
| 视频/音频分别 15 秒总时长上限 | Task 1、Task 5、Task 6 |
| `/v1/videos` 创建和轮询 | Task 1 |
| URL 变体和伪成功关闭 | Task 2 |
| 公开/私有任务 ID 隔离 | Task 3、Task 5 |
| 计费、成功结算、失败退款 | Task 3、Task 5 |
| 管理端类型和三模型目录 | Task 4 |
| mock 与真实验收 | Task 5、Task 6 |

### 类型与顺序

- Lucen 占用 62，本计划 MegaByAI 占用 63，并把 Dummy 移到 64。
- `NewMegaByAITaskAdaptor()`、`megaByAIProtocolProfile()` 和 `buildMegaByAIRequest()` 的命名在所有任务中一致。
- 苍原、派普和 Secure 计划依赖本计划的端点 profile 和直接任务响应投影，必须在本计划之后执行。

无模糊占位符；所有测试、文件、命令和失败边界均已给出。
