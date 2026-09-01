# Paipu MiniMax H3 渠道接入实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**目标：** 在不新增渠道类型、不改变客户端 Ark API 的前提下，让同一个 Paipu task adaptor 按三个 MiniMax H3 上游模型执行独立的请求校验、请求编码、路由合同、计费和配置导入。

**架构：** 保留 `constant.ChannelTypePaipu`、`NewPaipuTaskAdaptor` 及 `/v1/videos` 异步任务生命周期。新增按完整 `target.UpstreamModel` 查找的 H3 契约；未命中 H3 ID 时继续走现有 Paipu Seedance 协议。H3 模型 ID只存在于渠道模型映射和 route target，不加入 `pkg/modelrouting.CanonicalModels`，不进入 Seedance `/v1/video/generations` 公共协议。配置生成器为 `h3` 与 `h3官价` 建立独立的按次/按秒归一化路径，默认保持 disabled。

**技术栈：** Go 1.22、Gin、GORM、Testify、Ark 视频任务路由、Bun、TypeScript、ExcelJS、V1 配置导入状态机、httptest Mock 上游。

---

## 前置资料与工作区边界

- 协议事实来源：`docs/new-channels/cn-paipu.html`、`docs/new-channels/cn-paipu-h3.html`。
- 已完成设计：[2026-08-24-paipu-minimax-h3-channel-design.md](../specs/2026-08-24-paipu-minimax-h3-channel-design.md)。实施前再次核对模型 ID、字段、状态和价格单位；HTML 只决定协议，不决定最终模型目录。
- 当前工作区已有模板生成器相关未提交修改（`web/scripts/channel-model-template/*` 与 `docs/channel-model-template-generator.md`），执行本计划时保留这些修改，不使用 `git reset`、`git checkout` 或批量格式化覆盖它们。
- 没有 `PAIPU_API_KEY` 时仍完成 Mock/单元测试，但真实上游验收必须标记为 blocked，不能把 Mock 结果写成真实通过。

## 文件结构

- 新建：`relay/channel/task/newapivideo/paipu_h3_contract.go`，定义三个 H3 模型的固定能力、素材 URL 策略、分辨率 wire mode 和计费模式，并导出供 `relay` 路由合同调用的只读查找函数。
- 新建：`relay/channel/task/newapivideo/paipu_h3_contract_test.go`，表驱动验证模型查找和契约字段。
- 修改：`relay/channel/task/newapivideo/paipu_request.go`、`native.go`、`adaptor.go` 及对应测试，增加 H3 请求验证、编码和默认时长写回。
- 修改：`relay/video_route_contract.go`、`relay/video_route_contract_test.go`、`pkg/modelrouting/validate.go`、`pkg/modelrouting/validate_test.go`，使 Paipu H3 route target 表达 `1440p/720p/768p` 和 H3 边界，同时保持 Seedance canonical model 边界。
- 修改：`relay/channel/task/newapivideo/response_test.go`、`relay/relay_task_billing_test.go`、`e2e/paipu_upstream_e2e_test.go`，覆盖 Ark 生命周期、计费、退款、幂等和私有字段隔离。
- 修改：`web/scripts/channel-model-template/source.ts`、`types.ts`、`build.ts`、`generate.ts` 及 `__tests__/*`，读取并独立生成 H3 成本、映射、route blueprint 和销售行。
- 修改：`web/src/channel-config-converter/__tests__/v1.test.ts`（必要时补充 V1 adapter/文档类型），验证 H3 行进入 staging/review/publish/activate 输入且默认 disabled。
- 不修改：`constant.ChannelTypePaipu`、Paipu profile 的 `/v1/videos` 路径、Ark 客户端请求格式、`CanonicalModels` 的 H3 注册。

## 实施顺序与依赖

顺序固定为：H3 契约 → 请求校验与编码 → 路由合同 → 默认时长与计费 → Ark 生命周期测试 → H3 源表/价格导入 → 真实上游验收 → 全量验证。每个任务先写失败测试，再实现最小代码；任务完成后运行该任务的 focused tests，再提交该任务涉及的文件。共享注册文件由当前实现者单独维护，避免并行修改冲突。

### Task 1：建立 H3 模型契约

**文件：**

- 新建：`relay/channel/task/newapivideo/paipu_h3_contract.go`
- 新建：`relay/channel/task/newapivideo/paipu_h3_contract_test.go`

- [ ] **步骤 1：写模型契约失败测试。** 在测试中声明 `PaipuH3ContractForModel` 的期望结果：

```go
func TestPaipuH3ContractForModel(t *testing.T) {
	tests := []struct {
		model         string
		minDuration   int
		maxDuration   int
		defaultSecond int
		fixed         string
		minImages     int
		maxImages     int
		maxVideos     int
		maxAudios     int
		billing       paipuH3BillingMode
	}{
		{model: "lec-h3video-2k", minDuration: 15, maxDuration: 15, defaultSecond: 15, fixed: "1440p", maxImages: 9, billing: paipuH3BillingPerRequest},
		{model: "lec-minimax-h3", minDuration: 6, maxDuration: 15, fixed: "720p", minImages: 1, maxImages: 9, maxVideos: 3, maxAudios: 3, billing: paipuH3BillingPerDuration},
		{model: "lec-minimax-h3-768p", minDuration: 1, maxDuration: 15, defaultSecond: 5, fixed: "768p", maxImages: 9, maxAudios: 3, billing: paipuH3BillingPerDuration},
	}
	for _, test := range tests {
		t.Run(test.model, func(t *testing.T) {
			got, ok := PaipuH3ContractForModel(test.model)
			require.True(t, ok)
			assert.Equal(t, test.model, got.Model)
			assert.Equal(t, test.minDuration, got.MinDuration)
			assert.Equal(t, test.maxDuration, got.MaxDuration)
			assert.Equal(t, test.defaultSecond, got.DefaultDuration)
			assert.Equal(t, test.fixed, got.FixedResolution)
			assert.Equal(t, test.minImages, got.MinImages)
			assert.Equal(t, test.maxImages, got.MaxImages)
			assert.Equal(t, test.maxVideos, got.MaxVideos)
			assert.Equal(t, test.maxAudios, got.MaxAudios)
			assert.Equal(t, test.billing, got.BillingMode)
		})
	}
	_, ok := PaipuH3ContractForModel("imported-paipu-seedance")
	assert.False(t, ok)
}
```

- [ ] **步骤 2：运行红灯测试。** 执行 `go test ./relay/channel/task/newapivideo -run TestPaipuH3ContractForModel -count=1`；预期因类型或查找函数不存在而编译失败。
- [ ] **步骤 3：实现契约类型和完整 ID 查找。** 在 `paipu_h3_contract.go` 定义导出 `PaipuH3Contract`（包含 `Model`、`MinDuration`、`MaxDuration`、`DefaultDuration`、`DurationRequired`、`FixedResolution`、`AllowedRatios`、`MinImages`、`MaxImages`、`MaxVideos`、`MaxAudios`、`MaxTotal`、`AudioRequiresVisual`、`WritesDuration`、`WritesResolution`、URL policy、wire mode 和 billing mode 字段）以及 `PaipuH3ContractForModel(model string) (PaipuH3Contract, bool)`。用 `strings.EqualFold(strings.TrimSpace(model), contract.Model)` 只匹配三个完整 ID。字段固定为：2K `15/15/1440p/9/0/0/按次`；720p `6-15/必填/1-9/3/3/音频需视觉/按秒`；768p `1-15/默认 5/768p/0-9/0/3/不支持视频/按秒`。允许画幅仅 `16:9`、`9:16`。未知 ID 返回 `(PaipuH3Contract{}, false)`，不得改变旧 Seedance 路径。
- [ ] **步骤 4：运行绿灯测试并提交。** 执行 `go test ./relay/channel/task/newapivideo -run TestPaipuH3Contract -count=1`；通过后提交 `paipu_h3_contract.go` 和 `paipu_h3_contract_test.go`，提交信息为 `feat: define Paipu MiniMax H3 contracts`。

### Task 2：实现 H3 请求校验与 Paipu JSON 编码

**文件：**

- 修改：`relay/channel/task/newapivideo/paipu_request.go`
- 修改：`relay/channel/task/newapivideo/native.go`
- 修改：`relay/channel/task/newapivideo/adaptor.go`
- 修改：`relay/channel/task/newapivideo/paipu_request_test.go`
- 修改：`relay/channel/task/newapivideo/native_test.go`（仅在共享 Ark 校验需要新夹具时）

- [ ] **步骤 1：先写 H3 请求矩阵失败测试。** 在 `paipu_request_test.go` 增加三模型表驱动用例：合法边界（2K 15 秒/9 图、720p 6 和 15 秒/1 图、768p 省略时长和显式 1/15 秒）；非法时长、分辨率和画幅；720p 无图、音频单独提交、4 视频或 4 音频；768p 视频、4 音频；2K 视频/音频或非 1440p。断言错误码分别为 `InvalidParameter.duration`、`InvalidParameter.resolution`、`InvalidParameter.ratio`、`InvalidParameter.content`。
- [ ] **步骤 2：补齐素材角色和 URL 安全测试。** 覆盖 1 图首帧、2 图首尾帧、3+ 图参考素材的确定性转换；首尾帧与 3+ 参考图混用必须拒绝；图片/视频/音频数组顺序必须保持。按模型落实 URL 策略：2K 图片仅公网 HTTPS 或匹配的 `data:image/*`，720p 的视频/音频允许文档声明的公网 HTTP(S)，768p 只接受文档声明的公网 HTTPS；所有模型都拒绝私网 IP、`asset://`、`file://`、本地路径和错误 MIME data URI。音频不能独立提交的测试必须在预扣前失败。
- [ ] **步骤 3：运行红灯测试。** 执行 `go test ./relay/channel/task/newapivideo -run 'TestPaipuH3|TestBuildPaipuRequest' -count=1`；预期 H3 模型仍按旧 Seedance 边界或错误地透传字段而失败。
- [ ] **步骤 4：实现契约分派和 H3 校验。** `validatePaipuRequest` 先调用 `validateARKSemantics`，再用 `PaipuH3ContractForModel(upstreamModel)` 分派；命中 H3 时检查时长、固定分辨率、`16:9/9:16`、每类素材上限、总数、最小图片数、音频视觉素材依赖和角色可无损转换条件。实现入口保持以下形状，未命中时继续现有 Seedance 系列逻辑：

```go
func validatePaipuRequest(request arkRequest, upstreamModel string) error {
	if err := validateARKSemantics(request, paipuProtocolProfile()); err != nil {
		return err
	}
	if contract, ok := PaipuH3ContractForModel(upstreamModel); ok {
		return validatePaipuH3Request(request, contract)
	}
	return validatePaipuSeedanceRequest(request, upstreamModel)
}
```

先把当前 `validatePaipuRequest` 中未知模型使用的 Seedance 分支提取为 `validatePaipuSeedanceRequest`，保证行为不变；H3 错误统一返回 `*arkRequestError`，不能延迟到 `BuildRequestBody` 或预扣之后。
- [ ] **步骤 5：实现 H3 请求体字段白名单。** `buildPaipuRequest` 命中 H3 时只写 `model`、`prompt`、必要时的 `duration`、模型允许的 `aspect_ratio`/`resolution` 以及 `images`、`videos`、`audios` 数组；不写 `seconds`、`size`、`quality`、`watermark`、`generate_audio`、`seed`、`callback_url` 等 Seedance 专属字段。实现时先构造零值安全的 `paipuRequest`，再按 wire mode 选择字段：

```go
result := paipuRequest{Model: upstreamModel, Prompt: arkPrompt(request.Content)}
	if contract, ok := PaipuH3ContractForModel(upstreamModel); ok {
	if contract.WritesDuration && request.Duration != nil {
		result.Duration = request.Duration
	}
	if contract.WritesResolution && request.Resolution != nil {
		result.Resolution = request.Resolution
	}
	result.AspectRatio = request.Ratio
} else {
	result.Duration, result.AspectRatio, result.Resolution = request.Duration, request.Ratio, request.Resolution
}
```

所有可选标量继续使用指针和 `omitempty`：缺省字段不出现，显式 `0`/`false` 不被吞掉。2K 固定按次模型按 wire mode 决定是否发送固定 `duration=15`；768p 的 `resolution` 省略和显式 `768p` 保留两种测试路径，直到真实验收确认。
- [ ] **步骤 6：运行请求包测试并提交。** 执行 `go test ./relay/channel/task/newapivideo -count=1`；确认未知模型的既有 Paipu Seedance 测试仍通过，提交信息为 `feat: validate and encode Paipu H3 requests`。

### Task 3：接入 H3 路由合同而不污染 canonical model

**文件：**

- 修改：`relay/video_route_contract.go`
- 修改：`relay/video_route_contract_test.go`
- 修改：`pkg/modelrouting/validate.go`
- 修改：`pkg/modelrouting/validate_test.go`

- [ ] **步骤 1：增加路由合同失败测试。** Paipu target 的 `UpstreamModel` 分别设置为 `lec-h3video-2k`、`lec-minimax-h3`、`lec-minimax-h3-768p`，测试固定分辨率、时长边界、素材上限和最小素材数；加入 `1440p`、`768p` 可被通用 schema 接收、但不支持的 Paipu 分辨率由渠道合同拒绝的用例。断言 `CanonicalModels` 不包含任何 `lec-*`。
- [ ] **步骤 2：运行红灯测试。** 执行 `go test ./relay ./pkg/modelrouting -run 'Test.*Paipu.*H3|Test.*H3.*Route|Test.*Canonical' -count=1`；预期 `768p/1440p` 被通用分辨率白名单或旧 Paipu 合同拒绝。
- [ ] **步骤 3：实现 H3 优先分派。** 将现有 Seedance 分支提取为同文件的 `validatePaipuSeedanceRoute(canonicalModel string, target modelrouting.Target) error`，然后让 `validatePaipuVideoRoute` 首先按 `target.UpstreamModel` 调用 `tasknewapivideo.PaipuH3ContractForModel`；命中时要求固定 `1440p/720p/768p`、契约时长范围、图片/视频/音频上限、最小图片数、总数和 `16:9/9:16`。实现分支保持以下顺序，未知模型才进入旧 Seedance 合同：

```go
	if contract, ok := tasknewapivideo.PaipuH3ContractForModel(target.UpstreamModel); ok {
	if !allRouteResolutions(target.Constraints.OutputResolutions, contract.FixedResolution) ||
		!routeDurationWithin(target.Constraints.Durations, contract.MinDuration, contract.MaxDuration) {
		return newVideoRouteContractError("route_contract_h3", "Paipu H3 route exceeds the model contract")
	}
	// 检查 limits、minimums、总数、比例和固定视频/音频上限。
	return nil
}
return validatePaipuSeedanceRoute(canonicalModel, target)
```

不要把 H3 ID加入 `CanonicalModels`，不要修改 `/v1/video/generations` 的模型解析。
- [ ] **步骤 4：扩展 route constraint schema 的分辨率值并保持渠道隔离。** 在 `pkg/modelrouting/validate.go` 将 `768p`、`1440p` 加入仅供 `targets.constraints.output_resolutions` 使用的 `allowedConstraintResolutions`；保留 canonical policy 默认值的现有分辨率白名单，避免用户把 H3 私有档位误设为公共 Seedance 默认值。新增测试证明 Cangyuan、ZZone、Dimensio、Seedance 既有专属合同仍拒绝它们（除非该渠道已有明确能力），Paipu H3 才能通过固定值检查。H3 的比例白名单在 Paipu 分支独立执行，不放宽全局 `allowedRatios`。
- [ ] **步骤 5：运行路由测试并提交。** 执行 `go test ./relay ./pkg/modelrouting -count=1`；提交信息为 `feat: add Paipu H3 route contracts`。

### Task 4：在预扣前写入默认时长并接入计费模式

**文件：**

- 修改：`relay/channel/task/newapivideo/adaptor.go`
- 修改：`relay/channel/task/newapivideo/adaptor_test.go`
- 修改：`relay/relay_task_billing_test.go`

- [ ] **步骤 1：写默认时长和计费失败测试。** 构造真实 `gin.Context` 与 `requestState`，覆盖：2K 省略 duration 得到 15 秒且按次不乘时长；720p 省略 duration 被拒绝、6/15 秒按秒计费；768p 省略 duration 在 `ValidateBillingRequest` 后得到 5 秒、显式 1/15 秒按实际值计费。测试还要证明 `EstimateDurationSeconds` 读取的是已写入的 `state.Seconds`，而不是在 `BuildRequestBody` 时临时补值。
- [ ] **步骤 2：运行红灯测试。** 执行 `go test ./relay/channel/task/newapivideo ./relay -run 'Test.*H3.*Duration|Test.*TaskDurationQuota' -count=1`；预期默认时长尚未进入 request state，或 2K 按秒错误放大。
- [ ] **步骤 3：实现 `ValidateBillingRequest` 的 H3 归一化。** 在 Paipu provider validation 完成后取得 H3 契约：2K 将缺省时长归一化为 15 但选择 `per_request` 计费；720p 要求显式 6-15 秒并设置 `per_duration`；768p 缺省写入 5 秒、显式值保持原值。同步更新 `state.ARK.Duration`（仅当 wire mode 需要）与 `state.Seconds`，再 `c.Set(requestStateContextKey, state)`，确保预扣、`EstimateDurationSeconds`、构造请求体共享同一状态。状态更新采用以下顺序，避免只在请求体阶段补默认值。计费模式由 H3 配置导入的渠道成本规则决定（2K 生成 `per_request` 规则，720p/768p 生成 `per_duration` 规则），adaptor 不在请求运行时改写 `PriceData.BillingMode`：

```go
	contract, ok := PaipuH3ContractForModel(upstreamModel)
if ok {
	duration := state.ARK.Duration
	if duration == nil && contract.DefaultDuration > 0 {
		value := contract.DefaultDuration
		duration = &value
		if contract.WritesDuration {
			state.ARK.Duration = duration
		}
	}
	if duration == nil && contract.DurationRequired {
		return service.TaskErrorWrapperLocal(fmt.Errorf("duration is required"), "InvalidParameter.duration", http.StatusBadRequest)
	}
	if duration != nil {
		state.Seconds = decimal.NewFromInt(int64(*duration))
	}
	c.Set(requestStateContextKey, state)
}
```
- [ ] **步骤 4：复用现有 quota 饱和链路。** 使用既有 `taskDurationQuota`、`common.QuotaFrom*Checked` 和 `attachQuotaSaturation`；2K 的固定成本走 `per_request`，不新增裸浮点转 `int`。为 H3 的预扣、结算差额、失败退款和重复结算各增加断言，确认超大价格/时长无法产生负 quota。
- [ ] **步骤 5：运行计费测试并提交。** 执行 `go test ./relay/channel/task/newapivideo ./relay -run 'Test.*H3|TestTaskDurationQuota|Test.*Refund|Test.*Idempot' -count=1`；提交信息为 `fix: apply Paipu H3 duration before billing`。

### Task 5：补齐 Ark 任务生命周期与错误隔离测试

**文件：**

- 修改：`e2e/paipu_upstream_e2e_test.go`
- 修改：`relay/channel/task/newapivideo/response_test.go`
- 修改：`relay/relay_task_billing_test.go`

- [ ] **步骤 1：扩展 Mock 上游夹具。** 在 `paipuE2EMock` 增加按模型返回 queued/in_progress/completed/failed 的响应序列，记录 POST/GET 次数、请求体和 Authorization；完成态覆盖当前轮询响应中直接返回结果 URL、`content_url` 或嵌套 `data.content.video_url` 的字段投影。`/v1/videos/{task_id}/content` 只作为文档差异记录，不在没有真实响应证据时新增私有结果代理。Mock 只使用固定假 ID，不写入真实 key。
- [ ] **步骤 2：写三个 H3 模型的 Ark 生命周期测试。** 对每个模型执行 `POST /api/v3/contents/generations/tasks` → 查询列表/单任务 → 成功或失败；断言公共任务 ID 与上游私有 task ID 分离、结果 URL 映射、失败错误映射、任务日志和用量字段。输入矩阵至少包含文本、H3 合法图片、720p 的视频/音频组合、768p 音频以及 2K 9 图边界。
- [ ] **步骤 3：写重试与隐私回归测试。** 上游创建返回 429、5xx、连接超时或响应无 task ID 时，断言 POST 恰好一次、不会拿不确定结果重复创建；轮询 404/410 映射为现有任务不存在/过期错误。所有公开响应不得出现 API key、Paipu 上游 task ID、上游模型名、渠道 ID、quota、原始响应和内部计费字段。
- [ ] **步骤 4：写失败退款和幂等测试。** 失败任务只退款一次；重复轮询、重复结算和重复失败通知不重复退款；成功任务只结算一次。断言 `Task.Quota`、用户/Token/渠道余额、退款日志和 `quota_saturation` 审计标记。
- [ ] **步骤 5：运行 E2E/响应测试并提交。** 执行 `go test ./relay/channel/task/newapivideo ./relay ./e2e -run 'Paipu|H3|TaskResult|Refund|NoRetry' -count=1 -p=1`；提交信息为 `test: cover Paipu H3 Ark lifecycle`。

### Task 6：实现 H3 源表、官方价格和配置导入路径

**文件：**

- 修改：`web/scripts/channel-model-template/source.ts`
- 修改：`web/scripts/channel-model-template/types.ts`
- 修改：`web/scripts/channel-model-template/build.ts`
- 修改：`web/scripts/channel-model-template/generate.ts`
- 修改：`web/scripts/channel-model-template/__tests__/source.test.ts`
- 修改：`web/scripts/channel-model-template/__tests__/build.test.ts`
- 修改：`web/scripts/channel-model-template/__tests__/generate.test.ts`
- 修改：`web/src/channel-config-converter/__tests__/v1.test.ts`
- 必要时修改：`web/src/channel-config-converter/adapters/v1.ts`、`web/src/channel-config-converter/types.ts`

- [ ] **步骤 1：锁定 H3 源表字段测试。** `source.test.ts` 构造真实表头并断言保留 sheet/row：`h3` 至少包含 `渠道`、`模型ID`、`系列`、`版本`、`清晰度`、`计费方式`、`单价 元`；`h3官价` 使用系列、模型、版本、分辨率、素材计费项和输出计费项的实际列。缺表头、重复模型 ID、非正价格、非法计费方式和无行号来源必须返回可定位的 `FAIL`。
- [ ] **步骤 2：运行红灯测试。** 在 `web/` 执行 `bun test scripts/channel-model-template/__tests__/source.test.ts scripts/channel-model-template/__tests__/build.test.ts`；预期当前 `UNSUPPORTED_SOURCE_SHEET` 阻断 H3 生成。
- [ ] **步骤 3：扩展 `SourceWorkbook` 和读取器。** 增加 `h3Models`、`h3OfficialPrices` 结构（字段值和 `SourceLocation` 均保留），读取可选 sheet 时按各自表头解析；只有 `h3` 与 `h3官价` 同时存在才进入 H3 分支，缺一时生成明确 `FAIL`。现有 SD-only 工作簿继续按原规则工作。
- [ ] **步骤 4：实现独立 H3 归一化。** 在 `build.ts` 增加 `buildH3TemplateData` 或等价分支：`h3` 生成 Paipu 渠道成本、模型映射和 route blueprint；`h3官价` 生成 H3 `per_duration` 销售行。实现分支以 `billingMode` 选择成本模式，不调用 Seedance Token/M 公式；`pricePerRequest`、`pricePerSecond` 和 `contract` 必须来自同一模型 ID 的已校验源行：

```ts
const modelId = field(h3Model, '模型ID')
const billingMode = field(h3Model, '计费方式')
const mode = billingMode === 'call' ? 'per_request' : 'per_duration'
const sale = {
  billingMode: 'per_duration',
  nativePerSecond: modelId === 'lec-h3video-2k' ? pricePerRequest.div(15).toFixed() : pricePerSecond.toFixed(),
  minDurationSeconds: modelId === 'lec-h3video-2k' ? 15 : contract.minDuration,
  maxDurationSeconds: modelId === 'lec-h3video-2k' ? 15 : contract.maxDuration,
  status: 'disabled',
}
```

`h3官价` 的素材价格和输出价格分开落到销售/成本行；素材限制、最小时长、默认时长、画幅和固定分辨率必须进入 route blueprint。所有输出行写入 `source_ref`、原始 sheet、行号、价格单位和 disabled 状态。
- [ ] **步骤 5：调整生成器开关。** `generate.ts` 只允许 `--allow-unsupported-sheets` 降级真正未知的可选 sheet；已实现的 H3 sheet 不再产生 `UNSUPPORTED_SOURCE_SHEET`。生成的 H3 行默认 `status=disabled`，没有真实上游验收时禁止自动改为 active。
- [ ] **步骤 6：验证 V1 导入合同。** 在 `v1.test.ts` 增加 H3 渠道/模型 SKU/销售/成本/映射/route blueprint 计数、业务 ID、来源位置、价格单位和 disabled 断言；执行 staging → review → publish → activate 的转换测试，确认 Seedance 模板行和既有十张 V1 sheet 不回归。通过后提交信息为 `feat: import Paipu MiniMax H3 pricing`。

### Task 7：执行真实 Paipu H3 契约验收

**文件/产物：**

- 修改：`e2e/paipu_upstream_e2e_test.go`（仅在真实差异需要固定契约时）
- 新建但不提交密钥：`outputs/2026-08-24-paipu-h3-acceptance/验收报告.md` 及脱敏请求/响应摘要

- [ ] **步骤 1：准备环境并记录版本。** 设置 `PAIPU_API_KEY`，可选设置 `PAIPU_BASE_URL`；运行验收前记录 base URL、提交代码版本、模型映射快照 SHA-256。禁止把 key、完整 Authorization、签名 URL 或原始响应写入 Git、日志或报告。
- [ ] **步骤 2：逐模型提交合法矩阵。** 对 `lec-h3video-2k`、`lec-minimax-h3`、`lec-minimax-h3-768p` 分别提交文本、合法图片和声明支持的音视频组合；验证 `queued → in_progress → completed/failed`、结果 URL、轮询间隔和上游错误码。2K 验证 15 秒固定按次；720p 验证 6 秒和 15 秒；768p 验证省略 duration（网关默认 5 秒）和显式 768p/15 秒。
- [ ] **步骤 3：逐模型提交非法矩阵。** 验证越界时长、错误分辨率、错误画幅、超出图片/视频/音频上限、音频单独提交、私网 URL 和错误 MIME data URI 均在预扣和上游 POST 前返回 400，Mock/真实上游均只看到零次提交。
- [ ] **步骤 4：解决 768p resolution wire mode。** 对 `lec-minimax-h3-768p` 分别发送省略 `resolution` 和显式 `"resolution":"768p"` 两个请求，记录上游接受/拒绝及实际输出分辨率；只有两种行为之一被文档和实测共同确认后，才固定 `ResolutionWireMode` 并生成 active 候选。若无法确认，保持 disabled 并在报告中列出阻塞原因。
- [ ] **步骤 5：验证创建错误不重试。** 让真实/代理上游返回 429、5xx、超时和无 task ID 响应，断言只提交一次；结果不确定时只查询原任务 ID。验收完成后撤销环境变量或关闭终端，报告只保留脱敏摘要。

### Task 8：最终验证、审计和回滚门禁

- [ ] **步骤 1：运行 Go focused tests。**

```powershell
go test ./relay/channel/task/newapivideo -run 'TestPaipuH3|TestPaipu' -count=1
go test ./relay ./pkg/modelrouting ./service -run 'Paipu|H3|VideoRoute|ConfigImport' -count=1
go test ./e2e -run 'Paipu|H3' -count=1
```

预期三个命令退出码均为 0；失败时在验收报告记录首个失败命令、错误、影响和下一步，不标记完成。
- [ ] **步骤 2：运行前端测试、类型检查和构建。**

```powershell
Set-Location web
bun test scripts/channel-model-template src/channel-config-converter
bun run typecheck
bun run build
Set-Location ..
```

预期无 TypeScript 类型错误、V1 转换器测试通过、构建产物生成成功。
- [ ] **步骤 3：运行工作区卫生检查。** 执行 `git diff --check` 和 `git status --short`；确认无密钥、无临时输出进入提交、无 H3 ID误加 `CanonicalModels`、无 Seedance 公共端点协议变化。当前用户已有模板生成器修改继续保留。
- [ ] **步骤 4：完成审计报告。** 报告用简体中文记录：协议来源、能力矩阵、共享/专属代码边界、每条测试命令与退出码、真实验收是否执行、H3 源表和价格单位、模板/导入批次状态、默认 disabled 原因、未覆盖风险和回滚点。回滚只允许按任务提交逐个 `git revert <commit>`，不得回退用户已有未提交文件。
- [ ] **步骤 5：完成标准。** 只有同时满足以下条件才可把 H3 标记为可发布：三个模型契约单测和 Ark 生命周期测试通过；路由、预扣、结算、退款和幂等通过；H3 源表导入可追溯且默认 disabled；有 `PAIPU_API_KEY` 时真实创建/轮询/错误矩阵通过；无 key 时明确标记真实验收 blocked，不能宣称生产可用。

## 计划自审

- **规格覆盖：** H3 三模型能力、素材角色和 URL 安全、`resolution` 文档冲突、Ark 生命周期、计费默认值、路由 schema、价格导入、disabled 门禁和真实验收均有对应任务。
- **边界一致性：** H3 查找只按完整上游模型 ID；canonical model 仍是公共 Seedance 身份；未知模型继续走既有 Paipu Seedance 协议。
- **占位符检查：** 计划没有 `TODO`、`TBD` 或“稍后补充”步骤；所有阻塞项都绑定了具体命令、环境变量和报告结论。
- **安全检查：** 计划明确禁止密钥、私有 task ID、签名 URL 和原始上游响应进入日志、测试快照或提交；计费复用 quota 饱和 helper。
