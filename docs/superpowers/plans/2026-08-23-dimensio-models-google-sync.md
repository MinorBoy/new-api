# Dimensio 模型同步实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 获取 Dimensio `/v1/models` 模型目录及其中的 `pricing` 价格详情，并通过项目专用 skill 安全更新 Google 表格的独立模型价格快照页。

**Architecture:** Python 标准库脚本负责 HTTP、JSON 校验和模型/价格快照输出；新增的 `sync-dimensio-sd-models` skill 负责浏览器表格确认、幂等写入和审计记录。第一版保留原始积分和价格范围，不直接覆盖 `sd` 主表；后续版本按模型 ID + 清晰度投影并审阅。

**价格单位：** Dimensio 使用积分，Google 表格使用元；固定按 `100 积分 = 1 元` 换算，`sd` 的 `单价 元` 使用十进制 `credits / 100`，快照仍保留积分原值。

**Tech Stack:** Python 3 标准库、`unittest`、Codex in-app Browser、Google Sheets UI、现有 skill validator。

---

### Task 1: 建立模型快照契约测试

**Files:**
- Create: `.codex/skills/refreshing-sd-channel-config/scripts/test_fetch_dimensio_models.py`
- Test: `.codex/skills/refreshing-sd-channel-config/scripts/test_fetch_dimensio_models.py`

- [ ] **Step 1: 写失败测试**

覆盖 `parse_models_payload` 的正常模型、重复 ID、缺失 ID、空数据和非法 JSON；覆盖 `fetch_models` 的 mock HTTP 非 2xx。

- [ ] **Step 2: 运行测试确认失败**

运行：`python .codex/skills/refreshing-sd-channel-config/scripts/test_fetch_dimensio_models.py`

预期：因 `fetch_dimensio_models.py` 尚不存在而失败。

### Task 2: 实现 Dimensio 模型快照脚本

**Files:**
- Create: `.codex/skills/refreshing-sd-channel-config/scripts/fetch_dimensio_models.py`

- [ ] **Step 1: 实现最小脚本**

提供 `parse_models_payload(payload)`, `fetch_models(base_url, timeout)`, `build_snapshot(models, source_url, fetched_at)` 和 CLI；使用 `urllib.request`、`json`，输出稳定排序的模型对象和元数据。

- [ ] **Step 2: 运行契约测试确认通过**

运行：`python .codex/skills/refreshing-sd-channel-config/scripts/test_fetch_dimensio_models.py`

预期：所有测试通过，且失败响应不会生成可写入快照。

- [ ] **Step 3: 用真实公开接口做只读 smoke check**

运行：`python .codex/skills/refreshing-sd-channel-config/scripts/fetch_dimensio_models.py --output outputs/2026-08-23-dimensio-models/models.json`

预期：只读请求成功时生成 JSON；网络不可用时明确报告失败且不触碰 Google 表格。

### Task 3: 创建项目专用 models-only skill

**Files:**
- Create: `.codex/skills/sync-dimensio-sd-models/SKILL.md`
- Create: `.codex/skills/sync-dimensio-sd-models/references/dimensio-models.md`
- Create: `.codex/skills/sync-dimensio-sd-models/agents/openai.yaml`

- [ ] **Step 1: 写明路由边界**

说明用户只要求模型目录时进入 `models-only`，从 `/v1/models` 同时保存 `pricing`，不调用其他价格接口、不修改 `sd收录.xlsx` 主表、不执行配置发布。

- [ ] **Step 2: 写明表格写入合同**

固定 Google 表格 ID，要求确认标题和目标工作表；按 `dimensio-models` 固定列幂等刷新，失败保留旧快照；记录源 URL、时间、哈希、数量和首尾行。

补充 `pricing_json` 原始结构约束，禁止把自动路由的价格范围压成单价。

补充价格单位约束：向 `sd` 投影时将积分按 `100:1` 换算为元，并在报告中记录原始值与换算值。

- [ ] **Step 3: 增加引用文档与 UI 元数据**

参考文档只保留字段、停止条件和审计格式；更新 `openai.yaml` 的短描述与默认提示以明确 `/v1/models` 模式。

### Task 4: 验证交付物

**Files:**
- Verify: `.codex/skills/refreshing-sd-channel-config`

- [ ] **Step 1: 运行 skill validator**

运行：`python C:/Users/880pro/.codex/skills/.system/skill-creator/scripts/quick_validate.py .codex/skills/sync-dimensio-sd-models`

- [ ] **Step 2: 检查差异和敏感信息**

运行：`git diff --check` 与 `rg -n "api[_-]?key|cookie|authorization|Bearer" .codex/skills/refreshing-sd-channel-config`，确认没有凭据和格式错误。

- [ ] **Step 3: 运行最终测试**

运行：`python .codex/skills/refreshing-sd-channel-config/scripts/test_fetch_dimensio_models.py`。
