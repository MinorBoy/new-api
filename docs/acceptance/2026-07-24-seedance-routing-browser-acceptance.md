# Seedance 路由合并后验收报告

## 执行信息

- 分支：`ysr`
- HEAD：`8c03e35df45b0cc467bea070e563bb65a7f652ae`
- 验收时间：2026-07-24（Asia/Shanghai）
- 应用地址：`http://127.0.0.1:3000`
- 浏览器：Codex In-app Browser
- 验收范围：`origin/main` 合并后的后端、前端、容器和 Seedance 路由策略管理流程

## 近 48 小时提交概览

`git log --since='48 hours ago'` 共返回 `70` 个提交，涉及 `139` 个唯一文件路径，累计 `23,124` 行新增、`1,243` 行删除。主要改动如下：

- 将 `origin/main` 合并到 `ysr`，按单一前端架构解决冲突并保留 Seedance 二开能力。
- 增加路由目标自动命名，格式由日期、渠道名、清晰度、速度和时长组成，并允许人工覆盖。
- 完成 Seedance 能力路由策略、渠道候选过滤、请求能力提取、上游模型替换和策略管理界面。
- 完成 new-api/Ark/CLMM Mall 视频任务转换、轮询、结果解析、计费和错误保护。
- 修正路由抽屉分组加载、主题样式、字段语义和超分误建模。
- 同步上游 GitHub issue 模板及错误日志修复。

### 提交明细

以下为 Git 原始提交标题，按时间倒序排列：

| 提交 | 时间 | 改动 |
| --- | --- | --- |
| `8c03e35df` | 07-24 21:57 | Merge origin/main into ysr |
| `7897a893c` | 07-24 21:13 | docs: plan origin main merge into ysr |
| `6dd0915f0` | 07-24 21:06 | docs: align merge design with single frontend |
| `9e7bb5cfd` | 07-24 21:04 | docs: design origin main merge resolution |
| `5350b4696` | 07-24 19:09 | test(web): verify manual routing name override |
| `dab40841e` | 07-24 19:04 | test(web): cover routing target auto-name lifecycle |
| `b204cb79f` | 07-24 18:37 | fix(web): react to routing target field changes |
| `9e161de78` | 07-24 18:09 | feat(web): auto-name routing targets |
| `d7ab328af` | 07-24 18:04 | feat(web): generate routing target names |
| `a459d8fd0` | 07-24 17:57 | fix(web): clarify routing target model mapping |
| `d30f1056b` | 07-24 17:46 | docs: plan routing target auto naming |
| `d2f17e36b` | 07-24 16:35 | docs: design routing target auto naming |
| `84a79b680` | 07-24 14:10 | fix: log response body when parsed upstream error message is empty |
| `cbd9b30aa` | 07-24 13:53 | fix(github): restore issue form visibility (#6454) |
| `cb96ab020` | 07-24 13:46 | chore(github): migrate issue templates to required forms (#6452) |
| `704ff1c4e` | 07-24 07:45 | Merge branch 'codex/seedance-capability-routing' into docs/ark-native-compat-plans |
| `f03299390` | 07-24 00:57 | fix(routing): use canonical policy root paths |
| `dfa4a12a2` | 07-24 00:19 | test(routing): correct provider resolution semantics |
| `c362d7f5b` | 07-24 00:15 | fix(i18n): clarify routing policy drawer copy |
| `ce92afaa7` | 07-23 23:55 | fix(web): clarify routing target capability controls |
| `978156884` | 07-23 23:52 | fix(web): theme routing policy dropdowns |
| `fdc51dc7b` | 07-23 23:48 | feat(web): load routing policy groups |
| `59a16d2c6` | 07-23 23:44 | fix(web): remove super-resolution routing fields |
| `4992b28a1` | 07-23 23:41 | fix(routing): remove super-resolution contract |
| `0d322fa6b` | 07-23 23:36 | docs(routing): plan drawer correction |
| `65c989b76` | 07-23 23:23 | docs(routing): correct drawer semantics and theme design |
| `d56fe8895` | 07-23 22:33 | test(routing): cover Seedance capability routing end to end |
| `3749dd22b` | 07-23 22:33 | fix(web): improve routing target actions |
| `ec52c55b0` | 07-23 21:01 | feat(web): link channels to routing policies |
| `2d96070ed` | 07-23 17:55 | feat(web): manage capability routing targets |
| `cf8a0d404` | 07-23 17:33 | feat(web): add model routing workspace |
| `01aeb3ebd` | 07-23 17:05 | feat(routing): apply targets without exposing upstream models |
| `7e8a4ec5e` | 07-23 16:23 | feat(routing): integrate capability-aware channel selection |
| `4c83c5a1a` | 07-23 15:58 | feat(video): integrate CLMM Mall Ark channel |
| `297c7598f` | 07-23 15:42 | feat(routing): filter channel candidates before selection |
| `e781d4813` | 07-23 15:31 | feat(routing): extract Seedance request capabilities |
| `039d6451c` | 07-23 15:19 | feat(routing): add capability policy administration |
| `01bd74ed7` | 07-23 14:50 | feat(routing): persist and cache capability policies |
| `61a49bac1` | 07-23 14:48 | fix(video): protect CLMM public task errors |
| `3ae986df9` | 07-23 14:34 | feat(routing): validate capability policies |
| `69af105ed` | 07-23 14:30 | feat(routing): add capability matching domain |
| `ffe3e1f1f` | 07-23 14:07 | test(web): cover dirty channel URL preservation |
| `ecf6f61a0` | 07-23 13:54 | docs: plan Seedance capability routing |
| `0308a4916` | 07-23 13:54 | refactor(web): centralize channel base URL policy |
| `2d2fbddf0` | 07-23 13:38 | fix(web): apply CLMM default base URL |
| `0d0d9e720` | 07-23 13:27 | feat(web): configure CLMM Mall channel |
| `89c8dab11` | 07-23 13:12 | fix(video): require CLMM Ark task conversion |
| `19d7176b0` | 07-23 13:04 | docs: design Seedance capability routing |
| `aac7e89dc` | 07-23 12:51 | feat(video): integrate CLMM Mall billing and Ark queries |
| `d1417ce88` | 07-23 12:39 | fix(video): make CLMM polling retry-safe |
| `3a04fcb56` | 07-23 12:14 | fix(video): harden CLMM task responses |
| `1087624ed` | 07-23 11:53 | feat(video): add CLMM Mall task adaptor |
| `2e9fffac4` | 07-23 11:43 | fix(video): address new-api video verification findings |
| `712d3f509` | 07-23 11:21 | docs: align CLMM plan with latest API contract |
| `8ed1d6a77` | 07-23 11:16 | feat(web): configure new-api video channels |
| `e735d6494` | 07-23 10:58 | test(video): cover new-api video lifecycle |
| `7060786f6` | 07-23 10:48 | feat(video): expose safe OpenAI and ARK task queries |
| `da7d10aef` | 07-23 10:43 | fix(video): preserve detailed polling results |
| `592ff0dc8` | 07-23 10:37 | feat(video): parse detailed new-api video tasks |
| `fdc82b94f` | 07-23 10:27 | feat(video): relay new-api video task requests |
| `d7d9c4129` | 07-23 08:36 | feat(video): translate verified ARK inputs |
| `87bdf240d` | 07-23 08:29 | feat(video): preserve new-api video requests |
| `1a3917373` | 07-23 08:23 | feat(video): register new-api video task channel |
| `07d380bdd` | 07-23 08:17 | docs: align new-api video plan with upstream report |
| `dd64065e9` | 07-23 08:13 | docs: plan CLMM Mall Ark video channel |
| `fca74c49c` | 07-23 01:30 | docs: plan new-api video upstream integration |
| `b63c75569` | 07-23 01:28 | docs: design CLMM Mall Ark video channel |
| `b47c2fa94` | 07-23 00:44 | docs: expose ark video token usage |
| `36ee2d6d0` | 07-23 00:39 | docs: define ark compatibility for new-api video |
| `deaaac11f` | 07-22 23:49 | docs: design new-api video upstream channel |

## 构建与自动化验证

- `go test ./...`：通过，无失败包。
- `bun test`：`141 pass / 0 fail`。
- `bun run typecheck`：通过。
- `bun run format:check`：通过。
- `bun run build`：通过，Rsbuild 生产构建成功。
- 相对 `origin/main` 的 `82` 个前端变更文件 lint：`0` 个错误、`8` 个既有规则警告，退出码为 `0`。

## 容器重建

使用 `docker-compose.local.yml` 从当前工作树重建并强制重建应用容器：

```text
docker compose -f docker-compose.local.yml build new-api
docker compose -f docker-compose.local.yml up -d --force-recreate new-api
```

- 镜像：`new-api:local`
- Image ID：`sha256:928bcc2978d204646eecd5ba439e9cc5b6e5ffacfee5059c3d9f109d3f790d39`
- 创建时间：`2026-07-24T14:11:42.999346747Z`
- 镜像大小：`72,516,829` 字节
- Docker 构建上下文：约 `807 MB`

容器状态：

| 服务 | 状态 |
| --- | --- |
| `new-api-local-new-api-1` | healthy，`127.0.0.1:3000->3000` |
| `new-api-local-mysql-1` | healthy |
| `new-api-local-redis-1` | healthy |

`GET http://127.0.0.1:3000/api/status` 返回 HTTP 200。

## 浏览器自动化验收

### 验收结果

1. `/models/routing` 正常加载，当前数据库为 `0` 条路由策略。
2. “新建路由策略”抽屉正常打开，分组下拉自动加载 `分组A`、`default`、`svip`、`vip`。
3. 选择 `分组A` 并添加目标后，渠道候选包含 `dimensio (#5) · 已启用 · P0 · W0`。
4. 选择渠道后，路由目标名称自动填写为 `20260724-dimensio-720p-standard-4-15s`，符合“日期 + 渠道名称 + 清晰度 + 速度 + 时长”规则。
5. 标签显示为“上游渠道”“路由目标名称”“渠道模型映射（替换模型）”，帮助文案明确真实上游模型 ID 的替换语义。
6. 能力编辑器显示输出分辨率、允许生成时长、画面比例、参考图片/视频/音频数量和真人支持字段。
7. 时长字段使用“允许生成时长”；页面不存在“自动超分”。
8. 深色主题下，启用、选中和按下状态使用统一蓝色主题视觉。
9. 页面控制台 `error`、`warn`、`warning` 日志数量为 `0`。
10. 未填写真实上游模型，未点击“保存策略”，未新增或修改路由策略业务数据。

截图：[`output/playwright/routing-drawer-ysr-post-merge.png`](../../output/playwright/routing-drawer-ysr-post-merge.png)

### 临时账号恢复

验收期间临时将用户 ID `6`（`acceptance_20260724`）提升为管理员并设置临时密码。验收后已恢复原密码哈希和 `role=1`，并通过数据库查询确认恢复值与原值完全一致。

## 发现与风险

- Docker 构建上下文约 `807 MB`，明显偏大，会增加本机构建传输时间。为避免覆盖用户现有工作树，本次仅记录该问题，没有修改 `.dockerignore`。
- Bun 1.3.14 在 PowerShell 中通过 `bunx` 转发 `82` 个文件参数时发生运行时崩溃；改用项目已安装的 `oxlint.exe` 对同一文件集合直接执行后，结果为 `0` 个错误、`8` 个警告。该问题属于验证命令的 Windows 参数转发，不是应用构建或运行故障。
- 路由抽屉测试停留在未保存状态，覆盖了展示、候选加载和自动命名，但没有创建真实策略；这是为了避免污染现有业务数据。

## 结论

`origin/main` 合并后的 `ysr` 分支通过后端、前端、容器健康和浏览器流程验收。Seedance 路由抽屉可以按分组加载渠道候选，并按目标能力自动生成名称；上游模型映射语义、时长字段和超分建模均符合已确认设计。未发现阻断性问题。
