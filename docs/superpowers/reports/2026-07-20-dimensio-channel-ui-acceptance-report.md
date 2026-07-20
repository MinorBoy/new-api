# Dimensio 渠道前端验收报告

## 结论

通过。默认前端的管理员“创建渠道”流程现在可以选择 `Dimensio`，并保留现有渠道表单的凭证、模型、模型映射、分组、路由和保存能力。任务型 Dimensio 渠道不会再显示或执行通用渠道测试；系统自动填入的其他供应商地址会在切换到 Dimensio 时更新，管理员手工地址仍会保留。

## 代码改动

- 前端渠道类型注册 `59: Dimensio`，并加入标准类型选择顺序。
- 类型配置提供默认地址 `https://jimeng.dimensio.cn`、原始 API Key 提示和三个支持的上游模型：
  - `jimeng-video-seedance-2.0-fast-vip`
  - `jimeng-video-seedance-2.0-mini`
  - `jimeng-video-seedance-2.0-vip`
- 类型 `59` 保持在通用模型拉取集合之外，避免显示不适用的“从上游获取”。
- 渠道凭证区域显示 ARK `/api/v3` task-only 警告。
- 渠道类型切换结合表单 dirty 状态与已知供应商地址区分系统地址和管理员手工地址：新建或编辑时，供应商地址更新为 `https://jimeng.dimensio.cn`，手工地址不覆盖。
- 前端对 Dimensio 隐藏快捷测试、测试对话框入口、菜单测试项和“测试模型”字段；后端同时拒绝 type `59` 的通用渠道测试。
- `en`、`zh`、`zh-TW`、`fr`、`ja`、`ru`、`vi` locale 已同步；同步报告为所有 locale `missingCount=0`、`extrasCount=0`、`untranslatedCount=0`。

## 自动化验证

### 前端质量门槛

| 检查 | 结果 |
| --- | --- |
| `npx --yes bun test tests/channel-type-config.test.ts` | 通过，5 tests / 0 failures |
| `go test ./controller -count=1` | 通过 |
| `go test ./... -count=1` | 通过，包含 `e2e` 和 Dimensio task adaptor |
| `npx --yes bun run typecheck` | 通过，`tsgo -b` exit 0 |
| 变更文件 `oxlint` | 通过，0 errors |
| 变更文件 `oxfmt --check` | 通过 |
| `npx --yes bun run i18n:sync` | 通过，7 locales 无缺口 |
| `npx --yes bun run build` | 通过，Rsbuild exit 0 |
| 全库 `lint` | 未通过；错误均位于本次未修改的既有文件 |
| 全库 `format:check` | 未通过；问题均位于本次未修改的既有文件 |

全库 lint 和格式检查失败项来自现有代码和脚本（如 `src/components/`、`src/features/`、`scripts/` 的既有文件）；本次修改的 TypeScript、TSX 和测试均已通过目标文件 lint 与格式检查。

### Compose 和镜像

执行：

```text
docker compose -f docker-compose.local.yml up -d --build
```

结果：

- `new-api:local` 镜像构建成功。
- 镜像 ID：`sha256:ddc71ef6ac03086420cc7b7fb633ea73bb765217d02b7cdddd55cd0aaf6f7e1e`
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
   - 展开高级设置后，优先级、权重、自动封禁、模型映射和保存按钮仍可用，“测试模型”不出现。
4. 从火山方舟切换到 Dimensio，确认系统地址由 `https://ark.cn-beijing.volces.com` 自动替换为 `https://jimeng.dimensio.cn`。
5. 编辑已有使用北京官方地址的火山方舟渠道并改选 Dimensio，确认地址按规则替换；已有 `https://proxy.example.com` 自定义地址在表单 reset 后仍保持不变。BytePlus 官方地址由单元契约覆盖。
6. 手工输入 `https://proxy.example.com`，在新建表单的 Dimensio 与火山方舟之间往返切换，确认手工地址保持不变。
7. 使用虚拟 Key `acceptance-only-key` 和支持模型 `jimeng-video-seedance-2.0-vip` 完成一次真实的表单提交。页面提示创建成功，列表显示 `Dimensio Regression Acceptance`。
8. 创建后的卡片不显示快捷测试和测试对话框按钮；展开操作菜单后也不显示“测试连接”。
9. 验收结束后删除测试渠道和测试管理员；数据库确认测试数据均为 0 条。Playwright 控制台错误为 0。

本轮回归截图：

- [地址切换与任务型表单](../../../output/playwright/dimensio-channel-regression-form.png)
- [编辑渠道保留自定义地址](../../../output/playwright/dimensio-channel-regression-edit.png)
- [通用测试入口已隐藏](../../../output/playwright/dimensio-channel-regression-test-disabled.png)

## 限制

本轮没有使用真实 Dimensio API Key，也没有向 `https://jimeng.dimensio.cn` 发起上游请求；验收覆盖的是管理员 UI 配置、type `59` 落库和标准渠道创建链路。
