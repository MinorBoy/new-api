# Seedance 公共模型边界修正实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 仅对 Seedance 家族隐藏内部模型并拒绝直接调用，同时完整保留其他已接入模型的公开和调用能力。

**Architecture:** 在 `pkg/modelrouting` 提供 Seedance 家族判断与公共过滤，由路由策略缓存同步 canonical Seedance 对应的 upstream model 集合；控制器复用该过滤并仅覆盖三个官方 Seedance 的 Doubao 身份；Relay 只拒绝隐藏的 Seedance ID。内部渠道、能力、映射、成本和路由数据保持不变。

**Tech Stack:** Go 1.22、Gin、GORM、Testify、React 19、TypeScript、Bun/Vitest。

**设计依据:** `docs/superpowers/specs/2026-08-01-public-model-boundary-design.md`

---

### Task 1: 固化 Seedance 专用过滤契约

**Files:**
- Modify: `pkg/modelrouting/public_test.go`
- Modify: `pkg/modelrouting/public.go`
- Modify: `model/routing_policy_cache_test.go`
- Modify: `model/routing_policy_cache.go`

- [ ] 修改测试，断言 GPT、Claude、Gemini、DeepSeek、GLM 等非 Seedance ID 公开，三个官方 Seedance ID 公开，内部 Seedance、旧 Mini ID，以及 `4sdance431`、`videos-fast`、`video-2.0-pro` 等非字面 upstream 别名隐藏。
- [ ] 运行 `go test ./pkg/modelrouting -run 'Test(IsPublicSeedanceModel|IsHiddenSeedanceModel|FilterPublicModels)' -count=1`，确认测试因当前全局白名单语义而失败。
- [ ] 实现 `IsPublicSeedanceModel`、`SetHiddenSeedanceModels`、`IsHiddenSeedanceModel`，让路由策略缓存同步 upstream model 集合，并让 `FilterPublicModels` 去重且保持输入顺序。
- [ ] 重跑上述测试，确认通过。

### Task 2: 修正模型广场投影

**Files:**
- Modify: `controller/public_models_test.go`
- Modify: `controller/public_models.go`
- Verify: `controller/pricing.go`

- [ ] 修改定价测试，输入普通模型、三个官方 Seedance、内部 Seedance及多个供应商。
- [ ] 断言普通模型保留原供应商/owner/图标，官方 Seedance 显示为 Doubao，内部 Seedance 被过滤，端点和供应商只覆盖保留项。
- [ ] 运行 `go test ./controller -run TestProjectPublicPricing -count=1`，确认失败。
- [ ] 按输入顺序投影所有保留定价；依据原响应供应商列表保留普通供应商，并按需增加 Doubao。
- [ ] 重跑定价投影测试，确认通过。

### Task 3: 修正模型列表和单模型查询

**Files:**
- Modify: `controller/public_models_test.go`
- Modify: `controller/model.go`
- Verify: `controller/user.go`

- [ ] 修改 OpenAI、Anthropic、Gemini、用户模型、Dashboard 和单模型查询测试，使普通模型保留、内部 Seedance 隐藏。
- [ ] 运行相关控制器测试，确认当前全局白名单造成失败。
- [ ] 在过滤后先获取普通模型原 owner，只把三个官方 Seedance 的 owner 覆盖为 `doubao`。
- [ ] 单模型查询只拒绝隐藏 Seedance；普通模型继续使用现有元数据和 owner。
- [ ] 重跑相关控制器测试，确认通过。

### Task 4: 修正 Relay 请求边界

**Files:**
- Modify: `middleware/distributor_routing_test.go`
- Modify: `middleware/distributor.go`

- [ ] 新增/修改测试：内部 Seedance 在渠道选择前返回 `model_not_found`；普通模型不得被该边界提前拒绝；三个官方 Seedance 继续路由。
- [ ] 运行 `go test ./middleware -run TestDistribute -count=1`，确认普通模型用例因全局白名单失败。
- [ ] 将全局 `!IsPublicModel` 条件改为 `IsHiddenSeedanceModel`。
- [ ] 重跑相关 Middleware 测试，确认通过。

### Task 5: 同步文档契约

**Files:**
- Modify: `web/src/features/docs/__tests__/resolve-doc.test.ts`
- Modify: `web/src/features/docs/content/zh/pricing.md`
- Modify: `web/src/features/docs/content/en/pricing.md`

- [ ] 测试要求中英文定价文档列出三个官方 Seedance ID，且不出现旧 Mini ID 或内部 Seedance 示例。
- [ ] 删除“不得出现 Anthropic、聊天端点等非 Seedance 能力”的错误断言。
- [ ] 在定价文档明确其他模型正常公开，Seedance 只公开三个官方 ID，内部 Seedance 路由 ID 不公开。
- [ ] 运行文档测试并确认通过。

### Task 6: 全量验证与本地验收

- [ ] 运行 `gofmt` 格式化涉及的 Go 文件。
- [ ] 运行 `go test ./... -count=1`。
- [ ] 在 `web/` 运行文档测试、`bun run typecheck`、涉及文件 Lint 和 `bun run build:check`。
- [ ] 运行 `git diff --check`。
- [ ] 重建本地服务容器或确认开发服务已加载新代码。
- [ ] 在模型广场确认非 Seedance 家族仍存在，Seedance 只出现三个官方 ID且不出现内部 ID。
- [ ] 在桌面和移动端验收文档页面，并检查控制台错误。
- [ ] 审查暂存范围，只在本地 `ysr` 提交相关改动，禁止触碰 `main`。
