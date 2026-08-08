# 真实上游视频生成 Canary 验收报告

## 验收结论

本轮已通过 Ark SDK 兼容入口 `/api/v3/contents/generations/tasks` 对已配置的真实视频渠道执行固定渠道 Canary。每个有效创建请求只提交一次，渠道之间串行执行；失败后未自动重试，也未切换模型追加消费。MegaByAI 按要求跳过。

当前 12 个非 MegaByAI 渠道实例中，8 个已有至少一条真实成功链路证据，4 个本轮未通过。系统已经具备继续做单渠道真实验收的基础条件，但尚不具备“无条件启用全部渠道”的条件。Paipu、Secure 折扣、OmegaAI、4SToken 需要先处理供应商账户或模型可用性；Lucen、Cangyuan 和 Dimensio 仍有路由或成本风险。

本轮真实请求发现并修复三项响应兼容问题：Secure 企业返回带单位的 `duration: "5s"`、Secure 海外返回数值型 `seconds: 4`、Z5API 的 `seconds` 实际表示任务耗时而不是视频时长。修复后原任务继续轮询和结算，没有重复创建任务。

## 环境与约束

| 项目 | 值 |
| --- | --- |
| 分支 | `ysr` |
| 服务 | `http://127.0.0.1:3000` |
| 数据库 | MySQL 8.2 |
| 缓存 | Redis 7 Alpine |
| 视频元数据服务 | healthy |
| 配置导入批次 | `21`，已发布并激活 |
| 成本核算 | `tracking / 0%` |
| 真实请求方式 | 管理员 Token 固定具体渠道，凭据仅从数据库临时读取 |
| 重试规则 | 创建请求不自动重试；失败后不换模型追加消费 |
| 跳过项 | MegaByAI |

最终 `new-api`、MySQL、Redis、`video-metadata` 四个容器均为 healthy。报告、源码和 Git diff 中未写入渠道 API Key 或管理员 Token。

## 渠道结果

| 渠道 | 实例 | 实际上游模型 | 真实结果 | 验收结论 |
| --- | ---: | --- | --- | --- |
| Dimensio | `#5` | `jimeng-video-seedance-2.0-fast-vip` | 历史真实文本、图片、多模态任务成功 | 链路通过；多模态供应商扣费高于现有成本配置，暂不作为生产定价通过 |
| CLMM | `#21` | `mg-seedance2.0 -480p mini` | `task_O0xbKhPL7K0QK8UzHNAgduMOBRaxNvBC` 成功 | 通过；请求必须省略不支持的 `generate_audio` 字段 |
| Lucen | `#27` | `seedance-480p-token` | `task_N6bas5j31eMwoS8zYC1hynhz8IhCD7Iy` 成功 | 部分通过；固定 5 秒目标与当前 Key 分组不兼容，Token 路由轻微亏损 |
| Cangyuan | `#23` | `seedance-2.0-mini` | `task_P1G9G4z86JajJJcZvIBvJI6XOE2oIl5G` 成功 | 链路通过；4 秒请求在按次成本下严重亏损，暂不应生产放量 |
| Paipu | `#22` | `lec-seedance-videos-mini` | `task_4glBouzsCjrXnlkAERRPbnpUPxTRPCcb` 上游终态失败 | 未通过；供应商账户积分不足，本地退款和零成本确认完成 |
| Secure 企业 | `#24` | `video-2.0-pro` | `task_mO8aLcqUihgUsZnVhmV4Rk8fzLmRNe2C` 成功 | 通过 |
| Secure 海外 | `#25` | `video-2.0-mini` | `task_yvHRNjMuDxvVwCs5XEggxpt79HSSPdXt` 成功 | 通过；毛利率仅约 `0.48%` |
| Secure 折扣 | `#26` | `video-2.0-mini` | 提交返回 `400 当前模型正在维护` | 未通过；本地预扣已退款，不重试 |
| OmegaAI | `#49` | `klsdpro2-720p` | 提交返回 `400 模型不可用` | 未通过；本地预扣已退款，不切换目标追加消费 |
| 4SToken | `#48` | `4sdance_mini431` | 提交返回 `402 钱包余额不足` | 未通过；本地预扣已退款 |
| 8yes | `#46` | `videos-4-mini-480p` | 多条近期真实任务成功 | 通过 |
| Z5API | `#47` | `sd-2-c2` | `task_xidc6oXrhdFk11ItaxUgKlx8oB9anU8o` 成功 | 通过；修复后 Ark `duration` 正确返回请求值 `4` |

Lucen 的固定 5 秒请求到达真实上游后返回 `model_not_found`，对应成本请求 `4753` 进入保守的 `cost_unknown`。随后使用当前 Key 分组支持的 Token 模型完成了一次真实成功链路；该补充请求未由系统自动重试触发。

## 成本与利润

以下金额均为成本核算中的美元等值；失败且已退款的请求不计入完整利润汇总。

| 渠道 | 收入 | 供应商成本 | 毛利润 | 毛利率 | 状态 |
| --- | ---: | ---: | ---: | ---: | --- |
| CLMM `#21` | `$0.126582000` | `$0.070136986` | `$0.056445014` | `44.5917%` | `settled / complete` |
| Lucen `#27` | `$0.253164000` | `$0.255797808` | `-$0.002633808` | `-1.0404%` | `settled / complete` |
| Cangyuan `#23` | `$0.126582000` | `$0.328767123` | `-$0.202185123` | `-159.7266%` | `settled / complete` |
| Secure 企业 `#24` | `$0.680548000` | `$0.458561644` | `$0.221986356` | `32.6188%` | `settled / complete` |
| Secure 海外 `#25` | `$0.272220000` | `$0.270904110` | `$0.001315890` | `0.4834%` | `settled / complete` |
| 8yes `#46` | `$0.126582000` | `$0.077479452` | `$0.049102548` | `38.7911%` | `settled / complete` |
| Z5API `#47` | `$0.544438000` | `$0.438356164` | `$0.106081836` | `19.4846%` | `settled / complete` |
| Paipu `#22` | `$0` | `$0` | `$0` | - | `confirmed_zero / complete` |

Dimensio 的既有真实报告记录用户费用约 `¥1.920002`、供应商实际扣除 432 积分；在供应商确认多模态计价规则前，不纳入本表的美元成本比较。

## 验收发现与修复

### Secure 响应数字兼容

Secure 企业查询响应包含 `duration: "5s"`，旧解析器只接受 JSON number，导致后台轮询和公开查询持续返回解析错误。Secure 海外查询响应又使用数值型 `seconds: 4`，旧 DTO 只接受字符串。

当前实现使用结构化 JSON 解析同时兼容数字、数字字符串和带 `s` 单位的 duration，并在进入计费前继续执行时长上限校验。两条原有真实任务均在修复后恢复为成功和完整结算，没有重复提交。

### Z5API `seconds` 语义

Z5API 的 4 秒请求在终态返回 `seconds: "134"`，同时 Token 用量为 4 秒 720p 对应的 `86400`。真实证据表明该字段是任务处理耗时，不能作为视频时长。旧实现错误公开为 `duration=134`。

当前实现对 Z5API 忽略上游 `seconds` 的计费和视频时长含义，Ark 响应从原始用户请求或计费快照恢复 duration。复用同一成功任务复查，公开响应已从 `134` 修正为 `4`；Z5API 为按次计费，本次供应商成本金额未受该错误影响。

## 待处理问题

1. Paipu 需要补充供应商积分后重新执行一次 Mini Canary。
2. Secure 折扣需要等待模型维护结束，或由供应商确认可用模型后重新验收。
3. OmegaAI 需要修复或退休当前优先命中的 `klsdpro2-720p` 目标，再对明确可用的标准模型验收。
4. 4SToken 需要充值上游钱包后重新验收 `4sdance_mini431`。
5. Lucen 当前 Key 所属分组不支持固定 5/10/15 秒模型；应退休这些目标或更换匹配分组的 Key。Token 路由还需调价以消除负毛利。
6. Cangyuan 按次成本对短视频严重亏损，应提高销售倍率、限制最短时长，或在严格模式中阻断。
7. Dimensio 多模态成本规则需要按真实 432 积分口径确认并修正。
8. Lucen 固定模型、Secure 折扣、OmegaAI、4SToken 的供应商 HTTP 失败均按设计保守进入 `cost_unknown`，不能仅凭 HTTP 状态自动认定供应商零成本；需要供应商账单或明确不收费合同后人工对账。
9. 上述已完成本地退款的提交失败请求，其成本请求收入状态仍为 `pending`、利润状态为 `incomplete_revenue`。这与“全额退款应为 `confirmed_zero`”的成本合同不一致；应补充任务提交失败退款后的收入确认，并让利润状态体现为 `incomplete_cost`。

## 验证记录

```text
go test ./relay/channel/task/newapivideo -count=1
PASS

GET Secure 企业原任务
HTTP 200, status=succeeded, duration=5, video_url 存在

GET Secure 海外原任务
HTTP 200, status=succeeded, duration=4, video_url 存在

GET Z5API 原任务（修复后）
HTTP 200, status=succeeded, duration=4, video_url 存在
```

真实创建失败均有本地预扣退款日志。成功任务均为 `attempt_no=1`，并保存任务终态、公开视频 URL、成本尝试和请求级利润；本轮没有自动创建重试任务。
