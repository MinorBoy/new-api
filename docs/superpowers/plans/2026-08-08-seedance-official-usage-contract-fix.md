# Seedance 官方 Usage 契约修复实施计划

**目标：** 所有新 Seedance 任务统一返回并结算 `input_tokens=0`、`total_tokens=completion_tokens`，其中 `completion_tokens` 按输入视频与输出视频的合计时长计算。

**架构：** 保留现有的 `EstimateSeedanceTokens` 和 `CalculateSeedanceTaskUsage` 共享边界，统一修改其 Usage 元组语义。参考视频时长继续作为公式与审计事实，并继续参与 `with_video` 价格场景选择；任务快照、上游 Usage 信任校验、售价校验和公共任务响应全部使用同一口径。

**技术栈：** Go 1.22+、GORM 任务快照、`shopspring/decimal`、Testify。

---

## 任务 1：增加官方 Usage 回归测试

**文件：** `service/profit_routing_test.go`

- [x] 增加 3 秒输入视频加 5 秒输出、480p 的确定性用例，期望返回 `0, 80352, 80352`。
- [x] 运行定向测试，确认旧实现返回 `30132, 50220, 80352` 并按预期失败。

## 任务 2：修改共享 Seedance Token 元组

**文件：** `service/profit_routing.go`、`service/seedance_task_usage.go`

- [x] 保留 Decimal 精确计算和合计后单次向上取整，统一返回 `0, totalCeil, totalCeil`。
- [x] 更新注释，明确输入视频时长计入完整的生成 Token，但不产生输入 Token。
- [x] 运行回归测试并确认通过。

## 任务 3：约束任务快照和官方售价计算

**文件：** `service/seedance_task_usage.go`、`types/seedance_token_price.go` 及对应测试

- [x] 售价计算只接受 `InputTokens=0` 且 `OutputTokens=TotalTokens` 的 Usage。
- [x] 任务快照只接受 `UsageInputTokens=0` 且 `UsageCompletionTokens=UsageTotalTokens`。
- [x] 保留非零输入 Token、字段关系错误和越界值的拒绝测试。
- [x] 参考视频用例改为将完整时长写入 completion/total。

## 任务 4：统一中继、成本计量和公共任务响应

**文件：** `relay/`、`service/task_billing_test.go`、`controller/`、`e2e/` 中的相关测试

- [x] Token meter 使用 `InputTokens=0`，并令 output/completion/total 相等。
- [x] 任务提交快照和终态结算使用统一字段语义。
- [x] 公共任务响应强制 `total_tokens=completion_tokens`。
- [x] 成本预览、控制器持久化和 Seedance 生命周期端到端用例迁移到新契约。

## 任务 5：完整验证

- [x] 运行 `go test ./... -count=1`。
- [x] 运行 `gofmt` 和 `git diff --check`。
- [x] 确认 5 秒 480p 无输入视频为 `50220/50220`，3 秒输入加 5 秒输出为 `80352/80352`。
