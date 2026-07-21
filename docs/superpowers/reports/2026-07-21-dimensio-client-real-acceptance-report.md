# Dimensio 客户端真实验收报告

## 结论

`dimensio_acceptance` 的 `image`、`text` 和 `multimodal` 三种模式已通过真实 Dimensio 链路验收。默认模式为双图生视频，命令行可通过 `--mode` 切换模式；多模态模式实际提交 4 张参考图、1 个 4 秒参考视频和 1 个 3 秒参考音频。

本轮发现并修复两个客户端可靠性问题：提交阶段使用独立的 120 秒超时；成品视频下载对暂态网络错误最多尝试 3 次。多模态任务最终成功，第一次成品下载遇到 CapCut CDN TLS EOF；修复后复用同一成功任务的 URL 下载完成，没有创建或计费第二个任务。

功能链路通过，但发现一项待确认的计费差异：new-api 对 4 秒 fast-vip 多模态任务按 ¥1.920002 记账，上游实际扣除 432 积分（按 1 积分 = ¥0.01 为 ¥4.32）。在确认 Dimensio 多模态定价规则前，不应把当前销售价格视为已完成生产验收。

## 环境

- 分支：`docs/ark-native-compat-plans`
- 服务：`http://127.0.0.1:3000`
- 镜像：`new-api:local`
- 数据库：MySQL 8.2
- 缓存：Redis 7 Alpine
- 模型：`jimeng-video-seedance-2.0-fast-vip`
- 视频参数：`16:9`、`720p`、`4s`
- API Key：仅通过隐藏终端输入传递；源码、Git diff 和报告均未保存明文

## 真实任务结果

| 模式 | 客户端任务 ID | 上游状态 | 客户端结果 | 成品 |
| --- | --- | --- | --- | --- |
| `image` | `task_ucIwpkrmrKWvZOs9I3GFHRbTegU0FeBc` | 成功 | 通过 | 6,060,775 bytes |
| `text` | `task_2qqcvLv3F3r9UW17hV9WuHJyAhFzWC8u` | 成功 | 通过 | 8,570,834 bytes |
| `multimodal` | `task_zrRyj4gki2rUkdIjtbI7uNk7dtx0J1O1` | 成功 | 分阶段恢复通过 | 7,417,195 bytes，MP4 `ftyp` 有效 |

多模态任务的六个媒体预检全部通过：4 个 `image/jpeg`、1 个具有 MP4 `ftyp` box 的参考视频、1 个 `audio/mpeg`。最终上游状态从 `queued` 进入 `running`，再进入 `succeeded`，总耗时 252.281 秒。

原始多模态 `report.json` 保留第一次下载失败的事实，错误为 `SSL: UNEXPECTED_EOF_WHILE_READING`。同一目录的 `download-recovery.json` 记录修复后对同一任务成品的恢复下载，`submitted_new_task=false`。

## 失败消息核对

### 上游“失败”任务

脚本按预期拿到了完整错误。

- 公开任务 ID：`task_7fNcR3A6P2v56mdnAAVnUxY37FpraNYR`
- `final_response.status`：`failed`
- 错误类别：`task`
- 上游错误码：`1001`
- 上游消息：`请求参数非法，可能是参考音频文件问题，请更换或重新处理参考音频后重试`
- 上游控制台时间：2026-07-21 09:40:24

该错误由旧参考音频 `horse.mp3` 触发。参考音频已替换为标准 MP3 `sample-3s.mp3`，后续同结构多模态任务成功。

### 上游“被拦截”任务

旧版脚本没有拿到最终拦截消息，只拿到提交超时。这不是错误解析失败，而是提交阶段在收到任务 ID 前被旧的 30 秒客户端超时取消。

- 客户端错误类别：`network`
- 客户端消息：`Gateway request failed: timed out`
- `task_id`：`null`
- `submit_response`：`null`
- new-api 日志：上游 POST 在 30.124 秒后 `context canceled`，预扣 ¥1.920002 已返还
- 上游稍后生成记录：`fail_code 2042`
- 上游最终消息：`视频安全审核不通过` / `The video you uploaded may contain content that violates our Community Guidelines. Try another video.`
- 上游控制台时间：2026-07-21 01:18:01

由于客户端没有公开任务 ID，无法继续调用查询接口取得该终态。当前脚本已把提交超时提高到 120 秒，并有自动化测试锁定提交与轮询使用不同超时；后续只要提交响应在 120 秒内返回任务 ID，通用失败分支会保留上游错误码和消息。

## 日志与计费核对

成功的多模态任务在 new-api 中生成了任务记录和消费记录：

- 渠道：`#5`
- Token：`dimensio`
- 模型：`jimeng-video-seedance-2.0-fast-vip`
- 计费模式：`per_duration`
- 请求/计费时长：4 秒
- quota：`131507`
- 用户费用：¥1.920002
- 提交响应：HTTP 200，耗时 9.5868 秒
- 结算：预扣与实际消耗一致，无差额调整

Dimensio 上游控制台显示同一任务成功、耗时 201.0 秒、扣除 432 积分。按现有成本口径，4 秒普通 fast-vip 预期为 192 积分；多模态真实扣费高出 240 积分。需要向 Dimensio 确认 `omni_reference` 是否按独立倍率或固定附加成本收费，并据此调整 `DurationPrice` 或引入可审计的模式倍率。

## 自动化验证

```text
python -m unittest scripts.test_dimensio_acceptance
PASS: 14 tests, 0 failures

python -m unittest scripts.test_dimensio_acceptance.DimensioAcceptanceTest.test_download_retries_a_transient_network_failure
RED: 首次 URLError 直接导致 AcceptanceError
GREEN: 第二次下载成功，只调用下载 URL，不提交任务

git diff --check -- scripts/dimensio_acceptance.py scripts/test_dimensio_acceptance.py
PASS
```

浏览器只读验收确认 Dimensio 上游生成记录中的成功、失败、被拦截状态及详情消息。成品文件检查确认大小为 7,417,195 bytes，文件头包含标准 MP4 `ftyp` box。报告全文检索未发现 `Authorization`、`Bearer` 或 `sk-` 明文。

## 验收清单

- [x] 默认执行双图生视频
- [x] `--mode text` 执行文生视频
- [x] `--mode multimodal` 提交 4 图 + 1 视频 + 1 音频
- [x] 参考视频为 4 秒 MP4
- [x] 六个媒体 URL 在提交前匿名预检
- [x] 提交超时独立提高到 120 秒
- [x] 上游失败错误码和消息写入脱敏报告
- [x] 成功任务轮询到终态并下载 MP4
- [x] 暂态下载错误最多重试 3 次
- [x] API Key 不写入代码、Git 或报告
- [x] new-api 任务日志与使用日志存在
- [ ] 确认多模态 432 积分的正式计价规则并修正销售计费

## 相关提交

- `9c498a480` `feat(dimensio): add acceptance test modes`
- `aae6ef8b1` `test(dimensio): validate multimodal acceptance assets`
- `53325eb82` `fix(dimensio): extend acceptance submit timeout`
- `40525da2d` `fix(dimensio): use compatible reference audio`
- `eb2f427ba` `fix(dimensio): retry transient video downloads`
