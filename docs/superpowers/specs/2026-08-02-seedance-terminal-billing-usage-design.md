# Seedance 终态计费 Usage 归一化设计

## 目标

所有新提交且成功完成的 Seedance 任务都必须具有 `usage`，不受用户计费方式或供应商成本方式影响。当 Paipu、4stoken 等上游在成功轮询响应中不返回合法 usage 时，系统应在用户额度差额结算之前，基于提交时保存的已验证请求参数计算本地 `completion_tokens` 和 `total_tokens`。计算结果必须与 Ark 公共任务响应一致，使任务最终额度、结算日志和公共 usage 使用同一组可信事实。

本次修复不改变供应商成本台账对 `validated_request` meter 快照的处理，也不改变 Paipu 或 4stoken 的上游协议。

面向用户的 Seedance 计费只开放按 token 和按时长两种方式，不开放按次计费。按 token 时，终态 usage 参与用户额度结算；按时长时，终态 usage 仅用于用量展示与审计，用户价格仍由已验证时长决定。供应商的 `CostModePerRequest` 只描述上游成本，不得映射为用户按次计费。

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

### 方案三：持久化 usage 画像并共享纯计算核心

提交阶段在任务计费上下文中保存 `seedance` usage 画像。通用轮询服务按画像调用共享计算核心，不识别具体渠道类型；Seedance 任务适配器只负责将能够解析出的终态事实放入内存态 `TaskInfo`。Ark 公共响应也调用同一计算核心。该方案保持分层边界，覆盖所有 Seedance 新任务，并让 Paipu、4stoken 自动获得一致行为。

采用方案三。

## 架构

### 共享计算核心

在 `service` 中提供一个无副作用的 Seedance 终态 usage 计算函数。usage 的生成不得读取用户计费模式或供应商成本模式。输入包括：

- 持久化的 `TaskBillingContext`；
- 上游终态是否明确返回时长和帧率；
- 上游终态分辨率；
- 已验证请求时长、请求分辨率和参考视频聚合时长。

输出仅包含计算成功时的 `completion_tokens` 与 `total_tokens`。函数继续复用 `EstimateSeedanceTokens` 和 `seedancepricing.Profile`，不复制 token 公式。

事实优先级为：

1. 合法的上游终态时长、分辨率和帧率；
2. `TaskBillingContext.RequestedDurationSeconds`、`Resolution` 和共享分辨率画像默认帧率。

合法上游 usage 始终优先。上游 usage 缺失、全零或非法时，系统使用已验证请求事实计算本地 usage，并把来源标记为 `local_calculated`。上游明确返回非法的终态时长、分辨率或帧率时，这些值不进入计算；系统使用提交时保存的已验证请求事实，并保留本地计算来源，不能把结果记录为上游 usage。

### 终态归一化边界

`TaskBillingContext` 新增可选 usage 画像和 usage 来源字段。所有新 Seedance 任务在提交时写入 `seedance` 画像；旧任务缺少画像时保持兼容。轮询服务在终态持久化和用户差额结算之前，根据该画像归一化 `TaskInfo` usage，不硬编码 Paipu、4stoken 或其他渠道类型。

`TaskInfo` 增加仅供内部使用的终态时长、分辨率、帧率及其存在性。适配器能解析时写入这些事实；不能解析时不需要实现新的计费算法，共享核心会使用提交快照。`newapivideo` 需要保留其已解析的终态时长和帧率，避免在通用层重复解析供应商响应。

归一化遵循以下条件：

- 仅处理成功任务；
- 合法的权威上游 usage 原样保留；
- 上游 usage 缺失或不可用时，无论用户按 token 还是按时长、无论供应商按次、按时长还是按 token，都执行本地回退；
- 已知存在参考视频但没有持久化聚合时长时不得低估 usage；
- 计算结果必须通过现有 token 上限校验。

归一化成功后同时设置 token 数值、presence 标记和内部 usage 来源。用户按 token 计费时，`settleTaskBillingOnComplete` 走现有 token 差额结算路径；用户按时长计费时跳过 token 差额结算，但仍持久化 usage 与来源用于查询和审计。归一化不写入 `CostMeter`，不改变供应商成本台账的 meter source。供应商 `per_token` 规则缺少权威 usage 时，成本台账仍按既有规则进入异常或人工核对状态，不能把本地计算值伪装成供应商实际用量。

### 公共响应

现有 `populateSeedanceTaskUsage` 保留公共响应职责，但改为读取终态持久化结果或调用同一个共享计算核心。原始上游轮询响应和管理员审计载荷保持不变；本次修复不会把合成 usage 写回上游响应数据。公共响应不新增 usage 来源字段，来源仅保存在管理员可见的内部任务计费上下文和结算日志中。

### 提交门禁

usage 强保证适用于本次修复后新提交的成功任务。提交阶段必须在发送上游之前保存本地计算所需的完整可信事实：输出时长、输出分辨率、默认帧率画像，以及存在参考视频时的聚合时长。

该准备过程不再受成本核算开关或供应商成本模式限制。无法解析参考视频时长、不支持本地计算的分辨率、非法时长或缺失必要画像时，请求必须在上游发送前失败，避免产生一个完成后无法给出 usage 的新任务。

## 数据流

1. 提交阶段独立于用户计费和供应商成本模式，验证请求时长、分辨率与帧率画像，并把它们与参考视频聚合时长保存到 `TaskBillingContext`。
2. 轮询阶段由具体适配器解析成功响应，并在能够识别时保留终态时长、分辨率、帧率的值与存在性。
3. 通用轮询服务根据任务持久化的 `seedance` usage 画像调用共享归一化核心。
4. 若上游 usage 合法则保持原值并标记为 `upstream`；否则共享核心根据可信事实计算 usage 并标记为 `local_calculated`。
5. 用户按 token 计费时，`settleTaskBillingOnComplete` 使用归一化后的 token 重新计算实际额度，生成必要的补扣或退款日志；用户按时长计费时保持时长账单，usage 只进入任务结果与审计。
6. 供应商成本台账继续根据冻结规则与 `RequestMeterJSON` 独立结算。
7. Ark 任务查询使用同一核心生成一致的公共 usage，不修改上游审计数据。

## 失败处理与安全边界

- 非法、非整数、非正数或超过上限的上游时长和帧率不参与本地计算；新任务必须回退到提交时保存的已验证事实，并标记为 `local_calculated`。
- 不支持的请求分辨率、缺失的可信参考视频时长或缺少帧率画像必须在新任务发送上游前被拒绝。
- 如果上游成功后本地计算仍因数据损坏或 token 溢出失败，任务完成态不能被回滚；系统保留预扣结果，记录可关联任务的高优先级告警，并把该情况视为 usage 强保证违约。
- 所有成为计费乘数的时长和 token 必须继续受 `relaycommon.MaxTaskDurationSeconds` 与 `relaycommon.MaxTokensLimit` 约束。
- usage 画像和来源作为 `TaskBillingContext` 的可选字段保存，旧任务缺少这些字段时仍可读取。无需数据库迁移，不引入数据库方言差异。
- 历史任务只能使用已有快照尽力补算，不属于所有新成功任务的强保证范围。

## 测试策略

1. 共享计算测试覆盖请求事实回退、上游事实优先、非法上游事实改用请求快照、参考视频缺少时长、分辨率和 token 上限。
2. 提交门禁测试覆盖在成本核算关闭以及供应商 `per_token`、`per_request`、`per_duration` 三种模式下都保存完整 usage 事实，并在事实不可用时证明上游未被调用。
3. 轮询归一化测试覆盖带 `seedance` 画像且无 usage 的成功响应得到 presence 完整的本地 usage，并覆盖权威 usage 保留、供应商 `per_token` 缺 usage 时的本地回退，以及非 Seedance 画像不被处理。
4. 轮询结算测试验证用户按 token 时，本地 usage 会更新任务额度、`BillingTokens`、QuotaData 并生成差额日志；用户按时长时，额度保持时长结算结果，但任务仍保存 usage 与来源。
5. Ark 公共响应测试验证共享核心替换后输出不变，且公共 usage 与轮询结算 token 一致，不向普通用户暴露来源。
6. 素材矩阵 E2E 移除 Paipu/4stoken `per_duration` 的单日志例外，恢复为 1 条预扣消费日志和 1 条终态差额日志。
7. 成本台账断言验证 `validated_request` 规则仍以请求时长结算；供应商 `per_token` 缺权威 usage 时不使用本地计算值冒充成本 meter。
8. 新增计费模式边界测试，证明 Seedance 用户侧仅允许按 token 和按时长，供应商 `CostModePerRequest` 不会启用用户按次计费。

## 验收标准

- 所有新提交且成功完成的 Seedance 任务都返回合法的 `completion_tokens` 和 `total_tokens`。
- Paipu 和 4stoken 无上游 usage 的成功任务在用户按 token 计费时产生预期的终态差额日志。
- 用户按 token 时，任务最终额度、`BillingContext.BillingTokens`、QuotaData 和 Ark 公共 usage 相互一致；用户按时长时，usage 可查询但不改变时长账单。
- 合法上游 usage 不被覆盖；任何成本模式下缺失的公共 usage 都使用已验证事实本地计算，并明确记录内部来源。
- Seedance 用户侧不存在按次计费入口，供应商按次成本不会改变用户计费方式。
- `validated_request` 供应商成本台账仍从冻结请求 meter 结算。
- 供应商 `per_token` 缺权威 usage 时，本地 usage 不进入供应商成本 meter。
- 上游响应审计数据保持原样。
- 相关 relay、service、e2e 测试与 `git diff --check` 全部通过。
