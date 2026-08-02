# ARK 视频路由合约根因修复实施计划

> **给 agent 工作者：** 必选子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实施本计划。步骤使用复选框（`- [ ]`）语法跟踪进度。

**目标：** 在超出已验证提供商协议的 Seedance 路由目标被启用之前将其阻止，修正旧的 4stoken 绑定语义，并重新运行素材矩阵 E2E，不再把适配器拒绝视为成功验收。

**架构：** relay 暴露一个纯渠道路由合约校验器，其背后是每个专属任务适配器的文档化规则。service 在保存启用的路由目标时调用该校验器；配置导入对已知的 4stoken 旧类型做归一化，并拒绝绑定到不兼容渠道类型的任务行。E2E 区分静态被阻止的目标与成功的端到端任务，并验证被阻止的目标不产生任何计费或日志记录。

**技术栈：** Go 1.22+、Gin、GORM v2、testify、SQLite E2E、MySQL 持久化验收。

---

### 任务 1：提供商路由合约

**文件：**
- 新建：`relay/video_route_contract.go`
- 新建：`relay/video_route_contract_test.go`
- 修改：`relay/channel/task/newapivideo/profile.go`
- 修改：`relay/channel/task/clmmmall/translate.go`
- 修改：`relay/channel/task/dimensio/constants.go`

- [x] **步骤 1：编写失败的表格测试**

覆盖 Cangyuan/Paipu 媒体过度声明、CLMM 音频/1080p/模型语法、Dimensio 模型/分辨率/组合上限、Secure 分组规则，以及有效的 4stoken/Lucen/MegaByAI 对照组。断言稳定的问题编码，如 `route_contract_input_mode`、`route_contract_model`、`route_contract_resolution`、`route_contract_duration` 与 `route_contract_references`。

- [x] **步骤 2：运行合约测试并确认 RED**

运行：`go test ./relay -run TestValidateVideoRouteTargetContract -count=1`

预期：FAIL，因为 `ValidateVideoRouteTargetContract` 尚不存在。

- [x] **步骤 3：实现提供商合约校验器**

使用 `model.Channel`、`modelrouting.Target`、归一化后的渠道设置、精确模型列表、CLMM 模型控制解析、时长边界、声明的输入模式以及独立/组合参考上限。返回带稳定编码的带类型错误。不添加未经验证的提供商字段。

- [x] **步骤 4：运行聚焦的提供商测试**

运行：`go test ./relay ./relay/channel/task/newapivideo ./relay/channel/task/clmmmall ./relay/channel/task/dimensio -count=1`

预期：PASS。

### 任务 2：路由策略失败即关闭门禁

**文件：**
- 新建：`service/route_contract.go`
- 修改：`service/routing_policy.go`
- 修改：`service/routing_policy_test.go`
- 修改：`main.go`

- [x] **步骤 1：编写失败的 service 合约测试**

让 `service.RouteTargetContractValidator` 对一个启用目标返回类型化的不兼容。断言 `SaveRoutingPolicy` 返回 `incompatible_channel_contract`，指向 `targets.0.constraints`，且不持久化任何策略或目标。

- [x] **步骤 2：运行 service 测试并确认 RED**

运行：`go test ./service -run TestSaveRoutingPolicyRejectsIncompatibleChannelContract -count=1`

预期：FAIL，因为回调未被调用。

- [x] **步骤 3：加入门禁与生产接线**

在 `service/route_contract.go` 中定义回调。在 `SaveRoutingPolicy` 中，加载目标渠道，并在归一化之后、`ReplaceRoutingPolicy` 之前校验每一个启用目标。在 `main.go` 中注册 `relay.ValidateVideoRouteTargetContract`。

- [x] **步骤 4：验证 service 与 relay 测试**

运行：`go test ./service ./relay -count=1`

预期：PASS。

### 任务 3：配置导入类型与协议安全

**文件：**
- 修改：`service/config_import_stage.go`
- 修改：`service/config_import_stage_test.go`

- [x] **步骤 1：编写失败的绑定测试**

新增一个用例，证明旧的 `CH-4STOKEN` 类型 `1` 可以绑定到专属类型 `209` 渠道；另加一个用例，证明未解决的 `8yes` 任务协议无法绑定到 OpenAI 类型 `1` 渠道。

- [x] **步骤 2：运行绑定测试并确认 RED**

运行：`go test ./service -run 'TestConfigImportBindingNormalizesLegacyFourSTokenType|TestConfigImportBindingRejectsTaskProtocolOnOpenAIChannel' -count=1`

预期：第一个用例以 `BINDING_CHANNEL_TYPE` 失败；第二个用例错误地成功。

- [x] **步骤 3：实现导入边界的归一化**

仅将已验证的 `CH-4STOKEN` 身份归一化为类型 `209`。对于 `protocol=task`，以 `BINDING_CHANNEL_PROTOCOL` 拒绝 OpenAI 及其他非任务渠道类型。8yes 保持未解决；不为其指定猜测的类型。

- [x] **步骤 4：验证导入测试**

运行：`go test ./service -run 'ConfigImport.*Binding' -count=1`

预期：PASS。

### 任务 4：素材矩阵 E2E 合约发现项

**文件：**
- 修改：`e2e/seedance_material_matrix_e2e_test.go`
- 修改：`cmd/ark-video-material-seed/main.go`
- 修改：`cmd/ark-video-material-seed/main_test.go`

- [x] **步骤 1：替换预期适配器拒绝断言**

注册路由合约校验器。对每个导入目标先保存路由。合约冲突必须发生在提交之前，并且任务、日志、配额数据、成本请求、成本尝试与 mock 调用必须保持零增量。移除作为验收机制的 `expectedMaterialMatrixRejection` 与 `expectedTargetRejection`。

- [x] **步骤 2：对未接线的 service 门禁确认 RED**

运行：`go test ./e2e -run TestSeedanceImportedMaterialMatrixFullFlowE2E -count=1`

预期：如果有不兼容目标到达适配器提交或持久化路由，则 FAIL。

- [x] **步骤 3：更新 seed 摘要与报告数据**

将拒绝计数改名为被阻止的路由合约发现项。成功目标继续走完提交、轮询、任务查询、配额结算、用量日志与成本尝试。持久化 seed 必须成功退出，同时确定性地报告每个被阻止的目标。

- [x] **步骤 4：运行聚焦的 seed 与 E2E 测试**

运行：`go test ./cmd/ark-video-material-seed ./e2e -run 'TestSeedanceImportedMaterialMatrixFullFlowE2E|TestLoadTargets|TestRouteContract' -count=1`

预期：PASS，26 个成功目标与 72 个提交前合约阻止；除非更严格的模型校验扩大被阻止集合。最终精确计数必须来自测试输出，而非假设。

### 任务 5：持久化验收与报告

**文件：**
- 修改：`docs/superpowers/reports/2026-08-01-ark-sdk-video-material-matrix-acceptance.md`

- [x] **步骤 1：运行所有相关 Go 测试**

运行：`go test ./cmd/ark-video-material-seed ./service ./relay/... -count=1`

运行：`go test ./e2e -run 'TestSeedanceImportedMaterialMatrixFullFlowE2E|TestSeedanceCapabilityRoutingMatrixE2E|TestSeedanceNativeSeedance20MultimodalE2E' -count=1 -v`

预期：PASS。

- [x] **步骤 2：重新构建本地应用并重跑 MySQL seed**

运行现有本地 Compose 构建，并在应用容器内针对控制台 MySQL 执行 `cmd/ark-video-material-seed`。保持供应商 HTTP 指向本地 mock 服务器。

- [x] **步骤 3：查询持久化证据**

验证任务、类型 2 用量日志、配额数据、成本请求、已结算尝试、启用的 CNY 规则、零 `USD 0.2` 占位符、渠道类型分布与利润总额。

- [x] **步骤 4：用最终实测计数更新报告**

将被阻止的合约发现项与成功任务分开记录，并说明它们是在供应商派发之前被捕获的配置缺陷。

- [x] **步骤 5：格式化并验证最终 diff**

运行：`gofmt -w <变更的 Go 文件>`

运行：`git diff --check`

预期：退出码 0。

- [x] **步骤 6：提交任务变更**

仅暂存 ARK 路由合约、E2E、成本核算、seed、计划与报告文件。排除无关的工作区文件。以描述失败即关闭协议根因修复的提交信息提交。
