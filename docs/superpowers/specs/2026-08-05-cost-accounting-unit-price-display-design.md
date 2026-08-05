# 成本核算单价展示设计

## 背景

使用日志的供应商成本核算详情目前展示计费模式、原币金额和标准化金额，但没有展示冻结成本规则中的单位价格。管理员无法直接核对按次、按秒或按 Token 规则的合同单价，只能根据最终金额反推。

## 目标

在使用日志的供应商成本核算详情中，直接展示获胜尝试冻结规则中的单位价格，并同时提供供应商原币单价和标准化美元单价。展示只读取 `rule_config_json` 中的冻结事实，不根据最终金额或计量结果重新计算。

## 展示规则

| 成本模式 | 展示标签 | 原币字段 | 标准化美元字段 |
| --- | --- | --- | --- |
| `per_request` | 每次单价 | `unit_price` | `normalized_usd_prices.unit_price` |
| `per_duration` | 每秒单价 | `price_per_second` | `normalized_usd_prices.price_per_second` |
| `per_token` + `total_tokens` | 每 1M Token 单价 | `total_per_million` | `normalized_usd_prices.total_per_million` |
| `per_token` + `completion_tokens` | 每 1M 输出 Token 单价 | `completion_per_million` | `normalized_usd_prices.completion_per_million` |
| `per_token` + `input_output` | 每 1M 输入 Token 单价、每 1M 输出 Token 单价 | `input_per_million`、`output_per_million` | 对应的标准化美元字段 |

单价值使用紧凑格式：`原币金额/单位 · 美元金额/单位`。例如：

```text
¥2.90 / 1M Tokens · $0.397260274 / 1M Tokens
```

币种符号优先使用项目已有的币种格式能力；无法识别时显示冻结规则中的币种代码。美元部分固定使用 `$`。

## 数据与错误处理

- 数据源为成本尝试账本中的 `cost_mode` 和 `rule_config_json`。
- 前端解析冻结规则配置，不修改后端结算逻辑和账本结构。
- 单价必须是有限且非负的十进制字符串；字段缺失、JSON 无效或模式不匹配时显示“不可用”。
- `free` 模式不展示单位价格。
- 不使用 `original_cost`、`cost_nano_usd` 或计量数据反推单价，避免审计信息与冻结合同不一致。

## 组件边界

在成本核算功能内增加纯展示逻辑，将冻结规则解析为可渲染的单价行。`CostRequestDetail` 的尝试时间线消费这些行，不在使用日志模块复制成本规则解析逻辑。

## 国际化

新增的标签和单位通过 `useTranslation()` 渲染，并同步全部前端语言文件。代码中的翻译键使用英文源字符串。

## 测试

组件回归测试覆盖：

- 按次规则同时显示原币和标准化美元每次单价。
- 按秒规则同时显示原币和标准化美元每秒单价。
- 按总 Token、输出 Token、输入/输出 Token 三种规则显示正确的每 1M Token 单价行。
- 无效 JSON、缺失字段和免费模式不产生猜测价格，并展示可用的降级状态。

实施时先增加失败测试，再完成最小实现，并运行受影响测试、类型检查、涉及文件 lint、生产构建和 `git diff --check`。
