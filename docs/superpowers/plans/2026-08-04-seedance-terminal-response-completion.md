# Seedance 终态任务响应强制补全实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 所有 Seedance 渠道返回并审计结构完整、字段稳定的火山引擎 Ark 视频终态任务响应。

**Architecture:** 保留渠道适配器现有协议转换职责，在通用 `seedanceTaskResponse` 末端增加集中式响应补全器。补全器按“上游响应、用户请求快照、计费快照、固定默认值”的顺序填充字段；E2E 种子逐条校验最终用户响应，而不是只检查响应存在。

**执行偏差记录：** 首轮 E2E 发现后台轮询审计直接持久化适配器转换结果，绕过 `seedanceTaskResponse`。最终实现将统一规范化器下沉为 `service.NormalizeSeedanceTaskResponse`，并由 relay 查询和 service 轮询审计共同调用；Seedance 轮询审计固定保存 Ark 结构，不再按创建路径保存 OpenAI 简化结构。

**Tech Stack:** Go 1.22、Gin、GORM、`common` JSON 封装、Testify、MySQL/SQLite 测试夹具、现有 Ark 视频矩阵种子。

---

## 文件结构

- Create: `relay/seedance_task_response_completion.go`：集中处理 Ark 终态字段来源优先级、类型边界和默认值。
- Modify: `relay/seedance_task.go`：在状态、内容、时间和 Token 用量规范化后调用补全器。
- Modify: `relay/relay_task_seedance_test.go`：覆盖简化响应、来源优先级、默认值、非法值和失败响应。
- Modify: `cmd/ark-video-material-seed/main.go`：对每条 E2E 任务保存的最终用户响应执行完整合同校验。
- Modify: `cmd/ark-video-material-seed/main_test.go`：覆盖成功和失败合同校验器以及全部 mock 轮询路径。
- Modify: `docs/superpowers/reports/2026-08-04-ark-sdk-video-material-matrix-reimport-acceptance.md`：记录重跑后的完整响应验收结果。

### Task 1: 用失败测试定义成功响应完整合同

**Files:**
- Test: `relay/relay_task_seedance_test.go`

- [ ] **Step 1: 添加简化响应的失败测试**

在 `relay/relay_task_seedance_test.go` 添加：

```go
func TestSeedanceTaskResponseCompletesSimplifiedSuccessResponse(t *testing.T) {
	task := &model.Task{
		TaskID: "task_public_complete", Platform: constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeZ5API)),
		Status: model.TaskStatusSuccess, SubmitTime: 111, FinishTime: 222, UpdatedAt: 222,
		Properties: model.Properties{OriginModelName: "doubao-seedance-2-0-260128"},
		PrivateData: model.TaskPrivateData{
			ResultURL: "https://example.com/video.mp4",
			UserRequestData: json.RawMessage(`{"ratio":"21:9","duration":4,"resolution":"1080p","seed":7,"framespersecond":30,"execution_expires_after":3600,"generate_audio":false,"draft":true,"priority":2,"service_tier":"flex"}`),
			BillingContext: &model.TaskBillingContext{
				UsageProfile: model.TaskUsageProfileSeedance, UsageSnapshotVersion: model.TaskUsageSnapshotVersion1,
				UsageCompletionTokens: 100, UsageTotalTokens: 120, UsageInputTokens: 20,
				UsageSource: model.TaskUsageSourceLocalCalculated, RequestedDurationSeconds: 5,
				Resolution: "720p", ServiceTier: "default",
			},
		},
		Data: json.RawMessage(`{"task_id":"upstream-secret","status":"completed","progress":100,"video_url":"https://example.com/video.mp4"}`),
	}

	response, err := seedanceTaskResponse(task)

	require.NoError(t, err)
	assert.Equal(t, "task_public_complete", response["id"])
	assert.Equal(t, "doubao-seedance-2-0-260128", response["model"])
	assert.Equal(t, "succeeded", response["status"])
	assert.EqualValues(t, 7, response["seed"])
	assert.Equal(t, "1080p", response["resolution"])
	assert.Equal(t, "21:9", response["ratio"])
	assert.EqualValues(t, 4, response["duration"])
	assert.EqualValues(t, 30, response["framespersecond"])
	assert.Equal(t, "flex", response["service_tier"])
	assert.EqualValues(t, 3600, response["execution_expires_after"])
	assert.Equal(t, false, response["generate_audio"])
	assert.Equal(t, true, response["draft"])
	assert.EqualValues(t, 2, response["priority"])
	usage := response["usage"].(map[string]interface{})
	assert.EqualValues(t, 100, usage["completion_tokens"])
	assert.EqualValues(t, 120, usage["total_tokens"])
}
```

- [ ] **Step 2: 添加固定默认值的失败测试**

```go
func TestSeedanceTaskResponseUsesExplicitDefaultsWhenFactsAreUnavailable(t *testing.T) {
	task := &model.Task{
		TaskID: "task_public_defaults", Platform: constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeZ5API)),
		Status: model.TaskStatusSuccess, CreatedAt: 10, UpdatedAt: 20,
		Properties: model.Properties{OriginModelName: "doubao-seedance-2-0-260128"},
		PrivateData: model.TaskPrivateData{ResultURL: "https://example.com/video.mp4"},
		Data: json.RawMessage(`{"status":"completed","video_url":"https://example.com/video.mp4"}`),
	}

	response, err := seedanceTaskResponse(task)

	require.NoError(t, err)
	assert.EqualValues(t, 0, response["seed"])
	assert.Equal(t, "720p", response["resolution"])
	assert.Equal(t, "16:9", response["ratio"])
	assert.EqualValues(t, 5, response["duration"])
	assert.EqualValues(t, 24, response["framespersecond"])
	assert.Equal(t, "default", response["service_tier"])
	assert.EqualValues(t, 172800, response["execution_expires_after"])
	assert.Equal(t, true, response["generate_audio"])
	assert.Equal(t, false, response["draft"])
	assert.EqualValues(t, 0, response["priority"])
	assert.Equal(t, map[string]interface{}{"completion_tokens": int64(0), "total_tokens": int64(0)}, response["usage"])
}
```

- [ ] **Step 3: 运行测试并确认按预期失败**

Run:

```powershell
go test ./relay -run 'TestSeedanceTaskResponse(CompletesSimplifiedSuccessResponse|UsesExplicitDefaultsWhenFactsAreUnavailable)$' -count=1
```

Expected: FAIL，缺失 `seed`、`resolution`、`ratio`、`duration` 等字段。

- [ ] **Step 4: 提交测试红灯**

```powershell
git add relay/relay_task_seedance_test.go
git commit -m "测试 Seedance 终态响应完整合同"
```

### Task 2: 实现集中式成功响应补全器

**Files:**
- Create: `relay/seedance_task_response_completion.go`
- Modify: `relay/seedance_task.go`
- Test: `relay/relay_task_seedance_test.go`

- [ ] **Step 1: 创建请求快照和字段补全实现**

创建 `relay/seedance_task_response_completion.go`，使用 `encoding/json` 仅引用 `json.Number` 类型，所有解析通过 `common.Unmarshal`：

```go
package relay

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/shopspring/decimal"
)

const (
	defaultSeedanceResolution            = "720p"
	defaultSeedanceRatio                 = "16:9"
	defaultSeedanceDurationSeconds       = int64(5)
	defaultSeedanceFramesPerSecond       = int64(24)
	defaultSeedanceServiceTier           = "default"
	defaultSeedanceExecutionExpiresAfter = int64(172800)
)

type seedanceTaskRequestSnapshot struct {
	Seed                  *json.Number `json:"seed"`
	Resolution            string       `json:"resolution"`
	Ratio                 string       `json:"ratio"`
	Duration              *json.Number `json:"duration"`
	FramesPerSecond       *json.Number `json:"framespersecond"`
	ServiceTier           string       `json:"service_tier"`
	ExecutionExpiresAfter *json.Number `json:"execution_expires_after"`
	GenerateAudio         *bool        `json:"generate_audio"`
	Draft                 *bool        `json:"draft"`
	Priority              *json.Number `json:"priority"`
}

func completeSeedanceTerminalResponse(task *model.Task, response map[string]interface{}) {
	if task == nil || response == nil {
		return
	}
	request := seedanceTaskRequestSnapshot{}
	if len(task.PrivateData.UserRequestData) > 0 {
		_ = common.Unmarshal(task.PrivateData.UserRequestData, &request)
	}
	bc := task.PrivateData.BillingContext

	setSeedanceInteger(response, "seed", 0, 1<<31-1, request.Seed, int64(0))
	setSeedanceString(response, "resolution", request.Resolution, seedanceBillingResolution(bc), defaultSeedanceResolution)
	setSeedanceString(response, "ratio", request.Ratio, defaultSeedanceRatio)
	setSeedanceInteger(response, "duration", 1, relaycommon.MaxTaskDurationSeconds, request.Duration, seedanceRequestedDuration(bc), defaultSeedanceDurationSeconds)
	setSeedanceInteger(response, "framespersecond", 1, 240, request.FramesPerSecond, seedanceBillingFrameRate(bc), defaultSeedanceFramesPerSecond)
	setSeedanceString(response, "service_tier", request.ServiceTier, seedanceBillingServiceTier(bc), defaultSeedanceServiceTier)
	setSeedanceInteger(response, "execution_expires_after", 1, 1<<31-1, request.ExecutionExpiresAfter, defaultSeedanceExecutionExpiresAfter)
	setSeedanceBool(response, "generate_audio", request.GenerateAudio, seedanceBillingGenerateAudio(bc), true)
	setSeedanceBool(response, "draft", request.Draft, seedanceBillingDraft(bc), false)
	setSeedanceInteger(response, "priority", 0, 1<<31-1, request.Priority, int64(0))
	completeSeedanceUsage(response)
}

func setSeedanceString(response map[string]interface{}, key string, candidates ...string) {
	if value, ok := response[key].(string); ok && strings.TrimSpace(value) != "" {
		return
	}
	for _, candidate := range candidates {
		if candidate = strings.TrimSpace(candidate); candidate != "" {
			response[key] = candidate
			return
		}
	}
}

func setSeedanceInteger(response map[string]interface{}, key string, minimum, maximum int64, candidates ...interface{}) {
	if value, ok := boundedSeedanceInteger(response[key], minimum, maximum); ok {
		response[key] = value
		return
	}
	for _, candidate := range candidates {
		if value, ok := boundedSeedanceInteger(candidate, minimum, maximum); ok {
			response[key] = value
			return
		}
	}
}

func boundedSeedanceInteger(value interface{}, minimum, maximum int64) (int64, bool) {
	if value == nil {
		return 0, false
	}
	number, err := decimal.NewFromString(strings.TrimSpace(fmt.Sprint(value)))
	if err != nil || !number.Equal(number.Truncate(0)) || number.LessThan(decimal.NewFromInt(minimum)) || number.GreaterThan(decimal.NewFromInt(maximum)) {
		return 0, false
	}
	return number.IntPart(), true
}
```

同文件加入以下实现。布尔函数通过指针区分显式 `false` 与缺失；Token 用量缺失或非法时写入显式零值：

```go
func setSeedanceBool(response map[string]interface{}, key string, candidates ...interface{}) {
	if _, ok := response[key].(bool); ok {
		return
	}
	for _, candidate := range candidates {
		switch value := candidate.(type) {
		case bool:
			response[key] = value
			return
		case *bool:
			if value != nil {
				response[key] = *value
				return
			}
		}
	}
}

func seedanceBillingResolution(bc *model.TaskBillingContext) string {
	if bc == nil {
		return ""
	}
	if value := strings.TrimSpace(bc.Resolution); value != "" {
		return value
	}
	return strings.TrimSpace(bc.DurationResolution)
}

func seedanceRequestedDuration(bc *model.TaskBillingContext) interface{} {
	if bc == nil {
		return nil
	}
	return bc.RequestedDurationSeconds
}

func seedanceBillingFrameRate(bc *model.TaskBillingContext) interface{} {
	if bc == nil || bc.SeedanceTokenBilling == nil {
		return nil
	}
	return bc.SeedanceTokenBilling.FrameRate
}

func seedanceBillingServiceTier(bc *model.TaskBillingContext) string {
	if bc == nil {
		return ""
	}
	return bc.ServiceTier
}

func seedanceBillingGenerateAudio(bc *model.TaskBillingContext) interface{} {
	if bc == nil {
		return nil
	}
	return bc.GenerateAudio
}

func seedanceBillingDraft(bc *model.TaskBillingContext) interface{} {
	if bc == nil {
		return nil
	}
	return bc.Draft
}

func completeSeedanceUsage(response map[string]interface{}) {
	usage, ok := response["usage"].(map[string]interface{})
	if !ok {
		usage = make(map[string]interface{})
	}
	completion, ok := boundedSeedanceInteger(usage["completion_tokens"], 0, relaycommon.MaxTokensLimit)
	if !ok {
		completion = 0
	}
	total, ok := boundedSeedanceInteger(usage["total_tokens"], 0, relaycommon.MaxTokensLimit)
	if !ok {
		total = 0
	}
	if total < completion {
		total = completion
	}
	usage["completion_tokens"] = completion
	usage["total_tokens"] = total
	response["usage"] = usage
}
```

- [ ] **Step 2: 在通用响应末端调用补全器**

修改 `relay/seedance_task.go`：

```go
	if err := populateSeedanceTaskUsage(task, response); err != nil {
		return nil, err
	}
	completeSeedanceTerminalResponse(task, response)
	return response, nil
```

- [ ] **Step 3: 运行成功合同测试**

```powershell
go test ./relay -run 'TestSeedanceTaskResponse(CompletesSimplifiedSuccessResponse|UsesExplicitDefaultsWhenFactsAreUnavailable)$' -count=1
```

Expected: PASS。

- [ ] **Step 4: 增加上游优先和非法值回退测试**

```go
func TestSeedanceTaskResponseAppliesTerminalFactPriority(t *testing.T) {
	tests := []struct {
		name        string
		data        string
		requestData json.RawMessage
		wantDuration int64
	}{
		{name: "upstream wins", data: `{"status":"succeeded","duration":6,"content":{"video_url":"https://x/video.mp4"}}`, requestData: json.RawMessage(`{"duration":4}`), wantDuration: 6},
		{name: "invalid upstream falls back to request", data: `{"status":"succeeded","duration":999999,"content":{"video_url":"https://x/video.mp4"}}`, requestData: json.RawMessage(`{"duration":4}`), wantDuration: 4},
		{name: "broken request falls back to billing", data: `{"status":"succeeded","content":{"video_url":"https://x/video.mp4"}}`, requestData: json.RawMessage(`{"duration":`), wantDuration: 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &model.Task{
				TaskID: "task_priority", Platform: constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeZ5API)),
				Status: model.TaskStatusSuccess, Properties: model.Properties{OriginModelName: "doubao-seedance-2-0-260128"},
				PrivateData: model.TaskPrivateData{ResultURL: "https://x/video.mp4", UserRequestData: tt.requestData,
					BillingContext: &model.TaskBillingContext{RequestedDurationSeconds: 5, GenerateAudio: common.GetPointer(false)}},
				Data: json.RawMessage(tt.data),
			}
			response, err := seedanceTaskResponse(task)
			require.NoError(t, err)
			assert.EqualValues(t, tt.wantDuration, response["duration"])
			assert.Equal(t, false, response["generate_audio"])
		})
	}
}
```

- [ ] **Step 5: 运行 relay 回归测试**

```powershell
go test ./relay -count=1
```

Expected: PASS。

- [ ] **Step 6: 提交成功响应补全实现**

```powershell
git add relay/seedance_task_response_completion.go relay/seedance_task.go relay/relay_task_seedance_test.go
git commit -m "补全 Seedance 成功任务终态响应"
```

### Task 3: 补全失败响应合同

**Files:**
- Modify: `relay/seedance_task_response_completion.go`
- Test: `relay/relay_task_seedance_test.go`

- [ ] **Step 1: 添加失败响应红灯测试**

```go
func TestSeedanceTaskResponseCompletesFailureMetadataWithoutFakeVideo(t *testing.T) {
	task := &model.Task{
		TaskID: "task_public_failed", Platform: constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeZ5API)),
		Status: model.TaskStatusFailure, SubmitTime: 111, FinishTime: 222, UpdatedAt: 222,
		Properties: model.Properties{OriginModelName: "doubao-seedance-2-0-260128"},
		PrivateData: model.TaskPrivateData{UserRequestData: json.RawMessage(`{"duration":4,"resolution":"720p"}`)},
		Data: json.RawMessage(`{"status":"failed"}`),
	}

	response, err := seedanceTaskResponse(task)

	require.NoError(t, err)
	assert.Equal(t, "failed", response["status"])
	errorData := response["error"].(map[string]interface{})
	assert.Equal(t, "task_failed", errorData["code"])
	assert.Equal(t, "task failed", errorData["message"])
	assert.Contains(t, response, "duration")
	assert.Contains(t, response, "framespersecond")
	content, hasContent := response["content"].(map[string]interface{})
	assert.False(t, hasContent && strings.TrimSpace(fmt.Sprint(content["video_url"])) != "")
}
```

- [ ] **Step 2: 运行测试确认失败**

```powershell
go test ./relay -run TestSeedanceTaskResponseCompletesFailureMetadataWithoutFakeVideo -count=1
```

Expected: FAIL，缺少公共字段或默认 `error`。

- [ ] **Step 3: 实现失败错误补全**

在 `completeSeedanceTerminalResponse` 中增加：

```go
if task.Status == model.TaskStatusFailure {
	completeSeedanceFailure(response)
}
```

实现：

```go
func completeSeedanceFailure(response map[string]interface{}) {
	errorData, ok := response["error"].(map[string]interface{})
	if !ok {
		errorData = make(map[string]interface{})
	}
	if code, _ := errorData["code"].(string); strings.TrimSpace(code) == "" {
		errorData["code"] = "task_failed"
	}
	if message, _ := errorData["message"].(string); strings.TrimSpace(message) == "" {
		errorData["message"] = "task failed"
	}
	response["error"] = errorData
	if content, ok := response["content"].(map[string]interface{}); ok {
		videoURL, _ := content["video_url"].(string)
		if strings.TrimSpace(videoURL) == "" {
			delete(response, "content")
		}
	}
}
```

- [ ] **Step 4: 运行失败和成功合同测试**

```powershell
go test ./relay -run 'TestSeedanceTaskResponse.*(Complete|Default|Failure)' -count=1
```

Expected: PASS。

- [ ] **Step 5: 提交失败响应补全**

```powershell
git add relay/seedance_task_response_completion.go relay/relay_task_seedance_test.go
git commit -m "补全 Seedance 失败任务终态响应"
```

### Task 4: 让矩阵 E2E 对完整响应执行硬断言

**Files:**
- Modify: `cmd/ark-video-material-seed/main.go`
- Test: `cmd/ark-video-material-seed/main_test.go`

- [ ] **Step 1: 添加 E2E 合同校验器测试**

```go
func TestValidateArkTerminalResponse(t *testing.T) {
	complete := json.RawMessage(`{"id":"task_x","model":"m","status":"succeeded","content":{"video_url":"https://x/video.mp4"},"usage":{"completion_tokens":0,"total_tokens":0},"created_at":1,"updated_at":2,"seed":0,"resolution":"720p","ratio":"16:9","duration":5,"framespersecond":24,"service_tier":"default","execution_expires_after":172800,"generate_audio":true,"draft":false,"priority":0}`)
	require.NoError(t, validateArkTerminalResponse(complete, model.TaskStatusSuccess))

	missingFPS := json.RawMessage(strings.Replace(string(complete), `,"framespersecond":24`, "", 1))
	require.ErrorContains(t, validateArkTerminalResponse(missingFPS, model.TaskStatusSuccess), "framespersecond")

	failed := json.RawMessage(`{"id":"task_x","model":"m","status":"failed","usage":{"completion_tokens":0,"total_tokens":0},"error":{"code":"task_failed","message":"task failed"},"created_at":1,"updated_at":2,"seed":0,"resolution":"720p","ratio":"16:9","duration":5,"framespersecond":24,"service_tier":"default","execution_expires_after":172800,"generate_audio":true,"draft":false,"priority":0}`)
	require.NoError(t, validateArkTerminalResponse(failed, model.TaskStatusFailure))
	require.ErrorContains(t, validateArkTerminalResponse(json.RawMessage(strings.Replace(string(failed), `"status":"failed"`, `"status":"failed","content":{"video_url":"https://x/video.mp4"}`, 1)), model.TaskStatusFailure), "video_url")
}
```

- [ ] **Step 2: 运行测试确认失败**

```powershell
go test ./cmd/ark-video-material-seed -run TestValidateArkTerminalResponse -count=1
```

Expected: FAIL，校验函数尚不存在。

- [ ] **Step 3: 实现合同校验器并接入执行循环**

校验器解析以下结构并检查字段存在性和类型：

```go
type arkTerminalResponse struct {
	ID, Model, Status, Resolution, Ratio, ServiceTier string
	Content *struct{ VideoURL string `json:"video_url"` } `json:"content"`
	Usage *struct {
		CompletionTokens *int64 `json:"completion_tokens"`
		TotalTokens *int64 `json:"total_tokens"`
	} `json:"usage"`
	Error *struct{ Code, Message string } `json:"error"`
	CreatedAt, UpdatedAt, Seed, Duration, FramesPerSecond, ExecutionExpiresAfter, Priority *json.Number
	GenerateAudio, Draft *bool
}
```

使用下面的公共字段检查，数值和布尔字段必须使用指针才能识别“缺失”和“零值”的区别：

```go
func validateArkTerminalResponse(data json.RawMessage, wantStatus model.TaskStatus) error {
	var response arkTerminalResponse
	if err := common.Unmarshal(data, &response); err != nil {
		return err
	}
	for name, value := range map[string]string{
		"id": response.ID, "model": response.Model, "status": response.Status,
		"resolution": response.Resolution, "ratio": response.Ratio, "service_tier": response.ServiceTier,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("missing %s", name)
		}
	}
	for name, value := range map[string]*json.Number{
		"created_at": response.CreatedAt, "updated_at": response.UpdatedAt, "seed": response.Seed,
		"duration": response.Duration, "framespersecond": response.FramesPerSecond,
		"execution_expires_after": response.ExecutionExpiresAfter, "priority": response.Priority,
	} {
		if value == nil {
			return fmt.Errorf("missing %s", name)
		}
	}
	if response.GenerateAudio == nil || response.Draft == nil {
		return errors.New("missing generate_audio or draft")
	}
	if response.Usage == nil || response.Usage.CompletionTokens == nil || response.Usage.TotalTokens == nil {
		return errors.New("missing usage")
	}
	if wantStatus == model.TaskStatusSuccess {
		if response.Status != "succeeded" || response.Content == nil || strings.TrimSpace(response.Content.VideoURL) == "" {
			return errors.New("successful response is missing content.video_url")
		}
		return nil
	}
	if response.Status != "failed" || response.Error == nil || strings.TrimSpace(response.Error.Code) == "" || strings.TrimSpace(response.Error.Message) == "" {
		return errors.New("failed response is missing error")
	}
	if response.Content != nil && strings.TrimSpace(response.Content.VideoURL) != "" {
		return errors.New("failed response must not contain content.video_url")
	}
	return nil
}
```

在每条成功任务和失败夹具读取 `UserResponseData` 后调用该校验器。任何字段缺失立即终止 E2E，并输出 `case_id`、任务 ID 和字段名。

- [ ] **Step 4: 扩展 mock 路径测试**

在 `TestMockVideoServerSupportsProviderSubmitAndPollingPaths` 的轮询循环中加入：

```go
switch {
case strings.HasPrefix(testCase.path, "/v1/videos/tasks/"), strings.HasPrefix(testCase.path, "/v1/videos/"):
	require.NotContains(t, recorder.Body.String(), `"framespersecond"`, testCase.path)
default:
	require.Contains(t, recorder.Body.String(), `"framespersecond":24`, testCase.path)
}
```

这项断言固定两类供应商协议都存在：简化上游响应必须依靠集中补全器，完整包装响应则保留上游事实优先路径。

- [ ] **Step 5: 运行种子测试**

```powershell
go test ./cmd/ark-video-material-seed -count=1
```

Expected: PASS。

- [ ] **Step 6: 提交 E2E 合同断言**

```powershell
git add cmd/ark-video-material-seed/main.go cmd/ark-video-material-seed/main_test.go
git commit -m "强制校验 Ark 视频任务终态响应"
```

### Task 5: 重建、重跑 E2E 并更新验收报告

**Files:**
- Modify: `docs/superpowers/reports/2026-08-04-ark-sdk-video-material-matrix-reimport-acceptance.md`

- [ ] **Step 1: 运行后端定向测试**

```powershell
go test ./relay ./service ./cmd/ark-video-material-seed -count=1 -p=1
```

Expected: PASS。

- [ ] **Step 2: 重建本地服务**

```powershell
docker compose -f docker-compose.local.yml up -d --build
docker compose -f docker-compose.local.yml ps
curl.exe -sS -o NUL -w "%{http_code}" http://127.0.0.1:3000/api/status
```

Expected: 全部容器 healthy，HTTP `200`。

- [ ] **Step 3: 重新执行 Ark 视频矩阵种子**

```powershell
docker exec -w /data new-api-local-new-api-1 /data/ark-video-material-seed
```

Expected: 155 个目标、118 个成功任务、1 个失败任务、36 个合同阻断、1 个禁用价格，并且终态响应完整校验为 119/119。

- [ ] **Step 4: 数据库核验完整字段覆盖**

对 `ark_sdk_matrix_user` 的最新任务查询 JSON 字段覆盖，成功任务的 `usage`、`duration`、`framespersecond`、`execution_expires_after` 均为 118/118；失败任务包含 `error` 且无非空视频 URL。

- [ ] **Step 5: 更新验收报告**

在报告中增加“终态响应结构”小节，记录原问题为 93/118 条简化响应，修复后成功任务完整率 118/118、失败合同完整率 1/1，并说明默认值策略。

- [ ] **Step 6: 运行前端回归、类型检查和构建**

```powershell
cd web
bun test --parallel=1
bun run typecheck
bun run build
```

Expected: 0 fail，typecheck 和 build 退出码为 0。

- [ ] **Step 7: 最终差异校验并提交**

```powershell
git diff --check
git status --short
git add docs/superpowers/reports/2026-08-04-ark-sdk-video-material-matrix-reimport-acceptance.md
git commit -m "验收 Seedance 完整终态任务响应"
```

Expected: 提交后工作区干净。
