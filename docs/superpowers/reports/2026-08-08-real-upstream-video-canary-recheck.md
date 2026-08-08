# 真实上游视频生成 Canary 复验报告

## 验收结论

最新版 `sd收录.xlsx` 已重新生成渠道模板并通过批次 `24` 发布、激活。本轮仅复验此前没有真实成功证据的四个渠道，创建请求均固定具体渠道、串行执行。首轮失败后没有自动重试或自动切换模型；随后按用户指令分别对 Paipu、Secure 折扣和 OmegaAI 进行替代模型人工复测。

本轮新增两条真实成功链路：4SToken `#48`、Paipu `#22`（换用 `lec-seedance-2-0-933-stable`）。Secure 折扣 `#26`、OmegaAI `#49` 仍未通过真实上游 Canary。因此，当前 12 个已配置且不含 MegaByAI 的渠道实例中，已有 10 个渠道实例具备真实成功证据，2 个仍待供应商侧处理。

## MegaByAI 合同修订复核（2026-08-08）

用户已确认最新版 `sd收录.xlsx` 的 MegaByAI 分辨率记录有效，旧的“仅支持 480p、720p”文档不再作为依据。本次复核将 MegaByAI 的统一合同更新为 `480p`、`720p`、`1080p`、`4k`；`1440p` 等未收录值仍会被拒绝。

本次发现的根因是运行时路由合同 `relay/video_route_contract.go` 仍保留旧分辨率门禁，和上游请求校验、渠道模板生成器不一致。已补充运行时合同及回归测试，确保高分辨率通过、非合同分辨率继续阻断。

重新生成的 v2 模板和转换结果如下：

| 项目 | 结果 |
| --- | --- |
| 源文件 | `docs/new-channels/sd收录.xlsx`，SHA256 `fffa05e92693fe63aa9659db1cb2fcc9e69478b144f47fae1d872b67d7aeedab` |
| v2 模板 | `outputs/2026-08-08-sd-refresh-v2/渠道模型成本与利润模板-v1.xlsx` |
| v2 模板 SHA256 | `1c2050c9613b434d70b752d83486f4836cefc06c202f20c7841215e258311a17` |
| 完整规则 | 394 条成本规则，其中 390 条 `active`、4 条 `draft` |
| MegaByAI | 62 条成本规则全部 `active`，31 条映射全部启用，包含 1080p/4K |
| 剩余 draft | Cangyuan 收录表第 100 行的素材上限冲突，已按规则隔离，不阻断模板生成 |
| 转换导入范围 | 排除 MegaByAI、APIAW 后为 10 个渠道、12 条线路、156 条成本规则、156 条映射，阻断问题 0 |
| Excel 校验 | 公式错误 0；10 个工作表均完成结构检查，大小表采用分段视觉渲染并通过 |

MegaByAI 仍未绑定真实 API Key，因此 v2 模板没有直接导入或激活该渠道；批次 `24` 的线上路由和历史 Canary 结论不因本次合同修订自动改变。待 Key 配置并单独完成低成本真实 Canary 后，才可将 MegaByAI 纳入批次级激活。

## 配置批次

| 项目 | 结果 |
| --- | --- |
| 源文件 | `docs/new-channels/sd收录.xlsx` |
| 源文件 SHA256 | `fffa05e92693fe63aa9659db1cb2fcc9e69478b144f47fae1d872b67d7aeedab` |
| 模板文件 | `outputs/2026-08-08-sd-refresh/渠道模型成本与利润模板-v1.xlsx` |
| 模板 SHA256 | `b058fe729383b255b34c240a5621a6026025482d3e170dc1c852a683e1861538` |
| 导入载荷 | `outputs/2026-08-08-sd-refresh/channel-config-v1.json` |
| 载荷 SHA256 | `15e5e5ebdaa978ce4159c6ed87ca938eba404024b7120efaee00e071f0805ce1` |
| 最终批次 | `24`，`published` 且已激活 |
| 批次规模 | 10 个渠道、12 条线路、156 条成本规则、156 条映射、156 条路由 |
| 激活结果 | 新建 156 个目标，退休 148 个旧目标，阻断 0 |
| 本轮排除 | MegaByAI、APIAW 未进入批次 |

批次 `22` 和 `23` 保留为导入及门禁审计历史，不删除。

## Canary 范围

已跳过具备真实成功证据的渠道：Dimensio、CLMM、Lucen、Cangyuan、Secure 企业、Secure 海外、8yes、Z5API。

本轮请求均使用 Ark SDK 兼容入口 `POST /api/v3/contents/generations/tasks` 并固定具体渠道。正式请求均省略渠道不支持的 `generate_audio` 字段；人工复测没有自动重试或切换其他渠道。为精确验证指定替代上游模型，Secure `#26` 的目标 `#120119` 与 OmegaAI `#49` 的目标 `#120097` 均在提交前临时提升优先级，提交后立即恢复原值 `2`，并等待缓存同步。

| 渠道 | 请求模型与素材 | 实际目标 | 结果 |
| --- | --- | --- | --- |
| Paipu `#22` | Mini、文本、4 秒、480p、16:9 | `lec-seedance-videos-mini` | 创建成功，任务 `task_7FDVCevv6PZYCymyQ0Ut4VLAWxY2J3Nv` 进入上游后失败：账号积分不足 |
| Paipu `#22`（换模型复测） | Standard、文本、4 秒、1080p、16:9 | `lec-seedance-videos-standard` | 创建成功，任务 `task_eiPSEdRjH4p9qNHXqV2tUrtzVxKtoMan` 被上游接受，约 3 分 39 秒后失败：账号积分不足 |
| Paipu `#22`（Stable 933 复测） | Standard、文本、4 秒、480p、16:9 | `lec-seedance-2-0-933-stable` | 任务 `task_z9nKcYCCwHsMNVqR9fnX9zV9S0nroD7n` 成功，Range GET 返回 `206`、`video/mp4`，媒体总长度 `1191578` 字节 |
| Secure 折扣 `#26` | Mini、首帧图片、4 秒、720p、16:9 | `video-2.0-mini` | 本地返回 HTTP 502；上游 HTTP 400：当前模型正在维护 |
| Secure 折扣 `#26`（换模型复测） | Standard、首帧图片、4 秒、720p、16:9 | `video-2.0-pro` | 上游请求阶段返回 HTTP 500 `do request failed`，未形成可查询任务；成本尝试标记 `upstream_transport_ambiguous` |
| OmegaAI `#49` | Standard、文本、4 秒、720p、16:9 | `seedance-v2-720p` | 上游 HTTP 400：模型不可用 |
| OmegaAI `#49`（换模型复测） | Standard、文本、5 秒、720p、16:9 | `lingjing-video-v1` | 上游 HTTP 402：积分余额不足；未形成可查询任务 |
| 4SToken `#48` | Mini、文本、4 秒、480p、16:9 | `4sdance_mini431` | 任务 `task_u6h5voiBoVq30IDmMglB7Yv8MZqUpaiV` 成功，视频 URL 可访问 |

4SToken 成功任务公开响应为 `succeeded`，`duration=4`，`completion_tokens=40176`。结果 URL 的 HEAD 检查返回 HTTP 200、`Content-Type: video/mp4`、`Content-Length: 1391443`。

## 成本与利润

金额为美元等值。

| 渠道 | 成本请求 | 收入 | 供应商成本 | 毛利润 | 毛利率 | 尝试 | 核算状态 |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| Paipu `#22` | `4763` | `$0` | `$0` | `$0` | - | 1 | `confirmed_zero / complete` |
| Paipu `#22`（换模型复测） | `4767` | `$0` | `$0` | `$0` | - | 1 | `confirmed_zero / complete` |
| Paipu `#22`（Stable 933 复测） | `4768` | `$0.253164000` | `$0.301369863` | `-$0.048205863` | `-19.0414%` | 1 | `settled / complete` |
| Secure 折扣 `#26` | `4764` | 未确认 | 未确认 | 未确认 | - | 1 | `pending / incomplete_revenue`，attempt 为 `cost_unknown` |
| OmegaAI `#49` | `4765` | 未确认 | 未确认 | 未确认 | - | 1 | `pending / incomplete_revenue`，attempt 为 `cost_unknown` |
| Secure 折扣 `#26`（换模型复测） | `4770` | 未确认 | 未确认 | 未确认 | - | 1 | `pending / incomplete_revenue`，attempt 为 `upstream_transport_ambiguous / cost_unknown` |
| OmegaAI `#49`（换模型复测） | `4769` | 未确认 | 未确认 | 未确认 | - | 1 | `pending / incomplete_revenue`，attempt 为 `upstream_response_rejected / cost_unknown` |
| 4SToken `#48` | `4766` | `$0.126582000` | `$0.082191781` | `$0.044390219` | `35.0684%` | 1 | `settled / complete` |

Paipu 的两次失败请求均按供应商失败语义确认为零收入、零成本。Stable 933 复测预扣 `126582` quota，成功后按次成本规则结算；成本尝试实际记录 `lec-seedance-2-0-933-stable`、`upstream_accepted=1`、`task_succeeded / settled`。Secure 折扣和 OmegaAI 的替代模型请求均未形成成功任务或消费日志，新增成本请求分别为 `4770`（传输结果不明确）和 `4769`（供应商 402）；两者仍停留在 `pending / incomplete_revenue`，与“失败退款后收入应确认为零”的合同不一致，问题仍未闭环。

## 真人素材安全审核复验（2026-08-08）

为验证上游对真人素材的安全审核，本轮将提示词改为非性化内容，并只保留两张用户提供的参考图片；未提交原始脱衣提示词。

| 项目 | 设置/结果 |
| --- | --- |
| 提示词 | `从图1自然转身，镜头平滑运镜过渡到图2，人物表情害羞，动作轻柔动人。` |
| 参考素材 | 2 张公网图片，0 个视频，0 个音频 |
| 参数 | `doubao-seedance-2-0-mini-260615`、4 秒、480p、16:9 |
| 不卡真人 Key | 任务 `task_lFO8K48HpShmsPq4oYxiriWnsMk5IDn5`，任务日志 35 秒，终态 `failed` |
| 卡真人 Key | 任务 `task_jRHhpaeXesiW4z9jIBPaBvIMkqZrVcPS`，任务日志 28 秒，终态 `failed` |
| 审核结果 | 两条均返回 `内容审核未通过，请调整提示词或参考素材后重试`，`usage.total_tokens=0` |
| 用户扣费/成本核算 | 两条任务日志均为 `¥0`；成本核算筛选 `#46 + videos-4-mini` 无已完成记录 |

两条请求的任务日志均显示渠道 `#46`，上游创建响应模型为 `videos-4-mini`（首条上游任务 ID 为 `task_Z2VewkXVMP9sYVjbdy7VSfUuwqcdyxp9`）。路由策略页进一步确认，“不卡真人”和“卡真人”当前都绑定同一个目标 `route-target/MAP-8YES-R60-480`。因此，本轮证明了 8yes 上游对这组素材/请求触发了内容审核拦截，但尚未形成两个分组之间的渠道差异化验收；若期望“卡真人”与“不卡真人”验证不同上游，必须先为两个分组绑定不同的路由目标，再重新执行同一组请求。

## 本地验证

| 检查 | 结果 |
| --- | --- |
| `bun run typecheck` | 通过，`tsgo -b` 退出码 0 |
| 模板生成器测试 | 27 项通过，0 失败 |
| 转换器完整测试 | 68 项通过，0 失败 |
| 受影响文件 lint | 通过，0 error |
| `bun run build` | 通过，Rsbuild 生产构建退出码 0 |
| `git diff --check` | 通过，无空白错误 |
| 导入载荷结构 | 10 渠道、12 线路、156 条成本草稿、156 条映射、156 条路由，阻断问题 0 |
| 容器健康 | `new-api`、`mysql`、`redis`、`video-metadata` 均为 `healthy` |
| 凭据扫描 | 变更文件与本报告未发现凭据模式 |

模板文件当前由本机 Excel 独占打开，无法在不打断用户会话的情况下再次读取物理文件哈希；生成报告记录的模板 SHA 与导入清单 `manifest.source_sha256` 一致，源文件 SHA 和载荷 `manifest.payload_sha256` 也已复核。

## 关键发现

1. 批次 `24` 的旧路由退休生效。OmegaAI 首轮命中新的 `seedance-v2-720p`，替代模型复测命中 `lingjing-video-v1`，均未再命中已退休的 `klsdpro2-720p`。
2. 4SToken 供应商钱包已具备真实生成条件，`4sdance_mini431` 完成提交、轮询、公开视频、结算和利润核算，可从后续未通过清单中移除。
3. Paipu 的 Standard 1080p 目标仍因账号积分不足失败，但换用 `lec-seedance-2-0-933-stable` 的 480p 目标成功，说明当前账户余额足以覆盖 2.2 CNY 的 Stable 933 请求，但不足以覆盖更高价格的 Standard 请求。成功复测命中策略 `#3`、目标 `#120111`，没有切换其他渠道。
4. Secure 折扣的 `video-2.0-mini` 仍处于供应商维护状态；换用 `video-2.0-pro` 后返回 HTTP 500 `do request failed`，需要供应商确认该 Key 的 Pro 接口状态，并检查传输层错误详情。
5. OmegaAI 的 `seedance-v2-720p` 被供应商判定为不可用，`lingjing-video-v1` 则返回积分余额不足；需要补充 OmegaAI Key 余额后再复测。
6. Paipu 本轮实际选择 `lec-seedance-videos-mini`，而不是同条件下名义成本更低的 `lec-seedance-videos-stable-mini`。当前能力路由先按目标优先级选择匹配目标，成本跟踪模式不会在同一渠道内重新按成本排序；后续应单独审核模板生成的目标优先级是否符合“最低成本优先”的运营预期。
7. 批次激活后的 `COST_COVERAGE_INCOMPLETE` 警告仍存在：本批次关联 5 条历史映射缺少成本规则。该警告不阻断激活，但严格成本模式仍不能宣称全量成本闭环。

## 后续动作

1. Paipu Stable 933 已通过真实 Canary；若要开放 Standard 1080p，需先补充供应商积分后单独复测。
2. Secure 折扣向供应商确认 `video-2.0-pro` 的 HTTP 500 传输错误；在获得明确错误或修复后再验收。
3. OmegaAI 补充供应商积分，并确认 `lingjing-video-v1` 的可用性后再复测；不在 Canary 中临时切换未收录模型。
4. 修复提交阶段失败后的收入确认，使全额退款请求落为 `confirmed_zero`，成本未知时利润状态体现为 `incomplete_cost`。
5. 审核并明确同渠道多个兼容目标的优先级生成规则，决定采用“收录表顺序优先”还是“预测成本优先”。
6. 补齐 5 条本批次关联的历史成本覆盖缺口后，再进行严格模式全量真实上游联测。
