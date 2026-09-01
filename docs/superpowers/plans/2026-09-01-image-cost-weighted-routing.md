# 图像成本容差混流实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为统一 OpenAI Images 路由增加 `cost_weighted` 策略，使成本接近最低成本的渠道按成本倒数和现有 Weight 混流，同时保持 `manual` 与 `lowest_cost` 的兼容语义。

**Architecture:** 在现有 `image_setting.Policy` 增加策略枚举和成本容差字段；在 `service/image_routing.go` 中先完成严格成本/兼容性筛选，再构造成本容差池，并用有界整数有效权重调用现有随机选择器。管理端继续使用 JSON 配置编辑器，仅扩展后端校验、说明文本、预览返回的策略字段和多语言文案，不引入新数据库表。

**Tech Stack:** Go 1.22、Gin、GORM 现有成本规则、Testify、React 19、TypeScript、i18next、Bun。

---

### Task 1: 路由策略配置契约

**Files:**
- Modify: `setting/image_setting/routing.go`
- Test: `setting/image_setting/routing_test.go`
- Modify: `docs/superpowers/specs/2026-09-01-openai-images-channel-design.md`

- [ ] **Step 1: 写失败测试**

在 `setting/image_setting/routing_test.go` 增加以下行为断言：`cost_weighted` 能解析，`cost_tolerance_bps` 能保留；负数、大于 10000 以及非 `cost_weighted` 策略携带该字段时拒绝，并且失败不会替换当前快照。

```go
func TestRoutingAcceptsCostWeightedTolerance(t *testing.T) {
	t.Cleanup(func() { require.NoError(t, UpdateRoutingByJSONString(`{"version":1,"default":{"strategy":"manual"}}`)) })
	require.NoError(t, UpdateRoutingByJSONString(`{"version":1,"default":{"strategy":"cost_weighted","cost_tolerance_bps":1250}}`))
	policy := PolicyFor("", "")
	assert.Equal(t, StrategyCostWeighted, policy.Strategy)
	require.NotNil(t, policy.CostToleranceBPS)
	assert.Equal(t, 1250, *policy.CostToleranceBPS)
}

func TestRoutingRejectsInvalidCostWeightedTolerance(t *testing.T) {
	t.Cleanup(func() { require.NoError(t, UpdateRoutingByJSONString(`{"version":1,"default":{"strategy":"manual"}}`)) })
	require.NoError(t, UpdateRoutingByJSONString(`{"version":1,"default":{"strategy":"lowest_cost"}}`))
	for _, raw := range []string{
		`{"version":1,"default":{"strategy":"cost_weighted","cost_tolerance_bps":-1}}`,
		`{"version":1,"default":{"strategy":"cost_weighted","cost_tolerance_bps":10001}}`,
		`{"version":1,"default":{"strategy":"manual","cost_tolerance_bps":100}}`,
	} {
		require.Error(t, UpdateRoutingByJSONString(raw))
		assert.Equal(t, StrategyLowestCost, PolicyFor("", "").Strategy)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

运行 `go test ./setting/image_setting -run 'TestRoutingAcceptsCostWeightedTolerance|TestRoutingRejectsInvalidCostWeightedTolerance' -count=1`。预期失败原因是 `StrategyCostWeighted` 和 `CostToleranceBPS` 尚未定义，且校验仍只接受两个策略。

- [ ] **Step 3: 实现最小配置支持**

在 `routing.go` 增加 `StrategyCostWeighted`，在 `Policy` 增加 `*int CostToleranceBPS`；校验策略允许三个值、容差范围为 `0..10000`，且只有 `cost_weighted` 可以填写该字段。克隆策略时复制指针。未填写容差时保持 `nil`，运行时由服务层使用 1000 默认值。

- [ ] **Step 4: 运行测试确认通过**

运行同一条 `go test` 命令，预期两个测试通过；再运行 `go test ./setting/image_setting -count=1` 确认既有默认值和快照回滚测试通过。

- [ ] **Step 5: 提交配置契约**

```powershell
git add setting/image_setting/routing.go setting/image_setting/routing_test.go docs/superpowers/specs/2026-09-01-openai-images-channel-design.md
git commit -m "feat: add cost weighted image routing policy"
```

### Task 2: 成本容差池和有效权重选择器

**Files:**
- Modify: `service/image_routing.go`
- Test: `service/image_routing_test.go`

- [ ] **Step 1: 写失败测试**

在 `service/image_routing_test.go` 增加纯函数级测试，覆盖四个契约：默认容差为 1000 bps；高于容差的候选不入池；成本越低的候选有效权重越高；同成本仍严格使用 `Weight + 10`。测试通过注入 `imageRouteWeightSelector` 检查传入候选和权重。

```go
func TestCostWeightedImageRoutePoolUsesDefaultToleranceAndInverseCost(t *testing.T) {
	candidates := []ImageRouteCandidate{
		{ChannelID: 1, Weight: 0, CostKnown: true, EstimatedCostNanoUSD: 100},
		{ChannelID: 2, Weight: 0, CostKnown: true, EstimatedCostNanoUSD: 105},
		{ChannelID: 3, Weight: 0, CostKnown: true, EstimatedCostNanoUSD: 120},
	}
	pool := BuildCostWeightedImageRoutePool(candidates, nil)
	require.Equal(t, []int{1, 2}, imageRouteCandidateIDs(pool))
	assert.Greater(t, pool[0].EffectiveWeight, pool[1].EffectiveWeight)
}

func TestCostWeightedImageRoutePoolKeepsEqualCostWeightSemantics(t *testing.T) {
	candidates := []ImageRouteCandidate{
		{ChannelID: 1, Weight: 1, CostKnown: true, EstimatedCostNanoUSD: 100},
		{ChannelID: 2, Weight: 9, CostKnown: true, EstimatedCostNanoUSD: 100},
	}
	pool := BuildCostWeightedImageRoutePool(candidates, intPtr(0))
	require.Len(t, pool, 2)
	assert.Equal(t, 11, pool[0].EffectiveWeight)
	assert.Equal(t, 19, pool[1].EffectiveWeight)
}
```

测试辅助 `intPtr` 只在该测试文件中定义，并复用项目已有的 `require`/`assert` 风格。

- [ ] **Step 2: 运行测试确认失败**

运行 `go test ./service -run 'TestCostWeightedImageRoutePool' -count=1`。预期失败原因是 `BuildCostWeightedImageRoutePool` 和 `EffectiveWeight` 尚未定义。

- [ ] **Step 3: 实现最小选择器**

为 `ImageRouteCandidate` 增加 `EffectiveWeight int `json:"-"`` 字段（仅供选择器使用，不改变管理员预览 JSON），并新增 `BuildCostWeightedImageRoutePool(candidates []ImageRouteCandidate, toleranceBPS *int) []ImageRouteCandidate`：

1. 只处理 `CostKnown` 且成本大于等于零的候选；空输入返回空切片。
2. 找到最低成本；容差 `nil` 使用 1000，显式值限制在已校验的 `0..10000`。
3. 用 `shopspring/decimal` 计算 `minimum * (10000+tolerance) / 10000`，向下取整为 int64，避免乘法溢出；成本不超过上限的候选入池。
4. 有效权重按 `max(Weight+10, 1) * ceil(minimum * 10000 / cost) / 10000` 计算；使用 10000 倍基数保留成本差异。最低成本为零时，零成本候选使用基础权重，正成本候选使用 1；每个结果权重限制为 `int` 最大值，至少为 1。
5. 池内按成本升序、ChannelID 升序稳定排列，避免预览顺序抖动。

新增 `selectCostWeightedImageRouteCandidate`，复制池中的 `EffectiveWeight` 到临时权重后调用现有 `selectWeightedImageRouteCandidate`，不修改 `manual` 选择器的 `Weight` 字段。

- [ ] **Step 4: 运行测试确认通过**

运行 `go test ./service -run 'TestCostWeightedImageRoutePool' -count=1`，预期通过。随后运行 `go test ./service -run 'TestSelect(ImageRouteCandidate|ManualImageRouteCandidate)' -count=1`，确认既有 lowest-cost/manual 语义不变。

- [ ] **Step 5: 提交选择器**

```powershell
git add service/image_routing.go service/image_routing_test.go
git commit -m "feat: add cost tolerant image route selector"
```

### Task 3: 接入完整图像决策和重试

**Files:**
- Modify: `service/image_routing.go`
- Test: `service/image_model_routing_test.go`
- Test: `service/image_routing_test.go`
- Modify: `docs/openai-images-channel.md`

- [ ] **Step 1: 写失败测试**

扩展 `service/image_routing_test.go`，增加 `BuildCostWeightedImageRoutePool` 在候选被移除后重新以剩余候选计算最低成本的测试；在现有模型路由集成 fixture 中增加一个 `cost_weighted` 策略场景，断言低价和容差内高价候选都可进入决策，容差外候选不被选择。

```go
func TestCostWeightedImageRoutePoolRecomputesMinimumAfterRetry(t *testing.T) {
	remaining := []ImageRouteCandidate{
		{ChannelID: 2, CostKnown: true, EstimatedCostNanoUSD: 200},
		{ChannelID: 3, CostKnown: true, EstimatedCostNanoUSD: 220},
	}
	pool := BuildCostWeightedImageRoutePool(remaining, intPtr(1000))
	require.Equal(t, []int{2, 3}, imageRouteCandidateIDs(pool))
	assert.Equal(t, int64(200), pool[0].EstimatedCostNanoUSD)
}
```

- [ ] **Step 2: 运行测试确认失败**

运行 `go test ./service -run 'TestCostWeightedImageRoutePoolRecomputesMinimumAfterRetry|TestUnifiedImageRoutingCostWeighted' -count=1`，预期集成测试失败，因为 `BuildImageRouteDecision` 尚未处理新策略。

- [ ] **Step 3: 接入决策分支**

在 `BuildImageRouteDecision` 成本筛选完成后增加 `StrategyCostWeighted` 分支：严格成本模式沿用现有成本缺失/成本未知/毛利失败排除；非严格模式也只把 `CostKnown` 候选交给成本混流池，未知成本保留在管理员候选清单但不入池。调用 `BuildCostWeightedImageRoutePool(priced, policy.CostToleranceBPS)`，池为空返回 `ErrNoEligibleImageChannel`，否则选择池内有效权重最高概率候选并写入 `decision.Selected`。策略未知时的错误分支保留。

重试继续通过 `param.ExcludedChannelIDs` 重新调用 `BuildImageRouteDecision`，因此每次都会重新计算最低成本和容差池；不改变已接受/状态未知请求的保护逻辑。

- [ ] **Step 4: 运行测试确认通过**

运行 `go test ./service -run 'TestCostWeightedImageRoutePool|TestUnifiedImageRouting' -count=1`，再运行 `go test ./service -count=1`。

- [ ] **Step 5: 更新中文文档**

在 `docs/openai-images-channel.md` 补充 `cost_weighted` 的 JSON 示例、10% 默认容差、未知成本排除、成本倒数加权和“精确 80/20 仍用 manual 同 Priority + Weight”的说明；明确 native Gemini `generateContent` 仍不属于 OpenAI Images-compatible。

- [ ] **Step 6: 提交决策接入**

```powershell
git add service/image_routing.go service/image_routing_test.go service/image_model_routing_test.go docs/openai-images-channel.md
git commit -m "feat: route image traffic with cost tolerance"
```

### Task 4: 管理端校验、说明和预览

**Files:**
- Modify: `web/src/features/system-settings/models/image-settings-card.tsx`
- Modify: `web/src/i18n/static-keys.ts`
- Modify: `web/src/i18n/locales/en.json`
- Modify: `web/src/i18n/locales/zh.json`
- Modify: `web/src/i18n/locales/zh-TW.json`
- Modify: `web/src/i18n/locales/fr.json`
- Modify: `web/src/i18n/locales/ja.json`
- Modify: `web/src/i18n/locales/ru.json`
- Modify: `web/src/i18n/locales/vi.json`
- Test: `web/src/features/system-settings/models/__tests__/image-preview-options.test.ts`

- [ ] **Step 1: 写失败测试**

扩展前端策略 JSON 校验测试，确保 `cost_weighted` 被视为支持策略，且文案不再只列 `manual or lowest_cost`。如果现有测试没有直接暴露该描述，新增 `image-routing-policy` 的纯校验函数测试，拒绝策略以外的值、容差超界和非整数值。

- [ ] **Step 2: 运行测试确认失败**

运行 `cd web; bun test src/features/system-settings/models/__tests__/image-preview-options.test.ts`，预期新断言失败。

- [ ] **Step 3: 实现最小前端更新**

将路由说明改为包含 `manual`, `lowest_cost`, `cost_weighted` 和成本容差；如现有表单仅做 JSON object 级校验，新增独立 `image-routing-policy.ts` 的 Zod schema 对 default/groups 中的策略和 `cost_tolerance_bps` 做同样的 `0..10000` 校验，并在保存前调用。更新七个 locale 的英文 key 对应翻译，运行 i18n 同步工具检查静态 key。

- [ ] **Step 4: 运行测试确认通过**

运行 `cd web; bun test src/features/system-settings/models/__tests__/image-preview-options.test.ts; bun run typecheck; bun run build`，预期全部通过。

- [ ] **Step 5: 提交管理端更新**

```powershell
git add web/src/features/system-settings/models web/src/i18n
git commit -m "feat: expose cost weighted image routing in admin"
```

### Task 5: 全量验证和收尾

**Files:**
- No new production files.

- [ ] **Step 1: 运行后端定向测试**

运行 `go test ./controller ./middleware ./model ./relay/helper ./service ./types ./relay/channel/openai ./relay -count=1` 和 `go test ./relaykit/... -count=1`。

- [ ] **Step 2: 运行前端检查**

在 `web` 目录运行 `bun run typecheck`、`bun run build` 和受影响的 Bun 测试。

- [ ] **Step 3: 检查差异和文档**

运行 `git diff --check`，确认新增字段使用 `common.Marshal`/`common.Unmarshal`，没有凭据输出、数据库方言 SQL 或负计费路径变化；检查文档仍为简体中文。

- [ ] **Step 4: 记录根模块基线**

运行 `go test ./... -count=1`。若仍仅失败于既有 Seedance 模拟状态 `unknown new-api video task status: waiting_for_magic`，在最终报告中明确这是既有 E2E 基线，不归因于图像路由。

- [ ] **Step 5: 复核工作树**

运行 `git status --short --branch`，保留用户已有的未提交修改，不执行 reset、checkout 或清理命令。
