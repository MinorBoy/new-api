# Seedance 视频供应商测试台设计

## 目标

提供一个无需构建的单文件 HTML 工具，用于直接测试火山方舟视频生成 API，重点覆盖 Seedance 2.0 系列的文生视频与多模态参考生视频。

## 交互结构

- 左侧配置区：供应商下拉、添加/删除供应商、Base URL、API Key、是否记住本地配置。
- 中间请求编辑区：模型预设/自定义 Model ID、提示词、参考图片/视频/音频 URL 列表、分辨率、比例、时长、生成音频、水印、尾帧和过期时间。
- 右侧结果区：请求 JSON、创建响应、任务时间线、轮询控制、视频预览、尾帧预览、原始响应和复制 cURL。

## 数据与 API

- 创建：`POST {baseUrl}/api/v3/contents/generations/tasks`。
- 查询：`GET {baseUrl}/api/v3/contents/generations/tasks/{taskId}`。
- 鉴权：`Authorization: Bearer {apiKey}`。
- `content` 数组按顺序生成 text、reference_image、reference_video、reference_audio 内容块。
- 图片最多 9 张，视频最多 3 个，音频最多 3 个，且只接受 HTTP(S) URL。
- 创建成功后每 5 秒查询一次，`queued`/`running` 继续轮询，`succeeded`/`failed`/`expired`/`cancelled` 结束。

## 关键约束

- 仅 Seedance 2.0 系列启用 4K 分辨率； Fast/Mini 禁用 1080p/4K。
- Seedance 2.0 系列时长限制为 4-15 秒或 `-1` 智能选择。
- 参考音频不能单独提交，必须至少有参考图片或参考视频。
- API Key 默认只保留在当前页面会话，用户显式勾选后才写入 localStorage。
- 所有错误展示 HTTP 状态、服务端消息和 CORS 诊断建议。

## 视觉方向

深石墨工作台、暖白文字、珊瑚红主操作色与青绿色状态色；以任务时间线、媒体预览和 JSON 检视作为视觉锚点，避免营销式 Hero 和卡片堆叠。

## 验证

- 静态 HTML 可直接打开，另支持通过本地 HTTP 服务打开以避免 `file://` 限制。
- Playwright 验证桌面与移动视口、表单添加/删除、URL 上限、请求预览和 mock API 轮询。
