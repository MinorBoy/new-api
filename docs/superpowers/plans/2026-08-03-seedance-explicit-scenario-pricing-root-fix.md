# Seedance 显式场景定价根治实施计划

> **面向执行代理：** 必须按任务逐项执行，每个任务先写失败测试、确认 RED，再写最小实现并确认 GREEN。

**设计文档：** `docs/superpowers/specs/2026-08-03-seedance-explicit-scenario-pricing-design.md`

**目标：** 删除 Seedance 的官方矩阵倍率计费，改为按分辨率和输入场景保存版本化绝对单价，使用“官方场景输出单价 × 输出计费秒数 × 分组倍率”的可审计模型，并完成模板导入和 E2E 验收。参考视频时长不进入用户售价。

**架构：** 导入层从模板的官方售价行读取 `no_video`/`with_video` 两个场景的绝对 `USD/基准秒`，生成 `DurationPrice.Scenarios`。运行时在请求校验后冻结场景定价快照，按输出计费秒数乘官方场景单价，再应用分组倍率；参考视频时长只保留为已校验的能力和供应商成本计量事实，不进入用户售价。异步任务、使用日志和成本核算都读取同一个快照，供应商成本仍由渠道成本规则独立计算。

**技术栈：** Go 1.22、Gin、GORM、shopspring/decimal、React/TypeScript、Bun、Vitest、现有 E2E 测试框架。

## 文件边界

- 修改 `types/duration_price.go`、`types/price_data.go`、`model/task.go`：定义场景价格结构、校验和任务快照。
- 修改 `relay/relay_task.go`、`relay/helper/cost_preview.go`、`service/task_billing.go`：实现官方场景输出计费、预览一致性和审计分项。
- 修改 `types/config_import.go`、`service/config_import_stage.go`、`service/config_import_publish.go`：导入场景价格，删除 Seedance 隐式生成，发布时深度合并并拒绝同场景冲突。
- 修改 `web/src/channel-config-converter/document.ts`、相关 workbook 类型/校验和模板脚本：把官方售价场景与 USD/基准秒传到导入文档。
- 修改相关 Go/TypeScript 测试，删除锁定旧倍率公式的测试，新增缺失配置、冲突和日志合同测试。
- 生成 `outputs/` 下的新渠道模板和中文验收报告；不修改用户已有的 `web/public/favicon.ico`、`web/public/logo.png`。

## 实施任务

### 任务 1：建立显式场景价格合同

- 为 `DurationPrice` 增加版本、来源和按场景保存的官方输出单价；保持普通模型既有单一秒价能力。
- 为场景价格增加非负有限数、单位、舍入和最短时长校验；有参考视频时只校验真实素材时长，不生成输入视频售价。
- 在 `PriceData`/`TaskBillingContext` 中保存场景、输入时长和费用分项，禁止把场景价格编码为 `OtherRatios`。
- 先新增测试：无视频和有视频都只按对应官方场景输出秒价计费；缺分辨率场景价直接阻断；序列化快照保留来源与版本以及输入视频时长事实。

### 任务 2：替换运行时和成本预览

- 在 `taskDurationQuota` 使用显式场景价格，按 decimal 计算输出项，应用分组倍率并使用 checked quota 转换；参考视频时长只做能力校验和审计记录。
- 删除 `applySeedanceDurationPricing` 及其调用，不再写入 `seedance_price_matrix`。
- 成本预览调用与任务提交使用同一计费函数，输入视频时长必须来自已校验素材元数据。
- 先让旧倍率测试失败，再更新为官方场景输出计费合同测试；验证任务预扣、重算和异步结算使用冻结快照。

### 任务 3：日志和利润审计

- 任务快照日志写入场景、定价版本、输出单价、输出费用、计费秒数、总费用和参考视频时长事实，不再输出 `seedance_price_matrix`，也不生成参考视频输入售价。
- 供应商成本按渠道成本规则独立记录；收入、成本、毛利润和毛利率必须使用各自冻结的价格事实计算，负毛利请求必须如实展示。
- 添加负毛利审计字段测试，确保异步任务轮询不重新读取可变价格；只有现有利润路由策略明确启用门槛时才执行阻断或改路由。

### 任务 4：模板和导入根治

- 模板导出的官方售价行继续生成 `no_video`/`with_video`，但明确写出 USD/基准秒；导入文档将场景和绝对秒价放入 `DurationPrice.Scenarios`。用户售价只由该官方场景单价和分组倍率决定，不能读取渠道成本字段。
- 删除 `configImportSeedanceDurationPrice`、官方矩阵/像素比例生成基础价及 Seedance 销售兼容回退。
- 发布多个场景时深度合并同模型的场景 map；同一模型同一场景的不同价格直接生成冲突，不覆盖先后值。
- 增加导入、冲突、深度合并和模板转换测试。

### 任务 5：清理旧规则并验收

- 删除或断开 `pkg/seedancepricing.DurationMultiplier` 作为价格来源的实现和旧测试；保留能力校验所需的非价格资料时，禁止其进入销售计费链路。
- 用最新收录数据生成新渠道模板，清理测试库旧销售价格/旧 Seedance 价格选项后重新导入。
- 重跑所有渠道、模型、素材限制和 mock 上游 E2E，核验使用日志、任务日志、成本核算、利润和定价审计字段。
- 生成中文报告，列出测试范围、路由、费用分项、阻断项和提交号。

## 验证命令

```text
go test ./types ./relay ./relay/helper ./service ./pkg/seedancepricing -count=1
go test ./e2e -count=1
cd web && bun test && bun run typecheck && bun run build
```

完成前额外执行 `rg` 检查生产代码不再出现 `seedance_price_matrix`、`applySeedanceDurationPricing` 和 `DurationMultiplier` 价格调用，并检查 `git diff` 确认未包含用户图片改动。
