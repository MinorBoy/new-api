---
name: sync-dimensio-sd-models
description: 使用 Dimensio 公开的 /v1/models 模型目录及其中的 pricing 价格详情，校验并更新指定 sd收录 Google 表格的模型快照；第一版只维护快照，不直接发布配置。
---

# 同步 Dimensio 模型目录

将 Dimensio 当前公开模型目录和平台基础价格写入项目指定的 `sd收录` Google 表格，形成后续渠道模型和价格同步可复用的稳定快照。

## 适用边界

- 只请求 `https://jimeng.dimensio.cn/v1/models`，该接口无需 API Key；接口返回的 `pricing` 就是平台基础价格，必须原样保留。
- 第一版只更新 `dimensio-models` 工作表；不得直接修改 `sd`、`sd官价` 或其他工作表的价格、计费方式、素材能力、官方售价和渠道配置。
- 不调用其他价格接口，不把价格范围压成单一价格，不从模型名称猜测缺失字段，不导入、发布或激活 new-api 配置。
- 目标表格写入是外部状态变更，必须在写入前展示模型数量、模型 ID 列表和源 URL，写入后重新读取并核对。

## 固定资源

- Google 表格 ID：`1qnzFB8mmc4glK7Eo7xxulgNwipEbdmtKrgQcvdc0BUM`
- Google 表格 URL：`https://docs.google.com/spreadsheets/d/1qnzFB8mmc4glK7Eo7xxulgNwipEbdmtKrgQcvdc0BUM/edit`
- 模型接口：`https://jimeng.dimensio.cn/v1/models`
- 抓取脚本：`../refreshing-sd-channel-config/scripts/fetch_dimensio_models.py`
- 表格列约定：[references/dimensio-models.md](references/dimensio-models.md)

## 价格单位

- Dimensio 价格接口使用积分，固定换算为 `100 积分 = 1 元`，即 `price_yuan = credits / 100`。
- `dimensio-models` 的 `pricing_json` 保留接口原始积分；向 `sd` 投影时，`单价 元` 必须写换算后的元值，不得把积分数直接写入。
- 使用十进制换算，避免二进制浮点误差；例如 `48` 积分写为 `0.48` 元，`199` 积分写为 `1.99` 元。
- `per_second_range` 必须同时换算并保留 `min_yuan`、`max_yuan`；在 `sd` 没有价格范围字段或选价策略未确认时，不自动选择其中一个覆盖单价。

## 操作流程

### 1. 生成并审阅快照

在仓库根目录创建唯一输出目录，例如 `outputs/2026-08-23-dimensio-models/`，运行：

```powershell
python .codex/skills/refreshing-sd-channel-config/scripts/fetch_dimensio_models.py `
  --output outputs/2026-08-23-dimensio-models/models.json
```

读取 JSON，确认 `schema_version=1`、`model_count > 0`、`models_sha256` 存在，并列出每个 `models[].id`。任何网络、HTTP、JSON、空数据、空 ID 或重复 ID 错误都停止，不能清空或覆盖表格旧快照。

### 2. 确认目标表格

使用用户提供的已登录 in-app Browser 标签页打开固定 URL。确认页面标题为 `sd收录`，URL 中的表格 ID 完全匹配；只操作这个表格，不读取其他标签页的 Cookie、Token 或内容。

确认存在 `dimensio-models` 工作表。若不存在，新增该工作表；若已存在，先读取表头和当前数据行数，保存旧快照可恢复证据。

### 3. 幂等刷新模型快照

在 `dimensio-models` 的数据区保留第 1 行表头，按 [references/dimensio-models.md](references/dimensio-models.md) 的固定列写入本次快照：

1. 先写入临时区域或新建临时工作表，确认行数等于 `model_count`。
2. 校验通过后替换 `dimensio-models` 数据区；不要先清空旧数据再请求接口。
3. `pricing`、`parameters`、`highlights` 以 JSON 文本保存；`pricing` 必须保留 `kind`、`unit`、`credits` 或 `resolutions` 中的原始结构。
4. 同一 `model_id` 只允许一行；按接口返回顺序保留模型顺序。

后续执行 `sd` 投影时，严格按“积分除以 100”转换为元，并在审计报告中记录原始积分、换算值和换算规则。

### 4. 复核与报告

重新读取表头、数据行数、首行、末行和全部 `model_id`，与本地快照比较。记录以下审计信息到本轮输出目录的 `验收报告.md`：

- 固定表格 ID 和 `dimensio-models` 工作表名称；
- 源 URL、抓取时间、`model_count`、`models_sha256`；
- 写入前后行数和模型 ID 差异；
- 是否保留 `sd`、`sd官价` 原内容；
- 失败时的阶段、首个错误和旧快照保留情况。

表格复核不一致时停止，不继续 `sd` 主表投影或配置发布。

## 停止条件

- 模型接口返回非 2xx、不可解析 JSON、`data` 缺失/为空、模型 ID 为空或重复；
- 固定表格标题或 ID 不匹配，无法定位 `dimensio-models` 工作表；
- 写入后行数或模型 ID 与快照不一致；
- 任何步骤需要 API Key、Cookie、其他价格接口、`sd` 主表改价或配置导入发布。

## 验证

修改本 skill 或其抓取脚本后运行：

```powershell
python .codex/skills/refreshing-sd-channel-config/scripts/test_fetch_dimensio_models.py
python C:/Users/880pro/.codex/skills/.system/skill-creator/scripts/quick_validate.py .codex/skills/sync-dimensio-sd-models
```

未实际操作 Google 表格时，只能报告本地快照校验结果，不得声称表格已更新。
