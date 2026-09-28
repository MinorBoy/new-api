# Chrome DevTools MCP 渠道模型同步编排器实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (- [ ]) syntax for tracking.

**Goal:** 使用 Chrome DevTools MCP 的 --autoConnect 接管本机已经打开的 Chrome 默认 profile，按固定顺序识别 20 个渠道标签页并执行对应同步 skill，生成可审计的模型数据补丁；写入、发布、激活和 E2E 验证保持分阶段、可暂停、可恢复。

**Architecture:** 新增一个编排 skill 作为唯一入口，使用渠道 manifest 将渠道、skill、来源标签页特征、写入范围和校验阈值绑定起来；每个已有 sync-*-models skill 通过独立 adapter 接入，不把 skill 名称当作可盲目循环调用的函数。运行时采用 preflight → page registry → precheck → collect → normalize → validate → dry-run diff → user confirmation → apply → verify → checkpoint 状态机，单渠道失败隔离，支持 resume、retry-failed 和 only。

**Tech Stack:** Chrome DevTools MCP、--autoConnect、pageId 路由、Codex SKILL.md、YAML manifest、Bun/Node 校验脚本、现有 .codex/skills/sync-*-models/、Google Sheets 已登录标签页、sync-channel-models-contract.md、outputs/YYYY-MM-DD-* 审计产物。

---

## 事实边界与非目标

- 仓库已有 20 个渠道同步 skill：4stoken、8yes、aotian、apiaw、banliapi、cangyuansuanli、clmm、datacodex、dimensio、fflink、lucen、megabyai、mikoto、omegaai、paipu、secure、uniart、wxart、z5、zzone。
- 统一数据契约位于 .codex/skills/sync-channel-models-contract.md；它要求数字渠道 ID、精确模型身份键、原始响应、标准化快照、补丁、报告、写入前确认和写入后回读。
- 4stoken、fflink、wxart 依赖已登录 HTML 标签页；z5、zzone 使用公开 pricing API；其余渠道以各自 skill 的权威来源为准。
- 不自动点击 Chrome 原生远程调试授权弹窗，不读取 Cookie、Token、API Key、localStorage、sessionStorage 或密码，不抓取前端 bundle，不通过未公开接口绕过页面。
- 模型同步、Google 表格更新、配置导入、发布/激活和 Ark SDK E2E 不合并为一次不可逆操作；refreshing-sd-channel-config 的发布链路必须单独触发并人工确认。

## 文件结构

- Create: .codex/skills/sync-channel-orchestrator/SKILL.md：编排入口、状态机、MCP 前置检查、确认门禁和恢复命令。
- Create: .codex/skills/sync-channel-orchestrator/agents/openai.yaml：Agent 触发描述。
- Create: .codex/skills/sync-channel-orchestrator/channel-manifest.yaml：20 个渠道的顺序、skill、来源模式、标签页匹配、写入范围和阈值。
- Create: .codex/skills/sync-channel-orchestrator/adapters/*.yaml：每个已有同步 skill 一个 adapter。
- Create: .codex/skills/sync-channel-orchestrator/references/run-contract.md：运行记录、checkpoint、脱敏日志和状态转换契约。
- Create: .codex/skills/sync-channel-orchestrator/scripts/validate-manifest.mjs：manifest、adapter 和 run record 静态校验器。
- Create: .codex/skills/sync-channel-orchestrator/scripts/run-ledger.mjs：批次 checkpoint、resume、retry-failed、only 规划器。
- Create: .codex/skills/sync-channel-orchestrator/tests/manifest.test.mjs：确定性单元测试。
- Create: docs/channel/chrome-devtools-mcp-channel-sync.md：Windows 配置和操作手册。
- Create: docs/channel/chrome-devtools-mcp-channel-sync-runbook.md：一次批次的执行清单和人工确认点。
- Modify: 20 个已有 SKILL.md，仅在 adapter 契约需要时补充明确的输入/输出标记，不改写来源、字段映射和停止条件。

### Task 1: 固化运行契约与批次状态模型

**Files:**
- Create: .codex/skills/sync-channel-orchestrator/references/run-contract.md
- Test: .codex/skills/sync-channel-orchestrator/tests/manifest.test.mjs

- [ ] Step 1: 写失败测试，锁定 run record 的最小审计字段和敏感字段拒绝规则。

```js
import { test, expect } from "bun:test";
import { validateRunRecord } from "../scripts/validate-manifest.mjs";

test("run record rejects secrets and requires checkpoint fields", () => {
  const result = validateRunRecord({
    runId: "20260920-001",
    status: "running",
    currentOrder: 1,
    source: { url: "https://provider.example/pricing", cookies: "secret" },
  });
  expect(result.ok).toBe(false);
  expect(result.errors).toContain("forbidden secret field");
});
```

- [ ] Step 2: 运行 `bun test .codex/skills/sync-channel-orchestrator/tests/manifest.test.mjs`，预期因校验器尚不存在而 FAIL。
- [ ] Step 3: 在 run-contract.md 定义固定状态：run_status 为 preflight、page_registry、running、awaiting_confirmation、blocked、paused、completed、failed；channel_status 为 pending、ready、running、awaiting_confirmation、success、skipped、blocked、failed；resume_modes 为 resume、retry-failed、only。
- [ ] Step 4: 定义每次运行必须产生 run.json、page-registry.json、raw-source.json、normalized-snapshot.json、sd-update-patch.json、sd-update-report.md、verify-result.json；禁止字段为 cookie、cookies、authorization、api_key、access_token、refresh_token、password、local_storage、session_storage。
- [ ] Step 5: 重新运行测试，预期状态字段和敏感字段测试 PASS。
- [ ] Step 6: 提交：`git add .codex/skills/sync-channel-orchestrator/references/run-contract.md .codex/skills/sync-channel-orchestrator/tests/manifest.test.mjs`；`git commit -m "docs: define channel sync run contract"`。

### Task 2: 建立 20 渠道 manifest 与 adapter 注册表

**Files:**
- Create: .codex/skills/sync-channel-orchestrator/channel-manifest.yaml
- Create: .codex/skills/sync-channel-orchestrator/adapters/4stoken.yaml、8yes.yaml、aotian.yaml、apiaw.yaml、banliapi.yaml、cangyuansuanli.yaml、clmm.yaml、datacodex.yaml、dimensio.yaml、fflink.yaml、lucen.yaml、megabyai.yaml、mikoto.yaml、omegaai.yaml、paipu.yaml、secure.yaml、uniart.yaml、wxart.yaml、z5.yaml、zzone.yaml
- Modify: .codex/skills/sync-channel-orchestrator/tests/manifest.test.mjs

- [ ] Step 1: 写 manifest 完整性测试，断言恰好 20 个条目、order 连续为 1 到 20、channel 唯一、skillPath 和 adapterPath 存在、writeScope 只能是 sd:<channel>。
- [ ] Step 2: 写 manifest 顶层配置：version=1、defaultMode=serial、requireUserConfirmationBeforeApply=true、stopOnUnmatchedPage=true、stopOnSchemaDrift=true、maxRetriesPerChannel=1。
- [ ] Step 3: 为每个条目填写 order、channel、skillPath、adapterPath、sourceMode、pageMatch、writeScope、minModels、anomalyThresholds；未知 URL 不得猜测，使用从 channel 工作表读取权威 URL 的 adapter preflight。
- [ ] Step 4: 为 4stoken、fflink、wxart 标记 sourceMode=authenticated-html；为 z5、zzone 标记 sourceMode=public-pricing-api；其余按各自 SKILL.md 的权威来源标记。
- [ ] Step 5: 每个 adapter 定义 input.pageId、input.sourceUrl、input.channelId、input.domSnapshot/rawResponse，以及 output.required、output.writeScope、output.forbiddenWrites、precheck；禁止跨渠道 pageId、channel 财务字段、路由、发布和激活。
- [ ] Step 6: 运行 `bun test .codex/skills/sync-channel-orchestrator/tests/manifest.test.mjs`，预期 20 条注册校验 PASS。
- [ ] Step 7: 提交：`git add .codex/skills/sync-channel-orchestrator/channel-manifest.yaml .codex/skills/sync-channel-orchestrator/adapters .codex/skills/sync-channel-orchestrator/tests/manifest.test.mjs`；`git commit -m "feat: register channel sync adapters"`。

### Task 3: 实现 manifest、adapter 和 run record 校验器

**Files:**
- Create: .codex/skills/sync-channel-orchestrator/scripts/validate-manifest.mjs
- Modify: .codex/skills/sync-channel-orchestrator/tests/manifest.test.mjs

- [ ] Step 1: 写失败测试，覆盖重复 order、未知 skill、非法 writeScope、缺少 pageMatch、敏感字段和缺少 artifacts。
- [ ] Step 2: 实现纯函数 `validateManifest(manifest, { fileExists })` 和 `validateRunRecord(record)`，不发网络请求、不访问浏览器机密；校验 order 唯一、skill/adapter 文件存在、writeScope 匹配 `/^sd:[a-z0-9-]+$/`、来源模式为受控枚举。
- [ ] Step 3: 让 validateRunRecord 扫描序列化记录，发现 cookie、authorization、api_key、access_token、refresh_token、password、local_storage、session_storage 即失败；要求 runId、status、currentOrder、artifacts。
- [ ] Step 4: 运行 `bun test .codex/skills/sync-channel-orchestrator/tests/manifest.test.mjs`，预期全部 PASS。
- [ ] Step 5: 提交：`git add .codex/skills/sync-channel-orchestrator/scripts/validate-manifest.mjs .codex/skills/sync-channel-orchestrator/tests/manifest.test.mjs`；`git commit -m "feat: validate channel sync manifests"`。

### Task 4: 编写编排 skill 与 MCP/Chrome 前置流程

**Files:**
- Create: .codex/skills/sync-channel-orchestrator/SKILL.md
- Create: .codex/skills/sync-channel-orchestrator/agents/openai.yaml
- Modify: 20 个已有 .codex/skills/sync-*-models/SKILL.md，仅补充编排器输入输出锚点

- [ ] Step 1: 在 SKILL.md 固定 preflight：复用唯一 chrome-devtools MCP，参数为 `--autoConnect --page-id-routing`；若 Chrome 原生 Allow 未确认，返回 NEED_USER_APPROVAL，禁止 OCR、AutoHotkey、坐标点击或 CDP 绕过。
- [ ] Step 2: 固定 page registry：枚举 pageId、title、url、host 和可见状态；只按 manifest 的 host/url/title/DOM 特征唯一匹配；零匹配或多匹配均 blocked；不依赖 tab 顺序。
- [ ] Step 3: 固定单渠道流程：precheck → collect → normalize → validate → dry-run diff → 用户确认 → apply → verify → checkpoint；每次工具调用只使用该渠道 pageId。
- [ ] Step 4: 固定命令模式：run、resume RUN_ID、retry-failed RUN_ID、only channel1,channel2；默认串行；单渠道失败隔离；成功渠道不得被恢复动作重跑。
- [ ] Step 5: 写 openai.yaml：name=sync-channel-orchestrator；description 明确使用已登录 Chrome 标签页、生成审计补丁、不自动批准调试权限、不发布/激活配置；argument-hint 包含 run、resume、retry-failed、only。
- [ ] Step 6: 20 个已有 SKILL.md 增加统一“编排器输入输出”段：接收唯一 pageId、运行时数字 channel_id、权威来源 URL 和批次输出目录；输出原始来源、标准化快照、sd-update-patch.json、sd-update-report.md 和回读结果；保留原有确认门禁。
- [ ] Step 7: 运行 `rg -n "autoConnect|pageId|NEED_USER_APPROVAL|sd-update-patch|回读|resume|retry-failed|only" .codex/skills/sync-channel-orchestrator .codex/skills/sync-*-models/SKILL.md`，预期编排 skill 和 20 个渠道 skill 均出现对应门禁。
- [ ] Step 8: 提交：`git add .codex/skills/sync-channel-orchestrator .codex/skills/sync-*-models/SKILL.md`；`git commit -m "feat: add Chrome MCP channel sync orchestrator"`。

### Task 5: 实现批次 checkpoint、恢复和审计产物

**Files:**
- Create: .codex/skills/sync-channel-orchestrator/scripts/run-ledger.mjs
- Modify: .codex/skills/sync-channel-orchestrator/references/run-contract.md
- Modify: .codex/skills/sync-channel-orchestrator/tests/manifest.test.mjs

- [ ] Step 1: 写恢复行为测试：ledger 中 4stoken=success、8yes=failed、aotian=pending 时，resume 返回 8yes/aotian，retry-failed 只返回 8yes，only aotian 只返回 aotian。
- [ ] Step 2: 实现 `resumePlan(ledger, mode, only)`；按 order 排序；resume 从第一个非 success/skipped 开始；retry-failed 只取 failed/blocked；only 只取白名单渠道。
- [ ] Step 3: 实现 `appendCheckpoint(outputDir, event)`，采用追加事件而不是覆盖；拒绝敏感字段；每个渠道成功后保存 status.json、raw-source、normalized-snapshot、patch、report、verify-result。
- [ ] Step 4: 固定输出目录为 `outputs/YYYY-MM-DD-sync-channel-orchestrator/<runId>/<order>-<channel>/`；run.json 保存 manifest 版本、skill 版本、状态和错误摘要，不保存 Cookie/Token/Storage。
- [ ] Step 5: 运行 `bun test .codex/skills/sync-channel-orchestrator/tests/manifest.test.mjs`，预期恢复、敏感字段和重复执行保护全部 PASS。
- [ ] Step 6: 提交：`git add .codex/skills/sync-channel-orchestrator/scripts/run-ledger.mjs .codex/skills/sync-channel-orchestrator/references/run-contract.md .codex/skills/sync-channel-orchestrator/tests/manifest.test.mjs`；`git commit -m "feat: add resumable channel sync ledger"`。

### Task 6: 接入现有 sd 契约和在线表格写入门禁

**Files:**
- Create: docs/channel/chrome-devtools-mcp-channel-sync.md
- Create: docs/channel/chrome-devtools-mcp-channel-sync-runbook.md
- Modify: .codex/skills/sync-channel-orchestrator/SKILL.md
- Reference: .codex/skills/sync-channel-models-contract.md

- [ ] Step 1: 文档写 Windows 首次配置：打开主 Chrome 并登录渠道；打开 chrome://inspect/#remote-debugging；人工点击 Allow；启动唯一 MCP；枚举页面；不要新建匿名页替代已登录页。
- [ ] Step 2: 文档给出 MCP 配置，包含 command=npx、chrome-devtools-mcp@latest、--autoConnect、--page-id-routing、--usage-statistics=false、--redact-network-headers=true。
- [ ] Step 3: runbook 固定写入前：保存在线目标范围哈希、表头、数字渠道 ID、完整模型 ID 列表和 patch；展示差异并获取用户确认；未确认不得 apply。
- [ ] Step 4: 固定写入范围：只写模型 ID、系列、版本、清晰度、计费方式、单价、明确能力列和协议；保留人工列、公式列、其他渠道、channel 财务字段、路由和发布状态。
- [ ] Step 5: 固定写入后回读：渠道 ID、模型 ID、系列、版本、清晰度、计费方式、单价、协议、能力列、公式列和人工列；不一致即停止并保留旧值和证据。
- [ ] Step 6: 文档明确模型同步成功不等于配置发布；只有用户另行请求时才进入 refreshing-sd-channel-config 的导入、审阅、发布、激活和 Ark E2E，且每步独立确认。
- [ ] Step 7: 运行 `git diff --check -- docs/channel/chrome-devtools-mcp-channel-sync.md docs/channel/chrome-devtools-mcp-channel-sync-runbook.md`，预期无空白错误；文档新增内容使用简体中文。
- [ ] Step 8: 提交：`git add docs/channel/chrome-devtools-mcp-channel-sync.md docs/channel/chrome-devtools-mcp-channel-sync-runbook.md .codex/skills/sync-channel-orchestrator/SKILL.md`；`git commit -m "docs: add Chrome MCP channel sync runbook"`。

### Task 7: 三渠道试运行与故障注入验证

**Files:**
- Modify: docs/channel/chrome-devtools-mcp-channel-sync-runbook.md
- Create: outputs/2026-09-20-sync-channel-orchestrator/README.md，仅记录脱敏产物结构

- [ ] Step 1: 固定试运行集合为 4stoken、fflink、zzone，覆盖已认证 HTML、已认证模型计费页和公开 pricing API；首次禁止直接执行 20 个渠道。
- [ ] Step 2: 执行 `/goal run --only 4stoken,fflink,zzone --dry-run`；预期输出唯一 pageId、标题、URL、来源模式、渠道 ID 和写入范围；缺页或多匹配时为 blocked 且无 apply。
- [ ] Step 3: 执行 `/goal run --only 4stoken,fflink,zzone --dry-run --report`；预期生成原始来源、标准化快照、sd-update-patch.json、sd-update-report.md，并列出 proposed_update、proposed_add、draft、review_missing、ignored_non_sd 数量。
- [ ] Step 4: 关闭一个渠道标签页或使其跳转登录页，再运行同一批次；预期失败渠道为 blocked/failed，已成功渠道保留，后续渠道不盲写。
- [ ] Step 5: 执行 `/goal resume --run RUN_ID`；预期从第一个非 success 渠道继续，不重跑成功渠道。
- [ ] Step 6: 只在报告、完整模型 ID、目标行快照和差异核对后，由用户确认指定渠道 apply；回读不一致时停止，不进入发布链路。

### Task 8: 完整 20 渠道验收与交付

**Files:**
- Create: docs/acceptance/chrome-devtools-mcp-channel-sync.md
- Modify: docs/channel/chrome-devtools-mcp-channel-sync-runbook.md
- Modify: .codex/skills/sync-channel-orchestrator/channel-manifest.yaml，仅根据试运行事实修正匹配器和阈值

- [ ] Step 1: 执行 `/goal run --dry-run`；预期 20 个条目顺序连续，所有 skill/adapter 文件存在，page registry 唯一匹配，没有敏感字段写入计划。
- [ ] Step 2: 执行 `/goal run --dry-run --report`；模型数量为 0、重复模型 ID、端点未声明、价格单位不明、schema drift、匹配不唯一的渠道必须 blocked/draft，不产生正式写入。
- [ ] Step 3: 按用户确认拆成最多 5 个渠道一批，执行 `/goal apply --run RUN_ID --only channel-a,channel-b`；写入前重新读取在线目标范围哈希，哈希变化即停止；写入后完整回读并记录 verify-result。
- [ ] Step 4: 生成验收报告，至少包含 Chrome/MCP 版本、runId、manifest 版本、20 渠道状态、读取/匹配/更新/跳过数量、draft/review_missing/ignored_non_sd 数量、失败和恢复记录、确认记录、回读结果、未执行的发布/激活动作。
- [ ] Step 5: 运行 `bun test .codex/skills/sync-channel-orchestrator/tests/manifest.test.mjs`；运行 `git diff --check`；运行 `rg -n "cookie|authorization|api[_-]?key|access[_-]?token|refresh[_-]?token|password|local_storage|session_storage" outputs/2026-09-20-sync-channel-orchestrator --glob "*.json" --glob "*.md"`，预期测试 PASS、diff 无错误、敏感字段无命中。
- [ ] Step 6: 提交：`git add docs/acceptance/chrome-devtools-mcp-channel-sync.md docs/channel/chrome-devtools-mcp-channel-sync-runbook.md .codex/skills/sync-channel-orchestrator/channel-manifest.yaml`；`git commit -m "docs: accept Chrome MCP channel sync rollout"`。

## 交付验收门槛

1. MCP 只启动一个 `--autoConnect` 实例；原生 Allow 由用户完成，仓库无自动点击代码。
2. manifest 恰好包含 20 个有序渠道，每个渠道有存在的 skillPath、adapterPath 和限定为自身的 `sd:<channel>` 写入范围。
3. 每个渠道通过唯一 pageId 路由；缺页、多匹配、标题/URL 不符或登录失效会阻断该渠道。
4. 每个渠道生成原始来源、标准化快照、补丁、报告和回读结果；不记录 Cookie、Token、Storage 或密码。
5. 写入前展示差异并获得用户确认；写入后回读模型数据、公式和人工字段；异常时保留旧值并停止。
6. resume、retry-failed、only 有确定性测试，成功渠道不会被恢复动作重复执行。
7. 模型同步与配置导入/发布/激活/Ark E2E 分离，后者没有被隐式触发。
8. 三渠道试运行通过后才允许完整 20 渠道批次；完整批次按小批次 apply 并有最终验收报告。

## 风险与处理

- Chrome 原生授权弹窗：只允许人工确认；无法确认时返回 NEED_USER_APPROVAL，不执行页面写入。
- 主 Chrome profile 数据暴露：仅对信任 Agent 开启；manifest 限制来源和写入范围；日志脱敏。
- 标签页漂移：每次运行重新发现并唯一匹配，不依赖 tab 顺序；工具调用带 pageId。
- 页面结构变化：adapter precheck 失败即 blocked；不得通过猜 CSS、前端 bundle 或未公开 API 继续。
- 价格/模型大幅变化：触发阈值后只生成 draft，等待人工审核。
- Google 表格并发修改：写入前保存目标范围哈希，确认后重新读取；变化即停止。
- 单渠道失败扩大影响：串行执行、渠道级 checkpoint、恢复从失败点开始。
- 过早发布配置：编排器禁止 publish/activate；发布必须调用独立 skill 并重新确认。

## 计划自检

- [ ] 规格覆盖：MCP 授权、pageId 路由、20 个 skill、来源模式、标准化、补丁、确认、回读、checkpoint、resume/retry-failed/only、发布隔离均有对应任务。
- [ ] 占位符检查：计划不包含未定义的待办内容；未知渠道 URL 明确要求从 channel 工作表读取，不允许猜测。
- [ ] 类型一致性：manifest 使用 order/channel/skillPath/adapterPath/sourceMode/pageMatch/writeScope；ledger 使用 runId/status/currentOrder/artifacts；测试与任务中的字段一致。
- [ ] 文档语言：新增文档与计划内容使用简体中文，协议、命令、skill 名称和品牌原文保留。
- [ ] 安全检查：没有自动批准原生弹窗、读取敏感凭据或隐式发布的步骤。
