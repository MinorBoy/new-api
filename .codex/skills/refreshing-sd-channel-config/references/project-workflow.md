# 项目流程合同

仅在执行 `refreshing-sd-channel-config` 时读取本文件。命令从仓库根目录 `C:\Users\880pro\Documents\new-api` 执行；实际项目路径不同时，使用当前仓库绝对路径替换，不硬编码用户目录。

## 1. 生成模板

在 `web/` 目录运行：

```powershell
bun run channel-model-template:generate -- `
  --source "..\outputs\YYYY-MM-DD-<run-name>\sd收录.xlsx" `
  --rules "scripts\channel-model-template\conversion-rules.json" `
  --base "src\channel-config-converter\__fixtures__\channel-config-v1-corrected.xlsx" `
  --output "<current-project-path>\outputs\YYYY-MM-DD-<run-name>\渠道模型成本与利润模板-v1.xlsx" `
  --report "<current-project-path>\outputs\YYYY-MM-DD-<run-name>\渠道模型成本与利润模板-v1.report.json" `
  --allow-warnings
```

仅在明确替换同一轮失败输出时使用 `--force`。`--allow-warnings` 只允许生成供审阅的 draft，不允许忽略问题或自动激活 draft。

Focused tests：

```powershell
Set-Location web
bun test --parallel=1 scripts/channel-model-template/__tests__ src/channel-config-converter
```

## 2. 电子表格校验

调用 `codex_app__load_workspace_dependencies` 获取电子表格运行时，然后使用 `spreadsheets:Spreadsheets` 要求的 `artifact_tool` 流程：

1. 检查源表 `channel`、`sd`、`sd官价` 的表头、有效范围、公式和值：
   - `sd` 必须包含 `系列`、`计费方式`、`单价 元`，且不得包含 `元/秒`、`元/次`、`元/1M`、`视频输入`；
   - `sd` 与 `sd官价` 的有效业务行必须具有合法正数 `系列`；只填写系列的分组行不计入渠道模型；
   - `参考视频数` 必须为非负整数，`> 0` 推导为支持视频输入，`= 0` 推导为不支持；
   - 官方价格按“系列 + 模型 + 分辨率”匹配；只有 `sd` 存在同系列渠道模型行时才允许生成对应活动配置。
2. 检查模板全部受管理工作表：`使用说明`、`参数`、`渠道`、`模型SKU`、`官方售价`、`渠道成本`、`模型映射`、`利润测算`、`来源`、`校验`。
3. 使用正则扫描 `#REF!|#DIV/0!|#VALUE!|#NAME\?|#N/A`。
4. 渲染每个工作表；大表可分段渲染，但必须覆盖表头、代表性数据和尾部。
5. 抽查公式的显示值和依赖输入，不只检查公式字符串。

利润抽查最少选择：一条按秒成本、一条按次成本、一条 Token 成本、一个负毛利样本、一个正毛利样本。按次成本必须保持整次金额。

## 3. 配置导入状态机

本地入口：`http://localhost:3000/config-import`。

按页面状态执行：

```text
Excel 转换与线路选择
-> 导出选中配置 JSON
-> 导入选中配置
-> 绑定真实渠道并暂存
-> 解决冲突并重新暂存
-> 定价审阅并重新暂存
-> 路由差异审阅
-> 校验
-> 发布
-> 激活
-> 必要时刷新缓存
```

对应 API 位于 `/api/config-import/batches`：

```text
POST   /batches
PUT    /batches/:id/bindings
PUT    /batches/:id/resolutions
PUT    /batches/:id/pricing-review
PUT    /batches/:id/route-reviews
POST   /batches/:id/stage
POST   /batches/:id/validate
POST   /batches/:id/publish
POST   /batches/:id/activate
POST   /batches/:id/refresh-cache
```

优先使用页面和现有登录态；不要从其他标签页提取 Cookie 或把管理会话写入脚本。相同语义 JSON 已存在批次时，复用审计结果或显式“复制并重新绑定”，不要误认为是新上传失败。

绑定检查：

- `line_ref` 唯一；
- 渠道类型与导入定义一致；
- Base URL 与供应商线路一致；
- 渠道已配置非空凭据，但不读取或输出凭据内容；
- Secure 线路的 `secure_video_group` 一致；
- 缺少实例的线路使用 skip，并写入验收报告。

## 4. E2E 数据与命令

在 `/config-import` 的 Excel 转换步骤下载“选中配置 JSON”，保存为：

```text
outputs/YYYY-MM-DD-<run-name>/channel-config-v1.json
```

确认 `manifest.source_sha256` 对应本轮模板，并核对实体计数。只有确认本轮 E2E 需要使用该配置时，才更新 `e2e/testdata/channel-config-v1.json`。

隔离数据库的 Mock 上游 E2E：

```powershell
go test ./e2e -run '^TestSeedanceImportedMaterialMatrixFullFlowE2E$' -count=1 -p=1
```

完整 focused 回归：

```powershell
go test ./cmd/ark-video-material-seed ./e2e -count=1 -p=1
```

需要把 Mock 全链路结果写入本地运行库和管理日志时，先确认本地容器健康并定位 new-api 服务容器，再执行容器内既有 `ark-video-material-seed`。不要硬编码容器名。先保存成本核算模式、最低毛利率和分组倍率，完成或失败后恢复并通过 API/UI 复核。

真实上游 Canary 不属于默认 E2E。只有用户明确授权后执行，并遵循：固定目标、低成本参数、串行、无自动切换、记录供应商任务 ID、跳过已有真实成功证据。

## 5. 验收证据

每轮至少保留：

| 文件 | 内容 |
| --- | --- |
| `sd收录.xlsx` | 下载的权威源表副本 |
| `渠道模型成本与利润模板-v1.xlsx` | 生成模板 |
| `渠道模型成本与利润模板-v1.report.json` | 转换问题与计数 |
| `channel-config-v1.json` | 实际选中并导入的语义配置 |
| `channel-config-issues.json` | 转换器问题报告；没有问题时也记录空数组 |
| `e2e.log` | 命令、退出码和关键统计 |
| `验收报告.md` | 下载、生成、导入、激活、E2E 和恢复结论 |

报告中的金额统一注明货币或 USD 等值；毛利率按汇总金额计算，不能平均单行毛利率。Mock 上游结果不得写成真实供应商验收通过。

## 6. 失败恢复

- 下载失败：保留旧仓库源表，不生成或发布。
- 生成 `FAIL`：修复源表事实、规则或生成器；不得直接编辑输出模板绕过。
- 导入阻断：批次保留作审计，修复后创建新批次或按状态机恢复。
- 发布失败：不要激活；记录失败审计。
- 激活成功但缓存失败：调用 `refresh-cache`，不要重复激活事务。
- E2E 失败：恢复运行时设置，保留失败日志，停止真实上游步骤。
