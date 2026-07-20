# Dimensio 多模式客户端验收脚本设计

## 目标

扩展 `scripts/dimensio_acceptance.py`，让同一个 Python 标准库脚本支持纯文生视频、双图首尾帧生视频和多模态参考生视频。使用者仍然只需在文件顶部配置 new-api 网关 `BASE_URL` 与控制台 `API_KEY`；脚本内置固定公网测试素材，并通过 `--mode` 选择本次真实验收模式。

默认模式从现有纯文生视频改为双图图生视频。现有提交、轮询、视频下载、报告生成、错误分类和 API Key 脱敏行为继续保留。

## 范围

- 修改 `scripts/dimensio_acceptance.py`。
- 修改 `scripts/test_dimensio_acceptance.py`。
- 不修改 Dimensio 后端适配器、渠道配置、计费规则或控制台页面。
- 不引入第三方 Python 包。
- 不要求使用者提供本地文件、上传素材或配置额外 URL。
- 每次命令只运行一个视频生成模式，不增加一次运行全部模式的入口。

## 命令行契约

脚本使用 Python 标准库 `argparse`：

```text
python scripts/dimensio_acceptance.py [--mode {text,image,multimodal}]
```

模式定义：

| 参数值 | 含义 | Dimensio 模式 |
| --- | --- | --- |
| `text` | 纯文生视频，保留现有请求 | `first_last_frames`，无图片 |
| `image` | 两张图片分别作为首帧和尾帧 | `first_last_frames` |
| `multimodal` | 4 张参考图、1 个参考视频和 1 个参考音频 | `omni_reference` |

不传 `--mode` 时使用 `image`。非法值由 `argparse` 返回退出码 `2`，不创建运行目录、不检查素材、不提交任务，因此不会产生费用。

`run_acceptance()` 增加关键字参数 `mode: str = "image"`，使自动化测试和导入调用使用与 CLI 相同的模式选择逻辑。内部 `build_payload(mode)` 每次返回独立负载，避免一个运行或测试修改另一个模式的共享数据。

## 公共参数

三个模式都使用相同的真实验收参数：

```json
{
  "model": "jimeng-video-seedance-2.0-fast-vip",
  "ratio": "16:9",
  "resolution": "720p",
  "duration": 4
}
```

该模型按当前 Dimensio 定价为 720p 每秒 48 积分，因此每个真实 4 秒任务预计消耗 192 上游积分。脚本不并行运行模式，也不在失败后自动提交另一模式。

## 固定公网素材

素材使用无需鉴权的固定公网 URL：

| 标识 | 类型 | URL |
| --- | --- | --- |
| `image_1` | JPEG | `https://www.w3schools.com/w3images/lights.jpg` |
| `image_2` | JPEG | `https://www.w3schools.com/w3images/nature.jpg` |
| `image_3` | JPEG | `https://www.w3schools.com/w3images/mountains.jpg` |
| `image_4` | JPEG | `https://www.w3schools.com/w3images/forest.jpg` |
| `video_1` | MP4 | `https://media.githubusercontent.com/media/adilentiq/test-images/02f54833d4d08fc33d2090eaeda9f0d2dbf1c7b0/video/duration/v1_4s.mp4` |
| `audio_1` | MP3 | `https://www.w3schools.com/html/horse.mp3` |

设计阶段已验证 4 张图片和音频返回 HTTP 200 及对应媒体类型。参考视频固定到 Git 提交 `02f54833d4d08fc33d2090eaeda9f0d2dbf1c7b0`，文件大小为 699,838 字节；MP4 `mvhd` 元数据时长为 4.967 秒，符合 4-5 秒要求。

图片模式复用 `image_1`、`image_2`；多模态模式使用全部 4 张图片及固定视频和音频。固定 URL 集中在一个模块级映射中，用户无需编辑，但测试可临时替换为本地 mock 地址。

## 三种请求负载

### 纯文生视频

`text` 模式保留现有提示词和负载：

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

### 双图图生视频

`image` 模式是默认模式。第 1 张图为首帧，第 2 张图为尾帧：

```json
{
  "model": "jimeng-video-seedance-2.0-fast-vip",
  "content": [
    {
      "type": "image_url",
      "role": "first_frame",
      "image_url": {
        "url": "https://www.w3schools.com/w3images/lights.jpg"
      }
    },
    {
      "type": "image_url",
      "role": "last_frame",
      "image_url": {
        "url": "https://www.w3schools.com/w3images/nature.jpg"
      }
    },
    {
      "type": "text",
      "text": "Create a smooth cinematic transition from @image_file_1 as the first frame to @image_file_2 as the last frame, preserve realistic detail and use a slow forward camera movement."
    }
  ],
  "ratio": "16:9",
  "resolution": "720p",
  "duration": 4
}
```

网关根据 `first_frame` 和 `last_frame` 角色生成 Dimensio `first_last_frames` 请求，分别映射为 `image_file_1` 和 `image_file_2`。

### 多模态参考生视频

`multimodal` 模式包含 6 个参考素材和 1 条文本内容：

```json
{
  "model": "jimeng-video-seedance-2.0-fast-vip",
  "content": [
    {
      "type": "image_url",
      "role": "reference_image",
      "image_url": {"url": "https://www.w3schools.com/w3images/lights.jpg"}
    },
    {
      "type": "image_url",
      "role": "reference_image",
      "image_url": {"url": "https://www.w3schools.com/w3images/nature.jpg"}
    },
    {
      "type": "image_url",
      "role": "reference_image",
      "image_url": {"url": "https://www.w3schools.com/w3images/mountains.jpg"}
    },
    {
      "type": "image_url",
      "role": "reference_image",
      "image_url": {"url": "https://www.w3schools.com/w3images/forest.jpg"}
    },
    {
      "type": "video_url",
      "role": "reference_video",
      "video_url": {
        "url": "https://media.githubusercontent.com/media/adilentiq/test-images/02f54833d4d08fc33d2090eaeda9f0d2dbf1c7b0/video/duration/v1_4s.mp4"
      }
    },
    {
      "type": "audio_url",
      "role": "reference_audio",
      "audio_url": {"url": "https://www.w3schools.com/html/horse.mp3"}
    },
    {
      "type": "text",
      "text": "Use @image_file_1 through @image_file_4 as visual references, follow the motion from @video_file_1 and the rhythm from @audio_file_1, then create a coherent cinematic scene with a slow forward camera movement."
    }
  ],
  "ratio": "16:9",
  "resolution": "720p",
  "duration": 4
}
```

4 个图片角色均为 `reference_image`，视频和音频分别为 `reference_video`、`reference_audio`。网关将它们转换为 `image_file_1..4`、`video_file_1`、`audio_file_1`，并选择 `omni_reference`。媒体总数为 6，低于 Dimensio 的 12 个总素材上限。

## 素材预检

只对 `image` 和 `multimodal` 模式执行素材预检；`text` 模式没有素材请求。

预检要求：

1. 素材 URL 必须是完整 HTTP(S) 地址。
2. 请求不得包含 new-api `Authorization` 或 API Key。
3. 图片 HEAD 响应必须为 2xx 且 `Content-Type` 以 `image/` 开头。
4. 音频 HEAD 响应必须为 2xx 且 `Content-Type` 以 `audio/` 开头。
5. MP4 视频执行小范围 GET，只读取开头 12 字节；响应必须为 2xx，URL 后缀为 `.mp4`，且字节 4-7 为 `ftyp`。
6. 参考视频服务器返回 `application/octet-stream` 是允许的，因为 MP4 文件头是更强的格式信号。

任一素材不可访问或类型不符时抛出 `AcceptanceError(category="asset")`，报告记录失败 URL、素材类型、HTTP 状态或格式原因。脚本在素材全部通过前不调用网关任务提交接口，因此预检失败不产生上游费用。

预检只能证明客户端当前能访问素材，不能保证 Dimensio 所在网络一定能下载。若预检通过但上游素材处理失败，仍按已有 HTTP 或任务失败路径记录真实上游错误。

## 运行流程

1. CLI 解析 `--mode`；非法模式退出 2。
2. 创建 `output/dimensio-client-acceptance/<run-id>/`。
3. 校验 `BASE_URL` 和 `API_KEY`。
4. 通过 `build_payload(mode)` 构造独立请求负载。
5. 提取当前负载的媒体项并完成无凭据预检。
6. 调用 `POST /api/v3/contents/generations/tasks`。
7. 每 5 秒查询公开任务 ID，最长 15 分钟。
8. 成功后不携带网关 API Key 下载 `content.video_url` 到原子临时文件，再改名为 `video.mp4`。
9. 写入脱敏 `report.json` 并返回既有退出码：成功 0、运行失败 1、中断 130；仅 CLI 参数错误为 2。

## 报告结构

保留现有报告字段，并新增：

- `mode`：`text`、`image` 或 `multimodal`。
- `assets`：从实际负载提取的素材类型、角色和 URL。
- `asset_checks`：每个素材的检查结果、HTTP 状态、响应媒体类型和 MP4 文件头结果。

`request.payload` 必须记录本次模式的精确负载。报告继续仅保存 `api_key_hint=***<last4>`；完整 API Key 不得出现在请求负载、素材检查、HTTP 错误、任务响应、控制台或输出文件中。

每个模式仍产生独立目录中的 `video.mp4` 和 `report.json`。不改变输出文件名；模式通过报告字段区分。

## 测试设计

测试继续只使用 Python 标准库 `unittest` 和 `ThreadingHTTPServer`，不联系真实网关、W3Schools、GitHub 或 Dimensio。

### 模式与负载测试

- 无 CLI 参数时解析为 `image`。
- `--mode text`、`--mode image`、`--mode multimodal` 精确选择对应模式。
- 非法模式由 CLI 返回 2，且不调用 `run_acceptance()`。
- `text` 负载只有文本内容。
- `image` 负载精确包含 2 张图，角色为 `first_frame` 和 `last_frame`。
- `multimodal` 负载精确包含 4 个 `reference_image`、1 个 `reference_video`、1 个 `reference_audio` 和文本提示词。
- 三个负载公共模型、比例、分辨率和时长完全一致。
- 连续调用 `build_payload()` 返回互不共享的对象。

### 素材预检测试

测试临时替换固定素材映射为本地 HTTP server URL：

- 图片、音频和有效 MP4 文件头通过。
- 素材请求不含 `Authorization`。
- 非 2xx、图片类型错误、音频类型错误和无效 MP4 `ftyp` 分别返回 `asset` 错误。
- 任一预检失败后网关提交请求计数保持为 0，且仍生成失败报告。

### 生命周期测试

以 `text`、`image`、`multimodal` 三个子场景执行完整本地生命周期，验证：

- 提交路径、轮询路径和公开任务 ID。
- 网关请求携带 API Key，素材与结果视频下载不携带 API Key。
- mock 收到的 JSON 与所选模式负载完全一致。
- `queued -> running -> succeeded` 状态历史、视频字节和成功报告。
- 报告中的 `mode`、`assets`、`asset_checks` 与本次模式一致。

现有任务失败、非 2xx ARK/顶层错误、配置错误和密钥脱敏测试继续保留，并更新为显式模式或默认 `image` 行为。

## 真实验收

实现完成并通过本地测试后，使用隐藏输入传入真实 API Key，依次运行：

```text
python scripts/dimensio_acceptance.py
python scripts/dimensio_acceptance.py --mode multimodal
python scripts/dimensio_acceptance.py --mode text
```

第一条命令必须证明无参数默认提交双图图生视频。每个成功任务都需下载非空、具有有效 MP4 文件头的 `video.mp4`，并生成脱敏 `report.json`。控制台任务日志和使用日志用于交叉核对模式、模型、渠道、公开任务 ID、状态、耗时与 192 积分对应的本地费用。

## 验收标准

- 使用者仍然只编辑 `BASE_URL` 和 `API_KEY`。
- 无参数默认执行双图图生视频。
- `--mode text` 和 `--mode multimodal` 可选择其余模式。
- 多模态请求精确包含 4 图、1 个 4.967 秒 MP4 和 1 个音频。
- 素材失效在网关提交前失败，不产生任务费用。
- 三种模式均能完成真实提交、轮询、视频下载和脱敏报告流程。
- 单元测试不访问公网或真实上游。
- API Key 不发送给素材服务器或结果视频服务器，也不写入任何产物。
