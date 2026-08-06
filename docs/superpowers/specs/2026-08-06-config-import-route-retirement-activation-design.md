# 配置导入路由归属、软退休与批次激活设计

## 目标

将配置导入从“发布后仍需逐个手工启用渠道、策略和目标”升级为可长期维护的两阶段流程：

1. 管理员只负责首次手工创建真实上游渠道并录入 API Key。
2. 后续下载最新 `sd收录.xlsx`，生成渠道模板和无凭据导入 JSON。
3. 导入 JSON，完成渠道线绑定、冲突处理、定价与路由审阅。
4. 发布批次，生成可审计但尚未参与流量的候选配置。
5. 执行批次激活，在一个事务内启用本批次路由目标、策略和绑定渠道，并软退休被本批次替代的旧导入路由。

激活成功后，新请求只使用当前批次的有效导入路由；历史导入路由保留在数据库中供审计，不立即物理删除。人工创建的路由目标不因配置导入而被删除或自动退休。

## 当前问题

现有实现已经具备上传、绑定、暂存、校验、发布、成本规则激活、模型映射和缓存刷新，但不具备批次级路由归属与激活边界：

- `route_targets` 没有来源字段，无法区分人工目标和配置导入目标。
- `configImportMergeRouteTargets` 只按 `RouteTarget.Name` 合并，旧目标可能长期保留，也可能在完整替换时失去来源信息。
- `ReplaceRoutingPolicyWithTx` 删除策略下全部目标再重建，目标 ID 和历史归属无法稳定保留。
- 导入发布时策略和新目标固定为禁用，没有后续批次级激活入口。
- 渠道状态、`abilities.enabled`、策略、目标和缓存没有一个统一的原子激活操作。
- 模型快照发布会直接禁用引用下架模型的任意启用目标，无法保证人工目标不受导入影响。
- 已发布批次不能重新绑定；历史 Mock 批次不能直接改绑为真实渠道，必须通过新批次完成真实渠道绑定和切换。

## 核心原则

### 两阶段发布

`publish` 和 `activate` 是两个不同动作：

- `publish`：固化审核结果，写入禁用候选目标并封存激活基线，不修改活动售价、活动成本、渠道模型、能力、旧路由或运行时缓存。
- `activate`：重新执行运行时门禁，在事务内应用售价、激活成本草稿、替换模型映射、完成新旧路由切换、启用渠道与能力，并写入激活审计。

发布不代表接入真实流量。只有激活完成且缓存刷新成功后，本批次才具备真实请求条件。

发布到激活之间不能提前修改任何活动售价、活动成本规则、渠道模型快照、能力状态或旧路由状态，否则旧路由可能在切换前使用新成本或失去模型能力。现有发布函数中的活动配置写入必须移动到激活事务。

### 显式复制为新绑定批次

相同权威配置重复上传继续保持幂等，返回已有批次；不得因为已发布批次无法重新绑定而让普通上传静默创建重复批次。

对需要重新绑定真实渠道的已发布批次，提供显式操作：

```text
POST /api/config-import/batches/:id/copy-for-binding
```

该操作在事务内创建一个新的 `binding` 批次，保留源批次的 schema、模板版本、源文件哈希、权威载荷哈希和规范化实体内容，并记录 `copied_from_batch_id` 和当前管理员。新批次不复制渠道绑定、凭据确认、冲突处理记录、问题状态、物化 ID、发布审计、激活审计、失败状态和缓存状态；所有实体状态重置为 `new`，由新绑定后的暂存与校验重新计算。

`payload_sha256` 继续表达权威实体内容，因此源批次与复制批次允许相同。批次新增内部唯一 `deduplication_key`：普通上传固定使用 `upload:<payload_sha256>`，显式复制使用独立的 `copy:<uuid>`。这样既保留重复上传的并发幂等约束，也不会伪造新的权威载荷哈希。

复制只接受 `published` 源批次；未发布批次继续通过原批次恢复流程维护。复制成功后前端立即进入新批次的渠道绑定步骤，源批次保持不变。

### 软退休

“旧路由退休”定义为：

- `enabled=false`，停止参与新请求路由。
- 写入 `retired_at`，记录退休时间。
- 保留完整目标行、来源批次和策略关系。
- 不物理删除历史导入目标。

### 人工配置优先保护

任何无法确定来源的历史目标默认视为 `manual`。配置导入激活只能自动退休 `managed_by=config_import` 的目标，不能自动退休 `manual` 目标。

如果人工目标与即将激活的导入目标产生同优先级能力重叠，激活预检必须失败并报告冲突，不能通过删除或改写人工目标解决冲突。

## 数据模型

### 路由目标归属

`RouteTarget` 新增：

| 字段 | 类型 | 语义 |
|---|---|---|
| `managed_by` | `varchar(32)` | `manual` 或 `config_import` |
| `source_batch_id` | 可空 `bigint` | 创建该候选目标的配置导入批次 ID |
| `retired_at` | 可空 `bigint` | 软退休时间；未退休为 `NULL` |

不新增 `source_entity_id`。策略中的 `group_name + model`、目标的稳定 `Name`（即 `route_target_ref`）和 `source_batch_id` 已能确定导入来源；重复保存同一个实体 ID 会增加迁移和维护成本。

新人工目标必须显式写入 `managed_by=manual`。新导入目标必须显式写入 `managed_by=config_import` 和 `source_batch_id`。结构体不使用数据库默认标签，旧表列添加和回填由跨数据库迁移函数完成。

### 批次激活状态

`ConfigImportBatch` 新增 `activated_at`。其语义是该批次首次成功完成激活事务的时间，不改变现有 `published` 状态机。

`ConfigImportBatch` 同时新增内部唯一 `deduplication_key` 和可空 `copied_from_batch_id`。历史批次迁移为 `deduplication_key=upload:<payload_sha256>`；`payload_sha256` 从唯一索引调整为普通索引。`deduplication_key` 不通过 API 返回，`copied_from_batch_id` 作为无凭据来源信息返回。

批次状态继续保持：

```text
ready -> publishing -> published
```

激活通过 `activated_at`、批次目标当前状态和激活审计表达，不新增 `activating` 或 `activated` 批次状态，避免破坏现有发布恢复逻辑。

### 激活审计

新增 `ConfigImportActivationAudit`，记录：

- 批次和管理员 ID。
- `activated`、`rejected`、`cache_refresh_pending` 或 `cache_refreshed` 结果。
- 启用渠道数、策略数、目标数和退休目标数。
- 激活前后配置哈希。
- 失败代码与安全摘要；不记录 API Key。
- 创建时间。

成功审计与激活数据在同一事务提交。预检拒绝和提交后缓存失败由独立短事务追加审计。

每一条激活审计都必须同时保存 `before_sha256` 和 `after_sha256`：`rejected` 使用发布后基线哈希与预检时当前哈希，`activated` 使用发布后基线哈希与事务写入后的活动配置哈希，`cache_refresh_pending` 和 `cache_refreshed` 复用对应 `activated` 审计的两端哈希。这样缓存恢复事件也能明确归属于同一次配置切换。

### 历史归属回填审计

新增 `ConfigImportRouteOwnershipChange`，逐目标记录一次历史归属操作：

- `operation_id`。
- 目标 ID。
- 修改前 `managed_by` 和 `source_batch_id`。
- 确定匹配的批次 ID。
- 应用后的目标 `updated_at`。
- 应用后目标完整业务字段的 SHA-256 指纹。
- 操作人、应用时间、回滚人和回滚时间。

该表使历史归属回填可预览、可审计、可幂等执行，并可在目标未被后续修改时精确回滚。

## 发布语义

发布阶段按策略键 `default + canonical_model` 聚合本批次所有未排除且未跳过的 `route_blueprints`，生成确定性的批次路由计划。成本规则继续保持暂存阶段创建的 `draft`；售价选项、渠道模型、模型映射和能力不在发布阶段修改。

对每个策略：

1. 策略不存在时创建禁用策略。
2. 策略已存在时保留当前 `enabled` 和默认参数，避免发布候选配置时改变活动路由行为。
3. 删除并重建的范围仅限 `source_batch_id=当前批次` 的候选目标，用于安全重试。
4. 插入的新目标固定 `enabled=false`、`retired_at=NULL`、`managed_by=config_import`。
5. 将对应 `ConfigImportItem.MaterializedType` 写为 `routing_policy`，`MaterializedID` 写为策略 ID。
6. 不退休旧导入目标，不删除人工目标，不启用渠道或策略。
7. 捕获包含当前批次禁用候选的发布后基线，并回写 `ConfigImportBatch.BaselineJSON`，供激活时再次执行乐观并发检查。

同一策略下多个蓝图必须使用同一个 `merge_mode`，且计算出的策略默认参数必须一致；不一致时暂存阶段产生阻断问题，不能任意选择其中一个。

### `merge` 和 `replace`

两种模式都在发布时保留旧目标，差异只在激活事务中生效：

- `merge`：只退休旧批次中与本批次相同 `route_target_ref` 的导入目标；其他旧导入目标继续保持原状态。
- `replace`：退休该策略下所有非当前批次的导入目标；人工目标仍保留。
- `skip`：不创建目标，也不参与激活。

## 通用路由保存的归属保护

路由策略管理接口继续允许管理员编辑策略，但必须保留导入目标的来源：

- 写请求携带可选目标 ID。
- 已有目标按 ID 更新，不再整表删除重建。
- 新增目标写为 `manual`。
- 请求中删除的人工目标可以物理删除。
- 请求中删除的导入目标只能软退休，不能物理删除。
- 更新已有导入目标时保留 `managed_by`、`source_batch_id` 和已有 `retired_at`；显式重新启用时清空 `retired_at`。

这保证配置导入审计不会被一次普通策略保存破坏。

## 激活预览与门禁

批次详情为已发布批次返回 `activation_preview`：

```json
{
  "ready": true,
  "target_count": 13,
  "retire_target_count": 67,
  "policy_count": 3,
  "channel_count": 8,
  "blockers": []
}
```

预览和实际激活必须调用同一个计划构造器。前端不得自行推导可激活性。

激活前至少验证：

1. 批次状态为 `published`，且尚未成功激活。
2. 当前活动配置仍等于发布后保存的基线；不一致时报告 `ACTIVATION_STALE_BASE_VERSION`。
3. 不存在未解决的 `error` 或 `warning`；`CACHE_REFRESH_PENDING` 必须先恢复。
4. 所有参与路由的渠道线已绑定、未跳过，且凭据已由管理员确认。
5. 每个计划目标都存在唯一的当前批次候选行，字段与蓝图一致。
6. 渠道存在，API Key 非空，状态为已启用或手动禁用；自动禁用渠道必须先人工排障。
7. 本批次模型映射能为每个目标建立对应标准模型能力。
8. 每个目标的配置导入成本草稿存在，且再次通过成本能力合同校验；预检不要求它已经是活动规则。
9. 通过 `RouteTargetContractValidator` 验证真实渠道类型和目标能力合同。
10. 将本次启用、旧导入目标退休和人工目标保留后的完整策略快照通过 `modelrouting.ValidatePolicy`。
11. 新目标不会与保留的人工目标或 `merge` 模式下保留的旧导入目标发生同优先级能力重叠。

任一门禁失败时返回结构化 blocker，激活不写任何运行时状态。

## 激活事务

新增：

```text
POST /api/config-import/batches/:id/activate
```

使用现有 `config_import.publish` 权限。服务在主数据库事务中：

1. 使用 `lockForUpdate` 锁定批次。
2. 锁定受影响成本规则、售价选项、策略、当前批次目标、待退休导入目标、渠道和能力行。
3. 在锁内重新构造并验证激活计划，防止预览后并发修改。
4. 应用本批次权威成本快照，激活草稿并退休被替代的活动成本版本。
5. 应用销售选项和渠道模型映射，按本批次快照重建渠道能力；此时手动禁用渠道的能力仍保持禁用。
6. 按 `merge` 或 `replace` 规则将旧导入目标设为 `enabled=false` 并写入同一个 `retired_at`。
7. 将当前批次目标设为 `enabled=true`、`retired_at=NULL`。
8. 更新策略默认参数并设为 `enabled=true`。
9. 将手动禁用的参与渠道设为 `ChannelStatusEnabled`，同步将其 `abilities.enabled=true`；自动禁用渠道不在事务中强制恢复。
10. 捕获激活后基线，写入 `ConfigImportActivationAudit(outcome=activated)` 和批次 `activated_at`。
11. 提交事务。

事务提交后按顺序刷新路由策略缓存、渠道缓存和代理客户端缓存。缓存刷新失败不回滚已提交激活，而是：

- 写入 `ACTIVATION_CACHE_REFRESH_PENDING` 问题。
- 追加 `cache_refresh_pending` 审计。
- 允许复用 `/refresh-cache` 恢复。
- 在恢复成功后解决问题并追加 `cache_refreshed` 审计。

## 模型快照与人工目标

`publishConfigImportModelMappings` 改为仅由激活事务调用，并且不再因为上游模型从绑定渠道快照中消失而禁用任意路由目标。路由退休统一由带来源判断的激活事务处理。

成本规则仍按现有权威快照逻辑退役，但该逻辑从发布移动到激活事务。激活预检必须确认本批次每个目标的成本草稿完整且可激活。如果草稿缺失或合同不通过，批次保持已发布但不可激活，管理员必须通过修正后的新导入批次解决，不能绕过门禁。

## 历史目标归属回填

旧数据库升级后，所有没有来源字段的目标先回填为 `manual`，不会自动猜测为导入目标。

提供三个管理员操作：

```text
GET  /api/config-import/route-ownership/backfill-preview
POST /api/config-import/route-ownership/backfill
POST /api/config-import/route-ownership/backfill/:operation_id/rollback
```

### 确定性匹配

对每个 `manual` 历史目标，根据已发布批次重建候选目标，并比较：

- 策略 `group_name` 和规范模型。
- 绑定后的渠道 ID。
- `route_target_ref`，即目标 `Name`。
- 上游模型。
- 规范化后的 `cost_variant_key`。
- 规范化能力约束。

若语义候选多于一个，再用目标 `created_at` 与批次 `published_at` 精确相等缩小范围。只有最终恰好一个候选时才允许回填。

结果分类：

- `matched`：可安全应用。
- `ambiguous`：多个候选，保持 `manual`。
- `unmatched`：没有候选，保持 `manual`。

预览不写数据库。应用只修改 `matched`，为每条修改写入同一个 `operation_id` 的变更审计。重复应用不会再次修改已归属目标。

回滚锁定目标后重新计算业务字段 SHA-256 指纹。只有当前归属仍等于该操作写入值、`updated_at` 等于审计保存值且指纹完全一致时才执行；目标已被激活、编辑或退休时拒绝回滚，避免同一秒内发生的后续修改绕过版本检查。

## 前端流程

配置导入向导新增“激活”步骤：

```text
上传 -> 渠道绑定 -> 冲突处理 -> 定价 -> 路由差异 -> 发布确认 -> 激活 -> 结果
```

发布成功后展示后端 `activation_preview`：

- 将启用的渠道、策略和目标数量。
- 将软退休的旧导入目标数量。
- 所有阻断项。
- 二次确认复选框和“激活导入”按钮。

存在 blocker、批次尚未发布、正在请求或未确认时按钮禁用。激活成功后展示最终计数和缓存状态，不再展示发布按钮；若数据库已激活但缓存刷新失败，前端重新加载批次进入结果页，并提供调用既有 `/refresh-cache` 的重试按钮。

所有新增文案通过 `useTranslation()`，并通过规定脚本同步 `en`、`zh`、`zh-TW`、`fr`、`ja`、`ru`、`vi` 七种语言。

已发布结果页提供“复制为新绑定批次”按钮。按钮只在后端 `allowed_actions` 包含 `copy_for_binding` 时可用；请求成功后向导切换到返回的新批次，不修改或覆盖源批次。

## 数据库兼容性

- 迁移使用 GORM Migrator 和已有 `quoteIdentifier`，兼容 SQLite、MySQL 5.7.8+、PostgreSQL 9.6+。
- 新列添加在 `AutoMigrate` 前完成，历史行显式回填为 `manual`。
- 不使用数据库原生 JSON 列；审计摘要使用 `TEXT`/`LONGTEXT` 和 `common.Marshal`。
- 锁查询使用 `lockForUpdate`；SQLite 自动跳过不支持的 `FOR UPDATE`。
- 批量状态更新使用 GORM `Where`/`Updates`，不引入方言专用 SQL。

## 安全与审计

- 导入文档、预览和审计均不包含 API Key。
- 激活只信任数据库中的绑定确认元数据，不信任前端布尔值。
- 权限复用 `ConfigImportPublish`，普通只读或写入管理员不能激活或执行历史回填。
- 错误响应只返回渠道 ID、线路引用、目标引用和错误代码，不回显 Key、完整上游响应或敏感渠道配置。

## 上线顺序

1. 上线数据模型和迁移，历史目标全部保持 `manual`，运行时行为不变。
2. 执行历史归属预览，保存 `matched`、`ambiguous`、`unmatched` 报告。
3. 人工核对当前 Mock 批次及目标数量后执行回填。
4. 再次运行预览，确认待应用数为 0；抽查来源批次和目标状态。
5. 上线新的发布与激活逻辑。
6. 创建新的真实渠道导入批次，绑定已录入 Key 的真实渠道。
7. 发布并检查激活预览。
8. 激活后执行真实 Ark SDK 视频链路验收。

## 回退策略

- 代码回退不会物理删除历史路由；已退休目标仍可审计。
- 历史归属回填使用 `operation_id` 回滚，且只允许回滚未被后续修改的目标。
- 激活事务失败时所有渠道、能力、策略和目标状态保持原样。
- 激活已成功但缓存失败时使用 `/refresh-cache` 恢复，不重复执行激活事务。
- 业务配置回退通过修正后的新导入批次再次发布和激活完成，不直接删除审计或批量改写历史目标。

## 验收标准

- 新导入目标能追溯到唯一批次，人工目标保持 `manual`。
- 发布不会让新导入目标接收流量，也不会提前退休旧导入目标。
- 激活在一个事务内完成旧导入目标软退休、新目标启用、策略启用、渠道与能力启用。
- `merge`、`replace`、`skip` 行为符合本设计，人工目标不会被自动删除或退休。
- 自动禁用渠道、缺少 Key、缺少成本草稿、合同不兼容、能力重叠均阻断激活。
- 缓存失败可恢复且有审计，不会重复写入激活状态。
- 历史回填支持 dry-run、歧义报告、幂等应用和受版本保护的回滚。
- SQLite、MySQL 和 PostgreSQL 迁移与核心事务测试通过。
- 前端能够完成发布后激活，并准确展示启用、退休和 blocker 数量。
- 相同 JSON 重复上传仍返回原批次；显式复制同一已发布批次可以连续创建不同的新绑定批次，且每个新批次均无绑定、无审阅状态、无物化结果并可追溯到源批次。
