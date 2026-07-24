# 渠道模型成本与计费毛利润设计

## 1. 目标

在不改变现有用户计费契约的前提下，为 new-api 增加渠道级上游成本核算和仅管理员可见的计费毛利润报表。

系统继续按客户端模型的全局售价和有效用户分组倍率计算用户费用。上游成本作为独立账本，按每次真实发送给供应商的 attempt、实际渠道和最终计费上游模型核算。

第一期交付：

1. 版本化渠道模型成本规则；
2. 同步与异步请求的 attempt 级成本核算；
3. 缺少成本规则时阻止发送的严格路由；
4. 管理员请求详情、异常对账队列和利润报表。

Excel/CSV 价格表导入、在线价格源同步和可执行成本表达式不属于第一期，只保留兼容边界。

## 2. 术语与统计口径

### 2.1 客户端请求

客户端请求是网关接收的一次逻辑请求。同步请求使用稳定的 `request_id` 标识；异步任务同时预生成公开 `task_id`。

一次客户端请求可能因为重试而产生多个上游 attempt。

### 2.2 上游 attempt

attempt 是一次已经进入供应商发送流程的独立尝试。不同 attempt 可以使用不同渠道、不同上游模型和不同成本规则。

只有最终为客户端提供成功结果的 attempt 是获胜 attempt。其他 attempt 即使没有产生客户端结果，也可能已经产生供应商成本。

### 2.3 计费收入等值

第一期不实现现金收入确认。报表中的收入使用现有用户最终计费额度换算出的 USD 等值：

```text
billed_revenue_equivalent_nano_usd =
    final_user_quota
    / quota_per_unit_snapshot
    * 1,000,000,000
```

`final_user_quota` 必须非负，`quota_per_unit_snapshot` 必须是大于零的规范 Decimal。换算使用与成本金额相同的明确舍入和 `int64` 溢出检查；无法确认或换算收入时进入 `revenue_failed`。

该值表示按当前网关计费契约确认的售价等值，不等于实际现金收入。充值折扣、赠送额度、订阅套餐售价、支付手续费、税费和未使用订阅额度都不会在第一期分摊到单次请求。

钱包和订阅请求都使用相同的计费收入等值公式，但必须快照并展示资金来源。UI 和 API 必须使用“计费收入等值”“计费毛利润”等名称，不能简称为“现金收入”或“净利润”。

### 2.4 计费毛利润

```text
billed_gross_profit_nano_usd =
    billed_revenue_equivalent_nano_usd
    - sum(all_confirmed_attempt_cost_nano_usd)
```

利润可以为负数。收入和成本不能为负数。收入为零时毛利率为空。

## 3. 已确认的业务决策

1. 用户售价计费与供应商成本核算是两个相互独立的账本。
2. 用户售价继续以 `OriginModelName` 为键，不因渠道选择或模型映射而改变。
3. 有效用户分组倍率继续用于给客户折扣或加价。
4. 成本按每次真实上游 attempt 记录，不能只记录最终成功渠道。
5. 成本规则以 `channel_id + billable_upstream_model` 为业务键。
6. `billable_upstream_model` 必须来自最终上游请求身份，不能直接使用客户端模型或初始映射结果。
7. 成本规则、换算参数、计量来源和最终计费身份在发送前快照；后续配置变化不回算历史记录。
8. 缺少有效成本规则时，严格模式不得向该渠道发送请求；系统不能假定“成本等于售价”。
9. 请求发送后缺少权威计量或成本结算失败，不能覆盖已经成功的客户端结果。
10. 只有能证明请求未发送或供应商明确不收费时，成本才能确认为零。
11. 超时、断连和发送后进程崩溃等不明确情况进入 `cost_unknown`，不能默认零成本。
12. 第一期开启后同时交付规则配置、请求详情、异常队列和利润聚合报表。

## 4. 保持不变的现有契约

现有用户计费链路继续负责预扣、结算、退款、订阅额度、钱包额度、令牌额度、用户已用额度和渠道已用额度。

`Channel.UsedQuota` 继续表示用户计费额度，不能改为供应商成本。

现有用户售价模式保持不变：

- Token 售价使用 `ModelRatio` 及相关倍率；
- 按次售价使用 `ModelPrice`；
- 按时长售价使用 `per_duration`；
- 表达式售价使用 `billing_expr`。

成本核算只读取现有链路最终确认的用户费用，不执行第二次用户扣费，不使用供应商成本反推用户售价，也不把成本差异转换成用户额度。

请求失败后单独收取的违规费用不属于模型计费收入等值，第一期不计入利润账本。

## 5. 总体架构

第一期在主数据库增加四类实体：

```text
channel_model_cost_rules
        |
        v
cost_accounting_requests 1 ─── n cost_accounting_attempts
        |                             |
        └──────── cost_accounting_audits <────────┘
```

- `channel_model_cost_rules`：版本化成本规则；
- `cost_accounting_requests`：客户端请求级收入与利润汇总；
- `cost_accounting_attempts`：每次上游发送的成本事实；
- `cost_accounting_audits`：人工对账和状态修复的追加式审计记录。

所有表都位于主数据库，以便使用同一事务完成 attempt 结算和请求级汇总更新。利润报表查询主数据库，不解析日志 JSON，也不依赖日志数据库类型。

## 6. 成本规则与版本生命周期

### 6.1 业务标识

规则业务键为：

```text
channel_id + billable_upstream_model
```

版本唯一键为：

```text
channel_id + billable_upstream_model + version
```

`version` 在业务键内单调递增。

### 6.2 生命周期

第一期只支持立即激活，不支持预约生效。规则状态为：

- `draft`：可编辑，不能参与路由或结算；
- `active`：当前活动版本，不可直接编辑；
- `retired`：已被后继版本替换或被管理员停用，不可重新激活。

激活操作必须在事务中使用共享的 `lockForUpdate` 锁定业务键：

1. 重新校验草稿和适配器能力；
2. 为新版本设置 `effective_from`；
3. 将旧活动版本设置为 `retired`，并把 `effective_to` 设置为同一时刻；
4. 激活新版本；
5. 提交后清理受影响的覆盖缓存。

不依赖数据库部分索引保证单活动版本。事务必须显式查询并拒绝重复活动版本。

### 6.3 公共字段

| 字段 | 含义 |
|---|---|
| `id` | 数据库生成的规则 ID |
| `channel_id` | 实际供应商渠道 |
| `billable_upstream_model` | 供应商实际计费模型名 |
| `version` | 业务键内版本号 |
| `status` | `draft`、`active` 或 `retired` |
| `cost_mode` | `free`、`per_request`、`per_duration` 或 `per_token` |
| `schema_version` | 规则 JSON 契约版本，第一期为 `1` |
| `config_json` | 规范 JSON 文本形式的完整规则配置，包含原始参数和标准化 USD 单价 |
| `source` | 第一期为 `manual` |
| `note` | 管理员备注 |
| `created_by` | 创建草稿的管理员 ID |
| `activated_by` | 激活版本的管理员 ID |
| `effective_from` | 实际激活时间 |
| `effective_to` | 结束时间；当前活动版本为空 |
| 时间戳 | 创建、更新和激活审计时间 |

规则公共查询字段使用普通列。模式参数和换算参数使用规范 JSON 文本保存，避免依赖数据库特有 JSON 类型或 JSON 查询运算符。

`config_json` 必须由版本化 DTO 校验后通过 `common.Marshal` 生成，不能接受或拼接任意 JSON。第一期的模式字段为：

| 模式 | 必填配置 |
|---|---|
| `free` | `zero_cost_reason` |
| `per_request` | `unit_price`、`charge_event` |
| `per_duration` | `price_per_second`、`charge_event`、`meter_source` |
| `per_token` | `token_mode`、`charge_event`、`meter_source`，以及对应的每百万 Token 单价 |

`per_token` 的单价字段随子模式固定：`total_tokens` 使用 `total_per_million`，`completion_tokens` 使用 `completion_per_million`，`input_output` 使用 `input_per_million + output_per_million`。未被当前模式使用的价格字段必须为空，避免同一规则出现两种解释。

## 7. 原币价格与标准化成本

规则配置保留下列原始商业参数：

| 字段 | 约束 |
|---|---|
| `currency` | 大写币种代码，例如 `CNY` 或 `USD` |
| `billing_multiplier` | 必须为正，默认 `1` |
| `purchase_discount_ratio` | 必须为正，默认 `1` |
| `recharge_exchange_ratio` | 每一实付单位获得的供应商余额单位数，必须为正，默认 `1` |
| `fee_rate` | 非负小数费率，默认 `0` |
| `currency_to_usd_rate` | 一单位原币对应的 USD 金额，必须为正 |

所有 Decimal 输入使用经过校验的规范字符串，不使用二进制浮点数。保存规则时同时生成 `normalized_usd_prices` 并写入 `config_json`；激活时重新计算并验证两者一致，attempt 快照复制完整配置。

标准化 USD 单价为：

```text
normalized_usd_unit_price =
    original_currency_unit_price
    * billing_multiplier
    * purchase_discount_ratio
    / recharge_exchange_ratio
    * (1 + fee_rate)
    * currency_to_usd_rate
```

`currency_to_usd_rate` 的方向必须明确。例如 CNY 规则保存 CNY 1 对应的 USD 金额，而不是含义不明确的 `7.3`。

Lucen 类价格表中的“无 V”或“含 V”折扣属于管理员选择的配置输入。第一期不导入文件，也不在请求期间动态切换折扣。

所有金额计算使用 `shopspring/decimal`。原币成本保存规范 Decimal 字符串；查询和汇总金额保存带检查的有符号 `int64` nano-USD，其中 USD 1 等于 `1,000,000,000` nano-USD。

Decimal 转 nano-USD 必须经过统一函数，明确使用 half-away-from-zero 舍入并检查 `int64` 溢出。溢出时不得环绕或静默饱和；对应 attempt 进入 `settlement_failed`，并产生管理员告警。

## 8. 四种结构化成本模式

### 8.1 显式免费成本

`free` 表示供应商明确不收取该模型成本。

规则必须填写 `zero_cost_reason`。缺少规则、价格为空或解析失败不能解释为免费。

发送事件确认后，attempt 进入 `confirmed_zero`，不需要时长或 Token 计量。

### 8.2 按次成本

`per_request` 保存每次可计费 attempt 的原币价格。第一期每个 attempt 的 `billable_request_count` 固定为一。

规则必须选择计费事件：

- `response_succeeded`：同步上游成功接受并返回成功响应；
- `submit_accepted`：异步任务提交被供应商接受；
- `task_succeeded`：异步任务只有终态成功才收费。

`response_succeeded` 由适配器按供应商协议确认，不能只根据任意 HTTP 状态码推断。对于流式响应，它表示供应商已接受并开始成功响应，而不是要求客户端完整读取响应体。

```text
original_cost = request_unit_price * 1
```

### 8.3 按时长成本

`per_duration` 保存每秒原币价格，并选择一种计量来源：

- `validated_request`：发送前已完成边界校验和标准化的计费时长；
- `upstream_actual`：供应商返回的权威时长，或适配器明确认可的媒体元数据时长。

```text
original_cost = price_per_second * billable_duration_seconds
```

请求时长必须复用 `relaycommon.MaxTaskDurationSeconds` 上限。适配器不得静默切换来源。声明的计量缺失时进入 `settlement_failed`。

### 8.4 按 Token 成本

`per_token` 保存每 1M Token 的价格，计量来源为：

- `upstream_usage`：供应商返回的权威用量；
- `local_usage`：适配器或现有本地计数器生成的确定性用量。

结构化子模式为：

- `total_tokens`：对 `total_tokens` 使用一个单价；
- `completion_tokens`：对 `completion_tokens` 使用一个单价；
- `input_output`：输入和输出 Token 分别使用单价。

```text
single_cost = selected_tokens / 1,000,000 * token_price

split_cost = input_tokens / 1,000,000 * input_token_price
           + output_tokens / 1,000,000 * output_token_price
```

NewAPIVideo 的轮询结果已经保留 `completion_tokens` 和 `total_tokens`，规则可以选择供应商实际采用的字段。

所有计量标量必须使用可空字段区分“缺失”和“显式零值”。Token 数必须非负并受现有 max-tokens 安全上限约束；实现时应复用或导出当前 `maxTokensLimit` 语义，不另建冲突上限。

如果供应商既不返回 Token，也不存在可靠的本地用量，就不能激活相应的 `per_token` 规则。

## 9. 适配器成本契约

成本计量不能直接复用面向用户计费的隐式假设。适配器必须向成本核算层提供明确契约：

1. 能否确认最终 `billable_upstream_model`；
2. 支持哪些计费事件；
3. 支持哪些计量来源；
4. 能提供哪些可空计量字段；
5. 哪些响应能明确证明供应商不收费。

统一计量对象至少包含：

```text
source
duration_seconds?
input_tokens?
output_tokens?
completion_tokens?
total_tokens?
```

适配器可以按供应商协议确定性地归一化字段，例如在协议明确定义时计算 `total_tokens = input_tokens + output_tokens`。归一化规则属于适配器契约，必须有测试，不能在通用成本服务中猜测。

规则激活时校验声明能力，运行时仍重新校验实际计量。第一期不支持估算成本冒充权威成本。

## 10. 最终计费身份

当前请求链路中的上游模型可能依次受到以下因素影响：

1. 能力路由目标；
2. 普通渠道模型映射；
3. 适配器协议转换或模型后缀处理；
4. 渠道参数覆盖；
5. 透传请求中的实际模型字段。

因此，成本身份分为两类：

- `predicted_upstream_model`：渠道选择阶段根据路由目标或模型映射得到，只用于覆盖过滤；
- `billable_upstream_model`：最终上游请求完成转换和覆盖后，由转换层或适配器显式确认，是规则快照和成本结算的唯一键。

`RelayInfo` 增加独立的最终成本身份字段。严格模式下，在 `DoRequest` 前无法可靠确认该字段时必须拒绝当前 attempt。

通用代码不能假定 `OriginModelName`、`UpstreamModelName` 或原始请求 JSON 的 `model` 一定是最终计费身份。会改写模型的适配器和参数覆盖路径必须同步更新或解析最终身份。

## 11. 请求级账本

`cost_accounting_requests` 每个客户端请求一条，至少包含：

| 字段组 | 内容 |
|---|---|
| 标识 | `id`、唯一 `request_id`、可空且唯一的公开 `task_id` |
| 用户维度 | 用户、令牌、用户分组、使用分组 |
| 模型维度 | 客户端模型 `OriginModelName` |
| 资金来源 | `wallet` 或 `subscription`，以及可选订阅实例和计划 ID |
| 收入快照 | 最终用户额度、规范 Decimal `quota_per_unit_snapshot`、计费收入等值 |
| attempt 汇总 | 已确认成本合计、attempt 数量、获胜 attempt ID |
| 利润 | 计费毛利润、可空 `gross_margin_ppm` |
| 状态 | 收入状态、利润完整性状态、稳定失败代码 |
| 时间 | 请求时间、收入确认时间、利润确认时间和更新时间 |

请求账本不得保存 API Key、原始提示词、完整请求头或完整请求体。

`gross_margin_ppm` 中 `1,000,000` 表示 100%。使用 Decimal 计算：

```text
gross_margin_ppm =
    billed_gross_profit_nano_usd
    / billed_revenue_equivalent_nano_usd
    * 1,000,000
```

收入为零时该字段为空。负利润和负毛利率是有效结果。

## 12. attempt 级账本

`cost_accounting_attempts` 每次进入上游发送流程创建一条，至少包含：

| 字段组 | 内容 |
|---|---|
| 标识 | `id`、`cost_request_id`、`attempt_no` |
| 渠道快照 | 渠道 ID、名称、类型；不保存密钥 |
| 模型身份 | 预计上游模型、最终计费上游模型 |
| 规则快照 | 规则 ID、版本、模式、`schema_version`、完整配置和标准化单价 |
| 计量 | 计费事件、声明来源、请求计量和实际计量 |
| 金额 | 原币成本 Decimal 字符串、`cost_nano_usd` |
| 结果 | 上游接受状态、HTTP/协议结果摘要、稳定失败代码 |
| 状态 | attempt 成本状态和对账状态 |
| 时间 | 准备、发送、接受、终态和结算时间 |

唯一约束为：

```text
cost_request_id + attempt_no
```

attempt 快照在 `prepared` 首次持久化后就不可替换。规则停用、渠道删除、模型映射变化和汇率变化都不能修改历史快照。

## 13. 收入与成本状态机

### 13.1 收入状态

```text
pending -> settled
pending -> confirmed_zero
pending -> revenue_failed
revenue_failed -> settled
revenue_failed -> confirmed_zero
```

- `settled`：现有用户计费链路已确认最终额度；
- `confirmed_zero`：请求未产生模型用户费用，或已完成全额退款；
- `revenue_failed`：用户计费持久化结果无法确认，需要管理员处理。

订阅额度扣减仍按最终用户额度计算计费收入等值，并通过资金来源字段明确其不是现金收入。

### 13.2 attempt 成本状态

```text
prepared -> dispatching
prepared -> not_dispatched

dispatching -> awaiting_meter
dispatching -> settled
dispatching -> confirmed_zero
dispatching -> cost_unknown

awaiting_meter -> settled
awaiting_meter -> confirmed_zero
awaiting_meter -> settlement_failed
awaiting_meter -> cost_unknown

settlement_failed -> settled
settlement_failed -> confirmed_zero
cost_unknown -> settled
cost_unknown -> confirmed_zero
```

- `prepared`：规则已快照，但尚未授权发送；
- `dispatching`：发送授权已持久化，即将或正在调用上游；
- `not_dispatched`：可以证明没有进入上游发送；
- `awaiting_meter`：供应商已接受，但必须等待响应或异步终态计量；
- `settled`：供应商成本已完整确认；
- `confirmed_zero`：供应商契约明确确认不收费；
- `cost_unknown`：无法确认是否收费；
- `settlement_failed`：已知需要计费，但计量缺失或计算失败。

### 13.3 利润完整性

请求级利润状态为：

- `complete`：收入已确认，且全部 attempt 为 `settled`、`confirmed_zero` 或 `not_dispatched`；
- `incomplete_cost`：任一 attempt 仍为其他状态；
- `incomplete_revenue`：收入尚未确认。

只有 `complete` 请求进入收入、成本、利润和毛利率金额汇总。不完整请求可以展示数量和已知成本小计，但 UI 和 API 必须明确标记该小计不完整。

## 14. 严格路由

成本核算提供全局 `disabled` 和 `strict` 两种模式。迁移后默认 `disabled`。

### 14.1 渠道选择阶段

严格模式根据请求路径、渠道和 `predicted_upstream_model` 过滤没有活动规则或适配器能力不足的候选渠道。

该过滤必须覆盖：

- 普通随机选择；
- 能力路由；
- 已知渠道校验；
- 渠道亲和；
- 同步重试；
- 异步任务重试。

排除集合必须同时适用于普通路由和能力路由，不能只在能力路由下生效。

### 14.2 发送前最终校验

最终上游请求构建完成后，在发送前执行权威校验：

1. 确认 `billable_upstream_model`；
2. 在第一个主数据库事务中读取当前活动规则；
3. 校验规则、适配器能力和请求需要的计量来源；
4. 创建或复用请求账本，创建 `prepared` attempt 并快照规则；
5. 提交第一个事务，使 `prepared` 成为可恢复的持久状态；
6. 使用条件更新把 attempt 从 `prepared` 改为 `dispatching`，并确认该更新已经提交；
7. 只有 `dispatching` 持久化成功后才允许调用 `DoRequest`。

步骤 6 失败时不得发送。若进程在步骤 5 后、步骤 6 前退出，后台可以证明该 attempt 未获得发送授权并把它转为 `not_dispatched`。若步骤 6 已提交但进程在调用上游前退出，系统无法证明是否发送，必须按 `cost_unknown` 保守处理。

若最终身份没有规则，当前渠道必须在尚未发送时被排除，并允许选择下一个渠道。所有候选均不满足时返回现有安全的“渠道不可用”响应和稳定错误码，不泄露缺失规则或价格。

### 14.3 缓存

选择阶段可以使用活动规则覆盖缓存，但发送前事务必须读取权威主数据库记录。

规则激活、停用、渠道删除和相关模型映射变化都应清理受影响缓存。若缓存判断所有候选均不满足，系统必须回源主数据库复核一次，避免缓存滞后导致错误拒绝。

缓存只影响路由效率，不能成为发送安全边界。

## 15. 同步请求数据流

1. 现有售价逻辑按客户端模型计算预扣额度。
2. 现有用户计费会话执行预扣。
3. 路由选择存在预计成本覆盖的渠道。
4. 适配器完成模型映射、请求转换和参数覆盖。
5. 成本核算确认最终计费身份，并持久化请求账本和 attempt 快照。
6. attempt 进入 `dispatching` 后调用上游。
7. 每次重试创建新的 attempt，前序 attempt 不会被覆盖。
8. 适配器提供规则声明的计费事件和计量。
9. 成本核算独立结算每个 attempt。
10. 现有用户计费链路确认最终用户额度，收入账本据此结算。
11. 最终成功 attempt 被标记为获胜 attempt。
12. 当收入和全部 attempt 成本完整时，事务更新请求级利润和确认时间。

流式客户端断开不能阻止已发送 attempt 的成本状态持久化。发送后的成本状态更新应使用有超时的服务端上下文，不能只依赖已取消的客户端请求上下文。

## 16. 异步任务数据流

1. 现有任务链路完成请求校验、售价计算和用户预扣。
2. 第一次发送前预生成公开 `task_id`。
3. 路由和适配器确认最终计费身份。
4. 使用 `request_id + task_id` 创建请求账本和 attempt 快照。
5. attempt 进入 `dispatching` 后提交供应商。
6. `submit_accepted` 规则可以立即结算；其他规则进入 `awaiting_meter`。
7. 上游成功返回后，按现有流程创建 `Task`，并在 `Task.PrivateData` 保存成本请求 ID。
8. 轮询阶段从成本账本读取不可变规则快照，保存权威时长或 Token。
9. 现有任务计费逻辑结算或退还用户额度。
10. 成本核算通过条件状态转换结算 attempt，并更新请求级利润。

发送前不要求已经存在 `Task` 行。发送前的持久化锚点是主数据库中的成本请求和 attempt。

如果供应商接受提交后 `Task` 插入失败，attempt 记录仍必须保留，并产生孤立任务管理员告警。公开 `task_id`、`request_id` 和上游请求摘要用于后续人工恢复，不能删除或伪装成零成本。

关闭严格模式、停用规则或修改映射只影响新请求。已有异步 attempt 始终使用原始快照继续结算。

## 17. 重试与失败分类

### 17.1 重试成本

同一客户端请求的每次真实发送都创建独立 attempt。前序超时、错误响应或失败任务可能产生供应商成本，必须保留。

请求级利润为一次计费收入等值减去全部已确认 attempt 成本。

### 17.2 可以确认零成本的情况

只有以下情况可以进入 `not_dispatched` 或 `confirmed_zero`：

- 在 attempt 进入 `dispatching` 前终止；
- 传输层能够证明没有发送任何请求数据；
- 供应商协议或合同明确声明该拒绝结果不收费；
- 规则的任务计费事件为 `task_succeeded`，且任务明确失败；
- 显式 `free` 规则。

### 17.3 不明确情况

下列情况默认进入 `cost_unknown`：

- 进入 `dispatching` 后连接超时；
- 请求可能已写出后断连；
- 收到无法按供应商契约判定收费结果的错误响应；
- 发送期间进程崩溃；
- 上游已接受，但结果和权威计量永久丢失。

HTTP 状态码本身不能通用地证明“不收费”。适配器只有在供应商契约明确时才能把特定响应分类为 `confirmed_zero`。

### 17.4 客户端结果

发送前的规则查询、身份确认或 attempt 持久化失败必须阻止当前发送。

发送后，成本状态更新失败不能隐藏已经成功的客户端结果。attempt 保持在可恢复状态，系统发出带请求关联的管理员告警，并由后台扫描或人工对账完成修复。

## 18. 幂等、并发与恢复

请求账本使用服务端生成的唯一 `request_id` 幂等。异步公开 `task_id` 使用可空唯一索引。

attempt 使用 `(cost_request_id, attempt_no)` 幂等。`attempt_no` 由单次请求内的单调序号产生，不能在渠道切换时重置。

所有状态转换使用带当前状态条件的 GORM `Updates`，并要求 `RowsAffected == 1` 才能继续后续金额更新。重复轮询、重复回调、客户端断连重入和人工对账都不能重复确认成本。

当 attempt 金额变化时，attempt 更新和请求级已确认成本汇总必须在主数据库同一事务中完成。利润和毛利率只在事务中从确认后的收入及 attempt 重新计算，不能在多个调用点增量拼接浮点结果。

后台恢复任务处理停留过久的状态：

- 过期 `prepared` 且从未进入 `dispatching`：转为 `not_dispatched`；
- 过期 `dispatching`：保守转为 `cost_unknown`；
- 有完整计量的 `awaiting_meter`：幂等重试结算；
- `settlement_failed` 和 `cost_unknown`：进入管理员异常队列，不自动猜测成本。

## 19. 人工对账与审计

管理员只能对 `settlement_failed`、`cost_unknown` 或 `revenue_failed` 记录执行修复。第一期不允许直接修改已经确认的 `settled` 金额；供应商账单调整属于未来的更正分录设计。

人工操作必须填写原因，并在 `cost_accounting_audits` 追加保存：

- 管理员 ID；
- 请求和 attempt ID；
- 原状态和新状态；
- 补录计量；
- 使用的原始规则快照 ID 和版本；
- 原金额和新金额；
- 操作原因和时间。

人工重新结算继续使用发送前快照，不能使用当前活动规则或当前汇率。状态修复、金额更新、请求级汇总和审计记录必须在同一主数据库事务中提交。

管理员操作日志可以保存展示副本，但追加式审计表是权威记录，不能依赖可能配置 TTL 的日志数据库。

## 20. 收入归因与报表时间

### 20.1 请求级归因

请求级报表记录一次计费收入等值和全部 attempt 成本。

### 20.2 渠道和上游模型归因

按渠道或上游模型分组时：

- 全部请求收入只归属于获胜 attempt；
- 非获胜 attempt 的归属收入为零；
- 每个 attempt 的实际成本归属于自己的渠道和最终计费模型。

因此失败重试会在对应渠道形成负贡献。所有渠道重新相加必须与请求级总账一致，不能把一笔请求收入重复归属给多个渠道。

### 20.3 时间口径

请求保存：

- `requested_at`：客户端请求发生时间；
- `revenue_settled_at`：用户最终额度确认时间；
- `profit_recognized_at`：收入和全部 attempt 成本首次完整确认的时间。

利润报表默认按 `profit_recognized_at` 过滤和分组。异步任务在最终结算前不进入已实现利润金额汇总。

第一期已确认金额不可直接修改，因此 `profit_recognized_at` 一经设置不发生历史期间重算。

### 20.4 汇总算法

汇总毛利率必须按汇总金额计算：

```text
summary_margin = sum(profit) / sum(revenue)
```

不能平均请求级毛利率。

数据库或应用层汇总发生 `int64` nano-USD 溢出时必须返回稳定错误并告警，不能环绕或返回错误利润。

## 21. 管理员界面

### 21.1 渠道模型成本

渠道管理增加“模型成本”页签，展示：

- 最终计费上游模型；
- 映射的客户端模型；
- 当前用户官方售价；
- 成本模式、原币价格和标准化 USD 价格；
- 当前活动版本、来源、状态和生效时间；
- 适配器计量能力和覆盖状态。

编辑抽屉按模式显示字段，支持草稿保存、校验、激活、停用和版本历史。

预览显示选定用户分组下的预计计费收入等值、预计成本、预计计费毛利润和预计毛利率。预览必须标记为估算，不能写入历史账本。

### 21.2 覆盖检查

严格模式启用前提供覆盖检查，至少列出：

- 当前启用渠道和模型；
- 预计映射后的上游模型；
- 是否存在活动规则；
- 适配器是否能确认最终计费身份；
- 是否支持规则声明的计量来源和计费事件；
- 多密钥或多请求路径是否违反第一期单一成本契约限制。

覆盖检查只能作为启用前诊断；发送前权威校验仍不可省略。

### 21.3 请求详情

管理员请求详情展示：

- 计费收入等值、资金来源和最终用户额度；
- attempt 时间线；
- 每次渠道、最终计费模型和是否获胜；
- 规则版本、计费事件、计量来源和计量值；
- 原币金额、换算快照和 USD 成本；
- 成本状态、失败代码和审计历史。

### 21.4 异常队列

异常队列集中展示：

- `cost_unknown`；
- `settlement_failed`；
- `revenue_failed`；
- 供应商已接受但本地 `Task` 插入失败的孤立异步请求。

### 21.5 利润报表

报表支持按利润确认时间、请求时间、渠道、最终计费上游模型、客户端模型、用户分组、资金来源和状态筛选。

报表展示：

- 计费收入等值、供应商成本、计费毛利润和毛利率；
- 完整请求数、负利润请求数和重试 attempt 数；
- 待计量、未知成本、结算失败和收入失败数量；
- 按渠道和模型拆分的明细。

只有 `complete` 请求参与金额汇总。

## 22. API 与隐私边界

仅管理员可访问：

- 成本规则列表、草稿创建、校验、激活、停用和版本历史；
- 成本覆盖检查；
- 请求与 attempt 详情；
- 异常队列和带审计的人工重新结算；
- 利润汇总和分组明细。

当“未提供”和“显式零值”语义不同时，请求 DTO 必须使用指针标量和 `omitempty`。

所有 JSON 编解码必须使用 `common/json.go` 包装函数。

API 返回规范 Decimal 字符串和带范围检查的整数 nano-USD。前端不得依赖二进制浮点序列化计算账务金额。

现有消费日志只在 `other.admin_info` 中保存成本请求 ID 和简要管理员摘要。普通用户日志整形继续整体移除 `admin_info`。

公开日志、普通用户日志、公开定价 API 和模型列表 API 不得返回成本、利润、汇率、规则 ID、attempt 状态或对账字段。

成本账本不得保存供应商 API Key、完整认证头、原始提示词或未脱敏响应体。

## 23. 校验与异常处理

规则出现以下情况时不能激活：

- 缺少渠道或最终计费上游模型；
- 不支持的成本模式、计费事件或计量来源；
- 非 `free` 模式的必填价格为零或负数；
- `free` 模式缺少明确原因；
- 倍率、兑换比例或汇率不是正 Decimal；
- 手续费为负数；
- Token 子模式字段不完整；
- 选定渠道或适配器不能提供声明能力；
- 同一业务键存在另一个活动版本；
- 标准化单价超出支持范围；
- 同一渠道的多密钥或多路径成本契约不一致。

请求运行时出现以下情况时进入 `settlement_failed`：

- 权威计量字段缺失；
- 计量为负数或超过安全上限；
- Decimal 计算失败；
- nano-USD 转换溢出；
- 实际计量来源与规则声明不一致。

系统不得虚构用量、假定成本等于售价、静默切换计量来源、把未知成本记成零，或把成本结果转换成用户额度返还。

## 24. 数据库兼容与迁移

所有新表使用 GORM，并同时支持 SQLite、MySQL 5.7.8+ 和 PostgreSQL 9.6+。

实现要求：

- 主键由 GORM 和数据库生成；
- 行锁使用共享 `lockForUpdate`；
- 不依赖部分索引、数据库 JSON 操作符或数据库特有布尔语法；
- Decimal 和规则快照使用规范文本；
- 业务布尔默认值由代码设置，不使用 GORM 布尔默认标签；
- 普通迁移和快速迁移路径都包含四个新模型；
- `request_id` 使用适合现有请求 ID 的有限长度字符串并建立唯一索引，`task_id` 使用不超过 191 字符的可空字符串；
- 可空唯一 `task_id` 和组合唯一 attempt 索引必须在三种数据库中验证；
- 不对渠道建立级联删除，历史记录依赖渠道快照保留可读性。

迁移后功能保持 `disabled`。管理员先创建并校验规则，执行覆盖检查，再显式启用严格模式。

旧请求和旧日志不生成虚假成本记录，应显示为“历史成本不可用”，不能显示为零成本或零利润。

## 25. 第一阶段范围约束

第一期成本键只有 `channel_id + billable_upstream_model`，因此：

1. 同一渠道内的多密钥必须共享同一商业价格；价格不同的账户必须拆成不同渠道。
2. 同一最终计费上游模型在该渠道的所有启用请求路径必须使用同一成本契约。
3. 批量、实时、服务等级或其他价格不同的能力必须拆分渠道或使用不同的上游模型别名。

第一期不包含：

- 实际现金收入确认和订阅收入分摊；
- 充值折扣、赠送额度、支付手续费、税费或公司净利润；
- 供应商价格在线同步；
- Excel/CSV 文件导入；
- 自动回算历史利润；
- 已确认金额的更正分录；
- 可执行供应商成本表达式；
- 静默估算 Token 或供应商收费状态；
- 向普通用户公开上游成本或利润。

## 26. 测试策略

后端测试必须保护可观察的核算、路由和隐私契约。

### 26.1 规则与金额

1. `free`、按次、按时长、单一 Token 和输入/输出 Token 的精确成本；
2. 原币、计费倍率、采购折扣、充值兑换比例、手续费和汇率换算；
3. Decimal 到 nano-USD 的舍入、负值拒绝和溢出处理；
4. 活动版本切换、并发激活及历史快照不可变；
5. 多密钥和多路径单一成本契约校验。

### 26.2 路由与最终身份

1. 普通选择、能力路由、渠道亲和和重试的严格过滤；
2. 缓存全不匹配时的权威数据库复核；
3. 普通模型映射、能力路由目标、适配器模型改写和参数覆盖后的最终计费身份；
4. 最终规则不匹配时发送前排除当前渠道；
5. 全部候选不满足时拒绝请求且不泄露成本配置。

### 26.3 attempt 与失败分类

1. 同一请求跨多个渠道重试时生成多个 attempt；
2. 收入只归属获胜 attempt，所有 attempt 成本都进入请求总账；
3. 发送前失败进入 `not_dispatched`；
4. 明确不收费响应进入 `confirmed_zero`；
5. 超时、断连和发送后崩溃进入 `cost_unknown`；
6. 缺少权威计量进入 `settlement_failed`；
7. 重复回调、轮询和人工对账不重复结算。

### 26.4 同步与异步

1. 同步非流式和流式请求的收入与成本确认；
2. 客户端断开后继续持久化成本状态；
3. 异步公开 `task_id` 在发送前关联成本账本；
4. `submit_accepted`、`task_succeeded` 和失败零成本规则；
5. `Task.PrivateData` 通过成本请求 ID 完成轮询结算；
6. 供应商已接受但 `Task` 插入失败时保留孤立账本和告警；
7. 关闭功能或停用规则后，既有异步 attempt 继续使用快照。

### 26.5 收入、利润与报表

1. 钱包和订阅均按计费收入等值计算，并保留资金来源；
2. 用户退款后的零收入、负利润和空毛利率；
3. 请求级汇总与渠道级归因重新相加一致；
4. 汇总毛利率使用汇总金额而不是平均请求毛利率；
5. 只有 `complete` 请求参与金额汇总；
6. `profit_recognized_at` 作为默认报表时间；
7. 汇总溢出返回稳定错误而不是环绕。

### 26.6 权限与兼容

1. 管理员 API 授权；
2. 普通用户和公开 API 字段隔离；
3. 成本引用只位于日志 `admin_info`；
4. SQLite、MySQL 和 PostgreSQL 下的迁移、唯一约束、事务和条件更新一致；
5. 新增 Go 测试使用 `testify/require` 和 `testify/assert`。

前端测试覆盖按模式变化的表单、换算预览、版本历史、覆盖检查、attempt 时间线、异常对账、报表筛选、空状态、错误状态、加载状态和响应式文本布局。

所有新增前端文案必须加入全部受支持语言。

## 27. 未来扩展边界

### 27.1 Excel/CSV 导入

Lucen 类 Excel/CSV 导入需要单独设计。未来导入器只能生成 `draft` 规则和售价提案，必须复用本设计的校验、版本激活和审计接口，不能直接更新活动规则或历史账本。

### 27.2 成本表达式

`expression` 是未来保留模式，第一期不能配置或执行。

未来成本表达式必须遵循 `pkg/billingexpr/expr.md` 的版本化、变量边界和安全原则，但使用独立的供应商成本契约，不能与用户售价表达式共享隐式状态。

“分辨率 × 时长 × Token 系数”、缓存 Token 分档、服务等级差价和组合参数定价等超出结构化模式的规则，必须等成本表达式单独设计后再支持。

### 27.3 现金收入与更正分录

实际现金毛利润需要独立的收入确认与分摊设计，覆盖充值、订阅订单、赠送额度、退款、支付费用和税费。

供应商账单对已确认成本的调整需要追加式更正分录，不能直接修改本设计中的已确认 attempt。
