# ARK SDK 视频生成真实配置 E2E 验收报告

## 验收结论

本轮使用 `e2e/testdata/channel-config-v1.json` 中的真实渠道类型、渠道行、上游模型、素材限制、成本模式、供应商原价、币种和汇率。只有供应商 HTTP 响应使用本地 mock，不访问真实供应商，也不会产生真实供应商账单。

98 个导入路由目标全部进入矩阵：

- 26 个目标完成真实网关链路：提交、轮询、任务入库、结算、使用日志、任务日志和成本核算。
- 72 个目标在 `SaveRoutingPolicy` 阶段被生产路由合同阻断，错误码为 `incompatible_channel_contract`，字段为 `targets.0.constraints`。
- 被阻断目标未提交上游，不产生任务、日志、配额数据、成本请求、成本尝试或 mock 调用。

## 真实数据边界

| 数据 | 来源 |
| --- | --- |
| 渠道类型、渠道行 | 导入文件真实值；`CH-4STOKEN` 的历史类型 `1` 定点归一化为专用类型 `209` |
| 客户端模型、上游模型 | 导入文件的 `route_blueprints`、`model_mappings` |
| 素材限制、最少素材 | 导入文件的 `reference_limits`、`reference_minimums` |
| 成本模式、供应商原价、币种、汇率 | 导入文件的 `cost_rule_drafts` |
| 标准化美元价格 | 服务端按导入汇率执行 `NormalizeCostRuleConfig` 计算 |
| 任务、使用日志、任务日志、配额、成本流水 | 真实网关代码，持久化到本地 MySQL |
| 供应商提交和轮询响应 | 本地 mock，仅用于隔离外部网络和账单 |

控制台测试渠道不是统一的 `NewAPIVideo(201)`，类型分布为：

`
1: 1, 200: 1, 202: 1, 203: 1, 204: 2,
205: 1, 206: 1, 207: 3, 209: 1
`

## 矩阵结果

| 项目 | 数量 |
| --- | ---: |
| 路由目标总数 | 98 |
| 完整成功任务 | 26 |
| 提交前协议合同阻断 | 72 |
| mock 上游提交/轮询请求 | 52 |

完整成功任务按真实渠道类型分布：

| 渠道 | 类型 | 成功任务 |
| --- | ---: | ---: |
| 4stoken | 209 | 10 |
| Lucen | 203 | 12 |
| MegaByAI | 204 | 4 |

合同阻断分布为：Cangyuan 10、CLMM 10、Dimensio 10、Paipu 32、Secure 10，共 72 条。阻断原因包括：文生渠道声明了媒体素材、CLMM 声明了音频或不支持的分辨率/模型/时长、Dimensio 声明了未验证模型或不支持的分辨率/素材总量、Secure 分组的最少素材/分辨率/时长/组合素材条件不一致。

素材限制覆盖保持为导入矩阵总量：

| 素材限制 | 覆盖数 |
| --- | ---: |
| `431` | 22 |
| `900` | 4 |
| `903` | 1 |
| `933` | 71 |

## 上游协议覆盖

mock 记录并响应生产适配器使用的提交路径：

- `/v1/video/generations`
- `/v1/videos/generations`
- `/v1/videos`
- `/api/generate-video`

轮询路径覆盖：

- `/v1/video/generations/{id}`
- `/api/task/{id}`
- `/v1/videos/tasks/{id}`
- `/v1/videos/{id}`

每个成功目标均校验渠道 ID、渠道类型、任务平台、上游模型、成本 variant、任务状态、用户额度、Token 额度、渠道额度、`quota_data`、使用日志、任务日志、成本请求和成本尝试。

## 成本核算

本轮成本规则来自导入文件真实成本草稿，不使用 `USD 0.2` 默认占位规则：

| 数据项 | 数量 |
| --- | ---: |
| 活动真实成本规则 | 98 |
| CNY 成本规则 | 98 |
| `normalized_usd_prices` 精确为 `0.2` 的规则 | 0 |
| 成本核算请求 | 26 |
| 成本核算尝试 | 26 |
| settled 尝试 | 26 |

MySQL 实测成本汇总：

`
供应商成本:    18.617356165 USD
计费收入:      36.862012000 USD
计费毛利润:    18.244655835 USD
`

成本尝试保存了规则快照、原币成本、标准化美元成本和结算状态。示例：4stoken `4sdance_fast431/480p` 的供应商原价为 `CNY 3.35`，标准化成本为 `USD 0.458904110`；Lucen `seedance-720p-5s` 为 `CNY 1/秒`，标准化成本为 `USD 0.136986301369863/秒`。

## MySQL 持久化结果

seed 在 `new-api-local-new-api-1` 容器内执行，使用容器环境中的 MySQL `SQL_DSN`。执行结果：

| 数据项 | 数量 |
| --- | ---: |
| 任务记录 | 26 |
| 视频消费使用日志（`type=2`） | 26 |
| `quota_data` 聚合行 | 5 |
| 成本请求 | 26 |
| 成本尝试 | 26 |
| settled 成本尝试 | 26 |

seed 输出：

`
targets: 98, accepted tasks: 26, contract blocks before submit: 72
task rows: 26, usage logs for user: 26, quota_data rows: 5
cost accounting requests: 26, attempts: 26, mock upstream calls: 52
material limits: 431=22, 900=4, 903=1, 933=71
`

## 控制台查看

测试账号：

`
username: ark_sdk_matrix_user
password: local-seed-password
token:    arkmatrixlocal
group:    ark-sdk-material-matrix-local
`

| 内容 | 页面 |
| --- | --- |
| 请求数、用量、余额 | `http://127.0.0.1:3000/dashboard/overview` |
| 使用日志 | `http://127.0.0.1:3000/usage-logs/common` |
| 视频任务日志 | `http://127.0.0.1:3000/usage-logs/task` |
| 利润和成本核算 | `http://127.0.0.1:3000/cost-accounting` |
| 渠道模型成本规则 | `http://127.0.0.1:3000/channels`，筛选 `ark-sdk-matrix-mock-` 后打开“模型成本” |

## 验证命令

`
go test ./cmd/ark-video-material-seed ./service ./relay/... -count=1
go test ./e2e -run 'TestSeedanceImportedMaterialMatrixFullFlowE2E|TestSeedanceCapabilityRoutingMatrixE2E|TestSeedanceNativeSeedance20MultimodalE2E' -count=1 -v
docker compose -f docker-compose.local.yml up -d --build new-api
git diff --check
`

## 残余风险

- 本轮不验证真实供应商 endpoint 的可用性、鉴权和响应漂移；供应商 HTTP 仍为本地 mock。
- 72 个目标已经被提交前合同门禁阻断，不能在合同证据补齐前发布为可用路由。
- `8yes` 的渠道类型、协议和素材限制仍缺少已验证证据，因此没有进入本轮路由目标。
