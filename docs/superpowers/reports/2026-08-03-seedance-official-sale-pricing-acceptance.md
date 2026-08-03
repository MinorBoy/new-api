# Seedance 官方售价与供应商成本解耦 E2E 验收报告

## 验收结论

本轮已完成 Seedance 用户售价根治、渠道模板重生成、正式导入和完整 mock 上游 E2E。用户售价与供应商成本完全脱钩，唯一用户售价来源是渠道模板“官方售价”工作表导入的显式场景价格：

```text
用户售价 = 官方场景 USD/秒 × 输出计费秒数 × 分组倍率
```

`no_video` 与 `with_video` 是两个独立的官方价格场景。参考视频时长不参与用户售价，只用于素材能力校验、供应商成本计量和审计快照。供应商成本继续从渠道成本规则独立计算，因此出现负毛利时会真实记录，不会用供应商价格反推用户售价。

## 输入与生成物

| 数据 | 结果 |
| --- | --- |
| 源表 | `docs/new-channels/sd收录.xlsx` |
| 源表 SHA256 | `361565061dc8b7683adb730451a1a2e9597b52cdf70031991443a0097a0a4eea` |
| 渠道模板 | `outputs/2026-08-03-import/渠道模型成本与利润模板-v1.xlsx` |
| 渠道模板 SHA256 | `90175a6c9b6d4669f16a5ca6b2c313c58e1369e9795e899e1087f91f5bc12799` |
| 模板报告 SHA256 | `8b762205be5a674d7326fabd7e7557d6d9af9a58069b140afaad1d977e55a84e` |
| 导入配置 | `e2e/testdata/channel-config-v1.json` |
| 导入配置 SHA256 | `7d63f8150f3ef51a1e134b4581b0412816daa07f8ed8858fdb608392d5af564b` |

模板生成的实体数量：

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

16 条官方售价场景来自火山引擎官方售价工作表，按 Seedance 2.0、Fast、Mini 的可用分辨率和 `no_video`/`with_video` 场景保存绝对 USD/秒，不再由 720p 基准价、像素比例或参考视频时长推导倍率。唯一告警是 Dimensio `pxv-seedance-2.0-standard / 480p` 原币按秒价格为 0，该草稿保持禁用，不形成活动成本规则。

## 正式导入

正式导入批次为 `14`，状态为 `published`，`payload_sha256` 为 `b20e89fb587d47f4431fb886b303d9ce9598f06e9e48ac8070e14148fa19a6a1`，批次开放问题数为 `0`。新工作簿仅生成时间和文件哈希变化，实体语义载荷未变化，因此管理 API 按语义幂等规则复用批次 `14`，没有创建重复批次。价格评审阶段产生的 16 条 `PRICING_GROUP_SCOPE_UNREPRESENTABLE` 警告已全部解决，不影响发布结果。

批次数据库记录核验：

- `cost_rule_drafts`: 147，其中 146 条启用并物化，1 条零价格草稿排除。
- `model_mappings`: 147。
- `route_blueprints`: 147。
- `sale_proposals`: 16。
- 所有 Seedance 旧 `ModelPrice`、`ModelRatio`、`CompletionRatio` 销售规则已删除；`billing_setting.billing_expr` 中不存在 Seedance 规则。
- 代码内置的 3 条 Dimensio Seedance 平铺时长售价已删除，不再作为默认用户售价或配置同步数据。
- `billing_setting.billing_mode` 与 `billing_setting.duration_price` 仅保留官方售价覆盖的 4 个运行时模型键；6 个销售配置表均不存在 `jimeng-video-seedance-*` 供应商别名残留。
- 活动 Seedance 销售使用 `billing_setting.duration_price` 的显式场景矩阵，任务快照定价版本均为 `official-sheet-v1`。
- 通用 Option API 只能原样保留已导入的 Seedance `per_duration` 模式和场景价格；新增、修改或删除必须通过渠道模板导入。
- Stage 并发基线覆盖本批次、历史批次及六类当前销售 Option 中的全部 Seedance 键，发布清理不会绕过 `STALE_BASE_VERSION` 覆盖并发修改。

## E2E 覆盖

本轮使用真实导入的渠道类型、渠道线路、客户端模型、上游模型、素材限制、成本模式、供应商原价、币种和汇率。仅供应商 HTTP 提交与轮询响应使用本地 mock，任务提交、轮询、能力路由、用户扣费、任务日志、使用日志、成本核算和利润识别均走生产代码。

矩阵结果：

| 项目 | 数量 |
| --- | ---: |
| 导入路由目标 | 147 |
| 接受并完成任务 | 110 |
| 提交前渠道合同阻断 | 36 |
| 零价格禁用草稿 | 1 |
| 成功任务 | 110 |
| 失败任务 | 1 |
| 终态公开结果 | 111 |
| 单次尝试 | 111 |
| mock 上游调用 | 222 |

合同阻断分布为：Cangyuan 12、CLMM 18、8yes 1、MegaByAI 4、Secure 企业组 1。素材限制覆盖 `431` 38 条、`900` 6 条、`903` 4 条、`933` 99 条。阻断发生在渠道合同校验阶段，未产生上游调用或计费流水。

成功任务的售价场景覆盖：

| 场景 | 分辨率 | 数量 |
| --- | --- | ---: |
| `no_video` | 720p | 6 |
| `with_video` | 480p | 35 |
| `with_video` | 720p | 55 |
| `with_video` | 1080p | 10 |
| `with_video` | 4k | 4 |

## 售价根治核验

在本地 MySQL 对 110 条任务快照执行核验：

- 110/110 条任务使用 `official-sheet-v1`。
- 110/110 条任务的分组倍率为 `1.25`。
- 110/110 条任务满足 `output_charge = 官方 USD/秒 × 输出计费秒数`。
- 110/110 条任务满足 `final_charge = output_charge × 1.25`。
- 110/110 条任务配额满足 `round(final_charge × 500000)`，并与使用日志、成本核算请求中的最终配额一致。
- 104 条任务包含参考视频时长事实，6 条为文生视频；参考视频时长没有改变用户售价公式。
- 5 组相同模型、分辨率和场景覆盖了多个参考视频时长，每组最终售价均只有 1 个值。
- 旧 `seedance_price_matrix` 在任务快照、使用日志和运行时售价配置中的数量均为 0。

典型快照：`480p + with_video + 输出 4 秒` 使用官方 `0.0193150684931507 USD/秒`，输出基础售价为 `0.0772602739726028 USD`，乘 `1.25` 分组倍率后的最终用户售价为 `0.0965753424657535 USD`。参考视频时长 `3000 ms` 只保存在审计事实中，不参与售价计算。

## 日志与成本核算

测试用户 `ark_sdk_matrix_user`、分组 `ark-sdk-material-matrix-local` 在本地 Docker MySQL 中的实际数据为：

| 数据 | 数量/金额 |
| --- | ---: |
| 成功任务 | 110 |
| 失败任务 | 1 |
| 视频使用日志 `type=2` | 111 |
| 任务日志 | 111 |
| 已保存终态公开结果 | 111 |
| 成本核算请求 | 111 |
| 成本核算尝试 | 111 |
| 收入状态 `settled` | 110 |
| 利润状态 `complete` | 111 |
| 单次尝试 `attempt_count=1` | 111 |
| 负毛利请求 | 60 |
| 计费收入 | `$47.145912` |
| 供应商成本 | `$66.713589042` |
| 计费毛利润 | `-$19.567677042` |
| 整体毛利率 | `-41.5045%` |

负毛利是官方售价与当前供应商成本组合的真实结果，成本核算没有把供应商成本回写为用户售价，也没有使用 `$0.2` 占位价格。

额外失败样本通过真实提交、轮询、退款和成本结算链路生成。最终任务状态为 `FAILURE`，公开错误码为 `content_policy_violation`，公开错误信息为 `mock content policy rejection`。成功与失败共 111 条任务均已保存最终返回给用户的协议响应；扫描公开结果后，渠道密钥、上游任务 ID、`channel_id` 和 `api_key` 泄漏数量为 0。

## 控制台核验

已登录 `http://127.0.0.1:3000` 逐页检查：

| 页面 | 核验信号 |
| --- | --- |
| `/dashboard/overview` | 显示近 24 小时请求数 `111` 和消耗金额 |
| `/usage-logs/common` | 显示测试用户、测试分组、真实渠道和 Seedance 模型消费记录 |
| `/usage-logs/task` | 显示 110 条成功任务和 1 条失败任务；任务详情包含最终公开成功/失败结果 |
| `/cost-accounting` | 显示官方售价收入、供应商成本、毛利润和毛利率明细；正值为绿色，负值为红色 |
| `/channels` | 显示 4stoken、8yes、Cangyuan、CLMM、Dimensio 等真实渠道类型；不再统一显示 `NewAPIVideo` |

## 验证命令

以下命令已执行：

```text
docker exec -w /data new-api-local-new-api-1 /data/ark-video-material-seed
go test ./... -count=1 -p=1
cd web && bun test --parallel=1
cd web && bun run typecheck
cd web && bun run build
```

结果：持久化 Seed E2E 输出 `ARK SDK video material matrix seed completed`，生成 110 条成功任务、1 条失败任务、111 条终态公开结果、111 条视频使用日志、111 条成本请求和 111 次尝试；Go 全量测试通过；前端串行全量测试 `456 pass / 0 fail`；TypeScript 类型检查通过；生产构建通过。前端 `bun run lint` 未作为通过项报告，因为仓库当前存在与本轮无关的既有全量 lint 错误；本轮改动以定向测试、类型检查和生产构建为准。
