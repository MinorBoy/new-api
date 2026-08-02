# CLMM Mall 渠道协议同步实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 恢复 CLMM 完整模型 ID 的协议控制规则，增加参考音频支持，并禁止创建任务自动重试，同时保持现有时长计费和任务查询行为不变。

**Architecture:** SD 收录表或渠道模板表继续提供完整模型 ID、能力和成本数据；`clmmmall` 适配器继续解析完整模型 ID 中的渠道前缀及控制后缀来构造 `seconds`、`mySeconds` 和素材字段。控制器只对 CLMM 创建请求增加不重试策略，查询调度保持原有行为。

**Tech Stack:** Go 1.22+、Gin、testify、项目 `common` JSON 包装、React/Bun 配置导入测试。

**Design:** `docs/superpowers/specs/2026-08-02-clmm-mall-protocol-sync-design.md`

---

## 文件职责

- `docs/new-channels/cn-clmm.md`：面向接入方的创建、查询、模型控制段、示例和错误码文档。
- `relay/channel/task/clmmmall/dto.go`：CLMM 上游请求和响应 DTO。
- `relay/channel/task/clmmmall/translate.go`：Ark 内容校验、模型控制段解析、时长和素材转换。
- `relay/channel/task/clmmmall/translate_test.go`：请求转换、后缀控制和素材边界回归测试。
- `relay/channel/task/clmmmall/adaptor.go`：创建、查询、错误映射和 Ark 任务投影。
- `relay/channel/task/clmmmall/adaptor_test.go`：上游错误状态的稳定映射测试。
- `relay/video_route_contract.go`：导入 route target 的 CLMM 协议上限校验。
- `relay/video_route_contract_test.go`：CLMM route target 音频能力和上限测试。
- `controller/relay.go`：任务创建请求的跨渠道重试决策。
- `controller/task_retry_policy_test.go`：CLMM 创建不重试和其他任务渠道兼容性测试。

### Task 1: 恢复渠道 API 文档中的模型控制规则

**Files:**
- Modify: `docs/new-channels/cn-clmm.md`

- [ ] **Step 1: 修正文档真相源说明**

将开头说明改为以下语义：

```markdown
渠道可用模型及正确的完整模型 ID 以 SD 收录表或渠道模板表的导入结果为准。完整模型 ID 中的渠道前缀和控制后缀属于 CLMM Mall 请求协议，接入方必须按本文规则解析并生成 `seconds`、`mySeconds`、分辨率和参考素材字段；不得自行枚举前缀后的具体基础模型名。
```

- [ ] **Step 2: 在“创建视频任务”中恢复控制段说明**

在请求参数与示例之间增加“模型 ID 控制规则”和“`mySeconds` 规则”，明确：

```text
支持前缀：sh-、grok-、veo-、bbv3-、bbv4-、me-、hj-、mowc-、op-
-480p/-480P/-720p/-720P：覆盖请求分辨率
-Ns：发送 seconds="1"，实际时长写入 mySeconds
-gz：必须与 -Ns 同时存在，并把实际时长固定为 N
-Nimg：至少需要 N 张 reference_image_urls
-nv：丢弃 reference_videos
```

同时保留以下边界：

```text
具体基础模型名不设置本地白名单。
未定义的 -sr、-nsp、-nyp、-nyy 等字符串不产生控制行为。
发送给 CLMM Mall 的 model 保持导入后的完整模型 ID，不删除控制段。
```

- [ ] **Step 3: 更新创建示例**

把 JSON、cURL 和 Python 示例调整为固定时长示例：

```json
{
  "model": "op-video-gz-10s",
  "prompt": "一只橘猫在雨天窗台伸懒腰，电影感镜头",
  "aspect_ratio": "16:9",
  "resolution": "480p",
  "size": "1280x720",
  "seconds": "1",
  "mySeconds": "10",
  "reference_audios": []
}
```

示例旁注明该模型 ID 仅用于解释协议，实际调用必须使用导入结果中的完整模型 ID。

- [ ] **Step 4: 运行文档结构检查**

Run:

```powershell
$path = 'docs/new-channels/cn-clmm.md'
$content = [System.IO.File]::ReadAllText((Resolve-Path $path))
$required = @('SD 收录表或渠道模板表', 'sh-', '-gz', '-Nimg', '-nv', 'mySeconds', 'reference_audios', 'POST /videos', 'GET /videos/{task_id}', '## 错误码')
$missing = @($required | Where-Object { -not $content.Contains($_) })
if ($missing.Count -gt 0 -or ([regex]::Matches($content, '```')).Count % 2 -ne 0) { throw "CLMM 文档检查失败: $($missing -join ', ')" }
```

Expected: 命令退出码为 0，没有缺失项，代码围栏成对。

### Task 2: 支持 Ark 参考音频转换

**Files:**
- Modify: `relay/channel/task/clmmmall/dto.go`
- Modify: `relay/channel/task/clmmmall/translate.go`
- Test: `relay/channel/task/clmmmall/translate_test.go`

- [ ] **Step 1: 写入失败的音频精确输出测试**

在 `translate_test.go` 增加：

```go
func TestArkToClmmMapsReferenceAudiosInContentOrder(t *testing.T) {
	converted, _, err := arkToClmm(arkRequest{
		Model: "client-model",
		Content: []arkContent{
			{Type: "text", Text: "audio prompt"},
			{Type: "audio_url", Role: "reference_audio", AudioURL: &arkMedia{URL: "https://example.com/first.mp3"}},
			{Type: "audio_url", AudioURL: &arkMedia{URL: "data:audio/wav;base64,AA=="}},
		},
	}, "op-any-imported-model")

	require.NoError(t, err)
	assert.JSONEq(t, `{
		"model":"op-any-imported-model",
		"prompt":"audio prompt",
		"aspect_ratio":"16:9",
		"resolution":"480p",
		"size":"1280x720",
		"seconds":"5",
		"reference_audios":["https://example.com/first.mp3","data:audio/wav;base64,AA=="]
	}`, string(mustMarshalClmm(t, converted)))
}
```

从现有“不支持 Ark 输入”表中删除 `audio` 用例，并增加拒绝用例：音频 URL 为空、role 为 `background_audio`、音频数量为 4、总素材数量为 13。

- [ ] **Step 2: 运行测试确认失败**

Run:

```bash
go test ./relay/channel/task/clmmmall -run 'TestArkToClmmMapsReferenceAudios|TestArkToClmmRejectsUnsupportedArkInput|TestArkToClmmEnforcesMediaLimits' -count=1
```

Expected: FAIL，现有实现返回 `audio input is not supported by CLMM Mall` 或缺少 `ReferenceAudios` 字段。

- [ ] **Step 3: 扩展上游 DTO**

在 `clmmRequest` 中增加：

```go
ReferenceAudios []string `json:"reference_audios,omitempty"`
```

- [ ] **Step 4: 实现音频规范化与边界校验**

在 `normalizedArkRequest` 增加 `audios []string`。将 `audio_url` 分支改为：

```go
case "audio_url":
	if item.AudioURL == nil || strings.TrimSpace(item.AudioURL.URL) == "" {
		return normalizedArkRequest{}, fmt.Errorf("audio_url.url is required")
	}
	role := strings.TrimSpace(item.Role)
	if role != "" && role != "reference_audio" {
		return normalizedArkRequest{}, fmt.Errorf("audio role must be reference_audio")
	}
	audios = append(audios, item.AudioURL.URL)
```

在返回前执行：

```go
if len(audios) > 3 {
	return normalizedArkRequest{}, fmt.Errorf("too many reference audios: maximum is 3")
}
if len(images)+len(videos)+len(audios) > 12 {
	return normalizedArkRequest{}, fmt.Errorf("too many media items: maximum is 12")
}
```

将 `audios` 写入 `normalizedArkRequest`，并在 `arkToClmm` 返回的 `clmmRequest` 中设置 `ReferenceAudios: normalized.audios`。

- [ ] **Step 5: 格式化并运行 focused tests**

Run:

```bash
gofmt -w relay/channel/task/clmmmall/dto.go relay/channel/task/clmmmall/translate.go relay/channel/task/clmmmall/translate_test.go
go test ./relay/channel/task/clmmmall -run 'TestArkToClmm' -count=1
```

Expected: PASS。

- [ ] **Step 6: 提交音频转换改动**

```bash
git add relay/channel/task/clmmmall/dto.go relay/channel/task/clmmmall/translate.go relay/channel/task/clmmmall/translate_test.go
git commit -m "feat(video): support CLMM reference audio"
```

### Task 3: 允许导入的 CLMM 音频路由能力

**Files:**
- Modify: `relay/video_route_contract.go`
- Test: `relay/video_route_contract_test.go`

- [ ] **Step 1: 把现有拒绝音频测试改为接受导入能力**

将 `clmm rejects audio declaration` 替换为：

```go
{
	name: "clmm accepts imported audio capability", channelType: constant.ChannelTypeClmmMall,
	target: videoContractTarget("op-video-720p", []string{"720p"}, 5, 15, nil, modelrouting.ReferenceLimits{Images: 4, Videos: 3, Audios: 1}),
},
```

增加失败用例：`Audios: 4`，以及 `Images: 9, Videos: 3, Audios: 1`。两者都必须返回 `route_contract_references`。

- [ ] **Step 2: 运行路由契约测试确认失败**

Run:

```bash
go test ./relay -run '^TestValidateVideoRouteTargetContract$' -count=1
```

Expected: FAIL，合法音频能力仍被拒绝。

- [ ] **Step 3: 更新 CLMM route target 协议上限**

将素材判断改为：

```go
limits := target.Constraints.ReferenceLimits
if limits.Images > 9 || limits.Videos > 3 || limits.Audios > 3 || limits.Images+limits.Videos+limits.Audios > 12 {
	return newVideoRouteContractError("route_contract_references", "CLMM route reference limits exceed the verified protocol")
}
```

模型语法、分辨率和时长验证保持不变。

- [ ] **Step 4: 格式化并运行测试**

Run:

```bash
gofmt -w relay/video_route_contract.go relay/video_route_contract_test.go
go test ./relay -run '^TestValidateVideoRouteTargetContract$' -count=1
```

Expected: PASS。

- [ ] **Step 5: 提交路由契约改动**

```bash
git add relay/video_route_contract.go relay/video_route_contract_test.go
git commit -m "fix(video): accept CLMM audio route capability"
```

### Task 4: 禁止 CLMM 创建任务自动重试

**Files:**
- Modify: `controller/relay.go`
- Create: `controller/task_retry_policy_test.go`

- [ ] **Step 1: 写入失败的重试策略测试**

创建 `controller/task_retry_policy_test.go`：

```go
package controller

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	taskdto "github.com/QuantumNous/new-api/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestShouldRetryTaskRelayNeverRetriesClmmCreate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	for _, statusCode := range []int{http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable} {
		t.Run(http.StatusText(statusCode), func(t *testing.T) {
			taskErr := &taskdto.TaskError{StatusCode: statusCode, Error: errors.New("upstream create failed")}
			assert.False(t, shouldRetryTaskRelay(c, constant.ChannelTypeClmmMall, taskErr, 1))
		})
	}
}

func TestShouldRetryTaskRelayKeepsOtherTaskChannelPolicy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	for _, statusCode := range []int{http.StatusTooManyRequests, http.StatusInternalServerError} {
		taskErr := &taskdto.TaskError{StatusCode: statusCode, Error: errors.New("upstream create failed")}
		assert.True(t, shouldRetryTaskRelay(c, constant.ChannelTypeMegaByAI, taskErr, 1))
	}
}
```

- [ ] **Step 2: 运行测试确认行为失败**

Run: `go test ./controller -run '^TestShouldRetryTaskRelay' -count=1`

Expected: FAIL，CLMM 的 `429` 和 `5xx` 仍返回 `true`。

- [ ] **Step 3: 把重试决策参数改为渠道类型**

把调用点改为：

```go
shouldRetryRequest := shouldRetryTaskRelay(c, channel.Type, taskErr, common.RetryTimes-retryParam.GetRetry())
```

把函数签名和完整实现改为：

```go
func shouldRetryTaskRelay(c *gin.Context, channelType int, taskErr *taskdto.TaskError, retryTimes int) bool {
	if taskErr == nil {
		return false
	}
	if channelType == constant.ChannelTypeClmmMall {
		return false
	}
	if service.ShouldSkipRetryAfterChannelAffinityFailure(c) {
		return false
	}
	if retryTimes <= 0 {
		return false
	}
	if _, ok := c.Get("specific_channel_id"); ok {
		return false
	}
	if taskErr.StatusCode == http.StatusTooManyRequests || taskErr.StatusCode == 307 {
		return true
	}
	if taskErr.StatusCode/100 == 5 {
		return !operation_setting.IsAlwaysSkipRetryStatusCode(taskErr.StatusCode)
	}
	if taskErr.StatusCode == http.StatusBadRequest || taskErr.StatusCode == http.StatusRequestTimeout {
		return false
	}
	if taskErr.LocalError || taskErr.StatusCode/100 == 2 {
		return false
	}
	return true
}
```

`handleTaskCostCoverageFailure` 仍在该函数之前处理尚未发出上游请求的成本覆盖失败。

- [ ] **Step 4: 格式化并运行 focused tests**

Run:

```bash
gofmt -w controller/relay.go controller/task_retry_policy_test.go
go test ./controller -run 'TestShouldRetryTaskRelay|TestHandleTaskCostCoverageFailure|TestHandleTaskProfitEligibilityFailure' -count=1
```

Expected: PASS。

- [ ] **Step 5: 提交创建重试策略**

```bash
git add controller/relay.go controller/task_retry_policy_test.go
git commit -m "fix(video): prevent CLMM task creation retries"
```

### Task 5: 同步 CLMM `402` 和 `403` 错误映射

**Files:**
- Modify: `relay/channel/task/clmmmall/adaptor.go`
- Test: `relay/channel/task/clmmmall/adaptor_test.go`

- [ ] **Step 1: 扩展稳定错误映射测试**

在错误映射表增加：

```go
{
	name: "insufficient balance", responseStatus: http.StatusPaymentRequired,
	taskStatus: http.StatusPaymentRequired, body: `{"error":{"message":"private balance diagnostic"}}`,
	code: "insufficient_quota", message: "CLMM Mall balance is insufficient",
},
{
	name: "permission denied", responseStatus: http.StatusForbidden,
	taskStatus: http.StatusForbidden, body: `{"error":{"message":"private permission diagnostic"}}`,
	code: "permission_denied", message: "CLMM Mall permission denied",
},
```

从默认网关错误循环中移除 `http.StatusForbidden`。

- [ ] **Step 2: 运行错误映射测试确认失败**

Run: `go test ./relay/channel/task/clmmmall -run '^TestTaskAdaptorParseTaskErrorUsesStableMappings$' -count=1`

Expected: FAIL，现有实现把 `402` 和 `403` 映射为 `502 upstream_error`。

- [ ] **Step 3: 实现稳定错误映射**

在 `ParseTaskError` 中增加：

```go
case http.StatusPaymentRequired:
	message := "CLMM Mall balance is insufficient"
	return &dto.TaskError{Code: "insufficient_quota", Message: message, StatusCode: http.StatusPaymentRequired, Error: errors.New(message)}
case http.StatusForbidden:
	message := "CLMM Mall permission denied"
	return &dto.TaskError{Code: "permission_denied", Message: message, StatusCode: http.StatusForbidden, Error: errors.New(message)}
```

不解析或转发上游响应正文。

- [ ] **Step 4: 格式化并运行 adaptor tests**

Run:

```bash
gofmt -w relay/channel/task/clmmmall/adaptor.go relay/channel/task/clmmmall/adaptor_test.go
go test ./relay/channel/task/clmmmall -run 'TestTaskAdaptorParseTaskError|TestTaskAdaptorFetchTask|TestParseTaskResult' -count=1
```

Expected: PASS。

- [ ] **Step 5: 提交错误映射改动**

```bash
git add relay/channel/task/clmmmall/adaptor.go relay/channel/task/clmmmall/adaptor_test.go
git commit -m "fix(video): map CLMM balance and permission errors"
```

### Task 6: 完整验证与范围审计

**Files:**
- Verify: `docs/new-channels/cn-clmm.md`
- Verify: `relay/channel/task/clmmmall/*.go`
- Verify: `relay/video_route_contract.go`
- Verify: `controller/relay.go`

- [ ] **Step 1: 运行 CLMM 包完整测试**

Run: `go test ./relay/channel/task/clmmmall -count=1`

Expected: PASS，0 failures。

- [ ] **Step 2: 运行路由和控制器测试**

Run: `go test ./relay ./controller -count=1`

Expected: PASS，0 failures。

- [ ] **Step 3: 运行配置导入转换测试**

Run:

```bash
cd web
bun test src/channel-config-converter/__tests__/v1.test.ts
```

Expected: PASS，并继续断言 CLMM target 的 `audios: 1`。

- [ ] **Step 4: 运行静态范围检查**

Run:

```bash
rg -n 'encoding/json|OtherRatios\[|int\(.*quota|math\.Round' relay/channel/task/clmmmall
rg -n 'parseModelControls|clmmModelPrefixes|ReferenceAudios|reference_audios' relay/channel/task/clmmmall
git diff --check
```

Expected: 第一条没有新增违规 JSON 或计费转换；第二条显示前缀、后缀解析和音频字段均存在；`git diff --check` 退出码为 0。

- [ ] **Step 5: 审查最终差异**

Run:

```bash
git status --short
git log -5 --oneline
git diff HEAD~4 --stat
git diff HEAD~4 -- relay/channel/task/clmmmall relay/video_route_contract.go relay/video_route_contract_test.go controller/relay.go controller/task_retry_policy_test.go
```

Expected: 最近四个实现提交仅包含本计划列出的代码和测试；`docs/new-channels/cn-clmm.md` 因目录被 `.gitignore` 忽略，需单独确认本地文件内容。
