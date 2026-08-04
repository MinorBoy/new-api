# Ark SDK 视频素材矩阵重导入 E2E 验收报告

## 验收结论

本轮已使用更新后的 `sd收录.xlsx` 重新生成渠道模板，正式导入批次 `18`，并完成 Ark SDK 视频生成素材矩阵 E2E。新增渠道 11（Z5API）已按真实渠道类型 `211` 发布和运行；11 个供应商、13 条渠道线路、真实上游模型、素材限制、成本模式、原币价格与汇率均来自最新模板。只有供应商 HTTP 提交与轮询响应使用本地 mock，其余任务提交、合同校验、能力路由、用户计费、任务轮询、使用日志、任务日志、供应商成本和利润识别均执行生产代码。

本轮共读取 155 个路由目标：118 个通过渠道合同并完成成功任务，36 个在提交上游前被渠道合同阻断，1 个明确零价格草稿保持禁用。另增加 1 个真实失败链路样本，最终持久化 119 条任务、119 条使用日志、119 条成本请求和 119 条成本尝试；不存在成本结算失败或利润不完整。

## 输入与生成物

| 数据 | 结果 |
| --- | --- |
| 源表 | `docs/new-channels/sd收录.xlsx` |
| 源表 SHA256 | `5026d48bd1e63094c0c585cb6294e0af64009cfed70bce90e08b7d3b2c073862` |
| 渠道模板 | `outputs/2026-08-04-import/渠道模型成本与利润模板-v1.xlsx` |
| 渠道模板 SHA256 | `773e21cf32c54e96be74f6b729869f9f2dd1b5f1e20f9a1f42a85ccedb5f4830` |
| 模板报告 SHA256 | `a56cd203991247683ee186c031bbce349e1c46d35cd88f890940c4fc443457f4` |
| 导入配置 | `e2e/testdata/channel-config-v1.json` |
| 导入配置文件 SHA256 | `c8d4e62f424bd4932ffe89a696cd05b7a63bbff9d31624e6846355cd09cb2a3b` |
| 导入语义载荷 SHA256 | `71dc124ba8b3fdd4da1078cc95e259c67c91d19f4692531f5a15d170be0676a5` |

生成与导入实体如下：

| 实体 | 数量 |
| --- | ---: |
| 渠道 | 11 |
| 渠道线路 | 13 |
| 模型 SKU | 8 |
| 官方售价场景 | 16 |
| 成本规则草稿 | 155 |
| 模型映射 | 155 |
| 路由蓝图 | 155 |
| 来源 | 14 |
| 未解析价格变体 | 0 |

模板生成器已兼容源表将 `计费` 重命名为 `计费方式`、新增 `上游模型分组` 的结构变化，并将渠道编号 11 映射为 `CH-Z5API`。唯一模板告警为 `sd!101` 缺少正数秒价；该行在源表中是明确禁用的 Dimensio 草稿，不会发布活动成本规则。

## 权威导入与旧数据清理

正式导入批次 `18` 状态为 `published`，16 条 `PRICING_GROUP_SCOPE_UNREPRESENTABLE` 告警均已解决，开放问题为 0。

本轮将渠道模板视为受影响渠道的权威全量快照，不再对旧“渠道 + 模型价格”做兼容：

- 发布时退休受影响渠道中不在最新模板权威键集合内的全部活动成本规则。
- 基线捕获覆盖受影响渠道的全部活动成本规则，保证发布冲突检查和回滚语义完整。
- 只有“该线路 + 上游模型仅存在明确禁用成本，且不存在任何启用价格变体”时才排除模型映射；同模型仍有 720p、1080p 或 4K 启用变体时保留模型。
- E2E 种子会禁用本轮源表已删除的旧测试渠道，并退休其活动成本规则。旧渠道 36、43 当前均为手工禁用且活动成本为 0。
- 成本覆盖统计同时要求能力启用和渠道启用，已禁用渠道的残留能力不再制造 `COST_COVERAGE_INCOMPLETE`。

发布后共有 13 条启用渠道线路、154 条活动成本规则和 109 个“渠道 + 上游模型”组合。155 条草稿中 1 条明确禁用；154 条活动规则按成本模式分布为：按时长 80、按次 71、按 Token 3。供应商原币均为 CNY，并按每条规则冻结的汇率标准化为 USD，不存在统一 `$0.2` 占位价格。

当前全局成本覆盖只剩渠道 41、42 的 `gpt-image-2` 缺口，与本轮视频渠道无关；批次 18 的全部启用视频渠道均具备权威成本覆盖。

## E2E 覆盖

终态响应根治后的最新持久化执行时间为 `2026-08-04 23:39:05` 至 `2026-08-04 23:40:33`，结果如下：

| 项目 | 数量 |
| --- | ---: |
| 导入路由目标 | 155 |
| 通过渠道合同的成功任务 | 118 |
| mock 失败任务 | 1 |
| 提交前渠道合同阻断 | 36 |
| 零价格禁用草稿 | 1 |
| 终态用户响应 | 119 |
| mock 上游调用 | 238 |

素材限制覆盖为：

| 素材码 | 数量 |
| --- | ---: |
| 431 | 41 |
| 900 | 6 |
| 903 | 4 |
| 933 | 104 |

36 条合同阻断分布为：Cangyuan 12、CLMM 18、8yes 1、MegaByAI 4、Secure 企业组 1。阻断全部发生在渠道合同校验阶段，没有上游调用和计费流水。其余渠道线路均按最新模板完成真实协议路由；其中新增 Z5API 类型 `211` 完成 6 条成功任务。

## 售价与供应商成本

Seedance 用户售价继续使用官方 Token 公式，并与供应商成本完全独立：

```text
总 Token = ceil((输入视频时长 + 输出视频时长) × 输出宽 × 输出高 × 输出帧率 / 1024)
用户基础售价 = 官方 USD/1M Token × 总 Token / 1,000,000
最终用户售价 = 用户基础售价 × 分组倍率
```

测试分组倍率为 `1.25`。供应商成本分别按模板中的按时长、按次或按 Token 规则结算，不参与用户售价计算，也不会生成 `seedance_price_matrix=0.91` 一类推导倍率。终态上游用量仅在成本规则确实需要终态时长或 Token 时进入供应商成本计量；用户售价使用本地官方公式重算时，可信的上游成本计量仍独立保留。

## 日志与成本核算

测试用户 `ark_sdk_matrix_user`、分组 `ark-sdk-material-matrix-local` 的持久化结果如下：

| 数据 | 数量/金额 |
| --- | ---: |
| 视频使用日志 `type=2` | 119 |
| 任务日志 | 119 |
| 已保存终态用户响应 | 119 |
| 成本状态 `settled` | 118 |
| 成本状态 `confirmed_zero` | 1 |
| 成本状态 `settlement_failed` | 0 |
| 利润状态 `complete` | 119 |
| 单次尝试 `attempt_count=1` | 119 |
| 负毛利请求 | 43 |
| 计费收入 | `$82.172270000` |
| 供应商成本 | `$72.687068494` |
| 计费毛利润 | `$9.485201506` |
| 整体毛利率 | `11.5431%` |

## 终态响应结构

根治前 118 条成功任务中只有 25 条具备完整 Ark 结构，另外 93 条为简化响应：4SToken 26、8yes 11、Dimensio 11、Paipu 20、MegaByAI 19、Z5API 6。任务日志没有裁剪字段，缺陷来自两个后端入口的响应规范化不一致：查询接口经过 `seedanceTaskResponse`，后台轮询审计则按创建路径直接保存 OpenAI 或 Ark 转换结果。

本轮将完整规范化下沉为 `service.NormalizeSeedanceTaskResponse`，由用户查询和后台轮询审计共同调用。Seedance 任务无论通过 `/v1/video/generations` 还是 Ark 任务路径创建，轮询审计都固定保存 Ark 结构，不再由创建路径决定任务日志格式。OpenAI 查询入口遇到 Seedance 任务时也会重新生成完整 Ark 审计，不能再用 OpenAI 简化响应覆盖已经保存的任务详情。

Seedance 识别不再只依赖共享渠道类型，而是要求渠道平台之外还存在 Ark 官方请求路径、冻结的 Seedance 计费档案或可识别的 Seedance 模型证据。共享 `NewAPIVideo`、MegaByAI 等渠道承载的普通视频任务仍保持原 OpenAI 响应合同。DoubaoVideo 和 VolcEngine 使用的 Doubao adaptor 已补齐 Ark 转换器，避免这两类真实 Seedance 任务在后台轮询终态静默缺失审计。Doubao Ark 转换只构造官方字段白名单，不再把原始上游响应中的诊断字段、供应商账号或上游模型身份直接公开。

字段来源优先级为“上游有效事实、用户请求快照、冻结计费快照、固定默认值”。公开模型身份固定优先使用任务属性中的客户端模型，再回退冻结计费快照中的客户端模型，不接受上游非空模型覆盖。事实不可得时强制使用 `seed=0`、`resolution=720p`、`ratio=16:9`、`duration=5`、`framespersecond=24`、`service_tier=default`、`execution_expires_after=172800`、`generate_audio=true`、`draft=false`、`priority=0` 和零值 Token 用量。成功终态的 `content.video_url` 必须来自上游有效事实或已保存的 `ResultURL`；两者均为空时规范化直接失败，不会保存伪造或空地址。非法、负数或空时间先按 `SubmitTime/CreatedAt`、`FinishTime/UpdatedAt` 回退，仍无事实时显式返回 `0`。损坏的版本化 Token 快照在任务详情展示层回退为零值，但轮询结算继续严格拒绝，避免展示 500 的同时保持计费安全。成功终态删除上游遗留 `error`，失败终态删除 `content`；失败状态允许 Ark 合法的 `failed`、`expired`、`cancelled`，失败消息统一脱敏，不向用户响应和任务日志暴露私有上游任务 ID。该补全只作用于成功或失败终态，排队和运行中响应不会提前注入终态默认字段。

Ark 任务列表在数据库层按请求路径、冻结计费档案和 Seedance 模型证据筛选，兼容 SQLite、MySQL 和 PostgreSQL。无 JSON 业务筛选时恢复数据库 `COUNT + LIMIT/OFFSET`，不会为了首页总数加载近七天全部共享视频渠道任务；本轮 MySQL 实际接口核验返回 HTTP 200、`total=119`。

数据库逐字段核验结果如下：

| 合同 | 完整率 |
| --- | ---: |
| 全部终态公共字段 | `119/119` |
| 公开任务 ID、模型、状态与任务事实一致 | `119/119` |
| 成功任务公共字段、`content.video_url`、`usage` | `118/118` |
| 成功任务 `duration`、`framespersecond`、`execution_expires_after` | `118/118` |
| 失败任务公共字段、`usage`、`error` | `1/1` |
| 失败任务无非空 `content.video_url` | `1/1` |
| 终态用户响应包含私有上游任务 ID | `0/119` |
| 终态用户响应包含非官方诊断/任务字段 | `0/119` |

失败样本经过真实提交、轮询、退款和零成本确认链路。任务详情已展示最终返回给用户的完整 mock 结果：

```json
{
  "id": "task_Agm9qT0KwhN1CgBbBUwHzKEoWyPd2n06",
  "model": "doubao-seedance-2-0-mini-260615",
  "status": "failed",
  "usage": {
    "completion_tokens": 0,
    "total_tokens": 0
  },
  "error": {
    "code": "content_policy_violation",
    "message": "mock content policy rejection"
  },
  "created_at": 1785858032,
  "updated_at": 1785858032,
  "seed": 0,
  "resolution": "480p",
  "ratio": "16:9",
  "duration": 4,
  "framespersecond": 24,
  "service_tier": "default",
  "execution_expires_after": 172800,
  "generate_audio": true,
  "draft": false,
  "priority": 0
}
```

## 控制台核验

已登录 `http://127.0.0.1:3000` 逐页检查：

| 页面 | 核验结果 |
| --- | --- |
| `/dashboard/overview` | 显示近 24 小时 119 个请求及真实消费 |
| `/usage-logs/common?type=["2"]` | 显示测试用户、分组、真实渠道类型、客户端模型、1.25 倍分组倍率和用户费用 |
| `/usage-logs/task` | 显示 118 条成功、1 条失败；失败详情包含最终用户状态和 `content_policy_violation` |
| `/cost-accounting` | 汇总显示 `$82.172270000` 收入、`$72.687068494` 成本和 `$9.485201506` 毛利润；正利润/毛利率为绿色，负值为红色 |
| `/channels` | 显示 11 个真实供应商协议类型，Z5API 为类型 `211`，未统一创建为 `NewAPIVideo` |

成本核算表中的“尝试次数”是该“渠道 + 计费上游模型”聚合行包含的上游成本尝试总数，不是人工重试次数。本轮每个请求都只有 1 次尝试，聚合行大于 1 表示该组合有多个请求；汇总“重试尝试次数”为 0。

## 根因修复回归

本轮补充的关键回归保护包括：

- 已禁用渠道能力不参与权威成本覆盖检查。
- 模板发布会退休受影响渠道中已从新快照删除的活动成本规则。
- 明确禁用价格只移除没有任何启用价格变体的模型映射。
- 源表重命名结构和新增渠道 11 可稳定生成相同导入语义。
- 生成配置只保留实际有模型映射和路由目标的渠道线路。
- 本地计算用户 Token 售价时，按终态计量的供应商成本仍保留可信上游计量。
- 发布成功后导入向导退出审查步骤，避免继续显示已发布批次的旧审查状态。
- E2E 失败任务必须保存并展示最终用户失败结果，成功与失败任务均必须完成成本和利润断言。
- Seedance 查询接口和后台轮询审计必须经过同一个终态规范化器；创建路径不得改变任务日志的 Ark 响应合同。
- OpenAI 查询入口不得用简化响应覆盖 Seedance 的完整 Ark 审计。
- Seedance 识别必须结合请求路径、冻结计费档案或模型证据；共享渠道上的普通视频任务不得被转换为 Ark。
- DoubaoVideo 和 VolcEngine 的 Seedance 终态必须具备 Ark 转换能力。
- 非法时间、损坏的展示用量快照和失败消息脱敏必须保留完整任务详情，同时不放宽计费结算校验。
- Doubao Ark 转换必须使用字段白名单，公开模型必须来自客户端任务事实或冻结计费快照，成功响应不得残留上游 `error`。
- 成功终态缺少非空视频地址时必须拒绝规范化；严格 E2E 必须接受 `failed`、`expired`、`cancelled` 三种 Ark 失败终态。
- Ark 列表无筛选首页必须在数据库层完成证据过滤、计数和分页，不得加载全部共享渠道任务。
- E2E 对全部 119 条终态响应执行字段、数值边界、任务 ID、模型身份、成功/失败互斥结构和未知字段硬断言，任一渠道违反合同立即失败。

## 验证命令

```text
docker exec -w /data new-api-local-new-api-1 /data/ark-video-material-seed
go test ./model ./service ./cmd/ark-video-material-seed -count=1
cd web && bun test --parallel=1
cd web && bun run typecheck
cd web && bun run lint
cd web && bun run build
git diff --check
```

本次提交前的最新验证结果：

- Docker 镜像重建成功，`new-api`、`video-metadata`、MySQL、Redis 均为 healthy。
- `GET /api/status` 返回 HTTP 200，`success=true`。
- `go test ./model ./relay ./service ./cmd/ark-video-material-seed -count=1 -p=1`：通过。
- `cd web && bun test --parallel=1`：480 pass、0 fail。
- `cd web && bun run typecheck`：通过。
- 受影响前端文件定向 `oxlint`：通过、零输出。
- `git diff --check`：通过。
- `cd web && bun run build`：通过。
- 全仓 `bun run lint` 仍受仓库既有未修改文件的 lint 错误阻断，本轮修改文件的定向 lint 已通过。
