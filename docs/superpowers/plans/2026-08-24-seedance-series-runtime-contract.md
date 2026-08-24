# Seedance 系列运行时合同修复实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让 Seedance 2.0 与 2.5 在静态路由合同、Ark 请求校验和 Dimensio/Paipu/ZZone 适配器中使用同一组系列能力边界，并完成本地批次激活与 Mock 验收。

**Architecture:** 在 `pkg/modelrouting` 集中定义 canonical model 到系列能力的映射；服务层把 canonical model 传入路由合同；运行时请求校验使用请求模型，供应商模型名只用于上游转发。渠道专属校验复用共享上限并保留自身分辨率、角色和字段规则。

**Tech Stack:** Go 1.22、Gin/GORM、Testify、Docker Compose、Bun/ExcelJS、new-api 配置导入向导、Mock Ark SDK E2E。

---

## 文件边界

- `pkg/modelrouting/seedance_series.go`、`seedance_series_test.go`：共享 2.0/2.5 素材与时长合同。
- `service/route_contract.go`、`routing_policy.go`、`config_import_activation.go` 及对应测试：传递 canonical model。
- `relay/video_route_contract.go` 及测试：静态渠道合同按系列校验。
- `relay/channel/task/newapivideo/native.go`、`paipu_request.go`、`zzone_request.go` 及测试：运行时请求边界。
- `relay/channel/task/dimensio/translate.go`、`adaptor.go` 及测试：分离 canonical model 与上游模型。
- `cmd/ark-video-material-seed`、`e2e/seedance_material_matrix_e2e_test.go`：矩阵工具加载 2.5。
- `web/scripts/channel-model-template/conversion-rules.json`：移除 15 条系列误判 draft，保留 OmegaAI 3 条。
- `outputs/2026-08-23-sd-series-contract/`：保留源表、模板、导入 JSON、日志和验收报告，不纳入提交。

## Task 1：共享系列合同（TDD）

- [ ] 在 `pkg/modelrouting/seedance_series_test.go` 先添加 2.0、Fast、Mini、2.5 和未知别名的表驱动失败测试。
- [ ] 运行 `go test ./pkg/modelrouting -run TestSeedanceSeriesContractForModel -count=1`，确认因 API 未定义而失败。
- [ ] 新增 `SeedanceSeriesContract` 与 `SeedanceSeriesContractForModel`，未知名称使用保守 2.0 边界。
- [ ] 让 `pkg/modelrouting/validate.go` 复用合同的素材、总数、时长和 2.5 分辨率边界。
- [ ] 运行 `go test ./pkg/modelrouting -count=1`，确认通过并执行 `gofmt`。

## Task 2：静态路由合同传递 canonical model（TDD）

- [ ] 修改服务路由合同测试，先断言回调收到 `request.Model`/`policy.Model`，运行定向测试确认旧签名编译失败。
- [ ] 将 `RouteTargetContractValidator` 和 `ValidateVideoRouteTargetContract` 增加 canonical model 参数，更新全部调用点。
- [ ] 在 `relay/video_route_contract.go` 让 Dimensio、Paipu、ZZone 按共享系列校验素材上限、总数和最大时长，保留供应商专属规则。
- [ ] 添加 2.0 越界拒绝与 2.5 `30/10/10`、50、30 秒边界通过测试；运行 `go test ./service ./relay -count=1 -p=1`。

## Task 3：运行时 Ark 与渠道适配器（TDD）

- [ ] 在公共 Ark、Paipu、ZZone、4SToken 测试中先添加 2.5 exact boundary 和 2.0 回归失败测试，确认旧常量造成红灯。
- [ ] 让 `validateARKSemantics`、Paipu 和 ZZone 通过 `request.Model` 读取共享合同；保持角色、URL、比例和字段限制不变。
- [ ] 运行 `go test ./relay/channel/task/newapivideo -count=1`，确认所有适配器测试通过。

## Task 4：Dimensio 模型职责分离（TDD）

- [ ] 添加 2.5 30 秒及 `30/10/10` 翻译/适配器失败测试，同时保留 2.0 `jmg-*` 总数 12 特例。
- [ ] 修改翻译器显式接收 canonical model 和 upstream model；校验前不覆盖 `ArkRequest.Model`，生成上游 JSON 时才使用映射模型。
- [ ] 按系列校验时长，运行 `go test ./relay/channel/task/dimensio -count=1`。

## Task 5：矩阵工具、规则和本地配置刷新

- [ ] 为 `seedance-2.5` 添加矩阵运行时模型测试并实现映射；运行 `go test ./cmd/ark-video-material-seed ./e2e -run 'TestRuntimeModel|TestImportedMaterialMatrix' -count=1 -p=1`。
- [ ] 用结构化脚本从 `conversion-rules.json` 删除 Dimensio 8、Paipu 3、ZZone 4 条系列 draft，核对 OmegaAI 3 条仍存在。
- [ ] 使用已验证的 `outputs/2026-08-23-sd-series-contract/sd收录.xlsx` 重新生成模板，确认报告 `FAIL=0`、公式错误 0、系列计数和哈希不变。
- [ ] 通过 `/config-import` 复制批次 #45，将 FFLINK `MAP-FFLINK-R253-480` 路由差异设为 `replace`，重新暂存、审阅、校验并发布。

## Task 6：激活、Mock E2E 和验收报告

- [ ] 重建 `new-api-local-new-api-1`，在导入页执行激活预检；若仍有 blocker，记录原始证据并停止，不绕过状态机。
- [ ] 预检通过后运行 `go test ./e2e -run '^TestSeedanceImportedMaterialMatrixFullFlowE2E$' -count=1 -p=1`，仅使用 Mock。
- [ ] 运行 focused 回归、`git diff --check`，记录最新退出码与输出。
- [ ] 生成简体中文 `outputs/2026-08-23-sd-series-contract/验收报告.md`，包含路径/哈希、批次、激活统计、E2E、剩余 OmegaAI draft 和真实上游未执行说明。

## 自审

- 需求覆盖：共享合同、canonical model 传递、三渠道静态合同、四类运行时请求、Dimensio 模型分离、规则刷新、批次冲突、激活、Mock E2E 和报告均有任务。
- 占位符扫描：计划不使用 TBD/TODO 或未定义的后续动作；每一步都有具体文件和命令。
- 类型一致性：所有合同入口统一采用 `(channel, canonicalModel, target)`，运行时使用 `request.Model`，上游只使用 `UpstreamModelName`。
