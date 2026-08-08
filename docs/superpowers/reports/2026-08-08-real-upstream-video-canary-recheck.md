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

### 分组路由更新后的第二轮复验

路由策略更新后，使用完全相同的提示词、两张图片和生成参数再次执行真实上游复验。本轮明确跳过已复验的 `route-target/MAP-8YES-R60-480`，两条请求均未命中 8yes。

| 分组 | 路由与实际上游 | 任务与终态 | 审核/失败结论 |
| --- | --- | --- | --- |
| 不卡真人 | 策略 `#8`、目标 `#120202` `route-target/MAP-PAIPU-R30-480`、Paipu `#22`、`lec-seedance-videos-mini` | 本地任务 `task_YAOuHpFaZgm6cYTNqDsIDhhAAhJaD8KC`；上游任务 `task_Xm1C6cMIzLEVUXnddEXPuCoKyo0OroXe`；约 7 分 14 秒后失败 | 上游已接受任务，终态错误为 `账号积分不足`，没有返回内容审核拦截；受余额问题影响，仍不能据此宣称该目标已通过真人素材生成验收 |
| 卡真人 | 策略 `#7`、目标 `#120217` `route-target/MAP-CLMM-R6-480`、CLMM `#21`、`mg-seedance2.0 -480p mini` | 本地任务 `task_AdtNdKgfLBe1COy33lqFVM9eivHxuYP4`；上游任务 `task_5h6VD4Wojpl6NlqPqJnaPfzYnkjzdnbb`；约 1 分 16 秒后失败 | 明确返回 `内容审核未通过，请调整提示词或参考素材后重试`，确认 CLMM 对本组素材执行了审核拦截 |

两条请求分别生成成本请求 `4771`、`4772`。两条尝试均记录 `upstream_accepted=1`、`task_failed_no_charge`，最终用户扣费、供应商成本和毛利润均为 `$0`，收入状态为 `confirmed_zero`、利润状态为 `complete`；创建阶段各预扣 `63291` quota，终态后均已退款并归零。

另发现页面“生成音频”开关未开启，提交预览也未包含 `generate_audio`，但两条终态响应均显示 `generate_audio=true`。本轮失败任务没有因此产生费用，但仍需单独确认未勾选时应明确发送 `false`，还是沿用上游默认值。

第二轮证明分组路由差异化已经生效：`卡真人` 与 `不卡真人` 分别到达 CLMM 和 Paipu，不再共用 8yes 目标。当前可确认 CLMM 的审核拦截符合“卡真人”预期；Paipu 因积分不足未完成生成，补充余额后仍需使用同一组素材复验，才能完成“不卡真人”目标的最终验收。

### 五轮未覆盖目标真实复验

继续使用同一提示词和两张审核素材执行 5 轮、共 10 次固定渠道真实请求。所有请求均为 `doubao-seedance-2-0-mini-260615`、4 秒、16:9，并按目标合同使用 480p 或 720p。为命中 4SToken 的 933 目标，第 3、4 轮“不卡真人”请求将同一组两张素材重复组成 5 个图片引用；其余请求均为 2 个图片引用。

本轮排除此前已经复验的 `route-target/MAP-8YES-R60-480`、`route-target/MAP-CLMM-R6-480` 和 `route-target/MAP-PAIPU-R30-480`。10 个成本尝试均记录 `upstream_accepted=1`，没有自动重试或切换目标。

| 轮次 | 分组 | 实际目标与渠道 | 本地/上游任务 | 终态 |
| --- | --- | --- | --- | --- |
| 1 | 卡真人 | `#120214` `MAP-CANGYUANSUANLI-R96-480`，Cangyuan `#23`，`seedance-2.0-mini` | `task_Jd1Dn9Dj5SMEL82CeCeNqeJqF34MXsGk` / `task_Kpze5zpBSGscIVCJ6tA4p8HFDRTTbpO9` | 50 秒后失败；素材已上传并通过基础检查，但上游未提供具体原因，不能明确归因为内容审核 |
| 1 | 不卡真人 | `#120198` `MAP-4STOKEN-R164-480`，4SToken `#48`，`4sdance_mini431` | `task_ZRh36iVUQPaJPkeKm0VCMFoA5O1HSAwR` / `vid-bca9f1c003e149b7ab429d4fb419e98e` | 255 秒后失败：`素材过于典型，卡验证了，请重试看看` |
| 2 | 卡真人 | `#120213` `MAP-CANGYUANSUANLI-R101-720`，Cangyuan `#23`，`seedance-2.0-mini-720p` | 成本请求任务标记 `task_AsRIVJ8yRZCqOIDWNHxTyY1Uznp5cgg8` | 创建阶段上游 HTTP 400：`model seedance-2.0-mini-720p not found` |
| 2 | 不卡真人 | `#120199` `MAP-4STOKEN-R165-720`，4SToken `#48`，`4sdance_mini431` | `task_4vegUZetxrJ1wb9qdjX8k3QUzuYr8NWm` / `vid-52b6a78b79a443c9a3a22be92a796b90` | 223 秒后失败：`素材过于典型，卡验证了，请重试看看` |
| 3 | 卡真人 | `#119882` `MAP-CLMM-R8-720`，CLMM `#21`，`mg-seedance2.0 -720p mini` | `task_yNQFiJz7k6dWmj8i7sgCJbGdgghzFP0t` / `task_qtj9vbwoZffznFLMlpH6eWOwrKjzds2Y` | 68 秒后明确返回内容审核未通过 |
| 3 | 不卡真人 | `#120200` `MAP-4STOKEN-R166-480`，4SToken `#48`，`4sdance_mini933` | 成本请求任务标记 `task_CgNIvEswUWC9FPOIL4XtUjhBQ0N2kHlA` | 创建阶段上游 HTTP 400：`模型不存在或不可用` |
| 4 | 卡真人 | `#120209` `MAP-8YES-R70-480`，8yes `#46`，`videos-4-mini-480p` | `task_x12bOrQ8XYSLGetlYFcI7dc0mx7M9Ww7` / `task_NKKd5pXzLEDAlvZSMo6XynAD1rngl1EW` | 58 秒后明确返回内容审核未通过 |
| 4 | 不卡真人 | `#120201` `MAP-4STOKEN-R167-720`，4SToken `#48`，`4sdance_mini933` | 成本请求任务标记 `task_h8xNCeCdmJG0li7br4nCoWCWLtU1zsje` | 创建阶段上游 HTTP 400：`模型不存在或不可用` |
| 5 | 卡真人 | `#120210` `MAP-8YES-R71-720`，8yes `#46`，`videos-4-mini-720p` | `task_YetmZxTlkKFoFU0fs09WCaXYr1zx1Dc0` / `task_7nFeTeji7EJzVwXiyEuCIyZQhXTTduxg` | 50 秒后明确返回内容审核未通过 |
| 5 | 不卡真人 | `#120203` `MAP-PAIPU-R31-720`，Paipu `#22`，`lec-seedance-videos-mini` | `task_upwXb14yqVf9VUw6i6sVlEU4fQKTZV8V` / `task_1R1zXEJzo0ySzIQdQt6at8jyL5UxUWIi` | 73 秒后失败：`账号积分不足` |

成本请求 `4773`、`4774`、`4776`、`4777`、`4779`、`4781`、`4782` 均在终态确认为 `task_failed_no_charge`，用户收入、供应商成本和毛利润均为 `$0`，状态为 `confirmed_zero / complete`。创建阶段被拒绝的 `4775`、`4778`、`4780` 最终用户 quota 已归零，但供应商成本仍为未知，尝试状态为 `upstream_response_rejected / cost_unknown`，请求仍停留在 `pending / incomplete_revenue`。

五轮没有任何成功视频。“卡真人”侧 5 个目标中，CLMM 720p 和 8yes 480p/720p 共 3 个目标明确执行了内容审核拦截；Cangyuan 480p 只返回通用生成失败，720p 目标则配置了上游不存在的模型。“不卡真人”侧两个 4SToken 431 目标仍对本组素材触发供应商验证拦截，两个 933 目标不可用，Paipu 仍受积分不足阻断，因此该分组仍没有真人素材成功证据。

真实复验后已执行运行时止损：卡真人策略 `#7` 禁用目标 `#120213`；不卡真人策略 `#8` 禁用目标 `#120200`、`#120201`；默认 Mini 策略 `#2` 同步禁用批次 24 目标 `#120180`、`#120174`、`#120175`。前两组通过管理后台保存并立即刷新路由缓存；默认策略目标在数据库中禁用，并由周期缓存同步加载。缓存门禁复核分别使用默认 Key 和“不卡真人”Key 固定 4SToken、提交 5 图片请求，两次均在本地返回 `no_compatible_route`，成本请求最大 ID 保持 `4782`，确认没有再次请求已禁用的 933 目标。默认策略的三个目标仍由配置导入批次 24 管理，后续重新导入可能再次启用，因此必须在 `sd收录.xlsx` 或模板生成规则中同步修正上游模型记录。

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
8. Cangyuan 目标 `MAP-CANGYUANSUANLI-R101-720` 当前绑定的 `seedance-2.0-mini-720p` 被上游判定不存在；该目标不应继续作为可用候选。
9. 4SToken 的 `MAP-4STOKEN-R166-480`、`MAP-4STOKEN-R167-720` 均绑定 `4sdance_mini933`，真实创建请求被上游判定模型不可用；两个目标的 `supports_real_person=true` 也尚无真实成功证据。
10. 4SToken 的 `4sdance_mini431` 虽有普通素材成功历史，但本轮 480p、720p 真人审核素材均返回供应商验证拦截；不能把普通 Canary 成功直接等同为真人素材通过。

## 后续动作

1. Paipu Stable 933 已通过真实 Canary；Standard 1080p 和本轮 Mini 真人素材请求均因供应商积分不足失败，需先补充供应商积分后分别复测。
2. Secure 折扣向供应商确认 `video-2.0-pro` 的 HTTP 500 传输错误；在获得明确错误或修复后再验收。
3. OmegaAI 补充供应商积分，并确认 `lingjing-video-v1` 的可用性后再复测；不在 Canary 中临时切换未收录模型。
4. 修复提交阶段失败后的收入确认，使全额退款请求落为 `confirmed_zero`，成本未知时利润状态体现为 `incomplete_cost`。
5. 审核并明确同渠道多个兼容目标的优先级生成规则，决定采用“收录表顺序优先”还是“预测成本优先”。
6. 补齐 5 条本批次关联的历史成本覆盖缺口后，再进行严格模式全量真实上游联测。
7. Cangyuan 720p 的 `seedance-2.0-mini-720p` 目标及 4SToken 的两个 `4sdance_mini933` 目标已在当前运行时禁用；下一步更新源收录表或生成规则，防止后续配置导入重新启用。修复真实模型 ID 后再单独复验。
8. 复核 4SToken `4sdance_mini431` 的真人素材政策；若供应商确认该模型会拦截此类素材，应将两个目标从“不卡真人”策略移除或把 `supports_real_person` 改为 `false`。
