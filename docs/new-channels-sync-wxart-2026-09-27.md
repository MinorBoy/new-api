# WxArt（x deal）渠道模型同步记录 — 2026-09-27/28

来源：用户已登录的 `x deal` 标签页（`https://wxart.space/console/models`，已登录）。写入目标为仓库工作簿 `docs/new-channels/sd收录.xlsx`（该目录被 `.gitignore` 忽略，工作簿与审计产物不入库；本记录为入库的操作摘要，完整报告与 DOM 证据、补丁、校验脚本保存在 `outputs/2026-09-27-sync-wxart-models/`）。

## 渠道身份

- channel 表 row 19：`wxart`，数字渠道 ID `17`，模型数据 API URL `https://wxart.space/console/models`（逐字一致）。
- 协议/能力证据：渠道合同 `docs/new-channels/cn-x-deal-api-docs.md`（视频统一 `POST https://api.wxart.space/v1/videos` → 协议=自有）。
- 价格单位：`R` 按渠道既定用户确认规则映射人民币元（`R/秒`→元/秒、`R/次`→元/次）；交叉印证：页面 seedance2.5 480p `0.35R/秒` 与 sd 表既有行单价 0.35 元完全一致。

## 业务类型判定

页面徽标为厂商家族标记（Google/OpenAI/Video）而非业务类型：`Google` 徽标同时覆盖 Veo/Omni 视频模型与 Nano Banana 图片模型。初筛改用双证据：卡片图标（`lucide-video`/`lucide-sparkles`）+ 渠道合同端点。视频候选 7 个（veo3.1、omni-flash、veo3.1-fast、veo3.1-lite、seedance2.0、minimax-h3、seedance2.5）；图片模型 3 个标记 `ignored_non_sd`。

## sd 工作表写入（渠道 17）

- 价格更新 5 格：seedance2.0 480p/720p/1080p → 0.3/0.45/0.9，seedance2.0 4k → **99**（页面原文 `99R/秒`，疑似限制性定价，需人工复核），seedance2.5 720p → 0.6。seedance2.5 480p 0.35 与页面一致未写。
- 新增 15 行（row 341–355）：seedance2.5 1080p（0.92）；veo3.1 标准/fast/lite 各 720p/1080p/4k（call，10/0.75/0.4 元/次档）；omni-flash 720p/1080p/4k（call，1.1/2/2.8）；minimax-h3 768p/2k（second，0.2/0.3）。系列约定：veo=`3.1`（同 sd 既有数字系列先例）、omni=`Omni`、minimax-h3=`h3`（同 h3 表先例）；能力列来自渠道合同参数表。
- `draft` 1 项未写入：omni-flash (edit mode) 独立计价行（模式条件价，sd 无模式列，无法构造唯一匹配键）。
- 结构维护：插入点下方 307 个公式单元格行号平移 +15；条件格式 Z3:AD1095 → Z3:AD1110；新行折扣列按 sd 模板重建 ArrayFormula。
- 回读校验：13 表逐单元格（公式引用平移归一化）0 意外差异。

## h3 工作表（渠道 17 H3 系列）

`h3` 表 row 14–15（minimax-h3 768p/2k）在本次同步之前即已存在，逐列核验与页面数据一致（价格 0.2/0.3 元/秒、second、素材 9/3/3 总 12、参考视频总时长 15 秒、时长 4-15、折扣公式查 `h3官价` 768p→0.5/2k→0.8），无需补写；仅将 `sd` 表 row 354/355 系列由 `H3` 对齐为 `h3`。

## 工作簿哈希链

| 阶段 | SHA-256 |
| --- | --- |
| 会话初见（23:28） | `f3302961…67fcc` |
| 外部更新基线（23:47，写入前重读） | `1f8ebf1c…cb193f7` |
| sd 写入后 | `462a84f8…ab42eeb` |
| 系列对齐后（最终） | `ab610545…0354cd0` |

## 过程异常

1. 会话中工作簿被外部进程更新一次（23:28→23:47），重读基线后确认渠道 17 既有行未变再写入。
2. 首次保存因 openpyxl 条件格式改写缺陷失败并短暂截断目标文件，已从写入前备份恢复；此后写入均为「临时文件→加载验证→替换」三步。

未写在线 Google 表格；未执行配置发布/激活。
