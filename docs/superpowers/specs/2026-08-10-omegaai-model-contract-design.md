# OmegaAI 新模型契约补齐设计

## 1. 背景

最新版 `sd收录.xlsx` 为 OmegaAI 新增两个活动路由目标：

- `MAP-OMEGAAI-R193-720` -> `huanjue-25-720`
- `MAP-OMEGAAI-R194-720` -> `db-ai-video-v1`

当前 OmegaAI 适配器只允许 `klsdpro2-720p`、`seedance-v2-720p`、`dola-seedance-2.0`、`lingjing-video-v1`。因此素材矩阵 E2E 在请求进入 Mock 上游前返回 `InvalidParameter.model`，全仓 Go 测试无法通过。

本设计依据供应商公开模型目录补齐模型契约，不通过放宽未知模型校验或修改 E2E 期望来绕过失败。

## 2. 契约证据

2026-08-10 读取 OmegaAI 公开接口 `https://omegaai.xin/api/models`，得到以下活动模型信息：

| 模型 | 类型与状态 | 素材能力 | 时长信息 |
| --- | --- | --- | --- |
| `huanjue-25-720` | `video`、`active`、`openaivideo` | 最多 9 图、3 视频、3 音频 | `minDuration=5`、`maxDuration=15` |
| `db-ai-video-v1` | `video`、`active`、`openaivideo` | 必须至少 1 图，最多 9 图，不支持视频和音频 | 模型元数据为 `fixedDuration=15` |

供应商公开页面同时为视频模型生成 `POST /v1/media/generate` 示例，并使用 `GET /v1/tasks/{task_id}` 查询任务。因此两个模型可以复用现有 OmegaAI 传输协议和响应解析。

存在一个需要真实 Canary 继续确认的差异：

- 最新 `sd收录.xlsx` 的 `db-ai-video-v1` 时长范围为 `5,10`。
- 供应商模型元数据为 `fixedDuration=15`。
- 供应商公开请求示例仍发送 `duration=5`。

当前证据不足以判定 `fixedDuration=15` 表示“只接受 15 秒”还是“上游固定输出 15 秒但兼容其他请求值”。本轮不擅自修改权威源表，也不在适配器中强制改写请求时长。

## 3. 目标

1. 让两个公开活动模型通过 OmegaAI 明确白名单校验。
2. 按模型执行素材数量和必填条件校验，拒绝供应商明确不支持的输入。
3. 保持未知 OmegaAI 模型、非 720p、`generate_audio`、`watermark`、`draft` 等现有拒绝行为。
4. 让后台和渠道编辑页展示同一份六模型目录。
5. 通过 Mock 素材矩阵和低成本真实 Canary 分别验证本地协议转换与供应商实际响应。

## 4. 非目标

- 不修改 Google 表格或本地 `sd收录.xlsx` 的 `db-ai-video-v1` 时长字段。
- 不手工修改自动生成 fixture 来隐藏能力差异。
- 不把任意自定义模型加入 OmegaAI 白名单。
- 不自动重试、切换目标或扩大真实上游 Canary 范围。
- 不把 HTTP 402、余额不足或模型校验通过描述为视频生成成功。

## 5. 适配器设计

### 5.1 模型目录

OmegaAI 模型目录增加：

```text
huanjue-25-720
db-ai-video-v1
```

后端 `GetModelList()` 和前端 `supportedModels` 使用相同顺序。渠道提示从“四个已记录模型”改为“六个已记录模型”。

### 5.2 模型级素材校验

现有全局上限继续作为第一层防线：最多 9 图、3 视频、3 音频。通过白名单后再应用模型级约束：

| 模型类别 | 图片 | 视频 | 音频 | 额外条件 |
| --- | ---: | ---: | ---: | --- |
| `klsdpro2-720p` | <= 9 | <= 3 | <= 3 | 保持现有行为 |
| `huanjue-25-720` | <= 9 | <= 3 | <= 3 | 请求时长存在时必须为 5–15 秒 |
| `db-ai-video-v1` | 1–9 | 0 | 0 | 缺图或包含视频/音频时返回 `InvalidParameter.content` |
| 其余已记录模型 | <= 9 | 0 | 0 | 保持现有仅文本和参考图行为 |

不新增只有单个调用者的通用抽象。模型级分支直接保留在 `validateOmegaAIRequest` 中，使供应商契约能够集中审阅。

### 5.3 时长冲突处理

`db-ai-video-v1` 暂时沿用路由目标允许的 5 秒或 10 秒请求，不在传输层改写为 15 秒。原因是传输层不能覆盖路由和成本核算已经采用的请求时长，否则可能造成用户收入、供应商成本和实际输出时长不一致。

真实 Canary 若明确返回时长不支持：

1. 停止后续请求，不自动改用 15 秒。
2. 保留供应商响应和本地成本核算证据。
3. 将 Google 表格的时长字段列为待业务确认项。
4. 源表修正并重新生成、导入、激活后再复验。

## 6. 测试设计

按 TDD 执行，先增加失败断言，再修改实现。

后端单元测试覆盖：

- 模型目录包含六个明确模型。
- `huanjue-25-720` 接受视频和音频素材。
- `huanjue-25-720` 拒绝 5 秒以下或 15 秒以上请求。
- `db-ai-video-v1` 拒绝无图请求。
- `db-ai-video-v1` 接受至少一张参考图。
- `db-ai-video-v1` 拒绝视频或音频素材。
- 未知模型继续返回 `InvalidParameter.model`。

前端测试覆盖六模型目录和更新后的提示。随后运行：

```powershell
go test ./relay/channel/task/newapivideo -count=1
Set-Location web
bun test tests/channel-type-config.test.ts
bun run typecheck
```

Mock 全链路运行：

```powershell
go test ./e2e -run '^TestSeedanceImportedMaterialMatrixFullFlowE2E$' -count=1 -p=1
go test ./... -count=1
```

## 7. 真实 Canary

只有本地测试全部通过后执行。使用本地已有 OmegaAI 渠道和凭据，不读取或输出 API Key；固定渠道、固定目标、串行且每个目标只提交一次：

| 目标 | 请求 |
| --- | --- |
| `huanjue-25-720` | 文本、5 秒、720p、16:9 |
| `db-ai-video-v1` | 1 张公开测试图、5 秒、720p、16:9 |

每次请求记录请求 ID、命中目标、HTTP 状态、供应商任务 ID、最终任务状态、收入、供应商成本、毛利润和毛利率。若首个请求暴露系统性配置错误或凭据/余额阻塞，不继续产生没有诊断价值的请求。

Canary 完成后恢复为执行前的成本模式、最低毛利率、分组倍率、目标优先级和渠道状态，并通过 API 或数据库复核。

## 8. 报告与验收

更新现有路由毛利目录验收报告，增加以下证据：

- OmegaAI 公开模型目录的读取日期和契约摘要。
- 单元测试、前端测试、Mock E2E 和全仓测试命令及结果。
- 两个真实 Canary 的独立结果，不以 Mock 成功替代真实供应商成功。
- `db-ai-video-v1` 时长冲突的最终判断或继续阻塞原因。
- 运行时设置和临时目标优先级的恢复结果。

验收成功条件是：本地全仓测试不再因两个已确认模型被白名单拒绝；真实 Canary 结果可审计；任何供应商余额、模型状态或时长冲突均如实保留，不修改契约来制造通过结论。

## 9. 回退

代码回退只需移除新增模型和模型级分支，并恢复前端目录及测试。该变更不涉及数据库迁移、持久化格式或公开客户端 API。若真实 Canary 失败，代码是否回退取决于失败性质：供应商明确否认模型或协议时回退模型支持；余额不足、排队或内容审核失败不等同于本地契约错误。
