# Dimensio 协议翻译 Adaptor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

> **Billing authority:** 本文保留 Dimensio 协议翻译、模型限制、错误处理和公开任务 ID 的设计决定。计费实现与验收以 [`2026-07-19-per-duration-billing.md`](./2026-07-19-per-duration-billing.md) 为准；其中定义的源模型 `per_duration` 规则、请求时长快照、保留倍率名和退款日志契约覆盖本文早期的倍率计费草案。

**Goal:** 新增 dimensio 渠道,作为"协议翻译网关"——客户端用火山方舟(ARK)v3 协议发请求,new-api 把 ARK 数据结构翻译成 dimensio 原生数据结构发给上游,再把 dimensio 响应翻译回 ARK v3 格式返回客户端。让 ARK SDK 用户只替换 `base_url` 和 token,客户端路径、请求代码和轮询逻辑均不变。

**Architecture:** 复用计划 1(`2026-07-18-ark-native-compat.md`)已经落地的 `/api/v3` 入口和任务管线。Dimensio 是 task-only 渠道：不新增 API type，也不接入 chat/image 的通用 adaptor；`GetTaskAdaptor` 按 platform 59 返回独立 adaptor。提交阶段先完成渠道模型映射，再由 adaptor 把 ARK `content[]` 翻译为 dimensio 顶层素材字段；轮询阶段仍以 `PrivateData.UpstreamTaskID` 访问上游，原生查询出口通过窄接口恢复 ARK responseTask，OpenAI `/v1/videos` 出口继续使用独立的 `OpenAIVideoConverter`。

**Tech Stack:** Go 1.22+, Gin, GORM v2, testify(`require` + `assert`),JSON 统一走 `common.*`。销售计费由客户端源模型的 `BillingModePerDuration` + `DurationPrice` 定义；请求时长进入专用快照字段，`OtherRatios` 只保留 `resolution` 等非时长倍率。

**输入文档:** `docs/channel/jimeng.dimensio.cn.md`(dimensio 官方接入规格)

**依赖:** 计划 1(`2026-07-18-ark-native-compat.md`)的入口路由、`SeedanceRequestConvert`、`KeySeedanceOfficialAPI`、公开 `task_*` ID 和原生任务查询必须先落地。本计划不得重复注册路由，也不得修改计划 1 已定义的 `/api/v3` 路径语义。

## Review Decisions

1. **ARK 公共入口只有 `/api/v3`:** 不新增、重写或文档化 `/seedance/api/v3`。旧前缀应保持 404,避免形成第二套 SDK 基址。
2. **SDK 基址契约:** 示例中的 ARK SDK `base_url` 固定为 `https://<new-api>/api/v3`(curl 示例可展开为完整 `/api/v3/...` 路径),认证只使用 new-api token。
3. **公开/上游 ID 隔离:** 提交响应的 `id` 必须是 `info.PublicTaskID`(通常为 `task_*`); `dimensio.task_id` 仅作为 `DoResponse` 返回值写入 `PrivateData.UpstreamTaskID`,绝不出现在提交、查询或列表响应中。查询始终按公开 ID 取任务。
4. **响应接口分离:** `ConvertToOpenAIVideo` 继续只服务 `/v1/videos` 的 OpenAI Video 响应; ARK `/api/v3` 单查和列表使用新增的 ARK 原生转换接口,不能把一个接口的 JSON 形状冒充另一个接口。
5. **列表查询覆盖新渠道:** 原生任务列表的 platform 条件必须包含 `ChannelTypeDimensio(59)`,并继续按用户和公开 `task_id` 隔离。
6. **Dimensio 是 task-only channel:** 不在 `common.ChannelType2APIType` 中新增 API type 映射；这样不会把 Dimensio 模型错误地暴露为通用 OpenAI 模型。管理员通过渠道 API 配置 type=59 和模型映射，前端配置 UI 不在本计划范围内。
7. **请求语义不能静默丢失:** ARK `seed`、`camera_fixed`、`watermark`、`generate_audio`、`frames`、`draft`、`tools` 等 Dimensio 不支持的显式字段在映射后返回 400，并在测试中确认不会访问上游；不再“丢弃并记 debug”。
8. **默认值显式归一化:** ARK/Seedance 的 `adaptive` ratio 在 Dimensio 没有等价能力，返回 400；缺省 ratio 显式发送 `16:9`，避免使用 Dimensio 文档的 `9:16` 默认值造成隐式语义变化。缺省 duration 显式发送 5 秒，显式 `0`、`-1`、超过 15 秒或超过 `relaycommon.MaxTaskDurationSeconds` 均在上游前拒绝。
9. **计费包含分辨率:** Dimensio `jimeng-video-seedance-2.0-vip` 的 1080p 单价是 720p 的 2.5 倍；`EstimateDurationSeconds` 提供已校验的请求时长，`EstimateBilling` 只返回 `resolution` 倍率并通过 `PriceData.AddOtherRatio` 合并。`seconds`/`duration` 是 `per_duration` 保留名，不得写入 `OtherRatios`。

---

## Scope

✅ 新增 `ChannelTypeDimensio`(59)渠道类型
✅ 新建 `relay/channel/task/dimensio/` adaptor,实现 ARK↔dimensio 双向翻译
✅ 在 `GetTaskAdaptor` 注册 dimensio
✅ 源模型 `per_duration` 计费 + 请求时长快照 + 分辨率倍率(`OtherRatio: resolution`)
✅ ARK 原生查询响应与 OpenAI Video 查询响应分别转换
✅ 原生任务列表纳入 dimensio platform
❌ 入口路由(计划 1 负责)
❌ `/seedance` 兼容别名(明确禁止)
❌ 前端渠道配置 UI(后续独立 PR,管理员可通过 API 先配)
❌ chat/embeddings/responses(仅视频)

---

## 协议翻译映射(实施依据)

### 请求方向:ARK v3 → dimensio

| ARK v3 字段 | dimensio 字段 | 翻译逻辑 |
|---|---|---|
| `model` | `model` | 渠道模型映射(`doubao-seedance-2-0-260128` → `jimeng-video-seedance-2.0-vip`) |
| `content[].type=text` 的 text | `prompt` | 提取第一个 text 项 |
| `content[].type=image_url` 的 url(按顺序) | `image_file_1`..`image_file_N` | 枚举填充；不再同时发送 `file_paths`，避免同一素材被上游重复计数 |
| `content[].type=video_url` 的 url | `video_file_1`..`video_file_N` | 枚举填充(N≤3,总时长≤15s) |
| `content[].type=audio_url` 的 url | `audio_file_1`..`audio_file_N` | 枚举填充(N≤3) |
| `ratio` | `ratio` | 固定值直传；缺省写 `16:9`；`adaptive` 返回 400 |
| `resolution` | `resolution` | 直传 |
| `duration` | `duration` | 缺省写 5；显式值必须是 4-15 的整数，并受 `relaycommon.MaxTaskDurationSeconds` 约束 |
| (无,需推导) | `functionMode` ⚠️**必填** | 从 content 推导(见下) |
| `intelligent_ratio`(若客户端传) | `intelligent_ratio` | 直传(可选) |
| `face_grid`(若客户端传) | `face_grid` | 直传(可选) |
| `seed`/`camera_fixed`/`watermark`/`generate_audio` | (无对应) | **拒绝**(dimensio 不支持，返回 400，不访问上游) |

**`functionMode` 推导规则**(ARK 无此字段,dimensio 必填):
```
if content[] 含 video_url 或 audio_url 或任意 image_url.role == "reference_image":
    functionMode = "omni_reference"    // 总素材数≤12，图片≤9、视频≤3、音频≤3
else:
    functionMode = "first_last_frames"  // 文生/单图/首尾帧，图片≤2
```

`image_url` 未填写 role 时按 ARK 首帧语义处理；`first_frame`/`last_frame` 与 `reference_image`、视频、音频互斥。纯音频、空 prompt、缺失媒体 URL 和总素材数超过 12 均返回 400。对于 `video_url`/`audio_url`，role 必须分别为 `reference_video`/`reference_audio`。

`intelligent_ratio`、`face_grid` 仅在客户端显式提供时透传；Dimensio 的 multipart 兼容字段不在本 adaptor 范围内，提交始终使用 JSON。

### 响应方向:dimensio → ARK v3

**提交响应:**
| dimensio | ARK v3 | 翻译 |
|---|---|---|
| `{created, task_id, status:"pending"}` | `{id}` | `id = info.PublicTaskID`; `task_id` 仅内部持久化为上游 ID |

**查询响应(dimensio → ARK responseTask):**
| dimensio | ARK responseTask | 翻译 |
|---|---|---|
| `task_id` | `id` | **不直传**; `id = task.TaskID`(公开 ID) |
| `status: pending` | `status: queued` | 映射 |
| `status: processing` | `status: running` | 映射 |
| `status: completed` | `status: succeeded` | 映射 |
| `status: failed` | `status: failed` | 直传 |
| `status: not_found` | `status: failed` + `error.message: "task not found"` | 映射 |
| `progress` | (不输出) | ARK responseTask 无此字段；仅用于内部轮询进度 |
| `result.url` | `content.video_url` | 嵌入 content 对象 |
| `error` | `error.message` | 移入 error |
| `error_code` | `error.code` | 移入 error |
| (缺失) | `model` | 从任务记录 `OriginModelName` 补 |
| (缺失) | `created_at` | 从 `task.SubmitTime` 补 |
| (缺失) | `updated_at` | 从 `task.UpdatedAt` 补 |
| (缺失) | `usage.completion_tokens` | 留空(dimensio 无 token；`per_duration` 计费不依赖它) |

---

## File Structure

| 文件 | 责任 |
|---|---|
| `constant/channel.go` | 新增 `ChannelTypeDimensio = 59`,Dummy 顺移到 60 |
| `constant/channel.go` | `ChannelBaseURLs` 追加 dimensio 默认 URL,显示名映射 |
| `constant/channel_test.go` | 锁定 type 59、Dummy 60 和默认 URL 映射 |
| `relay/relay_adaptor.go` | `GetTaskAdaptor` 注册 dimensio case |
| `relay/channel/task/dimensio/constants.go` | 模型列表 + 计费辅助 |
| `relay/channel/task/dimensio/adaptor.go` | `TaskAdaptor` 实现(核心:翻译逻辑、请求错误解析) |
| `relay/common/relay_info.go` | 为任务失败结果增加供应商错误码，供 ARK 错误响应使用 |
| `relay/channel/task/dimensio/adaptor_test.go` | adaptor 生命周期、校验、HTTP 错误和响应测试 |
| `relay/channel/task/dimensio/translate.go` | ARK↔dimensio 纯函数翻译(易测试) |
| `relay/channel/task/dimensio/translate_test.go` | 翻译函数表驱动测试 |
| `relay/channel/adapter.go` | 新增独立的 ARK 原生视频任务转换接口 |
| `relay/channel/adapter.go` | 新增可选的上游 task 错误解析接口，覆盖非 2xx 的 Dimensio `{code,message,data}` |
| `relay/relay_task.go` | 在非 2xx 分支调用 task adaptor 的错误解析接口 |
| `relay/seedance_task.go` | 原生单查/列表调用 ARK 转换接口,platform 纳入 59 |
| `relay/relay_task_seedance_test.go` | 公开 ID、时间戳、dimensio 响应形状和列表 platform 回归测试 |

---

## Task 1: 新增渠道类型常量

**Files:**
- Modify: `constant/channel.go`
- Create: `constant/channel_test.go`

- [ ] **Step 1: 新增 `ChannelTypeDimensio`**

修改 `constant/channel.go`,在 `ChannelTypeAdvancedCustom = 58` 之后显式定义新渠道和 Dummy 上界:

```go
	ChannelTypeAdvancedCustom = 58
	ChannelTypeDimensio       = 59 // dimensio 视频生成(ARK v3 协议翻译网关)
	ChannelTypeDummy          = 60 // this one is only for count, do not add any channel after this
```

> **注意:** 这里没有使用 `iota`。省略表达式只会重复上一行的值,所以 `ChannelTypeDummy` 必须显式写成 60;否则它会与 `ChannelTypeDimensio` 同为 59。`controller/model.go` 的循环上界因此仍能覆盖新渠道。

- [ ] **Step 2: 追加 ChannelBaseURLs 和显示名**

在 `constant/channel.go` 的 `ChannelBaseURLs` slice 末尾(对应 index 59)追加:

```go
	"https://jimeng.dimensio.cn",             // 59 Dimensio
```

当前仓库已核对 `ChannelBaseURLs[58]` 是 Advanced Custom 的空位；直接追加 index 59，不要重复插入 index 58。

在同一文件的 `ChannelTypeNames` 映射追加:

```go
	ChannelTypeDimensio: "Dimensio",
```

- [ ] **Step 3: 验证编译**

Run: `go build ./constant/`
Expected: 无输出

- [ ] **Step 4: 写渠道常量回归测试**

创建 `constant/channel_test.go`，使用 `require` 断言以下公开契约：

```go
package constant

import (
    "testing"

    "github.com/stretchr/testify/require"
)

func TestDimensioChannelConstants(t *testing.T) {
    require.Equal(t, 59, ChannelTypeDimensio)
    require.Equal(t, 60, ChannelTypeDummy)
    require.Equal(t, "https://jimeng.dimensio.cn", ChannelBaseURLs[ChannelTypeDimensio])
    require.Equal(t, "Dimensio", GetChannelTypeName(ChannelTypeDimensio))
}
```

Run: `go test ./constant ./controller -count=1`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add constant/channel.go constant/channel_test.go
git commit -m "feat(dimensio): add ChannelTypeDimensio (59) channel type constant"
```

---

## Task 2: 翻译纯函数(ARK↔dimensio,核心,最先 TDD)

**Files:**
- Create: `relay/channel/task/dimensio/translate.go`
- Test: `relay/channel/task/dimensio/translate_test.go`

> 翻译逻辑是纯函数,最适合 TDD。先写表驱动测试覆盖所有映射,再实现。这是整个 adaptor 的核心价值。

- [ ] **Step 1: 写失败测试(请求翻译:多模态 content 拆解)**

创建 `relay/channel/task/dimensio/translate_test.go`:

```go
package dimensio

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestArkToDimensioMultimodal 验证 ARK content[] 多模态结构被正确拆解为 dimensio 的
// image_file_N / video_file_N / audio_file_N 顶层字段。
// 这是问题1中"9图3视频1音频"场景在 dimensio 渠道的翻译不变量。
func TestArkToDimensioMultimodal(t *testing.T) {
	arkReq := ArkRequest{
		Model: "doubao-seedance-2-0-260128",
		Content: []ArkContent{
			{Type: "image_url", Role: "reference_image", ImageURL: &ArkMedia{URL: "https://x/img1.jpg"}},
			{Type: "image_url", Role: "reference_image", ImageURL: &ArkMedia{URL: "https://x/img2.jpg"}},
			{Type: "video_url", Role: "reference_video", VideoURL: &ArkMedia{URL: "https://x/ref1.mp4"}},
			{Type: "video_url", Role: "reference_video", VideoURL: &ArkMedia{URL: "https://x/ref2.mp4"}},
			{Type: "video_url", Role: "reference_video", VideoURL: &ArkMedia{URL: "https://x/ref3.mp4"}},
			{Type: "audio_url", Role: "reference_audio", AudioURL: &ArkMedia{URL: "https://x/bg.mp3"}},
			{Type: "text", Text: "镜头缓慢推进"},
		},
		Duration:   common.GetPointer(10),
		Resolution: "720p",
		Ratio:      "16:9",
	}

	dim, err := ArkToDimensio(arkReq)
	require.NoError(t, err)

	// 基础字段直传
	assert.Equal(t, "doubao-seedance-2-0-260128", dim.Model) // 映射前保留原名,映射在 adaptor 层做
	assert.Equal(t, "镜头缓慢推进", dim.Prompt)
	require.NotNil(t, dim.Duration)
	assert.Equal(t, 10, *dim.Duration)
	assert.Equal(t, "720p", dim.Resolution)
	assert.Equal(t, "16:9", dim.Ratio)

	// functionMode 推导:含视频/音频 → omni_reference
	assert.Equal(t, "omni_reference", dim.FunctionMode)

	// 多模态素材按类型+顺序枚举到顶层字段
	assert.Equal(t, "https://x/img1.jpg", dim.ImageFiles["image_file_1"])
	assert.Equal(t, "https://x/img2.jpg", dim.ImageFiles["image_file_2"])
	assert.Equal(t, "https://x/ref1.mp4", dim.VideoFiles["video_file_1"])
	assert.Equal(t, "https://x/ref2.mp4", dim.VideoFiles["video_file_2"])
	assert.Equal(t, "https://x/ref3.mp4", dim.VideoFiles["video_file_3"])
	assert.Equal(t, "https://x/bg.mp3", dim.AudioFiles["audio_file_1"])

	// 解析阶段保留图片顺序，但序列化时只发送 image_file_N，避免重复计数。
	assert.Equal(t, []string{"https://x/img1.jpg", "https://x/img2.jpg"}, dim.FilePaths)
}
```

- [ ] **Step 2: 运行,验证失败**

Run: `go test ./relay/channel/task/dimensio/ -run TestArkToDimensioMultimodal -count=1 -v`
Expected: FAIL with `undefined: ArkRequest` / `undefined: ArkToDimensio`

- [ ] **Step 3: 实现翻译类型和函数**

创建 `relay/channel/task/dimensio/translate.go`:

```go
package dimensio

import (
	"fmt"
	"strings"
)

// ===== ARK v3 请求侧类型(解析客户端发来的 ARK 格式)=====

// ArkRequest 对应 ARK v3 视频提交请求体
type ArkRequest struct {
	Model      string        `json:"model"`
	Content    []ArkContent  `json:"content"`
	Resolution string        `json:"resolution,omitempty"`
	Ratio      string        `json:"ratio,omitempty"`
	Duration   *int          `json:"duration,omitempty"`
	// ARK 字段；Dimensio 不支持的字段由 adaptor 校验并拒绝
	Seed           *int   `json:"seed,omitempty"`
	CameraFixed    *bool  `json:"camera_fixed,omitempty"`
	Watermark      *bool  `json:"watermark,omitempty"`
	GenerateAudio  *bool  `json:"generate_audio,omitempty"`
	Frames         *int   `json:"frames,omitempty"`
	Draft          *bool  `json:"draft,omitempty"`
	Priority       *int   `json:"priority,omitempty"`
	ExecutionExpiresAfter *int `json:"execution_expires_after,omitempty"`
	ReturnLastFrame *bool `json:"return_last_frame,omitempty"`
	SafetyIdentifier *string `json:"safety_identifier,omitempty"`
	Tools          *[]struct { Type string `json:"type,omitempty"` } `json:"tools,omitempty"`
	// dimensio 可选字段,若客户端在 ARK 请求里带了就透传
	IntelligentRatio *bool `json:"intelligent_ratio,omitempty"`
	FaceGrid         *bool `json:"face_grid,omitempty"`
}

// ArkContent ARK content[] 数组项
type ArkContent struct {
	Type     string    `json:"type"`               // text/image_url/video_url/audio_url
	Text     string    `json:"text,omitempty"`
	ImageURL *ArkMedia `json:"image_url,omitempty"`
	VideoURL *ArkMedia `json:"video_url,omitempty"`
	AudioURL *ArkMedia `json:"audio_url,omitempty"`
	Role     string    `json:"role,omitempty"` // first_frame/last_frame/reference_image 等
}

// ArkMedia image_url/video_url/audio_url 的 {url} 对象
type ArkMedia struct {
	URL string `json:"url"`
}

// ===== dimensio 请求侧类型(发给上游 dimensio)=====

// DimensioRequest 对应 dimensio 视频提交请求体
// 注意:ImageFiles/VideoFiles/AudioFiles 用 map 存 image_file_N 等,序列化时展开为顶层字段(见 MarshalDimensioRequest)
type DimensioRequest struct {
	Model           string            `json:"model"`
	Prompt          string            `json:"prompt"`
	FunctionMode    string            `json:"functionMode"` // 必填
	Ratio           string            `json:"ratio,omitempty"`
	Resolution      string            `json:"resolution,omitempty"`
	Duration        *int              `json:"duration,omitempty"`
	IntelligentRatio *bool            `json:"intelligent_ratio,omitempty"`
	FaceGrid         *bool            `json:"face_grid,omitempty"`
	FilePaths       []string          `json:"-"`                    // 保留解析结果，不发送；避免与 image_file_N 重复计数
	ImageFiles      map[string]string `json:"-"`                    // image_file_1..9,序列化时合并到顶层
	VideoFiles      map[string]string `json:"-"`                    // video_file_1..3
	AudioFiles      map[string]string `json:"-"`                    // audio_file_1..3
}

// ===== ARK↔dimensio 翻译纯函数 =====

// ArkToDimensio 把 ARK v3 请求翻译成 dimensio 请求。
// 纯函数,不做 HTTP/模型映射(模型映射在 adaptor 层用 info.UpstreamModelName 做)。
func ArkToDimensio(ark ArkRequest) (DimensioRequest, error) {
	if err := validateArkContentRoles(ark.Content); err != nil {
		return DimensioRequest{}, err
	}
	dim := DimensioRequest{
		Model:        ark.Model,
		Ratio:        ark.Ratio,
		Resolution:   ark.Resolution,
		Duration:     ark.Duration,
		FilePaths:    []string{},
		ImageFiles:   map[string]string{},
		VideoFiles:   map[string]string{},
		AudioFiles:   map[string]string{},
		IntelligentRatio: ark.IntelligentRatio,
		FaceGrid:         ark.FaceGrid,
	}

	imgIdx, vidIdx, audIdx := 0, 0, 0
	for _, item := range ark.Content {
		switch item.Type {
		case "text":
			if dim.Prompt == "" && strings.TrimSpace(item.Text) != "" {
				dim.Prompt = item.Text
			}
		case "image_url":
			if item.ImageURL == nil || strings.TrimSpace(item.ImageURL.URL) == "" {
				return DimensioRequest{}, fmt.Errorf("image_url.url is required")
			}
			imgIdx++
			if imgIdx > 9 {
				return DimensioRequest{}, fmt.Errorf("too many images: dimensio allows at most 9 (image_file_1..9)")
			}
			key := fmt.Sprintf("image_file_%d", imgIdx)
			role := strings.TrimSpace(item.Role)
			if role != "" && role != "first_frame" && role != "last_frame" && role != "reference_image" {
				return DimensioRequest{}, fmt.Errorf("unsupported image role: %s", role)
			}
			dim.ImageFiles[key] = item.ImageURL.URL
			dim.FilePaths = append(dim.FilePaths, item.ImageURL.URL)
		case "video_url":
			if item.VideoURL == nil || strings.TrimSpace(item.VideoURL.URL) == "" {
				return DimensioRequest{}, fmt.Errorf("video_url.url is required")
			}
			vidIdx++
			if vidIdx > 3 {
				return DimensioRequest{}, fmt.Errorf("too many videos: dimensio allows at most 3 (video_file_1..3)")
			}
			dim.VideoFiles[fmt.Sprintf("video_file_%d", vidIdx)] = item.VideoURL.URL
		case "audio_url":
			if item.AudioURL == nil || strings.TrimSpace(item.AudioURL.URL) == "" {
				return DimensioRequest{}, fmt.Errorf("audio_url.url is required")
			}
			audIdx++
			if audIdx > 3 {
				return DimensioRequest{}, fmt.Errorf("too many audios: dimensio allows at most 3 (audio_file_1..3)")
			}
			dim.AudioFiles[fmt.Sprintf("audio_file_%d", audIdx)] = item.AudioURL.URL
		default:
			return DimensioRequest{}, fmt.Errorf("unsupported content type: %s", item.Type)
		}
	}
	if imgIdx+vidIdx+audIdx > 12 {
		return DimensioRequest{}, fmt.Errorf("too many media items: dimensio allows at most 12 total")
	}

	if strings.TrimSpace(dim.Prompt) == "" {
		return DimensioRequest{}, fmt.Errorf("text prompt is required")
	}
	dim.FunctionMode = deriveFunctionMode(ark.Content)
	return dim, nil
}

// deriveFunctionMode 从 ARK content 的媒体类型和 role 推导 dimensio 必填字段。
func deriveFunctionMode(content []ArkContent) string {
	for _, item := range content {
		if item.Type == "video_url" || item.Type == "audio_url" || (item.Type == "image_url" && item.Role == "reference_image") {
			return "omni_reference"
		}
	}
	return "first_last_frames"
}

func validateArkContentRoles(content []ArkContent) error {
	images, first, last, references, videos, audios := 0, 0, 0, 0, 0, 0
	for _, item := range content {
		switch item.Type {
		case "image_url":
			images++
			switch item.Role {
			case "", "first_frame":
				first++
			case "last_frame":
				last++
			case "reference_image":
				references++
			default:
				return fmt.Errorf("unsupported image role: %s", item.Role)
			}
		case "video_url":
			videos++
			// role validation is performed during ArkToDimensio where the URL
			// payload is available; this pass only computes the scene counts.
		case "audio_url":
			audios++
		}
	}
	if images+videos+audios > 12 {
		return fmt.Errorf("too many media items: dimensio allows at most 12 total")
	}
	if images > 9 {
		return fmt.Errorf("too many images: dimensio allows at most 9 (image_file_1..9)")
	}
	if videos > 3 {
		return fmt.Errorf("too many videos: dimensio allows at most 3 (video_file_1..3)")
	}
	if audios > 3 {
		return fmt.Errorf("too many audios: dimensio allows at most 3 (audio_file_1..3)")
	}
	if audios > 0 && images == 0 && videos == 0 {
		return fmt.Errorf("audio input requires an image or video")
	}
	if references > 0 && (first > 0 || last > 0 || videos > 0 || audios > 0) {
		return fmt.Errorf("reference media cannot mix with first/last frames")
	}
	if last > 0 && first != 1 || first > 1 || last > 1 {
		return fmt.Errorf("first/last frames require one first frame and at most one last frame")
	}
	if videos > 0 || audios > 0 {
		for _, item := range content {
			if item.Type == "video_url" && item.Role != "reference_video" {
				return fmt.Errorf("video role must be reference_video")
			}
			if item.Type == "audio_url" && item.Role != "reference_audio" {
				return fmt.Errorf("audio role must be reference_audio")
			}
		}
	}
	return nil
}

```

- [ ] **Step 4: 运行,验证通过**

Run: `go test ./relay/channel/task/dimensio/ -run TestArkToDimensioMultimodal -v`
Expected: PASS

- [ ] **Step 5: 写测试(functionMode 和内容角色矩阵)**

在 `translate_test.go` 追加:

```go
// TestDeriveFunctionModeMatrix 覆盖 functionMode 推导的所有组合
func TestDeriveFunctionModeMatrix(t *testing.T) {
	cases := []struct {
		name     string
		content  []ArkContent
		expected string
	}{
		{"text_only", []ArkContent{{Type: "text", Text: "x"}}, "first_last_frames"},
		{"single_image", []ArkContent{{Type: "image_url", Role: "first_frame"}}, "first_last_frames"},
		{"two_frames", []ArkContent{{Type: "image_url", Role: "first_frame"}, {Type: "image_url", Role: "last_frame"}}, "first_last_frames"},
		{"two_reference_images", []ArkContent{{Type: "image_url", Role: "reference_image"}, {Type: "image_url", Role: "reference_image"}}, "omni_reference"},
		{"image_plus_video", []ArkContent{{Type: "image_url", Role: "reference_image"}, {Type: "video_url", Role: "reference_video"}}, "omni_reference"},
		{"image_plus_audio", []ArkContent{{Type: "image_url", Role: "reference_image"}, {Type: "audio_url", Role: "reference_audio"}}, "omni_reference"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, deriveFunctionMode(tc.content))
		})
	}
}
```

- [ ] **Step 6: 运行,验证通过**

Run: `go test ./relay/channel/task/dimensio/ -run TestDeriveFunctionModeMatrix -count=1 -v`
Expected: PASS(逻辑已在 Step 3 实现)

- [ ] **Step 7: 写测试(素材超限报错)**

在 `translate_test.go` 追加:

```go
// TestArkToDimensioRejectsTooManyImages 验证图片超 9 张报错
func TestArkToDimensioRejectsTooManyImages(t *testing.T) {
	content := make([]ArkContent, 0, 11)
	for i := 0; i < 10; i++ { // 10 张图,超 9 张上限
		content = append(content, ArkContent{Type: "image_url", Role: "reference_image", ImageURL: &ArkMedia{URL: "https://x/img.jpg"}})
	}
	content = append(content, ArkContent{Type: "text", Text: "hi"})

	_, err := ArkToDimensio(ArkRequest{Model: "m", Content: content})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "too many images")
}

func TestArkToDimensioRejectsTooManyVideos(t *testing.T) {
	content := []ArkContent{
		{Type: "video_url", Role: "reference_video", VideoURL: &ArkMedia{URL: "v1"}},
		{Type: "video_url", Role: "reference_video", VideoURL: &ArkMedia{URL: "v2"}},
		{Type: "video_url", Role: "reference_video", VideoURL: &ArkMedia{URL: "v3"}},
		{Type: "video_url", Role: "reference_video", VideoURL: &ArkMedia{URL: "v4"}}, // 第4个,超 3 个上限
		{Type: "text", Text: "hi"},
	}
	_, err := ArkToDimensio(ArkRequest{Model: "m", Content: content})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "too many videos")
}

func TestArkToDimensioRejectsRoleMixAndEmptyPrompt(t *testing.T) {
	_, err := ArkToDimensio(ArkRequest{Model: "m", Content: []ArkContent{
			{Type: "image_url", Role: "reference_image", ImageURL: &ArkMedia{URL: "i"}},
		{Type: "image_url", Role: "first_frame", ImageURL: &ArkMedia{URL: "f"}},
		{Type: "text", Text: "x"},
	}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot mix")

	_, err = ArkToDimensio(ArkRequest{Model: "m", Content: []ArkContent{{Type: "image_url", Role: "first_frame", ImageURL: &ArkMedia{URL: "i"}}}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "prompt is required")
}
```

- [ ] **Step 8: 运行,验证通过**

Run: `go test ./relay/channel/task/dimensio/ -run "TestArkToDimensioRejects" -count=1 -v`
Expected: PASS

- [ ] **Step 9: Commit**

```bash
git add relay/channel/task/dimensio/translate.go relay/channel/task/dimensio/translate_test.go
git commit -m "feat(dimensio): ARK→dimensio request translation pure functions"
```

---

## Task 3: 响应翻译函数(dimensio→ARK)

**Files:**
- Modify: `relay/channel/task/dimensio/translate.go`
- Modify: `relay/channel/task/dimensio/translate_test.go`

- [ ] **Step 1: 写失败测试(查询响应翻译)**

在 `translate_test.go` 追加:

```go
// TestDimensioToArkTaskResponse 把 dimensio 查询响应翻译成 ARK responseTask 格式
// 这是 ARK SDK 客户端零改造的关键:查询收到的必须是 ARK 形态
func TestDimensioToArkTaskResponse(t *testing.T) {
	cases := []struct {
		name           string
		dimensioStatus string
		arkStatus      string
		resultURL      string
	}{
		{"pending", "pending", "queued", ""},
		{"processing", "processing", "running", ""},
		{"completed", "completed", "succeeded", "https://x/v.mp4"},
		{"failed", "failed", "failed", ""},
		{"not_found", "not_found", "failed", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dim := DimensioTaskResponse{
				TaskID:    "dim-123",
				Status:    tc.dimensioStatus,
				Progress:  50,
				Result:    DimensioResult{URL: tc.resultURL},
				Error:     "审核不通过",
				ErrorCode: "2043",
			}
			// 补全字段(模拟任务记录提供)
			ark, err := DimensioToArkTask(dim, "task_public", "doubao-seedance-2-0-260128", int64(1710000000), int64(1710000100))
			require.NoError(t, err)

			assert.Equal(t, "task_public", ark.ID)
			assert.NotEqual(t, dim.TaskID, ark.ID)
			assert.Equal(t, tc.arkStatus, ark.Status)
			assert.Equal(t, "doubao-seedance-2-0-260128", ark.Model)
			assert.Equal(t, int64(1710000000), ark.CreatedAt)
			assert.Equal(t, int64(1710000100), ark.UpdatedAt)

			// 视频URL:completed 时填入 content.video_url
			if tc.resultURL != "" {
				assert.Equal(t, tc.resultURL, ark.Content.VideoURL)
			}

			// 错误信息移入 error 对象(failed/not_found)
			if tc.dimensioStatus == "failed" || tc.dimensioStatus == "not_found" {
				require.NotNil(t, ark.Error)
				assert.Equal(t, "2043", ark.Error.Code)
			}
		})
	}
}
```

- [ ] **Step 2: 运行,验证失败**

Run: `go test ./relay/channel/task/dimensio/ -run TestDimensioToArkTaskResponse -count=1 -v`
Expected: FAIL with `undefined: DimensioTaskResponse` / `DimensioToArkTask`

- [ ] **Step 3: 实现响应翻译类型和函数**

在 `translate.go` 追加:

```go
// ===== dimensio 响应侧类型 =====

// DimensioTaskResponse dimensio 查询任务响应
type DimensioTaskResponse struct {
	TaskID    string        `json:"task_id"`
	Status    string        `json:"status"` // pending/processing/completed/failed/not_found
	Progress  int           `json:"progress"`
	Result    DimensioResult `json:"result"`
	Error     string        `json:"error,omitempty"`
	ErrorCode string        `json:"error_code,omitempty"`
}

// DimensioResult dimensio result 对象(含视频 URL)
type DimensioResult struct {
	URL string `json:"url"`
}

// DimensioSubmitResponse dimensio 提交任务响应
type DimensioSubmitResponse struct {
	Created int64  `json:"created"`
	TaskID  string `json:"task_id"`
	Status  string `json:"status"`
}

// ===== ARK 响应侧类型(返回给客户端)=====

// ArkTaskResponse 对应 ARK v3 responseTask 格式(查询时返回)
type ArkTaskResponse struct {
	ID         string         `json:"id"`
	Model      string         `json:"model,omitempty"`
	Status     string         `json:"status"`
	Content    ArkContentResp `json:"content"`
	Usage      ArkUsage       `json:"usage,omitempty"`
	Error      *ArkError      `json:"error,omitempty"`
	CreatedAt  int64          `json:"created_at,omitempty"`
	UpdatedAt  int64          `json:"updated_at,omitempty"`
}

type ArkContentResp struct {
	VideoURL string `json:"video_url,omitempty"`
}

type ArkUsage struct {
	CompletionTokens int `json:"completion_tokens,omitempty"`
	TotalTokens      int `json:"total_tokens,omitempty"`
}

type ArkError struct {
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

// DimensioToArkTask 把 dimensio 查询响应翻译成 ARK responseTask 格式。
// publicTaskID/model/createdAt/updatedAt 由任务记录补全(dimensio 响应不含这些)。
func DimensioToArkTask(dim DimensioTaskResponse, publicTaskID, modelName string, createdAt, updatedAt int64) (ArkTaskResponse, error) {
	ark := ArkTaskResponse{
		ID:        publicTaskID,
		Model:     modelName,
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
	}

	switch dim.Status {
	case "pending":
		ark.Status = "queued"
	case "processing":
		ark.Status = "running"
	case "completed":
		ark.Status = "succeeded"
		ark.Content.VideoURL = dim.Result.URL
	case "failed":
		ark.Status = "failed"
		message := dim.Error
		if message == "" {
			message = "task failed"
		}
		ark.Error = &ArkError{Code: dim.ErrorCode, Message: message}
	case "not_found":
		ark.Status = "failed"
		ark.Error = &ArkError{Code: dim.ErrorCode, Message: "task not found or expired"}
	default:
		return ArkTaskResponse{}, fmt.Errorf("unknown dimensio task status: %s", dim.Status)
	}

	return ark, nil
}
```

- [ ] **Step 4: 运行,验证通过**

Run: `go test ./relay/channel/task/dimensio/ -run TestDimensioToArkTaskResponse -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add relay/channel/task/dimensio/translate.go relay/channel/task/dimensio/translate_test.go
git commit -m "feat(dimensio): dimensio→ARK response translation functions"
```

响应翻译测试还要断言：`DimensioToArkTask` 不把 `progress` 或 `task_id` 泄漏到 ARK 响应，`completed` 缺失 URL 仍保持 `succeeded` 但不伪造 `content.video_url`，`failed` 空 error 使用稳定的 `task failed` 文案。

---

## Task 4: dimensio adaptor 主体(实现 TaskAdaptor 接口)

**Files:**
- Create: `relay/channel/task/dimensio/adaptor.go`
- Create: `relay/channel/task/dimensio/adaptor_test.go`
- Create: `relay/channel/task/dimensio/constants.go`
- Modify: `relay/common/relay_info.go`

- [ ] **Step 1: 创建 constants.go**

创建 `relay/channel/task/dimensio/constants.go`:

```go
package dimensio

// ModelList dimensio 当前开放模型(管理员可在此基础上配模型映射,
// 把 ARK 的 doubao-seedance-* 映射到这些 dimensio 模型)。
// 依据 jimeng.dimensio.cn.md "当前开放模型"表。
var ModelList = []string{
	"jimeng-video-seedance-2.0-fast-vip",
	"jimeng-video-seedance-2.0-mini",
	"jimeng-video-seedance-2.0-vip",
}

var ChannelName = "dimensio"
```

- [ ] **Step 2: 创建 adaptor.go 骨架**

创建 `relay/channel/task/dimensio/adaptor.go`:

```go
package dimensio

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	taskcommon "github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
)

type TaskAdaptor struct {
	taskcommon.BaseBilling
	ChannelType int
	apiKey      string
	baseURL     string
}

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.ChannelType = info.ChannelType
	a.baseURL = info.ChannelBaseUrl
	a.apiKey = info.ApiKey
}

func (a *TaskAdaptor) GetModelList() []string {
	return ModelList
}

func (a *TaskAdaptor) GetChannelName() string {
	return ChannelName
}
```

- [ ] **Step 3: 实现 ValidateRequestAndSetAction**

在 `adaptor.go` 追加:

```go
// ValidateRequestAndSetAction 解析 ARK v3 原生请求体,校验后存入 context。
// 入口由计划1的 /api/v3/ 路由触发,KeySeedanceOfficialAPI=true。
func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) (taskErr *dto.TaskError) {
	if !c.GetBool(common.KeySeedanceOfficialAPI) {
		return service.TaskErrorWrapperLocal(fmt.Errorf("dimensio requires the ARK /api/v3 task API"), "invalid_request", http.StatusBadRequest)
	}
	var arkReq ArkRequest
	if err := common.UnmarshalBodyReusable(c, &arkReq); err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}
	if arkReq.Model == "" {
		return service.TaskErrorWrapperLocal(fmt.Errorf("field model is required"), "missing_model", http.StatusBadRequest)
	}
	if len(arkReq.Content) == 0 {
		return service.TaskErrorWrapperLocal(fmt.Errorf("field content is required"), "missing_content", http.StatusBadRequest)
	}

	if arkReq.Duration != nil && (*arkReq.Duration < 4 || *arkReq.Duration > 15 || *arkReq.Duration > relaycommon.MaxTaskDurationSeconds) {
		return service.TaskErrorWrapperLocal(
			fmt.Errorf("duration must be 4-15 seconds, got %d", *arkReq.Duration),
			"invalid_duration", http.StatusBadRequest,
		)
	}
	if arkReq.Duration == nil {
		arkReq.Duration = common.GetPointer(5) // dimensio 默认 5 秒
	}
	if arkReq.Resolution == "" {
		arkReq.Resolution = "720p"
	}
	arkReq.Resolution = strings.ToLower(strings.TrimSpace(arkReq.Resolution))
	if arkReq.Resolution != "720p" && arkReq.Resolution != "1080p" {
		return service.TaskErrorWrapperLocal(fmt.Errorf("resolution %s is not supported by dimensio", arkReq.Resolution), "invalid_resolution", http.StatusBadRequest)
	}
	arkReq.Ratio = strings.TrimSpace(arkReq.Ratio)
	if arkReq.Ratio == "" {
		arkReq.Ratio = "16:9"
	}
	validRatios := map[string]bool{"16:9": true, "4:3": true, "1:1": true, "3:4": true, "9:16": true, "21:9": true}
	if !validRatios[arkReq.Ratio] {
		return service.TaskErrorWrapperLocal(fmt.Errorf("ratio %s is not supported by dimensio", arkReq.Ratio), "invalid_ratio", http.StatusBadRequest)
	}
	if strings.EqualFold(strings.TrimSpace(arkReq.Ratio), "adaptive") {
		return service.TaskErrorWrapperLocal(fmt.Errorf("ratio adaptive is not supported by dimensio"), "invalid_ratio", http.StatusBadRequest)
	}
	if arkReq.Seed != nil || arkReq.CameraFixed != nil || arkReq.Watermark != nil || arkReq.GenerateAudio != nil ||
		arkReq.Frames != nil || arkReq.Draft != nil || arkReq.Priority != nil || arkReq.ExecutionExpiresAfter != nil ||
		arkReq.ReturnLastFrame != nil || arkReq.SafetyIdentifier != nil || arkReq.Tools != nil {
		return service.TaskErrorWrapperLocal(fmt.Errorf("ARK field is not supported by dimensio adaptor"), "invalid_request", http.StatusBadRequest)
	}

	prompt := ""
	for _, item := range arkReq.Content {
		if item.Type == "text" && strings.TrimSpace(item.Text) != "" {
			prompt = item.Text
			break
		}
	}
	if prompt == "" {
		return service.TaskErrorWrapperLocal(fmt.Errorf("text prompt is required"), "invalid_request", http.StatusBadRequest)
	}
	if _, err := ArkToDimensio(arkReq); err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}
	relaycommon.StoreTaskRequest(c, info, constant.TaskActionGenerate, relaycommon.TaskSubmitReq{
		Model:    arkReq.Model,
		Prompt:   prompt,
		Duration: *arkReq.Duration,
		Metadata: map[string]interface{}{"resolution": arkReq.Resolution, "ratio": arkReq.Ratio},
	})
	c.Set("dimensio_ark_request", arkReq) // 供 BuildRequestBody/EstimateBilling 读取
	return nil
}
```

> 必须调用 `StoreTaskRequest`,否则统一 task pipeline 无法保存规范化请求、动作和计费上下文。原始 ARK 请求仍单独保存在 `dimensio_ark_request` 中供协议翻译使用。

- [ ] **Step 4: 实现请求时长估算和分辨率倍率**

在 `adaptor.go` 追加:

```go
// EstimateDurationSeconds returns the normalized request duration. The central
// per_duration pipeline validates the configured DurationPrice and freezes the
// requested/billable duration snapshot.
func (a *TaskAdaptor) EstimateDurationSeconds(c *gin.Context, _ *relaycommon.RelayInfo) (int, *dto.TaskError) {
	v, ok := c.Get("dimensio_ark_request")
	req, valid := v.(ArkRequest)
	if !ok || !valid || req.Duration == nil || *req.Duration < 4 || *req.Duration > 15 || *req.Duration > relaycommon.MaxTaskDurationSeconds {
		return 0, service.TaskErrorWrapperLocal(fmt.Errorf("duration must be between 4 and 15 seconds"), "invalid_duration", http.StatusBadRequest)
	}
	return *req.Duration, nil
}

// EstimateBilling returns only non-duration multipliers. seconds/duration are
// reserved by per_duration billing. VIP 1080p retains the documented 2.5 ratio.
func (a *TaskAdaptor) EstimateBilling(c *gin.Context, _ *relaycommon.RelayInfo) map[string]float64 {
	if strings.EqualFold(strings.TrimSpace(c.GetString("task_resolution")), "1080p") {
		return map[string]float64{"resolution": 2.5}
	}
	return map[string]float64{"resolution": 1}
}
```

价格按客户端源模型名查找，不能按映射后的 Dimensio 目标模型查价。中央公式、配置 API、快照和退款日志详见 `2026-07-19-per-duration-billing.md`。

- [ ] **Step 5: 实现 BuildRequestURL + BuildRequestHeader**

在 `adaptor.go` 追加:

```go
// BuildRequestURL dimensio 提交端点(依据 jimeng.dimensio.cn.md line 16)
func (a *TaskAdaptor) BuildRequestURL(_ *relaycommon.RelayInfo) (string, error) {
	return fmt.Sprintf("%s/v1/videos/generations", a.baseURL), nil
}

// BuildRequestHeader dimensio 用 Bearer Token 鉴权
func (a *TaskAdaptor) BuildRequestHeader(_ *gin.Context, req *http.Request, _ *relaycommon.RelayInfo) error {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	return nil
}
```

- [ ] **Step 6: 实现 BuildRequestBody(翻译 ARK→dimensio + 序列化)**

在 `adaptor.go` 追加:

```go
// BuildRequestBody 把 ARK 请求翻译成 dimensio 请求并序列化。
// 关键:dimensio 的 image_file_N/video_file_N/audio_file_N 是顶层字段,
// 需把 DimensioRequest 的 map 展开合并到 JSON 顶层。
func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	v, exists := c.Get("dimensio_ark_request")
	if !exists {
		return nil, fmt.Errorf("dimensio_ark_request not found in context")
	}
	arkReq, ok := v.(ArkRequest)
	if !ok {
		return nil, fmt.Errorf("invalid ark_request type")
	}

	dim, err := ArkToDimensio(arkReq)
	if err != nil {
		return nil, errors.Wrap(err, "translate ARK to dimensio failed")
	}

	// 模型映射:info.UpstreamModelName 由 ModelMappedHelper 设置(管理员配置的映射)
	if info.UpstreamModelName != "" {
		dim.Model = info.UpstreamModelName
	}

	data, err := MarshalDimensioRequest(dim)
	if err != nil {
		return nil, errors.Wrap(err, "marshal dimensio request failed")
	}
	return bytes.NewReader(data), nil
}
```

- [ ] **Step 7: 实现 MarshalDimensioRequest(map 展开到顶层)**

在 `translate.go` 追加:

```go
import (
	// 已有 fmt, strings;追加 common
	"github.com/QuantumNous/new-api/common"
)

// MarshalDimensioRequest 把 DimensioRequest 序列化为 dimensio 期望的 JSON。
// image_file_N/video_file_N/audio_file_N 是顶层字段,需把 map 展开合并到主 map。
func MarshalDimensioRequest(dim DimensioRequest) ([]byte, error) {
	// 先序列化基础字段到 map
	baseBytes, err := common.Marshal(dim)
	if err != nil {
		return nil, err
	}
	var merged map[string]interface{}
	if err := common.Unmarshal(baseBytes, &merged); err != nil {
		return nil, err
	}
	// 合并 image_file_N / video_file_N / audio_file_N 到顶层
	for k, v := range dim.ImageFiles {
		merged[k] = v
	}
	for k, v := range dim.VideoFiles {
		merged[k] = v
	}
	for k, v := range dim.AudioFiles {
		merged[k] = v
	}
	return common.Marshal(merged)
}
```

`MarshalDimensioRequest` 的测试必须断言：输出包含 `image_file_1`、`video_file_1`、`audio_file_1`，不包含 `file_paths`，且 `functionMode`、`model`、`prompt`、`duration` 均位于 JSON 顶层。

- [ ] **Step 8: 实现 DoRequest + DoResponse**

先在 `adaptor_test.go` 写失败测试,固定公开/上游 ID 隔离:

```go
func TestDoResponseReturnsPublicTaskID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(`{"created":1709123456,"task_id":"dim-upstream","status":"pending"}`)),
	}
	info := &relaycommon.RelayInfo{PublicTaskID: "task_public", OriginModelName: "client-model"}

	upstreamID, _, taskErr := (&TaskAdaptor{}).DoResponse(c, resp, info)
	require.Nil(t, taskErr)
	assert.Equal(t, "dim-upstream", upstreamID)

	var body map[string]interface{}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &body))
	assert.Equal(t, "task_public", body["id"])
	assert.NotContains(t, recorder.Body.String(), "dim-upstream")
}
```

Run: `go test ./relay/channel/task/dimensio -run TestDoResponseReturnsPublicTaskID -count=1 -v`
Expected: FAIL,因为 `DoResponse` 尚未实现。

在 `adaptor.go` 追加:

```go
// DoRequest 委托通用 helper
func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

// DoResponse 解析 dimensio 提交响应,向 ARK 客户端返回 new-api 公共任务 ID。
func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (taskID string, taskData []byte, taskErr *dto.TaskError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		taskErr = service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
		return
	}
	_ = resp.Body.Close()

	// 先检查 dimensio 请求级错误(jimeng.dimensio.cn.md line 174: {code, message, data})
	var errResp struct {
		Code    int         `json:"code"`
		Message string      `json:"message"`
		Data    interface{} `json:"data"`
	}
	if err := common.Unmarshal(responseBody, &errResp); err == nil && errResp.Code != 0 && errResp.Message != "" {
		taskErr = a.ParseTaskError(responseBody, http.StatusBadGateway)
		return
	}

	// 解析 dimensio 成功响应 {created, task_id, status}
	var dResp DimensioSubmitResponse
	if err := common.Unmarshal(responseBody, &dResp); err != nil {
		taskErr = service.TaskErrorWrapper(errors.Wrapf(err, "body: %s", responseBody), "unmarshal_response_body_failed", http.StatusInternalServerError)
		return
	}
	if dResp.TaskID == "" {
		taskErr = service.TaskErrorWrapper(fmt.Errorf("task_id is empty"), "invalid_response", http.StatusInternalServerError)
		return
	}
	if strings.TrimSpace(info.PublicTaskID) == "" {
		taskErr = service.TaskErrorWrapper(fmt.Errorf("public task_id is empty"), "invalid_response", http.StatusInternalServerError)
		return
	}

	// 绝不把 dResp.TaskID 返回客户端;它只作为本函数返回值进入 PrivateData.UpstreamTaskID。
	c.JSON(http.StatusOK, gin.H{"id": info.PublicTaskID})
	return dResp.TaskID, responseBody, nil
}

func (a *TaskAdaptor) ParseTaskError(body []byte, statusCode int) *dto.TaskError {
	var response struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if err := common.Unmarshal(body, &response); err != nil || response.Message == "" {
		return service.TaskErrorWrapper(fmt.Errorf("%s", string(body)), "fail_to_fetch_task", statusCode)
	}
	if response.Code == -2000 {
		statusCode = http.StatusBadRequest
	} else if response.Code < 0 {
		statusCode = http.StatusBadGateway
	}
	return service.TaskErrorWrapperLocal(fmt.Errorf("%s", response.Message), strconv.Itoa(response.Code), statusCode)
}
```

错误解析只作为 `TaskErrorParser` 的示例签名；具体实现需放在 `adaptor.go`，并在 `TaskErrorParser` 接口测试中验证，避免把示例代码重复粘贴到 `DoResponse` 函数内部。

为避免把正常成功响应误判为错误，错误探测条件必须同时要求非零 `code` 和非空 `message`。渠道文档同时列出负数码与正数码（如 `1006/4001/5000`），两类都必须保留；实现使用 `common.Unmarshal`。新增测试覆盖 `-2000` 返回 400、`1006` 不丢失、成功响应返回公开 `id` 且不包含上游 ID、空 `task_id` 返回 500。

同一测试文件增加 `ParseTaskError` 表格：`-2000 -> 400`、`-2001 -> 502`、无效 JSON -> 原 HTTP 状态码和 `fail_to_fetch_task`；确认 `controller.respondTaskError` 在 ARK 标记下输出 `{"error":{"code":...,"message":...}}`。

- [ ] **Step 9: 实现 FetchTask + ParseTaskResult**

渠道文档的查询完成响应只定义 `task_id/status/progress/result.url`，没有实际 `duration`。`ParseTaskResult` 不得从查询响应提取或推断计费时长；计费始终使用提交阶段已经校验并持久化的请求时长。失败响应额外保存 `error_code`，供 ARK 错误响应使用。

在 `adaptor.go` 追加:

```go
// FetchTask 查询 dimensio 任务状态(依据 jimeng.dimensio.cn.md line 19)
func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid task_id")
	}
	uri := fmt.Sprintf("%s/v1/videos/tasks/%s", baseUrl, taskID)
	req, err := http.NewRequest(http.MethodGet, uri, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Accept", "application/json")

	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, fmt.Errorf("new proxy http client failed: %w", err)
	}
	return client.Do(req)
}

// ParseTaskResult 解析 dimensio 查询响应,映射到内部 TaskInfo(供轮询/计费用)。
// 注意:这里返回的是内部状态,不是给客户端的 ARK 格式(ARK 格式由 ConvertToOpenAIVideo/查询出口构造)。
func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	var dim DimensioTaskResponse
	if err := common.Unmarshal(respBody, &dim); err != nil {
		return nil, errors.Wrap(err, "unmarshal dimensio task result failed")
	}

	taskResult := relaycommon.TaskInfo{Code: 0}
	switch dim.Status {
	case "pending":
		taskResult.Status = model.TaskStatusQueued
		taskResult.Progress = "10%"
	case "processing":
		taskResult.Status = model.TaskStatusInProgress
		if dim.Progress > 0 && dim.Progress < 100 {
			taskResult.Progress = strconv.Itoa(dim.Progress) + "%"
		} else {
			taskResult.Progress = "50%"
		}
	case "completed":
		taskResult.Status = model.TaskStatusSuccess
		taskResult.Progress = "100%"
		taskResult.Url = dim.Result.URL
		if dim.Duration != nil && *dim.Duration >= 4 && *dim.Duration <= relaycommon.MaxTaskDurationSeconds {
			taskResult.Duration = *dim.Duration
		}
	case "failed":
		taskResult.Status = model.TaskStatusFailure
		taskResult.Progress = "100%"
		taskResult.Reason = dim.Error
		if taskResult.Reason == "" {
			taskResult.Reason = "task failed"
		}
	case "not_found":
		taskResult.Status = model.TaskStatusFailure
		taskResult.Progress = "100%"
		taskResult.Reason = "task not found or expired"
	default:
		return nil, fmt.Errorf("unknown dimensio task status: %s", dim.Status)
	}
	return &taskResult, nil
}
```

- [ ] **Step 10: 实现 ARK/OpenAI 两种查询转换**

在 `adaptor.go` 追加:

```go
// ConvertToArkVideoTask 把存储的 dimensio 任务翻译成 ARK responseTask 格式。
// 仅由 /api/v3 原生查询出口调用。
func (a *TaskAdaptor) ConvertToArkVideoTask(originTask *model.Task) ([]byte, error) {
	var dim DimensioTaskResponse
	if err := common.Unmarshal(originTask.Data, &dim); err != nil {
		return nil, errors.Wrap(err, "unmarshal dimensio task data failed")
	}
	ark, err := DimensioToArkTask(dim,
		originTask.TaskID,
		originTask.Properties.OriginModelName,
		originTask.SubmitTime,
		originTask.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return common.Marshal(ark)
}

// ConvertToOpenAIVideo 只构造 /v1/videos 的 OpenAI Video 形态,不能返回 ARK JSON。
func (a *TaskAdaptor) ConvertToOpenAIVideo(originTask *model.Task) ([]byte, error) {
	var dim DimensioTaskResponse
	if err := common.Unmarshal(originTask.Data, &dim); err != nil {
		return nil, errors.Wrap(err, "unmarshal dimensio task data failed")
	}
	video := dto.NewOpenAIVideo()
	video.ID = originTask.TaskID
	video.TaskID = originTask.TaskID
	video.Status = originTask.Status.ToVideoStatus()
	video.SetProgressStr(originTask.Progress)
	video.CreatedAt = originTask.CreatedAt
	video.CompletedAt = originTask.UpdatedAt
	video.Model = originTask.Properties.OriginModelName
	if dim.Result.URL != "" {
		video.SetMetadata("url", dim.Result.URL)
	}
	if dim.Status == "failed" || dim.Status == "not_found" {
		message := dim.Error
		if message == "" {
			message = "task not found or expired"
		}
		video.Error = &dto.OpenAIVideoError{Code: dim.ErrorCode, Message: message}
	}
	return common.Marshal(video)
}
```

Dimensio 复用 `BaseBilling.AdjustBillingOnComplete` 的 no-op 实现。成功任务保持提交时预扣额度；失败任务由公共任务轮询逻辑全额退款。

- [ ] **Step 11: 验证 adaptor 契约和计费边界**

新增 `adaptor_test.go` 的确定性测试：

- 非 ARK 标记请求返回 `invalid_request`，不会调用上游。
- 缺省 duration/ratio 被归一化为 5 秒/16:9；显式 `0`、`-1`、`adaptive`、超限 duration、未支持字段返回 400。
- 已校验的请求时长由 `EstimateDurationSeconds` 返回；720p 的 `OtherRatios` 只有 `resolution=1`，1080p 只有 `resolution=2.5`，任何 `seconds`/`duration` 倍率均被禁止；未知 resolution 返回 400。
- 映射后的目标模型必须属于文档列出的三个开放模型；fast-vip/mini 的 1080p 和未知模型均在访问上游前返回 400。
- `PriceData.AddOtherRatio` 拒绝的非正、NaN、Inf 值不会进入 adaptor 返回值。
- 查询响应不提供 `duration`，不得覆盖提交时冻结的 `DurationSource=request`、`RequestedDurationSeconds` 和 `BillableDurationSeconds`。
- 查询错误 `1057/121101` 必须保持可重试，不得把任务标记失败或触发退款。
- ARK 与 OpenAI Video 查询都必须保留 `{code,message}` 错误 envelope 的 code/message。
- 成功任务保持提交阶段按 `DurationPrice × billable duration × resolution × group ratio` 计算的预扣额度。
- 失败任务进入公共退款路径，用户、渠道和 Token 三个账本均退回预扣额度。
- 计费规则配置在客户端源模型上，使用 `BillingModePerDuration` 和 `DurationPrice`；不得要求或断言映射后目标模型的 `ModelRatio`。

Run: `go test ./relay/channel/task/dimensio/ -run 'Test(Validate|Estimate|Adjust|DoResponse|Parse)' -count=1 -v`
Expected: PASS

- [ ] **Step 12: 验证编译**

Run: `go build ./relay/channel/task/dimensio/`
Expected: 无输出

- [ ] **Step 13: Commit**

```bash
git add relay/channel/task/dimensio/adaptor.go relay/channel/task/dimensio/adaptor_test.go relay/channel/task/dimensio/constants.go relay/channel/task/dimensio/translate.go relay/common/relay_info.go
git commit -m "feat(dimensio): TaskAdaptor implementation with ARK↔dimensio translation"
```

---

## Task 5: 注册 adaptor 并接入 ARK 原生查询

**Files:**
- Modify: `relay/relay_adaptor.go`
- Modify: `relay/channel/adapter.go`
- Modify: `relay/relay_task.go`
- Modify: `relay/seedance_task.go`
- Modify: `relay/relay_task_seedance_test.go`
- Modify: `relay/relay_task_ark_error_test.go`

- [ ] **Step 1: 注册 task adaptor，不新增 API type**

修改 `relay/relay_adaptor.go`,在 `GetTaskAdaptor` 的 switch 中(import 别名 `taskdimensio`)追加 case:

```go
// 文件顶部 import 区追加:
taskdimensio "github.com/QuantumNous/new-api/relay/channel/task/dimensio"

// GetTaskAdaptor 函数的 switch 追加:
case constant.ChannelTypeDimensio:
    return &taskdimensio.TaskAdaptor{}
```

> **注意:** 插入位置在现有 `case constant.ChannelTypeMiniMax:` 等同级 case 之间,具体行号执行时 grep `case constant.ChannelType` 确认。

不要修改 `constant/api_type.go` 或 `common/api_type.go`。因为 type 59 是 task-only channel，`controller/model.go` 的 `ChannelType2APIType(59)` 应保持 `success=false`；新增测试断言 `GetTaskAdaptor(constant.TaskPlatform("59"))` 非 nil，同时 `common.ChannelType2APIType(constant.ChannelTypeDimensio)` 返回 `success=false`。

- [ ] **Step 2: 新增 ARK 原生查询转换接口**

在 `relay/channel/adapter.go` 追加,与 `OpenAIVideoConverter` 并列:

```go
type ArkVideoTaskConverter interface {
	ConvertToArkVideoTask(originTask *model.Task) ([]byte, error)
}

// TaskErrorParser lets a task adaptor preserve provider-specific request
// error codes when the upstream HTTP status is non-2xx.
type TaskErrorParser interface {
	ParseTaskError(body []byte, statusCode int) *dto.TaskError
}
```

该接口只描述 `/api/v3/contents/generations/tasks` 的响应形状。不得修改 `OpenAIVideoConverter` 的既有语义。

- [ ] **Step 3: 原生单查/列表调用转换接口并纳入 platform 59**

在 `relay/relay_task.go` 的非 2xx 分支先尝试 adaptor 的 `TaskErrorParser`，再回退到 `arkTaskErrorFromResponse`。注意这里的 `adaptor` 必须在分支前按 `platform` 取得；不能引用只在 `RelayTaskSubmit` 内部存在的局部变量：

```go
if resp != nil && (resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices) {
	responseBody, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	adaptor := GetTaskAdaptor(platform)
	if parser, ok := adaptor.(channel.TaskErrorParser); ok {
		return nil, parser.ParseTaskError(responseBody, resp.StatusCode)
	}
	return nil, arkTaskErrorFromResponse(responseBody, resp.StatusCode)
}
```

在 `relay/channel/task/dimensio/adaptor.go` 中实现 `ParseTaskError`：`-2000` 映射为 HTTP 400，任意 2xx 中携带的其他非零业务错误映射为 HTTP 502，并保留字符串 code；非 2xx 响应保留其 HTTP 状态，响应无法解析时返回 `fail_to_fetch_task`。这样 Dimensio 的负数或正数业务码都不会被通用 ARK 解析器吞掉。

在 `relay/seedance_task.go` 的基础查询条件中加入 dimensio:

```go
Where("platform IN ?", []string{
	strconv.Itoa(constant.ChannelTypeVolcEngine),
	strconv.Itoa(constant.ChannelTypeDoubaoVideo),
	strconv.Itoa(constant.ChannelTypeDimensio),
})
```

在 `seedanceTaskResponse` 读取通用 `Task.Data` 前,先按 `task.Platform` 获取 adaptor。若 adaptor 实现 `channel.ArkVideoTaskConverter`,调用 `ConvertToArkVideoTask`,再用 `common.Unmarshal` 解为 `map[string]interface{}`;否则保留现有 Doubao/VolcEngine 通用恢复逻辑。转换后只覆盖公开 ID 和缺失的补充字段：

```go
response["id"] = task.TaskID
response["status"] = seedanceTaskStatus(task.Status)
if _, ok := response["model"]; !ok || response["model"] == "" { response["model"] = modelName }
if _, ok := response["created_at"]; !ok { response["created_at"] = createdAt }
if _, ok := response["updated_at"]; !ok { response["updated_at"] = task.UpdatedAt }
```

这样即使 adaptor 转换错误地带回上游 `task_id`,公开出口也不会泄漏它；同时保留上游 ARK 字段和时间戳，符合 `2026-07-18-ark-sdk-response-compat.md`。

- [ ] **Step 4: 写回归测试**

在 `relay/relay_task_seedance_test.go` 增加确定性场景:

- dimensio 任务的 `Task.TaskID="task_public"`,上游数据含 `task_id="dim-upstream"`;单查和列表都只返回 `task_public`。
- `completed/result.url` 转为 ARK `succeeded/content.video_url`。
- `failed/error/error_code` 转为 ARK `failed/error.message/error.code`。
- 上游已有 `created_at`/`updated_at` 时保持原值；缺失时才用任务记录补全。
- platform 59 的任务出现在当前用户列表,其他用户不可见。
- `/seedance/api/v3/contents/generations/tasks` 不注册路由并返回 404（只验证路由层，不在本计划重复实现）。

- [ ] **Step 5: 验证编译 + 运行全部 dimensio/原生查询测试**

Run: `go build ./...`
Run: `go test ./relay/channel/task/dimensio/ -count=1 -v`
Run: `go test ./relay -run SeedanceTask -count=1 -v`
Run: `go test ./relay -run 'ArkError|SeedanceTask' -count=1 -v`
Expected: 编译通过,全部测试 PASS

- [ ] **Step 6: Commit**

```bash
git add relay/relay_adaptor.go relay/channel/adapter.go relay/relay_task.go relay/seedance_task.go relay/relay_task_seedance_test.go relay/relay_task_ark_error_test.go
git commit -m "feat(dimensio): register adaptor and ARK task conversion"
```

---

## Task 6: 端到端验证

**Files:**
- Create: `relay/channel/task/dimensio/e2e_test.go`
- Modify: `e2e/seedance_native_e2e_test.go`
- Create: `docs/superpowers/reports/2026-07-19-dimensio-e2e-acceptance-report.md`

- [ ] **Step 1: 全项目编译 + vet**

Run: `go build ./...`
Run: `go vet ./relay/channel/task/dimensio/ ./relay ./constant`
Expected: 无输出

- [ ] **Step 2: 全项目测试无回归**

Run: `go test ./...`
Expected: 无新增 FAIL

若仓库已有与本计划无关的失败，记录完整包名和失败摘要，不得把它们改写为“无新增 FAIL”；Dimensio 相关的 focused tests 必须全部通过。

- [ ] **Step 3: 运行 mock 自动化 E2E**

Run: `go test ./relay/channel/task/dimensio/ -count=1 -v`
Run: `go test ./e2e -run TestDimensioSeedance20MultimodalLifecycleE2E -count=1 -v`

自动化场景必须从 `/api/v3/contents/generations/tasks` 路由进入，真实执行渠道分发、模型映射、预扣、任务落库、轮询、ARK 查询转换和失败退款。Dimensio 提交/查询端点由 `httptest` mock，三个开放模型都使用提示词 + 参考图 + 参考视频 + 参考音频，并覆盖 completed、failed、`{code:-2011,message}` 终态错误和 `{code:1057,message}` 可重试限流四种状态。

- [ ] **Step 4: 生成验收报告**

将测试实际捕获的 ARK 请求、Dimensio 请求、Dimensio 成功/失败响应和 ARK 成功/失败响应写入 `docs/superpowers/reports/2026-07-19-dimensio-e2e-acceptance-report.md`。报告必须列出源模型 `DurationPrice`、`duration_source=request`、请求/计费时长、分辨率倍率、精确预扣 quota、成功保持预扣、失败全账本退款和退款日志快照，以及未调用真实上游的边界。

- [ ] **Step 5: 可选真实渠道验证(需 Dimensio API Key)**

管理员配置:
1. 通过渠道 API 新建 type=59 的 dimensio task 渠道，填 Bearer API Key，base_url=`https://jimeng.dimensio.cn`。
2. 配模型映射：`doubao-seedance-2-0-260128` → `jimeng-video-seedance-2.0-vip`；在价格设置中为客户端源模型 `doubao-seedance-2-0-260128` 配置 `per_duration` 和 USD/秒 `DurationPrice`。映射后的目标模型不参与销售查价。
3. 先验证管理员配置的模型已能通过现有模型列表/渠道能力查询；不要求前端下拉框出现 type=59。

客户端(ARK SDK,只替换配置):

```python
from volcenginesdkarkruntime import Ark

client = Ark(
    base_url="http://localhost:3000/api/v3",
    api_key="<new-api-token>",
)

# 后续仍调用原 Ark SDK 的 content generation API,不拼接 /seedance。
```

```bash
# 提交(ARK v3 格式)
curl -X POST http://localhost:3000/api/v3/contents/generations/tasks \
  -H "Authorization: Bearer <new-api-token>" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "doubao-seedance-2-0-260128",
    "content": [
      {"type":"image_url","image_url":{"url":"https://x/img.jpg"}},
      {"type":"text","text":"镜头推进"}
    ],
    "duration": 5,
    "resolution": "720p"
  }'
# 预期:new-api 翻译为 dimensio {model:"jimeng-video-seedance-2.0-vip", prompt:"镜头推进",
#        image_file_1:"...", functionMode:"first_last_frames", ratio:"16:9", duration:5}
# 客户端收到:{"id":"task_<public>"};不得出现 dimensio 上游 task_id

# 查询
curl http://localhost:3000/api/v3/contents/generations/tasks/task_<public> \
  -H "Authorization: Bearer <new-api-token>"
# 预期:new-api 翻译 dimensio {status:completed,result:{url}} → ARK {status:succeeded,content:{video_url}}
```

- [ ] **Step 6: Commit(若有遗漏改动)**

```bash
git status
# 只暂存本计划明确修改的文件,不得使用 git add -A 带入工作区其他改动。
```

端到端验收还必须覆盖：

- mock Dimensio 非 2xx `{"code":-2000,"message":"duration invalid"}` 经 `TaskErrorParser` 变成 ARK `error.code="-2000"`、HTTP 400。
- mock 轮询响应 `{"code":-2011,"message":"task expired"}` 变成可查询的 ARK failed/error 终态并触发公共退款，不能在公开查询时返回 500。
- mock 轮询响应 `1057/121101` 保持任务未完成和预扣额度，不能标记失败或退款。
- mock completed 响应只使用文档定义的 `status/progress/result.url`；成功后计费快照仍为 `duration_source=request`、请求/计费时长和 `resolution`，`OtherRatios` 不含 `seconds`/`duration`。
- `task_*` 单查、列表和 `/v1/videos/:task_id` 均不包含 `dim-upstream`；其他用户查询返回 404。

---

## Self-Review

### Spec 覆盖核对

| 需求点 | 覆盖 Task |
|---|---|
| 新增专用渠道类型 | Task 1 |
| ARK→dimensio 请求翻译(content 拆解) | Task 2 |
| functionMode 推导 | Task 2 Step 5-6 |
| 素材上限校验(9图/3视频/3音频) | Task 2 Step 7-8 |
| 内容 role 互斥、纯音频拒绝、prompt/media URL 校验 | Task 2、Task 4 |
| dimensio→ARK 响应翻译及公开 ID 隔离 | Task 3、Task 5 |
| ARK SDK 标准 `/api/v3` 基址且无私有前缀 | Task 5、Task 6 |
| dimensio 任务进入 ARK 单查/列表 | Task 5 |
| 提交响应 {id} 格式 | Task 3 Step 5-6 + Task 4 Step 8 |
| 查询响应 ARK responseTask 格式 | Task 3 Step 1-4 + Task 4 Step 10 |
| 源模型 `per_duration` 计费和 1080p 分辨率倍率 | Task 4 Step 4、Step 11；权威计费计划 `2026-07-19-per-duration-billing.md` |
| duration 4-15 校验 | Task 4 Step 3 |
| 请求 duration 计费快照和成功保持预扣 | Task 4 Step 4、Step 9-11 |
| 注册 adaptor | Task 5 |
| 端到端零改造验证 | Task 6 |

### 已核对的实现前提

1. `ChannelBaseURLs` 当前 58 已有空位，Task 1 追加 59；`ChannelTypeDummy` 显式为 60。
2. `ChannelType2APIType` 不新增 59 映射；task-only 渠道通过 `GetTaskAdaptor` 注册。
3. Dimensio 成功响应无 `code` 字段；错误响应既有负数码，也有 `1006/4001/5000` 等正数码。Task 4 以“非零 code + 非空 message”识别请求级错误。
4. ARK 查询已有路由和 `RelaySeedanceTaskFetch` 由计划 1 提供，本计划只修改响应转换和 platform 条件。
5. 前端渠道配置 UI 不在本计划范围，type=59 通过管理员 API 配置。

### 类型一致性

- `ArkRequest` / `DimensioRequest` / `ArkContent` / `ArkMedia`:Task 2 Step 3 定义,Task 4 使用 ✓
- `ArkToDimensio(ark ArkRequest) (DimensioRequest, error)`:Task 2 定义,Task 4 Step 6 调用 ✓
- `deriveFunctionMode(content []ArkContent) string`:Task 2 定义,Task 2 测试 ✓
- `DimensioTaskResponse` / `ArkTaskResponse` / `DimensioToArkTask`:Task 3 定义,Task 4 Step 10 调用,显式传入公开 ID ✓
- `MarshalDimensioRequest(dim DimensioRequest) ([]byte, error)`:Task 4 Step 7 定义,Task 4 Step 6 调用 ✓
- `TaskAdaptor` 实现 `TaskAdaptor` + `OpenAIVideoConverter` + `ArkVideoTaskConverter`;ARK/OpenAI 响应语义分离 ✓
- 查询响应不读取未定义的 `duration`；任务成功保持请求时计费快照，失败全额退款 ✓

无类型不一致问题。

### 计划完整性

- 所有新接口均列出创建/修改文件、失败测试、通过测试和提交边界。
- 不新增 API type 59，避免 `controller/model.go` 初始化阶段调用 nil 通用 adaptor。
- 计费路径覆盖请求校验 → 源模型 `DurationPrice` 查价 → 请求时长快照 → 非时长 `OtherRatios` → 预扣 → 成功保持预扣/失败退款。
- 上游错误覆盖 adaptor 解析和 ARK 错误出口，避免把 Dimensio 正负业务码降级为通用 `fail_to_fetch_task`。

---

## 与前两份计划的协调

| 交叉点 | 协调方式 |
|---|---|
| 入口路由 `/api/v3/` | **计划1 负责**,本计划复用,不新增 `/seedance` 前缀或兼容别名 |
| `KeySeedanceOfficialAPI` 标记 | **计划1 定义**,本计划用它限制 dimensio 提交只接受标准 ARK 原生入口 |
| `GetTaskAdaptor` switch | 计划1不改它(只改 adaptor 内部),本计划 Task 5 新增 dimensio case,无冲突 |
| doubao adaptor | 本计划不改 doubao,新建独立 dimensio 包,零冲突 |
| 公开任务 ID | **计划1 定义生成与查询语义**,本计划提交响应和双向转换只使用 `Task.TaskID`/`info.PublicTaskID` |
| ARK 任务列表 platform | 本计划 Task 5 把 59 加入既有 45/54 条件 |
| Seedance 计费计划 | `2026-07-19-per-duration-billing.md` 为权威来源；type 59 写入请求时长专用快照和分辨率倍率，不写 `seconds`/`duration` OtherRatio，不按目标模型 `ModelRatio` 查价 |

**执行顺序:** 计划1(入口与原生查询)→ 本计划(Dimensio adaptor、task-only 注册、协议接入)→ `2026-07-19-per-duration-billing.md`(统一时长计费)。协议决策继续由本文定义；计费配置、公式、快照、日志和验收以时长计费计划为准。

---

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-07-18-dimensio-translator.md`. Implementation must use `superpowers:subagent-driven-development` or `superpowers:executing-plans`, then run the focused verification commands in Task 6 before claiming completion.
