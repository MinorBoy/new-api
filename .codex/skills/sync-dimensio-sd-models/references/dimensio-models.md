# Dimensio 模型快照表结构

`dimensio-models` 是 `sd收录` 的辅助模型与价格快照工作表，不是价格和渠道配置主表。每次同步只替换数据区，不改变其他工作表。

## 固定列

| 列 | 来源 | 规则 |
| --- | --- | --- |
| `model_id` | `data[].id` | 必填、去首尾空白、全表唯一；后续价格和配置用此键关联 |
| `display_name` | `data[].display_name` | 缺失留空 |
| `media_type` | `data[].media_type` | 原样保存，例如 `video`、`image` |
| `provider` | `data[].provider` | 原样保存；`router` 不展开成真实渠道 |
| `description` | `data[].description` | 原样保存 |
| `highlights_json` | `data[].highlights` | JSON 数组文本，缺失保存 `[]` |
| `pricing_json` | `data[].pricing` | JSON 对象文本；保存 `per_image`、`per_second`、`per_second_range` 等 `kind` 及其 `credits`/`resolutions`，第一版不丢失范围、不擅自换算 |
| `parameters_json` | `data[].parameters` | JSON 数组文本；第一版只保存，不推导能力 |
| `source_url` | 固定接口地址 | 每行相同，便于审计 |
| `fetched_at` | 抓取时间 | UTC ISO-8601 文本 |

## 更新契约

- 第 1 行为表头，字段名必须与上表一致。
- 数据行数量必须等于快照 `model_count`。
- 不能按名称、展示名或数组位置合并；只按完全相同的 `model_id` 识别模型。
- 不在本表把积分直接写成 `sd` 的 `单价 元`，也不把 `per_second_range` 的 `min` 或 `max` 单独冒充成本价。
- 后续投影到 `sd` 时，关联键必须是 `渠道=dimensio`、完整 `模型ID` 和 `清晰度`；`per_second` 才能按分辨率展开为 `计费方式=second` 的候选行。
- `per_second_range` 是自动路由的动态范围，必须保留最小/最大价格语义；没有 `sd` 对应范围字段或明确选价策略时，只生成待审阅差异，不自动覆盖 `单价 元`。
- Google 表格的价格单位为元；Dimensio 积分必须按固定规则 `100 积分 = 1 元` 换算，即 `单价 元 = credits / 100`。例如 `48` 积分为 `0.48` 元，`199` 积分为 `1.99` 元。换算必须使用十进制，并同时保留原始积分值和换算依据。
