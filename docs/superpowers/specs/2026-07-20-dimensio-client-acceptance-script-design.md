# Dimensio 客户端真实验收脚本设计

## 目标

提供一个 Python 标准库脚本。使用者只需在文件顶部填写 new-api 网关 `BASE_URL` 和控制台创建的 `API_KEY`，即可通过 ARK `/api/v3` 接口提交真实 Dimensio 文生视频任务、轮询到终态、下载视频，并生成可审计的 JSON 报告。

## 范围

- 新增 `scripts/dimensio_acceptance.py`，不依赖第三方 Python 包。
- 新增 `scripts/test_dimensio_acceptance.py`，使用本地 HTTP mock 验证完整生命周期。
- `BASE_URL` 指向 new-api 网关，例如 `http://127.0.0.1:3000`；不是 `https://jimeng.dimensio.cn` 上游地址。
- `API_KEY` 是 new-api 控制台创建的 API 令牌；脚本和报告不得输出完整令牌。
- 默认执行一次 4 秒、720p、16:9 的纯文生视频验收，不需要使用者再准备图片、视频或音频 URL。
- 不测试通用渠道测试接口，不绕过 new-api 直接调用 Dimensio。

## 固定验收请求

脚本调用：

```text
POST {BASE_URL}/api/v3/contents/generations/tasks
GET  {BASE_URL}/api/v3/contents/generations/tasks/{PUBLIC_TASK_ID}
```

两个请求均携带：

```text
Authorization: Bearer {API_KEY}
Accept: application/json
```

提交请求固定为最小有效 ARK v3 文生视频负载：

```json
{
  "model": "jimeng-video-seedance-2.0-fast-vip",
  "content": [
    {
      "type": "text",
      "text": "A calm cinematic shot of morning light moving across a modern city skyline, slow camera push forward."
    }
  ],
  "ratio": "16:9",
  "resolution": "720p",
  "duration": 4
}
```

该模型和参数组合符合当前 Dimensio 约束，并将真实经过渠道选择、模型路由、时长计费、上游提交和任务查询链路。

## 运行流程

1. 校验 `BASE_URL` 是 HTTP(S) 地址，且 `API_KEY` 已替换占位值。
2. 规范化 `BASE_URL`，避免用户填写末尾 `/` 时生成双斜线。
3. 创建带时间戳的 `output/dimensio-client-acceptance/<run-id>/` 目录。
4. 提交任务并要求响应含非空公开 `id`；保存提交响应。
5. 每 5 秒查询一次任务，最长等待 15 分钟：
   - `queued`、`running`：继续轮询；
   - `succeeded`：要求 `content.video_url` 非空；
   - `failed`：提取 `error.code` 和 `error.message` 后失败退出；
   - 其他状态：按协议异常失败退出。
6. 将 `content.video_url` 下载到临时文件，成功后原子改名为 `video.mp4`，避免失败时留下看似完整的视频。
7. 无论成功或失败，都写入 `report.json`；成功时退出码为 0，失败时退出码为 1，Ctrl+C 返回 130。

## 输出与安全

成功输出目录包含：

- `video.mp4`：下载后的真实生成结果；
- `report.json`：请求参数、公开任务 ID、状态历史、提交/最终响应、视频 URL、本地路径、开始/结束时间和耗时。

报告只记录 `api_key_hint`，格式为末四位掩码，例如 `***abcd`，不记录 Authorization 请求头或完整 API Key。控制台同样不打印完整令牌。

## 错误处理

- 网络连接、DNS、TLS、HTTP 超时和下载错误均转为简洁错误信息，并保留到报告。
- HTTP 非 2xx 响应优先解析 ARK `error.code/error.message`，兼容顶层 `code/message`，并保留 HTTP 状态和 JSON 响应。
- 2xx 但非 JSON、缺少任务 `id`、未知状态、成功但缺少 `content.video_url` 均视为协议失败。
- 下载使用分块写入；失败时删除临时文件，不覆盖已有完整文件。

## 测试设计

测试只使用 Python 标准库 `unittest` 和本地 `ThreadingHTTPServer`，不调用真实上游：

1. 成功生命周期：mock 依次返回 `queued`、`running`、`succeeded`，验证 Authorization、请求路径、固定负载、状态历史、视频字节和通过报告。
2. 任务失败：mock 返回 `failed`，验证错误码/消息进入失败报告且不下载视频。
3. 请求错误：mock 返回非 2xx ARK 错误，验证 HTTP 状态和错误结构被保留。
4. 配置校验：占位 API Key 或非法 Base URL 在发起网络请求前失败。

实现阶段先写失败测试并确认 RED，再写最小实现转为 GREEN；最后运行完整 `unittest`、脚本语法编译和本地 mock 验收。

## 验收标准

- 使用者只修改 `BASE_URL` 和 `API_KEY` 即可运行。
- 真实成功任务最终生成可播放的 `video.mp4` 和 `report.json`。
- 报告能区分配置、HTTP、协议、任务和下载失败。
- 任何输出文件均不泄露完整 API Key。
- 脚本在 Windows PowerShell、macOS 和 Linux 的 Python 3.10+ 环境可运行。
