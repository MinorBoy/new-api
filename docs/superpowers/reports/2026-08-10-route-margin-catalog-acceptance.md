# 路由毛利目录验收报告

## 结论

“成本核算 -> 路由毛利”已完成默认 30% 毛利率口径的功能验收。默认参数为输出 4 秒、分组倍率 1、同时计算 `no_video` 与 `with_video` 场景；页面、查询 API、CSV 导出和高级筛选均能返回一致结果。默认统计与既有《全局毛利率30-路由目标统计》一致：156 个路由目标中，75 个至少一个场景达标，其中 60 个全部场景达标、15 个部分场景达标。

本轮发现并修复了一个验收缺陷：`with_video` 静态矩阵原先只设置参考视频存在，但输入视频时长为 0。运行时售价预览会将其判定为参考视频时长不可用，导致该场景收入未知。静态矩阵现统一使用 4000ms 的代表性参考视频时长；真实请求仍使用实际媒体元数据时长。

## 功能提交

| 提交 | 内容 |
| --- | --- |
| `31db6caa4` | 增加路由目标查询 |
| `24fa3eda2` | 支持分组倍率收入预览 |
| `65346bd6e` | 实现路由毛利计算、筛选、排序和汇总 |
| `88394c786` | 暴露路由毛利查询与导出 API |
| `0112938f9` | 增加前端客户端和 URL 参数映射 |
| `bed493152` | 增加路由毛利页面、筛选、表格、移动视图和导出 |
| `55c065113` | 补齐七种前端语言翻译 |

## 默认矩阵

请求参数：

```text
min_margin_ppm=300000
duration_seconds=4
group_ratio=1
scenario=all
page_size=100
sort_by=gross_margin_ppm
sort_order=desc
```

| 指标 | 数量 |
| --- | ---: |
| 路由目标 | 156 |
| 场景行 | 312 |
| 达标目标 | 75 |
| 全部达标 | 60 |
| 部分达标 | 15 |
| 未达标目标 | 81 |
| 达标场景行 | 135 |

代表时长敏感性复核：

| 输出时长 | 达标目标 | 全部达标 | 部分达标 | 未达标目标 | 达标场景行 |
| --- | ---: | ---: | ---: | ---: | ---: |
| 3 秒 | 78 | 56 | 22 | 78 | 134 |
| 4 秒 | 75 | 60 | 15 | 81 | 135 |
| 5 秒 | 76 | 64 | 12 | 80 | 140 |

4 秒结果复现既有统计口径，因此作为默认静态矩阵参数。

## API 与导出

- 默认查询 API 返回 HTTP 200，汇总值与页面一致。
- CSV 导出 API 返回 HTTP 200。
- `Content-Disposition` 同时包含 ASCII 文件名和 RFC 5987 UTF-8 中文文件名。
- `X-Exported-Row-Count` 为 `312`。
- CSV 共 313 行，包含 1 行表头和 312 行数据。
- 表头包含路由目标、渠道、规范模型、上游模型、场景、输出时长、预计收入、预计成本、预计利润、毛利率、达标状态和规则来源等审计字段。

## 页面验收

- 桌面视口 1440×900：页面非空，汇总为 `156/312/75/60/15/81/135`，首屏 50 行正常渲染，无页面级横向溢出，长模型名未发生内容溢出。
- 高级模式可以展开；将输出时长改为 5 秒后，URL 写入 `marginDurationSeconds=5`，汇总实时更新为 `76/64/12/80/140`，证明高级筛选参数实际进入查询链路。
- 移动视口 390×844：`documentElement.scrollWidth` 与 `body.scrollWidth` 均为 390，无横向滚动，筛选、汇总和移动结果列表可纵向浏览。
- “供应商成本目录”标签仍可访问，切换后 URL 为 `tab=catalog` 且标签保持选中。

验收截图：

- `docs/superpowers/reports/_shots/2026-08-10-route-margin-desktop.png`
- `docs/superpowers/reports/_shots/2026-08-10-route-margin-mobile.png`

## 自动化验证

| 验证 | 结果 |
| --- | --- |
| `go test ./model ./service ./controller ./router ./relay/helper -count=1` | 通过 |
| `go test ./cmd/ark-video-material-seed -count=1` | 通过；刷新后素材矩阵 187 条，分布为 `431=55`、`900=12`、`903=4`、`933=116` |
| `bun test src/features/cost-accounting` | 67 通过，0 失败 |
| `bun run typecheck` | 通过 |
| `bun run build` | 通过 |
| `git diff --check` | 通过 |

新增回归测试直接断言 `with_video` 收入预览收到 4000ms 代表性参考视频时长，并验证两个场景均可产生确定的毛利结论。

## 全仓测试已知失败

全仓 Go 测试仍保留 1 项供应商契约阻塞。本轮已同步素材种子测试与刷新后的 187 条 fixture，并未修改旧 fixture 或供应商适配器来掩盖问题：

1. `e2e` 的 `TestSeedanceImportedMaterialMatrixFullFlowE2E` 中，OmegaAI 目标 `MAP-OMEGAAI-R193-720`、`MAP-OMEGAAI-R194-720` 预期 HTTP 200，实际因映射模型不受支持返回 HTTP 400。当前 OmegaAI 适配器白名单仅包含供应商文档已确认的 `klsdpro2-720p`、`seedance-v2-720p`、`dola-seedance-2.0`、`lingjing-video-v1`；在没有权威契约证据前不放行新增模型。

## 清理

本轮为浏览器验收创建的临时管理员账号已通过 Root PAT 硬删除。数据库复核该用户、登录会话和 API Token 剩余数量均为 0；报告未记录任何凭据、媒体地址或用户敏感数据。
