# Seedance 提交前 Usage 兜底快照设计

## 背景

现有 Seedance 终态 usage 归一化能够在上游缺少合法 usage 时，根据任务计费上下文中的可信请求事实重新计算 token。该计算目前主要发生在轮询成功阶段。如果任务计费上下文被意外破坏，或者终态计算依赖的事实没有完整持久化，轮询只能记录错误并继续处理，无法从数据契约上证明所有新成功任务一定具有 usage。

本设计把本地 usage 的可计算性前移到供应商请求发送之前。所有新 Seedance 任务只有在系统已经得到一组合法、可持久化的本地 usage 兜底值后，才允许调用上游。这样，上游成功但不返回 usage 时，终态不再依赖临时重算才能满足用户响应契约。

## 目标

- 所有本设计上线后新提交并成功完成的 Seedance 任务，都具有合法的 `completion_tokens` 和 `total_tokens`。
- usage 的生成不受用户按 token 或按时长计费方式影响，也不受供应商成本规则影响。
- 上游缺少 usage、返回零值、字段不完整或越界值时，使用提交前锁定的本地 usage 兜底值。
- 上游已经接受任务后，内部 usage 异常不得触发用户退款，避免已产生供应商成本后造成收入损失。
- 本地 usage 不得冒充供应商实际 usage，也不得改变供应商成本台账的 meter source 语义。

## 范围

本设计只覆盖 `seedancepricing.Family` 能识别的 Seedance 系列模型，包括通过 Doubao、Dimensio、CLMM Mall、NewAPIVideo 及 Lucen、MegaByAI、Cangyuan、Paipu、Secure、OmegaAI、4stoken、8yes 等适配器提交的 Seedance 任务。

非 Seedance 视频模型不在本次范围内。历史任务缺少提交前快照时只根据已有可信事实尽力恢复，不承诺强制补齐，也不通过价格或已扣额度反推 token。

## 方案比较

### 方案一：终态失败并退款

当上游成功但本地 usage 无法计算时，把任务改为失败并退款。该方案能够避免返回缺少 usage 的成功响应，但供应商已经产生实际成本，退款会直接造成平台损失，因此不采用。

### 方案二：根据用户收费额度反推 usage

根据预扣额度、模型价格和分组倍率反推出 token。该结果会随管理员价格配置和用户分组变化，同一视频可能得到不同 usage，无法代表稳定的视频工作量，也会把计费结果反向污染用量事实，因此不采用。

### 方案三：提交前计算并持久化本地 usage 兜底值

在调用上游之前，使用已验证的请求时长、分辨率、帧率画像和参考视频聚合时长计算本地 usage。计算失败时拒绝提交；计算成功后把 token 对保存到任务上下文。终态优先使用合法上游 usage，否则使用本地值。

采用方案三。

## 数据契约

`TaskBillingContext` 增加一个快照版本和两个可选整数事实：

- `UsageSnapshotVersion`：提交前 usage 快照契约版本；本设计写入版本 `1`；
- `UsageCompletionTokens`：当前锁定的用户侧输出视频 token；
- `UsageTotalTokens`：当前锁定的用户侧总 token，包含需要计入 usage 的参考视频输入 token。

对应的临时字段在 `TaskRelayInfo` 中保存，用于上游请求尚未返回、任务记录尚未创建时传递提交前计算结果。`persistSubmittedTask` 在上游接受任务后把这两个值复制到 `TaskBillingContext`。提交时它们保存本地兜底值；终态归一化后，它们被更新为最终选定的用户侧 usage，无论最终来源是上游还是本地。

两个 token 字段必须同时存在、均为正数、`UsageTotalTokens >= UsageCompletionTokens`，并且不超过 `relaycommon.MaxTokensLimit`。`UsageSnapshotVersion=0` 表示历史任务，继续尽力恢复；版本 `1` 但 token 对缺失或非法表示新契约数据损坏，不能把单个残留字段当成有效快照。

现有字段继续承担原有职责：

- `BillingTokens` 保存用户 token 差额结算采用的最终 completion token；
- `UsageSource` 标记终态最终采用 `upstream` 还是 `local_calculated`；
- `UsageProfile=seedance` 标记任务受 Seedance usage 归一化约束。

## 提交流程

1. 完成模型映射、请求语义校验、能力路由默认值解析和参考视频元数据聚合。
2. 规范化最终请求时长与分辨率，并确保所有计费乘数满足共享上限。
3. 使用现有 `CalculateSeedanceTaskUsage` 和空终态事实，根据已验证请求快照计算本地 usage。
4. 校验 completion、total 的正数关系和 token 上限，把结果写入 `TaskRelayInfo`。
5. 只有上述步骤全部成功后才执行 `DoRequest` 调用供应商。
6. 上游接受任务后，`persistSubmittedTask` 把本地 token 对与 `UsageSnapshotVersion=1`、其他计费事实一起持久化。

提交前计算失败按原因分类：用户输入或媒体非法时返回 400；元数据服务不可用时返回 503；内部画像或模型配置缺失时返回 500。所有这些失败都发生在供应商调用之前，不产生上游成本。

## 终态归一化

成功轮询的 usage 选择顺序固定为：

1. 合法且未被 clamp 的上游 `completion_tokens` 与 `total_tokens`；
2. 使用合法终态时长、分辨率、帧率重新计算出的本地 usage；
3. 提交前持久化的 `UsageCompletionTokens` 与 `UsageTotalTokens` 兜底值。

第二步用于在上游明确返回实际输出规格时提高准确性。重新计算失败时不影响第三步的兜底能力。选择完成后始终把最终 completion/total token 对写回 `UsageCompletionTokens` 与 `UsageTotalTokens`，使公共响应不依赖原始上游载荷是否保留 usage 字段。

选择上游 usage 时设置 `UsageSource=upstream`。选择本地 usage 时设置 `UsageSource=local_calculated`，并把本地 token 对写入 `TaskInfo` 的数值和 presence 字段。两种来源都必须把最终 completion token 写入 `BillingTokens`，并持久化最终 completion/total token 对。

用户按 token 计费时，最终选择的 usage 进入现有差额结算。用户按时长计费时，终态仍保存和返回 usage，但跳过 token 差额结算，账单继续只由已验证时长决定。Seedance 用户侧不开放按次计费。

## 异常与历史兼容

对于带 `UsageProfile=seedance` 且 `UsageSnapshotVersion=1` 的新任务，如果上游 usage 非法、终态重算失败且提交前本地 token 对也缺失或非法，轮询不得提交一个缺少 usage 的成功终态。系统保持已有预扣和供应商成本状态，不退款，记录带任务 ID、请求 ID 和渠道 ID 的高优先级管理员告警，并让终态处理返回可重试的内部错误。

该异常只应由数据库损坏、旧节点写入不完整数据或程序缺陷触发。正常的新任务会在调用上游之前通过本地 token 对门禁。

`UsageSnapshotVersion=0` 的历史任务没有本地 token 对时继续执行既有尽力恢复逻辑：合法上游 usage 优先，其次使用已有可信计费上下文计算。仍无法恢复时不伪造 token，不根据价格反推 usage，也不自动退款。

## 公共响应

Seedance 成功任务的公共响应必须从 `TaskBillingContext` 中持久化的最终 completion/total token 对生成 usage，并校验其正数关系与共享上限：

- `UsageSource=upstream` 时，持久化值来自通过共享信任边界校验的上游 usage；
- `UsageSource=local_calculated` 时，持久化值来自终态重算或提交前兜底值；
- 新任务没有任何有效来源时，不返回缺少 usage 的成功响应，而是返回内部错误并触发管理员告警。

普通用户响应不暴露 `UsageSource`。管理员可通过任务计费上下文、结算日志和后端告警判断 usage 来源。原始上游响应审计数据保持不变。

## 供应商成本隔离

本地 token 对属于用户 usage 事实，不等同于供应商实际 token。`preparePolledTaskCostSettlement` 在 `UsageSource=local_calculated` 时，继续向供应商成本 normalizer 隐藏本地 token presence。

供应商成本规则使用 `validated_request` 时仍读取冻结请求 meter；使用 `upstream_usage` 时仍要求合法上游 usage。供应商缺少权威 token 时可以进入成本异常或人工核对状态，但不得阻止用户任务通过本地兜底 usage 完成，也不得把本地值伪装成供应商 meter。

## 测试策略

1. 提交门禁单元测试：Seedance 请求在上游调用前生成本地 completion/total token 对；缺少可信时长、分辨率或参考视频时长时上游调用次数为零。
2. 持久化测试：不同用户计费模式和供应商成本模式都写入相同的本地 token 对，且 Seedance 不启用用户按次计费。
3. 终态优先级测试：合法上游 usage 优先；合法终态事实更新本地值；终态重算失败时使用提交前值；三层来源都不可用时不提交成功 CAS，也不退款。
4. 公共响应测试：按 token 和按时长的成功任务都返回正数 usage，响应统一使用持久化的最终 token 对，上游非法 usage 不得覆盖本地结果。
5. 成本隔离测试：本地 token 对不进入供应商 `upstream_usage` meter；`validated_request` 时长成本规则保持不变。
6. Seedance 适配器矩阵 E2E：覆盖 Doubao、Dimensio、CLMM Mall、NewAPIVideo 及各协议 profile，并组合上游有 usage、无 usage、无效 usage、用户按 token、用户按时长和参考视频输入。
7. 历史兼容测试：没有提交前 token 对的旧任务仍能使用合法上游 usage 或旧计费上下文尽力恢复，无法恢复时不伪造数据。

## 验收标准

- 所有新提交的 Seedance 请求在供应商调用前已经具有合法本地 usage token 对。
- 所有对用户返回成功状态的新 Seedance 任务都包含合法 `completion_tokens` 和 `total_tokens`。
- 用户按 token 时 usage 参与差额结算；用户按时长时 usage 不改变时长账单。
- 上游 usage 缺失、为零、不完整或越界时稳定使用本地值。
- 终态 usage 内部异常不触发用户退款，也不产生缺 usage 的成功响应。
- 本地 usage 不进入要求上游权威 usage 的供应商成本 meter。
- 历史任务保持尽力兼容，不反推、不伪造、不自动退款。
- 相关单元测试、E2E、`go vet` 和 `git diff --check` 全部通过。
