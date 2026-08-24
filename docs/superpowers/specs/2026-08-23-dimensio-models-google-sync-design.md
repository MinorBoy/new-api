# Dimensio 模型目录同步设计

## 目标

第一版为项目提供一个可审计、可回滚的 Dimensio 模型目录和平台基础价格同步基础链路：从公开的 `GET /v1/models` 获取模型目录，校验并保存模型及 `pricing` 快照，再由项目专用 skill 将快照更新到指定 Google 表格的 `dimensio-models` 工作表。

本版不调用其他价格接口，不修改 `sd` 主业务表的价格、计费方式、能力限制或官方售价，也不执行 new-api 配置导入、发布和激活。`/v1/models` 返回的 `pricing` 属于本版快照内容，但不在本版直接投影为 `sd` 的单价。

## 协议事实

- 默认 Base URL：`https://jimeng.dimensio.cn`。
- 请求：`GET /v1/models`，无需 API Key。
- 返回：顶层 `data` 数组；每项至少包含非空 `id`，并可包含 `object`、`owned_by`、`description`、`display_name`、`media_type`、`provider`、`highlights`、`pricing`、`parameters`。
- `id` 是后续模型、价格和渠道配置的稳定关联键；重复 ID 只保留一条，重复或空 ID 视为源数据错误。
- `pricing` 是平台基础价格：图片通常按张，视频按清晰度返回每秒价格，自动路由返回当前可用渠道的价格范围；价格单位是平台积分，不包含个人会员价。
- Google 表格的价格单位是元，Dimensio 价格按固定规则换算：`100 积分 = 1 元`，即 `单价 元 = credits / 100`。快照保留积分原值，`sd` 投影才执行换算。

## 方案

### 目录抓取

在 `.codex/skills/refreshing-sd-channel-config/scripts/fetch_dimensio_models.py` 中实现标准库脚本：

1. 拼接并请求 `/v1/models`，限制超时并检查 2xx 状态。
2. 使用 JSON 解析，不通过正则提取模型字段。
3. 校验顶层对象、`data` 数组和每项非空唯一 `id`。
4. 输出带来源 URL、抓取时间、模型数量和完整模型对象的 JSON 快照；支持指定输出文件，默认 stdout。
5. 不输出或接受凭据，错误消息不包含请求头内容。

### Google 表格更新

skill 的 `models-only` 模式使用用户提供的固定 Google 表格，在确认标题、表格 ID 和目标工作表后：

1. 创建或清空 `dimensio-models` 工作表的数据区。
2. 写入固定列：`model_id`、`display_name`、`media_type`、`provider`、`description`、`pricing_json`、`parameters_json`、`source_url`、`fetched_at`。
3. `highlights` 作为 JSON 数组并入 `description` 后的备注字段或单独 JSON 文本；`pricing` 作为原始 JSON 文本保存，不拆成不可审计的多列。
4. 保留 `sd`、`sd官价` 和其他工作表原内容；模型快照为空、校验失败或写入未确认时停止，不清空旧快照。
5. 更新完成后重新读取表头、行数和首尾代表性行，记录快照 SHA-256 与表格更新时间。

## 错误与安全

- 网络错误、非 2xx、JSON 错误、`data` 缺失、空 ID、重复 ID 都阻止表格写入。
- 只访问用户明确提供的 Google 表格和 Dimensio 模型接口；不读取其他标签页、Cookie 或 API Key。
- 不把 `pricing` 的积分或自动路由范围未经换算、审阅就写成 new-api 的单一成本价，不从模型 ID 猜测分辨率、时长或参考素材。
- 表格更新只影响 `dimensio-models` 工作表，失败时保留旧数据。

## 验收

1. 单元测试覆盖成功响应、空数据、重复 ID、缺失 ID、非法 JSON 和非 2xx 响应。
2. skill 快速校验通过，引用的脚本和参考文档路径存在。
3. 使用 mock HTTP 服务运行脚本，输出 JSON 可被再次解析且模型 ID 顺序稳定。
4. 真实表格操作仅在用户明确运行 skill 时执行；本次代码交付不伪造 Google 表格写入成功。

## 后续投影到 sd

独立快照可以作为 `sd` 主表的唯一输入，但不能按一行模型直接覆盖主表。后续同步应按 `(渠道=dimensio, 模型ID, 清晰度)` 展开：精确 `per_second` 价格可生成 `计费方式=second` 候选行；`per_second_range` 必须保留 `min/max`，在主表没有范围字段或未确认选价策略前只能生成待审阅差异。写入 Google 表格的 `单价 元` 时严格使用 `credits / 100` 的十进制换算，并保留原始积分、换算值、换算规则和快照哈希。
