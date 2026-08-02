# Seedance 提交前 Usage 兜底快照实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在调用供应商之前为每个新 Seedance 任务锁定合法的用户侧 usage token 对，并保证成功终态和公共响应始终使用持久化的最终 usage，同时不改变按时长账单或供应商成本 meter 语义。

**Architecture:** 依据[设计文档](../specs/2026-08-02-seedance-predispatch-usage-fallback-design.md)，提交阶段复用现有 Seedance 纯计算核心生成版本化 usage 快照，通过 `TaskRelayInfo` 传递并持久化到 `TaskBillingContext`。终态归一化按“合法上游 usage、合法终态重算、提交前快照”选择最终 token 对；版本化新任务缺少全部来源时阻止成功 CAS、不退款，历史任务继续尽力兼容。

**Tech Stack:** Go 1.22、Gin、GORM、shopspring/decimal、testify、SQLite E2E。

---

## 文件边界

- `model/task.go`：定义版本化、持久化的最终用户 usage token 对。
- `relay/common/relay_info.go`：在任务尚未持久化时传递提交前 token 对。
- `service/seedance_task_usage.go`：验证、读取和终态选择 Seedance usage 的唯一业务入口。
- `relay/relay_task.go`：在 `DoRequest` 前计算提交快照。
- `controller/relay.go`：把临时 token 对和快照版本写入任务计费上下文。
- `service/task_polling.go`：在任何成功状态变更前完成 usage 归一化，失败时保留预扣且不提交成功 CAS。
- `relay/seedance_task.go`：新快照任务只从持久化最终 token 对构建公共 usage；历史任务保留尽力计算。
- `e2e/seedance_material_matrix_e2e_test.go`：验证所有 Seedance 协议、用户计费模式和供应商成本模式的强保证。

### 任务一：定义版本化 usage 快照契约

**Files:**
- Modify: `model/task.go`
- Modify: `relay/common/relay_info.go`
- Modify: `service/seedance_task_usage.go`
- Modify: `service/seedance_task_usage_test.go`

- [ ] **Step 1: 写持久化快照验证的失败测试**

在 `service/seedance_task_usage_test.go` 增加以下表驱动测试，覆盖完整快照、单字段缺失、total 小于 completion、零值和超限值：

```go
func TestPersistedSeedanceTaskUsage(t *testing.T) {
	tests := []struct {
		name string
		bc   *model.TaskBillingContext
		want SeedanceTaskUsage
		ok   bool
	}{
		{
			name: "version one snapshot",
			bc: &model.TaskBillingContext{
				UsageSnapshotVersion: model.TaskUsageSnapshotVersion1,
				UsageCompletionTokens: 108000,
				UsageTotalTokens:      172800,
			},
			want: SeedanceTaskUsage{CompletionTokens: 108000, TotalTokens: 172800},
			ok:   true,
		},
		{name: "legacy version", bc: &model.TaskBillingContext{UsageCompletionTokens: 108000, UsageTotalTokens: 108000}},
		{name: "partial snapshot", bc: &model.TaskBillingContext{UsageSnapshotVersion: model.TaskUsageSnapshotVersion1, UsageCompletionTokens: 108000}},
		{name: "invalid relation", bc: &model.TaskBillingContext{UsageSnapshotVersion: model.TaskUsageSnapshotVersion1, UsageCompletionTokens: 108000, UsageTotalTokens: 100000}},
		{name: "over limit", bc: &model.TaskBillingContext{UsageSnapshotVersion: model.TaskUsageSnapshotVersion1, UsageCompletionTokens: relaycommon.MaxTokensLimit + 1, UsageTotalTokens: relaycommon.MaxTokensLimit + 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := PersistedSeedanceTaskUsage(tt.bc)
			assert.Equal(t, tt.ok, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}
```

- [ ] **Step 2: 运行测试确认 RED**

Run: `go test ./service -run '^TestPersistedSeedanceTaskUsage$' -count=1`

Expected: FAIL，提示 `UsageSnapshotVersion`、`UsageCompletionTokens`、`UsageTotalTokens`、`TaskUsageSnapshotVersion1` 或 `PersistedSeedanceTaskUsage` 尚未定义。

- [ ] **Step 3: 增加最小数据契约和共享验证函数**

在 `model/task.go` 增加：

```go
const TaskUsageSnapshotVersion1 = 1

UsageSnapshotVersion  int `json:"usage_snapshot_version,omitempty"`
UsageCompletionTokens int `json:"usage_completion_tokens,omitempty"`
UsageTotalTokens      int `json:"usage_total_tokens,omitempty"`
```

字段放在现有 `BillingTokens`、`UsageProfile`、`UsageSource` 附近，不重排无关字段。

在 `relay/common/relay_info.go` 的 `TaskRelayInfo` 增加临时传递字段：

```go
UsageCompletionTokens int
UsageTotalTokens      int
```

在 `service/seedance_task_usage.go` 增加通用验证和持久化读取：

```go
func IsValidSeedanceUsage(completionTokens, totalTokens int64) bool {
	return completionTokens > 0 &&
		totalTokens >= completionTokens &&
		completionTokens <= int64(relaycommon.MaxTokensLimit) &&
		totalTokens <= int64(relaycommon.MaxTokensLimit)
}

func IsValidSeedanceUpstreamUsage(completionTokens, totalTokens int64) bool {
	return IsValidSeedanceUsage(completionTokens, totalTokens)
}

func PersistedSeedanceTaskUsage(bc *model.TaskBillingContext) (SeedanceTaskUsage, bool) {
	if bc == nil || bc.UsageSnapshotVersion != model.TaskUsageSnapshotVersion1 ||
		!IsValidSeedanceUsage(int64(bc.UsageCompletionTokens), int64(bc.UsageTotalTokens)) {
		return SeedanceTaskUsage{}, false
	}
	return SeedanceTaskUsage{
		CompletionTokens: bc.UsageCompletionTokens,
		TotalTokens:      bc.UsageTotalTokens,
	}, true
}
```

- [ ] **Step 4: 运行测试确认 GREEN**

Run: `go test ./service -run 'TestPersistedSeedanceTaskUsage|TestCalculateSeedanceTaskUsage' -count=1`

Expected: PASS。

- [ ] **Step 5: 提交数据契约**

```bash
git add model/task.go relay/common/relay_info.go service/seedance_task_usage.go service/seedance_task_usage_test.go
git commit -m "feat: add versioned Seedance usage snapshots"
```

### 任务二：在供应商调用前生成 usage 快照

**Files:**
- Modify: `relay/relay_task.go`
- Modify: `relay/relay_task_usage_inputs_test.go`

- [ ] **Step 1: 写提交前计算的失败测试**

在 `relay/relay_task_usage_inputs_test.go` 增加：

```go
func TestPrepareSeedanceUsageSnapshot(t *testing.T) {
	tests := []struct {
		name             string
		inputDurationMS  int64
		hasVideoInput    bool
		resolution       string
		wantCompletion   int
		wantTotal        int
		wantError        string
	}{
		{name: "text or image input", resolution: "720p", wantCompletion: 108000, wantTotal: 108000},
		{name: "reference video", inputDurationMS: 3000, hasVideoInput: true, resolution: "720p", wantCompletion: 108000, wantTotal: 172800},
		{name: "missing reference duration", hasVideoInput: true, resolution: "720p", wantError: "reference video duration is unavailable"},
		{name: "unsupported resolution", resolution: "bad", wantError: "output resolution is unsupported"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := &relaycommon.RelayInfo{TaskRelayInfo: &relaycommon.TaskRelayInfo{InputVideoDurationMS: tt.inputDurationMS}}
			err := prepareSeedanceUsageSnapshot(info, 5, tt.resolution, tt.hasVideoInput)
			if tt.wantError != "" {
				require.EqualError(t, err, tt.wantError)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantCompletion, info.TaskRelayInfo.UsageCompletionTokens)
			assert.Equal(t, tt.wantTotal, info.TaskRelayInfo.UsageTotalTokens)
		})
	}
}
```

- [ ] **Step 2: 运行测试确认 RED**

Run: `go test ./relay -run '^TestPrepareSeedanceUsageSnapshot$' -count=1`

Expected: FAIL，提示 `prepareSeedanceUsageSnapshot` 未定义。

- [ ] **Step 3: 实现提交快照业务函数**

在 `relay/relay_task.go` 的 `prepareSeedanceUsageInputs` 前增加：

```go
func prepareSeedanceUsageSnapshot(info *relaycommon.RelayInfo, requestedSeconds int, resolution string, hasVideoInput bool) error {
	if info == nil || info.TaskRelayInfo == nil {
		return errors.New("task relay info is unavailable")
	}
	usage, err := service.CalculateSeedanceTaskUsage(&model.TaskBillingContext{
		UsageProfile:             model.TaskUsageProfileSeedance,
		RequestedDurationSeconds: requestedSeconds,
		Resolution:               resolution,
		HasVideoInput:            hasVideoInput,
		InputVideoDurationMS:     info.TaskRelayInfo.InputVideoDurationMS,
	}, service.SeedanceTerminalFacts{})
	if err != nil {
		return err
	}
	if !service.IsValidSeedanceUsage(int64(usage.CompletionTokens), int64(usage.TotalTokens)) {
		return errors.New("calculated Seedance usage is invalid")
	}
	info.TaskRelayInfo.UsageCompletionTokens = usage.CompletionTokens
	info.TaskRelayInfo.UsageTotalTokens = usage.TotalTokens
	return nil
}
```

- [ ] **Step 4: 把快照计算接入 `RelayTaskSubmit`**

在 `seedanceTask` 分支中保存最终 `hasVideoInput`，完成参考视频元数据准备后、进入按时长价格计算前调用：

```go
if err := prepareSeedanceUsageSnapshot(info, requestedSeconds, resolutionProfile.Name, hasVideoInput); err != nil {
	return nil, service.TaskErrorWrapperLocal(err, "video_usage_context_unavailable", http.StatusInternalServerError)
}
```

保持请求输入或媒体验证原有的 400/503 错误优先返回；该 500 只代表已经通过请求验证但内部无法生成快照。确认调用位置严格早于 `adaptor.DoRequest`。

- [ ] **Step 5: 运行相关测试确认 GREEN**

Run: `go test ./relay -run 'TestPrepareSeedanceUsageInputs|TestPrepareSeedanceUsageSnapshot|TestRelayTask' -count=1`

Expected: PASS。

- [ ] **Step 6: 提交提交门禁**

```bash
git add relay/relay_task.go relay/relay_task_usage_inputs_test.go
git commit -m "fix: lock Seedance usage before dispatch"
```

### 任务三：持久化版本和 token 对

**Files:**
- Modify: `controller/relay.go`
- Modify: `controller/cost_task_relay_test.go`

- [ ] **Step 1: 扩展持久化失败测试**

修改 `TestPersistSubmittedSeedanceTaskStoresUsageProfileWithoutPerCallBilling` 的 `TaskRelayInfo`：

```go
TaskRelayInfo: &relaycommon.TaskRelayInfo{
	PublicTaskID:          "task-seedance-profile",
	UsageCompletionTokens: 108000,
	UsageTotalTokens:      172800,
},
```

并新增断言：

```go
assert.Equal(t, model.TaskUsageSnapshotVersion1, task.PrivateData.BillingContext.UsageSnapshotVersion)
assert.Equal(t, 108000, task.PrivateData.BillingContext.UsageCompletionTokens)
assert.Equal(t, 172800, task.PrivateData.BillingContext.UsageTotalTokens)
```

- [ ] **Step 2: 运行测试确认 RED**

Run: `go test ./controller -run '^TestPersistSubmittedSeedanceTaskStoresUsageProfileWithoutPerCallBilling$' -count=1`

Expected: FAIL，持久化上下文中的版本和 token 对仍为零。

- [ ] **Step 3: 在 `persistSubmittedTask` 复制快照**

在确定 `usageProfile` 后，对新 Seedance 任务验证临时 token 对：

```go
usageSnapshotVersion := 0
usageCompletionTokens := 0
usageTotalTokens := 0
if usageProfile == model.TaskUsageProfileSeedance {
	if relayInfo.TaskRelayInfo == nil || !service.IsValidSeedanceUsage(
		int64(relayInfo.TaskRelayInfo.UsageCompletionTokens),
		int64(relayInfo.TaskRelayInfo.UsageTotalTokens),
	) {
		return errors.New("submitted Seedance task is missing its usage snapshot")
	}
	usageSnapshotVersion = model.TaskUsageSnapshotVersion1
	usageCompletionTokens = relayInfo.TaskRelayInfo.UsageCompletionTokens
	usageTotalTokens = relayInfo.TaskRelayInfo.UsageTotalTokens
}
```

在 `TaskBillingContext` 字面量中增加：

```go
UsageSnapshotVersion:  usageSnapshotVersion,
UsageCompletionTokens: usageCompletionTokens,
UsageTotalTokens:      usageTotalTokens,
```

非 Seedance 任务保持三个字段的零值。

- [ ] **Step 4: 运行控制器测试确认 GREEN**

Run: `go test ./controller -run 'TestPersistSubmittedTask|TestPersistSubmittedSeedanceTask' -count=1`

Expected: PASS。

- [ ] **Step 5: 提交持久化改动**

```bash
git add controller/relay.go controller/cost_task_relay_test.go
git commit -m "feat: persist Seedance usage snapshots"
```

### 任务四：终态选择最终 usage 并阻止损坏快照成功落库

**Files:**
- Modify: `service/seedance_task_usage.go`
- Modify: `service/seedance_task_usage_test.go`
- Modify: `service/task_polling.go`
- Modify: `service/task_polling_test.go`

- [ ] **Step 1: 写终态优先级和兼容性失败测试**

在 `TestNormalizeSeedanceTaskUsage` 增加四个子测试：

```go
t.Run("authoritative usage replaces persisted pair", func(t *testing.T) {
	task := &model.Task{PrivateData: model.TaskPrivateData{BillingContext: &model.TaskBillingContext{
		UsageProfile: model.TaskUsageProfileSeedance, UsageSnapshotVersion: model.TaskUsageSnapshotVersion1,
		UsageCompletionTokens: 108000, UsageTotalTokens: 108000,
	}}}
	result := &relaycommon.TaskInfo{Status: string(model.TaskStatusSuccess), CompletionTokens: 108900, CompletionTokensPresent: true, TotalTokens: 109000, TotalTokensPresent: true}
	require.NoError(t, NormalizeSeedanceTaskUsage(task, result))
	assert.Equal(t, 108900, task.PrivateData.BillingContext.UsageCompletionTokens)
	assert.Equal(t, 109000, task.PrivateData.BillingContext.UsageTotalTokens)
	assert.Equal(t, model.TaskUsageSourceUpstream, task.PrivateData.BillingContext.UsageSource)
})

t.Run("terminal calculation replaces request fallback", func(t *testing.T) {
	bc := &model.TaskBillingContext{
		UsageProfile: model.TaskUsageProfileSeedance, UsageSnapshotVersion: model.TaskUsageSnapshotVersion1,
		RequestedDurationSeconds: 5, Resolution: "720p", UsageCompletionTokens: 108000, UsageTotalTokens: 108000,
	}
	task := &model.Task{PrivateData: model.TaskPrivateData{BillingContext: bc}}
	result := &relaycommon.TaskInfo{Status: string(model.TaskStatusSuccess), FramesPerSecond: 30, FramesPerSecondPresent: true}
	require.NoError(t, NormalizeSeedanceTaskUsage(task, result))
	assert.Equal(t, 135000, bc.UsageCompletionTokens)
	assert.Equal(t, 135000, bc.UsageTotalTokens)
})

t.Run("broken request facts use persisted fallback", func(t *testing.T) {
	bc := &model.TaskBillingContext{
		UsageProfile: model.TaskUsageProfileSeedance, UsageSnapshotVersion: model.TaskUsageSnapshotVersion1,
		UsageCompletionTokens: 108000, UsageTotalTokens: 172800, HasVideoInput: true,
	}
	task := &model.Task{PrivateData: model.TaskPrivateData{BillingContext: bc}}
	result := &relaycommon.TaskInfo{Status: string(model.TaskStatusSuccess)}
	require.NoError(t, NormalizeSeedanceTaskUsage(task, result))
	assert.Equal(t, 108000, result.CompletionTokens)
	assert.Equal(t, 172800, result.TotalTokens)
})

t.Run("versioned task rejects missing fallback", func(t *testing.T) {
	bc := &model.TaskBillingContext{UsageProfile: model.TaskUsageProfileSeedance, UsageSnapshotVersion: model.TaskUsageSnapshotVersion1, HasVideoInput: true}
	task := &model.Task{PrivateData: model.TaskPrivateData{BillingContext: bc}}
	err := NormalizeSeedanceTaskUsage(task, &relaycommon.TaskInfo{Status: string(model.TaskStatusSuccess)})
	require.EqualError(t, err, "versioned Seedance usage snapshot is unavailable")
})
```

保留并扩展历史兼容测试：`UsageSnapshotVersion=0` 且旧计费上下文无法计算时，`NormalizeSeedanceTaskUsage` 返回 nil，不生成 usage，也不阻止历史任务完成。

- [ ] **Step 2: 运行服务测试确认 RED**

Run: `go test ./service -run '^TestNormalizeSeedanceTaskUsage$' -count=1`

Expected: FAIL，当前实现不持久化最终 total token，也不会使用提交前 token 对。

- [ ] **Step 3: 实现统一写回和三层选择**

在 `service/seedance_task_usage.go` 增加内部写回函数：

```go
func applySeedanceTaskUsage(task *model.Task, result *relaycommon.TaskInfo, usage SeedanceTaskUsage, source string) {
	result.CompletionTokens = usage.CompletionTokens
	result.TotalTokens = usage.TotalTokens
	result.CompletionTokensPresent = true
	result.TotalTokensPresent = true
	result.UsageSource = source
	bc := task.PrivateData.BillingContext
	bc.UsageCompletionTokens = usage.CompletionTokens
	bc.UsageTotalTokens = usage.TotalTokens
	bc.BillingTokens = usage.CompletionTokens
	bc.UsageSource = source
}
```

在 `service/seedance_task_usage.go` 增加 `errors` import，并把 `NormalizeSeedanceTaskUsage` 替换为以下完整实现：

```go
func NormalizeSeedanceTaskUsage(task *model.Task, result *relaycommon.TaskInfo) error {
	if task == nil || result == nil || model.TaskStatus(result.Status) != model.TaskStatusSuccess {
		return nil
	}
	billingContext := task.PrivateData.BillingContext
	if billingContext == nil || billingContext.UsageProfile != model.TaskUsageProfileSeedance {
		return nil
	}

	if result.BillingClamp == nil &&
		result.CompletionTokensPresent && result.TotalTokensPresent &&
		IsValidSeedanceUpstreamUsage(int64(result.CompletionTokens), int64(result.TotalTokens)) {
		applySeedanceTaskUsage(task, result, SeedanceTaskUsage{
			CompletionTokens: result.CompletionTokens,
			TotalTokens:      result.TotalTokens,
		}, model.TaskUsageSourceUpstream)
		return nil
	}

	usage, err := CalculateSeedanceTaskUsage(billingContext, SeedanceTerminalFacts{
		DurationSeconds:        result.DurationSeconds,
		DurationPresent:        result.DurationPresent,
		Resolution:             result.Resolution,
		ResolutionPresent:      result.ResolutionPresent,
		FramesPerSecond:        result.FramesPerSecond,
		FramesPerSecondPresent: result.FramesPerSecondPresent,
	})
	if err == nil {
		applySeedanceTaskUsage(task, result, usage, model.TaskUsageSourceLocalCalculated)
		return nil
	}
	if usage, ok := PersistedSeedanceTaskUsage(billingContext); ok {
		applySeedanceTaskUsage(task, result, usage, model.TaskUsageSourceLocalCalculated)
		return nil
	}
	if billingContext.UsageSnapshotVersion == model.TaskUsageSnapshotVersion1 {
		return errors.New("versioned Seedance usage snapshot is unavailable")
	}
	return nil
}
```

合法上游 usage 仍要求没有 `BillingClamp`。不要把本地值写入 `CostMeter`。

- [ ] **Step 4: 写轮询阻止成功 CAS 的失败测试**

在 `service/task_polling_test.go` 增加：

```go
func TestSeedanceVersionedTaskDoesNotFinalizeWithoutUsage(t *testing.T) {
	truncate(t)
	task := seedPollingTask(t, 0, "task_public_broken_usage", "upstream_broken_usage")
	task.Quota = 4000
	task.PrivateData.BillingContext = &model.TaskBillingContext{
		UsageProfile: model.TaskUsageProfileSeedance,
		UsageSnapshotVersion: model.TaskUsageSnapshotVersion1,
		HasVideoInput: true,
	}
	require.NoError(t, task.Update())
	adaptor := &taskPollingFetchAdaptor{parseResult: &relaycommon.TaskInfo{Status: string(model.TaskStatusSuccess), Url: "https://x/video.mp4"}}

	err := runSinglePollingUpdate(t, adaptor, task)

	require.ErrorContains(t, err, "versioned Seedance usage snapshot is unavailable")
	var stored model.Task
	require.NoError(t, model.DB.First(&stored, task.ID).Error)
	assert.Equal(t, model.TaskStatus(model.TaskStatusInProgress), stored.Status)
	assert.Equal(t, 4000, stored.Quota)
}
```

- [ ] **Step 5: 运行轮询测试确认 RED**

Run: `go test ./service -run '^TestSeedanceVersionedTaskDoesNotFinalizeWithoutUsage$' -count=1`

Expected: FAIL，当前轮询只记录归一化错误并继续提交成功状态。

- [ ] **Step 6: 把归一化移动到成功状态变更之前**

在 `updateVideoSingleTask` 解析并清洗 `taskResult` 后、写入 `task.Data` 和修改 `task.Status` 前增加：

```go
if model.TaskStatus(taskResult.Status) == model.TaskStatusSuccess {
	if err := NormalizeSeedanceTaskUsage(task, taskResult); err != nil {
		logger.LogError(ctx, fmt.Sprintf(
			"Seedance task usage normalization failed: task_id=%s channel_id=%d cost_request_id=%d error=%s",
			task.TaskID, task.ChannelId, task.PrivateData.CostRequestID,
			sanitizeTaskPollingText(err.Error(), privateTaskID),
		))
		return fmt.Errorf("normalize Seedance task usage for task %s: %s", task.TaskID, sanitizeTaskPollingText(err.Error(), privateTaskID))
	}
}
```

删除 switch 后现有“只记录错误”的归一化块。提前返回发生在 `shouldRefund`、`UpdateWithStatus`、用户差额结算和供应商成本结算之前，因此保持预扣，不退款，也不提交成功 CAS。

- [ ] **Step 7: 运行服务回归确认 GREEN**

Run: `go test ./service -run 'TestNormalizeSeedanceTaskUsage|TestSeedanceTaskPolling|TestSeedanceVersionedTask|TestSeedanceLocalUsageDoesNotBecomeSupplierTokenMeter' -count=1`

Expected: PASS。

- [ ] **Step 8: 提交终态归一化**

```bash
git add service/seedance_task_usage.go service/seedance_task_usage_test.go service/task_polling.go service/task_polling_test.go
git commit -m "fix: finalize Seedance tasks with persisted usage"
```

### 任务五：公共响应只读取最终持久化 usage

**Files:**
- Modify: `relay/seedance_task.go`
- Modify: `relay/relay_task_seedance_test.go`

- [ ] **Step 1: 写新快照公共响应失败测试**

增加两个测试：原始响应缺 usage 但持久化来源为 upstream 时仍返回最终值；版本 1 快照损坏时拒绝成功响应。

```go
func TestSeedanceTaskResponseUsesPersistedFinalUsage(t *testing.T) {
	task := &model.Task{
		TaskID: "task_public_persisted_usage", Platform: constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeNewAPIVideo)),
		Status: model.TaskStatusSuccess, Properties: model.Properties{OriginModelName: "doubao-seedance-2-0-260128"},
		PrivateData: model.TaskPrivateData{BillingContext: &model.TaskBillingContext{
			UsageProfile: model.TaskUsageProfileSeedance, UsageSnapshotVersion: model.TaskUsageSnapshotVersion1,
			UsageCompletionTokens: 108900, UsageTotalTokens: 109000, UsageSource: model.TaskUsageSourceUpstream,
		}},
		Data: json.RawMessage(`{"status":"succeeded","content":{"video_url":"https://x/video.mp4"}}`),
	}
	response, err := seedanceTaskResponse(task)
	require.NoError(t, err)
	usage := response["usage"].(map[string]interface{})
	assert.EqualValues(t, 108900, usage["completion_tokens"])
	assert.EqualValues(t, 109000, usage["total_tokens"])
}

func TestSeedanceTaskResponseRejectsBrokenVersionedUsage(t *testing.T) {
	task := &model.Task{
		TaskID: "task_public_broken_usage", Platform: constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeNewAPIVideo)),
		Status: model.TaskStatusSuccess, Properties: model.Properties{OriginModelName: "doubao-seedance-2-0-260128"},
		PrivateData: model.TaskPrivateData{BillingContext: &model.TaskBillingContext{
			UsageProfile: model.TaskUsageProfileSeedance, UsageSnapshotVersion: model.TaskUsageSnapshotVersion1,
			UsageSource: model.TaskUsageSourceLocalCalculated,
		}},
		Data: json.RawMessage(`{"status":"succeeded","content":{"video_url":"https://x/video.mp4"}}`),
	}
	_, err := seedanceTaskResponse(task)
	require.ErrorContains(t, err, "Seedance terminal usage is unavailable")
}
```

- [ ] **Step 2: 运行测试确认 RED**

Run: `go test ./relay -run 'TestSeedanceTaskResponseUsesPersistedFinalUsage|TestSeedanceTaskResponseRejectsBrokenVersionedUsage' -count=1`

Expected: FAIL，当前响应仍依赖原始 payload 或现场重算，并且损坏快照会静默返回缺 usage 的成功响应。

- [ ] **Step 3: 修改公共响应契约**

把 `populateSeedanceTaskUsage` 改为返回 error，并在 `seedanceTaskResponse` 中传播：

```go
if err := populateSeedanceTaskUsage(task, response); err != nil {
	return nil, err
}
```

在 `populateSeedanceTaskUsage` 开头加入版本 1 分支：

```go
if billingContext != nil && billingContext.UsageSnapshotVersion == model.TaskUsageSnapshotVersion1 {
	if billingContext.UsageSource != model.TaskUsageSourceUpstream &&
		billingContext.UsageSource != model.TaskUsageSourceLocalCalculated {
		return errors.New("Seedance terminal usage is unavailable")
	}
	usage, ok := service.PersistedSeedanceTaskUsage(billingContext)
	if !ok {
		return errors.New("Seedance terminal usage is unavailable")
	}
	response["usage"] = map[string]interface{}{
		"completion_tokens": usage.CompletionTokens,
		"total_tokens":      usage.TotalTokens,
	}
	return nil
}
```

版本 0 历史任务继续执行当前的原始 usage 校验和可信上下文重算逻辑。所有早退改为 `return nil`，计算或序列化失败仍按历史尽力兼容，不升级为新契约错误。

- [ ] **Step 4: 更新两个既有新任务 fixture**

在 `TestSeedanceTaskResponseCalculatesUsageForPerTokenUpstream` 中设置：

```go
UsageSnapshotVersion:  model.TaskUsageSnapshotVersion1,
UsageCompletionTokens: 108000,
UsageTotalTokens:      108000,
UsageSource:           model.TaskUsageSourceLocalCalculated,
```

在 `TestSeedanceTaskResponseReplacesOverLimitUpstreamUsageWithSettledUsage` 的归一化前计费上下文中设置版本 1 和 108000/108000 提交快照，归一化后继续断言持久化 token 对仍为 108000/108000。保留其他没有版本字段的用例，证明历史任务仍走尽力恢复。

- [ ] **Step 5: 运行公共响应回归确认 GREEN**

Run: `go test ./relay -run '^TestSeedanceTaskResponse' -count=1`

Expected: PASS。

- [ ] **Step 6: 提交公共响应改动**

```bash
git add relay/seedance_task.go relay/relay_task_seedance_test.go
git commit -m "fix: serve persisted Seedance terminal usage"
```

### 任务六：强化 Seedance 矩阵并完成验证

**Files:**
- Modify: `e2e/seedance_material_matrix_e2e_test.go`
- Modify: `e2e/paipu_upstream_e2e_test.go`

- [ ] **Step 1: 在轮询前断言提交快照**

在素材矩阵读取刚提交的 task 后增加：

```go
require.NotNil(t, task.PrivateData.BillingContext, target.CaseID)
require.Equal(t, model.TaskUsageSnapshotVersion1, task.PrivateData.BillingContext.UsageSnapshotVersion, target.CaseID)
require.Positive(t, task.PrivateData.BillingContext.UsageCompletionTokens, target.CaseID)
require.GreaterOrEqual(t, task.PrivateData.BillingContext.UsageTotalTokens, task.PrivateData.BillingContext.UsageCompletionTokens, target.CaseID)
```

该断言发生在 `RunTaskPollingOnce` 之前，证明 token 对是在供应商提交阶段锁定，而不是终态补写。

- [ ] **Step 2: 在轮询后断言最终持久化 usage**

在现有公共 usage 断言旁增加：

```go
require.Equal(t, task.PrivateData.BillingContext.UsageCompletionTokens, publicTask.Usage.CompletionTokens, target.CaseID)
require.Equal(t, task.PrivateData.BillingContext.UsageTotalTokens, publicTask.Usage.TotalTokens, target.CaseID)
require.Equal(t, task.PrivateData.BillingContext.UsageCompletionTokens, task.PrivateData.BillingContext.BillingTokens, target.CaseID)
```

保留按 token 差额结算、按时长账单、供应商 `upstream_usage` 隔离和两条日志的既有断言。

- [ ] **Step 3: 扩展 Paipu 缺 usage 生命周期断言**

在 `TestPaipuMissingUsageUsesValidatedResolutionE2E` 中，提交后、调用 `pollNewAPIVideoTask` 前读取任务并断言版本 1，以及 1080p 对应 243000/243000、4k 对应 972000/972000 的提交快照。轮询后断言最终 token 对与公共 usage 一致。

- [ ] **Step 4: 运行针对性 E2E**

Run: `go test ./e2e -run 'TestPaipuMissingUsageUsesValidatedResolutionE2E|TestFourSToken|TestSeedanceImportedMaterialMatrixFullFlowE2E' -count=1`

Expected: PASS。

- [ ] **Step 5: 格式化并运行完整验证**

```bash
gofmt -w model/task.go relay/common/relay_info.go service/seedance_task_usage.go service/seedance_task_usage_test.go relay/relay_task.go relay/relay_task_usage_inputs_test.go controller/relay.go controller/cost_task_relay_test.go service/task_polling.go service/task_polling_test.go relay/seedance_task.go relay/relay_task_seedance_test.go e2e/seedance_material_matrix_e2e_test.go e2e/paipu_upstream_e2e_test.go
go test ./... -count=1
go vet ./relay/channel/task/taskcommon ./relay/channel/task/newapivideo ./relay/channel/task/doubao ./service ./relay ./controller ./e2e
git diff --check
git status --short
```

Expected: 全仓测试退出码为 0；相关范围 `go vet` 退出码为 0；`git diff --check` 无输出；状态只包含本计划涉及的文件。

- [ ] **Step 6: 提交矩阵与最终回归**

```bash
git add e2e/seedance_material_matrix_e2e_test.go e2e/paipu_upstream_e2e_test.go
git commit -m "test: enforce Seedance usage snapshots across matrix"
```
