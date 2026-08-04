# Seedance 官方 Token 售价根治设计

## 目标

Seedance 面向用户的售价严格按火山引擎官方公式结算：

```text
用户售价 = 官方 USD/1M Token × 总 Token / 1,000,000 × 分组倍率
总 Token = ceil((输入视频时长 + 输出视频时长) × 输出宽 × 输出高 × 帧率 / 1024)
```

输入视频时长必须参与 `with_video` 场景的 Token 用量。供应商成本继续由渠道成本规则独立计算，不得反向影响用户售价。

## 根因

现有实现把官方表中已舍入的“元/秒”快查值导入 `DurationPrice.Scenarios.output_price`，运行时只按输出时长收费。该合同同时丢失输入视频 Token 和官方 `元/1M Token` 精度，导致 `480p + 3 秒输入 + 4 秒输出` 的 Mini 场景从正确的 `$0.168546575342` 低估为 `$0.096575342466`。

## 架构

新增独立计费模式 `seedance_tokens` 和设置项 `billing_setting.seedance_token_price`。每个模型按 `<resolution>:<scenario>` 保存：

- `price_per_million`：官方精确 `USD/1M Token`。
- `width`、`height`、`frame_rate`：渠道模板模型 SKU 中的官方输出参数。
- `pricing_version`、`source`：官方价格审计来源。

任务提交阶段先完成素材时长校验和 Seedance Token 计算，再从冻结的官方场景价格计算用户配额。任务私有快照保存输入 Token、输出 Token、总 Token、官方单价、几何参数、基础售价、分组倍率和最终售价。成本预览、利润路由和日志读取同一合同。

## 删除策略

- 删除 Seedance 对 `per_duration`、`DurationPrice.Scenarios` 和 `output_price` 的依赖。
- 发布官方 Seedance 售价时，清理 Seedance 在 `ModelPrice`、`ModelRatio`、`CompletionRatio`、`billing_expr` 和 `duration_price` 中的旧规则。
- 不提供旧场景秒价兼容或回退；缺少新合同必须在请求上游前返回 `seedance_sale_price_not_configured`。
- 非 Seedance 的普通 `per_duration` 计费保持不变。

## 数据流

1. 模板生成器从 `sd官价` 读取未舍入的 `元/1M Token`，换算并写入 `USD/1M`。
2. 转换器生成 `seedance_token_price` 提案，不再生成 Seedance `duration_price`。
3. 导入暂存校验官方来源、场景、分辨率、单价和几何参数。
4. 发布事务合并不同场景，同场景冲突则整批回滚，并删除旧 Seedance 售价规则。
5. 请求校验后计算并冻结 Token 用量，按官方 Token 单价预扣。
6. 成功任务保留冻结售价；失败任务按既有任务退款链路退款。
7. 使用日志和任务日志展示官方公式中的全部输入事实。

## 验收标准

- Mini `480p + with_video + 输入 3000 ms + 输出 4 秒 + 1.25x` 的最终售价为 `$0.168546575342`，对应配额按统一配额函数转换。
- 参考视频时长变化会按官方公式改变用户售价。
- 模板和导入 JSON 中 Seedance 不再出现 `duration_price`、`output_price` 或 `per_duration`。
- 运行时 Seedance 售价日志包含 `price_per_million`、输入/输出/总 Token、宽高、帧率、基础售价和最终售价。
- 所有渠道成本、利润和 E2E 继续使用真实导入数据，仅上游 HTTP 响应使用 mock。
