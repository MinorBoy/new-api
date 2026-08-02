# Seedance 终态计费 Usage 归一化实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (- [ ]) syntax for tracking.

**目标：** 保证所有新提交且成功完成的 Seedance 任务都具有可信 usage；上游 usage 缺失时，在用户结算前用已验证请求事实本地计算，同时保持用户按时长计费和供应商成本台账语义不变。

**架构：** 依据[设计文档](../specs/2026-08-02-seedance-terminal-billing-usage-design.md)，提交阶段持久化 seedance usage 画像及完整请求事实；轮询阶段在通用服务中按画像归一化 usage，适配器只提供终态事实。共享纯计算核心同时服务于轮询结算和 Ark 公共响应，本地计算值通过来源标记与供应商权威 usage 隔离。

**技术栈：** Go 1.22、Gin、GORM、shopspring/decimal、testify、SQLite E2E。

---

## 文件边界

- model/task.go：定义持久化 usage 画像、来源和 Seedance 用户计费语义。
- relay/common/relay_info.go：承载适配器解析出的终态时长、分辨率、帧率及存在性。
- service/seedance_task_usage.go：唯一的 Seedance usage 纯计算与轮询归一化入口。
- relay/relay_task.go：在上游发送前准备全部 usage 请求事实，不再依赖成本核算模式。
- controller/relay.go：持久化画像并区分按时长与旧按次跳过语义。
- relay/channel/task/newapivideo/response.go：只解析终态事实，不实现计费公式。
- service/task_polling.go：在终态 CAS 前归一化 usage、结算用户额度并隔离供应商 meter。
- relay/seedance_task.go：公共响应复用共享计算核心。
- e2e/seedance_material_matrix_e2e_test.go：移除 Paipu/4stoken 单日志例外并验证 usage、日志与成本台账。

### 任务一：定义持久化契约和共享纯计算核心

**文件：**
- 修改：model/task.go:127-149
- 修改：relay/common/relay_info.go:962-978
- 新建：service/seedance_task_usage.go
- 新建：service/seedance_task_usage_test.go

- [ ] **步骤 1：先写共享计算失败测试**

使用表驱动测试覆盖：请求快照计算 108000/108000、3 秒参考视频得到 108000/172800、合法终态 30 fps 优先得到 135000/135000、非法终态事实回退请求快照、参考视频缺少聚合时长时报错、请求时长或 token 越界时报错。

~~~go
func TestCalculateSeedanceTaskUsage(t *testing.T) {
	bc := &model.TaskBillingContext{
		UsageProfile: model.TaskUsageProfileSeedance,
		RequestedDurationSeconds: 5,
		Resolution: "720p",
	}
	usage, err := CalculateSeedanceTaskUsage(bc, SeedanceTerminalFacts{})
	require.NoError(t, err)
	assert.Equal(t, 108000, usage.CompletionTokens)
	assert.Equal(t, 108000, usage.TotalTokens)
}
~~~

- [ ] **步骤 2：运行测试确认 RED**

运行：go test ./service -run '^TestCalculateSeedanceTaskUsage$'

预期：FAIL，提示 CalculateSeedanceTaskUsage、SeedanceTerminalFacts 或新字段尚未定义。

- [ ] **步骤 3：实现最小数据契约和纯计算函数**

model/task.go 增加 TaskUsageProfileSeedance、TaskUsageSourceUpstream、TaskUsageSourceLocalCalculated 常量，并给 TaskBillingContext 增加可选 UsageProfile、UsageSource 字段。TaskInfo 增加 DurationSeconds/DurationPresent、ResolutionPresent、FramesPerSecond/FramesPerSecondPresent 和 UsageSource 内部字段。

service/seedance_task_usage.go 定义 SeedanceTerminalFacts、SeedanceTaskUsage 和 CalculateSeedanceTaskUsage。函数按“合法终态事实优先，否则请求快照”选择时长、分辨率、帧率，调用 seedancepricing.Profile 与 EstimateSeedanceTokens，拒绝缺少参考视频时长及所有越界输入。

- [ ] **步骤 4：运行测试确认 GREEN**

运行：go test ./service -run '^TestCalculateSeedanceTaskUsage$'

预期：PASS。

- [ ] **步骤 5：提交本阶段**

~~~bash
git add model/task.go relay/common/relay_info.go service/seedance_task_usage.go service/seedance_task_usage_test.go
git commit -m "feat: add Seedance terminal usage calculation"
~~~

### 任务二：在提交阶段建立 usage 强保证

**文件：**
- 修改：relay/relay_task.go:230-290,449-479
- 修改：relay/channel/task/doubao/adaptor.go
- 修改：relay/channel/task/doubao/adaptor_test.go
- 修改：relay/relay_task_usage_inputs_test.go
- 修改：controller/relay.go:769-815
- 修改：controller/cost_task_relay_test.go

- [ ] **步骤 1：先写全成本模式准备与持久化失败测试**

把既有跳过测试改为验证 disabled、free、per_token、per_request、per_duration 均读取参考视频元数据并保存聚合时长。新增 persistSubmittedTask 测试，断言 Seedance 任务持久化 UsageProfile=seedance，供应商 per_request 不会令 PerCallBilling=true，按时长仍由 BillingMode 表达。

~~~go
for _, costMode := range []types.CostMode{"", types.CostModeFree, types.CostModePerToken, types.CostModePerRequest, types.CostModePerDuration} {
	taskErr := prepareSeedanceUsageInputs(t.Context(), retryParam, info)
	require.Nil(t, taskErr)
	assert.Equal(t, int64(2500), info.TaskRelayInfo.InputVideoDurationMS)
}
~~~

- [ ] **步骤 2：运行测试确认 RED**

运行：go test ./relay ./controller -run 'TestPrepareSeedanceUsageInputs|TestPersistSubmittedSeedanceTask'

预期：FAIL，当前 per_token/free/disabled 被跳过且计费上下文没有画像。

- [ ] **步骤 3：实现提交门禁和持久化**

RelayTaskSubmit 完成请求校验及 EstimateBilling 后，对 Seedance 任务统一调用 TaskDurationEstimator 保存 RequestedDurationSeconds，校验 seedancepricing.Profile，并在存在参考视频时调用去掉 costMode 参数的 prepareSeedanceUsageInputs。Doubao 原生适配器补齐该接口：显式正整数使用已验证值，省略时长和 `duration=-1` 智能时长保存 5 秒的确定性回退快照；终态返回合法实际时长时仍由终态事实优先。按时长价格继续使用同一快照，不重复读取元数据。

persistSubmittedTask 按模型族写入 UsageProfile。Seedance 的 PerCallBilling 恒为 false；非 Seedance 保留 TaskPricePatches/UsePrice 旧语义。BillingModePerDuration 由结算阶段单独识别。

- [ ] **步骤 4：运行测试确认 GREEN**

运行：go test ./relay ./controller -run 'TestPrepareSeedanceUsageInputs|TestPersistSubmittedSeedanceTask'

预期：PASS。

- [ ] **步骤 5：提交本阶段**

~~~bash
git add relay/relay_task.go relay/relay_task_usage_inputs_test.go controller/relay.go controller/cost_task_relay_test.go
git commit -m "fix: prepare Seedance usage facts before dispatch"
~~~

### 任务三：让 newapivideo 暴露终态事实

**文件：**
- 修改：relay/channel/task/newapivideo/response.go:20-70
- 修改：relay/channel/task/newapivideo/response_test.go

- [ ] **步骤 1：写适配器事实解析失败测试**

新增表驱动测试，验证合法 duration/resolution/framespersecond 的值与 presence；分数、零、负数、超过 MaxTaskDurationSeconds 或 240 fps 的值保持 presence 但不产生合法值；usage presence 仍按上游 JSON 独立解析。

- [ ] **步骤 2：运行测试确认 RED**

运行：go test ./relay/channel/task/newapivideo -run '^TestParseTaskResultExposesTerminalUsageFacts$'

预期：FAIL，新终态字段不存在或未赋值。

- [ ] **步骤 3：实现事实解析**

ParseTaskResult 从 parsed.Nested 复制分辨率存在性，并用 json.Number.Int64 严格接受正整数时长和帧率。非法值只留下 presence，供共享核心回退提交快照；现有 CostMeter 时长逻辑保持原样。

- [ ] **步骤 4：运行测试确认 GREEN**

运行：go test ./relay/channel/task/newapivideo -run '^TestParseTaskResultExposesTerminalUsageFacts$'

预期：PASS。

- [ ] **步骤 5：提交本阶段**

~~~bash
git add relay/channel/task/newapivideo/response.go relay/channel/task/newapivideo/response_test.go
git commit -m "feat: expose video terminal usage facts"
~~~

### 任务四：在终态 CAS 前归一化并正确结算

**文件：**
- 修改：service/seedance_task_usage.go
- 修改：service/task_polling.go:650-855
- 修改：service/task_polling_test.go

- [ ] **步骤 1：写轮询归一化和结算失败测试**

覆盖：有 seedance 画像且上游 usage 缺失时设置 108000/108000、presence 和 local_calculated；合法 usage 原样保留并标记 upstream；非 Seedance 不处理；用户 token 计费生成差额并写回 BillingTokens；按时长任务保存 usage 但不执行 token 差额；供应商 per_token/upstream_usage 缺权威 usage 仍为 settlement_failed。

- [ ] **步骤 2：运行测试确认 RED**

运行：go test ./service -run 'TestNormalizeSeedanceTaskUsage|TestSeedanceTaskPolling|TestTaskPollingCostMissingAuthoritativeMeterFailsSettlement'

预期：新增测试 FAIL；既有供应商 meter 隔离测试仍 PASS。

- [ ] **步骤 3：实现归一化、结算边界和成本隔离**

在任务状态被置为成功后、preparePolledTaskCostSettlement 与 UpdateWithStatus 之前调用 NormalizeSeedanceTaskUsage。完整、正数、未 clamp 的上游 usage 标记 upstream；否则调用纯计算核心，设置两个 token 值、presence、BillingTokens 与 local_calculated。

settleTaskBillingOnComplete 先以 BillingModePerDuration 跳过 token 差额，旧 PerCallBilling 仅保留给非 Seedance 固定价任务。preparePolledTaskCostSettlement 在 UsageSource=local_calculated 时向成本 normalizer 传递清除 token 数值与 presence 的副本，同时保留适配器的 duration CostMeter，防止本地 usage 冒充供应商 usage。

- [ ] **步骤 4：运行测试确认 GREEN**

运行：go test ./service -run 'TestNormalizeSeedanceTaskUsage|TestSeedanceTaskPolling|TestTaskPollingCostMissingAuthoritativeMeterFailsSettlement'

预期：PASS。

- [ ] **步骤 5：提交本阶段**

~~~bash
git add service/seedance_task_usage.go service/task_polling.go service/task_polling_test.go
git commit -m "fix: normalize Seedance usage before task settlement"
~~~

### 任务五：公共响应复用共享核心

**文件：**
- 修改：relay/seedance_task.go:265-351
- 修改：relay/relay_task_seedance_test.go:851-1017

- [ ] **步骤 1：更新公共响应测试**

为新任务上下文增加 UsageProfile=seedance；将供应商 per_token 用例改为期望本地 usage；保留历史无画像且无可信输入时的零 usage 兼容测试；断言合法上游 usage 不覆盖且普通响应不出现 usage_source。

- [ ] **步骤 2：运行测试确认 RED**

运行：go test ./relay -run '^TestSeedanceTaskResponse'

预期：per-token 新任务用例 FAIL，当前实现按供应商成本模式跳过。

- [ ] **步骤 3：替换重复公式**

populateSeedanceTaskUsage 继续负责解析公共响应终态事实，但调用 service.CalculateSeedanceTaskUsage 生成 usage；新任务按 UsageProfile 判定，旧任务仅保留现有 per_request/per_duration 尽力兼容。删除本文件重复的分辨率、帧率和 token 公式，保留严格 json.Number 解析辅助函数。

- [ ] **步骤 4：运行测试确认 GREEN**

运行：go test ./relay -run '^TestSeedanceTaskResponse'

预期：PASS。

- [ ] **步骤 5：提交本阶段**

~~~bash
git add relay/seedance_task.go relay/relay_task_seedance_test.go
git commit -m "refactor: share Seedance terminal usage calculation"
~~~

### 任务六：恢复矩阵强断言并完成回归验证

**文件：**
- 修改：e2e/seedance_material_matrix_e2e_test.go:178-240

- [ ] **步骤 1：移除单日志例外并增加 usage/成本断言**

所有完成的 Seedance token 计费矩阵项统一断言 2 条日志、1 条终态差额日志、正数 BillingTokens，公共响应具有正数 usage。供应商 validated_request 时长规则继续断言请求时长 meter；供应商 per_token 缺权威 usage 时断言成本结算失败且不写本地 token meter。

- [ ] **步骤 2：运行矩阵确认行为**

运行：go test ./e2e -run '^TestSeedanceImportedMaterialMatrixFullFlowE2E$' -count=1

预期：PASS，Paipu/4stoken per_duration 不再需要单日志例外。

- [ ] **步骤 3：格式化并运行完整相关验证**

~~~bash
gofmt -w model/task.go relay/common/relay_info.go service/seedance_task_usage.go service/seedance_task_usage_test.go relay/relay_task.go relay/relay_task_usage_inputs_test.go controller/relay.go controller/cost_task_relay_test.go relay/channel/task/newapivideo/response.go relay/channel/task/newapivideo/response_test.go service/task_polling.go service/task_polling_test.go relay/seedance_task.go relay/relay_task_seedance_test.go e2e/seedance_material_matrix_e2e_test.go
go test ./relay/channel/task/newapivideo ./service ./relay ./controller ./e2e -count=1
git diff --check
git status --short
~~~

预期：所有测试退出码为 0，git diff --check 无输出，状态中只包含本计划涉及的文件。

- [ ] **步骤 4：提交最终矩阵变更**

~~~bash
git add e2e/seedance_material_matrix_e2e_test.go
git commit -m "test: require Seedance terminal usage across matrix"
~~~

### 任务七：补齐原生 Doubao 和既有 E2E fixture 兼容

**文件：**
- 修改：relay/channel/task/doubao/adaptor.go
- 修改：relay/channel/task/doubao/adaptor_test.go
- 修改：e2e/fourstoken_upstream_e2e_test.go
- 修改：e2e/lucen_upstream_e2e_test.go
- 修改：e2e/newapi_video_upstream_e2e_test.go
- 修改：e2e/omegaai_upstream_e2e_test.go
- 修改：e2e/paipu_upstream_e2e_test.go
- 修改：e2e/seedance_billing_matrix_e2e_test.go
- 修改：e2e/seedance_capability_routing_e2e_test.go
- 修改：e2e/seedance_native_e2e_test.go

- [x] **步骤 1：先写 Doubao 时长估算与终态事实失败测试**

覆盖显式时长、原生省略时长、`duration=-1` 智能时长，以及成功响应中的时长、分辨率、帧率和 token presence。非法或缺失终态事实必须保留 presence 语义并回退提交快照。

- [x] **步骤 2：实现 Doubao 契约**

实现 `TaskDurationEstimator`，只读取 `ValidateRequestAndSetAction` 已存入上下文的请求状态；省略和智能时长使用 5 秒提交快照，避免终态事实缺失时无法生成 usage。`ParseTaskResult` 暴露合法终态事实，供共享核心优先使用实际输出时长。

- [x] **步骤 3：补齐旧多模态 E2E 的确定性元数据依赖**

FourSToken、Lucen、NewAPIVideo、OmegaAI、Paipu、能力路由和原生 Doubao 生命周期 setup 显式安装确定性视频元数据 client，并在 cleanup 恢复。Seedance 计费矩阵的 metadata client 从既有 URL fixture 读取每段时长，使网关与 mock 上游使用同一测试事实，同时保留上游拒绝非法官方范围并触发退款的既有覆盖。生产提交门禁不降级，旧 fixture 只补齐新契约要求的外部依赖。

- [x] **步骤 4：运行完整相关回归**

~~~bash
go test ./relay/channel/task/doubao -count=1
go test ./e2e -run 'TestFourSToken|TestLucen|TestNewAPIVideo|TestOmegaAI|TestSeedanceBilling' -count=1
go test ./relay/channel/task/taskcommon ./relay/channel/task/newapivideo ./relay/channel/task/doubao ./service ./relay ./controller ./e2e -count=1
go vet ./relay/channel/task/taskcommon ./relay/channel/task/newapivideo ./relay/channel/task/doubao ./service ./relay ./controller ./e2e
git diff --check
~~~
