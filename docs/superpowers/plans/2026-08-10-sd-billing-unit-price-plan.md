# sd 收录表最新版结构与完整刷新实施计划

> **供 agentic worker 使用：** 必须使用 `superpowers:subagent-driven-development`（推荐）或 `superpowers:executing-plans` 按任务执行本计划。所有步骤使用复选框跟踪。

**目标：** 让渠道模型模板生成器严格读取包含 `系列 + 计费方式 + 单价 元`、不再包含 `视频输入` 的最新版源表，并完成模板生成、配置导入激活和 Mock Ark SDK E2E 验收。

**架构：** `source.ts` 在工作簿边界规范化系列与计费字段，并过滤仅用于分组的系列行；`build.ts` 用系列隔离官方价格匹配，从结构化 `参考视频数` 推导视频输入能力。生成器通过 focused TDD 后，再按 `refreshing-sd-channel-config` 状态机执行下载、表格门禁、导入发布、激活和 E2E。

**技术栈：** TypeScript、Bun test、ExcelJS、`@oai/artifact-tool`、React config-import UI、Go E2E、Chrome 浏览器控制。

---

## 文件结构

- `web/scripts/channel-model-template/source.ts`：最新版源表表头、模型行过滤、计费和系列规范化。
- `web/scripts/channel-model-template/build.ts`：素材合同、系列感知官方价格索引、SKU/售价/成本构建。
- `web/scripts/channel-model-template/types.ts`：构建期 `SkuRow.series` 字段；写入器不新增 V1 列。
- `web/scripts/channel-model-template/__fixtures__/sd-source-v1.xlsx`：最小最新版源表契约 fixture。
- `web/scripts/channel-model-template/__tests__/source.test.ts`：源表结构、拒绝旧字段、系列分组行和系列值校验。
- `web/scripts/channel-model-template/__tests__/build.test.ts`：统一单价、视频输入能力和跨系列价格隔离。
- `web/scripts/channel-model-template/__tests__/generate.test.ts`：最新版源表的报告与工作簿生成。
- `web/scripts/channel-model-template/conversion-rules.json`：最新版渠道编号到稳定渠道代码的显式映射。
- `docs/channel-model-template-generator.md`：最新版主数据维护契约。
- `.codex/skills/refreshing-sd-channel-config/SKILL.md`：刷新流程中的系列和视频能力门禁。
- `.codex/skills/refreshing-sd-channel-config/references/project-workflow.md`：项目命令、表格和验收合同。
- `docs/new-channels/sd收录.xlsx`：全部门禁通过后更新的仓库权威副本。
- `outputs/2026-08-10-sd-series-refresh/`：本轮下载物、模板、导入 JSON、日志和验收报告；若目录已存在则使用递增后缀，禁止覆盖。

### 任务 1：把测试 fixture 切换到最新版源表契约

**文件：**

- 修改：`web/scripts/channel-model-template/__fixtures__/sd-source-v1.xlsx`
- 修改：`web/scripts/channel-model-template/__tests__/source.test.ts:28-166`

- [ ] **步骤 1：使用电子表格运行时更新 fixture**

通过 `codex_app__load_workspace_dependencies` 获取 Node 与 `@oai/artifact-tool` 路径，在唯一临时目录创建 `node_modules` junction。导入 fixture 后把 `sd` 第 2 行调整为以下核心表头，删除旧价格列和 `视频输入`：

```text
渠道,充值汇率,手续费,计费倍率,付费模式,模型ID,系列,版本,清晰度,计费方式,单价 元,参考图数,参考视频数,参考音频数,最大素材数,视频音频合计上限,,参考视频总时长上限 秒,最小参考图数,超分,时长范围,比例,过真人脸,素材库,NSFW,协议,状态,并发数,按次 15s 折扣 比较,折扣 秒 无V,折扣 秒 含V,折扣 M 无V,折扣 M 含V,接入,已测,售价,利润,上游模型分组,备注
```

代表模型行写入 `系列=2`、`计费方式=second`、`单价 元=1.38`、`参考视频数=3`。增加一条只填写 `系列=2.5` 的分组行。`sd官价` 第 6 行增加 `系列`，代表价格行写入 `系列=2`。

- [ ] **步骤 2：渲染 fixture 并核对表头和值**

用 `workbook.inspect` 检查 `sd!A1:AM6` 和 `sd官价!A1:L10`，再渲染三个工作表。确认原格式未被清空、表头可见、代表值未被字符串化，然后导出覆盖 fixture。

- [ ] **步骤 3：写最新版读取失败测试**

在 `source.test.ts` 增加并调整以下行为断言：

```ts
test('reads the latest series and unit-price contract and skips series-only group rows', async () => {
  const source = await readSourceWorkbook(fixturePath)
  const model = source.models[0]
  const official = source.officialPrices[0]

  assert.equal(model?.fields.系列, 2)
  assert.equal(model?.fields.计费, 'second')
  assert.equal(model?.fields['单价 元'], 1.38)
  assert.equal(model?.fields.视频输入, undefined)
  assert.equal(source.models.some((record) => record.fields.系列 === 2.5), false)
  assert.equal(official?.fields.系列, 2)
})
```

把结构化表头测试中的列号和字段同步到最新版表头；移除 `视频输入` 写值。

- [ ] **步骤 4：写旧字段和非法系列拒绝测试**

通过 ExcelJS 在临时副本中分别制造以下输入并断言拒绝：

```ts
const forbiddenHeaders = ['元/秒', '元/次', '元/1M', '视频输入']
```

每个旧表头都必须匹配 `/sd header mismatch/`；缺失 `系列` 或 `单价 元` 必须包含对应 `missing=`；模型或官方价格行的系列为空、零、负数、`invalid` 时必须匹配 `series invalid` 和准确工作表行号。

- [ ] **步骤 5：运行测试确认 RED**

```powershell
Set-Location web
bun test --parallel=1 scripts/channel-model-template/__tests__/source.test.ts
```

预期：当前解析器仍要求旧价格列或 `视频输入`，最新版 fixture 读取失败；不能出现测试代码语法错误。

### 任务 2：实现严格最新版源表解析

**文件：**

- 修改：`web/scripts/channel-model-template/source.ts:24-361`
- 测试：`web/scripts/channel-model-template/__tests__/source.test.ts`

- [ ] **步骤 1：收敛为唯一最新版表头**

删除 `SD_LEGACY_HEADERS`、`SD_RENAMED_STRUCTURED_HEADERS` 和旧结构分支。`SD_HEADERS` 使用任务 1 的最新版字段，`OFFICIAL_PRICE_HEADERS` 以 `系列` 开头：

```ts
export const OFFICIAL_PRICE_HEADERS = [
  '系列',
  '模型',
  '版本',
  '分辨率',
  '不含视频 元/M',
  '包含视频 元/M',
  '帧率',
  '长边',
  '短边',
  '不含视频 元/秒',
  '包含视频 元/秒',
  '备注',
] as const
```

- [ ] **步骤 2：实现系列规范化和分组行过滤**

增加可复用的源表领域函数，对真实模型行和官方价格行执行：

```ts
function normalizeSeries(record: SourceRecord): SourceRecord {
  const text = cellText(record.fields.系列 ?? null)
  const value = Number(text)
  if (text === '' || !Number.isFinite(value) || value <= 0) {
    throw new Error(
      `${record.location.sheet} series invalid at row ${record.location.row}`
    )
  }
  return { ...record, fields: { ...record.fields, 系列: value } }
}
```

读取 `sd` 时把 `blankCheckColumns` 明确设为 `渠道` 与 `模型ID` 对应列，使仅填写 `系列` 的分组行被跳过；读取后把 `计费方式` 复制到内部 `计费` 并删除源字段，再执行 `normalizeSeries`。官方价格记录也执行同一规范化。

- [ ] **步骤 3：主动拒绝旧字段**

在读取 `sd` 表头后检查：

```ts
const forbidden = ['元/秒', '元/次', '元/1M', '视频输入'].filter((header) =>
  modelHeaderSet.has(header)
)
if (forbidden.length > 0) {
  throw new Error(`sd header mismatch; forbidden=${forbidden.join(',')}`)
}
```

随后只使用 `readHeaders(models, 2, SD_HEADERS, 'sd')`；保留额外非业务列和空分隔列的兼容性。

- [ ] **步骤 4：运行 source 测试确认 GREEN**

```powershell
Set-Location web
bun test --parallel=1 scripts/channel-model-template/__tests__/source.test.ts
```

预期：全部通过，且输出无未处理异常。

- [ ] **步骤 5：提交源表解析迁移**

```powershell
git add -- web/scripts/channel-model-template/__fixtures__/sd-source-v1.xlsx web/scripts/channel-model-template/__tests__/source.test.ts web/scripts/channel-model-template/source.ts
git commit -m "fix: parse latest sd source schema"
```

### 任务 3：用失败测试固定统一单价、视频能力和系列隔离

**文件：**

- 修改：`web/scripts/channel-model-template/__tests__/build.test.ts:45-650`
- 修改：`web/scripts/channel-model-template/__tests__/generate.test.ts:50-110`

- [ ] **步骤 1：更新内存 SourceWorkbook fixture**

`sourceWithOfficialPrice()` 的模型字段使用：

```ts
系列: 2,
版本: 'fast',
清晰度: '720',
计费: 'second',
'单价 元': 1.38,
参考视频数: 3,
```

删除 `元/秒`、`元/次`、`元/1M` 和 `视频输入`。官方价格记录增加 `系列: 2`。

- [ ] **步骤 2：写三种计费模式和非法价格测试**

```ts
test('maps all latest billing modes from one unit price field', () => {
  const cases = [
    ['second', 'per_duration', 'nativePerSecond'],
    ['call', 'per_request', 'nativePerRequest'],
    ['token', 'per_token', 'nativePerMillion'],
  ] as const

  for (const [billingMode, expectedMode, priceField] of cases) {
    const source = sourceWithOfficialPrice()
    const model = firstSourceModel(source)
    model.fields.计费 = billingMode
    model.fields['单价 元'] = 2
    const cost = buildTemplateData(source, rules).costs[0]
    assert.equal(cost?.mode, expectedMode)
    assert.equal(cost?.[priceField], '2')
  }
})
```

对 `null`、`0`、`-1`、`invalid` 逐项断言 `COST_PRICE_INVALID/WARN`、成本 `draft`、映射禁用；`计费=minute` 断言 `COST_MODE_UNKNOWN/FAIL`。

- [ ] **步骤 3：写参考视频数能力测试**

```ts
test('derives video input support from the reference-video count', () => {
  for (const [count, expected] of [[0, '否'], [3, '是']] as const) {
    const source = sourceWithOfficialPrice()
    firstSourceModel(source).fields.参考视频数 = count
    const output = buildTemplateData(source, rules)
    assert.equal(output.skus[0]?.supportsVideoInput, expected)
  }
})
```

保留 `参考视频数=0` 时视频总时长归零测试，但删除对旧 `视频输入` 字段的赋值。

- [ ] **步骤 4：写跨系列官方价格隔离测试**

构造两个模型名和分辨率相同、系列分别为 2 与 2.5、价格不同的官方记录，把 2.0 记录放在前面，渠道模型使用 `系列=2.5`。断言生成售价使用 2.5 价格且 `sku.series === '2.5'`。该测试必须在旧的“模型 + 分辨率”索引下失败。

- [ ] **步骤 5：更新生成器警告测试**

`generate.test.ts` 查找 `单价 元` 列并写入 0，不再查找 `元/秒`。保持对 `COST_PRICE_INVALID`、生成工作簿和转换成功的断言。

- [ ] **步骤 6：运行测试确认 RED**

```powershell
Set-Location web
bun test --parallel=1 scripts/channel-model-template/__tests__/build.test.ts scripts/channel-model-template/__tests__/generate.test.ts
```

预期：统一单价、正数参考视频能力或跨系列隔离至少一项失败，证明测试覆盖旧实现。

### 任务 4：实现系列感知构建和视频能力推导

**文件：**

- 修改：`web/scripts/channel-model-template/types.ts:86-106`
- 修改：`web/scripts/channel-model-template/build.ts:77-1092`
- 测试：`web/scripts/channel-model-template/__tests__/build.test.ts`
- 测试：`web/scripts/channel-model-template/__tests__/generate.test.ts`

- [ ] **步骤 1：给构建期 SKU 增加系列字段**

```ts
export type SkuRow = {
  businessId: string
  series: string
  model: string
  // 其余字段保持不变
}
```

`write.ts` 继续显式写现有 V1 列，不新增“系列”列。

- [ ] **步骤 2：实现系列感知官方价格索引**

新增稳定键函数并替换全部索引调用：

```ts
function officialPriceKey(
  seriesValue: string,
  model: string,
  resolutionValue: string
): string {
  return `${seriesValue}\u0000${model}\u0000${resolution(resolutionValue)}`
}
```

`indexOfficialPrices` 使用官方记录的 `系列`；`findOfficial` 增加 `seriesValue` 参数。`buildSkus` 只从同系列官方记录推断模型并写入 `SkuRow.series`；`buildSales` 使用 `sku.series`；`buildCostsAndMappings` 的 SKU 查找键也包含渠道记录的系列。

- [ ] **步骤 3：从素材合同推导视频输入能力**

在 `buildSkus` 每个模型循环中复用 `referenceContract(modelRecord)`：

```ts
const materialContract = referenceContract(modelRecord).contract
const supportsVideoInput =
  materialContract !== null && materialContract.videos > 0 ? '是' : '否'
```

写入 SKU 时使用该值，并删除 `field(modelRecord, '视频输入')`。同时删除 `referenceContract` 中旧的“视频输入为否”交叉校验。

- [ ] **步骤 4：统一读取单价**

```ts
function sourcePrice(record: SourceRecord): Decimal | null {
  return numericField(record, '单价 元')
}
```

成本构建继续先使用 override，否则使用 `sourcePrice(record)`；模式仍决定成本列、单位和计费事件，不参与选源字段。

- [ ] **步骤 5：运行测试确认 GREEN**

```powershell
Set-Location web
bun test --parallel=1 scripts/channel-model-template/__tests__/build.test.ts scripts/channel-model-template/__tests__/generate.test.ts
```

预期：全部通过。

- [ ] **步骤 6：运行模板与转换器 focused 回归**

```powershell
Set-Location web
bun test --parallel=1 scripts/channel-model-template/__tests__ src/channel-config-converter
bun run typecheck
bun run lint -- scripts/channel-model-template/source.ts scripts/channel-model-template/build.ts scripts/channel-model-template/types.ts scripts/channel-model-template/__tests__/source.test.ts scripts/channel-model-template/__tests__/build.test.ts scripts/channel-model-template/__tests__/generate.test.ts
```

预期：测试零失败、类型检查退出码 0、涉及文件无 lint error。若 `oxlint` 不支持尾随路径参数，改为执行项目既有 `bun run lint` 并记录完整结果。

- [ ] **步骤 7：提交构建迁移**

```powershell
git add -- web/scripts/channel-model-template/types.ts web/scripts/channel-model-template/build.ts web/scripts/channel-model-template/__tests__/build.test.ts web/scripts/channel-model-template/__tests__/generate.test.ts
git commit -m "fix: derive sd capabilities from latest source"
```

### 任务 5：更新项目维护文档和刷新技能

**文件：**

- 修改：`docs/channel-model-template-generator.md`
- 修改：`.codex/skills/refreshing-sd-channel-config/SKILL.md`
- 修改：`.codex/skills/refreshing-sd-channel-config/references/project-workflow.md`
- 校验：`.codex/skills/refreshing-sd-channel-config/agents/openai.yaml`

- [ ] **步骤 1：运行技能变更基线场景**

在修改技能前，用一个无泄漏上下文的子代理执行：

```text
使用 C:\Users\880pro\Documents\new-api\.codex\skills\refreshing-sd-channel-config\SKILL.md，说明最新版 sd 收录表校验时如何判定视频输入能力，以及系列字段如何参与 Seedance 2.0/2.5 的模板生成门禁。源表已删除“视频输入”，新增“系列”。不要修改文件。
```

记录基线是否仍要求读取 `视频输入`、是否遗漏 `参考视频数 > 0` 和系列感知匹配。

- [ ] **步骤 2：更新简体中文维护文档**

在 `docs/channel-model-template-generator.md` 明确：

- `系列` 是渠道模型与官方价格匹配维度；
- `计费方式 + 单价 元` 是唯一供应商成本来源；
- `参考视频数 > 0` 表示支持视频输入，0 表示不支持；
- 旧价格列和旧 `视频输入` 不再支持；
- 2.5 官方价格可先存在，只有渠道模型行加入后才生成对应活动配置。

- [ ] **步骤 3：更新刷新技能和项目流程合同**

在 `SKILL.md` 的源表校验、模板复核、停止条件和常见错误中加入最新版字段语义；在 `project-workflow.md` 的电子表格校验中明确检查 `系列`、统一单价和参考视频数推导。保持技能主体简洁，不复制大段生成器实现。

- [ ] **步骤 4：校验技能元数据和目录**

```powershell
& 'C:\Users\880pro\.cache\codex-runtimes\codex-primary-runtime\dependencies\python\python.exe' `
  'C:\Users\880pro\.codex\skills\.system\skill-creator\scripts\quick_validate.py' `
  'C:\Users\880pro\Documents\new-api\.codex\skills\refreshing-sd-channel-config'
```

检查 `agents/openai.yaml` 的显示名称、简述和默认提示仍与技能一致；只有内容过时时才用 `generate_openai_yaml.py` 重生成。

- [ ] **步骤 5：运行同一场景确认 GREEN**

用新的无泄漏上下文子代理重复步骤 1，要求明确输出：不再读取 `视频输入`；`参考视频数 > 0` 为支持；0 为不支持；系列参与官方价格匹配；2.5 无渠道行时不发布活动配置。

- [ ] **步骤 6：提交仓库可跟踪文档**

先运行 `git status --short`。`.codex` 若按仓库规则保持忽略，则不强制提交技能文件，只在验收报告中记录绝对路径和校验结果；提交可跟踪的维护文档：

```powershell
git add -- docs/channel-model-template-generator.md
git commit -m "docs: document latest sd source contract"
```

### 任务 6：下载、验证并生成最新版模板

**文件：**

- 创建：`outputs/2026-08-10-sd-series-refresh/sd收录.xlsx`
- 创建：`outputs/2026-08-10-sd-series-refresh/渠道模型成本与利润模板-v1.xlsx`
- 创建：`outputs/2026-08-10-sd-series-refresh/渠道模型成本与利润模板-v1.report.json`
- 可能修改：`web/scripts/channel-model-template/conversion-rules.json`
- 最后修改：`docs/new-channels/sd收录.xlsx`

- [ ] **步骤 1：建立基线和唯一输出目录**

记录 `git status --short`、仓库源表 SHA-256、本地服务、当前发布/激活批次、成本模式、最低毛利率和分组倍率。若目标目录存在，改用 `outputs/2026-08-10-sd-series-refresh-2`，不得删除或覆盖旧目录。

- [ ] **步骤 2：通过已登录 Chrome 重新下载**

确认 Google 表格标题与 ID `1qnzFB8mmc4glK7Eo7xxulgNwipEbdmtKrgQcvdc0BUM`，使用“文件 -> 下载 -> Microsoft Excel (.xlsx)”。通过下载前后文件列表、时间和 SHA-256 确认新文件，再复制到本轮目录为 `sd收录.xlsx`。不读取其他标签页，不输出凭据。

- [ ] **步骤 3：使用电子表格技能完成源表门禁**

用 `@oai/artifact-tool` 检查 `channel`、`sd`、`sd官价` 的范围、表头、有效模型行、公式和值；扫描：

```text
#REF!|#DIV/0!|#VALUE!|#NAME?|#N/A
```

确认 `sd` 无 `视频输入` 和旧价格列，系列只分布为合法正数，模型行 `参考视频数` 是非负整数；渲染三个工作表并目视检查表头、代表行和尾部。

- [ ] **步骤 4：补齐显式渠道映射并验证**

若最新版 `channel` 包含规则文件尚未映射的渠道编号，按源表名称增加稳定代码，例如当前下载中的：

```json
{
  "13": "CH-MIKOTO",
  "14": "CH-ZZONE",
  "15": "CH-FFLINK",
  "16": "CH-AOTIAN"
}
```

不得根据显示名称覆盖已有代码。运行 `rules.test.ts` 和生成器预检；任何未知渠道保持 `FAIL`，不能用临时跳过绕过。

- [ ] **步骤 5：生成模板和报告**

```powershell
Set-Location web
bun run channel-model-template:generate -- `
  --source "..\outputs\2026-08-10-sd-series-refresh\sd收录.xlsx" `
  --rules "scripts\channel-model-template\conversion-rules.json" `
  --base "src\channel-config-converter\__fixtures__\channel-config-v1-corrected.xlsx" `
  --output "C:\Users\880pro\Documents\new-api\outputs\2026-08-10-sd-series-refresh\渠道模型成本与利润模板-v1.xlsx" `
  --report "C:\Users\880pro\Documents\new-api\outputs\2026-08-10-sd-series-refresh\渠道模型成本与利润模板-v1.report.json" `
  --allow-warnings
```

预期：退出码 0、报告 `FAIL=0`。逐项归类每个 `WARN` 和 `draft`；无法解释时停止。

- [ ] **步骤 6：校验生成模板**

用电子表格技能检查全部受管理工作表，扫描公式错误并渲染每个工作表。至少抽查一条按秒、一条按次、一条 Token 成本，以及正负毛利样本；按次成本必须保持整次金额。

- [ ] **步骤 7：更新仓库源表并提交配置事实**

只有步骤 3-6 全部通过后，才把本轮 `sd收录.xlsx` 更新到 `docs/new-channels/sd收录.xlsx`。重新计算 SHA-256，运行 focused tests，再提交源表和必要的规则文件：

```powershell
git add -- docs/new-channels/sd收录.xlsx web/scripts/channel-model-template/conversion-rules.json
git commit -m "chore: refresh sd channel source data"
```

### 任务 7：转换、导入、发布和激活

**文件：**

- 创建：`outputs/2026-08-10-sd-series-refresh/channel-config-v1.json`
- 创建：`outputs/2026-08-10-sd-series-refresh/channel-config-issues.json`

- [ ] **步骤 1：确认本地服务并打开转换器**

确认 new-api 后端和前端可访问；若没有前端服务，在 `web/` 使用 `bun run dev` 启动并记录实际端口。通过 Chrome 打开 `/config-import`，保持现有登录态。

- [ ] **步骤 2：转换模板并选择线路**

在“Excel 转换”上传已验证模板。默认纳入所有已存在、类型匹配且配置凭据的真实渠道；缺少实例、凭据或唯一匹配的线路使用 skip。绑定时核对类型、Base URL、分组和 Secure 的 `secure_video_group`，不读取或输出凭据内容。

- [ ] **步骤 3：导出选中 JSON 和问题报告**

下载“选中配置 JSON”为 `channel-config-v1.json`，保存问题报告为 `channel-config-issues.json`。核对 `manifest.source_sha256`、渠道、SKU、售价、成本、映射和策略计数；JSON 存在 error 时停止。

- [ ] **步骤 4：按状态机完成导入**

依次执行：导入 -> 绑定并暂存 -> 解决冲突并重新暂存 -> 定价审阅并重新暂存 -> 路由差异审阅 -> 校验 -> 发布 -> 激活。相同语义批次按 UI 提示复用审计结果或复制并重新绑定，不把重复导入当作成功的新批次。

- [ ] **步骤 5：验证激活和缓存**

只激活本批次拥有且预检通过的目标。记录批次 ID、状态、实体计数、绑定、退休和启用目标数；缓存状态不明确时调用现有刷新缓存步骤，不重复激活事务。

### 任务 8：执行 Mock E2E 并生成验收报告

**文件：**

- 可能修改：`e2e/testdata/channel-config-v1.json`
- 创建：`outputs/2026-08-10-sd-series-refresh/e2e.log`
- 创建：`outputs/2026-08-10-sd-series-refresh/验收报告.md`

- [ ] **步骤 1：比较并更新 E2E fixture**

比较导出的 `channel-config-v1.json` 与 `e2e/testdata/channel-config-v1.json` 的实体计数和语义差异。只有本轮 E2E 需要新配置且差异已审阅时才更新仓库 fixture；不得把下载物中的凭据带入 fixture。

- [ ] **步骤 2：运行 Mock Ark SDK 素材矩阵 E2E**

```powershell
go test ./e2e -run '^TestSeedanceImportedMaterialMatrixFullFlowE2E$' -count=1 -p=1
go test ./cmd/ark-video-material-seed ./e2e -count=1 -p=1
```

把命令、退出码和关键统计写入 `e2e.log`。失败时记录首个失败目标和阶段，停止后续步骤，不运行真实供应商 Canary。

- [ ] **步骤 3：执行最终回归**

```powershell
Set-Location web
bun test --parallel=1 scripts/channel-model-template/__tests__ src/channel-config-converter
bun run typecheck
Set-Location ..
git diff --check
git status --short
```

只有最新完整输出显示零失败，才能声明通过。

- [ ] **步骤 4：生成简体中文验收报告**

报告必须包含：源表、模板、报告和选中 JSON 的绝对路径与 SHA-256；各工作表行数；`FAIL/WARN/draft` 数和处理决定；批次、绑定、发布、激活、缓存结果；E2E 命令、退出码、任务/日志/成本/利润结果；运行设置恢复、未覆盖项和下一步。明确标注本轮为 Mock 上游验收。

- [ ] **步骤 5：提交必要的可跟踪 E2E fixture**

若步骤 1 确认需要更新 fixture：

```powershell
git add -- e2e/testdata/channel-config-v1.json
git commit -m "test: refresh imported channel config fixture"
```

输出目录、下载物、运行日志和凭据不提交。完成前使用 `superpowers:verification-before-completion` 逐项核对本计划与设计文档。
