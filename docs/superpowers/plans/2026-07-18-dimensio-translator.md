# Dimensio 协议翻译 Adaptor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 新增 dimensio 渠道,作为"协议翻译网关"——客户端用火山方舟(ARK)v3 协议发请求,new-api 把 ARK 数据结构翻译成 dimensio 原生数据结构发给上游,再把 dimensio 响应翻译回 ARK v3 格式返回客户端。让 ARK SDK 用户切换 dimensio 后端时**客户端零改动**。

**Architecture:** 复用计划 1(`2026-07-18-ark-native-compat.md`)的 `/seedance/api/v3/` 入口——客户端侧的 ARK v3 协议是共享的。仅在上游侧新增独立的 `dimensio` task adaptor,实现 `TaskAdaptor` 接口 + `OpenAIVideoConverter` 接口。渠道路由由现有 `Distribute` 中间件按模型映射选渠道:客户端发 `doubao-seedance-*`,管理员配置映射到 `jimeng-video-*`,选中 dimensio 渠道后 `GetTaskAdaptor` 返回 dimensio adaptor,adaptor 内部完成 ARK↔dimensio 双向翻译。

**Tech Stack:** Go 1.22+, Gin, GORM v2, testify(require+assert),JSON 统一走 `common.*`。按秒计费(`OtherRatio: seconds`)。

**输入文档:** `jimeng.dimensio.cn.md`(项目根目录,dimensio 官方接入规格)

**依赖:** 计划 1(`2026-07-18-ark-native-compat.md`)的入口路由 + `KeySeedanceOfficialAPI` 标记必须先落地。本计划假设入口已存在,只做 dimensio adaptor。

---

## Scope

✅ 新增 `ChannelTypeDimensio`(59)渠道类型
✅ 新建 `relay/channel/task/dimensio/` adaptor,实现 ARK↔dimensio 双向翻译
✅ 在 `GetTaskAdaptor` 注册 dimensio
✅ 按秒计费(OtherRatio: seconds)
✅ 实现 `ConvertToOpenAIVideo`(响应方向翻译回 ARK 格式)
❌ 入口路由(计划 1 负责)
❌ 前端渠道配置 UI(后续独立 PR,管理员可通过 API 先配)
❌ chat/embeddings/responses(仅视频)

---

## 协议翻译映射(实施依据)

### 请求方向:ARK v3 → dimensio

| ARK v3 字段 | dimensio 字段 | 翻译逻辑 |
|---|---|---|
| `model` | `model` | 渠道模型映射(`doubao-seedance-2-0-260128` → `jimeng-video-seedance-2.0-vip`) |
| `content[].type=text` 的 text | `prompt` | 提取第一个 text 项 |
| `content[].type=image_url` 的 url(按顺序) | `image_file_1`..`image_file_N` + `file_paths[]` | 枚举填充(N≤9) |
| `content[].type=video_url` 的 url | `video_file_1`..`video_file_N` | 枚举填充(N≤3,总时长≤15s) |
| `content[].type=audio_url` 的 url | `audio_file_1`..`audio_file_N` | 枚举填充(N≤3) |
| `ratio` | `ratio` | 直传 |
| `resolution` | `resolution` | 直传 |
| `duration` | `duration` | 直传(整数秒 4-15) |
| (无,需推导) | `functionMode` ⚠️**必填** | 从 content 推导(见下) |
| `intelligent_ratio`(若客户端传) | `intelligent_ratio` | 直传(可选) |
| `face_grid`(若客户端传) | `face_grid` | 直传(可选) |
| `seed`/`camera_fixed`/`watermark`/`generate_audio` | (无对应) | **丢弃**(dimensio 不支持,记 debug 日志) |

**`functionMode` 推导规则**(ARK 无此字段,dimensio 必填):
```
imageCount = content[] 中 image_url 项数
hasVideoOrAudio = content[] 含 video_url 或 audio_url
if hasVideoOrAudio or imageCount > 2:
    functionMode = "omni_reference"    // 全能参考,支持图/视频/音频,素材总数≤12
else:
    functionMode = "first_last_frames"  // 文生/图生/首尾帧,图≤2
```

### 响应方向:dimensio → ARK v3

**提交响应:**
| dimensio | ARK v3 | 翻译 |
|---|---|---|
| `{created, task_id, status:"pending"}` | `{id}` | `id = task_id` |

**查询响应(dimensio → ARK responseTask):**
| dimensio | ARK responseTask | 翻译 |
|---|---|---|
| `task_id` | `id` | 直传 |
| `status: pending` | `status: pending` | 直传 |
| `status: processing` | `status: running` | 映射 |
| `status: completed` | `status: succeeded` | 映射 |
| `status: failed` | `status: failed` | 直传 |
| `status: not_found` | `status: failed` + `error.message: "task not found"` | 映射 |
| `progress` | (丢弃) | ARK 无此字段 |
| `result.url` | `content.video_url` | 嵌入 content 对象 |
| `error` | `error.message` | 移入 error |
| `error_code` | `error.code` | 移入 error |
| (缺失) | `model` | 从任务记录 `OriginModelName` 补 |
| (缺失) | `created_at` | 从 `task.SubmitTime` 补 |
| (缺失) | `updated_at` | 从 `task.UpdatedAt` 补 |
| (缺失) | `usage.completion_tokens` | 留空(dimensio 无 token,按秒计费不依赖它) |

---

## File Structure

| 文件 | 责任 |
|---|---|
| `constant/channel.go` | 新增 `ChannelTypeDimensio = 59`,Dummy 顺移到 60 |
| `constant/channel.go` | `ChannelBaseURLs` 追加 dimensio 默认 URL,显示名映射 |
| `relay/relay_adaptor.go` | `GetTaskAdaptor` 注册 dimensio case |
| `relay/channel/task/dimensio/constants.go` | 模型列表 + 计费辅助 |
| `relay/channel/task/dimensio/adaptor.go` | `TaskAdaptor` 实现(核心:翻译逻辑) |
| `relay/channel/task/dimensio/adaptor_test.go` | 翻译逻辑测试 |
| `relay/channel/task/dimensio/translate.go` | ARK↔dimensio 纯函数翻译(易测试) |
| `relay/channel/task/dimensio/translate_test.go` | 翻译函数表驱动测试 |

---

## Task 1: 新增渠道类型常量

**Files:**
- Modify: `constant/channel.go`

- [ ] **Step 1: 新增 `ChannelTypeDimensio`**

修改 `constant/channel.go`,在 `ChannelTypeAdvancedCustom = 58` 之后、`ChannelTypeDummy` 之前插入:

```go
	ChannelTypeAdvancedCustom = 58
	ChannelTypeDimensio       = 59 // dimensio 视频生成(ARK v3 协议翻译网关)
	ChannelTypeDummy          // this one is only for count, do not add any channel after this
```

> **注意:** 原本 `ChannelTypeDummy` 是 59,现在顺移到 60。因为它是 iota 自增,插在它前面的常量会让它自动 +1。`controller/model.go:96` 的 `for i:=1; i<=ChannelTypeDummy` 循环会自动覆盖新值。

- [ ] **Step 2: 追加 ChannelBaseURLs 和显示名**

在 `constant/channel.go` 的 `ChannelBaseURLs` slice 末尾(对应 index 59)追加:

```go
	// index 58 AdvancedCustom
	"",                                       // 58
	"https://jimeng.dimensio.cn",             // 59 Dimensio
```

> **注意:** 需核对 `ChannelBaseURLs` 当前最后一个 index 是不是 58(AdvancedCustom)。如果 AdvancedCustom 之前已有对应空字符串,执行时以实际为准。关键是 dimensio 在 index 59。

在渠道类型名称映射(类似 `ChannelTypeVolcEngine: "VolcEngine"`)追加:

```go
	constant.ChannelTypeDimensio: "Dimensio",
```

- [ ] **Step 3: 验证编译**

Run: `go build ./constant/`
Expected: 无输出

- [ ] **Step 4: 验证 controller/model.go 的循环仍正确**

Run: `go test ./controller/ -run TestModel -v 2>&1 | head -20`
Expected: 无 panic(Dummy 顺移后循环边界自动正确)

- [ ] **Step 5: Commit**

```bash
git add constant/channel.go
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
			{Type: "image_url", ImageURL: &ArkMedia{URL: "https://x/img1.jpg"}},
			{Type: "image_url", ImageURL: &ArkMedia{URL: "https://x/img2.jpg"}},
			{Type: "video_url", VideoURL: &ArkMedia{URL: "https://x/ref1.mp4"}},
			{Type: "video_url", VideoURL: &ArkMedia{URL: "https://x/ref2.mp4"}},
			{Type: "video_url", VideoURL: &ArkMedia{URL: "https://x/ref3.mp4"}},
			{Type: "audio_url", AudioURL: &ArkMedia{URL: "https://x/bg.mp3"}},
			{Type: "text", Text: "镜头缓慢推进"},
		},
		Duration:   10,
		Resolution: "720p",
		Ratio:      "16:9",
	}

	dim, err := ArkToDimensio(arkReq)
	require.NoError(t, err)

	// 基础字段直传
	assert.Equal(t, "doubao-seedance-2-0-260128", dim.Model) // 映射前保留原名,映射在 adaptor 层做
	assert.Equal(t, "镜头缓慢推进", dim.Prompt)
	assert.Equal(t, 10, dim.Duration)
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

	// file_paths 同步包含所有图片 URL(dimensio 文档:也兼容 file_paths 数组)
	assert.Equal(t, []string{"https://x/img1.jpg", "https://x/img2.jpg"}, dim.FilePaths)
}
```

- [ ] **Step 2: 运行,验证失败**

Run: `go test ./relay/channel/task/dimensio/ -run TestArkToDimensioMultimodal -v`
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
	Duration   int           `json:"duration,omitempty"`
	// ARK 特有但 dimensio 不支持的字段(解析后丢弃,仅用于透传不报错)
	Seed           *int   `json:"seed,omitempty"`
	CameraFixed    *bool  `json:"camera_fixed,omitempty"`
	Watermark      *bool  `json:"watermark,omitempty"`
	GenerateAudio  *bool  `json:"generate_audio,omitempty"`
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
	Role     string    `json:"role,omitempty"` // first_frame/last_frame 等,dimensio 忽略
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
	Duration        int               `json:"duration,omitempty"`
	IntelligentRatio *bool            `json:"intelligent_ratio,omitempty"`
	FaceGrid         *bool            `json:"face_grid,omitempty"`
	FilePaths       []string          `json:"file_paths,omitempty"` // 图片 URL 数组(与 image_file_N 冗余,dimensio 都接受)
	ImageFiles      map[string]string `json:"-"`                    // image_file_1..9,序列化时合并到顶层
	VideoFiles      map[string]string `json:"-"`                    // video_file_1..3
	AudioFiles      map[string]string `json:"-"`                    // audio_file_1..3
}

// ===== ARK↔dimensio 翻译纯函数 =====

// ArkToDimensio 把 ARK v3 请求翻译成 dimensio 请求。
// 纯函数,不做 HTTP/模型映射(模型映射在 adaptor 层用 info.UpstreamModelName 做)。
func ArkToDimensio(ark ArkRequest) (DimensioRequest, error) {
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
			if item.ImageURL != nil && item.ImageURL.URL != "" {
				imgIdx++
				if imgIdx > 9 {
					return DimensioRequest{}, fmt.Errorf("too many images: dimensio allows at most 9 (image_file_1..9)")
				}
				key := fmt.Sprintf("image_file_%d", imgIdx)
				dim.ImageFiles[key] = item.ImageURL.URL
				dim.FilePaths = append(dim.FilePaths, item.ImageURL.URL)
			}
		case "video_url":
			if item.VideoURL != nil && item.VideoURL.URL != "" {
				vidIdx++
				if vidIdx > 3 {
					return DimensioRequest{}, fmt.Errorf("too many videos: dimensio allows at most 3 (video_file_1..3)")
				}
				dim.VideoFiles[fmt.Sprintf("video_file_%d", vidIdx)] = item.VideoURL.URL
			}
		case "audio_url":
			if item.AudioURL != nil && item.AudioURL.URL != "" {
				audIdx++
				if audIdx > 3 {
					return DimensioRequest{}, fmt.Errorf("too many audios: dimensio allows at most 3 (audio_file_1..3)")
				}
				dim.AudioFiles[fmt.Sprintf("audio_file_%d", audIdx)] = item.AudioURL.URL
			}
		}
	}

	dim.FunctionMode = deriveFunctionMode(imgIdx, vidIdx, audIdx)
	return dim, nil
}

// deriveFunctionMode 从素材组成推导 dimensio functionMode(ARK 无此字段,dimensio 必填)。
// - 含视频/音频 或 图片>2:omni_reference(全能参考,素材总数≤12)
// - 其他:first_last_frames(文生/图生/首尾帧,图≤2)
func deriveFunctionMode(imgCount, vidCount, audCount int) string {
	if vidCount > 0 || audCount > 0 || imgCount > 2 {
		return "omni_reference"
	}
	return "first_last_frames"
}
```

- [ ] **Step 4: 运行,验证通过**

Run: `go test ./relay/channel/task/dimensio/ -run TestArkToDimensioMultimodal -v`
Expected: PASS

- [ ] **Step 5: 写测试(functionMode 推导矩阵)**

在 `translate_test.go` 追加:

```go
// TestDeriveFunctionModeMatrix 覆盖 functionMode 推导的所有组合
func TestDeriveFunctionModeMatrix(t *testing.T) {
	cases := []struct {
		name              string
		img, vid, aud     int
		expectedMode      string
	}{
		{"text_only", 0, 0, 0, "first_last_frames"},
		{"single_image", 1, 0, 0, "first_last_frames"},
		{"two_images", 2, 0, 0, "first_last_frames"},
		{"three_images", 3, 0, 0, "omni_reference"},     // 图片>2 → omni
		{"image_plus_video", 1, 1, 0, "omni_reference"}, // 含视频 → omni
		{"image_plus_audio", 1, 0, 1, "omni_reference"}, // 含音频 → omni
		{"video_only", 0, 1, 0, "omni_reference"},
		{"audio_only", 0, 0, 1, "omni_reference"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expectedMode, deriveFunctionMode(tc.img, tc.vid, tc.aud))
		})
	}
}
```

- [ ] **Step 6: 运行,验证通过**

Run: `go test ./relay/channel/task/dimensio/ -run TestDeriveFunctionModeMatrix -v`
Expected: PASS(逻辑已在 Step 3 实现)

- [ ] **Step 7: 写测试(素材超限报错)**

在 `translate_test.go` 追加:

```go
// TestArkToDimensioRejectsTooManyImages 验证图片超 9 张报错
func TestArkToDimensioRejectsTooManyImages(t *testing.T) {
	content := make([]ArkContent, 0, 11)
	for i := 0; i < 10; i++ { // 10 张图,超 9 张上限
		content = append(content, ArkContent{Type: "image_url", ImageURL: &ArkMedia{URL: "https://x/img.jpg"}})
	}
	content = append(content, ArkContent{Type: "text", Text: "hi"})

	_, err := ArkToDimensio(ArkRequest{Model: "m", Content: content})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "too many images")
}

func TestArkToDimensioRejectsTooManyVideos(t *testing.T) {
	content := []ArkContent{
		{Type: "video_url", VideoURL: &ArkMedia{URL: "v1"}},
		{Type: "video_url", VideoURL: &ArkMedia{URL: "v2"}},
		{Type: "video_url", VideoURL: &ArkMedia{URL: "v3"}},
		{Type: "video_url", VideoURL: &ArkMedia{URL: "v4"}}, // 第4个,超 3 个上限
		{Type: "text", Text: "hi"},
	}
	_, err := ArkToDimensio(ArkRequest{Model: "m", Content: content})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "too many videos")
}
```

- [ ] **Step 8: 运行,验证通过**

Run: `go test ./relay/channel/task/dimensio/ -run "TestArkToDimensioRejects" -v`
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
		{"pending", "pending", "pending", ""},
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
			ark, err := DimensioToArkTask(dim, "doubao-seedance-2-0-260128", int64(1710000000), int64(1710000100))
			require.NoError(t, err)

			assert.Equal(t, "dim-123", ark.ID)
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

Run: `go test ./relay/channel/task/dimensio/ -run TestDimensioToArkTaskResponse -v`
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
// model/createdAt/updatedAt 由任务记录补全(dimensio 响应不含这些)。
func DimensioToArkTask(dim DimensioTaskResponse, modelName string, createdAt, updatedAt int64) (ArkTaskResponse, error) {
	ark := ArkTaskResponse{
		ID:        dim.TaskID,
		Model:     modelName,
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
	}

	switch dim.Status {
	case "pending":
		ark.Status = "pending"
	case "processing":
		ark.Status = "running"
	case "completed":
		ark.Status = "succeeded"
		ark.Content.VideoURL = dim.Result.URL
	case "failed":
		ark.Status = "failed"
		ark.Error = &ArkError{Code: dim.ErrorCode, Message: dim.Error}
	case "not_found":
		ark.Status = "failed"
		ark.Error = &ArkError{Code: dim.ErrorCode, Message: "task not found or expired"}
		if ark.Error.Message == "" || dim.Status == "not_found" {
			ark.Error.Message = "task not found or expired"
		}
	default:
		ark.Status = "running" // 未知状态按运行中处理
	}

	return ark, nil
}
```

- [ ] **Step 4: 运行,验证通过**

Run: `go test ./relay/channel/task/dimensio/ -run TestDimensioToArkTaskResponse -v`
Expected: PASS

- [ ] **Step 5: 写测试(提交响应翻译 {task_id} → {id})**

在 `translate_test.go` 追加:

```go
// TestDimensioSubmitToArkID 验证提交响应 dimensio {task_id,...} → ARK {id}
func TestDimensioSubmitToArkID(t *testing.T) {
	dim := DimensioSubmitResponse{
		Created: 1709123456,
		TaskID:  "dim-abc",
		Status:  "pending",
	}
	id := DimensioSubmitToArkID(dim)
	assert.Equal(t, "dim-abc", id)
}
```

- [ ] **Step 6: 实现并验证**

在 `translate.go` 追加:

```go
// DimensioSubmitToArkID 提交响应翻译:dimensio task_id → ARK id(ARK 提交响应只有 {id})
func DimensioSubmitToArkID(dim DimensioSubmitResponse) string {
	return dim.TaskID
}
```

Run: `go test ./relay/channel/task/dimensio/ -run TestDimensioSubmitToArkID -v`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add relay/channel/task/dimensio/translate.go relay/channel/task/dimensio/translate_test.go
git commit -m "feat(dimensio): dimensio→ARK response translation functions"
```

---

## Task 4: dimensio adaptor 主体(实现 TaskAdaptor 接口)

**Files:**
- Create: `relay/channel/task/dimensio/adaptor.go`
- Create: `relay/channel/task/dimensio/constants.go`

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

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	taskcommon "github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"

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
// 入口由计划1的 /seedance/api/v3/ 路由触发,KeySeedanceOfficialAPI=true。
func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) (taskErr *dto.TaskError) {
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

	// dimensio duration 限制:4-15 整数秒(jimeng.dimensio.cn.md line 68)
	if arkReq.Duration != 0 && (arkReq.Duration < 4 || arkReq.Duration > 15) {
		return service.TaskErrorWrapperLocal(
			fmt.Errorf("duration must be 4-15 seconds, got %d", arkReq.Duration),
			"invalid_duration", http.StatusBadRequest,
		)
	}
	if arkReq.Duration == 0 {
		arkReq.Duration = 5 // dimensio 默认 5 秒
	}

	info.Action = constant.TaskActionGenerate
	c.Set("ark_request", arkReq) // 供 BuildRequestBody 读取
	return nil
}
```

- [ ] **Step 4: 实现 EstimateBilling(按秒计费)**

在 `adaptor.go` 追加:

```go
// EstimateBilling 按秒计费:返回 {seconds: duration} 作为 OtherRatio。
// 管理员配置 ModelRatio = 单秒基准价,最终 quota = ModelRatio × seconds × GroupRatio。
// 分辨率差异(720p vs 1080p)由管理员通过不同模型映射区分(不同模型不同 ModelRatio)。
func (a *TaskAdaptor) EstimateBilling(c *gin.Context, info *relaycommon.RelayInfo) map[string]float64 {
	v, exists := c.Get("ark_request")
	if !exists {
		return nil
	}
	arkReq, ok := v.(ArkRequest)
	if !ok {
		return nil
	}
	seconds := arkReq.Duration
	if seconds <= 0 {
		seconds = 5
	}
	return map[string]float64{"seconds": float64(seconds)}
}
```

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
	v, exists := c.Get("ark_request")
	if !exists {
		return nil, fmt.Errorf("ark_request not found in context")
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

- [ ] **Step 8: 实现 DoRequest + DoResponse**

在 `adaptor.go` 追加:

```go
// DoRequest 委托通用 helper
func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

// DoResponse 解析 dimensio 提交响应,翻译成 ARK {id} 格式返回客户端。
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
		taskErr = service.TaskErrorWrapper(
			fmt.Errorf("%s", errResp.Message),
			fmt.Sprintf("%d", errResp.Code),
			http.StatusInternalServerError,
		)
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

	// ARK 入口:返回 ARK 原生 {id} 格式(与 doubao adaptor 的 Seedance 分支一致)
	if c.GetBool(common.KeySeedanceOfficialAPI) {
		c.JSON(http.StatusOK, gin.H{"id": dResp.TaskID})
	} else {
		// 非 ARK 入口:返回 OpenAIVideo 兼容格式
		ov := dto.NewOpenAIVideo()
		ov.ID = info.PublicTaskID
		ov.TaskID = info.PublicTaskID
		ov.CreatedAt = dResp.Created
		ov.Model = info.OriginModelName
		c.JSON(http.StatusOK, ov)
	}
	return dResp.TaskID, responseBody, nil
}
```

- [ ] **Step 9: 实现 FetchTask + ParseTaskResult**

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
	case "failed":
		taskResult.Status = model.TaskStatusFailure
		taskResult.Progress = "100%"
		taskResult.Reason = dim.Error
	case "not_found":
		taskResult.Status = model.TaskStatusFailure
		taskResult.Progress = "100%"
		taskResult.Reason = "task not found or expired"
	default:
		taskResult.Status = model.TaskStatusInProgress
		taskResult.Progress = "30%"
	}
	return &taskResult, nil
}
```

- [ ] **Step 10: 实现 ConvertToOpenAIVideo + AdjustBillingOnComplete**

在 `adaptor.go` 追加:

```go
// ConvertToOpenAIVideo 把存储的 dimensio 任务翻译成 ARK responseTask 格式返回客户端。
// 这是 ARK SDK 查询时收到的响应(通过计划1的查询出口调用)。
func (a *TaskAdaptor) ConvertToOpenAIVideo(originTask *model.Task) ([]byte, error) {
	// task.Data 存的是 dimensio 查询响应(轮询时写入)
	var dim DimensioTaskResponse
	if err := common.Unmarshal(originTask.Data, &dim); err != nil {
		return nil, errors.Wrap(err, "unmarshal dimensio task data failed")
	}

	ark, err := DimensioToArkTask(dim,
		originTask.Properties.OriginModelName,
		originTask.SubmitTime,
		originTask.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return common.Marshal(ark)
}

// AdjustBillingOnComplete 按 BaseBilling no-op:dimensio 按秒计费,
// 预扣阶段已用 duration 精确估算,完成时无需重算(dimensio 不返回 token/实际时长)。
// 返回 0 保持预扣额。若未来 dimensio 响应返回实际计费秒数,可在此重算。
func (a *TaskAdaptor) AdjustBillingOnComplete(_ *model.Task, _ *relaycommon.TaskInfo) int {
	return 0
}
```

- [ ] **Step 11: 验证编译**

Run: `go build ./relay/channel/task/dimensio/`
Expected: 无输出

- [ ] **Step 12: Commit**

```bash
git add relay/channel/task/dimensio/adaptor.go relay/channel/task/dimensio/constants.go relay/channel/task/dimensio/translate.go
git commit -m "feat(dimensio): TaskAdaptor implementation with ARK↔dimensio translation"
```

---

## Task 5: 注册 dimensio adaptor

**Files:**
- Modify: `relay/relay_adaptor.go`

- [ ] **Step 1: 在 GetTaskAdaptor 注册**

修改 `relay/relay_adaptor.go`,在 `GetTaskAdaptor` 的 switch 中(import 别名 `taskdimensio`)追加 case:

```go
// 文件顶部 import 区追加:
taskdimensio "github.com/QuantumNous/new-api/relay/channel/task/dimensio"

// GetTaskAdaptor 函数的 switch 追加:
case constant.ChannelTypeDimensio:
    return &taskdimensio.TaskAdaptor{}
```

> **注意:** 插入位置在现有 `case constant.ChannelTypeMiniMax:` 等同级 case 之间,具体行号执行时 grep `case constant.ChannelType` 确认。

- [ ] **Step 2: 验证编译 + 运行全部 dimensio 测试**

Run: `go build ./...`
Run: `go test ./relay/channel/task/dimensio/ -v`
Expected: 编译通过,全部测试 PASS

- [ ] **Step 3: Commit**

```bash
git add relay/relay_adaptor.go
git commit -m "feat(dimensio): register dimensio TaskAdaptor in GetTaskAdaptor"
```

---

## Task 6: 端到端验证

- [ ] **Step 1: 全项目编译 + vet**

Run: `go build ./...`
Run: `go vet ./relay/channel/task/dimensio/`
Expected: 无输出

- [ ] **Step 2: 全项目测试无回归**

Run: `go test ./... 2>&1 | tail -30`
Expected: 无新增 FAIL

- [ ] **Step 3: 端到端手动验证(需 dimensio 渠道)**

管理员配置:
1. 新建 dimensio 渠道(type=59),填 dimensio API Key,base_url=`https://jimeng.dimensio.cn`
2. 配模型映射:`doubao-seedance-2-0-260128` → `jimeng-video-seedance-2.0-vip`

客户端(ARK SDK,base_url 指向 new-api/seedance):

```bash
# 提交(ARK v3 格式)
curl -X POST http://localhost:3000/seedance/api/v3/contents/generations/tasks \
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
#        image_file_1:"...", functionMode:"first_last_frames", duration:5}
# 客户端收到:{"id":"dim-xxx"}

# 查询
curl http://localhost:3000/seedance/api/v3/contents/generations/tasks/dim-xxx \
  -H "Authorization: Bearer <new-api-token>"
# 预期:new-api 翻译 dimensio {status:completed,result:{url}} → ARK {status:succeeded,content:{video_url}}
```

- [ ] **Step 4: Commit(若有遗漏改动)**

```bash
git status
# 若有未提交改动:
git add -A && git commit -m "test(dimensio): full regression + e2e verification"
```

---

## Self-Review

### Spec 覆盖核对

| 需求点 | 覆盖 Task |
|---|---|
| 新增专用渠道类型 | Task 1 |
| ARK→dimensio 请求翻译(content 拆解) | Task 2 |
| functionMode 推导 | Task 2 Step 5-6 |
| 素材上限校验(9图/3视频/3音频) | Task 2 Step 7-8 |
| dimensio→ARK 响应翻译 | Task 3 |
| 提交响应 {id} 格式 | Task 3 Step 5-6 + Task 4 Step 8 |
| 查询响应 ARK responseTask 格式 | Task 3 Step 1-4 + Task 4 Step 10 |
| 按秒计费 | Task 4 Step 4 |
| duration 4-15 校验 | Task 4 Step 3 |
| 注册 adaptor | Task 5 |
| 端到端零改造验证 | Task 6 |

### 待确认项(执行时核实)

1. **`ChannelBaseURLs` index 59 对齐**:Task 1 Step 2 需核对 slice 当前长度,确保 dimensio 在 index 59
2. **渠道类型名称映射位置**:Task 1 Step 2 的 `ChannelTypeDimensio: "Dimensio"` 插入位置需 grep 现有映射
3. **`GetTaskAdaptor` case 插入位置**:Task 5 Step 1 需 grep `case constant.ChannelType` 确认
4. **前端渠道配置 UI**:本计划不含,管理员需通过 API 配置 type=59 渠道(或后续 PR 补 UI)
5. **dimensio 请求级错误码识别**:Task 4 Step 8 的 `errResp.Code != 0` 判断需核实 dimensio 成功响应是否真的不含 code 字段(文档 line 125-131 成功响应无 code,line 174-179 错误响应有 code)

### 类型一致性

- `ArkRequest` / `DimensioRequest` / `ArkContent` / `ArkMedia`:Task 2 Step 3 定义,Task 4 使用 ✓
- `ArkToDimensio(ark ArkRequest) (DimensioRequest, error)`:Task 2 定义,Task 4 Step 6 调用 ✓
- `deriveFunctionMode(img, vid, aud int) string`:Task 2 定义,Task 2 测试 ✓
- `DimensioTaskResponse` / `ArkTaskResponse` / `DimensioToArkTask`:Task 3 定义,Task 4 Step 10 调用 ✓
- `MarshalDimensioRequest(dim DimensioRequest) ([]byte, error)`:Task 4 Step 7 定义,Task 4 Step 6 调用 ✓
- `TaskAdaptor` 实现 `TaskAdaptor` 接口(10 方法)+ `OpenAIVideoConverter`(ConvertToOpenAIVideo)✓

无类型不一致问题。

---

## 与前两份计划的协调

| 交叉点 | 协调方式 |
|---|---|
| 入口路由 `/seedance/api/v3/` | **计划1 负责**,本计划复用,不重复实现 |
| `KeySeedanceOfficialAPI` 标记 | **计划1 定义**,本计划在 DoResponse 读取它决定返回 {id} 还是 OpenAIVideo |
| `GetTaskAdaptor` switch | 计划1不改它(只改 adaptor 内部),本计划 Task 5 新增 dimensio case,无冲突 |
| doubao adaptor | 本计划不改 doubao,新建独立 dimensio 包,零冲突 |

**执行顺序:** 计划1(入口)→ 本计划(dimensio adaptor)→ 计划2(计费)。三者产出三个独立 PR。

---

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-07-18-dimensio-translator.md`. 见对话中的执行选项(subagent-driven vs inline)。
