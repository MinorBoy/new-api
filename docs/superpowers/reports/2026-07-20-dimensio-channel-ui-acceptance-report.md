# Dimensio 渠道前端验收报告

## 结论

通过。默认前端的管理员“创建渠道”流程现在可以选择 `Dimensio`，并保留现有渠道表单的凭证、模型、模型映射、分组、路由和保存能力。

## 代码改动

- 前端渠道类型注册 `59: Dimensio`，并加入标准类型选择顺序。
- 类型配置提供默认地址 `https://jimeng.dimensio.cn`、原始 API Key 提示和三个支持的上游模型：
  - `jimeng-video-seedance-2.0-fast-vip`
  - `jimeng-video-seedance-2.0-mini`
  - `jimeng-video-seedance-2.0-vip`
- 类型 `59` 保持在通用模型拉取集合之外，避免显示不适用的“从上游获取”。
- 渠道凭证区域显示 ARK `/api/v3` task-only 警告。
- `en`、`zh`、`zh-TW`、`fr`、`ja`、`ru`、`vi` locale 已同步；同步报告为所有 locale `missingCount=0`、`extrasCount=0`、`untranslatedCount=0`。

## 自动化验证

### 前端质量门槛

| 检查 | 结果 |
| --- | --- |
| `npx --yes bun test tests/channel-type-config.test.ts` | 通过，3 tests / 0 failures |
| `npx --yes bun run typecheck` | 通过，`tsgo -b` exit 0 |
| 变更文件 `oxlint` | 通过，0 errors |
| 变更文件 `oxfmt --check` | 通过 |
| `npx --yes bun run i18n:sync` | 通过，7 locales 无缺口 |
| `npx --yes bun run build` | 通过，Rsbuild exit 0 |
| 全库 `format:check` | 未通过；仅剩既有未格式化文件，未修改这些文件 |

全库格式检查失败项来自现有代码和脚本（如 `src/components/`、`src/features/` 的既有文件）；本次修改的 TypeScript、TSX、测试和同步脚本均已通过目标文件格式检查。

### Compose 和镜像

执行：

```text
docker compose -f docker-compose.local.yml up -d --build
```

结果：

- `new-api:local` 镜像构建成功。
- 镜像 ID：`sha256:1331d19589a50ebdf04180728071e7757e5c3647f021ff1bdb81cb9b66a063ef`
- MySQL、Redis、new-api 三个服务均为 `healthy`。
- `GET http://127.0.0.1:3000/api/status` 返回 HTTP 200，`success=true`。
- 容器继续运行于 `http://127.0.0.1:3000`。

### Playwright 浏览器验收

使用 `@playwright/cli` 独立浏览器会话完成：

1. 管理员登录控制台，进入“渠道”，点击“创建渠道”。
2. 类型列表出现 `Dimensio`，图标使用当前适配器的中性 `D` 首字母回退。
3. 选择后验证：
   - 标题变为“创建渠道 Dimensio”；
   - API 地址自动填充 `https://jimeng.dimensio.cn`；
   - 显示原始 Dimensio API Key 提示；
   - 显示三个支持的上游模型；
   - 显示“Dimensio 仅支持任务接口，请通过 ARK /api/v3 任务 API 调用。”；
   - 通用模型拉取按钮不出现；
   - 展开高级设置后，优先级、权重、测试模型、自动封禁、模型映射和保存按钮仍可用。
4. 使用虚拟 Key `acceptance-only-key` 和支持模型 `jimeng-video-seedance-2.0-vip` 完成一次真实的表单提交。页面提示“渠道创建成功”，列表显示 `Dimensio Browser Acceptance`，数据库确认 `type=59` 和默认地址正确。
5. 验收结束后删除测试渠道、测试管理员和测试令牌；数据库确认测试数据均为 0 条。Playwright 控制台错误为 0。

截图：

- [Dimensio 创建表单](../../../output/playwright/dimensio-channel-option.png)
- [创建成功后的渠道列表](../../../output/playwright/dimensio-channel-created.png)

## 限制

本轮没有使用真实 Dimensio API Key，也没有向 `https://jimeng.dimensio.cn` 发起上游请求；验收覆盖的是管理员 UI 配置、type `59` 落库和标准渠道创建链路。
