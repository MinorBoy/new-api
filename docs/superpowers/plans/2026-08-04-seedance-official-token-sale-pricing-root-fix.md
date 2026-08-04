# Seedance 官方 Token 售价根治实施计划

> **面向执行代理：** 必须按任务逐项执行，每项先写失败测试并确认 RED，再写最小实现并确认 GREEN。

**目标：** 删除 Seedance 旧场景秒价合同，改为官方 `USD/1M Token × 总 Token × 分组倍率` 的唯一用户售价链路。

**架构：** 新增 `seedance_tokens` 计费模式和独立设置结构，复用已验证的 Seedance Token 用量计算。模板、导入、发布、预扣、预览、任务快照和日志全部使用同一冻结合同；普通时长计费和供应商成本不变。

**技术栈：** Go 1.22、Gin、GORM、shopspring/decimal、React/TypeScript、Bun、ExcelJS、Vitest、项目 E2E。

---

### 任务 1：建立官方 Token 售价类型

**文件：**
- 新建 `types/seedance_token_price.go`
- 修改 `types/price_data.go`
- 修改 `model/task.go`
- 测试 `types/seedance_token_price_test.go`

- [ ] 写失败测试：验证 Mini 480p、输入 3000 ms、输出 4 秒产生 70308 Token，按 `14/7.3 USD/1M` 计算基础售价。
- [ ] 实现价格场景校验、选择、decimal 计费和审计分项。
- [ ] 验证无视频、有视频、缺场景、非法单价和超限 Token。

### 任务 2：接入运行时预扣、预览和日志

**文件：**
- 修改 `setting/billing_setting/tiered_billing.go`
- 新建 `setting/billing_setting/seedance_token_billing.go`
- 修改 `relay/helper/price.go`
- 修改 `relay/relay_task.go`
- 修改 `relay/helper/cost_preview.go`
- 修改 `service/cost_preview.go`
- 修改 `service/task_billing.go`
- 修改相关 Go 测试

- [ ] 写失败测试：`seedance_tokens` 模式必须使用输入/输出/总 Token，并得到 `$0.168546575342`。
- [ ] 在请求校验和 Token 快照后计算配额，冻结审计分项。
- [ ] 让成本预览、利润预览、消费日志和任务日志读取同一分项。
- [ ] 删除 Seedance 对 `per_duration` 和场景秒价的运行时分支。

### 任务 3：替换模板和导入合同

**文件：**
- 修改 `web/scripts/channel-model-template/build.ts`
- 修改 `web/scripts/channel-model-template/types.ts`
- 修改 `web/scripts/channel-model-template/write.ts`
- 修改 `web/src/channel-config-converter/document.ts`
- 修改 `types/config_import.go`
- 修改 `service/config_import_schema.go`
- 修改 `service/config_import_stage.go`
- 修改 `service/config_import_publish.go`
- 修改相关 TypeScript/Go 测试

- [ ] 写失败测试：官方售价行必须输出 `seedance_tokens` 和精确 `USD/1M`，不得输出 Seedance `duration_price`。
- [ ] 导入并校验官方单价、宽、高、帧率、版本和来源。
- [ ] 发布时合并场景并彻底清理旧 Seedance 售价选项。
- [ ] 同一模型同一场景价格冲突必须回滚。

### 任务 4：清理旧合同并重生成数据

**文件：**
- 修改 `types/duration_price.go` 及旧 Seedance 场景测试
- 重生成 `outputs/2026-08-03-import/渠道模型成本与利润模板-v1.xlsx`
- 重生成 `e2e/testdata/channel-config-v1.json`

- [ ] 删除 `DurationPrice.Scenarios`、`DurationPriceScenario` 和 Seedance `output_price` 生产引用。
- [ ] 运行残留扫描，确认 Seedance 导入配置不存在旧字段。
- [ ] 使用最新 `docs/new-channels/sd收录.xlsx` 重生成模板和导入 JSON。

### 任务 5：导入、E2E 和报告

**文件：**
- 修改 Seedance E2E 期望
- 更新 `docs/superpowers/reports/2026-08-03-seedance-official-sale-pricing-acceptance.md`

- [ ] 清理本地测试环境中的旧 Seedance 售价选项并导入新配置。
- [ ] 重跑持久化 Ark SDK 素材矩阵 E2E，核验使用日志、任务日志、成本和利润。
- [ ] 运行 Go 全量测试、前端测试、类型检查和生产构建。
- [ ] 更新中文验收报告，记录正确公式快照和测试证据。

### 任务 6：审查并提交

- [ ] 检查 `git diff`、残留旧合同、生成物哈希和测试结果。
- [ ] 提交根治改动。
