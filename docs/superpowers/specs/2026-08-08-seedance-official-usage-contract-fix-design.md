# Seedance 官方 Usage 契约修复设计

## 背景

Seedance 视频生成接口的官方 `usage` 契约规定：

- `completion_tokens` 表示本次视频生成消耗的 Token，可用于计费对账；
- 视频生成不统计输入 Token，`input_tokens` 固定为 `0`；
- `total_tokens` 必须等于 `completion_tokens`；
- Token 用量公式为：

```text
token 用量 = (输入视频时长 + 输出视频时长) × 输出视频宽度 × 输出视频高度 × 输出视频帧率 / 1024
```

当前实现将公式拆成了输入视频 Token 和输出视频 Token，并把输出部分写入 `completion_tokens`、两部分之和写入 `total_tokens`。包含参考视频时，这会造成 `completion_tokens < total_tokens`，违反官方响应契约，也使任务详情中的 `completion_tokens` 不能直接用于官方计费对账。

## 目标

- 所有新 Seedance 任务都满足 `input_tokens=0` 和 `total_tokens=completion_tokens`。
- `completion_tokens` 使用输入视频时长与输出视频时长之和计算，不遗漏参考视频工作量。
- 用户预扣费、终态结算、利润预估、供应商成本计量和公共任务响应共享同一 Token 语义。
- 参考视频是否存在仍用于选择 `no_video` 或 `with_video` 官方价格场景。
- 参考视频聚合时长继续作为公式输入和审计事实，但不再表示为输入 Token。
- 不修改历史已结算任务，不自动回算历史日志或账单。

## 非目标

- 不修改 Seedance 分辨率、帧率、时长和参考素材能力限制。
- 不改变渠道模型映射和成本规则匹配逻辑。
- 不将第三方渠道响应中的模型 ID 用作本站计费模型。
- 不迁移或重新结算历史任务。

## 方案

### 统一 Token 计算结果

共享计算函数继续使用可信的输入视频聚合时长、输出时长、输出宽高和帧率。中间计算使用 `decimal.Decimal`，最终结果向上取整并遵守 `relaycommon.MaxTokensLimit`。

函数对外返回值统一为：

```text
input_tokens = 0
completion_tokens = ceil((input_duration + output_duration) × width × height × fps / 1024)
total_tokens = completion_tokens
```

不再分别向上取整输入和输出部分。对精确的输入与输出工作量求和后只向上取整一次，避免重复取整产生偏差。

### 任务 Usage 快照

`TaskBillingContext` 保留现有字段，以兼容数据库 JSON 结构：

- `UsageInputTokens` 对新任务固定写入 `0`；
- `UsageCompletionTokens` 写入完整公式结果；
- `UsageTotalTokens` 写入与 `UsageCompletionTokens` 相同的值；
- `InputVideoDurationMS` 继续保存参考视频聚合时长，用于公式、场景选择和管理员审计。

有效快照必须满足：

```text
UsageInputTokens == 0
UsageCompletionTokens > 0
UsageTotalTokens == UsageCompletionTokens
UsageTotalTokens <= MaxTokensLimit
```

### 终态归一化

成功轮询后按以下顺序选择 usage：

1. 上游同时返回合法的 `completion_tokens` 和 `total_tokens`，且两者相等并与冻结公式结果一致时，标记为 `upstream`；
2. 否则使用冻结请求事实重新计算，标记为 `local_calculated`；
3. 新任务重算失败时使用提交前合法快照；历史任务继续执行现有尽力恢复策略。

任何 `total_tokens != completion_tokens` 的上游 Seedance usage 都不能成为用户侧官方 usage，也不能覆盖提交前快照。

### 用户计费

Seedance 官方 Token 价格仍按 `resolution × no_video/with_video` 选择价格场景。计费基数改为统一后的完整 `completion_tokens`，由于 `total_tokens` 与其相等，使用任一字段得到相同金额。

`with_video` 场景仍要求存在经过验证的参考视频事实。参考视频时长参与 Token 公式，但不作为额外倍率，也不生成非零 `input_tokens`。

### 供应商成本与利润预估

利润预估和供应商 Token 成本 meter 使用相同的标准字段：

- `input_tokens=0`；
- `output_tokens=completion_tokens`；
- `completion_tokens=completion_tokens`；
- `total_tokens=completion_tokens`。

按请求时长计费的供应商规则保持不变，继续使用 `validated_request.duration_seconds`。本地计算 usage 仍不得冒充上游权威 meter；现有 `UsageSource=local_calculated` 隔离逻辑保持有效。

## 历史兼容

修复只影响新提交任务和修复上线后尚未完成、需要重新归一化的任务。已完成、已结算任务及其审计快照不回写、不重新扣费。

读取历史快照时，旧的 `UsageInputTokens > 0`、`UsageTotalTokens > UsageCompletionTokens` 不能被当作新的官方 usage 契约。若历史任务仍需要生成公共响应，则优先使用合法上游 usage；否则按现有可信时长和几何事实重新计算为新口径，无法计算时保持现有错误处理，不根据已扣金额反推 Token。

## 错误处理

- 输入或输出时长缺失、越界：保持现有请求校验错误。
- 分辨率、宽高或帧率缺失：拒绝计算并沿用现有 usage 上下文错误。
- Token 超过共享上限：继续失败关闭，不饱和为一个可计费值。
- 上游 usage 字段缺失、不相等、非整数、负数或越界：不采用上游值，使用本地合法快照。

## 测试策略

1. 共享公式测试：无参考视频时 `completion_tokens=total_tokens`；有参考视频时完整公式结果同时写入两字段，`input_tokens=0`。
2. 官方价格测试：`with_video` 场景允许 `input_tokens=0`，并以完整 `completion_tokens` 计算金额。
3. 提交快照测试：参考视频任务写入零输入 Token 和相等的 completion/total。
4. 终态归一化测试：相等且匹配公式的上游 usage 被采用；旧式拆分 usage 被本地新口径覆盖。
5. 公共响应测试：任务详情始终返回 `total_tokens=completion_tokens`。
6. 成本 meter 测试：Token 成本字段使用零输入和相等的 completion/output/total；按时长成本规则不受影响。
7. 回归测试：运行 `go test ./service ./types ./relay/...`，并执行 `git diff --check`。

## 验收标准

- 5 秒 480p、无参考视频：`completion_tokens=50220`、`total_tokens=50220`。
- 3 秒参考视频加 5 秒输出、480p：`completion_tokens=80352`、`total_tokens=80352`。
- 所有成功 Seedance 新任务均满足 `total_tokens=completion_tokens`。
- 参考视频仍能正确选择 `with_video` 单价场景。
- 用户收入、成本预估和任务详情不再使用互相冲突的 Token 字段口径。
