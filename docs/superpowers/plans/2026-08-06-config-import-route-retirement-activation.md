# 配置导入路由归属、软退休与批次激活实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为配置导入建立可追溯的路由目标归属、可逆的历史归属回填、旧导入路由软退休和批次级原子激活，使后续维护只需更新收录表、导入、绑定、发布和激活。

**Architecture:** 发布阶段只写入带批次归属的禁用候选目标；激活阶段从同一后端计划执行预检，并在一个事务内退休被替代的旧导入目标、启用当前目标与策略、启用绑定渠道及能力。人工目标始终保留，历史目标通过确定性 dry-run 回填来源，所有激活和回填操作都有审计与缓存恢复路径。

**Tech Stack:** Go 1.22+、Gin、GORM v2、SQLite/MySQL/PostgreSQL、React 19、TypeScript、Zod、i18next、Bun、testify。

**Design:** `docs/superpowers/specs/2026-08-06-config-import-route-retirement-activation-design.md`

---

## 文件结构

- Create `types/routing_ownership.go`: 路由目标来源常量与规范化函数。
- Modify `model/routing_policy.go`: 路由目标来源字段和保留来源的增量保存。
- Modify `model/config_import.go`: 批次激活时间、激活审计和历史归属变更审计。
- Modify `model/config_import.go`: 批次实例去重键、复制来源和已发布批次复制语义。
- Modify `model/locking.go`: 为 service 层提供统一的跨数据库模型行锁入口。
- Modify `model/main.go`: 三种数据库兼容的来源列迁移和新审计表迁移。
- Modify `model/config_import_migration_test.go`: SQLite/MySQL/PostgreSQL 迁移契约。
- Modify `model/routing_policy_test.go`: 增量保存的来源保护和软退休契约。
- Create `service/config_import_route_ownership.go`: 历史归属预览、应用和回滚。
- Create `service/config_import_route_ownership_test.go`: 确定性匹配、歧义、幂等和回滚测试。
- Modify `service/config_import_publish.go`: 发布批次候选目标，不提前退休旧路由。
- Modify `service/config_import_publish_test.go`: 发布、来源保留和模型快照回归测试。
- Modify `service/config_import_stage.go`: 同策略蓝图一致性门禁和发布后基线支持。
- Modify `service/config_import_stage_test.go`: 蓝图一致性门禁回归测试。
- Create `service/config_import_activation.go`: 激活计划、预检、事务、缓存与审计。
- Create `service/config_import_activation_test.go`: 激活成功、阻断、回滚和缓存恢复测试。
- Modify `service/config_import.go`: 批次详情、允许操作和激活预览。
- Modify `service/config_import_schema.go`: 为结构化 blocker 和缓存待恢复错误携带安全数据。
- Modify `dto/config_import.go`: 激活预览、结果和历史归属报告契约。
- Modify `controller/config_import.go`: 激活与历史归属管理器。
- Create `controller/config_import_test.go`: 激活响应、结构化错误和幂等重试测试。
- Modify `router/config-import-router.go`: 激活、预览回填、应用和回滚路由。
- Create `router/config_import_router_test.go`: 权限和处理器注册契约。
- Modify `service/authz/resources_config_import.go`: 复用并说明 publish 权限覆盖激活与回填。
- Modify `service/routing_policy.go`: 普通策略写请求携带目标 ID 并保留导入来源。
- Modify `service/routing_policy_test.go`: 导入目标软删除和来源保留测试。
- Modify `web/src/features/model-routing/types.ts`: 普通路由编辑保存目标 ID。
- Modify `web/src/features/model-routing/components/route-target-editor-client.test.tsx`: 目标 ID 写请求回归测试。
- Modify `web/src/features/config-import/types.ts`: 激活预览和响应 Zod 契约。
- Modify `web/src/features/config-import/api.ts`: 激活 API。
- Modify `web/src/features/config-import/lib/batch-state.ts`: 新增激活步骤状态推导。
- Modify `web/src/features/config-import/lib/__tests__/batch-state.test.ts`: 激活步骤状态测试。
- Modify `web/src/features/config-import/index.tsx`: 串联发布后激活。
- Modify `web/src/features/config-import/components/config-import-stepper.tsx`: 注册激活步骤标签。
- Modify `web/src/features/config-import/components/publish-result-step.tsx`: 展示激活结果和缓存恢复状态。
- Create `web/src/features/config-import/components/activation-step.tsx`: 激活预览、blocker 和确认控件。
- Create `web/src/features/config-import/components/__tests__/activation-step.test.tsx`: 激活组件行为测试。
- Modify `web/src/features/config-import/components/__tests__/config-import-wizard.test.tsx`: 发布到激活到结果的流程测试。
- Modify through script `web/src/i18n/locales/{en,zh,zh-TW,fr,ja,ru,vi}.json`: 新增激活文案。
- Create then delete `web/scripts/add-missing-keys.mjs`: 按项目规则原子写入七种语言。

### Task 1: 添加路由归属、激活和回填审计模型

**Files:**
- Create: `types/routing_ownership.go`
- Modify: `model/routing_policy.go`
- Modify: `model/config_import.go`
- Modify: `model/locking.go`
- Modify: `model/main.go`
- Modify: `model/config_import_migration_test.go`

- [ ] **Step 1: 编写失败的迁移测试**

在 `model/config_import_migration_test.go` 增加 SQLite 和已配置数据库用例，先创建没有来源列的旧 `route_targets` 表和一条启用目标，再运行迁移并断言：

```go
func TestRouteTargetOwnershipMigrationBackfillsManualWithoutChangingRuntimeState(t *testing.T) {
	prepareConfigImportDB(t)
	require.NoError(t, DB.Migrator().DropTable(&RouteTarget{}))
	require.NoError(t, DB.Exec(`CREATE TABLE route_targets (
		id integer primary key,
		policy_id integer not null,
		channel_id integer not null,
		name varchar(128) not null,
		upstream_model varchar(255) not null,
		cost_variant_key varchar(64) not null,
		target_priority integer not null,
		constraints text not null,
		enabled numeric not null,
		created_at bigint,
		updated_at bigint
	)`).Error)
	require.NoError(t, DB.Exec(`INSERT INTO route_targets
		(id, policy_id, channel_id, name, upstream_model, cost_variant_key, target_priority, constraints, enabled, created_at, updated_at)
		VALUES (1, 2, 3, 'legacy', 'upstream', 'default', 10, '{}', 1, 11, 12)`).Error)

	require.NoError(t, migrateRouteTargetOwnershipColumns())
	require.NoError(t, DB.AutoMigrate(&RouteTarget{}, &ConfigImportActivationAudit{}, &ConfigImportRouteOwnershipChange{}))

	var target RouteTarget
	require.NoError(t, DB.First(&target, 1).Error)
	assert.Equal(t, string(types.RouteTargetManagedByManual), target.ManagedBy)
	assert.Nil(t, target.SourceBatchID)
	assert.Nil(t, target.RetiredAt)
	assert.True(t, target.Enabled)
}
```

将 `TestConfigImportMigrationUsesTextForCanonicalJSON` 中配置导入表数量从 `6` 更新为 `8`：

```go
require.Len(t, rows, 8)
```

扩展 `testConfigImportMigrationContracts` 的 `AutoMigrate` 和清理列表，加入 `ConfigImportActivationAudit`、`ConfigImportRouteOwnershipChange`；验证新表可写入文本摘要、每条激活审计的 `before_sha256`/`after_sha256` 均非空、同一 `operation_id + route_target_id` 唯一，并在第二次迁移时不重复执行 DDL。

- [ ] **Step 2: 运行测试并确认按预期失败**

Run: `go test ./model -run 'TestRouteTargetOwnershipMigration|TestConfigImportMigrationConfiguredDatabases' -count=1`

Expected: FAIL，缺少来源字段、迁移函数和审计模型。

- [ ] **Step 3: 添加来源常量和模型字段**

创建 `types/routing_ownership.go`：

```go
package types

import (
	"fmt"
	"strings"
)

type RouteTargetManagedBy string

const (
	RouteTargetManagedByManual       RouteTargetManagedBy = "manual"
	RouteTargetManagedByConfigImport RouteTargetManagedBy = "config_import"
)

func NormalizeRouteTargetManagedBy(value string) (RouteTargetManagedBy, error) {
	normalized := RouteTargetManagedBy(strings.ToLower(strings.TrimSpace(value)))
	if normalized == "" {
		return RouteTargetManagedByManual, nil
	}
	switch normalized {
	case RouteTargetManagedByManual, RouteTargetManagedByConfigImport:
		return normalized, nil
	default:
		return "", fmt.Errorf("invalid route target owner %q", value)
	}
}
```

在 `model/locking.go` 增加 service 层共用入口；调用方只能通过它或已有 `LockChannelsForUpdate` 使用标准行锁，不能直接拼装 `clause.Locking`：

```go
// LockModelForUpdate applies the shared cross-database row-lock policy to a
// model query owned by a surrounding transaction.
func LockModelForUpdate(tx *gorm.DB, value any) *gorm.DB {
	return lockForUpdate(tx).Model(value)
}
```

在 `model/routing_policy.go` 的 `RouteTarget` 增加：

```go
ManagedBy     string `json:"managed_by" gorm:"type:varchar(32);not null;index"`
SourceBatchID *int64 `json:"source_batch_id,omitempty" gorm:"index"`
RetiredAt     *int64 `json:"retired_at,omitempty" gorm:"index"`
```

在 `model/config_import.go` 增加 `ConfigImportBatch.ActivatedAt`，并定义：

```go
type ConfigImportActivationAudit struct {
	ID                  int64                   `json:"id" gorm:"primaryKey"`
	BatchID             int64                   `json:"batch_id" gorm:"not null;index"`
	AdminID             int                     `json:"admin_id" gorm:"index"`
	Outcome             string                  `json:"outcome" gorm:"type:varchar(64);index"`
	ChannelCount        int                     `json:"channel_count"`
	PolicyCount         int                     `json:"policy_count"`
	TargetCount         int                     `json:"target_count"`
	RetiredTargetCount  int                     `json:"retired_target_count"`
	BeforeSHA256        string                  `json:"before_sha256" gorm:"type:varchar(64);not null"`
	AfterSHA256         string                  `json:"after_sha256" gorm:"type:varchar(64);not null"`
	FailureCode         string                  `json:"failure_code" gorm:"type:varchar(64)"`
	FailureMessage      string                  `json:"failure_message" gorm:"type:text"`
	SummaryJSON         ConfigImportSummaryJSON `json:"summary_json"`
	CreatedAt           int64                   `json:"created_at" gorm:"autoCreateTime"`
}

type ConfigImportRouteOwnershipChange struct {
	ID                    int64  `json:"id" gorm:"primaryKey"`
	OperationID           string `json:"operation_id" gorm:"type:varchar(64);not null;uniqueIndex:idx_route_ownership_change,priority:1;index"`
	RouteTargetID         int    `json:"route_target_id" gorm:"not null;uniqueIndex:idx_route_ownership_change,priority:2;index"`
	PreviousManagedBy     string `json:"previous_managed_by" gorm:"type:varchar(32);not null"`
	PreviousSourceBatchID *int64 `json:"previous_source_batch_id,omitempty"`
	AssignedBatchID       int64  `json:"assigned_batch_id" gorm:"not null;index"`
	AppliedTargetUpdatedAt int64 `json:"applied_target_updated_at"`
	AppliedTargetSHA256    string `json:"applied_target_sha256" gorm:"type:varchar(64);not null"`
	AppliedBy             int    `json:"applied_by" gorm:"index"`
	RevertedBy            int    `json:"reverted_by" gorm:"index"`
	RevertedAt            *int64 `json:"reverted_at,omitempty"`
	CreatedAt             int64  `json:"created_at" gorm:"autoCreateTime"`
}
```

为两个新模型定义固定表名 `config_import_activation_audits` 和 `config_import_route_ownership_changes`。

- [ ] **Step 4: 实现三数据库兼容迁移**

在 `migrateDB` 和 `migrateDBFast` 的 `AutoMigrate` 前调用 `migrateRouteTargetOwnershipColumns()`。函数必须幂等并使用 `quoteIdentifier`：

```go
func migrateRouteTargetOwnershipColumns() error {
	if DB == nil || !DB.Migrator().HasTable(&RouteTarget{}) {
		return nil
	}
	columns := []struct {
		Name string
		DDL  string
	}{
		{Name: "managed_by", DDL: "VARCHAR(32) NOT NULL DEFAULT 'manual'"},
		{Name: "source_batch_id", DDL: "BIGINT NULL"},
		{Name: "retired_at", DDL: "BIGINT NULL"},
	}
	for _, column := range columns {
		if DB.Migrator().HasColumn(&RouteTarget{}, column.Name) {
			continue
		}
		statement := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s",
			quoteIdentifier("route_targets"), quoteIdentifier(column.Name), column.DDL)
		if err := DB.Exec(statement).Error; err != nil {
			return fmt.Errorf("add route_targets.%s: %w", column.Name, err)
		}
	}
	return DB.Model(&RouteTarget{}).
		Where("managed_by IS NULL OR managed_by = ?", "").
		Update("managed_by", string(types.RouteTargetManagedByManual)).Error
}
```

将 `ConfigImportActivationAudit` 和 `ConfigImportRouteOwnershipChange` 加入普通和快速迁移列表。模型结构不添加 `gorm:"default:..."`。

- [ ] **Step 5: 运行迁移测试并确认通过**

Run: `go test ./model -run 'TestRouteTargetOwnershipMigration|TestConfigImportMigration' -count=1`

Expected: PASS；未设置 `TEST_MYSQL_DSN`/`TEST_POSTGRES_DSN` 时对应子测试明确 SKIP。

- [ ] **Step 6: 提交模型和迁移**

```powershell
git add types/routing_ownership.go model/routing_policy.go model/config_import.go model/locking.go model/main.go model/config_import_migration_test.go
git commit -m "feat: add config import route ownership models"
```

### Task 2: 让普通路由策略保存保留导入来源和历史目标

**Files:**
- Modify: `service/routing_policy.go`
- Modify: `model/routing_policy.go`
- Modify: `service/routing_policy_test.go`
- Modify: `model/routing_policy_test.go`
- Modify: `web/src/features/model-routing/types.ts`
- Test: `web/src/features/model-routing/components/route-target-editor-client.test.tsx`

- [ ] **Step 1: 编写失败的后端行为测试**

在 `service/routing_policy_test.go` 增加两个用例：

```go
func TestSaveRoutingPolicyPreservesImportedTargetOwnershipByID(t *testing.T) {
	prepareRoutingPolicyServiceTest(t)
	seedRoutingCandidate(t, 11, "A1", "分组A", modelrouting.Seedance20, true)
	created, err := service.SaveRoutingPolicy(0, validRoutingPolicyWriteRequest())
	require.NoError(t, err)
	require.Len(t, created.Targets, 1)
	batchID := int64(20)
	require.NoError(t, model.DB.Model(&model.RouteTarget{}).
		Where("id = ?", created.Targets[0].ID).
		Updates(map[string]any{
			"managed_by": string(types.RouteTargetManagedByConfigImport),
			"source_batch_id": batchID,
		}).Error)

	view, err := service.GetRoutingPolicyView(created.ID)
	require.NoError(t, err)
	request := routingPolicyWriteRequestForTest(view)
	request.Targets[0].TargetPriority = 101
	updated, err := service.SaveRoutingPolicy(created.ID, request)
	require.NoError(t, err)

	var target model.RouteTarget
	require.NoError(t, model.DB.First(&target, updated.Targets[0].ID).Error)
	assert.Equal(t, string(types.RouteTargetManagedByConfigImport), target.ManagedBy)
	require.NotNil(t, target.SourceBatchID)
	assert.Equal(t, batchID, *target.SourceBatchID)
}

func TestSaveRoutingPolicySoftRetiresOmittedImportedTargetAndDeletesOmittedManualTarget(t *testing.T) {
	prepareRoutingPolicyServiceTest(t)
	seedRoutingCandidate(t, 11, "A1", "分组A", modelrouting.Seedance20, true)
	request := validRoutingPolicyWriteRequest()
	manual := request.Targets[0]
	manual.Name = "manual"
	manual.TargetPriority = 90
	keeper := request.Targets[0]
	keeper.Name = "keeper"
	keeper.TargetPriority = 80
	request.Targets = []service.RouteTargetWriteRequest{request.Targets[0], manual, keeper}
	created, err := service.SaveRoutingPolicy(0, request)
	require.NoError(t, err)
	require.Len(t, created.Targets, 3)
	batchID := int64(20)
	require.NoError(t, model.DB.Model(&model.RouteTarget{}).
		Where("id = ?", created.Targets[0].ID).
		Updates(map[string]any{
			"managed_by": string(types.RouteTargetManagedByConfigImport),
			"source_batch_id": batchID,
		}).Error)

	view, err := service.GetRoutingPolicyView(created.ID)
	require.NoError(t, err)
	write := routingPolicyWriteRequestForTest(view)
	write.Targets = write.Targets[2:]
	_, err = service.SaveRoutingPolicy(created.ID, write)
	require.NoError(t, err)

	var imported model.RouteTarget
	require.NoError(t, model.DB.First(&imported, created.Targets[0].ID).Error)
	assert.False(t, imported.Enabled)
	assert.NotNil(t, imported.RetiredAt)
	var manualCount int64
	require.NoError(t, model.DB.Model(&model.RouteTarget{}).
		Where("id = ?", created.Targets[1].ID).Count(&manualCount).Error)
	assert.Zero(t, manualCount)
	var keeperCount int64
	require.NoError(t, model.DB.Model(&model.RouteTarget{}).
		Where("id = ?", created.Targets[2].ID).Count(&keeperCount).Error)
	assert.Equal(t, int64(1), keeperCount)
}
```

- [ ] **Step 2: 运行后端测试并确认失败**

Run: `go test ./service ./model -run 'TestSaveRoutingPolicy.*Imported|TestReplaceRoutingPolicy.*Ownership' -count=1`

Expected: FAIL，写请求不携带 ID，模型仍删除全部目标。

- [ ] **Step 3: 扩展写请求和视图**

在 `service/routing_policy.go` 修改请求结构：

```go
type RouteTargetWriteRequest struct {
	ID                       *int                     `json:"id,omitempty"`
	ChannelID                int                      `json:"channel_id"`
	Name                     string                   `json:"name"`
	UpstreamModel            string                   `json:"upstream_model"`
	CostVariantKey           string                   `json:"cost_variant_key"`
	TargetPriority           int                      `json:"target_priority"`
	MinimumExpectedMarginBPS *int                     `json:"minimum_expected_margin_bps"`
	Enabled                  bool                     `json:"enabled"`
	Constraints              modelrouting.Constraints `json:"constraints"`
}
```

`routingPolicyWriteRequestFromView` 把 `RouteTargetView.ID` 转为指针。`SaveRoutingPolicy` 构造持久化行时保留正 ID；客户端提供的 ID 必须属于当前策略，否则返回 `invalid_target_id`。

- [ ] **Step 4: 将完整删除重建改为增量保存**

重写 `ReplaceRoutingPolicyWithTx` 的目标持久化部分：

```go
existingByID := make(map[int]RouteTarget)
var existingTargets []RouteTarget
if policy.ID > 0 {
	if err := tx.Where("policy_id = ?", policy.ID).Find(&existingTargets).Error; err != nil {
		return nil, err
	}
	for _, target := range existingTargets {
		existingByID[target.ID] = target
	}
}

seen := make(map[int]struct{})
for index := range persistedTargets {
	target := &persistedTargets[index]
	target.PolicyID = policy.ID
	if target.ID == 0 {
		target.ManagedBy = string(types.RouteTargetManagedByManual)
		target.SourceBatchID = nil
		target.RetiredAt = nil
		if err := tx.Create(target).Error; err != nil {
			return nil, err
		}
		continue
	}
	existing, ok := existingByID[target.ID]
	if !ok {
		return nil, fmt.Errorf("route target %d does not belong to policy %d", target.ID, policy.ID)
	}
	seen[target.ID] = struct{}{}
	target.ManagedBy = existing.ManagedBy
	target.SourceBatchID = existing.SourceBatchID
	if target.Enabled {
		target.RetiredAt = nil
	} else {
		target.RetiredAt = existing.RetiredAt
	}
	if err := tx.Model(&RouteTarget{}).Where("id = ? AND policy_id = ?", target.ID, policy.ID).
		Select("channel_id", "name", "upstream_model", "cost_variant_key", "target_priority", "minimum_expected_margin_bps", "constraints", "enabled", "retired_at", "updated_at").
		Updates(target).Error; err != nil {
		return nil, err
	}
}

now := common.GetTimestamp()
for _, existing := range existingTargets {
	if _, ok := seen[existing.ID]; ok {
		continue
	}
	if existing.ManagedBy == string(types.RouteTargetManagedByConfigImport) {
		if err := tx.Model(&RouteTarget{}).Where("id = ?", existing.ID).Updates(map[string]any{
			"enabled": false, "retired_at": now, "updated_at": now,
		}).Error; err != nil {
			return nil, err
		}
		continue
	}
	if err := tx.Delete(&RouteTarget{}, existing.ID).Error; err != nil {
		return nil, err
	}
}
```

验证快照中的临时负 ID 只用于校验，不进入持久化。更新后重新查询策略返回真实目标 ID。

- [ ] **Step 5: 让前端写请求保留目标 ID**

在 `web/src/features/model-routing/types.ts` 将写请求 schema 改为仅删除 `channel_name`：

```ts
export const routeTargetWriteRequestSchema = routeTargetSchema
  .omit({ channel_name: true })
  .extend({ id: z.number().int().positive().optional() })
```

`toWriteRequest` 增加 `id: target.id`。在 `route-target-editor-client.test.tsx` 断言编辑已有目标时请求包含 ID，复制目标仍不包含 ID。

- [ ] **Step 6: 运行后端和前端测试**

Run: `go test ./model ./service -run 'RoutingPolicy|RouteTarget' -count=1`

Run: `cd web; bun test --parallel=1 src/features/model-routing/components/route-target-editor-client.test.tsx`

Expected: 全部 PASS，导入来源和历史目标不因普通保存丢失。

- [ ] **Step 7: 提交来源保护**

```powershell
git add service/routing_policy.go model/routing_policy.go service/routing_policy_test.go model/routing_policy_test.go web/src/features/model-routing/types.ts web/src/features/model-routing/components/route-target-editor-client.test.tsx
git commit -m "fix: preserve imported route target history"
```

### Task 3: 实现历史路由归属 dry-run、应用和安全回滚

**Files:**
- Create: `service/config_import_route_ownership.go`
- Create: `service/config_import_route_ownership_test.go`
- Modify: `dto/config_import.go`
- Modify: `controller/config_import.go`
- Modify: `router/config-import-router.go`
- Create: `router/config_import_router_test.go`

- [ ] **Step 1: 编写失败的确定性匹配测试**

在 `service/config_import_route_ownership_test.go` 创建三个已发布批次候选和四个历史目标，覆盖：唯一语义匹配、多个语义候选但 `created_at == published_at` 唯一、仍然歧义、完全不匹配。

```go
func TestPreviewConfigImportRouteOwnershipBackfillClassifiesWithoutWrites(t *testing.T) {
	prepareConfigImportServiceDB(t)
	fixture := seedRouteOwnershipBackfillFixture(t)

	report, err := PreviewConfigImportRouteOwnershipBackfill(context.Background())
	require.NoError(t, err)
	require.Len(t, report.Matched, 2)
	assert.Equal(t, fixture.UniqueTargetID, report.Matched[0].RouteTargetID)
	assert.Equal(t, fixture.TimestampTargetID, report.Matched[1].RouteTargetID)
	require.Len(t, report.Ambiguous, 1)
	assert.Equal(t, fixture.AmbiguousTargetID, report.Ambiguous[0].RouteTargetID)
	require.Len(t, report.Unmatched, 1)
	assert.Equal(t, fixture.UnmatchedTargetID, report.Unmatched[0].RouteTargetID)

	var changed int64
	require.NoError(t, model.DB.Model(&model.RouteTarget{}).
		Where("managed_by = ?", types.RouteTargetManagedByConfigImport).Count(&changed).Error)
	assert.Zero(t, changed)
}
```

同一测试文件必须完整定义 `seedRouteOwnershipBackfillFixture`。固定数据矩阵如下，不能依赖随机数或当前时间：

| 批次 | `published_at` | 候选 `route_target_ref` |
|---|---:|---|
| batch-1 | 100 | `unique-target`、`timestamp-target`、`ambiguous-target` |
| batch-2 | 200 | `timestamp-target`、`ambiguous-target` |
| batch-3 | 300 | `ambiguous-target` |

四个历史目标按顺序创建：`unique-target(created_at=900)`、`timestamp-target(created_at=200)`、`ambiguous-target(created_at=900)`、`unmatched-target(created_at=900)`。所有候选使用同一策略、渠道、上游模型、成本变体、优先级和规范化约束；因此结果必须固定为唯一匹配、时间戳消歧、仍然歧义和未匹配各一类。夹具定义为：

```go
type routeOwnershipBackfillFixture struct {
	UniqueTargetID    int
	TimestampTargetID int
	AmbiguousTargetID int
	UnmatchedTargetID int
}

func seedRouteOwnershipBackfillFixture(t *testing.T) routeOwnershipBackfillFixture {
	t.Helper()
	const channelID = 41
	const canonicalModel = "canonical-video"
	const upstreamModel = "upstream-video"
	priority := 100
	enabled := false

	policy := model.RoutingPolicy{
		GroupName: "default", Model: canonicalModel, Enabled: true,
		DefaultResolution: "720p", DefaultDuration: 4, DefaultRatio: "adaptive",
	}
	require.NoError(t, model.DB.Create(&policy).Error)

	batches := []struct {
		name        string
		publishedAt int64
		targetRefs  []string
	}{
		{name: "batch-1", publishedAt: 100, targetRefs: []string{"unique-target", "timestamp-target", "ambiguous-target"}},
		{name: "batch-2", publishedAt: 200, targetRefs: []string{"timestamp-target", "ambiguous-target"}},
		{name: "batch-3", publishedAt: 300, targetRefs: []string{"ambiguous-target"}},
	}
	for index, input := range batches {
		publishedAt := input.publishedAt
		batch := model.ConfigImportBatch{
			SchemaVersion: 1, TemplateVersion: "1",
			SourceSHA256: fmt.Sprintf("source-%d", index), PayloadSHA256: fmt.Sprintf("payload-%d", index),
			Status: string(types.ConfigImportBatchStatusPublished), CreatedBy: 42, PublishedAt: &publishedAt,
		}
		require.NoError(t, model.DB.Create(&batch).Error)
		lineRef := fmt.Sprintf("line-%d", index)
		boundChannelID := channelID
		confirmedAt := int64(90 + index)
		require.NoError(t, model.DB.Create(&model.ConfigImportBinding{
			BatchID: batch.ID, LineRef: lineRef, Action: string(types.ConfigImportBindingActionBind), ChannelID: &boundChannelID,
			CredentialsConfirmedBy: 42, CredentialsConfirmedAt: &confirmedAt,
		}).Error)
		targets := make([]types.ConfigImportRouteTarget, 0, len(input.targetRefs))
		for _, targetRef := range input.targetRefs {
			targets = append(targets, types.ConfigImportRouteTarget{
				RouteTargetRef: targetRef, LineRef: lineRef, UpstreamModel: upstreamModel,
				SKURef: "sku-video", CostVariantKey: "default", Priority: &priority, Enabled: &enabled,
			})
		}
		blueprint := types.ConfigImportRouteBlueprint{
			ConfigImportAuthoritativeEntity: types.ConfigImportAuthoritativeEntity{BusinessID: fmt.Sprintf("route-%d", index)},
			CanonicalModel: canonicalModel, ClientModel: canonicalModel,
			MergeMode: types.ConfigImportRouteMergeModeMerge, Targets: targets,
		}
		encoded, err := common.Marshal(blueprint)
		require.NoError(t, err)
		require.NoError(t, model.DB.Create(&model.ConfigImportItem{
			BatchID: batch.ID, EntityType: "route_blueprints", BusinessID: blueprint.BusinessID,
			EntityHash: fmt.Sprintf("hash-%d", index), CanonicalJSON: string(encoded), State: string(types.ConfigImportItemStateNew),
		}).Error)
	}

	blueprint := types.ConfigImportRouteBlueprint{
		CanonicalModel: canonicalModel, ClientModel: canonicalModel, MergeMode: types.ConfigImportRouteMergeModeMerge,
		Targets: []types.ConfigImportRouteTarget{{
			RouteTargetRef: "unique-target", LineRef: "fixture-line", UpstreamModel: upstreamModel,
			SKURef: "sku-video", CostVariantKey: "default", Priority: &priority, Enabled: &enabled,
		}},
	}
	_, rows, err := configImportRouteRows(map[string]int{"fixture-line": channelID}, blueprint)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	template := rows[0]

	created := make([]model.RouteTarget, 0, 4)
	for _, input := range []struct {
		name      string
		createdAt int64
	}{
		{name: "unique-target", createdAt: 900},
		{name: "timestamp-target", createdAt: 200},
		{name: "ambiguous-target", createdAt: 900},
		{name: "unmatched-target", createdAt: 900},
	} {
		target := template
		target.ID = 0
		target.PolicyID = policy.ID
		target.Name = input.name
		target.Enabled = true
		target.ManagedBy = string(types.RouteTargetManagedByManual)
		target.CreatedAt = input.createdAt
		target.UpdatedAt = input.createdAt
		require.NoError(t, model.DB.Create(&target).Error)
		created = append(created, target)
	}
	return routeOwnershipBackfillFixture{
		UniqueTargetID: created[0].ID, TimestampTargetID: created[1].ID,
		AmbiguousTargetID: created[2].ID, UnmatchedTargetID: created[3].ID,
	}
}
```

再添加应用幂等测试和回滚测试；回滚前人工修改目标 `updated_at` 时必须返回 `ROUTE_OWNERSHIP_ROLLBACK_CONFLICT` 且不覆盖修改。

- [ ] **Step 2: 运行测试并确认失败**

Run: `go test ./service -run 'RouteOwnershipBackfill' -count=1`

Expected: FAIL，预览、审计和回滚服务尚不存在。

- [ ] **Step 3: 定义报告 DTO**

在 `dto/config_import.go` 添加：

```go
type ConfigImportRouteOwnershipCandidate struct {
	RouteTargetID int     `json:"route_target_id"`
	PolicyID      int     `json:"policy_id"`
	RouteTargetRef string  `json:"route_target_ref"`
	ChannelID     int     `json:"channel_id"`
	BatchID       *int64  `json:"batch_id,omitempty"`
	CandidateBatchIDs []int64 `json:"candidate_batch_ids,omitempty"`
	Reason         string `json:"reason"`
}

type ConfigImportRouteOwnershipReport struct {
	OperationID string                                `json:"operation_id,omitempty"`
	Matched     []ConfigImportRouteOwnershipCandidate `json:"matched"`
	Ambiguous   []ConfigImportRouteOwnershipCandidate `json:"ambiguous"`
	Unmatched   []ConfigImportRouteOwnershipCandidate `json:"unmatched"`
	AppliedCount int                                  `json:"applied_count"`
	RevertedCount int                                 `json:"reverted_count"`
}
```

所有切片初始化为空数组并按目标 ID 排序。

- [ ] **Step 4: 实现候选重建和匹配**

创建 `service/config_import_route_ownership.go`，复用配置导入绑定、规范模型、成本变体和约束构造逻辑。定义稳定键：

```go
type configImportRouteOwnershipKey struct {
	GroupName      string
	CanonicalModel string
	ChannelID      int
	RouteTargetRef string
	UpstreamModel  string
	CostVariantKey string
	Constraints    string
}
```

约束必须先 `common.UnmarshalJsonStr` 到 `modelrouting.Constraints`，再 `common.Marshal` 规范化；不能直接比较原始 JSON 字符串。匹配顺序固定为：语义键唯一；否则筛选 `batch.PublishedAt != nil && *batch.PublishedAt == target.CreatedAt`；最终不是唯一即报告歧义。

- [ ] **Step 5: 实现应用和回滚事务**

实现：

```go
func ApplyConfigImportRouteOwnershipBackfill(ctx context.Context, adminID int) (*dto.ConfigImportRouteOwnershipReport, error)
func RollbackConfigImportRouteOwnershipBackfill(ctx context.Context, adminID int, operationID string) (*dto.ConfigImportRouteOwnershipReport, error)
```

应用事务生成 `operationID := common.GetUUID()`，仅锁定并更新当前仍为 `manual` 的 matched 目标。每条更新后重新读取完整目标，通过 `common.Marshal` 和 `sha256.Sum256` 计算稳定业务指纹，并将 `updated_at` 与指纹写入 `ConfigImportRouteOwnershipChange`。指纹只包含稳定业务字段，排除独立比较的 `UpdatedAt` 和无关的 `CreatedAt`：

```go
func configImportRouteTargetOwnershipFingerprint(target model.RouteTarget) (string, error) {
	payload := struct {
		ID                       int    `json:"id"`
		PolicyID                 int    `json:"policy_id"`
		ChannelID                int    `json:"channel_id"`
		Name                     string `json:"name"`
		UpstreamModel            string `json:"upstream_model"`
		CostVariantKey           string `json:"cost_variant_key"`
		TargetPriority           int    `json:"target_priority"`
		MinimumExpectedMarginBPS *int   `json:"minimum_expected_margin_bps"`
		Constraints              string `json:"constraints"`
		Enabled                  bool   `json:"enabled"`
		ManagedBy                string `json:"managed_by"`
		SourceBatchID            *int64 `json:"source_batch_id"`
		RetiredAt                *int64 `json:"retired_at"`
	}{
		target.ID, target.PolicyID, target.ChannelID, target.Name, target.UpstreamModel,
		target.CostVariantKey, target.TargetPriority, target.MinimumExpectedMarginBPS,
		target.Constraints, target.Enabled, target.ManagedBy, target.SourceBatchID, target.RetiredAt,
	}
	encoded, err := common.Marshal(payload)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}
```

回滚使用以下检查，行锁通过 model 层共享入口获取：

```go
var current model.RouteTarget
if err := model.LockModelForUpdate(tx, &model.RouteTarget{}).
	Where("id = ?", change.RouteTargetID).First(&current).Error; err != nil {
	return err
}
fingerprint, err := configImportRouteTargetOwnershipFingerprint(current)
if err != nil {
	return err
}
if current.ManagedBy != string(types.RouteTargetManagedByConfigImport) ||
	current.SourceBatchID == nil || *current.SourceBatchID != change.AssignedBatchID ||
	current.UpdatedAt != change.AppliedTargetUpdatedAt ||
	fingerprint != change.AppliedTargetSHA256 {
	return configImportError("ROUTE_OWNERSHIP_ROLLBACK_CONFLICT", "route target %d changed after operation %s", change.RouteTargetID, operationID)
}
if err := tx.Model(&model.RouteTarget{}).Where("id = ?", change.RouteTargetID).Updates(map[string]any{
	"managed_by": change.PreviousManagedBy,
	"source_batch_id": change.PreviousSourceBatchID,
	"updated_at": common.GetTimestamp(),
}).Error; err != nil {
	return err
}
```

全部目标成功后写 `reverted_by` 和 `reverted_at`。空操作和重复回滚返回结构化错误，不部分提交。测试必须分别覆盖：修改 `updated_at` 后拒绝；保持 `updated_at` 等于 `AppliedTargetUpdatedAt` 但用 `UpdateColumns` 修改 `enabled` 后仍因 `AppliedTargetSHA256` 不同而拒绝。两种冲突都断言整次事务零部分回滚。

- [ ] **Step 6: 注册管理员 API 和权限测试**

在 `controller/config_import.go` 添加三个处理器，在 `router/config-import-router.go` 注册：

```go
{method: http.MethodGet, path: "/route-ownership/backfill-preview", permission: authz.ConfigImportPublish, handler: controller.PreviewConfigImportRouteOwnershipBackfill},
{method: http.MethodPost, path: "/route-ownership/backfill", permission: authz.ConfigImportPublish, handler: controller.ApplyConfigImportRouteOwnershipBackfill},
{method: http.MethodPost, path: "/route-ownership/backfill/:operation_id/rollback", permission: authz.ConfigImportPublish, handler: controller.RollbackConfigImportRouteOwnershipBackfill},
```

`router/config_import_router_test.go` 使用与 `routing_policy_router_test.go` 相同的反射断言，验证三个路由均要求 `ConfigImportPublish`。

- [ ] **Step 7: 运行服务和路由测试**

Run: `go test ./service ./controller ./router -run 'RouteOwnership|ConfigImportRoute' -count=1`

Expected: PASS；预览无写入，应用幂等，歧义不改写，回滚受版本保护。

- [ ] **Step 8: 提交历史归属工具**

```powershell
git add service/config_import_route_ownership.go service/config_import_route_ownership_test.go dto/config_import.go controller/config_import.go router/config-import-router.go router/config_import_router_test.go
git commit -m "feat: add route ownership backfill workflow"
```

### Task 4: 将配置发布改为只封存候选配置

**Files:**
- Modify: `service/config_import_publish.go`
- Modify: `service/config_import_stage.go`
- Modify: `service/config_import_publish_test.go`
- Modify: `service/config_import_stage_test.go`

- [ ] **Step 1: 编写失败的发布语义测试**

在 `service/config_import_publish_test.go` 添加集成用例，先建立：一个活动成本规则、现有售价选项、现有渠道模型映射、一个启用人工目标和一个批次 19 启用导入目标，再发布批次 21。断言发布只写入当前批次候选：

```go
assert.Equal(t, string(types.CostRuleActive), previousCostRule.Status)
assert.Equal(t, string(types.CostRuleDraft), currentCostDraft.Status)
assert.Equal(t, previousOptionValue, loadedOption.Value)
assert.Equal(t, previousModels, loadedChannel.Models)
assert.Equal(t, previousMapping, loadedChannel.GetModelMapping())
assert.True(t, manual.Enabled)
assert.Equal(t, string(types.RouteTargetManagedByManual), manual.ManagedBy)
assert.True(t, previousImported.Enabled)
assert.Nil(t, previousImported.RetiredAt)
assert.False(t, currentCandidate.Enabled)
assert.Equal(t, string(types.RouteTargetManagedByConfigImport), currentCandidate.ManagedBy)
require.NotNil(t, currentCandidate.SourceBatchID)
assert.Equal(t, batch.ID, *currentCandidate.SourceBatchID)
assert.True(t, loadedPolicy.Enabled)
assert.Equal(t, "1080p", loadedPolicy.DefaultResolution)
assert.Equal(t, 10, loadedPolicy.DefaultDuration)
assert.Equal(t, "16:9", loadedPolicy.DefaultRatio)
assert.Equal(t, string(types.ConfigImportBatchStatusPublished), loadedBatch.Status)
require.NotEmpty(t, loadedBatch.BaselineJSON)
```

增加回归测试：从渠道模型快照移除上游模型时，发布不退役旧成本规则、不更新渠道模型和能力，也不禁用任何人工或旧导入路由目标。

- [ ] **Step 2: 运行测试并确认失败**

Run: `go test ./service -run 'TestPublishConfigImport.*Candidate|TestPublishConfigImportDoesNotChangeActiveConfiguration' -count=1`

Expected: FAIL，当前发布会激活成本规则、更新售价和模型映射，并按名称替换目标。

- [ ] **Step 3: 在暂存阶段阻断同策略不一致蓝图**

在 `service/config_import_stage.go` 的路由提案校验中按 `default|runtimeCanonicalModel` 聚合蓝图，检测同一策略的 `merge_mode` 和默认参数是否一致。新增问题代码：

```go
const (
	configImportIssueRouteMergeModeConflict = "ROUTE_MERGE_MODE_CONFLICT"
	configImportIssueRouteDefaultsConflict  = "ROUTE_DEFAULTS_CONFLICT"
)
```

同一策略混用 `merge`/`replace` 或推导出不同默认清晰度、时长、比例时写入 `error` 且保持批次 `staged`。在 `service/config_import_stage_test.go` 用两个蓝图验证两个代码。

- [ ] **Step 4: 构造批次路由计划并写入禁用候选**

在 `service/config_import_publish.go` 用一个共享构造器替换逐蓝图完整策略替换：

```go
type configImportPublishedRoutePlan struct {
	PolicyKey model.RoutingPolicyKey
	MergeMode types.ConfigImportRouteMergeMode
	Defaults  modelrouting.Defaults
	Targets   []model.RouteTarget
	ItemIDs   []int64
}

func buildConfigImportPublishedRoutePlans(tx *gorm.DB, items []model.ConfigImportItem) ([]configImportPublishedRoutePlan, error)
```

对每个计划：

```go
if errors.Is(findErr, gorm.ErrRecordNotFound) {
	policy = model.RoutingPolicy{
		GroupName: plan.PolicyKey.GroupName,
		Model: plan.PolicyKey.Model,
		Enabled: false,
		DefaultResolution: plan.Defaults.OutputResolution,
		DefaultDuration: plan.Defaults.DurationSeconds,
		DefaultRatio: plan.Defaults.AspectRatio,
	}
	if err := tx.Create(&policy).Error; err != nil { return err }
}
if err := tx.Where("policy_id = ? AND source_batch_id = ?", policy.ID, batchID).
	Delete(&model.RouteTarget{}).Error; err != nil { return err }
for index := range plan.Targets {
	plan.Targets[index].ID = 0
	plan.Targets[index].PolicyID = policy.ID
	plan.Targets[index].ManagedBy = string(types.RouteTargetManagedByConfigImport)
	plan.Targets[index].SourceBatchID = &batchID
	plan.Targets[index].RetiredAt = nil
	plan.Targets[index].Enabled = false
}
if err := tx.Create(&plan.Targets).Error; err != nil { return err }
if err := tx.Model(&model.ConfigImportItem{}).Where("id IN ?", plan.ItemIDs).Updates(map[string]any{
	"materialized_type": "routing_policy", "materialized_id": policy.ID,
}).Error; err != nil { return err }
```

已有策略不更新 `enabled` 或默认参数。新策略保持禁用。

- [ ] **Step 5: 将活动配置应用从发布移动到激活**

从 `PublishConfigImportBatch` 删除以下调用和成本草稿激活循环：

```go
publishConfigImportAuthoritativeCostRules
ActivateChannelModelCostRuleWithTx
publishConfigImportSaleOptions
publishConfigImportModelMappings
```

这些活动配置函数保留在 service 包内，Task 6 在激活事务中按原顺序调用。删除发布后的成本覆盖调用，并将对应函数重命名为 `recordPostActivationCostCoverage`，只在激活提交和缓存刷新后调用。`publishConfigImportModelMappings` 删除按 `removedModels` 禁用 `RouteTarget` 的分支；路由退休只允许在带来源判断的激活事务中发生。

同时删除 `PublishConfigImportBatch` 提交后的 `RefreshPublishedConfig(refresh)` 调用和新建 `CACHE_REFRESH_PENDING` 的分支。新发布只产生数据库中的禁用候选，不刷新路由、渠道、售价、成本或代理客户端缓存，也不创建新的发布缓存问题；`RetryConfigImportBatchCache` 继续识别历史 `CACHE_REFRESH_PENDING`，用于升级前已存在批次的兼容恢复。发布路径不再维护 `ConfigImportRefreshKeys`，激活计划在 Task 6 独立收集完整刷新范围。

同时将发布事务现有的批次锁改为共享入口，消除 service 层直接使用 `clause.Locking`：

```go
var batch model.ConfigImportBatch
if err := model.LockModelForUpdate(tx, &model.ConfigImportBatch{}).
	Where("id = ?", batchID).First(&batch).Error; err != nil {
	return err
}
```

发布事务只执行当前批次候选路由写入、发布审计和批次状态变更。候选写入后重新调用 `CaptureConfigImportBaseline`，将完整 JSON 回写 `baseline_json`：

```go
after, err := CaptureConfigImportBaseline(tx, batchID)
if err != nil { return err }
afterJSON, err := common.Marshal(after)
if err != nil { return err }
if err := tx.Model(&model.ConfigImportBatch{}).
	Where("id = ? AND status = ?", batchID, types.ConfigImportBatchStatusPublishing).
	Updates(map[string]any{
		"status": string(types.ConfigImportBatchStatusPublished),
		"baseline_json": string(afterJSON),
		"published_at": now,
		"updated_at": now,
	}).Error; err != nil { return err }
```

这样激活预览和事务能够检测发布后发生的活动配置并发修改，同时不会把本批次自己的禁用候选误判为外部变化。

- [ ] **Step 6: 更新旧发布测试的契约**

将 `TestPublishConfigImportRoutesMergesReplacesAndSkipsTargets` 拆成明确契约：

- `merge` 和 `replace` 发布均保留人工目标与旧导入目标。
- 当前批次目标固定禁用并带来源。
- `skip` 不创建当前批次目标。
- 同批次重试只替换本批次候选，不重复插入。
- 成本草稿保持 `draft`，售价、渠道模型、能力、活动成本规则和运行时缓存保持发布前状态。
- 新发布不会创建 `CACHE_REFRESH_PENDING`；历史问题仍可通过 `/refresh-cache` 恢复。
- 发布后 `BaselineJSON.Hash` 等于包含候选目标的当前基线。

删除对 `configImportMergeRouteTargets` 保留启用状态的旧断言，并删除该不再使用的函数。

- [ ] **Step 7: 运行暂存和发布测试**

Run: `go test ./service -run 'ConfigImport.*Route|PublishConfigImport|StageConfigImport' -count=1`

Expected: PASS；发布不会提前切换任何活动配置。

- [ ] **Step 8: 提交候选发布语义**

```powershell
git add service/config_import_publish.go service/config_import_stage.go service/config_import_publish_test.go service/config_import_stage_test.go
git commit -m "feat: publish batch-owned routing candidates"
```

### Task 5: 构建激活预览和统一预检计划

**Files:**
- Create: `service/config_import_activation.go`
- Create: `service/config_import_activation_test.go`
- Modify: `dto/config_import.go`
- Modify: `service/config_import.go`
- Modify: `service/config_import_schema.go`

- [ ] **Step 1: 编写失败的预检表格测试**

在 `service/config_import_activation_test.go` 使用确定性表格测试分别构造以下单一失败条件，并断言唯一 blocker 代码：

```go
type activationFixture struct {
	BatchID        int64
	BindingID      int64
	ChannelID      int
	TargetID       int
	MappingItemID  int64
	CostDraftID    int64
}

func persistActivationBaseline(t *testing.T, batchID int64) {
	t.Helper()
	baseline, err := CaptureConfigImportBaseline(model.DB, batchID)
	require.NoError(t, err)
	encoded, err := common.Marshal(baseline)
	require.NoError(t, err)
	require.NoError(t, model.DB.Model(&model.ConfigImportBatch{}).
		Where("id = ?", batchID).Update("baseline_json", string(encoded)).Error)
}

tests := []struct {
	name string
	mutate func(t *testing.T, fixture *activationFixture)
	code string
}{
	{name: "unpublished batch", code: "ACTIVATION_BATCH_STATUS", mutate: func(t *testing.T, fixture *activationFixture) {
		require.NoError(t, model.DB.Model(&model.ConfigImportBatch{}).Where("id = ?", fixture.BatchID).Update("status", string(types.ConfigImportBatchStatusReady)).Error)
	}},
	{name: "open issue", code: "ACTIVATION_OPEN_ISSUES", mutate: func(t *testing.T, fixture *activationFixture) {
		require.NoError(t, model.DB.Create(&model.ConfigImportIssue{BatchID: fixture.BatchID, Severity: string(types.ConfigImportIssueSeverityWarning), Code: "OPEN_WARNING", Message: "review required", ResolutionStatus: "open"}).Error)
	}},
	{name: "stale activation baseline", code: "ACTIVATION_STALE_BASE_VERSION", mutate: func(t *testing.T, fixture *activationFixture) {
		require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", fixture.ChannelID).Update("models", "concurrent-model").Error)
	}},
	{name: "unconfirmed binding", code: "ACTIVATION_CREDENTIALS_UNCONFIRMED", mutate: func(t *testing.T, fixture *activationFixture) {
		require.NoError(t, model.DB.Model(&model.ConfigImportBinding{}).Where("id = ?", fixture.BindingID).Updates(map[string]any{"credentials_confirmed_by": 0, "credentials_confirmed_at": nil}).Error)
	}},
	{name: "missing candidate", code: "ACTIVATION_TARGET_MISSING", mutate: func(t *testing.T, fixture *activationFixture) {
		require.NoError(t, model.DB.Delete(&model.RouteTarget{}, fixture.TargetID).Error)
		persistActivationBaseline(t, fixture.BatchID)
	}},
	{name: "empty key", code: "ACTIVATION_CHANNEL_KEY_MISSING", mutate: func(t *testing.T, fixture *activationFixture) {
		require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", fixture.ChannelID).Update("key", "").Error)
	}},
	{name: "auto disabled", code: "ACTIVATION_CHANNEL_AUTO_DISABLED", mutate: func(t *testing.T, fixture *activationFixture) {
		require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", fixture.ChannelID).Update("status", common.ChannelStatusAutoDisabled).Error)
	}},
	{name: "missing model mapping", code: "ACTIVATION_MODEL_MAPPING_MISSING", mutate: func(t *testing.T, fixture *activationFixture) {
		require.NoError(t, model.DB.Delete(&model.ConfigImportItem{}, fixture.MappingItemID).Error)
		persistActivationBaseline(t, fixture.BatchID)
	}},
	{name: "missing cost draft", code: "ACTIVATION_COST_DRAFT_MISSING", mutate: func(t *testing.T, fixture *activationFixture) {
		require.NoError(t, model.DB.Delete(&model.ChannelModelCostRule{}, fixture.CostDraftID).Error)
	}},
}
```

另写 `TestPreviewConfigImportBatchActivationRejectsContractMismatch`，临时替换 `RouteTargetContractValidator` 返回固定错误；另写 `TestPreviewConfigImportBatchActivationRejectsManualOverlap`，插入同渠道、同优先级、同约束的启用人工目标后调用 `persistActivationBaseline`，确保该用例只断言 `ACTIVATION_ROUTING_CONFLICT` 而不是先命中陈旧基线。成功用例断言渠道、策略、目标、退休目标计数和排序稳定，且预览不修改任何状态。

- [ ] **Step 2: 运行测试并确认失败**

Run: `go test ./service -run 'TestPreviewConfigImportBatchActivation' -count=1`

Expected: FAIL，激活预览和 blocker DTO 尚不存在。

- [ ] **Step 3: 定义激活预览契约**

在 `dto/config_import.go` 添加：

```go
type ConfigImportActivationBlocker struct {
	Code           string `json:"code"`
	Message        string `json:"message"`
	LineRef        string `json:"line_ref,omitempty"`
	RouteTargetRef string `json:"route_target_ref,omitempty"`
	ChannelID      *int   `json:"channel_id,omitempty"`
}

type ConfigImportActivationPreview struct {
	Ready              bool                            `json:"ready"`
	ChannelCount       int                             `json:"channel_count"`
	PolicyCount        int                             `json:"policy_count"`
	TargetCount        int                             `json:"target_count"`
	RetireTargetCount  int                             `json:"retire_target_count"`
	Blockers           []ConfigImportActivationBlocker `json:"blockers"`
}
```

在 `ConfigImportBatchSummary` 增加 `ActivatedAt *int64`，在 `ConfigImportBatchDetail` 增加 `ActivationPreview *ConfigImportActivationPreview`。

同时在 `service/config_import_schema.go` 扩展现有结构化错误，使后续激活服务可以在 Task 7 注册控制器前先编译并测试：

```go
type ConfigImportSchemaError struct {
	Code    string
	Message string
	Data    any
}

func configImportErrorWithData(code string, data any, format string, args ...any) error {
	return &ConfigImportSchemaError{Code: code, Message: fmt.Sprintf(format, args...), Data: data}
}
```

现有 `configImportError` 继续创建 `Data=nil` 的错误。

- [ ] **Step 4: 实现统一激活计划构造器**

在新文件中定义：

```go
type configImportActivationPolicyPlan struct {
	Policy       model.RoutingPolicy
	Defaults     modelrouting.Defaults
	MergeMode    types.ConfigImportRouteMergeMode
	EnableIDs    []int
	RetireIDs    []int
	ChannelIDs   []int
	Snapshot     modelrouting.PolicySnapshot
}

type configImportActivationPlan struct {
	Batch        model.ConfigImportBatch
	Policies     []configImportActivationPolicyPlan
	ChannelIDs   []int
	Blockers     []dto.ConfigImportActivationBlocker
	BeforeSHA256 string
	CurrentSHA256 string
}

func buildConfigImportActivationPlan(tx *gorm.DB, batchID int64, lock bool) (*configImportActivationPlan, error)
func PreviewConfigImportBatchActivation(ctx context.Context, batchID int64) (*dto.ConfigImportActivationPreview, error)
```

`lock=true` 时，批次、导入项、绑定、问题、成本规则、售价选项、策略、目标和能力读取分别使用 `model.LockModelForUpdate(tx, &model.ConfigImportBatch{})`、`model.LockModelForUpdate(tx, &model.ConfigImportItem{})`、`model.LockModelForUpdate(tx, &model.ConfigImportBinding{})`、`model.LockModelForUpdate(tx, &model.ConfigImportIssue{})`、`model.LockModelForUpdate(tx, &model.ChannelModelCostRule{})`、`model.LockModelForUpdate(tx, &model.Option{})`、`model.LockModelForUpdate(tx, &model.RoutingPolicy{})`、`model.LockModelForUpdate(tx, &model.RouteTarget{})`、`model.LockModelForUpdate(tx, &model.Ability{})`；渠道读取使用 `model.LockChannelsForUpdate(tx)`。禁止 service 层直接使用 `clause.Locking`。两个入口都委托 `model.lockForUpdate`，因此 MySQL/PostgreSQL 发出 `FOR UPDATE`，SQLite 自动跳过不支持的语法。所有查询按主键升序获取锁，避免不同计划以不同顺序锁行。计划从发布时保存的 `source_batch_id` 和批次蓝图重建，不接受客户端提供目标 ID。

- [ ] **Step 5: 实现 blocker 和最终策略快照验证**

先解码批次保存的发布后 `BaselineJSON`，将其 `Hash` 保存到 `plan.BeforeSHA256`；重新调用 `CaptureConfigImportBaseline`，将当前 `Hash` 保存到 `plan.CurrentSHA256`。哈希不一致时追加 `ACTIVATION_STALE_BASE_VERSION`。对每个计划目标加载渠道、本批次模型映射和暂存成本草稿；草稿必须由对应 `ConfigImportItem.MaterializedID` 指向、状态为 `draft`，并通过 `ValidateCostRuleDraft`。缺少映射或草稿分别返回 `ACTIVATION_MODEL_MAPPING_MISSING` 和 `ACTIVATION_COST_DRAFT_MISSING`。调用：

```go
if RouteTargetContractValidator != nil {
	if err := RouteTargetContractValidator(&channel, targetSnapshot); err != nil {
		appendBlocker("ACTIVATION_CHANNEL_CONTRACT", err.Error(), target.Name, channel.Id)
	}
}
```

构造最终快照时：当前批次目标启用；`replace` 退休所有其他导入目标；`merge` 只退休同名其他导入目标；人工目标保持当前启用状态。调用 `modelrouting.ValidatePolicy(snapshot, relaycommon.MaxTaskDurationSeconds)`，将重叠和默认路由错误映射为 `ACTIVATION_ROUTING_CONFLICT`。

blocker 按 `code + channel_id + route_target_ref + line_ref` 排序并去重。

- [ ] **Step 6: 将预览接入批次详情**

在 `GetConfigImportBatch` 中，仅当批次为 `published` 且 `ActivatedAt == nil` 时调用预览并写入详情。`configImportAllowedActions` 增加 `activatedAt` 参数；已发布未激活且不存在开放的 `CACHE_REFRESH_PENDING` 或 `ACTIVATION_CACHE_REFRESH_PENDING` 时返回 `activate`，已激活且存在任一缓存问题时返回 `refresh_cache`，已激活且没有缓存问题时返回空操作。

列表摘要只依据 `ActivatedAt` 给出操作；详情页以 `activation_preview.ready` 作为最终按钮门禁。

- [ ] **Step 7: 运行预览和详情测试**

Run: `go test ./service -run 'ConfigImportBatchActivation|ConfigImportAllowedActions|GetConfigImportBatch' -count=1`

Expected: PASS；预览只读且前后数据库哈希一致。

- [ ] **Step 8: 提交激活预检**

```powershell
git add service/config_import_activation.go service/config_import_activation_test.go dto/config_import.go service/config_import.go service/config_import_schema.go
git commit -m "feat: add config import activation preview"
```

### Task 6: 实现原子激活、软退休和缓存恢复

**Files:**
- Modify: `service/config_import_activation.go`
- Modify: `service/config_import_activation_test.go`
- Modify: `service/config_import_publish.go`
- Modify: `service/config_import_publish_test.go`

- [ ] **Step 1: 编写失败的激活事务测试**

成功用例建立：一个旧活动成本规则、当前批次成本草稿、旧售价、旧模型映射、一个启用人工目标、一个旧批次导入目标、一个当前批次候选、一个手动禁用渠道和禁用能力。调用激活后断言所有活动配置在同一事务切换：

```go
assert.Equal(t, string(types.CostRuleRetired), previousCostRule.Status)
assert.Equal(t, string(types.CostRuleActive), currentCostRule.Status)
assert.Equal(t, expectedOptionValue, loadedOption.Value)
assert.Equal(t, expectedModels, channel.Models)
assert.Equal(t, expectedMapping, channel.GetModelMapping())
assert.True(t, current.Enabled)
assert.Nil(t, current.RetiredAt)
assert.False(t, previous.Enabled)
require.NotNil(t, previous.RetiredAt)
assert.True(t, manual.Enabled)
assert.Nil(t, manual.RetiredAt)
assert.True(t, policy.Enabled)
assert.Equal(t, common.ChannelStatusEnabled, channel.Status)
assert.True(t, ability.Enabled)
require.NotNil(t, batch.ActivatedAt)
assert.Equal(t, *batch.ActivatedAt, *previous.RetiredAt)
```

分别测试 `merge` 只退休同名旧导入目标、`replace` 退休全部旧导入目标、`skip` 不参与。使用 GORM callback 在渠道模型/能力更新时注入错误，断言成本、售价、映射、路由、渠道、能力和批次时间全部回滚。

- [ ] **Step 2: 运行激活测试并确认失败**

Run: `go test ./service -run 'TestActivateConfigImportBatch' -count=1`

Expected: FAIL，激活事务尚不存在。

- [ ] **Step 3: 实现幂等激活入口**

在 `service/config_import_activation.go` 添加 `ActivateConfigImportBatch(ctx context.Context, batchID int64, adminID int) (*dto.ConfigImportBatchDetail, error)`。函数开头对 `adminID <= 0` 返回 `SCHEMA_ADMIN`，对 `batchID <= 0` 返回 `SCHEMA_BATCH_ID`，`ctx == nil` 时改用 `context.Background()`；随后直接进入 Step 4 的事务实现，不保留临时 stub。锁定批次后若 `ActivatedAt != nil`，事务不写任何配置或审计，提交后直接调用 `GetConfigImportBatch(ctx, batchID)` 返回最新详情，以支持客户端在响应丢失后的安全重试。

在同文件定义事务内外传递拒绝审计所需的完整值，避免事务回滚后重新推导 blocker 或哈希：

```go
type configImportActivationAuditInput struct {
	Preview        dto.ConfigImportActivationPreview
	BeforeSHA256   string
	AfterSHA256    string
	FailureCode    string
	FailureMessage string
}
```

`FailureMessage` 只保存固定安全摘要，例如 `activation blocked by 2 checks`；详细 blocker 已在响应中返回，审计不复制任意验证器错误或上游文本。

- [ ] **Step 4: 在事务中重新预检并切换状态**

在同一事务内调用 `buildConfigImportActivationPlan(tx, batchID, true)`。在事务外声明 `var rejectedAudit *configImportActivationAuditInput`；存在 blocker 时把预览、`plan.BeforeSHA256`、`plan.CurrentSHA256` 和失败摘要复制到该值，再返回带 blocker 数据的 `ACTIVATION_BLOCKED`。事务回滚后，如果 `rejectedAudit != nil`，使用独立短事务调用 `appendConfigImportActivationAudit(..., outcome="rejected")`；审计失败只写 `common.SysError`，不能覆盖原始 blocker 错误。加载批次 items 后，先应用所有非路由活动配置：

```go
now := common.GetTimestamp()
var items []model.ConfigImportItem
if err := tx.Where("batch_id = ?", batchID).
	Order("entity_type ASC, business_id ASC, id ASC").Find(&items).Error; err != nil {
	return err
}
if err := publishConfigImportAuthoritativeCostRules(tx, items, &refresh); err != nil {
	return err
}
for _, item := range items {
	if item.EntityType != "cost_rule_drafts" || item.MaterializedID == nil ||
		item.State == string(types.ConfigImportItemStateExcluded) ||
		item.State == string(types.ConfigImportItemStateUnchanged) {
		continue
	}
	if *item.MaterializedID <= 0 {
		return configImportError("ACTIVATION_COST_DRAFT_ID", "cost draft item %d has an invalid materialized ID", item.ID)
	}
	activated, err := model.ActivateChannelModelCostRuleWithTx(tx, int64(*item.MaterializedID), adminID, now, nil)
	if err != nil { return err }
	refresh.CostModelKeys = appendConfigImportRefreshString(refresh.CostModelKeys,
		fmt.Sprintf("%d|%s|%s", activated.ChannelID, activated.BillableUpstreamModel, activated.CostVariantKey))
}
if err := publishConfigImportSaleOptions(tx, items, &refresh); err != nil {
	return err
}
if err := publishConfigImportModelMappings(tx, items, &refresh); err != nil {
	return err
}
```

`MaterializedID` 当前为 `*int`，转换为成本规则 `int64` 前必须先验证大于 0。完成成本、售价和模型映射后，再对每个策略计划执行路由切换：

对每个策略计划执行，并断言实际更新行数等于计划数量，避免候选在预检后被漏写：

```go
if len(policyPlan.RetireIDs) > 0 {
	result := tx.Model(&model.RouteTarget{}).Where("id IN ? AND managed_by = ?", policyPlan.RetireIDs, string(types.RouteTargetManagedByConfigImport)).Updates(map[string]any{
		"enabled": false, "retired_at": now, "updated_at": now,
	})
	if result.Error != nil { return result.Error }
	if result.RowsAffected != int64(len(policyPlan.RetireIDs)) {
		return configImportError("ACTIVATION_CONCURRENT", "route retirement set changed concurrently")
	}
}
result := tx.Model(&model.RouteTarget{}).Where("id IN ? AND source_batch_id = ?", policyPlan.EnableIDs, batchID).Updates(map[string]any{
	"enabled": true, "retired_at": nil, "updated_at": now,
})
if result.Error != nil { return result.Error }
if result.RowsAffected != int64(len(policyPlan.EnableIDs)) {
	return configImportError("ACTIVATION_CONCURRENT", "route activation set changed concurrently")
}
if err := tx.Model(&model.RoutingPolicy{}).Where("id = ?", policyPlan.Policy.ID).Updates(map[string]any{
	"enabled": true,
	"default_resolution": policyPlan.Defaults.OutputResolution,
	"default_duration": policyPlan.Defaults.DurationSeconds,
	"default_ratio": policyPlan.Defaults.AspectRatio,
	"updated_at": now,
}).Error; err != nil { return err }
```

渠道只允许从 `ChannelStatusManuallyDisabled` 切到启用；已启用保持不变，自动禁用已在预检阻断。使用 `WHERE id IN ? AND status = ?` 将手动禁用渠道更新为 `ChannelStatusEnabled`，再重新读取全部计划渠道并断言状态均为启用。随后同一事务执行：

```go
if err := tx.Model(&model.Ability{}).
	Where("channel_id IN ?", plan.ChannelIDs).
	Update("enabled", true).Error; err != nil {
	return err
}
```

该更新发生在 `publishConfigImportModelMappings` 重建能力之后，确保新增标准模型能力最终启用。

- [ ] **Step 5: 写入成功审计和批次时间**

激活计划已保存发布后基线哈希作为 `BeforeSHA256`。完成全部写入后调用 `CaptureConfigImportBaseline(tx, batchID)` 得到 `AfterSHA256`。定义 `appendConfigImportActivationAudit` 作为四种 outcome 共用的审计写入入口，要求两个哈希都非空，计数摘要统一通过 `common.Marshal` 写入 `ConfigImportActivationAudit`。`activated` 审计写入成功后，再用 `WHERE id=? AND activated_at IS NULL` 更新批次：

```go
func appendConfigImportActivationAudit(
	tx *gorm.DB,
	batchID int64,
	adminID int,
	outcome string,
	beforeSHA256 string,
	afterSHA256 string,
	preview dto.ConfigImportActivationPreview,
	failureCode string,
	failureMessage string,
) error {
	if beforeSHA256 == "" || afterSHA256 == "" {
		return configImportError("ACTIVATION_AUDIT_HASH", "activation audit requires before and after hashes")
	}
	type auditBlocker struct {
		Code           string `json:"code"`
		LineRef        string `json:"line_ref,omitempty"`
		RouteTargetRef string `json:"route_target_ref,omitempty"`
		ChannelID      *int   `json:"channel_id,omitempty"`
	}
	blockers := make([]auditBlocker, 0, len(preview.Blockers))
	for _, blocker := range preview.Blockers {
		blockers = append(blockers, auditBlocker{
			Code: blocker.Code, LineRef: blocker.LineRef,
			RouteTargetRef: blocker.RouteTargetRef, ChannelID: blocker.ChannelID,
		})
	}
	summary, err := common.Marshal(struct {
		Ready             bool           `json:"ready"`
		ChannelCount      int            `json:"channel_count"`
		PolicyCount       int            `json:"policy_count"`
		TargetCount       int            `json:"target_count"`
		RetireTargetCount int            `json:"retire_target_count"`
		Blockers          []auditBlocker `json:"blockers"`
	}{
		Ready: preview.Ready, ChannelCount: preview.ChannelCount,
		PolicyCount: preview.PolicyCount, TargetCount: preview.TargetCount,
		RetireTargetCount: preview.RetireTargetCount, Blockers: blockers,
	})
	if err != nil {
		return err
	}
	return tx.Create(&model.ConfigImportActivationAudit{
		BatchID: batchID, AdminID: adminID, Outcome: outcome,
		ChannelCount: preview.ChannelCount, PolicyCount: preview.PolicyCount,
		TargetCount: preview.TargetCount, RetiredTargetCount: preview.RetireTargetCount,
		BeforeSHA256: beforeSHA256, AfterSHA256: afterSHA256,
		FailureCode: failureCode, FailureMessage: failureMessage,
		SummaryJSON: model.ConfigImportSummaryJSON(summary), CreatedAt: common.GetTimestamp(),
	}).Error
}
```

随后更新批次：

```go
result := tx.Model(&model.ConfigImportBatch{}).
	Where("id = ? AND activated_at IS NULL", batchID).
	Updates(map[string]any{"activated_at": now, "updated_at": now})
if result.Error != nil || result.RowsAffected != 1 {
	return configImportError("ACTIVATION_CONCURRENT", "batch %d activation changed concurrently", batchID)
}
```

成功审计和批次时间必须与成本、售价、模型映射、目标、策略、渠道、能力状态同事务提交。

- [ ] **Step 6: 实现提交后缓存刷新和恢复问题**

定义可测试的窄函数变量：

```go
var refreshConfigImportActivation = func(keys ConfigImportRefreshKeys) error {
	if err := RefreshPublishedConfig(keys); err != nil {
		return err
	}
	ResetProxyClientCache()
	return nil
}
```

提交后使用各应用函数累计的 `OptionKeys`、`CostModelKeys`、`RoutingPolicyKeys` 和 `ChannelIDs` 刷新全部受影响缓存。失败时创建或重新打开 `ACTIVATION_CACHE_REFRESH_PENDING`，追加 `cache_refresh_pending` 审计，并返回：

```go
return nil, configImportErrorWithData(
	"ACTIVATION_CACHE_REFRESH_PENDING",
	map[string]any{"batch_id": batchID, "activated": true},
	"batch %d activated but cache refresh is pending",
	batchID,
)
```

该审计从本批次最新 `activated` 审计复制 `BeforeSHA256`、`AfterSHA256` 和计数，不能留空或重新把当前配置当作 before。

扩展 `RetryConfigImportBatchCache`：同时处理 `CACHE_REFRESH_PENDING` 和 `ACTIVATION_CACHE_REFRESH_PENDING`；恢复成功后解决两个问题，若存在激活问题则追加 `cache_refreshed` 审计并调用 `ResetProxyClientCache()`。`cache_refreshed` 同样复制对应 `activated` 审计的两个哈希和计数。

激活缓存刷新成功后调用 `recordPostActivationCostCoverage(ctx, batchID, refresh)`。成本覆盖异常继续写入 `COST_COVERAGE_INCOMPLETE`，但此时它描述的是已经激活的真实配置，不再错误地评价仅发布的草稿。最后调用 `GetConfigImportBatch(ctx, batchID)` 返回带 `activated_at`、最终问题和允许操作的详情。

- [ ] **Step 7: 添加缓存失败和恢复测试**

将 `refreshConfigImportActivation` 临时替换为返回固定错误，断言数据库已激活、问题开放、审计 outcome 为 `cache_refresh_pending`，且其前后哈希等于 `activated` 审计。恢复原函数后调用 `RetryConfigImportBatchCache`，断言问题已解决、审计增加 `cache_refreshed`、前后哈希仍一致，目标状态未重复变化。另加 blocker 用例，断言 `rejected` 审计的两个哈希均非空；陈旧基线时二者不同，其他 blocker 时二者相同。

- [ ] **Step 8: 运行激活、发布和缓存测试**

Run: `go test ./service -run 'ActivateConfigImportBatch|ConfigImport.*Cache|PublishConfigImportBatch' -count=1`

Expected: PASS；事务失败零部分写入，缓存失败可恢复。

- [ ] **Step 9: 提交激活事务**

```powershell
git add service/config_import_activation.go service/config_import_activation_test.go service/config_import_publish.go service/config_import_publish_test.go
git commit -m "feat: activate imported routing batches atomically"
```

### Task 7: 暴露激活 API、权限和结构化错误

**Files:**
- Modify: `controller/config_import.go`
- Modify: `router/config-import-router.go`
- Modify: `router/config_import_router_test.go`
- Modify: `service/authz/resources_config_import.go`
- Create: `controller/config_import_test.go`

- [ ] **Step 1: 编写失败的控制器和路由测试**

新增 `controller/config_import_test.go`，覆盖：

- 已发布可激活批次返回 HTTP 200 和完整批次详情。
- `ACTIVATION_BLOCKED` 返回 HTTP 409，响应包含 `code` 和 `data.blockers`。
- 无效批次 ID 返回 HTTP 400。
- 已激活重试返回 HTTP 200 且不新增成功审计。
- 激活已提交但缓存刷新失败返回 HTTP 503、错误代码 `ACTIVATION_CACHE_REFRESH_PENDING`，随后 GET 批次可见 `activated_at` 和 `refresh_cache` 操作。

在 `router/config_import_router_test.go` 断言：

```go
assertConfigImportRoutePermission(t, http.MethodPost, "/batches/:id/activate", authz.ConfigImportPublish, controller.ActivateConfigImportBatch)
```

- [ ] **Step 2: 运行测试并确认失败**

Run: `go test ./controller ./router -run 'ConfigImport.*Activate|ConfigImportRoutes' -count=1`

Expected: FAIL，处理器和路由尚未注册。

- [ ] **Step 3: 添加处理器和响应**

在 `controller/config_import.go` 添加：

```go
func ActivateConfigImportBatch(c *gin.Context) {
	id, err := configImportID(c)
	if err != nil {
		writeConfigImportError(c, err)
		return
	}
	detail, err := service.ActivateConfigImportBatch(c, id, c.GetInt("id"))
	if err != nil {
		writeConfigImportError(c, err)
		return
	}
	common.ApiSuccess(c, detail)
}
```

同时将 `configImportID` 的无效 ID 错误改为 `&service.ConfigImportSchemaError{Code: "SCHEMA_BATCH_ID", Message: "invalid config import batch id"}`，保证所有批次路径参数错误稳定返回 400。注册 `POST /batches/:id/activate`，权限为 `ConfigImportPublish`。

- [ ] **Step 4: 扩展结构化错误映射**

`ConfigImportSchemaError.Data` 和 `configImportErrorWithData` 已在 Task 5 定义。修改 `writeConfigImportError`，先构造 `payload := gin.H{"success": false, "code": schemaErr.Code, "message": schemaErr.Message}`，仅在 `schemaErr.Data != nil` 时设置 `payload["data"] = schemaErr.Data`；对以下代码返回 409：

```text
STALE_BASE_VERSION
ACTIVATION_BLOCKED
ACTIVATION_CONCURRENT
ROUTE_OWNERSHIP_ROLLBACK_CONFLICT
```

上述代码返回 409。`ACTIVATION_CACHE_REFRESH_PENDING` 返回 503。`ACTIVATION_BLOCKED` 的 `data` 固定为 `{"blockers":[...]}`；缓存待恢复错误的 `data` 固定为 `{"batch_id":<id>,"activated":true}`，不包含 Key 或完整上游错误。其他 schema/参数错误保持 400，数据库未知错误保持 500。

- [ ] **Step 5: 更新权限资源说明**

保持 action 名 `publish` 不变，将 `DescriptionKey` 精确更新为 `Publish, activate, and backfill ownership for reviewed imported configuration.`；不新增角色默认权限，避免现有管理员丢失能力。该新键在 Task 10 通过规定脚本写入七种语言。

- [ ] **Step 6: 运行控制器、路由和权限测试**

Run: `go test ./controller ./router ./service/authz -run 'ConfigImport|Capabilities' -count=1`

Expected: PASS，所有高风险操作只允许 `ConfigImportPublish`。

- [ ] **Step 7: 提交 API**

```powershell
git add controller/config_import.go controller/config_import_test.go router/config-import-router.go router/config_import_router_test.go service/authz/resources_config_import.go
git commit -m "feat: expose config import batch activation"
```

### Task 8: 扩展前端类型、API 和向导状态

**Files:**
- Modify: `web/src/features/config-import/types.ts`
- Modify: `web/src/features/config-import/api.ts`
- Modify: `web/src/features/config-import/lib/batch-state.ts`
- Modify: `web/src/features/config-import/lib/__tests__/batch-state.test.ts`

- [ ] **Step 1: 编写失败的类型和状态测试**

在 `batch-state.test.ts` 添加：

```ts
test('routes a published unactivated batch to activation', () => {
  const current = batch({
    status: 'published',
    activated_at: null,
    allowed_actions: ['activate'],
    activation_preview: {
      ready: true,
      channel_count: 2,
      policy_count: 3,
      target_count: 13,
      retire_target_count: 67,
      blockers: [],
    },
  })
  const state = deriveWizardState(current)
  assert.equal(state.step, 'activation')
  assert.equal(state.canActivate, true)
})

test('routes an activated batch to the final result', () => {
  const state = deriveWizardState(batch({ status: 'published', activated_at: 123 }))
  assert.equal(state.step, 'publish_result')
  assert.equal(state.canActivate, false)
})
```

再断言 blocker 非空、preview 缺失或 allowed action 缺失时 `canActivate=false`。

- [ ] **Step 2: 运行测试并确认失败**

Run: `cd web; bun test --parallel=1 src/features/config-import/lib/__tests__/batch-state.test.ts`

Expected: FAIL，类型和激活步骤尚不存在。

- [ ] **Step 3: 扩展 Zod 契约**

在 `types.ts` 添加严格 schema：

```ts
export const configImportActivationBlockerSchema = z.object({
  code: z.string().min(1),
  message: z.string().min(1),
  line_ref: z.string().optional(),
  route_target_ref: z.string().optional(),
  channel_id: z.number().int().positive().optional(),
}).strict()

export const configImportActivationPreviewSchema = z.object({
  ready: z.boolean(),
  channel_count: z.number().int().nonnegative(),
  policy_count: z.number().int().nonnegative(),
  target_count: z.number().int().nonnegative(),
  retire_target_count: z.number().int().nonnegative(),
  blockers: z.array(configImportActivationBlockerSchema),
}).strict()
```

批次 summary 增加 `activated_at: z.number().int().positive().nullish()`，detail 增加 `activation_preview: configImportActivationPreviewSchema.nullish()`。

- [ ] **Step 4: 添加激活 API**

在 `api.ts` 添加：

```ts
export async function activateConfigImport(
  id: number
): Promise<ConfigImportBatchDetail> {
  const response = await api.post(`${CONFIG_IMPORT_PATH}/${id}/activate`)
  return parseDetail(response.data)
}
```

再添加缓存恢复 API；它复用已有后端入口，并在成功后重新读取完整批次详情：

```ts
export async function refreshConfigImportCache(
  id: number
): Promise<ConfigImportBatchDetail> {
  await api.post(`${CONFIG_IMPORT_PATH}/${id}/refresh-cache`)
  return getConfigImportBatch(id)
}
```

不在客户端发送目标、渠道或确认状态。

- [ ] **Step 5: 扩展向导状态**

在 `CONFIG_IMPORT_STEPS` 中把 `activation` 放在 `publish_review` 和 `publish_result` 之间。`ConfigImportWizardState` 增加 `canActivate`。`published` 且 `activated_at` 为空时进入激活步骤；已有激活时间进入最终结果。

`canActivate` 必须同时满足：状态 published、allowed action 包含 activate、preview.ready、blockers 为空。

- [ ] **Step 6: 运行状态测试和类型检查**

Run: `cd web; bun test --parallel=1 src/features/config-import/lib/__tests__/batch-state.test.ts; bun run typecheck`

Expected: PASS。

- [ ] **Step 7: 提交前端契约**

```powershell
git add web/src/features/config-import/types.ts web/src/features/config-import/api.ts web/src/features/config-import/lib/batch-state.ts web/src/features/config-import/lib/__tests__/batch-state.test.ts
git commit -m "feat: add config import activation state"
```

### Task 9: 实现激活确认页和完整向导流程

**Files:**
- Create: `web/src/features/config-import/components/activation-step.tsx`
- Create: `web/src/features/config-import/components/__tests__/activation-step.test.tsx`
- Modify: `web/src/features/config-import/components/config-import-stepper.tsx`
- Modify: `web/src/features/config-import/components/publish-result-step.tsx`
- Modify: `web/src/features/config-import/index.tsx`
- Modify: `web/src/features/config-import/components/__tests__/config-import-wizard.test.tsx`

- [ ] **Step 1: 编写失败的激活组件测试**

使用现有 Happy DOM 和 i18next 测试模式，覆盖：

```tsx
test('requires a ready preview and explicit confirmation', async () => {
  const mounted = await mount({ ready: true, blockers: [] })
  const activate = button(mounted.container, 'Activate import')
  assert.equal(activate.disabled, true)
  await act(async () => checkbox(mounted.container, 'Confirm activation').click())
  assert.equal(activate.disabled, false)
  await act(async () => activate.click())
  assert.equal(mounted.calls, 1)
})

test('shows every blocker and keeps activation disabled', async () => {
  const mounted = await mount({
    ready: false,
    blockers: [
      { code: 'ACTIVATION_COST_DRAFT_MISSING', message: 'Missing cost draft.', route_target_ref: 'route-a' },
      { code: 'ACTIVATION_CHANNEL_AUTO_DISABLED', message: 'Channel is auto disabled.', channel_id: 9 },
    ],
  })
  assert.match(mounted.container.textContent ?? '', /ACTIVATION_COST_DRAFT_MISSING/)
  assert.match(mounted.container.textContent ?? '', /route-a/)
  assert.equal(button(mounted.container, 'Activate import').disabled, true)
})
```

增加计数展示、异步 loading、后端错误 alert 和长目标引用换行测试。

- [ ] **Step 2: 运行组件测试并确认失败**

Run: `cd web; bun test --parallel=1 src/features/config-import/components/__tests__/activation-step.test.tsx`

Expected: FAIL，组件不存在。

- [ ] **Step 3: 实现激活组件**

创建 `ActivationStep`，使用现有 `Button`、普通边框分区和原生 checkbox，不嵌套卡片。核心接口：

```tsx
export interface ActivationStepProps {
  batch: ConfigImportBatchDetail
  canActivate: boolean
  isActivating?: boolean
  onActivate: () => Promise<void>
}
```

展示四个稳定尺寸计数：渠道、策略、新目标、退休目标。blocker 使用列表并显示代码、消息、渠道 ID、线路和目标引用。按钮禁用条件：

```tsx
const disabled =
  !props.canActivate ||
  !preview?.ready ||
  preview.blockers.length > 0 ||
  !confirmed ||
  props.isActivating
```

- [ ] **Step 4: 串联向导发布和激活**

在 `ConfigImportWizardProps` 增加 `onActivate?: (id: number) => Promise<ConfigImportBatchDetail>` 和 `onRefreshCache?: (id: number) => Promise<ConfigImportBatchDetail>`。发布成功后后端详情进入 activation；激活使用：

```tsx
const activated = await runMutation(
  props.onActivate ?? activateConfigImport
)
if (activated) setReviewStep(undefined)
```

扩展 `runMutation` 的错误分支：当错误代码为 `ACTIVATION_CACHE_REFRESH_PENDING` 时，立即调用 `props.onLoadBatch ?? getConfigImportBatch` 重新加载当前批次并 `setBatch`，使已经提交的激活进入结果页，同时保留“缓存待恢复”错误提示。`ConfigImportStepper` 为 `activation` 返回 `Route activation` 标签。`PublishResultStep` 在 `activated_at` 非空时显示激活成功及最终计数，并把 `CACHE_REFRESH_PENDING`、`ACTIVATION_CACHE_REFRESH_PENDING` 都视为缓存恢复状态；存在 `refresh_cache` 操作时显示 `Retry cache refresh` 按钮，点击后调用 `runMutation(props.onRefreshCache ?? refreshConfigImportCache)`。

- [ ] **Step 5: 更新完整向导测试**

修改 `config-import-wizard.test.tsx` 的 fixture，增加 `activated_at` 和 `activation_preview`。完整成功调用序列必须为：

```ts
[
  'pricing:12',
  'stage:12',
  'routes:12',
  'validate:12',
  'publish:12',
  'activate:12',
]
```

发布后先断言页面显示 `Route activation`，完成确认和激活后再断言 `Published` 与激活成功摘要。blocker 用例不得调用 `onActivate`。另加缓存失败用例：`onActivate` 抛出带 `code='ACTIVATION_CACHE_REFRESH_PENDING'` 的错误，`onLoadBatch` 返回含 `activated_at`、开放缓存问题和 `refresh_cache` 操作的详情；断言页面进入结果步骤、显示恢复提示且不会再次调用激活。点击 `Retry cache refresh` 后只调用一次 `onRefreshCache`，成功详情移除缓存问题和该操作。

- [ ] **Step 6: 运行配置导入前端测试**

Run: `cd web; bun test --parallel=1 src/features/config-import`

Expected: PASS，无 React act 警告。

- [ ] **Step 7: 提交激活 UI**

```powershell
git add web/src/features/config-import/components/activation-step.tsx web/src/features/config-import/components/__tests__/activation-step.test.tsx web/src/features/config-import/components/config-import-stepper.tsx web/src/features/config-import/components/publish-result-step.tsx web/src/features/config-import/index.tsx web/src/features/config-import/components/__tests__/config-import-wizard.test.tsx
git commit -m "feat: add config import activation review"
```

### Task 10: 通过规定脚本补齐七种语言

**Files:**
- Create temporarily: `web/scripts/add-missing-keys.mjs`
- Modify through script: `web/src/i18n/locales/en.json`
- Modify through script: `web/src/i18n/locales/zh.json`
- Modify through script: `web/src/i18n/locales/zh-TW.json`
- Modify through script: `web/src/i18n/locales/fr.json`
- Modify through script: `web/src/i18n/locales/ja.json`
- Modify through script: `web/src/i18n/locales/ru.json`
- Modify through script: `web/src/i18n/locales/vi.json`
- Delete after use: `web/scripts/add-missing-keys.mjs`

- [ ] **Step 1: 运行 i18n 预检**

Run: `cd web; bun run i18n:sync`

读取 `web/src/i18n/locales/_reports/_sync-report.json`，记录七种语言的 missing、extras、untranslated 现状；本任务只处理下述 15 个新键，不顺带改写无关历史翻译。

- [ ] **Step 2: 创建脚本并填入完整翻译**

使用 `i18n-translate` skill 规定的 `stableStringify` 和逐 locale 写入结构，`newKeys` 使用以下完整值：

```js
const newKeys = {
  en: {
    'Activate import': 'Activate import',
    'Activation blockers': 'Activation blockers',
    'Activation failed.': 'Activation failed.',
    'Activation review': 'Activation review',
    'Channels to enable': 'Channels to enable',
    'Confirm activation': 'Confirm activation',
    'I confirm this published batch is ready to become active.': 'I confirm this published batch is ready to become active.',
    'Policies to enable': 'Policies to enable',
    'Publish, activate, and backfill ownership for reviewed imported configuration.': 'Publish, activate, and backfill ownership for reviewed imported configuration.',
    'Retry cache refresh': 'Retry cache refresh',
    'Route activation': 'Route activation',
    'Targets to enable': 'Targets to enable',
    'Targets to retire': 'Targets to retire',
    'The published configuration is active.': 'The published configuration is active.',
    'This batch is published but cannot be activated.': 'This batch is published but cannot be activated.',
  },
  zh: {
    'Activate import': '激活导入',
    'Activation blockers': '激活阻断项',
    'Activation failed.': '激活失败。',
    'Activation review': '激活确认',
    'Channels to enable': '将启用的渠道',
    'Confirm activation': '确认激活',
    'I confirm this published batch is ready to become active.': '我确认此已发布批次可以正式启用。',
    'Policies to enable': '将启用的策略',
    'Publish, activate, and backfill ownership for reviewed imported configuration.': '发布、激活已审核的导入配置，并回填路由归属。',
    'Retry cache refresh': '重试刷新缓存',
    'Route activation': '路由激活',
    'Targets to enable': '将启用的目标',
    'Targets to retire': '将退休的目标',
    'The published configuration is active.': '已发布配置现已启用。',
    'This batch is published but cannot be activated.': '此批次已发布，但当前无法激活。',
  },
  'zh-TW': {
    'Activate import': '啟用匯入',
    'Activation blockers': '啟用阻擋項目',
    'Activation failed.': '啟用失敗。',
    'Activation review': '啟用確認',
    'Channels to enable': '將啟用的渠道',
    'Confirm activation': '確認啟用',
    'I confirm this published batch is ready to become active.': '我確認此已發佈批次可以正式啟用。',
    'Policies to enable': '將啟用的策略',
    'Publish, activate, and backfill ownership for reviewed imported configuration.': '發佈、啟用已審核的匯入設定，並回填路由歸屬。',
    'Retry cache refresh': '重試重新整理快取',
    'Route activation': '路由啟用',
    'Targets to enable': '將啟用的目標',
    'Targets to retire': '將退役的目標',
    'The published configuration is active.': '已發佈設定現已啟用。',
    'This batch is published but cannot be activated.': '此批次已發佈，但目前無法啟用。',
  },
  fr: {
    'Activate import': "Activer l’import",
    'Activation blockers': "Blocages de l’activation",
    'Activation failed.': "Échec de l’activation.",
    'Activation review': "Vérification de l’activation",
    'Channels to enable': "Canaux à activer",
    'Confirm activation': "Confirmer l’activation",
    'I confirm this published batch is ready to become active.': "Je confirme que ce lot publié est prêt à être activé.",
    'Policies to enable': "Règles à activer",
    'Publish, activate, and backfill ownership for reviewed imported configuration.': "Publier et activer la configuration importée validée, puis réattribuer les routes.",
    'Retry cache refresh': "Relancer l’actualisation du cache",
    'Route activation': "Activation du routage",
    'Targets to enable': "Cibles à activer",
    'Targets to retire': "Cibles à retirer",
    'The published configuration is active.': "La configuration publiée est active.",
    'This batch is published but cannot be activated.': "Ce lot est publié, mais ne peut pas être activé.",
  },
  ja: {
    'Activate import': 'インポートを有効化',
    'Activation blockers': '有効化のブロッカー',
    'Activation failed.': '有効化に失敗しました。',
    'Activation review': '有効化の確認',
    'Channels to enable': '有効化するチャネル',
    'Confirm activation': '有効化を確認',
    'I confirm this published batch is ready to become active.': '公開済みバッチを有効化できることを確認しました。',
    'Policies to enable': '有効化するポリシー',
    'Publish, activate, and backfill ownership for reviewed imported configuration.': '確認済みのインポート設定を公開・有効化し、ルート所有情報を補完します。',
    'Retry cache refresh': 'キャッシュ更新を再試行',
    'Route activation': 'ルートの有効化',
    'Targets to enable': '有効化するターゲット',
    'Targets to retire': '廃止するターゲット',
    'The published configuration is active.': '公開済み設定が有効になりました。',
    'This batch is published but cannot be activated.': 'このバッチは公開済みですが、有効化できません。',
  },
  ru: {
    'Activate import': 'Активировать импорт',
    'Activation blockers': 'Препятствия для активации',
    'Activation failed.': 'Не удалось выполнить активацию.',
    'Activation review': 'Проверка активации',
    'Channels to enable': 'Каналы для включения',
    'Confirm activation': 'Подтвердить активацию',
    'I confirm this published batch is ready to become active.': 'Я подтверждаю, что опубликованный пакет готов к активации.',
    'Policies to enable': 'Политики для включения',
    'Publish, activate, and backfill ownership for reviewed imported configuration.': 'Публикация и активация проверенной импортированной конфигурации, а также восстановление принадлежности маршрутов.',
    'Retry cache refresh': 'Повторить обновление кеша',
    'Route activation': 'Активация маршрутов',
    'Targets to enable': 'Цели для включения',
    'Targets to retire': 'Цели для вывода',
    'The published configuration is active.': 'Опубликованная конфигурация активна.',
    'This batch is published but cannot be activated.': 'Пакет опубликован, но сейчас его нельзя активировать.',
  },
  vi: {
    'Activate import': 'Kích hoạt bản nhập',
    'Activation blockers': 'Điều kiện chặn kích hoạt',
    'Activation failed.': 'Kích hoạt thất bại.',
    'Activation review': 'Xác nhận kích hoạt',
    'Channels to enable': 'Kênh sẽ bật',
    'Confirm activation': 'Xác nhận kích hoạt',
    'I confirm this published batch is ready to become active.': 'Tôi xác nhận lô đã phát hành này sẵn sàng được kích hoạt.',
    'Policies to enable': 'Chính sách sẽ bật',
    'Publish, activate, and backfill ownership for reviewed imported configuration.': 'Phát hành, kích hoạt cấu hình nhập đã được duyệt và bổ sung quyền sở hữu tuyến.',
    'Retry cache refresh': 'Thử làm mới bộ nhớ đệm',
    'Route activation': 'Kích hoạt định tuyến',
    'Targets to enable': 'Đích sẽ bật',
    'Targets to retire': 'Đích sẽ ngừng dùng',
    'The published configuration is active.': 'Cấu hình đã phát hành đang hoạt động.',
    'This batch is published but cannot be activated.': 'Lô này đã phát hành nhưng hiện chưa thể kích hoạt.',
  },
}
```

- [ ] **Step 3: 应用翻译并同步**

Run: `cd web; node scripts/add-missing-keys.mjs; bun run i18n:sync`

Expected: 七种语言均应用 15 个键，sync 退出码为 0。

- [ ] **Step 4: 删除临时脚本并检查键完整性**

用 `apply_patch` 删除 `web/scripts/add-missing-keys.mjs`，然后运行：

Run: `rg -n 'Activate import|Route activation|Activation blockers|Retry cache refresh|Publish, activate, and backfill ownership' web/src/i18n/locales/{en,zh,zh-TW,fr,ja,ru,vi}.json`

Expected: 每个键在七个 locale 文件中各出现一次。

- [ ] **Step 5: 运行前端测试、类型、lint 和格式检查**

Run: `cd web; bun test --parallel=1 src/features/config-import; bun run typecheck; bunx oxlint -c .oxlintrc.json src/features/config-import; bun run format:check`

Expected: 全部 PASS。

- [ ] **Step 6: 提交翻译**

```powershell
git add web/src/i18n/locales/en.json web/src/i18n/locales/zh.json web/src/i18n/locales/zh-TW.json web/src/i18n/locales/fr.json web/src/i18n/locales/ja.json web/src/i18n/locales/ru.json web/src/i18n/locales/vi.json
git commit -m "feat: translate config import activation"
```

### Task 11: 实现已发布批次显式复制为新绑定批次

**Files:**
- Modify: `model/config_import.go`
- Modify: `model/main.go`
- Modify: `model/config_import_migration_test.go`
- Modify: `dto/config_import.go`
- Modify: `service/config_import.go`
- Modify: `service/config_import_test.go`
- Modify: `controller/config_import.go`
- Modify: `controller/config_import_test.go`
- Modify: `router/config-import-router.go`
- Modify: `router/config_import_router_test.go`
- Modify: `web/src/features/config-import/api.ts`
- Modify: `web/src/features/config-import/types.ts`
- Modify: `web/src/features/config-import/index.tsx`
- Modify: `web/src/features/config-import/components/publish-result-step.tsx`
- Modify: `web/src/features/config-import/components/__tests__/config-import-wizard.test.tsx`
- Modify via script: `web/src/i18n/locales/{en,zh,zh-TW,fr,ja,ru,vi}.json`

- [ ] **Step 1: 写批次复制和迁移失败测试**

覆盖：普通重复上传仍返回同一批次；已发布源批次复制后保留相同 `payload_sha256`，但新批次 ID 和 `deduplication_key` 不同；新批次状态为 `binding`，`copied_from_batch_id` 指向源批次，实体重置为 `new`，且不存在绑定、问题、处理记录、物化 ID、发布或激活时间。未发布源批次返回 `COPY_FOR_BINDING_SOURCE_STATUS`。迁移测试验证历史 `payload_sha256` 唯一索引被移除、历史批次去重键回填且相同载荷可保存复制批次。

- [ ] **Step 2: 运行失败测试**

Run: `go test ./model ./service -run 'ConfigImport.*(Copy|Identity|Idempotent)' -count=1`

Expected: 因复制服务、迁移和字段尚不存在而 FAIL。

- [ ] **Step 3: 实现模型、迁移和复制事务**

`ConfigImportBatch` 新增可空唯一 `DeduplicationKey` 与 `CopiedFromBatchID`，将 `PayloadSHA256` 调整为普通索引。`migrateConfigImportBatchIdentity` 在 `migrateDB` 和 `migrateDBFast` 的 `AutoMigrate` 前执行：添加缺失列、逐批次回填 `upload:<payload_sha256>`、移除旧载荷唯一索引。普通上传使用 `upload:<payload_sha256>` 查询和写入；复制使用 `copy:<uuid>`。

实现 `CopyConfigImportBatchForBinding(ctx, adminID, sourceBatchID)`：锁定并校验已发布源批次，复制批次元数据和规范化实体，清空运行时与审阅状态，不复制关联表，事务提交后返回新批次详情。

- [ ] **Step 4: 实现控制器、权限路由和结构化响应**

新增 `POST /api/config-import/batches/:id/copy-for-binding`，复用 `config_import.write` 权限。响应直接返回新批次详情；源状态不合法返回 409 和 `COPY_FOR_BINDING_SOURCE_STATUS`。已发布批次的 `allowed_actions` 增加 `copy_for_binding`。

- [ ] **Step 5: 运行后端测试至通过**

Run: `go test ./model ./service ./controller ./router -run 'ConfigImport.*(Copy|Identity|Idempotent)|ConfigImportAllowedActions' -count=1`

Expected: PASS。

- [ ] **Step 6: 先写前端交互失败测试**

在已发布结果页点击“复制为新绑定批次”，断言只调用一次复制 API，并切换到返回的新批次渠道绑定步骤；请求期间按钮禁用。没有 `copy_for_binding` 允许动作时不显示按钮。

- [ ] **Step 7: 实现前端 API、类型和入口**

增加复制 API 函数和 `copied_from_batch_id` 类型。结果页使用带复制图标的按钮；向导收到新批次后清除结果页状态并进入渠道绑定。

- [ ] **Step 8: 通过规定脚本同步七种语言**

新增 `Copy as new binding batch` 和复制失败相关文案，使用 `web/scripts/add-missing-keys.mjs` 写入全部七种语言，再运行 `bun run i18n:sync`，最后删除临时脚本。

- [ ] **Step 9: 运行前端验证**

Run: `cd web; bun test --parallel=1 src/features/config-import; bun run typecheck; bunx oxlint -c .oxlintrc.json src/features/config-import`

Expected: PASS。

- [ ] **Step 10: 在本地真实数据库创建新绑定批次**

从当前已发布批次 20 调用复制接口，记录新批次 ID；确认 `status=binding`、`copied_from_batch_id=20`、`payload_sha256` 与源批次相同、绑定数为 0、实体数与源批次一致。普通重新上传当前 JSON仍返回批次 20。

### Task 12: 执行全量验证、历史回填和真实链路前置验收

**Files:**
- Verify only
- Update after execution: `docs/superpowers/plans/2026-08-06-config-import-route-retirement-activation.md`

- [ ] **Step 1: 格式化受影响 Go 文件**

Run:

```powershell
gofmt -w types/routing_ownership.go model/routing_policy.go model/config_import.go model/locking.go model/main.go model/config_import_migration_test.go service/routing_policy.go service/routing_policy_test.go model/routing_policy_test.go service/config_import_route_ownership.go service/config_import_route_ownership_test.go service/config_import_publish.go service/config_import_publish_test.go service/config_import_stage.go service/config_import_stage_test.go service/config_import_activation.go service/config_import_activation_test.go service/config_import.go service/config_import_schema.go dto/config_import.go controller/config_import.go controller/config_import_test.go router/config-import-router.go router/config_import_router_test.go service/authz/resources_config_import.go
```

Expected: 命令退出 0。

- [ ] **Step 2: 运行后端聚焦测试**

Run: `go test ./model ./service ./controller ./router -run 'ConfigImport|RoutingPolicy|RouteTargetOwnership' -count=1`

Expected: PASS。

- [ ] **Step 3: 运行后端完整相关包测试**

Run: `go test ./model ./service ./controller ./router -count=1`

Expected: PASS。

- [ ] **Step 4: 运行三数据库迁移测试**

始终运行 SQLite：

Run: `go test ./model -run 'TestRouteTargetOwnershipMigration|TestConfigImportMigrationConfiguredDatabases' -count=1 -v`

若测试数据库可用，设置：

```powershell
$env:TEST_MYSQL_DSN='root:password@tcp(127.0.0.1:3306)/newapi_test?charset=utf8mb4&parseTime=True&loc=Local'
$env:TEST_POSTGRES_DSN='host=127.0.0.1 user=postgres password=password dbname=newapi_test port=5432 sslmode=disable TimeZone=UTC'
go test ./model -run 'TestRouteTargetOwnershipMigration|TestConfigImportMigrationConfiguredDatabases' -count=1 -v
```

Expected: SQLite、MySQL、PostgreSQL 全部 PASS；未提供 DSN 的环境必须在交付说明中明确为 SKIP。

- [ ] **Step 5: 运行前端测试和生产构建**

Run: `cd web; bun test --parallel=1 src/features/config-import src/features/model-routing/components/route-target-editor-client.test.tsx; bun run typecheck; bun run lint; bun run build`

Expected: 全部 PASS，Rsbuild 成功生成生产产物。

- [ ] **Step 6: 运行静态和敏感信息检查**

Run: `git diff --check`

Run: `rg -n '(sk-[A-Za-z0-9_-]{16,}|api[_-]?key\s*[:=]\s*["''][^"'']+)' docs/superpowers/specs/2026-08-06-config-import-route-retirement-activation-design.md docs/superpowers/plans/2026-08-06-config-import-route-retirement-activation.md service model controller router web/src/features/config-import`

Expected: `git diff --check` 无输出；敏感信息搜索无真实凭据匹配。

- [ ] **Step 7: 在目标环境执行历史归属 dry-run**

使用已登录管理员会话调用：

```text
GET /api/config-import/route-ownership/backfill-preview
```

保存响应并核对：

- `matched` 中当前 Mock 目标的策略、渠道、`route_target_ref` 和批次 ID 正确。
- `ambiguous` 不会被应用；逐条记录原因。
- `unmatched` 保持人工归属。
- 预览前后 `route_targets.enabled` 数量完全一致。

- [ ] **Step 8: 应用回填并验证幂等**

调用：

```text
POST /api/config-import/route-ownership/backfill
```

记录返回的 `operation_id`。再次调用 preview，预期 `matched` 待应用数为 0。抽查已回填目标：`managed_by=config_import`、`source_batch_id` 正确，`enabled` 未变化。再次调用 apply，预期 `applied_count=0`。

- [ ] **Step 9: 在测试数据库验证回填回滚**

复制生产结构到测试数据库，使用记录的测试 `operation_id` 调用：

```text
POST /api/config-import/route-ownership/backfill/:operation_id/rollback
```

Expected: 未被后续修改的目标恢复原归属；先人工修改一条测试目标后，整次回滚返回 `ROUTE_OWNERSHIP_ROLLBACK_CONFLICT` 且零部分回滚。生产环境不执行回滚，除非 dry-run 审核发现错误且目标尚未进入激活流程。

- [ ] **Step 10: 验收新真实渠道批次**

按长期维护流程执行一次真实批次：

```text
下载最新 sd收录.xlsx
-> 生成最新渠道模板与导入 JSON
-> 导入
-> 绑定已手工录入 API Key 的真实渠道
-> 暂存与校验
-> 发布
-> 检查 activation_preview
-> 激活
```

发布后、激活前验证旧路由和渠道状态未变化；激活后验证本批次目标、策略、渠道和能力启用，旧导入目标写入 `retired_at`，人工目标不变。

- [ ] **Step 11: 执行真实 Ark SDK 视频链路测试**

对激活后的每个代表性真实渠道执行：

```text
POST /api/v3/contents/generations/tasks
GET  /api/v3/contents/generations/tasks/:task_id
GET  /api/v3/contents/generations/tasks
```

至少覆盖文本生视频和渠道声明支持的图片、视频、音频参考模式。每个渠道先设置单次验收预算和最小时长/最低清晰度用例；任务创建请求只发送一次，创建响应超时或连接中断时先按请求日志、计费日志和上游任务列表核对是否已创建，禁止自动重发可能重复计费的创建请求。轮询和列表查询可以重试，但必须使用退避并设置总超时。

记录上游请求成功、上游任务 ID、轮询状态、结果 URL、计费日志、命中渠道、`route_target_ref`、`cost_variant_key`、实际费用和失败映射。任何渠道缺少真实 Key、余额或预算批准时标记为真实验收阻断，不把 mock/contract 测试记为真实通过，也不为追求全覆盖绕过激活门禁。

- [ ] **Step 12: 最终提交**

确认工作树只包含本计划文件后执行：

```powershell
git status --short
git diff --stat
git add docs/superpowers/specs/2026-08-06-config-import-route-retirement-activation-design.md docs/superpowers/plans/2026-08-06-config-import-route-retirement-activation.md
git commit -m "docs: plan config import route activation"
```

Expected: 提交成功，未包含 API Key、数据库导出、运行日志或无关文件。
