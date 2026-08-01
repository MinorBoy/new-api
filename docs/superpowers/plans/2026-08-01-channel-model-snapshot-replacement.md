# 渠道模型全量快照替换实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让配置导入把 `sd收录.xlsx` 作为已绑定渠道的权威全量模型快照，并在发布前展示、发布时原子执行模型下架。

**Architecture:** 在服务层从批次 `model_mappings` 与绑定关系构造每渠道快照，暂存时返回确定性差异，发布时在现有事务内完整替换渠道模型和映射，同时退役下架模型的成本规则、禁用相关路由目标并重建能力。前端只渲染后端给出的权威差异，不在浏览器重复计算。

**Tech Stack:** Go、Gin、GORM、React 19、TypeScript、React Query、i18next、Bun。

**Design:** `docs/superpowers/specs/2026-08-01-channel-model-snapshot-replacement-design.md`

---

### Task 1: 定义渠道模型快照差异契约

**Files:**
- Modify: `types/config_import.go`
- Modify: `dto/config_import.go`
- Modify: `service/config_import.go`
- Test: `service/config_import_pricing_test.go`

- [ ] **Step 1: 编写失败测试**

构造一个批次、绑定渠道和两条 `model_mappings`，断言批次详情返回 `added_models`、`retained_models`、`removed_models`，且每个列表排序稳定。

- [ ] **Step 2: 运行测试并确认按预期失败**

Run: `go test ./service -run 'TestGetConfigImportBatchIncludesChannelModelSnapshotDiff' -count=1`

Expected: FAIL，响应契约或差异构造尚不存在。

- [ ] **Step 3: 添加最小契约和纯差异构造**

新增 `ConfigImportChannelModelSnapshotDiff`，字段包含 `channel_id`、`channel_name`、`line_refs`、`added_models`、`retained_models`、`removed_models`。实现按绑定渠道聚合有效映射并与当前渠道模型比较的服务函数，所有输出排序。

- [ ] **Step 4: 运行测试并确认通过**

Run: `go test ./service -run 'TestGetConfigImportBatchIncludesChannelModelSnapshotDiff' -count=1`

Expected: PASS。

### Task 2: 允许绑定渠道接受快照新增模型

**Files:**
- Modify: `model/config_import.go`
- Modify: `model/main.go`
- Test: `model/config_import_migration_test.go`
- Modify: `service/config_import_stage.go`
- Test: `service/config_import_stage_test.go`

- [ ] **Step 1: 编写失败测试**

创建类型匹配但尚未声明新上游模型的现有渠道，绑定包含该模型的渠道线，断言绑定成功且绑定阶段不修改 `channels.models` 或 `abilities`。

- [ ] **Step 2: 运行测试并确认按预期失败**

Run: `go test ./service -run 'TestUpdateConfigImportBindingsAllowsSnapshotToAddModels' -count=1`

Expected: FAIL，当前 `validateConfigImportBindingChannel` 返回 `BINDING_CHANNEL_MODEL`。

- [ ] **Step 3: 实现最小绑定变更**

删除现有渠道必须预先声明每个上游模型的验证；保留渠道类型、Secure 渠道线和 MegaByAI 能力验证。删除绑定阶段为新建渠道追加模型的副作用，所有模型写入统一延迟到发布事务。删除旧 `idx_config_import_binding_channel` 唯一约束并加入幂等迁移，使同一渠道可绑定多条渠道线；保留每条渠道线在批次内唯一。

- [ ] **Step 4: 运行绑定相关测试**

Run: `go test ./service -run 'ConfigImportBinding|UpdateConfigImportBindings' -count=1`

Expected: PASS。

### Task 3: 在发布事务内替换快照并下架依赖

**Files:**
- Modify: `service/config_import_publish.go`
- Test: `service/config_import_publish_test.go`

- [ ] **Step 1: 编写失败集成测试**

渠道当前模型为 `keep,old,manual`，当前映射包含 `keep` 和 `old`；批次快照为 `keep,new`。同时建立 `old` 的活动成本规则和启用路由目标。发布后断言：

```text
channels.models = keep,new 及对应标准模型并集
abilities 只包含目标集合
model_mapping 只包含本次快照映射
old/manual 不再声明
old 的活动成本规则为 retired
old 的路由目标 enabled=false
```

- [ ] **Step 2: 运行测试并确认按预期失败**

Run: `go test ./service -run 'TestPublishConfigImportBatchReplacesChannelModelSnapshot' -count=1`

Expected: FAIL，当前发布逻辑只合并模型与映射。

- [ ] **Step 3: 实现快照替换**

在 `publishConfigImportModelMappings` 中按渠道聚合未排除映射，使用 `lockForUpdate` 等现有模型层锁约定读取渠道，构造目标集合和映射；通过 GORM 更新成本规则状态与结束时间、禁用相关路由目标、完整更新渠道并调用 `UpdateAbilities(tx)`。收集成本、路由和渠道缓存刷新键。

- [ ] **Step 4: 添加事务回滚测试**

注入能力重建失败，断言渠道模型、映射、成本规则和路由目标均保持发布前状态。

- [ ] **Step 5: 运行发布测试**

Run: `go test ./service -run 'ConfigImportPublish|PublishConfigImportBatch' -count=1`

Expected: PASS。

### Task 4: 将模型差异纳入基线和批次详情

**Files:**
- Modify: `service/config_import_stage.go`
- Modify: `service/config_import.go`
- Test: `service/config_import_pricing_test.go`
- Test: `service/config_import_publish_test.go`

- [ ] **Step 1: 编写失败测试**

暂存后直接修改绑定渠道模型，发布应返回 `STALE_BASE_VERSION`；重新暂存后的批次详情应反映新的新增、保留和下架列表。

- [ ] **Step 2: 运行测试并确认按预期失败**

Run: `go test ./service -run 'TestConfigImportModelSnapshotDiffParticipatesInBaseline' -count=1`

Expected: FAIL，当前基线未覆盖完整渠道模型快照。

- [ ] **Step 3: 扩展基线捕获和批次详情**

将绑定渠道的完整 `models` 与 `model_mapping` 确定性序列化到基线；批次详情调用同一差异构造函数，避免暂存、确认和发布使用不同算法。

- [ ] **Step 4: 运行相关测试**

Run: `go test ./service -run 'ConfigImport.*Baseline|ChannelModelSnapshot' -count=1`

Expected: PASS。

### Task 5: 在导入确认页展示下架明细

**Files:**
- Modify: `web/src/features/config-import/types.ts`
- Modify: `web/src/features/config-import/components/publish-review-step.tsx`
- Test: `web/src/features/config-import/components/__tests__/publish-review-step.test.tsx`
- Modify through script: `web/src/i18n/locales/{en,zh,zh-TW,fr,ja,ru,vi}.json`

- [ ] **Step 1: 编写失败组件测试**

模拟批次详情包含两个渠道差异，断言确认页逐项显示渠道名、下架数量和每个下架模型；没有下架模型时显示“本次没有模型下架”。长模型名所在元素必须允许换行而不产生水平遮挡。

- [ ] **Step 2: 运行测试并确认按预期失败**

Run: `cd web && bun test src/features/config-import/components/__tests__/publish-review-step.test.tsx`

Expected: FAIL，类型和确认区块尚不存在。

- [ ] **Step 3: 实现确认区块**

复用现有 Alert、Badge、Separator 等项目组件。在确认页现有差异摘要中加入“渠道模型快照”区块；存在下架时使用警示色和无序列表，模型名使用等宽文本并允许任意位置换行。不添加嵌套卡片。

- [ ] **Step 4: 通过 i18n 脚本添加七种语言**

创建临时 `web/scripts/add-missing-keys.mjs`，为新增英文键提供 `en`、`zh`、`zh-TW`、`fr`、`ja`、`ru`、`vi` 值，运行：

```powershell
cd web
node scripts/add-missing-keys.mjs
bun run i18n:sync
```

完成后删除临时脚本。

- [ ] **Step 5: 运行组件测试、类型和格式检查**

Run: `cd web && bun test src/features/config-import/components/__tests__/publish-review-step.test.tsx && bun run typecheck && bun run lint && bun run format:check`

Expected: 全部 PASS。

### Task 6: 全量验证与本地验收

**Files:**
- Verify only

- [ ] **Step 1: 运行后端相关测试**

Run: `go test ./service ./model -count=1`

Expected: PASS。

- [ ] **Step 2: 运行前端相关测试与生产构建**

Run: `cd web && bun test src/features/config-import scripts/channel-model-template && bun run typecheck && bun run build`

Expected: PASS。

- [ ] **Step 3: 运行静态检查**

Run: `gofmt -w types/config_import.go dto/config_import.go service/config_import.go service/config_import_stage.go service/config_import_publish.go service/config_import_pricing_test.go service/config_import_publish_test.go`，然后运行 `git diff --check`。

Expected: 无格式错误。

- [ ] **Step 4: 更新容器并浏览器验收**

构建并启动本地容器，重新导入当前模板，在确认页核对新增、保留、下架列表；发布后核对数据库中模型、能力、映射、路由目标与成本规则状态完全符合确认内容。

- [ ] **Step 5: 提交到 ysr**

仅暂存本任务文件，确认当前分支为 `ysr` 后提交；严禁提交或合并到 `main`。
