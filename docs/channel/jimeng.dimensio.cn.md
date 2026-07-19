视频接口规格
面向开发者和 coding agent 的视频生成集成说明。
1. 认证
Authorization: Bearer <YOUR_API_KEY>

所有接口均需使用用户控制台创建的 API Key 或登录令牌认证，不再接受直接传入即梦 session token 作为 Authorization。任务状态只能由任务归属者查询。

2. 接口
Base URL
https://jimeng.dimensio.cn

方法
完整地址
说明
POST
https://jimeng.dimensio.cn/v1/videos/generations
提交视频任务，立即返回 task_id
GET
https://jimeng.dimensio.cn/v1/videos/tasks/:taskId
查询任务状态和结果 URL

视频生成是异步接口。提交接口只返回任务 ID；完成结果必须通过查询接口获取。

3. 当前开放模型
模型
分辨率
价格
jimeng-video-seedance-2.0-fast-vip
720p
720p: 48 积分/秒（不支持 1080p）
jimeng-video-seedance-2.0-mini
720p
720p: 39 积分/秒（不支持 1080p）
jimeng-video-seedance-2.0-vip
720p / 1080p
720p: 62 积分/秒，1080p: 155 积分/秒

1 积分 = CNY 0.01。供应商按视频实际生成时长消耗上述积分/秒成本。查询响应不返回 `duration`，因此系统的 `per_duration` 销售计费使用 `duration_source=request`，以提交时已校验的请求 `duration` 作为请求时长和计费时长，不在轮询完成后按查询响应重算。

4. 请求参数
字段
类型
必填
默认值
说明
model
string
是
-
视频模型名。可用值以“当前开放模型”表为准。支持地区前缀由服务端 token 解析。
prompt
string
是
-
视频生成提示词。使用 multipart 且 prompt 中包含 @ 时，curl 必须用 --form-string。
ratio
string
否
9:16
画幅比例。支持 1:1、4:3、3:4、16:9、9:16、21:9；列表外的值按 16:9 处理。图生视频时可能由素材比例决定。
resolution
string
否
720p
输出分辨率。是否支持 1080p 以模型表为准。
duration
number
否
5
秒数。Seedance 2.0 系列支持 4-15 整数秒。
functionMode
string
是
-
生成模式。first_last_frames 用于文生视频/图生视频/首尾帧（无图即纯文生）；omni_reference 为全能参考；agentic 是旧兼容别名。
intelligent_ratio
boolean
否
false
智能比例开关。JSON 使用布尔值；multipart 可传 true 或 false 字符串。
face_grid
boolean
否
后台默认
人脸网格处理开关，仅 Seedance 视频模型生效。不传按后台对应区域默认值；传 true 或 false 可覆盖本次请求。
file_paths
string[]
否
[]
图片 URL 数组。first_last_frames 最多 2 张；omni_reference 最多 9 张，agentic 按 omni_reference 处理。（也兼容驼峰别名 filePaths 及旧字段 reference_asset_urls。）

5. 模式与素材规则
functionMode
默认
素材
适用模型
omni_reference
默认
图片/视频/音频，多素材总数最多 12 个
Seedance 2.0 系列
agentic
兼容别名
按 omni_reference 处理
旧客户端兼容
first_last_frames
-
图片最多 2 张，第 1 张首帧，第 2 张尾帧
Seedance 2.0 系列
素材
字段名
限制
来源
图片
image_file_1 ... image_file_9
最多 9 张
multipart URL 字符串、file_paths 数组
视频
video_file_1 ... video_file_3
最多 3 个，视频总时长不超过 15 秒
multipart URL 字符串
音频
audio_file_1 ... audio_file_3
最多 3 个
multipart URL 字符串
￼
6. 返回格式
提交成功
{
  "created": 1709123456,
  "task_id": "1897234567890123456",
  "status": "pending"
}

查询中
{
  "task_id": "1897234567890123456",
  "status": "processing",
  "progress": 50
}

查询完成
{
  "task_id": "1897234567890123456",
  "status": "completed",
  "progress": 100,
  "result": {
    "url": "https://example.com/video.mp4"
  }
}

任务失败
{
  "task_id": "1897234567890123456",
  "status": "failed",
  "error": "视频安全审核不通过，请重试",
  "error_code": "2043"
}

result.url 由上游签发，有效期较短。链接失效时重新查询同一个 task_id 获取新 URL。

7. 状态值
status
说明
pending
任务已提交，等待开始。提交接口通常返回此状态。
processing
任务生成中。可继续轮询查询接口。
completed
任务完成。读取 result.url 获取视频地址。
failed
任务失败。读取 error 和 error_code。
not_found
任务不存在、本地无记录，或上游记录已过期。

8. 错误格式
请求级错误返回统一结构；不要只依赖 HTTP 状态码，调用方应读取 JSON 内的 code 和 message。
{
  "code": -2000,
  "message": "duration 必须是整数，当前值: 4.5",
  "data": null
}


code / error_code
含义
-2000
请求参数非法，例如字段类型错误、duration 非整数。
-2001
请求失败，例如素材处理失败、模式和模型不匹配。
-2008
视频生成失败。
-2011
资源不存在，例如任务不存在或已过期。
-2012
平台积分预算已用完。
1006 / 4001 / 5000
积分不足。
1014 / 1015 / 34010105
签名失败、登录失效或 Token 无效。
2038 / 2039 / 2042 / 2043
文本、图片或视频安全审核不通过。
1057 / 121101
请求过频或达到每日生成上限。

9. 示例
创建视频任务
curl -X POST https://jimeng.dimensio.cn/v1/videos/generations \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <YOUR_API_KEY>" \
  -d '{
    "model": "jimeng-video-seedance-2.0-fast-vip",
    "functionMode": "omni_reference",
    "prompt": "@image_file_1作为首帧，@image_file_2作为尾帧，动作参考@video_file_1，背景音乐参考@audio_file_1，镜头缓慢向前推进",
    "ratio": "16:9",
    "resolution": "720p",
    "duration": 4,
    "face_grid": true,
    "image_file_1": "https://example.com/first.jpg",
    "image_file_2": "https://example.com/last.jpg",
    "video_file_1": "https://example.com/reference.mp4",
    "audio_file_1": "https://example.com/reference.mp3"
  }'

查询任务
curl https://jimeng.dimensio.cn/v1/videos/tasks/<TASK_ID> \
  -H "Authorization: Bearer <YOUR_API_KEY>"
