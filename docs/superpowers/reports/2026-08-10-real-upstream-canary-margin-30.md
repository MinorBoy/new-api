# 严格模式 30% 毛利率真实上游视频 Canary 验收报告

日期：2026-08-10（Asia/Shanghai）

## 结论

本轮已将全局成本核算设置为 `strict`、最低预计毛利率 `3000 BPS`（30%），并通过官方火山 Ark Python SDK 兼容入口完成一次串行真实上游 Canary。测试使用 `/video-generation` 页面默认素材与提示词：2 张图片、1 个视频、1 个音频，默认模型 `doubao-seedance-2-0-260128`、5 秒、16:9；各渠道仅按目标约束调整分辨率。

9 个当前启用、且其上游模型 ID 没有历史真实成功证据的候选中：

- 2 个完整链路成功并完成媒体验证、任务日志和成本结算：Dimensio、8yes。
- 2 个通过本地毛利门禁并真实到达上游，但供应商失败：CLMM、Z5API。
- 4 个在本地严格 30% 门禁前被阻断，没有创建任务、成本请求或访问供应商：Paipu、Cangyuan、Lucen、4SToken。
- 1 个请求到达 OmegaAI，但上游拒绝当前映射模型：OmegaAI。

本轮没有自动重试、自动换模型或跨渠道回退。临时目标优先级在每次提交后恢复，最终所有目标和应用状态均已恢复。

## 前置设置与范围

| 项目 | 结果 |
| --- | --- |
| 成本模式 | `strict` |
| 全局最低预计毛利率 | `3000 BPS`（30%） |
| Ark SDK | `volcengine-python-sdk 5.0.44` |
| SDK 重试 | `max_retries=0`；轮询只读 |
| 本地入口 | `POST /api/v3/contents/generations/tasks` |
| 认证/固定渠道 | 管理员 Token `#2`，通过 `sk-<token>-<channel_id>` 固定目标；凭据未写入文件或报告 |
| 页面默认素材 | `r2v_tea_pic1.jpg`、`r2v_tea_pic2.jpg`、`r2v_tea_video1.mp4`、`r2v_tea_audio1.mp3` |
| 默认提示词 | 取自 `web/src/features/video-generation/lib/defaults.ts`，果茶第一视角广告提示词 |

官方 SDK 的高层 `tasks.create()` 会把多个可选字段以 `null` 发出，而本地兼容层会按字段存在性拒绝这些字段。首轮预检因此全部在本地请求校验层结束，未产生任务或供应商费用；正式轮改用同一官方 SDK 的稀疏请求体、官方传输和响应解析，确保真实请求体只包含非空 Ark 字段。首轮预检不计入正式 Canary。

已跳过的历史真实成功上游模型 ID：`jimeng-video-seedance-2.0-fast-vip`、`mg-seedance2.0 -480p mini`、`seedance-480p-token`、`seedance-2.0-mini`、`lec-seedance-2-0-933-stable`、`video-2.0-pro`、`video-2.0-mini`、`4sdance_mini431`、`videos-4-mini-480p`、`sd-2-c2`。

## 正式 Canary 结果

| 渠道/目标 | 上游模型 ID | 请求结果 | 本地任务/成本请求 | 说明 |
| --- | --- | --- | --- | --- |
| Dimensio `#5` / `#120071` `MAP-DIMENSIO-R108-720` | `jmg-video-seedance-2.0-vip` | 成功 | `#4807` / `#4786` | 真实任务成功并结算 |
| CLMM `#21` / `#120068` `MAP-CLMM-R20-720` | `seedance2.0 720p-pro` | 上游 HTTP 502 | 无任务 / `#4787` | `upstream_response_rejected`，成本未知 |
| Paipu `#22` / `#120101` `MAP-PAIPU-R24-480` | `lec-seedance-videos-standard` | 本地 HTTP 503 | 无 / 无 | `margin_below_threshold` |
| Cangyuan `#23` / `#120059` `MAP-CANGYUANSUANLI-R102-480` | `sd5-seedance-2.0` | 本地 HTTP 503 | 无 / 无 | `margin_below_threshold` |
| Lucen `#27` / `#120084` `MAP-LUCEN-R53-720` | `seedance-720p-5s` | 本地 HTTP 503 | 无 / 无 | `margin_below_threshold` |
| 8yes `#46` / `#120055` `MAP-8YES-R66-480` | `videos-4-480p` | 成功 | `#4808` / `#4788` | 真实任务成功并结算 |
| Z5API `#47` / `#120125` `MAP-Z5API-R197-720` | `sd-2-c3` | 上游 HTTP 503 | 无任务 / `#4789` | `upstream_response_rejected`，成本未知 |
| 4SToken `#48` / `#120042` `MAP-4STOKEN-R177-480` | `4sdance431` | 本地 HTTP 503 | 无 / 无 | `margin_below_threshold` |
| OmegaAI `#49` / `#120098` `MAP-OMEGAAI-R193-720` | `huanjue-25-720` | 上游 HTTP 400 | 无任务 / 无 | `InvalidParameter.model`，映射模型不被 OmegaAI 支持 |

## 成功链路审计

### Dimensio

- Ark 任务 ID：`task_CUmPLCngMA3E4H8JCmOUmRUDfK3IOWXy`
- 数据库任务：`#4807`，`SUCCESS`；上游任务 ID：`af697f12-0901-4258-83b1-0fb062844aad`
- 路由策略/目标：策略 `#3`，目标 `#120071`，上游模型 `jmg-video-seedance-2.0-vip`
- 使用日志：`#5897`；成本请求/尝试：`#4786` / `#4786`
- 成本模式：`per_duration`；5 秒计量；尝试状态 `settled`
- 收入：`$0.831976000`；供应商成本：`$0.465753425`；毛利润：`$0.366222575`；毛利率：`44.0184%`
- 媒体验证：Range GET 返回 `206`、`video/mp4`，媒体总长度 `2,289,198` 字节
- Ark 终态 usage：`216908` completion tokens；公开 duration `5`、resolution `720p`

### 8yes

- Ark 任务 ID：`task_kUMQdGEF5dvSMnuRksBLmpdpIsof0lqS`
- 数据库任务：`#4808`，`SUCCESS`；上游任务 ID：`task_kMdFnCdPdqkQAA7TAhZ77uH5Q542UetJ`
- 路由策略/目标：策略 `#3`，目标 `#120055`，上游模型 `videos-4-480p`
- 使用日志：`#5902`；成本请求/尝试：`#4788` / `#4788`
- 成本模式：`per_duration`；5 秒计量；尝试状态 `settled`
- 收入：`$0.386868000`；供应商成本：`$0.242123288`；毛利润：`$0.144744712`；毛利率：`37.4145%`
- 媒体验证：Range GET 返回 `206`、`video/mp4`，媒体总长度 `1,238,354` 字节
- Ark 终态 usage：`100862` completion tokens；公开 duration `5`、resolution `480p`

两条成功链路合计收入 `$1.218844000`、供应商成本 `$0.707876713`、毛利润 `$0.510967287`，加权毛利率约 `41.9223%`，高于全局 30% 门槛。

## 严格门禁证据

以下请求均记录管理员可见路由诊断，`status=503`，没有成本请求和任务：

| 渠道 | 预计收入 | 预计成本 | 预计毛利率 | 诊断 |
| --- | ---: | ---: | ---: | --- |
| Paipu | `$0.386868000` | `$0.547945205` | `-41.6362%` | `margin_below_threshold`，日志 `#5893/#5899` |
| Cangyuan | `$0.386868000` | `$0.458904110` | `-18.6203%` | `margin_below_threshold`，日志 `#5894/#5900` |
| Lucen | `$0.831976000` | `$0.684931507` | `17.6741%` | `margin_below_threshold`，日志 `#5895/#5901` |
| 4SToken | `$0.386868000` | `$0.493150685` | `-27.4726%` | `margin_below_threshold`，日志 `#5896/#5904` |

这证明严格模式使用完整供应商成本进行提交前门禁；未达 30% 的请求没有触发上游费用。

## 未闭环项与风险

1. CLMM 成本请求 `#4787` 和 Z5API 成本请求 `#4789` 均为 `upstream_response_rejected / cost_unknown`、收入状态 `pending`、利润状态 `incomplete_revenue`。供应商明确失败但尚未形成零成本收口，沿用现有成本核算遗留问题。
2. Dimensio 与 8yes 成功任务的 Ark 终态均显示 `generate_audio=true`，尽管页面默认开关为关闭且请求体省略该字段；本轮未产生额外费用，但需要单独确认“省略字段”与上游默认值的产品契约。
3. 8yes 任务 `#4808` 的数据库 `progress` 仍为 `30%`，但 `status=SUCCESS`、媒体和成本均已完成；这是任务进度字段与终态状态不一致的观测问题。
4. 成功链路使用管理员固定渠道 Token 以确保目标可审计；正式运营前仍应使用内部普通用户 Token 做一次不固定渠道的端到端回归。

## 验证与恢复

- `run_canary.py` 已通过 Python 编译检查；官方 SDK 稀疏请求使用 `max_retries=0`。
- 新增正式任务仅 `#4807`、`#4808`；新增成本请求仅 `#4786` 至 `#4789`。
- 所有候选目标优先级已恢复：`120071=12`、`120068=2`、`120101=16`、`120059=5`、`120084=11`、`120055=3`、`120125=4`、`120042=12`、`120098=1`。
- 应用、MySQL、Redis、视频元数据服务均保持健康；成本设置保持 `strict + 3000 BPS`。
- 输出工件：`outputs/2026-08-09-real-upstream-canary-margin-30/canary-results-v2.json`。其中不含 API Key、管理员 Token 或供应商密钥；首轮 SDK null 字段预检记录在 `canary-results.json`，未产生真实任务或费用。
