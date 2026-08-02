# Seedance 终态计费 Usage 归一化设计

## 目标

当 Paipu、4stoken 等 Seedance 任务上游在成功轮询响应中不返回 `usage` 时，系统应在用户额度差额结算之前，基于提交时保存的已验证请求参数计算本地 `completion_tokens` 和 `total_tokens`。计算结果必须与 Ark 公共任务响应已有的本地 usage 回退一致，使任务最终额度、结算日志和公共 usage 使用同一组可信事实。

本次修复不改变供应商成本台账对 `validated_request` meter 快照的处理，也不改变 Paipu 或 4stoken 的上游协议。

## 根因

当前系统存在两个相互独立的终态处理阶段：

1. 任务轮询完成后，`settleTaskBillingOnComplete` 根据适配器调整结果或 `TaskInfo` 中的上游 token usage 结算用户额度。Paipu 和 4stoken 的成功响应没有 usage，`newapivideo` 的默认 `AdjustBillingOnComplete` 又返回零，因此系统保留预扣额度，不产生差额日志。
2. 用户随后查询 Ark 任务时，`populateSeedanceTaskUsage` 才根据 `TaskBillingContext` 合成本地 usage。该计算发生在用户额度结算之后，只影响公共响应，无法补做差额结算。

`newapivideo.TaskAdaptor` 已通过 `taskcommon.BaseBilling` 实现 `NormalizeTaskCostMeter`。供应商成本台账还会在规则 meter source 为 `validated_request` 时，从 `CostAccountingAttempt.RequestMeterJSON` 恢复请求时长。因此，单日志问题不是供应商成本 meter 丢失，不能通过无条件改写 `NormalizeTaskCostMeter` 解决。

## 方案比较

### 方案一：扩展 `NormalizeTaskCostMeter`

让 `newapivideo` 在缺少 usage 时返回 `validated_request` 时长。该方案不能给用户额度结算提供 token usage，而且适配器无法仅凭任务判断冻结规则期望的是 `validated_request`、`upstream_actual` 还是 `upstream_usage`。无条件回退可能把请求时长错误地当成上游实际时长，因此不采用。

### 方案二：在轮询服务中按渠道类型硬编码

在 `service/task_polling.go` 中识别 Paipu 和 4stoken，并直接计算 usage。该方案能修复当前现象，但会让通用轮询服务持有具体供应商知识，并与 Ark 公共响应中的同类算法继续重复，后续新渠道容易再次漂移，因此不采用。

### 方案三：新增终态 usage 归一化契约并共享纯计算核心

在任务适配器边界新增终态 usage 归一化能力。通用轮询服务只在适配器声明该能力时调用；`newapivideo` 负责将其解析出的终态事实交给共享 Seedance 计算核心。Ark 公共响应也调用同一计算核心。该方案保持分层边界，覆盖所有 `newapivideo` 协议画像，并让 Paipu、4stoken 自动获得一致行为。

采用方案三。

## 架构

### 共享计算核心

在 `service` 中提供一个无副作用的 Seedance 终态 usage 计算函数。输入包括：

- 持久化的 `TaskBillingContext`；
- 上游终态是否明确返回时长和帧率；
- 上游终态分辨率；
- 已验证请求时长、请求分辨率和参考视频聚合时长。

输出仅包含计算成功时的 `completion_tokens` 与 `total_tokens`。函数继续复用 `EstimateSeedanceTokens` 和 `seedancepricing.Profile`，不复制 token 公式。

事实优先级为：

1. 合法的上游终态时长、分辨率和帧率；
2. `TaskBillingContext.RequestedDurationSeconds`、`Resolution` 和共享分辨率画像默认帧率。

上游明确返回某个事实但其值非法时，不得回退到请求值掩盖异常。

### 适配器契约

轮询服务新增一个内部可选接口，用于在终态持久化和用户差额结算之前归一化 `TaskInfo` usage。`newapivideo.TaskAdaptor` 实现该接口，并使用解析阶段保存的终态时长、分辨率和帧率调用共享计算核心。

归一化遵循以下条件：

- 仅处理成功任务；
- 合法的权威上游 usage 原样保留；
- 仅对成本模式为 `per_request` 或 `per_duration` 的任务执行本地回退；
- `per_token` 任务缺少 usage 时不得本地推测；
- 已知存在参考视频但没有持久化聚合时长时不得低估 usage；
- 计算结果必须通过现有 token 上限校验。

归一化成功后同时设置 token 数值和 presence 标记，使 `settleTaskBillingOnComplete` 走现有 token 差额结算路径。它不写入 `CostMeter`，不改变供应商成本台账的 meter source。

### 公共响应

现有 `populateSeedanceTaskUsage` 保留公共响应职责，但改为调用同一个共享计算核心。原始上游轮询响应和管理员审计载荷保持不变；本次修复不会把合成 usage 写回上游响应数据。

## 数据流

1. 提交阶段验证请求时长、分辨率，并把它们与参考视频聚合时长保存到 `TaskBillingContext`。
2. 轮询阶段由 `newapivideo` 解析成功响应，并保留终态时长、分辨率、帧率的值与存在性。
3. 通用轮询服务调用可选终态 usage 归一化接口。
4. 若上游 usage 合法则保持原值；否则共享核心根据可信事实计算本地 usage。
5. `settleTaskBillingOnComplete` 使用归一化后的 token 重新计算用户实际额度，生成必要的补扣或退款日志。
6. 供应商成本台账继续根据冻结规则与 `RequestMeterJSON` 独立结算。
7. Ark 任务查询使用同一核心生成一致的公共 usage，不修改上游审计数据。

## 失败处理与安全边界

- 非法、非整数、非正数或超过上限的上游时长和帧率不参与本地计算，也不回退到请求值。
- 不支持的分辨率、缺失的可信参考视频时长或 token 溢出均保持当前预扣额度，不生成伪造 usage。
- 归一化失败不得阻止已成功的上游任务进入完成态；系统保留预扣结果，并记录可关联任务的告警。
- 所有成为计费乘数的时长和 token 必须继续受 `relaycommon.MaxTaskDurationSeconds` 与 `relaycommon.MaxTokensLimit` 约束。
- 不新增数据库字段，不修改持久化 JSON 格式，不引入数据库方言差异。

## 测试策略

1. 共享计算测试覆盖请求事实回退、上游事实优先、显式非法事实不回退、参考视频缺少时长、分辨率和 token 上限。
2. `newapivideo` 测试覆盖无 usage 的 Paipu/4stoken 成功响应在轮询结算前得到 presence 完整的本地 usage，并覆盖权威 usage 保留与 `per_token` 禁止回退。
3. 轮询结算测试验证本地 usage 会更新任务额度、`BillingTokens`、QuotaData，并生成差额日志。
4. Ark 公共响应测试验证共享核心替换后输出不变，且公共 usage 与轮询结算 token 一致。
5. 素材矩阵 E2E 移除 Paipu/4stoken `per_duration` 的单日志例外，恢复为 1 条预扣消费日志和 1 条终态差额日志。
6. 成本台账断言验证 `validated_request` 规则仍以请求时长结算，`CostAccountingAttempt` 最终为 `settled`，不依赖合成 token usage。

## 验收标准

- Paipu 和 4stoken 无上游 usage 的 `per_duration` 成功任务完成后产生预期的第二条差额日志。
- 任务最终额度、`BillingContext.BillingTokens`、QuotaData 和 Ark 公共 usage 相互一致。
- 合法上游 usage 不被覆盖，`per_token` 缺失 usage 不被本地推测。
- `validated_request` 供应商成本台账仍从冻结请求 meter 结算。
- 上游响应审计数据保持原样。
- 相关 relay、service、e2e 测试与 `git diff --check` 全部通过。
