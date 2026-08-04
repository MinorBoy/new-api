# Ark 视频合同阻断根治实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 根治 Ark SDK 视频矩阵的 36 条合同阻断，重生成并导入渠道模板，使完整 E2E 的活动路由合同阻断为 0。

**Architecture:** CLMM 适配器删除基础模型前缀白名单，统一解析模型尾部控制段并让路由合同复用同一分析结果。模板生成器把时长从公共 SKU 下沉到渠道模型映射，公共 SKU 仅聚合总体能力；源表中与已验证供应商协议冲突的行直接修正或删除，并在生成阶段增加合同门禁。

**Tech Stack:** Go 1.22、Testify、React/TypeScript、Bun、ExcelJS、`@oai/artifact-tool`、Docker、本地 MySQL、Ark SDK 视频矩阵种子。

---

## 文件结构

- Modify: `relay/channel/task/clmmmall/translate.go`：统一解析 CLMM 基础模型和尾部控制段，接受真实模型 ID 和普通 4 秒时长。
- Modify: `relay/channel/task/clmmmall/translate_test.go`：覆盖真实模型、空格模型 ID、控制段和普通时长边界。
- Modify: `relay/channel/task/clmmmall/billing_test.go`：覆盖提交转换后的时长与计费秒数。
- Modify: `relay/video_route_contract.go`：路由合同复用 CLMM 模型分析结果，删除重复控制判断。
- Modify: `relay/video_route_contract_test.go`：覆盖真实 CLMM 模型和 4 秒 route target。
- Modify: `web/scripts/channel-model-template/build.ts`：修正单值时长解析、SKU 聚合和映射级时长保存。
- Modify: `web/scripts/channel-model-template/types.ts`：为映射记录增加结构化时长字段。
- Modify: `web/scripts/channel-model-template/write.ts`：在“模型映射”工作表写入时长列并执行合同门禁。
- Modify: `web/scripts/channel-model-template/__tests__/build.test.ts`：覆盖单值时长、SKU 聚合和映射独立时长。
- Modify: `web/scripts/channel-model-template/__tests__/write.test.ts`：覆盖模型映射时长列和门禁输出。
- Modify: `web/src/channel-config-converter/workbook.ts`：扩展 V1 模型映射表头。
- Modify: `web/src/channel-config-converter/adapters/v1.ts`：读取映射级时长。
- Modify: `web/src/channel-config-converter/document.ts`：route target 从 mapping 而不是 SKU 读取时长。
- Modify: `web/src/channel-config-converter/__tests__/v1.test.ts`：覆盖 Secure `5-15` 与公共 SKU `4-15` 并存。
- Modify: `docs/new-channels/sd收录.xlsx`：修正 Cangyuan、8yes 并删除 MegaByAI 冲突行。
- Create: `outputs/2026-08-04-contract-root-cure/渠道模型成本与利润模板-v1.xlsx`：根治后模板。
- Modify: `e2e/testdata/channel-config-v1.json`：根治后导入配置。
- Modify: `docs/superpowers/reports/2026-08-04-ark-sdk-video-material-matrix-reimport-acceptance.md`：记录根因、重导入与 E2E 结果。

### Task 1: 用失败测试固定 CLMM 真实模型合同

**Files:**
- Test: `relay/channel/task/clmmmall/translate_test.go`
- Test: `relay/video_route_contract_test.go`

- [ ] **Step 1: 添加真实模型 ID 的失败测试**

在 `translate_test.go` 添加表驱动测试，调用公开的 `ValidateRouteModel`：

```go
func TestValidateRouteModelAcceptsConfiguredCLMMModelIDs(t *testing.T) {
	for _, modelName := range []string{
		"mg-seedance-1-5-pro",
		"ov-Seedance 1.0 Pro-720p",
		"seedance2.0-preview-10s",
	} {
		t.Run(modelName, func(t *testing.T) {
			require.NoError(t, ValidateRouteModel(modelName))
		})
	}
}
```

- [ ] **Step 2: 添加普通 4 秒和空格模型控制段失败测试**

构造 Ark 请求并调用 `arkToClmm`，断言普通模型显式 4 秒成功，带空格基础 ID 的 `-720p-10s` 控制段可被识别，3 秒和 16 秒失败。

- [ ] **Step 3: 添加 route target 合同失败测试**

在 `video_route_contract_test.go` 构造 CLMM target：真实上游模型、`480p/720p`、`4-15` 秒、合法素材边界，断言 `ValidateVideoRouteTargetContract` 成功。

- [ ] **Step 4: 运行测试确认红灯**

```powershell
go test ./relay/channel/task/clmmmall ./relay -run 'Test.*CLMM|TestValidateRouteModelAcceptsConfiguredCLMMModelIDs' -count=1 -p=1
```

Expected: 真实模型因前缀白名单失败，普通 4 秒因现有 `5-15` 限制失败。

### Task 2: 实现 CLMM 模型分析和合同复用

**Files:**
- Modify: `relay/channel/task/clmmmall/translate.go`
- Modify: `relay/video_route_contract.go`
- Test: `relay/channel/task/clmmmall/translate_test.go`
- Test: `relay/channel/task/clmmmall/billing_test.go`

- [ ] **Step 1: 删除基础模型前缀白名单**

`parseModelControls` 必须接受任意非空基础模型 ID，只从末尾消费完整控制段。解析结束后基础模型为空时返回错误。

- [ ] **Step 2: 导出稳定的模型分析结果**

新增供路由合同复用的查询函数，例如：

```go
type RouteModelContract struct {
	ControlsDuration bool
}

func AnalyzeRouteModel(modelName string) (RouteModelContract, error)
```

函数内部调用与 `arkToClmm` 相同的解析器，不复制正则或控制段判断。

- [ ] **Step 3: 普通模型接受 4 至 15 秒**

未携带时长控制段时，默认保持 5 秒；显式时长允许 4 至 15 秒，并继续受 `relaycommon.MaxTaskDurationSeconds` 上限保护。

- [ ] **Step 4: 路由合同复用分析结果**

删除 `clmmModelControlsDuration`，在 CLMM 合同校验中调用 `AnalyzeRouteModel`。普通模型 route 时长范围允许 `4-15`，受控模型跳过普通时长范围判断。

- [ ] **Step 5: 运行定向测试确认绿灯**

```powershell
go test ./relay/channel/task/clmmmall ./relay -count=1 -p=1
```

Expected: PASS。

### Task 3: 用失败测试固定渠道级时长

**Files:**
- Test: `web/scripts/channel-model-template/__tests__/build.test.ts`
- Test: `web/scripts/channel-model-template/__tests__/write.test.ts`
- Test: `web/src/channel-config-converter/__tests__/v1.test.ts`

- [ ] **Step 1: 添加单值时长测试**

构造 `时长范围: "15"` 的来源行，断言 SKU 和映射的最小、最大时长都为 15。

- [ ] **Step 2: 添加公共 SKU 聚合与映射独立时长测试**

构造两个相同 `canonical model + version + resolution` 的来源行，一个渠道 `4-15`，另一个渠道 `5-15`。断言公共 SKU 为 `4-15`，两条映射分别保留 `4-15` 和 `5-15`。

- [ ] **Step 3: 添加工作表列测试**

断言“模型映射”包含 `最小时长秒`、`最大时长秒`，并写入对应映射值。

- [ ] **Step 4: 添加转换器测试**

构造公共 SKU `4-15`、Secure 映射 `5-15` 的 V1 工作簿，断言输出 route target 为 `5-15`。

- [ ] **Step 5: 运行测试确认红灯**

```powershell
cd web
bun test --parallel=1 scripts/channel-model-template/__tests__/build.test.ts scripts/channel-model-template/__tests__/write.test.ts src/channel-config-converter/__tests__/v1.test.ts
```

Expected: 单值时长、映射列或 Secure route 时长断言失败。

### Task 4: 实现映射级时长和生成门禁

**Files:**
- Modify: `web/scripts/channel-model-template/build.ts`
- Modify: `web/scripts/channel-model-template/types.ts`
- Modify: `web/scripts/channel-model-template/write.ts`
- Modify: `web/src/channel-config-converter/workbook.ts`
- Modify: `web/src/channel-config-converter/adapters/v1.ts`
- Modify: `web/src/channel-config-converter/document.ts`

- [ ] **Step 1: 修正 `parseDuration`**

单个合法整数返回 `[value, value]`；范围仍返回排序后的两个边界；缺失值使用现有明确默认，不猜测第二个边界。

- [ ] **Step 2: 聚合公共 SKU 总体能力**

遇到已有 SKU 时更新其 `durationMin = min(existing, current)`、`durationMax = max(existing, current)`，不得直接 `continue` 丢弃来源能力。

- [ ] **Step 3: 保存和写出映射级时长**

映射记录携带当前来源行的 `durationMin`、`durationMax`；“模型映射”表头和行数据写入 `最小时长秒`、`最大时长秒`。

- [ ] **Step 4: 转换器读取映射级时长**

V1 表头加入两个字段。生成 route target 时使用 mapping 的结构化时长；删除从 SKU 复制渠道时长的逻辑。

- [ ] **Step 5: 增加合同门禁**

生成器根据已验证渠道合同检查分辨率、时长、模型语法和素材边界。合同冲突写入 error，`--allow-warnings` 不得降级 error。

- [ ] **Step 6: 运行前端定向测试确认绿灯**

```powershell
cd web
bun test --parallel=1 scripts/channel-model-template src/channel-config-converter
```

Expected: PASS。

### Task 5: 修正并核验 SD 收录源表

**Files:**
- Modify: `docs/new-channels/sd收录.xlsx`

- [ ] **Step 1: 使用 spreadsheet artifact 工具加载源表**

读取 `sd` 工作表的表头、目标行、公式和样式，禁止使用文本重写破坏工作簿格式。

- [ ] **Step 2: 修正 Cangyuan 12 行**

将已确认只支持文本的 12 行素材码改为 `000`，输入模式改为 `text`，清除与媒体素材有关但无协议证据的能力字段。

- [ ] **Step 3: 修正 8yes 冲突行**

将 `sd!64` 修正为供应商证据一致的 `videos-mini-480p + 480p` 组合。

- [ ] **Step 4: 删除 MegaByAI 4 行**

删除 `sd!108`、`sd!109`、`sd!124`、`sd!125` 对应的 `1080p/4k` 价格和能力，不保留禁用副本。

- [ ] **Step 5: 渲染并核验**

重新读取目标区域，确认公式、合并单元格、样式和其他渠道行未发生非预期变化。

### Task 6: 重生成模板和导入配置

**Files:**
- Create: `outputs/2026-08-04-contract-root-cure/渠道模型成本与利润模板-v1.xlsx`
- Modify: `e2e/testdata/channel-config-v1.json`

- [ ] **Step 1: 生成模板**

```powershell
cd web
bun run channel-model-template:generate -- `
  --source "..\docs\new-channels\sd收录.xlsx" `
  --rules "scripts\channel-model-template\conversion-rules.json" `
  --base "src\channel-config-converter\__fixtures__\channel-config-v1-corrected.xlsx" `
  --output "C:\Users\880pro\Documents\new-api\outputs\2026-08-04-contract-root-cure\渠道模型成本与利润模板-v1.xlsx" `
  --allow-warnings
```

Expected: 无合同 error，允许的非合同 warning 明确列出。

- [ ] **Step 2: 转换为 JSON 配置**

使用项目既有 CLI 将新模板转换为 `e2e/testdata/channel-config-v1.json`，确认 `hasFailures=false`。

- [ ] **Step 3: 静态合同核验**

对 JSON 中全部活动 route target 调用后端合同校验，预期 0 阻断；重点核对 CLMM 18 条和 Secure 企业组 `5-15`。

### Task 7: 导入本地环境并重跑 E2E

**Files:**
- Modify: `docs/superpowers/reports/2026-08-04-ark-sdk-video-material-matrix-reimport-acceptance.md`

- [ ] **Step 1: 重建并检查服务**

```powershell
docker compose -f docker-compose.local.yml up -d --build
docker compose -f docker-compose.local.yml ps
curl.exe -sS -o NUL -w "%{http_code}" http://127.0.0.1:3000/api/status
```

Expected: 所有必需容器 healthy，状态接口返回 `200`。

- [ ] **Step 2: 导入新模板配置**

通过既有导入 API 或 UI 导入根治后的 JSON，删除或替换旧渠道、模型映射和价格，避免保留冲突规则。

- [ ] **Step 3: 执行完整 Ark 视频矩阵**

```powershell
docker exec -w /data new-api-local-new-api-1 /data/ark-video-material-seed
```

Expected: 活动目标合同阻断为 0；全部应执行用例形成任务、使用日志和成本核算数据。

- [ ] **Step 4: 核对控制台数据**

核对 `/usage-logs/common`、`/usage-logs/task`、`/cost-accounting` 的请求 ID、渠道、模型、终态用户响应、供应商成本、用户收入、毛利润和尝试时间线可相互追溯。

- [ ] **Step 5: 更新验收报告**

记录源表修正、模板计数、导入计数、E2E 结果、合同阻断 36 到 0、任务终态完整率、日志和成本核算抽样。

### Task 8: 回归验证并提交

**Files:**
- Modify: 本计划涉及的全部文件

- [ ] **Step 1: 后端回归**

```powershell
go test ./relay/channel/task/clmmmall ./relay ./cmd/ark-video-material-seed -count=1 -p=1
```

- [ ] **Step 2: 前端回归**

```powershell
cd web
bun test --parallel=1 scripts/channel-model-template src/channel-config-converter
bun run typecheck
bun run build
```

- [ ] **Step 3: 最终一致性检查**

```powershell
git diff --check
git status --short
```

- [ ] **Step 4: 提交全部改动**

```powershell
git add relay web docs/new-channels/sd收录.xlsx docs/superpowers outputs/2026-08-04-contract-root-cure e2e/testdata/channel-config-v1.json
git commit -m "根治 Ark 视频渠道合同阻断"
```

Expected: 提交成功，工作区干净。

