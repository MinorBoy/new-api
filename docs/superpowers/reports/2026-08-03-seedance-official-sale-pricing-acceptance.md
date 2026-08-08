# Seedance 官方 Token 售价根治 E2E 验收报告

## 验收结论

本轮已完成 Seedance 用户售价根治、渠道模板重生成、正式导入、真实渠道素材矩阵 E2E 和控制台核验。Seedance 面向用户的唯一售价合同为：

```text
总 Token = ceil((输入视频时长 + 输出视频时长) × 输出宽 × 输出高 × 输出帧率 / 1024)
用户基础售价 = 官方 USD/1M Token × 总 Token / 1,000,000
最终用户售价 = 用户基础售价 × 分组倍率
```

输入视频时长为 0 时使用 `no_video` 官方单价，存在参考视频时使用 `with_video` 官方单价。供应商成本继续按渠道成本规则独立结算，不能参与或反推用户售价。因此，真实供应商成本高于用户收入时会保留负毛利，不再通过隐式倍率修正售价。

旧 Seedance `per_duration`、`output_price`、`seedance_price_matrix`、模型倍率和计费表达式售价均已删除，不提供兼容回退；缺少官方 Token 售价配置时，请求必须在调用上游前失败。

## 输入与生成物

| 数据 | 结果 |
| --- | --- |
| 源表 | `docs/new-channels/sd收录.xlsx` |
| 源表 SHA256 | `361565061dc8b7683adb730451a1a2e9597b52cdf70031991443a0097a0a4eea` |
| 渠道模板 | `outputs/2026-08-03-import/渠道模型成本与利润模板-v1.xlsx` |
| 渠道模板 SHA256 | `2af369a9586a1509b34366bd6475e485ef770f2036dd14fc6d004be2d47741fe` |
| 模板报告 SHA256 | `861c4483643c36a031af17c71aeb490cbaf19baac37afde905c6162474082cc` |
| 导入配置 | `e2e/testdata/channel-config-v1.json` |
| 导入配置 SHA256 | `8693138b519edca96cff3bc811093483b7713735578fcfc57e61ddc7ad17aa73` |
| 导入语义载荷 SHA256 | `7cbab1db2c8dc1a7cbbb5e24a39124a92fc0bb5e3db5908bde7fabb0d37440ce` |

导入实体如下：

| 实体 | 数量 |
| --- | ---: |
| 渠道 | 10 |
| 渠道线路 | 14 |
| 模型 SKU | 8 |
| 官方售价场景 | 16 |
| 成本规则草稿 | 147 |
| 模型映射 | 147 |
| 路由蓝图 | 147 |
| 来源 | 13 |
| 未解析价格变体 | 0 |

16 条官方售价场景从 `sd官价` 工作表读取精确原始 Token 单价，并换算成 `USD/1M Token`。每个场景同时冻结分辨率、宽、高、帧率、价格版本和来源，不再从已舍入的元/秒价格或 720p 基准价推导。唯一数据告警是 Dimensio `pxv-seedance-2.0-standard / 480p` 原币按秒价格为 0；该成本草稿保持禁用，不形成活动成本规则。

## 正式导入

正式导入批次为 `15`，状态为 `published`。16 条价格组告警均已标记为 `resolved`，不存在开放问题。

发布后运行配置核验：

- 4 个 Seedance 客户端模型的 `billing_setting.billing_mode` 均为 `seedance_tokens`。
- `billing_setting.seedance_token_price` 保存 16 个官方分辨率与素材场景。
- `billing_setting.duration_price` 为空，不存在 Seedance 按时长售价。
- `billing_setting.billing_expr` 为空，不存在 Seedance 表达式售价。
- Seedance 旧 `ModelPrice`、`ModelRatio`、`CompletionRatio` 和供应商别名售价已删除。
- 同模型同场景价格冲突会阻断整批发布，不做旧规则兼容或静默覆盖。

提交前配置接口审查还发现并修复了两处保护遗漏：`GetOptions` 现在从当前结构化配置序列化 `billing_setting.seedance_token_price`，不会返回陈旧的原始配置字符串；`UpdateOption` 在持久化前校验 Seedance 官方 Token 售价合同，禁止绕过配置导入直接新增、删除或修改 Seedance 售价。对应回归测试覆盖了读取当前合同和拒绝手工覆盖两条路径。

## E2E 覆盖

本轮使用导入后的真实渠道类型、渠道线路、客户端模型、上游模型、素材限制、成本模式、供应商原价、币种和汇率。只有供应商 HTTP 提交及轮询响应使用本地 mock；任务提交、素材合同校验、能力路由、用户扣费、任务轮询、使用日志、任务日志、成本核算和利润识别均执行生产代码。

最新持久化执行时间为 `2026-08-04 12:12:55`，结果如下：

| 项目 | 数量 |
| --- | ---: |
| 导入路由目标 | 147 |
| 成功任务 | 110 |
| 失败任务 | 1 |
| 提交前渠道合同阻断 | 36 |
| 零价格禁用草稿 | 1 |
| 终态公开结果 | 111 |
| 成本核算请求 | 111 |
| 成本核算尝试 | 111 |
| mock 上游调用 | 222 |

合同阻断分布为 Cangyuan 12、CLMM 18、8yes 1、MegaByAI 4、Secure 企业组 1。素材限制覆盖 `431=38`、`900=6`、`903=4`、`933=99`。36 条阻断均发生在渠道合同校验阶段，没有产生上游调用和计费流水。

## Token 售价核验

110 条成功任务全部使用 `official-token-v1`，并满足以下不变量：

- `input_tokens=0`，`completion_tokens=output_tokens=total_tokens`，完整 Token 用量与冻结的输入时长、输出时长、宽、高、帧率一致。
- `base_charge = price_per_million × total_tokens / 1,000,000`。
- `final_charge = base_charge × 1.25`，公式误差记录为 0。
- 最终配额由统一配额转换函数生成，并与使用日志、任务快照和成本核算请求一致。
- 参考视频时长参与完整的生成 Token 公式，但不产生输入 Token。

典型快照为 Seedance 2.0 Mini `480p + with_video + 输入 3000 ms + 输出 4 秒`：

```text
width=864, height=496, frame_rate=24
input_tokens=0
completion_tokens=output_tokens=ceil(7 × 864 × 496 × 24 / 1024)=70308
total_tokens=completion_tokens=70308
price_per_million=1.917808219178082 USD
base_charge=1.917808219178082 × 70308 / 1,000,000
           =0.134837260273972589256 USD
group_ratio=1.25
final_charge=0.16854657534246573657 USD
```

这取代了旧报告中只按输出 4 秒计算出的 `$0.0965753424657535`。旧值漏计 3 秒输入视频 Token，不符合官方公式。

任务终态用量还执行统一信任边界：上游必须返回相等的 `completion_tokens` 和 `total_tokens`，且该值与冻结公式一致，才能作为可信上游用量；否则按请求时长和冻结几何重算。例如 720p、输入 5 秒、输出 10 秒会得到 `input_tokens=0`、`completion_tokens=output_tokens=total_tokens=324000`，不会采用旧式拆分或与公式不一致的用量。

用户售价用量与供应商成本计量在任务终态明确分离：上游同时返回实际时长和 Token 时，用户售价仍只接受冻结的官方 Token 公式；供应商 `per_token` 成本读取终态上游 Token 快照，供应商 `per_duration` 成本读取上游时长快照。上游 Token 与冻结售价不一致不会再造成供应商成本计量丢失或错误改用另一种计量源。

## 日志与成本核算

测试用户 `ark_sdk_matrix_user`、分组 `ark-sdk-material-matrix-local` 的持久化数据如下：

| 数据 | 数量/金额 |
| --- | ---: |
| 视频使用日志 `type=2` | 111 |
| 任务日志 | 111 |
| 已保存终态公开结果 | 111 |
| 成本状态 `settled` | 110 |
| 成本状态 `confirmed_zero` | 1 |
| 成本状态 `settlement_failed` | 0 |
| 利润状态 `complete` | 111 |
| 单次尝试 `attempt_count=1` | 111 |
| 负毛利请求 | 39 |
| 计费收入 | `$77.524162000` |
| 供应商成本 | `$67.982958905` |
| 计费毛利润 | `$9.541203095` |
| 整体毛利率 | `12.3074%` |

额外失败样本经过真实提交、轮询、退款和零成本确认链路。任务详情保存并展示最终公开结果：

```json
{
  "status": "failed",
  "error": {
    "code": "content_policy_violation",
    "message": "mock content policy rejection"
  }
}
```

成功任务详情保存 `succeeded` 和最终视频 URL。111 条公开结果均未泄漏渠道密钥、上游任务 ID、`channel_id` 或 `api_key`。本轮 seed 退出前额外校验成本请求数与任务数一致、每条成功任务均已 `settled`、失败任务均为 `confirmed_zero`、不存在 `settlement_failed`，并校验收入减供应商成本等于毛利润。

## 控制台核验

已登录 `http://127.0.0.1:3000` 逐页检查：

| 页面 | 核验结果 |
| --- | --- |
| `/dashboard/overview` | 显示近 24 小时请求数 111 和真实消耗 |
| `/usage-logs/common?type=["2"]` | 显示测试用户、测试分组、真实渠道、模型和用户消费 |
| `/usage-logs/task` | 显示 110 条成功任务和 1 条失败任务，详情包含最终公开任务结果 |
| `/cost-accounting` | 显示收入、供应商成本、毛利润和毛利率；正值绿色、负值红色 |
| `/channels` | 显示各真实供应商协议类型，不再统一创建为 `NewAPIVideo` |

## 验证命令

```text
docker exec -w /data new-api-local-new-api-1 /data/ark-video-material-seed
go test ./... -count=1 -p=1
cd web && bun test --parallel=1
cd web && bun run typecheck
cd web && bun run build
cd web && bun run lint
```

持久化 Seed E2E 已生成上述 111 条完整链路数据，且成本结算断言全部通过。提交前最终验证结果：Go 全量测试通过；前端全量测试 `469 pass / 0 fail`；TypeScript 类型检查通过；生产构建通过；本轮 30 个变更前端文件定向 lint 为零错误。全仓 `bun run lint` 仍被仓库既有、且不在本轮改动范围内的错误阻断，因此不作为本轮通过项。
