# ARK SDK 视频生成真实配置 E2E 验收报告

## 验收结论

本轮已基于最新版 `docs/new-channels/sd收录.xlsx` 重新生成渠道模板和 E2E 配置，并导入本地 Docker MySQL 环境完成全链路验收。

E2E 使用真实渠道类型、渠道行、客户端模型、上游模型、素材限制、成本模式、供应商原价、币种和汇率。只有供应商 HTTP 提交与轮询响应使用本地 mock，不访问真实供应商，也不会产生真实供应商账单。

147 个导入路由目标全部进入矩阵：

- 110 个目标完成提交、轮询、任务入库、结算、使用日志、任务日志和成本核算。
- 36 个目标被生产渠道合同在提交前阻断，错误码为 `incompatible_channel_contract`，未产生上游调用和计费流水。
- 1 个 Dimensio `480p` 目标因供应商原价为零保持禁用，未物化活动成本规则，也未进入路由执行。
- 110 个成功任务均为 `SUCCESS`，110 个成本请求均完成收入结算和利润确认。

## 输入与生成物

| 文件 | SHA256 |
| --- | --- |
| `docs/new-channels/sd收录.xlsx` | `0e08346795d24266a600e89aedb775ec8cf561ba1dd7f37f2022ae64b96be249` |
| `outputs/2026-08-02-import/渠道模型成本与利润模板-v1.xlsx` | `8d80213ab8419e2283edec93019df45ba34b9f85d3d7b8b37e52dcddd3954fc2` |
| `outputs/2026-08-02-import/渠道模型成本与利润模板-v1.report.json` | `e849d589dd7fbd7be36cea98bead5b88dca82b5a9e3bf0a229ccbda28cabf4e5` |
| `e2e/testdata/channel-config-v1.json` 文件 | `997647b44fb818175f013bb29f69ced372e32661d1673029498a3e78bf8fbd46` |
| 配置清单 `payload_sha256` | `d81cda25a0c68173541f63f27c34ed8296f5259cfd02c54ea063919538855182` |

配置实体数量：

| 实体 | 数量 |
| --- | ---: |
| 渠道 | 10 |
| 渠道行 | 14 |
| 模型 SKU | 8 |
| 售价提案 | 16 |
| 成本规则草稿 | 147 |
| 模型映射 | 147 |
| 路由蓝图 | 147 |
| 来源 | 13 |
| 未解决价格变体 | 0 |

## 真实数据边界

| 数据 | 来源 |
| --- | --- |
| 渠道类型、渠道行 | 模板导入真实值；不再统一创建为 `NewAPIVideo(201)` |
| 客户端模型、上游模型 | 导入配置的 `route_blueprints`、`model_mappings` |
| 素材限制、最少素材 | 导入配置的 `reference_limits`、`reference_minimums` |
| 成本模式、供应商原价、币种、汇率 | 导入配置的 `cost_rule_drafts` |
| 标准化美元价格 | 服务端按导入汇率执行 `NormalizeCostRuleConfig` 计算 |
| 任务、日志、配额、成本流水 | 真实网关代码，持久化到本地 Docker MySQL |
| 供应商提交和轮询响应 | 本地 mock，仅隔离外部网络、鉴权和账单 |

视频素材通过测试 HTTP 客户端下载真实 MP4 fixture，并由视频元数据服务解析；音频素材通过测试 HTTP 客户端下载真实 WAV fixture，并执行时长解析。mock 边界仅覆盖供应商任务提交与轮询响应。

导入后 14 条渠道行的真实渠道类型分布为：

```text
200:1, 202:1, 203:1, 204:3, 205:1,
206:1, 207:3, 208:1, 209:1, 210:1
```

## 矩阵结果

| 渠道行 | 类型 | 目标 | 成功 | 合同阻断 | 价格禁用 |
| --- | ---: | ---: | ---: | ---: | ---: |
| 4stoken | 209 | 26 | 26 | 0 | 0 |
| 8yes | 210 | 12 | 11 | 1 | 0 |
| Cangyuan | 205 | 12 | 0 | 12 | 0 |
| CLMM | 202 | 18 | 0 | 18 | 0 |
| Dimensio | 200 | 12 | 11 | 0 | 1 |
| Lucen | 203 | 12 | 12 | 0 | 0 |
| MegaByAI | 204 | 7 | 7 | 0 | 0 |
| MegaByAI 无真人 | 204 | 4 | 2 | 2 | 0 |
| MegaByAI 真人 | 204 | 10 | 8 | 2 | 0 |
| OmegaAI | 208 | 4 | 4 | 0 | 0 |
| Paipu | 206 | 20 | 20 | 0 | 0 |
| Secure 折扣组 | 207 | 5 | 5 | 0 | 0 |
| Secure 企业组 | 207 | 1 | 0 | 1 | 0 |
| Secure 海外组 | 207 | 4 | 4 | 0 | 0 |
| **合计** |  | **147** | **110** | **36** | **1** |

36 条阻断均为当前导入数据与已验证供应商协议合同不一致，并非导入器误删或路由运行失败：

- Cangyuan 12 条声明了该文生协议不支持的媒体素材输入。
- CLMM 18 条在模型语法、分辨率、时长或素材组合上不符合已验证合同。
- 8yes 1 条的模型后缀分辨率与声明分辨率冲突。
- MegaByAI 4 条声明了该协议不支持的 `1080p` 或 `4k`。
- Secure 企业组 1 条不符合企业组时长或素材约束。

素材限制覆盖：

| 素材限制 | 覆盖数 |
| --- | ---: |
| `431` | 38 |
| `900` | 6 |
| `903` | 4 |
| `933` | 99 |

## 协议根治

本轮根治了 E2E 暴露的渠道协议和 mock 合同问题：

- OmegaAI 增加 `720p` 路由合同，并覆盖真实提交路径 `/v1/media/generate` 与轮询路径 `/v1/tasks/{id}`。
- Dimensio 轮询完成态统一为上游真实合同值 `completed`。
- 8yes 增加模型后缀分辨率一致性校验。
- MegaByAI 明确限制为 `480p/720p`。
- 素材输入模式按真实素材能力生成，不再由旧默认值制造冲突。
- 混合路由策略中，视频元数据服务不可用时只排除依赖视频总时长的目标，不再连带阻断不依赖该元数据的可用目标。
- 成本草稿增加显式 `enabled`；零价格草稿保持禁用，不生成成本规则或售价。
- 删除旧“渠道 + 模型价格冲突”兼容逻辑，价格合同以稳定 hash 生成 `cost_variant_key`，前后端 payload hash 规范保持一致。
- V1 转换器完整保留 `charge_event`、`meter_source`、`token_mode`；异步视频按次规则不再因字段丢失回退为 `response_succeeded`，本轮 146 条活动成本规则的计费事件均为 `task_succeeded`。
- NOV/VID 场景合并只比较实际进入运行时计费合同的字段，并将 Excel 日期对象和序列值统一到秒级时间戳；描述字段差异不会制造假冲突，真实价格或计费合同差异仍以 `COST_SCENARIO_CONFLICT` 阻断。
- 正式 Stage 对同一 `channel/model/cost_variant_key` 的完全相同成本合同复用同一物化规则，不再依赖 `no_video/with_video` 场景配对；不同合同仍报告 `COST_VARIANT_AMBIGUOUS`。
- 同一标准模型对应多个上游模型时，渠道模型集合保留标准模型和全部上游模型，但不再写入有歧义的旧单值 `ModelMapping`；实际路由由结构化目标中的 `UpstreamModel` 决定。
- 正式 Publish 将 146 条启用成本草稿原子激活为 146 条活动规则，并在同一事务中发布售价、渠道模型快照和结构化路由目标。
- 导入文档明确禁用某个 `channel/upstream_model/cost_variant_key` 时，Publish 会淘汰同键旧活动规则并刷新成本缓存；该键同时纳入基线并发校验，发布期间发生修改会按 `STALE_BASE_VERSION` 拒绝，并将批次退回 `staged` 以允许重新核验。
- 发布后成本覆盖告警只统计本批次影响的渠道，不再把全库其他渠道的缺价状态错误挂到当前导入批次。

mock 提交路径覆盖：

- `/v1/video/generations`
- `/v1/videos/generations`
- `/v1/videos`
- `/v1/media/generate`
- `/api/generate-video`

mock 轮询路径覆盖：

- `/v1/video/generations/{id}`
- `/api/task/{id}`
- `/v1/tasks/{id}`
- `/v1/videos/tasks/{id}`
- `/v1/videos/{id}`

## 成本核算

147 条成本草稿中，146 条启用草稿在 Stage 物化为 146 条待发布规则。MegaByAI 两个不同 `route_target_ref` 使用不同稳定成本变体，保持为两个独立规则，不再被旧场景兼容逻辑错误合并。正式 Publish 后 146 条规则全部转为活动状态，残留草稿为 0。Dimensio `COST-DIMENSIO-R101-480-DUR` 的 `price_per_second=0`，保持禁用且活动规则数量为 0。

正式批次 11 通过 `/api/config-import/batches` 完成上传、14 条渠道绑定、价格组确认、Stage 校验和 Publish，最终状态为 `published`，`payload_sha256` 为 `d81cda25...`，批次开放问题为 0。Stage 未产生 `COST_VARIANT_AMBIGUOUS` 或 `MODEL_SNAPSHOT_MAPPING_CONFLICT`；模板生成仅保留 1 个预期的 `COST_PRICE_INVALID` 警告，对应上述零价格禁用草稿。`/validate` 与 Stage 使用同一服务端校验操作；当 Stage 已直接进入 `ready` 时，允许动作只剩 Publish，不重复执行 Stage。

正式 Publish E2E 进一步验证了 146 条规则激活、多上游模型不写入歧义单值映射，以及 Dimensio 两个具体上游模型仍存在于结构化路由目标中。随后通过生产 `SaveRoutingPolicy` 启用已发布的 Dimensio `720p` 目标并执行真实渠道选择，最终路由上下文使用 `jmg-video-seedance-2.0-vip` 及其对应 `cost_variant_key`。

| 数据项 | 数量 |
| --- | ---: |
| 活动真实成本规则 | 146 |
| CNY 成本规则 | 146 |
| `normalized_usd_prices` 精确为 `0.2` | 0 |
| `source=local_seed` 活动规则 | 0 |
| 残留草稿规则 | 0 |
| `charge_event=task_succeeded` | 146 |
| 按时长规则 | 79 |
| 按次规则 | 64 |
| 按 Token 规则 | 3 |
| 成本核算请求 | 110 |
| 有 winning attempt 的请求 | 110 |
| 成本核算尝试 | 110 |
| `rule_id` 为空 | 0 |
| `cost_variant_key` 为空 | 0 |
| settled 尝试 | 110 |

MySQL 实测汇总：

```text
供应商成本: 66.713589042 USD
计费收入:   56.905486000 USD
计费毛利润: -9.808103042 USD
整体毛利率: -17.2358%
单次成本范围: 0.049315068 USD - 6.164383562 USD
不同确认成本值: 75
```

负毛利是本轮真实供应商成本与当前正式售价组合的实际核算结果，不使用占位价格，也不在 E2E 中改写为正利润。该结果应作为后续售价或路由利润门槛调整的输入。

真实价格示例：

```text
CNY 0.15/秒 -> USD 0.02054794520547945/秒
CNY 3.5/次  -> USD 0.4794520547945205/次
CNY 8.9/次  -> USD 1.2191780821917807/次
汇率        -> 0.136986301369863
```

## MySQL 与控制台核验

seed 在 `new-api-local-new-api-1` 容器内执行，使用容器配置的 MySQL。结果如下：

| 数据项 | 数量/状态 |
| --- | ---: |
| 任务记录 | 110 |
| `SUCCESS` 任务 | 110 |
| 视频消费使用日志（`type=2`） | 110 |
| `quota_data` 聚合行 | 27 |
| 成本请求 | 110 |
| 成本尝试 | 110 |
| 收入状态 | 110 `settled` |
| 利润状态 | 110 `complete` |
| mock 上游请求 | 220 |
| 正式导入批次 | 11 `published` |
| 批次开放问题 | 0 |

控制台已逐页核验：

| 页面 | 核验结果 |
| --- | --- |
| `http://127.0.0.1:3000/dashboard/overview` | 显示 110 个请求和本轮消费统计 |
| `http://127.0.0.1:3000/usage-logs/common` | 显示 `ark_sdk_matrix_user`、测试分组、真实渠道和消费金额 |
| `http://127.0.0.1:3000/usage-logs/task` | 显示成功任务、真实渠道类型、任务 ID 和 100% 进度 |
| `http://127.0.0.1:3000/cost-accounting` | 显示收入 `$56.905486`、供应商成本 `$66.713589042`、毛利 `-$9.808103042` 和毛利率 `-17.2358%` |
| `http://127.0.0.1:3000/channels` | 显示 14 条真实渠道行类型；“模型成本”显示 CNY 原价、标准化 USD 和成本变体 |

成本核算页顶部仍显示全局“未覆盖 2 项”，对应现有的两个非本批次 `mikoto/gpt-image-2` 渠道（ID 41、42），不属于 ARK 视频导入。批次 11 自身的成本覆盖开放问题为 0；本轮未删除或修改这两个无关渠道。

测试账号：

```text
username: ark_sdk_matrix_user
password: local-seed-password
token:    arkmatrixlocal
group:    ark-sdk-material-matrix-local
```

## 验证命令

```text
cd web
bun test --parallel=1 src/channel-config-converter scripts/channel-model-template
bun run typecheck
bun run build

cd ..
go test ./pkg/modelrouting ./relay ./service ./cmd/ark-video-material-seed -count=1
go test ./e2e -count=1
git diff --check
```

## 残余边界

- 本轮不验证真实供应商 endpoint 的在线可用性、鉴权和响应漂移；供应商 HTTP 仍为本地 mock。
- 36 条合同阻断必须修改源数据或补充经过验证的供应商协议证据后才能放行，不应通过兼容逻辑绕过。
- seed 逐目标替换测试路由策略，因此数据库最终只保留最后一组活动测试目标；110 条历史任务、日志和成本流水均保留完整路由与成本证据。
- 当前真实价格组合的整体毛利为负，发布前应结合业务目标调整售价、渠道优先级或最低期望毛利门槛。
