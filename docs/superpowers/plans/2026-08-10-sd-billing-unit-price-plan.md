# sd 收录表单价字段迁移实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让渠道模型模板生成器严格读取最新版 `sd` 表的 `计费方式 + 单价 元` 契约，并完成一轮可审计的模板刷新、配置导入激活和 Mock Ark SDK 验收。

**Architecture:** `source.ts` 在源表边界严格识别最新版表头，把 `计费方式` 规范化为内部 `计费`，保留统一的 `单价 元`；`build.ts` 仅按计费模式消费统一单价，不再保留旧三列分支。代码通过 focused TDD 后，再下载权威源表，执行表格门禁、config-import 状态机和隔离数据库 E2E。

**Tech Stack:** TypeScript、Bun test、ExcelJS、`@oai/artifact-tool`、React config-import UI、Go E2E。

---

### Task 1: 将测试 fixture 和读取测试切换到新表头

**Files:**
- Modify: `web/scripts/channel-model-template/__fixtures__/sd-source-v1.xlsx`
- Modify: `web/scripts/channel-model-template/__tests__/source.test.ts`

- [ ] **Step 1: 更新 fixture 的 `sd` 表头**

将第 2 行设为最新版顺序；核心前十列必须是：

```text
渠道,充值汇率,手续费,计费倍率,付费模式,模型ID,版本,清晰度,计费方式,单价 元
```

后续依次保留 `参考图数`、`参考视频数`、`参考音频数`、`最大素材数`、`视频音频合计上限`、`参考视频总时长上限 秒`、`最小参考图数`、`超分`、`时长范围`、`比例`、`视频输入`、`过真人脸`、`素材库`、`NSFW`、`协议`、`状态`、`并发数`、四个折扣列、`接入`、`已测`、`售价`、`利润`、`上游模型分组`、`备注`。把原有效成本移到 `单价 元`，代表行使用 `second` 和 `1.38`。

- [ ] **Step 2: 写最新版字段读取测试**

在 `source.test.ts` 增加：

```ts
test('reads the latest billing mode and unified unit price', async () => {
  const source = await readSourceWorkbook(fixturePath)
  const model = source.models[0]
  assert.equal(model?.fields.计费, 'second')
  assert.equal(model?.fields['单价 元'], 1.38)
  assert.equal(model?.fields['元/秒'], undefined)
  assert.equal(model?.fields['元/次'], undefined)
  assert.equal(model?.fields['元/1M'], undefined)
})
```

- [ ] **Step 3: 写旧表头、混用表头和缺失新列拒绝测试**

从新 fixture 复制临时工作簿：第一份把 `单价 元` 改成 `元/秒`，第二份同时增加 `元/次`，第三份清空 `单价 元` 表头。分别断言 `readSourceWorkbook` 拒绝并匹配 `sd header mismatch`；缺列用例还要匹配 `missing=单价 元`。

- [ ] **Step 4: 运行测试确认 RED**

```powershell
Set-Location web
bun test --parallel=1 scripts/channel-model-template/__tests__/source.test.ts
```

预期：新增成功路径因生产解析仍要求旧三列而失败，且不是语法或 fixture 损坏错误。

### Task 2: 以 TDD 实现统一单价映射

**Files:**
- Modify: `web/scripts/channel-model-template/__tests__/build.test.ts`
- Modify: `web/scripts/channel-model-template/source.ts`
- Modify: `web/scripts/channel-model-template/build.ts`
- Modify: `docs/channel-model-template-generator.md`

- [ ] **Step 1: 将 build fixture 改为新字段**

`sourceWithOfficialPrice()` 使用 `计费: 'second'` 和 `'单价 元': 1.38`，删除三个旧价格字段；结构化素材合同字段保持不变。

- [ ] **Step 2: 写三种模式映射测试**

```ts
test('maps all latest billing modes from one unit price field', () => {
  for (const [billingMode, expectedMode, expectedField] of [
    ['second', 'per_duration', 'nativePerSecond'],
    ['call', 'per_request', 'nativePerRequest'],
    ['token', 'per_token', 'nativePerMillion'],
  ] as const) {
    const source = sourceWithOfficialPrice()
    const model = firstSourceModel(source)
    model.fields.计费 = billingMode
    model.fields['单价 元'] = 2
    const cost = buildTemplateData(source, rules).costs.find(
      (item) => item.scenario === 'no_video'
    )
    assert.equal(cost?.mode, expectedMode)
    assert.equal(cost?.[expectedField], '2')
  }
})
```

- [ ] **Step 3: 写单价边界和未知模式测试**

对 `null`、`0`、`-1`、`'invalid'` 逐项设置 `单价 元`，断言 `COST_PRICE_INVALID` 为 `WARN` 且成本为 `draft`；设置 `计费='minute'`，断言出现 `COST_MODE_UNKNOWN` 且严重级别为 `FAIL`。

- [ ] **Step 4: 运行 build 测试确认 RED**

```powershell
Set-Location web
bun test --parallel=1 scripts/channel-model-template/__tests__/build.test.ts
```

预期：新字段取价用例失败，证明测试捕获了旧三列分支。

- [ ] **Step 5: 实现最小源表解析改动**

在 `source.ts` 删除 legacy 和含旧价格列的结构化表头数组，保留一个最新版 `SD_HEADERS`；I/J 为 `计费方式`、`单价 元`。结构化 `sd` 只按该数组读取，读取后把 `计费方式` 复制为内部 `计费` 并删除原字段，保留 `单价 元`。检测到任一旧价格表头时主动抛出 `sd header mismatch`，不回退旧结构。

- [ ] **Step 6: 实现最小统一单价取值改动**

在 `build.ts` 将 `priceForMode(record, mode)` 改为只返回 `numericField(record, '单价 元')`；模式参数只保留类型签名需要时使用，或删除该参数并同步调用点。保留 `COST_MODES`、正数校验、单位和模式专属输出字段。

- [ ] **Step 7: 运行新增测试确认 GREEN**

```powershell
Set-Location web
bun test --parallel=1 scripts/channel-model-template/__tests__/source.test.ts scripts/channel-model-template/__tests__/build.test.ts
```

预期：全部通过，三种模式从同一单价生成正确的 `CostRow`。

- [ ] **Step 8: 运行 focused 回归**

```powershell
Set-Location web
bun test --parallel=1 scripts/channel-model-template/__tests__ src/channel-config-converter
```

预期：退出码 0，0 failures。

- [ ] **Step 9: 更新维护文档**

在 `docs/channel-model-template-generator.md` 的 `sd` 主数据说明中明确：供应商成本只读取 `计费方式 + 单价 元`；`second/call/token` 分别映射为按秒、按次、按百万 Token，旧三列不再支持。保持现有简体中文文档风格。

- [ ] **Step 10: 提交生成器迁移**

```powershell
git add -- `
  web/scripts/channel-model-template/__fixtures__/sd-source-v1.xlsx `
  web/scripts/channel-model-template/__tests__/source.test.ts `
  web/scripts/channel-model-template/__tests__/build.test.ts `
  web/scripts/channel-model-template/source.ts `
  web/scripts/channel-model-template/build.ts `
  docs/channel-model-template-generator.md
git commit -m "fix: migrate sd source pricing headers"
```

提交前运行 `git diff --cached --check`，只暂存以上文件，不包含工作树中既有的 Cangyuan 修改或验收报告。

### Task 3: 下载、生成并校验本轮模板

**Files:**
- Create: `outputs/2026-08-10-sd-unit-price-refresh/sd收录.xlsx`
- Create: `outputs/2026-08-10-sd-unit-price-refresh/渠道模型成本与利润模板-v1.xlsx`
- Create: `outputs/2026-08-10-sd-unit-price-refresh/渠道模型成本与利润模板-v1.report.json`
- Create: `outputs/2026-08-10-sd-unit-price-refresh/source-validation.json`

- [ ] **Step 1: 下载固定 Google 表格**

通过用户提供的已登录 Chrome 标签核对标题和表格 ID `1qnzFB8mmc4glK7Eo7xxulgNwipEbdmtKrgQcvdc0BUM`，使用“下载 -> Microsoft Excel (.xlsx)”。按下载前后文件列表、修改时间和 SHA-256 确认身份，复制为本轮 `sd收录.xlsx`，不输出 Cookie、Token 或凭据。

- [ ] **Step 2: 用 artifact-tool 校验源表**

通过 `codex_app__load_workspace_dependencies` 获取依赖，按电子表格技能创建会话工作目录和 `node_modules` junction。检查 `channel`、`sd`、`sd官价` 的范围、表头、有效行数和值；扫描 `#REF!|#DIV/0!|#VALUE!|#NAME\\?|#N/A`；渲染每张表的头部、代表性数据和尾部。将范围、行数、错误数、计费方式计数和 SHA-256 写入 `source-validation.json`。

- [ ] **Step 3: 运行生成器**

```powershell
Set-Location web
bun run channel-model-template:generate -- `
  --source "..\\outputs\\2026-08-10-sd-unit-price-refresh\\sd收录.xlsx" `
  --rules "scripts\\channel-model-template\\conversion-rules.json" `
  --base "src\\channel-config-converter\\__fixtures__\\channel-config-v1-corrected.xlsx" `
  --output "C:\\Users\\880pro\\Documents\\new-api\\outputs\\2026-08-10-sd-unit-price-refresh\\渠道模型成本与利润模板-v1.xlsx" `
  --report "C:\\Users\\880pro\\Documents\\new-api\\outputs\\2026-08-10-sd-unit-price-refresh\\渠道模型成本与利润模板-v1.report.json" `
  --allow-warnings
```

生成报告必须 `FAIL=0`；逐项记录每个 `WARN` 和 `draft` 的原因、影响和处置，无法解释时停止。

- [ ] **Step 4: 用 artifact-tool 校验模板**

检查并渲染 `使用说明`、`参数`、`渠道`、`模型SKU`、`官方售价`、`渠道成本`、`模型映射`、`利润测算`、`来源`、`校验`。扫描公式错误；至少抽查按次、按秒、按 Token、一个负毛利和一个正毛利样本，确认按次成本不按时长摊薄，分组倍率只影响用户收入。

- [ ] **Step 5: 更新仓库源表**

仅在上述门禁通过后，把本轮源表更新到 `docs/new-channels/sd收录.xlsx`，记录更新前后 SHA-256；失败时不改仓库源表。

- [ ] **Step 6: 提交通过门禁的仓库源表**

```powershell
git add -- docs/new-channels/sd收录.xlsx
git diff --cached --check
git commit -m "data: refresh sd channel catalog"
```

若新下载源表与仓库源表哈希相同，则跳过该提交。

### Task 4: 配置导入、发布和激活

**Files:**
- Create: `outputs/2026-08-10-sd-unit-price-refresh/channel-config-v1.json`
- Create: `outputs/2026-08-10-sd-unit-price-refresh/channel-config-issues.json`
- Create: `outputs/2026-08-10-sd-unit-price-refresh/import-audit.json`

- [ ] **Step 1: 在 `/config-import` 转换并导出选中配置**

上传通过门禁的模板，选择类型匹配且已有凭据的既有真实渠道；缺少凭据或匹配不唯一的线路跳过并记录。下载“选中配置 JSON”，核对 `manifest.source_sha256` 与本轮模板 SHA-256。

- [ ] **Step 2: 完成审阅状态机**

通过页面和现有登录态依次完成导入、绑定暂存、冲突解决暂存、定价审阅暂存、路由差异审阅和校验。记录批次 ID、实体计数、绑定及跳过原因，不提取或输出凭据。

- [ ] **Step 3: 发布、激活和缓存确认**

仅在 blocker=0 且激活预检通过时发布并激活；记录退休目标数、新启用目标数和缓存结果。缓存异常时只刷新缓存，不重复激活。把全过程写入 `import-audit.json`。

### Task 5: 执行 Mock Ark SDK E2E

**Files:**
- Create: `outputs/2026-08-10-sd-unit-price-refresh/e2e.log`
- Inspect: `e2e/testdata/channel-config-v1.json`

- [ ] **Step 1: 核对 E2E fixture**

确认本轮 `channel-config-v1.json` 的实体计数和语义差异；只有 E2E 必须使用新配置时才更新仓库 fixture，并记录原哈希。

- [ ] **Step 2: 运行隔离数据库 E2E**

```powershell
go test ./e2e -run '^TestSeedanceImportedMaterialMatrixFullFlowE2E$' -count=1 -p=1
```

记录完整命令、退出码、首个失败目标、任务/使用日志、供应商成本和利润统计。该步骤只使用 Mock 上游；失败时停止，不把失败目标标为通过。

- [ ] **Step 3: 恢复运行时设置**

若流程写入本地运行库，执行前保存成本模式、最低毛利率和分组倍率，成功或失败后均恢复，并通过 API/UI 复核。

### Task 6: 报告和最终验证

**Files:**
- Create: `outputs/2026-08-10-sd-unit-price-refresh/验收报告.md`

- [ ] **Step 1: 编写简体中文报告**

列出源表、模板、生成报告、选中 JSON、问题报告、导入审计和 E2E 日志的绝对路径及 SHA-256；列出工作表行数、`FAIL/WARN/draft` 数、利润抽查、批次状态、绑定/发布/激活/缓存结果和 E2E 退出码。

- [ ] **Step 2: 明确覆盖范围**

报告区分真实源表数据、本地代码和 Mock 上游结果；写明旧表头已停止兼容、跳过渠道及原因、未覆盖项和设置恢复结论。未执行真实 Canary 时明确写明未访问供应商。

- [ ] **Step 3: 执行最终验证**

重新运行 focused tests、E2E 和 `git diff --check`，读取最新退出码和失败计数后再下结论。释放本轮占用的浏览器标签，不提交 `outputs/` 下载物和日志。
